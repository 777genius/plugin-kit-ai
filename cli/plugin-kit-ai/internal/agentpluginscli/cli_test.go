package agentpluginscli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/dirswap"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/locks"
	processadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/process"
	sourceadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/source"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/loader"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/processlock"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/specregistry"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/statemigration"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/statev2"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/ports"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/providers"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/usecase"
)

func TestHelpKeepsAutomationConfirmationFlagOutOfUserFlow(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, nil)
	stdout, _, err := fixture.execute(true, "--help")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout, "--yes") {
		t.Fatalf("user-facing help exposed the automation-only flag: %s", stdout)
	}
}

func TestAddListAndInfoProduceVersionedPathRedactedJSON(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	stdout, _, err := fixture.execute(false, "add", plugin, "--target", "cursor", "--yes", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	assertVersionedJSON(t, stdout, "add")
	if strings.Contains(stdout, fixture.root) {
		t.Fatalf("add JSON leaked sandbox path: %s", stdout)
	}
	stdout, _, err = fixture.execute(false, "list", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	assertVersionedJSON(t, stdout, "list")
	if strings.Contains(stdout, fixture.root) {
		t.Fatalf("list JSON leaked sandbox path: %s", stdout)
	}
	stdout, _, err = fixture.execute(false, "info", "demo", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	assertVersionedJSON(t, stdout, "info")
	if strings.Contains(stdout, fixture.root) {
		t.Fatalf("info JSON leaked sandbox path: %s", stdout)
	}
	stdout, _, err = fixture.execute(false, "doctor", "demo", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	assertVersionedJSON(t, stdout, "doctor")
	if strings.Contains(stdout, fixture.root) {
		t.Fatalf("doctor JSON leaked sandbox path: %s", stdout)
	}
}

func TestNonInteractiveAddRequiresOneExplicitTargetAndYes(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{
		fixtureClient(t, domain.ClientCursor), fixtureClient(t, domain.ClientCodex),
	})
	plugin := writeCLIPlugin(t)
	for name, args := range map[string][]string{
		"missing_target": {"add", plugin, "--yes"},
		"missing_yes":    {"add", plugin, "--target", "cursor"},
	} {
		name, args := name, args
		t.Run(name, func(t *testing.T) {
			if _, _, err := fixture.execute(false, args...); err == nil {
				t.Fatal("unsafe non-interactive add succeeded")
			}
			state, err := fixture.store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if len(state.Installations) != 0 {
				t.Fatalf("state mutated: %+v", state)
			}
		})
	}
}

func TestDryRunAndDoctorAreStrictlyReadOnly(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	stdout, _, err := fixture.execute(false, "add", plugin, "--dry-run", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	assertVersionedJSON(t, stdout, "add")
	if _, err := os.Lstat(fixture.store.Path); !os.IsNotExist(err) {
		t.Fatalf("dry-run created state: %v", err)
	}
	stdout, _, err = fixture.execute(false, "doctor", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	assertVersionedJSON(t, stdout, "doctor")
	if _, err := os.Lstat(fixture.store.Path); !os.IsNotExist(err) {
		t.Fatalf("doctor created state: %v", err)
	}
	if _, err := os.Lstat(fixture.operations); !os.IsNotExist(err) {
		t.Fatalf("doctor created operation journal: %v", err)
	}
}

func TestDoctorDiagnosesManagedDirectoryChangesWithRecovery(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	if _, _, err := fixture.execute(false, "add", plugin, "--target", "cursor", "--yes", "--format", "json"); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding := onlyCLIClient(state.Installations[0])
	if err := os.WriteFile(filepath.Join(binding.TargetLocator, "unexpected.txt"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := fixture.execute(false, "doctor", "demo", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		Data struct {
			Findings []doctorFinding `json:"findings"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, finding := range output.Data.Findings {
		if finding.Status == "degraded" && finding.RecoveryAction == "" {
			t.Fatalf("degraded finding has no recovery: %+v", finding)
		}
		if finding.Code == "managed_directory_changed" {
			found = true
			if finding.InstallationName != "demo" || finding.InstallationID == "" || finding.ClientID != "cursor" {
				t.Fatalf("managed finding scope = %+v", finding)
			}
			if finding.RecoveryAction != "run `agentplugins repair "+finding.InstallationID+" --target cursor`" {
				t.Fatalf("managed finding recovery = %q", finding.RecoveryAction)
			}
		}
	}
	if !found {
		t.Fatalf("managed integrity finding missing: %+v", output.Data.Findings)
	}
}

func TestDoctorRecoveryIncludesProjectScopeAndExactTarget(t *testing.T) {
	t.Parallel()
	installation := domain.Installation{InstallationID: "00000000-0000-4000-8000-000000000001", DeclaredName: "demo"}
	binding := domain.ClientBinding{ClientID: "cursor", Scope: string(domain.ScopeProject)}
	if got := repairAction(installation, binding); got != "run `agentplugins repair 00000000-0000-4000-8000-000000000001 --target cursor --scope project`" {
		t.Fatalf("project repair action = %q", got)
	}
}

func TestRepairSourcePinsSelectedClientRevision(t *testing.T) {
	t.Parallel()
	installation := domain.Installation{Source: domain.SourceBinding{
		Repository: "acme/plugins", PackageSubpath: "plugins/demo", ResolvedRevision: "moving-head",
	}}
	binding := domain.ClientBinding{PackageRevision: &domain.ClientPackageRevision{ResolvedRevision: "client-commit"}}
	got, err := repairSource(installation, binding)
	if err != nil {
		t.Fatal(err)
	}
	if got != "github:acme/plugins@client-commit//plugins/demo" {
		t.Fatalf("repair source = %q", got)
	}
	binding.PackageRevision.ResolvedRevision = ""
	if _, err := repairSource(installation, binding); err == nil || !strings.Contains(err.Error(), "moving source") {
		t.Fatalf("unpinned repair error = %v", err)
	}
}

func TestDoctorScopesPendingRecoveryAndRendersIdentity(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	if _, _, err := fixture.execute(false, "add", plugin, "--target", "cursor", "--yes"); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for key, binding := range state.Installations[0].Clients {
		binding.Authentication = domain.AuthenticationPending
		state.Installations[0].Clients[key] = binding
	}
	if err := fixture.store.Save(state); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := fixture.execute(false, "doctor", "demo")
	if err != nil {
		t.Fatal(err)
	}
	installationID := state.Installations[0].InstallationID
	for _, expected := range []string{
		"Installation: demo (" + installationID + ")",
		"Client: cursor",
		"complete the displayed external activation step, then rerun the same `agentplugins add ... --target cursor` command",
		"complete the displayed external authentication step, then rerun the same `agentplugins add ... --target cursor` command",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("doctor output omitted %q:\n%s", expected, stdout)
		}
	}
	if strings.Contains(stdout, "then rerun doctor") {
		t.Fatalf("pending lifecycle recovery incorrectly recommends doctor:\n%s", stdout)
	}
}

func TestDoctorBlocksAutomaticMutationForMissingPhysicalIdentity(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	if _, _, err := fixture.execute(false, "add", plugin, "--target", "cursor", "--yes"); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for key, binding := range state.Installations[0].Clients {
		binding.PhysicalArtifact = ""
		state.Installations[0].Clients[key] = binding
	}
	findings := doctorFindings(context.Background(), fixture.app, fixture.app.Detector.(staticDetector).clients, state, nil, &state.Installations[0])
	var found doctorFinding
	for _, finding := range findings {
		if finding.Code == "managed_target_unverifiable" {
			found = finding
		}
	}
	if found.Code == "" || !strings.Contains(found.RecoveryAction, "automatic mutation is intentionally blocked") {
		t.Fatalf("missing identity recovery = %+v", findings)
	}
	if strings.Contains(found.RecoveryAction, "update") || strings.Contains(found.RecoveryAction, "remove") {
		t.Fatalf("blocked identity recovery invented a refusing mutation: %+v", found)
	}
}

func TestDoctorDoesNotRequireCopilotCLIForVSCodeBinding(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientVSCode)})
	plugin := writeCLIPlugin(t)
	if _, _, err := fixture.execute(false, "add", plugin, "--target", "vscode", "--yes"); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := fixture.execute(false, "doctor", "demo", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout, "copilot_cli_missing") {
		t.Fatalf("VS Code binding incorrectly required Copilot CLI: %s", stdout)
	}
}

func TestDoctorDoesNotCallVerifierFailureChangedContent(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	if _, _, err := fixture.execute(false, "add", plugin, "--target", "cursor", "--yes"); err != nil {
		t.Fatal(err)
	}
	fixture.app.Stager = failingVerifyStager{err: errors.New("temporary verifier unavailable")}
	stdout, _, err := fixture.execute(false, "doctor", "demo", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "managed_integrity_check_failed") || strings.Contains(stdout, "managed_directory_changed") {
		t.Fatalf("verification infrastructure error was mislabeled: %s", stdout)
	}
}

func TestDoctorSkipsClientWarningsForAbsentBinding(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	if _, _, err := fixture.execute(false, "add", plugin, "--target", "cursor", "--yes"); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for key, binding := range state.Installations[0].Clients {
		binding.Materialization = domain.MaterializationAbsent
		state.Installations[0].Clients[key] = binding
	}
	findings := doctorFindings(context.Background(), fixture.app, nil, state, nil, &state.Installations[0])
	for _, finding := range findings {
		if finding.ClientID != "" {
			t.Fatalf("absent binding produced client finding: %+v", finding)
		}
	}
}

func TestHumanCodexFlowNeverClaimsPreparedPackageIsInstalled(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCodex)})
	plugin := writeCLIPlugin(t)
	stdout, _, err := fixture.execute(true, "add", plugin, "--target", "codex", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Package prepared") || strings.Contains(stdout, "Installed and verified") {
		t.Fatalf("human output overclaimed activation: %s", stdout)
	}
	if strings.Count(stdout, "Next:") != 1 || !strings.Contains(stdout, fixture.root) || !strings.Contains(stdout, "verify") {
		t.Fatalf("human output must contain one path-bearing verification step: %s", stdout)
	}
}

func TestRepeatedAddResumesManualLifecycleWithoutAnotherReceipt(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	if _, _, err := fixture.execute(false, "add", plugin, "--target", "cursor", "--yes"); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := fixture.execute(false, "add", plugin, "--target", "cursor", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(stdout, "Next:") != 1 || !strings.Contains(stdout, "verify") || !strings.Contains(stdout, "plugins/local") {
		t.Fatalf("resume output = %q", stdout)
	}
	state, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if receipts := onlyCLIClient(state.Installations[0]).Receipts; len(receipts) != 1 {
		t.Fatalf("resume created another materialization receipt: %+v", receipts)
	}
}

func TestInitialInteractiveAddPromptsForApplyBeforeLifecycle(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	stdout, _, err := fixture.executeInput(true, "n\n", "add", plugin, "--target", "cursor")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Apply this plan? [y/N]") || strings.Contains(stdout, "Have you completed activation") || strings.Contains(stdout, "authentication, or reviewed") {
		t.Fatalf("initial prompt order = %q", stdout)
	}
	state, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Installations) != 0 {
		t.Fatalf("declined plan materialized state: %+v", state)
	}
}

func TestInteractiveActivationYesAuthNoKeepsIndependentResumableState(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	stdout, _, err := fixture.executeInput(true, "y\ny\nn\n", "add", plugin, "--target", "cursor")
	if err != nil {
		t.Fatal(err)
	}
	apply := strings.Index(stdout, "Apply this plan? [y/N]")
	activation := strings.Index(stdout, "Have you completed activation")
	auth := strings.Index(stdout, "Have you completed required authentication")
	if apply < 0 || activation <= apply || auth <= activation {
		t.Fatalf("interactive prompt order = %q", stdout)
	}
	if strings.Count(stdout, "Next:") != 1 || !strings.Contains(stdout, "Activation is user-attested") {
		t.Fatalf("interactive next action or attestation output = %q", stdout)
	}
	state, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding := onlyCLIClient(state.Installations[0])
	if binding.Activation != domain.ActivationActive || binding.Verification != domain.VerificationInstalled || binding.Authentication != domain.AuthenticationNotChecked {
		t.Fatalf("declined auth state = %+v", binding)
	}
}

func TestCLICompletionFlagsRequirePriorMaterialization(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	_, _, err := fixture.execute(false, "add", plugin, "--target", "cursor", "--yes", "--activation-complete", "--auth-complete")
	if err == nil || !strings.Contains(err.Error(), "already materialized") {
		t.Fatalf("fresh completion error = %v", err)
	}
}

func TestRepairExplicitlyRestoresMissingManagedDirectory(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	if _, _, err := fixture.execute(false, "add", plugin, "--target", "cursor", "--yes"); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	target := onlyCLIClient(state.Installations[0]).TargetLocator
	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := fixture.execute(false, "repair", "demo", "--target", "cursor", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "repaired from the exact installed revision") {
		t.Fatalf("repair output = %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(target, "plugin.json")); err != nil {
		t.Fatalf("repaired package: %v", err)
	}
}

func TestChatGPTTargetIsNotAliasedToCodexCLI(t *testing.T) {
	t.Parallel()
	if got := normalizeTarget("chatgpt"); got == domain.ClientCodex {
		t.Fatalf("ChatGPT GUI target was aliased to Codex CLI: %s", got)
	}
}

func TestHumanOutputNeverCallsAuthPendingPackageInstalled(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	err := renderAddResult(&output, "human", domain.PackageEnvelope{Manifest: domain.PluginManifest{Name: "oauth-demo"}}, usecase.AddResult{
		Mutated: true,
		Activation: domain.ActivationOutcome{
			Activation: domain.ActivationActive, Authentication: domain.AuthenticationPending,
			Verification: domain.VerificationInstalled,
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "Installed") || !strings.Contains(output.String(), "Authentication is pending") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestMultipleDetectedClientsAreNeverSelectedByYesAlone(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{
		fixtureClient(t, domain.ClientCursor), fixtureClient(t, domain.ClientCodex),
	})
	plugin := writeCLIPlugin(t)
	if _, _, err := fixture.execute(true, "add", plugin, "--yes"); err == nil || !strings.Contains(err.Error(), "choose exactly one") {
		t.Fatalf("multiple-client error = %v", err)
	}
}

func TestUpdateAndRemoveCompleteLifecycleWithVersionedRedactedJSON(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	if _, _, err := fixture.execute(false, "add", plugin, "--target", "cursor", "--yes", "--format", "json"); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(plugin, "plugin.json")
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(strings.Replace(string(body), `"version": "1.0.0"`, `"version": "2.0.0"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := fixture.execute(false, "update", "demo", "--target", "cursor", "--yes", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	assertVersionedJSON(t, stdout, "update")
	if strings.Contains(stdout, fixture.root) {
		t.Fatalf("update JSON leaked sandbox path: %s", stdout)
	}
	state, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding := onlyCLIClient(state.Installations[0])
	if state.Installations[0].Package.Version != "2.0.0" || len(binding.Receipts) != 2 {
		t.Fatalf("updated state = %+v", state.Installations[0])
	}
	stdout, _, err = fixture.execute(false, "remove", "demo", "--target", "cursor", "--yes", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	assertVersionedJSON(t, stdout, "remove")
	if strings.Contains(stdout, fixture.root) {
		t.Fatalf("remove JSON leaked sandbox path: %s", stdout)
	}
	state, err = fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding = onlyCLIClient(state.Installations[0])
	if binding.Materialization != domain.MaterializationAbsent || len(binding.Receipts) != 3 {
		t.Fatalf("removed state = %+v", state.Installations[0])
	}
}

func TestNonInteractiveUpdateAndRemoveRequireExplicitTargetAndYes(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	if _, _, err := fixture.execute(false, "add", plugin, "--target", "cursor", "--yes"); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"update_missing_target": {"update", "demo", "--yes"},
		"update_missing_yes":    {"update", "demo", "--target", "cursor"},
		"remove_missing_target": {"remove", "demo", "--yes"},
		"remove_missing_yes":    {"remove", "demo", "--target", "cursor"},
	} {
		name, args := name, args
		t.Run(name, func(t *testing.T) {
			if _, _, err := fixture.execute(false, args...); err == nil {
				t.Fatal("unsafe non-interactive mutation succeeded")
			}
		})
	}
	state, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if binding := onlyCLIClient(state.Installations[0]); binding.Materialization != domain.MaterializationMaterialized || len(binding.Receipts) != 1 {
		t.Fatalf("unsafe command mutated state: %+v", binding)
	}
}

func TestRebindIsPlanFirstAndRequiresRemovedTargets(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	first := writeCLIPlugin(t)
	second := writeCLIPlugin(t)
	if _, _, err := fixture.execute(false, "add", first, "--target", "cursor", "--yes"); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := fixture.execute(false, "rebind", "demo", second, "--dry-run", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	assertVersionedJSON(t, stdout, "rebind")
	if strings.Contains(stdout, fixture.root) {
		t.Fatalf("rebind plan leaked sandbox path: %s", stdout)
	}
	if _, _, err := fixture.execute(false, "rebind", "demo", second, "--yes"); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("materialized rebind error = %v", err)
	}
	if _, _, err := fixture.execute(false, "remove", "demo", "--target", "cursor", "--yes"); err != nil {
		t.Fatal(err)
	}
	stdout, _, err = fixture.execute(false, "rebind", "demo", second, "--yes", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	assertVersionedJSON(t, stdout, "rebind")
	state, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.Installations[0].Source.CanonicalSource != second || state.Installations[0].Source.RequestedSource != second {
		t.Fatalf("rebound source = %+v", state.Installations[0].Source)
	}
}

func TestHumanBindingPlanShowsCompleteReviewedDiff(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	renderHumanBindingPlan(&output, usecase.BindingChangePlan{
		OldName: "legacy-demo", NewName: "standard-demo",
		OldSource: usecase.ProvenanceSummary{
			Kind: "github", Repository: "example/legacy", PackageSubpath: "plugins/demo",
			ResolvedRevision: "old-revision", TreeDigest: "sha256:old-tree",
		},
		NewSource: usecase.ProvenanceSummary{
			Kind: "github", Repository: "example/standard", PackageSubpath: "plugins/demo",
			ResolvedRevision: "new-revision", TreeDigest: "sha256:new-tree",
		},
		OldFormat:     usecase.FormatSummary{FormatID: domain.FormatIDLegacyV1, SchemaURI: "plugin.yaml/v1"},
		NewFormat:     usecase.FormatSummary{FormatID: domain.FormatIDAgentPluginsV1, SchemaURI: domain.PluginSchemaV1},
		OldComponents: domain.ComponentInventory{Skills: []string{"old-skill"}},
		NewComponents: domain.ComponentInventory{
			MCPPresent: true, MCPEnabled: true, MCPServers: []string{"docs"},
			Skills: []string{"new-skill"}, Extensions: []string{"com.example.client"},
		},
		NativeObjectCount: 2,
	})
	for _, expected := range []string{
		"Schema: plugin.yaml/v1 -> " + domain.PluginSchemaV1,
		"example/legacy//plugins/demo@old-revision#sha256:old-tree",
		"example/standard//plugins/demo@new-revision#sha256:new-tree",
		"Components: mcp=absent skills=[old-skill] extensions=[] -> mcp=enabled[docs] skills=[new-skill] extensions=[com.example.client]",
		"Native objects: 2",
		"PLUGIN_DATA: not transferred",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output omitted %q: %s", expected, output.String())
		}
	}
}

func TestMigrateStateIsExplicitPlanFirstAndPathRedacted(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, nil)
	legacyPath := filepath.Join(fixture.root, "legacy", "state.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"schema_version":1,"installations":[{"integration_id":"legacy-demo","requested_source_ref":{"kind":"github_repo_path","value":"legacy-demo"},"resolved_source_ref":{"kind":"git_commit","value":"https://example.com/demo@abc"},"source_digest":"sha256:tree","manifest_digest":"sha256:manifest","policy":{"scope":"user"},"targets":{}}]}`
	if err := os.WriteFile(legacyPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	migrator := statemigration.Migrator{LegacyPath: legacyPath, V2Store: fixture.store}
	fixture.app.StateMigrator = &migrator
	stdout, _, err := fixture.execute(false, "migrate-state", "--dry-run", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	assertVersionedJSON(t, stdout, "migrate-state")
	if strings.Contains(stdout, fixture.root) {
		t.Fatalf("migration plan leaked a sandbox path: %s", stdout)
	}
	if _, err := os.Stat(fixture.store.Path); !os.IsNotExist(err) {
		t.Fatalf("dry-run created state v2: %v", err)
	}
	stdout, _, err = fixture.execute(false, "migrate-state", "--yes", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	assertVersionedJSON(t, stdout, "migrate-state")
	state, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Installations) != 1 || state.Installations[0].Package.LoaderKind != domain.LoaderKindLegacy {
		t.Fatalf("migrated state = %+v", state)
	}
}

func TestLegacyRemovalRequiresExplicitAllTargetAndReconcilesV2(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, nil)
	legacy := &cliLegacyLifecycleStub{exists: true}
	fixture.app.LegacyLifecycle = legacy
	installationID := "00000000-0000-4000-8000-000000000001"
	clientID := domain.ComputeClientBindingID(installationID, "codex", "user", "/legacy/plugin")
	state := domain.StateFileV2{SchemaVersion: domain.StateSchemaVersion, Installations: []domain.Installation{{
		InstallationID: installationID, DeclaredName: "legacy-demo",
		Source:  domain.SourceBinding{SourceBindingID: "src_legacy", RequestedSource: "legacy-demo", CanonicalSource: "legacy-demo"},
		Package: domain.PackageBinding{LoaderKind: domain.LoaderKindLegacy, FormatID: domain.FormatIDLegacyV1, SchemaURI: "plugin.yaml/v1", DeclaredName: "legacy-demo"},
		Clients: map[string]domain.ClientBinding{clientID: {
			ClientBindingID: clientID, ClientID: "codex", Scope: "user", TargetLocator: "/legacy/plugin",
			PhysicalArtifact: domain.ComputePhysicalArtifactID("legacy-demo", installationID), Materialization: domain.MaterializationMaterialized,
			Activation: domain.ActivationManual, Authentication: domain.AuthenticationNotRequired,
			Policy: domain.PolicyAllowed, Verification: domain.VerificationNotRun,
		}},
	}}}
	if err := fixture.store.Save(state); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fixture.execute(false, "remove", "legacy-demo", "--yes"); err == nil || !strings.Contains(err.Error(), "legacy-all") {
		t.Fatalf("unsafe legacy non-TTY removal error = %v", err)
	}
	stdout, _, err := fixture.execute(false, "remove", "legacy-demo", "--target", "legacy-all", "--yes", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	assertVersionedJSON(t, stdout, "remove")
	updated, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, client := range updated.Installations[0].Clients {
		if client.Materialization != domain.MaterializationAbsent {
			t.Fatalf("legacy client was not reconciled: %+v", client)
		}
	}
}

type cliFixture struct {
	root       string
	app        App
	store      statev2.Store
	operations string
}

func newCLIFixture(t *testing.T, clients []domain.DetectedClient) cliFixture {
	t.Helper()
	root := t.TempDir()
	registry, err := specregistry.New()
	if err != nil {
		t.Fatal(err)
	}
	store := statev2.Store{Path: filepath.Join(root, "data", "state-v2.json")}
	operations := filepath.Join(root, "data", "operations-v2")
	runner := processadapter.OS{}
	return cliFixture{
		root: root, store: store, operations: operations,
		app: App{
			Version: "0.1.0", UserHome: filepath.Join(root, "home"),
			ManagedRoot: filepath.Join(root, "data", "managed"), StateStore: store,
			Directory: dirswap.Manager{JournalDir: operations}, Detector: staticDetector{clients: clients},
			SourceResolver: sourceadapter.Resolver{Runner: runner, DisableAliases: true},
			PackageLoader:  loader.Loader{Registry: registry}, Stager: providers.Stager{},
			Activator:       providers.Activator{},
			LegacyStateLock: locks.FileLock{BaseDir: filepath.Join(root, "legacy-locks")},
			MutationLock:    processlock.Lock{Path: filepath.Join(root, "data", "mutation.lock")},
		},
	}
}

func (fixture cliFixture) execute(terminal bool, args ...string) (string, string, error) {
	return fixture.executeInput(terminal, "", args...)
}

func (fixture cliFixture) executeInput(terminal bool, input string, args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	app := fixture.app
	app.Input = strings.NewReader(input)
	app.Output = &stdout
	app.ErrorOutput = &stderr
	app.Terminal = terminal
	command := NewRoot(app)
	command.SetArgs(args)
	err := command.ExecuteContext(context.Background())
	return stdout.String(), stderr.String(), err
}

type staticDetector struct {
	clients []domain.DetectedClient
}

type failingVerifyStager struct {
	err error
}

func (failingVerifyStager) Stage(context.Context, domain.PackageEnvelope, domain.DeliveryPlan, string, domain.CompatibilityHints) (domain.StagedDelivery, error) {
	return domain.StagedDelivery{}, errors.New("unexpected stage")
}

func (failingVerifyStager) Discard(context.Context, domain.StagedDelivery) error {
	return errors.New("unexpected discard")
}

func (stager failingVerifyStager) Verify(context.Context, string, string) error {
	return stager.err
}

type cliLegacyLifecycleStub struct {
	exists bool
}

func (stub *cliLegacyLifecycleStub) Exists(context.Context, string) (bool, error) {
	return stub.exists, nil
}

func (stub *cliLegacyLifecycleStub) PlanRemove(context.Context, string) (ports.LegacyRemovalPlan, error) {
	return ports.LegacyRemovalPlan{Summary: "remove", TargetIDs: []string{"codex"}}, nil
}

func (stub *cliLegacyLifecycleStub) Remove(context.Context, string) (ports.LegacyRemovalPlan, error) {
	stub.exists = false
	return ports.LegacyRemovalPlan{Summary: "removed", TargetIDs: []string{"codex"}}, nil
}

func (detector staticDetector) Detect(context.Context) ([]domain.DetectedClient, error) {
	return append([]domain.DetectedClient(nil), detector.clients...), nil
}

func fixtureClient(t *testing.T, client domain.ClientID) domain.DetectedClient {
	t.Helper()
	return domain.DetectedClient{
		ClientID: client, DisplayName: string(client), Status: domain.DetectionDetected,
		ConfigRoot: filepath.Join(t.TempDir(), "home", "."+string(client)),
	}
}

func writeCLIPlugin(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	body := `{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name": "demo",
  "version": "1.0.0",
  "description": "Demo plugin"
}`
	if err := os.WriteFile(filepath.Join(root, "plugin.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func assertVersionedJSON(t *testing.T, body, command string) {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		t.Fatalf("decode JSON output %q: %v", body, err)
	}
	if value["schema_version"] != float64(1) || value["command"] != command {
		t.Fatalf("JSON envelope = %+v", value)
	}
}

func onlyCLIClient(installation domain.Installation) domain.ClientBinding {
	for _, binding := range installation.Clients {
		return binding
	}
	return domain.ClientBinding{}
}
