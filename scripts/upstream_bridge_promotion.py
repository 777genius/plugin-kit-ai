#!/usr/bin/env python3
"""Prepare and finalize a review-required locked bridge promotion."""

from __future__ import annotations

import argparse
import copy
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any

import yaml

sys.path.insert(0, str(Path(__file__).resolve().parent))
from build_bridges import BridgeError, build_one, load_recipe, sha256 as bridge_sha256
from build_registry import (
    RegistryError,
    canonical_manifest_repository,
    directory_preview,
    directory_search,
    encoded,
    validate_directory,
    validate_locked_npm_runtime,
    validated_package_facts,
)
from upstream_promotion import PromotionError, git, pretty, read_object, require, sha256


ROOT = Path(__file__).resolve().parents[1]
VERSION_RE = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$")
SHA_RE = re.compile(r"^[0-9a-f]{40}$")


def write_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(pretty(value))


def official_path(repository: Path, package_path: str, relative: str) -> Path:
    base = repository if package_path == "." else repository.joinpath(*package_path.split("/"))
    return base.joinpath(*relative.split("/"))


def exact_runtime_identity(entry: dict[str, Any], repository: Path) -> dict[str, str]:
    package_path = entry["package_path"]
    package_root = repository if package_path == "." else repository.joinpath(*package_path.split("/"))
    facts = validated_package_facts(package_root, require_directory_name=False)
    require(facts["manifest_name"] == entry["product_id"], "official bridge manifest name differs from product")
    manifest = read_object(package_root / "plugin.json")
    require(
        canonical_manifest_repository(manifest.get("repository")) == entry["repository"],
        "official bridge manifest repository differs from watched upstream",
    )
    mcp = read_object(package_root / "mcp.json")
    servers = mcp.get("mcpServers")
    bridge = entry["bridge"]
    require(isinstance(servers, dict) and set(servers) == {bridge["server"]}, "official bridge MCP inventory changed")
    server = servers[bridge["server"]]
    require(
        isinstance(server, dict) and server.get("type") == "stdio" and server.get("command") == "npx",
        "official bridge no longer uses the reviewed npx transport",
    )
    arguments = server.get("args")
    require(
        isinstance(arguments, list) and len(arguments) == 3
        and arguments[:2] == ["--prefix", "${PLUGIN_DATA}"]
        and isinstance(arguments[2], str),
        "official bridge npx arguments changed",
    )
    package_spec = arguments[2]
    npm_package, separator, version = package_spec.rpartition("@")
    require(separator == "@" and npm_package == bridge["npm_package"], "official bridge npm package changed")
    require(VERSION_RE.fullmatch(version) is not None, "official bridge npm version is not exact")
    require(version == facts["package_version"], "official plugin and npm runtime versions differ")
    upstream_package = read_object(official_path(repository, package_path, "package.json"))
    require(
        upstream_package.get("name") == npm_package and upstream_package.get("version") == version,
        "official package.json identity differs from the Agent Plugin runtime",
    )
    return {"npm_package": npm_package, "version": version, "package_version": f"{version}-uap.1"}


