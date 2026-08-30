package pluginkitairepo_test

import (
	"strings"
	"testing"
)

func TestAgentpluginsReleaseContractsStayFailClosed(t *testing.T) {
	root := RepoRoot(t)
	makefile := readRepoFile(t, root, "Makefile")
	releaseWorkflow := readRepoFile(t, root, ".github", "workflows", "agentplugins-release.yml")
	npmWorkflow := readRepoFile(t, root, ".github", "workflows", "agentplugins-npm-publish.yml")
	platformWorkflow := readRepoFile(t, root, ".github", "workflows", "agentplugins-platform-proof.yml")
	platformPrepareJob := yamlJob(t, platformWorkflow, "prepare")
	platformNativeJob := yamlJob(t, platformWorkflow, "native-runtime")
	platformCompleteJob := yamlJob(t, platformWorkflow, "proof-complete")
	releaseDraftJob := yamlJob(t, releaseWorkflow, "stage-draft")
	releaseProofJob := yamlJob(t, releaseWorkflow, "platform-proof")
	releasePromoteJob := yamlJob(t, releaseWorkflow, "promote-release")
	npmReleaseIdentityJob := yamlJob(t, npmWorkflow, "release-identity")
	npmPrepublishConformanceJob := yamlJob(t, npmWorkflow, "prepublish-conformance")
	npmPublishJob := yamlJob(t, npmWorkflow, "publish")
	npmVerifyJob := yamlJob(t, npmWorkflow, "verify")
	removedBoundaryScript := readRepoFile(t, root, "scripts", "check-removed-contract-boundary.sh")
	runbook := readRepoFile(t, root, "docs", "agentplugins-release.md")

	for _, want := range []string{
		"go test -count=1 -timeout=$(REQUIRED_TEST_TIMEOUT) ./...",
		"go test -count=1 -timeout=$(REQUIRED_TEST_TIMEOUT) ./cli/plugin-kit-ai/...",
		"go test -count=1 -timeout=$(REQUIRED_TEST_TIMEOUT) ./install/integrationctl/...",
		"go test -count=1 -timeout=$(REQUIRED_TEST_TIMEOUT) ./install/plugininstall/...",
		"go test -count=1 -timeout=$(REQUIRED_TEST_TIMEOUT) ./sdk/...",
		"cd npm/agentplugins && npm test && npm pack --dry-run --ignore-scripts",
		"cd install/integrationctl && go vet ./...",
	} {
		mustContain(t, makefile, want)
	}

	mustContain(t, releaseWorkflow, "actions/attest@")
	mustContain(t, releaseWorkflow, "gh release create")
	mustAppearBefore(t, releaseWorkflow, "actions/attest@", "gh release create")
	mustContain(t, releaseWorkflow, "^agentplugins-v[0-9]+\\.[0-9]+\\.[0-9]+$")
	mustNotContain(t, releaseWorkflow, "--prerelease")
	mustContain(t, releaseDraftJob, "exact resumable draft; refusing to overwrite immutable assets")
	mustContain(t, releaseDraftJob, "--draft")
	mustContain(t, releaseDraftJob, "existing draft asset differs from the frozen build")
	mustContain(t, releaseDraftJob, "gh api graphql")
	mustContain(t, releaseDraftJob, ".data.repository.release")
	mustContain(t, releaseDraftJob, "Share frozen draft assets with the read-only proof workflow")
	mustContain(t, releaseDraftJob, "assets_artifact=agentplugins-draft-assets-${FROZEN_COMMIT}")
	mustNotContain(t, releaseDraftJob, "target_commitish")
	mustContain(t, releaseProofJob, "needs: [validate, stage-draft]")
	mustContain(t, releaseProofJob, "contents: read")
	mustContain(t, releaseProofJob, "require_draft: true")
	mustContain(t, releaseProofJob, "expected_asset_set_digest: ${{ needs.stage-draft.outputs.asset_set_digest }}")
	mustContain(t, releaseProofJob, "release_assets_artifact: ${{ needs.stage-draft.outputs.assets_artifact }}")
	mustContain(t, releasePromoteJob, "needs: [validate, stage-draft, platform-proof]")
	mustContain(t, releasePromoteJob, "EXPECTED_ASSET_SET_DIGEST: ${{ needs.stage-draft.outputs.asset_set_digest }}")
	mustContain(t, releasePromoteJob, "gh release edit \"${TAG}\"")
	mustContain(t, releasePromoteJob, "--draft=false")
	mustContain(t, releasePromoteJob, `git fetch --force origin "refs/tags/${TAG}:refs/tags/${TAG}"`)
	mustContain(t, releasePromoteJob, `git rev-list -n 1 "refs/tags/${TAG}"`)
	mustContain(t, releasePromoteJob, "gh api graphql")
	mustNotContain(t, releasePromoteJob, "target_commitish")
	mustAppearBefore(t, releaseWorkflow, "gh release create", "gh release edit")
	mustAppearBefore(t, releaseWorkflow, "uses: ./.github/workflows/agentplugins-platform-proof.yml", "gh release edit")
	mustContain(t, releaseWorkflow, "uses: ./.github/workflows/agentplugins-platform-proof.yml")
	mustContain(t, releaseWorkflow, "expected_commit: ${{ needs.validate.outputs.commit }}")
	mustContain(t, releaseWorkflow, `test "${WORKFLOW_REF}" = "refs/heads/main"`)
	mustContain(t, releaseWorkflow, "release workflow source does not match the exact tagged main commit")
	mustContain(t, releaseWorkflow, "commits/${COMMIT}/pulls")
	mustContain(t, releaseWorkflow, "release commit must come from a merged pull request into main")
	mustContain(t, releaseWorkflow, "check-runs?filter=latest&per_page=100")
	for _, name := range []string{
		"dependency-review",
		"test",
		"docs-check",
		"polyglot-smoke (ubuntu-latest)",
		"polyglot-smoke (windows-latest)",
		"analyze (go)",
		"analyze (javascript-typescript)",
		"govulncheck (root)",
		"govulncheck (cli)",
		"govulncheck (integrationctl)",
		"govulncheck (plugininstall)",
		"govulncheck (sdk)",
	} {
		mustContain(t, releaseWorkflow, name)
	}

	mustContain(t, npmReleaseIdentityJob, "if: ${{ !inputs.verify_only }}")
	mustContain(t, npmReleaseIdentityJob, `test "${commit}" = "$(git rev-parse refs/remotes/origin/main)"`)
	mustContain(t, npmReleaseIdentityJob, `test "${WORKFLOW_REF}" = "refs/heads/main"`)
	mustContain(t, npmReleaseIdentityJob, "npm publish workflow source does not match the exact tagged main commit")
	mustContain(t, npmReleaseIdentityJob, `git fetch --force origin main:refs/remotes/origin/main "refs/tags/${TAG}:refs/tags/${TAG}"`)
	mustContain(t, npmReleaseIdentityJob, `test "${commit}" = "$(git rev-list -n 1 "refs/tags/${TAG}")"`)
	mustContain(t, npmPrepublishConformanceJob, "needs: release-identity")
	mustContain(t, npmPrepublishConformanceJob, "if: ${{ !inputs.verify_only }}")
	mustContain(t, npmPrepublishConformanceJob, "allow_legacy_manifest: false")
	mustContain(t, npmPrepublishConformanceJob, "contents: read")
	for _, want := range []string{
		"needs: [release-identity, prepublish-conformance]",
		"if: ${{ !inputs.verify_only }}",
		"environment: npm-agentplugins",
		"id-token: write",
		`package_name="$(node -p 'require("./npm/agentplugins/package.json").name')"`,
		`test "$(node -p 'require("./npm/agentplugins/package.json").bin.agentplugins')" = "bin/agentplugins.js"`,
		`npm view "${package_name}" versions --json`,
		"grep -q 'E404'",
		"unable to prove whether ${package_name} already exists in npm",
		"npm publish --access public --tag latest --provenance",
		"trusted publishing only supports existing packages",
	} {
		mustContain(t, npmPublishJob, want)
	}
	for _, want := range []string{
		"needs: publish",
		"always() && !cancelled() && (inputs.verify_only || needs.publish.result == 'success')",
		`ref: ${{ inputs.tag }}`,
		"Resolve immutable verification target",
		`NPM_PACKAGE: ${{ steps.verify-target.outputs.package_name }}`,
		`npm view --prefer-online "${NPM_PACKAGE}@${version}" version`,
		`npm view --prefer-online "${NPM_PACKAGE}@latest" version`,
		`[[ "${latest_version}" = "${version}" ]]`,
		"max_attempts=30",
		`npm install --ignore-scripts --save-exact "${NPM_PACKAGE}@${version}"`,
		"npm audit signatures --json --include-attestations",
		`.attestations.provenance.predicateType == "https://slsa.dev/provenance/v1"`,
		`lifecycle_targets="codex,cursor"`,
		`run_agentplugins add "${synthetic}" --target "${lifecycle_targets}" --format json > add.json`,
		`run_agentplugins add "${synthetic}" --target "${lifecycle_targets}" --activation-complete --auth-complete --format json > complete.json`,
		`.data.batch == true and .data.succeeded == 2 and .data.failed == 0`,
		`([.data.targets[].target] == ["codex", "cursor"])`,
		`.output.operation_id == $operation_id`,
		`.output.result.installation_id == $installation_id`,
		`.output.result.activation.activation_attested == true`,
		`.output.result.activation.authentication_attested == true`,
		`.output.result.no_change == true and .output.result.mutated == false`,
		`.data.status == "data_retained"`,
		`.data.plugin_data_preserved == true`,
		`(.data.retained_data | length) > 0`,
		`(keys | sort) == ["data_receipt_id", "physical_backend_id", "scope", "state"]`,
		`contains("agentplugins remove " + $installation_id + " --purge-data")`,
		`select(type == "string" and startswith("/"))`,
	} {
		mustContain(t, npmVerifyJob, want)
	}
	mustContain(t, npmWorkflow, "verify_only:")
	mustContain(t, npmWorkflow, "Verify an existing public version without publishing")
	for _, unwanted := range []string{
		"npm view agentplugins versions --json",
		"npm view agentplugins version >/dev/null",
		"--tag beta",
		"bootstrap_publish",
		"NPM_TOKEN",
		"NODE_AUTH_TOKEN",
	} {
		mustNotContain(t, npmWorkflow, unwanted)
	}
	mustNotContain(t, npmPublishJob, "Verify exact published stable lifecycle from a clean project")
	mustNotContain(t, npmVerifyJob, "npm publish --access public --tag latest --provenance")
	mustNotContain(t, npmVerifyJob, "environment: npm-agentplugins")
	mustNotContain(t, npmVerifyJob, "id-token: write")
	mustNotContain(t, npmVerifyJob, "platform-proof")
	mustNotContain(t, npmVerifyJob, "--target cursor --yes")
	mustAppearBefore(t, npmVerifyJob, "Resolve immutable verification target", `npm view --prefer-online "${NPM_PACKAGE}@${version}" version`)
	mustAppearBefore(t, npmVerifyJob, `npm view --prefer-online "${NPM_PACKAGE}@${version}" version`, `npm install --ignore-scripts --save-exact "${NPM_PACKAGE}@${version}"`)
	mustAppearBefore(t, npmVerifyJob, `npm view --prefer-online "${NPM_PACKAGE}@latest" version`, `npm install --ignore-scripts --save-exact "${NPM_PACKAGE}@${version}"`)
	mustAppearBefore(t, npmVerifyJob, `test "${available}" = true`, `npm install --ignore-scripts --save-exact "${NPM_PACKAGE}@${version}"`)
	mustAppearBefore(t, npmVerifyJob, `npm install --ignore-scripts --save-exact "${NPM_PACKAGE}@${version}"`, "npm audit signatures --json --include-attestations")
	mustAppearBefore(t, npmVerifyJob, "npm audit signatures --json --include-attestations", "run_agentplugins version")
	mustNotContain(t, npmVerifyJob, `.data.result.`)
	mustAppearBefore(t, npmVerifyJob, `run_agentplugins add "${synthetic}" --target "${lifecycle_targets}" --format json > add.json`, `run_agentplugins add "${synthetic}" --target "${lifecycle_targets}" --activation-complete --auth-complete --format json > complete.json`)
	mustAppearBefore(t, npmVerifyJob, `run_agentplugins add "${synthetic}" --target "${lifecycle_targets}" --activation-complete --auth-complete --format json > complete.json`, `run_agentplugins update registry-proof-synthetic --target "${lifecycle_targets}" --format json > update.json`)
	mustAppearBefore(t, npmVerifyJob, `run_agentplugins update registry-proof-synthetic --target "${lifecycle_targets}" --format json > update.json`, `run_agentplugins remove registry-proof-synthetic --target "${lifecycle_targets}" --external-uninstalled --format json > remove.json`)

	for _, want := range []string{
		"workflow_call:",
		"allow_legacy_manifest:",
		"Audit a schema-v1 historical release; never eligible as an npm publish gate",
		"native runtime E2E (${{ matrix.target }})",
		"target: darwin-amd64",
		"target: darwin-arm64",
		"target: linux-amd64",
		"target: linux-arm64",
		"target: windows-amd64",
		"target: windows-arm64",
		"macos-15-intel",
		"ubuntu-24.04-arm",
		"windows-11-arm",
		"platform-proof.js",
		"verified-release.json",
		"Aggregate all six native platform proofs",
		"draft proof requires caller-staged release assets",
		"Download caller-staged draft assets",
		"bootstrap_mode:",
		"local_frozen_asset",
		"public_release_download",
		"platform proof workflow source does not match the frozen release commit",
		"historical audit must be dispatched with --ref ${TAG}",
		".isDraft == false and .isPrerelease == false and .tagName == $tag",
		`git -C release-source fetch --force origin "refs/tags/${TAG}:refs/tags/${TAG}"`,
		`git -C release-source rev-list -n 1 "refs/tags/${TAG}"`,
	} {
		mustContain(t, platformWorkflow, want)
	}
	for _, want := range []string{
		"contents: read",
		"attestations: read",
		"caller-staged assets are only valid for draft proof",
		`[[ ! "${EXPECTED_COMMIT}" =~ ^[0-9a-f]{40}$ ]]`,
		`if [[ "${commit}" != "${EXPECTED_COMMIT}" ]]`,
		"release tag commit does not match the caller's frozen commit",
		"ref: ${{ inputs.expected_commit }}",
		"agentplugins-proof-bundle",
		"release-assets",
		"git -C release-source diff --quiet HEAD --",
		"docs/AGENTPLUGINS_CLIENT_E2E.md",
		"docs/evidence/agentplugins-client-e2e-2026-08-30.json",
		`--evidence-root "${GITHUB_WORKSPACE}/release-source/docs"`,
	} {
		mustContain(t, platformPrepareJob, want)
	}
	mustAppearBefore(t, platformPrepareJob, "git -C release-source diff --quiet HEAD --", "stage-release.js")
	mustAppearBefore(t, platformPrepareJob, "stage-release.js", "npm test && npm pack --dry-run --ignore-scripts")
	for _, want := range []string{
		"ref: ${{ inputs.expected_commit }}",
		`test "$(git rev-parse HEAD)" = "${{ inputs.expected_commit }}"`,
		"needs.prepare.outputs.bootstrap_mode",
		"release_assets=\"-\"",
		"npm/agentplugins/scripts/platform-proof.js",
	} {
		mustContain(t, platformNativeJob, want)
	}
	for _, want := range []string{
		"Download all native platform proofs",
		"expected exactly six machine-readable platform proofs",
		"expected exactly one machine-readable proof for ${target}",
		".schema_version == 1",
		".release_version == $version",
		".proofs.isolated_add_update_remove == $lifecycle",
		`.bootstrap_source == $bootstrap_mode`,
		`.proofs.local_frozen_asset_bootstrap == ($bootstrap_mode == "local_frozen_asset")`,
		`.proofs.anonymous_public_release_download == ($bootstrap_mode == "public_release_download")`,
		".proofs.warm_cache_without_proof_source == true",
	} {
		mustContain(t, platformCompleteJob, want)
	}
	mustNotContain(t, platformWorkflow, "target_commitish")
	mustNotContain(t, platformWorkflow, `releases/tags/${TAG}`)
	mustContain(t, npmWorkflow, "uses: ./.github/workflows/agentplugins-platform-proof.yml")
	mustContain(t, npmWorkflow, "test \"${{ needs.prepublish-conformance.outputs.gate_eligible }}\" = \"true\"")
	mustContain(t, readRepoFile(t, root, "npm", "agentplugins", "scripts", "platform-proof.js"), "assertPublicJSONPathFree")
	mustAppearBefore(t, npmWorkflow, "prepublish-conformance:", "npm publish --access public --tag latest --provenance")
	mustContain(t, npmWorkflow, "npm publish --access public --tag latest --provenance \"${TARBALL}\"")
	mustNotContain(t, npmWorkflow, "add context7")

	mustContain(t, removedBoundaryScript, "if command -v rg")
	mustContain(t, removedBoundaryScript, "git grep --untracked --exclude-standard")
	mustContain(t, removedBoundaryScript, "removed-contract scan failed")

	for _, want := range []string{
		"attests all six binaries, `checksums.txt`, and",
		"`release-manifest.json` before creating the non-public draft",
		"promotes that exact draft only after all six native platform proofs succeed",
		"requires a merged pull request into",
		"Repository settings are not treated as the release proof",
		"short-lived bootstrap",
		"disallow bypass-2FA tokens",
		"Do not add a bootstrap token back",
		"publish through",
		"GitHub OIDC with provenance",
		"`latest` dist-tag resolves to the exact published version",
		"returned to `false` immediately after the publish",
		"dispatched with `verify_only=true`",
		"historical tag after publication",
		"skips release identity",
		"schema-v2 six-platform proof",
		"not require the tag to point to current `main`",
		"public registry, provenance, and isolated",
		"same-run frozen",
		"normal anonymous GitHub release download",
	} {
		mustContain(t, runbook, want)
	}
}

func TestAgentpluginsReadmesUseUnversionedNpxExamples(t *testing.T) {
	root := RepoRoot(t)
	for _, path := range [][]string{
		{"README.md"},
		{"npm", "agentplugins", "README.md"},
	} {
		readme := readRepoFile(t, root, path...)
		mustContain(t, readme, "npx universal-agent-plugins add context7")
		mustNotContain(t, readme, "npx universal-agent-plugins@")
	}
}

func mustAppearBefore(t *testing.T, text, first, second string) {
	t.Helper()
	firstIndex := strings.Index(text, first)
	secondIndex := strings.Index(text, second)
	if firstIndex < 0 || secondIndex < 0 || firstIndex >= secondIndex {
		t.Fatalf("expected %q to appear before %q", first, second)
	}
}

func yamlJob(t *testing.T, workflow, name string) string {
	t.Helper()
	marker := "\n  " + name + ":\n"
	start := strings.Index(workflow, marker)
	if start < 0 {
		t.Fatalf("missing workflow job %q", name)
	}

	lines := strings.SplitAfter(workflow[start+1:], "\n")
	var job strings.Builder
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if index > 0 && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":") {
			break
		}
		job.WriteString(line)
	}
	return job.String()
}
