package providers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/providers/nativeconfig"
)

func TestClineLifecycleInstallsUpdatesAndRemovesExactOwnedObjects(t *testing.T) {
	root := t.TempDir()
	configRoot := filepath.Join(root, ".cline")
	settings := filepath.Join(root, "isolated", "cline_mcp_settings.json")
	t.Setenv("CLINE_MCP_SETTINGS_PATH", settings)
	writeTestFile(t, settings, `{"theme":"night","mcpServers":{"foreign":{"command":"foreign"}}}`)

	active := filepath.Join(root, "managed", "demo")
	writeTestFile(t, filepath.Join(active, "skills", "guide", "SKILL.md"), "---\nname: guide\ndescription: Guide\n---\n")
	server := nativeconfig.Server{Type: "stdio", Command: "node", Args: []string{filepath.Join(active, "server.js")}}
	writeClineProjectionFixture(t, active, map[string]nativeconfig.Server{"docs": server})
	desired := clineFixtureObjects(t, configRoot, active, "guide", "docs", server)

	request := domain.ActivationRequest{
		Client: domain.DetectedClient{ClientID: domain.ClientCline, Status: domain.DetectionDetected, ConfigRoot: configRoot},
		Plan: domain.DeliveryPlan{ClientID: domain.ClientCline, ActivePath: active, Components: []domain.ComponentDecision{
			{Kind: domain.ComponentSkill, Name: "guide", Support: domain.SupportPrepared},
			{Kind: domain.ComponentMCPServer, Name: "docs", Support: domain.SupportPrepared},
		}},
		Delivery:     domain.StagedDelivery{ClientID: domain.ClientCline, OwnedBase: filepath.Dir(active), ActivePath: active, NativeObjects: desired},
		DeclaredName: "demo",
	}
	outcome, err := (Activator{}).Activate(context.Background(), request)
	if err != nil || outcome.Activation != domain.ActivationActive || outcome.Verification != domain.VerificationInstalled {
		t.Fatalf("activation = %+v, %v", outcome, err)
	}
	doc := readObject(t, settings)
	if doc["theme"] != "night" || doc["mcpServers"].(map[string]any)["foreign"] == nil {
		t.Fatalf("foreign config changed: %+v", doc)
	}
	transport := doc["mcpServers"].(map[string]any)["docs"].(map[string]any)["transport"].(map[string]any)
	if transport["type"] != "stdio" || transport["command"] != "node" {
		t.Fatalf("Cline modern transport = %+v", transport)
	}
	if _, err := os.Stat(filepath.Join(configRoot, "skills", "guide", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	request.VerifyOnly = true
	if _, err := (Activator{}).Activate(context.Background(), request); err != nil {
		t.Fatalf("verify-only: %v", err)
	}
	request.VerifyOnly = false
	request.Replacing = true
	request.PreviousNativeObjects = append([]domain.NativeObjectOwnership(nil), desired...)
	updatedServer := nativeconfig.Server{Type: "stdio", Command: "deno", Args: []string{filepath.Join(active, "server.ts")}}
	writeClineProjectionFixture(t, active, map[string]nativeconfig.Server{"docs": updatedServer})
	updated := clineFixtureObjects(t, configRoot, active, "guide", "docs", updatedServer)
	request.Delivery.NativeObjects = updated
	if _, err := (Activator{}).Activate(context.Background(), request); err != nil {
		t.Fatalf("update: %v", err)
	}
	doc = readObject(t, settings)
	transport = doc["mcpServers"].(map[string]any)["docs"].(map[string]any)["transport"].(map[string]any)
	if transport["command"] != "deno" {
		t.Fatalf("update was not applied: %+v", transport)
	}
	desired = updated

	remove := domain.DeactivationRequest{Client: request.Client, DeclaredName: "demo", CurrentActivation: domain.ActivationActive, Confirmed: true, NativeObjects: desired}
	removed, err := (Activator{}).Deactivate(context.Background(), remove)
	if err != nil || !removed.ExternalRemovalComplete || !removed.ArtifactRemovalAllowed {
		t.Fatalf("remove = %+v, %v", removed, err)
	}
	doc = readObject(t, settings)
	servers := doc["mcpServers"].(map[string]any)
	if servers["foreign"] == nil || servers["docs"] != nil || doc["theme"] != "night" {
		t.Fatalf("remove touched foreign config: %+v", doc)
	}
	if _, err := os.Lstat(filepath.Join(configRoot, "skills", "guide")); !os.IsNotExist(err) {
		t.Fatalf("managed skill survived removal: %v", err)
	}
}

func TestClineCollisionAndBusyLockLeaveNoPartialActivation(t *testing.T) {
	for _, test := range []struct {
		name string
		busy bool
	}{
		{name: "foreign collision"},
		{name: "busy compatible lock", busy: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			configRoot := filepath.Join(root, ".cline")
			settings := filepath.Join(root, "settings.json")
			t.Setenv("CLINE_MCP_SETTINGS_PATH", settings)
			foreign := `{"mcpServers":{"docs":{"transport":{"type":"stdio","command":"foreign"}}}}`
			if test.busy {
				foreign = `{"mcpServers":{}}`
				if err := os.Mkdir(settings+".lock", 0o700); err != nil {
					t.Fatal(err)
				}
			}
			writeTestFile(t, settings, foreign)
			active := filepath.Join(root, "managed", "demo")
			writeTestFile(t, filepath.Join(active, "skills", "guide", "SKILL.md"), "guide")
			server := nativeconfig.Server{Type: "stdio", Command: "node"}
			writeClineProjectionFixture(t, active, map[string]nativeconfig.Server{"docs": server})
			desired := clineFixtureObjects(t, configRoot, active, "guide", "docs", server)
			err := applyClineNativeMutation(configRoot, active, nil, desired)
			if err == nil {
				t.Fatal("unsafe activation unexpectedly succeeded")
			}
			if test.busy && !strings.Contains(err.Error(), "Cline native config lock") {
				t.Fatalf("wrong lock error: %v", err)
			}
			body, _ := os.ReadFile(settings)
			if string(body) != foreign {
				t.Fatalf("config changed on failure: %s", body)
			}
			if _, err := os.Lstat(filepath.Join(configRoot, "skills", "guide")); !os.IsNotExist(err) {
				t.Fatalf("partial skill survived: %v", err)
			}
		})
	}
}

func TestStagerBuildsClineNestedTransportAndOwnership(t *testing.T) {
	t.Setenv("CLINE_MCP_SETTINGS_PATH", "")
	t.Setenv("CLINE_DATA_DIR", "")
	envelope := stagingEnvelope(t)
	plan := stagingPlan(t, domain.ClientCline, domain.PackageNative)
	plan.NativeRegistryRoot = filepath.Join(t.TempDir(), ".cline")
	delivery, err := (Stager{}).Stage(context.Background(), envelope, plan, "cline-operation", domain.CompatibilityHints{})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := readClineProjection(delivery.StagingPath)
	if err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(t.TempDir(), "cline_mcp_settings.json")
	if _, err := nativeconfig.New().Apply(nativeconfig.Request{Paths: nativeconfig.Paths{JSON: settings}, Codec: nativeconfig.CodecCline, Action: nativeconfig.ActionAdd, Name: "notion", Server: projection.Servers["notion"]}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	projected := document["mcpServers"].(map[string]any)["notion"].(map[string]any)
	transport := projected["transport"].(map[string]any)
	if transport["type"] != "streamableHttp" || transport["url"] != "https://mcp.notion.com/mcp" {
		t.Fatalf("projection = %+v", projected)
	}
	var foundSkill, foundMCP bool
	for _, object := range delivery.NativeObjects {
		foundSkill = foundSkill || object.Kind == clineSkillObjectKind
		foundMCP = foundMCP || object.Kind == clineMCPObjectKind
	}
	if !foundSkill || !foundMCP {
		t.Fatalf("Cline ownership missing: %+v", delivery.NativeObjects)
	}
}

func TestClineTamperedReceiptFailsClosed(t *testing.T) {
	root := t.TempDir()
	configRoot := filepath.Join(root, ".cline")
	settings := filepath.Join(root, "settings.json")
	t.Setenv("CLINE_MCP_SETTINGS_PATH", settings)
	writeTestFile(t, settings, `{"mcpServers":{}}`)
	active := filepath.Join(root, "managed", "demo")
	server := nativeconfig.Server{Type: "stdio", Command: "node"}
	writeClineProjectionFixture(t, active, map[string]nativeconfig.Server{"docs": server})
	desired := clineFixtureObjects(t, configRoot, active, "", "docs", server)
	if err := applyClineNativeMutation(configRoot, active, nil, desired); err != nil {
		t.Fatal(err)
	}
	desired[0].ManagedDigest = "sha256:00"
	if err := verifyClineNativeObjects(configRoot, desired, false); !errors.Is(err, nativeconfig.ErrNotOwned) && !strings.Contains(err.Error(), "changed outside") {
		t.Fatalf("tamper was not rejected: %v", err)
	}
}

func TestClineRejectsRelativeSettingsOverrideBeforeMutation(t *testing.T) {
	t.Setenv("CLINE_MCP_SETTINGS_PATH", "relative/settings.json")
	envelope := stagingEnvelope(t)
	plan := stagingPlan(t, domain.ClientCline, domain.PackageNative)
	plan.NativeRegistryRoot = filepath.Join(t.TempDir(), ".cline")
	if _, err := (Stager{}).Stage(context.Background(), envelope, plan, "cline-relative", domain.CompatibilityHints{}); err == nil || !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("relative Cline override was accepted: %v", err)
	}
}

func writeClineProjectionFixture(t *testing.T, active string, servers map[string]nativeconfig.Server) {
	t.Helper()
	body, err := json.Marshal(clineProjection{Servers: servers})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(active, clineProjectionFile), string(body))
}

