#!/usr/bin/env python3
"""Build the observer's ChatGPT app binding and projection receipt.

The two output files are projections of already authenticated inputs.  This
command does not register an app, resolve a package, or invent release
metadata: the app identity comes from the signed Directory target, package and
source identity come from the selected signed release, and the native files
must be the exact projection retained by released agentplugins state.
"""

from __future__ import annotations

import argparse
import ctypes
import errno
import hashlib
import json
import os
import re
import secrets
import stat
import subprocess
import sys
from dataclasses import dataclass
from datetime import timezone
from pathlib import Path
from typing import Any
from urllib.parse import urlsplit

sys.path.insert(0, str(Path(__file__).resolve().parent))

from directory_publication import (  # noqa: E402
    MAX_ENVELOPE_BYTES,
    MAX_LATEST_BYTES,
    MAX_SNAPSHOT_BYTES,
    PublicationError,
    canonical_json as directory_json,
    load_public_keys,
    parse_json_bytes,
    parse_timestamp,
    read_bytes_bounded,
    require,
    sha256_digest,
    validate_latest,
    validate_snapshot_semantics,
    verify_envelope,
)


@dataclass(frozen=True)
class ProtectedAuthority:
    path: Path
    identities: tuple[tuple[int, int], ...]


@dataclass(frozen=True)
class ProtectedTree:
    authority: ProtectedAuthority
    seal: str
    package_contract: bool


from observe_launch_scenario import (  # noqa: E402
    strict_state_json_loads,
    validate_released_state_v4,
)
from portable_paths import (  # noqa: E402
    MAX_DEPTH as PORTABLE_MAX_DEPTH,
    MAX_FILES as PORTABLE_MAX_FILES,
    MAX_FILE_BYTES as PORTABLE_MAX_FILE_BYTES,
    MAX_TREE_BYTES as PORTABLE_MAX_TREE_BYTES,
    validate_segment,
)


