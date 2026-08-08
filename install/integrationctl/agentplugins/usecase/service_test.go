package usecase

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/dirswap"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/processlock"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/statev2"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	clientplanner "github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/planner"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/providers"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/transaction"
)

func TestAddDryRunAndUnconfirmedPlanDoNotMutateStateOrClient(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"dry_run", "unconfirmed"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			service, store, client := serviceFixture(t)
			input := addInput(t, client, "https://example.com/one")
			input.DryRun = mode == "dry_run"
			input.Confirmed = false
			result, err := service.Add(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "unconfirmed" && !result.RequiresConfirmation {
				t.Fatal("unconfirmed mutation did not require confirmation")
			}
			state, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if len(state.Installations) != 0 {
				t.Fatalf("state mutated: %+v", state)
			}
			if _, err := os.Lstat(result.Plan.ActivePath); !os.IsNotExist(err) {
				t.Fatalf("client path mutated: %v", err)
			}
		})
	}
}

func TestAddCommitsCursorPackageReceiptAndLeavesDiscoveryManual(t *testing.T) {
	t.Parallel()
	service, store, client := serviceFixture(t)
	input := addInput(t, client, "https://example.com/one")
	input.Confirmed = true
	result, err := service.Add(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Mutated || result.Activation.Activation != domain.ActivationManual || result.Activation.Verification != domain.VerificationPackageValid {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(result.Plan.ActivePath, "plugin.json")); err != nil {
		t.Fatalf("installed package: %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Installations) != 1 {
		t.Fatalf("installations = %+v", state.Installations)
	}
	clientState := onlyBinding(state.Installations[0])
	if clientState.Materialization != domain.MaterializationMaterialized || clientState.Activation != domain.ActivationManual || clientState.Verification != domain.VerificationPackageValid {
		t.Fatalf("client state = %+v", clientState)
	}
	if len(clientState.Receipts) != 1 || clientState.Receipts[0].Phase != transaction.ReceiptPhaseCommitted {
		t.Fatalf("receipts = %+v", clientState.Receipts)
	}
	if clientState.PackageRevision == nil || clientState.PackageRevision.TreeDigest != input.Envelope.TreeDigest {
		t.Fatalf("package revision = %+v", clientState.PackageRevision)
	}
	if _, err := service.Add(context.Background(), input); err == nil || !strings.Contains(err.Error(), "use update") {
		t.Fatalf("duplicate add error = %v", err)
	}
}

func TestAddKeepsCodexProjectionInManualActivationState(t *testing.T) {
	t.Parallel()
	service, store, _ := serviceFixture(t)
	client := domain.DetectedClient{
		ClientID: domain.ClientCodex, Status: domain.DetectionDetected,
		ConfigRoot: filepath.Join(t.TempDir(), ".codex"),
	}
	input := addInput(t, client, "https://example.com/openai")
	input.Confirmed = true
	result, err := service.Add(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Activation.Activation != domain.ActivationManual || result.Activation.Verification != domain.VerificationPackageValid {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(result.Plan.ActivePath, ".codex-plugin", "plugin.json")); err != nil {
		t.Fatal(err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	clientState := onlyBinding(state.Installations[0])
	if clientState.Activation != domain.ActivationManual || clientState.Verification != domain.VerificationPackageValid {
		t.Fatalf("client state = %+v", clientState)
	}
}

func TestAddKeepsOAuthPackageInAuthPendingState(t *testing.T) {
	t.Parallel()
	service, store, client := serviceFixture(t)
	input := addInput(t, client, "https://example.com/oauth")
	input.Envelope.MCP = domain.MCPComponent{
		Present: true, Enabled: true,
		Servers: map[string]domain.MCPServer{"notion": {Name: "notion", Type: "streamable-http", Decoded: map[string]any{"url": "https://mcp.example.com"}}},
	}
	input.Hints = domain.CompatibilityHints{OpenAIMCPAuth: map[string]domain.OpenAIMCPAuthHint{
		"notion": {OAuthResource: "https://mcp.example.com"},
	}}
	input.Confirmed = true
	result, err := service.Add(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Activation.Authentication != domain.AuthenticationPending {
		t.Fatalf("activation = %+v", result.Activation)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if onlyBinding(state.Installations[0]).Authentication != domain.AuthenticationPending {
		t.Fatalf("client = %+v", onlyBinding(state.Installations[0]))
	}
}

func TestAddRejectsSameNativePluginNameFromDifferentSources(t *testing.T) {
	t.Parallel()
	service, store, client := serviceFixture(t)
	first := addInput(t, client, "https://example.com/one")
	first.Confirmed = true
	if _, err := service.Add(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := addInput(t, client, "https://example.com/two")
	second.Confirmed = true
	second.InstallationID = "00000000-0000-4000-8000-000000000002"
	second.OperationID = "operation-two"
	if _, err := service.Add(context.Background(), second); err == nil || !strings.Contains(err.Error(), "native client name collision") {
		t.Fatalf("collision error = %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Installations) != 1 {
		t.Fatalf("collision mutated state: %+v", state.Installations)
	}
}

func TestUpdateReplacesExistingArtifactAndPreservesReceiptHistory(t *testing.T) {
	t.Parallel()
	service, store, client := serviceFixture(t)
	first := addInput(t, client, "https://example.com/one")
	first.Confirmed = true
	if _, err := service.Add(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := addInput(t, client, "https://example.com/one")
	second.Confirmed = true
	second.OperationID = "operation-update"
	second.Envelope.Manifest.Version = "2.0.0"
	second.Envelope.TreeDigest = "sha256:source-tree-v2"
	second.Envelope.ManifestDigest = "sha256:manifest-v2"
	manifestPath := filepath.Join(second.Envelope.SnapshotRoot, "plugin.json")
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(strings.Replace(string(body), "1.0.0", "2.0.0", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := service.Update(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.Installations[0].Package.Version != "2.0.0" {
		t.Fatalf("package = %+v", state.Installations[0].Package)
	}
	clientState := onlyBinding(state.Installations[0])
	if len(clientState.Receipts) != 2 || clientState.Receipts[1].BeforeDigest != clientState.Receipts[0].AfterDigest {
		t.Fatalf("receipts = %+v", clientState.Receipts)
	}
	installed, err := os.ReadFile(filepath.Join(result.Plan.ActivePath, "plugin.json"))
	if err != nil || !strings.Contains(string(installed), "2.0.0") {
		t.Fatalf("installed manifest = %q, err=%v", installed, err)
	}
}

func TestUpdateRefusesUserEditedManagedPackage(t *testing.T) {
	t.Parallel()
	service, _, client := serviceFixture(t)
	first := addInput(t, client, "https://example.com/one")
	first.Confirmed = true
	installed, err := service.Add(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installed.Plan.ActivePath, "user-note.txt"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	update := addInput(t, client, "https://example.com/one")
	update.Confirmed = true
	update.OperationID = "operation-update-edited"
	if _, err := service.Update(context.Background(), update); err == nil || !strings.Contains(err.Error(), "refusing silent update") {
		t.Fatalf("update error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(installed.Plan.ActivePath, "user-note.txt")); err != nil {
		t.Fatalf("user file was lost: %v", err)
	}
}

func TestUpdateRejectsSameVersionDigestConflictAndDowngrade(t *testing.T) {
	t.Parallel()
	for name, version := range map[string]string{"same_version_conflict": "1.0.0", "downgrade": "0.9.0"} {
		name, version := name, version
		t.Run(name, func(t *testing.T) {
			service, _, client := serviceFixture(t)
			first := addInput(t, client, "https://example.com/one")
			first.Confirmed = true
			if _, err := service.Add(context.Background(), first); err != nil {
				t.Fatal(err)
			}
			update := addInput(t, client, "https://example.com/one")
			update.Confirmed = true
			update.Envelope.Manifest.Version = version
			update.Envelope.TreeDigest = "sha256:different-tree"
			update.Envelope.ManifestDigest = "sha256:different-manifest"
			_, err := service.Update(context.Background(), update)
			if err == nil {
				t.Fatal("unsafe update succeeded")
			}
			want := "supply-chain conflict"
			if name == "downgrade" {
				want = "downgrade"
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("update error = %v", err)
			}
		})
	}
}

func TestUpdateRejectsIncomparableOpaqueVersionTransitions(t *testing.T) {
	t.Parallel()
	for name, versions := range map[string][2]string{
		"stable_to_opaque": {"1.0.0", "release-candidate"},
		"opaque_to_stable": {"release-candidate", "1.0.0"},
	} {
		name, versions := name, versions
		t.Run(name, func(t *testing.T) {
			service, _, client := serviceFixture(t)
			first := addInput(t, client, "https://example.com/opaque")
			first.Envelope.Manifest.Version = versions[0]
			first.Confirmed = true
			if _, err := service.Add(context.Background(), first); err != nil {
				t.Fatal(err)
			}
			update := addInput(t, client, "https://example.com/opaque")
			update.Envelope.Manifest.Version = versions[1]
			update.Envelope.TreeDigest = "sha256:opaque-transition"
			update.Envelope.ManifestDigest = "sha256:opaque-transition-manifest"
			update.Confirmed = true
			if _, err := service.Update(context.Background(), update); err == nil || !strings.Contains(err.Error(), "incomparable package version transition") {
				t.Fatalf("update error = %v", err)
			}
		})
	}
}

func TestExistingSourceRejectsManifestRenameWithoutChangingAnyClient(t *testing.T) {
	t.Parallel()
	service, store, cursor := serviceFixture(t)
	first := addInput(t, cursor, "https://example.com/identity")
	first.Confirmed = true
	cursorResult, err := service.Add(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	codex := domain.DetectedClient{
		ClientID: domain.ClientCodex, Status: domain.DetectionDetected,
		ConfigRoot: filepath.Join(t.TempDir(), ".codex"),
	}
	second := addInput(t, codex, "https://example.com/identity")
	second.Confirmed = true
	second.OperationID = "operation-identity-codex"
	codexResult, err := service.Add(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	rename := addInput(t, cursor, "https://example.com/identity")
	rename.Envelope.Manifest.Name = "renamed-demo"
	rename.Envelope.Manifest.Version = "2.0.0"
	rename.Envelope.TreeDigest = "sha256:renamed-tree"
	rename.Envelope.ManifestDigest = "sha256:renamed-manifest"
	rename.Confirmed = true
	if _, err := service.Update(context.Background(), rename); err == nil || !strings.Contains(err.Error(), "refuse package identity change") {
		t.Fatalf("rename update error = %v", err)
	}
	thirdClient := domain.DetectedClient{
		ClientID: domain.ClientKiro, Status: domain.DetectionDetected,
		ConfigRoot: filepath.Join(t.TempDir(), ".kiro"),
	}
	rename.Client = thirdClient
	if _, err := service.Add(context.Background(), rename); err == nil || !strings.Contains(err.Error(), "refuse package identity change") {
		t.Fatalf("rename add-target error = %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Installations) != 1 || len(state.Installations[0].Clients) != 2 || state.Installations[0].DeclaredName != "demo" || state.Installations[0].Package.Version != "1.0.0" {
		t.Fatalf("state changed after rename attempts: %+v", state)
	}
	for _, path := range []string{cursorResult.Plan.ActivePath, codexResult.Plan.ActivePath} {
		if _, err := os.Stat(filepath.Join(path, "plugin.json")); err != nil {
			t.Fatalf("existing artifact changed: %v", err)
		}
	}
}

func TestUpdateReturnsNoChangeWithoutMutation(t *testing.T) {
	t.Parallel()
	service, store, client := serviceFixture(t)
	first := addInput(t, client, "https://example.com/one")
	first.Confirmed = true
	if _, err := service.Add(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	result, err := service.Update(context.Background(), addInput(t, client, "https://example.com/one"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.NoChange || result.Mutated || result.RequiresConfirmation {
		t.Fatalf("update result = %+v", result)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyBinding(state.Installations[0]).Receipts) != 1 {
		t.Fatalf("no-op update wrote a receipt: %+v", onlyBinding(state.Installations[0]).Receipts)
	}
}

func TestLifecycleConvergenceRetriesOnlyNativeCopilotBackends(t *testing.T) {
	t.Parallel()
	manual := domain.ClientBinding{Activation: domain.ActivationManual, Verification: domain.VerificationPackageValid}
	for _, client := range []domain.ClientID{domain.ClientCursor, domain.ClientCodex, domain.ClientKiro} {
		manual.ClientID = string(client)
		if !lifecycleConverged(manual) {
			t.Fatalf("manual lifecycle for %s should preserve normal no-change behavior", client)
		}
	}
	for _, client := range []domain.ClientID{domain.ClientCopilot, domain.ClientVSCode} {
		manual.ClientID = string(client)
		if lifecycleConverged(manual) {
			t.Fatalf("manual lifecycle for %s should retry native activation", client)
		}
		manual.Activation = domain.ActivationActive
		manual.Verification = domain.VerificationInstalled
		if !lifecycleConverged(manual) {
			t.Fatalf("installed lifecycle for %s should be converged", client)
		}
		manual.Activation = domain.ActivationManual
		manual.Verification = domain.VerificationPackageValid
	}
}

func TestCopilotAndVSCodeShareOneNativeBackend(t *testing.T) {
	t.Parallel()
	installation := domain.Installation{Clients: map[string]domain.ClientBinding{
		"copilot": {
			ClientID: string(domain.ClientCopilot), Materialization: domain.MaterializationMaterialized,
		},
	}}
	if err := rejectSharedCopilotBackendDuplicate(installation, domain.ClientVSCode); err == nil || !strings.Contains(err.Error(), "available in both") {
		t.Fatalf("shared backend error = %v", err)
	}
	if err := rejectSharedCopilotBackendDuplicate(installation, domain.ClientCopilot); err != nil {
		t.Fatalf("same target was rejected: %v", err)
	}
	if !sameNativeBackend(domain.ClientCopilot, domain.ClientVSCode) || sameNativeBackend(domain.ClientCursor, domain.ClientVSCode) {
		t.Fatal("native backend family mapping is incorrect")
	}
}

func TestMultiClientUpdateConvergesEachClientRevision(t *testing.T) {
	t.Parallel()
	service, store, cursor := serviceFixture(t)
	codex := domain.DetectedClient{
		ClientID: domain.ClientCodex, Status: domain.DetectionDetected,
		ConfigRoot: filepath.Join(t.TempDir(), ".codex"),
	}
	firstCursor := addInput(t, cursor, "https://example.com/shared")
	firstCursor.Confirmed = true
	firstCursor.OperationID = "operation-cursor-v1"
	if _, err := service.Add(context.Background(), firstCursor); err != nil {
		t.Fatal(err)
	}
	firstCodex := addInput(t, codex, "https://example.com/shared")
	firstCodex.Confirmed = true
	firstCodex.OperationID = "operation-codex-v1"
	if _, err := service.Add(context.Background(), firstCodex); err != nil {
		t.Fatal(err)
	}

	updateCursor := addInput(t, cursor, "https://example.com/shared")
	setEnvelopeVersion(t, &updateCursor.Envelope, "2.0.0", "sha256:tree-v2", "sha256:manifest-v2")
	updateCursor.Confirmed = true
	updateCursor.OperationID = "operation-cursor-v2"
	if result, err := service.Update(context.Background(), updateCursor); err != nil || result.NoChange {
		t.Fatalf("cursor update = %+v, err=%v", result, err)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if revisionForClient(t, state.Installations[0], domain.ClientCodex).Version != "1.0.0" {
		t.Fatalf("Codex revision advanced before its update: %+v", state.Installations[0].Clients)
	}

	updateCodex := addInput(t, codex, "https://example.com/shared")
	setEnvelopeVersion(t, &updateCodex.Envelope, "2.0.0", "sha256:tree-v2", "sha256:manifest-v2")
	updateCodex.Confirmed = true
	updateCodex.OperationID = "operation-codex-v2"
	if result, err := service.Update(context.Background(), updateCodex); err != nil || result.NoChange || !result.Mutated {
		t.Fatalf("codex update = %+v, err=%v", result, err)
	}
	state, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, clientID := range []domain.ClientID{domain.ClientCursor, domain.ClientCodex} {
		if revision := revisionForClient(t, state.Installations[0], clientID); revision.Version != "2.0.0" || revision.TreeDigest != "sha256:tree-v2" {
			t.Fatalf("%s revision = %+v", clientID, revision)
		}
	}
}

func TestPartialMultiClientUpdateFailurePreservesIndependentRevisions(t *testing.T) {
	t.Parallel()
	service, store, cursor := serviceFixture(t)
	codex := domain.DetectedClient{
		ClientID: domain.ClientCodex, Status: domain.DetectionDetected,
		ConfigRoot: filepath.Join(t.TempDir(), ".codex"),
	}

	firstCursor := addInput(t, cursor, "https://example.com/partial-update")
	firstCursor.Confirmed = true
	firstCursor.OperationID = "operation-partial-cursor-v1"
	cursorInstalled, err := service.Add(context.Background(), firstCursor)
	if err != nil {
		t.Fatal(err)
	}
	firstCodex := addInput(t, codex, "https://example.com/partial-update")
	firstCodex.Confirmed = true
	firstCodex.OperationID = "operation-partial-codex-v1"
	codexInstalled, err := service.Add(context.Background(), firstCodex)
	if err != nil {
		t.Fatal(err)
	}

	updateCursor := addInput(t, cursor, "https://example.com/partial-update")
	setEnvelopeVersion(t, &updateCursor.Envelope, "2.0.0", "sha256:partial-tree-v2", "sha256:partial-manifest-v2")
	updateCursor.Confirmed = true
	updateCursor.OperationID = "operation-partial-cursor-v2"
	if result, updateErr := service.Update(context.Background(), updateCursor); updateErr != nil || !result.Mutated {
		t.Fatalf("cursor update = %+v, err=%v", result, updateErr)
	}

	userFile := filepath.Join(codexInstalled.Plan.ActivePath, "user-note.txt")
	if err := os.WriteFile(userFile, []byte("preserve me"), 0o644); err != nil {
		t.Fatal(err)
	}
	updateCodex := addInput(t, codex, "https://example.com/partial-update")
	setEnvelopeVersion(t, &updateCodex.Envelope, "2.0.0", "sha256:partial-tree-v2", "sha256:partial-manifest-v2")
	updateCodex.Confirmed = true
	updateCodex.OperationID = "operation-partial-codex-v2"
	if _, err := service.Update(context.Background(), updateCodex); err == nil || !strings.Contains(err.Error(), "refusing silent update") {
		t.Fatalf("codex update error = %v", err)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	installation := state.Installations[0]
	if revision := revisionForClient(t, installation, domain.ClientCursor); revision.Version != "2.0.0" || revision.TreeDigest != "sha256:partial-tree-v2" {
		t.Fatalf("cursor revision = %+v", revision)
	}
	if revision := revisionForClient(t, installation, domain.ClientCodex); revision.Version != "1.0.0" || revision.TreeDigest != "sha256:source-tree" {
		t.Fatalf("codex revision advanced after failed update: %+v", revision)
	}

	cursorManifest, err := os.ReadFile(filepath.Join(cursorInstalled.Plan.ActivePath, "plugin.json"))
	if err != nil || !strings.Contains(string(cursorManifest), "2.0.0") {
		t.Fatalf("cursor manifest = %q, err=%v", cursorManifest, err)
	}
	codexManifest, err := os.ReadFile(filepath.Join(codexInstalled.Plan.ActivePath, "plugin.json"))
	if err != nil || !strings.Contains(string(codexManifest), "1.0.0") {
		t.Fatalf("codex manifest = %q, err=%v", codexManifest, err)
	}
	if body, err := os.ReadFile(userFile); err != nil || string(body) != "preserve me" {
		t.Fatalf("user file = %q, err=%v", body, err)
	}
}

func TestAddNewTargetEnforcesExistingPackageIdentity(t *testing.T) {
	t.Parallel()
	service, _, cursor := serviceFixture(t)
	first := addInput(t, cursor, "https://example.com/shared")
	first.Confirmed = true
	if _, err := service.Add(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	codex := domain.DetectedClient{ClientID: domain.ClientCodex, Status: domain.DetectionDetected, ConfigRoot: filepath.Join(t.TempDir(), ".codex")}
	conflict := addInput(t, codex, "https://example.com/shared")
	conflict.Envelope.TreeDigest = "sha256:changed-same-version"
	conflict.Envelope.ManifestDigest = "sha256:changed-manifest"
	conflict.Confirmed = true
	if _, err := service.Add(context.Background(), conflict); err == nil || !strings.Contains(err.Error(), "supply-chain conflict") {
		t.Fatalf("same-version add error = %v", err)
	}
}

func TestRemoveCommitsReceiptAndLeavesBindingAbsent(t *testing.T) {
	t.Parallel()
	service, store, client := serviceFixture(t)
	add := addInput(t, client, "https://example.com/one")
	add.Confirmed = true
	installed, err := service.Add(context.Background(), add)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := service.Remove(context.Background(), RemoveInput{
		Selector: installed.InstallationID, Client: client, Scope: domain.ScopeUser,
		Confirmed: true, OperationID: "operation-remove",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !removed.Mutated {
		t.Fatalf("remove result = %+v", removed)
	}
	if _, err := os.Lstat(installed.Plan.ActivePath); !os.IsNotExist(err) {
		t.Fatalf("managed package still exists: %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding := onlyBinding(state.Installations[0])
	if binding.Materialization != domain.MaterializationAbsent || binding.Activation != domain.ActivationNotRequired || binding.Verification != domain.VerificationNotRun {
		t.Fatalf("binding = %+v", binding)
	}
	if len(binding.Receipts) != 2 || binding.Receipts[1].MutationType != "directory_remove" || binding.Receipts[1].Phase != transaction.ReceiptPhaseCommitted {
		t.Fatalf("receipts = %+v", binding.Receipts)
	}
}

func TestRemovePlanExposesExternalUninstallAndPreservesCodexArtifactUntilAcknowledged(t *testing.T) {
	t.Parallel()
	service, store, _ := serviceFixture(t)
	client := domain.DetectedClient{
		ClientID: domain.ClientCodex, Status: domain.DetectionDetected,
		ConfigRoot: filepath.Join(t.TempDir(), ".codex"),
	}
	add := addInput(t, client, "https://example.com/copied")
	add.Confirmed = true
	installed, err := service.Add(context.Background(), add)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []RemoveInput{
		{Selector: installed.InstallationID, Client: client, Scope: domain.ScopeUser, DryRun: true},
		{Selector: installed.InstallationID, Client: client, Scope: domain.ScopeUser, Confirmed: true},
	} {
		result, err := service.Remove(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		if result.Mutated || result.Deactivation.ArtifactRemovalAllowed || len(result.Deactivation.UserActions) != 1 {
			t.Fatalf("blocked remove result = %+v", result)
		}
		if _, err := os.Stat(installed.Plan.ActivePath); err != nil {
			t.Fatalf("managed artifact changed before acknowledgement: %v", err)
		}
	}
	removed, err := service.Remove(context.Background(), RemoveInput{
		Selector: installed.InstallationID, Client: client, Scope: domain.ScopeUser,
		Confirmed: true, ExternalUninstalled: true, OperationID: "operation-remove-codex",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !removed.Mutated || !removed.Deactivation.ExternalRemovalComplete {
		t.Fatalf("acknowledged remove result = %+v", removed)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if onlyBinding(state.Installations[0]).Materialization != domain.MaterializationAbsent {
		t.Fatalf("binding remained materialized: %+v", onlyBinding(state.Installations[0]))
	}
}

func TestRemoveRefusesUserEditedManagedPackage(t *testing.T) {
	t.Parallel()
	service, store, client := serviceFixture(t)
	add := addInput(t, client, "https://example.com/one")
	add.Confirmed = true
	installed, err := service.Add(context.Background(), add)
	if err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(installed.Plan.ActivePath, "user-note.txt")
	if err := os.WriteFile(userFile, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Remove(context.Background(), RemoveInput{
		Selector: installed.InstallationID, Client: client, Scope: domain.ScopeUser,
		Confirmed: true, OperationID: "operation-remove-edited",
	}); err == nil || !strings.Contains(err.Error(), "refusing silent removal") {
		t.Fatalf("remove error = %v", err)
	}
	if _, err := os.Stat(userFile); err != nil {
		t.Fatalf("user file was removed: %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if onlyBinding(state.Installations[0]).Materialization != domain.MaterializationMaterialized {
		t.Fatalf("failed remove changed materialization: %+v", onlyBinding(state.Installations[0]))
	}
}

func TestRemoveRetainsManagedPackageContainingExcludedOwnershipMarkers(t *testing.T) {
	t.Parallel()
	for _, marker := range []string{filepath.Join(".git", "config"), filepath.Join("nested", ".plugin-kit-ai.lock")} {
		marker := marker
		t.Run(marker, func(t *testing.T) {
			service, store, client := serviceFixture(t)
			add := addInput(t, client, "https://example.com/marker-"+filepath.Base(marker))
			add.Confirmed = true
			installed, err := service.Add(context.Background(), add)
			if err != nil {
				t.Fatal(err)
			}
			userFile := filepath.Join(installed.Plan.ActivePath, marker)
			if err := os.MkdirAll(filepath.Dir(userFile), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(userFile, []byte("keep me"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Remove(context.Background(), RemoveInput{
				Selector: installed.InstallationID, Client: client, Scope: domain.ScopeUser,
				Confirmed: true, OperationID: "remove-marker-" + strings.ReplaceAll(filepath.Base(marker), ".", "x"),
			}); err == nil || !strings.Contains(err.Error(), "excluded ownership marker") {
				t.Fatalf("remove error = %v", err)
			}
			if _, err := os.Stat(userFile); err != nil {
				t.Fatalf("ownership marker was removed: %v", err)
			}
			state, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if onlyBinding(state.Installations[0]).Materialization != domain.MaterializationMaterialized {
				t.Fatalf("failed remove changed state: %+v", onlyBinding(state.Installations[0]))
			}
		})
	}
}

func TestRemoveRejectsTamperedPersistedTarget(t *testing.T) {
	t.Parallel()
	service, store, client := serviceFixture(t)
	add := addInput(t, client, "https://example.com/one")
	add.Confirmed = true
	installed, err := service.Add(context.Background(), add)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "unrelated")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for key, binding := range state.Installations[0].Clients {
		binding.TargetLocator = outside
		state.Installations[0].Clients[key] = binding
	}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Remove(context.Background(), RemoveInput{
		Selector: installed.InstallationID, Client: client, Scope: domain.ScopeUser,
		Confirmed: true, OperationID: "operation-remove-tampered",
	}); err == nil || !strings.Contains(err.Error(), "untrusted persisted target") {
		t.Fatalf("remove error = %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("unrelated target was removed: %v", err)
	}
}

func serviceFixture(t *testing.T) (Service, statev2.Store, domain.DetectedClient) {
	t.Helper()
	root := t.TempDir()
	store := statev2.Store{Path: filepath.Join(root, "state", "state-v2.json")}
	managed := filepath.Join(root, "managed")
	client := domain.DetectedClient{
		ClientID: domain.ClientCursor, Status: domain.DetectionDetected,
		ConfigRoot: filepath.Join(root, "home", ".cursor"),
	}
	stager := providers.Stager{}
	targetPlanner := clientplanner.Planner{ManagedRoot: managed}
	return Service{
		StateStore: store,
		Planner:    targetPlanner,
		Targets:    targetPlanner,
		Stager:     stager,
		Activator:  providers.Activator{},
		Lock:       processlock.Lock{Path: filepath.Join(root, "state", "mutation.lock")},
		Kernel: transaction.Kernel{
			Directory: dirswap.Manager{JournalDir: filepath.Join(root, "state", "operations-v2")},
		},
		Now: func() time.Time { return time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC) },
	}, store, client
}

func addInput(t *testing.T, client domain.DetectedClient, canonicalSource string) AddInput {
	t.Helper()
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name": "demo",
  "version": "1.0.0",
  "description": "Demo plugin"
}`
	if err := os.WriteFile(filepath.Join(snapshot, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return AddInput{
		Envelope: domain.PackageEnvelope{
			LoaderKind: domain.LoaderKindAgentPlugins, FormatID: domain.FormatIDAgentPluginsV1,
			SchemaURI: domain.PluginSchemaV1, SchemaVersion: "1.0.0",
			Manifest: domain.PluginManifest{Name: "demo", Version: "1.0.0", Description: "Demo plugin"},
			Source: domain.SourceIdentity{
				RequestedSource: canonicalSource, CanonicalSource: canonicalSource, ResolvedRevision: "abc123",
			},
			TreeDigest: "sha256:source-tree", ManifestDigest: "sha256:manifest", SnapshotRoot: snapshot,
		},
		Client: client, Scope: domain.ScopeUser,
		InstallationID: "00000000-0000-4000-8000-000000000001",
		OperationID:    "operation-one",
	}
}

func onlyBinding(installation domain.Installation) domain.ClientBinding {
	for _, client := range installation.Clients {
		return client
	}
	return domain.ClientBinding{}
}

func setEnvelopeVersion(t *testing.T, envelope *domain.PackageEnvelope, version, treeDigest, manifestDigest string) {
	t.Helper()
	envelope.Manifest.Version = version
	envelope.TreeDigest = treeDigest
	envelope.ManifestDigest = manifestDigest
	path := filepath.Join(envelope.SnapshotRoot, "plugin.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(body), "1.0.0", version, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func revisionForClient(t *testing.T, installation domain.Installation, clientID domain.ClientID) domain.ClientPackageRevision {
	t.Helper()
	for _, client := range installation.Clients {
		if client.ClientID == string(clientID) {
			if client.PackageRevision == nil {
				t.Fatalf("%s package revision is missing", clientID)
			}
			return *client.PackageRevision
		}
	}
	t.Fatalf("%s binding is missing", clientID)
	return domain.ClientPackageRevision{}
}
