#!/usr/bin/env python3
"""Reconstruct one sealed profile seed from authenticated observer recovery data."""

from __future__ import annotations

import argparse
import ctypes
import hashlib
import importlib.util
import os
import re
import secrets
import stat
from pathlib import Path


def _load_provisioner():
    path = Path(__file__).with_name("uap-observer-provision-profile.py")
    spec = importlib.util.spec_from_file_location("uap_observer_provision_profile_recovery_dependency", path)
    if spec is None or spec.loader is None:
        raise RuntimeError("installed profile provisioner could not be loaded")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


PROVISIONER = _load_provisioner()
CLIENTS = PROVISIONER.CLIENTS
HEROES = PROVISIONER.HEROES
MAX_FILES = PROVISIONER.MAX_FILES
MAX_BYTES = PROVISIONER.MAX_BYTES
MAX_DEPTH = 128
PROOF_SEED_NAME = PROVISIONER.PROOF_SEED_NAME
OPEN_DIRECTORY = PROVISIONER.OPEN_DIRECTORY
ADAPTER_CONFIG_FIELDS = {
    "schema_version", "request_policy", "git", "clients", "matrix", "consent_record",
    "chatgpt", "chrome_for_testing", "workspace_root", "external_pr_evidence", "egress_hosts",
}
MATRIX_FIELDS = {"plugin", "client", "tuple", "application_id", "endpoint"}
CLIENT_BASE_FIELDS = {"binary", "sha256", "profile", "client_id", "native_projection"}


def checkpoint(_name: str) -> None:
    """Failpoint seam for durability tests."""


def _identity(info: os.stat_result) -> tuple[int, ...]:
    return (info.st_dev, info.st_ino, info.st_mode, info.st_nlink, info.st_uid,
            info.st_gid, info.st_size, info.st_mtime_ns, info.st_ctime_ns)


def open_protected_directory(path: Path, *, owner_uid: int = 0) -> int:
    if not path.is_absolute() or ".." in path.parts:
        raise ValueError("recovery directory path is invalid")
    descriptor = os.open("/", os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for component in path.parts[1:]:
            child = os.open(component, OPEN_DIRECTORY, dir_fd=descriptor)
            os.close(descriptor)
            descriptor = child
            info = os.fstat(descriptor)
            if not stat.S_ISDIR(info.st_mode) or info.st_uid != owner_uid or info.st_mode & 0o022:
                raise ValueError("recovery directory is not protected and root-owned")
        return descriptor
    except Exception:
        os.close(descriptor)
        raise


def checked_entry(parent_fd: int, name: str, owner_uid: int) -> os.stat_result:
    info = os.stat(name, dir_fd=parent_fd, follow_symlinks=False)
    if not (stat.S_ISREG(info.st_mode) or stat.S_ISDIR(info.st_mode)):
        raise ValueError("recovery source contains a link or special file")
    if info.st_uid != owner_uid or info.st_mode & 0o022:
        raise ValueError("recovery source entry is not protected and root-owned")
    if stat.S_ISREG(info.st_mode) and info.st_nlink != 1:
        raise ValueError("recovery source contains a hardlinked file")
    return info


def copy_tree(
    source_fd: int, destination_fd: int, *, owner_uid: int = 0, owner_gid: int = 0,
    limits: tuple[int, int] = (MAX_FILES, MAX_BYTES), counters: list[int] | None = None,
    depth: int = 0,
) -> tuple[int, int]:
    if depth > MAX_DEPTH:
        raise ValueError("recovery source exceeds directory-depth bound")
    counters = counters if counters is not None else [0, 0]
    before = os.fstat(source_fd)
    for name in sorted(os.listdir(source_fd)):
        if name in {"", ".", ".."} or "/" in name or "\x00" in name:
            raise ValueError("recovery source entry name is invalid")
        info = checked_entry(source_fd, name, owner_uid)
        counters[0] += 1
        if counters[0] > limits[0]:
            raise ValueError("recovery source exceeds file-count bound")
        if stat.S_ISDIR(info.st_mode):
            source_child = os.open(name, OPEN_DIRECTORY, dir_fd=source_fd)
            destination_child = -1
            try:
                if _identity(os.fstat(source_child)) != _identity(info):
                    raise ValueError("recovery source directory was substituted")
                os.mkdir(name, 0o700, dir_fd=destination_fd)
                destination_child = os.open(name, OPEN_DIRECTORY, dir_fd=destination_fd)
                os.fchown(destination_child, owner_uid, owner_gid)
                os.fchmod(destination_child, 0o700)
                copy_tree(source_child, destination_child, owner_uid=owner_uid, owner_gid=owner_gid,
                          limits=limits, counters=counters, depth=depth + 1)
                if _identity(os.fstat(source_child)) != _identity(info) or _identity(
                    os.stat(name, dir_fd=source_fd, follow_symlinks=False)
                ) != _identity(info):
                    raise ValueError("recovery source directory changed during copy")
                os.fsync(destination_child)
            finally:
                if destination_child >= 0:
                    os.close(destination_child)
                os.close(source_child)
            continue
        source = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=source_fd)
        output = -1
        try:
            current = os.fstat(source)
            if _identity(current) != _identity(info):
                raise ValueError("recovery source file was substituted")
            counters[1] += current.st_size
            if counters[1] > limits[1]:
                raise ValueError("recovery source exceeds byte bound")
            output = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW |
                             os.O_CLOEXEC, 0o600, dir_fd=destination_fd)
            os.fchown(output, owner_uid, owner_gid)
            os.fchmod(output, 0o600)
            copied = 0
            while chunk := os.read(source, 1 << 20):
                copied += len(chunk)
                view = memoryview(chunk)
                while view:
                    written = os.write(output, view)
                    if written <= 0:
                        raise OSError("recovery copy made no progress")
                    view = view[written:]
            if copied != current.st_size or _identity(os.fstat(source)) != _identity(current) or _identity(
                os.stat(name, dir_fd=source_fd, follow_symlinks=False)
            ) != _identity(current):
                raise ValueError("recovery source file changed during copy")
            os.fsync(output)
        finally:
            if output >= 0:
                os.close(output)
            os.close(source)
    if _identity(os.fstat(source_fd)) != _identity(before):
        raise ValueError("recovery source directory changed during copy")
    return counters[0], counters[1]


