#!/usr/bin/env python3
"""Validate, derive, and persist permanent stable-launch evidence."""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path, PurePosixPath
from typing import Any, Mapping, Sequence

sys.path.insert(0, str(Path(__file__).resolve().parent))
from directory_publication import (  # noqa: E402
    PublicationError,
    canonical_json,
    parse_json_bytes,
    read_bytes_bounded,
    require,
    validate_with_schema,
)
from build_registry import RegistryError, validate_directory  # noqa: E402
from launch_observer_signatures import (  # noqa: E402
    validate_evidence_redaction,
    verify_observer_bundle,
)


ROOT = Path(__file__).resolve().parents[1]
LAUNCH_SCHEMA = ROOT / "tests" / "e2e" / "schemas" / "launch-evidence.schema.json"
OBSERVER_SCHEMA_ROOT = ROOT / "tests" / "e2e" / "schemas"
EVIDENCE_SCHEMA = ROOT / "schemas" / "directory-evidence-artifact.schema.json"
DIRECTORY_EVIDENCE_SCHEMA = ROOT / "schemas" / "directory-evidence.schema.json"
GIT = "/usr/bin/git"
GH = "/usr/bin/gh"
SHA_RE = re.compile(r"^[0-9a-f]{40}$")
DIGEST_RE = re.compile(r"^sha256:[0-9a-f]{64}$")
REPOSITORY_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9][A-Za-z0-9._-]*$")
WORKFLOW_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9][A-Za-z0-9._-]*/\.github/workflows/[A-Za-z0-9._-]+\.ya?ml$")
SOURCE_REF_RE = re.compile(r"^refs/heads/[A-Za-z0-9._/-]+$")
HEROES = {"agent-code-navigator", "context7", "cloudflare-docs", "chrome-devtools", "notion"}
HERO_CLIENTS = {"codex", "cursor", "kiro"}
MAX_FILES = 64
MAX_FILE_BYTES = 4 << 20
MAX_BUNDLE_BYTES = 16 << 20
ATTESTATION_BUNDLE_NAME = "github-attestation.jsonl"
LEDGER_ROOT = PurePosixPath("registry/evidence/sha256")
BOT_NAME = "uap-directory-publisher[bot]"
BOT_EMAIL = "uap-directory-publisher[bot]@users.noreply.github.com"
AUTHORITATIVE_DETAIL_FIELDS = (
    "challenge", "started_at", "observed_at", "consent_artifact_digest",
    "consent_attested", "isolated_identity", "identity_id", "client_id",
    "application_id", "endpoint", "command_traces", "github_attestation",
    "runtime_invocation", "discovery_verified", "manager_observation",
    "native_observation", "receipt_reconciled", "native_discovery_reconciled",
    "projection_receipt_digest", "native_app_digest", "native_mcp_digest",
    "oauth_artifact_approved", "registered_app_binding", "ui_activation",
    "read_only", "scenario_id", "run_id", "run_attempt",
    "pseudonymous_identity_id", "pseudonymous_workspace_id",
    "dedicated_identity", "disposable_project_status", "operation_mode",
    "auth_origin", "cleanup_outcome", "no_real_project_proof",
    "native_discovery_evidence",
    "public_mcp_evidence", "release_manifest_digest", "release_checksums_digest",
    "directory_digest", "scenario_contract_digest",
)


class EvidenceError(PublicationError):
    """The launch evidence cannot be promoted into permanent Directory data."""


def fail(message: str) -> None:
    raise EvidenceError(message)


def sha256(body: bytes) -> str:
    return "sha256:" + hashlib.sha256(body).hexdigest()


def safe_relative(path: str) -> PurePosixPath:
    candidate = PurePosixPath(path)
    if (
        not path or path.startswith("/") or "\\" in path
        or candidate.as_posix() != path or any(part in {"", ".", ".."} for part in candidate.parts)
    ):
        fail(f"unsafe evidence path: {path!r}")
    return candidate


def _git(repo: Path, args: Sequence[str], *, input_bytes: bytes | None = None,
         env: Mapping[str, str] | None = None, check: bool = True) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        [GIT, "-C", str(repo), *args], input=input_bytes, stdout=subprocess.PIPE,
        stderr=subprocess.PIPE, env=env, check=check,
    )


def require_sha(value: str, label: str) -> None:
    if SHA_RE.fullmatch(value) is None:
        fail(f"{label} must be a full lowercase commit SHA")


def read_json(path: Path, label: str) -> dict[str, Any]:
    value = parse_json_bytes(read_bytes_bounded(path, MAX_FILE_BYTES), label, max_bytes=MAX_FILE_BYTES)
    if not isinstance(value, dict):
        fail(f"{label} must be a JSON object")
    return value


def applicability(row: dict[str, Any]) -> dict[str, Any]:
    tuple_value = row["tuple"]
    fields = (
        "product_id", "tree_digest", "manifest_digest", "distribution_id",
        "release_sequence", "source_repository", "source_revision", "source_path",
        "dependency_identity", "installer_version", "adapter_version", "client_version",
        "os", "architecture",
    )
    return {"level": row["level"], "client": row["client"], **{field: tuple_value[field] for field in fields}}


