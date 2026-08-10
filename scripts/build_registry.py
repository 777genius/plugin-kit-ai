#!/usr/bin/env python3
"""Build the deterministic, Git-native public plugin registry.

External packages are treated strictly as data. This module downloads a pinned
GitHub archive, bounds and validates it, and never invokes package content.
"""

from __future__ import annotations

import argparse
import gzip
import hashlib
import json
import os
import re
import sys
import tarfile
import tempfile
import time
import unicodedata
import urllib.error
import urllib.request
from pathlib import Path, PurePosixPath
from urllib.parse import quote, urlsplit

sys.path.insert(0, str(Path(__file__).resolve().parent))
from build_agentplugins_catalog import package_tree_digest
from portable_paths import validate_segment, validate_tree
from validate_catalog import ValidationError, validate_plugin


ROOT = Path(__file__).resolve().parents[1]
ENTRIES = ROOT / "registry" / "entries"
OUTPUT = ROOT / "registry" / "index.json"
REPOSITORY_RE = re.compile(r"^[a-z0-9](?:[a-z0-9-]{0,37}[a-z0-9])?/[a-z0-9](?:[a-z0-9._-]{0,98}[a-z0-9])?$")
SHA_RE = re.compile(r"^[0-9a-f]{40}$")
CATEGORY_RE = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
DESCRIPTOR_FIELDS = {"schema_version", "repository", "revision", "path", "categories"}
APPROVED_ARCHIVE_HOSTS = {"codeload.github.com"}
CONNECT_TIMEOUT_SECONDS = 15
TOTAL_DOWNLOAD_SECONDS = 30
ARCHIVE_PROCESS_SECONDS = 30
MAX_DOWNLOAD_BYTES = 25 << 20
MAX_ARCHIVE_BYTES = 300 << 20
MAX_EXTRACTED_BYTES = 128 << 20
MAX_FILES = 5_000
MAX_MEMBERS = 6_000
MAX_FILE_BYTES = 16 << 20
MAX_PATH_DEPTH = 32
MAX_CATEGORIES = 8
ICON_NAMES = {"chrome-devtools": "googlechrome.svg", "docker-hub": "docker.svg", "hubspot-crm": "hubspot.svg", "hubspot-developer": "hubspot.svg"}
CLIENT_IDS = ("codex", "chatgpt", "cursor", "copilot", "vscode", "kiro")


class RegistryError(Exception):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise RegistryError(message)


def digest_bytes(body: bytes) -> str:
    return "sha256:" + hashlib.sha256(body).hexdigest()


def read_json(path: Path) -> object:
    def unique_object(pairs):  # type: ignore[no-untyped-def]
        result = {}
        normalized_keys = set()
        for key, item in pairs:
            require(key not in result, f"{path}: duplicate JSON key {key!r}")
            normalized = unicodedata.normalize("NFC", key).casefold()
            require(normalized not in normalized_keys, f"{path}: case/Unicode-colliding JSON key {key!r}")
            normalized_keys.add(normalized)
            result[key] = item
        return result

    def reject_constant(value: str) -> None:
        raise RegistryError(
            f"{path}: non-finite JSON number {value!r} is forbidden"
        )

    try:
        return json.loads(
            path.read_text(encoding="utf-8"),
            object_pairs_hook=unique_object,
            parse_constant=reject_constant,
        )
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        raise RegistryError(f"{path}: invalid UTF-8 JSON: {error}") from error


def read_object(path: Path) -> dict[str, object]:
    value = read_json(path)
    require(isinstance(value, dict), f"{path}: top level must be an object")
    return value


def validate_repository(value: object) -> str:
    require(isinstance(value, str) and REPOSITORY_RE.fullmatch(value) is not None, "repository must be canonical lowercase GitHub owner/repo")
    require(not value.endswith(".git"), "repository must not use a .git suffix")
    return value


