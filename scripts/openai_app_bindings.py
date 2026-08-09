#!/usr/bin/env python3
"""Load and validate host-only ChatGPT app bindings."""

from __future__ import annotations

import json
import re
from datetime import date, datetime
from pathlib import Path
from urllib.parse import urlsplit
from zoneinfo import ZoneInfo


ROOT = Path(__file__).resolve().parents[1]
APP_BINDINGS = ROOT / "compat" / "openai" / "app-bindings.json"
PLUGIN_NAME = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
DEVELOPMENT_APP_ID = re.compile(r"^plugin_asdk_app_[A-Za-z0-9]+$")
RUNTIME_EVIDENCE_PATH = re.compile(
    r"^tests/e2e/results/[a-z0-9-]+-(?P<date>[0-9]{4}-[0-9]{2}-[0-9]{2})\.json$"
)
EVIDENCE_TIMEZONE = ZoneInfo("Europe/Kyiv")
ENTRY_FIELDS = {
    "app_key",
    "id",
    "mcp_server",
    "mcp_url",
    "runtime_evidence",
    "registration",
}
DIRECT_RUNTIME_CHECKS = {
    "connect": "passed",
    "list_resources": "passed",
    "search_cloudflare_documentation": "passed",
    "package_ui_install": "skipped",
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


def _load_object(path: Path, description: str) -> dict[str, object]:
    try:
        value = json.loads(path.read_text(), object_pairs_hook=_unique_object)
    except (OSError, json.JSONDecodeError, ValueError) as error:
        raise ValueError(f"{path}: invalid {description} JSON: {error}") from error
    if not isinstance(value, dict):
        raise ValueError(f"{path}: {description} must contain an object")
    return value


def _runtime_evidence_path(value: object, root: Path, field: str) -> Path:
    if not isinstance(value, str) or not RUNTIME_EVIDENCE_PATH.fullmatch(value):
        raise ValueError(f"{field} must be a repository-relative client evidence path")
    relative = Path(value)
    resolved_root = root.resolve()
    resolved = (resolved_root / relative).resolve()
    if not resolved.is_relative_to(resolved_root) or not resolved.is_file():
        raise ValueError(f"{field} must reference an existing in-repository file")
    return resolved


def _validate_runtime_evidence(
    plugin_name: str,
    binding: dict[str, object],
    evidence_path: Path,
) -> None:
    evidence = _load_object(evidence_path, "runtime evidence")
    evidence_date = evidence.get("date")
    try:
        observed_date = date.fromisoformat(str(evidence_date))
    except ValueError as error:
        raise ValueError(f"{evidence_path}: invalid runtime evidence date") from error
    path_match = RUNTIME_EVIDENCE_PATH.fullmatch(str(binding["runtime_evidence"]))
    if path_match is None or evidence_date != path_match.group("date"):
        raise ValueError(f"{evidence_path}: runtime evidence date does not match filename")
    if evidence.get("date_timezone") != EVIDENCE_TIMEZONE.key:
        raise ValueError(f"{evidence_path}: runtime evidence timezone must be Europe/Kyiv")
    if observed_date > datetime.now(EVIDENCE_TIMEZONE).date():
        raise ValueError(f"{evidence_path}: future-dated runtime evidence is forbidden")
    expected_binding = {
        "plugin": plugin_name,
        "app_id": binding["id"],
        "mcp_url": binding["mcp_url"],
    }
    if evidence.get("binding") != expected_binding:
        raise ValueError(f"{evidence_path}: binding identity does not match sidecar")
    if evidence.get("client") != "ChatGPT Developer Mode" or evidence.get(
        "evidence_type"
    ) != "interactive_direct_mcp_runtime":
        raise ValueError(f"{evidence_path}: expected direct ChatGPT Developer Mode evidence")
    source = evidence.get("source")
    if not isinstance(source, dict) or source.get("plugin") != plugin_name:
        raise ValueError(f"{evidence_path}: evidence plugin does not match sidecar")
    if source.get("delivery") != (
        "direct registered connection; repository package not installed"
    ):
        raise ValueError(f"{evidence_path}: evidence must keep package installation pending")
    checks = evidence.get("checks")
    if not isinstance(checks, list):
        raise ValueError(f"{evidence_path}: direct runtime checks are required")
    actual_checks: dict[str, object] = {}
    for check in checks:
        if not isinstance(check, dict) or not isinstance(check.get("operation"), str):
            raise ValueError(f"{evidence_path}: every direct check needs an operation")
        operation = check["operation"]
        if operation in actual_checks:
            raise ValueError(f"{evidence_path}: duplicate direct check: {operation}")
        actual_checks[operation] = check.get("status")
    if actual_checks != DIRECT_RUNTIME_CHECKS:
        raise ValueError(f"{evidence_path}: direct runtime checks do not match binding")


def load_app_bindings(
    path: Path = APP_BINDINGS,
    root: Path = ROOT,
) -> dict[str, dict[str, object]]:
    """Return validated bindings, or no bindings when the sidecar is absent."""
    if not path.exists():
        return {}
    document = _load_object(path, "app bindings")
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
    seen_app_ids: set[str] = set()
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
        if app_id in seen_app_ids:
            raise ValueError(f"{prefix}.id: duplicate ChatGPT development app ID")
        seen_app_ids.add(app_id)
        _require_https_url(raw["mcp_url"], f"{prefix}.mcp_url")
        registration = raw["registration"]
        if not isinstance(registration, dict) or registration != {
            "surface": "chatgpt_developer_mode",
            "status": "development",
            "authentication": "none",
        }:
            raise ValueError(f"{prefix}.registration: unsupported registration metadata")
        evidence_path = _runtime_evidence_path(
            raw["runtime_evidence"], root, f"{prefix}.runtime_evidence"
        )
        _validate_runtime_evidence(plugin_name, raw, evidence_path)
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