def selected_rows(launch: dict[str, Any]) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for row in launch["matrix"]:
        hero_runtime = (
            row.get("scenario") == "hero_5x3_runtime"
            and row.get("plugin") in HEROES and row.get("client") in HERO_CLIENTS
            and row.get("level") == "runtime"
        )
        chatgpt_runtime = (
            row.get("scenario") == "chatgpt_registered_binding"
            and row.get("plugin") == "cloudflare-docs" and row.get("client") == "chatgpt"
            and row.get("level") == "runtime"
        )
        if not (hero_runtime or chatgpt_runtime):
            continue
        if row.get("outcome") != "passed":
            fail(f"authoritative launch row did not pass: {row.get('id')}")
        details = row.get("details")
        if (
            not isinstance(details, dict)
            or details.get("evidence_basis") != "protected_external_observer"
            or (row.get("client") == "chatgpt" and details.get("public_mcp_proof") is not True)
            or (row.get("client") != "chatgpt" and details.get("native_discovery_proof") is not True)
        ):
            fail(f"launch row is not authoritative protected observer evidence: {row.get('id')}")
        rows.append(row)
    if len(rows) != 16:
        fail(f"expected 16 authoritative hero runtime/OAuth rows, found {len(rows)}")
    expected_runtime = {(plugin, client) for plugin in HEROES for client in HERO_CLIENTS}
    actual_runtime = {(row["plugin"], row["client"]) for row in rows if row["client"] in HERO_CLIENTS}
    if actual_runtime != expected_runtime:
        fail("hero runtime applicability matrix is incomplete or duplicated")
    if sum(row["client"] == "chatgpt" and row["level"] == "runtime" for row in rows) != 1:
        fail("exactly one Cloudflare ChatGPT public-MCP runtime record is required")
    keys = [canonical_json(applicability(row)) for row in rows]
    if len(keys) != len(set(keys)):
        fail("duplicate current-evidence applicability tuple")
    return sorted(rows, key=lambda row: canonical_json(applicability(row)))


def verify_authoritative_observer_rows(
    launch: dict[str, Any], observer: dict[str, Any], *, repository: str,
    public_key: str, key_id: str, enforce_freshness: bool = True,
) -> None:
    artifacts = verify_observer_bundle(
        observer, challenge=launch["run"]["challenge"],
        public_key_base64=public_key, expected_key_id=key_id,
        enforce_freshness=enforce_freshness,
    )
    validate_observer_artifact_schemas(artifacts)
    source: dict[tuple[str, str, str], dict[str, Any]] = {}
    expected_file_pairs = {
        "runtime-attestations.json": {
            (plugin, client) for plugin in HEROES - {"notion"} for client in HERO_CLIENTS
        },
        "notion-oauth-attestations.json": {("notion", client) for client in HERO_CLIENTS},
        "chatgpt-cloudflare-attestation.json": {("cloudflare-docs", "chatgpt")},
    }
    for name in (
        "runtime-attestations.json", "notion-oauth-attestations.json",
        "chatgpt-cloudflare-attestation.json",
    ):
        artifact = artifacts.get(name)
        if not isinstance(artifact, dict) or artifact.get("schema_version") != 1:
            fail(f"signed observer artifact is invalid: {name}")
        records = artifact.get("attestations")
        if not isinstance(records, list):
            fail(f"signed observer artifact omitted attestations: {name}")
        if {(record.get("plugin"), record.get("client")) for record in records if isinstance(record, dict)} != expected_file_pairs[name] or len(records) != len(expected_file_pairs[name]):
            fail(f"signed observer artifact does not contain the exact expected pair set: {name}")
        for record in records:
            if not isinstance(record, dict):
                fail(f"signed observer artifact contains a non-object record: {name}")
            key = (record.get("plugin"), record.get("client"), record.get("level"))
            if not all(isinstance(part, str) for part in key) or key in source:
                fail("signed observer artifact contains an invalid or duplicate row")
            github = record.get("github_attestation")
            if (
                record.get("challenge") != launch["run"]["challenge"]
                or str(record.get("run_id")) != launch["run"]["github_run_id"]
                or str(record.get("run_attempt")) != launch["run"]["github_run_attempt"]
                or not isinstance(github, dict)
                or github.get("repository") != repository
                or github.get("sha") != launch["run"]["github_sha"]
                or str(github.get("run_id")) != launch["run"]["github_run_id"]
                or str(github.get("run_attempt")) != launch["run"]["github_run_attempt"]
                or github.get("workflow") != "launch-evidence-e2e.yml"
                or github.get("job") != "protected-observer-inputs"
                or github.get("challenge") != launch["run"]["challenge"]
                or record.get("release_manifest_digest") != launch["release"]["manifest_digest"]
                or record.get("release_checksums_digest") != launch["release"]["checksums_digest"]
                or record.get("directory_digest") != launch["directory"]["snapshot_digest"]
                or record.get("scenario_contract_digest") != launch["scenario_contract"]["digest"]
            ):
                fail(f"signed observer row is not bound to the protected OIDC job: {key}")
            source[key] = record
    rows = selected_rows(launch)
    expected = {(row["plugin"], row["client"], row["level"]): row for row in rows}
    if set(source) != set(expected):
        fail("signed observer and canonical launch rows differ")
    for key, row in expected.items():
        record = source[key]
        default_reason = "explicit OAuth/runtime attestation" if key[1] == "chatgpt" else "explicit runtime attestation"
        projected_details = {
            **{field: record[field] for field in AUTHORITATIVE_DETAIL_FIELDS if field in record},
            "evidence_basis": "protected_external_observer",
            "runtime_proof": True,
            "native_discovery_proof": key[1] != "chatgpt",
            **({"public_mcp_proof": True} if key[1] == "chatgpt" else {}),
        }
        row_details = row.get("details")
        mismatches = []
        if record.get("outcome") != "passed": mismatches.append("outcome")
        if record.get("tuple") != row.get("tuple"): mismatches.append("tuple")
        if row.get("reason") != record.get("reason", default_reason): mismatches.append("reason")
        if not isinstance(row_details, dict):
            mismatches.append("details")
        else:
            if set(row_details) != set(projected_details) | {"resolution"}:
                mismatches.append(f"detail fields observed={sorted(row_details)} expected={sorted(set(projected_details) | {'resolution'})}")
            differing = sorted(field for field, value in projected_details.items() if row_details.get(field) != value)
            if differing: mismatches.append(f"detail values={differing}")
        if mismatches:
            fail(f"canonical launch row differs from signed observer row ({', '.join(mismatches)}): {key}")


