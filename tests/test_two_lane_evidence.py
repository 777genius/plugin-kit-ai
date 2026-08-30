from __future__ import annotations

import copy
import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

import materialize_launch_evidence as materialize
import run_launch_evidence_e2e as launch
import run_source_policy_conformance as producer
import two_lane_evidence as lanes


def digest(character: str = "a") -> str:
    return "sha256:" + character * 64


FIXTURE_UAP_SHA = "b" * 40
FIXTURE_LEDGER_SHA = "a" * 40
FIXTURE_PUBLICATION_ID = "fixture-publication"
FIXTURE_SOURCE_COMMIT = "c" * 40


def runtime_evidence(passed: int = 15, *, schema_version: int = 4) -> dict:
    current = schema_version == 5
    release_version = "0.1.24" if current else "0.1.18"
    release_tag = lanes.PLUGIN_KIT_TAG if current else lanes.V4_PLUGIN_KIT_TAG
    release_commit = lanes.PLUGIN_KIT_COMMIT if current else lanes.V4_PLUGIN_KIT_COMMIT
    binary_digest = lanes.RELEASED_LINUX_AMD64_DIGEST if current else lanes.V4_RELEASED_LINUX_AMD64_DIGEST
    manifest_digest = lanes.RELEASE_MANIFEST_DIGEST if current else lanes.V4_RELEASE_MANIFEST_DIGEST
    checksums_digest = lanes.RELEASE_CHECKSUMS_DIGEST if current else lanes.V4_RELEASE_CHECKSUMS_DIGEST
    rows = []
    pairs = ((plugin, client) for plugin in lanes.HERO_PLUGINS for client in lanes.RUNTIME_CLIENTS)
    for ordinal, (plugin, client) in enumerate(pairs):
        tuple_value = {
            "product_id": plugin, "tree_digest": digest("1"), "manifest_digest": digest("2"),
            "distribution_id": f"fixture/{plugin}", "distribution_kind": "community",
            "release_sequence": 1, "package_version": "1.0.0", "source_repository": "fixture/repository",
            "source_revision": "d" * 40, "source_path": f"plugins/{plugin}", "snapshot_sequence": 1,
            "snapshot_digest": digest("a"), "binary_digest": binary_digest,
            "dependency_identity": "fixture", "installer_version": release_version, "adapter_version": release_version,
            "client_version": "1.0.0", "os": "linux", "architecture": "amd64",
            "observed_at": "2026-08-28T00:00:00Z",
        }
        rows.append({
            "id": f"{ordinal:024x}", "scenario": "hero_5x3_runtime", "plugin": plugin, "client": client,
            "level": "runtime", "outcome": "passed" if ordinal < passed else "failed", "tuple": tuple_value,
            "reason": "fixture", "details": {"evidence_basis": "protected_external_observer",
                "runtime_proof": True, "native_discovery_proof": True,
                "release_manifest_digest": manifest_digest,
                "release_checksums_digest": checksums_digest,
                "directory_digest": digest("a"), "scenario_id": "hero_5x3_runtime",
                "native_discovery_evidence": {}},
        })
    if current:
        tuple_value = copy.deepcopy(rows[0]["tuple"])
        tuple_value.update({
            "product_id": "cloudflare-docs", "distribution_id": "fixture/cloudflare-docs",
            "source_path": "plugins/cloudflare-docs",
        })
        rows.append({
            "id": f"{len(rows):024x}", "scenario": "chatgpt_registered_binding",
            "plugin": "cloudflare-docs", "client": "chatgpt", "level": "runtime",
            "outcome": "passed", "tuple": tuple_value, "reason": "fixture",
            "details": {
                "evidence_basis": "protected_external_observer", "runtime_proof": True,
                "native_discovery_proof": False, "public_mcp_proof": True,
                "release_manifest_digest": manifest_digest,
                "release_checksums_digest": checksums_digest,
                "directory_digest": digest("a"), "scenario_id": "chatgpt_registered_binding",
                "public_mcp_evidence": {},
            },
        })
    return {
        "schema_version": schema_version,
        "evidence_class": "released_binary",
        "run": {"id": "1" * 16, "mode": "enforced", "runtime_claims": True, "github_sha": FIXTURE_UAP_SHA,
                "observed_at": "2026-08-28T00:00:00Z", "platform": "linux", "architecture": "amd64",
                "disposable": True, "root_id": "2" * 16, "github_run_id": "1", "github_run_attempt": "1",
                "caller_event_name": "push", "caller_ref": "refs/heads/main",
                "caller_workflow_ref": "fixture/repository/.github/workflows/directory-publication.yml@refs/heads/main",
                "challenge": "3" * 64, "observer_bundle_digest": digest("4"),
                "cli": {"available": True, "version": release_version, "binary_digest": binary_digest}},
        "release": {"repository": lanes.PLUGIN_KIT_REPOSITORY, "tag": release_tag,
                    "tag_commit": release_commit, "release_id": 379284682 if current else 1, "immutable": True,
                    "manifest_digest": manifest_digest,
                    "checksums_digest": checksums_digest},
        "directory": {"origin": "https://raw.githubusercontent.com/777genius/universal-agent-plugins/" + FIXTURE_LEDGER_SHA + "/registry/schemas/1/",
                      "ledger_commit": FIXTURE_LEDGER_SHA, "publication_id": FIXTURE_PUBLICATION_ID,
                      "source_commit": FIXTURE_SOURCE_COMMIT, "sequence": 1,
                      "snapshot_digest": digest("a"), "trust_root_digest": digest("b")},
        "scenario_contract": {"id": "acceptance-26.1-stable-release-v2", "digest": digest("c"),
            "expected_ids": [f"acceptance-{i}" for i in range(10)],
            "required_singleton_ids": [f"singleton-{i}" for i in range(27)],
            "expected_counts": {f"count-{i}": 1 for i in range(9)}},
        "summary": {
            "passed": passed + (1 if current else 0), "failed": 15 - passed, "inconclusive": 0, "not_applicable": 0,
            "required_gates_complete": passed == 15,
            "released_binary_gate_complete": passed == 15,
            "hero_runtime_results": passed,
        },
        "matrix": rows,
        "privacy": {"redacted_export": True, "consent_artifact_digest": digest("6"),
            "pseudonymous_identity_id": "fixture-id", "pseudonymous_workspace_id": "fixture-workspace",
            "dedicated_identity": True, "disposable_project_status": "disposed", "operation_mode": "synthetic",
            "auth_origin": "none", "cleanup_outcome": "cleaned", "contains_absolute_home_paths": False,
            "contains_credentials": False, "real_user_project_used": False, "auth_copied": False},
    }


