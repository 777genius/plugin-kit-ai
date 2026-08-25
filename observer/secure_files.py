"""No-follow, owner-checked file access for observer trust boundaries."""

from __future__ import annotations

import os
import stat
from pathlib import Path


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
) -> bytes:
    parent_fd = open_trusted_directory(path.parent, owner_uid=owner_uid)
    descriptor = -1
    try:
        descriptor = os.open(path.name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC, dir_fd=parent_fd)
        info = os.fstat(descriptor)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != owner_uid or info.st_mode & 0o022 or info.st_nlink != 1:
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
        os.close(parent_fd)


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
