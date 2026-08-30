from __future__ import annotations

import hashlib
import importlib.util
import json
import os
import copy
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
SCRIPTS = ROOT / "scripts"
sys.path.insert(0, str(ROOT))
sys.path.insert(0, str(SCRIPTS))

from scripts import observe_discovery_index, sequence_boundaries, verify_discovery_index
from directory_publication import PublicationError
import verify_directory_publication
import prepare_directory_publication
import prepare_launch_evidence
import materialize_launch_evidence
import observe_production_latest
import sign_directory_publication

OBSERVER_SPEC = importlib.util.spec_from_file_location(
    "sequence_boundary_launch_observer", SCRIPTS / "observe_launch_scenario.py",
)
assert OBSERVER_SPEC and OBSERVER_SPEC.loader
launch_observer = importlib.util.module_from_spec(OBSERVER_SPEC)
OBSERVER_SPEC.loader.exec_module(launch_observer)

PROVISION_SPEC = importlib.util.spec_from_file_location(
    "sequence_boundary_profile_provisioner", ROOT / "deploy/uap-observer-provision-profile.py",
)
assert PROVISION_SPEC and PROVISION_SPEC.loader
profile_provisioner = importlib.util.module_from_spec(PROVISION_SPEC)
PROVISION_SPEC.loader.exec_module(profile_provisioner)

MAXIMUM = sequence_boundaries.JSON_SAFE_INTEGER_MAX
ALIASES = (MAXIMUM + 1, MAXIMUM + 2)


def challenge_context(snapshot_sequence: int = MAXIMUM, scenario_id: str = "directory_offline") -> dict[str, object]:
    snapshot = json.loads((ROOT / "tests/fixtures/directory-publication/snapshot.json").read_text())
    product = snapshot["products"][0]
    distribution = next(item for item in snapshot["distributions"] if item["id"] == product["default_distribution"])
    selected = distribution["releases"][0]
    source = selected["package_source"]
    release = {
        "product_id": product["id"], "distribution_id": distribution["id"],
        "distribution_kind": distribution["kind"], "release_sequence": selected["sequence"],
        "package_version": selected["package_version"], "tree_digest": selected["tree_digest"],
        "manifest_digest": selected["manifest_digest"], "source_repository": source["repository"],
        "source_revision": source["revision"], "source_path": source["path"],
        "compatible_clients": ["codex", "cursor"], "resolved_targets": ["cursor"],
        "fallback_reason": None,
    }
    identity = {key: release[key] for key in launch_observer.CHALLENGE_SOURCE_IDENTITY_FIELDS - {"canonical_source"}}
    identity["canonical_source"] = f'https://github.com/{source["repository"]}@{source["revision"]}//{source["path"]}'
    context = {
        "nonce": "b" * 64, "github_sha": "a" * 40, "run_id": "1", "run_attempt": "1",
        "release_manifest_digest": "sha256:" + "c" * 64,
        "directory_digest": "sha256:" + "d" * 64,
        "scenario_contract_digest": "sha256:" + "e" * 64,
        "root_id": "f" * 64, "binary_digest": "sha256:" + "1" * 64,
        "expected_version": "0.1.18", "snapshot_sequence": snapshot_sequence,
        "release": release, "catalog_repository": "777genius/universal-agent-plugins",
        "directory_product": product, "directory_distribution": distribution,
        "source_identity": identity, "scenario_id": scenario_id,
    }
    framed = json.dumps({key: context[key] for key in (
        "github_sha", "run_id", "run_attempt", "release_manifest_digest", "directory_digest",
        "scenario_contract_digest", "root_id", "nonce",
    )}, sort_keys=True, separators=(",", ":")).encode()
    context["value"] = hashlib.sha256(launch_observer.CHALLENGE_DOMAIN + framed).hexdigest()
    context["context_digest"] = launch_observer.scenario_challenge_context_digest(context)
    return context


