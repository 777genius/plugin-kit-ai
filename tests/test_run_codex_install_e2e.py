from __future__ import annotations

import importlib.util
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


MODULE_PATH = (
    Path(__file__).resolve().parents[1] / "scripts" / "run_codex_install_e2e.py"
)
SPEC = importlib.util.spec_from_file_location("run_codex_install_e2e", MODULE_PATH)
assert SPEC and SPEC.loader
e2e = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(e2e)


class CodexInstallE2ETests(unittest.TestCase):
    def test_parses_pretty_json_after_logs(self) -> None:
        output = 'starting\n{\n  "pluginId": "context7@catalog"\n}\ndone\n'
        self.assertEqual(
            e2e.parse_json_output(output),
            {"pluginId": "context7@catalog"},
        )

    def test_uses_last_json_object(self) -> None:
        output = '{"first": true}\nlog\n{"second": true}\n'
        self.assertEqual(e2e.parse_json_output(output), {"second": True})

    def test_rejects_missing_json(self) -> None:
        with self.assertRaises(e2e.E2EError):
            e2e.parse_json_output("server log only")

    def test_confines_reported_install_path(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            installed = root / "plugins" / "context7"
            installed.mkdir(parents=True)
            self.assertEqual(
                e2e.confined_path(str(installed), root, "plugin"),
                installed.resolve(),
            )
            with self.assertRaises(e2e.E2EError):
                e2e.confined_path(str(root.parent), root, "plugin")

    def test_rejects_mcp_symlink_outside_installed_package(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            sandbox = Path(tmp)
            installed = sandbox / "installed"
            installed.mkdir()
            normal = installed / ".mcp.json"
            normal.write_text("{}")
            self.assertEqual(
                e2e.confined_file(normal, installed, "mcp"),
                normal.resolve(),
            )
            normal.unlink()
            outside = sandbox / "outside.json"
            outside.write_text("{}")
            normal.symlink_to(outside)
            with self.assertRaises(e2e.E2EError):
                e2e.confined_file(normal, installed, "mcp")

    def test_extracts_only_expected_public_marker(self) -> None:
        payload = {
            "result": {
                "content": [
                    {
                        "type": "text",
                        "text": "Library ID: /microsoft/playwright\nprivate detail omitted",
                    }
                ]
            }
        }
        self.assertEqual(
            e2e.extract_context7_marker(payload),
            "/microsoft/playwright",
        )

    def test_rejects_tool_errors_and_wrong_marker(self) -> None:
        cases = (
            {"error": {"code": "failed"}},
            {"result": {"isError": True, "content": []}},
            {"result": {"content": [{"type": "text", "text": "not found"}]}},
        )
        for payload in cases:
            with self.subTest(payload=payload), self.assertRaises(e2e.E2EError):
                e2e.extract_context7_marker(payload)

    def test_builds_github_workflow_url(self) -> None:
        environment = {
            "GITHUB_REPOSITORY": "777genius/universal-agent-plugins",
            "GITHUB_RUN_ID": "12345",
            "GITHUB_SHA": "a" * 40,
            "GITHUB_SERVER_URL": "https://github.com",
            "GITHUB_EVENT_NAME": "workflow_dispatch",
        }
        with patch.dict(os.environ, environment, clear=True):
            self.assertEqual(
                e2e.workflow_metadata(True),
                {
                    "url": "https://github.com/777genius/universal-agent-plugins/actions/runs/12345",
                    "commit_sha": "a" * 40,
                    "event": "workflow_dispatch",
                },
            )

    def test_requires_complete_ci_provenance(self) -> None:
        with patch.dict(os.environ, {}, clear=True), self.assertRaises(e2e.E2EError):
            e2e.workflow_metadata(True)

    def test_isolated_environment_does_not_inherit_credentials(self) -> None:
        inherited = {
            "PATH": "/usr/bin:/bin",
            "LANG": "C.UTF-8",
            "AWS_SECRET_ACCESS_KEY": "secret",
            "GH_TOKEN": "secret",
            "GITHUB_TOKEN": "secret",
            "NPM_TOKEN": "secret",
            "NODE_AUTH_TOKEN": "secret",
            "GIT_ASKPASS": "/secret/helper",
            "SSH_AUTH_SOCK": "/secret/socket",
        }
        with tempfile.TemporaryDirectory() as tmp, patch.dict(
            os.environ, inherited, clear=True
        ):
            environment = e2e.isolated_environment(Path(tmp))
        self.assertEqual(environment["PATH"], inherited["PATH"])
        self.assertEqual(environment["LANG"], inherited["LANG"])
        for name in inherited.keys() - {"PATH", "LANG"}:
            self.assertNotIn(name, environment)
        self.assertEqual(environment["GIT_TERMINAL_PROMPT"], "0")
        self.assertEqual(environment["CI"], "true")

    def test_tag_event_is_bound_to_source_ref_and_commit(self) -> None:
        environment = {
            "GITHUB_REF_TYPE": "tag",
            "GITHUB_REF_NAME": "v0.1.1",
            "GITHUB_SHA": "a" * 40,
        }
        with patch.dict(os.environ, environment, clear=True):
            e2e.validate_tag_provenance("v0.1.1", "a" * 40)
            with self.assertRaises(e2e.E2EError):
                e2e.validate_tag_provenance("v0.1.0", "a" * 40)
            with self.assertRaises(e2e.E2EError):
                e2e.validate_tag_provenance("v0.1.1", "b" * 40)

    def test_non_tag_event_skips_tag_binding(self) -> None:
        with patch.dict(
            os.environ,
            {"GITHUB_REF_TYPE": "branch", "GITHUB_SHA": "a" * 40},
            clear=True,
        ):
            e2e.validate_tag_provenance("v0.1.1", "b" * 40)


if __name__ == "__main__":
    unittest.main()