def validate_observer_artifact_schemas(artifacts: dict[str, Any]) -> None:
    """Apply the reviewed observer schemas inside the minimal attester."""
    try:
        import jsonschema
    except ImportError as error:  # pragma: no cover - workflows install it
        fail("jsonschema is required for protected observer validation")
    base = "https://uap.invalid/observer-schemas/"
    schemas: dict[str, dict[str, Any]] = {}
    for path in OBSERVER_SCHEMA_ROOT.glob("*.schema.json"):
        value = read_json(path, f"observer schema {path.name}")
        schemas[path.name] = {**value, "$id": base + path.name}
    # The protected observer's reviewed consent schema is deployed with the
    # observer commit. Reconstruct its one hardened field here so this worker
    # does not take ownership of that separately-owned schema path.
    consent_schema = copy.deepcopy(schemas["consent.schema.json"])
    proof = consent_schema["properties"]["no_real_project_proof"]
    proof["properties"]["enforcement"] = {"const": "systemd-positive-mount-allowlist-v1"}
    consent_schema["allOf"][0]["then"]["properties"]["no_real_project_proof"] = {"required": ["enforcement"]}
    schemas["consent.schema.json"] = consent_schema
    store = {base + name: value for name, value in schemas.items()}
    for name, schema_name in (
        ("runtime-attestations.json", "runtime-attestations.schema.json"),
        ("notion-oauth-attestations.json", "runtime-attestations.schema.json"),
        ("chatgpt-cloudflare-attestation.json", "runtime-attestations.schema.json"),
        ("consent.json", "consent.schema.json"),
    ):
        schema = schemas[schema_name]
        resolver = jsonschema.RefResolver(base + schema_name, schema, store=store)
        validator = jsonschema.Draft202012Validator(
            schema, resolver=resolver, format_checker=jsonschema.FormatChecker(),
        )
        errors = sorted(validator.iter_errors(artifacts[name]), key=lambda item: list(item.absolute_path))
        if errors:
            fail(f"signed observer artifact does not match the reviewed schema: {name}")


def evidence_record(row: dict[str, Any]) -> dict[str, Any]:
    tuple_value = row["tuple"]
    evidence_identity = {
        "applicability": applicability(row),
        "observed_at": tuple_value["observed_at"],
    }
    applicability_digest = hashlib.sha256(canonical_json(evidence_identity)).hexdigest()
    record = {
        "schema_version": 1,
        "id": f"launch/{row['plugin']}/{row['client']}/{row['level']}/{applicability_digest[:24]}",
        "product_id": tuple_value["product_id"],
        "distribution_id": tuple_value["distribution_id"],
        "release_sequence": tuple_value["release_sequence"],
        "package_tree_digest": tuple_value["tree_digest"],
        "manifest_digest": tuple_value["manifest_digest"],
        "source_repository": tuple_value["source_repository"],
        "source_revision": tuple_value["source_revision"],
        "source_path": tuple_value["source_path"],
        "level": row["level"],
        "outcome": row["outcome"],
        "client": row["client"],
        "client_version": tuple_value["client_version"],
        "installer_version": tuple_value["installer_version"],
        "adapter_version": tuple_value["adapter_version"],
        "os": tuple_value["os"],
        "architecture": tuple_value["architecture"],
        "dependency_identity": tuple_value["dependency_identity"],
        "observed_at": tuple_value["observed_at"],
    }
    validate_with_schema(record, EVIDENCE_SCHEMA)
    return record


def checksum_bytes(files: Mapping[str, bytes]) -> bytes:
    lines = [f"{hashlib.sha256(files[path]).hexdigest()}  {path}\n" for path in sorted(files)]
    return "".join(lines).encode("ascii")


