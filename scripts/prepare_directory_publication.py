#!/usr/bin/env python3
"""Validate canonical Directory source and build a no-secret publication candidate."""

from __future__ import annotations

import argparse
import copy
import hashlib
import os
import stat
import subprocess
import sys
import tempfile
import time
from pathlib import Path
from typing import Any

sys.path.insert(0, str(Path(__file__).resolve().parent))
from build_registry import (
    RegistryError,
    directory_tree_digest as package_tree_digest,
    policy_eligibility_broadened,
    release_eligibility_broadened,
    releases_requiring_validation,
    validate_active_local_runtime_closures,
    validate_bridge_bindings,
    validate_changed_external_releases,
    validate_release_package,
)
from directory_publication import (
    CANDIDATE_SCHEMA,
    PublicationError,
    SHA_RE,
    atomic_write,
    candidate_digest,
    canonical_json,
    load_ledger_latest,
    load_public_keys,
    parse_json_bytes,
    read_json,
    require,
    validate_with_schema,
)
from directory_publication_cas import CasError, validate_marker
from sequence_boundaries import parse_public_sequence
from publication_trust_policy import (
    load_publication_trust_config,
    validate_publication_eligibility_trust,
)


ROOT = Path(__file__).resolve().parents[1]
SOURCE_SCHEMA = ROOT / "schemas" / "directory-source.schema.json"
EVIDENCE_ARTIFACT_SCHEMA = ROOT / "schemas" / "directory-evidence-artifact.schema.json"
LAUNCH_EVIDENCE_SCHEMA = ROOT / "tests" / "e2e" / "schemas" / "launch-evidence.schema.json"
GIT = "/usr/bin/git"
GH = "/usr/bin/gh"
ACQUISITION_TIMEOUT_SECONDS = 180
MAX_PLUGIN_FILES = 10_000
MAX_PLUGIN_FILE_BYTES = 16 << 20
MAX_PLUGIN_TREE_BYTES = 64 << 20
MAX_EVIDENCE_BYTES = 4 << 20
PUBLIC_EVIDENCE_FIELDS = (
    "schema_version", "id", "distribution_id", "release_sequence",
    "package_tree_digest", "level", "outcome", "client", "client_version",
    "installer_version", "os", "architecture", "dependency_identity",
    "observed_at", "artifact",
)


def public_evidence_id(evidence_id: str) -> str:
    """Give the schema-1 wire projection its own immutable identity."""
    return f"{evidence_id}.wire-v1"


def public_evidence_projection(record: dict[str, Any]) -> dict[str, Any]:
    """Project attested evidence onto the immutable schema-1 wire contract."""
    projected = {
        field: copy.deepcopy(record[field])
        for field in PUBLIC_EVIDENCE_FIELDS
        if field in record
    }
    projected["id"] = public_evidence_id(record["id"])
    trust = record["trust"]
    if trust["kind"] == "github_actions":
        projected["trust"] = {
            "kind": trust["kind"],
            "workflow": trust["workflow"],
            "source_ref": trust["source_ref"],
            # The private pointer's digest authenticates the protected workflow
            # source.  The public contract instead binds the signer-vouched
            # projection to the immutable artifact revision materialized above.
            "source_digest": projected["artifact"]["revision"],
        }
    else:
        projected["trust"] = {"kind": "reviewed_external"}
    return projected


def validate_projected_evidence_ids(
    records: list[tuple[str, dict[str, Any]]],
    historical_evidence: dict[str, dict[str, Any]] | None,
) -> None:
    """Reject a source identity that aliases different immutable public evidence."""
    if not historical_evidence:
        return
    for source_id, public_record in records:
        public_id = public_record["id"]
        historical = historical_evidence.get(public_id)
        if historical is not None and historical != public_record:
            require(
                False,
                f"{source_id}: projected public evidence ID {public_id!r} collides with "
                "a different immutable historical evidence record; choose a new source evidence ID",
            )


def repository_override(overrides: dict[str, Path], repository: str) -> Path | None:
    """Resolve GitHub repository identities without case-sensitive bypasses."""
    identity = repository.casefold()
    return next((path for name, path in overrides.items() if name.casefold() == identity), None)


load_config = load_publication_trust_config


def validate_local_evidence_anchor(
    config: dict[str, Any], repository_root: Path, evidence: list[dict[str, Any]],
) -> None:
    local_repository = config["repository"].casefold()
    artifacts = [
        item["artifact"] for item in evidence
        if item.get("trust", {}).get("kind") == "reviewed_external"
        and item["artifact"]["repository"].casefold() == local_repository
    ]
    if not artifacts:
        return
    anchor = config.get("local_evidence_main_anchor")
    require(
        isinstance(anchor, str) and SHA_RE.fullmatch(anchor) is not None,
        "catalog-local reviewed evidence requires a durable main anchor",
    )
    anchor_reachable = subprocess.run(
        [GIT, "-C", str(repository_root), "merge-base", "--is-ancestor", anchor, "HEAD"],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE,
    )
    require(
        anchor_reachable.returncode == 0,
        f"local evidence main anchor {anchor} is unavailable from source HEAD",
    )
    for artifact in artifacts:
        reachable = subprocess.run(
            [
                GIT, "-C", str(repository_root), "merge-base", "--is-ancestor",
                artifact["revision"], anchor,
            ],
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )
        require(
            reachable.returncode == 0,
            f"{artifact['revision']} is not durable from local evidence main anchor {anchor}",
        )


def previous_releases(snapshot: dict[str, Any] | None) -> dict[tuple[str, int], dict[str, Any]]:
    if snapshot is None:
        return {}
    return {
        (distribution["id"], release["sequence"]): release
        for distribution in snapshot["distributions"]
        for release in distribution["releases"]
    }


