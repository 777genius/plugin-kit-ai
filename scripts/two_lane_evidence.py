#!/usr/bin/env python3
"""Strict validators for the two protected launch-evidence lanes.

This module is deliberately an artifact contract, not a production CLI mode.
Policy evidence can gate a release, but can never be projected into Directory
runtime evidence.
"""

from __future__ import annotations

import hashlib
import json
import re
from collections import Counter
from pathlib import Path
from typing import Any, Literal


ROOT = Path(__file__).resolve().parents[1]
_LAUNCH_CONTRACT = json.loads((ROOT / "tests/e2e/launch-scenarios.json").read_text())
_POLICY_HARNESS = json.loads((ROOT / "tests/e2e/source-policy-tests.json").read_text())
POLICY_SCENARIO_IDS = tuple(_POLICY_HARNESS["tests"])
POLICY_SCENARIO_SET = frozenset(POLICY_SCENARIO_IDS)
HERO_PLUGINS = tuple(_LAUNCH_CONTRACT["heroes"])
RUNTIME_CLIENTS = tuple(_LAUNCH_CONTRACT["runtime_clients"])
HERO_RUNTIME_PAIRS = frozenset((plugin, client) for plugin in HERO_PLUGINS for client in RUNTIME_CLIENTS)
RUNTIME_RESULT_COUNT = len(HERO_RUNTIME_PAIRS)
POLICY_RESULT_COUNT = len(POLICY_SCENARIO_IDS)
PLUGIN_KIT_REPOSITORY = "777genius/plugin-kit-ai"
PLUGIN_KIT_TAG = "agentplugins-v0.1.24"
PLUGIN_KIT_COMMIT = "c78c79e44efd5ad07083d63436d9170b107df6cb"
PLUGIN_KIT_PRODUCTION_TREE_DIGEST = "sha256:3635457d320bc2c78a86b9b3d8e4937d14ac59848ffae70c6571167204130de8"
FIXTURE_KEY_ID = "launch-conformance-only"
RELEASED_LINUX_AMD64_DIGEST = "sha256:e79125f7ffabd11c6e211d6b049c2eb2b36eb1aba3a76ce27cac819aeba1e6ca"
RELEASE_MANIFEST_DIGEST = "sha256:eb834da8237b13ed36061aeafb4fbb6f4aadeb5a6fbd4a31d43781f456f3d1e2"
RELEASE_CHECKSUMS_DIGEST = "sha256:623fb73d0e2f59da8b01399842b0d82b8f6456c6e43db2251c0ea5f9e32f37e3"

# Frozen v4 replay identity. These values must never authorize a fresh run.
V4_PLUGIN_KIT_TAG = "agentplugins-v0.1.18"
V4_PLUGIN_KIT_COMMIT = "74a3790ee15d92afda8e8e3dd8f903c04811cfc7"
V4_PLUGIN_KIT_PRODUCTION_TREE_DIGEST = "sha256:4a64cddbf6680d55270a8bec9b3810673995b7328d8ff62feab8421a65378607"
V4_RELEASED_LINUX_AMD64_DIGEST = "sha256:9a294d2d117d6be2042aa28f911999edccf051ccbc3f1c7f0f46920cfd6b5779"
V4_RELEASE_MANIFEST_DIGEST = "sha256:0e8f7316ddef542067bdd7276273fffa3bc00532afed8fd42be12f612aedea57"
V4_RELEASE_CHECKSUMS_DIGEST = "sha256:d581ac34d9880afe998f8f871df285b5474623778d2eae98ebc8780a932a9fa8"
DIGEST = re.compile(r"^sha256:[0-9a-f]{64}$")
SHA = re.compile(r"^[0-9a-f]{40}$")


class TwoLaneEvidenceError(ValueError):
    pass


def canonical_json(value: Any) -> bytes:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode() + b"\n"


def sha256(body: bytes) -> str:
    return "sha256:" + hashlib.sha256(body).hexdigest()


