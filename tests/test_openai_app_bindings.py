from __future__ import annotations

import copy
import json
import shutil
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

from openai_app_bindings import (  # noqa: E402
    app_document,
    load_app_bindings,
    validate_binding_target,
)
import build_openai_compat as builder  # noqa: E402
from build_openai_compat import openai_manifest  # noqa: E402
from validate_openai_compat import ValidationError, validate_plugin  # noqa: E402


APP_ID = "plugin_asdk_app_6a78e90cf73481918ef10cdb87cd4bb4"
MCP_URL = "https://docs.mcp.cloudflare.com/mcp"
EVIDENCE_PATH = "tests/e2e/results/chatgpt-cloudflare-docs-direct-2026-08-10.json"


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
                "runtime_evidence": EVIDENCE_PATH,
                "registration": {
                    "surface": "chatgpt_developer_mode",
                    "status": "development",
                    "authentication": "none",
                },
            }
        },
    }


def valid_evidence() -> dict[str, object]:
    return {
        "client": "ChatGPT Developer Mode",
        "version": "rolling web release; build identifier not exposed",
        "date": "2026-08-10",
        "date_timezone": "Europe/Kyiv",
        "evidence_type": "interactive_direct_mcp_runtime",
        "binding": {
            "plugin": "cloudflare-docs",
            "app_id": APP_ID,
            "mcp_url": MCP_URL,
        },
        "source": {
            "plugin": "cloudflare-docs",
            "delivery": "direct registered connection; repository package not installed",
        },
        "checks": [
            {"scenario": "connect", "operation": "connect", "status": "passed"},
            {
                "scenario": "list resources",
                "operation": "list_resources",
                "status": "passed",
            },
            {
                "scenario": "search docs",
                "operation": "search_cloudflare_documentation",
                "status": "passed",
            },
            {
                "scenario": "install package",
                "operation": "package_ui_install",
                "status": "skipped",
            },
        ],
    }


