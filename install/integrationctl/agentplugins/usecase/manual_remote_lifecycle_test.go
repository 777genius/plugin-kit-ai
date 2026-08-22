package usecase

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/providers"
)

func TestSignedChatGPTPreparationSupportsAddUpdateAndRepairWhileRemoteActivationIsPending(t *testing.T) {
	t.Parallel()
	service, store, _ := serviceFixture(t)
	service.NativeObserver = providers.NativeIdentityObserver{Stager: service.Stager}
	client := domain.DetectedClient{ClientID: domain.ClientChatGPT, DisplayName: "ChatGPT", Status: domain.DetectionNotDetected}

	add := signedChatGPTInput(t, client, "1.0.0", "sha256:chatgpt-v1", "sha256:chatgpt-manifest-v1")
	added, err := service.AddGroup(context.Background(), GroupInput{Targets: []AddInput{add}, OperationGroupID: "chatgpt-add", Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if added.Targets[0].Activation.Activation != domain.ActivationManual || added.Targets[0].Activation.Authentication != domain.AuthenticationPending || added.Targets[0].Activation.Verification == domain.VerificationInstalled {
		t.Fatalf("add claimed remote ChatGPT completion: %+v", added.Targets[0])
	}

	update := signedChatGPTInput(t, client, "2.0.0", "sha256:chatgpt-v2", "sha256:chatgpt-manifest-v2")
	updated, err := service.UpdateGroup(context.Background(), GroupInput{Targets: []AddInput{update}, CompatibilityChecks: []AddInput{update}, OperationGroupID: "chatgpt-update", Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Targets[0].Activation.Activation != domain.ActivationManual || updated.Targets[0].Activation.Verification == domain.VerificationInstalled {
		t.Fatalf("update claimed remote ChatGPT completion: %+v", updated.Targets[0])
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding := onlyBinding(state.Installations[0])
	if err := os.RemoveAll(binding.TargetLocator); err != nil {
		t.Fatal(err)
	}
	update.InstallationID = added.InstallationID
	repaired, err := service.RepairGroup(context.Background(), GroupInput{Targets: []AddInput{update}, OperationGroupID: "chatgpt-repair", Confirmed: true, Repair: true})
	if err != nil {
		t.Fatal(err)
	}
	if repaired.Targets[0].Activation.Activation != domain.ActivationManual || repaired.Targets[0].Activation.Verification == domain.VerificationInstalled {
		t.Fatalf("repair claimed remote ChatGPT completion: %+v", repaired.Targets[0])
	}
}

func TestNonMCPKiroPowerSupportsAddUpdateAndRepairWhileImportIsPending(t *testing.T) {
	t.Parallel()
	service, store, _ := serviceFixture(t)
	service.NativeObserver = providers.NativeIdentityObserver{Stager: service.Stager}
	client := domain.DetectedClient{ClientID: domain.ClientKiro, DisplayName: "Kiro", Status: domain.DetectionDetected, ConfigRoot: filepath.Join(t.TempDir(), ".kiro")}

	add := kiroPowerInput(t, client, "1.0.0", "sha256:kiro-v1", "sha256:kiro-manifest-v1")
	added, err := service.AddGroup(context.Background(), GroupInput{Targets: []AddInput{add}, OperationGroupID: "kiro-add", Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if added.Targets[0].Activation.Activation != domain.ActivationManual || added.Targets[0].Activation.Verification == domain.VerificationInstalled {
		t.Fatalf("Kiro add claimed manual import completion: %+v", added.Targets[0])
	}

	update := kiroPowerInput(t, client, "2.0.0", "sha256:kiro-v2", "sha256:kiro-manifest-v2")
	if _, err := service.UpdateGroup(context.Background(), GroupInput{Targets: []AddInput{update}, CompatibilityChecks: []AddInput{update}, OperationGroupID: "kiro-update", Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding := onlyBinding(state.Installations[0])
	if err := os.RemoveAll(binding.TargetLocator); err != nil {
		t.Fatal(err)
	}
	update.InstallationID = added.InstallationID
	repaired, err := service.RepairGroup(context.Background(), GroupInput{Targets: []AddInput{update}, OperationGroupID: "kiro-repair", Confirmed: true, Repair: true})
	if err != nil {
		t.Fatal(err)
	}
	if repaired.Targets[0].Activation.Activation != domain.ActivationManual || repaired.Targets[0].Activation.Verification == domain.VerificationInstalled {
		t.Fatalf("Kiro repair claimed manual import completion: %+v", repaired.Targets[0])
	}
}

func signedChatGPTInput(t *testing.T, client domain.DetectedClient, version, treeDigest, manifestDigest string) AddInput {
	t.Helper()
	input := addInput(t, client, "https://example.com/chatgpt")
	setEnvelopeVersion(t, &input.Envelope, version, treeDigest, manifestDigest)
	appBody := []byte(`{"apps":{"docs":{"id":"asdk_app_docs_123"}}}`)
	if err := os.WriteFile(filepath.Join(input.Envelope.SnapshotRoot, ".app.json"), appBody, 0o644); err != nil {
		t.Fatal(err)
	}
	mcpBody := []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"docs":{"type":"streamable-http","url":"https://example.test/mcp"}}}`)
	if err := os.WriteFile(filepath.Join(input.Envelope.SnapshotRoot, "mcp.json"), mcpBody, 0o644); err != nil {
		t.Fatal(err)
	}
	input.Envelope.App = domain.AppComponent{Present: true, Declared: true, Enabled: true, Raw: appBody, Bindings: map[string]domain.AppBinding{
		"docs": {Alias: "docs", ID: "asdk_app_docs_123", Raw: []byte(`{"id":"asdk_app_docs_123"}`)},
	}}
	input.Envelope.MCP = domain.MCPComponent{Present: true, Enabled: true, SchemaURI: domain.MCPSchemaV1, Raw: mcpBody, Servers: map[string]domain.MCPServer{
		"docs": {Name: "docs", Type: "streamable-http", Raw: []byte(`{"type":"streamable-http","url":"https://example.test/mcp"}`), Decoded: map[string]any{"type": "streamable-http", "url": "https://example.test/mcp"}},
	}}
	input.Envelope.Inventory.AppPresent = true
	input.Envelope.Inventory.AppBindings = []string{"docs"}
	input.Envelope.Inventory.MCPPresent = true
	input.Envelope.Inventory.MCPEnabled = true
	input.Envelope.Inventory.MCPServers = []string{"docs"}
	input.Envelope.CatalogEvidence = &domain.CatalogEvidence{SchemaVersion: 1, CatalogVersion: "directory-snapshot-1", Digest: "sha256:" + strings.Repeat("a", 64), Compatibility: map[string]domain.CatalogCompatibility{
		"chatgpt": {Package: "projected", Verification: "tested", Authentication: domain.AuthenticationRequirementRequired,
			AppBinding: &domain.CatalogAppBinding{AppKey: "docs", ID: "asdk_app_docs_123", MCPServer: "docs", MCPURL: "https://example.test/mcp"}},
	}}
	input.Hints.Compatibility = input.Envelope.CatalogEvidence.Compatibility
	return input
}

func kiroPowerInput(t *testing.T, client domain.DetectedClient, version, treeDigest, manifestDigest string) AddInput {
	t.Helper()
	input := addInput(t, client, "https://example.com/kiro")
	setEnvelopeVersion(t, &input.Envelope, version, treeDigest, manifestDigest)
	skillRoot := filepath.Join(input.Envelope.SnapshotRoot, "skills", "docs")
	if err := os.MkdirAll(skillRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("---\nname: docs\ndescription: docs\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	input.Envelope.Skills = map[string]domain.Skill{"docs": {Name: "docs", Description: "docs", RelativePath: "skills/docs/SKILL.md", Raw: []byte("---\nname: docs\ndescription: docs\n---\n")}}
	input.Envelope.Inventory.Skills = []string{"docs"}
	return input
}
