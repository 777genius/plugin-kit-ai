"""Offline fixtures only: no package commands, accounts or network."""
from __future__ import annotations

import argparse
import copy
import json
import os
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
            lambda value: value["cli"].update(version="0.1.25"),
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
            policy["minimum_installer_version"] = "0.1.25"
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
        completed = subprocess.CompletedProcess([], 0, '{"result":"success","data":{}}', "")
        with tempfile.TemporaryDirectory() as temporary, patch.object(gate, "child", return_value=completed) as child:
            root = Path(temporary)
            gate.command(root / "agentplugins", ["list"], {}, root, root=root, readonly=(root / "tools",))
            self.assertEqual(child.call_args.kwargs["root"], root)
            self.assertEqual(child.call_args.kwargs["readonly"], (root / "tools",))
            child.return_value = subprocess.CompletedProcess([], 0, '[]', "")
            gate.native_listing(Path("claude"), {}, root, "chrome-devtools", False, root=root, readonly=())
            self.assertEqual(child.call_args.args[0], ["claude", "plugin", "list", "--json"])

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
