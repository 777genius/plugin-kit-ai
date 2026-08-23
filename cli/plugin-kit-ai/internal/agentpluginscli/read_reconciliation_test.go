package agentpluginscli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func TestInfoReconcilesExactCopilotIdentityWithoutMutationOrPathLeak(t *testing.T) {
	fixture := newCLIFixture(t, nil)
	client := domain.DetectedClient{ClientID: domain.ClientCopilot, DisplayName: "GitHub Copilot CLI", Status: domain.DetectionDetected,
		Version: "1.0.80", ExecutablePath: "/test/bin/copilot", ConfigRoot: filepath.Join(fixture.root, "home", ".copilot")}
	detector := &observedProbingDetector{clients: []domain.DetectedClient{client}}
	fixture.app.Detector = detector
	fixture.app.Lifecycle.NativeObserver = reconciledNativeObserver{}

	installationID := "00000000-0000-4000-8000-000000000001"
	physical := domain.ComputePhysicalArtifactID("demo", installationID)
	target, err := fixture.app.Lifecycle.Targets.ResolveTarget(context.Background(), client, domain.ScopeUser, physical)
	if err != nil {
		t.Fatal(err)
	}
	bindingID := domain.ComputeClientBindingID(installationID, string(domain.ClientCopilot), string(domain.ScopeUser), target.ActivePath)
	digest := "sha256:owned"
	binding := domain.ClientBinding{
		ClientBindingID: bindingID, ClientID: string(domain.ClientCopilot), Scope: string(domain.ScopeUser), TargetLocator: target.ActivePath,
		PhysicalArtifact: physical, Materialization: domain.MaterializationMaterialized, Activation: domain.ActivationActive,
		Authentication: domain.AuthenticationNotRequired, Policy: domain.PolicyAllowed, Verification: domain.VerificationInstalled,
		NativeObjects: []domain.NativeObjectOwnership{{ObjectID: "package:copilot:" + physical, Kind: "managed_package_directory", LogicalName: "demo", ManagedDigest: digest}},
		Receipts:      []domain.MutationReceipt{{OperationID: "op-0000000000000001", Sequence: 1, ClientBindingID: bindingID, AfterDigest: digest, Phase: "committed"}},
	}
	state := domain.StateFileV2{SchemaVersion: domain.StateSchemaVersion, Installations: []domain.Installation{{
		InstallationID: installationID, DeclaredName: "demo",
		Source:  domain.SourceBinding{SourceBindingID: "src_demo", RequestedSource: "demo", CanonicalSource: "https://example.test/demo", ResolvedRevision: strings.Repeat("a", 40), TreeDigest: "sha256:tree"},
		Package: domain.PackageBinding{LoaderKind: domain.LoaderKindAgentPlugins, FormatID: domain.FormatIDAgentPluginsV1, SchemaURI: domain.PluginSchemaV1, DeclaredName: "demo", ManifestDigest: "sha256:manifest"},
		Clients: map[string]domain.ClientBinding{bindingID: binding},
	}}}
	if err := fixture.store.Save(state); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(fixture.store.Path)
	if err != nil {
		t.Fatal(err)
	}
	stdout, _, err := fixture.execute(false, "info", "demo", "--target", "copilot", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(fixture.store.Path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("info mutated state: %v", err)
	}
	if strings.Contains(stdout, fixture.root) || strings.Contains(stdout, client.ExecutablePath) || strings.Contains(stdout, client.ConfigRoot) {
		t.Fatalf("info leaked a local path: %s", stdout)
	}
	var output struct {
		SchemaVersion int `json:"schema_version"`
		Data          struct {
			Clients []struct {
				ReceiptReconciled         bool   `json:"receipt_reconciled"`
				NativeDiscoveryReconciled bool   `json:"native_discovery_reconciled"`
				NativeIdentityState       string `json:"native_identity_state"`
				ClientVersion             string `json:"client_version"`
				NativeDiscoveryEvidence   struct {
					Basis            string `json:"basis"`
					VersionOperation struct {
						Argv                  []string `json:"argv"`
						ObservedClientVersion string   `json:"observed_client_version"`
					} `json:"version_operation"`
					DiscoveryOperation struct {
						Argv       []string `json:"argv"`
						Discovered bool     `json:"discovered"`
						ProductID  string   `json:"product_id"`
					} `json:"discovery_operation"`
				} `json:"native_discovery_evidence"`
			} `json:"clients"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatal(err)
	}
	if output.SchemaVersion != 1 || len(output.Data.Clients) != 1 {
		t.Fatalf("output = %s", stdout)
	}
	got := output.Data.Clients[0]
	if !got.ReceiptReconciled || !got.NativeDiscoveryReconciled || got.NativeIdentityState != "managed" || got.ClientVersion != "1.0.80" {
		t.Fatalf("reconciliation = %+v", got)
	}
	evidence := got.NativeDiscoveryEvidence
	if evidence.Basis != "native_client_command" || evidence.VersionOperation.ObservedClientVersion != "1.0.80" ||
		!evidence.DiscoveryOperation.Discovered || !strings.HasPrefix(evidence.DiscoveryOperation.ProductID, "demo@agentplugins-") ||
		strings.Join(evidence.VersionOperation.Argv, " ") != "copilot --version" || strings.Join(evidence.DiscoveryOperation.Argv, " ") != "copilot plugin list" {
		t.Fatalf("native discovery evidence = %+v", evidence)
	}
	if detector.probeCalls != 1 || detector.readOnlyCalls != 0 {
		t.Fatalf("detector calls = read-only:%d probe:%d", detector.readOnlyCalls, detector.probeCalls)
	}
}

func TestInfoInvalidOwnershipReceiptIsInconclusive(t *testing.T) {
	binding := domain.ClientBinding{ClientBindingID: "binding", ClientID: "copilot", PhysicalArtifact: "demo-0123456789ab"}
	if hasReconciledOwnershipReceipt(binding, binding.PhysicalArtifact, "demo") {
		t.Fatal("missing ownership receipt reconciled")
	}
}

type reconciledNativeObserver struct{}

func (reconciledNativeObserver) ObserveNativeIdentity(context.Context, domain.DetectedClient, domain.DeliveryPlan, *domain.ClientBinding) (domain.NativeIdentityObservation, error) {
	return domain.NativeIdentityObservation{State: domain.NativeIdentityManaged, ReceiptReconciled: true, NativeDiscoveryReconciled: true, NativeDiscoveryState: domain.NativeIdentityManaged}, nil
}
