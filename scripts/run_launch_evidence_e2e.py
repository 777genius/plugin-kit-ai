#!/usr/bin/env python3
"""Run the Phase 6 launch matrix without turning unavailable systems into passes.

The runner creates fresh client homes, invokes the supplied Agent Plugins binary,
and exports only tuple-scoped, redacted evidence. Runtime/OAuth observations are
accepted only from an explicit attestation file; package projection is never
promoted to runtime evidence.
"""

from __future__ import annotations

import argparse
import base64
import copy
import hashlib
import hmac
import json
import os
import platform
import re
import secrets
import shutil
import subprocess
import sys
import tempfile
from collections import Counter
from datetime import datetime, timedelta, timezone
from pathlib import Path, PurePosixPath
from typing import Any, Callable
from urllib.error import HTTPError, URLError
from urllib.parse import urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener, urlopen

sys.path.insert(0, str(Path(__file__).resolve().parent))
from directory_publication import (  # noqa: E402
    MAX_ENVELOPE_BYTES,
    MAX_LATEST_BYTES,
    MAX_SNAPSHOT_BYTES,
    PublicationError,
    canonical_json,
    load_public_keys,
    parse_json_bytes,
    parse_timestamp,
    validate_latest,
    validate_snapshot_semantics,
    verify_envelope,
)
from launch_observer_signatures import (  # noqa: E402
    sanitize_evidence,
    validate_evidence_redaction,
    verify_observer_bundle,
)
from build_registry import RegistryError, directory_tree_digest, resolve_directory  # noqa: E402
from observe_launch_scenario import (  # noqa: E402
    _managed_native_product_id,
    _stable_tree_snapshot,
    grouped_acquisition_closure_digest,
    installation_receipts,
    selected_manager_installation,
    strict_json_loads,
    validate_cli_envelope,
)


ROOT = Path(__file__).resolve().parents[1]
SCENARIOS = ROOT / "tests" / "e2e" / "launch-scenarios.json"
EXTERNAL_PACKAGE = ROOT / "tests" / "e2e" / "fixtures" / "external-package"
FORK_PACKAGE = ROOT / "tests" / "fixtures" / "plugins" / "fixture-bridge"
STATE_FIXTURE = ROOT / "tests" / "e2e" / "fixtures" / "state-schema-2.json"
CAPTURE_PROVENANCE = ROOT / "tests" / "fixtures" / "agentplugins-0.1.14" / "provenance.json"
RECOVERY_FIXTURE = ROOT / "tests" / "e2e" / "fixtures" / "recovery-cases.json"
SCENARIO_OBSERVER = ROOT / "scripts" / "observe_launch_scenario.py"
PRODUCTION_CONFIG = ROOT / "tests" / "e2e" / "production-launch.json"
STABLE_LAUNCH_VERSION_FILE = ROOT / "tests" / "e2e" / "stable-launch-version.txt"
PRODUCTION_DIRECTORY_TRUST = ROOT / "registry" / "publication" / "trusted-keys.json"
RELEASE_MANIFEST_NAME = "release-manifest.json"
RELEASE_CHECKSUMS_NAME = "checksums.txt"
RELEASE_MANIFEST_SCHEMA = ROOT / "schemas" / "e2e" / "release-manifest.schema.json"
TRUSTED_CATALOG_REPOSITORY = "777genius/universal-agent-plugins"
TRUSTED_CATALOG_REPOSITORY_OWNER = "777genius"
TRUSTED_CATALOG_REPOSITORY_ID = "1326737541"
TRUSTED_CATALOG_REPOSITORY_OWNER_ID = "13103045"
TRUSTED_OBSERVER_REF = "refs/heads/main"
TRUSTED_OBSERVER_ENVIRONMENT = "stable-launch-e2e"
TRUSTED_OBSERVER_SUBJECT = f"repo:{TRUSTED_CATALOG_REPOSITORY}:environment:{TRUSTED_OBSERVER_ENVIRONMENT}"
TRUSTED_OBSERVER_WORKFLOW_REF = f"{TRUSTED_CATALOG_REPOSITORY}/.github/workflows/directory-publication.yml@{TRUSTED_OBSERVER_REF}"
TRUSTED_OBSERVER_JOB_WORKFLOW_REF = f"{TRUSTED_CATALOG_REPOSITORY}/.github/workflows/launch-evidence-e2e.yml@{TRUSTED_OBSERVER_REF}"
TRUSTED_CLI_RELEASE_REPOSITORY = "777genius/plugin-kit-ai"
TRUSTED_CLI_RELEASE_TAG = "agentplugins-v" + STABLE_LAUNCH_VERSION_FILE.read_text(encoding="utf-8").strip()
TRUSTED_CLI_RELEASE_WORKFLOW = "777genius/plugin-kit-ai/.github/workflows/agentplugins-release.yml"
TRUSTED_CLI_RELEASE_COMMIT = "74a3790ee15d92afda8e8e3dd8f903c04811cfc7"
TRUSTED_CLI_RELEASE_SOURCE_REF = "refs/heads/main"
TRUSTED_SANITIZED_CAPTURE_MANIFEST = "sha256:1e7e5ca4d72be2e188bbfa002cf19975b4e1b100913a329bbaf963b5633abb85"


def validate_capture_provenance(path: Path = CAPTURE_PROVENANCE) -> dict[str, Any]:
    """Verify retained capture bytes and reject path/secret-bearing provenance."""
    value = strict_json_loads(path.read_bytes())
    if set(value) != {"schema_version", "release", "sanitization_rules", "captures"} or value["schema_version"] != 1:
        raise ValueError("invalid capture provenance envelope")
    release = value["release"]
    if not (
        set(release) == {
            "repository", "tag", "tag_commit", "asset", "version_stdout", "binary_sha256",
            "binary_digest_status", "capture_evidence_level", "release_binary_authenticated",
            "release_provenance_linkage",
        }
        and
        release.get("repository") == "777genius/plugin-kit-ai"
        and release.get("tag") == "agentplugins-v0.1.14"
        and re.fullmatch(r"[0-9a-f]{40}", str(release.get("tag_commit", "")))
        and release.get("asset") == "agentplugins_0.1.14_linux_amd64"
        and release.get("version_stdout") == "agentplugins 0.1.14"
        and release.get("binary_sha256") is None
        and release.get("binary_digest_status") == "not_retained_in_capture"
        and release.get("capture_evidence_level") == "sanitized_manifest_only_no_raw_or_binary_linkage"
        and release.get("release_binary_authenticated") is False
        and release.get("release_provenance_linkage") == "unlinked_sanitized_fixture_provenance"
    ):
        raise ValueError("invalid capture release identity")
    if not isinstance(value["sanitization_rules"], list) or len(value["sanitization_rules"]) < 5:
        raise ValueError("incomplete capture sanitization rules")
    roots = {"tests/e2e/fixtures": ROOT / "tests/e2e/fixtures"}
    default_root = path.parent
    seen: set[tuple[str, str]] = set()
    for capture in value["captures"]:
        if not isinstance(capture, dict) or not {
            "fixture", "argv", "sanitized_sha256",
        } <= set(capture) <= {
            "fixture", "fixture_root", "argv", "sanitized_sha256",
            "stderr_fixture", "stderr_sha256",
        }:
            raise ValueError("invalid sanitized capture manifest record")
        fixture_root = capture.get("fixture_root", ".")
        root = default_root if fixture_root == "." else roots.get(fixture_root)
        fixture = capture.get("fixture")
        if root is None or not isinstance(fixture, str) or Path(fixture).name != fixture:
            raise ValueError("non-canonical capture fixture locator")
        key = (fixture_root, fixture)
        if key in seen or not isinstance(capture.get("argv"), list) or not capture["argv"]:
            raise ValueError("duplicate or unbound capture provenance")
        seen.add(key)
        try:
            body = (root / fixture).read_bytes()
        except OSError as error:
            raise ValueError("capture fixture is missing") from error
        if capture.get("sanitized_sha256") != "sha256:" + hashlib.sha256(body).hexdigest():
            raise ValueError("capture fixture digest mismatch")
        stderr_fixture = capture.get("stderr_fixture")
        if stderr_fixture is not None:
            if Path(stderr_fixture).name != stderr_fixture:
                raise ValueError("non-canonical stderr fixture locator")
            try:
                stderr_body = (root / stderr_fixture).read_bytes()
            except OSError as error:
                raise ValueError("stderr capture fixture is missing") from error
            if capture.get("stderr_sha256") != "sha256:" + hashlib.sha256(stderr_body).hexdigest():
                raise ValueError("stderr capture digest mismatch")
    serialized = json.dumps(value, sort_keys=True)
    if re.search(r"/(?:home|tmp|srv|root)/", serialized, re.IGNORECASE):
        raise ValueError("capture provenance contains a host path")
    authenticated_manifest = {
        "release": release,
        "sanitization_rules": value["sanitization_rules"],
        "captures": [
            {
                key: capture[key]
                for key in ("fixture", "fixture_root", "argv", "sanitized_sha256", "stderr_fixture", "stderr_sha256")
                if key in capture
            }
            for capture in value["captures"]
        ],
    }
    if "sha256:" + hashlib.sha256(canonical_json(authenticated_manifest)).hexdigest() != TRUSTED_SANITIZED_CAPTURE_MANIFEST:
        raise ValueError("sanitized capture manifest differs from the trusted repository root")
    return value


def validate_capture_release_binding(
    *, release_manifest: dict[str, Any], release_identity: dict[str, Any],
    release_manifest_digest: str, release_checksums_digest: str,
    asset_name: str, asset_digest: str, asset_attestation: dict[str, Any],
    provenance_path: Path = CAPTURE_PROVENANCE,
) -> dict[str, Any]:
    """Validate fixture truth and independently authenticate the enforced binary.

    The immutable fixtures have no retained raw transcript or binary linkage.
    Successful release verification says only that the binary selected for the
    enforced run is the independently authenticated immutable release asset.
    """
    provenance = validate_capture_provenance(provenance_path)
    captured = provenance["release"]
    declared = next(
        (item for item in release_manifest.get("assets", {}).values() if item.get("file") == asset_name),
        None,
    )
    if not (
        set(release_identity) == {"repository", "tag", "tag_commit", "release_id", "immutable"}
        and release_identity == {
            "repository": TRUSTED_CLI_RELEASE_REPOSITORY, "tag": TRUSTED_CLI_RELEASE_TAG,
            "tag_commit": TRUSTED_CLI_RELEASE_COMMIT, "release_id": release_identity.get("release_id"),
            "immutable": True,
        }
        and type(release_identity.get("release_id")) is int and release_identity["release_id"] > 0
        and release_manifest.get("tag") == TRUSTED_CLI_RELEASE_TAG
        and release_manifest.get("commit") == TRUSTED_CLI_RELEASE_COMMIT
        # The retained 0.1.14 captures are historical schema fixtures only. They
        # are deliberately unlinked from the current release binary and cannot
        # become current runtime evidence. Authenticate the enforced binary
        # against the independently frozen stable release instead.
        and release_manifest.get("version") == STABLE_LAUNCH_VERSION_FILE.read_text(encoding="utf-8").strip()
        and isinstance(declared, dict) and declared.get("file") == asset_name
        and isinstance(declared.get("sha256"), str)
        and asset_digest == "sha256:" + declared["sha256"] and DIGEST.fullmatch(asset_digest)
        and DIGEST.fullmatch(release_manifest_digest) and DIGEST.fullmatch(release_checksums_digest)
        and strict_asset_attestation_matches(
            asset_attestation, repository=TRUSTED_CLI_RELEASE_REPOSITORY,
            workflow=TRUSTED_CLI_RELEASE_WORKFLOW, tag=TRUSTED_CLI_RELEASE_TAG,
            commit=TRUSTED_CLI_RELEASE_COMMIT, asset_name=asset_name, asset_digest=asset_digest,
        )
    ):
        raise ValueError("enforced binary is not bound to the authenticated immutable release")
    validate_release_manifest(
        release_manifest, repository=TRUSTED_CLI_RELEASE_REPOSITORY,
        tag=TRUSTED_CLI_RELEASE_TAG, tag_commit=TRUSTED_CLI_RELEASE_COMMIT,
    )
    return {
        "fixture_release_binary_authenticated": False,
        "fixture_release_provenance_linkage": "unlinked",
        "enforced_binary_authenticated": True,
        "capture_evidence_level": captured["capture_evidence_level"],
        "binary_digest": asset_digest,
        "fixture_recorded_tag_commit": captured["tag_commit"],
        "enforced_release_tag_commit": TRUSTED_CLI_RELEASE_COMMIT,
        "release_manifest_digest": release_manifest_digest,
        "release_checksums_digest": release_checksums_digest,
        "attestation": copy.deepcopy(asset_attestation),
    }


def strict_asset_attestation_matches(
    value: Any, *, repository: str, workflow: str, tag: str, commit: str,
    asset_name: str, asset_digest: str,
) -> bool:
    """Bind strict SLSA subject identity to the authenticated legacy asset fields."""
    return value == {
        "repository": repository, "workflow": workflow, "tag": tag, "tag_commit": commit,
        "issuer": "https://token.actions.githubusercontent.com",
        "source_ref": TRUSTED_CLI_RELEASE_SOURCE_REF, "source_digest": commit,
        "predicate_type": "https://slsa.dev/provenance/v1",
        "subject_name": asset_name, "subject_digest": asset_digest,
        "runner_environment": "github-hosted",
        "asset_name": asset_name, "asset_digest": asset_digest, "verified": True,
    }


DIRECTORY_INPUT_ENVIRONMENT_KEYS = frozenset({
    "AGENTPLUGINS_DIRECTORY_ORIGIN",
    "AGENTPLUGINS_DIRECTORY_SNAPSHOT",
    "AGENTPLUGINS_DIRECTORY_ENVELOPE",
    "AGENTPLUGINS_DIRECTORY_TRUST",
})
DIRECTORY_LAUNCH_ENVIRONMENT_KEYS = DIRECTORY_INPUT_ENVIRONMENT_KEYS | {
    "AGENTPLUGINS_DIRECTORY_CACHE",
    "AGENTPLUGINS_DIRECTORY_CONFORMANCE_ONLY",
}
CHALLENGE_DOMAIN = b"UAP-STABLE-LAUNCH-CHALLENGE-V1\0"
ATTESTATION_DOMAIN = b"UAP-STABLE-LAUNCH-OBSERVER-V1\0"
MAX_ATTESTATION_AGE = timedelta(minutes=30)
EXPECTED_ACCEPTANCE_SCENARIOS = (
    "retained_data_readd_before_changed_default", "schema_1_0_0_accepted",
    "schema_draft_rejected", "schema_unknown_rejected", "project_scope_zero_mutation",
    "direct_full_sha_immutable", "public_help_no_hidden_yes", "revoked_operations_boundary",
    "readd_sticky_distribution", "repair_sticky_distribution", "missing_runtime_exact_guidance",
    "plugin_data_lifecycle_boundary", "signed_sequence_not_semver",
)
EXPECTED_COUNTS = {
    "directory_products": 26, "directory_lifecycle_rows": 78,
    "hero_lifecycle_rows": 15, "hero_runtime_rows": 15,
    "context7_grouped_rows": 4, "chatgpt_rows": 1,
    "shared_backend_rows": 1, "acceptance_postcondition_rows": 13,
    "native_platform_rows": 7, "fault_rows": 23, "journey_rows": 3,
}
OUTCOMES = {"passed", "failed", "inconclusive", "not_applicable"}
DIGEST = re.compile(r"^sha256:[a-f0-9]{64}$")
FULL_SHA = re.compile(r"^[a-f0-9]{40}$")
GITHUB_REPOSITORY = re.compile(
    r"^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9](?:[A-Za-z0-9._-]{0,99})$"
)
GITHUB_SOURCE_PATH = re.compile(r"^[A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)*$")
VERSION = re.compile(r"^(\d+)\.(\d+)\.(\d+)$")
MINIMUM_STABLE_VERSION = (0, 1, 17)
IDENTITY_ID = re.compile(r"^[a-z0-9][a-z0-9._-]{2,63}$")
CONTRIBUTOR_PATH = re.compile(
    r"^(?:plugins/[a-z0-9]+(?:-[a-z0-9]+)*/[^/].*|"
    r"bridges/[a-z0-9]+(?:-[a-z0-9]+)*/[^/].*|"
    r"registry/(?:directory|review-preview|review-search)\.json)$"
)
PROTECTED_RUNTIME_DISCOVERY_ARGV = {
    "codex": ["codex", "mcp", "list", "--json"],
    "cursor": ["cursor", "agent", "mcp", "list", "--json"],
    "kiro": ["kiro", "mcp", "list", "--json"],
}
PROTECTED_RUNTIME_TOOLS = {
    "agent-code-navigator": "search_code",
    "context7": "resolve-library-id",
    "cloudflare-docs": "search_cloudflare_documentation",
    "chrome-devtools": "list_pages",
    "notion": "search",
}
CLIENT_ROOTS = {
    "codex": ".codex",
    "cursor": ".cursor",
    "kiro": ".kiro",
    "copilot": ".copilot",
    "vscode": ".config/Code/User",
}
SECRET_NAME = re.compile(r"(?i)(token|secret|password|cookie|authorization|oauth[_-]?code)")
GITHUB_API_ORIGIN = "https://api.github.com"


class RejectRedirects(HTTPRedirectHandler):
    """Never forward a credential to a redirect target."""

    def redirect_request(self, request, file_pointer, code, message, headers, new_url):
        return None


def complete_acquisition_proof(
    value: Any, targets: tuple[str, ...], *, tree_digest: str, manifest_digest: str,
    source_repository: str, source_revision: str, source_path: str, source_kind: str = "github",
) -> dict[str, Any] | None:
    """Validate the complete grouped acquisition at the evidence boundary."""
    if not isinstance(value, dict) or set(value) != {
        "acquisition_id", "acquisition_count", "tree_digest", "manifest_digest",
        "closure_digest", "source_kind", "fetched", "validated", "source_repository",
        "source_revision", "source_path", "targets", "target_outcomes",
    }:
        return None
    acquisition_id = value.get("acquisition_id")
    if not (
        isinstance(acquisition_id, str) and acquisition_id.strip()
        and type(value.get("acquisition_count")) is int and value["acquisition_count"] == 1
        and value.get("tree_digest") == tree_digest and DIGEST.fullmatch(tree_digest)
        and value.get("manifest_digest") == manifest_digest and DIGEST.fullmatch(manifest_digest)
        and isinstance(value.get("closure_digest"), str) and DIGEST.fullmatch(value["closure_digest"])
        and value.get("source_kind") == source_kind and source_kind in {"github", "local", "directory"}
        and value.get("source_repository") == source_repository and GITHUB_REPOSITORY.fullmatch(source_repository)
        and value.get("source_revision") == source_revision and FULL_SHA.fullmatch(source_revision)
        and value.get("source_path") == source_path and GITHUB_SOURCE_PATH.fullmatch(source_path)
        and value.get("fetched") is (source_kind != "local") and value.get("validated") is True
        and len(targets) == len(set(targets))
    ):
        return None
    outcomes = value.get("target_outcomes")
    target_bindings = value.get("targets")
    if (
        not isinstance(outcomes, dict) or set(outcomes) != set(targets)
        or not isinstance(target_bindings, list) or len(target_bindings) != len(targets)
        or [item.get("target") if isinstance(item, dict) else None for item in target_bindings] != list(targets)
    ):
        return None
    binding = {
        "outcome": "passed", "acquisition_id": acquisition_id,
        "tree_digest": tree_digest, "manifest_digest": manifest_digest,
        "closure_digest": value["closure_digest"],
    }
    if any(outcome != binding for outcome in outcomes.values()):
        return None
    if value["closure_digest"] != grouped_acquisition_closure_digest(
        source_kind, source_repository, source_path, source_revision, tree_digest, manifest_digest,
    ):
        return None
    return copy.deepcopy(value)


def parse_canonical_github_source(value: Any) -> dict[str, str] | None:
    """Normalize documented and production GitHub canonical source identities."""
    if not isinstance(value, str) or any(character in value for character in "?#\\%"):
        return None
    repository_locator = value.removeprefix("https://github.com/")
    try:
        repository, locator = repository_locator.split("@", 1)
        revision, package_path = locator.split("//", 1)
    except ValueError:
        return None
    if (
        not GITHUB_REPOSITORY.fullmatch(repository)
        or not FULL_SHA.fullmatch(revision)
        or not GITHUB_SOURCE_PATH.fullmatch(package_path)
        or not package_path
        or package_path.startswith("/")
        or package_path.endswith("/")
        or "//" in package_path
        or any(part in {"", ".", ".."} for part in package_path.split("/"))
        or PurePosixPath(package_path).as_posix() != package_path
    ):
        return None
    return {
        "source_repository": repository,
        "source_revision": revision,
        "source_path": package_path,
    }


def authoritative_native_client_evidence(
    evidence: Any, *, client_version: Any, product_id: str | None = None,
    client: str | None = None,
) -> bool:
    """Validate signed protected evidence from real native client commands."""
    if not isinstance(evidence, dict) or evidence.get("basis") != "protected_external_observer":
        return False
    if evidence.get("observer") != "native-client-command-v1":
        return False
    version = evidence.get("version_operation")
    discovery = evidence.get("discovery_operation")
    expected_client = client or evidence.get("client")
    return bool(
        expected_client in {"copilot", "vscode"}
        and evidence.get("client") == expected_client
        and isinstance(client_version, str) and client_version
        and isinstance(version, dict)
        and version.get("operation") == "version"
        and version.get("argv") == ["copilot", "--version"]
        and version.get("observed_client_version") == client_version
        and isinstance(discovery, dict)
        and discovery.get("operation") == "list"
        and discovery.get("argv") == ["copilot", "plugin", "list"]
        and discovery.get("discovered") is True
        and (product_id is None or discovery.get("product_id") == product_id)
    )


def authoritative_protected_runtime_evidence(
    evidence: Any, *, client_version: Any, product_id: str, client: str,
    challenge: str,
) -> bool:
    """Validate the exact Codex/Cursor/Kiro operations emitted by the observer."""
    if not isinstance(evidence, dict) or set(evidence) != {
        "basis", "observer", "client", "version_operation",
        "discovery_operation", "invocation_operation",
    }:
        return False
    version = evidence.get("version_operation")
    discovery = evidence.get("discovery_operation")
    invocation = evidence.get("invocation_operation")
    tool = PROTECTED_RUNTIME_TOOLS.get(product_id)
    return bool(
        client in PROTECTED_RUNTIME_DISCOVERY_ARGV
        and evidence.get("basis") == "protected_external_observer"
        and evidence.get("observer") == "native-client-command-v1"
        and evidence.get("client") == client
        and isinstance(client_version, str) and client_version
        and isinstance(version, dict)
        and set(version) == {"operation", "argv", "observed_client_version"}
        and version.get("operation") == "version"
        and version.get("argv") == [client, "--version"]
        and version.get("observed_client_version") == client_version
        and isinstance(discovery, dict)
        and set(discovery) == {"operation", "argv", "discovered", "product_id"}
        and discovery.get("operation") == "discovery"
        and discovery.get("argv") == PROTECTED_RUNTIME_DISCOVERY_ARGV[client]
        and discovery.get("discovered") is True
        and discovery.get("product_id") == product_id
        and isinstance(invocation, dict)
        and set(invocation) == {"operation", "tool", "product_id", "marker_digest", "succeeded"}
        and invocation.get("operation") == "tool_call"
        and invocation.get("tool") == tool
        and invocation.get("product_id") == product_id
        and invocation.get("marker_digest") == "sha256:" + hashlib.sha256(
            f"UAP_OBSERVER_OK {client} {product_id} {challenge}".encode()
        ).hexdigest()
        and invocation.get("succeeded") is True
    )