def validate_proof_inventory(proof_fd: int, owner_uid: int = 0) -> None:
    if set(os.listdir(proof_fd)) != {"receipts.json", "native-projection.json", "native"}:
        raise ValueError("archived proof inventory differs")
    for name in ("receipts.json", "native-projection.json"):
        if not stat.S_ISREG(checked_entry(proof_fd, name, owner_uid).st_mode):
            raise ValueError("archived proof inventory differs")
    if not stat.S_ISDIR(checked_entry(proof_fd, "native", owner_uid).st_mode):
        raise ValueError("archived native proof inventory differs")
    native_fd = os.open("native", OPEN_DIRECTORY, dir_fd=proof_fd)
    try:
        if set(os.listdir(native_fd)) != {f"{plugin}.blob" for plugin in HEROES}:
            raise ValueError("archived native proof inventory differs")
        for name in os.listdir(native_fd):
            if not stat.S_ISREG(checked_entry(native_fd, name, owner_uid).st_mode):
                raise ValueError("archived native proof inventory differs")
    finally:
        os.close(native_fd)


def _read_regular(parent_fd: int, name: str, owner_uid: int, limit: int = 4 << 20) -> bytes:
    info = checked_entry(parent_fd, name, owner_uid)
    if not stat.S_ISREG(info.st_mode) or info.st_size > limit:
        raise ValueError("archived proof control file differs")
    descriptor = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=parent_fd)
    try:
        current = os.fstat(descriptor)
        body = PROVISIONER.read_bounded_regular(descriptor, current, limit)
        if _identity(current) != _identity(info) or _identity(os.fstat(descriptor)) != _identity(info) or _identity(
            os.stat(name, dir_fd=parent_fd, follow_symlinks=False)
        ) != _identity(info):
            raise ValueError("archived proof control file changed during validation")
        return body
    finally:
        os.close(descriptor)


def read_protected_regular(path: Path, *, limit: int = 16 << 20) -> bytes:
    if not path.is_absolute() or ".." in path.parts or not path.name:
        raise ValueError("protected recovery input path is invalid")
    parent_fd = open_protected_directory(path.parent)
    descriptor = -1
    try:
        descriptor = os.open(path.name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=parent_fd)
        before = os.fstat(descriptor)
        if (
            not stat.S_ISREG(before.st_mode) or before.st_uid != 0 or before.st_nlink != 1
            or before.st_mode & 0o022 or before.st_size < 0 or before.st_size > limit
        ):
            raise ValueError("adapter config is not a protected root-owned regular file")
        body = PROVISIONER.read_bounded_regular(descriptor, before, limit)
        if _identity(os.fstat(descriptor)) != _identity(before) or _identity(
            os.stat(path.name, dir_fd=parent_fd, follow_symlinks=False)
        ) != _identity(before):
            raise ValueError("adapter config changed while being read")
        return body
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        os.close(parent_fd)


