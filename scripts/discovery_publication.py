#!/usr/bin/env python3
"""Fail-closed contracts for the signed static Discovery Index feed."""

from __future__ import annotations

import base64
import json
import sys
from datetime import timedelta
from pathlib import Path
from typing import Any, Callable

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from scripts.directory_publication import (
    PublicationError,
    atomic_write,
    b64decode_exact,
    canonical_json,
    ed25519_sign,
    ed25519_verify,
    load_public_keys,
    parse_json_bytes,
    parse_timestamp,
    read_bytes_bounded,
    read_json,
    require,
    require_integer_const,
    sha256_digest,
    validate_with_schema,
)
from scripts.sequence_boundaries import JSON_SAFE_INTEGER_MAX, next_public_sequence


ROOT = Path(__file__).resolve().parents[1]
SNAPSHOT_SCHEMA = ROOT / "schemas" / "discovery-snapshot.schema.json"
ENVELOPE_SCHEMA = ROOT / "schemas" / "discovery-envelope.schema.json"
LATEST_SCHEMA = ROOT / "schemas" / "discovery-latest.schema.json"
SEARCH_SCHEMA = ROOT / "schemas" / "discovery-search.schema.json"
SIGNATURE_DOMAIN = b"UAP-DISCOVERY-INDEX-ED25519-V1\x00"
MAX_SNAPSHOT_BYTES = 16 << 20
MAX_SEARCH_BYTES = 10 << 20
MAX_ENVELOPE_BYTES = 16 << 10
MAX_LATEST_BYTES = 16 << 10

SignatureVerifier = Callable[[bytes, bytes, bytes], None]


def signature_message(snapshot: bytes) -> bytes:
    return SIGNATURE_DOMAIN + len(snapshot).to_bytes(8, "big") + snapshot


def validate_envelope(envelope: dict[str, Any]) -> None:
    validate_with_schema(envelope, ENVELOPE_SCHEMA)
    require_integer_const(envelope.get("envelope_schema_version"), 1, "Discovery envelope version is invalid")
    require(envelope.get("signature_domain") == "UAP-DISCOVERY-INDEX-ED25519-V1", "Discovery signature domain is invalid")
    b64decode_exact(envelope.get("signature"), 64, "Discovery signature")


def compact_record(record: dict[str, Any]) -> dict[str, Any]:
    return {key: value for key, value in record.items() if key not in {"author", "first_seen", "last_seen"}}


def validate_records(records: list[dict[str, Any]], generated_at: str) -> None:
    expected_order = sorted(records, key=lambda item: (
        item["repository"], item["package_path"].casefold(), item["slug"],
    ))
    require(records == expected_order, "Discovery records are not canonically ordered")
    identities: set[tuple[str, str]] = set()
    slugs: set[str] = set()
    generated = parse_timestamp(generated_at, "Discovery generated_at")
    for record in records:
        identity = (record["repository"].casefold(), record["package_path"].casefold())
        require(identity not in identities, f"duplicate Discovery package identity: {record['repository']}//{record['package_path']}")
        require(record["slug"] not in slugs, f"duplicate Discovery slug: {record['slug']}")
        expected_slug = "discovery:" + record["repository"].casefold()
        if record["package_path"]:
            expected_slug += "//" + record["package_path"]
        require(record["slug"] == expected_slug, f"non-canonical Discovery slug: {record['slug']}")
        first_seen = parse_timestamp(record["first_seen"], f"{record['slug']}.first_seen")
        last_seen = parse_timestamp(record["last_seen"], f"{record['slug']}.last_seen")
        require(first_seen <= last_seen <= generated, f"invalid Discovery seen interval: {record['slug']}")
        identities.add(identity)
        slugs.add(record["slug"])


def validate_search(search: dict[str, Any], snapshot: dict[str, Any]) -> bytes:
    validate_with_schema(search, SEARCH_SCHEMA)
    require(search["sequence"] == snapshot["sequence"], "Discovery search sequence mismatch")
    require(search["generated_at"] == snapshot["generated_at"], "Discovery search generation mismatch")
    expected = [compact_record(record) for record in snapshot["records"]]
    require(search["records"] == expected, "Discovery search projection does not match signed records")
    body = canonical_json(search)
    projection = snapshot["search_projection"]
    require(projection["record_count"] == len(search["records"]), "Discovery search record count mismatch")
    require(projection["digest"] == sha256_digest(body), "Discovery search digest mismatch")
    return body


