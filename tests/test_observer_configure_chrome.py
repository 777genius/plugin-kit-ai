from __future__ import annotations

import importlib.util
import json
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "deploy/uap-observer-configure-chrome.py"
SPEC = importlib.util.spec_from_file_location("uap_observer_configure_chrome", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class ObserverChromeConfigurationTests(unittest.TestCase):
    def fixture(self, arguments: list[str] | None = None) -> dict:
        return {
            "mcpServers": {
                "chrome-devtools": {
                    "command": "node",
                    "args": arguments or [
                        "/profile/plugins/chrome/runtime/launcher.mjs",
                        "--no-usage-statistics",
                    ],
                },
                "context7": {"type": "http", "url": "https://example.test/mcp"},
            },
        }

    def test_configure_appends_exact_arguments_and_is_idempotent(self) -> None:
        configured = MODULE.configure(self.fixture(), Path("/profile"))
        value = json.loads(configured)
        args = value["mcpServers"]["chrome-devtools"]["args"]
        self.assertEqual(tuple(args[-len(MODULE.CHROME_RUNTIME_ARGUMENTS):]), MODULE.CHROME_RUNTIME_ARGUMENTS)
        self.assertEqual(value["mcpServers"]["context7"]["url"], "https://example.test/mcp")
        self.assertEqual(MODULE.configure(value, Path("/profile")), configured)

    def test_configure_rejects_conflicting_partial_and_escaping_inputs(self) -> None:
        rejected = (
            ["/profile/launcher.mjs", "--no-usage-statistics", "--browserUrl=http://127.0.0.1:9222"],
            ["/profile/launcher.mjs", "--no-usage-statistics", MODULE.CHROME_RUNTIME_ARGUMENTS[0]],
            ["/outside/launcher.mjs", "--no-usage-statistics"],
        )
        for arguments in rejected:
            with self.subTest(arguments=arguments), self.assertRaises(ValueError):
                MODULE.configure(self.fixture(arguments), Path("/profile"))


if __name__ == "__main__":
    unittest.main()
