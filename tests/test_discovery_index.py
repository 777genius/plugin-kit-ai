from __future__ import annotations

import base64
import io
import json
import subprocess
import tempfile
import unittest
import urllib.error
import urllib.request
from pathlib import Path
from unittest import mock

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey, Ed25519PublicKey

from scripts.build_bridges import PinnedRepository
from scripts.build_discovery_index import (
    DiscoveryError,
    GitHubAPI,
    SameOriginRedirect,
    build_candidate,
    discover_search_items,
    make_record,
    repository_states,
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


class DiscoveryIndexTests(unittest.TestCase):
    def test_github_api_redirect_cannot_exfiltrate_the_job_token(self):
        handler = SameOriginRedirect("https://api.github.com")
        request = urllib.request.Request(
            "https://api.github.com/repos/owner/repo",
            headers={"Authorization": "Bearer secret"},
        )
        with self.assertRaisesRegex(DiscoveryError, "cross-origin redirect"):
            handler.redirect_request(request, None, 302, "Found", {}, "https://attacker.example/token")

    def test_github_api_retries_transient_server_failure(self):
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = b'{"ok":true}'
        api = GitHubAPI("test-token")
        api.opener = mock.Mock()
        api.opener.open.side_effect = [
            urllib.error.HTTPError("https://api.github.com/test", 503, "Unavailable", {}, io.BytesIO(b"retry")),
            response,
        ]
        with mock.patch("scripts.build_discovery_index.time.sleep"):
            self.assertEqual(api.get("test"), {"ok": True})
        self.assertEqual(api.opener.open.call_count, 2)

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
        self.assertEqual(candidate["records"][0]["last_seen"], "2026-08-27T06:00:00Z")
        self.assertEqual(candidate["records"][0]["revision"], "a" * 40)

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
