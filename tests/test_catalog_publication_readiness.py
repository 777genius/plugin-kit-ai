"""Offline fixtures only: no package commands, accounts or network."""
from __future__ import annotations

import argparse
import copy
import hashlib
import json
import os
import shutil
import subprocess
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
import catalog_publication_readiness as gate
import directory_publication as publication
import run_mcp_e2e as mcp


def catalog():
    value = json.loads((ROOT / "registry/directory.json").read_text())
    for distribution in value["distributions"]:
        for release in distribution["releases"]:
            if release["package_source"]["revision"] is None:
                release["package_source"]["revision"] = "a" * 40
    value.update(sequence=20, publication_id="test-publication", source_commit="a" * 40,
                 snapshot_schema_version=1, generated_at="2026-08-30T00:00:00Z", expires_at="2099-01-01T00:00:00Z")
    return value


def arguments(snapshot):
    values = dict(repository="777genius/universal-agent-plugins", source_sha="a" * 40,
                  workflow_sha="b" * 40, signed_ledger_sha="c" * 40,
                  materialized_ledger_sha="d" * 40, publication_id=snapshot["publication_id"],
                  sequence=20, snapshot_digest="sha256:" + "e" * 64, run_id="123", run_attempt=1,
                  directory_origin="https://raw.githubusercontent.com/777genius/universal-agent-plugins/" + "d" * 40 + "/registry/schemas/1/")
    return argparse.Namespace(**values)


