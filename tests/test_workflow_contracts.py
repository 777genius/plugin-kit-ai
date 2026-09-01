import ast
import base64
import copy
import json
import hashlib
import importlib.util
import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[1]
LAUNCH = ROOT / ".github/workflows/launch-evidence-e2e.yml"
LIVE = ROOT / ".github/workflows/live-e2e.yml"
PAGES = ROOT / ".github/workflows/pages.yml"
VALIDATE = ROOT / ".github/workflows/validate.yml"
DIRECTORY_PUBLICATION = ROOT / ".github/workflows/directory-publication.yml"
CATALOG_READINESS = ROOT / ".github/workflows/catalog-publication-readiness.yml"
DISCOVERY_INDEX = ROOT / ".github/workflows/discovery-index.yml"
UPSTREAM_PROMOTION = ROOT / ".github/workflows/upstream-promotion-readiness.yml"
OBSERVER_RUNBOOK = ROOT / "docs/OBSERVER_OPERATIONS.md"
PUBLICATION_INPUTS = {
    "publication_id": "string",
    "publication_sequence": "string",
    "publication_snapshot_digest": "string",
    "publication_source_commit": "string",
    "publication_ledger_commit": "string",
}
CALLER_INPUTS = {"caller_event_name", "caller_ref", "caller_workflow_ref"}


def load(path: Path):
    return yaml.load(path.read_text(), Loader=yaml.BaseLoader)


def commands(job):
    return "\n".join(step.get("run", "") for step in job["steps"] if isinstance(step, dict))


def pinned_requirements(body: str) -> set[str]:
    return set(re.findall(r"(?<![A-Za-z0-9_.-])[A-Za-z0-9_.-]+==[A-Za-z0-9_.+-]+", body))


