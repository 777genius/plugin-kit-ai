#!/usr/bin/env python3
"""Execute one manifest-bound release facade and emit challenge-correlated evidence."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import shutil
import subprocess
from datetime import datetime, timezone
from pathlib import Path


PROOF_MODE = "local-frozen-release-asset-v1"
PROOF_MODE_ENV = "AGENTPLUGINS_INTERNAL_PROOF_MODE"
PROOF_BINARY_ENV = "AGENTPLUGINS_INTERNAL_PROOF_BINARY"


def now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def verify_installed_npm_payload(binary_cache_root: Path, native: dict[str, object]) -> tuple[Path, str]:
    root = binary_cache_root.resolve()
    expected_digest = "sha256:" + str(native["sha256"])
    matches: list[Path] = []
    for candidate in root.rglob("*"):
        if candidate.is_file() and not candidate.is_symlink() and candidate.stat().st_size == native["size"]:
            digest = "sha256:" + hashlib.sha256(candidate.read_bytes()).hexdigest()
            if digest == expected_digest:
                matches.append(candidate.resolve())
    if len(matches) != 1 or root not in matches[0].parents or not os.access(matches[0], os.X_OK):
        raise RuntimeError("installed npm executable does not match the authenticated GitHub release binary")
    return matches[0], expected_digest


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--context", type=Path, required=True)
    parser.add_argument("--executable", required=True)
    parser.add_argument("--asset-name", required=True)
    parser.add_argument("--kind", choices=("binary", "npm"), required=True)
    parser.add_argument("--os", required=True)
    parser.add_argument("--architecture", required=True)
    parser.add_argument("--node-major", type=int)
    parser.add_argument("--npm-project", type=Path)
    parser.add_argument("--npm-package-root", type=Path)
    parser.add_argument("--npm-binary-cache", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    context = json.loads(args.context.read_text())
    if args.kind == "npm" and (
        args.node_major != 22
        or args.npm_project is None
        or args.npm_package_root is None
        or args.npm_binary_cache is None
    ):
        raise RuntimeError("npm facade evidence requires Node 22 and exact install/cache roots")
    executable = shutil.which(args.executable) or args.executable
    execution_environment = None
    if args.kind == "npm":
        package_root = args.npm_package_root.resolve()
        if package_root not in Path(executable).resolve().parents:
            raise RuntimeError("npm facade executable is outside the exact installed package")
        proof_asset = (args.context.parent / "release" / args.asset_name).resolve()
        execution_environment = os.environ.copy()
        execution_environment.update(
            {
                "AGENTPLUGINS_CACHE_DIR": str(args.npm_binary_cache.resolve()),
                PROOF_MODE_ENV: PROOF_MODE,
                PROOF_BINARY_ENV: str(proof_asset),
            }
        )
    started = now()
    completed = subprocess.run(
        [executable, "version"], text=True, capture_output=True, timeout=30,
        check=False, env=execution_environment,
    )
    observed = now()
    version = completed.stdout.strip().removeprefix("agentplugins ").strip()
    if completed.returncode or version != context["release_manifest"]["version"]:
        raise RuntimeError("manifest-bound facade returned an unexpected version")
    if args.kind == "npm":
        declared = context.get("npm_package")
        if not declared or declared.get("name") != "universal-agent-plugins" or declared.get("version") != version:
            raise RuntimeError("npm facade is not bound to exact registry package metadata")
        asset_path = args.context.parent / "npm" / f"universal-agent-plugins-{version}.tgz"
        declared_digest = declared["sha256"]
        native = next((item for item in context["release_manifest"]["assets"].values() if item["file"] == args.asset_name), None)
        if native is None:
            raise RuntimeError("npm facade native asset is absent from the authenticated release manifest")
        executable_path, executable_digest = verify_installed_npm_payload(args.npm_binary_cache, native)
        withheld = executable_path.with_name(executable_path.name + ".launch-evidence-withheld")
        executable_path.rename(withheld)
        try:
            blocked_environment = execution_environment.copy()
            blocked_environment[PROOF_BINARY_ENV] = str(args.npm_binary_cache / ".missing-proof-asset")
            without_native = subprocess.run(
                [executable, "version"], text=True, capture_output=True,
                timeout=30, check=False, env=blocked_environment,
            )
        finally:
            withheld.rename(executable_path)
        if without_native.returncode == 0:
            raise RuntimeError("npm facade does not execute the authenticated GitHub release binary")
        direct_native = subprocess.run([str(executable_path), "version"], text=True, capture_output=True, timeout=30, check=False)
        if direct_native.returncode or direct_native.stdout.strip().removeprefix("agentplugins ").strip() != version:
            raise RuntimeError("authenticated installed npm native payload is not executable")
        lock = json.loads((args.npm_project / "package-lock.json").read_text())
        locked = lock.get("packages", {}).get("node_modules/universal-agent-plugins", {})
        if locked.get("version") != version or locked.get("resolved") != declared["tarball"] or locked.get("integrity") != declared["integrity"]:
            raise RuntimeError("npm lock does not bind the exact registry tarball URL, version, and SRI")
        provenance = subprocess.run(["npm", "audit", "signatures", "--json"], cwd=args.npm_project, text=True, capture_output=True, timeout=120, check=False)
        if provenance.returncode:
            raise RuntimeError("npm registry provenance/signature verification failed")
    else:
        declared = next(item for item in context["release_manifest"]["assets"].values() if item["file"] == args.asset_name)
        asset_path = args.context.parent / "release" / args.asset_name
        declared_digest = declared["sha256"]
    asset_body = asset_path.read_bytes()
    if len(asset_body) != declared["size"] or hashlib.sha256(asset_body).hexdigest() != declared_digest:
        raise RuntimeError("executed facade asset differs from its authenticated distribution identity")
    value = {
        "schema_version": 1, "kind": args.kind, "os": args.os, "architecture": args.architecture,
        "node_major": args.node_major, "executed": True, "version": version,
        "catalog_repository": context["catalog_repository"], "catalog_sha": context["github"]["sha"],
        "cli_release_repository": context["cli_release_repository"], "cli_release_tag": context["cli_release_tag"],
        "release_manifest_digest": context["release_manifest_digest"],
        "release_checksums_digest": context["release_checksums_digest"],
        "github_release_identity": context["github_release_identity"],
        "directory_digest": context["directory"]["digest"],
        "asset_name": args.asset_name, "asset_digest": "sha256:" + (native["sha256"] if args.kind == "npm" else declared_digest),
        "github_asset_attestation": context["github_asset_attestation"],
        "challenge": context["challenge"]["value"], "challenge_context": context["challenge"],
        "started_at": started, "observed_at": observed,
        "command_trace": {
            "challenge": context["challenge"]["value"], "argv": ["agentplugins", "version"],
            "started_at": started, "ended_at": observed, "exit_code": completed.returncode,
            "stdout_digest": "sha256:" + hashlib.sha256(completed.stdout.encode()).hexdigest(),
            "stderr_digest": "sha256:" + hashlib.sha256(completed.stderr.encode()).hexdigest(),
        },
        "runner_platform": platform.platform(),
    }
    if args.kind == "npm":
        value["npm_package"] = {
            "name": declared["name"], "version": declared["version"],
            "integrity": declared["integrity"], "tarball": declared["tarball"],
            "metadata_digest": declared["metadata_digest"],
            "provenance_url": declared["provenance_url"],
            "provenance_predicate_type": declared["provenance_predicate_type"],
            "installed_executable_digest": executable_digest,
            "native_asset_name": args.asset_name,
            "native_asset_digest": "sha256:" + native["sha256"],
            "provenance_verified": True,
            "provenance_output_digest": "sha256:" + hashlib.sha256(provenance.stdout.encode()).hexdigest(),
        }
    args.output.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
