#!/usr/bin/env python3
"""Install Context7 from the public Codex marketplace and call its MCP tool."""

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


MARKETPLACE_SOURCE = "777genius/universal-agent-plugins"
MARKETPLACE_NAME = "universal-agent-plugins"
PLUGIN_ID = f"context7@{MARKETPLACE_NAME}"
INSPECTOR = "@modelcontextprotocol/inspector@2.1.0"
EXPECTED_MARKER = "/microsoft/playwright"
SAFE_REF = re.compile(r"[A-Za-z0-9][A-Za-z0-9._/-]{0,127}\Z")
COMMIT_SHA = re.compile(r"[0-9a-f]{40}\Z")


class E2EError(RuntimeError):
    """Raised when a reproducible install or tool-call invariant fails."""


def parse_json_output(output: str) -> dict[str, Any]:
    """Return the last compact or pretty-printed JSON object in command output."""
    decoder = json.JSONDecoder()
    values: list[dict[str, Any]] = []
    for match in re.finditer(r"(?m)^\s*\{", output):
        candidate = output[match.start() :].lstrip()
        try:
            value, _ = decoder.raw_decode(candidate)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict):
            values.append(value)
    if values:
        return values[-1]
    raise E2EError("command did not emit a JSON object")


def run_command(
    command: list[str],
    *,
    cwd: Path,
    env: dict[str, str],
    timeout: int = 180,
) -> subprocess.CompletedProcess[str]:
    """Run a command and fail without echoing potentially sensitive output."""
    try:
        completed = subprocess.run(
            command,
            cwd=cwd,
            env=env,
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired) as error:
        raise E2EError(f"{command[0]} failed: {type(error).__name__}") from error
    if completed.returncode != 0:
        raise E2EError(f"{command[0]} exited with code {completed.returncode}")
    return completed


def require_text(data: dict[str, Any], field: str, context: str) -> str:
    """Return a non-empty string field from structured CLI output."""
    value = data.get(field)
    if not isinstance(value, str) or not value:
        raise E2EError(f"{context} omitted {field}")
    return value


def confined_path(raw_path: str, root: Path, context: str) -> Path:
    """Require a CLI-reported path to stay inside the disposable CODEX_HOME."""
    path = Path(raw_path).resolve()
    if not path.is_relative_to(root.resolve()):
        raise E2EError(f"{context} escaped the disposable CODEX_HOME")
    return path


def confined_file(path: Path, root: Path, context: str) -> Path:
    """Require an existing file, including its symlink target, inside root."""
    resolved = path.resolve()
    if not resolved.is_relative_to(root.resolve()) or not resolved.is_file():
        raise E2EError(f"{context} is missing or escaped its installed package")
    return resolved


def extract_context7_marker(payload: dict[str, Any]) -> str:
    """Verify a successful Context7 call and return only its public marker."""
    if "error" in payload:
        raise E2EError("Context7 returned an MCP error")
    result = payload.get("result")
    if not isinstance(result, dict) or result.get("isError", False) is not False:
        raise E2EError("Context7 returned an invalid result")
    content = result.get("content")
    if not isinstance(content, list) or not content:
        raise E2EError("Context7 returned no content")
    text = "\n".join(
        item.get("text", "")
        for item in content
        if isinstance(item, dict)
        and item.get("type") == "text"
        and isinstance(item.get("text"), str)
    )
    if EXPECTED_MARKER not in text:
        raise E2EError("Context7 result did not contain the expected library ID")
    return EXPECTED_MARKER


def workflow_metadata(require_ci_metadata: bool) -> dict[str, str | None]:
    """Build verifiable GitHub Actions provenance from runner metadata."""
    repository = os.environ.get("GITHUB_REPOSITORY")
    run_id = os.environ.get("GITHUB_RUN_ID")
    commit = os.environ.get("GITHUB_SHA")
    server = os.environ.get("GITHUB_SERVER_URL", "https://github.com")
    if require_ci_metadata and not (
        repository and run_id and commit and COMMIT_SHA.fullmatch(commit)
    ):
        raise E2EError("required GitHub Actions provenance is unavailable")
    url = f"{server}/{repository}/actions/runs/{run_id}" if repository and run_id else None
    return {
        "url": url,
        "commit_sha": commit,
        "event": os.environ.get("GITHUB_EVENT_NAME"),
    }


