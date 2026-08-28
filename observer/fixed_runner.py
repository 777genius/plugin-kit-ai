#!/usr/bin/env python3
"""Reviewed fixed-adapter runner, isolated from observer state and signing."""

from __future__ import annotations

import argparse
import grp
import hashlib
import json
import math
import os
import pwd
import re
import shutil
import signal
import socket
import struct
import subprocess
import tempfile
import time
import stat
import secrets
import select
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable

from observer.client_bundle import verify_bundle
from observer.secure_files import IMMUTABLE_CLOSURE_ALIAS, read_immutable_closure_regular

MAX_MESSAGE = 8 << 20
MAX_ADAPTER_OUTPUT = 4 << 20
RUNNER_TOTAL_SECONDS = 840
KILL_WAIT_SECONDS = 2.0
CGROUP_EMPTY_WAIT_SECONDS = 5.0
ARTIFACT_NAMES = {
    "runtime-attestations.json", "notion-oauth-attestations.json",
    "chatgpt-cloudflare-attestation.json", "consent.json",
}
ARTIFACT_ORDER = (
    "runtime-attestations.json", "notion-oauth-attestations.json",
    "chatgpt-cloudflare-attestation.json", "consent.json",
)
ARTIFACT_IDENTITIES = {
    "runtime-attestations.json": ("codex", "cursor", "kiro"),
    "notion-oauth-attestations.json": ("codex", "cursor", "kiro"),
    "chatgpt-cloudflare-attestation.json": ("control",),
    "consent.json": ("control",),
}
RUNTIME_HEROES = {
    "agent-code-navigator", "context7", "cloudflare-docs", "chrome-devtools", "notion",
}
FIXED_CONFIG = Path("/opt/uap-observer-current/etc/uap-observer-adapter-config.json")
FIXED_ENTRYPOINTS = {
        artifact: Path(f"/opt/uap-observer-current/libexec/uap-observer-adapter-{name}")
    for artifact, name in {
        "runtime-attestations.json": "runtime", "notion-oauth-attestations.json": "notion",
        "chatgpt-cloudflare-attestation.json": "chatgpt", "consent.json": "consent",
    }.items()
}

SERVICE_ROLES = {
    "caddy": ("caddy", "caddy", "/var/lib/caddy", ()),
    "egress": ("uap-observer-egress", "uap-observer-egress", "/nonexistent", ()),
    "observer": (
        "uap-observer", "uap-observer", "/nonexistent",
        ("uap-observer-signer-ipc", "uap-observer-runner-ipc"),
    ),
    **{
        identity: (
            f"uap-observer-{identity}", f"uap-observer-{identity}",
            f"/var/empty/uap-observer-{identity}",
            ("uap-observer-adapter-config",),
        )
        for identity in ("codex", "cursor", "kiro", "control")
    },
}


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

    return json.loads(encoded, object_pairs_hook=object_from_pairs,
                      parse_constant=reject_constant, parse_float=finite_float)


def reviewed_service_identities() -> dict[str, tuple[int, int, frozenset[int]]]:
    """Resolve the exact, alias-free NSS identities used by observer services."""
    required_groups = {
        group_name
        for _, group_name, _, supplemental in SERVICE_ROLES.values()
        for group_name in (group_name, *supplemental)
    }
    groups = {name: grp.getgrnam(name) for name in required_groups}
    if any(group.gr_gid == 0 for group in groups.values()):
        raise ValueError("reviewed service group uses the root identity")
    if len({group.gr_gid for group in groups.values()}) != len(groups):
        raise ValueError("reviewed service groups are not distinct")
    all_groups = grp.getgrall()
    for name, group in groups.items():
        if grp.getgrgid(group.gr_gid).gr_name != name:
            raise ValueError("reviewed service group lookup is not canonical")
        aliases = {item.gr_name for item in all_groups if item.gr_gid == group.gr_gid}
        if aliases != {name}:
            raise ValueError("reviewed service group has an NSS alias")

    result: dict[str, tuple[int, int, frozenset[int]]] = {}
    all_users = pwd.getpwall()
    for role, (name, group_name, home, supplemental) in SERVICE_ROLES.items():
        user = pwd.getpwnam(name)
        expected_group_ids = frozenset(
            groups[item].gr_gid for item in (group_name, *supplemental)
        )
        actual_group_ids = frozenset(os.getgrouplist(name, user.pw_gid))
        if (
            user.pw_uid == 0 or user.pw_gid != groups[group_name].gr_gid
            or user.pw_dir != home
            or user.pw_shell not in {"/usr/sbin/nologin", "/sbin/nologin", "/bin/false"}
            or actual_group_ids != expected_group_ids
        ):
            raise ValueError(f"reviewed service account {name} differs")
        if pwd.getpwuid(user.pw_uid).pw_name != name:
            raise ValueError("reviewed service UID lookup is not canonical")
        aliases = {item.pw_name for item in all_users if item.pw_uid == user.pw_uid}
        if aliases != {name}:
            raise ValueError("reviewed service account has an NSS alias")
        result[role] = (user.pw_uid, user.pw_gid, actual_group_ids)
    if len({uid for uid, _, _ in result.values()}) != len(result):
        raise ValueError("reviewed service UIDs are not distinct")

    supplemental_members = {name: set() for name in required_groups}
    supplemental_members.update({
        "uap-observer-adapter-config": {
            "uap-observer-codex", "uap-observer-cursor",
            "uap-observer-kiro", "uap-observer-control",
        },
        "uap-observer-signer-ipc": {"uap-observer"},
        "uap-observer-runner-ipc": {"uap-observer"},
    })
    for name, expected in supplemental_members.items():
        if set(groups[name].gr_mem) != expected:
            raise ValueError("reviewed supplemental group membership differs")
    return result


