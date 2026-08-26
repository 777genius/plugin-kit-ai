"""Bounded client and verifier for the narrow root-owned signing socket."""

from __future__ import annotations

import base64
import binascii
import json
import socket
import struct
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey

from .canonical import canonical_json, signed_payload, validate_artifacts
from .schema_validation import validate_artifact_schemas

MAX_SIGN_BYTES = 8 << 20


class CacheExpiredError(ValueError):
    """A valid cache entry outside the 30-minute retry window."""


class SocketSigner:
    def __init__(self, socket_path: Path, public_key_base64: str, key_id: str):
        self.socket_path, self.key_id = socket_path, key_id
        try:
            raw = base64.b64decode(public_key_base64, validate=True)
        except (ValueError, binascii.Error):
            raise ValueError("observer public key is invalid") from None
        if len(raw) != 32:
            raise ValueError("observer public key is invalid")
        self.public_key = Ed25519PublicKey.from_public_bytes(raw)

    def sign(self, bundle_without_signature: dict[str, Any], *, deadline: float | None = None) -> str:
        payload = signed_payload(bundle_without_signature)
        if len(payload) > MAX_SIGN_BYTES:
            raise ValueError("observer bundle exceeds signing size bound")
        remaining = 10.0 if deadline is None else min(10.0, deadline - time.monotonic())
        if remaining <= 0:
            raise TimeoutError("observer deadline expired before signing")
        with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as client:
            client.settimeout(remaining)
            client.connect(str(self.socket_path))
            if _peer_uid(client) != 0:
                raise ValueError("signer socket peer is not trusted")
            try:
                client.sendall(struct.pack("!I", len(payload)) + payload)
                length = struct.unpack("!I", _read_exact(client, 4, deadline))[0]
            except (BrokenPipeError, ConnectionResetError):
                raise ValueError("signer returned an invalid response") from None
            if length > 256:
                raise ValueError("signer response exceeds size bound")
            response = json.loads(_read_exact(client, length, deadline))
        if not isinstance(response, dict) or set(response) != {"signature"}:
            raise ValueError("signer returned an invalid response")
        signature = response["signature"]
        try:
            raw_signature = base64.b64decode(signature, validate=True)
            self.public_key.verify(raw_signature, payload)
        except Exception:
            raise ValueError("signer returned an invalid signature") from None
        return signature

    def verify_cached(self, encoded: bytes, *, challenge: str, now: float) -> dict[str, Any]:
        try:
            bundle = json.loads(encoded)
        except (ValueError, UnicodeError):
            raise ValueError("cached observer response is invalid") from None
        required = {"schema_version", "challenge", "signed_at", "key_id", "artifacts", "signature"}
        if not isinstance(bundle, dict) or set(bundle) != required or type(bundle.get("schema_version")) is not int or bundle.get("schema_version") != 1:
            raise ValueError("cached observer response is not canonical")
        if canonical_json(bundle) != encoded or bundle.get("challenge") != challenge or bundle.get("key_id") != self.key_id:
            raise ValueError("cached observer response identity is invalid")
        try:
            signed_at = datetime.fromisoformat(str(bundle["signed_at"]).replace("Z", "+00:00"))
        except ValueError:
            raise ValueError("cached observer timestamp is invalid") from None
        if signed_at.tzinfo is None or now - signed_at.timestamp() > 1800 or signed_at.timestamp() > now + 120:
            raise CacheExpiredError("cached observer response is stale")
        artifacts = validate_artifacts(bundle["artifacts"])
        validate_artifact_schemas(artifacts, challenge=challenge)
        unsigned = {key: value for key, value in bundle.items() if key != "signature"}
        try:
            signature = base64.b64decode(bundle["signature"], validate=True)
            self.public_key.verify(signature, signed_payload(unsigned))
        except Exception:
            raise ValueError("cached observer signature is invalid") from None
        return bundle


def _read_exact(stream: socket.socket, size: int, deadline: float | None) -> bytes:
    chunks = []
    while size:
        if deadline is not None:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError("observer signer exceeded end-to-end deadline")
            stream.settimeout(remaining)
        chunk = stream.recv(size)
        if not chunk:
            raise ValueError("signer closed the connection")
        chunks.append(chunk)
        size -= len(chunk)
    return b"".join(chunks)


def _peer_uid(stream: socket.socket) -> int:
    if not hasattr(socket, "SO_PEERCRED"):
        raise ValueError("signer client requires Linux SO_PEERCRED")
    _, uid, _ = struct.unpack("3i", stream.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12))
    return uid
