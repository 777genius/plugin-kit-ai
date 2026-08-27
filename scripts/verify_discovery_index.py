#!/usr/bin/env python3
"""Verify the latest signed Discovery snapshot and compact search projection."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from scripts.directory_publication import PublicationError, parse_timestamp
from scripts.discovery_publication import load_latest


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--feed", type=Path, required=True)
    parser.add_argument("--trusted-keys", type=Path, required=True)
    parser.add_argument("--minimum-sequence", type=int, default=0)
    parser.add_argument("--now")
    args = parser.parse_args()
    try:
        loaded = load_latest(args.feed, args.trusted_keys)
        if loaded is None:
            raise PublicationError("Discovery latest pointer is missing")
        snapshot, _latest = loaded
        if snapshot["sequence"] < args.minimum_sequence:
            raise PublicationError("Discovery sequence is below the required floor")
        if args.now:
            now = parse_timestamp(args.now, "now")
            generated = parse_timestamp(snapshot["generated_at"], "generated_at")
            expires = parse_timestamp(snapshot["expires_at"], "expires_at")
            if now < generated or now >= expires:
                raise PublicationError("Discovery snapshot is not currently valid")
        print(f"verified Discovery sequence {snapshot['sequence']} with {len(snapshot['records'])} records")
        return 0
    except (PublicationError, OSError, ValueError) as error:
        print(f"Discovery verification failed: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