def cryptography_ed25519_verify(public_key: bytes, message: bytes, signature: bytes) -> None:
    """Verify Ed25519 material portably in read-only consumers."""
    require(len(public_key) == 32, "Ed25519 public key must be 32 bytes")
    require(len(signature) == 64, "Ed25519 signature must be 64 bytes")
    try:
        from cryptography.exceptions import InvalidSignature
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
    except ImportError as error:
        raise PublicationError("cryptography is required for portable Discovery verification") from error
    try:
        Ed25519PublicKey.from_public_bytes(public_key).verify(signature, message)
    except (InvalidSignature, ValueError) as error:
        raise PublicationError("Ed25519 signature verification failed") from error


def verify_bundle(
    snapshot_body: bytes,
    envelope_body: bytes,
    search_body: bytes,
    trusted_keys_path: Path,
    *,
    signature_verifier: SignatureVerifier | None = None,
) -> dict[str, Any]:
    require(len(snapshot_body) <= MAX_SNAPSHOT_BYTES, "Discovery snapshot exceeds size limit")
    require(len(envelope_body) <= MAX_ENVELOPE_BYTES, "Discovery envelope exceeds size limit")
    require(len(search_body) <= MAX_SEARCH_BYTES, "Discovery search projection exceeds size limit")
    snapshot = parse_json_bytes(snapshot_body, "Discovery snapshot", max_bytes=MAX_SNAPSHOT_BYTES)
    envelope = parse_json_bytes(envelope_body, "Discovery envelope", max_bytes=MAX_ENVELOPE_BYTES)
    search = parse_json_bytes(search_body, "Discovery search projection", max_bytes=MAX_SEARCH_BYTES)
    require(canonical_json(snapshot) == snapshot_body, "Discovery snapshot is not canonical JSON")
    require(canonical_json(envelope) == envelope_body, "Discovery envelope is not canonical JSON")
    require(canonical_json(search) == search_body, "Discovery search projection is not canonical JSON")
    validate_with_schema(snapshot, SNAPSHOT_SCHEMA)
    validate_envelope(envelope)
    require(snapshot["complete"] is True, "partial Discovery snapshot cannot be published")
    generated = parse_timestamp(snapshot["generated_at"], "Discovery generated_at")
    expires = parse_timestamp(snapshot["expires_at"], "Discovery expires_at")
    require(generated < expires <= generated + timedelta(days=7), "Discovery validity interval is invalid")
    validate_records(snapshot["records"], snapshot["generated_at"])
    require(envelope["sequence"] == snapshot["sequence"], "Discovery envelope sequence mismatch")
    require(envelope["snapshot_digest"] == sha256_digest(snapshot_body), "Discovery snapshot digest mismatch")
    trusted = load_public_keys(trusted_keys_path)
    require(envelope["key_id"] in trusted, "unknown Discovery signing key")
    try:
        verifier = ed25519_verify if signature_verifier is None else signature_verifier
        verifier(
            trusted[envelope["key_id"]], signature_message(snapshot_body),
            b64decode_exact(envelope["signature"], 64, "Discovery signature"),
        )
    except PublicationError as error:
        raise PublicationError("invalid Discovery snapshot signature") from error
    validate_search(search, snapshot)
    return snapshot


def load_latest(
    feed: Path,
    trusted_keys: Path,
    *,
    signature_verifier: SignatureVerifier | None = None,
) -> tuple[dict[str, Any], dict[str, Any]] | None:
    latest_path = feed / "latest.json"
    if not latest_path.exists():
        return None
    latest_body = read_bytes_bounded(latest_path, MAX_LATEST_BYTES)
    latest = parse_json_bytes(latest_body, "Discovery latest pointer", max_bytes=MAX_LATEST_BYTES)
    require(canonical_json(latest) == latest_body, "Discovery latest pointer is not canonical JSON")
    validate_with_schema(latest, LATEST_SCHEMA)
    snapshot_path = feed / latest["snapshot_path"]
    envelope_path = feed / latest["envelope_path"]
    search_path = feed / latest["search_path"]
    snapshot = verify_bundle(
        read_bytes_bounded(snapshot_path, latest["fetch_contract"]["snapshot_max_bytes"]),
        read_bytes_bounded(envelope_path, latest["fetch_contract"]["envelope_max_bytes"]),
        read_bytes_bounded(search_path, latest["fetch_contract"]["search_max_bytes"]),
        trusted_keys,
        signature_verifier=signature_verifier,
    )
    require(snapshot["sequence"] == latest["sequence"], "Discovery latest sequence mismatch")
    require(snapshot["search_projection"]["path"] == latest["search_path"], "Discovery latest search path mismatch")
    return snapshot, latest


def load_latest_portably(feed: Path, trusted_keys: Path) -> tuple[dict[str, Any], dict[str, Any]] | None:
    """Load a feed without relying on the host OpenSSL CLI implementation."""
    return load_latest(feed, trusted_keys, signature_verifier=cryptography_ed25519_verify)