def sha256_file(path: Path) -> str:
    return sha256(path.read_bytes())


def require_uap_sha(value: str) -> str:
    if not isinstance(value, str) or SHA.fullmatch(value) is None:
        raise TwoLaneEvidenceError("UAP SHA must be one explicit canonical 40-hex commit")
    return value


def require_directory_ledger_sha(value: str, *, uap_sha: str) -> str:
    """Validate the independent Directory authority and forbid UAP collapse."""
    if not isinstance(value, str) or SHA.fullmatch(value) is None:
        raise TwoLaneEvidenceError("Directory ledger SHA must be one explicit canonical 40-hex commit")
    if value == uap_sha:
        raise TwoLaneEvidenceError("Directory ledger SHA must differ from the UAP SHA")
    return value


LaunchEvidencePurpose = Literal["current", "historical"]


def validate_launch_schema(
    value: dict[str, Any], *, purpose: LaunchEvidencePurpose = "current",
) -> None:
    """Route launch evidence by caller purpose; artifact bytes cannot downgrade it."""
    if purpose not in {"current", "historical"}:
        raise TwoLaneEvidenceError("unknown launch evidence validation purpose")
    version = value.get("schema_version")
    if type(version) is not int or version not in {3, 4, 5}:
        raise TwoLaneEvidenceError("unknown launch evidence schema_version")
    if purpose == "current" and version != 5:
        raise TwoLaneEvidenceError("current launch evidence requires schema_version 5")
    try:
        import jsonschema
        schema = json.loads((ROOT / f"tests/e2e/schemas/launch-evidence-v{version}.schema.json").read_text())
        errors = sorted(jsonschema.Draft202012Validator(
            schema, format_checker=jsonschema.FormatChecker(),
        ).iter_errors(value), key=lambda item: list(item.absolute_path))
    except ImportError as error:  # pragma: no cover - protected jobs install it
        raise TwoLaneEvidenceError("jsonschema is required for launch evidence validation") from error
    if errors:
        raise TwoLaneEvidenceError(f"launch evidence v{version} schema mismatch: {errors[0].message}")


def classified_runtime_lists(config: dict[str, Any]) -> dict[str, tuple[str, ...]]:
    """Return immutable runtime-only lists and reject classification drift."""
    classified = tuple(
        config[family]
        for family in ("fault_scenarios", "adapter_repair_faults", "advanced_scenarios")
    )
    flat = tuple(item for family in classified for item in family)
    if len(flat) != len(set(flat)):
        raise TwoLaneEvidenceError("fault/adapter/advanced contract IDs must be unique")
    present = tuple(item for item in flat if item in POLICY_SCENARIO_SET)
    acceptance = tuple(config["acceptance_postconditions"])
    if tuple(item for item in acceptance if item in POLICY_SCENARIO_SET) != POLICY_SCENARIO_IDS[:3]:
        raise TwoLaneEvidenceError("acceptance policy classification drift")
    if set(present) != POLICY_SCENARIO_SET - set(POLICY_SCENARIO_IDS[:3]):
        raise TwoLaneEvidenceError("fault/advanced policy classification drift")
    runtime = {
        "fault_adapter_advanced": tuple(item for item in flat if item not in POLICY_SCENARIO_SET),
        "acceptance": tuple(item for item in acceptance if item not in POLICY_SCENARIO_SET),
    }
    expected_counts = config.get("expected_counts", {})
    if (
        len(runtime["fault_adapter_advanced"]) != expected_counts.get("fault_rows")
        or len(runtime["acceptance"]) != expected_counts.get("acceptance_postcondition_rows")
    ):
        raise TwoLaneEvidenceError("runtime scenario lists and canonical expected counts differ")
    return runtime


def reject_policy_scenario_before_effect(scenario_id: str) -> None:
    """Released-lane preflight. Call before creating a subprocess or path."""
    if scenario_id in POLICY_SCENARIO_SET:
        raise TwoLaneEvidenceError(
            f"{scenario_id} is source-policy conformance and is forbidden in released-binary execution"
        )


