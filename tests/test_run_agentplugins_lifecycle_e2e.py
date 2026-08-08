from __future__ import annotations

import importlib.util
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "scripts" / "run_agentplugins_lifecycle_e2e.py"
SPEC = importlib.util.spec_from_file_location("run_agentplugins_lifecycle_e2e", MODULE_PATH)
assert SPEC and SPEC.loader
e2e = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(e2e)

HERO_MODULE_PATH = ROOT / "scripts" / "run_agentplugins_hero_matrix_e2e.py"
HERO_SPEC = importlib.util.spec_from_file_location(
    "run_agentplugins_hero_matrix_e2e", HERO_MODULE_PATH
)
assert HERO_SPEC and HERO_SPEC.loader
hero_e2e = importlib.util.module_from_spec(HERO_SPEC)
HERO_SPEC.loader.exec_module(hero_e2e)

CATALOG_URL = (
    "https://raw.githubusercontent.com/777genius/universal-agent-plugins/"
    "03ff2da92135acc0fdd49e40ac4e385cb0f51b15/catalog/v1/catalog.json"
)
CATALOG_DIGEST = "sha256:" + "a" * 64


class AgentpluginsLifecycleE2ETests(unittest.TestCase):
    def test_isolated_environment_does_not_inherit_credentials(self) -> None:
        inherited = {
            "PATH": "/usr/bin:/bin",
            "LANG": "C.UTF-8",
            "AWS_SECRET_ACCESS_KEY": "secret",
            "GITHUB_TOKEN": "secret",
            "NPM_TOKEN": "secret",
            "SSH_AUTH_SOCK": "/secret/socket",
            "AGENTPLUGINS_CATALOG_URL": "https://user:secret@example.test/catalog.json",
            "AGENTPLUGINS_CATALOG_DIGEST": "sha256:" + "b" * 64,
        }
        with tempfile.TemporaryDirectory() as tmp, mock.patch.dict(
            os.environ, inherited, clear=True
        ):
            environment = e2e.isolated_environment(
                Path(tmp), CATALOG_URL, CATALOG_DIGEST
            )
        self.assertEqual(environment["PATH"], inherited["PATH"])
        self.assertEqual(environment["LANG"], inherited["LANG"])
        for name in {
            "AWS_SECRET_ACCESS_KEY",
            "GITHUB_TOKEN",
            "NPM_TOKEN",
            "SSH_AUTH_SOCK",
        }:
            self.assertNotIn(name, environment)
        self.assertEqual(environment["GIT_TERMINAL_PROMPT"], "0")
        self.assertEqual(environment["CI"], "true")
        self.assertEqual(environment["AGENTPLUGINS_CATALOG_URL"], CATALOG_URL)
        self.assertEqual(
            environment["AGENTPLUGINS_CATALOG_DIGEST"], CATALOG_DIGEST
        )

    def test_catalog_input_rejects_credentials_and_unpinned_values(self) -> None:
        for module in (e2e, hero_e2e):
            with self.subTest(module=module.__name__, case="http"), self.assertRaises(
                ValueError
            ):
                module.catalog_environment("http://example.test/catalog.json", CATALOG_DIGEST)
            with self.subTest(
                module=module.__name__, case="credentials"
            ), self.assertRaises(ValueError):
                module.catalog_environment(
                    "https://user:secret@example.test/catalog.json", CATALOG_DIGEST
                )
            with self.subTest(
                module=module.__name__, case="digest"
            ), self.assertRaises(ValueError):
                module.catalog_environment(CATALOG_URL, "sha256:not-pinned")

    def test_binary_version_is_semantic_and_exact(self) -> None:
        success = SimpleNamespace(stdout="agentplugins 0.1.1\n")
        for module in (e2e, hero_e2e):
            with self.subTest(module=module.__name__), mock.patch.object(
                module.subprocess, "run", return_value=success
            ):
                self.assertEqual(
                    module.binary_version(
                        Path("/tmp/agentplugins"), Path("/tmp"), {}, "0.1.1"
                    ),
                    "0.1.1",
                )
            for output in (
                "agentplugins development\n",
                "agentplugins 0.1\n",
                "agentplugins 0.1.2\n",
            ):
                with self.subTest(
                    module=module.__name__, output=output
                ), mock.patch.object(
                    module.subprocess,
                    "run",
                    return_value=SimpleNamespace(stdout=output),
                ), self.assertRaises(RuntimeError):
                    module.binary_version(
                        Path("/tmp/agentplugins"), Path("/tmp"), {}, "0.1.1"
                    )

    def test_cli_timeout_names_plugin_and_operation(self) -> None:
        with mock.patch.object(
            e2e.subprocess,
            "run",
            side_effect=subprocess.TimeoutExpired(["agentplugins"], 120),
        ), self.assertRaisesRegex(RuntimeError, "context7: add timed out after 120s"):
            e2e.run_cli(
                Path("/tmp/agentplugins"),
                "add",
                "context7",
                Path("/tmp/sandbox"),
                {},
                check=True,
            )


if __name__ == "__main__":
    unittest.main()
