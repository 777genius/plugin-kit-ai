#!/usr/bin/env python3
"""Produce sanitized Chrome DevTools five-client lifecycle evidence.

The runner deliberately installs only Claude Code. Gemini, OpenCode, Cline, and
Windsurf are represented by fresh, product-shaped configuration roots so the
evidence proves only the manager-owned projection/configuration contract for
those clients. No model runtime, browser control, OAuth flow, or user project is
used.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import stat
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable
from zoneinfo import ZoneInfo


CLIENTS = ("claude", "gemini", "opencode", "cline", "windsurf")
TARGETS = ",".join(CLIENTS)
EXPECTED_MODES = {
    "claude": "compatibility_projection",
    "gemini": "native",
    "opencode": "prepared_package",
    "cline": "native",
    "windsurf": "prepared_package",
}
EXPECTED_SKILLS = {
    "a11y-debugging",
    "chrome-devtools",
    "chrome-devtools-cli",
    "cookie-debugging",
    "debug-optimize-lcp",
    "memory-leak-debugging",
    "troubleshooting",
}
PRIVACY_EXCLUSIONS = (
    "temporary_paths",
    "operation_ids",
    "account identifiers",
    "environment_variables",
    "credentials",
    "tokens",
    "cookies",
    "OAuth codes",
    "OAuth state",
    "authorization URLs",
)
GENERATOR_PATHS = (
    ".github/workflows/upstream-package-e2e.yml",
    "schemas/e2e/chrome-five-client-lifecycle.schema.json",
    "scripts/run_chrome_five_client_lifecycle.py",
)
CLIENT_SECRET_ENV_KEYS = frozenset(("GH_TOKEN", "GITHUB_TOKEN"))
SHA_PATTERN = re.compile(r"^[0-9a-f]{40}$")


class EvidenceError(RuntimeError):
    """Raised when a lifecycle or evidence invariant is not satisfied."""


def utc_now() -> datetime:
    return datetime.now(timezone.utc)


def rfc3339(value: datetime) -> str:
    return value.astimezone(timezone.utc).isoformat(timespec="seconds").replace(
        "+00:00", "Z"
    )


def parse_rfc3339(value: str) -> datetime:
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as error:
        raise EvidenceError(f"invalid RFC3339 timestamp: {value}") from error
    if parsed.tzinfo is None:
        raise EvidenceError(f"timestamp is not timezone-aware: {value}")
    return parsed.astimezone(timezone.utc)


def scrub_client_env(env: dict[str, str]) -> dict[str, str]:
    """Remove GitHub credentials before invoking a client or the manager."""
    return {
        key: value
        for key, value in env.items()
        if key not in CLIENT_SECRET_ENV_KEYS
    }


def require_generator_at_commit(
    git: Path, repository_root: Path, source_commit: str
) -> None:
    """Bind evidence to the exact committed workflow, schema, and generator."""
    if not SHA_PATTERN.fullmatch(source_commit):
        raise EvidenceError("source commit must be an exact lowercase Git SHA")
    completed = subprocess.run(
        [str(git), "-C", str(repository_root), "rev-parse", "HEAD"],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=30,
        check=False,
    )
    if completed.returncode != 0 or completed.stdout.strip() != source_commit:
        raise EvidenceError("source commit is not the checked-out generator commit")
    for relative in GENERATOR_PATHS:
        committed = subprocess.run(
            [
                str(git),
                "-C",
                str(repository_root),
                "cat-file",
                "-e",
                f"{source_commit}:{relative}",
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE,
            timeout=30,
            check=False,
        )
        if committed.returncode != 0:
            raise EvidenceError(
                f"evidence generator file is unavailable at source commit: {relative}"
            )
    unchanged = subprocess.run(
        [
            str(git),
            "-C",
            str(repository_root),
            "diff",
            "--quiet",
            source_commit,
            "--",
            *GENERATOR_PATHS,
        ],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.PIPE,
        timeout=30,
        check=False,
    )
    if unchanged.returncode != 0:
        raise EvidenceError("evidence generator files differ from the source commit")


def checked_json(
    argv: list[str],
    *,
    env: dict[str, str] | None = None,
    cwd: Path | None = None,
    expect_success: bool = True,
    timeout: int = 180,
) -> tuple[dict[str, Any], str, int]:
    completed = subprocess.run(
        argv,
        env=env,
        cwd=cwd,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=timeout,
        check=False,
    )
    if expect_success and completed.returncode != 0:
        raise EvidenceError(
            f"command {argv[1] if len(argv) > 1 else argv[0]!r} failed: "
            f"{completed.stderr.strip()[:1000]}"
        )
    if not expect_success and completed.returncode == 0:
        raise EvidenceError(
            f"command {argv[1] if len(argv) > 1 else argv[0]!r} unexpectedly succeeded"
        )
    try:
        value = json.loads(completed.stdout)
    except json.JSONDecodeError as error:
        raise EvidenceError(
            f"command {argv[1] if len(argv) > 1 else argv[0]!r} did not emit JSON"
        ) from error
    if not isinstance(value, dict):
        raise EvidenceError("command JSON root must be an object")
    return value, completed.stderr, completed.returncode


def gh_json(gh: Path, argv: list[str], *, env: dict[str, str]) -> dict[str, Any]:
    value, _, _ = checked_json([str(gh), *argv], env=env, timeout=60)
    return value


def observe_upstream_head(
    gh: Path,
    *,
    repository: str,
    pr_number: int,
    expected_head: str,
    env: dict[str, str],
) -> dict[str, str]:
    """Observe an open PR head twice around its Git commit metadata lookup."""

    def pr() -> dict[str, Any]:
        return gh_json(
            gh,
            [
                "pr",
                "view",
                str(pr_number),
                "--repo",
                repository,
                "--json",
                "headRefOid,state,url",
            ],
            env=env,
        )

    first = pr()
    if first.get("state") != "OPEN" or first.get("headRefOid") != expected_head:
        raise EvidenceError("upstream PR is not open at the expected immutable head")
    commit = gh_json(
        gh,
        ["api", f"repos/{repository}/git/commits/{expected_head}"],
        env=env,
    )
    second = pr()
    if second.get("state") != "OPEN" or second.get("headRefOid") != expected_head:
        raise EvidenceError("upstream PR head changed during observation")
    if first.get("url") != second.get("url"):
        raise EvidenceError("upstream PR URL changed during observation")
    tree = commit.get("tree")
    committer = commit.get("committer")
    if not isinstance(tree, dict) or not isinstance(committer, dict):
        raise EvidenceError("upstream commit metadata is incomplete")
    tree_sha = tree.get("sha")
    committed_at = committer.get("date")
    if not isinstance(tree_sha, str) or len(tree_sha) != 40:
        raise EvidenceError("upstream Git tree SHA is invalid")
    if not isinstance(committed_at, str):
        raise EvidenceError("upstream commit timestamp is missing")
    observed = utc_now()
    if parse_rfc3339(committed_at) > observed:
        raise EvidenceError("upstream commit timestamp is later than its observation")
    return {
        "url": str(first["url"]),
        "commit_sha": expected_head,
        "git_tree_sha": tree_sha,
        "commit_timestamp_utc": rfc3339(parse_rfc3339(committed_at)),
        "observed_at_utc": rfc3339(observed),
    }


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def snapshot_roots(roots: dict[str, Path]) -> str:
    """Hash topology, modes, symlink targets, and file bytes without mtimes."""
    digest = hashlib.sha256()
    for label, root in sorted(roots.items()):
        if not root.is_dir() or root.is_symlink():
            raise EvidenceError(f"snapshot root {label!r} is not a real directory")
        paths = [root, *sorted(root.rglob("*"), key=lambda value: value.as_posix())]
        for path in paths:
            relative = "." if path == root else path.relative_to(root).as_posix()
            info = path.lstat()
            mode = stat.S_IMODE(info.st_mode)
            if stat.S_ISREG(info.st_mode):
                kind = "file"
                body = path.read_bytes()
            elif stat.S_ISDIR(info.st_mode):
                kind = "directory"
                body = b""
            elif stat.S_ISLNK(info.st_mode):
                kind = "symlink"
                body = os.readlink(path).encode("utf-8")
            else:
                raise EvidenceError(f"unsupported filesystem object in {label}: {relative}")
            digest.update(f"{label}\0{relative}\0{kind}\0{mode:o}\0".encode())
            digest.update(len(body).to_bytes(8, "big"))
            digest.update(body)
    return "sha256:" + digest.hexdigest()


def require_success(value: dict[str, Any], command: str) -> dict[str, Any]:
    if value.get("result") != "success" or value.get("command") != command:
        raise EvidenceError(f"{command} did not report success")
    data = value.get("data")
    if not isinstance(data, dict):
        raise EvidenceError(f"{command} data is not an object")
    return data


def validate_batch(
    value: dict[str, Any],
    *,
    command: str,
    expected_status: str,
) -> tuple[dict[str, Any], dict[str, dict[str, Any]]]:
    data = require_success(value, command)
    if (
        data.get("status") != expected_status
        or data.get("succeeded") != 5
        or data.get("failed") != 0
    ):
        raise EvidenceError(f"{command} did not complete all five targets")
    targets = data.get("targets")
    if not isinstance(targets, list) or len(targets) != 5:
        raise EvidenceError(f"{command} target result count is not five")
    by_id: dict[str, dict[str, Any]] = {}
    for item in targets:
        if not isinstance(item, dict) or not isinstance(item.get("target"), str):
            raise EvidenceError(f"{command} contains a malformed target result")
        by_id[item["target"]] = item
    if set(by_id) != set(CLIENTS):
        raise EvidenceError(f"{command} target set is not the exact five-client set")
    return data, by_id


def validate_dry_run(value: dict[str, Any], revision: str) -> dict[str, str]:
    data, targets = validate_batch(value, command="add", expected_status="planned")
    if data.get("dry_run") is not True or data.get("revision") != revision:
        raise EvidenceError("dry-run is not bound to the expected immutable revision")
    if "acquisition" in data or "target_outcomes" in data:
        raise EvidenceError("dry-run exposed a completed acquisition proof")
    shared: dict[str, set[str]] = {
        "tree_digest": set(),
        "manifest_digest": set(),
        "physical_artifact_id": set(),
    }
    for client, item in targets.items():
        if item.get("status") != "ready":
            raise EvidenceError(f"dry-run target {client} is not ready")
        output = item.get("output")
        result = output.get("result") if isinstance(output, dict) else None
        plan = result.get("plan") if isinstance(result, dict) else None
        if not isinstance(plan, dict) or plan.get("client_id") != client:
            raise EvidenceError(f"dry-run target {client} has no exact plan")
        if plan.get("package_mode") != EXPECTED_MODES[client]:
            raise EvidenceError(f"dry-run target {client} has an unexpected package mode")
        components = plan.get("components")
        skill_names = {
            component.get("name")
            for component in components
            if isinstance(component, dict) and component.get("kind") == "skill"
        } if isinstance(components, list) else set()
        if skill_names != EXPECTED_SKILLS:
            raise EvidenceError(f"dry-run target {client} has an unexpected skill surface")
        if not isinstance(output, dict):
            raise EvidenceError(f"dry-run target {client} output is missing")
        for field in ("tree_digest", "manifest_digest"):
            shared[field].add(str(output.get(field, "")))
        shared["physical_artifact_id"].add(str(plan.get("physical_artifact_id", "")))
    if any(len(values) != 1 or "" in values for values in shared.values()):
        raise EvidenceError("five dry-run plans do not share one acquired package identity")
    identity = {field: next(iter(values)) for field, values in shared.items()}
    if data.get("tree_digest") != identity["tree_digest"] or data.get(
        "manifest_digest"
    ) != identity["manifest_digest"]:
        raise EvidenceError("batch and target package digests differ")
    return identity


def validate_completed_acquisition(
    data: dict[str, Any], identity: dict[str, str]
) -> None:
    acquisition = data.get("acquisition")
    expected_fields = {
        "acquisition_id",
        "acquisition_count",
        "tree_digest",
        "manifest_digest",
        "closure_digest",
        "source_kind",
        "fetched",
        "validated",
    }
    if not isinstance(acquisition, dict) or set(acquisition) != expected_fields:
        raise EvidenceError("add acquisition proof is missing or malformed")
    acquisition_id = acquisition.get("acquisition_id")
    closure_digest = acquisition.get("closure_digest")
    if (
        not isinstance(acquisition_id, str)
        or re.fullmatch(r"acq-[0-9a-f]{32}", acquisition_id) is None
        or type(acquisition.get("acquisition_count")) is not int
        or acquisition.get("acquisition_count") != 1
        or acquisition.get("tree_digest") != identity["tree_digest"]
        or acquisition.get("manifest_digest") != identity["manifest_digest"]
        or not isinstance(closure_digest, str)
        or re.fullmatch(r"sha256:[0-9a-f]{64}", closure_digest) is None
        or acquisition.get("source_kind") != "github"
        or acquisition.get("fetched") is not True
        or acquisition.get("validated") is not True
    ):
        raise EvidenceError("add acquisition proof does not prove one validated fetch")

    outcomes = data.get("target_outcomes")
    if not isinstance(outcomes, dict) or set(outcomes) != set(CLIENTS):
        raise EvidenceError("add acquisition outcomes are not the exact five targets")
    expected_outcome_fields = {
        "outcome",
        "acquisition_id",
        "tree_digest",
        "manifest_digest",
        "closure_digest",
    }
    for client, outcome in outcomes.items():
        if not isinstance(outcome, dict) or set(outcome) != expected_outcome_fields:
            raise EvidenceError(f"add acquisition outcome for {client} is malformed")
        if (
            outcome.get("outcome") != "passed"
            or outcome.get("acquisition_id") != acquisition_id
            or outcome.get("tree_digest") != acquisition["tree_digest"]
            or outcome.get("manifest_digest") != acquisition["manifest_digest"]
            or outcome.get("closure_digest") != closure_digest
        ):
            raise EvidenceError(
                f"add acquisition outcome for {client} is not bound to the shared fetch"
            )


def validate_add(value: dict[str, Any], revision: str, identity: dict[str, str]) -> str:
    data, targets = validate_batch(value, command="add", expected_status="completed")
    if data.get("revision") != revision:
        raise EvidenceError("add revision changed after dry-run")
    for field in ("tree_digest", "manifest_digest"):
        if data.get(field) != identity[field]:
            raise EvidenceError(f"add {field} changed after dry-run")
    validate_completed_acquisition(data, identity)
    artifact_ids: set[str] = set()
    for client, item in targets.items():
        if item.get("status") != "external_completed":
            raise EvidenceError(f"add target {client} was not externally completed")
        output = item.get("output")
        result = output.get("result") if isinstance(output, dict) else None
        plan = result.get("plan") if isinstance(result, dict) else None
        activation = result.get("activation") if isinstance(result, dict) else None
        if not isinstance(plan, dict) or plan.get("package_mode") != EXPECTED_MODES[client]:
            raise EvidenceError(f"add target {client} changed its package mode")
        if not isinstance(activation, dict) or activation.get("verification") != "installation_verified":
            raise EvidenceError(f"add target {client} was not installation-verified")
        if result.get("mutated") is not True:
            raise EvidenceError(f"add target {client} did not mutate the sandbox")
        artifact_ids.add(str(plan.get("physical_artifact_id", "")))
    if len(artifact_ids) != 1 or "" in artifact_ids:
        raise EvidenceError("add did not reuse one physical artifact across five targets")
    return next(iter(artifact_ids))


def validate_info(value: dict[str, Any], revision: str) -> None:
    data = require_success(value, "info")
    if data.get("name") != "chrome-devtools" or data.get("mixed_version") is not False:
        raise EvidenceError("info does not describe one consistent chrome-devtools installation")
    clients = data.get("clients")
    if not isinstance(clients, list) or len(clients) != 5:
        raise EvidenceError("info does not contain five clients")
    by_id = {item.get("client_id"): item for item in clients if isinstance(item, dict)}
    if set(by_id) != set(CLIENTS):
        raise EvidenceError("info client set is not exact")
    for client, item in by_id.items():
        package_revision = item.get("package_revision")
        if (
            item.get("materialization") != "materialized"
            or item.get("verification") != "installation_verified"
            or not isinstance(package_revision, dict)
            or package_revision.get("resolved_revision") != revision
        ):
            raise EvidenceError(f"info target {client} is not materialized at the exact revision")
    claude = by_id["claude"]
    if (
        claude.get("receipt_reconciled") is not True
        or claude.get("native_discovery_reconciled") is not True
        or claude.get("native_identity_state") != "managed"
    ):
        raise EvidenceError("Claude CLI discovery did not reconcile the managed plugin")


def validate_doctor(value: dict[str, Any], *, post_remove: bool) -> None:
    data = require_success(value, "doctor")
    if data.get("read_only") is not True:
        raise EvidenceError("doctor did not report read-only execution")
    findings = data.get("findings")
    if not isinstance(findings, list):
        raise EvidenceError("doctor findings are missing")
    if post_remove:
        if findings != [
            {
                "status": "healthy",
                "code": "no_degradation_detected",
                "message": "no tracked degradation was detected",
            }
        ]:
            raise EvidenceError("post-remove doctor did not report a clean state")
        return
    if len(findings) != 5:
        raise EvidenceError("installed doctor finding count is not five")
    clients = set()
    for finding in findings:
        if not isinstance(finding, dict):
            raise EvidenceError("doctor finding is malformed")
        if finding.get("status") != "unknown" or finding.get("code") != "authentication_not_checked":
            raise EvidenceError("doctor reported an unexpected installed-state finding")
        clients.add(finding.get("client_id"))
    if clients != set(CLIENTS):
        raise EvidenceError("doctor findings are not scoped to the exact five clients")


def validate_claude_plugins(value: Any, *, installed: bool, version: str) -> None:
    if not isinstance(value, list):
        raise EvidenceError("Claude plugin list is not an array")
    if not installed:
        if value:
            raise EvidenceError("Claude still discovers a plugin after removal")
        return
    if len(value) != 1 or not isinstance(value[0], dict):
        raise EvidenceError("Claude did not discover exactly one installed plugin")
    plugin = value[0]
    if (
        plugin.get("id") != "chrome-devtools@skills-dir"
        or plugin.get("version") != version
        or plugin.get("scope") != "user"
        or plugin.get("enabled") is not True
    ):
        raise EvidenceError("Claude discovered an unexpected plugin identity")


def ensure_no_managed_residue(sandbox: Path) -> None:
    state_path = sandbox / "state" / "state-v2.json"
    state = json.loads(state_path.read_text(encoding="utf-8"))
    if state.get("installations") not in (None, []):
        raise EvidenceError("manager state still contains an installation")
    managed = sandbox / "state" / "managed"
    if managed.exists() and any(path.is_file() or path.is_symlink() for path in managed.rglob("*")):
        raise EvidenceError("managed package files remain after removal")
    plugin_data = sandbox / "state" / "plugin-data"
    if plugin_data.exists() and any(plugin_data.iterdir()):
        raise EvidenceError("owned plugin data remains after --purge-data")

    forbidden_names = EXPECTED_SKILLS
    for root in (sandbox / "home", sandbox / "config"):
        for path in root.rglob("*"):
            if path.name in forbidden_names or path.name.startswith("chrome-devtools-"):
                raise EvidenceError(f"managed client artifact remains after removal: {path.name}")
            if path.is_file() and path.stat().st_size <= 2 * 1024 * 1024:
                body = path.read_text(encoding="utf-8", errors="ignore")
                if '"chrome-devtools"' in body or "chrome-devtools@skills-dir" in body:
                    raise EvidenceError("client configuration still references chrome-devtools")


def validate_timestamp_order(source: dict[str, str]) -> None:
    ordered = [
        "commit_timestamp_utc",
        "head_observed_at_utc",
        "lifecycle_started_at_utc",
        "lifecycle_completed_at_utc",
        "head_rechecked_at_utc",
    ]
    values = [parse_rfc3339(source[field]) for field in ordered]
    if any(left > right for left, right in zip(values, values[1:])):
        raise EvidenceError("source and lifecycle timestamps are not causally ordered")


def ensure_sanitized(evidence: dict[str, Any], sandbox: Path) -> None:
    serialized = json.dumps(evidence, sort_keys=True)
    forbidden_values = {
        str(sandbox),
        str(sandbox.parent),
        os.environ.get("HOME", ""),
    }
    for value in forbidden_values:
        if value and value != "/" and value in serialized:
            raise EvidenceError("sanitized evidence contains a local filesystem path")
    forbidden_keys = {"operation_id", "installPath", "environment", "stdout", "stderr"}

    def visit(value: Any) -> None:
        if isinstance(value, dict):
            if forbidden_keys.intersection(value):
                raise EvidenceError("sanitized evidence contains a forbidden raw field")
            for child in value.values():
                visit(child)
        elif isinstance(value, list):
            for child in value:
                visit(child)

    visit(evidence)


def binary_digest(cache: Path, version: str, package_root: Path) -> str:
    package = json.loads((package_root / "package.json").read_text(encoding="utf-8"))
    assets = json.loads((package_root / "assets.json").read_text(encoding="utf-8"))
    if package.get("name") != "universal-agent-plugins" or package.get("version") != version:
        raise EvidenceError("installed npm CLI package identity is not exact")
    candidates = [path for path in (cache / "agentplugins" / version).glob("*/agentplugins*") if path.is_file()]
    if len(candidates) != 1:
        raise EvidenceError("expected exactly one authenticated cached agentplugins binary")
    digest = file_sha256(candidates[0])
    expected = {
        value.get("sha256")
        for value in assets.get("assets", {}).values()
        if isinstance(value, dict)
    }
    if digest not in expected:
        raise EvidenceError("cached CLI binary does not match the npm package manifest")
    return "sha256:" + digest


def seed_clients(sandbox: Path) -> None:
    home = sandbox / "home"
    config = sandbox / "config"
    for path in (
        home / ".gemini",
        config / "opencode",
        config / "Code" / "User" / "globalStorage" / "saoudrizwan.claude-dev",
        home / "Library" / "Application Support" / "Code" / "User" / "globalStorage" / "saoudrizwan.claude-dev",
        home / ".codeium" / "windsurf",
    ):
        path.mkdir(parents=True, mode=0o700)
    (home / ".claude.json").write_text("{}\n", encoding="utf-8")


def make_tool_bin(sandbox: Path, tools: Iterable[Path]) -> Path:
    root = sandbox / "bin"
    root.mkdir(mode=0o700)
    for tool in tools:
        resolved = tool.resolve(strict=True)
        destination = root / tool.name
        if destination.exists():
            raise EvidenceError(f"duplicate isolated tool name: {tool.name}")
        destination.symlink_to(resolved)
    return root


def run(args: argparse.Namespace) -> dict[str, Any]:
    repository_root = Path(__file__).resolve().parents[1]
    require_generator_at_commit(args.git, repository_root, args.source_commit)
    sandbox = args.sandbox.resolve()
    if sandbox.exists():
        raise EvidenceError("sandbox must not exist before the lifecycle")
    if sandbox == Path.home().resolve() or sandbox == Path("/"):
        raise EvidenceError("refusing an unsafe sandbox root")
    output = args.output.resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    gh_env = dict(os.environ)
    before = observe_upstream_head(
        args.gh,
        repository=args.upstream_repository,
        pr_number=args.upstream_pr,
        expected_head=args.expected_head,
        env=gh_env,
    )

    sandbox.mkdir(mode=0o700)
    try:
        for name in ("home", "state", "config", "cache", "tmp", "workspace"):
            (sandbox / name).mkdir(mode=0o700)
        seed_clients(sandbox)
        tool_bin = make_tool_bin(
            sandbox, (args.agentplugins, args.claude, args.node, args.npx, args.git)
        )
        env = scrub_client_env({
            "PATH": os.pathsep.join((str(tool_bin), "/usr/bin", "/bin")),
            "LANG": "C.UTF-8",
            "LC_ALL": "C.UTF-8",
            "HOME": str(sandbox / "home"),
            "USERPROFILE": str(sandbox / "home"),
            "XDG_CONFIG_HOME": str(sandbox / "config"),
            "XDG_CACHE_HOME": str(sandbox / "cache"),
            "AGENTPLUGINS_HOME": str(sandbox / "state"),
            "TMPDIR": str(sandbox / "tmp"),
            "GIT_CONFIG_GLOBAL": str(sandbox / "home" / ".gitconfig"),
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_TERMINAL_PROMPT": "0",
            "CI": "true",
        })
        source = f"{args.upstream_repository}@{args.expected_head}"
        common = ["--target", TARGETS, "--format", "json"]
        cli = str(args.agentplugins.resolve(strict=True))
        workspace = sandbox / "workspace"

        claude_version = subprocess.run(
            [str(args.claude.resolve(strict=True)), "--version"],
            env=scrub_client_env(env),
            cwd=workspace,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=True,
            timeout=30,
        ).stdout.strip()
        if not claude_version.startswith(args.claude_version):
            raise EvidenceError("installed Claude CLI version is not exact")

        lifecycle_started = utc_now()
        dry_run, _, _ = checked_json(
            [cli, "add", source, *common, "--dry-run"], env=env, cwd=workspace
        )
        identity = validate_dry_run(dry_run, args.expected_head)

        add, _, _ = checked_json([cli, "add", source, *common], env=env, cwd=workspace)
        installed_artifact_id = validate_add(add, args.expected_head, identity)

        claude_list = claude_plugin_list(args.claude, env=env, cwd=workspace)
        validate_claude_plugins(
            claude_list, installed=True, version=str(add["data"]["version"])
        )

        info, _, _ = checked_json(
            [cli, "info", "chrome-devtools", *common], env=env, cwd=workspace
        )
        validate_info(info, args.expected_head)

        doctor, _, _ = checked_json(
            [cli, "doctor", "chrome-devtools", *common], env=env, cwd=workspace
        )
        validate_doctor(doctor, post_remove=False)

        immutable_before = snapshot_roots({"sandbox": sandbox})
        update, update_stderr, _ = checked_json(
            [cli, "update", "chrome-devtools", *common],
            env=env,
            cwd=workspace,
            expect_success=False,
        )
        update_data = update.get("data")
        if (
            update.get("command") != "update"
            or update.get("result") != "failure"
            or not isinstance(update_data, dict)
            or update_data.get("status") != "preflight_failed"
            or update_data.get("succeeded") != 0
            or update_data.get("failed") != 5
            or update_data.get("targets") != []
            or "direct full-SHA installations require explicit switch" not in update_stderr
        ):
            raise EvidenceError("immutable full-SHA update did not fail closed")
        immutable_after = snapshot_roots({"sandbox": sandbox})
        if immutable_before != immutable_after:
            raise EvidenceError("failed immutable update changed an isolated writable root")

        repair, _, _ = checked_json(
            [cli, "repair", "chrome-devtools", *common], env=env, cwd=workspace
        )
        _, repair_targets = validate_batch(
            repair, command="repair", expected_status="completed"
        )
        if any(item.get("status") != "external_completed" for item in repair_targets.values()):
            raise EvidenceError("repair did not externally complete all five targets")

        remove, _, _ = checked_json(
            [cli, "remove", "chrome-devtools", *common, "--purge-data"],
            env=env,
            cwd=workspace,
        )
        remove_data, remove_targets = validate_batch(
            remove, command="remove", expected_status="completed"
        )
        if (
            remove_data.get("plugin_data_preserved") is not False
            or remove_data.get("data_retained") is not False
            or any(item.get("status") != "external_completed" for item in remove_targets.values())
        ):
            raise EvidenceError("remove did not purge owned data for all five targets")

        claude_after = claude_plugin_list(args.claude, env=env, cwd=workspace)
        validate_claude_plugins(claude_after, installed=False, version="")

        listed, _, _ = checked_json([cli, "list", "--format", "json"], env=env, cwd=workspace)
        list_data = require_success(listed, "list")
        if list_data.get("installations") != []:
            raise EvidenceError("list still reports an installation after removal")

        post_doctor, _, _ = checked_json(
            [cli, "doctor", "--format", "json"], env=env, cwd=workspace
        )
        validate_doctor(post_doctor, post_remove=True)
        ensure_no_managed_residue(sandbox)
        lifecycle_completed = utc_now()

        after = observe_upstream_head(
            args.gh,
            repository=args.upstream_repository,
            pr_number=args.upstream_pr,
            expected_head=args.expected_head,
            env=gh_env,
        )
        if any(
            before[field] != after[field]
            for field in ("url", "commit_sha", "git_tree_sha", "commit_timestamp_utc")
        ):
            raise EvidenceError("upstream source identity changed across the lifecycle")

        source_evidence = {
            "repository": args.upstream_repository,
            "pr_number": args.upstream_pr,
            "pr_url": before["url"],
            "package_path": ".",
            "commit_sha": args.expected_head,
            "git_tree_sha": before["git_tree_sha"],
            "commit_timestamp_utc": before["commit_timestamp_utc"],
            "head_observed_at_utc": before["observed_at_utc"],
            "lifecycle_started_at_utc": rfc3339(lifecycle_started),
            "lifecycle_completed_at_utc": rfc3339(lifecycle_completed),
            "head_rechecked_at_utc": after["observed_at_utc"],
        }
        validate_timestamp_order(source_evidence)

        digest = binary_digest(
            sandbox / "cache", args.agentplugins_version, args.agentplugins_package_root
        )
        package_version = str(add["data"]["version"])
        evidence: dict[str, Any] = {
            "schema_version": 1,
            "evidence_type": "chrome_five_client_manager_lifecycle_v1",
            "client": "Agent Plugins CLI multi-client lifecycle",
            "date": lifecycle_completed.astimezone(ZoneInfo("Europe/Kyiv")).date().isoformat(),
            "date_timezone": "Europe/Kyiv",
            "observed_at_utc": rfc3339(lifecycle_completed),
            "outcome": "passed",
            "product_id": "chrome-devtools",
            "distribution_kind": "direct",
            "package_version": package_version,
            "installer_version": args.agentplugins_version,
            "adapter_version": f"agentplugins-{args.agentplugins_version}",
            "binary_digest": digest,
            "immutable_source_revision": args.expected_head,
            "source": source_evidence,
            "installer": {
                "npm_package": "universal-agent-plugins",
                "version": args.agentplugins_version,
                "npm_integrity": args.agentplugins_npm_integrity,
                "binary_digest": digest,
            },
            "claude_cli": {
                "npm_package": "@anthropic-ai/claude-code",
                "version": args.claude_version,
                "npm_integrity": args.claude_npm_integrity,
                "plugin_id": "chrome-devtools@skills-dir",
                "plugin_discovered_after_add": True,
                "plugin_absent_after_remove": True,
            },
            "package": {
                "source_selector": f"{args.upstream_repository}@{args.expected_head}",
                "revision": args.expected_head,
                "git_tree_sha": before["git_tree_sha"],
                "tree_digest": identity["tree_digest"],
                "manifest_digest": identity["manifest_digest"],
                "physical_artifact_id": installed_artifact_id,
                "one_acquisition_per_multi_target_operation": True,
            },
            "targets": [
                {
                    "client_id": client,
                    "package_mode": EXPECTED_MODES[client],
                    "claim_class": (
                        "claude_cli_plugin_discovery"
                        if client == "claude"
                        else "manager_projection_and_configuration_only"
                    ),
                    "add": "passed",
                    "info": "passed",
                    "repair": "passed",
                    "remove": "passed",
                }
                for client in CLIENTS
            ],
            "operations": {
                "dry_run": "passed",
                "add": "passed_5_of_5",
                "info": "passed_5_of_5",
                "doctor": "passed_read_only_with_authentication_unchecked",
                "immutable_update": {
                    "outcome": "expected_failure",
                    "reason": "direct_full_sha_requires_explicit_switch",
                    "targets_changed": 0,
                    "all_writable_roots_snapshot_before": immutable_before,
                    "all_writable_roots_snapshot_after": immutable_after,
                    "byte_identical": True,
                },
                "repair": "passed_5_of_5",
                "remove": "passed_5_of_5_with_owned_data_purged",
                "list": "zero_installations",
                "post_doctor": "healthy",
            },
            "checks": [
                {"scenario": "resolve one exact upstream package for five targets", "operation": "multi_target_dry_run", "status": "passed"},
                {"scenario": "install and inspect five manager-owned client projections", "operation": "add_info_doctor", "status": "passed"},
                {"scenario": "discover the installed package through the real Claude CLI", "operation": "claude_plugin_list", "status": "passed"},
                {"scenario": "reject immutable direct-source update without changing bytes", "operation": "update", "status": "passed"},
                {"scenario": "repair, remove, purge owned data, and verify zero managed residue", "operation": "repair_remove", "status": "passed"},
            ],
            "scope": {
                "proved": [
                    "exact_upstream_agent_plugins_package_resolution",
                    "one_command_multi_target_install",
                    "manager_add_info_doctor_repair_remove_lifecycle",
                    "client_specific_projection_and_configuration",
                    "claude_cli_plugin_discovery_and_removal",
                    "immutable_update_zero_mutation",
                ],
                "not_proved": [
                    "gemini_cli_runtime",
                    "opencode_runtime",
                    "cline_runtime",
                    "windsurf_runtime",
                    "live_model_tool_invocation",
                    "chrome_browser_runtime_control",
                    "oauth_flow",
                    "directory_short_alias_resolution",
                ],
            },
            "cleanup": {
                "managed_installations_remaining": 0,
                "owned_plugin_data_remaining": 0,
                "claude_plugin_discovered_after_remove": False,
                "named_managed_artifacts_remaining": 0,
                "sandbox_removed_after_evidence": not args.keep_sandbox,
            },
            "run": {
                "repository": args.run_repository,
                "run_id": args.run_id,
                "run_attempt": args.run_attempt,
                "source_commit": args.source_commit,
            },
            "secrets_recorded": False,
            "real_user_project_used": False,
            "privacy": {"sanitized": True, "excluded": list(PRIVACY_EXCLUSIONS)},
        }
        ensure_sanitized(evidence, sandbox)
        return evidence
    finally:
        if not args.keep_sandbox:
            shutil.rmtree(sandbox, ignore_errors=False)


def claude_plugin_list(
    claude: Path, *, env: dict[str, str], cwd: Path
) -> list[dict[str, Any]]:
    completed = subprocess.run(
        [str(claude.resolve(strict=True)), "plugin", "list", "--json"],
        env=scrub_client_env(env),
        cwd=cwd,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=60,
        check=False,
    )
    if completed.returncode != 0:
        raise EvidenceError(f"Claude plugin list failed: {completed.stderr.strip()[:1000]}")
    try:
        value = json.loads(completed.stdout)
    except json.JSONDecodeError as error:
        raise EvidenceError("Claude plugin list did not emit JSON") from error
    if not isinstance(value, list) or any(not isinstance(item, dict) for item in value):
        raise EvidenceError("Claude plugin list JSON is malformed")
    return value


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser()
    parser.add_argument("--agentplugins", type=Path, required=True)
    parser.add_argument("--agentplugins-package-root", type=Path, required=True)
    parser.add_argument("--agentplugins-version", default="0.1.24")
    parser.add_argument("--agentplugins-npm-integrity", required=True)
    parser.add_argument("--claude", type=Path, required=True)
    parser.add_argument("--claude-version", default="2.1.251")
    parser.add_argument("--claude-npm-integrity", required=True)
    parser.add_argument("--node", type=Path, required=True)
    parser.add_argument("--npx", type=Path, required=True)
    parser.add_argument("--git", type=Path, required=True)
    parser.add_argument("--gh", type=Path, required=True)
    parser.add_argument("--upstream-repository", default="ChromeDevTools/chrome-devtools-mcp")
    parser.add_argument("--upstream-pr", type=int, default=2623)
    parser.add_argument("--expected-head", required=True)
    parser.add_argument("--sandbox", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--run-repository", default="local")
    parser.add_argument("--run-id", default="local")
    parser.add_argument("--run-attempt", default="1")
    parser.add_argument("--source-commit", required=True)
    parser.add_argument("--keep-sandbox", action="store_true")
    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    try:
        evidence = run(args)
        args.output.write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n")
    except (EvidenceError, OSError, subprocess.SubprocessError, json.JSONDecodeError) as error:
        print(f"chrome five-client lifecycle failed: {error}", file=sys.stderr)
        return 1
    print("OK: sanitized Chrome DevTools five-client lifecycle evidence produced")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
