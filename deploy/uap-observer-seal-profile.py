#!/usr/bin/env python3
"""Seal reviewed manager and native-config evidence into one profile seed."""

from __future__ import annotations

import argparse
import ctypes
import hashlib
import json
import math
import os
import re
import stat
import tempfile
from pathlib import Path
from typing import Any

CLIENTS = {"codex", "cursor", "kiro"}
HEROES = {"agent-code-navigator", "context7", "cloudflare-docs", "chrome-devtools", "notion"}
ADD_SUCCESS_STATUSES = {"success", "completed", "external_completed"}
LIFECYCLE_FIELDS = {
    "status", "state", "outcome", "activation", "materialization", "verification",
    "policy", "native_identity_state", "health", "readiness", "connection",
}
FALSE_CONTROL_FIELDS = {
    "health", "healthy", "readiness", "ready", "connection", "connected",
    "connectivity", "enabled", "running", "loaded",
}
PROHIBITED_STATES = re.compile(
    r"(?:^|_)(?:manual(?:_activation)?_required|partial|unreconciled|mixed|"
    r"degraded|suspended|blocked|denied|disabled|inactive|unhealthy|not_ready|"
    r"error|failed|failure|cancelled|canceled)(?:$|_)",
)
MAX_NATIVE_CONFIG_BYTES = 4 << 20