def validate_released_binary_evidence(
    value: dict[str, Any], *, uap_sha: str, directory_ledger_sha: str,
    publication_id: str, publication_sequence: int,
    publication_snapshot_digest: str, publication_source_commit: str,
    purpose: Literal["current", "historical"] = "current",
) -> str:
    uap_sha = require_uap_sha(uap_sha)
    directory_ledger_sha = require_directory_ledger_sha(
        directory_ledger_sha, uap_sha=uap_sha,
    )
    if (
        not isinstance(publication_id, str) or not publication_id
        or type(publication_sequence) is not int
        or not 1 <= publication_sequence <= 9_007_199_254_740_991
        or not isinstance(publication_snapshot_digest, str)
        or DIGEST.fullmatch(publication_snapshot_digest) is None
        or not isinstance(publication_source_commit, str)
        or SHA.fullmatch(publication_source_commit) is None
    ):
        raise TwoLaneEvidenceError("Directory publication identity is incomplete or invalid")
    validate_launch_schema(value, purpose=purpose)
    version = value.get("schema_version")
    if version == 5:
        release_version = "0.1.24"
        release_tag = PLUGIN_KIT_TAG
        release_commit = PLUGIN_KIT_COMMIT
        binary_digest = RELEASED_LINUX_AMD64_DIGEST
        manifest_digest = RELEASE_MANIFEST_DIGEST
        checksums_digest = RELEASE_CHECKSUMS_DIGEST
    else:
        release_version = "0.1.18"
        release_tag = V4_PLUGIN_KIT_TAG
        release_commit = V4_PLUGIN_KIT_COMMIT
        binary_digest = V4_RELEASED_LINUX_AMD64_DIGEST
        manifest_digest = V4_RELEASE_MANIFEST_DIGEST
        checksums_digest = V4_RELEASE_CHECKSUMS_DIGEST
    if value.get("evidence_class") != "released_binary":
        raise TwoLaneEvidenceError("runtime evidence_class must be released_binary")
    run = value.get("run", {})
    summary = value.get("summary", {})
    if run.get("mode") != "enforced" or run.get("runtime_claims") is not True:
        raise TwoLaneEvidenceError("released evidence must make an enforced runtime claim")
    if summary.get("released_binary_gate_complete") is not True:
        raise TwoLaneEvidenceError("released binary gate is incomplete")
    cli = run.get("cli", {})
    release = value.get("release", {})
    directory = value.get("directory", {})
    if not (
        cli.get("version") == release_version
        and cli.get("binary_digest") == binary_digest
        and release.get("repository") == PLUGIN_KIT_REPOSITORY
        and release.get("tag") == release_tag
        and release.get("tag_commit") == release_commit
        and release.get("immutable") is True
        and release.get("manifest_digest") == manifest_digest
        and release.get("checksums_digest") == checksums_digest
        and run.get("github_sha") == uap_sha
        and directory.get("origin") == f"https://raw.githubusercontent.com/777genius/universal-agent-plugins/{directory_ledger_sha}/registry/schemas/1/"
        and directory.get("ledger_commit") == directory_ledger_sha
        and directory.get("publication_id") == publication_id
        and directory.get("source_commit") == publication_source_commit
        and directory.get("sequence") == publication_sequence
        and directory.get("snapshot_digest") == publication_snapshot_digest
        and DIGEST.fullmatch(str(directory.get("trust_root_digest", "")))
    ):
        raise TwoLaneEvidenceError("released binary/release/UAP/Directory identity mismatch")
    rows = value.get("matrix")
    if not isinstance(rows, list) or any(row.get("scenario") in POLICY_SCENARIO_SET for row in rows):
        raise TwoLaneEvidenceError("released evidence contains a source-policy row")
    hero = [row for row in rows if row.get("scenario") == "hero_5x3_runtime"]
    pairs = Counter((row.get("plugin"), row.get("client")) for row in hero)
    if set(pairs) != HERO_RUNTIME_PAIRS or any(count != 1 for count in pairs.values()):
        raise TwoLaneEvidenceError("released evidence requires the exact 5x3 hero runtime matrix once each")
    protected = list(hero)
    if version == 5:
        chatgpt = [
            row for row in rows
            if row.get("scenario") == "chatgpt_registered_binding"
            and row.get("plugin") == "cloudflare-docs"
            and row.get("client") == "chatgpt"
            and row.get("level") == "runtime"
        ]
        if len(chatgpt) != 1:
            raise TwoLaneEvidenceError("released evidence requires exactly one Cloudflare Docs/ChatGPT runtime row")
        protected.extend(chatgpt)
    expected_snapshot = directory.get("snapshot_digest")
    expected_sequence = directory.get("sequence")
    for row in protected:
        tuple_value = row.get("tuple", {})
        details = row.get("details", {})
        is_chatgpt = row.get("client") == "chatgpt"
        if (
            row.get("level") != "runtime" or row.get("outcome") != "passed"
            or not isinstance(tuple_value, dict) or not isinstance(details, dict)
            or tuple_value.get("product_id") != row.get("plugin")
            or tuple_value.get("binary_digest") != binary_digest
            or tuple_value.get("snapshot_digest") != expected_snapshot
            or tuple_value.get("snapshot_sequence") != expected_sequence
            or tuple_value.get("installer_version") != release_version
            or tuple_value.get("adapter_version") != release_version
            or details.get("evidence_basis") != "protected_external_observer"
            or details.get("runtime_proof") is not True
            or details.get("native_discovery_proof") is not (False if is_chatgpt else True)
            or (is_chatgpt and details.get("public_mcp_proof") is not True)
            or details.get("release_manifest_digest") != manifest_digest
            or details.get("release_checksums_digest") != checksums_digest
            or details.get("directory_digest") != expected_snapshot
            or details.get("scenario_id") != row.get("scenario")
        ):
            raise TwoLaneEvidenceError("protected runtime row lacks the canonical released-runtime tuple/fields")
    if summary.get("hero_runtime_results") != RUNTIME_RESULT_COUNT:
        raise TwoLaneEvidenceError("released evidence summary differs from the canonical runtime matrix")
    return sha256(canonical_json(value))


