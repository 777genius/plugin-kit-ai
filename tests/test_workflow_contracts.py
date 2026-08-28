import json
import hashlib
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
DISCOVERY_INDEX = ROOT / ".github/workflows/discovery-index.yml"
UPSTREAM_PROMOTION = ROOT / ".github/workflows/upstream-promotion-readiness.yml"
OBSERVER_RUNBOOK = ROOT / "docs/OBSERVER_OPERATIONS.md"
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


def pinned_requirements(body: str) -> set[str]:
    return set(re.findall(r"(?<![A-Za-z0-9_.-])[A-Za-z0-9_.-]+==[A-Za-z0-9_.+-]+", body))


class WorkflowContractTests(unittest.TestCase):
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
        writable = next(line for line in runner_unit.splitlines() if line.startswith("BindPaths=-/var/lib/uap-observer/profiles/"))
        self.assertEqual(writable.split("=")[1].split(), [
            f"-/var/lib/uap-observer/profiles/{client}/{leaf}"
            for client in ("codex", "cursor", "kiro") for leaf in (".auth", ".state")
        ])
        for forbidden in ("/.config", "/.cache", "/.codex", "/.cursor", "/.kiro", "/.local"):
            self.assertNotIn(forbidden, writable)

    def test_installed_state_projection_validator_is_strict_and_nonempty(self) -> None:
        library = (ROOT / "deploy/uap-observer-install-lib.sh").read_text()
        start = library.index("def native_projection(encoded,suffix,profile,proof):")
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
                "snapshot_digest": digest, "binary_digest": digest,
                "dependency_identity": "locked", "installer_version": "0.1.18",
                "adapter_version": "r14d", "client_version": None,
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
            "data": {"clients": [{"client_id": "codex", "detected": True}]},
        }
        with tempfile.TemporaryDirectory() as temporary:
            evidence = Path(temporary) / "doctor.json"
            evidence.write_text(json.dumps(valid))
            accepted = subprocess.run(
                ["/usr/bin/python3", "-c", snippet, "codex", str(evidence)],
                text=True, capture_output=True,
            )
            self.assertEqual(accepted.returncode, 0, accepted.stderr)
            adversarial = (
                b'{"schema_version":true,"command":"doctor","result":"success","data":{}}',
                b'{"schema_version":1,"Schema_Version":1,"command":"doctor","result":"success","data":{}}',
                b'{"schema_version":1,"command":"doctor","result":"success","data":{},"value":NaN}',
                b'{"schema_version":1,"command":"doctor","result":"success","data":{},"value":1e400}',
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

    def test_installer_runtime_pins_equal_current_manifest_and_files(self) -> None:
        installer = (ROOT / "deploy/uap-observer-install.sh").read_text()
        manifest_path = ROOT / "deploy/uap-observer-runtime.sha256"
        entries = [line.split(maxsplit=1) for line in manifest_path.read_text().splitlines() if line.strip()]
        self.assertTrue(entries)
        self.assertEqual(len(entries), len({relative for _, relative in entries}))
        manifest = {relative: digest for digest, relative in entries}
        def file_digest(relative: str) -> str:
            return hashlib.sha256((ROOT / relative).read_bytes()).hexdigest()
        self.assertEqual(manifest, {relative: file_digest(relative) for relative in manifest})
        self.assertEqual(manifest["observer/fixed_runner.py"], file_digest("observer/fixed_runner.py"))
        self.assertEqual(manifest["observer/fixed_adapters.py"], file_digest("observer/fixed_adapters.py"))
        assignments = dict(re.findall(
            r"^(runner_digest|adapter_digest)=([a-f0-9]{64})$", installer, re.MULTILINE,
        ))
        self.assertEqual(assignments, {
            "runner_digest": manifest["observer/fixed_runner.py"],
            "adapter_digest": manifest["observer/fixed_adapters.py"],
        })
        idempotent = re.search(
            r"observer_validate_installed_closure_sources .*?\\\n"
            r"(?:.*?\\\n){1,3}\s+([a-f0-9]{64}) \\\n\s+([a-f0-9]{64}) \\\n",
            installer,
        )
        self.assertIsNotNone(idempotent)
        self.assertEqual(idempotent.groups(), (
            manifest["observer/fixed_runner.py"], manifest["observer/fixed_adapters.py"],
        ))
        library_pin = re.search(r'sha256sum "\$install_lib".*?= ([a-f0-9]{64})$', installer, re.MULTILINE)
        self.assertIsNotNone(library_pin)
        self.assertEqual(library_pin.group(1), file_digest("deploy/uap-observer-install-lib.sh"))
        closure_pin = re.search(r"^runtime_manifest_digest=([a-f0-9]{64})$", installer, re.MULTILINE)
        self.assertIsNotNone(closure_pin)
        self.assertEqual(closure_pin.group(1), hashlib.sha256(manifest_path.read_bytes()).hexdigest())

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
        self.assertEqual(version, "0.1.18")
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
            "0.1.18",
            "sha256:0e8f7316ddef542067bdd7276273fffa3bc00532afed8fd42be12f612aedea57",
            "sha256:d581ac34d9880afe998f8f871df285b5474623778d2eae98ebc8780a932a9fa8",
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

    def test_discovery_index_is_lkg_protected_and_deploys_the_exact_ledger(self) -> None:
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
        self.assertIn("discovery-index-sequence-", commands(signer))
        incomplete = workflow["jobs"]["incomplete-scan"]
        self.assertIn("needs.scan.outputs.complete != 'true'", incomplete["if"])
        self.assertIn("exit 1", commands(incomplete))
        deploy = workflow["jobs"]["deploy"]
        checkout = next(step for step in deploy["steps"] if step.get("uses", "").startswith("actions/checkout"))
        self.assertEqual(checkout["with"]["ref"], "${{ needs.sign-and-publish.outputs.ledger_commit }}")
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
        self.assertIn("diff --exit-code \"${EXPECTED_LEDGER_COMMIT}\" HEAD -- discovery", body)
        self.assertEqual(body.count("':!discovery'"), 2)

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
            "current installer supports a fresh host only",
            "repository does not install UID-, cgroup-, or service-identity firewall rules",
            "`IPAddressDeny=any`",
            "resolve_github_release",
            '"agentplugins-v0.1.18"',
            'asset_name="agentplugins_0.1.18_linux_amd64"',
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
        self.assertIn("five minutes before", runbook)
        self.assertIn("genuinely\nexternal unmerged PR", runbook)
        self.assertNotIn("uap-observer-egress-fqdns.txt", runbook)
        self.assertNotIn("NO_OPEN_BROWSER=1 agent login", runbook)
        self.assertNotIn("/root/uap-observer-egress-proxy.socket", runbook)
        self.assertNotIn("/root/uap-observer-egress-proxy.service", runbook)
        self.assertNotIn("/usr/local/libexec/", runbook)
        self.assertIn("/opt/uap-observer-current/libexec/uap-observer-provision-profile", runbook)
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
            "agentplugins_0.1.18_darwin_arm64", "agentplugins_0.1.18_darwin_amd64",
            "agentplugins_0.1.18_linux_arm64", "agentplugins_0.1.18_linux_amd64",
            "agentplugins_0.1.18_windows_amd64.exe", "agentplugins_0.1.18_windows_arm64.exe",
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
        self.assertIn("universal-agent-plugins-0.1.18.tgz", commands(npm))
        self.assertIn("--asset-name agentplugins_0.1.18_linux_amd64", commands(npm))
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
        self.assertIn('"cli_release_tag": "agentplugins-v0.1.18"', production)
        self.assertIn('"cli_release_commit": "74a3790ee15d92afda8e8e3dd8f903c04811cfc7"', production)
        prepare = (ROOT / "scripts/prepare_launch_evidence.py").read_text()
        self.assertNotIn('os.environ.get("GITHUB_TOKEN")', prepare)
        self.assertIn('token=os.environ.get("GH_TOKEN")', prepare)
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
                    self.assertIn("agentplugins_0.1.18_linux_amd64", text)
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
        self.assertIn("AGENTPLUGINS_COMMIT: 74a3790ee15d92afda8e8e3dd8f903c04811cfc7", body)
        self.assertIn("--pattern release-manifest.json", body)
        self.assertIn('manifest["commit"] == os.environ["AGENTPLUGINS_COMMIT"]', body)
        self.assertEqual(body.count("gh attestation verify"), 1)
        self.assertIn('--source-digest "$AGENTPLUGINS_COMMIT"', body)
        self.assertIn('mkdir -p "$run_root/home"/{.codex,.cursor,.kiro}', body)
        self.assertIn('{item["status"] for item in data["targets"]} == {"external_completed"}', body)
        self.assertNotIn("kiro-cli", body)

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
        self.assertEqual(replay["permissions"], {"contents": "read"})
        state = next(step for step in replay["steps"] if step.get("id") == "state")
        self.assertIn("materialize_launch_evidence.py verify-completed", state["run"])
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