class ExactSequenceHelperTests(unittest.TestCase):
    def test_exact_values_and_canonical_text(self) -> None:
        self.assertEqual(sequence_boundaries.require_public_sequence(MAXIMUM), MAXIMUM)
        self.assertEqual(sequence_boundaries.parse_public_sequence(str(MAXIMUM)), MAXIMUM)
        for value in (True, False, 0, -1, *ALIASES):
            with self.subTest(value=value), self.assertRaises(ValueError):
                sequence_boundaries.require_public_sequence(value)
        for text in ("0", "+1", "-1", " 1", "1 ", "01", *(str(item) for item in ALIASES)):
            with self.subTest(text=text), self.assertRaises(ValueError):
                sequence_boundaries.parse_public_sequence(text)

    def test_successor_initializes_and_fails_closed_at_rollover(self) -> None:
        self.assertEqual(sequence_boundaries.next_public_sequence(None), 1)
        self.assertEqual(sequence_boundaries.next_public_sequence(MAXIMUM - 1), MAXIMUM)
        with self.assertRaises(ValueError):
            sequence_boundaries.next_public_sequence(MAXIMUM)
        self.assertEqual(sequence_boundaries.parse_padded_public_sequence(f"{MAXIMUM:020d}"), MAXIMUM)
        for text in ("1", "0" * 20, str(MAXIMUM + 1).zfill(20)):
            with self.subTest(text=text), self.assertRaises(ValueError):
                sequence_boundaries.parse_padded_public_sequence(text)

    def test_successor_cli_emits_no_sequence_at_maximum(self) -> None:
        with mock.patch.object(sys, "argv", ["sequence", "successor", "--current", str(MAXIMUM)]), mock.patch.object(
            sys, "stdout",
        ) as stdout:
            self.assertEqual(sequence_boundaries.main(), 1)
            stdout.write.assert_not_called()


