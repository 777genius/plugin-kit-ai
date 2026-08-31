from __future__ import annotations

import importlib.util
import base64
import ctypes
import errno
import hashlib
import copy
import json
import os
import platform
import shutil
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from datetime import datetime, timedelta, timezone
from pathlib import Path
from unittest import mock

import jsonschema
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey


ROOT = Path(__file__).resolve().parents[1]
AGENTPLUGINS_0_1_14_FIXTURES = Path(__file__).resolve().parent / "fixtures" / "agentplugins-0.1.14"
AGENTPLUGINS_0_1_14_ADD = AGENTPLUGINS_0_1_14_FIXTURES / "add.json"
AGENTPLUGINS_0_1_14_STATE_V2 = AGENTPLUGINS_0_1_14_FIXTURES / "state-v2.json"


def release_fixture(name: str) -> dict:
    return json.loads((AGENTPLUGINS_0_1_14_FIXTURES / name).read_text())
MODULE = ROOT / "scripts" / "run_launch_evidence_e2e.py"
SPEC = importlib.util.spec_from_file_location("run_launch_evidence_e2e", MODULE)
assert SPEC and SPEC.loader
e2e = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(e2e)
import launch_observer_signatures as observer_signatures
OBSERVER_SPEC = importlib.util.spec_from_file_location("observe_launch_scenario", ROOT / "scripts" / "observe_launch_scenario.py")
assert OBSERVER_SPEC and OBSERVER_SPEC.loader
observer = importlib.util.module_from_spec(OBSERVER_SPEC)
OBSERVER_SPEC.loader.exec_module(observer)
FACADE_SPEC = importlib.util.spec_from_file_location("observe_release_facade", ROOT / "scripts" / "observe_release_facade.py")
assert FACADE_SPEC and FACADE_SPEC.loader
facade = importlib.util.module_from_spec(FACADE_SPEC)
FACADE_SPEC.loader.exec_module(facade)
CONSENT = ROOT / "tests/e2e/fixtures/fixture-only-consent.json"
PUBLICATION = ROOT / "tests/fixtures/directory-publication"