def authoritative_public_mcp_evidence(evidence: Any) -> bool:
    """Validate the distinct tokenless public-MCP proof used by ChatGPT."""
    if not isinstance(evidence, dict) or set(evidence) != {
        "basis", "observer", "endpoint", "protocol_version", "initialize", "list", "read",
    }:
        return False
    initialize, listed, read = evidence.get("initialize"), evidence.get("list"), evidence.get("read")
    return bool(
        evidence.get("basis") == "protected_external_observer"
        and evidence.get("observer") == "public-mcp-command-v1"
        and evidence.get("endpoint") == "https://docs.mcp.cloudflare.com/mcp"
        and evidence.get("protocol_version") == "2025-06-18"
        and initialize == {"method": "initialize", "passed": True}
        and listed == {"method": "tools/list", "required_name": "search_cloudflare_documentation", "passed": True}
        and isinstance(read, dict) and set(read) == {"method", "name", "read_only", "marker_digest", "passed"}
        and read.get("method") == "tools/call" and read.get("name") == "search_cloudflare_documentation"
        and read.get("read_only") is True and read.get("passed") is True
        and DIGEST.fullmatch(str(read.get("marker_digest", ""))) is not None
    )


def authoritative_repository_copilot_evidence(
    evidence: Any, *, client_version: Any, product_id: str, physical_artifact_id: str,
    expected_version: str,
) -> bool:
    """Validate native Copilot commands executed directly by Agent Plugins."""
    if (
        not isinstance(evidence, dict)
        or set(evidence) != {"basis", "version_operation", "discovery_operation"}
        or evidence.get("basis") != "native_client_command"
    ):
        return False
    version = evidence.get("version_operation")
    discovery = evidence.get("discovery_operation")
    return bool(
        client_version == expected_version
        and isinstance(version, dict)
        and set(version) == {"argv", "observed_client_version"}
        and version.get("argv") == ["copilot", "--version"]
        and version.get("observed_client_version") == expected_version
        and isinstance(discovery, dict)
        and set(discovery) == {"argv", "discovered", "product_id"}
        and discovery.get("argv") == ["copilot", "plugin", "list"]
        and discovery.get("discovered") is True
        and discovery.get("product_id") == _managed_native_product_id(product_id, physical_artifact_id)
    )

def read_production_config() -> dict[str, Any]:
    value = json.loads(PRODUCTION_CONFIG.read_text())
    if (
        value.get("schema_version") != 1
        or value.get("catalog_repository") != TRUSTED_CATALOG_REPOSITORY
        or value.get("cli_release_repository") != TRUSTED_CLI_RELEASE_REPOSITORY
        or value.get("cli_release_tag") != TRUSTED_CLI_RELEASE_TAG
        or value.get("cli_release_commit") != TRUSTED_CLI_RELEASE_COMMIT
        or value.get("cli_release_workflow") != TRUSTED_CLI_RELEASE_WORKFLOW
        or value.get("cli_release_manifest_digest") != "sha256:0e8f7316ddef542067bdd7276273fffa3bc00532afed8fd42be12f612aedea57"
        or value.get("cli_release_checksums_digest") != "sha256:d581ac34d9880afe998f8f871df285b5474623778d2eae98ebc8780a932a9fa8"
        or value.get("npm_facade_package") != "universal-agent-plugins"
        or value.get("npm_facade_version") != "0.1.18"
        or value.get("npm_facade_integrity") != "sha512-48UfVVaGrvmniWQpoiXQYZvTS3QqrCN0HFLSQBVCsqQJwPdecRtuK9XEsOs4nTMQuWImjX6ZAflUeY/79biRZg=="
        or value.get("directory_source_digest") != "sha256:8e8fdfc231a95a6b59fde7f552701b32ccc29eab51386b6d663f9ace7067de91"
        or value.get("scenario_contract_digest") != "sha256:d62f160de32f9af53d7bb2d7c03fbfa8c3203e2fe5c99e616985b1e583996e66"
        or value.get("copilot_cli_package") != "@github/copilot"
        or value.get("copilot_cli_version") != "1.0.80"
        or value.get("copilot_cli_integrity") != "sha512-6tf93ZF56KOiTTAjK/UhLZkl1W543IzaTQly288kockJZFswpRTnQEI00Yvacpb39DTvTYu3/ha9SeKpo/pgZQ=="
        or value.get("copilot_node_major") != 22
    ):
        raise ValueError("checked-in production repository configuration is invalid")
    origin = urlsplit(str(value.get("production_origin", "")))
    if origin.scheme != "https" or not origin.hostname or origin.query or origin.fragment or origin.username or origin.password:
        raise ValueError("checked-in production Directory origin is invalid")
    return value

def valid_copilot_installation(
    executable: Path | None, metadata_path: Path | None, metadata: Any,
    run_root: Path | None, config: dict[str, Any],
) -> bool:
    expected = {
        "schema_version": 1,
        "package": config["copilot_cli_package"],
        "version": config["copilot_cli_version"],
        "integrity": config["copilot_cli_integrity"],
        "node_major": config["copilot_node_major"],
        "signature_audit": True,
        "version_argv": ["copilot", "--version"],
        "observed_version": config["copilot_cli_version"],
    }
    return bool(
        executable
        and metadata_path
        and run_root
        and executable.is_file()
        and os.access(executable, os.X_OK)
        and metadata_path.is_file()
        and executable.is_relative_to(run_root)
        and metadata_path.is_relative_to(run_root)
        and executable.resolve().is_relative_to(run_root)
        and metadata_path.resolve().is_relative_to(run_root)
        and isinstance(metadata, dict)
        and set(metadata) == {*expected, "executable_digest", "version_stdout_digest"}
        and all(metadata.get(key) == value for key, value in expected.items())
        and metadata.get("executable_digest") == sha256_file(executable)
        and DIGEST.fullmatch(str(metadata.get("version_stdout_digest", "")))
    )


def bounded_https_get(url: str, *, maximum: int, accept: str = "application/octet-stream") -> bytes:
    parsed = urlsplit(url)
    if parsed.scheme != "https" or not parsed.hostname or parsed.username or parsed.password or parsed.fragment:
        raise ValueError("launch evidence downloads require credential-free HTTPS URLs")
    headers = {"Accept": accept, "User-Agent": "uap-stable-launch-evidence/1"}
    try:
        with urlopen(Request(url, headers=headers), timeout=60) as response:
            body = response.read(maximum + 1)
    except (HTTPError, URLError, TimeoutError) as error:
        raise ValueError(f"failed to fetch required immutable input from {parsed.hostname}") from error
    if len(body) > maximum:
        raise ValueError("download exceeds stable launch size bound")
    return body


def github_api_get(url: str, *, maximum: int, token: str | None = None) -> bytes:
    """Fetch exact GitHub API JSON without allowing credentialed redirects."""
    parsed = urlsplit(url)
    if (
        parsed.scheme != "https"
        or parsed.hostname != "api.github.com"
        or parsed.port is not None
        or parsed.username
        or parsed.password
        or parsed.query
        or parsed.fragment
        or not parsed.path.startswith("/repos/")
    ):
        raise ValueError("GitHub credentials are restricted to exact api.github.com repository JSON URLs")
    headers = {
        "Accept": "application/vnd.github+json",
        "User-Agent": "uap-stable-launch-evidence/1",
    }
    if token:
        headers["Authorization"] = "Bearer " + token
    try:
        opener = build_opener(RejectRedirects())
        with opener.open(Request(url, headers=headers), timeout=60) as response:
            if response.geturl() != url:
                raise ValueError("credentialed GitHub API requests must not redirect")
            body = response.read(maximum + 1)
    except (HTTPError, URLError, TimeoutError) as error:
        raise ValueError("failed to fetch exact GitHub API JSON without redirecting credentials") from error
    if len(body) > maximum:
        raise ValueError("GitHub API response exceeds stable launch size bound")
    return body


def github_json(repository: str, path: str, *, token: str | None = None) -> dict[str, Any]:
    if repository != TRUSTED_CLI_RELEASE_REPOSITORY or not GITHUB_REPOSITORY.fullmatch(repository):
        raise ValueError("GitHub API release resolution is restricted to the trusted CLI repository")
    normalized_path = path.lstrip("/")
    if not re.fullmatch(
        r"(?:releases/tags/agentplugins-v[0-9]+\.[0-9]+\.[0-9]+|git/ref/tags/agentplugins-v[0-9]+\.[0-9]+\.[0-9]+|git/tags/[a-f0-9]{40})",
        normalized_path,
    ):
        raise ValueError("GitHub API release resolution requires an exact release or tag JSON path")
    body = github_api_get(
        f"{GITHUB_API_ORIGIN}/repos/{repository}/{normalized_path}", maximum=2 << 20, token=token,
    )
    value = json.loads(body)
    if not isinstance(value, dict):
        raise ValueError("GitHub API returned an invalid object")
    return value


def resolve_tag_commit(repository: str, tag: str, *, token: str | None = None) -> str:
    value = github_json(repository, f"git/ref/tags/{tag}", token=token)
    target = value.get("object", {})
    for _ in range(4):
        sha = target.get("sha")
        kind = target.get("type")
        if not isinstance(sha, str) or not FULL_SHA.fullmatch(sha):
            raise ValueError("GitHub tag target has an invalid SHA")
        if kind == "commit":
            return sha
        if kind != "tag":
            raise ValueError("release tag does not resolve to a commit")
        target = github_json(repository, f"git/tags/{sha}", token=token).get("object", {})
    raise ValueError("release tag annotation chain is too deep")


def validate_release_manifest(value: dict[str, Any], *, repository: str, tag: str, tag_commit: str) -> None:
    try:
        import jsonschema
        jsonschema.Draft202012Validator(json.loads(RELEASE_MANIFEST_SCHEMA.read_text())).validate(value)
    except ImportError as error:
        raise ValueError("jsonschema is required to validate the release manifest") from error
    except Exception as error:
        if error.__class__.__module__.startswith("jsonschema"):
            raise ValueError("release manifest omits a required native asset or has an invalid field") from error
        raise
    if repository != TRUSTED_CLI_RELEASE_REPOSITORY:
        raise ValueError("release manifest was not resolved from the trusted CLI repository")
    if value.get("tag") != tag or value.get("commit") != tag_commit:
        raise ValueError("release manifest tag/commit identity does not match GitHub")
    if tag != "agentplugins-v" + str(value.get("version")):
        raise ValueError("release manifest tag and version disagree")
    names = [asset["file"] for asset in value["assets"].values()]
    if len(names) != len(set(names)):
        raise ValueError("release manifest contains duplicate asset names")
    expected = {
        "darwin-amd64": f"agentplugins_{value['version']}_darwin_amd64",
        "darwin-arm64": f"agentplugins_{value['version']}_darwin_arm64",
        "linux-amd64": f"agentplugins_{value['version']}_linux_amd64",
        "linux-arm64": f"agentplugins_{value['version']}_linux_arm64",
        "windows-amd64": f"agentplugins_{value['version']}_windows_amd64.exe",
        "windows-arm64": f"agentplugins_{value['version']}_windows_arm64.exe",
    }
    if set(value["assets"]) != set(expected) or any(value["assets"][key]["file"] != name for key, name in expected.items()):
        raise ValueError("release manifest omits or renames a required native asset")


def resolve_github_release(
    repository: str, tag: str, destination: Path, *, asset_name: str,
    token: str | None = None, fixture_fetch: Callable[[str, int, str], bytes] | None = None,
    attestation_verifier: Callable[[Path, str, str, str, str, str, str], dict[str, Any]] | None = None,
) -> tuple[Path, dict[str, Any], str]:
    """Resolve one published exact-tag release and checksum its selected asset.

    ``fixture_fetch`` exists solely for direct unit tests. Production callers cannot
    select URLs, checksums, or versions independently.
    """
    if not re.fullmatch(r"agentplugins-v[0-9]+\.[0-9]+\.[0-9]+", tag):
        raise ValueError("release tag must be an exact stable agentplugins-vX.Y.Z tag")
    release = github_json(repository, f"releases/tags/{tag}", token=token)
    if release.get("draft") or release.get("prerelease") or release.get("tag_name") != tag:
        raise ValueError("GitHub release is not an exact published stable tag")
    if release.get("immutable") is not True:
        raise ValueError("GitHub release is mutable; stable evidence requires immutable: true")
    tag_commit = resolve_tag_commit(repository, tag, token=token)
    api_assets = {item.get("name"): item for item in release.get("assets", []) if isinstance(item, dict)}
    expected_release_assets = {
        RELEASE_MANIFEST_NAME, RELEASE_CHECKSUMS_NAME,
        "agentplugins_" + tag.removeprefix("agentplugins-v") + "_darwin_amd64",
        "agentplugins_" + tag.removeprefix("agentplugins-v") + "_darwin_arm64",
        "agentplugins_" + tag.removeprefix("agentplugins-v") + "_linux_amd64",
        "agentplugins_" + tag.removeprefix("agentplugins-v") + "_linux_arm64",
        "agentplugins_" + tag.removeprefix("agentplugins-v") + "_windows_amd64.exe",
        "agentplugins_" + tag.removeprefix("agentplugins-v") + "_windows_arm64.exe",
    }
    if len(release.get("assets", [])) != len(expected_release_assets) or set(api_assets) != expected_release_assets:
        raise ValueError("GitHub release does not contain the exact stable native asset set")
    manifest_api = api_assets.get(RELEASE_MANIFEST_NAME)
    checksums_api = api_assets.get(RELEASE_CHECKSUMS_NAME)
    selected_api = api_assets.get(asset_name)
    if not manifest_api or not checksums_api or not selected_api:
        raise ValueError("GitHub release is missing manifest/checksums or selected asset")

    def fetch(asset: dict[str, Any], limit: int) -> bytes:
        name = str(asset.get("name", ""))
        url = str(asset.get("browser_download_url", ""))
        expected_url = f"https://github.com/{repository}/releases/download/{tag}/{name}"
        if url != expected_url:
            raise ValueError("GitHub release asset has an untrusted browser_download_url")
        expected_size = asset.get("size")
        if not isinstance(expected_size, int) or expected_size < 0 or expected_size > limit:
            raise ValueError("GitHub release asset has an invalid or excessive API size")
        if fixture_fetch:
            body = fixture_fetch(url, limit, "application/octet-stream")
        else:
            body = bounded_https_get(url, maximum=limit, accept="application/octet-stream")
        if len(body) != expected_size:
            raise ValueError("GitHub release asset bytes disagree with the API size")
        return body

    manifest_body = fetch(manifest_api, 1 << 20)
    manifest = json.loads(manifest_body)
    validate_release_manifest(manifest, repository=repository, tag=tag, tag_commit=tag_commit)
    manifest_digest = "sha256:" + hashlib.sha256(manifest_body).hexdigest()
    checksums_body = fetch(checksums_api, 1 << 20)
    checksum_entries: dict[str, str] = {}
    for line in checksums_body.decode("ascii").splitlines():
        match = re.fullmatch(r"([a-f0-9]{64})  ([A-Za-z0-9_.-]+)", line)
        if not match or match.group(2) in checksum_entries:
            raise ValueError("GitHub release checksums.txt is non-canonical")
        checksum_entries[match.group(2)] = match.group(1)
    checksum_names = {item["file"] for item in manifest["assets"].values()} | {RELEASE_MANIFEST_NAME}
    if set(checksum_entries) != checksum_names:
        raise ValueError("GitHub release checksums.txt does not cover the exact manifest asset set")
    if checksum_entries[RELEASE_MANIFEST_NAME] != hashlib.sha256(manifest_body).hexdigest():
        raise ValueError("GitHub release manifest disagrees with checksums.txt")
    declared = next((item for item in manifest["assets"].values() if item["file"] == asset_name), None)
    if declared is None:
        raise ValueError("selected GitHub asset is absent from the immutable release manifest")
    if selected_api.get("size") != declared["size"]:
        raise ValueError("GitHub asset size disagrees with the release manifest")
    body = fetch(selected_api, declared["size"])
    if (
        len(body) != declared["size"]
        or hashlib.sha256(body).hexdigest() != declared["sha256"]
        or checksum_entries[asset_name] != declared["sha256"]
    ):
        raise ValueError("GitHub release asset digest disagrees with the immutable manifest")
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_bytes(body)
    destination.chmod(0o500)
    (destination.parent / RELEASE_MANIFEST_NAME).write_bytes(manifest_body)
    (destination.parent / RELEASE_CHECKSUMS_NAME).write_bytes(checksums_body)
    (destination.parent / "github-release-identity.json").write_text(json.dumps({
        "repository": repository, "tag": tag, "tag_commit": tag_commit,
        "release_id": release.get("id"), "immutable": True,
    }, sort_keys=True) + "\n")
    verifier = attestation_verifier or verify_github_asset_attestation
    attestation = verifier(
        destination, repository, TRUSTED_CLI_RELEASE_WORKFLOW, tag, tag_commit,
        "sha256:" + declared["sha256"], asset_name,
    )
    (destination.parent / f"{asset_name}.attestation.json").write_text(json.dumps(attestation, sort_keys=True) + "\n")
    return destination, manifest, manifest_digest


def validate_prepared_github_release(
    prepared_root: Path, prepared: dict[str, Any], asset_name: str,
) -> tuple[Path, dict[str, Any], dict[str, Any], str, str]:
    """Validate immutable, previously authenticated release inputs without networking."""
    config = read_production_config()
    release_dir = prepared_root / "release"
    paths = {
        "binary": release_dir / asset_name,
        "manifest": release_dir / RELEASE_MANIFEST_NAME,
        "checksums": release_dir / RELEASE_CHECKSUMS_NAME,
        "identity": release_dir / "github-release-identity.json",
        "attestation": release_dir / f"{asset_name}.attestation.json",
    }
    resolved_release_dir = release_dir.resolve()
    if (
        release_dir.is_symlink()
        or resolved_release_dir.parent != prepared_root.resolve()
        or any(
            path.is_symlink() or not path.is_file() or path.resolve().parent != resolved_release_dir
            for path in paths.values()
        )
    ):
        raise ValueError("prepared release inputs must be regular files in the authenticated release directory")
    if any(
        paths[name].stat().st_size > (1 << 20)
        for name in ("manifest", "checksums", "identity", "attestation")
    ):
        raise ValueError("prepared release metadata exceeds stable launch size bounds")

    manifest_body = paths["manifest"].read_bytes()
    checksums_body = paths["checksums"].read_bytes()
    manifest = json.loads(manifest_body)
    identity = json.loads(paths["identity"].read_text())
    attestation = json.loads(paths["attestation"].read_text())
    manifest_digest = "sha256:" + hashlib.sha256(manifest_body).hexdigest()
    checksums_digest = "sha256:" + hashlib.sha256(checksums_body).hexdigest()
    tag = prepared.get("cli_release_tag")
    validate_release_manifest(
        manifest,
        repository=config["cli_release_repository"],
        tag=str(tag),
        tag_commit=str(identity.get("tag_commit", "")),
    )
    if (
        manifest != prepared.get("release_manifest")
        or manifest_digest != prepared.get("release_manifest_digest")
        or checksums_digest != prepared.get("release_checksums_digest")
        or identity != prepared.get("github_release_identity")
        or set(identity) != {"repository", "tag", "tag_commit", "release_id", "immutable"}
        or identity.get("repository") != config["cli_release_repository"]
        or identity.get("tag") != config["cli_release_tag"]
        or identity.get("tag_commit") != config["cli_release_commit"]
        or not isinstance(identity.get("release_id"), int)
        or isinstance(identity.get("release_id"), bool)
        or identity["release_id"] <= 0
        or identity.get("immutable") is not True
    ):
        raise ValueError("prepared release metadata differs from the immutable reviewed identity")

    checksum_entries: dict[str, str] = {}
    for line in checksums_body.decode("ascii").splitlines():
        match = re.fullmatch(r"([a-f0-9]{64})  ([A-Za-z0-9_.-]+)", line)
        if not match or match.group(2) in checksum_entries:
            raise ValueError("prepared checksums.txt is non-canonical")
        checksum_entries[match.group(2)] = match.group(1)
    checksum_names = {item["file"] for item in manifest["assets"].values()} | {RELEASE_MANIFEST_NAME}
    if (
        set(checksum_entries) != checksum_names
        or checksum_entries[RELEASE_MANIFEST_NAME] != manifest_digest.removeprefix("sha256:")
    ):
        raise ValueError("prepared checksums do not authenticate the exact release manifest asset set")

    declared = next((item for item in manifest["assets"].values() if item["file"] == asset_name), None)
    if declared is None or paths["binary"].stat().st_size != declared["size"] or declared["size"] > (1 << 28):
        raise ValueError("prepared native asset has an invalid or excessive size")
    binary_body = paths["binary"].read_bytes()
    binary_digest = "sha256:" + hashlib.sha256(binary_body).hexdigest()
    authenticated_asset = prepared.get("authenticated_asset")
    if (
        len(binary_body) != declared["size"]
        or binary_digest != "sha256:" + declared["sha256"]
        or checksum_entries.get(asset_name) != declared["sha256"]
        or authenticated_asset != {"name": asset_name, "digest": binary_digest}
    ):
        raise ValueError("prepared native asset bytes differ from the authenticated manifest/checksum identity")

    expected_attestation = {
        "repository": config["cli_release_repository"],
        "workflow": config["cli_release_workflow"],
        "tag": config["cli_release_tag"],
        "tag_commit": config["cli_release_commit"],
        "asset_name": asset_name,
        "asset_digest": binary_digest,
        "verified": True,
    }
    if attestation != expected_attestation or attestation != prepared.get("github_asset_attestation"):
        raise ValueError("prepared native asset attestation differs from the trusted release identity")
    return paths["binary"], manifest, identity, manifest_digest, checksums_digest


