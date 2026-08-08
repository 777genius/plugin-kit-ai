"""Portable package-tree path rules shared by catalog build and validation."""

from __future__ import annotations

import unicodedata
from pathlib import Path


WINDOWS_RESERVED = {"CON", "PRN", "AUX", "NUL", "CLOCK$"} | {
    f"{prefix}{number}" for prefix in ("COM", "LPT") for number in range(1, 10)
}
WINDOWS_INVALID = set('<>:"/\\|?*')
MAX_FILES = 10_000
MAX_FILE_BYTES = 64 << 20
MAX_TREE_BYTES = 256 << 20
MAX_DEPTH = 64


def validate_segment(value: str) -> None:
    if not value or value in {".", ".."} or unicodedata.normalize("NFC", value) != value:
        raise ValueError(f"invalid portable path segment: {value!r}")
    if len(value.encode("utf-16-le")) // 2 > 255:
        raise ValueError(f"portable path segment is too long: {value!r}")
    if value.endswith((" ", ".")) or any(char in WINDOWS_INVALID or ord(char) < 0x20 for char in value):
        raise ValueError(f"Windows-incompatible path segment: {value!r}")
    if value.split(".", 1)[0].upper() in WINDOWS_RESERVED:
        raise ValueError(f"Windows-reserved path segment: {value!r}")


def validate_tree(root: Path) -> None:
    seen: dict[str, str] = {}
    file_count = 0
    total_bytes = 0
    for path in root.rglob("*"):
        relative_path = path.relative_to(root)
        relative = relative_path.as_posix()
        if path.is_symlink():
            raise ValueError(f"portable package contains a symlink: {relative!r}")
        if not (path.is_dir() or path.is_file()):
            raise ValueError(f"portable package contains a special file: {relative!r}")
        if len(relative_path.parts) > MAX_DEPTH:
            raise ValueError(f"portable package path exceeds depth {MAX_DEPTH}: {relative!r}")
        for part in relative_path.parts:
            validate_segment(part)
        folded = relative.casefold()
        previous = seen.get(folded)
        if previous is not None and previous != relative:
            raise ValueError(f"portable path collision: {previous!r} and {relative!r}")
        seen[folded] = relative
        if path.is_file():
            file_count += 1
            if file_count > MAX_FILES:
                raise ValueError(f"portable package exceeds {MAX_FILES} files")
            size = path.stat().st_size
            if size > MAX_FILE_BYTES:
                raise ValueError(f"portable package file exceeds {MAX_FILE_BYTES} bytes: {relative!r}")
            total_bytes += size
            if total_bytes > MAX_TREE_BYTES:
                raise ValueError(f"portable package exceeds {MAX_TREE_BYTES} total bytes")