def validate_adapter_contract(config_body: bytes, projection_body: bytes, entries: list[dict], client: str) -> None:
    config = PROVISIONER.strict_json_loads(config_body)
    clients = config.get("clients") if isinstance(config, dict) else None
    matrix = config.get("matrix") if isinstance(config, dict) else None
    if (
        not isinstance(config, dict) or set(config) != ADAPTER_CONFIG_FIELDS
        or type(config.get("schema_version")) is not int or config["schema_version"] != 1
        or not isinstance(clients, dict) or set(clients) != CLIENTS
        or not isinstance(matrix, list) or len(matrix) != len(CLIENTS) * len(HEROES)
    ):
        raise ValueError("adapter config recovery contract is invalid")
    record = clients.get(client)
    projection = record.get("native_projection") if isinstance(record, dict) else None
    expected_path = f"/var/lib/uap-observer/proofs/{client}/native-projection.json"
    expected_profile = f"/var/lib/uap-observer/profiles/{client}"
    expected_digest = "sha256:" + hashlib.sha256(projection_body).hexdigest()
    expected_record_fields = CLIENT_BASE_FIELDS | (
        {"bundle"} if client == "cursor" else {"companion_binary", "companion_sha256"}
    )
    if (
        not isinstance(record, dict) or set(record) != expected_record_fields
        or record.get("client_id") != client or record.get("profile") != expected_profile
        or type(record.get("binary")) is not str or not record["binary"].startswith("/opt/uap-observer-inputs/")
        or type(record.get("sha256")) is not str or re.fullmatch(r"sha256:[a-f0-9]{64}", record["sha256"]) is None
        or not isinstance(projection, dict) or set(projection) != {"path", "sha256"}
        or projection != {"path": expected_path, "sha256": expected_digest}
    ):
        raise ValueError("adapter config native projection binding differs")
    approved: dict[tuple[str, str], dict] = {}
    for row in matrix:
        if (
            not isinstance(row, dict) or set(row) != MATRIX_FIELDS
            or row.get("client") not in CLIENTS or row.get("plugin") not in HEROES
            or type(row.get("application_id")) is not str or not row["application_id"]
            or type(row.get("endpoint")) is not str or not row["endpoint"].startswith("https://")
            or not isinstance(row.get("tuple"), dict)
        ):
            raise ValueError("adapter config matrix is invalid")
        key = (row["client"], row["plugin"])
        if key in approved:
            raise ValueError("adapter config matrix repeats a client-plugin tuple")
        PROVISIONER.validate_release_tuple(row["tuple"], row["plugin"])
        approved[key] = row["tuple"]
    expected_pairs = {(candidate, plugin) for candidate in CLIENTS for plugin in HEROES}
    if set(approved) != expected_pairs:
        raise ValueError("adapter config matrix is incomplete")
    archived = {entry["plugin"]: entry["tuple"] for entry in entries}
    if set(archived) != HEROES or any(archived[plugin] != approved[(client, plugin)] for plugin in HEROES):
        raise ValueError("archived tuple differs from current approved adapter config")


def validate_proof_contract(
    proof_fd: int, client: str, adapter_config_body: bytes, owner_uid: int = 0,
) -> None:
    projection_body = _read_regular(proof_fd, "native-projection.json", owner_uid)
    projection = PROVISIONER.strict_json_loads(projection_body)
    receipts = PROVISIONER.strict_json_loads(_read_regular(proof_fd, "receipts.json", owner_uid))
    entries = PROVISIONER.validate_native_projection(projection, client)
    PROVISIONER.validate_receipts(receipts, entries)
    validate_adapter_contract(adapter_config_body, projection_body, entries, client)


