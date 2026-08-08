from __future__ import annotations

import hashlib
import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
SCRIPTS = ROOT / "scripts"
sys.path.insert(0, str(SCRIPTS))
SPEC = importlib.util.spec_from_file_location("build_agentplugins_catalog", SCRIPTS / "build_agentplugins_catalog.py")
assert SPEC and SPEC.loader
builder = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(builder)


class AgentpluginsCatalogBuilderTests(unittest.TestCase):
    def test_committed_catalog_is_reproducible(self) -> None:
        current = json.loads((ROOT / "catalog" / "v1" / "catalog.json").read_text())
        rebuilt = builder.build(current["revision"], current["published_at"])
        self.assertEqual(rebuilt, current)
        self.assertEqual(len(rebuilt["plugins"]), 26)

    def test_tree_digest_matches_engine_header_contract(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "bin").mkdir()
            executable = root / "bin" / "run"
            executable.write_bytes(b"run\n")
            executable.chmod(0o755)
            (root / "plugin.json").write_bytes(b"{}\n")
            digest = hashlib.sha256()
            digest.update(b"dir\0bin\0false\0" + b"0\0")
            digest.update(b"file\0bin/run\0true\0" + b"4\0run\n")
            digest.update(b"file\0plugin.json\0false\0" + b"3\0{}\n")
            self.assertEqual(builder.package_tree_digest(root), "sha256:" + digest.hexdigest())

    def test_same_version_catalog_entries_are_content_pinned(self) -> None:
        current = json.loads((ROOT / "catalog" / "v1" / "catalog.json").read_text())
        for plugin in current["plugins"]:
            self.assertRegex(plugin["tree_digest"], r"^sha256:[0-9a-f]{64}$")
            self.assertRegex(plugin["manifest_digest"], r"^sha256:[0-9a-f]{64}$")
            self.assertEqual(set(plugin["compatibility"]), {"codex", "cursor", "copilot", "vscode", "kiro"})

    def test_runtime_tested_claims_match_pinned_hero_evidence(self) -> None:
        current = json.loads((ROOT / "catalog" / "v1" / "catalog.json").read_text())
        evidence = json.loads(
            (ROOT / "tests" / "e2e" / "results" / "agentplugins-hero-runtime-matrix-2026-08-08.json").read_text()
        )
        comparison = subprocess.run(
            [
                "git",
                "diff",
                "--quiet",
                evidence["source"]["commit_sha"],
                current["revision"],
                "--",
                "plugins",
            ],
            cwd=ROOT,
            check=False,
        )
        self.assertEqual(comparison.returncode, 0, "runtime evidence does not match the catalog package tree")
        client_ids = {"Codex CLI": "codex", "Cursor Agent": "cursor", "Kiro CLI": "kiro"}
        evidenced = {
            (check["plugin"], client_ids[check["client"]])
            for check in evidence["checks"]
            if check["status"] == "passed" and check["client"] in client_ids
        }
        claimed = {
            (plugin["name"], client)
            for plugin in current["plugins"]
            for client, status in plugin["compatibility"].items()
            if status["verification"] == "tested"
        }
        self.assertEqual(claimed, evidenced)

    def test_manifest_name_must_match_hashed_directory(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            plugins = Path(tmp)
            package = plugins / "actual-directory"
            package.mkdir()
            (package / "plugin.json").write_text(
                json.dumps(
                    {
                        "$schema": builder.PLUGIN_SCHEMA,
                        "name": "different-name",
                        "version": "1.0.0",
                    }
                )
            )
            with mock.patch.object(builder, "PLUGINS", plugins), self.assertRaisesRegex(
                ValueError, "does not match directory"
            ):
                builder.build("a" * 40, "2026-08-08T00:00:00Z")

    def test_git_diff_distinguishes_drift_from_execution_failure(self) -> None:
        with mock.patch.object(
            builder.subprocess,
            "run",
            side_effect=[
                SimpleNamespace(returncode=0, stderr=""),
                SimpleNamespace(returncode=1, stderr=""),
            ],
        ), self.assertRaisesRegex(ValueError, "differs from"):
            builder.ensure_plugins_match_revision("a" * 40)
        with mock.patch.object(
            builder.subprocess,
            "run",
            side_effect=[
                SimpleNamespace(returncode=0, stderr=""),
                SimpleNamespace(returncode=128, stderr="fatal: bad object"),
            ],
        ), self.assertRaisesRegex(ValueError, "fatal: bad object"):
            builder.ensure_plugins_match_revision("a" * 40)

    def test_catalog_revision_must_be_an_ancestor_of_the_catalog_commit(self) -> None:
        with mock.patch.object(
            builder.subprocess,
            "run",
            return_value=SimpleNamespace(returncode=1, stderr=""),
        ), self.assertRaisesRegex(ValueError, "must be an ancestor"):
            builder.ensure_plugins_match_revision("a" * 40)


if __name__ == "__main__":
    unittest.main()
