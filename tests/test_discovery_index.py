from __future__ import annotations

import base64
import io
import json
import subprocess
import tempfile
import threading
import time
import unittest
import urllib.error
import urllib.request
from pathlib import Path
from unittest import mock

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey, Ed25519PublicKey

from scripts.build_bridges import BridgeError, PinnedRepository
from scripts.build_discovery_index import (
    CODE_SEARCH_REQUEST_INTERVAL_SECONDS,
    DiscoveryError,
    GitHubAPI,
    GitHubHTTPError,
    MAX_GITHUB_RETRY_DELAY_SECONDS,
    MAX_PREVIOUS_SNAPSHOT_BYTES,
    SameOriginRedirect,
    bounded_package_files,
    build_candidate,
    discover_search_items,
    load_previous,
    make_record,
    package_facts,
    repository_states,
    scan_repository,
)
from scripts.directory_publication import canonical_json
import scripts.discovery_publication as discovery_publication
from scripts.discovery_publication import load_latest, publish

PublicationError = discovery_publication.PublicationError


ROOT = Path(__file__).resolve().parents[1]


class PartitionAPI:
    def get(self, path: str, parameters: dict[str, object] | None = None):
        self.assert_path(path)
        if parameters.get("sort") != "indexed" or parameters.get("order") != "asc":
            raise AssertionError(parameters)
        query = str(parameters["q"])
        page = int(parameters["page"])
        if "size:0..10" in query:
            return {"total_count": 101, "incomplete_results": False, "items": []}
        if "size:0..5" in query:
            total, prefix = 51, "left"
        elif "size:6..10" in query:
            total, prefix = 50, "right"
        else:
            raise AssertionError(query)
        start = (page - 1) * 100
        stop = min(start + 100, total)
        items = [
            {"path": f"plugins/{prefix}-{index}/plugin.json", "repository": {"full_name": "owner/repo"}}
            for index in range(start, stop)
        ]
        return {"total_count": total, "incomplete_results": False, "items": items}

    @staticmethod
    def assert_path(path: str) -> None:
        if path != "search/code":
            raise AssertionError(path)


class FixtureAPI:
    def __init__(self, revision: str):
        self.revision = revision

    def get(self, path: str, parameters: dict[str, object] | None = None):
        if path == "search/code":
            query = str(parameters["q"])
            if "repo:owner/repo" in query or "filename:plugin.json" in query:
                return {"total_count": 1, "incomplete_results": False, "items": [
                    {"path": "packages/demo/plugin.json", "repository": {"full_name": "owner/repo"}},
                ]}
        raise AssertionError((path, parameters))

    def graphql(self, query: str, variables: dict[str, object]):
        self.assert_graphql_query(query)
        return {
            "r0": {
                "nameWithOwner": "owner/repo", "isPrivate": False, "isArchived": False,
                "stargazerCount": 42, "pushedAt": "2026-08-27T00:00:00Z",
                "updatedAt": "2026-08-27T00:00:00Z",
                "defaultBranchRef": {"target": {"oid": self.revision}},
            },
        }

    @staticmethod
    def assert_graphql_query(query: str) -> None:
        if "defaultBranchRef" not in query:
            raise AssertionError(query)


class DiscoveryAcquisitionTests(unittest.TestCase):
    def test_nonportable_git_path_is_package_invalid_not_scan_incomplete(self) -> None:
        repository = mock.Mock(root=Path("/tmp/inert-mirror"), revision="a" * 40)
        tree = b"100644 blob " + b"b" * 40 + b"\tplugins/demo/\xc3\xa9.txt\0"
        with mock.patch("scripts.build_discovery_index.git", return_value=tree):
            with self.assertRaisesRegex(DiscoveryError, "path must be non-empty ASCII"):
                bounded_package_files(repository, "plugins/demo")

    def test_scan_passes_job_token_only_to_remote_acquisition(self) -> None:
        pinned = mock.Mock()
        with mock.patch("scripts.build_discovery_index.PinnedRepository", return_value=pinned) as constructor:
            records, diagnostics = scan_repository(
                "owner/repo",
                {"repository": "owner/repo", "revision": "a" * 40},
                [], "2026-08-28T00:00:00Z", {}, None, "job-token",
            )
        self.assertEqual(records, {})
        self.assertEqual(diagnostics, [])
        constructor.assert_called_once_with(
            "owner/repo", "a" * 40, None, github_token="job-token",
        )
        pinned.close.assert_called_once_with()


def git(directory: Path, *arguments: str) -> str:
    completed = subprocess.run(["git", *arguments], cwd=directory, check=True, text=True, stdout=subprocess.PIPE)
    return completed.stdout.strip()


def create_mirror(root: Path, package_path: str = "packages/demo", repository: str = "owner/repo") -> tuple[Path, str]:
    source = root / "source"
    source.mkdir()
    git(source, "init", "--quiet", "--initial-branch=main")
    git(source, "config", "user.email", "fixture@example.test")
    git(source, "config", "user.name", "Fixture")
    package = source / package_path
    package.mkdir(parents=True, exist_ok=True)
    (package / "plugin.json").write_text(json.dumps({
        "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
        "name": "demo",
        "version": "1.2.3",
        "description": "Discovery fixture",
        "author": {"name": "Fixture"},
        "repository": "https://github.com/" + repository,
        "license": "Apache-2.0",
    }), encoding="utf-8")
    (package / "mcp.json").write_text(json.dumps({
        "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
        "mcpServers": {"demo": {"type": "streamable-http", "url": "https://example.test/mcp"}},
    }), encoding="utf-8")
    git(source, "add", "plugin.json" if not package_path else package_path + "/plugin.json",
        "mcp.json" if not package_path else package_path + "/mcp.json")
    git(source, "commit", "--quiet", "-m", "fixture")
    revision = git(source, "rev-parse", "HEAD")
    mirror_root = root / "mirrors"
    bare = mirror_root / (repository + ".git")
    bare.parent.mkdir(parents=True)
    subprocess.run(["git", "clone", "--quiet", "--bare", str(source), str(bare)], check=True)
    return mirror_root, revision


