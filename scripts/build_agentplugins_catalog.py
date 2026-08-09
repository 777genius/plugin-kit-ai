#!/usr/bin/env python3
"""Build or verify the immutable catalog consumed by agentplugins."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
from pathlib import Path, PurePosixPath

from build_openai_compat import OPENAI_MCP_AUTH
from openai_app_bindings import load_app_bindings
from portable_paths import validate_tree


ROOT = Path(__file__).resolve().parents[1]
PLUGINS = ROOT / "plugins"
OUTPUTS = {
    1: ROOT / "catalog" / "v1" / "catalog.json",
    2: ROOT / "catalog" / "v2" / "catalog.json",
}
SCHEMAS = {
    1: "https://github.com/777genius/universal-agent-plugins/schemas/catalog-v1.schema.json",
    2: "https://github.com/777genius/universal-agent-plugins/schemas/catalog-v2.schema.json",
}
CATALOG_VERSIONS = {1: "0.1.0", 2: "0.2.0"}
PLUGIN_SCHEMA = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
CLIENT_PACKAGE = {
    "codex": "projected",
    "cursor": "native",
    "copilot": "native",
    "vscode": "prepared",
    "kiro": "native",
}
TESTED = {
    "agent-code-navigator": {"codex", "cursor", "kiro"},
    "chrome-devtools": {"codex", "cursor", "kiro"},
    "cloudflare-docs": {"codex", "cursor", "kiro"},
    "context7": {"codex", "cursor", "kiro"},
    "notion": {"codex", "cursor", "kiro"},
}
AUTH_NOT_REQUIRED = {
    "agent-code-navigator",
    "chrome-devtools",
    "cloudflare-docs",
    "context7",
    "docker-hub",
}


def validate_revision(revision: str) -> None:
    if len(revision) != 40 or any(char not in "0123456789abcdef" for char in revision):
        raise ValueError("revision must be a lowercase 40-character commit SHA")


def sha256(value: bytes) -> str:
    return "sha256:" + hashlib.sha256(value).hexdigest()


def package_tree_digest(root: Path) -> str:
    validate_tree(root)
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


def git_blob_at_revision(root: Path, revision: str, relative_path: str) -> bytes:
    validate_revision(revision)
    path = PurePosixPath(relative_path)
    if path.is_absolute() or not path.parts or ".." in path.parts:
        raise ValueError("evidence path must be repository-relative without traversal")
    resolved = subprocess.run(
        ["git", "rev-parse", "--verify", f"{revision}^{{commit}}"],
        cwd=root,
        check=False,
        capture_output=True,
        text=True,
    )
    if resolved.returncode != 0 or resolved.stdout.strip() != revision:
        raise ValueError("runtime evidence revision is not an exact local commit")
    ancestor = subprocess.run(
        ["git", "merge-base", "--is-ancestor", revision, "HEAD"],
        cwd=root,
        check=False,
        capture_output=True,
    )
    if ancestor.returncode != 0:
        raise ValueError("runtime evidence revision must be an ancestor of HEAD")
    blob = subprocess.run(
        ["git", "show", f"{revision}:{path.as_posix()}"],
        cwd=root,
        check=False,
        capture_output=True,
    )
    if blob.returncode != 0:
        raise ValueError("runtime evidence file is missing at the pinned revision")
    return blob.stdout


def validate_pinned_runtime_evidence(
    binding: dict[str, object],
    root: Path = ROOT,
) -> bytes:
    relative_path = str(binding["personal_app_evidence"])
    revision = str(binding["personal_app_evidence_revision"])
    pinned = git_blob_at_revision(root, revision, relative_path)
    try:
        live = (root / relative_path).read_bytes()
    except OSError as error:
        raise ValueError("pinned runtime evidence is unavailable in the live tree") from error
    if live != pinned:
        raise ValueError("live runtime evidence differs from its pinned revision")
    return pinned


def compatibility(
    name: str,
    app_bindings: dict[str, dict[str, object]],
    schema_version: int,
) -> dict[str, object]:
    authentication = "not_required" if name in AUTH_NOT_REQUIRED else "required"
    result = {
        client: {
            "package": package,
            "verification": "tested" if client in TESTED.get(name, set()) else "schema_only",
            "authentication": authentication,
        }
        for client, package in CLIENT_PACKAGE.items()
    }
    binding = app_bindings.get(name) if schema_version == 2 else None
    if binding is not None:
        registration = binding.get("registration")
        if not isinstance(registration, dict) or registration.get("authentication") != "none":
            raise ValueError(f"{name}: ChatGPT compatibility requires explicit auth evidence")
        validate_pinned_runtime_evidence(binding)
        result["chatgpt"] = {
            "package": "projected",
            "verification": "tested",
            "authentication": "not_required",
            "app_binding": {
                "app_key": binding["app_key"],
                "id": binding["id"],
                "mcp_server": binding["mcp_server"],
                "mcp_url": binding["mcp_url"],
                "runtime_evidence": binding["personal_app_evidence"],
                "runtime_evidence_revision": binding[
                    "personal_app_evidence_revision"
                ],
            },
        }
    return result


def auth_hints(name: str, plugin_root: Path) -> dict[str, object]:
    hint = OPENAI_MCP_AUTH.get(name)
    if not hint or not (plugin_root / "mcp.json").is_file():
        return {}
    document = json.loads((plugin_root / "mcp.json").read_text())
    servers = document.get("mcpServers", {})
    return {server: dict(hint) for server in sorted(servers)}


def build(
    revision: str,
    published_at: str,
    schema_version: int = 1,
) -> dict[str, object]:
    validate_revision(revision)
    if schema_version not in OUTPUTS:
        raise ValueError(f"unsupported catalog schema version: {schema_version}")
    app_bindings = load_app_bindings() if schema_version == 2 else {}
    entries = []
    for plugin_root in sorted(path for path in PLUGINS.iterdir() if path.is_dir()):
        manifest_path = plugin_root / "plugin.json"
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        name = str(manifest["name"])
        if name != plugin_root.name:
            raise ValueError(
                f"manifest name {name!r} does not match directory {plugin_root.name!r}"
            )
        entry: dict[str, object] = {
            "name": name,
            "version": manifest["version"],
            "agent_plugins_schema": manifest["$schema"],
            "minimum_cli_version": "0.1.0",
            "source_path": f"plugins/{plugin_root.name}",
            "tree_digest": package_tree_digest(plugin_root),
            "manifest_digest": sha256(manifest_path.read_bytes()),
            "components": components(plugin_root, manifest),
            "compatibility": compatibility(name, app_bindings, schema_version),
        }
        hints = auth_hints(name, plugin_root)
        if hints:
            entry["openai_mcp_auth"] = hints
        entries.append(entry)
    catalog = {
        "$schema": SCHEMAS[schema_version],
        "schema_version": schema_version,
        "catalog_version": CATALOG_VERSIONS[schema_version],
        "repository": "777genius/universal-agent-plugins",
        "revision": revision,
        "published_at": published_at,
        "plugins": entries,
    }
    if schema_version == 2:
        validate_chatgpt_catalog_evidence(catalog)
    return catalog


def validate_chatgpt_catalog_evidence(catalog: dict[str, object]) -> None:
    for plugin in catalog["plugins"]:
        chatgpt = plugin["compatibility"].get("chatgpt")
        if chatgpt is None:
            continue
        binding = chatgpt["app_binding"]
        evidence_revision = binding["runtime_evidence_revision"]
        try:
            evidence_bytes = git_blob_at_revision(
                ROOT, evidence_revision, binding["runtime_evidence"]
            )
            historical_catalog_bytes = git_blob_at_revision(
                ROOT, evidence_revision, "catalog/v1/catalog.json"
            )
            evidence = json.loads(evidence_bytes)
            historical_catalog = json.loads(historical_catalog_bytes)
        except (ValueError, json.JSONDecodeError) as error:
            raise ValueError(
                f"{plugin['name']}: pinned ChatGPT evidence is unavailable or invalid"
            ) from error
        expected_identity = {
            "revision": historical_catalog.get("revision"),
            "digest": sha256(historical_catalog_bytes),
        }
        source_revision = expected_identity["revision"]
        current_revision = catalog["revision"]
        if evidence.get("catalog") != expected_identity:
            raise ValueError(
                f"{plugin['name']}: pinned ChatGPT evidence does not match its historical catalog"
            )
        try:
            validate_revision(str(source_revision))
            validate_revision(str(current_revision))
        except ValueError as error:
            raise ValueError(
                f"{plugin['name']}: invalid ChatGPT evidence catalog revision"
            ) from error
        comparison = subprocess.run(
            ["git", "diff", "--quiet", source_revision, current_revision, "--", "plugins"],
            cwd=ROOT,
            check=False,
            capture_output=True,
        )
        if comparison.returncode != 0:
            raise ValueError(
                f"{plugin['name']}: portable packages differ from the evidence catalog"
            )


def encoded(value: object) -> bytes:
    return (json.dumps(value, indent=2, ensure_ascii=False) + "\n").encode()


def ensure_plugins_match_revision(revision: str) -> None:
    validate_revision(revision)
    resolved = subprocess.run(
        ["git", "rev-parse", "--verify", f"{revision}^{{commit}}"],
        cwd=ROOT,
        check=False,
        capture_output=True,
        text=True,
    )
    if resolved.returncode != 0 or resolved.stdout.strip() != revision:
        detail = (resolved.stderr or "revision is not a commit object").strip()
        raise ValueError(f"catalog revision must resolve to the exact commit: {detail[:500]}")
    ancestor = subprocess.run(
        ["git", "merge-base", "--is-ancestor", revision, "HEAD"],
        cwd=ROOT,
        check=False,
        capture_output=True,
        text=True,
    )
    if ancestor.returncode == 1:
        raise ValueError(
            "catalog revision must be an ancestor of the catalog commit; "
            "merge with history preserved or repin the catalog after merging"
        )
    if ancestor.returncode != 0:
        detail = (ancestor.stderr or "git merge-base failed without stderr").strip()
        raise ValueError(f"could not verify catalog revision ancestry: {detail[:500]}")
    status = subprocess.run(
        [
            "git",
            "status",
            "--porcelain=v1",
            "--untracked-files=all",
            "--ignored",
            "--",
            "plugins",
        ],
        cwd=ROOT,
        check=False,
        capture_output=True,
        text=True,
    )
    if status.returncode != 0:
        detail = (status.stderr or "git status failed without stderr").strip()
        raise ValueError(f"could not inspect the live plugin tree: {detail[:500]}")
    if status.stdout.strip():
        raise ValueError("plugins/ contains tracked changes, untracked paths, or ignored paths")
    result = subprocess.run(
        ["git", "diff", "--quiet", revision, "--", "plugins"],
        cwd=ROOT,
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode == 1:
        raise ValueError("plugins/ differs from the pinned catalog revision")
    if result.returncode != 0:
        detail = (result.stderr or "git diff failed without stderr").strip()
        raise ValueError(f"could not compare plugins/ with pinned revision: {detail[:500]}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--revision")
    parser.add_argument("--published-at")
    parser.add_argument("--schema-version", type=int, choices=sorted(OUTPUTS))
    parser.add_argument("--output", type=Path)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    if args.check:
        if args.output is not None and args.schema_version is None:
            parser.error("--output with --check requires --schema-version")
        versions = [args.schema_version] if args.schema_version else sorted(OUTPUTS)
        for schema_version in versions:
            output = args.output or OUTPUTS[schema_version]
            current = json.loads(output.read_text())
            revision = str(current["revision"])
            published_at = str(current["published_at"])
            ensure_plugins_match_revision(revision)
            body = encoded(build(revision, published_at, schema_version))
            if output.read_bytes() != body:
                raise SystemExit(f"ERROR: catalog/v{schema_version}/catalog.json is out of date")
            print(
                f"OK: catalog v{schema_version} contains "
                f"{len(json.loads(body)['plugins'])} pinned plugins; {sha256(body)}"
            )
        return 0

    schema_version = args.schema_version or 1
    output = args.output or OUTPUTS[schema_version]
    revision = args.revision or subprocess.check_output(
        ["git", "rev-parse", "HEAD"], cwd=ROOT, text=True
    ).strip()
    published_at = args.published_at
    if not published_at:
        parser.error("--published-at is required for reproducible catalog generation")
    ensure_plugins_match_revision(revision)
    body = encoded(build(revision, published_at, schema_version))
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_suffix(output.suffix + ".tmp")
    temporary.write_bytes(body)
    os.replace(temporary, output)
    print(
        f"Generated catalog v{schema_version} with "
        f"{len(json.loads(body)['plugins'])} pinned plugins; {sha256(body)}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
