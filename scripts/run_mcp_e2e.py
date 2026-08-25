#!/usr/bin/env python3
"""Run reproducible, credential-free MCP checks in disposable directories."""

from __future__ import annotations

import argparse
import json
import os
import platform
import re
import subprocess
import tempfile
from datetime import UTC, datetime
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
INSPECTOR = "@modelcontextprotocol/inspector@2.1.0"
RESULTS_DIR = ROOT / "tests" / "e2e" / "results"


def credential_free_environment(
    sandbox: Path, plugin_root: Path, plugin_data: Path,
) -> dict[str, str]:
    """Return the minimum host-independent environment needed by npx and Node."""
    directories = {
        "HOME": sandbox / "home",
        "USERPROFILE": sandbox / "home",
        "XDG_CACHE_HOME": sandbox / "xdg-cache",
        "XDG_CONFIG_HOME": sandbox / "xdg-config",
        "XDG_DATA_HOME": sandbox / "xdg-data",
        "APPDATA": sandbox / "app-data",
        "LOCALAPPDATA": sandbox / "local-app-data",
        "TMPDIR": sandbox / "tmp",
        "TMP": sandbox / "tmp",
        "TEMP": sandbox / "tmp",
        "COREPACK_HOME": sandbox / "corepack",
    }
    for directory in set(directories.values()):
        directory.mkdir(mode=0o700)

    environment = {
        key: value
        for key in (
            "PATH", "PATHEXT", "SystemRoot", "WINDIR", "COMSPEC",
            "LANG", "LC_ALL", "LC_CTYPE", "TZ",
        )
        if (value := os.environ.get(key))
    }
    environment.update({key: str(value) for key, value in directories.items()})
    environment.update(
        {
            "CI": "true",
            "NO_COLOR": "1",
            "NODE_REPL_HISTORY": str(sandbox / "node-repl-history"),
            "npm_config_audit": "false",
            "npm_config_cache": str(sandbox / "npm-cache"),
            "npm_config_fund": "false",
            "npm_config_globalconfig": str(sandbox / "global.npmrc"),
            "npm_config_update_notifier": "false",
            "npm_config_userconfig": str(sandbox / "user.npmrc"),
            "PLUGIN_ROOT": str(plugin_root),
            "PLUGIN_DATA": str(plugin_data),
        }
    )
    return environment


def materialize_inspector_config(plugin: str, sandbox: Path) -> tuple[Path, dict[str, str]]:
    """Bind client-provided plugin paths to one disposable Inspector sandbox."""
    plugin_root = (ROOT / "plugins" / plugin).resolve()
    plugin_data = (sandbox / "plugin-data").resolve()
    plugin_data.mkdir(mode=0o700)

    def expand(value: Any) -> Any:
        if isinstance(value, dict):
            return {key: expand(item) for key, item in value.items()}
        if isinstance(value, list):
            return [expand(item) for item in value]
        if isinstance(value, str):
            return value.replace("${PLUGIN_ROOT}", str(plugin_root))
        return value

    source = plugin_root / "mcp.json"
    materialized = expand(json.loads(source.read_text()))
    server = materialized.get("mcpServers", {}).get(plugin)
    if not isinstance(server, dict):
        raise ValueError(f"{plugin} does not define its expected MCP server")
    server_environment = server.setdefault("env", {})
    if not isinstance(server_environment, dict):
        raise ValueError(f"{plugin} MCP server env must be an object")
    server_environment.update(
        {
            "PLUGIN_ROOT": str(plugin_root),
            "PLUGIN_DATA": str(plugin_data),
        }
    )

    destination = sandbox / "mcp.json"
    destination.write_text(json.dumps(materialized) + "\n")
    environment = credential_free_environment(sandbox, plugin_root, plugin_data)
    return destination, environment


def parse_inspector_json(output: str) -> dict[str, Any]:
    """Return the final JSON object emitted by Inspector, ignoring server logs."""
    for line in reversed(output.splitlines()):
        candidate = line.strip()
        if not candidate.startswith("{"):
            continue
        try:
            value = json.loads(candidate)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict):
            return value
    raise ValueError("Inspector did not emit a JSON object")


def summarize_result(
    payload: dict[str, Any],
    *,
    method: str | None = None,
    exit_code: int = 0,
) -> dict[str, Any]:
    """Classify only a valid Inspector result for the requested MCP method."""
    error = payload.get("error")
    if isinstance(error, dict):
        code = str(error.get("code", "error"))
        return {
            "status": "auth_required" if code == "auth_required" else "failed",
            "error_code": code,
        }

    if exit_code != 0:
        return {"status": "failed", "error_code": f"inspector_exit_{exit_code}"}

    result = payload.get("result")
    if not isinstance(result, dict):
        return {"status": "failed", "error_code": "unexpected_result"}

    if method in (None, "tools/list") and "tools" in result:
        tools = result.get("tools")
        if not isinstance(tools, list) or not tools or not all(
            isinstance(tool, dict)
            and isinstance(tool.get("name"), str)
            and bool(tool["name"])
            for tool in tools
        ):
            return {"status": "failed", "error_code": "unexpected_tools_result"}
        names = sorted(str(tool["name"]) for tool in tools)
        return {"status": "passed", "tool_count": len(names), "tools": names}

    if method in (None, "tools/call") and "content" in result:
        content = result.get("content")
        is_error = result.get("isError", False)
        if (
            not isinstance(is_error, bool)
            or is_error
            or not isinstance(content, list)
            or not content
            or not all(
                isinstance(item, dict)
                and isinstance(item.get("type"), str)
                and bool(item["type"])
                for item in content
            )
        ):
            return {"status": "failed", "error_code": "unexpected_content_result"}
        return {"status": "passed", "content_items": len(content)}

    return {"status": "failed", "error_code": "unexpected_result_shape"}


