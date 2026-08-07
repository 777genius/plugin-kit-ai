from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).resolve().parents[1] / "scripts" / "run_mcp_e2e.py"
SPEC = importlib.util.spec_from_file_location("run_mcp_e2e", MODULE_PATH)
assert SPEC and SPEC.loader
e2e = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(e2e)


class InspectorOutputTests(unittest.TestCase):
    def test_ignores_server_logs(self) -> None:
        payload = e2e.parse_inspector_json(
            'server started\n{"result":{"tools":[{"name":"z"},{"name":"a"}]}}\n'
        )
        self.assertEqual(
            e2e.summarize_result(payload),
            {"status": "passed", "tool_count": 2, "tools": ["a", "z"]},
        )

    def test_classifies_auth_discovery(self) -> None:
        payload = e2e.parse_inspector_json(
            '{"error":{"code":"auth_required","message":"Unauthorized"}}'
        )
        self.assertEqual(
            e2e.summarize_result(payload),
            {"status": "auth_required", "error_code": "auth_required"},
        )


if __name__ == "__main__":
    unittest.main()
