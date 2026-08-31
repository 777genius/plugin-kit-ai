#!/usr/bin/env python3
"""Fail-closed reset transaction for one disposable UAP observer VM.

The production CLI intentionally has no root/path overrides.  Tests construct a
``Layout.for_test`` and call ``ResetController`` directly; an operator can never
redirect the privileged command at an arbitrary tree.
"""

from __future__ import annotations

import argparse
import configparser
import dataclasses
import fcntl
import hashlib
import json
import os
import re
import shlex
import stat
import subprocess
import sys
from pathlib import Path
from typing import Any, Callable, Iterable, Mapping, Sequence


SCHEMA_VERSION = 1
SENTINEL_PURPOSE = "uap-observer-e2e-disposable"
DIGEST = re.compile(r"[0-9a-f]{64}\Z")
MACHINE_ID = re.compile(r"[0-9a-f]{32}\Z")
UNITS = (
    "uap-observer.service",
    "uap-observer-signer.service",
    "uap-observer-runner.service",
    "uap-observer-runner.socket",
    "uap-observer-caddy.service",
    "uap-observer-egress-proxy.service",
    "uap-observer-egress-proxy.socket",
)
PUBLIC_HOP_UNIT = "uap-observer-caddy-internal.service"
ALL_UNITS = (*UNITS, PUBLIC_HOP_UNIT)
MANAGED = (
    "/opt/uap-observer-current",
    "/opt/uap-observer-closures",
    "/var/lib/uap-observer",
    "/var/lib/uap-observer-human",
    "/var/lib/uap-observer-consent",
    *(f"/etc/systemd/system/{unit}" for unit in UNITS),
    "/etc/systemd/system/uap-observer.service.d",
    "/etc/systemd/system/uap-observer-runner.service.d",
)
PRESERVED = (
    "/etc/uap-observer-ed25519.key",
    "/opt/uap-observer-inputs",
    "/etc/caddy",
    "/var/lib/caddy",
    "/var/log/caddy",
    "/var/lib/caddy/uap-vm-internal-Caddyfile",
    "/var/lib/caddy/.local/share/caddy/pki/authorities/local/root.crt",
    "/var/lib/caddy/.local/share/caddy/pki/authorities/local/root.key",
    "/var/lib/caddy/.local/share/caddy/pki/authorities/local/intermediate.crt",
    "/var/lib/caddy/.local/share/caddy/pki/authorities/local/intermediate.key",
    "/root/caddy_2.11.4_linux_amd64.tar.gz",
    "/root/Caddyfile",
    "/root/uap-observer-adapter-config.json",
    "/root/uap-observer.json",
    "/root/uap-observer-egress-allowlist.json",
)
VOLATILE_PRESERVED = frozenset(("/var/lib/caddy", "/var/log/caddy"))
STABLE_HELPER = "/usr/local/libexec/uap-observer-reset"
STABLE_INSTALL_LIB = "/usr/local/libexec/uap-observer-reset-install-lib"
CA_AUTHORITY_ROOT = "/var/lib/caddy/.local/share/caddy/pki/authorities/local"
CA_FILES = frozenset(("root.crt", "root.key", "intermediate.crt", "intermediate.key"))
PREPARED_ROOT = "/root/uap-observer-reset-prepared-v1"
PREPARED_INVENTORY = ("evidence", "path", "projection-digests", "seeds")
PREPARED_PURPOSE = "uap-observer-same-vm-reset-prepared-v1"
PREPARED_TREE_DOMAIN = b"uap-observer-reset-prepared-tree-v1\0"
CLIENTS = ("codex", "cursor", "kiro")
PARTIALS = (
    "/opt/uap-observer-source.new",
    "/opt/uap-observer-source.new.resolved-tombstone",
    "/opt/uap-observer-venv.new",
    "/opt/uap-observer-runtime.new",
    "/opt/uap-observer-current.new",
    "/usr/local/libexec/uap-observer-runner.new",
    "/usr/local/libexec/uap-observer-egress-proxy.new",
    "/usr/local/libexec/uap-observer-fixed-adapter.new",
    "/usr/local/libexec/uap-observer-attest-chatgpt.new",
    "/usr/local/libexec/uap-observer-attest-consent.new",
    "/usr/local/libexec/uap-observer-provision-profile.new",
    "/usr/local/libexec/uap-observer-recover-profile-seed.new",
    "/usr/local/libexec/uap-observer-adapter-runtime.new",
    "/usr/local/libexec/uap-observer-adapter-notion.new",
    "/usr/local/libexec/uap-observer-adapter-chatgpt.new",
    "/usr/local/libexec/uap-observer-adapter-consent.new",
    "/usr/local/bin/caddy.new",
    "/etc/uap-observer.json.new",
    "/etc/uap-observer-egress-allowlist.json.new",
    "/etc/uap-observer-adapter-config.json.new",
    "/etc/uap-observer-adapters.json.new",
    "/etc/caddy/Caddyfile.new",
)
HARDLINKED_ADAPTER_PARTIALS = frozenset((
    "/usr/local/libexec/uap-observer-fixed-adapter.new",
    "/usr/local/libexec/uap-observer-adapter-runtime.new",
    "/usr/local/libexec/uap-observer-adapter-notion.new",
    "/usr/local/libexec/uap-observer-adapter-chatgpt.new",
    "/usr/local/libexec/uap-observer-adapter-consent.new",
))


class ResetError(RuntimeError):
    pass


class InjectedFailure(ResetError):
    pass


def strict_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    value: dict[str, Any] = {}
    folded: set[str] = set()
    for key, child in pairs:
        normalized = key.casefold()
        if key in value or normalized in folded:
            raise ResetError("duplicate or case-confusable JSON member")
        value[key] = child
        folded.add(normalized)
    return value


def strict_json(encoded: bytes) -> Any:
    def reject_constant(value: str) -> None:
        raise ResetError(f"non-finite JSON value {value}")

    return json.loads(
        encoded,
        object_pairs_hook=strict_object,
        parse_constant=reject_constant,
    )


@dataclasses.dataclass(frozen=True)
class Layout:
    root: Path | None
    expected_uid: int

    @classmethod
    def production(cls) -> "Layout":
        return cls(None, 0)

    @classmethod
    def for_test(cls, root: Path) -> "Layout":
        return cls(root.resolve(), os.getuid())

    def path(self, logical: str) -> Path:
        if not logical.startswith("/") or ".." in Path(logical).parts:
            raise ResetError("non-canonical managed path")
        if self.root is None:
            return Path(logical)
        return self.root / logical.removeprefix("/")


def metadata(path: Path) -> dict[str, Any]:
    info = os.lstat(path)
    kind = (
        "directory" if stat.S_ISDIR(info.st_mode)
        else "regular" if stat.S_ISREG(info.st_mode)
        else "symlink" if stat.S_ISLNK(info.st_mode)
        else "special"
    )
    result: dict[str, Any] = {
        "device": info.st_dev,
        "inode": info.st_ino,
        "kind": kind,
        "mode": stat.S_IMODE(info.st_mode),
        "uid": info.st_uid,
        "gid": info.st_gid,
        "nlink": info.st_nlink,
        "size": info.st_size,
        "mtime_ns": info.st_mtime_ns,
        "ctime_ns": info.st_ctime_ns,
    }
    if kind == "symlink":
        result["target"] = os.readlink(path)
    return result


def stable_metadata(value: Mapping[str, Any]) -> tuple[Any, ...]:
    return tuple(value.get(field) for field in (
        "device", "inode", "kind", "mode", "uid", "gid", "nlink", "size",
        "mtime_ns", "target",
    ))


def volatile_directory_identity(value: Mapping[str, Any]) -> tuple[Any, ...]:
    return tuple(value.get(field) for field in (
        "device", "inode", "kind", "mode", "uid", "gid",
    ))


def validate_root_control(path: Path, uid: int, mode: int, kind: str) -> dict[str, Any]:
    observed = metadata(path)
    if (
        observed["kind"] != kind
        or observed["uid"] != uid
        or observed["mode"] != mode
        or (kind == "regular" and observed["nlink"] != 1)
    ):
        raise ResetError(f"protected control path differs: {path}")
    return observed


def read_control(path: Path, uid: int, mode: int, maximum: int = 64 << 10) -> bytes:
    before = validate_root_control(path, uid, mode, "regular")
    flags = os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW
    if hasattr(os, "O_NOATIME"):
        flags |= os.O_NOATIME
    descriptor = os.open(path, flags)
    try:
        if stable_metadata(metadata_from_stat(os.fstat(descriptor))) != stable_metadata(before):
            raise ResetError(f"protected control path changed while opening: {path}")
        chunks: list[bytes] = []
        remaining = maximum + 1
        while remaining:
            block = os.read(descriptor, min(65536, remaining))
            if not block:
                break
            chunks.append(block)
            remaining -= len(block)
        encoded = b"".join(chunks)
        if len(encoded) > maximum:
            raise ResetError(f"protected control path is oversized: {path}")
        if stable_metadata(metadata_from_stat(os.fstat(descriptor))) != stable_metadata(before):
            raise ResetError(f"protected control path changed while reading: {path}")
        if stable_metadata(metadata(path)) != stable_metadata(before):
            raise ResetError(f"protected control pathname changed while reading: {path}")
        return encoded
    finally:
        os.close(descriptor)


def metadata_from_stat(info: os.stat_result) -> dict[str, Any]:
    kind = (
        "directory" if stat.S_ISDIR(info.st_mode)
        else "regular" if stat.S_ISREG(info.st_mode)
        else "symlink" if stat.S_ISLNK(info.st_mode)
        else "special"
    )
    return {
        "device": info.st_dev, "inode": info.st_ino, "kind": kind,
        "mode": stat.S_IMODE(info.st_mode), "uid": info.st_uid,
        "gid": info.st_gid, "nlink": info.st_nlink, "size": info.st_size,
        "mtime_ns": info.st_mtime_ns, "ctime_ns": info.st_ctime_ns,
    }


def fsync_directory(path: Path) -> None:
    descriptor = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def descriptor_mount_id(descriptor: int) -> str:
    """Return a mount identity for an already-open object.

    Linux exposes the VFS mount ID in fdinfo, which detects same-device bind
    mounts that st_dev cannot distinguish.  Tests on non-Linux hosts retain a
    device fallback; the production helper is Linux-only.
    """
    fdinfo = Path(f"/proc/self/fdinfo/{descriptor}")
    if sys.platform.startswith("linux"):
        try:
            record = os.open(fdinfo, os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW)
            try:
                encoded = os.read(record, 4097)
            finally:
                os.close(record)
        except OSError as error:
            raise ResetError("Linux mount identity record is unavailable") from error
        if len(encoded) > 4096:
            raise ResetError("Linux mount identity record is oversized")
        matches = re.findall(rb"(?m)^mnt_id:\s*([0-9]+)\s*$", encoded)
        if len(matches) != 1:
            raise ResetError("open object has no unique Linux mount identity")
        return "mnt:" + matches[0].decode("ascii")
    return f"dev:{os.fstat(descriptor).st_dev}"


def assert_same_mount_as_parent(path: Path, kind: str | None = None) -> str:
    parent = os.open(
        path.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
    )
    try:
        before = os.stat(path.name, dir_fd=parent, follow_symlinks=False)
        observed_kind = metadata_from_stat(before)["kind"]
        kind = kind or observed_kind
        flags = os.O_CLOEXEC | os.O_NOFOLLOW
        if kind == "directory":
            flags |= os.O_RDONLY | os.O_DIRECTORY
        elif kind == "regular":
            flags |= os.O_RDONLY
        elif hasattr(os, "O_PATH"):
            flags |= os.O_PATH
        else:
            child_mount = f"dev:{before.st_dev}"
            if child_mount != descriptor_mount_id(parent):
                raise ResetError(f"protected path is a mount boundary: {path}")
            return child_mount
        child = os.open(path.name, flags, dir_fd=parent)
        try:
            after = os.fstat(child)
            if (
                after.st_dev != before.st_dev or after.st_ino != before.st_ino
                or after.st_mode != before.st_mode
            ):
                raise ResetError(f"protected path changed while opening: {path}")
            child_mount = descriptor_mount_id(child)
        finally:
            os.close(child)
        if child_mount != descriptor_mount_id(parent):
            raise ResetError(f"protected path is a mount boundary: {path}")
    finally:
        os.close(parent)
    return child_mount


def cleanup_identity(value: Mapping[str, Any]) -> tuple[Any, ...]:
    """Bind leaf metadata without freezing partially deleted directories.

    Link counts may decrease during cleanup; regular-file aliases are checked
    separately by the descriptor-based, complete cleanup-scope inventory.
    """
    fields = (
        "device", "inode", "kind", "mode", "uid", "gid",
    )
    if value.get("kind") != "directory":
        fields += ("size", "mtime_ns", "target")
    return tuple(value.get(field) for field in fields)


def length_prefix(value: bytes) -> bytes:
    return len(value).to_bytes(8, "big") + value


def prepared_tree_digest(root: Path, expected_uid: int) -> str:
    digest = hashlib.sha256(PREPARED_TREE_DOMAIN)
    parent_descriptor = os.open(
        root.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
    )
    root_descriptor = os.open(
        root.name, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
        dir_fd=parent_descriptor,
    )
    root_mount = descriptor_mount_id(root_descriptor)
    try:
        if descriptor_mount_id(parent_descriptor) != root_mount:
            raise ResetError("prepared tree root is a mount boundary")
    except Exception:
        os.close(root_descriptor)
        os.close(parent_descriptor)
        raise
    count = 0

    def consume(descriptor: int, relative: bytes, before: os.stat_result) -> None:
        nonlocal count
        count += 1
        if count > 1_000_000:
            raise ResetError("prepared tree entry bound exceeded")
        if descriptor_mount_id(descriptor) != root_mount:
            raise ResetError("prepared tree crosses a mount boundary")
        current = os.fstat(descriptor)
        if (
            current.st_ino != before.st_ino
            or current.st_dev != before.st_dev
            or current.st_mode != before.st_mode
            or current.st_uid != expected_uid
            or stat.S_IMODE(current.st_mode) & 0o022
        ):
            raise ResetError("prepared tree ownership or identity differs")
        if stat.S_ISREG(current.st_mode) and current.st_nlink != 1:
            raise ResetError("prepared tree contains a hardlinked file")
        kind = b"directory" if stat.S_ISDIR(current.st_mode) else b"regular"
        fields = (
            relative, kind,
            f"{stat.S_IMODE(current.st_mode):04o}".encode(), str(current.st_uid).encode(),
            str(current.st_gid).encode(), str(current.st_nlink).encode(), str(current.st_size).encode(),
        )
        for field in fields:
            digest.update(length_prefix(field))
        if kind == b"directory":
            digest.update(length_prefix(b""))
            for display_name in sorted(os.listdir(descriptor), key=os.fsencode):
                name = os.fsencode(display_name)
                child_info = os.stat(name, dir_fd=descriptor, follow_symlinks=False)
                if stat.S_ISLNK(child_info.st_mode) or not (
                    stat.S_ISDIR(child_info.st_mode) or stat.S_ISREG(child_info.st_mode)
                ):
                    raise ResetError("prepared tree contains a link or special file")
                flags = os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW
                if stat.S_ISDIR(child_info.st_mode):
                    flags |= os.O_DIRECTORY
                child_descriptor = os.open(name, flags, dir_fd=descriptor)
                try:
                    child_relative = name if relative == b"." else relative + b"/" + name
                    consume(child_descriptor, child_relative, child_info)
                    if stable_metadata(metadata_from_stat(os.stat(
                        name, dir_fd=descriptor, follow_symlinks=False,
                    ))) != stable_metadata(metadata_from_stat(child_info)):
                        raise ResetError("prepared tree entry changed while reading")
                finally:
                    os.close(child_descriptor)
            return
        before_read = os.fstat(descriptor)
        contents = hashlib.sha256()
        size = 0
        os.lseek(descriptor, 0, os.SEEK_SET)
        while True:
            block = os.read(descriptor, 1 << 20)
            if not block:
                break
            contents.update(block)
            size += len(block)
        after_read = os.fstat(descriptor)
        def stable(info: os.stat_result) -> tuple[int, ...]:
            return (
                info.st_dev, info.st_ino, info.st_mode, info.st_nlink,
                info.st_uid, info.st_gid, info.st_size, info.st_mtime_ns,
            )
        if size != current.st_size or stable(before_read) != stable(after_read):
            raise ResetError("prepared file changed while reading")
        digest.update(length_prefix(str(size).encode()))
        digest.update(length_prefix(contents.digest()))

    try:
        root_info = os.fstat(root_descriptor)
        if not stat.S_ISDIR(root_info.st_mode):
            raise ResetError("prepared tree root is not a directory")
        consume(root_descriptor, b".", root_info)
        if stable_metadata(metadata_from_stat(os.stat(
            root.name, dir_fd=parent_descriptor, follow_symlinks=False,
        ))) != stable_metadata(metadata_from_stat(root_info)):
            raise ResetError("prepared tree root changed while reading")
    finally:
        os.close(root_descriptor)
        os.close(parent_descriptor)
    return "sha256:" + digest.hexdigest()


