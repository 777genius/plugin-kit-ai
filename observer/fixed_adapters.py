#!/usr/bin/env python3
"""Fixed protected observation adapters; no request-selected executable or argv."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import re
import selectors
import shutil
import signal
import stat
import subprocess
import tempfile
import time
import urllib.request
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from observer.client_bundle import verify_bundle

HEROES = {"agent-code-navigator", "context7", "cloudflare-docs", "chrome-devtools", "notion"}
CLIENTS = {"codex", "cursor", "kiro"}
ARTIFACT_MODE = {
    "runtime-attestations.json": "runtime",
    "notion-oauth-attestations.json": "notion-oauth",
    "chatgpt-cloudflare-attestation.json": "chatgpt-cloudflare",
    "consent.json": "consent",
}
ENTRYPOINT_ARTIFACT = {
    "uap-observer-adapter-runtime": "runtime-attestations.json",
    "uap-observer-adapter-notion": "notion-oauth-attestations.json",
    "uap-observer-adapter-chatgpt": "chatgpt-cloudflare-attestation.json",
    "uap-observer-adapter-consent": "consent.json",
}
CONFIG_PATH = Path("/opt/uap-observer-current/etc/uap-observer-adapter-config.json")
CONSENT_DIRECTORY = Path("/var/lib/uap-observer-consent/pending")
GIT_BINARY = Path("/opt/uap-observer-inputs/bin/git")
FIXED_INPUT_PATHS = {
    str(GIT_BINARY),
    "/opt/uap-observer-inputs/bin/codex",
    "/opt/uap-observer-inputs/cursor",
    "/opt/uap-observer-inputs/cursor/cursor-agent",
    "/opt/uap-observer-inputs/cursor-bundle.json",
    "/opt/uap-observer-inputs/bin/kiro",
    "/opt/uap-observer-inputs/bin/kiro-cli-chat",
    "/opt/uap-observer-inputs/chatgpt/app-binding.json",
    "/opt/uap-observer-inputs/chatgpt/projection-receipt.json",
    "/opt/uap-observer-inputs/external-pr-evidence.json",
}
FIXED_MOUNT_PATHS = FIXED_INPUT_PATHS - {
    "/opt/uap-observer-inputs/cursor/cursor-agent",
}
PRIVACY_RESULT = {
    "real_project_accessed": False, "absolute_paths_exported": False,
    "credential_material_exported": False, "auth_copied": False,
    "enforcement": "systemd-positive-mount-allowlist-v1",
}
MAX_FILE = 4 << 20
MAX_STDOUT = 1 << 20
MAX_SSE_RECORDS = 64
MAX_SSE_LINE = 256 << 10
KILL_WAIT_SECONDS = 2.0
COMMAND_SECONDS = 45
HUMAN_WAIT_SECONDS = 300
FIXED_HTTPS_PROXY = "http://127.0.0.2:8766"
MCP_ENDPOINT = "https://docs.mcp.cloudflare.com/mcp"
MCP_MARKER = "cloudflare-docs-read-only-v1"
MCP_READ_TOOL = "search_cloudflare_documentation"
MCP_READ_ARGUMENTS = {"query": "Cloudflare Durable Objects SQLite storage API marker cloudflare-docs-read-only-v1"}
TUPLE_FIELDS = {
    "product_id", "tree_digest", "manifest_digest", "distribution_id", "distribution_kind",
    "release_sequence", "package_version", "source_repository", "source_revision", "source_path",
    "snapshot_sequence", "snapshot_digest", "binary_digest", "dependency_identity", "installer_version",
    "adapter_version", "client_version", "os", "architecture", "observed_at",
}
TUPLE_DIGEST_FIELDS = {"tree_digest", "manifest_digest", "snapshot_digest", "binary_digest"}
TUPLE_SEQUENCE_FIELDS = {"release_sequence", "snapshot_sequence"}
CLIENT_ARGUMENTS = {
    "codex": ("exec", "--skip-git-repo-check", "--json", "--ephemeral", "--sandbox", "read-only"),
    "cursor": ("--print", "--output-format", "stream-json", "--mode", "ask", "--force", "--sandbox", "enabled", "--trust", "--approve-mcps"),
    "kiro": ("acp", "--agent-engine", "v3", "--auth-method", "cli"),
}
CLIENT_DISCOVERY_ARGUMENTS = {
    "codex": ("mcp", "list", "--json"),
    "cursor": ("mcp", "list"),
    "kiro": ("mcp", "list"),
}


def validate_release_tuple(value: Any, plugin: str, *, sealed: bool = True) -> dict[str, Any]:
    """Validate the one exact tuple shape shared by sealing and runtime output."""
    if not isinstance(value, dict) or set(value) != TUPLE_FIELDS or value.get("product_id") != plugin:
        raise ValueError("fixed client tuple is incomplete")
    if any(type(value.get(field)) is not int or value[field] < 1 for field in TUPLE_SEQUENCE_FIELDS):
        raise ValueError("fixed client tuple sequence is invalid")
    strings = TUPLE_FIELDS - TUPLE_SEQUENCE_FIELDS - {"client_version"}
    if any(type(value.get(field)) is not str or not value[field] for field in strings):
        raise ValueError("fixed client tuple identifier is invalid")
    if (sealed and value.get("client_version") is not None) or (
        not sealed and (type(value.get("client_version")) is not str or not value["client_version"])
    ):
        raise ValueError("fixed client tuple version state is invalid")
    if (
        re.fullmatch(r"[a-f0-9]{40}", value["source_revision"]) is None
        or any(re.fullmatch(r"sha256:[a-f0-9]{64}", value[field]) is None for field in TUPLE_DIGEST_FIELDS)
        or value["source_repository"].startswith("/") or "//" in value["source_repository"]
        or Path(value["source_path"]).is_absolute() or ".." in Path(value["source_path"]).parts
    ):
        raise ValueError("fixed client tuple provenance is invalid")
    return value
PROBE_TOOLS = {
    "agent-code-navigator": "search_code",
    "context7": "resolve-library-id",
    "cloudflare-docs": "search_cloudflare_documentation",
    "chrome-devtools": "list_pages",
    "notion": "search",
}
KIRO_PROTOCOL_VERSION = 1
KIRO_CLI_SHA256 = "sha256:adab7305f27302bb4da93590ecb6d6ac49b9cad6d7f4cd17010735358cf32336"
KIRO_CHAT_SHA256 = "sha256:c8c4edf122e66b07cc96729823ffa04d6f9a4dfd887590d36b76f809fce039c4"
KIRO_MAX_LINE = 256 << 10
KIRO_MAX_OUTPUT = 1 << 20
KIRO_MAX_TOOLS = 64
KIRO_MAX_TOOL_NAME = 256
KIRO_TOOL_NAME = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.:/-]{0,255}")


def canonical_json(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()


def sha256(value: bytes) -> str:
    return "sha256:" + hashlib.sha256(value).hexdigest()


def exported_json(value: Any) -> bytes:
    return (json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode()


def strict_json_loads(encoded: bytes | str) -> Any:
    """Apply the observer's fail-closed JSON evidence decoding policy."""
    def object_from_pairs(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
        value: dict[str, Any] = {}
        folded: set[str] = set()
        for key, child in pairs:
            normalized = key.casefold()
            if key in value or normalized in folded:
                raise ValueError("duplicate or case-confusable JSON object member")
            value[key] = child
            folded.add(normalized)
        return value

    def reject_constant(value: str) -> Any:
        raise ValueError(f"non-finite JSON number: {value}")

    def finite_float(value: str) -> float:
        decoded = float(value)
        if not math.isfinite(decoded):
            raise ValueError(f"non-finite JSON number: {value}")
        return decoded

    return json.loads(
        encoded, object_pairs_hook=object_from_pairs,
        parse_constant=reject_constant, parse_float=finite_float,
    )


def open_directory(path: Path, *, allowed_owners: set[int]) -> int:
    if not path.is_absolute() or ".." in path.parts:
        raise ValueError("protected directory path is invalid")
    path_flags = getattr(os, "O_PATH", os.O_RDONLY) | os.O_DIRECTORY | os.O_CLOEXEC
    descriptor = os.open("/", path_flags)
    try:
        for component in path.parts[1:]:
            child = os.open(component, path_flags | os.O_NOFOLLOW, dir_fd=descriptor)
            os.close(descriptor)
            descriptor = child
            info = os.fstat(descriptor)
            sticky_root = info.st_uid == 0 and bool(info.st_mode & stat.S_ISVTX)
            if not stat.S_ISDIR(info.st_mode) or info.st_uid not in allowed_owners | {0}:
                raise ValueError("protected directory component is not trusted")
            if info.st_mode & 0o022 and not sticky_root:
                raise ValueError("protected directory component is writable")
        return descriptor
    except Exception:
        os.close(descriptor)
        raise


def verify_root_readonly_directory(path: Path, *, label: str) -> int:
    """Open a root-controlled 0510 boundary without requiring directory read."""
    descriptor = open_directory(path, allowed_owners={0})
    info = os.fstat(descriptor)
    authorized_groups = {os.getegid(), *os.getgroups()}
    if (
        info.st_uid != 0 or info.st_gid not in authorized_groups
        or stat.S_IMODE(info.st_mode) != 0o510
    ):
        os.close(descriptor)
        raise ValueError(f"{label} is not the exact root/group read-only boundary")
    return descriptor


def verify_root_readonly_ancestors(profile: Path, parent: Path) -> None:
    """Validate every protected active-config ancestor through O_PATH handles."""
    try:
        relative = parent.relative_to(profile)
    except ValueError as error:
        raise ValueError("active native config escapes its protected profile") from error
    descriptor = verify_root_readonly_directory(profile, label="fixed client profile")
    path_flags = getattr(os, "O_PATH", os.O_RDONLY) | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC
    try:
        for component in relative.parts:
            child = os.open(component, path_flags, dir_fd=descriptor)
            os.close(descriptor)
            descriptor = child
            info = os.fstat(descriptor)
            if (
                info.st_uid != 0 or info.st_gid not in {os.getegid(), *os.getgroups()}
                or stat.S_IMODE(info.st_mode) != 0o510
            ):
                raise ValueError("active native config ancestor is not the exact root/group read-only boundary")
    finally:
        os.close(descriptor)


def read_regular(path: Path, expected_digest: str | None, *, owner_uid: int, mode: int | None = None) -> bytes:
    parent_fd = open_directory(path.parent, allowed_owners={owner_uid, os.geteuid()})
    descriptor = -1
    try:
        descriptor = os.open(path.name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC, dir_fd=parent_fd)
        info = os.fstat(descriptor)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != owner_uid or info.st_mode & 0o022 or info.st_nlink != 1:
            raise ValueError("protected file is not trusted")
        if mode is not None and stat.S_IMODE(info.st_mode) != mode:
            raise ValueError("protected file mode is not exact")
        if mode is not None and mode & 0o070 and info.st_gid not in {os.getegid(), *os.getgroups()}:
            raise ValueError("protected file group is not authorized")
        if info.st_size > MAX_FILE:
            raise ValueError("protected file exceeds size bound")
        chunks, remaining = [], MAX_FILE + 1
        while remaining:
            chunk = os.read(descriptor, remaining)
            if not chunk:
                break
            chunks.append(chunk)
            remaining -= len(chunk)
        encoded = b"".join(chunks)
        after = os.fstat(descriptor)
        if (after.st_dev, after.st_ino, after.st_size, after.st_mode, after.st_uid, after.st_gid, after.st_nlink, after.st_mtime_ns, after.st_ctime_ns) != (
            info.st_dev, info.st_ino, info.st_size, info.st_mode, info.st_uid, info.st_gid, info.st_nlink, info.st_mtime_ns, info.st_ctime_ns
        ):
            raise ValueError("protected file changed while being read")
        if len(encoded) > MAX_FILE or (expected_digest is not None and sha256(encoded) != expected_digest):
            raise ValueError("protected file digest differs")
        return encoded
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        os.close(parent_fd)


def regular_snapshot(path: Path, expected_digest: str | None, *, owner_uid: int, mode: int) -> dict[str, Any]:
    """Capture exact proof bytes and link metadata for later TOCTOU revalidation."""
    body = read_regular(path, expected_digest, owner_uid=owner_uid, mode=mode)
    info = os.lstat(path)
    if not stat.S_ISREG(info.st_mode) or info.st_uid != owner_uid or info.st_nlink != 1 or stat.S_IMODE(info.st_mode) != mode:
        raise ValueError("protected proof metadata differs")
    return {
        "body": body, "sha256": sha256(body), "mode": stat.S_IMODE(info.st_mode),
        "uid": info.st_uid, "gid": info.st_gid, "nlink": info.st_nlink,
        "device": info.st_dev, "inode": info.st_ino,
    }


def revalidate_snapshot(path: Path, snapshot: dict[str, Any], *, owner_uid: int, mode: int) -> None:
    current = regular_snapshot(path, snapshot["sha256"], owner_uid=owner_uid, mode=mode)
    if current != snapshot:
        raise ValueError("protected proof changed during client invocation")


def load_json(path: Path, digest: str, *, owner_uid: int, mode: int | None = None) -> dict[str, Any]:
    value = strict_json_loads(read_regular(path, digest, owner_uid=owner_uid, mode=mode))
    if not isinstance(value, dict):
        raise ValueError("protected JSON must be an object")
    return value


def verify_executable_file(path: Path, expected_digest: str, *, owner_uid: int) -> None:
    parent_fd = open_directory(path.parent, allowed_owners={owner_uid})
    descriptor = -1
    try:
        descriptor = os.open(path.name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=parent_fd)
        info = os.fstat(descriptor)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != owner_uid or info.st_mode & 0o022 or info.st_nlink != 1 or not info.st_mode & stat.S_IXUSR or info.st_size > 512 << 20:
            raise ValueError("fixed executable is not trusted")
        digest = hashlib.sha256()
        while chunk := os.read(descriptor, 1 << 20):
            digest.update(chunk)
        if "sha256:" + digest.hexdigest() != expected_digest:
            raise ValueError("fixed executable digest differs")
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        os.close(parent_fd)


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def validate_context(value: Any) -> tuple[dict[str, Any], dict[str, Any]]:
    if not isinstance(value, dict) or set(value) != {"request", "github_attestation"}:
        raise ValueError("adapter context is not canonical")
    request, github = value["request"], value["github_attestation"]
    if not isinstance(request, dict) or not isinstance(github, dict):
        raise ValueError("adapter context is invalid")
    challenge = request.get("challenge")
    if not isinstance(challenge, dict) or challenge.get("value") != github.get("challenge"):
        raise ValueError("adapter challenge is not bound")
    return request, github


def validate_config(value: dict[str, Any]) -> None:
    required = {"schema_version", "request_policy", "git", "clients", "matrix", "consent_record", "chatgpt", "workspace_root", "external_pr_evidence", "egress_hosts"}
    if set(value) != required or type(value.get("schema_version")) is not int or value.get("schema_version") != 1:
        raise ValueError("adapter config is not canonical")
    if set(value.get("clients", {})) != CLIENTS:
        raise ValueError("adapter client allowlist differs")
    if any(value["clients"][client].get("client_id") != client for client in CLIENTS):
        raise ValueError("adapter client identity differs")
    expected_binaries = {
        "codex": "/opt/uap-observer-inputs/bin/codex",
        "cursor": "/opt/uap-observer-inputs/cursor/cursor-agent",
        "kiro": "/opt/uap-observer-inputs/bin/kiro",
    }
    if any(value["clients"][client].get("binary") != expected_binaries[client] for client in CLIENTS):
        raise ValueError("adapter client binary differs from its literal dedicated path")
    cursor_bundle = value["clients"]["cursor"].get("bundle")
    if (
        not isinstance(cursor_bundle, dict)
        or set(cursor_bundle) != {"root", "manifest", "manifest_sha256"}
        or cursor_bundle.get("root") != "/opt/uap-observer-inputs/cursor"
        or cursor_bundle.get("manifest") != "/opt/uap-observer-inputs/cursor-bundle.json"
        or not re.fullmatch(r"sha256:[a-f0-9]{64}", str(cursor_bundle.get("manifest_sha256", "")))
    ):
        raise ValueError("Cursor bundle contract differs")
    kiro = value["clients"]["kiro"]
    if (
        kiro.get("sha256") != KIRO_CLI_SHA256
        or kiro.get("companion_binary") != "/opt/uap-observer-inputs/bin/kiro-cli-chat"
        or kiro.get("companion_sha256") != KIRO_CHAT_SHA256
    ):
        raise ValueError("Kiro ACP executables differ from the captured 2.19.1 closure")
    if any("companion_binary" in value["clients"][client] or "companion_sha256" in value["clients"][client] for client in {"codex", "cursor"}):
        raise ValueError("non-Kiro client has an unreviewed companion executable")
    if "bundle" in value["clients"]["codex"] or "bundle" in value["clients"]["kiro"]:
        raise ValueError("non-Cursor client has an unreviewed bundle")
    if any(value["clients"][client].get("profile") != f"/var/lib/uap-observer/profiles/{client}" for client in CLIENTS):
        raise ValueError("adapter client profile root differs")
    for client in CLIENTS:
        projection = value["clients"][client].get("native_projection")
        expected = f"/var/lib/uap-observer/proofs/{client}/native-projection.json"
        if not isinstance(projection, dict) or set(projection) != {"path", "sha256"} or projection.get("path") != expected or not re.fullmatch(r"sha256:[a-f0-9]{64}", str(projection.get("sha256", ""))):
            raise ValueError("adapter client native projection contract differs")
    egress_hosts = value.get("egress_hosts")
    fqdn = re.compile(r"(?=.{1,253}\Z)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?")
    if not isinstance(egress_hosts, list) or not egress_hosts or egress_hosts != sorted(set(egress_hosts)) or any(not isinstance(host, str) or fqdn.fullmatch(host) is None for host in egress_hosts):
        raise ValueError("adapter egress hosts must be sorted unique lowercase exact FQDNs")
    actual_paths = {
        value.get("git", {}).get("binary"),
        *(value["clients"][client].get("binary") for client in CLIENTS),
        value["clients"]["kiro"].get("companion_binary"),
        cursor_bundle.get("root"),
        cursor_bundle.get("manifest"),
        value.get("chatgpt", {}).get("app_binding_path"),
        value.get("chatgpt", {}).get("projection_receipt_path"),
        value.get("external_pr_evidence", {}).get("path"),
    }
    if actual_paths != FIXED_INPUT_PATHS:
        raise ValueError("adapter input paths differ from the literal dedicated allowlist")
    matrix = value.get("matrix")
    if not isinstance(matrix, list) or {
        (item.get("plugin"), item.get("client")) for item in matrix if isinstance(item, dict)
    } != {(hero, client) for hero in HEROES for client in CLIENTS}:
        raise ValueError("adapter hero matrix differs")
    common_fields = {"plugin", "client", "tuple", "application_id", "endpoint"}
    for item in matrix:
        if set(item) != common_fields or not isinstance(item.get("application_id"), str) or not str(item.get("endpoint", "")).startswith("https://"):
            raise ValueError("adapter matrix entry is not canonical")
        validate_release_tuple(item.get("tuple"), item["plugin"])
    root = Path(str(value.get("workspace_root")))
    if not root.is_absolute() or root != Path("/var/lib/uap-observer/workspaces"):
        raise ValueError("adapter workspace root differs")
    chat = value.get("chatgpt")
    chat_fields = {
        "app_binding_path", "app_binding_sha256", "app_id", "mcp_endpoint",
        "human_attestation_directory", "tuple", "client_version",
        "projection_receipt_path", "projection_receipt_sha256",
    }
    if not isinstance(chat, dict) or set(chat) != chat_fields:
        raise ValueError("ChatGPT adapter config is not canonical")
    validate_release_tuple(chat.get("tuple"), "cloudflare-docs")
    if chat["human_attestation_directory"] != "/var/lib/uap-observer-human/pending":
        raise ValueError("ChatGPT human attestation directory differs")
    external = value.get("external_pr_evidence")
    if not isinstance(external, dict) or set(external) != {"path", "sha256"}:
        raise ValueError("external PR evidence source is not digest-bound")


def validate_request_policy(config: dict[str, Any], request: dict[str, Any]) -> None:
    policy = config["request_policy"]
    fields = {
        "catalog_repository", "cli_release_repository", "cli_release_tag",
        "release_manifest_digest", "release_checksums_digest", "directory_digest",
        "scenario_contract_digest",
    }
    if not isinstance(policy, dict) or set(policy) != fields or any(request.get(key) != policy[key] for key in fields):
        raise ValueError("adapter request differs from digest-pinned policy")


def evidence_bindings(request: dict[str, Any]) -> dict[str, str]:
    fields = (
        "release_manifest_digest", "release_checksums_digest", "directory_digest",
        "scenario_contract_digest",
    )
    return {field: request[field] for field in fields}


def consent_record(config: dict[str, Any], request: dict[str, Any], owner_uid: int) -> dict[str, Any]:
    item = config["consent_record"]
    if item != {"directory": str(CONSENT_DIRECTORY)}:
        raise ValueError("consent record config is invalid")
    challenge = request["challenge"]["value"]
    record = load_json(CONSENT_DIRECTORY / f"{challenge}.json", None, owner_uid=owner_uid, mode=0o640)
    if record.pop("request_digest", None) != sha256(canonical_json(request)):
        raise ValueError("consent record is not bound to the complete request")
    challenge, github = request["challenge"], request["github"]
    expected = {
        "purpose": "stable-launch-e2e", "consent": True, "mode": "enforced",
        "challenge": challenge["value"], "run_id": github["run_id"],
        "run_attempt": github["run_attempt"], "catalog_sha": github["sha"],
        "scenario_contract_digest": request["scenario_contract_digest"],
        "pseudonymous_workspace_id": challenge["root_id"],
        "dedicated_identity": True, "disposable_project_status": "disposed",
        "cleanup_outcome": "cleaned", "no_real_project_proof": isolation_proof(config),
    }
    if type(record.get("schema_version")) is not int or record.get("schema_version") != 1 or any(record.get(key) != expected_value for key, expected_value in expected.items()):
        raise ValueError("consent record is not bound to this disposable run")
    if record.get("operation_mode") not in {"read-only", "synthetic"} or record.get("auth_origin") not in {"fresh-dedicated-identity", "none"}:
        raise ValueError("consent record privacy identity is invalid")
    return record


def _mount_path(value: str) -> str:
    return re.sub(
        r"\\([0-7]{3})", lambda match: chr(int(match.group(1), 8)), value,
    )


def verify_positive_mount_namespace(mountinfo: str) -> None:
    """Prove every non-kernel filesystem is mounted at an explicit runtime path."""
    if len(mountinfo) > (1 << 20):
        raise ValueError("mount namespace description exceeds size bound")
    fixed_paths = tuple(sorted(FIXED_MOUNT_PATHS))
    allowed = (
        "/opt/uap-observer-current", "/usr/bin", "/usr/lib", "/usr/lib64",
        "/lib", "/lib64",
        "/etc/passwd", "/etc/group", "/etc/nsswitch.conf", "/etc/hosts", "/etc/ssl", "/etc/pki",
        "/var/lib/uap-observer/jobs", "/var/lib/uap-observer/workspaces", "/var/lib/uap-observer/profiles", "/var/lib/uap-observer/proofs",
        "/var/lib/uap-observer-human/pending", "/var/lib/uap-observer-human/reserved", "/var/lib/uap-observer-human/consumed",
        "/var/lib/uap-observer-consent/pending", "/var/lib/uap-observer-consent/reserved", "/var/lib/uap-observer-consent/consumed",
    )
    kernel_filesystems = {
        "tmpfs", "proc", "sysfs", "cgroup2", "devtmpfs", "devpts", "mqueue",
        "hugetlbfs", "securityfs", "tracefs", "pstore", "bpf", "autofs", "ramfs",
    }
    kernel_targets = ("/proc", "/sys", "/dev", "/tmp", "/var/tmp", "/run/credentials", "/run/systemd")
    root_seen = False
    for line in mountinfo.splitlines():
        fields = line.split()
        try:
            separator = fields.index("-")
            source_root, target = _mount_path(fields[3]), _mount_path(fields[4])
            filesystem = fields[separator + 1]
        except (ValueError, IndexError):
            raise ValueError("mount namespace description is malformed") from None
        if target == "/":
            if filesystem not in {"tmpfs", "ramfs"}:
                raise ValueError("mount namespace root is not an empty synthetic filesystem")
            root_seen = True
            continue
        if filesystem in kernel_filesystems and any(target == prefix or target.startswith(prefix + "/") for prefix in kernel_targets):
            continue
        matched = next((prefix for prefix in allowed if target == prefix or target.startswith(prefix + "/")), None)
        if matched is None and target not in fixed_paths:
            raise ValueError(f"mount namespace exposes a non-allowlisted filesystem at {target}")
        if target in fixed_paths and source_root != target:
            raise ValueError(f"fixed runtime input at {target} is an alternate-path bind")
        closure_alias = matched in {"/opt/uap-observer-current", "/etc/hosts"} and source_root.startswith("/opt/uap-observer-closures/")
        same_source = matched is not None and (source_root == matched or source_root.startswith(matched + "/"))
        if target not in fixed_paths and not same_source and not closure_alias:
            raise ValueError(f"allowlisted runtime path at {target} has a foreign mount source")
    if not root_seen:
        raise ValueError("mount namespace root was not identified")


def isolation_proof(config: dict[str, Any]) -> dict[str, Any]:
    """Derive the privacy result from the kernel's effective positive allowlist."""
    if os.environ.get("UAP_OBSERVER_ISOLATION") != "systemd-positive-mount-allowlist-v1":
        raise ValueError("reviewed adapter mount isolation was not established")
    mountinfo = Path("/proc/self/mountinfo").read_text(encoding="utf-8")
    verify_positive_mount_namespace(mountinfo)
    return dict(PRIVACY_RESULT)


def verified_executable(item: dict[str, Any], owner_uid: int) -> Path:
    expected = {"binary", "sha256", "profile", "client_id", "native_projection"}
    if isinstance(item, dict) and item.get("client_id") == "cursor":
        expected |= {"bundle"}
    if isinstance(item, dict) and item.get("client_id") == "kiro":
        expected |= {"companion_binary", "companion_sha256"}
    if not isinstance(item, dict) or set(item) != expected:
        raise ValueError("fixed client config is invalid")
    binary = Path(item["binary"])
    if item["client_id"] == "cursor":
        bundle = item["bundle"]
        if (
            not isinstance(bundle, dict)
            or set(bundle) != {"root", "manifest", "manifest_sha256"}
            or bundle.get("root") != "/opt/uap-observer-inputs/cursor"
            or bundle.get("manifest") != "/opt/uap-observer-inputs/cursor-bundle.json"
        ):
            raise ValueError("fixed Cursor bundle config differs")
        files = verify_bundle(
            root=Path(bundle["root"]),
            manifest=Path(bundle["manifest"]),
            manifest_sha256=bundle["manifest_sha256"],
            owner_uid=owner_uid,
        )
        if binary not in files:
            raise ValueError("fixed Cursor executable is absent from its bundle")
    verify_executable_file(binary, item["sha256"], owner_uid=owner_uid)
    profile = Path(item["profile"])
    profile_fd = verify_root_readonly_directory(profile, label="fixed client profile")
    os.close(profile_fd)
    if item["client_id"] == "kiro":
        if item["sha256"] != KIRO_CLI_SHA256 or item["companion_sha256"] != KIRO_CHAT_SHA256:
            raise ValueError("Kiro executable digest contract differs")
        companion = Path(item["companion_binary"])
        if companion != Path("/opt/uap-observer-inputs/bin/kiro-cli-chat"):
            raise ValueError("Kiro companion executable path differs")
        verify_executable_file(companion, item["companion_sha256"], owner_uid=owner_uid)
    return binary


def verified_native_projection(
    item: dict[str, Any], plugin: str, approved_tuple: dict[str, Any], *, owner_uid: int,
) -> dict[str, Any]:
    """Read the deployment-bound native projection proof from the client profile."""
    projection = item.get("native_projection")
    if not isinstance(projection, dict) or set(projection) != {"path", "sha256"}:
        raise ValueError("fixed client native projection config is invalid")
    profile = Path(item["profile"])
    path = Path(projection.get("path", ""))
    if path.name != "native-projection.json" or path.parent.name != item.get("client_id") or not path.is_absolute() or profile in path.parents:
        raise ValueError("fixed client native projection path is not protected")
    profile_fd = verify_root_readonly_directory(profile, label="fixed client profile")
    os.close(profile_fd)
    proof_fd = verify_root_readonly_directory(path.parent, label="fixed client proof directory")
    os.close(proof_fd)
    value = load_json(path, projection["sha256"], owner_uid=0, mode=0o440)
    if set(value) != {"schema_version", "client_id", "entries"} or type(value.get("schema_version")) is not int or value.get("schema_version") != 1:
        raise ValueError("fixed client native projection proof is not canonical")
    entries = value.get("entries")
    if value.get("client_id") != item.get("client_id") or not isinstance(entries, list):
        raise ValueError("fixed client native projection identity differs")
    evidence_fields = {"manager_add_sha256", "manager_info_sha256", "post_add_doctor_sha256"}
    if any(not isinstance(entry, dict) or set(entry) != {"plugin", "tuple", "native_config", "client_config", *evidence_fields} for entry in entries):
        raise ValueError("fixed client native projection entry is invalid")
    if {entry["plugin"] for entry in entries} != HEROES:
        raise ValueError("fixed client native projection omits a hero")
    for entry in entries:
        validate_release_tuple(entry.get("tuple"), entry["plugin"])
    validate_release_tuple(approved_tuple, plugin)
    matches = [
        entry for entry in entries
        if entry.get("plugin") == plugin
        and _static_tuple(entry.get("tuple")) == _static_tuple(approved_tuple)
    ]
    if len(matches) != 1 or len({entry["plugin"] for entry in entries}) != len(entries):
        raise ValueError("fixed client native projection does not bind the exact approved tuple")
    match = matches[0]
    native = match["native_config"]
    if not isinstance(native, dict) or set(native) != {"path", "sha256"}:
        raise ValueError("fixed client native config proof is invalid")
    native_path = Path(str(native.get("path", "")))
    if not native_path.is_absolute() or path.parent not in native_path.parents or native_path == path:
        raise ValueError("fixed client native config proof escapes its protected hierarchy")
    verify_root_readonly_ancestors(path.parent, native_path.parent)
    proof_body = read_regular(native_path, native.get("sha256"), owner_uid=0, mode=0o440)
    client_native = match["client_config"]
    if not isinstance(client_native, dict) or set(client_native) != {"path", "sha256"} or client_native.get("sha256") != native.get("sha256"):
        raise ValueError("fixed client active native config contract differs")
    client_path = Path(str(client_native.get("path", "")))
    if not client_path.is_absolute() or profile not in client_path.parents:
        raise ValueError("fixed client active native config differs from protected proof")
    verify_root_readonly_ancestors(profile, client_path.parent)
    if read_regular(client_path, client_native["sha256"], owner_uid=0, mode=0o440) != proof_body:
        raise ValueError("fixed client active native config differs from protected proof")
    if any(not re.fullmatch(r"sha256:[a-f0-9]{64}", str(match.get(field, ""))) for field in evidence_fields):
        raise ValueError("fixed client manager evidence proof digest is invalid")
    return value


def verified_git(item: Any, owner_uid: int) -> Path:
    if not isinstance(item, dict) or set(item) != {"binary", "sha256"}:
        raise ValueError("fixed Git config is invalid")
    binary = Path(item["binary"])
    if binary != GIT_BINARY:
        raise ValueError("fixed Git executable differs")
    verify_executable_file(binary, item["sha256"], owner_uid=owner_uid)
    return binary


def parsed_json_stream(encoded: bytes) -> list[Any]:
    try:
        value = strict_json_loads(encoded)
        # One complete JSON value is one outer record.  In particular, never
        # flatten a top-level array into events: Cursor's reviewed stream-json
        # contract is JSONL, so an array line is an unreviewed envelope and
        # must remain visible to the fail-closed evidence recognizer.
        return [value]
    except (json.JSONDecodeError, ValueError):
        values = []
        for line in encoded.splitlines():
            if line.strip():
                values.append(strict_json_loads(line))
        if not values:
            raise ValueError("fixed client emitted no structured evidence")
        return values


def nested_strings(value: Any) -> list[str]:
    if isinstance(value, str):
        return [value]
    if isinstance(value, list):
        return [text for item in value for text in nested_strings(item)]
    if isinstance(value, dict):
        return [text for key, item in value.items() for text in (str(key), *nested_strings(item))]
    return []


def exact_identity_record(value: Any, plugin: str) -> bool:
    if isinstance(value, str):
        return value == plugin
    if not isinstance(value, dict):
        return False
    negative_health = re.compile(r"\b(?:not\s+(?:connected|ready|running|enabled|loaded|healthy|available)|needs\s+approval|disconnected|stopped|disabled|failed|failure|error|unavailable|unhealthy|degraded|cancelled|canceled)\b", re.IGNORECASE)
    if (
        explicit_failure_marker(value)
        or any(isinstance(value.get(field), str) and negative_health.search(value[field]) for field in ("status", "state", "health", "connection", "connectivity", "readiness"))
    ):
        return False
    identities = [
        value.get(field) for field in ("product_id", "plugin", "name", "id", "server", "server_name")
        if field in value
    ]
    return bool(identities) and all(identity == plugin for identity in identities)


def identity_in_collection(value: Any, plugin: str, collections: set[str]) -> bool:
    """Accept identities only in a typed inventory collection, never incidental text."""
    if not isinstance(value, dict):
        return False
    for key in collections:
        collection = value.get(key)
        if isinstance(collection, dict):
            if plugin in collection:
                candidate = collection[plugin]
                if candidate is True or candidate is None:
                    return True
                if isinstance(candidate, dict) and exact_identity_record({**candidate, "name": plugin}, plugin):
                    return True
            if any(exact_identity_record(item, plugin) for item in collection.values()):
                return True
        elif isinstance(collection, list) and any(exact_identity_record(item, plugin) for item in collection):
            return True
    return False


def _static_tuple(value: Any) -> dict[str, Any] | None:
    if not isinstance(value, dict):
        return None
    return {key: child for key, child in value.items() if key not in {"observed_at", "client_version"}}


def exact_observed_identity(value: Any, plugin: str, collections: set[str], approved_tuple: dict[str, Any]) -> dict[str, Any] | None:
    if not isinstance(value, dict):
        return None
    for key in collections:
        collection = value.get(key)
        candidates = list(collection.items()) if isinstance(collection, dict) else [(None, item) for item in collection] if isinstance(collection, list) else []
        for record_key, candidate in candidates:
            identity_fields = {"product_id", "plugin", "name", "id", "server", "server_name"}
            identity_matches = isinstance(candidate, dict) and (
                exact_identity_record(candidate, plugin)
                if identity_fields.intersection(candidate)
                else record_key == plugin
            )
            if (
                identity_matches
                and _static_tuple(candidate.get("tuple")) == _static_tuple(approved_tuple)
            ):
                return candidate
    return None


def manager_receipt_present(value: Any, plugin: str, approved_tuple: dict[str, Any] | None = None) -> bool:
    if approved_tuple is None:
        return identity_in_collection(value, plugin, {"products", "receipts", "plugins", "installations", "entries"})
    return exact_observed_identity(value, plugin, {"products", "receipts", "plugins", "installations", "entries"}, approved_tuple) is not None


def receipt_binds_projection(receipt: Any, projection: Any) -> bool:
    """Require every sealed manager-evidence digest in both protected views."""
    if (
        not isinstance(receipt, dict) or set(receipt) != {"schema_version", "receipts"}
        or type(receipt.get("schema_version")) is not int or receipt.get("schema_version") != 1 or not isinstance(receipt.get("receipts"), list)
        or not isinstance(projection, dict) or not isinstance(projection.get("entries"), list)
    ):
        return False
    evidence = {"manager_add_sha256", "manager_info_sha256", "post_add_doctor_sha256"}
    expected_keys = {"name", "tuple", *evidence}
    records = receipt["receipts"]
    entries = projection["entries"]
    if (
        len(records) != len(entries)
        or any(not isinstance(record, dict) or set(record) != expected_keys for record in records)
        or len({record["name"] for record in records}) != len(records)
    ):
        return False
    by_name = {record["name"]: record for record in records}
    return all(
        isinstance(entry, dict) and entry.get("plugin") in by_name
        and entry.get("tuple") == by_name[entry["plugin"]].get("tuple")
        and all(
            entry.get(field) == by_name[entry["plugin"]].get(field)
            and re.fullmatch(r"sha256:[a-f0-9]{64}", str(entry.get(field, ""))) is not None
            for field in evidence
        )
        for entry in entries
    )


def native_discovery_present(value: Any, plugin: str, approved_tuple: dict[str, Any] | None = None) -> bool:
    # A false health/control anywhere in the complete captured envelope makes
    # every nested identity unusable.  Validate before selecting a candidate so
    # siblings and ancestors cannot be ignored by the identity search.
    if explicit_failure_marker(value):
        return False
    collections = {"servers", "mcp_servers", "mcpServers", "connections", "entries"}
    identity_fields = {"product_id", "plugin", "name", "id", "server", "server_name"}
    records: list[Any] = []
    conflicting_identity = False

    def collect(item: Any) -> None:
        nonlocal conflicting_identity
        if isinstance(item, list):
            for child in item:
                collect(child)
            return
        if not isinstance(item, dict):
            return
        for key, child in item.items():
            if key in collections:
                members = child.items() if isinstance(child, dict) else ((None, member) for member in child) if isinstance(child, list) else ()
                for record_key, record in members:
                    if record_key == plugin:
                        if isinstance(record, dict) and identity_fields.intersection(record):
                            if not exact_identity_record(record, plugin):
                                conflicting_identity = True
                            else:
                                records.append(record)
                        else:
                            records.append({**record, "name": plugin} if isinstance(record, dict) else {"name": plugin, "value": record})
                    elif exact_identity_record(record, plugin):
                        records.append(record)
            collect(child)

    collect(value)
    # One requested identity is the only unambiguous discovery.  A second
    # requested-plugin record is rejected even when it repeats the same tuple:
    # independent collection/depth records can otherwise hide a conflicting or
    # partial authoritative source behind a valid sibling.
    if conflicting_identity or len(records) != 1:
        return False
    record = records[0]
    if not isinstance(record, dict) or not exact_identity_record(record, plugin):
        return False
    if approved_tuple is None:
        return True
    return _static_tuple(record.get("tuple")) == _static_tuple(approved_tuple)


ANSI_ESCAPE = re.compile(r"\x1b\[[0-?]*[ -/]*[@-~]")


def parsed_native_discovery(client: str, encoded: bytes) -> Any:
    if client == "codex":
        return parsed_json_stream(encoded)
    text = encoded.decode("utf-8", "strict")
    if "\x00" in text:
        raise ValueError("fixed client emitted invalid discovery text")
    lines = [ANSI_ESCAPE.sub("", line).strip() for line in text.splitlines() if line.strip()]
    if not lines:
        raise ValueError("fixed client emitted no discovery evidence")
    return lines


def native_discovery_output_present(
    client: str, value: Any, plugin: str, approved_tuple: dict[str, Any] | None = None,
    *, phase: str = "after",
) -> bool:
    if phase not in {"before", "after"}:
        raise ValueError("native discovery phase is invalid")
    if client == "codex":
        return native_discovery_present(value, plugin, approved_tuple)
    # Cursor and Kiro expose text tables. The product must be the exact leading
    # identity and the status must be explicit; a bare identity is not health.
    identity = re.compile(
        rf"^(?:[-*+✓✔●]\s*)?{re.escape(plugin)}(?P<status>(?:(?:\s+|:\s*).*)?)$"
    )
    unhealthy = re.compile(
        r"\b(?:not\s+(?:connected|ready|running|enabled|loaded|healthy|available)|needs\s+approval|disconnected|stopped|disabled|failed|failure|error|unavailable|unhealthy|degraded|cancelled|canceled)\b",
        re.IGNORECASE,
    )
    reviewed_healthy_lines = {"connected", "running", "ready", "enabled"}
    if not isinstance(value, list):
        return False
    healthy = False
    contradictory = False
    for line in value:
        match = identity.fullmatch(line) if isinstance(line, str) else None
        if match is None:
            continue
        status = match.group("status").strip()
        if status.startswith(":"):
            status = status[1:].strip()
        # This one Cursor state proves installed presence before --approve-mcps,
        # but must never be promoted to post-invocation health.
        if client == "cursor" and phase == "before" and status == "not loaded (needs approval)":
            healthy = True
            continue
        # Reject every explicit negative before consulting the complete-line
        # allowlist; positive substrings inside an unreviewed sentence prove nothing.
        if unhealthy.search(status) is not None:
            contradictory = True
            continue
        if status.lower() in reviewed_healthy_lines:
            healthy = True
    return healthy and not contradictory


def successful_tool_event(value: Any, tool: str, plugin: str) -> bool:
    """Recognize only the reviewed Codex completed MCP-call JSONL record."""
    if not isinstance(value, dict) or set(value) != {"type", "item"} or value.get("type") != "item.completed":
        return False
    candidate = value.get("item")
    if (
        not isinstance(candidate, dict)
        or set(candidate) != {"type", "server", "tool_name", "status", "result"}
        or candidate.get("type") != "mcp_tool_call"
    ):
        return False
    return bool(
        candidate.get("tool_name") == tool and candidate.get("server") == plugin
        and candidate.get("status") == "completed"
        and not explicit_failure_marker(candidate)
        and reviewed_success_payload(candidate.get("result"))
    )


def successful_marker_event(value: Any, expected_marker: str) -> bool:
    """Recognize only the reviewed Codex assistant marker JSONL record."""
    if not isinstance(value, dict) or set(value) != {"type", "item"} or value.get("type") != "item.completed":
        return False
    candidate = value.get("item")
    return bool(
        isinstance(candidate, dict) and set(candidate) == {"type", "text"}
        and candidate.get("type") == "agent_message"
        and candidate.get("text") == expected_marker
    )


def successful_cursor_tool_event(value: Any, tool: str, plugin: str) -> bool:
    """Recognize Cursor's typed stream-json completed MCP tool event only."""
    if not isinstance(value, dict) or set(value) not in (
        {"type", "subtype", "tool_call"},
        {"type", "subtype", "call_id", "tool_call"},
    ) or value.get("type") != "tool_call" or value.get("subtype") != "completed":
        return False
    if "call_id" in value and (not isinstance(value["call_id"], str) or not value["call_id"]):
        return False
    envelope = value.get("tool_call")
    if not isinstance(envelope, dict) or set(envelope) != {"mcpToolCall"}:
        return False
    candidate = envelope["mcpToolCall"]
    if not isinstance(candidate, dict) or set(candidate) != {"args", "result"}:
        return False
    arguments, result = candidate.get("args"), candidate.get("result")
    if (
        not isinstance(arguments, dict)
        or set(arguments) not in ({"serverName", "toolName"}, {"serverName", "toolName", "arguments"})
        or ("arguments" in arguments and arguments["arguments"] != {})
        or not isinstance(result, dict)
    ):
        return False
    server = arguments.get("serverName")
    name = arguments.get("toolName")
    success = result.get("success")
    return bool(
        server == plugin and name == tool
        and set(result) == {"success"}
        and not explicit_failure_marker(candidate)
        and reviewed_cursor_success_payload(success)
    )


def successful_cursor_marker_event(value: Any, expected_marker: str) -> bool:
    """Recognize an exact assistant text block, never user/prompt/result text."""
    if not isinstance(value, dict) or set(value) != {"type", "message"} or value.get("type") != "assistant":
        return False
    message = value.get("message")
    if not isinstance(message, dict) or set(message) != {"role", "content"} or message.get("role") != "assistant":
        return False
    content = message.get("content")
    return bool(
        isinstance(content, list) and len(content) == 1
        and isinstance(content[0], dict) and set(content[0]) == {"type", "text"} and content[0].get("type") == "text"
        and content[0].get("text") == expected_marker
    )


def reviewed_success_payload(value: Any) -> bool:
    """Accept only a non-empty structured result with no embedded failure marker."""
    if isinstance(value, bool) or isinstance(value, (int, float, str)) or value is None:
        return False
    if isinstance(value, list):
        return bool(value) and all(reviewed_success_payload(item) for item in value)
    if not isinstance(value, dict) or not value:
        return False
    for key, child in value.items():
        lowered = str(key).lower()
        if lowered.replace("_", "") == "iserror" and child is not False:
            return False
        if lowered in {"success", "ok"} and child is not True and not isinstance(child, (dict, list)):
            return False
        if lowered in {"error", "errors", "exception", "failure", "failed"} and child not in (None, "", [], {}):
            return False
        if lowered in {"status", "state", "outcome"} and isinstance(child, str) and child.lower() in {"error", "failed", "failure", "cancelled", "canceled"}:
            return False
    return any(
        isinstance(child, str) and bool(child.strip())
        or isinstance(child, bool) and child is True
        or isinstance(child, (int, float)) and not isinstance(child, bool) and child != 0
        or isinstance(child, (dict, list)) and reviewed_success_payload(child)
        for child in value.values()
    )


def reviewed_cursor_success_payload(value: Any) -> bool:
    """Exact result shapes captured from pinned Cursor stream-json fixtures."""
    if not isinstance(value, dict) or set(value) != {"content"}:
        return False
    content = value["content"]
    return bool(
        isinstance(content, str) and content.strip()
        or isinstance(content, dict) and set(content) == {"text"}
        and isinstance(content["text"], str) and content["text"].strip()
    )


def explicit_failure_marker(value: Any) -> bool:
    """Reject explicit failure controls at any depth, including false/zero flags."""
    if isinstance(value, list):
        return any(explicit_failure_marker(item) for item in value)
    if not isinstance(value, dict):
        return False
    for key, child in value.items():
        lowered = str(key).lower()
        if lowered in {"health", "healthy", "readiness", "ready", "connection", "connected", "connectivity", "enabled", "running", "loaded"}:
            if isinstance(child, bool) and child is False:
                return True
            if isinstance(child, (int, float)) and not isinstance(child, bool) and child == 0:
                return True
        if lowered.replace("_", "") == "iserror" and child is not False:
            return True
        if lowered in {"success", "ok"} and not isinstance(child, (dict, list)):
            if isinstance(child, bool) and child is False:
                return True
            if isinstance(child, (int, float)) and not isinstance(child, bool):
                if child == 0:
                    return True
            elif child is not True:
                return True
        if lowered in {"error", "errors", "exception", "failure", "failed"} and child not in (None, "", [], {}):
            return True
        if lowered in {"status", "state", "outcome"} and isinstance(child, str) and child.lower() in {"error", "failed", "failure", "cancelled", "canceled"}:
            return True
        if lowered in {"type", "subtype"} and isinstance(child, str) and child.lower() in {"error", "failed", "failure", "cancelled", "canceled", "turn.failed", "item.failed", "item.cancelled", "item.canceled"}:
            return True
        if explicit_failure_marker(child):
            return True
    return False


def _codex_related_terminal(value: Any, tool: str, plugin: str) -> bool:
    if not isinstance(value, dict) or value.get("type") not in {"item.completed", "item.failed", "item.cancelled", "item.canceled"} or not isinstance(value.get("item"), dict):
        return False
    item = value["item"]
    return item.get("type") == "mcp_tool_call" and tool in {item.get(key) for key in ("name", "tool", "tool_name")} and plugin in {item.get(key) for key in ("server", "server_name", "mcp_server", "product_id")}


def successful_codex_turn_completion(value: Any) -> bool:
    """Recognize the reviewed final record emitted by ``codex exec --json``."""
    if not isinstance(value, dict) or set(value) != {"type", "usage"} or value.get("type") != "turn.completed":
        return False
    usage = value.get("usage")
    return bool(
        isinstance(usage, dict)
        and set(usage) == {"input_tokens", "cached_input_tokens", "output_tokens"}
        and all(type(usage.get(field)) is int and usage[field] >= 0 for field in usage)
        and usage["cached_input_tokens"] <= usage["input_tokens"]
    )


def _cursor_related_terminal(value: Any, tool: str, plugin: str) -> bool:
    if not isinstance(value, dict) or value.get("type") != "tool_call" or value.get("subtype") in {"started", "pending"}:
        return False
    envelope = value.get("tool_call")
    candidate = envelope.get("mcpToolCall") if isinstance(envelope, dict) else None
    arguments = candidate.get("args") if isinstance(candidate, dict) else None
    return isinstance(arguments, dict) and arguments.get("serverName", arguments.get("server")) == plugin and arguments.get("toolName", arguments.get("tool")) == tool


def successful_client_evidence(client: str, value: Any, tool: str, plugin: str, expected_marker: str) -> bool:
    events = value if isinstance(value, list) else [value]
    if not events or any(explicit_failure_marker(event) for event in events):
        return False
    if client == "codex":
        successes = [index for index, event in enumerate(events) if successful_tool_event(event, tool, plugin)]
        markers = [index for index, event in enumerate(events) if successful_marker_event(event, expected_marker)]
        turn_completions = [index for index, event in enumerate(events) if successful_codex_turn_completion(event)]
        terminals = [index for index, event in enumerate(events) if isinstance(event, dict) and (
            event.get("type") in {"turn.completed", "turn.failed", "turn.cancelled", "turn.canceled"}
            or event.get("type") in {"item.completed", "item.failed", "item.cancelled", "item.canceled"}
            and isinstance(event.get("item"), dict) and event["item"].get("type") in {"mcp_tool_call", "agent_message"}
        )]
        return bool(
            len(successes) == 1 and len(markers) == 1
            and turn_completions == [len(events) - 1]
            and terminals == [successes[0], markers[0], turn_completions[0]]
            and markers[0] == successes[0] + 1
        )
    if client == "cursor":
        # Each JSONL line is one outer event.  Arrays (including nested arrays)
        # are never event envelopes and must not be collapsed into an accepted
        # tool or marker record.
        if len(events) != 2 or any(not isinstance(event, dict) for event in events):
            return False
        successes = [index for index, event in enumerate(events) if successful_cursor_tool_event(event, tool, plugin)]
        markers = [index for index, event in enumerate(events) if successful_cursor_marker_event(event, expected_marker)]
        # The pinned stream contract contains exactly one completed MCP call
        # followed by exactly one assistant marker.  No progress, result,
        # summary, ambiguity-count, or other unreviewed event is permitted.
        return successes == [0] and markers == [1]
    # Kiro has no reviewed structured output contract. Text, including an exact
    # challenge marker, cannot independently establish a tool call.
    return False


class KiroACPContract:
    """Fail-closed recognizer for the captured Kiro CLI 2.19.1 ACP v1 flow."""

    def __init__(self, plugin: str, tool: str, marker: str) -> None:
        self.plugin, self.tool, self.marker = plugin, tool, marker
        self.session_id: str | None = None
        self.tool_call_id: str | None = None
        self.permission_id: Any = None
        self.allow_once_id: str | None = None
        self.phase = "initialize"
        self.discovery: list[str] = []
        self.discovery_tools: tuple[str, ...] | None = None
        self.marker_parts: list[str] = []
        self.turn_completion = self.turn_end = self.prompt_done = False

    @staticmethod
    def _outer(record: Any, fields: set[str]) -> bool:
        return isinstance(record, dict) and set(record) == fields and record.get("jsonrpc") == "2.0"

    def _session_update(self, record: dict[str, Any]) -> dict[str, Any] | None:
        if not self._outer(record, {"jsonrpc", "method", "params"}) or record.get("method") != "session/update":
            return None
        params = record.get("params")
        if not isinstance(params, dict) or set(params) != {"sessionId", "update"} or params.get("sessionId") != self.session_id:
            raise ValueError("Kiro ACP session update envelope differs")
        update = params.get("update")
        if not isinstance(update, dict) or not isinstance(update.get("sessionUpdate"), str):
            raise ValueError("Kiro ACP update is malformed")
        return update

    def accept(self, record: Any) -> dict[str, Any] | None:
        if not isinstance(record, dict) or explicit_failure_marker(record):
            raise ValueError("Kiro ACP emitted an explicit failure or malformed record")
        if self.phase == "initialize":
            if not self._outer(record, {"jsonrpc", "id", "result"}) or type(record.get("id")) is not int or record.get("id") != 0:
                raise ValueError("Kiro ACP initialize response differs")
            result = record.get("result")
            capabilities = result.get("agentCapabilities") if isinstance(result, dict) else None
            mcp = capabilities.get("mcpCapabilities") if isinstance(capabilities, dict) else None
            if type(result.get("protocolVersion")) is not int or result.get("protocolVersion") != KIRO_PROTOCOL_VERSION or not isinstance(mcp, dict) or mcp.get("http") is not True:
                raise ValueError("Kiro ACP did not negotiate v1 with MCP HTTP capability")
            self.phase = "new"
            return None

        if self.phase in {"new", "running"} and self._outer(record, {"jsonrpc", "id", "result"}) and type(record.get("id")) is int and record.get("id") == 1:
            if self.phase != "new":
                raise ValueError("Kiro ACP duplicated session/new response")
            result = record.get("result")
            if not isinstance(result, dict) or set(result) != {"sessionId"} or not isinstance(result.get("sessionId"), str) or not result["sessionId"]:
                raise ValueError("Kiro ACP session/new response differs")
            self.session_id = result["sessionId"]
            self.phase = "running"
            return None

        update = self._session_update(record)
        if update is not None:
            kind = update["sessionUpdate"]
            if kind == "_kiro/mcp/status":
                if self.tool_call_id is not None:
                    raise ValueError("Kiro ACP discovery arrived after target execution")
                allowed = {"sessionUpdate", "status", "serverName", "tools"}
                if set(update) != allowed or update.get("serverName") != self.plugin:
                    raise ValueError("Kiro ACP discovery server differs")
                status, tools = update.get("status"), update.get("tools")
                if status not in {"connecting", "connected"} or not isinstance(tools, list) or not 1 <= len(tools) <= KIRO_MAX_TOOLS:
                    raise ValueError("Kiro ACP discovery tool or status differs")
                identities: list[str] = []
                for item in tools:
                    if not isinstance(item, dict) or set(item) not in ({"name"}, {"name", "enabled"}):
                        raise ValueError("Kiro ACP discovery catalog contains a malformed tool")
                    name = item.get("name")
                    if (
                        not isinstance(name, str) or len(name) > KIRO_MAX_TOOL_NAME
                        or KIRO_TOOL_NAME.fullmatch(name) is None
                        or "enabled" in item and item["enabled"] is not True
                    ):
                        raise ValueError("Kiro ACP discovery catalog contains a malformed or disabled tool")
                    identities.append(name)
                if len(set(identities)) != len(identities) or identities.count(self.tool) != 1:
                    raise ValueError("Kiro ACP discovery catalog is ambiguous or missing the target")
                catalog = tuple(identities)
                if self.discovery_tools is not None and catalog != self.discovery_tools:
                    raise ValueError("Kiro ACP discovery catalog changed while connecting")
                self.discovery_tools = catalog
                self.discovery.append(status)
                if self.discovery not in (["connecting"], ["connecting", "connected"]):
                    raise ValueError("Kiro ACP discovery is conflicting or duplicated")
                return None
            if kind == "tool_call":
                expected_title = f"@{self.plugin}/{self.tool}"
                if set(update) != {"sessionUpdate", "status", "title", "toolCallId", "_meta"} or update.get("status") != "pending" or update.get("title") != expected_title:
                    raise ValueError("Kiro ACP target pending record differs")
                meta, call_id = update.get("_meta"), update.get("toolCallId")
                if meta != {"kiro": {"serverName": self.plugin}} or not isinstance(call_id, str) or not call_id or self.discovery != ["connecting", "connected"] or self.tool_call_id is not None:
                    raise ValueError("Kiro ACP target pending identity differs")
                self.tool_call_id = call_id
                self.phase = "pending"
                return None
            if kind == "tool_call_update":
                if update.get("toolCallId") != self.tool_call_id or self.phase not in {"permitted", "in_progress"}:
                    raise ValueError("Kiro ACP tool update is reordered or foreign")
                if update.get("status") == "in_progress":
                    if self.phase != "permitted" or set(update) != {"sessionUpdate", "status", "toolCallId", "_meta"} or update.get("_meta") != {"kiro": {"serverName": self.plugin}}:
                        raise ValueError("Kiro ACP in-progress record differs")
                    self.phase = "in_progress"
                    return None
                if update.get("status") == "completed":
                    expected_title = f"@{self.plugin}/{self.tool}"
                    if self.phase != "in_progress" or set(update) != {"sessionUpdate", "status", "title", "toolCallId", "content", "rawOutput", "_meta"} or update.get("title") != expected_title or update.get("_meta") != {"kiro": {"serverName": self.plugin}}:
                        raise ValueError("Kiro ACP completion record differs")
                    content, raw = update.get("content"), update.get("rawOutput")
                    typed = lambda item: isinstance(item, dict) and set(item) == {"type", "content"} and item.get("type") == "content" and substantive_mcp_content(item.get("content"))
                    response = raw.get("response") if isinstance(raw, dict) and set(raw) == {"response"} else None
                    if not isinstance(content, list) or not content or not all(typed(item) for item in content) or not isinstance(response, dict) or not response or explicit_failure_marker(response):
                        raise ValueError("Kiro ACP completion result is empty or unhealthy")
                    self.phase = "completed"
                    return None
                raise ValueError("Kiro ACP tool update has an unknown status")
            if kind == "agent_message_chunk":
                if self.phase not in {"completed", "marker"} or set(update) != {"sessionUpdate", "content"}:
                    raise ValueError("Kiro ACP assistant marker is reordered")
                content = update.get("content")
                if not isinstance(content, dict) or set(content) != {"type", "text"} or content.get("type") != "text" or not isinstance(content.get("text"), str) or not content["text"]:
                    raise ValueError("Kiro ACP assistant marker chunk differs")
                self.marker_parts.append(content["text"])
                joined = "".join(self.marker_parts)
                if not self.marker.startswith(joined):
                    raise ValueError("Kiro ACP assistant marker differs")
                self.phase = "marker"
                return None
            if kind == "session_info_update":
                if self.phase not in {"marker", "turn_completion"} or "".join(self.marker_parts) != self.marker:
                    raise ValueError("Kiro ACP terminal update is reordered")
                meta = update.get("_meta")
                if set(update) == {"sessionUpdate", "status", "_meta"} and meta == {"kiro": {"kind": "turn_completion"}} and update.get("status") == "success" and not self.turn_completion:
                    self.turn_completion, self.phase = True, "turn_completion"
                    return None
                if set(update) == {"sessionUpdate", "stopReason", "_meta"} and meta == {"kiro": {"kind": "turn_end"}} and update.get("stopReason") == "end_turn" and self.turn_completion and not self.turn_end:
                    self.turn_end, self.phase = True, "turn_end"
                    return None
                raise ValueError("Kiro ACP terminal status differs or is duplicated")
            raise ValueError("Kiro ACP emitted an unknown control update")

        if self._outer(record, {"jsonrpc", "id", "method", "params"}) and record.get("method") == "session/request_permission":
            if self.phase != "pending" or self.permission_id is not None:
                raise ValueError("Kiro ACP permission request is extra or reordered")
            permission_id = record.get("id")
            if type(permission_id) not in {int, str} or isinstance(permission_id, str) and not permission_id:
                raise ValueError("Kiro ACP permission request id has an unsupported type")
            params = record.get("params")
            if not isinstance(params, dict) or set(params) != {"sessionId", "toolCall", "options"} or params.get("sessionId") != self.session_id:
                raise ValueError("Kiro ACP permission envelope differs")
            call = params.get("toolCall")
            if call != {"toolCallId": self.tool_call_id, "title": f"@{self.plugin}/{self.tool}", "status": "pending"}:
                raise ValueError("Kiro ACP permission target differs")
            options = params.get("options")
            if not isinstance(options, list) or len(options) != 4 or any(not isinstance(option, dict) or set(option) != {"optionId", "name", "kind"} or not isinstance(option.get("optionId"), str) or not option["optionId"] or not isinstance(option.get("name"), str) or not option["name"] for option in options):
                raise ValueError("Kiro ACP permission options are malformed")
            kinds = [option["kind"] for option in options]
            expected = ["allow_once", "allow_always", "reject_once", "reject_always"]
            if kinds != expected or len({option["optionId"] for option in options}) != 4:
                raise ValueError("Kiro ACP permission options are ambiguous")
            self.permission_id, self.allow_once_id = permission_id, options[0]["optionId"]
            self.phase = "permitted"
            return {"jsonrpc": "2.0", "id": self.permission_id, "result": {"outcome": {"outcome": "selected", "optionId": self.allow_once_id}}}

        if self._outer(record, {"jsonrpc", "id", "result"}) and type(record.get("id")) is int and record.get("id") == 2:
            if self.phase != "turn_end" or record.get("result") != {"stopReason": "end_turn"}:
                raise ValueError("Kiro ACP prompt response differs")
            self.prompt_done, self.phase = True, "done"
            return None
        raise ValueError("Kiro ACP emitted an unknown control record")

    def complete(self) -> bool:
        return self.phase == "done" and self.prompt_done and self.turn_end and self.discovery == ["connecting", "connected"] and "".join(self.marker_parts) == self.marker


def _acp_request(identifier: int, method: str, params: dict[str, Any]) -> bytes:
    return canonical_json({"jsonrpc": "2.0", "id": identifier, "method": method, "params": params}) + b"\n"


def run_kiro_acp(binary: Path, *, workspace: Path, environment: dict[str, str], plugin: str, tool: str, marker: str, timeout: int = COMMAND_SECONDS) -> tuple[dict[str, Any], str, str]:
    """Run the fixed Kiro ACP process with bounded newline JSON I/O."""
    started, deadline = utc_now(), time.monotonic() + timeout
    process = subprocess.Popen(
        [str(binary), *CLIENT_ARGUMENTS["kiro"]], cwd=workspace, env=environment,
        stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
        start_new_session=True, close_fds=True,
    )
    assert process.stdin is not None and process.stdout is not None
    contract = KiroACPContract(plugin, tool, marker)
    try:
        selector = selectors.DefaultSelector()
        selector.register(process.stdout, selectors.EVENT_READ)
    except BaseException:
        process.stdin.close()
        process.stdout.close()
        terminate_group(process)
        raise
    pending = bytearray()
    total = 0
    sent_new = sent_prompt = False
    requests = [
        _acp_request(0, "initialize", {"protocolVersion": 1, "clientCapabilities": {"fs": {"readTextFile": False, "writeTextFile": False}, "terminal": False}, "clientInfo": {"name": "uap-observer", "version": "1"}}),
    ]
    try:
        process.stdin.write(requests[0]); process.stdin.flush()
        while not contract.complete():
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError("Kiro ACP observation exceeded deadline")
            ready = selector.select(min(remaining, 0.25))
            if not ready:
                if process.poll() is not None:
                    raise ValueError("Kiro ACP exited before the structured contract completed")
                continue
            chunk = os.read(process.stdout.fileno(), min(65536, KIRO_MAX_OUTPUT + 1 - total))
            if not chunk:
                raise ValueError("Kiro ACP closed stdout before completion")
            total += len(chunk)
            if total > KIRO_MAX_OUTPUT:
                raise ValueError("Kiro ACP output exceeded size bound")
            pending.extend(chunk)
            if len(pending) > KIRO_MAX_LINE and b"\n" not in pending:
                raise ValueError("Kiro ACP record exceeded line bound")
            while b"\n" in pending:
                line, _, remainder = pending.partition(b"\n")
                pending[:] = remainder
                if not line or len(line) > KIRO_MAX_LINE:
                    raise ValueError("Kiro ACP emitted an empty or oversized record")
                try:
                    record = strict_json_loads(line)
                except (UnicodeDecodeError, json.JSONDecodeError, ValueError) as error:
                    raise ValueError("Kiro ACP emitted malformed JSON") from error
                response = contract.accept(record)
                if response is not None:
                    process.stdin.write(canonical_json(response) + b"\n"); process.stdin.flush()
                if contract.phase == "new" and not sent_new:
                    process.stdin.write(_acp_request(1, "session/new", {"cwd": str(workspace), "mcpServers": []})); process.stdin.flush(); sent_new = True
                if contract.phase == "running" and not sent_prompt:
                    prompt = f"Read-only disposable test. Invoke the installed {plugin} MCP tool named {tool} exactly once. After the tool succeeds, return the exact marker only: {marker}"
                    process.stdin.write(_acp_request(2, "session/prompt", {"sessionId": contract.session_id, "prompt": [{"type": "text", "text": prompt}]})); process.stdin.flush(); sent_prompt = True
                # The exact successful prompt response is the framing boundary.
                # Bytes after it are outside this observation even when already
                # present in the same pipe read.
                if contract.complete():
                    pending.clear()
                    break
    finally:
        selector.close()
        try: process.stdin.close()
        except BrokenPipeError: pass
        process.stdout.close()
        terminate_group(process)
    summary = {
        "protocol_version": KIRO_PROTOCOL_VERSION, "mcp_http": True,
        "server": plugin, "tool": tool, "discovery": list(contract.discovery),
        "permission": "allow_once", "target_chain": ["pending", "in_progress", "completed"],
        "turn_completion": "success", "turn_end": "end_turn",
    }
    return summary, started, utc_now()


def run_client(argv: list[str], *, workspace: Path, environment: dict[str, str], timeout: int = COMMAND_SECONDS) -> tuple[bytes, str, str]:
    started = utc_now()
    process = subprocess.Popen(
        argv, cwd=workspace, env=environment, stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, start_new_session=True,
        shell=False,
    )
    output = bytearray()
    selector = selectors.DefaultSelector()
    assert process.stdout is not None
    os.set_blocking(process.stdout.fileno(), False)
    selector.register(process.stdout, selectors.EVENT_READ)
    deadline = time.monotonic() + timeout
    try:
        while selector.get_map():
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                terminate_group(process)
                raise TimeoutError("fixed client observation exceeded deadline")
            for key, _ in selector.select(min(remaining, 0.25)):
                chunk = os.read(key.fd, min(65536, MAX_STDOUT + 1 - len(output)))
                if not chunk:
                    selector.unregister(key.fileobj)
                    continue
                output.extend(chunk)
                if len(output) > MAX_STDOUT:
                    terminate_group(process)
                    raise ValueError("fixed client output exceeded size bound")
        process.wait(timeout=max(0.01, deadline - time.monotonic()))
    except subprocess.TimeoutExpired:
        terminate_group(process)
        raise TimeoutError("fixed client observation exceeded deadline") from None
    finally:
        selector.close()
        process.stdout.close()
        terminate_group(process)
    ended = utc_now()
    if process.returncode != 0:
        raise ValueError("fixed client observation failed")
    return bytes(output), started, ended


def terminate_group(
    process: subprocess.Popen[bytes], *, wait_seconds: float = KILL_WAIT_SECONDS,
    fatal: Any = os._exit,
) -> None:
    try:
        os.killpg(process.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    if process.poll() is None:
        try:
            process.wait(timeout=wait_seconds)
        except subprocess.TimeoutExpired:
            fatal(70)
            raise RuntimeError("fatal adapter cleanup callback returned")


def invoke(
    item: dict[str, Any], plugin: str, client: str, challenge: str, workspace: Path,
    owner_uid: int, approved_tuple: dict[str, Any] | None = None,
) -> tuple[dict[str, Any], list[str], str, str]:
    binary = verified_executable(item, owner_uid)
    profile = Path(item["profile"])
    expected_marker = f"UAP_OBSERVER_OK {client} {plugin} {challenge}"
    tool = PROBE_TOOLS[plugin]
    prompt = (
        f"Read-only disposable test. Invoke the installed {plugin} MCP tool named {tool} exactly once. "
        f"After the tool succeeds, return the exact marker only: {expected_marker}"
    )
    argv = [str(binary), *CLIENT_ARGUMENTS[client], prompt]
    environment = {
        "PATH": str(GIT_BINARY.parent), "LANG": "C.UTF-8", "LC_ALL": "C.UTF-8",
        "HOME": str(profile), "XDG_CONFIG_HOME": str(profile / ".config"),
        "XDG_CACHE_HOME": str(profile / ".cache"),
        "HTTPS_PROXY": FIXED_HTTPS_PROXY, "https_proxy": FIXED_HTTPS_PROXY,
        "HTTP_PROXY": FIXED_HTTPS_PROXY, "http_proxy": FIXED_HTTPS_PROXY,
        "NO_PROXY": "", "no_proxy": "",
    }
    if client == "codex":
        environment["CODEX_HOME"] = str(profile / ".codex")
    proof_path = Path(item["native_projection"]["path"])
    manager_inventory = proof_path.with_name("receipts.json")
    profile_uid = os.geteuid()
    receipt_before = regular_snapshot(manager_inventory, None, owner_uid=0, mode=0o440)
    projection_before = regular_snapshot(proof_path, item["native_projection"]["sha256"], owner_uid=0, mode=0o440)
    manager_before = strict_json_loads(receipt_before["body"])
    if not manager_receipt_present(manager_before, plugin, approved_tuple):
        raise ValueError("manager receipt does not contain the exact approved release identity")
    if approved_tuple is None:
        raise ValueError("fixed client observation requires an exact approved tuple")
    native_projection = verified_native_projection(item, plugin, approved_tuple, owner_uid=profile_uid)
    if not receipt_binds_projection(manager_before, native_projection):
        raise ValueError("manager receipt does not bind the sealed add/info/doctor evidence")
    native_entry = next(entry for entry in native_projection["entries"] if entry["plugin"] == plugin)
    native_path = Path(native_entry["client_config"]["path"])
    native_config_before = regular_snapshot(native_path, native_entry["client_config"]["sha256"], owner_uid=0, mode=0o440)
    version_stdout, _, _ = run_client([str(binary), "--version"], workspace=workspace, environment=environment, timeout=10)
    if len(version_stdout) > 4096:
        raise ValueError("fixed client version observation failed")
    client_version = version_stdout.decode("utf-8", "strict").strip()
    if not client_version or any(character in client_version for character in "\r\n/\\"):
        raise ValueError("fixed client version marker is invalid")
    if client == "kiro":
        summary, started, ended = run_kiro_acp(
            binary, workspace=workspace, environment=environment,
            plugin=plugin, tool=tool, marker=expected_marker,
        )
        native_before = native_after = summary
        discovery_argv = [str(binary), *CLIENT_ARGUMENTS[client]]
        argv = list(discovery_argv)
        succeeded = True
    else:
        discovery_argv = [str(binary), *CLIENT_DISCOVERY_ARGUMENTS[client]]
        native_before_bytes, _, _ = run_client(discovery_argv, workspace=workspace, environment=environment, timeout=10)
        native_before = parsed_native_discovery(client, native_before_bytes)
        if not native_discovery_output_present(client, native_before, plugin, approved_tuple, phase="before"):
            raise ValueError("native discovery did not contain the approved product")
        stdout, started, ended = run_client(argv, workspace=workspace, environment=environment)
        invocation = parsed_json_stream(stdout)
        succeeded = successful_client_evidence(client, invocation, tool, plugin, expected_marker)
        if not succeeded:
            raise ValueError("fixed client did not emit a successful exact tool invocation")
        native_after_bytes, _, _ = run_client(discovery_argv, workspace=workspace, environment=environment, timeout=10)
        native_after = parsed_native_discovery(client, native_after_bytes)
        if not native_discovery_output_present(client, native_after, plugin, approved_tuple, phase="after"):
            raise ValueError("native discovery disappeared after invocation")
    # These are the final filesystem operations before evidence is constructed.
    # Every run_client call has already reaped and killed its process group.
    revalidate_snapshot(manager_inventory, receipt_before, owner_uid=0, mode=0o440)
    revalidate_snapshot(proof_path, projection_before, owner_uid=0, mode=0o440)
    revalidate_snapshot(native_path, native_config_before, owner_uid=0, mode=0o440)
    verified_native_projection(item, plugin, approved_tuple, owner_uid=profile_uid)
    manager_after_bytes = receipt_before["body"]
    manager_after = strict_json_loads(manager_after_bytes)
    if not manager_receipt_present(manager_after, plugin, approved_tuple):
        raise ValueError("manager receipt disappeared after invocation")
    marker = {
        "client_version": client_version, "client_id": item["client_id"],
        "manager_before_digest": receipt_before["sha256"], "manager_after_digest": sha256(manager_after_bytes),
        "native_before_digest": sha256(canonical_json(native_before)), "native_after_digest": sha256(canonical_json(native_after)),
        "native_projection_digest": projection_before["sha256"],
        "discovery_argv": [client, *(CLIENT_ARGUMENTS[client] if client == "kiro" else CLIENT_DISCOVERY_ARGUMENTS[client])], "tool": tool,
    }
    return marker, argv, started, ended


def complete_tuple(item: dict[str, Any], marker: dict[str, Any], observed_at: str) -> dict[str, Any]:
    value = item.get("tuple")
    validate_release_tuple(value, item["plugin"])
    value = dict(value)
    value["client_version"] = marker.get("client_version")
    value["observed_at"] = observed_at
    validate_release_tuple(value, item["plugin"], sealed=False)
    return value


def historical_external_pr_evidence_matches_request(evidence: Any, request: dict[str, Any]) -> bool:
    """Bind a historical PR capture to this exact catalog and Directory state."""
    if not isinstance(evidence, dict):
        return False
    binding = evidence.get("binding")
    return bool(
        isinstance(binding, dict)
        and evidence.get("catalog_repository") == request.get("catalog_repository")
        and binding.get("catalog_repository") == request.get("catalog_repository")
        and binding.get("catalog_sha") == evidence.get("base_sha") == request.get("github", {}).get("sha")
        and binding.get("directory_snapshot_digest") == request.get("directory_digest")
        and binding.get("release_repository") == request.get("cli_release_repository")
        and binding.get("release_tag") == request.get("cli_release_tag")
        and binding.get("release_manifest_digest") == request.get("release_manifest_digest")
    )


def runtime_record(item: dict[str, Any], client_config: dict[str, Any], request: dict[str, Any], github: dict[str, Any], consent: dict[str, Any], workspace: Path, owner_uid: int, *, mount_config: dict[str, Any] | None = None) -> dict[str, Any]:
    plugin, client, challenge = item["plugin"], item["client"], request["challenge"]["value"]
    marker, argv, started, observed = invoke(
        client_config, plugin, client, challenge, workspace, owner_uid, item["tuple"],
    )
    digest = lambda name: marker[name] if isinstance(marker.get(name), str) and str(marker[name]).startswith("sha256:") else (_ for _ in ()).throw(ValueError("fixed client digest marker is invalid"))
    safe_trace = [client, "protected-runtime-check", plugin, sha256(canonical_json(argv))]
    record = {
        "plugin": plugin, "client": client, "level": "runtime", "outcome": "passed",
        "reason": "fixed protected disposable runtime marker verified", "tuple": complete_tuple(item, marker, observed),
        "challenge": challenge, "run_id": request["github"]["run_id"], "run_attempt": request["github"]["run_attempt"],
        "scenario_id": "hero_5x3_runtime", "started_at": started, "observed_at": observed,
        "client_id": marker["client_id"], "application_id": item["application_id"], "endpoint": item["endpoint"],
        "command_traces": [{"challenge": challenge, "argv": safe_trace, "started_at": started, "ended_at": observed, "exit_code": 0}],
        "github_attestation": github, "identity_id": consent["pseudonymous_identity_id"],
        "consent_artifact_digest": sha256(exported_json(consent)), "runtime_invocation": True,
        "discovery_verified": True, "isolated_identity": True,
        "manager_observation": {"observer": "fixed-adapter-v1", "before_digest": digest("manager_before_digest"), "after_digest": digest("manager_after_digest"), "observed_at": observed},
        "native_observation": {"observer": "fixed-adapter-v1", "before_digest": digest("native_before_digest"), "after_digest": digest("native_after_digest"), "observed_at": observed},
        "receipt_reconciled": True, "native_discovery_reconciled": True,
        "native_discovery_evidence": {
            "basis": "protected_external_observer", "observer": "native-client-command-v1", "client": client,
            "version_operation": {"operation": "version", "argv": [client, "--version"], "observed_client_version": marker["client_version"]},
            "discovery_operation": {"operation": "discovery", "argv": marker["discovery_argv"], "discovered": True, "product_id": plugin},
            "invocation_operation": {
                "operation": "tool_call", "tool": marker["tool"], "product_id": plugin,
                "marker_digest": sha256(f"UAP_OBSERVER_OK {client} {plugin} {challenge}".encode()), "succeeded": True,
            },
        },
        "pseudonymous_identity_id": consent["pseudonymous_identity_id"],
        "pseudonymous_workspace_id": consent["pseudonymous_workspace_id"], "dedicated_identity": True,
        "disposable_project_status": "disposed", "operation_mode": consent["operation_mode"],
        "auth_origin": consent["auth_origin"], "cleanup_outcome": "cleaned", "no_real_project_proof": isolation_proof(mount_config or {}),
        **evidence_bindings(request),
    }
    if plugin == "notion":
        record.update({
            "consent_attested": True, "oauth_artifact_approved": True,
            "projection_receipt_digest": marker["manager_after_digest"],
            "native_app_digest": marker["manager_after_digest"], "native_mcp_digest": marker["native_after_digest"],
        })
    return record


def inconclusive_runtime_record(
    item: dict[str, Any], request: dict[str, Any], github: dict[str, Any],
    consent: dict[str, Any], *, reason: str, mount_config: dict[str, Any] | None = None,
) -> dict[str, Any]:
    observed = utc_now()
    tuple_value = dict(item["tuple"])
    tuple_value["observed_at"] = observed
    return {
        "plugin": item["plugin"], "client": item["client"], "level": "runtime",
        "outcome": "inconclusive", "reason": reason, "tuple": tuple_value,
        "challenge": request["challenge"]["value"], "run_id": request["github"]["run_id"],
        "run_attempt": request["github"]["run_attempt"], "scenario_id": "hero_5x3_runtime",
        "identity_id": consent["pseudonymous_identity_id"],
        "consent_artifact_digest": sha256(exported_json(consent)),
        "pseudonymous_identity_id": consent["pseudonymous_identity_id"],
        "pseudonymous_workspace_id": consent["pseudonymous_workspace_id"],
        "dedicated_identity": True, "disposable_project_status": "disposed",
        "operation_mode": consent["operation_mode"], "auth_origin": consent["auth_origin"],
        "cleanup_outcome": "cleaned", "no_real_project_proof": isolation_proof(mount_config or {}),
        "github_attestation": github, **evidence_bindings(request),
    }


def runtime_artifact(config: dict[str, Any], request: dict[str, Any], github: dict[str, Any], consent: dict[str, Any], owner_uid: int, *, notion_only: bool, selected_client: str) -> dict[str, Any]:
    root = Path(config["workspace_root"]) / selected_client
    root_fd = open_directory(root, allowed_owners={os.geteuid()})
    try:
        info = os.fstat(root_fd)
        if info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) != 0o700:
            raise ValueError("client workspace root is not isolated")
    finally:
        os.close(root_fd)
    records = []
    selected = [item for item in config["matrix"] if item["client"] == selected_client and (item["plugin"] == "notion") == notion_only]
    for item in sorted(selected, key=lambda value: (value["plugin"], value["client"])):
        client_config = config["clients"][item["client"]]
        workspace = Path(tempfile.mkdtemp(prefix="disposable-git-", dir=root))
        record = None
        try:
            try:
                git = verified_git(config["git"], owner_uid)
                result = subprocess.run([str(git), "init", "--quiet", str(workspace)], stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10, check=False, shell=False)
                if result.returncode != 0 or not (workspace / ".git").is_dir():
                    raise ValueError("disposable Git root creation failed")
                record = runtime_record(item, client_config, request, github, consent, workspace, owner_uid, mount_config=config)
            except (OSError, TimeoutError, ValueError, json.JSONDecodeError):
                record = inconclusive_runtime_record(
                    item, request, github, consent,
                    reason="fixed client discovery or challenge-bound tool invocation was not independently verified",
                    mount_config=config,
                )
        finally:
            shutil.rmtree(workspace, ignore_errors=True)
            if workspace.exists():
                raise ValueError("disposable Git root cleanup failed")
        if record is None:
            raise ValueError("fixed runtime observation produced no record")
        records.append(record)
    artifact = {"schema_version": 1, "attestations": records}
    if not notion_only:
        source = config["external_pr_evidence"]
        evidence = load_json(Path(source["path"]), source["sha256"], owner_uid=0, mode=0o640)
        if not historical_external_pr_evidence_matches_request(evidence, request):
            raise ValueError("immutable historical external PR evidence targets another catalog, Directory, or stable release")
        artifact["external_pr_evidence"] = evidence
    return artifact


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):  # type: ignore[no-untyped-def]
        raise ValueError("Cloudflare MCP redirects are forbidden")