def generate_lock(
    npm: Path, bridge_id: str, npm_package: str, version: str, cache: Path | None,
) -> tuple[bytes, str]:
    with tempfile.TemporaryDirectory(prefix="upstream-bridge-lock-") as temporary:
        root = Path(temporary)
        package = {
            "name": f"agentplugins-runtime-{bridge_id}",
            "version": "1.0.0",
            "private": True,
            "dependencies": {npm_package: version},
        }
        write_json(root / "package.json", package)
        environment = {
            "PATH": os.environ.get("PATH", ""), "LANG": "C", "LC_ALL": "C",
            "npm_config_audit": "false", "npm_config_fund": "false",
            "npm_config_ignore_scripts": "true", "npm_config_update_notifier": "false",
        }
        if cache is not None:
            cache.mkdir(parents=True, exist_ok=True)
            environment["npm_config_cache"] = str(cache)
        completed = subprocess.run(
            [str(npm), "install", "--package-lock-only", "--ignore-scripts", "--omit=dev", "--no-audit", "--no-fund"],
            cwd=root, env=environment, check=False, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=180,
        )
        require(completed.returncode == 0, f"npm lock generation failed: {completed.stderr.decode('utf-8', 'replace')[:500]}")
        lock_body = (root / "package-lock.json").read_bytes()
        lock = json.loads(lock_body)
        packages = lock.get("packages") if isinstance(lock, dict) else None
        dependency = packages.get(f"node_modules/{npm_package}") if isinstance(packages, dict) else None
        require(
            isinstance(dependency, dict) and dependency.get("version") == version
            and isinstance(dependency.get("integrity"), str) and dependency["integrity"].startswith("sha512-"),
            "generated npm lock does not bind the exact runtime integrity",
        )
        return lock_body, dependency["integrity"]


