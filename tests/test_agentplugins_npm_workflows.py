import re
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[1]
PUBLISH = ROOT / ".github/workflows/agentplugins-npm-publish.yml"
PROOF = ROOT / ".github/workflows/agentplugins-platform-proof.yml"


def load(path: Path):
    return yaml.safe_load(path.read_text())


class AgentpluginsNpmWorkflowContractTests(unittest.TestCase):
    def test_every_node_job_uses_the_pinned_current_node_22_lts_runtime(self):
        publish = load(PUBLISH)
        proof = load(PROOF)

        def setup_node_versions(job):
            return [
                step.get("with", {}).get("node-version")
                for step in job["steps"]
                if step.get("uses", "").startswith("actions/setup-node@")
            ]

        self.assertEqual(setup_node_versions(publish["jobs"]["stage"]), ["22.23.2"])
        self.assertEqual(setup_node_versions(publish["jobs"]["publish"]), ["22.23.2"])
        self.assertEqual(setup_node_versions(proof["jobs"]["native-proof"]), ["22.23.2"])
        self.assertEqual(
            re.findall(r'node-version:\s*["\']?([^"\'\s]+)', PUBLISH.read_text() + PROOF.read_text()),
            ["22.23.2", "22.23.2", "22.23.2"],
        )
        package = load(ROOT / "npm/universal-agent-plugins/package.json")
        self.assertEqual(package["engines"], {"node": ">=22"})
        npm_readme = (ROOT / "npm/universal-agent-plugins/README.md").read_text()
        self.assertIn("Node.js 22 or newer", npm_readme)
        release_doc = (ROOT / "docs/AGENTPLUGINS_NPM_RELEASE.md").read_text()
        self.assertIn("pinned current\nNode 22 LTS CI and release runtime", release_doc)
        self.assertIn("22.23.2` is not the minimum\npackage version", release_doc)

    def test_publish_is_manual_version_serialized_and_verify_only_by_default(self):
        workflow = load(PUBLISH)
        dispatch = workflow[True]["workflow_dispatch"]["inputs"]
        self.assertEqual(dispatch["verify_only"]["default"], True)
        self.assertEqual(
            {"uap_tag", "version", "plugin_kit_tag", "plugin_kit_commit", "verify_only"},
            set(dispatch),
        )
        self.assertIn("inputs.version", workflow["concurrency"]["group"])
        self.assertEqual(workflow["concurrency"]["cancel-in-progress"], False)
        self.assertNotIn("push", workflow[True])
        self.assertNotIn("release", workflow[True])

    def test_stage_authenticates_exact_cross_repository_release_and_tarball(self):
        body = PUBLISH.read_text()
        for value in (
            "777genius/plugin-kit-ai",
            "agentplugins-release.yml",
            "--source-digest \"$PLUGIN_KIT_COMMIT\"",
            "--source-ref refs/heads/main",
            "--deny-self-hosted-runners",
            "release-manifest.json",
            "checksums.txt",
            "release.immutable !== true",
            "scripts/release-assets.js verify",
            "assets.repository !== \"777genius/plugin-kit-ai\"",
            "vendored binary",
        ):
            self.assertIn(value, body)
        self.assertRegex(body, r"for asset in \"\$release_root\"/\*; do\s+gh attestation verify")
        self.assertIn('test "$UAP_TAG" = "v$VERSION"', body)
        self.assertIn('test "$PLUGIN_KIT_TAG" = "agentplugins-v$VERSION"', body)
        self.assertIn('test "$GITHUB_REF" = "refs/tags/$UAP_TAG"', body)
        self.assertIn('test "$GITHUB_SHA" = "$(git rev-parse HEAD)"', body)
        self.assertIn(
            '"$GITHUB_WORKSPACE/npm/universal-agent-plugins/test/fixtures/historical-evidence"',
            body,
        )

    def test_caller_and_reusable_proof_require_public_main_ancestry(self):
        for path in (PUBLISH, PROOF):
            body = path.read_text()
            self.assertIn('test "$GITHUB_REPOSITORY" = "777genius/universal-agent-plugins"', body)
            self.assertIn("gh api repos/777genius/universal-agent-plugins --jq .default_branch", body)
            self.assertIn("https://github.com/777genius/universal-agent-plugins.git", body)
            self.assertIn("refs/heads/main:refs/remotes/public/main", body)
            self.assertIn("git merge-base --is-ancestor HEAD refs/remotes/public/main", body)
            self.assertLess(body.index("git fetch --no-tags --force"), body.index("git merge-base --is-ancestor"))

    def test_publish_is_downstream_of_every_native_gate_and_uses_oidc_provenance(self):
        workflow = load(PUBLISH)
        publish = workflow["jobs"]["publish"]
        self.assertEqual(set(publish["needs"]), {"stage", "six-platform-proof"})
        self.assertEqual(publish["permissions"], {"contents": "read", "id-token": "write"})
        self.assertIn("inputs.verify_only == false", publish["if"])
        body = "\n".join(step.get("run", "") for step in publish["steps"])
        self.assertIn("npm publish", body)
        self.assertIn("--provenance", body)
        self.assertIn('test -z "${NPM_TOKEN:-}"', body)
        self.assertIn("npm view universal-agent-plugins versions", body)
        self.assertIn("npm dist-tag ls universal-agent-plugins", body)

    def test_publish_pins_an_oidc_capable_npm_cli_before_publication(self):
        workflow = load(PUBLISH)
        steps = workflow["jobs"]["publish"]["steps"]
        install = next(step for step in steps if step.get("name") ==
                       "Install the pinned npm CLI required for trusted publishing")
        self.assertIn("npm install --global npm@12.0.2", install["run"])
        self.assertIn('test "$(npm --version)" = 12.0.2', install["run"])
        install_index = steps.index(install)
        publish_index = next(i for i, step in enumerate(steps) if "npm publish" in step.get("run", ""))
        self.assertLess(install_index, publish_index)

    def test_publish_lock_serializes_all_versions_and_eligibility_is_rechecked_last(self):
        workflow = load(PUBLISH)
        self.assertIn("inputs.version", workflow["concurrency"]["group"])
        self.assertEqual(workflow["concurrency"]["cancel-in-progress"], False)
        publish = workflow["jobs"]["publish"]
        self.assertEqual(publish["concurrency"], {
            "group": "agentplugins-npm-publish",
            "cancel-in-progress": False,
        })
        body = "\n".join(step.get("run", "") for step in publish["steps"])
        tarball = body.index('tarball=$(find "$RUNNER_TEMP/npm-tarball"')
        versions = body.index("npm view universal-agent-plugins versions")
        tags = body.index("npm dist-tag ls universal-agent-plugins")
        publication = body.index('npm publish "$tarball"')
        self.assertLess(tarball, versions)
        self.assertLess(versions, tags)
        self.assertLess(tags, publication)
        self.assertRegex(body, r"(?m)^NODE\nnpm publish \"\$tarball\"")

    def test_proof_matrix_is_six_native_hosted_architectures(self):
        workflow = load(PROOF)
        matrix = workflow["jobs"]["native-proof"]["strategy"]["matrix"]["include"]
        self.assertEqual({row["target"] for row in matrix}, {
            "darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64",
            "windows-amd64", "windows-arm64",
        })
        self.assertEqual({row["runner"] for row in matrix}, {
            "macos-15-intel", "macos-15", "ubuntu-24.04", "ubuntu-24.04-arm",
            "windows-2025", "windows-11-arm",
        })
        self.assertTrue(workflow["jobs"]["native-proof"]["strategy"]["fail-fast"] is False)
        self.assertNotRegex(PROOF.read_text(), r"(?i)qemu|docker|virtualbox|vmware")

    def test_six_platform_job_consumes_installed_npm_shim_evidence(self):
        body = PROOF.read_text()
        self.assertIn("proof.proofs?.installed_npm_shim_executed !== true", body)
        self.assertIn("six-platform evidence did not prove installed npm shim execution", body)
        script = (ROOT / "npm/universal-agent-plugins/scripts/platform-proof.js").read_text()
        self.assertIn("installed_npm_shim_executed: true", script)

    def test_public_proof_is_anonymous_isolated_and_exercises_lifecycle(self):
        publish = load(PUBLISH)
        public = publish["jobs"]["public-six-platform-proof"]
        self.assertEqual(set(public["needs"]), {"stage", "publish"})
        self.assertEqual(public["with"]["package_source"], "public_npm")
        self.assertEqual(public["with"]["expected_integrity"], "${{ needs.stage.outputs.tarball_integrity }}")
        self.assertEqual(public["with"]["expected_shasum"], "${{ needs.stage.outputs.tarball_shasum }}")
        proof_body = PROOF.read_text()
        self.assertIn("env -i", proof_body)
        self.assertIn("HOME=", proof_body)
        self.assertIn("XDG_CONFIG_HOME=", proof_body)
        self.assertIn("NPM_CONFIG_CACHE=", proof_body)
        self.assertIn("npm-public-contract.js metadata", proof_body)
        self.assertIn("npm-public-contract.js attestation", proof_body)
        self.assertIn("npm-public-contract.js download", proof_body)
        self.assertIn("https://registry.npmjs.org/universal-agent-plugins/$VERSION", proof_body)
        self.assertIn("https://registry.npmjs.org/-/npm/v1/attestations/universal-agent-plugins@$VERSION", proof_body)
        self.assertEqual(proof_body.count("for delay in 0 10 20 40 80 160"), 3)
        self.assertIn("metadata_ready=false", proof_body)
        self.assertIn('test "$metadata_ready" = true', proof_body)
        self.assertIn("attestations_ready=false", proof_body)
        self.assertIn('test "$attestations_ready" = true', proof_body)
        self.assertIn('npm install --prefix "$npm_root"', proof_body)
        self.assertIn("--save-exact npm@12.0.2", proof_body)
        self.assertIn('npm_cli="$npm_root/node_modules/npm/bin/npm-cli.js"', proof_body)
        self.assertIn('test "$(node "$npm_cli" --version)" = 12.0.2', proof_body)
        self.assertIn("audit_ready=false", proof_body)
        self.assertIn('test "$audit_ready" = true', proof_body)
        self.assertIn('"$UAP_TAG" "$GITHUB_SHA"', proof_body)
        self.assertIn('node "$npm_cli" audit signatures --prefix "$proof_root/audit" --json --include-attestations', proof_body)
        self.assertEqual(proof_body.count('node "$npm_cli"'), 4)
        self.assertIn("npm-public-contract.js audit", proof_body)
        script = (ROOT / "npm/universal-agent-plugins/scripts/platform-proof.js").read_text()
        for command in ('["version"]', '["doctor"]', '["search", "context7"]',
                        '["info", "platform-proof-synthetic"', 'lifecycleCommands(synthetic)'):
            self.assertIn(command, script)
        self.assertIn("isolated_add_info_update_remove", script)

    def test_stage_exports_exact_pack_digests_and_public_proof_consumes_them(self):
        workflow = load(PUBLISH)
        stage = workflow["jobs"]["stage"]
        self.assertEqual(stage["outputs"]["tarball_integrity"], "${{ steps.pack.outputs.tarball_integrity }}")
        self.assertEqual(stage["outputs"]["tarball_shasum"], "${{ steps.pack.outputs.tarball_shasum }}")
        pack = next(step for step in stage["steps"] if step.get("id") == "pack")
        self.assertIn("npm pack", pack["run"])
        self.assertIn("npm-public-contract.js stage-outputs", pack["run"])
        proof = workflow["jobs"]["six-platform-proof"]["with"]
        self.assertEqual(proof["expected_integrity"], "${{ needs.stage.outputs.tarball_integrity }}")
        self.assertEqual(proof["expected_shasum"], "${{ needs.stage.outputs.tarball_shasum }}")

    def test_release_runbook_has_exact_non_destructive_0126_fallback(self):
        body = (ROOT / "docs/AGENTPLUGINS_NPM_RELEASE.md").read_text()
        self.assertIn(
            "npm exec --package=universal-agent-plugins@0.1.26 -- agentplugins doctor", body
        )
        self.assertIn("npm install --global universal-agent-plugins@0.1.26", body)
        self.assertIn("agentplugins doctor", body)
        self.assertRegex(body, r"do not\s+move npm `latest` backward")
        self.assertRegex(body, r"do not overwrite or unpublish")
        self.assertIn("do not change any npm\ndist-tag", body)

    def test_all_third_party_actions_are_commit_pinned(self):
        for path in (PUBLISH, PROOF):
            for action in re.findall(r"uses:\s*([^\s]+)", path.read_text()):
                if action.startswith("./"):
                    continue
                self.assertRegex(action, r"@[0-9a-f]{40}$", f"unpinned action in {path}: {action}")


if __name__ == "__main__":
    unittest.main()
