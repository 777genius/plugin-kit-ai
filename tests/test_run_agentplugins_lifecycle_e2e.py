from __future__ import annotations

import importlib.util
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "scripts" / "run_agentplugins_lifecycle_e2e.py"
SPEC = importlib.util.spec_from_file_location("run_agentplugins_lifecycle_e2e", MODULE_PATH)
assert SPEC and SPEC.loader
e2e = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(e2e)


class AgentpluginsLifecycleE2ETests(unittest.TestCase):
    def test_isolated_environment_does_not_inherit_credentials(self) -> None:
        inherited = {
            "PATH": "/usr/bin:/bin",
            "LANG": "C.UTF-8",
            "AWS_SECRET_ACCESS_KEY": "secret",
            "GITHUB_TOKEN": "secret",
            "NPM_TOKEN": "secret",
            "SSH_AUTH_SOCK": "/secret/socket",
        }
        with tempfile.TemporaryDirectory() as tmp, mock.patch.dict(
            os.environ, inherited, clear=True
        ):
            environment = e2e.isolated_environment(Path(tmp))
        self.assertEqual(environment["PATH"], inherited["PATH"])
        self.assertEqual(environment["LANG"], inherited["LANG"])
        for name in inherited.keys() - {"PATH", "LANG"}:
            self.assertNotIn(name, environment)
        self.assertEqual(environment["GIT_TERMINAL_PROMPT"], "0")
        self.assertEqual(environment["CI"], "true")

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