def _probe_adapter_identity_access(
    uid: int, gid: int, gids: frozenset[int], paths: tuple[tuple[Path, bool], ...],
    *, timeout: float = 2.0,
) -> None:
    """Use the kernel's DAC/ACL checks under the exact service credentials."""
    read_fd, write_fd = os.pipe2(os.O_CLOEXEC | os.O_NONBLOCK)
    child = os.fork()
    if child == 0:
        os.close(read_fd)
        try:
            os.setgroups(sorted(gids))
            os.setresgid(gid, gid, gid)
            os.setresuid(uid, uid, uid)
            try:
                os.setuid(0)
            except PermissionError:
                pass
            else:
                raise PermissionError("adapter probe retained privilege-regain capability")
            directory_flags = _identity_directory_open_flags(getattr(os, "O_PATH", 0))
            for path, executable in paths:
                descriptor = os.open("/", directory_flags)
                try:
                    for component in path.parts[1:-1]:
                        child_fd = os.open(
                            component, directory_flags,
                            dir_fd=descriptor,
                        )
                        os.close(descriptor)
                        descriptor = child_fd
                    file_fd = os.open(
                        path.name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC,
                        dir_fd=descriptor,
                    )
                    os.close(file_fd)
                    if executable and not os.access(path, os.R_OK | os.X_OK):
                        raise PermissionError(f"cannot execute {path}")
                finally:
                    os.close(descriptor)
        except BaseException as error:
            try:
                os.write(write_fd, str(error).encode("utf-8", "replace")[:4096])
            finally:
                os._exit(1)
        os._exit(0)
    os.close(write_fd)
    deadline = time.monotonic() + timeout
    status: int | None = None
    try:
        while time.monotonic() < deadline:
            completed, value = os.waitpid(child, os.WNOHANG)
            if completed:
                status = value
                break
            select.select([read_fd], [], [], min(0.02, max(0.0, deadline - time.monotonic())))
        if status is None:
            os.kill(child, signal.SIGKILL)
            _, status = os.waitpid(child, 0)
            raise ValueError("adapter input accessibility probe timed out")
        error = os.read(read_fd, 4096).decode("utf-8", "replace")
        if not os.WIFEXITED(status) or os.WEXITSTATUS(status) != 0:
            raise ValueError(f"adapter input is not accessible to exact identity: {error}")
    finally:
        os.close(read_fd)


def _identity_directory_open_flags(o_path: int) -> int:
    """Select search-only traversal when the platform exposes O_PATH."""
    access_mode = o_path if o_path else os.O_RDONLY
    return access_mode | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC


