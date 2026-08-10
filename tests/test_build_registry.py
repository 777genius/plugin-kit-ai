from __future__ import annotations

import gzip
import importlib.util
import io
import json
import tarfile
import tempfile
import unittest
from pathlib import Path
from unittest import mock


MODULE_PATH = Path(__file__).resolve().parents[1] / "scripts" / "build_registry.py"
SPEC = importlib.util.spec_from_file_location("build_registry", MODULE_PATH)
assert SPEC and SPEC.loader
registry = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(registry)


class FakeResponse:
    def __init__(self, body: bytes, url: str, *, status: int = 200, length: str | None = None):
        self._body = io.BytesIO(body)
        self._url = url
        self.status = status
        self.headers = {} if length is None else {"Content-Length": length}

    def read(self, size: int = -1) -> bytes:
        return self._body.read(size)

    def geturl(self) -> str:
        return self._url

    def __enter__(self):
        return self

    def __exit__(self, *_args):
        return None


class FakeOpener:
    def __init__(self, response: FakeResponse):
        self.response = response
        self.request = None
        self.timeout = None

    def open(self, request, timeout=None):
        self.request, self.timeout = request, timeout
        return self.response


def archive_bytes(entries: list[tuple[str, bytes | None, str]], root: str = "repo-deadbeef") -> bytes:
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode="w:gz") as archive:
        for name, body, kind in entries:
            info = tarfile.TarInfo(f"{root}/{name}")
            if kind == "dir":
                info.type = tarfile.DIRTYPE
                info.size = 0
                archive.addfile(info)
            elif kind == "symlink":
                info.type = tarfile.SYMTYPE
                info.linkname = "../../escape"
                archive.addfile(info)
            elif kind == "fifo":
                info.type = tarfile.FIFOTYPE
                archive.addfile(info)
            elif kind == "sparse":
                info.type = tarfile.REGTYPE
                info.size = 1
                info.pax_headers = {"GNU.sparse.map": "0,1"}
                archive.addfile(info, io.BytesIO(b"x"))
            else:
                assert body is not None
                info.size = len(body)
                archive.addfile(info, io.BytesIO(body))
    return output.getvalue()


def valid_entries(name: str = "demo") -> list[tuple[str, bytes | None, str]]:
    manifest = {
        "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
        "name": name,
        "version": "1.2.3",
        "description": "Pinned demo plugin",
        "author": {"name": "Example Author", "url": "https://github.com/example"},
        "repository": "https://github.com/example/plugins",
        "license": "Apache-2.0",
        "keywords": ["demo"],
    }
    mcp = {
        "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
        "mcpServers": {"demo": {"type": "streamable-http", "url": "https://example.com/mcp"}},
    }
    base = f"packages/{name}"
    return [
        (base, None, "dir"),
        (f"{base}/plugin.json", json.dumps(manifest).encode(), "file"),
        (f"{base}/README.md", b"# Demo\n", "file"),
        (f"{base}/mcp.json", json.dumps(mcp).encode(), "file"),
    ]


