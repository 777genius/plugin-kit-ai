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
import shlex
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
NODE_BINARY = Path("/opt/uap-observer-inputs/cursor/node")
CHROME_ROOT = Path("/opt/uap-observer-inputs/chrome-for-testing")
CHROME_MANIFEST = Path("/opt/uap-observer-inputs/chrome-for-testing-bundle.json")
CHROME_BINARY = CHROME_ROOT / "chrome"
CHROME_VERSION = "152.0.7977.64"
CHROME_RUNTIME_ARGUMENTS = (
    "--headless",
    f"--executablePath={CHROME_BINARY}",
    "--isolated",
    "--chrome-arg=--no-sandbox",
    "--chrome-arg=--disable-setuid-sandbox",
)
FIXED_INPUT_PATHS = {
    str(GIT_BINARY),
    "/opt/uap-observer-inputs/bin/codex",
    "/opt/uap-observer-inputs/cursor",
    "/opt/uap-observer-inputs/cursor/cursor-agent",
    "/opt/uap-observer-inputs/cursor-bundle.json",
    str(CHROME_ROOT),
    str(CHROME_MANIFEST),
    str(CHROME_BINARY),
    "/opt/uap-observer-inputs/bin/kiro",
    "/opt/uap-observer-inputs/bin/kiro-cli-chat",
    "/opt/uap-observer-inputs/chatgpt/app-binding.json",
    "/opt/uap-observer-inputs/chatgpt/projection-receipt.json",
    "/opt/uap-observer-inputs/external-pr-evidence.json",
}
FIXED_MOUNT_PATHS = FIXED_INPUT_PATHS - {
    "/opt/uap-observer-inputs/cursor/cursor-agent",
    str(CHROME_BINARY),
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
MCP_PROBE_TOOLS = {
    "context7": "resolve-library-id",
    "cloudflare-docs": "search_cloudflare_documentation",
    "chrome-devtools": "list_pages",
    "notion": "search",
}
SKILL_PROBE_TOOL = "grep_search"
SKILL_PROBE_TITLE = "Grep Search"
SKILL_PROBE_QUERY = "^UAP_SKILL_SECRET_"
SKILL_NAME = "code-tool-router"
SKILL_PROBE_PROMPT = (
    "Read-only disposable test. Use the installed code-tool-router skill to find the only line "
    "beginning UAP_SKILL_SECRET_ in this workspace. Return that exact line only."
)
MCP_PROBE_HINTS = {
    "context7": " with libraryName React",
    "cloudflare-docs": " with query Cloudflare Durable Objects SQLite storage API",
    "chrome-devtools": "",
    "notion": " with query UAP read-only probe",
}
CURSOR_MAX_THINKING_EVENTS = 32
KIRO_PROTOCOL_VERSION = 1
KIRO_CLI_SHA256 = "sha256:14d835aff3772afb9ffb71e395b433df516c091dea8c43daef46e7cb66368358"
KIRO_CHAT_SHA256 = "sha256:59f47eb75928fa158df1cea31382cb39a4eb0d8ec7afbcfc4c6e75693d35163e"
KIRO_MAX_LINE = 256 << 10
KIRO_MAX_OUTPUT = 1 << 20
KIRO_MAX_TOOLS = 64
KIRO_MAX_TOOL_NAME = 256
KIRO_MAX_AUXILIARY = 256
KIRO_TOOL_NAME = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.:/-]{0,255}")
KIRO_AUXILIARY_METHODS = {
    "_kiro/mcp/status", "_kiro/governance/state", "_kiro/tools/didChange",
    "_kiro/powers/items_changed", "_kiro/steering/documents_changed",
    "_kiro/sessions/changed", "_kiro/hooks/didChange", "_kiro/diagnostics/changed",
}
KIRO_AUXILIARY_UPDATES = {"available_commands_update", "config_option_update", "current_mode_update"}


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
    required = {"schema_version", "request_policy", "git", "clients", "matrix", "consent_record", "chatgpt", "chrome_for_testing", "workspace_root", "external_pr_evidence", "egress_hosts"}
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
        raise ValueError("Kiro ACP executables differ from the captured 2.20.0 closure")
    if any("companion_binary" in value["clients"][client] or "companion_sha256" in value["clients"][client] for client in {"codex", "cursor"}):
        raise ValueError("non-Kiro client has an unreviewed companion executable")
    if "bundle" in value["clients"]["codex"] or "bundle" in value["clients"]["kiro"]:
        raise ValueError("non-Cursor client has an unreviewed bundle")
    chrome = value.get("chrome_for_testing")
    if (
        not isinstance(chrome, dict)
        or set(chrome) != {"root", "manifest", "manifest_sha256", "binary", "binary_sha256", "version"}
        or chrome.get("root") != str(CHROME_ROOT)
        or chrome.get("manifest") != str(CHROME_MANIFEST)
        or chrome.get("binary") != str(CHROME_BINARY)
        or chrome.get("version") != CHROME_VERSION
        or any(
            re.fullmatch(r"sha256:[a-f0-9]{64}", str(chrome.get(field, ""))) is None
            for field in ("manifest_sha256", "binary_sha256")
        )
    ):
        raise ValueError("Chrome for Testing bundle contract differs")
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
        chrome.get("root"),
        chrome.get("manifest"),
        chrome.get("binary"),
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


def verified_runtime_node(bundle: Any, owner_uid: int) -> Path:
    """Return the exact Node runtime already bound by the Cursor bundle."""
    if (
        not isinstance(bundle, dict)
        or set(bundle) != {"root", "manifest", "manifest_sha256"}
        or bundle.get("root") != str(NODE_BINARY.parent)
        or bundle.get("manifest") != "/opt/uap-observer-inputs/cursor-bundle.json"
    ):
        raise ValueError("fixed Node runtime bundle config differs")
    files = verify_bundle(
        root=Path(bundle["root"]), manifest=Path(bundle["manifest"]),
        manifest_sha256=bundle["manifest_sha256"], owner_uid=owner_uid,
    )
    if NODE_BINARY not in files:
        raise ValueError("fixed Node runtime is absent from its verified bundle")
    info = os.lstat(NODE_BINARY)
    if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o755:
        raise ValueError("fixed Node runtime is not executable")
    return NODE_BINARY


def verified_runtime_browser(bundle: Any, owner_uid: int) -> Path:
    """Return the exact Chrome for Testing binary from its complete bundle."""
    if (
        not isinstance(bundle, dict)
        or set(bundle) != {"root", "manifest", "manifest_sha256", "binary", "binary_sha256", "version"}
        or bundle.get("root") != str(CHROME_ROOT)
        or bundle.get("manifest") != str(CHROME_MANIFEST)
        or bundle.get("binary") != str(CHROME_BINARY)
        or bundle.get("version") != CHROME_VERSION
    ):
        raise ValueError("fixed Chrome for Testing bundle config differs")
    files = verify_bundle(
        root=CHROME_ROOT, manifest=CHROME_MANIFEST,
        manifest_sha256=bundle["manifest_sha256"], owner_uid=owner_uid,
    )
    if CHROME_BINARY not in files:
        raise ValueError("fixed Chrome executable is absent from its verified bundle")
    verify_executable_file(CHROME_BINARY, bundle["binary_sha256"], owner_uid=owner_uid)
    return CHROME_BINARY


