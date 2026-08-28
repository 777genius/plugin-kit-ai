"""No-follow, owner-checked file access for observer trust boundaries."""

from __future__ import annotations

import os
import re
import stat
from pathlib import Path


IMMUTABLE_CLOSURE_ALIAS = Path("/opt/uap-observer-current")
_CLOSURE_TARGET = re.compile(r"uap-observer-closures/[a-f0-9]{64}")


def open_trusted_directory(path: Path, *, owner_uid: int, exact_mode: int | None = None) -> int:
    """Open an absolute directory without ever resolving a link in its chain."""
    if not path.is_absolute() or ".." in path.parts:
        raise ValueError("protected directory path is invalid")
    descriptor = os.open("/", os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for component in path.parts[1:]:
            child = os.open(
                component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC,
                dir_fd=descriptor,
            )
            os.close(descriptor)
            descriptor = child
            info = os.fstat(descriptor)
            # Root-owned sticky traversal anchors such as /tmp are safe only as
            # ancestors; every other component must exclude untrusted writes.
            sticky_root = info.st_uid == 0 and bool(info.st_mode & stat.S_ISVTX)
            if not stat.S_ISDIR(info.st_mode) or info.st_uid not in {0, owner_uid}:
                raise ValueError("protected directory component is not trusted")
            if info.st_mode & 0o022 and not sticky_root:
                raise ValueError("protected directory component is writable")
        info = os.fstat(descriptor)
        if exact_mode is not None and (info.st_uid != owner_uid or stat.S_IMODE(info.st_mode) != exact_mode):
            raise ValueError("protected directory ownership or mode differs")
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
            expected_nlink=expected_nlink, alias=IMMUTABLE_CLOSURE_ALIAS,
        )
    parent_fd = open_trusted_directory(path.parent, owner_uid=owner_uid)
    try:
        return _read_regular_at(
            parent_fd, path.name, limit, owner_uid=owner_uid, executable=executable,
            exact_mode=exact_mode, group_gid=group_gid,
            expected_nlink=expected_nlink,
        )
    finally:
        os.close(parent_fd)


def read_immutable_closure_regular(
    relative: Path, limit: int, *, owner_uid: int, executable: bool = False,
    exact_mode: int | None = None, group_gid: int | None = None,
    expected_nlink: int = 1, alias: Path = IMMUTABLE_CLOSURE_ALIAS,
    alias_owner_uid: int = 0,
) -> bytes:
    """Read through the single root-owned current-closure alias without TOCTOU."""
    if relative.is_absolute() or not relative.parts or ".." in relative.parts:
        raise ValueError("immutable closure path is invalid")
    opt_fd = open_trusted_directory(alias.parent, owner_uid=alias_owner_uid)
    closure_fd = -1
    parent_fd = -1
    try:
        alias_info = os.stat(alias.name, dir_fd=opt_fd, follow_symlinks=False)
        if not stat.S_ISLNK(alias_info.st_mode) or alias_info.st_uid != alias_owner_uid:
            raise ValueError("immutable closure alias is not a trusted root-owned symlink")
        target = os.readlink(alias.name, dir_fd=opt_fd)
        if not _CLOSURE_TARGET.fullmatch(target):
            raise ValueError("immutable closure alias target differs")
        closure_parent, digest = target.split("/", 1)
        closure_parent_fd = os.open(
            closure_parent, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC,
            dir_fd=opt_fd,
        )
        try:
            _require_trusted_directory(closure_parent_fd, owner_uid=alias_owner_uid)
            closure_fd = os.open(
                digest, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC,
                dir_fd=closure_parent_fd,
            )
        finally:
            os.close(closure_parent_fd)
        _require_trusted_directory(closure_fd, owner_uid=alias_owner_uid)
        parent_fd = os.dup(closure_fd)
        for component in relative.parts[:-1]:
            child = os.open(
                component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC,
                dir_fd=parent_fd,
            )
            os.close(parent_fd)
            parent_fd = child
            _require_trusted_directory(parent_fd, owner_uid=alias_owner_uid)
        return _read_regular_at(
            parent_fd, relative.name, limit, owner_uid=owner_uid,
            executable=executable, exact_mode=exact_mode, group_gid=group_gid,
            expected_nlink=expected_nlink,
        )
    finally:
        if parent_fd >= 0:
            os.close(parent_fd)
        if closure_fd >= 0:
            os.close(closure_fd)
        os.close(opt_fd)


def _require_trusted_directory(descriptor: int, *, owner_uid: int) -> None:
    info = os.fstat(descriptor)
    if (
        not stat.S_ISDIR(info.st_mode) or info.st_uid != owner_uid
        or info.st_mode & 0o022
    ):
        raise ValueError("immutable closure directory is not trusted")


def _read_regular_at(
    parent_fd: int, name: str, limit: int, *, owner_uid: int,
    executable: bool, exact_mode: int | None, group_gid: int | None,
    expected_nlink: int,
) -> bytes:
    descriptor = -1
    try:
        descriptor = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC, dir_fd=parent_fd)
        info = os.fstat(descriptor)
        if (
            not stat.S_ISREG(info.st_mode) or info.st_uid != owner_uid
            or info.st_mode & 0o022 or info.st_nlink != expected_nlink
        ):
            raise ValueError("protected file is not a trusted regular file")
        if executable and not info.st_mode & stat.S_IXUSR:
            raise ValueError("protected executable is not executable")
        if exact_mode is not None and stat.S_IMODE(info.st_mode) != exact_mode:
            raise ValueError("protected file mode differs")
        if group_gid is not None and info.st_gid != group_gid:
            raise ValueError("protected file group differs")
        if info.st_size > limit:
            raise ValueError("protected file exceeds size bound")
        chunks = []
        remaining = limit + 1
        while remaining:
            chunk = os.read(descriptor, min(64 << 10, remaining))
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


def write_new_owned(path: Path, value: bytes, *, mode: int = 0o600) -> None:
    parent_fd = open_trusted_directory(path.parent, owner_uid=os.geteuid())
    descriptor = -1
    try:
        descriptor = os.open(
            path.name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC,
            mode, dir_fd=parent_fd,
        )
        view = memoryview(value)
        while view:
            written = os.write(descriptor, view)
            view = view[written:]
        os.fsync(descriptor)
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        os.close(parent_fd)