def validate_source_policy_evidence(
    value: dict[str, Any], *, scenario_digest: str, harness_digest: str,
    overlay_digest: str, uap_sha: str, expected_schema_version: Literal[1, 2] = 2,
) -> str:
    if type(expected_schema_version) is not int or expected_schema_version not in {1, 2}:
        raise TwoLaneEvidenceError("unknown caller-selected source-policy schema_version")
    if expected_schema_version == 2:
        try:
            import jsonschema
            schema = json.loads(
                (ROOT / "schemas/e2e/source-policy-conformance-v2.schema.json").read_text()
            )
            pending: list[Any] = [schema]
            while pending:
                node = pending.pop()
                if isinstance(node, dict):
                    reference = node.get("$ref")
                    if reference is not None and (
                        not isinstance(reference, str) or not reference.startswith("#/")
                    ):
                        raise TwoLaneEvidenceError(
                            "source-policy v2 schema contains a non-local reference"
                        )
                    pending.extend(node.values())
                elif isinstance(node, list):
                    pending.extend(node)
            errors = sorted(
                jsonschema.Draft202012Validator(schema).iter_errors(value),
                key=lambda item: list(item.absolute_path),
            )
        except ImportError as error:  # pragma: no cover - protected jobs install it
            raise TwoLaneEvidenceError(
                "jsonschema is required for source-policy evidence validation"
            ) from error
        if errors:
            raise TwoLaneEvidenceError(
                f"source-policy evidence v2 schema mismatch: {errors[0].message}"
            )
    uap_sha = require_uap_sha(uap_sha)
    if type(value.get("schema_version")) is not int or value.get("schema_version") != expected_schema_version:
        raise TwoLaneEvidenceError("source-policy schema_version differs from caller purpose")
    if expected_schema_version == 2:
        release_tag = PLUGIN_KIT_TAG
        release_commit = PLUGIN_KIT_COMMIT
        manifest_digest = RELEASE_MANIFEST_DIGEST
        checksums_digest = RELEASE_CHECKSUMS_DIGEST
        production_tree_digest = PLUGIN_KIT_PRODUCTION_TREE_DIGEST
    else:
        release_tag = V4_PLUGIN_KIT_TAG
        release_commit = V4_PLUGIN_KIT_COMMIT
        manifest_digest = V4_RELEASE_MANIFEST_DIGEST
        checksums_digest = V4_RELEASE_CHECKSUMS_DIGEST
        production_tree_digest = V4_PLUGIN_KIT_PRODUCTION_TREE_DIGEST
    identities = value.get("identities", {})
    expected = {
        "plugin_kit_repository": PLUGIN_KIT_REPOSITORY,
        "plugin_kit_tag": release_tag,
        "plugin_kit_commit": release_commit,
        "uap_sha": uap_sha,
        "scenario_digest": scenario_digest,
        "harness_digest": harness_digest,
        "overlay_digest": overlay_digest,
    }
    if value.get("evidence_class") != "source_policy_conformance":
        raise TwoLaneEvidenceError("policy evidence_class mismatch")
    if value.get("runtime_claims") is not False or value.get("released_binary_executed") is not False:
        raise TwoLaneEvidenceError("policy evidence cannot claim released runtime execution")
    if value.get("fixture_key_id") != FIXTURE_KEY_ID:
        raise TwoLaneEvidenceError("policy fixture key mismatch")
    if any(identities.get(key) != expected_value for key, expected_value in expected.items()):
        raise TwoLaneEvidenceError("source-policy identity mismatch")
    if (
        identities.get("release_manifest_digest") != manifest_digest
        or identities.get("release_checksums_digest") != checksums_digest
    ):
        raise TwoLaneEvidenceError("release manifest/checksum identity mismatch")
    rows = value.get("results")
    if not isinstance(rows, list):
        raise TwoLaneEvidenceError("policy results must be an array")
    actual = Counter(row.get("id") for row in rows)
    if set(actual) != POLICY_SCENARIO_SET or any(actual[item] != 1 for item in POLICY_SCENARIO_IDS):
        raise TwoLaneEvidenceError("policy evidence must contain the exact 11 IDs once each")
    canonical_tests = _POLICY_HARNESS["tests"]
    for row in rows:
        test = row.get("test")
        proof = row.get("proof")
        if (
            row.get("outcome") != "passed" or not isinstance(test, dict)
            or test.get("passed") is not True
            or {key: test.get(key) for key in ("package", "name")} != canonical_tests.get(row.get("id"))
            or not DIGEST.fullmatch(str(test.get("transcript_digest", "")))
            or not isinstance(proof, dict) or proof.get("id") != row.get("id")
            or proof.get("source_test") != test
            or proof.get("fixture_key_id") != FIXTURE_KEY_ID
            or proof.get("overlay_digest") != overlay_digest
            or proof.get("runtime_evidence_eligible") is not False
            or sha256(canonical_json(proof)) != row.get("proof_digest")
            or not DIGEST.fullmatch(str(row.get("proof_digest", "")))
        ):
            raise TwoLaneEvidenceError("policy evidence contains a non-canonical test or proof")
    revoked = next(row for row in rows if row["id"] == "revoked_operations_boundary")["proof"].get("unit_oracle", {})
    revoked_stderr = (
        "Resolving and validating one Agent Plugin package for every selected target...\n"
        "agentplugins: no eligible directory release for \"context7\": "
        "upstash/context7: release 1 is revoked\n"
    )
    if not (
        set(revoked) == {"argv", "exit_code", "zero_mutation", "runtime_evidence_eligible", "stdout_digest", "stderr_digest"}
        and
        revoked.get("argv") == ["add", "context7", "--target", "codex", "--format", "json"]
        and revoked.get("exit_code") == 1
        and revoked.get("zero_mutation") is True
        and revoked.get("runtime_evidence_eligible") is False
        and revoked.get("stdout_digest") == sha256(b"")
        and revoked.get("stderr_digest") == sha256(revoked_stderr.encode())
    ):
        raise TwoLaneEvidenceError("revoked policy unit oracle is incomplete")
    before = identities.get("production_source_tree_before")
    after = identities.get("production_source_tree_after")
    if value.get("production_source_unchanged") is not True or before != after or before != production_tree_digest:
        raise TwoLaneEvidenceError("production source tree was not proven unchanged")
    if value.get("policy_conformance_gate_complete") is not True:
        raise TwoLaneEvidenceError("policy conformance gate is incomplete")
    return sha256(canonical_json(value))


