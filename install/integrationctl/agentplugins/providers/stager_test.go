package providers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func TestStagerBuildsOpenAIProjectionWithoutMutatingPortableSnapshot(t *testing.T) {
	t.Parallel()
	envelope := stagingEnvelope(t)
	plan := stagingPlan(t, domain.ClientCodex, domain.PackageProjection)
	delivery, err := (Stager{}).Stage(context.Background(), envelope, plan, "operation-1", domain.CompatibilityHints{
		OpenAIMCPAuth: map[string]domain.OpenAIMCPAuthHint{
			"notion": {OAuthResource: "https://mcp.notion.com"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(delivery.ArtifactDigest, "sha256:") {
		t.Fatalf("artifact digest = %q", delivery.ArtifactDigest)
	}
	assertMissing(t, filepath.Join(delivery.StagingPath, "skills", "bad"))
	assertExecutable(t, filepath.Join(delivery.StagingPath, "bin", "run"))

	portableManifest := readObject(t, filepath.Join(envelope.SnapshotRoot, "plugin.json"))
	if _, ok := portableManifest["extensions"]; !ok {
		t.Fatal("source snapshot was mutated")
	}
	stagedManifest := readObject(t, filepath.Join(delivery.StagingPath, "plugin.json"))
	if _, ok := stagedManifest["extensions"]; ok {
		t.Fatal("unsupported extension survived staged standard manifest")
	}
	openAIManifest := readObject(t, filepath.Join(delivery.StagingPath, ".codex-plugin", "plugin.json"))
	if openAIManifest["mcpServers"] != "./.mcp.json" || openAIManifest["skills"] != "./skills/" {
		t.Fatalf("OpenAI manifest = %+v", openAIManifest)
	}
	openAIMCP := readObject(t, filepath.Join(delivery.StagingPath, ".mcp.json"))
	servers := openAIMCP["mcpServers"].(map[string]any)
	notion := servers["notion"].(map[string]any)
	if notion["type"] != "http" || notion["oauth_resource"] != "https://mcp.notion.com" {
		t.Fatalf("OpenAI MCP = %+v", notion)
	}
	codexMarketplace := readObject(t, filepath.Join(delivery.StagingPath, ".agents", "plugins", "marketplace.json"))
	if codexMarketplace["name"] != managedMarketplaceName(plan.PhysicalArtifactID) {
		t.Fatalf("Codex marketplace = %+v", codexMarketplace)
	}
	plugins := codexMarketplace["plugins"].([]any)
	source := plugins[0].(map[string]any)["source"].(map[string]any)
	if source["source"] != "local" || source["path"] != "./" {
		t.Fatalf("Codex marketplace source = %+v", source)
	}
	standardMCP := readObject(t, filepath.Join(delivery.StagingPath, "mcp.json"))
	standardServers := standardMCP["mcpServers"].(map[string]any)
	if _, exists := standardServers["broken"]; exists {
		t.Fatal("invalid MCP server survived staging")
	}
}

func TestStagerProjectsExactOwnedPluginDataContract(t *testing.T) {
	t.Parallel()
	envelope := stagingEnvelope(t)
	server := domain.MCPServer{Name: "local", Type: "stdio", Decoded: map[string]any{
		"type": "stdio", "command": "${PLUGIN_ROOT}/must-not-expand",
		"args": []any{"${PLUGIN_ROOT}/bin/run", "${PLUGIN_DATA}/cache"},
		"env":  map[string]any{"CACHE": "${PLUGIN_DATA}/cache"},
		"cwd":  "${PLUGIN_DATA}/work",
	}}
	envelope.MCP.Servers = map[string]domain.MCPServer{"local": server}
	plan := stagingPlan(t, domain.ClientCodex, domain.PackageProjection)
	plan.Components = []domain.ComponentDecision{{Kind: domain.ComponentMCPServer, Name: "local", Support: domain.SupportProjected}}
	ownedData := filepath.Join(t.TempDir(), "owned-plugin-data")
	delivery, err := (Stager{}).StageWithPluginData(context.Background(), envelope, plan, "operation-data", domain.CompatibilityHints{}, ownedData)
	if err != nil {
		t.Fatal(err)
	}
	document := readObject(t, filepath.Join(delivery.StagingPath, ".mcp.json"))
	projected := document["mcpServers"].(map[string]any)["local"].(map[string]any)
	if projected["command"] != "${PLUGIN_ROOT}/must-not-expand" {
		t.Fatalf("stdio command placeholder was expanded: %+v", projected)
	}
	env := projected["env"].(map[string]any)
	if env["PLUGIN_ROOT"] != plan.ActivePath || env["PLUGIN_DATA"] != ownedData || env["CACHE"] != filepath.Join(ownedData, "cache") {
		t.Fatalf("stdio env = %+v", env)
	}
	args := projected["args"].([]any)
	if args[0] != filepath.Join(plan.ActivePath, "bin", "run") || args[1] != filepath.Join(ownedData, "cache") || projected["cwd"] != filepath.Join(ownedData, "work") {
		t.Fatalf("stdio args/cwd = %+v / %v", args, projected["cwd"])
	}
}

func TestStagerBuildsChatGPTAppProjectionWithBundledMCPParity(t *testing.T) {
	t.Parallel()
	envelope := stagingEnvelope(t)
	app := `{"apps":{"notion":{"id":"asdk_app_notion_123","required":true}}}`
	writeTestFile(t, filepath.Join(envelope.SnapshotRoot, ".app.json"), app)
	envelope.App = domain.AppComponent{
		Present: true, Declared: true, Enabled: true, Raw: json.RawMessage(app),
		Bindings: map[string]domain.AppBinding{"notion": {Alias: "notion", ID: "asdk_app_notion_123", Required: true}},
	}
	envelope.Inventory.AppPresent = true
	envelope.Inventory.AppBindings = []string{"notion"}
	plan := stagingPlan(t, domain.ClientChatGPT, domain.PackageProjection)
	plan.Components = append(plan.Components, domain.ComponentDecision{Kind: domain.ComponentApp, Name: "notion", Support: domain.SupportProjected})

	delivery, err := (Stager{}).Stage(context.Background(), envelope, plan, "operation-chatgpt", domain.CompatibilityHints{
		OpenAIMCPAuth: map[string]domain.OpenAIMCPAuthHint{
			"notion": {OAuthResource: "https://mcp.notion.com"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest := readObject(t, filepath.Join(delivery.StagingPath, ".codex-plugin", "plugin.json"))
	if manifest["apps"] != "./.app.json" || manifest["mcpServers"] != "./.mcp.json" || manifest["skills"] != "./skills/" {
		t.Fatalf("ChatGPT manifest = %+v", manifest)
	}
	openAIMCP := readObject(t, filepath.Join(delivery.StagingPath, ".mcp.json"))
	servers := openAIMCP["mcpServers"].(map[string]any)
	notion := servers["notion"].(map[string]any)
	if notion["type"] != "http" || notion["oauth_resource"] != "https://mcp.notion.com" {
		t.Fatalf("ChatGPT MCP parity projection = %+v", notion)
	}
	assertMissing(t, filepath.Join(delivery.StagingPath, "plugin.json"))
	assertMissing(t, filepath.Join(delivery.StagingPath, "mcp.json"))
	body, err := os.ReadFile(filepath.Join(delivery.StagingPath, ".app.json"))
	if err != nil || string(body) != app {
		t.Fatalf("lossless app projection = %q, %v", body, err)
	}
	if source, err := os.ReadFile(filepath.Join(envelope.SnapshotRoot, "mcp.json")); err != nil || len(source) == 0 {
		t.Fatal("portable source MCP was mutated")
	}
}

func TestStagerBuildsManagedCopilotMarketplaceForCopilotAndVSCode(t *testing.T) {
	t.Parallel()
	for _, client := range []domain.ClientID{domain.ClientCopilot, domain.ClientVSCode} {
		client := client
		t.Run(string(client), func(t *testing.T) {
			envelope := stagingEnvelope(t)
			plan := stagingPlan(t, client, domain.PackageNative)
			delivery, err := (Stager{}).Stage(context.Background(), envelope, plan, "operation-"+string(client), domain.CompatibilityHints{})
			if err != nil {
				t.Fatal(err)
			}
			marketplace := readObject(t, filepath.Join(delivery.StagingPath, ".github", "plugin", "marketplace.json"))
			if marketplace["name"] != managedMarketplaceName(plan.PhysicalArtifactID) {
				t.Fatalf("marketplace = %+v", marketplace)
			}
			plugins := marketplace["plugins"].([]any)
			plugin := plugins[0].(map[string]any)
			if plugin["name"] != "demo" || plugin["source"] != "." || plugin["version"] != "1.0.0" {
				t.Fatalf("plugin entry = %+v", plugin)
			}
			assertMissing(t, filepath.Join(envelope.SnapshotRoot, ".github"))
		})
	}
}

func TestStagerKeepsNativePackageAndSanitizesFailureBoundaries(t *testing.T) {
	t.Parallel()
	envelope := stagingEnvelope(t)
	plan := stagingPlan(t, domain.ClientCursor, domain.PackageNative)
	for index := range plan.Components {
		if plan.Components[index].Kind == domain.ComponentExtension {
			plan.Components[index].Support = domain.SupportNative
		}
	}
	delivery, err := (Stager{}).Stage(context.Background(), envelope, plan, "operation-2", domain.CompatibilityHints{})
	if err != nil {
		t.Fatal(err)
	}
	assertMissing(t, filepath.Join(delivery.StagingPath, ".codex-plugin"))
	manifest := readObject(t, filepath.Join(delivery.StagingPath, "plugin.json"))
	if _, ok := manifest["extensions"]; !ok {
		t.Fatal("supported native extension was removed")
	}
	mcp := readObject(t, filepath.Join(delivery.StagingPath, "mcp.json"))
	servers := mcp["mcpServers"].(map[string]any)
	if len(servers) != 1 || servers["notion"] == nil {
		t.Fatalf("sanitized servers = %+v", servers)
	}
}

func TestStagerInvalidSkillNamedSkillsDoesNotRemoveSkillsRoot(t *testing.T) {
	t.Parallel()
	envelope := stagingEnvelope(t)
	writeTestFile(t, filepath.Join(envelope.SnapshotRoot, "skills", "skills", "SKILL.md"), "invalid\n")
	envelope.Inventory.InvalidSkills = append(envelope.Inventory.InvalidSkills, "skills")
	plan := stagingPlan(t, domain.ClientCursor, domain.PackageNative)
	delivery, err := (Stager{}).Stage(context.Background(), envelope, plan, "operation-skill-name", domain.CompatibilityHints{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(delivery.StagingPath, "skills", "good", "SKILL.md")); err != nil {
		t.Fatalf("valid sibling skill was removed: %v", err)
	}
	assertMissing(t, filepath.Join(delivery.StagingPath, "skills", "skills"))
}

func TestStagerRejectsUnsupportedPlanAndExistingOperationStaging(t *testing.T) {
	t.Parallel()
	envelope := stagingEnvelope(t)
	plan := stagingPlan(t, domain.ClientKiro, domain.PackageNative)
	plan.Status = domain.PlanUnsupported
	if _, err := (Stager{}).Stage(context.Background(), envelope, plan, "operation-3", domain.CompatibilityHints{}); err == nil {
		t.Fatal("unsupported plan was staged")
	}
	plan.Status = domain.PlanManualActivationRequired
	first, err := (Stager{}).Stage(context.Background(), envelope, plan, "operation-3", domain.CompatibilityHints{})
	if err != nil {
		t.Fatal(err)
	}
	if first.StagingPath == "" {
		t.Fatal("first staging path is empty")
	}
	if _, err := (Stager{}).Stage(context.Background(), envelope, plan, "operation-3", domain.CompatibilityHints{}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate staging error = %v", err)
	}
}

func TestStagerVerifiesAndSafelyDiscardsOnlyItsStagingDirectory(t *testing.T) {
	t.Parallel()
	envelope := stagingEnvelope(t)
	plan := stagingPlan(t, domain.ClientCursor, domain.PackageNative)
	for index := range plan.Components {
		if plan.Components[index].Kind == domain.ComponentExtension {
			plan.Components[index].Support = domain.SupportNative
		}
	}
	stager := Stager{}
	delivery, err := stager.Stage(context.Background(), envelope, plan, "operation-4", domain.CompatibilityHints{})
	if err != nil {
		t.Fatal(err)
	}
	if err := stager.Verify(context.Background(), delivery.StagingPath, delivery.ArtifactDigest); err != nil {
		t.Fatalf("verify: %v", err)
	}
	unsafe := delivery
	unsafe.StagingPath = delivery.ActivePath
	if err := stager.Discard(context.Background(), unsafe); err == nil {
		t.Fatal("active path accepted as staging cleanup target")
	}
	if err := stager.Discard(context.Background(), delivery); err != nil {
		t.Fatal(err)
	}
	assertMissing(t, delivery.StagingPath)
}

func TestStagerVerifyRejectsMarkersExcludedFromPortableSnapshotDigest(t *testing.T) {
	t.Parallel()
	for _, marker := range []string{filepath.Join(".git", "config"), filepath.Join("nested", ".plugin-kit-ai.lock")} {
		marker := marker
		t.Run(marker, func(t *testing.T) {
			envelope := stagingEnvelope(t)
			plan := stagingPlan(t, domain.ClientCursor, domain.PackageNative)
			delivery, err := (Stager{}).Stage(context.Background(), envelope, plan, "operation-marker-"+strings.ReplaceAll(filepath.Base(marker), ".", "x"), domain.CompatibilityHints{})
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, filepath.Join(delivery.StagingPath, marker), "user-owned\n")
			if err := (Stager{}).Verify(context.Background(), delivery.StagingPath, delivery.ArtifactDigest); err == nil || !strings.Contains(err.Error(), "excluded ownership marker") {
				t.Fatalf("verify error = %v", err)
			}
		})
	}
}

func stagingEnvelope(t *testing.T) domain.PackageEnvelope {
	t.Helper()
	root := filepath.Join(t.TempDir(), "snapshot")
	writeTestFile(t, filepath.Join(root, "plugin.json"), `{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name": "demo",
  "version": "1.0.0",
  "description": "Demo plugin",
  "extensions": {"cursor": {"enabled": true}}
}`)
	writeTestFile(t, filepath.Join(root, "mcp.json"), `{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers": {
    "notion": {"type": "streamable-http", "url": "https://mcp.notion.com/mcp"},
    "broken": {"type": "unknown"}
  }
}`)
	writeTestFile(t, filepath.Join(root, "skills", "good", "SKILL.md"), "---\nname: good\ndescription: Good skill\n---\nUse it.\n")
	writeTestFile(t, filepath.Join(root, "skills", "bad", "SKILL.md"), "invalid\n")
	writeTestFile(t, filepath.Join(root, "bin", "run"), "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(root, "bin", "run"), 0o755); err != nil {
		t.Fatal(err)
	}
	serverRaw := json.RawMessage(`{"type":"streamable-http","url":"https://mcp.notion.com/mcp"}`)
	return domain.PackageEnvelope{
		LoaderKind: domain.LoaderKindAgentPlugins,
		FormatID:   domain.FormatIDAgentPluginsV1,
		Manifest: domain.PluginManifest{
			Name: "demo", Version: "1.0.0", Description: "Demo plugin",
			Extensions: map[string]json.RawMessage{"cursor": json.RawMessage(`{"enabled":true}`)},
		},
		MCP: domain.MCPComponent{
			Present: true, Enabled: true,
			Servers: map[string]domain.MCPServer{
				"notion": {
					Name: "notion", Type: "streamable-http", Raw: serverRaw,
					Decoded: map[string]any{"type": "streamable-http", "url": "https://mcp.notion.com/mcp"},
				},
			},
		},
		Skills:       map[string]domain.Skill{"good": {Name: "good"}},
		Inventory:    domain.ComponentInventory{InvalidSkills: []string{"bad"}, InvalidMCPServer: []string{"broken"}},
		SnapshotRoot: root,
	}
}

func stagingPlan(t *testing.T, client domain.ClientID, mode domain.PackageMode) domain.DeliveryPlan {
	t.Helper()
	anchor := filepath.Join(t.TempDir(), "managed")
	target := filepath.Join(anchor, "clients", string(client))
	active := filepath.Join(target, "demo-0123456789ab")
	status := domain.PlanReady
	activation := domain.ActivationActive
	if client == domain.ClientCodex || client == domain.ClientChatGPT || client == domain.ClientKiro {
		status = domain.PlanManualActivationRequired
		activation = domain.ActivationManual
	}
	return domain.DeliveryPlan{
		ClientID: client, Scope: domain.ScopeUser, Status: status, PackageMode: mode,
		Activation: activation, Authentication: domain.AuthenticationNotChecked,
		Policy: domain.PolicyAllowed, Verification: domain.VerificationPackageValid,
		PhysicalArtifactID: "demo-0123456789ab", TargetAnchor: anchor, TargetRoot: target, ActivePath: active,
		Components: []domain.ComponentDecision{
			{Kind: domain.ComponentSkill, Name: "good", Support: supportFor(mode)},
			{Kind: domain.ComponentMCPServer, Name: "notion", Support: supportFor(mode)},
			{Kind: domain.ComponentExtension, Name: "cursor", Support: domain.SupportUnsupported},
		},
	}
}

func supportFor(mode domain.PackageMode) domain.SupportLevel {
	if mode == domain.PackageProjection {
		return domain.SupportProjected
	}
	return domain.SupportNative
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readObject(t *testing.T, path string) map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("path %s exists or could not be checked: %v", path, err)
	}
}

func assertExecutable(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("path %s is not executable", path)
	}
}
