"""Portable package-tree path rules shared by catalog build and validation."""

from __future__ import annotations

import unicodedata
from pathlib import Path


WINDOWS_RESERVED = {"CON", "PRN", "AUX", "NUL", "CLOCK$"} | {
    f"{prefix}{number}" for prefix in ("COM", "LPT") for number in range(1, 10)
}
WINDOWS_INVALID = set('<>:"/\\|?*')


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
    for path in root.rglob("*"):
        relative = path.relative_to(root).as_posix()
        for part in path.relative_to(root).parts:
            validate_segment(part)
        folded = relative.casefold()
        previous = seen.get(folded)
        if previous is not None and previous != relative:
            raise ValueError(f"portable path collision: {previous!r} and {relative!r}")
        seen[folded] = relative
