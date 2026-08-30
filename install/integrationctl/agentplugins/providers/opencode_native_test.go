package providers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/providers/nativeconfig"
	"github.com/tailscale/hujson"
)

func TestOpenCodeInstalledCLIDiscoversIsolatedNativeConfig(t *testing.T) {
	binary := os.Getenv("AGENTPLUGINS_OPENCODE_BIN")
	if binary == "" {
		t.Skip("set AGENTPLUGINS_OPENCODE_BIN for the isolated installed-client E2E")
	}
	root := t.TempDir()
	home, xdg := filepath.Join(root, "home"), filepath.Join(root, "xdg")
	configRoot := filepath.Join(xdg, "opencode")
	active := filepath.Join(root, "managed", "clients", "opencode", "demo")
	envelope, plan := openCodeTestPackage(t, active, configRoot, "isolated")
	objects, err := buildOpenCodeNativeObjects(active, envelope, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOpenCodeNative(configRoot, active, nil, objects); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "debug", "config")
	command.Dir = project
	command.Env = []string{
		"HOME=" + home, "XDG_CONFIG_HOME=" + xdg,
		"XDG_DATA_HOME=" + filepath.Join(root, "data"), "XDG_CACHE_HOME=" + filepath.Join(root, "cache"),
		"XDG_STATE_HOME=" + filepath.Join(root, "state"), "OPENCODE_PURE=1", "PATH=" + os.Getenv("PATH"),
	}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated OpenCode config discovery failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "docs") || !strings.Contains(string(output), filepath.Join(active, "server.js")) {
		t.Fatalf("OpenCode did not discover the managed MCP entry:\n%s", output)
	}
}

