package providers

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	legacyports "github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type identityRunner struct {
	result   legacyports.CommandResult
	commands [][]string
}

func (runner *identityRunner) Run(_ context.Context, command legacyports.Command) (legacyports.CommandResult, error) {
	runner.commands = append(runner.commands, append([]string(nil), command.Argv...))
	return runner.result, nil
}

func TestNativeIdentityCursorReadsEveryAuthoritativeLocalManifest(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".cursor", "plugins", "local")
	writeIdentityFile(t, filepath.Join(root, "foreign-path", ".cursor-plugin", "plugin.json"), `{"name":"demo"}`)
	plan := identityPlan(root)
	observation, err := (NativeIdentityObserver{}).ObserveNativeIdentity(context.Background(), domain.DetectedClient{ClientID: domain.ClientCursor}, plan, nil)
	if err != nil || observation.State != domain.NativeIdentityUnmanaged {
		t.Fatalf("observation = %+v, err = %v", observation, err)
	}

	writeIdentityFile(t, filepath.Join(root, "foreign-path", ".cursor-plugin", "plugin.json"), `{"name":`)
	observation, err = (NativeIdentityObserver{}).ObserveNativeIdentity(context.Background(), domain.DetectedClient{ClientID: domain.ClientCursor}, plan, nil)
	if observation.State != domain.NativeIdentityIndeterminate || err == nil {
		t.Fatalf("malformed observation = %+v, err = %v", observation, err)
	}
}

func TestNativeIdentityQualifiedPreparedMarketplaceCoexistsOnlyWithPositiveNamespace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "prepared")
	writeIdentityFile(t, filepath.Join(root, "foreign", ".agents", "plugins", "marketplace.json"), `{"name":"foreign-market","plugins":[{"name":"demo"}]}`)
	plan := identityPlan(root)
	plan.NativeRegistryRoot = filepath.Join(t.TempDir(), "missing-codex-root")
	observation, err := (NativeIdentityObserver{}).ObserveNativeIdentity(context.Background(), domain.DetectedClient{ClientID: domain.ClientCodex}, plan, nil)
	if err != nil || observation.State != domain.NativeIdentityAbsent {
		t.Fatalf("qualified observation = %+v, err = %v", observation, err)
	}

	writeIdentityFile(t, filepath.Join(root, "unqualified", "plugin.json"), `{"name":"demo"}`)
	observation, err = (NativeIdentityObserver{}).ObserveNativeIdentity(context.Background(), domain.DetectedClient{ClientID: domain.ClientCodex}, plan, nil)
	if err != nil || observation.State != domain.NativeIdentityUnmanaged {
		t.Fatalf("unqualified observation = %+v, err = %v", observation, err)
	}
}

func TestNativeIdentityCodexUsesExactCLIRegistryIdentity(t *testing.T) {
	plan := identityPlan(filepath.Join(t.TempDir(), "prepared"))
	plan.NativeRegistryExecutable = "/test/bin/codex"
	marketplace := managedMarketplaceName(plan.PhysicalArtifactID)
	runner := &identityRunner{result: legacyports.CommandResult{Stdout: []byte(`{"installed":[{"pluginId":"demo@foreign","name":"demo","marketplaceName":"foreign","installed":true,"enabled":true}]}`)}}
	observation, err := (NativeIdentityObserver{Runner: runner}).ObserveNativeIdentity(context.Background(), domain.DetectedClient{ClientID: domain.ClientCodex}, plan, nil)
	if err != nil || observation.State != domain.NativeIdentityAbsent {
		t.Fatalf("foreign namespace observation = %+v, err = %v", observation, err)
	}
	if !reflect.DeepEqual(runner.commands, [][]string{{"/test/bin/codex", "plugin", "list", "--json"}}) {
		t.Fatalf("commands = %#v", runner.commands)
	}

	runner.result.Stdout = []byte(`{"installed":[{"pluginId":"demo@` + marketplace + `","name":"demo","marketplaceName":"` + marketplace + `","installed":true,"enabled":true}]}`)
	observation, err = (NativeIdentityObserver{Runner: runner}).ObserveNativeIdentity(context.Background(), domain.DetectedClient{ClientID: domain.ClientCodex}, plan, nil)
	if err != nil || observation.State != domain.NativeIdentityUnmanaged {
		t.Fatalf("occupied managed namespace observation = %+v, err = %v", observation, err)
	}
}