def verify_github_asset_attestation(
    asset: Path, repository: str, workflow: str, tag: str, tag_commit: str, digest: str,
    subject_name: str | None = None,
) -> dict[str, Any]:
    """Cryptographically verify one GitHub artifact attestation with fixed identities."""
    if (
        repository != TRUSTED_CLI_RELEASE_REPOSITORY
        or workflow != TRUSTED_CLI_RELEASE_WORKFLOW
        or tag != TRUSTED_CLI_RELEASE_TAG
        or tag_commit != TRUSTED_CLI_RELEASE_COMMIT
    ):
        raise ValueError("artifact attestation repository/workflow is not the trusted release identity")
    if not FULL_SHA.fullmatch(tag_commit) or not DIGEST.fullmatch(digest):
        raise ValueError("artifact attestation commit or digest is invalid")
    subject_name = asset.name if subject_name is None else subject_name
    if not isinstance(subject_name, str) or Path(subject_name).name != subject_name or not subject_name:
        raise ValueError("artifact attestation subject name is invalid")
    command = [
        "gh", "attestation", "verify", str(asset), "--repo", repository,
        "--signer-workflow", workflow, "--source-ref", TRUSTED_CLI_RELEASE_SOURCE_REF,
        "--source-digest", tag_commit, "--predicate-type", "https://slsa.dev/provenance/v1",
        "--deny-self-hosted-runners", "--format", "json",
    ]
    try:
        completed = subprocess.run(command, text=True, capture_output=True, timeout=120, check=False)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise ValueError("GitHub artifact attestation verifier is unavailable") from error
    if completed.returncode:
        raise ValueError("GitHub artifact attestation is missing, invalid, or has the wrong release identity")
    try:
        records = strict_json_loads(completed.stdout)
    except (json.JSONDecodeError, ValueError) as error:
        raise ValueError("GitHub artifact attestation verifier returned invalid JSON") from error
    if not isinstance(records, list) or not records:
        raise ValueError("GitHub artifact attestation verifier returned no verified statement")
    expected_sha = digest.removeprefix("sha256:")
    matching = []
    for record in records:
        statement = record.get("verificationResult", {}).get("statement", {}) if isinstance(record, dict) else {}
        subjects = statement.get("subject", []) if isinstance(statement, dict) else []
        if (
            statement.get("predicateType") == "https://slsa.dev/provenance/v1"
            and any(subject.get("name") == subject_name and subject.get("digest", {}).get("sha256") == expected_sha for subject in subjects if isinstance(subject, dict))
        ):
            matching.append(record)
    if not matching:
        raise ValueError("GitHub artifact attestation subject name/digest does not match the native asset")
    return {
        "repository": repository, "workflow": workflow, "tag": tag,
        "tag_commit": tag_commit, "issuer": "https://token.actions.githubusercontent.com",
        "source_ref": TRUSTED_CLI_RELEASE_SOURCE_REF, "source_digest": tag_commit,
        "predicate_type": "https://slsa.dev/provenance/v1", "subject_name": subject_name,
        "subject_digest": digest, "runner_environment": "github-hosted",
        "asset_name": asset.name, "asset_digest": digest,
        "verified": True,
    }


def resolve_npm_package(
    package: str, version: str, destination: Path, *,
    fixture_fetch: Callable[[str, int, str], bytes] | None = None,
) -> tuple[Path, dict[str, Any]]:
    """Resolve an exact npm version from the registry and verify its SRI bytes.

    The registry endpoints and package identity are derived here. Production
    callers cannot pair an arbitrary tarball URL with caller-supplied integrity.
    ``fixture_fetch`` is reserved for direct tests.
    """
    if package != "universal-agent-plugins" or not VERSION.fullmatch(version):
        raise ValueError("npm facade must be the exact universal-agent-plugins semantic version")
    metadata_url = f"https://registry.npmjs.org/{package}/{version}"

    def fetch(url: str, limit: int, accept: str) -> bytes:
        if fixture_fetch:
            return fixture_fetch(url, limit, accept)
        return bounded_https_get(url, maximum=limit, accept=accept)

    metadata_body = fetch(metadata_url, 1 << 20, "application/json")
    metadata = json.loads(metadata_body)
    dist = metadata.get("dist", {})
    integrity = dist.get("integrity")
    tarball_url = dist.get("tarball")
    attestations = dist.get("attestations", {})
    expected_tarball = f"https://registry.npmjs.org/{package}/-/{package}-{version}.tgz"
    expected_provenance_url = f"https://registry.npmjs.org/-/npm/v1/attestations/{package}@{version}"
    if metadata.get("name") != package or metadata.get("version") != version:
        raise ValueError("npm registry metadata does not match the exact requested package version")
    if not isinstance(integrity, str) or not integrity.startswith("sha512-") or tarball_url != expected_tarball:
        raise ValueError("npm registry metadata lacks the expected SHA-512 integrity/tarball identity")
    if (
        not isinstance(attestations, dict)
        or attestations.get("url") != expected_provenance_url
        or attestations.get("provenance", {}).get("predicateType") != "https://slsa.dev/provenance/v1"
    ):
        raise ValueError("npm registry metadata lacks exact SLSA npm provenance")
    try:
        expected = base64.b64decode(integrity.removeprefix("sha512-"), validate=True)
    except ValueError as error:
        raise ValueError("npm registry dist.integrity is invalid") from error
    body = fetch(expected_tarball, 1 << 28, "application/octet-stream")
    if not hmac.compare_digest(hashlib.sha512(body).digest(), expected):
        raise ValueError("npm tarball bytes disagree with registry dist.integrity")
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_bytes(body)
    destination.chmod(0o400)
    return destination, {
        "name": package, "version": version, "integrity": integrity,
        "tarball": expected_tarball, "sha256": hashlib.sha256(body).hexdigest(),
        "size": len(body), "metadata_digest": "sha256:" + hashlib.sha256(metadata_body).hexdigest(),
        "provenance_url": expected_provenance_url,
        "provenance_predicate_type": "https://slsa.dev/provenance/v1",
    }


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def fresh_observation_interval(started_value: Any, observed_value: Any, *, now: datetime | None = None) -> bool:
    """Apply the same hard freshness window to current- and earlier-attempt evidence."""
    try:
        started = datetime.fromisoformat(str(started_value).replace("Z", "+00:00"))
        observed = datetime.fromisoformat(str(observed_value).replace("Z", "+00:00"))
    except ValueError:
        return False
    current = now or datetime.now(timezone.utc)
    return bool(
        started.tzinfo is not None and observed.tzinfo is not None
        and started <= observed <= current + timedelta(minutes=2)
        and current - observed <= MAX_ATTESTATION_AGE
    )


def current_or_earlier_attempt(observed_attempt: Any, current_attempt: Any) -> bool:
    return bool(
        str(observed_attempt).isascii() and str(observed_attempt).isdigit()
        and str(current_attempt).isascii() and str(current_attempt).isdigit()
        and 1 <= int(str(observed_attempt)) <= int(str(current_attempt))
    )


def sha256_file(path: Path) -> str:
    return "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()


def validate_observer_bundle_files(
    bundle_path: Path, *, challenge: str, public_key_base64: str,
    expected_key_id: str, artifact_paths: dict[str, Path | None],
) -> None:
    bundle = json.loads(bundle_path.read_text())
    artifacts = verify_observer_bundle(
        bundle, challenge=challenge, public_key_base64=public_key_base64,
        expected_key_id=expected_key_id,
    )
    if set(artifact_paths) != set(artifacts) or any(path is None for path in artifact_paths.values()):
        raise ValueError("signed observer bundle and launch inputs have different artifact sets")
    for name, path in artifact_paths.items():
        assert path is not None
        if json.loads(path.read_text()) != artifacts[name]:
            raise ValueError(f"launch input differs from signed observer artifact: {name}")


def package_digest(path: Path) -> str:
    """Return the Directory/Go CLI package identity, including dirs and modes."""
    return directory_tree_digest(path)


def materialization_digest(path: Path) -> str:
    """Digest arbitrary observer state; this is not a package identity."""
    framed = bytearray(b"uap-fixture-materialization-v1\0")
    snapshot, bodies = _stable_tree_snapshot(path)
    if not snapshot:
        framed.extend(b"absent")
        return "sha256:" + hashlib.sha256(framed).hexdigest()
    for name, item in sorted(snapshot.items()):
        if name == ".":
            continue
        relative = name.encode()
        kind = item["kind"].encode()
        mode = b"100755" if item["kind"] == "file" and item["mode"] & 0o111 else b"100644"
        body = bodies.get(name, b"")
        for field in (relative, kind, mode, body):
            framed.extend(len(field).to_bytes(8, "big") + field)
    return "sha256:" + hashlib.sha256(framed).hexdigest()


def observed_state_identity(environment: dict[str, str], product_id: str, clients: tuple[str, ...]) -> dict[str, Any]:
    manager = Path(environment["AGENTPLUGINS_HOME"])
    home = Path(environment["HOME"])
    installation = selected_manager_installation(manager, product_id)
    roots = {client: home / CLIENT_ROOTS[client] for client in clients}
    native_digests = {client: materialization_digest(path) for client, path in roots.items()}
    # Materialized files are not proof that a client discovered a plugin, and
    # their digest is not a client version. Runtime claims come only from the
    # protected external observer contract validated by _load_attestations.
    receipts = installation_receipts(manager, product_id) if installation else None
    committed = len(receipts or [])
    physical_ids = {
        binding.get("physical_artifact_id")
        for binding in installation.get("clients", {}).values()
        if isinstance(binding, dict) and isinstance(binding.get("physical_artifact_id"), str)
    } if installation else set()
    return {
        "product_id": find_value(installation, {"product_id"}) if installation else None,
        "tree_digest": find_value(installation, {"tree_digest"}) if installation else None,
        "manifest_digest": find_value(installation, {"manifest_digest"}) if installation else None,
        "distribution_id": find_value(installation, {"distribution_id"}) if installation else None,
        "distribution_kind": find_value(installation, {"distribution_kind"}) if installation else None,
        "release_sequence": find_value(installation, {"desired_release_sequence"}) if installation else None,
        "package_version": find_value(installation.get("package", {}), {"version"}) if installation else None,
        "physical_artifact_id": next(iter(physical_ids)) if len(physical_ids) == 1 else None,
        "snapshot_sequence": find_value(installation, {"snapshot_sequence"}) if installation else None,
        "snapshot_digest": find_value(installation, {"snapshot_digest"}) if installation else None,
        "client_version": None,
        "receipt_reconciled": committed > 0,
        "native_discovery_reconciled": False,
        "evidence_basis": "fixture_materialization", "runtime_proof": False,
        "manager_digest": materialization_digest(manager), "native_digests": native_digests,
    }


def parse_stable_version(value: str) -> tuple[int, int, int]:
    match = VERSION.fullmatch(value)
    if not match:
        raise ValueError("Agent Plugins version must be an exact semantic version")
    parsed = tuple(int(match.group(index)) for index in (1, 2, 3))
    if parsed < MINIMUM_STABLE_VERSION:
        raise ValueError("stable launch requires agentplugins 0.1.18 or newer")
    return parsed


def portable_ed25519_verify(public_key: bytes, message: bytes, signature: bytes) -> None:
    """Verify Directory signatures with the pinned cross-platform dependency."""
    from cryptography.exceptions import InvalidSignature
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey

    try:
        Ed25519PublicKey.from_public_bytes(public_key).verify(signature, message)
    except (InvalidSignature, ValueError) as error:
        raise PublicationError("Ed25519 signature verification failed") from error


def validated_directory_environment(
    origin: str, snapshot_path: Path, envelope_path: Path, trust_path: Path
) -> tuple[dict[str, str], dict[str, Any], str]:
    parsed = urlsplit(origin)
    if parsed.scheme != "https" or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise ValueError("Directory origin must be credential-free public HTTPS")
    snapshot_bytes = snapshot_path.read_bytes()
    try:
        snapshot = parse_json_bytes(snapshot_bytes, "Directory snapshot", max_bytes=MAX_SNAPSHOT_BYTES)
        envelope_bytes = envelope_path.read_bytes()
        envelope = parse_json_bytes(envelope_bytes, "Directory envelope", max_bytes=MAX_ENVELOPE_BYTES)
        if canonical_json(snapshot) != snapshot_bytes or canonical_json(envelope) != envelope_bytes:
            raise PublicationError("Directory snapshot and envelope must be canonical JSON")
        verify_envelope(
            snapshot_bytes,
            envelope,
            load_public_keys(trust_path),
            signature_verifier=portable_ed25519_verify,
        )
        validate_snapshot_semantics(snapshot)
        if envelope["sequence"] != snapshot["sequence"] or envelope["snapshot_schema_version"] != snapshot["snapshot_schema_version"]:
            raise PublicationError("Directory envelope identity does not match snapshot")
        now = datetime.now(timezone.utc)
        if now < parse_timestamp(snapshot["generated_at"], "generated_at"):
            raise PublicationError("Directory snapshot is not yet valid")
        if now >= parse_timestamp(snapshot["expires_at"], "expires_at"):
            raise PublicationError("Directory snapshot is expired")
    except (PublicationError, OSError, KeyError, TypeError) as error:
        raise ValueError(f"invalid signed Directory: {error}") from error
    digest = "sha256:" + hashlib.sha256(snapshot_bytes).hexdigest()
    return ({
        "AGENTPLUGINS_DIRECTORY_ORIGIN": origin,
        "AGENTPLUGINS_DIRECTORY_SNAPSHOT": str(snapshot_path.resolve()),
        "AGENTPLUGINS_DIRECTORY_ENVELOPE": str(envelope_path.resolve()),
        "AGENTPLUGINS_DIRECTORY_TRUST": str(trust_path.resolve()),
    }, snapshot, digest)


def production_identity_from_materialized_ledger(root: Path) -> dict[str, Any]:
    """Authenticate the current materialized ledger before probing Pages."""
    root = root.resolve(strict=True)
    latest_path = root / "latest.json"
    latest_bytes = latest_path.read_bytes()
    try:
        latest = parse_json_bytes(
            latest_bytes, "materialized production latest pointer",
            max_bytes=MAX_LATEST_BYTES,
        )
        if canonical_json(latest) != latest_bytes:
            raise PublicationError("materialized production latest pointer is not canonical JSON")
        validate_latest(latest)
    except (PublicationError, OSError, TypeError) as error:
        raise ValueError(f"invalid materialized production pointer: {error}") from error

    resolved: dict[str, Path] = {}
    for key in ("snapshot_path", "envelope_path"):
        relative = latest[key]
        candidate = root / relative
        cursor = root
        for part in Path(relative).parts:
            cursor /= part
            if cursor.is_symlink():
                raise ValueError("materialized production path contains a symlink")
        try:
            candidate.resolve(strict=True).relative_to(root)
        except (OSError, ValueError) as error:
            raise ValueError("materialized production path escapes its ledger root") from error
        if not candidate.is_file():
            raise ValueError("materialized production artifact is not a regular file")
        resolved[key] = candidate

    config = read_production_config()
    _, snapshot, digest = validated_directory_environment(
        config["production_origin"], resolved["snapshot_path"],
        resolved["envelope_path"], PRODUCTION_DIRECTORY_TRUST,
    )
    if snapshot.get("sequence") != latest.get("sequence"):
        raise ValueError("materialized production pointer and signed snapshot disagree")
    identity = {
        "publication_id": snapshot.get("publication_id"),
        "sequence": snapshot.get("sequence"),
        "snapshot_digest": digest,
        "source_commit": snapshot.get("source_commit"),
    }
    if (
        not isinstance(identity["publication_id"], str) or not identity["publication_id"]
        or type(identity["sequence"]) is not int or identity["sequence"] < 1
        or not DIGEST.fullmatch(identity["snapshot_digest"])
        or not FULL_SHA.fullmatch(str(identity["source_commit"]))
    ):
        raise ValueError("materialized production identity is incomplete")
    return identity


def fetch_production_directory(
    destination: Path, *, expected_publication_id: str, expected_sequence: int,
    expected_snapshot_digest: str, expected_source_commit: str,
) -> tuple[dict[str, str], dict[str, Any], str]:
    """Fetch pointer/envelope/snapshot from the checked-in production origin.

    The trust root is deliberately never fetched. It is copied from the exact
    checked-out repository revision after real Ed25519 verification.
    """
    if expected_sequence < 1 or not expected_publication_id or not FULL_SHA.fullmatch(expected_source_commit) or not DIGEST.fullmatch(expected_snapshot_digest):
        raise ValueError("caller publication identity is incomplete or invalid")
    config = read_production_config()
    origin = config["production_origin"].rstrip("/") + "/"
    latest_body = bounded_https_get(origin + "latest.json", maximum=MAX_LATEST_BYTES, accept="application/json")
    try:
        latest = parse_json_bytes(latest_body, "production latest pointer", max_bytes=MAX_LATEST_BYTES)
        if canonical_json(latest) != latest_body:
            raise PublicationError("production latest pointer is not canonical JSON")
        validate_latest(latest)
    except (PublicationError, TypeError) as error:
        raise ValueError(f"invalid production Directory pointer: {error}") from error
    expected_snapshot_path = f"snapshots/{expected_sequence:020d}.json"
    expected_envelope_path = f"snapshots/{expected_sequence:020d}.envelope.json"
    if (
        latest.get("sequence") != expected_sequence
        or latest.get("snapshot_path") != expected_snapshot_path
        or latest.get("envelope_path") != expected_envelope_path
    ):
        raise ValueError("production Directory pointer does not match the exact caller publication identity")
    snapshot_body = bounded_https_get(origin + latest["snapshot_path"], maximum=latest["fetch_contract"]["snapshot_max_bytes"], accept="application/json")
    envelope_body = bounded_https_get(origin + latest["envelope_path"], maximum=latest["fetch_contract"]["envelope_max_bytes"], accept="application/json")
    destination.mkdir(parents=True, exist_ok=False)
    snapshot_path = destination / "snapshot.json"
    envelope_path = destination / "envelope.json"
    trust_path = destination / "trusted-keys.json"
    snapshot_path.write_bytes(snapshot_body)
    envelope_path.write_bytes(envelope_body)
    shutil.copy2(PRODUCTION_DIRECTORY_TRUST, trust_path)
    environment, snapshot, digest = validated_directory_environment(origin, snapshot_path, envelope_path, trust_path)
    if snapshot["sequence"] != latest["sequence"]:
        raise ValueError("production Directory pointer and signed snapshot sequence disagree")
    if (
        snapshot.get("publication_id") != expected_publication_id
        or snapshot.get("source_commit") != expected_source_commit
        or digest != expected_snapshot_digest
    ):
        raise ValueError("signed production Directory snapshot is stale or differs from the exact caller publication identity")
    return environment, snapshot, digest


def fetch_staged_directory(
    destination: Path, *, repository: str, ledger_commit: str,
    expected_publication_id: str, expected_sequence: int,
    expected_snapshot_digest: str, expected_source_commit: str,
    fixture_fetch: Callable[[str, int, str], bytes] | None = None,
) -> tuple[dict[str, str], dict[str, Any], str]:
    """Reacquire and verify one publication from an immutable ledger commit."""
    if repository != TRUSTED_CATALOG_REPOSITORY or not FULL_SHA.fullmatch(ledger_commit):
        raise ValueError("staged publication ledger identity is invalid")
    if expected_sequence < 1 or not expected_publication_id or not FULL_SHA.fullmatch(expected_source_commit) or not DIGEST.fullmatch(expected_snapshot_digest):
        raise ValueError("caller publication identity is incomplete or invalid")
    origin = f"https://raw.githubusercontent.com/{repository}/{ledger_commit}/registry/schemas/1/"

    def fetch(relative: str, maximum: int) -> bytes:
        url = origin + relative
        if fixture_fetch:
            return fixture_fetch(url, maximum, "application/json")
        return bounded_https_get(url, maximum=maximum, accept="application/json")

    latest_body = fetch("latest.json", MAX_LATEST_BYTES)
    try:
        latest = parse_json_bytes(latest_body, "staged latest pointer", max_bytes=MAX_LATEST_BYTES)
        if canonical_json(latest) != latest_body:
            raise PublicationError("staged latest pointer is not canonical JSON")
        validate_latest(latest)
    except (PublicationError, TypeError) as error:
        raise ValueError(f"invalid staged Directory pointer: {error}") from error
    expected_snapshot_path = f"snapshots/{expected_sequence:020d}.json"
    expected_envelope_path = f"snapshots/{expected_sequence:020d}.envelope.json"
    if (
        latest.get("sequence") != expected_sequence
        or latest.get("snapshot_path") != expected_snapshot_path
        or latest.get("envelope_path") != expected_envelope_path
    ):
        raise ValueError("staged Directory pointer does not match the exact caller publication identity")
    snapshot_body = fetch(latest["snapshot_path"], latest["fetch_contract"]["snapshot_max_bytes"])
    envelope_body = fetch(latest["envelope_path"], latest["fetch_contract"]["envelope_max_bytes"])
    destination.mkdir(parents=True, exist_ok=False)
    snapshot_path = destination / "snapshot.json"
    envelope_path = destination / "envelope.json"
    trust_path = destination / "trusted-keys.json"
    snapshot_path.write_bytes(snapshot_body)
    envelope_path.write_bytes(envelope_body)
    shutil.copy2(PRODUCTION_DIRECTORY_TRUST, trust_path)
    environment, snapshot, digest = validated_directory_environment(origin, snapshot_path, envelope_path, trust_path)
    if snapshot["sequence"] != latest["sequence"]:
        raise ValueError("staged Directory pointer and signed snapshot sequence disagree")
    if (
        snapshot.get("publication_id") != expected_publication_id
        or snapshot.get("source_commit") != expected_source_commit
        or digest != expected_snapshot_digest
    ):
        raise ValueError("signed staged Directory snapshot differs from the exact caller publication identity")
    return environment, snapshot, digest


def isolated_environment(sandbox: Path, clients: tuple[str, ...], directory_environment: dict[str, str] | None = None) -> dict[str, str]:
    """Return an allowlisted environment with disposable homes and no credentials."""
    allowed = ("PATH", "LANG", "LC_ALL", "LC_CTYPE", "TZ", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "SSL_CERT_FILE", "SSL_CERT_DIR")
    env = {key: os.environ[key] for key in allowed if key in os.environ}
    home = sandbox / "home"
    temp = sandbox / "runtime" / "tmp"
    for path in (home, temp, sandbox / "config", sandbox / "cache", sandbox / "workspace", sandbox / "runtime", sandbox / "evidence"):
        path.mkdir(parents=True, exist_ok=True)
    for client in clients:
        (home / CLIENT_ROOTS[client]).mkdir(parents=True, exist_ok=True)
    env.update({
        "HOME": str(home), "USERPROFILE": str(home),
        "XDG_CONFIG_HOME": str(sandbox / "config"), "XDG_CACHE_HOME": str(sandbox / "cache"),
        "AGENTPLUGINS_HOME": str(sandbox / "runtime" / "agentplugins"),
        "AGENTPLUGINS_EVIDENCE_ROOT": str(sandbox / "evidence"),
        "TMPDIR": str(temp), "TMP": str(temp), "TEMP": str(temp),
        "GIT_CONFIG_GLOBAL": str(sandbox / "gitconfig"), "GIT_CONFIG_NOSYSTEM": "1",
        "GIT_TERMINAL_PROMPT": "0", "CI": "true",
    })
    if directory_environment:
        if set(directory_environment) != DIRECTORY_INPUT_ENVIRONMENT_KEYS:
            raise ValueError("Directory environment must contain the complete origin/snapshot/envelope/trust tuple")
        fixture_root = sandbox / "config" / "directory-trust"
        fixture_root.mkdir(parents=True, exist_ok=True)
        launch_directory_environment = {
            "AGENTPLUGINS_DIRECTORY_ORIGIN": directory_environment["AGENTPLUGINS_DIRECTORY_ORIGIN"],
        }
        for key, filename in (
            ("AGENTPLUGINS_DIRECTORY_SNAPSHOT", "snapshot.json"),
            ("AGENTPLUGINS_DIRECTORY_ENVELOPE", "envelope.json"),
            ("AGENTPLUGINS_DIRECTORY_TRUST", "trusted-keys.json"),
        ):
            target = fixture_root / filename
            shutil.copy2(directory_environment[key], target)
            launch_directory_environment[key] = str(target)
        launch_directory_environment["AGENTPLUGINS_DIRECTORY_CACHE"] = str(sandbox / "cache" / "directory")
        launch_directory_environment["AGENTPLUGINS_DIRECTORY_CONFORMANCE_ONLY"] = "1"
        if set(launch_directory_environment) != DIRECTORY_LAUNCH_ENVIRONMENT_KEYS:
            raise AssertionError("incomplete Directory launch environment")
        env.update(launch_directory_environment)
    return env