def validate_chrome_runtime_config(encoded: bytes) -> None:
    """Require the observer-only headless browser closure in the sealed MCP config."""
    value = strict_json_loads(encoded)
    servers = value.get("mcpServers") if isinstance(value, dict) else None
    server = servers.get("chrome-devtools") if isinstance(servers, dict) else None
    args = server.get("args") if isinstance(server, dict) else None
    if (
        not isinstance(args, list)
        or any(not isinstance(argument, str) for argument in args)
        or len(args) < len(CHROME_RUNTIME_ARGUMENTS)
        or tuple(args[-len(CHROME_RUNTIME_ARGUMENTS):]) != CHROME_RUNTIME_ARGUMENTS
        or any(args.count(argument) != 1 for argument in CHROME_RUNTIME_ARGUMENTS)
    ):
        raise ValueError("sealed chrome-devtools config omits the fixed headless browser closure")
    forbidden = (
        "--browserUrl", "--browser-url", "--wsEndpoint", "--ws-endpoint",
        "--channel", "--autoConnect", "--auto-connect", "--userDataDir", "--user-data-dir",
    )
    prefix = args[:-len(CHROME_RUNTIME_ARGUMENTS)]
    if any(argument == name or argument.startswith(name + "=") for argument in prefix for name in forbidden):
        raise ValueError("sealed chrome-devtools config contains a conflicting browser selector")


def runtime_environment(profile: Path, runtime_node: Path | None = None) -> dict[str, str]:
    """Build the minimal fixed environment inherited by the reviewed client."""
    runtime_path = str(GIT_BINARY.parent)
    if runtime_node is not None:
        runtime_path += os.pathsep + str(runtime_node.parent)
    environment = {
        "PATH": runtime_path, "LANG": "C.UTF-8", "LC_ALL": "C.UTF-8",
        "HOME": str(profile), "XDG_CONFIG_HOME": str(profile / ".config"),
        "XDG_CACHE_HOME": str(profile / ".cache"),
        "HTTPS_PROXY": FIXED_HTTPS_PROXY, "https_proxy": FIXED_HTTPS_PROXY,
        "HTTP_PROXY": FIXED_HTTPS_PROXY, "http_proxy": FIXED_HTTPS_PROXY,
        "NO_PROXY": "", "no_proxy": "",
    }
    if runtime_node is not None:
        environment["NODE_USE_ENV_PROXY"] = "1"
    return environment


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
    if set(value) != {"schema_version", "client_id", "entries"} or type(value.get("schema_version")) is not int or value.get("schema_version") != 2:
        raise ValueError("fixed client native projection proof is not canonical")
    entries = value.get("entries")
    if value.get("client_id") != item.get("client_id") or not isinstance(entries, list):
        raise ValueError("fixed client native projection identity differs")
    evidence_fields = {"manager_add_sha256", "manager_info_sha256", "post_add_doctor_sha256"}
    entry_fields = {"plugin", "component_kind", "tuple", "native_config", "client_config", *evidence_fields}
    if any(not isinstance(entry, dict) or set(entry) != entry_fields for entry in entries):
        raise ValueError("fixed client native projection entry is invalid")
    if {entry["plugin"] for entry in entries} != HEROES:
        raise ValueError("fixed client native projection omits a hero")
    active_groups: dict[Path, list[dict[str, Any]]] = {}
    for entry in entries:
        validate_release_tuple(entry.get("tuple"), entry["plugin"])
        expected_kind = "skill" if entry["plugin"] == "agent-code-navigator" else "mcp"
        if entry.get("component_kind") != expected_kind:
            raise ValueError("fixed client native projection component kind is invalid")
        native = entry.get("native_config")
        client_native = entry.get("client_config")
        if (
            not isinstance(native, dict) or set(native) != {"path", "sha256"}
            or not isinstance(client_native, dict) or set(client_native) != {"path", "sha256"}
            or native.get("sha256") != client_native.get("sha256")
            or re.fullmatch(r"sha256:[a-f0-9]{64}", str(native.get("sha256", ""))) is None
        ):
            raise ValueError("fixed client native projection config is invalid")
        native_path = Path(str(native.get("path", "")))
        expected_native_path = path.parent / "native" / f'{entry["plugin"]}.blob'
        client_path = Path(str(client_native.get("path", "")))
        if native_path != expected_native_path or not client_path.is_absolute() or profile not in client_path.parents:
            raise ValueError("fixed client native projection path is invalid")
        skill_suffix = client_path.parts[-3:] == ("skills", "code-tool-router", "SKILL.md")
        if (expected_kind == "skill") != skill_suffix:
            raise ValueError("fixed client native projection capability path is invalid")
        active_groups.setdefault(client_path, []).append(entry)
    duplicates = [group for group in active_groups.values() if len(group) > 1]
    if item.get("client_id") == "kiro":
        shared = profile / ".kiro" / "settings" / "mcp.json"
        skill = profile / ".kiro" / "skills" / "code-tool-router" / "SKILL.md"
        if (
            set(active_groups) != {shared, skill}
            or len(duplicates) != 1 or len(duplicates[0]) != len(HEROES) - 1
            or {entry["plugin"] for entry in duplicates[0]} != HEROES - {"agent-code-navigator"}
            or any(entry["component_kind"] != "mcp" for entry in duplicates[0])
            or len({entry["client_config"]["sha256"] for entry in duplicates[0]}) != 1
        ):
            raise ValueError("fixed client native projection has conflicting active configs")
    elif duplicates:
        raise ValueError("fixed client native projection has conflicting active configs")
    validate_release_tuple(approved_tuple, plugin)
    matches = [
        entry for entry in entries
        if entry.get("plugin") == plugin
        and _static_tuple(entry.get("tuple")) == _static_tuple(approved_tuple)
    ]
    if len(matches) != 1 or len({entry["plugin"] for entry in entries}) != len(entries):
        raise ValueError("fixed client native projection does not bind the exact approved tuple")
    match = matches[0]
    if match["component_kind"] != ("skill" if plugin == "agent-code-navigator" else "mcp"):
        raise ValueError("fixed client native projection capability differs")
    native = match["native_config"]
    if not isinstance(native, dict) or set(native) != {"path", "sha256"}:
        raise ValueError("fixed client native config proof is invalid")
    native_path = Path(str(native.get("path", "")))
    if not native_path.is_absolute() or path.parent not in native_path.parents or native_path == path:
        raise ValueError("fixed client native config proof escapes its protected hierarchy")
    verify_root_readonly_ancestors(path.parent, native_path.parent)
    proof_body = read_regular(native_path, native.get("sha256"), owner_uid=0, mode=0o440)
    if match["component_kind"] == "skill":
        if not proof_body.strip():
            raise ValueError("fixed client native skill proof is empty")
    else:
        decoded = strict_json_loads(proof_body)
        if not isinstance(decoded, dict):
            raise ValueError("fixed client native MCP proof is not a JSON object")
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
    known_shapes = (
        {"input_tokens", "cached_input_tokens", "output_tokens"},
        {"input_tokens", "cached_input_tokens", "cache_write_input_tokens", "output_tokens", "reasoning_output_tokens"},
    )
    return bool(
        isinstance(usage, dict) and set(usage) in known_shapes
        and all(type(usage.get(field)) is int and usage[field] >= 0 for field in usage)
        and usage["cached_input_tokens"] <= usage["input_tokens"]
    )