func TestNativeIdentityCodexManualModeReadsAuthoritativeConfig(t *testing.T) {
	configRoot := filepath.Join(t.TempDir(), ".codex")
	plan := identityPlan(filepath.Join(t.TempDir(), "prepared"))
	plan.NativeRegistryRoot = configRoot
	marketplace := managedMarketplaceName(plan.PhysicalArtifactID)
	writeIdentityFile(t, filepath.Join(configRoot, "config.toml"), "[plugins.\"demo@"+marketplace+"\"]\nenabled = true\n")

	observation, err := (NativeIdentityObserver{}).ObserveNativeIdentity(context.Background(), domain.DetectedClient{ClientID: domain.ClientCodex}, plan, nil)
	if err != nil || observation.State != domain.NativeIdentityUnmanaged {
		t.Fatalf("observation = %+v, err = %v", observation, err)
	}
}

func TestNativeIdentityCopilotAndVSCodeUseSharedAuthoritativeBackend(t *testing.T) {
	for _, clientID := range []domain.ClientID{domain.ClientCopilot, domain.ClientVSCode} {
		t.Run(string(clientID), func(t *testing.T) {
			plan := identityPlan(filepath.Join(t.TempDir(), "prepared"))
			plan.NativeRegistryExecutable = "/test/bin/copilot"
			marketplace := managedMarketplaceName(plan.PhysicalArtifactID)
			runner := &identityRunner{result: legacyports.CommandResult{Stdout: []byte("Installed plugins:\n  • demo@" + marketplace + " (v1.0.0)\n")}}
			observation, err := (NativeIdentityObserver{Runner: runner}).ObserveNativeIdentity(context.Background(), domain.DetectedClient{ClientID: clientID}, plan, nil)
			if err != nil || observation.State != domain.NativeIdentityUnmanaged {
				t.Fatalf("observation = %+v, err = %v", observation, err)
			}
			if !reflect.DeepEqual(runner.commands, [][]string{{"/test/bin/copilot", "plugin", "list"}}) {
				t.Fatalf("commands = %#v", runner.commands)
			}
		})
	}
}

func TestCopilotRegistryFindingRequiresExactNativeIdentity(t *testing.T) {
	marketplace := "agentplugins-demo-0123456789ab"
	tests := []struct {
		name string
		body string
		want registryFinding
	}{
		{name: "exact", body: "Installed plugins:\n  • demo@" + marketplace + " (v1.0.0)\n", want: registryExpected},
		{name: "unrelated", body: "Installed plugins:\n  • other@" + marketplace + " (v1.0.0)\n", want: registryClear},
		{name: "duplicate", body: "Installed plugins:\n  • demo@" + marketplace + " (v1.0.0)\n  • demo@" + marketplace + " (v1.0.0)\n", want: registryIndeterminate},
		{name: "absent_short", body: "No plugins installed.\n", want: registryClear},
		{name: "absent_official_1_0_80", body: "No plugins installed.\n\nUse 'copilot plugin install <source>' to install a plugin.\n", want: registryClear},
		{name: "unknown", body: "Copilot plugins are unavailable right now\n", want: registryIndeterminate},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := copilotRegistryFinding([]byte(test.body), "demo", marketplace, true); got != test.want {
				t.Fatalf("finding = %d, want %d", got, test.want)
			}
		})
	}
}

