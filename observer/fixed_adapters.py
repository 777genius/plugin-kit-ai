#!/usr/bin/env python3
"""Fixed protected observation adapters; no request-selected executable or argv."""

from __future__ import annotations

import argparse
import hashlib
import json
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
FIXED_INPUT_PATHS = {
    "/opt/uap-observer-inputs/bin/git",
    "/opt/uap-observer-inputs/bin/codex",
    "/opt/uap-observer-inputs/bin/cursor",
    "/opt/uap-observer-inputs/bin/kiro",
    "/opt/uap-observer-inputs/chatgpt/app-binding.json",
    "/opt/uap-observer-inputs/chatgpt/projection-receipt.json",
    "/opt/uap-observer-inputs/external-pr-evidence.json",
}
PRIVACY_RESULT = {
    "real_project_accessed": False, "absolute_paths_exported": False,
    "credential_material_exported": False, "auth_copied": False,
    "enforcement": "systemd-positive-mount-allowlist-v1",
}
MAX_FILE = 4 << 20
MAX_STDOUT = 1 << 20
KILL_WAIT_SECONDS = 2.0
COMMAND_SECONDS = 45
HUMAN_WAIT_SECONDS = 300
MCP_ENDPOINT = "https://docs.mcp.cloudflare.com/mcp"
MCP_MARKER = "cloudflare-docs-read-only-v1"
MCP_READ_TOOL = "search_cloudflare_documentation"
MCP_READ_ARGUMENTS = {"query": "Cloudflare Durable Objects SQLite storage API marker cloudflare-docs-read-only-v1"}
CLIENT_ARGUMENTS = {
    "codex": ("exec", "--skip-git-repo-check", "--json"),
    "cursor": ("agent", "--print", "--output-format", "json"),
    "kiro": ("chat", "--no-interactive"),
}
CLIENT_DISCOVERY_ARGUMENTS = {
    "codex": ("mcp", "list", "--json"),
    "cursor": ("agent", "mcp", "list", "--json"),
    "kiro": ("mcp", "list", "--json"),
}
PROBE_TOOLS = {
    "agent-code-navigator": "search_code",
    "context7": "resolve-library-id",
    "cloudflare-docs": "search_cloudflare_documentation",
    "chrome-devtools": "list_pages",
    "notion": "search",
}