class OpenAIAppBindingTests(unittest.TestCase):
    def write_document(
        self,
        root: Path,
        document: object,
        evidence: object | None = None,
        evidence_path_value: str = EVIDENCE_PATH,
    ) -> Path:
        evidence_path = root / evidence_path_value
        evidence_path.parent.mkdir(parents=True, exist_ok=True)
        evidence_path.write_text(
            json.dumps(valid_evidence() if evidence is None else evidence)
        )
        path = root / "app-bindings.json"
        path.write_text(json.dumps(document))
        return path

    def copy_generated_plugin(self, root: Path) -> Path:
        source = ROOT / "compat" / "openai" / "plugins" / "cloudflare-docs"
        target = root / "cloudflare-docs"
        shutil.copytree(source, target)
        return target

    def test_missing_sidecar_is_optional(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            self.assertEqual(load_app_bindings(Path(tmp) / "missing.json"), {})

    def test_valid_development_binding_loads(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            path = self.write_document(root, valid_document())

            bindings = load_app_bindings(path, root)

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
                    load_app_bindings(path, Path(tmp))

    def test_sidecar_rejects_duplicate_app_id(self) -> None:
        document = valid_document()
        duplicate = copy.deepcopy(document["bindings"]["cloudflare-docs"])
        duplicate["app_key"] = "other"
        duplicate["mcp_server"] = "other"
        document["bindings"]["other"] = duplicate

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            path = self.write_document(root, document)
            with self.assertRaisesRegex(ValueError, "duplicate ChatGPT development app ID"):
                load_app_bindings(path, root)

    def test_sidecar_rejects_runtime_evidence_drift(self) -> None:
        cases: dict[str, tuple[dict[str, object], str]] = {}
        wrong_id = valid_evidence()
        wrong_id["binding"]["app_id"] = "plugin_asdk_app_different"
        cases["wrong app ID"] = (wrong_id, "binding identity does not match sidecar")
        wrong_endpoint = valid_evidence()
        wrong_endpoint["binding"]["mcp_url"] = "https://example.test/mcp"
        cases["wrong endpoint"] = (
            wrong_endpoint,
            "binding identity does not match sidecar",
        )
        failed_runtime = valid_evidence()
        failed_runtime["checks"][2]["status"] = "failed"
        cases["failed direct check"] = (
            failed_runtime,
            "direct runtime checks do not match binding",
        )

        for name, (evidence, message) in cases.items():
            with self.subTest(name=name), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                path = self.write_document(root, valid_document(), evidence)
                with self.assertRaisesRegex(ValueError, message):
                    load_app_bindings(path, root)

    def test_sidecar_rejects_future_dated_runtime_evidence(self) -> None:
        future_path = (
            "tests/e2e/results/chatgpt-cloudflare-docs-direct-2999-01-01.json"
        )
        document = valid_document()
        document["bindings"]["cloudflare-docs"]["runtime_evidence"] = future_path
        evidence = valid_evidence()
        evidence["date"] = "2999-01-01"

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            path = self.write_document(root, document, evidence, future_path)
            with self.assertRaisesRegex(ValueError, "future-dated runtime evidence"):
                load_app_bindings(path, root)

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

    def test_bound_generated_plugin_rejects_wrong_app_id(self) -> None:
        bindings = load_app_bindings()
        with tempfile.TemporaryDirectory() as tmp:
            plugin = self.copy_generated_plugin(Path(tmp))
            app_path = plugin / ".app.json"
            app = json.loads(app_path.read_text())
            app["apps"]["cloudflare-docs"]["id"] = "plugin_asdk_app_different"
            app_path.write_text(json.dumps(app))

            with self.assertRaisesRegex(ValidationError, "app binding drift"):
                validate_plugin(plugin, bindings)

    def test_bound_generated_plugin_rejects_missing_app_file(self) -> None:
        bindings = load_app_bindings()
        with tempfile.TemporaryDirectory() as tmp:
            plugin = self.copy_generated_plugin(Path(tmp))
            (plugin / ".app.json").unlink()

            with self.assertRaisesRegex(ValidationError, "invalid JSON"):
                validate_plugin(plugin, bindings)

    def test_bound_generated_plugin_rejects_missing_manifest_apps(self) -> None:
        bindings = load_app_bindings()
        with tempfile.TemporaryDirectory() as tmp:
            plugin = self.copy_generated_plugin(Path(tmp))
            manifest_path = plugin / ".codex-plugin" / "plugin.json"
            manifest = json.loads(manifest_path.read_text())
            del manifest["apps"]
            manifest_path.write_text(json.dumps(manifest))

            with self.assertRaisesRegex(ValidationError, "invalid apps path"):
                validate_plugin(plugin, bindings)

    def test_bound_generated_plugin_rejects_mcp_endpoint_drift(self) -> None:
        bindings = load_app_bindings()
        with tempfile.TemporaryDirectory() as tmp:
            plugin = self.copy_generated_plugin(Path(tmp))
            mcp_path = plugin / ".mcp.json"
            mcp = json.loads(mcp_path.read_text())
            mcp["mcpServers"]["cloudflare-docs"]["url"] = "https://example.test/mcp"
            mcp_path.write_text(json.dumps(mcp))

            with self.assertRaisesRegex(ValidationError, "app endpoint drift"):
                validate_plugin(plugin, bindings)

    def test_builder_rejects_unknown_plugin_binding(self) -> None:
        binding = valid_document()["bindings"]["cloudflare-docs"]
        with tempfile.TemporaryDirectory() as tmp, patch.object(
            builder,
            "load_app_bindings",
            return_value={"unknown-plugin": binding},
        ):
            root = Path(tmp)
            with self.assertRaisesRegex(ValueError, "unknown plugins"):
                builder.build(root / "plugins", root / "marketplace.json")


if __name__ == "__main__":
    unittest.main()