func TestNativeIdentityObservationExposesReceiptAndExactDiscovery(t *testing.T) {
	plan := identityPlan(filepath.Join(t.TempDir(), "prepared"))
	plan.NativeRegistryExecutable = "/test/bin/copilot"
	if err := os.MkdirAll(plan.ActivePath, 0o700); err != nil {
		t.Fatal(err)
	}
	writeIdentityFile(t, filepath.Join(plan.ActivePath, "plugin.json"), `{"name":"demo"}`)
	marketplace := managedMarketplaceName(plan.PhysicalArtifactID)
	digest := "sha256:owned"
	managed := &domain.ClientBinding{NativeObjects: []domain.NativeObjectOwnership{{Kind: "managed_package_directory", ManagedDigest: digest}}}
	runner := &identityRunner{result: legacyports.CommandResult{Stdout: []byte("Installed plugins:\n  • demo@" + marketplace + " (v1.0.0)\n")}}
	observation, err := (NativeIdentityObserver{Runner: runner, Stager: acceptingPackageVerifier{}}).ObserveNativeIdentity(context.Background(), domain.DetectedClient{ClientID: domain.ClientCopilot}, plan, managed)
	if err != nil || observation.State != domain.NativeIdentityManaged || !observation.ReceiptReconciled || !observation.NativeDiscoveryReconciled || observation.NativeDiscoveryState != domain.NativeIdentityManaged {
		t.Fatalf("observation = %+v, err = %v", observation, err)
	}
}

type acceptingPackageVerifier struct{}

func (acceptingPackageVerifier) Verify(context.Context, string, string) error { return nil }

func TestNativeIdentityManualRemoteAndUnknownSharedBackendsFailClosed(t *testing.T) {
	plan := identityPlan(filepath.Join(t.TempDir(), "prepared"))
	for _, test := range []struct {
		client domain.ClientID
		root   string
	}{
		{client: domain.ClientChatGPT},
		{client: domain.ClientCopilot, root: filepath.Join(t.TempDir(), ".copilot")},
		{client: domain.ClientVSCode, root: filepath.Join(t.TempDir(), ".copilot")},
	} {
		if test.root != "" {
			if err := os.MkdirAll(test.root, 0700); err != nil {
				t.Fatal(err)
			}
		}
		plan.NativeRegistryRoot = test.root
		observation, err := (NativeIdentityObserver{}).ObserveNativeIdentity(context.Background(), domain.DetectedClient{ClientID: test.client}, plan, nil)
		if err != nil || observation.State != domain.NativeIdentityIndeterminate {
			t.Fatalf("%s observation = %+v, err = %v", test.client, observation, err)
		}
	}
}

func TestNativeIdentityKiroReadsGlobalMCPRegistry(t *testing.T) {
	configRoot := filepath.Join(t.TempDir(), ".kiro")
	writeIdentityFile(t, filepath.Join(configRoot, "settings", "mcp.json"), `{"mcpServers":{"docs":{"command":"foreign"}}}`)
	plan := identityPlan(filepath.Join(t.TempDir(), "prepared"))
	plan.NativeRegistryRoot = configRoot
	plan.Components = []domain.ComponentDecision{{Kind: domain.ComponentMCPServer, Name: "docs", Support: domain.SupportNative}}
	observation, err := (NativeIdentityObserver{}).ObserveNativeIdentity(context.Background(), domain.DetectedClient{ClientID: domain.ClientKiro}, plan, nil)
	if err != nil || observation.State != domain.NativeIdentityUnmanaged {
		t.Fatalf("observation = %+v, err = %v", observation, err)
	}
}

func TestNativeIdentityKiroManualPowerAuthorizesOnlyLocalPreparation(t *testing.T) {
	configRoot := filepath.Join(t.TempDir(), ".kiro")
	plan := identityPlan(filepath.Join(t.TempDir(), "prepared"))
	plan.NativeRegistryRoot = configRoot
	plan.Components = []domain.ComponentDecision{{Kind: domain.ComponentSkill, Name: "docs", Support: domain.SupportNative}}

	observation, err := (NativeIdentityObserver{}).ObserveNativeIdentity(context.Background(), domain.DetectedClient{ClientID: domain.ClientKiro}, plan, nil)
	if err != nil || observation.State != domain.NativeIdentityAbsent {
		t.Fatalf("observation = %+v, err = %v", observation, err)
	}
}

func identityPlan(root string) domain.DeliveryPlan {
	artifact := "demo-0123456789ab"
	return domain.DeliveryPlan{
		DeclaredName:       "demo",
		PhysicalArtifactID: artifact,
		TargetRoot:         root,
		ActivePath:         filepath.Join(root, artifact),
	}
}

func writeIdentityFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