def candidate_record(revision: str) -> dict[str, object]:
    return {
        "slug": "discovery:owner/repo//packages/demo",
        "name": "demo",
        "description": "Discovery fixture",
        "owner": "owner",
        "repository": "owner/repo",
        "package_path": "packages/demo",
        "revision": revision,
        "version": "1.2.3",
        "license": "Apache-2.0",
        "schema_version": "1.0.0",
        "components": {"extensions": 0, "mcp": 1, "skills": 0},
        "mcp_transports": ["streamable-http"],
        "compatible_clients": ["codex", "cursor", "copilot", "vscode", "kiro"],
        "authentication": "unknown",
        "status": "conformant_unreviewed",
        "runtime_reviewed": False,
        "tree_digest": "sha256:" + "1" * 64,
        "manifest_digest": "sha256:" + "2" * 64,
        "stars": 42,
        "repository_updated_at": "2026-08-27T00:00:00Z",
        "reviewed_distribution_id": None,
        "availability": "available",
        "author": {"name": "Fixture"},
        "first_seen": "2026-08-27T00:00:00Z",
        "last_seen": "2026-08-27T00:00:00Z",
    }


def previous_snapshot(records: list[dict[str, object]]) -> dict[str, object]:
    return {
        "discovery_schema_version": 1,
        "sequence": 1,
        "publication_id": "fixture-1",
        "source_commit": "b" * 40,
        "generated_at": "2026-08-27T00:00:00Z",
        "expires_at": "2026-08-30T00:00:00Z",
        "complete": True,
        "query_manifest_digest": "sha256:" + "3" * 64,
        "partitions": [],
        "search_projection": {
            "path": "search/00000000000000000001.json",
            "digest": "sha256:" + "4" * 64,
            "record_count": len(records),
        },
        "records": records,
    }