def previous_distributions(snapshot: dict[str, Any] | None) -> dict[str, dict[str, Any]]:
    return {} if snapshot is None else {item["id"]: item for item in snapshot["distributions"]}


def policy_map(distribution: dict[str, Any]) -> dict[int, dict[str, Any]]:
    return {item["release_sequence"]: item for item in distribution["release_policies"]}


def eligibility_broadened(
    distribution: dict[str, Any], policy: dict[str, Any],
    old_distribution: dict[str, Any] | None, old_policy: dict[str, Any] | None,
) -> bool:
    return policy_eligibility_broadened(
        distribution, policy, old_distribution, old_policy,
    )


def manifest_digest(package_root: Path) -> str:
    manifest = package_root / "plugin.json"
    require(manifest.is_file(), f"{package_root}: plugin.json is missing")
    return "sha256:" + hashlib.sha256(manifest.read_bytes()).hexdigest()


def verify_package(
    package_root: Path, release: dict[str, Any], identity: str, *,
    require_closed_runtime: bool = True,
) -> None:
    try:
        validate_release_package(
            package_root, release, label=identity,
            allow_unresolved_revision=release["package_source"]["revision"] is None,
            require_closed_runtime=require_closed_runtime,
        )
    except RegistryError as error:
        raise PublicationError(str(error)) from error


def acquisition_environment(temporary_root: Path) -> dict[str, str]:
    return {
        "PATH": "/usr/bin:/bin",
        "HOME": str(temporary_root),
        "GIT_CONFIG_NOSYSTEM": "1",
        "GIT_CONFIG_GLOBAL": os.devnull,
        "GIT_TERMINAL_PROMPT": "0",
        "GIT_ASKPASS": os.devnull,
        "SSH_ASKPASS": os.devnull,
        "GIT_LFS_SKIP_SMUDGE": "1",
        "GIT_OPTIONAL_LOCKS": "0",
        "LC_ALL": "C",
    }


def inspect_plugin_root(package_root: Path, identity: str) -> None:
    require(package_root.is_dir(), f"{identity}: package path is unavailable")
    file_count = 0
    total_bytes = 0
    for path in package_root.rglob("*"):
        metadata = path.lstat()
        if stat.S_ISDIR(metadata.st_mode):
            continue
        relative = path.relative_to(package_root)
        require(
            stat.S_ISREG(metadata.st_mode) or stat.S_ISLNK(metadata.st_mode),
            f"{identity}: package contains unsupported special file {relative}",
        )
        file_count += 1
        total_bytes += metadata.st_size
        require(file_count <= MAX_PLUGIN_FILES, f"{identity}: package exceeds {MAX_PLUGIN_FILES} files")
        require(metadata.st_size <= MAX_PLUGIN_FILE_BYTES, f"{identity}: file {relative} exceeds {MAX_PLUGIN_FILE_BYTES} bytes")
        require(total_bytes <= MAX_PLUGIN_TREE_BYTES, f"{identity}: package exceeds {MAX_PLUGIN_TREE_BYTES} bytes")
        if stat.S_ISLNK(metadata.st_mode):
            continue
        with path.open("rb") as source:
            prefix = source.read(256)
        require(
            not prefix.startswith(b"version https://git-lfs.github.com/spec/v1\n"),
            f"{identity}: Git LFS pointer {relative} is unsupported",
        )


def acquire_external(repository: str, revision: str, package_path: str, override: Path | None = None) -> tempfile.TemporaryDirectory[str]:
    temporary = tempfile.TemporaryDirectory(prefix="directory-publication-")
    checkout = Path(temporary.name) / "checkout"
    source = override.resolve().as_uri() if override is not None else f"https://github.com/{repository}.git"
    environment = acquisition_environment(Path(temporary.name))
    deadline = time.monotonic() + ACQUISITION_TIMEOUT_SECONDS

    def git(*arguments: str, text: bool = False) -> subprocess.CompletedProcess[Any]:
        remaining = deadline - time.monotonic()
        require(remaining > 0, f"{repository}@{revision}//{package_path}: reacquisition timed out")
        return subprocess.run(
            [
                GIT, "-c", "credential.helper=", "-c", "core.askPass=", "-c", "submodule.recurse=false",
                "-c", "http.followRedirects=false",
                *arguments,
            ],
            check=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=remaining,
            env=environment,
            text=text,
        )

    try:
        git("init", "--quiet", str(checkout))
        if package_path != ".":
            git("-C", str(checkout), "sparse-checkout", "init", "--no-cone")
            git("-C", str(checkout), "sparse-checkout", "set", "--no-cone", f"/{package_path}/")
        git(
            "-C", str(checkout), "-c", "protocol.file.allow=always", "fetch", "--quiet",
            "--no-tags", "--no-recurse-submodules", "--depth=1", "--filter=blob:none", source, revision,
        )
        resolved = git("-C", str(checkout), "rev-parse", "FETCH_HEAD", text=True).stdout.strip()
        require(resolved == revision, f"{repository}@{revision}: reacquisition resolved {resolved}")
        tree = git("-C", str(checkout), "ls-tree", "-r", "-z", "FETCH_HEAD", "--", package_path).stdout
        entries = tree.split(b"\0") if isinstance(tree, bytes) else tree.encode().split(b"\0")
        for entry in entries:
            if not entry:
                continue
            metadata, raw_path = entry.split(b"\t", 1)
            if metadata.split()[:2] == [b"160000", b"commit"]:
                path = raw_path.decode("utf-8", "strict")
                relative = path if package_path == "." else path.removeprefix(package_path.rstrip("/") + "/")
                component_owned = relative in {"plugin.json", "mcp.json", "skills"} or relative.startswith("skills/")
                require(
                    package_path == "." and not component_owned,
                    f"{repository}@{revision}//{package_path}: Git submodule content is unsupported",
                )
        package_object = "FETCH_HEAD^{tree}" if package_path == "." else f"FETCH_HEAD:{package_path}"
        kind = git("-C", str(checkout), "cat-file", "-t", package_object, text=True).stdout.strip()
        require(kind == "tree", f"{repository}@{revision}//{package_path}: package path is unavailable")
        git("-C", str(checkout), "checkout", "--quiet", "--detach", "--no-recurse-submodules", "FETCH_HEAD")
        inspect_plugin_root(checkout / package_path, f"{repository}@{revision}//{package_path}")
        return temporary
    except PublicationError:
        temporary.cleanup()
        raise
    except (OSError, subprocess.SubprocessError) as error:
        temporary.cleanup()
        raise PublicationError(f"{repository}@{revision}//{package_path}: reacquisition failed: {error}") from error


