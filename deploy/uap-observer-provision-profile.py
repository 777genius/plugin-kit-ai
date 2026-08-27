#!/usr/bin/env python3
"""Provision one isolated test-auth profile without modifying its root-owned seed."""

from __future__ import annotations

import argparse
import fcntl
import hashlib
import json
import math
import os
import pwd
import re
import stat
from pathlib import Path
from typing import Protocol

CLIENTS = {"codex", "cursor", "kiro"}
HEROES = {"agent-code-navigator", "context7", "cloudflare-docs", "chrome-devtools", "notion"}
PROFILE_ROOT = Path("/var/lib/uap-observer/profiles")
PROOF_ROOT = Path("/var/lib/uap-observer/proofs")
PROOF_SEED_NAME = ".uap-observer-proof"
TRANSACTION_VERSION = 1
MAX_FILES = 50_000
MAX_BYTES = 2 << 30
OPEN_DIRECTORY = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC | getattr(os, "O_NOATIME", 0)
TUPLE_FIELDS = {
    "product_id", "tree_digest", "manifest_digest", "distribution_id", "distribution_kind",
    "release_sequence", "package_version", "source_repository", "source_revision", "source_path",
    "snapshot_sequence", "snapshot_digest", "binary_digest", "dependency_identity", "installer_version",
    "adapter_version", "client_version", "os", "architecture", "observed_at",
}
TUPLE_DIGEST_FIELDS = {"tree_digest", "manifest_digest", "snapshot_digest", "binary_digest"}


def strict_json_loads(encoded: bytes | str):
    """Apply the observer's fail-closed JSON evidence decoding policy."""
    def object_from_pairs(pairs):
        value, folded = {}, set()
        for key, child in pairs:
            normalized = key.casefold()
            if key in value or normalized in folded:
                raise ValueError("duplicate or case-confusable JSON object member")
            value[key] = child
            folded.add(normalized)
        return value

    def reject_constant(value):
        raise ValueError(f"non-finite JSON number: {value}")

    def finite_float(value):
        decoded = float(value)
        if not math.isfinite(decoded):
            raise ValueError(f"non-finite JSON number: {value}")
        return decoded

    return json.loads(encoded, object_pairs_hook=object_from_pairs,
                      parse_constant=reject_constant, parse_float=finite_float)


def validate_release_tuple(value, plugin: str) -> None:
    if not isinstance(value, dict) or set(value) != TUPLE_FIELDS or value.get("product_id") != plugin:
        raise ValueError("profile seed release tuple is invalid")
    if any(type(value.get(field)) is not int or value[field] < 1 for field in ("release_sequence", "snapshot_sequence")):
        raise ValueError("profile seed release tuple sequence is invalid")
    strings = TUPLE_FIELDS - {"release_sequence", "snapshot_sequence", "client_version"}
    if any(type(value.get(field)) is not str or not value[field] for field in strings) or value.get("client_version") is not None:
        raise ValueError("profile seed release tuple identifier is invalid")
    if (
        re.fullmatch(r"[a-f0-9]{40}", value["source_revision"]) is None
        or any(re.fullmatch(r"sha256:[a-f0-9]{64}", value[field]) is None for field in TUPLE_DIGEST_FIELDS)
        or value["source_repository"].startswith("/") or "//" in value["source_repository"]
        or Path(value["source_path"]).is_absolute() or ".." in Path(value["source_path"]).parts
    ):
        raise ValueError("profile seed release tuple provenance is invalid")


def checkpoint(_name: str) -> None:
    """Internal failpoint seam used by the privileged durability tests."""


def fsync_directory(directory_fd: int) -> None:
    os.fsync(directory_fd)


class Digest(Protocol):
    def update(self, value: bytes) -> None: ...


