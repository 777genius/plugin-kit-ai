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
		"environment: npm-agentplugins",
		"id-token: write",
		`package_name="$(node -p 'require("./npm/agentplugins/package.json").name')"`,
		`test "$(node -p 'require("./npm/agentplugins/package.json").bin.agentplugins')" = "bin/agentplugins.js"`,
		`npm view "${package_name}" versions --json`,
		"grep -q 'E404'",
		"unable to prove whether ${package_name} already exists in npm",
		`NPM_PACKAGE: ${{ steps.publish-gate.outputs.package_name }}`,
		`npm view "${NPM_PACKAGE}" dist-tags --json`,
		"npm publish --access public --tag latest --provenance",
		"trusted publishing only supports existing packages",
		".latest == $version",
	} {
		mustContain(t, npmWorkflow, want)
	}
	mustNotContain(t, npmWorkflow, "npm view agentplugins versions --json")
	mustNotContain(t, npmWorkflow, "npm view agentplugins version >/dev/null")
	mustNotContain(t, npmWorkflow, "--tag beta")
	mustNotContain(t, npmWorkflow, "bootstrap_publish")
	mustNotContain(t, npmWorkflow, "NPM_TOKEN")
	mustNotContain(t, npmWorkflow, "NODE_AUTH_TOKEN")
	mustAppearBefore(t, npmWorkflow, "npm publish --access public --tag latest --provenance", "Verify exact published stable lifecycle from a clean project")

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