def build_bundle(artifact_dir: Path, *, repository: str, workflow: str,
                 source_ref: str, source_digest: str, expected_run_id: str,
                 expected_run_attempt: str,
                 expected_caller_event_name: str, expected_caller_ref: str,
                 expected_caller_workflow_ref: str,
                 expected_publication_id: str, expected_sequence: int,
                 expected_snapshot_digest: str, expected_source_commit: str,
                 verify_observer: bool = False, observer_public_key: str = "",
                 observer_key_id: str = "", enforce_observer_freshness: bool = True,
                 ) -> tuple[str, dict[str, bytes]]:
    if REPOSITORY_RE.fullmatch(repository) is None or WORKFLOW_RE.fullmatch(workflow) is None:
        fail("repository or workflow identity is invalid")
    if not workflow.startswith(repository + "/.github/workflows/"):
        fail("workflow and repository identities differ")
    if SOURCE_REF_RE.fullmatch(source_ref) is None:
        fail("protected workflow source ref is invalid")
    require_sha(source_digest, "workflow source digest")
    require_sha(expected_source_commit, "publication source commit")
    if not expected_run_id.isdigit() or not expected_run_attempt.isdigit() or not expected_publication_id.isdigit():
        fail("run ID, run attempt, and publication ID must be decimal strings")
    if type(expected_sequence) is not int or expected_sequence < 1:
        fail("publication sequence must be a positive integer")
    if DIGEST_RE.fullmatch(expected_snapshot_digest) is None:
        fail("publication snapshot digest is invalid")

    launch_path = artifact_dir / "launch-evidence.json"
    bundle_path = artifact_dir / "signed-observer-bundle.json"
    launch = read_json(launch_path, "launch evidence")
    observer_bytes = read_bytes_bounded(bundle_path, MAX_FILE_BYTES)
    observer = parse_json_bytes(observer_bytes, "signed observer bundle", max_bytes=MAX_FILE_BYTES)
    validate_with_schema(launch, LAUNCH_SCHEMA)
    validate_evidence_redaction(launch, context="canonical launch evidence")
    if launch["run"].get("mode") != "enforced" or launch["run"].get("runtime_claims") is not True:
        fail("only enforced runtime launch evidence is publishable")
    if launch["summary"].get("required_gates_complete") is not True or launch["summary"].get("hero_runtime_results") != 15:
        fail("launch evidence does not prove the complete stable gate")
    if launch["run"].get("github_sha") != source_digest:
        fail("launch evidence workflow source digest mismatch")
    if launch["run"].get("github_run_id") != expected_run_id:
        fail("launch evidence workflow run ID mismatch")
    if launch["run"].get("github_run_attempt") != expected_run_attempt:
        fail("launch evidence workflow run attempt mismatch")
    if (
        launch["run"].get("caller_event_name") != expected_caller_event_name
        or launch["run"].get("caller_ref") != expected_caller_ref
        or launch["run"].get("caller_workflow_ref") != expected_caller_workflow_ref
        or expected_caller_event_name not in {"push", "schedule", "workflow_dispatch"}
        or expected_caller_ref != "refs/heads/main"
        or expected_caller_workflow_ref != f"{repository}/.github/workflows/directory-publication.yml@refs/heads/main"
    ):
        fail("launch evidence protected caller identity mismatch")
    directory = launch["directory"]
    expected_directory = {
        "sequence": expected_sequence,
        "snapshot_digest": expected_snapshot_digest,
    }
    if any(directory.get(key) != value for key, value in expected_directory.items()):
        fail("launch evidence Directory identity mismatch")
    # The launch harness challenge binds the exact staged source commit.  Each
    # selected row independently carries the immutable release source revision.
    if not isinstance(observer, dict) or sha256(observer_bytes) != launch["run"]["observer_bundle_digest"]:
        fail("signed observer bundle digest mismatch")
    if verify_observer:
        if not observer_public_key or not observer_key_id:
            fail("observer signature verification requires an explicit public key and key ID")
        verify_authoritative_observer_rows(
            launch, observer, repository=repository,
            public_key=observer_public_key, key_id=observer_key_id,
            enforce_freshness=enforce_observer_freshness,
        )

    launch_bytes = canonical_json(launch)
    digest = sha256(launch_bytes)
    records = [evidence_record(row) for row in selected_rows(launch)]
    files: dict[str, bytes] = {
        "launch-evidence.json": launch_bytes,
        "signed-observer-bundle.json": observer_bytes,
    }
    index_records: list[dict[str, Any]] = []
    for record in records:
        suffix = record["id"].split("/")[-1]
        filename = f"directory-evidence/records/{record['product_id']}-{record['client']}-{record['level']}-{suffix}.json"
        body = canonical_json(record)
        files[filename] = body
        index_records.append({"id": record["id"], "path": filename, "digest": sha256(body)})
    index = {
        "schema_version": 1,
        "launch_evidence_digest": digest,
        "signed_observer_bundle_digest": sha256(observer_bytes),
        "repository": repository,
        "workflow": workflow,
        "source_ref": source_ref,
        "source_digest": source_digest,
        "workflow_run_id": expected_run_id,
        "workflow_run_attempt": expected_run_attempt,
        "caller_event_name": expected_caller_event_name,
        "caller_ref": expected_caller_ref,
        "caller_workflow_ref": expected_caller_workflow_ref,
        "publication_id": expected_publication_id,
        "publication_sequence": expected_sequence,
        "publication_snapshot_digest": expected_snapshot_digest,
        "publication_source_commit": expected_source_commit,
        "records": sorted(index_records, key=lambda item: item["id"]),
    }
    files["directory-evidence/index.json"] = canonical_json(index)
    bundle_identity = {
        "schema_version": 1,
        "launch_evidence_digest": digest,
        "signed_observer_bundle_digest": sha256(observer_bytes),
        "directory_evidence_index_digest": sha256(files["directory-evidence/index.json"]),
        "records": index["records"],
    }
    files["bundle-identity.json"] = canonical_json(bundle_identity)
    files["SHA256SUMS"] = checksum_bytes(files)
    validate_bounded_files(files)
    return digest, files


def validate_bounded_files(files: Mapping[str, bytes]) -> None:
    if not 1 <= len(files) <= MAX_FILES:
        fail(f"evidence bundle must contain between 1 and {MAX_FILES} files")
    total = 0
    folded: set[str] = set()
    for path, body in files.items():
        safe_relative(path)
        normalized = path.casefold()
        if normalized in folded:
            fail(f"case-colliding evidence path: {path}")
        folded.add(normalized)
        if len(body) > MAX_FILE_BYTES:
            fail(f"evidence file exceeds {MAX_FILE_BYTES} bytes: {path}")
        total += len(body)
    if total > MAX_BUNDLE_BYTES:
        fail(f"evidence bundle exceeds {MAX_BUNDLE_BYTES} bytes")


def write_bundle(directory: Path, files: Mapping[str, bytes]) -> None:
    directory.mkdir(parents=True, exist_ok=True)
    expected = set(files)
    for existing in directory.rglob("*"):
        if existing.is_symlink() or (existing.is_file() and existing.relative_to(directory).as_posix() not in expected):
            fail(f"unexpected evidence bundle entry: {existing.relative_to(directory)}")
    for relative, body in files.items():
        destination = directory / relative
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_bytes(body)


def verify_exact_bundle(directory: Path, expected: Mapping[str, bytes]) -> None:
    actual: dict[str, bytes] = {}
    for path in directory.rglob("*"):
        if path.is_symlink() or not path.is_file():
            if path.is_dir():
                continue
            fail(f"evidence bundle contains a non-regular entry: {path}")
        relative = path.relative_to(directory).as_posix()
        safe_relative(relative)
        actual[relative] = read_bytes_bounded(path, MAX_FILE_BYTES)
    validate_bounded_files(actual)
    if set(actual) != set(expected):
        fail(f"evidence bundle paths differ: expected {sorted(expected)}, observed {sorted(actual)}")
    for path, body in expected.items():
        if actual[path] != body:
            fail(f"evidence bundle file is not canonical or checksum-bound: {path}")


def read_attestation_bundle(path: Path) -> bytes:
    if path.is_symlink() or not path.is_file():
        fail("GitHub attestation bundle must be a regular file")
    body = read_bytes_bounded(path, MAX_FILE_BYTES)
    if not body.strip():
        fail("GitHub attestation bundle is empty")
    return body


def attach_attestation_bundle(files: Mapping[str, bytes], body: bytes) -> dict[str, bytes]:
    result = dict(files)
    result.pop("SHA256SUMS", None)
    result[ATTESTATION_BUNDLE_NAME] = body
    result["SHA256SUMS"] = checksum_bytes(result)
    validate_bounded_files(result)
    return result