def acquire_evidence_bytes(
    artifact: dict[str, str], override: Path | None = None,
) -> tuple[tempfile.TemporaryDirectory[str], bytes]:
    """Acquire one exact evidence blob without executing repository content."""
    repository, revision, artifact_path = (
        artifact["repository"], artifact["revision"], artifact["path"]
    )
    require(not artifact_path.startswith("/") and "\\" not in artifact_path and ".." not in Path(artifact_path).parts, "evidence artifact path is unsafe")
    temporary = tempfile.TemporaryDirectory(prefix="directory-evidence-")
    checkout = Path(temporary.name) / "checkout"
    source = override.resolve().as_uri() if override is not None else f"https://github.com/{repository}.git"
    environment = acquisition_environment(Path(temporary.name))
    label = f"{repository}@{revision}//{artifact_path}"
    try:
        subprocess.run([GIT, "init", "--quiet", str(checkout)], check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=environment)
        subprocess.run(
            [GIT, "-C", str(checkout), "-c", "credential.helper=", "-c", "core.askPass=", "-c", "submodule.recurse=false",
             "-c", "protocol.file.allow=always", "fetch", "--quiet", "--no-tags", "--no-recurse-submodules", "--depth=1", "--filter=blob:none", source, revision],
            check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=ACQUISITION_TIMEOUT_SECONDS, env=environment,
        )
        resolved = subprocess.check_output([GIT, "-C", str(checkout), "rev-parse", "FETCH_HEAD"], text=True, env=environment).strip()
        require(resolved == revision, f"{label}: reacquisition resolved {resolved}")
        object_name = f"FETCH_HEAD:{artifact_path}"
        kind_result = subprocess.run(
            [GIT, "-C", str(checkout), "cat-file", "-t", object_name], check=False,
            text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=environment,
        )
        require(kind_result.returncode == 0 and kind_result.stdout.strip() == "blob", f"{label}: evidence artifact is unavailable or not a regular file")
        size_text = subprocess.check_output([GIT, "-C", str(checkout), "cat-file", "-s", object_name], text=True, env=environment).strip()
        require(size_text.isdigit() and int(size_text) <= MAX_EVIDENCE_BYTES, f"{label}: evidence artifact exceeds {MAX_EVIDENCE_BYTES} bytes")
        body = subprocess.check_output([GIT, "-C", str(checkout), "show", object_name], env=environment, timeout=ACQUISITION_TIMEOUT_SECONDS)
        require(len(body) == int(size_text), f"{label}: evidence artifact size changed during acquisition")
        return temporary, body
    except PublicationError:
        temporary.cleanup()
        raise
    except (OSError, subprocess.SubprocessError) as error:
        temporary.cleanup()
        raise PublicationError(f"{label}: evidence reacquisition failed: {error}") from error