class FloorBeforeIOTests(unittest.TestCase):
    def test_launch_preparer_rejects_sequence_before_filesystem_or_network_seams(self) -> None:
        base = [
            "prepare", "--asset-name", "asset", "--run-root", "run", "--output", "out",
            "--publication-id", "id", "--publication-snapshot-digest", "sha256:" + "a" * 64,
            "--publication-source-commit", "b" * 40, "--publication-ledger-commit", "c" * 40,
            "--caller-event-name", "workflow_call", "--caller-ref", "refs/heads/main",
            "--caller-workflow-ref", "owner/repo/.github/workflows/test.yml@refs/heads/main",
            "--publication-sequence",
        ]
        for text in ("01", "+1", str(MAXIMUM + 1)):
            with self.subTest(text=text), mock.patch.object(sys, "argv", [*base, text]), mock.patch.object(
                Path, "exists",
            ) as exists, mock.patch.object(prepare_launch_evidence, "read_production_config") as config, mock.patch.object(
                prepare_launch_evidence, "resolve_github_release",
            ) as network:
                with self.assertRaises(SystemExit):
                    prepare_launch_evidence.main()
                exists.assert_not_called()
                config.assert_not_called()
                network.assert_not_called()

    def test_directory_floor_clis_reject_before_candidate_key_ledger_or_output(self) -> None:
        invalid = ("01", "+1", str(MAXIMUM + 1))
        prepare_base = [
            "prepare", "--directory", "directory", "--config", "config", "--source-commit", "a" * 40,
            "--publication-id", "id", "--output", "candidate", "--digest-output", "digest",
            "--ledger-sequence-floor",
        ]
        sign_base = [
            "sign", "--candidate", "candidate", "--candidate-digest", "sha256:" + "a" * 64,
            "--config", "config", "--ledger", "ledger", "--trusted-keys", "keys", "--key-id", "key",
            "--now", "2026-08-28T00:00:00Z", "--result", "result", "--ledger-sequence-floor",
        ]
        for text in invalid:
            with self.subTest(tool="prepare", text=text), mock.patch.object(sys, "argv", [*prepare_base, text]), mock.patch.object(
                prepare_directory_publication.subprocess, "check_output",
            ) as process, mock.patch.object(prepare_directory_publication, "atomic_write") as output:
                with self.assertRaises(SystemExit):
                    prepare_directory_publication.main()
                process.assert_not_called()
                output.assert_not_called()
            with self.subTest(tool="sign", text=text), mock.patch.object(sys, "argv", [*sign_base, text]), mock.patch.object(
                sign_directory_publication, "read_bytes_bounded",
            ) as candidate, mock.patch.object(sign_directory_publication, "load_public_keys") as keys, mock.patch.object(
                sign_directory_publication, "atomic_write",
            ) as output:
                with self.assertRaises(SystemExit):
                    sign_directory_publication.main()
                candidate.assert_not_called()
                keys.assert_not_called()
                output.assert_not_called()

    def test_remaining_directory_flow_sequence_clis_reject_before_io(self) -> None:
        observe_base = [
            "observe", "--publication-id", "id", "--publication-snapshot-digest", "sha256:" + "a" * 64,
            "--publication-source-commit", "b" * 40, "--publication-ledger-commit", "c" * 40,
            "--output", "output", "--publication-sequence",
        ]
        materialize_base = [
            "materialize", "prepare-bundle", "--artifact-dir", "artifacts", "--repository", "owner/repo",
            "--workflow", "owner/repo/.github/workflows/test.yml", "--source-ref", "refs/heads/main",
            "--source-digest", "a" * 40, "--expected-run-id", "1", "--expected-run-attempt", "1",
            "--expected-caller-event-name", "workflow_call", "--expected-caller-ref", "refs/heads/main",
            "--expected-caller-workflow-ref", "owner/repo/.github/workflows/test.yml@refs/heads/main",
            "--expected-publication-id", "id", "--expected-publication-snapshot-digest", "sha256:" + "b" * 64,
            "--expected-publication-source-commit", "c" * 40, "--expected-publication-sequence",
        ]
        for text in ("01", "+1", str(MAXIMUM + 1)):
            with self.subTest(tool="observe", text=text), mock.patch.object(sys, "argv", [*observe_base, text]), mock.patch.object(
                observe_production_latest.tempfile, "TemporaryDirectory",
            ) as temporary, mock.patch.object(observe_production_latest, "fetch_production_directory") as network:
                with self.assertRaises(SystemExit):
                    observe_production_latest.main()
                temporary.assert_not_called()
                network.assert_not_called()
            with self.subTest(tool="materialize", text=text), mock.patch.object(sys, "argv", [*materialize_base, text]), mock.patch.object(
                materialize_launch_evidence, "build_bundle",
            ) as build, mock.patch.object(materialize_launch_evidence, "write_outputs") as output:
                with self.assertRaises(SystemExit):
                    materialize_launch_evidence.main()
                build.assert_not_called()
                output.assert_not_called()
    def test_observer_callable_rejects_before_opener_and_accepts_maximum(self) -> None:
        for value in (True, 0, *ALIASES):
            with self.subTest(value=value), mock.patch.object(observe_discovery_index.urllib.request, "build_opener") as opener:
                with self.assertRaises(ValueError):
                    observe_discovery_index.observe_once("https://example.invalid", Path("keys"), value)
                opener.assert_not_called()
        with mock.patch.object(observe_discovery_index.urllib.request, "build_opener", side_effect=RuntimeError("accepted")) as opener:
            with self.assertRaisesRegex(RuntimeError, "accepted"):
                observe_discovery_index.observe_once("https://example.invalid", Path("keys"), MAXIMUM)
            opener.assert_called_once()

    def test_discovery_verifier_cli_rejects_before_load_and_accepts_maximum(self) -> None:
        base = ["verify", "--feed", "feed", "--trusted-keys", "keys", "--minimum-sequence"]
        for text in ("0", "+1", " 1", str(ALIASES[0]), str(ALIASES[1])):
            with self.subTest(text=text), mock.patch.object(sys, "argv", [*base, text]), mock.patch.object(verify_discovery_index, "load_latest_portably") as load:
                with self.assertRaises(SystemExit):
                    verify_discovery_index.main()
                load.assert_not_called()
        with mock.patch.object(sys, "argv", [*base, str(MAXIMUM)]), mock.patch.object(verify_discovery_index, "load_latest_portably", side_effect=OSError("accepted")) as load:
            self.assertEqual(verify_discovery_index.main(), 1)
            load.assert_called_once()

    def test_directory_verifier_cli_rejects_before_path_read_and_accepts_maximum(self) -> None:
        base = ["verify", "--feed", "feed", "--trusted-keys", "keys", "--now", "2026-08-20T00:00:00Z", "--minimum-sequence"]
        for text in ("0", "-1", "1 ", str(ALIASES[0]), str(ALIASES[1])):
            with self.subTest(text=text), mock.patch.object(sys, "argv", [*base, text]), mock.patch.object(verify_directory_publication, "read_bytes_bounded") as read:
                with self.assertRaises(SystemExit):
                    verify_directory_publication.main()
                read.assert_not_called()
        with mock.patch.object(sys, "argv", [*base, str(MAXIMUM)]), mock.patch.object(verify_directory_publication, "read_bytes_bounded", side_effect=OSError("accepted")) as read:
            self.assertEqual(verify_directory_publication.main(), 1)
            read.assert_called_once()