class RegistryDescriptorTests(unittest.TestCase):
    def descriptor(self, root: Path, **updates) -> Path:
        value = {"schema_version": 1, "repository": "example/plugins", "revision": "a" * 40, "path": "packages/demo", "categories": ["developer-tools"]}
        value.update(updates)
        path = root / "demo.json"
        path.write_text(json.dumps(value))
        return path

    def test_valid_descriptor(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            value = registry.validate_descriptor(self.descriptor(Path(tmp)))
            self.assertEqual(value["name"], "demo")

    def test_rejects_mutable_ref_credentials_and_url_syntax(self) -> None:
        invalid = [
            {"revision": "main"},
            {"repository": "user:token@github.com/example/plugins"},
            {"repository": "https://github.com/example/plugins?x=1"},
            {"repository": "Example/plugins"},
            {"repository": "example/plugins.git"},
        ]
        for update in invalid:
            with self.subTest(update=update), tempfile.TemporaryDirectory() as tmp:
                with self.assertRaises(registry.RegistryError):
                    registry.validate_descriptor(self.descriptor(Path(tmp), **update))

    def test_rejects_traversal_absolute_ambiguous_and_unicode_paths(self) -> None:
        for value in ["../demo", "/demo", "packages//demo", "packages/./demo", "packages\\demo", "packages/%64emo", "packages/de\u0301mo"]:
            with self.subTest(path=value), tempfile.TemporaryDirectory() as tmp:
                with self.assertRaises(registry.RegistryError):
                    registry.validate_descriptor(self.descriptor(Path(tmp), path=value))

    def test_rejects_unsorted_duplicate_or_invalid_categories(self) -> None:
        for value in [["z", "a"], ["a", "a"], ["Not-Slug"], [], ["a"] * 9]:
            with self.subTest(categories=value), tempfile.TemporaryDirectory() as tmp:
                with self.assertRaises(registry.RegistryError):
                    registry.validate_descriptor(self.descriptor(Path(tmp), categories=value))

    def test_submitter_cannot_assign_claim_fields(self) -> None:
        for field in ["featured", "verified", "official", "tested", "downloads", "ranking", "name", "description"]:
            with self.subTest(field=field), tempfile.TemporaryDirectory() as tmp:
                with self.assertRaises(registry.RegistryError):
                    registry.validate_descriptor(self.descriptor(Path(tmp), **{field: True}))

    def test_rejects_duplicate_keys_and_nonstandard_json_numbers(self) -> None:
        documents = [
            '{"schema_version":1,"schema_version":1}',
            '{"repository":"example/plugins","Repository":"example/plugins"}',
            '{"café":1,"cafe\\u0301":2}',
            '{"schema_version":NaN}',
        ]
        for document in documents:
            with self.subTest(document=document), tempfile.TemporaryDirectory() as tmp:
                path = Path(tmp) / "demo.json"
                path.write_text(document)
                with self.assertRaises(registry.RegistryError):
                    registry.validate_descriptor(path)


class NetworkLimitTests(unittest.TestCase):
    def download(self, response: FakeResponse, destination: Path) -> None:
        registry.download_archive("example/plugins", "a" * 40, destination, FakeOpener(response))

    def test_uses_only_exact_approved_archive_url_and_timeout(self) -> None:
        url = registry.archive_url("example/plugins", "a" * 40)
        opener = FakeOpener(FakeResponse(b"abc", url))
        with tempfile.TemporaryDirectory() as tmp:
            registry.download_archive("example/plugins", "a" * 40, Path(tmp) / "a.tgz", opener)
        self.assertEqual(opener.request.full_url, url)
        self.assertEqual(opener.timeout, registry.CONNECT_TIMEOUT_SECONDS)

    def test_rejects_redirect_or_unapproved_final_host(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            response = FakeResponse(b"x", "https://evil.example/archive")
            with self.assertRaises(registry.RegistryError):
                self.download(response, Path(tmp) / "a.tgz")

    def test_rejects_bad_status_and_content_length(self) -> None:
        url = registry.archive_url("example/plugins", "a" * 40)
        responses = [FakeResponse(b"", url, status=404), FakeResponse(b"", url, length=str(registry.MAX_DOWNLOAD_BYTES + 1)), FakeResponse(b"", url, length="-1")]
        for response in responses:
            with self.subTest(status=response.status, headers=response.headers), tempfile.TemporaryDirectory() as tmp:
                with self.assertRaises(registry.RegistryError):
                    self.download(response, Path(tmp) / "a.tgz")

    def test_enforces_streamed_compressed_size(self) -> None:
        url = registry.archive_url("example/plugins", "a" * 40)
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(registry, "MAX_DOWNLOAD_BYTES", 4):
            with self.assertRaises(registry.RegistryError):
                self.download(FakeResponse(b"12345", url), Path(tmp) / "a.tgz")

    def test_enforces_total_elapsed_time(self) -> None:
        url = registry.archive_url("example/plugins", "a" * 40)
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(registry.time, "monotonic", side_effect=[0, registry.TOTAL_DOWNLOAD_SECONDS + 1]):
            with self.assertRaises(registry.RegistryError):
                self.download(FakeResponse(b"x", url), Path(tmp) / "a.tgz")


class ArchiveLimitTests(unittest.TestCase):
    def extract(self, body: bytes, destination: Path) -> None:
        compressed, expanded = destination.parent / "a.tgz", destination.parent / "a.tar"
        compressed.write_bytes(body)
        registry.decompress_archive(compressed, expanded)
        destination.mkdir()
        registry.extract_package(expanded, "packages/demo", destination)

    def test_valid_package_extracts(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            destination = Path(tmp) / "package"
            self.extract(archive_bytes(valid_entries()), destination)
            self.assertTrue((destination / "plugin.json").is_file())

    def test_rejects_links_special_and_sparse_files(self) -> None:
        for kind in ["symlink", "fifo", "sparse"]:
            entries = valid_entries() + [(f"packages/demo/{kind}", None, kind)]
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as tmp:
                with self.assertRaises(registry.RegistryError):
                    self.extract(archive_bytes(entries), Path(tmp) / "package")

    def test_rejects_archive_traversal_absolute_ambiguous_and_unicode(self) -> None:
        names = ["../escape", "/absolute", "packages/demo/bad\\name", "packages/demo/de\u0301mo"]
        for name in names:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as tmp:
                with self.assertRaises(registry.RegistryError):
                    self.extract(archive_bytes(valid_entries() + [(name, b"x", "file")]), Path(tmp) / "package")

    def test_enforces_file_count_per_file_total_and_depth(self) -> None:
        cases = [
            ("MAX_FILES", 1, valid_entries() + [("packages/demo/extra", b"x", "file")]),
            ("MAX_FILE_BYTES", 1, valid_entries() + [("packages/demo/extra", b"xx", "file")]),
            ("MAX_EXTRACTED_BYTES", 3, valid_entries()),
            ("MAX_PATH_DEPTH", 2, valid_entries() + [("packages/demo/a/b/c", b"x", "file")]),
        ]
        for constant, limit, entries in cases:
            with self.subTest(limit=constant), tempfile.TemporaryDirectory() as tmp, mock.patch.object(registry, constant, limit):
                with self.assertRaises(registry.RegistryError):
                    self.extract(archive_bytes(entries), Path(tmp) / "package")

    def test_enforces_expanded_archive_limit(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            compressed, expanded = Path(tmp) / "a.gz", Path(tmp) / "a"
            compressed.write_bytes(gzip.compress(b"12345"))
            with mock.patch.object(registry, "MAX_ARCHIVE_BYTES", 4), self.assertRaises(registry.RegistryError):
                registry.decompress_archive(compressed, expanded)

    def test_enforces_archive_processing_time(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            compressed, expanded = Path(tmp) / "a.gz", Path(tmp) / "a"
            compressed.write_bytes(gzip.compress(b"12345"))
            with mock.patch.object(registry.time, "monotonic", side_effect=[0, registry.ARCHIVE_PROCESS_SECONDS + 1]), self.assertRaises(registry.RegistryError):
                registry.decompress_archive(compressed, expanded)

    def test_rejects_case_collisions_and_multiple_roots(self) -> None:
        collisions = valid_entries() + [("packages/demo/README.MD", b"x", "file")]
        roots = archive_bytes(valid_entries())
        # A second tar root is constructed explicitly because archive_bytes prefixes all entries.
        output = io.BytesIO()
        with tarfile.open(fileobj=output, mode="w:gz") as archive:
            for name in ["root-a/packages/demo", "root-b/other"]:
                info = tarfile.TarInfo(name)
                info.type = tarfile.DIRTYPE
                archive.addfile(info)
        for body in [archive_bytes(collisions), output.getvalue()]:
            with tempfile.TemporaryDirectory() as tmp, self.assertRaises(registry.RegistryError):
                self.extract(body, Path(tmp) / "package")

    def test_rejects_duplicates_outside_selected_package_and_member_floods(self) -> None:
        duplicate = valid_entries() + [("other/file", b"one", "file"), ("OTHER/FILE", b"two", "file")]
        with tempfile.TemporaryDirectory() as tmp, self.assertRaises(registry.RegistryError):
            self.extract(archive_bytes(duplicate), Path(tmp) / "package")
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(registry, "MAX_MEMBERS", 1), self.assertRaises(registry.RegistryError):
            self.extract(archive_bytes(valid_entries()), Path(tmp) / "package")


class ExternalPackageTests(unittest.TestCase):
    def test_external_entry_is_pinned_schema_only_and_derived_from_package(self) -> None:
        body = archive_bytes(valid_entries() + [("packages/demo/icon.svg", b"<svg/>", "file")])
        descriptor = {"name": "demo", "repository": "example/plugins", "revision": "a" * 40, "path": "packages/demo", "categories": ["developer-tools"]}
        url = registry.archive_url("example/plugins", "a" * 40)
        item = registry.external_entry(descriptor, FakeOpener(FakeResponse(body, url)))
        self.assertEqual(item["install_source"], f"example/plugins@{'a' * 40}//packages/demo")
        self.assertEqual(item["author"]["name"], "Example Author")
        self.assertEqual(item["license"], "Apache-2.0")
        self.assertEqual(item["validation"]["level"], "schema_only")
        self.assertEqual(item["client_support"], {"resolution": "install_time", "clients": list(registry.CLIENT_IDS)})
        self.assertRegex(item["source"]["tree_sha256"], r"^sha256:[0-9a-f]{64}$")
        self.assertNotIn("icon", item)
        self.assertNotIn("icon_sha256", item["source"])

    def test_rejects_duplicate_keys_in_component_json(self) -> None:
        entries = valid_entries()
        entries[3] = (entries[3][0], b'{"mcpServers":{},"mcpServers":{}}', "file")
        descriptor = {"name": "demo", "repository": "example/plugins", "revision": "a" * 40, "path": "packages/demo", "categories": ["developer-tools"]}
        url = registry.archive_url("example/plugins", "a" * 40)
        with self.assertRaises(registry.RegistryError):
            registry.external_entry(descriptor, FakeOpener(FakeResponse(archive_bytes(entries), url)))

    def test_allows_array_data_json_without_weakening_duplicate_checks(self) -> None:
        entries = valid_entries() + [("packages/demo/data.json", b'[1, 2, 3]', "file")]
        descriptor = {"name": "demo", "repository": "example/plugins", "revision": "a" * 40, "path": "packages/demo", "categories": ["developer-tools"]}
        url = registry.archive_url("example/plugins", "a" * 40)
        item = registry.external_entry(descriptor, FakeOpener(FakeResponse(archive_bytes(entries), url)))
        self.assertEqual(item["name"], "demo")

    def test_rejects_manifest_name_or_repository_mismatch_and_missing_license(self) -> None:
        mutations = [("name", "other"), ("repository", "https://github.com/other/plugins"), ("license", "")]
        for field, value in mutations:
            entries = valid_entries()
            manifest = json.loads(entries[1][1])
            manifest[field] = value
            entries[1] = (entries[1][0], json.dumps(manifest).encode(), "file")
            descriptor = {"name": "demo", "repository": "example/plugins", "revision": "a" * 40, "path": "packages/demo", "categories": ["developer-tools"]}
            url = registry.archive_url("example/plugins", "a" * 40)
            with self.subTest(field=field), self.assertRaises(registry.RegistryError):
                registry.external_entry(descriptor, FakeOpener(FakeResponse(archive_bytes(entries), url)))

    def test_existing_mcp_secret_checks_are_preserved(self) -> None:
        entries = valid_entries()
        mcp = json.loads(entries[3][1])
        mcp["mcpServers"]["demo"]["headers"] = {"Authorization": "token"}
        entries[3] = (entries[3][0], json.dumps(mcp).encode(), "file")
        descriptor = {"name": "demo", "repository": "example/plugins", "revision": "a" * 40, "path": "packages/demo", "categories": ["developer-tools"]}
        url = registry.archive_url("example/plugins", "a" * 40)
        with self.assertRaises(registry.RegistryError):
            registry.external_entry(descriptor, FakeOpener(FakeResponse(archive_bytes(entries), url)))


class GeneratedIndexTests(unittest.TestCase):
    def test_committed_index_is_deterministic_complete_and_sorted(self) -> None:
        first = registry.encoded(registry.build())
        second = registry.encoded(registry.build())
        self.assertEqual(first, second)
        self.assertEqual(first, registry.OUTPUT.read_bytes())
        index = json.loads(first)
        self.assertEqual(index["schema_version"], 1)
        self.assertEqual(len(index["plugins"]), 26)
        names = [item["name"] for item in index["plugins"]]
        self.assertEqual(names, sorted(names))
        required = {"name", "version", "description", "author", "license", "categories", "keywords", "source", "install_source", "built_in", "client_support", "validation", "components"}
        for item in index["plugins"]:
            self.assertTrue(required.issubset(item))
            self.assertTrue(item["built_in"])
            self.assertEqual(item["install_source"], item["name"])
            self.assertEqual(item["client_support"]["resolution"], "catalog")
            self.assertTrue(item["client_support"]["clients"])
        context7 = next(item for item in index["plugins"] if item["name"] == "context7")
        cloudflare_docs = next(item for item in index["plugins"] if item["name"] == "cloudflare-docs")
        self.assertNotIn("chatgpt", context7["client_support"]["clients"])
        self.assertIn("chatgpt", cloudflare_docs["client_support"]["clients"])

    def test_builtin_name_cannot_be_claimed_by_descriptor(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            entries = Path(tmp)
            descriptor = {"schema_version": 1, "repository": "example/plugins", "revision": "a" * 40, "path": "packages/demo", "categories": ["developer-tools"]}
            (entries / "demo.json").write_text(json.dumps(descriptor))
            builtin = {"name": "demo"}
            with mock.patch.object(registry, "ENTRIES", entries), mock.patch.object(registry, "builtin_entries", return_value=[builtin]), self.assertRaises(registry.RegistryError):
                registry.build()


if __name__ == "__main__":
    unittest.main()
