from __future__ import annotations

import copy
import hashlib
import json
import sys
import unittest
from pathlib import Path

import jsonschema


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
sys.path.insert(0, str(ROOT / "tests"))

import test_two_lane_evidence as fixtures
import two_lane_evidence as lanes


VERSION = "0.1.24"
TAG = "agentplugins-v0.1.24"
COMMIT = "c78c79e44efd5ad07083d63436d9170b107df6cb"
BINARY_DIGEST = "sha256:e79125f7ffabd11c6e211d6b049c2eb2b36eb1aba3a76ce27cac819aeba1e6ca"
MANIFEST_DIGEST = "sha256:eb834da8237b13ed36061aeafb4fbb6f4aadeb5a6fbd4a31d43781f456f3d1e2"
CHECKSUMS_DIGEST = "sha256:623fb73d0e2f59da8b01399842b0d82b8f6456c6e43db2251c0ea5f9e32f37e3"


def launch_v5() -> dict:
    value = fixtures.runtime_evidence()
    value["schema_version"] = 5
    value["run"]["cli"].update(version=VERSION, binary_digest=BINARY_DIGEST)
    value["release"].update(
        tag=TAG,
        tag_commit=COMMIT,
        release_id=379284682,
        manifest_digest=MANIFEST_DIGEST,
        checksums_digest=CHECKSUMS_DIGEST,
    )
    for row in value["matrix"]:
        row["tuple"].update(
            binary_digest=BINARY_DIGEST,
            installer_version=VERSION,
            adapter_version=VERSION,
        )
        row["details"].update(
            release_manifest_digest=MANIFEST_DIGEST,
            release_checksums_digest=CHECKSUMS_DIGEST,
        )
    chatgpt = copy.deepcopy(value["matrix"][0])
    chatgpt.update(
        id="f" * 24,
        scenario="chatgpt_registered_binding",
        plugin="cloudflare-docs",
        client="chatgpt",
    )
    chatgpt["tuple"].update(
        product_id="cloudflare-docs",
        distribution_id="fixture/cloudflare-docs",
    )
    chatgpt["details"].update(
        scenario_id="chatgpt_registered_binding",
        native_discovery_proof=False,
        public_mcp_proof=True,
        public_mcp_evidence={},
    )
    chatgpt["details"].pop("native_discovery_evidence", None)
    value["matrix"].append(chatgpt)
    value["summary"]["passed"] = 16
    return value


