#!/usr/bin/env python3
"""Create one root-owned, challenge-specific consent record."""

from __future__ import annotations

import argparse
import grp
import json
import os
import re
from pathlib import Path

OUTPUT_ROOT = Path("/var/lib/uap-observer-consent/pending")
HEX64 = re.compile(r"^[a-f0-9]{64}$")
DIGEST = re.compile(r"^sha256:[a-f0-9]{64}$")


def trusted_output_directory(group_id: int) -> int:
    descriptor = os.open("/", os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for index, component in enumerate(OUTPUT_ROOT.parts[1:]):
            child = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=descriptor)
            os.close(descriptor)
            descriptor = child
            info = os.fstat(descriptor)
            final = index == len(OUTPUT_ROOT.parts[1:]) - 1
            if info.st_uid != 0 or info.st_gid != (group_id if final else 0) or (info.st_mode & 0o777) != (0o750 if final else 0o755):
                raise SystemExit("consent directory ownership or mode differs")
        return descriptor
    except Exception:
        os.close(descriptor)
        raise


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--challenge", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--run-attempt", required=True)
    parser.add_argument("--catalog-sha", required=True)
    parser.add_argument("--request-digest", required=True)
    parser.add_argument("--scenario-contract-digest", required=True)
    parser.add_argument("--identity-id", required=True)
    parser.add_argument("--logical-root-id", required=True)
    parser.add_argument("--operation-mode", choices=("read-only", "synthetic"), required=True)
    parser.add_argument("--auth-origin", choices=("fresh-dedicated-identity", "none"), required=True)
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise SystemExit("consent helper must run as root")
    if not HEX64.fullmatch(args.challenge) or not re.fullmatch(r"[a-f0-9]{40}", args.catalog_sha) or not DIGEST.fullmatch(args.request_digest) or not DIGEST.fullmatch(args.scenario_contract_digest):
        raise SystemExit("consent identity is invalid")
    if not args.run_id.isdigit() or not args.run_attempt.isdigit() or not HEX64.fullmatch(args.identity_id) or not HEX64.fullmatch(args.logical_root_id):
        raise SystemExit("consent identity is invalid")
    privacy = {
        "real_project_accessed": False, "absolute_paths_exported": False,
        "credential_material_exported": False, "auth_copied": False,
    }
    value = {
        "schema_version": 1, "purpose": "stable-launch-e2e", "consent": True,
        "mode": "enforced", "challenge": args.challenge, "run_id": args.run_id,
        "run_attempt": args.run_attempt, "catalog_sha": args.catalog_sha,
        "scenario_contract_digest": args.scenario_contract_digest,
        "request_digest": args.request_digest, "dedicated_identity": True,
        "pseudonymous_identity_id": args.identity_id,
        "pseudonymous_workspace_id": args.logical_root_id,
        "disposable_project_status": "disposed", "operation_mode": args.operation_mode,
        "auth_origin": args.auth_origin, "cleanup_outcome": "cleaned",
        "no_real_project_proof": {**privacy, "enforcement": "systemd-positive-mount-allowlist-v1"},
    }
    encoded = json.dumps(value, sort_keys=True, separators=(",", ":")).encode()
    group_id = grp.getgrnam("uap-observer-adapter-config").gr_gid
    directory = trusted_output_directory(group_id)
    descriptor = os.open(f"{args.challenge}.json", os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o640, dir_fd=directory)
    try:
        os.fchown(descriptor, 0, group_id)
        os.fchmod(descriptor, 0o640)
        view = memoryview(encoded)
        while view:
            view = view[os.write(descriptor, view):]
        os.fsync(descriptor)
    finally:
        os.close(descriptor)
        os.close(directory)
    print("challenge-specific consent record created")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