def canonical_json(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()


def sha256(value: bytes) -> str:
    return "sha256:" + hashlib.sha256(value).hexdigest()


def exported_json(value: Any) -> bytes:
    return (json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode()


def open_directory(path: Path, *, allowed_owners: set[int]) -> int:
    if not path.is_absolute() or ".." in path.parts:
        raise ValueError("protected directory path is invalid")
    descriptor = os.open("/", os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for component in path.parts[1:]:
            child = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=descriptor)
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
        if len(encoded) > MAX_FILE or (expected_digest is not None and sha256(encoded) != expected_digest):
            raise ValueError("protected file digest differs")
        return encoded
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        os.close(parent_fd)


def load_json(path: Path, digest: str, *, owner_uid: int, mode: int | None = None) -> dict[str, Any]:
    value = json.loads(read_regular(path, digest, owner_uid=owner_uid, mode=mode))
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
    required = {"schema_version", "request_policy", "git", "clients", "matrix", "consent_record", "chatgpt", "workspace_root", "external_pr_evidence"}
    if set(value) != required or value.get("schema_version") != 1:
        raise ValueError("adapter config is not canonical")
    if set(value.get("clients", {})) != CLIENTS:
        raise ValueError("adapter client allowlist differs")
    if any(value["clients"][client].get("client_id") != client for client in CLIENTS):
        raise ValueError("adapter client identity differs")
    if any(value["clients"][client].get("binary") != f"/opt/uap-observer-inputs/bin/{client}" for client in CLIENTS):
        raise ValueError("adapter client binary differs from its literal dedicated path")
    if any(value["clients"][client].get("profile") != f"/var/lib/uap-observer/profiles/{client}" for client in CLIENTS):
        raise ValueError("adapter client profile root differs")
    actual_paths = {
        value.get("git", {}).get("binary"),
        *(value["clients"][client].get("binary") for client in CLIENTS),
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
    if record.get("schema_version") != 1 or any(record.get(key) != expected_value for key, expected_value in expected.items()):
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
    fixed_paths = tuple(sorted(FIXED_INPUT_PATHS))
    allowed = (
        "/opt/uap-observer-current", "/usr/bin", "/usr/lib", "/usr/lib64",
        "/lib", "/lib64",
        "/etc/passwd", "/etc/group", "/etc/nsswitch.conf", "/etc/hosts", "/etc/ssl", "/etc/pki",
        "/var/lib/uap-observer/jobs", "/var/lib/uap-observer/workspaces", "/var/lib/uap-observer/profiles",
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
    if not isinstance(item, dict) or set(item) != {"binary", "sha256", "profile", "client_id"}:
        raise ValueError("fixed client config is invalid")
    binary = Path(item["binary"])
    verify_executable_file(binary, item["sha256"], owner_uid=owner_uid)
    profile = Path(item["profile"])
    profile_fd = open_directory(profile, allowed_owners={os.geteuid()})
    try:
        profile_info = os.fstat(profile_fd)
        if profile_info.st_uid != os.geteuid() or stat.S_IMODE(profile_info.st_mode) != 0o700:
            raise ValueError("fixed client profile is not runner-private")
    finally:
        os.close(profile_fd)
    return binary


def verified_git(item: Any, owner_uid: int) -> Path:
    if not isinstance(item, dict) or set(item) != {"binary", "sha256"}:
        raise ValueError("fixed Git config is invalid")
    binary = Path(item["binary"])
    verify_executable_file(binary, item["sha256"], owner_uid=owner_uid)
    if binary != Path("/usr/bin/git"):
        raise ValueError("fixed Git executable differs")
    return binary


def parsed_json_stream(encoded: bytes) -> list[Any]:
    try:
        value = json.loads(encoded)
        return value if isinstance(value, list) else [value]
    except json.JSONDecodeError:
        values = []
        for line in encoded.splitlines():
            if line.strip():
                values.append(json.loads(line))
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
    if value.get("enabled") is False or value.get("error") not in (None, "") or value.get("status") in {"failed", "disabled", "error"}:
        return False
    return any(value.get(field) == plugin for field in ("product_id", "plugin", "name", "id", "server", "server_name"))


def identity_in_collection(value: Any, plugin: str, collections: set[str]) -> bool:
    """Accept identities only in a typed inventory collection, never incidental text."""
    if not isinstance(value, dict):
        return False
    for key in collections:
        collection = value.get(key)
        if isinstance(collection, dict):
            if plugin in collection and isinstance(collection[plugin], (dict, bool, str, type(None))):
                return collection[plugin] is not False
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
            if (
                isinstance(candidate, dict) and (record_key == plugin or exact_identity_record(candidate, plugin))
                and _static_tuple(candidate.get("tuple")) == _static_tuple(approved_tuple)
            ):
                return candidate
    return None


def manager_receipt_present(value: Any, plugin: str, approved_tuple: dict[str, Any] | None = None) -> bool:
    if approved_tuple is None:
        return identity_in_collection(value, plugin, {"products", "receipts", "plugins", "installations", "entries"})
    return exact_observed_identity(value, plugin, {"products", "receipts", "plugins", "installations", "entries"}, approved_tuple) is not None


def native_discovery_present(value: Any, plugin: str, approved_tuple: dict[str, Any] | None = None) -> bool:
    values = value if isinstance(value, list) else [value]
    if approved_tuple is not None:
        return any(
            exact_observed_identity(item, plugin, {"servers", "mcp_servers", "mcpServers", "connections", "entries"}, approved_tuple) is not None
            for item in values
        )
    return any(
        exact_identity_record(item, plugin) or
        identity_in_collection(item, plugin, {"servers", "mcp_servers", "mcpServers", "connections", "entries"})
        for item in values
    )


def successful_tool_event(value: Any, tool: str, plugin: str) -> bool:
    if isinstance(value, list):
        return any(successful_tool_event(item, tool, plugin) for item in value)
    if not isinstance(value, dict):
        return False
    candidate = value.get("item") if value.get("type") in {"item.completed", "tool.completed"} else value
    if not isinstance(candidate, dict) or candidate.get("type") not in {"mcp_tool_call", "tool_call", "tool_result"}:
        return False
    names = {candidate.get(key) for key in ("name", "tool", "tool_name")}
    servers = {candidate.get(key) for key in ("server", "server_name", "mcp_server", "product_id")}
    error = candidate.get("error")
    payload = next((candidate.get(key) for key in ("result", "content", "output") if candidate.get(key) not in (None, "", [], {})), None)
    status = candidate.get("status")
    if tool in names and plugin in servers and error in (None, "") and (payload is not None or status in {"completed", "succeeded", "success"}):
        return True
    return False


def successful_marker_event(value: Any, expected_marker: str) -> bool:
    if isinstance(value, list):
        return any(successful_marker_event(item, expected_marker) for item in value)
    if not isinstance(value, dict):
        return False
    candidate = value.get("item") if value.get("type") in {"item.completed", "message.completed"} else value
    if isinstance(candidate, dict) and candidate.get("type") in {"agent_message", "assistant_message", "final_response"}:
        messages = [candidate.get(key) for key in ("text", "message", "output", "content")]
        if expected_marker in messages:
            return True
    return False


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
        "PATH": "/usr/bin:/bin", "LANG": "C.UTF-8", "LC_ALL": "C.UTF-8",
        "HOME": str(profile), "XDG_CONFIG_HOME": str(profile / ".config"),
        "XDG_CACHE_HOME": str(profile / ".cache"), "CODEX_HOME": str(profile / ".codex"),
    }
    manager_inventory = profile / ".agentplugins" / "receipts.json"
    profile_uid = os.geteuid()
    manager_before = json.loads(read_regular(manager_inventory, None, owner_uid=profile_uid, mode=0o600))
    if not manager_receipt_present(manager_before, plugin, approved_tuple):
        raise ValueError("manager receipt does not contain the exact approved release identity")
    version_stdout, _, _ = run_client([str(binary), "--version"], workspace=workspace, environment=environment, timeout=10)
    if len(version_stdout) > 4096:
        raise ValueError("fixed client version observation failed")
    client_version = version_stdout.decode("utf-8", "strict").strip()
    if not client_version or any(character in client_version for character in "\r\n/\\"):
        raise ValueError("fixed client version marker is invalid")
    discovery_argv = [str(binary), *CLIENT_DISCOVERY_ARGUMENTS[client]]
    native_before_bytes, _, _ = run_client(discovery_argv, workspace=workspace, environment=environment, timeout=10)
    native_before = parsed_json_stream(native_before_bytes)
    if not native_discovery_present(native_before, plugin, approved_tuple):
        raise ValueError("native discovery did not contain the exact approved release identity")
    stdout, started, ended = run_client(argv, workspace=workspace, environment=environment)
    invocation = parsed_json_stream(stdout)
    if not successful_marker_event(invocation, expected_marker) or not successful_tool_event(invocation, tool, plugin):
        raise ValueError("fixed client did not emit a successful exact tool invocation")
    native_after_bytes, _, _ = run_client(discovery_argv, workspace=workspace, environment=environment, timeout=10)
    native_after = parsed_json_stream(native_after_bytes)
    if not native_discovery_present(native_after, plugin, approved_tuple):
        raise ValueError("native discovery disappeared after invocation")
    manager_after_bytes = read_regular(manager_inventory, None, owner_uid=profile_uid, mode=0o600)
    manager_after = json.loads(manager_after_bytes)
    if not manager_receipt_present(manager_after, plugin, approved_tuple):
        raise ValueError("manager receipt disappeared after invocation")
    marker = {
        "client_version": client_version, "client_id": item["client_id"],
        "manager_before_digest": sha256(canonical_json(manager_before)), "manager_after_digest": sha256(canonical_json(manager_after)),
        "native_before_digest": sha256(canonical_json(native_before)), "native_after_digest": sha256(canonical_json(native_after)),
        "discovery_argv": [client, *CLIENT_DISCOVERY_ARGUMENTS[client]], "tool": tool,
    }
    return marker, argv, started, ended


def complete_tuple(item: dict[str, Any], marker: dict[str, Any], observed_at: str) -> dict[str, Any]:
    value = item.get("tuple")
    required = {
        "product_id", "tree_digest", "manifest_digest", "distribution_id", "distribution_kind",
        "release_sequence", "package_version", "source_repository", "source_revision", "source_path",
        "snapshot_sequence", "snapshot_digest", "binary_digest", "dependency_identity", "installer_version",
        "adapter_version", "client_version", "os", "architecture", "observed_at",
    }
    if not isinstance(value, dict) or set(value) != required:
        raise ValueError("fixed client tuple is incomplete")
    value = dict(value)
    if value["product_id"] != item["plugin"]:
        raise ValueError("fixed client tuple product differs")
    value["client_version"] = marker.get("client_version")
    value["observed_at"] = observed_at
    if not isinstance(value["client_version"], str) or not value["client_version"]:
        raise ValueError("fixed client version marker is absent")
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


def mcp_call(endpoint: str, request_id: int, method: str, params: dict[str, Any], session: str | None = None) -> tuple[dict[str, Any], str | None]:
    payload = canonical_json({"jsonrpc": "2.0", "id": request_id, "method": method, "params": params})
    headers = {"Content-Type": "application/json", "Accept": "application/json, text/event-stream", "MCP-Protocol-Version": "2025-06-18"}
    if session:
        headers["Mcp-Session-Id"] = session
    request = urllib.request.Request(endpoint, data=payload, headers=headers, method="POST")
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    with opener.open(request, timeout=15) as response:
        if response.status != 200 or response.geturl() != endpoint:
            raise ValueError("Cloudflare MCP response identity differs")
        encoded = response.read(MAX_STDOUT + 1)
        session = response.headers.get("Mcp-Session-Id") or session
    if len(encoded) > MAX_STDOUT:
        raise ValueError("Cloudflare MCP response exceeds size bound")
    text = encoded.decode()
    if text.startswith("event:") or "\ndata:" in text:
        data_lines = [line[5:].strip() for line in text.splitlines() if line.startswith("data:")]
        text = data_lines[-1] if data_lines else ""
    value = json.loads(text)
    if not isinstance(value, dict) or value.get("jsonrpc") != "2.0" or value.get("id") != request_id or "error" in value or not isinstance(value.get("result"), dict):
        raise ValueError("Cloudflare MCP response is invalid")
    return value["result"], session


def mcp_initialized(endpoint: str, session: str) -> None:
    payload = canonical_json({"jsonrpc": "2.0", "method": "notifications/initialized"})
    request = urllib.request.Request(
        endpoint, data=payload, method="POST",
        headers={"Content-Type": "application/json", "Accept": "application/json, text/event-stream", "MCP-Protocol-Version": "2025-06-18", "Mcp-Session-Id": session},
    )
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    with opener.open(request, timeout=15) as response:
        if response.status not in {200, 202, 204} or response.geturl() != endpoint or len(response.read(4097)) > 4096:
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
    value = json.loads(encoded)
    expected = {
        "schema_version": 1, "challenge": challenge, "run_id": request["github"]["run_id"],
        "run_attempt": request["github"]["run_attempt"], "app_id": config["chatgpt"]["app_id"],
        "request_digest": sha256(canonical_json(request)), "mcp_url": MCP_ENDPOINT,
        "consent": True, "ui_activation": True, "runtime_observed": True, "read_only": True,
        "no_secrets": True, "no_real_project": True,
    }
    if not isinstance(value, dict) or set(value) != {*expected, "observed_at", "expires_at"} or any(value.get(key) != expected_value for key, expected_value in expected.items()):
        raise ValueError("human ChatGPT attestation is invalid")
    observed = datetime.fromisoformat(str(value["observed_at"]).replace("Z", "+00:00"))
    expires = datetime.fromisoformat(str(value["expires_at"]).replace("Z", "+00:00"))
    if observed.tzinfo is None or expires.tzinfo is None or not observed.timestamp() <= time.time() <= expires.timestamp() or expires.timestamp() - observed.timestamp() > 900:
        raise ValueError("human ChatGPT attestation is stale")
    return value


def chatgpt_artifact(config: dict[str, Any], request: dict[str, Any], github: dict[str, Any], consent: dict[str, Any], owner_uid: int) -> dict[str, Any]:
    chat = config["chatgpt"]
    binding_path = Path(chat["app_binding_path"])
    if binding_path != Path("/opt/uap-observer-inputs/chatgpt/app-binding.json"):
        raise ValueError("ChatGPT app binding path is not exact")
    binding = load_json(binding_path, chat["app_binding_sha256"], owner_uid=owner_uid, mode=0o640)
    if binding != {"apps": {"cloudflare-docs": {"id": chat["app_id"]}}} or chat["mcp_endpoint"] != MCP_ENDPOINT:
        raise ValueError("ChatGPT app binding differs")
    receipt = load_json(Path(chat["projection_receipt_path"]), chat["projection_receipt_sha256"], owner_uid=owner_uid, mode=0o640)
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
    if result.get("isError") is True or not isinstance(content, list) or not content or not all(substantive_mcp_content(item) for item in content):
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
    context = json.loads(read_regular(args.context, None, owner_uid=os.geteuid(), mode=0o600))
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
