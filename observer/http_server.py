"""Bounded loopback HTTP boundary for the stable-launch observer."""

from __future__ import annotations

import argparse
import json
import socket
import socketserver
import threading
import time
from collections import deque
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from typing import Any

from .auth import AuthenticationError
from .config import Config
from .service import MAX_RESPONSE_BYTES, ObserverService, RequestValidationError, WorkBusyError

MAX_REQUEST_BYTES = 128 << 10
ROUTE = "/v1/stable-launch/observe"
REQUEST_TOTAL_SECONDS = 20
MAX_CONNECTIONS = 16
END_TO_END_SECONDS = 900


class VerifiedRateLimitError(ValueError):
    pass


class SlidingWindowRateLimiter:
    def __init__(self, limit: int = 30, window_seconds: float = 60):
        self.limit, self.window_seconds = limit, window_seconds
        self._requests: deque[float] = deque()
        self._lock = threading.Lock()

    def allow(self, now: float) -> bool:
        with self._lock:
            while self._requests and self._requests[0] <= now - self.window_seconds:
                self._requests.popleft()
            if len(self._requests) >= self.limit:
                return False
            self._requests.append(now)
            return True


class PerSourceRateLimiter:
    def __init__(self, limit: int = 30, window_seconds: float = 60, max_sources: int = 1024):
        self.limit, self.window_seconds, self.max_sources = limit, window_seconds, max_sources
        self._sources: dict[str, deque[float]] = {}
        self._lock = threading.Lock()

    def allow(self, source: str, now: float) -> bool:
        with self._lock:
            for key in list(self._sources):
                values = self._sources[key]
                while values and values[0] <= now - self.window_seconds:
                    values.popleft()
                if not values:
                    del self._sources[key]
            values = self._sources.setdefault(source, deque())
            if len(values) >= self.limit:
                return False
            if len(self._sources) > self.max_sources:
                oldest = min(self._sources, key=lambda key: self._sources[key][-1])
                if oldest != source:
                    del self._sources[oldest]
            values.append(now)
            return True


def _no_duplicates(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    value: dict[str, Any] = {}
    for key, child in pairs:
        if key in value:
            raise ValueError("duplicate JSON object key")
        value[key] = child
    return value


class ObserverHandler(BaseHTTPRequestHandler):
    server_version = "uap-observer"
    sys_version = ""

    @property
    def service(self) -> ObserverService:
        return self.server.service  # type: ignore[attr-defined,no-any-return]

    def setup(self) -> None:
        super().setup()
        self.connection.settimeout(15)

    def do_POST(self) -> None:
        started = time.monotonic()
        if self.path != ROUTE:
            self._json_error(404, "not found")
            return
        try:
            authorizations = self.headers.get_all("Authorization", [])
            if len(authorizations) != 1:
                raise AuthenticationError("GitHub OIDC identity is required")
            authorization = authorizations[0]
            if not authorization.startswith("Bearer ") or authorization.count(" ") != 1:
                raise AuthenticationError("GitHub OIDC identity is required")
        except AuthenticationError:
            self._json_error(401, "authentication failed")
            return
        try:
            def charge_verified_caller() -> None:
                if not self.server.rate_limiter.allow(time.monotonic()):  # type: ignore[attr-defined]
                    raise VerifiedRateLimitError("verified caller rate limit exceeded")
            auth = self.service.authenticate(authorization[7:], on_authenticated=charge_verified_caller)
        except AuthenticationError:
            self._json_error(401, "authentication failed")
            return
        except VerifiedRateLimitError:
            self._json_error(429, "rate limit exceeded", retry_after="60")
            return
        try:
            request = self._read_request()
        except (ValueError, UnicodeError, json.JSONDecodeError, TimeoutError, socket.timeout):
            self._json_error(400, "request rejected")
            return
        try:
            response = self.service.observe_authenticated(request, auth, deadline=started + END_TO_END_SECONDS)
            if len(response) > MAX_RESPONSE_BYTES:
                raise ValueError("observer response exceeds size bound")
        except AuthenticationError:
            self._json_error(401, "authentication failed")
            return
        except RequestValidationError:
            self._json_error(400, "request rejected")
            return
        except VerifiedRateLimitError:
            self._json_error(429, "rate limit exceeded", retry_after="60")
            return
        except WorkBusyError:
            self._json_error(409, "observer busy", retry_after="30")
            return
        except Exception:
            self._json_error(503, "observer unavailable")
            return
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(response)))
        self.send_header("Cache-Control", "private, no-store")
        self.send_header("X-Content-Type-Options", "nosniff")
        self.end_headers()
        self.wfile.write(response)

    def _read_request(self) -> Any:
        if self.headers.get("Transfer-Encoding") is not None:
            raise ValueError("chunked requests are forbidden")
        if self.headers.get("Content-Type") != "application/json":
            raise ValueError("content type must be application/json")
        lengths = self.headers.get_all("Content-Length", [])
        if len(lengths) != 1:
            raise ValueError("one content length is required")
        raw_length = lengths[0]
        if raw_length is None or not raw_length.isascii() or not raw_length.isdigit():
            raise ValueError("content length is required")
        length = int(raw_length)
        if length <= 0 or length > MAX_REQUEST_BYTES:
            raise ValueError("request exceeds size bound")
        deadline = time.monotonic() + REQUEST_TOTAL_SECONDS
        chunks = []
        remaining = length
        while remaining:
            available = deadline - time.monotonic()
            if available <= 0:
                raise TimeoutError("request body exceeded total deadline")
            self.connection.settimeout(available)
            chunk = self.rfile.read(min(64 << 10, remaining))
            if not chunk:
                raise ValueError("request body length differs")
            chunks.append(chunk)
            remaining -= len(chunk)
        return json.loads(b"".join(chunks), object_pairs_hook=_no_duplicates)

    def do_GET(self) -> None:
        self._json_error(404, "not found")

    def do_PUT(self) -> None:
        self._json_error(405, "method not allowed")

    def _json_error(self, status: int, message: str, *, retry_after: str | None = None) -> None:
        body = json.dumps({"error": message}, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        if retry_after is not None:
            self.send_header("Retry-After", retry_after)
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format: str, *args: Any) -> None:
        # Authorization and request bodies are intentionally never logged.
        return


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", type=Path, required=True)
    args = parser.parse_args()
    config = Config.load(args.config)
    server = BoundedThreadingHTTPServer((config.bind_host, config.bind_port), ObserverHandler)
    server.service = ObserverService(config)  # type: ignore[attr-defined]
    server.serve_forever()
    return 0


class BoundedThreadingHTTPServer(socketserver.ThreadingMixIn, HTTPServer):
    daemon_threads = True

    def __init__(self, *args: Any, **kwargs: Any):
        self._connections = threading.BoundedSemaphore(MAX_CONNECTIONS)
        # This scarce global execution bucket is charged only by the callback
        # reached after OIDC validation, public-run corroboration, and replay
        # rejection. Tokenless, invalid, and replayed requests cannot drain it.
        self.rate_limiter = SlidingWindowRateLimiter(limit=6, window_seconds=60)
        super().__init__(*args, **kwargs)

    def process_request(self, request: socket.socket, client_address: Any) -> None:
        if not self._connections.acquire(blocking=False):
            self.shutdown_request(request)
            return
        super().process_request(request, client_address)

    def process_request_thread(self, request: socket.socket, client_address: Any) -> None:
        try:
            super().process_request_thread(request, client_address)
        finally:
            self._connections.release()


if __name__ == "__main__":
    raise SystemExit(main())