def decode_mcp_response(encoded: bytes) -> dict[str, Any]:
    """Decode exactly one JSON or reviewed SSE response without masking records."""
    try:
        text = encoded.decode("utf-8")
    except UnicodeDecodeError as error:
        raise ValueError("Cloudflare MCP response is not UTF-8") from error
    lines = text.splitlines()
    is_sse = any(line.startswith(("event:", "data:")) for line in lines)
    if not is_sse:
        value = strict_json_loads(text)
        if not isinstance(value, dict):
            raise ValueError("Cloudflare MCP JSON response is not an object")
        return value
    if any(len(line.encode("utf-8")) > MAX_SSE_LINE for line in lines):
        raise ValueError("Cloudflare MCP SSE line exceeds size bound")
    records: list[list[str]] = []
    current: list[str] = []
    for line in lines:
        if line == "":
            if current:
                records.append(current)
                current = []
            continue
        current.append(line)
    if current:
        records.append(current)
    if not records or len(records) > MAX_SSE_RECORDS:
        raise ValueError("Cloudflare MCP SSE record count is invalid")
    values: list[dict[str, Any]] = []
    for record in records:
        events = [line[6:].strip() for line in record if line.startswith("event:")]
        data = [line[5:].strip() for line in record if line.startswith("data:")]
        if (
            any(not line.startswith(("event:", "data:")) for line in record)
            or len(events) > 1 or (events and events != ["message"]) or len(data) != 1
        ):
            raise ValueError("Cloudflare MCP SSE framing is unexpected or ambiguous")
        value = strict_json_loads(data[0])
        if not isinstance(value, dict):
            raise ValueError("Cloudflare MCP SSE data is not an object")
        values.append(value)
    if len(values) != 1:
        raise ValueError("Cloudflare MCP SSE contains multiple response records")
    return values[0]


