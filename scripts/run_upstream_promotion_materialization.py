#!/usr/bin/env python3
"""Prove one exact upstream package lifecycle in disposable client homes."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


class MaterializationError(Exception):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise MaterializationError(message)


def invoke(cli: Path, command: str, arguments: list[str], *, env: dict[str, str], cwd: Path) -> dict[str, Any]:
    completed = subprocess.run(
        [str(cli), command, *arguments, "--format", "json"], env=env, cwd=cwd,
        text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=300, check=False,
    )
    try:
        value = json.loads(completed.stdout)
    except json.JSONDecodeError as error:
        raise MaterializationError(f"{command} did not emit JSON: {completed.stderr[:500]}") from error
    require(isinstance(value, dict), f"{command} emitted a non-object")
    require(completed.returncode == 0 and value.get("result") == "success", f"{command} failed: {completed.stderr[:500]}")
    require(value.get("command") == command, f"{command} response identity differs")
    return value


def batch(value: dict[str, Any], command: str, clients: list[str]) -> dict[str, Any]:
    data = value.get("data")
    require(isinstance(data, dict), f"{command} response data is absent")
    require(data.get("status") == "completed", f"{command} did not complete")
    require(type(data.get("succeeded")) is int and data["succeeded"] == len(clients), f"{command} succeeded count differs")
    require(type(data.get("failed")) is int and data["failed"] == 0, f"{command} failed count differs")
    targets = data.get("targets")
    require(isinstance(targets, list) and len(targets) == len(clients), f"{command} target count differs")
    require({item.get("target") for item in targets if isinstance(item, dict)} == set(clients), f"{command} target identities differ")
    require(all(item.get("status") == "external_completed" for item in targets), f"{command} did not externally complete every target")
    return data


def checked_identity(data: dict[str, Any], args: argparse.Namespace) -> dict[str, Any]:
    require(data.get("plugin") == args.product_id, "installed manifest name differs from the watched product")
    if getattr(args, "local_path", None) is None:
        require(data.get("revision") == args.revision, "installed revision differs from the official merge commit")
    for field in ("tree_digest", "manifest_digest"):
        value = data.get(field)
        require(isinstance(value, str) and value.startswith("sha256:") and len(value) == 71, f"installed {field} is invalid")
    return {
        "package_version": data.get("version", ""),
        "tree_digest": data["tree_digest"], "manifest_digest": data["manifest_digest"],
    }


def run(args: argparse.Namespace) -> dict[str, Any]:
    clients = args.targets.split(",")
    require(clients and len(clients) == len(set(clients)) and set(clients) <= {"codex", "cursor", "kiro"}, "targets are invalid")
    root = args.sandbox.resolve()
    require(not root.exists(), "sandbox already exists")
    roots = {name: root / name for name in ("home", "state", "workspace", "config", "cache", "tmp")}
    for path in roots.values():
        path.mkdir(parents=True)
    for name in (".codex", ".cursor", ".kiro"):
        (roots["home"] / name).mkdir()
    environment = {
        "PATH": os.environ.get("PATH", ""), "LANG": "C.UTF-8", "LC_ALL": "C.UTF-8",
        "HOME": str(roots["home"]), "USERPROFILE": str(roots["home"]),
        "XDG_CONFIG_HOME": str(roots["config"]), "XDG_CACHE_HOME": str(roots["cache"]),
        "AGENTPLUGINS_HOME": str(roots["state"]), "TMPDIR": str(roots["tmp"]),
        "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_NOSYSTEM": "1",
        "GIT_TERMINAL_PROMPT": "0", "CI": "true",
    }
    local_path = getattr(args, "local_path", None)
    source = str(local_path.resolve()) if local_path is not None else f"{args.repository}@{args.revision}"
    if local_path is None and args.path != ".":
        source += f"//{args.path}"
    common = ["--target", args.targets]
    try:
        dry_run = invoke(args.cli, "add", [source, *common, "--dry-run"], env=environment, cwd=roots["workspace"])
        dry_data = dry_run.get("data")
        require(isinstance(dry_data, dict), "dry-run data is absent")
        if local_path is None:
            require(dry_data.get("revision") == args.revision, "dry-run revision differs")
        add = invoke(args.cli, "add", [source, *common], env=environment, cwd=roots["workspace"])
        add_data = batch(add, "add", clients)
        identity = checked_identity(add_data, args)
        outcomes = add_data.get("target_outcomes")
        require(isinstance(outcomes, dict) and set(outcomes) == set(clients), "add target outcomes differ")
        require(all(item.get("outcome") == "passed" for item in outcomes.values()), "add target outcome did not pass")
        info = invoke(args.cli, "info", [args.product_id, *common], env=environment, cwd=roots["workspace"])
        info_data = info.get("data")
        require(isinstance(info_data, dict) and info_data.get("name") == args.product_id, "info identity differs")
        doctor = invoke(args.cli, "doctor", [args.product_id, *common], env=environment, cwd=roots["workspace"])
        require(isinstance(doctor.get("data"), dict), "doctor data is absent")
        remove = invoke(
            args.cli, "remove", [args.product_id, *common, "--external-uninstalled", "--purge-data"],
            env=environment, cwd=roots["workspace"],
        )
        removed = batch(remove, "remove", clients)
        require(removed.get("plugin_data_preserved") is False and removed.get("data_retained") is False, "remove retained owned data")
        listed = invoke(args.cli, "list", [], env=environment, cwd=roots["workspace"])
        require(listed.get("data", {}).get("installations") == [], "installation remains after remove")
        observed_at = datetime.now(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")
        return {
            "schema_version": 1, "outcome": "passed", "product_id": args.product_id,
            "repository": args.repository, "revision": args.revision, "path": args.path,
            "materialized_source": {
                "kind": "local_bridge" if local_path is not None else "official_upstream",
                "path": str(local_path.resolve()) if local_path is not None else args.path,
            },
            "clients": clients, "installer_version": args.installer_version,
            "package": identity,
            "operations": {"dry_run": "passed", "add": "passed", "info": "passed", "doctor": "passed", "remove": "passed", "cleanup": "passed"},
            "observed_at": observed_at,
            "run": {"repository": args.run_repository, "id": args.run_id, "attempt": args.run_attempt, "source_sha": args.source_sha},
            "sandbox": {"kind": "disposable", "real_user_project_used": False, "removed": not args.keep_sandbox},
        }
    finally:
        if not args.keep_sandbox:
            shutil.rmtree(root, ignore_errors=False)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cli", type=Path, required=True)
    parser.add_argument("--installer-version", required=True)
    parser.add_argument("--product-id", required=True)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--revision", required=True)
    parser.add_argument("--path", required=True)
    parser.add_argument("--local-path", type=Path)
    parser.add_argument("--targets", required=True)
    parser.add_argument("--sandbox", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--run-repository", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--run-attempt", required=True)
    parser.add_argument("--source-sha", required=True)
    parser.add_argument("--keep-sandbox", action="store_true")
    args = parser.parse_args()
    try:
        result = run(args)
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
    except (MaterializationError, OSError, subprocess.SubprocessError, ValueError) as error:
        print(f"upstream promotion materialization failed: {error}", file=sys.stderr)
        return 1
    print("OK: exact upstream package materialization passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