def verify_evidence_trust(
    pointer: dict[str, Any], config: dict[str, Any], temporary_root: Path, body: bytes,
    manifest_body: bytes | None = None, launch_body: bytes | None = None,
    observer_body: bytes | None = None, index_body: bytes | None = None,
) -> None:
    artifact = pointer["artifact"]
    trust = pointer["trust"]
    if trust["kind"] == "reviewed_external":
        require(
            any(artifact == allowed for allowed in config["trusted_external_evidence"]),
            f"{pointer['id']}: external evidence artifact is not explicitly trusted",
        )
        return
    workflow = trust["workflow"]
    policy = next((item for item in config["trusted_evidence_workflows"] if item["workflow"] == workflow), None)
    require(policy is not None, f"{pointer['id']}: evidence workflow has no reviewed trust policy")
    require(workflow.startswith(artifact["repository"] + "/.github/workflows/"), f"{pointer['id']}: workflow and artifact repositories differ")
    require(trust["source_ref"] == policy["protected_source_ref"], f"{pointer['id']}: evidence source ref is not trusted")
    manifest_artifact = trust.get("bundle_manifest")
    launch_artifact = trust.get("launch_artifact")
    observer_artifact = trust.get("observer_artifact")
    evidence_index = trust.get("evidence_index")
    if policy["source_digest_policy"] == "artifact_revision":
        require(
            trust["source_digest"] == artifact["revision"]
            and all(item is None for item in (manifest_artifact, launch_artifact, observer_artifact, evidence_index)),
            f"{pointer['id']}: evidence source digest does not match the exact artifact revision",
        )
        verified_body = body
    else:
        require(
            all(isinstance(item, dict) for item in (manifest_artifact, launch_artifact, observer_artifact, evidence_index))
            and all(item is not None for item in (manifest_body, launch_body, observer_body, index_body)),
            f"{pointer['id']}: protected workflow evidence lacks its complete canonical bundle chain",
        )
        assert isinstance(manifest_artifact, dict) and isinstance(launch_artifact, dict)
        assert isinstance(observer_artifact, dict) and isinstance(evidence_index, dict)
        assert manifest_body is not None and launch_body is not None and observer_body is not None and index_body is not None
        chain = (artifact, manifest_artifact, launch_artifact, observer_artifact, evidence_index)
        require(
            len({item["repository"] for item in chain}) == 1
            and len({item["revision"] for item in chain}) == 1,
            f"{pointer['id']}: evidence chain crosses repository revisions",
        )
        for label, artifact_identity, acquired_body in (
            ("bundle manifest", manifest_artifact, manifest_body),
            ("launch artifact", launch_artifact, launch_body),
            ("observer artifact", observer_artifact, observer_body),
            ("evidence index", evidence_index, index_body),
        ):
            require(
                "sha256:" + hashlib.sha256(acquired_body).hexdigest() == artifact_identity["digest"],
                f"{pointer['id']}: {label} digest mismatch",
            )
        manifest = parse_json_bytes(manifest_body, f"bundle manifest for {pointer['id']}", max_bytes=MAX_EVIDENCE_BYTES)
        index = parse_json_bytes(index_body, f"evidence index for {pointer['id']}", max_bytes=MAX_EVIDENCE_BYTES)
        require(isinstance(manifest, dict) and canonical_json(manifest) == manifest_body, f"{pointer['id']}: bundle manifest is not canonical JSON")
        require(isinstance(index, dict) and canonical_json(index) == index_body, f"{pointer['id']}: evidence index is not canonical JSON")
        manifest_path = Path(manifest_artifact["path"])
        require(manifest_path.name == "bundle-identity.json", f"{pointer['id']}: attested artifact is not the canonical bundle identity")
        root = manifest_path.parent.as_posix()
        require(
            launch_artifact["path"] == f"{root}/launch-evidence.json"
            and observer_artifact["path"] == f"{root}/signed-observer-bundle.json"
            and evidence_index["path"] == f"{root}/directory-evidence/index.json"
            and len(index.get("records", [])) == 16
            and index.get("repository") == artifact["repository"]
            and index.get("workflow") == workflow
            and index.get("source_ref") == trust["source_ref"]
            and index.get("source_digest") == trust["source_digest"],
            f"{pointer['id']}: evidence index identity mismatch",
        )
        with tempfile.TemporaryDirectory(prefix="uap-rederive-", dir=temporary_root) as bundle_directory:
            bundle_path = Path(bundle_directory)
            (bundle_path / "launch-evidence.json").write_bytes(launch_body)
            (bundle_path / "signed-observer-bundle.json").write_bytes(observer_body)
            from materialize_launch_evidence import build_bundle
            derived_digest, derived = build_bundle(
                bundle_path, repository=index["repository"], workflow=index["workflow"],
                source_ref=index["source_ref"], source_digest=index["source_digest"],
                expected_run_id=index["workflow_run_id"],
                expected_run_attempt=index["workflow_run_attempt"],
                expected_caller_event_name=index["caller_event_name"],
                expected_caller_ref=index["caller_ref"],
                expected_caller_workflow_ref=index["caller_workflow_ref"],
                expected_publication_id=index["publication_id"],
                expected_sequence=index["publication_sequence"],
                expected_snapshot_digest=index["publication_snapshot_digest"],
                expected_source_commit=index["publication_source_commit"],
            )
        require(derived_digest == index["launch_evidence_digest"], f"{pointer['id']}: re-derived launch digest mismatch")
        require(derived["bundle-identity.json"] == manifest_body, f"{pointer['id']}: bundle manifest was not deterministically derived")
        require(derived["directory-evidence/index.json"] == index_body, f"{pointer['id']}: evidence index was not deterministically derived")
        record = next((item for item in index.get("records", []) if item.get("id") == pointer["id"]), None)
        require(
            isinstance(record, dict)
            and artifact["path"] == f"{root}/{record.get('path', '')}"
            and artifact["digest"] == record.get("digest")
            and derived.get(record.get("path", "")) == body,
            f"{pointer['id']}: evidence leaf is not bound by the attested launch index",
        )
        verified_body = manifest_body
    require(Path(GH).is_file(), f"reviewed evidence verifier is missing: {GH}")
    acquired = temporary_root / "verified-evidence-bundle-identity.json"
    acquired.write_bytes(verified_body)
    command = [
        GH, "attestation", "verify", str(acquired), "--repo", artifact["repository"],
        "--signer-workflow", workflow, "--source-ref", trust["source_ref"],
        "--source-digest", trust["source_digest"],
    ]
    if not policy["allow_self_hosted_runners"]:
        command.append("--deny-self-hosted-runners")
    completed = subprocess.run(
        command,
        check=False, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        env=acquisition_environment(temporary_root), timeout=ACQUISITION_TIMEOUT_SECONDS,
    )
    require(completed.returncode == 0, f"{pointer['id']}: trusted workflow attestation verification failed")


