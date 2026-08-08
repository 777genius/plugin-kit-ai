#!/usr/bin/env python3
"""Build or verify the immutable catalog consumed by agentplugins."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
from pathlib import Path

from build_openai_compat import OPENAI_MCP_AUTH


ROOT = Path(__file__).resolve().parents[1]
PLUGINS = ROOT / "plugins"
OUTPUT = ROOT / "catalog" / "v1" / "catalog.json"
SCHEMA = "https://github.com/777genius/universal-agent-plugins/schemas/catalog-v1.schema.json"
PLUGIN_SCHEMA = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
CLIENT_PACKAGE = {
    "codex": "projected",
    "cursor": "native",
    "copilot": "native",
    "vscode": "prepared",
    "kiro": "native",
}
TESTED = {
    "agent-code-navigator": {"codex"},
    "context7": {"codex", "cursor", "kiro"},
}
AUTH_NOT_REQUIRED = {
    "agent-code-navigator",
    "chrome-devtools",
    "cloudflare-docs",
    "context7",
    "docker-hub",
}


def sha256(value: bytes) -> str:
    return "sha256:" + hashlib.sha256(value).hexdigest()


def package_tree_digest(root: Path) -> str:
    entries: list[tuple[str, Path, bool]] = []
    for path in root.rglob("*"):
        relative = path.relative_to(root).as_posix()
        if ".git" in path.relative_to(root).parts or path.name == ".plugin-kit-ai.lock":
            continue
        if path.is_symlink() or not (path.is_dir() or path.is_file()):
            raise ValueError(f"unsupported package entry: {path}")
        entries.append((relative, path, path.is_dir()))
    entries.sort(key=lambda item: item[0])
    digest = hashlib.sha256()
    for relative, path, is_directory in entries:
        if is_directory:
            digest.update(f"dir\0{relative}\0false\0{0}\0".encode())
            continue
        executable = bool(path.stat().st_mode & 0o111)
        body = path.read_bytes()
        digest.update(f"file\0{relative}\0{str(executable).lower()}\0{len(body)}\0".encode())
        digest.update(body)
    return "sha256:" + digest.hexdigest()


def components(plugin_root: Path, manifest: dict[str, object]) -> list[str]:
    values: list[str] = []
    if (plugin_root / "skills").is_dir():
        values.append("skills")
    if (plugin_root / "mcp.json").is_file():
        values.append("mcp")
    if manifest.get("extensions"):
        values.append("extensions")
    return values


def compatibility(name: str) -> dict[str, object]:
    authentication = "not_required" if name in AUTH_NOT_REQUIRED else "required"
    return {
        client: {
            "package": package,
            "verification": "tested" if client in TESTED.get(name, set()) else "schema_only",
            "authentication": authentication,
        }
        for client, package in CLIENT_PACKAGE.items()
    }


def auth_hints(name: str, plugin_root: Path) -> dict[str, object]:
    hint = OPENAI_MCP_AUTH.get(name)
    if not hint or not (plugin_root / "mcp.json").is_file():
        return {}
    document = json.loads((plugin_root / "mcp.json").read_text())
    servers = document.get("mcpServers", {})
    return {server: dict(hint) for server in sorted(servers)}


def build(revision: str, published_at: str) -> dict[str, object]:
    if len(revision) != 40 or any(char not in "0123456789abcdef" for char in revision):
        raise ValueError("revision must be a lowercase 40-character commit SHA")
    entries = []
    for plugin_root in sorted(path for path in PLUGINS.iterdir() if path.is_dir()):
        manifest_path = plugin_root / "plugin.json"
        manifest = json.loads(manifest_path.read_text())
        name = str(manifest["name"])
        entry: dict[str, object] = {
            "name": name,
            "version": manifest["version"],
            "agent_plugins_schema": manifest["$schema"],
            "minimum_cli_version": "0.1.0-beta.1",
            "source_path": f"plugins/{name}",
            "tree_digest": package_tree_digest(plugin_root),
            "manifest_digest": sha256(manifest_path.read_bytes()),
            "components": components(plugin_root, manifest),
            "compatibility": compatibility(name),
        }
        hints = auth_hints(name, plugin_root)
        if hints:
            entry["openai_mcp_auth"] = hints
        entries.append(entry)
    return {
        "$schema": SCHEMA,
        "schema_version": 1,
        "catalog_version": "0.1.0",
        "repository": "777genius/universal-agent-plugins",
        "revision": revision,
        "published_at": published_at,
        "plugins": entries,
    }


def encoded(value: object) -> bytes:
    return (json.dumps(value, indent=2, ensure_ascii=False) + "\n").encode()


def ensure_plugins_match_revision(revision: str) -> None:
    result = subprocess.run(
        ["git", "diff", "--quiet", revision, "--", "plugins"],
        cwd=ROOT,
        check=False,
    )
    if result.returncode != 0:
        raise ValueError("plugins/ differs from the pinned catalog revision")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--revision")
    parser.add_argument("--published-at")
    parser.add_argument("--output", type=Path, default=OUTPUT)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    if args.check:
        current = json.loads(args.output.read_text())
        revision = str(current["revision"])
        published_at = str(current["published_at"])
    else:
        revision = args.revision or subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
        published_at = args.published_at
        if not published_at:
            parser.error("--published-at is required for reproducible catalog generation")
    ensure_plugins_match_revision(revision)
    body = encoded(build(revision, published_at))
    if args.check:
        if args.output.read_bytes() != body:
            raise SystemExit("ERROR: catalog/v1/catalog.json is out of date")
        print(f"OK: catalog contains {len(json.loads(body)['plugins'])} pinned plugins; {sha256(body)}")
        return 0
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_suffix(args.output.suffix + ".tmp")
    temporary.write_bytes(body)
    os.replace(temporary, args.output)
    print(f"Generated {len(json.loads(body)['plugins'])} pinned plugins; {sha256(body)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