def mcp_call(endpoint: str, request_id: int, method: str, params: dict[str, Any], session: str | None = None) -> tuple[dict[str, Any], str | None]:
    if type(request_id) is not int:
        raise ValueError("Cloudflare MCP request id has an unsupported type")
    payload = canonical_json({"jsonrpc": "2.0", "id": request_id, "method": method, "params": params})
    headers = {"Content-Type": "application/json", "Accept": "application/json, text/event-stream", "MCP-Protocol-Version": "2025-06-18"}
    if session:
        headers["Mcp-Session-Id"] = session
    request = urllib.request.Request(endpoint, data=payload, headers=headers, method="POST")
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({"https": FIXED_HTTPS_PROXY}), NoRedirect())
    with opener.open(request, timeout=15) as response:
        if response.status != 200 or response.geturl() != endpoint:
            raise ValueError("Cloudflare MCP response identity differs")
        encoded = response.read(MAX_STDOUT + 1)
        session = response.headers.get("Mcp-Session-Id") or session
    if len(encoded) > MAX_STDOUT:
        raise ValueError("Cloudflare MCP response exceeds size bound")
    value = decode_mcp_response(encoded)
    if not isinstance(value, dict) or value.get("jsonrpc") != "2.0" or type(value.get("id")) is not int or value.get("id") != request_id or "error" in value or not isinstance(value.get("result"), dict):
        raise ValueError("Cloudflare MCP response is invalid")
    return value["result"], session