def find_value(value: Any, keys: set[str]) -> Any:
    if isinstance(value, dict):
        for key, child in value.items():
            if key in keys and child not in (None, ""):
                return child
        for child in value.values():
            found = find_value(child, keys)
            if found not in (None, ""):
                return found
    elif isinstance(value, list):
        for child in value:
            found = find_value(child, keys)
            if found not in (None, ""):
                return found
    return None


def collect_digests(value: Any) -> set[str]:
    found: set[str] = set()
    if isinstance(value, dict):
        for key, child in value.items():
            if key in {"package_digest", "tree_digest"} and isinstance(child, str) and DIGEST.fullmatch(child):
                found.add(child)
            found.update(collect_digests(child))
    elif isinstance(value, list):
        for child in value:
            found.update(collect_digests(child))
    return found


def logical_root_id(github_sha: str, run_id: str, run_attempt: str) -> str:
    framed = "\0".join((github_sha, run_id, run_attempt)).encode("ascii")
    return hashlib.sha256(b"UAP-STABLE-LAUNCH-LOGICAL-ROOT-V1\0" + framed).hexdigest()


def exported_root_id(challenge: dict[str, str] | None) -> str:
    """Return the portable public identifier, never a temporary-path hash."""
    return challenge["root_id"][:16] if challenge else "0" * 16


def make_challenge(
    github_sha: str, run_id: str, run_attempt: str,
    caller_event_name: str, caller_ref: str, caller_workflow_ref: str,
    release_digest: str, directory_digest: str, scenario_contract_digest: str,
    run_root: Path,
) -> dict[str, str]:
    if not FULL_SHA.fullmatch(github_sha) or not run_id.isdigit() or not run_attempt.isdigit():
        raise ValueError("enforced evidence requires exact GitHub SHA/run identity")
    if not all(DIGEST.fullmatch(item) for item in (release_digest, directory_digest, scenario_contract_digest)):
        raise ValueError("challenge inputs require release and Directory digests")
    expected_workflow_ref = f"{TRUSTED_CATALOG_REPOSITORY}/.github/workflows/directory-publication.yml@refs/heads/main"
    if caller_event_name not in {"push", "schedule", "workflow_dispatch"} or caller_ref != "refs/heads/main" or caller_workflow_ref != expected_workflow_ref:
        raise ValueError("enforced evidence requires an approved protected Directory publication caller")
    nonce = secrets.token_hex(32)
    # This identity crosses isolated GitHub jobs. The concrete temporary path
    # is independently safety-checked by each consumer and is not portable.
    root_id = logical_root_id(github_sha, run_id, run_attempt)
    framed = json.dumps({
        "github_sha": github_sha, "run_id": run_id, "run_attempt": run_attempt,
        "release_manifest_digest": release_digest, "directory_digest": directory_digest,
        "scenario_contract_digest": scenario_contract_digest,
        "root_id": root_id, "nonce": nonce,
    }, sort_keys=True, separators=(",", ":")).encode()
    return {
        "value": hashlib.sha256(CHALLENGE_DOMAIN + framed).hexdigest(),
        "nonce": nonce, "github_sha": github_sha, "run_id": run_id,
        "run_attempt": run_attempt, "release_manifest_digest": release_digest,
        "directory_digest": directory_digest, "scenario_contract_digest": scenario_contract_digest, "root_id": root_id,
    }


def challenge_context_valid(value: Any) -> bool:
    if not isinstance(value, dict) or set(value) != {"value", "nonce", "github_sha", "run_id", "run_attempt", "release_manifest_digest", "directory_digest", "scenario_contract_digest", "root_id"}:
        return False
    if not all(isinstance(value.get(field), str) for field in value):
        return False
    if not FULL_SHA.fullmatch(value["github_sha"]) or not value["run_id"].isdigit() or not value["run_attempt"].isdigit():
        return False
    if not all(DIGEST.fullmatch(value[field]) for field in ("release_manifest_digest", "directory_digest", "scenario_contract_digest")):
        return False
    if not re.fullmatch(r"[a-f0-9]{64}", value["nonce"]) or not re.fullmatch(r"[a-f0-9]{64}", value["root_id"]):
        return False
    framed = json.dumps({
        "github_sha": value["github_sha"], "run_id": value["run_id"], "run_attempt": value["run_attempt"],
        "release_manifest_digest": value["release_manifest_digest"], "directory_digest": value["directory_digest"],
        "scenario_contract_digest": value["scenario_contract_digest"],
        "root_id": value["root_id"], "nonce": value["nonce"],
    }, sort_keys=True, separators=(",", ":")).encode()
    return value["value"] == hashlib.sha256(CHALLENGE_DOMAIN + framed).hexdigest()


def external_pr_evidence_valid(
    record: Any, *, catalog_repository: str,
    catalog_sha: str | None, snapshot: dict[str, Any], snapshot_digest: str | None,
    release_repository: str, release_tag: str | None, release_commit: str | None,
    release_manifest_digest: str | None, now: datetime | None = None,
) -> tuple[bool, str]:
    """Verify a reusable PR capture for this exact catalog and Directory state."""
    if not isinstance(record, dict):
        return False, "immutable external-fork PR evidence was not supplied"
    required_fields = {
        "schema_version", "challenge", "catalog_repository", "fork_owner", "fork_repository",
        "pr_number", "pr_url", "head_sha", "base_sha", "merge_commit_sha", "changed_paths",
        "check_runs", "final_review", "observed_at", "immutable_artifact", "binding",
    }
    if set(record) != required_fields or record.get("schema_version") != 1:
        return False, "external-fork PR evidence record is non-canonical"
    current = now or datetime.now(timezone.utc)
    try:
        observed = datetime.fromisoformat(str(record["observed_at"]).replace("Z", "+00:00"))
    except (KeyError, TypeError, ValueError):
        return False, "external-fork PR observation timestamp is invalid"
    if observed.tzinfo is None or observed > current + timedelta(minutes=2):
        return False, "external-fork PR evidence is from the future"
    if not isinstance(record.get("challenge"), str) or not re.fullmatch(r"[a-f0-9]{64}", record["challenge"]):
        return False, "external-fork PR capture challenge is invalid"
    binding = record.get("binding")
    binding_fields = {
        "catalog_repository", "catalog_sha", "directory_snapshot_digest",
        "directory_sequence", "directory_publication_id", "directory_source_commit",
        "release_repository", "release_tag", "release_commit", "release_manifest_digest",
    }
    if not isinstance(binding, dict) or set(binding) != binding_fields:
        return False, "external-fork PR capture binding is non-canonical"
    expected_binding = {
        "catalog_repository": catalog_repository,
        "catalog_sha": catalog_sha,
        "directory_snapshot_digest": snapshot_digest,
        "directory_sequence": snapshot.get("sequence"),
        "directory_publication_id": snapshot.get("publication_id"),
        "directory_source_commit": snapshot.get("source_commit"),
        "release_repository": release_repository,
        "release_tag": release_tag,
        "release_commit": release_commit,
        "release_manifest_digest": release_manifest_digest,
    }
    if binding != expected_binding:
        return False, "external-fork PR capture is bound to another catalog, Directory, or stable release"
    repository = record.get("catalog_repository")
    fork_owner = record.get("fork_owner")
    fork_repository = record.get("fork_repository")
    if repository != catalog_repository or not isinstance(fork_owner, str) or not IDENTITY_ID.fullmatch(fork_owner):
        return False, "external-fork PR repository identity is invalid"
    catalog_owner = catalog_repository.split("/", 1)[0].lower()
    if fork_owner.lower() == catalog_owner or not isinstance(fork_repository, str) or fork_repository.split("/", 1)[0].lower() != fork_owner.lower():
        return False, "external-fork PR must come from an owner distinct from the catalog owner"
    if fork_repository.lower() == catalog_repository.lower() or not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", fork_repository):
        return False, "external-fork PR fork repository is missing, local, or self-owned"
    number = record.get("pr_number")
    if not isinstance(number, int) or isinstance(number, bool) or number < 1 or record.get("pr_url") != f"https://github.com/{catalog_repository}/pull/{number}":
        return False, "external-fork PR number/URL is not canonical"
    for field in ("head_sha", "base_sha"):
        if not isinstance(record.get(field), str) or not FULL_SHA.fullmatch(record[field]):
            return False, f"external-fork PR {field} is invalid"
    if record["head_sha"] == record["base_sha"] or record["base_sha"] != catalog_sha or record.get("merge_commit_sha") is not None:
        return False, "external-fork PR exact head/base identity or no-merge proof is inconsistent"
    paths = record.get("changed_paths")
    paths_valid = isinstance(paths, list) and bool(paths) and all(isinstance(path, str) for path in paths)
    paths_valid = paths_valid and len(paths) == len(set(paths))
    if paths_valid:
        paths_valid = all(
            isinstance(path, str)
            and CONTRIBUTOR_PATH.fullmatch(path)
            and "\\" not in path
            and all(part not in {"", ".", ".."} for part in PurePosixPath(path).parts)
            for path in paths
        )
    if not paths_valid:
        return False, "external-fork PR changed paths escape the current contributor flow"
    checks = record.get("check_runs")
    if not isinstance(checks, list) or not checks:
        return False, "external-fork PR has no check-run evidence"
    names: set[str] = set()
    for check in checks:
        if not isinstance(check, dict) or set(check) != {"name", "conclusion", "head_sha"}:
            return False, "external-fork PR check-run record is non-canonical"
        if not isinstance(check.get("name"), str) or not check["name"] or check["name"] in names:
            return False, "external-fork PR check-run names are missing or duplicated"
        names.add(check["name"])
        if check.get("conclusion") != "success":
            return False, "external-fork PR contains a failed or incomplete check run"
        if check.get("head_sha") != record["head_sha"]:
            return False, "external-fork PR check run is bound to the wrong head SHA"
    review = record.get("final_review")
    if not isinstance(review, dict) or review.get("state") != "closed" or review.get("decision") != "validated" or not isinstance(review.get("reviewer_count"), int) or isinstance(review.get("reviewer_count"), bool) or review["reviewer_count"] < 1:
        return False, "external-fork PR lacks a final validated closed-without-merge outcome"
    if set(review) != {"state", "decision", "reviewer_count", "closed_at", "merged_at"} or not isinstance(review.get("closed_at"), str) or review.get("merged_at") is not None:
        return False, "external-fork PR final review timestamp or no-merge proof is missing"
    try:
        closed = datetime.fromisoformat(review["closed_at"].replace("Z", "+00:00"))
    except ValueError:
        return False, "external-fork PR final review timestamps are invalid"
    if closed.tzinfo is None or closed > observed:
        return False, "external-fork PR final review chronology is invalid"
    artifact = record.get("immutable_artifact")
    if not isinstance(artifact, dict) or set(artifact) != {"digest", "reference"} or not DIGEST.fullmatch(str(artifact.get("digest", ""))):
        return False, "external-fork PR immutable artifact digest/reference is invalid"
    if artifact.get("reference") != "urn:" + artifact["digest"]:
        return False, "external-fork PR immutable artifact reference does not match its digest"
    return True, "signed immutable historical external-fork PR evidence verified"