def trees_equal(
    left_fd: int, right_fd: int, owner_uid: int, owner_gid: int, depth: int = 0,
) -> bool:
    """Compare two normalized root-owned trees through stable descriptors."""
    left_root = os.fstat(left_fd)
    right_root = os.fstat(right_fd)
    if (
        depth > MAX_DEPTH or left_root.st_uid != owner_uid or right_root.st_uid != owner_uid
        or left_root.st_gid != owner_gid or right_root.st_gid != owner_gid
        or stat.S_IMODE(left_root.st_mode) != 0o700 or stat.S_IMODE(right_root.st_mode) != 0o700
    ):
        return False
    left_names = sorted(os.listdir(left_fd))
    if left_names != sorted(os.listdir(right_fd)):
        return False
    for name in left_names:
        left_info = os.stat(name, dir_fd=left_fd, follow_symlinks=False)
        right_info = os.stat(name, dir_fd=right_fd, follow_symlinks=False)
        if (
            left_info.st_uid != owner_uid or right_info.st_uid != owner_uid
            or left_info.st_gid != owner_gid or right_info.st_gid != owner_gid
        ):
            return False
        if stat.S_ISDIR(left_info.st_mode) and stat.S_ISDIR(right_info.st_mode):
            if stat.S_IMODE(left_info.st_mode) != 0o700 or stat.S_IMODE(right_info.st_mode) != 0o700:
                return False
            left_child = os.open(name, OPEN_DIRECTORY, dir_fd=left_fd)
            right_child = os.open(name, OPEN_DIRECTORY, dir_fd=right_fd)
            try:
                if not trees_equal(left_child, right_child, owner_uid, owner_gid, depth + 1):
                    return False
            finally:
                os.close(right_child); os.close(left_child)
        elif stat.S_ISREG(left_info.st_mode) and stat.S_ISREG(right_info.st_mode):
            if (
                left_info.st_nlink != 1 or right_info.st_nlink != 1
                or stat.S_IMODE(left_info.st_mode) != 0o600
                or stat.S_IMODE(right_info.st_mode) != 0o600
                or left_info.st_size != right_info.st_size
            ):
                return False
            left = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=left_fd)
            right = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=right_fd)
            try:
                while True:
                    left_chunk = os.read(left, 1 << 20)
                    right_chunk = os.read(right, 1 << 20)
                    if left_chunk != right_chunk:
                        return False
                    if not left_chunk:
                        break
                if _identity(os.fstat(left)) != _identity(left_info) or _identity(os.fstat(right)) != _identity(right_info):
                    return False
            finally:
                os.close(right); os.close(left)
        else:
            return False
        if _identity(os.stat(name, dir_fd=left_fd, follow_symlinks=False)) != _identity(left_info) or _identity(
            os.stat(name, dir_fd=right_fd, follow_symlinks=False)
        ) != _identity(right_info):
            return False
    return (
        left_names == sorted(os.listdir(left_fd)) == sorted(os.listdir(right_fd))
        and _identity(os.fstat(left_fd)) == _identity(left_root)
        and _identity(os.fstat(right_fd)) == _identity(right_root)
    )


def _remove_tree(parent_fd: int, name: str) -> None:
    PROVISIONER.remove_tree(parent_fd, name)
    os.fsync(parent_fd)


def publish_noreplace(parent_fd: int, staged: str, final: str) -> None:
    """Publish atomically without allowing a raced destination replacement."""
    libc = ctypes.CDLL(None, use_errno=True)
    renameat2 = getattr(libc, "renameat2", None)
    if renameat2 is None:
        raise OSError("renameat2 is required for atomic recovery publication")
    renameat2.argtypes = (ctypes.c_int, ctypes.c_char_p, ctypes.c_int, ctypes.c_char_p,
                          ctypes.c_uint)
    renameat2.restype = ctypes.c_int
    if renameat2(parent_fd, os.fsencode(staged), parent_fd, os.fsencode(final), 1) != 0:
        error = ctypes.get_errno()
        raise OSError(error, os.strerror(error), final)