class DiscoveryIndexTests(unittest.TestCase):
    def test_package_facts_omits_author_without_required_name(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            manifest = {
                "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
                "name": "author-without-name",
                "version": "1.0.0",
                "description": "Valid Agent Plugin author without a display name",
                "author": {"url": "https://example.test/author"},
            }
            (root / "plugin.json").write_text(json.dumps(manifest), encoding="utf-8")

            self.assertIsNone(package_facts(root)["author"])

    def test_candidate_records_are_schema_checked_before_publication(self):
        previous = candidate_record("a" * 40)
        previous["author"] = {"url": "https://example.test/author"}
        with self.assertRaisesRegex(DiscoveryError, "Discovery candidate records: schema error"):
            build_candidate(
                api=FixtureAPI("a" * 40),
                config={"schema_version": 1, "query": "schema", "maximum_file_size": 10,
                        "maximum_records": 100, "seeds": []},
                mode="refresh", generated_at="2026-08-27T06:00:00Z",
                previous_records=[previous],
            )

    def test_valid_previous_snapshot_is_loaded_by_bounded_parser(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "snapshot.json"
            record = candidate_record("a" * 40)
            path.write_bytes(canonical_json(previous_snapshot([record])))
            self.assertEqual(load_previous(path), [record])

    def test_previous_snapshot_parser_fails_closed(self):
        cases = {
            "oversized": (
                b" " * (MAX_PREVIOUS_SNAPSHOT_BYTES + 1),
                f"exceeds {MAX_PREVIOUS_SNAPSHOT_BYTES} bytes",
            ),
            "duplicate-key": (b'{"complete":true,"complete":true}', "duplicate JSON key 'complete'"),
            "non-finite": (b'{"sequence":NaN}', "non-integer JSON number 'NaN' is forbidden"),
        }
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for name, (body, error) in cases.items():
                with self.subTest(name=name):
                    path = root / f"{name}.json"
                    path.write_bytes(body)
                    with self.assertRaisesRegex(DiscoveryError, error):
                        load_previous(path)

    def test_github_api_redirect_cannot_exfiltrate_the_job_token(self):
        handler = SameOriginRedirect("https://api.github.com")
        request = urllib.request.Request(
            "https://api.github.com/repos/owner/repo",
            headers={"Authorization": "Bearer secret"},
        )
        with self.assertRaisesRegex(DiscoveryError, "cross-origin redirect"):
            handler.redirect_request(request, None, 302, "Found", {}, "https://attacker.example/token")

    def test_github_api_first_code_search_request_is_not_delayed(self):
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = b'{"ok":true}'
        monotonic = mock.Mock(return_value=100.0)
        sleep = mock.Mock()
        api = GitHubAPI("test-token", search_monotonic=monotonic, search_sleep=sleep)
        api.opener = mock.Mock()
        api.opener.open.return_value = response

        self.assertEqual(api.get("search/code", {"q": "plugin.json"}), {"ok": True})

        sleep.assert_not_called()
        self.assertEqual(api.opener.open.call_count, 1)

    def test_github_api_consecutive_code_search_requests_are_paced(self):
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = b'{"ok":true}'
        clock = [100.0]

        def sleep(delay: float) -> None:
            clock[0] += delay

        api = GitHubAPI(
            "test-token", search_monotonic=lambda: clock[0], search_sleep=mock.Mock(side_effect=sleep),
        )
        api.opener = mock.Mock()
        api.opener.open.return_value = response

        api.get("search/code", {"q": "first"})
        api.get("search/code", {"q": "second"})

        api._search_sleep.assert_called_once_with(CODE_SEARCH_REQUEST_INTERVAL_SECONDS)
        self.assertEqual(api.opener.open.call_count, 2)

    def test_github_api_concurrent_code_search_requests_are_serialized(self):
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = b'{"ok":true}'
        clock = [100.0]
        clock_lock = threading.Lock()
        starts: list[float] = []

        def monotonic() -> float:
            with clock_lock:
                return clock[0]

        def sleep(delay: float) -> None:
            with clock_lock:
                clock[0] += delay

        def open_request(request, timeout):  # noqa: ANN001
            starts.append(monotonic())
            return response

        api = GitHubAPI("test-token", search_monotonic=monotonic, search_sleep=sleep)
        api.opener = mock.Mock()
        api.opener.open.side_effect = open_request
        barrier = threading.Barrier(3)

        def search(query: str) -> None:
            barrier.wait()
            api.get("search/code", {"q": query})

        threads = [threading.Thread(target=search, args=(query,)) for query in ("first", "second")]
        for thread in threads:
            thread.start()
        barrier.wait()
        for thread in threads:
            thread.join()

        self.assertEqual(len(starts), 2)
        self.assertEqual(starts[1] - starts[0], CODE_SEARCH_REQUEST_INTERVAL_SECONDS)

    def test_github_api_non_search_requests_are_not_delayed(self):
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = b'{"data":{}}'
        sleep = mock.Mock()
        api = GitHubAPI("test-token", search_monotonic=lambda: 100.0, search_sleep=sleep)
        api.opener = mock.Mock()
        api.opener.open.return_value = response

        api.get("repos/owner/repo")
        api.graphql("query { viewer { login } }", {})

        sleep.assert_not_called()
        self.assertEqual(api.opener.open.call_count, 2)

    def test_github_api_retries_transient_server_failure(self):
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = b'{"ok":true}'
        api = GitHubAPI("test-token")
        api.opener = mock.Mock()
        api.opener.open.side_effect = [
            urllib.error.HTTPError(
                "https://api.github.com/test", 503, "Unavailable", {"Retry-After": "75"},
                io.BytesIO(b"retry"),
            ),
            response,
        ]
        with mock.patch("scripts.build_discovery_index.time.sleep") as sleep:
            self.assertEqual(api.get("test"), {"ok": True})
        sleep.assert_called_once_with(30)
        self.assertEqual(api.opener.open.call_count, 2)

    def test_github_api_retries_all_reviewed_http_statuses(self):
        cases = [(403, 120), (500, 30), (502, 30), (504, 30)]
        for status, expected_delay in cases:
            with self.subTest(status=status):
                response = mock.MagicMock()
                response.__enter__.return_value.read.return_value = b'{"ok":true}'
                api = GitHubAPI("test-token")
                api.opener = mock.Mock()
                api.opener.open.side_effect = [
                    urllib.error.HTTPError(
                        "https://api.github.com/test", status, "Retryable",
                        {"Retry-After": "999"}, io.BytesIO(b"retry"),
                    ),
                    response,
                ]
                with mock.patch("scripts.build_discovery_index.time.sleep") as sleep:
                    self.assertEqual(api.get("test"), {"ok": True})
                sleep.assert_called_once_with(expected_delay)
                self.assertEqual(api.opener.open.call_count, 2)

    def test_github_api_http_error_body_read_failure_stays_in_http_retry_path(self):
        for body_error in (TimeoutError("timed out"), OSError("read failed")):
            with self.subTest(error=type(body_error).__name__):
                body = mock.Mock()
                body.read.side_effect = body_error
                response = mock.MagicMock()
                response.__enter__.return_value.read.return_value = b'{"ok":true}'
                api = GitHubAPI("test-token")
                api.opener = mock.Mock()
                api.opener.open.side_effect = [
                    urllib.error.HTTPError(
                        "https://api.github.com/test", 503, "Unavailable",
                        {"Retry-After": "75"}, body,
                    ),
                    response,
                ]
                with mock.patch("scripts.build_discovery_index.time.sleep") as sleep:
                    self.assertEqual(api.get("test"), {"ok": True})
                body.read.assert_called_once_with(4096)
                sleep.assert_called_once_with(30)
                self.assertEqual(api.opener.open.call_count, 2)

    def test_github_api_body_read_failure_preserves_http_status_and_path(self):
        body = mock.Mock()
        body.read.side_effect = OSError("read failed")
        api = GitHubAPI("test-token")
        api.opener = mock.Mock()
        api.opener.open.side_effect = urllib.error.HTTPError(
            "https://api.github.com/repos/owner/repo", 404, "Not Found", {}, body,
        )
        with mock.patch("scripts.build_discovery_index.time.sleep") as sleep:
            with self.assertRaisesRegex(
                GitHubHTTPError,
                r"GitHub API repos/owner/repo failed with HTTP 404: "
                r"<unable to read response body: OSError>",
            ) as raised:
                api.get("repos/owner/repo")
        self.assertEqual(raised.exception.status, 404)
        body.read.assert_called_once_with(4096)
        sleep.assert_not_called()

    def test_github_api_honors_secondary_limit_message_then_succeeds(self):
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = b'{"ok":true}'
        api = GitHubAPI("test-token")
        api.opener = mock.Mock()
        api.opener.open.side_effect = [
            urllib.error.HTTPError(
                "https://api.github.com/test", 429, "Too Many Requests", {},
                io.BytesIO(b'{"message":"try again in 75.004104311s"}'),
            ),
            response,
        ]
        with mock.patch("scripts.build_discovery_index.time.sleep") as sleep:
            self.assertEqual(api.get("test"), {"ok": True})
        sleep.assert_called_once_with(76)

    def test_github_api_uses_largest_rate_limit_hint_within_cap(self):
        cases = [
            ("header", {"Retry-After": "80", "X-RateLimit-Reset": "150"}, "75.1", 80),
            ("message", {"Retry-After": "25", "X-RateLimit-Reset": "150"}, "75.1", 76),
            ("reset", {"Retry-After": "25", "X-RateLimit-Reset": "180"}, "75.1", 81),
            ("cap", {}, "999999999999999999999999999999999999999.1", MAX_GITHUB_RETRY_DELAY_SECONDS),
            ("huge-header", {"Retry-After": "9" * 5_000}, "invalid", MAX_GITHUB_RETRY_DELAY_SECONDS),
        ]
        for name, headers, message_delay, expected in cases:
            with self.subTest(name=name):
                response = mock.MagicMock()
                response.__enter__.return_value.read.return_value = b'{"ok":true}'
                api = GitHubAPI("test-token")
                api.opener = mock.Mock()
                api.opener.open.side_effect = [
                    urllib.error.HTTPError(
                        "https://api.github.com/test", 429, "Too Many Requests", headers,
                        io.BytesIO(f'{{"message":"try again in {message_delay}s"}}'.encode()),
                    ),
                    response,
                ]
                with (
                    mock.patch("scripts.build_discovery_index.time.time", return_value=100),
                    mock.patch("scripts.build_discovery_index.time.sleep") as sleep,
                ):
                    self.assertEqual(api.get("test"), {"ok": True})
                sleep.assert_called_once_with(expected)

    def test_github_api_malformed_rate_limit_hints_use_bounded_fallback(self):
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = b'{"ok":true}'
        api = GitHubAPI("test-token")
        api.opener = mock.Mock()
        api.opener.open.side_effect = [
            urllib.error.HTTPError(
                "https://api.github.com/test", 429, "Too Many Requests",
                {"Retry-After": "-10", "X-RateLimit-Reset": "not-a-time"},
                io.BytesIO(b'{"message":"try again in -5s"}'),
            ),
            response,
        ]
        with mock.patch("scripts.build_discovery_index.time.sleep") as sleep:
            self.assertEqual(api.get("test"), {"ok": True})
        sleep.assert_called_once_with(1)

    def test_github_api_five_sleep_exhaustion_includes_bounded_body(self):
        api = GitHubAPI("test-token")
        api.opener = mock.Mock()
        bodies = [mock.Mock() for _ in range(6)]
        for body in bodies:
            body.read.return_value = b'{"message":"still unavailable"}'
        api.opener.open.side_effect = [
            urllib.error.HTTPError(
                "https://api.github.com/test", 504, "Unavailable", {"Retry-After": "75"}, body,
            )
            for body in bodies
        ]
        with mock.patch("scripts.build_discovery_index.time.sleep") as sleep:
            with self.assertRaisesRegex(GitHubHTTPError, "still unavailable"):
                api.get("test")
        self.assertEqual(api.opener.open.call_count, 6)
        self.assertEqual(sleep.call_args_list, [mock.call(30)] * 5)
        for body in bodies:
            body.read.assert_called_once_with(4096)

    def test_github_api_non_retryable_error_does_not_sleep(self):
        api = GitHubAPI("test-token")
        api.opener = mock.Mock()
        api.opener.open.side_effect = urllib.error.HTTPError(
            "https://api.github.com/test", 404, "Not Found", {}, io.BytesIO(b"missing"),
        )
        with mock.patch("scripts.build_discovery_index.time.sleep") as sleep:
            with self.assertRaisesRegex(GitHubHTTPError, "missing"):
                api.get("test")
        sleep.assert_not_called()

    def test_graphql_accepts_scoped_not_found_but_rejects_unrelated_partial_errors(self):
        api = GitHubAPI("test-token")
        api._request = mock.Mock(return_value={
            "data": {"r0": None, "r1": {"nameWithOwner": "owner/repo"}},
            "errors": [{"type": "NOT_FOUND", "path": ["r0"], "message": "Could not resolve"}],
        })
        self.assertEqual(api.graphql("query", {}), {
            "r0": None, "r1": {"nameWithOwner": "owner/repo"},
        })

        api._request.return_value = {
            "data": {"r0": None},
            "errors": [{"type": "FORBIDDEN", "path": ["r0"], "message": "Forbidden"}],
        }
        with self.assertRaisesRegex(DiscoveryError, "GraphQL returned errors"):
            api.graphql("query", {})

    def test_search_partitions_before_paginating_past_github_cap(self):
        items, partitions = discover_search_items(PartitionAPI(), "schema filename:plugin.json", 10)
        self.assertEqual(len(items), 101)
        self.assertEqual([(item["size_min"], item["size_max"], item["total_count"]) for item in partitions], [
            (0, 5, 51), (6, 10, 50),
        ])

    def test_search_ignores_filename_near_matches_without_marking_scan_incomplete(self):
        class NearMatchAPI:
            @staticmethod
            def get(path: str, parameters: dict[str, object] | None = None):
                return {"total_count": 1, "incomplete_results": False, "items": [
                    {"path": "packages/demo/not-a-plugin.json", "repository": {"full_name": "owner/repo"}},
                ]}

        items, partitions = discover_search_items(NearMatchAPI(), "schema filename:plugin.json", 10)
        self.assertEqual(items, [])
        self.assertEqual(partitions[0]["total_count"], 1)

    def test_search_retries_when_total_changes_during_pagination(self):
        class MovingAPI:
            def __init__(self):
                self.calls = 0

            def get(self, path: str, parameters: dict[str, object] | None = None):
                self.calls += 1
                if self.calls == 1:
                    total, indices = 2, [0]
                else:
                    total, indices = 2, [0, 1]
                return {
                    "total_count": total, "incomplete_results": False,
                    "items": [
                        {"path": f"plugins/demo-{index}/plugin.json", "repository": {"full_name": "owner/repo"}}
                        for index in indices
                    ],
                }

        api = MovingAPI()
        with mock.patch("scripts.build_discovery_index.time.sleep"):
            items, partitions = discover_search_items(api, "schema filename:plugin.json", 10)
        self.assertEqual(len(items), 2)
        self.assertEqual(partitions[0]["total_count"], 2)
        self.assertEqual(api.calls, 2)

    def test_repository_heads_are_resolved_in_bounded_graphql_batches(self):
        class BatchedAPI:
            def __init__(self):
                self.calls: list[dict[str, object]] = []

            def graphql(self, query: str, variables: dict[str, object]):
                self.calls.append(dict(variables))
                count = len([key for key in variables if key.startswith("owner")])
                offset = (len(self.calls) - 1) * 50
                return {
                    f"r{index}": {
                        "nameWithOwner": f"owner/{variables[f'name{index}']}",
                        "isPrivate": False, "isArchived": False, "stargazerCount": index,
                        "pushedAt": "2026-08-27T00:00:00Z", "updatedAt": "2026-08-27T00:00:00Z",
                        "defaultBranchRef": {"target": {"oid": f"{offset + index:040x}"}},
                    }
                    for index in range(count)
                }

        api = BatchedAPI()
        repositories = [f"owner/repo-{index:03d}" for index in range(51)]
        states = repository_states(api, repositories)
        self.assertEqual(len(api.calls), 2)
        self.assertEqual([len(call) for call in api.calls], [100, 2])
        self.assertEqual(set(states), set(repositories))
        self.assertTrue(all(state["available"] for state in states.values()))

    def test_repository_lookup_preserves_github_owner_and_name_casing(self):
        class CasingAPI:
            def graphql(self, query: str, variables: dict[str, object]):
                self.variables = variables
                return {
                    "r0": {
                        "nameWithOwner": "MixedOwner/MixedRepo", "isPrivate": False, "isArchived": False,
                        "stargazerCount": 1, "pushedAt": "2026-08-27T00:00:00Z",
                        "updatedAt": "2026-08-27T00:00:00Z",
                        "defaultBranchRef": {"target": {"oid": "a" * 40}},
                    },
                }

        api = CasingAPI()
        states = repository_states(api, ["MixedOwner/MixedRepo"])
        self.assertEqual(api.variables, {"owner0": "MixedOwner", "name0": "MixedRepo"})
        self.assertIn("MixedOwner/MixedRepo", states)

    def test_exact_head_package_is_validated_without_execution(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            mirror, revision = create_mirror(root)
            pinned = PinnedRepository("owner/repo", revision, mirror)
            try:
                record = make_record(
                    pinned,
                    {"repository": "owner/repo", "revision": revision, "stars": 42, "updated_at": "2026-08-27T00:00:00Z"},
                    "packages/demo", "2026-08-27T00:00:00Z", None, {},
                )
            finally:
                pinned.close()
            self.assertEqual(record["revision"], revision)
            self.assertEqual(record["components"], {"extensions": 0, "mcp": 1, "skills": 0})
            self.assertEqual(record["mcp_transports"], ["streamable-http"])
            self.assertEqual(record["compatible_clients"], ["codex", "cursor", "copilot", "vscode", "kiro"])
            self.assertNotIn("chatgpt", record["compatible_clients"])
            self.assertEqual(record["runtime_reviewed"], False)

    def test_root_package_is_bounded_and_uses_canonical_slug(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            mirror, revision = create_mirror(root, package_path="")
            pinned = PinnedRepository("owner/repo", revision, mirror)
            try:
                record = make_record(
                    pinned,
                    {"repository": "owner/repo", "revision": revision, "stars": 42,
                     "updated_at": "2026-08-27T00:00:00Z"},
                    "", "2026-08-27T00:00:00Z", None, {},
                )
            finally:
                pinned.close()
            self.assertEqual(record["package_path"], "")
            self.assertEqual(record["slug"], "discovery:owner/repo")

    def test_build_candidate_is_complete_with_schema_invalid_diagnostic_excluded(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            mirror, revision = create_mirror(root)
            config = {
                "schema_version": 1,
                "query": '"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json" filename:plugin.json',
                "maximum_file_size": 10,
                "maximum_records": 100,
                "seeds": [],
            }
            candidate, diagnostics = build_candidate(
                api=FixtureAPI(revision), config=config, mode="discover", generated_at="2026-08-27T00:00:00Z",
                previous_records=[], mirror_root=mirror,
            )
            self.assertTrue(candidate["complete"])
            self.assertEqual(diagnostics, [])
            self.assertEqual(len(candidate["records"]), 1)
            self.assertEqual(candidate["mode"], "discover")

    def test_empty_refresh_continues_as_deterministic_discover(self):
        with tempfile.TemporaryDirectory() as temporary:
            mirror, revision = create_mirror(Path(temporary))
            config = {
                "schema_version": 1,
                "query": '"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json" filename:plugin.json',
                "maximum_file_size": 10,
                "maximum_records": 100,
                "seeds": [],
            }
            arguments = {
                "api": FixtureAPI(revision),
                "config": config,
                "generated_at": "2026-08-27T00:00:00Z",
                "previous_records": [],
                "mirror_root": mirror,
            }
            refreshed = build_candidate(mode="refresh", **arguments)
            discovered = build_candidate(mode="discover", **arguments)
        self.assertEqual(refreshed, discovered)
        self.assertEqual(refreshed[0]["mode"], "discover")
        self.assertEqual(len(refreshed[0]["records"]), 1)

    def test_refresh_retains_unavailable_previous_record_but_blocks_new_install(self):
        class MissingAPI:
            @staticmethod
            def graphql(query: str, variables: dict[str, object]):
                return {"r0": None}

        previous = candidate_record("a" * 40)
        candidate, diagnostics = build_candidate(
            api=MissingAPI(),
            config={"schema_version": 1, "query": "schema", "maximum_file_size": 10,
                    "maximum_records": 100, "seeds": []},
            mode="refresh", generated_at="2026-08-27T06:00:00Z",
            previous_records=[previous],
        )
        self.assertTrue(candidate["complete"])
        self.assertEqual(candidate["records"][0]["availability"], "unavailable")
        self.assertEqual(diagnostics[0]["kind"], "unavailable")

    def test_mass_availability_drop_marks_candidate_partial(self):
        class MissingBatchAPI:
            @staticmethod
            def graphql(query: str, variables: dict[str, object]):
                count = len([key for key in variables if key.startswith("owner")])
                return {f"r{index}": None for index in range(count)}

        previous = []
        for index in range(20):
            record = candidate_record(f"{index:040x}")
            record["repository"] = f"owner/repo-{index:02d}"
            record["slug"] = f"discovery:owner/repo-{index:02d}//packages/demo"
            previous.append(record)
        candidate, diagnostics = build_candidate(
            api=MissingBatchAPI(),
            config={"schema_version": 1, "query": "schema", "maximum_file_size": 10,
                    "maximum_records": 100, "seeds": []},
            mode="refresh", generated_at="2026-08-27T06:00:00Z",
            previous_records=previous,
        )
        self.assertFalse(candidate["complete"])
        self.assertEqual(sum(record["availability"] == "available" for record in candidate["records"]), 0)
        self.assertTrue(any(
            item["kind"] == "scan_error" and "preserving the last-known-good index" in item["error"]
            for item in diagnostics
        ))

    def test_refresh_does_not_fetch_unchanged_package_bytes(self):
        previous = candidate_record("a" * 40)
        with mock.patch("scripts.build_discovery_index.PinnedRepository") as pinned:
            candidate, diagnostics = build_candidate(
                api=FixtureAPI("a" * 40),
                config={"schema_version": 1, "query": "schema", "maximum_file_size": 10,
                        "maximum_records": 100, "seeds": []},
                mode="refresh", generated_at="2026-08-27T06:00:00Z",
                previous_records=[previous],
            )
        pinned.assert_not_called()
        self.assertTrue(candidate["complete"])
        self.assertEqual(diagnostics, [])
        self.assertEqual(candidate["mode"], "refresh")
        self.assertEqual(candidate["partitions"], [])
        self.assertEqual(candidate["records"][0]["last_seen"], "2026-08-27T06:00:00Z")
        self.assertEqual(candidate["records"][0]["revision"], "a" * 40)

    def test_repository_scans_are_bounded_reused_and_merged_deterministically(self):
        previous = []
        for repository, package_paths in {
            "owner/repo-a": ["packages/a", "packages/b"],
            "owner/repo-b": ["packages/a"],
            "owner/repo-c": ["packages/a"],
        }.items():
            for package_path in package_paths:
                record = candidate_record("a" * 40)
                record.update({
                    "repository": repository,
                    "owner": "owner",
                    "package_path": package_path,
                    "slug": f"discovery:{repository}//{package_path}",
                })
                previous.append(record)

        states = {
            repository: {
                "repository": repository, "revision": "b" * 40, "stars": 43,
                "updated_at": "2026-08-27T06:00:00Z", "available": True,
            }
            for repository in {record["repository"] for record in previous}
        }
        lock = threading.Lock()
        active = 0
        maximum_active = 0
        materializations: dict[str, int] = {}
        completion_order: list[str] = []

        class TrackingPinned:
            def __init__(self, repository: str, revision: str, mirror_root: Path | None,
                         *, github_token: str | None = None):
                nonlocal active, maximum_active
                del revision, mirror_root, github_token
                self.repository = repository
                with lock:
                    materializations[repository] = materializations.get(repository, 0) + 1
                    active += 1
                    maximum_active = max(maximum_active, active)

            def close(self):
                nonlocal active
                with lock:
                    active -= 1
                    completion_order.append(self.repository)

        def fake_make_record(pinned, state, package_path, generated_at, prior, reviewed):
            time.sleep({"owner/repo-a": 0.04, "owner/repo-b": 0.005, "owner/repo-c": 0.005}[pinned.repository])
            if package_path == "packages/a":
                raise ValueError(f"{pinned.repository} is invalid")
            return {
                **prior, "revision": state["revision"], "stars": state["stars"],
                "repository_updated_at": state["updated_at"], "last_seen": generated_at,
            }

        with mock.patch("scripts.build_discovery_index.repository_states", return_value=states), \
             mock.patch("scripts.build_discovery_index.PinnedRepository", TrackingPinned), \
             mock.patch("scripts.build_discovery_index.make_record", side_effect=fake_make_record):
            candidate, diagnostics = build_candidate(
                api=mock.Mock(),
                config={"schema_version": 1, "query": "schema", "maximum_file_size": 10,
                        "maximum_records": 100, "seeds": []},
                mode="refresh", generated_at="2026-08-27T06:00:00Z",
                previous_records=previous, repository_workers=2,
            )

        self.assertEqual(maximum_active, 2)
        self.assertEqual(materializations, {"owner/repo-a": 1, "owner/repo-b": 1, "owner/repo-c": 1})
        self.assertNotEqual(completion_order, sorted(completion_order))
        self.assertEqual(
            [(item["repository"], item["path"]) for item in diagnostics],
            [("owner/repo-a", "packages/a"), ("owner/repo-b", "packages/a"),
             ("owner/repo-c", "packages/a")],
        )
        self.assertTrue(candidate["complete"])
        self.assertEqual(
            [(record["repository"], record["package_path"]) for record in candidate["records"]],
            [("owner/repo-a", "packages/a"), ("owner/repo-a", "packages/b"),
             ("owner/repo-b", "packages/a"), ("owner/repo-c", "packages/a")],
        )

    def test_repository_scan_timeout_and_unexpected_exception_are_fail_closed(self):
        previous = []
        for repository in ("owner/repo-a", "owner/repo-b"):
            record = candidate_record("a" * 40)
            record.update({"repository": repository, "slug": f"discovery:{repository}//packages/demo"})
            previous.append(record)
        states = {
            repository: {
                "repository": repository, "revision": "b" * 40, "stars": 43,
                "updated_at": "2026-08-27T06:00:00Z", "available": True,
            }
            for repository in ("owner/repo-a", "owner/repo-b")
        }

        def failed_materialization(repository: str, revision: str, mirror_root: Path | None,
                                   *, github_token: str | None = None):
            del revision, mirror_root, github_token
            if repository == "owner/repo-a":
                raise BridgeError("Git invocation failed: timed out after 120 seconds")
            raise RuntimeError("unexpected worker failure")

        with mock.patch("scripts.build_discovery_index.repository_states", return_value=states), \
             mock.patch("scripts.build_discovery_index.PinnedRepository", side_effect=failed_materialization):
            candidate, diagnostics = build_candidate(
                api=mock.Mock(),
                config={"schema_version": 1, "query": "schema", "maximum_file_size": 10,
                        "maximum_records": 100, "seeds": []},
                mode="refresh", generated_at="2026-08-27T06:00:00Z",
                previous_records=previous, repository_workers=2,
            )

        self.assertFalse(candidate["complete"])
        self.assertEqual([item["repository"] for item in diagnostics], ["owner/repo-a", "owner/repo-b"])
        self.assertIn("timed out", diagnostics[0]["error"])
        self.assertIn("unexpected worker failure", diagnostics[1]["error"])
        self.assertEqual([record["revision"] for record in candidate["records"]], ["a" * 40, "a" * 40])

    def test_repository_worker_bound_is_validated(self):
        with self.assertRaisesRegex(DiscoveryError, "repository workers must be between 1 and 16"):
            build_candidate(
                api=mock.Mock(),
                config={"schema_version": 1, "query": "schema", "maximum_file_size": 10,
                        "maximum_records": 100, "seeds": []},
                mode="refresh", generated_at="2026-08-27T06:00:00Z",
                previous_records=[candidate_record("a" * 40)], repository_workers=17,
            )

    def test_repository_transfer_rebuilds_canonical_identity_even_at_same_revision(self):
        class TransferredAPI:
            def __init__(self, revision: str):
                self.revision = revision

            def graphql(self, query: str, variables: dict[str, object]):
                return {
                    "r0": {
                        "nameWithOwner": "new-owner/new-repo", "isPrivate": False, "isArchived": False,
                        "stargazerCount": 43, "pushedAt": "2026-08-27T06:00:00Z",
                        "updatedAt": "2026-08-27T06:00:00Z",
                        "defaultBranchRef": {"target": {"oid": self.revision}},
                    },
                }

            @staticmethod
            def get(path: str, parameters: dict[str, object] | None = None):
                return {"total_count": 1, "incomplete_results": False, "items": [
                    {"path": "packages/demo/plugin.json", "repository": {"full_name": "owner/repo"}},
                ]}

        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            mirror, revision = create_mirror(root, repository="new-owner/new-repo")
            previous = candidate_record(revision)
            candidate, diagnostics = build_candidate(
                api=TransferredAPI(revision),
                config={"schema_version": 1, "query": "schema", "maximum_file_size": 10,
                        "maximum_records": 100, "seeds": []},
                mode="reconcile", generated_at="2026-08-27T06:00:00Z",
                previous_records=[previous], mirror_root=mirror,
            )
        self.assertTrue(candidate["complete"])
        self.assertEqual(diagnostics, [])
        by_repository = {record["repository"]: record for record in candidate["records"]}
        self.assertEqual(by_repository["owner/repo"]["availability"], "unavailable")
        self.assertEqual(by_repository["new-owner/new-repo"]["availability"], "available")
        self.assertEqual(by_repository["new-owner/new-repo"]["slug"], "discovery:new-owner/new-repo//packages/demo")

    def test_duplicate_previous_identity_is_rejected(self):
        first = candidate_record("a" * 40)
        duplicate = dict(first)
        duplicate["repository"] = "OWNER/REPO"
        with self.assertRaisesRegex(DiscoveryError, "duplicate package identity"):
            build_candidate(
                api=FixtureAPI("a" * 40),
                config={"schema_version": 1, "query": "schema", "maximum_file_size": 10,
                        "maximum_records": 100, "seeds": []},
                mode="refresh", generated_at="2026-08-27T06:00:00Z",
                previous_records=[first, duplicate],
            )

    def test_partial_candidate_never_replaces_last_known_good(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            feed = root / "feed"
            feed.mkdir()
            candidate = root / "candidate.json"
            candidate.write_bytes(canonical_json({
                "candidate_schema_version": 1,
                "mode": "refresh",
                "generated_at": "2026-08-27T00:00:00Z",
                "complete": False,
                "query_manifest_digest": "sha256:" + "3" * 64,
                "partitions": [],
                "records": [],
            }))
            with self.assertRaisesRegex(PublicationError, "partial Discovery candidate"):
                publish(candidate, feed, root / "unused.json", bytes(range(32)), "unused", "run-1", "b" * 40, 3)
            self.assertFalse((feed / "latest.json").exists())
            self.assertEqual(list(feed.rglob("*")), [])

    def test_failed_bundle_verification_never_advances_latest_pointer(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            feed = root / "feed"
            feed.mkdir()
            candidate = root / "candidate.json"
            candidate.write_bytes(canonical_json({
                "candidate_schema_version": 1,
                "mode": "discover",
                "generated_at": "2026-08-27T00:00:00Z",
                "complete": True,
                "query_manifest_digest": "sha256:" + "3" * 64,
                "partitions": [],
                "records": [candidate_record("a" * 40)],
            }))
            with mock.patch.object(discovery_publication, "ed25519_sign", return_value=b"0" * 64), \
                 mock.patch.object(
                     discovery_publication, "verify_bundle",
                     side_effect=PublicationError("verification failed"),
                 ):
                with self.assertRaisesRegex(PublicationError, "verification failed"):
                    publish(
                        candidate, feed, root / "unused.json", bytes(range(32)),
                        "discovery-test", "run-1", "b" * 40, 3,
                    )
            self.assertFalse((feed / "latest.json").exists())
            self.assertEqual(list(feed.rglob("*")), [])

    def test_discovery_modules_share_one_publication_error_type(self):
        from scripts import directory_publication

        self.assertIs(discovery_publication.PublicationError, directory_publication.PublicationError)

    def test_signed_feed_is_append_only_and_search_tampering_fails(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            feed = root / "feed"
            feed.mkdir()
            seed = bytes(range(32))
            private = Ed25519PrivateKey.from_private_bytes(seed)
            public = private.public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw)
            trusted = root / "trusted.json"
            trusted.write_bytes(canonical_json({
                "schema_version": 1,
                "keys": [{"key_id": "discovery-test", "public_key": base64.b64encode(public).decode("ascii")}],
            }))
            revision = "a" * 40
            candidate = root / "candidate.json"
            candidate.write_bytes(canonical_json({
                "candidate_schema_version": 1,
                "mode": "discover",
                "generated_at": "2026-08-27T00:00:00Z",
                "complete": True,
                "query_manifest_digest": "sha256:" + "3" * 64,
                "partitions": [{"query": "query size:0..10", "size_min": 0, "size_max": 10, "total_count": 1}],
                "records": [candidate_record(revision)],
            }))
            def verify(public_bytes: bytes, message: bytes, signature: bytes) -> None:
                Ed25519PublicKey.from_public_bytes(public_bytes).verify(signature, message)

            with mock.patch.object(discovery_publication, "ed25519_sign", side_effect=lambda _seed, message: private.sign(message)), \
                 mock.patch.object(discovery_publication, "ed25519_verify", side_effect=verify):
                first = publish(candidate, feed, trusted, seed, "discovery-test", "run-1", "b" * 40, 3)
                self.assertEqual(first["sequence"], 1)
                self.assertEqual(load_latest(feed, trusted)[0]["records"][0]["revision"], revision)
                candidate_value = json.loads(candidate.read_text())
                candidate_value["generated_at"] = "2026-08-27T06:00:00Z"
                candidate_value["records"][0]["last_seen"] = "2026-08-27T06:00:00Z"
                candidate.write_bytes(canonical_json(candidate_value))
                second = publish(candidate, feed, trusted, seed, "discovery-test", "run-2", "c" * 40, 3)
                self.assertEqual(second["sequence"], 2)
                latest = json.loads((feed / "latest.json").read_text())
                search = feed / latest["search_path"]
                original = search.read_bytes()
                tampered = json.loads(original)
                tampered["records"][0]["name"] = "tampered"
                search.write_bytes(canonical_json(tampered))
                with self.assertRaises(PublicationError):
                    load_latest(feed, trusted)
                search.write_bytes(original)
                self.assertEqual(load_latest(feed, trusted)[0]["sequence"], 2)


if __name__ == "__main__":
    unittest.main()
