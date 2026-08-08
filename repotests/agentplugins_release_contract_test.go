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

	for _, want := range []string{
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
	mustAppearBefore(t, npmVerifyJob, "Resolve immutable verification target", `npm view --prefer-online "${NPM_PACKAGE}@${version}" version`)
	mustAppearBefore(t, npmVerifyJob, `npm view --prefer-online "${NPM_PACKAGE}@${version}" version`, `npm install --ignore-scripts --save-exact "${NPM_PACKAGE}@${version}"`)
	mustAppearBefore(t, npmVerifyJob, `npm view --prefer-online "${NPM_PACKAGE}@latest" version`, `npm install --ignore-scripts --save-exact "${NPM_PACKAGE}@${version}"`)
	mustAppearBefore(t, npmVerifyJob, `test "${available}" = true`, `npm install --ignore-scripts --save-exact "${NPM_PACKAGE}@${version}"`)
	mustAppearBefore(t, npmVerifyJob, `npm install --ignore-scripts --save-exact "${NPM_PACKAGE}@${version}"`, "npm audit signatures --json --include-attestations")
	mustAppearBefore(t, npmVerifyJob, "npm audit signatures --json --include-attestations", "run_agentplugins version")

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
	} {
		mustContain(t, runbook, want)
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