def publish(candidate_path: Path, feed: Path, trusted_keys: Path, private_seed: bytes, key_id: str,
            publication_id: str, source_commit: str, lifetime_days: int) -> dict[str, Any]:
    require(len(private_seed) == 32, "Discovery signing seed must be 32 bytes")
    require(1 <= lifetime_days <= 7, "Discovery lifetime must be between 1 and 7 days")
    candidate_body = read_bytes_bounded(candidate_path, MAX_SNAPSHOT_BYTES)
    candidate = parse_json_bytes(candidate_body, "Discovery candidate", max_bytes=MAX_SNAPSHOT_BYTES)
    require(canonical_json(candidate) == candidate_body, "Discovery candidate is not canonical JSON")
    require(set(candidate) == {
        "candidate_schema_version", "mode", "generated_at", "complete",
        "query_manifest_digest", "partitions", "records",
    }, "Discovery candidate fields are invalid")
    require(candidate.get("candidate_schema_version") == 1, "Discovery candidate version is invalid")
    require(candidate.get("complete") is True, "partial Discovery candidate cannot replace last-known-good")
    require(candidate.get("mode") in {"refresh", "discover", "reconcile"}, "Discovery candidate mode is invalid")
    require(isinstance(candidate.get("records"), list), "Discovery candidate records are invalid")
    previous = load_latest(feed, trusted_keys)
    try:
        sequence = next_public_sequence(None if previous is None else previous[0]["sequence"])
    except ValueError as error:
        raise PublicationError(str(error)) from error
    generated = parse_timestamp(candidate["generated_at"], "Discovery generated_at")
    stem = f"{sequence:020d}"
    search_path = f"search/{stem}.json"
    search = {
        "search_schema_version": 1,
        "sequence": sequence,
        "generated_at": candidate["generated_at"],
        "records": [compact_record(record) for record in candidate["records"]],
    }
    search_body = canonical_json(search)
    snapshot = {
        "discovery_schema_version": 1,
        "sequence": sequence,
        "publication_id": publication_id,
        "source_commit": source_commit,
        "generated_at": candidate["generated_at"],
        "expires_at": (generated + timedelta(days=lifetime_days)).isoformat().replace("+00:00", "Z"),
        "complete": True,
        "query_manifest_digest": candidate["query_manifest_digest"],
        "partitions": candidate["partitions"],
        "search_projection": {"path": search_path, "digest": sha256_digest(search_body), "record_count": len(search["records"])},
        "records": candidate["records"],
    }
    snapshot_body = canonical_json(snapshot)
    validate_with_schema(snapshot, SNAPSHOT_SCHEMA)
    validate_records(snapshot["records"], snapshot["generated_at"])
    validate_search(search, snapshot)
    signature = ed25519_sign(private_seed, signature_message(snapshot_body))
    envelope = {
        "envelope_schema_version": 1,
        "snapshot_schema_version": 1,
        "sequence": sequence,
        "key_id": key_id,
        "algorithm": "Ed25519",
        "signature_domain": "UAP-DISCOVERY-INDEX-ED25519-V1",
        "snapshot_digest": sha256_digest(snapshot_body),
        "signature": base64.b64encode(signature).decode("ascii"),
    }
    envelope_body = canonical_json(envelope)
    validate_envelope(envelope)
    snapshot_path = f"snapshots/{stem}.json"
    envelope_path = f"snapshots/{stem}.envelope.json"
    latest = {
        "pointer_schema_version": 1,
        "snapshot_schema_version": 1,
        "sequence": sequence,
        "snapshot_path": snapshot_path,
        "envelope_path": envelope_path,
        "search_path": search_path,
        "fetch_contract": {
            "max_redirects": 0,
            "latest_max_bytes": MAX_LATEST_BYTES,
            "snapshot_max_bytes": MAX_SNAPSHOT_BYTES,
            "envelope_max_bytes": MAX_ENVELOPE_BYTES,
            "search_max_bytes": MAX_SEARCH_BYTES,
            "retry_attempts": 3,
        },
    }
    validate_with_schema(latest, LATEST_SCHEMA)
    for relative in (snapshot_path, envelope_path, search_path):
        require(not (feed / relative).exists(), f"Discovery artifact already exists: {relative}")
    verify_bundle(snapshot_body, envelope_body, search_body, trusted_keys)
    atomic_write(feed / snapshot_path, snapshot_body)
    atomic_write(feed / envelope_path, envelope_body)
    atomic_write(feed / search_path, search_body)
    atomic_write(feed / "latest.json", canonical_json(latest))
    return {"sequence": sequence, "snapshot_digest": envelope["snapshot_digest"], "record_count": len(search["records"])}