def _codex_command_event(
    value: Any, *, event_type: str, item_id: str | None, command: str,
    output: str, exit_code: int | None, status: str,
) -> str | None:
    if not isinstance(value, dict) or set(value) != {"type", "item"} or value.get("type") != event_type:
        return None
    item = value.get("item")
    if not isinstance(item, dict) or set(item) != {"id", "type", "command", "aggregated_output", "exit_code", "status"}:
        return None
    identifier = item.get("id")
    if (
        not isinstance(identifier, str) or not identifier
        or item_id is not None and identifier != item_id
        or item.get("type") != "command_execution" or item.get("command") != command
        or item.get("aggregated_output") != output or item.get("exit_code") != exit_code
        or item.get("status") != status
    ):
        return None
    return identifier


def _successful_codex_skill_marker_event(value: Any, expected_marker: str) -> bool:
    if not isinstance(value, dict) or set(value) != {"type", "item"} or value.get("type") != "item.completed":
        return False
    item = value.get("item")
    return bool(
        isinstance(item, dict) and set(item) == {"id", "type", "text"}
        and isinstance(item.get("id"), str) and item["id"]
        and item.get("type") == "agent_message" and item.get("text") == expected_marker
    )


def successful_codex_skill_evidence(
    events: Any, expected_marker: str, skill_path: Path, skill_body: bytes,
) -> bool:
    """Recognize the reviewed Codex 0.147 installed-skill and exact-search stream."""
    if not isinstance(events, list) or len(events) not in {8, 9} or any(not isinstance(event, dict) for event in events):
        return False
    if any(explicit_failure_marker(event) for event in events):
        return False
    if set(events[0]) != {"type", "thread_id"} or events[0].get("type") != "thread.started" or not isinstance(events[0].get("thread_id"), str) or not events[0]["thread_id"]:
        return False
    if events[1] != {"type": "turn.started"}:
        return False
    cursor = 2
    if len(events) == 9:
        preface = events[cursor]
        item = preface.get("item") if isinstance(preface, dict) else None
        if (
            set(preface) != {"type", "item"} or preface.get("type") != "item.completed"
            or not isinstance(item, dict) or set(item) != {"id", "type", "text"}
            or item.get("type") != "agent_message" or not isinstance(item.get("text"), str)
            or SKILL_NAME not in item["text"] or expected_marker in item["text"]
        ):
            return False
        cursor += 1
    try:
        decoded_skill = skill_body.decode("utf-8", "strict")
    except UnicodeDecodeError:
        return False
    read_command = f'/bin/bash -lc "sed -n \'1,240p\' {shlex.quote(str(skill_path))}"'
    search_command = f'/bin/bash -lc "rg -n \'{SKILL_PROBE_QUERY}\' ."'
    read_id = _codex_command_event(
        events[cursor], event_type="item.started", item_id=None, command=read_command,
        output="", exit_code=None, status="in_progress",
    )
    if read_id is None or _codex_command_event(
        events[cursor + 1], event_type="item.completed", item_id=read_id,
        command=read_command, output=decoded_skill, exit_code=0, status="completed",
    ) is None:
        return False
    search_id = _codex_command_event(
        events[cursor + 2], event_type="item.started", item_id=None, command=search_command,
        output="", exit_code=None, status="in_progress",
    )
    if search_id is None or search_id == read_id or _codex_command_event(
        events[cursor + 3], event_type="item.completed", item_id=search_id,
        command=search_command, output=f"./uap-skill-probe.txt:1:{expected_marker}\n",
        exit_code=0, status="completed",
    ) is None:
        return False
    return bool(
        _successful_codex_skill_marker_event(events[cursor + 4], expected_marker)
        and successful_codex_turn_completion(events[cursor + 5])
        and cursor + 5 == len(events) - 1
    )


def _cursor_related_terminal(value: Any, tool: str, plugin: str) -> bool:
    if not isinstance(value, dict) or value.get("type") != "tool_call" or value.get("subtype") in {"started", "pending"}:
        return False
    envelope = value.get("tool_call")
    candidate = envelope.get("mcpToolCall") if isinstance(envelope, dict) else None
    arguments = candidate.get("args") if isinstance(candidate, dict) else None
    return isinstance(arguments, dict) and arguments.get("serverName", arguments.get("server")) == plugin and arguments.get("toolName", arguments.get("tool")) == tool


def mcp_probe_prompt(plugin: str, tool: str, marker: str) -> str:
    return (
        f"Read-only disposable test. Invoke the installed {plugin} MCP tool named {tool} exactly once"
        f"{MCP_PROBE_HINTS[plugin]}. After the tool succeeds, return the exact marker only: {marker}"
    )


def _same_path(value: Any, expected: Path) -> bool:
    return isinstance(value, str) and Path(value).resolve(strict=False) == expected.resolve(strict=False)


def _cursor_thinking_block(events: list[Any], index: int, session: str, marker: str) -> int:
    deltas = 0
    while index < len(events):
        event = events[index]
        if not isinstance(event, dict) or event.get("type") != "thinking" or event.get("subtype") != "delta":
            break
        if (
            set(event) != {"type", "subtype", "session_id", "text", "timestamp_ms"}
            or event.get("session_id") != session or not isinstance(event.get("text"), str)
            or not event["text"] or marker in event["text"]
            or type(event.get("timestamp_ms")) is not int or event["timestamp_ms"] < 1
        ):
            raise ValueError("Cursor thinking delta differs from the reviewed stream")
        deltas += 1
        if deltas > CURSOR_MAX_THINKING_EVENTS:
            raise ValueError("Cursor thinking delta bound exceeded")
        index += 1
    if deltas < 1 or index >= len(events):
        raise ValueError("Cursor thinking block is absent")
    completed = events[index]
    if (
        not isinstance(completed, dict)
        or set(completed) != {"type", "subtype", "session_id", "timestamp_ms"}
        or completed.get("type") != "thinking" or completed.get("subtype") != "completed"
        or completed.get("session_id") != session
        or type(completed.get("timestamp_ms")) is not int or completed["timestamp_ms"] < 1
    ):
        raise ValueError("Cursor thinking completion differs from the reviewed stream")
    return index + 1


