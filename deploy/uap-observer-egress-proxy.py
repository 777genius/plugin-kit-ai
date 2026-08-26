#!/usr/bin/python3
"""Small socket-activated HTTPS CONNECT allowlist gateway.

The gateway deliberately understands only CONNECT to an exact configured
FQDN on port 443.  It never terminates TLS and never emits request or tunnel
metadata to logs.
"""

from __future__ import annotations

import argparse
import ipaddress
import json
import re
import selectors
import socket
import threading
import time
from pathlib import Path

MAX_CONFIG_BYTES = 8192
MAX_HOSTS = 32
MAX_HEADER_BYTES = 16384
MAX_CONNECTIONS = 32
HEADER_TIMEOUT = 5.0
CONNECT_TIMEOUT = 10.0
IDLE_TIMEOUT = 120.0
TUNNEL_TIMEOUT = 900.0
BUFFER_BYTES = 65536
HOST = re.compile(r"(?=.{1,253}\Z)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?")


class ProxyError(ValueError):
    pass


def validate_host(value: object) -> str:
    if not isinstance(value, str) or not HOST.fullmatch(value):
        raise ProxyError("allowlist host is not a canonical FQDN")
    if value != value.lower() or value.endswith(".") or "*" in value:
        raise ProxyError("allowlist host is not canonical")
    try:
        ipaddress.ip_address(value)
    except ValueError:
        return value
    raise ProxyError("IP literals are forbidden")


def load_allowlist(path: Path) -> frozenset[str]:
    encoded = path.read_bytes()
    if not encoded or len(encoded) > MAX_CONFIG_BYTES:
        raise ProxyError("allowlist size is invalid")
    try:
        value = json.loads(encoded)
    except (UnicodeError, json.JSONDecodeError):
        raise ProxyError("allowlist is not valid JSON") from None
    if not isinstance(value, dict) or set(value) != {"schema_version", "hosts"} or type(value["schema_version"]) is not int or value["schema_version"] != 1:
        raise ProxyError("allowlist object is not canonical")
    hosts = value["hosts"]
    if not isinstance(hosts, list) or not 1 <= len(hosts) <= MAX_HOSTS:
        raise ProxyError("allowlist host count is invalid")
    checked = [validate_host(host) for host in hosts]
    if checked != sorted(checked) or len(set(checked)) != len(checked):
        raise ProxyError("allowlist hosts must be unique and sorted")
    canonical = json.dumps(value, ensure_ascii=True, sort_keys=True, separators=(",", ":")).encode() + b"\n"
    if encoded != canonical:
        raise ProxyError("allowlist encoding is not canonical")
    return frozenset(checked)


def parse_connect(header: bytes, allowlist: frozenset[str]) -> str:
    if len(header) > MAX_HEADER_BYTES or not header.endswith(b"\r\n\r\n") or b"\x00" in header:
        raise ProxyError("invalid CONNECT header")
    try:
        lines = header[:-4].decode("ascii").split("\r\n")
    except UnicodeDecodeError:
        raise ProxyError("invalid CONNECT header") from None
    if not lines or len(lines) > 64:
        raise ProxyError("invalid CONNECT header")
    parts = lines[0].split(" ")
    if len(parts) != 3 or parts[0] != "CONNECT" or parts[2] != "HTTP/1.1":
        raise ProxyError("only HTTP/1.1 CONNECT is supported")
    authority = parts[1]
    if any(mark in authority for mark in "@/?#[]") or authority.count(":") != 1:
        raise ProxyError("invalid HTTPS authority")
    host, port = authority.rsplit(":", 1)
    if port != "443" or validate_host(host) != host or host not in allowlist:
        raise ProxyError("HTTPS authority is not allowed")
    seen: set[str] = set()
    for line in lines[1:]:
        if not line or line[0] in " \t" or ":" not in line:
            raise ProxyError("invalid CONNECT header")
        name, value = line.split(":", 1)
        if not re.fullmatch(r"[A-Za-z0-9!#$%&'*+.^_`|~-]+", name) or any(
            (ord(c) < 32 and c != "\t") or ord(c) == 127 for c in value
        ):
            raise ProxyError("invalid CONNECT header")
        lowered = name.lower()
        if lowered in seen or lowered in {"proxy-authorization", "authorization"}:
            raise ProxyError("credentials and duplicate headers are forbidden")
        seen.add(lowered)
    if "host" not in seen:
        raise ProxyError("Host is required")
    host_line = next(line.split(":", 1)[1].strip() for line in lines[1:] if line.split(":", 1)[0].lower() == "host")
    if host_line != authority:
        raise ProxyError("Host differs from CONNECT authority")
    return host


def resolve_public(host: str) -> list[tuple[int, tuple[object, ...]]]:
    resolved: list[tuple[int, tuple[object, ...]]] = []
    seen: set[tuple[int, str]] = set()
    for family, socktype, proto, _, address in socket.getaddrinfo(host, 443, type=socket.SOCK_STREAM):
        if family not in {socket.AF_INET, socket.AF_INET6} or socktype != socket.SOCK_STREAM:
            continue
        ip = ipaddress.ip_address(address[0])
        if not ip.is_global:
            continue
        key = (family, str(ip))
        if key not in seen:
            seen.add(key)
            resolved.append((family, address))
    if not resolved:
        raise ProxyError("host has no public address")
    return resolved