def verified_evidence(
    pointer: dict[str, Any], config: dict[str, Any], overrides: dict[str, Path],
) -> dict[str, Any]:
    require("trust" in pointer, f"{pointer['id']}: evidence has no trusted workflow or external-attestation path")
    artifact = pointer["artifact"]
    temporary, body = acquire_evidence_bytes(
        artifact, repository_override(overrides, artifact["repository"]),
    )
    chained: list[tempfile.TemporaryDirectory[str]] = []
    try:
        require("sha256:" + hashlib.sha256(body).hexdigest() == artifact["digest"], f"{pointer['id']}: evidence artifact digest mismatch")
        # Parse the verified bytes directly; never derive signed fields from the
        # contributor-authored pointer.
        payload = parse_json_bytes(body, f"evidence {pointer['id']}", max_bytes=MAX_EVIDENCE_BYTES)
        require(isinstance(payload, dict), f"{pointer['id']}: evidence artifact must be an object")
        validate_with_schema(payload, EVIDENCE_ARTIFACT_SCHEMA)
        require(payload["id"] == pointer["id"], f"{pointer['id']}: evidence artifact identity mismatch")
        manifest_body = None
        launch_body = None
        observer_body = None
        index_body = None
        trust = pointer["trust"]
        if trust["kind"] == "github_actions" and "bundle_manifest" in trust:
            for name, destination in (
                ("bundle_manifest", "manifest"), ("launch_artifact", "launch"),
                ("observer_artifact", "observer"), ("evidence_index", "index"),
            ):
                chained_temporary, chained_body = acquire_evidence_bytes(
                    trust[name], repository_override(overrides, trust[name]["repository"]),
                )
                chained.append(chained_temporary)
                require(
                    "sha256:" + hashlib.sha256(chained_body).hexdigest() == trust[name]["digest"],
                    f"{pointer['id']}: {destination} artifact digest mismatch",
                )
                if destination == "manifest": manifest_body = chained_body
                elif destination == "launch": launch_body = chained_body
                elif destination == "observer": observer_body = chained_body
                else: index_body = chained_body
        verify_evidence_trust(
            pointer, config, Path(temporary.name), body,
            manifest_body=manifest_body, launch_body=launch_body,
            observer_body=observer_body, index_body=index_body,
        )
        return {
            **copy.deepcopy(payload),
            "artifact": copy.deepcopy(artifact),
            "trust": copy.deepcopy(pointer["trust"]),
        }
    finally:
        for chained_temporary in chained:
            chained_temporary.cleanup()
        temporary.cleanup()


def selected_evidence(
    source: dict[str, Any], published_distributions: set[str], config: dict[str, Any],
    overrides: dict[str, Path], historical_evidence: dict[str, dict[str, Any]] | None = None,
) -> list[dict[str, Any]]:
    evidence = {item["id"]: item for item in source["evidence"]}
    require(len(evidence) == len(source["evidence"]), "duplicate evidence identity")
    selected: set[str] = set()
    verified: dict[str, dict[str, Any]] = {}
    releases = {
        (distribution["id"], release["sequence"]): (distribution["product_id"], release)
        for distribution in source["distributions"] for release in distribution["releases"]
    }
    for distribution in source["distributions"]:
        if distribution["id"] not in published_distributions:
            continue
        for policy in distribution["release_policies"]:
            identity = (distribution["id"], policy["release_sequence"])
            require(identity in releases, f"{distribution['id']}: policy references missing release {policy['release_sequence']}")
            for evidence_id in policy["current_evidence"]:
                require(evidence_id in evidence, f"{distribution['id']}@{policy['release_sequence']}: missing evidence {evidence_id}")
                record = verified.get(evidence_id)
                if record is None:
                    record = verified_evidence(evidence[evidence_id], config, overrides)
                    verified[evidence_id] = record
                require((record["distribution_id"], record["release_sequence"]) == identity, f"{evidence_id}: evidence release identity mismatch")
                product_id, release = releases[identity]
                source_identity = release["package_source"]
                require(
                    record["product_id"] == product_id
                    and record["package_tree_digest"] == release["tree_digest"]
                    and record["manifest_digest"] == release["manifest_digest"]
                    and record["source_repository"] == source_identity["repository"]
                    and record["source_revision"] == source_identity["revision"]
                    and record["source_path"] == source_identity["path"],
                    f"{evidence_id}: evidence source identity mismatch",
                )
                selected.add(evidence_id)
    source_ids = sorted(selected)
    projected = [public_evidence_projection(verified[item]) for item in source_ids]
    validate_projected_evidence_ids(list(zip(source_ids, projected)), historical_evidence)
    return projected


validate_upstream_default_evidence = validate_publication_eligibility_trust