def validate_tag_provenance(marketplace_ref: str, source_commit: str) -> None:
    """Bind tag-triggered evidence to the exact GitHub tag and workflow commit."""
    if os.environ.get("GITHUB_REF_TYPE") != "tag":
        return
    if os.environ.get("GITHUB_REF_NAME") != marketplace_ref:
        raise E2EError("tag event marketplace ref does not match GITHUB_REF_NAME")
    if os.environ.get("GITHUB_SHA") != source_commit:
        raise E2EError("tag event marketplace commit does not match GITHUB_SHA")


def isolated_environment(sandbox: Path) -> dict[str, str]:
    """Return an environment that cannot reuse the user's Codex or npm state."""
    allowed = (
        "PATH",
        "LANG",
        "LC_ALL",
        "LC_CTYPE",
        "TZ",
        "SYSTEMROOT",
        "WINDIR",
        "COMSPEC",
        "PATHEXT",
        "SSL_CERT_FILE",
        "SSL_CERT_DIR",
        "NODE_EXTRA_CA_CERTS",
    )
    environment = {name: os.environ[name] for name in allowed if name in os.environ}
    temp_dir = sandbox / "tmp"
    temp_dir.mkdir(parents=True, exist_ok=True)
    environment.update(
        {
            "CODEX_HOME": str(sandbox / "codex-home"),
            "HOME": str(sandbox / "home"),
            "NPM_CONFIG_CACHE": str(sandbox / "npm-cache"),
            "NPM_CONFIG_USERCONFIG": str(sandbox / "home" / ".npmrc"),
            "XDG_CACHE_HOME": str(sandbox / "xdg-cache"),
            "XDG_CONFIG_HOME": str(sandbox / "xdg-config"),
            "TMPDIR": str(temp_dir),
            "TMP": str(temp_dir),
            "TEMP": str(temp_dir),
            "GIT_CONFIG_GLOBAL": str(sandbox / "home" / ".gitconfig"),
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_TERMINAL_PROMPT": "0",
            "CI": "true",
            "NO_COLOR": "1",
        }
    )
    return environment


