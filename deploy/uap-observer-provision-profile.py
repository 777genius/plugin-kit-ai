#!/usr/bin/env python3
"""Provision one isolated test-auth profile without modifying its root-owned seed."""

from __future__ import annotations

import argparse
import hashlib
import os
import pwd
import stat
from pathlib import Path
from typing import Protocol

CLIENTS = {"codex", "cursor", "kiro"}
PROFILE_ROOT = Path("/var/lib/uap-observer/profiles")
MAX_FILES = 50_000
MAX_BYTES = 2 << 30
OPEN_DIRECTORY = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC | getattr(os, "O_NOATIME", 0)


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
    source_fd: int, destination_fd: int, framed: Digest, logical_parent: tuple[str, ...] = (),
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
                os.mkdir(name, 0o700, dir_fd=destination_fd)
                destination_child = os.open(name, OPEN_DIRECTORY, dir_fd=destination_fd)
                try:
                    child_count, child_total = copy_tree(source_child, destination_child, framed, logical_path)
                    count += child_count
                    total += child_total
                    if count > MAX_FILES or total > MAX_BYTES:
                        raise ValueError("profile seed exceeds copy bounds")
                finally:
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
            output = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600, dir_fd=destination_fd)
            copied = 0
            while chunk := os.read(source, 1 << 20):
                copied += len(chunk)
                total += len(chunk)
                if total > MAX_BYTES:
                    raise ValueError("profile seed exceeds byte bound")
                framed.update(chunk)
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
            os.fsync(output)
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


def assign_tree(directory_fd: int, uid: int, gid: int, *, include_self: bool = True) -> None:
    """Assign only descriptors within the root-created staging tree, bottom-up."""
    for name in os.listdir(directory_fd):
        info = os.stat(name, dir_fd=directory_fd, follow_symlinks=False)
        flags = OPEN_DIRECTORY if stat.S_ISDIR(info.st_mode) else os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC
        descriptor = os.open(name, flags, dir_fd=directory_fd)
        try:
            current = os.fstat(descriptor)
            if (current.st_dev, current.st_ino) != (info.st_dev, info.st_ino):
                raise ValueError("staged profile changed during ownership assignment")
            if stat.S_ISDIR(current.st_mode):
                assign_tree(descriptor, uid, gid)
            elif not stat.S_ISREG(current.st_mode) or current.st_nlink != 1:
                raise ValueError("staged profile was substituted")
            os.fchown(descriptor, uid, gid)
            os.fchmod(descriptor, 0o700 if stat.S_ISDIR(current.st_mode) else 0o600)
        finally:
            os.close(descriptor)
    if include_self:
        os.fchown(directory_fd, uid, gid)
        os.fchmod(directory_fd, 0o700)


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
    profile_root_fd = open_root_owned_directory(PROFILE_ROOT, final_mode=0o711)
    staging_name = f".{args.client}.new"
    staging_fd = -1
    created = False
    try:
        try:
            os.stat(staging_name, dir_fd=profile_root_fd, follow_symlinks=False)
        except FileNotFoundError:
            pass
        else:
            raise ValueError("stale profile staging entry exists")
        os.mkdir(staging_name, 0o700, dir_fd=profile_root_fd)
        created = True
        staging_fd = os.open(staging_name, OPEN_DIRECTORY, dir_fd=profile_root_fd)
        framed = hashlib.sha256(b"uap-observer-profile-seed-v1\0")
        copy_tree(source_fd, staging_fd, framed)
        digest = "sha256:" + framed.hexdigest()
        if args.seed_digest == "show":
            print(digest)
            return 0
        if not args.seed_digest.startswith("sha256:") or digest != args.seed_digest:
            raise ValueError("profile seed digest differs")
        try:
            target_info = os.stat(args.client, dir_fd=profile_root_fd, follow_symlinks=False)
        except FileNotFoundError:
            target_info = None
        if target_info is not None:
            if not stat.S_ISDIR(target_info.st_mode) or target_info.st_uid != account.pw_uid or stat.S_IMODE(target_info.st_mode) != 0o700:
                raise ValueError("client profile target is not the installed empty directory")
            target_fd = os.open(args.client, OPEN_DIRECTORY, dir_fd=profile_root_fd)
            try:
                if os.listdir(target_fd):
                    raise ValueError("client profile is already provisioned")
            finally:
                os.close(target_fd)
            os.rmdir(args.client, dir_fd=profile_root_fd)
        assign_tree(staging_fd, account.pw_uid, account.pw_gid, include_self=False)
        os.fsync(staging_fd)
        os.rename(staging_name, args.client, src_dir_fd=profile_root_fd, dst_dir_fd=profile_root_fd)
        created = False
        os.fchown(staging_fd, account.pw_uid, account.pw_gid)
        os.close(staging_fd)
        staging_fd = -1
    finally:
        if staging_fd >= 0:
            os.close(staging_fd)
        if created:
            remove_tree(profile_root_fd, staging_name)
        os.close(profile_root_fd)
        os.close(source_fd)
    print(f"isolated {args.client} profile provisioned; seed left untouched")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
