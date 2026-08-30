#!/usr/bin/env python3
"""Build or replay the canonical two-lane readiness envelope."""

from __future__ import annotations

import argparse
import inspect
import json
from pathlib import Path
from typing import Any

from two_lane_evidence import (
    build_readiness_envelope, canonical_json, require_directory_ledger_sha,
    require_uap_sha, sha256_file, validate_completed_readiness,
)


ROOT = Path(__file__).resolve().parents[1]
SCENARIOS = ROOT / "tests/e2e/launch-scenarios.json"
HARNESS = ROOT / "tests/e2e/source-policy-tests.json"
OVERLAY = ROOT / "tests/e2e/source-policy-overlay.json"


def call_versioned(
    function: Any, *values: dict[str, Any], schema_version: int,
    identity: dict[str, Any],
) -> Any:
    parameters = inspect.signature(function).parameters
    supports_schema_version = "schema_version" in parameters
    supports_purpose = "purpose" in parameters
    if schema_version == 2 and not (supports_schema_version and supports_purpose):
        raise RuntimeError("readiness v2 requires the version-aware evidence validator")
    versioned: dict[str, Any] = {}
    if supports_schema_version:
        versioned["schema_version"] = schema_version
    if supports_purpose:
        versioned["purpose"] = "historical" if schema_version == 1 else "current"
    return function(*values, **versioned, **identity)


def identities(args: argparse.Namespace, uap_sha: str) -> dict[str, Any]:
    return {
        "scenario_digest": sha256_file(SCENARIOS),
        "harness_digest": sha256_file(HARNESS),
        "overlay_digest": sha256_file(OVERLAY),
        "uap_sha": uap_sha,
        "directory_ledger_sha": args.directory_ledger_sha,
        "publication_id": args.publication_id,
        "publication_sequence": args.publication_sequence,
        "publication_snapshot_digest": args.publication_snapshot_digest,
        "publication_source_commit": args.publication_source_commit,
    }


def process(args: argparse.Namespace) -> None:
    uap_sha = require_uap_sha(args.uap_sha)
    args.directory_ledger_sha = require_directory_ledger_sha(
        args.directory_ledger_sha, uap_sha=uap_sha,
    )
    runtime_body = args.runtime.read_bytes()
    policy_body = args.policy.read_bytes()
    runtime = json.loads(runtime_body)
    policy = json.loads(policy_body)
    if runtime_body != canonical_json(runtime) or policy_body != canonical_json(policy):
        raise ValueError("runtime and policy inputs must be canonical evidence bytes")
    if bool(args.output) == bool(args.completed):
        raise ValueError("select exactly one of --output or --completed")
    expected_runtime_schema = {1: 4, 2: 5}[args.schema_version]
    if type(runtime.get("schema_version")) is not int or runtime["schema_version"] != expected_runtime_schema:
        raise ValueError(
            f"readiness v{args.schema_version} requires launch evidence v{expected_runtime_schema}"
        )
    if type(policy.get("schema_version")) is not int or policy["schema_version"] != args.schema_version:
        raise ValueError(
            f"readiness v{args.schema_version} requires source-policy evidence v{args.schema_version}"
        )
    identity = identities(args, uap_sha)
    if args.completed:
        completed_body = args.completed.read_bytes()
        completed = json.loads(completed_body)
        if completed_body != canonical_json(completed):
            raise ValueError("completed readiness must use canonical bytes")
        if type(completed.get("schema_version")) is not int or completed["schema_version"] != args.schema_version:
            raise ValueError("completed readiness schema version differs from the requested contract")
        call_versioned(
            validate_completed_readiness, completed, runtime, policy,
            schema_version=args.schema_version, identity=identity,
        )
    else:
        if args.output.exists() or args.output.is_symlink():
            raise ValueError("readiness output must not already exist")
        value = call_versioned(
            build_readiness_envelope, runtime, policy,
            schema_version=args.schema_version, identity=identity,
        )
        expected_runtime_results = 15 if args.schema_version == 1 else 16
        if not (
            value.get("schema_version") == args.schema_version
            and value.get("runtime_results") == expected_runtime_results
        ):
            raise ValueError("readiness builder returned a cross-version runtime result contract")
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_bytes(canonical_json(value))


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--runtime", type=Path, required=True)
    parser.add_argument("--policy", type=Path, required=True)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--completed", type=Path)
    parser.add_argument("--uap-sha", required=True)
    parser.add_argument("--directory-ledger-sha", required=True)
    parser.add_argument("--publication-id", required=True)
    parser.add_argument("--publication-sequence", type=int, required=True)
    parser.add_argument("--publication-snapshot-digest", required=True)
    parser.add_argument("--publication-source-commit", required=True)
    parser.add_argument("--schema-version", type=int, choices=(1, 2), default=1)
    args = parser.parse_args()
    process(args)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
