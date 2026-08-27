"""Digest-bound immutable client bundle validation."""

from __future__ import annotations

import hashlib
import json
import math
import os
import stat
from pathlib import Path, PurePosixPath
from typing import Any

MAX_MANIFEST_BYTES = 8 << 20
MAX_FILES = 20_000
MAX_FILE_BYTES = 2 << 30
MAX_TOTAL_BYTES = 3 << 30
ALLOWED_MODES = {0o644, 0o755}


def canonical_json(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode() + b"\n"


def strict_json(encoded: bytes) -> Any:
    def pairs(items: list[tuple[str, Any]]) -> dict[str, Any]:
        value: dict[str, Any] = {}
        folded: set[str] = set()
        for key, child in items:
            normalized = key.casefold()
            if key in value or normalized in folded:
                raise ValueError("client bundle manifest has duplicate or case-confusable members")
            value[key] = child
            folded.add(normalized)
        return value

    def constant(value: str) -> None:
        raise ValueError(f"client bundle manifest has a non-finite number: {value}")

    def finite(value: str) -> float:
        decoded = float(value)
        if not math.isfinite(decoded):
            constant(value)
        return decoded

    return json.loads(encoded, object_pairs_hook=pairs, parse_constant=constant, parse_float=finite)


def _safe_relative(value: Any) -> PurePosixPath:
    if not isinstance(value, str) or not value or "\\" in value or "\x00" in value:
        raise ValueError("client bundle path is invalid")
    path = PurePosixPath(value)
    if path.is_absolute() or any(part in {"", ".", ".."} for part in path.parts) or str(path) != value:
        raise ValueError("client bundle path is not canonical")
    return path


def _digest_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        while block := stream.read(1 << 20):
            digest.update(block)
    return "sha256:" + digest.hexdigest()


def inventory_bundle(
    root: Path, *, owner_uid: int = 0, owner_gid: int = 0,
) -> dict[str, Any]:
    if not root.is_absolute() or root.resolve(strict=True) != root:
        raise ValueError("client bundle root is not an exact absolute directory")
    root_info = os.lstat(root)
    if (
        not stat.S_ISDIR(root_info.st_mode)
        or root_info.st_uid != owner_uid
        or root_info.st_gid != owner_gid
        or stat.S_IMODE(root_info.st_mode) != 0o755
    ):
        raise ValueError("client bundle root metadata differs")

    records: list[dict[str, Any]] = []
    total_size = 0
    for current, directories, files in os.walk(root, topdown=True, followlinks=False):
        current_path = Path(current)
        directories.sort()
        files.sort()
        for name in directories:
            path = current_path / name
            info = os.lstat(path)
            if (
                not stat.S_ISDIR(info.st_mode)
                or info.st_uid != owner_uid
                or info.st_gid != owner_gid
                or stat.S_IMODE(info.st_mode) != 0o755
            ):
                raise ValueError("client bundle directory metadata differs")
        for name in files:
            path = current_path / name
            info = os.lstat(path)
            mode = stat.S_IMODE(info.st_mode)
            if (
                not stat.S_ISREG(info.st_mode)
                or info.st_uid != owner_uid
                or info.st_gid != owner_gid
                or info.st_nlink != 1
                or mode not in ALLOWED_MODES
                or info.st_size > MAX_FILE_BYTES
            ):
                raise ValueError("client bundle file metadata differs")
            relative = path.relative_to(root).as_posix()
            _safe_relative(relative)
            total_size += info.st_size
            if total_size > MAX_TOTAL_BYTES or len(records) >= MAX_FILES:
                raise ValueError("client bundle exceeds its bounded closure")
            records.append({
                "mode": f"{mode:04o}",
                "path": relative,
                "sha256": _digest_file(path),
                "size": info.st_size,
            })
    records.sort(key=lambda item: item["path"])
    if not records:
        raise ValueError("client bundle is empty")
    return {"files": records, "schema_version": 1}


def verify_bundle(
    *, root: Path, manifest: Path, manifest_sha256: str,
    owner_uid: int = 0, owner_gid: int = 0,
) -> tuple[Path, ...]:
    if not manifest.is_absolute() or manifest.resolve(strict=True) != manifest:
        raise ValueError("client bundle manifest path is invalid")
    info = os.lstat(manifest)
    if (
        not stat.S_ISREG(info.st_mode)
        or info.st_uid != owner_uid
        or info.st_gid != owner_gid
        or info.st_nlink != 1
        or stat.S_IMODE(info.st_mode) != 0o644
        or info.st_size > MAX_MANIFEST_BYTES
    ):
        raise ValueError("client bundle manifest metadata differs")
    encoded = manifest.read_bytes()
    if "sha256:" + hashlib.sha256(encoded).hexdigest() != manifest_sha256:
        raise ValueError("client bundle manifest digest differs")
    value = strict_json(encoded)
    if not isinstance(value, dict) or set(value) != {"files", "schema_version"} or type(value.get("schema_version")) is not int or value["schema_version"] != 1:
        raise ValueError("client bundle manifest is not canonical")
    files = value.get("files")
    if not isinstance(files, list) or not files or len(files) > MAX_FILES:
        raise ValueError("client bundle manifest file list is invalid")
    previous = ""
    for item in files:
        if not isinstance(item, dict) or set(item) != {"mode", "path", "sha256", "size"}:
            raise ValueError("client bundle manifest entry differs")
        path = _safe_relative(item["path"])
        if item["path"] <= previous:
            raise ValueError("client bundle manifest paths are not strictly sorted")
        previous = item["path"]
        if item["mode"] not in {"0644", "0755"} or type(item["size"]) is not int or item["size"] < 0:
            raise ValueError("client bundle manifest metadata is invalid")
        if not isinstance(item["sha256"], str) or len(item["sha256"]) != 71 or not item["sha256"].startswith("sha256:"):
            raise ValueError("client bundle manifest digest is invalid")
        if any(character not in "0123456789abcdef" for character in item["sha256"][7:]):
            raise ValueError("client bundle manifest digest is invalid")
        candidate = root.joinpath(*path.parts)
        if candidate.parent != root and root not in candidate.parents:
            raise ValueError("client bundle path escapes its root")
    if canonical_json(value) != encoded:
        raise ValueError("client bundle manifest bytes are not canonical")
    observed = inventory_bundle(root, owner_uid=owner_uid, owner_gid=owner_gid)
    if observed != value:
        raise ValueError("client bundle bytes differ from the manifest")
    return tuple(root / item["path"] for item in files)
