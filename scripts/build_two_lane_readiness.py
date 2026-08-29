#!/usr/bin/env python3
"""Build or replay the canonical two-lane readiness envelope."""

from __future__ import annotations

import argparse
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
    args = parser.parse_args()
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
    if args.completed:
        completed_body = args.completed.read_bytes()
        completed = json.loads(completed_body)
        if completed_body != canonical_json(completed):
            raise ValueError("completed readiness must use canonical bytes")
        validate_completed_readiness(completed, runtime, policy, **identities(args, uap_sha))
    else:
        value = build_readiness_envelope(runtime, policy, **identities(args, uap_sha))
        if args.output.exists() or args.output.is_symlink():
            raise ValueError("readiness output must not already exist")
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_bytes(canonical_json(value))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
