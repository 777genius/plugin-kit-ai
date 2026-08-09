from __future__ import annotations

import importlib.util
import hashlib
import json
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

    def test_e2e_catalog_digest_is_bound_to_exact_local_file(self) -> None:
        for module in (e2e, hero_e2e):
            actual = "sha256:" + hashlib.sha256(module.CATALOG.read_bytes()).hexdigest()
            with self.subTest(module=module.__name__, case="match"):
                catalog, digest = module.verified_catalog(actual)
                self.assertEqual(digest, actual)
                self.assertEqual(catalog["revision"], json.loads(module.CATALOG.read_text())["revision"])
            with self.subTest(module=module.__name__, case="mismatch"), self.assertRaisesRegex(
                ValueError, "does not match the local catalog"
            ):
                module.verified_catalog("sha256:" + "0" * 64)

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

    def test_lifecycle_commands_do_not_pass_yes(self) -> None:
        completed = subprocess.CompletedProcess([], 0, stdout="{}", stderr="")
        with mock.patch.object(
            e2e.subprocess, "run", return_value=completed
        ) as run_command:
            e2e.run_cli(
                Path("/tmp/agentplugins"),
                "add",
                "context7",
                Path("/tmp/sandbox"),
                {},
                check=True,
            )

        argv = run_command.call_args.args[0]
        self.assertEqual(
            argv,
            [
                "/tmp/agentplugins",
                "add",
                "context7",
                "--target",
                "cursor",
                "--format",
                "json",
            ],
        )
        self.assertNotIn("--yes", argv)

    def test_projection_commands_do_not_pass_yes(self) -> None:
        completed = subprocess.CompletedProcess([], 0, stdout="{}", stderr="")
        with mock.patch.object(
            hero_e2e.subprocess, "run", return_value=completed
        ) as run_command:
            hero_e2e.run_cli(
                Path("/tmp/agentplugins"),
                "remove",
                "context7",
                "kiro",
                Path("/tmp/sandbox"),
                {},
            )

        argv = run_command.call_args.args[0]
        self.assertEqual(
            argv,
            [
                "/tmp/agentplugins",
                "remove",
                "context7",
                "--target",
                "kiro",
                "--format",
                "json",
                "--external-uninstalled",
            ],
        )
        self.assertNotIn("--yes", argv)

    def test_vscode_detection_root_maps_each_platform(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            sandbox = Path(tmp)
            home = sandbox / "home"
            environment = hero_e2e.isolated_environment(
                sandbox, home, CATALOG_URL, CATALOG_DIGEST
            )
            custom_xdg = sandbox / "custom-xdg"
            environment["XDG_CONFIG_HOME"] = str(custom_xdg)

            self.assertEqual(
                hero_e2e.vscode_detection_root(home, environment, "linux"),
                custom_xdg / "Code" / "User",
            )
            self.assertEqual(
                hero_e2e.vscode_detection_root(home, environment, "darwin"),
                home / "Library" / "Application Support" / "Code" / "User",
            )
            self.assertEqual(
                hero_e2e.vscode_detection_root(home, environment, "win32"),
                sandbox / "appdata" / "Code" / "User",
            )

    def test_vscode_prepares_only_current_platform_real_directory(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            sandbox = Path(tmp)
            home = sandbox / "home"
            environment = hero_e2e.isolated_environment(
                sandbox, home, CATALOG_URL, CATALOG_DIGEST
            )

            roots = hero_e2e.prepare_client(
                home, "vscode", environment, platform_name="linux"
            )

            self.assertEqual(
                roots, (sandbox / "config" / "Code" / "User",)
            )
            self.assertTrue(roots[0].is_dir())
            self.assertFalse(roots[0].is_symlink())
            self.assertFalse(
                (home / "Library" / "Application Support" / "Code").exists()
            )
            self.assertFalse((sandbox / "appdata" / "Code").exists())

    def test_projection_environment_does_not_inherit_windows_appdata(self) -> None:
        with tempfile.TemporaryDirectory() as tmp, mock.patch.dict(
            os.environ,
            {"APPDATA": "C:\\Users\\real\\AppData", "LOCALAPPDATA": "secret"},
            clear=True,
        ):
            sandbox = Path(tmp)
            environment = hero_e2e.isolated_environment(
                sandbox, sandbox / "home", CATALOG_URL, CATALOG_DIGEST
            )

        self.assertEqual(environment["APPDATA"], str(sandbox / "appdata"))
        self.assertEqual(
            environment["LOCALAPPDATA"], str(sandbox / "local-appdata")
        )

    def test_projection_failure_names_flow_and_sanitizes_stderr(self) -> None:
        sandbox = Path("/tmp/private-sandbox")
        environment = {
            "HOME": str(sandbox / "home"),
            "XDG_CONFIG_HOME": str(sandbox / "config"),
        }
        stderr = (
            f"cannot write {sandbox}/config/Code/User; "
            '"api_key":"json-secret"; token=super-secret; '
            "Authorization: Bearer bearer-secret; "
            "https://user:password@example.test/catalog"
        )
        failure = subprocess.CalledProcessError(
            7, ["agentplugins"], stderr=stderr
        )
        with mock.patch.object(
            hero_e2e.subprocess, "run", side_effect=failure
        ), self.assertRaisesRegex(
            RuntimeError, r"vscode/context7: add failed with exit 7"
        ) as raised:
            hero_e2e.run_cli(
                Path("/tmp/agentplugins"),
                "add",
                "context7",
                "vscode",
                sandbox,
                environment,
            )

        message = str(raised.exception)
        self.assertIn("stderr: cannot write <path>", message)
        self.assertNotIn(str(sandbox), message)
        self.assertNotIn("super-secret", message)
        self.assertNotIn("json-secret", message)
        self.assertNotIn("bearer-secret", message)
        self.assertNotIn("user:password", message)

    def test_sanitizer_redacts_complete_authorization_values(self) -> None:
        cases = {
            "Basic Zm9vOmJhcg== trailing-value": ("Zm9vOmJhcg==", "trailing-value"),
            "token opaque-token trailing-value": ("opaque-token", "trailing-value"),
            "Bearer bearer-token trailing-value": ("bearer-token", "trailing-value"),
            'Digest username="private", response="digest-secret"': (
                "private",
                "digest-secret",
            ),
            "CustomScheme custom-secret extra-parameter": (
                "custom-secret",
                "extra-parameter",
            ),
        }
        for authorization, secrets in cases.items():
            with self.subTest(authorization=authorization):
                sanitized = hero_e2e.sanitized_failure_output(
                    f"request failed\nAuthorization: {authorization}\nretry disabled",
                    Path("/tmp/sandbox"),
                    {},
                )
                self.assertEqual(
                    sanitized,
                    "request failed\nAuthorization: <redacted>\nretry disabled",
                )
                for secret in secrets:
                    self.assertNotIn(secret, sanitized)

    def test_sanitizer_redacts_oauth_query_code_and_state(self) -> None:
        sanitized = hero_e2e.sanitized_failure_output(
            "callback rejected: https://example.test/cb?code=private-code&state=private-state&next=public",
            Path("/tmp/sandbox"),
            {},
        )

        self.assertEqual(
            sanitized,
            "callback rejected: https://example.test/cb?code=<redacted>&state=<redacted>&next=public",
        )
        self.assertNotIn("private-code", sanitized)
        self.assertNotIn("private-state", sanitized)

    def test_projection_timeout_names_target_plugin_and_command(self) -> None:
        with mock.patch.object(
            hero_e2e.subprocess,
            "run",
            side_effect=subprocess.TimeoutExpired(["agentplugins"], 120),
        ), self.assertRaisesRegex(
            RuntimeError, r"kiro/notion: remove timed out after 120s"
        ):
            hero_e2e.run_cli(
                Path("/tmp/agentplugins"),
                "remove",
                "notion",
                "kiro",
                Path("/tmp/sandbox"),
                {},
            )

    def test_e2e_defaults_target_agentplugins_0_1_5(self) -> None:
        self.assertEqual(e2e.EXPECTED_CLI_VERSION, "0.1.5")
        self.assertEqual(hero_e2e.EXPECTED_CLI_VERSION, "0.1.5")

    def test_e2e_flow_dimensions_cover_catalog_and_hero_matrix(self) -> None:
        catalog = json.loads(e2e.CATALOG.read_text())

        self.assertEqual(len(catalog["plugins"]), 26)
        self.assertEqual(len(hero_e2e.HERO_PLUGINS), 5)
        self.assertEqual(len(hero_e2e.CLIENTS), 5)
        self.assertEqual(len(hero_e2e.HERO_PLUGINS) * len(hero_e2e.CLIENTS), 25)


if __name__ == "__main__":
    unittest.main()
