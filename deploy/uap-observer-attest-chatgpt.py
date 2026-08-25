#!/usr/bin/env python3
"""Create one sanitized human ChatGPT attestation; never accepts free-form text."""

from __future__ import annotations

import argparse
import grp
import json
import os
import re
from datetime import datetime, timedelta, timezone
from pathlib import Path

OUTPUT_ROOT = Path("/var/lib/uap-observer-human/pending")
APP_ID = re.compile(r"^plugin_asdk_app_[a-f0-9]{32}$")
HEX64 = re.compile(r"^[a-f0-9]{64}$")


def yes(value: str) -> bool:
    if value not in {"yes", "no"}:
        raise argparse.ArgumentTypeError("expected yes or no")
    return value == "yes"


def trusted_output_directory(group_id: int) -> int:
    descriptor = os.open("/", os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for index, component in enumerate(OUTPUT_ROOT.parts[1:]):
            child = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=descriptor)
            os.close(descriptor)
            descriptor = child
            info = os.fstat(descriptor)
            final = index == len(OUTPUT_ROOT.parts[1:]) - 1
            expected_mode = 0o750 if final else 0o755
            expected_group = group_id if final else 0
            if info.st_uid != 0 or info.st_gid != expected_group or (info.st_mode & 0o777) != expected_mode:
                raise SystemExit("attestation directory ownership or mode differs")
        return descriptor
    except Exception:
        os.close(descriptor)
        raise


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--challenge", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--run-attempt", required=True)
    parser.add_argument("--app-id", required=True)
    parser.add_argument("--request-digest", required=True)
    parser.add_argument("--consent", type=yes, required=True)
    parser.add_argument("--ui-activation", type=yes, required=True)
    parser.add_argument("--runtime-observed", type=yes, required=True)
    parser.add_argument("--read-only", type=yes, required=True)
    parser.add_argument("--no-secrets", type=yes, required=True)
    parser.add_argument("--no-real-project", type=yes, required=True)
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise SystemExit("attestation helper must run as root")
    if not HEX64.fullmatch(args.challenge) or not args.run_id.isdigit() or not args.run_attempt.isdigit() or not APP_ID.fullmatch(args.app_id) or not re.fullmatch(r"sha256:[a-f0-9]{64}", args.request_digest):
        raise SystemExit("attestation identity is invalid")
    observed = datetime.now(timezone.utc)
    value = {
        "schema_version": 1, "challenge": args.challenge, "run_id": args.run_id,
        "run_attempt": args.run_attempt, "app_id": args.app_id,
        "consent": args.consent, "ui_activation": args.ui_activation,
        "runtime_observed": args.runtime_observed, "read_only": args.read_only,
        "no_secrets": args.no_secrets, "no_real_project": args.no_real_project,
        "request_digest": args.request_digest, "mcp_url": "https://docs.mcp.cloudflare.com/mcp",
        "observed_at": observed.isoformat().replace("+00:00", "Z"),
        "expires_at": (observed + timedelta(minutes=10)).isoformat().replace("+00:00", "Z"),
    }
    encoded = json.dumps(value, sort_keys=True, separators=(",", ":")).encode()
    group_id = grp.getgrnam("uap-observer-control").gr_gid
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
    print("sanitized ChatGPT attestation created")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