def policy_evidence(passed: int = 11, *, schema_version: int = 2) -> dict:
    current = schema_version == 2
    identities = {
        "plugin_kit_repository": lanes.PLUGIN_KIT_REPOSITORY,
        "plugin_kit_tag": lanes.PLUGIN_KIT_TAG if current else lanes.V4_PLUGIN_KIT_TAG,
        "plugin_kit_commit": lanes.PLUGIN_KIT_COMMIT if current else lanes.V4_PLUGIN_KIT_COMMIT,
        "release_manifest_digest": lanes.RELEASE_MANIFEST_DIGEST if current else lanes.V4_RELEASE_MANIFEST_DIGEST,
        "release_checksums_digest": lanes.RELEASE_CHECKSUMS_DIGEST if current else lanes.V4_RELEASE_CHECKSUMS_DIGEST,
        "uap_sha": FIXTURE_UAP_SHA,
        "scenario_digest": digest("3"),
        "harness_digest": digest("4"),
        "overlay_digest": digest("5"),
        "production_source_tree_before": lanes.PLUGIN_KIT_PRODUCTION_TREE_DIGEST if current else lanes.V4_PLUGIN_KIT_PRODUCTION_TREE_DIGEST,
        "production_source_tree_after": lanes.PLUGIN_KIT_PRODUCTION_TREE_DIGEST if current else lanes.V4_PLUGIN_KIT_PRODUCTION_TREE_DIGEST,
    }
    results = []
    harness = json.loads((ROOT / "tests/e2e/source-policy-tests.json").read_text())["tests"]
    for index, scenario_id in enumerate(lanes.POLICY_SCENARIO_IDS):
        test = {**harness[scenario_id], "passed": index < passed, "transcript_digest": digest("9")}
        proof = {"id": scenario_id, "source_test": test, "fixture_key_id": lanes.FIXTURE_KEY_ID,
                 "overlay_digest": digest("5"), "runtime_evidence_eligible": False}
        if scenario_id == "revoked_operations_boundary":
            stderr = (
                "Resolving and validating one Agent Plugin package for every selected target...\n"
                "agentplugins: no eligible directory release for \"context7\": "
                "upstash/context7: release 1 is revoked\n"
            )
            proof["unit_oracle"] = {
                "argv": ["add", "context7", "--target", "codex", "--format", "json"],
                "exit_code": 1, "stdout_digest": lanes.sha256(b""),
                "stderr_digest": lanes.sha256(stderr.encode()), "zero_mutation": True,
                "runtime_evidence_eligible": False,
            }
        results.append({
            "id": scenario_id,
            "outcome": "passed" if index < passed else "failed",
            "test": test,
            "proof": proof,
            "proof_digest": lanes.sha256(lanes.canonical_json(proof)),
        })
    return {
        "schema_version": schema_version,
        "evidence_class": "source_policy_conformance",
        "runtime_claims": False,
        "released_binary_executed": False,
        "fixture_key_id": lanes.FIXTURE_KEY_ID,
        "identities": identities,
        "results": results,
        "production_source_unchanged": True,
        "policy_conformance_gate_complete": passed == 11,
    }


