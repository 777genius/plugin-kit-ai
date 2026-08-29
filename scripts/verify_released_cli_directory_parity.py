#!/usr/bin/env python3
"""Verify released-CLI evidence against one exact signed Directory snapshot."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any


SEMVER = re.compile(r"^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$")


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def object_list(value: Any, label: str) -> list[dict[str, Any]]:
    require(isinstance(value, list), f"{label} must be a list")
    require(all(isinstance(item, dict) for item in value), f"{label} must contain objects")
    return value


def positive_int(value: Any, label: str) -> int:
    require(type(value) is int and value > 0, f"{label} must be a positive JSON integer")
    return value


def version(value: Any, label: str) -> tuple[int, int, int]:
    require(isinstance(value, str) and SEMVER.fullmatch(value) is not None, f"{label} must be a semantic version")
    return tuple(int(part) for part in value.split("."))  # type: ignore[return-value]


def one(items: list[dict[str, Any]], label: str) -> dict[str, Any]:
    require(len(items) == 1, f"expected exactly one {label}, found {len(items)}")
    return items[0]


def selected_release(snapshot: dict[str, Any], product_id: str, cli_version: str) -> tuple[dict[str, Any], dict[str, Any]]:
    products = object_list(snapshot.get("products"), "snapshot.products")
    product = one([item for item in products if item.get("id") == product_id], f"product {product_id!r}")
    distribution_id = product.get("default_distribution")
    require(isinstance(distribution_id, str) and distribution_id, "product.default_distribution must be a non-empty string")
    declared = product.get("distributions")
    require(isinstance(declared, list) and distribution_id in declared, "default distribution is not declared by the product")

    distributions = object_list(snapshot.get("distributions"), "snapshot.distributions")
    distribution = one(
        [item for item in distributions if item.get("id") == distribution_id],
        f"default distribution {distribution_id!r}",
    )
    require(distribution.get("product_id") == product_id, "default distribution belongs to a different product")
    require(distribution.get("status") == "active", "default distribution is not active")
    require(isinstance(distribution.get("kind"), str) and distribution["kind"], "default distribution kind must be a non-empty string")

    installer_version = version(cli_version, "CLI version")
    policies = object_list(distribution.get("release_policies"), "default distribution release_policies")
    applicable: list[dict[str, Any]] = []
    for policy in policies:
        targets = object_list(policy.get("targets"), "release policy targets")
        cursor_targets = [target for target in targets if target.get("client") == "cursor"]
        if policy.get("status") == "active" and cursor_targets:
            require(len(cursor_targets) == 1, "active release policy has ambiguous Cursor targets")
            if version(policy.get("minimum_installer_version"), "minimum_installer_version") <= installer_version:
                applicable.append(policy)
    policy = one(applicable, f"applicable active Cursor policy for CLI {cli_version}")
    sequence = positive_int(policy.get("release_sequence"), "release policy sequence")
    releases = object_list(distribution.get("releases"), "default distribution releases")
    for candidate in releases:
        positive_int(candidate.get("sequence"), "release sequence")
    release = one([item for item in releases if item["sequence"] == sequence], f"release sequence {sequence}")
    for field in ("package_version", "tree_digest", "manifest_digest"):
        require(isinstance(release.get(field), str) and release[field], f"selected release {field} must be a non-empty string")
    package_source = release.get("package_source")
    require(isinstance(package_source, dict), "selected release package_source must be an object")
    for field in ("repository", "revision", "path"):
        require(isinstance(package_source.get(field), str) and package_source[field], f"package_source.{field} must be a non-empty string")
    return distribution, release


def verify(
    snapshot: dict[str, Any], search: dict[str, Any], add: dict[str, Any], *,
    sequence: int, snapshot_digest: str, product_id: str, cli_version: str,
) -> None:
    require(positive_int(snapshot.get("sequence"), "snapshot sequence") == sequence, "snapshot sequence does not match the gated sequence")
    snapshot_schema = positive_int(snapshot.get("snapshot_schema_version"), "snapshot schema")
    distribution, release = selected_release(snapshot, product_id, cli_version)
    source = release["package_source"]

    require(search.get("result") == "success", "released CLI search did not succeed")
    search_data = search.get("data")
    require(isinstance(search_data, dict), "search.data must be an object")
    require(positive_int(search_data.get("snapshot_sequence"), "search snapshot sequence") == sequence, "search used a different snapshot sequence")
    require(search_data.get("snapshot_digest") == snapshot_digest, "search used a different snapshot digest")
    results = object_list(search_data.get("results"), "search.data.results")
    result = one(
        [
            item for item in results
            if item.get("product_id") == product_id
            and item.get("distribution_id") == distribution["id"]
        ],
        f"search result for {product_id!r} and default distribution {distribution['id']!r}",
    )
    require(result.get("status") == "available", "selected search result is not available")
    expected_search = {
        "distribution_id": distribution["id"],
        "distribution_kind": distribution["kind"],
        "package_version": release["package_version"],
        "install_selector": product_id,
        "repository": source["repository"],
        "revision": source["revision"],
        "package_path": source["path"],
    }
    for field, expected in expected_search.items():
        require(result.get(field) == expected, f"search result {field} does not match the selected release")

    require(add.get("result") == "success", "released CLI Cursor dry-run did not succeed")
    data = add.get("data")
    require(isinstance(data, dict), "add.data must be an object")
    targets = object_list(data.get("targets"), "add.data.targets")
    target = one(targets, "add target")
    require(target.get("target") == "cursor", "add dry-run selected a non-Cursor target")
    output = target.get("output")
    require(isinstance(output, dict), "Cursor target output must be an object")
    target_result = output.get("result")
    require(isinstance(target_result, dict), "Cursor target result must be an object")
    require(target_result.get("mutated") is False, "Cursor dry-run reported a mutation")
    plan = target_result.get("plan")
    require(isinstance(plan, dict), "Cursor target result plan must be an object")
    require(plan.get("client_id") == "cursor", "Cursor target result plan.client_id must be cursor")

    expected_add = {
        "plugin": product_id,
        "version": release.get("package_version"),
        "source": f"{source['repository']}//{source['path']}",
        "revision": source["revision"],
        "tree_digest": release.get("tree_digest"),
        "manifest_digest": release.get("manifest_digest"),
    }
    for container, label in ((data, "add.data"), (output, "Cursor target output")):
        for field, expected in expected_add.items():
            require(container.get(field) == expected, f"{label}.{field} does not match the selected release")
        require(container.get("dry_run") is True, f"{label}.dry_run must be true")

    directory = data.get("directory")
    require(isinstance(directory, dict), "add.data.directory must be an object")
    expected_directory = {
        "product_id": product_id,
        "distribution_id": distribution["id"],
        "distribution_kind": distribution["kind"],
        "snapshot_digest": snapshot_digest,
    }
    for field, expected in expected_directory.items():
        require(directory.get(field) == expected, f"add.data.directory.{field} does not match the selected release")
    require(positive_int(directory.get("desired_release_sequence"), "add.data.directory.desired_release_sequence") == release["sequence"], "add.data.directory.desired_release_sequence does not match the selected release")
    require(positive_int(directory.get("snapshot_schema"), "add.data.directory.snapshot_schema") == snapshot_schema, "add.data.directory.snapshot_schema does not match the selected snapshot")
    require(positive_int(directory.get("snapshot_sequence"), "add.data.directory.snapshot_sequence") == sequence, "add.data.directory.snapshot_sequence does not match the gated sequence")


def load(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text())
    require(isinstance(value, dict), f"{path} must contain a JSON object")
    return value


def load_snapshot(path: Path, expected_digest: str) -> dict[str, Any]:
    snapshot_bytes = path.read_bytes()
    actual_digest = f"sha256:{hashlib.sha256(snapshot_bytes).hexdigest()}"
    require(actual_digest == expected_digest, "snapshot bytes do not match --snapshot-digest")
    value = json.loads(snapshot_bytes)
    require(isinstance(value, dict), f"{path} must contain a JSON object")
    return value


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--snapshot", type=Path, required=True)
    parser.add_argument("--search", type=Path, required=True)
    parser.add_argument("--add", type=Path, required=True)
    parser.add_argument("--sequence", type=int, required=True)
    parser.add_argument("--snapshot-digest", required=True)
    parser.add_argument("--product-id", required=True)
    parser.add_argument("--cli-version", required=True)
    args = parser.parse_args()
    verify(
        load_snapshot(args.snapshot, args.snapshot_digest), load(args.search), load(args.add), sequence=args.sequence,
        snapshot_digest=args.snapshot_digest,
        product_id=args.product_id, cli_version=args.cli_version,
    )


if __name__ == "__main__":
    main()