def validate_adapter_input_access(
    config_path: Path, *, protected_root: Path = Path("/opt/uap-observer-inputs"),
    identities: dict[str, tuple[int, int, frozenset[int]]] | None = None,
    access_probe: Callable[[int, int, frozenset[int], tuple[tuple[Path, bool], ...]], None] = _probe_adapter_identity_access,
) -> tuple[Path, ...]:
    """Validate immutable inputs and prove each adapter identity can access them."""
    identities = identities or reviewed_service_identities()
    adapters = {name: identities[name] for name in ("codex", "cursor", "kiro", "control")}
    config = strict_json_loads(config_path.read_text(encoding="utf-8"))
    config_gid = grp.getgrnam("uap-observer-adapter-config").gr_gid
    cursor = config["clients"]["cursor"]
    bundle = cursor.get("bundle")
    expected_bundle = {
        "root": str(protected_root / "cursor"),
        "manifest": str(protected_root / "cursor-bundle.json"),
    }
    if (
        not isinstance(bundle, dict)
        or set(bundle) != {"root", "manifest", "manifest_sha256"}
        or any(bundle.get(key) != value for key, value in expected_bundle.items())
    ):
        raise ValueError("protected Cursor bundle config differs")
    bundle_root = Path(bundle["root"])
    bundle_manifest = Path(bundle["manifest"])
    bundle_files = verify_bundle(
        root=bundle_root,
        manifest=bundle_manifest,
        manifest_sha256=bundle["manifest_sha256"],
    )
    required_cursor_runtime = {
        bundle_root / name
        for name in ("cursor-agent", "node", "bash", "basename", "dirname", "realpath")
    }
    if Path(cursor["binary"]) not in bundle_files or not required_cursor_runtime.issubset(bundle_files):
        raise ValueError("protected Cursor runtime closure is absent from its bundle")
    cursor_digest = "sha256:" + hashlib.sha256(Path(cursor["binary"]).read_bytes()).hexdigest()
    if cursor_digest != cursor["sha256"]:
        raise ValueError("protected Cursor executable digest differs")

    chrome = config.get("chrome_for_testing")
    expected_chrome = {
        "root": str(protected_root / "chrome-for-testing"),
        "manifest": str(protected_root / "chrome-for-testing-bundle.json"),
        "binary": str(protected_root / "chrome-for-testing/chrome"),
        "version": "152.0.7977.64",
    }
    if (
        not isinstance(chrome, dict)
        or set(chrome) != {*expected_chrome, "manifest_sha256", "binary_sha256"}
        or any(chrome.get(key) != value for key, value in expected_chrome.items())
    ):
        raise ValueError("protected Chrome for Testing bundle config differs")
    chrome_root = Path(chrome["root"])
    chrome_manifest = Path(chrome["manifest"])
    chrome_files = verify_bundle(
        root=chrome_root,
        manifest=chrome_manifest,
        manifest_sha256=chrome["manifest_sha256"],
    )
    chrome_binary = Path(chrome["binary"])
    if chrome_binary not in chrome_files:
        raise ValueError("protected Chrome executable is absent from its bundle")
    chrome_digest = "sha256:" + hashlib.sha256(chrome_binary.read_bytes()).hexdigest()
    if chrome_digest != chrome["binary_sha256"]:
        raise ValueError("protected Chrome executable digest differs")

    expected_files = {
        Path(config["git"]["binary"]): config["git"]["sha256"],
        **{
            Path(config["clients"][name]["binary"]): config["clients"][name]["sha256"]
            for name in ("codex", "kiro")
        },
        Path(config["clients"]["codex"]["companion_binary"]): config["clients"]["codex"]["companion_sha256"],
        Path(config["clients"]["kiro"]["companion_binary"]): config["clients"]["kiro"]["companion_sha256"],
        Path(config["chatgpt"]["app_binding_path"]): config["chatgpt"]["app_binding_sha256"],
        Path(config["chatgpt"]["projection_receipt_path"]): config["chatgpt"]["projection_receipt_sha256"],
        Path(config["external_pr_evidence"]["path"]): config["external_pr_evidence"]["sha256"],
    }
    literal = (
        {protected_root / "bin" / name for name in ("git", "codex", "codex-code-mode-host", "kiro", "kiro-cli-chat")}
        | {protected_root / "chatgpt" / name for name in ("app-binding.json", "projection-receipt.json")}
        | {protected_root / "external-pr-evidence.json"}
    )
    if set(expected_files) != literal:
        raise ValueError("protected input paths differ")
    cursor_tree = set(bundle_root.rglob("*")) | {bundle_root}
    bundle_dirs = {path for path in cursor_tree if path.is_dir()}
    bundle_paths = {path for path in cursor_tree if path.is_file()}
    chrome_tree = set(chrome_root.rglob("*")) | {chrome_root}
    chrome_dirs = {path for path in chrome_tree if path.is_dir()}
    chrome_paths = {path for path in chrome_tree if path.is_file()}
    if bundle_paths != set(bundle_files) or chrome_paths != set(chrome_files):
        raise ValueError("protected bundle inventory differs")
    expected_dirs = {
        protected_root, protected_root / "bin", protected_root / "chatgpt",
    } | bundle_dirs | chrome_dirs
    actual = set(protected_root.rglob("*")) | {protected_root}
    if actual != expected_dirs | literal | bundle_paths | chrome_paths | {bundle_manifest, chrome_manifest}:
        raise ValueError("protected input tree contains an unexpected path")

    for directory in expected_dirs:
        if directory.resolve(strict=True) != directory:
            raise ValueError("protected input path traverses a symlink")
        info = os.lstat(directory)
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
            raise ValueError("protected input directory is not root-controlled")
    for path, digest in expected_files.items():
        if path.resolve(strict=True) != path:
            raise ValueError("protected input path traverses a symlink")
        info = os.lstat(path)
        executable = path.parent == protected_root / "bin"
        expected_mode = 0o755 if executable else 0o640
        if (
            not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_nlink != 1
            or stat.S_IMODE(info.st_mode) != expected_mode
            or (not executable and info.st_gid != config_gid)
        ):
            raise ValueError("protected input metadata differs")
        actual_digest = "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()
        if actual_digest != digest:
            raise ValueError("protected input digest differs")
    probes = tuple(
        [(path, path.parent == protected_root / "bin") for path in sorted(expected_files)]
        + [(bundle_manifest, False)]
        + [(path, bool(os.lstat(path).st_mode & stat.S_IXUSR)) for path in bundle_files]
        + [(chrome_manifest, False)]
        + [(path, bool(os.lstat(path).st_mode & stat.S_IXUSR)) for path in chrome_files]
    )
    for uid, gid, gids in adapters.values():
        access_probe(uid, gid, gids, probes)
    return tuple(sorted(literal | {bundle_root, bundle_manifest, chrome_root, chrome_manifest}))