class WorkflowContractTests(unittest.TestCase):
    def test_catalog_gate_is_fresh_per_publication_and_has_no_skip_bypass(self) -> None:
        workflow = load(DIRECTORY_PUBLICATION)
        gate = workflow["jobs"]["required_catalog_readiness"]
        deploy = workflow["jobs"]["deploy"]
        self.assertEqual(gate["uses"], "./.github/workflows/catalog-publication-readiness.yml")
        self.assertEqual(set(gate["needs"]), {"sign", "materialize_site", "gate_exact_staged_publication"})
        self.assertEqual(gate["with"], {
            "publication_id": "${{ needs.sign.outputs.publication_id }}",
            "sequence": "${{ needs.sign.outputs.sequence }}",
            "snapshot_digest": "${{ needs.sign.outputs.snapshot_digest }}",
            "source_sha": "${{ needs.sign.outputs.marker_commit }}",
            "workflow_sha": "${{ github.sha }}",
            "signed_ledger_sha": "${{ needs.sign.outputs.ledger_commit }}",
            "materialized_ledger_sha": "${{ needs.materialize_site.outputs.publication_commit }}",
            "baseline_ledger_sha": "${{ needs.gate_exact_staged_publication.outputs.baseline_ledger_commit }}",
        })
        self.assertNotIn("launch_approved", gate["if"] + deploy["if"])
        self.assertNotIn("gate_launch_approval", deploy["needs"])
        self.assertNotIn("skipped", deploy["if"])
        self.assertNotIn("||", deploy["if"])
        self.assertIn("needs.required_catalog_readiness.outputs.run_attempt == github.run_attempt", deploy["if"])
        for job in ("sign", "materialize_site", "gate_exact_staged_publication", "required_catalog_readiness"):
            self.assertIn(f"needs.{job}.result == 'success'", deploy["if"])
        exact = workflow["jobs"]["gate_exact_staged_publication"]
        baseline = next(step for step in exact["steps"] if step.get("id") == "baseline")
        self.assertIn("production-marker.json", baseline["run"])
        self.assertIn("bootstrap_materialized_commit", baseline["run"])
        self.assertIn('merge-base --is-ancestor "$baseline" "$SIGNED_LEDGER_COMMIT"', baseline["run"])
        self.assertNotIn("HEAD^", baseline["run"])

    def test_catalog_producer_and_attester_have_separate_credential_boundaries(self) -> None:
        workflow = load(CATALOG_READINESS)
        self.assertEqual(set(workflow["on"]), {"workflow_call"})
        producer, attester, verifier = (workflow["jobs"][name] for name in ("produce", "attest", "verify"))
        self.assertEqual(producer["permissions"], {"contents": "read", "attestations": "read"})
        for job in (producer, attester, verifier):
            self.assertNotIn("environment", job)
            self.assertNotIn("secrets.", yaml.safe_dump(job))
            self.assertNotIn("self-hosted", job["runs-on"])
            for step in job["steps"]:
                if step.get("uses", "").startswith("actions/checkout"):
                    self.assertEqual(step["with"]["persist-credentials"], "false")
        self.assertEqual(attester["permissions"]["id-token"], "write")
        self.assertEqual(attester["permissions"]["attestations"], "write")
        self.assertNotIn("id-token", verifier["permissions"])
        for job in (attester, verifier):
            body = commands(job)
            self.assertIn("catalog_publication_readiness.py verify", body)
            self.assertNotIn("catalog_publication_readiness.py produce", body)
            self.assertNotIn("npm install", body)
            self.assertNotIn("npx ", body)
        for step in producer["steps"]:
            if "GH_TOKEN" in step.get("env", {}):
                self.assertIn("Download and authenticate", step["name"])
                self.assertNotIn("catalog_publication_readiness.py", step["run"])
                self.assertNotIn(" version", step["run"])
        self.assertEqual(attester["needs"], "produce")
        self.assertIn("needs.produce.result == 'success'", attester["if"])
        self.assertEqual(set(verifier["needs"]), {"produce", "attest"})
        self.assertIn("needs.produce.result == 'success'", verifier["if"])
        self.assertIn("needs.attest.result == 'success'", verifier["if"])

    def test_production_baseline_distinguishes_missing_marker_from_remote_failure(self) -> None:
        job = load(DIRECTORY_PUBLICATION)["jobs"]["gate_exact_staged_publication"]
        body = next(step["run"] for step in job["steps"] if step.get("id") == "baseline")
        stub = '''git() {
          if test "$3" = ls-remote; then
            case "$REMOTE_CASE" in
              absent) return 0 ;;
              present) printf '%s\\t%s\\n' "$REMOTE_SHA" "$6"; return 0 ;;
              failed) return 128 ;;
              failed_with_output) printf '%s\\t%s\\n' "$REMOTE_SHA" "$6"; return 128 ;;
            esac
          fi
          return 0
        }
        '''
        for remote_case, expected in (("absent", "a" * 40), ("present", "b" * 40), ("failed", None), ("failed_with_output", None)):
            with self.subTest(remote_case=remote_case), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                marker_root = root / "trusted-source/registry/publication"
                marker_root.mkdir(parents=True)
                (marker_root / "production-marker.json").write_text(json.dumps({
                    "marker_ref": "refs/tags/directory-publication-schema-1-production",
                    "bootstrap_materialized_commit": "a" * 40,
                }))
                output = root / "output"
                environment = {
                    "PATH": os.environ["PATH"], "REMOTE_CASE": remote_case, "REMOTE_SHA": "b" * 40,
                    "SIGNED_LEDGER_COMMIT": "c" * 40, "GITHUB_OUTPUT": str(output),
                }
                result = subprocess.run(["bash", "-e", "-c", stub + body], cwd=root, env=environment, capture_output=True)
                if expected is None:
                    self.assertNotEqual(result.returncode, 0)
                    self.assertFalse(output.exists())
                    self.assertIn(b"Cannot authenticate", result.stderr)
                else:
                    self.assertEqual(result.returncode, 0, result.stderr.decode())
                    self.assertEqual(output.read_text(), f"commit={expected}\n")

    def test_catalog_isolation_is_required_on_matching_linux_ci_and_producer_runners(self) -> None:
        producer = load(CATALOG_READINESS)["jobs"]["produce"]
        portable = load(VALIDATE)["jobs"]["portable-catalog"]
        self.assertEqual(producer["runs-on"], "ubuntu-24.04")
        self.assertEqual(portable["runs-on"], producer["runs-on"])
        for job in (producer, portable):
            install = next(step for step in job["steps"] if step.get("name") == "Install the current stable Ubuntu bubblewrap package and record namespace policy")
            preflight = next(step for step in job["steps"] if step.get("name", "").startswith("Require Linux child confinement"))
            self.assertIn("sudo apt-get update", install["run"])
            self.assertIn('"bubblewrap=$candidate"', install["run"])
            self.assertIn("dpkg-query", install["run"])
            self.assertIn("apparmor_restrict_unprivileged_userns", install["run"])
            self.assertNotIn("sysctl -w", install["run"])
            self.assertNotIn("systemctl", install["run"])
            self.assertNotIn("chmod", install["run"])
            self.assertNotIn("if", preflight)
            self.assertNotIn("continue-on-error", preflight)
            self.assertEqual(preflight["run"], "python3 -m unittest tests.test_catalog_process_isolation -v")
            self.assertLess(job["steps"].index(install), job["steps"].index(preflight))
        install_client = next(step for step in producer["steps"] if step.get("name") == "Install exact public clients without account credentials")
        preflight = next(step for step in producer["steps"] if step.get("name", "").startswith("Require Linux child confinement"))
        self.assertLess(producer["steps"].index(preflight), producer["steps"].index(install_client))
        self.assertIn("from catalog_process_isolation import run_isolated", install_client["run"])
        self.assertIn("writable_root=root.resolve()", install_client["run"])
        self.assertIn("read_only_paths=(node_root,)", install_client["run"])
        self.assertNotRegex(install_client["run"], r"(?m)^\s*npm (?:install|audit)")

    def test_catalog_npm_install_and_audit_use_only_the_isolated_allowlisted_environment(self) -> None:
        producer = load(CATALOG_READINESS)["jobs"]["produce"]
        install = next(step for step in producer["steps"] if step.get("name") == "Install exact public clients without account credentials")
        code = install["run"].split("python3 - <<'PY'\n", 1)[1].split("\nPY", 1)[0]
        prelude = '''import json,os,shutil,subprocess,sys,types
from pathlib import Path
shutil.which=lambda name: '/usr/bin/true'
module=types.ModuleType('catalog_process_isolation')
def isolated(argv, **kwargs):
    root=Path(os.environ['TOOLS_ROOT']).resolve()
    assert kwargs['writable_root'] == kwargs['cwd'] == root
    assert kwargs['timeout'] == 600
    assert 'GITHUB_TOKEN' not in kwargs['env']
    assert kwargs['env']['HOME'] == str(root/'home')
    assert kwargs['env']['NPM_CONFIG_CACHE'] == str(root/'npm-cache')
    assert kwargs['read_only_paths'] == (Path('/usr'),)
    if argv[1] == 'install':
        for name,key in (('@anthropic-ai/claude-code','CLAUDE_CODE_VERSION'),('@github/copilot','COPILOT_VERSION'),('@modelcontextprotocol/inspector','INSPECTOR_VERSION')):
            path=root/'node_modules'/name/'package.json'
            path.parent.mkdir(parents=True,exist_ok=True)
            path.write_text(json.dumps({'version':os.environ[key]}))
    with (root/'calls').open('a') as stream: stream.write(argv[1]+'\\n')
    return subprocess.CompletedProcess(argv, 1 if os.environ['FAIL_PHASE']==argv[1] else 0, '', '')
module.run_isolated=isolated
sys.modules['catalog_process_isolation']=module
'''
        for phase, expected in (("none", True), ("install", False), ("audit", False)):
            with self.subTest(phase=phase), tempfile.TemporaryDirectory() as directory:
                root = Path(directory) / "tools"
                environment = {
                    "PATH": os.environ["PATH"], "TOOLS_ROOT": str(root), "FAIL_PHASE": phase,
                    "GITHUB_TOKEN": "fixture-must-not-reach-child",
                    "CLAUDE_CODE_VERSION": "2.1.251", "COPILOT_VERSION": "1.0.82", "INSPECTOR_VERSION": "2.1.0",
                }
                result = subprocess.run(["python3", "-c", prelude + code], cwd=directory, env=environment, capture_output=True)
                self.assertEqual(result.returncode == 0, expected, result.stderr.decode())
                self.assertEqual((root / "calls").read_text(), "install\n" if phase == "install" else "install\naudit\n")

    def test_every_full_python_suite_consumer_prepares_mandatory_linux_isolation(self) -> None:
        producer = load(CATALOG_READINESS)["jobs"]["produce"]
        install_name = "Install the current stable Ubuntu bubblewrap package and record namespace policy"
        required_install = next(step["run"] for step in producer["steps"] if step.get("name") == install_name)
        consumers = set()
        for path in sorted((ROOT / ".github/workflows").glob("*.y*ml")):
            for name, job in load(path).get("jobs", {}).items():
                steps = job.get("steps", [])
                for suite_index, step in enumerate(steps):
                    if not re.search(r"\bunittest(?:\s+-[^\s]+)*\s+discover\b", step.get("run", "")):
                        continue
                    consumers.add((path.name, name))
                    with self.subTest(workflow=path.name, job=name):
                        self.assertEqual(job["runs-on"], "ubuntu-24.04")
                        install = next((item for item in steps[:suite_index] if item.get("name") == install_name), None)
                        self.assertIsNotNone(install, "full discovery requires bubblewrap setup before the suite")
                        self.assertEqual(install["run"], required_install)
                        self.assertNotIn("if", install)
                        self.assertNotIn("continue-on-error", install)
                        preflight = next((item for item in steps[:suite_index] if item.get("name", "").startswith("Require Linux child confinement")), None)
                        self.assertIsNotNone(preflight, "full discovery must not silently skip Linux isolation")
                        self.assertEqual(preflight["run"], "python3 -m unittest tests.test_catalog_process_isolation -v")
                        self.assertNotIn("if", preflight)
                        self.assertNotIn("continue-on-error", preflight)
                        self.assertLess(steps.index(install), steps.index(preflight))
                        profile = next((item for item in steps[:steps.index(preflight)] if item.get("id") == "isolation_profile"), None)
                        self.assertIsNotNone(profile, "Linux isolation requires its per-executable AppArmor allowance")
                        cleanup = next((item for item in steps[suite_index + 1:] if item.get("name") == "Remove only this job's ephemeral AppArmor allowance"), None)
                        self.assertIsNotNone(cleanup, "every full-suite consumer must remove its own allowance")
        self.assertTrue({("pages.yml", "build"), ("validate.yml", "portable-catalog")} <= consumers)

    def test_catalog_apparmor_allowance_is_exact_root_owned_and_always_cleaned(self) -> None:
        rules = [line.strip() for line in (ROOT / "tests/e2e/catalog-bwrap.apparmor").read_text().splitlines() if line.strip() and not line.lstrip().startswith("#")]
        self.assertEqual(rules, [
            "abi <abi/4.0>,", "include <tunables/global>",
            "profile uap-catalog-bwrap /usr/bin/bwrap flags=(unconfined) {", "userns,", "}",
        ])
        jobs = (load(CATALOG_READINESS)["jobs"]["produce"], load(VALIDATE)["jobs"]["portable-catalog"], load(PAGES)["jobs"]["build"])
        profiles, cleanups = [], []
        for job in jobs:
            steps = job["steps"]
            profile = next(step for step in steps if step.get("id") == "isolation_profile")
            cleanup = next(step for step in steps if step.get("name") == "Remove only this job's ephemeral AppArmor allowance")
            preflight = next(step for step in steps if step.get("name", "").startswith("Require Linux child confinement"))
            self.assertNotIn("if", profile)
            self.assertNotIn("continue-on-error", profile)
            self.assertLess(steps.index(profile), steps.index(preflight))
            self.assertIs(steps[-1], cleanup)
            self.assertEqual(cleanup["if"], "${{ always() && steps.isolation_profile.outputs.profile_owned == 'true' }}")
            body = profile["run"]
            for guard in (
                'test "$GITHUB_ACTIONS" = true', 'test ! -e "$profile"', 'test ! -L "$profile"',
                "root:root:755", "root:root:644", "sudo grep -R -q -F /usr/bin/bwrap /etc/apparmor.d",
                "^uap-catalog-bwrap ", "--skip-kernel-load --skip-cache tests/e2e/catalog-bwrap.apparmor",
                "sudo install -o root -g root -m 0644", '--add --skip-cache "$profile"',
            ):
                self.assertIn(guard, body)
            self.assertLess(body.index("--skip-kernel-load"), body.index('echo "profile_owned=true"'))
            self.assertLess(body.index('echo "profile_owned=true"'), body.index("sudo install"))
            self.assertNotIn("--replace", body)
            self.assertIn('--remove --skip-cache "$profile" || status=$?', cleanup["run"])
            self.assertIn('sudo rm -- "$profile"', cleanup["run"])
            self.assertIn('exit "$status"', cleanup["run"])
            for forbidden in ("sysctl -w", "/etc/sysctl", "systemctl", "aa-disable", "rm -r", "sudo bwrap", "|| true"):
                self.assertNotIn(forbidden, body + cleanup["run"])
            profiles.append(body)
            cleanups.append(cleanup["run"])
        self.assertEqual(profiles, [profiles[0]] * 3)
        self.assertEqual(cleanups, [cleanups[0]] * 3)

    def test_catalog_gate_attests_both_canonical_subjects_and_exact_oidc_invocation(self) -> None:
        workflow = load(CATALOG_READINESS)
        producer, attester, verifier = (workflow["jobs"][name] for name in ("produce", "attest", "verify"))
        policy = next(step for step in producer["steps"] if step.get("name") == "Require all eleven exact-source policy cases")
        self.assertNotIn("if", policy)
        self.assertNotIn("continue-on-error", policy)
        self.assertIn("--schema-version 2", policy["run"])
        self.assertIn("run_source_policy_conformance.py", policy["run"])
        self.assertEqual(workflow["env"]["AGENTPLUGINS_VERSION"], "0.1.26")
        self.assertEqual(workflow["env"]["AGENTPLUGINS_COMMIT"], "24c2a74340d382abdc03a9f65563b951a9c1fcfb")
        self.assertEqual(workflow["env"]["SOURCE_POLICY_AGENTPLUGINS_VERSION"], "0.1.24")
        self.assertEqual(workflow["env"]["SOURCE_POLICY_AGENTPLUGINS_COMMIT"], "c78c79e44efd5ad07083d63436d9170b107df6cb")
        self.assertIn('ref: c78c79e44efd5ad07083d63436d9170b107df6cb', CATALOG_READINESS.read_text())
        policy_release = next(step for step in producer["steps"] if step.get("name") == "Download and authenticate the frozen source-policy release")
        self.assertIn('--source-digest "$SOURCE_POLICY_AGENTPLUGINS_COMMIT"', policy_release["run"])
        self.assertIn("623fb73d0e2f59da8b01399842b0d82b8f6456c6e43db2251c0ea5f9e32f37e3", policy_release["run"])
        self.assertIn("eb834da8237b13ed36061aeafb4fbb6f4aadeb5a6fbd4a31d43781f456f3d1e2", policy_release["run"])
        self.assertIn("source-policy-release/release-manifest.json", policy["run"])
        self.assertIn("source-policy-release/checksums.txt", policy["run"])
        self.assertEqual(workflow["env"]["COPILOT_VERSION"], "1.0.82")
        self.assertEqual(workflow["env"]["CLAUDE_CODE_VERSION"], "2.1.251")
        self.assertEqual(workflow["env"]["INSPECTOR_VERSION"], "2.1.0")
        self.assertIn('--inspector "$RUNNER_TEMP/catalog-tools/node_modules/.bin/mcp-inspector"', commands(producer))
        failure = next(step for step in producer["steps"] if step.get("name") == "Preserve bounded sanitized failure diagnostics only")
        self.assertEqual(failure["if"], "${{ failure() }}")
        self.assertEqual(failure["with"]["path"], "evidence/catalog-readiness-failure.json")
        self.assertNotIn("*", failure["with"]["path"])
        attest_step = next(step for step in attester["steps"] if step.get("id") == "attestation")
        self.assertEqual(set(attest_step["with"]["subject-path"].splitlines()), {
            "evidence/catalog-readiness.json", "evidence/source-policy-conformance.json",
        })
        attest_index = attester["steps"].index(attest_step)
        self.assertIn("catalog_publication_readiness.py verify", attester["steps"][attest_index - 1]["run"])
        for job in (attester, verifier):
            body = commands(job)
            self.assertIn("validate_source_policy_evidence", body)
            self.assertIn("expected_schema_version=2", body)
            self.assertIn("canonical_json", body)
            for argument in (
                "--publication-id", "--sequence", "--snapshot-digest", "--source-sha", "--workflow-sha",
                "--signed-ledger-sha", "--materialized-ledger-sha", "--run-id", "--run-attempt", "--baseline-feed",
                "--directory-origin",
            ):
                self.assertIn(argument, body)
        gate = commands(verifier)
        for binding in (
            '--signer-digest "$WORKFLOW_SHA"', '--source-digest "$WORKFLOW_SHA"', "--source-ref refs/heads/main",
            "--deny-self-hosted-runners", "catalog-publication-readiness.yml", 'certificate["runInvocationURI"] == invocation',
            'certificate["buildConfigURI"]', 'statement["predicate"]["runDetails"]["metadata"]["invocationId"] == invocation',
        ):
            self.assertIn(binding, gate)
        self.assertIn("for subject in catalog-readiness.json source-policy-conformance.json", gate)
        self.assertIn('echo "run_attempt=$GITHUB_RUN_ATTEMPT"', gate)
        self.assertEqual(workflow["on"]["workflow_call"]["outputs"]["run_attempt"]["value"], "${{ jobs.verify.outputs.run_attempt }}")

    def test_account_runtime_mode_is_explicit_dispatch_exact_resume_and_cannot_deploy(self) -> None:
        workflow = load(DIRECTORY_PUBLICATION)
        mode = workflow["on"]["workflow_dispatch"]["inputs"]["publication_mode"]
        self.assertEqual(mode["default"], "catalog")
        self.assertEqual(mode["options"], ["catalog", "account-runtime-evidence"])
        for name in ("required_stable_launch_evidence", "record_launch_approval", "gate_launch_approval"):
            self.assertIn("github.event_name == 'workflow_dispatch'", workflow["jobs"][name]["if"])
            self.assertIn("inputs.publication_mode == 'account-runtime-evidence'", workflow["jobs"][name]["if"])
        self.assertIn("inputs.publication_mode != 'account-runtime-evidence'", workflow["jobs"]["deploy"]["if"])
        validator = workflow["jobs"]["authenticate-completed-state"]["steps"][0]
        base_env = {
            "PATH": os.environ["PATH"], "PUBLICATION_MODE": "account-runtime-evidence", "EVENT_NAME": "workflow_dispatch",
            "RESUME_ID": "123", "RESUME_SEQUENCE": "19", "INITIALIZE_LEDGER": "false", "SUPERSEDE_ID": "", "SUPERSEDE_SEQUENCE": "",
        }
        cases = [({}, True), ({"PUBLICATION_MODE": "catalog", "RESUME_ID": "", "RESUME_SEQUENCE": ""}, True)]
        cases.extend((changes, False) for changes in (
            {"PUBLICATION_MODE": "unknown"}, {"EVENT_NAME": "push"}, {"EVENT_NAME": "schedule"},
            {"RESUME_ID": ""}, {"RESUME_SEQUENCE": ""}, {"INITIALIZE_LEDGER": "true"},
            {"SUPERSEDE_ID": "123"}, {"SUPERSEDE_SEQUENCE": "19"},
        ))
        for changes, expected in cases:
            with self.subTest(changes=changes):
                result = subprocess.run(["bash", "-e", "-c", validator["run"]], env={**base_env, **changes}, capture_output=True)
                self.assertEqual(result.returncode == 0, expected)
        # Existing post-promotion resume restriction is intentionally unchanged.
        resume = next(step for step in workflow["jobs"]["prepare"]["steps"] if step.get("id") == "resume")
        self.assertIn('test "${production_sequence}" -lt "${sequence}"', resume["run"])

    def test_catalog_verified_provenance_rejects_stale_forged_and_ambiguous_invocations(self) -> None:
        verifier = load(CATALOG_READINESS)["jobs"]["verify"]
        gate = next(step for step in verifier["steps"] if step.get("id") == "gate")
        code = gate["run"].split("python3 - <<'PY'\n", 1)[1].split("\nPY\n", 1)[0]
        repository = "777genius/universal-agent-plugins"
        invocation = f"https://github.com/{repository}/actions/runs/123/attempts/2"
        result = [{"verificationResult": {
            "signature": {"certificate": {
                "runInvocationURI": invocation,
                "buildConfigURI": f"https://github.com/{repository}/.github/workflows/directory-publication.yml@refs/heads/main",
            }},
            "statement": {
                "predicateType": "https://slsa.dev/provenance/v1",
                "predicate": {"runDetails": {"metadata": {"invocationId": invocation}}},
            },
        }}]
        cases = [("valid", result, True)]
        for label, mutation in (
            ("stale-certificate-attempt", lambda item: item["signature"]["certificate"].update(runInvocationURI=invocation.replace("attempts/2", "attempts/1"))),
            ("different-certificate-run", lambda item: item["signature"]["certificate"].update(runInvocationURI=invocation.replace("runs/123", "runs/456"))),
            ("wrong-caller", lambda item: item["signature"]["certificate"].update(buildConfigURI=f"https://github.com/{repository}/.github/workflows/other.yml@refs/heads/main")),
            ("wrong-predicate", lambda item: item["statement"].update(predicateType="https://example.invalid/predicate")),
            ("forged-statement-attempt", lambda item: item["statement"]["predicate"]["runDetails"]["metadata"].update(invocationId=invocation.replace("attempts/2", "attempts/1"))),
            ("missing-certificate-invocation", lambda item: item["signature"]["certificate"].pop("runInvocationURI")),
        ):
            changed = copy.deepcopy(result)
            mutation(changed[0]["verificationResult"])
            cases.append((label, changed, False))
        cases.extend((("empty", [], False), ("ambiguous", result + result, False)))
        # Isolate the provenance assertion block; policy validation has its own real validators/tests.
        prelude = (
            "import json,sys,types\n"
            "module=types.ModuleType('two_lane_evidence')\n"
            "module.canonical_json=lambda value: json.dumps(value).encode()\n"
            "module.sha256_file=lambda path: 'unused-policy-fixture-digest'\n"
            "module.validate_source_policy_evidence=lambda *args,**kwargs: None\n"
            "sys.modules['two_lane_evidence']=module\n"
        )
        for name, verified, expected in cases:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                (root / "evidence").mkdir()
                (root / "evidence/source-policy-conformance.json").write_bytes(b"{}")
                for subject in ("catalog-readiness.json", "source-policy-conformance.json"):
                    (root / f"{subject}.verified.json").write_text(json.dumps(verified))
                environment = {
                    "PATH": os.environ["PATH"], "RUNNER_TEMP": directory,
                    "GITHUB_REPOSITORY": repository, "GITHUB_RUN_ID": "123", "GITHUB_RUN_ATTEMPT": "2",
                    "WORKFLOW_SHA": "a" * 40,
                }
                completed = subprocess.run(["python3", "-c", prelude + code], cwd=root, env=environment, capture_output=True)
                self.assertEqual(completed.returncode == 0, expected, completed.stderr.decode())

    def test_every_launch_harness_invocation_supplies_the_explicit_uap_sha(self) -> None:
        command = re.compile(r"run_launch_evidence_e2e\.py(?:[^\n]*\\\n)*[^\n]*")
        observed = []
        for path in (LAUNCH, LIVE):
            workflow = load(path)
            body = "\n".join(commands(job) for job in workflow["jobs"].values() if "steps" in job)
            matches = command.findall(body)
            observed.extend((path, invocation) for invocation in matches)
            for invocation in matches:
                with self.subTest(path=path.name, invocation=invocation):
                    self.assertIn("--uap-sha", invocation)
        documentation = ROOT / "tests/e2e/LAUNCH_EVIDENCE.md"
        for invocation in command.findall(documentation.read_text()):
            observed.append((documentation, invocation))
            self.assertIn("--uap-sha", invocation)
        self.assertEqual(len(observed), 4)

    def test_two_lane_evidence_is_independent_until_canonical_readiness(self) -> None:
        launch = load(LAUNCH)
        runtime = launch["jobs"]["enforced-stable-gate"]
        policy = launch["jobs"]["source-policy-conformance"]
        readiness = launch["jobs"]["two-lane-readiness"]
        self.assertNotIn("source-policy-conformance", runtime["needs"])
        self.assertEqual(set(readiness["needs"]), {"attest-stable-evidence", "source-policy-conformance"})
        self.assertIn("needs.attest-stable-evidence.result == 'success'", readiness["if"])
        self.assertIn("needs.source-policy-conformance.result == 'success'", readiness["if"])
        for job in (policy, readiness):
            self.assertEqual(job["steps"][0]["name"], "Validate the unmodified canonical publication sequence")
        policy_body = commands(policy)
        self.assertIn("c78c79e44efd5ad07083d63436d9170b107df6cb", yaml.safe_dump(policy))
        self.assertIn("agentplugins-v0.1.24", policy_body)
        self.assertIn("--schema-version 2", policy_body)
        self.assertIn("run_source_policy_conformance.py", policy_body)
        self.assertNotIn("DIRECTORY_SIGNING", yaml.safe_dump(policy))
        readiness_body = commands(readiness)
        self.assertIn("build_two_lane_readiness.py", readiness_body)
        for argument in (
            "--uap-sha", "--directory-ledger-sha", "--publication-id",
            "--publication-sequence", "--publication-snapshot-digest",
            "--publication-source-commit",
        ):
            self.assertIn(argument, readiness_body)
        self.assertIn("--schema-version 2", readiness_body)
        self.assertIn("two-lane-readiness-v2.schema.json", readiness_body)

    def test_directory_completed_readiness_replay_uses_exact_authenticated_identity(self) -> None:
        publication = load(DIRECTORY_PUBLICATION)
        body = "\n".join(commands(job) for job in publication["jobs"].values() if "steps" in job)
        invocation_pattern = re.compile(
            r"python3 trusted-source/scripts/build_two_lane_readiness\.py(?:[^\n]*\\\n)*[^\n]*"
        )
        invocations = invocation_pattern.findall(body)
        self.assertEqual(len(invocations), 1, "stale or duplicate production readiness invocation")
        self.assertEqual(invocations[0], """python3 trusted-source/scripts/build_two_lane_readiness.py \\
  --runtime launch-evidence/launch-evidence.json \\
  --policy source-policy-evidence/source-policy-conformance.json \\
  --uap-sha "${WORKFLOW_SOURCE_DIGEST}" \\
  --directory-ledger-sha "${EXPECTED_LEDGER_COMMIT}" \\
  --publication-id "${EXPECTED_PUBLICATION_ID}" \\
  --publication-sequence "${EXPECTED_PUBLICATION_SEQUENCE}" \\
  --publication-snapshot-digest "${EXPECTED_SNAPSHOT_DIGEST}" \\
  --publication-source-commit "${EXPECTED_SOURCE_COMMIT}" \\
  --schema-version 2 \\
  --completed two-lane-readiness/two-lane-readiness.json""")

    def test_source_policy_transport_contains_no_released_executable(self) -> None:
        launch = load(LAUNCH)
        producer = launch["jobs"]["node22-npm-facade"]
        policy = launch["jobs"]["source-policy-conformance"]
        upload = next(step for step in producer["steps"] if step.get("with", {}).get("name", "").startswith("prepared-policy-inputs-"))
        self.assertEqual(set(upload["with"]["path"].splitlines()), {
            "prepared-policy-inputs/release-manifest.json", "prepared-policy-inputs/checksums.txt",
        })
        downloads = [step["with"]["name"] for step in policy["steps"] if "download-artifact" in step.get("uses", "")]
        self.assertEqual(len(downloads), 1)
        self.assertTrue(downloads[0].startswith("prepared-policy-inputs-"))
        self.assertNotIn("prepared-producer-inputs", yaml.safe_dump(policy))
        self.assertNotRegex(yaml.safe_dump(policy), r"agentplugins_0\.1\.18_(?:linux|darwin|windows)")

    def test_public_sequence_inputs_stay_canonical_strings_across_reusable_edges(self) -> None:
        launch = load(LAUNCH)
        live = load(LIVE)
        publication = load(DIRECTORY_PUBLICATION)
        for workflow in (launch, live):
            for trigger in ("workflow_call", "workflow_dispatch"):
                self.assertEqual(workflow["on"][trigger]["inputs"]["publication_sequence"]["type"], "string")
            gate = workflow["jobs"]["validate-publication-sequence"]
            first = gate["steps"][0]
            self.assertNotIn("uses", first)
            self.assertEqual(first["env"]["PUBLICATION_SEQUENCE"], "${{ inputs.publication_sequence }}")
            script = first["run"]
            self.assertNotIn("fromJSON", script)
            accepted = ("1", "9007199254740991")
            rejected = (
                "", "0", "+1", "-1", " 1", "1 ", "01", "1e3", "1.0",
                "9007199254740992", "9007199254740993", "12x", "12_", "12 ", "12/evil",
            )
            for value in accepted + rejected:
                completed = subprocess.run(
                    ["/bin/bash", "-c", script], env={
                        "PATH": "/usr/bin:/bin:/usr/local/bin", "EVENT_NAME": "workflow_dispatch",
                        "PUBLICATION_SEQUENCE": value,
                    },
                    text=True, capture_output=True,
                )
                with self.subTest(workflow=workflow["name"], value=value):
                    self.assertEqual(completed.returncode == 0, value in accepted, completed.stderr)
            if workflow is live:
                scheduled = subprocess.run(
                    ["/bin/bash", "-c", script], env={
                        "PATH": "/usr/bin:/bin:/usr/local/bin", "EVENT_NAME": "schedule",
                        "PUBLICATION_SEQUENCE": "",
                    }, text=True, capture_output=True,
                )
                self.assertEqual(scheduled.returncode, 0, scheduled.stderr)
        for name in (
            "native-release", "node22-npm-facade", "aggregate-one-release",
            "protected-observer-inputs", "enforced-stable-gate", "attest-stable-evidence",
        ):
            first = launch["jobs"][name]["steps"][0]
            self.assertEqual(first["env"]["PUBLICATION_SEQUENCE"], "${{ inputs.publication_sequence }}")
            self.assertNotIn("uses", first)
        caller = publication["jobs"]["required_stable_launch_evidence"]["with"]["publication_sequence"]
        self.assertEqual(caller, "${{ needs.sign.outputs.sequence }}")
        for path in (LAUNCH, LIVE, DIRECTORY_PUBLICATION):
            self.assertNotRegex(path.read_text(), r"fromJSON\([^)]*(?:publication|snapshot|release|sequence)")

    def test_upstream_promotion_readiness_is_manual_and_read_only(self) -> None:
        workflow = load(UPSTREAM_PROMOTION)
        self.assertEqual(set(workflow["on"]), {"workflow_dispatch"})
        self.assertEqual(set(workflow["on"]["workflow_dispatch"]["inputs"]), {
            "upstream_pr_number", "repository_id", "package_path", "review_record_json",
        })
        self.assertEqual(workflow["on"]["workflow_dispatch"]["inputs"]["upstream_pr_number"]["type"], "number")
        self.assertEqual(workflow["permissions"], {"contents": "read"})
        self.assertEqual(set(workflow["jobs"]), {"readiness"})
        job = workflow["jobs"]["readiness"]
        self.assertEqual(job["runs-on"], "ubuntu-latest")
        self.assertEqual(job["timeout-minutes"], "30")
        body = commands(job)
        self.assertEqual(pinned_requirements(body), {"jsonschema==4.25.1", "PyYAML==6.0.3"})
        self.assertIn("gh repo view", body)
        self.assertIn("gh pr view", body)
        self.assertIn('"state": pr.get("state")', body)
        self.assertIn('"head_ref_oid": pr.get("headRefOid")', body)
        self.assertIn('"merge_commit_oid": merge.get("oid")', body)
        self.assertIn('("base_ref_name", "default_branch")', body)
        self.assertNotIn('metadata["base_ref_name"] != metadata["default_branch"]', body)
        self.assertIn('"base_ref_name": pr.get("baseRefName")', body)
        self.assertIn('"default_branch": default.get("name")', body)
        self.assertIn("env -u GH_TOKEN GIT_TERMINAL_PROMPT=0 git -c credential.helper= clone", body)
        self.assertIn("--filter=blob:none --no-checkout --single-branch --sparse", body)
        self.assertIn("env -u GH_TOKEN GIT_TERMINAL_PROMPT=0 git -C official -c credential.helper= fetch", body)
        self.assertIn("refs/pull/${UPSTREAM_PR_NUMBER}/head", body)
        self.assertIn('test "$(git -C official rev-parse', body)
        self.assertIn('sparse-checkout set --no-cone "/${PACKAGE_PATH}/"', body)
        self.assertIn('checkout --detach "$HEAD_OID"', body)
        self.assertIn("validate_review_journey.py promotion", body)
        self.assertIn("--pr-metadata pr-metadata.json", body)
        for forbidden_input in ("reviewed_pr_head_sha", "official_candidate_sha", "official_default_branch"):
            self.assertNotIn(forbidden_input, workflow["on"]["workflow_dispatch"]["inputs"])
        self.assertNotIn("git push", body)
        self.assertNotRegex(body, r"\bgh pr (?:create|merge)\b")
        dumped = yaml.safe_dump(workflow)
        for forbidden in ("contents: write", "pull-requests: write", "id-token: write", "secrets.", "SIGNING", "PUBLISHER"):
            self.assertNotIn(forbidden, dumped)
        upload = next(step for step in job["steps"] if step.get("uses", "").startswith("actions/upload-artifact"))
        self.assertEqual(set(upload["with"]["path"].splitlines()), {"pr-metadata.json", "promotion-candidate.json", "readiness-diagnostics.json"})

    def test_adapter_egress_and_native_projection_are_canonical_generic_contracts(self) -> None:
        schema = json.loads((ROOT / "deploy/uap-observer-adapter-config.schema.json").read_text())
        self.assertIn("egress_hosts", schema["required"])
        egress = schema["properties"]["egress_hosts"]
        self.assertTrue(egress["uniqueItems"])
        pattern = re.compile(egress["items"]["pattern"])
        self.assertIsNotNone(pattern.fullmatch("api.github.com"))
        for rejected in ("API.GITHUB.COM", "127.0.0.1", "*.github.com", "github.com.", "localhost"):
            self.assertIsNone(pattern.fullmatch(rejected))
        client = schema["$defs"]["client"]
        release_tuple = schema["$defs"]["tuple"]["properties"]
        self.assertEqual(release_tuple["release_sequence"]["maximum"], 9_007_199_254_740_991)
        self.assertEqual(release_tuple["snapshot_sequence"]["maximum"], 9_007_199_254_740_991)
        self.assertIn("native_projection", client["required"])
        self.assertEqual(set(client["properties"]["native_projection"]["required"]), {"path", "sha256"})
        codex_rule = client["allOf"][0]["then"]
        self.assertEqual(codex_rule["required"], ["companion_binary", "companion_sha256"])
        self.assertEqual(
            codex_rule["properties"]["companion_binary"]["const"],
            "/opt/uap-observer-inputs/bin/codex-code-mode-host",
        )
        cursor_rule = client["allOf"][1]["then"]
        self.assertIn("bundle", cursor_rule["required"])
        self.assertEqual(cursor_rule["properties"]["binary"]["const"], "/opt/uap-observer-inputs/cursor/cursor-agent")
        self.assertEqual(schema["$defs"]["bundle"]["properties"]["manifest"]["const"], "/opt/uap-observer-inputs/cursor-bundle.json")
        self.assertIn("chrome_for_testing", schema["required"])
        chrome = schema["$defs"]["chrome_for_testing"]
        self.assertEqual(chrome["properties"]["root"]["const"], "/opt/uap-observer-inputs/chrome-for-testing")
        self.assertEqual(chrome["properties"]["manifest"]["const"], "/opt/uap-observer-inputs/chrome-for-testing-bundle.json")
        self.assertEqual(chrome["properties"]["binary"]["const"], "/opt/uap-observer-inputs/chrome-for-testing/chrome")
        self.assertEqual(chrome["properties"]["version"]["const"], "152.0.7977.64")
        kiro_rule = client["allOf"][2]["then"]
        self.assertEqual(kiro_rule["required"], ["companion_binary", "companion_sha256"])
        self.assertEqual(kiro_rule["properties"]["sha256"]["const"], "sha256:14d835aff3772afb9ffb71e395b433df516c091dea8c43daef46e7cb66368358")
        self.assertEqual(kiro_rule["properties"]["companion_sha256"]["const"], "sha256:59f47eb75928fa158df1cea31382cb39a4eb0d8ec7afbcfc4c6e75693d35163e")
        installer = (ROOT / "deploy/uap-observer-install.sh").read_text()
        self.assertIn('required_hosts={urlsplit(url).hostname for url in urls} | {"github.com"}', installer)
        self.assertIn("required_hosts <= set(egress_hosts)", installer)
        self.assertIn('allowlist.get("hosts") != egress_hosts', installer)
        self.assertIn('type(allowlist.get("schema_version")) is not int', installer)
        runner = (ROOT / "observer/fixed_runner.py").read_text()
        self.assertIn('Path(config["clients"]["codex"]["companion_binary"])', runner)
        self.assertIn('Path(config["clients"]["kiro"]["companion_binary"])', runner)
        self.assertIn('("cursor-agent", "node", "bash", "basename", "dirname", "realpath")', runner)
        self.assertIn("verify_bundle(", runner)
        self.assertIn("literal | {bundle_root, bundle_manifest, chrome_root, chrome_manifest}", runner)
        runner_unit = (ROOT / "deploy/uap-observer-runner.service").read_text()
        self.assertIn("BindReadOnlyPaths=/opt/uap-observer-current /var/lib/uap-observer/proofs /var/lib/uap-observer/profiles", runner_unit)
        self.assertEqual(runner_unit.splitlines().count("BindReadOnlyPaths=/opt/uap-observer-inputs"), 1)
        writable = next(line for line in runner_unit.splitlines() if line.startswith("BindPaths=-/var/lib/uap-observer/profiles/"))
        self.assertEqual(writable.split("=")[1].split(), [
            f"-/var/lib/uap-observer/profiles/{client}/{leaf}"
            for client in ("codex", "cursor", "kiro") for leaf in (".auth", ".state")
        ])
        for forbidden in ("/.config", "/.cache", "/.codex", "/.cursor", "/.kiro", "/.local"):
            self.assertNotIn(forbidden, writable)

    def test_installed_state_projection_validator_is_strict_and_nonempty(self) -> None:
        library = (ROOT / "deploy/uap-observer-install-lib.sh").read_text()
        start = library.index('heroes={"agent-code-navigator"')
        end = library.index('\ndirectory("/var/empty"', start)
        namespace = {}
        exec("import json,math,re\nfrom pathlib import Path\n" + library[start:end], namespace)
        validate = namespace["native_projection"]
        digest = "sha256:" + "a" * 64
        def release_tuple(plugin: str) -> dict:
            return {
                "product_id": plugin, "tree_digest": digest, "manifest_digest": digest,
                "distribution_id": f"owner/{plugin}", "distribution_kind": "upstream",
                "release_sequence": 1, "package_version": "1.0.0",
                "source_repository": f"owner/{plugin}", "source_revision": "b" * 40,
                "source_path": f"plugins/{plugin}", "snapshot_sequence": 1,
                "snapshot_digest": digest,
                "binary_digest": "sha256:e79125f7ffabd11c6e211d6b049c2eb2b36eb1aba3a76ce27cac819aeba1e6ca",
                "dependency_identity": "locked", "installer_version": "0.1.24",
                "adapter_version": "0.1.24", "client_version": None,
                "os": "linux", "architecture": "x86_64", "observed_at": "2026-08-26T00:00:00Z",
            }
        entry = {
            "plugin": "context7", "component_kind": "mcp", "tuple": release_tuple("context7"),
            "native_config": {"path": "/proof/codex/native/context7.blob", "sha256": digest},
            "client_config": {"path": "/profile/codex/context7.json", "sha256": digest},
            "manager_add_sha256": digest, "manager_info_sha256": digest,
            "post_add_doctor_sha256": digest,
        }
        heroes = ("agent-code-navigator", "context7", "cloudflare-docs", "chrome-devtools", "notion")
        entries = [
            {
                **entry, "plugin": plugin,
                "component_kind": "skill" if plugin == "agent-code-navigator" else "mcp",
                "tuple": release_tuple(plugin),
                "native_config": {"path": f"/proof/codex/native/{plugin}.blob", "sha256": digest},
                "client_config": {
                    "path": (
                        "/profile/codex/skills/code-tool-router/SKILL.md"
                        if plugin == "agent-code-navigator" else f"/profile/codex/{plugin}.json"
                    ),
                    "sha256": digest,
                },
            }
            for plugin in heroes
        ]
        valid = {"schema_version": 2, "client_id": "codex", "entries": entries}
        self.assertEqual(
            validate(json.dumps(valid).encode(), "codex", Path("/profile/codex"), Path("/proof/codex")),
            valid,
        )
        boundary = json.loads(json.dumps(valid))
        boundary["entries"][0]["tuple"].update(
            release_sequence=9_007_199_254_740_991,
            snapshot_sequence=9_007_199_254_740_991,
        )
        self.assertEqual(
            validate(json.dumps(boundary).encode(), "codex", Path("/profile/codex"), Path("/proof/codex")),
            boundary,
        )
        for field in ("release_sequence", "snapshot_sequence"):
            for unsafe in (9_007_199_254_740_992, 9_007_199_254_740_993):
                forged = json.loads(json.dumps(valid))
                forged["entries"][0]["tuple"][field] = unsafe
                with self.subTest(field=field, unsafe=unsafe), self.assertRaises(SystemExit):
                    validate(json.dumps(forged).encode(), "codex", Path("/profile/codex"), Path("/proof/codex"))
        kiro_entries = [{
            **item,
            "native_config": {
                "path": f'/proof/kiro/native/{item["plugin"]}.blob', "sha256": digest,
            },
            "client_config": {
                "path": (
                    "/profile/kiro/.kiro/skills/code-tool-router/SKILL.md"
                    if item["plugin"] == "agent-code-navigator"
                    else "/profile/kiro/.kiro/settings/mcp.json"
                ),
                "sha256": digest,
            },
        } for item in entries]
        valid_kiro = {"schema_version": 2, "client_id": "kiro", "entries": kiro_entries}
        self.assertEqual(
            validate(json.dumps(valid_kiro).encode(), "kiro", Path("/profile/kiro"), Path("/proof/kiro")),
            valid_kiro,
        )
        cursor_entries = [{
            **item,
            "native_config": {
                "path": f'/proof/cursor/native/{item["plugin"]}.blob', "sha256": digest,
            },
            "client_config": {
                "path": (
                    "/profile/cursor/.cursor/skills/code-tool-router/SKILL.md"
                    if item["plugin"] == "agent-code-navigator"
                    else "/profile/cursor/.cursor/mcp.json"
                ),
                "sha256": digest,
            },
        } for item in entries]
        valid_cursor = {"schema_version": 2, "client_id": "cursor", "entries": cursor_entries}
        self.assertEqual(
            validate(
                json.dumps(valid_cursor).encode(), "cursor",
                Path("/profile/cursor"), Path("/proof/cursor"),
            ),
            valid_cursor,
        )
        for conflicting in (
            {**valid_kiro, "entries": [
                {**kiro_entries[0], "client_config": {"path": "/profile/kiro/other/mcp.json", "sha256": digest}},
                *kiro_entries[1:],
            ]},
            {**valid_kiro, "entries": [
                kiro_entries[0],
                {**kiro_entries[1],
                 "native_config": {**kiro_entries[1]["native_config"], "sha256": "sha256:" + "c" * 64},
                 "client_config": {**kiro_entries[1]["client_config"], "sha256": "sha256:" + "c" * 64}},
                *kiro_entries[2:],
            ]},
            {**valid_kiro, "client_id": "codex", "entries": [{
                **item,
                "native_config": {"path": f'/proof/codex/native/{item["plugin"]}.blob', "sha256": digest},
                "client_config": {"path": "/profile/codex/.kiro/settings/mcp.json", "sha256": digest},
            } for item in entries]},
            {**valid_cursor, "entries": [
                cursor_entries[0],
                {**cursor_entries[1],
                 "native_config": {**cursor_entries[1]["native_config"], "sha256": "sha256:" + "c" * 64},
                 "client_config": {**cursor_entries[1]["client_config"], "sha256": "sha256:" + "c" * 64}},
                *cursor_entries[2:],
            ]},
            {**valid_cursor, "entries": [
                cursor_entries[0],
                {**cursor_entries[1], "client_config": {
                    "path": "/profile/cursor/other/mcp.json", "sha256": digest,
                }},
                *cursor_entries[2:],
            ]},
        ):
            with self.subTest(conflicting_shared_path=conflicting), self.assertRaises(SystemExit):
                validate(
                    json.dumps(conflicting).encode(), conflicting["client_id"],
                    Path(f'/profile/{conflicting["client_id"]}'), Path(f'/proof/{conflicting["client_id"]}'),
                )
        malformed = (
            {**valid, "schema_version": True},
            {**valid, "client_id": "cursor"},
            {**valid, "entries": []},
            {**valid, "extra": 1},
            {**valid, "entries": [{key: value for key, value in entry.items() if key != "client_config"}]},
            {**valid, "entries": [{**entry, "client_config": {"path": entry["client_config"]["path"]}}]},
            {**valid, "entries": [{**entries[0], "tuple": {"product_id": entries[0]["plugin"]}}, *entries[1:]]},
            {**valid, "entries": [{**entries[0], "tuple": {**entries[0]["tuple"], "unexpected": 1}}, *entries[1:]]},
            {**valid, "entries": [{**entries[0], "tuple": {**entries[0]["tuple"], "release_sequence": True}}, *entries[1:]]},
        )
        for value in malformed:
            with self.subTest(value=value), self.assertRaises(SystemExit):
                validate(json.dumps(value).encode(), "codex", Path("/profile/codex"), Path("/proof/codex"))
        for encoded in (
            b'{"schema_version":1,"schema_version":1,"client_id":"codex","entries":[]}',
            b'{"schema_version":1,"Schema_Version":1,"client_id":"codex","entries":[]}',
            b'{"schema_version":1,"client_id":"codex","entries":[],"value":NaN}',
            b'{"schema_version":1,"client_id":"codex","entries":[],"value":1e400}',
        ):
            with self.subTest(encoded=encoded), self.assertRaises(SystemExit):
                validate(encoded, "codex", Path("/profile/codex"), Path("/proof/codex"))
        self.assertIn('set(receipts)!={"schema_version","receipts"}', library)
        self.assertIn('type(receipts.get("schema_version")) is not int', library)
        self.assertIn('record["tuple"]!=entry["tuple"]', library)
        self.assertIn('record.get(field)!=entry.get(field)', library)

    def test_installed_state_validator_accepts_populated_projection_and_receipts(self) -> None:
        library = (ROOT / "deploy/uap-observer-install-lib.sh").read_text()
        function = library.index("observer_validate_installed_accounts_and_state()")
        start = library.index("import grp,hashlib,json,math,os,pwd,re,stat", function)
        source = library[start:library.index("\nPY\n}", start)]
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            replacements = {
                path: str(root / path.lstrip("/"))
                for path in (
                    "/var/lib/uap-observer-consent", "/var/lib/uap-observer-human",
                    "/var/lib/uap-observer", "/var/empty", "/var/lib/caddy", "/var/log/caddy",
                )
            }
            for index, original in enumerate(sorted(replacements, key=len, reverse=True)):
                token = f"__UAP_FIXTURE_PATH_{index}__"
                source = source.replace(original, token)
                replacements[token] = replacements[original]
            for token, replacement in tuple(replacements.items()):
                if token.startswith("__UAP_FIXTURE_PATH_"):
                    source = source.replace(token, replacement)
            source = source.replace(
                "from observer.fixed_runner import reviewed_service_identities",
                "owner_uid,owner_gid=os.geteuid(),os.getegid()\n"
                "def reviewed_service_identities():\n"
                "    identities={name:(owner_uid,owner_gid,'fixture') for name in "
                "('codex','cursor','kiro','control','observer','caddy')}\n"
                "    identities['egress']=(1,1,'fixture')\n"
                "    return identities",
            )
            source = source.replace(
                "from observer.fixed_adapters import verify_kiro_runtime",
                "kiro_verifications=[]\n"
                "def verify_kiro_runtime(profile,*,expected_gid,verify_tree_digest):\n"
                "    kiro_verifications.append((str(profile),expected_gid,verify_tree_digest))",
            )
            source = source.replace(
                'config_gid=grp.getgrnam("uap-observer-adapter-config").gr_gid', "config_gid=owner_gid",
            )
            source = source.replace(",0,0,", ",owner_uid,owner_gid,")
            source = source.replace(",0,gid,", ",owner_uid,gid,")
            source = source.replace(",0,identities[3][1],", ",owner_uid,identities[3][1],")
            source = source.replace(",0,config_gid,", ",owner_uid,config_gid,")
            source = source.replace("native_info.st_uid != 0", "native_info.st_uid != owner_uid")
            source = re.sub(r"(?<![A-Za-z_])info\.st_uid != 0", "info.st_uid != owner_uid", source)

            def mkdir(path: Path, mode: int) -> None:
                path.mkdir(parents=True, exist_ok=True)
                path.chmod(mode)

            empty = Path(replacements["/var/empty"])
            mkdir(empty, 0o755)
            for client in ("codex", "cursor", "kiro", "control"):
                mkdir(empty / f"uap-observer-{client}", 0o700)
            state = Path(replacements["/var/lib/uap-observer"])
            mkdir(state, 0o711)
            mkdir(state / "state", 0o700)
            for name in ("jobs", "workspaces", "profiles", "proofs"):
                mkdir(state / name, 0o711)
            for client in ("codex", "cursor", "kiro"):
                mkdir(state / "workspaces" / client, 0o700)
            for client in ("cursor", "kiro"):
                mkdir(state / "profiles" / client, 0o700)

            digest_value = "sha256:" + "a" * 64
            heroes = ("agent-code-navigator", "context7", "cloudflare-docs", "chrome-devtools", "notion")
            def populate(client: str) -> None:
                profile = state / "profiles" / client
                proof = state / "proofs" / client
                mkdir(profile, 0o700)
                mkdir(proof / "native", 0o700)
                entries, receipt_rows = [], []
                protected_directories = {profile, proof, proof / "native"}
                for plugin in heroes:
                    if client == "kiro":
                        body = (
                            b'{"skill":"code-tool-router"}' if plugin == "agent-code-navigator"
                            else b'{"mcpServers":{"fixture":{}}}'
                        )
                        active = (
                            profile / ".kiro" / "skills" / "code-tool-router" / "SKILL.md"
                            if plugin == "agent-code-navigator"
                            else profile / ".kiro" / "settings" / "mcp.json"
                        )
                    else:
                        body = json.dumps({"plugin": plugin}, separators=(",", ":")).encode()
                        active = (
                            profile / "skills" / "code-tool-router" / "SKILL.md"
                            if plugin == "agent-code-navigator" else profile / f"{plugin}.json"
                        )
                    mkdir(active.parent, 0o700)
                    if not active.exists():
                        active.write_bytes(body)
                    self.assertEqual(active.read_bytes(), body)
                    active.chmod(0o440)
                    parent = active.parent
                    while parent != profile:
                        protected_directories.add(parent); parent = parent.parent
                    native = proof / "native" / f"{plugin}.blob"
                    native.write_bytes(body); native.chmod(0o440)
                    body_digest = "sha256:" + hashlib.sha256(body).hexdigest()
                    release = {
                        "product_id": plugin, "tree_digest": digest_value, "manifest_digest": digest_value,
                        "distribution_id": f"owner/{plugin}", "distribution_kind": "upstream",
                        "release_sequence": 1, "package_version": "1.0.0",
                        "source_repository": f"owner/{plugin}", "source_revision": "b" * 40,
                        "source_path": f"plugins/{plugin}", "snapshot_sequence": 1,
                        "snapshot_digest": digest_value,
                        "binary_digest": "sha256:e79125f7ffabd11c6e211d6b049c2eb2b36eb1aba3a76ce27cac819aeba1e6ca",
                        "dependency_identity": "locked", "installer_version": "0.1.24",
                        "adapter_version": "0.1.24", "client_version": None, "os": "linux",
                        "architecture": "x86_64", "observed_at": "2026-08-30T00:00:00Z",
                    }
                    evidence = {
                        "manager_add_sha256": digest_value, "manager_info_sha256": digest_value,
                        "post_add_doctor_sha256": digest_value,
                    }
                    entries.append({
                        "plugin": plugin, "component_kind": "skill" if plugin == "agent-code-navigator" else "mcp",
                        "tuple": release, "native_config": {"path": str(native), "sha256": body_digest},
                        "client_config": {"path": str(active), "sha256": body_digest}, **evidence,
                    })
                    receipt_rows.append({"name": plugin, "tuple": release, **evidence})
                (proof / "native-projection.json").write_text(json.dumps({
                    "schema_version": 2, "client_id": client, "entries": entries,
                }))
                (proof / "receipts.json").write_text(json.dumps({
                    "schema_version": 1, "receipts": receipt_rows,
                }))
                for path in (proof / "native-projection.json", proof / "receipts.json"):
                    path.chmod(0o440)
                for path in protected_directories:
                    path.chmod(0o510)

            populate("codex")
            populate("kiro")

            for base in (Path(replacements["/var/lib/uap-observer-human"]),
                         Path(replacements["/var/lib/uap-observer-consent"])):
                mkdir(base, 0o755)
                mkdir(base / "pending", 0o750)
                mkdir(base / "consumed", 0o700)
                mkdir(base / "reserved", 0o700)
            mkdir(Path(replacements["/var/lib/caddy"]), 0o700)
            mkdir(Path(replacements["/var/log/caddy"]), 0o700)

            namespace: dict[str, object] = {}
            exec(compile(source, "installed-state-validator", "exec"), namespace)
            self.assertEqual(namespace["kiro_verifications"], [(
                str(state / "profiles" / "kiro"), os.getegid(), True,
            )])

    def test_profile_provisioner_accepts_only_canonical_shared_client_configs(self) -> None:
        path = ROOT / "deploy/uap-observer-provision-profile.py"
        spec = importlib.util.spec_from_file_location("uap_observer_provision_profile", path)
        self.assertIsNotNone(spec)
        self.assertIsNotNone(spec.loader)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        digest = "sha256:" + "a" * 64

        def release_tuple(plugin: str) -> dict:
            return {
                "product_id": plugin, "tree_digest": digest, "manifest_digest": digest,
                "distribution_id": f"owner/{plugin}", "distribution_kind": "upstream",
                "release_sequence": 1, "package_version": "1.0.0",
                "source_repository": f"owner/{plugin}", "source_revision": "b" * 40,
                "source_path": f"plugins/{plugin}", "snapshot_sequence": 1,
                "snapshot_digest": digest,
                "binary_digest": "sha256:e79125f7ffabd11c6e211d6b049c2eb2b36eb1aba3a76ce27cac819aeba1e6ca",
                "dependency_identity": "locked", "installer_version": "0.1.24",
                "adapter_version": "0.1.24", "client_version": None,
                "os": "linux", "architecture": "x86_64", "observed_at": "2026-08-26T00:00:00Z",
            }

        heroes = ("agent-code-navigator", "context7", "cloudflare-docs", "chrome-devtools", "notion")

        def projection(client: str) -> dict:
            if client == "cursor":
                shared = "/var/lib/uap-observer/profiles/cursor/.cursor/mcp.json"
                skill = "/var/lib/uap-observer/profiles/cursor/.cursor/skills/code-tool-router/SKILL.md"
            elif client == "kiro":
                shared = "/var/lib/uap-observer/profiles/kiro/.kiro/settings/mcp.json"
                skill = "/var/lib/uap-observer/profiles/kiro/.kiro/skills/code-tool-router/SKILL.md"
            else:
                shared = None
                skill = "/var/lib/uap-observer/profiles/codex/skills/code-tool-router/SKILL.md"
            entries = []
            for plugin in heroes:
                kind = "skill" if plugin == "agent-code-navigator" else "mcp"
                config_path = skill if kind == "skill" else (
                    shared or f"/var/lib/uap-observer/profiles/codex/{plugin}.json"
                )
                entries.append({
                    "plugin": plugin, "component_kind": kind, "tuple": release_tuple(plugin),
                    "native_config": {
                        "path": f"/var/lib/uap-observer/proofs/{client}/native/{plugin}.blob",
                        "sha256": digest,
                    },
                    "client_config": {"path": config_path, "sha256": digest},
                    "manager_add_sha256": digest, "manager_info_sha256": digest,
                    "post_add_doctor_sha256": digest,
                })
            return {"schema_version": 2, "client_id": client, "entries": entries}

        for client in ("cursor", "kiro", "codex"):
            value = projection(client)
            with self.subTest(client=client):
                self.assertEqual(module.validate_native_projection(value, client), value["entries"])

        valid_cursor = projection("cursor")
        conflicting_digest = json.loads(json.dumps(valid_cursor))
        conflicting_digest["entries"][1]["native_config"]["sha256"] = "sha256:" + "c" * 64
        conflicting_digest["entries"][1]["client_config"]["sha256"] = "sha256:" + "c" * 64
        unexpected_path = json.loads(json.dumps(valid_cursor))
        unexpected_path["entries"][1]["client_config"]["path"] = "/var/lib/uap-observer/profiles/cursor/other/mcp.json"
        duplicated_codex = projection("codex")
        for entry in duplicated_codex["entries"]:
            if entry["component_kind"] == "mcp":
                entry["client_config"]["path"] = "/var/lib/uap-observer/profiles/codex/mcp.json"
        for value, client in (
            (conflicting_digest, "cursor"), (unexpected_path, "cursor"), (duplicated_codex, "codex"),
        ):
            with self.subTest(rejected_client=client), self.assertRaises(ValueError):
                module.validate_native_projection(value, client)

    def test_runbook_strictly_decodes_and_validates_before_canonicalization(self) -> None:
        runbook = (ROOT / "docs/OBSERVER_OPERATIONS.md").read_text()
        self.assertNotIn('json.loads(source.read_text(encoding="utf-8"))', runbook)
        self.assertEqual(runbook.count("object_pairs_hook=pairs"), 3)
        self.assertEqual(runbook.count("parse_constant=constant, parse_float=finite"), 3)
        self.assertEqual(runbook.count("jsonschema.validate("), 2)
        for phrase, count in (
            ("duplicate or case-confusable JSON member", 3),
            ('type(value.get("schema_version")) is not int', 3),
            ('"native_projection" in record', 2),
        ):
            self.assertEqual(runbook.count(phrase), count)

    def test_observer_readme_installer_example_has_all_ten_arguments(self) -> None:
        readme = (ROOT / "observer/README.md").read_text()
        invocation = re.search(r"`deploy/uap-observer-install\.sh ([^`]+)`", readme)
        self.assertIsNotNone(invocation)
        self.assertEqual(invocation.group(1).split(), [
            "SOURCE_ROOT", "ADAPTER_CONFIG", "sha256:ADAPTER_DIGEST",
            "OBSERVER_CONFIG", "sha256:OBSERVER_DIGEST",
            "CADDY_2.11.4_LINUX_AMD64_ARCHIVE", "CADDY_CONFIG",
            "sha256:CADDY_CONFIG_DIGEST", "EGRESS_ALLOWLIST",
            "sha256:EGRESS_ALLOWLIST_DIGEST",
        ])

    def test_documented_pre_add_doctor_gate_executes_strictly(self) -> None:
        runbook = (ROOT / "docs/OBSERVER_OPERATIONS.md").read_text()
        marker = 'python3 - "$client" "$evidence/doctor/detection.json" <<\'PY\'\n'
        start = runbook.index(marker) + len(marker)
        snippet = runbook[start:runbook.index("\nPY", start)]
        valid = {
            "schema_version": 1, "command": "doctor", "result": "success",
            "data": {
                "tool_version": "0.1.24",
                "clients": [
                    {"client_id": name, "detected": name == "codex"}
                    for name in (
                        "chatgpt", "claude", "cline", "codex", "copilot", "cursor",
                        "gemini", "kiro", "opencode", "vscode", "windsurf",
                    )
                ],
                "supported_clients": [
                    {"client_id": name, "package_mode": "native"}
                    for name in (
                        "chatgpt", "claude", "cline", "codex", "copilot", "cursor",
                        "gemini", "kiro", "opencode", "vscode", "windsurf",
                    )
                ],
            },
        }
        with tempfile.TemporaryDirectory() as temporary:
            evidence = Path(temporary) / "doctor.json"
            evidence.write_text(json.dumps(valid))
            accepted = subprocess.run(
                ["/usr/bin/python3", "-c", snippet, "codex", str(evidence)],
                text=True, capture_output=True,
            )
            self.assertEqual(accepted.returncode, 0, accepted.stderr)
            stale_binary = json.loads(json.dumps(valid))
            stale_binary["data"]["tool_version"] = "0.1.18"
            incomplete_inventory = json.loads(json.dumps(valid))
            incomplete_inventory["data"]["clients"].pop()
            incomplete_inventory["data"]["supported_clients"].pop()
            duplicate_inventory = json.loads(json.dumps(valid))
            duplicate_inventory["data"]["clients"].append(
                json.loads(json.dumps(duplicate_inventory["data"]["clients"][0])),
            )
            malformed_inventory = json.loads(json.dumps(valid))
            malformed_inventory["data"]["supported_clients"][-1] = "windsurf"
            adversarial = (
                b'{"schema_version":true,"command":"doctor","result":"success","data":{}}',
                b'{"schema_version":1,"Schema_Version":1,"command":"doctor","result":"success","data":{}}',
                b'{"schema_version":1,"command":"doctor","result":"success","data":{},"value":NaN}',
                b'{"schema_version":1,"command":"doctor","result":"success","data":{},"value":1e400}',
                json.dumps(stale_binary).encode(),
                json.dumps(incomplete_inventory).encode(),
                json.dumps(duplicate_inventory).encode(),
                json.dumps(malformed_inventory).encode(),
            )
            for body in adversarial:
                evidence.write_bytes(body)
                rejected = subprocess.run(
                    ["/usr/bin/python3", "-c", snippet, "codex", str(evidence)],
                    text=True, capture_output=True,
                )
                with self.subTest(body=body):
                    self.assertNotEqual(rejected.returncode, 0)

    def test_kiro_acp_2200_grammar_is_distinguished_from_pending_live_matrix(self) -> None:
        adapter = (ROOT / "observer/fixed_adapters.py").read_text()
        plan = (ROOT / "docs/E2E_AND_COMPETITIVE_LAUNCH_PLAN.md").read_text()
        runbook = (ROOT / "docs/OBSERVER_OPERATIONS.md").read_text()
        for text in (adapter, plan, runbook):
            self.assertIn("2.20.0", text)
        self.assertIn('("acp", "--agent-engine", "v3", "--auth-method", "cli")', adapter)
        self.assertIn("capability probes do not replace the five-result launch matrix", plan)
        self.assertIn("are not the final five-plugin-by-three-client launch matrix", runbook)
        self.assertNotIn("KIRO_STRUCTURED_CAPTURE_GATE", adapter)
        fixture = json.loads((ROOT / "observer/tests/fixtures/kiro-acp-2.20.0-sanitized.json").read_text())
        summary = fixture["observed_shape_summary"]
        self.assertEqual(summary["final_native_5x3_matrix"], "pending_external")
        self.assertEqual(summary["evidence_kind"], "sanitized_observed_shape_summary_not_raw_ordered_acp_frames")
        self.assertEqual(summary["ordering_claim"], "none")
        self.assertNotIn("ordered structured successful tool-call", (ROOT / "observer/README.md").read_text())
        self.assertNotIn("post-boundary unrelated", (ROOT / "observer/README.md").read_text())
        self.assertNotIn("pending-to-completed", (ROOT / "observer/README.md").read_text())
        self.assertIn("Their sequence is not asserted", plan)
        self.assertEqual(fixture["public_contract"]["session_new_mcp_servers"], [])
        self.assertIn("multi-tool catalog", runbook)
        self.assertIn("unrelated `kiro_power` failure shapes", (ROOT / "observer/README.md").read_text())

    def test_kiro_runtime_closure_is_bound_across_provision_invoke_and_install(self) -> None:
        provisioner = (ROOT / "deploy/uap-observer-provision-profile.py").read_text()
        adapter = (ROOT / "observer/fixed_adapters.py").read_text()
        installer = (ROOT / "deploy/uap-observer-install-lib.sh").read_text()
        spec = importlib.util.spec_from_file_location("workflow_contract_provisioner", ROOT / "deploy/uap-observer-provision-profile.py")
        self.assertIsNotNone(spec)
        self.assertIsNotNone(spec.loader)
        provisioner_module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(provisioner_module)
        from observer import fixed_adapters as adapter_module
        for digest in (
            "81925c0995b5c1427b5d538e6a90ca2fdc4daffb786b09af749beaf7369d4e90",
            "b29d78892abd5a9398e0700f0cb602f725089602ed1a5082d681c7257b2bf4d0",
            "2e4db6323c38b6fba18367e2fbf2a7e2951bed87638fd030e207e45a152d5fc2",
            "e15cb01e83da5989999ca6529dc9b2f0992de588e6d87107651bd4a45dea8901",
            "193906679498de4d939345b937fa24e0e69a03c244bd70c859f5e41232713f21",
        ):
            for body in (provisioner, adapter):
                self.assertIn(digest, body)
        for body in (provisioner, adapter):
            self.assertIn("2.20.0-426339cf358a40306b73d09e69160be6201aba88d449893179d2a15a10020bdd", body)
        normalized_digests = {
            path[len(provisioner_module.KIRO_RUNTIME_ROOT):]: digest
            for path, digest in provisioner_module.KIRO_RUNTIME_DIGESTS.items()
        }
        normalized_executables = {
            path[len(provisioner_module.KIRO_RUNTIME_ROOT):]
            for path in provisioner_module.KIRO_RUNTIME_EXECUTABLES
        }
        self.assertEqual(provisioner_module.KIRO_KAS_DIRECTORY, adapter_module.KIRO_KAS_DIRECTORY)
        self.assertEqual(provisioner_module.KIRO_FEED_SOURCE, adapter_module.KIRO_FEED_SOURCE)
        self.assertEqual(provisioner_module.KIRO_FEED_CACHE_SHA256, adapter_module.KIRO_FEED_CACHE_SHA256)
        self.assertEqual(provisioner_module.KIRO_KAS_TREE_SHA256, adapter_module.KIRO_KAS_TREE_SHA256)
        self.assertEqual(normalized_digests, adapter_module.KIRO_RUNTIME_DIGESTS)
        self.assertEqual(normalized_executables, adapter_module.KIRO_RUNTIME_EXECUTABLES)
        with tempfile.TemporaryDirectory() as temporary:
            tree = Path(temporary)
            (tree / "a").mkdir(); (tree / "a" / "child").write_bytes(b"one")
            (tree / "a.b").write_bytes(b"two")
            expected = "546f0a17356cc4fcb39223e6bc1eb52239931c013bb08ca257de4e885767708e"
            tree_fd = os.open(tree, os.O_RDONLY | os.O_DIRECTORY)
            try:
                self.assertEqual(provisioner_module.canonical_kiro_tree_digest(tree_fd), expected)
                self.assertEqual(adapter_module.canonical_kiro_tree_digest(tree_fd), expected)
            finally:
                os.close(tree_fd)
        self.assertIn("protected_executables=protected_executables", provisioner)
        self.assertEqual(adapter.count("verified_kiro_call("), 3)
        self.assertIn("from observer.fixed_adapters import verify_kiro_runtime", installer)
        self.assertIn('if suffix=="kiro": verify_kiro_runtime(profile,expected_gid=gid,verify_tree_digest=True)', installer)

    def test_installer_runtime_pins_equal_current_manifest_and_files(self) -> None:
        installer = (ROOT / "deploy/uap-observer-install.sh").read_text()
        manifest_path = ROOT / "deploy/uap-observer-runtime.sha256"
        entries = [line.split(maxsplit=1) for line in manifest_path.read_text().splitlines() if line.strip()]
        self.assertTrue(entries)
        relatives = [relative for _, relative in entries]
        self.assertEqual(len(entries), 47)
        self.assertEqual(len(entries), len(set(relatives)))
        self.assertEqual(relatives, sorted(relatives))
        self.assertIn("tests/e2e/schemas/launch-evidence-v5.schema.json", relatives)
        self.assertIn("tests/e2e/schemas/native-release-observation-v2.schema.json", relatives)
        manifest = {relative: digest for digest, relative in entries}
        def file_digest(relative: str) -> str:
            return hashlib.sha256((ROOT / relative).read_bytes()).hexdigest()
        self.assertEqual(manifest, {relative: file_digest(relative) for relative in manifest})
        self.assertEqual(manifest["observer/fixed_runner.py"], file_digest("observer/fixed_runner.py"))
        self.assertEqual(manifest["observer/fixed_adapters.py"], file_digest("observer/fixed_adapters.py"))
        recovery_helper = "deploy/uap-observer-recover-profile-seed.py"
        self.assertEqual(manifest[recovery_helper], file_digest(recovery_helper))
        for phrase in (
            'install -o root -g root -m 0555 "$source_root/deploy/uap-observer-recover-profile-seed.py"',
            'mv /usr/local/libexec/uap-observer-recover-profile-seed.new "$closure_stage/libexec/uap-observer-recover-profile-seed"',
        ):
            self.assertIn(phrase, installer)
        install_library = (ROOT / "deploy/uap-observer-install-lib.sh").read_text()
        self.assertIn(
            'observer_compare_regular_files_neutral "$source_root/deploy/uap-observer-recover-profile-seed.py" "$closure/libexec/uap-observer-recover-profile-seed"',
            install_library,
        )
        assignments = dict(re.findall(
            r"^(runner_digest|adapter_digest)=([a-f0-9]{64})$", installer, re.MULTILINE,
        ))
        self.assertEqual(assignments, {
            "runner_digest": manifest["observer/fixed_runner.py"],
            "adapter_digest": manifest["observer/fixed_adapters.py"],
        })
        self.assertIn('    "$runner_digest" \\\n    "$adapter_digest" \\\n', installer)
        library_pin = re.search(r'sha256sum "\$install_lib".*?= ([a-f0-9]{64})$', installer, re.MULTILINE)
        self.assertIsNotNone(library_pin)
        self.assertEqual(library_pin.group(1), file_digest("deploy/uap-observer-install-lib.sh"))
        closure_pin = re.search(r"^runtime_manifest_digest=([a-f0-9]{64})$", installer, re.MULTILINE)
        self.assertIsNotNone(closure_pin)
        self.assertEqual(closure_pin.group(1), hashlib.sha256(manifest_path.read_bytes()).hexdigest())

    def test_checked_in_observer_config_binds_exact_runner_closure(self) -> None:
        config = json.loads((ROOT / "deploy/uap-observer.json").read_text())
        runner_digest = hashlib.sha256((ROOT / "observer/fixed_runner.py").read_bytes()).hexdigest()
        self.assertEqual(config["runner_source_digest"], f"sha256:{runner_digest}")
        self.assertEqual(
            config["runner_source_path"],
            "/opt/uap-observer-current/libexec/uap-observer-runner",
        )
        self.assertEqual(config["runner_socket"], "/run/uap-observer-runner.sock")
        self.assertEqual(config["runner_user"], "root")

    def test_installed_adapter_manifest_uses_runtime_strict_json_and_version_rules(self) -> None:
        library = (ROOT / "deploy/uap-observer-install-lib.sh").read_text()
        start = library.index("def strict_adapter_manifest(encoded,expected):")
        end = library.index("\nconfig_digest=", start)
        namespace = {}
        exec("import json,math\n" + library[start:end], namespace)
        validate = namespace["strict_adapter_manifest"]
        expected = {"schema_version": 1, "config": {}, "artifacts": {}}
        valid = b'{"schema_version":1,"config":{},"artifacts":{}}'
        self.assertEqual(validate(valid, expected), expected)
        rejected = (
            b'{"schema_version":1,"schema_version":1,"config":{},"artifacts":{}}',
            b'{"schema_version":true,"config":{},"artifacts":{}}',
            b'{"schema_version":1.0,"config":{},"artifacts":{}}',
            b'{"schema_version":NaN,"config":{},"artifacts":{}}',
        )
        for encoded in rejected:
            with self.subTest(encoded=encoded), self.assertRaises((SystemExit, ValueError)):
                validate(encoded, expected)

    def test_stable_launch_versions_equal_trusted_contract(self) -> None:
        version = (ROOT / "tests/e2e/stable-launch-version.txt").read_text().strip()
        self.assertEqual(version, "0.1.24")
        tag = f"agentplugins-v{version}"
        asset_prefix = f"agentplugins_{version}_"
        production = json.loads((ROOT / "tests/e2e/production-launch.json").read_text())
        schema = json.loads((ROOT / "tests/e2e/schemas/native-release-observation-v2.schema.json").read_text())
        adapter_schema = json.loads((ROOT / "deploy/uap-observer-adapter-config.schema.json").read_text())
        observer = json.loads((ROOT / "deploy/uap-observer.json").read_text())
        workflow = load(LAUNCH)
        assets = {slot["asset"] for slot in workflow["jobs"]["native-release"]["strategy"]["matrix"]["include"]}
        npm = schema["properties"]["npm_package"]["properties"]
        self.assertEqual(workflow["env"]["AGENTPLUGINS_VERSION"], version)
        self.assertEqual(production["cli_release_tag"], tag)
        self.assertEqual(production["cli_release_id"], 379284682)
        self.assertEqual(production["cli_release_assets"], {
            "agentplugins_0.1.24_darwin_amd64": {"sha256": "93f7cc8fd9300e23719e63d08af1eb2cc1ed9743bac98de1e59c352623328bf2", "size": 12299152},
            "agentplugins_0.1.24_darwin_arm64": {"sha256": "c9d3dfe4b4b06d70733841d72fd9c8b9070ce066f6fa5dd7260073cf69565972", "size": 11474578},
            "agentplugins_0.1.24_linux_amd64": {"sha256": "e79125f7ffabd11c6e211d6b049c2eb2b36eb1aba3a76ce27cac819aeba1e6ca", "size": 12185784},
            "agentplugins_0.1.24_linux_arm64": {"sha256": "6768db4cdc3faf41ec31194284ac8c92bc58953737a6164e0fe88cc13aae57a1", "size": 11337912},
            "agentplugins_0.1.24_windows_amd64.exe": {"sha256": "0fc327e31009d5c9dc01b2b8cc091f98dd90012376fedb2b688b3e2293a0507d", "size": 12459520},
            "agentplugins_0.1.24_windows_arm64.exe": {"sha256": "e178f6fc3318fd26056c8bcc073cc4426102063f80974c2b23801d50c749109c", "size": 11418112},
        })
        self.assertEqual(observer["cli_release_tag"], tag)
        self.assertEqual(adapter_schema["properties"]["request_policy"]["properties"]["cli_release_tag"]["const"], tag)
        # Bind the version and both independently authenticated public release
        # assets in one regression contract.  Updating only the version cannot
        # leave a stale observer trust pin behind.
        request_policy = adapter_schema["properties"]["request_policy"]["properties"]
        release_tuple = adapter_schema["$defs"]["tuple"]["properties"]
        self.assertEqual((version, request_policy["release_manifest_digest"]["const"], request_policy["release_checksums_digest"]["const"]), (
            "0.1.24",
            "sha256:eb834da8237b13ed36061aeafb4fbb6f4aadeb5a6fbd4a31d43781f456f3d1e2",
            "sha256:623fb73d0e2f59da8b01399842b0d82b8f6456c6e43db2251c0ea5f9e32f37e3",
        ))
        self.assertEqual(schema["properties"]["cli_release_tag"]["const"], tag)
        self.assertEqual(schema["properties"]["github_release_identity"]["properties"]["tag"]["const"], tag)
        self.assertEqual(schema["properties"]["github_release_identity"]["properties"]["release_id"]["const"], 379284682)
        self.assertEqual(schema["properties"]["github_asset_attestation"]["properties"]["tag"]["const"], tag)
        self.assertEqual(release_tuple["binary_digest"]["const"], "sha256:e79125f7ffabd11c6e211d6b049c2eb2b36eb1aba3a76ce27cac819aeba1e6ca")
        self.assertEqual(release_tuple["installer_version"]["const"], version)
        self.assertEqual(release_tuple["adapter_version"]["const"], version)
        self.assertTrue(assets)
        self.assertTrue(all(asset.startswith(asset_prefix) for asset in assets))
        self.assertEqual(npm["version"]["const"], version)
        self.assertEqual(npm["integrity"]["const"], production["npm_facade_integrity"])
        self.assertEqual(npm["tarball"]["const"], f"https://registry.npmjs.org/universal-agent-plugins/-/universal-agent-plugins-{version}.tgz")
        self.assertEqual(npm["provenance_url"]["const"], f"https://registry.npmjs.org/-/npm/v1/attestations/universal-agent-plugins@{version}")
        self.assertEqual(npm["native_asset_name"]["const"], f"{asset_prefix}linux_amd64")
        stable_files = (
            LAUNCH, ROOT / "scripts/run_launch_evidence_e2e.py", ROOT / "tests/e2e/production-launch.json",
            ROOT / "tests/e2e/schemas/native-release-observation-v2.schema.json", ROOT / "deploy/uap-observer.json",
            ROOT / "deploy/uap-observer-adapter-config.schema.json", ROOT / "observer/config.py",
        )
        for path in stable_files:
            with self.subTest(path=path.name):
                self.assertNotRegex(path.read_text(), r"(?:agentplugins[-_v@]|universal-agent-plugins[-@])0\.1\.(?:12|13)")

    def test_directory_publication_prepare_installs_bridge_runtime_dependencies(self) -> None:
        workflow = load(DIRECTORY_PUBLICATION)
        prepare_commands = commands(workflow["jobs"]["prepare"])
        self.assertIn("PyYAML==6.0.3", prepare_commands)

    def test_discovery_index_is_lkg_protected_and_preserves_promoted_directory(self) -> None:
        workflow = load(DISCOVERY_INDEX)
        self.assertEqual(workflow["concurrency"], {
            "group": "directory-publication-schema-1",
            "cancel-in-progress": "false",
        })
        self.assertEqual(
            {entry["cron"] for entry in workflow["on"]["schedule"]},
            {"17 */6 * * *", "43 2 * * *", "11 3 * * 0"},
        )
        self.assertNotIn("pull_request", workflow["on"])
        scan = workflow["jobs"]["scan"]
        signer = workflow["jobs"]["sign-and-publish"]
        publisher_preflight = workflow["jobs"]["publisher-preflight"]
        self.assertEqual(publisher_preflight["environment"], "discovery-publication")
        self.assertEqual(publisher_preflight["permissions"], {"contents": "read"})
        self.assertEqual(scan["needs"], "publisher-preflight")
        preflight_body = yaml.safe_dump(publisher_preflight)
        self.assertIn("DISCOVERY_PUBLISHER_APP_PRIVATE_KEY", preflight_body)
        self.assertIn("permission-contents: write", preflight_body)
        self.assertEqual(scan["environment"], "discovery-read")
        self.assertEqual(signer["environment"], "discovery-publication")
        scan_body = yaml.safe_dump(scan)
        self.assertIn("build_discovery_index.py", scan_body)
        self.assertIn("previous-snapshot", scan_body)
        self.assertIn("--repository-workers", scan_body)
        scan_step = next(step for step in scan["steps"] if step.get("id") == "scan")
        self.assertEqual(scan_step["env"]["DISCOVERY_REPOSITORY_WORKERS"], "8")
        self.assertIn("GITHUB_TOKEN: ${{ github.token }}", scan_body)
        self.assertNotIn("secrets.", scan_body)
        self.assertNotIn("DISCOVERY_ED25519_PRIVATE_KEY", scan_body)
        self.assertNotIn("DIRECTORY_ED25519_PRIVATE_KEY", scan_body)
        signer_body = yaml.safe_dump(signer)
        self.assertIn("DISCOVERY_ED25519_PRIVATE_KEY", signer_body)
        self.assertIn("DISCOVERY_PUBLISHER_APP_PRIVATE_KEY", signer_body)
        self.assertNotIn("DIRECTORY_ED25519_PRIVATE_KEY", signer_body)
        self.assertNotIn("DIRECTORY_PUBLISHER_APP_PRIVATE_KEY", signer_body)
        self.assertNotIn("${{ github.token }}", signer_body)
        self.assertIn("registry/discovery/trusted-keys.json", signer_body)
        self.assertIn("UAP-DISCOVERY", (ROOT / "scripts/discovery_publication.py").read_text())
        self.assertIn("git push --atomic", commands(signer))
        self.assertIn("permission-contents: write", signer_body)
        self.assertIn("for attempt in 1 2 3", commands(signer))
        self.assertIn("HTTP (429|5[0-9]{2})", commands(signer))
        self.assertIn("git ls-remote --refs origin", commands(signer))
        self.assertIn('test "${branch_commit}" = "${local_commit}"', commands(signer))
        self.assertIn('test "${tag_commit}" = "${local_commit}"', commands(signer))
        self.assertIn('test "${branch_commit}" = "${base_commit}"', commands(signer))
        self.assertIn('test -z "${tag_commit}"', commands(signer))
        self.assertIn("refusing an uncertain retry", commands(signer))
        self.assertIn("discovery-index-sequence-", commands(signer))
        self.assertIn('merge-base --is-ancestor "refs/tags/${tag}" HEAD', commands(signer))
        self.assertIn('ls-tree -r --name-only "refs/tags/${latest_sequence_tag}"', commands(signer))
        self.assertIn('test "${entry%% *}" = 100644', commands(signer))
        self.assertIn('test "${remainder%% *}" = blob', commands(signer))
        self.assertIn('test "${ledger_entries[*]}" = "${tagged_entries[*]}"', commands(signer))
        self.assertIn('checkout "refs/tags/${recovery_tag}" -- "${FEED_PATH}"', commands(signer))
        self.assertIn('test "${restored_paths[*]}" = "${expected_feed[*]}"', commands(signer))
        self.assertIn('test "${change%%$\'\\t\'*}" = A', commands(signer))
        self.assertIn('RECOVERY_TAG: ${{ steps.preflight.outputs.recovery_tag }}', signer_body)
        self.assertIn('sort -u', commands(signer))
        artifact_names = [
            step["with"]["name"]
            for job in (scan, signer)
            for step in job["steps"]
            if isinstance(step, dict) and "with" in step and "name" in step["with"]
            and step.get("uses", "").startswith(("actions/upload-artifact", "actions/download-artifact"))
        ]
        self.assertEqual(artifact_names, [
            "discovery-candidate-${{ github.run_id }}",
            "discovery-candidate-${{ github.run_id }}",
        ])
        incomplete = workflow["jobs"]["incomplete-scan"]
        self.assertIn("needs.scan.outputs.complete != 'true'", incomplete["if"])
        self.assertIn("exit 1", commands(incomplete))
        deploy = workflow["jobs"]["deploy"]
        checkouts = [step for step in deploy["steps"] if step.get("uses", "").startswith("actions/checkout")]
        self.assertEqual(checkouts[0]["with"]["ref"], "${{ needs.sign-and-publish.outputs.ledger_commit }}")
        self.assertEqual(checkouts[0]["with"]["fetch-depth"], "0")
        self.assertEqual(checkouts[0]["with"]["fetch-tags"], "true")
        self.assertEqual(checkouts[1]["with"]["ref"], "${{ github.sha }}")
        deploy_body = commands(deploy)
        self.assertIn("production-marker.json", deploy_body)
        self.assertIn("bootstrap_materialized_commit", deploy_body)
        self.assertIn("git -C exact-discovery-tree ls-remote --refs origin", deploy_body)
        self.assertIn("merge-base --is-ancestor", deploy_body)
        self.assertIn("sequence_boundaries.py validate", deploy_body)
        self.assertIn("directory_publication_cas.py staged-lineage-verify", deploy_body)
        self.assertIn('--signed "${production_signed}" --current "${production_commit}"', deploy_body)
        self.assertIn("rsync -a --delete exact-discovery-tree/discovery/ production-pages-tree/discovery/", deploy_body)
        self.assertIn("diff -qr exact-discovery-tree/discovery production-pages-tree/discovery", deploy_body)
        self.assertIn("diff --name-only -- . ':!discovery'", deploy_body)
        self.assertIn("tar --directory production-pages-tree", deploy_body)
        self.assertNotIn("tar --directory exact-discovery-tree", deploy_body)
        marker = json.loads((ROOT / "registry/publication/production-marker.json").read_text())
        self.assertRegex(marker["bootstrap_materialized_commit"], r"^[0-9a-f]{40}$")
        self.assertEqual(marker["bootstrap_sequence"], 13)
        self.assertIn("observe_discovery_index.py", commands(workflow["jobs"]["observe"]))
        for job_name in ("scan", "sign-and-publish", "observe"):
            with self.subTest(job=job_name):
                body = commands(workflow["jobs"][job_name])
                self.assertIn("cryptography==46.0.3", pinned_requirements(body))
                self.assertIn("jsonschema==4.26.0", pinned_requirements(body))

    def test_directory_materialization_preserves_the_discovery_feed(self) -> None:
        workflow = load(DIRECTORY_PUBLICATION)
        materialize = workflow["jobs"]["materialize_site"]
        body = commands(materialize)
        self.assertIn("rsync -a --delete --exclude=.git --exclude=registry --exclude=/discovery", body)
        self.assertIn("diff --exit-code -- discovery", body)
        self.assertIn("diff --cached --exit-code -- discovery", body)
        self.assertIn('[[ "${path}" == discovery/* ]]', body)
        self.assertIn('"${EXISTING_MATERIALIZED_COMMIT}..${EXPECTED_LEDGER_HEAD}"', body)
        self.assertLess(
            body.index('if test -n "${EXISTING_MATERIALIZED_COMMIT}"'),
            body.index("rsync -a --delete"),
        )
        self.assertIn("commit --allow-empty", body)
        self.assertEqual(body.count("':!discovery'"), 1)

    def test_directory_materialization_delete_semantics_keep_signed_feeds(self) -> None:
        rsync = shutil.which("rsync")
        if rsync is None:
            self.skipTest("rsync is not installed")
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            generated = root / "generated"
            ledger = root / "ledger"
            generated.mkdir()
            (ledger / "registry").mkdir(parents=True)
            (ledger / "discovery").mkdir()
            (generated / "index.html").write_text("new generated site\n")
            (ledger / "index.html").write_text("old site\n")
            (ledger / "stale.html").write_text("remove me\n")
            (ledger / "registry" / "latest.json").write_text("directory\n")
            (ledger / "discovery" / "latest.json").write_text("discovery\n")
            subprocess.run([
                rsync, "-a", "--delete", "--exclude=.git", "--exclude=registry",
                "--exclude=/discovery", str(generated) + "/", str(ledger) + "/",
            ], check=True)
            self.assertEqual((ledger / "index.html").read_text(), "new generated site\n")
            self.assertFalse((ledger / "stale.html").exists())
            self.assertEqual((ledger / "registry" / "latest.json").read_text(), "directory\n")
            self.assertEqual((ledger / "discovery" / "latest.json").read_text(), "discovery\n")

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

    def test_observer_policy_accepts_exactly_the_protected_workflow_events(self) -> None:
        workflow = load(LAUNCH)
        protected_if = workflow["jobs"]["protected-observer-inputs"]["if"]
        workflow_events = set(re.findall(
            r"inputs\.caller_event_name\s*==\s*'([^']+)'", protected_if,
        ))
        observer = json.loads((ROOT / "deploy/uap-observer.json").read_text())
        policy_events = set(observer["policies"][0]["event_names"])
        self.assertEqual(workflow_events, {"push", "schedule", "workflow_dispatch"})
        self.assertEqual(policy_events, workflow_events)

    def test_observer_runbook_binds_fail_closed_loopback_proxy_operations(self) -> None:
        runbook = OBSERVER_RUNBOOK.read_text()
        required = (
            "EGRESS_ALLOWLIST=/root/uap-observer-egress-allowlist.json",
            "EGRESS_SHA256=sha256:<64-lowercase-hex>",
            "EGRESS_ALLOWLIST_MODE=0:0:644:1",
            '"$EGRESS_ALLOWLIST" "$EGRESS_SHA256"',
            "--config \"$EGRESS_ALLOWLIST\" --validate-config",
            "The proxy executable and units are repository source/runtime-closure files",
            "`schema_version` set to `1` and a non-empty `hosts` array",
            "bytewise-sorted,\nunique, exact lowercase ASCII FQDNs",
            "Its `hosts` array must equal adapter `egress_hosts` byte for\nbyte",
            "Do not stage\nor copy units into `/etc/systemd/system` manually",
            "NO_OPEN_BROWSER=1 login",
            "systemctl enable --now uap-observer-egress-proxy.socket",
            "systemctl is-active uap-observer-egress-proxy.service",
            "systemctl stop uap-observer-egress-proxy.socket",
            "127.0.0.1:8765",
            "127.0.0.2:8766",
            "no transparent or direct-fallback mode",
            "HTTP_PROXY=http://127.0.0.2:8766",
            "HTTPS_PROXY=http://127.0.0.2:8766",
            "ALL_PROXY=http://127.0.0.2:8766",
            "explicitly empty `NO_PROXY`",
            "current\nprovider-owned host list for Codex, Cursor, Kiro, GitHub",
            "installer itself still supports a clean state only",
            "repository does not install UID-, cgroup-, or service-identity firewall rules",
            "`IPAddressDeny=any`",
            "resolve_github_release",
            '"agentplugins-v0.1.24"',
            'asset_name="agentplugins_0.1.24_linux_amd64"',
            '"$AGENTPLUGINS" add "$source" --target "$client" --format json',
            '"$AGENTPLUGINS" info "$plugin" --target "$client" --format json',
            "manual_activation_required",
            "source_repository@source_revision//source_path",
            'PATH="$client_path"',
            '"$AGENTPLUGINS" doctor --format json',
            '"$client_path/kiro-cli"',
            "An unauthenticated Kiro probe mutated its seed and then\nfailed four MCP activations",
            "package_revision` has exactly `version`,\n`resolved_revision`, `tree_digest`, and `manifest_digest",
            "uap-observer-seal-profile.py",
            "external live matrix supplies all 15 required results",
        )
        for phrase in required:
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, runbook)
        self.assertIn("Do not bypass OIDC", runbook)

    def test_runbook_uses_one_exact_current_release_for_all_manager_operations(self) -> None:
        runbook = OBSERVER_RUNBOOK.read_text()
        current = runbook.split(
            'root = Path("/root/approved-inputs/agentplugins-0.1.24")', 1,
        )[1].split("```", 1)[0]
        self.assertIn('"agentplugins-v0.1.24"', current)
        self.assertNotIn("attestation_verifier=", current)
        self.assertNotIn("agentplugins-0.1.18", runbook)
        self.assertNotIn("agentplugins-v0.1.18", runbook)
        self.assertNotIn("verify_historical_github_asset_attestation", runbook)
        self.assertEqual(
            runbook.count("AGENTPLUGINS=/root/approved-inputs/agentplugins-0.1.24/agentplugins"), 1,
        )
        self.assertIn("Every manager\noperation in this procedure", runbook)
        self.assertIn('data.get("tool_version") != "0.1.24"', runbook)
        self.assertIn("doctor did not report the exact 11-client inventory", runbook)
        self.assertIn("five minutes before", runbook)
        self.assertIn("genuinely\nexternal unmerged PR", runbook)
        self.assertNotIn("uap-observer-egress-fqdns.txt", runbook)
        self.assertNotIn("NO_OPEN_BROWSER=1 agent login", runbook)
        self.assertNotIn("/root/uap-observer-egress-proxy.socket", runbook)
        self.assertNotIn("/root/uap-observer-egress-proxy.service", runbook)
        self.assertIn("STABLE_RESET_HELPER=/usr/local/libexec/uap-observer-reset", runbook)
        self.assertIn("/opt/uap-observer-current/libexec/uap-observer-provision-profile", runbook)
        for recovery_contract in (
            "test ! -e /root/uap-observer-recovery",
            "tar --restrict --no-same-owner --no-same-permissions --keep-old-files",
            'python3 -B /opt/uap-observer-current/libexec/uap-observer-recover-profile-seed',
            '--adapter-config /root/uap-observer-adapter-config.json',
            '/extracted/var/lib/uap-observer/profiles/$client',
            '/extracted/var/lib/uap-observer/proofs/$client',
            '/extracted/var -mindepth 1 -maxdepth 1',
            '/extracted/var/lib -mindepth 1 -maxdepth 1',
        ):
            self.assertIn(recovery_contract, runbook)
        self.assertNotIn('-xpf "$RECOVERY_ARCHIVE"', runbook)
        self.assertIn("--matrix-file /root/uap-observer-matrix.json", runbook)
        self.assertIn("--post-doctor-directory", runbook)
        self.assertIn('if [ "$client" = codex ]; then\n    install -d -o root -g root -m 0700 "$seed/.codex"', runbook)
        self.assertNotIn('"$seed/.agentplugins" "$seed/.codex"', runbook)
        digest_phase = runbook.index("--digest-only")
        config_phase = runbook.index("uap-observer-adapter-config.template.json", digest_phase)
        validate_phase = runbook.index("python3 -m jsonschema", config_phase)
        seal_phase = runbook.index('sealed_digest="$(python3', validate_phase)
        self.assertLess(digest_phase, config_phase)
        self.assertLess(config_phase, validate_phase)
        self.assertLess(validate_phase, seal_phase)
        final_config = "/root/uap-observer-adapter-config.json"
        first_final_use = runbook.index(final_config)
        self.assertGreater(first_final_use, digest_phase)
        bootstrap_prefix = runbook[:digest_phase]
        self.assertNotIn(final_config, bootstrap_prefix)
        self.assertIn('/root/uap-observer-matrix.json <<\'PY\'', bootstrap_prefix)
        self.assertNotIn("Do not reject a successful add merely", runbook)
        proxy_probe = runbook.split(
            '"https://<reviewed-allowlisted-health-fqdn>/"', 1,
        )[0].rsplit("curl ", 1)[-1]
        self.assertNotIn("--fail", proxy_probe)

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
            "agentplugins_0.1.24_darwin_arm64", "agentplugins_0.1.24_darwin_amd64",
            "agentplugins_0.1.24_linux_arm64", "agentplugins_0.1.24_linux_amd64",
            "agentplugins_0.1.24_windows_amd64.exe", "agentplugins_0.1.24_windows_arm64.exe",
        })
        aggregate_commands = commands(aggregate)
        self.assertIn("config['cli_release_assets']", aggregate_commands)
        self.assertIn("config['cli_release_id']", aggregate_commands)
        self.assertIn("config['npm_facade_integrity']", aggregate_commands)
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
        self.assertIn("universal-agent-plugins-0.1.24.tgz", commands(npm))
        self.assertIn("--asset-name agentplugins_0.1.24_linux_amd64", commands(npm))
        self.assertEqual(commands(native).count("--schema-version 2"), 1)
        self.assertEqual(commands(npm).count("--schema-version 2"), 1)
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
        self.assertIn("native-release-observation-v2.schema.json", commands(enforced))
        self.assertIn("launch-evidence-v5.schema.json", commands(enforced))
        self.assertIn("request_observer_bundle", commands(observer))
        self.assertIn('"scenario_contract_digest": scenario_digest', commands(observer))
        observer_commands = " ".join(commands(observer).split())
        self.assertIn('"release_manifest_digest": context.get("release_manifest_digest")', observer_commands)
        self.assertIn('"directory_digest": context.get("directory", {}).get("digest")', observer_commands)
        self.assertNotIn("make_challenge", observer_commands)
        self.assertIn('"run_attempt": os.environ["GITHUB_RUN_ATTEMPT"]', commands(observer))
        self.assertIn('context["producer_run_attempt"] = producer_attempt', commands(observer))
        self.assertIn("enforce_freshness=True", commands(observer))
        self.assertNotIn("id-token", enforced["permissions"])
        self.assertIn("--observer-bundle", commands(enforced))
        self.assertIn("observer-bundle.schema.json", commands(observer))
        observer_request = next(step for step in observer["steps"] if step.get("id") == "observer")
        challenge_guard = next(
            step for step in observer["steps"]
            if step.get("name") == "Require the exact prepared challenge for this workflow attempt"
        )
        oidc_step_index = next(
            index for index, step in enumerate(observer["steps"])
            if step.get("name") == "Obtain short-lived GitHub OIDC identity"
        )
        self.assertLess(observer["steps"].index(challenge_guard), oidc_step_index)
        self.assertIn("challenge_context_valid", challenge_guard["run"])
        self.assertIn("rerun all jobs", challenge_guard["run"])
        self.assertNotIn("make_challenge", observer_request["run"])
        self.assertEqual(
            observer["outputs"]["observer_public_key"],
            "${{ steps.observer.outputs.public_key }}",
        )
        self.assertEqual(
            observer["outputs"]["observer_key_id"],
            "${{ steps.observer.outputs.key_id }}",
        )
        self.assertIn("printf 'public_key=%s\\n'", observer_request["run"])
        self.assertIn("printf 'key_id=%s\\n'", observer_request["run"])
        enforced_bundle = next(step for step in enforced["steps"] if step.get("id") == "bundle")
        self.assertEqual(
            enforced_bundle["env"]["OBSERVER_ED25519_PUBLIC_KEY"],
            "${{ needs.protected-observer-inputs.outputs.observer_public_key }}",
        )
        self.assertEqual(
            enforced_bundle["env"]["OBSERVER_KEY_ID"],
            "${{ needs.protected-observer-inputs.outputs.observer_key_id }}",
        )
        self.assertNotIn("STABLE_LAUNCH_OBSERVER_ED25519_PUBLIC_KEY", yaml.safe_dump(enforced))
        self.assertNotIn("STABLE_LAUNCH_OBSERVER_KEY_ID", yaml.safe_dump(enforced))
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
        self.assertIn('"cli_release_tag": "agentplugins-v0.1.24"', production)
        self.assertIn('"cli_release_commit": "c78c79e44efd5ad07083d63436d9170b107df6cb"', production)
        self.assertIn('"cli_release_id": 379284682', production)
        prepare = (ROOT / "scripts/prepare_launch_evidence.py").read_text()
        self.assertNotIn('os.environ.get("GITHUB_TOKEN")', prepare)
        self.assertIn('token=os.environ.get("GH_TOKEN")', prepare)
        self.assertNotIn("fetch_production_directory", prepare)
        self.assertIn("fetch_staged_directory", prepare)
        self.assertIn('config["cli_release_id"]', prepare)
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
                    self.assertIn("agentplugins_0.1.24_linux_amd64", text)
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

    def test_upstream_package_cohort_authenticates_exact_release_assets(self) -> None:
        body = (ROOT / ".github/workflows/upstream-package-e2e.yml").read_text()
        workflow = load(ROOT / ".github/workflows/upstream-package-e2e.yml")
        self.assertEqual(workflow["permissions"], {"attestations": "read", "contents": "read"})
        self.assertIn('AGENTPLUGINS_VERSION: "0.1.26"', body)
        self.assertIn("AGENTPLUGINS_COMMIT: 24c2a74340d382abdc03a9f65563b951a9c1fcfb", body)
        self.assertIn("AGENTPLUGINS_NPM_INTEGRITY: sha512-7aPo4aoulltyx9zTbhJLxjCpQSvjKN0/YW/ATF6AUl2tnpWMwZf440SZbXfTGeuPRo0Ol2K5epNJFOguaPj7WQ==", body)
        self.assertIn("9867ad3cac009c45616ff41c06e019ada2c74a10e14a4f025c003971732a20a4", body)
        self.assertIn("--pattern release-manifest.json", body)
        self.assertIn('manifest["commit"] == os.environ["AGENTPLUGINS_COMMIT"]', body)
        self.assertEqual(body.count("gh attestation verify"), 1)
        self.assertIn('--source-digest "$AGENTPLUGINS_COMMIT"', body)
        self.assertIn('mkdir -p "$run_root/home"/{.codex,.cursor,.kiro}', body)
        self.assertIn('{item["status"] for item in data["targets"]} == {"external_completed"}', body)
        self.assertNotIn("kiro-cli", body)

    def upstream_inline_contract(self):
        workflow = load(ROOT / ".github/workflows/upstream-package-e2e.yml")
        step = next(step for step in workflow["jobs"]["install-lifecycle"]["steps"]
                    if step.get("name", "").startswith("Prove isolated preparation"))
        source = step["run"].split("<<'PY'\n", 1)[1].split("\nPY", 1)[0]
        parsed = ast.parse(source)
        spec = importlib.util.spec_from_file_location("upstream_test_helpers", ROOT / "scripts/run_chrome_five_client_lifecycle.py")
        helper = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(helper)
        namespace = {"json": json, "hashlib": hashlib, "base64": base64,
                     "require_success": helper.require_success, "validate_doctor": helper.validate_doctor}
        definitions = ast.Module(body=[node for node in parsed.body if isinstance(node, ast.FunctionDef)], type_ignores=[])
        exec(compile(definitions, "upstream-package-e2e.yml", "exec"), namespace)
        return namespace

    def test_upstream_update_distinguishes_immutable_rejection_from_no_change(self) -> None:
        validate = self.upstream_inline_contract()["validate_update"]
        blocked = {"command": "update", "result": "failure", "data": {
            "status": "preflight_failed", "succeeded": 0, "failed": 3, "targets": [],
        }}
        reason = "direct full-SHA installations require explicit switch"
        validate(blocked, reason, False)
        for field, changed in (("failed", 2), ("succeeded", False), ("targets", [{}])):
            forged = copy.deepcopy(blocked)
            forged["data"][field] = changed
            with self.subTest(field=field), self.assertRaises(AssertionError):
                validate(forged, reason, False)
        with self.assertRaises(AssertionError):
            validate(blocked, "unrelated failure", False)
        unchanged = {"command": "update", "result": "success", "data": {
            "status": "completed", "succeeded": 3, "failed": 0, "targets": [
                {"target": client, "status": "external_completed", "output": {
                    "result": {"no_change": True, "mutated": False}}}
                for client in ("codex", "cursor", "kiro")],
        }}
        validate(unchanged, "", True)
        for field, changed in (("no_change", False), ("mutated", True)):
            forged = copy.deepcopy(unchanged)
            forged["data"]["targets"][0]["output"]["result"][field] = changed
            with self.subTest(field=field), self.assertRaises(AssertionError):
                validate(forged, "", True)
        forged = copy.deepcopy(unchanged)
        forged["data"]["targets"][2]["target"] = "cursor"
        with self.assertRaises(AssertionError):
            validate(forged, "", True)

    def test_upstream_cleanup_rejects_open_operations_and_owned_residue(self) -> None:
        validate = self.upstream_inline_contract()["validate_cleanup"]
        listed = {"command": "list", "result": "success", "data": {"installations": []}}
        doctor = {"command": "doctor", "result": "success", "data": {
            "read_only": True, "installation_count": 0, "open_operation_count": 0,
            "findings": [{"status": "healthy", "code": "no_degradation_detected",
                          "message": "no tracked degradation was detected"}],
        }}
        with tempfile.TemporaryDirectory() as temporary:
            sandbox = Path(temporary)
            (sandbox / "state").mkdir()
            (sandbox / "state/state-v2.json").write_text(json.dumps({"schema_version": 4, "installations": []}))
            owned = sandbox / "owned-package"
            validate(sandbox, [owned], listed, doctor)
            for field, changed in (("open_operation_count", 1), ("installation_count", False), ("read_only", False)):
                forged = copy.deepcopy(doctor)
                forged["data"][field] = changed
                with self.subTest(field=field), self.assertRaises((AssertionError, RuntimeError)):
                    validate(sandbox, [owned], listed, forged)
            owned.symlink_to(sandbox / "missing")
            with self.assertRaises(AssertionError):
                validate(sandbox, [owned], listed, doctor)
            owned.unlink()
            residue = sandbox / "state/managed/residue"
            residue.parent.mkdir()
            residue.write_text("must not survive removal")
            with self.assertRaises(AssertionError):
                validate(sandbox, [owned], listed, doctor)

    def test_upstream_discovery_records_observed_sequence_without_repinning_source(self) -> None:
        validate = self.upstream_inline_contract()["discovery_identity"]
        selector = "discovery:upstash/context7//plugins/agent-plugins/context7"
        data = {"revision": "4e980f6b494d6f970cc5ec1df417ba684b2f6e0b",
                "tree_digest": "sha256:" + "a" * 64, "manifest_digest": "sha256:" + "b" * 64}
        record = {**data, "slug": selector, "repository": "upstash/context7", "package_path": "plugins/agent-plugins/context7"}
        encode = lambda value: base64.b64encode(json.dumps(value).encode()).decode()
        with tempfile.TemporaryDirectory() as temporary:
            sandbox = Path(temporary)
            (sandbox / "state").mkdir()
            def cache_fixture(sequence=20, count=2000, snapshot_count=None, projected_count=None):
                records = [record] + [{"slug": f"padding-{index}"} for index in range(count - 1)]
                search = {"sequence": sequence, "records": records}
                snapshot = {"sequence": sequence, "complete": True,
                            "records": records if snapshot_count is None else records[:snapshot_count],
                            "search_projection": {"record_count": count if projected_count is None else projected_count,
                                "digest": "sha256:" + hashlib.sha256(json.dumps(search).encode()).hexdigest()}}
                digest = "sha256:" + hashlib.sha256(json.dumps(snapshot).encode()).hexdigest()
                return {"sequence": sequence, "pointer": encode({"sequence": sequence}), "snapshot": encode(snapshot),
                        "envelope": encode({"sequence": sequence, "snapshot_digest": digest, "key_id": "test-key"}),
                        "search": encode(search)}
            path = sandbox / "state/discovery-v1-cache.json"
            for sequence in (20, 21):
                cache = cache_fixture(sequence)
                path.write_text(json.dumps(cache))
                identity = validate(sandbox, selector, data)
                self.assertEqual(identity["sequence"], sequence)
                self.assertEqual(identity["record_count"], 2000)
                forged_data = {**data, "revision": "f" * 40}
                with self.assertRaises(AssertionError):
                    validate(sandbox, selector, forged_data)
            for changed in ({"sequence": 13}, {"sequence": 19}, {"sequence": True},
                            {"count": 1999}, {"count": 1}, {"snapshot_count": 1999},
                            {"projected_count": 1999}, {"projected_count": True}):
                path.write_text(json.dumps(cache_fixture(**changed)))
                with self.subTest(changed=changed), self.assertRaises(AssertionError):
                    validate(sandbox, selector, data)
            for field, forged in (("sequence", 21), ("records", [record])):
                cache = cache_fixture()
                search = json.loads(base64.b64decode(cache["search"]))
                search[field] = forged
                cache["search"] = encode(search)
                path.write_text(json.dumps(cache))
                with self.subTest(field=field), self.assertRaises(AssertionError):
                    validate(sandbox, selector, data)

    def test_upstream_proof_is_credential_free_and_uploads_only_sanitized_summary(self) -> None:
        workflow = load(ROOT / ".github/workflows/upstream-package-e2e.yml")
        job = workflow["jobs"]["install-lifecycle"]
        packages = job["strategy"]["matrix"]["package"]
        self.assertEqual(len(packages), 5)
        self.assertEqual(packages[-2], {"id": "discovered-context7",
            "source": "discovery:upstash/context7//plugins/agent-plugins/context7", "repository": "upstash/context7",
            "plugin": "context7", "version": "1.0.0"})
        step = next(step for step in job["steps"] if step.get("name", "").startswith("Prove isolated preparation"))
        body = step["run"]
        self.assertNotIn("GH_TOKEN", json.dumps(step))
        self.assertIn('env = {"PATH": os.environ["PATH"]', body)
        self.assertIn('state_file.read_bytes() == state_before', body)
        self.assertIn('snapshot_roots(roots) == final_before', body)
        self.assertIn('ensure_sanitized(evidence, sandbox)', body)
        self.assertIn('if not directory and not discovered:', body)
        for claim in ("activation_executed", "runtime_executed", "authentication_executed", "version_upgrade_executed"):
            self.assertIn(f'"{claim}": False', body)
        self.assertEqual(job["steps"][-1]["with"]["path"].splitlines(), [
            "${{ runner.temp }}/upstream-${{ matrix.package.id }}/evidence/evidence.json",
            "${{ runner.temp }}/upstream-${{ matrix.package.id }}/evidence/evidence.sha256",
        ])

    def test_upstream_public_npm_and_reviewed_alias_have_no_private_overrides(self) -> None:
        workflow = load(ROOT / ".github/workflows/upstream-package-e2e.yml")
        job = workflow["jobs"]["install-lifecycle"]
        self.assertEqual(job["strategy"]["matrix"]["package"][-1], {
            "id": "reviewed-chrome-short-alias", "source": "chrome-devtools",
            "repository": "777genius/universal-agent-plugins", "revision": "signed-directory-release",
            "plugin": "chrome-devtools", "version": "1.7.0-uap.1"})
        install = next(step for step in job["steps"] if step.get("name") == "Install exact public npm wrapper without credentials")
        self.assertNotIn("env", install)
        for required in ('env -i PATH="$PATH"', 'NPM_CONFIG_CACHE="$tools_root/cache"',
                         'NPM_CONFIG_USERCONFIG="$tools_root/home/user.npmrc"', 'NPM_CONFIG_GLOBALCONFIG="$tools_root/home/global.npmrc"',
                         'NPM_CONFIG_REGISTRY=https://registry.npmjs.org', 'dist.integrity',
                         'npm audit signatures --prefix "$TOOLS_ROOT"'):
            self.assertIn(required, install["run"])
        body = commands(job)
        self.assertNotIn("AGENTPLUGINS_DIRECTORY_ORIGIN", body)
        self.assertNotIn("AGENTPLUGINS_INTERNAL_PROOF", body)
        self.assertIn('binary="$RUNNER_TEMP/upstream-npm/node_modules/.bin/agentplugins"', body)
        self.assertIn('native_digest == "sha256:" + hashlib.sha256(release_binary.read_bytes()).hexdigest()', body)
        self.assertIn('mutable_selector = discovered or directory', body)
        self.assertIn('success=mutable_selector', body)
        token_steps = [step.get("name") for step in job["steps"] if "GH_TOKEN" in json.dumps(step)]
        self.assertEqual(token_steps, ["Download and authenticate the exact released CLI"])

    def test_upstream_directory_identity_rejects_old_or_mismatched_reviewed_release(self) -> None:
        validate = self.upstream_inline_contract()["directory_identity"]
        registry = json.loads((ROOT / "registry/directory.json").read_text())
        product = next(item for item in registry["products"] if item["id"] == "chrome-devtools")
        distribution = copy.deepcopy(next(item for item in registry["distributions"] if item["id"] == "777genius/chrome-devtools-bridge"))
        declared = next(item for item in registry["distributions"] if item["id"] == product["default_distribution"])
        release = next(item for item in distribution["releases"] if item["sequence"] == 2)
        release["package_source"]["revision"] = "a" * 40
        snapshot = {"snapshot_schema_version": 1, "sequence": 20, "products": [product],
                    "distributions": [distribution, declared]}
        encode = lambda value: base64.b64encode(json.dumps(value).encode()).decode()
        with tempfile.TemporaryDirectory() as temporary:
            sandbox = Path(temporary)
            (sandbox / "state").mkdir()
            def check(candidate, *, sequence=20, source_path="plugins/chrome-devtools", bad_digest=False):
                candidate = copy.deepcopy(candidate)
                candidate["sequence"] = sequence
                digest = "sha256:" + hashlib.sha256(json.dumps(candidate).encode()).hexdigest()
                origin = {"product_id": "chrome-devtools", "distribution_id": distribution["id"],
                          "distribution_kind": "community_bridge", "desired_release_sequence": 2,
                          "snapshot_schema": 1, "snapshot_sequence": sequence, "snapshot_digest": digest}
                data = {"version": release["package_version"], "tree_digest": release["tree_digest"],
                        "manifest_digest": release["manifest_digest"], "revision": "a" * 40, "directory": origin}
                installed = {"origin_mode": "directory", "directory": origin,
                             "source": {"resolved_revision": "a" * 40, "package_subpath": source_path}}
                cache = {"sequence": sequence, "snapshot": encode(candidate),
                         "envelope": encode({"sequence": sequence, "snapshot_digest": "wrong" if bad_digest else digest, "key_id": "test-key"})}
                (sandbox / "state/directory-v1-cache.json").write_text(json.dumps(cache))
                return validate(sandbox, data, installed)
            self.assertEqual(check(snapshot)["snapshot_sequence"], 20)
            self.assertEqual(check(snapshot, sequence=21)["snapshot_sequence"], 21)
            for changed in ({"sequence": 13}, {"source_path": "wrong"}, {"bad_digest": True}):
                with self.subTest(changed=changed), self.assertRaises(AssertionError):
                    check(snapshot, **changed)
            for field, wrong in (("package_version", "1.7.0"), ("tree_digest", "sha256:" + "b" * 64),
                                 ("manifest_digest", "sha256:" + "c" * 64)):
                forged = copy.deepcopy(snapshot)
                forged["distributions"][0]["releases"][1][field] = wrong
                with self.subTest(field=field), self.assertRaises(AssertionError):
                    check(forged)

    def test_upstream_public_site_smoke_uses_real_pages_and_disposable_dependency_roots(self) -> None:
        workflow = load(ROOT / ".github/workflows/upstream-package-e2e.yml")
        job = workflow["jobs"]["public-site"]
        body = commands(job)
        source = (ROOT / "site/scripts/check-public-site.mjs").read_text()
        self.assertEqual(job["runs-on"], "ubuntu-24.04")
        self.assertIn('smoke_root="$RUNNER_TEMP/public-site-smoke"', body)
        self.assertIn('env -i PATH="$PATH"', body)
        self.assertIn('pnpm install --frozen-lockfile --ignore-scripts --store-dir "$SMOKE_ROOT/store"', body)
        self.assertIn('pnpm exec playwright install --with-deps chromium', body)
        for forbidden in ("GH_TOKEN", "UAP_SIGNED_SNAPSHOT_PATH", "pnpm generate"):
            self.assertNotIn(forbidden, body)
        for required in ('https://777genius.github.io/universal-agent-plugins/',
                         'registry.snapshot_sequence >= 20', 'discovery.sequence >= 20',
                         'discovery.records.length >= 2000', 'navigator.clipboard.readText()',
                         'recently found community packages',
                         "toContainText('Found on GitHub')",
                         "toContainText('Package format checked')",
                         'assert.deepEqual(errors, [])', 'width: 390', 'width: 1440',
                         "hasText: /^Reviewed plugin$/",
                         "failure === 'net::ERR_ABORTED'",
                         "pathname === '/universal-agent-plugins/discovery/latest.json'",
                         'discovery:upstash/context7//plugins/agent-plugins/context7'):
            self.assertIn(required, source)
        for forbidden in ('.route(', 'route.fulfill', 'addInitScript', 'executablePath', 'child_process'):
            self.assertNotIn(forbidden, source)
        self.assertNotIn('unreviewed packages from', source)

    def test_upstream_public_site_artifacts_are_json_and_newline_terminated_sha(self) -> None:
        node = shutil.which("node")
        if not node:
            self.skipTest("Node is required to execute the artifact serialization contract")
        script = r'''
import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
const source = readFileSync(process.argv[1], 'utf8').split('await mkdir(evidenceRoot,')[1];
const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
const files = {};
await new AsyncFunction('evidenceRoot', 'evidence', 'mkdir', 'writeFile', 'join', 'digest',
  'await mkdir(evidenceRoot,' + source)('memory', { schema_version: 1, viewports: [] },
  async () => {}, async (path, body) => { files[path] = body }, (_, name) => name,
  bytes => 'sha256:' + createHash('sha256').update(bytes).digest('hex'));
console.log(JSON.stringify(files));
'''
        result = subprocess.run([node, "--input-type=module", "-e", script,
                                 str(ROOT / "site/scripts/check-public-site.mjs")],
                                capture_output=True, text=True, check=True)
        files = json.loads(result.stdout)
        body = files["public-site.json"]
        self.assertEqual(json.loads(body), {"schema_version": 1, "viewports": []})
        self.assertTrue(body.endswith("\n"))
        self.assertEqual(files["public-site.sha256"], hashlib.sha256(body.encode()).hexdigest() + "  public-site.json\n")

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
        self.assertIn("inputs.publication_sequence != ''", required["if"])
        self.assertIn("inputs.caller_ref == 'refs/heads/main'", required["if"])
        self.assertIn("directory-publication.yml@refs/heads/main", required["if"])
        scheduled = {
            name: workflow["jobs"][name]
            for name in (
                "scheduled-fixture-contract",
                "scheduled-production-directory-observation",
            )
        }
        self.assertEqual(
            set(scheduled),
            {"scheduled-fixture-contract", "scheduled-production-directory-observation"},
        )
        for job in scheduled.values():
            self.assertEqual(
                job["if"],
                "github.event_name == 'schedule' || inputs.run_scheduled_regression",
            )
        scheduled_body = yaml.safe_dump(scheduled)
        self.assertNotIn("inputs.publication_", scheduled_body)
        self.assertNotIn("launch-evidence-e2e.yml", scheduled_body)
        for name, job in scheduled.items():
            with self.subTest(job=name):
                job_commands = commands(job)
                requirements = pinned_requirements(job_commands)
                self.assertIn("cryptography==46.0.3", requirements)
                self.assertIn("jsonschema==4.26.0", requirements)
                self.assertIn("PyYAML==6.0.3", requirements)
                self.assertLess(
                    job_commands.index("jsonschema==4.26.0"),
                    job_commands.index("scripts/run_launch_evidence_e2e.py")
                    if "scripts/run_launch_evidence_e2e.py" in job_commands
                    else job_commands.index("from scripts.run_launch_evidence_e2e import"),
                )
        observation = commands(scheduled["scheduled-production-directory-observation"])
        self.assertIn("fetch_production_directory", observation)
        self.assertIn("production_identity_from_materialized_ledger", observation)
        for keyword in (
            "expected_publication_id", "expected_sequence",
            "expected_snapshot_digest", "expected_source_commit",
        ):
            self.assertIn(keyword, observation)
        self.assertIn('"runtime_claims": False', observation)
        self.assertIn('"oauth_claims": False', observation)
        self.assertIn("SHA256SUMS", observation)
        self.assertIn("production-marker.json", observation)
        self.assertIn("directory-publication-schema-1-production", (
            ROOT / "registry/publication/production-marker.json"
        ).read_text())
        self.assertIn("git ls-remote --refs origin", observation)
        self.assertIn("git worktree add --detach _production-ledger", observation)
        self.assertNotIn("ref: directory-publication-ledger", yaml.safe_dump(
            scheduled["scheduled-production-directory-observation"]
        ))
        public_reads = workflow["jobs"]["public-read-flows"]
        self.assertEqual(
            public_reads["if"],
            "github.event_name == 'schedule' || inputs.consent || inputs.run_scheduled_regression",
        )

    def test_live_sequence_gate_dominates_every_effectful_job(self) -> None:
        workflow = load(LIVE)
        gate_name = "validate-publication-sequence"
        gate = workflow["jobs"][gate_name]
        self.assertNotIn("if", gate)
        self.assertEqual(gate["steps"][0]["env"]["EVENT_NAME"], "${{ github.event_name }}")
        effectful = set(workflow["jobs"]) - {gate_name}
        for name in effectful:
            job = workflow["jobs"][name]
            needs = job.get("needs", [])
            needs = {needs} if isinstance(needs, str) else set(needs)
            with self.subTest(job=name):
                self.assertIn(gate_name, needs)
                if "always()" in job.get("if", ""):
                    self.assertIn("needs.validate-publication-sequence.result == 'success'", job["if"])

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
                self.assertIn("inputs.publication_sequence != ''", condition)
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
        self.assertIn("always()", gate["if"])
        self.assertIn("needs.validate-publication-sequence.result == 'success'", gate["if"])
        self.assertIn("inputs.consent", gate["if"])
        self.assertIn("inputs.caller_ref == 'refs/heads/main'", gate["if"])
        self.assertIn("directory-publication.yml@refs/heads/main", gate["if"])
        self.assertEqual(
            set(gate["needs"]),
            {"validate-publication-sequence", "required-stable-launch-evidence", "public-read-flows"},
        )
        step = gate["steps"][0]
        self.assertEqual(
            step["env"],
            {
                "STABLE_E2E_RESULT": "${{ needs.required-stable-launch-evidence.result }}",
                "PUBLIC_READ_RESULT": "${{ needs.public-read-flows.result }}",
                "READINESS_DIGEST": "${{ needs.required-stable-launch-evidence.outputs.readiness_evidence_digest }}",
                "POLICY_DIGEST": "${{ needs.required-stable-launch-evidence.outputs.source_policy_evidence_digest }}",
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
        self.assertEqual(publication_call["publication_sequence"], "${{ needs.sign.outputs.sequence }}")
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
            "EXPECTED_LEDGER_HEAD", "EXPECTED_PUBLICATION_COMMIT",
        ):
            self.assertIn(field, verify["env"])
        self.assertIn("raw.githubusercontent.com", verify["run"])
        compatibility = next(
            step for step in exact["steps"]
            if step.get("name") == "Prove compatibility with the exact released CLI"
        )
        self.assertNotIn("EXPECTED_SOURCE_COMMIT", compatibility["env"])
        self.assertEqual(
            compatibility["env"]["EXPECTED_SNAPSHOT_DIGEST"],
            "${{ needs.sign.outputs.snapshot_digest }}",
        )
        self.assertIn("verify_released_cli_directory_parity.py", compatibility["run"])
        self.assertIn('--snapshot "${snapshot}"', compatibility["run"])
        self.assertIn('--sequence "${EXPECTED_SEQUENCE}"', compatibility["run"])
        self.assertIn('--snapshot-digest "${EXPECTED_SNAPSHOT_DIGEST}"', compatibility["run"])
        self.assertIn("--product-id context7", compatibility["run"])
        self.assertNotIn('item["distribution_id"] == "777genius/context7"', compatibility["run"])
        self.assertNotIn('result["revision"] == expected_source["revision"]', compatibility["run"])
        deploy_needs = workflow["jobs"]["deploy"]["needs"]
        self.assertIn("sign", deploy_needs)
        self.assertIn("gate_exact_staged_publication", deploy_needs)
        self.assertIn("required_catalog_readiness", deploy_needs)
        production = workflow["jobs"]["observe_production_latest"]
        self.assertIn("deploy", production["needs"])
        self.assertIn("record_production_marker", production["needs"])
        self.assertNotIn("observe_production_latest", required["needs"])
        self.assertIn("observe_production_latest.py", commands(production))
        self.assertEqual(production["permissions"], {"contents": "read"})
        marker = workflow["jobs"]["record_production_marker"]
        self.assertEqual(set(marker["needs"]), {"materialize_site", "deploy"})
        self.assertIn("needs.deploy.result == 'success'", marker["if"])
        self.assertEqual(marker["environment"], "directory-publication")
        marker_body = commands(marker)
        self.assertIn("directory_publication_cas.py production-publish", marker_body)
        self.assertIn("production-marker.json", marker_body)
        self.assertIn('--production-new "${EXPECTED_PRODUCTION_COMMIT}"', marker_body)

    def test_account_runtime_ceremony_remains_separate_from_catalog_publication(self) -> None:
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
        self.assertIn("needs.required_catalog_readiness.result == 'success'", deploy_if)
        self.assertNotIn("gate_launch_approval", deploy_if)

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
        exact_native_pattern = 'native-*-${{ github.run_id }}-${{ needs.native-release.outputs.evidence_run_attempt }}'
        self.assertTrue(any(
            step.get("with", {}).get("pattern") == exact_native_pattern
            for step in launch["jobs"]["enforced-stable-gate"]["steps"]
        ))
        self.assertIn("needs.node22-npm-facade.outputs.evidence_run_attempt", enforced_body)
        self.assertIn("needs.protected-observer-inputs.outputs.evidence_run_attempt", enforced_body)
        self.assertIn('run_attempt=${EVIDENCE_RUN_ATTEMPT}', enforced_body)
        aggregate_body = yaml.safe_dump(launch["jobs"]["aggregate-one-release"])
        self.assertTrue(any(
            step.get("with", {}).get("pattern") == exact_native_pattern
            for step in launch["jobs"]["aggregate-one-release"]["steps"]
        ))
        self.assertNotIn('test "$NATIVE_ATTEMPT" = "$NODE_ATTEMPT"', aggregate_body)
        self.assertEqual(
            attestation["uses"],
            "actions/attest-build-provenance@4d101475d8b20a2381f78447822ac1eab6504dd8",
        )
        expected_outputs = {
            "evidence_artifact_name", "launch_evidence_digest", "workflow_source_digest",
            "evidence_run_attempt", "attestation_artifact_name",
            "source_policy_artifact_name", "source_policy_evidence_digest",
            "readiness_artifact_name", "readiness_evidence_digest",
        }
        self.assertEqual(set(launch["on"]["workflow_call"]["outputs"]), expected_outputs)
        self.assertEqual(set(live["on"]["workflow_call"]["outputs"]), expected_outputs)
        self.assertEqual(attestation["id"], "attestation")
        attestation_upload = next(
            step for step in attester["steps"]
            if "upload-artifact" in step.get("uses", "")
        )
        self.assertEqual(
            attestation_upload["with"]["name"],
            "${{ steps.attestation-identity.outputs.artifact_name }}",
        )
        self.assertEqual(
            attestation_upload["with"]["path"],
            "attestation/github-attestation.jsonl",
        )
        persist = publication["jobs"]["record_launch_approval"]
        body = commands(persist)
        self.assertIn("jsonschema==4.26.0", pinned_requirements(body))
        self.assertNotIn("jsonschema==4.25.1", str(publication))
        self.assertIn("materialize_launch_evidence.py verify-bundle", body)
        self.assertIn("--verify-attestation", body)
        self.assertEqual(body.count("--attestation-bundle launch-attestation/github-attestation.jsonl"), 2)
        self.assertNotIn("GH_TOKEN", yaml.safe_dump(persist))
        self.assertIn('--expected-run-attempt "${EXPECTED_EVIDENCE_RUN_ATTEMPT}"', body)
        self.assertIn("materialize_launch_evidence.py commit", body)
        self.assertIn("directory_publication_cas.py evidence-publish", body)
        self.assertNotIn("ls-remote", body)
        self.assertIn("validate_directory", (ROOT / "scripts/materialize_launch_evidence.py").read_text())
        self.assertIn('--main-old "${EXPECTED_MAIN_PARENT}"', body)
        self.assertIn('--ledger-old "${EXPECTED_LEDGER_COMMIT}"', body)
        self.assertIn('--approval-tag "${marker_ref}"', body)

    def test_staged_publication_resume_reuses_exact_signed_identity_without_signing(self) -> None:
        workflow = load(DIRECTORY_PUBLICATION)
        dispatch = workflow["on"]["workflow_dispatch"]["inputs"]
        self.assertEqual(dispatch["resume_publication_id"]["type"], "string")
        self.assertEqual(dispatch["resume_sequence"]["type"], "string")
        self.assertNotIn(
            ".github/workflows/directory-publication.yml",
            workflow["on"]["push"]["paths"],
        )

        prepare = workflow["jobs"]["prepare"]
        resume = next(step for step in prepare["steps"] if step.get("id") == "resume")
        guard = next(
            step for step in prepare["steps"]
            if step.get("name") == "Reject a new append while a signed sequence is still staged"
        )
        self.assertIn("resume_publication_id != ''", resume["if"])
        self.assertIn("resume_publication_id == ''", guard["if"])
        for required in (
            "refs/tags/${tag_name}^{commit}",
            "directory_publication_cas.py staged-lineage-verify",
            'echo "ledger_head=${ledger_head}"',
            "verify_directory_publication.py",
            'test "${production_sequence}" -lt "${sequence}"',
            'git merge-base --is-ancestor "${marker_commit}" "${SOURCE_HEAD}"',
        ):
            self.assertIn(required, resume["run"])
        self.assertIn("use the explicit resume inputs", guard["run"])

        signer = workflow["jobs"]["sign"]
        for output in (
            "ledger_commit", "materialized_ledger_commit", "ledger_head", "marker_commit",
            "publication_id", "sequence", "snapshot_digest", "main_parent",
        ):
            self.assertIn("needs.prepare.outputs.resume_", signer["outputs"][output])
        signing_step = next(step for step in signer["steps"] if step.get("id") == "signed")
        publisher_step = next(step for step in signer["steps"] if step.get("id") == "publisher")
        self.assertEqual(signing_step["if"], "inputs.resume_publication_id == ''")
        self.assertEqual(publisher_step["if"], "inputs.resume_publication_id == ''")

        persist = workflow["jobs"]["record_launch_approval"]
        source_checkout = next(
            step for step in persist["steps"]
            if step.get("with", {}).get("path") == "trusted-source"
        )
        self.assertEqual(source_checkout["with"]["ref"], "${{ needs.sign.outputs.main_parent }}")
        persist_body = commands(persist)
        self.assertIn('--main-parent "${EXPECTED_MAIN_PARENT}"', persist_body)
        self.assertIn('--main-old "${EXPECTED_MAIN_PARENT}"', persist_body)
        self.assertIn('--approval-target "${EXPECTED_PUBLICATION_COMMIT}"', persist_body)
        self.assertIn('--expected-publication-source-commit "${EXPECTED_SOURCE_COMMIT}"', persist_body)

    def test_staged_publication_supersession_authenticates_then_appends_higher_sequence(self) -> None:
        workflow = load(DIRECTORY_PUBLICATION)
        dispatch = workflow["on"]["workflow_dispatch"]["inputs"]
        self.assertEqual(dispatch["supersede_publication_id"]["type"], "string")
        self.assertEqual(dispatch["supersede_sequence"]["type"], "string")

        prepare = workflow["jobs"]["prepare"]
        ledger = next(step for step in prepare["steps"] if step.get("id") == "ledger")
        staged = next(step for step in prepare["steps"] if step.get("id") == "resume")
        guard = next(
            step for step in prepare["steps"]
            if step.get("name") == "Reject a new append while a signed sequence is still staged"
        )
        candidate = next(step for step in prepare["steps"] if step.get("id") == "candidate")
        self.assertIn("supersession are mutually exclusive", ledger["run"])
        self.assertIn("supersede_publication_id and supersede_sequence must be supplied together", ledger["run"])
        self.assertIn("supersede_publication_id != ''", staged["if"])
        self.assertIn("supersede_publication_id == ''", guard["if"])
        for exact_authentication in (
            "refs/tags/${tag_name}^{commit}",
            "directory_publication_cas.py staged-lineage-verify",
            'test "$(python3 -c \'import json,sys; print(json.load(open(sys.argv[1]))["publication_id"])\' "${feed}/${snapshot_path}")" = "${REQUESTED_PUBLICATION_ID}"',
            'test "${production_sequence}" -lt "${sequence}"',
        ):
            self.assertIn(exact_authentication, staged["run"])
        self.assertIn("directory_staged_supersession.py", candidate["run"])
        self.assertLess(
            prepare["steps"].index(staged), prepare["steps"].index(candidate),
            "stale or mismatched staged inputs must fail before candidate preparation",
        )
        self.assertLess(
            prepare["steps"].index(candidate),
            next(index for index, step in enumerate(prepare["steps"])
                 if str(step.get("uses", "")).startswith("actions/upload-artifact")),
            "unchanged supersession must fail before candidate upload or signing",
        )

        signer = workflow["jobs"]["sign"]
        signing_step = next(step for step in signer["steps"] if step.get("id") == "signed")
        publisher_step = next(step for step in signer["steps"] if step.get("id") == "publisher")
        self.assertEqual(signing_step["if"], "inputs.resume_publication_id == ''")
        self.assertEqual(publisher_step["if"], "inputs.resume_publication_id == ''")

    def test_protected_observer_preflights_oidc_claim_names_without_logging_token(self) -> None:
        workflow = load(LAUNCH)
        observer = workflow["jobs"]["protected-observer-inputs"]
        claim_step = next(
            step for step in observer["steps"]
            if step.get("name") == "Validate non-secret OIDC identity claims locally"
        )
        token_index = next(
            index for index, step in enumerate(observer["steps"])
            if step.get("name") == "Obtain short-lived GitHub OIDC identity"
        )
        claim_index = observer["steps"].index(claim_step)
        request_index = next(
            index for index, step in enumerate(observer["steps"])
            if step.get("id") == "observer"
        )
        self.assertLess(token_index, claim_index)
        self.assertLess(claim_index, request_index)
        body = claim_step["run"]
        for field in (
            "repository_id", "repository_owner_id", "workflow_ref",
            "job_workflow_ref", "workflow_sha", "job_workflow_sha",
            "environment", "event_name",
        ):
            self.assertIn(f'"{field}"', body)
        self.assertIn("claim mismatch", body)
        self.assertIn("GITHUB_REPOSITORY_OWNER_ID", body)
        self.assertIn("GITHUB_REPOSITORY_ID", body)
        self.assertIn("immutable_subject", body)
        self.assertNotIn("print(claims", body)
        self.assertNotIn("print(token", body)

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
        self.assertIn("needs.required_catalog_readiness.result == 'success'", deploy_if)
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
        self.assertEqual(replay["permissions"], {"contents": "read"})
        state = next(step for step in replay["steps"] if step.get("id") == "state")
        self.assertIn("materialize_launch_evidence.py verify-completed", state["run"])
        self.assertIn('--expected-publication-id "$expected_publication_id"', state["run"])
        self.assertIn(
            '--expected-publication-source-commit "$expected_publication_source_commit"',
            state["run"],
        )
        self.assertIn('expected_main_parent="${EVENT_SOURCE_COMMIT}"', state["run"])
        self.assertNotIn("GH_TOKEN", yaml.safe_dump(replay))
        self.assertNotIn("github.token", yaml.safe_dump(workflow))
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
            {"sign", "materialize_site", "gate_exact_staged_publication", "required_catalog_readiness"},
        )
        self.assertIn("needs.required_catalog_readiness.result == 'success'", deploy["if"])
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
