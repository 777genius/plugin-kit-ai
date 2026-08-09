from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

from openai_app_bindings import (  # noqa: E402
    app_document,
    load_app_bindings,
    validate_binding_target,
)
from build_openai_compat import openai_manifest  # noqa: E402
from validate_openai_compat import ValidationError, validate_plugin  # noqa: E402


APP_ID = "plugin_asdk_app_6a78e90cf73481918ef10cdb87cd4bb4"
MCP_URL = "https://docs.mcp.cloudflare.com/mcp"


def valid_document() -> dict[str, object]:
    return {
        "$schema": "../../schemas/openai-app-bindings.schema.json",
        "schema_version": 1,
        "bindings": {
            "cloudflare-docs": {
                "app_key": "cloudflare-docs",
                "id": APP_ID,
                "mcp_server": "cloudflare-docs",
                "mcp_url": MCP_URL,
                "registration": {
                    "surface": "chatgpt_developer_mode",
                    "status": "development",
                    "authentication": "none",
                },
            }
        },
    }


class OpenAIAppBindingTests(unittest.TestCase):
    def write_document(self, root: Path, document: object) -> Path:
        path = root / "app-bindings.json"
        path.write_text(json.dumps(document))
        return path

    def test_missing_sidecar_is_optional(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            self.assertEqual(load_app_bindings(Path(tmp) / "missing.json"), {})

    def test_valid_development_binding_loads(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = self.write_document(Path(tmp), valid_document())

            bindings = load_app_bindings(path)

        self.assertEqual(bindings["cloudflare-docs"]["id"], APP_ID)
        self.assertEqual(
            app_document(bindings["cloudflare-docs"]),
            {"apps": {"cloudflare-docs": {"id": APP_ID}}},
        )

    def test_sidecar_rejects_unknown_or_unsafe_values(self) -> None:
        cases: dict[str, tuple[object, str]] = {}
        unknown = valid_document()
        unknown["unexpected"] = True
        cases["unknown top-level field"] = (unknown, "only \\$schema")
        invalid_id = valid_document()
        invalid_id["bindings"]["cloudflare-docs"]["id"] = "connector_vendor_id"
        cases["non-development app ID"] = (invalid_id, "invalid ChatGPT development app ID")
        mismatched_name = valid_document()
        mismatched_name["bindings"]["cloudflare-docs"]["app_key"] = "other"
        cases["mismatched app key"] = (mismatched_name, "must match the plugin name")
        auth = valid_document()
        auth["bindings"]["cloudflare-docs"]["registration"]["authentication"] = "oauth"
        cases["unapproved auth"] = (auth, "unsupported registration metadata")
        query = valid_document()
        query["bindings"]["cloudflare-docs"]["mcp_url"] = MCP_URL + "?token=x"
        cases["endpoint query"] = (query, "without query or fragment")

        for name, (document, message) in cases.items():
            with self.subTest(name=name), tempfile.TemporaryDirectory() as tmp:
                path = self.write_document(Path(tmp), document)
                with self.assertRaisesRegex(ValueError, message):
                    load_app_bindings(path)

    def test_binding_requires_exact_single_streamable_http_server(self) -> None:
        binding = valid_document()["bindings"]["cloudflare-docs"]
        valid_mcp = {
            "mcpServers": {
                "cloudflare-docs": {
                    "type": "streamable-http",
                    "url": MCP_URL,
                }
            }
        }
        validate_binding_target("cloudflare-docs", binding, valid_mcp)

        for name, document in {
            "endpoint mismatch": {
                "mcpServers": {
                    "cloudflare-docs": {
                        "type": "streamable-http",
                        "url": "https://example.test/mcp",
                    }
                }
            },
            "stdio": {
                "mcpServers": {
                    "cloudflare-docs": {"type": "stdio", "command": "demo"}
                }
            },
            "extra server": {
                "mcpServers": {
                    **valid_mcp["mcpServers"],
                    "other": {"type": "streamable-http", "url": MCP_URL},
                }
            },
        }.items():
            with self.subTest(name=name), self.assertRaises(ValueError):
                validate_binding_target("cloudflare-docs", binding, document)

    def test_openai_manifest_declares_apps_only_for_bound_package(self) -> None:
        portable = {
            "name": "cloudflare-docs",
            "version": "0.1.0",
            "description": "Cloudflare documentation",
            "author": {"name": "777genius"},
            "homepage": "https://example.test",
            "repository": "https://example.test/repository",
            "license": "Apache-2.0",
            "keywords": ["docs"],
        }

        unbound = openai_manifest(portable, False, True, False)
        bound = openai_manifest(portable, False, True, True)

        self.assertNotIn("apps", unbound)
        self.assertEqual(bound["apps"], "./.app.json")

    def test_unbound_generated_plugin_rejects_rogue_app_file(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            plugin = Path(tmp) / "demo"
            (plugin / ".codex-plugin").mkdir(parents=True)
            (plugin / "assets").mkdir()
            for name in ("icon.png", "logo.png"):
                (plugin / "assets" / name).write_bytes(b"asset")
            manifest = {
                "name": "demo",
                "version": "0.1.0",
                "description": "Demo plugin",
                "license": "Apache-2.0",
                "interface": {
                    "capabilities": ["Read"],
                    "composerIcon": "./assets/icon.png",
                    "logo": "./assets/logo.png",
                    "logoDark": "./assets/logo.png",
                },
            }
            (plugin / ".codex-plugin" / "plugin.json").write_text(
                json.dumps(manifest)
            )
            (plugin / ".app.json").write_text(
                json.dumps({"apps": {"demo": {"id": APP_ID}}})
            )

            with self.assertRaisesRegex(ValidationError, "rogue .app.json"):
                validate_plugin(plugin, {})


if __name__ == "__main__":
    unittest.main()