def mcp_initialized(endpoint: str, session: str) -> None:
    payload = canonical_json({"jsonrpc": "2.0", "method": "notifications/initialized"})
    request = urllib.request.Request(
        endpoint, data=payload, method="POST",
        headers={"Content-Type": "application/json", "Accept": "application/json, text/event-stream", "MCP-Protocol-Version": "2025-06-18", "Mcp-Session-Id": session},
    )
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({"https": FIXED_HTTPS_PROXY}), NoRedirect())
    with opener.open(request, timeout=15) as response:
        # A JSON-RPC notification has no response object.  Accept only the
        # reviewed empty acknowledgement; parsing or ignoring a body here
        # could let earlier malformed/error stream records escape review.
        encoded = response.read(4097)
        if response.status not in {200, 202, 204} or response.geturl() != endpoint or encoded:
            raise ValueError("Cloudflare MCP initialized notification failed")


def substantive_mcp_content(item: Any) -> bool:
    if not isinstance(item, dict):
        return False
    kind = item.get("type")
    if kind == "text":
        return isinstance(item.get("text"), str) and bool(item["text"].strip())
    if kind == "resource":
        resource = item.get("resource")
        return isinstance(resource, dict) and any(
            isinstance(resource.get(field), str) and bool(resource[field]) for field in ("text", "blob")
        )
    if kind == "resource_link":
        return isinstance(item.get("uri"), str) and bool(item["uri"].strip())
    return False


