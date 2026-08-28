#!/usr/bin/env python3
"""Build a bounded static index of public Agent Plugins 1.0 packages.

Package repositories are treated as inert data. Discovery resolves every
candidate to an immutable default-head commit, fetches only its package tree,
and validates local vendored schemas without executing package content.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime, timezone
from pathlib import Path, PurePosixPath
from typing import Any

import jsonschema

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from scripts.build_bridges import BridgeError, GIT_MODES, LFS_HEADER, PinnedRepository, git, portable_path
from scripts.build_registry import directory_tree_digest, digest_bytes, read_json
from scripts.directory_publication import (
    PublicationError,
    canonical_json,
    format_timestamp,
    parse_timestamp,
    read_json as read_bounded_json,
    sha256_digest,
    validate_with_schema,
)


ROOT = Path(__file__).resolve().parents[1]
PLUGIN_SCHEMA = ROOT / "schemas" / "1.0.0" / "plugin.schema.json"
MCP_SCHEMA = ROOT / "schemas" / "1.0.0" / "mcp.schema.json"
DIRECTORY_SOURCE = ROOT / "registry" / "directory.json"
SNAPSHOT_SCHEMA = ROOT / "schemas" / "discovery-snapshot.schema.json"
SCHEMA_URI = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
# ChatGPT requires a separately registered and reviewed app binding. Static
# Agent Plugins conformance alone cannot prove that binding, so unreviewed
# Discovery records deliberately exclude ChatGPT until Directory promotion.
PORTABLE_CLIENTS = ["codex", "cursor", "copilot", "vscode", "kiro"]
SHA_RE = re.compile(r"^[0-9a-f]{40}$")
REPOSITORY_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9][A-Za-z0-9._-]*$")
MIN_AVAILABLE_RECORDS_FOR_DROP_GUARD = 20
MAX_FILES = 5_000
MAX_FILE_BYTES = 16 << 20
MAX_TREE_BYTES = 64 << 20
MAX_RECORDS = 10_000
MAX_API_BYTES = 8 << 20
MAX_PREVIOUS_SNAPSHOT_BYTES = 16 << 20
# Retry delays remain bounded even when GitHub returns an invalid or hostile
# rate-limit hint. This is deliberately large enough for secondary-limit
# windows such as the 75-second delay observed in production.
MAX_GITHUB_RETRY_DELAY_SECONDS = 120
MAX_GITHUB_SERVER_RETRY_DELAY_SECONDS = 30
CODE_SEARCH_REQUEST_INTERVAL_SECONDS = 6.5
REPOSITORY_GRAPHQL_BATCH = 50
SEARCH_STABILITY_ATTEMPTS = 3
SEARCH_PARTITION_MAX = 90
DEFAULT_REPOSITORY_WORKERS = 8
MAX_REPOSITORY_WORKERS = 16


class DiscoveryError(Exception):
    pass


class GitHubHTTPError(DiscoveryError):
    def __init__(self, status: int, path: str, detail: str):
        super().__init__(f"GitHub API {path} failed with HTTP {status}: {detail}")
        self.status = status


def require(condition: bool, message: str) -> None:
    if not condition:
        raise DiscoveryError(message)


def _bounded_nonnegative_integer(value: str | None, maximum: int) -> int:
    if value is None or re.fullmatch(r"[0-9]+", value.strip()) is None:
        return 0
    digits = value.strip().lstrip("0") or "0"
    limit = str(maximum)
    if len(digits) > len(limit) or (len(digits) == len(limit) and digits > limit):
        return maximum
    return int(digits)


def normalized_path(value: str) -> str:
    value = value.strip("/")
    if not value:
        return ""
    path = portable_path(value, "package path")
    return path.as_posix()


def record_identity(repository: str, package_path: str) -> str:
    return repository.casefold() + "\x00" + normalized_path(package_path).casefold()


def discovery_slug(repository: str, package_path: str) -> str:
    slug = "discovery:" + repository.casefold()
    if package_path:
        slug += "//" + package_path
    return slug


class SameOriginRedirect(urllib.request.HTTPRedirectHandler):
    def __init__(self, origin: str):
        super().__init__()
        self.origin = urllib.parse.urlsplit(origin)

    def redirect_request(self, request, file_pointer, code, message, headers, new_url):  # noqa: ANN001
        target = urllib.parse.urlsplit(new_url)
        require(
            target.scheme == self.origin.scheme and target.netloc == self.origin.netloc,
            "GitHub API attempted a cross-origin redirect",
        )
        return super().redirect_request(request, file_pointer, code, message, headers, new_url)


class GitHubAPI:
    def __init__(self, token: str, *, search_monotonic=None, search_sleep=None):  # noqa: ANN001
        require(bool(token), "GITHUB_TOKEN is required")
        self.token = token
        self.origin = "https://api.github.com"
        self.opener = urllib.request.build_opener(SameOriginRedirect(self.origin))
        self._search_monotonic = search_monotonic or time.monotonic
        self._search_sleep = search_sleep or time.sleep
        self._search_lock = threading.Lock()
        self._last_search_request_at: float | None = None

    def _pace_code_search(self) -> None:
        if self._last_search_request_at is not None:
            deadline = self._last_search_request_at + CODE_SEARCH_REQUEST_INTERVAL_SECONDS
            delay = deadline - self._search_monotonic()
            if delay > 0:
                self._search_sleep(delay)
        self._last_search_request_at = self._search_monotonic()

    def _request(self, path: str, parameters: dict[str, object] | None = None,
                 payload: dict[str, object] | None = None) -> dict[str, Any]:
        query = "" if not parameters else "?" + urllib.parse.urlencode(parameters)
        url = self.origin + "/" + path.lstrip("/") + query
        encoded = None if payload is None else json.dumps(payload, separators=(",", ":"), sort_keys=True).encode("utf-8")
        code_search = path.lstrip("/") == "search/code"
        for attempt in range(6):
            request = urllib.request.Request(url, headers={
                "Accept": "application/vnd.github+json",
                "Authorization": "Bearer " + self.token,
                "Content-Type": "application/json",
                "User-Agent": "universal-agent-plugins-discovery/1",
                "X-GitHub-Api-Version": "2022-11-28",
            }, data=encoded, method="POST" if encoded is not None else "GET")
            try:
                if code_search:
                    with self._search_lock:
                        self._pace_code_search()
                        with self.opener.open(request, timeout=30) as response:
                            body = response.read(MAX_API_BYTES + 1)
                else:
                    with self.opener.open(request, timeout=30) as response:
                        body = response.read(MAX_API_BYTES + 1)
                require(len(body) <= MAX_API_BYTES, f"GitHub response exceeds {MAX_API_BYTES} bytes")
                value = json.loads(body.decode("utf-8"))
                require(isinstance(value, dict), "GitHub response must be an object")
                return value
            except urllib.error.HTTPError as error:
                try:
                    detail = error.read(4096).decode("utf-8", "replace")
                except OSError as body_error:
                    detail = f"<unable to read response body: {type(body_error).__name__}>"
                if error.code not in {403, 429, 500, 502, 503, 504} or attempt == 5:
                    raise GitHubHTTPError(error.code, path, detail) from error
                secondary_limit = error.code in {403, 429}
                delay_cap = (
                    MAX_GITHUB_RETRY_DELAY_SECONDS
                    if secondary_limit
                    else MAX_GITHUB_SERVER_RETRY_DELAY_SECONDS
                )
                reset = error.headers.get("X-RateLimit-Reset")
                retry_after = error.headers.get("Retry-After")
                delay = _bounded_nonnegative_integer(retry_after, delay_cap)
                now = int(time.time())
                reset_at = _bounded_nonnegative_integer(reset, now + delay_cap - 1)
                if reset_at:
                    delay = max(delay, reset_at - now + 1)
                if secondary_limit:
                    match = re.search(
                        r"try again in\s+([0-9]+(?:\.[0-9]+)?)s\b",
                        detail,
                        flags=re.IGNORECASE,
                    )
                    if match:
                        seconds, point, fraction = match.group(1).partition(".")
                        message_delay = _bounded_nonnegative_integer(seconds, delay_cap)
                        if message_delay < delay_cap and point and fraction.strip("0"):
                            message_delay += 1
                        delay = max(delay, message_delay)
                time.sleep(max(1, min(delay, delay_cap)))
            except (OSError, UnicodeError, json.JSONDecodeError) as error:
                if attempt == 5:
                    raise DiscoveryError(f"GitHub API {path} failed: {error}") from error
                time.sleep(min(attempt + 1, 5))
        raise DiscoveryError(f"GitHub API {path} exhausted retries")

    def get(self, path: str, parameters: dict[str, object] | None = None) -> dict[str, Any]:
        return self._request(path, parameters=parameters)

    def graphql(self, query: str, variables: dict[str, object]) -> dict[str, Any]:
        response = self._request("graphql", payload={"query": query, "variables": variables})
        errors = response.get("errors")
        data = response.get("data")
        require(isinstance(data, dict), f"GitHub GraphQL returned invalid data: {data!r}")
        if errors:
            require(isinstance(errors, list), "GitHub GraphQL errors must be an array")
            for error in errors:
                path = error.get("path") if isinstance(error, dict) else None
                require(
                    isinstance(error, dict)
                    and error.get("type") == "NOT_FOUND"
                    and isinstance(path, list)
                    and bool(path)
                    and isinstance(path[0], str)
                    and re.fullmatch(r"r[0-9]+", path[0]) is not None,
                    f"GitHub GraphQL returned errors: {errors!r}",
                )
        return data


def partition_query(base: str, minimum: int, maximum: int) -> str:
    return f"{base} size:{minimum}..{maximum}"


def search_code_page(api: GitHubAPI, query: str, page: int) -> dict[str, Any]:
    return api.get("search/code", {
        "q": query, "per_page": 100, "page": page, "sort": "indexed", "order": "asc",
    })


def discover_search_items(api: GitHubAPI, base_query: str, maximum_size: int) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    queue = [(0, maximum_size)]
    partitions: list[dict[str, Any]] = []
    items: dict[tuple[str, str], dict[str, Any]] = {}
    while queue:
        minimum, maximum = queue.pop(0)
        query = partition_query(base_query, minimum, maximum)
        first = search_code_page(api, query, 1)
        total = first.get("total_count")
        require(type(total) is int and total >= 0, "GitHub code search returned an invalid total_count")
        require(first.get("incomplete_results") is False, f"GitHub code search is incomplete for {query!r}")
        first_items = first.get("items")
        require(isinstance(first_items, list), "GitHub code search items must be an array")
        if total > SEARCH_PARTITION_MAX or len(first_items) >= 100:
            require(minimum < maximum, f"exact-size search partition {query!r} still exceeds {SEARCH_PARTITION_MAX} results")
            midpoint = (minimum + maximum) // 2
            queue[0:0] = [(minimum, midpoint), (midpoint + 1, maximum)]
            continue
        stable_items: list[dict[str, Any]] | None = None
        stable_total = 0
        previous_identities: set[tuple[str, str]] | None = None
        instability = "not observed"
        for stability_attempt in range(SEARCH_STABILITY_ATTEMPTS):
            if stability_attempt:
                first = search_code_page(api, query, 1)
                total = first.get("total_count")
                first_items = first.get("items")
                require(type(total) is int and 0 <= total <= SEARCH_PARTITION_MAX and isinstance(first_items, list) and len(first_items) < 100,
                        f"search partition {query!r} changed beyond its deterministic boundary")
            responses = [first]
            observed = 0
            raw_identities: set[tuple[str, str]] = set()
            exact_items: list[dict[str, Any]] = []
            stable = True
            for response in responses:
                if response.get("incomplete_results") is not False or response.get("total_count") != total:
                    stable = False
                    break
                page_items = response.get("items")
                require(isinstance(page_items, list), "GitHub code search items must be an array")
                observed += len(page_items)
                for item in page_items:
                    require(isinstance(item, dict), "GitHub code search item must be an object")
                    repository = item.get("repository")
                    path = item.get("path")
                    require(isinstance(repository, dict) and isinstance(repository.get("full_name"), str), "search result repository is invalid")
                    require(isinstance(path, str), "search result path is invalid")
                    identity = (repository["full_name"].casefold(), path.casefold())
                    raw_identities.add(identity)
                    # GitHub filename search is relevance-ranked and can return
                    # near-matches. Only the exact portable manifest basename is
                    # a candidate; one fuzzy hit must not invalidate a complete scan.
                    if PurePosixPath(path).name == "plugin.json":
                        exact_items.append({"repository": repository["full_name"], "manifest_path": path})
            unique_response = stable and len(raw_identities) == observed
            if unique_response and (observed == total or raw_identities == previous_identities):
                stable_items = exact_items
                stable_total = observed
                break
            previous_identities = raw_identities if unique_response else None
            instability = (
                f"total={total}, observed={observed}, unique={len(raw_identities)}, "
                f"page_totals={[response.get('total_count') for response in responses]}"
            )
            if stability_attempt + 1 < SEARCH_STABILITY_ATTEMPTS:
                time.sleep(stability_attempt + 1)
        require(stable_items is not None, f"search partition {query!r} changed while pages were acquired ({instability})")
        partitions.append({"query": query, "size_min": minimum, "size_max": maximum, "total_count": stable_total})
        for item in stable_items:
            key = (item["repository"].casefold(), item["manifest_path"].casefold())
            items[key] = item
    partitions.sort(key=lambda item: (item["size_min"], item["size_max"]))
    return [items[key] for key in sorted(items)], partitions


def repository_states(api: GitHubAPI, repositories: list[str]) -> dict[str, dict[str, Any]]:
    """Resolve immutable default heads in bounded GraphQL batches.

    A per-repository REST metadata + commit flow exceeds the 1,000 requests/hour
    Actions token limit once Discovery reaches the measured 2,300 candidates.
    GraphQL aliases retain the same repository/default-head trust boundary while
    reducing that phase to one request per 50 repositories.
    """
    original_by_identity: dict[str, str] = {}
    for repository in repositories:
        original_by_identity.setdefault(repository.casefold(), repository)
    identities = sorted(original_by_identity)
    states: dict[str, dict[str, Any]] = {}
    for offset in range(0, len(identities), REPOSITORY_GRAPHQL_BATCH):
        batch = identities[offset:offset + REPOSITORY_GRAPHQL_BATCH]
        declarations: list[str] = []
        selections: list[str] = []
        variables: dict[str, object] = {}
        for index, repository_identity in enumerate(batch):
            repository = original_by_identity[repository_identity]
            require(REPOSITORY_RE.fullmatch(repository) is not None, f"invalid repository {repository!r}")
            owner, name = repository.split("/", 1)
            declarations.extend((f"$owner{index}: String!", f"$name{index}: String!"))
            variables[f"owner{index}"], variables[f"name{index}"] = owner, name
            selections.append(
                f"r{index}: repository(owner: $owner{index}, name: $name{index}) {{ "
                "nameWithOwner isPrivate isArchived stargazerCount pushedAt updatedAt "
                "defaultBranchRef { target { ... on Commit { oid } } } }"
            )
        query = "query(" + ", ".join(declarations) + ") { " + " ".join(selections) + " }"
        data = api.graphql(query, variables)
        require(set(data) == {f"r{index}" for index in range(len(batch))}, "GitHub GraphQL repository batch is incomplete")
        for index, repository_identity in enumerate(batch):
            repository = original_by_identity[repository_identity]
            metadata = data[f"r{index}"]
            if metadata is None:
                states[repository] = {"repository": repository, "available": False}
                continue
            require(isinstance(metadata, dict), f"{repository}: repository metadata is invalid")
            if metadata.get("isPrivate") is not False or metadata.get("isArchived") is True:
                states[repository] = {"repository": repository, "available": False}
                continue
            full_name = metadata.get("nameWithOwner")
            default_ref = metadata.get("defaultBranchRef")
            target = default_ref.get("target") if isinstance(default_ref, dict) else None
            revision = target.get("oid") if isinstance(target, dict) else None
            require(isinstance(full_name, str) and REPOSITORY_RE.fullmatch(full_name) is not None,
                    f"{repository}: canonical repository identity is invalid")
            require(isinstance(revision, str) and SHA_RE.fullmatch(revision) is not None,
                    f"{repository}: default head is not a full SHA")
            updated = metadata.get("pushedAt") or metadata.get("updatedAt")
            parse_timestamp(updated, f"{repository}.updated_at")
            stars = metadata.get("stargazerCount")
            require(type(stars) is int and stars >= 0, f"{repository}: invalid star count")
            states[repository] = {
                "repository": full_name.casefold(), "revision": revision, "stars": stars,
                "updated_at": updated, "available": True,
            }
    return states


def bounded_package_files(repository: PinnedRepository, package_path: str) -> list[tuple[str, int, bytes]]:
    prefix = normalized_path(package_path)
    pathspec = prefix or "."
    raw = git(repository.root, "ls-tree", "-rz", "-r", repository.revision, "--", pathspec)
    records: list[tuple[str, str, str]] = []
    seen: set[str] = set()
    total = 0
    for row in raw.split(b"\0"):
        if not row:
            continue
        try:
            metadata, encoded_path = row.split(b"\t", 1)
            mode, kind, object_id = metadata.decode("ascii").split(" ")
            full_path = encoded_path.decode("utf-8")
        except (ValueError, UnicodeError) as error:
            raise DiscoveryError("package tree contains an invalid Git entry") from error
        require(kind == "blob" and mode in GIT_MODES, f"unsupported package tree entry: {full_path}")
        portable_path(full_path, "package tree")
        folded = full_path.casefold()
        require(folded not in seen, f"package tree contains a case-colliding path: {full_path}")
        seen.add(folded)
        size_body = git(repository.root, "cat-file", "-s", object_id)
        try:
            size = int(size_body.strip())
        except ValueError as error:
            raise DiscoveryError(f"invalid blob size for {full_path}") from error
        require(0 <= size <= MAX_FILE_BYTES, f"package file exceeds {MAX_FILE_BYTES} bytes: {full_path}")
        total += size
        require(total <= MAX_TREE_BYTES, f"package tree exceeds {MAX_TREE_BYTES} bytes")
        records.append((full_path, mode, object_id))
        require(len(records) <= MAX_FILES, f"package tree exceeds {MAX_FILES} files")
    require(bool(records), f"package path {pathspec!r} is empty")
    result: list[tuple[str, int, bytes]] = []
    prefix_with_slash = prefix + "/" if prefix else ""
    for full_path, mode, object_id in records:
        require(not prefix_with_slash or full_path.startswith(prefix_with_slash), f"package tree escaped {prefix!r}")
        relative = full_path[len(prefix_with_slash):]
        require(relative and relative != full_path or not prefix, "package tree path normalization failed")
        body = git(repository.root, "show", f"{repository.revision}:{full_path}")
        require(not body.startswith(LFS_HEADER), f"Git LFS pointer is forbidden: {full_path}")
        result.append((relative, GIT_MODES[mode], body))
    return result


def materialize_package(files: list[tuple[str, int, bytes]], root: Path) -> None:
    for relative, mode, body in files:
        path = portable_path(relative, "package file")
        destination = root.joinpath(*PurePosixPath(path).parts)
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_bytes(body)
        destination.chmod(mode)


def validate_document(document: object, schema_path: Path, label: str) -> None:
    schema = read_json(schema_path)
    validator = jsonschema.Draft202012Validator(schema)
    errors = sorted(validator.iter_errors(document), key=lambda item: list(item.absolute_path))
    if errors:
        error = errors[0]
        location = "/".join(str(part) for part in error.absolute_path) or "<root>"
        raise DiscoveryError(f"{label}: schema error at {location}: {error.message}")


def package_facts(root: Path) -> dict[str, Any]:
    manifest_path = root / "plugin.json"
    require(manifest_path.is_file(), "plugin.json is missing")
    manifest = read_json(manifest_path)
    validate_document(manifest, PLUGIN_SCHEMA, "plugin.json")
    mcp_count = 0
    transports: set[str] = set()
    mcp_path = root / "mcp.json"
    if mcp_path.is_file():
        mcp = read_json(mcp_path)
        validate_document(mcp, MCP_SCHEMA, "mcp.json")
        servers = mcp["mcpServers"]
        mcp_count = len(servers)
        transports = {server["type"] for server in servers.values()}
    skills = [path for path in root.glob("skills/*/SKILL.md") if path.is_file() and not path.is_symlink()]
    extensions = manifest.get("extensions") or {}
    component_counts = {"extensions": len(extensions), "mcp": mcp_count, "skills": len(skills)}
    portable = mcp_count > 0 or len(skills) > 0
    author = manifest.get("author")
    if isinstance(author, dict):
        author = {key: str(author[key])[:2048] for key in ("name", "email", "url") if key in author}
    else:
        author = None
    return {
        "name": manifest["name"],
        "version": manifest.get("version"),
        "description": str(manifest.get("description") or "")[:500],
        "author": author,
        "license": manifest.get("license"),
        "components": component_counts,
        "mcp_transports": sorted(transports),
        "compatible_clients": PORTABLE_CLIENTS if portable else [],
        "authentication": "unknown" if mcp_count else "not_required",
        "manifest_digest": digest_bytes(manifest_path.read_bytes()),
        "tree_digest": directory_tree_digest(root),
    }


def reviewed_release_map(path: Path) -> dict[tuple[str, str, str], str]:
    source = read_json(path)
    result: dict[tuple[str, str, str], str] = {}
    for distribution in source.get("distributions", []):
        for release in distribution.get("releases", []):
            package_source = release.get("package_source", {})
            repository = str(package_source.get("repository", "")).casefold()
            revision = str(package_source.get("revision", ""))
            package_path = normalized_path(str(package_source.get("path", "")))
            if repository and SHA_RE.fullmatch(revision):
                result[(repository, revision, package_path.casefold())] = distribution["id"]
    return result


def make_record(repository: PinnedRepository, state: dict[str, Any], package_path: str, generated_at: str,
                previous: dict[str, Any] | None, reviewed: dict[tuple[str, str, str], str]) -> dict[str, Any]:
    files = bounded_package_files(repository, package_path)
    with tempfile.TemporaryDirectory(prefix="uap-discovery-package-") as temporary:
        root = Path(temporary)
        materialize_package(files, root)
        facts = package_facts(root)
    canonical_repository = state["repository"]
    identity = (canonical_repository, state["revision"], package_path.casefold())
    return {
        "slug": discovery_slug(canonical_repository, package_path),
        "name": facts["name"],
        "description": facts["description"],
        "owner": canonical_repository.split("/", 1)[0],
        "repository": canonical_repository,
        "package_path": package_path,
        "revision": state["revision"],
        "version": facts["version"],
        "license": facts["license"],
        "schema_version": "1.0.0",
        "components": facts["components"],
        "mcp_transports": facts["mcp_transports"],
        "compatible_clients": facts["compatible_clients"],
        "authentication": facts["authentication"],
        "status": "conformant_unreviewed",
        "runtime_reviewed": False,
        "tree_digest": facts["tree_digest"],
        "manifest_digest": facts["manifest_digest"],
        "stars": state["stars"],
        "repository_updated_at": state["updated_at"],
        "reviewed_distribution_id": reviewed.get(identity),
        "availability": "available",
        "author": facts["author"],
        "first_seen": previous["first_seen"] if previous else generated_at,
        "last_seen": generated_at,
    }


def load_previous(path: Path | None) -> list[dict[str, Any]]:
    if path is None:
        return []
    try:
        value = read_bounded_json(path, max_bytes=MAX_PREVIOUS_SNAPSHOT_BYTES)
        validate_with_schema(value, SNAPSHOT_SCHEMA)
    except PublicationError as error:
        raise DiscoveryError(str(error)) from error
    require(value.get("complete") is True, "previous Discovery snapshot is incomplete")
    validate_previous_records(value["records"])
    return value["records"]


def validate_previous_records(records: list[dict[str, Any]]) -> None:
    identities: set[str] = set()
    slugs: set[str] = set()
    for record in records:
        identity = record_identity(record["repository"], record["package_path"])
        require(identity not in identities, f"previous Discovery snapshot has duplicate package identity {identity!r}")
        require(record["slug"] not in slugs, f"previous Discovery snapshot has duplicate slug {record['slug']!r}")
        require(record["slug"] == discovery_slug(record["repository"], record["package_path"]),
                f"previous Discovery snapshot has a non-canonical slug {record['slug']!r}")
        identities.add(identity)
        slugs.add(record["slug"])


def candidate_paths(items: list[dict[str, Any]]) -> dict[str, set[str]]:
    result: dict[str, set[str]] = {}
    canonical_keys: dict[str, str] = {}
    for item in items:
        repository = item["repository"]
        key = canonical_keys.setdefault(repository.casefold(), repository)
        manifest = PurePosixPath(item["manifest_path"])
        require(manifest.name == "plugin.json", f"invalid manifest path {manifest}")
        package_path = "" if str(manifest.parent) == "." else normalized_path(str(manifest.parent))
        result.setdefault(key, set()).add(package_path)
    return result


def scan_repository(repository_name: str, state: dict[str, Any],
                    pending: list[tuple[str, str, dict[str, Any] | None]], generated_at: str,
                    reviewed: dict[tuple[str, str, str], str], mirror_root: Path | None,
                    ) -> tuple[dict[str, dict[str, Any]], list[dict[str, str]]]:
    """Materialize one pinned repository once and validate all of its packages."""
    records: dict[str, dict[str, Any]] = {}
    diagnostics: list[dict[str, str]] = []
    try:
        pinned = PinnedRepository(state["repository"], state["revision"], mirror_root)
    except BridgeError as error:
        diagnostics.append({
            "kind": "scan_error", "repository": repository_name, "path": "", "error": str(error),
        })
        return records, diagnostics
    try:
        for package_path, identity, prior in pending:
            try:
                records[identity] = make_record(pinned, state, package_path, generated_at, prior, reviewed)
            except BridgeError as error:
                diagnostics.append({
                    "kind": "scan_error", "repository": state["repository"],
                    "path": package_path, "error": str(error),
                })
            except Exception as error:
                diagnostics.append({
                    "kind": "invalid", "repository": state["repository"],
                    "path": package_path, "error": str(error),
                })
                if prior:
                    records[identity] = {**prior, "availability": "unavailable"}
    finally:
        pinned.close()
    return records, diagnostics


def build_candidate(*, api: GitHubAPI, config: dict[str, Any], mode: str, generated_at: str,
                    previous_records: list[dict[str, Any]], mirror_root: Path | None = None,
                    repository_workers: int = DEFAULT_REPOSITORY_WORKERS,
                    ) -> tuple[dict[str, Any], list[dict[str, str]]]:
    parse_timestamp(generated_at, "generated_at")
    require(type(repository_workers) is int and 1 <= repository_workers <= MAX_REPOSITORY_WORKERS,
            f"repository workers must be between 1 and {MAX_REPOSITORY_WORKERS}")
    validate_previous_records(previous_records)
    # A refresh is intentionally metadata-only once records exist. On a fresh
    # ledger (or after an authenticated empty snapshot), however, there is
    # nothing to refresh; continue as a discover scan so the signed index does
    # not remain permanently empty while accurately recording the scan mode.
    effective_mode = "discover" if mode == "refresh" and not previous_records else mode
    previous = {record_identity(item["repository"], item["package_path"]): item for item in previous_records}
    partitions: list[dict[str, Any]] = []
    if effective_mode == "refresh":
        paths: dict[str, set[str]] = {}
        for record in previous_records:
            paths.setdefault(record["repository"], set()).add(record["package_path"])
    else:
        items, partitions = discover_search_items(api, config["query"], config["maximum_file_size"])
        by_identity = {(item["repository"].casefold(), item["manifest_path"].casefold()): item for item in items}
        for seed in config["seeds"]:
            seed_query = config["query"] + " repo:" + seed["repository"]
            for prefix in seed["paths"] or [""]:
                scoped_query = seed_query + (" path:" + prefix if prefix else "")
                seed_items, seed_partitions = discover_search_items(api, scoped_query, config["maximum_file_size"])
                partitions.extend(seed_partitions)
                for item in seed_items:
                    by_identity[(item["repository"].casefold(), item["manifest_path"].casefold())] = item
        items = [by_identity[key] for key in sorted(by_identity)]
        partitions.sort(key=lambda item: (item["query"], item["size_min"], item["size_max"]))
        paths = candidate_paths(items)
    records: dict[str, dict[str, Any]] = dict(previous)
    diagnostics: list[dict[str, str]] = []
    reviewed = reviewed_release_map(DIRECTORY_SOURCE)
    discovered = {record_identity(repository, path) for repository, package_paths in paths.items() for path in package_paths}
    states = repository_states(api, list(paths))
    repository_jobs: dict[str, tuple[dict[str, Any], list[tuple[str, str, dict[str, Any] | None]]]] = {}
    for repository_name in sorted(paths):
        state = states[repository_name]
        if not state["available"]:
            for package_path in paths[repository_name]:
                identity = record_identity(repository_name, package_path)
                if identity in records:
                    records[identity] = {**records[identity], "availability": "unavailable"}
            diagnostics.append({"kind": "unavailable", "repository": repository_name, "path": "", "error": "repository is archived or no longer public"})
            continue
        canonical_repository = state["repository"]
        transferred = canonical_repository != repository_name.casefold()
        for package_path in paths[repository_name]:
            discovered.add(record_identity(canonical_repository, package_path))
        if transferred:
            for package_path in paths[repository_name]:
                old_identity = record_identity(repository_name, package_path)
                if old_identity in records:
                    records[old_identity] = {**records[old_identity], "availability": "unavailable"}
        pending: list[tuple[str, str, dict[str, Any] | None]] = []
        for package_path in sorted(paths[repository_name]):
            identity = record_identity(canonical_repository, package_path)
            prior = previous.get(identity)
            if prior is None and transferred:
                prior = previous.get(record_identity(repository_name, package_path))
            # Repository metadata comes from the immutable default-head lookup.
            # If that identity did not change, no package bytes need to be fetched
            # or parsed again. A transfer is deliberately excluded: its canonical
            # repository and install slug must be rebuilt even when the commit SHA
            # happens to remain identical.
            if not transferred and prior and prior["revision"] == state["revision"] and prior.get("availability") == "available":
                retained = dict(prior)
                retained.update({"stars": state["stars"], "repository_updated_at": state["updated_at"], "last_seen": generated_at})
                records[identity] = retained
                continue
            pending.append((package_path, identity, prior))
        if not pending:
            continue
        repository_jobs[repository_name] = (state, pending)
    completed: dict[str, tuple[dict[str, dict[str, Any]], list[dict[str, str]]]] = {}
    if repository_jobs:
        worker_count = min(repository_workers, len(repository_jobs))
        with ThreadPoolExecutor(max_workers=worker_count, thread_name_prefix="discovery-repository") as executor:
            futures = {
                executor.submit(
                    scan_repository, repository_name, state, pending, generated_at, reviewed, mirror_root,
                ): repository_name
                for repository_name, (state, pending) in repository_jobs.items()
            }
            for future in as_completed(futures):
                repository_name = futures[future]
                try:
                    completed[repository_name] = future.result()
                except Exception as error:
                    completed[repository_name] = ({}, [{
                        "kind": "scan_error", "repository": repository_name, "path": "",
                        "error": f"repository scan failed: {error}",
                    }])
    # Futures may complete in any order. Merge only by source order so records
    # and diagnostics remain byte-for-byte deterministic.
    for repository_name in sorted(completed):
        repository_records, repository_diagnostics = completed[repository_name]
        records.update(repository_records)
        diagnostics.extend(repository_diagnostics)
    if effective_mode == "reconcile":
        for identity, record in list(records.items()):
            if identity in discovered:
                continue
            unavailable = dict(record)
            unavailable["availability"] = "unavailable"
            records[identity] = unavailable
    ordered = sorted(records.values(), key=lambda item: (item["repository"], item["package_path"].casefold(), item["slug"]))
    maximum_records = min(config.get("maximum_records", MAX_RECORDS), MAX_RECORDS)
    require(len(ordered) <= maximum_records, f"Discovery Index exceeds {maximum_records} records")
    previous_available = sum(record.get("availability") == "available" for record in previous_records)
    current_available = sum(record.get("availability") == "available" for record in ordered)
    if (
        previous_available >= MIN_AVAILABLE_RECORDS_FOR_DROP_GUARD
        and current_available * 2 < previous_available
    ):
        diagnostics.append({
            "kind": "scan_error",
            "repository": "*",
            "path": "",
            "error": (
                "available Discovery records fell from "
                f"{previous_available} to {current_available}; preserving the last-known-good index"
            ),
        })
    complete = not any(item["kind"] == "scan_error" for item in diagnostics)
    candidate_path_identities = sorted(
        record_identity(repository, package_path)
        for repository, package_paths in paths.items()
        for package_path in package_paths
    )
    query_manifest = {
        "schema_version": config["schema_version"], "mode": effective_mode, "query": config["query"],
        "partitions": partitions, "candidate_paths_digest": sha256_digest(canonical_json(candidate_path_identities)),
    }
    candidate = {
        "candidate_schema_version": 1,
        "mode": effective_mode,
        "generated_at": generated_at,
        "complete": complete,
        "query_manifest_digest": sha256_digest(canonical_json(query_manifest)),
        "partitions": partitions,
        "records": ordered,
    }
    return candidate, diagnostics


def load_config(path: Path) -> dict[str, Any]:
    value = read_json(path)
    require(set(value) == {"schema_version", "query", "maximum_file_size", "maximum_records", "seeds"}, "Discovery config fields are invalid")
    require(value["schema_version"] == 1, "Discovery config schema version is invalid")
    require(isinstance(value["query"], str) and SCHEMA_URI in value["query"], "Discovery query must bind Agent Plugins 1.0")
    require(type(value["maximum_file_size"]) is int and 1 <= value["maximum_file_size"] <= 1 << 20, "invalid maximum file size")
    require(type(value["maximum_records"]) is int and 1 <= value["maximum_records"] <= MAX_RECORDS, "invalid maximum records")
    require(isinstance(value["seeds"], list), "Discovery seeds must be an array")
    for seed in value["seeds"]:
        require(isinstance(seed, dict) and set(seed) == {"repository", "paths"}, "Discovery seed fields are invalid")
        require(isinstance(seed["repository"], str) and REPOSITORY_RE.fullmatch(seed["repository"]), "Discovery seed repository is invalid")
        require(isinstance(seed["paths"], list), "Discovery seed paths must be an array")
        for path in seed["paths"]:
            require(isinstance(path, str), "Discovery seed path must be a string")
            normalized_path(path)
    return value


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=("refresh", "discover", "reconcile"), required=True)
    parser.add_argument("--config", type=Path, default=ROOT / "registry" / "discovery" / "config.json")
    parser.add_argument("--previous-snapshot", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--diagnostics-output", type=Path, required=True)
    parser.add_argument("--generated-at", default=format_timestamp(datetime.now(timezone.utc)))
    parser.add_argument("--mirror-root", type=Path)
    parser.add_argument("--repository-workers", type=int, default=DEFAULT_REPOSITORY_WORKERS)
    args = parser.parse_args()
    try:
        api = GitHubAPI(os.environ.get("GITHUB_TOKEN", ""))
        candidate, diagnostics = build_candidate(
            api=api, config=load_config(args.config), mode=args.mode, generated_at=args.generated_at,
            previous_records=load_previous(args.previous_snapshot), mirror_root=args.mirror_root,
            repository_workers=args.repository_workers,
        )
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.diagnostics_output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_bytes(canonical_json(candidate))
        args.diagnostics_output.write_bytes(canonical_json({"schema_version": 1, "diagnostics": diagnostics}))
        if not candidate["complete"]:
            print(f"Discovery scan incomplete: {len(diagnostics)} diagnostics; previous index must remain active", file=sys.stderr)
            return 3
        print(f"Discovery scan complete: {len(candidate['records'])} records")
        return 0
    except (DiscoveryError, OSError, ValueError, jsonschema.SchemaError) as error:
        print(f"Discovery build failed: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
