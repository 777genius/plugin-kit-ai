package usecase

import (
	"context"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func TestRebindRequiresAllManagedTargetsRemovedThenChangesOnlyBinding(t *testing.T) {
	t.Parallel()
	service, store, client := serviceFixture(t)
	first := addInput(t, client, "https://example.com/one")
	first.Confirmed = true
	installed, err := service.Add(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	second := addInput(t, client, "https://example.com/two").Envelope
	blocked, err := service.Rebind(context.Background(), BindingChangeInput{Selector: installed.InstallationID, Envelope: second})
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Plan.CanApply || len(blocked.Plan.Blockers) == 0 || blocked.Plan.PluginDataDecision != "not_transferred" {
		t.Fatalf("blocked plan = %+v", blocked.Plan)
	}
	if _, err := service.Rebind(context.Background(), BindingChangeInput{Selector: installed.InstallationID, Envelope: second, Confirmed: true}); err == nil || !strings.Contains(err.Error(), "remove") {
		t.Fatalf("confirmed blocked rebind error = %v", err)
	}
	if _, err := service.Remove(context.Background(), RemoveInput{
		Selector: installed.InstallationID, Client: client, Scope: domain.ScopeUser,
		Confirmed: true, OperationID: "operation-remove-before-rebind",
	}); err != nil {
		t.Fatal(err)
	}
	result, err := service.Rebind(context.Background(), BindingChangeInput{
		Selector: installed.InstallationID, Envelope: second, Confirmed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Mutated || !result.Plan.CanApply {
		t.Fatalf("rebind result = %+v", result)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.Installations[0].Source.SourceBindingID != domain.ComputeSourceBindingID(second.Source) || state.Installations[0].Source.TreeDigest != second.TreeDigest {
		t.Fatalf("source binding = %+v", state.Installations[0].Source)
	}
	if binding := onlyBinding(state.Installations[0]); binding.Materialization != domain.MaterializationAbsent || len(binding.Receipts) != 2 {
		t.Fatalf("rebind changed lifecycle history: %+v", binding)
	}
}

func TestMigrateFormatRequiresExplicitConfirmationAndPreservesNoPluginData(t *testing.T) {
	t.Parallel()
	service, store, client := serviceFixture(t)
	legacy := domain.Installation{
		InstallationID: "00000000-0000-4000-8000-000000000009", DeclaredName: "legacy-demo",
		Source: domain.SourceBinding{
			SourceBindingID: "src_legacy", RequestedSource: "legacy-demo", CanonicalSource: "https://example.com/legacy", ResolvedRevision: "abc", TreeDigest: "sha256:legacy-tree",
		},
		Package: domain.PackageBinding{
			LoaderKind: domain.LoaderKindLegacy, FormatID: domain.FormatIDLegacyV1,
			SchemaURI: "plugin.yaml/v1", DeclaredName: "legacy-demo", Version: "1.0.0", ManifestDigest: "sha256:legacy-manifest",
		},
		Clients: map[string]domain.ClientBinding{}, CreatedAt: "2026-08-08T12:00:00Z", UpdatedAt: "2026-08-08T12:00:00Z",
	}
	if err := store.Save(domain.StateFileV2{SchemaVersion: domain.StateSchemaVersion, Installations: []domain.Installation{legacy}}); err != nil {
		t.Fatal(err)
	}
	standard := addInput(t, client, "https://example.com/standard").Envelope
	planned, err := service.MigrateFormat(context.Background(), BindingChangeInput{Selector: legacy.InstallationID, Envelope: standard})
	if err != nil {
		t.Fatal(err)
	}
	if !planned.RequiresConfirmation || planned.Mutated || planned.Plan.OldFormat.LoaderKind != domain.LoaderKindLegacy || planned.Plan.PluginDataDecision != "not_transferred" {
		t.Fatalf("migration plan = %+v", planned)
	}
	result, err := service.MigrateFormat(context.Background(), BindingChangeInput{Selector: legacy.InstallationID, Envelope: standard, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Mutated {
		t.Fatalf("migration result = %+v", result)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.Installations[0].Package.LoaderKind != domain.LoaderKindAgentPlugins || state.Installations[0].Package.FormatID != domain.FormatIDAgentPluginsV1 {
		t.Fatalf("migrated package = %+v", state.Installations[0].Package)
	}
}