MAX_EVIDENCE_BYTES = 8 << 20
MAX_PROJECTION_BYTES = 64 << 10
MAX_BINARY_BYTES = 256 << 20
MAX_PROJECTION_FILES = 10_000
MAX_PROJECTION_TREE_BYTES = 256 << 20
MAX_SNAPSHOT_DIRECTORIES = 512
CURRENT_INSTALLER_VERSION = "0.1.24"
CURRENT_LINUX_AMD64_DIGEST = "sha256:e79125f7ffabd11c6e211d6b049c2eb2b36eb1aba3a76ce27cac819aeba1e6ca"
APP_ID = re.compile(r"plugin_asdk_app_[a-f0-9]{32}")
NAME = re.compile(r"[a-z0-9]+(?:-[a-z0-9]+)*")
SEMVER = re.compile(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)")
TIMESTAMP = re.compile(r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z")
INSTALLATION_ID = re.compile(
    r"[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}"
)


def exact_object(value: Any, required: set[str], optional: set[str], label: str) -> dict[str, Any]:
    require(isinstance(value, dict), f"{label} must be an object")
    require(required <= set(value) <= required | optional, f"{label} fields are invalid")
    return value


def one(values: list[dict[str, Any]], label: str) -> dict[str, Any]:
    require(len(values) == 1, f"expected exactly one {label}, found {len(values)}")
    return values[0]


def semver(value: Any, label: str) -> tuple[int, int, int]:
    require(isinstance(value, str) and SEMVER.fullmatch(value) is not None, f"{label} is invalid")
    return tuple(int(part) for part in value.split("."))  # type: ignore[return-value]


def canonical_output(value: Any) -> bytes:
    """Match the official package JSON profile consumed by the observer."""
    body = directory_json(value)
    return body[:-1]


def _go_json(value: Any, *, indent: bool) -> bytes:
    """Encode the JSON subset exactly like Go's encoding/json package."""
    kwargs: dict[str, Any] = {"allow_nan": False, "ensure_ascii": False, "sort_keys": True}
    if indent:
        kwargs["indent"] = 2
    else:
        kwargs["separators"] = (",", ":")
    body = json.dumps(value, **kwargs)
    # encoding/json escapes these runes even when all other non-ASCII text is
    # emitted as UTF-8. Python deliberately does not, so bind the difference.
    body = (
        body.replace("&", r"\u0026")
        .replace("<", r"\u003c")
        .replace(">", r"\u003e")
        .replace("\u2028", r"\u2028")
        .replace("\u2029", r"\u2029")
    )
    return body.encode("utf-8")


def released_json(value: Any) -> bytes:
    """Match Go json.MarshalIndent plus the newline used by agentplugins 0.1.24."""
    return _go_json(value, indent=True) + b"\n"


def released_manifest_json(value: dict[str, Any]) -> bytes:
    """Match the projected manifest map containing a typed Go Author value."""
    ordered: dict[str, Any] = {}
    for key in sorted(value):
        item = value[key]
        if key == "author" and isinstance(item, dict):
            # domain.Author is a struct, so encoding/json preserves declaration
            # order rather than recursively sorting these fields as map keys.
            author: dict[str, str] = {}
            for field in ("name", "email", "url"):
                author_value = item.get(field, "")
                require(isinstance(author_value, str), "projected manifest author is invalid")
                if author_value:
                    author[field] = author_value
            item = author
        ordered[key] = item
    kwargs: dict[str, Any] = {"allow_nan": False, "ensure_ascii": False, "indent": 2, "sort_keys": False}
    body = json.dumps(ordered, **kwargs)
    body = (
        body.replace("&", r"\u0026")
        .replace("<", r"\u003c")
        .replace(">", r"\u003e")
        .replace("\u2028", r"\u2028")
        .replace("\u2029", r"\u2029")
    )
    return body.encode("utf-8") + b"\n"


def released_compact_json(value: Any) -> bytes:
    """Match Go json.Marshal used for the synthesized Directory app manifest."""
    return _go_json(value, indent=False)


def physical_artifact_id(declared_name: str, installation_id: str) -> str:
    """Match domain.ComputePhysicalArtifactID in the released Go CLI."""
    suffix = hashlib.sha256(installation_id.encode()).hexdigest()[:12]
    maximum_name_bytes = 64 - 1 - len(suffix)
    name = declared_name.strip().encode("utf-8")[:maximum_name_bytes].decode("utf-8", "strict")
    name = name.rstrip(".-")
    return f"{name}-{suffix}"


def read_regular_bounded(path: Path, limit: int, label: str) -> bytes:
    before = path.lstat()
    require(stat.S_ISREG(before.st_mode) and before.st_nlink == 1, f"{label} must be a one-link regular file")
    body = read_bytes_bounded(path, limit)
    after = path.lstat()
    require(
        (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns, before.st_ctime_ns)
        == (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns, after.st_ctime_ns),
        f"{label} changed while reading",
    )
    return body


def load_object(path: Path, limit: int, label: str) -> tuple[dict[str, Any], bytes]:
    body = read_regular_bounded(path, limit, label)
    value = parse_json_bytes(body, label, max_bytes=limit)
    require(isinstance(value, dict), f"{label} must be an object")
    return value, body


def verified_snapshot(feed: Path, trusted_keys: Path, now: str, minimum_sequence: int) -> tuple[dict[str, Any], str]:
    latest, latest_body = load_object(feed / "latest.json", MAX_LATEST_BYTES, "latest pointer")
    require(directory_json(latest) == latest_body, "latest pointer is not canonical JSON")
    validate_latest(latest)
    require(latest["sequence"] >= minimum_sequence, "Directory snapshot is below the requested sequence floor")
    limits = latest["fetch_contract"]
    snapshot_body = read_regular_bounded(feed / latest["snapshot_path"], limits["snapshot_max_bytes"], "snapshot")
    require(len(snapshot_body) <= MAX_SNAPSHOT_BYTES, "snapshot exceeds the implementation limit")
    envelope, envelope_body = load_object(feed / latest["envelope_path"], limits["envelope_max_bytes"], "signature envelope")
    require(len(envelope_body) <= MAX_ENVELOPE_BYTES, "signature envelope exceeds the implementation limit")
    require(directory_json(envelope) == envelope_body, "signature envelope is not canonical JSON")
    verify_envelope(snapshot_body, envelope, load_public_keys(trusted_keys))
    snapshot = parse_json_bytes(snapshot_body, "snapshot", max_bytes=MAX_SNAPSHOT_BYTES)
    require(isinstance(snapshot, dict), "snapshot must be an object")
    validate_snapshot_semantics(snapshot)
    require(snapshot["sequence"] == envelope["sequence"] == latest["sequence"], "Directory artifact sequence mismatch")
    moment = parse_timestamp(now, "now")
    require(moment >= parse_timestamp(snapshot["generated_at"], "generated_at"), "Directory snapshot is not yet valid")
    require(moment < parse_timestamp(snapshot["expires_at"], "expires_at"), "Directory snapshot is expired")
    return snapshot, sha256_digest(snapshot_body)


def selected_identity(
    snapshot: dict[str, Any], *, product_id: str, distribution_id: str,
    release_sequence: int, installer_version: str, app_key: str, app_id: str,
) -> tuple[dict[str, Any], dict[str, Any], dict[str, Any]]:
    product = one([item for item in snapshot["products"] if item["id"] == product_id], f"product {product_id!r}")
    require(product["default_distribution"] == distribution_id, "requested distribution is not the product default")
    require(distribution_id in product["distributions"], "requested distribution is not declared by the product")
    distribution = one([item for item in snapshot["distributions"] if item["id"] == distribution_id], f"distribution {distribution_id!r}")
    require(distribution["product_id"] == product_id and distribution["status"] == "active", "requested distribution is not active for the product")
    require([item for item in snapshot["revocations"] if item["distribution_id"] == distribution_id and item["release_sequence"] == release_sequence] == [], "requested release is revoked")
    eligible: list[tuple[dict[str, Any], dict[str, Any]]] = []
    for policy in distribution["release_policies"]:
        targets = [target for target in policy["targets"] if target["client"] == "chatgpt"]
        require(len(targets) <= 1, "release policy has ambiguous ChatGPT targets")
        if (
            policy["status"] == "active" and targets
            and semver(policy["minimum_installer_version"], "minimum installer version") <= semver(installer_version, "installer version")
        ):
            eligible.append((policy, targets[0]))
    selected = one(
        [{"policy": policy, "target": target} for policy, target in eligible],
        "eligible active ChatGPT release policy",
    )
    policy, target = selected["policy"], selected["target"]
    require(policy["release_sequence"] == release_sequence, "requested release is not the eligible ChatGPT release")
    binding = target.get("app_binding")
    exact_object(binding, {"app_key", "id", "mcp_server"}, set(), "signed ChatGPT app binding")
    require(binding["app_key"] == app_key, "requested app key differs from the signed target")
    require(binding["id"] == app_id, "requested app ID differs from the signed target")
    release = one([item for item in distribution["releases"] if item["sequence"] == release_sequence], f"release {release_sequence}")
    return distribution, release, binding


def validate_add(
    add: dict[str, Any], *, product_id: str, distribution: dict[str, Any], release: dict[str, Any],
    snapshot: dict[str, Any], snapshot_digest: str, binding: dict[str, Any],
) -> tuple[str, str]:
    exact_object(add, {"schema_version", "command", "result", "data"}, set(), "CLI add evidence")
    require(isinstance(add["schema_version"], int) and not isinstance(add["schema_version"], bool) and add["schema_version"] == 1, "CLI add schema is invalid")
    require(add["command"] == "add" and add["result"] == "success", "CLI add did not succeed")
    data = exact_object(
        add["data"],
        {
            "operation_id", "batch", "status", "succeeded", "failed", "plugin", "version",
            "source", "revision", "tree_digest", "manifest_digest", "dry_run", "targets",
            "acquisition", "target_outcomes", "directory",
        },
        set(),
        "CLI add data",
    )
    source = release["package_source"]
    expected = {
        "plugin": product_id, "version": release["package_version"],
        "source": f"{source['repository']}//{source['path']}", "revision": source["revision"],
        "tree_digest": release["tree_digest"], "manifest_digest": release["manifest_digest"],
    }
    require(data["dry_run"] is False, "CLI add evidence must be a real isolated materialization")
    for field, value in expected.items():
        require(isinstance(data.get(field), str) and data.get(field) == value, f"CLI add {field} differs from the signed release")
    operation_id = data.get("operation_id")
    require(isinstance(operation_id, str) and operation_id != "", "CLI add operation ID is invalid")
    require(
        data.get("batch") is True and data.get("status") == "completed"
        and isinstance(data.get("succeeded"), int) and not isinstance(data["succeeded"], bool) and data["succeeded"] == 1
        and isinstance(data.get("failed"), int) and not isinstance(data["failed"], bool) and data["failed"] == 0,
        "CLI add completion summary is invalid",
    )
    acquisition = exact_object(
        data["acquisition"],
        {"acquisition_id", "acquisition_count", "tree_digest", "manifest_digest", "closure_digest", "source_kind", "fetched", "validated"},
        set(), "CLI acquisition",
    )
    require(
        isinstance(acquisition["acquisition_id"], str) and acquisition["acquisition_id"] != ""
        and isinstance(acquisition["acquisition_count"], int) and not isinstance(acquisition["acquisition_count"], bool)
        and acquisition["acquisition_count"] == 1
        and acquisition["tree_digest"] == release["tree_digest"]
        and acquisition["manifest_digest"] == release["manifest_digest"]
        and isinstance(acquisition["closure_digest"], str) and re.fullmatch(r"sha256:[a-f0-9]{64}", acquisition["closure_digest"])
        and acquisition["source_kind"] in {"github", "directory"}
        and acquisition["fetched"] is True and acquisition["validated"] is True,
        "CLI acquisition identity differs from the signed release",
    )
    outcomes = exact_object(data["target_outcomes"], {"chatgpt"}, set(), "CLI target outcomes")
    outcome = exact_object(
        outcomes["chatgpt"],
        {"outcome", "acquisition_id", "tree_digest", "manifest_digest", "closure_digest"},
        set(), "CLI ChatGPT target outcome",
    )
    require(
        outcome == {
            "outcome": "passed", "acquisition_id": acquisition["acquisition_id"],
            "tree_digest": acquisition["tree_digest"], "manifest_digest": acquisition["manifest_digest"],
            "closure_digest": acquisition["closure_digest"],
        },
        "CLI ChatGPT target outcome differs from the single acquisition",
    )
    directory = exact_object(
        data["directory"],
        {"product_id", "distribution_id", "distribution_kind", "desired_release_sequence", "snapshot_schema", "snapshot_sequence", "snapshot_digest"},
        set(), "CLI add Directory evidence",
    )
    expected_directory = {
        "product_id": product_id, "distribution_id": distribution["id"], "distribution_kind": distribution["kind"],
        "desired_release_sequence": release["sequence"], "snapshot_schema": snapshot["snapshot_schema_version"],
        "snapshot_sequence": snapshot["sequence"], "snapshot_digest": snapshot_digest,
    }
    require(directory == expected_directory, "CLI add Directory identity differs from the signed snapshot")
    require(isinstance(data["targets"], list), "CLI add targets must be a list")
    target = one([item for item in data["targets"] if isinstance(item, dict) and item.get("target") == "chatgpt"], "ChatGPT add target")
    require(len(data["targets"]) == 1, "CLI add evidence contains ambiguous targets")
    exact_object(target, {"target", "status", "next_action", "output"}, set(), "CLI ChatGPT target")
    require(target["status"] == "external_completed" and isinstance(target["next_action"], str) and target["next_action"] != "", "CLI ChatGPT target did not complete external preparation")
    output = exact_object(
        target["output"],
        {"operation_id", "plugin", "version", "source", "revision", "tree_digest", "manifest_digest", "dry_run", "result", "next_action"},
        set(), "CLI ChatGPT output",
    )
    for field, value in expected.items():
        require(isinstance(output.get(field), str) and output.get(field) == value, f"CLI ChatGPT output {field} differs from the signed release")
    require(output["dry_run"] is False, "CLI ChatGPT output is a dry run")
    require(output["next_action"] == target["next_action"], "CLI ChatGPT next action is inconsistent")
    require(output["operation_id"] == operation_id, "CLI ChatGPT operation ID differs")
    result = exact_object(
        output["result"], {"installation_id", "plan", "activation", "requires_confirmation", "mutated", "group_phase"}, set(),
        "CLI ChatGPT result",
    )
    require(result["mutated"] is True, "CLI ChatGPT add did not materialize the package")
    installation_id = result["installation_id"]
    require(
        isinstance(installation_id, str) and INSTALLATION_ID.fullmatch(installation_id) is not None,
        "CLI ChatGPT installation ID is invalid",
    )
    require(result["requires_confirmation"] is False and result["group_phase"] == "external_completed", "CLI ChatGPT group completion is invalid")
    plan = exact_object(
        result["plan"],
        {"client_id", "scope", "status", "package_mode", "activation", "authentication", "policy", "verification", "physical_artifact_id", "components", "user_actions", "warnings"},
        set(), "CLI ChatGPT plan",
    )
    require(
        plan["client_id"] == "chatgpt" and plan["scope"] == "user"
        and plan["status"] == "manual_activation_required"
        and plan["package_mode"] == "compatibility_projection"
        and plan["activation"] == "manual_activation_required"
        and plan["policy"] == "allowed" and plan["verification"] == "package_validated",
        "CLI ChatGPT plan selected another client or lifecycle",
    )
    components = plan.get("components")
    require(isinstance(components, list), "CLI ChatGPT plan components are invalid")
    require(all(isinstance(item, dict) and set(item) == {"kind", "name", "support"} for item in components), "CLI ChatGPT plan component fields are invalid")
    require(
        {(item["kind"], item["name"], item["support"]) for item in components}
        == {("app", binding["app_key"], "projected"), ("mcp_server", binding["mcp_server"], "projected")}
        and len(components) == 2,
        "CLI ChatGPT plan did not project the exact signed app and MCP components",
    )
    activation = exact_object(
        result["activation"], {"activation", "authentication", "policy", "verification", "user_actions"}, set(),
        "CLI ChatGPT activation",
    )
    require(
        activation["activation"] == "manual_activation_required"
        and activation["policy"] == "allowed" and activation["verification"] == "package_validated",
        "CLI ChatGPT activation state is invalid",
    )
    artifact_id = plan["physical_artifact_id"]
    require(
        isinstance(artifact_id, str)
        and artifact_id == physical_artifact_id(product_id, installation_id),
        "CLI ChatGPT physical artifact ID is invalid",
    )
    return installation_id, artifact_id


def _snapshot_identity(metadata: os.stat_result) -> tuple[int, int, int, int, int, int, int]:
    return (
        metadata.st_dev, metadata.st_ino, metadata.st_mode, metadata.st_nlink,
        metadata.st_size, metadata.st_mtime_ns, metadata.st_ctime_ns,
    )


def _open_absolute_directory(path: Path, label: str) -> tuple[Path, list[int]]:
    require(path.is_absolute(), f"{label} must be an absolute path")
    require(all(part not in {"", ".", ".."} for part in path.parts[1:]), f"{label} contains an unsafe component")
    flags = os.O_RDONLY | os.O_CLOEXEC | getattr(os, "O_DIRECTORY", 0) | getattr(os, "O_NOFOLLOW", 0)
    descriptors = [os.open(path.anchor, flags)]
    try:
        for component in path.parts[1:]:
            descriptors.append(os.open(component, flags, dir_fd=descriptors[-1]))
        return path, descriptors
    except Exception:
        for descriptor in reversed(descriptors):
            os.close(descriptor)
        raise


def _read_snapshot_file(descriptor: int, size: int, label: str) -> bytes:
    require(0 <= size <= MAX_PROJECTION_TREE_BYTES, f"{label} size is invalid")
    body = bytearray()
    while len(body) < size:
        block = os.read(descriptor, min(1 << 20, size - len(body)))
        require(bool(block), f"{label} was truncated while reading")
        body.extend(block)
    require(os.read(descriptor, 1) == b"", f"{label} grew while reading")
    return bytes(body)


def _tree_seal(entries: dict[str, dict[str, Any]], contents: dict[str, bytes]) -> str:
    digest = hashlib.sha256()
    for relative in sorted(entries):
        metadata = entries[relative]
        record = (
            relative, metadata["kind"], stat.S_IMODE(metadata["mode"]), metadata["size"],
            *metadata["identity"], tuple(metadata.get("children", ())),
        )
        encoded = repr(record).encode("utf-8")
        digest.update(len(encoded).to_bytes(8, "big"))
        digest.update(encoded)
        body = contents.get(relative, b"")
        digest.update(len(body).to_bytes(8, "big"))
        digest.update(body)
    return "sha256:" + digest.hexdigest()


def _stable_tree_snapshot(
    root: Path, label: str, *, package_contract: bool = False,
) -> tuple[dict[str, dict[str, Any]], dict[str, bytes], ProtectedTree]:
    """Read and finally revalidate every member through held no-follow descriptors."""
    try:
        absolute, ancestry = _open_absolute_directory(root, label)
    except OSError as error:
        raise PublicationError(f"{label} path is not a real directory: {error}") from error
    held: list[int] = []
    entries: dict[str, dict[str, Any]] = {}
    contents: dict[str, bytes] = {}
    file_count = 0
    directory_count = 1
    total_bytes = 0
    discovered_members = 0
    folded_paths: dict[str, str] = {}

    def visible_names(descriptor: int, relative: str, expected_count: int | None = None) -> list[str]:
        nonlocal discovered_members
        maximum_files = PORTABLE_MAX_FILES if package_contract else MAX_PROJECTION_FILES
        member_budget = maximum_files + MAX_SNAPSHOT_DIRECTORIES - 1
        visible: list[str] = []
        with os.scandir(descriptor) as iterator:
            for item in iterator:
                if expected_count is None:
                    require(discovered_members < member_budget, f"{label} exceeds snapshot member limits")
                    discovered_members += 1
                else:
                    require(len(visible) < expected_count, f"{label} directory {relative or '.'!r} changed after traversal")
                visible.append(item.name)
        for name in visible:
            metadata = os.stat(name, dir_fd=descriptor, follow_symlinks=False)
            child_relative = name if not relative else f"{relative}/{name}"
            require(
                not (name.casefold() == ".git" and stat.S_ISDIR(metadata.st_mode))
                and not (name.casefold() == ".plugin-kit-ai.lock" and stat.S_ISREG(metadata.st_mode)),
                f"{label} contains reserved ownership metadata: {child_relative!r}",
            )
        return sorted(visible)

    def traverse(descriptor: int, relative: str) -> None:
        nonlocal directory_count, file_count, total_bytes
        metadata = os.fstat(descriptor)
        require(stat.S_ISDIR(metadata.st_mode), f"{label} directory {relative or '.'!r} is invalid")
        entry = {
            "kind": "dir", "mode": metadata.st_mode, "size": 0,
            "identity": _snapshot_identity(metadata), "descriptor": descriptor, "children": visible_names(descriptor, relative),
        }
        entries[relative or "."] = entry
        for name in entry["children"]:
            require(name not in {"", ".", ".."} and "/" not in name, f"{label} contains an unsafe member")
            child_relative = name if not relative else f"{relative}/{name}"
            if package_contract:
                parts = child_relative.split("/")
                require(len(parts) <= PORTABLE_MAX_DEPTH, f"{label} path exceeds depth {PORTABLE_MAX_DEPTH}: {child_relative!r}")
                for part in parts:
                    try:
                        validate_segment(part)
                    except ValueError as error:
                        raise PublicationError(f"{label} has invalid path {child_relative!r}: {error}") from error
                    require(part.casefold() != ".git", f"{label} contains reserved Git metadata path: {child_relative!r}")
                require(child_relative.casefold() != ".plugin-kit-ai.lock", f"{label} contains reserved ownership-marker path: {child_relative!r}")
                folded = child_relative.casefold()
                require(folded not in folded_paths or folded_paths[folded] == child_relative, f"{label} contains a case-confusable path collision")
                folded_paths[folded] = child_relative
            child_metadata = os.stat(name, dir_fd=descriptor, follow_symlinks=False)
            if stat.S_ISDIR(child_metadata.st_mode):
                directory_count += 1
                require(directory_count <= MAX_SNAPSHOT_DIRECTORIES, f"{label} has too many directories")
                child = os.open(
                    name,
                    os.O_RDONLY | os.O_CLOEXEC | getattr(os, "O_DIRECTORY", 0) | getattr(os, "O_NOFOLLOW", 0),
                    dir_fd=descriptor,
                )
                held.append(child)
                require(_snapshot_identity(os.fstat(child)) == _snapshot_identity(child_metadata), f"{label} directory {child_relative!r} changed while opening")
                traverse(child, child_relative)
            else:
                require(stat.S_ISREG(child_metadata.st_mode) and child_metadata.st_nlink == 1, f"{label} file {child_relative!r} must be a one-link regular file")
                file_count += 1
                total_bytes += child_metadata.st_size
                maximum_files = PORTABLE_MAX_FILES if package_contract else MAX_PROJECTION_FILES
                maximum_tree = PORTABLE_MAX_TREE_BYTES if package_contract else MAX_PROJECTION_TREE_BYTES
                require(file_count <= maximum_files and total_bytes <= maximum_tree, f"{label} exceeds snapshot limits")
                if package_contract:
                    require(child_metadata.st_size <= PORTABLE_MAX_FILE_BYTES, f"{label} file {child_relative!r} exceeds its size limit")
                child = os.open(name, os.O_RDONLY | os.O_CLOEXEC | getattr(os, "O_NOFOLLOW", 0), dir_fd=descriptor)
                try:
                    opened = os.fstat(child)
                    require(_snapshot_identity(opened) == _snapshot_identity(child_metadata), f"{label} file {child_relative!r} changed while opening")
                    contents[child_relative] = _read_snapshot_file(child, opened.st_size, f"{label} file {child_relative!r}")
                    require(_snapshot_identity(os.fstat(child)) == _snapshot_identity(opened), f"{label} file {child_relative!r} changed while reading")
                    entries[child_relative] = {
                        "kind": "file", "mode": opened.st_mode, "size": opened.st_size,
                        "identity": _snapshot_identity(opened),
                    }
                finally:
                    os.close(child)

    def revalidate(relative: str) -> None:
        entry = entries[relative or "."]
        if entry["kind"] != "dir":
            return
        descriptor = entry["descriptor"]
        require(_snapshot_identity(os.fstat(descriptor)) == entry["identity"], f"{label} {relative or 'root'} changed after reading")
        require(
            visible_names(descriptor, relative, len(entry["children"])) == entry["children"],
            f"{label} directory {relative or '.'!r} changed after traversal",
        )
        for name in entry["children"]:
            child_relative = name if not relative else f"{relative}/{name}"
            child_entry = entries[child_relative]
            flags = os.O_RDONLY | os.O_CLOEXEC | getattr(os, "O_NOFOLLOW", 0)
            if child_entry["kind"] == "dir":
                flags |= getattr(os, "O_DIRECTORY", 0)
            reopened = os.open(name, flags, dir_fd=descriptor)
            try:
                require(_snapshot_identity(os.fstat(reopened)) == child_entry["identity"], f"{label} member {child_relative!r} changed after reading")
            finally:
                os.close(reopened)
            revalidate(child_relative)

    try:
        root_descriptor = ancestry[-1]
        traverse(root_descriptor, "")
        revalidate("")
        # Also prove that the original absolute pathname still reaches every held ancestor.
        identities = [_directory_identity(os.fstat(descriptor)) for descriptor in ancestry]
        reopened_path, reopened = _open_absolute_directory(absolute, label)
        try:
            require(reopened_path == absolute and len(reopened) == len(ancestry), f"{label} authority is incomplete")
            require(
                [_directory_identity(os.fstat(descriptor)) for descriptor in reopened] == identities,
                f"{label} pathname changed while reading",
            )
        finally:
            for descriptor in reversed(reopened):
                os.close(descriptor)
        return entries, contents, ProtectedTree(
            ProtectedAuthority(absolute, tuple(identities)),
            _tree_seal(entries, contents), package_contract,
        )
    except OSError as error:
        raise PublicationError(f"{label} changed or contains an unsafe member: {error}") from error
    finally:
        for descriptor in reversed(held):
            os.close(descriptor)
        for descriptor in reversed(ancestry):
            os.close(descriptor)


def _projection_artifact_snapshot_details(
    root: Path,
) -> tuple[str, dict[str, bytes], set[str], ProtectedTree]:
    """Reproduce the released manager's packagesnapshot digest over a stable tree."""
    entries, contents, authority = _stable_tree_snapshot(root, "ChatGPT projection")
    digest = hashlib.sha256()
    for relative in sorted(name for name in entries if name != "."):
        metadata = entries[relative]
        if metadata["kind"] == "dir":
            digest.update(b"dir\0" + relative.encode() + b"\0false\0" + b"0\0")
            continue
        executable = metadata["mode"] & 0o111 != 0
        digest.update(f"file\0{relative}\0{'true' if executable else 'false'}\0{metadata['size']}\0".encode())
        digest.update(contents[relative])
    directories = {name for name, metadata in entries.items() if name != "." and metadata["kind"] == "dir"}
    return "sha256:" + digest.hexdigest(), contents, directories, authority


def projection_artifact_snapshot(root: Path) -> tuple[str, dict[str, bytes]]:
    digest, contents, _, _ = _projection_artifact_snapshot_details(root)
    return digest, contents


def projection_artifact_digest(root: Path) -> str:
    return projection_artifact_snapshot(root)[0]


def authenticated_package_projection(
    root: Path, release: dict[str, Any], binding: dict[str, Any], product_id: str,
    physical_artifact_id: str,
) -> tuple[dict[str, Any], str, dict[str, Any], dict[str, bytes], ProtectedTree]:
    """Authenticate source package bytes and derive the security-relevant projection."""
    entries, files, authority = _stable_tree_snapshot(root, "approved source package", package_contract=True)
    manifest_body = files.get("plugin.json")
    require(manifest_body is not None, "approved source package plugin.json is missing")
    manifest = parse_json_bytes(manifest_body, "approved source package plugin.json", max_bytes=MAX_PROJECTION_BYTES)
    require(isinstance(manifest, dict), "approved source package plugin.json must be an object")
    require(
        manifest.get("$schema") == "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
        and manifest.get("name") == product_id and manifest.get("version") == release["package_version"],
        "approved source package identity differs from the signed release",
    )
    tree = hashlib.sha256()
    domain = b"agentplugins.package-tree\x00sha256\x00v1"
    tree.update(len(domain).to_bytes(8, "big"))
    tree.update(domain)
    for relative in sorted(name for name in entries if name != "."):
        metadata = entries[relative]
        kind = b"directory" if metadata["kind"] == "dir" else b"file"
        mode = (
            b"100755" if metadata["kind"] == "file" and metadata["mode"] & 0o111
            else b"100644" if metadata["kind"] == "file" else b"040000"
        )
        for field in (b"entry", relative.encode(), kind, mode, b""):
            tree.update(len(field).to_bytes(8, "big"))
            tree.update(field)
        body = files.get(relative, b"")
        tree.update(len(body).to_bytes(8, "big"))
        if metadata["kind"] == "file":
            tree.update(body)
    identity = {
        "tree_digest": "sha256:" + tree.hexdigest(),
        "manifest_digest": "sha256:" + hashlib.sha256(manifest_body).hexdigest(),
    }
    require(
        identity == {"tree_digest": release["tree_digest"], "manifest_digest": release["manifest_digest"]},
        "approved source package bytes differ from the signed release",
    )
    source_members = set(entries) - {"."}
    require(
        source_members <= {"plugin.json", "mcp.json", "README.md", "NOTICE"}
        and {"README.md", "NOTICE"} <= set(files)
        and all(entries[name]["kind"] == "file" for name in source_members),
        "approved source package is outside the supported remote-app projection contract",
    )
    mcp_body = files.get("mcp.json")
    require(mcp_body is not None, "approved source package mcp.json is missing")
    portable = parse_json_bytes(mcp_body, "approved source package mcp.json", max_bytes=MAX_PROJECTION_BYTES)
    portable = exact_object(portable, {"mcpServers"}, {"$schema"}, "approved source package mcp.json")
    require(portable.get("$schema") == "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json", "approved source package mcp.json schema is invalid")
    servers = portable["mcpServers"]
    require(isinstance(servers, dict) and set(servers) == {binding["mcp_server"]}, "approved source package has ambiguous MCP servers")
    server = exact_object(servers[binding["mcp_server"]], {"type", "url"}, set(), "approved source package MCP server")
    require(server["type"] == "streamable-http" and isinstance(server["url"], str), "approved source package MCP transport is invalid")
    expected_mcp = {
        "mcpServers": {
            binding["mcp_server"]: {"type": "http", "url": server["url"]},
        },
    }
    expected_manifest = {"name": manifest["name"]}
    for field in ("version", "description", "homepage", "repository", "license"):
        if isinstance(manifest.get(field), str) and manifest[field].strip():
            expected_manifest[field] = manifest[field]
    if manifest.get("author") is not None:
        author = manifest["author"]
        require(isinstance(author, dict), "approved source package author is invalid")
        typed_author: dict[str, str] = {}
        for field in ("name", "email", "url"):
            value = author.get(field, "")
            require(isinstance(value, str), "approved source package author is invalid")
            if value:
                typed_author[field] = value
        expected_manifest["author"] = typed_author
    if isinstance(manifest.get("keywords"), list) and manifest["keywords"]:
        expected_manifest["keywords"] = manifest["keywords"]
    expected_manifest.update({"mcpServers": "./.mcp.json", "apps": "./.app.json"})
    marketplace_name = "agentplugins-" + hashlib.sha256(physical_artifact_id.strip().encode()).hexdigest()[:12]
    marketplace = {
        "name": marketplace_name,
        "plugins": [{
            "name": product_id,
            "source": {"source": "local", "path": "./"},
            "policy": {"installation": "AVAILABLE", "authentication": "ON_INSTALL"},
            "category": "Productivity",
        }],
    }
    expected_files = {
        "README.md": files["README.md"],
        "NOTICE": files["NOTICE"],
        ".agents/plugins/marketplace.json": released_json(marketplace),
    }
    return expected_mcp, server["url"], expected_manifest, expected_files, authority


def validate_state(
    state: dict[str, Any], projection_root: Path, *, product_id: str,
    distribution: dict[str, Any], release: dict[str, Any], snapshot: dict[str, Any], snapshot_digest: str,
    binding: dict[str, Any], installation_id: str, physical_artifact_id: str,
    managed_digest: str,
) -> None:
    require(validate_released_state_v4(state), "released CLI State v4 is invalid")
    matches = [item for item in state["installations"] if item.get("declared_name") == product_id]
    installation = one(matches, f"State v4 installation {product_id!r}")
    require(installation.get("installation_id") == installation_id, "State v4 installation ID differs from the add result")
    require(installation.get("origin_mode") == "directory", "State v4 installation did not originate from Directory")
    expected_directory = {
        "product_id": product_id, "distribution_id": distribution["id"], "distribution_kind": distribution["kind"],
        "desired_release_sequence": release["sequence"], "snapshot_schema": snapshot["snapshot_schema_version"],
        "snapshot_sequence": snapshot["sequence"], "snapshot_digest": snapshot_digest,
    }
    require(installation.get("directory") == expected_directory, "State v4 Directory identity differs")
    source = installation.get("source")
    package_source = release["package_source"]
    require(isinstance(source, dict), "State v4 source is invalid")
    expected_source = {
        "repository": package_source["repository"], "package_subpath": package_source["path"],
        "resolved_revision": package_source["revision"], "tree_digest": release["tree_digest"],
    }
    for field, value in expected_source.items():
        require(source.get(field) == value, f"State v4 source {field} differs from the signed release")
    package = installation.get("package")
    require(isinstance(package, dict), "State v4 package is invalid")
    require(package.get("version") == release["package_version"] and package.get("manifest_digest") == release["manifest_digest"], "State v4 package identity differs")
    clients = installation.get("clients")
    require(isinstance(clients, dict), "State v4 clients are invalid")
    client = one([item for item in clients.values() if isinstance(item, dict) and item.get("client_id") == "chatgpt"], "State v4 ChatGPT client")
    require(len(clients) == 1, "State v4 installation contains ambiguous clients")
    require(Path(client.get("target_locator", "")).resolve() == projection_root.resolve(), "State v4 target locator differs from the supplied projection")
    require(client.get("physical_artifact_id") == physical_artifact_id, "State v4 physical artifact differs from the add result")
    revision = client.get("package_revision")
    require(isinstance(revision, dict), "State v4 ChatGPT package revision is missing")
    expected_revision = {
        "version": release["package_version"], "resolved_revision": package_source["revision"],
        "tree_digest": release["tree_digest"], "manifest_digest": release["manifest_digest"],
        "distribution_id": distribution["id"], "release_sequence": release["sequence"],
    }
    for field, value in expected_revision.items():
        require(revision.get(field) == value, f"State v4 ChatGPT revision {field} differs")
    catalog = revision.get("catalog_evidence")
    if catalog is not None:
        require(isinstance(catalog, dict) and catalog.get("schema_version") == 2, "State v4 catalog evidence is invalid")
        compatibility = catalog.get("compatibility")
        require(isinstance(compatibility, dict), "State v4 compatibility evidence is invalid")
        chat = compatibility.get("chatgpt")
        require(isinstance(chat, dict), "State v4 ChatGPT catalog evidence is missing")
        app = chat.get("app_binding")
        require(isinstance(app, dict), "State v4 app binding evidence is missing")
        for field in ("app_key", "id", "mcp_server"):
            require(app.get(field) == binding[field], f"State v4 app binding {field} differs from the signed target")
    native = one(
        [
            item for item in client.get("native_objects", [])
            if isinstance(item, dict) and item.get("kind") == "managed_package_directory"
        ],
        "State v4 managed ChatGPT projection",
    )
    require(
        native.get("path") == str(projection_root)
        and native.get("logical_name") == product_id
        and native.get("protection_class") == "managed"
        and native.get("managed_digest") == managed_digest,
        "State v4 managed projection digest differs from the actual projection bytes",
    )
    receipts = client.get("receipts")
    require(isinstance(receipts, list) and receipts, "State v4 ChatGPT mutation receipt is missing")
    receipt = receipts[-1]
    require(
        isinstance(receipt, dict)
        and receipt.get("phase") == "committed"
        and receipt.get("active_path") == str(projection_root)
        and receipt.get("after_digest") == managed_digest,
        "State v4 mutation receipt does not bind the actual projection bytes",
    )


def _projection_object(files: dict[str, bytes], name: str, label: str) -> dict[str, Any]:
    body = files.get(name)
    require(body is not None, f"{label} is missing from the authenticated projection snapshot")
    value = parse_json_bytes(body, label, max_bytes=MAX_PROJECTION_BYTES)
    require(isinstance(value, dict), f"{label} must be an object")
    return value


def validate_projection(
    files: dict[str, bytes], binding: dict[str, Any], product_id: str,
    expected_mcp: dict[str, Any], expected_url: str, expected_manifest: dict[str, Any],
    expected_files: dict[str, bytes], directories: set[str],
) -> tuple[str, dict[str, str]]:
    manifest = _projection_object(files, ".codex-plugin/plugin.json", "ChatGPT official manifest")
    require(manifest == expected_manifest, "ChatGPT official manifest was not derived from the approved source package")
    app = _projection_object(files, ".app.json", "ChatGPT .app.json")
    expected_app = {"apps": {binding["app_key"]: {"id": binding["id"]}}}
    require(app == expected_app, "ChatGPT .app.json differs from the signed target")
    mcp = _projection_object(files, ".mcp.json", "ChatGPT .mcp.json")
    require(mcp == expected_mcp, "ChatGPT .mcp.json was not derived from the approved source package")
    require(
        files[".codex-plugin/plugin.json"] == released_manifest_json(expected_manifest),
        "ChatGPT official manifest bytes differ from the released stager",
    )
    require(
        files[".mcp.json"] == released_json(expected_mcp),
        "ChatGPT .mcp.json bytes differ from the released stager",
    )
    require(
        files[".app.json"] == released_compact_json(expected_app),
        "ChatGPT .app.json bytes differ from the released Directory synthesis",
    )
    expected_members = {
        ".codex-plugin/plugin.json", ".app.json", ".mcp.json", *expected_files,
    }
    require(
        set(files) == expected_members
        and directories == {".codex-plugin", ".agents", ".agents/plugins"},
        "ChatGPT projection has members outside the exact approved transformation",
    )
    for name, body in expected_files.items():
        require(files[name] == body, f"ChatGPT projection file {name!r} differs from the approved transformation")
    servers = exact_object(mcp, {"mcpServers"}, set(), "ChatGPT .mcp.json")["mcpServers"]
    require(isinstance(servers, dict) and set(servers) == {binding["mcp_server"]}, "ChatGPT .mcp.json has ambiguous MCP servers")
    server = exact_object(servers[binding["mcp_server"]], {"url", "type"}, set(), "ChatGPT MCP server")
    require(server["type"] == "http" and isinstance(server["url"], str), "ChatGPT MCP projection is invalid")
    require(server["url"] == expected_url, "ChatGPT MCP URL differs from the approved source package")
    parsed = urlsplit(expected_url)
    try:
        expected_url.encode("ascii")
        port = parsed.port
    except (UnicodeEncodeError, ValueError) as error:
        raise PublicationError(f"ChatGPT MCP URL is not canonical ASCII HTTPS: {error}") from error
    require(parsed.scheme == "https" and parsed.hostname is not None and parsed.username is None and parsed.password is None and not parsed.query and not parsed.fragment, "ChatGPT MCP URL is unsafe")
    path_segments = parsed.path.split("/")[1:]
    require(
        parsed.hostname == parsed.hostname.lower() and port is None
        and parsed.netloc == parsed.hostname and parsed.path.startswith("/")
        and all(segment not in {"", ".", ".."} for segment in path_segments)
        and "%" not in parsed.path and "\\" not in parsed.path
        and expected_url == f"https://{parsed.hostname}{parsed.path}",
        "ChatGPT MCP endpoint is not canonical HTTPS",
    )
    digests = {
        "app_digest": "sha256:" + hashlib.sha256(files[".app.json"]).hexdigest(),
        "mcp_digest": "sha256:" + hashlib.sha256(files[".mcp.json"]).hexdigest(),
        "manifest_digest": "sha256:" + hashlib.sha256(files[".codex-plugin/plugin.json"]).hexdigest(),
    }
    return expected_url, digests


def _identity(metadata: os.stat_result) -> tuple[int, int, int, int, int]:
    return metadata.st_dev, metadata.st_ino, metadata.st_size, metadata.st_mtime_ns, metadata.st_ctime_ns


def _directory_identity(metadata: os.stat_result) -> tuple[int, int]:
    return metadata.st_dev, metadata.st_ino


def _open_authority(path: Path) -> tuple[Path, list[int], int]:
    require(path.is_absolute(), "released CLI path must be absolute")
    absolute = path
    require(
        absolute.name not in {"", ".", ".."}
        and all(component not in {"", ".", ".."} for component in absolute.parts[1:]),
        "released CLI path is invalid",
    )
    directory_flags = os.O_RDONLY | os.O_CLOEXEC | getattr(os, "O_DIRECTORY", 0) | getattr(os, "O_NOFOLLOW", 0)
    descriptors = [os.open(absolute.anchor, directory_flags)]
    try:
        for component in absolute.parts[1:-1]:
            require(component not in {"", ".", ".."}, "released CLI path contains an unsafe component")
            descriptors.append(os.open(component, directory_flags, dir_fd=descriptors[-1]))
        leaf = os.open(absolute.name, os.O_RDONLY | os.O_CLOEXEC | getattr(os, "O_NOFOLLOW", 0), dir_fd=descriptors[-1])
        return absolute, descriptors, leaf
    except Exception:
        for descriptor in reversed(descriptors):
            os.close(descriptor)
        raise


def _revalidate_authority(
    path: Path, descriptors: list[int], identities: list[tuple[int, int]],
    leaf_identity: tuple[int, int, int, int, int],
) -> None:
    absolute = path
    require(len(descriptors) == len(identities), "released CLI ancestor authority is incomplete")
    for index, descriptor in enumerate(descriptors):
        require(_directory_identity(os.fstat(descriptor)) == identities[index], "released CLI ancestor authority changed")
        if index == 0:
            continue
        reopened = os.open(
            absolute.parts[index],
            os.O_RDONLY | os.O_CLOEXEC | getattr(os, "O_DIRECTORY", 0) | getattr(os, "O_NOFOLLOW", 0),
            dir_fd=descriptors[index - 1],
        )
        try:
            require(_directory_identity(os.fstat(reopened)) == identities[index], "released CLI ancestor pathname was replaced")
        finally:
            os.close(reopened)
    leaf = os.open(
        absolute.name, os.O_RDONLY | os.O_CLOEXEC | getattr(os, "O_NOFOLLOW", 0),
        dir_fd=descriptors[-1],
    )
    try:
        require(_identity(os.fstat(leaf)) == leaf_identity, "released CLI leaf pathname was replaced")
    finally:
        os.close(leaf)


def _digest_descriptor(descriptor: int) -> tuple[str, tuple[int, int, int, int, int]]:
    before = os.fstat(descriptor)
    require(stat.S_ISREG(before.st_mode) and before.st_nlink == 1, "released CLI binary must be a one-link regular file")
    require(before.st_mode & 0o111 != 0, "released CLI binary is not executable")
    require(0 < before.st_size <= MAX_BINARY_BYTES, "released CLI binary size is invalid")
    os.lseek(descriptor, 0, os.SEEK_SET)
    digest = hashlib.sha256()
    remaining = before.st_size
    while remaining:
        block = os.read(descriptor, min(1 << 20, remaining))
        require(bool(block), "released CLI binary was truncated")
        digest.update(block)
        remaining -= len(block)
    after = os.fstat(descriptor)
    require(_identity(before) == _identity(after), "released CLI binary changed while hashing")
    return "sha256:" + digest.hexdigest(), _identity(before)


def _run_authenticated_binary(descriptor: int) -> subprocess.CompletedProcess[str]:
    # Linux can execute the held image directly. Other supported development
    # hosts use a private, parent-anchored copy made only from that descriptor;
    # the original pathname is never consulted after authentication.
    if sys.platform.startswith("linux") and Path("/proc/self/fd").is_dir():
        executable = f"/proc/self/fd/{descriptor}"
        return subprocess.run(
            [executable, "version"], executable=executable, pass_fds=(descriptor,),
            stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
            cwd="/", env={"HOME": "/nonexistent", "PATH": "/usr/bin:/bin"},
            timeout=10, check=False,
        )
    import tempfile
    with tempfile.TemporaryDirectory(prefix="uap-authenticated-cli-") as temporary:
        image = Path(temporary) / "agentplugins"
        output = os.open(image, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC, 0o500)
        try:
            os.lseek(descriptor, 0, os.SEEK_SET)
            while True:
                block = os.read(descriptor, 1 << 20)
                if not block:
                    break
                written = 0
                while written < len(block):
                    count = os.write(output, block[written:])
                    require(count > 0, "short write while sealing authenticated CLI image")
                    written += count
            os.fsync(output)
        finally:
            os.close(output)
        return subprocess.run(
            [str(image), "version"], stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
            cwd=temporary, env={"HOME": "/nonexistent", "PATH": "/usr/bin:/bin"},
            timeout=10, check=False,
        )


def validate_binary(path: Path, installer_version: str, approved_digest: str) -> str:
    try:
        authority_path, ancestors, descriptor = _open_authority(path)
    except OSError as error:
        raise PublicationError(f"released CLI path is unsafe: {error}") from error
    try:
        ancestor_identities = [_directory_identity(os.fstat(item)) for item in ancestors]
        digest, identity = _digest_descriptor(descriptor)
        require(
            digest == approved_digest,
            "released CLI binary is not the exact approved Linux/amd64 asset",
        )
        try:
            _revalidate_authority(authority_path, ancestors, ancestor_identities, identity)
        except OSError as error:
            raise PublicationError(f"released CLI path changed before version verification: {error}") from error
        try:
            completed = _run_authenticated_binary(descriptor)
        except (OSError, subprocess.SubprocessError) as error:
            raise PublicationError(f"released CLI version check failed: {error}") from error
        try:
            _revalidate_authority(authority_path, ancestors, ancestor_identities, identity)
        except OSError as error:
            raise PublicationError(f"released CLI path changed during version verification: {error}") from error
        after_digest, after_identity = _digest_descriptor(descriptor)
        require((after_digest, after_identity) == (digest, identity), "released CLI binary changed during version verification")
        require(
            completed.returncode == 0
            and completed.stdout == f"agentplugins {installer_version}\n"
            and completed.stderr == "",
            "released CLI version output differs from the requested installer version",
        )
        return digest
    finally:
        os.close(descriptor)
        for ancestor in reversed(ancestors):
            os.close(ancestor)


def _rename_noreplace(parent_descriptor: int, source: str, destination: str) -> None:
    libc = ctypes.CDLL(None, use_errno=True)
    encoded_source = os.fsencode(source)
    encoded_destination = os.fsencode(destination)
    if sys.platform.startswith("linux") and hasattr(libc, "renameat2"):
        result = libc.renameat2(
            ctypes.c_int(parent_descriptor), ctypes.c_char_p(encoded_source),
            ctypes.c_int(parent_descriptor), ctypes.c_char_p(encoded_destination), ctypes.c_uint(1),
        )
    elif sys.platform == "darwin" and hasattr(libc, "renameatx_np"):
        result = libc.renameatx_np(
            ctypes.c_int(parent_descriptor), ctypes.c_char_p(encoded_source),
            ctypes.c_int(parent_descriptor), ctypes.c_char_p(encoded_destination), ctypes.c_uint(0x00000004),
        )
    else:
        raise PublicationError("atomic no-replace directory publication is unsupported on this platform")
    if result != 0:
        error = ctypes.get_errno()
        if error in {errno.EEXIST, errno.ENOTEMPTY}:
            raise PublicationError("output directory already exists; refusing to overwrite approved inputs")
        raise OSError(error, os.strerror(error), destination)


def _quarantine_owned_stage(
    parent_descriptor: int,
    expected_identity: tuple[int, int],
    preferred_name: str | None = None,
    quarantine_prefix: str = ".rejected-stage-",
) -> str | None:
    """Preserve the exact owned stage; never pathname-delete during error recovery."""
    if preferred_name is not None:
        quarantine = f"{quarantine_prefix}{secrets.token_hex(12)}"
        try:
            _rename_noreplace(parent_descriptor, preferred_name, quarantine)
        except (OSError, PublicationError):
            pass
        else:
            # Preserve a substituted foreign inode too. If it is not ours, the
            # bounded scan below may still locate the exact owned stage.
            if _path_identity(parent_descriptor, quarantine) == expected_identity:
                return quarantine
    for _ in range(4):
        candidates: list[str] = []
        with os.scandir(parent_descriptor) as iterator:
            for item in iterator:
                require(len(candidates) < 4096, "output parent has too many entries for bounded stage recovery")
                candidates.append(item.name)
        for candidate in candidates:
            try:
                metadata = os.stat(candidate, dir_fd=parent_descriptor, follow_symlinks=False)
            except OSError:
                continue
            if not stat.S_ISDIR(metadata.st_mode) or _directory_identity(metadata) != expected_identity:
                continue
            quarantine = f"{quarantine_prefix}{secrets.token_hex(12)}"
            try:
                _rename_noreplace(parent_descriptor, candidate, quarantine)
            except OSError:
                continue
            # A same-UID racer may substitute candidate between stat and rename.
            # Preserve that foreign inode too, then continue looking for ours.
            if _path_identity(parent_descriptor, quarantine) == expected_identity:
                return quarantine
    return None


def _quarantine_unidentified_stage(parent_descriptor: int, preferred_name: str) -> str | None:
    """Preserve the create-once stage when its initial stat could not be read."""
    quarantine = f".rejected-stage-{secrets.token_hex(12)}"
    try:
        _rename_noreplace(parent_descriptor, preferred_name, quarantine)
    except (OSError, PublicationError):
        return None
    return quarantine


def _stage_path_identity(parent_descriptor: int, stage_name: str) -> tuple[int, int]:
    metadata = os.stat(stage_name, dir_fd=parent_descriptor, follow_symlinks=False)
    require(stat.S_ISDIR(metadata.st_mode), "publication stage pathname is not a directory")
    return _directory_identity(metadata)


def _path_identity(parent_descriptor: int, name: str) -> tuple[int, int]:
    return _directory_identity(os.stat(name, dir_fd=parent_descriptor, follow_symlinks=False))


def _publication_directory_seal(
    descriptor: int, expected_bodies: dict[str, bytes],
) -> tuple[tuple[str, tuple[int, ...], str], ...]:
    names: list[str] = []
    with os.scandir(descriptor) as iterator:
        for item in iterator:
            require(len(names) < len(expected_bodies), "publication directory has unexpected members")
            names.append(item.name)
    require(set(names) == set(expected_bodies), "publication directory member set changed")
    seal: list[tuple[str, tuple[int, ...], str]] = []
    for name in sorted(names):
        metadata = os.stat(name, dir_fd=descriptor, follow_symlinks=False)
        expected = expected_bodies[name]
        require(
            stat.S_ISREG(metadata.st_mode)
            and metadata.st_nlink == 1
            and stat.S_IMODE(metadata.st_mode) == 0o640
            and metadata.st_size == len(expected),
            f"publication member {name!r} has unsafe metadata",
        )
        member = os.open(
            name,
            os.O_RDONLY | os.O_CLOEXEC | getattr(os, "O_NOFOLLOW", 0),
            dir_fd=descriptor,
        )
        try:
            opened = os.fstat(member)
            identity = _snapshot_identity(opened)
            require(identity == _snapshot_identity(metadata), f"publication member {name!r} changed while opening")
            body = _read_snapshot_file(member, opened.st_size, f"publication member {name!r}")
            require(body == expected, f"publication member {name!r} differs from generated bytes")
            require(_snapshot_identity(os.fstat(member)) == identity, f"publication member {name!r} changed while reading")
            seal.append((name, identity, hashlib.sha256(body).hexdigest()))
        finally:
            os.close(member)
    return tuple(seal)


def _reject_published_output(
    parent_descriptor: int, output_name: str, expected_identity: tuple[int, int],
) -> str | None:
    """Preserve substitutions and quarantine the exact owned publication when found."""
    for _ in range(4):
        try:
            observed_identity = _path_identity(parent_descriptor, output_name)
        except FileNotFoundError:
            break
        prefix = f".rejected-{output_name}." if observed_identity == expected_identity else f".foreign-{output_name}."
        quarantine = f"{prefix}{secrets.token_hex(12)}"
        try:
            _rename_noreplace(parent_descriptor, output_name, quarantine)
        except (OSError, PublicationError):
            continue
        # The move is the authority boundary: a racer may have substituted a
        # different inode after our stat. Preserve it and keep looking for ours.
        if _path_identity(parent_descriptor, quarantine) == expected_identity:
            return quarantine
    return _quarantine_owned_stage(
        parent_descriptor,
        expected_identity,
        quarantine_prefix=f".rejected-{output_name}.",
    )


def _require_disjoint_authorities(
    candidate_path: Path, candidate_identities: list[tuple[int, int]],
    protected_path: Path, protected_identities: list[tuple[int, int]],
) -> None:
    common_components = 0
    for candidate_component, protected_component in zip(candidate_path.parts, protected_path.parts):
        if candidate_component != protected_component:
            break
        common_components += 1
    require(
        set(candidate_identities[common_components:]).isdisjoint(protected_identities[common_components:]),
        "output directory aliases an authenticated input root",
    )


def validate_output_authority(output: Path, protected_roots: tuple[Path, ...]) -> None:
    """Reject overlap and symlinked existing ancestors before any output mutation."""
    require(output.is_absolute() and output.name not in {"", ".", ".."}, "output directory must be an absolute non-root path")
    require(all(part not in {"", ".", ".."} for part in output.parts[1:]), "output directory contains an unsafe component")
    flags = os.O_RDONLY | os.O_CLOEXEC | getattr(os, "O_DIRECTORY", 0) | getattr(os, "O_NOFOLLOW", 0)

    def ancestry(path: Path, *, allow_missing: bool, label: str) -> list[tuple[int, int]]:
        descriptor = os.open(path.anchor, flags)
        identities = [_directory_identity(os.fstat(descriptor))]
        try:
            for component in path.parts[1:]:
                try:
                    child = os.open(component, flags, dir_fd=descriptor)
                except FileNotFoundError:
                    if allow_missing:
                        return identities
                    raise PublicationError(f"{label} is missing")
                except OSError as error:
                    raise PublicationError(f"{label} has a symlink or non-directory ancestor: {error}") from error
                os.close(descriptor)
                descriptor = child
                identities.append(_directory_identity(os.fstat(descriptor)))
            return identities
        finally:
            os.close(descriptor)

    output_identities = ancestry(output, allow_missing=True, label="output path")
    for protected in protected_roots:
        require(protected.is_absolute(), "protected input root must be absolute")
        common = Path(os.path.commonpath((str(output), str(protected))))
        require(
            common not in {output, protected},
            "output directory overlaps an authenticated input root",
        )
        protected_identities = ancestry(protected, allow_missing=False, label="protected input root")
        _require_disjoint_authorities(output, output_identities, protected, protected_identities)


def _open_or_create_output_parent(
    output: Path,
    protected_authorities: tuple[tuple[Path, list[tuple[int, int]]], ...],
) -> int:
    """Create missing parent components without ever following a symlink."""
    flags = os.O_RDONLY | os.O_CLOEXEC | getattr(os, "O_DIRECTORY", 0) | getattr(os, "O_NOFOLLOW", 0)
    descriptor = os.open(output.anchor, flags)
    identities = [_directory_identity(os.fstat(descriptor))]

    def check() -> None:
        for protected_path, protected_identities in protected_authorities:
            _require_disjoint_authorities(output.parent, identities, protected_path, protected_identities)

    try:
        for component in output.parts[1:-1]:
            try:
                child = os.open(component, flags, dir_fd=descriptor)
            except FileNotFoundError:
                check()
                os.mkdir(component, mode=0o755, dir_fd=descriptor)
                child = os.open(component, flags, dir_fd=descriptor)
            os.close(descriptor)
            descriptor = child
            identities.append(_directory_identity(os.fstat(descriptor)))
        check()
        result = descriptor
        descriptor = -1
        return result
    except OSError as error:
        raise PublicationError(f"output parent changed or contains a symlink: {error}") from error
    finally:
        if descriptor >= 0:
            os.close(descriptor)


def _revalidate_output_parent(parent: Path, identity: tuple[int, int]) -> None:
    try:
        _, descriptors = _open_absolute_directory(parent, "output parent")
    except OSError as error:
        raise PublicationError(f"output parent pathname changed: {error}") from error
    try:
        require(_directory_identity(os.fstat(descriptors[-1])) == identity, "output parent pathname was replaced")
    finally:
        for descriptor in reversed(descriptors):
            os.close(descriptor)


def _revalidate_protected_tree(expected: ProtectedTree) -> None:
    _, _, observed = _stable_tree_snapshot(
        expected.authority.path, "authenticated input root",
        package_contract=expected.package_contract,
    )
    require(
        observed.authority == expected.authority and observed.seal == expected.seal,
        "authenticated input tree changed after validation",
    )


def _publication_preflight_hook() -> None:
    """Test seam before the private publication closure; never authorizes writes."""


def generate(args: argparse.Namespace) -> tuple[dict[str, Any], dict[str, Any]]:
    app, receipt, protected_trees = _build(args)
    _publication_preflight_hook()
    output = args.output

    def write_outputs() -> None:
        require(len(protected_trees) == 2, "validated publication has incomplete protected trees")
        held_protected: list[list[int]] = []
        protected_authorities: list[tuple[Path, list[tuple[int, int]]]] = []
        try:
            for expected_tree in protected_trees:
                expected = expected_tree.authority
                protected = expected.path
                try:
                    _, descriptors = _open_absolute_directory(protected, "protected input root")
                except OSError as error:
                    raise PublicationError(f"protected input root is not a real directory: {error}") from error
                held_protected.append(descriptors)
                actual_identities = [_directory_identity(os.fstat(descriptor)) for descriptor in descriptors]
                require(
                    actual_identities == list(expected.identities),
                    "authenticated input root pathname changed between validation and publication",
                )
                protected_authorities.append((protected, actual_identities))

            for expected_tree in protected_trees:
                _revalidate_protected_tree(expected_tree)

            validate_output_authority(output, tuple(item.authority.path for item in protected_trees))
            require(output.name not in {"", ".", ".."}, "output directory name is invalid")
            parent = output.parent
            parent_descriptor = _open_or_create_output_parent(output, tuple(protected_authorities))
            try:
                parent_identity = _directory_identity(os.fstat(parent_descriptor))
                for protected_path, expected_identities in protected_authorities:
                    try:
                        _, reopened = _open_absolute_directory(protected_path, "protected input root")
                    except OSError as error:
                        raise PublicationError(f"protected input root pathname changed before publication: {error}") from error
                    try:
                        require(
                            [_directory_identity(os.fstat(descriptor)) for descriptor in reopened] == expected_identities,
                            "protected input root pathname changed before publication",
                        )
                    finally:
                        for descriptor in reversed(reopened):
                            os.close(descriptor)

                stage_name = f".{output.name}.{os.getpid()}.{secrets.token_hex(8)}.tmp"
                stage_created = False
                stage_identity: tuple[int, int] | None = None
                stage_descriptor = -1
                preserve_rejected_output = False
                try:
                    os.mkdir(stage_name, mode=0o700, dir_fd=parent_descriptor)
                    stage_created = True
                    stage_identity = _stage_path_identity(parent_descriptor, stage_name)
                    stage_descriptor = os.open(
                        stage_name,
                        os.O_RDONLY | os.O_CLOEXEC | getattr(os, "O_DIRECTORY", 0) | getattr(os, "O_NOFOLLOW", 0),
                        dir_fd=parent_descriptor,
                    )
                    require(
                        _directory_identity(os.fstat(stage_descriptor)) == stage_identity,
                        "publication stage changed before descriptor open",
                    )
                    publication_bodies = {
                        "app-binding.json": canonical_output(app),
                        "projection-receipt.json": canonical_output(receipt),
                    }
                    for name, body in publication_bodies.items():
                        descriptor = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC, 0o640, dir_fd=stage_descriptor)
                        try:
                            written = 0
                            while written < len(body):
                                count = os.write(descriptor, body[written:])
                                require(count > 0, f"short write while publishing {name}")
                                written += count
                            os.fsync(descriptor)
                        finally:
                            os.close(descriptor)
                    os.fsync(stage_descriptor)
                    for expected_tree in protected_trees:
                        _revalidate_protected_tree(expected_tree)
                    require(_directory_identity(os.fstat(parent_descriptor)) == parent_identity, "output parent authority changed before publication")
                    _revalidate_output_parent(parent, parent_identity)
                    require(
                        _stage_path_identity(parent_descriptor, stage_name) == stage_identity,
                        "publication stage pathname was replaced before rename",
                    )
                    prepublication_seal = _publication_directory_seal(stage_descriptor, publication_bodies)
                    _rename_noreplace(parent_descriptor, stage_name, output.name)
                    try:
                        published_identity = _stage_path_identity(parent_descriptor, output.name)
                    except Exception as error:
                        preserve_rejected_output = True
                        try:
                            _reject_published_output(parent_descriptor, output.name, stage_identity)
                        except (OSError, PublicationError):
                            pass
                        raise PublicationError("publication stage pathname was replaced during rename") from error
                    if published_identity != stage_identity:
                        try:
                            _reject_published_output(parent_descriptor, output.name, stage_identity)
                        except (OSError, PublicationError):
                            pass
                        preserve_rejected_output = True
                        raise PublicationError("publication stage pathname was replaced during rename")
                    try:
                        for expected_tree in protected_trees:
                            _revalidate_protected_tree(expected_tree)
                    except Exception:
                        preserve_rejected_output = True
                        try:
                            _reject_published_output(parent_descriptor, output.name, stage_identity)
                        except (OSError, PublicationError):
                            pass
                        try:
                            os.fsync(parent_descriptor)
                        except OSError:
                            pass
                        raise
                    os.fsync(parent_descriptor)
                    require(_directory_identity(os.fstat(parent_descriptor)) == parent_identity, "output parent authority changed during publication")
                    _revalidate_output_parent(parent, parent_identity)
                    try:
                        final_identity = _stage_path_identity(parent_descriptor, output.name)
                    except Exception as error:
                        preserve_rejected_output = True
                        try:
                            _reject_published_output(parent_descriptor, output.name, stage_identity)
                        except (OSError, PublicationError):
                            pass
                        raise PublicationError("published output pathname was replaced during finalization") from error
                    if final_identity != stage_identity:
                        try:
                            _reject_published_output(parent_descriptor, output.name, stage_identity)
                        except (OSError, PublicationError):
                            pass
                        preserve_rejected_output = True
                        raise PublicationError("published output pathname was replaced during finalization")
                    try:
                        require(
                            _publication_directory_seal(stage_descriptor, publication_bodies) == prepublication_seal,
                            "published output members changed during publication",
                        )
                        require(
                            _stage_path_identity(parent_descriptor, output.name) == stage_identity,
                            "published output pathname was replaced after content validation",
                        )
                    except Exception:
                        preserve_rejected_output = True
                        try:
                            _reject_published_output(parent_descriptor, output.name, stage_identity)
                        except (OSError, PublicationError):
                            pass
                        raise
                except Exception:
                    if stage_created and not preserve_rejected_output:
                        try:
                            if stage_identity is None:
                                _quarantine_unidentified_stage(parent_descriptor, stage_name)
                            else:
                                _quarantine_owned_stage(parent_descriptor, stage_identity, stage_name)
                        except (OSError, PublicationError):
                            # Preserve the primary failure. Recovery never deletes by
                            # pathname; an unlocated owned inode remains audit residue.
                            pass
                    raise
                finally:
                    if stage_descriptor >= 0:
                        os.close(stage_descriptor)
            finally:
                os.close(parent_descriptor)
        finally:
            for descriptors in reversed(held_protected):
                for descriptor in reversed(descriptors):
                    os.close(descriptor)
    write_outputs()
    return app, receipt


