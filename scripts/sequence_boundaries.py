#!/usr/bin/env python3
"""Exact public sequence boundaries shared by publishers and consumers."""

from __future__ import annotations

import argparse
import re
import sys
from typing import Any


JSON_SAFE_INTEGER_MAX = 9_007_199_254_740_991
_CANONICAL_SEQUENCE = re.compile(r"[1-9][0-9]*\Z", re.ASCII)
_PADDED_SEQUENCE = re.compile(r"[0-9]{20}\Z", re.ASCII)


def require_public_sequence(value: Any, label: str = "sequence") -> int:
    """Return an ordinary safe positive integer, rejecting bool and aliases."""
    if type(value) is not int or not 1 <= value <= JSON_SAFE_INTEGER_MAX:
        raise ValueError(f"{label} must be an exact integer from 1 through {JSON_SAFE_INTEGER_MAX}")
    return value


def parse_public_sequence(text: str, label: str = "sequence") -> int:
    """Parse the one canonical decimal CLI spelling of a public sequence."""
    if not isinstance(text, str) or _CANONICAL_SEQUENCE.fullmatch(text) is None:
        raise ValueError(f"{label} must be canonical positive decimal text")
    return require_public_sequence(int(text), label)


def next_public_sequence(current: int | None) -> int:
    """Return sequence 1 for initialization or a bounded exact successor."""
    if current is None:
        return 1
    current = require_public_sequence(current, "current sequence")
    if current == JSON_SAFE_INTEGER_MAX:
        raise ValueError("public safe-integer range is exhausted")
    return current + 1


def parse_padded_public_sequence(text: str, label: str = "padded sequence") -> int:
    """Parse the exact fixed-width spelling used by immutable sequence tags."""
    if not isinstance(text, str) or _PADDED_SEQUENCE.fullmatch(text) is None:
        raise ValueError(f"{label} must be exactly 20 decimal digits")
    value = require_public_sequence(int(text), label)
    if text != f"{value:020d}":
        raise ValueError(f"{label} is not canonically padded")
    return value


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)
    validate = subparsers.add_parser("validate")
    validate.add_argument("sequence", type=parse_public_sequence)
    successor = subparsers.add_parser("successor")
    successor.add_argument("--current", type=parse_public_sequence)
    tag_value = subparsers.add_parser("tag-value")
    tag_value.add_argument("sequence", type=parse_padded_public_sequence)
    args = parser.parse_args()
    try:
        if args.command == "validate":
            print(require_public_sequence(args.sequence))
        elif args.command == "successor":
            print(next_public_sequence(args.current))
        else:
            print(require_public_sequence(args.sequence))
        return 0
    except ValueError as error:
        print(f"sequence boundary failure: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