def _cursor_preface(events: list[Any], index: int, session: str, marker: str) -> int:
    if index >= len(events) or not isinstance(events[index], dict) or events[index].get("type") != "assistant":
        return index
    event = events[index]
    if set(event) != {"type", "session_id", "model_call_id", "timestamp_ms", "message"}:
        raise ValueError("Cursor assistant preface envelope differs")
    message = event.get("message")
    content = message.get("content") if isinstance(message, dict) else None
    if (
        event.get("session_id") != session or not isinstance(event.get("model_call_id"), str) or not event["model_call_id"]
        or type(event.get("timestamp_ms")) is not int or event["timestamp_ms"] < 1
        or not isinstance(message, dict) or set(message) != {"role", "content"} or message.get("role") != "assistant"
        or not isinstance(content, list) or len(content) != 1 or not isinstance(content[0], dict)
        or set(content[0]) != {"type", "text"} or content[0].get("type") != "text"
        or not isinstance(content[0].get("text"), str) or not content[0]["text"] or marker in content[0]["text"]
    ):
        raise ValueError("Cursor assistant preface differs")
    return index + 1


def _cursor_tool_pair(events: list[Any], index: int, session: str, kind: str) -> tuple[dict[str, Any], dict[str, Any], str, int]:
    if index + 1 >= len(events):
        raise ValueError("Cursor tool pair is incomplete")
    start, end = events[index], events[index + 1]
    outer_start = {"type", "subtype", "session_id", "model_call_id", "call_id", "timestamp_ms", "tool_call"}
    if not isinstance(start, dict) or not isinstance(end, dict) or set(start) != outer_start or set(end) != outer_start:
        raise ValueError("Cursor tool event envelope differs")
    identity = (start.get("session_id"), start.get("model_call_id"), start.get("call_id"))
    if (
        start.get("type") != "tool_call" or start.get("subtype") != "started"
        or end.get("type") != "tool_call" or end.get("subtype") != "completed"
        or identity != (end.get("session_id"), end.get("model_call_id"), end.get("call_id"))
        or identity[0] != session or any(not isinstance(value, str) or not value for value in identity[1:])
        or any(type(event.get("timestamp_ms")) is not int or event["timestamp_ms"] < 1 for event in (start, end))
    ):
        raise ValueError("Cursor tool event identity differs")
    key = kind + "ToolCall"
    start_envelope, end_envelope = start.get("tool_call"), end.get("tool_call")
    if (
        not isinstance(start_envelope, dict) or set(start_envelope) != {"hookAdditionalContexts", "startedAtMs", "toolCallId", key}
        or not isinstance(end_envelope, dict) or set(end_envelope) != {"hookAdditionalContexts", "startedAtMs", "completedAtMs", "toolCallId", key}
        or start_envelope["hookAdditionalContexts"] != [] or end_envelope["hookAdditionalContexts"] != []
        or start_envelope["toolCallId"] != end_envelope["toolCallId"]
        or not isinstance(start_envelope["toolCallId"], str) or not start_envelope["toolCallId"]
        or start_envelope["startedAtMs"] != end_envelope["startedAtMs"]
        or any(not isinstance(envelope.get(field), str) or not envelope[field].isdigit() for envelope, field in ((start_envelope, "startedAtMs"), (end_envelope, "completedAtMs")))
    ):
        raise ValueError("Cursor tool payload framing differs")
    return start_envelope[key], end_envelope[key], start_envelope["toolCallId"], index + 2


def _cursor_stream_start(events: Any, prompt: str, workspace: Path) -> tuple[list[Any], str, int]:
    if not isinstance(events, list) or len(events) < 8 or len(events) > 96 or any(not isinstance(event, dict) for event in events):
        raise ValueError("Cursor stream size differs")
    if any(explicit_failure_marker(event) for event in events):
        raise ValueError("Cursor stream contains an explicit failure")
    system, user = events[0], events[1]
    if (
        set(system) != {"type", "subtype", "session_id", "apiKeySource", "cwd", "model", "permissionMode"}
        or system.get("type") != "system" or system.get("subtype") != "init"
        or not isinstance(system.get("session_id"), str) or not system["session_id"]
        or system.get("apiKeySource") != "login" or not _same_path(system.get("cwd"), workspace)
        or not isinstance(system.get("model"), str) or not system["model"] or system.get("permissionMode") != "default"
    ):
        raise ValueError("Cursor initialization differs")
    expected_user = {"role": "user", "content": [{"type": "text", "text": prompt}]}
    if set(user) != {"type", "session_id", "message"} or user.get("type") != "user" or user.get("session_id") != system["session_id"] or user.get("message") != expected_user:
        raise ValueError("Cursor user prompt differs")
    return events, system["session_id"], 2


def _cursor_stream_finish(events: list[Any], index: int, session: str, marker: str) -> bool:
    index = _cursor_thinking_block(events, index, session, marker)
    if index + 2 != len(events):
        return False
    assistant, result = events[index], events[index + 1]
    if set(assistant) != {"type", "session_id", "message"} or assistant.get("session_id") != session:
        return False
    message = assistant.get("message")
    if message != {"role": "assistant", "content": [{"type": "text", "text": marker}]}:
        return False
    usage = result.get("usage") if isinstance(result, dict) else None
    return bool(
        set(result) == {"type", "subtype", "session_id", "request_id", "duration_ms", "duration_api_ms", "is_error", "result", "usage"}
        and result.get("type") == "result" and result.get("subtype") == "success" and result.get("session_id") == session
        and isinstance(result.get("request_id"), str) and result["request_id"]
        and type(result.get("duration_ms")) is int and result["duration_ms"] >= 0
        and type(result.get("duration_api_ms")) is int and result["duration_api_ms"] >= 0
        and result.get("is_error") is False and isinstance(result.get("result"), str)
        and result["result"].endswith(marker) and result["result"].count(marker) == 1
        and isinstance(usage, dict) and set(usage) == {"cacheReadTokens", "cacheWriteTokens", "inputTokens", "outputTokens"}
        and all(type(value) is int and value >= 0 for value in usage.values())
    )