def reconstruct_seed(
    profile_fd: int, proof_fd: int, output_parent_fd: int, output_name: str, client: str,
    adapter_config_body: bytes,
    *, owner_uid: int = 0, owner_gid: int = 0,
    limits: tuple[int, int] = (MAX_FILES, MAX_BYTES),
) -> tuple[int, int, bool]:
    if output_name in {"", ".", ".."} or "/" in output_name or "\x00" in output_name:
        raise ValueError("recovery output name is invalid")
    if PROOF_SEED_NAME in os.listdir(profile_fd):
        raise ValueError("archived profile collides with reserved proof directory")
    validate_proof_inventory(proof_fd, owner_uid)
    validate_proof_contract(proof_fd, client, adapter_config_body, owner_uid)
    staging = f".{output_name}.recovery-{secrets.token_hex(8)}"
    created = False
    try:
        os.mkdir(staging, 0o700, dir_fd=output_parent_fd)
        created = True
        staging_fd = os.open(staging, OPEN_DIRECTORY, dir_fd=output_parent_fd)
        os.fchown(staging_fd, owner_uid, owner_gid)
        os.fchmod(staging_fd, 0o700)
        counters = [0, 0]
        copy_tree(profile_fd, staging_fd, owner_uid=owner_uid, owner_gid=owner_gid,
                  limits=limits, counters=counters)
        counters[0] += 1
        if counters[0] > limits[0]:
            raise ValueError("recovery source exceeds file-count bound")
        os.mkdir(PROOF_SEED_NAME, 0o700, dir_fd=staging_fd)
        proof_seed_fd = os.open(PROOF_SEED_NAME, OPEN_DIRECTORY, dir_fd=staging_fd)
        try:
            os.fchown(proof_seed_fd, owner_uid, owner_gid)
            os.fchmod(proof_seed_fd, 0o700)
            copy_tree(proof_fd, proof_seed_fd, owner_uid=owner_uid, owner_gid=owner_gid,
                      limits=limits, counters=counters)
            validate_proof_inventory(proof_seed_fd, owner_uid)
            validate_proof_contract(proof_seed_fd, client, adapter_config_body, owner_uid)
            os.fsync(proof_seed_fd)
        finally:
            os.close(proof_seed_fd)
        os.fsync(staging_fd)
        checkpoint("after_staging_fsync")
        try:
            output_fd = os.open(output_name, OPEN_DIRECTORY, dir_fd=output_parent_fd)
        except FileNotFoundError:
            output_fd = -1
        if output_fd >= 0:
            try:
                if not trees_equal(staging_fd, output_fd, owner_uid, owner_gid):
                    raise FileExistsError("recovery output exists with different content")
            finally:
                os.close(output_fd)
            os.close(staging_fd)
            staging_fd = -1
            _remove_tree(output_parent_fd, staging)
            created = False
            return counters[0], counters[1], True
        os.close(staging_fd)
        staging_fd = -1
        publish_noreplace(output_parent_fd, staging, output_name)
        created = False
        os.fsync(output_parent_fd)
        return counters[0], counters[1], False
    except Exception:
        if "staging_fd" in locals() and staging_fd >= 0:
            os.close(staging_fd)
        if created:
            _remove_tree(output_parent_fd, staging)
        raise


def _overlap(left: Path, right: Path) -> bool:
    return left == right or left in right.parents or right in left.parents


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--client", choices=sorted(CLIENTS), required=True)
    parser.add_argument("--archived-profile", type=Path, required=True)
    parser.add_argument("--archived-proof", type=Path, required=True)
    parser.add_argument("--adapter-config", type=Path, required=True)
    parser.add_argument("--output-seed", type=Path, required=True)
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise SystemExit("profile recovery requires root")
    paths = (args.archived_profile, args.archived_proof, args.adapter_config, args.output_seed)
    if any(not path.is_absolute() or ".." in path.parts for path in paths):
        raise SystemExit("profile recovery paths must be absolute and traversal-free")
    directories = (args.archived_profile, args.archived_proof, args.output_seed)
    if any(_overlap(left, right) for index, left in enumerate(directories) for right in directories[index + 1:]):
        raise SystemExit("profile recovery paths must not overlap")
    if any(directory == args.adapter_config or directory in args.adapter_config.parents for directory in directories):
        raise SystemExit("adapter config must be outside recovery source and output trees")
    profile_fd = proof_fd = parent_fd = -1
    try:
        adapter_config_body = read_protected_regular(args.adapter_config)
        profile_fd = open_protected_directory(args.archived_profile)
        proof_fd = open_protected_directory(args.archived_proof)
        parent_fd = open_protected_directory(args.output_seed.parent)
        source_identities = {(os.fstat(profile_fd).st_dev, os.fstat(profile_fd).st_ino),
                             (os.fstat(proof_fd).st_dev, os.fstat(proof_fd).st_ino)}
        if len(source_identities) != 2 or (os.fstat(parent_fd).st_dev, os.fstat(parent_fd).st_ino) in source_identities:
            raise ValueError("profile recovery directories must be distinct")
        _, _, already_reconstructed = reconstruct_seed(
            profile_fd, proof_fd, parent_fd, args.output_seed.name, args.client,
            adapter_config_body,
        )
    finally:
        for descriptor in (parent_fd, proof_fd, profile_fd):
            if descriptor >= 0:
                os.close(descriptor)
    if already_reconstructed:
        print("identical sealed recovery seed already reconstructed; use the installed provisioner two-pass flow")
    else:
        print("sealed recovery seed reconstructed; use the installed provisioner two-pass flow")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
