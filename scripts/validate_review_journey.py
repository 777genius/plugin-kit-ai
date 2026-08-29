#!/usr/bin/env python3
"""Validate disposable promotion and contributor journeys at exact Git revisions.

This is deliberately a read-only decision tool.  A successful promotion writes
one deterministic review candidate when ``--candidate-output`` is supplied;
every refusal removes no files and writes no partial candidate.  Submission
validation never creates a PR or invokes a network client.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path, PurePosixPath
from typing import Any
from urllib.parse import urlsplit

import jsonschema

sys.path.insert(0, str(Path(__file__).resolve().parent))
from build_bridges import BridgeError, check_all
from build_registry import (
    DIRECTORY_TREE_DIGEST_ALGORITHM, RegistryError, directory_tree_digest,
    PORTABLE_MAX_FILE_BYTES, PORTABLE_MAX_FILES, PORTABLE_MAX_TREE_BYTES,
    required_components, validate_directory, validate_registry_path,
    validated_package_facts,
    validate_release_package,
)
from validate_catalog import PLUGIN_SCHEMA, ValidationError, validate_plugin


ROOT = Path(__file__).resolve().parents[1]
SHA_RE = __import__("re").compile(r"^[0-9a-f]{40}$")
MERGED_AT_RE = __import__("re").compile(r"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]+)?Z$")
REPOSITORY_RE = __import__("re").compile(r"^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9][A-Za-z0-9._-]*$")
REQUIRED_PROMOTION_GATES = ("pr_metadata", "repository_identity", "default_history", "reviewed_identity", "candidate_identity", "package", "policy", "evidence")
REQUIRED_SUBMISSION_GATES = ("git_fork_branch", "schema", "package", "registry_policy", "bridge_reproduction", "side_effect_boundary")
CLIENT_IDS = ("codex", "chatgpt", "cursor", "copilot", "vscode", "kiro")
MAX_PROMOTION_EVIDENCE = 24
JSON_SAFE_INTEGER_MAX = 9_007_199_254_740_991


class JourneyError(Exception):
    def __init__(self, message: str, **diagnostics: object):
        super().__init__(message)
        self.diagnostics = diagnostics


def require(condition: bool, message: str) -> None:
    if not condition:
        raise JourneyError(message)


def digest(body: bytes) -> str:
    return "sha256:" + hashlib.sha256(body).hexdigest()


def canonical(value: object) -> bytes:
    return (json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n").encode()


def read_object(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        raise JourneyError(f"{path}: invalid JSON: {error}") from error
    require(isinstance(value, dict), f"{path}: expected a JSON object")
    return value


def git(repository: Path, *arguments: str, allow_failure: bool = False) -> subprocess.CompletedProcess[bytes]:
    environment = {
        "PATH": os.environ.get("PATH", ""), "GIT_CONFIG_NOSYSTEM": "1",
        "GIT_CONFIG_GLOBAL": os.devnull, "GIT_TERMINAL_PROMPT": "0",
        "GIT_OPTIONAL_LOCKS": "0", "GIT_NO_LAZY_FETCH": "1", "LANG": "C", "LC_ALL": "C",
    }
    try:
        completed = subprocess.run(
            ["git", *arguments], cwd=repository, env=environment,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False, timeout=60,
        )
    except (OSError, subprocess.TimeoutExpired) as error:
        raise JourneyError(f"Git invocation failed: {error}") from error
    if completed.returncode and not allow_failure:
        detail = completed.stderr.decode("utf-8", "replace").strip()
        raise JourneyError(f"git {' '.join(arguments[:2])} failed: {detail}")
    return completed


def exact_commit(repository: Path, revision: str, field: str) -> str:
    require(SHA_RE.fullmatch(revision) is not None, f"{field} must be a full lowercase Git SHA")
    actual = git(repository, "rev-parse", f"{revision}^{{commit}}").stdout.decode().strip()
    require(actual == revision, f"{field} did not resolve to its exact commit")
    return actual


def safe_git_path(value: str) -> str:
    try:
        return validate_registry_path(value)
    except RegistryError as error:
        raise JourneyError(str(error)) from error


def pr_metadata(path: Path) -> dict[str, Any]:
    metadata = read_object(path)
    fields = {
        "schema_version", "repository_id", "upstream_pr_number", "url", "state",
        "is_draft", "head_ref_oid", "merge_commit_oid", "base_ref_name",
        "default_branch", "merged_at",
    }
    require(set(metadata) == fields and metadata["schema_version"] == 1, "PR metadata has an invalid field set or schema version")
    require(isinstance(metadata["repository_id"], str) and REPOSITORY_RE.fullmatch(metadata["repository_id"]) is not None, "PR metadata repository identity is invalid")
    number = metadata["upstream_pr_number"]
    require(type(number) is int and number > 0, "PR metadata PR number must be positive")
    require(metadata["url"] == f"https://github.com/{metadata['repository_id']}/pull/{number}", "PR metadata URL does not match the official repository and PR")
    require(metadata["state"] == "MERGED" and metadata["is_draft"] is False, "official PR must be non-draft and MERGED")
    require(isinstance(metadata["head_ref_oid"], str) and SHA_RE.fullmatch(metadata["head_ref_oid"]) is not None, "PR metadata headRefOid must be a full lowercase SHA")
    require(isinstance(metadata["merge_commit_oid"], str) and SHA_RE.fullmatch(metadata["merge_commit_oid"]) is not None, "PR metadata merge commit oid must be a full lowercase SHA")
    for field in ("base_ref_name", "default_branch"):
        require(isinstance(metadata[field], str) and metadata[field] and "\n" not in metadata[field], f"PR metadata {field} is invalid")
    require(isinstance(metadata["merged_at"], str) and MERGED_AT_RE.fullmatch(metadata["merged_at"]) is not None, "PR metadata merged timestamp is invalid")
    require(len(canonical(metadata)) <= 16_384, "PR metadata exceeds the canonical size bound")
    return metadata


def github_repository_from_origin(value: str) -> str:
    """Return the unescaped owner/name from one unambiguous GitHub remote."""
    require("\n" not in value and "\r" not in value, "origin URL is ambiguous")
    repository: str | None = None
    if value.startswith("git@github.com:"):
        repository = value.removeprefix("git@github.com:")
    else:
        parsed = urlsplit(value)
        if parsed.scheme == "https":
            require(parsed.hostname == "github.com" and parsed.port in {None, 443}, "origin must use canonical GitHub HTTPS or SSH")
            require(parsed.username is None and parsed.password is None, "credential-bearing origin URLs are forbidden")
            require(not parsed.query and not parsed.fragment, "origin URL is ambiguous")
            repository = parsed.path.removeprefix("/")
        elif parsed.scheme == "ssh":
            require(parsed.hostname == "github.com" and parsed.username == "git" and parsed.password is None and parsed.port in {None, 22}, "origin must use canonical GitHub HTTPS or SSH")
            require(not parsed.query and not parsed.fragment, "origin URL is ambiguous")
            repository = parsed.path.removeprefix("/")
    require(repository is not None, "origin must use canonical GitHub HTTPS or SSH")
    repository = repository.removesuffix(".git")
    require(REPOSITORY_RE.fullmatch(repository) is not None, "origin does not identify one GitHub repository")
    return repository


def bind_official_repository(repository: Path, repository_id: str) -> str:
    require(REPOSITORY_RE.fullmatch(repository_id) is not None, "repository identity is invalid")
    urls = [line for line in git(repository, "remote", "get-url", "--all", "origin").stdout.decode().splitlines() if line]
    require(len(urls) == 1, "origin must have exactly one fetch URL")
    origin_id = github_repository_from_origin(urls[0])
    push_urls = [line for line in git(repository, "remote", "get-url", "--push", "--all", "origin").stdout.decode().splitlines() if line]
    require(len(push_urls) == 1, "origin must have exactly one push URL identity")
    require(github_repository_from_origin(push_urls[0]).casefold() == origin_id.casefold(), "origin fetch and push identities differ")
    require(origin_id.casefold() == repository_id.casefold(), "origin GitHub repository differs from PR metadata")
    return origin_id


def official_default_history(repository: Path, reference: str, candidate: str) -> str:
    require(reference.startswith("refs/remotes/origin/") and not reference.endswith("/HEAD"), "official default ref must be an explicit refs/remotes/origin branch")
    require(git(repository, "show-ref", "--verify", "--quiet", reference, allow_failure=True).returncode == 0, "official default ref is not present in the local clone")
    tip = git(repository, "rev-parse", f"{reference}^{{commit}}").stdout.decode().strip()
    require(SHA_RE.fullmatch(tip) is not None, "official default ref does not resolve to a commit")
    require(git(repository, "merge-base", "--is-ancestor", candidate, reference, allow_failure=True).returncode == 0, "candidate revision is not reachable from the official default ref")
    return tip


def git_tree(repository: Path, revision: str, source: str) -> str | None:
    result = git(repository, "rev-parse", f"{revision}:{source}", allow_failure=True)
    if result.returncode:
        return None
    value = result.stdout.decode().strip()
    kind = git(repository, "cat-file", "-t", value, allow_failure=True)
    return value if kind.returncode == 0 and kind.stdout == b"tree\n" else None


def classify_candidate_bytes(repository: Path, reviewed_revision: str, candidate_revision: str, source: str) -> tuple[str, list[str]]:
    reviewed_tree = git_tree(repository, reviewed_revision, source)
    require(reviewed_tree is not None, f"{source} is absent at reviewed revision")
    candidate_tree = git_tree(repository, candidate_revision, source)
    if candidate_tree == reviewed_tree:
        return "exact", []
    if candidate_tree is not None:
        return "changed", []
    moved = []
    for record in git(repository, "ls-tree", "-rz", "-d", "-r", candidate_revision).stdout.split(b"\0"):
        if not record:
            continue
        metadata, raw_path = record.split(b"\t", 1)
        _mode, kind, object_id = metadata.decode("ascii").split(" ")
        if kind == "tree" and object_id == reviewed_tree:
            moved.append(raw_path.decode("utf-8"))
            require(len(moved) <= 8, "reviewed package bytes occur at too many candidate paths")
    return ("moved", sorted(moved)) if moved else ("missing", [])


def materialize(repository: Path, revision: str, source: str, destination: Path) -> None:
    safe_git_path(source)
    records = git(repository, "ls-tree", "-rz", "-r", revision, "--", source).stdout.split(b"\0")
    prefix = source.rstrip("/") + "/"
    entries: list[tuple[str, str, str, int]] = []
    total_bytes = 0
    for record in records:
        if not record:
            continue
        metadata, raw_path = record.split(b"\t", 1)
        mode, kind, object_id = metadata.decode("ascii").split(" ")
        path = raw_path.decode("utf-8")
        require(kind == "blob" and mode in {"100644", "100755"}, f"unsupported Git entry at {path}")
        require(path.startswith(prefix), f"Git returned a path outside {source}")
        relative = safe_git_path(path[len(prefix):])
        size_text = git(repository, "cat-file", "-s", object_id).stdout.decode("ascii").strip()
        require(size_text.isdigit(), f"Git returned an invalid blob size for {path}")
        size = int(size_text)
        require(size <= PORTABLE_MAX_FILE_BYTES, f"portable package file exceeds {PORTABLE_MAX_FILE_BYTES} bytes: {relative!r}")
        total_bytes += size
        require(total_bytes <= PORTABLE_MAX_TREE_BYTES, f"portable package exceeds {PORTABLE_MAX_TREE_BYTES} total bytes")
        entries.append((mode, object_id, relative, size))
        require(len(entries) <= PORTABLE_MAX_FILES, f"portable package exceeds {PORTABLE_MAX_FILES} files")
    require(entries, f"{source} is absent or empty at {revision}")

    for mode, object_id, relative, size in entries:
        target = destination.joinpath(*PurePosixPath(relative).parts)
        target.parent.mkdir(parents=True, exist_ok=True)
        body = git(repository, "cat-file", "blob", object_id).stdout
        require(len(body) == size, f"Git blob size changed during materialization: {relative}")
        target.write_bytes(body)
        target.chmod(0o755 if mode == "100755" else 0o644)


def package_facts(package: Path) -> dict[str, Any]:
    manifest_body = (package / "plugin.json").read_bytes()
    manifest = json.loads(manifest_body)
    validated = validated_package_facts(package)
    return {
        "manifest_name": manifest["name"], "package_version": manifest.get("version", ""),
        "agent_plugins_schema": manifest["$schema"],
        "manifest_repository": canonical_upstream_manifest_repository(manifest.get("repository")),
        "tree_digest": directory_tree_digest(package), "manifest_digest": digest(manifest_body),
        "components": validated["components"],
    }


def canonical_upstream_manifest_repository(value: object) -> str | None:
    if not isinstance(value, str):
        return None
    parsed = urlsplit(value)
    if parsed.scheme != "https" or parsed.hostname != "github.com" or parsed.port is not None or parsed.username or parsed.password or parsed.query or parsed.fragment:
        return None
    candidate = parsed.path.strip("/")
    return candidate if REPOSITORY_RE.fullmatch(candidate) and not candidate.endswith(".git") else None


def gate(name: str, artifact: object) -> dict[str, Any]:
    return {"name": name, "outcome": "passed", "artifact_digest": digest(canonical(artifact)), "artifact": artifact}


def validate_proposed_release_sequence(directory: dict[str, Any], distribution_id: str, product_id: str, sequence: object) -> None:
    require(
        type(sequence) is int and 1 <= sequence <= JSON_SAFE_INTEGER_MAX,
        "proposed upstream release sequence must be a safe positive integer",
    )
    distributions = [item for item in directory.get("distributions", []) if item.get("id") == distribution_id]
    require(len(distributions) <= 1, "Directory contains a colliding distribution identity")
    if distributions:
        distribution = distributions[0]
        require(distribution["product_id"] == product_id and distribution["kind"] == "upstream", "distribution identity collides with another product or kind")
        highest = max(item["sequence"] for item in distribution["releases"])
        require(highest < JSON_SAFE_INTEGER_MAX, "upstream release sequence exhausted the safe-integer range")
        expected = highest + 1
    else:
        expected = 1
    require(sequence == expected, f"proposed upstream release sequence must be {expected}")


def promotion(args: argparse.Namespace) -> dict[str, Any]:
    metadata = pr_metadata(args.pr_metadata)
    repository_id = metadata["repository_id"]
    pr_number = metadata["upstream_pr_number"]
    reviewed_revision = metadata["head_ref_oid"]
    candidate_revision = metadata["merge_commit_oid"]
    default_ref = f"refs/remotes/origin/{metadata['default_branch']}"
    pr_head_ref = f"refs/remotes/origin/pull/{pr_number}/head"
    require(git(args.repository, "check-ref-format", f"refs/heads/{metadata['default_branch']}", allow_failure=True).returncode == 0, "PR metadata default branch is not a valid Git branch")
    require(git(args.repository, "check-ref-format", f"refs/heads/{metadata['base_ref_name']}", allow_failure=True).returncode == 0, "PR metadata historical base branch is not a valid Git branch")
    record = read_object(args.review_record)
    expected_fields = {
        "schema_version", "repository", "path", "reviewed_revision", "reviewed_tree_digest",
        "reviewed_manifest_digest", "product_id", "manifest_name", "distribution_id",
        "release_sequence", "policy", "evidence",
    }
    require(set(record) == expected_fields and record["schema_version"] == 3, "review record has an invalid field set or schema version")
    origin_id = bind_official_repository(args.repository, repository_id)
    require(record["repository"].casefold() == repository_id.casefold(), "official PR repository differs from reviewed repository")
    directory = read_object(ROOT / "registry/directory.json")
    products = [item for item in directory.get("products", []) if item.get("id") == record["product_id"]]
    require(len(products) == 1, "promotion product is not an existing Directory product")
    product = products[0]
    enforced_components = sorted(required_components(product))
    require(record["manifest_name"] == product["manifest_name"], "review record manifest name differs from the existing Directory product")
    require(str(record["distribution_id"]).split("/", 1)[0].casefold() == repository_id.split("/", 1)[0].casefold(), "upstream distribution publisher differs from the official repository owner")
    require(record["path"] == args.path, "requested package path differs from reviewed path")
    require(record["reviewed_revision"] == reviewed_revision, "official PR head differs from the review record")
    safe_git_path(args.path)
    reviewed_revision = exact_commit(args.repository, reviewed_revision, "reviewed PR head")
    candidate_revision = exact_commit(args.repository, candidate_revision, "official merge commit")
    fetched_pr_head = exact_commit(args.repository, git(args.repository, "rev-parse", f"{pr_head_ref}^{{commit}}").stdout.decode().strip(), "fetched PR head")
    require(fetched_pr_head == reviewed_revision, "fetched PR ref differs from GitHub headRefOid")
    default_tip = official_default_history(args.repository, default_ref, candidate_revision)
    byte_classification, moved_paths = classify_candidate_bytes(args.repository, reviewed_revision, candidate_revision, args.path)
    if byte_classification != "exact":
        raise JourneyError(
            f"official candidate package bytes are {byte_classification} relative to the reviewed PR head",
            byte_classification=byte_classification, moved_paths=moved_paths,
            reviewed_revision=reviewed_revision, candidate_revision=candidate_revision,
        )
    policy = record["policy"]
    require(isinstance(policy, dict) and set(policy) == {"status", "minimum_installer_version", "targets"}, "promotion policy has an invalid field set")
    require(policy["status"] == "active", "promotion policy must be active")
    require(isinstance(policy["minimum_installer_version"], str), "promotion policy requires a minimum installer version")
    targets = policy["targets"]
    require(isinstance(targets, list) and targets, "promotion requires target policy")
    target_clients = [item.get("client") if isinstance(item, dict) else None for item in targets]
    require(target_clients == [client for client in CLIENT_IDS if client in target_clients] and len(set(target_clients)) == len(target_clients), "promotion targets must be unique and in canonical order")
    evidence = record["evidence"]
    require(isinstance(evidence, list) and 0 < len(evidence) <= MAX_PROMOTION_EVIDENCE, "promotion evidence count is empty or exceeds the bound")
    evidence_schema = read_object(ROOT / "schemas/directory-evidence.schema.json")
    for index, item in enumerate(evidence):
        try:
            jsonschema.Draft202012Validator(evidence_schema, format_checker=jsonschema.FormatChecker()).validate(item)
        except jsonschema.ValidationError as error:
            raise JourneyError(f"evidence[{index}] is not valid Directory evidence: {error.message}") from error
    evidence_ids = [item["id"] for item in evidence]
    require(evidence_ids == sorted(set(evidence_ids)), "promotion evidence IDs must be unique and sorted")
    validate_proposed_release_sequence(directory, record["distribution_id"], record["product_id"], record["release_sequence"])

    gates = [
        gate("pr_metadata", {"repository": repository_id, "number": pr_number, "url": metadata["url"], "merged_at": metadata["merged_at"], "state": "MERGED", "is_draft": False}),
        gate("repository_identity", {"repository": origin_id, "path": args.path, "origin_remote": "origin"}),
        gate("default_history", {"reference": default_ref, "tip_revision": default_tip, "candidate_revision": candidate_revision}),
    ]
    with tempfile.TemporaryDirectory(prefix="promotion-review-") as reviewed_tmp, tempfile.TemporaryDirectory(prefix="promotion-candidate-") as candidate_tmp:
        package_name = PurePosixPath(args.path).name
        reviewed_root, candidate_root = Path(reviewed_tmp) / package_name, Path(candidate_tmp) / package_name
        reviewed_root.mkdir()
        candidate_root.mkdir()
        materialize(args.repository, reviewed_revision, args.path, reviewed_root)
        reviewed = package_facts(reviewed_root)
        require(reviewed["tree_digest"] == record["reviewed_tree_digest"], "reviewed package tree digest differs from the review record")
        require(reviewed["manifest_digest"] == record["reviewed_manifest_digest"], "reviewed manifest digest differs from the review record")
        require(reviewed["manifest_name"] == record["manifest_name"], "reviewed manifest name differs from the Directory product manifest name")
        require(reviewed["manifest_repository"] is not None and reviewed["manifest_repository"].casefold() == repository_id.casefold(), "reviewed manifest repository differs from the official repository")
        gates.append(gate("reviewed_identity", {"revision": reviewed_revision, **reviewed}))
        materialize(args.repository, candidate_revision, args.path, candidate_root)
        candidate = package_facts(candidate_root)
        require(candidate["manifest_name"] == record["manifest_name"], "candidate manifest name differs from the Directory product manifest name")
        require(candidate["manifest_repository"] is not None and candidate["manifest_repository"].casefold() == repository_id.casefold(), "candidate manifest repository differs from the official repository")
        require(set(enforced_components).issubset(candidate["components"]), "candidate is missing product minimum capabilities")
        gates.append(gate("candidate_identity", {"revision": candidate_revision, **candidate}))
        proposed_release = {
            "sequence": record["release_sequence"], "package_version": candidate["package_version"],
            "manifest_name": candidate["manifest_name"], "agent_plugins_schema": candidate["agent_plugins_schema"],
            "package_source": {"repository": repository_id, "revision": candidate_revision, "path": args.path},
            "tree_digest_algorithm": DIRECTORY_TREE_DIGEST_ALGORITHM, "tree_digest": candidate["tree_digest"],
            "manifest_digest": candidate["manifest_digest"], "components": candidate["components"],
        }
        validate_release_package(
            candidate_root, proposed_release,
            label=f"{record['distribution_id']}@{record['release_sequence']}",
            require_closed_runtime=True, runtime_policy=policy,
        )
        gates.append(gate("package", {
            "validator": "build_registry.validate_release_package",
            "require_closed_runtime": True, "runtime_policy_enforced": True,
            "minimum_installer_version": policy["minimum_installer_version"],
            "enforced_capabilities": enforced_components, "components": candidate["components"],
        }))
        gates.append(gate("policy", {**policy, "enforced_capabilities": enforced_components}))

        exact_evidence = []
        evidence_tuples: set[tuple[object, ...]] = set()
        for item in evidence:
            require(
                item["product_id"] == record["product_id"]
                and item["distribution_id"] == record["distribution_id"]
                and item["release_sequence"] == record["release_sequence"]
                and item["package_tree_digest"] == candidate["tree_digest"]
                and item["manifest_digest"] == candidate["manifest_digest"]
                and item["source_repository"] == repository_id
                and item["source_revision"] == candidate_revision
                and item["source_path"] == args.path,
                f"{item['id']}: evidence identity does not match the exact official release tuple",
            )
            require(item.get("client") is None or item.get("client") in target_clients, f"{item['id']}: evidence client is not a proposed target")
            evidence_tuple = tuple(item.get(field) for field in (
                "level", "client", "dependency_identity", "client_version",
                "installer_version", "adapter_version", "os", "architecture",
            ))
            require(evidence_tuple not in evidence_tuples, "promotion has multiple evidence records for one Directory applicability tuple")
            evidence_tuples.add(evidence_tuple)
            if (
                item["level"] == "materialization" and item["outcome"] == "passed"
                and item.get("installer_version") == policy["minimum_installer_version"]
                and item.get("client") in target_clients
            ):
                exact_evidence.append(item)
        passed_clients = [client for client in CLIENT_IDS if any(item.get("client") == client for item in exact_evidence)]
        missing_targets = [client for client in target_clients if client not in passed_clients]
        require(not missing_targets, f"promotion lacks exact passed materialization evidence for targets: {','.join(missing_targets)}")
        require(len(exact_evidence) == len(target_clients), "promotion requires exactly one positive materialization record for every target")
        gates.append(gate("evidence", {"evidence_ids": evidence_ids, "passed_materialization_targets": passed_clients}))

    exact_match = candidate["tree_digest"] == reviewed["tree_digest"] and candidate["manifest_digest"] == reviewed["manifest_digest"]
    require(exact_match, "official candidate package bytes are changed relative to the reviewed PR head")
    candidate_output = {
        "schema_version": 1, "decision": "reviewable_promotion_candidate",
        "product": {"id": record["product_id"], "manifest_name": record["manifest_name"]},
        "distribution": {"id": record["distribution_id"], "kind": "upstream"},
        "release": {
            "sequence": record["release_sequence"], "package_version": candidate["package_version"],
            "agent_plugins_schema": candidate["agent_plugins_schema"], "components": candidate["components"],
            "tree_digest_algorithm": DIRECTORY_TREE_DIGEST_ALGORITHM,
            "tree_digest": candidate["tree_digest"], "manifest_digest": candidate["manifest_digest"],
        },
        "policy": policy,
        "source": {
            "repository": repository_id, "path": args.path,
            "upstream_pr_number": pr_number, "upstream_pr_url": metadata["url"], "merged_at": metadata["merged_at"],
            "reviewed_pr_head_sha": reviewed_revision, "official_candidate_sha": candidate_revision,
            "official_default_ref": default_ref, "official_default_tip_sha": default_tip,
            "byte_classification": "exact",
        },
        "evidence": [{
            "id": item["id"], "record_digest": digest(canonical(item)), "level": item["level"],
            "client": item.get("client"), "installer_version": item.get("installer_version"),
            "artifact": item["artifact"],
        } for item in evidence],
        "gate_artifacts": [{"name": item["name"], "artifact_digest": item["artifact_digest"]} for item in gates],
    }
    candidate_schema = read_object(ROOT / "schemas/promotion-candidate.schema.json")
    jsonschema.Draft202012Validator(candidate_schema).validate(candidate_output)
    require(len(canonical(candidate_output)) <= 262_144, "promotion candidate exceeds the canonical size bound")
    if args.candidate_output:
        require(not args.candidate_output.exists(), "refusing to overwrite a promotion candidate")
        args.candidate_output.parent.mkdir(parents=True, exist_ok=True)
        args.candidate_output.write_bytes(canonical(candidate_output))
    return {
        "schema_version": 1, "kind": "promotion", "outcome": "accepted", "exact_match": True,
        "candidate_emitted": bool(args.candidate_output), "candidate_digest": digest(canonical(candidate_output)),
        "candidate": candidate_output, "gates": gates,
        "required_gate_names": list(REQUIRED_PROMOTION_GATES),
    }


def submission_source(record: dict[str, Any], revision: str, facts: dict[str, Any], bridge: dict[str, Any]) -> dict[str, Any]:
    components = facts["components"]
    required = {"skills": "required" if "skills" in components else "optional", "mcp": "required" if "mcp" in components else "optional"}
    distribution_id = record["distribution_id"]
    product_id = record["product_id"]
    return {
        "schema_version": 1,
        "products": [{
            "schema_version": 1, "id": product_id, "display_name": "Fixture Bridge",
            "description": "Disposable external contributor validation fixture.", "manifest_name": product_id,
            "aliases": [product_id], "reserved_aliases": [product_id], "categories": ["bridge"],
            "minimum_capabilities": required, "default_distribution": distribution_id,
            "distributions": [distribution_id],
        }],
        "distributions": [{
            "schema_version": 1, "id": distribution_id, "product_id": product_id,
            "kind": "community_bridge", "status": "active", "packager": distribution_id.split("/", 1)[0],
            "releases": [{
                "sequence": 1, "package_version": facts["package_version"], "manifest_name": product_id,
                "agent_plugins_schema": PLUGIN_SCHEMA,
                "package_source": {"repository": record["fork_repository"], "revision": revision, "path": record["package_path"]},
                "build_provenance": {"upstream_repository": bridge["upstream_repository"], "upstream_revision": bridge["upstream_revision"]},
                "tree_digest_algorithm": DIRECTORY_TREE_DIGEST_ALGORITHM, "tree_digest": facts["tree_digest"],
                "manifest_digest": facts["manifest_digest"], "components": components,
            }],
            "release_policies": [{
                "release_sequence": 1, "status": "active", "minimum_installer_version": "0.1.8",
                "targets": [{
                    "client": "codex", "scopes": ["user"], "delivery": "managed",
                    "authentication": "not_required",
                }],
                "current_evidence": [],
            }],
        }],
        "evidence": [],
    }


def submission(args: argparse.Namespace) -> dict[str, Any]:
    record = read_object(args.submission_record)
    expected_fields = {"schema_version", "fork_repository", "base_revision", "branch", "branch_revision", "package_path", "bridge_root", "bridge_id", "product_id", "distribution_id"}
    require(set(record) == expected_fields and record["schema_version"] == 1, "submission record has an invalid field set or schema version")
    require(REPOSITORY_RE.fullmatch(record["fork_repository"]) is not None, "fork repository identity is invalid")
    require(record["branch"] not in {"main", "master"} and record["branch"].startswith("contribution/"), "submission must use a contribution branch")
    base = exact_commit(args.repository, record["base_revision"], "base revision")
    revision = exact_commit(args.repository, record["branch_revision"], "branch revision")
    head = git(args.repository, "rev-parse", "HEAD^{commit}").stdout.decode().strip()
    branch = git(args.repository, "branch", "--show-current").stdout.decode().strip()
    require(head == revision and branch == record["branch"], "checked-out fork branch identity differs from the submission")
    require(git(args.repository, "merge-base", "--is-ancestor", base, revision, allow_failure=True).returncode == 0, "submission branch is not based on the pinned base")
    require(base != revision, "submission branch has no contribution commit")
    origin = git(args.repository, "remote", "get-url", "origin").stdout.decode().strip()
    require(not origin.startswith(("http://", "https://", "ssh://", "git@")), "submission validation requires a local disposable fork remote")
    gates = [gate("git_fork_branch", {"base_revision": base, "branch_revision": revision, "branch": branch, "origin_kind": "local"})]

    package = args.repository / safe_git_path(record["package_path"])
    bridge_root = args.repository if record["bridge_root"] == "." else args.repository / safe_git_path(record["bridge_root"])
    manifest = read_object(package / "plugin.json")
    schema = read_object(ROOT / "schemas/1.0.0/plugin.schema.json")
    jsonschema.Draft202012Validator(schema).validate(manifest)
    gates.append(gate("schema", {"schema": PLUGIN_SCHEMA, "manifest_digest": digest((package / "plugin.json").read_bytes())}))
    facts = package_facts(package)
    require(facts["manifest_name"] == record["product_id"], "submission product and manifest identities differ")
    gates.append(gate("package", {"validator": "validate_catalog.validate_plugin", **facts}))
    reports = check_all(bridge_root, args.upstream_mirror)
    report = next((item for item in reports if item["bridge_id"] == record["bridge_id"]), None)
    require(report is not None, "submitted bridge was not reproduced")
    require(report["tree_digest"] == facts["tree_digest"] and report["manifest_digest"] == facts["manifest_digest"], "bridge reproduction differs from submitted package")
    source = submission_source(record, revision, facts, report)
    validate_directory(source, verify_packages=False)
    gates.append(gate("registry_policy", {"validator": "build_registry.validate_directory", "source_digest": digest(canonical(source)), "status": "active", "targets": ["codex"]}))
    gates.append(gate("bridge_reproduction", {"validator": "build_bridges.check_all", "report_digest": digest(canonical(report)), "tree_digest": report["tree_digest"]}))
    gates.append(gate("side_effect_boundary", {"remote_kind": "local", "network_commands": 0, "pr_created": 0, "publication_created": 0}))
    return {
        "schema_version": 1, "kind": "submission", "outcome": "accepted",
        "repository": record["fork_repository"], "base_revision": base, "branch_revision": revision,
        "branch": branch, "package": facts, "gates": gates,
        "required_gate_names": list(REQUIRED_SUBMISSION_GATES),
        "side_effects": {"network_commands": 0, "pr_created": 0, "publication_created": 0},
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)
    promote = subparsers.add_parser("promotion")
    promote.add_argument("--repository", type=Path, required=True)
    promote.add_argument("--pr-metadata", type=Path, required=True)
    promote.add_argument("--path", required=True)
    promote.add_argument("--review-record", type=Path, required=True)
    promote.add_argument("--candidate-output", type=Path)
    submit = subparsers.add_parser("submission")
    submit.add_argument("--repository", type=Path, required=True)
    submit.add_argument("--submission-record", type=Path, required=True)
    submit.add_argument("--upstream-mirror", type=Path, required=True)
    args = parser.parse_args()
    try:
        result = promotion(args) if args.command == "promotion" else submission(args)
    except (JourneyError, ValidationError, RegistryError, BridgeError, jsonschema.ValidationError, OSError, ValueError, json.JSONDecodeError) as error:
        failure = {"schema_version": 1, "kind": args.command, "outcome": "rejected", "reason": str(error)}
        if isinstance(error, JourneyError):
            failure.update(error.diagnostics)
        print(json.dumps(failure, sort_keys=True, separators=(",", ":")))
        return 2
    print(json.dumps(result, sort_keys=True, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
