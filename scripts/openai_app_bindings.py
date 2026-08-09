#!/usr/bin/env python3
"""Load and validate host-only ChatGPT app bindings."""

from __future__ import annotations

import json
import re
from pathlib import Path
from urllib.parse import urlsplit


ROOT = Path(__file__).resolve().parents[1]
APP_BINDINGS = ROOT / "compat" / "openai" / "app-bindings.json"
PLUGIN_NAME = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
DEVELOPMENT_APP_ID = re.compile(r"^plugin_asdk_app_[A-Za-z0-9]+$")
ENTRY_FIELDS = {
    "app_key",
    "id",
    "mcp_server",
    "mcp_url",
    "registration",
}


def _unique_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate JSON field: {key}")
        result[key] = value
    return result


def _require_https_url(value: object, field: str) -> str:
    if not isinstance(value, str):
        raise ValueError(f"{field} must be a string")
    parsed = urlsplit(value)
    if (
        parsed.scheme != "https"
        or not parsed.hostname
        or parsed.username is not None
        or parsed.password is not None
        or parsed.query
        or parsed.fragment
    ):
        raise ValueError(f"{field} must be credential-free HTTPS without query or fragment")
    return value


def load_app_bindings(path: Path = APP_BINDINGS) -> dict[str, dict[str, object]]:
    """Return validated bindings, or no bindings when the sidecar is absent."""
    if not path.exists():
        return {}
    try:
        document = json.loads(path.read_text(), object_pairs_hook=_unique_object)
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"{path}: invalid app bindings JSON: {error}") from error
    if not isinstance(document, dict) or set(document) != {
        "$schema",
        "schema_version",
        "bindings",
    }:
        raise ValueError(f"{path}: only $schema, schema_version, and bindings are allowed")
    if document["$schema"] != "../../schemas/openai-app-bindings.schema.json":
        raise ValueError(f"{path}: unexpected $schema")
    if document["schema_version"] != 1:
        raise ValueError(f"{path}: schema_version must be 1")
    bindings = document["bindings"]
    if not isinstance(bindings, dict):
        raise ValueError(f"{path}: bindings must be an object")

    validated: dict[str, dict[str, object]] = {}
    for plugin_name, raw in bindings.items():
        prefix = f"{path}: bindings.{plugin_name}"
        if not isinstance(plugin_name, str) or not PLUGIN_NAME.fullmatch(plugin_name):
            raise ValueError(f"{prefix}: invalid plugin name")
        if not isinstance(raw, dict) or set(raw) != ENTRY_FIELDS:
            raise ValueError(f"{prefix}: unexpected or missing fields")
        if raw["app_key"] != plugin_name or raw["mcp_server"] != plugin_name:
            raise ValueError(f"{prefix}: app_key and mcp_server must match the plugin name")
        app_id = raw["id"]
        if not isinstance(app_id, str) or not DEVELOPMENT_APP_ID.fullmatch(app_id):
            raise ValueError(f"{prefix}.id: invalid ChatGPT development app ID")
        _require_https_url(raw["mcp_url"], f"{prefix}.mcp_url")
        registration = raw["registration"]
        if not isinstance(registration, dict) or registration != {
            "surface": "chatgpt_developer_mode",
            "status": "development",
            "authentication": "none",
        }:
            raise ValueError(f"{prefix}.registration: unsupported registration metadata")
        validated[plugin_name] = raw
    return validated


def validate_binding_target(
    plugin_name: str,
    binding: dict[str, object],
    portable_mcp: dict[str, object],
) -> None:
    """Bind only one exact public Streamable HTTP MCP endpoint."""
    servers = portable_mcp.get("mcpServers")
    server_name = str(binding["mcp_server"])
    if not isinstance(servers, dict) or set(servers) != {server_name}:
        raise ValueError(f"{plugin_name}: app binding requires one matching MCP server")
    server = servers[server_name]
    if not isinstance(server, dict) or server.get("type") != "streamable-http":
        raise ValueError(f"{plugin_name}: ChatGPT app binding requires Streamable HTTP")
    if server.get("url") != binding["mcp_url"]:
        raise ValueError(f"{plugin_name}: app binding endpoint does not match portable MCP")


def app_document(binding: dict[str, object]) -> dict[str, object]:
    """Return the official `.app.json` compatibility shape."""
    return {
        "apps": {
            str(binding["app_key"]): {
                "id": binding["id"],
            }
        }
    }