def run_e2e(marketplace_ref: str, require_ci_metadata: bool) -> dict[str, Any]:
    """Run the public marketplace install and a real installed Context7 call."""
    if not SAFE_REF.fullmatch(marketplace_ref):
        raise E2EError("marketplace ref contains unsupported characters")

    with tempfile.TemporaryDirectory(prefix="uap-codex-e2e-") as raw_sandbox:
        sandbox = Path(raw_sandbox)
        codex_home = sandbox / "codex-home"
        project = sandbox / "project"
        for directory in (codex_home, project, sandbox / "home"):
            directory.mkdir(parents=True)
        environment = isolated_environment(sandbox)

        run_command(
            ["git", "init", "--initial-branch=main", "--quiet"],
            cwd=project,
            env=environment,
        )
        version_output = run_command(
            ["codex", "--version"], cwd=project, env=environment
        ).stdout.strip()
        version_match = re.fullmatch(r"codex-cli ([0-9]+(?:\.[0-9]+){2})", version_output)
        if not version_match:
            raise E2EError("Codex CLI returned an unexpected version string")
        codex_version = version_match.group(1)

        marketplace_output = run_command(
            [
                "codex",
                "plugin",
                "marketplace",
                "add",
                MARKETPLACE_SOURCE,
                "--ref",
                marketplace_ref,
                "--json",
            ],
            cwd=project,
            env=environment,
        )
        marketplace = parse_json_output(
            "\n".join((marketplace_output.stdout, marketplace_output.stderr))
        )
        if marketplace.get("marketplaceName") != MARKETPLACE_NAME:
            raise E2EError("Codex added an unexpected marketplace")
        marketplace_root = confined_path(
            require_text(marketplace, "installedRoot", "marketplace result"),
            codex_home,
            "marketplace checkout",
        )
        source_commit = run_command(
            ["git", "rev-parse", "HEAD"],
            cwd=marketplace_root,
            env=environment,
        ).stdout.strip()
        if not COMMIT_SHA.fullmatch(source_commit):
            raise E2EError("marketplace checkout did not resolve to a commit SHA")
        validate_tag_provenance(marketplace_ref, source_commit)

        install_output = run_command(
            ["codex", "plugin", "add", PLUGIN_ID, "--json"],
            cwd=project,
            env=environment,
        )
        install = parse_json_output(
            "\n".join((install_output.stdout, install_output.stderr))
        )
        if install.get("pluginId") != PLUGIN_ID:
            raise E2EError("Codex installed an unexpected plugin")
        installed_path = confined_path(
            require_text(install, "installedPath", "plugin result"),
            codex_home,
            "installed plugin",
        )
        plugin_version = require_text(install, "version", "plugin result")
        mcp_config = confined_file(
            installed_path / ".mcp.json",
            installed_path,
            "installed Context7 .mcp.json",
        )

        inspector_output = run_command(
            [
                "npx",
                "-y",
                INSPECTOR,
                "--cli",
                "--config",
                str(mcp_config),
                "--server",
                "context7",
                "--method",
                "tools/call",
                "--tool-name",
                "resolve-library-id",
                "--tool-args-json",
                json.dumps(
                    {
                        "libraryName": "playwright",
                        "query": "Playwright locators quick start",
                    }
                ),
                "--format",
                "json",
                "--client-config",
                str(sandbox / "inspector-client.json"),
            ],
            cwd=project,
            env=environment,
        )
        inspector_payload = parse_json_output(
            "\n".join((inspector_output.stdout, inspector_output.stderr))
        )
        marker = extract_context7_marker(inspector_payload)

    now = datetime.now(UTC)
    workflow = workflow_metadata(require_ci_metadata)
    return {
        "client": "Codex CLI",
        "version": codex_version,
        "date": now.date().isoformat(),
        "observed_at_utc": now.isoformat(timespec="seconds"),
        "evidence_type": "automated_public_install",
        "sandbox": "fresh disposable git repository and isolated CODEX_HOME",
        "source": {
            "repository": f"https://github.com/{MARKETPLACE_SOURCE}",
            "ref": marketplace_ref,
            "commit_sha": source_commit,
        },
        "workflow": workflow,
        "reproduction": {
            "requirements": [f"Codex CLI {codex_version}", "Node.js 24"],
            "commands": [
                f"codex plugin marketplace add {MARKETPLACE_SOURCE} --ref {marketplace_ref} --json",
                f"codex plugin add {PLUGIN_ID} --json",
                f"python3 scripts/run_codex_install_e2e.py --marketplace-ref {marketplace_ref}",
            ],
        },
        "checks": [
            {
                "scenario": "add the public GitHub marketplace at an explicit ref",
                "status": "passed",
                "ref": marketplace_ref,
                "commit_sha": source_commit,
            },
            {
                "scenario": "install Context7 from the Codex marketplace cache",
                "status": "passed",
                "plugin_id": PLUGIN_ID,
                "plugin_version": plugin_version,
            },
            {
                "scenario": "call resolve-library-id from the installed package",
                "status": "passed",
                "tool": "resolve-library-id",
                "result_marker": marker,
            },
        ],
        "transcript": [
            f"marketplace.add passed ref={marketplace_ref} commit={source_commit}",
            f"plugin.add passed id={PLUGIN_ID} version={plugin_version}",
            f"tools.call passed name=resolve-library-id marker={marker}",
        ],
        "environment": {
            "platform": platform.system(),
            "architecture": platform.machine(),
        },
        "secrets_recorded": False,
        "real_user_project_used": False,
        "privacy": {
            "sanitized": True,
            "excluded": [
                "credentials",
                "tokens",
                "cookies",
                "OAuth codes",
                "OAuth state",
                "account identifiers",
                "authorization URLs",
                "absolute temporary paths",
                "raw tool output",
            ],
        },
    }


def main() -> int:
    """Parse arguments, run E2E, and write sanitized structured evidence."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--marketplace-ref", default="v0.1.5")
    parser.add_argument("--output", type=Path, default=Path("/tmp/codex-install-e2e.json"))
    parser.add_argument("--require-ci-metadata", action="store_true")
    args = parser.parse_args()
    try:
        report = run_e2e(args.marketplace_ref, args.require_ci_metadata)
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    except (E2EError, OSError) as error:
        print(f"Codex install E2E failed: {error}")
        return 1
    print("Codex install E2E passed: public marketplace + Context7 tool call")
    print(f"evidence={args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