def successful_cursor_skill_evidence(events: Any, marker: str, skill_path: Path, skill_body: bytes, workspace: Path) -> bool:
    try:
        events, session, index = _cursor_stream_start(events, SKILL_PROBE_PROMPT, workspace)
        index = _cursor_thinking_block(events, index, session, marker)
        index = _cursor_preface(events, index, session, marker)
        read_start, read_end, _, index = _cursor_tool_pair(events, index, session, "read")
        if not isinstance(read_start, dict) or set(read_start) != {"args"} or not isinstance(read_end, dict) or set(read_end) != {"args", "result"} or read_start["args"] != read_end["args"]:
            return False
        if not isinstance(read_start["args"], dict) or set(read_start["args"]) != {"path"} or not _same_path(read_start["args"]["path"], skill_path):
            return False
        decoded = skill_body.decode("utf-8", "strict")
        success = read_end["result"].get("success") if isinstance(read_end["result"], dict) and set(read_end["result"]) == {"success"} else None
        # Cursor's read tool reports the terminal empty position after a final
        # newline as one additional line.
        expected_lines = decoded.count("\n") + 1
        if (
            not isinstance(success, dict) or set(success) != {"content", "exceededLimit", "fileSize", "isEmpty", "path", "readRange", "relatedCursorRulePaths", "relatedCursorRules", "totalLines"}
            or success.get("content") != decoded or success.get("exceededLimit") is not False or success.get("isEmpty") is not False
            or success.get("fileSize") != len(skill_body) or not _same_path(success.get("path"), skill_path)
            or success.get("readRange") != {"startLine": 1, "endLine": expected_lines}
            or success.get("totalLines") != expected_lines or success.get("relatedCursorRulePaths") != [] or success.get("relatedCursorRules") != []
        ):
            return False
        index = _cursor_thinking_block(events, index, session, marker)
        index = _cursor_preface(events, index, session, marker)
        grep_start, grep_end, grep_id, index = _cursor_tool_pair(events, index, session, "grep")
        if not isinstance(grep_start, dict) or set(grep_start) != {"args"} or not isinstance(grep_end, dict) or set(grep_end) != {"args", "result"} or grep_start["args"] != grep_end["args"]:
            return False
        args = grep_start["args"]
        if not isinstance(args, dict) or set(args) != {"caseInsensitive", "multiline", "offset", "path", "pattern", "toolCallId"} or args.get("toolCallId") != grep_id or args.get("caseInsensitive") is not False or args.get("multiline") is not False or args.get("offset") != 0 or args.get("pattern") != SKILL_PROBE_QUERY or not _same_path(args.get("path"), workspace):
            return False
        success = grep_end["result"].get("success") if isinstance(grep_end["result"], dict) and set(grep_end["result"]) == {"success"} else None
        roots = success.get("workspaceResults") if isinstance(success, dict) else None
        if not isinstance(roots, dict) or len(roots) != 1:
            return False
        root_name, root_result = next(iter(roots.items()))
        expected_content = {"clientTruncated": False, "matches": [{"file": "./uap-skill-probe.txt", "matches": [{"content": marker, "contentTruncated": False, "isContextLine": False, "lineNumber": 1}]}], "ripgrepTruncated": False, "totalLines": 1, "totalMatchedLines": 1}
        if set(success) != {"outputMode", "path", "pattern", "workspaceResults"} or success.get("outputMode") != "content" or success.get("pattern") != SKILL_PROBE_QUERY or not _same_path(success.get("path"), workspace) or not _same_path(root_name, workspace) or root_result != {"content": expected_content}:
            return False
        return _cursor_stream_finish(events, index, session, marker)
    except (UnicodeDecodeError, ValueError, OSError):
        return False


def successful_cursor_mcp_evidence(events: Any, tool: str, plugin: str, marker: str, workspace: Path) -> bool:
    prompt = mcp_probe_prompt(plugin, tool, marker)
    try:
        events, session, index = _cursor_stream_start(events, prompt, workspace)
        index = _cursor_thinking_block(events, index, session, marker)
        index = _cursor_preface(events, index, session, marker)
        discovery_start, discovery_end, discovery_id, index = _cursor_tool_pair(events, index, session, "getMcpTools")
        if not isinstance(discovery_start, dict) or set(discovery_start) != {"args"} or not isinstance(discovery_end, dict) or set(discovery_end) != {"args", "result"} or discovery_start["args"] != discovery_end["args"]:
            return False
        discovery_args = discovery_start["args"]
        if not isinstance(discovery_args, dict) or set(discovery_args) != {"server", "toolCallId", "toolName"} or discovery_args.get("toolCallId") != discovery_id or discovery_args.get("server") != plugin or discovery_args.get("toolName") != tool:
            return False
        success = discovery_end["result"].get("success") if isinstance(discovery_end["result"], dict) and set(discovery_end["result"]) == {"success"} else None
        content = success.get("content") if isinstance(success, dict) and set(success) == {"content"} else None
        description = strict_json_loads(content) if isinstance(content, str) else None
        described_tool = description.get("tool") if isinstance(description, dict) else None
        if not isinstance(description, dict) or description.get("mode") != "single_tool" or description.get("namespace") != plugin or description.get("namespaceStatus") != "ready" or not isinstance(described_tool, dict) or described_tool.get("tool") != tool:
            return False
        index = _cursor_thinking_block(events, index, session, marker)
        target_start, target_end, target_id, index = _cursor_tool_pair(events, index, session, "mcp")
        if not isinstance(target_start, dict) or not isinstance(target_end, dict) or set(target_start) != {"args", "description"} or set(target_end) != {"args", "description", "result"} or target_start["args"] != target_end["args"] or target_start["description"] != target_end["description"]:
            return False
        args = target_start["args"]
        if (
            not isinstance(args, dict) or set(args) != {"args", "name", "providerIdentifier", "serverIdentifier", "skipApproval", "smartModeApprovalOnly", "toolCallId", "toolName"}
            or args.get("toolCallId") != target_id or args.get("providerIdentifier") != plugin or args.get("serverIdentifier") != plugin or args.get("toolName") != tool
            or args.get("name") != f"{plugin}-{tool}" or args.get("skipApproval") is not False or args.get("smartModeApprovalOnly") is not False
            or not isinstance(args.get("args"), dict) or not isinstance(target_start.get("description"), str) or not target_start["description"] or marker in target_start["description"]
        ):
            return False
        result = target_end["result"]
        success = result.get("success") if isinstance(result, dict) and set(result) == {"success"} else None
        if not reviewed_success_payload(success):
            return False
        return _cursor_stream_finish(events, index, session, marker)
    except (ValueError, OSError, json.JSONDecodeError):
        return False


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
        # Current Cursor evidence is accepted only by the complete pinned
        # 2026.08.25 stream recognizers above, which also bind the prompt,
        # session, discovery/read phase, target tool, marker, and final result.
        return False
    # Kiro has no reviewed structured output contract. Text, including an exact
    # challenge marker, cannot independently establish a tool call.
    return False