def connect_public(host: str) -> socket.socket:
    deadline = time.monotonic() + CONNECT_TIMEOUT
    for family, address in resolve_public(host):
        upstream = socket.socket(family, socket.SOCK_STREAM)
        try:
            upstream.settimeout(max(0.01, deadline - time.monotonic()))
            upstream.connect(address)
            upstream.setblocking(False)
            return upstream
        except OSError:
            upstream.close()
            if time.monotonic() >= deadline:
                break
    raise ProxyError("public endpoint connection failed")


def read_header(client: socket.socket) -> bytes:
    client.settimeout(HEADER_TIMEOUT)
    data = bytearray()
    while b"\r\n\r\n" not in data:
        chunk = client.recv(min(4096, MAX_HEADER_BYTES + 1 - len(data)))
        if not chunk:
            raise ProxyError("incomplete CONNECT header")
        data.extend(chunk)
        if len(data) > MAX_HEADER_BYTES:
            raise ProxyError("CONNECT header exceeds bound")
    end = data.index(b"\r\n\r\n") + 4
    if end != len(data):
        raise ProxyError("tunnel bytes before CONNECT approval are forbidden")
    return bytes(data)


def relay(client: socket.socket, upstream: socket.socket) -> None:
    """Relay both directions with bounded buffers and readiness-driven writes."""
    client.setblocking(False)
    selector = selectors.DefaultSelector()
    sockets = (client, upstream)
    peer = {client: upstream, upstream: client}
    pending = {client: bytearray(), upstream: bytearray()}
    read_open = {client: True, upstream: True}
    started = last_activity = time.monotonic()

    def refresh(sock: socket.socket) -> None:
        events = 0
        if read_open[sock] and len(pending[peer[sock]]) < BUFFER_BYTES:
            events |= selectors.EVENT_READ
        if pending[sock]:
            events |= selectors.EVENT_WRITE
        try:
            if events:
                selector.modify(sock, events)
            else:
                selector.unregister(sock)
        except KeyError:
            if events:
                selector.register(sock, events)

    for sock in sockets:
        refresh(sock)
    try:
        while True:
            if not any(read_open.values()) and not any(pending.values()):
                return
            now = time.monotonic()
            remaining = min(IDLE_TIMEOUT - (now - last_activity), TUNNEL_TIMEOUT - (now - started))
            if remaining <= 0:
                return
            events = selector.select(remaining)
            if not events:
                return
            for key, mask in events:
                sock = key.fileobj
                other = peer[sock]
                if mask & selectors.EVENT_WRITE and pending[sock]:
                    sent = sock.send(pending[sock])
                    if sent:
                        del pending[sock][:sent]
                        last_activity = time.monotonic()
                        refresh(other)
                    if not pending[sock] and not read_open[other]:
                        try:
                            sock.shutdown(socket.SHUT_WR)
                        except OSError:
                            pass
                if mask & selectors.EVENT_READ and read_open[sock]:
                    capacity = BUFFER_BYTES - len(pending[other])
                    if capacity:
                        chunk = sock.recv(capacity)
                        if chunk:
                            pending[other].extend(chunk)
                            last_activity = time.monotonic()
                        else:
                            read_open[sock] = False
                            if not pending[other]:
                                try:
                                    other.shutdown(socket.SHUT_WR)
                                except OSError:
                                    pass
                refresh(sock)
                refresh(other)
    finally:
        selector.close()


def handle(client: socket.socket, allowlist: frozenset[str], slots: threading.BoundedSemaphore) -> None:
    upstream: socket.socket | None = None
    established = False
    try:
        host = parse_connect(read_header(client), allowlist)
        upstream = connect_public(host)
        client.sendall(b"HTTP/1.1 200 Connection Established\r\n\r\n")
        established = True
        relay(client, upstream)
    except (OSError, ProxyError):
        if not established:
            try:
                client.sendall(b"HTTP/1.1 403 Forbidden\r\nConnection: close\r\nContent-Length: 0\r\n\r\n")
            except OSError:
                pass
    finally:
        if upstream is not None:
            upstream.close()
        client.close()
        slots.release()


def serve(listener: socket.socket, allowlist: frozenset[str]) -> None:
    slots = threading.BoundedSemaphore(MAX_CONNECTIONS)
    while True:
        client, _ = listener.accept()
        if not slots.acquire(blocking=False):
            client.close()
            continue
        threading.Thread(target=handle, args=(client, allowlist, slots), daemon=True).start()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", type=Path, required=True)
    parser.add_argument("--socket-fd", type=int, default=3)
    parser.add_argument("--validate-config", action="store_true")
    args = parser.parse_args()
    allowlist = load_allowlist(args.config)
    if args.validate_config:
        return
    listener = socket.socket(fileno=args.socket_fd)
    if listener.getsockname() != ("127.0.0.2", 8766):
        raise SystemExit("gateway socket address differs")
    serve(listener, allowlist)


if __name__ == "__main__":
    main()