def validate_registry_path(value: object) -> str:
    require(isinstance(value, str) and value.isascii(), "path must be non-empty ASCII")
    require(len(value) <= 512, "path exceeds 512 characters")
    require(value == unicodedata.normalize("NFC", value), "path must be NFC normalized")
    require("\\" not in value and "%" not in value, "path contains an ambiguous separator or escape")
    path = PurePosixPath(value)
    require(value and not path.is_absolute() and path.as_posix() == value, "path must be a normalized relative POSIX path")
    require(len(path.parts) <= MAX_PATH_DEPTH, f"path exceeds depth {MAX_PATH_DEPTH}")
    for segment in path.parts:
        try:
            validate_segment(segment)
        except ValueError as error:
            raise RegistryError(str(error)) from error
    require(".git" not in path.parts, "path must not address Git metadata")
    return value


def validate_descriptor(path: Path) -> dict[str, object]:
    descriptor = read_object(path)
    require(set(descriptor) == DESCRIPTOR_FIELDS, f"{path}: descriptor must contain only {sorted(DESCRIPTOR_FIELDS)}")
    require(descriptor["schema_version"] == 1, f"{path}: schema_version must be 1")
    repository = validate_repository(descriptor["repository"])
    revision = descriptor["revision"]
    require(isinstance(revision, str) and SHA_RE.fullmatch(revision) is not None, f"{path}: revision must be a full lowercase commit SHA")
    plugin_path = validate_registry_path(descriptor["path"])
    categories = descriptor["categories"]
    require(isinstance(categories, list) and 1 <= len(categories) <= MAX_CATEGORIES, f"{path}: categories must contain 1-{MAX_CATEGORIES} values")
    require(all(isinstance(item, str) and len(item) <= 40 and CATEGORY_RE.fullmatch(item) for item in categories), f"{path}: invalid category")
    require(categories == sorted(set(categories)), f"{path}: categories must be unique and sorted")
    name = path.stem
    require(path.name == f"{name}.json" and name.isascii() and re.fullmatch(r"(?!.*(?:--|\.\.))[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?", name) is not None, f"{path}: filename must be a normalized plugin name")
    require(PurePosixPath(plugin_path).name == name, f"{path}: path directory must match descriptor filename")
    return {"name": name, "repository": repository, "revision": revision, "path": plugin_path, "categories": categories}


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):  # type: ignore[no-untyped-def]
        raise RegistryError(f"archive download redirect is forbidden ({code})")


def archive_url(repository: str, revision: str) -> str:
    url = f"https://codeload.github.com/{quote(repository, safe='/')}/tar.gz/{revision}"
    parsed = urlsplit(url)
    require(parsed.scheme == "https" and parsed.hostname in APPROVED_ARCHIVE_HOSTS and parsed.username is None and parsed.password is None and not parsed.query and not parsed.fragment, "unsafe archive URL")
    return url


def download_archive(repository: str, revision: str, destination: Path, opener=None) -> None:  # type: ignore[no-untyped-def]
    url = archive_url(repository, revision)
    opener = opener or urllib.request.build_opener(NoRedirect())
    request = urllib.request.Request(url, headers={"Accept": "application/x-gzip", "User-Agent": "uap-registry-builder/1"})
    started = time.monotonic()
    try:
        response = opener.open(request, timeout=CONNECT_TIMEOUT_SECONDS)
        with response, destination.open("wb") as output:
            final = urlsplit(response.geturl())
            require(response.status == 200, f"archive download returned HTTP {response.status}")
            require(final.scheme == "https" and final.hostname in APPROVED_ARCHIVE_HOSTS and response.geturl() == url, "archive response URL is not approved")
            length = response.headers.get("Content-Length")
            if length is not None:
                require(length.isascii() and length.isdigit() and int(length) <= MAX_DOWNLOAD_BYTES, "archive Content-Length exceeds limit")
            total = 0
            while True:
                require(time.monotonic() - started <= TOTAL_DOWNLOAD_SECONDS, "archive download exceeded total time limit")
                chunk = response.read(min(64 << 10, MAX_DOWNLOAD_BYTES - total + 1))
                if not chunk:
                    break
                total += len(chunk)
                require(total <= MAX_DOWNLOAD_BYTES, "archive download exceeds compressed size limit")
                output.write(chunk)
    except RegistryError:
        raise
    except (OSError, urllib.error.URLError) as error:
        raise RegistryError(f"archive download failed closed: {error}") from error


