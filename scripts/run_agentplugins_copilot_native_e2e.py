#!/usr/bin/env python3
"""Exercise native Agent Plugins lifecycle through a disposable Copilot CLI."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import subprocess
import tempfile
from datetime import datetime, timezone
from pathlib import Path

from run_agentplugins_lifecycle_e2e import (
    CATALOG,
    CLI_TIMEOUT_SECONDS,
    EXPECTED_CLI_VERSION,
    catalog_environment,
)


HERO_PLUGINS = (
    "context7",
    "cloudflare-docs",
    "agent-code-navigator",
    "notion",
    "chrome-devtools",
)
COPILOT_VERSION_PATTERN = re.compile(r"GitHub Copilot CLI ([0-9]+\.[0-9]+\.[0-9]+)\.")
COPILOT_PLUGIN_ENTRY_PATTERN = re.compile(
    r"^\s*•\s+(?P<plugin_id>[^\s]+)\s+\([^\r\n)]+\)\s*$"
)


def isolated_environment(
    sandbox: Path,
    copilot_binary: Path,
    catalog_url: str,
    catalog_digest: str,
) -> dict[str, str]:
    """Build an isolated environment that exposes only the selected CLI binary."""
    allowed = (
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
    home = sandbox / "home"
    temporary = sandbox / "tmp"
    (home / ".copilot").mkdir(parents=True)
    temporary.mkdir(parents=True)
    inherited_path = os.environ.get("PATH", "")
    environment.update(
        {
            "PATH": os.pathsep.join(
                value for value in (str(copilot_binary.parent), inherited_path) if value
            ),
            "HOME": str(home),
            "USERPROFILE": str(home),
            "XDG_CONFIG_HOME": str(sandbox / "config"),
            "XDG_CACHE_HOME": str(sandbox / "cache"),
            "AGENTPLUGINS_HOME": str(sandbox / "agentplugins-state"),
            "COPILOT_HOME": str(sandbox / "copilot-state"),
            "TMPDIR": str(temporary),
            "TMP": str(temporary),
            "TEMP": str(temporary),
            "GIT_CONFIG_GLOBAL": str(home / ".gitconfig"),
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_TERMINAL_PROMPT": "0",
            "CI": "true",
        }
    )
    environment.update(catalog_environment(catalog_url, catalog_digest))
    return environment


def command(
    argv: list[str], sandbox: Path, environment: dict[str, str]
) -> subprocess.CompletedProcess[str]:
    """Run one bounded command and retain output only for assertions."""
    try:
        return subprocess.run(
            argv,
            cwd=sandbox,
            env=environment,
            check=True,
            capture_output=True,
            text=True,
            timeout=CLI_TIMEOUT_SECONDS,
        )
    except subprocess.TimeoutExpired as error:
        raise RuntimeError(
            f"command timed out after {CLI_TIMEOUT_SECONDS}s: {argv[1:4]}"
        ) from error


def agentplugins_json(
    binary: Path,
    action: str,
    plugin: str,
    sandbox: Path,
    environment: dict[str, str],
) -> dict[str, object]:
    """Run one explicit Copilot lifecycle mutation and decode its public JSON."""
    completed = command(
        [
            str(binary),
            action,
            plugin,
            "--target",
            "copilot",
            "--format",
            "json",
        ],
        sandbox,
        environment,
    )
    value = json.loads(completed.stdout)
    if value.get("schema_version") != 1 or value.get("command") != action:
        raise RuntimeError(f"{plugin}: invalid {action} JSON envelope")
    return value


def copilot_plugin_ids(output: str) -> set[str]:
    """Parse complete installed-plugin rows from Copilot's text output."""
    return {
        match.group("plugin_id")
        for line in output.splitlines()
        if (match := COPILOT_PLUGIN_ENTRY_PATTERN.fullmatch(line)) is not None
    }