class KiroACPContract:
    """Fail-closed recognizer for the captured Kiro CLI 2.20.0 ACP v1 flow."""

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
        self.auxiliary_count = 0

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

    def _count_auxiliary(self) -> None:
        self.auxiliary_count += 1
        if self.auxiliary_count > KIRO_MAX_AUXILIARY:
            raise ValueError("Kiro ACP auxiliary notification bound exceeded")

    def _external_mcp_status(self, record: dict[str, Any]) -> bool:
        if not self._outer(record, {"jsonrpc", "method", "params"}) or record.get("method") != "_kiro/mcp/status":
            return False
        params = record.get("params")
        if not isinstance(params, dict) or not isinstance(params.get("servers"), list):
            raise ValueError("Kiro ACP MCP status notification is malformed")
        if "sessionId" in params and self.session_id is not None and params["sessionId"] != self.session_id:
            raise ValueError("Kiro ACP MCP status belongs to another session")
        matches = [server for server in params["servers"] if isinstance(server, dict) and server.get("name") == self.plugin]
        if not matches:
            self._count_auxiliary()
            return True
        if len(matches) != 1 or self.tool_call_id is not None:
            raise ValueError("Kiro ACP MCP status target is ambiguous or late")
        server = matches[0]
        status, tools = server.get("status"), server.get("tools", [])
        if status not in {"connecting", "connected"} or not isinstance(tools, list) or len(tools) > KIRO_MAX_TOOLS:
            raise ValueError("Kiro ACP MCP status target differs")
        identities = []
        for item in tools:
            name = item.get("name") if isinstance(item, dict) else None
            if not isinstance(name, str) or KIRO_TOOL_NAME.fullmatch(name) is None:
                raise ValueError("Kiro ACP MCP status catalog is malformed")
            identities.append(name)
        if len(set(identities)) != len(identities) or status == "connected" and identities.count(self.tool) != 1:
            raise ValueError("Kiro ACP MCP status catalog is ambiguous or missing the target")
        catalog = tuple(identities)
        if status == "connected":
            self.discovery_tools = catalog
        elif self.discovery_tools is not None:
            raise ValueError("Kiro ACP MCP status regressed after connection")
        self.discovery.append(status)
        if self.discovery not in (["connecting"], ["connecting", "connected"]):
            raise ValueError("Kiro ACP MCP status is conflicting or duplicated")
        return True

    def _accept_auxiliary(self, record: dict[str, Any]) -> bool:
        if not self._outer(record, {"jsonrpc", "method", "params"}) or record.get("method") not in KIRO_AUXILIARY_METHODS:
            return False
        params = record.get("params")
        if not isinstance(params, dict) or not params:
            raise ValueError("Kiro ACP auxiliary notification is malformed")
        if "sessionId" in params and self.session_id is not None and params["sessionId"] != self.session_id:
            raise ValueError("Kiro ACP auxiliary notification belongs to another session")
        self._count_auxiliary()
        return True

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
            if kind in KIRO_AUXILIARY_UPDATES:
                if self.tool_call_id is not None:
                    raise ValueError("Kiro ACP auxiliary update arrived after target execution")
                self._count_auxiliary()
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
                meta = update.get("_meta")
                kiro = meta.get("kiro") if isinstance(meta, dict) else None
                if isinstance(kiro, dict) and kiro.get("kind") == "context_usage" and self.tool_call_id is None:
                    self._count_auxiliary()
                    return None
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

        if self._external_mcp_status(record) or self._accept_auxiliary(record):
            return None
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


def _nested_strings(value: Any) -> list[str]:
    if isinstance(value, str):
        return [value]
    if isinstance(value, list):
        return [text for child in value for text in _nested_strings(child)]
    if isinstance(value, dict):
        return [text for child in value.values() for text in _nested_strings(child)]
    return []


def _contains_exact_line(value: Any, expected: str) -> bool:
    return any(
        line == expected or line.endswith(":" + expected)
        for text in _nested_strings(value)
        for line in text.splitlines()
    )


