from __future__ import annotations

import hashlib
import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path


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


if __name__ == "__main__":
    unittest.main()
