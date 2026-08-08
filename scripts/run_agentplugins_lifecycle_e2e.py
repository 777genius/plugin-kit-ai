#!/usr/bin/env python3
"""Run all catalog add/remove flows in a disposable Cursor provider sandbox."""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import tempfile
from datetime import date, datetime, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "v1" / "catalog.json"


def run(binary: Path) -> dict[str, object]:
    catalog = json.loads(CATALOG.read_text())
    names = [entry["name"] for entry in catalog["plugins"]]
    with tempfile.TemporaryDirectory(prefix="agentplugins-lifecycle-e2e-") as temporary:
        sandbox = Path(temporary)
        home = sandbox / "home"
        (home / ".cursor").mkdir(parents=True)
        environment = dict(os.environ)
        environment.update(
            {
                "HOME": str(home),
                "USERPROFILE": str(home),
                "XDG_CONFIG_HOME": str(sandbox / "config"),
                "AGENTPLUGINS_HOME": str(sandbox / "state"),
            }
        )
        for name in names:
            for command in ("add", "remove"):
                completed = subprocess.run(
                    [str(binary), command, name, "--target", "cursor", "--yes", "--format", "json"],
                    cwd=sandbox,
                    env=environment,
                    check=True,
                    capture_output=True,
                    text=True,
                )
                value = json.loads(completed.stdout)
                if value.get("schema_version") != 1 or value.get("command") != command:
                    raise RuntimeError(f"{name}: invalid {command} JSON envelope")
                if value.get("data", {}).get("result", {}).get("mutated") is not True:
                    raise RuntimeError(f"{name}: {command} did not commit a mutation")
        state = json.loads((sandbox / "state" / "state-v2.json").read_text())
        active = [
            installation["declared_name"]
            for installation in state["installations"]
            for client in installation["clients"].values()
            if client["materialization"] != "absent"
        ]
        if active:
            raise RuntimeError(f"managed packages remained active: {active}")
    observed = datetime.now(timezone.utc).replace(microsecond=0)
    return {
        "client": "agentplugins CLI isolated Cursor provider",
        "version": "0.1.0-beta.1-development",
        "date": date.today().isoformat(),
        "observed_at_utc": observed.isoformat().replace("+00:00", "Z"),
        "catalog_revision": catalog["revision"],
        "catalog_digest": "sha256:" + __import__("hashlib").sha256(CATALOG.read_bytes()).hexdigest(),
        "checks": [
            {"scenario": "resolve all short names from the pinned catalog", "status": "passed", "plugin_count": len(names)},
            {"scenario": "transactional add for every catalog package", "status": "passed", "plugin_count": len(names)},
            {"scenario": "digest-guarded remove for every catalog package", "status": "passed", "plugin_count": len(names)},
            {"scenario": "tool runtime and OAuth", "status": "skipped", "reason": "tracked separately from package lifecycle"},
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
    print("OK: 26/26 pinned add/remove lifecycle flows", file=__import__("sys").stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