class KiroACPSkillContract:
    """Recognize one installed-skill disclosure and challenge-bound built-in search."""

    AUXILIARY_METHODS = KIRO_AUXILIARY_METHODS
    AUXILIARY_UPDATES = {"config_option_update", "current_mode_update"}

    def __init__(self, marker: str, skill_path: Path) -> None:
        self.marker = marker
        self.skill_path = skill_path
        self.session_id: str | None = None
        self.phase = "initialize"
        self.skill_catalog_seen = False
        self.disclose_call_id: str | None = None
        self.permission_id: Any = None
        self.allow_once_id: str | None = None
        self.search_call_id: str | None = None
        self.search_output_seen = False
        self.marker_parts: list[str] = []
        self.turn_completion = self.turn_end = self.prompt_done = False
        self.auxiliary_count = 0

    @staticmethod
    def _outer(record: Any, fields: set[str]) -> bool:
        return isinstance(record, dict) and set(record) == fields and record.get("jsonrpc") == "2.0"

    def _session_update(self, record: dict[str, Any]) -> dict[str, Any] | None:
        if not self._outer(record, {"jsonrpc", "method", "params"}) or record.get("method") != "session/update":
            return None
        params = record.get("params")
        if not isinstance(params, dict) or set(params) != {"sessionId", "update"} or params.get("sessionId") != self.session_id:
            raise ValueError("Kiro ACP skill session update envelope differs")
        update = params.get("update")
        if not isinstance(update, dict) or not isinstance(update.get("sessionUpdate"), str):
            raise ValueError("Kiro ACP skill update is malformed")
        return update

    def _accept_auxiliary(self, record: dict[str, Any]) -> bool:
        if not self._outer(record, {"jsonrpc", "method", "params"}) or record.get("method") not in self.AUXILIARY_METHODS:
            return False
        params = record.get("params")
        if not isinstance(params, dict) or not params:
            raise ValueError("Kiro ACP auxiliary notification is malformed")
        if "sessionId" in params and self.session_id is not None and params["sessionId"] != self.session_id:
            raise ValueError("Kiro ACP auxiliary notification belongs to another session")
        self.auxiliary_count += 1
        if self.auxiliary_count > KIRO_MAX_AUXILIARY:
            raise ValueError("Kiro ACP auxiliary notification bound exceeded")
        return True

    def _catalog_match(self, update: dict[str, Any]) -> bool:
        commands = update.get("availableCommands")
        if not isinstance(commands, list) or not commands:
            return False
        matches = []
        for command in commands:
            if not isinstance(command, dict) or command.get("name") != SKILL_NAME:
                continue
            strings = _nested_strings(command)
            matches.append(
                str(self.skill_path) in strings
                and any(value is True for value in self._nested_values(command, "matched"))
            )
        return matches == [True]

    @staticmethod
    def _nested_values(value: Any, key: str) -> list[Any]:
        values: list[Any] = []
        if isinstance(value, list):
            for child in value:
                values.extend(KiroACPSkillContract._nested_values(child, key))
        elif isinstance(value, dict):
            for name, child in value.items():
                if name == key:
                    values.append(child)
                values.extend(KiroACPSkillContract._nested_values(child, key))
        return values

    @staticmethod
    def _meta(update: dict[str, Any]) -> dict[str, Any]:
        meta = update.get("_meta")
        kiro = meta.get("kiro") if isinstance(meta, dict) else None
        if not isinstance(kiro, dict):
            raise ValueError("Kiro ACP skill tool metadata is absent")
        return kiro

    def _accept_disclose_call(self, update: dict[str, Any]) -> None:
        if self.phase != "running" or not self.skill_catalog_seen or self.disclose_call_id is not None:
            raise ValueError("Kiro ACP skill disclosure is reordered")
        call_id = update.get("toolCallId")
        raw_input = update.get("rawInput")
        meta = self._meta(update)
        disclosed = meta.get("disclosedContext")
        if (
            update.get("status") != "pending"
            or not isinstance(call_id, str) or not call_id
            or raw_input != {"name": SKILL_NAME}
            or meta.get("toolOrigin") != "acp"
            or not isinstance(disclosed, dict)
            or disclosed.get("type") != "skill"
            or disclosed.get("displayName") != SKILL_NAME
            or disclosed.get("uri") != self.skill_path.as_uri()
        ):
            raise ValueError("Kiro ACP skill disclosure identity differs")
        self.disclose_call_id, self.phase = call_id, "disclose_pending"

    def _accept_search_call(self, update: dict[str, Any]) -> None:
        if self.phase != "disclosed" or self.search_call_id is not None:
            raise ValueError("Kiro ACP built-in search is reordered")
        call_id = update.get("toolCallId")
        raw_input = update.get("rawInput")
        meta = self._meta(update)
        if (
            update.get("status") != "pending" or update.get("title") != SKILL_PROBE_TITLE
            or not isinstance(call_id, str) or not call_id
            or not isinstance(raw_input, dict) or set(raw_input) - {"query", "explanation"}
            or raw_input.get("query") != SKILL_PROBE_QUERY
            or "explanation" in raw_input and (not isinstance(raw_input["explanation"], str) or not raw_input["explanation"])
            or meta.get("toolOrigin") != "default"
        ):
            raise ValueError("Kiro ACP built-in search identity differs")
        self.search_call_id, self.phase = call_id, "search_pending"

    def _accept_tool_update(self, update: dict[str, Any]) -> None:
        call_id, status = update.get("toolCallId"), update.get("status")
        if call_id == self.disclose_call_id:
            if status == "in_progress" and self.phase == "disclose_permitted":
                self.phase = "disclose_running"
                return
            if status == "completed" and self.phase in {"disclose_permitted", "disclose_running"}:
                if str(self.skill_path) not in _nested_strings(update) or SKILL_NAME not in _nested_strings(update):
                    raise ValueError("Kiro ACP skill disclosure result differs")
                self.phase = "disclosed"
                return
            raise ValueError("Kiro ACP skill disclosure update is reordered")
        if call_id == self.search_call_id:
            if status == "in_progress" and self.phase == "search_pending":
                self.phase = "search_running"
                return
            if status == "completed" and self.phase == "search_running":
                if not _contains_exact_line(update, self.marker):
                    raise ValueError("Kiro ACP built-in search did not return the hidden marker")
                self.search_output_seen, self.phase = True, "search_completed"
                return
            raise ValueError("Kiro ACP built-in search update is reordered")
        raise ValueError("Kiro ACP skill flow contains a foreign tool update")

    def accept(self, record: Any) -> dict[str, Any] | None:
        if not isinstance(record, dict) or explicit_failure_marker(record):
            raise ValueError("Kiro ACP skill flow emitted an explicit failure or malformed record")
        if self.phase == "initialize":
            if not self._outer(record, {"jsonrpc", "id", "result"}) or type(record.get("id")) is not int or record.get("id") != 0:
                raise ValueError("Kiro ACP skill initialize response differs")
            result = record.get("result")
            capabilities = result.get("agentCapabilities") if isinstance(result, dict) else None
            if type(result.get("protocolVersion")) is not int or result.get("protocolVersion") != KIRO_PROTOCOL_VERSION or not isinstance(capabilities, dict):
                raise ValueError("Kiro ACP skill protocol negotiation differs")
            self.phase = "new"
            return None
        if self.phase == "new" and self._outer(record, {"jsonrpc", "id", "result"}) and type(record.get("id")) is int and record.get("id") == 1:
            result = record.get("result")
            if not isinstance(result, dict) or set(result) != {"sessionId"} or not isinstance(result.get("sessionId"), str) or not result["sessionId"]:
                raise ValueError("Kiro ACP skill session/new response differs")
            self.session_id, self.phase = result["sessionId"], "running"
            return None
        update = self._session_update(record)
        if update is not None:
            kind = update["sessionUpdate"]
            if kind == "available_commands_update":
                if self.skill_catalog_seen or self.disclose_call_id is not None or not self._catalog_match(update):
                    raise ValueError("Kiro ACP installed skill catalog differs")
                self.skill_catalog_seen = True
                return None
            if kind in self.AUXILIARY_UPDATES:
                if self.disclose_call_id is not None:
                    raise ValueError("Kiro ACP auxiliary update arrived after skill execution")
                self.auxiliary_count += 1
                if self.auxiliary_count > KIRO_MAX_AUXILIARY:
                    raise ValueError("Kiro ACP auxiliary update bound exceeded")
                return None
            if kind == "tool_call":
                title = update.get("title")
                if title == SKILL_PROBE_TITLE:
                    self._accept_search_call(update)
                else:
                    self._accept_disclose_call(update)
                return None
            if kind == "tool_call_update":
                self._accept_tool_update(update)
                return None
            if kind == "agent_message_chunk":
                if self.phase not in {"search_completed", "marker"} or set(update) != {"sessionUpdate", "content"}:
                    raise ValueError("Kiro ACP skill marker is reordered")
                content = update.get("content")
                if not isinstance(content, dict) or set(content) != {"type", "text"} or content.get("type") != "text" or not isinstance(content.get("text"), str) or not content["text"]:
                    raise ValueError("Kiro ACP skill marker chunk differs")
                self.marker_parts.append(content["text"])
                joined = "".join(self.marker_parts)
                if not any(candidate.startswith(joined) for candidate in (self.marker, f"`{self.marker}`")):
                    raise ValueError("Kiro ACP skill marker differs")
                self.phase = "marker"
                return None
            if kind == "session_info_update":
                meta = update.get("_meta")
                kiro = meta.get("kiro") if isinstance(meta, dict) else None
                if isinstance(kiro, dict) and kiro.get("kind") == "context_usage":
                    self.auxiliary_count += 1
                    if self.auxiliary_count > KIRO_MAX_AUXILIARY:
                        raise ValueError("Kiro ACP auxiliary update bound exceeded")
                    return None
                if self.phase not in {"marker", "turn_completion"} or "".join(self.marker_parts) not in {self.marker, f"`{self.marker}`"}:
                    raise ValueError("Kiro ACP skill terminal update is reordered")
                if set(update) == {"sessionUpdate", "status", "_meta"} and kiro == {"kind": "turn_completion"} and update.get("status") == "success" and not self.turn_completion:
                    self.turn_completion, self.phase = True, "turn_completion"
                    return None
                if set(update) == {"sessionUpdate", "stopReason", "_meta"} and kiro == {"kind": "turn_end"} and update.get("stopReason") == "end_turn" and self.turn_completion and not self.turn_end:
                    self.turn_end, self.phase = True, "turn_end"
                    return None
                raise ValueError("Kiro ACP skill terminal status differs")
            raise ValueError("Kiro ACP skill flow emitted an unknown control update")
        if self._accept_auxiliary(record):
            return None
        if self._outer(record, {"jsonrpc", "id", "method", "params"}) and record.get("method") == "session/request_permission":
            if self.phase != "disclose_pending" or self.permission_id is not None:
                raise ValueError("Kiro ACP skill permission request is extra or reordered")
            permission_id = record.get("id")
            if type(permission_id) not in {int, str} or isinstance(permission_id, str) and not permission_id:
                raise ValueError("Kiro ACP skill permission request id has an unsupported type")
            params = record.get("params")
            if not isinstance(params, dict) or set(params) != {"sessionId", "toolCall", "options"} or params.get("sessionId") != self.session_id:
                raise ValueError("Kiro ACP skill permission envelope differs")
            call = params.get("toolCall")
            if not isinstance(call, dict) or call.get("toolCallId") != self.disclose_call_id or call.get("status") != "pending":
                raise ValueError("Kiro ACP skill permission target differs")
            options = params.get("options")
            if not isinstance(options, list) or [item.get("kind") for item in options if isinstance(item, dict)] != ["allow_once", "allow_always", "reject_once", "reject_always"]:
                raise ValueError("Kiro ACP skill permission options differ")
            if (
                any(set(item) != {"optionId", "name", "kind"} or not isinstance(item.get("optionId"), str) or not item["optionId"] for item in options)
                or len({item["optionId"] for item in options}) != len(options)
            ):
                raise ValueError("Kiro ACP skill permission options are malformed")
            self.permission_id, self.allow_once_id = permission_id, options[0]["optionId"]
            self.phase = "disclose_permitted"
            return {"jsonrpc": "2.0", "id": self.permission_id, "result": {"outcome": {"outcome": "selected", "optionId": self.allow_once_id}}}
        if self._outer(record, {"jsonrpc", "id", "result"}) and type(record.get("id")) is int and record.get("id") == 2:
            if self.phase != "turn_end" or record.get("result") != {"stopReason": "end_turn"}:
                raise ValueError("Kiro ACP skill prompt response differs")
            self.prompt_done, self.phase = True, "done"
            return None
        raise ValueError("Kiro ACP skill flow emitted an unknown control record")

    def complete(self) -> bool:
        return bool(
            self.phase == "done" and self.prompt_done and self.turn_end
            and self.skill_catalog_seen and self.search_output_seen
            and "".join(self.marker_parts) in {self.marker, f"`{self.marker}`"}
        )


