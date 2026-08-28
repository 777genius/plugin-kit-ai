#!/usr/bin/env python3
"""Bind the protected headless Chrome closure into one disposable profile seed."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import stat
import tempfile
from pathlib import Path
from typing import Any


MAX_CONFIG_BYTES = 4 << 20
CHROME_BINARY = "/opt/uap-observer-inputs/chrome-for-testing/chrome"
CHROME_RUNTIME_ARGUMENTS = (
    "--headless",
    f"--executablePath={CHROME_BINARY}",
    "--isolated",
    "--chrome-arg=--no-sandbox",
    "--chrome-arg=--disable-setuid-sandbox",
)
BROWSER_SELECTORS = (
    "--browserUrl", "--browser-url", "--wsEndpoint", "--ws-endpoint",
    "--channel", "--autoConnect", "--auto-connect", "--userDataDir",
    "--user-data-dir", "--executablePath", "--executable-path",
)


def strict_json(encoded: bytes) -> Any:
    def pairs(items: list[tuple[str, Any]]) -> dict[str, Any]:
        value: dict[str, Any] = {}
        folded: set[str] = set()
        for key, child in items:
            normalized = key.casefold()
            if key in value or normalized in folded:
                raise ValueError("JSON contains a duplicate or case-confusable member")
            value[key] = child
            folded.add(normalized)
        return value

    def constant(value: str) -> None:
        raise ValueError(f"JSON contains a non-finite number: {value}")

    def finite(value: str) -> float:
        decoded = float(value)
        if not math.isfinite(decoded):
            constant(value)
        return decoded

    return json.loads(encoded, object_pairs_hook=pairs, parse_constant=constant, parse_float=finite)


def protected_regular(path: Path, *, modes: set[int]) -> bytes:
    if not path.is_absolute() or ".." in path.parts or path.is_symlink():
        raise ValueError("protected file path is invalid")
    descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        before = os.fstat(descriptor)
        if (
            not stat.S_ISREG(before.st_mode) or before.st_uid != 0
            or stat.S_IMODE(before.st_mode) not in modes or before.st_nlink != 1
            or before.st_size > MAX_CONFIG_BYTES
        ):
            raise ValueError("protected file metadata differs")
        encoded = bytearray()
        while len(encoded) <= MAX_CONFIG_BYTES:
            chunk = os.read(descriptor, min(1 << 20, MAX_CONFIG_BYTES + 1 - len(encoded)))
            if not chunk:
                break
            encoded.extend(chunk)
        after = os.fstat(descriptor)
        if len(encoded) != before.st_size or len(encoded) > MAX_CONFIG_BYTES or (
            after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns, after.st_ctime_ns
        ) != (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns, before.st_ctime_ns):
            raise ValueError("protected file changed while being read")
        return bytes(encoded)
    finally:
        os.close(descriptor)


def checked_target(root: Path, relative: str) -> Path:
    if (
        not isinstance(relative, str) or not relative
        or Path(relative).is_absolute()
        or any(part in {"", ".", ".."} for part in Path(relative).parts)
    ):
        raise ValueError("native config map path is invalid")
    target = root.joinpath(relative)
    if target.is_symlink() or target.resolve(strict=True) != target or root not in target.parents:
        raise ValueError("native config escapes the protected profile seed")
    return target


def has_selector(argument: str, selector: str) -> bool:
    return argument == selector or argument.startswith(selector + "=")


def configure(value: Any, root: Path) -> bytes:
    servers = value.get("mcpServers") if isinstance(value, dict) else None
    server = servers.get("chrome-devtools") if isinstance(servers, dict) else None
    args = server.get("args") if isinstance(server, dict) else None
    if (
        not isinstance(server, dict) or server.get("command") != "node"
        or not isinstance(args, list) or any(not isinstance(argument, str) for argument in args)
        or len(args) < 2 or args.count("--no-usage-statistics") != 1
    ):
        raise ValueError("chrome-devtools native MCP config differs from the reviewed package")
    launcher = Path(args[0])
    if (
        not launcher.is_absolute() or launcher.name != "launcher.mjs"
        or root not in launcher.parents
    ):
        raise ValueError("chrome-devtools launcher escapes the disposable profile seed")
    if tuple(args[-len(CHROME_RUNTIME_ARGUMENTS):]) == CHROME_RUNTIME_ARGUMENTS:
        if any(args.count(argument) != 1 for argument in CHROME_RUNTIME_ARGUMENTS):
            raise ValueError("chrome-devtools runtime arguments are duplicated")
        return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode() + b"\n"
    if any(has_selector(argument, selector) for argument in args for selector in BROWSER_SELECTORS):
        raise ValueError("chrome-devtools already contains a conflicting browser selector")
    if any(argument in CHROME_RUNTIME_ARGUMENTS for argument in args):
        raise ValueError("chrome-devtools contains a partial protected browser configuration")
    server["args"] = [*args, *CHROME_RUNTIME_ARGUMENTS]
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode() + b"\n"


def atomic_replace(path: Path, encoded: bytes) -> None:
    descriptor, temporary = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    temporary_path = Path(temporary)
    try:
        os.fchmod(descriptor, 0o600)
        os.fchown(descriptor, 0, 0)
        view = memoryview(encoded)
        while view:
            written = os.write(descriptor, view)
            if written <= 0:
                raise OSError("short write while configuring Chrome")
            view = view[written:]
        os.fsync(descriptor)
        os.close(descriptor)
        descriptor = -1
        protected_regular(path, modes={0o600})
        os.replace(temporary_path, path)
        parent = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
        try:
            os.fsync(parent)
        finally:
            os.close(parent)
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        try:
            temporary_path.unlink()
        except FileNotFoundError:
            pass


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--root-owned-seed", type=Path, required=True)
    parser.add_argument("--native-config-map", type=Path, required=True)
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise SystemExit("Chrome profile configuration requires root")
    root = args.root_owned_seed.resolve(strict=True)
    root_info = os.lstat(root)
    if (
        root != args.root_owned_seed or not stat.S_ISDIR(root_info.st_mode)
        or root_info.st_uid != 0 or stat.S_IMODE(root_info.st_mode) != 0o700
    ):
        raise ValueError("profile seed is not the exact root-owned 0700 directory")
    mapping = strict_json(protected_regular(args.native_config_map, modes={0o400, 0o600}))
    if not isinstance(mapping, dict) or set(mapping) != {
        "agent-code-navigator", "context7", "cloudflare-docs", "chrome-devtools", "notion",
    }:
        raise ValueError("native config map does not contain the exact hero set")
    target = checked_target(root, mapping["chrome-devtools"])
    current = protected_regular(target, modes={0o600})
    configured = configure(strict_json(current), root)
    if configured != current:
        atomic_replace(target, configured)
    print("sha256:" + hashlib.sha256(configured).hexdigest())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