class LaunchHarness:
    def __init__(
        self,
        binary: Path | None,
        attestations: Path | None,
        *,
        mode: str = "enforced",
        binary_digest: str | None = None,
        expected_version: str | None = None,
        directory_origin: str | None = None,
        directory_snapshot: Path | None = None,
        directory_envelope: Path | None = None,
        directory_trust: Path | None = None,
        run_root: Path | None = None,
        consent: Path | None = None,
        notion_oauth: Path | None = None,
        chatgpt_attestation: Path | None = None,
        release_manifest: dict[str, Any] | None = None,
        release_identity: dict[str, Any] | None = None,
        release_manifest_digest: str | None = None,
        release_checksums_digest: str | None = None,
        release_tag: str | None = None,
        github_sha: str | None = None,
        github_run_id: str | None = None,
        github_run_attempt: str | None = None,
        producer_run_attempt: str | None = None,
        caller_event_name: str | None = None,
        caller_ref: str | None = None,
        caller_workflow_ref: str | None = None,
        challenge: dict[str, str] | None = None,
        native_observations: Path | None = None,
        observer_bundle_digest: str | None = None,
        copilot_executable: Path | None = None,
        copilot_metadata: Path | None = None,
    ) -> None:
        self.config = json.loads(SCENARIOS.read_text())
        if mode not in {"enforced", "fixture-only"}:
            raise ValueError("mode must be enforced or fixture-only")
        self.mode = mode
        self.binary = binary.resolve() if binary else None
        self.expected_version = expected_version
        self.binary_digest = binary_digest
        self.release_manifest = release_manifest or {}
        self.release_identity = release_identity or {}
        self.release_manifest_digest = release_manifest_digest
        self.release_checksums_digest = release_checksums_digest
        self.release_tag = release_tag
        self.github_sha = github_sha
        self.github_run_id = github_run_id
        self.github_run_attempt = github_run_attempt
        self.producer_run_attempt = producer_run_attempt or github_run_attempt
        self.caller_event_name = caller_event_name
        self.caller_ref = caller_ref
        self.caller_workflow_ref = caller_workflow_ref
        self.native_observations = native_observations
        self.observer_bundle_digest = observer_bundle_digest
        self.copilot_executable = Path(os.path.abspath(copilot_executable)) if copilot_executable else None
        self.copilot_metadata_path = Path(os.path.abspath(copilot_metadata)) if copilot_metadata else None
        self.copilot_metadata = json.loads(self.copilot_metadata_path.read_text()) if self.copilot_metadata_path else {}
        self.directory_environment: dict[str, str] = {}
        self.snapshot: dict[str, Any] = {}
        self.snapshot_digest: str | None = None
        self.run_root = run_root.resolve() if run_root else None
        self._sandbox_counter = 0
        self.observed_at = utc_now()
        self.os_name = platform.system() or "unknown"
        self.architecture = platform.machine() or "unknown"
        self.cli_version: str | None = None
        self.rows: list[dict[str, Any]] = []
        supplied_directory = (directory_origin, directory_snapshot, directory_envelope, directory_trust)
        if any(item is not None for item in supplied_directory):
            if not all(item is not None for item in supplied_directory):
                raise ValueError("Directory origin, snapshot, envelope, and trust fixture are required together")
            self.directory_environment, self.snapshot, self.snapshot_digest = validated_directory_environment(
                str(directory_origin), Path(directory_snapshot), Path(directory_envelope), Path(directory_trust)
            )
        self.challenge = challenge
        if mode == "enforced" and self.challenge is None and self.run_root and self.release_manifest_digest and self.snapshot_digest:
            self.challenge = make_challenge(
                str(github_sha), str(github_run_id), str(github_run_attempt),
                "push", "refs/heads/main",
                f"{TRUSTED_CATALOG_REPOSITORY}/.github/workflows/directory-publication.yml@refs/heads/main",
                self.release_manifest_digest, self.snapshot_digest, sha256_file(SCENARIOS), self.run_root,
            )
        self.consent, self.consent_digest = self._load_consent(consent)
        self.external_pr_evidence: dict[str, Any] | None = None
        self.attestations = self._load_attestations(attestations, allow_external_pr=True)
        expected_runtime = {
            (plugin, client, "runtime")
            for plugin in ("agent-code-navigator", "context7", "cloudflare-docs", "chrome-devtools")
            for client in ("codex", "cursor", "kiro")
        }
        if self.mode == "enforced" and set(self.attestations) != expected_runtime:
            raise ValueError("primary runtime artifact must contain the exact 12 non-Notion pairs")
        notion_records = self._load_attestations(notion_oauth)
        expected_notion = {("notion", client, "runtime") for client in ("codex", "cursor", "kiro")}
        if any(key[0] == "notion" for key in self.attestations):
            raise ValueError("primary runtime artifact must not contain Notion records")
        if notion_records and set(notion_records) != expected_notion:
            raise ValueError("Notion artifact must contain exactly codex, cursor, and kiro runtime records")
        if any(record.get("outcome") != "passed" for record in notion_records.values()):
            raise ValueError("all Notion runtime records must be passed")
        chatgpt_records = self._load_attestations(chatgpt_attestation)
        if any(key != ("cloudflare-docs", "chatgpt", "runtime") for key in chatgpt_records):
            raise ValueError("ChatGPT artifact is scoped only to Cloudflare Docs registered binding")
        for records in (notion_records, chatgpt_records):
            overlap = set(self.attestations).intersection(records)
            if overlap:
                raise ValueError(f"duplicate attestation tuple across artifacts: {sorted(overlap)}")
            self.attestations.update(records)
        self.notion_oauth_supplied = set(notion_records) == expected_notion
        self.chatgpt_attestation_supplied = bool(chatgpt_records)
        self._preflight()

    @property
    def cli_available(self) -> bool:
        return bool(self.binary and self.binary.is_file() and os.access(self.binary, os.X_OK))

    def _load_consent(self, path: Path | None) -> tuple[dict[str, Any], str | None]:
        if path is None:
            return {}, None
        value = json.loads(path.read_text())
        common = (
            value.get("schema_version") == 1
            and value.get("purpose") == "stable-launch-e2e"
            and value.get("consent") is True
            and value.get("dedicated_identity") is True
            and value.get("operation_mode") in {"read-only", "synthetic"}
            and value.get("auth_origin") in {"fresh-dedicated-identity", "none"}
            and value.get("cleanup_outcome") in {"cleaned", "not-created"}
            and isinstance(value.get("pseudonymous_identity_id"), str)
            and IDENTITY_ID.fullmatch(value["pseudonymous_identity_id"])
            and isinstance(value.get("pseudonymous_workspace_id"), str)
            and IDENTITY_ID.fullmatch(value["pseudonymous_workspace_id"])
            and value.get("scenario_contract_digest") == sha256_file(SCENARIOS)
        )
        proof = value.get("no_real_project_proof")
        base_proof = {
            "real_project_accessed": False,
            "absolute_paths_exported": False,
            "credential_material_exported": False,
            "auth_copied": False,
        }
        proof_valid = isinstance(proof, dict) and proof == (
            {**base_proof, "enforcement": "systemd-positive-mount-allowlist-v1"}
            if self.mode == "enforced" else base_proof
        )
        if self.mode == "enforced":
            bound = self.challenge is None or (
                value.get("mode") == "enforced"
                and value.get("challenge") == self.challenge.get("value")
                and str(value.get("run_id")) == str(self.github_run_id)
                and str(value.get("run_attempt")) == str(self.github_run_attempt)
                and value.get("catalog_sha") == self.github_sha
                and value.get("disposable_project_status") == "disposed"
                and value.get("cleanup_outcome") == "cleaned"
            )
        else:
            bound = (
                value.get("mode") == "fixture-only"
                and value.get("challenge") is None
                and value.get("run_id") == "fixture-only"
                and value.get("run_attempt") == "0"
                and value.get("catalog_sha") is None
                and value.get("disposable_project_status") == "not-created"
                and value.get("operation_mode") == "synthetic"
                and value.get("auth_origin") == "none"
            )
        if not common or not proof_valid or not bound:
            raise ValueError("consent artifact does not authorize stable-launch disposable E2E")
        return value, sha256_file(path)

    def _preflight(self) -> None:
        if self.run_root is not None:
            if self.run_root.exists():
                expected_root_id = logical_root_id(
                    self.github_sha or "", str(self.github_run_id), str(self.github_run_attempt),
                )
                prepared = self.mode == "enforced" and self.challenge and self.challenge.get("root_id") == expected_root_id
                if not prepared or (self.run_root / "runs").exists() or (self.run_root / "evidence").exists():
                    raise ValueError("disposable run root must not already exist unless it is the authenticated prepared root")
            real_home = Path.home().resolve()
            repository = ROOT.resolve()
            if self.run_root == real_home or self.run_root == repository or self.run_root in real_home.parents or self.run_root in repository.parents or repository in self.run_root.parents:
                raise ValueError("refusing a real existing home/project path as disposable root")
            self.run_root.mkdir(parents=True, exist_ok=True)
        if self.mode == "enforced":
            missing = []
            if not self.cli_available: missing.append("exact executable binary")
            if not self.binary_digest: missing.append("binary checksum")
            if not self.expected_version: missing.append("binary version")
            if not self.release_manifest or not self.release_identity or not self.release_manifest_digest or not self.release_checksums_digest or not self.release_tag: missing.append("immutable GitHub release manifest/checksums identity")
            if not self.snapshot: missing.append("signed Directory fixture")
            if not self.challenge: missing.append("GitHub-bound random challenge")
            if not self.native_observations or not self.native_observations.is_dir(): missing.append("native macOS/Linux/Windows and Node 22 observations")
            if not self.attestations: missing.append("runtime/OAuth attestations")
            if not self.observer_bundle_digest: missing.append("signed observer bundle digest")
            if not self.copilot_executable or not self.copilot_metadata: missing.append("exact GitHub Copilot CLI executable and npm metadata")
            if not self.notion_oauth_supplied: missing.append("separate Notion OAuth artifact")
            if not self.chatgpt_attestation_supplied: missing.append("separate ChatGPT Cloudflare artifact")
            if not self.consent_digest: missing.append("consent artifact")
            if not self.run_root: missing.append("fresh disposable run root")
            if missing:
                raise ValueError("enforced launch gate missing required input: " + ", ".join(missing))
        if not self.consent_digest:
            raise ValueError("no evidence is emitted without an explicit consent artifact")
        if self.expected_version:
            parse_stable_version(self.expected_version)
        if self.binary_digest:
            if not DIGEST.fullmatch(self.binary_digest):
                raise ValueError("binary checksum must be lowercase sha256:<64 hex>")
            if self.cli_available and sha256_file(self.binary) != self.binary_digest:
                raise ValueError("binary checksum does not match exact executable")
        if self.mode == "enforced":
            config = read_production_config()
            if not valid_copilot_installation(
                self.copilot_executable, self.copilot_metadata_path,
                self.copilot_metadata, self.run_root, config,
            ):
                raise ValueError("GitHub Copilot CLI metadata, integrity, executable, or version proof is invalid")
            validate_release_manifest(
                self.release_manifest, repository=config["cli_release_repository"], tag=str(self.release_tag),
                tag_commit=str(self.release_manifest.get("commit", "")),
            )
            if self.expected_version != self.release_manifest.get("version"):
                raise ValueError("binary version must come from the immutable GitHub release manifest")
            if self.release_identity != {
                "repository": config["cli_release_repository"], "tag": self.release_tag,
                "tag_commit": self.release_manifest.get("commit"),
                "release_id": self.release_identity.get("release_id"), "immutable": True,
            } or not isinstance(self.release_identity.get("release_id"), int):
                raise ValueError("GitHub release identity is mutable or does not match repository/tag/commit")

    def fresh_sandbox(self, label: str) -> Path:
        if self.run_root is None:
            # Contract-only tests may request disposable roots without executing runtime.
            self.run_root = Path(tempfile.mkdtemp(prefix="uap-fixture-only-root-"))
        self._sandbox_counter += 1
        sandbox = self.run_root / "runs" / f"{self._sandbox_counter:04d}-{label}"
        if sandbox.exists():
            raise ValueError("disposable scenario root already exists")
        sandbox.mkdir(parents=True)
        return sandbox

    def tuple(self, *, product_id: str | None = None, digest: str | None = None, manifest_digest: str | None = None, distribution_id: str | None = None, distribution_kind: str | None = None, release_sequence: int | None = None, package_version: str | None = None, source_repository: str | None = None, source_revision: str | None = None, source_path: str | None = None, client_version: str | None = None, dependency: str | None = None) -> dict[str, Any]:
        return {
            "product_id": product_id,
            "tree_digest": digest,
            "manifest_digest": manifest_digest,
            "distribution_id": distribution_id,
            "distribution_kind": distribution_kind,
            "release_sequence": release_sequence,
            "package_version": package_version,
            "source_repository": source_repository,
            "source_revision": source_revision,
            "source_path": source_path,
            "snapshot_sequence": self.snapshot.get("sequence"),
            "snapshot_digest": self.snapshot_digest,
            "binary_digest": self.binary_digest,
            "dependency_identity": dependency,
            "installer_version": self.cli_version or self.expected_version,
            "adapter_version": self.cli_version,
            "client_version": client_version,
            "os": self.os_name,
            "architecture": self.architecture,
            "observed_at": self.observed_at,
        }

    def add(self, scenario: str, plugin: str | None, client: str | None, level: str, outcome: str, reason: str, *, tuple_value: dict[str, Any] | None = None, details: dict[str, Any] | None = None) -> None:
        if outcome not in OUTCOMES:
            raise ValueError(f"invalid outcome: {outcome}")
        identity = json.dumps([scenario, plugin, client, level], separators=(",", ":"))
        if any(row["scenario"] == scenario and row["plugin"] == plugin and row["client"] == client and row["level"] == level for row in self.rows):
            raise ValueError(f"duplicate evidence tuple: {scenario}/{plugin}/{client}/{level}")
        tuple_value = tuple_value or self.tuple()
        synthetic_version = str(tuple_value.get("client_version") or "").startswith(
            ("native-state-v1@", "native-observation-v1@")
        )
        basis = details.get("evidence_basis") if isinstance(details, dict) else None
        repository_basis = basis == "repository_owned_disposable_observer"
        untrusted_fixture_basis = basis == "fixture_materialization"
        if synthetic_version or untrusted_fixture_basis or repository_basis:
            tuple_value = {**tuple_value, "client_version": None}
        if outcome == "passed" and level in {"discovery", "runtime", "oauth"} and (synthetic_version or untrusted_fixture_basis or repository_basis):
            outcome = "inconclusive"
            reason = "repository/materialization evidence cannot establish native discovery, runtime, or a real client version"
        row = {
            "id": hashlib.sha256(identity.encode()).hexdigest()[:24],
            "scenario": scenario, "plugin": plugin, "client": client,
            "level": level, "outcome": outcome,
            "tuple": tuple_value, "reason": reason,
        }
        if details:
            row["details"] = details
        self.rows.append(row)

    def _load_attestations(self, path: Path | None, *, allow_external_pr: bool = False) -> dict[tuple[str, str, str], dict[str, Any]]:
        if path is None:
            return {}
        value = json.loads(path.read_text())
        if value.get("schema_version") != 1:
            raise ValueError("runtime attestation schema_version must be 1")
        if "external_pr_evidence" in value:
            if not allow_external_pr:
                raise ValueError("external PR evidence is allowed only in the primary signed runtime artifact")
            if not isinstance(value["external_pr_evidence"], dict):
                raise ValueError("external PR evidence must be an object")
            self.external_pr_evidence = value["external_pr_evidence"]
        records = value.get("attestations", [])
        result: dict[tuple[str, str, str], dict[str, Any]] = {}
        for record in records:
            self._reject_mutable_refs(record)
            key = (record["plugin"], record["client"], record["level"])
            if record["level"] not in {"runtime", "oauth"}:
                raise ValueError(f"external observer artifact contains unsupported level: {key}")
            if key in result:
                raise ValueError(f"duplicate attestation tuple: {key}")
            if record.get("outcome") not in OUTCOMES:
                raise ValueError(f"invalid attestation outcome: {key}")
            tuple_value = record.get("tuple", {})
            expected_scenario = "chatgpt_registered_binding" if record["client"] == "chatgpt" else "hero_5x3_runtime"
            privacy_fields = (
                "pseudonymous_identity_id", "pseudonymous_workspace_id", "dedicated_identity",
                "disposable_project_status", "operation_mode", "auth_origin", "cleanup_outcome",
                "no_real_project_proof",
            )
            if (
                not self.challenge
                or record.get("challenge") != self.challenge.get("value")
                or str(record.get("run_id")) != str(self.github_run_id)
                or str(record.get("run_attempt")) != str(self.github_run_attempt)
                or record.get("scenario_id") != expected_scenario
            ):
                raise ValueError(f"attestation is not challenge/run/scenario-bound: {key}")
            if hasattr(self, "release_manifest_digest"):
                expected_bindings = {
                    "release_manifest_digest": self.release_manifest_digest,
                    "release_checksums_digest": self.release_checksums_digest,
                    "directory_digest": self.snapshot_digest,
                    "scenario_contract_digest": sha256_file(SCENARIOS),
                }
            else:
                # Minimal in-memory verifier instances used by cross-consumer
                # contract tests still bind every value carried by the shared
                # challenge; normal LaunchHarness construction always takes
                # the stricter branch above, including checksums identity.
                expected_bindings = {
                    "release_manifest_digest": self.challenge.get("release_manifest_digest"),
                    "directory_digest": self.challenge.get("directory_digest"),
                    "scenario_contract_digest": self.challenge.get("scenario_contract_digest"),
                }
            if any(record.get(field) != expected for field, expected in expected_bindings.items()):
                raise ValueError(f"attestation is not release/Directory/scenario-bound: {key}")
            if record.get("consent_artifact_digest") != self.consent_digest or any(record.get(field) != self.consent.get(field) for field in privacy_fields):
                raise ValueError(f"attestation privacy fields differ from signed consent: {key}")
            if record.get("identity_id") != record.get("pseudonymous_identity_id"):
                raise ValueError(f"attestation identity is not the consented pseudonymous identity: {key}")
            if record.get("outcome") == "passed":
                try:
                    started = datetime.fromisoformat(record["started_at"].replace("Z", "+00:00"))
                    observed = datetime.fromisoformat(record["observed_at"].replace("Z", "+00:00"))
                except (KeyError, TypeError, ValueError) as error:
                    raise ValueError(f"passed attestation timestamps are invalid: {key}") from error
                now = datetime.now(timezone.utc)
                if started > observed or observed > now + timedelta(minutes=2) or now - observed > MAX_ATTESTATION_AGE:
                    raise ValueError(f"passed attestation is stale or has an invalid time interval: {key}")
                traces = record.get("command_traces")
                if not isinstance(traces, list) or not traces or not all(trace.get("challenge") == self.challenge["value"] and isinstance(trace.get("argv"), list) for trace in traces):
                    raise ValueError(f"passed attestation lacks challenge-bound command traces: {key}")
                for trace in traces:
                    try:
                        trace_started = datetime.fromisoformat(trace["started_at"].replace("Z", "+00:00"))
                        trace_ended = datetime.fromisoformat(trace["ended_at"].replace("Z", "+00:00"))
                    except (KeyError, TypeError, ValueError) as error:
                        raise ValueError(f"passed attestation has invalid command trace timestamps: {key}") from error
                    if trace_started < started or trace_started > trace_ended or trace_ended > observed or trace.get("exit_code") != 0:
                        raise ValueError(f"passed attestation command trace is outside its observation interval or failed: {key}")
                if not all(isinstance(record.get(field), str) and record[field] for field in ("client_id", "application_id", "endpoint")):
                    raise ValueError(f"passed attestation lacks exact client/app/endpoint identity: {key}")
                endpoint = urlsplit(record["endpoint"])
                if endpoint.scheme != "https" or not endpoint.hostname or endpoint.username or endpoint.password or endpoint.fragment:
                    raise ValueError(f"passed attestation has an invalid public HTTPS endpoint: {key}")
                github = record.get("github_attestation")
                github_valid = bool(
                    isinstance(github, dict)
                    and github.get("subject") == TRUSTED_OBSERVER_SUBJECT
                    and github.get("repository") == TRUSTED_CATALOG_REPOSITORY
                    and github.get("repository_owner") == TRUSTED_CATALOG_REPOSITORY_OWNER
                    and str(github.get("repository_id")) == TRUSTED_CATALOG_REPOSITORY_ID
                    and str(github.get("repository_owner_id")) == TRUSTED_CATALOG_REPOSITORY_OWNER_ID
                    and github.get("ref") == TRUSTED_OBSERVER_REF
                    and github.get("environment") == TRUSTED_OBSERVER_ENVIRONMENT
                    and github.get("workflow_ref") == TRUSTED_OBSERVER_WORKFLOW_REF
                    and github.get("job_workflow_ref") == TRUSTED_OBSERVER_JOB_WORKFLOW_REF
                    and github.get("sha") == self.github_sha
                    and str(github.get("run_id")) == str(self.github_run_id)
                    and str(github.get("run_attempt")) == str(self.github_run_attempt)
                    and github.get("challenge") == self.challenge["value"]
                    and github.get("workflow") == "launch-evidence-e2e.yml"
                    and github.get("job") == "protected-observer-inputs"
                )
                if not github_valid:
                    raise ValueError(f"passed external observation is not GitHub-attested for this run: {key}")
                required = ("product_id", "tree_digest", "manifest_digest", "distribution_id", "distribution_kind", "release_sequence", "package_version", "source_repository", "source_revision", "source_path", "snapshot_sequence", "snapshot_digest", "binary_digest", "dependency_identity", "installer_version", "adapter_version", "client_version", "os", "architecture", "observed_at")
                if any(not tuple_value.get(item) for item in required):
                    raise ValueError(f"passed attestation has incomplete tuple: {key}")
                for field in ("tree_digest", "manifest_digest", "snapshot_digest", "binary_digest"):
                    if not DIGEST.fullmatch(tuple_value[field]):
                        raise ValueError(f"passed attestation has invalid {field}: {key}")
                if tuple_value["installer_version"] != self.expected_version:
                    raise ValueError(f"attestation installer version does not match supplied binary: {key}")
                if tuple_value["binary_digest"] != self.binary_digest:
                    raise ValueError(f"attestation binary digest does not match supplied binary: {key}")
                if tuple_value["observed_at"] != record["observed_at"]:
                    raise ValueError(f"attestation tuple timestamp differs from observer timestamp: {key}")
                release = self.directory_release(record["plugin"], [record["client"]])
                expected_identity = {
                    "product_id": record["plugin"],
                    "distribution_id": release["distribution_id"],
                    "distribution_kind": release["distribution_kind"],
                    "release_sequence": release["release_sequence"],
                    "package_version": release["package_version"],
                    "tree_digest": release["tree_digest"],
                    "manifest_digest": release["manifest_digest"],
                    "source_repository": release["source_repository"],
                    "source_revision": release["source_revision"],
                    "source_path": release["source_path"],
                    "snapshot_sequence": self.snapshot.get("sequence"),
                    "snapshot_digest": self.snapshot_digest,
                }
                if any(tuple_value.get(field) != expected for field, expected in expected_identity.items()):
                    raise ValueError(f"attestation identity does not match signed Directory release: {key}")
                if record.get("consent_artifact_digest") != self.consent_digest:
                    raise ValueError(f"runtime pass lacks the supplied consent artifact: {key}")
                if record.get("runtime_invocation") is not True or record.get("discovery_verified") is not True:
                    raise ValueError(f"runtime pass lacks invocation/discovery proof: {key}")
                if record["client"] == "chatgpt":
                    if "native_discovery_evidence" in record or not authoritative_public_mcp_evidence(record.get("public_mcp_evidence")):
                        raise ValueError(f"ChatGPT runtime pass lacks authoritative public MCP evidence: {key}")
                else:
                    native_evidence = record.get("native_discovery_evidence")
                    if not authoritative_protected_runtime_evidence(
                        native_evidence, client_version=tuple_value.get("client_version"),
                        product_id=record["plugin"], client=record["client"],
                        challenge=self.challenge["value"],
                    ):
                        raise ValueError(f"runtime pass lacks authoritative exact-version client discovery evidence: {key}")
                for observation_name in ("manager_observation", "native_observation"):
                    observation = record.get(observation_name)
                    if not isinstance(observation, dict) or not all(isinstance(observation.get(field), str) and observation[field] for field in ("observer", "before_digest", "after_digest", "observed_at")):
                        raise ValueError(f"runtime pass lacks independent {observation_name}: {key}")
                    if not DIGEST.fullmatch(observation["before_digest"]) or not DIGEST.fullmatch(observation["after_digest"]) or observation["observed_at"] != record["observed_at"]:
                        raise ValueError(f"runtime pass has invalid independent {observation_name}: {key}")
                if record.get("receipt_reconciled") is not True or record.get("native_discovery_reconciled") is not True:
                    raise ValueError(f"runtime pass lacks receipt/native discovery reconciliation: {key}")
                if not isinstance(record.get("identity_id"), str) or not IDENTITY_ID.fullmatch(record["identity_id"]) or record.get("isolated_identity") is not True:
                    raise ValueError(f"runtime pass lacks explicit isolated test identity: {key}")
                if (record["plugin"] == "notion" or record["client"] == "chatgpt") and not (record.get("consent_attested") is True and record.get("isolated_identity") is True):
                    raise ValueError(f"OAuth pass lacks consent/isolated identity: {key}")
                if (record["plugin"] == "notion" or record["client"] == "chatgpt") and not record.get("identity_id"):
                    raise ValueError(f"OAuth pass lacks explicit test identity: {key}")
                if record["plugin"] == "notion" and record.get("oauth_artifact_approved") is not True:
                    raise ValueError(f"Notion runtime pass lacks approved OAuth artifact: {key}")
                if record["client"] == "chatgpt" and not all(record.get(field) is True for field in ("registered_app_binding", "ui_activation", "read_only")):
                    raise ValueError(f"ChatGPT pass lacks registered binding/UI/read-only proof: {key}")
                if record["client"] == "chatgpt" and not all(DIGEST.fullmatch(str(record.get(field, ""))) for field in ("projection_receipt_digest", "native_app_digest", "native_mcp_digest")):
                    raise ValueError(f"ChatGPT pass lacks exact projection receipt/app/MCP digests: {key}")
            result[key] = record
        return result

    @staticmethod
    def _reject_mutable_refs(value: Any) -> None:
        if isinstance(value, dict):
            for key, child in value.items():
                if key in {"revision", "source_revision", "commit_sha"} and child is not None and (not isinstance(child, str) or not FULL_SHA.fullmatch(child)):
                    raise ValueError("evidence contains a mutable or invalid source revision")
                if key == "ref" and not (
                    value.get("subject") == TRUSTED_OBSERVER_SUBJECT
                    and value.get("repository") == TRUSTED_CATALOG_REPOSITORY
                    and child == TRUSTED_OBSERVER_REF
                ):
                    raise ValueError("evidence must not contain mutable refs")
                LaunchHarness._reject_mutable_refs(child)
        elif isinstance(value, list):
            for child in value:
                LaunchHarness._reject_mutable_refs(child)

    def command(self, argv: list[str], sandbox: Path, clients: tuple[str, ...]) -> tuple[str, dict[str, Any] | None, str | None]:
        if not self.cli_available:
            return "inconclusive", None, "fixture-only non-runtime mode: Agent Plugins CLI binary was not supplied"
        env = isolated_environment(sandbox, clients, self.directory_environment)
        if "copilot" in clients:
            if not self.copilot_executable:
                return "failed", None, "exact protected GitHub Copilot CLI executable was not supplied"
            env["PATH"] = str(self.copilot_executable.parent) + os.pathsep + env.get("PATH", "")
        product_id = argv[1] if len(argv) > 1 else "unknown"
        before_state = observed_state_identity(env, product_id, clients)
        started_at = utc_now()
        try:
            completed = subprocess.run([str(self.binary), *argv], cwd=sandbox / "workspace", env=env, text=True, capture_output=True, timeout=180, check=False)
        except subprocess.TimeoutExpired:
            return "inconclusive", None, "isolated CLI command timed out"
        if completed.returncode:
            return "failed", None, f"CLI returned exit status {completed.returncode}"
        try:
            value = strict_json_loads(completed.stdout)
        except (json.JSONDecodeError, ValueError):
            return "failed", None, "CLI did not return structured JSON"
        if not validate_cli_envelope(value, argv[0]):
            return "failed", None, "CLI returned a malformed command envelope"
        if argv[0] in {"add", "update", "repair", "remove"}:
            observed_targets = [item.get("target") for item in value["data"]["targets"]]
            if observed_targets != list(clients):
                return "failed", None, "CLI target coverage did not exactly match the selected targets"
        self._assert_result_paths(value, sandbox)
        after_state = observed_state_identity(env, product_id, clients)
        value["_observed_state"] = after_state
        value["_launch_command_trace"] = {
            "challenge": self.challenge.get("value") if self.challenge else None,
            "argv": argv, "started_at": started_at, "ended_at": utc_now(),
            "exit_code": completed.returncode,
            "stdout_digest": "sha256:" + hashlib.sha256(completed.stdout.encode()).hexdigest(),
            "stderr_digest": "sha256:" + hashlib.sha256(completed.stderr.encode()).hexdigest(),
            "manager_before_digest": before_state["manager_digest"],
            "manager_after_digest": after_state["manager_digest"],
            "native_before_digests": before_state["native_digests"],
            "native_after_digests": after_state["native_digests"],
        }
        if type(value.get("schema_version")) is not int or value.get("schema_version") != 1 or value.get("command") != argv[0]:
            return "failed", value, "CLI returned an invalid command envelope"
        data = value.get("data", {})
        target_results = [item["output"]["result"] for item in data.get("targets", [])]
        mutated = None if not target_results else all(result.get("mutated") is True for result in target_results)
        if argv[0] == "update" and target_results:
            mutated = any(result.get("mutated") is True for result in target_results)
        if argv[0] in {"add", "repair", "remove"} and mutated is not True:
            return "failed", value, "CLI did not report a committed mutation"
        if argv[0] == "update" and (not isinstance(mutated, bool) or any(result.get("no_change") is not (not result["mutated"]) for result in target_results)):
            return "failed", value, "CLI did not report whether update mutated state"
        if argv[0] == "update" and mutated is False and (
            before_state["manager_digest"] != after_state["manager_digest"]
            or before_state["native_digests"] != after_state["native_digests"]
        ):
            return "failed", value, "CLI reported a no-op update but changed manager/client materialization"
        if self.cli_version is None:
            return "inconclusive", value, "CLI version could not be recorded for the evidence tuple"
        return "passed", value, "isolated CLI command completed"

    def driven_scenario(self, scenario: str) -> tuple[str, dict[str, Any] | None, str]:
        if not self.cli_available or not self.challenge:
            return "inconclusive", None, "fixture-only non-runtime mode: CLI/challenge was not supplied"
        sandbox = self.fresh_sandbox("driver-" + scenario)
        challenge_path = sandbox / "config" / "challenge.json"
        product_id = "context7"
        source_selection = self.config.get("source_identity_scenarios", {}).get(scenario)
        if source_selection:
            product_id = source_selection["product_id"]
        if scenario.startswith("hero_lifecycle_"):
            product_id = scenario.removeprefix("hero_lifecycle_").rsplit("_", 1)[0]
        targets: tuple[str, ...]
        if scenario == "context7_grouped_lifecycle":
            targets = tuple(self.config["context7_targets"])
        elif scenario == "shared_copilot_vscode_backend":
            targets = tuple(self.config["shared_backend_targets"])
        elif scenario.startswith("hero_lifecycle_"):
            targets = (scenario.rsplit("_", 1)[1],)
        elif scenario.startswith("repair_"):
            targets = (scenario.removeprefix("repair_"),)
        else:
            targets = ("cursor",)
        env = isolated_environment(sandbox, targets, self.directory_environment)
        if "copilot" in targets:
            if not self.copilot_executable:
                return "failed", None, "exact protected GitHub Copilot CLI executable was not supplied"
            env["PATH"] = str(self.copilot_executable.parent) + os.pathsep + env.get("PATH", "")
        release = self.configured_source_release(scenario, targets) if source_selection else self.directory_release(product_id, targets)
        source_identity = {
            "product_id": release["product_id"],
            "distribution_id": release["distribution_id"], "distribution_kind": release["distribution_kind"],
            "release_sequence": release["release_sequence"], "source_revision": release.get("source_revision"),
            "source_repository": release.get("source_repository"), "source_path": release.get("source_path"),
            "canonical_source": f'https://github.com/{release.get("source_repository")}@{release.get("source_revision")}//{release.get("source_path")}',
            "tree_digest": release["tree_digest"], "manifest_digest": release["manifest_digest"],
        }
        observer_context = {
            **self.challenge,
            "binary_digest": self.binary_digest,
            "expected_version": self.expected_version,
            "snapshot_sequence": self.snapshot.get("sequence"),
            "release": release,
            "catalog_repository": read_production_config()["catalog_repository"],
            "directory_product": next(item for item in self.snapshot["products"] if item["id"] == product_id),
            "directory_distribution": next(item for item in self.snapshot["distributions"] if item["id"] == release["distribution_id"]),
            "source_identity": source_identity,
        }
        challenge_path.write_text(json.dumps(observer_context, sort_keys=True))
        completed = subprocess.run([
            sys.executable, str(SCENARIO_OBSERVER), "--scenario", scenario,
            "--binary", str(self.binary), "--root", str(sandbox / "workspace"),
            "--challenge-context", str(challenge_path),
        ], cwd=sandbox / "workspace", env=env, text=True, capture_output=True, timeout=240, check=False)
        try:
            value = strict_json_loads(completed.stdout)
        except (json.JSONDecodeError, ValueError):
            return "failed", None, "repository-owned observer did not return JSON"
        outcome = value.get("outcome")
        if outcome not in OUTCOMES:
            return "failed", None, "repository-owned observer returned an invalid outcome"
        if value.get("scenario_id") != scenario or value.get("challenge") != self.challenge["value"]:
            return "failed", None, "repository-owned observer result is not challenge-correlated"
        traces = value.get("command_traces")
        if not isinstance(traces, list) or not traces or not all(trace.get("challenge") == self.challenge["value"] for trace in traces):
            return "failed", None, "repository-owned observer omitted challenge-bound command traces"
        now = datetime.now(timezone.utc)
        for trace in traces:
            try:
                started = datetime.fromisoformat(trace["started_at"].replace("Z", "+00:00"))
                ended = datetime.fromisoformat(trace["ended_at"].replace("Z", "+00:00"))
            except (KeyError, TypeError, ValueError):
                return "failed", None, "repository-owned observer returned an invalid trace timestamp"
            if started > ended or ended > now + timedelta(minutes=2) or now - ended > MAX_ATTESTATION_AGE:
                return "failed", None, "repository-owned observer returned a stale trace"
            if not isinstance(trace.get("argv"), list) or not trace["argv"] or not all(isinstance(argument, str) for argument in trace["argv"]):
                return "failed", None, "repository-owned observer returned an invalid command trace"
        if not all(key in value for key in ("before", "after", "started_at", "observed_at", "manager_observer", "native_observer")):
            return "failed", None, "repository-owned observer omitted independent before/after observations"
        self._assert_result_paths(value, sandbox)
        if completed.returncode not in (0, 2):
            return "failed", value, f"repository-owned observer returned exit status {completed.returncode}"
        return outcome, value, str(value.get("reason") or "repository-owned observation")

    @staticmethod
    def _assert_result_paths(value: Any, sandbox: Path) -> None:
        root = sandbox.resolve()
        if isinstance(value, dict):
            for key, child in value.items():
                if (key.endswith("_path") or key.endswith("_root")) and isinstance(child, str):
                    path = Path(child)
                    if path.is_absolute() and path.resolve() != root and root not in path.resolve().parents:
                        raise ValueError("scenario result path is outside the disposable root")
                LaunchHarness._assert_result_paths(child, sandbox)
        elif isinstance(value, list):
            for child in value:
                LaunchHarness._assert_result_paths(child, sandbox)

    def discover_version(self) -> None:
        if not self.cli_available:
            return
        sandbox = self.fresh_sandbox("version")
        env = isolated_environment(sandbox, ("cursor",), self.directory_environment)
        result = subprocess.run([str(self.binary), "version"], cwd=sandbox / "workspace", env=env, text=True, capture_output=True, timeout=30, check=False)
        if result.returncode == 0:
            self.cli_version = result.stdout.strip().removeprefix("agentplugins ").strip() or None
        if self.cli_version != self.expected_version:
            raise ValueError(f"binary version mismatch: expected {self.expected_version}, observed {self.cli_version}")
        parse_stable_version(self.cli_version)

    def validate_fixtures(self) -> None:
        state = json.loads(STATE_FIXTURE.read_text())
        recovery = json.loads(RECOVERY_FIXTURE.read_text())
        expected = set(self.config["fault_scenarios"]) - {"state_schema_2_migration"}
        actual = {item["id"] for item in recovery["cases"]}
        valid = (
            state.get("schema_version") == 2 and expected == actual
            and (EXTERNAL_PACKAGE / "plugin.json").is_file()
            and tuple(self.config.get("acceptance_postconditions", ())) == EXPECTED_ACCEPTANCE_SCENARIOS
            and self.config.get("expected_counts") == EXPECTED_COUNTS
        )
        self.add("fixture_contracts", None, None, "harness", "passed" if valid else "failed", "scenario fixtures are structurally complete" if valid else "scenario fixture mismatch", details={"fault_case_count": len(actual), "external_package_digest": package_digest(EXTERNAL_PACKAGE)})

    def directory_release(self, product_id: str, targets: list[str] | tuple[str, ...]) -> dict[str, Any]:
        if not targets:
            raise ValueError(f"authoritative Directory resolution requires targets for {product_id}")
        products = {item["id"]: item for item in self.snapshot.get("products", [])}
        distributions = {item["id"]: item for item in self.snapshot.get("distributions", [])}
        product = products.get(product_id)
        if not product:
            raise ValueError(f"signed Directory snapshot lacks product {product_id}")
        try:
            resolved = resolve_directory(self.snapshot, product_id, list(targets))
        except RegistryError as error:
            raise ValueError(f"authoritative Directory resolution failed for {product_id}: {error}") from error
        distribution = distributions.get(resolved["distribution_id"])
        if not distribution:
            raise ValueError(f"authoritative Directory resolver selected a missing distribution for {product_id}")
        policies = {item["release_sequence"]: item for item in distribution.get("release_policies", [])}
        release = next((item for item in distribution.get("releases", []) if item["sequence"] == resolved["release_sequence"]), None)
        if not release:
            raise ValueError(f"authoritative Directory resolver selected a missing release for {product_id}")
        policy = policies[release["sequence"]]
        clients = sorted({target["client"] for target in policy.get("targets", []) if "user" in target.get("scopes", [])})
        source = release.get("package_source", {})
        return {"product_id": product_id, "distribution_id": distribution["id"], "distribution_kind": distribution["kind"], "release_sequence": release["sequence"], "package_version": release.get("package_version"), "tree_digest": release["tree_digest"], "manifest_digest": release["manifest_digest"], "source_repository": source.get("repository"), "source_revision": source.get("revision"), "source_path": source.get("path"), "compatible_clients": clients, "resolved_targets": list(targets), "fallback_reason": resolved["fallback_reason"]}

    def configured_source_release(self, scenario: str, targets: list[str] | tuple[str, ...]) -> dict[str, Any]:
        selection = self.config["source_identity_scenarios"][scenario]
        product_id, distribution_id = selection["product_id"], selection["distribution_id"]
        product = next((item for item in self.snapshot.get("products", []) if item["id"] == product_id), None)
        distribution = next((item for item in self.snapshot.get("distributions", []) if item["id"] == distribution_id), None)
        if not product or not distribution or distribution_id not in product.get("distributions", []):
            raise ValueError(f"source scenario {scenario} is not a reviewed Directory selection")
        policy = next((item for item in distribution.get("release_policies", []) if item.get("status") == "active" and all(any(target.get("client") == client and "user" in target.get("scopes", []) for target in item.get("targets", [])) for client in targets)), None)
        release = next((item for item in distribution.get("releases", []) if policy and item["sequence"] == policy["release_sequence"]), None)
        source = release.get("package_source", {}) if release else {}
        revision = source.get("revision")
        expected_kind = selection["distribution_kind"]
        if not release or distribution.get("kind") != expected_kind or not FULL_SHA.fullmatch(str(revision)):
            raise ValueError(f"source scenario {scenario} lacks an immutable reviewed {expected_kind} release")
        clients = sorted(target["client"] for target in policy["targets"] if "user" in target.get("scopes", []))
        return {"product_id": product_id, "distribution_id": distribution_id, "distribution_kind": expected_kind, "release_sequence": release["sequence"], "package_version": release.get("package_version"), "tree_digest": release["tree_digest"], "manifest_digest": release["manifest_digest"], "source_repository": source.get("repository"), "source_revision": revision, "source_path": source.get("path"), "compatible_clients": clients, "resolved_targets": list(targets), "fallback_reason": None}

    @staticmethod
    def source_identity_matches_release(release: dict[str, Any], observed: Any) -> bool:
        fields = (
            "product_id", "distribution_id", "distribution_kind", "release_sequence",
            "tree_digest", "manifest_digest", "source_repository", "source_revision", "source_path",
            "canonical_source",
        )
        if not isinstance(observed, dict) or set(observed) != set(fields):
            return False
        canonical = parse_canonical_github_source(observed["canonical_source"])
        return bool(
            canonical
            and all(canonical[field] == observed[field] for field in ("source_repository", "source_revision", "source_path"))
            and all(observed[field] == release[field] for field in fields if field != "canonical_source")
        )

    def evidence_tuple(self, product_id: str, targets: list[str] | tuple[str, ...], *, client_version: str | None, dependency: str) -> dict[str, Any]:
        release = self.directory_release(product_id, targets)
        return self.tuple(
            product_id=product_id,
            digest=release["tree_digest"], manifest_digest=release["manifest_digest"],
            distribution_id=release["distribution_id"], distribution_kind=release["distribution_kind"],
            release_sequence=release["release_sequence"], package_version=release["package_version"],
            source_repository=release["source_repository"], source_revision=release["source_revision"],
            source_path=release["source_path"],
            client_version=client_version, dependency=dependency,
        )

    def tuple_matches_release(self, product_id: str, targets: list[str] | tuple[str, ...], value: dict[str, Any] | None) -> bool:
        if not value:
            return False
        expected = self.evidence_tuple(product_id, targets, client_version=value.get("client_version"), dependency=value.get("dependency_identity"))
        identity_fields = ("product_id", "tree_digest", "manifest_digest", "distribution_id", "distribution_kind", "release_sequence", "package_version", "source_repository", "source_revision", "source_path", "snapshot_sequence", "snapshot_digest", "binary_digest", "installer_version")
        return all(value.get(field) == expected.get(field) for field in identity_fields)

    def command_matches_release(self, product_id: str, targets: list[str] | tuple[str, ...], value: dict[str, Any] | None) -> bool:
        if not value:
            return False
        release = self.directory_release(product_id, targets)
        canonical = parse_canonical_github_source(find_value(value, {"canonical_source"}))
        expected = {
            "product_id": product_id,
            "distribution_id": release["distribution_id"],
            "distribution_kind": release["distribution_kind"],
            "release_sequence": release["release_sequence"],
            "tree_digest": release["tree_digest"],
            "manifest_digest": release["manifest_digest"],
            "source_repository": release["source_repository"],
            "source_revision": release["source_revision"],
            "source_path": release["source_path"],
            "snapshot_sequence": self.snapshot.get("sequence"),
            "snapshot_digest": self.snapshot_digest,
        }
        observed = {
            "product_id": find_value(value, {"product_id"}),
            "distribution_id": find_value(value, {"distribution_id"}),
            "distribution_kind": find_value(value, {"distribution_kind"}),
            "release_sequence": find_value(value, {"release_sequence"}),
            "tree_digest": find_value(value, {"tree_digest", "package_digest"}),
            "manifest_digest": find_value(value, {"manifest_digest"}),
            "source_repository": canonical.get("source_repository") if canonical else None,
            "source_revision": canonical.get("source_revision") if canonical else None,
            "source_path": canonical.get("source_path") if canonical else None,
            "snapshot_sequence": find_value(value, {"snapshot_sequence"}),
            "snapshot_digest": find_value(value, {"snapshot_digest"}),
        }
        return observed == expected

    @staticmethod
    def info_reconciled(value: dict[str, Any] | None, client: str | None = None) -> bool:
        native_evidence = find_value(value, {"native_discovery_evidence"}) if value else None
        client_version = find_value(value, {"client_version"}) if value else None
        return bool(
            value
            and find_value(value, {"receipt_reconciled"}) is True
            and find_value(value, {"native_discovery_reconciled"}) is True
            and authoritative_native_client_evidence(native_evidence, client_version=client_version, client=client)
        )

    def all_package_client(self, plugin: str) -> tuple[str, dict[str, Any]]:
        client = self.config["all_package_client"]
        if client != "copilot":
            raise ValueError("all-package launch proof must use GitHub Copilot CLI")
        try:
            return client, self.directory_release(plugin, [client])
        except ValueError as error:
            raise ValueError(f"signed Directory release does not support Copilot lifecycle for {plugin}") from error

    def all_package_matrix(self) -> None:
        names = [item["id"] for item in self.snapshot.get("products", [])]
        if len(names) != 26 or len(set(names)) != 26:
            raise RuntimeError(f"signed launch Directory must contain 26 unique products, found {len(set(names))}")
        for plugin in names:
            sandbox = self.fresh_sandbox("package-" + plugin)
            client, release = self.all_package_client(plugin)
            resolved_digest: str | None = None
            for operation in self.config["all_package_operations"]:
                outcome, value, reason = self.command([operation, plugin, "--target", client, "--format", "json"], sandbox, (client,))
                digest = find_value(value, {"package_digest", "tree_digest"}) if value else None
                if digest is not None and not DIGEST.fullmatch(str(digest)):
                    outcome, reason = "failed", "CLI returned an invalid package digest"
                    digest = None
                if isinstance(digest, str):
                    resolved_digest = digest
                if outcome == "passed" and resolved_digest is None:
                    outcome, reason = "inconclusive", "CLI output did not expose the immutable package digest"
                if outcome == "passed" and not self.command_matches_release(plugin, [client], value):
                    outcome, reason = "failed", "CLI result identity does not match the signed Directory release"
                tuple_value = self.evidence_tuple(plugin, [client], client_version=None, dependency=f"signed-directory@{self.snapshot_digest}")
                details = {
                    "evidence_basis": "repository_owned_disposable_observer",
                    "runtime_proof": False, "native_discovery_proof": False,
                    "operation": operation, **release,
                    "receipt_reconciliation_required": operation == "info",
                    "command_trace": value.get("_launch_command_trace") if value else None,
                }
                if outcome == "passed" and operation == "info":
                    config = read_production_config()
                    client_version = find_value(value, {"client_version"}) if value else None
                    native_evidence = find_value(value, {"native_discovery_evidence"}) if value else None
                    if (
                        find_value(value, {"receipt_reconciled"}) is not True
                        or find_value(value, {"native_discovery_reconciled"}) is not True
                        or find_value(value, {"native_identity_state"}) != "managed"
                        or not authoritative_repository_copilot_evidence(
                            native_evidence, client_version=client_version, product_id=plugin,
                            physical_artifact_id=str(value.get("_observed_state", {}).get("physical_artifact_id") or ""),
                            expected_version=config["copilot_cli_version"],
                        )
                    ):
                        outcome, reason = "failed", "Agent Plugins info omitted exact receipt and native Copilot discovery/version reconciliation"
                    else:
                        dependency = f"{config['copilot_cli_package']}@{config['copilot_cli_version']}#{config['copilot_cli_integrity']}"
                        tuple_value = self.evidence_tuple(plugin, [client], client_version=client_version, dependency=dependency)
                        details = {
                            "evidence_basis": "native_client_command", "runtime_proof": False,
                            "native_discovery_proof": True, "native_discovery_evidence": native_evidence,
                            "physical_artifact_id": value.get("_observed_state", {}).get("physical_artifact_id"),
                            "native_identity_state": "managed",
                            "copilot_package": self.copilot_metadata, "operation": operation,
                            "resolution": release, "command_trace": value.get("_launch_command_trace"),
                        }
                        reason = "repository-owned Agent Plugins process reconciled exact native Copilot discovery"
                if outcome == "passed" and resolved_digest != release["tree_digest"]:
                    outcome, reason = "failed", "CLI package digest does not match the signed Directory release"
                self.add(f"all_26_{operation}", plugin, client, "discovery" if operation == "info" else "materialization", outcome, reason or "unknown result", tuple_value=tuple_value, details=details)

    def context7_multi_target(self) -> None:
        targets = tuple(self.config["context7_targets"])
        target_arg = ",".join(targets)
        release = self.directory_release("context7", targets)
        expected_digest = release["tree_digest"]
        driver_outcome, value, driver_reason = self.driven_scenario("context7_grouped_lifecycle")
        expected_commands = [[operation, "context7", "--target", target_arg, "--format", "json"] for operation in self.config["context7_lifecycle"]]
        valid = bool(value) and value.get("commands") == expected_commands
        valid = valid and value.get("acquisition_digests") == [expected_digest]
        acquisition = complete_acquisition_proof(
            value.get("acquisition") if value else None, targets,
            tree_digest=expected_digest, manifest_digest=release["manifest_digest"],
            source_repository=release["source_repository"], source_revision=release["source_revision"],
            source_path=release["source_path"], source_kind="directory",
        )
        valid = valid and acquisition is not None
        valid = valid and set(value.get("target_outcomes", {})) == set(targets)
        valid = valid and all(value["target_outcomes"][target] == "passed" for target in targets)
        valid = valid and self.tuple_matches_release("context7", targets, value.get("tuple") if value else None)
        operation_outcomes = value.get("operation_outcomes", {}) if value else {}
        for operation in self.config["context7_lifecycle"]:
            outcome = operation_outcomes.get(operation, driver_outcome)
            reason = driver_reason
            if driver_outcome != "passed" or not valid or outcome != "passed":
                outcome = "failed" if driver_outcome == "passed" else driver_outcome
                reason = "grouped driver did not prove one acquisition, exact commands, and three outcomes" if driver_outcome == "passed" else driver_reason
            self.add(
                f"context7_three_target_{operation}", "context7", target_arg, "materialization", outcome, reason,
                tuple_value=value.get("tuple") if value else self.evidence_tuple("context7", targets, client_version=None, dependency="single-acquisition"),
                details={"evidence_basis": "repository_owned_disposable_observer", "runtime_proof": False, "native_discovery_proof": False, "operation": operation, "target_argument": target_arg, "single_process_invocation": True, "reported_target_count": len(value.get("target_outcomes", {})) if value else 0, "acquisition": acquisition, "command_traces": value.get("command_traces", []) if value else [], "operation_observations": value.get("operation_observations", []) if value else [], "resolution": release},
            )

    @staticmethod
    def runtime_attestation_details(record: dict[str, Any]) -> dict[str, Any]:
        fields = (
            "challenge", "started_at", "observed_at", "consent_artifact_digest",
            "consent_attested", "isolated_identity", "identity_id", "client_id",
            "application_id", "endpoint", "command_traces", "github_attestation",
            "runtime_invocation", "discovery_verified", "manager_observation",
            "native_observation", "receipt_reconciled", "native_discovery_reconciled",
            "projection_receipt_digest", "native_app_digest", "native_mcp_digest",
            "oauth_artifact_approved", "registered_app_binding", "ui_activation",
            "read_only", "scenario_id", "run_id", "run_attempt", "pseudonymous_identity_id",
            "pseudonymous_workspace_id", "dedicated_identity", "disposable_project_status",
            "operation_mode", "auth_origin", "cleanup_outcome", "no_real_project_proof",
            "native_discovery_evidence",
            "public_mcp_evidence", "release_manifest_digest", "release_checksums_digest",
            "directory_digest", "scenario_contract_digest",
        )
        return {
            **{field: record[field] for field in fields if field in record},
            "evidence_basis": "protected_external_observer",
            "runtime_proof": record.get("level") in {"runtime", "oauth"},
            "native_discovery_proof": record.get("client") != "chatgpt",
            **({"public_mcp_proof": True} if record.get("client") == "chatgpt" else {}),
        }

    def hero_runtime_matrix(self) -> None:
        for plugin in self.config["heroes"]:
            for client in self.config["runtime_clients"]:
                release = self.directory_release(plugin, [client])
                record = self.attestations.get((plugin, client, "runtime"))
                if record:
                    details = self.runtime_attestation_details(record)
                    details["resolution"] = release
                    self.add("hero_5x3_runtime", plugin, client, "runtime", record["outcome"], record.get("reason", "explicit runtime attestation"), tuple_value=record.get("tuple"), details=details)
                else:
                    reason = "runtime client/isolated identity attestation was not supplied" if plugin == "notion" else "client runtime attestation was not supplied"
                    self.add("hero_5x3_runtime", plugin, client, "runtime", "failed", reason, tuple_value=self.evidence_tuple(plugin, [client], client_version=None, dependency=f"signed-directory@{self.snapshot_digest}"), details={"resolution": release})
        chatgpt_release = self.directory_release("cloudflare-docs", ["chatgpt"])
        chatgpt = self.attestations.get(("cloudflare-docs", "chatgpt", "runtime"))
        if chatgpt:
            details = self.runtime_attestation_details(chatgpt)
            details["resolution"] = chatgpt_release
            self.add("chatgpt_registered_binding", "cloudflare-docs", "chatgpt", "runtime", chatgpt["outcome"], chatgpt.get("reason", "explicit registered-binding/UI/public-MCP runtime attestation"), tuple_value=chatgpt.get("tuple"), details=details)
        else:
            self.add("chatgpt_registered_binding", "cloudflare-docs", "chatgpt", "runtime", "failed", "registered app binding, human UI, and public MCP runtime attestation were not supplied", tuple_value=self.evidence_tuple("cloudflare-docs", ["chatgpt"], client_version=None, dependency=f"signed-directory@{self.snapshot_digest}"), details={"resolution": chatgpt_release})

    def hero_lifecycle_matrix(self) -> None:
        for plugin in self.config["heroes"]:
            for client in self.config["runtime_clients"]:
                release = self.directory_release(plugin, [client])
                expected_digest = release["tree_digest"]
                outcome, value, reason = self.driven_scenario(f"hero_lifecycle_{plugin}_{client}")
                required_operations = {"add", "update", "info", "remove"}
                operation_outcomes = value.get("operation_outcomes", {}) if value else {}
                tuple_value = value.get("tuple") if value else None
                valid = set(operation_outcomes) == required_operations and all(result == "passed" for result in operation_outcomes.values())
                valid = valid and tuple_value is not None and tuple_value.get("tree_digest") == expected_digest
                valid = valid and self.tuple_matches_release(plugin, [client], tuple_value)
                if outcome == "passed" and not valid:
                    outcome, reason = "failed", "hero driver omitted exact disposable add/update/info/remove lifecycle proof"
                self.add("hero_5x3_lifecycle", plugin, client, "materialization", outcome, reason, tuple_value=tuple_value or self.evidence_tuple(plugin, [client], client_version=None, dependency=f"signed-directory@{self.snapshot_digest}"), details={"evidence_basis": "repository_owned_disposable_observer", "runtime_proof": False, "native_discovery_proof": False, "operations": sorted(required_operations), "operation_outcomes": operation_outcomes, "command_traces": value.get("command_traces", []) if value else [], "resolution": release})

    def shared_backend(self) -> None:
        targets = tuple(self.config["shared_backend_targets"])
        outcome, value, reason = self.driven_scenario("shared_copilot_vscode_backend")
        valid = bool(value) and value.get("affected_surfaces") == list(targets)
        valid = valid and value.get("physical_mutations") == {"add": 1, "remove": 1}
        release = self.directory_release("context7", targets)
        valid = valid and self.tuple_matches_release("context7", targets, value.get("tuple") if value else None)
        if outcome == "passed" and not valid:
            outcome, reason = "failed", "shared-backend driver did not prove one add/remove mutation affecting both surfaces"
        self.add("shared_copilot_vscode_backend", "context7", "copilot,vscode", "materialization", outcome, reason, tuple_value=value.get("tuple") if value else self.evidence_tuple("context7", targets, client_version=None, dependency="copilot-shared-backend"), details={"evidence_basis": "repository_owned_disposable_observer", "runtime_proof": False, "native_discovery_proof": False, "expected_physical_mutations_per_operation": 1, "operations": ["add", "remove"], "command_traces": value.get("command_traces", []) if value else [], "operation_observations": value.get("operation_observations", []) if value else [], "resolution": release})

    def fault_matrix(self) -> None:
        for scenario in (*self.config["fault_scenarios"], *self.config["adapter_repair_faults"], *self.config["advanced_scenarios"]):
            outcome, value, reason = self.driven_scenario(scenario)
            if outcome == "passed" and not self.driver_proof_valid(scenario, value):
                outcome, reason = "failed", "scenario driver omitted the required exact proof fields"
            tuple_value = value.get("tuple") if value else None
            client = scenario.removeprefix("repair_") if scenario.startswith("repair_") else "cursor"
            source_selection = self.config.get("source_identity_scenarios", {}).get(scenario)
            product = source_selection["product_id"] if source_selection else "context7"
            release = self.configured_source_release(scenario, [client]) if source_selection else self.directory_release(product, [client])
            if source_selection:
                observed = value.get("source_identity") if value else None
                exact_source_identity = self.source_identity_matches_release(release, observed)
                # Source rows are always derived from the manager-state identity;
                # never retain a tuple supplied by the observer or fill an absent
                # source authority field from the expected Directory release.
                tuple_value = None
                if outcome == "passed" and not exact_source_identity:
                    outcome, reason = "failed", "scenario observer source identity differs from the exact signed Directory tuple"
                if outcome == "passed":
                    observed_client_version = find_value(value, {"client_version"}) if value else None
                    tuple_value = self.tuple(product_id=observed["product_id"], digest=observed["tree_digest"], manifest_digest=observed["manifest_digest"], distribution_id=observed["distribution_id"], distribution_kind=observed["distribution_kind"], release_sequence=observed["release_sequence"], package_version=release["package_version"], source_repository=observed["source_repository"], source_revision=observed["source_revision"], source_path=observed["source_path"], client_version=observed_client_version if isinstance(observed_client_version, str) else None, dependency=f"signed-directory-source@{observed['source_revision']}")
            if outcome == "passed" and not source_selection and tuple_value is not None and (not isinstance(tuple_value, dict) or not self.tuple_matches_release(product, [client], tuple_value)):
                outcome, reason = "failed", "scenario observer tuple differs from authoritative target-aware resolution"
            if tuple_value is None:
                observed_client_version = find_value(value, {"client_version"}) if value else None
                tuple_value = None if source_selection else self.evidence_tuple(product, [client], client_version=observed_client_version if isinstance(observed_client_version, str) else None, dependency="repository-owned-fault-observer")
            details = {"evidence_basis": "repository_owned_disposable_observer", "runtime_proof": False, "native_discovery_proof": False, "fixture_contract_present": scenario in self.config["fault_scenarios"], "repository_observer_required": True, "proof": value.get("proof", {}) if value else {}, "command_traces": value.get("command_traces", []) if value else [], "validator_artifact": value.get("validator_artifact") if value else None, "source_identity": value.get("source_identity") if value else None, "resolution": release}
            self.add(scenario, product, client, "materialization", outcome, reason, tuple_value=tuple_value, details=details)

    def acceptance_postconditions(self) -> None:
        required: dict[str, dict[str, Any]] = {
            "retained_data_readd_before_changed_default": {"data_retained_found_before_resolution": True, "changed_default_ignored": True},
            "schema_1_0_0_accepted": {"exact_schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json", "accepted": True},
            "schema_draft_rejected": {"draft_rejected": True, "zero_mutation": True},
            "schema_unknown_rejected": {"unknown_rejected": True, "zero_mutation": True},
            "project_scope_zero_mutation": {"project_scope_rejected": True, "manager_unchanged": True, "native_unchanged": True},
            "direct_full_sha_immutable": {"full_sha": True, "network_refetch_unchanged": True, "mutable_ref_followed": False},
            "public_help_no_hidden_yes": {"help_exit_zero": True, "hidden_yes_absent": True, "mutating_commands_rejected": True, "unknown_option_reported": True, "manager_unchanged": True, "native_unchanged": True},
            "revoked_operations_boundary": {"install_blocked": True, "new_target_blocked": True, "repair_blocked": True, "remove_available": True, "safe_update_available": True},
            "readd_sticky_distribution": {"recorded_distribution_retained": True, "recorded_revision_retained": True},
            "repair_sticky_distribution": {"recorded_distribution_retained": True, "recorded_revision_retained": True},
            "missing_runtime_exact_guidance": {"zero_mutation": True, "dependency_installed": False, "guidance_exact": True},
            "plugin_data_lifecycle_boundary": {"update_preserved": True, "repair_preserved": True, "switch_preserved": True, "remove_preserved": True, "explicit_owned_purge_deleted": True},
            "signed_sequence_not_semver": {"higher_sequence_selected": True, "semver_order_ignored": True},
        }
        for scenario in EXPECTED_ACCEPTANCE_SCENARIOS:
            outcome, value, reason = self.driven_scenario(scenario)
            proof = value.get("proof", {}) if value else {}
            observations_present = bool(value and isinstance(value.get("before"), dict) and isinstance(value.get("after"), dict))
            if outcome == "passed" and (not observations_present or any(proof.get(key) != expected for key, expected in required[scenario].items())):
                outcome, reason = "failed", "repository-owned observer omitted the exact independent postcondition"
            self.add(
                scenario, "context7", "cursor", "materialization", outcome, reason,
                tuple_value=self.evidence_tuple("context7", ["cursor"], client_version=find_value(value, {"client_version"}) if value else None, dependency="repository-owned-observer"),
                details={"evidence_basis": "repository_owned_disposable_observer", "runtime_proof": False, "native_discovery_proof": False, "expected_postcondition_id": scenario, "command_traces": value.get("command_traces", []) if value else [], "manager_and_native_before_after": observations_present, "resolution": self.directory_release("context7", ["cursor"])},
            )

    def native_platform_matrix(self) -> None:
        expected = {
            ("binary", "macos", "arm64"), ("binary", "macos", "amd64"),
            ("binary", "linux", "arm64"), ("binary", "linux", "amd64"),
            ("binary", "windows", "amd64"), ("binary", "windows", "arm64"),
            ("npm", "any", "any"),
        }
        records: dict[tuple[str, str, str], dict[str, Any]] = {}
        for path in sorted(self.native_observations.rglob("*.json")):
            value = json.loads(path.read_text())
            if value.get("schema_version") != 1:
                continue
            kind = value.get("kind", "npm" if value.get("node_major") == 22 else "binary")
            key = (kind, value.get("os", "any"), value.get("architecture", "any"))
            if key in expected:
                if key in records:
                    raise ValueError(f"duplicate native platform observation: {key}")
                records[key] = value
        if set(records) != expected:
            raise ValueError(f"native observations differ from immutable platform set: {sorted(set(records))}")
        for kind, os_name, architecture in sorted(expected):
            value = records[(kind, os_name, architecture)]
            challenge_context = value.get("challenge_context")
            trace = value.get("command_trace", {})
            declared = next((item for item in self.release_manifest.get("assets", {}).values() if item.get("file") == value.get("asset_name")), None) if kind == "binary" else None
            npm_package = value.get("npm_package", {}) if kind == "npm" else {}
            expected_npm_tarball = f"https://registry.npmjs.org/universal-agent-plugins/-/universal-agent-plugins-{self.expected_version}.tgz"
            distribution_valid = (
                declared is not None
                and value.get("asset_digest") == "sha256:" + declared["sha256"]
                and value.get("asset_name") == f"agentplugins_{self.expected_version}_{'darwin' if os_name == 'macos' else os_name}_{architecture}{'.exe' if os_name == 'windows' else ''}"
            ) if kind == "binary" else (
                (npm_native := next((item for item in self.release_manifest.get("assets", {}).values() if item.get("file") == value.get("asset_name")), None)) is not None
                and value.get("asset_digest") == "sha256:" + npm_native["sha256"]
                and npm_package.get("name") == "universal-agent-plugins"
                and npm_package.get("version") == self.expected_version
                and npm_package.get("tarball") == expected_npm_tarball
                and isinstance(npm_package.get("integrity"), str)
                and re.fullmatch(r"sha512-[A-Za-z0-9+/]+={0,2}", npm_package["integrity"]) is not None
                and DIGEST.fullmatch(str(npm_package.get("metadata_digest", ""))) is not None
                and npm_package.get("provenance_url") == f"https://registry.npmjs.org/-/npm/v1/attestations/universal-agent-plugins@{self.expected_version}"
                and npm_package.get("provenance_predicate_type") == "https://slsa.dev/provenance/v1"
                and npm_package.get("native_asset_name") == value.get("asset_name")
                and npm_package.get("native_asset_digest") == value.get("asset_digest")
                and npm_package.get("installed_executable_digest") == value.get("asset_digest")
                and npm_package.get("provenance_verified") is True
                and DIGEST.fullmatch(str(npm_package.get("provenance_output_digest", ""))) is not None
            )
            attested_asset = value.get("github_asset_attestation", {})
            release_identity_valid = value.get("github_release_identity") == self.release_identity
            attestation_valid = strict_asset_attestation_matches(
                attested_asset, repository=TRUSTED_CLI_RELEASE_REPOSITORY,
                workflow=TRUSTED_CLI_RELEASE_WORKFLOW, tag=self.release_tag,
                commit=self.release_manifest.get("commit"), asset_name=value.get("asset_name"),
                asset_digest=value.get("asset_digest"),
            )
            valid = (
                value.get("catalog_repository") == read_production_config()["catalog_repository"]
                and value.get("catalog_sha") == self.github_sha
                and value.get("cli_release_repository") == read_production_config()["cli_release_repository"]
                and value.get("cli_release_tag") == self.release_tag
                and value.get("release_manifest_digest") == self.release_manifest_digest
                and value.get("release_checksums_digest") == self.release_checksums_digest
                and value.get("directory_digest") == self.snapshot_digest
                and value.get("executed") is True
                and value.get("version") == self.expected_version
                and (kind != "npm" or value.get("node_major") == 22)
                and challenge_context_valid(challenge_context)
                and challenge_context.get("github_sha") == self.github_sha
                and str(challenge_context.get("run_id")) == str(self.github_run_id)
                and str(challenge_context.get("run_attempt", "")).isdigit()
                and str(self.github_run_attempt or "").isdigit()
                and 1 <= int(challenge_context["run_attempt"]) <= int(self.github_run_attempt)
                and challenge_context.get("root_id") == logical_root_id(
                    str(self.github_sha), str(self.github_run_id), str(challenge_context["run_attempt"]),
                )
                and challenge_context.get("release_manifest_digest") == self.release_manifest_digest
                and challenge_context.get("directory_digest") == self.snapshot_digest
                and challenge_context.get("scenario_contract_digest") == sha256_file(SCENARIOS)
                and value.get("challenge") == challenge_context.get("value")
                and trace.get("challenge") == value.get("challenge") and trace.get("exit_code") == 0
                and trace.get("argv") == ["agentplugins", "version"]
                and fresh_observation_interval(value.get("started_at"), value.get("observed_at"))
                and distribution_valid
                and attestation_valid
                and release_identity_valid
            )
            self.add(
                f"native_{kind}_{os_name}_{architecture}", None, f"{os_name}/{architecture}", "harness",
                "passed" if valid else "failed", "manifest-bound native execution observed" if valid else "native execution observation is incomplete or bound to another manifest",
                details={"kind": kind, "os": os_name, "architecture": architecture, "release_manifest_digest": value.get("release_manifest_digest"), "release_checksums_digest": value.get("release_checksums_digest"), "asset_name": value.get("asset_name"), "asset_digest": value.get("asset_digest"), "github_asset_attestation": attested_asset, "challenge": value.get("challenge"), "producer_run_attempt": str(challenge_context.get("run_attempt")) if isinstance(challenge_context, dict) else None, "command_trace": trace, "started_at": value.get("started_at"), "observed_at": value.get("observed_at")},
            )

    @staticmethod
    def driver_proof_valid(scenario: str, value: dict[str, Any] | None) -> bool:
        if not value:
            return False
        expected: dict[str, dict[str, Any]] = {
            "state_schema_2_migration": {"migration_applied": True, "provenance_preserved": True, "backup_verified": True},
            "crash_journal_recovery": {"crash_injected": True, "journal_recovered": True, "ownership_reconciled": True},
            "directory_offline": {"offline_cache_used": True, "signature_verified": True},
            "directory_expired": {"expired_snapshot_rejected": True, "zero_mutation": True},
            "directory_tampered": {"tampered_snapshot_rejected": True, "zero_mutation": True},
            "directory_sequence_rollback": {"lower_sequence_rejected": True, "zero_mutation": True},
            "managed_package_tamper": {"tamper_detected": True, "repair_required": True},
            "upstream_owned_short_name": {"source_kind": "upstream", "immutable_revision": True, "exact_source_identity": True},
            "community_bridge_short_name": {"source_kind": "community_bridge", "immutable_revision": True, "exact_source_identity": True},
            "plugin_data_update_repair_switch_remove_purge": {"marker_preserved": True, "explicit_purge_deleted": True},
            "stdio_environment_and_containment": {"plugin_root_verified": True, "plugin_data_verified": True, "writable": True, "contained": True},
            "missing_runtime_zero_mutation": {"zero_mutation": True, "copy_ready_requirement": True, "dependency_installed": False},
            "explicit_source_switch": {"switch_applied": True, "rollback_verified": True},
            "distribution_sticky_update": {"distribution_unchanged": True, "release_advanced": True},
            "managed_rollback": {"failure_injected": True, "managed_state_restored": True},
            "external_activation_failure": {"materialization_retained": True, "repair_action_recorded": True},
            "promotion_gate_digest_match": {"digest_match": True, "promotion_simulated": True},
            "promotion_gate_digest_mismatch": {"digest_mismatch": True, "promotion_refused": True, "zero_mutation": True},
            "cross_platform_binary_npm_install": {"required_slots_complete": True, "checksums_verified": True},
            "binary_macos_arm64": {"os": "macos", "architecture": "arm64", "checksum_verified": True},
            "binary_macos_amd64": {"os": "macos", "architecture": "amd64", "checksum_verified": True},
            "binary_linux_arm64": {"os": "linux", "architecture": "arm64", "checksum_verified": True},
            "binary_linux_amd64": {"os": "linux", "architecture": "amd64", "checksum_verified": True},
            "binary_windows_amd64": {"os": "windows", "architecture": "amd64", "checksum_verified": True},
            "binary_windows_arm64": {"os": "windows", "architecture": "arm64", "checksum_verified": True},
            "npm_install_node22": {"node_major": 22, "npm_install_verified": True, "binary_checksum_verified": True},
        }
        if scenario.startswith("repair_"):
            expected_values = {"fault_injected_once": True, "repair_succeeded": True, "client": scenario.removeprefix("repair_")}
        else:
            expected_values = expected.get(scenario)
        return bool(expected_values) and all(value.get(key) == expected_value for key, expected_value in expected_values.items())

    def journeys(self) -> None:
        digest = package_digest(EXTERNAL_PACKAGE)
        sandbox = self.fresh_sandbox("direct-external")
        disposable_package = sandbox / "workspace" / "external-package"
        shutil.copytree(EXTERNAL_PACKAGE, disposable_package)
        local_targets = ("codex", "cursor", "kiro")
        target_argument = ",".join(local_targets)
        add_outcome, add_value, add_reason = self.command(["add", "./external-package", "--target", target_argument, "--format", "json"], sandbox, local_targets)
        observed_digest = find_value(add_value, {"package_digest", "tree_digest"}) if add_value else None
        add_identity_valid = observed_digest == digest
        if add_outcome == "passed" and not add_identity_valid:
            add_outcome, add_reason = "failed", "direct-source result omitted or disagreed with the canonical package digest"

        info_outcome, info_value, info_reason = "inconclusive", None, "add did not commit; info was not run"
        update_outcome, update_value, update_reason = "inconclusive", None, "add did not commit; grouped no-change update was not run"
        remove_outcome, remove_value, remove_reason = "inconclusive", None, "add did not commit; remove was not run"
        if add_value is not None and find_value(add_value, {"mutated"}) is True:
            info_outcome, info_value, info_reason = self.command(["info", "e2e-external-package", "--target", target_argument, "--format", "json"], sandbox, local_targets)
            if info_outcome == "passed" and find_value(info_value, {"receipt_reconciled"}) is not True:
                info_outcome, info_reason = "failed", "direct-source info omitted owned-receipt reconciliation"
            update_outcome, update_value, update_reason = self.command(["update", "e2e-external-package", "--target", target_argument, "--format", "json"], sandbox, local_targets)
            if update_outcome == "passed" and not all(
                item["output"]["result"].get("mutated") is False
                and item["output"]["result"].get("no_change") is True
                and item["output"]["result"].get("group_phase") == "external_completed"
                for item in update_value["data"]["targets"]
            ):
                update_outcome, update_reason = "failed", "local grouped update was not an exact successful no-change result"
            # Cleanup is mandatory after a committed add, including when identity or info validation fails.
            remove_outcome, remove_value, remove_reason = self.command(["remove", "e2e-external-package", "--target", target_argument, "--format", "json"], sandbox, local_targets)

        operations = {
            "add": {"outcome": add_outcome, "reason": add_reason},
            "info": {"outcome": info_outcome, "reason": info_reason},
            "update": {"outcome": update_outcome, "reason": update_reason},
            "remove": {"outcome": remove_outcome, "reason": remove_reason},
        }
        traces = [
            value["_launch_command_trace"]
            for value in (add_value, info_value, update_value, remove_value)
            if isinstance(value, dict) and isinstance(value.get("_launch_command_trace"), dict)
        ]
        lifecycle_outcomes = (add_outcome, info_outcome, update_outcome, remove_outcome)
        if lifecycle_outcomes == ("passed", "passed", "passed", "passed"):
            outcome, reason = "passed", "local grouped add, info, no-change update, and remove completed"
        else:
            outcome = "failed" if "failed" in lifecycle_outcomes else "inconclusive"
            reason = next((str(item["reason"]) for item in operations.values() if item["outcome"] != "passed"), "direct-source lifecycle did not complete")
        client_version = None
        self.add("direct_external_package", "e2e-external-package", "cursor", "materialization", outcome, reason, tuple_value=self.tuple(product_id="e2e-external-package", digest=digest, manifest_digest=sha256_file(EXTERNAL_PACKAGE / "plugin.json"), distribution_id="direct/e2e-external-package", distribution_kind="direct", release_sequence=1, package_version="1.0.0", client_version=client_version if isinstance(client_version, str) else None, dependency="direct-local-source"), details={"evidence_basis": "repository_owned_disposable_observer", "runtime_proof": False, "native_discovery_proof": False, "tree_digest_algorithm": "agentplugins-tree-sha256-v1", "directory_submission_used": False, "source_locator": "fixture://external-package", "operations": operations, "command_traces": traces})
        fork_outcome, fork_value, fork_reason = self.driven_scenario("fork_submission")
        if fork_outcome == "passed" and not (
            fork_value
            and fork_value.get("fork_created") is True
            and fork_value.get("branch_submission") is True
            and fork_value.get("submission_validated") is True
            and fork_value.get("publication_performed") is False
            and fork_value.get("pr_created") is False
            and fork_value.get("network_performed") is False
        ):
            fork_outcome, fork_reason = "failed", "fork driver omitted validated non-publication submission proof"
        fork_client_version = find_value(fork_value, {"client_version"}) if fork_value else None
        fork_artifact = fork_value.get("validator_artifact", {}) if fork_value else {}
        fork_package = fork_artifact.get("package", {})
        self.add("fork_submission", "fixture-bridge", None, "schema", fork_outcome, fork_reason, tuple_value=self.tuple(product_id="fixture-bridge", digest=fork_package.get("tree_digest") or package_digest(FORK_PACKAGE), manifest_digest=fork_package.get("manifest_digest") or sha256_file(FORK_PACKAGE / "plugin.json"), distribution_id="contributor/fixture-bridge", distribution_kind="community_bridge", release_sequence=1, package_version=fork_package.get("package_version") or "1.2.3", client_version=fork_client_version if isinstance(fork_client_version, str) else None, dependency="disposable-fork-validation"), details={"evidence_basis": "repository_owned_disposable_observer", "runtime_proof": False, "native_discovery_proof": False, "publication_or_pr_created": False, "publication_required": False, "supplemental_contract_evidence": True, "satisfies_first_stable_external_pr_gate": False, "validator_artifact": fork_artifact, "command_traces": fork_value.get("command_traces", []) if fork_value else []})
        rejected_outcome, rejected_value, rejected_reason = self.driven_scenario("fork_submission_rejected")
        if rejected_outcome == "passed" and not (
            rejected_value
            and rejected_value.get("fork_created") is True
            and rejected_value.get("submission_rejected") is True
            and rejected_value.get("no_side_effect") is True
            and rejected_value.get("no_candidate") is True
        ):
            rejected_outcome, rejected_reason = "failed", "rejected fork driver omitted rejection and zero-side-effect proof"
        rejected_client_version = find_value(rejected_value, {"client_version"}) if rejected_value else None
        self.add("fork_submission_rejected", "fixture-bridge", None, "schema", rejected_outcome, rejected_reason, tuple_value=self.tuple(product_id="fixture-bridge", digest=package_digest(FORK_PACKAGE), manifest_digest=sha256_file(FORK_PACKAGE / "plugin.json"), distribution_id="contributor/fixture-bridge", distribution_kind="community_bridge", release_sequence=1, package_version="1.2.3", client_version=rejected_client_version if isinstance(rejected_client_version, str) else None, dependency="disposable-fork-validation"), details={"evidence_basis": "repository_owned_disposable_observer", "runtime_proof": False, "native_discovery_proof": False, "publication_or_pr_created": False, "publication_required": False, "expected_rejection": True, "validator_artifact": rejected_value.get("validator_artifact") if rejected_value else None, "command_traces": rejected_value.get("command_traces", []) if rejected_value else []})

    def external_pr_gate(self) -> None:
        config = read_production_config()
        valid, reason = external_pr_evidence_valid(
            self.external_pr_evidence,
            catalog_repository=config["catalog_repository"],
            catalog_sha=self.github_sha,
            snapshot=self.snapshot,
            snapshot_digest=self.snapshot_digest,
            release_repository=config["cli_release_repository"],
            release_tag=self.release_tag,
            release_commit=self.release_manifest.get("commit"),
            release_manifest_digest=self.release_manifest_digest,
        )
        self.add(
            "first_stable_external_fork_pr", None, None, "harness",
            "passed" if valid else "failed", reason,
            details={
                "required_for_first_stable_launch": True,
                "local_fork_clone_is_supplemental_only": True,
                "signed_observer_bundle_digest": self.observer_bundle_digest,
                "evidence": self.external_pr_evidence,
            },
        )

    def export(self) -> dict[str, Any]:
        self.validate_fixtures()
        if self.mode == "fixture-only":
            self.add("fixture_only_non_runtime_contract", None, None, "harness", "passed", "fixture-only mode validates contracts and emits no runtime claim")
        else:
            self.discover_version()
            self.native_platform_matrix()
            self.all_package_matrix()
            self.context7_multi_target()
            self.hero_lifecycle_matrix()
            self.hero_runtime_matrix()
            self.shared_backend()
            self.fault_matrix()
            self.acceptance_postconditions()
            self.journeys()
            self.external_pr_gate()
        counts = Counter(row["outcome"] for row in self.rows)
        required = [row for row in self.rows if row["level"] != "harness" or row["scenario"].startswith("native_") or row["scenario"] == "first_stable_external_fork_pr"]
        complete = self.mode == "enforced" and bool(required) and all(row["outcome"] == "passed" for row in required)
        run_seed = json.dumps([self.observed_at, self.os_name, self.architecture, sha256_file(SCENARIOS)])
        required_ids = self.config["fault_scenarios"] + self.config["adapter_repair_faults"] + self.config["advanced_scenarios"] + self.config["acceptance_postconditions"] + self.config["journeys"] + ["shared_copilot_vscode_backend"]
        return {
            "schema_version": 3,
            "run": {"id": hashlib.sha256(run_seed.encode()).hexdigest()[:16], "mode": self.mode, "runtime_claims": self.mode == "enforced", "observed_at": self.observed_at, "platform": self.os_name, "architecture": self.architecture, "disposable": True, "root_id": exported_root_id(self.challenge), "github_sha": self.github_sha, "github_run_id": self.github_run_id, "github_run_attempt": self.github_run_attempt, "caller_event_name": self.caller_event_name, "caller_ref": self.caller_ref, "caller_workflow_ref": self.caller_workflow_ref, "challenge": self.challenge.get("value") if self.challenge else None, "observer_bundle_digest": self.observer_bundle_digest, "cli": {"available": self.cli_available, "version": self.cli_version or self.expected_version, "binary_digest": self.binary_digest}},
            "release": {"repository": read_production_config()["cli_release_repository"] if self.mode == "enforced" else None, "tag": self.release_tag, "tag_commit": self.release_manifest.get("commit"), "release_id": self.release_identity.get("release_id"), "immutable": self.release_identity.get("immutable") if self.mode == "enforced" else None, "manifest_digest": self.release_manifest_digest, "checksums_digest": self.release_checksums_digest},
            "directory": {"origin": self.directory_environment.get("AGENTPLUGINS_DIRECTORY_ORIGIN"), "snapshot_digest": self.snapshot_digest, "sequence": self.snapshot.get("sequence"), "trust_root_digest": sha256_file(PRODUCTION_DIRECTORY_TRUST) if self.mode == "enforced" else None},
            "scenario_contract": {"id": self.config["contract_id"], "digest": sha256_file(SCENARIOS), "expected_ids": list(EXPECTED_ACCEPTANCE_SCENARIOS), "required_singleton_ids": required_ids, "expected_counts": EXPECTED_COUNTS},
            "matrix": self.rows,
            "summary": {**{name: counts[name] for name in ("passed", "failed", "inconclusive", "not_applicable")}, "required_gates_complete": complete, "hero_runtime_results": sum(row["scenario"] == "hero_5x3_runtime" and row["outcome"] == "passed" for row in self.rows)},
            "privacy": {
                "redacted_export": not self.consent["no_real_project_proof"]["absolute_paths_exported"] and not self.consent["no_real_project_proof"]["credential_material_exported"],
                "consent_artifact_digest": self.consent_digest,
                "pseudonymous_identity_id": self.consent["pseudonymous_identity_id"],
                "pseudonymous_workspace_id": self.consent["pseudonymous_workspace_id"],
                "dedicated_identity": self.consent["dedicated_identity"],
                "disposable_project_status": self.consent["disposable_project_status"],
                "operation_mode": self.consent["operation_mode"],
                "auth_origin": self.consent["auth_origin"],
                "cleanup_outcome": self.consent["cleanup_outcome"],
                "contains_absolute_home_paths": self.consent["no_real_project_proof"]["absolute_paths_exported"],
                "contains_credentials": self.consent["no_real_project_proof"]["credential_material_exported"],
                "real_user_project_used": self.consent["no_real_project_proof"]["real_project_accessed"],
                "auth_copied": self.consent["no_real_project_proof"]["auth_copied"],
            },
        }