def canonical_json(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()


def validate_artifacts(value: Any) -> dict[str, Any]:
    if not isinstance(value, dict) or set(value) != ARTIFACT_NAMES or not all(isinstance(item, dict) for item in value.values()):
        raise ValueError("adapter artifact set is not canonical")
    return value


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


def read_owned_regular(
    path: Path, limit: int, *, owner_uid: int, executable: bool = False,
    exact_mode: int | None = None, group_gid: int | None = None,
    expected_nlink: int = 1,
) -> bytes:
    try:
        relative = path.relative_to(IMMUTABLE_CLOSURE_ALIAS)
    except ValueError:
        relative = None
    if relative is not None:
        return read_immutable_closure_regular(
            relative, limit, owner_uid=owner_uid, executable=executable,
            exact_mode=exact_mode, group_gid=group_gid,
            expected_nlink=expected_nlink,
        )
    parent_fd = open_directory(path.parent, allowed_owners={owner_uid, os.geteuid()})
    descriptor = -1
    try:
        descriptor = os.open(path.name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC, dir_fd=parent_fd)
        info = os.fstat(descriptor)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != owner_uid or info.st_mode & 0o022:
            raise ValueError("protected file is not trusted")
        if info.st_nlink != expected_nlink:
            raise ValueError("protected file hardlink count differs")
        if exact_mode is not None and stat.S_IMODE(info.st_mode) != exact_mode:
            raise ValueError("protected file mode differs")
        if group_gid is not None and info.st_gid != group_gid:
            raise ValueError("protected file group differs")
        if executable and not info.st_mode & stat.S_IXUSR:
            raise ValueError("protected executable is not executable")
        if info.st_size > limit:
            raise ValueError("protected file exceeds size bound")
        chunks: list[bytes] = []
        remaining = limit + 1
        while remaining:
            chunk = os.read(descriptor, remaining)
            if not chunk:
                break
            chunks.append(chunk)
            remaining -= len(chunk)
        value = b"".join(chunks)
        if len(value) > limit:
            raise ValueError("protected file exceeds size bound")
        return value
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        os.close(parent_fd)


def write_new_owned(path: Path, value: bytes, *, uid: int | None = None, gid: int | None = None) -> None:
    parent_fd = open_directory(path.parent, allowed_owners={os.geteuid()})
    descriptor = -1
    try:
        descriptor = os.open(path.name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600, dir_fd=parent_fd)
        view = memoryview(value)
        while view:
            view = view[os.write(descriptor, view):]
        if uid is not None and gid is not None:
            os.fchown(descriptor, uid, gid)
        os.fsync(descriptor)
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        os.close(parent_fd)


def revalidate_client_proofs(
    config_path: Path, config_digest: str, client: str, uid: int, gid: int,
) -> None:
    """Revalidate sealed proof and native bytes after the job cgroup is empty."""
    encoded_config = read_owned_regular(config_path, 4 << 20, owner_uid=0, exact_mode=0o640)
    if "sha256:" + hashlib.sha256(encoded_config).hexdigest() != config_digest:
        raise ValueError("adapter config changed during client observation")
    config = strict_json_loads(encoded_config)
    item = config.get("clients", {}).get(client)
    if not isinstance(item, dict):
        raise ValueError("adapter client proof config is absent")
    profile = Path(str(item.get("profile", "")))
    projection_item = item.get("native_projection")
    if not isinstance(projection_item, dict) or set(projection_item) != {"path", "sha256"}:
        raise ValueError("adapter client projection config differs")
    projection_path = Path(str(projection_item["path"]))
    projection_body = read_owned_regular(
        projection_path, 4 << 20, owner_uid=0, exact_mode=0o440, group_gid=gid,
    )
    if "sha256:" + hashlib.sha256(projection_body).hexdigest() != projection_item["sha256"]:
        raise ValueError("sealed client projection changed after observation")
    receipt = read_owned_regular(
        projection_path.with_name("receipts.json"), 4 << 20,
        owner_uid=0, exact_mode=0o440, group_gid=gid,
    )
    receipt_value = strict_json_loads(receipt)
    if not isinstance(receipt_value, dict) or set(receipt_value) != {"schema_version", "receipts"} or type(receipt_value.get("schema_version")) is not int or receipt_value.get("schema_version") != 1:
        raise ValueError("sealed client receipt differs after observation")
    projection = strict_json_loads(projection_body)
    if (
        not isinstance(projection, dict)
        or set(projection) != {"schema_version", "client_id", "entries"}
        or type(projection.get("schema_version")) is not int
        or projection.get("schema_version") != 2
    ):
        raise ValueError("sealed client projection schema differs")
    entries = projection["entries"]
    entry_fields = {
        "plugin", "component_kind", "tuple", "native_config", "client_config",
        "manager_add_sha256", "manager_info_sha256", "post_add_doctor_sha256",
    }
    if (
        not isinstance(entries, list) or projection.get("client_id") != client
        or len(entries) != len(RUNTIME_HEROES)
        or any(not isinstance(entry, dict) or set(entry) != entry_fields for entry in entries)
        or {entry["plugin"] for entry in entries} != RUNTIME_HEROES
        or len({entry["plugin"] for entry in entries}) != len(entries)
        or any(
            entry["component_kind"]
            != ("skill" if entry["plugin"] == "agent-code-navigator" else "mcp")
            for entry in entries
        )
    ):
        raise ValueError("sealed client projection identity differs")
    receipts = receipt_value["receipts"]
    evidence_fields = {"manager_add_sha256", "manager_info_sha256", "post_add_doctor_sha256"}
    if (
        not isinstance(receipts, list) or len(receipts) != len(entries)
        or any(not isinstance(record, dict) or set(record) != {"name", "tuple", *evidence_fields} for record in receipts)
        or len({record["name"] for record in receipts}) != len(receipts)
    ):
        raise ValueError("sealed manager evidence receipt differs")
    receipts_by_name = {record["name"]: record for record in receipts}
    active_groups: dict[Path, list[dict[str, Any]]] = {}
    for entry in entries:
        record = receipts_by_name.get(entry.get("plugin")) if isinstance(entry, dict) else None
        if (
            record is None or entry.get("tuple") != record.get("tuple")
            or any(
                entry.get(field) != record.get(field)
                or re.fullmatch(r"sha256:[a-f0-9]{64}", str(entry.get(field, ""))) is None
                for field in evidence_fields
            )
        ):
            raise ValueError("sealed manager evidence binding differs")
        native = entry.get("native_config") if isinstance(entry, dict) else None
        client_native = entry.get("client_config") if isinstance(entry, dict) else None
        if not isinstance(native, dict) or set(native) != {"path", "sha256"} or not isinstance(client_native, dict) or set(client_native) != {"path", "sha256"} or native["sha256"] != client_native["sha256"]:
            raise ValueError("sealed native config proof differs")
        proof_native_path = Path(str(native["path"]))
        expected_proof = projection_path.parent / "native" / f'{entry["plugin"]}.blob'
        if proof_native_path != expected_proof:
            raise ValueError("sealed native config proof path differs")
        proof_body = read_owned_regular(proof_native_path, 4 << 20, owner_uid=0, exact_mode=0o440, group_gid=gid)
        native_path = Path(str(client_native["path"]))
        if not native_path.is_absolute() or profile not in native_path.parents:
            raise ValueError("sealed active native config path escapes its profile")
        active_groups.setdefault(native_path, []).append(entry)
        skill_suffix = native_path.parts[-3:] == ("skills", "code-tool-router", "SKILL.md")
        if entry["component_kind"] == "skill":
            if not skill_suffix or not proof_body.strip():
                raise ValueError("sealed active skill config differs")
        else:
            if skill_suffix:
                raise ValueError("sealed MCP config aliases the skill path")
            decoded = strict_json_loads(proof_body)
            if not isinstance(decoded, dict):
                raise ValueError("sealed MCP config is not a JSON object")
        body = read_owned_regular(native_path, 4 << 20, owner_uid=0, exact_mode=0o440, group_gid=gid)
        if body != proof_body or "sha256:" + hashlib.sha256(body).hexdigest() != native["sha256"]:
            raise ValueError("native config changed after all client processes terminated")
    duplicates = [group for group in active_groups.values() if len(group) > 1]
    if client == "kiro":
        shared = profile / ".kiro" / "settings" / "mcp.json"
        skill = profile / ".kiro" / "skills" / "code-tool-router" / "SKILL.md"
        if (
            set(active_groups) != {shared, skill}
            or len(duplicates) != 1 or len(duplicates[0]) != len(RUNTIME_HEROES) - 1
            or {entry["plugin"] for entry in duplicates[0]}
            != RUNTIME_HEROES - {"agent-code-navigator"}
            or any(entry["component_kind"] != "mcp" for entry in duplicates[0])
            or len({entry["client_config"]["sha256"] for entry in duplicates[0]}) != 1
        ):
            raise ValueError("sealed client projection has conflicting active configs")
    elif duplicates:
        raise ValueError("sealed client projection has conflicting active configs")


def kill_process_group(
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
            raise RuntimeError("fatal runner cleanup callback returned")


def delegated_job_cgroup(
    cgroup_root: Path = Path("/sys/fs/cgroup"), cgroup_file: Path = Path("/proc/self/cgroup"),
) -> Path:
    controllers = cgroup_root / "cgroup.controllers"
    if not controllers.is_file():
        raise ValueError("reviewed runner requires delegated cgroup v2")
    relative: str | None = None
    for line in cgroup_file.read_text().splitlines():
        if line.startswith("0::/"):
            relative = line[3:]
            break
    if relative is None or not relative.startswith("/") or ".." in Path(relative).parts:
        raise ValueError("reviewed runner cgroup identity is invalid")
    # cgroup paths are namespace-absolute.  Joining an absolute Path would
    # silently discard cgroup_root, so make it explicitly root-relative.
    parent = cgroup_root.joinpath(*Path(relative).parts[1:])
    if parent != cgroup_root and cgroup_root not in parent.parents:
        raise ValueError("reviewed runner cgroup escaped the cgroup v2 mount")
    target = parent / f"uap-job-{os.getpid()}-{secrets.token_hex(8)}"
    target.mkdir(mode=0o700)
    return target


def destroy_job_cgroup(
    target: Path, *, kill: Any | None = None, events: Any | None = None,
    remove: Any | None = None, wait_seconds: float = CGROUP_EMPTY_WAIT_SECONDS,
    fatal: Any = os._exit,
) -> None:
    kill = kill or (lambda: (target / "cgroup.kill").write_text("1"))
    events = events or (lambda: (target / "cgroup.events").read_text())
    remove = remove or target.rmdir
    kill()
    deadline = time.monotonic() + wait_seconds
    while True:
        if "populated 0" in events():
            break
        if time.monotonic() >= deadline:
            fatal(70)
            raise RuntimeError("fatal runner cleanup callback returned")
        time.sleep(min(0.01, max(0.0, deadline - time.monotonic())))
    try:
        remove()
    except OSError:
        # Once cgroup.events proves the group empty, removal is advisory.
        pass


def finalize_client_job(
    job_cgroup: Path | None, revalidate: Any | None, primary_error: BaseException | None,
) -> None:
    """Empty the job cgroup, then fail closed on descriptor-bound proof drift."""
    cleanup_error: BaseException | None = None
    try:
        if job_cgroup is not None:
            destroy_job_cgroup(job_cgroup)
    except BaseException as error:
        cleanup_error = error
    try:
        if revalidate is not None:
            revalidate()
    except BaseException as revalidation_error:
        if primary_error is not None or cleanup_error is not None:
            raise ValueError("client proof revalidation failed after adapter termination") from (primary_error or cleanup_error)
        raise revalidation_error
    if cleanup_error is not None:
        raise cleanup_error


def transition_record(root: Path, challenge: str, action: str) -> None:
    """Reserve, commit, or roll back a one-use root-created challenge record."""
    if not re.fullmatch(r"[a-f0-9]{64}", challenge):
        raise ValueError("challenge record identity is invalid")
    if action not in {"reserve", "commit", "rollback"}:
        raise ValueError("challenge transaction action is invalid")
    directories = {name: open_directory(root / name, allowed_owners={0}) for name in ("pending", "reserved", "consumed")}
    try:
        source = f"{challenge}.json"
        if action == "reserve":
            source_name, destination_name = "pending", "reserved"
            try:
                os.stat(source, dir_fd=directories["consumed"], follow_symlinks=False)
            except FileNotFoundError:
                pass
            else:
                raise ValueError("challenge record was already consumed")
        elif action == "commit":
            source_name, destination_name = "reserved", "consumed"
            try:
                os.stat(source, dir_fd=directories["consumed"], follow_symlinks=False)
            except FileNotFoundError:
                pass
            else:
                return
        else:
            source_name, destination_name = "reserved", "pending"
            try:
                os.stat(source, dir_fd=directories["reserved"], follow_symlinks=False)
            except FileNotFoundError:
                return
        info = os.stat(source, dir_fd=directories[source_name], follow_symlinks=False)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or stat.S_IMODE(info.st_mode) != 0o640 or info.st_nlink != 1:
            raise ValueError("challenge record is not trusted")
        os.rename(source, source, src_dir_fd=directories[source_name], dst_dir_fd=directories[destination_name])
        os.fsync(directories[source_name])
        os.fsync(directories[destination_name])
    finally:
        for descriptor in directories.values():
            os.close(descriptor)


def transition_records(challenge: str, action: str) -> None:
    for root in (Path("/var/lib/uap-observer-consent"), Path("/var/lib/uap-observer-human")):
        transition_record(root, challenge, action)


@dataclass(frozen=True)
class Adapter:
    artifact: str
    executable: Path
    digest: str
    config: Path
    config_digest: str


class ReviewedRunner:
    def __init__(self, adapters: tuple[Adapter, ...], state_root: Path, *, protected: bool = True):
        if {adapter.artifact for adapter in adapters} != ARTIFACT_NAMES:
            raise ValueError("reviewed adapter set is not canonical")
        if tuple(adapter.artifact for adapter in adapters) != ARTIFACT_ORDER:
            raise ValueError("reviewed adapter order is not canonical")
        self.adapters, self.state_root = adapters, state_root
        self.protected = protected
        self._owner_uid = 0 if protected else os.geteuid()
        if protected:
            service_identities = reviewed_service_identities()
            self._identities = {}
            self._config_gid = grp.getgrnam("uap-observer-adapter-config").gr_gid
            for identity in ("codex", "cursor", "kiro", "control"):
                uid, gid, _ = service_identities[identity]
                expected_home = f"/var/empty/uap-observer-{identity}"
                home = os.stat(expected_home, follow_symlinks=False)
                if (
                    not stat.S_ISDIR(home.st_mode) or home.st_uid != uid
                    or home.st_gid != gid or stat.S_IMODE(home.st_mode) != 0o700
                ):
                    raise ValueError("reviewed adapter home or groups differ")
                self._identities[identity] = (uid, gid)
        else:
            self._identities = {identity: (os.geteuid(), os.getegid()) for identity in ("codex", "cursor", "kiro", "control")}
            self._config_gid = os.getegid()
        for adapter in adapters:
            links = 5 if protected else 1
            encoded = read_owned_regular(adapter.executable, 1 << 20, owner_uid=self._owner_uid, executable=True, expected_nlink=links)
            if "sha256:" + hashlib.sha256(encoded).hexdigest() != adapter.digest:
                raise ValueError("reviewed adapter digest mismatch")

    def execute(self, context: dict[str, Any]) -> dict[str, Any]:
        if not isinstance(context, dict) or set(context) != {"request", "github_attestation"}:
            raise ValueError("runner context is not canonical")
        self.state_root.mkdir(parents=True, exist_ok=True, mode=0o711)
        state_info = self.state_root.stat()
        if self.protected and (state_info.st_uid != 0 or stat.S_IMODE(state_info.st_mode) != 0o711):
            raise ValueError("runner job root is not root-controlled")
        run_dir = Path(tempfile.mkdtemp(prefix="run-", dir=self.state_root))
        os.chmod(run_dir, 0o711)
        deadline = time.monotonic() + RUNNER_TOTAL_SECONDS
        try:
            artifacts: dict[str, Any] = {}
            for index, adapter in enumerate(self.adapters):
                links = 5 if self.protected else 1
                encoded_adapter = read_owned_regular(adapter.executable, 1 << 20, owner_uid=self._owner_uid, executable=True, expected_nlink=links)
                if "sha256:" + hashlib.sha256(encoded_adapter).hexdigest() != adapter.digest:
                    raise ValueError("reviewed adapter changed after runner startup")
                partials = []
                for identity in ARTIFACT_IDENTITIES[adapter.artifact]:
                    uid, gid = self._identities[identity]
                    invocation_dir = run_dir / f"adapter-{index}-{identity}"
                    invocation_dir.mkdir(mode=0o700)
                    context_path, output = invocation_dir / "context.json", invocation_dir / "artifact.json"
                    write_new_owned(context_path, canonical_json(context), uid=uid if self.protected else None, gid=gid if self.protected else None)
                    if self.protected:
                        os.chown(invocation_dir, uid, gid)
                    remaining = deadline - time.monotonic()
                    if remaining <= 0:
                        raise TimeoutError("reviewed adapters exceeded total deadline")
                    job_cgroup = delegated_job_cgroup() if self.protected else None
                    primary_error: BaseException | None = None
                    try:
                        def contain_and_drop_privileges() -> None:
                            if job_cgroup is not None:
                                (job_cgroup / "cgroup.procs").write_text(str(os.getpid()))
                            if self.protected:
                                os.setgroups([self._config_gid])
                                os.setgid(gid)
                                os.setuid(uid)
                        process = subprocess.Popen(
                            [str(adapter.executable), "--context", str(context_path), "--output", str(output)],
                            cwd=invocation_dir,
                            env={
                                "PATH": "/usr/bin:/bin", "LANG": "C.UTF-8", "LC_ALL": "C.UTF-8",
                                "UAP_OBSERVER_ADAPTER_CONFIG_SHA256": adapter.config_digest,
                                "UAP_OBSERVER_ADAPTER_CLIENT": identity,
                            },
                            stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                            start_new_session=True,
                            preexec_fn=contain_and_drop_privileges if self.protected else None,
                            shell=False,
                        )
                        try:
                            return_code = process.wait(timeout=remaining)
                        except subprocess.TimeoutExpired:
                            kill_process_group(process)
                            raise TimeoutError("reviewed adapter exceeded deadline") from None
                        finally:
                            kill_process_group(process)
                        if return_code != 0:
                            raise ValueError("reviewed adapter failed")
                    except BaseException as error:
                        primary_error = error
                        raise
                    finally:
                        revalidate = None
                        if self.protected and identity in {"codex", "cursor", "kiro"}:
                            revalidate = lambda: revalidate_client_proofs(
                                adapter.config, adapter.config_digest, identity, uid, gid,
                            )
                        finalize_client_job(job_cgroup, revalidate, primary_error)
                    partials.append(strict_json_loads(read_owned_regular(output, MAX_ADAPTER_OUTPUT, owner_uid=uid)))
                if len(partials) == 1:
                    artifacts[adapter.artifact] = partials[0]
                else:
                    allowed_partial_fields = {"schema_version", "attestations", "external_pr_evidence"}
                    if not all(isinstance(partial, dict) and set(partial).issubset(allowed_partial_fields) and {"schema_version", "attestations"}.issubset(partial) and type(partial["schema_version"]) is int and partial["schema_version"] == 1 and isinstance(partial["attestations"], list) for partial in partials):
                        raise ValueError("split adapter artifact is not canonical")
                    merged = {"schema_version": 1, "attestations": [record for partial in partials for record in partial["attestations"]]}
                    external_values = [partial.get("external_pr_evidence") for partial in partials]
                    if any(value is not None for value in external_values):
                        if any(value != external_values[0] for value in external_values):
                            raise ValueError("split adapters disagree on immutable external PR evidence")
                        merged["external_pr_evidence"] = external_values[0]
                    artifacts[adapter.artifact] = merged
            result = validate_artifacts(artifacts)
            if self.protected:
                transition_records(context["request"]["challenge"]["value"], "reserve")
            return result
        finally:
            shutil.rmtree(run_dir, ignore_errors=True)


def read_exact(stream: socket.socket, size: int) -> bytes:
    chunks = []
    while size:
        chunk = stream.recv(size)
        if not chunk:
            raise ValueError("runner request was truncated")
        chunks.append(chunk)
        size -= len(chunk)
    return b"".join(chunks)


def peer_uid(stream: socket.socket) -> int:
    if not hasattr(socket, "SO_PEERCRED"):
        raise ValueError("runner requires Linux SO_PEERCRED")
    _, uid, _ = struct.unpack("3i", stream.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12))
    return uid


def handle(stream: socket.socket, runner: ReviewedRunner, allowed_uid: int) -> None:
    try:
        if peer_uid(stream) != allowed_uid:
            raise ValueError("runner peer is not authorized")
        stream.settimeout(20)
        length = struct.unpack("!I", read_exact(stream, 4))[0]
        if not 1 <= length <= MAX_MESSAGE:
            raise ValueError("runner request length is invalid")
        request = strict_json_loads(read_exact(stream, length))
        if isinstance(request, dict) and request.get("operation") == "execute" and set(request) == {"operation", "context"}:
            response = canonical_json({"artifacts": runner.execute(request["context"])})
        elif isinstance(request, dict) and request.get("operation") in {"commit", "rollback"} and set(request) == {"operation", "challenge"}:
            transition_records(request["challenge"], request["operation"])
            response = canonical_json({"transaction": request["operation"]})
        else:
            response = canonical_json({"artifacts": runner.execute(request)})
    except Exception:
        response = canonical_json({"error": "reviewed runner failed"})
    try:
        stream.sendall(struct.pack("!I", len(response)) + response)
    except (BrokenPipeError, ConnectionResetError):
        pass


def serve(listener: socket.socket, runner: ReviewedRunner, allowed_uid: int, *, once: bool = False) -> None:
    while True:
        stream, _ = listener.accept()
        with stream:
            handle(stream, runner, allowed_uid)
        if once:
            return


def load_adapters(path: Path, *, adapter_gid: int | None = None, enforce_fixed: bool = False) -> tuple[Adapter, ...]:
    value = strict_json_loads(read_owned_regular(path, 64 << 10, owner_uid=0))
    if not isinstance(value, dict) or set(value) != {"schema_version", "config", "artifacts"} or type(value.get("schema_version")) is not int or value.get("schema_version") != 1:
        raise ValueError("adapter manifest is not canonical")
    config_item, artifact_items = value["config"], value["artifacts"]
    if not isinstance(config_item, dict) or set(config_item) != {"path", "sha256"} or not isinstance(artifact_items, dict) or set(artifact_items) != ARTIFACT_NAMES:
        raise ValueError("adapter manifest is not canonical")
    config = Path(config_item["path"])
    config_digest = config_item["sha256"]
    if (enforce_fixed and config != FIXED_CONFIG) or not config.is_absolute() or not isinstance(config_digest, str) or not re.fullmatch(r"sha256:[a-f0-9]{64}", config_digest):
        raise ValueError("adapter manifest config is invalid")
    expected_gid = os.getegid() if adapter_gid is None else adapter_gid
    encoded_config = read_owned_regular(config, 4 << 20, owner_uid=0, exact_mode=0o640, group_gid=expected_gid)
    if "sha256:" + hashlib.sha256(encoded_config).hexdigest() != config_digest:
        raise ValueError("adapter config digest mismatch")
    adapters = []
    for artifact in ARTIFACT_ORDER:
        item = artifact_items[artifact]
        if not isinstance(item, dict) or set(item) != {"path", "sha256"}:
            raise ValueError("adapter manifest entry is invalid")
        executable = Path(item["path"])
        digest = item["sha256"]
        if (enforce_fixed and executable != FIXED_ENTRYPOINTS[artifact]) or not executable.is_absolute() or not isinstance(digest, str) or not re.fullmatch(r"sha256:[a-f0-9]{64}", digest):
            raise ValueError("adapter manifest entry is invalid")
        adapters.append(Adapter(artifact, executable, digest, config, config_digest))
    executable_stats = [os.stat(adapter.executable, follow_symlinks=False) for adapter in adapters]
    expected_links = 5 if enforce_fixed else 4
    if len({(item.st_dev, item.st_ino) for item in executable_stats}) != 1 or any(item.st_nlink != expected_links for item in executable_stats) or len({adapter.digest for adapter in adapters}) != 1:
        raise ValueError("adapter entrypoints are not exact hardlinks")
    return tuple(adapters)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--socket-fd", type=int, default=3)
    args = parser.parse_args()
    allowed_uid = pwd.getpwnam("uap-observer").pw_uid
    adapter_gid = grp.getgrnam("uap-observer-adapter-config").gr_gid
    listener = socket.socket(fileno=args.socket_fd)
    adapters = load_adapters(Path("/opt/uap-observer-current/etc/uap-observer-adapters.json"), adapter_gid=adapter_gid, enforce_fixed=True)
    serve(listener, ReviewedRunner(adapters, Path("/var/lib/uap-observer/jobs")), allowed_uid)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