class LaunchContextBoundaryTests(unittest.TestCase):
    def assert_launch_rejected_before_effects(self, context: dict[str, object], scenario: str = "directory_offline") -> None:
        with mock.patch.object(launch_observer, "observe") as observe, mock.patch.object(
            launch_observer, "directory_fault_scenario",
        ) as dispatch:
            with self.assertRaises((ValueError, PublicationError)):
                launch_observer.run(Path("binary"), scenario, Path("root"), context)
            observe.assert_not_called()
            dispatch.assert_not_called()

    def test_dispatch_accepts_maximum_and_rejects_aliases_without_observation(self) -> None:
        environment = {"HOME": "/tmp/launch-home", "AGENTPLUGINS_HOME": "/tmp/launch-manager"}
        with mock.patch.dict(os.environ, environment, clear=False), mock.patch.object(launch_observer, "observe", return_value={}) as observe, mock.patch.object(
            launch_observer, "directory_fault_scenario",
            return_value=(True, {"command_traces": [], "before": {}, "after": {}, "proof": {}}),
        ) as dispatch:
            result = launch_observer.run(Path("binary"), "directory_offline", Path("root"), challenge_context())
            self.assertEqual(result["outcome"], "passed")
            self.assertEqual(observe.call_count, 2)
            dispatch.assert_called_once()
        for value in ALIASES:
            with self.subTest(value=value), mock.patch.object(launch_observer, "observe") as observe, mock.patch.object(launch_observer, "directory_fault_scenario") as dispatch:
                with self.assertRaises(ValueError):
                    launch_observer.run(Path("binary"), "directory_offline", Path("root"), challenge_context(value))
                observe.assert_not_called()
                dispatch.assert_not_called()

    def test_duplicate_and_extra_context_fields_are_rejected(self) -> None:
        context = challenge_context()
        context["extra"] = "no"
        with self.assertRaises(ValueError):
            launch_observer.validate_challenge_context(context)
        with self.assertRaises(launch_observer.DuplicateKeyError):
            launch_observer.strict_json_loads('{"value":"a","value":"b"}')

    def test_malformed_nested_directory_values_reject_before_observation_or_dispatch(self) -> None:
        mutations = {
            "product wrong type": lambda value: value["directory_product"].update(description=7),
            "product extra": lambda value: value["directory_product"].update(extra="no"),
            "distribution missing": lambda value: value["directory_distribution"].pop("status"),
            "release source extra": lambda value: value["directory_distribution"]["releases"][0]["package_source"].update(extra="no"),
            "policy wrong type": lambda value: value["directory_distribution"]["release_policies"][0].update(targets="cursor"),
        }
        for label, mutate in mutations.items():
            context = copy.deepcopy(challenge_context())
            mutate(context)
            with self.subTest(label=label), mock.patch.object(launch_observer, "observe") as observe, mock.patch.object(
                launch_observer, "directory_fault_scenario",
            ) as dispatch:
                with self.assertRaises(Exception):
                    launch_observer.run(Path("binary"), "directory_offline", Path("root"), context)
                observe.assert_not_called()
                dispatch.assert_not_called()

    def test_cli_authenticates_nested_directory_before_run_boundary(self) -> None:
        context = challenge_context()
        context["directory_distribution"]["releases"][0]["package_source"].pop("revision")
        argv = ["observe", "--binary", "binary", "--scenario", "directory_offline", "--root", "root", "--challenge-context", "challenge", "--expected-context-digest", context["context_digest"]]
        with mock.patch.object(sys, "argv", argv), mock.patch.object(Path, "read_bytes", return_value=json.dumps(context).encode()), mock.patch.object(
            launch_observer, "run",
        ) as run:
            with self.assertRaises(Exception):
                launch_observer.main()
            run.assert_not_called()

    def test_duplicate_user_policy_client_rejects_before_effects(self) -> None:
        context = challenge_context()
        policy = context["directory_distribution"]["release_policies"][0]
        policy["targets"].append(copy.deepcopy(policy["targets"][1]))
        self.assert_launch_rejected_before_effects(context)

    def test_non_active_selected_policy_rejects_before_effects(self) -> None:
        for status in ("revoked", "superseded", "suspended"):
            context = challenge_context()
            context["directory_distribution"]["release_policies"][0]["status"] = status
            with self.subTest(status=status):
                self.assert_launch_rejected_before_effects(context)

    def test_empty_resolved_targets_rejects_before_effects(self) -> None:
        context = challenge_context()
        context["release"]["resolved_targets"] = []
        self.assert_launch_rejected_before_effects(context)

    def test_different_known_compatible_target_rejects_before_effects(self) -> None:
        context = challenge_context()
        context["release"]["resolved_targets"] = ["codex"]
        self.assert_launch_rejected_before_effects(context)

    def test_one_and_multi_target_scenario_bindings_are_accepted(self) -> None:
        environment = {"HOME": "/tmp/launch-home", "AGENTPLUGINS_HOME": "/tmp/launch-manager"}
        one = challenge_context()
        with mock.patch.dict(os.environ, environment, clear=False), mock.patch.object(
            launch_observer, "observe", return_value={},
        ), mock.patch.object(
            launch_observer, "directory_fault_scenario",
            return_value=(True, {"command_traces": [], "before": {}, "after": {}, "proof": {}}),
        ) as dispatch:
            self.assertEqual(
                launch_observer.run(Path("binary"), "directory_offline", Path("root"), one)["outcome"],
                "passed",
            )
            dispatch.assert_called_once()

        sticky_value = {
            "command_traces": [{
                "argv": ["repair", "context7", "--target", "cursor", "--format", "json"],
                "started_at": "2026-08-28T00:00:00Z",
            }],
            "before": {}, "after": {},
            "proof": {
                "recorded_distribution_retained": True,
                "recorded_revision_retained": True,
            },
        }
        with mock.patch.dict(os.environ, environment, clear=False), mock.patch.object(
            launch_observer, "observe", return_value={},
        ), mock.patch.object(
            launch_observer, "sticky_scenario", return_value=(True, sticky_value),
        ) as sticky_dispatch:
            result = launch_observer.run(
                Path("binary"), "repair_sticky_distribution", Path("root"), challenge_context(scenario_id="repair_sticky_distribution"),
            )
            self.assertEqual(result["outcome"], "passed")
            sticky_dispatch.assert_called_once()

        multi = challenge_context()
        policy = multi["directory_distribution"]["release_policies"][0]
        policy["targets"].append({**copy.deepcopy(policy["targets"][0]), "client": "kiro"})
        multi["release"]["compatible_clients"] = ["codex", "cursor", "kiro"]
        multi["release"]["resolved_targets"] = ["codex", "cursor", "kiro"]
        multi["scenario_id"] = "context7_grouped_lifecycle"
        multi["context_digest"] = launch_observer.scenario_challenge_context_digest(multi)
        lifecycle_value = {
            "command_traces": [], "values": {}, "operation_observations": [],
            "operation_outcomes": {}, "identities": {}, "tuple": {},
        }
        with mock.patch.dict(os.environ, environment, clear=False), mock.patch.object(
            launch_observer, "observe", return_value={},
        ), mock.patch.object(
            launch_observer, "lifecycle", return_value=(False, lifecycle_value),
        ) as dispatch:
            launch_observer.run(Path("binary"), "context7_grouped_lifecycle", Path("root"), multi)
            dispatch.assert_called_once()

    def test_direct_run_and_cli_share_scenario_target_guard(self) -> None:
        context = challenge_context()
        context["release"]["resolved_targets"] = ["codex"]
        context["context_digest"] = launch_observer.scenario_challenge_context_digest(context)
        self.assert_launch_rejected_before_effects(context)

        argv = [
            "observe", "--binary", "binary", "--scenario", "directory_offline",
            "--root", "root", "--challenge-context", "challenge",
            "--expected-context-digest", context["context_digest"],
        ]
        with mock.patch.object(sys, "argv", argv), mock.patch.object(
            Path, "read_bytes", return_value=json.dumps(context).encode(),
        ), mock.patch.object(launch_observer, "run") as run, mock.patch.object(
            launch_observer, "observe",
        ) as observe:
            with self.assertRaisesRegex(ValueError, "requested scenario"):
                launch_observer.main()
            run.assert_not_called()
            observe.assert_not_called()