def wait_human(config: dict[str, Any], request: dict[str, Any], owner_uid: int) -> dict[str, Any]:
    directory = Path(config["chatgpt"]["human_attestation_directory"])
    challenge = request["challenge"]["value"]
    target = directory / f"{challenge}.json"
    deadline = time.monotonic() + HUMAN_WAIT_SECONDS
    while True:
        try:
            encoded = read_regular(target, None, owner_uid=0, mode=0o640)
            break
        except FileNotFoundError:
            if time.monotonic() >= deadline:
                raise TimeoutError("human ChatGPT attestation was not supplied") from None
            time.sleep(1)
    value = strict_json_loads(encoded)
    expected = {
        "schema_version": 1, "challenge": challenge, "run_id": request["github"]["run_id"],
        "run_attempt": request["github"]["run_attempt"], "app_id": config["chatgpt"]["app_id"],
        "request_digest": sha256(canonical_json(request)), "mcp_url": MCP_ENDPOINT,
        "consent": True, "ui_activation": True, "runtime_observed": True, "read_only": True,
        "no_secrets": True, "no_real_project": True,
    }
    if (
        not isinstance(value, dict)
        or set(value) != {*expected, "observed_at", "expires_at"}
        or any(
            type(value.get(key)) is not type(expected_value) or value.get(key) != expected_value
            for key, expected_value in expected.items()
        )
        or type(value.get("observed_at")) is not str
        or type(value.get("expires_at")) is not str
    ):
        raise ValueError("human ChatGPT attestation is invalid")
    observed = datetime.fromisoformat(value["observed_at"].replace("Z", "+00:00"))
    expires = datetime.fromisoformat(value["expires_at"].replace("Z", "+00:00"))
    if observed.tzinfo is None or expires.tzinfo is None or not observed.timestamp() <= time.time() <= expires.timestamp() or expires.timestamp() - observed.timestamp() > 900:
        raise ValueError("human ChatGPT attestation is stale")
    return value