def build_readiness_envelope(runtime: dict[str, Any], policy: dict[str, Any], *,
                             scenario_digest: str, harness_digest: str,
                             overlay_digest: str, uap_sha: str,
                             directory_ledger_sha: str, publication_id: str,
                             publication_sequence: int, publication_snapshot_digest: str,
                             publication_source_commit: str,
                             schema_version: Literal[1, 2] = 2,
                             purpose: Literal["current", "historical"] = "current") -> dict[str, Any]:
    if type(schema_version) is not int or schema_version not in {1, 2}:
        raise TwoLaneEvidenceError("unknown caller-selected readiness schema_version")
    expected_schema_version = 2 if purpose == "current" else (
        2 if runtime.get("schema_version") == 5 else 1
    )
    if schema_version != expected_schema_version:
        raise TwoLaneEvidenceError("readiness schema_version differs from caller purpose")
    uap_sha = require_uap_sha(uap_sha)
    directory_ledger_sha = require_directory_ledger_sha(
        directory_ledger_sha, uap_sha=uap_sha,
    )
    runtime_digest = validate_released_binary_evidence(
        runtime, uap_sha=uap_sha, directory_ledger_sha=directory_ledger_sha,
        publication_id=publication_id, publication_sequence=publication_sequence,
        publication_snapshot_digest=publication_snapshot_digest,
        publication_source_commit=publication_source_commit,
        purpose=purpose,
    )
    policy_digest = validate_source_policy_evidence(
        policy, scenario_digest=scenario_digest, harness_digest=harness_digest,
        overlay_digest=overlay_digest, uap_sha=uap_sha,
        expected_schema_version=schema_version,
    )
    return {
        "schema_version": schema_version,
        "evidence_class": "two_lane_readiness",
        "runtime_evidence_digest": runtime_digest,
        "source_policy_evidence_digest": policy_digest,
        "uap_sha": uap_sha,
        "directory_ledger_sha": directory_ledger_sha,
        "publication_id": publication_id,
        "publication_sequence": publication_sequence,
        "publication_snapshot_digest": publication_snapshot_digest,
        "publication_source_commit": publication_source_commit,
        "runtime_results": RUNTIME_RESULT_COUNT + (1 if schema_version == 2 else 0),
        "policy_results": POLICY_RESULT_COUNT,
        "readiness_gate_complete": True,
    }


def validate_completed_readiness(envelope: dict[str, Any], runtime: dict[str, Any],
                                 policy: dict[str, Any], *, schema_version: Literal[1, 2] = 2,
                                 purpose: Literal["current", "historical"] = "current",
                                 **identity: Any) -> None:
    expected = build_readiness_envelope(
        runtime, policy, schema_version=schema_version, purpose=purpose, **identity,
    )
    if envelope != expected:
        raise TwoLaneEvidenceError("completed readiness replay differs from either canonical evidence digest")