def decompress_archive(compressed: Path, expanded: Path) -> None:
    total = 0
    started = time.monotonic()
    try:
        with gzip.open(compressed, "rb") as source, expanded.open("wb") as output:
            while True:
                require(time.monotonic() - started <= ARCHIVE_PROCESS_SECONDS, "archive decompression exceeded time limit")
                chunk = source.read(min(1 << 20, MAX_ARCHIVE_BYTES - total + 1))
                if not chunk:
                    break
                total += len(chunk)
                require(total <= MAX_ARCHIVE_BYTES, "expanded archive exceeds limit")
                output.write(chunk)
    except (OSError, EOFError) as error:
        raise RegistryError(f"invalid gzip archive: {error}") from error


def safe_member_path(name: str) -> PurePosixPath:
    require(name.isascii() and name == unicodedata.normalize("NFC", name), "archive path must be normalized ASCII")
    require("\\" not in name and "%" not in name and not name.startswith("/"), "archive contains an ambiguous or absolute path")
    path = PurePosixPath(name)
    require(path.as_posix() == name.rstrip("/") and path.parts and ".." not in path.parts, "archive contains a non-normalized path")
    require(len(path.parts) <= MAX_PATH_DEPTH + 1, "archive path exceeds depth limit")
    for segment in path.parts:
        try:
            validate_segment(segment)
        except ValueError as error:
            raise RegistryError(str(error)) from error
    return path


def extract_package(expanded: Path, plugin_path: str, destination: Path) -> None:
    prefix_parts: tuple[str, ...] | None = None
    selected: list[tuple[tarfile.TarInfo, tuple[str, ...]]] = []
    seen: set[str] = set()
    total = archive_files = archive_members = 0
    started = time.monotonic()
    try:
        with tarfile.open(expanded, mode="r:") as archive:
            for member in archive:
                require(time.monotonic() - started <= ARCHIVE_PROCESS_SECONDS, "archive validation exceeded time limit")
                archive_members += 1
                require(archive_members <= MAX_MEMBERS, "archive exceeds member-count limit")
                path = safe_member_path(member.name)
                require(not (member.issym() or member.islnk()) and (member.isdir() or member.isfile()), f"archive contains link or special file: {member.name!r}")
                require(not member.sparse and not any("sparse" in key.casefold() for key in member.pax_headers), f"archive contains a sparse file: {member.name!r}")
                folded = path.as_posix().casefold()
                require(folded not in seen, "archive contains duplicate or case-colliding paths")
                seen.add(folded)
                if member.isfile():
                    archive_files += 1
                    require(archive_files <= MAX_FILES, "archive exceeds file-count limit")
                    require(0 <= member.size <= MAX_FILE_BYTES, "archive file exceeds size limit")
                if prefix_parts is None:
                    prefix_parts = (path.parts[0],)
                require(path.parts[:1] == prefix_parts, "archive has multiple top-level roots")
                relative = path.parts[1:]
                target_prefix = PurePosixPath(plugin_path).parts
                if relative[:len(target_prefix)] != target_prefix:
                    continue
                package_relative = relative[len(target_prefix):]
                if not package_relative:
                    require(member.isdir(), "plugin path is not a directory")
                    continue
                require(len(package_relative) <= MAX_PATH_DEPTH, "package path exceeds depth limit")
                if member.isfile():
                    total += member.size
                    require(total <= MAX_EXTRACTED_BYTES, "package exceeds extracted-size limit")
                selected.append((member, package_relative))
            require(selected, "descriptor path does not exist in pinned archive")
            for member, relative in selected:
                target = destination.joinpath(*relative)
                if member.isdir():
                    target.mkdir(parents=True, exist_ok=True)
                    continue
                target.parent.mkdir(parents=True, exist_ok=True)
                source = archive.extractfile(member)
                require(source is not None, f"cannot read archive member {member.name!r}")
                remaining = member.size
                with target.open("wb") as output:
                    while remaining:
                        require(time.monotonic() - started <= ARCHIVE_PROCESS_SECONDS, "archive extraction exceeded time limit")
                        chunk = source.read(min(64 << 10, remaining))
                        require(bool(chunk), f"truncated archive member {member.name!r}")
                        remaining -= len(chunk)
                        output.write(chunk)
                os.chmod(target, 0o755 if member.mode & 0o111 else 0o644)
    except (tarfile.TarError, OSError) as error:
        raise RegistryError(f"invalid tar archive: {error}") from error