class TwoLaneEvidenceTests(unittest.TestCase):
    def identity(self) -> dict[str, object]:
        return {
            "scenario_digest": digest("3"), "harness_digest": digest("4"),
            "overlay_digest": digest("5"), "uap_sha": FIXTURE_UAP_SHA,
            "directory_ledger_sha": FIXTURE_LEDGER_SHA,
            "publication_id": FIXTURE_PUBLICATION_ID, "publication_sequence": 1,
            "publication_snapshot_digest": digest("a"),
            "publication_source_commit": FIXTURE_SOURCE_COMMIT,
        }

    def policy_identity(self) -> dict[str, str]:
        return {
            key: value for key, value in self.identity().items()
            if key in {"scenario_digest", "harness_digest", "overlay_digest", "uap_sha"}
        }

    def test_exact_policy_set_rejects_missing_duplicate_renamed_and_extra(self) -> None:
        baseline = policy_evidence()
        lanes.validate_source_policy_evidence(baseline, **self.policy_identity())
        mutations = []
        missing = copy.deepcopy(baseline); missing["results"].pop(); mutations.append(missing)
        duplicate = copy.deepcopy(baseline); duplicate["results"][-1]["id"] = duplicate["results"][0]["id"]; mutations.append(duplicate)
        renamed = copy.deepcopy(baseline); renamed["results"][0]["id"] += "_renamed"; mutations.append(renamed)
        extra = copy.deepcopy(baseline); extra["results"].append(copy.deepcopy(extra["results"][0])); extra["results"][-1]["id"] = "extra"; mutations.append(extra)
        for value in mutations:
            with self.subTest(ids=[row["id"] for row in value["results"]]):
                with self.assertRaises(lanes.TwoLaneEvidenceError):
                    lanes.validate_source_policy_evidence(value, **self.policy_identity())

    def test_readiness_requires_15_runtime_and_11_policy(self) -> None:
        complete = lanes.build_readiness_envelope(runtime_evidence(schema_version=5), policy_evidence(), **self.identity())
        self.assertTrue(complete["readiness_gate_complete"])
        self.assertEqual((complete["schema_version"], complete["runtime_results"]), (2, 16))
        for runtime, policy in ((runtime_evidence(schema_version=5), policy_evidence(10)), (runtime_evidence(14, schema_version=5), policy_evidence())):
            with self.assertRaises(lanes.TwoLaneEvidenceError):
                lanes.build_readiness_envelope(runtime, policy, **self.identity())

    def test_historical_v4_replay_keeps_v1_sidecars_but_cannot_authorize_current(self) -> None:
        runtime = runtime_evidence(schema_version=4)
        policy = policy_evidence(schema_version=1)
        complete = lanes.build_readiness_envelope(
            runtime, policy, schema_version=1, purpose="historical", **self.identity(),
        )
        self.assertEqual((complete["schema_version"], complete["runtime_results"]), (1, 15))
        lanes.validate_completed_readiness(
            complete, runtime, policy, schema_version=1, purpose="historical", **self.identity(),
        )
        for schema_version, candidate_policy in (
            (1, policy), (2, policy_evidence()),
        ):
            with self.subTest(schema_version=schema_version), self.assertRaises(lanes.TwoLaneEvidenceError):
                lanes.build_readiness_envelope(
                    runtime, candidate_policy, schema_version=schema_version,
                    purpose="current", **self.identity(),
                )
        invalid_policy = copy.deepcopy(policy)
        invalid_policy["schema_version"] = True
        with self.assertRaisesRegex(lanes.TwoLaneEvidenceError, "schema_version"):
            lanes.validate_source_policy_evidence(
                invalid_policy, expected_schema_version=1, **self.policy_identity(),
            )
        with self.assertRaisesRegex(lanes.TwoLaneEvidenceError, "caller-selected"):
            lanes.build_readiness_envelope(
                runtime, policy, schema_version=True, purpose="historical", **self.identity(),
            )

    def test_uap_and_directory_publication_identities_are_distinct_and_exact(self) -> None:
        runtime = runtime_evidence(schema_version=5)
        complete = lanes.build_readiness_envelope(runtime, policy_evidence(), **self.identity())
        self.assertNotEqual(complete["uap_sha"], complete["directory_ledger_sha"])
        substitutions = (
            {"directory_ledger_sha": FIXTURE_UAP_SHA},
            {"publication_id": "substituted"},
            {"publication_sequence": 2},
            {"publication_snapshot_digest": digest("f")},
            {"publication_source_commit": FIXTURE_UAP_SHA},
        )
        for replacement in substitutions:
            identity = {**self.identity(), **replacement}
            with self.subTest(replacement=replacement), self.assertRaises(lanes.TwoLaneEvidenceError):
                lanes.build_readiness_envelope(runtime, policy_evidence(), **identity)
        forged = copy.deepcopy(runtime)
        forged["directory"]["origin"] = (
            "https://raw.githubusercontent.com/777genius/universal-agent-plugins/"
            + FIXTURE_UAP_SHA + "/registry/schemas/1/"
        )
        with self.assertRaises(lanes.TwoLaneEvidenceError):
            lanes.build_readiness_envelope(forged, policy_evidence(), **self.identity())

        # A self-consistent forgery still cannot collapse the two authorities:
        # both supplied identities and both runtime Directory fields agree here.
        collapsed = copy.deepcopy(runtime)
        collapsed["directory"]["ledger_commit"] = FIXTURE_UAP_SHA
        collapsed["directory"]["origin"] = (
            "https://raw.githubusercontent.com/777genius/universal-agent-plugins/"
            + FIXTURE_UAP_SHA + "/registry/schemas/1/"
        )
        collapsed_identity = {
            **self.identity(), "directory_ledger_sha": FIXTURE_UAP_SHA,
        }
        runtime_identity = {
            key: collapsed_identity[key] for key in (
                "uap_sha", "directory_ledger_sha", "publication_id",
                "publication_sequence", "publication_snapshot_digest",
                "publication_source_commit",
            )
        }
        with self.assertRaisesRegex(lanes.TwoLaneEvidenceError, "must differ"):
            lanes.validate_released_binary_evidence(collapsed, **runtime_identity)
        with self.assertRaisesRegex(lanes.TwoLaneEvidenceError, "must differ"):
            lanes.build_readiness_envelope(
                collapsed, policy_evidence(), **collapsed_identity,
            )
        completed = copy.deepcopy(complete)
        completed["directory_ledger_sha"] = FIXTURE_UAP_SHA
        with self.assertRaisesRegex(lanes.TwoLaneEvidenceError, "must differ"):
            lanes.validate_completed_readiness(
                completed, collapsed, policy_evidence(), **collapsed_identity,
            )

    def test_runtime_rejects_arbitrary_fifteen_rows_and_identity_forgery(self) -> None:
        baseline = runtime_evidence(schema_version=5)
        mutations = []
        duplicate = copy.deepcopy(baseline)
        duplicate["matrix"][0]["plugin"] = duplicate["matrix"][1]["plugin"]
        duplicate["matrix"][0]["client"] = duplicate["matrix"][1]["client"]
        mutations.append(duplicate)
        wrong_outcome = copy.deepcopy(baseline); wrong_outcome["matrix"][0]["outcome"] = "failed"; mutations.append(wrong_outcome)
        wrong_tuple = copy.deepcopy(baseline); wrong_tuple["matrix"][0]["tuple"]["binary_digest"] = digest("7"); mutations.append(wrong_tuple)
        policy_row = copy.deepcopy(baseline); policy_row["matrix"][0]["scenario"] = lanes.POLICY_SCENARIO_IDS[0]; mutations.append(policy_row)
        wrong_sha = copy.deepcopy(baseline); wrong_sha["run"]["github_sha"] = "c" * 40; mutations.append(wrong_sha)
        import jsonschema
        schema = json.loads((ROOT / "tests/e2e/schemas/launch-evidence-v5.schema.json").read_text())
        for value in mutations[:4]:
            with self.assertRaises(jsonschema.ValidationError):
                jsonschema.Draft202012Validator(schema).validate(value)
        for value in mutations:
            with self.subTest(mutation=mutations.index(value)), self.assertRaises(lanes.TwoLaneEvidenceError):
                lanes.build_readiness_envelope(value, policy_evidence(), **self.identity())

    def test_policy_rejects_canonical_mapping_proof_and_tree_defects(self) -> None:
        baseline = policy_evidence()
        mutations = []
        wrong_package = copy.deepcopy(baseline); wrong_package["results"][0]["test"]["package"] = "./attacker"; mutations.append(wrong_package)
        mismatched_test = copy.deepcopy(baseline); mismatched_test["results"][0]["proof"]["source_test"] = {**mismatched_test["results"][0]["test"], "name": "Other"}; mutations.append(mismatched_test)
        bad_transcript = copy.deepcopy(baseline); bad_transcript["results"][0]["test"]["transcript_digest"] = "bad"; mutations.append(bad_transcript)
        changed_tree = copy.deepcopy(baseline); changed_tree["identities"]["production_source_tree_after"] = digest("8"); mutations.append(changed_tree)
        import jsonschema
        schema = json.loads((ROOT / "schemas/e2e/source-policy-conformance-v2.schema.json").read_text())
        for value in (wrong_package, bad_transcript, changed_tree):
            with self.assertRaises(jsonschema.ValidationError):
                jsonschema.Draft202012Validator(schema).validate(value)
        for value in mutations:
            with self.assertRaises(lanes.TwoLaneEvidenceError):
                lanes.validate_source_policy_evidence(value, **self.policy_identity())

    def test_current_readiness_rejects_policy_shape_outside_v2_schema(self) -> None:
        mutations = []
        root_extra = copy.deepcopy(policy_evidence())
        root_extra["unexpected"] = True
        mutations.append(root_extra)
        nested_extra = copy.deepcopy(policy_evidence())
        nested_extra["identities"]["unexpected"] = True
        mutations.append(nested_extra)
        malformed_details = copy.deepcopy(policy_evidence())
        malformed_details["results"][0]["details"] = {"claimed": True}
        mutations.append(malformed_details)
        for policy in mutations:
            with self.subTest(policy=mutations.index(policy)), self.assertRaisesRegex(
                lanes.TwoLaneEvidenceError, "v2 schema mismatch",
            ):
                lanes.build_readiness_envelope(
                    runtime_evidence(schema_version=5), policy, **self.identity(),
                )

        # Frozen v1 remains governed by its historical semantic validator.
        historical = policy_evidence(schema_version=1)
        historical["historical_annotation"] = "preserved"
        lanes.build_readiness_envelope(
            runtime_evidence(schema_version=4), historical,
            schema_version=1, purpose="historical", **self.identity(),
        )

    def test_schema_versions_route_without_reinterpreting_v3(self) -> None:
        v3 = runtime_evidence(schema_version=4)
        v3["schema_version"] = 3
        v3.pop("evidence_class")
        for field in ("ledger_commit", "publication_id", "source_commit"):
            v3["directory"].pop(field)
        v3["scenario_contract"]["expected_ids"] = [f"acceptance-{i}" for i in range(13)]
        v3["scenario_contract"]["required_singleton_ids"] = [f"singleton-{i}" for i in range(38)]
        v3["summary"].pop("released_binary_gate_complete")
        lanes.validate_launch_schema(v3, purpose="historical")
        with self.assertRaisesRegex(lanes.TwoLaneEvidenceError, "current"):
            lanes.validate_launch_schema(v3)
        v4 = runtime_evidence(schema_version=4)
        lanes.validate_launch_schema(v4, purpose="historical")
        with self.assertRaisesRegex(lanes.TwoLaneEvidenceError, "current"):
            lanes.validate_launch_schema(v4)
        lanes.validate_launch_schema(runtime_evidence(schema_version=5), purpose="current")
        unknown = copy.deepcopy(v3); unknown["schema_version"] = 6
        with self.assertRaisesRegex(lanes.TwoLaneEvidenceError, "unknown"):
            lanes.validate_launch_schema(unknown, purpose="historical")

    def test_all_two_lane_clis_require_explicit_uap_sha(self) -> None:
        scripts = (
            "run_launch_evidence_e2e.py", "run_source_policy_conformance.py",
            "build_two_lane_readiness.py", "materialize_launch_evidence.py",
        )
        for name in scripts:
            body = (ROOT / "scripts" / name).read_text()
            with self.subTest(script=name):
                self.assertRegex(body, r'add_argument\("--uap-sha", required=True(?:,|\))')
                self.assertNotIn("b37eda9a710b4e41bde3cc27ada56dd3b17edc40", body)

    def test_completed_replay_rejects_either_digest_mutation(self) -> None:
        runtime, policy = runtime_evidence(schema_version=5), policy_evidence()
        complete = lanes.build_readiness_envelope(runtime, policy, **self.identity())
        lanes.validate_completed_readiness(complete, runtime, policy, **self.identity())
        runtime_changed = copy.deepcopy(runtime); runtime_changed["matrix"][0]["note"] = "changed"
        policy_changed = copy.deepcopy(policy); policy_changed["results"][0]["proof_digest"] = digest("7")
        for left, right in ((runtime_changed, policy), (runtime, policy_changed)):
            with self.assertRaises(lanes.TwoLaneEvidenceError):
                lanes.validate_completed_readiness(complete, left, right, **self.identity())

    def test_wrong_source_identities_and_digests_reject(self) -> None:
        for field, replacement in (
            ("plugin_kit_repository", "attacker/repo"), ("plugin_kit_tag", "agentplugins-v0.1.17"),
            ("plugin_kit_commit", "0" * 40), ("release_manifest_digest", "bad"),
            ("release_checksums_digest", "bad"), ("uap_sha", "0" * 40),
            ("scenario_digest", digest("8")), ("harness_digest", digest("8")),
            ("overlay_digest", digest("8")),
        ):
            value = policy_evidence(); value["identities"][field] = replacement
            with self.subTest(field=field), self.assertRaises(lanes.TwoLaneEvidenceError):
                lanes.validate_source_policy_evidence(value, **self.policy_identity())

    def test_policy_preflight_has_no_path_or_process_effect(self) -> None:
        harness = object.__new__(launch.LaunchHarness)
        with mock.patch.object(launch.LaunchHarness, "fresh_sandbox") as sandbox, \
             mock.patch.object(launch.subprocess, "run") as process:
            for scenario_id in lanes.POLICY_SCENARIO_IDS:
                with self.assertRaises(lanes.TwoLaneEvidenceError):
                    harness.driven_scenario(scenario_id)
        sandbox.assert_not_called()
        process.assert_not_called()

    def test_invalid_caller_directory_overrides_are_ignored(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            sandbox = Path(temporary) / "run"
            source = Path(temporary) / "verified"
            source.mkdir()
            paths = {}
            for name in ("snapshot", "envelope", "trust"):
                path = source / f"{name}.json"; path.write_text("{}")
                paths[name] = str(path)
            supplied = {
                "AGENTPLUGINS_DIRECTORY_ORIGIN": "https://raw.githubusercontent.com/777genius/universal-agent-plugins/" + "a" * 40 + "/registry/schemas/1/",
                "AGENTPLUGINS_DIRECTORY_SNAPSHOT": paths["snapshot"],
                "AGENTPLUGINS_DIRECTORY_ENVELOPE": paths["envelope"],
                "AGENTPLUGINS_DIRECTORY_TRUST": paths["trust"],
            }
            poison = {
                "AGENTPLUGINS_DIRECTORY_SNAPSHOT": "/caller/invalid-snapshot",
                "AGENTPLUGINS_DIRECTORY_TRUST": "/caller/invalid-trust",
                "AGENTPLUGINS_DIRECTORY_CACHE": "/caller/invalid-cache",
            }
            with mock.patch.dict(launch.os.environ, poison, clear=False):
                environment = launch.isolated_environment(sandbox, ("cursor",), supplied)
            self.assertEqual(environment["AGENTPLUGINS_DIRECTORY_ORIGIN"], supplied["AGENTPLUGINS_DIRECTORY_ORIGIN"])
            self.assertFalse(set(poison) & set(environment))
            for key in ("HOME", "AGENTPLUGINS_HOME", "XDG_CACHE_HOME", "TMPDIR"):
                self.assertIn(sandbox.resolve(), Path(environment[key]).resolve().parents)

    def test_directory_rejects_policy_artifacts_and_rows(self) -> None:
        with self.assertRaisesRegex(materialize.EvidenceError, "released-binary"):
            materialize.selected_rows({"evidence_class": "source_policy_conformance", "matrix": []})
        value = runtime_evidence()
        value["matrix"] = [{"scenario": lanes.POLICY_SCENARIO_IDS[0]}]
        with self.assertRaisesRegex(materialize.EvidenceError, "source-policy"):
            materialize.selected_rows(value)

    def test_count_contract_is_derived_from_7_plus_3_plus_11(self) -> None:
        config = json.loads((ROOT / "tests/e2e/launch-scenarios.json").read_text())
        self.assertEqual([len(config[key]) for key in ("fault_scenarios", "adapter_repair_faults", "advanced_scenarios")], [7, 3, 11])
        self.assertEqual(len(lanes.classified_runtime_lists(config)["fault_adapter_advanced"]), 13)
        self.assertEqual(config["expected_counts"]["fault_rows"], 13)

    def test_source_producer_binds_exact_clean_source_without_modifying_it(self) -> None:
        source = ROOT / ".runtime-reference/plugin-kit-ai"
        if not source.is_dir():
            self.skipTest("exact read-only plugin-kit source checkout is unavailable")
        with tempfile.TemporaryDirectory() as temporary:
            manifest = Path(temporary) / "release-manifest.json"
            checksums = Path(temporary) / "checksums.txt"
            manifest.write_text(json.dumps({
                "repository": lanes.PLUGIN_KIT_REPOSITORY, "tag": lanes.PLUGIN_KIT_TAG,
                "commit": lanes.PLUGIN_KIT_COMMIT, "version": "0.1.24",
            }))
            checksums.write_text("9a294d2d117d6be2042aa28f911999edccf051ccbc3f1c7f0f46920cfd6b5779  agentplugins_0.1.18_linux_amd64\n")
            fake = lambda source, package, name, go, **kwargs: {
                "package": package, "name": name, "passed": True,
                "transcript_digest": digest("d"),
            }
            with mock.patch.object(producer, "run_test", side_effect=fake), mock.patch.object(
                producer, "validate_release_identity",
                return_value=(lanes.RELEASE_MANIFEST_DIGEST, lanes.RELEASE_CHECKSUMS_DIGEST),
            ):
                value = producer.produce(
                    source, manifest, checksums, go="go", uap_sha=FIXTURE_UAP_SHA,
                    schema_version=2,
                )
        self.assertTrue(value["production_source_unchanged"])
        self.assertEqual([row["id"] for row in value["results"]], list(lanes.POLICY_SCENARIO_IDS))
        self.assertEqual(
            value["identities"]["production_source_tree_before"],
            value["identities"]["production_source_tree_after"],
        )

    def test_new_json_schemas_accept_canonical_complete_artifacts(self) -> None:
        import jsonschema
        policy = policy_evidence()
        for row in policy["results"]:
            row["test"].update({
                "transcript_digest": digest("e"),
            })
            row["proof"].update({
                "id": row["id"], "source_test": row["test"],
                "fixture_key_id": lanes.FIXTURE_KEY_ID,
                "overlay_digest": digest("5"),
            })
            row["proof_digest"] = lanes.sha256(lanes.canonical_json(row["proof"]))
        policy["identities"].update({
            "production_source_tree_before": lanes.PLUGIN_KIT_PRODUCTION_TREE_DIGEST,
            "production_source_tree_after": lanes.PLUGIN_KIT_PRODUCTION_TREE_DIGEST,
        })
        policy_schema = json.loads((ROOT / "schemas/e2e/source-policy-conformance-v2.schema.json").read_text())
        readiness_schema = json.loads((ROOT / "schemas/e2e/two-lane-readiness-v2.schema.json").read_text())
        jsonschema.Draft202012Validator(policy_schema).validate(policy)
        readiness = lanes.build_readiness_envelope(runtime_evidence(schema_version=5), policy_evidence(), **self.identity())
        jsonschema.Draft202012Validator(readiness_schema).validate(readiness)


if __name__ == "__main__":
    unittest.main()