def open_root_owned_directory(path: Path, *, final_mode: int | None = None) -> int:
    if not path.is_absolute() or ".." in path.parts:
        raise ValueError("protected directory path is invalid")
    descriptor = os.open("/", os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for component in path.parts[1:]:
            child = os.open(component, OPEN_DIRECTORY, dir_fd=descriptor)
            os.close(descriptor)
            descriptor = child
            info = os.fstat(descriptor)
            if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
                raise ValueError("protected directory must be immutable and root-owned")
        info = os.fstat(descriptor)
        if final_mode is not None and stat.S_IMODE(info.st_mode) != final_mode:
            raise ValueError("protected directory mode differs")
        return descriptor
    except Exception:
        os.close(descriptor)
        raise


def checked_entry(parent_fd: int, name: str) -> os.stat_result:
    info = os.stat(name, dir_fd=parent_fd, follow_symlinks=False)
    if not (stat.S_ISREG(info.st_mode) or stat.S_ISDIR(info.st_mode)):
        raise ValueError("profile seed contains a link or special file")
    if info.st_uid != 0 or info.st_mode & 0o022:
        raise ValueError("profile seed entry is not protected and root-owned")
    if stat.S_ISREG(info.st_mode):
        if info.st_nlink != 1:
            raise ValueError("profile seed contains a hardlinked file")
    return info


def copy_tree(
    source_fd: int, destination_fd: int | None, framed: Digest, logical_parent: tuple[str, ...] = (),
) -> tuple[int, int]:
    count = total = 0
    for name in sorted(os.listdir(source_fd)):
        if name in {".", ".."} or "/" in name or "\x00" in name:
            raise ValueError("profile seed entry name is invalid")
        info = checked_entry(source_fd, name)
        logical_path = (*logical_parent, name)
        encoded_path = b"/".join(os.fsencode(component) for component in logical_path)
        framed.update(len(encoded_path).to_bytes(8, "big") + encoded_path)
        count += 1
        if count > MAX_FILES:
            raise ValueError("profile seed exceeds file-count bound")
        if stat.S_ISDIR(info.st_mode):
            framed.update(b"D" + (0).to_bytes(8, "big"))
            source_child = os.open(name, OPEN_DIRECTORY, dir_fd=source_fd)
            try:
                current = os.fstat(source_child)
                if (current.st_dev, current.st_ino) != (info.st_dev, info.st_ino):
                    raise ValueError("profile seed directory changed during copy")
                if destination_fd is not None:
                    os.mkdir(name, 0o700, dir_fd=destination_fd)
                    destination_child = os.open(name, OPEN_DIRECTORY, dir_fd=destination_fd)
                else:
                    destination_child = None
                try:
                    child_count, child_total = copy_tree(source_child, destination_child, framed, logical_path)
                    count += child_count
                    total += child_total
                    if count > MAX_FILES or total > MAX_BYTES:
                        raise ValueError("profile seed exceeds copy bounds")
                finally:
                    if destination_child is not None:
                        fsync_directory(destination_child)
                        checkpoint("after_staged_directory_fsync")
                        os.close(destination_child)
                after = os.fstat(source_child)
                if (after.st_dev, after.st_ino, after.st_mtime_ns, after.st_ctime_ns) != (info.st_dev, info.st_ino, info.st_mtime_ns, info.st_ctime_ns):
                    raise ValueError("profile seed directory changed during copy")
            finally:
                os.close(source_child)
            continue
        source = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC | getattr(os, "O_NOATIME", 0), dir_fd=source_fd)
        output = -1
        try:
            current = os.fstat(source)
            if (
                not stat.S_ISREG(current.st_mode) or current.st_uid != 0 or
                current.st_mode & 0o022 or current.st_nlink != 1 or
                (current.st_dev, current.st_ino) != (info.st_dev, info.st_ino)
            ):
                raise ValueError("profile seed file changed during copy")
            framed.update(b"F" + current.st_size.to_bytes(8, "big"))
            if destination_fd is not None:
                output = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600, dir_fd=destination_fd)
            copied = 0
            while chunk := os.read(source, 1 << 20):
                copied += len(chunk)
                total += len(chunk)
                if total > MAX_BYTES:
                    raise ValueError("profile seed exceeds byte bound")
                framed.update(chunk)
                if output >= 0:
                    view = memoryview(chunk)
                    while view:
                        view = view[os.write(output, view):]
            after = os.fstat(source)
            if copied != current.st_size or (
                after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns, after.st_ctime_ns
            ) != (
                current.st_dev, current.st_ino, current.st_size, current.st_mtime_ns, current.st_ctime_ns
            ):
                raise ValueError("profile seed changed during copy")
            if output >= 0:
                os.fsync(output)
                checkpoint("after_staged_file_fsync")
        finally:
            if output >= 0:
                os.close(output)
            os.close(source)
    return count, total


def remove_tree(parent_fd: int, name: str) -> None:
    directory = os.open(name, OPEN_DIRECTORY, dir_fd=parent_fd)
    try:
        for child in os.listdir(directory):
            info = os.stat(child, dir_fd=directory, follow_symlinks=False)
            if stat.S_ISDIR(info.st_mode):
                remove_tree(directory, child)
            else:
                os.unlink(child, dir_fd=directory)
    finally:
        os.close(directory)
    os.rmdir(name, dir_fd=parent_fd)


def remove_tree_durable(parent_fd: int, name: str) -> None:
    """Remove a staged tree and durably publish that parent mutation."""
    remove_tree(parent_fd, name)
    fsync_directory(parent_fd)


def assign_tree(
    directory_fd: int, uid: int, gid: int, *, include_self: bool = True,
    logical_parent: tuple[str, ...] = (), protected_files: set[tuple[str, ...]] | None = None,
    protected_directories: set[tuple[str, ...]] | None = None,
) -> None:
    """Assign only descriptors within the root-created staging tree, bottom-up."""
    protected_files = protected_files or set()
    protected_directories = protected_directories or set()
    for name in os.listdir(directory_fd):
        info = os.stat(name, dir_fd=directory_fd, follow_symlinks=False)
        flags = OPEN_DIRECTORY if stat.S_ISDIR(info.st_mode) else os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC
        descriptor = os.open(name, flags, dir_fd=directory_fd)
        try:
            current = os.fstat(descriptor)
            if (current.st_dev, current.st_ino) != (info.st_dev, info.st_ino):
                raise ValueError("staged profile changed during ownership assignment")
            if stat.S_ISDIR(current.st_mode):
                assign_tree(
                    descriptor, uid, gid, logical_parent=(*logical_parent, name),
                    protected_files=protected_files, protected_directories=protected_directories,
                )
            elif not stat.S_ISREG(current.st_mode) or current.st_nlink != 1:
                raise ValueError("staged profile was substituted")
            logical_path = (*logical_parent, name)
            protected = logical_path in (protected_directories if stat.S_ISDIR(current.st_mode) else protected_files)
            os.fchown(descriptor, 0 if protected else uid, gid)
            os.fchmod(descriptor, (0o510 if protected else 0o700) if stat.S_ISDIR(current.st_mode) else (0o440 if protected else 0o600))
            os.fsync(descriptor)
        finally:
            os.close(descriptor)
    if include_self:
        protected = logical_parent in protected_directories
        os.fchown(directory_fd, 0 if protected else uid, gid)
        os.fchmod(directory_fd, 0o510 if protected else 0o700)
        os.fsync(directory_fd)


def transaction_body(
    client: str, seed_digest: str, profile_preexisting: bool, phase: str = "preparing",
    previous_publication: str = "",
) -> bytes:
    payload = {
        "schema_version": TRANSACTION_VERSION, "client": client,
        "seed_digest": seed_digest, "profile_preexisting": profile_preexisting, "phase": phase,
        "previous_publication": previous_publication,
    }
    canonical = json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()
    return json.dumps({**payload, "payload_sha256": hashlib.sha256(canonical).hexdigest()}, sort_keys=True, separators=(",", ":")).encode()


