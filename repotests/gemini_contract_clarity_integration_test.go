package pluginkitairepo_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestContractClarity_GeminiRuntimeDocsStayAligned(t *testing.T) {
	root := RepoRoot(t)
	pluginKitAIBin := buildPluginKitAI(t)

	matrixBody, err := os.ReadFile(filepath.Join(root, "docs", "generated", "support_matrix.md"))
	if err != nil {
		t.Fatal(err)
	}
	matrix := string(matrixBody)
	mustContain(t, matrix, "| gemini | SessionStart | runtime_supported | stable | production-ready | true |")
	mustContain(t, matrix, "| gemini | SessionEnd | runtime_supported | stable | production-ready | true |")
	mustContain(t, matrix, "| gemini | BeforeModel | runtime_supported | stable | production-ready | true |")
	mustContain(t, matrix, "| gemini | AfterModel | runtime_supported | stable | production-ready | true |")
	mustContain(t, matrix, "| gemini | BeforeToolSelection | runtime_supported | stable | production-ready | true |")
	mustContain(t, matrix, "| gemini | BeforeAgent | runtime_supported | stable | production-ready | true |")
	mustContain(t, matrix, "| gemini | AfterAgent | runtime_supported | stable | production-ready | true |")
	mustContain(t, matrix, "| gemini | BeforeTool | runtime_supported | stable | production-ready | true |")
	mustContain(t, matrix, "| gemini | AfterTool | runtime_supported | stable | production-ready | true |")

	targetMatrixBody, err := os.ReadFile(filepath.Join(root, "docs", "generated", "target_support_matrix.md"))
	if err != nil {
		t.Fatal(err)
	}
	targetMatrix := string(targetMatrixBody)
	mustContain(t, targetMatrix, "| gemini | extension_package | mcp_extension | optional | extension | copy install | link | restart required | ~/.gemini/extensions/<name> | production-ready extension target |")

	cmd := exec.Command(pluginKitAIBin, "capabilities", "--mode", "runtime", "--format", "json", "--platform", "gemini")
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
	assertCapabilityContract(t, byKey, "gemini/SessionStart", "stable", "production-ready")
	assertCapabilityContract(t, byKey, "gemini/SessionEnd", "stable", "production-ready")
	assertCapabilityContract(t, byKey, "gemini/BeforeModel", "stable", "production-ready")
	assertCapabilityContract(t, byKey, "gemini/AfterModel", "stable", "production-ready")
	assertCapabilityContract(t, byKey, "gemini/BeforeToolSelection", "stable", "production-ready")
	assertCapabilityContract(t, byKey, "gemini/BeforeAgent", "stable", "production-ready")
	assertCapabilityContract(t, byKey, "gemini/AfterAgent", "stable", "production-ready")
	assertCapabilityContract(t, byKey, "gemini/BeforeTool", "stable", "production-ready")
	assertCapabilityContract(t, byKey, "gemini/AfterTool", "stable", "production-ready")
	productionDoc, err := os.ReadFile(filepath.Join(root, "docs", "PRODUCTION.md"))
	if err != nil {
		t.Fatal(err)
	}
	supportDoc, err := os.ReadFile(filepath.Join(root, "docs", "SUPPORT.md"))
	if err != nil {
		t.Fatal(err)
	}
	sdkReadme, err := os.ReadFile(filepath.Join(root, "sdk", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	sdkStability, err := os.ReadFile(filepath.Join(root, "sdk", "STABILITY.md"))
	if err != nil {
		t.Fatal(err)
	}
	repoTestsReadme, err := os.ReadFile(filepath.Join(root, "repotests", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	geminiStarterReadme, err := os.ReadFile(filepath.Join(root, "cli", "plugin-kit-ai", "internal", "scaffold", "templates", "gemini.README.go.md.tmpl"))
	if err != nil {
		t.Fatal(err)
	}

	mustContain(t, string(productionDoc), "production-ready Go runtime for `SessionStart`, `SessionEnd`, `BeforeModel`, `AfterModel`, `BeforeToolSelection`, `BeforeAgent`, `AfterAgent`, `BeforeTool`, and `AfterTool`")
	mustContain(t, string(productionDoc), "make test-gemini-runtime")
	mustContain(t, string(productionDoc), "make test-gemini-runtime-live")

	mustContain(t, string(supportDoc), "`github.com/777genius/plugin-kit-ai/sdk/gemini`")
	mustContain(t, string(supportDoc), "`(*plugin-kit-ai.App).Gemini`")
	mustContain(t, string(supportDoc), "production-ready Go runtime for `SessionStart`, `SessionEnd`, `BeforeModel`, `AfterModel`, `BeforeToolSelection`, `BeforeAgent`, `AfterAgent`, `BeforeTool`, and `AfterTool`")
	mustContain(t, string(supportDoc), "[GEMINI_RUNTIME_AUDIT.md](./GEMINI_RUNTIME_AUDIT.md)")

	mustContain(t, string(sdkReadme), "`gemini/SessionStart`")
	mustContain(t, string(sdkReadme), "`gemini/BeforeToolSelection`")
	mustContain(t, string(sdkReadme), "`gemini/BeforeTool`")
	mustContain(t, string(sdkReadme), "`gemini/AfterTool`")
	mustContain(t, string(sdkReadme), "`gemini.BeforeToolSelectionForceAny(...)`")
	mustContain(t, string(sdkReadme), "`gemini.AfterToolTailCallValue(...)`")
	mustContain(t, string(sdkReadme), "[../../docs/GEMINI_RUNTIME_AUDIT.md](../../docs/GEMINI_RUNTIME_AUDIT.md)")

	mustContain(t, string(sdkStability), "`(*plugin-kit-ai.App).Gemini`")
	mustContain(t, string(sdkStability), "approved exported Gemini event and response types for:")
	mustContain(t, string(sdkStability), "`BeforeToolSelectionForceAny`")
	mustContain(t, string(sdkStability), "`AfterToolTailCallValue`")

	mustContain(t, string(repoTestsReadme), "`PLUGIN_KIT_AI_RUN_GEMINI_RUNTIME_LIVE=1`")
	mustContain(t, string(repoTestsReadme), "`PLUGIN_KIT_AI_E2E_GEMINI`")
	mustContain(t, string(repoTestsReadme), "make test-gemini-runtime")
	mustContain(t, string(repoTestsReadme), "make test-gemini-runtime-live")
	mustContain(t, string(repoTestsReadme), "production-ready runtime")
	mustContain(t, string(repoTestsReadme), "blocked-tool control semantics")

	mustContain(t, string(geminiStarterReadme), "This lane is production-ready for the supported Gemini Go runtime")
	mustContain(t, string(geminiStarterReadme), "make test-gemini-runtime")
	mustContain(t, string(geminiStarterReadme), "make test-gemini-runtime-live")
	mustContain(t, string(geminiStarterReadme), "`plugin-kit-ai capabilities --mode runtime --platform gemini`")
}