def component_names(root: Path, manifest: dict[str, object]) -> list[str]:
    result = []
    if manifest.get("extensions"):
        result.append("extensions")
    if (root / "mcp.json").is_file():
        result.append("mcp")
    if (root / "skills").is_dir():
        result.append("skills")
    return sorted(result)


def package_fields(root: Path, categories: list[str]) -> dict[str, object]:
    # json.load silently accepts duplicate object keys. Parse every submitted
    # JSON file with the registry's fail-closed reader before schema validation.
    for json_path in sorted(root.rglob("*.json")):
        read_json(json_path)
    try:
        validate_plugin(root)
    except (ValidationError, ValueError) as error:
        raise RegistryError(str(error)) from error
    manifest_path = root / "plugin.json"
    manifest = read_object(manifest_path)
    license_value = manifest.get("license")
    require(isinstance(license_value, str) and license_value.strip(), f"{manifest_path}: license required")
    author = manifest.get("author")
    require(isinstance(author, dict) and isinstance(author.get("name"), str) and author["name"], f"{manifest_path}: author metadata required")
    return {
        "name": manifest["name"], "version": manifest["version"], "description": manifest["description"],
        "author": author, "license": license_value, "categories": sorted(set(categories)),
        "keywords": sorted(set(manifest.get("keywords", []))), "components": component_names(root, manifest),
        "manifest_sha256": digest_bytes(manifest_path.read_bytes()), "tree_sha256": package_tree_digest(root),
    }


def canonical_manifest_repository(value: object) -> str | None:
    if not isinstance(value, str):
        return None
    parsed = urlsplit(value)
    if parsed.scheme != "https" or parsed.hostname != "github.com" or parsed.username or parsed.password or parsed.query or parsed.fragment:
        return None
    candidate = parsed.path.strip("/")
    return candidate if REPOSITORY_RE.fullmatch(candidate) and not candidate.endswith(".git") else None


def external_entry(descriptor: dict[str, object], opener=None) -> dict[str, object]:  # type: ignore[no-untyped-def]
    with tempfile.TemporaryDirectory(prefix="uap-registry-") as temporary:
        temp = Path(temporary)
        compressed, expanded, package = temp / "source.tar.gz", temp / "source.tar", temp / str(descriptor["name"])
        package.mkdir()
        download_archive(str(descriptor["repository"]), str(descriptor["revision"]), compressed, opener)
        decompress_archive(compressed, expanded)
        extract_package(expanded, str(descriptor["path"]), package)
        fields = package_fields(package, list(descriptor["categories"]))
        require(fields["name"] == descriptor["name"], "manifest name must match descriptor filename")
        manifest = read_object(package / "plugin.json")
        require(canonical_manifest_repository(manifest.get("repository")) == descriptor["repository"], "manifest repository must exactly match the pinned descriptor repository")
        source = {"repository": descriptor["repository"], "revision": descriptor["revision"], "path": descriptor["path"], "manifest_sha256": fields.pop("manifest_sha256"), "tree_sha256": fields.pop("tree_sha256")}
        result = {
            **fields,
            "source": source,
            "install_source": f"{descriptor['repository']}@{descriptor['revision']}//{descriptor['path']}",
            "built_in": False,
            "client_support": {"resolution": "install_time", "clients": list(CLIENT_IDS)},
            "validation": {"level": "schema_only", "schema": "agent-plugins-1.0", "runtime_evidence": []},
        }
        return result


