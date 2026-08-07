#!/usr/bin/env python3
"""Validate the generated OpenAI host-package compatibility layer."""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path
from urllib.parse import urlsplit


ROOT = Path(__file__).resolve().parents[1]
PLUGINS_ROOT = ROOT / "compat" / "openai" / "plugins"
MARKETPLACE = ROOT / ".agents" / "plugins" / "marketplace.json"
MANIFEST_FIELDS = {
    "name", "version", "description", "author", "homepage", "repository",
    "license", "keywords", "skills", "mcpServers", "interface",
}
INTERFACE_FIELDS = {
    "displayName", "shortDescription", "longDescription", "developerName",
    "category", "capabilities", "websiteURL", "defaultPrompt", "brandColor",
    "composerIcon", "logo", "logoDark", "screenshots",
}
SERVER_FIELDS = {
    "type", "command", "args", "env", "cwd", "url", "headers",
    "bearer_token_env_var", "oauth_resource",
}
EXPECTED_AUTH = {
    "github": {"bearer_token_env_var": "GITHUB_PAT_TOKEN"},
    "figma": {"oauth_resource": "https://mcp.figma.com/mcp"},
    "linear": {"oauth_resource": "https://mcp.linear.app/mcp"},
    "notion": {"oauth_resource": "https://mcp.notion.com"},
}
EXPECTED_CAPABILITIES = {
    "figma": ["Interactive", "Read", "Write"],
    "sentry": ["Interactive", "Write"],
}
ENV_NAME = re.compile(r"^[A-Z][A-Z0-9_]*$")


class ValidationError(Exception):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValidationError(message)


def load(path: Path) -> dict[str, object]:
    try:
        value = json.loads(path.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        raise ValidationError(f"{path}: invalid JSON: {exc}") from exc
    require(isinstance(value, dict), f"{path}: top level must be an object")
    return value


def require_https(value: object, field: str) -> None:
    require(isinstance(value, str), f"{field}: must be a string")
    parsed = urlsplit(value)
    require(parsed.scheme == "https" and bool(parsed.hostname), f"{field}: must be an HTTPS URL")


def validate_mcp(plugin_root: Path, plugin_name: str) -> None:
    path = plugin_root / ".mcp.json"
    doc = load(path)
    require(set(doc) == {"mcpServers"}, f"{path}: only mcpServers is allowed")
    servers = doc["mcpServers"]
    require(isinstance(servers, dict) and servers, f"{path}: mcpServers must be a non-empty object")
    for server_name, server in servers.items():
        require(isinstance(server_name, str) and server_name, f"{path}: invalid server name")
        require(isinstance(server, dict), f"{path}: {server_name} must be an object")
        require(not (set(server) - SERVER_FIELDS), f"{path}: unknown fields for {server_name}")
        transport = server.get("type", "stdio" if "command" in server else None)
        require(transport in {"stdio", "http", "sse"}, f"{path}: invalid transport for {server_name}")
        if transport == "stdio":
            require(isinstance(server.get("command"), str) and server["command"], f"{path}: command required")
        else:
            require_https(server.get("url"), f"{path}: {server_name}.url")
        bearer = server.get("bearer_token_env_var")
        if bearer is not None:
            require(isinstance(bearer, str) and bool(ENV_NAME.fullmatch(bearer)), f"{path}: invalid bearer env var")
        oauth_resource = server.get("oauth_resource")
        if oauth_resource is not None:
            require_https(oauth_resource, f"{path}: {server_name}.oauth_resource")
        expected = EXPECTED_AUTH.get(plugin_name, {})
        actual = {key: server[key] for key in ("bearer_token_env_var", "oauth_resource") if key in server}
        require(actual == expected, f"{path}: auth metadata drift for {plugin_name}: {actual!r}")


def validate_plugin(plugin_root: Path) -> str:
    path = plugin_root / ".codex-plugin" / "plugin.json"
    manifest = load(path)
    require(not (set(manifest) - MANIFEST_FIELDS), f"{path}: unknown manifest fields")
    name = manifest.get("name")
    require(name == plugin_root.name, f"{path}: name must match directory")
    for field in ("version", "description", "license"):
        require(isinstance(manifest.get(field), str) and manifest[field], f"{path}: {field} required")
    interface = manifest.get("interface")
    require(isinstance(interface, dict), f"{path}: interface required")
    require(not (set(interface) - INTERFACE_FIELDS), f"{path}: unknown interface fields")
    capabilities = interface.get("capabilities")
    require(
        isinstance(capabilities, list)
        and capabilities
        and set(capabilities) <= {"Interactive", "Read", "Write"},
        f"{path}: invalid capabilities",
    )
    expected_capabilities = EXPECTED_CAPABILITIES.get(str(name))
    if expected_capabilities is not None:
        require(
            capabilities == expected_capabilities,
            f"{path}: capability metadata drift for {name}",
        )
    for field in ("composerIcon", "logo", "logoDark"):
        asset = interface.get(field)
        require(isinstance(asset, str) and asset.startswith("./assets/"), f"{path}: invalid {field}")
        require((plugin_root / asset[2:]).is_file(), f"{path}: missing {field} asset")
    if "skills" in manifest:
        require(manifest["skills"] == "./skills/" and (plugin_root / "skills").is_dir(), f"{path}: invalid skills path")
    if "mcpServers" in manifest:
        require(manifest["mcpServers"] == "./.mcp.json", f"{path}: invalid mcpServers path")
        validate_mcp(plugin_root, str(name))
    return str(name)


def validate() -> int:
    require(PLUGINS_ROOT.is_dir(), f"{PLUGINS_ROOT}: missing generated plugins")
    names = [validate_plugin(path) for path in sorted(PLUGINS_ROOT.iterdir()) if path.is_dir()]
    marketplace = load(MARKETPLACE)
    entries = marketplace.get("plugins")
    require(isinstance(entries, list), f"{MARKETPLACE}: plugins must be an array")
    marketplace_names = [entry.get("name") for entry in entries if isinstance(entry, dict)]
    require(marketplace_names == names, f"{MARKETPLACE}: entries do not match generated plugins")
    print(f"OK: {len(names)} OpenAI compatibility packages")
    return len(names)


def main() -> int:
    try:
        validate()
    except ValidationError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