def validate_reproduced_bridges(
    source: dict[str, Any], repository_root: Path, repository: str,
    previous: dict[str, Any] | None,
) -> None:
    """Reproduce every local bridge binding that gains or changes eligibility."""
    try:
        # First reject cheap source/recipe/package mismatches without network.
        validate_bridge_bindings(
            source, repository_root=repository_root, repository=repository,
        )
        old_distributions = previous_distributions(previous)
        products = {item["id"]: item for item in source["products"]}
        old_products = {
            item["id"]: item for item in previous["products"]
        } if previous else {}
        reproduce: set[str] = set()
        for distribution in source["distributions"]:
            if distribution["kind"] != "community_bridge":
                continue
            local = [
                release for release in distribution["releases"]
                if release["package_source"]["repository"] == repository
            ]
            if not local:
                continue
            current_release = max(local, key=lambda item: item["sequence"])
            policies = policy_map(distribution)
            old_distribution = old_distributions.get(distribution["id"])
            old_releases = {
                item["sequence"]: item for item in old_distribution["releases"]
            } if old_distribution else {}
            old_policies = policy_map(old_distribution) if old_distribution else {}
            product = products[distribution["product_id"]]
            old_product = old_products.get(distribution["product_id"])
            for release in local:
                sequence = release["sequence"]
                policy = policies[sequence]
                old_release = old_releases.get(sequence)
                old_policy = old_policies.get(sequence)
                eligible = distribution["status"] == "active" and policy["status"] == "active"
                changed_eligible_binding = eligible and (
                    old_release is None
                    or old_release != release
                    or old_policy is None
                    or old_policy != policy
                    or eligibility_broadened(
                        distribution, policy, old_distribution, old_policy,
                    )
                    or release_eligibility_broadened(
                        product, distribution, release, policy,
                        old_product, old_distribution, old_policy,
                    )
                )
                if changed_eligible_binding:
                    require(
                        sequence == current_release["sequence"],
                        f"{distribution['id']}@{sequence}: active historical bridge requires reproduction, "
                        f"but the canonical recipe represents release {current_release['sequence']}; "
                        "versioned historical reproduction inputs are unavailable",
                    )
                    reproduce.add(distribution["product_id"])
                elif not eligible:
                    if old_release is None:
                        retired_historical = (
                            policy["status"] == "revoked"
                            and isinstance(release["package_source"]["revision"], str)
                            and SHA_RE.fullmatch(release["package_source"]["revision"]) is not None
                            and release.get("published_at") is None
                        )
                        if not retired_historical:
                            require(
                                sequence == current_release["sequence"],
                                f"{distribution['id']}@{sequence}: inactive historical bridge has no reproducible recipe",
                            )
                            reproduce.add(distribution["product_id"])
                    else:
                        require(
                            old_release == release,
                            f"{distribution['id']}@{sequence}: inactive bridge changed after its signed binding",
                        )
        if not reproduce:
            return
        from build_bridges import BridgeError, assemble, compare_trees

        with tempfile.TemporaryDirectory(prefix="bridge-publication-check-") as temporary:
            for bridge_id in sorted(reproduce):
                destination = Path(temporary) / bridge_id
                destination.mkdir()
                try:
                    report = assemble(repository_root, bridge_id, destination, None)
                    compare_trees(repository_root / str(report["package_path"]), destination)
                except (BridgeError, OSError, ValueError) as error:
                    raise PublicationError(f"bridge reproduction failed: {error}") from error
    except RegistryError as error:
        raise PublicationError(str(error)) from error


def validate_signing_boundary_packages(
    source: dict[str, Any], previous: dict[str, Any] | None, repository_root: Path,
    repository: str, overrides: dict[str, Path],
) -> None:
    """Invoke the centralized package policy at the last pre-signing boundary."""
    try:
        validate_changed_external_releases(
            source, previous, repository=repository,
            repository_overrides=overrides,
        )
        # Existing local bindings normally remain offline-capable.  Reacquire
        # only a previously signed binding whose eligibility is broadened, and
        # always from its immutable revision rather than the newest checkout.
        prior = previous_releases(previous)
        distributions = {item["id"]: item for item in source["distributions"]}
        for identity in sorted(releases_requiring_validation(source, previous)):
            if identity not in prior:
                continue  # New/current bindings were validated while binding above.
            distribution = distributions[identity[0]]
            release = next(item for item in distribution["releases"] if item["sequence"] == identity[1])
            package_source = release["package_source"]
            if package_source["repository"] != repository:
                continue  # Central external reacquisition handled above.
            revision = package_source["revision"]
            require(
                isinstance(revision, str) and SHA_RE.fullmatch(revision) is not None,
                f"{identity[0]}@{identity[1]}: historical source requires a full pinned revision",
            )
            temporary = acquire_external(
                repository, revision, package_source["path"], repository_root,
            )
            try:
                validate_release_package(
                    Path(temporary.name) / "checkout" / package_source["path"],
                    release, label=f"{identity[0]}@{identity[1]}",
                )
            finally:
                temporary.cleanup()
        validate_active_local_runtime_closures(
            source, repository_root=repository_root, repository=repository,
        )
    except RegistryError as error:
        raise PublicationError(str(error)) from error


