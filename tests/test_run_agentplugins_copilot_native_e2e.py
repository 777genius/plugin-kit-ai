from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


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


if __name__ == "__main__":
    unittest.main()