func TestOpenCodeNativeAddUpdateVerifyRemovePreservesJSONCAndForeignConfig(t *testing.T) {
	root := t.TempDir()
	configRoot := filepath.Join(root, "xdg", "opencode")
	active := filepath.Join(root, "managed", "clients", "opencode", "demo")
	if err := os.MkdirAll(configRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	jsonc := filepath.Join(configRoot, "opencode.jsonc")
	writeOpenCodeTestFile(t, jsonc, "{\n  // keep this\n  \"theme\": \"night\",\n  \"mcp\": {\"foreign\": {\"type\":\"remote\",\"url\":\"https://foreign.test\"}},\n}\n")

	firstEnvelope, firstPlan := openCodeTestPackage(t, active, configRoot, "old")
	firstObjects, err := buildOpenCodeNativeObjects(active, firstEnvelope, firstPlan)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOpenCodeNative(configRoot, active, nil, firstObjects); err != nil {
		t.Fatal(err)
	}
	if err := verifyOpenCodeNativeObjects(configRoot, active, firstObjects); err != nil {
		t.Fatal(err)
	}
	body := readOpenCodeTestFile(t, jsonc)
	if !strings.Contains(body, "// keep this") || !strings.Contains(body, "foreign") {
		t.Fatalf("foreign JSONC content was lost:\n%s", body)
	}
	assertOpenCodeEntry(t, body, true, "node", filepath.Join(active, "server.js"))
	if got := readOpenCodeTestFile(t, filepath.Join(configRoot, "skills", "docs", "SKILL.md")); !strings.Contains(got, "old") {
		t.Fatalf("unexpected installed skill: %s", got)
	}

	secondEnvelope, secondPlan := openCodeTestPackage(t, active, configRoot, "new")
	secondEnvelope.MCP.Servers["docs"] = domain.MCPServer{Name: "docs", Type: "stdio", Decoded: map[string]any{
		"command": "bun", "args": []any{"${PLUGIN_ROOT}/server.js"}, "env": map[string]any{"DATA": "${PLUGIN_DATA}"},
	}}
	if err := os.Remove(filepath.Join(active, openCodeProjectionFile)); err != nil {
		t.Fatal(err)
	}
	if err := projectOpenCodeNative(active, secondEnvelope, secondPlan, filepath.Join(root, "data")); err != nil {
		t.Fatal(err)
	}
	secondObjects, err := buildOpenCodeNativeObjects(active, secondEnvelope, secondPlan)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOpenCodeNative(configRoot, active, firstObjects, secondObjects); err != nil {
		t.Fatal(err)
	}
	body = readOpenCodeTestFile(t, jsonc)
	assertOpenCodeEntry(t, body, true, "bun", filepath.Join(active, "server.js"))
	if got := readOpenCodeTestFile(t, filepath.Join(configRoot, "skills", "docs", "SKILL.md")); !strings.Contains(got, "new") {
		t.Fatalf("skill was not updated: %s", got)
	}

	if err := applyOpenCodeNative(configRoot, "", secondObjects, nil); err != nil {
		t.Fatal(err)
	}
	body = readOpenCodeTestFile(t, jsonc)
	assertOpenCodeEntry(t, body, false, "", "")
	if !strings.Contains(body, "// keep this") || !strings.Contains(body, "foreign") {
		t.Fatalf("remove changed foreign config:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(configRoot, "skills", "docs")); !os.IsNotExist(err) {
		t.Fatalf("managed skill still exists: %v", err)
	}
}

func TestOpenCodeCollisionRollsBackStagedSkill(t *testing.T) {
	root := t.TempDir()
	configRoot := filepath.Join(root, "xdg", "opencode")
	active := filepath.Join(root, "managed", "clients", "opencode", "demo")
	writeOpenCodeTestFile(t, filepath.Join(configRoot, "opencode.json"), `{"mcp":{"docs":{"type":"remote","url":"https://foreign.test"}}}`)
	envelope, plan := openCodeTestPackage(t, active, configRoot, "owned")
	objects, err := buildOpenCodeNativeObjects(active, envelope, plan)
	if err != nil {
		t.Fatal(err)
	}
	err = applyOpenCodeNative(configRoot, active, nil, objects)
	if !errors.Is(err, nativeconfig.ErrCollision) {
		t.Fatalf("expected foreign collision, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(configRoot, "skills", "docs")); !os.IsNotExist(err) {
		t.Fatalf("skill survived failed MCP transaction: %v", err)
	}
	if body := readOpenCodeTestFile(t, filepath.Join(configRoot, "opencode.json")); !strings.Contains(body, "https://foreign.test") {
		t.Fatalf("foreign MCP entry changed: %s", body)
	}
}

func TestOpenCodeFailedUpdateRestoresPreviousManagedSkill(t *testing.T) {
	root := t.TempDir()
	configRoot := filepath.Join(root, "xdg", "opencode")
	active := filepath.Join(root, "managed", "demo")
	firstEnvelope, firstPlan := openCodeTestPackage(t, active, configRoot, "old")
	first, err := buildOpenCodeNativeObjects(active, firstEnvelope, firstPlan)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOpenCodeNative(configRoot, active, nil, first); err != nil {
		t.Fatal(err)
	}
	// Simulate an out-of-band MCP edit before an update. The provider must
	// reject the stale receipt and restore the old skill directory.
	writeOpenCodeTestFile(t, filepath.Join(configRoot, "opencode.json"), `{"mcp":{"docs":{"type":"local","command":["foreign"]}}}`)
	secondEnvelope, secondPlan := openCodeTestPackage(t, active, configRoot, "new")
	second, err := buildOpenCodeNativeObjects(active, secondEnvelope, secondPlan)
	if err != nil {
		t.Fatal(err)
	}
	err = applyOpenCodeNative(configRoot, active, first, second)
	if !errors.Is(err, nativeconfig.ErrNotOwned) {
		t.Fatalf("expected stale ownership rejection, got %v", err)
	}
	if got := readOpenCodeTestFile(t, filepath.Join(configRoot, "skills", "docs", "SKILL.md")); !strings.Contains(got, "old") {
		t.Fatalf("previous skill was not restored: %s", got)
	}
	if body := readOpenCodeTestFile(t, filepath.Join(configRoot, "opencode.json")); !strings.Contains(body, "foreign") {
		t.Fatalf("out-of-band config was overwritten: %s", body)
	}
}

func TestOpenCodeRepairRecreatesOnlyAnAbsentExactOwnedEntryAndRemoveIsIdempotent(t *testing.T) {
	root := t.TempDir()
	configRoot := filepath.Join(root, "opencode")
	active := filepath.Join(root, "managed", "demo")
	configPath := filepath.Join(configRoot, "opencode.jsonc")
	writeOpenCodeTestFile(t, configPath, "{\n  // selected JSONC\n  \"mcp\": {},\n}\n")
	envelope, plan := openCodeTestPackage(t, active, configRoot, "owned")
	objects, err := buildOpenCodeNativeObjects(active, envelope, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOpenCodeNative(configRoot, active, nil, objects); err != nil {
		t.Fatal(err)
	}

	// Missing is the only state that exact-same repair may recreate.
	writeOpenCodeTestFile(t, configPath, "{\n  // selected JSONC\n  \"mcp\": {\"foreign\": {\"type\":\"remote\",\"url\":\"https://foreign.test\"}},\n}\n")
	if err := applyOpenCodeNative(configRoot, active, objects, objects); err != nil {
		t.Fatalf("repair absent exact-owned entry: %v", err)
	}
	body := readOpenCodeTestFile(t, configPath)
	assertOpenCodeEntry(t, body, true, "node", filepath.Join(active, "server.js"))
	if _, err := os.Stat(filepath.Join(configRoot, "opencode.json")); !os.IsNotExist(err) {
		t.Fatalf("repair switched away from the exact owned JSONC path: %v", err)
	}

	// A present entry with different bytes is foreign/tampered and must never
	// be adopted by repair.
	writeOpenCodeTestFile(t, configPath, `{"mcp":{"docs":{"type":"local","command":["foreign"]}}}`)
	if err := applyOpenCodeNative(configRoot, active, objects, objects); !errors.Is(err, nativeconfig.ErrNotOwned) {
		t.Fatalf("tampered entry was not rejected: %v", err)
	}
	if body := readOpenCodeTestFile(t, configPath); !strings.Contains(body, "foreign") {
		t.Fatalf("tampered entry changed: %s", body)
	}

	// An entry already absent at remove time is a safe idempotent success. The
	// separately owned skill still has to be removed.
	if err := os.Remove(configPath); err != nil {
		t.Fatal(err)
	}
	if err := applyOpenCodeNative(configRoot, "", objects, nil); err != nil {
		t.Fatalf("idempotent absent remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configRoot, "skills", "docs")); !os.IsNotExist(err) {
		t.Fatalf("managed skill survived idempotent remove: %v", err)
	}
}

func TestOpenCodeProjectionPreservesOfficialLocalCWD(t *testing.T) {
	root := t.TempDir()
	configRoot := filepath.Join(root, "opencode")
	active := filepath.Join(root, "managed", "demo")
	envelope, plan := openCodeTestPackage(t, active, configRoot, "owned")
	envelope.MCP.Servers["docs"] = domain.MCPServer{Name: "docs", Type: "stdio", Decoded: map[string]any{
		"command": "node", "args": []any{"${PLUGIN_ROOT}/server.js"}, "cwd": "./workspace",
	}}
	if err := os.Remove(filepath.Join(active, openCodeProjectionFile)); err != nil {
		t.Fatal(err)
	}
	if err := projectOpenCodeNative(active, envelope, plan, filepath.Join(root, "data")); err != nil {
		t.Fatal(err)
	}
	objects, err := buildOpenCodeNativeObjects(active, envelope, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOpenCodeNative(configRoot, active, nil, objects); err != nil {
		t.Fatal(err)
	}
	body := readOpenCodeTestFile(t, filepath.Join(configRoot, "opencode.json"))
	var document map[string]any
	if err := json.Unmarshal([]byte(body), &document); err != nil {
		t.Fatal(err)
	}
	entry := document["mcp"].(map[string]any)["docs"].(map[string]any)
	if entry["cwd"] != filepath.Join(active, "workspace") {
		t.Fatalf("OpenCode cwd was dropped or changed: %#v", entry)
	}
}

func TestOpenCodeProjectsClientManagedStdioDataContract(t *testing.T) {
	root := t.TempDir()
	configRoot := filepath.Join(root, "opencode")
	active := filepath.Join(root, "managed", "demo")
	dataRoot := filepath.Join(root, "managed", "data")
	envelope, plan := openCodeTestPackage(t, active, configRoot, "owned")
	authorEnv := envelope.MCP.Servers["docs"].Decoded["env"].(map[string]any)
	if err := os.Remove(filepath.Join(active, openCodeProjectionFile)); err != nil {
		t.Fatal(err)
	}
	if err := projectOpenCodeNative(active, envelope, plan, dataRoot); err != nil {
		t.Fatal(err)
	}
	objects, err := buildOpenCodeNativeObjects(active, envelope, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOpenCodeNative(configRoot, active, nil, objects); err != nil {
		t.Fatal(err)
	}
	document := map[string]any{}
	if err := json.Unmarshal([]byte(readOpenCodeTestFile(t, filepath.Join(configRoot, "opencode.json"))), &document); err != nil {
		t.Fatal(err)
	}
	entry := document["mcp"].(map[string]any)["docs"].(map[string]any)
	env := entry["environment"].(map[string]any)
	if env["PLUGIN_ROOT"] != active || env["PLUGIN_DATA"] != dataRoot || env["DATA"] != dataRoot {
		t.Fatalf("OpenCode stdio environment = %#v", env)
	}
	if entry["cwd"] != active {
		t.Fatalf("OpenCode default cwd = %v, want %v", entry["cwd"], active)
	}
	if len(authorEnv) != 1 || authorEnv["DATA"] != "${PLUGIN_DATA}" {
		t.Fatalf("OpenCode projection mutated package env: %#v", authorEnv)
	}
}

func TestOpenCodeRejectsReservedStdioEnvWithoutProjection(t *testing.T) {
	root := t.TempDir()
	active := filepath.Join(root, "active")
	if err := os.MkdirAll(active, 0o700); err != nil {
		t.Fatal(err)
	}
	authorEnv := map[string]any{"KEEP": "value", "PLUGIN_ROOT": "/attacker"}
	envelope := domain.PackageEnvelope{MCP: domain.MCPComponent{Servers: map[string]domain.MCPServer{
		"docs": {Name: "docs", Type: "stdio", Decoded: map[string]any{"command": "node", "env": authorEnv}},
	}}}
	plan := domain.DeliveryPlan{ClientID: domain.ClientOpenCode, NativeRegistryRoot: filepath.Join(root, "config"), ActivePath: active,
		Components: []domain.ComponentDecision{{Kind: domain.ComponentMCPServer, Name: "docs", Support: domain.SupportPrepared}}}
	err := projectOpenCodeNative(active, envelope, plan, filepath.Join(root, "data"))
	if err == nil || !strings.Contains(err.Error(), "PLUGIN_ROOT is reserved") {
		t.Fatalf("reserved OpenCode env error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(active, openCodeProjectionFile)); !os.IsNotExist(err) {
		t.Fatalf("rejected OpenCode env created projection: %v", err)
	}
	if len(authorEnv) != 2 || authorEnv["KEEP"] != "value" || authorEnv["PLUGIN_ROOT"] != "/attacker" {
		t.Fatalf("rejected OpenCode projection mutated package env: %#v", authorEnv)
	}
}

func TestOpenCodeAmbiguousJSONVariantsFailBeforeProjection(t *testing.T) {
	root := t.TempDir()
	configRoot := filepath.Join(root, "opencode")
	writeOpenCodeTestFile(t, filepath.Join(configRoot, "opencode.json"), `{}`)
	writeOpenCodeTestFile(t, filepath.Join(configRoot, "opencode.jsonc"), `{}`)
	active := filepath.Join(root, "managed", "demo")
	if err := os.MkdirAll(active, 0o700); err != nil {
		t.Fatal(err)
	}
	envelope := domain.PackageEnvelope{MCP: domain.MCPComponent{Servers: map[string]domain.MCPServer{}}}
	plan := domain.DeliveryPlan{ClientID: domain.ClientOpenCode, NativeRegistryRoot: configRoot, ActivePath: active}
	if err := projectOpenCodeNative(active, envelope, plan, ""); !errors.Is(err, nativeconfig.ErrAmbiguousConfig) {
		t.Fatalf("expected ambiguous config rejection, got %v", err)
	}
}

func TestOpenCodeRejectsPackageOwnedProjectionCollision(t *testing.T) {
	root := t.TempDir()
	active := filepath.Join(root, "active")
	writeOpenCodeTestFile(t, filepath.Join(active, openCodeProjectionFile), `{"attacker":true}`)
	plan := domain.DeliveryPlan{ClientID: domain.ClientOpenCode, NativeRegistryRoot: filepath.Join(root, "config"), ActivePath: active}
	err := projectOpenCodeNative(active, domain.PackageEnvelope{}, plan, "")
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("expected reserved projection rejection, got %v", err)
	}
}

func TestOpenCodeConfigSelectionDriftFailsBeforeSkillMutation(t *testing.T) {
	root := t.TempDir()
	configRoot := filepath.Join(root, "opencode")
	active := filepath.Join(root, "managed", "demo")
	envelope, plan := openCodeTestPackage(t, active, configRoot, "owned")
	objects, err := buildOpenCodeNativeObjects(active, envelope, plan)
	if err != nil {
		t.Fatal(err)
	}
	writeOpenCodeTestFile(t, filepath.Join(configRoot, "opencode.jsonc"), `{}`)
	err = applyOpenCodeNative(configRoot, active, nil, objects)
	if err == nil || !strings.Contains(err.Error(), "selection changed") {
		t.Fatalf("expected selection drift rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(configRoot, "skills", "docs")); !os.IsNotExist(err) {
		t.Fatalf("selection drift mutated skills: %v", err)
	}
	if body := readOpenCodeTestFile(t, filepath.Join(configRoot, "opencode.jsonc")); body != `{}` {
		t.Fatalf("selection drift mutated alternate config: %s", body)
	}
}

func openCodeTestPackage(t *testing.T, active, configRoot, skillText string) (domain.PackageEnvelope, domain.DeliveryPlan) {
	t.Helper()
	if err := os.Remove(filepath.Join(active, openCodeProjectionFile)); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	writeOpenCodeTestFile(t, filepath.Join(active, "server.js"), "// server")
	writeOpenCodeTestFile(t, filepath.Join(active, "skills", "docs", "SKILL.md"), "# "+skillText)
	envelope := domain.PackageEnvelope{
		Skills: map[string]domain.Skill{"docs": {Name: "docs", RelativePath: "skills/docs/SKILL.md"}},
		MCP: domain.MCPComponent{Servers: map[string]domain.MCPServer{"docs": {Name: "docs", Type: "stdio", Decoded: map[string]any{
			"command": "node", "args": []any{"${PLUGIN_ROOT}/server.js"}, "env": map[string]any{"DATA": "${PLUGIN_DATA}"},
		}}}},
	}
	plan := domain.DeliveryPlan{ClientID: domain.ClientOpenCode, NativeRegistryRoot: configRoot, ActivePath: active, Components: []domain.ComponentDecision{
		{Kind: domain.ComponentSkill, Name: "docs", Support: domain.SupportPrepared},
		{Kind: domain.ComponentMCPServer, Name: "docs", Support: domain.SupportPrepared},
	}}
	if err := projectOpenCodeNative(active, envelope, plan, filepath.Join(filepath.Dir(active), "data")); err != nil {
		t.Fatal(err)
	}
	return envelope, plan
}

func writeOpenCodeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readOpenCodeTestFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func assertOpenCodeEntry(t *testing.T, body string, present bool, command, arg string) {
	t.Helper()
	standard, err := hujson.Standardize([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(standard, &document); err != nil {
		t.Fatal(err)
	}
	mcp := document["mcp"].(map[string]any)
	entry, exists := mcp["docs"].(map[string]any)
	if exists != present {
		t.Fatalf("docs presence = %v, want %v: %#v", exists, present, mcp)
	}
	if present {
		argv := entry["command"].([]any)
		if entry["type"] != "local" || argv[0] != command || argv[1] != arg {
			t.Fatalf("unexpected OpenCode entry: %#v", entry)
		}
	}
}