def prepare(args: argparse.Namespace) -> dict[str, Any]:
    selection = read_object(args.selection)
    require(selection.get("decision") == "promote_bridge", "selection is not a locked bridge promotion")
    entry, metadata = selection["entry"], selection["pr_metadata"]
    require(entry["promotion_mode"] == "locked_bridge_manual", "bridge promotion mode is invalid")
    require(SHA_RE.fullmatch(metadata["merge_commit_oid"]) is not None, "official bridge merge SHA is invalid")
    identity = exact_runtime_identity(entry, args.official_repository)
    package_path = entry["package_path"]
    license_body = official_path(args.official_repository, package_path, "LICENSE").read_bytes()
    package_body = official_path(args.official_repository, package_path, "package.json").read_bytes()
    lock_body, npm_integrity = generate_lock(
        args.npm, entry["bridge"]["id"], identity["npm_package"], identity["version"], args.npm_cache,
    )

    bridge_id = entry["bridge"]["id"]
    recipe_path, recipe = load_recipe(args.root, bridge_id)
    require(recipe["distribution_id"] == entry["distribution_id"], "watched and recipe bridge distributions differ")
    recipe["upstream"]["repository"] = entry["repository"]
    recipe["upstream"]["revision"] = metadata["merge_commit_oid"]
    recipe["upstream"]["license"]["attribution_paths"] = [{"path": "LICENSE", "sha256": bridge_sha256(license_body)}]
    recipe["upstream"]["provenance"]["paths"] = [{"path": "package.json", "sha256": bridge_sha256(package_body)}]
    recipe["expected_version"] = identity["package_version"]
    recipe_path.write_text(yaml.safe_dump(recipe, sort_keys=False), encoding="utf-8")

    overlay = recipe_path.parent / recipe["overlay"]
    manifest = read_object(overlay / "plugin.json")
    manifest["version"] = identity["package_version"]
    write_json(overlay / "plugin.json", manifest)
    write_json(overlay / "mcp.json", {
        "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
        "mcpServers": {
            entry["bridge"]["server"]: {
                "type": "stdio", "command": "node",
                "args": ["${PLUGIN_ROOT}/io.github.777genius.agentplugins/runtime/launcher.mjs", *entry["bridge"]["runtime_args"]],
            },
        },
    })
    runtime_root = overlay / "io.github.777genius.agentplugins/runtime"
    write_json(runtime_root / "package.json", {
        "name": f"agentplugins-runtime-{bridge_id}", "version": "1.0.0", "private": True,
        "dependencies": {identity["npm_package"]: identity["version"]},
    })
    (runtime_root / "package-lock.json").write_bytes(lock_body)
    write_json(runtime_root / "runtime.json", {
        "schema_version": 1, "package": identity["npm_package"], "version": identity["version"],
        "entrypoint": entry["bridge"]["entrypoint"], "package_lock_sha256": sha256(lock_body),
        "omit_optional": False,
    })
    (overlay / "NOTICE").write_text(
        "Chrome DevTools community bridge\n\n"
        "This Agent Plugins package was independently assembled by 777genius. It is not\n"
        "authored, published, sponsored, or endorsed by ChromeDevTools or Google.\n\n"
        f"Upstream project: {entry['repository']}\n"
        f"Pinned revision: {metadata['merge_commit_oid']}\n"
        f"Upstream version: {identity['version']}\n"
        "Upstream license: Apache-2.0\n"
        f"License evidence: LICENSE (SHA-256 {bridge_sha256(license_body).removeprefix('sha256:')})\n"
        f"Command/version evidence: package.json (SHA-256 {bridge_sha256(package_body).removeprefix('sha256:')})\n\n"
        "No upstream source or executable bytes are copied into this package. The\n"
        "upstream files above are provenance evidence only.\n",
        encoding="utf-8",
    )
    (overlay / "README.md").write_text(
        "# Chrome DevTools\n\n"
        "Community package for Chrome DevTools MCP. Inspect pages, automate flows, analyze performance, and debug browser state.\n\n"
        "<!-- agentplugins-install:start -->\n## Install\n\n"
        "```bash\nnpx universal-agent-plugins add chrome-devtools --target codex\n```\n"
        "<!-- agentplugins-install:end -->\n\n"
        f"This package is independently assembled by 777genius from configuration anchored to {entry['repository']} at commit `{metadata['merge_commit_oid']}`. It is not authored, published, or endorsed by ChromeDevTools or Google.\n\n"
        "- Component: MCP server\n- Transport: `stdio`\n"
        f"- Runtime: integrity-locked `{identity['npm_package']}@{identity['version']}`; install scripts are disabled\n"
        "- Requirement: Node.js 22 or newer; the first launch downloads the locked npm closure into plugin data\n"
        "- Privacy: upstream usage statistics are disabled by default with `--no-usage-statistics`\n"
        f"- Upstream source: https://github.com/{entry['repository']}\n"
        "- Authentication: No service credential is declared; the launched browser controls its own session.\n\n"
        "Review the server's tools, scopes, and write capabilities before enabling it. Agent Plugins 1.0 standardizes packaging, not permissions or sandboxing.\n",
        encoding="utf-8",
    )

    report = build_one(args.root, bridge_id, args.upstream_mirror)
    output = args.root / report["package_path"]
    validate_locked_npm_runtime(output)
    plan = {
        "schema_version": 1, "promotion_kind": "locked_bridge", "auto_merge": False,
        "manual_review_required": True,
        "risk_signals": ["upstream_runtime_uses_live_npx", "new_upstream_runtime_bytes"],
        "product_id": entry["product_id"], "distribution_id": entry["distribution_id"],
        "release_sequence": entry["release_sequence"], "minimum_installer_version": entry["minimum_installer_version"],
        "upstream": {
            "repository": entry["repository"], "pull_request": entry["upstream_pr_number"],
            "reviewed_head_sha": entry["reviewed_head_sha"], "merge_sha": metadata["merge_commit_oid"],
            "merged_at": metadata["merged_at"], "package_path": package_path,
        },
        "runtime": {
            "npm_package": identity["npm_package"], "version": identity["version"],
            "integrity": npm_integrity, "package_lock_sha256": sha256(lock_body),
            "entrypoint": entry["bridge"]["entrypoint"],
        },
        "package": {
            "path": report["package_path"], "version": identity["package_version"],
            "tree_digest": report["tree_digest"], "manifest_digest": report["manifest_digest"],
            "components": [name for name, values in (("mcp", report["components"]["mcp_servers"]), ("skills", report["components"]["skills"])) if values],
        },
    }
    write_json(args.output, plan)
    return plan


