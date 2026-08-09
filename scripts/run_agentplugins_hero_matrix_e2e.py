#!/usr/bin/env python3
"""Exercise five hero packages through five disposable client projections."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import subprocess
import sys
import tempfile
from datetime import date, datetime, timezone
from pathlib import Path
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "v1" / "catalog.json"
HERO_PLUGINS = (
    "context7",
    "cloudflare-docs",
    "agent-code-navigator",
    "notion",
    "chrome-devtools",
)
CLIENTS = ("codex", "cursor", "copilot", "vscode", "kiro")
COPIED_CLIENTS = {"codex", "copilot", "vscode", "kiro"}
CLI_TIMEOUT_SECONDS = 120
EXPECTED_CLI_VERSION = "0.1.5"
SEMVER_PATTERN = re.compile(
    r"(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)"
    r"(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?"
    r"(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?"
)
CATALOG_DIGEST_PATTERN = re.compile(r"sha256:[0-9a-f]{64}")
CATALOG_REVISION_PATTERN = re.compile(r"[0-9a-f]{40}")


def verified_catalog(catalog_digest: str) -> tuple[dict[str, object], str]:
    """Load the exact local catalog after binding it to the supplied digest."""
    if not CATALOG_DIGEST_PATTERN.fullmatch(catalog_digest):
        raise ValueError("catalog digest must be lowercase sha256:<64 hex>")
    body = CATALOG.read_bytes()
    actual_digest = "sha256:" + hashlib.sha256(body).hexdigest()
    if catalog_digest != actual_digest:
        raise ValueError(
            f"catalog digest does not match the local catalog: expected {actual_digest}"
        )
    catalog = json.loads(body)
    revision = catalog.get("revision")
    if not isinstance(revision, str) or not CATALOG_REVISION_PATTERN.fullmatch(
        revision
    ):
        raise ValueError("local catalog revision must be a full lowercase commit SHA")
    return catalog, actual_digest


def prepare_client(
    home: Path,
    target: str,
    environment: dict[str, str],
    platform_name: str | None = None,
) -> tuple[Path, ...]:
    """Create isolated detection roots for the selected client."""
    roots = {
        "codex": (home / ".codex",),
        "cursor": (home / ".cursor",),
        "copilot": (home / ".copilot",),
        "vscode": (
            vscode_detection_root(home, environment, platform_name or sys.platform),
        ),
        "kiro": (home / ".kiro",),
    }
    prepared = roots[target]
    for root in prepared:
        root.mkdir(parents=True, exist_ok=True)
        if not root.is_dir() or root.is_symlink():
            raise RuntimeError(f"{target}: client detection root is not a real directory")
    return prepared


def vscode_detection_root(
    home: Path, environment: dict[str, str], platform_name: str
) -> Path:
    """Map one platform to the VS Code user root used by client detection."""
    if platform_name == "win32":
        return Path(environment["APPDATA"]) / "Code" / "User"
    if platform_name == "darwin":
        return home / "Library" / "Application Support" / "Code" / "User"
    return Path(environment["XDG_CONFIG_HOME"]) / "Code" / "User"


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
    sandbox: Path,
    home: Path,
    catalog_url: str,
    catalog_digest: str,
) -> dict[str, str]:
    """Build a client environment without inheriting host credentials."""
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
    temp_dir = sandbox / "tmp"
    temp_dir.mkdir(parents=True, exist_ok=True)
    environment.update(
        {
            "HOME": str(home),
            "USERPROFILE": str(home),
            "XDG_CONFIG_HOME": str(sandbox / "config"),
            "XDG_CACHE_HOME": str(sandbox / "cache"),
            "APPDATA": str(sandbox / "appdata"),
            "LOCALAPPDATA": str(sandbox / "local-appdata"),
            "AGENTPLUGINS_HOME": str(sandbox / "state"),
            "TMPDIR": str(temp_dir),
            "TMP": str(temp_dir),
            "TEMP": str(temp_dir),
            "GIT_CONFIG_GLOBAL": str(home / ".gitconfig"),
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_TERMINAL_PROMPT": "0",
            "CI": "true",
        }
    )
    environment.update(catalog_environment(catalog_url, catalog_digest))
    return environment


def binary_version(
    binary: Path,
    sandbox: Path,
    environment: dict[str, str],
    expected_version: str,
) -> str:
    completed = subprocess.run(
        [str(binary), "version"],
        cwd=sandbox,
        env=environment,
        check=True,
        capture_output=True,
        text=True,
        timeout=CLI_TIMEOUT_SECONDS,
    )
    prefix = "agentplugins "
    value = completed.stdout.strip()
    version = value.removeprefix(prefix).strip() if value.startswith(prefix) else ""
    if not SEMVER_PATTERN.fullmatch(version):
        raise RuntimeError(f"unexpected agentplugins version output: {value!r}")
    if version != expected_version:
        raise RuntimeError(
            f"agentplugins version {version!r} does not match expected {expected_version!r}"
        )
    return version


def run_cli(
    binary: Path,
    command: str,
    plugin: str,
    target: str,
    sandbox: Path,
    environment: dict[str, str],
) -> subprocess.CompletedProcess[str]:
    """Run one non-interactive package projection without confirmation flags."""
    extra = (
        ["--external-uninstalled"]
        if command == "remove" and target in COPIED_CLIENTS
        else []
    )
    argv = [
        str(binary),
        command,
        plugin,
        "--target",
        target,
        "--format",
        "json",
        *extra,
    ]
    try:
        return subprocess.run(
            argv,
            cwd=sandbox,
            env=environment,
            check=True,
            capture_output=True,
            text=True,
            timeout=CLI_TIMEOUT_SECONDS,
        )
    except subprocess.TimeoutExpired:
        raise RuntimeError(
            f"{target}/{plugin}: {command} timed out after {CLI_TIMEOUT_SECONDS}s"
        ) from None
    except subprocess.CalledProcessError as error:
        stderr = sanitized_failure_output(error.stderr, sandbox, environment)
        raise RuntimeError(
            f"{target}/{plugin}: {command} failed with exit {error.returncode}; "
            f"stderr: {stderr}"
        ) from None


def sanitized_failure_output(
    stderr: str | None, sandbox: Path, environment: dict[str, str]
) -> str:
    """Keep failure context while excluding credentials and machine paths."""
    value = (stderr or "").strip() or "<empty>"
    path_values = {str(sandbox), str(sandbox.resolve())}
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
            path_values.add(environment[name])
    for path in sorted(path_values, key=len, reverse=True):
        value = value.replace(path, "<path>")
    value = re.sub(r"(?i)\bfile:///(?:[^\s,;]+)", "<path>", value)
    value = re.sub(r"(?i)(https?://)[^/@\s]+:[^/@\s]+@", r"\1<credentials>@", value)
    value = re.sub(
        r"(?im)(\bauthorization\s*[:=]\s*)[^\r\n]+",
        r"\1<redacted>",
        value,
    )
    value = re.sub(
        r"(?i)\bBearer\s+[^\s,;]+", "Bearer <redacted>", value
    )
    value = re.sub(
        r"(?i)([?&#](?:code|state)=)[^&#\s]*", r"\1<redacted>", value
    )
    value = re.sub(
        r'''(?ix)
        (?P<key>["']?(?:api[_-]?key|token|password|secret|authorization|cookie|
        oauth[_-]?(?:code|state))["']?\s*[:=]\s*)
        (?:"[^"]*"|'[^']*'|[^\s,;]+)
        ''',
        r"\g<key><redacted>",
        value,
    )
    value = re.sub(r"(?<![\w./])(?:/[A-Za-z0-9._+@%=-]+){2,}", "<path>", value)
    value = re.sub(r"(?i)\b[A-Z]:\\(?:[^\s\\]+\\)*[^\s\\]+", "<path>", value)
    return value[:1000]


def run(
    binary: Path,
    expected_version: str,
    catalog_url: str,
    catalog_digest: str,
) -> dict[str, object]:
    catalog, actual_catalog_digest = verified_catalog(catalog_digest)
    available = {entry["name"] for entry in catalog["plugins"]}
    missing = sorted(set(HERO_PLUGINS) - available)
    if missing:
        raise RuntimeError(f"hero plugins missing from catalog: {missing}")
    results: list[dict[str, str]] = []
    versions: set[str] = set()
    for target in CLIENTS:
        with tempfile.TemporaryDirectory(prefix=f"agentplugins-{target}-e2e-") as temporary:
            sandbox = Path(temporary)
            home = sandbox / "home"
            environment = isolated_environment(
                sandbox, home, catalog_url, catalog_digest
            )
            prepare_client(home, target, environment)
            versions.add(
                binary_version(binary, sandbox, environment, expected_version)
            )
            for plugin in HERO_PLUGINS:
                for command in ("add", "remove"):
                    completed = run_cli(
                        binary,
                        command,
                        plugin,
                        target,
                        sandbox,
                        environment,
                    )
                    value = json.loads(completed.stdout)
                    result = value.get("data", {}).get("result", {})
                    if value.get("schema_version") != 1 or value.get("command") != command:
                        raise RuntimeError(f"{target}/{plugin}: invalid {command} JSON envelope")
                    if result.get("mutated") is not True:
                        raise RuntimeError(f"{target}/{plugin}: {command} did not commit")
                results.append({"plugin": plugin, "client": target, "status": "passed"})
            state = json.loads((sandbox / "state" / "state-v2.json").read_text())
            if any(
                client["materialization"] != "absent"
                for installation in state["installations"]
                for client in installation["clients"].values()
            ):
                raise RuntimeError(f"{target}: a package remained materialized")
    if len(versions) != 1:
        raise RuntimeError(f"inconsistent agentplugins versions: {sorted(versions)}")
    observed = datetime.now(timezone.utc).replace(microsecond=0)
    return {
        "client": "agentplugins CLI isolated multi-client package projections",
        "version": versions.pop(),
        "date": date.today().isoformat(),
        "observed_at_utc": observed.isoformat().replace("+00:00", "Z"),
        "catalog_revision": catalog["revision"],
        "catalog_digest": actual_catalog_digest,
        "checks": [
            {
                "scenario": "five hero packages add/remove across five client projections",
                "status": "passed",
                "package_lifecycle_flows": len(results),
                "results": results,
            },
            {
                "scenario": "client process launch, tool runtime, and OAuth",
                "status": "skipped",
                "reason": "package projections do not launch client processes or prove tool or OAuth runtime",
            },
        ],
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
    result = run(
        args.binary.resolve(),
        args.expected_version,
        args.catalog_url,
        args.catalog_digest,
    )
    body = json.dumps(result, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(body)
    else:
        print(body, end="")
    print("OK: 25/25 hero package/client add-remove flows", file=__import__("sys").stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