def assert_redacted(value: dict[str, Any]) -> None:
    """Refuse evidence containing obvious credentials or absolute home paths."""
    validate_evidence_redaction(value, context="evidence export")
    LaunchHarness._reject_mutable_refs(value)
    for row in value.get("matrix", []):
        if row.get("outcome") != "passed" or row.get("level") == "harness":
            continue
        tuple_value = row.get("tuple", {})
        details = row.get("details", {})
        client_version = str(tuple_value.get("client_version") or "")
        native_level = row.get("level") in {"discovery", "runtime", "oauth"}
        if client_version.startswith(("native-state-v1@", "native-observation-v1@")):
            raise ValueError(f"synthetic client version cannot be promoted: {row.get('id')}")
        if native_level:
            native_evidence = details.get("native_discovery_evidence")
            if row.get("level") == "discovery":
                production = read_production_config()
                expected_package = {
                    "schema_version": 1, "package": production["copilot_cli_package"],
                    "version": production["copilot_cli_version"], "integrity": production["copilot_cli_integrity"],
                    "node_major": production["copilot_node_major"], "signature_audit": True,
                    "version_argv": ["copilot", "--version"], "observed_version": production["copilot_cli_version"],
                }
                package = details.get("copilot_package")
                expected_dependency = (
                    f"{production['copilot_cli_package']}@{production['copilot_cli_version']}"
                    f"#{production['copilot_cli_integrity']}"
                )
                if (
                    row.get("scenario") != "all_26_info" or row.get("client") != "copilot"
                    or details.get("evidence_basis") != "native_client_command"
                    or details.get("native_identity_state") != "managed"
                    or not isinstance(package, dict)
                    or set(package) != {*expected_package, "executable_digest", "version_stdout_digest"}
                    or any(package.get(key) != expected for key, expected in expected_package.items())
                    or not DIGEST.fullmatch(str(package.get("executable_digest", "")))
                    or not DIGEST.fullmatch(str(package.get("version_stdout_digest", "")))
                    or tuple_value.get("dependency_identity") != expected_dependency
                    or not authoritative_repository_copilot_evidence(
                        native_evidence, client_version=tuple_value.get("client_version"),
                        product_id=str(row.get("plugin")), physical_artifact_id=str(details.get("physical_artifact_id") or ""),
                        expected_version=production["copilot_cli_version"],
                    )
                ):
                    raise ValueError(f"passed discovery evidence lacks exact repository-owned Copilot operations: {row.get('id')}")
            elif row.get("client") == "chatgpt":
                if (
                    row.get("scenario") != "chatgpt_registered_binding"
                    or row.get("plugin") != "cloudflare-docs"
                    or details.get("evidence_basis") != "protected_external_observer"
                    or details.get("native_discovery_proof") is not False
                    or details.get("public_mcp_proof") is not True
                    or "native_discovery_evidence" in details
                    or not authoritative_public_mcp_evidence(details.get("public_mcp_evidence"))
                ):
                    raise ValueError(f"passed ChatGPT runtime lacks authoritative public MCP operations: {row.get('id')}")
            elif (
                details.get("evidence_basis") != "protected_external_observer"
                or details.get("native_discovery_proof") is not True
                or not authoritative_protected_runtime_evidence(
                    native_evidence, client_version=tuple_value.get("client_version"),
                    product_id=str(row.get("plugin")), client=str(row.get("client")),
                    challenge=str(details.get("challenge", "")),
                )
            ):
                raise ValueError(f"passed runtime evidence lacks authoritative external client operations: {row.get('id')}")
            required = ("product_id", "tree_digest", "manifest_digest", "distribution_id", "distribution_kind", "release_sequence", "package_version", "source_repository", "source_revision", "source_path", "snapshot_sequence", "snapshot_digest", "binary_digest", "installer_version", "adapter_version", "client_version", "os", "architecture", "observed_at")
        else:
            if details.get("evidence_basis") != "repository_owned_disposable_observer" or details.get("runtime_proof") is not False or details.get("native_discovery_proof") is not False or tuple_value.get("client_version") is not None:
                raise ValueError(f"repository-owned proof crossed into native/runtime semantics: {row.get('id')}")
            required = ("product_id", "tree_digest", "manifest_digest", "distribution_id", "distribution_kind", "release_sequence", "package_version", "snapshot_sequence", "snapshot_digest", "binary_digest", "installer_version", "adapter_version", "os", "architecture", "observed_at")
        if any(not tuple_value.get(field) for field in required):
            raise ValueError(f"passed evidence has an incomplete applicability tuple: {row.get('id')}")
        for field in ("tree_digest", "manifest_digest", "snapshot_digest", "binary_digest"):
            if not DIGEST.fullmatch(str(tuple_value[field])):
                raise ValueError(f"passed evidence has an invalid digest: {row.get('id')}")
    identities = [(row.get("scenario"), row.get("plugin"), row.get("client"), row.get("level")) for row in value.get("matrix", [])]
    if len(identities) != len(set(identities)):
        raise ValueError("evidence contains duplicate tuples")
    if value.get("run", {}).get("mode") == "enforced" and value.get("summary", {}).get("hero_runtime_results") != 15:
        raise ValueError("enforced evidence requires exactly 15 hero runtime results")
    if value.get("run", {}).get("mode") == "enforced":
        scenarios = value.get("scenario_contract", {})
        config = json.loads(SCENARIOS.read_text())
        required_singletons = config["fault_scenarios"] + config["adapter_repair_faults"] + config["advanced_scenarios"] + config["acceptance_postconditions"] + config["journeys"] + ["shared_copilot_vscode_backend"]
        if tuple(scenarios.get("expected_ids", ())) != EXPECTED_ACCEPTANCE_SCENARIOS or scenarios.get("required_singleton_ids") != required_singletons or scenarios.get("expected_counts") != EXPECTED_COUNTS:
            raise ValueError("enforced evidence scenario IDs/counts differ from the immutable contract")
        challenge = value.get("run", {}).get("challenge")
        if not isinstance(challenge, str) or not re.fullmatch(r"[a-f0-9]{64}", challenge):
            raise ValueError("enforced evidence lacks a GitHub/release/Directory/root-bound challenge")
        rows = value.get("matrix", [])
        actual_counts = {
            "directory_products": len({row.get("plugin") for row in rows if row.get("scenario", "").startswith("all_26_")}),
            "directory_lifecycle_rows": sum(row.get("scenario", "").startswith("all_26_") for row in rows),
            "hero_lifecycle_rows": sum(row.get("scenario") == "hero_5x3_lifecycle" for row in rows),
            "hero_runtime_rows": sum(row.get("scenario") == "hero_5x3_runtime" for row in rows),
            "context7_grouped_rows": sum(row.get("scenario", "").startswith("context7_three_target_") for row in rows),
            "chatgpt_rows": sum(row.get("scenario") == "chatgpt_registered_binding" for row in rows),
            "shared_backend_rows": sum(row.get("scenario") == "shared_copilot_vscode_backend" for row in rows),
            "acceptance_postcondition_rows": sum(row.get("scenario") in EXPECTED_ACCEPTANCE_SCENARIOS for row in rows),
            "native_platform_rows": sum(row.get("scenario", "").startswith("native_") for row in rows),
            "fault_rows": sum(row.get("scenario") in set(config["fault_scenarios"] + config["adapter_repair_faults"] + config["advanced_scenarios"]) for row in rows),
            "journey_rows": sum(row.get("scenario") in set(config["journeys"]) for row in rows),
        }
        if actual_counts != EXPECTED_COUNTS:
            raise ValueError(f"enforced evidence row counts differ from immutable contract: {actual_counts}")
        validate_enforced_scenario_coverage(rows, config)
    if any(row.get("client") == "chatgpt" and row.get("plugin") != "cloudflare-docs" for row in value.get("matrix", [])):
        raise ValueError("evidence makes an unsupported broad ChatGPT inference")
    if value.get("run", {}).get("mode") == "fixture-only":
        if value.get("run", {}).get("runtime_claims") is not False or value.get("summary", {}).get("required_gates_complete") is not False:
            raise ValueError("fixture-only evidence cannot escalate to a runtime or stable-gate claim")
        if any(row.get("level") != "harness" for row in value.get("matrix", [])):
            raise ValueError("fixture-only evidence may contain only explicitly non-runtime harness rows")
    body = json.dumps(value, sort_keys=True)
    if SECRET_NAME.search(body):
        # Schema field names describe privacy exclusions; only reject assignment-like values.
        if re.search(r'(?i)(token|secret|password|cookie|authorization|oauth[_-]?code)["\s]*[:=]["\s]+(?!false|null)', body):
            raise ValueError("evidence export contains a credential-like value")
    def strings(item: Any):
        if isinstance(item, str):
            yield item
        elif isinstance(item, dict):
            for child in item.values():
                yield from strings(child)
        elif isinstance(item, list):
            for child in item:
                yield from strings(child)

    absolute_path = re.compile(r"(?:^|\s)(?:/(?!/)[^\s]+|[A-Za-z]:\\\\[^\s]+)")
    for string in strings(value):
        if absolute_path.search(string):
            raise ValueError("evidence export contains an absolute local path")
        if string.startswith(("http://", "https://")):
            parsed = urlsplit(string)
            if parsed.username or parsed.password:
                raise ValueError("evidence export contains URL credentials")