def apply_bridge_release(directory: dict[str, Any], plan: dict[str, Any]) -> None:
    distributions = [item for item in directory["distributions"] if item["id"] == plan["distribution_id"]]
    require(len(distributions) == 1, "locked bridge distribution is absent or ambiguous")
    distribution = distributions[0]
    require(distribution["kind"] == "community_bridge" and distribution["product_id"] == plan["product_id"], "locked bridge distribution identity differs")
    sequence = plan["release_sequence"]
    require(sequence == max(item["sequence"] for item in distribution["releases"]) + 1, "locked bridge release sequence is not the next sequence")
    require(all(item["sequence"] != sequence for item in distribution["releases"]), "locked bridge release already exists")
    previous_revision = plan["previous_revision"]
    require(SHA_RE.fullmatch(previous_revision) is not None, "previous locked bridge revision is invalid")
    unresolved = [
        item for item in distribution["releases"]
        if item["package_source"]["repository"] == "777genius/universal-agent-plugins"
        and item["package_source"]["revision"] is None
    ]
    require(
        len(unresolved) == 1 and unresolved[0]["sequence"] == sequence - 1
        and unresolved[0]["package_source"]["path"] == f"plugins/{plan['product_id']}",
        "previous locked bridge release is not the exact unresolved predecessor",
    )
    unresolved[0]["package_source"]["revision"] = previous_revision
    current_policy = max(
        (item for item in distribution["release_policies"] if item["status"] == "active"),
        key=lambda item: item["release_sequence"],
    )
    package = plan["package"]
    distribution["releases"].append({
        "sequence": sequence, "package_version": package["version"], "manifest_name": plan["product_id"],
        "agent_plugins_schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
        "package_source": {"repository": "777genius/universal-agent-plugins", "revision": None, "path": package["path"]},
        "build_provenance": {
            "upstream_repository": plan["upstream"]["repository"],
            "upstream_revision": plan["upstream"]["merge_sha"],
        },
        "tree_digest_algorithm": "agentplugins-tree-sha256-v1", "tree_digest": package["tree_digest"],
        "manifest_digest": package["manifest_digest"], "components": package["components"],
    })
    policy = copy.deepcopy(current_policy)
    policy.update({
        "release_sequence": sequence, "status": "active",
        "minimum_installer_version": plan["minimum_installer_version"], "current_evidence": [],
    })
    distribution["release_policies"].append(policy)
    distribution["status"] = "active"


def finalize(args: argparse.Namespace) -> dict[str, Any]:
    selection, plan, materialization = map(read_object, (args.selection, args.plan, args.materialization))
    require(selection.get("decision") == "promote_bridge", "selection is not a bridge promotion")
    require(plan.get("promotion_kind") == "locked_bridge" and plan.get("auto_merge") is False, "bridge plan review boundary is invalid")
    require(materialization.get("outcome") == "passed", "locked bridge materialization did not pass")
    require(
        materialization.get("product_id") == plan["product_id"]
        and materialization.get("package", {}).get("tree_digest") == plan["package"]["tree_digest"]
        and materialization.get("package", {}).get("manifest_digest") == plan["package"]["manifest_digest"],
        "locked bridge materialization identity differs from the prepared package",
    )
    require(
        materialization.get("package", {}).get("package_version") == plan["package"]["version"]
        and materialization.get("clients") == [target["client"] for target in selection["entry"]["targets"]],
        "locked bridge materialization version or clients differ from the prepared package",
    )
    require(SHA_RE.fullmatch(args.artifact_revision) is not None, "bridge artifact revision is invalid")
    require(SHA_RE.fullmatch(args.previous_revision) is not None, "previous bridge revision is invalid")
    directory = read_object(args.directory)
    result = copy.deepcopy(plan)
    result["previous_revision"] = args.previous_revision
    apply_bridge_release(directory, result)
    validate_directory(directory, repository_root=args.root)
    args.directory.write_bytes(pretty(directory))
    result["materialization"] = {
        "artifact": {
            "repository": "777genius/universal-agent-plugins", "revision": args.artifact_revision,
            "path": args.artifact_path, "digest": sha256(args.materialization.read_bytes()),
        },
        "run": materialization["run"], "clients": materialization["clients"],
    }
    write_json(args.output, result)
    return result