def canonical(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()


def digest(body: bytes) -> str:
    return "sha256:" + hashlib.sha256(body).hexdigest()


def strict_json_loads(encoded: bytes) -> Any:
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


def lifecycle_name(value: Any) -> str:
    """Canonicalize lifecycle keys/values across case and word separators."""
    text = re.sub(r"(?<=[a-z0-9])(?=[A-Z])", "_", str(value))
    return re.sub(r"[_\s-]+", "_", text.casefold()).strip("_")


def prohibited_lifecycle_state(value: Any) -> bool:
    if isinstance(value, list):
        return any(prohibited_lifecycle_state(item) for item in value)
    if isinstance(value, str):
        return PROHIBITED_STATES.search(lifecycle_name(value)) is not None
    if not isinstance(value, dict):
        return False
    for key, child in value.items():
        lowered = lifecycle_name(key)
        if PROHIBITED_STATES.search(lowered) and child not in (None, "", False, 0, [], {}):
            return True
        if lowered in FALSE_CONTROL_FIELDS and (
            child is False
            or isinstance(child, (int, float)) and not isinstance(child, bool) and child == 0
        ):
            return True
        if lowered.replace("_", "") == "iserror" and child is not False:
            return True
        if lowered in {"success", "ok"} and not isinstance(child, (dict, list)):
            if isinstance(child, bool) and child is False:
                return True
            if isinstance(child, (int, float)) and not isinstance(child, bool):
                if child == 0:
                    return True
            elif child is not True:
                return True
        if lowered in {"error", "errors", "failed", "failure", "failures", "cancelled", "canceled", "cancellations"} and child not in (None, "", False, 0, [], {}):
            return True
        if lowered in {"mixed", "mixed_version", "degraded", "partial", "unreconciled", "suspended"} and child is not False:
            return True
        if lowered in {"receipt_reconciled", "native_discovery_reconciled", "audit_passed", "events_ok"} and child is not True:
            return True
        if lowered in {"policy_warnings", "audit_failures", "failed_events"} and child not in (None, "", [], {}):
            return True
        if lowered in {"warnings", "warning"} and child not in (None, "", [], {}):
            warning_text = json.dumps(child, ensure_ascii=False)
            if re.search(r"policy|suspend|degrad|manual[_\s-]activation|partial|unreconciled|fail|error|cancel", warning_text, re.IGNORECASE):
                return True
        if prohibited_lifecycle_state(child):
            return True
    return False


def read_protected(path: Path, *, exact_mode: int | None = None) -> bytes:
    """Read one root-owned, one-link regular file without following links."""
    if not path.is_absolute() or ".." in path.parts:
        raise ValueError("protected input path is invalid")
    parent = os.open("/", os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    descriptor = -1
    try:
        for component in path.parts[1:-1]:
            child = os.open(
                component,
                os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC,
                dir_fd=parent,
            )
            os.close(parent)
            parent = child
            info = os.fstat(parent)
            sticky_root = info.st_uid == 0 and bool(info.st_mode & stat.S_ISVTX)
            if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or (info.st_mode & 0o022 and not sticky_root):
                raise ValueError("protected input parent is writable or unowned")
        descriptor = os.open(path.name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=parent)
        info = os.fstat(descriptor)
        if (
            not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_nlink != 1
            or info.st_mode & 0o022
            or (exact_mode is not None and stat.S_IMODE(info.st_mode) != exact_mode)
        ):
            raise ValueError("protected input is not an exact root-owned regular file")
        chunks: list[bytes] = []
        while chunk := os.read(descriptor, 1 << 20):
            chunks.append(chunk)
        after = os.fstat(descriptor)
        if (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns, after.st_ctime_ns) != (
            info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns
        ):
            raise ValueError("protected input changed while being read")
        return b"".join(chunks)
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        os.close(parent)


def protected_file(root: Path, relative: str, *, mode: int) -> tuple[Path, bytes]:
    if not isinstance(relative, str) or not relative or Path(relative).is_absolute() or ".." in Path(relative).parts:
        raise ValueError("native config path is not profile-relative")
    root = root.resolve(strict=True)
    path = root.joinpath(relative)
    if path.is_symlink() or path.resolve(strict=True) != path or root not in path.parents:
        raise ValueError("native config path escapes the profile seed")
    return path, read_protected(path, exact_mode=mode)


def protected_file_at(root_fd: int, relative: str, *, mode: int) -> bytes:
    """Read a profile-relative file through the already authenticated seed."""
    path = Path(relative) if isinstance(relative, str) else Path()
    if not relative or path.is_absolute() or ".." in path.parts or any(part in {"", "."} for part in path.parts):
        raise ValueError("native config path is not profile-relative")
    parent = os.dup(root_fd)
    descriptor = -1
    try:
        for component in path.parts[:-1]:
            child = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=parent)
            os.close(parent)
            parent = child
            info = os.fstat(parent)
            if info.st_uid != 0 or info.st_mode & 0o022:
                raise ValueError("native config parent is not protected")
        descriptor = os.open(path.name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=parent)
        before = os.fstat(descriptor)
        if (
            not stat.S_ISREG(before.st_mode) or before.st_uid != 0 or before.st_nlink != 1
            or stat.S_IMODE(before.st_mode) != mode
        ):
            raise ValueError("native config is not an exact protected regular file")
        if before.st_size < 0 or before.st_size > MAX_NATIVE_CONFIG_BYTES:
            raise ValueError("native config exceeds the 4 MiB provisioning bound")
        body = bytearray()
        while len(body) <= MAX_NATIVE_CONFIG_BYTES:
            chunk = os.read(
                descriptor,
                min(1 << 20, MAX_NATIVE_CONFIG_BYTES + 1 - len(body)),
            )
            if not chunk:
                break
            body.extend(chunk)
        after = os.fstat(descriptor)
        if (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns, after.st_ctime_ns) != (
            before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns, before.st_ctime_ns
        ):
            raise ValueError("native config changed while being read")
        if len(body) != before.st_size or len(body) > MAX_NATIVE_CONFIG_BYTES:
            raise ValueError("native config changed or exceeds the 4 MiB provisioning bound")
        return bytes(body)
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        os.close(parent)


def load_object(path: Path) -> tuple[dict[str, Any], bytes]:
    body = read_protected(path)
    value = strict_json_loads(body)
    if not isinstance(value, dict):
        raise ValueError(f"{path}: expected a JSON object")
    return value, body


REVISION_ALIASES = {"revision", "package_revision", "source_revision", "resolved_revision"}
SOURCE_AUTHORITY_FIELDS = REVISION_ALIASES | {
    "source", "source_repository", "source_path", "tuple",
}
SOURCE_AUTHORITY_KEY_BY_TOKEN = {
    re.sub(r"[_\s-]", "", key).casefold(): key for key in SOURCE_AUTHORITY_FIELDS
}
SOURCE_OBJECT_KEY_BY_TOKEN = {
    "repository": "repository", "revision": "revision", "path": "path",
}
IDENTITY_KEY_BY_TOKEN = {
    re.sub(r"[_\s-]", "", key).casefold(): key
    for key in ("plugin", "product_id", "name")
}

TUPLE_FIELDS = {
    "product_id", "tree_digest", "manifest_digest", "distribution_id", "distribution_kind",
    "release_sequence", "package_version", "source_repository", "source_revision", "source_path",
    "snapshot_sequence", "snapshot_digest", "binary_digest", "dependency_identity", "installer_version",
    "adapter_version", "client_version", "os", "architecture", "observed_at",
}
DIGEST_FIELDS = {"tree_digest", "manifest_digest", "snapshot_digest", "binary_digest"}
SEQUENCE_FIELDS = {"release_sequence", "snapshot_sequence"}


def source_authority_name(key: Any, *, in_source: bool = False) -> str | None:
    """Recognize only the finite source-authority vocabulary across separators/case."""
    if not isinstance(key, str):
        return None
    token = re.sub(r"[_\s-]", "", key).casefold()
    authority = SOURCE_AUTHORITY_KEY_BY_TOKEN.get(token)
    if authority is not None:
        return authority
    return SOURCE_OBJECT_KEY_BY_TOKEN.get(token) if in_source else None


def exact_equal(left: Any, right: Any) -> bool:
    """Compare decoded evidence without Python's bool/int/float coercions."""
    if type(left) is not type(right):
        return False
    if isinstance(left, dict):
        return left.keys() == right.keys() and all(exact_equal(left[key], right[key]) for key in left)
    if isinstance(left, list):
        return len(left) == len(right) and all(exact_equal(a, b) for a, b in zip(left, right))
    return left == right


def validate_approved_tuple(value: Any, plugin: str) -> None:
    if not isinstance(value, dict) or set(value) != TUPLE_FIELDS or value.get("product_id") != plugin:
        raise ValueError(f"{plugin}: approved source tuple is not exact")
    if any(type(value.get(field)) is not int or value[field] < 1 for field in SEQUENCE_FIELDS):
        raise ValueError(f"{plugin}: approved release sequence is invalid")
    string_fields = TUPLE_FIELDS - SEQUENCE_FIELDS - {"client_version"}
    if (
        any(type(value.get(field)) is not str or not value[field] for field in string_fields)
        or value.get("client_version") is not None
    ):
        raise ValueError(f"{plugin}: approved source tuple contains a malformed identifier")
    if (
        re.fullmatch(r"[a-f0-9]{40}", value["source_revision"]) is None
        or any(re.fullmatch(r"sha256:[a-f0-9]{64}", value[field]) is None for field in DIGEST_FIELDS)
        or "//" in value["source_repository"] or value["source_repository"].startswith("/")
        or Path(value["source_path"]).is_absolute() or ".." in Path(value["source_path"]).parts
    ):
        raise ValueError(f"{plugin}: approved source tuple is malformed")


def reconcile_revision_aliases(
    value: Any, approved: dict[str, Any], context: str, *, recursive: bool = True,
    in_source: bool = False,
) -> None:
    """Recursively reconcile every source authority against one approved tuple."""
    expected = approved.get("source_revision")
    if not isinstance(expected, str) or re.fullmatch(r"[a-f0-9]{40}", expected) is None:
        raise ValueError(f"{context}: approved revision is invalid")
    package_fields = {
        "version": "package_version", "resolved_revision": "source_revision",
        "tree_digest": "tree_digest", "manifest_digest": "manifest_digest",
    }

    expected_source = f'{approved.get("source_repository")}//{approved.get("source_path")}'
    source_object_fields = {
        "repository": "source_repository", "revision": "source_revision", "path": "source_path",
    }

    def visit(item: Any, *, source_object: bool = False) -> None:
        if isinstance(item, list):
            if recursive:
                for child in item:
                    visit(child, source_object=source_object)
            return
        if not isinstance(item, dict):
            return
        for key, child in item.items():
            normalized = source_authority_name(key, in_source=source_object)
            if normalized is not None and key != normalized:
                label = "revision" if normalized in REVISION_ALIASES or normalized == "revision" else "source"
                raise ValueError(f"{context}: unexpected {label} authority {key}")
            if key == "source":
                if isinstance(child, str):
                    if child != expected_source:
                        raise ValueError(f"{context}: conflicting source authority (source tuple)")
                elif isinstance(child, dict):
                    if (
                        set(child) != set(source_object_fields)
                        or any(not exact_equal(child.get(source), approved.get(target)) for source, target in source_object_fields.items())
                    ):
                        raise ValueError(f"{context}: malformed or conflicting source authority")
                else:
                    raise ValueError(f"{context}: malformed source authority")
            if key in {"source_repository", "source_path"}:
                if not exact_equal(child, approved.get(key)):
                    raise ValueError(f"{context}: conflicting source authority {key} (source tuple)")
            if source_object and key in source_object_fields and not exact_equal(child, approved.get(source_object_fields[key])):
                raise ValueError(f"{context}: conflicting source authority {key} (source tuple)")
            if key in {"revision", "source_revision", "resolved_revision"} and not exact_equal(child, expected):
                raise ValueError(f"{context}: conflicting revision alias {key} (source tuple)")
            if key == "package_revision":
                if (
                    not isinstance(child, dict) or set(child) != set(package_fields)
                    or any(not exact_equal(child.get(source), approved.get(target)) for source, target in package_fields.items())
                ):
                    raise ValueError(f"{context}: conflicting or unexpected package_revision authority")
            if key == "tuple" and not exact_equal(child, approved):
                raise ValueError(f"{context}: conflicting or malformed source tuple")
            if recursive:
                visit(child, source_object=(key == "source" and isinstance(child, dict)))

    visit(value, source_object=in_source)


def matching_client(info: dict[str, Any], plugin: str, client: str, approved: dict[str, Any]) -> None:
    if set(info) != {"schema_version", "command", "result", "data"} or type(info.get("schema_version")) is not int or info.get("schema_version") != 1 or info.get("command") != "info" or info.get("result") != "success":
        raise ValueError(f"{plugin}: manager info is not exact successful agentplugins JSON")
    data = info.get("data")
    clients = data.get("clients") if isinstance(data, dict) else None
    if not isinstance(data, dict) or data.get("name") != plugin or not isinstance(clients, list):
        raise ValueError(f"{plugin}: manager info identity differs")
    expected_source = f'{approved.get("source_repository")}//{approved.get("source_path")}'
    if data.get("source") != expected_source:
        raise ValueError(f"{plugin}: manager info source differs from the approved package source")
    matches = [item for item in clients if isinstance(item, dict) and item.get("client_id") == client]
    if len(clients) != 1 or len(matches) != 1:
        raise ValueError(f"{plugin}: manager info does not contain exactly one requested client")
    record = matches[0]
    completed = {
        "scope": "user", "materialization": "materialized", "activation": "active",
        "verification": "installation_verified", "policy": "allowed",
        "receipt_reconciled": True, "native_discovery_reconciled": True,
        "native_identity_state": "managed",
    }
    lifecycle_differs = any(
        (record.get(key) is not expected) if isinstance(expected, bool) else (record.get(key) != expected)
        for key, expected in completed.items()
    )
    if data.get("mixed_version") is not False or lifecycle_differs:
        raise ValueError(f"{plugin}: manager info lifecycle is incomplete or unreconciled")
    if prohibited_lifecycle_state(data):
        raise ValueError(f"{plugin}: manager info contains a prohibited lifecycle state")
    revision = record.get("package_revision")
    fields = {
        "version": "package_version", "resolved_revision": "source_revision",
        "tree_digest": "tree_digest", "manifest_digest": "manifest_digest",
    }
    if (
        not isinstance(revision, dict) or set(revision) != set(fields)
        or any(not exact_equal(revision.get(source), approved.get(target)) for source, target in fields.items())
        or not isinstance(revision.get("resolved_revision"), str)
        or len(revision["resolved_revision"]) != 40
        or any(character not in "0123456789abcdef" for character in revision["resolved_revision"])
    ):
        raise ValueError(f"{plugin}: manager info does not bind the approved release tuple")
    reconcile_revision_aliases(info, approved, f"{plugin}: manager info")


def matching_add(value: dict[str, Any], plugin: str, client: str, approved: dict[str, Any]) -> None:
    """Validate the real agentplugins 0.1.16 add envelope and approved source."""
    if set(value) != {"schema_version", "command", "result", "data"} or type(value.get("schema_version")) is not int or value.get("schema_version") != 1 or value.get("command") != "add" or value.get("result") != "success":
        raise ValueError(f"{plugin}: manager add is not successful agentplugins JSON")
    data = value.get("data")
    expected_source = f'{approved.get("source_repository")}//{approved.get("source_path")}'
    expected_revision = approved.get("source_revision")
    if (
        not isinstance(data, dict) or data.get("status") != "completed"
        or data.get("plugin") != plugin or data.get("source") != expected_source
        or data.get("revision") != expected_revision
        or type(data.get("failed")) is not int or data.get("failed") != 0
        or not isinstance(expected_revision, str) or len(expected_revision) != 40
        or any(character not in "0123456789abcdef" for character in expected_revision)
    ):
        raise ValueError(f"{plugin}: manager add does not bind the approved canonical source")
    targets = data.get("targets")
    if not isinstance(targets, list) or len(targets) != 1 or not isinstance(targets[0], dict):
        raise ValueError(f"{plugin}: manager add does not contain exactly the requested target")
    matches = [item for item in targets if item.get("target") == client]
    if len(matches) != 1 or matches[0].get("status") not in ADD_SUCCESS_STATUSES:
        raise ValueError(f"{plugin}: manager add target did not complete successfully")
    bindings = {
        "source": expected_source, "revision": expected_revision,
        "version": approved.get("package_version"), "tree_digest": approved.get("tree_digest"),
        "manifest_digest": approved.get("manifest_digest"),
    }
    def reject_conflict(item: Any) -> bool:
        if isinstance(item, list):
            return any(reject_conflict(child) for child in item)
        if not isinstance(item, dict):
            return False
        return any(key in item and not exact_equal(item[key], expected) for key, expected in bindings.items()) or any(reject_conflict(child) for child in item.values())
    if reject_conflict(data):
        raise ValueError(f"{plugin}: manager add contains a conflicting approved source tuple")
    reconcile_revision_aliases(value, approved, f"{plugin}: manager add")
    if prohibited_lifecycle_state(value):
        raise ValueError(f"{plugin}: manager add contains an incomplete or prohibited lifecycle state")


def matching_doctor(value: dict[str, Any], client: str, approved: dict[str, dict[str, Any]]) -> None:
    """Validate one complete post-add 0.1.16 doctor inventory."""
    if set(value) != {"schema_version", "command", "result", "data"} or type(value.get("schema_version")) is not int or value.get("schema_version") != 1 or value.get("command") != "doctor" or value.get("result") != "success":
        raise ValueError("post-add doctor is not the exact successful 0.1.16 envelope")
    if prohibited_lifecycle_state(value):
        raise ValueError("post-add doctor contains an incomplete or prohibited lifecycle state")
    detected: list[str] = []
    inventory: list[tuple[str, dict[str, Any]]] = []

    def visit(
        item: Any, inherited: str | None = None, *, in_source: bool = False,
        inventory_candidate: bool = True,
    ) -> None:
        if isinstance(item, list):
            for child in item:
                visit(child, inherited, in_source=in_source, inventory_candidate=inventory_candidate)
            return
        if not isinstance(item, dict):
            return
        identity = item.get("client_id")
        if type(identity) is str and (item.get("detected") is True or item.get("status") == "detected"):
            detected.append(identity)
        identities: list[str] = []
        for key, child in item.items():
            token = re.sub(r"[_\s-]", "", key).casefold() if isinstance(key, str) else ""
            canonical = IDENTITY_KEY_BY_TOKEN.get(token)
            if canonical is not None:
                if key != canonical or type(child) is not str or child not in HEROES:
                    raise ValueError("post-add doctor contains an unexpected or unattributed hero identity")
                identities.append(child)
        if identities and (len(set(identities)) != 1 or (inherited is not None and identities[0] != inherited)):
            raise ValueError("post-add doctor contains conflicting hero identity")
        local = identities[0] if identities else inherited
        authority = any(source_authority_name(key, in_source=in_source) is not None for key in item)
        if authority:
            if local is None:
                raise ValueError("post-add doctor contains unattributed source authority")
            reconcile_revision_aliases(
                item, approved[local], f"{local}: post-add doctor",
                recursive=False, in_source=in_source,
            )
            if identities and not in_source and inventory_candidate:
                inventory.append((local, item))
        for key, child in item.items():
            visit(
                child, local, in_source=(key == "source" and isinstance(child, dict)),
                inventory_candidate=(key != "tuple"),
            )

    visit(value["data"])
    names = {plugin for plugin, _item in inventory}
    if detected != [client]:
        raise ValueError("post-add doctor did not detect exactly the intended client")
    if len(inventory) != len(HEROES) or names != HEROES:
        raise ValueError("post-add doctor does not inventory exactly the five intended heroes")
    for plugin, item in inventory:
        expected = approved[plugin]
        source = f'{expected.get("source_repository")}//{expected.get("source_path")}'
        revision = expected.get("source_revision")
        item_tuple = item.get("tuple")
        if isinstance(item_tuple, dict):
            if not exact_equal(item_tuple, expected):
                raise ValueError(f"{plugin}: post-add doctor tuple differs")
        elif item.get("source") != source or not any(item.get(key) == revision for key in ("revision", "source_revision")):
            raise ValueError(f"{plugin}: post-add doctor source tuple differs")
        if "source" in item and item["source"] != source:
            raise ValueError(f"{plugin}: post-add doctor source tuple differs")
        if any(key in item and not exact_equal(item[key], revision) for key in ("revision", "source_revision")):
            raise ValueError(f"{plugin}: post-add doctor source tuple differs")
        reconcile_revision_aliases(item, expected, f"{plugin}: post-add doctor")


def seal_failpoint(name: str) -> None:
    ready = os.environ.get("UAP_OBSERVER_SEAL_TEST_READY_FD")
    resume = os.environ.get("UAP_OBSERVER_SEAL_TEST_RESUME_FD")
    if (ready is None) != (resume is None):
        raise ValueError("incomplete profile sealing race synchronization")
    if ready is not None and os.environ.get("UAP_OBSERVER_SEAL_TEST_RACE_POINT") == name:
        os.write(int(ready), b"1")
        if os.read(int(resume), 1) != b"1":
            raise OSError("profile sealing race synchronization closed")
    if os.environ.get("UAP_OBSERVER_SEAL_FAILPOINT") == name:
        raise OSError(f"profile sealing failpoint: {name}")


def remove_staged_proof(seed_fd: int, name: str) -> None:
    """Remove only the unpublished directory created by this transaction."""
    stage_fd = os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=seed_fd)
    try:
        try:
            native_fd = os.open("native", os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=stage_fd)
        except FileNotFoundError:
            native_fd = -1
        if native_fd >= 0:
            try:
                for plugin in HEROES:
                    try:
                        os.unlink(f"{plugin}.json", dir_fd=native_fd)
                    except FileNotFoundError:
                        pass
            finally:
                os.close(native_fd)
            os.rmdir("native", dir_fd=stage_fd)
        for child in ("receipts.json", "native-projection.json"):
            try:
                os.unlink(child, dir_fd=stage_fd)
            except FileNotFoundError:
                pass
    finally:
        os.close(stage_fd)
    os.rmdir(name, dir_fd=seed_fd)
    os.fsync(seed_fd)


