"""Bounded client for the separately privileged fixed runner service."""

from __future__ import annotations

import hashlib
import json
import os
import socket
import struct
import time
from pathlib import Path
from typing import Any

from .canonical import canonical_json, validate_artifacts
from .secure_files import read_owned_regular

MAX_RUNNER_MESSAGE = 8 << 20
MAX_RUNNER_SOURCE = 1 << 20


class SocketRunner:
    def __init__(
        self, socket_path: Path, source_path: Path, source_digest: str,
        timeout_seconds: int, *, runner_user: str = "root",
        enforce_root_ownership: bool = True,
    ):
        self.socket_path, self.timeout_seconds = socket_path, timeout_seconds
        del runner_user
        # systemd owns the activated listener; its root peer plus the root-owned
        # /run socket path anchors the server identity. The server verifies the
        # observer UID reciprocally with SO_PEERCRED before reading a request.
        self.expected_uid = 0 if enforce_root_ownership else os.geteuid()
        owner = 0 if enforce_root_ownership else source_path.stat().st_uid
        source = read_owned_regular(
            source_path, MAX_RUNNER_SOURCE, owner_uid=owner, executable=True,
            exact_mode=0o755 if enforce_root_ownership else None,
            group_gid=0 if enforce_root_ownership else None,
        )
        actual = "sha256:" + hashlib.sha256(source).hexdigest()
        if actual != source_digest:
            raise ValueError("installed reviewed runner digest differs from configuration")

    def run(self, run_dir: Path, context: dict[str, Any], *, deadline: float | None = None) -> dict[str, Any]:
        del run_dir  # The runner UID cannot access observer-owned state directories.
        payload = canonical_json({"operation": "execute", "context": context})
        if len(payload) > MAX_RUNNER_MESSAGE:
            raise ValueError("runner request exceeds size bound")
        remaining = self.timeout_seconds
        if deadline is not None:
            remaining = min(remaining, deadline - time.monotonic())
        if remaining <= 0:
            raise TimeoutError("observer deadline expired before runner execution")
        try:
            with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as client:
                client.settimeout(remaining)
                client.connect(str(self.socket_path))
                if _peer_uid(client) != self.expected_uid:
                    raise ValueError("runner socket peer is not trusted")
                client.sendall(struct.pack("!I", len(payload)) + payload)
                length = struct.unpack("!I", _read_exact(client, 4, deadline))[0]
                if length > MAX_RUNNER_MESSAGE:
                    raise ValueError("runner response exceeds size bound")
                response = json.loads(_read_exact(client, length, deadline))
        except (OSError, TimeoutError, socket.timeout, json.JSONDecodeError, UnicodeError):
            raise ValueError("reviewed observer runner failed") from None
        if not isinstance(response, dict) or set(response) != {"artifacts"}:
            raise ValueError("reviewed observer runner returned an invalid response")
        return validate_artifacts(response["artifacts"])

    def transaction(self, challenge: str, action: str, *, deadline: float | None = None) -> None:
        if action not in {"commit", "rollback"}:
            raise ValueError("runner transaction action is invalid")
        payload = canonical_json({"operation": action, "challenge": challenge})
        remaining = self.timeout_seconds if deadline is None else min(self.timeout_seconds, deadline - time.monotonic())
        if remaining <= 0:
            raise TimeoutError("observer deadline expired before runner transaction")
        try:
            with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as client:
                client.settimeout(remaining)
                client.connect(str(self.socket_path))
                if _peer_uid(client) != self.expected_uid:
                    raise ValueError("runner socket peer is not trusted")
                client.sendall(struct.pack("!I", len(payload)) + payload)
                length = struct.unpack("!I", _read_exact(client, 4, deadline))[0]
                response = json.loads(_read_exact(client, length, deadline))
        except (OSError, TimeoutError, socket.timeout, json.JSONDecodeError, UnicodeError):
            raise ValueError("reviewed observer runner transaction failed") from None
        if response != {"transaction": action}:
            raise ValueError("reviewed observer runner transaction was not acknowledged")


def _read_exact(stream: socket.socket, size: int, deadline: float | None) -> bytes:
    chunks = []
    while size:
        if deadline is not None:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError("observer runner exceeded end-to-end deadline")
            stream.settimeout(remaining)
        chunk = stream.recv(size)
        if not chunk:
            raise ValueError("runner closed the connection")
        chunks.append(chunk)
        size -= len(chunk)
    return b"".join(chunks)


def _peer_uid(stream: socket.socket) -> int:
    if not hasattr(socket, "SO_PEERCRED"):
        raise ValueError("runner client requires Linux SO_PEERCRED")
    _, uid, _ = struct.unpack("3i", stream.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12))
    return uid