def verify_pr(
    *, repository: Path, base_sha: str, head_sha: str, branch: str,
    product_id: str, short_sha: str, commits: list[str], audit_path: str,
) -> dict[str, Any]:
    require(len(commits) == 2, "locked bridge promotion PR must contain exactly two commits")
    bridge_commit, promotion_commit = commits
    require(promotion_commit == head_sha, "locked bridge promotion commit is not the PR head")
    require(git(repository, "rev-parse", f"{bridge_commit}^") == base_sha, "locked bridge commit is not based on the PR base")
    require(git(repository, "rev-parse", f"{promotion_commit}^") == bridge_commit, "locked bridge promotion is not based on bridge evidence")

    bridge_paths = git(repository, "diff", "--name-only", base_sha, bridge_commit).splitlines()
    evidence_paths = [item for item in bridge_paths if item.startswith(f"evidence/upstream-promotions/{product_id}/")]
    require(len(evidence_paths) == 1, "locked bridge commit must contain exactly one materialization record")
    raw_path = evidence_paths[0]
    require(raw_path.endswith("/materialization.json") and f"/{short_sha}" in raw_path, "locked bridge evidence path differs from branch")
    require(
        all(
            item == raw_path or item.startswith(f"bridges/{product_id}/") or item.startswith(f"plugins/{product_id}/")
            for item in bridge_paths
        ),
        "locked bridge commit changed paths outside its recipe, package, and evidence",
    )
    required_bridge_paths = {
        f"bridges/{product_id}/bridge.yaml", f"bridges/{product_id}/overlay/plugin.json",
        f"bridges/{product_id}/overlay/mcp.json",
        f"bridges/{product_id}/overlay/io.github.777genius.agentplugins/runtime/package.json",
        f"bridges/{product_id}/overlay/io.github.777genius.agentplugins/runtime/package-lock.json",
        f"bridges/{product_id}/overlay/io.github.777genius.agentplugins/runtime/runtime.json",
        f"plugins/{product_id}/plugin.json", f"plugins/{product_id}/mcp.json",
        f"plugins/{product_id}/io.github.777genius.agentplugins/runtime/package.json",
        f"plugins/{product_id}/io.github.777genius.agentplugins/runtime/package-lock.json",
        f"plugins/{product_id}/io.github.777genius.agentplugins/runtime/runtime.json",
    }
    require(
        all((repository / item).is_file() and not (repository / item).is_symlink() for item in required_bridge_paths),
        "locked bridge commit omitted required reviewed boundaries",
    )
    required_changes = required_bridge_paths - {
        f"bridges/{product_id}/overlay/mcp.json", f"plugins/{product_id}/mcp.json",
    }
    require(required_changes <= set(bridge_paths), "locked bridge update omitted required runtime identity changes")

    promotion_paths = sorted(git(repository, "diff", "--name-only", bridge_commit, head_sha).splitlines())
    allowed_promotion_paths = {
        "registry/directory.json", "registry/review-preview.json", "registry/review-search.json", audit_path,
    }
    require(
        set(promotion_paths) <= allowed_promotion_paths
        and {"registry/directory.json", audit_path} <= set(promotion_paths),
        f"locked bridge promotion changed unexpected review paths: {promotion_paths!r}",
    )
    raw, review = read_object(repository / raw_path), read_object(repository / audit_path)
    require(review.get("promotion_kind") == "locked_bridge", "locked bridge review kind is invalid")
    require(review.get("auto_merge") is False and review.get("manual_review_required") is True, "locked bridge manual review boundary is absent")
    require(
        review.get("risk_signals") == ["upstream_runtime_uses_live_npx", "new_upstream_runtime_bytes"],
        "locked bridge risk signals changed",
    )
    upstream = review["upstream"]
    require(
        upstream["merge_sha"] == raw.get("revision") and upstream["merge_sha"].startswith(short_sha)
        and upstream["repository"] == raw.get("repository") and upstream["package_path"] == raw.get("path"),
        "locked bridge upstream identity differs from materialization evidence",
    )
    require(
        audit_path == f"registry/upstream-promotion-audit/{product_id}/{upstream['merge_sha']}/manual-review.json",
        "locked bridge review path differs from the exact upstream merge",
    )
    with tempfile.TemporaryDirectory(prefix="verify-locked-bridge-watch-") as temporary:
        watch_path = Path(temporary) / "upstream-promotions.json"
        watch_path.write_bytes(subprocess.run(
            ["git", "show", f"{base_sha}:registry/upstream-promotions.json"], cwd=repository,
            check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30,
            env={"PATH": os.environ.get("PATH", ""), "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull, "LANG": "C", "LC_ALL": "C"},
        ).stdout)
        from upstream_promotion import validate_watch
        watched = [item for item in validate_watch(watch_path)["entries"] if item["product_id"] == product_id]
    require(len(watched) == 1, "locked bridge product is absent or ambiguous in the trusted watch list")
    entry = watched[0]
    require(
        entry["promotion_mode"] == "locked_bridge_manual"
        and entry["distribution_id"] == review["distribution_id"]
        and entry["release_sequence"] == review["release_sequence"]
        and entry["minimum_installer_version"] == review["minimum_installer_version"]
        and entry["repository"] == upstream["repository"]
        and entry["upstream_pr_number"] == upstream["pull_request"]
        and entry["reviewed_head_sha"] == upstream["reviewed_head_sha"]
        and entry["package_path"] == upstream["package_path"],
        "locked bridge review differs from the trusted watch policy",
    )
    artifact = review.get("materialization", {}).get("artifact", {})
    require(
        artifact == {
            "repository": "777genius/universal-agent-plugins", "revision": bridge_commit,
            "path": raw_path, "digest": sha256((repository / raw_path).read_bytes()),
        },
        "locked bridge review does not bind the exact evidence commit",
    )
    require(
        raw.get("outcome") == "passed" and raw.get("product_id") == product_id
        and raw.get("materialized_source", {}).get("kind") == "local_bridge"
        and raw.get("package", {}).get("tree_digest") == review["package"]["tree_digest"]
        and raw.get("package", {}).get("manifest_digest") == review["package"]["manifest_digest"]
        and raw.get("package", {}).get("package_version") == review["package"]["version"]
        and raw.get("clients") == [target["client"] for target in entry["targets"]],
        "locked bridge materialization did not bind the prepared package",
    )
    package_root = repository / review["package"]["path"]
    require(review["package"]["path"] == f"plugins/{product_id}", "locked bridge package path is invalid")
    validate_locked_npm_runtime(package_root)
    runtime_root = package_root / "io.github.777genius.agentplugins/runtime"
    runtime = read_object(runtime_root / "runtime.json")
    lock_body = (runtime_root / "package-lock.json").read_bytes()
    lock = read_object(runtime_root / "package-lock.json")
    locked_package = lock.get("packages", {}).get(f"node_modules/{entry['bridge']['npm_package']}")
    require(
        review["runtime"] == {
            "npm_package": entry["bridge"]["npm_package"],
            "version": runtime.get("version"),
            "integrity": locked_package.get("integrity") if isinstance(locked_package, dict) else None,
            "package_lock_sha256": sha256(lock_body),
            "entrypoint": runtime.get("entrypoint"),
        },
        "locked bridge review does not bind the committed runtime closure",
    )
    require(
        review["runtime"]["entrypoint"] == entry["bridge"]["entrypoint"]
        and review["runtime"]["npm_package"] == entry["bridge"]["npm_package"],
        "locked bridge runtime differs from the trusted bridge policy",
    )
    mcp = read_object(package_root / "mcp.json")
    server = mcp.get("mcpServers", {}).get(entry["bridge"]["server"])
    require(
        isinstance(server, dict) and server.get("command") == "node"
        and server.get("args") == [
            "${PLUGIN_ROOT}/io.github.777genius.agentplugins/runtime/launcher.mjs",
            *entry["bridge"]["runtime_args"],
        ],
        "locked bridge MCP command differs from the trusted bridge policy",
    )
    candidate_directory = read_object(repository / "registry/directory.json")
    validate_directory(candidate_directory, repository_root=repository)
    with tempfile.TemporaryDirectory(prefix="verify-locked-bridge-promotion-") as temporary:
        reconstructed_path = Path(temporary) / "directory.json"
        base_directory = subprocess.run(
            ["git", "show", f"{base_sha}:registry/directory.json"], cwd=repository,
            check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30,
            env={"PATH": os.environ.get("PATH", ""), "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull, "LANG": "C", "LC_ALL": "C"},
        ).stdout
        reconstructed = json.loads(base_directory)
        apply_bridge_release(reconstructed, review)
        reconstructed_path.write_bytes(pretty(reconstructed))
        require(reconstructed_path.read_bytes() == (repository / "registry/directory.json").read_bytes(), "locked bridge Directory change is not deterministic")
        require(encoded(directory_preview(reconstructed)) == (repository / "registry/review-preview.json").read_bytes(), "locked bridge review preview is stale")
        require(encoded(directory_search(reconstructed)) == (repository / "registry/review-search.json").read_bytes(), "locked bridge review search is stale")
    run = raw.get("run")
    require(isinstance(run, dict), "locked bridge observer run identity is absent")
    return {
        "schema_version": 1, "outcome": "verified", "product_id": product_id,
        "head_sha": head_sha, "auto_merge": False, "promotion_kind": "locked_bridge",
        "observer_run_id": run["id"], "observer_run_attempt": run["attempt"],
        "observer_source_sha": run["source_sha"], "materialization_path": raw_path,
        "materialization_digest": sha256((repository / raw_path).read_bytes()),
        "upstream_repository": upstream["repository"], "upstream_pr_number": upstream["pull_request"],
        "upstream_reviewed_head": upstream["reviewed_head_sha"], "upstream_merge_sha": upstream["merge_sha"],
    }


def parser() -> argparse.ArgumentParser:
    root = argparse.ArgumentParser(description=__doc__)
    commands = root.add_subparsers(dest="command", required=True)
    prepare_parser = commands.add_parser("prepare")
    prepare_parser.add_argument("--selection", type=Path, required=True)
    prepare_parser.add_argument("--official-repository", type=Path, required=True)
    prepare_parser.add_argument("--root", type=Path, default=ROOT)
    prepare_parser.add_argument("--npm", type=Path, required=True)
    prepare_parser.add_argument("--npm-cache", type=Path)
    prepare_parser.add_argument("--upstream-mirror", type=Path)
    prepare_parser.add_argument("--output", type=Path, required=True)
    finalize_parser = commands.add_parser("finalize")
    finalize_parser.add_argument("--selection", type=Path, required=True)
    finalize_parser.add_argument("--plan", type=Path, required=True)
    finalize_parser.add_argument("--materialization", type=Path, required=True)
    finalize_parser.add_argument("--artifact-revision", required=True)
    finalize_parser.add_argument("--previous-revision", required=True)
    finalize_parser.add_argument("--artifact-path", required=True)
    finalize_parser.add_argument("--directory", type=Path, required=True)
    finalize_parser.add_argument("--output", type=Path, required=True)
    finalize_parser.add_argument("--root", type=Path, default=ROOT)
    return root


def main() -> int:
    args = parser().parse_args()
    try:
        result = prepare(args) if args.command == "prepare" else finalize(args)
    except (PromotionError, BridgeError, RegistryError, OSError, ValueError, KeyError, json.JSONDecodeError, subprocess.SubprocessError) as error:
        print(f"upstream bridge promotion failed: {error}", file=sys.stderr)
        return 1
    print(json.dumps({"schema_version": 1, "outcome": "prepared" if args.command == "prepare" else "finalized", "product_id": result["product_id"]}, sort_keys=True, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
