package pluginkitairepo_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseSurface_MakefileDocsAndWorkflowsStayAligned(t *testing.T) {
	root := RepoRoot(t)

	makefile := readRepoFile(t, root, "Makefile")
	releaseDoc := readRepoFile(t, root, "docs", "RELEASE.md")
	checklist := readRepoFile(t, root, "docs", "RELEASE_CHECKLIST.md")
	releaseNotes := readRepoFile(t, root, "docs", "RELEASE_NOTES_TEMPLATE.md")
	statusDoc := readRepoFile(t, root, "docs", "STATUS.md")
	ciWorkflow := readRepoFile(t, root, ".github", "workflows", "ci.yml")
	polyglotWorkflow := readRepoFile(t, root, ".github", "workflows", "polyglot-smoke.yml")
	extendedWorkflow := readRepoFile(t, root, ".github", "workflows", "extended.yml")
	liveWorkflow := readRepoFile(t, root, ".github", "workflows", "live.yml")
	npmPublishWorkflow := readRepoFile(t, root, ".github", "workflows", "npm-publish.yml")

	mustContain(t, makefile, "release-gate:\n\t$(MAKE) test-required\n\t$(MAKE) vet\n\t$(MAKE) generated-check")
	mustContain(t, makefile, "release-rehearsal: release-gate\n\t$(MAKE) test-install-compat\n\t$(MAKE) test-polyglot-smoke")

	mustContain(t, releaseDoc, "- `generated-sync`: deterministic generated-artifact drift check used by release gates and rehearsal")
	mustContain(t, releaseDoc, "- `make release-gate`: `test-required -> vet -> generated-check`")
	mustContain(t, releaseDoc, "- `make release-rehearsal`: `release-gate -> test-install-compat -> test-polyglot-smoke`")
	mustContain(t, releaseDoc, "2. run `make release-gate`")
	mustContain(t, releaseDoc, "3. run `make test-install-compat`")
	mustContain(t, releaseDoc, "4. run `make test-polyglot-smoke`")
	mustContain(t, releaseDoc, "- generated-artifact sync result")
	mustContain(t, releaseDoc, "- generated-config/runtime-contract drift result")
	mustContain(t, releaseDoc, "- `required`: blocking on normal PR flow")
	mustContain(t, releaseDoc, "- `polyglot-smoke`: separate deterministic lane required for runtime/ABI/bootstrap-affecting changes and for release rehearsal")
	mustContain(t, releaseDoc, "generated Claude/Codex config canaries")
	mustContain(t, releaseDoc, "the `public-beta` npm wrapper contract")
	mustContain(t, releaseDoc, "npm publish result and optional live npm smoke result")

	mustContain(t, checklist, "- `make release-gate` green")
	mustContain(t, checklist, "- `make release-gate` includes `test-required`, `vet`, and `generated-check`")
	mustContain(t, checklist, "- `make release-rehearsal` may be used as the canonical deterministic local rehearsal shortcut")
	mustContain(t, checklist, "- `make test-install-compat` green")
	mustContain(t, checklist, "- generated-config/runtime-contract drift evidence recorded when changes affect `render`, scaffolded target files, target contracts, or runtime docs")
	mustContain(t, checklist, "- release notes use the same evidence fields as the release playbook")
	mustContain(t, checklist, "- npm publish result recorded when the `plugin-kit-ai` CLI npm channel changed")

	mustContain(t, releaseNotes, "- candidate commit SHA:")
	mustContain(t, releaseNotes, "- required:")
	mustContain(t, releaseNotes, "- install-compat:")
	mustContain(t, releaseNotes, "- polyglot-smoke:")
	mustContain(t, releaseNotes, "- generated-config/runtime-contract drift:")
	mustContain(t, releaseNotes, "- extended:")
	mustContain(t, releaseNotes, "- live:")
	mustContain(t, releaseNotes, "- waivers:")

	mustContain(t, statusDoc, "| Quality gates | done | `required`, `polyglot-smoke`, `extended`, and `live` lanes exist in repo automation. `polyglot-smoke` now covers launcher/ABI checks plus generated Claude/Codex config canaries and rendered runtime-artifact drift protection.")
	mustContain(t, statusDoc, "generated-sync gate")
	mustContain(t, statusDoc, "Release rehearsal now includes the executable-runtime deterministic gate.")

	mustContain(t, ciWorkflow, "name: Required")
	mustContain(t, ciWorkflow, "- name: Run required lane")
	mustContain(t, polyglotWorkflow, "name: Polyglot Smoke")
	mustContain(t, polyglotWorkflow, "name: polyglot-smoke (${{ matrix.os }})")
	mustContain(t, polyglotWorkflow, "- name: Run polyglot-smoke lane")
	mustContain(t, extendedWorkflow, "name: Extended")
	mustContain(t, extendedWorkflow, "name: extended")
	mustContain(t, extendedWorkflow, "- name: Run extended evidence lane")
	mustContain(t, liveWorkflow, "name: Live")
	mustContain(t, liveWorkflow, "name: live")
	mustContain(t, liveWorkflow, "- name: Run live evidence lane")
	mustContain(t, liveWorkflow, "run_npm_install")
	mustContain(t, liveWorkflow, "npm i -g \"plugin-kit-ai@${version}\"")
	mustContain(t, liveWorkflow, "npm list -g plugin-kit-ai --depth=0")
	mustContain(t, liveWorkflow, "npm exec --yes --package \"plugin-kit-ai@${version}\" -- plugin-kit-ai version")
	mustContain(t, npmPublishWorkflow, "name: NPM Publish")
	mustContain(t, npmPublishWorkflow, "types: [published]")
	mustContain(t, npmPublishWorkflow, "NPM_TOKEN")
	mustContain(t, npmPublishWorkflow, "checksums.txt")
	mustContain(t, npmPublishWorkflow, "npm publish --access public")
}

func readRepoFile(t *testing.T, root string, parts ...string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(append([]string{root}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