func clineFixtureObjects(t *testing.T, configRoot, active, skillName, serverName string, server nativeconfig.Server) []domain.NativeObjectOwnership {
	t.Helper()
	var objects []domain.NativeObjectOwnership
	if skillName != "" {
		digest, err := digestKiroSkillDirectory(filepath.Join(active, "skills", skillName))
		if err != nil {
			t.Fatal(err)
		}
		objects = append(objects, domain.NativeObjectOwnership{ObjectID: "cline-skill:" + skillName, Kind: clineSkillObjectKind, LogicalName: skillName,
			Path: filepath.Join(configRoot, "skills", skillName), SourceRelative: filepath.ToSlash(filepath.Join("skills", skillName)), ManagedDigest: digest, ProtectionClass: "managed"})
	}
	if serverName != "" {
		receipt, err := nativeconfig.DesiredReceipt(clineMCPSettingsPath(configRoot), nativeconfig.CodecCline, serverName, server, nativeconfig.Placeholders{})
		if err != nil {
			t.Fatal(err)
		}
		objects = append(objects, domain.NativeObjectOwnership{ObjectID: "cline-mcp:" + serverName, Kind: clineMCPObjectKind, LogicalName: serverName,
			Path: clineMCPSettingsPath(configRoot), SourceRelative: clineProjectionFile, ManagedDigest: receipt.Digest, ProtectionClass: "managed"})
	}
	return objects
}
