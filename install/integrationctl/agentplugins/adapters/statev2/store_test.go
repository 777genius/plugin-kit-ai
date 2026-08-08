package statev2

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func TestStoreRoundTripAndDuplicateDeclaredNames(t *testing.T) {
	t.Parallel()
	store := Store{Path: filepath.Join(t.TempDir(), "state-v2.json")}
	state := domain.StateFileV2{
		SchemaVersion: domain.StateSchemaVersion,
		Installations: []domain.Installation{
			validInstallation("00000000-0000-4000-8000-000000000001", "src_one", "demo-000000000001"),
			validInstallation("00000000-0000-4000-8000-000000000002", "src_two", "demo-000000000002"),
		},
	}
	if err := store.Save(state); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Installations) != 2 || loaded.Installations[0].DeclaredName != "demo" || loaded.Installations[1].DeclaredName != "demo" {
		t.Fatalf("installations = %+v", loaded.Installations)
	}
	info, err := os.Stat(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %o", info.Mode().Perm())
	}
}

func TestStoreFailsClosedOnFutureOrUnknownState(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"future":  `{"schema_version":3,"installations":[]}`,
		"unknown": `{"schema_version":2,"installations":[],"future":true}`,
	} {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state-v2.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := (Store{Path: path}).Load(); err == nil {
				t.Fatal("invalid future state accepted")
			}
		})
	}
}

func TestStoreRejectsDuplicateInstallationIdentity(t *testing.T) {
	t.Parallel()
	installation := validInstallation("00000000-0000-4000-8000-000000000001", "src_one", "demo-000000000001")
	state := domain.StateFileV2{SchemaVersion: domain.StateSchemaVersion, Installations: []domain.Installation{installation, installation}}
	err := (Store{Path: filepath.Join(t.TempDir(), "state-v2.json")}).Save(state)
	if err == nil || !strings.Contains(err.Error(), "duplicate installation_id") {
		t.Fatalf("save error = %v", err)
	}
}

func TestStoreRejectsDuplicateReceiptOperationIDsAcrossClients(t *testing.T) {
	t.Parallel()
	installations := []domain.Installation{
		validInstallation("00000000-0000-4000-8000-000000000001", "src_one", "demo-000000000001"),
		validInstallation("00000000-0000-4000-8000-000000000002", "src_two", "demo-000000000002"),
	}
	for installationIndex := range installations {
		for key, client := range installations[installationIndex].Clients {
			client.Receipts = []domain.MutationReceipt{{
				OperationID: "duplicate-op", Sequence: 1, MutationType: "directory_swap",
				ClientBindingID: client.ClientBindingID, Phase: "committed",
			}}
			installations[installationIndex].Clients[key] = client
		}
	}
	state := domain.StateFileV2{SchemaVersion: domain.StateSchemaVersion, Installations: installations}
	err := (Store{Path: filepath.Join(t.TempDir(), "state-v2.json")}).Save(state)
	if err == nil || !strings.Contains(err.Error(), "duplicate receipt operation_id") {
		t.Fatalf("save error = %v", err)
	}
}

func TestStoreRejectsUnknownLifecycleState(t *testing.T) {
	t.Parallel()
	installation := validInstallation("00000000-0000-4000-8000-000000000001", "src_one", "demo-000000000001")
	for key, client := range installation.Clients {
		client.Activation = domain.ActivationState("future_activation_state")
		installation.Clients[key] = client
	}
	state := domain.StateFileV2{SchemaVersion: domain.StateSchemaVersion, Installations: []domain.Installation{installation}}
	err := (Store{Path: filepath.Join(t.TempDir(), "state-v2.json")}).Save(state)
	if err == nil || !strings.Contains(err.Error(), "unknown lifecycle state") {
		t.Fatalf("save error = %v", err)
	}
}

func validInstallation(installationID, sourceID, physicalID string) domain.Installation {
	clientID := domain.ComputeClientBindingID(installationID, "codex", "user", "test-home")
	return domain.Installation{
		InstallationID: installationID,
		DeclaredName:   "demo",
		Source: domain.SourceBinding{
			SourceBindingID:  sourceID,
			RequestedSource:  "demo",
			CanonicalSource:  "https://example.com/demo",
			ResolvedRevision: "abc123",
			TreeDigest:       "sha256:tree",
		},
		Package: domain.PackageBinding{
			LoaderKind:     domain.LoaderKindAgentPlugins,
			FormatID:       domain.FormatIDAgentPluginsV1,
			SchemaURI:      domain.PluginSchemaV1,
			DeclaredName:   "demo",
			ManifestDigest: "sha256:manifest",
		},
		Clients: map[string]domain.ClientBinding{
			clientID: {
				ClientBindingID:  clientID,
				ClientID:         "codex",
				Scope:            "user",
				TargetLocator:    "test-home",
				PhysicalArtifact: physicalID,
				Materialization:  domain.MaterializationMaterialized,
				Activation:       domain.ActivationPrepared,
				Authentication:   domain.AuthenticationNotRequired,
				Policy:           domain.PolicyAllowed,
				Verification:     domain.VerificationPackageValid,
			},
		},
	}
}
