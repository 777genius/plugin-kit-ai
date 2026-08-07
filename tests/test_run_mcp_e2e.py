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

    def test_rejects_unknown_result_shapes(self) -> None:
        cases = (
            ({}, {"status": "failed", "error_code": "unexpected_result"}),
            (
                {"result": {}},
                {"status": "failed", "error_code": "unexpected_result_shape"},
            ),
            (
                {"result": {"unexpected": "shape"}},
                {"status": "failed", "error_code": "unexpected_result_shape"},
            ),
        )
        for payload, expected in cases:
            with self.subTest(payload=payload):
                self.assertEqual(e2e.summarize_result(payload), expected)

    def test_rejects_nonzero_exit_for_apparent_success(self) -> None:
        payload = {"result": {"tools": [{"name": "search"}]}}
        self.assertEqual(
            e2e.summarize_result(payload, method="tools/list", exit_code=1),
            {"status": "failed", "error_code": "inspector_exit_1"},
        )

    def test_requires_nonempty_method_specific_result(self) -> None:
        self.assertEqual(
            e2e.summarize_result({"result": {"tools": []}}, method="tools/list"),
            {"status": "failed", "error_code": "unexpected_tools_result"},
        )
        self.assertEqual(
            e2e.summarize_result({"result": {"content": []}}, method="tools/call"),
            {"status": "failed", "error_code": "unexpected_content_result"},
        )
        self.assertEqual(
            e2e.summarize_result(
                {
                    "result": {
                        "isError": True,
                        "content": [{"type": "text", "text": "tool failed"}],
                    }
                },
                method="tools/call",
            ),
            {"status": "failed", "error_code": "unexpected_content_result"},
        )
        self.assertEqual(
            e2e.summarize_result(
                {"result": {"content": [{"text": "missing type"}]}},
                method="tools/call",
            ),
            {"status": "failed", "error_code": "unexpected_content_result"},
        )


if __name__ == "__main__":
    unittest.main()
