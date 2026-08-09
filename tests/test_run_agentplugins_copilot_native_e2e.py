from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
SCRIPTS = ROOT / "scripts"
sys.path.insert(0, str(SCRIPTS))
MODULE_PATH = SCRIPTS / "run_agentplugins_copilot_native_e2e.py"
SPEC = importlib.util.spec_from_file_location(
    "run_agentplugins_copilot_native_e2e", MODULE_PATH
)
assert SPEC and SPEC.loader
copilot_e2e = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(copilot_e2e)


class CopilotNativeE2ETests(unittest.TestCase):
    def test_default_agentplugins_version_is_0_1_5(self) -> None:
        self.assertEqual(copilot_e2e.EXPECTED_CLI_VERSION, "0.1.5")

    def test_plugin_list_parser_returns_only_complete_entries(self) -> None:
        output = """Installed plugins:
  • context7@agentplugins-0123456789ab (v0.1.0)
  • cloudflare-docs@agentplugins-fedcba987654 (v0.1.0)
"""
        self.assertEqual(
            copilot_e2e.copilot_plugin_ids(output),
            {
                "context7@agentplugins-0123456789ab",
                "cloudflare-docs@agentplugins-fedcba987654",
            },
        )

    def test_plugin_list_parser_does_not_join_unrelated_substrings(self) -> None:
        output = """Installed plugins:
  • context7 (v0.1.0)
warning: stale @agentplugins-0123456789ab marketplace
"""
        self.assertNotIn(
            "context7@agentplugins-0123456789ab",
            copilot_e2e.copilot_plugin_ids(output),
        )

    def test_agentplugins_commands_do_not_pass_yes(self) -> None:
        completed = SimpleNamespace(stdout='{"schema_version": 1, "command": "add"}')
        with mock.patch.object(
            copilot_e2e, "command", return_value=completed
        ) as run_command:
            copilot_e2e.agentplugins_json(
                Path("/tmp/agentplugins"),
                "add",
                "context7",
                Path("/tmp/sandbox"),
                {},
            )

        argv = run_command.call_args.args[0]
        self.assertEqual(
            argv,
            [
                "/tmp/agentplugins",
                "add",
                "context7",
                "--target",
                "copilot",
                "--format",
                "json",
            ],
        )
        self.assertNotIn("--yes", argv)


if __name__ == "__main__":
    unittest.main()
