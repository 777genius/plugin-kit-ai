#!/usr/bin/env python3
"""Reacquire and verify one bounded production Discovery publication."""

from __future__ import annotations

import argparse
import json
import sys
import tempfile
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from scripts.directory_publication import (
    PublicationError,
    canonical_json,
    format_timestamp,
    parse_json_bytes,
    parse_timestamp,
    read_bytes_bounded,
    sha256_digest,
    validate_with_schema,
)
from scripts.discovery_publication import (
    LATEST_SCHEMA,
    MAX_LATEST_BYTES,
    load_latest_portably,
)
from scripts.sequence_boundaries import parse_public_sequence, require_public_sequence


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):  # noqa: ANN001
        raise PublicationError(f"Discovery origin returned forbidden redirect HTTP {code}")


def fetch(opener: urllib.request.OpenerDirector, url: str, maximum: int) -> bytes:
    request = urllib.request.Request(url, headers={
        "Accept": "application/json",
        "Cache-Control": "no-cache",
        "User-Agent": "universal-agent-plugins-discovery-observer/1",
    })
    try:
        with opener.open(request, timeout=20) as response:
            if response.status != 200:
                raise PublicationError(f"Discovery origin returned HTTP {response.status}")
            body = response.read(maximum + 1)
    except PublicationError:
        raise
    except (OSError, urllib.error.URLError) as error:
        raise PublicationError(f"Discovery origin fetch failed: {error}") from error
    if len(body) > maximum:
        raise PublicationError(f"Discovery origin response exceeds {maximum} bytes")
    return body


def observe_once(origin: str, trusted_keys: Path, minimum_sequence: int) -> dict[str, object]:
    minimum_sequence = require_public_sequence(minimum_sequence, "minimum sequence")
    opener = urllib.request.build_opener(NoRedirect())
    origin = origin.rstrip("/")
    latest_body = fetch(opener, origin + "/latest.json", MAX_LATEST_BYTES)
    latest = parse_json_bytes(latest_body, "Discovery latest pointer", max_bytes=MAX_LATEST_BYTES)
    if canonical_json(latest) != latest_body:
        raise PublicationError("Discovery latest pointer is not canonical JSON")
    validate_with_schema(latest, LATEST_SCHEMA)
    if latest["sequence"] < minimum_sequence:
        raise PublicationError("Discovery production sequence is below the required floor")

    with tempfile.TemporaryDirectory(prefix="uap-discovery-observe-") as temporary:
        feed = Path(temporary)
        (feed / "latest.json").write_bytes(latest_body)
        for field, maximum_field in (
            ("snapshot_path", "snapshot_max_bytes"),
            ("envelope_path", "envelope_max_bytes"),
            ("search_path", "search_max_bytes"),
        ):
            relative = latest[field]
            destination = feed / relative
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_bytes(fetch(opener, origin + "/" + relative, latest["fetch_contract"][maximum_field]))
        loaded = load_latest_portably(feed, trusted_keys)
        if loaded is None:
            raise PublicationError("Discovery latest pointer disappeared")
        snapshot, _pointer = loaded
        now = datetime.now(timezone.utc)
        if now < parse_timestamp(snapshot["generated_at"], "generated_at"):
            raise PublicationError("Discovery production snapshot is not active yet")
        if now >= parse_timestamp(snapshot["expires_at"], "expires_at"):
            raise PublicationError("Discovery production snapshot is expired")
        envelope = parse_json_bytes(
            read_bytes_bounded(feed / latest["envelope_path"], latest["fetch_contract"]["envelope_max_bytes"]),
            "Discovery envelope",
            max_bytes=latest["fetch_contract"]["envelope_max_bytes"],
        )
        return {
            "observation_schema_version": 1,
            "origin": origin,
            "sequence": snapshot["sequence"],
            "publication_id": snapshot["publication_id"],
            "source_commit": snapshot["source_commit"],
            "snapshot_digest": envelope["snapshot_digest"],
            "record_count": len(snapshot["records"]),
            "observed_at": format_timestamp(now),
            "latest_digest": sha256_digest(latest_body),
        }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--origin", required=True)
    parser.add_argument("--trusted-keys", required=True, type=Path)
    parser.add_argument("--minimum-sequence", required=True, type=parse_public_sequence)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--attempts", type=int, default=6)
    args = parser.parse_args()
    if not args.origin.startswith("https://") or not 1 <= args.attempts <= 6:
        print("Discovery observation failed: invalid observer arguments", file=sys.stderr)
        return 2
    error: Exception | None = None
    for attempt in range(args.attempts):
        try:
            observation = observe_once(args.origin, args.trusted_keys, args.minimum_sequence)
            args.output.parent.mkdir(parents=True, exist_ok=True)
            args.output.write_bytes(canonical_json(observation))
            print(f"observed Discovery sequence {observation['sequence']} with {observation['record_count']} records")
            return 0
        except (PublicationError, OSError, ValueError, json.JSONDecodeError) as current:
            error = current
            if attempt + 1 < args.attempts:
                time.sleep(5)
    print(f"Discovery observation failed after {args.attempts} attempts: {error}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
