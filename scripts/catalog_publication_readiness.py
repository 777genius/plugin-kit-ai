#!/usr/bin/env python3
"""Credential-free, exact-publication catalog proof; never account/runtime evidence.

produce executes released CLI operations only in a newly created disposable root.
verify is offline: it authenticates both feeds and reconstructs the entire fixed
proof contract. Workflow provenance/attestation is checked by the caller, not by
trusting claims inside this JSON document.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

import build_registry as registry
import directory_publication as publication
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
from cryptography.exceptions import InvalidSignature
from run_chrome_five_client_lifecycle import EvidenceError, snapshot_roots, seed_clients, validate_doctor
from run_mcp_e2e import inspector_check
from two_lane_evidence import PLUGIN_KIT_COMMIT, PLUGIN_KIT_TAG, RELEASED_LINUX_AMD64_DIGEST

ALIASES = tuple("agent-code-navigator atlassian chrome-devtools cloudflare cloudflare-bindings "
                "cloudflare-docs cloudflare-observability cloudflare-radar context7 docker-hub "
                "figma firebase github gitlab greptile heroku hubspot-crm hubspot-developer "
                "linear neon notion sentry statsig stripe supabase vercel".split())
CORE = ("codex", "cursor", "kiro")
CHROME = CORE + ("copilot", "vscode", "claude", "gemini", "opencode", "cline", "windsurf")
STATIC = ("777genius/cloudflare-docs", "777genius/cloudflare-docs-bridge")
CONTEXT_FIELDS = ("repository", "source_sha", "workflow_sha", "signed_ledger_sha",
                  "materialized_ledger_sha", "publication_id", "sequence", "snapshot_digest",
                  "run_id", "run_attempt", "directory_origin")
SHA = re.compile(r"[0-9a-f]{40}")
DIGEST = re.compile(r"sha256:[0-9a-f]{64}")
CLI = {"version": "0.1.24", "tag": PLUGIN_KIT_TAG, "commit": PLUGIN_KIT_COMMIT,
       "binary_digest": RELEASED_LINUX_AMD64_DIGEST,
       "native_clients": {"claude": "2.1.251", "copilot": "1.0.82"},
       "mcp_inspector": "2.1.0"}
LIMIT = 4 * 1024 * 1024
LIFECYCLE_STEPS = frozenset({
    "prepare", "native_version_claude_probe", "native_version_claude_exit", "native_version_claude_format",
    "native_version_copilot_probe", "native_version_copilot_exit", "native_version_copilot_format",
    "add", "validate_add", "info", "validate_info",
    "native_list_installed", "snapshot_before_update", "update", "validate_update",
    "snapshot_after_update", "remove_native", "native_list_removed", "remove_remaining",
    "list", "snapshot_before_doctor", "doctor", "validate_doctor", "snapshot_after_doctor",
    "validate_cleanup", "complete",
})


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def lifecycle_step(args: argparse.Namespace, selector: str, step: str) -> None:
    require(selector in ALIASES and step in LIFECYCLE_STEPS, "unknown lifecycle diagnostic step")
    args.failure_phase = f"lifecycle:{selector}:{step}"


def digest(body: bytes) -> str:
    return "sha256:" + hashlib.sha256(body).hexdigest()


def verify_signature(public_key: bytes, message: bytes, signature: bytes) -> None:
    try:
        Ed25519PublicKey.from_public_bytes(public_key).verify(signature, message)
    except (InvalidSignature, ValueError) as error:
        raise publication.PublicationError("invalid Ed25519 signature") from error


def read_json(path: Path, *, canonical: bool = False, limit: int = LIMIT):
    body = publication.read_bytes_bounded(path, limit)
    value = publication.parse_json_bytes(body, path.name, max_bytes=limit)
    if canonical:
        require(publication.canonical_json(value) == body, f"noncanonical JSON: {path.name}")
    return value


def load_feed(root: Path, keys: Path, *, baseline: bool = False) -> tuple[dict, dict]:
    latest = read_json(root / "latest.json", canonical=True, limit=publication.MAX_LATEST_BYTES)
    publication.validate_latest(latest)
    # validate_latest constrains paths, but also reject filesystem indirection.
    paths = [root / latest[name] for name in ("snapshot_path", "envelope_path")]
    for path in paths:
        require(path.resolve().is_relative_to(root.resolve()), "feed path escapes root")
        require(not path.is_symlink(), "symlink feed input")
    body = publication.read_bytes_bounded(paths[0], publication.MAX_SNAPSHOT_BYTES)
    envelope = read_json(paths[1], canonical=True, limit=publication.MAX_ENVELOPE_BYTES)
    publication.verify_envelope(body, envelope, publication.load_public_keys(keys), signature_verifier=verify_signature)
    snapshot = publication.parse_json_bytes(body, "snapshot", max_bytes=publication.MAX_SNAPSHOT_BYTES)
    publication.validate_snapshot_semantics(snapshot)
    require(snapshot["sequence"] == latest["sequence"] == envelope["sequence"], "feed sequence mismatch")
    now = datetime.now(timezone.utc)
    require(publication.parse_timestamp(snapshot["generated_at"], "generated_at") <= now, "future feed")
    require(baseline or now < publication.parse_timestamp(snapshot["expires_at"], "expires_at"), "expired candidate")
    return snapshot, {"sequence": snapshot["sequence"], "snapshot_digest": digest(body)}


def context(args: argparse.Namespace, snapshot: dict, identity: dict) -> dict:
    result = {key: getattr(args, key) for key in CONTEXT_FIELDS}
    for key in ("source_sha", "workflow_sha", "signed_ledger_sha", "materialized_ledger_sha"):
        require(isinstance(result[key], str) and SHA.fullmatch(result[key]) is not None, f"invalid {key}")
    require(result["repository"] == "777genius/universal-agent-plugins", "wrong repository")
    for key in ("sequence", "run_attempt"):
        require(type(result[key]) is int and result[key] > 0, f"invalid {key}")
    require(isinstance(result["run_id"], str) and re.fullmatch(r"[1-9][0-9]*", result["run_id"]), "invalid run ID")
    require(result["sequence"] == identity["sequence"] and result["snapshot_digest"] == identity["snapshot_digest"], "candidate identity mismatch")
    require(result["publication_id"] == snapshot["publication_id"], "publication ID mismatch")
    require(result["source_sha"] == snapshot["source_commit"], "source commit mismatch")
    expected = f"https://raw.githubusercontent.com/{result['repository']}/{result['materialized_ledger_sha']}/registry/schemas/1/"
    require(result["directory_origin"] == expected, "origin must name exact materialized ledger")
    return result


def selected(snapshot: dict, alias: str, clients: tuple[str, ...]) -> dict:
    resolved = registry.resolve_directory(snapshot, alias, list(clients))
    distribution = next(item for item in snapshot["distributions"] if item["id"] == resolved["distribution_id"])
    release = next(item for item in distribution["releases"] if item["sequence"] == resolved["release_sequence"])
    policy = next(item for item in distribution["release_policies"] if item["release_sequence"] == release["sequence"])
    floor = tuple(int(part) for part in policy["minimum_installer_version"].split("."))
    require(floor <= (0, 1, 24), f"{alias}: selected release requires newer CLI")
    source = release["package_source"]
    require(SHA.fullmatch(source["revision"]) is not None, f"{alias}: source is not immutable")
    return {"selector": alias, "product_id": resolved["product_id"], "distribution_id": distribution["id"],
            "distribution_kind": distribution["kind"], "release_sequence": release["sequence"],
            "package_version": release["package_version"], "package_source": source,
            "tree_digest": release["tree_digest"], "manifest_digest": release["manifest_digest"],
            "policy_digest": digest(publication.canonical_json(policy)), "policy": policy,
            "fallback_reason": resolved["fallback_reason"]}


def row_identity(selection: dict, client: str) -> dict:
    return {**{key: value for key, value in selection.items() if key != "policy"}, "client": client,
            "target_policy": next(item for item in selection["policy"]["targets"] if item["client"] == client)}


def plan(snapshot: dict) -> list[tuple[dict, tuple[str, ...]]]:
    # A new publication cannot silently shrink this accepted catalog matrix.
    return [(selected(snapshot, alias, CHROME if alias == "chrome-devtools" else CORE),
             CHROME if alias == "chrome-devtools" else CORE) for alias in ALIASES]


def policy_scope(baseline: dict, snapshot: dict, selections: list) -> None:
    """Fail new target expansions outside the fixed lifecycle/static coverage.

    Requiring authentication is a restriction, not a claim that accounts work.
    The two Cloudflare ChatGPT app replacements receive metadata-only checks.
    """
    covered = {(selection["distribution_id"], selection["release_sequence"], client)
               for selection, clients in selections for client in clients}
    covered.update((selection["distribution_id"], selection["release_sequence"], "chatgpt")
                   for selection in (selected(snapshot, selector, ("chatgpt",)) for selector in STATIC))
    old = {item["id"]: item for item in baseline["distributions"]}
    for distribution in snapshot["distributions"]:
        before = old.get(distribution["id"], {})
        policies = before.get("release_policies", [])
        previous = {(policy["release_sequence"], target["client"]): (target, policy)
                    for policy in policies if policy["status"] == "active" and before.get("status") == "active"
                    for target in policy["targets"]}
        for policy in distribution["release_policies"]:
            if policy["status"] != "active" or distribution["status"] != "active":
                continue
            for target in policy["targets"]:
                if (distribution["id"], policy["release_sequence"], target["client"]) in covered:
                    continue
                old_entry = previous.get((policy["release_sequence"], target["client"]))
                prior = old_entry[0] if old_entry else None
                restricted = dict(target)
                if prior and prior.get("authentication") == "not_required" and target.get("authentication") == "required":
                    restricted["authentication"] = "not_required"
                require(prior == restricted, f"uncovered target policy change: {distribution['id']}:{target['client']}")
                prior_floor = tuple(int(part) for part in old_entry[1]["minimum_installer_version"].split("."))
                new_floor = tuple(int(part) for part in policy["minimum_installer_version"].split("."))
                require(new_floor >= prior_floor, f"uncovered installer floor expansion: {distribution['id']}")


def lifecycle_proof(client: str) -> dict:
    return {"add": "passed", "info": "passed", "update": "passed_no_change",
            "state_bytes_unchanged": True, "owned_artifacts_unchanged": True,
            "remove": "passed_purged", "doctor": "passed_read_only", "installation_count": 0,
            "open_operation_count": 0, "owned_artifacts_absent": True,
            "acquisition_count": 1, "shared_installation": True,
            "native_plugin_listing": ("shared_copilot_registry" if client == "vscode" else
                                      "passed" if client in ("claude", "copilot") else "not_tested"),
            "account_runtime": "not_tested", "oauth": "not_tested"}


def static_rows(snapshot: dict) -> list[dict]:
    result = []
    for selector in STATIC:
        selection = selected(snapshot, selector, ("chatgpt",))
        target = next(item for item in selection["policy"]["targets"] if item["client"] == "chatgpt")
        require(target.get("delivery") == "manual_activation", f"{selector}: nonmanual ChatGPT delivery")
        binding = target.get("app_binding", {})
        require(binding.get("app_key") == "cloudflare-docs" and binding.get("mcp_server") == "cloudflare-docs"
                and isinstance(binding.get("id"), str) and bool(binding["id"]), "ChatGPT app binding incomplete")
        result.append({**row_identity(selection, "chatgpt"), "proof": "signed_package_policy_metadata_only",
                       "native_activation": "not_tested", "account_runtime": "not_tested", "oauth": "not_tested"})
    return result


def probe_contract(selection: dict, method: str) -> dict:
    plugin = selection["product_id"]
    return {"product_id": plugin, "distribution_id": selection["distribution_id"],
            "release_sequence": selection["release_sequence"], "package_source": selection["package_source"],
            "tree_digest": selection["tree_digest"], "manifest_digest": selection["manifest_digest"],
            "method": method, "tool": None if method == "tools/list" else
            ("resolve-library-id" if plugin == "context7" else "search_cloudflare_documentation"),
            "status": "passed", "scope": "package_mcp_component", "account_runtime": "not_tested", "oauth": "not_tested"}


def expected_artifact(snapshot: dict, baseline: dict, identity: dict, ctx: dict) -> dict:
    selections = plan(snapshot)
    policy_scope(baseline, snapshot, selections)
    rows = [{**row_identity(selection, client), "proof": lifecycle_proof(client)} for selection, clients in selections for client in clients]
    probes = [probe_contract(selection, method) for selection, _ in selections
              if selection["product_id"] in ("context7", "cloudflare-docs") for method in ("tools/list", "tools/call")]
    require(len(rows) == 85 and len(probes) == 4, "fixed matrix cardinality mismatch")
    return {"schema_version": 1, "kind": "catalog-publication-readiness-v1", "outcome": "passed",
            "context": ctx, "cli": CLI, "baseline": identity, "rows": rows,
            "static_metadata": static_rows(snapshot), "mcp_probes": probes,
            "process_isolation": "linux-bubblewrap-per-child-v1",
            "runtime_claims": False, "account_runtime": "not_tested", "oauth": "not_tested"}


def validate_artifact(value: dict, expected: dict) -> None:
    # Canonical bytes also distinguish true from 1 and false from 0.
    require(publication.canonical_json(value) == publication.canonical_json(expected), "artifact differs from fixed exact-publication proof")


def tool_paths(args: argparse.Namespace) -> tuple[Path, ...]:
    paths = {args.binary.absolute().parent}
    for tool in (args.claude, args.copilot, args.inspector, args.npx):
        if tool is not None:
            # npm's .bin links need their sibling packages/dependencies, not only .bin.
            paths.add(tool.absolute().parent.parent if tool.parent.name == ".bin" else tool.resolve().parent)
    return tuple(sorted(paths))


def child(argv: list[str], *, root: Path, readonly: tuple[Path, ...], cwd: Path, env: dict, timeout: int):
    from catalog_process_isolation import run_isolated
    return run_isolated(argv, writable_root=root.resolve(strict=True),
                        read_only_paths=tuple(sorted({path.resolve(strict=True) for path in readonly})),
                        cwd=cwd.resolve(strict=True), env=env, timeout=timeout)


def command(binary: Path, args: list[str], env: dict, cwd: Path, *, root: Path, readonly: tuple[Path, ...]) -> dict:
    completed = child([str(binary), *args, "--format", "json"], root=root, readonly=readonly, env=env, cwd=cwd, timeout=240)
    require(completed.returncode == 0, f"released CLI {args[0]} failed (exit {completed.returncode})")
    value = publication.parse_json_bytes(completed.stdout.encode(), "CLI response", max_bytes=LIMIT)
    require(value.get("command") == args[0] and value.get("result") == "success"
            and isinstance(value.get("data"), dict), f"released CLI {args[0]} failed")
    return value["data"]


def isolated_environment(root: Path, origin: str, tools: tuple[Path, ...]) -> tuple[dict, dict]:
    roots = {name: root / name for name in ("home", "state", "workspace", "config", "cache", "tmp")}
    for path in roots.values():
        path.mkdir(mode=0o700)
    seed_clients(root)
    for path in (roots["home"] / ".codex", roots["home"] / ".cursor", roots["home"] / ".kiro", roots["home"] / ".copilot"):
        path.mkdir(exist_ok=True)
    tool_bin = root / "bin"
    tool_bin.mkdir()
    for tool in tools:
        (tool_bin / tool.name).symlink_to(tool.resolve(strict=True))
    env = {"PATH": str(tool_bin) + os.pathsep + os.environ.get("PATH", "/usr/bin:/bin"),
           "LANG": "C.UTF-8", "HOME": str(roots["home"]), "USERPROFILE": str(roots["home"]),
           "XDG_CONFIG_HOME": str(roots["config"]), "XDG_CACHE_HOME": str(roots["cache"]),
           "XDG_DATA_HOME": str(roots["home"] / ".local/share"), "AGENTPLUGINS_HOME": str(roots["state"]),
           "TMPDIR": str(roots["tmp"]), "GIT_CONFIG_GLOBAL": str(roots["home"] / ".gitconfig"),
           "GIT_CONFIG_NOSYSTEM": "1", "GIT_TERMINAL_PROMPT": "0", "CI": "true",
           "AGENTPLUGINS_DIRECTORY_ORIGIN": origin,
           "npm_config_userconfig": str(root / "empty.npmrc"), "npm_config_globalconfig": str(root / "empty-global.npmrc")}
    return roots, env


def check_identity(data: dict, selection: dict, ctx: dict) -> None:
    source = selection["package_source"]
    for key, expected in {"plugin": selection["product_id"], "version": selection["package_version"],
                          "source": f"{source['repository']}//{source['path']}", "revision": source["revision"],
                          "tree_digest": selection["tree_digest"], "manifest_digest": selection["manifest_digest"]}.items():
        require(data.get(key) == expected, f"CLI selected wrong {key}")
    directory = data.get("directory", {})
    for key, expected in {"product_id": selection["product_id"], "distribution_id": selection["distribution_id"],
                          "distribution_kind": selection["distribution_kind"], "desired_release_sequence": selection["release_sequence"],
                          "snapshot_schema": 1, "snapshot_sequence": ctx["sequence"], "snapshot_digest": ctx["snapshot_digest"]}.items():
        require(directory.get(key) == expected, f"CLI Directory {key} mismatch")


def native_listing(tool: Path, env: dict, workspace: Path, plugin: str, installed: bool, *, root: Path, readonly: tuple[Path, ...]) -> None:
    argv = [str(tool), "plugin", "list"] + (["--json"] if tool.name == "claude" else [])
    completed = child(argv, root=root, readonly=readonly, env=env, cwd=workspace, timeout=60)
    require(completed.returncode == 0, "native plugin listing failed")
    if tool.name == "claude":
        values = json.loads(completed.stdout)
        require(isinstance(values, list), "Claude listing shape changed")
        matches = [item for item in values if item.get("id", "").split("@")[0] == plugin]
        require(len(matches) == int(installed), "Claude native listing mismatch")
        require(not installed or (matches[0].get("enabled") is True and matches[0].get("scope") == "user"), "Claude plugin not enabled in user scope")
    else:
        # Exact released native_identity.go empty-registry document: normalize
        # CRLF and remove at most one final newline, never arbitrary whitespace.
        document = completed.stdout.replace("\r\n", "\n").removesuffix("\n")
        if document == "No plugins installed.\n\nUse 'copilot plugin install <source>' to install a plugin.":
            require(not installed, "Copilot native listing unexpectedly empty")
            return
        # Same recognized installed-section contract as released .24, not substring logs.
        section, recognized, entries = False, False, []
        for line in completed.stdout.splitlines():
            if line.strip() == "Installed plugins:":
                require(not recognized, "duplicate Copilot installed section")
                section = recognized = True
                continue
            if section and line and not line[0].isspace():
                section = False
            if section:
                match = re.fullmatch(r"[ \t]+•[ \t]+([A-Za-z0-9][A-Za-z0-9._-]*@[A-Za-z0-9][A-Za-z0-9._-]*)[ \t]+\(v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?\)[ \t]*", line)
                if match:
                    entries.append(match[1])
                else:
                    require(line.strip() in ("", "No plugins installed.", "No plugins installed"), "unrecognized Copilot plugin listing")
        require(recognized and sum(item.split("@")[0] == plugin for item in entries) == int(installed), "Copilot native listing mismatch")


def exact_targets(data: dict, clients: tuple) -> list:
    require(type(data.get("succeeded")) is int and data["succeeded"] == len(clients), "wrong successful target count")
    require(type(data.get("failed")) is int and data["failed"] == 0, "target failure reported")
    targets = data.get("targets", [])
    require(len(targets) == len(clients) and {item["target"] for item in targets} == set(clients), "target set mismatch")
    require({item.get("status") for item in targets} == {"external_completed"}, "target status not completed")
    return targets


def validate_native_version(client: str, output: str) -> None:
    require(client in CLI["native_clients"], "unknown native version client")
    expected = re.escape(CLI["native_clients"][client])
    # Released .24's detector_test.go documents Copilot's terminal period and
    # optional v prefix. Match the branded first line, not an update notice that
    # might mention the expected version while a different binary is running.
    pattern = (rf"GitHub Copilot CLI v?{expected}\.?"
               if client == "copilot" else rf"{expected} \(Claude Code\)")
    first_line = output.splitlines()[0].strip() if output else ""
    require(re.fullmatch(pattern, first_line) is not None, "native CLI version mismatch")


def remove_targets(run, plugin: str, clients: tuple, verify_native_absent) -> dict:
    remaining = clients
    if "copilot" in clients or "vscode" in clients:
        shared = tuple(client for client in clients if client in ("copilot", "vscode"))
        # The released adapter skips native uninstall with --external-uninstalled.
        # Remove the shared native registry first, preserving data for other clients.
        first = run("remove", plugin, "--target", ",".join(shared))
        exact_targets(first, shared)
        verify_native_absent()
        remaining = tuple(client for client in clients if client not in shared)
    require(bool(remaining), "fixed lifecycle expects nonnative targets remaining")
    result = run("remove", plugin, "--target", ",".join(remaining), "--external-uninstalled", "--purge-data")
    exact_targets(result, remaining)
    require(result.get("plugin_data_preserved") is False and result.get("data_retained") is False, "remove retained data")
    return result


def run_lifecycle(args: argparse.Namespace, root: Path, selection: dict, clients: tuple, ctx: dict) -> None:
    step = lambda name: lifecycle_step(args, selection["selector"], name)
    step("prepare")
    root.mkdir(mode=0o700)
    roots, env = isolated_environment(root, ctx["directory_origin"], (args.claude, args.copilot))
    readonly = tool_paths(args)
    if "claude" in clients:
        for tool in (args.claude, args.copilot):
            step(f"native_version_{tool.name}_probe")
            result = child([str(tool), "--version"], root=root, readonly=readonly, env=env, cwd=roots["workspace"], timeout=30)
            step(f"native_version_{tool.name}_exit")
            require(result.returncode == 0, "native CLI version command failed")
            step(f"native_version_{tool.name}_format")
            validate_native_version(tool.name, result.stdout)
    common = ["--target", ",".join(clients)]
    def run(phase, *tail):
        if phase == "remove":
            step("remove_remaining" if "--external-uninstalled" in tail else "remove_native")
        else:
            step(phase)
        return command(args.binary, [phase, *tail], env, roots["workspace"], root=root, readonly=readonly)
    data = run("add", selection["selector"], *common)
    step("validate_add")
    check_identity(data, selection, ctx)
    targets = exact_targets(data, clients)
    acquisition = data.get("acquisition", {})
    require(type(acquisition.get("acquisition_count")) is int and acquisition["acquisition_count"] == 1, "not one package acquisition")
    require(acquisition.get("fetched") is True and acquisition.get("validated") is True, "package not acquired and validated")
    outcomes = data.get("target_outcomes", {})
    require(set(outcomes) == set(clients), "target outcomes incomplete")
    for outcome in outcomes.values():
        require(outcome.get("outcome") == "passed", "target preparation failed")
        for field in ("acquisition_id", "tree_digest", "manifest_digest", "closure_digest"):
            require(outcome.get(field) == acquisition.get(field) and bool(acquisition.get(field)), "target acquisition differs")
    for field in ("tree_digest", "manifest_digest"):
        require(acquisition[field] == selection[field], "acquired digest mismatch")
    info = run("info", selection["product_id"], *common)
    step("validate_info")
    state_file = roots["state"] / "state-v2.json"
    installations = read_json(state_file)["installations"]
    require(len(installations) == 1, "not exactly one installation")
    installation = installations[0]
    require(installation["origin_mode"] == "directory", "not Directory-managed")
    require(installation["source"].get("repository") == selection["package_source"]["repository"]
            and installation["source"].get("resolved_revision") == selection["package_source"]["revision"]
            and installation["source"].get("tree_digest") == selection["tree_digest"], "stored source differs")
    require(info["installation_id"] == installation["installation_id"], "info installation mismatch")
    info_surfaces = {surface for client in info.get("clients", []) for surface in client.get("affected_surfaces", [client["client_id"]])}
    require(info_surfaces == set(clients), "info target set mismatch")
    for client in info["clients"]:
        require(client["package_revision"].get("tree_digest") == selection["tree_digest"]
                and client["package_revision"].get("manifest_digest") == selection["manifest_digest"], "info package identity mismatch")
    require({item["output"]["result"]["installation_id"] for item in targets} == {installation["installation_id"]}, "not shared installation")
    bindings = list(installation["clients"].values())
    surfaces = {surface for binding in bindings for surface in binding.get("affected_surfaces", [binding["client_id"]])}
    require(surfaces == set(clients), "physical bindings do not cover logical targets")
    for binding in bindings:
        revision = binding["package_revision"]
        require(revision["tree_digest"] == selection["tree_digest"] and revision["manifest_digest"] == selection["manifest_digest"], "binding package differs")
    owned = [Path(binding["target_locator"]) for binding in bindings]
    require(all(path.resolve().is_relative_to(root.resolve()) and path.exists() for path in owned), "owned path escaped or missing")
    if "claude" in clients:
        step("native_list_installed")
        for tool in (args.claude, args.copilot):
            native_listing(tool, env, roots["workspace"], selection["product_id"], True, root=root, readonly=readonly)
    immutable = {name: path for name, path in roots.items() if name != "state"}
    immutable.update({f"owned{index}": path for index, path in enumerate(owned)})
    step("snapshot_before_update")
    before, state_before = snapshot_roots(immutable), state_file.read_bytes()
    updated = run("update", selection["product_id"], *common)
    step("validate_update")
    require(updated.get("status") == "completed", "no-change update failed")
    for target in exact_targets(updated, clients):
        result = target.get("output", {}).get("result", {})
        require(result.get("no_change") is True and result.get("mutated") is False, "update changed package")
    step("snapshot_after_update")
    require(state_file.read_bytes() == state_before and snapshot_roots(immutable) == before, "no-change update mutated state or artifacts")
    def verify_native_absent():
        step("native_list_removed")
        native_listing(args.copilot, env, roots["workspace"], selection["product_id"], False, root=root, readonly=readonly)
    remove_targets(run, selection["product_id"], clients, verify_native_absent)
    require(run("list").get("installations") == [], "installations remain")
    step("snapshot_before_doctor")
    before = snapshot_roots(roots)
    doctor = run("doctor")
    step("validate_doctor")
    # command() has authenticated the full envelope before returning only data.
    validate_doctor({"command": "doctor", "result": "success", "data": doctor}, post_remove=True)
    step("snapshot_after_doctor")
    require(snapshot_roots(roots) == before, "doctor mutated roots")
    step("validate_cleanup")
    require(doctor.get("read_only") is True and doctor.get("installation_count") == 0 and doctor.get("open_operation_count") == 0, "doctor not clean/read-only")
    require(all(not path.exists() and not path.is_symlink() for path in owned), "owned artifact remains")
    for directory in (roots["state"] / "managed", roots["state"] / "plugin-data"):
        require(not directory.exists() or not any(path.is_file() or path.is_symlink() for path in directory.rglob("*")), "managed residue remains")
    if "claude" in clients:
        step("native_list_removed")
        for tool in (args.claude, args.copilot):
            native_listing(tool, env, roots["workspace"], selection["product_id"], False, root=root, readonly=readonly)
    step("complete")


def acquired_package(root: Path, selection: dict) -> Path:
    root.mkdir(mode=0o700)
    source = selection["package_source"]
    archive, expanded, package = root / "source.tar.gz", root / "source.tar", root / "package"
    registry.download_archive(source["repository"], source["revision"], archive)
    registry.decompress_archive(archive, expanded)
    package.mkdir()
    registry.extract_package(expanded, source["path"], package)
    require(registry.directory_tree_digest(package) == selection["tree_digest"], "acquired tree mismatch")
    require(digest((package / "plugin.json").read_bytes()) == selection["manifest_digest"], "acquired manifest mismatch")
    return package


def check_static_package(package: Path, selection: dict) -> None:
    target = next(item for item in selection["policy"]["targets"] if item["client"] == "chatgpt")
    binding = target["app_binding"]
    manifest = read_json(package / "plugin.json")
    require(manifest.get("name") == selection["product_id"], "static manifest product mismatch")
    server = read_json(package / "mcp.json").get("mcpServers", {}).get(binding["mcp_server"], {})
    require(server.get("type") == "streamable-http" and server.get("url") == "https://docs.mcp.cloudflare.com/mcp", "ChatGPT binding does not name acquired Cloudflare HTTP interface")


def produce(args: argparse.Namespace, snapshot: dict, expected: dict) -> None:
    require(digest(args.binary.read_bytes()) == CLI["binary_digest"], "wrong released binary")
    root = args.sandbox.absolute()
    require(not root.exists() and not root.is_symlink() and root.parent.exists(), "sandbox must be a new directory")
    require(not args.output.absolute().is_relative_to(root), "output cannot be inside disposable root")
    require(not args.output.exists(), "output must be new; refusing stale success artifact")
    root.mkdir(mode=0o700)
    try:
        for selection, clients in plan(snapshot):
            args.failure_phase = "lifecycle:" + selection["selector"]
            print(f"catalog readiness: {selection['selector']} ({len(clients)} targets)", flush=True)
            run_lifecycle(args, root / selection["selector"], selection, clients, expected["context"])
            if selection["product_id"] not in ("context7", "cloudflare-docs"):
                continue
            package = acquired_package(root / (selection["selector"] + "-probe"), selection)
            probe_parent = root / (selection["selector"] + "-sessions")
            probe_parent.mkdir(mode=0o700)
            def probe_runner(argv, *, cwd, env, timeout, **_):
                return child(argv, root=Path(cwd), readonly=(*tool_paths(args), package), cwd=Path(cwd), env=env, timeout=timeout)
            for method in ("tools/list", "tools/call"):
                args.failure_phase = "mcp:" + selection["product_id"] + ":" + method
                contract = probe_contract(selection, method)
                tool_args = ({"libraryName": "playwright", "query": "Playwright locators quick start"}
                             if selection["product_id"] == "context7" else {"query": "Workers bindings versus environment variables"})
                result = inspector_check(selection["product_id"], method=method, plugin_root=package,
                                         npx=str(args.npx), tool_name=contract["tool"],
                                         inspector=args.inspector,
                                         sandbox_parent=probe_parent, process_runner=probe_runner,
                                         tool_args=tool_args if method == "tools/call" else None)
                require(result.get("status") == "passed", f"{selection['product_id']} {method} component probe failed")
        # Static ChatGPT records bind signed release policy and actual immutable package bytes.
        for index, selector in enumerate(STATIC):
            args.failure_phase = "static:" + selector
            selection = selected(snapshot, selector, ("chatgpt",))
            check_static_package(acquired_package(root / f"static-{index}", selection), selection)
    finally:
        shutil.rmtree(root)
    # Cleanup is part of the proof, not a best-effort action after success.
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_bytes(publication.canonical_json(expected))


def parser() -> argparse.ArgumentParser:
    result = argparse.ArgumentParser(description=__doc__)
    result.add_argument("mode", choices=("produce", "verify"))
    for name in ("feed", "trusted-keys", "baseline-feed"):
        result.add_argument("--" + name, type=Path, required=True)
    for name in CONTEXT_FIELDS:
        result.add_argument("--" + name.replace("_", "-"), type=int if name in ("sequence", "run_attempt") else str, required=True)
    for name in ("binary", "sandbox", "output", "artifact", "claude", "copilot", "inspector"):
        result.add_argument("--" + name, type=Path)
    result.add_argument("--npx", type=Path, default=Path(shutil.which("npx") or "npx"))
    return result


def main(argv: list[str] | None = None) -> int:
    args = parser().parse_args(argv)
    args.failure_phase = "validate_inputs"
    try:
        snapshot, identity = load_feed(args.feed, args.trusted_keys)
        baseline, baseline_identity = load_feed(args.baseline_feed, args.trusted_keys, baseline=True)
        require(baseline_identity["sequence"] < identity["sequence"], "baseline must precede candidate")
        ctx = context(args, snapshot, identity)
        expected = expected_artifact(snapshot, baseline, baseline_identity, ctx)
        if args.mode == "verify":
            require(args.artifact is not None, "--artifact required")
            validate_artifact(read_json(args.artifact, canonical=True), expected)
        else:
            require(all(getattr(args, name) is not None for name in ("binary", "sandbox", "output", "claude", "copilot")), "producer paths required")
            produce(args, snapshot, expected)
        return 0
    except (OSError, ValueError, RuntimeError, KeyError, TypeError, StopIteration, EvidenceError, subprocess.SubprocessError, registry.RegistryError, publication.PublicationError) as error:
        # Never persist raw command responses, paths, credentials or native logs.
        if args.mode == "produce" and args.output is not None:
            args.output.parent.mkdir(parents=True, exist_ok=True)
            (args.output.parent / "catalog-readiness-failure.json").write_bytes(publication.canonical_json({
                "kind": "catalog-readiness-failure-v1", "outcome": "failed",
                "phase": args.failure_phase, "error_class": type(error).__name__,
            }))
        print(f"catalog readiness failed: {args.failure_phase}: {type(error).__name__}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