class PrivilegedTupleBoundaryTests(unittest.TestCase):
    def test_unsafe_source_tuple_rejects_before_mutable_profile_roots(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            seed = Path(temporary) / "seed"
            proof = seed / profile_provisioner.PROOF_SEED_NAME
            proof.mkdir(parents=True)
            release_tuple = {
                field: "value" for field in profile_provisioner.TUPLE_FIELDS
                if field not in {"release_sequence", "snapshot_sequence", "client_version"}
            }
            release_tuple.update({
                "product_id": "context7", "release_sequence": MAXIMUM + 1,
                "snapshot_sequence": 1, "client_version": None,
                "source_revision": "a" * 40, "source_repository": "owner/repo", "source_path": "plugin",
                **{field: "sha256:" + "b" * 64 for field in profile_provisioner.TUPLE_DIGEST_FIELDS},
            })
            digest = "sha256:" + "c" * 64
            entry = {
                "plugin": "context7", "component_kind": "mcp", "tuple": release_tuple,
                "native_config": {"path": "/var/lib/uap-observer/proofs/codex/native/context7.blob", "sha256": digest},
                "client_config": {"path": "/var/lib/uap-observer/profiles/codex/context7.json", "sha256": digest},
                "manager_add_sha256": digest, "manager_info_sha256": digest, "post_add_doctor_sha256": digest,
            }
            (proof / "native-projection.json").write_text(json.dumps({
                "schema_version": 2, "client_id": "codex", "entries": [entry],
            }))
            (proof / "receipts.json").write_text("{}")
            real_stat = profile_provisioner.os.stat
            real_fstat = profile_provisioner.os.fstat

            def protected(info: os.stat_result) -> os.stat_result:
                values = list(info)
                values[4] = 0
                return os.stat_result(values, {
                    "st_atime_ns": info.st_atime_ns,
                    "st_mtime_ns": info.st_mtime_ns,
                    "st_ctime_ns": info.st_ctime_ns,
                })

            # This fixture isolates sequence validation; ownership behavior is tested separately.
            with mock.patch.object(
                profile_provisioner.os, "stat",
                side_effect=lambda *args, **kwargs: protected(real_stat(*args, **kwargs)),
            ), mock.patch.object(
                profile_provisioner.os, "fstat",
                side_effect=lambda *args, **kwargs: protected(real_fstat(*args, **kwargs)),
            ):
                framed = hashlib.sha256(b"uap-observer-profile-seed-v1\0")
                source_fd = os.open(seed, profile_provisioner.OPEN_DIRECTORY)
                try:
                    profile_provisioner.copy_tree(source_fd, None, framed)
                finally:
                    os.close(source_fd)
                expected_digest = "sha256:" + framed.hexdigest()
                main_source_fd = os.open(seed, profile_provisioner.OPEN_DIRECTORY)
                argv = ["provision", "--client", "codex", "--root-owned-seed", str(seed), "--seed-digest", expected_digest]
                account = mock.Mock(pw_uid=123, pw_gid=456)
                with mock.patch.object(sys, "argv", argv), mock.patch.object(profile_provisioner.os, "geteuid", return_value=0), mock.patch.object(
                    profile_provisioner.pwd, "getpwnam", return_value=account,
                ), mock.patch.object(profile_provisioner, "open_root_owned_directory", return_value=main_source_fd) as open_root, mock.patch.object(
                    profile_provisioner, "write_transaction",
                ) as transaction:
                    with self.assertRaisesRegex(ValueError, "sequence"):
                        profile_provisioner.main()
                    open_root.assert_called_once_with(seed)
                    transaction.assert_not_called()


class WorkflowSequenceContractTests(unittest.TestCase):
    def test_workflow_uses_reviewed_successor_without_bash_increment(self) -> None:
        workflow = (ROOT / ".github/workflows/discovery-index.yml").read_text()
        self.assertIn("scripts/sequence_boundaries.py successor", workflow)
        self.assertNotIn("$((sequence + 1))", workflow)
        self.assertNotIn("sequence=0", workflow)


if __name__ == "__main__":
    unittest.main()
