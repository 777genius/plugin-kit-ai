package pluginkitairepo_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractClarity_RuntimeMetadataAndDocsStayAligned(t *testing.T) {
	root := RepoRoot(t)
	pluginKitAIBin := buildPluginKitAI(t)

	matrixPath := filepath.Join(root, "docs", "generated", "support_matrix.md")
	matrixBody, err := os.ReadFile(matrixPath)
	if err != nil {
		t.Fatal(err)
	}
	matrix := string(matrixBody)
	mustContain(t, matrix, "| claude | Stop | runtime_supported | stable | production-ready | true |")
	mustContain(t, matrix, "| claude | SessionStart | runtime_supported | beta | runtime-supported but not stable | false |")
	mustContain(t, matrix, "| codex | Notify | runtime_supported | stable | production-ready | true |")
	targetMatrixBody, err := os.ReadFile(filepath.Join(root, "docs", "generated", "target_support_matrix.md"))
	if err != nil {
		t.Fatal(err)
	}
	targetMatrix := string(targetMatrixBody)
	mustContain(t, targetMatrix, "| claude | packaged_runtime | hook_runtime | required | plugin | marketplace or local plugin install |")
	mustContain(t, targetMatrix, "| codex-package | packaged_runtime | plugin_package | ignored | plugin | plugin directory or marketplace cache |")
	mustContain(t, targetMatrix, "| codex-runtime | packaged_runtime | local_runtime_integration | required | plugin | repo-local config wiring |")
	mustContain(t, targetMatrix, "| gemini | extension_package | mcp_extension | ignored | extension | copy install | link | restart required | ~/.gemini/extensions/<name> | packaging-only target |")
	mustContain(t, targetMatrix, "| opencode | code_plugin | workspace_config_lane | ignored | workspace | workspace config file | config authoring workspace | config reload or restart | opencode.json | packaging-only target | workspace-config lane with first-class npm plugin refs, MCP, skills, commands, agents, themes, JSON/JSONC native import, explicit opt-in user-scope import, and config passthrough; local JS/TS plugin code stays for the later code-plugin wave |")
	mustContain(t, targetMatrix, "agent_config=passthrough_only")
	mustContain(t, targetMatrix, "permission_config=passthrough_only")
	mustContain(t, targetMatrix, "instructions_config=passthrough_only")
	mustContain(t, targetMatrix, "tools_config=passthrough_only")
	mustContain(t, targetMatrix, "commands=stable")
	mustContain(t, targetMatrix, "agents=stable")
	mustContain(t, targetMatrix, "themes=stable")
	mustContain(t, targetMatrix, "modes=unsupported")

	cmd := exec.Command(pluginKitAIBin, "capabilities", "--mode", "runtime", "--format", "json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("capabilities json: %v\n%s", err, out)
	}
	var entries []map[string]any
	if err := json.Unmarshal(out, &entries); err != nil {
		t.Fatalf("parse capabilities json: %v\n%s", err, out)
	}
	byKey := map[string]map[string]any{}
	for _, entry := range entries {
		key := entry["platform"].(string) + "/" + entry["event"].(string)
		byKey[key] = entry
	}
	assertCapabilityContract(t, byKey, "claude/Stop", "stable", "production-ready")
	assertCapabilityContract(t, byKey, "claude/PreToolUse", "stable", "production-ready")
	assertCapabilityContract(t, byKey, "claude/UserPromptSubmit", "stable", "production-ready")
	assertCapabilityContract(t, byKey, "codex/Notify", "stable", "production-ready")
	assertCapabilityContract(t, byKey, "claude/SessionStart", "beta", "runtime-supported but not stable")

	rootReadme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	cliReadme, err := os.ReadFile(filepath.Join(root, "cli", "plugin-kit-ai", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	pluginsExamplesReadme, err := os.ReadFile(filepath.Join(root, "examples", "plugins", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	supportDoc, err := os.ReadFile(filepath.Join(root, "docs", "SUPPORT.md"))
	if err != nil {
		t.Fatal(err)
	}
	statusDoc, err := os.ReadFile(filepath.Join(root, "docs", "STATUS.md"))
	if err != nil {
		t.Fatal(err)
	}
	productionDoc, err := os.ReadFile(filepath.Join(root, "docs", "PRODUCTION.md"))
	if err != nil {
		t.Fatal(err)
	}
	interpretedPromotionDoc, err := os.ReadFile(filepath.Join(root, "docs", "INTERPRETED_STABLE_SUBSET_AUDIT.md"))
	if err != nil {
		t.Fatal(err)
	}
	hardeningDoc, err := os.ReadFile(filepath.Join(root, "docs", "V1_0_X_HARDENING.md"))
	if err != nil {
		t.Fatal(err)
	}
	releaseDoc, err := os.ReadFile(filepath.Join(root, "docs", "RELEASE.md"))
	if err != nil {
		t.Fatal(err)
	}
	releaseChecklist, err := os.ReadFile(filepath.Join(root, "docs", "RELEASE_CHECKLIST.md"))
	if err != nil {
		t.Fatal(err)
	}
	releaseNotesTemplate, err := os.ReadFile(filepath.Join(root, "docs", "RELEASE_NOTES_TEMPLATE.md"))
	if err != nil {
		t.Fatal(err)
	}
	rehearsalTemplate, err := os.ReadFile(filepath.Join(root, "docs", "REHEARSAL_TEMPLATE.md"))
	if err != nil {
		t.Fatal(err)
	}
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	polyglotWorkflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "polyglot-smoke.yml"))
	if err != nil {
		t.Fatal(err)
	}

	mustContain(t, string(rootReadme), "full Gemini CLI extension packaging lane through `render|import|validate`")
	mustContain(t, string(rootReadme), "### Fast Local Plugin")
	mustContain(t, string(rootReadme), "### Production-Ready Plugin Repo")
	mustContain(t, string(rootReadme), "### Already Have Native Config")
	mustContain(t, string(rootReadme), "| local notify/runtime plugin in your repo | `codex-runtime` |")
	mustContain(t, string(rootReadme), "Reference repos: [examples/local/README.md](examples/local/README.md)")
	mustContain(t, string(rootReadme), "`plugin-kit-ai capabilities` now defaults to target/package introspection")
	mustContain(t, string(rootReadme), "repo-local local-runtime authoring for `python` and `node` on `codex-runtime` and `claude`, including `doctor`, `bootstrap`, `validate --strict`, and `export`")
	mustContain(t, string(rootReadme), "`bundle install` for local exported Python/Node bundles")
	mustContain(t, string(rootReadme), "`bundle fetch` for remote exported Python/Node bundles")
	mustContain(t, string(rootReadme), "`bundle publish` for GitHub Releases handoff of exported Python/Node bundles")
	mustContain(t, string(rootReadme), "| `python` | stable local-runtime subset | repo-local executable ABI on `codex-runtime` and `claude` | lockfile-first manager detection; `venv`/`requirements`/`uv` use repo-local `.venv`, `poetry`/`pipenv` can use manager-owned envs |")
	mustContain(t, string(rootReadme), "./bin/plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript")
	mustContain(t, string(rootReadme), "./bin/plugin-kit-ai doctor ./my-plugin")
	mustContain(t, string(rootReadme), "./bin/plugin-kit-ai bootstrap ./my-plugin")
	mustContain(t, string(rootReadme), "./bin/plugin-kit-ai bundle publish ./my-plugin --platform codex-runtime --repo owner/repo --tag v1")
	mustContain(t, string(rootReadme), "./bin/plugin-kit-ai bundle install ./bundle.tar.gz --dest ./plugin-copy")
	mustContain(t, string(rootReadme), "./bin/plugin-kit-ai bundle fetch --url https://example.com/my-plugin_codex-runtime_python_bundle.tar.gz --dest ./handoff-plugin")
	mustContain(t, string(rootReadme), "./bin/plugin-kit-ai import ./native-plugin --from codex-native")
	mustContain(t, string(rootReadme), "| `node` | stable local-runtime subset | repo-local executable ABI on `codex-runtime` and `claude` | system Node.js `20+`; JavaScript by default, TypeScript via `--runtime node --typescript` |")
	mustContain(t, string(rootReadme), "| `shell` | public-beta | repo-local executable ABI | POSIX shell on Unix, `bash` required on Windows |")
	mustContain(t, string(rootReadme), "Shell remains `public-beta` and stays outside that stable local-runtime subset.")
	mustContain(t, string(rootReadme), "Generated Claude/Codex package-runtime config shapes are part of the repo-owned contract surface")
	mustContain(t, string(rootReadme), "`validate --strict` is the canonical CI-grade readiness gate")
	mustContain(t, string(cliReadme), "## Fast Local Plugin")
	mustContain(t, string(cliReadme), "## Production-Ready Plugin Repo")
	mustContain(t, string(cliReadme), "## Already Have Native Config")
	mustContain(t, string(cliReadme), "| local notify/runtime plugin in your repo | `codex-runtime` |")
	mustContain(t, string(cliReadme), "Reference repos: [../../examples/local/README.md](../../examples/local/README.md)")
	mustContain(t, string(cliReadme), "Gemini is a `packaging-only Gemini CLI extension target` in this CLI surface, not a production-ready runtime target")
	mustContain(t, string(cliReadme), "`plugin-kit-ai capabilities` defaults to the target/package view")
	mustContain(t, string(cliReadme), "Builds the **`plugin-kit-ai`** binary: `init`, `bootstrap`, `doctor`, `export`, `bundle install`, `bundle fetch`, `bundle publish`, `render`, `import`, `inspect`, `normalize`, `validate`, `capabilities`, `install`, `version`")
	mustContain(t, string(cliReadme), "`plugin-kit-ai bootstrap` is the stable repo-local first-run helper for `python` and `node` launcher-based projects on `codex-runtime` and `claude`")
	mustContain(t, string(cliReadme), "`plugin-kit-ai doctor` is the stable read-only readiness check for `python` and `node` launcher-based projects on `codex-runtime` and `claude`")
	mustContain(t, string(cliReadme), "`plugin-kit-ai export` is the stable portable handoff surface for `python` and `node` launcher-based projects on `codex-runtime` and `claude`")
	mustContain(t, string(cliReadme), "`plugin-kit-ai bundle install` is the stable local bundle installer for exported Python/Node handoff archives")
	mustContain(t, string(cliReadme), "`plugin-kit-ai bundle fetch` is the `public-beta` remote bundle fetch/install companion for exported Python/Node handoff archives")
	mustContain(t, string(cliReadme), "`plugin-kit-ai bundle publish` is the `public-beta` GitHub Releases publish companion for exported Python/Node handoff archives")
	mustContain(t, string(cliReadme), "creates a published release by default")
	mustContain(t, string(cliReadme), "`--draft` as an opt-in safety mode")
	mustContain(t, string(cliReadme), "URL mode verifies `--sha256` or `<url>.sha256`, GitHub Releases mode prefers `checksums.txt` and falls back to `<asset>.sha256`")
	mustContain(t, string(cliReadme), "./bin/plugin-kit-ai import ./native-plugin --from codex-native")
	mustContain(t, string(cliReadme), "| `node` | stable local-runtime subset | repo-local on `codex-runtime` and `claude` | lockfile-first manager detection (`bun`, `pnpm`, `yarn`, `npm`); JavaScript by default, TypeScript via `--runtime node --typescript` |")
	mustContain(t, string(cliReadme), "| `shell` | public-beta | repo-local only | POSIX shell on Unix, `bash` in `PATH` on Windows |")
	mustContain(t, string(cliReadme), "Generated Claude/Codex package-runtime config shapes are part of the repo-owned contract surface")
	mustContain(t, string(pluginsExamplesReadme), "# Production Plugin Examples")
	mustContain(t, string(pluginsExamplesReadme), "For repo-local Python/Node entrance examples, see [../local/README.md](../local/README.md).")
	mustContain(t, string(pluginsExamplesReadme), "Executable `python` and `node` plugins are now the stable repo-local local-runtime subset")
	mustContain(t, string(supportDoc), "Gemini: full Gemini CLI extension packaging lane through `plugin-kit-ai render|import|validate` and local `extensions link|config|disable|enable`; not a production-ready runtime target")
	mustContain(t, string(supportDoc), "Codex runtime: production-ready within the stable `Notify` path")
	mustContain(t, string(supportDoc), "Codex package: production-ready official plugin package lane")
	mustContain(t, string(supportDoc), "- `plugin-kit-ai bootstrap` for `python` and `node` launcher-based projects on `codex-runtime` and `claude`")
	mustContain(t, string(supportDoc), "- `plugin-kit-ai doctor` for `python` and `node` launcher-based projects on `codex-runtime` and `claude`")
	mustContain(t, string(supportDoc), "- `plugin-kit-ai export` for `python` and `node` launcher-based projects on `codex-runtime` and `claude`")
	mustContain(t, string(supportDoc), "- `plugin-kit-ai bundle install` for local exported Python/Node bundles on `codex-runtime` and `claude`")
	mustContain(t, string(supportDoc), "Current beta CLI commands:")
	mustContain(t, string(supportDoc), "- `plugin-kit-ai bootstrap` for launcher-based `shell` projects")
	mustContain(t, string(supportDoc), "- `plugin-kit-ai doctor` for launcher-based `shell` projects")
	mustContain(t, string(supportDoc), "- `plugin-kit-ai export` for launcher-based `shell` projects")
	mustContain(t, string(supportDoc), "- `plugin-kit-ai bundle fetch` for remote exported Python/Node bundles")
	mustContain(t, string(supportDoc), "- `plugin-kit-ai bundle publish` for GitHub Releases handoff of exported Python/Node bundles")
	mustContain(t, string(supportDoc), "stable local-runtime interpreted subset:")
	mustContain(t, string(supportDoc), "stable scope is scaffold, validate, launcher execution, repo-local bootstrap, read-only doctor checks, bounded portable export bundles, and local exported bundle install")
	mustContain(t, string(supportDoc), "beta local-runtime remainder:")
	mustContain(t, string(supportDoc), "stable local bundle-install subset:")
	mustContain(t, string(supportDoc), "beta remote bundle-fetch subset:")
	mustContain(t, string(supportDoc), "beta GitHub bundle-publish subset:")
	mustContain(t, string(supportDoc), "creates a published release by default; `--draft` keeps the target release as draft")
	mustContain(t, string(supportDoc), "supported subset: exported `python` and `node` bundles for `codex-runtime` and `claude`")
	mustContain(t, string(supportDoc), "URL mode verifies `--sha256` or `<url>.sha256`")
	mustContain(t, string(supportDoc), "GitHub Releases mode prefers `checksums.txt` and falls back to `<asset>.sha256`")
	mustContain(t, string(supportDoc), "unsupported scope is universal package-management policy and packaged distribution through `plugin-kit-ai install`")
	mustContain(t, string(supportDoc), "target/package contract matrix")
	mustContain(t, string(supportDoc), "generated Claude/Codex config wiring is a repo-owned contract surface guarded by `render --check`")
	mustContain(t, string(statusDoc), "community-first interpreted stable subset promoted on main")
	mustContain(t, string(statusDoc), "Community polyglot subset")
	mustContain(t, string(interpretedPromotionDoc), "# Interpreted Stable Subset Audit")
	mustContain(t, string(interpretedPromotionDoc), "- `python`: `stable-approved`")
	mustContain(t, string(interpretedPromotionDoc), "- `node`: `stable-approved`")
	mustContain(t, string(interpretedPromotionDoc), "- `plugin-kit-ai bundle install`")
	mustContain(t, string(interpretedPromotionDoc), "- `plugin-kit-ai bundle fetch`")
	mustContain(t, string(interpretedPromotionDoc), "- `plugin-kit-ai bundle publish`")
	mustContain(t, string(interpretedPromotionDoc), "- `bundle install for exported python/node local bundles`: `stable-approved`")
	mustContain(t, string(interpretedPromotionDoc), "- `shell`: `stays-beta`")
	mustContain(t, string(productionDoc), "Claude: production-ready within the stable `Stop`, `PreToolUse`, and `UserPromptSubmit` event set")
	mustContain(t, string(productionDoc), "Codex runtime: production-ready within the stable `Notify` path")
	mustContain(t, string(productionDoc), "Codex package: production-ready official plugin package lane")
	mustContain(t, string(productionDoc), "Node/TypeScript and Python are the stable repo-local interpreted subset")
	mustContain(t, string(productionDoc), "After bootstrap, treat `validate --strict` as the CI-grade readiness gate for interpreted runtimes.")
	mustContain(t, string(productionDoc), "plugin-kit-ai doctor .")
	mustContain(t, string(productionDoc), "plugin-kit-ai export . --platform <codex-runtime|claude>")
	mustContain(t, string(productionDoc), "plugin-kit-ai bundle publish . --platform <codex-runtime|claude> --repo <owner/repo> --tag <tag>")
	mustContain(t, string(productionDoc), "creates a published release by default")
	mustContain(t, string(productionDoc), "plugin-kit-ai bundle install <bundle.tar.gz> --dest <path>")
	mustContain(t, string(productionDoc), "plugin-kit-ai bundle fetch --url <https://...tar.gz> --dest <path>")
	mustContain(t, string(productionDoc), "URL mode verifies `--sha256` or `<url>.sha256`, GitHub Releases mode prefers `checksums.txt` and falls back to `<asset>.sha256`")
	mustContain(t, string(productionDoc), "plugin-kit-ai import --from codex-runtime")
	mustContain(t, string(hardeningDoc), "beta contract cleanup, change-note hygiene, and documentation follow-through for beta leftovers")
	mustContain(t, string(hardeningDoc), "`python` and `node` are now the stable repo-local subset on `codex-runtime` and `claude`, while `shell` remains `public-beta`")
	mustContain(t, string(hardeningDoc), "local exported bundle install for Python/Node is now part of the promoted stable subset")
	mustContain(t, string(hardeningDoc), "remote bundle fetch for Python/Node remains `public-beta`")
	mustContain(t, string(hardeningDoc), "GitHub Releases bundle publish for Python/Node is now `public-beta`")
	mustContain(t, string(releaseDoc), "stable Node/Python doctor/bootstrap/export/bundle-install claims, beta remote bundle-fetch claims, beta GitHub bundle-publish claims")
	mustContain(t, string(releaseDoc), "beta change notes")
	mustContain(t, string(releaseChecklist), "beta change note written when beta user code, scaffold output, readiness semantics, or bundle contents change")
	mustContain(t, string(releaseChecklist), "stable Node/Python local-runtime or local bundle-install claims")
	mustContain(t, string(releaseChecklist), "[INTERPRETED_STABLE_SUBSET_AUDIT.md](./INTERPRETED_STABLE_SUBSET_AUDIT.md) updated when the promoted Node/Python local-runtime subset changes")
	mustContain(t, string(releaseNotesTemplate), "## Beta Contract Changes")
	mustContain(t, string(rehearsalTemplate), "- beta change notes updated:")
	mustContain(t, string(rehearsalTemplate), "`docs/INTERPRETED_STABLE_SUBSET_AUDIT.md` updated when the Node/Python local-runtime stable subset changed:")
	mustContain(t, string(makefile), "DoctorReadyNeedsBootstrapNeedsBuildAndBlocked")
	mustContain(t, string(makefile), "BundleInstallInstallsPythonBundleIntoDestination")
	mustContain(t, string(makefile), "BundleInstallRejectsRemoteURL")
	mustContain(t, string(makefile), "BundleFetchURLInstallsPythonBundleWithExplicitChecksum")
	mustContain(t, string(makefile), "BundleFetchGitHubInstallsNodeBundleFromChecksumsTxt")
	mustContain(t, string(makefile), "BundleFetchGitHubRejectsMetadataMismatch")
	mustContain(t, string(makefile), "BundlePublishCreatesPublishedReleaseByDefault")
	mustContain(t, string(makefile), "BundlePublishCreatesDraftReleaseWhenRequested")
	mustContain(t, string(makefile), "BundlePublishPromotesExistingDraftReleaseToPublished")
	mustContain(t, string(makefile), "BundlePublishReusesExistingDraftReleaseWhenRequested")
	mustContain(t, string(makefile), "BundlePublishReusesExistingPublishedReleaseWithForce")
	mustContain(t, string(makefile), "BundlePublishFailsWhenAssetExistsWithoutForce")
	mustContain(t, string(makefile), "GitHubClaudeNodeTypeScriptFlow")
	mustContain(t, string(makefile), "PublishFetch")
	mustContain(t, string(makefile), "BundleInstall")
	mustContain(t, string(makefile), "ClaudeNodeTypeScriptFlow")
	mustContain(t, string(makefile), "BundleFetchURL")
	mustContain(t, string(makefile), "ShellBundleFlow")
	mustContain(t, string(polyglotWorkflow), "DoctorReadyNeedsBootstrapNeedsBuildAndBlocked")
	mustContain(t, string(polyglotWorkflow), "BundleInstallInstallsPythonBundleIntoDestination")
	mustContain(t, string(polyglotWorkflow), "BundleInstallRejectsRemoteURL")
	mustContain(t, string(polyglotWorkflow), "BundleFetchURLInstallsPythonBundleWithExplicitChecksum")
	mustContain(t, string(polyglotWorkflow), "BundleFetchGitHubInstallsNodeBundleFromChecksumsTxt")
	mustContain(t, string(polyglotWorkflow), "BundleFetchGitHubRejectsMetadataMismatch")
	mustContain(t, string(polyglotWorkflow), "BundlePublishCreatesPublishedReleaseByDefault")
	mustContain(t, string(polyglotWorkflow), "BundlePublishCreatesDraftReleaseWhenRequested")
	mustContain(t, string(polyglotWorkflow), "BundlePublishPromotesExistingDraftReleaseToPublished")
	mustContain(t, string(polyglotWorkflow), "BundlePublishReusesExistingDraftReleaseWhenRequested")
	mustContain(t, string(polyglotWorkflow), "BundlePublishReusesExistingPublishedReleaseWithForce")
	mustContain(t, string(polyglotWorkflow), "BundlePublishFailsWhenAssetExistsWithoutForce")
	mustContain(t, string(polyglotWorkflow), "GitHubClaudeNodeTypeScriptFlow")
	mustContain(t, string(polyglotWorkflow), "PublishFetch")
	mustContain(t, string(polyglotWorkflow), "BundleInstall")
	mustContain(t, string(polyglotWorkflow), "ClaudeNodeTypeScriptFlow")
	mustContain(t, string(polyglotWorkflow), "BundleFetchURL")
	mustContain(t, string(polyglotWorkflow), "ShellBundleFlow")

	mustNotContain(t, string(rootReadme), "./bin/plugin-kit-ai validate ./my-plugin --platform codex --strict")
	mustNotContain(t, string(rootReadme), "./bin/plugin-kit-ai init my-plugin --runtime python")
	mustNotContain(t, string(rootReadme), "./bin/plugin-kit-ai import ./native-plugin --from codex\n")
	mustNotContain(t, string(rootReadme), "| `python` | public-beta |")
	mustNotContain(t, string(rootReadme), "| `node` | public-beta |")
	mustNotContain(t, string(cliReadme), "TypeScript via build-to-JS only")
	mustNotContain(t, string(cliReadme), "./bin/plugin-kit-ai import ./native-plugin --from codex\n")
	mustNotContain(t, string(cliReadme), "| `python` | public-beta |")
	mustNotContain(t, string(cliReadme), "| `node` | public-beta |")
	mustNotContain(t, string(cliReadme), "`plugin-kit-ai bundle install` is the `public-beta`")
	mustNotContain(t, string(cliReadme), "`plugin-kit-ai bundle fetch` is the stable")
	mustNotContain(t, string(cliReadme), "insecure-skip-tls-verify")
	mustNotContain(t, string(rootReadme), "creates a draft release when the tag is missing")
	mustNotContain(t, string(cliReadme), "creates a draft release when the tag is missing")
	mustNotContain(t, string(supportDoc), "missing `--tag` release creates a draft release")
	mustNotContain(t, string(productionDoc), "creates a draft release when the tag is missing")
	mustNotContain(t, string(releaseChecklist), "migration note written")
	mustNotContain(t, string(supportDoc), "should ship with migration guidance")
	mustNotContain(t, string(supportDoc), "local bundle-install beta surface:")

	abiDoc, err := os.ReadFile(filepath.Join(root, "docs", "EXECUTABLE_ABI.md"))
	if err != nil {
		t.Fatal(err)
	}
	mustNotContain(t, string(abiDoc), "creates a draft release when the tag is missing")
	mustContain(t, string(abiDoc), "`plugin-kit-ai validate --strict` is the canonical CI-grade readiness gate for interpreted runtimes")
	mustContain(t, string(abiDoc), "`plugin-kit-ai doctor` is the stable read-only readiness surface for the `python`/`node` local-runtime subset on `codex-runtime` and `claude`; `shell` remains beta")
	mustContain(t, string(abiDoc), "`plugin-kit-ai export` is the stable portable handoff surface for the `python`/`node` local-runtime subset on `codex-runtime` and `claude`; `shell` remains beta")
	mustContain(t, string(abiDoc), "`plugin-kit-ai bundle install` is the stable local bundle installer for exported `python`/`node` handoff bundles")
	mustContain(t, string(abiDoc), "`plugin-kit-ai bundle fetch` is the `public-beta` remote handoff companion for exported `python`/`node` bundles")
	mustContain(t, string(abiDoc), "`plugin-kit-ai bundle publish` is the `public-beta` GitHub Releases producer-side companion for exported `python`/`node` bundles")
	mustContain(t, string(abiDoc), "creates a published release by default")
	mustContain(t, string(abiDoc), "URL mode verifies `--sha256` or `<url>.sha256`, GitHub Releases mode prefers `checksums.txt` and falls back to `<asset>.sha256`")
	mustContain(t, string(abiDoc), "uses the same runtime lookup order as the generated launcher contract")
	mustContain(t, string(abiDoc), "| `python` | stable local-runtime subset | repo-local executable ABI on `codex-runtime` and `claude` | lockfile-first manager detection; `venv`/`requirements`/`uv` expect repo-local `.venv`, `poetry`/`pipenv` can use manager-owned envs |")
	mustContain(t, string(abiDoc), "| `node` | stable local-runtime subset | repo-local executable ABI on `codex-runtime` and `claude` | system Node.js `20+`; JavaScript by default, TypeScript via `--runtime node --typescript` |")
	mustNotContain(t, string(abiDoc), "TypeScript only via build-to-JavaScript")

	help := exec.Command(pluginKitAIBin, "bundle", "fetch", "--help")
	helpOut, err := help.CombinedOutput()
	if err != nil {
		t.Fatalf("bundle fetch help: %v\n%s", err, helpOut)
	}
	mustNotContain(t, string(helpOut), "insecure-skip-tls-verify")
	publishHelp := exec.Command(pluginKitAIBin, "bundle", "publish", "--help")
	publishHelpOut, err := publishHelp.CombinedOutput()
	if err != nil {
		t.Fatalf("bundle publish help: %v\n%s", err, publishHelpOut)
	}
	mustContain(t, string(publishHelpOut), "GitHub Releases")
	mustContain(t, string(publishHelpOut), "--draft")
}

func assertCapabilityContract(t *testing.T, entries map[string]map[string]any, key, wantMaturity, wantContract string) {
	t.Helper()
	entry, ok := entries[key]
	if !ok {
		t.Fatalf("missing capabilities entry %s", key)
	}
	if got := entry["maturity"]; got != wantMaturity {
		t.Fatalf("%s maturity = %v want %q", key, got, wantMaturity)
	}
	if got := entry["contract_class"]; got != wantContract {
		t.Fatalf("%s contract_class = %v want %q", key, got, wantContract)
	}
}

func mustContain(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("missing substring %q\n--- text ---\n%s", want, text)
	}
}

func mustNotContain(t *testing.T, text, want string) {
	t.Helper()
	if strings.Contains(text, want) {
		t.Fatalf("unexpected substring %q\n--- text ---\n%s", want, text)
	}
}