class CatalogContractTests(unittest.TestCase):
    def setUp(self):
        self.snapshot = catalog()
        self.args = arguments(self.snapshot)
        self.identity = {"sequence": 20, "snapshot_digest": self.args.snapshot_digest}
        self.baseline = copy.deepcopy(self.snapshot)
        self.baseline["sequence"] = 13
        self.baseline_identity = {"sequence": 13, "snapshot_digest": "sha256:" + "f" * 64}

    def expected(self):
        return gate.expected_artifact(self.snapshot, self.baseline, self.baseline_identity,
                                      gate.context(self.args, self.snapshot, self.identity))

    def test_fixed_matrix_fallback_and_honest_claims(self):
        artifact = self.expected()
        self.assertEqual(len(artifact["rows"]), 85)
        self.assertEqual(len(artifact["static_metadata"]), 2)
        self.assertEqual(len(artifact["mcp_probes"]), 4)
        chrome = [row for row in artifact["rows"] if row["selector"] == "chrome-devtools"]
        self.assertEqual({row["client"] for row in chrome}, set(gate.CHROME))
        self.assertEqual({row["distribution_id"] for row in chrome}, {"777genius/chrome-devtools-bridge"})
        self.assertEqual({row["release_sequence"] for row in chrome}, {2})
        self.assertTrue(all(row["fallback_reason"] for row in chrome))
        self.assertIs(artifact["runtime_claims"], False)
        self.assertTrue(all(row["oauth"] == "not_tested" for row in artifact["mcp_probes"]))
        gate.validate_artifact(artifact, self.expected())

    def test_rejects_mutated_identity_partial_rows_runtime_claims_and_extra_fields(self):
        for mutation in (
            lambda value: value["rows"].pop(),
            lambda value: value["rows"].append(value["rows"][0]),
            lambda value: value.update(runtime_claims=True),
            lambda value: value.update(runtime_claims=0),
            lambda value: value["cli"].update(version="0.1.26"),
            lambda value: value["context"].update(run_attempt=2),
            lambda value: value["mcp_probes"].pop(),
            lambda value: value["rows"][0]["proof"].update(acquisition_count=True),
            lambda value: value["static_metadata"][0].update(native_activation="passed"),
            lambda value: value.update(stdout="private"),
        ):
            with self.subTest(mutation=mutation):
                expected = self.expected()
                changed = copy.deepcopy(expected)
                mutation(changed)
                with self.assertRaises(ValueError):
                    gate.validate_artifact(changed, expected)

    def test_context_rejects_wrong_source_run_origin_and_snapshot(self):
        for field, value in (("source_sha", "b" * 40), ("run_id", "1/attempts/2"),
                             ("directory_origin", "https://example.com/feed/"), ("sequence", True),
                             ("snapshot_digest", "sha256:" + "1" * 64), ("publication_id", "old")):
            args = copy.deepcopy(self.args)
            setattr(args, field, value)
            with self.subTest(field=field), self.assertRaises(ValueError):
                gate.context(args, self.snapshot, self.identity)

    def test_new_uncovered_target_policy_fails_closed(self):
        distribution = next(item for item in self.snapshot["distributions"] if item["id"] == "upstash/context7")
        policy = next(item for item in distribution["release_policies"] if item["status"] == "active")
        policy["targets"][0]["delivery"] = "surprise"
        with self.assertRaisesRegex(ValueError, "uncovered target"):
            self.expected()

    def test_uncovered_alternate_reactivation_or_new_release_cannot_reuse_old_target(self):
        for change in ("reactivation", "new_release", "lower_floor"):
            with self.subTest(change=change):
                self.snapshot, self.baseline = catalog(), catalog()
                old = next(item for item in self.baseline["distributions"] if item["id"] == "upstash/context7")
                new = next(item for item in self.snapshot["distributions"] if item["id"] == "upstash/context7")
                if change == "reactivation":
                    old["status"] = "suspended"
                elif change == "new_release":
                    for policy in new["release_policies"]:
                        policy["release_sequence"] += 100
                else:
                    for policy in old["release_policies"]:
                        policy["minimum_installer_version"] = "0.1.25"
                with self.assertRaisesRegex(ValueError, "uncovered"):
                    self.expected()

    def test_unselected_release_cannot_borrow_selected_distribution_coverage(self):
        selection = gate.selected(self.snapshot, "chrome-devtools", gate.CHROME)
        distribution = next(item for item in self.snapshot["distributions"] if item["id"] == selection["distribution_id"])
        extra = copy.deepcopy(selection["policy"])
        extra["release_sequence"] = 0
        distribution["release_policies"].append(extra)
        with self.assertRaisesRegex(ValueError, "uncovered"):
            self.expected()

    def test_authentication_tightening_is_not_account_runtime_expansion(self):
        for snapshot, authentication in ((self.baseline, "not_required"), (self.snapshot, "required")):
            distribution = next(item for item in snapshot["distributions"] if item["id"] == "upstash/context7")
            for policy in distribution["release_policies"]:
                for target in policy["targets"]:
                    target["authentication"] = authentication
        self.expected()
        self.snapshot, self.baseline = self.baseline, self.snapshot
        with self.assertRaisesRegex(ValueError, "uncovered target"):
            self.expected()

    def test_new_cli_floor_and_missing_alias_fail(self):
        selected = gate.selected(self.snapshot, "chrome-devtools", gate.CHROME)
        distribution = next(item for item in self.snapshot["distributions"] if item["id"] == selected["distribution_id"])
        for policy in distribution["release_policies"]:
            policy["minimum_installer_version"] = "0.1.26"
        with self.assertRaisesRegex(ValueError, "newer CLI"):
            gate.plan(self.snapshot)
        self.snapshot = catalog()
        self.snapshot["products"] = [p for p in self.snapshot["products"] if p["id"] != "github"]
        with self.assertRaises(Exception):
            gate.plan(self.snapshot)

    def test_read_json_rejects_duplicates_and_noncanonical_bytes(self):
        with tempfile.TemporaryDirectory() as root:
            path = Path(root) / "artifact.json"
            for body in (b'{"x":1,"x":2}\n', b'{ "x": 1 }\n'):
                path.write_bytes(body)
                with self.assertRaises((ValueError, publication.PublicationError)):
                    gate.read_json(path, canonical=True)

    def test_actual_signed_feed_and_tamper(self):
        fixture = ROOT / "tests/fixtures/directory-publication"
        with tempfile.TemporaryDirectory() as root:
            feed = Path(root)
            (feed / "snapshots").mkdir()
            latest = json.loads((fixture / "latest.json").read_text())
            for destination, source in (("latest.json", "latest.json"),
                                        (latest["snapshot_path"], "snapshot.json"),
                                        (latest["envelope_path"], "envelope-current.json")):
                (feed / destination).write_bytes((fixture / source).read_bytes())
            snapshot, identity = gate.load_feed(feed, fixture / "trusted-keys.json", baseline=True)
            self.assertEqual(identity["sequence"], 15)
            (feed / latest["snapshot_path"]).write_bytes(publication.canonical_json({**snapshot, "sequence": 16}))
            with self.assertRaises(publication.PublicationError):
                gate.load_feed(feed, fixture / "trusted-keys.json", baseline=True)

    def test_environment_does_not_inherit_credentials_or_directory_injection(self):
        with tempfile.TemporaryDirectory() as root, patch.dict(os.environ, {"GH_TOKEN": "secret", "OPENAI_API_KEY": "secret", "AGENTPLUGINS_DIRECTORY_SNAPSHOT": "/bad"}):
            _, env = gate.isolated_environment(Path(root), self.args.directory_origin, ())
            self.assertFalse({"GH_TOKEN", "OPENAI_API_KEY", "AGENTPLUGINS_DIRECTORY_SNAPSHOT"} & env.keys())
            self.assertEqual(env["AGENTPLUGINS_DIRECTORY_ORIGIN"], self.args.directory_origin)

    def test_acquired_root_mcp_override_does_not_use_repository_plugin(self):
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            package, sandbox = root / "acquired", root / "sandbox"
            package.mkdir(); sandbox.mkdir()
            (package / "mcp.json").write_text(json.dumps({"mcpServers": {"context7": {"command": "node", "args": ["${PLUGIN_ROOT}/exact.mjs"]}}}))
            config, env = mcp.materialize_inspector_config("context7", sandbox, plugin_root=package)
            server = json.loads(config.read_text())["mcpServers"]["context7"]
            self.assertEqual(server["args"], [str(package.resolve() / "exact.mjs")])
            self.assertEqual(env["PLUGIN_ROOT"], str(package.resolve()))

    def test_exact_target_status_rejects_success_failure_contradiction(self):
        good = {"succeeded": 3, "failed": 0, "targets": [{"target": client, "status": "external_completed"} for client in gate.CORE]}
        gate.exact_targets(good, gate.CORE)
        for field, value in (("succeeded", True), ("failed", 1), ("targets", good["targets"][:2])):
            with self.subTest(field=field), self.assertRaises(ValueError):
                gate.exact_targets({**good, field: value}, gate.CORE)
        good["targets"][0]["status"] = "failed"
        with self.assertRaises(ValueError):
            gate.exact_targets(good, gate.CORE)

    def test_native_version_accepts_exact_branded_release_formats(self):
        # Copilot punctuation/notice format is documented by the exact released
        # c78c79 clientdetect/detector_test.go:397-435 fixtures (then 1.0.80).
        for output in ("GitHub Copilot CLI 1.0.82.\n", "GitHub Copilot CLI v1.0.82.\n",
                       "GitHub Copilot CLI 1.0.82\n", "GitHub Copilot CLI 1.0.82.\nA newer version is available."):
            with self.subTest(output=output):
                gate.validate_native_version("copilot", output)
        gate.validate_native_version("claude", "2.1.251 (Claude Code)\n")

    def test_native_version_rejects_malformed_or_wrong_version_even_in_notice(self):
        for output in ("", "GitHub Copilot CLI 1.0.82..", "GitHub Copilot CLI .1.0.82.",
                       "GitHub Copilot CLI 1..82.", "GitHub Copilot CLI 1.0.x.",
                       "GitHub Copilot CLI 1.0.82beta.", "GitHub Copilot CLI 1.0.82-beta.1.",
                       "GitHub Copilot CLI 1.0.82+build.1.", "GitHub Copilot CLI 11.0.82.",
                       "GitHub Copilot CLI 1.0.820.", "warning: GitHub Copilot CLI 1.0.82.",
                       "GitHub Copilot CLI 1.0.81.\nA newer version 1.0.82 is available.",
                       "GitHub Copilot CLI 1.0.81.\nGitHub Copilot CLI 1.0.82."):
            with self.subTest(output=output), self.assertRaises(ValueError):
                gate.validate_native_version("copilot", output)
        for output in ("2.1.250 (Claude Code)\nUpdate to 2.1.251", "12.1.251 (Claude Code)",
                       "2.1.251-beta (Claude Code)", "other-program 2.1.251", "2.1.251 (Claude Code) extra"):
            with self.subTest(output=output), self.assertRaises(ValueError):
                gate.validate_native_version("claude", output)

    def test_native_version_diagnostics_distinguish_client_exit_and_format(self):
        for client, failure in (("claude", "exit"), ("claude", "format"), ("copilot", "exit"), ("copilot", "format")):
            with self.subTest(client=client, failure=failure), tempfile.TemporaryDirectory() as temporary:
                parent = Path(temporary).resolve()
                tools = parent / "tools"
                tools.mkdir()
                for name in ("agentplugins", "claude", "copilot", "npx"):
                    (tools / name).write_bytes(b"fixture only; not executed")
                args = argparse.Namespace(binary=tools / "agentplugins", claude=tools / "claude",
                                          copilot=tools / "copilot", npx=tools / "npx", inspector=None)
                def child(argv, **_):
                    name = Path(argv[0]).name
                    self.assertEqual(argv[1:], ["--version"])
                    output = "2.1.251 (Claude Code)\n" if name == "claude" else "GitHub Copilot CLI 1.0.82.\n"
                    code = 0
                    if name == client:
                        code = 9 if failure == "exit" else 0
                        output = "wrong format" if failure == "format" else output
                    return subprocess.CompletedProcess(argv, code, output, "not persisted")
                with patch.object(gate, "child", side_effect=child), self.assertRaises(ValueError):
                    gate.run_lifecycle(args, parent / "case", gate.selected(self.snapshot, "chrome-devtools", gate.CHROME),
                                       gate.CHROME, vars(self.args))
                self.assertEqual(args.failure_phase, f"lifecycle:chrome-devtools:native_version_{client}_{failure}")

    def test_static_package_binds_signed_app_key_to_acquired_http_interface(self):
        selection = gate.selected(self.snapshot, gate.STATIC[0], ("chatgpt",))
        with tempfile.TemporaryDirectory() as root:
            package = Path(root)
            (package / "plugin.json").write_text('{"name":"cloudflare-docs"}')
            mcp_path = package / "mcp.json"
            valid = {"mcpServers": {"cloudflare-docs": {"type": "streamable-http", "url": "https://docs.mcp.cloudflare.com/mcp"}}}
            mcp_path.write_text(json.dumps(valid))
            gate.check_static_package(package, selection)
            valid["mcpServers"]["cloudflare-docs"]["url"] = "https://other.example/mcp"
            mcp_path.write_text(json.dumps(valid))
            with self.assertRaisesRegex(ValueError, "acquired Cloudflare"):
                gate.check_static_package(package, selection)

    def test_shared_copilot_removes_natively_before_remaining_targets_and_purge(self):
        events = []
        def run(*argv):
            events.append(argv)
            clients = tuple(argv[argv.index("--target") + 1].split(","))
            return {"succeeded": len(clients), "failed": 0,
                    "targets": [{"target": client, "status": "external_completed"} for client in clients],
                    "plugin_data_preserved": False, "data_retained": False}
        gate.remove_targets(run, "chrome-devtools", gate.CHROME, lambda: events.append("native_absent"))
        self.assertEqual(events[0], ("remove", "chrome-devtools", "--target", "copilot,vscode"))
        self.assertEqual(events[1], "native_absent")
        self.assertEqual(events[2], ("remove", "chrome-devtools", "--target",
                                    "codex,cursor,kiro,claude,gemini,opencode,cline,windsurf",
                                    "--external-uninstalled", "--purge-data"))

    def test_shared_native_failure_does_not_proceed_to_purge(self):
        run = unittest.mock.Mock(return_value={"succeeded": 0, "failed": 2, "targets": []})
        with self.assertRaises(ValueError):
            gate.remove_targets(run, "chrome-devtools", gate.CHROME, unittest.mock.Mock())
        self.assertEqual(run.call_count, 1)

    def test_cli_and_native_inspection_use_required_isolated_child(self):
        completed = subprocess.CompletedProcess([], 0, '{"command":"list","result":"success","data":{}}', "")
        with tempfile.TemporaryDirectory() as temporary, patch.object(gate, "child", return_value=completed) as child:
            root = Path(temporary)
            gate.command(root / "agentplugins", ["list"], {}, root, root=root, readonly=(root / "tools",))
            self.assertEqual(child.call_args.kwargs["root"], root)
            self.assertEqual(child.call_args.kwargs["readonly"], (root / "tools",))
            child.return_value = subprocess.CompletedProcess([], 0, '[]', "")
            gate.native_listing(Path("claude"), {}, root, "chrome-devtools", False, root=root, readonly=())
            self.assertEqual(child.call_args.args[0], ["claude", "plugin", "list", "--json"])

    def test_copilot_exact_official_empty_registry_after_removal(self):
        official = "No plugins installed.\n\nUse 'copilot plugin install <source>' to install a plugin."
        for output in (official, official + "\n", official.replace("\n", "\r\n"),
                       (official + "\n").replace("\n", "\r\n")):
            with self.subTest(output=output), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                completed = subprocess.CompletedProcess([], 0, output, "")
                with patch.object(gate, "child", return_value=completed) as child:
                    gate.native_listing(Path("copilot"), {}, root, "chrome-devtools", False, root=root, readonly=())
                    self.assertEqual(child.call_args.args[0], ["copilot", "plugin", "list"])
                    self.assertEqual(child.call_args.kwargs["root"], root)
                    with self.assertRaisesRegex(ValueError, "unexpectedly empty"):
                        gate.native_listing(Path("copilot"), {}, root, "chrome-devtools", True, root=root, readonly=(),
                                            expected_marketplace="agentplugins-123456789abc", expected_version="1.0.0",
                                            expected_path=root / "managed")

    def test_copilot_empty_registry_rejects_truncation_prefix_suffix_and_failed_process(self):
        # Mirrors c78c79 native_identity_test.go's absent_short/prefix/suffix
        # negatives; no broad prefix/suffix tolerance is introduced.
        official = "No plugins installed.\n\nUse 'copilot plugin install <source>' to install a plugin."
        for output in ("No plugins installed.\n", "\n" + official + "\n", official + "\n\n",
                       official + " extra", "prefix\n" + official, official.replace("\n\n", "\n"),
                       official + "\r", "", "No plugins installed"):
            with self.subTest(output=output), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                with patch.object(gate, "child", return_value=subprocess.CompletedProcess([], 0, output, "")):
                    with self.assertRaises(ValueError):
                        gate.native_listing(Path("copilot"), {}, root, "chrome-devtools", False, root=root, readonly=())
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            with patch.object(gate, "child", return_value=subprocess.CompletedProcess([], 1, official, "not persisted")):
                with self.assertRaisesRegex(ValueError, "listing failed"):
                    gate.native_listing(Path("copilot"), {}, root, "chrome-devtools", False, root=root, readonly=())

    def check_native_diagnostic(self, client, output, expected_stage, *, installed=True, returncode=0,
                                expected_marketplace="agentplugins-123456789abc", expected_version="1.0.0",
                                expected_path=None):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            args = argparse.Namespace()
            completed = subprocess.CompletedProcess([], returncode, output, "private stderr must not escape")
            callback = lambda phase: gate.lifecycle_step(args, "chrome-devtools", phase)
            expected_path = expected_path or root / "managed"
            with patch.object(gate, "child", return_value=completed) as child:
                with self.assertRaises(ValueError):
                    gate.native_listing(Path(client), {}, root, "chrome-devtools", installed,
                                        root=root, readonly=(), expected_marketplace=expected_marketplace,
                                        expected_version=expected_version, expected_path=expected_path, diagnostic=callback)
                self.assertEqual(child.call_count, 1)
            state = "installed" if installed else "removed"
            self.assertEqual(args.failure_phase, f"lifecycle:chrome-devtools:native_list_{client}_{state}_{expected_stage}")
            self.assertNotIn("private", args.failure_phase)

    def test_claude_native_json_shape_fails_cleanly_with_bounded_diagnostics(self):
        self.check_native_diagnostic("claude", '{"private raw value":', "json_decode")
        for value in ({}, None, True, [None], [42], ["private raw value"], [{}],
                      [{"id": None}], [{"id": 9}], [{"id": []}], [{"id": ""}]):
            with self.subTest(value=value):
                self.check_native_diagnostic("claude", json.dumps(value), "json_shape")

    def test_claude_native_diagnostics_distinguish_exit_matching_scope_and_enabled(self):
        good = {"id": "chrome-devtools@skills-dir", "scope": "user", "enabled": True}
        self.check_native_diagnostic("claude", json.dumps([good]), "exit", returncode=7)
        self.check_native_diagnostic("claude", "[]", "matching")
        self.check_native_diagnostic("claude", json.dumps([good, good]), "matching")
        self.check_native_diagnostic("claude", json.dumps([good]), "matching", installed=False)
        for scope in (None, "project", True, {}):
            self.check_native_diagnostic("claude", json.dumps([{**good, "scope": scope}]), "scope")
        for enabled in (None, False, 1, "true", {}):
            self.check_native_diagnostic("claude", json.dumps([{**good, "enabled": enabled}]), "enabled")

    def test_copilot_native_diagnostics_distinguish_exit_section_format_count(self):
        good = "Installed plugins:\n  • chrome-devtools@agentplugins-123456789abc (v1.0.0)\n"
        self.check_native_diagnostic("copilot", good, "exit", returncode=1)
        self.check_native_diagnostic("copilot", "private unexpected response", "section")
        self.check_native_diagnostic("copilot", "Installed plugins:\n  invalid entry\n", "format")
        self.check_native_diagnostic("copilot", "Installed plugins:\n", "count")
        self.check_native_diagnostic("copilot", good + "Installed plugins:\n", "section")
        self.check_native_diagnostic("copilot", good, "count", installed=False)

    def test_copilot_exact_live_plugin_contract_binds_identity_version_status_and_path(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            managed = root / "state/managed/copilot/chrome-devtools-fixture"
            marketplace = gate.managed_marketplace("chrome-devtools-fixture")
            body = ("Live Plugins (loaded from a local marketplace directory, never copied):\n"
                    f"  • chrome-devtools@{marketplace} (v1.7.0-uap.1) (enabled)\n"
                    f"      from {managed}")
            for output in (body, body + "\n", body.replace("\n", "\r\n"), (body + "\n").replace("\n", "\r\n")):
                seen = []
                with self.subTest(output=output), patch.object(
                    gate, "child", return_value=subprocess.CompletedProcess([], 0, output, "")
                ):
                    gate.native_listing(Path("copilot"), {}, root, "chrome-devtools", True,
                                        root=root, readonly=(), expected_marketplace=marketplace,
                                        expected_version="1.7.0-uap.1", expected_path=managed,
                                        diagnostic=seen.append)
                self.assertEqual(seen[-1], "native_list_copilot_installed_count")

    def test_copilot_live_plugin_rejects_malformed_or_unbound_fields(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            managed = root / "state/managed/copilot/chrome-devtools-fixture"
            marketplace = gate.managed_marketplace("chrome-devtools-fixture")
            header = "Live Plugins (loaded from a local marketplace directory, never copied):"
            valid_entry = f"  • chrome-devtools@{marketplace} (v1.7.0-uap.1) (enabled)"
            valid_path = f"      from {managed}"
            cases = {
                "section": "Live plugins:\n" + valid_entry + "\n" + valid_path,
                "format": header + "\n" + valid_entry,
                "format_extra": header + "\n" + valid_entry + "\n" + valid_path + "\nextra",
                "identity": header + "\n" + valid_entry.replace("chrome-devtools@", "other@") + "\n" + valid_path,
                "marketplace": header + "\n" + valid_entry.replace(marketplace, "agentplugins-deadbeefcafe") + "\n" + valid_path,
                "version": header + "\n" + valid_entry.replace("1.7.0-uap.1", "1.7.0-uap.2") + "\n" + valid_path,
                "status": header + "\n" + valid_entry.replace("enabled", "disabled") + "\n" + valid_path,
                "at_literal": header + "\n" + valid_entry + "\n      at " + str(managed),
                "path": header + "\n" + valid_entry + "\n      from " + str(root / "other"),
                "relative_path": header + "\n" + valid_entry + "\n      from relative/path",
                "duplicate": header + "\n" + valid_entry + "\n" + valid_path + "\n" + valid_entry + "\n" + valid_path,
            }
            for name, output in cases.items():
                expected_stage = ("identity" if name == "marketplace" else "format" if name in ("format_extra", "duplicate", "at_literal")
                                  else "path" if name == "relative_path" else name)
                with self.subTest(name=name):
                    self.check_native_diagnostic("copilot", output, expected_stage,
                                                 expected_marketplace=marketplace, expected_version="1.7.0-uap.1")

    def test_claude_installed_path_is_absolute_exact_binding_path(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            managed = root / "state/managed/claude/chrome-devtools-fixture"
            base = {"id": "chrome-devtools@skills-dir", "scope": "user", "enabled": True}
            for install_path in (None, 7, [], {}, "relative/path", str(root / "other")):
                with self.subTest(install_path=install_path):
                    output = json.dumps([{**base, "installPath": install_path}])
                    expected_stage = "path"
                    self.check_native_diagnostic("claude", output, expected_stage)

    def test_marketplace_identity_matches_released_sha256_prefix_contract(self):
        physical = "chrome-devtools-123456789abc"
        self.assertEqual(gate.managed_marketplace(physical),
                         "agentplugins-" + hashlib.sha256(physical.encode()).hexdigest()[:12])
        for invalid in (None, 7, "", "  "):
            with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                gate.managed_marketplace(invalid)

    def test_shared_copilot_binding_can_be_owned_by_vscode(self):
        shared = {"client_id": "vscode", "affected_surfaces": ["copilot", "vscode"],
                  "target_locator": "/isolated/shared"}
        claude = {"client_id": "claude", "affected_surfaces": ["claude"],
                  "target_locator": "/isolated/claude"}
        self.assertIs(gate.native_binding([shared, claude], "copilot"), shared)
        self.assertIs(gate.native_binding([shared, claude], "claude"), claude)
        with self.assertRaisesRegex(ValueError, "ambiguous"):
            gate.native_binding([shared, {**shared, "client_id": "copilot"}], "copilot")
        with self.assertRaisesRegex(ValueError, "missing"):
            gate.native_binding([claude], "copilot")

    def test_installed_marketplace_requires_exact_lowercase_twelve_hex(self):
        output = "Installed plugins:\n  • chrome-devtools@agentplugins-123456789abc (v1.0.0)\n"
        for marketplace in ("agentplugins-fixture", "agentplugins-123", "agentplugins-123456789abcd",
                            "agentplugins-123456789abG", "prefix-agentplugins-123456789abc"):
            with self.subTest(marketplace=marketplace), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                with patch.object(gate, "child", return_value=subprocess.CompletedProcess([], 0, output, "")):
                    with self.assertRaisesRegex(ValueError, "identity is incomplete"):
                        gate.native_listing(Path("copilot"), {}, root, "chrome-devtools", True,
                                            root=root, readonly=(), expected_marketplace=marketplace,
                                            expected_version="1.0.0", expected_path=root / "managed")

    def test_successful_native_diagnostics_only_emit_fixed_client_stage_enums(self):
        cases = (("claude", json.dumps([{"id": "chrome-devtools@skills-dir", "scope": "user", "enabled": True,
                                          "installPath": None}]), "path"),
                 ("copilot", "Installed plugins:\n  • chrome-devtools@agentplugins-123456789abc (v1.0.0)\n", "version"))
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for client, output, last in cases:
                seen = []
                managed = root / "managed"
                if client == "claude":
                    output = json.dumps([{"id": "chrome-devtools@skills-dir", "scope": "user",
                                          "enabled": True, "installPath": str(managed)}])
                with patch.object(gate, "child", return_value=subprocess.CompletedProcess([], 0, output, "")):
                    gate.native_listing(Path(client), {}, root, "chrome-devtools", True,
                                        root=root, readonly=(), expected_marketplace="agentplugins-123456789abc",
                                        expected_version="1.0.0", expected_path=managed, diagnostic=seen.append)
                self.assertTrue(set(seen).issubset(gate.LIFECYCLE_STEPS))
                self.assertEqual(seen[0], f"native_list_{client}_installed_probe")
                self.assertEqual(seen[-1], f"native_list_{client}_installed_{last}")

    def run_first_package_fixture(self, parent: Path, mutation: str | None = None):
        """Only the process boundary is fake; state, files and lifecycle checks are real.

        Released c78c79 service.go stores plan.ActivePath as TargetLocator;
        planner.go joins the physical artifact ID and stager.go materializes a
        managed_package_directory, including for skills-only agent-code-navigator.
        """
        tools = parent / "tools"
        tools.mkdir()
        for name in ("agentplugins", "claude", "copilot", "npx"):
            (tools / name).write_bytes(b"fixture only; never executed")
        args = argparse.Namespace(binary=tools / "agentplugins", claude=tools / "claude",
                                  copilot=tools / "copilot", npx=tools / "npx", inspector=None)
        root = parent / "case"
        selection = gate.selected(self.snapshot, "agent-code-navigator", gate.CORE)
        package_revision = {key: selection[key] for key in ("tree_digest", "manifest_digest")}
        owned = {client: root / "state" / "managed" / client / "agent-code-navigator-fixture" for client in gate.CORE}
        clients = {client: {"client_id": client, "target_locator": str(path),
                            "affected_surfaces": [client], "package_revision": package_revision}
                   for client, path in owned.items()}
        state = {"installations": [{"installation_id": "fixture-installation", "origin_mode": "directory",
                                   "source": {"repository": selection["package_source"]["repository"],
                                              "resolved_revision": selection["package_source"]["revision"],
                                              "tree_digest": selection["tree_digest"]}, "clients": clients}]}
        state_path = root / "state" / "state-v2.json"
        calls = []

        def targets():
            physical_id = None if mutation == "missing_physical_id" else "agent-code-navigator-fixture"
            return {"succeeded": 3, "failed": 0, "targets": [
                {"target": client, "status": "external_completed", "output": {"result": {
                    "installation_id": "fixture-installation", "no_change": True, "mutated": False,
                        "plan": {"physical_artifact_id": physical_id}}}}
                for client in gate.CORE]}

        def child(argv, **kwargs):
            self.assertEqual(kwargs["root"], root)
            phase = argv[1]
            calls.append(phase)
            if phase == "add":
                for client, path in owned.items():
                    path.parent.mkdir(parents=True)
                    if mutation == "file_locator" and client == "kiro":
                        path.write_bytes(b"not a managed package directory")
                    else:
                        path.mkdir()
                        (path / "SKILL.md").write_bytes(b"# Fixture skill\n")
                        (path / "SKILL.md").chmod(0o640)
                        (path / "linked-skill").symlink_to("SKILL.md")
                state_path.write_bytes(publication.canonical_json(state))
                acquisition = {**package_revision, "acquisition_id": "fixture-acquisition",
                               "closure_digest": "sha256:" + "1" * 64, "acquisition_count": 1,
                               "fetched": True, "validated": True}
                data = {**targets(), **package_revision, "plugin": selection["product_id"],
                        "version": selection["package_version"],
                        "source": selection["package_source"]["repository"] + "//" + selection["package_source"]["path"],
                        "revision": selection["package_source"]["revision"], "acquisition": acquisition,
                        "target_outcomes": {client: {**acquisition, "outcome": "passed"} for client in gate.CORE},
                        "directory": {"product_id": selection["product_id"], "distribution_id": selection["distribution_id"],
                                      "distribution_kind": selection["distribution_kind"],
                                      "desired_release_sequence": selection["release_sequence"],
                                      "snapshot_schema": 1, "snapshot_sequence": self.args.sequence,
                                      "snapshot_digest": self.args.snapshot_digest}}
            elif phase == "info":
                data = {"installation_id": "fixture-installation", "clients": list(clients.values())}
            elif phase == "update":
                path = owned["codex"] / "SKILL.md"
                if mutation == "file_bytes":
                    path.write_bytes(b"changed skill")
                elif mutation == "file_mode":
                    path.chmod(0o600)
                elif mutation == "topology":
                    (owned["codex"] / "unexpected-file").write_bytes(b"new artifact")
                elif mutation == "symlink_target":
                    link = owned["codex"] / "linked-skill"
                    link.unlink()
                    link.symlink_to("different-target")
                data = {**targets(), "status": "completed"}
            elif phase == "remove":
                for path in owned.values():
                    shutil.rmtree(path)
                state_path.write_bytes(publication.canonical_json({"installations": []}))
                data = {**targets(), "plugin_data_preserved": False, "data_retained": False}
            elif phase == "list":
                data = {"installations": []}
            elif phase == "doctor":
                data = {"read_only": True, "installation_count": 0, "open_operation_count": 0,
                        "findings": [{"status": "healthy", "code": "no_degradation_detected",
                                      "message": "no tracked degradation was detected"}]}
                if mutation == "doctor_findings":
                    data["findings"] = []
                elif mutation == "doctor_mutation":
                    (root / "home" / "unexpected-write").write_bytes(b"doctor must be read-only")
                elif mutation == "doctor_open_operation":
                    data["open_operation_count"] = 1
            else:
                self.fail(f"unexpected fixture phase: {phase}")
            reported = "list" if phase == "doctor" and mutation == "wrong_command" else phase
            body = json.dumps({"command": reported, "result": "success", "data": data})
            return subprocess.CompletedProcess(argv, 0, body, "")

        try:
            with patch.object(gate, "child", side_effect=child):
                gate.run_lifecycle(args, root, selection, gate.CORE, vars(self.args))
        except (ValueError, gate.EvidenceError) as error:
            return args, calls, error
        return args, calls, None

    def test_first_package_full_lifecycle_accepts_exact_doctor_envelope(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve()
            args, calls, error = self.run_first_package_fixture(root)
            self.assertIsNone(error)
            self.assertEqual(calls, ["add", "info", "update", "remove", "list", "doctor"])
            self.assertEqual(args.failure_phase, "lifecycle:agent-code-navigator:complete")
            self.assertEqual(gate.read_json(root / "case/state/state-v2.json")["installations"], [])
            self.assertFalse(any(path.is_file() or path.is_symlink() for path in (root / "case/state/managed").rglob("*")))

    def test_first_package_real_files_modes_and_symlink_targets_are_not_ignored(self):
        for mutation in ("file_bytes", "file_mode", "topology", "symlink_target"):
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as temporary:
                args, calls, error = self.run_first_package_fixture(Path(temporary).resolve(), mutation)
                self.assertIsInstance(error, ValueError)
                self.assertEqual(calls, ["add", "info", "update"])
                self.assertEqual(args.failure_phase, "lifecycle:agent-code-navigator:snapshot_after_update")

    def test_file_locator_is_rejected_not_silently_dropped_from_snapshot(self):
        with tempfile.TemporaryDirectory() as temporary:
            args, calls, error = self.run_first_package_fixture(Path(temporary).resolve(), "file_locator")
            self.assertIsInstance(error, gate.EvidenceError)
            self.assertEqual(calls, ["add", "info"])
            self.assertEqual(args.failure_phase, "lifecycle:agent-code-navigator:snapshot_before_update")

    def test_missing_physical_artifact_id_fails_before_identity_hashing(self):
        with tempfile.TemporaryDirectory() as temporary:
            args, calls, error = self.run_first_package_fixture(Path(temporary).resolve(), "missing_physical_id")
            self.assertIsInstance(error, ValueError)
            self.assertIn("valid physical artifact", str(error))
            self.assertEqual(calls, ["add"])
            self.assertEqual(args.failure_phase, "lifecycle:agent-code-navigator:validate_add")

    def test_doctor_wrong_command_findings_mutation_and_open_operation_fail_precisely(self):
        for mutation, phase in (("wrong_command", "doctor"), ("doctor_findings", "validate_doctor"),
                                ("doctor_mutation", "snapshot_after_doctor"), ("doctor_open_operation", "validate_cleanup")):
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as temporary:
                args, calls, error = self.run_first_package_fixture(Path(temporary).resolve(), mutation)
                self.assertIsNotNone(error)
                self.assertEqual(calls[-1], "doctor")
                self.assertEqual(args.failure_phase, "lifecycle:agent-code-navigator:" + phase)

    def test_diagnostic_steps_reject_unbounded_input(self):
        for selector, step in (("/private/path", "add"), ("agent-code-navigator", "raw log secret")):
            with self.assertRaisesRegex(ValueError, "unknown lifecycle"):
                gate.lifecycle_step(argparse.Namespace(), selector, step)

    def test_inspector_runner_receives_fresh_bounded_sandbox(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve()
            package, parent = root / "package", root / "sessions"
            package.mkdir(); parent.mkdir()
            (package / "mcp.json").write_text('{"mcpServers":{"context7":{"command":"node","args":[]}}}')
            observed = []
            def runner(argv, **kwargs):
                sandbox = Path(kwargs["cwd"])
                self.assertTrue(sandbox.is_relative_to(parent))
                self.assertNotEqual(sandbox, parent)
                self.assertEqual(kwargs["env"]["PLUGIN_ROOT"], str(package))
                self.assertTrue((sandbox / "mcp.json").is_file())
                observed.append(sandbox)
                return subprocess.CompletedProcess(argv, 0, '{"result":{"tools":[{"name":"resolve-library-id"}]}}', "")
            result = mcp.inspector_check("context7", plugin_root=package, sandbox_parent=parent,
                                         process_runner=runner, inspector=Path("/exact/mcp-inspector"))
            self.assertEqual(result["status"], "passed")
            self.assertEqual(len(observed), 1)
            self.assertFalse(observed[0].exists())

    def test_producer_composes_all_phases_and_only_emits_after_cleanup(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            args = argparse.Namespace(binary=root / "binary", sandbox=root / "isolated", output=root / "evidence.json",
                                      claude=Path("claude"), copilot=Path("copilot"), npx=Path("npx"), inspector=None)
            args.binary.write_bytes(b"fixture binary")
            with patch.dict(gate.CLI, {"binary_digest": gate.digest(args.binary.read_bytes())}), \
                    patch.object(gate, "run_lifecycle") as lifecycle, \
                    patch.object(gate, "acquired_package", return_value=root) as acquire, \
                    patch.object(gate, "check_static_package") as static, \
                    patch.object(gate, "inspector_check", return_value={"status": "passed"}) as probe:
                expected = self.expected()
                gate.produce(args, self.snapshot, expected)
                self.assertEqual(lifecycle.call_count, 26)
                self.assertEqual(acquire.call_count, 4)
                self.assertEqual(static.call_count, 2)
                self.assertEqual(probe.call_count, 4)
                self.assertFalse(args.sandbox.exists())
                gate.validate_artifact(gate.read_json(args.output, canonical=True), expected)

    def test_failed_cli_mcp_static_or_cleanup_never_emits_success(self):
        for failing in ("run_lifecycle", "inspector_check", "check_static_package", "cleanup"):
            with self.subTest(failing=failing), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                args = argparse.Namespace(binary=root / "binary", sandbox=root / "isolated", output=root / "evidence.json",
                                          claude=Path("claude"), copilot=Path("copilot"), npx=Path("npx"), inspector=None)
                args.binary.write_bytes(b"fixture binary")
                with patch.dict(gate.CLI, {"binary_digest": gate.digest(args.binary.read_bytes())}), \
                        patch.object(gate, "run_lifecycle") as lifecycle, \
                        patch.object(gate, "acquired_package", return_value=root), \
                        patch.object(gate, "check_static_package") as static, \
                        patch.object(gate, "inspector_check", return_value={"status": "passed"}) as probe:
                    expected = self.expected()
                    if failing == "cleanup":
                        with patch.object(gate.shutil, "rmtree", side_effect=OSError("fixture cleanup failure")):
                            with self.assertRaises(OSError):
                                gate.produce(args, self.snapshot, expected)
                    else:
                        {"run_lifecycle": lifecycle, "inspector_check": probe, "check_static_package": static}[failing].side_effect = ValueError("fixture phase failure")
                        with self.assertRaises(ValueError):
                            gate.produce(args, self.snapshot, expected)
                        self.assertFalse(args.sandbox.exists())
                    self.assertFalse(args.output.exists())

    def test_wrong_binary_does_not_create_sandbox_or_artifact(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            binary = root / "binary"
            binary.write_bytes(b"not the release")
            args = argparse.Namespace(binary=binary, sandbox=root / "isolated", output=root / "evidence.json")
            with self.assertRaisesRegex(ValueError, "wrong released binary"):
                gate.produce(args, self.snapshot, self.expected())
            self.assertFalse(args.sandbox.exists())
            self.assertFalse(args.output.exists())


if __name__ == "__main__":
    unittest.main()
