#!/usr/bin/env python3
"""Produce non-runtime policy conformance evidence from the exact 0.1.18 source."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import tempfile
from pathlib import Path
from typing import Any

from two_lane_evidence import (
    FIXTURE_KEY_ID,
    PLUGIN_KIT_COMMIT,
    PLUGIN_KIT_REPOSITORY,
    PLUGIN_KIT_TAG,
    RELEASE_CHECKSUMS_DIGEST,
    RELEASE_MANIFEST_DIGEST,
    POLICY_SCENARIO_IDS,
    canonical_json,
    sha256,
    sha256_file,
    validate_source_policy_evidence,
)


ROOT = Path(__file__).resolve().parents[1]
HARNESS = ROOT / "tests" / "e2e" / "source-policy-tests.json"
OVERLAY = ROOT / "tests" / "e2e" / "source-policy-overlay.json"
SCENARIOS = ROOT / "tests" / "e2e" / "launch-scenarios.json"
GIT = "/usr/bin/git"


def git(source: Path, *args: str) -> str:
    result = subprocess.run([GIT, "-C", str(source), *args], text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    if result.returncode:
        raise ValueError("plugin-kit source identity check failed")
    return result.stdout.strip()


def source_tree_digest(source: Path) -> str:
    """Digest all production bytes without depending on mutable Git metadata."""
    framed = bytearray(b"uap-source-policy-production-tree-v1\0")
    for path in sorted(item for item in source.rglob("*") if item.is_file() and ".git" not in item.parts):
        relative = path.relative_to(source).as_posix().encode()
        body = path.read_bytes()
        framed.extend(len(relative).to_bytes(8, "big") + relative)
        framed.extend(len(body).to_bytes(8, "big") + body)
    return sha256(bytes(framed))


def validate_source_identity(source: Path) -> str:
    if git(source, "rev-parse", "HEAD") != PLUGIN_KIT_COMMIT:
        raise ValueError("wrong plugin-kit commit")
    if git(source, "describe", "--tags", "--exact-match", "HEAD") != PLUGIN_KIT_TAG:
        raise ValueError("wrong plugin-kit tag")
    origins = git(source, "remote", "get-url", "origin")
    accepted = {
        f"https://github.com/{PLUGIN_KIT_REPOSITORY}.git",
        f"https://github.com/{PLUGIN_KIT_REPOSITORY}",
        f"git@github.com:{PLUGIN_KIT_REPOSITORY}.git",
    }
    if origins not in accepted:
        raise ValueError("wrong plugin-kit repository")
    if git(source, "status", "--porcelain", "--untracked-files=all"):
        raise ValueError("plugin-kit production source checkout is not clean")
    return source_tree_digest(source)


def validate_release_identity(manifest_path: Path, checksums_path: Path) -> tuple[str, str]:
    manifest = json.loads(manifest_path.read_text())
    if (
        manifest.get("repository") not in (None, PLUGIN_KIT_REPOSITORY)
        or manifest.get("tag") != PLUGIN_KIT_TAG
        or manifest.get("commit") != PLUGIN_KIT_COMMIT
        or manifest.get("version") != "0.1.18"
    ):
        raise ValueError("release manifest identity mismatch")
    checksums = checksums_path.read_text()
    if "agentplugins_0.1.18_linux_amd64" not in checksums:
        raise ValueError("release checksums omit the pinned Linux asset")
    identities = (sha256_file(manifest_path), sha256_file(checksums_path))
    if identities != (RELEASE_MANIFEST_DIGEST, RELEASE_CHECKSUMS_DIGEST):
        raise ValueError("release manifest/checksum bytes differ from the frozen 0.1.18 identity")
    return identities


def run_test(source: Path, package: str, name: str, go: str, *, scratch: Path) -> dict[str, Any]:
    environment = {
        key: os.environ[key] for key in ("PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT")
        if key in os.environ
    }
    command = [go, "test", "-count=1", "-run", f"^{name}$", package]
    environment.update({
        "CGO_ENABLED": "0", "HOME": str(scratch / "home"),
        "GOCACHE": str(scratch / "build-cache"),
        "GOTMPDIR": str(scratch / "tmp"),
    })
    for path in (scratch / "home", scratch / "build-cache", scratch / "tmp"):
        path.mkdir(parents=True, exist_ok=True)
    if "GOMODCACHE" in os.environ:
        environment["GOMODCACHE"] = os.environ["GOMODCACHE"]
    completed = subprocess.run(command, cwd=source, env=environment, text=True,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    transcript = canonical_json({
        "argv": command[1:], "exit_code": completed.returncode,
        "stdout_digest": sha256(completed.stdout.encode()),
        "stderr_digest": sha256(completed.stderr.encode()),
    })
    return {
        "package": package, "name": name, "passed": completed.returncode == 0,
        "transcript_digest": sha256(transcript),
    }


def produce(source: Path, manifest: Path, checksums: Path, *, go: str,
            uap_sha: str) -> dict[str, Any]:
    if len(uap_sha) != 40 or any(character not in "0123456789abcdef" for character in uap_sha):
        raise ValueError("invalid universal-agent-plugins source SHA")
    before = validate_source_identity(source)
    manifest_digest, checksums_digest = validate_release_identity(manifest, checksums)
    harness = json.loads(HARNESS.read_text())
    overlay = json.loads(OVERLAY.read_text())
    if (
        harness.get("fixture_key_id") != FIXTURE_KEY_ID
        or overlay.get("fixture_key_id") != FIXTURE_KEY_ID
        or tuple(harness.get("tests", {})) != POLICY_SCENARIO_IDS
        or overlay.get("runtime_claims") is not False
        or overlay.get("directory_contracts") != ["cold", "offline_lkg", "expired", "tampered", "sequence_rollback"]
    ):
        raise ValueError("source-policy harness/overlay contract drift")
    results = []
    with tempfile.TemporaryDirectory(prefix="uap-source-policy-go-") as temporary:
        scratch = Path(temporary)
        for scenario_id in POLICY_SCENARIO_IDS:
            spec = harness["tests"][scenario_id]
            test = run_test(source, spec["package"], spec["name"], go, scratch=scratch)
            proof = {
                "id": scenario_id, "source_test": test,
                "fixture_key_id": FIXTURE_KEY_ID,
                "overlay_digest": sha256_file(OVERLAY),
                "runtime_evidence_eligible": False,
            }
            if scenario_id == "revoked_operations_boundary":
                oracle = overlay["revoked_oracle"]
                stderr = oracle["stderr_template"].format(
                    distribution_id=oracle["distribution_id"],
                    release_sequence=oracle["release_sequence"],
                )
                proof["unit_oracle"] = {
                    "argv": oracle["argv"], "exit_code": oracle["exit_code"],
                    "stdout_digest": sha256(oracle["stdout"].encode()),
                    "stderr_digest": sha256(stderr.encode()),
                    "zero_mutation": oracle["zero_mutation"],
                    "runtime_evidence_eligible": False,
                }
            results.append({
                "id": scenario_id, "outcome": "passed" if test["passed"] else "failed",
                "test": test, "proof": proof, "proof_digest": sha256(canonical_json(proof)),
            })
    after = validate_source_identity(source)
    unchanged = before == after
    identities = {
        "plugin_kit_repository": PLUGIN_KIT_REPOSITORY,
        "plugin_kit_tag": PLUGIN_KIT_TAG,
        "plugin_kit_commit": PLUGIN_KIT_COMMIT,
        "release_manifest_digest": manifest_digest,
        "release_checksums_digest": checksums_digest,
        "uap_sha": uap_sha,
        "scenario_digest": sha256_file(SCENARIOS),
        "harness_digest": sha256_file(HARNESS),
        "overlay_digest": sha256_file(OVERLAY),
        "production_source_tree_before": before,
        "production_source_tree_after": after,
    }
    complete = unchanged and all(row["outcome"] == "passed" for row in results)
    value = {
        "schema_version": 1,
        "evidence_class": "source_policy_conformance",
        "runtime_claims": False,
        "released_binary_executed": False,
        "fixture_key_id": FIXTURE_KEY_ID,
        "identities": identities,
        "results": results,
        "production_source_unchanged": unchanged,
        "policy_conformance_gate_complete": complete,
    }
    if complete:
        validate_source_policy_evidence(
            value, scenario_digest=identities["scenario_digest"],
            harness_digest=identities["harness_digest"], overlay_digest=identities["overlay_digest"],
            uap_sha=uap_sha,
        )
    return value


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--release-manifest", type=Path, required=True)
    parser.add_argument("--release-checksums", type=Path, required=True)
    parser.add_argument("--go", default="go")
    parser.add_argument("--uap-sha", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if args.output.exists() or args.output.is_symlink():
        raise ValueError("policy evidence output must not already exist")
    value = produce(args.source.resolve(strict=True), args.release_manifest.resolve(strict=True),
                    args.release_checksums.resolve(strict=True), go=args.go, uap_sha=args.uap_sha)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_bytes(canonical_json(value))
    print(json.dumps({"policy_conformance_gate_complete": value["policy_conformance_gate_complete"]}))
    return 0 if value["policy_conformance_gate_complete"] else 2


if __name__ == "__main__":
    raise SystemExit(main())