def _build(
    args: argparse.Namespace,
) -> tuple[dict[str, Any], dict[str, Any], tuple[ProtectedTree, ProtectedTree]]:
    validate_output_authority(args.output, (args.package_root, args.projection_root))
    require(NAME.fullmatch(args.product_id) is not None, "product ID is invalid")
    require(NAME.fullmatch(args.app_key) is not None, "app key is invalid")
    require(APP_ID.fullmatch(args.app_id) is not None, "app ID is invalid")
    require(isinstance(args.release_sequence, int) and not isinstance(args.release_sequence, bool) and 1 <= args.release_sequence <= 9007199254740991, "release sequence is invalid")
    require(isinstance(args.minimum_sequence, int) and not isinstance(args.minimum_sequence, bool) and 1 <= args.minimum_sequence <= 9007199254740991, "minimum sequence is invalid")
    require(
        args.installer_version == CURRENT_INSTALLER_VERSION,
        f"generator requires agentplugins {CURRENT_INSTALLER_VERSION}",
    )
    require(TIMESTAMP.fullmatch(args.observed_at) is not None, "observed-at must be canonical UTC seconds")
    observed = parse_timestamp(args.observed_at, "observed-at")
    require(observed.tzinfo == timezone.utc, "observed-at must be UTC")
    require(args.now == args.observed_at, "now and observed-at must identify the same deterministic observation")
    snapshot, snapshot_digest = verified_snapshot(args.feed, args.trusted_keys, args.now, args.minimum_sequence)
    distribution, release, binding = selected_identity(
        snapshot, product_id=args.product_id, distribution_id=args.distribution_id,
        release_sequence=args.release_sequence, installer_version=args.installer_version,
        app_key=args.app_key, app_id=args.app_id,
    )
    add, _ = load_object(args.add_evidence, MAX_EVIDENCE_BYTES, "CLI add evidence")
    installation_id, physical_artifact_id = validate_add(
        add, product_id=args.product_id, distribution=distribution, release=release,
        snapshot=snapshot, snapshot_digest=snapshot_digest, binding=binding,
    )
    state_body = read_regular_bounded(args.state, MAX_EVIDENCE_BYTES, "released CLI state")
    state = strict_state_json_loads(state_body)
    require(isinstance(state, dict), "released CLI state must be an object")
    expected_mcp, approved_mcp_url, expected_manifest, expected_files, package_authority = authenticated_package_projection(
        args.package_root, release, binding, args.product_id, physical_artifact_id,
    )
    managed_digest, projection_files, projection_directories, projection_authority = _projection_artifact_snapshot_details(args.projection_root)
    validate_state(
        state, args.projection_root, product_id=args.product_id, distribution=distribution,
        release=release, snapshot=snapshot, snapshot_digest=snapshot_digest,
        binding=binding, installation_id=installation_id, physical_artifact_id=physical_artifact_id,
        managed_digest=managed_digest,
    )
    mcp_url, projection_digests = validate_projection(
        projection_files, binding, args.product_id, expected_mcp, approved_mcp_url,
        expected_manifest, expected_files, projection_directories,
    )
    binary_digest = validate_binary(
        args.cli_binary,
        args.installer_version,
        CURRENT_LINUX_AMD64_DIGEST,
    )
    endpoint_host = urlsplit(mcp_url).hostname
    require(endpoint_host is not None, "ChatGPT MCP endpoint has no host")
    source = release["package_source"]
    receipt = {
        "application_id": args.app_id,
        "product_id": args.product_id,
        "projection": {
            "app_json_digest": projection_digests["app_digest"],
            "codex_manifest_digest": projection_digests["manifest_digest"],
            "managed_digest": managed_digest,
            "mcp_json_digest": projection_digests["mcp_digest"],
            "mcp_url": mcp_url,
        },
        "tuple": {
            "adapter_version": args.installer_version,
            "architecture": "amd64",
            "binary_digest": binary_digest,
            "client_version": None,
            "dependency_identity": f"remote-mcp:{endpoint_host}",
            "distribution_id": distribution["id"],
            "distribution_kind": distribution["kind"],
            "installer_version": args.installer_version,
            "manifest_digest": release["manifest_digest"],
            "observed_at": args.observed_at,
            "os": "linux",
            "package_version": release["package_version"],
            "product_id": args.product_id,
            "release_sequence": release["sequence"],
            "snapshot_digest": snapshot_digest,
            "snapshot_sequence": snapshot["sequence"],
            "source_path": source["path"],
            "source_repository": source["repository"],
            "source_revision": source["revision"],
            "tree_digest": release["tree_digest"],
        },
    }
    app = {"apps": {args.app_key: {"id": args.app_id}}}
    return app, receipt, (package_authority, projection_authority)