def validate_transaction(body: bytes, client: str) -> dict[str, str | int]:
    value = strict_json_loads(body)
    if (
        not isinstance(value, dict)
        or set(value) != {"schema_version", "client", "seed_digest", "profile_preexisting", "phase", "previous_publication", "payload_sha256"}
        or type(value.get("schema_version")) is not int or value.get("schema_version") != TRANSACTION_VERSION
        or type(value.get("profile_preexisting")) is not bool
        or type(value.get("client")) is not str or value.get("client") != client
        or type(value.get("seed_digest")) is not str
        or re.fullmatch(r"sha256:[a-f0-9]{64}", value["seed_digest"]) is None
        or type(value.get("phase")) is not str or value.get("phase") not in {"preparing", "rollback", "committed"}
        or type(value.get("previous_publication")) is not str
        or (value["previous_publication"] != "" and re.fullmatch(r"sha256:[a-f0-9]{64}", value["previous_publication"]) is None)
        or type(value.get("payload_sha256")) is not str
        or re.fullmatch(r"[a-f0-9]{64}", value["payload_sha256"]) is None
    ):
        raise ValueError("profile transaction marker is invalid")
    payload = {key: value[key] for key in ("schema_version", "client", "seed_digest", "profile_preexisting", "phase", "previous_publication")}
    canonical = json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()
    if value["payload_sha256"] != hashlib.sha256(canonical).hexdigest():
        raise ValueError("profile transaction marker authentication differs")
    return value