def regular_file_digest(path: Path, expected: Mapping[str, Any] | None = None) -> str:
    before = metadata(path)
    if before["kind"] != "regular" or before["nlink"] != 1:
        raise ResetError("preserved file is not a single-link regular file")
    if expected is not None and stable_metadata(before) != stable_metadata(expected):
        raise ResetError("preserved file metadata differs")
    flags = os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW
    if hasattr(os, "O_NOATIME"):
        flags |= os.O_NOATIME
    descriptor = os.open(path, flags)
    try:
        if stable_metadata(metadata_from_stat(os.fstat(descriptor))) != stable_metadata(before):
            raise ResetError("preserved file changed while opening")
        digest = hashlib.sha256()
        size = 0
        while True:
            block = os.read(descriptor, 1 << 20)
            if not block:
                break
            digest.update(block)
            size += len(block)
        if (
            size != before["size"]
            or stable_metadata(metadata_from_stat(os.fstat(descriptor))) != stable_metadata(before)
            or stable_metadata(metadata(path)) != stable_metadata(before)
        ):
            raise ResetError("preserved file changed while reading")
        return "sha256:" + digest.hexdigest()
    finally:
        os.close(descriptor)


def git_blob_oid(descriptor: int, expected: os.stat_result) -> str:
    try:
        digest = hashlib.sha1(usedforsecurity=False)
    except TypeError:  # pragma: no cover - compatibility with older Python
        digest = hashlib.sha1()
    digest.update(f"blob {expected.st_size}\0".encode("ascii"))
    size = 0
    os.lseek(descriptor, 0, os.SEEK_SET)
    while True:
        block = os.read(descriptor, 1 << 20)
        if not block:
            break
        digest.update(block)
        size += len(block)
    if (
        size != expected.st_size
        or stable_metadata(metadata_from_stat(os.fstat(descriptor)))
        != stable_metadata(metadata_from_stat(expected))
    ):
        raise ResetError("prepared source file changed while hashing Git identity")
    return digest.hexdigest()


def source_worktree_inventory(source: Path) -> tuple[dict[bytes, tuple[bool, str]], set[bytes]]:
    parent = os.open(
        source.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
    )
    root = os.open(
        source.name,
        os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
        dir_fd=parent,
    )
    root_mount = descriptor_mount_id(root)
    if descriptor_mount_id(parent) != root_mount:
        os.close(root)
        os.close(parent)
        raise ResetError("prepared source root is a mount boundary")
    files: dict[bytes, tuple[bool, str]] = {}
    directories: set[bytes] = set()
    count = 0

    def consume(descriptor: int, relative: bytes) -> None:
        nonlocal count
        if descriptor_mount_id(descriptor) != root_mount:
            raise ResetError("prepared source crosses a mount boundary")
        for display_name in sorted(os.listdir(descriptor), key=os.fsencode):
            name = os.fsencode(display_name)
            if relative == b"" and name == b".git":
                continue
            child_relative = name if not relative else relative + b"/" + name
            before = os.stat(name, dir_fd=descriptor, follow_symlinks=False)
            count += 1
            if count > 1_000_000:
                raise ResetError("prepared source entry bound exceeded")
            if stat.S_ISDIR(before.st_mode):
                child = os.open(
                    name,
                    os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
                    dir_fd=descriptor,
                )
                try:
                    if descriptor_mount_id(child) != root_mount:
                        raise ResetError("prepared source crosses a mount boundary")
                    if stable_metadata(metadata_from_stat(os.fstat(child))) != stable_metadata(
                        metadata_from_stat(before)
                    ):
                        raise ResetError("prepared source directory changed while opening")
                    directories.add(child_relative)
                    consume(child, child_relative)
                    if stable_metadata(metadata_from_stat(os.stat(
                        name, dir_fd=descriptor, follow_symlinks=False,
                    ))) != stable_metadata(metadata_from_stat(before)):
                        raise ResetError("prepared source directory changed while reading")
                finally:
                    os.close(child)
            elif stat.S_ISREG(before.st_mode):
                if before.st_nlink != 1:
                    raise ResetError("prepared source contains a hardlinked file")
                child = os.open(
                    name, os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW,
                    dir_fd=descriptor,
                )
                try:
                    if descriptor_mount_id(child) != root_mount:
                        raise ResetError("prepared source crosses a mount boundary")
                    opened = os.fstat(child)
                    if stable_metadata(metadata_from_stat(opened)) != stable_metadata(
                        metadata_from_stat(before)
                    ):
                        raise ResetError("prepared source file changed while opening")
                    files[child_relative] = (
                        bool(stat.S_IMODE(opened.st_mode) & 0o111),
                        git_blob_oid(child, opened),
                    )
                    if stable_metadata(metadata_from_stat(os.stat(
                        name, dir_fd=descriptor, follow_symlinks=False,
                    ))) != stable_metadata(metadata_from_stat(before)):
                        raise ResetError("prepared source file changed while reading")
                finally:
                    os.close(child)
            else:
                raise ResetError("prepared source contains a link or special file")

    try:
        consume(root, b"")
    finally:
        os.close(root)
        os.close(parent)
    return files, directories


def preserved_tree_digest(root: Path, excluded: frozenset[str] = frozenset()) -> str:
    digest = hashlib.sha256(b"uap-observer-reset-preserved-tree-v1\0")
    count = 0
    parent_descriptor = os.open(
        root.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
    )
    root_descriptor = os.open(
        root.name, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
        dir_fd=parent_descriptor,
    )
    root_mount = descriptor_mount_id(root_descriptor)
    try:
        if descriptor_mount_id(parent_descriptor) != root_mount:
            raise ResetError("preserved tree root is a mount boundary")
    except Exception:
        os.close(root_descriptor)
        os.close(parent_descriptor)
        raise

    excluded_bytes = frozenset(os.fsencode(value) for value in excluded)

    def consume(descriptor: int, relative: bytes, before: os.stat_result) -> None:
        nonlocal count
        if relative in excluded_bytes:
            return
        count += 1
        if count > 1_000_000:
            raise ResetError("preserved tree entry bound exceeded")
        info = os.fstat(descriptor)
        if descriptor_mount_id(descriptor) != root_mount:
            raise ResetError("preserved tree crosses a mount boundary")
        if (
            info.st_dev != before.st_dev or info.st_ino != before.st_ino
            or info.st_mode != before.st_mode
        ):
            raise ResetError("preserved tree identity changed while opening")
        if not (stat.S_ISDIR(info.st_mode) or stat.S_ISREG(info.st_mode)):
            raise ResetError("preserved tree contains a link or special file")
        if stat.S_ISREG(info.st_mode) and info.st_nlink != 1:
            raise ResetError("preserved tree contains a hardlinked file")
        kind = b"directory" if stat.S_ISDIR(info.st_mode) else b"regular"
        bound_nlink = 0 if kind == b"directory" else info.st_nlink
        bound_size = 0 if kind == b"directory" else info.st_size
        for field in (
            relative, kind, f"{stat.S_IMODE(info.st_mode):04o}".encode(),
            str(info.st_uid).encode(), str(info.st_gid).encode(), str(bound_nlink).encode(),
            str(bound_size).encode(),
        ):
            digest.update(length_prefix(field))
        if kind == b"regular":
            contents = hashlib.sha256()
            size = 0
            os.lseek(descriptor, 0, os.SEEK_SET)
            while True:
                block = os.read(descriptor, 1 << 20)
                if not block:
                    break
                contents.update(block)
                size += len(block)
            after = os.fstat(descriptor)
            if size != info.st_size or metadata_from_stat(after) != metadata_from_stat(info):
                raise ResetError("preserved file changed while reading")
            digest.update(length_prefix(("sha256:" + contents.hexdigest()).encode()))
        else:
            digest.update(length_prefix(b""))
            for display_name in sorted(os.listdir(descriptor), key=os.fsencode):
                name = os.fsencode(display_name)
                child_relative = name if relative == b"." else relative + b"/" + name
                if child_relative in excluded_bytes:
                    continue
                child_info = os.stat(name, dir_fd=descriptor, follow_symlinks=False)
                if stat.S_ISLNK(child_info.st_mode) or not (
                    stat.S_ISDIR(child_info.st_mode) or stat.S_ISREG(child_info.st_mode)
                ):
                    raise ResetError("preserved tree contains a link or special file")
                flags = os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW
                if stat.S_ISDIR(child_info.st_mode):
                    flags |= os.O_DIRECTORY
                child_descriptor = os.open(name, flags, dir_fd=descriptor)
                try:
                    consume(child_descriptor, child_relative, child_info)
                    if stable_metadata(metadata_from_stat(os.stat(
                        name, dir_fd=descriptor, follow_symlinks=False,
                    ))) != stable_metadata(metadata_from_stat(child_info)):
                        raise ResetError("preserved tree entry changed while reading")
                finally:
                    os.close(child_descriptor)

    try:
        root_info = os.fstat(root_descriptor)
        if not stat.S_ISDIR(root_info.st_mode):
            raise ResetError("preserved tree root is not a directory")
        consume(root_descriptor, b".", root_info)
        if stable_metadata(metadata_from_stat(os.stat(
            root.name, dir_fd=parent_descriptor, follow_symlinks=False,
        ))) != stable_metadata(metadata_from_stat(root_info)):
            raise ResetError("preserved tree root changed while reading")
    finally:
        os.close(root_descriptor)
        os.close(parent_descriptor)
    return "sha256:" + digest.hexdigest()


def caddy_hostname(encoded: str) -> str:
    matches = re.findall(
        r"(?m)^\s*([A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?)\s*\{\s*$",
        encoded,
    )
    hosts = [host for host in matches if "." in host and not host.replace(".", "").isdigit()]
    if len(hosts) != 1:
        raise ResetError("public Caddy hop must contain one exact hostname")
    return hosts[0].lower()