def chatgpt_artifact(config: dict[str, Any], request: dict[str, Any], github: dict[str, Any], consent: dict[str, Any], owner_uid: int) -> dict[str, Any]:
    chat = config["chatgpt"]
    binding_path = Path(chat["app_binding_path"])
    if binding_path != Path("/opt/uap-observer-inputs/chatgpt/app-binding.json"):
        raise ValueError("ChatGPT app binding path is not exact")
    binding_snapshot = regular_snapshot(binding_path, chat["app_binding_sha256"], owner_uid=owner_uid, mode=0o640)
    binding = strict_json_loads(binding_snapshot["body"])
    if binding != {"apps": {"cloudflare-docs": {"id": chat["app_id"]}}} or chat["mcp_endpoint"] != MCP_ENDPOINT:
        raise ValueError("ChatGPT app binding differs")
    receipt_path = Path(chat["projection_receipt_path"])
    receipt_snapshot = regular_snapshot(receipt_path, chat["projection_receipt_sha256"], owner_uid=owner_uid, mode=0o640)
    receipt = strict_json_loads(receipt_snapshot["body"])
    if (
        receipt.get("product_id") != "cloudflare-docs" or receipt.get("application_id") != chat["app_id"]
        or _static_tuple(receipt.get("tuple")) != _static_tuple(chat["tuple"])
    ):
        raise ValueError("ChatGPT projection receipt differs from the approved release identity")
    initialized, session = mcp_call(MCP_ENDPOINT, 1, "initialize", {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "uap-observer", "version": "1"}})
    if not session or initialized.get("protocolVersion") != "2025-06-18":
        raise ValueError("Cloudflare MCP initialize contract differs")
    mcp_initialized(MCP_ENDPOINT, session)
    tools, session = mcp_call(MCP_ENDPOINT, 2, "tools/list", {}, session)
    names = {item.get("name") for item in tools.get("tools", []) if isinstance(item, dict)}
    if MCP_READ_TOOL not in names:
        raise ValueError("Cloudflare MCP read-only tool is unavailable")
    result, _ = mcp_call(MCP_ENDPOINT, 3, "tools/call", {"name": MCP_READ_TOOL, "arguments": MCP_READ_ARGUMENTS}, session)
    content = result.get("content")
    if explicit_failure_marker(result) or not isinstance(content, list) or not content or not all(substantive_mcp_content(item) for item in content):
        raise ValueError("Cloudflare MCP read-only marker was not observed")
    public_mcp = {
        "basis": "protected_external_observer", "observer": "public-mcp-command-v1",
        "endpoint": MCP_ENDPOINT, "protocol_version": "2025-06-18",
        "initialize": {"method": "initialize", "passed": True},
        "list": {"method": "tools/list", "required_name": MCP_READ_TOOL, "passed": True},
        "read": {"method": "tools/call", "name": MCP_READ_TOOL, "read_only": True, "marker_digest": sha256(canonical_json(content)), "passed": True},
    }
    binding_digest = sha256(canonical_json(binding))
    mcp_digest = sha256(canonical_json(public_mcp))
    human = wait_human(config, request, owner_uid)
    revalidate_snapshot(binding_path, binding_snapshot, owner_uid=owner_uid, mode=0o640)
    revalidate_snapshot(receipt_path, receipt_snapshot, owner_uid=owner_uid, mode=0o640)
    observed = human["observed_at"]
    item = chat["tuple"]
    marker = {"client_version": chat["client_version"]}
    record = {
        "plugin": "cloudflare-docs", "client": "chatgpt", "level": "runtime", "outcome": "passed",
        "reason": "exact app binding, public read-only MCP, and one-time human activation verified",
        "tuple": complete_tuple({"plugin": "cloudflare-docs", "tuple": item}, marker, observed),
        "challenge": request["challenge"]["value"], "run_id": request["github"]["run_id"], "run_attempt": request["github"]["run_attempt"],
        "scenario_id": "chatgpt_registered_binding", "started_at": observed, "observed_at": observed,
        "client_id": "chatgpt", "application_id": chat["app_id"], "endpoint": MCP_ENDPOINT,
        "command_traces": [{"challenge": request["challenge"]["value"], "argv": ["cloudflare-docs", "public-mcp-read", MCP_MARKER], "started_at": observed, "ended_at": observed, "exit_code": 0}],
        "github_attestation": github, "identity_id": consent["pseudonymous_identity_id"],
        "consent_artifact_digest": sha256(exported_json(consent)), "runtime_invocation": True, "discovery_verified": True,
        "consent_attested": True, "isolated_identity": True, "registered_app_binding": True, "ui_activation": True, "read_only": True,
        "projection_receipt_digest": sha256(canonical_json(receipt)), "native_app_digest": binding_digest,
        "native_mcp_digest": mcp_digest, "public_mcp_evidence": public_mcp,
        "manager_observation": {"observer": "fixed-adapter-v1", "before_digest": binding_digest, "after_digest": binding_digest, "observed_at": observed},
        "native_observation": {"observer": "public-cloudflare-mcp-v1", "before_digest": mcp_digest, "after_digest": mcp_digest, "observed_at": observed},
        "receipt_reconciled": True, "native_discovery_reconciled": True,
        "pseudonymous_identity_id": consent["pseudonymous_identity_id"], "pseudonymous_workspace_id": consent["pseudonymous_workspace_id"],
        "dedicated_identity": True, "disposable_project_status": "disposed", "operation_mode": "read-only",
        "auth_origin": consent["auth_origin"], "cleanup_outcome": "cleaned", "no_real_project_proof": isolation_proof(config),
        **evidence_bindings(request),
    }
    return {"schema_version": 1, "attestations": [record]}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--context", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    try:
        artifact_name = ENTRYPOINT_ARTIFACT[Path(os.path.basename(os.sys.argv[0])).name]
    except KeyError:
        raise ValueError("fixed adapter entrypoint is not allowlisted") from None
    if not args.context.is_absolute() or not args.output.is_absolute() or args.context.parent != Path.cwd() or args.output.parent != Path.cwd():
        raise ValueError("fixed adapter paths must be runner-created")
    config_digest = os.environ.get("UAP_OBSERVER_ADAPTER_CONFIG_SHA256", "")
    owner_uid = 0
    config = load_json(CONFIG_PATH, config_digest, owner_uid=owner_uid, mode=0o640)
    validate_config(config)
    context = strict_json_loads(read_regular(args.context, None, owner_uid=os.geteuid(), mode=0o600))
    request, github = validate_context(context)
    validate_request_policy(config, request)
    consent = consent_record(config, request, owner_uid)
    mode = ARTIFACT_MODE[artifact_name]
    selected_client = os.environ.get("UAP_OBSERVER_ADAPTER_CLIENT", "")
    expected_clients = CLIENTS if mode in {"runtime", "notion-oauth"} else {"control"}
    if selected_client not in expected_clients:
        raise ValueError("fixed adapter execution identity differs")
    if mode == "consent":
        artifact = consent
    elif mode == "runtime":
        artifact = runtime_artifact(config, request, github, consent, owner_uid, notion_only=False, selected_client=selected_client)
    elif mode == "notion-oauth":
        artifact = runtime_artifact(config, request, github, consent, owner_uid, notion_only=True, selected_client=selected_client)
    else:
        artifact = chatgpt_artifact(config, request, github, consent, owner_uid)
    descriptor = os.open(args.output, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600)
    with os.fdopen(descriptor, "wb") as stream:
        stream.write(canonical_json(artifact))
        stream.flush()
        os.fsync(stream.fileno())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