def parser() -> argparse.ArgumentParser:
    value = argparse.ArgumentParser(description=__doc__)
    value.add_argument("--feed", type=Path, required=True)
    value.add_argument("--trusted-keys", type=Path, required=True)
    value.add_argument("--now", required=True)
    value.add_argument("--minimum-sequence", type=int, default=1)
    value.add_argument("--add-evidence", type=Path, required=True)
    value.add_argument("--state", type=Path, required=True)
    value.add_argument("--package-root", type=Path, required=True)
    value.add_argument("--projection-root", type=Path, required=True)
    value.add_argument("--cli-binary", type=Path, required=True)
    value.add_argument("--installer-version", required=True)
    value.add_argument("--product-id", required=True)
    value.add_argument("--distribution-id", required=True)
    value.add_argument("--release-sequence", type=int, required=True)
    value.add_argument("--app-key", required=True)
    value.add_argument("--app-id", required=True)
    value.add_argument("--observed-at", required=True)
    value.add_argument("--output", type=Path, required=True)
    return value


def main() -> int:
    args = parser().parse_args()
    try:
        generate(args)
        print(f"wrote canonical ChatGPT observer projection to {args.output}")
        return 0
    except (OSError, ValueError, PublicationError, KeyError, TypeError) as error:
        print(f"build-chatgpt-observer-projection: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
