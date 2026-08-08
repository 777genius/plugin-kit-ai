#!/usr/bin/env python3
"""Exercise five hero packages through five disposable client projections."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import tempfile
from datetime import date, datetime, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "v1" / "catalog.json"
HERO_PLUGINS = (
    "context7",
    "cloudflare-docs",
    "agent-code-navigator",
    "notion",
    "chrome-devtools",
)
CLIENTS = ("codex", "cursor", "copilot", "vscode", "kiro")


def prepare_client(home: Path, target: str) -> None:
    roots = {
        "codex": home / ".codex",
        "cursor": home / ".cursor",
        "copilot": home / ".copilot",
        "vscode": home / "Library" / "Application Support" / "Code" / "User",
        "kiro": home / ".kiro",
    }
    roots[target].mkdir(parents=True)


def run(binary: Path) -> dict[str, object]:
    catalog = json.loads(CATALOG.read_text())
    available = {entry["name"] for entry in catalog["plugins"]}
    missing = sorted(set(HERO_PLUGINS) - available)
    if missing:
        raise RuntimeError(f"hero plugins missing from catalog: {missing}")
    results: list[dict[str, str]] = []
    for target in CLIENTS:
        with tempfile.TemporaryDirectory(prefix=f"agentplugins-{target}-e2e-") as temporary:
            sandbox = Path(temporary)
            home = sandbox / "home"
            prepare_client(home, target)
            environment = dict(os.environ)
            environment.update(
                {
                    "HOME": str(home),
                    "USERPROFILE": str(home),
                    "XDG_CONFIG_HOME": str(sandbox / "config"),
                    "AGENTPLUGINS_HOME": str(sandbox / "state"),
                }
            )
            for plugin in HERO_PLUGINS:
                for command in ("add", "remove"):
                    extra = ["--external-uninstalled"] if command == "remove" and target in {"copilot", "vscode"} else []
                    completed = subprocess.run(
                        [
                            str(binary),
                            command,
                            plugin,
                            "--target",
                            target,
                            "--yes",
                            "--format",
                            "json",
                            *extra,
                        ],
                        cwd=sandbox,
                        env=environment,
                        check=True,
                        capture_output=True,
                        text=True,
                    )
                    value = json.loads(completed.stdout)
                    result = value.get("data", {}).get("result", {})
                    if value.get("schema_version") != 1 or value.get("command") != command:
                        raise RuntimeError(f"{target}/{plugin}: invalid {command} JSON envelope")
                    if result.get("mutated") is not True:
                        raise RuntimeError(f"{target}/{plugin}: {command} did not commit")
                results.append({"plugin": plugin, "client": target, "status": "passed"})
            state = json.loads((sandbox / "state" / "state-v2.json").read_text())
            if any(
                client["materialization"] != "absent"
                for installation in state["installations"]
                for client in installation["clients"].values()
            ):
                raise RuntimeError(f"{target}: a package remained materialized")
    observed = datetime.now(timezone.utc).replace(microsecond=0)
    return {
        "client": "agentplugins CLI isolated multi-client package projections",
        "version": "0.1.0-beta.1-development",
        "date": date.today().isoformat(),
        "observed_at_utc": observed.isoformat().replace("+00:00", "Z"),
        "catalog_revision": catalog["revision"],
        "catalog_digest": "sha256:" + hashlib.sha256(CATALOG.read_bytes()).hexdigest(),
        "checks": [
            {
                "scenario": "five hero packages add/remove across five client projections",
                "status": "passed",
                "package_lifecycle_flows": len(results),
                "results": results,
            },
            {
                "scenario": "client process launch, tool runtime, and OAuth",
                "status": "skipped",
                "reason": "package projection evidence is intentionally separate from runtime evidence",
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
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    result = run(args.binary.resolve())
    body = json.dumps(result, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(body)
    else:
        print(body, end="")
    print("OK: 25/25 hero package/client add-remove flows", file=__import__("sys").stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
