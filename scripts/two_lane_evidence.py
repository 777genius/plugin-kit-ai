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
from typing import Any


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
PLUGIN_KIT_TAG = "agentplugins-v0.1.18"
PLUGIN_KIT_COMMIT = "74a3790ee15d92afda8e8e3dd8f903c04811cfc7"
PLUGIN_KIT_PRODUCTION_TREE_DIGEST = "sha256:4a64cddbf6680d55270a8bec9b3810673995b7328d8ff62feab8421a65378607"
FIXTURE_KEY_ID = "launch-conformance-only"
RELEASED_LINUX_AMD64_DIGEST = "sha256:9a294d2d117d6be2042aa28f911999edccf051ccbc3f1c7f0f46920cfd6b5779"
RELEASE_MANIFEST_DIGEST = "sha256:0e8f7316ddef542067bdd7276273fffa3bc00532afed8fd42be12f612aedea57"
RELEASE_CHECKSUMS_DIGEST = "sha256:d581ac34d9880afe998f8f871df285b5474623778d2eae98ebc8780a932a9fa8"
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


def validate_launch_schema(value: dict[str, Any], *, historical: bool = False) -> None:
    """Route launch evidence to its frozen schema; unknown versions fail closed."""
    version = value.get("schema_version")
    if type(version) is not int or version not in {3, 4}:
        raise TwoLaneEvidenceError("unknown launch evidence schema_version")
    if version == 3 and not historical:
        raise TwoLaneEvidenceError("launch evidence v3 is historical replay only")
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
    validate_launch_schema(value)
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
        cli.get("version") == "0.1.18"
        and cli.get("binary_digest") == RELEASED_LINUX_AMD64_DIGEST
        and release.get("repository") == PLUGIN_KIT_REPOSITORY
        and release.get("tag") == PLUGIN_KIT_TAG
        and release.get("tag_commit") == PLUGIN_KIT_COMMIT
        and release.get("immutable") is True
        and release.get("manifest_digest") == RELEASE_MANIFEST_DIGEST
        and release.get("checksums_digest") == RELEASE_CHECKSUMS_DIGEST
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
    expected_snapshot = directory.get("snapshot_digest")
    expected_sequence = directory.get("sequence")
    for row in hero:
        tuple_value = row.get("tuple", {})
        details = row.get("details", {})
        if (
            row.get("level") != "runtime" or row.get("outcome") != "passed"
            or not isinstance(tuple_value, dict) or not isinstance(details, dict)
            or tuple_value.get("product_id") != row.get("plugin")
            or tuple_value.get("binary_digest") != RELEASED_LINUX_AMD64_DIGEST
            or tuple_value.get("snapshot_digest") != expected_snapshot
            or tuple_value.get("snapshot_sequence") != expected_sequence
            or tuple_value.get("installer_version") != "0.1.18"
            or tuple_value.get("adapter_version") != "0.1.18"
            or details.get("evidence_basis") != "protected_external_observer"
            or details.get("runtime_proof") is not True
            or details.get("native_discovery_proof") is not True
            or details.get("release_manifest_digest") != RELEASE_MANIFEST_DIGEST
            or details.get("release_checksums_digest") != RELEASE_CHECKSUMS_DIGEST
            or details.get("directory_digest") != expected_snapshot
            or details.get("scenario_id") != "hero_5x3_runtime"
        ):
            raise TwoLaneEvidenceError("hero runtime row lacks the canonical released-runtime tuple/fields")
    if summary.get("hero_runtime_results") != RUNTIME_RESULT_COUNT:
        raise TwoLaneEvidenceError("released evidence summary differs from the canonical runtime matrix")
    return sha256(canonical_json(value))


def validate_source_policy_evidence(
    value: dict[str, Any], *, scenario_digest: str, harness_digest: str,
    overlay_digest: str, uap_sha: str,
) -> str:
    uap_sha = require_uap_sha(uap_sha)
    identities = value.get("identities", {})
    expected = {
        "plugin_kit_repository": PLUGIN_KIT_REPOSITORY,
        "plugin_kit_tag": PLUGIN_KIT_TAG,
        "plugin_kit_commit": PLUGIN_KIT_COMMIT,
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
        identities.get("release_manifest_digest") != RELEASE_MANIFEST_DIGEST
        or identities.get("release_checksums_digest") != RELEASE_CHECKSUMS_DIGEST
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
    if value.get("production_source_unchanged") is not True or before != after or before != PLUGIN_KIT_PRODUCTION_TREE_DIGEST:
        raise TwoLaneEvidenceError("production source tree was not proven unchanged")
    if value.get("policy_conformance_gate_complete") is not True:
        raise TwoLaneEvidenceError("policy conformance gate is incomplete")
    return sha256(canonical_json(value))


def build_readiness_envelope(runtime: dict[str, Any], policy: dict[str, Any], *,
                             scenario_digest: str, harness_digest: str,
                             overlay_digest: str, uap_sha: str,
                             directory_ledger_sha: str, publication_id: str,
                             publication_sequence: int, publication_snapshot_digest: str,
                             publication_source_commit: str) -> dict[str, Any]:
    uap_sha = require_uap_sha(uap_sha)
    directory_ledger_sha = require_directory_ledger_sha(
        directory_ledger_sha, uap_sha=uap_sha,
    )
    runtime_digest = validate_released_binary_evidence(
        runtime, uap_sha=uap_sha, directory_ledger_sha=directory_ledger_sha,
        publication_id=publication_id, publication_sequence=publication_sequence,
        publication_snapshot_digest=publication_snapshot_digest,
        publication_source_commit=publication_source_commit,
    )
    policy_digest = validate_source_policy_evidence(
        policy, scenario_digest=scenario_digest, harness_digest=harness_digest,
        overlay_digest=overlay_digest, uap_sha=uap_sha,
    )
    return {
        "schema_version": 1,
        "evidence_class": "two_lane_readiness",
        "runtime_evidence_digest": runtime_digest,
        "source_policy_evidence_digest": policy_digest,
        "uap_sha": uap_sha,
        "directory_ledger_sha": directory_ledger_sha,
        "publication_id": publication_id,
        "publication_sequence": publication_sequence,
        "publication_snapshot_digest": publication_snapshot_digest,
        "publication_source_commit": publication_source_commit,
        "runtime_results": RUNTIME_RESULT_COUNT,
        "policy_results": POLICY_RESULT_COUNT,
        "readiness_gate_complete": True,
    }


def validate_completed_readiness(envelope: dict[str, Any], runtime: dict[str, Any],
                                 policy: dict[str, Any], **identity: Any) -> None:
    expected = build_readiness_envelope(runtime, policy, **identity)
    if envelope != expected:
        raise TwoLaneEvidenceError("completed readiness replay differs from either canonical evidence digest")