def inspector_check(
    plugin: str,
    *,
    method: str = "tools/list",
    tool_name: str | None = None,
    tool_args: dict[str, Any] | None = None,
    stored_auth_only: bool = False,
    timeout: int = 90,
) -> dict[str, Any]:
    """Run one MCP Inspector check in a disposable client directory."""
    with tempfile.TemporaryDirectory(prefix=f"uap-{plugin}-") as sandbox:
        config, environment = materialize_inspector_config(plugin, Path(sandbox))
        command = [
            "npx",
            "-y",
            INSPECTOR,
            "--cli",
            "--config",
            str(config),
            "--server",
            plugin,
            "--method",
            method,
            "--format",
            "json",
            "--client-config",
            str(Path(sandbox) / "client.json"),
        ]
        if tool_name:
            command.extend(["--tool-name", tool_name])
        if tool_args is not None:
            command.extend(["--tool-args-json", json.dumps(tool_args)])
        if stored_auth_only:
            command.append("--stored-auth-only")

        try:
            completed = subprocess.run(
                command,
                cwd=sandbox,
                capture_output=True,
                text=True,
                timeout=timeout,
                check=False,
                env=environment,
            )
            combined = "\n".join((completed.stdout, completed.stderr))
            payload = parse_inspector_json(combined)
            summary = summarize_result(
                payload,
                method=method,
                exit_code=completed.returncode,
            )
            summary["exit_code"] = completed.returncode
            return summary
        except (OSError, subprocess.TimeoutExpired, ValueError) as error:
            return {"status": "failed", "error_code": type(error).__name__}


def doctor_check() -> dict[str, Any]:
    """Run the packaged code-intelligence diagnostic in a sandbox."""
    script = (
        ROOT
        / "plugins"
        / "agent-code-navigator"
        / "skills"
        / "code-intelligence-doctor"
        / "scripts"
        / "doctor.sh"
    )
    with tempfile.TemporaryDirectory(prefix="uap-agent-code-navigator-") as sandbox:
        completed = subprocess.run(
            ["bash", str(script)],
            cwd=sandbox,
            capture_output=True,
            text=True,
            timeout=60,
            check=False,
        )
    match = re.search(r"warnings=(\d+)", completed.stdout)
    return {
        "status": "passed" if completed.returncode == 0 else "failed",
        "exit_code": completed.returncode,
        "optional_tool_warnings": int(match.group(1)) if match else None,
    }


def run_profile(profile: str) -> list[dict[str, Any]]:
    """Run the credential-free checks selected by an E2E profile."""
    checks: list[tuple[str, dict[str, Any]]] = [
        ("context7:tools/list", inspector_check("context7")),
        (
            "context7:resolve-library-id",
            inspector_check(
                "context7",
                method="tools/call",
                tool_name="resolve-library-id",
                tool_args={
                    "libraryName": "playwright",
                    "query": "Playwright locators quick start",
                },
            ),
        ),
        ("cloudflare-docs:tools/list", inspector_check("cloudflare-docs")),
        (
            "cloudflare-docs:search",
            inspector_check(
                "cloudflare-docs",
                method="tools/call",
                tool_name="search_cloudflare_documentation",
                tool_args={"query": "Workers bindings versus environment variables"},
            ),
        ),
    ]

    if profile == "full":
        checks.extend(
            [
                ("agent-code-navigator:doctor", doctor_check()),
                ("chrome-devtools:tools/list", inspector_check("chrome-devtools")),
            ]
        )
        for plugin in ("cloudflare-radar", "figma", "github", "linear", "notion", "sentry"):
            checks.append(
                (
                    f"{plugin}:auth-discovery",
                    inspector_check(plugin, stored_auth_only=True),
                )
            )

    return [{"check": name, **result} for name, result in checks]


def main() -> int:
    """Run MCP E2E checks and write their structured report."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--profile", choices=("ci", "full"), default="full")
    parser.add_argument("--output", type=Path, default=RESULTS_DIR / "latest.json")
    args = parser.parse_args()

    checks = run_profile(args.profile)
    expected_auth = {
        "cloudflare-radar:auth-discovery",
        "figma:auth-discovery",
        "github:auth-discovery",
        "linear:auth-discovery",
        "notion:auth-discovery",
        "sentry:auth-discovery",
    }
    failures = [
        item
        for item in checks
        if item["status"] != ("auth_required" if item["check"] in expected_auth else "passed")
    ]
    report = {
        "schema_version": 1,
        "generated_at": datetime.now(UTC).isoformat(timespec="seconds"),
        "profile": args.profile,
        "harness": {"name": "MCP Inspector", "version": "2.1.0"},
        "environment": {"platform": platform.system(), "architecture": platform.machine()},
        "checks": checks,
        "summary": {"total": len(checks), "unexpected": len(failures)},
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report["summary"]))
    print(f"evidence={args.output}")
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
