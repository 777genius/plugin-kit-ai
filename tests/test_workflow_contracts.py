import json
import re
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[1]
LAUNCH = ROOT / ".github/workflows/launch-evidence-e2e.yml"
LIVE = ROOT / ".github/workflows/live-e2e.yml"
PAGES = ROOT / ".github/workflows/pages.yml"
VALIDATE = ROOT / ".github/workflows/validate.yml"
DIRECTORY_PUBLICATION = ROOT / ".github/workflows/directory-publication.yml"
PUBLICATION_INPUTS = {
    "publication_id": "string",
    "publication_sequence": "number",
    "publication_snapshot_digest": "string",
    "publication_source_commit": "string",
    "publication_ledger_commit": "string",
}
CALLER_INPUTS = {"caller_event_name", "caller_ref", "caller_workflow_ref"}


def load(path: Path):
    return yaml.load(path.read_text(), Loader=yaml.BaseLoader)


def commands(job):
    return "\n".join(step.get("run", "") for step in job["steps"] if isinstance(step, dict))


class WorkflowContractTests(unittest.TestCase):
    def test_stable_launch_versions_equal_trusted_contract(self) -> None:
        version = (ROOT / "tests/e2e/stable-launch-version.txt").read_text().strip()
        self.assertEqual(version, "0.1.14")
        tag = f"agentplugins-v{version}"
        asset_prefix = f"agentplugins_{version}_"
        production = json.loads((ROOT / "tests/e2e/production-launch.json").read_text())
        schema = json.loads((ROOT / "tests/e2e/schemas/native-release-observation.schema.json").read_text())
        adapter_schema = json.loads((ROOT / "deploy/uap-observer-adapter-config.schema.json").read_text())
        observer = json.loads((ROOT / "deploy/uap-observer.json").read_text())
        workflow = load(LAUNCH)
        assets = {slot["asset"] for slot in workflow["jobs"]["native-release"]["strategy"]["matrix"]["include"]}
        npm = schema["properties"]["npm_package"]["properties"]
        self.assertEqual(workflow["env"]["AGENTPLUGINS_VERSION"], version)
        self.assertEqual(production["cli_release_tag"], tag)
        self.assertEqual(observer["cli_release_tag"], tag)
        self.assertEqual(adapter_schema["properties"]["request_policy"]["properties"]["cli_release_tag"]["const"], tag)
        # Bind the version and both independently authenticated public release
        # assets in one regression contract.  Updating only the version cannot
        # leave a stale observer trust pin behind.
        request_policy = adapter_schema["properties"]["request_policy"]["properties"]
        self.assertEqual((version, request_policy["release_manifest_digest"]["const"], request_policy["release_checksums_digest"]["const"]), (
            "0.1.14",
            "sha256:21b72bb9fc82df2b45ce2e83ea79eeb5b8436cfd9b09f8ccfcbb25c8d0fda8f9",
            "sha256:bd9f8de83b9b04589d2b29ce36ae079bf5f67b10b8f44c5ab811fc5d6706ff6b",
        ))
        self.assertEqual(schema["properties"]["cli_release_tag"]["const"], tag)
        self.assertEqual(schema["properties"]["github_release_identity"]["properties"]["tag"]["const"], tag)
        self.assertEqual(schema["properties"]["github_asset_attestation"]["properties"]["tag"]["const"], tag)
        self.assertTrue(assets)
        self.assertTrue(all(asset.startswith(asset_prefix) for asset in assets))
        self.assertEqual(npm["version"]["const"], version)
        self.assertEqual(npm["tarball"]["const"], f"https://registry.npmjs.org/universal-agent-plugins/-/universal-agent-plugins-{version}.tgz")
        self.assertEqual(npm["provenance_url"]["const"], f"https://registry.npmjs.org/-/npm/v1/attestations/universal-agent-plugins@{version}")
        self.assertEqual(npm["native_asset_name"]["const"], f"{asset_prefix}linux_amd64")
        stable_files = (
            LAUNCH, ROOT / "scripts/run_launch_evidence_e2e.py", ROOT / "tests/e2e/production-launch.json",
            ROOT / "tests/e2e/schemas/native-release-observation.schema.json", ROOT / "deploy/uap-observer.json",
            ROOT / "deploy/uap-observer-adapter-config.schema.json", ROOT / "observer/config.py",
        )
        for path in stable_files:
            with self.subTest(path=path.name):
                self.assertNotRegex(path.read_text(), r"(?:agentplugins[-_v@]|universal-agent-plugins[-@])0\.1\.(?:12|13)")

    def test_directory_publication_prepare_installs_bridge_runtime_dependencies(self) -> None:
        workflow = load(DIRECTORY_PUBLICATION)
        prepare_commands = commands(workflow["jobs"]["prepare"])
        self.assertIn("PyYAML==6.0.3", prepare_commands)

    def test_pages_concurrency_isolates_prs_from_production(self) -> None:
        workflow = load(PAGES)
        self.assertEqual(workflow["concurrency"]["group"], "${{ github.event_name == 'pull_request' && format('pages-pr-{0}', github.event.pull_request.number) || 'pages-production' }}")
        self.assertEqual(workflow["concurrency"]["cancel-in-progress"], "true")

    def test_launch_pr_is_fixture_only_and_has_no_secrets_or_runtime_claim(self) -> None:
        workflow = load(LAUNCH)
        job = workflow["jobs"]["fixture-only-non-runtime"]
        body = yaml.safe_dump(job)
        self.assertIn("pull_request", workflow["on"])
        self.assertIn("--mode fixture-only", commands(job))
        self.assertNotIn("secrets.", body)
        self.assertEqual(job["permissions"], {"contents": "read"})
        self.assertLessEqual(int(job["timeout-minutes"]), 10)

    def test_launch_evidence_jobs_fetch_parent_for_patch_binding(self) -> None:
        launch = load(LAUNCH)
        live = load(LIVE)
        jobs = (
            (launch["jobs"]["fixture-only-non-runtime"], "fixture-only-non-runtime"),
            (launch["jobs"]["enforced-stable-gate"], "enforced-stable-gate"),
            (live["jobs"]["scheduled-fixture-contract"], "scheduled-fixture-contract"),
        )
        for job, name in jobs:
            with self.subTest(job=name):
                checkout = next(
                    step for step in job["steps"]
                    if step.get("uses", "").startswith("actions/checkout")
                )
                self.assertEqual(checkout["with"]["fetch-depth"], "2")

    def test_enforced_jobs_are_not_disabled_by_impossible_capture_lineage(self) -> None:
        workflow = load(LAUNCH)
        self.assertNotIn("CAPTURE_TRUST_READY", workflow["env"])
        self.assertNotIn("capture-trust-unavailable", workflow["jobs"])
        for name in (
            "native-release", "node22-npm-facade", "aggregate-one-release",
            "protected-observer-inputs", "enforced-stable-gate", "attest-stable-evidence",
        ):
            self.assertNotIn("CAPTURE_TRUST", workflow["jobs"][name]["if"])
        self.assertNotIn("capture-transcript", commands(workflow["jobs"]["enforced-stable-gate"]))

    def test_live_gate_resolves_official_release_and_native_matrix(self) -> None:
        workflow = load(LAUNCH)
        native = workflow["jobs"]["native-release"]
        npm = workflow["jobs"]["node22-npm-facade"]
        aggregate = workflow["jobs"]["aggregate-one-release"]
        enforced = workflow["jobs"]["enforced-stable-gate"]
        inputs = workflow["on"]["workflow_dispatch"]["inputs"]
        self.assertEqual(inputs["consent"]["required"], "true")
        self.assertNotIn("release_tag", inputs)
        self.assertEqual(set(workflow["on"]["workflow_call"]["inputs"]), {"consent", *PUBLICATION_INPUTS, *CALLER_INPUTS})
        self.assertTrue(all(workflow["on"]["workflow_call"]["inputs"][name]["required"] == "true" for name in PUBLICATION_INPUTS))
        self.assertIn("workflow_call", workflow["on"])
        slots = native["strategy"]["matrix"]["include"]
        self.assertEqual({(slot["os"], slot["architecture"]) for slot in slots}, {
            ("macos", "arm64"), ("macos", "amd64"), ("linux", "arm64"),
            ("linux", "amd64"), ("windows", "amd64"), ("windows", "arm64"),
        })
        self.assertEqual({slot["asset"] for slot in slots}, {
            "agentplugins_0.1.14_darwin_arm64", "agentplugins_0.1.14_darwin_amd64",
            "agentplugins_0.1.14_linux_arm64", "agentplugins_0.1.14_linux_amd64",
            "agentplugins_0.1.14_windows_amd64.exe", "agentplugins_0.1.14_windows_arm64.exe",
        })
        aggregate_commands = commands(aggregate)
        self.assertIn("['subject_name'] == x['asset_name']", aggregate_commands)
        self.assertIn("['subject_digest'] == x['asset_digest']", aggregate_commands)
        self.assertIn("set(x) == expected_attestation_keys", aggregate_commands)
        self.assertIn("prepare_launch_evidence.py", commands(native))
        self.assertIn("node-version: '22'", yaml.safe_dump(npm))
        self.assertIn("npm install --global", commands(npm))
        self.assertIn("npm audit signatures", commands(npm))
        self.assertIn("--npm-facade", commands(npm))
        self.assertIn("--npm-binary-cache", commands(npm))
        self.assertIn('AGENTPLUGINS_CACHE_DIR="$run_root/npm-binary-cache"', commands(npm))
        self.assertIn("universal-agent-plugins-0.1.14.tgz", commands(npm))
        self.assertIn("--asset-name agentplugins_0.1.14_linux_amd64", commands(npm))
        self.assertNotIn("universal-agent-plugins.tgz", commands(npm))
        self.assertEqual(yaml.safe_dump(native).count("tzdata==2026.3"), 1)
        self.assertEqual(yaml.safe_dump(npm).count("tzdata==2026.3"), 1)
        self.assertNotRegex(commands(npm), r"github\.com/.*\.tgz")
        self.assertIn("inputs.consent", aggregate["if"])
        self.assertIn("release_manifest_digest", commands(aggregate))
        self.assertEqual(set(enforced["needs"]), {"native-release", "node22-npm-facade", "aggregate-one-release", "protected-observer-inputs"})
        observer = workflow["jobs"]["protected-observer-inputs"]
        self.assertEqual(observer["environment"], "stable-launch-e2e")
        self.assertNotIn("environment", enforced)
        self.assertIn("--prepared-context", commands(enforced))
        self.assertIn("--native-observations", commands(enforced))
        self.assertIn("request_observer_bundle", commands(observer))
        self.assertIn('"scenario_contract_digest": scenario_digest', commands(observer))
        observer_commands = " ".join(commands(observer).split())
        self.assertIn(
            'context["release_manifest_digest"], context["directory"]["digest"], '
            'scenario_digest, Path("prepared-context")',
            observer_commands,
        )
        self.assertNotIn(
            'context["release_manifest_digest"], context["directory"]["digest"], '
            'Path("prepared-context"), scenario_digest',
            observer_commands,
        )
        self.assertIn('"run_attempt": os.environ["GITHUB_RUN_ATTEMPT"]', commands(observer))
        self.assertIn('context["producer_run_attempt"] = producer_attempt', commands(observer))
        self.assertIn("enforce_freshness=True", commands(observer))
        self.assertNotIn("id-token", enforced["permissions"])
        self.assertIn("--observer-bundle", commands(enforced))
        self.assertIn("observer-bundle.schema.json", commands(observer))
        self.assertIn("STABLE_LAUNCH_OBSERVER_ED25519_PUBLIC_KEY", yaml.safe_dump(enforced))
        self.assertIn("STABLE_LAUNCH_OBSERVER_KEY_ID", yaml.safe_dump(enforced))
        self.assertIn("node-version: '22'", yaml.safe_dump(enforced))
        self.assertIn("npm install --ignore-scripts=false --save-exact @github/copilot@1.0.80", commands(enforced))
        self.assertIn("npm audit signatures", commands(enforced))
        self.assertIn('"package": "@github/copilot"', commands(enforced))
        self.assertIn('"version": "1.0.80"', commands(enforced))
        self.assertIn("sha512-6tf93ZF56KOiTTAjK/UhLZkl1W543IzaTQly288kockJZFswpRTnQEI00Yvacpb39DTvTYu3/ha9SeKpo/pgZQ==", commands(enforced))
        self.assertIn('["copilot", "--version"]', commands(enforced))
        self.assertIn("--copilot-executable", commands(enforced))
        self.assertIn("--copilot-metadata", commands(enforced))
        self.assertNotIn("npm install --global @github/copilot", commands(enforced))
        body = LAUNCH.read_text()
        for forbidden in ("binary_url", "binary_sha256", "directory_bundle_url", "directory_bundle_sha256", "live_inputs_url", "scenario-driver"):
            self.assertNotIn(forbidden, body)
        self.assertNotIn("inputs.release_tag", body)
        self.assertNotIn("--release-tag", body)
        production = (ROOT / "tests/e2e/production-launch.json").read_text()
        self.assertIn('"cli_release_repository": "777genius/plugin-kit-ai"', production)
        self.assertIn('"cli_release_tag": "agentplugins-v0.1.14"', production)
        self.assertIn('"cli_release_commit": "caffa9ac2a962462a05d5342250f4810ddce0856"', production)
        prepare = (ROOT / "scripts/prepare_launch_evidence.py").read_text()
        self.assertNotIn('os.environ.get("GITHUB_TOKEN")', prepare)
        self.assertIn("token=None", prepare)
        self.assertNotIn("fetch_production_directory", prepare)
        self.assertIn("fetch_staged_directory", prepare)
        self.assertNotIn('config["cli_release_id"]', prepare)
        self.assertEqual(LAUNCH.read_text().count("python scripts/prepare_launch_evidence.py --asset-name"), 2)
        self.assertIn("--publication-ledger-commit", commands(native))
        for job, next_command in (
            (native, "python scripts/observe_release_facade.py"),
            (npm, 'mkdir "$run_root/npm-audit"'),
        ):
            resolve = next(step for step in job["steps"] if "prepare_launch_evidence.py" in step.get("run", ""))
            self.assertEqual(resolve["env"]["GH_TOKEN"], "${{ github.token }}")
            body = resolve["run"]
            self.assertLess(body.index("prepare_launch_evidence.py"), body.index("unset GH_TOKEN GITHUB_TOKEN"))
            self.assertLess(body.index("unset GH_TOKEN GITHUB_TOKEN"), body.index(next_command))

    def test_false_consent_skips_every_live_and_aggregate_job(self) -> None:
        workflow = load(LAUNCH)
        for name in ("native-release", "node22-npm-facade", "aggregate-one-release", "enforced-stable-gate"):
            with self.subTest(job=name):
                condition = workflow["jobs"][name]["if"]
                self.assertIn("inputs.consent", condition)
        self.assertEqual(workflow["jobs"]["fixture-only-non-runtime"]["if"], "github.event_name == 'pull_request'")

    def test_owned_workflows_pin_actions_and_upload_checksums_immutably(self) -> None:
        for path in (LAUNCH, LIVE):
            text = path.read_text()
            with self.subTest(path=path.name):
                uses = re.findall(r"uses:\s+([^\s#]+)", text)
                self.assertTrue(uses)
                self.assertTrue(all(item.startswith("./") or re.search(r"@[0-9a-f]{40}$", item) for item in uses))
                self.assertIn("SHA256SUMS", text)
                self.assertIn("overwrite: false", text)
                if path == LAUNCH:
                    self.assertIn("agentplugins_0.1.14_linux_amd64", text)
                self.assertNotIn("AGENTPLUGINS_VERSION: \"0.1.6\"", text)

    def test_live_workflow_is_read_only_and_does_not_publish(self) -> None:
        workflow = load(LIVE)
        text = LIVE.read_text()
        self.assertEqual(workflow["permissions"], {"contents": "read"})
        self.assertNotIn("publish-release", text)
        self.assertNotIn("contents: write", text)
        self.assertNotIn("catalog/v1", text)
        self.assertNotIn("catalog/v2", text)
        self.assertIn("workflow_call", workflow["on"])
        required = workflow["jobs"]["required-stable-launch-evidence"]
        self.assertEqual(required["uses"], "./.github/workflows/launch-evidence-e2e.yml")
        self.assertNotIn("release_tag", required.get("with", {}))
        self.assertEqual(required["permissions"], {
            "actions": "read",
            "attestations": "write",
            "contents": "read",
            "id-token": "write",
        })

    def test_nested_launch_workflow_permissions_never_escalate(self) -> None:
        publication = load(DIRECTORY_PUBLICATION)
        live = load(LIVE)
        launch = load(LAUNCH)
        expected = {
            "actions": "read",
            "attestations": "write",
            "contents": "read",
            "id-token": "write",
        }
        publication_call = publication["jobs"]["required_stable_launch_evidence"]
        live_call = live["jobs"]["required-stable-launch-evidence"]
        self.assertEqual(publication_call["permissions"], expected)
        self.assertEqual(live_call["permissions"], expected)

        permission_levels = {"none": 0, "read": 1, "write": 2}
        for job_name, job in launch["jobs"].items():
            for permission, level in job.get("permissions", {}).items():
                with self.subTest(job=job_name, permission=permission):
                    self.assertIn(permission, expected)
                    self.assertLessEqual(
                        permission_levels[level],
                        permission_levels[expected[permission]],
                    )

    def test_scheduled_live_workflow_never_calls_staged_publication_gate(self) -> None:
        workflow = load(LIVE)
        required = workflow["jobs"]["required-stable-launch-evidence"]
        self.assertIn("inputs.consent", required["if"])
        self.assertIn("inputs.publication_sequence > 0", required["if"])
        self.assertIn("inputs.caller_ref == 'refs/heads/main'", required["if"])
        self.assertIn("directory-publication.yml@refs/heads/main", required["if"])
        scheduled = {
            name: job
            for name, job in workflow["jobs"].items()
            if job.get("if") == "github.event_name == 'schedule'"
        }
        self.assertEqual(
            set(scheduled),
            {"scheduled-fixture-contract", "scheduled-production-directory-observation"},
        )
        scheduled_body = yaml.safe_dump(scheduled)
        self.assertNotIn("inputs.publication_", scheduled_body)
        self.assertNotIn("launch-evidence-e2e.yml", scheduled_body)
        observation = commands(scheduled["scheduled-production-directory-observation"])
        self.assertIn("fetch_production_directory", observation)
        self.assertIn('"runtime_claims": False', observation)
        self.assertIn('"oauth_claims": False', observation)
        self.assertIn("SHA256SUMS", observation)
        public_reads = workflow["jobs"]["public-read-flows"]
        self.assertEqual(
            public_reads["if"], "github.event_name == 'schedule' || inputs.consent"
        )

    def test_reusable_protected_jobs_gate_on_caller_inputs_not_event_name(self) -> None:
        launch = load(LAUNCH)
        protected_jobs = {
            "native-release", "node22-npm-facade", "aggregate-one-release",
            "protected-observer-inputs", "enforced-stable-gate", "attest-stable-evidence",
        }
        for name in protected_jobs:
            condition = launch["jobs"][name]["if"]
            with self.subTest(job=name):
                self.assertIn("inputs.consent", condition)
                self.assertIn("inputs.publication_sequence > 0", condition)
                self.assertIn("inputs.caller_ref == 'refs/heads/main'", condition)
                self.assertIn("directory-publication.yml@refs/heads/main", condition)
                self.assertNotIn("github.event_name", condition)

    def test_observer_attestation_claims_the_oidc_job(self) -> None:
        schema = json.loads((ROOT / "tests/e2e/schemas/runtime-attestations.schema.json").read_text())
        claimed_job = schema["properties"]["attestations"]["items"]["properties"]["github_attestation"]["properties"]["job"]["const"]
        self.assertEqual(claimed_job, "protected-observer-inputs")
        attestations = schema["properties"]["attestations"]
        self.assertEqual(attestations["maxItems"], 12)
        self.assertEqual({item["maxItems"] for item in attestations["oneOf"]}, {1, 3, 12})
        required = set(attestations["items"]["required"])
        self.assertTrue({"release_manifest_digest", "release_checksums_digest", "directory_digest", "scenario_contract_digest"} <= required)
        self.assertIn('github.get("job") == "protected-observer-inputs"', (ROOT / "scripts/run_launch_evidence_e2e.py").read_text())

    def test_live_terminal_gate_rejects_skipped_or_failed_nested_evidence(self) -> None:
        workflow = load(LIVE)
        gate = workflow["jobs"]["required-live-e2e"]
        self.assertIn("always() && inputs.consent", gate["if"])
        self.assertIn("inputs.caller_ref == 'refs/heads/main'", gate["if"])
        self.assertIn("directory-publication.yml@refs/heads/main", gate["if"])
        self.assertEqual(
            set(gate["needs"]),
            {"required-stable-launch-evidence", "public-read-flows"},
        )
        step = gate["steps"][0]
        self.assertEqual(
            step["env"],
            {
                "STABLE_E2E_RESULT": "${{ needs.required-stable-launch-evidence.result }}",
                "PUBLIC_READ_RESULT": "${{ needs.public-read-flows.result }}",
            },
        )
        body = commands(gate)
        self.assertIn('test "$STABLE_E2E_RESULT" = success', body)
        self.assertIn('test "$PUBLIC_READ_RESULT" = success', body)

    def test_publication_identity_contract_matches_across_both_reusable_edges(self) -> None:
        launch = load(LAUNCH)
        live = load(LIVE)
        publication = load(DIRECTORY_PUBLICATION)
        launch_inputs = launch["on"]["workflow_call"]["inputs"]
        live_inputs = live["on"]["workflow_call"]["inputs"]
        live_call = live["jobs"]["required-stable-launch-evidence"]["with"]
        publication_call = publication["jobs"]["required_stable_launch_evidence"]["with"]
        expected = {"consent", *PUBLICATION_INPUTS, *CALLER_INPUTS}
        self.assertEqual(set(launch_inputs), expected)
        self.assertEqual(set(live_inputs), expected)
        self.assertEqual(set(live_call), expected)
        self.assertEqual(set(publication_call), expected)
        for name, expected_type in PUBLICATION_INPUTS.items():
            with self.subTest(input=name):
                self.assertEqual(launch_inputs[name]["required"], "true")
                self.assertEqual(live_inputs[name]["required"], "true")
                self.assertEqual(launch_inputs[name]["type"], expected_type)
                self.assertEqual(live_inputs[name]["type"], expected_type)
                self.assertEqual(live_call[name], "${{ inputs." + name + " }}")
        for name in CALLER_INPUTS:
            self.assertEqual(launch_inputs[name]["required"], "true")
            self.assertEqual(live_inputs[name]["required"], "true")
            self.assertEqual(live_call[name], "${{ inputs." + name + " }}")
        self.assertEqual(publication_call["publication_id"], "${{ needs.sign.outputs.publication_id }}")
        self.assertEqual(publication_call["publication_sequence"], "${{ fromJSON(needs.sign.outputs.sequence) }}")
        self.assertEqual(publication_call["publication_snapshot_digest"], "${{ needs.sign.outputs.snapshot_digest }}")
        self.assertEqual(publication_call["publication_source_commit"], "${{ needs.sign.outputs.marker_commit }}")
        self.assertEqual(publication_call["publication_ledger_commit"], "${{ needs.materialize_site.outputs.ledger_commit }}")

    def test_directory_release_stages_exact_identity_before_reusable_gate_and_promotion(self) -> None:
        workflow = load(DIRECTORY_PUBLICATION)
        required = workflow["jobs"]["required_stable_launch_evidence"]
        self.assertEqual(set(required["needs"]), {"prepare", "sign", "materialize_site", "gate_exact_staged_publication"})
        self.assertEqual(required["uses"], "./.github/workflows/live-e2e.yml")
        self.assertEqual(required["with"]["consent"], "true")
        self.assertEqual(required["permissions"], {
            "actions": "read",
            "attestations": "write",
            "contents": "read",
            "id-token": "write",
        })
        exact = workflow["jobs"]["gate_exact_staged_publication"]
        verify = next(step for step in exact["steps"] if step.get("name") == "Verify staged bytes and immutable identity before promotion")
        self.assertEqual(exact["needs"], ["sign", "materialize_site"])
        for field in (
            "EXPECTED_SEQUENCE", "EXPECTED_SNAPSHOT_DIGEST", "EXPECTED_PUBLICATION_ID",
            "EXPECTED_SOURCE_COMMIT", "EXPECTED_SIGNED_LEDGER_COMMIT",
            "EXPECTED_MATERIALIZED_LEDGER_COMMIT",
        ):
            self.assertIn(field, verify["env"])
        self.assertIn("raw.githubusercontent.com", verify["run"])
        deploy_needs = workflow["jobs"]["deploy"]["needs"]
        self.assertIn("sign", deploy_needs)
        self.assertIn("gate_exact_staged_publication", deploy_needs)
        self.assertIn("gate_launch_approval", deploy_needs)
        production = workflow["jobs"]["observe_production_latest"]
        self.assertIn("deploy", production["needs"])
        self.assertNotIn("observe_production_latest", required["needs"])
        self.assertIn("observe_production_latest.py", commands(production))
        self.assertEqual(production["permissions"], {"contents": "read"})

    def test_first_unapproved_publication_cannot_promote_without_launch_ceremony(self) -> None:
        workflow = load(DIRECTORY_PUBLICATION)
        prepare = workflow["jobs"]["prepare"]
        launch_if = workflow["jobs"]["required_stable_launch_evidence"]["if"]
        record_if = workflow["jobs"]["record_launch_approval"]["if"]
        marker_if = workflow["jobs"]["gate_launch_approval"]["if"]
        deploy_if = workflow["jobs"]["deploy"]["if"]
        self.assertEqual(prepare["outputs"]["launch_approved"], "${{ steps.launch.outputs.approved }}")
        launch_step = next(step for step in prepare["steps"] if step.get("id") == "launch")
        self.assertIn("git ls-remote --refs origin", launch_step["run"])
        self.assertIn("needs.prepare.outputs.launch_approved == 'false'", launch_if)
        self.assertIn("needs.prepare.outputs.launch_approved == 'false'", record_if)
        self.assertIn("needs.required_stable_launch_evidence.result == 'success'", record_if)
        self.assertIn("needs.prepare.outputs.launch_approved == 'false'", marker_if)
        self.assertIn("needs.record_launch_approval.result == 'success'", marker_if)
        self.assertNotIn("needs.sign.outputs.sequence == '1'", launch_if + record_if + marker_if)
        self.assertIn("needs.gate_launch_approval.result == 'success'", deploy_if)

    def test_launch_evidence_is_attested_then_persisted_by_exact_two_ref_cas(self) -> None:
        launch = load(LAUNCH)
        live = load(LIVE)
        publication = load(DIRECTORY_PUBLICATION)
        enforced = launch["jobs"]["enforced-stable-gate"]
        observer = launch["jobs"]["protected-observer-inputs"]
        attester = launch["jobs"]["attest-stable-evidence"]
        self.assertNotIn("environment", enforced)
        self.assertNotIn("attestations", enforced["permissions"])
        self.assertNotIn("id-token", enforced["permissions"])
        self.assertEqual(observer["permissions"]["id-token"], "write")
        self.assertNotIn("attestations", observer["permissions"])
        observer_downloads = [
            step["with"].get("name", "") for step in observer["steps"]
            if "download-artifact" in step.get("uses", "")
        ]
        self.assertTrue(any(name.startswith("prepared-launch-context-") for name in observer_downloads))
        self.assertFalse(any(name.startswith("prepared-producer-inputs-") for name in observer_downloads))
        self.assertEqual(attester["permissions"]["attestations"], "write")
        self.assertEqual(attester["permissions"]["id-token"], "write")
        self.assertEqual(attester["environment"], "stable-launch-e2e")
        self.assertNotIn("OBSERVER_ENDPOINT", yaml.safe_dump(attester))
        self.assertIn("OBSERVER_ED25519_PUBLIC_KEY", yaml.safe_dump(attester))
        self.assertIn("materialize_launch_evidence.py prepare-bundle", commands(enforced))
        attestation = next(
            step for step in attester["steps"]
            if step.get("name") == "Attest the canonical bundle identity only"
        )
        self.assertEqual(attestation["with"]["subject-path"], "evidence/bundle-identity.json")
        self.assertIn("materialize_launch_evidence.py verify-bundle", commands(attester))
        self.assertIn("--verify-observer", commands(attester))
        self.assertIn("--observer-public-key", commands(attester))
        self.assertEqual(
            attester["outputs"]["evidence_run_attempt"],
            "${{ needs.enforced-stable-gate.outputs.evidence_run_attempt }}",
        )
        self.assertNotIn("github.run_attempt", attester["outputs"]["evidence_run_attempt"])
        attester_body = commands(attester)
        self.assertIn(
            '--expected-run-attempt "${{ needs.enforced-stable-gate.outputs.evidence_run_attempt }}"',
            attester_body,
        )
        enforced_body = yaml.safe_dump(launch["jobs"]["enforced-stable-gate"])
        self.assertIn('native-*-${{ github.run_id }}-*', enforced_body)
        self.assertIn("needs.node22-npm-facade.outputs.evidence_run_attempt", enforced_body)
        self.assertIn("needs.protected-observer-inputs.outputs.evidence_run_attempt", enforced_body)
        self.assertIn('run_attempt=${EVIDENCE_RUN_ATTEMPT}', enforced_body)
        aggregate_body = yaml.safe_dump(launch["jobs"]["aggregate-one-release"])
        self.assertIn('native-*-${{ github.run_id }}-*', aggregate_body)
        self.assertNotIn('test "$NATIVE_ATTEMPT" = "$NODE_ATTEMPT"', aggregate_body)
        self.assertEqual(
            attestation["uses"],
            "actions/attest-build-provenance@4d101475d8b20a2381f78447822ac1eab6504dd8",
        )
        self.assertEqual(
            set(launch["on"]["workflow_call"]["outputs"]),
            {"evidence_artifact_name", "launch_evidence_digest", "workflow_source_digest", "evidence_run_attempt"},
        )
        self.assertEqual(
            set(live["on"]["workflow_call"]["outputs"]),
            {"evidence_artifact_name", "launch_evidence_digest", "workflow_source_digest", "evidence_run_attempt"},
        )
        persist = publication["jobs"]["record_launch_approval"]
        body = commands(persist)
        self.assertIn("jsonschema==4.26.0", body)
        self.assertNotIn("jsonschema==4.25.1", str(publication))
        self.assertIn("materialize_launch_evidence.py verify-bundle", body)
        self.assertIn("--verify-attestation", body)
        self.assertIn('--expected-run-attempt "${EXPECTED_EVIDENCE_RUN_ATTEMPT}"', body)
        self.assertIn("materialize_launch_evidence.py commit", body)
        self.assertIn("directory_publication_cas.py evidence-publish", body)
        self.assertNotIn("ls-remote", body)
        self.assertIn("validate_directory", (ROOT / "scripts/materialize_launch_evidence.py").read_text())
        self.assertIn('--main-old "${EXPECTED_SOURCE_COMMIT}"', body)
        self.assertIn('--ledger-old "${EXPECTED_LEDGER_COMMIT}"', body)
        self.assertIn('--approval-tag "${marker_ref}"', body)

    def test_approved_refresh_skips_launch_but_keeps_exact_publication_gates(self) -> None:
        workflow = load(DIRECTORY_PUBLICATION)
        launch_if = workflow["jobs"]["required_stable_launch_evidence"]["if"]
        marker_if = workflow["jobs"]["gate_launch_approval"]["if"]
        deploy_if = workflow["jobs"]["deploy"]["if"]
        self.assertIn("needs.prepare.outputs.launch_approved == 'false'", launch_if)
        self.assertIn("needs.prepare.outputs.launch_approved == 'true'", marker_if)
        self.assertIn("needs.required_stable_launch_evidence.result == 'skipped'", marker_if)
        self.assertIn("needs.record_launch_approval.result == 'skipped'", marker_if)
        self.assertIn("always()", deploy_if)
        self.assertIn("needs.gate_launch_approval.result == 'success'", deploy_if)
        for required_result in (
            "needs.sign.result == 'success'",
            "needs.materialize_site.result == 'success'",
            "needs.gate_exact_staged_publication.result == 'success'",
        ):
            self.assertIn(required_result, deploy_if)

    def test_whole_run_retry_uses_exact_completed_state_noop(self) -> None:
        workflow = load(DIRECTORY_PUBLICATION)
        prepare = workflow["jobs"]["prepare"]
        replay = workflow["jobs"]["authenticate-completed-state"]
        self.assertEqual(prepare["outputs"]["completed"], "${{ needs.authenticate-completed-state.outputs.completed }}")
        self.assertEqual(prepare["permissions"], {"contents": "read"})
        self.assertEqual(replay["permissions"], {"attestations": "read", "contents": "read"})
        state = next(step for step in replay["steps"] if step.get("id") == "state")
        self.assertIn("materialize_launch_evidence.py verify-completed", state["run"])
        self.assertIn("needs.prepare.outputs.completed != 'true'", workflow["jobs"]["sign"]["if"])
        self.assertEqual(
            workflow["jobs"]["completed_rerun"]["if"],
            "needs.prepare.outputs.completed == 'true'",
        )

    def test_emergency_revocation_uses_the_same_higher_sequence_promotion_contract(self) -> None:
        workflow = load(DIRECTORY_PUBLICATION)
        deploy = workflow["jobs"]["deploy"]
        self.assertEqual(
            set(deploy["needs"]),
            {"sign", "materialize_site", "gate_exact_staged_publication", "gate_launch_approval"},
        )
        self.assertIn("needs.gate_launch_approval.result == 'success'", deploy["if"])
        self.assertIn("gate_exact_staged_publication", deploy["needs"])

    def test_untrusted_pull_request_bridge_reproduction_remains_secretless(self) -> None:
        workflow = load(VALIDATE)
        job = workflow["jobs"]["bridge-reproduction"]
        text = commands(job)
        self.assertEqual(job["permissions"], {"contents": "read"})
        self.assertNotIn("secrets.", yaml.safe_dump(job))
        self.assertIn("scripts/build-bridges check", text)
        self.assertNotIn("curl ", text)

    def test_pull_request_ci_reacquires_only_changed_external_releases(self) -> None:
        workflow = load(VALIDATE)
        job = workflow["jobs"]["portable-catalog"]
        step = next(item for item in job["steps"] if item.get("name") == "Reacquire and validate changed external releases")
        self.assertEqual(step["if"], "github.event_name == 'pull_request'")
        self.assertEqual(step["env"]["BASE_REVISION"], "${{ github.event.pull_request.base.sha }}")
        self.assertIn("--external-release-check changed", step["run"])
        self.assertIn('--base-revision "${BASE_REVISION}"', step["run"])
        self.assertEqual(next(item for item in job["steps"] if item.get("uses", "").startswith("actions/checkout"))["with"]["fetch-depth"], "0")


if __name__ == "__main__":
    unittest.main()