def builtin_entries() -> list[dict[str, object]]:
    catalog = read_object(ROOT / "catalog" / "v2" / "catalog.json")
    repository, revision = validate_repository(catalog.get("repository")), catalog.get("revision")
    require(isinstance(revision, str) and SHA_RE.fullmatch(revision) is not None, "catalog revision is not immutable")
    catalog_by_name = {item["name"]: item for item in catalog.get("plugins", []) if isinstance(item, dict) and isinstance(item.get("name"), str)}
    result = []
    for root in sorted(path for path in (ROOT / "plugins").iterdir() if path.is_dir()):
        fields = package_fields(root, [])
        name = str(fields["name"])
        require(name in catalog_by_name, f"{name}: missing from catalog/v2")
        catalog_item = catalog_by_name[name]
        require(catalog_item.get("source_path") == f"plugins/{name}", f"{name}: catalog source mismatch")
        require(catalog_item.get("manifest_digest") == fields["manifest_sha256"], f"{name}: local manifest differs from the pinned catalog revision")
        require(catalog_item.get("tree_digest") == fields["tree_sha256"], f"{name}: local tree differs from the pinned catalog revision")
        compatibility = catalog_item.get("compatibility")
        require(isinstance(compatibility, dict) and compatibility, f"{name}: catalog compatibility is missing")
        require(set(compatibility).issubset(CLIENT_IDS), f"{name}: catalog compatibility contains an unknown client")
        supported_clients = [client for client in CLIENT_IDS if client in compatibility]
        evidence = sorted(client for client, value in compatibility.items() if isinstance(value, dict) and value.get("verification") == "tested")
        fields["categories"] = sorted(set(fields["keywords"]))
        source = {"repository": repository, "revision": revision, "path": f"plugins/{name}", "manifest_sha256": fields.pop("manifest_sha256"), "tree_sha256": fields.pop("tree_sha256")}
        item = {
            **fields,
            "source": source,
            "install_source": name,
            "built_in": True,
            "client_support": {"resolution": "catalog", "clients": supported_clients},
            "validation": {"level": "runtime_evidence" if evidence else "schema_only", "schema": "agent-plugins-1.0", "runtime_evidence": evidence},
        }
        icon_name = ICON_NAMES.get(name, name + ".svg")
        icon_path = ROOT / "assets" / "plugin-icons" / icon_name
        if not icon_path.is_file():
            png = icon_path.with_suffix(".png")
            icon_path = png if png.is_file() else icon_path
        if icon_path.is_file():
            icon_digest = digest_bytes(icon_path.read_bytes())
            source["icon_sha256"] = icon_digest
            item["icon"] = {"path": icon_path.relative_to(ROOT).as_posix(), "sha256": icon_digest}
        result.append(item)
    require(len(result) == 26, f"expected 26 built-ins, found {len(result)}")
    return result


def build(opener=None) -> dict[str, object]:  # type: ignore[no-untyped-def]
    plugins = builtin_entries()
    seen = {str(item["name"]).casefold() for item in plugins}
    if ENTRIES.exists():
        descriptor_paths = []
        for candidate in sorted(ENTRIES.iterdir()):
            require(candidate.name == ".gitkeep" or (candidate.suffix == ".json" and candidate.is_file() and not candidate.is_symlink()), f"{candidate}: registry entries may contain only regular JSON descriptors")
            if candidate.suffix == ".json":
                descriptor_paths.append(candidate)
        for descriptor_path in descriptor_paths:
            descriptor = validate_descriptor(descriptor_path)
            normalized = str(descriptor["name"]).casefold()
            require(normalized not in seen, f"duplicate normalized plugin name: {descriptor['name']}")
            item = external_entry(descriptor, opener)
            require(str(item["name"]).casefold() not in seen, f"duplicate normalized manifest name: {item['name']}")
            seen.add(normalized)
            plugins.append(item)
    plugins.sort(key=lambda item: str(item["name"]))
    return {"schema_version": 1, "plugins": plugins}


def encoded(index: dict[str, object]) -> bytes:
    return (json.dumps(index, indent=2, ensure_ascii=False, sort_keys=False) + "\n").encode("utf-8")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true", help="fail if registry/index.json is stale")
    args = parser.parse_args()
    try:
        output = encoded(build())
        if args.check:
            require(OUTPUT.is_file() and OUTPUT.read_bytes() == output, f"{OUTPUT}: generated index is stale; run scripts/build_registry.py")
        else:
            OUTPUT.parent.mkdir(parents=True, exist_ok=True)
            OUTPUT.write_bytes(output)
    except RegistryError as error:
        print(f"registry build failed: {error}", file=sys.stderr)
        return 1
    print(f"registry index valid ({len(json.loads(output)['plugins'])} plugins)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
