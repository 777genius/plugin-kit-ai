#!/usr/bin/env python3
"""Run all catalog add/remove flows in a disposable Cursor provider sandbox."""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urlsplit


ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "v1" / "catalog.json"
CLI_TIMEOUT_SECONDS = 120
EXPECTED_CLI_VERSION = "0.1.5"
SEMVER_PATTERN = re.compile(
    r"(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)"
    r"(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?"
    r"(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?"
)
CATALOG_DIGEST_PATTERN = re.compile(r"sha256:[0-9a-f]{64}")


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
            "HOME": str(sandbox / "home"),
            "USERPROFILE": str(sandbox / "home"),
            "XDG_CONFIG_HOME": str(sandbox / "config"),
            "XDG_CACHE_HOME": str(sandbox / "cache"),
            "AGENTPLUGINS_HOME": str(sandbox / "state"),
            "TMPDIR": str(temp_dir),
            "TMP": str(temp_dir),
            "TEMP": str(temp_dir),
            "GIT_CONFIG_GLOBAL": str(sandbox / "home" / ".gitconfig"),
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_TERMINAL_PROMPT": "0",
            "CI": "true",
        }
    )
    environment.update(catalog_environment(catalog_url, catalog_digest))
    return environment


def run_cli(
    binary: Path,
    command: str,
    name: str,
    sandbox: Path,
    environment: dict[str, str],
    *,
    check: bool,
) -> subprocess.CompletedProcess[str]:
    try:
        return subprocess.run(
            [str(binary), command, name, "--target", "cursor", "--format", "json"],
            cwd=sandbox,
            env=environment,
            check=check,
            capture_output=True,
            text=True,
            timeout=CLI_TIMEOUT_SECONDS,
        )
    except subprocess.TimeoutExpired as error:
        raise RuntimeError(
            f"{name}: {command} timed out after {CLI_TIMEOUT_SECONDS}s"
        ) from error


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


def run(
    binary: Path,
    expected_version: str,
    catalog_url: str,
    catalog_digest: str,
) -> dict[str, object]:
    catalog = json.loads(CATALOG.read_text())
    names = [entry["name"] for entry in catalog["plugins"]]
    with tempfile.TemporaryDirectory(prefix="agentplugins-lifecycle-e2e-") as temporary:
        sandbox = Path(temporary)
        home = sandbox / "home"
        (home / ".cursor").mkdir(parents=True)
        environment = isolated_environment(sandbox, catalog_url, catalog_digest)
        version = binary_version(binary, sandbox, environment, expected_version)
        for name in names:
            for command in ("add", "remove"):
                completed = run_cli(
                    binary, command, name, sandbox, environment, check=True
                )
                value = json.loads(completed.stdout)
                if value.get("schema_version") != 1 or value.get("command") != command:
                    raise RuntimeError(f"{name}: invalid {command} JSON envelope")
                if value.get("data", {}).get("result", {}).get("mutated") is not True:
                    raise RuntimeError(f"{name}: {command} did not commit a mutation")
                state = json.loads((sandbox / "state" / "state-v2.json").read_text())
                installation = next(item for item in state["installations"] if item["declared_name"] == name)
                binding = next(iter(installation["clients"].values()))
                receipt = binding["receipts"][-1]
                expected_sequence = 1 if command == "add" else 2
                if receipt.get("phase") != "committed" or receipt.get("sequence") != expected_sequence:
                    raise RuntimeError(f"{name}: invalid committed {command} receipt")
                if command == "add":
                    if not receipt.get("after_digest", "").startswith("sha256:"):
                        raise RuntimeError(f"{name}: add receipt omitted artifact digest")
                else:
                    add_receipt = binding["receipts"][-2]
                    if receipt.get("before_digest") != add_receipt.get("after_digest"):
                        raise RuntimeError(
                            f"{name}: remove receipt did not preserve the installed digest"
                        )
                    if receipt.get("after_digest") not in (None, ""):
                        raise RuntimeError(
                            f"{name}: remove receipt retained an absent artifact digest"
                        )

        # One negative case proves the remove path checks the currently managed
        # directory digest instead of trusting state alone.
        run_cli(binary, "add", "context7", sandbox, environment, check=True)
        state = json.loads((sandbox / "state" / "state-v2.json").read_text())
        context7 = next(item for item in state["installations"] if item["declared_name"] == "context7")
        binding = next(iter(context7["clients"].values()))
        tamper = Path(binding["target_locator"]) / "unexpected-user-file.txt"
        tamper.write_text("must block removal")
        rejected = run_cli(binary, "remove", "context7", sandbox, environment, check=False)
        if rejected.returncode == 0:
            raise RuntimeError("digest guard accepted a modified managed package")
        if "refusing silent removal" not in rejected.stderr or "artifact digest mismatch" not in rejected.stderr:
            raise RuntimeError("tampered removal failed for an unexpected reason")
        rejected_state = json.loads((sandbox / "state" / "state-v2.json").read_text())
        rejected_context7 = next(
            item for item in rejected_state["installations"] if item["declared_name"] == "context7"
        )
        rejected_binding = next(iter(rejected_context7["clients"].values()))
        if rejected_binding["materialization"] != "materialized" or not Path(
            rejected_binding["target_locator"]
        ).is_dir():
            raise RuntimeError("rejected removal did not preserve materialized state and target")
        tamper.unlink()
        run_cli(binary, "remove", "context7", sandbox, environment, check=True)
        state = json.loads((sandbox / "state" / "state-v2.json").read_text())
        active = [
            installation["declared_name"]
            for installation in state["installations"]
            for client in installation["clients"].values()
            if client["materialization"] != "absent"
        ]
        if active:
            raise RuntimeError(f"managed packages remained active: {active}")
    observed = datetime.now(timezone.utc).replace(microsecond=0)
    return {
        "client": "agentplugins CLI isolated Cursor provider",
        "version": version,
        "date": observed.date().isoformat(),
        "observed_at_utc": observed.isoformat().replace("+00:00", "Z"),
        "catalog_revision": catalog["revision"],
        "catalog_digest": "sha256:" + __import__("hashlib").sha256(CATALOG.read_bytes()).hexdigest(),
        "checks": [
            {"scenario": "resolve all short names from the pinned catalog", "status": "passed", "plugin_count": len(names)},
            {"scenario": "transactional add for every catalog package", "status": "passed", "plugin_count": len(names)},
            {"scenario": "committed receipt sequence and transition digests for every add/remove", "status": "passed", "plugin_count": len(names)},
            {"scenario": "tampered managed package blocks removal", "status": "passed", "plugin_count": 1},
            {
                "scenario": "Cursor process launch, tool runtime, and OAuth",
                "status": "skipped",
                "reason": "isolated package lifecycle does not launch Cursor or prove tool or OAuth runtime",
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
    print("OK: 26/26 pinned add/remove lifecycle flows", file=__import__("sys").stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