def verify_attestation(directory: Path, *, bundle_path: Path, repository: str, workflow: str,
                       source_ref: str, source_digest: str) -> None:
    if not Path(GH).is_file():
        fail(f"required attestation verifier is missing: {GH}")
    read_attestation_bundle(bundle_path)
    command = [
        GH, "attestation", "verify", str(directory / "bundle-identity.json"),
        "--bundle", str(bundle_path),
        "--repo", repository, "--signer-workflow", workflow,
        "--source-ref", source_ref, "--source-digest", source_digest,
        "--deny-self-hosted-runners",
    ]
    completed = subprocess.run(command, check=False, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if completed.returncode != 0:
        fail("launch evidence workflow attestation verification failed")


def load_index(files: Mapping[str, bytes]) -> dict[str, Any]:
    value = parse_json_bytes(files["directory-evidence/index.json"], "evidence index", max_bytes=MAX_FILE_BYTES)
    if not isinstance(value, dict):
        fail("evidence index must be an object")
    return value


def pointer_for(record: dict[str, Any], *, repository: str, ledger_commit: str,
                root: str, index: dict[str, Any], index_digest: str,
                bundle_identity_digest: str) -> dict[str, Any]:
    index_record = next((item for item in index["records"] if item["id"] == record["id"]), None)
    if index_record is None:
        fail(f"evidence index omitted {record['id']}")
    artifact = {
        "repository": repository,
        "revision": ledger_commit,
        "path": f"{root}/{index_record['path']}",
        "digest": index_record["digest"],
    }
    pointer = {
        **copy.deepcopy(record),
        "artifact": artifact,
        "trust": {
            "kind": "github_actions",
            "workflow": index["workflow"],
            "source_ref": index["source_ref"],
            "source_digest": index["source_digest"],
            "bundle_manifest": {
                "repository": repository,
                "revision": ledger_commit,
                "path": f"{root}/bundle-identity.json",
                "digest": bundle_identity_digest,
            },
            "launch_artifact": {
                "repository": repository,
                "revision": ledger_commit,
                "path": f"{root}/launch-evidence.json",
                "digest": index["launch_evidence_digest"],
            },
            "observer_artifact": {
                "repository": repository,
                "revision": ledger_commit,
                "path": f"{root}/signed-observer-bundle.json",
                "digest": index["signed_observer_bundle_digest"],
            },
            "evidence_index": {
                "repository": repository,
                "revision": ledger_commit,
                "path": f"{root}/directory-evidence/index.json",
                "digest": index_digest,
            },
        },
    }
    validate_with_schema(pointer, DIRECTORY_EVIDENCE_SCHEMA)
    return pointer


def directory_applicability(record: Mapping[str, Any]) -> bytes:
    return canonical_json({
        "level": record["level"], "client": record.get("client"),
        **{name: record.get(name) for name in (
            "product_id", "package_tree_digest", "manifest_digest", "distribution_id",
            "release_sequence", "source_repository", "source_revision", "source_path",
            "dependency_identity", "installer_version", "adapter_version", "client_version",
            "os", "architecture",
        )},
    })


def update_directory(source: dict[str, Any], pointers: list[dict[str, Any]]) -> dict[str, Any]:
    result = copy.deepcopy(source)
    existing = {item["id"]: item for item in result["evidence"]}
    for pointer in pointers:
        if pointer["id"] in existing and existing[pointer["id"]] != pointer:
            fail(f"immutable evidence identity already exists with different bytes: {pointer['id']}")
        existing[pointer["id"]] = pointer
    result["evidence"] = [existing[key] for key in sorted(existing)]
    policies = {
        (distribution["id"], policy["release_sequence"]): policy
        for distribution in result["distributions"] for policy in distribution["release_policies"]
    }
    applicability_by_id = {
        evidence_id: directory_applicability(record)
        for evidence_id, record in existing.items()
        if record.get("level") != "schema"
    }
    by_applicability: dict[bytes, str] = {}
    for pointer in pointers:
        key = directory_applicability(pointer)
        if key in by_applicability:
            fail(f"duplicate applicability record: {pointer['id']}")
        by_applicability[key] = pointer["id"]
        identity = (pointer["distribution_id"], pointer["release_sequence"])
        policy = policies.get(identity)
        if policy is None:
            fail(f"evidence references missing release policy: {identity[0]}@{identity[1]}")
        current = {
            evidence_id for evidence_id in policy["current_evidence"]
            if applicability_by_id.get(evidence_id) != key
        }
        current.add(pointer["id"])
        policy["current_evidence"] = sorted(current)
    return result


def deterministic_timestamp(digest: str) -> int:
    if DIGEST_RE.fullmatch(digest) is None:
        fail("launch evidence digest is invalid")
    return 946684800 + int(digest.removeprefix("sha256:")[:16], 16) % (100 * 366 * 24 * 60 * 60)


def commit_with_files(repo: Path, parent: str, files: Mapping[str, bytes], message: str,
                      digest: str) -> str:
    require_sha(parent, "commit parent")
    with tempfile.NamedTemporaryFile(prefix="uap-evidence-index-", delete=True) as index:
        env = dict(os.environ)
        env["GIT_INDEX_FILE"] = index.name
        _git(repo, ["read-tree", parent], env=env)
        for path, body in sorted(files.items()):
            safe_relative(path)
            blob = _git(repo, ["hash-object", "-w", "--stdin"], input_bytes=body).stdout.decode().strip()
            _git(repo, ["update-index", "--add", "--cacheinfo", f"100644,{blob},{path}"], env=env)
        tree = _git(repo, ["write-tree"], env=env).stdout.decode().strip()
    timestamp = deterministic_timestamp(digest)
    identity = dict(os.environ)
    identity.update({
        "GIT_AUTHOR_NAME": BOT_NAME, "GIT_AUTHOR_EMAIL": BOT_EMAIL,
        "GIT_COMMITTER_NAME": BOT_NAME, "GIT_COMMITTER_EMAIL": BOT_EMAIL,
        "GIT_AUTHOR_DATE": f"@{timestamp} +0000", "GIT_COMMITTER_DATE": f"@{timestamp} +0000",
    })
    return _git(repo, ["commit-tree", tree, "-p", parent], input_bytes=message.encode("utf-8"), env=identity).stdout.decode().strip()


def materialize_commits(source_repo: Path, ledger_repo: Path, artifact_dir: Path, files: Mapping[str, bytes],
                        *, repository: str, main_parent: str, ledger_parent: str,
                        approval_target: str, digest: str) -> dict[str, str]:
    require_sha(main_parent, "main parent")
    require_sha(ledger_parent, "ledger parent")
    require_sha(approval_target, "approval target")
    if _git(source_repo, ["rev-parse", "HEAD"]).stdout.decode().strip() != main_parent:
        fail("source checkout is not the exact main parent")
    if _git(ledger_repo, ["rev-parse", "HEAD"]).stdout.decode().strip() != ledger_parent:
        fail("ledger checkout is not the exact ledger parent")
    if _git(
        ledger_repo, ["merge-base", "--is-ancestor", approval_target, ledger_parent], check=False,
    ).returncode != 0:
        fail("approval target is not an ancestor of the evidence parent")
    root = (LEDGER_ROOT / digest.removeprefix("sha256:")).as_posix()
    if _git(ledger_repo, ["cat-file", "-e", f"{ledger_parent}:{root}"], check=False).returncode == 0:
        fail("immutable evidence root already exists")
    # The launch and observer inputs remain necessary for independent future
    # re-derivation of every leaf from the attested bundle identity.
    ledger_files = {f"{root}/{path}": body for path, body in files.items()}
    ledger_message = (
        "chore(evidence): persist stable launch evidence\n\n"
        "Launch-Evidence-Persistence: 1\n"
        f"Launch-Evidence-Digest: {digest}\n"
        f"Workflow-Source-Commit: {load_index(files)['source_digest']}\n"
    )
    ledger_commit = commit_with_files(ledger_repo, ledger_parent, ledger_files, ledger_message, digest)
    changed = _git(ledger_repo, ["diff-tree", "--no-commit-id", "--name-status", "-r", ledger_parent, ledger_commit]).stdout.decode().splitlines()
    if not changed or any(not line.startswith(f"A\t{root}/") for line in changed):
        fail("evidence ledger commit is not a bounded append-only root")

    index = load_index(files)
    pointers: list[dict[str, Any]] = []
    for item in index["records"]:
        body = files[item["path"]]
        record = parse_json_bytes(body, item["path"], max_bytes=MAX_FILE_BYTES)
        if not isinstance(record, dict):
            fail(f"invalid Directory evidence leaf: {item['path']}")
        pointers.append(pointer_for(
            record, repository=repository, ledger_commit=ledger_commit, root=root,
            index=index, index_digest=sha256(files["directory-evidence/index.json"]),
            bundle_identity_digest=sha256(files["bundle-identity.json"]),
        ))
    directory_path = source_repo / "registry" / "directory.json"
    directory = read_json(directory_path, "Directory source")
    updated = update_directory(directory, pointers)
    try:
        validate_directory(
            updated, verify_packages=False,
            repository_root=source_repo, repository=repository,
        )
    except RegistryError as error:
        fail(f"mechanical evidence selection failed full Directory validation: {error}")
    main_message = (
        "chore(evidence): select stable launch evidence\n\n"
        "Launch-Evidence-Selection: 1\n"
        f"Launch-Evidence-Digest: {digest}\n"
        f"Evidence-Ledger-Commit: {ledger_commit}\n"
    )
    main_commit = commit_with_files(
        source_repo, main_parent, {"registry/directory.json": canonical_json(updated)},
        main_message, digest,
    )
    changed_main = _git(source_repo, ["diff-tree", "--no-commit-id", "--name-only", "-r", main_parent, main_commit]).stdout.decode().splitlines()
    if changed_main != ["registry/directory.json"]:
        fail("mechanical main commit changed paths outside registry/directory.json")
    return {
        "schema_version": "1", "launch_evidence_digest": digest,
        "main_parent": main_parent, "main_commit": main_commit,
        "ledger_parent": ledger_parent, "ledger_commit": ledger_commit,
        "approval_target": approval_target,
    }


def verify_completed_state(
    source_repo: Path, ledger_repo: Path, *, repository: str, main_commit: str,
    main_parent: str, expected_run_id: str, source_digest: str,
    expected_publication_id: str, expected_publication_source_commit: str,
    caller_event_name: str, caller_ref: str, caller_workflow_ref: str,
    approval_tag: str, observer_public_key: str, observer_key_id: str,
) -> None:
    """Authenticate the exact post-evidence CAS state for a whole-run retry."""
    require_sha(main_commit, "completed main commit")
    require_sha(main_parent, "completed main parent")
    require_sha(source_digest, "completed workflow source")
    require_sha(expected_publication_source_commit, "completed publication source")
    if not expected_run_id.isdigit():
        fail("completed workflow run ID must be decimal")
    if not expected_publication_id.isdigit():
        fail("completed publication ID must be decimal")
    ledger_commit = _git(ledger_repo, ["rev-parse", "HEAD"]).stdout.decode().strip()
    require_sha(ledger_commit, "completed ledger commit")
    main_parents = _git(source_repo, ["show", "-s", "--format=%P", main_commit]).stdout.decode().strip().split()
    if main_parents != [main_parent]:
        fail("completed main commit is not the exact evidence child")
    ledger_parents = _git(ledger_repo, ["show", "-s", "--format=%P", ledger_commit]).stdout.decode().strip().split()
    if len(ledger_parents) != 1:
        fail("completed evidence ledger commit must have one parent")
    approval = _git(ledger_repo, ["rev-parse", f"{approval_tag}^{{commit}}"], check=False)
    if approval.returncode != 0:
        fail("completed launch approval target is missing")
    approval_target = approval.stdout.decode().strip()
    evidence_parent = ledger_parents[0]
    if _git(
        ledger_repo, ["merge-base", "--is-ancestor", approval_target, evidence_parent], check=False,
    ).returncode != 0:
        fail("completed approval target is not an ancestor of the evidence parent")
    descendants = _git(
        ledger_repo,
        ["rev-list", "--reverse", "--ancestry-path", f"{approval_target}..{evidence_parent}"],
    ).stdout.decode().splitlines()
    previous = approval_target
    for descendant in descendants:
        parents = _git(
            ledger_repo, ["show", "-s", "--format=%P", descendant],
        ).stdout.decode().strip().split()
        if parents != [previous]:
            fail("completed pre-evidence lineage is not linear")
        changed = _git(
            ledger_repo,
            ["diff-tree", "--no-commit-id", "--name-only", "-r", previous, descendant],
        ).stdout.decode().splitlines()
        if not changed or any(not path.startswith("discovery/") for path in changed):
            fail("completed pre-evidence lineage contains a non-Discovery append")
        previous = descendant
    ledger_message = _git(ledger_repo, ["show", "-s", "--format=%B", ledger_commit]).stdout.decode()
    match = re.fullmatch(
        r"chore\(evidence\): persist stable launch evidence\n\n"
        r"Launch-Evidence-Persistence: 1\n"
        r"Launch-Evidence-Digest: (sha256:[0-9a-f]{64})\n"
        r"Workflow-Source-Commit: ([0-9a-f]{40})\n\n?",
        ledger_message,
    )
    if match is None or match.group(2) != source_digest:
        fail("completed evidence ledger message is invalid")
    digest = match.group(1)
    expected_main_message = (
        "chore(evidence): select stable launch evidence\n\n"
        "Launch-Evidence-Selection: 1\n"
        f"Launch-Evidence-Digest: {digest}\n"
        f"Evidence-Ledger-Commit: {ledger_commit}\n\n"
    )
    if _git(source_repo, ["show", "-s", "--format=%B", main_commit]).stdout.decode() != expected_main_message:
        fail("completed main evidence selection message is invalid")
    changed_main = _git(source_repo, ["diff-tree", "--no-commit-id", "--name-only", "-r", main_parent, main_commit]).stdout.decode().splitlines()
    if changed_main != ["registry/directory.json"]:
        fail("completed main evidence selection changed unexpected paths")
    root = (LEDGER_ROOT / digest.removeprefix("sha256:")).as_posix()
    paths = _git(ledger_repo, ["ls-tree", "-r", "--name-only", ledger_commit, "--", root]).stdout.decode().splitlines()
    if not paths or any(not path.startswith(root + "/") for path in paths):
        fail("completed ledger evidence root is missing or invalid")
    files = {
        path.removeprefix(root + "/"): _git(ledger_repo, ["show", f"{ledger_commit}:{path}"]).stdout
        for path in paths
    }
    index = load_index(files)
    if (
        index.get("repository") != repository
        or index.get("workflow") != f"{repository}/.github/workflows/launch-evidence-e2e.yml"
        or index.get("source_ref") != "refs/heads/main"
        or index.get("source_digest") != source_digest
        or index.get("workflow_run_id") != expected_run_id
        or index.get("caller_event_name") != caller_event_name
        or index.get("caller_ref") != caller_ref
        or index.get("caller_workflow_ref") != caller_workflow_ref
        or index.get("publication_id") != expected_publication_id
        or index.get("publication_source_commit") != expected_publication_source_commit
    ):
        fail("completed evidence index is not bound to this exact workflow run")
    with tempfile.TemporaryDirectory(prefix="uap-completed-evidence-") as temporary:
        bundle = Path(temporary)
        for name in (
            "launch-evidence.json", "signed-observer-bundle.json",
            "bundle-identity.json", ATTESTATION_BUNDLE_NAME,
        ):
            (bundle / name).write_bytes(files[name])
        # Authenticate the immutable permanent subject before allowing an old
        # observer timestamp to enter deterministic completed-state replay.
        verify_attestation(
            bundle, bundle_path=bundle / ATTESTATION_BUNDLE_NAME,
            repository=repository, workflow=index["workflow"],
            source_ref=index["source_ref"], source_digest=source_digest,
        )
        _, derived = build_bundle(
            bundle, repository=repository, workflow=index["workflow"],
            source_ref=index["source_ref"], source_digest=source_digest,
            expected_run_id=expected_run_id, expected_run_attempt=index["workflow_run_attempt"],
            expected_caller_event_name=caller_event_name, expected_caller_ref=caller_ref,
            expected_caller_workflow_ref=caller_workflow_ref,
            expected_publication_id=expected_publication_id,
            expected_sequence=index["publication_sequence"],
            expected_snapshot_digest=index["publication_snapshot_digest"],
            expected_source_commit=expected_publication_source_commit,
            verify_observer=True, observer_public_key=observer_public_key,
            observer_key_id=observer_key_id, enforce_observer_freshness=False,
        )
        derived = attach_attestation_bundle(derived, files[ATTESTATION_BUNDLE_NAME])
        write_bundle(bundle, derived)
    if files != derived:
        fail("completed permanent evidence root is not the exact canonical bundle")
    ledger_files = {f"{root}/{path}": body for path, body in files.items()}
    expected_ledger_message = (
        "chore(evidence): persist stable launch evidence\n\n"
        "Launch-Evidence-Persistence: 1\n"
        f"Launch-Evidence-Digest: {digest}\n"
        f"Workflow-Source-Commit: {source_digest}\n"
    )
    expected_ledger = commit_with_files(
        ledger_repo, ledger_parents[0], ledger_files, expected_ledger_message, digest,
    )
    if expected_ledger != ledger_commit:
        fail("completed evidence ledger commit is not the exact deterministic append")
    base_directory = parse_json_bytes(
        _git(source_repo, ["show", f"{main_parent}:registry/directory.json"]).stdout,
        "completed base Directory", max_bytes=MAX_FILE_BYTES,
    )
    if not isinstance(base_directory, dict):
        fail("completed base Directory must be an object")
    records = [
        parse_json_bytes(files[item["path"]], f"completed record {item['id']}", max_bytes=MAX_FILE_BYTES)
        for item in index["records"]
    ]
    if not all(isinstance(record, dict) for record in records):
        fail("completed evidence records must be objects")
    pointers = [
        pointer_for(
            record, repository=repository, ledger_commit=ledger_commit, root=root,
            index=index, index_digest=sha256(files["directory-evidence/index.json"]),
            bundle_identity_digest=sha256(files["bundle-identity.json"]),
        )
        for record in records
    ]
    expected_directory = update_directory(base_directory, pointers)
    validate_directory(
        expected_directory, verify_packages=False,
        repository_root=source_repo, repository=repository,
    )
    expected_main = commit_with_files(
        source_repo, main_parent,
        {"registry/directory.json": canonical_json(expected_directory)},
        expected_main_message.removesuffix("\n"), digest,
    )
    if expected_main != main_commit:
        fail("completed main evidence selection is not the exact deterministic commit")


def write_outputs(path: Path | None, values: Mapping[str, str]) -> None:
    body = canonical_json(dict(values))
    if path is None:
        sys.stdout.buffer.write(body)
    else:
        path.write_bytes(body)


def common_arguments(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--artifact-dir", type=Path, required=True)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--workflow", required=True)
    parser.add_argument("--source-ref", required=True)
    parser.add_argument("--source-digest", required=True)
    parser.add_argument("--expected-run-id", required=True)
    parser.add_argument("--expected-run-attempt", required=True)
    parser.add_argument("--expected-caller-event-name", required=True)
    parser.add_argument("--expected-caller-ref", required=True)
    parser.add_argument("--expected-caller-workflow-ref", required=True)
    parser.add_argument("--expected-publication-id", required=True)
    parser.add_argument("--expected-publication-sequence", type=int, required=True)
    parser.add_argument("--expected-publication-snapshot-digest", required=True)
    parser.add_argument("--expected-publication-source-commit", required=True)
    parser.add_argument("--verify-observer", action="store_true")
    parser.add_argument("--observer-public-key", default="")
    parser.add_argument("--observer-key-id", default="")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    prepare = commands.add_parser("prepare-bundle")
    common_arguments(prepare)
    prepare.add_argument("--output", type=Path)
    verify = commands.add_parser("verify-bundle")
    common_arguments(verify)
    verify.add_argument("--verify-attestation", action="store_true")
    verify.add_argument("--attestation-bundle", type=Path)
    verify.add_argument("--output", type=Path)
    commit = commands.add_parser("commit")
    common_arguments(commit)
    commit.add_argument("--source-repo", type=Path, required=True)
    commit.add_argument("--ledger-repo", type=Path, required=True)
    commit.add_argument("--main-parent", required=True)
    commit.add_argument("--ledger-parent", required=True)
    commit.add_argument("--approval-target", required=True)
    commit.add_argument("--attestation-bundle", type=Path, required=True)
    commit.add_argument("--output", type=Path, required=True)
    completed = commands.add_parser("verify-completed")
    completed.add_argument("--source-repo", type=Path, required=True)
    completed.add_argument("--ledger-repo", type=Path, required=True)
    completed.add_argument("--repository", required=True)
    completed.add_argument("--main-commit", required=True)
    completed.add_argument("--main-parent", required=True)
    completed.add_argument("--expected-run-id", required=True)
    completed.add_argument("--expected-publication-id", required=True)
    completed.add_argument("--expected-publication-source-commit", required=True)
    completed.add_argument("--source-digest", required=True)
    completed.add_argument("--caller-event-name", required=True)
    completed.add_argument("--caller-ref", required=True)
    completed.add_argument("--caller-workflow-ref", required=True)
    completed.add_argument("--approval-tag", required=True)
    completed.add_argument("--observer-public-key", required=True)
    completed.add_argument("--observer-key-id", required=True)
    args = parser.parse_args()
    try:
        if args.command == "verify-completed":
            verify_completed_state(
                args.source_repo, args.ledger_repo, repository=args.repository,
                main_commit=args.main_commit, main_parent=args.main_parent,
                expected_run_id=args.expected_run_id, source_digest=args.source_digest,
                expected_publication_id=args.expected_publication_id,
                expected_publication_source_commit=args.expected_publication_source_commit,
                caller_event_name=args.caller_event_name, caller_ref=args.caller_ref,
                caller_workflow_ref=args.caller_workflow_ref,
                approval_tag=args.approval_tag,
                observer_public_key=args.observer_public_key,
                observer_key_id=args.observer_key_id,
            )
            return 0
        digest, files = build_bundle(
            args.artifact_dir, repository=args.repository, workflow=args.workflow,
            source_ref=args.source_ref, source_digest=args.source_digest,
            expected_run_id=args.expected_run_id,
            expected_run_attempt=args.expected_run_attempt,
            expected_caller_event_name=args.expected_caller_event_name,
            expected_caller_ref=args.expected_caller_ref,
            expected_caller_workflow_ref=args.expected_caller_workflow_ref,
            expected_publication_id=args.expected_publication_id,
            expected_sequence=args.expected_publication_sequence,
            expected_snapshot_digest=args.expected_publication_snapshot_digest,
            expected_source_commit=args.expected_publication_source_commit,
            verify_observer=args.verify_observer,
            observer_public_key=args.observer_public_key,
            observer_key_id=args.observer_key_id,
        )
        if args.command == "prepare-bundle":
            write_bundle(args.artifact_dir, files)
            values = {"launch_evidence_digest": digest}
        else:
            verify_exact_bundle(args.artifact_dir, files)
            if getattr(args, "verify_attestation", False):
                if args.attestation_bundle is None:
                    fail("--verify-attestation requires --attestation-bundle")
                verify_attestation(
                    args.artifact_dir, bundle_path=args.attestation_bundle,
                    repository=args.repository, workflow=args.workflow,
                    source_ref=args.source_ref, source_digest=args.source_digest,
                )
            values = {"launch_evidence_digest": digest}
            if args.command == "commit":
                verify_attestation(
                    args.artifact_dir, bundle_path=args.attestation_bundle,
                    repository=args.repository, workflow=args.workflow,
                    source_ref=args.source_ref, source_digest=args.source_digest,
                )
                files = attach_attestation_bundle(
                    files, read_attestation_bundle(args.attestation_bundle),
                )
                values = materialize_commits(
                    args.source_repo, args.ledger_repo, args.artifact_dir, files,
                    repository=args.repository, main_parent=args.main_parent,
                    ledger_parent=args.ledger_parent, approval_target=args.approval_target,
                    digest=digest,
                )
        write_outputs(args.output, values)
    except (EvidenceError, PublicationError, RegistryError, OSError, subprocess.SubprocessError, KeyError, TypeError, ValueError) as error:
        print(f"launch-evidence-materializer: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