def build_candidate(
    source: dict[str, Any], config: dict[str, Any], source_commit: str,
    publication_id: str, previous: dict[str, Any] | None,
    *, repository_root: Path = ROOT,
    external_overrides: dict[str, Path] | None = None,
    historical_evidence: dict[str, dict[str, Any]] | None = None,
) -> dict[str, Any]:
    validate_with_schema(source, SOURCE_SCHEMA)
    require(SHA_RE.fullmatch(source_commit) is not None, "source commit must be a full lowercase SHA")
    try:
        # Establish recipe/source binding before package validation so a
        # contributor cannot substitute provenance while retaining package bytes.
        validate_bridge_bindings(
            source, repository_root=repository_root,
            repository=config["repository"],
        )
    except RegistryError as error:
        raise PublicationError(str(error)) from error
    prior = previous_releases(previous)
    prior_distributions = previous_distributions(previous)
    all_distributions = {item["id"]: item for item in source["distributions"]}
    require(len(all_distributions) == len(source["distributions"]), "duplicate distribution identity")
    distributions_by_id = {identity: item for identity, item in all_distributions.items() if item["status"] != "candidate"}
    products = sorted((copy.deepcopy(item) for item in source["products"]), key=lambda item: item["id"])
    aliases: dict[str, str] = {}
    referenced_distributions: set[str] = set()
    for product in products:
        require(set(product["aliases"]).issubset(product["reserved_aliases"]), f"{product['id']}: active aliases must remain reserved")
        for alias in product["reserved_aliases"]:
            require(alias not in aliases, f"reserved alias {alias} is assigned to multiple products")
            aliases[alias] = product["id"]
        referenced_distributions.update(product["distributions"])
        require(all(item in all_distributions for item in product["distributions"]), f"{product['id']}: references an unknown distribution")
        product["distributions"] = [item for item in product["distributions"] if item in distributions_by_id]
        require(product["default_distribution"] in product["distributions"], f"{product['id']}: default distribution is not listed")
        for distribution_id in product["distributions"]:
            require(distribution_id in distributions_by_id, f"{product['id']}: missing distribution {distribution_id}")
            require(distributions_by_id[distribution_id]["product_id"] == product["id"], f"{product['id']}: mismatched distribution {distribution_id}")
    require(referenced_distributions == set(all_distributions), "every distribution must be owned by exactly one product")

    if previous is not None:
        product_map = {product["id"]: product for product in products}
        for old_product in previous["products"]:
            product_id = old_product["id"]
            require(product_id in product_map, f"published product {product_id} was removed")
            product = product_map[product_id]
            require(product["manifest_name"] == old_product["manifest_name"], f"published product {product_id} manifest name changed")
            historical_aliases = set(old_product["aliases"]) | set(old_product["reserved_aliases"])
            require(historical_aliases.issubset(product["reserved_aliases"]), f"published product {product_id} reserved alias was removed")
            require(set(old_product["distributions"]).issubset(product["distributions"]), f"published product {product_id} distribution was removed")

    output_distributions: list[dict[str, Any]] = []
    overrides = external_overrides or {}
    for original in sorted(distributions_by_id.values(), key=lambda item: item["id"]):
        distribution = copy.deepcopy(original)
        policies = policy_map(distribution)
        require(len(policies) == len(distribution["release_policies"]), f"{distribution['id']}: duplicate release policy")
        require(set(policies) == {release["sequence"] for release in distribution["releases"]}, f"{distribution['id']}: releases and policies must be one-to-one")
        old_distribution = prior_distributions.get(distribution["id"])
        if old_distribution is not None:
            for field in ("id", "product_id", "kind", "packager"):
                require(distribution[field] == old_distribution[field], f"published distribution {distribution['id']} {field} changed")
        old_policies = policy_map(old_distribution) if old_distribution else {}
        for release in distribution["releases"]:
            owning_product = next(product for product in products if product["id"] == distribution["product_id"])
            require(release["manifest_name"] == owning_product["manifest_name"], f"{distribution['id']}@{release['sequence']}: manifest identity differs from product")
            policy = policies[release["sequence"]]
            required = {
                component for component, state in owning_product["minimum_capabilities"].items()
                if state == "required"
            }
            require_closed_runtime = (
                distribution["status"] == "active"
                and policy["status"] == "active"
                and required.issubset(release["components"])
            )
            identity = (distribution["id"], release["sequence"])
            label = f"{identity[0]}@{identity[1]}"
            old = prior.get(identity)
            prior_for_distribution = [item for (distribution_id, _), item in prior.items() if distribution_id == distribution["id"]]
            package_source = release["package_source"]
            in_repository = package_source["repository"] == config["repository"]
            if old is not None:
                immutable = {key: value for key, value in release.items() if key not in ("published_at", "package_source")}
                old_immutable = {key: value for key, value in old.items() if key not in ("published_at", "package_source")}
                require(immutable == old_immutable, f"{label}: published immutable release fields changed")
                require(package_source["repository"] == old["package_source"]["repository"] and package_source["path"] == old["package_source"]["path"], f"{label}: published package source changed")
                if not in_repository:
                    require(package_source["revision"] == old["package_source"]["revision"], f"{label}: published external source revision changed")
                release["package_source"] = copy.deepcopy(old["package_source"])
                release["published_at"] = old["published_at"]
            else:
                if prior_for_distribution:
                    highest = max(item["sequence"] for item in prior_for_distribution)
                    require(release["sequence"] > highest, f"{label}: new release sequence must be above {highest}")
                    require(
                        all((release["tree_digest"], release["manifest_digest"]) != (item["tree_digest"], item["manifest_digest"]) for item in prior_for_distribution),
                        f"{label}: unchanged package bytes must reuse their existing release; publish policy/evidence only",
                    )
                reviewed_revision = package_source["revision"]
                reviewed_published_at = release.get("published_at")
                historical = isinstance(reviewed_revision, str) and reviewed_published_at is not None
                if historical:
                    require(SHA_RE.fullmatch(reviewed_revision) is not None, f"{label}: historical source requires reviewed full SHA")
                    if in_repository:
                        temporary = acquire_external(
                            package_source["repository"], reviewed_revision, package_source["path"],
                            repository_root,
                        )
                        try:
                            verify_package(
                                Path(temporary.name) / "checkout" / package_source["path"], release, label,
                                require_closed_runtime=require_closed_runtime,
                            )
                        finally:
                            temporary.cleanup()
                elif in_repository:
                    retired_historical = (
                        policy["status"] == "revoked"
                        and isinstance(reviewed_revision, str)
                        and SHA_RE.fullmatch(reviewed_revision) is not None
                        and reviewed_published_at is None
                    )
                    if retired_historical:
                        # A never-published release may retain reviewed historical
                        # bytes after the live package path advances. This exception
                        # is terminally revoked; eligible releases still bind only
                        # to the checked-out post-merge tree below.
                        temporary = acquire_external(
                            package_source["repository"], reviewed_revision,
                            package_source["path"], repository_root,
                        )
                        try:
                            verify_package(
                                Path(temporary.name) / "checkout" / package_source["path"],
                                release, label, require_closed_runtime=False,
                            )
                        finally:
                            temporary.cleanup()
                        release["published_at"] = None
                    else:
                        require(
                            reviewed_revision is None and reviewed_published_at is None,
                            f"{label}: new in-repository release must have an unresolved revision and no published_at",
                        )
                        release["published_at"] = None
                        # Review source cannot author an eligible binding. Only an
                        # unresolved revision is bound after the checked-out merge
                        # tree passes the reviewed digest checks below.
                        verify_package(
                            repository_root / package_source["path"], release, label,
                            require_closed_runtime=require_closed_runtime,
                        )
                        package_source["revision"] = source_commit
                else:
                    require(
                        isinstance(reviewed_revision, str) and SHA_RE.fullmatch(reviewed_revision) is not None and reviewed_published_at is None,
                        f"{label}: new external release requires a reviewed full SHA and no published_at",
                    )
                    release["published_at"] = None
        distribution["releases"].sort(key=lambda item: item["sequence"])
        distribution["release_policies"].sort(key=lambda item: item["release_sequence"])
        output_distributions.append(distribution)

    # Published releases are append-only even if review source accidentally drops one.
    for identity in prior:
        require(any(d["id"] == identity[0] and any(r["sequence"] == identity[1] for r in d["releases"]) for d in output_distributions), f"published release {identity} was removed from canonical source")

    candidate_source = {**source, "distributions": output_distributions}
    validate_reproduced_bridges(
        candidate_source, repository_root, config["repository"], previous,
    )
    validate_signing_boundary_packages(
        candidate_source, previous, repository_root, config["repository"], overrides,
    )
    evidence_overrides = dict(overrides)
    if repository_override(evidence_overrides, config["repository"]) is None:
        evidence_overrides[config["repository"]] = repository_root
    evidence = selected_evidence(
        candidate_source, set(distributions_by_id), config, evidence_overrides,
        historical_evidence,
    )
    source_selected_ids = {
        evidence_id
        for distribution in output_distributions
        for policy in distribution["release_policies"]
        for evidence_id in policy["current_evidence"]
    }
    validate_local_evidence_anchor(
        config, repository_root,
        [item for item in candidate_source["evidence"] if item["id"] in source_selected_ids],
    )
    for distribution in output_distributions:
        for policy in distribution["release_policies"]:
            policy["current_evidence"] = [
                public_evidence_id(evidence_id)
                for evidence_id in policy["current_evidence"]
            ]
    validate_publication_eligibility_trust(
        products, output_distributions, evidence, config,
    )
    revocations = [
        {"distribution_id": distribution["id"], "release_sequence": policy["release_sequence"]}
        for distribution in output_distributions for policy in distribution["release_policies"]
        if policy["status"] == "revoked"
    ]
    return {
        "candidate_schema_version": 1,
        "snapshot_schema_version": 1,
        "publication_id": publication_id,
        "source_commit": source_commit,
        "lifetime_days": config["snapshot_lifetime_days"],
        "products": products,
        "distributions": output_distributions,
        "evidence": evidence,
        "revocations": revocations,
    }


