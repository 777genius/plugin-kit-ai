from __future__ import annotations

import importlib.util
import json
import socket
import tempfile
import threading
import unittest
from pathlib import Path
from unittest import mock


SOURCE = Path(__file__).parents[2] / "deploy/uap-observer-egress-proxy.py"
SPEC = importlib.util.spec_from_file_location("uap_observer_egress_proxy", SOURCE)
assert SPEC and SPEC.loader
proxy = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(proxy)


class AllowlistTests(unittest.TestCase):
    def load(self, value: object, *, canonical: bool = True):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "allowlist.json"
            if canonical:
                path.write_text(json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n")
            else:
                path.write_text(json.dumps(value))
            return proxy.load_allowlist(path)

    def test_accepts_small_sorted_canonical_fqdn_set(self):
        self.assertEqual(
            self.load({"schema_version": 1, "hosts": ["api.github.com", "token.actions.githubusercontent.com"]}),
            {"api.github.com", "token.actions.githubusercontent.com"},
        )

    def test_rejects_noncanonical_and_unsafe_hosts(self):
        invalid = [
            "API.github.com", "api.github.com.", "*.github.com", "127.0.0.1",
            "[::1]", "localhost", "-api.github.com", "api..github.com", "api.github.com:443",
        ]
        for host in invalid:
            with self.subTest(host=host), self.assertRaises(proxy.ProxyError):
                self.load({"schema_version": 1, "hosts": [host]})

    def test_rejects_duplicates_case_variants_order_and_noncanonical_json(self):
        for hosts in (["api.github.com", "api.github.com"], ["API.github.com", "api.github.com"], ["z.example.com", "a.example.com"]):
            with self.subTest(hosts=hosts), self.assertRaises(proxy.ProxyError):
                self.load({"schema_version": 1, "hosts": hosts})
        with self.assertRaises(proxy.ProxyError):
            self.load({"schema_version": 1, "hosts": ["api.github.com"]}, canonical=False)

    def test_enforces_config_bounds(self):
        with self.assertRaises(proxy.ProxyError):
            self.load({"schema_version": 1, "hosts": []})
        with self.assertRaises(proxy.ProxyError):
            self.load({"schema_version": 1, "hosts": [f"h{i}.example.com" for i in range(33)]})

    def test_rejects_boolean_schema_versions(self):
        for value in (True, False):
            with self.subTest(value=value), self.assertRaises(proxy.ProxyError):
                self.load({"schema_version": value, "hosts": ["api.github.com"]})


class ConnectTests(unittest.TestCase):
    allowlist = frozenset({"api.github.com"})

    def test_accepts_exact_connect_authority(self):
        header = b"CONNECT api.github.com:443 HTTP/1.1\r\nHost: api.github.com:443\r\n\r\n"
        self.assertEqual(proxy.parse_connect(header, self.allowlist), "api.github.com")

    def test_rejects_malformed_or_unsafe_authorities(self):
        authorities = [
            "user@api.github.com:443", "api.github.com:444", "api.github.com", "127.0.0.1:443",
            "api.github.com.:443", "*.github.com:443", "API.github.com:443",
            "api.github.com:443#fragment", "api.github.com:443/path", "[::1]:443",
        ]
        for authority in authorities:
            header = f"CONNECT {authority} HTTP/1.1\r\nHost: {authority}\r\n\r\n".encode()
            with self.subTest(authority=authority), self.assertRaises(proxy.ProxyError):
                proxy.parse_connect(header, self.allowlist)

    def test_rejects_credentials_duplicate_headers_and_early_tunnel_data(self):
        for extra in (
            b"Proxy-Authorization: Basic abc\r\n",
            b"Host: api.github.com:443\r\nHost: api.github.com:443\r\n",
            b"X-Invalid: value\x7f\r\n",
            b"X-Only: value\r\n",
        ):
            with self.assertRaises(proxy.ProxyError):
                proxy.parse_connect(b"CONNECT api.github.com:443 HTTP/1.1\r\n" + extra + b"\r\n", self.allowlist)

    def test_runtime_resolution_filters_every_nonpublic_address(self):
        answers = [
            (socket.AF_INET, socket.SOCK_STREAM, 6, "", ("127.0.0.1", 443)),
            (socket.AF_INET, socket.SOCK_STREAM, 6, "", ("10.0.0.1", 443)),
            (socket.AF_INET, socket.SOCK_STREAM, 6, "", ("192.0.2.1", 443)),
            (socket.AF_INET, socket.SOCK_STREAM, 6, "", ("8.8.8.8", 443)),
        ]
        with mock.patch.object(proxy.socket, "getaddrinfo", return_value=answers):
            self.assertEqual(proxy.resolve_public("api.github.com"), [(socket.AF_INET, ("8.8.8.8", 443))])

    def test_resolution_fails_closed_without_public_address(self):
        answers = [(socket.AF_INET, socket.SOCK_STREAM, 6, "", ("127.0.0.1", 443))]
        with mock.patch.object(proxy.socket, "getaddrinfo", return_value=answers), self.assertRaises(proxy.ProxyError):
            proxy.resolve_public("api.github.com")


class RelayTests(unittest.TestCase):
    class PartialSocket:
        def __init__(self, sock: socket.socket):
            self.sock = sock
            self.send_sizes: list[int] = []
            self.recv_limits: list[int] = []

        def fileno(self):
            return self.sock.fileno()

        def setblocking(self, value):
            self.sock.setblocking(value)

        def recv(self, size):
            self.recv_limits.append(size)
            return self.sock.recv(size)

        def send(self, value):
            size = self.sock.send(value[:17])
            self.send_sizes.append(size)
            return size

        def shutdown(self, how):
            self.sock.shutdown(how)

    def test_bidirectional_relay_handles_partial_writes_with_bounded_reads(self):
        client_peer, client_socket = socket.socketpair()
        upstream_socket, upstream_peer = socket.socketpair()
        client = self.PartialSocket(client_socket)
        upstream = self.PartialSocket(upstream_socket)
        left_to_right = b"left" * 40000
        right_to_left = b"right" * 40000
        received: dict[str, bytes] = {}

        relay_thread = threading.Thread(target=proxy.relay, args=(client, upstream))
        relay_thread.start()

        def send_and_finish(sock, value):
            sock.sendall(value)
            sock.shutdown(socket.SHUT_WR)

        def receive(name, sock):
            chunks = []
            while True:
                chunk = sock.recv(8192)
                if not chunk:
                    break
                chunks.append(chunk)
            received[name] = b"".join(chunks)

        workers = [
            threading.Thread(target=send_and_finish, args=(client_peer, left_to_right)),
            threading.Thread(target=send_and_finish, args=(upstream_peer, right_to_left)),
            threading.Thread(target=receive, args=("client", client_peer)),
            threading.Thread(target=receive, args=("upstream", upstream_peer)),
        ]
        for worker in workers:
            worker.start()
        for worker in workers:
            # The forced 17-byte writes deliberately require tens of
            # thousands of selector turns; leave headroom on loaded portable
            # CI runners while retaining a hard deadlock bound.
            worker.join(15)
            self.assertFalse(worker.is_alive())
        relay_thread.join(15)
        self.assertFalse(relay_thread.is_alive())
        self.assertEqual(received["client"], right_to_left)
        self.assertEqual(received["upstream"], left_to_right)
        self.assertTrue(client.send_sizes and upstream.send_sizes)
        self.assertLessEqual(max(client.send_sizes + upstream.send_sizes), 17)
        self.assertLessEqual(max(client.recv_limits + upstream.recv_limits), proxy.BUFFER_BYTES)
        for sock in (client_peer, client_socket, upstream_socket, upstream_peer):
            sock.close()

    def test_tunnel_time_bounds_meet_operator_minima(self):
        self.assertGreaterEqual(proxy.IDLE_TIMEOUT, 120)
        self.assertGreaterEqual(proxy.TUNNEL_TIMEOUT, 900)

    def test_handle_never_sends_http_after_connect_is_established(self):
        class FakeSocket:
            def __init__(self):
                self.writes = []
                self.closed = False

            def sendall(self, value):
                self.writes.append(value)

            def close(self):
                self.closed = True

        class Slot:
            released = False

            def release(self):
                self.released = True

        client, upstream, slot = FakeSocket(), FakeSocket(), Slot()
        header = b"CONNECT api.github.com:443 HTTP/1.1\r\nHost: api.github.com:443\r\n\r\n"
        with (
            mock.patch.object(proxy, "read_header", return_value=header),
            mock.patch.object(proxy, "connect_public", return_value=upstream),
            mock.patch.object(proxy, "relay", side_effect=proxy.ProxyError("tunnel failed")),
        ):
            proxy.handle(client, frozenset({"api.github.com"}), slot)
        self.assertEqual(client.writes, [b"HTTP/1.1 200 Connection Established\r\n\r\n"])
        self.assertTrue(client.closed and upstream.closed and slot.released)


if __name__ == "__main__":
    unittest.main()
