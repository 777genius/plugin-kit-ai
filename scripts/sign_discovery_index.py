#!/usr/bin/env python3
"""Append one complete Discovery candidate to the signed static feed."""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from scripts.directory_publication import PublicationError, ed25519_private_key
from scripts.discovery_publication import publish


SHA_RE = re.compile(r"^[0-9a-f]{40}$")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--candidate", type=Path, required=True)
    parser.add_argument("--feed", type=Path, required=True)
    parser.add_argument("--trusted-keys", type=Path, required=True)
    parser.add_argument("--publication-id", required=True)
    parser.add_argument("--source-commit", required=True)
    parser.add_argument("--key-id", required=True)
    parser.add_argument("--lifetime-days", type=int, default=3)
    parser.add_argument("--result", type=Path, required=True)
    args = parser.parse_args()
    try:
        if SHA_RE.fullmatch(args.source_commit) is None:
            raise PublicationError("source commit must be a full lowercase SHA")
        seed = ed25519_private_key(os.environ.get("DISCOVERY_ED25519_PRIVATE_KEY", ""))
        result = publish(
            args.candidate, args.feed, args.trusted_keys, seed, args.key_id,
            args.publication_id, args.source_commit, args.lifetime_days,
        )
        args.result.write_text(json.dumps(result, sort_keys=True, separators=(",", ":")) + "\n", encoding="utf-8")
        return 0
    except (PublicationError, OSError, ValueError) as error:
        print(f"Discovery signing failed: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