def validate_enforced_scenario_coverage(rows: list[dict[str, Any]], config: dict[str, Any]) -> None:
    singleton_ids = set(config["fault_scenarios"] + config["adapter_repair_faults"] + config["advanced_scenarios"] + config["acceptance_postconditions"] + config["journeys"] + ["shared_copilot_vscode_backend"])
    actual = Counter(row.get("scenario") for row in rows if row.get("scenario") in singleton_ids)
    if set(actual) != singleton_ids or any(count != 1 for count in actual.values()):
        raise ValueError("enforced evidence omitted or duplicated a required scenario family")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=("enforced", "fixture-only"), default="enforced")
    parser.add_argument("--binary", type=Path, help="exact Agent Plugins CLI binary")
    parser.add_argument("--binary-digest", help="sha256 checksum of the exact binary")
    parser.add_argument("--expected-version", help="exact stable agentplugins version (0.1.8 or newer)")
    parser.add_argument("--attestations", type=Path, help="reviewed runtime/OAuth attestation input")
    parser.add_argument("--notion-oauth-attestation", type=Path, help="separate approved Notion OAuth/runtime artifact")
    parser.add_argument("--chatgpt-attestation", type=Path, help="separate Cloudflare Docs ChatGPT binding/UI/runtime artifact")
    parser.add_argument("--observer-bundle", type=Path, help="Ed25519-signed protected observer response containing all live inputs")
    parser.add_argument("--directory-origin", help="credential-free signed Directory HTTPS origin")
    parser.add_argument("--directory-snapshot", type=Path)
    parser.add_argument("--directory-envelope", type=Path)
    parser.add_argument("--directory-trust", type=Path)
    parser.add_argument("--prepared-context", type=Path, help="workflow-prepared official release/Directory/challenge context")
    parser.add_argument("--asset-name", help="manifest-listed native binary asset for this runner")
    parser.add_argument("--native-observations", type=Path, help="directory containing six native and one Node 22 observations")
    parser.add_argument("--copilot-executable", type=Path, help="exact local GitHub Copilot CLI executable")
    parser.add_argument("--copilot-metadata", type=Path, help="validated npm signature/version/integrity metadata")
    parser.add_argument("--run-root", type=Path, required=True, help="nonexistent path reserved for this disposable run")
    parser.add_argument("--consent", type=Path, required=True, help="explicit stable-launch E2E consent artifact")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    release_manifest = None
    release_identity = None
    release_manifest_digest = None
    release_checksums_digest = None
    observer_bundle_digest = None
    release_tag = None
    challenge = None
    github: dict[str, str] = {}
    prepared: dict[str, Any] = {}
    if args.mode == "enforced":
        if not all((args.prepared_context, args.asset_name, args.observer_bundle)):
            raise ValueError("enforced mode requires prepared official context, asset, and observer")
        if any(value is not None for value in (args.binary, args.binary_digest, args.expected_version, args.directory_origin, args.directory_snapshot, args.directory_envelope, args.directory_trust)):
            raise ValueError("enforced mode forbids caller-paired binary/version/digest/Directory/driver inputs")
        prepared = json.loads(args.prepared_context.read_text())
        prepared_root = args.prepared_context.resolve().parent
        config = read_production_config()
        if (
            prepared.get("catalog_repository") != config["catalog_repository"]
            or prepared.get("cli_release_repository") != config["cli_release_repository"]
            or prepared.get("cli_release_tag") != config["cli_release_tag"]
        ):
            raise ValueError("prepared context repositories/tag do not match checked-in production identity")
        release_manifest = prepared["release_manifest"]
        release_identity = prepared["github_release_identity"]
        release_manifest_digest = prepared["release_manifest_digest"]
        release_checksums_digest = prepared["release_checksums_digest"]
        release_tag = prepared["cli_release_tag"]
        github = prepared["github"]
        challenge = prepared["challenge"]
        if challenge.get("release_manifest_digest") != release_manifest_digest or challenge.get("directory_digest") != prepared["directory"]["digest"] or challenge.get("scenario_contract_digest") != sha256_file(SCENARIOS):
            raise ValueError("prepared challenge is not bound to release and Directory digests")
        if any(github.get(field) != expected for field, expected in (
            ("caller_ref", "refs/heads/main"),
            ("caller_workflow_ref", f"{TRUSTED_CATALOG_REPOSITORY}/.github/workflows/directory-publication.yml@refs/heads/main"),
        )) or github.get("caller_event_name") not in {"push", "schedule", "workflow_dispatch"}:
            raise ValueError("prepared challenge is not bound to the protected caller identity")
        observer_public_key = os.environ.get("OBSERVER_ED25519_PUBLIC_KEY", "")
        observer_key_id = os.environ.get("OBSERVER_KEY_ID", "")
        if not observer_public_key or not observer_key_id:
            raise ValueError("enforced mode requires an explicit trusted observer Ed25519 key")
        validate_observer_bundle_files(
            args.observer_bundle, challenge=challenge["value"],
            public_key_base64=observer_public_key, expected_key_id=observer_key_id,
            artifact_paths={
                "runtime-attestations.json": args.attestations,
                "notion-oauth-attestations.json": args.notion_oauth_attestation,
                "chatgpt-cloudflare-attestation.json": args.chatgpt_attestation,
                "consent.json": args.consent,
            },
        )
        observer_bundle_digest = sha256_file(args.observer_bundle)
        binary_path, resolved_manifest, resolved_identity, resolved_digest, resolved_checksums_digest = (
            validate_prepared_github_release(prepared_root, prepared, args.asset_name)
        )
        if (
            resolved_manifest != release_manifest
            or resolved_identity != release_identity
            or resolved_digest != release_manifest_digest
            or resolved_checksums_digest != release_checksums_digest
        ):
            raise ValueError("validated prepared release differs from the authenticated context")
        declared = next(item for item in release_manifest["assets"].values() if item["file"] == args.asset_name)
        prepared_asset_digest = prepared.get("authenticated_asset", {}).get("digest")
        prepared_attestation = prepared.get("github_asset_attestation")
        fresh_attestation = json.loads(
            (prepared_root / "release" / f"{args.asset_name}.attestation.json").read_text()
        )
        if prepared_attestation != fresh_attestation or prepared_asset_digest != "sha256:" + declared["sha256"]:
            raise ValueError("fresh GitHub asset authentication differs from prepared context")
        validate_capture_release_binding(
            release_manifest=release_manifest, release_identity=release_identity,
            release_manifest_digest=release_manifest_digest,
            release_checksums_digest=release_checksums_digest,
            asset_name=args.asset_name, asset_digest=prepared_asset_digest,
            asset_attestation=prepared_attestation,
        )
        args.binary = binary_path
        args.binary_digest = "sha256:" + declared["sha256"]
        args.expected_version = release_manifest["version"]
        directory = prepared["directory"]
        ledger_commit = directory.get("ledger_commit", "")
        expected_staged_origin = f"https://raw.githubusercontent.com/{TRUSTED_CATALOG_REPOSITORY}/{ledger_commit}/registry/schemas/1/"
        if not FULL_SHA.fullmatch(ledger_commit) or directory.get("origin") != expected_staged_origin:
            raise ValueError("prepared Directory is not bound to the immutable staged ledger commit")
        args.directory_origin = directory["origin"]
        args.directory_snapshot = prepared_root / directory["snapshot"]
        args.directory_envelope = prepared_root / directory["envelope"]
        args.directory_trust = PRODUCTION_DIRECTORY_TRUST
        if sha256_file(args.directory_snapshot) != directory["digest"]:
            raise ValueError("prepared staged Directory snapshot digest changed")
    evidence = sanitize_evidence(LaunchHarness(
        args.binary, args.attestations, mode=args.mode,
        binary_digest=args.binary_digest, expected_version=args.expected_version,
        directory_origin=args.directory_origin, directory_snapshot=args.directory_snapshot,
        directory_envelope=args.directory_envelope, directory_trust=args.directory_trust,
        run_root=args.run_root, consent=args.consent, notion_oauth=args.notion_oauth_attestation,
        chatgpt_attestation=args.chatgpt_attestation,
        release_manifest=release_manifest, release_identity=release_identity, release_manifest_digest=release_manifest_digest,
        release_checksums_digest=release_checksums_digest,
        release_tag=release_tag, github_sha=github.get("sha"), github_run_id=github.get("run_id"),
        github_run_attempt=github.get("run_attempt"), challenge=challenge,
        producer_run_attempt=prepared.get("producer_run_attempt"),
        caller_event_name=github.get("caller_event_name"), caller_ref=github.get("caller_ref"),
        caller_workflow_ref=github.get("caller_workflow_ref"),
        native_observations=args.native_observations,
        observer_bundle_digest=observer_bundle_digest,
        copilot_executable=args.copilot_executable, copilot_metadata=args.copilot_metadata,
    ).export())
    assert_redacted(evidence)
    if args.run_root and args.output.resolve() != (args.run_root.resolve() / "evidence" / args.output.name):
        raise ValueError("output must be inside the disposable evidence root")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n")
    if args.mode == "enforced":
        shutil.copy2(args.observer_bundle, args.output.parent / "signed-observer-bundle.json")
    print(json.dumps(evidence["summary"], sort_keys=True))
    return 0 if args.mode == "fixture-only" or evidence["summary"]["required_gates_complete"] else 2


if __name__ == "__main__":
    raise SystemExit(main())