def write_transaction(
    parent_fd: int, name: str, client: str, seed_digest: str, profile_preexisting: bool, *,
    phase: str = "preparing", previous_publication: str = "", replace: bool = False,
) -> None:
    output_name = f"{name}.new"
    descriptor = os.open(output_name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600, dir_fd=parent_fd)
    boundary = "rollback" if phase == "rollback" else "commit" if replace else "transaction"
    try:
        checkpoint(f"after_transaction_{boundary}_staging_create" if boundary != "transaction" else "after_transaction_staging_create")
        body = transaction_body(client, seed_digest, profile_preexisting, phase, previous_publication)
        split = max(1, len(body) // 2)
        view = memoryview(body)[:split]
        while view:
            view = view[os.write(descriptor, view):]
        checkpoint(f"after_transaction_{boundary}_partial_write" if boundary != "transaction" else "after_transaction_partial_write")
        view = memoryview(body)[split:]
        while view:
            view = view[os.write(descriptor, view):]
        os.fsync(descriptor)
        checkpoint(
            "after_transaction_rollback_file_fsync" if phase == "rollback"
            else "after_transaction_commit_file_fsync" if replace
            else "after_transaction_file_fsync"
        )
    finally:
        os.close(descriptor)
    os.rename(output_name, name, src_dir_fd=parent_fd, dst_dir_fd=parent_fd)
    checkpoint(
        "after_transaction_rollback_rename" if phase == "rollback"
        else "after_transaction_commit_rename" if replace
        else "after_transaction_rename"
    )
    fsync_directory(parent_fd)
    checkpoint(
        "after_transaction_rollback_fsync" if phase == "rollback"
        else "after_transaction_commit_fsync" if replace
        else "after_transaction_fsync"
    )


def lock_client(parent_fd: int, name: str) -> int:
    try:
        descriptor = os.open(name, os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600, dir_fd=parent_fd)
        info = os.fstat(descriptor)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or stat.S_IMODE(info.st_mode) != 0o600 or info.st_nlink != 1:
            raise ValueError("profile provisioning lock is not protected")
        os.fsync(descriptor)
        fsync_directory(parent_fd)
        fcntl.flock(descriptor, fcntl.LOCK_EX)
        return descriptor
    except Exception:
        if 'descriptor' in locals():
            os.close(descriptor)
        raise


def published_seed_digest(lock_fd: int) -> str | None:
    os.lseek(lock_fd, 0, os.SEEK_SET)
    body = os.read(lock_fd, 256)
    if not body:
        return None
    try:
        value = body.decode("ascii")
    except UnicodeDecodeError as error:
        raise ValueError("profile provisioning lock publication record is invalid") from error
    if not value.startswith("sha256:") or len(value) != 71 or any(character not in "0123456789abcdef" for character in value[7:]):
        raise ValueError("profile provisioning lock publication record is invalid")
    return value


def write_publication_record(lock_fd: int, body: bytes) -> None:
    if body and (
        len(body) != 71 or not body.startswith(b"sha256:")
        or any(character not in b"0123456789abcdef" for character in body[7:])
    ):
        raise ValueError("profile publication record body is invalid")
    checkpoint("before_publication_record_update")
    os.lseek(lock_fd, 0, os.SEEK_SET)
    os.ftruncate(lock_fd, 0)
    checkpoint("after_publication_record_truncate")
    view = memoryview(body)
    while view:
        written = os.write(lock_fd, view)
        if written <= 0:
            raise OSError("profile publication record write made no progress")
        view = view[written:]
        if view:
            checkpoint("after_publication_record_partial_write")
    checkpoint("after_publication_record_full_write")
    os.fsync(lock_fd)
    checkpoint("after_publication_record_fsync")


def finalize_publication(profile_root_fd: int, lock_fd: int, marker_name: str, seed_digest: str) -> None:
    write_publication_record(lock_fd, seed_digest.encode("ascii"))
    # The authenticated committed marker remains durable across every
    # in-place record update boundary.  Recovery may therefore distinguish a
    # torn in-flight transition from an unauthorised malformed lock record.
    fsync_directory(profile_root_fd)
    checkpoint("after_transaction_cleanup_fsync")
    checkpoint("before_transaction_marker_cleanup")
    try:
        os.unlink(marker_name, dir_fd=profile_root_fd)
    except FileNotFoundError:
        pass
    checkpoint("after_transaction_marker_unlink")
    fsync_directory(profile_root_fd)
    checkpoint("after_transaction_marker_cleanup_fsync")


def exact_published_directories(profile_root_fd: int, proof_root_fd: int, client: str, uid: int, gid: int) -> bool:
    expected = ((profile_root_fd, 0, gid, 0o510), (proof_root_fd, 0, gid, 0o510))
    for parent_fd, owner, group, mode in expected:
        try:
            info = os.stat(client, dir_fd=parent_fd, follow_symlinks=False)
        except FileNotFoundError:
            return False
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != owner or info.st_gid != group or stat.S_IMODE(info.st_mode) != mode:
            return False
    return True


def remove_if_present(parent_fd: int, name: str) -> None:
    try:
        info = os.stat(name, dir_fd=parent_fd, follow_symlinks=False)
    except FileNotFoundError:
        # An earlier attempt may have completed unlink(2) and then failed its
        # directory fsync.  Absence is not durable evidence until the parent
        # has been synchronized by this attempt.
        fsync_directory(parent_fd)
        return
    if not stat.S_ISDIR(info.st_mode):
        raise ValueError("recoverable profile transaction target is not a directory")
    remove_tree_durable(parent_fd, name)


def unlink_regular_if_present(parent_fd: int, name: str) -> None:
    try:
        info = os.stat(name, dir_fd=parent_fd, follow_symlinks=False)
    except FileNotFoundError:
        return
    if (
        not stat.S_ISREG(info.st_mode) or info.st_uid != 0
        or stat.S_IMODE(info.st_mode) != 0o600 or info.st_nlink != 1
    ):
        raise ValueError("recoverable transaction sidecar is not protected")
    os.unlink(name, dir_fd=parent_fd)
    fsync_directory(parent_fd)


def _relative_to(path: Path, prefix: Path) -> Path | None:
    try:
        return path.relative_to(prefix)
    except ValueError:
        return None


def validate_native_projection(value, client: str) -> list[dict]:
    """Validate the exact projection schema before it can affect publication."""
    entries = value.get("entries") if isinstance(value, dict) else None
    evidence_fields = {"manager_add_sha256", "manager_info_sha256", "post_add_doctor_sha256"}
    entry_fields = {"plugin", "component_kind", "tuple", "native_config", "client_config", *evidence_fields}
    digest_pattern = re.compile(r"sha256:[a-f0-9]{64}")
    if (
        not isinstance(value, dict)
        or set(value) != {"schema_version", "client_id", "entries"}
        or type(value.get("schema_version")) is not int
        or value.get("schema_version") != 2
        or type(value.get("client_id")) is not str
        or value.get("client_id") != client
        or not isinstance(entries, list)
        or not entries
    ):
        raise ValueError("profile seed native projection is invalid")
    plugins: set[str] = set()
    for entry in entries:
        if (
            not isinstance(entry, dict) or set(entry) != entry_fields
            or type(entry.get("plugin")) is not str or not entry["plugin"] or entry["plugin"] in plugins
            or entry.get("component_kind") not in {"skill", "mcp"}
            or not isinstance(entry.get("tuple"), dict)
            or any(
                type(entry.get(field)) is not str or digest_pattern.fullmatch(entry[field]) is None
                for field in evidence_fields
            )
        ):
            raise ValueError("profile seed native projection entry is invalid")
        validate_release_tuple(entry["tuple"], entry["plugin"])
        plugins.add(entry["plugin"])
        config = entry["client_config"]
        native = entry["native_config"]
        if (
            not isinstance(config, dict) or set(config) != {"path", "sha256"}
            or not isinstance(native, dict) or set(native) != {"path", "sha256"}
            or type(config.get("path")) is not str or type(native.get("path")) is not str
            or type(config.get("sha256")) is not str or digest_pattern.fullmatch(config["sha256"]) is None
            or type(native.get("sha256")) is not str or digest_pattern.fullmatch(native["sha256"]) is None
            or config["sha256"] != native["sha256"]
        ):
            raise ValueError("profile seed native projection config is invalid")
        active = Path(config["path"])
        skill_suffix = active.parts[-3:] == ("skills", "code-tool-router", "SKILL.md")
        if (entry["component_kind"] == "skill") != skill_suffix:
            raise ValueError("profile seed native projection capability path is invalid")
    if plugins != HEROES:
        raise ValueError("profile seed native projection is incomplete")
    kinds = {entry["plugin"]: entry["component_kind"] for entry in entries}
    if kinds != {plugin: ("skill" if plugin == "agent-code-navigator" else "mcp") for plugin in HEROES}:
        raise ValueError("profile seed native projection component kind is invalid")
    by_active_path: dict[str, list[dict]] = {}
    for entry in entries:
        by_active_path.setdefault(entry["client_config"]["path"], []).append(entry)
    duplicates = [group for group in by_active_path.values() if len(group) > 1]
    if client == "kiro":
        shared = str(Path("/var/lib/uap-observer/profiles/kiro/.kiro/settings/mcp.json"))
        skill = str(Path("/var/lib/uap-observer/profiles/kiro/.kiro/skills/code-tool-router/SKILL.md"))
        if (
            set(by_active_path) != {shared, skill}
            or len(duplicates) != 1 or len(duplicates[0]) != len(HEROES) - 1
            or {entry["plugin"] for entry in duplicates[0]} != HEROES - {"agent-code-navigator"}
            or any(entry["component_kind"] != "mcp" for entry in duplicates[0])
            or len({entry["client_config"]["sha256"] for entry in duplicates[0]}) != 1
        ):
            raise ValueError("profile seed native projection has conflicting active configs")
    elif duplicates:
        raise ValueError("profile seed native projection has conflicting active configs")
    return entries


def validate_receipts(value, entries: list[dict]) -> None:
    evidence = {"manager_add_sha256", "manager_info_sha256", "post_add_doctor_sha256"}
    fields = {"name", "tuple", *evidence}
    receipts = value.get("receipts") if isinstance(value, dict) else None
    if (
        not isinstance(value, dict) or set(value) != {"schema_version", "receipts"}
        or type(value.get("schema_version")) is not int or value["schema_version"] != 1
        or not isinstance(receipts, list) or len(receipts) != len(HEROES)
        or any(not isinstance(record, dict) or set(record) != fields for record in receipts)
    ):
        raise ValueError("profile seed receipts are invalid")
    by_name = {record.get("name"): record for record in receipts}
    if set(by_name) != HEROES or len(by_name) != len(receipts):
        raise ValueError("profile seed receipts are incomplete")
    digest_pattern = re.compile(r"sha256:[a-f0-9]{64}")
    for entry in entries:
        record = by_name[entry["plugin"]]
        validate_release_tuple(record["tuple"], record["name"])
        if record["tuple"] != entry["tuple"] or any(
            type(record.get(field)) is not str or digest_pattern.fullmatch(record[field]) is None
            or record[field] != entry[field] for field in evidence
        ):
            raise ValueError("profile seed receipt does not bind native projection evidence")


def read_bounded_regular(descriptor: int, info: os.stat_result, limit: int) -> bytes:
    if info.st_size < 0 or info.st_size > limit:
        raise ValueError("profile seed native projection content exceeds its bound")
    body = bytearray()
    while len(body) <= limit:
        chunk = os.read(descriptor, min(1 << 20, limit + 1 - len(body)))
        if not chunk:
            break
        body.extend(chunk)
    if len(body) != info.st_size or len(body) > limit:
        raise ValueError("profile seed native projection content changed or exceeds its bound")
    return bytes(body)


def restore_empty_profile(profile_root_fd: int, client: str, uid: int, gid: int) -> None:
    """Idempotently restore the exact installed empty profile directory."""
    try:
        info = os.stat(client, dir_fd=profile_root_fd, follow_symlinks=False)
    except FileNotFoundError:
        os.mkdir(client, 0o700, dir_fd=profile_root_fd)
        descriptor = os.open(client, OPEN_DIRECTORY, dir_fd=profile_root_fd)
        try:
            os.fchown(descriptor, uid, gid)
            os.fchmod(descriptor, 0o700)
            os.fsync(descriptor)
        finally:
            os.close(descriptor)
        fsync_directory(profile_root_fd)
        return
    if (
        not stat.S_ISDIR(info.st_mode) or info.st_uid != uid or info.st_gid != gid
        or stat.S_IMODE(info.st_mode) != 0o700
    ):
        raise ValueError("recoverable empty profile differs")
    descriptor = os.open(client, OPEN_DIRECTORY, dir_fd=profile_root_fd)
    try:
        if os.listdir(descriptor):
            raise ValueError("recoverable empty profile is not empty")
    finally:
        os.close(descriptor)


def recover_transaction(
    profile_root_fd: int, proof_root_fd: int, client: str, marker_name: str,
    seed_digest: str, uid: int, gid: int, lock_fd: int | None = None,
) -> bool:
    try:
        marker_fd = os.open(marker_name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=profile_root_fd)
    except FileNotFoundError:
        unlink_regular_if_present(profile_root_fd, f"{marker_name}.new")
        return False
    try:
        info = os.fstat(marker_fd)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or stat.S_IMODE(info.st_mode) != 0o600 or info.st_nlink != 1:
            raise ValueError("profile transaction marker is not protected")
        marker_body = os.read(marker_fd, 4097)
        if len(marker_body) > 4096:
            raise ValueError("profile transaction marker is oversized")
        marker = validate_transaction(marker_body, client)
    finally:
        os.close(marker_fd)
    if marker["seed_digest"] != seed_digest:
        raise ValueError("profile transaction marker seed digest differs")
    sidecar_name = f"{marker_name}.new"
    sidecar_incomplete = False
    try:
        sidecar_fd = os.open(sidecar_name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=profile_root_fd)
    except FileNotFoundError:
        sidecar = None
    else:
        try:
            sidecar_info = os.fstat(sidecar_fd)
            if (
                not stat.S_ISREG(sidecar_info.st_mode) or sidecar_info.st_uid != 0
                or stat.S_IMODE(sidecar_info.st_mode) != 0o600 or sidecar_info.st_nlink != 1
            ):
                raise ValueError("profile transaction sidecar is not protected")
            sidecar_body = os.read(sidecar_fd, 4097)
            if len(sidecar_body) > 4096:
                sidecar = None
                sidecar_incomplete = True
            else:
                try:
                    sidecar = validate_transaction(sidecar_body, client)
                except (UnicodeDecodeError, ValueError, json.JSONDecodeError):
                    # The fixed .new name is only staging.  Until rename(2), its
                    # bytes have no transaction authority and SIGKILL may leave
                    # any prefix from zero bytes through the complete body.
                    # Metadata is checked above; malformed staging is therefore
                    # safe to discard while the already-renamed marker remains
                    # the sole authoritative state.
                    sidecar = None
                    sidecar_incomplete = True
        finally:
            os.close(sidecar_fd)
    if sidecar is not None:
        immutable = ("schema_version", "client", "seed_digest", "profile_preexisting", "previous_publication")
        if any(type(sidecar[key]) is not type(marker[key]) or sidecar[key] != marker[key] for key in immutable):
            raise ValueError("profile transaction sidecar differs from transaction")
        allowed_transition = (
            sidecar["phase"] == "rollback"
            or sidecar["phase"] == "committed" and marker["phase"] == "preparing"
        )
        if not allowed_transition:
            raise ValueError("profile transaction sidecar phase is ambiguous")
        os.rename(sidecar_name, marker_name, src_dir_fd=profile_root_fd, dst_dir_fd=profile_root_fd)
        fsync_directory(profile_root_fd)
        marker = sidecar
    else:
        unlink_regular_if_present(profile_root_fd, sidecar_name)
        if sidecar_incomplete and marker["phase"] == "committed":
            # A committed marker can only stage a subsequent rollback.  The
            # earlier preparing->committed update has already renamed away its
            # .new file, so even a zero-length replacement unambiguously means
            # that rollback started but was interrupted before its body became
            # authoritative.  Resume rollback instead of accepting forward
            # state that an ordinary-failure path had begun to unwind.
            write_transaction(
                profile_root_fd, marker_name, client, seed_digest,
                bool(marker["profile_preexisting"]), phase="rollback",
                previous_publication=str(marker["previous_publication"]), replace=True,
            )
            marker["phase"] = "rollback"
    publication = None
    publication_malformed = False
    if lock_fd is not None:
        try:
            publication = published_seed_digest(lock_fd)
        except ValueError:
            # A malformed live record remains fatal unless this authenticated
            # marker proves that an in-place publication transition was in
            # flight.  Only committed/rollback phases can have reached that
            # update; a preparing marker has no authority to repair it.
            publication_malformed = True
    previous_publication = str(marker["previous_publication"])
    previous = previous_publication or None
    if marker["phase"] == "preparing" and lock_fd is not None and (
        publication_malformed or publication != previous
    ):
        raise ValueError("profile publication record conflicts with preparing transaction")
    if marker["phase"] in {"committed", "rollback"} and lock_fd is not None and (
        not publication_malformed and publication not in {None, previous, seed_digest}
    ):
        raise ValueError("profile publication record conflicts with authenticated transaction")
    if marker["phase"] == "committed" and (
        lock_fd is None or publication == seed_digest
    ):
        for parent_fd, name in ((profile_root_fd, client), (proof_root_fd, client)):
            info = os.stat(name, dir_fd=parent_fd, follow_symlinks=False)
            if not stat.S_ISDIR(info.st_mode):
                raise ValueError("committed profile transaction target differs")
        for parent_fd, name in ((profile_root_fd, f".{client}.new"), (proof_root_fd, f".{client}.new")):
            remove_if_present(parent_fd, name)
        return True
    if marker["phase"] == "committed":
        write_transaction(
            profile_root_fd, marker_name, client, seed_digest,
            bool(marker["profile_preexisting"]), phase="rollback",
            previous_publication=str(marker["previous_publication"]), replace=True,
        )
        marker["phase"] = "rollback"
    for parent_fd, name in ((profile_root_fd, client), (profile_root_fd, f".{client}.new"), (proof_root_fd, client), (proof_root_fd, f".{client}.new")):
        remove_if_present(parent_fd, name)
    if marker["profile_preexisting"]:
        restore_empty_profile(profile_root_fd, client, uid, gid)
    if lock_fd is not None:
        write_publication_record(lock_fd, previous_publication.encode("ascii"))
        checkpoint("after_recovery_publication_restore")
    os.unlink(marker_name, dir_fd=profile_root_fd)
    fsync_directory(profile_root_fd)
    return False


def active_native_paths(proof_fd: int, client: str, staging_fd: int) -> tuple[set[tuple[str, ...]], set[tuple[str, ...]]]:
    descriptor = os.open("native-projection.json", os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=proof_fd)
    try:
        info = os.fstat(descriptor)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or stat.S_IMODE(info.st_mode) != 0o440 or info.st_nlink != 1:
            raise ValueError("profile seed native projection differs")
        value = strict_json_loads(read_bounded_regular(descriptor, info, 4 << 20))
    finally:
        os.close(descriptor)
    entries = validate_native_projection(value, client)
    receipt_fd = os.open("receipts.json", os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=proof_fd)
    try:
        receipt_info = os.fstat(receipt_fd)
        if not stat.S_ISREG(receipt_info.st_mode) or receipt_info.st_uid != 0 or stat.S_IMODE(receipt_info.st_mode) != 0o440 or receipt_info.st_nlink != 1:
            raise ValueError("profile seed receipts differ")
        receipts = strict_json_loads(read_bounded_regular(receipt_fd, receipt_info, 4 << 20))
    finally:
        os.close(receipt_fd)
    validate_receipts(receipts, entries)
    prefixes = {PROFILE_ROOT / client, Path("/var/lib/uap-observer/profiles") / client}
    proof_prefixes = {PROOF_ROOT / client, Path("/var/lib/uap-observer/proofs") / client}
    files: set[tuple[str, ...]] = set()
    for entry in entries:
        config = entry["client_config"]
        native = entry["native_config"]
        path = Path(config["path"])
        relative = next((candidate for prefix in prefixes if (candidate := _relative_to(path, prefix)) is not None), None)
        if relative is None:
            raise ValueError("active native config escapes its client profile")
        native_path = Path(native["path"])
        native_relative = next((candidate for prefix in proof_prefixes if (candidate := _relative_to(native_path, prefix)) is not None), None)
        if (
            native_relative is None or native_relative.parts != ("native", f'{entry["plugin"]}.blob')
        ):
            raise ValueError("profile seed native config proof escapes its proof hierarchy")
        parts = relative.parts
        if not parts or any(part in {"", ".", ".."} for part in parts):
            raise ValueError("active native config path is invalid")
        parent = staging_fd
        opened: list[int] = []
        target_fd = -1
        try:
            for component in parts[:-1]:
                parent = os.open(component, OPEN_DIRECTORY, dir_fd=parent)
                opened.append(parent)
            target_fd = os.open(parts[-1], os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=parent)
            target = os.fstat(target_fd)
            if not stat.S_ISREG(target.st_mode) or target.st_nlink != 1:
                raise ValueError("active native config is not a regular file")
            body = read_bounded_regular(target_fd, target, 4 << 20)
            if "sha256:" + hashlib.sha256(body).hexdigest() != config["sha256"]:
                raise ValueError("active native config digest differs")
        finally:
            if target_fd >= 0:
                os.close(target_fd)
            for opened_fd in reversed(opened):
                os.close(opened_fd)
        native_parent = os.open("native", OPEN_DIRECTORY, dir_fd=proof_fd)
        native_fd = -1
        try:
            native_fd = os.open(native_relative.parts[1], os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=native_parent)
            native_info = os.fstat(native_fd)
            if not stat.S_ISREG(native_info.st_mode) or native_info.st_nlink != 1:
                raise ValueError("active native config proof differs")
            native_body = read_bounded_regular(native_fd, native_info, 4 << 20)
            if (
                "sha256:" + hashlib.sha256(native_body).hexdigest() != native["sha256"]
                or native_body != body
            ):
                raise ValueError("active native config proof differs")
        finally:
            if native_fd >= 0:
                os.close(native_fd)
            os.close(native_parent)
        if parts in files and not (
            client == "kiro" and parts == (".kiro", "settings", "mcp.json")
            and entry["component_kind"] == "mcp"
        ):
            raise ValueError("profile seed native projection repeats an active config")
        files.add(parts)
    directories = {parts[:index] for parts in files for index in range(0, len(parts))}
    return files, directories


def seal_proof_directory(directory_fd: int, uid: int, gid: int) -> None:
    """Make proof and authoritative native bytes immutable below a root parent."""
    if set(os.listdir(directory_fd)) != {"receipts.json", "native-projection.json", "native"}:
        raise ValueError("profile seed proof inventory differs")
    for name in ("receipts.json", "native-projection.json"):
        descriptor = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=directory_fd)
        try:
            info = os.fstat(descriptor)
            if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_nlink != 1 or stat.S_IMODE(info.st_mode) != 0o600:
                raise ValueError("profile seed proof file differs")
            os.fchown(descriptor, 0, gid)
            os.fchmod(descriptor, 0o440)
            os.fsync(descriptor)
        finally:
            os.close(descriptor)
    native_fd = os.open("native", OPEN_DIRECTORY, dir_fd=directory_fd)
    try:
        if set(os.listdir(native_fd)) != {f"{plugin}.blob" for plugin in ("agent-code-navigator", "context7", "cloudflare-docs", "chrome-devtools", "notion")}:
            raise ValueError("profile seed native proof inventory differs")
        for name in os.listdir(native_fd):
            descriptor = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=native_fd)
            try:
                info = os.fstat(descriptor)
                if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_nlink != 1 or stat.S_IMODE(info.st_mode) != 0o600:
                    raise ValueError("profile seed native proof differs")
                os.fchown(descriptor, 0, gid)
                os.fchmod(descriptor, 0o440)
                os.fsync(descriptor)
            finally:
                os.close(descriptor)
        os.fchown(native_fd, 0, gid)
        os.fchmod(native_fd, 0o510)
        os.fsync(native_fd)
    finally:
        os.close(native_fd)
    os.fchown(directory_fd, 0, gid)
    os.fchmod(directory_fd, 0o510)
    os.fsync(directory_fd)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--client", choices=sorted(CLIENTS), required=True)
    parser.add_argument("--root-owned-seed", type=Path, required=True)
    parser.add_argument("--seed-digest", required=True)
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise SystemExit("profile provisioning requires root")
    account = pwd.getpwnam(f"uap-observer-{args.client}")
    source_fd = open_root_owned_directory(args.root_owned_seed)
    framed = hashlib.sha256(b"uap-observer-profile-seed-v1\0")
    copy_tree(source_fd, None, framed)
    digest = "sha256:" + framed.hexdigest()
    if args.seed_digest == "show":
        os.close(source_fd)
        print(digest)
        return 0
    if not args.seed_digest.startswith("sha256:") or digest != args.seed_digest:
        os.close(source_fd)
        raise ValueError("profile seed digest differs")
    profile_root_fd = open_root_owned_directory(PROFILE_ROOT, final_mode=0o711)
    proof_root_fd = open_root_owned_directory(PROOF_ROOT, final_mode=0o711)
    staging_name = f".{args.client}.new"
    marker_name = f".{args.client}.transaction"
    lock_fd = lock_client(profile_root_fd, f".{args.client}.lock")
    staging_fd = -1
    proof_fd = -1
    created = False
    proof_created = False
    transaction_created = False
    profile_published = False
    proof_published = False
    publication_before = ""
    try:
        if recover_transaction(
            profile_root_fd, proof_root_fd, args.client, marker_name, digest,
            account.pw_uid, account.pw_gid, lock_fd,
        ):
            finalize_publication(profile_root_fd, lock_fd, marker_name, digest)
            print(f"isolated {args.client} profile already provisioned; seed left untouched")
            return 0
        if published_seed_digest(lock_fd) == digest:
            if not exact_published_directories(profile_root_fd, proof_root_fd, args.client, account.pw_uid, account.pw_gid):
                raise ValueError("profile publication record differs from active directories")
            print(f"isolated {args.client} profile already provisioned; seed left untouched")
            return 0
        os.lseek(lock_fd, 0, os.SEEK_SET)
        publication_before = os.read(lock_fd, 256).decode("ascii")
        if publication_before and (
            len(publication_before) != 71 or not publication_before.startswith("sha256:")
            or any(character not in "0123456789abcdef" for character in publication_before[7:])
        ):
            raise ValueError("profile publication record is invalid")
        try:
            os.stat(staging_name, dir_fd=profile_root_fd, follow_symlinks=False)
        except FileNotFoundError:
            pass
        else:
            raise ValueError("stale profile staging entry exists")
        try:
            os.stat(args.client, dir_fd=proof_root_fd, follow_symlinks=False)
        except FileNotFoundError:
            pass
        else:
            raise ValueError("client proof target already exists")
        try:
            os.stat(staging_name, dir_fd=proof_root_fd, follow_symlinks=False)
        except FileNotFoundError:
            pass
        else:
            raise ValueError("stale proof staging entry exists")
        try:
            target_info = os.stat(args.client, dir_fd=profile_root_fd, follow_symlinks=False)
        except FileNotFoundError:
            target_info = None
        if target_info is not None:
            if not stat.S_ISDIR(target_info.st_mode) or target_info.st_uid != account.pw_uid or target_info.st_gid != account.pw_gid or stat.S_IMODE(target_info.st_mode) != 0o700:
                raise ValueError("client profile target is not the installed empty directory")
            target_fd = os.open(args.client, OPEN_DIRECTORY, dir_fd=profile_root_fd)
            try:
                if os.listdir(target_fd):
                    raise ValueError("client profile is already provisioned")
            finally:
                os.close(target_fd)
        transaction_created = True
        profile_preexisting = target_info is not None
        write_transaction(
            profile_root_fd, marker_name, args.client, digest, profile_preexisting,
            previous_publication=publication_before,
        )
        os.mkdir(staging_name, 0o700, dir_fd=profile_root_fd)
        created = True
        checkpoint("after_profile_staging_mkdir")
        fsync_directory(profile_root_fd)
        checkpoint("after_profile_staging_fsync")
        staging_fd = os.open(staging_name, OPEN_DIRECTORY, dir_fd=profile_root_fd)
        framed = hashlib.sha256(b"uap-observer-profile-seed-v1\0")
        copy_tree(source_fd, staging_fd, framed)
        checkpoint("after_profile_copy")
        if "sha256:" + framed.hexdigest() != digest:
            raise ValueError("profile seed changed between authentication and staging")
        fsync_directory(staging_fd)
        checkpoint("after_profile_copy_fsync")
        if target_info is not None:
            os.rmdir(args.client, dir_fd=profile_root_fd)
            checkpoint("after_empty_profile_remove")
            fsync_directory(profile_root_fd)
            checkpoint("after_empty_profile_remove_fsync")
        os.rename(PROOF_SEED_NAME, staging_name, src_dir_fd=staging_fd, dst_dir_fd=proof_root_fd)
        proof_created = True
        checkpoint("after_proof_staging_rename")
        fsync_directory(staging_fd)
        fsync_directory(proof_root_fd)
        checkpoint("after_proof_staging_fsync")
        proof_fd = os.open(staging_name, OPEN_DIRECTORY, dir_fd=proof_root_fd)
        seal_proof_directory(proof_fd, account.pw_uid, account.pw_gid)
        checkpoint("after_proof_ownership")
        protected_files, protected_directories = active_native_paths(proof_fd, args.client, staging_fd)
        assign_tree(
            staging_fd, account.pw_uid, account.pw_gid,
            protected_files=protected_files, protected_directories=protected_directories,
        )
        fsync_directory(staging_fd)
        checkpoint("after_profile_ownership")
        os.rename(staging_name, args.client, src_dir_fd=profile_root_fd, dst_dir_fd=profile_root_fd)
        created = False
        profile_published = True
        checkpoint("after_profile_publish")
        fsync_directory(profile_root_fd)
        checkpoint("after_profile_publish_fsync")
        os.rename(staging_name, args.client, src_dir_fd=proof_root_fd, dst_dir_fd=proof_root_fd)
        proof_created = False
        proof_published = True
        checkpoint("after_proof_publish")
        fsync_directory(proof_root_fd)
        checkpoint("after_proof_publish_fsync")
        write_transaction(
            profile_root_fd, marker_name, args.client, digest, profile_preexisting,
            phase="committed", previous_publication=publication_before, replace=True,
        )
        finalize_publication(profile_root_fd, lock_fd, marker_name, digest)
        transaction_created = False
        os.close(staging_fd)
        staging_fd = -1
    finally:
        try:
            if proof_fd >= 0:
                os.close(proof_fd)
                proof_fd = -1
            if staging_fd >= 0:
                os.close(staging_fd)
                staging_fd = -1
            if transaction_created:
                # Publish rollback intent before deleting any part of the installed
                # or staged trees.  Recovery can then resume the exact same path
                # after interruption instead of mistaking a partial rollback for a
                # forward commit.
                unlink_regular_if_present(profile_root_fd, f"{marker_name}.new")
                write_transaction(
                    profile_root_fd, marker_name, args.client, digest, profile_preexisting,
                    phase="rollback", previous_publication=publication_before, replace=True,
                )
            if created:
                remove_tree_durable(profile_root_fd, staging_name)
                checkpoint("after_rollback_profile_staging_removal")
            if proof_created:
                remove_tree_durable(proof_root_fd, staging_name)
                checkpoint("after_rollback_proof_staging_removal")
            if transaction_created:
                if profile_published:
                    remove_if_present(profile_root_fd, args.client)
                    checkpoint("after_rollback_profile_removal")
                if proof_published:
                    remove_if_present(proof_root_fd, args.client)
                    checkpoint("after_rollback_proof_removal")
                if target_info is not None:
                    restore_empty_profile(profile_root_fd, args.client, account.pw_uid, account.pw_gid)
                    checkpoint("after_rollback_empty_profile_restore")
                write_publication_record(lock_fd, publication_before.encode("ascii"))
                checkpoint("after_rollback_publication_restore")
                unlink_regular_if_present(profile_root_fd, f"{marker_name}.new")
                os.unlink(marker_name, dir_fd=profile_root_fd)
                fsync_directory(profile_root_fd)
                checkpoint("after_rollback_marker_cleanup_fsync")
        finally:
            for descriptor in (proof_fd, staging_fd, proof_root_fd, profile_root_fd, source_fd, lock_fd):
                if descriptor >= 0:
                    os.close(descriptor)
    print(f"isolated {args.client} profile provisioned; seed left untouched")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