class LaunchEvidenceE2ETests(unittest.TestCase):
    def test_scenario_target_contract_is_exhaustive_exact_and_ordered(self) -> None:
        config = json.loads((ROOT / "tests/e2e/launch-scenarios.json").read_text())
        targets = observer.validate_scenario_target_contract(config)
        self.assertEqual(set(targets), observer.EXPECTED_SCENARIOS)
        self.assertEqual(len(targets), 53)
        order = {client: index for index, client in enumerate(observer.SCENARIO_TARGET_ORDER)}
        for scenario, value in targets.items():
            with self.subTest(scenario=scenario):
                self.assertTrue(value)
                self.assertEqual(len(value), len(set(value)))
                self.assertTrue(set(value) <= observer.CLIENTS)
                self.assertEqual(value, tuple(sorted(value, key=order.__getitem__)))

        self.assertEqual(targets["state_schema_2_migration"], ("codex",))
        self.assertEqual(targets["managed_rollback"], ("codex", "cursor", "kiro"))
        self.assertEqual(targets["repair_sticky_distribution"], ("cursor",))
        self.assertEqual(targets["repair_codex"], ("codex",))
        self.assertEqual(targets["repair_cursor"], ("cursor",))
        self.assertEqual(targets["repair_kiro"], ("kiro",))
        self.assertEqual(targets["shared_copilot_vscode_backend"], ("copilot", "vscode"))
        for plugin in config["heroes"]:
            for client in config["runtime_clients"]:
                self.assertEqual(targets[f"hero_lifecycle_{plugin}_{client}"], (client,))

    def test_invalid_scenario_target_contracts_fail_before_effects(self) -> None:
        base = json.loads((ROOT / "tests/e2e/launch-scenarios.json").read_text())
        mutations = {
            "missing": lambda value: value["scenario_targets"].pop("directory_offline"),
            "extra": lambda value: value["scenario_targets"].update(unknown_scenario=["cursor"]),
            "empty": lambda value: value["scenario_targets"].update(directory_offline=[]),
            "duplicate": lambda value: value["scenario_targets"].update(directory_offline=["cursor", "cursor"]),
            "unsupported": lambda value: value["scenario_targets"].update(directory_offline=["unknown"]),
            "order": lambda value: value["scenario_targets"].update(managed_rollback=["kiro", "cursor", "codex"]),
        }
        for label, mutate in mutations.items():
            config = json.loads(json.dumps(base))
            mutate(config)
            with self.subTest(label=label), mock.patch.object(observer, "observe") as observe_effect, mock.patch.object(
                observer.subprocess, "run",
            ) as process_effect, self.assertRaises(ValueError):
                observer.validate_scenario_target_contract(config)
            observe_effect.assert_not_called()
            process_effect.assert_not_called()

    def test_harness_and_observer_resolve_every_scenario_from_same_contract(self) -> None:
        harness = self.fixture_harness()
        self.assertEqual(len(observer.EXPECTED_SCENARIOS), 53)
        self.assertEqual(harness.scenario_targets, observer.SCENARIO_CLIENT_TARGETS)
        for scenario in observer.EXPECTED_SCENARIOS:
            self.assertEqual(harness.scenario_targets[scenario], observer.scenario_client_targets(scenario))

    def test_scenario_target_argv_guard_rejects_drift(self) -> None:
        observer.validate_scenario_command_targets("state_schema_2_migration", [{
            "argv": ["info", "context7", "--target", "codex", "--format", "json"],
        }])
        with self.assertRaisesRegex(ValueError, "drift"):
            observer.validate_scenario_command_targets("state_schema_2_migration", [{
                "argv": ["info", "context7", "--target", "cursor", "--format", "json"],
            }])

    def test_target_boundary_rejects_contract_change_and_old_blanket_bypass_before_process(self) -> None:
        counterexamples = (
            (["add", "context7", "--target", "cursor", "--format", "json"], ("codex",)),
            (["remove", "context7", "--target", "codex", "--format", "json"], ("cursor",)),
        )
        for argv, targets in counterexamples:
            with self.subTest(argv=argv), mock.patch.object(observer.subprocess, "run") as process, self.assertRaisesRegex(ValueError, "drift"):
                observer.traced(Path("binary"), argv, Path("root"), "challenge", scenario_targets=targets)
            process.assert_not_called()

    def test_exact_revoked_probe_descriptor_is_accepted_and_near_misses_rejected(self) -> None:
        context = self.complete_scenario_context()
        with tempfile.TemporaryDirectory() as temporary:
            environment, digest = observer.conformance_directory(
                Path(temporary), context, sequence=12_001, revoked=True,
            )
            probe = observer.RevokedTargetProbe(
                "revoked_operations_boundary", "add", "context7", context["release"]["distribution_id"],
                observer.REVOKED_TARGET_PROBE_ARGV, digest, 1,
                observer.REVOKED_TARGET_PROBE_EXIT_CODE,
                observer.REVOKED_TARGET_PROBE_REJECTION_CLASS,
                observer.REVOKED_TARGET_PROBE_REJECTION_REASON,
                "", observer.revoked_target_probe_stderr(context["release"]["distribution_id"], 1), True,
            )
            argv = list(observer.REVOKED_TARGET_PROBE_ARGV)
            observer.authorize_scenario_command_target(argv, ("cursor",), environment=environment, revoked_probe=probe)
            near_misses = (
                (argv, ("cursor",), environment, probe._replace(operation="remove")),
                (argv, ("cursor",), environment, probe._replace(expected_exit_code=42)),
                (argv, ("cursor",), environment, probe._replace(expected_rejection_class="timeout")),
                (argv, ("cursor",), environment, probe._replace(expected_rejection_reason="revoked")),
                (argv, ("cursor",), environment, probe._replace(expected_stderr="revoked")),
                (["remove", "context7", "--target", "codex", "--format", "json"], ("cursor",), environment, probe),
                (argv, ("cursor",), {**environment, "AGENTPLUGINS_DIRECTORY_SNAPSHOT": "missing"}, probe),
                (argv, ("cursor",), environment, probe._replace(scenario_id="directory_offline")),
            )
            for candidate, targets, candidate_environment, candidate_probe in near_misses:
                with self.subTest(candidate=candidate, probe=candidate_probe), self.assertRaisesRegex(ValueError, "drift"):
                    observer.authorize_scenario_command_target(
                        candidate, targets, environment=candidate_environment, revoked_probe=candidate_probe,
                    )

    def test_revoked_probe_verifier_binds_exact_result_and_complete_disposable_root(self) -> None:
        context = self.complete_scenario_context()
        with tempfile.TemporaryDirectory() as temporary:
            scenario_root = Path(temporary) / "scenario"
            workspace = scenario_root / "workspace"
            for relative in ("workspace", "home", "manager", "config", "cache", "evidence", "tmp"):
                (scenario_root / relative).mkdir(parents=True)
            environment, digest = observer.conformance_directory(
                workspace, context, sequence=12_001, revoked=True,
            )
            environment.update({
                "HOME": str(scenario_root / "home"), "USERPROFILE": str(scenario_root / "home"),
                "AGENTPLUGINS_HOME": str(scenario_root / "manager"),
                "XDG_CONFIG_HOME": str(scenario_root / "config"),
                "XDG_CACHE_HOME": str(scenario_root / "cache"),
                "AGENTPLUGINS_EVIDENCE_ROOT": str(scenario_root / "evidence"),
                "TMPDIR": str(scenario_root / "tmp"), "TMP": str(scenario_root / "tmp"),
                "TEMP": str(scenario_root / "tmp"), "GIT_CONFIG_GLOBAL": str(scenario_root / "gitconfig"),
            })
            probe = observer.RevokedTargetProbe(
                "revoked_operations_boundary", "add", "context7", context["release"]["distribution_id"],
                observer.REVOKED_TARGET_PROBE_ARGV, digest, 1,
                observer.REVOKED_TARGET_PROBE_EXIT_CODE,
                observer.REVOKED_TARGET_PROBE_REJECTION_CLASS,
                observer.REVOKED_TARGET_PROBE_REJECTION_REASON,
                "", observer.revoked_target_probe_stderr(context["release"]["distribution_id"], 1), True,
            )
            root, bindings = observer.revoked_probe_scenario_root(workspace, environment)
            expected_binary = scenario_root / "release" / "agentplugins"
            expected_binary.parent.mkdir()
            body = (
                "#!/usr/bin/python3\nimport sys\n"
                "if sys.argv[1:] == ['version']:\n print('agentplugins 0.1.18')\n"
                "else:\n"
                f" sys.stderr.write({probe.expected_stderr!r})\n raise SystemExit(1)\n"
            ).encode()
            expected_binary.write_bytes(body)
            expected_binary.chmod(0o700)
            def issue():
                session = observer.AuthenticatedBinaryExecutionSession(
                    expected_binary.resolve(), cwd=workspace,
                    command_plan=(observer.REVOKED_TARGET_PROBE_ARGV,),
                )
                completed, evidence = session.execute(
                    expected_binary.resolve(), list(observer.REVOKED_TARGET_PROBE_ARGV),
                    cwd=workspace, write_authority=None,
                )
                return session, completed, evidence

            def verify(session, completed, evidence, *, candidate_probe=probe, before=None, after=None):
                before = observer.filesystem_snapshot(root) if before is None else before
                after = before if after is None else after
                return observer.verify_revoked_target_probe_result(
                    completed, candidate_probe, argv=list(probe.target_argv), environment=environment,
                    scenario_root=root, writable_bindings=bindings, before=before,
                    after=after,
                    execution_binding=evidence, execution_session=session,
                    binary=expected_binary,
                )

            def rejected_result(label, mutate):
                session, completed, evidence = issue()
                original = (copy.deepcopy(completed.args), completed.returncode, completed.stdout, completed.stderr)
                mutate(completed)
                with self.subTest(result=label):
                    self.assertFalse(verify(session, completed, evidence)["verified"])
                    completed.args, completed.returncode, completed.stdout, completed.stderr = original
                    self.assertFalse(verify(session, completed, evidence)["verified"])

            def rejected_evidence(label, mutate):
                session, completed, evidence = issue()
                original = copy.deepcopy(evidence)
                mutate(evidence)
                with self.subTest(evidence=label):
                    self.assertFalse(verify(session, completed, evidence)["verified"])
                    evidence.clear(); evidence.update(original)
                    self.assertFalse(verify(session, completed, evidence)["verified"])

            def rejected_probe(label, candidate_probe):
                session, completed, evidence = issue()
                with self.subTest(probe=label):
                    self.assertFalse(verify(
                        session, completed, evidence, candidate_probe=candidate_probe,
                    )["verified"])
                    self.assertFalse(verify(session, completed, evidence)["verified"])
            with (
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(body)),
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", hashlib.sha256(body).hexdigest()),
            ):
                session, completed, evidence = issue()
                self.assertIs(type(evidence), dict)
                self.assertFalse(hasattr(observer, "AuthenticatedBinaryExecutionBinding"))
                self.assertTrue(verify(session, completed, evidence)["verified"])
                self.assertFalse(verify(session, completed, evidence)["verified"])

                stderr_lines = probe.expected_stderr.splitlines(keepends=True)
                result_mutations = {
                    "wrong exit code": lambda item: setattr(item, "returncode", 42),
                    "extra argv": lambda item: setattr(item, "args", [*item.args, "extra"]),
                    "reordered argv": lambda item: setattr(
                        item, "args", [item.args[0], item.args[2], item.args[1], *item.args[3:]],
                    ),
                    "wrong argv": lambda item: setattr(
                        item, "args", [item.args[0], "remove", *item.args[2:]],
                    ),
                    "unexpected stdout": lambda item: setattr(item, "stdout", "unexpected\n"),
                    "omitted progress line": lambda item: setattr(item, "stderr", "".join(stderr_lines[1:])),
                    "wrong selector": lambda item: setattr(
                        item, "stderr", item.stderr.replace('"context7"', '"other"'),
                    ),
                    "wrong distribution": lambda item: setattr(
                        item, "stderr", item.stderr.replace(
                            context["release"]["distribution_id"], "fixture/wrong-distribution",
                        ),
                    ),
                    "wrong release sequence": lambda item: setattr(
                        item, "stderr", item.stderr.replace("release 1", "release 2"),
                    ),
                    "wrong reason": lambda item: setattr(
                        item, "stderr", item.stderr.replace("is revoked", "is suspended"),
                    ),
                    "reversed stderr lines": lambda item: setattr(item, "stderr", "".join(reversed(stderr_lines))),
                    "extra stderr output": lambda item: setattr(item, "stderr", item.stderr + "unexpected\n"),
                }
                for label, mutate in result_mutations.items():
                    rejected_result(label, mutate)

                for label, candidate_probe in {
                    "wrong reason descriptor": probe._replace(expected_rejection_reason="wrong reason"),
                    "wrong rejection class": probe._replace(expected_rejection_class="timeout"),
                }.items():
                    rejected_probe(label, candidate_probe)

                for label, result_factory in {
                    "identical result": lambda item: subprocess.CompletedProcess(
                        list(item.args), item.returncode, item.stdout, item.stderr,
                    ),
                    "identical executable output": lambda item: subprocess.run(
                        [sys.executable, "-c", f"import sys;sys.stderr.write({item.stderr!r});raise SystemExit(1)"],
                        text=True, capture_output=True, check=False,
                    ),
                }.items():
                    candidate_session, candidate_completed, candidate_evidence = issue()
                    with self.subTest(label=label):
                        self.assertFalse(verify(
                            candidate_session, result_factory(candidate_completed), candidate_evidence,
                        )["verified"])
                        self.assertFalse(verify(
                            candidate_session, candidate_completed, candidate_evidence,
                        )["verified"])

                def drift_identity(section, field):
                    return lambda value: value[section][field].__setitem__(
                        "device", value[section][field]["device"] + 1,
                    )

                def coherent_path_substitution(value):
                    substituted = scenario_root / "substituted" / expected_binary.name
                    value["path"] = str(substituted)
                    value["parent_path"] = str(substituted.parent)
                    value["pre"]["path"] = str(substituted)
                    value["post"]["path"] = str(substituted)

                evidence_mutations = {
                    "missing binding": lambda value: value.clear(),
                    "wrong mechanism": lambda value: value.__setitem__("mechanism", "path-subprocess-run"),
                    "wrong evidence argv": lambda value: value.__setitem__(
                        "argv", ["agentplugins", *probe.target_argv],
                    ),
                    "wrong descriptor digest": lambda value: value["post"].__setitem__(
                        "sha256", "sha256:" + "0" * 64,
                    ),
                    "wrong descriptor size": lambda value: value["pre"].__setitem__(
                        "size", value["pre"]["size"] + 1,
                    ),
                    "descriptor identity drift": drift_identity("post", "descriptor_identity"),
                    "path identity drift": drift_identity("post", "path_identity"),
                    "parent identity drift": drift_identity("post", "parent_identity"),
                    "path substitution": lambda value: value.__setitem__(
                        "path", str(scenario_root / "substituted" / expected_binary.name),
                    ),
                    "coherent public path substitution": coherent_path_substitution,
                    "missing pre observation": lambda value: value.pop("pre"),
                    "missing post observation": lambda value: value.pop("post"),
                }
                for label, mutate in evidence_mutations.items():
                    rejected_evidence(label, mutate)

                mutation_paths = {
                    "workspace": workspace / "mutation",
                    "manager": scenario_root / "manager" / "mutation",
                    "home": scenario_root / "home" / "mutation",
                    "config": scenario_root / "config" / "mutation",
                    "cache": scenario_root / "cache" / "mutation",
                    "evidence": scenario_root / "evidence" / "mutation",
                    "tmp": scenario_root / "tmp" / "mutation",
                    "Directory cache": Path(environment["AGENTPLUGINS_DIRECTORY_CACHE"]),
                    "git config": Path(environment["GIT_CONFIG_GLOBAL"]),
                }
                for label, path in mutation_paths.items():
                    candidate_session, candidate_completed, candidate_evidence = issue()
                    candidate_before = observer.filesystem_snapshot(root)
                    path.write_text("mutation\n")
                    candidate_after = observer.filesystem_snapshot(root)
                    with self.subTest(root_mutation=label):
                        self.assertFalse(verify(
                            candidate_session, candidate_completed, candidate_evidence,
                            before=candidate_before, after=candidate_after,
                        )["verified"])
                        path.unlink()
                        restored = observer.filesystem_snapshot(root)
                        self.assertFalse(verify(
                            candidate_session, candidate_completed, candidate_evidence,
                            before=restored, after=restored,
                        )["verified"])

                copy_factories = {
                    "copy": lambda value, _session: copy.copy(value),
                    "deepcopy": lambda value, _session: copy.deepcopy(value),
                    "dict": lambda value, _session: dict(value),
                    "json": lambda value, _session: json.loads(json.dumps(value)),
                    "internal lifecycle copy": lambda _value, bound_session: bound_session.command_observations[0],
                }
                for label, copy_factory in copy_factories.items():
                    copied_session, copied_completed, copied_evidence = issue()
                    candidate = copy_factory(copied_evidence, copied_session)
                    with self.subTest(copy_type=label):
                        self.assertFalse(verify(copied_session, copied_completed, candidate)["verified"])
                        self.assertFalse(verify(copied_session, copied_completed, copied_evidence)["verified"])

    def test_authenticated_binary_one_command_revoked_probe_plan_is_descriptor_bound(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            binary = root / "agentplugins"
            expected_stderr = "exact revoked probe rejection\n"
            body = (
                "#!/usr/bin/python3\n"
                "import sys\n"
                "if sys.argv[1:] == ['version']:\n"
                " print('agentplugins 0.1.18')\n"
                "else:\n"
                f" sys.stderr.write({expected_stderr!r})\n"
                " raise SystemExit(1)\n"
            ).encode()
            binary.write_bytes(body); binary.chmod(0o700)
            with (
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(body)),
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", hashlib.sha256(body).hexdigest()),
            ):
                session = observer.AuthenticatedBinaryExecutionSession(
                    binary.resolve(), cwd=root,
                    command_plan=(observer.REVOKED_TARGET_PROBE_ARGV,),
                )
                completed, binding = session.execute(
                    binary.resolve(), list(observer.REVOKED_TARGET_PROBE_ARGV),
                    cwd=root, write_authority=None,
                )
            self.assertEqual(completed.args, ["<authenticated-binary-fd>", *observer.REVOKED_TARGET_PROBE_ARGV])
            self.assertEqual((completed.returncode, completed.stdout, completed.stderr), (1, "", expected_stderr))
            self.assertIs(type(binding), dict)
            self.assertEqual(binding["mechanism"], "linux-raw-execveat-at-empty-path-authenticated-fd")
            self.assertEqual(binding["pre"]["descriptor_identity"], binding["post"]["descriptor_identity"])
            self.assertEqual(binding["pre"]["path_identity"], binding["post"]["path_identity"])
            self.assertEqual(binding["pre"]["parent_identity"], binding["post"]["parent_identity"])
            self.assertEqual(session.command_observations, [dict(binding)])
            self.assertIs(type(session.command_observations[0]), dict)
            self.assertIs(type(copy.deepcopy(session.command_observations[0])), dict)
            self.assertTrue(session._closed)

    def test_revoked_probe_rejects_outside_unbound_and_aliased_writable_roots(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            scenario_root = Path(temporary) / "scenario"
            workspace = scenario_root / "workspace"
            roots = {
                "HOME": scenario_root / "home", "USERPROFILE": scenario_root / "home",
                "AGENTPLUGINS_HOME": scenario_root / "manager", "XDG_CONFIG_HOME": scenario_root / "config",
                "XDG_CACHE_HOME": scenario_root / "cache", "AGENTPLUGINS_EVIDENCE_ROOT": scenario_root / "evidence",
                "TMPDIR": scenario_root / "tmp", "TMP": scenario_root / "tmp", "TEMP": scenario_root / "tmp",
            }
            for path in {workspace, *roots.values()}:
                path.mkdir(parents=True, exist_ok=True)
            environment = {name: str(path) for name, path in roots.items()}
            directory = workspace / "directory"
            directory.mkdir()
            for filename in ("snapshot.json", "envelope.json", "trust.json"):
                (directory / filename).write_text("{}")
            environment.update({
                "AGENTPLUGINS_DIRECTORY_CACHE": str(workspace / "directory-cache"),
                "AGENTPLUGINS_DIRECTORY_SNAPSHOT": str(directory / "snapshot.json"),
                "AGENTPLUGINS_DIRECTORY_ENVELOPE": str(directory / "envelope.json"),
                "AGENTPLUGINS_DIRECTORY_TRUST": str(directory / "trust.json"),
                "GIT_CONFIG_GLOBAL": str(scenario_root / "gitconfig"),
            })
            observer.revoked_probe_scenario_root(workspace, environment)
            for mutation in (
                {**environment, "TMPDIR": ""},
                {**environment, "XDG_CACHE_HOME": str(Path(temporary))},
                {**environment, "AGENTPLUGINS_EVIDENCE_ROOT": environment["XDG_CONFIG_HOME"]},
            ):
                with self.subTest(environment=mutation), self.assertRaisesRegex(ValueError, "unbound|outside|aliased|exactly bound"):
                    observer.revoked_probe_scenario_root(workspace, mutation)

    def test_managed_rollback_constructs_argv_from_exact_contract_tuple(self) -> None:
        process = mock.Mock(returncode=1)
        process.poll.return_value = 1
        process.communicate.return_value = ("", "fault")
        with mock.patch.object(observer.subprocess, "Popen", return_value=process) as popen, mock.patch.object(
            observer, "observe", return_value={},
        ), mock.patch.object(observer, "manager_facts", return_value={"installation_records": 0}), mock.patch.object(
            observer, "materialized_product_mentions", return_value={"codex": 0, "cursor": 0, "kiro": 0},
        ), mock.patch.dict(os.environ, {"HOME": "/tmp/contract-home", "AGENTPLUGINS_HOME": "/tmp/contract-manager"}, clear=False):
            observer.managed_rollback_scenario(Path("binary"), ("codex", "cursor", "kiro"), Path("root"), "challenge")
        self.assertEqual(
            popen.call_args.args[0][1:],
            ["add", "context7", "--target", "codex,cursor,kiro", "--format", "json"],
        )

    def test_authenticated_linux_binary_pin_matches_immutable_0_1_18_release(self) -> None:
        self.assertEqual(observer.RELEASED_AGENTPLUGINS_0_1_18_SIZE, 11_677_880)
        self.assertEqual(
            observer.RELEASED_AGENTPLUGINS_0_1_18_SHA256,
            "9a294d2d117d6be2042aa28f911999edccf051ccbc3f1c7f0f46920cfd6b5779",
        )

    def test_current_authenticated_linux_binary_pin_matches_immutable_0_1_24_release(self) -> None:
        self.assertEqual(observer.RELEASED_AGENTPLUGINS_0_1_24_SIZE, 12_185_784)
        self.assertEqual(
            observer.RELEASED_AGENTPLUGINS_0_1_24_SHA256,
            "e79125f7ffabd11c6e211d6b049c2eb2b36eb1aba3a76ce27cac819aeba1e6ca",
        )
        self.assertEqual(
            observer.released_agentplugins_identity("0.1.24"),
            (12_185_784, "e79125f7ffabd11c6e211d6b049c2eb2b36eb1aba3a76ce27cac819aeba1e6ca"),
        )

    def test_windows_release_preparation_import_does_not_load_linux_libc(self) -> None:
        program = r"""
import ctypes
import platform
import sys

def reject_cdll(*_args, **_kwargs):
    raise AssertionError("Windows preparation attempted to load Linux libc")

ctypes.CDLL = reject_cdll
platform.system = lambda: "Windows"
sys.path.insert(0, sys.argv[1])
import run_launch_evidence_e2e
"""
        completed = subprocess.run(
            [sys.executable, "-c", program, str(ROOT / "scripts")],
            text=True,
            capture_output=True,
            timeout=30,
            check=False,
        )
        self.assertEqual(completed.returncode, 0, completed.stderr)

    def agentplugins_0_1_14_add_fixture(self) -> tuple[bytes, dict]:
        raw = AGENTPLUGINS_0_1_14_ADD.read_bytes()
        value = json.loads(raw)
        self.assertEqual((value["schema_version"], value["command"], value["result"]), (1, "add", "success"))
        self.assertEqual((value["data"]["plugin"], value["data"]["version"]), ("context7", "1.0.0"))
        self.assertEqual(
            (value["data"]["source"], value["data"]["revision"]),
            ("upstash/context7//plugins/agent-plugins/context7", "769c6cd22c3d95462d1f55d789e9532cabefa5a9"),
        )
        return raw, value

    def agentplugins_0_1_14_state_fixture(self) -> tuple[str, dict]:
        raw = AGENTPLUGINS_0_1_14_STATE_V2.read_text()
        value = json.loads(raw)
        self.assertEqual(value["schema_version"], 4)
        installation = value["installations"][0]
        self.assertEqual(len(value["installations"]), 1)
        self.assertEqual(
            (installation["source"]["repository"], installation["source"]["resolved_revision"], installation["source"]["package_subpath"]),
            ("upstash/context7", "769c6cd22c3d95462d1f55d789e9532cabefa5a9", "plugins/agent-plugins/context7"),
        )
        self.assertEqual(
            (
                installation["package"]["loader_kind"], installation["package"]["format_id"],
                installation["package"]["schema_uri"], installation["package"]["version"],
            ),
            (
                "agent_plugins", "agent-plugins/1.0.0",
                "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json", "1.0.0",
            ),
        )
        return raw, value

    def sandbox_state_fixture(self, value: dict, manager: Path) -> dict:
        transformed, record = observer.transform_sanitized_placeholders(
            value,
            original_raw=AGENTPLUGINS_0_1_14_STATE_V2.read_bytes(),
            mappings=(("/fixture/agentplugins-home", manager),),
        )
        self.assertTrue(observer.validate_placeholder_transformation(
            value, transformed, record, original_raw=AGENTPLUGINS_0_1_14_STATE_V2.read_bytes(),
        ))
        return transformed

    def fixture_harness(self, root: Path | None = None, **kwargs):
        return e2e.LaunchHarness(
            None, None, mode="fixture-only", consent=CONSENT, run_root=root, **kwargs
        )

    def test_fixture_only_cli_does_not_require_prepared_context(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            run_root = Path(tmp) / "disposable-run"
            output = run_root / "evidence" / "launch-evidence.json"
            completed = subprocess.run([
                sys.executable, str(MODULE), "--mode", "fixture-only",
                "--uap-sha", "a" * 40,
                "--consent", str(CONSENT), "--run-root", str(run_root),
                "--output", str(output),
            ], cwd=ROOT, text=True, capture_output=True, check=False)
            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertTrue(output.is_file())

    def test_direct_external_fixture_is_a_valid_skill_package(self) -> None:
        skill = (e2e.EXTERNAL_PACKAGE / "skills/fixture/SKILL.md").read_text()
        self.assertTrue(skill.startswith("---\n"))
        frontmatter, body = skill.removeprefix("---\n").split("\n---\n", 1)
        lines = frontmatter.splitlines()
        self.assertIn("name: fixture", lines)
        self.assertTrue(any(line.startswith("description: ") for line in lines))
        self.assertIn("license: Apache-2.0", lines)
        self.assertIn("# External fixture", body)

    def test_direct_package_digest_matches_go_contract_with_directories_and_modes(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            package = Path(tmp)
            (package / "empty").mkdir()
            (package / "bin").mkdir()
            executable = package / "bin" / "run"
            executable.write_bytes(b"run\n")
            executable.chmod(0o755)
            (package / "plain").write_bytes(b"x")
            # Cross-contract vector produced by the Go
            # agentplugins-tree-sha256-v1 snapshotter.
            self.assertEqual(
                e2e.package_digest(package),
                "sha256:2e8071d58dd150284aebbbe1ec7e830afe7228522aa1c4bf1b2eb7d3f1d40143",
            )
            executable.chmod(0o644)
            self.assertEqual(
                e2e.package_digest(package),
                "sha256:ca762bbfbaf48fc3e199ed77dcc61442f42b576c3b9014a642472de8da0879bb",
            )

    def test_central_stable_snapshot_derives_the_same_package_tree_digest(self) -> None:
        self.assertEqual(observer.package_identity(e2e.EXTERNAL_PACKAGE)["tree_digest"], e2e.package_digest(e2e.EXTERNAL_PACKAGE))

    def test_fixture_mode_is_explicitly_non_runtime(self) -> None:
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(e2e, "ROOT", Path("/opt/test-repository")):
            evidence = self.fixture_harness(Path(tmp) / "fresh").export()
        self.assertEqual(evidence["schema_version"], 5)
        self.assertEqual(evidence["run"]["mode"], "fixture-only")
        self.assertFalse(evidence["run"]["runtime_claims"])
        self.assertFalse(evidence["summary"]["required_gates_complete"])
        self.assertEqual(evidence["summary"]["hero_runtime_results"], 0)
        e2e.assert_redacted(evidence)

    def test_enforced_mode_refuses_missing_live_inputs_before_evidence(self) -> None:
        with self.assertRaisesRegex(ValueError, "does not authorize|missing required input"):
            e2e.LaunchHarness(None, None, mode="enforced", consent=CONSENT)

    def test_stable_version_floor(self) -> None:
        for version in ("0.1.6", "0.1.8", "0.1.13", "0.1.14"):
            with self.assertRaisesRegex(ValueError, "0.1.18 or newer"):
                e2e.parse_stable_version(version)
        self.assertEqual(e2e.parse_stable_version("0.1.18"), (0, 1, 18))
        self.assertEqual(e2e.parse_stable_version("1.0.0"), (1, 0, 0))
        with self.assertRaisesRegex(ValueError, "exact semantic version"):
            e2e.parse_stable_version("latest")
        with self.assertRaisesRegex(ValueError, "exact semantic version"):
            e2e.parse_stable_version("0.1.8-rc.1")

    def test_lifecycle_git_commands_scope_safe_directory_to_exact_repository(self) -> None:
        git = "/opt/test-tools/git"
        with (
            mock.patch.object(observer.shutil, "which", return_value=git),
            mock.patch.object(
                observer.subprocess, "run",
                return_value=subprocess.CompletedProcess([], 0, b"", b""),
            ) as run,
        ):
            observer.lifecycle_source_hash()
        run.assert_called_once_with(
            [git, "-c", f"safe.directory={ROOT}", "ls-files", "-z"],
            cwd=ROOT, check=True, capture_output=True, timeout=30,
        )

        repository = Path("/opt/exact-trusted-repository")
        with (
            mock.patch.object(observer.shutil, "which", return_value=git),
            mock.patch.object(
                observer.subprocess, "run",
                return_value=subprocess.CompletedProcess([], 0, b"", b""),
            ) as run,
        ):
            self.assertTrue(observer._owned_sources_match_head(repository))
        run.assert_called_once_with(
            [
                git, "-c", f"safe.directory={repository}", "status",
                "--porcelain=v1", "-z", "--untracked-files=all",
            ],
            cwd=repository, check=False, capture_output=True, timeout=30,
        )
        for command in (run.call_args.args[0],):
            self.assertEqual(command.count("-c"), 1)
            self.assertNotIn("safe.directory=*", command)

    def test_lifecycle_git_failures_remain_fail_closed(self) -> None:
        with mock.patch.object(
            observer.subprocess, "run",
            side_effect=subprocess.CalledProcessError(128, ["git", "ls-files"]),
        ):
            with self.assertRaises(subprocess.CalledProcessError):
                observer.lifecycle_source_hash()
        with mock.patch.object(
            observer.subprocess, "run",
            return_value=subprocess.CompletedProcess([], 0, b"not-nul-terminated", b""),
        ):
            with self.assertRaisesRegex(ValueError, "malformed tracked-path list"):
                observer.lifecycle_source_hash()
        with mock.patch.object(observer.subprocess, "run", side_effect=FileNotFoundError()):
            with self.assertRaises(FileNotFoundError):
                observer.lifecycle_source_hash()
        with mock.patch.object(
            observer.subprocess, "run",
            return_value=subprocess.CompletedProcess([], 128, b"", b"fatal"),
        ):
            self.assertFalse(observer._owned_sources_match_head(ROOT))
        with mock.patch.object(
            observer.subprocess, "run",
            return_value=subprocess.CompletedProcess([], 0, b" M tracked-source.py\0", b""),
        ):
            self.assertFalse(observer._owned_sources_match_head(ROOT))
        with mock.patch.object(observer.subprocess, "run", side_effect=FileNotFoundError()):
            self.assertFalse(observer._owned_sources_match_head(ROOT))
        self.assertFalse(observer._owned_sources_match_head(Path("relative/repository")))

    def test_disposable_lifecycle_evidence_binds_every_replay_input(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            binary = Path(tmp) / "agentplugins"
            binary.write_text("#!/bin/sh\nprintf 'agentplugins 0.1.18\\n'\n")
            binary.chmod(0o700)
            commands = (
                ["add", "./package-plugin-data", "--target", "cursor", "--format", "json"],
                ["info", "e2e-external-package", "--target", "cursor", "--format", "json"],
                ["update", "e2e-external-package", "--target", "cursor", "--format", "json"],
                ["repair", "e2e-external-package", "--target", "cursor", "--format", "json"],
                ["switch", "e2e-external-package", "--to", "./package-plugin-data-alternate", "--format", "json"],
                ["remove", "e2e-external-package", "--target", "cursor", "--format", "json"],
                ["remove", "e2e-external-package", "--purge-data", "--format", "json"],
            )
            traces = [{
                "argv": command, "exit_code": 0,
                "stdout_digest": "sha256:" + hashlib.sha256("\0".join(command).encode()).hexdigest(),
                "stderr_digest": "sha256:" + hashlib.sha256(b"").hexdigest(),
                "process_creation_denied": True, "write_guarded": index != 0,
            } for index, command in enumerate(commands)]
            proof = {
                "info_preserved": True, "update_changed_package_digest": True,
                "update_preserved": True, "update_preserved_data_receipt": True,
                "repair_preserved": True, "switch_preserved": True,
                "remove_preserved": True, "explicit_owned_purge_deleted": True,
            }
            session = observer.LifecycleEvidenceSession()
            test_result = subprocess.CompletedProcess([], 0, "", "Ran 165 tests in 1.000s\n\nOK\n")
            with mock.patch.object(observer.subprocess, "run", return_value=test_result):
                test_execution = observer.TestExecutionSession.run_phase6(cwd=ROOT)
            with (
                mock.patch.object(observer, "_owned_sources_match_head", return_value=True),
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", binary.stat().st_size),
                mock.patch.object(
                    observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256",
                    hashlib.sha256(binary.read_bytes()).hexdigest(),
                ),
            ):
                execution_session = observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=ROOT)
                for index, command in enumerate(commands):
                    completed, binding = execution_session.execute(
                        binary.resolve(), command, cwd=ROOT, write_authority=None,
                    )
                    self.assertEqual(completed.returncode, 0)
                    traces[index]["binary_execution"] = binding
                execution_descriptors = (
                    execution_session._fd, execution_session._parent_fd,
                    execution_session._inotify_fd,
                )
                binary_execution = execution_session.finalize()
                self.assertEqual(binary_execution["commands"], execution_session.command_observations)
                self.assertEqual(len(binary_execution["commands"]), 7)
                self.assertTrue(all(type(command) is dict for command in binary_execution["commands"]))
                self.assertEqual(json.loads(json.dumps(binary_execution["commands"])), binary_execution["commands"])
                for descriptor in execution_descriptors:
                    with self.assertRaises(OSError):
                        os.fstat(descriptor)
                with self.assertRaisesRegex(ValueError, "cannot be reused"):
                    execution_session.execute(
                        binary.resolve(), commands[0], cwd=ROOT, write_authority=None,
                    )
                record = observer.bound_lifecycle_evidence(
                    binary.resolve(), traces, proof, session=session,
                    test_execution=test_execution, binary_execution=binary_execution,
                )
                with self.assertRaisesRegex(ValueError, "fresh pre-command"):
                    observer.bound_lifecycle_evidence(
                        binary.resolve(), traces, proof, session=session,
                        test_execution=test_execution, binary_execution=binary_execution,
                    )
        self.assertEqual(record["source_hash_before"], record["source_hash_after"])
        self.assertEqual(len(record["commands"]), 7)
        self.assertEqual(record["binary"]["version_stdout"], "agentplugins 0.1.18")
        self.assertEqual(record["binary"]["version_argv"], ["<authenticated-binary-fd>", "version"])
        self.assertEqual(record["binary"]["version_exit"], 0)
        self.assertEqual(
            record["binary"]["execution_session"]["mechanism"],
            "linux-raw-execveat-at-empty-path-authenticated-fd",
        )
        self.assertEqual(record["binary"]["execution_session"]["syscall_number"], 322)
        self.assertTrue(record["binary"]["execution_session"]["empty_path"])
        self.assertTrue(record["binary"]["execution_session"]["at_empty_path"])
        self.assertNotIn("/proc/self/fd", json.dumps(record))
        self.assertEqual(record["tests"]["skips"], 0)
        self.assertEqual(record["host"]["uid"], os.getuid())
        self.assertEqual(record["host"]["gid"], os.getgid())
        self.assertRegex(record["repository"]["commit"], r"^[0-9a-f]{40}$")
        self.assertRegex(record["repository"]["parent"], r"^[0-9a-f]{40}$")
        self.assertRegex(record["repository"]["patch_sha256"], r"^sha256:[0-9a-f]{64}$")

    def test_bound_evidence_harness_owns_fresh_credential_free_roots(self) -> None:
        inherited_home = os.environ.get("HOME")
        observed: dict[str, object] = {}

        def lifecycle(binary: Path, scenario_targets: tuple[str, ...], root: Path, challenge: str, *, binary_session):
            self.assertEqual(scenario_targets, ("cursor",))
            observed.update({
                "binary": binary, "root": root, "challenge": challenge,
                "home": os.environ.get("HOME"), "manager": os.environ.get("AGENTPLUGINS_HOME"),
                "tmp": os.environ.get("TMPDIR"), "secret": os.environ.get("PHASE6_TEST_SECRET"),
            })
            return True, {"command_traces": [], "proof": {}}

        with tempfile.TemporaryDirectory() as tmp:
            parent = Path(tmp)
            binary = parent / "agentplugins"
            binary.write_text("exact-binary-placeholder")
            root = parent / "fresh-evidence-root"
            test_execution = object()
            authenticated_session = mock.Mock()
            authenticated_session.finalize.return_value = {"finalized": True}
            with (
                mock.patch.dict(os.environ, {"PHASE6_TEST_SECRET": "must-not-propagate"}),
                mock.patch.object(observer, "AuthenticatedBinaryExecutionSession", return_value=authenticated_session),
                mock.patch.object(observer.TestExecutionSession, "run_phase6", return_value=test_execution),
                mock.patch.object(observer, "plugin_data_scenario", side_effect=lifecycle),
                mock.patch.object(observer, "bound_lifecycle_evidence", return_value={"bound": True}) as bind,
            ):
                self.assertEqual(
                    observer.run_bound_plugin_data_evidence(binary, root, "challenge"),
                    {"bound": True},
                )
                self.assertEqual(os.environ.get("PHASE6_TEST_SECRET"), "must-not-propagate")
            self.assertTrue(root.is_dir())
            self.assertEqual(observed["binary"], binary.resolve())
            self.assertEqual(observed["root"], root / "workspace")
            self.assertEqual(observed["challenge"], "challenge")
            self.assertEqual(observed["home"], str(root / "lifecycle-home"))
            self.assertEqual(observed["manager"], str(root / "lifecycle-manager"))
            self.assertEqual(observed["tmp"], str(root / "lifecycle-tmp"))
            self.assertIsNone(observed["secret"])
            bind.assert_called_once()
            self.assertEqual(bind.call_args.kwargs["binary_execution"], {"finalized": True})
        self.assertEqual(os.environ.get("HOME"), inherited_home)

    def test_authenticated_binary_session_rejects_swap_restore_before_execution(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / "agentplugins"
            body = b"#!/bin/sh\nprintf 'agentplugins 0.1.18\\n'\n"
            binary.write_bytes(body); binary.chmod(0o700)
            with (
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(body)),
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", hashlib.sha256(body).hexdigest()),
            ):
                session = observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)
                original = root / "authenticated-original"
                malicious = root / "malicious"
                malicious.write_text("#!/bin/sh\ntouch malicious-executed\n"); malicious.chmod(0o700)
                binary.rename(original); malicious.rename(binary)
                binary.rename(malicious); original.rename(binary)
                with self.assertRaisesRegex(ValueError, "identity epoch changed"):
                    session.execute(
                        binary.resolve(), list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]),
                        cwd=root, write_authority=None,
                    )
                session.abort()
            self.assertFalse((root / "malicious-executed").exists())

    def test_authenticated_descriptor_executes_original_and_rejects_during_command_swap_restore(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / "agentplugins"
            signal = root / "command-started"
            release = root / "release-command"
            marker = root / "malicious-executed"
            body = (
                "#!/usr/bin/python3\n"
                "import os, sys, time\n"
                "if sys.argv[1:] == ['version']:\n"
                " print('agentplugins 0.1.18')\n"
                "else:\n"
                " open(os.environ['PHASE6_SWAP_SIGNAL'], 'wb').close()\n"
                " while not os.path.exists(os.environ['PHASE6_SWAP_RELEASE']): time.sleep(0.01)\n"
            ).encode()
            binary.write_bytes(body); binary.chmod(0o700)
            inherited = os.environ.copy()
            os.environ.update({"PHASE6_SWAP_SIGNAL": str(signal), "PHASE6_SWAP_RELEASE": str(release)})
            try:
                with (
                    mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(body)),
                    mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", hashlib.sha256(body).hexdigest()),
                ):
                    session = observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)

                    def swap_restore() -> None:
                        deadline = time.monotonic() + 10
                        while not signal.exists() and time.monotonic() < deadline:
                            time.sleep(0.005)
                        original = root / "authenticated-original"
                        malicious = root / "malicious"
                        malicious.write_text(f"#!/bin/sh\ntouch {marker}\n"); malicious.chmod(0o700)
                        binary.rename(original); malicious.rename(binary)
                        binary.rename(malicious); original.rename(binary)
                        release.write_bytes(b"continue")

                    attacker = os.fork()
                    if attacker == 0:
                        try:
                            swap_restore()
                        except BaseException:
                            os._exit(1)
                        os._exit(0)
                    with self.assertRaisesRegex(ValueError, "identity epoch changed"):
                        session.execute(
                            binary.resolve(), list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]),
                            cwd=root, write_authority=None,
                        )
                    waited, status = os.waitpid(attacker, 0)
                    self.assertEqual(waited, attacker)
                    self.assertEqual(os.waitstatus_to_exitcode(status), 0)
                    session.abort()
            finally:
                os.environ.clear(); os.environ.update(inherited)
            self.assertFalse(marker.exists())

    def test_authenticated_binary_session_fails_closed_on_early_close_and_incomplete_set(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); binary = root / "agentplugins"
            body = b"#!/bin/sh\nprintf 'agentplugins 0.1.18\\n'\n"
            binary.write_bytes(body); binary.chmod(0o700)
            with (
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(body)),
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", hashlib.sha256(body).hexdigest()),
            ):
                incomplete = observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)
                descriptors = (incomplete._fd, incomplete._parent_fd, incomplete._inotify_fd)
                with self.assertRaisesRegex(ValueError, "incomplete command set"):
                    incomplete.finalize()
                for descriptor in descriptors:
                    with self.assertRaises(OSError):
                        os.fstat(descriptor)
                with self.assertRaisesRegex(ValueError, "cannot be reused"):
                    incomplete.execute(
                        binary.resolve(), list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]),
                        cwd=root, write_authority=None,
                    )

                observation_failure = observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)
                observation_descriptors = (
                    observation_failure._fd, observation_failure._parent_fd,
                    observation_failure._inotify_fd,
                )
                observation_failure._next_command = len(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV)
                with (
                    mock.patch.object(observation_failure, "_observe", side_effect=ValueError("final observation failed")),
                    self.assertRaisesRegex(ValueError, "final observation failed"),
                ):
                    observation_failure.finalize()
                for descriptor in observation_descriptors:
                    with self.assertRaises(OSError):
                        os.fstat(descriptor)

    def test_authenticated_binary_session_rejects_links_wrong_bytes_version_and_path(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); binary = root / "agentplugins"
            body = b"#!/bin/sh\nprintf 'wrong version\\n'\n"
            binary.write_bytes(body); binary.chmod(0o700)
            link = root / "agentplugins-link"; link.symlink_to(binary.name)
            with self.assertRaises(OSError):
                observer.AuthenticatedBinaryExecutionSession(link, cwd=root)
            with (
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(body)),
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", "0" * 64),
                self.assertRaisesRegex(ValueError, "authenticated public"),
            ):
                observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)
            with (
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(body)),
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", hashlib.sha256(body).hexdigest()),
                self.assertRaisesRegex(ValueError, "exact released"),
            ):
                observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)

            good = b"#!/bin/sh\nprintf 'agentplugins 0.1.18\\n'\n"
            binary.write_bytes(good)
            with (
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(good)),
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", hashlib.sha256(good).hexdigest()),
            ):
                session = observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)
                with self.assertRaisesRegex(ValueError, "wrong path"):
                    session.execute(
                        root / "different", list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]),
                        cwd=root, write_authority=None,
                    )
                self.assertTrue(session._closed)

    def test_authenticated_binary_execution_uses_only_an_integer_fd_authority(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); binary = root / "agentplugins"
            body = b"#!/bin/sh\nprintf 'agentplugins 0.1.18\\n'\n"
            binary.write_bytes(body); binary.chmod(0o700)
            marker = root / "exec-authority"
            with (
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(body)),
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", hashlib.sha256(body).hexdigest()),
            ):
                session = observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)
                real_execveat = observer._raw_execveat

                def audited_execveat(authority, argv, environment):
                    marker.write_text(f"{type(authority).__name__}:{authority}\n")
                    return real_execveat(authority, argv, environment)

                # A fabricated pathname bearing the old predictable spelling
                # exists, but descriptor execution never consults it.
                fabricated = root / "proc" / "self" / "fd"
                fabricated.mkdir(parents=True)
                (fabricated / str(session._fd)).write_text("malicious pathname")
                with mock.patch.object(observer, "_raw_execveat", side_effect=audited_execveat):
                    completed, binding = session.execute(
                        binary.resolve(), list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]),
                        cwd=root, write_authority=None,
                    )
                self.assertEqual(completed.returncode, 0)
                self.assertRegex(marker.read_text(), r"^int:[0-9]+\n$")
                self.assertEqual(
                    binding["mechanism"], "linux-raw-execveat-at-empty-path-authenticated-fd",
                )
                self.assertEqual(binding["syscall_number"], 322)
                self.assertTrue(binding["empty_path"])
                self.assertTrue(binding["at_empty_path"])
                self.assertFalse(binding["descriptor_inheritable_in_observer"])
                source = Path(observer.__file__).read_text()
                execution_source = source[
                    source.index("def _raw_execveat"):source.index("class TestExecutionSession")
                ]
                for forbidden in ("os.execve", "fexecve", "/proc/self/fd"):
                    self.assertNotIn(forbidden, execution_source)
                self.assertIn("ctypes.c_char_p(b\"\")", execution_source)
                self.assertIn("ctypes.c_int(_AT_EMPTY_PATH)", execution_source)
                session.abort()

    def test_elf_execution_uses_the_original_authenticated_cloexec_fd(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            elf = Path(shutil.which("true") or "/usr/bin/true")
            descriptor = os.open(elf, os.O_RDONLY | getattr(os, "O_CLOEXEC", 0))
            self.assertFalse(os.get_inheritable(descriptor))
            session = object.__new__(observer.AuthenticatedBinaryExecutionSession)
            session._fd = descriptor
            session._parent_fd = -1
            session._inotify_fd = -1
            session._body = b"\x7fELF"
            session._child_pid = None
            session._last_child_pid = None
            marker = root / "elf-exec-authority"
            real_execveat = observer._raw_execveat

            def audited_execveat(authority, argv, environment):
                marker.write_text(json.dumps({
                    "authority": authority,
                    "inheritable": os.get_inheritable(authority),
                    "argv": argv,
                }))
                return real_execveat(authority, argv, environment)

            try:
                with mock.patch.object(observer, "_raw_execveat", side_effect=audited_execveat):
                    completed = session._run_descriptor([], cwd=root, write_authority=None)
                self.assertEqual(completed.returncode, 0, completed.stderr)
                self.assertEqual(completed.stdout, "")
                self.assertEqual(completed.stderr, "")
                audit = json.loads(marker.read_text())
                self.assertEqual(audit, {
                    "authority": descriptor,
                    "inheritable": False,
                    "argv": ["<authenticated-binary-fd>"],
                })
                self.assertIsNone(session._child_pid)
                with self.assertRaises(ChildProcessError):
                    os.waitpid(session._last_child_pid, os.WNOHANG)
            finally:
                os.close(descriptor)

    def test_authenticated_image_cannot_inherit_an_unrelated_inheritable_descriptor(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / "agentplugins"
            token = b"unrelated-inheritable-descriptor"
            read_fd, write_fd = os.pipe()
            os.set_inheritable(read_fd, True)
            os.write(write_fd, token)
            body = (
                "#!/usr/bin/python3\nimport os,sys\n"
                "try:\n inherited = os.read(int(os.environ['UAP_UNRELATED_FD']), 31)\n"
                "except OSError:\n inherited = b''\n"
                f"if inherited == {token!r}:\n sys.stderr.write('unrelated descriptor inherited\\n');raise SystemExit(91)\n"
                "if sys.argv[1:] == ['version']:\n print('agentplugins 0.1.18')\n"
                "else:\n sys.stderr.write('exact revoked probe rejection\\n');raise SystemExit(1)\n"
            ).encode()
            binary.write_bytes(body)
            binary.chmod(0o700)
            try:
                with (
                    mock.patch.dict(os.environ, {"UAP_UNRELATED_FD": str(read_fd)}),
                    mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(body)),
                    mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", hashlib.sha256(body).hexdigest()),
                ):
                    session = observer.AuthenticatedBinaryExecutionSession(
                        binary.resolve(), cwd=root,
                        command_plan=(observer.REVOKED_TARGET_PROBE_ARGV,),
                    )
                    completed, _evidence = session.execute(
                        binary.resolve(), list(observer.REVOKED_TARGET_PROBE_ARGV),
                        cwd=root, write_authority=None,
                    )
                self.assertEqual((completed.returncode, completed.stdout, completed.stderr), (
                    1, "", "exact revoked probe rejection\n",
                ))
                self.assertTrue(session._closed)
                self.assertTrue(os.get_inheritable(read_fd))
                self.assertEqual(os.read(read_fd, len(token)), token)
            finally:
                os.close(read_fd)
                os.close(write_fd)

    def test_authenticated_binary_fd_exec_unavailable_fails_enotsup_without_path_fallback(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); binary = root / "agentplugins"
            body = b"#!/bin/sh\nprintf 'agentplugins 0.1.18\\n'\n"
            binary.write_bytes(body); binary.chmod(0o700)
            with (
                mock.patch.object(observer.platform, "machine", return_value="unsupported-test-arch"),
                mock.patch.object(observer.os, "open", wraps=os.open) as opened,
            ):
                with self.assertRaises(OSError) as unavailable:
                    observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)
                opened.assert_not_called()
            self.assertEqual(unavailable.exception.errno, errno.ENOTSUP)
            with (
                mock.patch.object(observer._LIBC, "syscall", None),
                mock.patch.object(observer.os, "open", wraps=os.open) as opened,
            ):
                with self.assertRaises(OSError) as unavailable:
                    observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)
                opened.assert_not_called()
            self.assertEqual(unavailable.exception.errno, errno.ENOTSUP)

            with (
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(body)),
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", hashlib.sha256(body).hexdigest()),
            ):
                session = observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)
                with mock.patch.object(observer, "_raw_execveat", side_effect=OSError(errno.ENOTSUP, "missing")):
                    with self.assertRaises(OSError) as runtime_unavailable:
                        session.execute(
                            binary.resolve(), list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]),
                            cwd=root, write_authority=None,
                        )
                self.assertEqual(runtime_unavailable.exception.errno, errno.ENOTSUP)
                self.assertTrue(session._closed)
                self.assertIsNone(session._child_pid)

    def test_raw_execveat_syscall_map_proves_exact_x86_64_host_number(self) -> None:
        self.assertEqual(platform.system(), "Linux")
        self.assertIn(platform.machine().lower(), {"amd64", "x86_64"})
        self.assertEqual(observer._execveat_syscall_number(), 322)
        self.assertEqual(observer._EXECVEAT_SYSCALLS, {"x86_64": 322, "aarch64": 281})

    def test_raw_execveat_rejects_embedded_nul_before_syscall(self) -> None:
        calls = []

        def forbidden_syscall(*arguments):
            calls.append(arguments)
            ctypes.set_errno(errno.EPERM)
            return -1

        with mock.patch.object(observer._LIBC, "syscall", side_effect=forbidden_syscall):
            with self.assertRaisesRegex(ValueError, "valid integer descriptor"):
                observer._raw_execveat(True, ["agentplugins"], {})
            with self.assertRaisesRegex(ValueError, "embedded NUL in argv"):
                observer._raw_execveat(3, ["agentplugins", "bad\0argument"], {})
            with self.assertRaisesRegex(ValueError, "embedded NUL in environment"):
                observer._raw_execveat(3, ["agentplugins"], {"KEY": "bad\0value"})
        self.assertEqual(calls, [])

    def test_inherited_seccomp_execveat_enosys_fails_closed_without_procfs_fallback(self) -> None:
        code = r'''
import ctypes, errno, hashlib, importlib.util, json, os, pathlib, sys, tempfile
root = pathlib.Path(os.environ["PHASE6_REPOSITORY"])
sys.path.insert(0, str(root / "scripts"))
spec = importlib.util.spec_from_file_location("isolated_observer", root / "scripts/observe_launch_scenario.py")
module = importlib.util.module_from_spec(spec); spec.loader.exec_module(module)

class Filter(ctypes.Structure):
    _fields_ = [("code", ctypes.c_ushort), ("jt", ctypes.c_ubyte), ("jf", ctypes.c_ubyte), ("k", ctypes.c_uint32)]
class Program(ctypes.Structure):
    _fields_ = [("length", ctypes.c_ushort), ("filter", ctypes.POINTER(Filter))]

def _closed(fd):
    try:
        os.fstat(fd)
    except OSError:
        return True
    return False

with tempfile.TemporaryDirectory() as temporary:
    directory = pathlib.Path(temporary)
    binary = directory / "agentplugins"
    body = b"#!/bin/sh\nprintf 'agentplugins 0.1.18\\n'\n"
    binary.write_bytes(body); binary.chmod(0o700)
    module.RELEASED_AGENTPLUGINS_0_1_18_SIZE = len(body)
    module.RELEASED_AGENTPLUGINS_0_1_18_SHA256 = hashlib.sha256(body).hexdigest()
    session = module.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=directory)
    descriptors = [session._fd, session._parent_fd, session._inotify_fd]
    fabricated = directory / "proc/self/fd"; fabricated.mkdir(parents=True)
    malicious_marker = directory / "malicious-executed"
    target = fabricated / str(session._fd)
    target.write_text("#!/bin/sh\ntouch " + str(malicious_marker) + "\n")
    target.chmod(0o700)

    number = module._execveat_syscall_number()
    return_errno = 0x00050000 | errno.ENOSYS
    allow = 0x7fff0000
    instructions = (Filter * 4)(
        Filter(0x20, 0, 0, 0),
        Filter(0x15, 0, 1, number),
        Filter(0x06, 0, 0, return_errno),
        Filter(0x06, 0, 0, allow),
    )
    program = Program(len(instructions), instructions)
    libc = ctypes.CDLL(None, use_errno=True)
    if libc.prctl(38, 1, 0, 0, 0) != 0 or libc.prctl(22, 2, ctypes.byref(program), 0, 0) != 0:
        raise OSError(ctypes.get_errno(), "could not install inherited ENOSYS filter")
    try:
        session.execute(
            binary.resolve(), list(module.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]),
            cwd=directory, write_authority=None,
        )
        outcome = {"unexpected_success": True}
    except OSError as error:
        outcome = {
            "errno": error.errno,
            "closed": session._closed,
            "child_pid": session._child_pid,
            "last_child_pid": session._last_child_pid,
            "malicious_executed": malicious_marker.exists(),
            "fabricated_target_exists": target.exists(),
            "descriptors_closed": all(_closed(fd) for fd in descriptors),
        }
        try:
            os.waitpid(session._last_child_pid, os.WNOHANG)
            outcome["reaped"] = False
        except ChildProcessError:
            outcome["reaped"] = True
    print(json.dumps(outcome))
'''
        # The fabricated tree is deliberately not bind-mounted over real
        # procfs: that requires privileges unavailable to the genuine-nonroot
        # suite. The inherited kernel filter is real; the source regression
        # above deterministically proves there is no compatibility path.
        completed = subprocess.run(
            [sys.executable, "-c", code], cwd=ROOT, text=True, capture_output=True,
            check=False, env={**os.environ, "PHASE6_REPOSITORY": str(ROOT)}, timeout=30,
        )
        self.assertEqual(completed.returncode, 0, completed.stderr)
        outcome = json.loads(completed.stdout)
        self.assertEqual(outcome["errno"], errno.ENOTSUP)
        self.assertTrue(outcome["closed"])
        self.assertIsNone(outcome["child_pid"])
        self.assertTrue(outcome["reaped"])
        self.assertTrue(outcome["descriptors_closed"])
        self.assertTrue(outcome["fabricated_target_exists"])
        self.assertFalse(outcome["malicious_executed"])

    def test_authenticated_binary_multithreaded_observer_fails_closed_before_fork(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); binary = root / "agentplugins"
            body = b"#!/bin/sh\nprintf 'agentplugins 0.1.18\\n'\n"
            binary.write_bytes(body); binary.chmod(0o700)
            release = threading.Event()
            started = threading.Event()

            def hold_thread() -> None:
                started.set()
                release.wait(timeout=10)

            with (
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(body)),
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", hashlib.sha256(body).hexdigest()),
            ):
                session = observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)
                descriptors = (session._fd, session._parent_fd, session._inotify_fd)
                blocker = threading.Thread(target=hold_thread)
                blocker.start()
                self.assertTrue(started.wait(timeout=10))
                try:
                    with self.assertRaises(OSError) as busy:
                        session.execute(
                            binary.resolve(), list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]),
                            cwd=root, write_authority=None,
                        )
                    self.assertEqual(busy.exception.errno, errno.EBUSY)
                    self.assertTrue(session._closed)
                    self.assertIsNone(session._child_pid)
                    for descriptor in descriptors:
                        with self.assertRaises(OSError):
                            os.fstat(descriptor)
                finally:
                    release.set()
                    blocker.join(timeout=10)
                self.assertFalse(blocker.is_alive())

    def test_authenticated_binary_modify_restore_and_replacement_epochs_fail_closed(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); binary = root / "agentplugins"
            body = b"#!/bin/sh\nprintf 'agentplugins 0.1.18\\n'\n"
            binary.write_bytes(body); binary.chmod(0o700)
            with (
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(body)),
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", hashlib.sha256(body).hexdigest()),
            ):
                modified = observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)
                binary.write_bytes(body + b"# changed\n"); binary.write_bytes(body); binary.chmod(0o700)
                with self.assertRaisesRegex(ValueError, "identity epoch changed"):
                    modified.execute(
                        binary.resolve(), list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]),
                        cwd=root, write_authority=None,
                    )
                self.assertTrue(modified._closed)

                replaced = observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)
                saved = root / "saved-agentplugins"
                replacement = root / "replacement"
                replacement.write_bytes(body); replacement.chmod(0o700)
                binary.rename(saved); replacement.rename(binary)
                with self.assertRaisesRegex(ValueError, "identity epoch changed|identity changed"):
                    replaced.execute(
                        binary.resolve(), list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]),
                        cwd=root, write_authority=None,
                    )
                self.assertTrue(replaced._closed)

                binary.unlink(); saved.rename(binary)
                symlink_swap = observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)
                saved_again = root / "saved-again"
                binary.rename(saved_again); binary.symlink_to(saved_again.name)
                with self.assertRaisesRegex(ValueError, "identity epoch changed|disappeared|identity changed"):
                    symlink_swap.execute(
                        binary.absolute(), list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]),
                        cwd=root, write_authority=None,
                    )
                self.assertTrue(symlink_swap._closed)

    def test_authenticated_binary_all_execute_failures_close_authority_and_session(self) -> None:
        def make_session(root: Path) -> tuple[observer.AuthenticatedBinaryExecutionSession, Path, bytes]:
            binary = root / "agentplugins"
            body = b"#!/bin/sh\nprintf 'agentplugins 0.1.18\\n'\n"
            binary.write_bytes(body); binary.chmod(0o700)
            return observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root), binary, body

        def authority(root: Path) -> int:
            return os.open(root, os.O_RDONLY | os.O_DIRECTORY | getattr(os, "O_CLOEXEC", 0))

        def assert_closed(session, supplied: int) -> None:
            self.assertTrue(session._closed)
            self.assertIsNone(session._child_pid)
            for descriptor in (supplied, session._fd, session._parent_fd, session._inotify_fd):
                with self.assertRaises(OSError):
                    os.fstat(descriptor)

        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            body = b"#!/bin/sh\nprintf 'agentplugins 0.1.18\\n'\n"
            with (
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(body)),
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", hashlib.sha256(body).hexdigest()),
            ):
                for name, operation, pattern in (
                    ("wrong-path", lambda s, b, fd: s.execute(base / "other", list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]), cwd=base, write_authority=(fd,)), "wrong path"),
                    ("out-of-order", lambda s, b, fd: s.execute(b, list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[1]), cwd=base, write_authority=(fd,)), "out-of-order"),
                ):
                    root = base / name; root.mkdir()
                    session, binary, _ = make_session(root); supplied = authority(root)
                    with self.assertRaisesRegex(ValueError, pattern):
                        operation(session, binary.resolve(), supplied)
                    assert_closed(session, supplied)

                root = base / "pre"; root.mkdir()
                session, binary, _ = make_session(root); supplied = authority(root)
                with mock.patch.object(session, "_observe", side_effect=ValueError("pre observation failed")):
                    with self.assertRaisesRegex(ValueError, "pre observation failed"):
                        session.execute(binary.resolve(), list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]), cwd=root, write_authority=(supplied,))
                assert_closed(session, supplied)

                root = base / "execution"; root.mkdir()
                session, binary, _ = make_session(root); supplied = authority(root)
                with mock.patch.object(session, "_run_descriptor", side_effect=OSError(errno.EIO, "execution failed")):
                    with self.assertRaisesRegex(OSError, "execution failed"):
                        session.execute(binary.resolve(), list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]), cwd=root, write_authority=(supplied,))
                assert_closed(session, supplied)

                root = base / "post"; root.mkdir()
                session, binary, _ = make_session(root); supplied = authority(root)
                real_observe = session._observe
                observations = 0

                def fail_post(stage):
                    nonlocal observations
                    observations += 1
                    if observations == 2:
                        raise ValueError("post observation failed")
                    return real_observe(stage)

                with (
                    mock.patch.dict(os.environ, {"AGENTPLUGINS_HOME": str(root)}),
                    mock.patch.object(session, "_observe", side_effect=fail_post),
                    self.assertRaisesRegex(ValueError, "post observation failed"),
                ):
                    session.execute(binary.resolve(), list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]), cwd=root, write_authority=(supplied,))
                assert_closed(session, supplied)

    def test_authenticated_binary_timeout_reaps_child_and_closes_every_descriptor(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); binary = root / "agentplugins"
            body = (
                "#!/usr/bin/python3\n"
                "import sys, time\n"
                "print('agentplugins 0.1.18') if sys.argv[1:] == ['version'] else time.sleep(60)\n"
            ).encode()
            binary.write_bytes(body); binary.chmod(0o700)
            supplied = os.open(root, os.O_RDONLY | os.O_DIRECTORY | getattr(os, "O_CLOEXEC", 0))
            with (
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(body)),
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", hashlib.sha256(body).hexdigest()),
            ):
                session = observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)
                descriptors = (session._fd, session._parent_fd, session._inotify_fd)
                with (
                    mock.patch.dict(os.environ, {"AGENTPLUGINS_HOME": str(root)}),
                    mock.patch.object(session, "_EXECUTION_TIMEOUT_SECONDS", 0.05),
                    self.assertRaises(subprocess.TimeoutExpired),
                ):
                    session.execute(binary.resolve(), list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]), cwd=root, write_authority=(supplied,))
                child = session._last_child_pid
                self.assertIsNotNone(child)
                self.assertIsNone(session._child_pid)
                with self.assertRaises(ChildProcessError):
                    os.waitpid(child, os.WNOHANG)
                for descriptor in (supplied, *descriptors):
                    with self.assertRaises(OSError):
                        os.fstat(descriptor)

    def test_authenticated_binary_fd_exec_preserves_output_status_cwd_and_environment(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); cwd = root / "cwd"; cwd.mkdir()
            binary = root / "agentplugins"
            body = (
                "#!/usr/bin/python3\n"
                "import os, sys\n"
                "if sys.argv[1:] == ['version']:\n"
                " print('agentplugins 0.1.18')\n"
                "else:\n"
                " os.write(1, (os.getcwd() + '\\n' + os.environ['PHASE6_FD_ENV'] + '\\n').encode() + b'o' * 200000)\n"
                " os.write(2, b'e' * 200000)\n"
                " raise SystemExit(23)\n"
            ).encode()
            binary.write_bytes(body); binary.chmod(0o700)
            with (
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SIZE", len(body)),
                mock.patch.object(observer, "RELEASED_AGENTPLUGINS_0_1_18_SHA256", hashlib.sha256(body).hexdigest()),
                mock.patch.dict(os.environ, {"PHASE6_FD_ENV": "exact-environment"}),
            ):
                session = observer.AuthenticatedBinaryExecutionSession(binary.resolve(), cwd=root)
                completed, _binding = session.execute(
                    binary.resolve(), list(observer.EXACT_PLUGIN_DATA_LIFECYCLE_ARGV[0]),
                    cwd=cwd, write_authority=None,
                )
                self.assertEqual(completed.returncode, 23)
                self.assertEqual(completed.stdout, f"{cwd}\nexact-environment\n" + "o" * 200000)
                self.assertEqual(completed.stderr, "e" * 200000)
                self.assertIsNone(session._child_pid)
                self.assertTrue(session._closed)
                child = session._last_child_pid
                with self.assertRaises(ChildProcessError):
                    os.waitpid(child, os.WNOHANG)

    def test_challenge_binds_github_release_directory_and_root(self) -> None:
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(e2e.secrets, "token_hex", return_value="ab" * 32):
            caller = ("push", "refs/heads/main", "777genius/universal-agent-plugins/.github/workflows/directory-publication.yml@refs/heads/main")
            scenario = e2e.sha256_file(e2e.SCENARIOS)
            first = e2e.make_challenge("a" * 40, "12", "3", *caller, "sha256:" + "b" * 64, "sha256:" + "c" * 64, scenario, Path(tmp))
            changed = e2e.make_challenge("a" * 40, "12", "3", *caller, "sha256:" + "d" * 64, "sha256:" + "c" * 64, scenario, Path(tmp) / "producer")
            self.assertNotEqual(first["value"], changed["value"])
        self.assertEqual(first["github_sha"], "a" * 40)
        self.assertEqual(first["scenario_contract_digest"], e2e.sha256_file(e2e.SCENARIOS))
        self.assertNotIn("caller_event_name", first)
        self.assertEqual(first["root_id"], e2e.logical_root_id("a" * 40, "12", "3"))
        self.assertEqual(
            first["root_id"],
            e2e.make_challenge("a" * 40, "12", "3", *caller, "sha256:" + "b" * 64, "sha256:" + "c" * 64, scenario, Path(tmp) / "other-job")["root_id"],
        )
        self.assertTrue(e2e.challenge_context_valid(first))
        self.assertFalse(e2e.challenge_context_valid({**first, "directory_digest": "sha256:" + "d" * 64}))

    def complete_scenario_context(self) -> dict:
        snapshot = json.loads((ROOT / "tests/fixtures/directory-publication/snapshot.json").read_text())
        product = snapshot["products"][0]
        distribution = snapshot["distributions"][0]
        selected = distribution["releases"][0]
        policy = distribution["release_policies"][0]
        challenge = e2e.make_challenge(
            "a" * 40, "12", "3", "push", "refs/heads/main",
            "777genius/universal-agent-plugins/.github/workflows/directory-publication.yml@refs/heads/main",
            "sha256:" + "b" * 64, "sha256:" + "c" * 64, e2e.sha256_file(e2e.SCENARIOS), Path("/tmp/unused"),
        )
        release = {
            "product_id": product["id"], "distribution_id": distribution["id"],
            "distribution_kind": distribution["kind"], "release_sequence": selected["sequence"],
            "package_version": selected["package_version"], "tree_digest": selected["tree_digest"],
            "manifest_digest": selected["manifest_digest"],
            "source_repository": selected["package_source"]["repository"],
            "source_revision": selected["package_source"]["revision"],
            "source_path": selected["package_source"]["path"],
            "compatible_clients": sorted(target["client"] for target in policy["targets"]),
            "resolved_targets": ["codex"], "fallback_reason": None,
        }
        identity = {
            **{key: release[key] for key in observer.CHALLENGE_SOURCE_IDENTITY_FIELDS if key != "canonical_source"},
            "canonical_source": f'https://github.com/{release["source_repository"]}@{release["source_revision"]}//{release["source_path"]}',
        }
        context = {
            **challenge, "binary_digest": "sha256:" + "d" * 64, "expected_version": "0.1.18",
            "snapshot_sequence": snapshot["sequence"], "release": release,
            "catalog_repository": "777genius/universal-agent-plugins",
            "directory_product": product, "directory_distribution": distribution,
            "source_identity": identity, "scenario_id": "state_schema_2_migration",
        }
        context["context_digest"] = observer.scenario_challenge_context_digest(context)
        return context

    def test_complete_scenario_digest_rejects_replay_mutation_and_substitution_before_effects(self) -> None:
        valid = self.complete_scenario_context()
        self.assertEqual(observer.validate_scenario_challenge_context(valid, "state_schema_2_migration"), valid)
        cases = {}
        nested = copy.deepcopy(valid); nested["release"]["source_path"] = "plugins/other"; cases["nested_mutation"] = nested
        cases["digest_substitution"] = {**valid, "context_digest": "sha256:" + "f" * 64}
        replay = copy.deepcopy(valid); replay["scenario_id"] = "repair_codex"; replay["context_digest"] = observer.scenario_challenge_context_digest(replay); cases["another_scenario_replay"] = replay
        for label, context in cases.items():
            with self.subTest(label=label), mock.patch.object(observer, "observe") as observation, mock.patch.object(
                observer.subprocess, "run",
            ) as process, mock.patch.object(observer.subprocess, "Popen") as popen, self.assertRaises(ValueError):
                observer.validate_scenario_challenge_context(context, "state_schema_2_migration")
            observation.assert_not_called(); process.assert_not_called(); popen.assert_not_called()

    def test_coherent_source_and_digest_substitution_differs_from_retained_digest(self) -> None:
        retained = self.complete_scenario_context()
        attack = copy.deepcopy(retained)
        source = {"repository": "attacker/packages", "revision": "e" * 40, "path": "plugins/demo"}
        selected = attack["directory_distribution"]["releases"][0]
        selected["package_source"] = source
        attack["release"].update(source_repository=source["repository"], source_revision=source["revision"], source_path=source["path"])
        attack["source_identity"].update(
            source_repository=source["repository"], source_revision=source["revision"], source_path=source["path"],
            canonical_source=f'https://github.com/{source["repository"]}@{source["revision"]}//{source["path"]}',
        )
        attack["context_digest"] = observer.scenario_challenge_context_digest(attack)
        self.assertEqual(observer.validate_scenario_challenge_context(attack, "state_schema_2_migration"), attack)
        self.assertNotEqual(attack["context_digest"], retained["context_digest"])
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "challenge.json"; path.write_text(json.dumps(attack))
            argv = [
                "observe_launch_scenario.py", "--binary", str(Path(temporary) / "binary"),
                "--scenario", "state_schema_2_migration", "--root", temporary,
                "--challenge-context", str(path), "--expected-context-digest", retained["context_digest"],
            ]
            with mock.patch.object(sys, "argv", argv), mock.patch.object(observer, "run") as dispatch, mock.patch.object(
                observer, "observe",
            ) as observation, mock.patch.object(observer.subprocess, "run") as process, self.assertRaisesRegex(ValueError, "retained"):
                observer.main()
            dispatch.assert_not_called(); observation.assert_not_called(); process.assert_not_called()

    def test_exported_root_identity_is_logical_not_temporary_path_derived(self) -> None:
        challenge = {
            "root_id": e2e.logical_root_id("a" * 40, "12", "3"),
            "caller_event_name": "push",
            "caller_ref": "refs/heads/main",
            "caller_workflow_ref": "777genius/universal-agent-plugins/.github/workflows/directory-publication.yml@refs/heads/main",
            "value": "b" * 64,
        }
        self.assertEqual(e2e.exported_root_id(challenge), challenge["root_id"][:16])
        self.assertEqual(e2e.exported_root_id(None), "0" * 16)

    def test_earlier_attempt_native_observations_keep_the_same_thirty_minute_freshness_bound(self) -> None:
        now = datetime.now(timezone.utc).replace(microsecond=0)
        self.assertTrue(e2e.current_or_earlier_attempt("2", "3"))
        self.assertFalse(e2e.current_or_earlier_attempt("4", "3"))
        self.assertTrue(e2e.fresh_observation_interval(
            (now - timedelta(minutes=5)).isoformat(), (now - timedelta(minutes=4)).isoformat(), now=now,
        ))
        self.assertFalse(e2e.fresh_observation_interval(
            (now - timedelta(minutes=31)).isoformat(), (now - timedelta(minutes=31)).isoformat(), now=now,
        ))

    def test_live_artifacts_require_fresh_challenge_bound_ed25519_bundle(self) -> None:
        private_key = Ed25519PrivateKey.generate()
        public_key = private_key.public_key().public_bytes(
            encoding=serialization.Encoding.Raw,
            format=serialization.PublicFormat.Raw,
        )
        challenge = "a" * 64
        artifacts = {
            "runtime-attestations.json": {"schema_version": 1, "attestations": []},
            "notion-oauth-attestations.json": {"schema_version": 1, "attestations": []},
            "chatgpt-cloudflare-attestation.json": {"schema_version": 1, "attestations": []},
            "consent.json": {"schema_version": 1, "purpose": "stable-launch-e2e", "consent": True, "disposable_only": True},
        }
        now = datetime.now(timezone.utc).replace(microsecond=0)
        bundle = {
            "schema_version": 1, "challenge": challenge,
            "signed_at": now.isoformat().replace("+00:00", "Z"),
            "key_id": "stable-observer-2026", "artifacts": artifacts,
        }
        bundle["signature"] = base64.b64encode(private_key.sign(observer_signatures.signed_payload(bundle))).decode()
        encoded_key = base64.b64encode(public_key).decode()
        self.assertEqual(
            observer_signatures.verify_observer_bundle(
                bundle, challenge=challenge, public_key_base64=encoded_key,
                expected_key_id="stable-observer-2026", now=now,
            ), artifacts,
        )
        with self.assertRaisesRegex(ValueError, "signature is invalid"):
            observer_signatures.verify_observer_bundle(
                {**bundle, "artifacts": {**artifacts, "consent.json": {"consent": False}}},
                challenge=challenge, public_key_base64=encoded_key,
                expected_key_id="stable-observer-2026", now=now,
            )
        stale = {**bundle, "signed_at": (now - timedelta(hours=1)).isoformat().replace("+00:00", "Z")}
        stale["signature"] = base64.b64encode(private_key.sign(observer_signatures.signed_payload(stale))).decode()
        with self.assertRaisesRegex(ValueError, "stale"):
            observer_signatures.verify_observer_bundle(
                stale, challenge=challenge, public_key_base64=encoded_key,
                expected_key_id="stable-observer-2026", now=now,
            )

    def test_release_manifest_requires_every_native_slot_and_exact_identity(self) -> None:
        assets = {
            key: {"file": f"agentplugins_1.2.3_{os_name}_{arch}{suffix}", "sha256": f"{index + 1:064x}", "size": 1}
            for index, (key, os_name, arch, suffix) in enumerate((
                ("darwin-amd64", "darwin", "amd64", ""), ("darwin-arm64", "darwin", "arm64", ""),
                ("linux-amd64", "linux", "amd64", ""), ("linux-arm64", "linux", "arm64", ""),
                ("windows-amd64", "windows", "amd64", ".exe"), ("windows-arm64", "windows", "arm64", ".exe"),
            ))
        }
        value = {"schema_version": 2, "tag": "agentplugins-v1.2.3", "commit": "a" * 40, "version": "1.2.3", "assets": assets}
        e2e.validate_release_manifest(value, repository=e2e.TRUSTED_CLI_RELEASE_REPOSITORY, tag="agentplugins-v1.2.3", tag_commit="a" * 40)
        with self.assertRaisesRegex(ValueError, "omits a required"):
            e2e.validate_release_manifest({**value, "assets": dict(list(assets.items())[:-1])}, repository=e2e.TRUSTED_CLI_RELEASE_REPOSITORY, tag="agentplugins-v1.2.3", tag_commit="a" * 40)

    def test_release_resolution_binds_github_tag_manifest_and_asset_bytes(self) -> None:
        selected = b"native-binary"
        slots = (
            ("darwin-amd64", "agentplugins_1.2.3_darwin_amd64", b"1"),
            ("darwin-arm64", "agentplugins_1.2.3_darwin_arm64", b"2"),
            ("linux-amd64", "agentplugins_1.2.3_linux_amd64", selected),
            ("linux-arm64", "agentplugins_1.2.3_linux_arm64", b"4"),
            ("windows-amd64", "agentplugins_1.2.3_windows_amd64.exe", b"5"),
            ("windows-arm64", "agentplugins_1.2.3_windows_arm64.exe", b"6"),
        )
        manifest = {
            "schema_version": 2, "tag": "agentplugins-v1.2.3", "commit": "a" * 40, "version": "1.2.3",
            "assets": {key: {"file": name, "sha256": e2e.hashlib.sha256(body).hexdigest(), "size": len(body)} for key, name, body in slots},
        }
        manifest_body = json.dumps(manifest, sort_keys=True, separators=(",", ":")).encode()
        checksum_bodies = [(name, body) for _, name, body in slots] + [(e2e.RELEASE_MANIFEST_NAME, manifest_body)]
        checksums_body = "".join(
            f"{e2e.hashlib.sha256(body).hexdigest()}  {name}\n" for name, body in checksum_bodies
        ).encode()
        download = f"https://github.com/{e2e.TRUSTED_CLI_RELEASE_REPOSITORY}/releases/download/agentplugins-v1.2.3"
        api_assets = [{"name": e2e.RELEASE_MANIFEST_NAME, "browser_download_url": f"{download}/{e2e.RELEASE_MANIFEST_NAME}", "size": len(manifest_body)}]
        api_assets.append({"name": e2e.RELEASE_CHECKSUMS_NAME, "browser_download_url": f"{download}/{e2e.RELEASE_CHECKSUMS_NAME}", "size": len(checksums_body)})
        api_assets += [{"name": name, "browser_download_url": f"{download}/{name}", "size": len(body)} for _, name, body in slots]
        release = {"id": 123, "draft": False, "prerelease": False, "immutable": True, "tag_name": "agentplugins-v1.2.3", "assets": api_assets}
        bodies = {
            f"{download}/{e2e.RELEASE_MANIFEST_NAME}": manifest_body,
            f"{download}/{e2e.RELEASE_CHECKSUMS_NAME}": checksums_body,
            **{f"{download}/{name}": body for _, name, body in slots},
        }

        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(e2e, "github_json", return_value=release), mock.patch.object(e2e, "resolve_tag_commit", return_value="a" * 40):
            destination, resolved, digest = e2e.resolve_github_release(
                e2e.TRUSTED_CLI_RELEASE_REPOSITORY, "agentplugins-v1.2.3", Path(tmp) / "agentplugins",
                asset_name="agentplugins_1.2.3_linux_amd64",
                fixture_fetch=lambda url, _limit, _accept: bodies[url],
                attestation_verifier=lambda _path, repo, workflow, tag, commit, digest, subject_name: {"repository": repo, "workflow": workflow, "tag": tag, "tag_commit": commit, "asset_name": subject_name, "asset_digest": digest, "verified": True},
            )
            self.assertEqual(destination.read_bytes(), selected)
            self.assertEqual(resolved, manifest)
            self.assertEqual(digest, "sha256:" + e2e.hashlib.sha256(manifest_body).hexdigest())
            self.assertEqual((destination.parent / e2e.RELEASE_CHECKSUMS_NAME).read_bytes(), checksums_body)
            attestation = json.loads((destination.parent / "agentplugins_1.2.3_linux_amd64.attestation.json").read_text())
            self.assertEqual(attestation["asset_name"], "agentplugins_1.2.3_linux_amd64")

            tampered = {**bodies, f"{download}/agentplugins_1.2.3_linux_amd64": b"x" * len(selected)}
            with self.assertRaisesRegex(ValueError, "digest disagrees"):
                e2e.resolve_github_release(
                    e2e.TRUSTED_CLI_RELEASE_REPOSITORY, "agentplugins-v1.2.3", Path(tmp) / "tampered",
                    asset_name="agentplugins_1.2.3_linux_amd64",
                    fixture_fetch=lambda url, _limit, _accept: tampered[url],
                    attestation_verifier=lambda *_args: {},
                )

            tampered_checksums = {**bodies, f"{download}/{e2e.RELEASE_CHECKSUMS_NAME}": checksums_body.replace(b"  release-manifest.json", b"  renamed-manifest.json")}
            with self.assertRaisesRegex(ValueError, "exact manifest asset set"):
                e2e.resolve_github_release(
                    e2e.TRUSTED_CLI_RELEASE_REPOSITORY, "agentplugins-v1.2.3", Path(tmp) / "bad-checksums",
                    asset_name="agentplugins_1.2.3_linux_amd64",
                    fixture_fetch=lambda url, _limit, _accept: tampered_checksums[url],
                    attestation_verifier=lambda *_args: {},
                )

            with mock.patch.object(e2e, "github_json", return_value={**release, "immutable": False}), self.assertRaisesRegex(ValueError, "mutable"):
                e2e.resolve_github_release(
                    e2e.TRUSTED_CLI_RELEASE_REPOSITORY, "agentplugins-v1.2.3", Path(tmp) / "mutable",
                    asset_name="agentplugins_1.2.3_linux_amd64", fixture_fetch=lambda url, _limit, _accept: bodies[url],
                    attestation_verifier=lambda *_args: {},
                )

            untrusted_assets = [dict(item) for item in api_assets]
            untrusted_assets[0]["browser_download_url"] = "https://objects.example.test/release-manifest.json"
            with (
                mock.patch.object(e2e, "github_json", return_value={**release, "assets": untrusted_assets}),
                self.assertRaisesRegex(ValueError, "untrusted browser_download_url"),
            ):
                e2e.resolve_github_release(
                    e2e.TRUSTED_CLI_RELEASE_REPOSITORY, "agentplugins-v1.2.3", Path(tmp) / "untrusted-url",
                    asset_name="agentplugins_1.2.3_linux_amd64",
                    fixture_fetch=lambda url, _limit, _accept: bodies[url],
                    attestation_verifier=lambda *_args: {},
                )

    def test_production_release_resolution_rejects_caller_selected_verifier(self) -> None:
        with self.assertRaisesRegex(ValueError, "statically allowlisted"):
            e2e.resolve_github_release(
                e2e.TRUSTED_CLI_RELEASE_REPOSITORY,
                e2e.TRUSTED_CLI_RELEASE_TAG,
                Path("/unused/agentplugins"),
                asset_name="agentplugins_0.1.24_linux_amd64",
                attestation_verifier=lambda *_args: {},
            )

    def test_github_token_is_scoped_to_exact_api_json_and_redirects_are_rejected(self) -> None:
        response = mock.MagicMock()
        response.__enter__.return_value = response
        response.read.return_value = b'{"id": 1}'
        url = f"https://api.github.com/repos/{e2e.TRUSTED_CLI_RELEASE_REPOSITORY}/releases/tags/agentplugins-v1.2.3"
        response.geturl.return_value = url
        opener = mock.Mock()
        opener.open.return_value = response
        with mock.patch.object(e2e, "build_opener", return_value=opener):
            self.assertEqual(e2e.github_api_get(url, maximum=1024, token="secret"), b'{"id": 1}')
        request = opener.open.call_args.args[0]
        self.assertEqual(request.get_header("Authorization"), "Bearer secret")
        with self.assertRaisesRegex(ValueError, "restricted to exact api.github.com"):
            e2e.github_api_get("https://github.com/release", maximum=1024, token="secret")
        response.geturl.return_value = "https://attacker.example/redirect"
        with (
            mock.patch.object(e2e, "build_opener", return_value=opener),
            self.assertRaisesRegex(ValueError, "must not redirect"),
        ):
            e2e.github_api_get(url, maximum=1024, token="secret")
        with mock.patch.object(e2e, "github_api_get", return_value=b'{"id": 1}') as api_get:
            self.assertEqual(
                e2e.github_json(
                    e2e.TRUSTED_CLI_RELEASE_REPOSITORY,
                    "releases/tags/agentplugins-v1.2.3",
                    token="secret",
                ),
                {"id": 1},
            )
        self.assertEqual(api_get.call_args.kwargs["token"], "secret")
        with self.assertRaisesRegex(ValueError, "trusted CLI repository"):
            e2e.github_json("attacker/example", "releases/tags/agentplugins-v1.2.3", token="secret")
        with self.assertRaisesRegex(ValueError, "exact release or tag JSON path"):
            e2e.github_json(e2e.TRUSTED_CLI_RELEASE_REPOSITORY, "issues", token="secret")

    def test_prepared_release_validation_is_offline_and_rejects_changed_bytes(self) -> None:
        config = e2e.read_production_config()
        version = config["cli_release_tag"].removeprefix("agentplugins-v")
        selected_name = f"agentplugins_{version}_linux_amd64"
        selected_body = b"authenticated native binary"
        asset_names = {
            "darwin-amd64": f"agentplugins_{version}_darwin_amd64",
            "darwin-arm64": f"agentplugins_{version}_darwin_arm64",
            "linux-amd64": selected_name,
            "linux-arm64": f"agentplugins_{version}_linux_arm64",
            "windows-amd64": f"agentplugins_{version}_windows_amd64.exe",
            "windows-arm64": f"agentplugins_{version}_windows_arm64.exe",
        }
        bodies = {
            name: selected_body if name == selected_name else key.encode()
            for key, name in asset_names.items()
        }
        manifest = {
            "schema_version": 2,
            "tag": config["cli_release_tag"],
            "commit": config["cli_release_commit"],
            "version": version,
            "assets": {
                key: {
                    "file": name,
                    "sha256": e2e.hashlib.sha256(bodies[name]).hexdigest(),
                    "size": len(bodies[name]),
                }
                for key, name in asset_names.items()
            },
        }
        manifest_body = json.dumps(manifest, sort_keys=True, separators=(",", ":")).encode()
        checksums_body = "".join(
            f"{e2e.hashlib.sha256(body).hexdigest()}  {name}\n"
            for name, body in [*bodies.items(), (e2e.RELEASE_MANIFEST_NAME, manifest_body)]
        ).encode()
        binary_digest = "sha256:" + e2e.hashlib.sha256(selected_body).hexdigest()
        identity = {
            "repository": config["cli_release_repository"],
            "tag": config["cli_release_tag"],
            "tag_commit": config["cli_release_commit"],
            "release_id": 123,
            "immutable": True,
        }
        attestation = e2e.current_github_asset_attestation(
            asset_name=selected_name, asset_digest=binary_digest, subject_name=selected_name,
        )
        prepared = {
            "cli_release_tag": config["cli_release_tag"],
            "release_manifest": manifest,
            "release_manifest_digest": "sha256:" + e2e.hashlib.sha256(manifest_body).hexdigest(),
            "release_checksums_digest": "sha256:" + e2e.hashlib.sha256(checksums_body).hexdigest(),
            "github_release_identity": identity,
            "authenticated_asset": {"name": selected_name, "digest": binary_digest},
            "github_asset_attestation": attestation,
        }
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            release_dir = root / "release"
            release_dir.mkdir()
            (release_dir / selected_name).write_bytes(selected_body)
            (release_dir / e2e.RELEASE_MANIFEST_NAME).write_bytes(manifest_body)
            (release_dir / e2e.RELEASE_CHECKSUMS_NAME).write_bytes(checksums_body)
            (release_dir / "github-release-identity.json").write_text(json.dumps(identity))
            (release_dir / f"{selected_name}.attestation.json").write_text(json.dumps(attestation))
            with mock.patch.object(
                e2e, "resolve_github_release", side_effect=AssertionError("network re-resolution is forbidden")
            ):
                path, resolved, resolved_identity, _, _ = e2e.validate_prepared_github_release(
                    root, prepared, selected_name
                )
            self.assertEqual(path.read_bytes(), selected_body)
            self.assertEqual(resolved, manifest)
            self.assertEqual(resolved_identity, identity)
            for field in attestation:
                with self.subTest(missing=field):
                    forged = json.loads(json.dumps(attestation))
                    del forged[field]
                    (release_dir / f"{selected_name}.attestation.json").write_text(json.dumps(forged))
                    with self.assertRaisesRegex(ValueError, "attestation differs"):
                        e2e.validate_prepared_github_release(root, {**prepared, "github_asset_attestation": forged}, selected_name)
            forged = {**attestation, "unexpected": "unreviewed provenance"}
            (release_dir / f"{selected_name}.attestation.json").write_text(json.dumps(forged))
            with self.assertRaisesRegex(ValueError, "attestation differs"):
                e2e.validate_prepared_github_release(root, {**prepared, "github_asset_attestation": forged}, selected_name)
            forged = {**attestation, "source_digest": "0" * 40}
            (release_dir / f"{selected_name}.attestation.json").write_text(json.dumps(forged))
            with self.assertRaisesRegex(ValueError, "attestation differs"):
                e2e.validate_prepared_github_release(root, {**prepared, "github_asset_attestation": forged}, selected_name)
            (release_dir / f"{selected_name}.attestation.json").write_text(json.dumps(attestation))
            path.write_bytes(b"x" * len(selected_body))
            with self.assertRaisesRegex(ValueError, "prepared native asset bytes differ"):
                e2e.validate_prepared_github_release(root, prepared, selected_name)

    def test_github_attestation_rejects_missing_or_wrong_subject(self) -> None:
        verified = mock.Mock(returncode=0, stdout="[]", stderr="")
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(e2e.subprocess, "run", return_value=verified):
            subject = "agentplugins_0.1.24_linux_amd64"
            digest = "sha256:" + e2e.TRUSTED_CLI_RELEASE_ASSETS[subject]["sha256"]
            asset = Path(tmp) / subject
            asset.write_bytes(b"native")
            with self.assertRaisesRegex(ValueError, "no verified"):
                e2e.verify_github_asset_attestation(asset, e2e.TRUSTED_CLI_RELEASE_REPOSITORY, e2e.TRUSTED_CLI_RELEASE_WORKFLOW, e2e.TRUSTED_CLI_RELEASE_TAG, e2e.TRUSTED_CLI_RELEASE_COMMIT, digest)
            wrong = [{"verificationResult": {"statement": {"predicateType": "https://slsa.dev/provenance/v1", "subject": [{"name": "wrong", "digest": {"sha256": "0" * 64}}]}}}]
            verified.stdout = json.dumps(wrong)
            with self.assertRaisesRegex(ValueError, "subject name/digest"):
                e2e.verify_github_asset_attestation(asset, e2e.TRUSTED_CLI_RELEASE_REPOSITORY, e2e.TRUSTED_CLI_RELEASE_WORKFLOW, e2e.TRUSTED_CLI_RELEASE_TAG, e2e.TRUSTED_CLI_RELEASE_COMMIT, digest)

    def test_github_attestation_pins_release_identity_predicate_subject_and_hosted_runner(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            asset = Path(tmp) / "agentplugins"
            asset.write_bytes(b"native")
            releases = (
                (
                    e2e.verify_github_asset_attestation,
                    e2e.TRUSTED_CLI_RELEASE_TAG,
                    e2e.TRUSTED_CLI_RELEASE_COMMIT,
                    e2e.TRUSTED_CLI_RELEASE_ASSETS,
                    "agentplugins_0.1.24_linux_amd64",
                ),
                (
                    e2e.verify_historical_github_asset_attestation,
                    e2e.HISTORICAL_CLI_RELEASE_TAG,
                    e2e.HISTORICAL_CLI_RELEASE_COMMIT,
                    e2e.HISTORICAL_CLI_RELEASE_ASSETS,
                    "agentplugins_0.1.18_linux_amd64",
                ),
            )
            for verifier, tag, commit, assets, subject_name in releases:
                digest = "sha256:" + assets[subject_name]["sha256"]
                statement = [{"verificationResult": {"statement": {
                    "predicateType": "https://slsa.dev/provenance/v1",
                    "subject": [{"name": subject_name, "digest": {"sha256": digest.removeprefix("sha256:")}}],
                }}}]
                completed = subprocess.CompletedProcess([], 0, json.dumps(statement), "")
                with self.subTest(tag=tag), mock.patch.object(e2e.subprocess, "run", return_value=completed) as invoked:
                    verified = verifier(
                        asset, e2e.TRUSTED_CLI_RELEASE_REPOSITORY, e2e.TRUSTED_CLI_RELEASE_WORKFLOW,
                        tag, commit, digest, subject_name,
                    )
                    argv = invoked.call_args.args[0]
                    for flag in (
                        "--repo", "--signer-workflow", "--source-ref", "--source-digest",
                        "--predicate-type", "--deny-self-hosted-runners",
                    ):
                        self.assertIn(flag, argv)
                    self.assertEqual(verified["tag"], tag)
                    self.assertEqual(verified["tag_commit"], commit)
                    self.assertEqual(verified["subject_name"], subject_name)
                    self.assertEqual(verified["subject_digest"], digest)
                    self.assertEqual(verified["runner_environment"], "github-hosted")
                    self.assertEqual(argv[argv.index("--source-ref") + 1], e2e.TRUSTED_CLI_RELEASE_SOURCE_REF)
                    for wrong_tag, wrong_commit, wrong_digest in (
                        (releases[1][1] if tag == releases[0][1] else releases[0][1], commit, digest),
                        (tag, "0" * 40, digest),
                        (tag, commit, "sha256:" + "0" * 64),
                    ):
                        with self.assertRaisesRegex(ValueError, "trusted release identity"):
                            verifier(
                                asset, e2e.TRUSTED_CLI_RELEASE_REPOSITORY, e2e.TRUSTED_CLI_RELEASE_WORKFLOW,
                                wrong_tag, wrong_commit, wrong_digest, subject_name,
                            )
                wrong_predicate = json.loads(json.dumps(statement))
                wrong_predicate[0]["verificationResult"]["statement"]["predicateType"] = "https://example.invalid/claim"
                with mock.patch.object(e2e.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, json.dumps(wrong_predicate), "")):
                    with self.assertRaisesRegex(ValueError, "subject name/digest"):
                        verifier(
                            asset, e2e.TRUSTED_CLI_RELEASE_REPOSITORY, e2e.TRUSTED_CLI_RELEASE_WORKFLOW,
                            tag, commit, digest, subject_name,
                        )
            with self.assertRaisesRegex(ValueError, "trusted release identity"):
                e2e.verify_github_asset_attestation(
                    asset, e2e.TRUSTED_CLI_RELEASE_REPOSITORY, "caller/workflow.yml",
                    e2e.TRUSTED_CLI_RELEASE_TAG, e2e.TRUSTED_CLI_RELEASE_COMMIT,
                    "sha256:" + e2e.TRUSTED_CLI_RELEASE_ASSETS["agentplugins_0.1.24_linux_amd64"]["sha256"],
                    "agentplugins_0.1.24_linux_amd64",
                )

    def test_native_observation_schema_accepts_exact_producer_attestation(self) -> None:
        schema = json.loads((ROOT / "tests/e2e/schemas/native-release-observation-v2.schema.json").read_text())
        asset = "agentplugins_0.1.24_linux_amd64"
        digest = e2e.RELEASED_LINUX_AMD64_DIGEST
        observation = {
            "schema_version": 2, "kind": "binary", "os": "linux", "architecture": "amd64",
            "node_major": None, "executed": True, "version": "0.1.24",
            "catalog_repository": e2e.TRUSTED_CATALOG_REPOSITORY, "catalog_sha": "b" * 40,
            "cli_release_repository": e2e.TRUSTED_CLI_RELEASE_REPOSITORY,
            "cli_release_tag": e2e.TRUSTED_CLI_RELEASE_TAG,
            "github_release_identity": {
                "repository": e2e.TRUSTED_CLI_RELEASE_REPOSITORY,
                "tag": e2e.TRUSTED_CLI_RELEASE_TAG, "tag_commit": e2e.TRUSTED_CLI_RELEASE_COMMIT,
                "release_id": 379284682, "immutable": True,
            },
            "release_manifest_digest": e2e.RELEASE_MANIFEST_DIGEST,
            "release_checksums_digest": e2e.RELEASE_CHECKSUMS_DIGEST,
            "directory_digest": "sha256:" + "e" * 64,
            "asset_name": asset, "asset_digest": digest,
            "github_asset_attestation": {
                "repository": e2e.TRUSTED_CLI_RELEASE_REPOSITORY,
                "workflow": e2e.TRUSTED_CLI_RELEASE_WORKFLOW,
                "tag": e2e.TRUSTED_CLI_RELEASE_TAG, "tag_commit": e2e.TRUSTED_CLI_RELEASE_COMMIT,
                "issuer": "https://token.actions.githubusercontent.com",
                "source_ref": e2e.TRUSTED_CLI_RELEASE_SOURCE_REF,
                "source_digest": e2e.TRUSTED_CLI_RELEASE_COMMIT,
                "predicate_type": "https://slsa.dev/provenance/v1",
                "subject_name": asset, "subject_digest": digest,
                "runner_environment": "github-hosted",
                "asset_name": asset, "asset_digest": digest, "verified": True,
            },
            "challenge": "f" * 64, "challenge_context": {},
            "started_at": "2026-08-24T00:00:00Z", "observed_at": "2026-08-24T00:00:01Z",
            "command_trace": {}, "runner_platform": "Linux X64",
        }
        jsonschema.Draft202012Validator(schema, format_checker=jsonschema.FormatChecker()).validate(observation)
        for missing in (
            "issuer", "source_ref", "source_digest", "predicate_type", "subject_name",
            "subject_digest", "runner_environment",
        ):
            forged = json.loads(json.dumps(observation))
            del forged["github_asset_attestation"][missing]
            with self.subTest(missing=missing), self.assertRaises(jsonschema.ValidationError):
                jsonschema.Draft202012Validator(schema).validate(forged)

    def test_current_native_matrix_rejects_v1_sidecars_before_use(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "historical.json").write_text(json.dumps({
                "schema_version": 1, "kind": "binary", "os": "linux",
                "architecture": "amd64",
            }))
            harness = object.__new__(e2e.LaunchHarness)
            harness.native_observations = root
            with self.assertRaisesRegex(ValueError, "schema_version 2"):
                harness.native_platform_matrix()

    def test_npm_installed_executable_must_equal_authenticated_native_asset(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            executable = Path(tmp) / "0.1.18" / "linux-amd64" / "agentplugins"
            executable.parent.mkdir(parents=True)
            executable.write_bytes(b"prints-correct-version-but-is-not-release-binary")
            executable.chmod(0o700)
            native = {"sha256": e2e.hashlib.sha256(b"real-release-binary").hexdigest(), "size": len(b"real-release-binary")}
            with self.assertRaisesRegex(RuntimeError, "does not match"):
                facade.verify_installed_npm_payload(Path(tmp), native)

            body = b"real-release-binary"
            executable.write_bytes(body)
            resolved, digest = facade.verify_installed_npm_payload(Path(tmp), native)
            self.assertEqual(resolved, executable.resolve())
            self.assertEqual(digest, "sha256:" + native["sha256"])

    def test_npm_resolution_binds_exact_registry_integrity_and_tarball(self) -> None:
        body = b"exact npm tarball"
        integrity = "sha512-" + base64.b64encode(e2e.hashlib.sha512(body).digest()).decode()
        metadata_url = "https://registry.npmjs.org/universal-agent-plugins/0.1.8"
        tarball_url = "https://registry.npmjs.org/universal-agent-plugins/-/universal-agent-plugins-0.1.8.tgz"
        provenance_url = "https://registry.npmjs.org/-/npm/v1/attestations/universal-agent-plugins@0.1.8"
        metadata = json.dumps({"name": "universal-agent-plugins", "version": "0.1.8", "dist": {"integrity": integrity, "tarball": tarball_url, "attestations": {"url": provenance_url, "provenance": {"predicateType": "https://slsa.dev/provenance/v1"}}}}).encode()
        bodies = {metadata_url: metadata, tarball_url: body}
        with tempfile.TemporaryDirectory() as tmp:
            path, identity = e2e.resolve_npm_package(
                "universal-agent-plugins", "0.1.8", Path(tmp) / "package.tgz",
                fixture_fetch=lambda url, _limit, _accept: bodies[url],
            )
            self.assertEqual(path.read_bytes(), body)
            self.assertEqual(identity["integrity"], integrity)
            self.assertEqual(identity["provenance_url"], provenance_url)
            with self.assertRaisesRegex(ValueError, "dist.integrity"):
                e2e.resolve_npm_package(
                    "universal-agent-plugins", "0.1.8", Path(tmp) / "tampered.tgz",
                    fixture_fetch=lambda url, _limit, _accept: b"tampered" if url == tarball_url else metadata,
                )
            without_provenance = json.dumps({"name": "universal-agent-plugins", "version": "0.1.8", "dist": {"integrity": integrity, "tarball": tarball_url}}).encode()
            with self.assertRaisesRegex(ValueError, "provenance"):
                e2e.resolve_npm_package(
                    "universal-agent-plugins", "0.1.8", Path(tmp) / "no-provenance.tgz",
                    fixture_fetch=lambda url, _limit, _accept: body if url == tarball_url else without_provenance,
                )

    def test_production_identity_is_fixed_cross_repository_configuration(self) -> None:
        attributes = (ROOT / ".gitattributes").read_text().splitlines()
        self.assertIn("/registry/directory.json text eol=lf", attributes)
        self.assertIn("/tests/e2e/launch-scenarios.json text eol=lf", attributes)
        config = e2e.read_production_config()
        self.assertEqual(config["catalog_repository"], "777genius/universal-agent-plugins")
        self.assertEqual(config["cli_release_repository"], "777genius/plugin-kit-ai")
        self.assertEqual(config["cli_release_tag"], "agentplugins-v0.1.24")
        self.assertEqual(config["cli_release_commit"], e2e.TRUSTED_CLI_RELEASE_COMMIT)
        self.assertEqual(config["cli_release_workflow"], "777genius/plugin-kit-ai/.github/workflows/agentplugins-release.yml")
        self.assertEqual(config["cli_release_manifest_digest"], e2e.RELEASE_MANIFEST_DIGEST)
        self.assertEqual(config["cli_release_checksums_digest"], e2e.RELEASE_CHECKSUMS_DIGEST)
        self.assertEqual(config["npm_facade_version"], "0.1.24")
        self.assertRegex(config["npm_facade_integrity"], r"^sha512-[A-Za-z0-9+/]+={0,2}$")
        self.assertEqual(config["npm_facade_integrity"], "sha512-hUMKvd2kAjTWA1obzAlXdbE3GxjRk8lhXRA9YuO2h2NINnYv/GQi2JwgkqWhOd95BpEKh5Do8vV1B4B/Unl+jw==")
        expected_directory_digest = "sha256:1046ec5f0baa8bbf604a264c62f80a757ee051bcce171753f7dd2d6d40fcd6dd"
        self.assertEqual(config["directory_source_digest"], expected_directory_digest)
        # A pull request may carry an untrusted Directory review candidate, but
        # must not rewrite the production launch identity to match that
        # candidate. Main and every non-PR execution still fail closed if the
        # production pin and checked-out Directory diverge.
        if os.environ.get("GITHUB_EVENT_NAME") != "pull_request":
            self.assertEqual(config["directory_source_digest"], e2e.sha256_file(ROOT / "registry/directory.json"))
        self.assertEqual(config["scenario_contract_digest"], e2e.sha256_file(e2e.SCENARIOS))
        schema = json.loads((ROOT / "tests/e2e/schemas/native-release-observation-v2.schema.json").read_text())
        self.assertEqual(
            schema["properties"]["github_asset_attestation"]["properties"]["workflow"]["const"],
            e2e.TRUSTED_CLI_RELEASE_WORKFLOW,
        )
        self.assertEqual(schema["properties"]["cli_release_tag"]["const"], e2e.TRUSTED_CLI_RELEASE_TAG)
        frozen_v1 = json.loads((ROOT / "tests/e2e/schemas/native-release-observation.schema.json").read_text())
        self.assertEqual(frozen_v1["properties"]["cli_release_tag"]["const"], "agentplugins-v0.1.18")
        self.assertNotIn("repository", config)
        observer = json.loads((ROOT / "deploy/uap-observer.json").read_text())
        self.assertEqual(observer["cli_release_tag"], e2e.TRUSTED_CLI_RELEASE_TAG)
        self.assertEqual(observer["policies"], [{
            "repository": e2e.TRUSTED_CATALOG_REPOSITORY,
            "repository_id": e2e.TRUSTED_CATALOG_REPOSITORY_ID,
            "repository_owner_id": e2e.TRUSTED_CATALOG_REPOSITORY_OWNER_ID,
            "ref": e2e.TRUSTED_OBSERVER_REF,
            "ref_type": "branch",
            "environment": e2e.TRUSTED_OBSERVER_ENVIRONMENT,
            "workflow_ref": e2e.TRUSTED_OBSERVER_WORKFLOW_REF,
            "job_workflow_ref": e2e.TRUSTED_OBSERVER_JOB_WORKFLOW_REF,
            "workflow": "Signed Directory publication",
            "event_names": ["push", "schedule", "workflow_dispatch"],
            "job_name_suffix": "protected-observer-inputs",
        }])

    def test_hero_contract_is_exactly_five_by_three(self) -> None:
        scenarios = json.loads(e2e.SCENARIOS.read_text())
        self.assertEqual(
            scenarios["heroes"],
            ["agent-code-navigator", "chrome-devtools", "context7", "cloudflare-docs", "notion"],
        )
        self.assertEqual(scenarios["runtime_clients"], ["codex", "cursor", "kiro"])
        expected_rows = len(scenarios["heroes"]) * len(scenarios["runtime_clients"])
        self.assertEqual(expected_rows, 15)
        self.assertEqual(scenarios["expected_counts"]["hero_lifecycle_rows"], expected_rows)
        self.assertEqual(scenarios["expected_counts"]["hero_runtime_rows"], expected_rows)
        self.assertEqual(e2e.EXPECTED_COUNTS["hero_lifecycle_rows"], expected_rows)
        self.assertEqual(e2e.EXPECTED_COUNTS["hero_runtime_rows"], expected_rows)

    def test_production_identity_rejects_configured_repository_or_tag_changes(self) -> None:
        original = json.loads(e2e.PRODUCTION_CONFIG.read_text())
        for field, changed in (
            ("catalog_repository", "attacker/catalog"),
            ("cli_release_repository", "attacker/binaries"),
            ("cli_release_tag", "agentplugins-v0.1.9"),
            ("cli_release_commit", "f" * 40),
            ("cli_release_workflow", "attacker/repo/.github/workflows/agentplugins-release.yml"),
            ("cli_release_manifest_digest", "sha256:" + "0" * 64),
            ("cli_release_checksums_digest", "sha256:" + "0" * 64),
            ("npm_facade_integrity", "sha512-invalid"),
            ("directory_source_digest", "sha256:" + "0" * 64),
            ("directory_source_digest", "sha256:7d2e82322377e8f83a94912113c28287aebaf7ddf68f1908e709adca865aa21b"),
            ("scenario_contract_digest", "sha256:" + "0" * 64),
        ):
            with self.subTest(field=field, changed=changed), tempfile.TemporaryDirectory() as tmp:
                path = Path(tmp) / "production-launch.json"
                path.write_text(json.dumps({**original, field: changed}))
                with mock.patch.object(e2e, "PRODUCTION_CONFIG", path), self.assertRaisesRegex(ValueError, "configuration is invalid"):
                    e2e.read_production_config()

    def test_signed_directory_fixture_binds_origin_digest_sequence_and_trust(self) -> None:
        env, snapshot, digest = e2e.validated_directory_environment(
            "https://directory.example.test/registry/",
            PUBLICATION / "snapshot.json",
            PUBLICATION / "envelope-current.json",
            PUBLICATION / "trusted-keys.json",
        )
        self.assertEqual(snapshot["sequence"], 15)
        self.assertEqual(digest, json.loads((PUBLICATION / "envelope-current.json").read_text())["snapshot_digest"])
        self.assertNotIn("CATALOG", " ".join(env))
        self.assertIn("AGENTPLUGINS_DIRECTORY_ORIGIN", env)
        self.assertEqual(set(env), e2e.DIRECTORY_INPUT_ENVIRONMENT_KEYS)

        with tempfile.TemporaryDirectory() as tmp:
            invalid_envelope = json.loads((PUBLICATION / "envelope-current.json").read_text())
            invalid_envelope["signature"] = "A" * 86 + "=="
            invalid_path = Path(tmp) / "envelope.json"
            invalid_path.write_bytes(e2e.canonical_json(invalid_envelope))
            with self.assertRaisesRegex(ValueError, "invalid Ed25519"):
                e2e.validated_directory_environment(
                    "https://directory.example.test/registry/",
                    PUBLICATION / "snapshot.json",
                    invalid_path,
                    PUBLICATION / "trusted-keys.json",
                )

    def test_real_binary_directory_environment_forwards_only_candidate_origin(self) -> None:
        directory_environment = {
            "AGENTPLUGINS_DIRECTORY_ORIGIN": "https://directory.example.test/registry/",
            "AGENTPLUGINS_DIRECTORY_SNAPSHOT": str(PUBLICATION / "snapshot.json"),
            "AGENTPLUGINS_DIRECTORY_ENVELOPE": str(PUBLICATION / "envelope-current.json"),
            "AGENTPLUGINS_DIRECTORY_TRUST": str(PUBLICATION / "trusted-keys.json"),
        }
        with tempfile.TemporaryDirectory() as tmp:
            sandbox = Path(tmp) / "scenario"
            sandbox.mkdir()
            env = e2e.isolated_environment(sandbox, ("cursor",), directory_environment)
        directory_keys = {key for key in env if key.startswith("AGENTPLUGINS_DIRECTORY_")}
        self.assertEqual(directory_keys, e2e.DIRECTORY_LAUNCH_ENVIRONMENT_KEYS)
        self.assertEqual(env["AGENTPLUGINS_DIRECTORY_ORIGIN"], directory_environment["AGENTPLUGINS_DIRECTORY_ORIGIN"])
        self.assertNotIn("AGENTPLUGINS_DIRECTORY_SNAPSHOT", env)
        self.assertNotIn("AGENTPLUGINS_DIRECTORY_TRUST", env)
        self.assertNotIn("AGENTPLUGINS_DIRECTORY_CACHE", env)

    def test_partial_real_binary_directory_environment_is_rejected(self) -> None:
        partial = {
            "AGENTPLUGINS_DIRECTORY_ORIGIN": "https://directory.example.test/registry/",
            "AGENTPLUGINS_DIRECTORY_SNAPSHOT": str(PUBLICATION / "snapshot.json"),
        }
        with tempfile.TemporaryDirectory() as tmp:
            sandbox = Path(tmp) / "scenario"
            sandbox.mkdir()
            with self.assertRaisesRegex(ValueError, "complete origin/snapshot/envelope/trust tuple"):
                e2e.isolated_environment(sandbox, ("cursor",), partial)

    def test_disposable_root_must_be_fresh(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaisesRegex(ValueError, "must not already exist"):
                self.fixture_harness(Path(tmp))

    def test_isolated_environment_has_separate_roots_and_drops_auth(self) -> None:
        inherited = {
            "PATH": "/bin", "HOME": "/real/home", "GH_TOKEN": "secret",
            "GITHUB_TOKEN": "secret", "AWS_SECRET_ACCESS_KEY": "secret",
        }
        with tempfile.TemporaryDirectory() as tmp, mock.patch.dict(os.environ, inherited, clear=True):
            sandbox = Path(tmp) / "scenario"
            sandbox.mkdir()
            env = e2e.isolated_environment(sandbox, ("codex", "cursor", "kiro"))
            roots = {env[name] for name in ("HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "AGENTPLUGINS_HOME", "AGENTPLUGINS_EVIDENCE_ROOT")}
            self.assertEqual(len(roots), 5)
            self.assertNotIn("GITHUB_TOKEN", env)
            self.assertNotIn("GH_TOKEN", env)
            self.assertNotIn("AWS_SECRET_ACCESS_KEY", env)
            self.assertTrue(all(Path(path).is_relative_to(sandbox) for path in roots))

    def test_driver_result_outside_disposable_root_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            sandbox = Path(tmp) / "scenario"
            sandbox.mkdir()
            with self.assertRaisesRegex(ValueError, "outside the disposable root"):
                e2e.LaunchHarness._assert_result_paths({"evidence_path": "/real/project/result.json"}, sandbox)

    def test_info_reconciliation_requires_exact_boolean_proofs(self) -> None:
        authoritative = {
            "receipt_reconciled": True, "native_discovery_reconciled": True,
            "client_version": "1.0.80",
            "native_discovery_evidence": {
                "basis": "protected_external_observer", "observer": "native-client-command-v1", "client": "vscode",
                "version_operation": {"operation": "version", "argv": ["copilot", "--version"], "observed_client_version": "1.0.80"},
                "discovery_operation": {"operation": "list", "argv": ["copilot", "plugin", "list"], "discovered": True, "product_id": "context7"},
            },
        }
        self.assertTrue(e2e.LaunchHarness.info_reconciled(authoritative, "vscode"))
        self.assertFalse(e2e.LaunchHarness.info_reconciled({"receipt_reconciled": True, "native_discovery_reconciled": True}))
        self.assertFalse(e2e.LaunchHarness.info_reconciled({"receipts": ["owned"], "discovery": {"state": "found"}}))
        self.assertFalse(e2e.LaunchHarness.info_reconciled({"receipt_reconciled": True, "native_discovery_reconciled": False}))
        missing_observer = json.loads(json.dumps(authoritative))
        del missing_observer["native_discovery_evidence"]["observer"]
        self.assertFalse(e2e.LaunchHarness.info_reconciled(missing_observer, "vscode"))
        wrong_client = json.loads(json.dumps(authoritative))
        wrong_client["native_discovery_evidence"]["client"] = "cursor"
        self.assertFalse(e2e.LaunchHarness.info_reconciled(wrong_client, "vscode"))

    def test_protected_hero_runtime_evidence_requires_exact_native_operations(self) -> None:
        challenge = "a" * 64
        discovery = {
            "codex": ["codex", "mcp", "list", "--json"],
            "cursor": ["cursor", "agent", "mcp", "list", "--json"],
            "kiro": ["kiro", "mcp", "list", "--json"],
        }
        for client, argv in discovery.items():
            with self.subTest(client=client):
                marker = "sha256:" + hashlib.sha256(
                    f"UAP_OBSERVER_OK {client} context7 {challenge}".encode()
                ).hexdigest()
                value = {
                    "basis": "protected_external_observer",
                    "observer": "native-client-command-v1",
                    "client": client,
                    "version_operation": {
                        "operation": "version", "argv": [client, "--version"],
                        "observed_client_version": f"{client}-1",
                    },
                    "discovery_operation": {
                        "operation": "discovery", "argv": argv,
                        "discovered": True, "product_id": "context7",
                    },
                    "invocation_operation": {
                        "operation": "tool_call", "tool": "resolve-library-id",
                        "product_id": "context7", "marker_digest": marker, "succeeded": True,
                    },
                }

                def valid(candidate):
                    return e2e.authoritative_protected_runtime_evidence(
                        candidate, client_version=f"{client}-1", product_id="context7",
                        client=client, challenge=challenge,
                    )

                self.assertTrue(valid(value))
                for mutation in (
                    lambda item: item["version_operation"].update(argv=[client, "version"]),
                    lambda item: item["discovery_operation"].update(argv=[client, "plugins"]),
                    lambda item: item["discovery_operation"].update(product_id="notion"),
                    lambda item: item["invocation_operation"].update(tool="wrong-tool"),
                    lambda item: item["invocation_operation"].update(marker_digest="sha256:" + "0" * 64),
                    lambda item: item["invocation_operation"].update(succeeded=False),
                ):
                    rejected = json.loads(json.dumps(value))
                    mutation(rejected)
                    self.assertFalse(valid(rejected))

    def test_driven_copilot_scenario_uses_the_exact_supplied_cli_on_path(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            binary = root / "agentplugins"
            binary.write_text("#!/bin/sh\nexit 0\n")
            binary.chmod(0o700)
            copilot = root / "exact-copilot" / "copilot"
            copilot.parent.mkdir()
            copilot.write_text("#!/bin/sh\nexit 0\n")
            copilot.chmod(0o700)
            harness = self.fixture_harness()
            harness.binary = binary
            harness.copilot_executable = copilot
            harness.challenge = {"value": "a" * 64}
            harness.run_root = root / "runs"
            harness.run_root.mkdir()
            release = {
                "product_id": "context7", "distribution_id": "upstash/context7",
                "distribution_kind": "upstream", "release_sequence": 1,
                "source_revision": "b" * 40, "source_repository": "upstash/context7",
                "source_path": "agent-plugin", "tree_digest": "sha256:" + "c" * 64,
                "manifest_digest": "sha256:" + "d" * 64,
            }
            harness.snapshot = {
                "sequence": 1,
                "products": [{"id": "context7"}],
                "distributions": [{"id": "upstash/context7"}],
            }
            now = datetime.now(timezone.utc).replace(microsecond=0).isoformat()
            payload = {
                "outcome": "passed", "scenario_id": "shared_copilot_vscode_backend",
                "challenge": harness.challenge["value"], "reason": "observed",
                "command_traces": [{"challenge": harness.challenge["value"], "argv": ["info"], "started_at": now, "ended_at": now}],
                "before": {}, "after": {}, "started_at": now, "observed_at": now,
                "manager_observer": {}, "native_observer": {},
            }
            completed = subprocess.CompletedProcess([], 0, json.dumps(payload), "")
            def observed(*args, **kwargs):
                context_path = Path(args[0][args[0].index("--challenge-context") + 1])
                payload["challenge_context_digest"] = json.loads(context_path.read_text())["context_digest"]
                return subprocess.CompletedProcess([], 0, json.dumps(payload), "")
            with mock.patch.object(harness, "directory_release", return_value=release), mock.patch.object(
                e2e.subprocess, "run", side_effect=observed,
            ) as run:
                outcome, _, _ = harness.driven_scenario("shared_copilot_vscode_backend")
            self.assertEqual(outcome, "passed")
            self.assertEqual(run.call_args.kwargs["env"]["PATH"].split(os.pathsep)[0], str(copilot.parent))

            payload["scenario_id"] = "missing_runtime_zero_mutation"
            with mock.patch.object(harness, "directory_release", return_value=release), mock.patch.object(
                e2e.subprocess, "run", side_effect=observed,
            ) as run:
                harness.driven_scenario("missing_runtime_zero_mutation")
            self.assertNotEqual(run.call_args.kwargs["env"]["PATH"].split(os.pathsep)[0], str(copilot.parent))

    def test_repository_owned_proof_cannot_be_promoted_to_discovery(self) -> None:
        harness = self.fixture_harness()
        details = {
            "evidence_basis": "repository_owned_disposable_observer",
            "runtime_proof": False, "native_discovery_proof": False,
        }
        tuple_value = harness.tuple(client_version="native-state-v1")
        harness.add("local-materialization", "context7", "cursor", "materialization", "passed", "proved", tuple_value=tuple_value, details=details)
        harness.add("fake-discovery", "context7", "cursor", "discovery", "passed", "claimed", tuple_value=tuple_value, details=details)
        self.assertEqual(harness.rows[0]["outcome"], "passed")
        self.assertIsNone(harness.rows[0]["tuple"]["client_version"])
        self.assertEqual(harness.rows[1]["outcome"], "inconclusive")
        self.assertIsNone(harness.rows[1]["tuple"]["client_version"])

    def test_all_package_native_proof_is_exact_copilot_lifecycle(self) -> None:
        valid = {
            "basis": "native_client_command",
            "version_operation": {
                "argv": ["copilot", "--version"], "observed_client_version": "1.0.80",
            },
            "discovery_operation": {
                "argv": ["copilot", "plugin", "list"], "discovered": True,
                "product_id": "context7@agentplugins-d1b2efbd0485",
            },
        }
        validate = lambda evidence: e2e.authoritative_repository_copilot_evidence(
            evidence, client_version="1.0.80", product_id="context7",
            physical_artifact_id="context7-05408cd487ce", expected_version="1.0.80",
        )
        self.assertTrue(validate(valid))
        for path, value in (
            (("basis",), "protected_external_observer"),
            (("version_operation", "argv"), ["sh", "-c", "copilot --version"]),
            (("version_operation", "observed_client_version"), "1.0.79"),
            (("discovery_operation", "argv"), ["copilot", "plugin", "list", "--fixture"]),
            (("discovery_operation", "product_id"), "context7"),
            (("discovery_operation", "product_id"), "context7@agentplugins-xyz"),
            (("discovery_operation", "product_id"), "other@agentplugins-d1b2efbd0485"),
            (("discovery_operation", "product_id"), "context7@agentplugins-05408cd487ce"),
            (("discovery_operation", "discovered"), False),
        ):
            mutated = json.loads(json.dumps(valid))
            target = mutated
            for key in path[:-1]:
                target = target[key]
            target[path[-1]] = value
            self.assertFalse(validate(mutated), path)
        self.assertFalse(e2e.authoritative_repository_copilot_evidence(
            valid, client_version="1.0.79", product_id="context7",
            physical_artifact_id="context7-05408cd487ce", expected_version="1.0.80",
        ))
        harness = self.fixture_harness()
        with mock.patch.object(harness, "directory_release", return_value={}):
            client, _ = harness.all_package_client("context7")
        self.assertEqual(client, "copilot")
        harness.config = {**harness.config, "all_package_client": "cursor"}
        with self.assertRaisesRegex(ValueError, "GitHub Copilot CLI"):
            harness.all_package_client("context7")

    def test_copilot_installation_metadata_is_exact_and_confined(self) -> None:
        config = e2e.read_production_config()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "run"
            root.mkdir()
            executable = root / "copilot"
            executable.write_bytes(b"exact copilot executable")
            executable.chmod(0o755)
            metadata_path = root / "copilot-metadata.json"
            metadata_path.write_text("{}")
            metadata = {
                "schema_version": 1,
                "package": config["copilot_cli_package"],
                "version": config["copilot_cli_version"],
                "integrity": config["copilot_cli_integrity"],
                "node_major": 22,
                "signature_audit": True,
                "version_argv": ["copilot", "--version"],
                "observed_version": "1.0.80",
                "executable_digest": e2e.sha256_file(executable),
                "version_stdout_digest": "sha256:" + "a" * 64,
            }
            validate = lambda value, path=executable: e2e.valid_copilot_installation(
                path, metadata_path, value, root, config,
            )
            self.assertTrue(validate(metadata))
            for key, replacement in (
                ("version", "1.0.79"),
                ("integrity", "sha512-untrusted"),
                ("signature_audit", False),
                ("observed_version", "1.0.79"),
                ("executable_digest", "sha256:" + "b" * 64),
            ):
                self.assertFalse(validate({**metadata, key: replacement}), key)
            outside = Path(tmp) / "outside-copilot"
            outside.write_bytes(executable.read_bytes())
            outside.chmod(0o755)
            self.assertFalse(validate(metadata, outside))
            escaped_link = root / "escaped-copilot"
            escaped_link.symlink_to(outside)
            self.assertFalse(validate(metadata, escaped_link))

    def test_external_observer_schema_cannot_supply_all_package_discovery(self) -> None:
        schema = json.loads((ROOT / "tests/e2e/schemas/runtime-attestations.schema.json").read_text())
        item = schema["properties"]["attestations"]["items"]
        self.assertNotIn("copilot", item["properties"]["client"]["enum"])
        self.assertNotIn("discovery", item["properties"]["level"]["enum"])
        self.assertNotIn("all_26_info", item["properties"]["scenario_id"]["enum"])
        self.assertNotIn("lifecycle_verified", item["properties"])
        self.assertNotIn("lifecycle_operations", item["properties"])
        self.assertNotIn("copilot", schema["$defs"]["nativeDiscoveryEvidence"]["properties"]["client"]["enum"])
        launch = json.loads((ROOT / "tests/e2e/schemas/launch-evidence-v4.schema.json").read_text())
        discovery_rule = launch["properties"]["matrix"]["items"]["allOf"][1]
        self.assertEqual(discovery_rule["then"]["properties"]["details"]["properties"]["evidence_basis"]["const"], "native_client_command")
        with tempfile.TemporaryDirectory() as tmp:
            artifact = Path(tmp) / "external.json"
            artifact.write_text(json.dumps({
                "schema_version": 1,
                "attestations": [{
                    "plugin": "context7", "client": "copilot",
                    "level": "discovery", "outcome": "passed",
                }],
            }))
            with self.assertRaisesRegex(ValueError, "unsupported level"):
                self.fixture_harness()._load_attestations(artifact)

    def test_all_package_info_fails_closed_on_incomplete_native_copilot_result(self) -> None:
        digest = "sha256:" + "a" * 64
        release = {
            "tree_digest": digest, "manifest_digest": digest,
            "distribution_id": "fixture/upstream", "distribution_kind": "upstream",
            "release_sequence": 1, "package_version": "1.0.0",
            "source_repository": "owner/repository", "source_revision": "b" * 40,
            "source_path": "plugins/fixture",
        }
        products = [{"id": f"plugin-{index}"} for index in range(26)]

        def run_with(mutate):
            harness = self.fixture_harness()
            harness.snapshot = {"products": products}
            harness.snapshot_digest = digest
            harness.config = {**harness.config, "all_package_operations": ["info"]}

            def command(argv, _sandbox, _clients):
                plugin = argv[1]
                physical_artifact_id = f"{plugin}-05408cd487ce"
                value = {
                    "tree_digest": digest, "receipt_reconciled": True,
                    "native_discovery_reconciled": True, "native_identity_state": "managed",
                    "client_version": "1.0.80",
                    "_observed_state": {"physical_artifact_id": physical_artifact_id},
                    "native_discovery_evidence": {
                        "basis": "native_client_command",
                        "version_operation": {
                            "argv": ["copilot", "--version"], "observed_client_version": "1.0.80",
                        },
                        "discovery_operation": {
                            "argv": ["copilot", "plugin", "list"], "discovered": True,
                            "product_id": observer._managed_native_product_id(plugin, physical_artifact_id),
                        },
                    },
                }
                mutate(value)
                return "passed", value, "reconciled"

            with mock.patch.object(harness, "fresh_sandbox", return_value=Path("/tmp/disposable")), \
                    mock.patch.object(harness, "all_package_client", return_value=("copilot", release)), \
                    mock.patch.object(harness, "directory_release", return_value=release), \
                    mock.patch.object(harness, "command_matches_release", return_value=True), \
                    mock.patch.object(harness, "command", side_effect=command):
                harness.all_package_matrix()
            return harness.rows

        self.assertTrue(all(row["outcome"] == "passed" for row in run_with(lambda _value: None)))
        self.assertTrue(all(row["details"]["native_identity_state"] == "managed" for row in run_with(lambda _value: None)))
        for mutation in (
            lambda value: value.pop("client_version"),
            lambda value: value.update(native_identity_state="copied"),
            lambda value: value["native_discovery_evidence"]["version_operation"].update(observed_client_version="1.0.79"),
            lambda value: value["native_discovery_evidence"]["discovery_operation"].update(argv=["sh", "-c", "copilot plugin list"]),
            lambda value: value["native_discovery_evidence"]["discovery_operation"].update(product_id="plugin-0"),
        ):
            self.assertTrue(all(row["outcome"] == "failed" for row in run_with(mutation)))

    def test_notion_records_are_exact_separate_and_all_passed(self) -> None:
        expected = {("notion", client, "runtime"): {"outcome": "passed"} for client in ("codex", "cursor", "kiro")}
        cases = (
            ({("notion", "codex", "runtime"): {"outcome": "passed"}}, expected, "primary runtime artifact"),
            ({}, {key: value for key, value in expected.items() if key[1] != "kiro"}, "exactly codex"),
            ({}, {key: ({"outcome": "failed"} if key[1] == "kiro" else value) for key, value in expected.items()}, "all Notion runtime records"),
        )
        for primary, notion, message in cases:
            with self.subTest(message=message), mock.patch.object(
                e2e.LaunchHarness, "_load_attestations", side_effect=[primary, notion, {}],
            ), self.assertRaisesRegex(ValueError, message):
                e2e.LaunchHarness(None, None, mode="fixture-only", consent=CONSENT)

    def test_empty_or_materialized_client_directories_never_become_native_evidence(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            home, manager = root / "home", root / "manager"
            (home / ".cursor").mkdir(parents=True)
            environment = {"HOME": str(home), "AGENTPLUGINS_HOME": str(manager)}
            empty = e2e.observed_state_identity(environment, "context7", ("cursor",))
            self.assertIsNone(empty["client_version"])
            self.assertFalse(empty["native_discovery_reconciled"])
            self.assertEqual(empty["evidence_basis"], "fixture_materialization")
            (home / ".cursor" / "context7.json").write_text('{"product":"context7"}')
            materialized = e2e.observed_state_identity(environment, "context7", ("cursor",))
            self.assertIsNone(materialized["client_version"])
            self.assertFalse(materialized["native_discovery_reconciled"])

    def test_no_newer_release_update_accepts_only_a_truthful_noop(self) -> None:
        update = release_fixture("local-update.json")
        self.assertTrue(observer.validate_cli_envelope(update, "update"))
        self.assertTrue(all(item["output"]["result"]["no_change"] for item in update["data"]["targets"]))
        forged = json.loads(json.dumps(update))
        forged["data"]["targets"][0]["output"]["result"]["mutated"] = True
        self.assertFalse(observer.validate_cli_envelope(forged, "update"))
    def test_mutable_refs_and_unknown_outcomes_are_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "mutable refs"):
            e2e.LaunchHarness._reject_mutable_refs({"ref": "main"})
        with tempfile.TemporaryDirectory() as tmp:
            attestation = Path(tmp) / "attestation.json"
            attestation.write_text(json.dumps({"schema_version": 1, "attestations": [{"plugin": "context7", "client": "codex", "level": "runtime", "outcome": "not_tested", "tuple": {}}]}))
            with self.assertRaisesRegex(ValueError, "invalid attestation outcome"):
                e2e.LaunchHarness(None, attestation, mode="fixture-only", consent=CONSENT)

    def test_duplicate_tuples_and_broad_chatgpt_claims_are_rejected(self) -> None:
        evidence = self.fixture_harness().export()
        evidence["matrix"].append(dict(evidence["matrix"][0]))
        with self.assertRaisesRegex(ValueError, "duplicate tuples"):
            e2e.assert_redacted(evidence)
        evidence = self.fixture_harness().export()
        evidence["matrix"].append({**evidence["matrix"][0], "id": "a" * 24, "scenario": "chatgpt", "plugin": "notion", "client": "chatgpt"})
        with self.assertRaisesRegex(ValueError, "ChatGPT"):
            e2e.assert_redacted(evidence)

    def test_chatgpt_runtime_uses_public_mcp_proof_instead_of_native_discovery(self) -> None:
        evidence = self.fixture_harness().export()
        digest = "sha256:" + "a" * 64
        public_mcp = {
            "basis": "protected_external_observer",
            "observer": "public-mcp-command-v1",
            "endpoint": "https://docs.mcp.cloudflare.com/mcp",
            "protocol_version": "2025-06-18",
            "initialize": {"method": "initialize", "passed": True},
            "list": {"method": "tools/list", "required_name": "search_cloudflare_documentation", "passed": True},
            "read": {
                "method": "tools/call", "name": "search_cloudflare_documentation",
                "read_only": True, "marker_digest": digest, "passed": True,
            },
        }
        row = {
            "id": "f" * 24, "scenario": "chatgpt_registered_binding",
            "plugin": "cloudflare-docs", "client": "chatgpt", "level": "runtime",
            "outcome": "passed", "reason": "public MCP observed",
            "tuple": {
                "product_id": "cloudflare-docs", "tree_digest": digest,
                "manifest_digest": digest, "distribution_id": "cloudflare/cloudflare-docs",
                "distribution_kind": "upstream", "release_sequence": 1,
                "package_version": "1.0.0", "source_repository": "cloudflare/cloudflare-docs",
                "source_revision": "b" * 40, "source_path": "agent-plugin",
                "snapshot_sequence": 1, "snapshot_digest": digest, "binary_digest": digest,
                "dependency_identity": "registered-app", "installer_version": "0.1.18",
                "adapter_version": "0.1.18", "client_version": "chatgpt-app-v1",
                "os": "linux", "architecture": "amd64", "observed_at": "2026-08-23T00:00:00Z",
            },
            "details": {
                "evidence_basis": "protected_external_observer", "runtime_proof": True,
                "native_discovery_proof": False, "public_mcp_proof": True,
                "public_mcp_evidence": public_mcp,
            },
        }
        evidence["matrix"].append(row)
        evidence["run"]["mode"] = "contract-test"
        e2e.assert_redacted(evidence)
        schema = json.loads((ROOT / "tests/e2e/schemas/launch-evidence-v4.schema.json").read_text())
        jsonschema.Draft202012Validator(schema).evolve(
            schema=schema["properties"]["matrix"]["items"],
        ).validate(row)
        for mutation in (
            lambda item: item["details"].update(public_mcp_proof=False),
            lambda item: item["details"].update(native_discovery_evidence={}),
            lambda item: item["details"]["public_mcp_evidence"].update(endpoint="https://example.test/mcp"),
            lambda item: item["details"]["public_mcp_evidence"]["read"].update(name="wrong-tool"),
        ):
            rejected = json.loads(json.dumps(evidence))
            mutation(rejected["matrix"][-1])
            with self.assertRaisesRegex(ValueError, "ChatGPT runtime"):
                e2e.assert_redacted(rejected)

    def test_launch_schema_rejects_unknown_outcome_and_mutable_ref(self) -> None:
        schema = json.loads((ROOT / "tests/e2e/schemas/launch-evidence-v5.schema.json").read_text())
        evidence = self.fixture_harness().export()
        jsonschema.Draft202012Validator(schema).validate(evidence)
        evidence["matrix"][0]["outcome"] = "not_tested"
        with self.assertRaises(jsonschema.ValidationError):
            jsonschema.Draft202012Validator(schema).validate(evidence)
        evidence = self.fixture_harness().export()
        evidence["matrix"][0]["details"]["ref"] = "main"
        with self.assertRaises(jsonschema.ValidationError):
            jsonschema.Draft202012Validator(schema).validate(evidence)

    def test_context7_contract_requires_one_three_target_grouped_lifecycle(self) -> None:
        harness = self.fixture_harness()
        harness.snapshot = {
            "sequence": 1,
            "evidence": [],
            "products": [{"id": "context7", "aliases": ["context7"], "default_distribution": "upstash/context7", "distributions": ["upstash/context7"], "minimum_capabilities": {"mcp": "required"}}],
            "distributions": [{"id": "upstash/context7", "product_id": "context7", "kind": "community", "status": "active", "release_policies": [{"release_sequence": 1, "status": "active", "current_evidence": [], "targets": [{"client": "codex", "scopes": ["user"]}, {"client": "cursor", "scopes": ["user"]}, {"client": "kiro", "scopes": ["user"]}]}], "releases": [{"sequence": 1, "components": ["mcp"], "package_version": "1.0.0", "tree_digest": "sha256:" + "a" * 64, "manifest_digest": "sha256:" + "b" * 64, "package_source": {"repository": "upstash/context7", "revision": "1" * 40, "path": "plugins/context7"}}]}],
        }
        harness.snapshot_digest = "sha256:" + "c" * 64
        harness.binary_digest = "sha256:" + "d" * 64
        harness.expected_version = "0.1.18"
        commands = [[operation, "context7", "--target", "codex,cursor,kiro", "--format", "json"] for operation in ("add", "update", "repair", "remove")]
        acquisition = {
            "acquisition_id": "fetch-1", "acquisition_count": 1,
            "tree_digest": "sha256:" + "a" * 64,
            "manifest_digest": "sha256:" + "b" * 64,
            "closure_digest": observer.grouped_acquisition_closure_digest(
                "directory", "upstash/context7", "plugins/context7", "1" * 40,
                "sha256:" + "a" * 64, "sha256:" + "b" * 64,
            ),
            "source_kind": "directory", "fetched": True, "validated": True,
            "source_repository": "upstash/context7", "source_revision": "1" * 40,
            "source_path": "plugins/context7",
            "targets": [{"target": client} for client in ("codex", "cursor", "kiro")],
        }
        acquisition["target_outcomes"] = {
            client: {
                "outcome": "passed", "acquisition_id": "fetch-1",
                "tree_digest": acquisition["tree_digest"],
                "manifest_digest": acquisition["manifest_digest"],
                "closure_digest": acquisition["closure_digest"],
            }
            for client in ("codex", "cursor", "kiro")
        }
        value = {
            "commands": commands, "acquisition_digests": ["sha256:" + "a" * 64],
            "acquisition": acquisition,
            "target_outcomes": {client: "passed" for client in ("codex", "cursor", "kiro")},
            "operation_outcomes": {operation: "passed" for operation in ("add", "update", "repair", "remove")},
            "tuple": harness.evidence_tuple("context7", ["codex", "cursor", "kiro"], client_version="driver", dependency="single-acquisition"),
        }
        with mock.patch.object(harness, "driven_scenario", return_value=("passed", value, "proved")):
            harness.context7_multi_target()
        self.assertEqual([row["outcome"] for row in harness.rows], ["passed"] * 4)
        self.assertTrue(all(row["details"]["evidence_basis"] == "repository_owned_disposable_observer" for row in harness.rows))
        self.assertTrue(all(row["details"]["target_argument"] == "codex,cursor,kiro" for row in harness.rows))
        self.assertTrue(all(row["details"]["acquisition"] == acquisition for row in harness.rows))
        self.assertNotIn("--yes", commands[0])

        invalid = json.loads(json.dumps(value))
        del invalid["acquisition"]["closure_digest"]
        rejected = self.fixture_harness()
        rejected.snapshot = harness.snapshot
        rejected.snapshot_digest = harness.snapshot_digest
        rejected.binary_digest = harness.binary_digest
        rejected.expected_version = harness.expected_version
        with mock.patch.object(rejected, "driven_scenario", return_value=("passed", invalid, "claimed")):
            rejected.context7_multi_target()
        self.assertEqual([row["outcome"] for row in rejected.rows], ["failed"] * 4)

    def test_repository_observer_derives_grouped_lifecycle_from_receipts_and_native_files(self) -> None:
        clients = ("codex", "cursor", "kiro")
        add = release_fixture("add.json")
        repair = release_fixture("repair.json")
        info = release_fixture("info.json")
        remove = release_fixture("remove.json")
        update = json.loads(json.dumps(add))
        update["command"] = "update"
        update["data"].pop("acquisition")
        update["data"].pop("target_outcomes")
        for target in update["data"]["targets"]:
            target["selected"] = True
            target["output"]["result"]["mutated"] = False
            target["output"]["result"]["no_change"] = True

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            home = root / "home"
            manager = root / "manager"
            workspace = root / "workspace"
            for path in (home, manager, workspace):
                path.mkdir()
            state_path = manager / "state-v2.json"
            state = self.sandbox_state_fixture(release_fixture("state-v2.json"), manager)
            state["installations"][0]["origin_mode"] = "directory"
            state["installations"][0]["directory"] = {
                "product_id": "context7", "distribution_id": "upstash/context7",
                "distribution_kind": "upstream", "desired_release_sequence": 1,
                "snapshot_schema": 1, "snapshot_sequence": 1,
                "snapshot_digest": "sha256:" + "c" * 64,
            }
            for binding in state["installations"][0]["clients"].values():
                binding["package_revision"].update(
                    distribution_id="upstash/context7", release_sequence=1,
                )
            retained_receipt = next(iter(state["installations"][0]["data_receipts"].values()))
            remove["data"]["retained_data"] = [{
                key: retained_receipt[key]
                for key in ("data_receipt_id", "physical_backend_id", "scope", "state")
            }]
            envelopes = {"add": add, "update": update, "repair": repair, "info": info, "remove": remove}

            def write_state(value: dict) -> None:
                state_path.write_text(json.dumps(value, sort_keys=True, separators=(",", ":")))

            def materialize() -> None:
                for binding in state["installations"][0]["clients"].values():
                    target = Path(binding["target_locator"]); target.mkdir(parents=True, exist_ok=True)
                    (target / "plugin.json").write_text(json.dumps({"name": "context7", "client": binding["client_id"]}))
                    managed = observer.package_identity(target)["tree_digest"]
                    binding["native_objects"][0]["managed_digest"] = managed
                    binding["receipts"][0]["after_digest"] = managed

            def invoke(binary: Path, argv: list[str], cwd: Path, challenge: str, **kwargs):
                del binary, cwd
                command = argv[0]
                if command == "add":
                    materialize()
                    write_state(state)
                elif command == "repair":
                    repaired = json.loads(json.dumps(state))
                    for binding in repaired["installations"][0]["clients"].values():
                        previous = binding["receipts"][-1]
                        receipt = {**previous, "operation_id": previous["operation_id"] + "-repair",
                                   "operation_group_id": repair["data"]["operation_id"], "sequence": 2,
                                   "before_digest": previous["after_digest"]}
                        receipt["staging_path"] += "-repair"
                        receipt["backup_path"] += "-repair"
                        binding["receipts"].append(receipt)
                    state.clear()
                    state.update(repaired)
                    write_state(state)
                elif command == "remove":
                    installation = state["installations"][0]
                    authority = observer.removal_authority_from_installation(installation)
                    self.assertIsNotNone(authority)
                    removed = json.loads(json.dumps(state))
                    removed_installation = removed["installations"][0]
                    removed_installation["clients"] = {}
                    removed_installation["data_retained"] = True
                    removed["transaction_receipts"] = [
                        {
                            "operation_id": f"remove-{index}",
                            "operation_group_id": remove["data"]["operation_id"],
                            "sequence": 1,
                            "mutation_type": "directory_remove",
                            "client_binding_id": record["client_binding_id"],
                            "active_path": record["active_path"],
                            "before_digest": record["before_digest"],
                            "backup_path": str(Path(record["active_path"]).parent / f".removed-{index}"),
                            "phase": "committed",
                        }
                        for index, record in enumerate(authority.values(), 1)
                    ]
                    write_state(removed)
                    for record in authority.values():
                        shutil.rmtree(record["active_path"])
                value = envelopes[command]
                completed = subprocess.CompletedProcess(
                    [str(root / "agentplugins"), *argv], 0, json.dumps(value), "",
                )
                trace = {
                    "challenge": challenge, "argv": argv,
                    "started_at": "2026-08-24T00:00:00Z", "ended_at": "2026-08-24T00:00:01Z",
                    "exit_code": 0, "stdout_digest": "sha256:" + "a" * 64,
                    "stderr_digest": "sha256:" + "b" * 64,
                }
                return completed, trace

            context = {
                "release": {
                    "product_id": "context7", "distribution_id": "upstash/context7",
                    "distribution_kind": "upstream", "release_sequence": 1,
                    "tree_digest": add["data"]["tree_digest"],
                    "manifest_digest": add["data"]["manifest_digest"],
                    "package_version": add["data"]["version"],
                    "source_repository": "upstash/context7",
                    "source_revision": add["data"]["revision"],
                    "source_path": "plugins/agent-plugins/context7",
                },
                "snapshot_sequence": 1, "snapshot_digest": "sha256:" + "c" * 64,
                "directory_digest": "sha256:" + "c" * 64,
                "binary_digest": "sha256:" + "d" * 64, "expected_version": "0.1.18",
            }
            with mock.patch.dict(os.environ, {"HOME": str(home), "AGENTPLUGINS_HOME": str(manager)}), mock.patch.object(
                observer, "traced", side_effect=invoke,
            ):
                passed, value = observer.lifecycle(
                    root / "agentplugins", "context7", clients, workspace, "challenge", context,
                    include_repair=True,
                )

        self.assertTrue(passed, value["operation_outcomes"])
        self.assertEqual(value["operation_outcomes"], {name: "passed" for name in ("add", "update", "repair", "info", "remove")})
        self.assertTrue(value["no_newer_release_update_noop"])
        self.assertEqual(len(value["operation_observations"]), 5)
        self.assertIsNotNone(observer.grouped_acquisition_proof(value["values"]["add"], clients))
    def test_grouped_acquisition_proof_fails_closed_when_event_is_missing_or_ambiguous(self) -> None:
        valid = release_fixture("add.json")
        clients = ("codex", "cursor", "kiro")
        self.assertIsNotNone(observer.grouped_acquisition_proof(valid, clients))
        for mutation in (
            {**valid, "command": "update"},
            {**valid, "data": {**valid["data"], "targets": valid["data"]["targets"][:-1]}},
            {**valid, "data": {**valid["data"], "target_outcomes": {"codex": valid["data"]["target_outcomes"]["codex"]}}},
        ):
            self.assertIsNone(observer.grouped_acquisition_proof(mutation, clients))
    def test_raw_cli_json_rejects_duplicate_keys_and_non_integer_schema_versions(self) -> None:
        event = '{"acquisition_id":"fetch-1","acquisition_count":1,"tree_digest":"sha256:' + "a" * 64 + '","manifest_digest":"sha256:' + "b" * 64 + '","closure_digest":"sha256:' + "c" * 64 + '","source_kind":"github","fetched":true,"validated":true}'
        binding = '{"outcome":"passed","acquisition_id":"fetch-1","tree_digest":"sha256:' + "a" * 64 + '","manifest_digest":"sha256:' + "b" * 64 + '","closure_digest":"sha256:' + "c" * 64 + '"}'
        outcomes = '"codex":' + binding + ',"cursor":' + binding + ',"kiro":' + binding
        prefix = '{"schema_version":1,"command":"add","result":"success","data":{'
        duplicate_proof = prefix + '"acquisition":' + event + ',"acquisition":' + event + ',"target_outcomes":{' + outcomes + '}}}'
        duplicate_client = prefix + '"acquisition":' + event + ',"target_outcomes":{' + outcomes + ',"cursor":' + binding + '}}}'
        for raw in (duplicate_proof, duplicate_client):
            completed = subprocess.CompletedProcess(["agentplugins"], 0, stdout=raw, stderr="")
            self.assertIsNone(observer.json_output(completed))
        for version in (True, 1.0):
            value = {
                "schema_version": version, "command": "add", "result": "success",
                "data": {"acquisition": json.loads(event), "target_outcomes": json.loads("{" + outcomes + "}")},
            }
            completed = subprocess.CompletedProcess(["agentplugins"], 0, stdout=json.dumps(value), stderr="")
            self.assertIsNone(observer.grouped_acquisition_proof(observer.json_output(completed), ("codex", "cursor", "kiro")))

    def test_exact_agentplugins_0_1_14_stdout_and_state_v4_fixtures(self) -> None:
        stdout_bytes, add_fixture = self.agentplugins_0_1_14_add_fixture()
        state_text, state_fixture = self.agentplugins_0_1_14_state_fixture()
        state_bytes = state_text.encode()
        completed = subprocess.CompletedProcess(
            ["agentplugins", "add", "context7", "--target", "codex,cursor,kiro", "--format", "json"],
            0, stdout_bytes.decode(), "",
        )
        envelope = observer.json_output(completed, "add")
        self.assertIsNotNone(envelope)
        proof = observer.grouped_acquisition_proof(envelope, ("codex", "cursor", "kiro"))
        self.assertEqual(
            (proof["source_repository"], proof["source_revision"], proof["source_path"]),
            ("upstash/context7", "769c6cd22c3d95462d1f55d789e9532cabefa5a9", "plugins/agent-plugins/context7"),
        )
        self.assertEqual([item["target"] for item in proof["targets"]], ["codex", "cursor", "kiro"])
        fixture_installation = state_fixture["installations"][0]
        self.assertEqual(
            add_fixture["data"]["targets"][0]["output"]["result"]["installation_id"],
            fixture_installation["installation_id"],
        )
        self.assertEqual(add_fixture["data"]["tree_digest"], fixture_installation["source"]["tree_digest"])
        self.assertEqual(add_fixture["data"]["manifest_digest"], fixture_installation["package"]["manifest_digest"])
        duplicated_target = json.loads(json.dumps(envelope))
        duplicated_target["data"]["targets"].append(duplicated_target["data"]["targets"][0])
        self.assertIsNone(observer.grouped_acquisition_proof(duplicated_target, ("codex", "cursor", "kiro")))
        with tempfile.TemporaryDirectory() as tmp:
            manager = Path(tmp)
            state_fixture = self.sandbox_state_fixture(state_fixture, manager)
            (manager / "state-v2.json").write_text(json.dumps(state_fixture))
            installation = observer.selected_manager_installation(manager, "context7")
            receipts = observer.installation_receipts(manager, "context7")
            self.assertEqual(installation["operation_group_id"], envelope["data"]["operation_id"])
            self.assertEqual([item["binding_client"] for item in receipts], ["cursor", "codex", "kiro"])
            self.assertTrue(all(item["receipt"]["phase"] == "committed" for item in receipts))

    def test_all_real_0_1_14_lifecycle_and_migration_envelopes_are_exact(self) -> None:
        fixtures = {
            "add.json": "add", "local-add.json": "add", "info.json": "info",
            "local-update.json": "update", "repair.json": "repair", "remove.json": "remove",
            "migrate-dry-run.json": "migrate-state", "migrate-apply.json": "migrate-state",
        }
        for name, command in fixtures.items():
            with self.subTest(name=name):
                self.assertTrue(observer.validate_cli_envelope(release_fixture(name), command))
        local = release_fixture("local-add.json")
        local_proof = observer.grouped_acquisition_proof(local, ("codex", "cursor", "kiro"))
        self.assertEqual(local_proof["source_kind"], "local")
        self.assertEqual((local_proof["source_repository"], local_proof["source_revision"], local_proof["source_path"]), ("", "", ""))
        update = release_fixture("local-update.json")
        self.assertTrue(all(
            item["selected"] is True and item["output"]["result"]["mutated"] is False
            and item["output"]["result"]["no_change"] is True
            and item["output"]["result"]["group_phase"] == "external_completed"
            for item in update["data"]["targets"]
        ))

    def test_directory_info_contract_preserves_every_released_optional_surface(self) -> None:
        value = release_fixture("info-directory.json")
        self.assertTrue(observer.validate_cli_envelope(value, "info"))
        data = value["data"]
        self.assertEqual(
            set(data["directory"]),
            {
                "product_id", "recorded_distribution", "current_distribution",
                "reviewed_default_distribution", "recorded_revision", "current_revision",
                "current_repository", "current_package_path", "recorded_release_sequence",
                "current_release_sequence", "recorded_snapshot_sequence",
                "current_snapshot_sequence",
            },
        )
        client = data["clients"][0]
        self.assertIn("evidence", client["package_revision"])
        self.assertIn("native_discovery_evidence", client)
        self.assertIn("warnings", data)
        self.assertFalse(data["mixed_version"])
        self.assertNotIn("convergence_action", data)
        for field, replacement in (
            ("directory", {**data["directory"], "distribution": "invented"}),
            ("warnings", [{**data["warnings"][0], "next_action": True}]),
            ("clients", [{**client, "native_discovery_evidence": "managed"}]),
        ):
            forged = json.loads(json.dumps(value)); forged["data"][field] = replacement
            self.assertFalse(observer.validate_cli_envelope(forged, "info"))
        for mutate in (
            lambda item: item["trust"].update(workflow="attacker/repo/.github/workflows/evidence.yml"),
            lambda item: item["trust"].update(source_digest="f" * 40),
            lambda item: item["artifact"].update(revision="e" * 40),
        ):
            forged = json.loads(json.dumps(value))
            mutate(forged["data"]["clients"][0]["package_revision"]["evidence"][0])
            self.assertFalse(observer.validate_cli_envelope(forged, "info"))
        for argv in (["copilot", "plugin", "list", "--all"], ["cursor", "plugin", "list"]):
            forged = json.loads(json.dumps(value))
            forged["data"]["clients"][0]["native_discovery_evidence"]["discovery_operation"]["argv"] = argv
            self.assertFalse(observer.validate_cli_envelope(forged, "info"))

    def test_real_envelopes_reject_wrong_commands_target_forgery_and_identity_mismatch(self) -> None:
        add = release_fixture("add.json")
        mutations = []
        for command in ("update", "repair", "remove", "migrate-state"):
            mutations.append({**add, "command": command})
        mutations.extend((
            {**add, "result": "failure"},
            {**add, "data": {**add["data"], "targets": add["data"]["targets"][:-1], "succeeded": 2}},
            {**add, "data": {**add["data"], "targets": [*add["data"]["targets"], add["data"]["targets"][0]], "succeeded": 4}},
        ))
        partial = json.loads(json.dumps(add))
        partial["data"]["targets"][0]["status"] = "external_partial"
        mutations.append(partial)
        unknown = json.loads(json.dumps(add))
        unknown["data"]["targets"][0]["output"]["result"]["group_phase"] = "managed_unknown"
        mutations.append(unknown)
        install = json.loads(json.dumps(add))
        install["data"]["targets"][0]["output"]["result"]["installation_id"] = "other"
        mutations.append(install)
        digest = json.loads(json.dumps(add))
        digest["data"]["targets"][0]["output"]["tree_digest"] = "sha256:" + "9" * 64
        mutations.append(digest)
        revision = json.loads(json.dumps(add))
        revision["data"]["targets"][0]["output"]["revision"] = "2" * 40
        mutations.append(revision)
        outcome = json.loads(json.dumps(add))
        outcome["data"]["target_outcomes"]["codex"]["outcome"] = "partial"
        mutations.append(outcome)
        for mutation in mutations:
            with self.subTest(mutation=mutation.get("command")):
                self.assertFalse(observer.validate_cli_envelope(mutation, "add"))

    def test_closure_digest_is_domain_separated_length_prefixed_and_origin_bound(self) -> None:
        github = release_fixture("add.json")
        proof = observer.grouped_acquisition_proof(github, ("codex", "cursor", "kiro"))
        self.assertEqual(
            proof["closure_digest"],
            observer.grouped_acquisition_closure_digest(
                "github", proof["source_repository"], proof["source_path"], proof["source_revision"],
                proof["tree_digest"], proof["manifest_digest"],
            ),
        )
        for field, replacement in (
            ("source_kind", "directory"), ("tree_digest", "sha256:" + "9" * 64),
            ("manifest_digest", "sha256:" + "8" * 64),
        ):
            forged = json.loads(json.dumps(github))
            forged["data"]["acquisition"][field] = replacement
            self.assertIsNone(observer.grouped_acquisition_proof(forged, ("codex", "cursor", "kiro")))

    def test_migration_envelopes_reject_old_flags_shapes_and_inconsistent_counts(self) -> None:
        dry = release_fixture("migrate-dry-run.json")
        applied = release_fixture("migrate-apply.json")
        for mutation in (
            {**dry, "data": {**dry["data"], "status": "dry_run", "mutated": False}},
            {**dry, "data": {**dry["data"], "backup_created": True}},
            {**applied, "data": {**applied["data"], "migrated": 0}},
            {**applied, "data": {**applied["data"], "source_schema": 4}},
            {**applied, "data": {**applied["data"], "installations": True}},
        ):
            self.assertFalse(observer.validate_cli_envelope(mutation, "migrate-state"))
        self.assertTrue(observer.copy_ready_migration_guidance("agentplugins migrate-state --dry-run\nagentplugins migrate-state"))
        self.assertFalse(observer.copy_ready_migration_guidance("agentplugins migrate-state --dry-run\nagentplugins migrate-state --expected-digest sha256:bad"))

    def test_state_v4_boundary_rejects_symlinks_duplicates_ambiguity_and_split_authority(self) -> None:
        raw, state = self.agentplugins_0_1_14_state_fixture()
        with tempfile.TemporaryDirectory() as tmp:
            manager = Path(tmp)
            state_path = manager / "state-v2.json"
            target = manager / "outside.json"
            target.write_text(json.dumps(state))
            state_path.symlink_to(target)
            self.assertIsNone(observer.manager_state(manager))
            state_path.unlink()
            state = self.sandbox_state_fixture(state, manager)
            state_path.write_text(json.dumps(state))
            (manager / "arbitrary.json").write_text('{"installations":[{"declared_name":"context7"}]}')
            self.assertIsNotNone(observer.selected_manager_installation(manager, "context7"))
            ambiguous = json.loads(json.dumps(state))
            ambiguous["installations"].append(ambiguous["installations"][0])
            state_path.write_text(json.dumps(ambiguous))
            self.assertIsNone(observer.selected_manager_installation(manager, "context7"))
            unrelated = json.loads(json.dumps(state))
            second = json.loads(json.dumps(unrelated["installations"][0]))
            second["installation_id"] = "other-installation"
            second["declared_name"] = second["package"]["declared_name"] = "other"
            unrelated["installations"].append(second)
            state_path.write_text(json.dumps(unrelated))
            self.assertIsNone(observer.selected_manager_installation(manager, "context7"))
            split = json.loads(json.dumps(state))
            binding = next(iter(split["installations"][0]["clients"].values()))
            binding["receipts"][0]["client_binding_id"] = "other-binding"
            state_path.write_text(json.dumps(split))
            self.assertIsNone(observer.installation_receipts(manager, "context7"))
            malformed_receipts = json.loads(json.dumps(state))
            malformed_receipts["installations"][0]["data_receipts"] = []
            state_path.write_text(json.dumps(malformed_receipts))
            self.assertIsNone(observer.manager_state(manager))
            state_path.write_text(raw.replace('"schema_version": 4', '"schema_version": 4, "schema_version": 4', 1))
            self.assertIsNone(observer.manager_state(manager))
            for invalid_version in (True, 4.0):
                invalid = json.loads(json.dumps(state))
                invalid["schema_version"] = invalid_version
                state_path.write_text(json.dumps(invalid))
                self.assertIsNone(observer.manager_state(manager))

    def test_non_add_envelopes_require_exact_command_result_status_and_fields(self) -> None:
        valid = {"info": release_fixture("info.json"), "repair": release_fixture("repair.json"), "remove": release_fixture("remove.json")}
        for command, envelope in valid.items():
            self.assertTrue(observer.validate_cli_envelope(envelope, command))
            for mutation in (
                {**envelope, "schema_version": True},
                {**envelope, "schema_version": 1.0},
                {**envelope, "command": "wrong"},
                {**envelope, "result": "failure"},
                {**envelope, "data": {}},
            ):
                self.assertFalse(observer.validate_cli_envelope(mutation, command))

    def test_public_envelopes_reject_hybrid_empty_typed_and_argv_mutations(self) -> None:
        add = release_fixture("add.json")
        valid_argv = ["add", "context7", "--target", "codex,cursor,kiro", "--format", "json"]
        self.assertTrue(observer.validate_cli_envelope(add, "add", requested_argv=valid_argv))
        mutations = []
        for field in ("version", "source"):
            changed = json.loads(json.dumps(add))
            changed["data"][field] = ""
            for target in changed["data"]["targets"]:
                target["output"][field] = ""
            mutations.append(changed)
        manifest = json.loads(json.dumps(add))
        manifest["data"]["targets"][0]["output"]["manifest_digest"] = "sha256:" + "0" * 64
        mutations.append(manifest)
        legacy = json.loads(json.dumps(add))
        legacy["data"]["mutated"] = True
        mutations.append(legacy)
        for changed in mutations:
            self.assertFalse(observer.validate_cli_envelope(changed, "add", requested_argv=valid_argv))
        self.assertFalse(observer.validate_cli_envelope(add, "add", requested_argv=[*valid_argv, "--yes"]))

        info = release_fixture("info.json")
        client = info["data"]["clients"][0]
        client["receipt_reconciled"] = "yes"
        self.assertFalse(observer.validate_cli_envelope(info, "info"))
        empty = release_fixture("info.json")
        empty["data"]["source"] = ""
        self.assertFalse(observer.validate_cli_envelope(empty, "info"))

        dry = release_fixture("migrate-dry-run.json")
        self.assertTrue(observer.validate_cli_envelope(
            dry, "migrate-state", requested_argv=["migrate-state", "--dry-run", "--format", "json"],
        ))
        self.assertFalse(observer.validate_cli_envelope(
            dry, "migrate-state", requested_argv=["migrate-state", "--format", "json"],
        ))

    def test_filesystem_snapshot_covers_symlink_empty_directory_mode_and_content(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            empty = root / "empty"
            empty.mkdir()
            body = root / "body"
            body.write_bytes(b"one")
            link = root / "link"
            link.symlink_to("body")
            with self.assertRaisesRegex(ValueError, "unsupported filesystem object"):
                observer.filesystem_snapshot(root)
            link.unlink()
            before = observer.filesystem_snapshot(root)
            empty.chmod(0o700)
            body.write_bytes(b"two")
            after = observer.filesystem_snapshot(root)
            self.assertEqual(before["empty"]["kind"], "directory")
            self.assertNotEqual(before["empty"]["mode"], after["empty"]["mode"])
            self.assertNotEqual(before["body"]["digest"], after["body"]["digest"])

    def test_marker_creation_rejects_leaf_symlink_and_directory_replacement(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            locator = root / "owned"
            locator.mkdir()
            (locator / "marker").symlink_to(root / "outside")
            self.assertIsNone(observer.create_contained_marker(locator, (root,), "marker", b"proof"))
            (locator / "marker").unlink()
            original_open = os.open
            swapped = False

            def replacing_open(path, flags, *args, **kwargs):
                nonlocal swapped
                if not swapped and Path(path) == locator:
                    swapped = True
                    locator.rename(root / "old-owned")
                    locator.mkdir()
                return original_open(path, flags, *args, **kwargs)

            with mock.patch.object(observer.os, "open", side_effect=replacing_open):
                self.assertIsNone(observer.create_contained_marker(locator, (root,), "marker", b"proof"))
            self.assertFalse((locator / "marker").exists())

    def test_conforming_stdout_without_lifecycle_mutation_fails(self) -> None:
        fixture_root = AGENTPLUGINS_0_1_14_FIXTURES
        fake = f'''#!/usr/bin/python3
import pathlib, sys
fixtures = pathlib.Path({str(AGENTPLUGINS_0_1_14_FIXTURES)!r})
name = {{"add":"add.json", "update":"local-update.json", "repair":"repair.json", "info":"info.json", "remove":"remove.json"}}[sys.argv[1]]
print((fixtures / name).read_text(), end="")
'''
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / "agentplugins"
            binary.write_text(fake)
            binary.chmod(0o700)
            home, manager, workspace = root / "home", root / "manager", root / "workspace"
            workspace.mkdir()
            context = {
                "release": {"product_id": "context7", "tree_digest": "sha256:" + "a" * 64, "manifest_digest": "sha256:" + "b" * 64, "distribution_id": "upstash/context7", "distribution_kind": "upstream", "release_sequence": 1, "package_version": "1.0.0", "source_repository": "upstash/context7", "source_revision": "1" * 40, "source_path": "plugins/agent-plugins/context7"},
                "snapshot_sequence": 1, "directory_digest": "sha256:" + "c" * 64,
                "binary_digest": "sha256:" + "d" * 64, "expected_version": "0.1.18",
            }
            with mock.patch.dict(os.environ, {"HOME": str(home), "AGENTPLUGINS_HOME": str(manager)}, clear=False):
                passed, value = observer.lifecycle(binary, "context7", ("codex", "cursor", "kiro"), workspace, "challenge", context, include_repair=True)
        self.assertFalse(passed, value)
        self.assertEqual(value["operation_outcomes"]["add"], "failed")

    def test_captured_full_sha_failure_requires_exact_envelope_stderr_and_argv(self) -> None:
        value = release_fixture("direct-update-failure.json")
        stderr = (AGENTPLUGINS_0_1_14_FIXTURES / "direct-update-failure.stderr.txt").read_text()
        kwargs = {
            "plugin": "context7", "source": "upstash/context7//plugins/agent-plugins/context7",
            "revision": "769c6cd22c3d95462d1f55d789e9532cabefa5a9",
            "tree_digest": "sha256:08eed3b67f2e71a11b68baa594380c2f69ec1bc97584d701deaf7942ac34c0d8",
            "expected_targets": ("codex", "cursor", "kiro"),
            "requested_argv": ["update", "context7", "--target", "codex,cursor,kiro", "--format", "json"],
        }
        self.assertTrue(observer.validate_full_sha_update_failure(value, stderr, **kwargs))
        forged = json.loads(json.dumps(value))
        forged["data"]["failed"] = 2
        self.assertFalse(observer.validate_full_sha_update_failure(forged, stderr, **kwargs))
        self.assertFalse(observer.validate_full_sha_update_failure(value, stderr.rstrip(), **kwargs))
        self.assertFalse(observer.validate_full_sha_update_failure(
            value, stderr, **{**kwargs, "requested_argv": [*kwargs["requested_argv"], "--yes"]},
        ))

    def test_grouped_optional_directory_envelope_is_semantically_exact(self) -> None:
        value = release_fixture("add.json")
        value["data"]["directory"] = {
            "product_id": "context7", "distribution_id": "upstash/context7",
            "distribution_kind": "upstream", "desired_release_sequence": 1,
            "snapshot_schema": 1, "snapshot_sequence": 41,
            "snapshot_digest": "sha256:" + "a" * 64,
        }
        self.assertTrue(observer.validate_cli_envelope(value, "add"))
        for mutation in (
            {**value["data"]["directory"], "product_id": "unrelated"},
            {key: child for key, child in value["data"]["directory"].items() if key != "snapshot_digest"},
            {**value["data"]["directory"], "distribution_kind": "invented"},
        ):
            forged = json.loads(json.dumps(value)); forged["data"]["directory"] = mutation
            self.assertFalse(observer.validate_cli_envelope(forged, "add"))

    def test_capture_provenance_binds_every_retained_fixture_byte(self) -> None:
        provenance = e2e.validate_capture_provenance()
        self.assertGreaterEqual(len(provenance["captures"]), 11)
        self.assertEqual(provenance["release"]["version_stdout"], "agentplugins 0.1.14")
        self.assertEqual(
            provenance["release"]["capture_evidence_level"],
            "sanitized_manifest_only_no_raw_or_binary_linkage",
        )
        self.assertTrue(all("capture_sha256" not in item for item in provenance["captures"]))
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); fixture = root / "one.json"; fixture.write_text("{}")
            forged = json.loads(json.dumps(provenance))
            forged["captures"][0]["sanitized_sha256"] = "sha256:" + "0" * 64
            path = root / "provenance.json"; path.write_text(json.dumps(forged))
            with self.assertRaises(ValueError):
                e2e.validate_capture_provenance(path)

    def test_capture_fixture_and_provenance_cannot_move_together_without_trusted_root(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            copied = Path(tmp) / "agentplugins-0.1.14"
            shutil.copytree(AGENTPLUGINS_0_1_14_FIXTURES, copied)
            fixture = copied / "add.json"
            fixture.write_bytes(fixture.read_bytes() + b"\n")
            provenance_path = copied / "provenance.json"
            provenance = json.loads(provenance_path.read_text())
            capture = next(item for item in provenance["captures"] if item["fixture"] == "add.json")
            capture["sanitized_sha256"] = "sha256:" + hashlib.sha256(fixture.read_bytes()).hexdigest()
            provenance_path.write_text(json.dumps(provenance))
            with mock.patch.object(e2e, "ROOT", ROOT):
                with self.assertRaisesRegex(ValueError, "trusted repository root"):
                    e2e.validate_capture_provenance(provenance_path)

    def test_sanitized_captures_bind_to_independently_authenticated_release_contract(self) -> None:
        captured = e2e.validate_capture_provenance()["release"]
        self.assertFalse(captured["release_binary_authenticated"])
        self.assertEqual(captured["tag_commit"], "cd766a39f79938b10c212dbe73e52ecf6731937a")
        self.assertNotEqual(captured["tag_commit"], e2e.TRUSTED_CLI_RELEASE_COMMIT)
        slots = (
            ("darwin-amd64", "darwin", "amd64", ""), ("darwin-arm64", "darwin", "arm64", ""),
            ("linux-amd64", "linux", "amd64", ""), ("linux-arm64", "linux", "arm64", ""),
            ("windows-amd64", "windows", "amd64", ".exe"),
            ("windows-arm64", "windows", "arm64", ".exe"),
        )
        assets = {
            key: {"file": f"agentplugins_0.1.24_{os_name}_{arch}{suffix}", "sha256": f"{index + 1:064x}", "size": index + 1}
            for index, (key, os_name, arch, suffix) in enumerate(slots)
        }
        manifest = {
            "schema_version": 2, "tag": e2e.TRUSTED_CLI_RELEASE_TAG, "commit": e2e.TRUSTED_CLI_RELEASE_COMMIT,
            "version": "0.1.24", "assets": assets,
        }
        asset = assets["linux-amd64"]
        digest = "sha256:" + asset["sha256"]
        identity = {
            "repository": e2e.TRUSTED_CLI_RELEASE_REPOSITORY, "tag": e2e.TRUSTED_CLI_RELEASE_TAG,
            "tag_commit": e2e.TRUSTED_CLI_RELEASE_COMMIT, "release_id": 114, "immutable": True,
        }
        attestation = {
            "repository": e2e.TRUSTED_CLI_RELEASE_REPOSITORY, "workflow": e2e.TRUSTED_CLI_RELEASE_WORKFLOW,
            "tag": e2e.TRUSTED_CLI_RELEASE_TAG, "tag_commit": e2e.TRUSTED_CLI_RELEASE_COMMIT,
            "issuer": "https://token.actions.githubusercontent.com",
            "source_ref": e2e.TRUSTED_CLI_RELEASE_SOURCE_REF,
            "source_digest": e2e.TRUSTED_CLI_RELEASE_COMMIT,
            "predicate_type": "https://slsa.dev/provenance/v1",
            "subject_name": asset["file"], "subject_digest": digest,
            "runner_environment": "github-hosted",
            "asset_name": asset["file"], "asset_digest": digest, "verified": True,
        }
        arguments = {
            "release_manifest": manifest, "release_identity": identity,
            "release_manifest_digest": "sha256:" + "a" * 64,
            "release_checksums_digest": "sha256:" + "b" * 64,
            "asset_name": asset["file"], "asset_digest": digest,
            "asset_attestation": attestation,
        }
        binding = e2e.validate_capture_release_binding(**arguments)
        self.assertFalse(binding["fixture_release_binary_authenticated"])
        self.assertTrue(binding["enforced_binary_authenticated"])
        self.assertEqual(binding["fixture_recorded_tag_commit"], captured["tag_commit"])
        self.assertEqual(binding["enforced_release_tag_commit"], e2e.TRUSTED_CLI_RELEASE_COMMIT)
        self.assertEqual(binding["capture_evidence_level"], "sanitized_manifest_only_no_raw_or_binary_linkage")
        for name, mutation in (
            ("binary", {**arguments, "asset_digest": "sha256:" + "f" * 64}),
            ("commit", {**arguments, "release_identity": {**identity, "tag_commit": "f" * 40}}),
            ("manifest", {**arguments, "release_manifest_digest": "invalid"}),
            ("checksums", {**arguments, "release_checksums_digest": "invalid"}),
            ("attestation", {**arguments, "asset_attestation": {**attestation, "verified": False}}),
            ("subject-name", {**arguments, "asset_attestation": {**attestation, "subject_name": "other-asset"}}),
            ("subject-digest", {**arguments, "asset_attestation": {**attestation, "subject_digest": "sha256:" + "e" * 64}}),
        ):
            with self.subTest(name=name), self.assertRaisesRegex(ValueError, "not bound"):
                e2e.validate_capture_release_binding(**mutation)

    def test_schema_two_capture_has_complete_cross_record_identities(self) -> None:
        state = json.loads((ROOT / "tests/e2e/fixtures/state-schema-2.json").read_text())
        self.assertTrue(observer.validate_schema_2_state(state))
        for field in ("tree_digest", "manifest_digest"):
            forged = json.loads(json.dumps(state))
            if field == "tree_digest":
                forged["installations"][0]["clients"]["client_a"]["package_revision"][field] = "sha256:" + "0" * 64
            else:
                forged["installations"][0]["clients"]["client_a"]["package_revision"][field] = "sha256:" + "0" * 64
            self.assertFalse(observer.validate_schema_2_state(forged))

    def test_sanitized_schema_two_paths_are_digest_bound_rehomed_and_normalized(self) -> None:
        fixture_path = ROOT / "tests/e2e/fixtures/state-schema-2.json"
        raw = fixture_path.read_bytes()
        legacy = json.loads(raw)
        with tempfile.TemporaryDirectory() as tmp:
            sandbox = Path(tmp)
            home = sandbox / "home"
            home.mkdir()
            transformed, record = observer.transform_sanitized_placeholders(
                legacy,
                original_raw=raw,
                mappings=(("/fixture/home", home),),
            )
            self.assertTrue(observer.validate_placeholder_transformation(legacy, transformed, record, original_raw=raw))
            binding = transformed["installations"][0]["clients"]["client_a"]
            self.assertEqual(binding["target_locator"], str(home / ".codex/plugins/demo"))
            expected = observer.migration_provenance(legacy["installations"][0], legacy=True)
            observed = observer.normalized_migration_provenance(
                transformed["installations"][0], inverse_mappings=((home, "/fixture/home"),), legacy=False,
            )
            self.assertEqual(observed, expected)
            forged = json.loads(json.dumps(record))
            forged["input_digest"] = "sha256:" + "0" * 64
            self.assertFalse(observer.validate_placeholder_transformation(legacy, transformed, forged, original_raw=raw))
            escaped = json.loads(json.dumps(legacy))
            escaped["installations"][0]["clients"]["client_a"]["target_locator"] = "/etc/demo"
            with self.assertRaisesRegex(ValueError, "does not match independently supplied raw bytes"):
                observer.transform_sanitized_placeholders(
                    escaped, original_raw=raw, mappings=(("/fixture/home", home),),
                )

    def test_state_v4_rejects_split_identity_receipt_and_path_authority(self) -> None:
        _, state = self.agentplugins_0_1_14_state_fixture()
        with tempfile.TemporaryDirectory() as tmp:
            manager = Path(tmp)
            state = self.sandbox_state_fixture(state, manager)
            mutations = []
            revision = json.loads(json.dumps(state))
            next(iter(revision["installations"][0]["clients"].values()))["package_revision"]["tree_digest"] = "sha256:" + "0" * 64
            path = manager / "state-v2.json"
            path.write_text(json.dumps(revision))
            self.assertIsNotNone(observer.manager_state(manager), "mixed client revisions are legitimate convergence state")
            duplicate = json.loads(json.dumps(state))
            bindings = list(duplicate["installations"][0]["clients"].values())
            bindings[1]["receipts"][0]["operation_id"] = bindings[0]["receipts"][0]["operation_id"]
            mutations.append(duplicate)
            escaped = json.loads(json.dumps(state))
            binding = next(iter(escaped["installations"][0]["clients"].values()))
            binding["target_locator"] = "/etc/agentplugins-context7"
            binding["native_objects"][0]["path"] = binding["target_locator"]
            binding["receipts"][0]["active_path"] = binding["target_locator"]
            mutations.append(escaped)
            for mutation in mutations:
                path.write_text(json.dumps(mutation))
                self.assertIsNone(observer.manager_state(manager))

    def test_state_v4_rejects_missing_revisions_dangling_data_and_unknown_transactions(self) -> None:
        _, fixture = self.agentplugins_0_1_14_state_fixture()
        with tempfile.TemporaryDirectory() as tmp:
            manager = Path(tmp)
            state = self.sandbox_state_fixture(fixture, manager)
            path = manager / "state-v2.json"
            mutations = []
            missing_revision = json.loads(json.dumps(state))
            next(iter(missing_revision["installations"][0]["clients"].values())).pop("package_revision")
            path.write_text(json.dumps(missing_revision))
            self.assertIsNotNone(observer.manager_state(manager), "released Go permits absent direct-local client revision")
            missing_revision["installations"][0]["origin_mode"] = "directory"
            missing_revision["installations"][0]["directory"] = {
                "product_id": "context7", "distribution_id": "upstash/context7",
                "distribution_kind": "upstream", "desired_release_sequence": 1,
            }
            mutations.append(missing_revision)
            dangling = json.loads(json.dumps(state))
            next(iter(dangling["installations"][0]["clients"].values()))["data_receipt_id"] = "missing-receipt"
            mutations.append(dangling)
            missing_receipts = json.loads(json.dumps(state))
            missing_receipts["installations"][0]["data_receipts"] = {}
            mutations.append(missing_receipts)
            unknown = json.loads(json.dumps(state))
            unknown["transaction_receipts"] = [{
                "operation_id": "unknown", "sequence": 1, "mutation_type": "file_copy",
                "client_binding_id": "missing", "phase": "committed",
            }]
            mutations.append(unknown)
            incomplete = json.loads(json.dumps(state))
            incomplete["transaction_receipts"] = [{
                "operation_id": "swap", "operation_group_id": state["installations"][0]["operation_group_id"],
                "sequence": 2, "mutation_type": "directory_swap",
                "client_binding_id": next(iter(state["installations"][0]["clients"])), "phase": "committed",
            }]
            mutations.append(incomplete)
            for mutation in mutations:
                path.write_text(json.dumps(mutation))
                self.assertIsNone(observer.manager_state(manager))

    def test_state_v4_ports_leaf_origin_snapshot_and_global_source_invariants(self) -> None:
        _, fixture = self.agentplugins_0_1_14_state_fixture()
        with tempfile.TemporaryDirectory() as tmp:
            manager = Path(tmp)
            state = self.sandbox_state_fixture(fixture, manager)
            path = manager / "state-v2.json"
            mutations = []
            missing_origin = json.loads(json.dumps(state)); missing_origin["installations"][0].pop("origin_mode")
            mutations.append(missing_origin)
            invalid_leaf = json.loads(json.dumps(state)); invalid_leaf["installations"][0]["installation_id"] = "../escape"
            mutations.append(invalid_leaf)
            duplicate_source = json.loads(json.dumps(state)); duplicate_source["installations"].append(json.loads(json.dumps(duplicate_source["installations"][0])))
            duplicate_source["installations"][1]["installation_id"] = "second-installation"
            mutations.append(duplicate_source)
            incoherent_snapshot = json.loads(json.dumps(state)); installation = incoherent_snapshot["installations"][0]
            installation["origin_mode"] = "directory"
            installation["directory"] = {
                "product_id": "context7", "distribution_id": "upstash/context7",
                "distribution_kind": "upstream", "desired_release_sequence": 1,
                "snapshot_sequence": 41,
            }
            mutations.append(incoherent_snapshot)
            for mutation in mutations:
                path.write_text(json.dumps(mutation))
                self.assertIsNone(observer.manager_state(manager))

    def test_state_v4_validate_matches_released_go_leaf_retention_and_directory_rules(self) -> None:
        _, fixture = self.agentplugins_0_1_14_state_fixture()
        self.assertTrue(observer.validate_released_state_v4(fixture))

        for container, field in (
            ("package", "format_id"), ("source", "tree_digest"),
            ("package", "manifest_digest"), ("package", "schema_uri"),
        ):
            whitespace_only = json.loads(json.dumps(fixture))
            whitespace_only["installations"][0][container][field] = "\u00a0\t "
            self.assertTrue(
                observer.validate_released_state_v4(whitespace_only),
                f"released Go uses direct != empty for {container}.{field}",
            )
        null_zero_values = json.loads(json.dumps(fixture))
        installation = null_zero_values["installations"][0]
        installation["operation_group_id"] = None
        installation["data_retained"] = None
        binding = next(iter(installation["clients"].values()))
        binding["data_receipt_id"] = None
        binding["receipts"][0]["operation_group_id"] = None
        self.assertTrue(
            observer.validate_released_state_v4(null_zero_values),
            "encoding/json null leaves scalar string/bool fields at their Go zero values",
        )
        null_phase = json.loads(json.dumps(fixture))
        next(iter(null_phase["installations"][0]["clients"].values()))["receipts"][0]["phase"] = None
        self.assertFalse(observer.validate_released_state_v4(null_phase), "null phase is Go's rejected empty string")
        absent_snapshot_nulls = json.loads(json.dumps(fixture))
        current = absent_snapshot_nulls["installations"][0]
        current["origin_mode"] = "directory"
        current["directory"] = {
            "product_id": "context7", "distribution_id": "upstash/context7",
            "distribution_kind": "upstream", "desired_release_sequence": 1,
            "snapshot_schema": None, "snapshot_sequence": None, "snapshot_digest": None,
        }
        current["source"]["resolved_revision"] = "b" * 40
        for client in current["clients"].values():
            client["package_revision"].update({
                "distribution_id": "upstash/context7", "release_sequence": 1,
                "resolved_revision": "a" * 40,
            })
        self.assertTrue(observer.validate_released_state_v4(absent_snapshot_nulls))

        uint_fixture = json.loads(json.dumps(absent_snapshot_nulls))
        directory = uint_fixture["installations"][0]["directory"]
        client = next(iter(uint_fixture["installations"][0]["clients"].values()))
        safe_maximum = 9_007_199_254_740_991
        directory["desired_release_sequence"] = safe_maximum
        directory["snapshot_sequence"] = safe_maximum
        directory["snapshot_schema"] = 1
        directory["snapshot_digest"] = "snapshot"
        client["package_revision"]["release_sequence"] = safe_maximum
        self.assertTrue(observer.validate_released_state_v4(uint_fixture), "safe maximum is representable")
        for field, value in (
            ("desired_release_sequence", 9_007_199_254_740_992),
            ("desired_release_sequence", 9_007_199_254_740_993),
            ("desired_release_sequence", 1 << 64),
            ("desired_release_sequence", -1),
            ("desired_release_sequence", True),
            ("snapshot_sequence", 1 << 64),
            ("snapshot_sequence", 9_007_199_254_740_992),
            ("snapshot_sequence", 9_007_199_254_740_993),
        ):
            invalid = json.loads(json.dumps(uint_fixture)); invalid["installations"][0]["directory"][field] = value
            self.assertFalse(observer.validate_released_state_v4(invalid), (field, value))
        for unsafe in (9_007_199_254_740_992, 9_007_199_254_740_993, 1 << 64):
            overflow_revision = json.loads(json.dumps(uint_fixture))
            next(iter(overflow_revision["installations"][0]["clients"].values()))["package_revision"]["release_sequence"] = unsafe
            self.assertFalse(observer.validate_released_state_v4(overflow_revision))
        catalog_uint = json.loads(json.dumps(fixture))
        catalog_revision = next(iter(catalog_uint["installations"][0]["clients"].values()))["package_revision"]
        catalog_revision["catalog_evidence"] = {
            "current_evidence": [{"release_sequence": safe_maximum}],
            "compatibility": {"cursor": {"evidence": [{"release_sequence": safe_maximum}]}},
        }
        self.assertTrue(observer.validate_released_state_v4(catalog_uint))
        for unsafe in (9_007_199_254_740_992, 9_007_199_254_740_993):
            catalog_revision["catalog_evidence"]["compatibility"]["cursor"]["evidence"][0]["release_sequence"] = unsafe
            self.assertFalse(observer.validate_released_state_v4(catalog_uint))
        null_catalog = json.loads(json.dumps(fixture))
        null_revision = next(iter(null_catalog["installations"][0]["clients"].values()))["package_revision"]
        null_revision["catalog_evidence"] = {
            "current_evidence": [None],
            "compatibility": {"cursor": {"evidence": [None]}},
        }
        self.assertTrue(
            observer.validate_released_state_v4(null_catalog),
            "encoding/json decodes null []struct elements as zero-value structs",
        )
        numeric_phase = json.loads(json.dumps(fixture))
        next(iter(numeric_phase["installations"][0]["clients"].values()))["receipts"][0]["phase"] = 123
        self.assertFalse(observer.validate_released_state_v4(numeric_phase), "Go cannot decode a number into a string field")
        for mutate in (
            lambda value: value["installations"][0]["source"].__setitem__("requested_source", 123),
            lambda value: value["installations"][0].__setitem__("created_at", 123),
            lambda value: next(iter(value["installations"][0]["clients"].values()))["receipts"][0].__setitem__("mutation_type", 123),
            lambda value: value.__setitem__("unknown_state_field", True),
        ):
            invalid = json.loads(json.dumps(fixture)); mutate(invalid)
            self.assertFalse(observer.validate_released_state_v4(invalid), "strict Go decoding must reject wrong or unknown fields")
        null_zero_string = json.loads(json.dumps(fixture))
        null_zero_string["installations"][0]["source"]["requested_source"] = None
        null_zero_string["installations"][0]["package"]["inventory"] = None
        self.assertTrue(observer.validate_released_state_v4(null_zero_string), "JSON null leaves non-pointer Go fields at zero values")
        oversized_go_int = json.loads(json.dumps(absent_snapshot_nulls))
        oversized_go_int["installations"][0]["directory"]["snapshot_schema"] = 1 << 80
        self.assertFalse(observer.validate_released_state_v4(oversized_go_int))

        directory_vector = json.loads(json.dumps(uint_fixture))
        raw = json.dumps(directory_vector, separators=(",", ":"))
        token = str(safe_maximum)
        parsed_max = observer.strict_state_json_loads(raw)
        self.assertTrue(observer.validate_released_state_v4(parsed_max))
        raw_uint64 = raw.replace(token, str((1 << 64) - 1), 1)
        decoded_uint64 = observer.strict_state_json_loads(raw_uint64)
        self.assertTrue(observer._released_state_v4_decodes(decoded_uint64))
        self.assertFalse(observer.validate_released_state_v4(decoded_uint64))
        self.assertFalse(observer.validate_released_state_v4(
            observer.strict_state_json_loads(raw.replace(token, "18446744073709551616", 1)),
        ))
        self.assertFalse(observer.validate_released_state_v4(
            observer.strict_state_json_loads(raw.replace(token, "-0", 1)),
        ))
        for malformed in (
            raw.replace("context7", "\\ud800context7", 1),
            b'{"schema_version":4,"installations":[],"transaction_receipts":["\xff"]}',
        ):
            with self.assertRaises((ValueError, UnicodeDecodeError)):
                observer.strict_state_json_loads(malformed)

        trimmed = json.loads(json.dumps(fixture))
        installation = trimmed["installations"][0]
        installation["installation_id"] = f"  {installation['installation_id']}  "
        binding_key, binding = next(iter(installation["clients"].items()))
        binding["physical_artifact_id"] = f"  {binding['physical_artifact_id']}  "
        binding["receipts"][0]["operation_id"] = f"  {binding['receipts'][0]['operation_id']}  "
        self.assertTrue(observer.validate_released_state_v4(trimmed), "Go validates leaf IDs after TrimSpace")

        for whitespace in ("\t", "\u0085", "\u00a0", "\u1680", "\u2007", "\u202f", "\u3000"):
            spaced = json.loads(json.dumps(fixture))
            spaced["installations"][0]["installation_id"] = f"{whitespace}portable-id{whitespace}"
            self.assertTrue(observer.validate_released_state_v4(spaced), repr(whitespace))
        for control in ("\u001c", "\u001d", "\u001e", "\u001f"):
            control_separator = json.loads(json.dumps(fixture))
            installation = control_separator["installations"][0]
            installation["declared_name"] = control
            installation["package"]["declared_name"] = control
            self.assertTrue(
                observer.validate_released_state_v4(control_separator),
                f"Go strings.TrimSpace does not trim U+{ord(control):04X}",
            )
        for whitespace in ("\t", "\u0085", "\u00a0", "\u1680", "\u2007", "\u202f", "\u3000"):
            blank = json.loads(json.dumps(fixture))
            installation = blank["installations"][0]
            installation["declared_name"] = whitespace
            installation["package"]["declared_name"] = whitespace
            self.assertFalse(observer.validate_released_state_v4(blank), repr(whitespace))
        for control in ("\u001c", "\u001d", "\u001e", "\u001f"):
            self.assertTrue(observer._released_directory_snapshot_coherent({
                "snapshot_schema": 1, "snapshot_sequence": 1, "snapshot_digest": control,
            }))
        for whitespace in ("\t", "\u0085", "\u00a0", "\u1680", "\u2007", "\u202f", "\u3000"):
            self.assertFalse(observer._released_directory_snapshot_coherent({
                "snapshot_schema": 1, "snapshot_sequence": 1, "snapshot_digest": whitespace,
            }))

        for field, replacement in (
            ("physical_artifact_id", "../artifact"),
            ("physical_artifact_id", "CON"),
            ("physical_artifact_id", "artifact..part"),
        ):
            invalid = json.loads(json.dumps(fixture))
            next(iter(invalid["installations"][0]["clients"].values()))[field] = replacement
            self.assertFalse(observer.validate_released_state_v4(invalid))

        retained_with_clients = json.loads(json.dumps(fixture))
        retained_with_clients["installations"][0]["data_retained"] = True
        self.assertFalse(observer.validate_released_state_v4(retained_with_clients))
        retained_without_receipts = json.loads(json.dumps(fixture))
        retained_without_receipts["installations"][0]["clients"] = {}
        retained_without_receipts["installations"][0]["data_receipts"] = {}
        retained_without_receipts["installations"][0]["data_retained"] = True
        self.assertFalse(observer.validate_released_state_v4(retained_without_receipts))
        retained = json.loads(json.dumps(fixture))
        retained["installations"][0]["clients"] = {}
        retained["installations"][0]["data_retained"] = True
        self.assertTrue(observer.validate_released_state_v4(retained))

        direct_revision = json.loads(json.dumps(fixture))
        next(iter(direct_revision["installations"][0]["clients"].values()))["package_revision"]["resolved_revision"] = "not-a-sha"
        self.assertTrue(observer.validate_released_state_v4(direct_revision))
        directory = json.loads(json.dumps(fixture))
        current = directory["installations"][0]
        current["origin_mode"] = "directory"
        current["directory"] = {
            "product_id": "different-product-is-accepted-by-Go-Validate",
            "distribution_id": "upstash/context7", "distribution_kind": "upstream",
            "desired_release_sequence": 2,
        }
        for client in current["clients"].values():
            client["package_revision"]["distribution_id"] = "upstash/context7"
            client["package_revision"]["release_sequence"] = 1
            client["package_revision"]["resolved_revision"] = "a" * 40
        current["source"]["resolved_revision"] = "b" * 40
        self.assertTrue(observer.validate_released_state_v4(directory))
        with tempfile.TemporaryDirectory() as tmp:
            manager = Path(tmp)
            strengthened = self.sandbox_state_fixture(fixture, manager)
            strengthened_installation = strengthened["installations"][0]
            strengthened_installation["origin_mode"] = "directory"
            strengthened_installation["directory"] = json.loads(json.dumps(current["directory"]))
            strengthened_installation["source"]["resolved_revision"] = "b" * 40
            for client in strengthened_installation["clients"].values():
                client["package_revision"]["distribution_id"] = "upstash/context7"
                client["package_revision"]["release_sequence"] = 1
                client["package_revision"]["resolved_revision"] = "a" * 40
            (manager / "state-v2.json").write_text(json.dumps(strengthened))
            self.assertIsNone(
                observer.manager_state(manager),
                "evidence validation binds directory.product_id even though released Go Validate does not",
            )
        current["directory"].update({"snapshot_schema": 1, "snapshot_sequence": 1, "snapshot_digest": "opaque-nonempty"})
        self.assertTrue(observer.validate_released_state_v4(directory), "Go only requires a nonblank snapshot digest")
        current["clients"][binding_key]["package_revision"]["release_sequence"] = 3
        self.assertFalse(observer.validate_released_state_v4(directory))

        nil_maps = json.loads(json.dumps(fixture))
        nil_maps["installations"][0]["clients"] = None
        nil_maps["installations"][0]["data_receipts"] = None
        self.assertTrue(observer.validate_released_state_v4(nil_maps), "JSON null decodes to nil Go maps")

    @unittest.skipUnless(
        Path("/tmp/agentplugins-0.1.14-public").is_file(),
        "exact public agentplugins 0.1.14 decoder is unavailable",
    )
    def test_state_v4_raw_vectors_match_released_go_decoder(self) -> None:
        binary = Path("/tmp/agentplugins-0.1.14-public")
        self.assertEqual(binary.stat().st_size, 11_190_456)
        self.assertEqual(
            hashlib.sha256(binary.read_bytes()).hexdigest(),
            "7313ad045fa2fa5621f9b9d75914d111f5101c4d3e758515022603fcfb57d31e",
        )
        _, fixture = self.agentplugins_0_1_14_state_fixture()

        def cloned() -> dict:
            return json.loads(json.dumps(fixture))

        def revision(value: dict) -> dict:
            return next(iter(value["installations"][0]["clients"].values()))["package_revision"]

        def encoded(mutate) -> bytes:
            value = cloned(); mutate(value)
            return json.dumps(value, separators=(",", ":")).encode()

        maximum = encoded(lambda value: revision(value).__setitem__("release_sequence", (1 << 64) - 1))
        vectors: list[tuple[str, bytes, bool]] = [
            ("uint64-max", maximum, True),
            ("uint64-overflow", encoded(lambda value: revision(value).__setitem__("release_sequence", 1 << 64)), False),
            ("uint64-negative", encoded(lambda value: revision(value).__setitem__("release_sequence", -1)), False),
            ("uint64-negative-zero", maximum.replace(str((1 << 64) - 1).encode(), b"-0", 1), False),
            ("null-struct-elements", encoded(lambda value: revision(value).__setitem__(
                "catalog_evidence", {"current_evidence": [None], "compatibility": {"cursor": {"evidence": [None]}}},
            )), True),
            ("null-string", encoded(lambda value: value["installations"][0]["source"].__setitem__("requested_source", None)), True),
            ("bool-for-string", encoded(lambda value: value["installations"][0]["source"].__setitem__("requested_source", True)), False),
            ("float-for-string", encoded(lambda value: value["installations"][0]["source"].__setitem__("requested_source", 1.5)), False),
            ("ascii-whitespace-trim", encoded(lambda value: value["installations"][0].__setitem__("installation_id", "\tportable-id\t")), True),
            ("nbsp-trim", encoded(lambda value: value["installations"][0].__setitem__("installation_id", "\u00a0portable-id\u00a0")), True),
            ("nbsp-blank", encoded(lambda value: (
                value["installations"][0].__setitem__("declared_name", "\u00a0"),
                value["installations"][0]["package"].__setitem__("declared_name", "\u00a0"),
            )), False),
        ]
        surrogate = encoded(lambda value: value["installations"][0].__setitem__("installation_id", "SURROGATE"))
        vectors.extend([
            ("unpaired-surrogate", surrogate.replace(b"SURROGATE", b"\\ud800portable", 1), False),
            ("invalid-utf8", surrogate.replace(b"SURROGATE", b"\xffportable", 1), False),
        ])

        for name, raw, expected in vectors:
            with self.subTest(vector=name), tempfile.TemporaryDirectory(
                prefix="state-v4-go-oracle-",
            ) as tmp:
                disposable = Path(tmp)
                home = disposable / "home"; manager = disposable / "manager"
                (home / ".cursor" / "plugins" / "local").mkdir(parents=True)
                manager.mkdir(); (manager / "state-v2.json").write_bytes(raw)
                completed = subprocess.run(
                    [str(binary), "info", "context7", "--target", "cursor", "--format", "json"],
                    env={**os.environ, "HOME": str(home), "AGENTPLUGINS_HOME": str(manager)},
                    cwd=disposable, text=True, capture_output=True, check=False, timeout=30,
                )
                go_accepted = completed.returncode == 0
                try:
                    python_accepted = observer.validate_released_state_v4(observer.strict_state_json_loads(raw))
                except (UnicodeError, ValueError, json.JSONDecodeError):
                    python_accepted = False
                self.assertEqual(go_accepted, expected, completed.stderr)
                self.assertEqual(python_accepted, go_accepted, name)

    def test_state_v4_validate_rejects_duplicate_sources_receipt_ids_and_operation_ids(self) -> None:
        _, fixture = self.agentplugins_0_1_14_state_fixture()
        duplicate_source = json.loads(json.dumps(fixture))
        duplicate_source["installations"].append(json.loads(json.dumps(duplicate_source["installations"][0])))
        duplicate_source["installations"][1]["installation_id"] = "different-installation"
        self.assertFalse(observer.validate_released_state_v4(duplicate_source))

        bad_receipt = json.loads(json.dumps(fixture))
        receipt_key, receipt = next(iter(bad_receipt["installations"][0]["data_receipts"].items()))
        receipt["data_receipt_id"] = receipt_key + "-different"
        self.assertFalse(observer.validate_released_state_v4(bad_receipt))

        duplicate_operation = json.loads(json.dumps(fixture))
        clients = list(duplicate_operation["installations"][0]["clients"].values())
        clients[1]["receipts"][0]["operation_id"] = clients[0]["receipts"][0]["operation_id"]
        self.assertFalse(observer.validate_released_state_v4(duplicate_operation))

    def test_state_v4_rejects_cross_owner_ancestor_target_overlap(self) -> None:
        _, fixture = self.agentplugins_0_1_14_state_fixture()
        with tempfile.TemporaryDirectory() as tmp:
            manager = Path(tmp)
            state = self.sandbox_state_fixture(fixture, manager)
            bindings = list(state["installations"][0]["clients"].values())
            ancestor = str(Path(bindings[1]["target_locator"]).parent)
            bindings[0]["target_locator"] = ancestor
            bindings[0]["native_objects"][0]["path"] = ancestor
            bindings[0]["receipts"][0]["active_path"] = ancestor
            (manager / "state-v2.json").write_text(json.dumps(state))
            self.assertIsNone(observer.manager_state(manager))

    def test_materialization_requires_exact_receipt_target_not_product_name_mentions(self) -> None:
        _, fixture = self.agentplugins_0_1_14_state_fixture()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); home = root / "home"; manager = root / "manager"
            home.mkdir(); manager.mkdir()
            state = self.sandbox_state_fixture(fixture, manager)
            installation = state["installations"][0]
            cursor_key = next(key for key, item in installation["clients"].items() if item["client_id"] == "cursor")
            binding = installation["clients"][cursor_key]
            installation["clients"] = {cursor_key: binding}
            unrelated = home / ".cursor/unrelated-context7.txt"
            unrelated.parent.mkdir(parents=True); unrelated.write_text("context7")
            (manager / "state-v2.json").write_text(json.dumps(state))
            self.assertEqual(observer.materialized_product_mentions(home, manager, "context7", ("cursor",))["cursor"], 0)
            target = Path(binding["target_locator"]); target.mkdir(parents=True)
            (target / "plugin.json").write_text(json.dumps({"name": "context7", "version": "1.0.0"}))
            managed = observer.package_identity(target)["tree_digest"]
            binding["native_objects"][0]["managed_digest"] = managed
            binding["receipts"][0]["after_digest"] = managed
            (manager / "state-v2.json").write_text(json.dumps(state))
            self.assertEqual(observer.materialized_product_mentions(home, manager, "context7", ("cursor",))["cursor"], 1)

    def test_direct_origin_cannot_prove_directory_release_provenance(self) -> None:
        _, state = self.agentplugins_0_1_14_state_fixture()
        installation = state["installations"][0]
        release = {
            "product_id": "context7", "distribution_id": "upstash/context7", "distribution_kind": "upstream",
            "release_sequence": 1, "source_repository": "upstash/context7",
            "source_revision": installation["source"]["resolved_revision"],
            "source_path": installation["source"]["package_subpath"],
            "tree_digest": installation["source"]["tree_digest"],
            "manifest_digest": installation["package"]["manifest_digest"],
        }
        context = {"release": release, "snapshot_sequence": 41, "directory_digest": "sha256:" + "9" * 64}
        self.assertFalse(observer.identity_matches_release(observer.installation_identity(installation), context))
        installation["origin_mode"] = "directory"
        installation["directory"] = {
            "product_id": "context7", "distribution_id": "upstash/context7", "distribution_kind": "upstream",
            "desired_release_sequence": 1, "snapshot_schema": 1, "snapshot_sequence": 41,
            "snapshot_digest": context["directory_digest"],
        }
        self.assertTrue(observer.identity_matches_release(observer.installation_identity(installation), context))

    def test_removal_receipt_requires_exact_pre_remove_binding_authority(self) -> None:
        _, fixture = self.agentplugins_0_1_14_state_fixture()
        with tempfile.TemporaryDirectory() as tmp:
            manager = Path(tmp)
            state = self.sandbox_state_fixture(fixture, manager)
            installation = state["installations"][0]
            binding_id, binding = next(iter(installation["clients"].items()))
            authority = observer.removal_authority_from_installation(installation)
            self.assertIsNotNone(authority)
            removals = [{
                "operation_id": f"remove-operation-{index}", "operation_group_id": "remove-group",
                "sequence": 2, "mutation_type": "directory_remove", "client_binding_id": item["client_binding_id"],
                "active_path": item["active_path"],
                "backup_path": str(Path(item["active_path"]).parent / f".agentplugins-remove-backup-{index}"),
                "before_digest": item["before_digest"], "phase": "committed",
            } for index, item in enumerate(authority.values())]
            retained = json.loads(json.dumps(installation)); retained["clients"] = {}; retained["data_retained"] = True
            removed_state = {"schema_version": 4, "installations": [retained], "transaction_receipts": removals}
            path = manager / "state-v2.json"
            path.write_text(json.dumps(removed_state))
            parsed = observer.manager_state(manager, removal_authority=authority)
            self.assertIsNotNone(parsed)
            self.assertTrue(observer.removal_receipts_bind_command(
                {"schema_version": 4, "installations": [installation]}, parsed, authority,
                "remove-group", tuple(item["client_id"] for item in authority.values()), retained,
            ))
            invented = json.loads(json.dumps(removed_state))
            invented["transaction_receipts"][0]["client_id"] = binding["client_id"]
            path.write_text(json.dumps(invented))
            self.assertIsNone(observer.manager_state(manager))
            for field, replacement in (
                ("client_binding_id", "other-binding"),
                ("active_path", str(manager / "other")), ("before_digest", "sha256:" + "0" * 64),
            ):
                forged = json.loads(json.dumps(removed_state))
                forged["transaction_receipts"][0][field] = replacement
                path.write_text(json.dumps(forged))
                forged_state = observer.manager_state(manager)
                self.assertFalse(forged_state is not None and observer.removal_receipts_bind_command(
                    {"schema_version": 4, "installations": [installation]}, forged_state, authority,
                    "remove-group", tuple(item["client_id"] for item in authority.values()), retained,
                ))

    def test_snapshot_revalidates_directory_binding_after_descendants(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); child = root / "child"; child.mkdir(); (child / "same").write_bytes(b"same")
            original_listdir = os.listdir
            calls = 0
            def replacing_listdir(descriptor):
                nonlocal calls
                calls += 1
                if calls == 2:
                    child.rename(root / "old-child")
                    child.mkdir(); (child / "same").write_bytes(b"same")
                return original_listdir(descriptor)
            with mock.patch.object(observer.os, "listdir", side_effect=replacing_listdir):
                with self.assertRaisesRegex(ValueError, "directory (?:binding )?changed"):
                    observer.filesystem_snapshot(root)

    def test_no_mutation_snapshot_detects_identical_atomic_file_and_directory_replacement(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            child = root / "child"
            child.mkdir()
            body = child / "same.txt"
            body.write_bytes(b"identical")
            before = observer.filesystem_snapshot(root)
            replacement = root / "replacement.txt"
            replacement.write_bytes(b"identical")
            replacement.chmod(body.stat().st_mode & 0o777)
            os.replace(replacement, body)
            after_file = observer.filesystem_snapshot(root)
            self.assertEqual(before["child/same.txt"]["digest"], after_file["child/same.txt"]["digest"])
            self.assertNotEqual(before["child/same.txt"]["inode"], after_file["child/same.txt"]["inode"])
            replacement_dir = root / "replacement-child"
            replacement_dir.mkdir()
            (replacement_dir / "same.txt").write_bytes(b"identical")
            old = root / "old-child"
            child.rename(old)
            replacement_dir.rename(child)
            after_directory = observer.filesystem_snapshot(root)
            self.assertEqual(after_file["child"]["digest"], after_directory["child"]["digest"])
            self.assertNotEqual(after_file["child"]["inode"], after_directory["child"]["inode"])

    def test_migration_stdout_without_state_transition_fails(self) -> None:
        fake = f'''#!/usr/bin/python3
import pathlib, sys
fixtures = pathlib.Path({str(AGENTPLUGINS_0_1_14_FIXTURES)!r})
if sys.argv[1] == "add": raise SystemExit(2)
if sys.argv[1] == "migrate-state":
    print((fixtures / ("migrate-dry-run.json" if "--dry-run" in sys.argv else "migrate-apply.json")).read_text(), end="")
else: raise SystemExit(2)
'''
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); binary = root / "agentplugins"; binary.write_text(fake); binary.chmod(0o700)
            workspace = root / "workspace"; workspace.mkdir()
            with mock.patch.dict(os.environ, {"HOME": str(root / "home"), "AGENTPLUGINS_HOME": str(root / "manager")}, clear=False):
                passed, value = observer.migration_scenario(binary, ("codex",), workspace, "challenge")
        self.assertFalse(passed, value)
        self.assertFalse(value["proof"]["migration_applied"])

    def test_migration_scenario_executes_positive_stateful_transition(self) -> None:
        fake = f'''#!/usr/bin/python3
import json, os, pathlib, shutil, sys
manager = pathlib.Path(os.environ["AGENTPLUGINS_HOME"])
state_path = manager / "state-v2.json"
command = sys.argv[1]
if command == "info":
    print("{{}}")
elif command == "add":
    print("migration required", file=sys.stderr)
    raise SystemExit(2)
elif command == "migrate-state" and "--dry-run" in sys.argv:
    print(pathlib.Path({str(AGENTPLUGINS_0_1_14_FIXTURES / "migrate-dry-run.json")!r}).read_text(), end="")
elif command == "migrate-state":
    body = state_path.read_bytes()
    (manager / "state-v2.json.schema2.backup-agentplugins-fixture").write_bytes(body)
    value = json.loads(body)
    value["schema_version"] = 4
    value["installations"][0]["origin_mode"] = "direct"
    value["installations"][0]["package"]["inventory"] = {{"mcp_present": False, "mcp_enabled": False}}
    state_path.write_text(json.dumps(value, sort_keys=True, separators=(",", ":")))
    print(pathlib.Path({str(AGENTPLUGINS_0_1_14_FIXTURES / "migrate-apply.json")!r}).read_text(), end="")
else:
    raise SystemExit(2)
'''
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / "agentplugins"
            binary.write_text(fake)
            binary.chmod(0o700)
            workspace = root / "workspace"
            workspace.mkdir()
            with mock.patch.dict(os.environ, {
                "HOME": str(root / "home"), "AGENTPLUGINS_HOME": str(root / "manager"),
            }, clear=False):
                passed, value = observer.migration_scenario(binary, ("codex",), workspace, "challenge")
        self.assertTrue(passed, value)
        self.assertTrue(all(value["proof"].values()))
        target_commands = [trace["argv"] for trace in value["command_traces"] if "--target" in trace["argv"]]
        self.assertEqual(
            target_commands,
            [
                ["info", "demo", "--target", "codex", "--format", "json"],
                ["add", "demo", "--target", "codex", "--format", "json"],
            ],
        )
        self.assertEqual(
            value["sandbox_transformation"]["algorithm"],
            "agentplugins-sanitized-placeholder-to-sandbox-v1",
        )
        self.assertEqual(value["sandbox_transformation"]["input_digest"], value["sanitized_fixture_digest"])

    def test_plugin_data_stdout_without_state_or_filesystem_mutation_fails(self) -> None:
        fake = '''#!/usr/bin/python3
import json, sys
print(json.dumps({"schema_version": 1, "command": sys.argv[1], "result": "success", "data": {}}))
'''
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); binary = root / "agentplugins"; binary.write_text(fake); binary.chmod(0o700)
            workspace = root / "workspace"; workspace.mkdir()
            with mock.patch.dict(os.environ, {"HOME": str(root / "home"), "AGENTPLUGINS_HOME": str(root / "manager")}, clear=False):
                passed, value = observer.plugin_data_scenario(binary, ("cursor",), workspace, "challenge")
        self.assertFalse(passed, value)
        self.assertIsNone(value["initial_data_receipt"])

    def test_retained_marker_rejects_identical_file_and_directory_replacement(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            locator = root / "owned"
            locator.mkdir()
            marker = observer.create_contained_marker(locator, (root,), "marker", b"proof")
            self.assertIsNotNone(marker)
            self.assertTrue(marker.verify())
            marker.path.unlink()
            marker.path.write_bytes(b"proof")
            marker.path.chmod(0o600)
            self.assertFalse(marker.verify())
            marker.close()

    def test_purge_proof_rejects_rename_and_accepts_exact_unlink(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            locator = root / "owned"
            locator.mkdir()
            marker = observer.create_contained_marker(locator, (root,), "marker", b"proof")
            self.assertIsNotNone(marker)
            marker.path.rename(root / "renamed-marker")
            self.assertFalse(marker.purged((root,)))
            (root / "renamed-marker").unlink()
            self.assertFalse(marker.purged((root,)))
            locator.rmdir()
            self.assertTrue(marker.purged((root,)))
            marker.close()

            locator.mkdir()
            marker = observer.create_contained_marker(locator, (root,), "second", b"proof")
            original = root / "original-owned"
            locator.rename(original)
            locator.mkdir()
            (locator / "second").write_bytes(b"proof")
            (locator / "second").chmod(0o600)
            self.assertFalse(marker.verify())
            marker.close()

    def test_purge_freezes_all_deduplicated_existing_and_absent_authority_paths(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            target = root / "active"; target.mkdir()
            data = root / "data"; data.mkdir()
            staging = root / ".staging"
            backup = root / ".backup"
            historical = root / ".old" / "nested-backup"
            receipt_id = "data-receipt"
            installation = {
                "data_receipts": {receipt_id: {
                    "data_receipt_id": receipt_id, "physical_backend_id": "artifact",
                    "scope": "user", "locator": str(data),
                    "ownership_digest": "sha256:" + "a" * 64, "state": "owned",
                }},
                "clients": {"binding": {
                    "target_locator": str(target),
                    "native_objects": [{"path": str(target)}],
                    "receipts": [{
                        "active_path": str(target), "staging_path": str(staging),
                        "backup_path": str(backup),
                    }],
                }},
            }
            public = [{
                "data_receipt_id": receipt_id, "physical_backend_id": "artifact",
                "scope": "user", "state": "owned",
            }]
            frozen_result = observer.freeze_complete_authority(
                installation, (root,), public, [{"backup_path": str(historical)}],
            )
            if frozen_result is None:
                self.skipTest("kernel ancestry lifetime primitives unavailable (authority failed closed)")
            self.assertIsNotNone(frozen_result)
            frozen, data_paths = frozen_result
            self.assertEqual(set(frozen.records), {str(target), str(data), str(staging), str(backup), str(historical)})
            self.assertEqual(data_paths, {str(data)})
            staging.mkdir()
            self.assertFalse(frozen.absent_and_unlinked(), "an already-absent authorized name must stay absent")
            staging.rmdir()
            target.rmdir(); data.rmdir()
            self.assertFalse(
                frozen.absent_and_unlinked(),
                "create/delete churn must remain latched through final verification",
            )
            frozen.close()

    def test_retained_tree_rejects_all_nested_modify_restore_epochs(self) -> None:
        mutators = {
            "bytes": lambda root: (root / "nested/file").write_bytes(b"changed"),
            "add_delete": lambda root: (root / "transient").write_text("transient"),
            "chmod": lambda root: (root / "nested/file").chmod(0o600),
            "hardlink": lambda root: os.link(root / "nested/file", root / "transient-link"),
            "symlink": lambda root: ((root / "link").unlink(), (root / "link").symlink_to("other")),
        }
        for name, mutate in mutators.items():
            with self.subTest(name=name), tempfile.TemporaryDirectory() as tmp:
                parent = Path(tmp); retained = parent / "retained"; (retained / "nested").mkdir(parents=True)
                file = retained / "nested/file"; file.write_bytes(b"original"); file.chmod(0o640)
                (retained / "link").symlink_to("nested/file")
                frozen = observer.freeze_path_authority(
                    {str(retained)}, (parent,), outcomes={str(retained): "retain"},
                )
                if frozen is None:
                    self.skipTest("kernel retained-tree lifetime primitives unavailable")
                mutate(retained)
                if name == "bytes": file.write_bytes(b"original")
                elif name == "add_delete": (retained / "transient").unlink()
                elif name == "chmod": file.chmod(0o640)
                elif name == "hardlink": (retained / "transient-link").unlink()
                elif name == "symlink":
                    (retained / "link").unlink(); (retained / "link").symlink_to("nested/file")
                self.assertFalse(frozen.expected(), f"{name} restore must not erase the mutation epoch")
                frozen.close()

        if os.geteuid() == 0:
            with tempfile.TemporaryDirectory() as tmp:
                parent = Path(tmp); retained = parent / "retained"; retained.mkdir()
                file = retained / "file"; file.write_text("x")
                original_owner = (file.stat().st_uid, file.stat().st_gid)
                frozen = observer.freeze_path_authority(
                    {str(retained)}, (parent,), outcomes={str(retained): "retain"},
                )
                if frozen is None:
                    self.skipTest("kernel retained-tree lifetime primitives unavailable")
                os.chown(file, *original_owner); os.chown(file, *original_owner)
                self.assertFalse(frozen.expected(), "owner-setting epoch must not erase the mutation proof")
                frozen.close()

        with tempfile.TemporaryDirectory() as tmp:
            parent = Path(tmp); retained = parent / "retained"; retained.mkdir(); file = retained / "file"; file.write_text("x")
            try:
                os.setxattr(file, "user.launch-proof", b"before")
            except OSError as error:
                if error.errno in {errno.ENOTSUP, errno.EPERM, errno.EACCES}:
                    self.skipTest("user xattrs unavailable")
                raise
            frozen = observer.freeze_path_authority({str(retained)}, (parent,), outcomes={str(retained): "retain"})
            if frozen is None:
                self.skipTest("kernel retained-tree lifetime primitives unavailable")
            os.setxattr(file, "user.launch-proof", b"after"); os.setxattr(file, "user.launch-proof", b"before")
            self.assertFalse(frozen.expected(), "xattr restore must not erase the mutation epoch")
            frozen.close()

    def test_expected_deletion_does_not_weaken_retained_sibling_watch(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            parent = Path(tmp); retained = parent / "retained"; retained.mkdir(); (retained / "file").write_text("safe")
            deleted = parent / "deleted"; deleted.mkdir()
            outcomes = {str(retained): "retain", str(deleted): "delete"}
            frozen = observer.freeze_path_authority(set(outcomes), (parent,), outcomes=outcomes)
            if frozen is None:
                self.skipTest("kernel authority lifetime primitives unavailable")
            deleted.rmdir()
            self.assertTrue(frozen.expected(), "one sibling's exact deletion is legitimate")
            frozen.close()
        with tempfile.TemporaryDirectory() as tmp:
            parent = Path(tmp); retained = parent / "retained"; retained.mkdir(); file = retained / "file"; file.write_text("safe")
            deleted = parent / "deleted"; deleted.mkdir()
            outcomes = {str(retained): "retain", str(deleted): "delete"}
            frozen = observer.freeze_path_authority(set(outcomes), (parent,), outcomes=outcomes)
            if frozen is None:
                self.skipTest("kernel authority lifetime primitives unavailable")
            deleted.rmdir(); file.write_text("changed"); file.write_text("safe")
            self.assertFalse(frozen.expected(), "shared wd masks must preserve retained sibling interest")
            frozen.close()

    def test_released_style_state_save_is_an_expected_new_inode_replacement(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            manager = Path(tmp) / "manager"; manager.mkdir(mode=0o700)
            state = manager / "state-v2.json"; state.write_text('{"schema_version":4,"installations":[{}]}')
            original_inode = state.stat().st_ino
            frozen = observer.freeze_path_authority(
                {str(state)}, (manager,), outcomes={str(state): "replace"},
            )
            if frozen is None:
                self.skipTest("kernel authority lifetime primitives unavailable")
            os.chmod(manager, 0o700)
            temp = manager / "state-v2.json.tmp-fixed"
            descriptor = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            try:
                os.write(descriptor, b'{"schema_version":4,"installations":[]}')
                os.fchmod(descriptor, 0o600); os.fsync(descriptor)
            finally:
                os.close(descriptor)
            temp.replace(state)
            self.assertNotEqual(state.stat().st_ino, original_inode)
            self.assertEqual(frozen.replacement_json(str(state)), {"schema_version": 4, "installations": []})
            self.assertTrue(frozen.expected())
            frozen.close()

    @unittest.skipUnless(sys.platform.startswith("linux"), "Landlock/inotify proof is Linux-only")
    def test_retained_rename_symlink_outside_delete_restore_is_denied_and_fails_evidence(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp); workspace = base / "workspace"; workspace.mkdir()
            manager = base / "manager"; manager.mkdir(); retained = manager / "retained"; retained.mkdir()
            home = base / "home"; home.mkdir(); outside = base / "outside"; outside.mkdir(); victim = outside / "victim"; victim.write_text("survives")
            frozen = observer.freeze_path_authority({str(retained)}, (manager,), outcomes={str(retained): "retain"})
            if frozen is None:
                self.skipTest("kernel authority lifetime primitives unavailable")
            attack = workspace / "attack"
            attack.write_text('''#!/usr/bin/python3
import os, pathlib
retained = pathlib.Path(os.environ["RETAINED"]); saved = retained.with_name("saved-retained")
retained.rename(saved)
try:
    try:
        retained.symlink_to(os.environ["OUTSIDE"], target_is_directory=True)
        try: (retained / "victim").unlink()
        except PermissionError: pass
    except PermissionError: pass
    if retained.is_symlink(): retained.unlink()
finally: saved.rename(retained)
''')
            attack.chmod(0o700)
            environment = {"HOME": str(home), "AGENTPLUGINS_HOME": str(manager), "RETAINED": str(retained), "OUTSIDE": str(outside)}
            write_authority = frozen.write_root_descriptors({str(retained)})
            self.assertIsNotNone(write_authority)
            with mock.patch.dict(os.environ, environment, clear=False):
                completed, _ = observer.traced(
                    attack, [], workspace, "challenge", write_authority=write_authority,
                )
            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertEqual(victim.read_text(), "survives", "Landlock must deny the outside unlink")
            self.assertFalse(frozen.expected(), "restoring the original retained name cannot erase churn")
            frozen.close()

    @unittest.skipUnless(sys.platform.startswith("linux"), "Landlock/inotify proof is Linux-only")
    def test_released_style_binding_backup_rename_allows_exact_parent_and_denies_sibling_roots(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp); workspace = base / "workspace"; workspace.mkdir()
            home = base / "home"; home.mkdir(); cursor_plugins = home / ".cursor" / "plugins"
            binding_parent = cursor_plugins / "local"; binding_parent.mkdir(parents=True)
            sibling_root = cursor_plugins / "sibling"; sibling_root.mkdir()
            outside = base / "outside"; outside.mkdir()
            manager = base / "manager"; manager.mkdir()
            data = manager / "plugin-data" / "owned-data"; data.mkdir(parents=True)
            active = binding_parent / "e2e-external-package-412e6dee0097"; active.mkdir()
            (active / "payload").write_text("managed")
            frozen = observer.freeze_path_authority(
                {str(active)}, (binding_parent,), outcomes={str(active): "delete"},
            )
            if frozen is None:
                self.skipTest("kernel authority lifetime primitives unavailable")
            write_authority = frozen.write_root_descriptors({str(active)})
            self.assertIsNotNone(write_authority)
            helper = workspace / "released-remove"
            helper.write_text('''#!/usr/bin/python3
import json, os, pathlib, shutil
active = pathlib.Path(os.environ["ACTIVE"])
backup = active.with_name(".agentplugins-backup-e6b2f87e0447d70f")
active.rename(backup)
shutil.rmtree(backup)
denied = []
for name in ("SIBLING", "OUTSIDE"):
    try:
        (pathlib.Path(os.environ[name]) / "forbidden").write_text("no")
        denied.append(False)
    except PermissionError:
        denied.append(True)
print(json.dumps(denied))
''')
            helper.chmod(0o700)
            environment = {
                "HOME": str(home), "AGENTPLUGINS_HOME": str(manager), "ACTIVE": str(active),
                "SIBLING": str(sibling_root), "OUTSIDE": str(outside),
            }
            with mock.patch.dict(os.environ, environment, clear=False):
                installation = {
                    "data_receipts": {"receipt": {"locator": str(data)}},
                    "clients": {"binding": {
                        "client_id": "cursor", "target_locator": str(active),
                    }},
                }
                self.assertEqual(
                    observer.released_operation_authority_roots(installation, manager),
                    (manager.resolve(), binding_parent.resolve()),
                )
                escaped = json.loads(json.dumps(installation))
                escaped["clients"]["binding"]["target_locator"] = str(sibling_root / active.name)
                self.assertIsNone(observer.released_operation_authority_roots(escaped, manager))
                escaped = json.loads(json.dumps(installation))
                escaped["data_receipts"]["receipt"]["locator"] = str(outside / "owned-data")
                self.assertIsNone(observer.released_operation_authority_roots(escaped, manager))
                completed, _ = observer.traced(
                    helper, [], workspace, "challenge", write_authority=write_authority,
                )
            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertEqual(json.loads(completed.stdout), [True, True])
            self.assertFalse((sibling_root / "forbidden").exists())
            self.assertFalse((outside / "forbidden").exists())
            self.assertTrue(frozen.expected(), "the exact released backup rename/delete must be admitted")
            frozen.close()

    @unittest.skipUnless(sys.platform.startswith("linux"), "Landlock proof is Linux-only")
    def test_landlock_abi3_denies_cross_root_refer_and_unrelated_truncate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp); workspace = base / "workspace"; workspace.mkdir()
            home = base / "home"; home.mkdir()
            manager = base / "manager"; manager.mkdir()
            data = manager / "plugin-data" / "owned-data"; data.mkdir(parents=True)
            binding_parent = home / ".cursor" / "plugins" / "local"; binding_parent.mkdir(parents=True)
            active = binding_parent / "e2e-external-package-active"; active.mkdir()
            source = manager / "manager-source"; source.write_text("manager")
            unrelated = home / "unrelated"; unrelated.write_text("untouched")
            helper = workspace / "authority-probe"
            helper.write_text('''#!/usr/bin/python3
import errno, json, os, pathlib
active = pathlib.Path(os.environ["ACTIVE"]); backup = active.with_name("backup")
active.rename(backup); backup.rename(active)
source = pathlib.Path(os.environ["SOURCE"]); binding = active.parent
denied = []
for operation in (
    lambda: source.rename(binding / "cross-root-rename"),
    lambda: os.link(source, binding / "cross-root-link"),
    lambda: os.truncate(os.environ["UNRELATED"], 0),
    lambda: pathlib.Path(os.environ["HOME"]).joinpath("unrelated-write").write_text("no"),
    lambda: source.with_name("forbidden-symlink").symlink_to(source),
    lambda: os.mkfifo(source.with_name("forbidden-fifo")),
):
    try: operation(); denied.append(False)
    except OSError as error: denied.append(error.errno in (errno.EPERM, errno.EXDEV, errno.EACCES))
os.truncate(source, 1)
print(json.dumps({"same_parent": active.is_dir(), "denied": denied, "source": source.read_text()}))
''')
            helper.chmod(0o700)
            installation = {
                "data_receipts": {"receipt": {"locator": str(data)}},
                "clients": {"binding": {"client_id": "cursor", "target_locator": str(active)}},
            }
            environment = {
                "HOME": str(home), "AGENTPLUGINS_HOME": str(manager), "ACTIVE": str(active),
                "SOURCE": str(source), "UNRELATED": str(unrelated),
            }
            with mock.patch.dict(os.environ, environment, clear=False):
                authority = observer.released_lifecycle_write_authority(installation, manager)
                self.assertIsNotNone(authority)
                completed, _ = observer.traced(helper, [], workspace, "challenge", write_authority=authority)
            self.assertEqual(completed.returncode, 0, completed.stderr)
            result = json.loads(completed.stdout)
            self.assertTrue(result["same_parent"])
            self.assertEqual(result["denied"], [True, True, True, True, True, True])
            self.assertEqual(result["source"], "m")
            self.assertEqual(unrelated.read_text(), "untouched")
            self.assertFalse((binding_parent / "cross-root-rename").exists())
            self.assertFalse((binding_parent / "cross-root-link").exists())
            self.assertFalse((manager / "forbidden-symlink").exists())
            self.assertFalse((manager / "forbidden-fifo").exists())

    def test_lifecycle_authority_rejects_ancestor_swap_during_descriptor_freeze(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp); home = base / "home"; manager = base / "manager"
            binding_parent = home / ".cursor" / "plugins" / "local"; binding_parent.mkdir(parents=True)
            data = manager / "plugin-data" / "owned-data"; data.mkdir(parents=True)
            active = binding_parent / "e2e-external-package-active"; active.mkdir()
            installation = {
                "data_receipts": {"receipt": {"locator": str(data)}},
                "clients": {"binding": {"client_id": "cursor", "target_locator": str(active)}},
            }
            original_open = observer.os.open
            saved = base / "saved-manager"
            swapped = False

            def swap_before_final_open(path, flags, *args, **kwargs):
                nonlocal swapped
                if path == "manager" and kwargs.get("dir_fd") is not None and not swapped:
                    swapped = True
                    manager.rename(saved); manager.mkdir()
                return original_open(path, flags, *args, **kwargs)

            environment = {"HOME": str(home), "AGENTPLUGINS_HOME": str(manager)}
            try:
                with mock.patch.dict(os.environ, environment, clear=False), mock.patch.object(
                    observer.os, "open", side_effect=swap_before_final_open,
                ):
                    self.assertIsNone(observer.released_lifecycle_write_authority(installation, manager))
                self.assertTrue(swapped)
            finally:
                if saved.exists():
                    if manager.exists(): manager.rmdir()
                    saved.rename(manager)

    def test_absent_purge_authority_rejects_lexical_ancestor_replacement(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            parent = Path(tmp)
            root = parent / "root"; root.mkdir()
            path = root / ".old" / "nested"
            frozen = observer.freeze_path_authority({str(path)}, (root,))
            if frozen is None:
                self.skipTest("kernel ancestry lifetime primitives unavailable (authority failed closed)")
            self.assertIsNotNone(frozen)
            original = parent / "original-root"
            root.rename(original)
            path.mkdir(parents=True)
            shutil.rmtree(original)
            self.assertFalse(
                frozen.absent_and_unlinked(),
                "a held ancestor descriptor cannot authenticate a replacement lexical root",
            )
            frozen.close()

    def test_absent_purge_authority_rejects_rename_use_delete_and_restore(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            parent = Path(tmp)
            root = parent / "root"; root.mkdir()
            path = root / ".old" / "nested"
            frozen = observer.freeze_path_authority({str(path)}, (root,))
            if frozen is None:
                self.skipTest("kernel ancestry lifetime primitives unavailable (authority failed closed)")
            original = parent / "original-root"
            root.rename(original)
            replacement = parent / "root"; replacement.mkdir()
            (replacement / ".old").mkdir()
            shutil.rmtree(replacement)
            original.rename(root)
            self.assertFalse(frozen.absent_and_unlinked(), "restoring the original inode must not erase entry churn")
            frozen.close()

    def test_absent_purge_authority_fails_closed_on_overflow_and_capability_loss(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "root"; root.mkdir()
            path = root / "missing" / "nested"
            with mock.patch.object(observer._LinuxAncestryLifetimeProof, "__init__", side_effect=OSError("unavailable")):
                self.assertIsNone(observer.freeze_path_authority({str(path)}, (root,)))
            proof = object.__new__(observer._LinuxAncestryLifetimeProof)
            proof._failed = False
            proof._names = {}
            proof._record_event(-1, observer._IN_Q_OVERFLOW, 0, b"")
            self.assertTrue(proof._failed, "queue overflow must be permanently latched")

    def test_authority_initialization_barrier_rejects_queued_setup_churn(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "root"; root.mkdir()
            missing = root / "missing"
            original = observer._LinuxAncestryLifetimeProof.initialize

            def churn_then_initialize(proof: object) -> bool:
                missing.mkdir()
                return original(proof)

            with mock.patch.object(
                observer._LinuxAncestryLifetimeProof, "initialize", autospec=True,
                side_effect=churn_then_initialize,
            ):
                self.assertIsNone(
                    observer.freeze_path_authority({str(missing)}, (root,)),
                    "setup churn queued after immutable interest must fail the freeze barrier",
                )

    def test_existing_final_leaf_has_explicit_delete_outcome_interest(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "root"; root.mkdir()
            leaf = root / "delete-me"; leaf.mkdir()
            frozen = observer.freeze_path_authority({str(leaf)}, (root,))
            if frozen is None:
                self.skipTest("kernel ancestry lifetime primitives unavailable (authority failed closed)")
            self.assertIn(os.fsencode(leaf.name), set().union(*frozen.lifetime._names.values()))
            leaf.rmdir()
            self.assertTrue(
                frozen.absent_and_unlinked(),
                "one exact unlink is the admitted deletion transition",
            )
            frozen.close()

    def test_inotify_stream_parser_fails_closed_on_noncanonical_records(self) -> None:
        def event(wd: int, mask: int, *, cookie: int = 0, name_buffer: bytes = b"") -> bytes:
            return b"".join((
                wd.to_bytes(4, sys.byteorder, signed=True),
                mask.to_bytes(4, sys.byteorder), cookie.to_bytes(4, sys.byteorder),
                len(name_buffer).to_bytes(4, sys.byteorder), name_buffer,
            ))

        def name(value: bytes) -> bytes:
            return value + b"\0" * (16 - len(value) % 16)

        def parses(body: bytes, names: dict[int, set[bytes]] | None = None) -> bool:
            proof = object.__new__(observer._LinuxAncestryLifetimeProof)
            proof._failed = False
            proof._names = names if names is not None else {7: {b"watched"}}
            proof._tree_wds = set()
            proof._journal = []
            proof._outcomes = {(7, b"watched"): "retain"}
            proof._masks = {
                wd: observer._INOTIFY_WATCH_MASK if interested else observer._INOTIFY_SELF_MASK
                for wd, interested in proof._names.items()
            }
            proof._parse_stream(body)
            return not proof._failed

        canonical_irrelevant = event(7, 0x00000100, name_buffer=name(b"other"))
        self.assertTrue(parses(canonical_irrelevant))
        self.assertTrue(parses(event(7, 0x00000004)), "watched-directory IN_ATTRIB is canonical")
        self.assertFalse(
            parses(canonical_irrelevant, {7: set()}),
            "a self-only final-leaf watch must reject an event it did not install",
        )
        proof = object.__new__(observer._LinuxAncestryLifetimeProof)
        proof._failed = False; proof._names = {7: {b"watched"}}; proof._tree_wds = set()
        proof._journal = []; proof._outcomes = {(7, b"watched"): "retain"}
        proof._masks = {7: observer._INOTIFY_WATCH_MASK}
        proof._parse_stream(event(7, observer._IN_CREATE, name_buffer=name(b"watched")))
        self.assertFalse(proof.validate_journal(), "retained-name churn is latched for final evaluation")
        malformed = (
            event(99, 0x00000100, name_buffer=name(b"name")),
            event(7, 0x10000000, name_buffer=name(b"name")),
            event(7, observer._IN_IGNORED),
            event(-1, observer._IN_Q_OVERFLOW),
            b"",
            b"short",
            event(7, 0x00000100, name_buffer=b"name"),
            event(7, 0x00000100, name_buffer=b"x\0y\0"),
            event(7, 0x00000100, name_buffer=b"abc"),
            event(7, 0x00000100, name_buffer=b"x\0" + b"\0" * 258),
            event(7, observer._IN_DELETE_SELF, name_buffer=name(b"x")),
            event(7, 0x00000040, cookie=0, name_buffer=name(b"x")),
            event(7, 0x00000008),
            event(7, 0x00000100, name_buffer=name(b".")),
            event(7, 0x00000100, name_buffer=name(b"a/b")),
        )
        for body in malformed:
            self.assertFalse(parses(body), body[:32])
        truncated = event(7, 0x00000100, name_buffer=name(b"name"))[:-1]
        self.assertFalse(parses(truncated))
        for read_result in (b"", b"short", OSError(errno.EBADF, "lost inotify fd")):
            proof = object.__new__(observer._LinuxAncestryLifetimeProof)
            proof.fd = 123
            proof._failed = False
            with mock.patch.object(
                observer.os, "read",
                side_effect=read_result if isinstance(read_result, OSError) else None,
                return_value=None if isinstance(read_result, OSError) else read_result,
            ):
                self.assertFalse(proof._drain(), repr(read_result))

    def test_final_drain_catches_rename_temp_use_delete_restore_during_checks(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            parent = Path(tmp)
            root = parent / "root"; root.mkdir()
            path = root / "missing" / "nested"
            frozen = observer.freeze_path_authority({str(path)}, (root,))
            if frozen is None:
                self.skipTest("kernel ancestry lifetime primitives unavailable (authority failed closed)")
            original_check = frozen.lifetime.check_mount_bindings

            def attack_after_checks() -> bool:
                checked = original_check()
                original = parent / "original-root"
                root.rename(original)
                replacement = parent / "root"; replacement.mkdir()
                (replacement / "missing").mkdir()
                shutil.rmtree(replacement)
                original.rename(root)
                return checked

            with mock.patch.object(frozen.lifetime, "check_mount_bindings", side_effect=attack_after_checks):
                self.assertFalse(
                    frozen.absent_and_unlinked(),
                    "the final drain must catch churn after precheck and all state checks",
                )
            frozen.close()

    @unittest.skipUnless(sys.platform.startswith("linux"), "seccomp is Linux-only and fails closed elsewhere")
    def test_path_authority_seccomp_denies_every_descendant_and_namespace_escape(self) -> None:
        machine = os.uname().machine.lower()
        if machine in {"amd64", "x86_64"}:
            clone_number = 56
        elif machine in {"arm64", "aarch64"}:
            clone_number = 220
        else:
            with self.assertRaises(OSError):
                observer._install_path_authority_seccomp()
            return
        code = f'''import ctypes, json, os, time
libc = ctypes.CDLL(None, use_errno=True)
status = open("/proc/self/status").read()
def call(number, *arguments):
    ctypes.set_errno(0)
    result = libc.syscall(number, *arguments)
    return [result, ctypes.get_errno()]
machine = os.uname().machine.lower()
numbers = {{"x86_64": [272, 308, 165, 166, 155, 161], "aarch64": [97, 268, 40, 39, 41, 51]}}
key = "aarch64" if machine in ("arm64", "aarch64") else "x86_64"
denied = [call(number, 0, 0, 0, 0, 0, 0) for number in numbers[key] + [428, 429, 430, 431, 432, 433, 435, 442]]
namespace_clone = call({clone_number}, 0x00020000 | 17, 0, 0, 0, 0)
ordinary_clone = call({clone_number}, 17, 0, 0, 0, 0)
thread_clone = call({clone_number}, 0x00010000 | 17, 0, 0, 0, 0)
forks = [call(number) for number in ({'57, 58' if machine in {'amd64', 'x86_64'} else ''})]
if ordinary_clone[0] == 0:
    os.setsid(); time.sleep(0.1); open(os.environ["DETACHED_TARGET"], "w").write("escaped"); os._exit(0)
print(json.dumps({{"no_new_privs": "NoNewPrivs:\\t1" in status, "seccomp": "Seccomp:\\t2" in status, "denied": denied, "namespace_clone": namespace_clone, "ordinary_clone": ordinary_clone, "thread_clone": thread_clone, "forks": forks}}))
'''
        with tempfile.TemporaryDirectory() as tmp, mock.patch.dict(
            os.environ, {"DETACHED_TARGET": str(Path(tmp) / "detached-mutation")}, clear=False,
        ):
            completed = subprocess.run(
                ["/usr/bin/python3", "-c", code], text=True, capture_output=True,
                check=False, preexec_fn=observer._install_path_authority_seccomp,
            )
            time.sleep(0.2)
            self.assertFalse((Path(tmp) / "detached-mutation").exists())
        self.assertEqual(completed.returncode, 0, completed.stderr)
        result = json.loads(completed.stdout)
        self.assertTrue(result["no_new_privs"])
        self.assertTrue(result["seccomp"])
        self.assertTrue(all(item == [-1, 1] for item in result["denied"]), result)
        self.assertEqual(result["namespace_clone"], [-1, 1])
        self.assertEqual(result["ordinary_clone"], [-1, 1])
        self.assertTrue(all(item == [-1, 1] for item in result["forks"]), result)
        self.assertEqual(result["thread_clone"][0], -1)
        self.assertNotEqual(result["thread_clone"][1], errno.EPERM, "CLONE_THREAD reached the kernel")

    def test_path_authority_seccomp_has_no_unconstrained_architecture_fallback(self) -> None:
        with mock.patch.object(observer.platform, "machine", return_value="unsupported-test-arch"):
            with self.assertRaisesRegex(OSError, "does not admit"):
                observer._install_path_authority_seccomp()

    def test_absent_purge_authority_binds_linux_mount_identity(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "root"; root.mkdir()
            mountpoint = root / "mountpoint"; mountpoint.mkdir()
            path = mountpoint / "missing"
            frozen = observer.freeze_path_authority({str(path)}, (root,))
            if frozen is None:
                with mock.patch.object(observer, "_statx_mount_id", side_effect=OSError("mount identity unavailable")):
                    self.assertIsNone(observer.freeze_path_authority({str(path)}, (root,)))
                return
            mount = getattr(observer._LIBC, "mount", None)
            umount2 = getattr(observer._LIBC, "umount2", None)
            mounted = bool(
                mount is not None and umount2 is not None
                and mount(os.fsencode(mountpoint), os.fsencode(mountpoint), None, 4096, None) == 0
            )
            if mounted:
                try:
                    opened = os.fstat(frozen.records[str(path)][4][-1][0])
                    replacement = mountpoint.stat()
                    self.assertEqual((opened.st_dev, opened.st_ino), (replacement.st_dev, replacement.st_ino))
                    self.assertFalse(frozen.absent_and_unlinked(), "same-inode bind mount must have a distinct mount identity")
                finally:
                    unmounted = umount2(os.fsencode(mountpoint), 0)
                    if unmounted != 0:
                        unmounted = umount2(os.fsencode(mountpoint), 2)
                    self.assertEqual(unmounted, 0, "disposable bind mount must be removed")
            else:
                frozen.close()
                with mock.patch.object(observer, "_statx_mount_id", side_effect=OSError("mount identity unavailable")):
                    self.assertIsNone(observer.freeze_path_authority({str(path)}, (root,)))
                return
            frozen.close()

    def test_state_migration_observes_stale_refusal_backup_and_exact_provenance(self) -> None:
        legacy = json.loads((ROOT / "tests/e2e/fixtures/state-schema-2.json").read_text())
        self.assertEqual((legacy["schema_version"], legacy["installations"][0]["declared_name"]), (2, "demo"))
        self.assertTrue(observer.validate_cli_envelope(release_fixture("migrate-dry-run.json"), "migrate-state"))
        self.assertTrue(observer.validate_cli_envelope(release_fixture("migrate-apply.json"), "migrate-state"))
    def test_migration_rejects_unrelated_mutation_and_bool_or_float_schema_four(self) -> None:
        applied = release_fixture("migrate-apply.json")
        for schema in (True, 4.0, 4):
            mutation = {**applied, "data": {**applied["data"], "source_schema": schema}}
            self.assertFalse(observer.validate_cli_envelope(mutation, "migrate-state"))
    def test_migration_provenance_fails_closed_on_missing_or_changed_identity(self) -> None:
        state = json.loads((ROOT / "tests/e2e/fixtures/state-schema-2.json").read_text())
        record = observer.installation_record(state, "demo")
        expected = observer.migration_provenance(record, legacy=True)
        self.assertIsNotNone(expected)
        missing = json.loads(json.dumps(record))
        del missing["source"]["repository"]
        self.assertIsNone(observer.migration_provenance(missing, legacy=True))
        changed = json.loads(json.dumps(record))
        changed["source"]["resolved_revision"] = "2" * 40
        self.assertNotEqual(expected, observer.migration_provenance(changed, legacy=True))
        self.assertIsNone(observer._one_semantic(
            {"input_digest": "sha256:" + "a" * 64, "state_digest": "sha256:" + "b" * 64},
            {"input_digest", "state_digest"},
        ))

    def test_plugin_data_update_requires_changed_package_and_preserves_exact_receipt(self) -> None:
        fake = f'''#!/usr/bin/python3
import json, os, pathlib, shutil, sys, time
manager = pathlib.Path(os.environ["AGENTPLUGINS_HOME"]); manager.mkdir(parents=True, exist_ok=True)
state_path = manager / "state-v2.json"; command = sys.argv[1]
def attempt_detached_descendant():
    if command != "add" or not os.environ.get("DETACHED_PROOF"):
        return
    try:
        child = os.fork()
    except PermissionError:
        pathlib.Path(os.environ["DETACHED_PROOF"]).write_text("denied")
        return
    if child == 0:
        os.setsid(); os.close(1); os.close(2); time.sleep(0.3)
        pathlib.Path(os.environ["DETACHED_PROOF"]).write_text("escaped")
        os._exit(0)
def malicious_lifetime_epoch():
    requested = os.environ.get("ATTACK_COMMAND")
    if requested != command and not (
        requested == "remove-retain" and command == "remove" and "--purge-data" not in sys.argv
    ):
        return
    retained = manager / "plugin-data/e2e-external-package-owned"
    marker = retained / "launch-marker.txt"
    mode = os.environ.get("ATTACK_MODE")
    if mode == "modify-restore":
        original = marker.read_bytes(); marker.write_bytes(b"changed"); marker.write_bytes(original)
    elif mode == "permanent-addition":
        (retained / "unrelated-attacker-content").write_text("persist")
    elif mode == "rename-symlink":
        saved = retained.with_name("saved-retained")
        retained.rename(saved)
        try:
            try:
                retained.symlink_to(os.environ["OUTSIDE"], target_is_directory=True)
                (retained / "victim").write_text("mutated")
            except PermissionError:
                pass
            if retained.is_symlink(): retained.unlink()
        finally:
            saved.rename(retained)
def atomic_state(state):
    os.chmod(manager, 0o700)
    atomic_state.epoch += 1
    temp = state_path.with_name(state_path.name + ".tmp-" + str(atomic_state.epoch))
    descriptor = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        os.write(descriptor, json.dumps(state, sort_keys=True).encode()); os.fchmod(descriptor, 0o600); os.fsync(descriptor)
    finally: os.close(descriptor)
    temp.replace(state_path)
atomic_state.epoch = 0
def write_state(package):
    manifest = json.loads((package / "plugin.json").read_text())
    identity = ({{"tree_digest": "sha256:c350f133f09cbf95f3ddb05ffad6d1064d8478f9046d19caaaefe60d8a2707bb", "manifest_digest": "sha256:5e07ebffdfc936c72db176327fa360d9dc825fcbe239259f1b3ca92d1ac9ff5e"}}
      if manifest.get("version") == "1.0.0" else
      {{"tree_digest": "sha256:342346b9df16ce5649862255326ac8c7ed56fe578c2f9d7b41d7d3f0e25c2bf9", "manifest_digest": "sha256:82224daacc9b4af90c77494728efbd632363d1501ac31c40da56e5e9edd51356"}})
    data = manager / "plugin-data/e2e-external-package-owned"; data.mkdir(parents=True, exist_ok=True)
    target = pathlib.Path(os.environ["HOME"]) / ".cursor/plugins/local/e2e-external-package-released"
    target.mkdir(parents=True, exist_ok=True)
    receipt_id = "plugin-data-receipt"; binding_id = "plugin-data-cursor"
    state = {{"schema_version": 4, "installations": [{{"installation_id": "plugin-data-installation",
      "declared_name": "e2e-external-package", "origin_mode": "direct", "source": {{"source_binding_id": "local-source",
      "requested_source": "./" + package.name, "canonical_source": "direct local source", "resolved_revision": "",
      "tree_digest": identity["tree_digest"]}}, "package": {{"loader_kind": "agent_plugins",
      "format_id": "agent-plugins/1.0.0", "schema_uri": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
      "declared_name": "e2e-external-package", "version": manifest.get("version", "1.0.0"),
      "manifest_digest": identity["manifest_digest"], "inventory": {{"mcp_present": False, "mcp_enabled": False}}}},
      "data_receipts": {{receipt_id: {{"data_receipt_id": receipt_id, "physical_backend_id": "owned-artifact",
      "scope": "user", "locator": str(data), "ownership_digest": "sha256:" + "3" * 64, "state": "owned"}}}},
      "clients": {{binding_id: {{"client_binding_id": binding_id, "client_id": "cursor", "scope": "user",
      "target_locator": str(target), "physical_artifact_id": "owned-artifact", "data_receipt_id": receipt_id,
      "materialization": "materialized", "activation": "manual_activation_required", "authentication": "not_checked",
      "policy": "allowed", "verification": "package_validated", "package_revision": {{"version": manifest.get("version", "1.0.0"),
      "tree_digest": identity["tree_digest"], "manifest_digest": identity["manifest_digest"]}},
      "updated_at": "2026-08-24T00:00:00Z"}}}}, "created_at": "2026-08-24T00:00:00Z", "updated_at": "2026-08-24T00:00:00Z"}}]}}
    atomic_state(state)
if command == "add":
    attempt_detached_descendant(); write_state(pathlib.Path.cwd() / sys.argv[2].removeprefix("./"))
elif command == "update":
    malicious_lifetime_epoch()
    state = json.loads(state_path.read_text()); write_state(pathlib.Path.cwd() / state["installations"][0]["source"]["requested_source"].removeprefix("./"))
elif command == "repair":
    malicious_lifetime_epoch()
    state = json.loads(state_path.read_text())
    for binding in state["installations"][0]["clients"].values(): pathlib.Path(binding["target_locator"]).mkdir(parents=True, exist_ok=True)
elif command == "switch": malicious_lifetime_epoch()
elif command == "info": malicious_lifetime_epoch()
elif command == "remove" and "--purge-data" not in sys.argv:
    malicious_lifetime_epoch()
    state = json.loads(state_path.read_text()); installation = state["installations"][0]
    for binding in installation["clients"].values():
        active = pathlib.Path(binding["target_locator"])
        backup = active.with_name(".agentplugins-backup-e6b2f87e0447d70f")
        active.rename(backup); shutil.rmtree(backup)
    receipt = next(iter(installation["data_receipts"].values()))
    installation["clients"] = {{}}; installation["data_retained"] = True
    os.chmod(manager, 0o700)
    atomic_state(state)
    atomic_state(state)
    print(json.dumps({{"schema_version": 1, "command": "remove", "result": "success", "data": {{
      "retained_data": [{{key: receipt[key] for key in ("data_receipt_id", "physical_backend_id", "scope", "state")}}]
    }}}}))
    raise SystemExit(0)
elif command == "remove":
    state = json.loads(state_path.read_text()); locator = pathlib.Path(next(iter(state["installations"][0]["data_receipts"].values()))["locator"])
    if os.environ.get("RENAME_INSTEAD_OF_UNLINK"):
        locator.joinpath("launch-marker.txt").rename(manager / "renamed-launch-marker.txt")
    shutil.rmtree(locator, ignore_errors=True)
    empty = {{"schema_version": 4, "installations": []}}
    os.chmod(manager, 0o700)
    atomic_state(empty)
    atomic_state(empty)
    if os.environ.get("EXTRA_REPLACEMENT_EPOCH"):
        atomic_state(empty)
    if os.environ.get("SYMLINK_STATE_EXCURSION"):
        saved = state_path.with_name("saved-state"); state_path.rename(saved)
        state_path.symlink_to(manager / "outside-state"); state_path.unlink(); saved.rename(state_path)
else: raise SystemExit(2)
print("{{}}")
'''
        for mode in ("atomic-state", "renamed-marker", "extra-replacement", "symlink-excursion"):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp); binary = root / "agentplugins"; binary.write_text(fake); binary.chmod(0o700)
                workspace = root / "workspace"; workspace.mkdir()
                environment = {"HOME": str(root / "home"), "AGENTPLUGINS_HOME": str(root / "manager"), "PYTHONDONTWRITEBYTECODE": "1"}
                if mode == "renamed-marker": environment["RENAME_INSTEAD_OF_UNLINK"] = "1"
                if mode == "extra-replacement": environment["EXTRA_REPLACEMENT_EPOCH"] = "1"
                if mode == "symlink-excursion": environment["SYMLINK_STATE_EXCURSION"] = "1"
                with mock.patch.dict(os.environ, environment, clear=False):
                    passed, value = observer.plugin_data_scenario(binary, ("cursor",), workspace, "challenge")
                expected = mode == "atomic-state"
                self.assertEqual(passed, expected, value)
                self.assertEqual(value["proof"]["explicit_owned_purge_deleted"], expected)

        for attacked_command in ("info", "update", "repair", "switch", "remove-retain"):
            for attack_mode in ("modify-restore", "permanent-addition", "rename-symlink"):
                with self.subTest(command=attacked_command, attack=attack_mode), tempfile.TemporaryDirectory() as tmp:
                    root = Path(tmp); binary = root / "agentplugins"; binary.write_text(fake); binary.chmod(0o700)
                    workspace = root / "workspace"; workspace.mkdir()
                    outside = root / "outside"; outside.mkdir(); victim = outside / "victim"; victim.write_text("survives")
                    environment = {
                        "HOME": str(root / "home"), "AGENTPLUGINS_HOME": str(root / "manager"),
                        "PYTHONDONTWRITEBYTECODE": "1", "ATTACK_COMMAND": attacked_command,
                        "ATTACK_MODE": attack_mode, "OUTSIDE": str(outside),
                    }
                    with mock.patch.dict(os.environ, environment, clear=False):
                        passed, value = observer.plugin_data_scenario(binary, ("cursor",), workspace, "challenge")
                    self.assertFalse(passed, value)
                    if attacked_command == "info":
                        self.assertNotEqual(value["command_traces"][1]["exit_code"], 0, value)
                    else:
                        self.assertFalse(value["proof"]["remove_preserved"], value)
                    self.assertEqual(victim.read_text(), "survives")

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); binary = root / "agentplugins"; binary.write_text(fake); binary.chmod(0o700)
            workspace = root / "workspace"; workspace.mkdir(); detached_proof = workspace / "detached-proof"
            environment = {
                "HOME": str(root / "home"), "AGENTPLUGINS_HOME": str(root / "manager"),
                "PYTHONDONTWRITEBYTECODE": "1", "DETACHED_PROOF": str(detached_proof),
            }
            with mock.patch.dict(os.environ, environment, clear=False):
                passed, value = observer.plugin_data_scenario(binary, ("cursor",), workspace, "challenge")
            time.sleep(0.5)
            self.assertTrue(passed, value)
            self.assertEqual(detached_proof.read_text(), "denied")
    def test_plugin_data_update_proof_fails_closed_for_no_change_or_receipt_replacement(self) -> None:
        first = {"tree_digest": "sha256:" + "a" * 64, "manifest_digest": "sha256:" + "e" * 64}
        second = {"tree_digest": "sha256:" + "b" * 64, "manifest_digest": "sha256:" + "f" * 64}
        receipt = {"locator": "/disposable/data", "ownership_digest": "sha256:" + "c" * 64}
        self.assertEqual(observer.plugin_data_update_proof(first, first, receipt, receipt, first, first), (False, True))
        self.assertEqual(
            observer.plugin_data_update_proof(
                first, second, receipt, {**receipt, "ownership_digest": "sha256:" + "d" * 64}, first, second,
            ),
            (True, False),
        )
        for field in ("tree_digest", "manifest_digest"):
            forged = {**second, field: "sha256:" + "9" * 64}
            self.assertEqual(
                observer.plugin_data_update_proof(first, forged, receipt, receipt, first, second),
                (False, True),
            )

    def test_receipts_are_scoped_to_one_installation_and_exact_command_targets(self) -> None:
        _, authoritative = self.agentplugins_0_1_14_state_fixture()
        with tempfile.TemporaryDirectory() as tmp:
            manager = Path(tmp)
            authoritative = self.sandbox_state_fixture(authoritative, manager)
            state_path = manager / "state-v2.json"
            state_path.write_text(json.dumps(authoritative))
            receipts = observer.installation_receipts(manager, "context7")
            self.assertEqual(len(receipts or []), 3)
            self.assertTrue(observer.receipts_bind_command([], receipts or [], "add", ("codex", "cursor", "kiro")))
            self.assertFalse(observer.receipts_bind_command([], receipts or [], "add", ("codex", "cursor", "cursor")))
            duplicated = json.loads(json.dumps(authoritative))
            duplicated["installations"].append(duplicated["installations"][0])
            state_path.write_text(json.dumps(duplicated))
            self.assertIsNone(observer.selected_manager_installation(manager, "context7"))
            split = json.loads(json.dumps(authoritative))
            receipt = next(iter(split["installations"][0]["clients"].values()))["receipts"][0]
            receipt["operation_group_id"] = "different-authority"
            state_path.write_text(json.dumps(split))
            historical = observer.installation_receipts(manager, "context7")
            self.assertIsNone(historical)
            self.assertFalse(observer.receipts_bind_command([], historical or [], "add", ("codex", "cursor", "kiro")))

    def test_canonical_data_locator_rejects_traversal_and_symlink_escape(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            allowed = base / "allowed"
            outside = base / "outside"
            allowed.mkdir()
            outside.mkdir()
            owned = allowed / "owned"
            owned.mkdir()
            (allowed / "escape").mkdir()
            self.assertEqual(observer.canonical_allowed_locator(owned, (allowed,)), owned.resolve())
            self.assertIsNone(observer.canonical_allowed_locator(allowed / "owned" / ".." / "escape", (allowed,)))
            (allowed / "link").symlink_to(outside, target_is_directory=True)
            self.assertIsNone(observer.canonical_allowed_locator(allowed / "link", (allowed,)))

    def test_full_sha_identity_and_refetch_signal_require_exact_complete_fields(self) -> None:
        update = release_fixture("local-update.json")
        self.assertTrue(observer.validate_cli_envelope(update, "update"))
        self.assertIsNone(observer.command_acquisition_proof(update, ("codex", "cursor", "kiro"), command="update"))
    def test_direct_full_sha_scenario_fails_without_explicit_update_refetch(self) -> None:
        fake = f'''#!/usr/bin/python3
import hashlib, json, os, pathlib, shutil, sys
manager = pathlib.Path(os.environ["AGENTPLUGINS_HOME"])
manager.mkdir(parents=True, exist_ok=True)
state_path = manager / "state-v2.json"
command = sys.argv[1]
tree = "sha256:" + "1" * 64
manifest = "sha256:" + "2" * 64
if command == "add":
    selector = sys.argv[2]
    repository_revision, package_path = selector.split("//", 1)
    repository, revision = repository_revision.split("@", 1)
    target = manager / "managed/cursor/e2e-external-package"
    target.mkdir(parents=True, exist_ok=True)
    (target / "plugin.json").write_text("{{}}")
    binding_id = "direct-cursor-binding"
    state = {{"schema_version": 4, "installations": [{{
      "installation_id": "direct-installation", "declared_name": "e2e-external-package", "origin_mode": "direct",
      "source": {{"source_binding_id": "direct-source", "requested_source": selector,
        "canonical_source": "https://github.com/" + repository + "@" + revision + "//" + package_path,
        "repository": repository, "package_subpath": package_path, "resolved_revision": revision, "tree_digest": tree}},
      "package": {{"loader_kind": "agent_plugins", "format_id": "agent-plugins/1.0.0",
        "schema_uri": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
        "declared_name": "e2e-external-package", "version": "1.0.0", "manifest_digest": manifest, "inventory": {{"mcp_present": False, "mcp_enabled": False}}}},
      "clients": {{binding_id: {{"client_binding_id": binding_id, "client_id": "cursor", "scope": "user",
        "target_locator": str(target), "physical_artifact_id": "direct-artifact", "materialization": "materialized",
        "activation": "manual_activation_required", "authentication": "not_checked", "policy": "allowed",
        "verification": "package_validated", "package_revision": {{"version": "1.0.0", "resolved_revision": revision,
          "tree_digest": tree, "manifest_digest": manifest}}, "updated_at": "2026-08-24T00:00:00Z"}}}},
      "created_at": "2026-08-24T00:00:00Z", "updated_at": "2026-08-24T00:00:00Z"
    }}]}}
    state_path.write_text(json.dumps(state, sort_keys=True))
    value = json.loads(pathlib.Path({str(AGENTPLUGINS_0_1_14_FIXTURES / "add.json")!r}).read_text())
    item = next(item for item in value["data"]["targets"] if item["target"] == "cursor")
    source = repository + "//" + package_path
    for output in (value["data"], item["output"]):
        output["source"] = source; output["revision"] = revision; output["tree_digest"] = tree; output["manifest_digest"] = manifest
        output["version"] = "1.0.0"
    item["output"]["result"]["installation_id"] = "direct-installation"
    value["data"]["targets"] = [item]; value["data"]["succeeded"] = 1
    acquisition = value["data"]["acquisition"]
    digest = hashlib.sha256()
    for field in ("agentplugins/grouped-acquisition-closure/v1", "github", repository, package_path, revision, tree, manifest):
        body = field.strip().encode(); digest.update(len(body).to_bytes(8, "big")); digest.update(body)
    acquisition.update({{"tree_digest": tree, "manifest_digest": manifest, "closure_digest": "sha256:" + digest.hexdigest(), "source_kind": "github", "fetched": True, "validated": True}})
    binding = {{"outcome": "passed", "acquisition_id": acquisition["acquisition_id"], "tree_digest": tree,
      "manifest_digest": manifest, "closure_digest": acquisition["closure_digest"]}}
    value["data"]["target_outcomes"] = {{"cursor": binding}}
    print(json.dumps(value))
elif command == "update":
    if os.environ.get("MUTATE_DURING_REFUSAL"):
        state_path.write_bytes(state_path.read_bytes() + b" ")
    value = json.loads(pathlib.Path({str(AGENTPLUGINS_0_1_14_FIXTURES / "direct-update-failure.json")!r}).read_text())
    state = json.loads(state_path.read_text()); source = state["installations"][0]["source"]
    value["data"].update({{"failed": 1, "plugin": "e2e-external-package", "source": source["repository"] + "//" + source["package_subpath"],
      "revision": source["resolved_revision"], "tree_digest": source["tree_digest"], "version": "1.0.0"}})
    print(json.dumps(value))
    print({observer.FULL_SHA_UPDATE_STDERR!r}, end="", file=sys.stderr)
    raise SystemExit(1)
elif command == "remove":
    if state_path.exists():
        state = json.loads(state_path.read_text()); shutil.rmtree(pathlib.Path(next(iter(state["installations"][0]["clients"].values()))["target_locator"]).parent.parent, ignore_errors=True); state_path.unlink()
    print("{{}}")
else: raise SystemExit(2)
'''
        context = {
            "github_sha": "a" * 40,
            "catalog_repository": "777genius/universal-agent-plugins",
        }
        for adversarial in (False, True):
            with self.subTest(adversarial=adversarial), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                binary = root / "agentplugins"
                binary.write_text(fake)
                binary.chmod(0o700)
                workspace = root / "workspace"
                workspace.mkdir()
                environment = {"HOME": str(root / "home"), "AGENTPLUGINS_HOME": str(root / "manager")}
                if adversarial:
                    environment["MUTATE_DURING_REFUSAL"] = "1"
                with mock.patch.dict(os.environ, environment, clear=False):
                    passed, value = observer.direct_full_sha_scenario(binary, ("cursor",), workspace, "challenge", context)
                self.assertEqual(passed, not adversarial, value)
                self.assertEqual(value["proof"]["direct_update_refused_requires_switch"], not adversarial)
    def test_evidence_boundary_rejects_incomplete_or_forged_acquisition(self) -> None:
        targets = ("codex", "cursor", "kiro")
        tree = "sha256:" + "a" * 64
        manifest = "sha256:" + "b" * 64
        proof = {
            "acquisition_id": "fetch-1", "acquisition_count": 1,
            "tree_digest": tree, "manifest_digest": manifest,
            "closure_digest": observer.grouped_acquisition_closure_digest(
                "github", "upstash/context7", "plugins/context7", "1" * 40, tree, manifest,
            ),
            "source_kind": "github", "fetched": True, "validated": True,
            "source_repository": "upstash/context7", "source_revision": "1" * 40,
            "source_path": "plugins/context7", "targets": [{"target": client} for client in targets],
        }
        proof["target_outcomes"] = {
            client: {"outcome": "passed", "acquisition_id": "fetch-1", "tree_digest": tree, "manifest_digest": manifest, "closure_digest": proof["closure_digest"]}
            for client in targets
        }
        arguments = {"tree_digest": tree, "manifest_digest": manifest, "source_repository": "upstash/context7", "source_revision": "1" * 40, "source_path": "plugins/context7"}
        self.assertEqual(e2e.complete_acquisition_proof(proof, targets, **arguments), proof)
        for field in ("acquisition_id", "acquisition_count", "source_kind", "fetched", "validated", "tree_digest", "manifest_digest", "closure_digest", "source_repository", "source_revision", "source_path", "targets", "target_outcomes"):
            with self.subTest(field=field):
                incomplete = {key: child for key, child in proof.items() if key != field}
                self.assertIsNone(e2e.complete_acquisition_proof(incomplete, targets, **arguments))
        self.assertIsNone(e2e.complete_acquisition_proof({**proof, "acquisition_count": True}, targets, **arguments))
        forged = json.loads(json.dumps(proof))
        forged["target_outcomes"]["kiro"]["tree_digest"] = "sha256:" + "d" * 64
        self.assertIsNone(e2e.complete_acquisition_proof(forged, targets, **arguments))

    def test_policy_conformance_directory_is_test_signed_and_never_the_production_root(self) -> None:
        snapshot = json.loads((PUBLICATION / "snapshot.json").read_text())
        distribution = snapshot["distributions"][0]
        release = distribution["releases"][0]
        context = {
            "github_sha": "a" * 40,
            "expected_version": e2e.CURRENT_LAUNCH_VERSION,
            "directory_product": snapshot["products"][0],
            "directory_distribution": distribution,
            "release": {
                "release_sequence": release["sequence"],
                "product_id": snapshot["products"][0]["id"],
                "distribution_id": distribution["id"],
                "tree_digest": release["tree_digest"],
            },
        }
        with tempfile.TemporaryDirectory() as tmp:
            environment, digest = observer.conformance_directory(
                Path(tmp), context, sequence=1007, sequence_over_semver=True,
            )
            trust = json.loads(Path(environment["AGENTPLUGINS_DIRECTORY_TRUST"]).read_text())
            generated = json.loads(Path(environment["AGENTPLUGINS_DIRECTORY_SNAPSHOT"]).read_text())
        self.assertTrue(digest.startswith("sha256:"))
        self.assertEqual(trust["keys"][0]["key_id"], "launch-conformance-only")
        self.assertNotEqual(trust, json.loads(e2e.PRODUCTION_DIRECTORY_TRUST.read_text()))
        self.assertEqual([item["sequence"] for item in generated["distributions"][0]["releases"]], [1, 2])
        self.assertEqual([item["package_version"] for item in generated["distributions"][0]["releases"]], ["9.0.0", "1.0.0"])
        self.assertEqual(
            [item["minimum_installer_version"] for item in generated["distributions"][0]["release_policies"]],
            [e2e.CURRENT_LAUNCH_VERSION, e2e.CURRENT_LAUNCH_VERSION],
        )

    def test_fixture_contracts_cover_required_fault_slots(self) -> None:
        config = json.loads(e2e.SCENARIOS.read_text())
        required = {
            "directory_offline", "directory_expired", "directory_tampered", "directory_sequence_rollback",
            "missing_runtime_zero_mutation", "plugin_data_update_repair_switch_remove_purge",
            "stdio_environment_and_containment", "promotion_gate_digest_mismatch",
            "distribution_sticky_update", "managed_rollback",
        }
        observed = set(config["fault_scenarios"] + config["advanced_scenarios"])
        self.assertTrue(required.issubset(observed))
        for scenario in config["fault_scenarios"] + config["adapter_repair_faults"] + config["advanced_scenarios"]:
            with self.subTest(scenario=scenario):
                self.assertFalse(e2e.LaunchHarness.driver_proof_valid(scenario, {"outcome": "passed"}))

    def test_source_identity_rows_fail_closed_for_missing_or_spoofed_manager_identity(self) -> None:
        expected_release = {
            "product_id": "context7", "distribution_id": "upstash/context7", "distribution_kind": "upstream",
            "release_sequence": 1, "source_revision": "1" * 40, "tree_digest": "sha256:" + "a" * 64,
            "manifest_digest": "sha256:" + "b" * 64, "package_version": "1.0.0",
            "source_repository": "upstash/context7", "source_path": "plugins/context7",
        }
        expected_identity = {key: expected_release[key] for key in (
            "product_id", "distribution_id", "distribution_kind", "release_sequence", "source_revision",
            "source_repository", "source_path", "tree_digest", "manifest_digest",
        )}
        expected_identity["canonical_source"] = "https://github.com/upstash/context7@" + "1" * 40 + "//plugins/context7"
        self.assertTrue(e2e.LaunchHarness.source_identity_matches_release(expected_release, expected_identity))
        self.assertTrue(e2e.LaunchHarness.source_identity_matches_release(
            expected_release,
            {**expected_identity, "canonical_source": "upstash/context7@" + "1" * 40 + "//plugins/context7"},
        ))
        # Short-name identity is now a source-policy contract. It cannot be
        # turned into a released-binary row, even when a manager-shaped value
        # is supplied by a test observer.
        harness = self.fixture_harness()
        with mock.patch.object(harness, "fresh_sandbox") as sandbox, mock.patch.object(e2e.subprocess, "run") as process:
            with self.assertRaisesRegex(ValueError, "source-policy conformance"):
                harness.driven_scenario("upstream_owned_short_name")
        sandbox.assert_not_called()
        process.assert_not_called()

    def test_canonical_github_source_parser_rejects_noncanonical_identity(self) -> None:
        revision = "1" * 40
        shorthand = f"upstash/context7@{revision}//plugins/context7"
        production = f"https://github.com/upstash/context7@{revision}//plugins/context7"
        expected = {
            "source_repository": "upstash/context7",
            "source_revision": revision,
            "source_path": "plugins/context7",
        }
        for exact in (shorthand, production):
            with self.subTest(exact=exact):
                self.assertEqual(observer.parse_canonical_github_source(exact), expected)
                self.assertEqual(e2e.parse_canonical_github_source(exact), expected)
        expected_identity = {**expected, "canonical_source": production}
        observed_identity = {**expected, "canonical_source": shorthand}
        self.assertTrue(observer.source_identities_match(expected_identity, observed_identity))
        invalid = (
            f"http://github.com/upstash/context7@{revision}//plugins/context7",
            f"https://user@github.com/upstash/context7@{revision}//plugins/context7",
            f"https://gitlab.com/upstash/context7@{revision}//plugins/context7",
            f"https://GitHub.com/upstash/context7@{revision}//plugins/context7",
            f"https://github.com:443/upstash/context7@{revision}//plugins/context7",
            f"https://github.com//upstash/context7@{revision}//plugins/context7",
            f"https://github.com/upstash/context7@{revision}///plugins/context7",
            "upstash/context7@main//plugins/context7",
            f"upstash/context7@{revision}//plugins/../context7",
            f"upstash/context7@{revision}//plugins//context7",
            f"upstash/context7@{revision}//plugins/context7?ref=main",
            f"upstash/context7@{revision}//plugins/context7#fragment",
            f"upstash/context7@{revision}//plugins/%2e%2e/context7",
            f"https:/github.com/upstash/context7@{revision}//plugins/context7",
            f"https://github.com/upstash/context7@{revision}//plugins/context 7",
        )
        for value in invalid:
            with self.subTest(value=value):
                self.assertIsNone(observer.parse_canonical_github_source(value))
                self.assertIsNone(e2e.parse_canonical_github_source(value))

    def test_directory_evidence_artifact_requires_complete_source_identity(self) -> None:
        schema = json.loads((ROOT / "schemas/directory-evidence-artifact.schema.json").read_text())
        artifact = {
            "schema_version": 1,
            "id": "runtime-context7-cursor",
            "product_id": "context7",
            "distribution_id": "upstash/context7",
            "release_sequence": 1,
            "package_tree_digest": "sha256:" + "a" * 64,
            "manifest_digest": "sha256:" + "b" * 64,
            "source_repository": "upstash/context7",
            "source_revision": "1" * 40,
            "source_path": "plugins/context7",
            "level": "runtime",
            "outcome": "passed",
            "client": "cursor",
            "client_version": "1.0.0",
            "installer_version": "0.1.8",
            "os": "linux",
            "architecture": "amd64",
            "observed_at": "2026-08-22T00:00:00Z",
        }
        jsonschema.Draft202012Validator(schema).validate(artifact)
        for field in ("product_id", "manifest_digest", "source_repository", "source_revision", "source_path"):
            with self.subTest(field=field):
                invalid = dict(artifact)
                invalid.pop(field)
                with self.assertRaises(jsonschema.ValidationError):
                    jsonschema.Draft202012Validator(schema).validate(invalid)

    def test_source_scenarios_select_concrete_reviewed_directory_distributions(self) -> None:
        harness = self.fixture_harness()
        harness.snapshot = json.loads((ROOT / "registry/directory.json").read_text())
        publication_revision = "f" * 40
        for distribution in harness.snapshot["distributions"]:
            for release in distribution["releases"]:
                source = release.get("package_source", {})
                if source.get("repository") == e2e.TRUSTED_CATALOG_REPOSITORY and source.get("revision") is None:
                    source["revision"] = publication_revision
        upstream = harness.configured_source_release("upstream_owned_short_name", ["cursor"])
        bridge = harness.configured_source_release("community_bridge_short_name", ["cursor"])
        self.assertEqual(
            (upstream["product_id"], upstream["distribution_id"], upstream["distribution_kind"], upstream["source_revision"]),
            ("context7", "upstash/context7", "upstream", "769c6cd22c3d95462d1f55d789e9532cabefa5a9"),
        )
        self.assertEqual(
            (bridge["product_id"], bridge["distribution_id"], bridge["distribution_kind"], bridge["source_revision"]),
            ("cloudflare-docs", "777genius/cloudflare-docs-bridge", "community_bridge", publication_revision),
        )

    def test_manager_identity_does_not_aggregate_authority_across_records(self) -> None:
        state = release_fixture("state-v2.json")
        with tempfile.TemporaryDirectory() as tmp:
            manager = Path(tmp)
            state = self.sandbox_state_fixture(state, manager)
            (manager / "state-v2.json").write_text(json.dumps(state))
            self.assertEqual(observer.manager_identity(manager, "context7")["resolved_revision"], "769c6cd22c3d95462d1f55d789e9532cabefa5a9")
            extra = json.loads(json.dumps(state["installations"][0]))
            extra["installation_id"] = "extra-installation"
            extra["declared_name"] = "other"
            extra["package"]["declared_name"] = "other"
            state["installations"].append(extra)
            (manager / "state-v2.json").write_text(json.dumps(state))
            self.assertEqual(observer.manager_identity(manager, "context7"), {})

    def test_promotion_and_fork_observers_execute_exact_local_validators(self) -> None:
        scenarios = (
            "promotion_gate_digest_match", "promotion_gate_digest_mismatch",
            "fork_submission", "fork_submission_rejected",
        )
        match_candidate_digest = None
        with tempfile.TemporaryDirectory() as tmp:
            parent = Path(tmp)
            for scenario in scenarios:
                with self.subTest(scenario=scenario):
                    root = parent / scenario
                    root.mkdir()
                    environment = {"HOME": str(root / "home"), "AGENTPLUGINS_HOME": str(root / "manager")}
                    with mock.patch.dict(os.environ, environment, clear=False):
                        if scenario.startswith("promotion_"):
                            passed, value = observer.promotion_scenario(Path("/not-used"), scenario, root, "a" * 64)
                        else:
                            passed, value = observer.fork_submission_scenario(scenario, root, "a" * 64)
                    self.assertTrue(passed, value)
                    artifact = value["validator_artifact"]
                    if scenario.endswith("mismatch") or scenario.endswith("rejected"):
                        self.assertEqual(artifact["outcome"], "rejected")
                    else:
                        self.assertEqual(artifact["outcome"], "accepted")
                        self.assertTrue(artifact["gates"])
                    if scenario.startswith("promotion_gate_"):
                        self.assertEqual(value["command_traces"][0]["argv"], [
                            str(observer.JOURNEY_VALIDATOR), "promotion",
                            "--repository", str(root / "upstream-repository"),
                            "--pr-metadata", str(root / "pr-metadata.json"),
                            "--path", "packages/chrome-devtools",
                            "--review-record", str(root / "promotion-review.json"),
                            "--candidate-output", str(root / "promotion-candidate.json"),
                        ])
                    if scenario == "promotion_gate_digest_match":
                        match_candidate_digest = artifact["candidate_digest"]
            repeat = parent / "promotion_gate_digest_match_repeat"
            repeat.mkdir()
            with mock.patch.dict(os.environ, {"HOME": str(repeat / "home"), "AGENTPLUGINS_HOME": str(repeat / "manager")}, clear=False):
                passed, value = observer.promotion_scenario(Path("/not-used"), "promotion_gate_digest_match", repeat, "a" * 64)
            self.assertTrue(passed, value)
            self.assertEqual(value["validator_artifact"]["candidate_digest"], match_candidate_digest)

    def test_git_worktree_digest_ignores_internal_stat_cache_only(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / ".git").mkdir()
            (root / ".git" / "index").write_bytes(b"index-v1")
            worktree = root / "plugin.json"
            worktree.write_bytes(b"plugin-v1")
            baseline = observer.git_worktree_digest(root)
            (root / ".git" / "index").write_bytes(b"index-v2")
            self.assertEqual(observer.git_worktree_digest(root), baseline)
            worktree.write_bytes(b"plugin-v2")
            self.assertNotEqual(observer.git_worktree_digest(root), baseline)

    def test_journey_aggregation_requires_accepted_and_rejected_fork_artifacts(self) -> None:
        harness = self.fixture_harness()
        harness.cli_version = "0.1.18"
        accepted = {
            "fork_created": True, "branch_submission": True, "submission_validated": True,
            "publication_performed": False, "pr_created": False, "network_performed": False,
            "client_version": "fixture-validator-v1",
        }
        rejected = {
            "fork_created": True, "submission_rejected": True, "no_side_effect": True,
            "no_candidate": True, "client_version": "fixture-validator-v1",
        }
        with mock.patch.object(harness, "command", return_value=("failed", None, "not under test")), mock.patch.object(
            harness, "driven_scenario", side_effect=[("passed", accepted, "accepted"), ("passed", rejected, "rejected")],
        ):
            harness.journeys()
        rows = {row["scenario"]: row for row in harness.rows}
        self.assertEqual(rows["fork_submission"]["outcome"], "passed")
        self.assertEqual(rows["fork_submission_rejected"]["outcome"], "passed")

    def test_direct_external_journey_requires_add_info_and_remove(self) -> None:
        harness = self.fixture_harness()
        harness.cli_version = "0.1.18"
        digest = e2e.package_digest(e2e.EXTERNAL_PACKAGE)
        command_results = [
            ("passed", {"package_digest": digest, "client_version": "cursor-test-v1", "mutated": True, "_launch_command_trace": {"argv": ["add"]}}, "added"),
            ("passed", {"receipt_reconciled": True, "native_discovery_reconciled": True, "client_version": "cursor-test-v1", "native_discovery_evidence": {"basis": "protected_external_observer", "version_operation": {"operation": "version", "argv": ["cursor", "--version"], "observed_client_version": "cursor-test-v1"}, "discovery_operation": {"operation": "list", "argv": ["cursor", "plugins", "list"], "discovered": True, "product_id": "e2e-external-package"}}, "_launch_command_trace": {"argv": ["info"]}}, "reconciled"),
            ("passed", {"data": {"targets": [{"output": {"result": {"mutated": False, "no_change": True, "group_phase": "external_completed"}}} for _ in range(3)]}, "_launch_command_trace": {"argv": ["update"]}}, "unchanged"),
            ("passed", {"mutated": True, "_launch_command_trace": {"argv": ["remove"]}}, "removed"),
        ]
        accepted = {"fork_created": True, "branch_submission": True, "submission_validated": True, "publication_performed": False, "pr_created": False, "network_performed": False}
        rejected = {"fork_created": True, "submission_rejected": True, "no_side_effect": True, "no_candidate": True}
        with mock.patch.object(harness, "command", side_effect=command_results) as command, mock.patch.object(
            harness, "driven_scenario", side_effect=[("passed", accepted, "accepted"), ("passed", rejected, "rejected")],
        ):
            harness.journeys()
        row = next(item for item in harness.rows if item["scenario"] == "direct_external_package")
        self.assertEqual(row["outcome"], "passed")
        self.assertEqual(row["details"]["evidence_basis"], "repository_owned_disposable_observer")
        self.assertEqual(row["details"]["tree_digest_algorithm"], "agentplugins-tree-sha256-v1")
        self.assertEqual([call.args[0][0] for call in command.call_args_list], ["add", "info", "update", "remove"])
        self.assertEqual(row["details"]["operations"]["info"]["outcome"], "passed")
        self.assertEqual(len(row["details"]["command_traces"]), 4)

    def test_direct_external_journey_fails_when_info_or_cleanup_is_not_proved(self) -> None:
        digest = e2e.package_digest(e2e.EXTERNAL_PACKAGE)
        accepted = {"fork_created": True, "branch_submission": True, "submission_validated": True, "publication_performed": False, "pr_created": False, "network_performed": False}
        rejected = {"fork_created": True, "submission_rejected": True, "no_side_effect": True, "no_candidate": True}
        cases = (
            (
                "receipt-only materialization",
                "passed",
                [
                    ("passed", {"package_digest": digest, "client_version": "cursor-test-v1", "mutated": True}, "added"),
                    ("passed", {"receipt_reconciled": True, "native_discovery_reconciled": False}, "partial"),
                    ("passed", {"data": {"targets": [{"output": {"result": {"mutated": False, "no_change": True, "group_phase": "external_completed"}}} for _ in range(3)]}}, "unchanged"),
                    ("passed", {"mutated": True}, "removed"),
                ],
            ),
            (
                "failed cleanup",
                "failed",
                [
                    ("passed", {"package_digest": digest, "client_version": "cursor-test-v1", "mutated": True}, "added"),
                    ("passed", {"receipt_reconciled": True, "native_discovery_reconciled": True}, "reconciled"),
                    ("passed", {"data": {"targets": [{"output": {"result": {"mutated": False, "no_change": True, "group_phase": "external_completed"}}} for _ in range(3)]}}, "unchanged"),
                    ("failed", None, "remove failed"),
                ],
            ),
        )
        for label, expected_outcome, command_results in cases:
            with self.subTest(label=label):
                harness = self.fixture_harness()
                harness.cli_version = "0.1.18"
                with mock.patch.object(harness, "command", side_effect=command_results) as command, mock.patch.object(
                    harness, "driven_scenario", side_effect=[("passed", accepted, "accepted"), ("passed", rejected, "rejected")],
                ):
                    harness.journeys()
                row = next(item for item in harness.rows if item["scenario"] == "direct_external_package")
                self.assertEqual(row["outcome"], expected_outcome)
                self.assertEqual([call.args[0][0] for call in command.call_args_list], ["add", "info", "update", "remove"])

    def test_missing_runtime_proof_requires_zero_mutation_and_no_install(self) -> None:
        proof = {"zero_mutation": True, "copy_ready_requirement": True, "dependency_installed": False}
        self.assertTrue(e2e.LaunchHarness.driver_proof_valid("missing_runtime_zero_mutation", proof))
        self.assertFalse(e2e.LaunchHarness.driver_proof_valid("missing_runtime_zero_mutation", {**proof, "dependency_installed": True}))

    def test_dead_required_scenario_omission_is_rejected(self) -> None:
        config = json.loads(e2e.SCENARIOS.read_text())
        required = config["fault_scenarios"] + config["adapter_repair_faults"] + config["advanced_scenarios"] + config["acceptance_postconditions"] + config["journeys"] + ["shared_copilot_vscode_backend"]
        rows = [{"scenario": scenario} for scenario in required]
        e2e.validate_enforced_scenario_coverage(rows, config)
        with self.assertRaisesRegex(ValueError, "omitted or duplicated"):
            e2e.validate_enforced_scenario_coverage(rows[1:], config)

    def test_fixture_only_claim_escalation_is_rejected(self) -> None:
        evidence = self.fixture_harness().export()
        evidence["run"]["runtime_claims"] = True
        with self.assertRaisesRegex(ValueError, "cannot escalate"):
            e2e.assert_redacted(evidence)

    def test_external_pr_gate_fails_closed_for_every_untrustworthy_evidence_class(self) -> None:
        now = datetime.now(timezone.utc).replace(microsecond=0)
        challenge = {"value": "a" * 64}
        snapshot = {"sequence": 9, "publication_id": "publication-9", "source_commit": "b" * 40}
        binding = {
            "catalog_repository": e2e.TRUSTED_CATALOG_REPOSITORY,
            "catalog_sha": "c" * 40,
            "directory_snapshot_digest": "sha256:" + "d" * 64,
            "directory_sequence": 9,
            "directory_publication_id": "publication-9",
            "directory_source_commit": "b" * 40,
            "release_repository": e2e.TRUSTED_CLI_RELEASE_REPOSITORY,
            "release_tag": e2e.TRUSTED_CLI_RELEASE_TAG,
            "release_commit": "e" * 40,
            "release_manifest_digest": "sha256:" + "f" * 64,
        }
        record = {
            "schema_version": 1, "challenge": challenge["value"],
            "catalog_repository": e2e.TRUSTED_CATALOG_REPOSITORY,
            "fork_owner": "external-contributor", "fork_repository": "external-contributor/universal-agent-plugins",
            "pr_number": 42, "pr_url": f"https://github.com/{e2e.TRUSTED_CATALOG_REPOSITORY}/pull/42",
            "head_sha": "1" * 40, "base_sha": "c" * 40, "merge_commit_sha": None,
            "changed_paths": ["registry/directory.json", "registry/review-preview.json", "registry/review-search.json"],
            "check_runs": [{"name": "portable-catalog", "conclusion": "success", "head_sha": "1" * 40}],
            "final_review": {"state": "closed", "decision": "validated", "reviewer_count": 1, "closed_at": now.isoformat().replace("+00:00", "Z"), "merged_at": None},
            "observed_at": now.isoformat().replace("+00:00", "Z"),
            "immutable_artifact": {"digest": "sha256:" + "3" * 64, "reference": "urn:sha256:" + "3" * 64},
            "binding": binding,
        }

        def verify(value):
            return e2e.external_pr_evidence_valid(
                value, catalog_repository=e2e.TRUSTED_CATALOG_REPOSITORY,
                catalog_sha="c" * 40, snapshot=snapshot,
                snapshot_digest="sha256:" + "d" * 64,
                release_repository=e2e.TRUSTED_CLI_RELEASE_REPOSITORY,
                release_tag=e2e.TRUSTED_CLI_RELEASE_TAG, release_commit="e" * 40,
                release_manifest_digest="sha256:" + "f" * 64, now=now,
            )

        self.assertEqual(
            verify(record),
            (True, "signed immutable historical external-fork PR evidence verified"),
        )
        self.assertTrue(e2e.external_pr_evidence_valid(
            record, catalog_repository=e2e.TRUSTED_CATALOG_REPOSITORY,
            catalog_sha="c" * 40, snapshot=snapshot,
            snapshot_digest="sha256:" + "d" * 64,
            release_repository=e2e.TRUSTED_CLI_RELEASE_REPOSITORY,
            release_tag=e2e.TRUSTED_CLI_RELEASE_TAG, release_commit="e" * 40,
            release_manifest_digest="sha256:" + "f" * 64,
            now=now + timedelta(days=365),
        )[0])
        negatives = {
            "missing": None,
            "local": {**record, "fork_repository": "local"},
            "self_owned": {**record, "fork_owner": "777genius", "fork_repository": e2e.TRUSTED_CATALOG_REPOSITORY},
            "future": {**record, "observed_at": (now + timedelta(hours=1)).isoformat().replace("+00:00", "Z")},
            "invalid_capture_challenge": {**record, "challenge": "not-a-challenge"},
            "wrong_release": {**record, "binding": {**binding, "release_tag": "v0.1.13"}},
            "wrong_directory": {**record, "binding": {**binding, "directory_sequence": 8}},
            "unsafe_directory_alias_low": {**record, "binding": {**binding, "directory_sequence": 9_007_199_254_740_992}},
            "unsafe_directory_alias_high": {**record, "binding": {**binding, "directory_sequence": 9_007_199_254_740_993}},
            "base_binding_mismatch": {**record, "binding": {**binding, "catalog_sha": "4" * 40}},
            "wrong_base": {**record, "base_sha": "4" * 40},
            "unexpected_merge": {**record, "merge_commit_sha": "4" * 40},
            "wrong_path": {**record, "changed_paths": ["site/index.html"]},
            "wrong_head": {**record, "check_runs": [{"name": "portable-catalog", "conclusion": "success", "head_sha": "4" * 40}]},
            "failed_check": {**record, "check_runs": [{"name": "portable-catalog", "conclusion": "failure", "head_sha": "1" * 40}]},
            "unreviewed": {**record, "final_review": {**record["final_review"], "reviewer_count": 0}},
            "merged_instead_of_closed": {**record, "final_review": {**record["final_review"], "state": "merged"}},
            "mutable_reference": {**record, "immutable_artifact": {**record["immutable_artifact"], "reference": "https://example.test/latest.json"}},
        }
        for name, value in negatives.items():
            with self.subTest(name=name):
                self.assertFalse(verify(value)[0])

        schema_path = ROOT / "tests/e2e/schemas/external-pr-evidence.schema.json"
        validator = jsonschema.Draft202012Validator(json.loads(schema_path.read_text()))
        boundary_record = {**record, "binding": {**binding, "directory_sequence": 9_007_199_254_740_991}}
        boundary_snapshot = {**snapshot, "sequence": 9_007_199_254_740_991}
        validator.validate(boundary_record)
        self.assertTrue(e2e.external_pr_evidence_valid(
            boundary_record, catalog_repository=e2e.TRUSTED_CATALOG_REPOSITORY,
            catalog_sha="c" * 40, snapshot=boundary_snapshot,
            snapshot_digest="sha256:" + "d" * 64,
            release_repository=e2e.TRUSTED_CLI_RELEASE_REPOSITORY,
            release_tag=e2e.TRUSTED_CLI_RELEASE_TAG, release_commit="e" * 40,
            release_manifest_digest="sha256:" + "f" * 64,
            now=now + timedelta(days=365),
        )[0])
        validator.validate(record)
        with self.assertRaises(jsonschema.ValidationError):
            validator.validate(negatives["failed_check"])
        for name in ("unsafe_directory_alias_low", "unsafe_directory_alias_high"):
            with self.subTest(schema=name), self.assertRaises(jsonschema.ValidationError):
                validator.validate(negatives[name])

    def test_authoritative_resolver_preserves_complete_targets_and_exact_fallback_reason(self) -> None:
        harness = self.fixture_harness()
        digest = lambda character: "sha256:" + character * 64
        targets = [{"client": client, "scopes": ["user"]} for client in ("codex", "cursor", "kiro")]
        harness.snapshot = {
            "sequence": 1, "evidence": [],
            "products": [{"id": "context7", "aliases": ["context7"], "default_distribution": "vendor/default", "distributions": ["vendor/default", "community/fallback"], "minimum_capabilities": {"mcp": "required"}}],
            "distributions": [
                {"id": "vendor/default", "product_id": "context7", "kind": "upstream", "status": "active", "releases": [{"sequence": 1, "components": ["mcp"], "tree_digest": digest("a"), "manifest_digest": digest("b"), "package_version": "1.0.0"}], "release_policies": [{"release_sequence": 1, "status": "active", "targets": targets[:1], "current_evidence": []}]},
                {"id": "community/fallback", "product_id": "context7", "kind": "community", "status": "active", "releases": [{"sequence": 7, "components": ["mcp"], "tree_digest": digest("c"), "manifest_digest": digest("d"), "package_version": "2.0.0"}], "release_policies": [{"release_sequence": 7, "status": "active", "targets": targets, "current_evidence": []}]},
            ],
        }
        resolved = harness.directory_release("context7", ["codex", "cursor", "kiro"])
        self.assertEqual(resolved["distribution_id"], "community/fallback")
        self.assertEqual(resolved["release_sequence"], 7)
        self.assertEqual(resolved["resolved_targets"], ["codex", "cursor", "kiro"])
        self.assertEqual(resolved["fallback_reason"], "declared default vendor/default was ineligible: release 1 does not support cursor,kiro")

    def test_fixture_privacy_output_is_derived_from_verified_consent_fields(self) -> None:
        evidence = self.fixture_harness().export()
        consent = json.loads(CONSENT.read_text())
        self.assertEqual(evidence["privacy"]["pseudonymous_identity_id"], consent["pseudonymous_identity_id"])
        self.assertEqual(evidence["privacy"]["cleanup_outcome"], consent["cleanup_outcome"])
        self.assertEqual(evidence["privacy"]["real_user_project_used"], consent["no_real_project_proof"]["real_project_accessed"])
        for field, invalid in (
            ("dedicated_identity", False), ("cleanup_outcome", "pending"),
            ("operation_mode", "write"), ("auth_origin", "copied-user-auth"),
        ):
            with self.subTest(field=field), tempfile.TemporaryDirectory() as tmp:
                path = Path(tmp) / "consent.json"
                path.write_text(json.dumps({**consent, field: invalid}))
                with self.assertRaisesRegex(ValueError, "does not authorize"):
                    e2e.LaunchHarness(None, None, mode="fixture-only", consent=path)
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "consent.json"
            path.write_text(json.dumps({**consent, "no_real_project_proof": {**consent["no_real_project_proof"], "real_project_accessed": True}}))
            with self.assertRaisesRegex(ValueError, "does not authorize"):
                e2e.LaunchHarness(None, None, mode="fixture-only", consent=path)

    def test_runtime_attestations_fail_closed_when_any_privacy_or_run_binding_changes(self) -> None:
        harness = self.fixture_harness()
        harness.challenge = {"value": "a" * 64}
        harness.github_run_id = "17"
        harness.github_run_attempt = "2"
        consent = json.loads(CONSENT.read_text())
        record = {
            "plugin": "context7", "client": "codex", "level": "runtime",
            "outcome": "failed", "reason": "fixture negative record", "tuple": {},
            "challenge": harness.challenge["value"], "run_id": "17", "run_attempt": "2",
            "scenario_id": "hero_5x3_runtime",
            "release_manifest_digest": harness.release_manifest_digest,
            "release_checksums_digest": harness.release_checksums_digest,
            "directory_digest": harness.snapshot_digest,
            "scenario_contract_digest": e2e.sha256_file(e2e.SCENARIOS),
            "identity_id": consent["pseudonymous_identity_id"],
            "consent_artifact_digest": harness.consent_digest,
            **{field: consent[field] for field in (
                "pseudonymous_identity_id", "pseudonymous_workspace_id", "dedicated_identity",
                "disposable_project_status", "operation_mode", "auth_origin", "cleanup_outcome",
                "no_real_project_proof",
            )},
        }

        def load(value):
            with tempfile.TemporaryDirectory() as tmp:
                path = Path(tmp) / "attestations.json"
                path.write_text(json.dumps({"schema_version": 1, "attestations": [value]}))
                return harness._load_attestations(path)

        self.assertIn(("context7", "codex", "runtime"), load(record))
        negatives = {
            "challenge": {**record, "challenge": "b" * 64},
            "run": {**record, "run_id": "18"},
            "scenario": {**record, "scenario_id": "chatgpt_registered_binding"},
            "identity": {**record, "identity_id": "different-identity"},
            "workspace": {**record, "pseudonymous_workspace_id": "different-workspace"},
            "cleanup": {**record, "cleanup_outcome": "pending"},
            "operation": {**record, "operation_mode": "write"},
            "auth": {**record, "auth_origin": "copied-user-auth"},
            "real_project": {**record, "no_real_project_proof": {**record["no_real_project_proof"], "real_project_accessed": True}},
            "release_binding": {**record, "release_manifest_digest": "sha256:" + "b" * 64},
        }
        for name, value in negatives.items():
            with self.subTest(name=name), self.assertRaisesRegex(ValueError, "bound|privacy|identity"):
                load(value)

    def test_chatgpt_attestation_application_id_is_bound_to_signed_directory_target(self) -> None:
        harness = self.fixture_harness()
        harness.challenge = {"value": "a" * 64}
        harness.github_run_id = "17"
        harness.github_run_attempt = "2"
        harness.snapshot_digest = "sha256:" + "d" * 64
        signed_app_id = "plugin_asdk_app_6a78e90cf73481918ef10cdb87cd4bb4"
        digest = lambda character: "sha256:" + character * 64
        chatgpt_target = {
            "client": "chatgpt", "scopes": ["user"], "delivery": "registered_app",
            "authentication": "oauth", "app_binding": {
                "app_key": "cloudflare-docs", "id": signed_app_id,
                "mcp_server": "cloudflare-docs",
            },
        }
        harness.snapshot = {
            "sequence": 19, "evidence": [],
            "products": [{
                "id": "cloudflare-docs", "aliases": ["cloudflare-docs"],
                "default_distribution": "cloudflare/cloudflare-docs",
                "distributions": ["cloudflare/cloudflare-docs"],
                "minimum_capabilities": {"mcp": "required"},
            }],
            "distributions": [{
                "id": "cloudflare/cloudflare-docs", "product_id": "cloudflare-docs",
                "kind": "upstream", "status": "active",
                "releases": [{
                    "sequence": 1, "components": ["mcp"],
                    "tree_digest": digest("b"), "manifest_digest": digest("c"),
                    "package_version": "1.0.0",
                }],
                "release_policies": [{
                    "release_sequence": 1, "status": "active",
                    "targets": [chatgpt_target], "current_evidence": [],
                }],
            }],
        }
        consent = json.loads(CONSENT.read_text())
        record = {
            "plugin": "cloudflare-docs", "client": "chatgpt", "level": "runtime",
            "outcome": "failed", "reason": "fixture negative record", "tuple": {},
            "application_id": signed_app_id,
            "challenge": harness.challenge["value"], "run_id": "17", "run_attempt": "2",
            "scenario_id": "chatgpt_registered_binding",
            "release_manifest_digest": harness.release_manifest_digest,
            "release_checksums_digest": harness.release_checksums_digest,
            "directory_digest": harness.snapshot_digest,
            "scenario_contract_digest": e2e.sha256_file(e2e.SCENARIOS),
            "identity_id": consent["pseudonymous_identity_id"],
            "consent_artifact_digest": harness.consent_digest,
            **{field: consent[field] for field in (
                "pseudonymous_identity_id", "pseudonymous_workspace_id", "dedicated_identity",
                "disposable_project_status", "operation_mode", "auth_origin", "cleanup_outcome",
                "no_real_project_proof",
            )},
        }

        def load(value):
            with tempfile.TemporaryDirectory() as tmp:
                path = Path(tmp) / "attestations.json"
                path.write_text(json.dumps({"schema_version": 1, "attestations": [value]}))
                with mock.patch.object(harness, "directory_release", return_value={
                    "distribution_id": "cloudflare/cloudflare-docs", "release_sequence": 1,
                }):
                    return harness._load_attestations(path)

        exact_pass = {**record, "outcome": "passed"}
        with mock.patch.object(harness, "directory_release", return_value={
            "distribution_id": "cloudflare/cloudflare-docs", "release_sequence": 1,
        }):
            harness.validate_signed_chatgpt_app_identity(exact_pass)
        substituted = {
            **exact_pass,
            "application_id": "plugin_asdk_app_6a92d29a704c8191931e76b47668cb0b",
        }
        with mock.patch.object(e2e.subprocess, "run") as process_effect, mock.patch.object(
            e2e, "urlopen",
        ) as network_effect, self.assertRaisesRegex(ValueError, "signed Directory target"):
            load(substituted)
        process_effect.assert_not_called()
        network_effect.assert_not_called()

        non_chatgpt = {
            **record, "plugin": "context7", "client": "codex",
            "scenario_id": "hero_5x3_runtime", "application_id": "unrelated-client-identity",
        }
        self.assertIn(("context7", "codex", "runtime"), load(non_chatgpt))
        harness.validate_signed_chatgpt_app_identity({**non_chatgpt, "outcome": "passed"})

    def test_hidden_yes_acceptance_or_mutation_fails_public_scenario(self) -> None:
        fake = '''#!/usr/bin/python3
import os, pathlib, sys
if sys.argv[1:] == ["--help"]:
    print("help")
    raise SystemExit(0)
path = pathlib.Path(os.environ["AGENTPLUGINS_HOME"]) / "state"
path.parent.mkdir(parents=True, exist_ok=True)
path.write_text("mutated")
print("accepted")
'''
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / "agentplugins"
            binary.write_text(fake)
            binary.chmod(0o700)
            workspace = root / "workspace"
            workspace.mkdir()
            with mock.patch.dict(os.environ, {"HOME": str(root / "home"), "AGENTPLUGINS_HOME": str(root / "manager")}, clear=False):
                passed, value = observer.no_hidden_yes_scenario(binary, ("cursor",), workspace, "a" * 64)
        self.assertFalse(passed)
        self.assertFalse(value["proof"]["manager_unchanged"])
        self.assertFalse(value["proof"]["unknown_option_reported"])

    def test_materialized_ledger_authenticates_current_production_identity(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "registry" / "schemas" / "1"
            snapshots = root / "snapshots"
            snapshots.mkdir(parents=True)
            shutil.copy2(PUBLICATION / "latest.json", root / "latest.json")
            snapshot = snapshots / "00000000000000000015.json"
            envelope = snapshots / "00000000000000000015.envelope.json"
            shutil.copy2(PUBLICATION / "snapshot.json", snapshot)
            shutil.copy2(PUBLICATION / "envelope-current.json", envelope)
            with mock.patch.object(
                e2e, "PRODUCTION_DIRECTORY_TRUST", PUBLICATION / "trusted-keys.json",
            ):
                identity = e2e.production_identity_from_materialized_ledger(root)
                self.assertEqual(identity, {
                    "publication_id": "fixture-1",
                    "sequence": 15,
                    "snapshot_digest": "sha256:a9b4e1ae54fcb3397269ef8aff50df794a7d431fc18ea4ffb3a102fa66a4fd60",
                    "source_commit": "d" * 40,
                })
                snapshot.unlink()
                snapshot.symlink_to(PUBLICATION / "snapshot.json")
                with self.assertRaisesRegex(ValueError, "contains a symlink"):
                    e2e.production_identity_from_materialized_ledger(root)

    def test_stale_public_pointer_is_rejected_against_caller_identity(self) -> None:
        latest = (PUBLICATION / "latest.json").read_bytes()
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(e2e, "bounded_https_get", return_value=latest):
            with self.assertRaisesRegex(ValueError, "exact caller publication identity"):
                e2e.fetch_production_directory(
                    Path(tmp) / "directory", expected_publication_id="fixture-1", expected_sequence=16,
                    expected_snapshot_digest="sha256:" + "b" * 64, expected_source_commit="d" * 40,
                )

    def test_public_directory_sequence_is_capped_before_any_network_access(self) -> None:
        arguments = {
            "expected_publication_id": "fixture-1",
            "expected_snapshot_digest": "sha256:" + "b" * 64,
            "expected_source_commit": "d" * 40,
        }
        for fetcher in (e2e.fetch_production_directory, e2e.fetch_staged_directory):
            for unsafe in (9_007_199_254_740_992, 9_007_199_254_740_993):
                with self.subTest(fetcher=fetcher.__name__, unsafe=unsafe), \
                     tempfile.TemporaryDirectory() as tmp, \
                     mock.patch.object(e2e, "bounded_https_get") as network:
                    call = {**arguments, "expected_sequence": unsafe}
                    if fetcher is e2e.fetch_staged_directory:
                        call.update(repository=e2e.TRUSTED_CATALOG_REPOSITORY,
                                    ledger_commit="e" * 40)
                    with self.assertRaisesRegex(ValueError, "identity is incomplete or invalid"):
                        fetcher(Path(tmp) / "directory", **call)
                    network.assert_not_called()
        with tempfile.TemporaryDirectory() as tmp, \
             mock.patch.object(e2e, "bounded_https_get", side_effect=RuntimeError("network seam")) as network:
            with self.assertRaisesRegex(RuntimeError, "network seam"):
                e2e.fetch_production_directory(
                    Path(tmp) / "directory", expected_sequence=9_007_199_254_740_991,
                    **arguments,
                )
            network.assert_called_once()

    def test_production_n_minus_one_does_not_block_valid_staged_n(self) -> None:
        latest = json.loads((PUBLICATION / "latest.json").read_bytes())
        production_n_minus_one = e2e.canonical_json({
            **latest,
            "sequence": 14,
            "snapshot_path": "snapshots/00000000000000000014.json",
            "envelope_path": "snapshots/00000000000000000014.envelope.json",
        })
        digest = json.loads((PUBLICATION / "envelope-current.json").read_text())["snapshot_digest"]
        ledger_commit = "e" * 40
        staged_origin = f"https://raw.githubusercontent.com/{e2e.TRUSTED_CATALOG_REPOSITORY}/{ledger_commit}/registry/schemas/1/"
        staged_bodies = {
            staged_origin + "latest.json": (PUBLICATION / "latest.json").read_bytes(),
            staged_origin + latest["snapshot_path"]: (PUBLICATION / "snapshot.json").read_bytes(),
            staged_origin + latest["envelope_path"]: (PUBLICATION / "envelope-current.json").read_bytes(),
        }
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(
            e2e, "PRODUCTION_DIRECTORY_TRUST", PUBLICATION / "trusted-keys.json"
        ), mock.patch.object(e2e, "bounded_https_get", return_value=production_n_minus_one):
            with self.assertRaisesRegex(ValueError, "exact caller publication identity"):
                e2e.fetch_production_directory(
                    Path(tmp) / "production", expected_publication_id="fixture-1", expected_sequence=15,
                    expected_snapshot_digest=digest, expected_source_commit="d" * 40,
                )
            environment, snapshot, staged_digest = e2e.fetch_staged_directory(
                Path(tmp) / "staged", repository=e2e.TRUSTED_CATALOG_REPOSITORY,
                ledger_commit=ledger_commit, expected_publication_id="fixture-1",
                expected_sequence=15, expected_snapshot_digest=digest,
                expected_source_commit="d" * 40,
                fixture_fetch=lambda url, _maximum, _accept: staged_bodies[url],
            )
            self.assertEqual(snapshot["sequence"], 15)
            self.assertEqual(staged_digest, digest)
            self.assertEqual(environment["AGENTPLUGINS_DIRECTORY_ORIGIN"], staged_origin)
            with self.assertRaisesRegex(ValueError, "differs from the exact caller publication identity"):
                e2e.fetch_staged_directory(
                    Path(tmp) / "mismatched-staged", repository=e2e.TRUSTED_CATALOG_REPOSITORY,
                    ledger_commit=ledger_commit, expected_publication_id="wrong-publication",
                    expected_sequence=15, expected_snapshot_digest=digest,
                    expected_source_commit="d" * 40,
                    fixture_fetch=lambda url, _maximum, _accept: staged_bodies[url],
                )


if __name__ == "__main__":
    unittest.main()
