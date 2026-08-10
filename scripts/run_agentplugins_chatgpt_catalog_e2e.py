#!/usr/bin/env python3
"""Exercise catalog v2 through the released ChatGPT package projection."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import subprocess
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urlsplit


ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "v2" / "catalog.json"
PLUGIN = "cloudflare-docs"
TARGET = "chatgpt"
EXPECTED_CLI_VERSION = "0.1.6"
CLI_TIMEOUT_SECONDS = 120
SEMVER_PATTERN = re.compile(
    r"(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)"
    r"(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?"
    r"(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?"
)
CATALOG_DIGEST_PATTERN = re.compile(r"sha256:[0-9a-f]{64}")
CATALOG_REVISION_PATTERN = re.compile(r"[0-9a-f]{40}")


def verified_catalog(catalog_digest: str) -> tuple[dict[str, object], dict[str, object]]:
    """Bind the local v2 catalog and selected plugin to exact public facts."""
    if not CATALOG_DIGEST_PATTERN.fullmatch(catalog_digest):
        raise ValueError("catalog digest must be lowercase sha256:<64 hex>")
    body = CATALOG.read_bytes()
    actual = "sha256:" + hashlib.sha256(body).hexdigest()
    if catalog_digest != actual:
        raise ValueError(f"catalog digest does not match the local catalog: expected {actual}")
    catalog = json.loads(body)
    if catalog.get("schema_version") != 2:
        raise ValueError("ChatGPT lifecycle E2E requires catalog schema v2")
    revision = catalog.get("revision")
    if not isinstance(revision, str) or not CATALOG_REVISION_PATTERN.fullmatch(revision):
        raise ValueError("local catalog revision must be a full lowercase commit SHA")
    matches = [entry for entry in catalog.get("plugins", []) if entry.get("name") == PLUGIN]
    if len(matches) != 1:
        raise ValueError(f"catalog must contain exactly one {PLUGIN} entry")
    entry = matches[0]
    if entry.get("minimum_cli_version") != EXPECTED_CLI_VERSION:
        raise ValueError(f"{PLUGIN}: catalog v2 must require CLI {EXPECTED_CLI_VERSION}")
    compatibility = entry.get("compatibility", {}).get(TARGET, {})
    binding = compatibility.get("app_binding")
    if compatibility.get("package") != "projected" or not isinstance(binding, dict):
        raise ValueError(f"{PLUGIN}: catalog v2 omitted the ChatGPT projection binding")
    return catalog, binding


def catalog_environment(catalog_url: str, catalog_digest: str) -> dict[str, str]:
    parsed = urlsplit(catalog_url)
    if (
        parsed.scheme != "https"
        or not parsed.hostname
        or parsed.username is not None
        or parsed.password is not None
        or parsed.query
        or parsed.fragment
    ):
        raise ValueError(
            "catalog URL must be public HTTPS without credentials, query, or fragment"
        )
    if not CATALOG_DIGEST_PATTERN.fullmatch(catalog_digest):
        raise ValueError("catalog digest must be lowercase sha256:<64 hex>")
    return {
        "AGENTPLUGINS_CATALOG_URL": catalog_url,
        "AGENTPLUGINS_CATALOG_DIGEST": catalog_digest,
    }


def isolated_environment(
    sandbox: Path, catalog_url: str, catalog_digest: str
) -> dict[str, str]:
    """Build an isolated process environment without host credentials or clients."""
    allowed = (
        "PATH",
        "LANG",
        "LC_ALL",
        "LC_CTYPE",
        "TZ",
        "SYSTEMROOT",
        "WINDIR",
        "COMSPEC",
        "PATHEXT",
        "SSL_CERT_FILE",
        "SSL_CERT_DIR",
        "NODE_EXTRA_CA_CERTS",
    )
    environment = {name: os.environ[name] for name in allowed if name in os.environ}
    home = sandbox / "home"
    temporary = sandbox / "tmp"
    home.mkdir(parents=True)
    temporary.mkdir(parents=True)
    environment.update(
        {
            "HOME": str(home),
            "USERPROFILE": str(home),
            "XDG_CONFIG_HOME": str(sandbox / "config"),
            "XDG_CACHE_HOME": str(sandbox / "cache"),
            "APPDATA": str(sandbox / "appdata"),
            "LOCALAPPDATA": str(sandbox / "local-appdata"),
            "AGENTPLUGINS_HOME": str(sandbox / "state"),
            "TMPDIR": str(temporary),
            "TMP": str(temporary),
            "TEMP": str(temporary),
            "GIT_CONFIG_GLOBAL": str(home / ".gitconfig"),
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_TERMINAL_PROMPT": "0",
            "CI": "true",
        }
    )
    environment.update(catalog_environment(catalog_url, catalog_digest))
    return environment


def sanitized_output(value: str, sandbox: Path, environment: dict[str, str]) -> str:
    result = (value or "").strip() or "<empty>"
    paths = {str(sandbox), str(sandbox.resolve())}
    for name in (
        "HOME",
        "USERPROFILE",
        "XDG_CONFIG_HOME",
        "XDG_CACHE_HOME",
        "APPDATA",
        "LOCALAPPDATA",
        "AGENTPLUGINS_HOME",
        "TMPDIR",
        "TMP",
        "TEMP",
    ):
        if environment.get(name):
            paths.add(environment[name])
    for path in sorted(paths, key=len, reverse=True):
        result = result.replace(path, "<path>")
    result = re.sub(r"(?i)(https?://)[^/@\s]+:[^/@\s]+@", r"\1<credentials>@", result)
    result = re.sub(r"(?i)\bBearer\s+[^\s,;]+", "Bearer <redacted>", result)
    result = re.sub(r"(?i)([?&#](?:code|state)=)[^&#\s]*", r"\1<redacted>", result)
    return result[:1000]


def command(
    argv: list[str], sandbox: Path, environment: dict[str, str], *, success: bool
) -> subprocess.CompletedProcess[str]:
    try:
        completed = subprocess.run(
            argv,
            cwd=sandbox,
            env=environment,
            check=False,
            capture_output=True,
            text=True,
            timeout=CLI_TIMEOUT_SECONDS,
        )
    except subprocess.TimeoutExpired as error:
        raise RuntimeError(f"command timed out after {CLI_TIMEOUT_SECONDS}s") from error
    if success and completed.returncode != 0:
        detail = sanitized_output(completed.stderr, sandbox, environment)
        raise RuntimeError(f"agentplugins command failed: {detail}")
    if not success and completed.returncode == 0:
        raise RuntimeError("agentplugins command unexpectedly succeeded")
    return completed


def agentplugins_json(
    binary: Path,
    action: str,
    sandbox: Path,
    environment: dict[str, str],
    *extra: str,
) -> dict[str, object]:
    completed = command(
        [
            str(binary),
            action,
            PLUGIN,
            "--target",
            TARGET,
            *extra,
            "--format",
            "json",
        ],
        sandbox,
        environment,
        success=True,
    )
    value = json.loads(completed.stdout)
    if value.get("schema_version") != 1 or value.get("command") != action:
        raise RuntimeError(f"{action}: invalid JSON envelope")
    return value


def result(value: dict[str, object]) -> dict[str, object]:
    data = value.get("data")
    if not isinstance(data, dict) or not isinstance(data.get("result"), dict):
        raise RuntimeError("agentplugins JSON omitted data.result")
    return data["result"]


def client_binding(state: dict[str, object]) -> dict[str, object]:
    matches = [
        installation
        for installation in state.get("installations", [])
        if installation.get("declared_name") == PLUGIN
    ]
    if len(matches) != 1:
        raise RuntimeError("state omitted the unique Cloudflare Docs installation")
    clients = [
        binding
        for binding in matches[0].get("clients", {}).values()
        if binding.get("client_id") == TARGET
    ]
    if len(clients) != 1:
        raise RuntimeError("state omitted the unique ChatGPT client binding")
    return clients[0]


def tree_snapshot(root: Path) -> dict[str, str]:
    if not root.is_dir() or root.is_symlink():
        raise RuntimeError("managed ChatGPT target must be a real directory")
    snapshot: dict[str, str] = {}
    for path in sorted(root.rglob("*")):
        relative = path.relative_to(root).as_posix()
        if path.is_symlink():
            raise RuntimeError(f"managed ChatGPT package contains a symlink: {relative}")
        if path.is_dir():
            snapshot[relative + "/"] = "directory"
        else:
            snapshot[relative] = hashlib.sha256(path.read_bytes()).hexdigest()
    return snapshot


def validate_projection(target: Path, binding: dict[str, object]) -> None:
    snapshot = tree_snapshot(target)
    required = {".codex-plugin/plugin.json", ".app.json", ".mcp.json"}
    if not required.issubset(snapshot):
        raise RuntimeError("ChatGPT projection omitted an official package component")
    if "plugin.json" in snapshot or "mcp.json" in snapshot:
        raise RuntimeError("ChatGPT projection retained a portable root manifest")
    if "hooks/" in snapshot or "hooks/hooks.json" in snapshot:
        raise RuntimeError("ChatGPT projection retained executable hooks")
    manifest = json.loads((target / ".codex-plugin" / "plugin.json").read_text())
    if (
        manifest.get("name") != PLUGIN
        or manifest.get("apps") != "./.app.json"
        or manifest.get("mcpServers") != "./.mcp.json"
        or "hooks" in manifest
    ):
        raise RuntimeError("generated official manifest does not bind app and MCP files")
    expected_app = {"apps": {binding["app_key"]: {"id": binding["id"]}}}
    if json.loads((target / ".app.json").read_text()) != expected_app:
        raise RuntimeError("generated .app.json does not match catalog v2 evidence")
    expected_mcp = {
        "mcpServers": {
            binding["mcp_server"]: {"url": binding["mcp_url"], "type": "http"}
        }
    }
    if json.loads((target / ".mcp.json").read_text()) != expected_mcp:
        raise RuntimeError("generated .mcp.json does not match the registered endpoint")


def run(
    binary: Path,
    expected_version: str,
    catalog_url: str,
    catalog_digest: str,
) -> dict[str, object]:
    if expected_version != EXPECTED_CLI_VERSION:
        raise ValueError(
            f"ChatGPT catalog v2 E2E requires released CLI {EXPECTED_CLI_VERSION}"
        )
    catalog, app_binding = verified_catalog(catalog_digest)
    with tempfile.TemporaryDirectory(prefix="agentplugins-chatgpt-catalog-e2e-") as temporary:
        sandbox = Path(temporary)
        environment = isolated_environment(sandbox, catalog_url, catalog_digest)
        version_output = command(
            [str(binary), "version"], sandbox, environment, success=True
        ).stdout.strip()
        if version_output != f"agentplugins {expected_version}":
            raise RuntimeError(f"unexpected agentplugins version: {version_output!r}")

        state_path = sandbox / "state" / "state-v2.json"
        managed_root = sandbox / "state" / "managed"
        bad_environment = dict(environment)
        bad_environment["AGENTPLUGINS_CATALOG_DIGEST"] = "sha256:" + "0" * 64
        rejected_digest = command(
            [str(binary), "add", PLUGIN, "--target", TARGET, "--dry-run"],
            sandbox,
            bad_environment,
            success=False,
        )
        if "catalog checksum mismatch" not in rejected_digest.stderr.lower():
            raise RuntimeError("bad catalog digest failed for an unexpected reason")
        if state_path.exists() or managed_root.exists():
            raise RuntimeError("bad catalog digest mutated managed state")

        dry_run = result(
            agentplugins_json(binary, "add", sandbox, environment, "--dry-run")
        )
        plan = dry_run.get("plan", {})
        components = {
            (component.get("kind"), component.get("support"))
            for component in plan.get("components", [])
        }
        if (
            dry_run.get("mutated") is not False
            or plan.get("client_id") != TARGET
            or plan.get("status") != "manual_activation_required"
            or components != {("mcp_server", "projected"), ("app", "projected")}
        ):
            raise RuntimeError("ChatGPT dry-run omitted the exact non-mutating plan")
        if state_path.exists() or managed_root.exists():
            raise RuntimeError("ChatGPT dry-run mutated managed state")

        added = result(agentplugins_json(binary, "add", sandbox, environment))
        activation = added.get("activation", {})
        if (
            added.get("mutated") is not True
            or activation.get("activation") != "manual_activation_required"
            or activation.get("authentication") != "not_required"
            or activation.get("verification") != "package_validated"
        ):
            raise RuntimeError("ChatGPT add reported an invalid lifecycle state")
        state = json.loads(state_path.read_text())
        if state.get("schema_version") != 3:
            raise RuntimeError("ChatGPT catalog v2 install did not persist State v3")
        binding = client_binding(state)
        revision = binding.get("package_revision", {})
        evidence = revision.get("catalog_evidence", {})
        persisted_app = evidence.get("compatibility", {}).get(TARGET, {}).get("app_binding")
        if (
            evidence.get("schema_version") != 2
            or evidence.get("digest") != catalog_digest
            or persisted_app != app_binding
        ):
            raise RuntimeError("State v3 did not retain exact per-client catalog evidence")
        target = Path(binding["target_locator"])
        expected_root = (sandbox / "state" / "managed" / "clients" / TARGET).resolve()
        if target.resolve().parent != expected_root:
            raise RuntimeError("ChatGPT projection escaped its isolated managed root")
        validate_projection(target, app_binding)
        original_tree = tree_snapshot(target)

        (target / ".app.json").unlink()
        repaired = result(agentplugins_json(binary, "repair", sandbox, environment))
        if repaired.get("mutated") is not True:
            raise RuntimeError("repair did not restore the missing ChatGPT app mapping")
        validate_projection(target, app_binding)
        if tree_snapshot(target) != original_tree:
            raise RuntimeError("repair did not restore the exact managed package tree")

        state_before_rejected_remove = state_path.read_bytes()
        tree_before_rejected_remove = tree_snapshot(target)
        blocked_remove = result(
            agentplugins_json(binary, "remove", sandbox, environment)
        )
        blocked_deactivation = blocked_remove.get("deactivation", {})
        if (
            blocked_remove.get("mutated") is not False
            or blocked_deactivation.get("artifact_removal_allowed") is not False
            or not any(
                "--external-uninstalled" in action
                for action in blocked_deactivation.get("user_actions", [])
            )
        ):
            raise RuntimeError("remove bypassed the required external-uninstall boundary")
        if (
            state_path.read_bytes() != state_before_rejected_remove
            or tree_snapshot(target) != tree_before_rejected_remove
        ):
            raise RuntimeError("rejected ChatGPT remove changed state or package files")

        removed = result(
            agentplugins_json(
                binary, "remove", sandbox, environment, "--external-uninstalled"
            )
        )
        deactivation = removed.get("deactivation", {})
        if (
            removed.get("mutated") is not True
            or deactivation.get("external_removal_complete") is not True
            or target.exists()
        ):
            raise RuntimeError("confirmed ChatGPT remove did not clean the managed target")
        final_binding = client_binding(json.loads(state_path.read_text()))
        receipts = final_binding.get("receipts", [])
        if (
            final_binding.get("materialization") != "absent"
            or [receipt.get("sequence") for receipt in receipts] != [1, 2, 3]
            or any(receipt.get("phase") != "committed" for receipt in receipts)
        ):
            raise RuntimeError("ChatGPT lifecycle receipts did not converge to absent")

    observed = datetime.now(timezone.utc).replace(microsecond=0)
    return {
        "client": "agentplugins CLI isolated ChatGPT catalog v2 projection",
        "version": expected_version,
        "date": observed.date().isoformat(),
        "observed_at_utc": observed.isoformat().replace("+00:00", "Z"),
        "catalog_revision": catalog["revision"],
        "catalog_digest": catalog_digest,
        "checks": [
            {
                "scenario": "reject a mismatched catalog v2 digest without mutation",
                "status": "passed",
            },
            {"scenario": "render an exact non-mutating ChatGPT dry-run plan", "status": "passed"},
            {
                "scenario": "project app and MCP bindings into an official package",
                "status": "passed",
            },
            {
                "scenario": "persist State v3 catalog evidence per client revision",
                "status": "passed",
            },
            {
                "scenario": "repair an exact missing app binding from pinned evidence",
                "status": "passed",
            },
            {
                "scenario": "require external uninstall confirmation before removal",
                "status": "passed",
            },
            {"scenario": "remove the isolated package and converge to absent", "status": "passed"},
            {
                "scenario": "ChatGPT UI activation, package-routed tool runtime, and OAuth",
                "status": "skipped",
                "reason": (
                    "isolated package lifecycle does not launch ChatGPT or attest "
                    "UI, tool, or OAuth runtime"
                ),
            },
        ],
        "scope": {
            "proved": [
                "released_cli_catalog_v2_consumption",
                "chatgpt_official_package_projection",
                "state_v3_per_client_evidence",
                "transactional_repair_and_remove",
            ],
            "not_proved": [
                "chatgpt_work_ui_activation",
                "package_routed_tool_runtime",
                "oauth_runtime",
            ],
        },
        "secrets_recorded": False,
        "real_user_project_used": False,
        "privacy": {
            "sanitized": True,
            "excluded": [
                "credentials",
                "tokens",
                "cookies",
                "OAuth codes",
                "OAuth state",
                "account identifiers",
                "authorization URLs",
                "absolute temporary paths",
            ],
        },
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--expected-version", default=EXPECTED_CLI_VERSION)
    parser.add_argument("--catalog-url", required=True)
    parser.add_argument("--catalog-digest", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    if not SEMVER_PATTERN.fullmatch(args.expected_version):
        parser.error("--expected-version must be a semantic version")
    evidence = run(
        args.binary.resolve(),
        args.expected_version,
        args.catalog_url,
        args.catalog_digest,
    )
    body = json.dumps(evidence, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(body)
    else:
        print(body, end="")
    print("OK: released ChatGPT catalog v2 lifecycle", file=__import__("sys").stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
