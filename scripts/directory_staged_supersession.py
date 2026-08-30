#!/usr/bin/env python3
"""Require a superseding Directory candidate to change signed source payload."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any


PAYLOAD_FIELDS = ("products", "distributions", "evidence", "revocations")
MAX_DOCUMENT_BYTES = 4 << 20


class SupersessionError(ValueError):
    """The requested staged supersession is not materially different."""


def load_object(path: Path, label: str) -> dict[str, Any]:
    body = path.read_bytes()
    if len(body) > MAX_DOCUMENT_BYTES:
        raise SupersessionError(f"{label} exceeds {MAX_DOCUMENT_BYTES} bytes")
    try:
        value = json.loads(body)
    except json.JSONDecodeError as error:
        raise SupersessionError(f"{label} is not valid JSON") from error
    if not isinstance(value, dict):
        raise SupersessionError(f"{label} must be an object")
    return value


def require_material_payload_change(
    candidate: dict[str, Any], staged_snapshot: dict[str, Any],
) -> None:
    for field in PAYLOAD_FIELDS:
        if field not in candidate or field not in staged_snapshot:
            raise SupersessionError(f"{field} is missing from the supersession comparison")
    if all(candidate[field] == staged_snapshot[field] for field in PAYLOAD_FIELDS):
        raise SupersessionError(
            "superseding candidate does not change products, distributions, evidence, or revocations"
        )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--candidate", type=Path, required=True)
    parser.add_argument("--staged-snapshot", type=Path, required=True)
    args = parser.parse_args()
    try:
        require_material_payload_change(
            load_object(args.candidate, "candidate"),
            load_object(args.staged_snapshot, "staged snapshot"),
        )
    except (OSError, SupersessionError) as error:
        print(f"directory-staged-supersession: {error}", file=sys.stderr)
        return 1
    print("staged Directory payload is materially superseded")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