class Systemd:
    def _run(self, *arguments: str, check: bool = True) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["systemctl", *arguments], check=check, text=True,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )

    def state_details(self, unit: str) -> tuple[dict[str, bool], bool, bool]:
        active_result = self._run("is-active", unit, check=False)
        active_value = active_result.stdout.strip()
        if (active_result.returncode, active_value) == (0, "active"):
            active = True
            failed = False
            active_missing = False
        elif (active_result.returncode, active_value) == (3, "inactive"):
            active = False
            failed = False
            active_missing = False
        elif (active_result.returncode, active_value) == (3, "failed"):
            active = False
            failed = True
            active_missing = False
        elif (active_result.returncode, active_value) == (4, "inactive"):
            active = False
            failed = False
            active_missing = True
        else:
            raise ResetError(f"systemd active-state query failed for {unit}")
        enabled_result = self._run("is-enabled", unit, check=False)
        enabled_value = enabled_result.stdout.strip()
        if (enabled_result.returncode, enabled_value) == (0, "enabled"):
            enabled = True
        elif (enabled_result.returncode, enabled_value) in {
            (1, "disabled"), (0, "static"), (0, "indirect"),
            (0, "generated"), (0, "transient"), (1, "masked"),
        }:
            enabled = False
        elif (enabled_result.returncode, enabled_value) == (4, "not-found"):
            enabled = False
        else:
            raise ResetError(f"systemd enablement query failed for {unit}")
        missing = (enabled_result.returncode, enabled_value) == (4, "not-found")
        if active_missing != missing and not (not active and not failed and missing):
            raise ResetError(f"systemd unit-presence query disagrees for {unit}")
        if missing and (active or failed):
            raise ResetError(f"systemd missing unit has an active state: {unit}")
        return {"active": active, "enabled": enabled}, failed, missing

    def state_with_failure(self, unit: str) -> tuple[dict[str, bool], bool]:
        state, failed, _missing = self.state_details(unit)
        return state, failed

    def state(self, unit: str) -> dict[str, bool]:
        return self.state_details(unit)[0]

    def is_failed(self, unit: str) -> bool:
        return self.state_details(unit)[1]

    def stop(self, units: Sequence[str]) -> None:
        self._run("stop", *units)

    def start(self, units: Sequence[str]) -> None:
        if units:
            self._run("start", *units)

    def enable(self, units: Sequence[str]) -> None:
        if units:
            self._run("enable", *units)

    def disable(self, units: Sequence[str]) -> None:
        if units:
            self._run("disable", *units)

    def daemon_reload(self) -> None:
        self._run("daemon-reload")

    def reset_failed(self, units: Sequence[str]) -> None:
        if units:
            self._run("reset-failed", *units)

    def _property(self, unit: str, name: str) -> str:
        return self._run("show", unit, f"--property={name}", "--value").stdout.strip()

    def public_hop_contract(self) -> dict[str, str]:
        names = (
            "FragmentPath", "Transient", "Type", "Restart", "User", "Group",
            "ExecStart", "PrivateTmp", "ProtectSystem", "ProtectHome",
            "ReadWritePaths", "ReadOnlyPaths", "AmbientCapabilities",
            "CapabilityBoundingSet", "NoNewPrivileges", "LimitNOFILE",
            "Description",
        )
        return {name: self._property(PUBLIC_HOP_UNIT, name) for name in names}

    @staticmethod
    def _normalized_exec_start(value: str) -> list[str]:
        expected = [
            "/opt/uap-observer-current/bin/caddy", "run", "--environ", "--config",
            "/var/lib/caddy/uap-vm-internal-Caddyfile", "--adapter", "caddyfile",
        ]
        if value.startswith("{"):
            path = re.search(r"(?:^|[;{])\s*path=([^ ;]+)", value)
            arguments = re.search(r"(?:^|;)\s*argv\[\]=(.*?)\s*;\s*ignore_errors=", value)
            ignored = re.search(r"(?:^|;)\s*ignore_errors=([^ ;}]+)", value)
            if path is None or arguments is None or ignored is None or ignored.group(1) != "no":
                raise ResetError("transient public Caddy hop command encoding differs")
            try:
                parsed = shlex.split(arguments.group(1))
            except ValueError as error:
                raise ResetError("transient public Caddy hop command encoding differs") from error
            if path.group(1) != expected[0] or parsed != expected:
                raise ResetError("transient public Caddy hop command differs")
            return expected
        try:
            parsed = shlex.split(value)
        except ValueError as error:
            raise ResetError("transient public Caddy hop command encoding differs") from error
        if parsed != expected:
            raise ResetError("transient public Caddy hop command differs")
        return expected

    def validate_public_hop_contract(self) -> dict[str, Any]:
        contract = self.public_hop_contract()
        scalar = {
            "FragmentPath": "/run/systemd/transient/uap-observer-caddy-internal.service",
            "Transient": "yes",
            "Type": "notify",
            "Restart": "no",
            "User": "caddy",
            "Group": "caddy",
            "PrivateTmp": "yes",
            "ProtectSystem": "strict",
            "ProtectHome": "yes",
            "AmbientCapabilities": "cap_net_bind_service",
            "CapabilityBoundingSet": "cap_net_bind_service",
            "NoNewPrivileges": "yes",
            "LimitNOFILE": "1048576",
            "Description": "Disposable UAP VM internal-CA ingress",
        }
        if any(contract.get(key) != expected for key, expected in scalar.items()):
            raise ResetError("transient public Caddy hop properties differ")
        normalized_exec = self._normalized_exec_start(contract["ExecStart"])
        read_write = contract["ReadWritePaths"].split()
        read_only = contract["ReadOnlyPaths"].split()
        if read_write != ["/var/lib/caddy", "/var/log/caddy"]:
            raise ResetError("transient public Caddy hop write paths differ")
        if read_only != ["/opt/uap-observer-current"]:
            raise ResetError("transient public Caddy hop read-only path differs")
        return {
            **scalar,
            "ExecStart": normalized_exec,
            "ReadWritePaths": read_write,
            "ReadOnlyPaths": read_only,
        }

    def public_hop_pid(self) -> int:
        value = self._property(PUBLIC_HOP_UNIT, "MainPID")
        if not value.isdecimal() or value == "0":
            raise ResetError("public Caddy hop has no main process")
        return int(value)

    def recreate_public_hop(self, closure: str) -> None:
        executable = f"/opt/uap-observer-closures/{closure}/bin/caddy"
        current = Path("/opt/uap-observer-current/bin/caddy")
        if not current.exists() or current.resolve() != Path(executable):
            raise ResetError("public Caddy executable is not the exact current closure")
        command = (
            "systemd-run", "--unit=uap-observer-caddy-internal.service",
            "--description=Disposable UAP VM internal-CA ingress",
            "--service-type=notify", "--uid=caddy", "--gid=caddy",
            "--property=Restart=no", "--property=PrivateTmp=yes",
            "--property=ProtectSystem=strict", "--property=ProtectHome=yes",
            "--property=ReadWritePaths=/var/lib/caddy /var/log/caddy",
            "--property=ReadOnlyPaths=/opt/uap-observer-current",
            "--property=AmbientCapabilities=cap_net_bind_service",
            "--property=CapabilityBoundingSet=cap_net_bind_service",
            "--property=NoNewPrivileges=yes", "--property=LimitNOFILE=1048576",
            "/opt/uap-observer-current/bin/caddy", "run", "--environ", "--config",
            "/var/lib/caddy/uap-vm-internal-Caddyfile", "--adapter", "caddyfile",
        )
        subprocess.run(command, check=True, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if not self.state(PUBLIC_HOP_UNIT)["active"]:
            raise ResetError("recreated public Caddy hop is not active")
        self.validate_public_hop_contract()
        main_pid = str(self.public_hop_pid())
        if Path(f"/proc/{main_pid}/exe").resolve() != Path(executable):
            raise ResetError("public Caddy hop process is not using the new closure")

    def verify_public_hop(self, closure: str) -> None:
        self.validate_public_hop_contract()
        executable = Path(f"/opt/uap-observer-closures/{closure}/bin/caddy")
        main_pid = self.public_hop_pid()
        if Path(f"/proc/{main_pid}/exe").resolve() != executable:
            raise ResetError("public Caddy hop process is not using the expected closure")
        config_path = Path("/var/lib/caddy/uap-vm-internal-Caddyfile")
        config = config_path.read_text(encoding="utf-8")
        host = caddy_hostname(config)
        ca_root = Path("/var/lib/caddy/.local/share/caddy/pki/authorities/local/root.crt")
        ca = metadata(ca_root)
        if ca["kind"] != "regular" or ca["nlink"] != 1 or ca["mode"] & 0o022:
            raise ResetError("public Caddy internal CA metadata differs")
        regular_file_digest(ca_root, ca)
        environment = {
            key: value for key, value in os.environ.items()
            if key.lower() not in {
                "http_proxy", "https_proxy", "all_proxy", "no_proxy"
            }
        }
        result = subprocess.run(
            ["curl", "--noproxy", "*", "--cacert", str(ca_root), "--silent", "--show-error", "--output", "/dev/null",
             "--connect-timeout", "5", "--max-time", "15", "--write-out", "%{http_code}",
             "--resolve", f"{host}:443:127.0.0.1", f"https://{host}/v1/stable-launch/observe"],
            check=True, text=True, env=environment,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )
        if result.stdout != "404":
            raise ResetError("public Caddy hop did not reach the observer GET endpoint")


class RuntimeProbe:
    def __init__(self, layout: Layout):
        self.layout = layout

    def assert_quiescent(self) -> None:
        cgroup = self.layout.path("/sys/fs/cgroup/system.slice/uap-observer-runner.service")
        if cgroup.exists():
            for path in cgroup.rglob("uap-job-*"):
                raise ResetError(f"observer runner job cgroup remains: {path.name}")
            procs = cgroup / "cgroup.procs"
            if procs.exists() and procs.read_text().strip():
                raise ResetError("observer runner cgroup still contains processes")
        proc = self.layout.path("/proc")
        if proc.exists():
            for child in proc.iterdir():
                if not child.name.isdecimal():
                    continue
                try:
                    cgroups = (child / "cgroup").read_bytes()
                    command = (child / "cmdline").read_bytes()
                except (FileNotFoundError, PermissionError, ProcessLookupError):
                    continue
                if b"uap-observer" in cgroups or b"/opt/uap-observer-current/" in command:
                    raise ResetError("observer runtime process remains")
        self.assert_no_pending_work()

    def assert_no_pending_work(self) -> None:
        for logical in (
            "/var/lib/uap-observer/jobs",
            "/var/lib/uap-observer-human/pending",
            "/var/lib/uap-observer-human/reserved",
            "/var/lib/uap-observer-consent/pending",
            "/var/lib/uap-observer-consent/reserved",
        ):
            path = self.layout.path(logical)
            if path.exists() and any(path.iterdir()):
                raise ResetError(f"observer pending work remains: {logical}")


class PreparedExecutor:
    def __init__(self, layout: Layout, systemd: Systemd):
        self.layout = layout
        self.systemd = systemd

    def _run(self, arguments: Sequence[str], *, capture: bool = False) -> subprocess.CompletedProcess[str]:
        environment = os.environ.copy()
        environment["UAP_OBSERVER_INSTALL_LOCK_FD"] = "9"
        return subprocess.run(
            list(arguments), check=True, text=True, env=environment, pass_fds=(9,),
            stdout=subprocess.PIPE if capture else None,
            stderr=subprocess.PIPE if capture else None,
        )

    def prepare_new(self, controller: "ResetController", journal: Mapping[str, Any]) -> dict[str, str]:
        prepared = self.layout.path(PREPARED_ROOT)
        manifest = journal["prepared"]["manifest"]
        installer = manifest["installer"]
        source = prepared / installer["source_root"]
        command = (
            str(source / "deploy/uap-observer-install.sh"), str(source),
            str(prepared / installer["adapter_config"]), installer["adapter_sha256"],
            str(prepared / installer["observer_config"]), installer["observer_sha256"],
            str(prepared / installer["caddy_archive"]),
            str(prepared / installer["caddy_config"]), installer["caddy_config_sha256"],
            str(prepared / installer["egress_allowlist"]), installer["egress_sha256"],
        )
        self._run(command)
        current = self.layout.path("/opt/uap-observer-current")
        target = os.readlink(current)
        prefix = "uap-observer-closures/"
        if not target.startswith(prefix) or DIGEST.fullmatch(target.removeprefix(prefix)) is None:
            raise ResetError("new observer current pointer is invalid")
        closure = target.removeprefix(prefix)
        controller._validate_install(manifest["new_install_identity"], closure)
        helper = current / "libexec/uap-observer-provision-profile"
        for client in CLIENTS:
            contract = manifest["clients"][client]
            seed = prepared / contract["seed"]
            shown = self._run((
                str(helper), "--client", client, "--root-owned-seed", str(seed),
                "--seed-digest", "show",
            ), capture=True).stdout.strip()
            if shown != contract["seed_digest"]:
                raise ResetError(f"prepared seed digest differs: {client}")
            self._run((
                str(helper), "--client", client, "--root-owned-seed", str(seed),
                "--seed-digest", contract["seed_digest"],
            ))
        library = source / "deploy/uap-observer-install-lib.sh"
        validation = (
            'set -eu; . "$1"; '
            'observer_validate_installed_accounts_and_state "$2" "$2/runtime"; '
            'observer_validate_protected_inputs "$2" "$2/runtime"'
        )
        self._run(("/bin/sh", "-c", validation, "reset-validation", str(library), str(current)))
        controller._activate_recorded_units(journal, closure)
        self._verify_listeners(journal)
        self._verify_egress_proxy(closure, journal)
        return {"install_identity": manifest["new_install_identity"], "closure_digest": closure}

    def verify_new(self, controller: "ResetController", journal: Mapping[str, Any]) -> None:
        new = journal.get("new")
        if not isinstance(new, dict):
            raise ResetError("new observer install is not journaled")
        controller._validate_install(new["install_identity"], new["closure_digest"])
        controller._verify_recorded_units(journal, new["closure_digest"])
        self._verify_listeners(journal)
        self._verify_egress_proxy(new["closure_digest"], journal)

    def _verify_egress_proxy(self, closure: str, journal: Mapping[str, Any]) -> None:
        expected = journal["units"]
        if not (
            expected["uap-observer-egress-proxy.service"]["active"]
            or expected["uap-observer-egress-proxy.socket"]["active"]
        ):
            return
        allowlist = self.layout.path(
            f"/opt/uap-observer-closures/{closure}/etc/uap-observer-egress-allowlist.json"
        )
        mode = stat.S_IMODE(os.lstat(allowlist).st_mode)
        value = strict_json(read_control(
            allowlist, self.layout.expected_uid, mode, 64 << 10,
        ))
        if (
            not isinstance(value, dict)
            or set(value) != {"schema_version", "hosts"}
            or value.get("schema_version") != 1
            or not isinstance(value.get("hosts"), list)
            or any(not isinstance(host, str) for host in value["hosts"])
            or "api.github.com" not in value["hosts"]
        ):
            raise ResetError("installed egress allowlist cannot prove the readiness target")
        environment = {
            key: child for key, child in os.environ.items()
            if key.lower() not in {"http_proxy", "https_proxy", "all_proxy", "no_proxy"}
        }
        completed = subprocess.run(
            [
                "curl", "--silent", "--show-error", "--output", "/dev/null",
                "--connect-timeout", "5", "--max-time", "15", "--write-out", "%{http_code}",
                "--proxy", "http://127.0.0.2:8766", "--noproxy", "",
                "https://api.github.com/",
            ],
            check=True, text=True, env=environment,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )
        if not completed.stdout.isdecimal() or completed.stdout == "000":
            raise ResetError("observer egress proxy readiness probe did not respond")

    def _verify_listeners(self, journal: Mapping[str, Any]) -> None:
        completed = subprocess.run(
            ["ss", "-H", "-lntp"], check=True, text=True,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )
        listeners: dict[int, set[str]] = {80: set(), 443: set(), 8765: set(), 8766: set()}
        public_pids: set[int] = set()
        for line in completed.stdout.splitlines():
            fields = line.split()
            if len(fields) < 4:
                continue
            address = fields[3]
            match = re.search(r"(?:\[([^]]+)\]|([^:]+)):(\d+)\Z", address)
            if match and int(match.group(3)) in listeners:
                port = int(match.group(3))
                listeners[port].add(match.group(1) or match.group(2))
                if port in (80, 443):
                    public_pids.update(int(value) for value in re.findall(r"pid=(\d+)", line))
        observer_expected = journal["units"]["uap-observer.service"]["active"]
        egress_expected = journal["units"]["uap-observer-egress-proxy.socket"]["active"]
        if listeners[8765] != ({"127.0.0.1"} if observer_expected else set()):
            raise ResetError("observer private listener inventory differs")
        if listeners[8766] != ({"127.0.0.2"} if egress_expected else set()):
            raise ResetError("observer private listener inventory differs")
        public_expected = journal["units"][PUBLIC_HOP_UNIT]["active"]
        if public_expected:
            if not listeners[80] or not listeners[443]:
                raise ResetError("observer public listener is absent")
            if public_pids != {self.systemd.public_hop_pid()}:
                raise ResetError("public ports are not owned only by transient Caddy ingress")
        elif listeners[80] or listeners[443]:
            raise ResetError("unexpected observer public listener is active")


class ResetController:
    def __init__(
        self,
        layout: Layout,
        systemd: Systemd | Any | None = None,
        runtime_probe: RuntimeProbe | Any | None = None,
        executor: PreparedExecutor | Any | None = None,
        failpoint: Callable[[str], None] | None = None,
        closure_identity: Callable[[Path], str] | None = None,
        source_revision: Callable[[Path], str] | None = None,
    ) -> None:
        self.layout = layout
        self.systemd = systemd or Systemd()
        self.runtime_probe = runtime_probe or RuntimeProbe(layout)
        self.executor = executor or PreparedExecutor(layout, self.systemd)
        self.failpoint = failpoint or (lambda _name: None)
        self.closure_identity = closure_identity or self._production_closure_identity
        self.source_revision = source_revision or self._production_source_revision
        self.journal_dir = layout.path("/var/lib/uap-observer-reset")
        self.journal_path = self.journal_dir / "journal.json"
        self.journal_temporary = self.journal_dir / ".journal.json.new"
        self.completion_path = layout.path("/var/lib/uap-observer-reset.completed")

    def locked(self):
        controller = self

        class Lock:
            def __enter__(self) -> int:
                lock_dir = controller.layout.path("/run/lock")
                lock_dir.mkdir(parents=True, exist_ok=True)
                lock_path = lock_dir / "uap-observer-install.lock"
                inherited = os.environ.get("UAP_OBSERVER_INSTALL_LOCK_FD")
                if inherited not in (None, "", "9"):
                    raise ResetError("observer install lock FD must be the reviewed descriptor 9")
                if inherited == "9":
                    try:
                        resolved = Path("/proc/self/fd/9").resolve(strict=True)
                    except (FileNotFoundError, OSError) as error:
                        raise ResetError("inherited observer install lock FD is unavailable") from error
                    if resolved != lock_path.resolve():
                        raise ResetError("inherited observer install lock FD points elsewhere")
                    self.descriptor = 9
                    self.owned = False
                else:
                    descriptor = os.open(
                        lock_path,
                        os.O_RDWR | os.O_CREAT | os.O_CLOEXEC | os.O_NOFOLLOW,
                        0o600,
                    )
                    self.owned = True
                    if controller.layout.root is None and descriptor != 9:
                        os.dup2(descriptor, 9, inheritable=True)
                        os.close(descriptor)
                        descriptor = 9
                    self.descriptor = descriptor
                try:
                    fcntl.flock(self.descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
                except BlockingIOError as error:
                    if self.owned:
                        os.close(self.descriptor)
                    raise ResetError("another observer install or reset is active") from error
                try:
                    observed = metadata(lock_path)
                    opened = metadata_from_stat(os.fstat(self.descriptor))
                    if (
                        observed["kind"] != "regular" or observed["nlink"] != 1
                        or observed["uid"] != controller.layout.expected_uid
                        or stable_metadata(opened) != stable_metadata(observed)
                    ):
                        raise ResetError("observer install lock path metadata differs")
                    # Older standalone installs created 0644 under umask 022.
                    # Normalize the held inode, never replace a live lock file.
                    os.fchmod(self.descriptor, 0o600)
                except BaseException:
                    if self.owned:
                        os.close(self.descriptor)
                    raise
                return self.descriptor

            def __exit__(self, *_args: Any) -> None:
                if self.owned:
                    os.close(self.descriptor)

        return Lock()

    def _machine(self, expected: str) -> None:
        if MACHINE_ID.fullmatch(expected) is None:
            raise ResetError("machine-id must be 32 lowercase hexadecimal characters")
        actual = read_control(
            self.layout.path("/etc/machine-id"), self.layout.expected_uid, 0o444, 4096,
        ).decode("ascii").strip()
        if actual != expected:
            raise ResetError("machine-id differs from the approved disposable VM")
        sentinel = strict_json(read_control(
            self.layout.path("/etc/uap-observer-disposable.json"),
            self.layout.expected_uid, 0o600,
        ))
        expected_sentinel = {
            "machine_id": expected,
            "purpose": SENTINEL_PURPOSE,
            "schema_version": SCHEMA_VERSION,
        }
        if sentinel != expected_sentinel:
            raise ResetError("disposable observer sentinel differs")

    def _ensure_stable_file(
        self, source: Path, logical_target: str, mode: int, *, allow_replace: bool,
    ) -> str:
        source_meta = validate_root_control(source, self.layout.expected_uid, mode, "regular")
        encoded = read_control(source, self.layout.expected_uid, mode, 4 << 20)
        digest = "sha256:" + hashlib.sha256(encoded).hexdigest()
        target = self.layout.path(logical_target)
        parent = target.parent
        validate_root_control(
            parent, self.layout.expected_uid, stat.S_IMODE(os.lstat(parent).st_mode),
            "directory",
        )
        temporary = target.with_name(target.name + ".new")
        if target.exists() or target.is_symlink():
            observed_target = validate_root_control(
                target, self.layout.expected_uid, mode, "regular",
            )
            observed_digest = regular_file_digest(target, observed_target)
            if observed_digest == digest:
                self._assert_stable_file(logical_target, mode, digest)
                if temporary.exists() or temporary.is_symlink():
                    if regular_file_digest(temporary) != digest:
                        raise ResetError("stable observer bootstrap temporary content differs")
                    temporary.unlink()
                    fsync_directory(parent)
                return digest
            if not allow_replace:
                raise ResetError("stable observer bootstrap belongs to another transaction")
        if temporary.exists() or temporary.is_symlink():
            if regular_file_digest(temporary) != digest:
                raise ResetError("stable observer bootstrap temporary content differs")
        else:
            descriptor = os.open(
                temporary,
                os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC | os.O_NOFOLLOW,
                mode,
            )
            try:
                view = memoryview(encoded)
                while view:
                    written = os.write(descriptor, view)
                    if written <= 0:
                        raise ResetError("short stable observer bootstrap write")
                    view = view[written:]
                os.fchmod(descriptor, stat.S_IMODE(source_meta["mode"]))
                os.fsync(descriptor)
            finally:
                os.close(descriptor)
        os.replace(temporary, target)
        fsync_directory(parent)
        self._assert_stable_file(logical_target, mode, digest)
        return digest

    def _ensure_stable_bootstrap(
        self, source_root: Path, *, allow_replace: bool,
    ) -> tuple[str, str]:
        source_helper = source_root / "deploy/uap-observer-reset.py"
        source_library = source_root / "deploy/uap-observer-install-lib.sh"
        helper = self._ensure_stable_file(
            source_helper, STABLE_HELPER, 0o755, allow_replace=allow_replace,
        )
        self.failpoint("after-stable-helper")
        library = self._ensure_stable_file(
            source_library, STABLE_INSTALL_LIB, 0o644, allow_replace=allow_replace,
        )
        self.failpoint("after-stable-install-lib")
        return helper, library

    def _assert_stable_file(self, logical: str, mode: int, expected_digest: str) -> None:
        if not isinstance(expected_digest, str) or re.fullmatch(
            r"sha256:[0-9a-f]{64}", expected_digest,
        ) is None:
            raise ResetError("stable observer bootstrap digest differs")
        target = self.layout.path(logical)
        observed = validate_root_control(
            target, self.layout.expected_uid, mode, "regular",
        )
        if regular_file_digest(target, observed) != expected_digest:
            raise ResetError("stable observer bootstrap content differs")

    def _assert_stable_bootstrap(self, journal: Mapping[str, Any]) -> None:
        self._assert_stable_file(STABLE_HELPER, 0o755, journal["helper_sha256"])
        self._assert_stable_file(
            STABLE_INSTALL_LIB, 0o644, journal["install_lib_sha256"],
        )

    def _production_closure_identity(self, closure: Path) -> str:
        completed = subprocess.run(
            [
                "/bin/sh", "-c",
                '. "$1"; observer_closure_identity "$2"',
                "reset-closure-identity", str(self.layout.path(STABLE_INSTALL_LIB)),
                str(closure),
            ],
            check=True, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )
        value = completed.stdout.strip()
        if DIGEST.fullmatch(value) is None:
            raise ResetError("observer closure identity output differs")
        return value

    def _production_source_revision(self, source: Path) -> str:
        git_directory = source / ".git"
        git_metadata = metadata(git_directory)
        if (
            git_metadata["kind"] != "directory"
            or git_metadata["uid"] != self.layout.expected_uid
            or git_metadata["mode"] & 0o022
        ):
            raise ResetError("prepared source Git authority is not an in-tree root-owned directory")
        assert_same_mount_as_parent(git_directory, "directory")
        for external_authority in (
            git_directory / "commondir",
            git_directory / "objects/info/alternates",
            git_directory / "info/grafts",
            git_directory / "shallow",
            git_directory / "refs/replace",
        ):
            if external_authority.exists() or external_authority.is_symlink():
                raise ResetError("prepared source Git authority references external state")
        if any(git_directory.glob("objects/pack/*.promisor")):
            raise ResetError("prepared source Git authority is partial")
        packed_refs = git_directory / "packed-refs"
        if packed_refs.exists() or packed_refs.is_symlink():
            packed_mode = stat.S_IMODE(os.lstat(packed_refs).st_mode)
            packed_body = read_control(
                packed_refs, self.layout.expected_uid, packed_mode, 16 << 20,
            )
            if b" refs/replace/" in packed_body:
                raise ResetError("prepared source Git authority contains replacement refs")
        for child in source.iterdir():
            if child.name == ".git":
                continue
            if child.is_dir() and any(path.name == ".git" for path in child.rglob(".git")):
                raise ResetError("prepared source contains a linked nested Git checkout")
        config_worktree = git_directory / "config.worktree"
        if config_worktree.exists() or config_worktree.is_symlink():
            raise ResetError("prepared source Git authority contains worktree-local config")
        config_path = git_directory / "config"
        config_mode = stat.S_IMODE(os.lstat(config_path).st_mode)
        config_encoded = read_control(
            config_path, self.layout.expected_uid, config_mode, 256 << 10,
        )
        parser = configparser.RawConfigParser(
            interpolation=None, strict=True, empty_lines_in_values=False,
        )
        parser.optionxform = str.lower
        try:
            config_body = config_encoded.decode("utf-8")
            parser.read_string(config_body)
        except (configparser.Error, UnicodeDecodeError) as error:
            raise ResetError("prepared source local Git config is not canonical and inert") from error
        allowed = {
            "core": {
                "repositoryformatversion", "filemode", "bare", "logallrefupdates",
                "ignorecase", "precomposeunicode", "symlinks",
            },
            "remote": {"url", "fetch"},
            "branch": {"remote", "merge"},
        }
        for section in parser.sections():
            if re.fullmatch(
                r'(core|remote "[^"\r\n]+"|branch "[^"\r\n]+")', section,
            ) is None:
                raise ResetError("prepared source local Git config has an unapproved section")
            family = section.split(" ", 1)[0].lower()
            if set(parser[section]) - allowed[family]:
                raise ResetError(
                    "prepared source local Git config has an executable or path-bearing key"
                )
        core = parser["core"] if parser.has_section("core") else None
        if (
            core is None
            or core.get("repositoryformatversion") != "0"
            or core.get("bare", "false").lower() != "false"
        ):
            raise ResetError("prepared source local Git config is not a normal checkout")
        environment = {
            key: value for key, value in os.environ.items()
            if not key.startswith("GIT_")
        }
        environment["GIT_CONFIG_NOSYSTEM"] = "1"
        environment["GIT_CONFIG_GLOBAL"] = "/dev/null"
        environment["GIT_OPTIONAL_LOCKS"] = "0"
        environment["GIT_NO_REPLACE_OBJECTS"] = "1"
        git_prefix = (
            "git", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null",
            "-C", str(source),
        )
        absolute_git = subprocess.run(
            [*git_prefix, "rev-parse", "--absolute-git-dir"],
            check=True, text=True, env=environment,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        ).stdout.strip()
        common_git = subprocess.run(
            [*git_prefix, "rev-parse", "--git-common-dir"],
            check=True, text=True, env=environment,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        ).stdout.strip()
        common_path = Path(common_git)
        if not common_path.is_absolute():
            common_path = source / common_path
        expected_git = git_directory.resolve()
        if Path(absolute_git).resolve() != expected_git or common_path.resolve() != expected_git:
            raise ResetError("prepared source Git authority escapes the prepared checkout")
        top = subprocess.run(
            [*git_prefix, "rev-parse", "--show-toplevel"],
            check=True, text=True, env=environment,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        ).stdout.strip()
        head = subprocess.run(
            [*git_prefix, "rev-parse", "HEAD"],
            check=True, text=True, env=environment,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        ).stdout.strip()
        object_format = subprocess.run(
            [*git_prefix, "rev-parse", "--show-object-format"],
            check=True, text=True, env=environment,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        ).stdout.strip()
        if (
            Path(top).resolve() != source.resolve()
            or re.fullmatch(r"[0-9a-f]{40}", head) is None
            or object_format != "sha1"
        ):
            raise ResetError("prepared source checkout is not exact and clean")
        subprocess.run(
            [*git_prefix, "fsck", "--full", "--strict", "--no-reflogs", "HEAD"],
            check=True, env=environment,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )
        tree = subprocess.run(
            [*git_prefix, "ls-tree", "-rz", "--full-tree", "HEAD"],
            check=True, env=environment,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        ).stdout
        expected_files: dict[bytes, tuple[bool, str]] = {}
        expected_directories: set[bytes] = set()
        for record in tree.split(b"\0"):
            if not record:
                continue
            try:
                header, path = record.split(b"\t", 1)
                mode, kind, object_id = header.split(b" ")
            except ValueError as error:
                raise ResetError("prepared source HEAD tree encoding differs") from error
            if (
                kind != b"blob"
                or mode not in {b"100644", b"100755"}
                or re.fullmatch(rb"[0-9a-f]{40}", object_id) is None
                or path == b".git"
                or path.startswith(b".git/")
                or path in expected_files
            ):
                raise ResetError("prepared source HEAD tree contains an unsupported entry")
            expected_files[path] = (mode == b"100755", object_id.decode("ascii"))
            components = path.split(b"/")[:-1]
            for index in range(1, len(components) + 1):
                expected_directories.add(b"/".join(components[:index]))
        observed_files, observed_directories = source_worktree_inventory(source)
        if observed_files != expected_files or observed_directories != expected_directories:
            raise ResetError("prepared source worktree differs from exact HEAD tree")
        return head

    def _validate_ids(self, install_identity: str, closure_digest: str) -> None:
        if DIGEST.fullmatch(install_identity) is None:
            raise ResetError("install identity must be 64 lowercase hexadecimal characters")
        if DIGEST.fullmatch(closure_digest) is None:
            raise ResetError("closure digest must be 64 lowercase hexadecimal characters")

    def _validate_install_markers(self, install_identity: str, closure_digest: str) -> Path:
        self._validate_ids(install_identity, closure_digest)
        current = self.layout.path("/opt/uap-observer-current")
        current_meta = metadata(current)
        expected_target = f"uap-observer-closures/{closure_digest}"
        if (
            current_meta["kind"] != "symlink"
            or current_meta["uid"] != self.layout.expected_uid
            or current_meta.get("target") != expected_target
        ):
            raise ResetError("observer current pointer differs from the expected closure")
        closures = self.layout.path("/opt/uap-observer-closures")
        validate_root_control(closures, self.layout.expected_uid, 0o755, "directory")
        assert_same_mount_as_parent(closures, "directory")
        names = sorted(path.name for path in closures.iterdir())
        if names != [closure_digest]:
            raise ResetError("observer closure inventory differs")
        closure = closures / closure_digest
        validate_root_control(closure, self.layout.expected_uid, 0o755, "directory")
        assert_same_mount_as_parent(closure, "directory")
        complete = read_control(closure / ".complete", self.layout.expected_uid, 0o644).decode().strip()
        identity = read_control(
            closure / ".install-identity", self.layout.expected_uid, 0o644,
        ).decode().strip()
        if complete != "complete-v1" or identity != install_identity:
            raise ResetError("observer closure markers differ")
        self._validate_deployed_systemd(closure)
        return closure

    def _validate_deployed_systemd(self, closure: Path) -> None:
        expected = sorted([
            *UNITS, "uap-observer.service.d", "uap-observer-runner.service.d",
        ])
        deployed = self.layout.path("/etc/systemd/system")
        reviewed = closure / "systemd"
        validate_root_control(deployed, self.layout.expected_uid, 0o755, "directory")
        validate_root_control(reviewed, self.layout.expected_uid, 0o755, "directory")
        assert_same_mount_as_parent(deployed, "directory")
        assert_same_mount_as_parent(reviewed, "directory")
        deployed_names = sorted(
            path.name for path in deployed.iterdir()
            if path.name.startswith("uap-observer")
        )
        reviewed_names = sorted(path.name for path in reviewed.iterdir())
        if deployed_names != expected or reviewed_names != expected:
            raise ResetError("observer deployed systemd inventory differs from closure")
        for name in expected:
            left = deployed / name
            right = reviewed / name
            left_metadata = metadata(left)
            right_metadata = metadata(right)
            if (
                left_metadata["kind"] != right_metadata["kind"]
                or left_metadata["kind"] not in {"regular", "directory"}
                or tuple(left_metadata[field] for field in ("mode", "uid", "gid"))
                != tuple(right_metadata[field] for field in ("mode", "uid", "gid"))
            ):
                raise ResetError(f"observer deployed systemd object differs: {name}")
            assert_same_mount_as_parent(left, left_metadata["kind"])
            assert_same_mount_as_parent(right, right_metadata["kind"])
            if left_metadata["kind"] == "regular":
                if left_metadata["nlink"] != 1 or right_metadata["nlink"] != 1:
                    raise ResetError(f"observer deployed systemd file is hardlinked: {name}")
                if (
                    left_metadata["size"] != right_metadata["size"]
                    or regular_file_digest(left, left_metadata)
                    != regular_file_digest(right, right_metadata)
                ):
                    raise ResetError(f"observer deployed systemd file differs: {name}")
            elif preserved_tree_digest(left) != preserved_tree_digest(right):
                raise ResetError(f"observer deployed systemd drop-in differs: {name}")

    def _validate_install(self, install_identity: str, closure_digest: str) -> None:
        closure = self._validate_install_markers(install_identity, closure_digest)
        if self.closure_identity(closure) != closure_digest:
            raise ResetError("observer closure content identity differs")

    def _validate_fixed_inventory(self) -> None:
        state = self.layout.path("/var/lib/uap-observer")
        validate_root_control(state, self.layout.expected_uid, 0o711, "directory")
        names = sorted(path.name for path in state.iterdir())
        if names != ["jobs", "profiles", "proofs", "state", "workspaces"]:
            raise ResetError("observer mutable inventory differs")
        systemd = self.layout.path("/etc/systemd/system")
        observed = sorted(path.name for path in systemd.iterdir() if path.name.startswith("uap-observer"))
        expected = sorted([*UNITS, "uap-observer.service.d", "uap-observer-runner.service.d"])
        if observed != expected:
            raise ResetError("observer systemd inventory differs")
        for logical in PARTIALS:
            path = self.layout.path(logical)
            if path.exists() or path.is_symlink():
                raise ResetError(f"installer partial exists: {logical}")
        closures = self.layout.path("/opt/uap-observer-closures")
        if any(path.name.startswith(".new-") for path in closures.iterdir()):
            raise ResetError("installer closure partial exists")

    def _validate_ca_inventory(self) -> None:
        volatile_root = self.layout.path("/var/lib/caddy")
        authority = self.layout.path(CA_AUTHORITY_ROOT)
        current = volatile_root
        for component in Path(CA_AUTHORITY_ROOT).relative_to("/var/lib/caddy").parts:
            current /= component
            observed = metadata(current)
            if observed["kind"] != "directory":
                raise ResetError("public Caddy CA path contains a link or non-directory")
            assert_same_mount_as_parent(current, "directory")
        authority_metadata = metadata(authority)
        if {path.name for path in authority.iterdir()} != CA_FILES:
            raise ResetError("public Caddy CA inventory differs")
        for name in CA_FILES:
            observed = metadata(authority / name)
            if (
                observed["kind"] != "regular"
                or observed["mode"] != 0o600
                or observed["nlink"] != 1
                or observed["uid"] != authority_metadata["uid"]
                or observed["gid"] != authority_metadata["gid"]
            ):
                raise ResetError("public Caddy CA file metadata differs")

    def _preserved(self) -> dict[str, dict[str, Any]]:
        self._validate_ca_inventory()
        values: dict[str, dict[str, Any]] = {}
        for logical in PRESERVED:
            path = self.layout.path(logical)
            observed = metadata(path)
            if observed["kind"] not in {"directory", "regular"}:
                raise ResetError(f"preserved path has unsafe type: {logical}")
            assert_same_mount_as_parent(path, observed["kind"])
            if observed["uid"] != self.layout.expected_uid and logical.startswith(("/etc/", "/opt/")):
                raise ResetError(f"preserved control path has unsafe owner: {logical}")
            if logical == "/etc/uap-observer-ed25519.key" and (
                observed["kind"] != "regular" or observed["mode"] != 0o600 or observed["nlink"] != 1
            ):
                raise ResetError("observer signing key metadata differs")
            volatile = logical in VOLATILE_PRESERVED
            if volatile and observed["kind"] != "directory":
                raise ResetError(f"volatile preserved root is not a directory: {logical}")
            values[logical] = {
                "metadata": observed,
                "policy": "volatile-root" if volatile else (
                    "content-tree" if observed["kind"] == "directory" else "content-file"
                ),
                "sha256": regular_file_digest(path, observed) if observed["kind"] == "regular" else None,
                "tree_digest": (
                    preserved_tree_digest(
                        path,
                        frozenset(("Caddyfile.new",)) if logical == "/etc/caddy" else frozenset(),
                    )
                    if observed["kind"] == "directory" and not volatile else None
                ),
            }
        return values

    def _assert_preserved(self, journal: Mapping[str, Any]) -> None:
        self._validate_ca_inventory()
        expected = journal.get("preserved")
        if not isinstance(expected, dict) or set(expected) != set(PRESERVED):
            raise ResetError("reset journal preserved inventory differs")
        for logical, before in expected.items():
            path = self.layout.path(logical)
            observed = metadata(path)
            assert_same_mount_as_parent(path, observed["kind"])
            expected_policy = (
                "volatile-root" if logical in VOLATILE_PRESERVED
                else "content-tree" if before["metadata"]["kind"] == "directory"
                else "content-file"
            )
            if before.get("policy") != expected_policy:
                raise ResetError(f"preserved path policy differs: {logical}")
            if expected_policy == "volatile-root":
                if volatile_directory_identity(observed) != volatile_directory_identity(before["metadata"]):
                    raise ResetError(f"volatile preserved root was substituted: {logical}")
            elif expected_policy == "content-tree":
                if cleanup_identity(observed) != cleanup_identity(before["metadata"]):
                    raise ResetError(f"preserved path changed during reset: {logical}")
            elif stable_metadata(observed) != stable_metadata(before["metadata"]):
                raise ResetError(f"preserved path changed during reset: {logical}")
            if before["sha256"] is not None and regular_file_digest(path, before["metadata"]) != before["sha256"]:
                raise ResetError(f"preserved file content changed during reset: {logical}")
            excluded = (
                frozenset(("Caddyfile.new",)) if logical == "/etc/caddy" else frozenset()
            )
            if before["tree_digest"] is not None and preserved_tree_digest(path, excluded) != before["tree_digest"]:
                raise ResetError(f"preserved tree content changed during reset: {logical}")
        caddy_partial = self.layout.path("/etc/caddy/Caddyfile.new")
        if caddy_partial.exists() or caddy_partial.is_symlink():
            observed = metadata(caddy_partial)
            expected_digest = journal["prepared"]["manifest"]["installer"]["caddy_config_sha256"]
            if (
                observed["kind"] != "regular"
                or observed["uid"] != self.layout.expected_uid
                or observed["mode"] != 0o640
                or observed["nlink"] != 1
                or regular_file_digest(caddy_partial, observed) != expected_digest
            ):
                raise ResetError("installer Caddy partial differs from the reviewed input")

    def _transaction_id(self, machine: str, catalog: str, install: str, closure: str) -> str:
        return hashlib.sha256(
            f"{machine}\0{catalog}\0{install}\0{closure}".encode()
        ).hexdigest()[:24]

    def _quarantine(self, logical: str, transaction: str, candidate: bool = False) -> str:
        prefix = f".uap-observer-reset-{transaction}"
        if candidate:
            prefix += "-candidate"
        if logical.startswith("/opt/"):
            root = Path("/opt") / prefix
        elif logical.startswith("/var/lib/"):
            root = Path("/var/lib") / prefix
        elif logical.startswith("/etc/systemd/system/"):
            root = Path("/etc/systemd/system") / prefix
        elif logical.startswith("/usr/local/libexec/"):
            root = Path("/usr/local/libexec") / prefix
        elif logical.startswith("/usr/local/bin/"):
            root = Path("/usr/local/bin") / prefix
        elif logical.startswith("/etc/caddy/"):
            # Keep installer partial quarantine outside the content-bound
            # /etc/caddy tree so rollback validation remains deterministic.
            root = Path("/etc") / prefix
        elif logical.startswith("/etc/"):
            root = Path("/etc") / prefix
        else:
            raise ResetError(f"unreviewed quarantine domain: {logical}")
        name = logical.strip("/").replace("/", "--")
        return str(root / name)

    def _entry(self, logical: str, transaction: str, candidate: bool = False) -> dict[str, Any]:
        path = self.layout.path(logical)
        observed = metadata(path)
        if (
            observed["kind"] == "regular" and observed["nlink"] != 1
            and not (candidate and logical in HARDLINKED_ADAPTER_PARTIALS)
        ):
            raise ResetError("managed reset path is hardlinked")
        assert_same_mount_as_parent(path, observed["kind"])
        return {
            "source": logical,
            "quarantine": self._quarantine(logical, transaction, candidate),
            "metadata": observed,
        }

    @staticmethod
    def _entry_schema(entry: Mapping[str, Any]) -> None:
        if (
            not isinstance(entry, dict)
            or set(entry) != {"source", "quarantine", "metadata"}
        ):
            raise ResetError("reset quarantine entry schema differs")

    def _write_journal(self, journal: Mapping[str, Any], create: bool = False) -> None:
        self.journal_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
        os.chmod(self.journal_dir, 0o700)
        validate_root_control(self.journal_dir, self.layout.expected_uid, 0o700, "directory")
        # Persist the journal directory entry before any managed path moves.
        fsync_directory(self.journal_dir.parent)
        temporary = self.journal_temporary
        encoded = json.dumps(journal, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode() + b"\n"
        if temporary.exists() or temporary.is_symlink():
            if read_control(
                temporary, self.layout.expected_uid, 0o600, 1 << 20,
            ) != encoded:
                raise ResetError("reset journal temporary content differs")
        else:
            descriptor = os.open(
                temporary,
                os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC | os.O_NOFOLLOW,
                0o600,
            )
            try:
                view = memoryview(encoded)
                while view:
                    written = os.write(descriptor, view)
                    if written <= 0:
                        raise ResetError("short reset journal write")
                    view = view[written:]
                os.fsync(descriptor)
            finally:
                os.close(descriptor)
        self.failpoint("after-journal-temp-fsync")
        if create and (self.journal_path.exists() or self.journal_path.is_symlink()):
            temporary.unlink()
            raise ResetError("reset transaction already exists")
        os.replace(temporary, self.journal_path)
        fsync_directory(self.journal_dir)
        self.failpoint("after-journal-replace")

    def _read_journal_at(self, path: Path) -> dict[str, Any]:
        value = strict_json(read_control(
            path, self.layout.expected_uid, 0o600, 1 << 20,
        ))
        required = {
            "schema_version", "transaction_id", "phase", "machine_id",
            "catalog_sha", "old_install_identity", "old_closure_digest",
            "preserved", "prepared",
            "units", "public_hop_contract", "old", "candidate", "new",
            "helper_sha256", "install_lib_sha256",
        }
        if not isinstance(value, dict) or set(value) != required or value.get("schema_version") != SCHEMA_VERSION:
            raise ResetError("reset journal schema differs")
        if (
            value.get("phase") not in {
                "prepared", "quarantining", "applied", "new-ready",
                "rolling_back", "rollback-cleanup", "finalizing",
            }
            or MACHINE_ID.fullmatch(str(value.get("machine_id", ""))) is None
            or re.fullmatch(r"[0-9a-f]{40}", str(value.get("catalog_sha", ""))) is None
            or DIGEST.fullmatch(str(value.get("old_install_identity", ""))) is None
            or DIGEST.fullmatch(str(value.get("old_closure_digest", ""))) is None
            or value.get("transaction_id") != self._transaction_id(
                value["machine_id"], value["catalog_sha"],
                value["old_install_identity"], value["old_closure_digest"],
            )
        ):
            raise ResetError("reset journal transaction binding differs")
        if not isinstance(value.get("old"), list) or not isinstance(value.get("candidate"), list):
            raise ResetError("reset journal quarantine inventory differs")
        for entry in [*value["old"], *value["candidate"]]:
            self._entry_schema(entry)
        if [entry["source"] for entry in value["old"]] != list(MANAGED) or any(
            entry["quarantine"] != self._quarantine(
                entry["source"], value["transaction_id"], candidate=False,
            )
            for entry in value["old"]
        ):
            raise ResetError("reset journal old inventory differs")
        candidate_sources = [entry["source"] for entry in value["candidate"]]
        if (
            len(candidate_sources) != len(set(candidate_sources))
            or any(source not in {*MANAGED, *PARTIALS} for source in candidate_sources)
            or any(
                entry["quarantine"] != self._quarantine(
                    entry["source"], value["transaction_id"], candidate=True,
                )
                for entry in value["candidate"]
            )
        ):
            raise ResetError("reset journal candidate inventory differs")
        units = value.get("units")
        if (
            not isinstance(units, dict) or set(units) != set(ALL_UNITS)
            or any(
                not isinstance(state, dict)
                or set(state) != {"active", "enabled"}
                or any(not isinstance(flag, bool) for flag in state.values())
                for state in units.values()
            )
        ):
            raise ResetError("reset journal unit inventory differs")
        self._validate_ingress_states(units)
        if (
            units[PUBLIC_HOP_UNIT]["active"]
            and not isinstance(value["public_hop_contract"], dict)
        ) or (
            not units[PUBLIC_HOP_UNIT]["active"]
            and value["public_hop_contract"] is not None
        ):
            raise ResetError("reset journal public ingress binding differs")
        new = value.get("new")
        if new is not None and (
            not isinstance(new, dict)
            or set(new) != {"install_identity", "closure_digest"}
            or DIGEST.fullmatch(str(new.get("install_identity", ""))) is None
            or DIGEST.fullmatch(str(new.get("closure_digest", ""))) is None
        ):
            raise ResetError("reset journal new install binding differs")
        return value

    def _read_journal(self) -> dict[str, Any]:
        return self._read_journal_at(self.journal_path)

    def _promote_journal_temporary(
        self, machine: str, catalog: str | None = None,
        old_install: str | None = None, old_closure: str | None = None,
    ) -> None:
        if not (self.journal_temporary.exists() or self.journal_temporary.is_symlink()):
            return
        validate_root_control(
            self.journal_dir, self.layout.expected_uid, 0o700, "directory",
        )
        candidate = self._read_journal_at(self.journal_temporary)
        self._bind(candidate, machine, catalog, old_install, old_closure)
        self._assert_stable_bootstrap(candidate)
        if self.journal_path.exists() or self.journal_path.is_symlink():
            current = self._read_journal()
            if (
                current["transaction_id"] != candidate["transaction_id"]
                or current["machine_id"] != candidate["machine_id"]
                or current["old_install_identity"] != candidate["old_install_identity"]
                or current["old_closure_digest"] != candidate["old_closure_digest"]
            ):
                raise ResetError("reset journal temporary transaction differs")
        os.replace(self.journal_temporary, self.journal_path)
        fsync_directory(self.journal_dir)

    def _bind(
        self, journal: Mapping[str, Any], machine: str, catalog: str | None,
        install: str | None, closure: str | None,
    ) -> None:
        self._machine(machine)
        if journal.get("machine_id") != machine:
            raise ResetError("reset journal machine identity differs")
        if catalog is not None and journal.get("catalog_sha") != catalog:
            raise ResetError("reset journal catalog SHA differs")
        if install is not None and journal.get("old_install_identity") != install:
            raise ResetError("reset journal old install identity differs")
        if closure is not None and journal.get("old_closure_digest") != closure:
            raise ResetError("reset journal old closure differs")

    def _ensure_quarantine_parent(self, logical: str) -> None:
        path = self.layout.path(logical)
        parent = path.parent
        base = parent.parent
        validate_root_control(base, self.layout.expected_uid, stat.S_IMODE(os.lstat(base).st_mode), "directory")
        if not parent.exists():
            os.mkdir(parent, 0o700)
            fsync_directory(base)
        validate_root_control(parent, self.layout.expected_uid, 0o700, "directory")

    def _move(self, entry: Mapping[str, Any], reverse: bool = False) -> None:
        self._entry_schema(entry)
        source_logical = str(entry["quarantine"] if reverse else entry["source"])
        target_logical = str(entry["source"] if reverse else entry["quarantine"])
        expected = entry["metadata"]
        source = self.layout.path(source_logical)
        target = self.layout.path(target_logical)
        self._ensure_quarantine_parent(str(entry["quarantine"]))
        source_exists = source.exists() or source.is_symlink()
        target_exists = target.exists() or target.is_symlink()
        if target_exists and not source_exists:
            if stable_metadata(metadata(target)) != stable_metadata(expected):
                raise ResetError(f"quarantine object was substituted: {target_logical}")
            assert_same_mount_as_parent(target, expected["kind"])
            return
        if source_exists and target_exists:
            raise ResetError("both reset source and quarantine target exist")
        if not source_exists:
            raise ResetError(f"reset source disappeared: {source_logical}")
        if stable_metadata(metadata(source)) != stable_metadata(expected):
            raise ResetError(f"reset source was substituted: {source_logical}")
        assert_same_mount_as_parent(source, expected["kind"])
        source_parent = os.open(source.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW)
        target_parent = os.open(target.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW)
        try:
            if os.fstat(source_parent).st_dev != os.fstat(target_parent).st_dev:
                raise ResetError("quarantine rename is not on one filesystem")
            os.rename(
                source.name, target.name,
                src_dir_fd=source_parent, dst_dir_fd=target_parent,
            )
            os.fsync(source_parent)
            os.fsync(target_parent)
        finally:
            os.close(target_parent)
            os.close(source_parent)
        if stable_metadata(metadata(target)) != stable_metadata(expected):
            raise ResetError("quarantine identity changed during rename")
        assert_same_mount_as_parent(target, expected["kind"])

    def _states(self) -> dict[str, dict[str, bool]]:
        states: dict[str, dict[str, bool]] = {}
        for unit in ALL_UNITS:
            state, failed, missing = self.systemd.state_details(unit)
            if failed:
                raise ResetError(f"observer unit is failed before reset: {unit}")
            if missing:
                raise ResetError(f"observer unit is missing before reset: {unit}")
            states[unit] = state
        self._validate_ingress_states(states)
        return states

    @staticmethod
    def _validate_ingress_states(states: Mapping[str, Mapping[str, bool]]) -> None:
        if states["uap-observer-caddy.service"] != {"active": False, "enabled": False}:
            raise ResetError("regular Caddy must remain inactive and disabled on this observer")
        if states[PUBLIC_HOP_UNIT] != {"active": True, "enabled": False}:
            raise ResetError("exact transient public ingress must be active on this observer")

    def _verify_recorded_units(self, journal: Mapping[str, Any], closure: str) -> None:
        expected = journal["units"]
        self._validate_ingress_states(expected)
        for unit in UNITS:
            observed, failed, missing = self.systemd.state_details(unit)
            if failed:
                raise ResetError(f"observer unit is failed after reset: {unit}")
            if missing:
                raise ResetError(f"observer unit is missing after reset: {unit}")
            if observed != expected[unit]:
                raise ResetError(f"observer unit state differs after reset: {unit}")
        public, public_failed, public_missing = self.systemd.state_details(PUBLIC_HOP_UNIT)
        if public_failed:
            raise ResetError("transient public ingress is failed after reset")
        if public_missing:
            raise ResetError("transient public ingress is missing after reset")
        if public != expected[PUBLIC_HOP_UNIT]:
            raise ResetError("transient public ingress state differs after reset")
        if public["active"]:
            regular, regular_failed, regular_missing = self.systemd.state_details(
                "uap-observer-caddy.service",
            )
            if regular_failed:
                raise ResetError("regular Caddy is failed after reset")
            if regular_missing:
                raise ResetError("regular Caddy is missing after reset")
            if regular["active"]:
                raise ResetError("regular Caddy conflicts with transient public ingress")
            if self.systemd.validate_public_hop_contract() != journal["public_hop_contract"]:
                raise ResetError("transient public ingress contract changed during reset")
            self.systemd.verify_public_hop(closure)

    def _activate_recorded_units(
        self, journal: Mapping[str, Any], closure: str, *, daemon_reload: bool = False,
    ) -> None:
        expected = journal["units"]
        self._validate_ingress_states(expected)
        if daemon_reload:
            self.systemd.daemon_reload()
        enabled = [unit for unit in UNITS if expected[unit]["enabled"]]
        disabled = [unit for unit in UNITS if not expected[unit]["enabled"]]
        self.systemd.enable(enabled)
        self.systemd.disable(disabled)
        active = [unit for unit in UNITS if expected[unit]["active"]]
        inactive = [unit for unit in UNITS if not expected[unit]["active"]]
        self.systemd.stop(inactive)
        failed = [unit for unit in ALL_UNITS if self.systemd.is_failed(unit)]
        if failed:
            self.systemd.stop(failed)
            self.systemd.reset_failed(failed)
        self.systemd.start(active)
        if expected[PUBLIC_HOP_UNIT]["active"]:
            if self.systemd.state("uap-observer-caddy.service")["active"]:
                raise ResetError("regular Caddy occupied public ports before transient recreation")
            if self.systemd.state(PUBLIC_HOP_UNIT)["active"]:
                if self.systemd.validate_public_hop_contract() != journal["public_hop_contract"]:
                    raise ResetError("active transient public ingress contract differs")
                self.systemd.verify_public_hop(closure)
            else:
                self.systemd.recreate_public_hop(closure)
        self._verify_recorded_units(journal, closure)

    def _prepared(self) -> dict[str, Any]:
        root = self.layout.path(PREPARED_ROOT)
        observed = validate_root_control(root, self.layout.expected_uid, 0o700, "directory")
        expected_root = [*PREPARED_INVENTORY, "manifest.json"]
        if sorted(path.name for path in root.iterdir()) != sorted(expected_root):
            raise ResetError("prepared reset root inventory differs")
        children: dict[str, dict[str, Any]] = {}
        for name in PREPARED_INVENTORY:
            child = root / name
            children[name] = validate_root_control(child, self.layout.expected_uid, 0o700, "directory")
        manifest_path = root / "manifest.json"
        encoded = read_control(manifest_path, self.layout.expected_uid, 0o400, 1 << 20)
        manifest = strict_json(encoded)
        digest_pattern = re.compile(r"sha256:[0-9a-f]{64}\Z")
        installer_fields = {
            "source_root", "adapter_config", "adapter_sha256", "observer_config",
            "observer_sha256", "caddy_archive", "caddy_config",
            "caddy_config_sha256", "egress_allowlist", "egress_sha256",
        }
        client_fields = {"seed", "seed_digest", "projection_digest"}
        if (
            not isinstance(manifest, dict)
            or set(manifest) != {"schema_version", "purpose", "catalog_sha", "new_install_identity", "installer", "clients", "tree_digests"}
            or manifest.get("schema_version") != 1
            or manifest.get("purpose") != PREPARED_PURPOSE
            or re.fullmatch(r"[0-9a-f]{40}", str(manifest.get("catalog_sha", ""))) is None
            or DIGEST.fullmatch(str(manifest.get("new_install_identity", ""))) is None
            or not isinstance(manifest.get("installer"), dict)
            or set(manifest["installer"]) != installer_fields
            or not isinstance(manifest.get("clients"), dict)
            or set(manifest["clients"]) != set(CLIENTS)
            or not isinstance(manifest.get("tree_digests"), dict)
            or set(manifest["tree_digests"]) != set(PREPARED_INVENTORY)
        ):
            raise ResetError("prepared reset manifest schema differs")
        exact_paths = {
            "source_root": "evidence/source",
            "adapter_config": "evidence/uap-observer-adapter-config.json",
            "observer_config": "evidence/uap-observer.json",
            "caddy_archive": "evidence/caddy_2.11.4_linux_amd64.tar.gz",
            "caddy_config": "evidence/Caddyfile",
            "egress_allowlist": "evidence/uap-observer-egress-allowlist.json",
        }
        installer = manifest["installer"]
        if any(installer.get(field) != value for field, value in exact_paths.items()):
            raise ResetError("prepared installer path differs")
        if any(
            digest_pattern.fullmatch(str(manifest["tree_digests"].get(name, ""))) is None
            for name in PREPARED_INVENTORY
        ):
            raise ResetError("prepared tree digest declaration differs")
        # Authenticate every byte in the prepared authority before invoking
        # Git. Local repository config is part of this evidence tree and is
        # subsequently restricted to an inert clone-only allowlist.
        tree_digests = {
            name: prepared_tree_digest(root / name, self.layout.expected_uid)
            for name in PREPARED_INVENTORY
        }
        if tree_digests != manifest["tree_digests"]:
            raise ResetError("prepared tree digest differs")
        source_revision = self.source_revision(root / installer["source_root"])
        if source_revision != manifest["catalog_sha"]:
            raise ResetError("prepared source revision differs from catalog SHA")
        for field in ("adapter_sha256", "observer_sha256", "caddy_config_sha256", "egress_sha256"):
            if not isinstance(installer.get(field), str) or digest_pattern.fullmatch(installer[field]) is None:
                raise ResetError("prepared installer digest differs")
        evidence_expected = {
            "source", "uap-observer-adapter-config.json", "uap-observer.json",
            "caddy_2.11.4_linux_amd64.tar.gz", "Caddyfile",
            "uap-observer-egress-allowlist.json",
        }
        if {path.name for path in (root / "evidence").iterdir()} != evidence_expected:
            raise ResetError("prepared evidence inventory differs")
        for client in CLIENTS:
            client_value = manifest["clients"][client]
            if (
                not isinstance(client_value, dict) or set(client_value) != client_fields
                or client_value.get("seed") != f"seeds/{client}"
                or any(digest_pattern.fullmatch(str(client_value.get(field, ""))) is None for field in ("seed_digest", "projection_digest"))
            ):
                raise ResetError(f"prepared client contract differs: {client}")
        adapter_path = root / installer["adapter_config"]
        adapter = strict_json(read_control(
            adapter_path, self.layout.expected_uid, 0o400, 4 << 20,
        ))
        adapter_clients = adapter.get("clients") if isinstance(adapter, dict) else None
        if not isinstance(adapter_clients, dict):
            raise ResetError("prepared adapter config client inventory differs")
        for client in CLIENTS:
            record = adapter_clients.get(client)
            projection_contract = record.get("native_projection") if isinstance(record, dict) else None
            expected_contract = {
                "path": f"/var/lib/uap-observer/proofs/{client}/native-projection.json",
                "sha256": manifest["clients"][client]["projection_digest"],
            }
            if projection_contract != expected_contract:
                raise ResetError(f"prepared adapter projection binding differs: {client}")
        for name in ("seeds", "path"):
            if {path.name for path in (root / name).iterdir()} != set(CLIENTS):
                raise ResetError(f"prepared {name} inventory differs")
        projection = root / "projection-digests"
        if {path.name for path in projection.iterdir()} != {f"{client}.sha256" for client in CLIENTS}:
            raise ResetError("prepared projection digest inventory differs")
        for client in CLIENTS:
            value = read_control(
                projection / f"{client}.sha256", self.layout.expected_uid, 0o400, 4096,
            ).decode("ascii")
            if value != manifest["clients"][client]["projection_digest"] + "\n":
                raise ResetError(f"prepared projection digest file differs: {client}")
        for field, relative in (
            ("adapter_sha256", installer["adapter_config"]),
            ("observer_sha256", installer["observer_config"]),
            ("caddy_config_sha256", installer["caddy_config"]),
            ("egress_sha256", installer["egress_allowlist"]),
        ):
            path = root / relative
            actual = "sha256:" + hashlib.sha256(read_control(
                path, self.layout.expected_uid, stat.S_IMODE(os.lstat(path).st_mode), 4 << 20,
            )).hexdigest()
            if actual != installer[field]:
                raise ResetError(f"prepared installer content digest differs: {field}")
        canonical = json.dumps(manifest, sort_keys=True, separators=(",", ":")).encode() + b"\n"
        if encoded != canonical:
            raise ResetError("prepared manifest is not canonical JSON")
        return {
            "root": observed,
            "children": children,
            "manifest_sha256": "sha256:" + hashlib.sha256(encoded).hexdigest(),
            "tree_digests": tree_digests,
            "manifest": manifest,
        }

    def _assert_prepared(self, journal: Mapping[str, Any], allow_removed: bool = False) -> None:
        root = self.layout.path(PREPARED_ROOT)
        if allow_removed and not root.exists() and not root.is_symlink():
            return
        expected = journal.get("prepared")
        if not isinstance(expected, dict) or set(expected) != {
            "root", "children", "manifest_sha256", "tree_digests", "manifest",
        }:
            raise ResetError("prepared reset journal binding differs")
        if allow_removed:
            observed = metadata(root)
            if cleanup_identity(observed) != cleanup_identity(expected["root"]):
                raise ResetError("prepared reset cleanup root was substituted")
            assert_same_mount_as_parent(root, observed["kind"])
            return
        observed = self._prepared()
        if observed != expected:
            raise ResetError("prepared reset bundle changed")

    def _stop_and_quiesce(self) -> None:
        public_present = not self.systemd.state_details(PUBLIC_HOP_UNIT)[2]
        if public_present:
            self.systemd.stop((PUBLIC_HOP_UNIT,))
        managed_present = [
            unit for unit in UNITS
            if not self.systemd.state_details(unit)[2]
        ]
        if managed_present:
            self.systemd.stop(managed_present)
        failed = [unit for unit in ALL_UNITS if self.systemd.is_failed(unit)]
        self.systemd.reset_failed(failed)
        for unit in ALL_UNITS:
            if self.systemd.state(unit)["active"]:
                raise ResetError(f"observer unit remains active: {unit}")
        self.runtime_probe.assert_quiescent()

    def _clean_postcheck(self) -> None:
        for logical in MANAGED:
            path = self.layout.path(logical)
            if path.exists() or path.is_symlink():
                raise ResetError(f"managed path remains after reset: {logical}")
        for logical in PARTIALS:
            path = self.layout.path(logical)
            if path.exists() or path.is_symlink():
                raise ResetError(f"installer partial remains after reset: {logical}")

    def _assert_installed_projections(self, manifest: Mapping[str, Any]) -> None:
        self._assert_projection_digests({
            client: manifest["clients"][client]["projection_digest"]
            for client in CLIENTS
        })

    def _assert_projection_digests(self, expected: Mapping[str, str]) -> None:
        if set(expected) != set(CLIENTS):
            raise ResetError("installed projection inventory differs")
        for client in CLIENTS:
            path = self.layout.path(
                f"/var/lib/uap-observer/proofs/{client}/native-projection.json"
            )
            encoded = read_control(path, self.layout.expected_uid, 0o440, 16 << 20)
            actual = "sha256:" + hashlib.sha256(encoded).hexdigest()
            if actual != expected[client]:
                raise ResetError(f"installed projection digest differs: {client}")

    def apply(
        self, machine: str, catalog: str, old_install: str, old_closure: str,
    ) -> dict[str, Any]:
        with self.locked():
            self._machine(machine)
            if re.fullmatch(r"[0-9a-f]{40}", catalog) is None:
                raise ResetError("catalog SHA must be 40 lowercase hexadecimal characters")
            self._validate_ids(old_install, old_closure)
            self._promote_journal_temporary(machine, catalog, old_install, old_closure)
            if self.completion_path.exists() or self.completion_path.is_symlink():
                raise ResetError("completed reset cleanup remains; run finalize or rollback")
            if self.journal_path.exists() or self.journal_path.is_symlink():
                journal = self._read_journal()
                self._bind(journal, machine, catalog, old_install, old_closure)
                if journal["phase"] not in {"prepared", "quarantining", "applied", "new-ready"}:
                    raise ResetError(f"reset transaction is {journal['phase']}; apply cannot continue")
            else:
                self.runtime_probe.assert_no_pending_work()
                states = self._states()
                prepared = self._prepared()
                if prepared["manifest"]["catalog_sha"] != catalog:
                    raise ResetError("prepared bundle catalog SHA differs from caller approval")
                self._assert_no_quarantine(None)
                # Bind the exact old pointer, inventory, and markers before
                # rotating even the stable recovery bootstrap. The new
                # digest-pinned install library is then used for the stronger
                # content-derived closure verification below.
                self._validate_install_markers(old_install, old_closure)
                prepared_source = self.layout.path(PREPARED_ROOT) / prepared["manifest"]["installer"]["source_root"]
                helper_sha256, install_lib_sha256 = self._ensure_stable_bootstrap(
                    prepared_source, allow_replace=True,
                )
                self._validate_install(old_install, old_closure)
                self._validate_fixed_inventory()
                transaction = self._transaction_id(machine, catalog, old_install, old_closure)
                journal = {
                    "schema_version": SCHEMA_VERSION,
                    "transaction_id": transaction,
                    "phase": "prepared",
                    "machine_id": machine,
                    "catalog_sha": catalog,
                    "old_install_identity": old_install,
                    "old_closure_digest": old_closure,
                    "helper_sha256": helper_sha256,
                    "install_lib_sha256": install_lib_sha256,
                    "preserved": self._preserved(),
                    "prepared": prepared,
                    "units": states,
                    "public_hop_contract": (
                        self.systemd.validate_public_hop_contract()
                        if states[PUBLIC_HOP_UNIT]["active"] else None
                    ),
                    "old": [self._entry(path, transaction) for path in MANAGED],
                    "candidate": [],
                    "new": None,
                }
                self._write_journal(journal, create=True)
                self.failpoint("after-journal")
            self._assert_stable_bootstrap(journal)
            self._assert_preserved(journal)
            self._assert_prepared(journal)
            if journal["phase"] == "new-ready":
                self.executor.verify_new(self, journal)
                self._assert_installed_projections(journal["prepared"]["manifest"])
                return self._status(journal)
            try:
                self._stop_and_quiesce()
            except Exception as primary:
                if journal["phase"] == "prepared":
                    try:
                        self._restore_unmoved_old(journal)
                    except Exception as restore_error:
                        raise ResetError(
                            f"observer stop failed: {primary}; old service restore failed: {restore_error}"
                        ) from primary
                raise
            self.failpoint("after-stop")
            if journal["phase"] != "applied":
                journal["phase"] = "quarantining"
                self._write_journal(journal)
                for index, entry in enumerate(journal["old"]):
                    self._move(entry)
                    self.failpoint(f"after-quarantine:{index}")
                self._clean_postcheck()
                self._assert_preserved(journal)
                journal["phase"] = "applied"
                self._write_journal(journal)
            else:
                self._clean_postcheck()
            self.failpoint("after-applied")
            try:
                self._assert_prepared(journal)
                new = self.executor.prepare_new(self, journal)
                self._assert_prepared(journal)
                self._assert_installed_projections(journal["prepared"]["manifest"])
                expected = journal["prepared"]["manifest"]["new_install_identity"]
                if new.get("install_identity") != expected or DIGEST.fullmatch(str(new.get("closure_digest", ""))) is None:
                    raise ResetError("prepared executor returned a different new install")
                journal["new"] = new
                journal["phase"] = "new-ready"
                self._write_journal(journal)
                self.executor.verify_new(self, journal)
                return self._status(journal)
            except InjectedFailure:
                raise
            except Exception as primary:
                try:
                    self._rollback_locked(journal)
                except Exception as rollback_error:
                    raise ResetError(
                        "new observer preparation failed: "
                        f"{primary}; automatic rollback also failed: {rollback_error}"
                    ) from primary
                raise

    def status(self, machine: str) -> dict[str, Any]:
        with self.locked():
            self._machine(machine)
            self._promote_journal_temporary(machine)
            if not self.journal_path.exists() and not self.journal_path.is_symlink():
                completion = self._read_completion(optional=True)
                if completion is None:
                    raise ResetError("reset transaction is absent")
                if completion["machine_id"] != machine:
                    raise ResetError("reset completion machine identity differs")
                self._assert_stable_bootstrap(completion)
                return {
                    "schema_version": SCHEMA_VERSION,
                    "transaction_id": completion["transaction_id"],
                    "phase": completion["outcome"],
                    "old_quarantined": 0,
                    "old_total": 0,
                    "candidate_quarantined": 0,
                    "new_install_identity": (
                        None if completion["new"] is None
                        else completion["new"]["install_identity"]
                    ),
                }
            journal = self._read_journal()
            self._bind(journal, machine, None, None, None)
            self._assert_stable_bootstrap(journal)
            self._assert_preserved(journal)
            self._assert_prepared(journal, allow_removed=journal["phase"] == "finalizing")
            return self._status(journal)

    def _status(self, journal: Mapping[str, Any]) -> dict[str, Any]:
        old_quarantined = sum(
            1 for entry in journal["old"]
            if self.layout.path(entry["quarantine"]).exists()
            or self.layout.path(entry["quarantine"]).is_symlink()
        )
        return {
            "schema_version": SCHEMA_VERSION,
            "transaction_id": journal["transaction_id"],
            "phase": journal["phase"],
            "old_quarantined": old_quarantined,
            "old_total": len(journal["old"]),
            "candidate_quarantined": len(journal["candidate"]),
            "new_install_identity": None if journal["new"] is None else journal["new"]["install_identity"],
        }

    def _candidate_paths(self) -> list[str]:
        values = list(MANAGED)
        for logical in PARTIALS:
            path = self.layout.path(logical)
            if path.exists() or path.is_symlink():
                values.append(logical)
        closures = self.layout.path("/opt/uap-observer-closures")
        if closures.is_dir():
            # The whole closures root is already in MANAGED; no child wildcard
            # is separately moved.
            pass
        return values

    def _prepare_candidate(self, journal: dict[str, Any]) -> None:
        if journal["candidate"]:
            return
        transaction = journal["transaction_id"]
        entries = []
        old_by_source = {entry["source"]: entry for entry in journal["old"]}
        for logical in self._candidate_paths():
            source = self.layout.path(logical)
            old = old_by_source.get(logical)
            old_quarantine_exists = old is not None and (
                self.layout.path(old["quarantine"]).exists()
                or self.layout.path(old["quarantine"]).is_symlink()
            )
            if not old_quarantine_exists and old is not None:
                # Apply did not yet move this old object; it is not candidate.
                continue
            if source.exists() or source.is_symlink():
                entries.append(self._entry(logical, transaction, candidate=True))
        journal["candidate"] = entries
        journal["phase"] = "rolling_back"
        self._write_journal(journal)

    def _restore_units(self, journal: Mapping[str, Any]) -> None:
        self._activate_recorded_units(
            journal, journal["old_closure_digest"], daemon_reload=True,
        )

    def _restore_unmoved_old(self, journal: Mapping[str, Any]) -> None:
        if journal["phase"] != "prepared":
            raise ResetError("old service restore requires an untouched prepared transaction")
        self._assert_no_quarantine(journal["transaction_id"])
        for entry in journal["old"]:
            source = self.layout.path(entry["source"])
            if not (source.exists() or source.is_symlink()):
                raise ResetError("prepared reset source is missing")
            if stable_metadata(metadata(source)) != stable_metadata(entry["metadata"]):
                raise ResetError("prepared reset source was substituted")
        self._validate_install(journal["old_install_identity"], journal["old_closure_digest"])
        self._restore_units(journal)

    def rollback(
        self, machine: str, catalog: str, old_install: str, old_closure: str,
    ) -> dict[str, Any]:
        with self.locked():
            self._machine(machine)
            if re.fullmatch(r"[0-9a-f]{40}", catalog) is None:
                raise ResetError("catalog SHA must be 40 lowercase hexadecimal characters")
            self._validate_ids(old_install, old_closure)
            self._promote_journal_temporary(machine, catalog, old_install, old_closure)
            if not self.journal_path.exists() and not self.journal_path.is_symlink():
                completion = self._read_completion(optional=True)
                if completion is not None:
                    self._validate_completion(
                        completion, "rolled_back", machine, catalog,
                        old_install, old_closure,
                    )
                    self._validate_install(old_install, old_closure)
                    self._assert_stable_bootstrap(completion)
                    self._activate_recorded_units(
                        completion, old_closure, daemon_reload=True,
                    )
                    self._validate_install(old_install, old_closure)
                    self._assert_no_quarantine(completion["transaction_id"])
                    self._finish_completion(completion)
                else:
                    self._validate_install(old_install, old_closure)
                    self._assert_no_quarantine(
                        self._transaction_id(machine, catalog, old_install, old_closure)
                    )
                return {"schema_version": SCHEMA_VERSION, "phase": "rolled_back"}
            journal = self._read_journal()
            self._bind(journal, machine, catalog, old_install, old_closure)
            return self._rollback_locked(journal)

    def _rollback_locked(self, journal: dict[str, Any]) -> dict[str, Any]:
        if journal["phase"] == "finalizing":
            raise ResetError("finalization already began; rollback is no longer possible")
        if journal["phase"] not in {
            "prepared", "quarantining", "applied", "new-ready", "rolling_back",
            "rollback-cleanup",
        }:
            raise ResetError(f"reset transaction is {journal['phase']}; rollback cannot continue")
        self._assert_preserved(journal)
        self._assert_stable_bootstrap(journal)
        self._assert_prepared(journal)
        if journal["phase"] == "prepared":
            # No rename is permitted until quarantining is durable. Pending
            # work must not prevent restoring the untouched old service.
            self._restore_unmoved_old(journal)
            self._remove_transaction_roots(journal, "rolled_back")
            return {"schema_version": SCHEMA_VERSION, "phase": "rolled_back"}
        if journal["phase"] != "rollback-cleanup":
            self._stop_and_quiesce()
            self._prepare_candidate(journal)
            for index, entry in enumerate(journal["candidate"]):
                quarantine = self.layout.path(entry["quarantine"])
                if quarantine.exists() or quarantine.is_symlink():
                    if stable_metadata(metadata(quarantine)) != stable_metadata(entry["metadata"]):
                        raise ResetError("candidate quarantine was substituted")
                else:
                    self._move(entry)
                self.failpoint(f"after-candidate-quarantine:{index}")
            for index, entry in enumerate(reversed(journal["old"])):
                quarantine = self.layout.path(entry["quarantine"])
                source = self.layout.path(entry["source"])
                if quarantine.exists() or quarantine.is_symlink():
                    if source.exists() or source.is_symlink():
                        raise ResetError("candidate source remains before old restore")
                    self._move(entry, reverse=True)
                elif stable_metadata(metadata(source)) != stable_metadata(entry["metadata"]):
                    raise ResetError("old reset object is missing or substituted")
                self.failpoint(f"after-old-restore:{index}")
            self._validate_install(journal["old_install_identity"], journal["old_closure_digest"])
            self._assert_preserved(journal)
            self._assert_prepared(journal)
            self._restore_units(journal)
            self._assert_preserved(journal)
            journal["phase"] = "rollback-cleanup"
            self._write_journal(journal)
        else:
            self._validate_install(journal["old_install_identity"], journal["old_closure_digest"])
            self._restore_units(journal)
            self._validate_install(
                journal["old_install_identity"], journal["old_closure_digest"],
            )
            self._assert_preserved(journal)
            self._assert_prepared(journal)
        self._delete_entries(journal["candidate"])
        self._remove_transaction_roots(journal, "rolled_back")
        return {"schema_version": SCHEMA_VERSION, "phase": "rolled_back"}

    def _verify_public_hop_new_pointer(self, journal: Mapping[str, Any], closure: str) -> None:
        self._activate_recorded_units(journal, closure)

    def finalize(
        self,
        machine: str,
        catalog: str,
        old_install: str,
        old_closure: str,
        new_install: str,
        new_closure: str,
    ) -> dict[str, Any]:
        with self.locked():
            self._machine(machine)
            if re.fullmatch(r"[0-9a-f]{40}", catalog) is None:
                raise ResetError("catalog SHA must be 40 lowercase hexadecimal characters")
            self._validate_ids(old_install, old_closure)
            self._validate_ids(new_install, new_closure)
            self._promote_journal_temporary(machine, catalog, old_install, old_closure)
            if not self.journal_path.exists() and not self.journal_path.is_symlink():
                completion = self._read_completion(optional=True)
                if completion is not None:
                    self._validate_completion(
                        completion, "finalized", machine, catalog,
                        old_install, old_closure,
                        new_install, new_closure,
                    )
                    self._assert_stable_bootstrap(completion)
                self._validate_install(new_install, new_closure)
                transaction = self._transaction_id(machine, catalog, old_install, old_closure)
                self._assert_no_quarantine(transaction)
                if self.layout.path(PREPARED_ROOT).exists() or self.layout.path(PREPARED_ROOT).is_symlink():
                    raise ResetError("completed reset still has prepared residue")
                if completion is not None:
                    completed_journal = {
                        "new": {"install_identity": new_install, "closure_digest": new_closure},
                        "units": completion["units"],
                        "public_hop_contract": completion["public_hop_contract"],
                    }
                    self._activate_recorded_units(
                        completion, new_closure, daemon_reload=True,
                    )
                    self.executor.verify_new(self, completed_journal)
                    self._assert_projection_digests(completion["projection_digests"])
                    self._finish_completion(completion)
                return {"schema_version": SCHEMA_VERSION, "phase": "finalized", "new_install_identity": new_install}
            journal = self._read_journal()
            self._bind(journal, machine, catalog, old_install, old_closure)
            if journal["phase"] not in {"new-ready", "finalizing"}:
                raise ResetError(f"reset transaction is {journal['phase']}; finalize cannot continue")
            self._validate_install(new_install, new_closure)
            self._assert_stable_bootstrap(journal)
            self._assert_preserved(journal)
            self._assert_prepared(journal, allow_removed=journal["phase"] == "finalizing")
            self._verify_public_hop_new_pointer(journal, new_closure)
            if journal.get("new") != {"install_identity": new_install, "closure_digest": new_closure}:
                raise ResetError("verified new install differs from the journal")
            self.executor.verify_new(self, journal)
            self._assert_installed_projections(journal["prepared"]["manifest"])
            self._assert_preserved(journal)
            if journal["new"] is None:
                journal["new"] = {
                    "install_identity": new_install,
                    "closure_digest": new_closure,
                }
            # Detect an unsafe old-quarantine shape while rollback remains
            # permitted. Deletion repeats the complete descriptor-relative
            # preflight after finalizing is durable to close the race.
            self._preflight_delete_entries(journal["old"])
            journal["phase"] = "finalizing"
            self._write_journal(journal)
            self._delete_entries(journal["old"])
            self._assert_preserved(journal)
            prepared = self.layout.path(PREPARED_ROOT)
            if prepared.exists() or prepared.is_symlink():
                self._assert_prepared(journal, allow_removed=True)
                self._remove_tree(
                    prepared, expected=journal["prepared"]["root"],
                )
            self._remove_transaction_roots(journal, "finalized")
            return {"schema_version": SCHEMA_VERSION, "phase": "finalized", "new_install_identity": new_install}

    @staticmethod
    def _hardlink_key(info: os.stat_result) -> tuple[int, int]:
        return info.st_dev, info.st_ino

    @classmethod
    def _record_cleanup_link(
        cls,
        inventory: dict[tuple[int, int], dict[str, int]],
        info: os.stat_result,
    ) -> None:
        key = cls._hardlink_key(info)
        value = inventory.setdefault(key, {"count": 0, "nlink": info.st_nlink})
        if value["nlink"] != info.st_nlink:
            raise ResetError("reset cleanup hardlink count changed during preflight")
        value["count"] += 1

    @classmethod
    def _assert_cleanup_link_contained(
        cls,
        remaining: Mapping[tuple[int, int], int] | None,
        info: os.stat_result,
    ) -> None:
        if remaining is None:
            if info.st_nlink != 1:
                raise ResetError("reset cleanup contains an unscoped hardlinked file")
            return
        if remaining.get(cls._hardlink_key(info)) != info.st_nlink:
            raise ResetError("reset cleanup hardlinked file escapes the cleanup scope")

    def _preflight_tree_descriptor(
        self,
        descriptor: int,
        root_mount: str,
        budget: list[int],
        *,
        inventory: dict[tuple[int, int], dict[str, int]] | None = None,
        remaining: Mapping[tuple[int, int], int] | None = None,
    ) -> None:
        if descriptor_mount_id(descriptor) != root_mount:
            raise ResetError("reset cleanup crosses a mount boundary")
        budget[0] += 1
        if budget[0] > 1_000_000:
            raise ResetError("reset cleanup entry bound exceeded")
        for name in os.listdir(descriptor):
            info = os.stat(name, dir_fd=descriptor, follow_symlinks=False)
            budget[0] += 1
            if budget[0] > 1_000_000:
                raise ResetError("reset cleanup entry bound exceeded")
            if stat.S_ISDIR(info.st_mode):
                child = os.open(
                    name,
                    os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
                    dir_fd=descriptor,
                )
                try:
                    if descriptor_mount_id(child) != root_mount:
                        raise ResetError("reset cleanup crosses a mount boundary")
                    if cleanup_identity(metadata_from_stat(os.fstat(child))) != cleanup_identity(
                        metadata_from_stat(info)
                    ):
                        raise ResetError("reset cleanup directory changed while opening")
                    self._preflight_tree_descriptor(
                        child, root_mount, budget,
                        inventory=inventory, remaining=remaining,
                    )
                finally:
                    os.close(child)
            elif stat.S_ISREG(info.st_mode):
                child = os.open(
                    name, os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW,
                    dir_fd=descriptor,
                )
                try:
                    if descriptor_mount_id(child) != root_mount:
                        raise ResetError("reset cleanup crosses a mount boundary")
                    opened = os.fstat(child)
                    if stable_metadata(metadata_from_stat(opened)) != stable_metadata(
                        metadata_from_stat(info)
                    ):
                        raise ResetError("reset cleanup file changed while opening")
                    if inventory is not None:
                        self._record_cleanup_link(inventory, opened)
                    else:
                        self._assert_cleanup_link_contained(remaining, opened)
                finally:
                    os.close(child)
            elif not stat.S_ISLNK(info.st_mode):
                raise ResetError("reset cleanup contains a special file")

    def _preflight_remove_tree(
        self,
        path: Path,
        expected: Mapping[str, Any] | None = None,
        *,
        inventory: dict[tuple[int, int], dict[str, int]] | None = None,
        remaining: Mapping[tuple[int, int], int] | None = None,
    ) -> None:
        observed = metadata(path)
        if expected is not None and cleanup_identity(observed) != cleanup_identity(expected):
            raise ResetError("reset cleanup root was substituted")
        parent = os.open(
            path.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
        )
        child: int | None = None
        try:
            before = os.stat(path.name, dir_fd=parent, follow_symlinks=False)
            before_metadata = metadata_from_stat(before)
            if before_metadata["kind"] == "symlink":
                before_metadata["target"] = os.readlink(path.name, dir_fd=parent)
            if cleanup_identity(before_metadata) != cleanup_identity(observed):
                raise ResetError("reset cleanup root changed while opening parent")
            if observed["kind"] == "symlink":
                return
            if observed["kind"] not in {"directory", "regular"}:
                raise ResetError("reset cleanup contains a special root")
            flags = os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW
            if observed["kind"] == "directory":
                flags |= os.O_DIRECTORY
            child = os.open(path.name, flags, dir_fd=parent)
            if stable_metadata(metadata_from_stat(os.fstat(child))) != stable_metadata(observed):
                raise ResetError("reset cleanup root was substituted while opening")
            if descriptor_mount_id(child) != descriptor_mount_id(parent):
                raise ResetError("reset cleanup root is a mount boundary")
            if observed["kind"] == "regular":
                opened = os.fstat(child)
                if stable_metadata(metadata_from_stat(opened)) != stable_metadata(
                    metadata_from_stat(before)
                ):
                    raise ResetError("reset cleanup root changed while opening")
                if inventory is not None:
                    self._record_cleanup_link(inventory, opened)
                else:
                    self._assert_cleanup_link_contained(remaining, opened)
            else:
                self._preflight_tree_descriptor(
                    child, descriptor_mount_id(child), [0],
                    inventory=inventory, remaining=remaining,
                )
        finally:
            if child is not None:
                os.close(child)
            os.close(parent)

    def _remove_tree(
        self,
        path: Path,
        *,
        expected: Mapping[str, Any] | None = None,
        hardlinks: dict[tuple[int, int], int] | None = None,
    ) -> None:
        root_metadata = metadata(path)
        if expected is not None and cleanup_identity(root_metadata) != cleanup_identity(expected):
            raise ResetError("reset cleanup root was substituted")
        parent_descriptor = os.open(
            path.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
        )
        root_descriptor: int | None = None
        try:
            parent_mount = descriptor_mount_id(parent_descriptor)
            before = os.stat(path.name, dir_fd=parent_descriptor, follow_symlinks=False)
            before_metadata = metadata_from_stat(before)
            if before_metadata["kind"] == "symlink":
                before_metadata["target"] = os.readlink(path.name, dir_fd=parent_descriptor)
            if cleanup_identity(before_metadata) != cleanup_identity(root_metadata):
                raise ResetError("reset cleanup root changed while opening parent")
            if root_metadata["kind"] != "directory":
                if root_metadata["kind"] not in {"regular", "symlink"}:
                    raise ResetError("reset cleanup contains a special root")
                if root_metadata["kind"] == "regular":
                    root_descriptor = os.open(
                        path.name, os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW,
                        dir_fd=parent_descriptor,
                    )
                    if descriptor_mount_id(root_descriptor) != parent_mount:
                        raise ResetError("reset cleanup root is a mount boundary")
                    opened = os.fstat(root_descriptor)
                    if stable_metadata(metadata_from_stat(opened)) != stable_metadata(
                        metadata_from_stat(before)
                    ):
                        raise ResetError("reset cleanup root changed while opening")
                    self._assert_cleanup_link_contained(hardlinks, opened)
                unlink_metadata = metadata_from_stat(
                    os.stat(path.name, dir_fd=parent_descriptor, follow_symlinks=False)
                )
                if unlink_metadata["kind"] == "symlink":
                    unlink_metadata["target"] = os.readlink(path.name, dir_fd=parent_descriptor)
                if stable_metadata(unlink_metadata) != stable_metadata(before_metadata):
                    raise ResetError("reset cleanup root was substituted before unlinking")
                os.unlink(path.name, dir_fd=parent_descriptor)
                if root_descriptor is not None:
                    key = self._hardlink_key(os.fstat(root_descriptor))
                    expected_remaining = 1 if hardlinks is None else hardlinks[key]
                    if os.fstat(root_descriptor).st_nlink != expected_remaining - 1:
                        raise ResetError("reset cleanup hardlink count changed while unlinking")
                    if hardlinks is not None:
                        if expected_remaining == 1:
                            del hardlinks[key]
                        else:
                            hardlinks[key] = expected_remaining - 1
                os.fsync(parent_descriptor)
                return
            root_descriptor = os.open(
                path.name,
                os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
                dir_fd=parent_descriptor,
            )
            if stable_metadata(metadata_from_stat(os.fstat(root_descriptor))) != stable_metadata(root_metadata):
                raise ResetError("reset cleanup root was substituted while opening")
            root_mount = descriptor_mount_id(root_descriptor)
            if root_mount != parent_mount:
                raise ResetError("reset cleanup root is a mount boundary")
            self._preflight_tree_descriptor(
                root_descriptor, root_mount, [0], remaining=hardlinks,
            )
        finally:
            if root_descriptor is not None:
                os.close(root_descriptor)
            os.close(parent_descriptor)

        deleted = 0

        def remove_contents(descriptor: int) -> None:
            nonlocal deleted
            if descriptor_mount_id(descriptor) != root_mount:
                raise ResetError("reset cleanup crosses a mount boundary")
            for name in sorted(os.listdir(descriptor), key=os.fsencode):
                before = os.stat(name, dir_fd=descriptor, follow_symlinks=False)
                if stat.S_ISDIR(before.st_mode):
                    child = os.open(
                        name,
                        os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
                        dir_fd=descriptor,
                    )
                    try:
                        if descriptor_mount_id(child) != root_mount:
                            raise ResetError("reset cleanup crosses a mount boundary")
                        if cleanup_identity(metadata_from_stat(os.fstat(child))) != cleanup_identity(
                            metadata_from_stat(before)
                        ):
                            raise ResetError("reset cleanup entry changed while opening")
                        remove_contents(child)
                        after = os.stat(name, dir_fd=descriptor, follow_symlinks=False)
                        if cleanup_identity(metadata_from_stat(after)) != cleanup_identity(
                            metadata_from_stat(before)
                        ):
                            raise ResetError("reset cleanup directory was substituted")
                    finally:
                        os.close(child)
                    os.rmdir(name, dir_fd=descriptor)
                elif stat.S_ISREG(before.st_mode):
                    child = os.open(
                        name, os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW,
                        dir_fd=descriptor,
                    )
                    try:
                        if descriptor_mount_id(child) != root_mount:
                            raise ResetError("reset cleanup crosses a mount boundary")
                        if stable_metadata(metadata_from_stat(os.fstat(child))) != stable_metadata(
                            metadata_from_stat(before)
                        ):
                            raise ResetError("reset cleanup file changed while opening")
                        after = os.stat(name, dir_fd=descriptor, follow_symlinks=False)
                        if stable_metadata(metadata_from_stat(after)) != stable_metadata(
                            metadata_from_stat(before)
                        ):
                            raise ResetError("reset cleanup file was substituted")
                        self._assert_cleanup_link_contained(hardlinks, after)
                        key = self._hardlink_key(after)
                        expected_remaining = 1 if hardlinks is None else hardlinks[key]
                        os.unlink(name, dir_fd=descriptor)
                        if os.fstat(child).st_nlink != expected_remaining - 1:
                            raise ResetError("reset cleanup hardlink count changed while unlinking")
                        if hardlinks is not None:
                            if expected_remaining == 1:
                                del hardlinks[key]
                            else:
                                hardlinks[key] = expected_remaining - 1
                    finally:
                        os.close(child)
                elif stat.S_ISLNK(before.st_mode):
                    after = os.stat(name, dir_fd=descriptor, follow_symlinks=False)
                    if stable_metadata(metadata_from_stat(after)) != stable_metadata(
                        metadata_from_stat(before)
                    ):
                        raise ResetError("reset cleanup symlink was substituted")
                    os.unlink(name, dir_fd=descriptor)
                else:
                    raise ResetError("reset cleanup contains a special file")
                os.fsync(descriptor)
                deleted += 1
                self.failpoint(f"after-tree-delete:{path.name}:{deleted}")

        parent_descriptor = os.open(
            path.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
        )
        try:
            root_descriptor = os.open(
                path.name,
                os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
                dir_fd=parent_descriptor,
            )
            try:
                if stable_metadata(metadata_from_stat(os.fstat(root_descriptor))) != stable_metadata(root_metadata):
                    raise ResetError("reset cleanup root was substituted before deleting contents")
                if (
                    descriptor_mount_id(parent_descriptor) != root_mount
                    or descriptor_mount_id(root_descriptor) != root_mount
                ):
                    raise ResetError("reset cleanup root crossed a mount boundary")
                remove_contents(root_descriptor)
            finally:
                os.close(root_descriptor)
            observed = os.stat(path.name, dir_fd=parent_descriptor, follow_symlinks=False)
            if cleanup_identity(metadata_from_stat(observed)) != cleanup_identity(root_metadata):
                raise ResetError("reset cleanup root was substituted before removal")
            os.rmdir(path.name, dir_fd=parent_descriptor)
            os.fsync(parent_descriptor)
        finally:
            os.close(parent_descriptor)

    def _preflight_delete_entries(
        self, entries: Iterable[Mapping[str, Any]],
    ) -> dict[tuple[int, int], int]:
        values = list(entries)
        inventory: dict[tuple[int, int], dict[str, int]] = {}
        for entry in values:
            self._entry_schema(entry)
            quarantine = self.layout.path(str(entry["quarantine"]))
            if not (quarantine.exists() or quarantine.is_symlink()):
                continue
            if cleanup_identity(metadata(quarantine)) != cleanup_identity(entry["metadata"]):
                raise ResetError("quarantined reset object was substituted before cleanup")
            self._preflight_remove_tree(
                quarantine, entry["metadata"], inventory=inventory,
            )
        for value in inventory.values():
            if value["count"] != value["nlink"]:
                raise ResetError("reset cleanup hardlinked file escapes the cleanup scope")
        return {
            key: value["count"] for key, value in inventory.items()
        }

    def _delete_entries(self, entries: Iterable[Mapping[str, Any]]) -> None:
        values = list(entries)
        remaining = self._preflight_delete_entries(values)
        for index, entry in enumerate(values):
            quarantine = self.layout.path(str(entry["quarantine"]))
            if not (quarantine.exists() or quarantine.is_symlink()):
                continue
            self._remove_tree(
                quarantine, expected=entry["metadata"], hardlinks=remaining,
            )
            self.failpoint(f"after-delete:{index}")
        if remaining:
            raise ResetError("reset cleanup hardlink inventory was not fully removed")

    def _completion_record(self, journal: Mapping[str, Any], outcome: str) -> dict[str, Any]:
        if outcome not in {"finalized", "rolled_back"}:
            raise ResetError("reset completion outcome differs")
        new = journal.get("new") if outcome == "finalized" else None
        if outcome == "finalized" and not isinstance(new, dict):
            raise ResetError("finalized reset has no new install identity")
        return {
            "schema_version": SCHEMA_VERSION,
            "outcome": outcome,
            "transaction_id": journal["transaction_id"],
            "machine_id": journal["machine_id"],
            "catalog_sha": journal["catalog_sha"],
            "old_install_identity": journal["old_install_identity"],
            "old_closure_digest": journal["old_closure_digest"],
            "helper_sha256": journal["helper_sha256"],
            "install_lib_sha256": journal["install_lib_sha256"],
            "units": journal["units"],
            "public_hop_contract": journal["public_hop_contract"],
            "projection_digests": {
                client: journal["prepared"]["manifest"]["clients"][client]["projection_digest"]
                for client in CLIENTS
            },
            "new": new,
        }

    def _read_completion(self, optional: bool = False) -> dict[str, Any] | None:
        if not (self.completion_path.exists() or self.completion_path.is_symlink()):
            if optional:
                return None
            raise ResetError("reset completion record is absent")
        value = strict_json(read_control(
            self.completion_path, self.layout.expected_uid, 0o600, 64 << 10,
        ))
        required = {
            "schema_version", "outcome", "transaction_id", "machine_id",
            "catalog_sha", "old_install_identity", "old_closure_digest", "helper_sha256",
            "install_lib_sha256", "units", "public_hop_contract",
            "projection_digests", "new",
        }
        if (
            not isinstance(value, dict)
            or set(value) != required
            or value.get("schema_version") != SCHEMA_VERSION
            or value.get("outcome") not in {"finalized", "rolled_back"}
            or re.fullmatch(r"[0-9a-f]{24}", str(value.get("transaction_id", ""))) is None
            or MACHINE_ID.fullmatch(str(value.get("machine_id", ""))) is None
            or re.fullmatch(r"[0-9a-f]{40}", str(value.get("catalog_sha", ""))) is None
            or DIGEST.fullmatch(str(value.get("old_install_identity", ""))) is None
            or DIGEST.fullmatch(str(value.get("old_closure_digest", ""))) is None
            or re.fullmatch(r"sha256:[0-9a-f]{64}", str(value.get("helper_sha256", ""))) is None
            or re.fullmatch(r"sha256:[0-9a-f]{64}", str(value.get("install_lib_sha256", ""))) is None
            or not isinstance(value.get("units"), dict)
            or set(value["units"]) != set(ALL_UNITS)
            or any(
                not isinstance(state, dict)
                or set(state) != {"active", "enabled"}
                or any(not isinstance(flag, bool) for flag in state.values())
                for state in value["units"].values()
            )
            or not isinstance(value.get("projection_digests"), dict)
            or set(value["projection_digests"]) != set(CLIENTS)
            or any(
                re.fullmatch(r"sha256:[0-9a-f]{64}", str(digest)) is None
                for digest in value["projection_digests"].values()
            )
        ):
            raise ResetError("reset completion record schema differs")
        self._validate_ingress_states(value["units"])
        if (
            value["units"][PUBLIC_HOP_UNIT]["active"]
            and not isinstance(value["public_hop_contract"], dict)
        ) or (
            not value["units"][PUBLIC_HOP_UNIT]["active"]
            and value["public_hop_contract"] is not None
        ):
            raise ResetError("reset completion public ingress binding differs")
        new = value["new"]
        if value["outcome"] == "rolled_back":
            if new is not None:
                raise ResetError("rolled-back completion unexpectedly binds a new install")
        elif (
            not isinstance(new, dict)
            or set(new) != {"install_identity", "closure_digest"}
            or DIGEST.fullmatch(str(new.get("install_identity", ""))) is None
            or DIGEST.fullmatch(str(new.get("closure_digest", ""))) is None
        ):
            raise ResetError("finalized completion new install binding differs")
        if value["transaction_id"] != self._transaction_id(
            value["machine_id"], value["catalog_sha"],
            value["old_install_identity"], value["old_closure_digest"],
        ):
            raise ResetError("reset completion transaction identity differs")
        return value

    def _write_completion(self, record: Mapping[str, Any]) -> None:
        encoded = json.dumps(
            record, sort_keys=True, separators=(",", ":"), ensure_ascii=True,
        ).encode() + b"\n"
        temporary = self.completion_path.with_name(self.completion_path.name + ".new")
        if self.completion_path.exists() or self.completion_path.is_symlink():
            if temporary.exists() or temporary.is_symlink():
                raise ResetError("reset completion temporary path also exists")
            if read_control(
                self.completion_path, self.layout.expected_uid, 0o600, 64 << 10,
            ) != encoded:
                raise ResetError("reset completion record differs")
            return
        if temporary.exists() or temporary.is_symlink():
            if read_control(
                temporary, self.layout.expected_uid, 0o600, 64 << 10,
            ) != encoded:
                raise ResetError("reset completion temporary record differs")
        else:
            descriptor = os.open(
                temporary,
                os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC | os.O_NOFOLLOW,
                0o600,
            )
            try:
                view = memoryview(encoded)
                while view:
                    written = os.write(descriptor, view)
                    if written <= 0:
                        raise ResetError("short reset completion write")
                    view = view[written:]
                os.fsync(descriptor)
            finally:
                os.close(descriptor)
        os.replace(temporary, self.completion_path)
        fsync_directory(self.completion_path.parent)

    def _validate_completion(
        self,
        completion: Mapping[str, Any],
        outcome: str,
        machine: str,
        catalog: str,
        old_install: str,
        old_closure: str,
        new_install: str | None = None,
        new_closure: str | None = None,
    ) -> None:
        expected_new = (
            None if new_install is None
            else {"install_identity": new_install, "closure_digest": new_closure}
        )
        expected = {
            "schema_version": SCHEMA_VERSION,
            "outcome": outcome,
            "transaction_id": self._transaction_id(
                machine, catalog, old_install, old_closure,
            ),
            "machine_id": machine,
            "catalog_sha": catalog,
            "old_install_identity": old_install,
            "old_closure_digest": old_closure,
            "helper_sha256": completion["helper_sha256"],
            "install_lib_sha256": completion["install_lib_sha256"],
            "units": completion["units"],
            "public_hop_contract": completion["public_hop_contract"],
            "projection_digests": completion["projection_digests"],
            "new": expected_new,
        }
        if completion != expected:
            raise ResetError("reset completion record binding differs")

    def _assert_no_quarantine(self, transaction: str | None) -> None:
        prefix = (
            ".uap-observer-reset-"
            if transaction is None else f".uap-observer-reset-{transaction}"
        )
        for parent in (
            "/opt", "/var/lib", "/etc/systemd/system", "/usr/local/libexec",
            "/usr/local/bin", "/etc/caddy", "/etc",
        ):
            root = self.layout.path(parent)
            if root.exists() and any(
                path.name.startswith(prefix)
                for path in root.iterdir()
            ):
                raise ResetError("completed reset still has quarantine residue")

    def _finish_completion(self, completion: Mapping[str, Any]) -> None:
        if self.journal_path.exists() or self.journal_path.is_symlink():
            raise ResetError("reset journal remains during completion recovery")
        if self.journal_dir.exists() or self.journal_dir.is_symlink():
            validate_root_control(
                self.journal_dir, self.layout.expected_uid, 0o700, "directory",
            )
            if any(self.journal_dir.iterdir()):
                raise ResetError("reset journal directory is not empty")
            self.journal_dir.rmdir()
            fsync_directory(self.journal_dir.parent)
            self.failpoint("after-journal-dir-remove")
        observed = self._read_completion()
        if observed != completion:
            raise ResetError("reset completion record changed")
        self.completion_path.unlink()
        fsync_directory(self.completion_path.parent)
        self.failpoint("after-completion-remove")

    def _remove_transaction_roots(self, journal: Mapping[str, Any], outcome: str) -> None:
        transaction = journal["transaction_id"]
        roots: set[Path] = set()
        for entry in [*journal["old"], *journal["candidate"]]:
            roots.add(self.layout.path(entry["quarantine"]).parent)
        if not all(f".uap-observer-reset-{transaction}" in str(root) for root in roots):
            raise ResetError("unexpected reset quarantine root")
        for root in sorted(roots, key=lambda path: len(path.parts), reverse=True):
            if root.exists():
                validate_root_control(root, self.layout.expected_uid, 0o700, "directory")
                if any(root.iterdir()):
                    raise ResetError(f"reset quarantine is not empty: {root.name}")
                root.rmdir()
                fsync_directory(root.parent)
        completion = self._completion_record(journal, outcome)
        self._write_completion(completion)
        self.failpoint("after-completion-marker")
        if self.journal_path.exists() or self.journal_path.is_symlink():
            if self._read_journal() != journal:
                raise ResetError("reset journal changed before completion")
            self.journal_path.unlink()
            fsync_directory(self.journal_dir)
        self.failpoint("after-journal-unlink")
        if any(self.journal_dir.iterdir()):
            raise ResetError("reset journal directory is not empty")
        self.journal_dir.rmdir()
        fsync_directory(self.journal_dir.parent)
        self.failpoint("after-journal-dir-remove")
        if self._read_completion() != completion:
            raise ResetError("reset completion record changed")
        self.completion_path.unlink()
        fsync_directory(self.completion_path.parent)
        self.failpoint("after-completion-remove")


def parser() -> argparse.ArgumentParser:
    value = argparse.ArgumentParser(description=__doc__)
    subcommands = value.add_subparsers(dest="command", required=True)

    def old(command: str) -> argparse.ArgumentParser:
        child = subcommands.add_parser(command)
        child.add_argument("--machine-id", required=True)
        child.add_argument("--catalog-sha", required=True)
        child.add_argument("--old-install-identity", required=True)
        child.add_argument("--old-closure-digest", required=True)
        return child

    old("apply")
    status = subcommands.add_parser("status")
    status.add_argument("--machine-id", required=True)
    old("rollback")
    final = old("finalize")
    final.add_argument("--new-install-identity", required=True)
    final.add_argument("--new-closure-digest", required=True)
    return value


def main(argv: Sequence[str] | None = None) -> int:
    if os.geteuid() != 0:
        raise ResetError("observer reset must run as root")
    arguments = parser().parse_args(argv)
    controller = ResetController(Layout.production())
    if arguments.command == "status":
        result = controller.status(arguments.machine_id)
    elif arguments.command == "apply":
        result = controller.apply(
            arguments.machine_id, arguments.catalog_sha, arguments.old_install_identity,
            arguments.old_closure_digest,
        )
    elif arguments.command == "rollback":
        result = controller.rollback(
            arguments.machine_id, arguments.catalog_sha, arguments.old_install_identity,
            arguments.old_closure_digest,
        )
    else:
        result = controller.finalize(
            arguments.machine_id, arguments.catalog_sha, arguments.old_install_identity,
            arguments.old_closure_digest, arguments.new_install_identity,
            arguments.new_closure_digest,
        )
    print(json.dumps(result, sort_keys=True, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except ResetError as error:
        print(f"observer reset failed: {error}", file=sys.stderr)
        raise SystemExit(1)
