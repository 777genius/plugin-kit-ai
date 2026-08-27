#!/usr/bin/env python3
"""Generate one canonical digest manifest for an extracted client bundle."""

from __future__ import annotations

import argparse
import os
import tempfile
from pathlib import Path

from observer.client_bundle import canonical_json, inventory_bundle


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("bundle", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    if args.output.exists():
        raise SystemExit("output already exists")
    body = canonical_json(inventory_bundle(args.bundle.resolve(strict=True)))
    descriptor, temporary = tempfile.mkstemp(prefix=f".{args.output.name}.", dir=args.output.parent)
    try:
        os.fchmod(descriptor, 0o644)
        with os.fdopen(descriptor, "wb", closefd=True) as stream:
            stream.write(body)
            stream.flush()
            os.fsync(stream.fileno())
        descriptor = -1
        os.link(temporary, args.output, follow_symlinks=False)
        os.unlink(temporary)
        directory = os.open(args.output.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        if os.path.exists(temporary):
            os.unlink(temporary)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
