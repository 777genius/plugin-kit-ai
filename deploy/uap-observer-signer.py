#!/usr/bin/env python3
"""Root-owned narrow Ed25519 signer for canonical observer bundles."""

from __future__ import annotations

import base64
import binascii
import grp
import json
import math
import os
import pwd
import socket
import socketserver
import stat
import struct
import time
from datetime import datetime
from pathlib import Path

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

from observer.canonical import SIGNATURE_DOMAIN, canonical_json, validate_artifacts
from observer.schema_validation import validate_artifact_schemas

MAX_PAYLOAD = 8 << 20
SOCKET_PATH = Path("/run/uap-observer-signer/sign.sock")
KEY_PATH = Path("/etc/uap-observer-ed25519.key")
KEY_ID = "uap-stable-launch-2026-08"


def load_key(path: Path) -> Ed25519PrivateKey:
    descriptor = os.open("/", os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for component in path.parent.parts[1:]:
            child = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=descriptor)
            os.close(descriptor)
            descriptor = child
            parent = os.fstat(descriptor)
            if parent.st_uid != 0 or parent.st_mode & 0o022:
                raise ValueError("signing key directory is not trusted")
        key_descriptor = os.open(path.name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=descriptor)
        os.close(descriptor)
        descriptor = key_descriptor
        opened = os.fstat(descriptor)
        if not stat.S_ISREG(opened.st_mode) or opened.st_uid != 0 or stat.S_IMODE(opened.st_mode) != 0o600 or opened.st_nlink != 1:
            raise ValueError("signing key must be a root-owned 0600 unlinked regular file")
        with os.fdopen(descriptor, "rb") as stream:
            descriptor = -1
            encoded = stream.read(257)
        if len(encoded) > 256:
            raise ValueError("signing key file exceeds size bound")
        raw = base64.b64decode(encoded.strip(), validate=True)
    except (ValueError, binascii.Error):
        raise ValueError("signing key must be canonical base64") from None
    finally:
        if descriptor >= 0:
            os.close(descriptor)
    if len(raw) != 32:
        raise ValueError("signing key has invalid length")
    return Ed25519PrivateKey.from_private_bytes(raw)


def validate_payload(payload: bytes, *, key_id: str) -> None:
    if not payload.startswith(SIGNATURE_DOMAIN):
        raise ValueError("invalid signature domain")
    encoded = payload[len(SIGNATURE_DOMAIN):]
    def object_from_pairs(pairs: list[tuple[str, object]]) -> dict[str, object]:
        result: dict[str, object] = {}
        folded: set[str] = set()
        for name, child in pairs:
            normalized = name.casefold()
            if name in result or normalized in folded:
                raise ValueError("duplicate or case-confusable JSON object member")
            result[name] = child
            folded.add(normalized)
        return result

    def reject_constant(value: str) -> object:
        raise ValueError(f"non-finite JSON number: {value}")

    def finite_float(value: str) -> float:
        decoded = float(value)
        if not math.isfinite(decoded):
            reject_constant(value)
        return decoded

    try:
        value = json.loads(
            encoded, object_pairs_hook=object_from_pairs,
            parse_constant=reject_constant, parse_float=finite_float,
        )
    except (UnicodeDecodeError, json.JSONDecodeError, ValueError, OverflowError) as error:
        raise ValueError("invalid unsigned bundle JSON") from error
    required = {"schema_version", "challenge", "signed_at", "key_id", "artifacts"}
    if not isinstance(value, dict) or set(value) != required or type(value.get("schema_version")) is not int or value.get("schema_version") != 1:
        raise ValueError("invalid unsigned bundle")
    if not isinstance(value.get("challenge"), str) or not all(child in "0123456789abcdef" for child in value["challenge"]) or len(value["challenge"]) != 64:
        raise ValueError("invalid unsigned bundle challenge")
    if value.get("key_id") != key_id:
        raise ValueError("invalid unsigned bundle key id")
    signed_at = datetime.fromisoformat(str(value["signed_at"]).replace("Z", "+00:00"))
    if signed_at.tzinfo is None or abs(signed_at.timestamp() - time.time()) > 120:
        raise ValueError("invalid unsigned bundle timestamp")
    artifacts = validate_artifacts(value.get("artifacts"))
    validate_artifact_schemas(artifacts, challenge=value["challenge"])
    if encoded != canonical_json(value):
        raise ValueError("unsigned bundle is not canonical")


def read_exact(stream: socket.socket, size: int) -> bytes:
    chunks = []
    while size:
        chunk = stream.recv(size)
        if not chunk:
            raise ValueError("truncated signer request")
        chunks.append(chunk)
        size -= len(chunk)
    return b"".join(chunks)


def peer_uid(stream: socket.socket) -> int:
    if not hasattr(socket, "SO_PEERCRED"):
        raise ValueError("signer requires Linux SO_PEERCRED")
    _, uid, _ = struct.unpack("3i", stream.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12))
    return uid


class Handler(socketserver.BaseRequestHandler):
    def handle(self) -> None:
        try:
            self.request.settimeout(10)
            if peer_uid(self.request) != self.server.allowed_uid:  # type: ignore[attr-defined]
                raise ValueError("signing peer is not authorized")
            length = struct.unpack("!I", read_exact(self.request, 4))[0]
            if not 1 <= length <= MAX_PAYLOAD:
                raise ValueError("invalid signer request length")
            payload = read_exact(self.request, length)
            validate_payload(payload, key_id=self.server.key_id)  # type: ignore[attr-defined]
            signature = base64.b64encode(self.server.private_key.sign(payload)).decode()  # type: ignore[attr-defined]
            response = json.dumps({"signature": signature}, separators=(",", ":")).encode()
        except Exception:
            response = json.dumps({"error": "signing request rejected"}, separators=(",", ":")).encode()
        self.request.sendall(struct.pack("!I", len(response)) + response)


def main() -> int:
    private_key = load_key(KEY_PATH)
    allowed_uid = pwd.getpwnam("uap-observer").pw_uid
    if SOCKET_PATH.exists() or SOCKET_PATH.is_symlink():
        raise ValueError("signer socket path must not preexist")
    with socketserver.UnixStreamServer(str(SOCKET_PATH), Handler) as server:
        server.private_key = private_key  # type: ignore[attr-defined]
        server.key_id = KEY_ID  # type: ignore[attr-defined]
        server.allowed_uid = allowed_uid  # type: ignore[attr-defined]
        if os.getgid() != grp.getgrnam("uap-observer-signer-ipc").gr_gid:
            raise ValueError("signer must run with the dedicated observer group")
        os.chmod(SOCKET_PATH, 0o660, follow_symlinks=False)
        server.serve_forever()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