def run(
    binary: Path,
    expected_version: str,
    copilot_binary: Path,
    expected_copilot_version: str,
    catalog_url: str,
    catalog_digest: str,
) -> dict[str, object]:
    """Run the five hero packages through real Copilot marketplace commands."""
    catalog = json.loads(CATALOG.read_text())
    available = {entry["name"] for entry in catalog["plugins"]}
    missing = sorted(set(HERO_PLUGINS) - available)
    if missing:
        raise RuntimeError(f"hero plugins missing from catalog: {missing}")

    with tempfile.TemporaryDirectory(
        prefix="agentplugins-copilot-native-e2e-"
    ) as temporary:
        sandbox = Path(temporary)
        environment = isolated_environment(
            sandbox, copilot_binary, catalog_url, catalog_digest
        )
        cli_version = command(
            [str(binary), "version"], sandbox, environment
        ).stdout.strip()
        if cli_version != f"agentplugins {expected_version}":
            raise RuntimeError(f"unexpected agentplugins version: {cli_version!r}")
        copilot_version_output = command(
            [str(copilot_binary), "--version"], sandbox, environment
        ).stdout
        match = COPILOT_VERSION_PATTERN.search(copilot_version_output)
        if match is None or match.group(1) != expected_copilot_version:
            raise RuntimeError(
                f"unexpected GitHub Copilot CLI version: {copilot_version_output!r}"
            )

        results: list[dict[str, str]] = []
        for plugin in HERO_PLUGINS:
            added = agentplugins_json(
                binary, "add", plugin, sandbox, environment
            ).get("data", {}).get("result", {})
            activation = added.get("activation", {})
            if (
                added.get("mutated") is not True
                or activation.get("activation") != "active"
                or activation.get("verification") != "installation_verified"
            ):
                raise RuntimeError(f"{plugin}: native activation was not verified")

            physical_artifact_id = added.get("plan", {}).get("physical_artifact_id")
            if not isinstance(physical_artifact_id, str) or not physical_artifact_id:
                raise RuntimeError(f"{plugin}: physical artifact identity was omitted")
            marketplace = "agentplugins-" + hashlib.sha256(
                physical_artifact_id.encode()
            ).hexdigest()[:12]
            expected_plugin_id = f"{plugin}@{marketplace}"

            installed = command(
                [str(copilot_binary), "plugin", "list"], sandbox, environment
            ).stdout
            if expected_plugin_id not in copilot_plugin_ids(installed):
                raise RuntimeError(
                    f"{plugin}: Copilot did not report {expected_plugin_id}"
                )

            removed = agentplugins_json(
                binary, "remove", plugin, sandbox, environment
            ).get("data", {}).get("result", {})
            deactivation = removed.get("deactivation", {})
            if (
                removed.get("mutated") is not True
                or deactivation.get("external_removal_complete") is not True
            ):
                raise RuntimeError(f"{plugin}: native deactivation was not verified")

            remaining_plugins = command(
                [str(copilot_binary), "plugin", "list"], sandbox, environment
            ).stdout
            remaining_marketplaces = command(
                [str(copilot_binary), "plugin", "marketplace", "list"],
                sandbox,
                environment,
            ).stdout
            if (
                expected_plugin_id in copilot_plugin_ids(remaining_plugins)
                or marketplace in remaining_marketplaces
            ):
                raise RuntimeError(f"{plugin}: managed Copilot state remained after remove")
            results.append({"plugin": plugin, "status": "passed"})

        state = json.loads(
            (sandbox / "agentplugins-state" / "state-v2.json").read_text()
        )
        if any(
            client["materialization"] != "absent"
            for installation in state["installations"]
            for client in installation["clients"].values()
        ):
            raise RuntimeError("an Agent Plugins package remained materialized")

    observed = datetime.now(timezone.utc).replace(microsecond=0)
    return {
        "client": "GitHub Copilot CLI native Agent Plugins lifecycle",
        "version": (
            f"agentplugins {expected_version}; GitHub Copilot CLI "
            f"{expected_copilot_version}"
        ),
        "date": observed.date().isoformat(),
        "observed_at_utc": observed.isoformat().replace("+00:00", "Z"),
        "evidence_type": "automated_native_lifecycle",
        "checks": [
            {
                "scenario": "five hero plugins install through managed local marketplaces",
                "status": "passed",
                "plugin_count": len(results),
                "results": results,
            },
            {
                "scenario": "Copilot reports every exact managed plugin ID",
                "status": "passed",
                "plugin_count": len(results),
            },
            {
                "scenario": "agentplugins removes every plugin and managed marketplace",
                "status": "passed",
                "plugin_count": len(results),
            },
            {
                "scenario": "tool runtime and OAuth",
                "status": "skipped",
                "reason": "native lifecycle verification does not invoke plugin tools or authenticate OAuth",
            },
        ],
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
            ],
        },
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--expected-version", default=EXPECTED_CLI_VERSION)
    parser.add_argument("--copilot-binary", type=Path, required=True)
    parser.add_argument("--expected-copilot-version", default="1.0.78")
    parser.add_argument("--catalog-url", required=True)
    parser.add_argument("--catalog-digest", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    result = run(
        args.binary.resolve(),
        args.expected_version,
        # Keep the executable entrypoint name instead of resolving its npm
        # symlink to npm-loader.js. Client detection intentionally looks for
        # `copilot` on PATH, not for the loader implementation behind it.
        args.copilot_binary.absolute(),
        args.expected_copilot_version,
        args.catalog_url,
        args.catalog_digest,
    )
    body = json.dumps(result, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(body)
    else:
        print(body, end="")
    print("OK: 5/5 native Copilot add/remove lifecycle flows", file=__import__("sys").stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
