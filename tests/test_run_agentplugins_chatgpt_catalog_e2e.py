from __future__ import annotations

import hashlib
import importlib.util
import json
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "scripts" / "run_agentplugins_chatgpt_catalog_e2e.py"
SPEC = importlib.util.spec_from_file_location("chatgpt_catalog_e2e", MODULE_PATH)
assert SPEC and SPEC.loader
e2e = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = e2e
SPEC.loader.exec_module(e2e)


class ChatGPTCatalogE2ETests(unittest.TestCase):
    def test_defaults_bind_released_cli_and_catalog_v2(self) -> None:
        digest = "sha256:" + hashlib.sha256(e2e.CATALOG.read_bytes()).hexdigest()
        catalog, binding = e2e.verified_catalog(digest)

        self.assertEqual(e2e.EXPECTED_CLI_VERSION, "0.1.6")
        self.assertEqual(e2e.CATALOG, ROOT / "catalog" / "v2" / "catalog.json")
        self.assertEqual(catalog["schema_version"], 2)
        self.assertEqual(binding["app_key"], "cloudflare-docs")
        self.assertEqual(binding["mcp_url"], "https://docs.mcp.cloudflare.com/mcp")

    def test_catalog_verification_rejects_drift_and_wrong_schema(self) -> None:
        with self.assertRaisesRegex(ValueError, "does not match"):
            e2e.verified_catalog("sha256:" + "0" * 64)

        source = json.loads(e2e.CATALOG.read_text())
        source["schema_version"] = 1
        body = (json.dumps(source) + "\n").encode()
        with tempfile.TemporaryDirectory() as temporary:
            catalog_path = Path(temporary) / "catalog.json"
            catalog_path.write_bytes(body)
            digest = "sha256:" + hashlib.sha256(body).hexdigest()
            with mock.patch.object(e2e, "CATALOG", catalog_path):
                with self.assertRaisesRegex(ValueError, "requires catalog schema v2"):
                    e2e.verified_catalog(digest)

    def test_catalog_url_rejects_credentials_query_fragment_and_http(self) -> None:
        digest = "sha256:" + "1" * 64
        invalid = (
            "http://example.test/catalog.json",
            "https://user@example.test/catalog.json",
            "https://example.test/catalog.json?token=secret",
            "https://example.test/catalog.json#fragment",
        )
        for url in invalid:
            with self.subTest(url=url), self.assertRaisesRegex(ValueError, "public HTTPS"):
                e2e.catalog_environment(url, digest)

        self.assertEqual(
            e2e.catalog_environment("https://example.test/catalog.json", digest),
            {
                "AGENTPLUGINS_CATALOG_URL": "https://example.test/catalog.json",
                "AGENTPLUGINS_CATALOG_DIGEST": digest,
            },
        )

    def test_run_rejects_a_different_cli_version_before_execution(self) -> None:
        digest = "sha256:" + hashlib.sha256(e2e.CATALOG.read_bytes()).hexdigest()
        with self.assertRaisesRegex(ValueError, "requires released CLI 0.1.6"):
            e2e.run(
                Path("/does/not/run"),
                "0.1.7",
                "https://example.test/catalog.json",
                digest,
            )

    def test_isolated_environment_does_not_inherit_credentials(self) -> None:
        digest = "sha256:" + "2" * 64
        with tempfile.TemporaryDirectory() as temporary, mock.patch.dict(
            os.environ,
            {"PATH": "/usr/bin", "GITHUB_TOKEN": "secret", "NPM_TOKEN": "secret"},
            clear=True,
        ):
            sandbox = Path(temporary)
            environment = e2e.isolated_environment(
                sandbox, "https://example.test/catalog.json", digest
            )

        self.assertNotIn("GITHUB_TOKEN", environment)
        self.assertNotIn("NPM_TOKEN", environment)
        self.assertEqual(environment["AGENTPLUGINS_HOME"], str(sandbox / "state"))
        self.assertEqual(environment["HOME"], str(sandbox / "home"))
        self.assertEqual(environment["GIT_CONFIG_NOSYSTEM"], "1")

    def test_failure_output_redacts_paths_credentials_and_oauth_values(self) -> None:
        sandbox = Path("/tmp/agentplugins-private-sandbox")
        environment = {
            "HOME": str(sandbox / "home"),
            "AGENTPLUGINS_HOME": str(sandbox / "state"),
        }
        value = (
            f"failed at {sandbox}/state/private; "
            "https://user:password@example.test/mcp; "
            "Authorization: Bearer token-value; "
            "https://example.test/callback?code=oauth-code&state=oauth-state"
        )

        sanitized = e2e.sanitized_output(value, sandbox, environment)

        self.assertNotIn(str(sandbox), sanitized)
        self.assertNotIn("user:password", sanitized)
        self.assertNotIn("token-value", sanitized)
        self.assertNotIn("oauth-code", sanitized)
        self.assertNotIn("oauth-state", sanitized)

    def _projection(self, root: Path, binding: dict[str, object]) -> None:
        (root / ".codex-plugin").mkdir(parents=True)
        (root / ".codex-plugin" / "plugin.json").write_text(
            json.dumps(
                {
                    "name": "cloudflare-docs",
                    "apps": "./.app.json",
                    "mcpServers": "./.mcp.json",
                }
            )
        )
        (root / ".app.json").write_text(
            json.dumps({"apps": {binding["app_key"]: {"id": binding["id"]}}})
        )
        (root / ".mcp.json").write_text(
            json.dumps(
                {
                    "mcpServers": {
                        binding["mcp_server"]: {
                            "url": binding["mcp_url"],
                            "type": "http",
                        }
                    }
                }
            )
        )

    def test_projection_validator_accepts_exact_official_package(self) -> None:
        digest = "sha256:" + hashlib.sha256(e2e.CATALOG.read_bytes()).hexdigest()
        _, binding = e2e.verified_catalog(digest)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "plugin"
            self._projection(root, binding)

            e2e.validate_projection(root, binding)

    def test_projection_validator_rejects_portable_manifest_and_hooks(self) -> None:
        digest = "sha256:" + hashlib.sha256(e2e.CATALOG.read_bytes()).hexdigest()
        _, binding = e2e.verified_catalog(digest)
        for rogue in ("plugin.json", "mcp.json", "hooks/hooks.json"):
            with self.subTest(rogue=rogue), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary) / "plugin"
                self._projection(root, binding)
                path = root / rogue
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("{}")

                with self.assertRaisesRegex(RuntimeError, "portable|hooks"):
                    e2e.validate_projection(root, binding)

    def test_projection_validator_rejects_binding_drift_and_symlinks(self) -> None:
        digest = "sha256:" + hashlib.sha256(e2e.CATALOG.read_bytes()).hexdigest()
        _, binding = e2e.verified_catalog(digest)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "plugin"
            self._projection(root, binding)
            (root / ".app.json").write_text(
                json.dumps({"apps": {binding["app_key"]: {"id": "wrong-app"}}})
            )
            with self.assertRaisesRegex(RuntimeError, "does not match catalog"):
                e2e.validate_projection(root, binding)

        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "plugin"
            self._projection(root, binding)
            outside = Path(temporary) / "outside.txt"
            outside.write_text("outside")
            (root / "linked-secret").symlink_to(outside)
            with self.assertRaisesRegex(RuntimeError, "contains a symlink"):
                e2e.validate_projection(root, binding)


if __name__ == "__main__":
    unittest.main()
