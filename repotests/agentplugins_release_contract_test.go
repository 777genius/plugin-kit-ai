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
	mustContain(t, releaseWorkflow, "release ${TAG} already exists; refusing to overwrite immutable assets")

	for _, want := range []string{
		"npm view agentplugins versions --json",
		"grep -q 'E404'",
		"unable to prove whether agentplugins already exists in npm",
		"npm view agentplugins dist-tags --json",
		"latest_before=${latest_before}",
		"npm publish --access public --tag beta --provenance",
		".beta == $version",
		"= \"${LATEST_BEFORE}\"",
	} {
		mustContain(t, npmWorkflow, want)
	}
	mustNotContain(t, npmWorkflow, "npm view agentplugins version >/dev/null")
	mustAppearBefore(t, npmWorkflow, "npm publish --access public --tag beta --provenance", "Verify exact published beta lifecycle from a clean project")

	for _, want := range []string{
		"attests all six binaries, `checksums.txt`, and",
		"`release-manifest.json` before creating the public prerelease",
		"Only an npm `E404` is accepted as",
		"proof that the package does not exist",
		"`beta` dist-tag resolves to the exact published version",
		"dist-tag is unchanged",
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