def parse_overrides(values: list[str]) -> dict[str, Path]:
    result: dict[str, Path] = {}
    for value in values:
        repository, separator, raw_path = value.partition("=")
        require(bool(separator and repository and raw_path), "--external-repository must be REPOSITORY=LOCAL_GIT_REPOSITORY")
        require(
            repository_override(result, repository) is None,
            f"duplicate external repository override {repository}",
        )
        result[repository] = Path(raw_path).resolve()
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--directory", type=Path, required=True)
    parser.add_argument("--config", type=Path, required=True)
    parser.add_argument("--source-commit", required=True)
    parser.add_argument("--source-tree-commit")
    parser.add_argument("--publication-id", required=True)
    parser.add_argument("--ledger", type=Path)
    parser.add_argument("--trusted-keys", type=Path)
    parser.add_argument("--initialize-ledger", action="store_true")
    parser.add_argument("--ledger-seed-commit")
    parser.add_argument("--ledger-sequence-floor", type=parse_public_sequence)
    parser.add_argument("--external-repository", action="append", default=[])
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--digest-output", type=Path, required=True)
    args = parser.parse_args()
    try:
        resolved_head = subprocess.check_output([GIT, "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
        checked_out_source = args.source_tree_commit or args.source_commit
        mismatch_label = "source tree commit" if args.source_tree_commit else "--source-commit"
        require(resolved_head == checked_out_source, f"checked-out HEAD {resolved_head} does not match {mismatch_label} {checked_out_source}")
        if args.source_tree_commit:
            validate_marker(ROOT, args.source_commit, args.source_tree_commit, args.publication_id)
        previous = None
        historical_evidence: dict[str, dict[str, Any]] = {}
        if args.ledger:
            require(args.trusted_keys is not None, "--trusted-keys is required with --ledger")
            loaded = load_ledger_latest(
                args.ledger, load_public_keys(args.trusted_keys),
                allow_initialization=args.initialize_ledger,
                seed_commit=args.ledger_seed_commit,
                minimum_sequence=args.ledger_sequence_floor,
                require_external_floor=True,
            )
            previous = loaded[0] if loaded else None
            historical_evidence = loaded[2] if loaded else {}
        candidate = build_candidate(
            read_json(args.directory), load_config(args.config), args.source_commit,
            args.publication_id, previous,
            external_overrides=parse_overrides(args.external_repository),
            historical_evidence=historical_evidence,
        )
        validate_with_schema(candidate, CANDIDATE_SCHEMA)
        body = canonical_json(candidate)
        digest = candidate_digest(body)
        atomic_write(args.output, body)
        atomic_write(args.digest_output, (digest + "\n").encode("ascii"))
    except (OSError, CasError, PublicationError, KeyError, TypeError, subprocess.SubprocessError) as error:
        print(f"prepare-directory-publication: {error}", file=sys.stderr)
        return 1
    print(digest)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