def publish_noreplace(seed_fd: int, staged: str, final: str) -> None:
    """Linux renameat2 gives atomic publication without replacement races."""
    libc = ctypes.CDLL(None, use_errno=True)
    renameat2 = getattr(libc, "renameat2", None)
    if renameat2 is None:
        raise OSError("renameat2 is required for failure-atomic profile sealing")
    renameat2.argtypes = (ctypes.c_int, ctypes.c_char_p, ctypes.c_int, ctypes.c_char_p, ctypes.c_uint)
    renameat2.restype = ctypes.c_int
    if renameat2(seed_fd, os.fsencode(staged), seed_fd, os.fsencode(final), 1) != 0:  # RENAME_NOREPLACE
        error = ctypes.get_errno()
        raise OSError(error, os.strerror(error), final)


def write_sealed_file(parent_fd: int, name: str, body: bytes, mode: int) -> None:
    descriptor = os.open(
        name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC,
        mode, dir_fd=parent_fd,
    )
    try:
        os.fchmod(descriptor, mode)
        view = memoryview(body)
        while view:
            written = os.write(descriptor, view)
            if written <= 0:
                raise OSError("short write while sealing proof")
            view = view[written:]
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--client", choices=sorted(CLIENTS), required=True)
    parser.add_argument("--root-owned-seed", type=Path, required=True)
    parser.add_argument("--matrix-file", type=Path, required=True)
    parser.add_argument("--adapter-config", type=Path)
    parser.add_argument("--manager-add-directory", type=Path, required=True)
    parser.add_argument("--manager-info-directory", type=Path, required=True)
    parser.add_argument("--post-doctor-directory", type=Path, required=True)
    parser.add_argument("--native-config-map", type=Path, required=True)
    parser.add_argument("--digest-only", action="store_true")
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise SystemExit("profile sealing requires root")
    supplied_seed = os.lstat(args.root_owned_seed)
    if stat.S_ISLNK(supplied_seed.st_mode):
        raise ValueError("profile seed pathname must not be a symbolic link")
    seed_fd = os.open(args.root_owned_seed, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
    seed_info = os.fstat(seed_fd)
    if (
        not stat.S_ISDIR(seed_info.st_mode) or seed_info.st_uid != 0
        or seed_info.st_mode & 0o022
        or (supplied_seed.st_dev, supplied_seed.st_ino) != (seed_info.st_dev, seed_info.st_ino)
    ):
        os.close(seed_fd)
        raise ValueError("profile seed is not a protected root-owned directory")
    matrix_input, _ = load_object(args.matrix_file)
    mapping, _ = load_object(args.native_config_map)
    matrix = matrix_input.get("matrix")
    selected = [item for item in matrix or [] if isinstance(item, dict) and item.get("client") == args.client]
    approved = {item.get("plugin"): item.get("tuple") for item in selected}
    if len(selected) != len(HEROES) or set(approved) != HEROES or set(mapping) != HEROES or any(not isinstance(value, dict) for value in approved.values()):
        raise ValueError("adapter tuple or native config map omits a hero")
    for plugin, source_tuple in approved.items():
        validate_approved_tuple(source_tuple, plugin)
    entries, receipts = [], []
    native_bodies: dict[str, bytes] = {}
    final_root = Path("/var/lib/uap-observer/profiles") / args.client
    for plugin in sorted(HEROES):
        add, add_body = load_object(args.manager_add_directory / f"{plugin}.json")
        matching_add(add, plugin, args.client, approved[plugin])
        info, info_body = load_object(args.manager_info_directory / f"{plugin}.json")
        matching_client(info, plugin, args.client, approved[plugin])
        relative = mapping[plugin]
        native_body = protected_file_at(seed_fd, relative, mode=0o600)
        native_value = strict_json_loads(native_body)
        if not isinstance(native_value, dict):
            raise ValueError(f"{plugin}: native config must be a JSON object")
        reconcile_revision_aliases(native_value, approved[plugin], f"{plugin}: native config")
        native_bodies[plugin] = native_body
        native_digest = digest(native_body)
        native = {"path": f"/var/lib/uap-observer/proofs/{args.client}/native/{plugin}.json", "sha256": native_digest}
        client_config = {"path": str(final_root / relative), "sha256": native_digest}
        evidence = {
            "manager_add_sha256": digest(add_body),
            "manager_info_sha256": digest(info_body),
        }
        entries.append({"plugin": plugin, "tuple": approved[plugin], "native_config": native, "client_config": client_config, **evidence})
        receipts.append({"name": plugin, "tuple": approved[plugin], **evidence})
    for plugin in sorted(HEROES):
        doctor, doctor_body = load_object(args.post_doctor_directory / f"{plugin}.json")
        matching_doctor(doctor, args.client, approved)
        doctor_digest = digest(doctor_body)
        entry = next(item for item in entries if item["plugin"] == plugin)
        receipt = next(item for item in receipts if item["name"] == plugin)
        entry["post_add_doctor_sha256"] = doctor_digest
        receipt["post_add_doctor_sha256"] = doctor_digest
    receipt_value = {"schema_version": 1, "receipts": receipts}
    projection_value = {"schema_version": 1, "client_id": args.client, "entries": entries}
    projection_digest = digest(canonical(projection_value))
    if args.digest_only:
        if args.adapter_config is not None:
            raise ValueError("digest bootstrap must not consume the not-yet-final adapter config")
        print(projection_digest)
        os.close(seed_fd)
        return 0
    if args.adapter_config is None:
        raise ValueError("final sealing requires the frozen adapter config")
    config, _ = load_object(args.adapter_config)
    projection_contract = config.get("clients", {}).get(args.client, {}).get("native_projection")
    if config.get("matrix") != matrix or projection_contract != {
        "path": f"/var/lib/uap-observer/proofs/{args.client}/native-projection.json",
        "sha256": projection_digest,
    }:
        raise ValueError("final adapter config does not bind the immutable matrix and projection digest")
    # Pin the resolved seed once and perform every publication operation through
    # that descriptor.  The user-supplied pathname is evidence only; it is
    # revalidated immediately before the atomic rename and never used to write.
    staged_name: str | None = None
    published = False
    stage_fd = -1
    native_fd = -1
    outputs = [
        ("receipts.json", canonical(receipt_value), 0o400),
        ("native-projection.json", canonical(projection_value), 0o400),
    ]
    try:
        staged_path = Path(tempfile.mkdtemp(prefix=".uap-observer-proof.stage.", dir=f"/proc/self/fd/{seed_fd}"))
        staged_name = staged_path.name
        stage_fd = os.open(staged_name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=seed_fd)
        stage_info = os.fstat(stage_fd)
        if stage_info.st_uid != 0 or stat.S_IMODE(stage_info.st_mode) != 0o700:
            raise ValueError("staged profile proof directory is not protected")
        os.mkdir("native", 0o700, dir_fd=stage_fd)
        native_fd = os.open("native", os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=stage_fd)
        for name, body, mode in outputs:
            write_sealed_file(stage_fd, name, body, mode)
        for plugin, native_body in sorted(native_bodies.items()):
            write_sealed_file(native_fd, f"{plugin}.json", native_body, 0o400)
        seal_failpoint("after-staging")
        os.fsync(native_fd)
        os.fsync(stage_fd)
        seal_failpoint("before-publication")
        current = os.lstat(args.root_owned_seed)
        if stat.S_ISLNK(current.st_mode) or (current.st_dev, current.st_ino) != (seed_info.st_dev, seed_info.st_ino):
            raise ValueError("profile seed pathname changed before publication")
        publish_noreplace(seed_fd, staged_name, ".uap-observer-proof")
        published = True
        os.fsync(seed_fd)
    except Exception:
        if published and staged_name is not None:
            # An ordinary post-rename durability failure must restore the
            # pre-transaction absence before reporting failure.
            os.rename(
                ".uap-observer-proof", staged_name,
                src_dir_fd=seed_fd, dst_dir_fd=seed_fd,
            )
            published = False
        raise
    finally:
        if native_fd >= 0:
            os.close(native_fd)
        if stage_fd >= 0:
            os.close(stage_fd)
        if not published and staged_name is not None:
            try:
                remove_staged_proof(seed_fd, staged_name)
            except FileNotFoundError:
                pass
        os.close(seed_fd)
    print(projection_digest)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
