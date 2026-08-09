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
	npmReleaseIdentityJob := yamlJob(t, npmWorkflow, "release-identity")
	npmPlatformProofJob := yamlJob(t, npmWorkflow, "platform-proof")
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
	mustContain(t, releaseWorkflow, "release ${TAG} already exists; refusing to overwrite immutable assets")
	mustContain(t, releaseWorkflow, "uses: ./.github/workflows/agentplugins-platform-proof.yml")
	mustContain(t, releaseWorkflow, "expected_commit: ${{ needs.validate.outputs.commit }}")
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
	mustContain(t, npmReleaseIdentityJob, `test "${commit}" = "$(git rev-list -n 1 "${TAG}")"`)
	mustContain(t, npmPlatformProofJob, "needs: release-identity")
	mustContain(t, npmPlatformProofJob, "if: ${{ !inputs.verify_only }}")
	mustContain(t, npmPlatformProofJob, "allow_legacy_manifest: false")
	for _, want := range []string{
		"needs: [release-identity, platform-proof]",
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
		`run_agentplugins add "${synthetic}" --target cursor --yes --format json > add.json`,
		`run_agentplugins add "${synthetic}" --target cursor --yes --activation-complete --auth-complete --format json > complete.json`,
		`.data.result.activation.authentication == "not_checked"`,
		`.data.result.activation.activation_attested == true`,
		`.data.result.activation.authentication_attested == true`,
		`.data.result.no_change == true and .data.result.mutated == false`,
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
	mustAppearBefore(t, npmVerifyJob, "Resolve immutable verification target", `npm view --prefer-online "${NPM_PACKAGE}@${version}" version`)
	mustAppearBefore(t, npmVerifyJob, `npm view --prefer-online "${NPM_PACKAGE}@${version}" version`, `npm install --ignore-scripts --save-exact "${NPM_PACKAGE}@${version}"`)
	mustAppearBefore(t, npmVerifyJob, `npm view --prefer-online "${NPM_PACKAGE}@latest" version`, `npm install --ignore-scripts --save-exact "${NPM_PACKAGE}@${version}"`)
	mustAppearBefore(t, npmVerifyJob, `test "${available}" = true`, `npm install --ignore-scripts --save-exact "${NPM_PACKAGE}@${version}"`)
	mustAppearBefore(t, npmVerifyJob, `npm install --ignore-scripts --save-exact "${NPM_PACKAGE}@${version}"`, "npm audit signatures --json --include-attestations")
	mustAppearBefore(t, npmVerifyJob, "npm audit signatures --json --include-attestations", "run_agentplugins version")
	mustAppearBefore(t, npmVerifyJob, `run_agentplugins add "${synthetic}" --target cursor --yes --format json > add.json`, `run_agentplugins add "${synthetic}" --target cursor --yes --activation-complete --auth-complete --format json > complete.json`)
	mustAppearBefore(t, npmVerifyJob, `run_agentplugins add "${synthetic}" --target cursor --yes --activation-complete --auth-complete --format json > complete.json`, `run_agentplugins update registry-proof-synthetic --target cursor --yes --format json > update.json`)

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
	} {
		mustContain(t, platformWorkflow, want)
	}
	mustContain(t, npmWorkflow, "uses: ./.github/workflows/agentplugins-platform-proof.yml")
	mustContain(t, npmWorkflow, "test \"${{ needs.platform-proof.outputs.gate_eligible }}\" = \"true\"")
	mustContain(t, npmWorkflow, "npm publish --access public --tag latest --provenance \"${TARBALL}\"")
	mustNotContain(t, npmWorkflow, "add context7")

	mustContain(t, removedBoundaryScript, "if command -v rg")
	mustContain(t, removedBoundaryScript, "git grep --untracked --exclude-standard")
	mustContain(t, removedBoundaryScript, "removed-contract scan failed")

	for _, want := range []string{
		"attests all six binaries, `checksums.txt`, and",
		"`release-manifest.json` before creating the public release",
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
