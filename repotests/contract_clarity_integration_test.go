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
	mustContain(t, targetMatrix, "| codex | packaged_runtime | mixed_package_runtime | required | plugin | plugin directory or marketplace cache |")
	mustContain(t, targetMatrix, "| gemini | extension_package | mcp_extension | ignored | extension | copy install | link | restart required | ~/.gemini/extensions/<name> | packaging-only target |")

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
	productionDoc, err := os.ReadFile(filepath.Join(root, "docs", "PRODUCTION.md"))
	if err != nil {
		t.Fatal(err)
	}

	mustContain(t, string(rootReadme), "full Gemini CLI extension packaging lane through `render|import|validate`")
	mustContain(t, string(rootReadme), "### Fast Local Plugin")
	mustContain(t, string(rootReadme), "### Production-Ready Plugin Repo")
	mustContain(t, string(rootReadme), "### Already Have Native Config")
	mustContain(t, string(rootReadme), "Reference repos: [examples/local/README.md](examples/local/README.md)")
	mustContain(t, string(rootReadme), "`plugin-kit-ai capabilities` now defaults to target/package introspection")
	mustContain(t, string(rootReadme), "| `python` | public-beta | repo-local executable ABI | prefer `.venv`, fallback to system Python `3.10+` |")
	mustContain(t, string(rootReadme), "Generated Claude/Codex config shapes are part of the repo-owned contract surface")
	mustContain(t, string(rootReadme), "`validate --strict` is the canonical CI-grade readiness gate")
	mustContain(t, string(cliReadme), "## Fast Local Plugin")
	mustContain(t, string(cliReadme), "## Production-Ready Plugin Repo")
	mustContain(t, string(cliReadme), "## Already Have Native Config")
	mustContain(t, string(cliReadme), "Reference repos: [../../examples/local/README.md](../../examples/local/README.md)")
	mustContain(t, string(cliReadme), "Gemini is a `packaging-only Gemini CLI extension target` in this CLI surface, not a production-ready runtime target")
	mustContain(t, string(cliReadme), "`plugin-kit-ai capabilities` defaults to the target/package view")
	mustContain(t, string(cliReadme), "| `node` | public-beta | repo-local only | system Node.js `20+`; TypeScript via build-to-JS only |")
	mustContain(t, string(cliReadme), "Generated Claude/Codex config shapes are part of the repo-owned contract surface")
	mustContain(t, string(pluginsExamplesReadme), "# Production Plugin Examples")
	mustContain(t, string(pluginsExamplesReadme), "For repo-local Python/Node entrance examples, see [../local/README.md](../local/README.md).")
	mustContain(t, string(supportDoc), "Gemini: full Gemini CLI extension packaging lane through `plugin-kit-ai render|import|validate` and local `extensions link|config|disable|enable`; not a production-ready runtime target")
	mustContain(t, string(supportDoc), "unsupported scope is dependency installation, package management, and packaged distribution through `plugin-kit-ai install`")
	mustContain(t, string(supportDoc), "target/package contract matrix")
	mustContain(t, string(supportDoc), "generated Claude/Codex config wiring is a repo-owned contract surface guarded by `render --check`")
	mustContain(t, string(productionDoc), "Claude: production-ready within the stable `Stop`, `PreToolUse`, and `UserPromptSubmit` event set")
	mustContain(t, string(productionDoc), "Codex: production-ready within the stable `Notify` path")
	mustContain(t, string(productionDoc), "Interpreted runtimes are production-hardened for scaffold, validate, launcher execution, and repo-local bootstrap only.")
	mustContain(t, string(productionDoc), "After bootstrap, treat `validate --strict` as the CI-grade readiness gate for interpreted runtimes.")

	abiDoc, err := os.ReadFile(filepath.Join(root, "docs", "EXECUTABLE_ABI.md"))
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, string(abiDoc), "`plugin-kit-ai validate --strict` is the canonical CI-grade readiness gate for interpreted runtimes")
	mustContain(t, string(abiDoc), "uses the same runtime lookup order as the generated launcher contract")
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