class LaunchEvidenceSchemaRouterTests(unittest.TestCase):
    def test_router_lists_all_frozen_schema_versions(self) -> None:
        router = json.loads((ROOT / "tests/e2e/schemas/launch-evidence.schema.json").read_text())
        self.assertEqual(router["oneOf"], [
            {"$ref": "launch-evidence-v3.schema.json"},
            {"$ref": "launch-evidence-v4.schema.json"},
            {"$ref": "launch-evidence-v5.schema.json"},
        ])

    def test_caller_purpose_controls_version_acceptance(self) -> None:
        v5 = launch_v5()
        lanes.validate_launch_schema(v5)
        lanes.validate_launch_schema(v5, purpose="historical")

        v4 = fixtures.runtime_evidence()
        v3 = copy.deepcopy(v4)
        v3["schema_version"] = 3
        v3.pop("evidence_class")
        for field in ("ledger_commit", "publication_id", "source_commit"):
            v3["directory"].pop(field)
        v3["scenario_contract"]["expected_ids"] = [f"acceptance-{i}" for i in range(13)]
        v3["scenario_contract"]["required_singleton_ids"] = [f"singleton-{i}" for i in range(38)]
        v3["summary"].pop("released_binary_gate_complete")

        for historical in (v3, v4):
            lanes.validate_launch_schema(historical, purpose="historical")
            with self.assertRaisesRegex(lanes.TwoLaneEvidenceError, "requires schema_version 5"):
                lanes.validate_launch_schema(historical, purpose="current")

        artifact_downgrade = copy.deepcopy(v4)
        artifact_downgrade["validation_purpose"] = "historical"
        with self.assertRaisesRegex(lanes.TwoLaneEvidenceError, "requires schema_version 5"):
            lanes.validate_launch_schema(artifact_downgrade)
        with self.assertRaisesRegex(lanes.TwoLaneEvidenceError, "unknown.*purpose"):
            lanes.validate_launch_schema(v5, purpose="artifact-selected")  # type: ignore[arg-type]

    def test_v5_requires_exact_release_tuple_and_single_chatgpt_row(self) -> None:
        baseline = launch_v5()
        lanes.validate_launch_schema(baseline)
        mutations = []
        for field in ("installer_version", "adapter_version", "binary_digest"):
            value = copy.deepcopy(baseline)
            value["matrix"][0]["tuple"][field] = "0.1.23" if field != "binary_digest" else "sha256:" + "0" * 64
            mutations.append(value)
            chatgpt_value = copy.deepcopy(baseline)
            chatgpt_value["matrix"][-1]["tuple"][field] = "0.1.23" if field != "binary_digest" else "sha256:" + "0" * 64
            mutations.append(chatgpt_value)
        missing_chatgpt = copy.deepcopy(baseline)
        missing_chatgpt["matrix"].pop()
        mutations.append(missing_chatgpt)
        duplicate_chatgpt = copy.deepcopy(baseline)
        duplicate = copy.deepcopy(duplicate_chatgpt["matrix"][-1])
        duplicate["id"] = "e" * 24
        duplicate_chatgpt["matrix"].append(duplicate)
        mutations.append(duplicate_chatgpt)
        failed_duplicate_chatgpt = copy.deepcopy(baseline)
        failed_duplicate = copy.deepcopy(failed_duplicate_chatgpt["matrix"][-1])
        failed_duplicate.update(id="d" * 24, outcome="failed")
        failed_duplicate_chatgpt["matrix"].append(failed_duplicate)
        mutations.append(failed_duplicate_chatgpt)
        wrong_release = copy.deepcopy(baseline)
        wrong_release["release"]["tag"] = "agentplugins-v0.1.23"
        mutations.append(wrong_release)
        wrong_release_id = copy.deepcopy(baseline)
        wrong_release_id["release"]["release_id"] -= 1
        mutations.append(wrong_release_id)
        wrong_cli = copy.deepcopy(baseline)
        wrong_cli["run"]["cli"]["version"] = "0.1.23"
        mutations.append(wrong_cli)
        for value in mutations:
            with self.subTest(mutation=mutations.index(value)), self.assertRaises(lanes.TwoLaneEvidenceError):
                lanes.validate_launch_schema(value)

    def test_v5_rejects_protected_rows_without_details(self) -> None:
        for row_index, row_name in ((0, "hero"), (-1, "chatgpt")):
            value = launch_v5()
            value["matrix"][row_index].pop("details")
            with self.subTest(row=row_name), self.assertRaises(lanes.TwoLaneEvidenceError):
                lanes.validate_launch_schema(value)

    def test_v2_sidecar_schemas_pin_versions_and_release_identity(self) -> None:
        source = json.loads((ROOT / "schemas/e2e/source-policy-conformance-v2.schema.json").read_text())
        readiness = json.loads((ROOT / "schemas/e2e/two-lane-readiness-v2.schema.json").read_text())
        native = json.loads((ROOT / "tests/e2e/schemas/native-release-observation-v2.schema.json").read_text())
        self.assertEqual(source["properties"]["schema_version"], {"const": 2})
        identities = source["properties"]["identities"]["properties"]
        self.assertEqual(identities["plugin_kit_tag"], {"const": TAG})
        self.assertEqual(identities["plugin_kit_commit"], {"const": COMMIT})
        self.assertEqual(identities["release_manifest_digest"], {"const": MANIFEST_DIGEST})
        self.assertEqual(identities["release_checksums_digest"], {"const": CHECKSUMS_DIGEST})
        self.assertEqual(
            identities["production_source_tree_before"],
            {"const": "sha256:3635457d320bc2c78a86b9b3d8e4937d14ac59848ffae70c6571167204130de8"},
        )
        self.assertEqual(readiness["properties"]["schema_version"], {"const": 2})
        self.assertEqual(readiness["properties"]["runtime_results"], {"const": 16})
        self.assertEqual(native["properties"]["schema_version"], {"const": 2})
        self.assertEqual(native["properties"]["version"], {"const": VERSION})
        self.assertEqual(native["properties"]["cli_release_tag"], {"const": TAG})
        self.assertEqual(
            native["properties"]["github_release_identity"]["properties"]["release_id"],
            {"const": 379284682},
        )

    def test_source_policy_v2_requires_each_policy_id_exactly_once(self) -> None:
        schema = json.loads((ROOT / "schemas/e2e/source-policy-conformance-v2.schema.json").read_text())
        validator = jsonschema.Draft202012Validator(schema)
        baseline = fixtures.policy_evidence()
        baseline["schema_version"] = 2
        baseline["identities"].update(
            plugin_kit_tag=TAG,
            plugin_kit_commit=COMMIT,
            release_manifest_digest=MANIFEST_DIGEST,
            release_checksums_digest=CHECKSUMS_DIGEST,
            production_source_tree_before="sha256:3635457d320bc2c78a86b9b3d8e4937d14ac59848ffae70c6571167204130de8",
            production_source_tree_after="sha256:3635457d320bc2c78a86b9b3d8e4937d14ac59848ffae70c6571167204130de8",
        )
        validator.validate(baseline)

        duplicate = copy.deepcopy(baseline)
        duplicate["results"][-1] = copy.deepcopy(duplicate["results"][0])
        missing = copy.deepcopy(baseline)
        missing["results"].pop()
        for mutation, value in (("duplicate", duplicate), ("missing", missing)):
            with self.subTest(mutation=mutation), self.assertRaises(jsonschema.ValidationError):
                validator.validate(value)

    def test_frozen_v3_v4_and_v1_sidecar_bytes_are_unchanged(self) -> None:
        expected = {
            "tests/e2e/schemas/launch-evidence-v3.schema.json": "ff460e5a4b2248f1dd5b6967391741382db0870d9350789222921ac81929ffae",
            "tests/e2e/schemas/launch-evidence-v4.schema.json": "44654fb1fca4b8a533b218d1a66030416d16798bcb2165f95c0253d48f253299",
            "schemas/e2e/source-policy-conformance.schema.json": "c2174e9977fe0b7fb5d0a8903e61f0beaeff47bdf502c1788b8b0a80e1f19d6c",
            "schemas/e2e/two-lane-readiness.schema.json": "707c1e9ecc8960632df4dd3ef398cb9cc0a313b3dc9777aab42488da2c40c303",
            "tests/e2e/schemas/native-release-observation.schema.json": "d75703af94ae7720abda56bb876d831caa3ad5874d03eda45a9ca578a92f2b66",
        }
        for path, digest in expected.items():
            with self.subTest(path=path):
                self.assertEqual(hashlib.sha256((ROOT / path).read_bytes()).hexdigest(), digest)

    def test_all_new_schemas_pass_draft_2020_12_meta_schema(self) -> None:
        for path in (
            "tests/e2e/schemas/launch-evidence-v5.schema.json",
            "schemas/e2e/source-policy-conformance-v2.schema.json",
            "schemas/e2e/two-lane-readiness-v2.schema.json",
            "tests/e2e/schemas/native-release-observation-v2.schema.json",
            "tests/e2e/schemas/launch-evidence.schema.json",
        ):
            with self.subTest(path=path):
                jsonschema.Draft202012Validator.check_schema(json.loads((ROOT / path).read_text()))


if __name__ == "__main__":
    unittest.main()