def _acp_request(identifier: int, method: str, params: dict[str, Any]) -> bytes:
    return canonical_json({"jsonrpc": "2.0", "id": identifier, "method": method, "params": params}) + b"\n"


def run_kiro_acp(
    binary: Path, *, workspace: Path, environment: dict[str, str], plugin: str,
    tool: str, marker: str, skill_path: Path | None = None,
    timeout: int = COMMAND_SECONDS,
) -> tuple[dict[str, Any], str, str]:
    """Run the fixed Kiro ACP process with bounded newline JSON I/O."""
    started, deadline = utc_now(), time.monotonic() + timeout
    process = subprocess.Popen(
        [str(binary), *CLIENT_ARGUMENTS["kiro"]], cwd=workspace, env=environment,
        stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
        start_new_session=True, close_fds=True,
    )
    assert process.stdin is not None and process.stdout is not None
    contract: KiroACPContract | KiroACPSkillContract = (
        KiroACPSkillContract(marker, skill_path)
        if skill_path is not None else KiroACPContract(plugin, tool, marker)
    )
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
                    prompt = (
                        SKILL_PROBE_PROMPT
                        if skill_path is not None else
                        mcp_probe_prompt(plugin, tool, marker)
                    )
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
    if skill_path is not None:
        summary = {
            "protocol_version": KIRO_PROTOCOL_VERSION, "capability": "skill",
            "skill": SKILL_NAME, "tool": tool, "catalog_match": True,
            "permission": "allow_once", "target_chain": ["disclose", "search", "marker"],
            "turn_completion": "success", "turn_end": "end_turn",
        }
    else:
        assert isinstance(contract, KiroACPContract)
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


def write_skill_probe(workspace: Path, marker: str) -> Path:
    """Create one challenge-bound read-only probe inside the disposable workspace."""
    workspace_fd = os.open(workspace, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
    descriptor = -1
    try:
        info = os.fstat(workspace_fd)
        if info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) != 0o700:
            raise ValueError("skill probe workspace is not private")
        descriptor = os.open(
            "uap-skill-probe.txt",
            os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC,
            0o400,
            dir_fd=workspace_fd,
        )
        encoded = (marker + "\n").encode()
        view = memoryview(encoded)
        while view:
            written = os.write(descriptor, view)
            if written <= 0:
                raise OSError("skill probe write was incomplete")
            view = view[written:]
        os.fsync(descriptor)
        os.fsync(workspace_fd)
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        os.close(workspace_fd)
    return workspace / "uap-skill-probe.txt"


def invoke(
    item: dict[str, Any], plugin: str, client: str, challenge: str, workspace: Path,
    owner_uid: int, approved_tuple: dict[str, Any] | None = None,
    runtime_node: Path | None = None, runtime_browser: Path | None = None,
) -> tuple[dict[str, Any], list[str], str, str]:
    binary = verified_executable(item, owner_uid)
    profile = Path(item["profile"])
    if plugin == "chrome-devtools" and (
        runtime_node != NODE_BINARY or runtime_browser != CHROME_BINARY
    ):
        raise ValueError("chrome-devtools requires the verified fixed Node and browser runtimes")
    environment = runtime_environment(profile, runtime_node)
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
    component_kind = native_entry["component_kind"]
    expected_marker = (
        f"UAP_SKILL_SECRET_{challenge}" if component_kind == "skill"
        else f"UAP_OBSERVER_OK {client} {plugin} {challenge}"
    )
    tool = SKILL_PROBE_TOOL if component_kind == "skill" else MCP_PROBE_TOOLS[plugin]
    prompt = (
        SKILL_PROBE_PROMPT
        if component_kind == "skill" else
        mcp_probe_prompt(plugin, tool, expected_marker)
    )
    argv = [str(binary), *CLIENT_ARGUMENTS[client], prompt]
    native_path = Path(native_entry["client_config"]["path"])
    native_config_before = regular_snapshot(native_path, native_entry["client_config"]["sha256"], owner_uid=0, mode=0o440)
    if plugin == "chrome-devtools":
        validate_chrome_runtime_config(native_config_before["body"])
    if component_kind == "skill":
        write_skill_probe(workspace, expected_marker)
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
            skill_path=native_path if component_kind == "skill" else None,
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
        stdout, started, ended = run_client(
            argv, workspace=workspace, environment=environment,
            timeout=120 if client == "cursor" else COMMAND_SECONDS,
        )
        invocation = parsed_json_stream(stdout)
        succeeded = (
            successful_codex_skill_evidence(
                invocation, expected_marker, native_path, native_config_before["body"],
            )
            if component_kind == "skill" and client == "codex"
            else successful_cursor_skill_evidence(
                invocation, expected_marker, native_path, native_config_before["body"], workspace,
            )
            if component_kind == "skill" and client == "cursor"
            else successful_cursor_mcp_evidence(
                invocation, tool, plugin, expected_marker, workspace,
            )
            if component_kind == "mcp" and client == "cursor"
            else successful_client_evidence(client, invocation, tool, plugin, expected_marker)
        )
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
        "discovery_argv": [client, *(CLIENT_ARGUMENTS[client] if client == "kiro" else CLIENT_DISCOVERY_ARGUMENTS[client])],
        "tool": tool, "component_kind": component_kind,
        "invocation_marker_digest": sha256(expected_marker.encode()),
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
    runtime_node = None
    runtime_browser = None
    if plugin == "chrome-devtools":
        if mount_config is None:
            raise ValueError("chrome-devtools runtime requires the protected adapter config")
        runtime_node = verified_runtime_node(mount_config["clients"]["cursor"].get("bundle"), owner_uid)
        runtime_browser = verified_runtime_browser(mount_config.get("chrome_for_testing"), owner_uid)
    marker, argv, started, observed = invoke(
        client_config, plugin, client, challenge, workspace, owner_uid, item["tuple"],
        runtime_node, runtime_browser,
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
                "marker_digest": digest("invocation_marker_digest"), "succeeded": True,
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
