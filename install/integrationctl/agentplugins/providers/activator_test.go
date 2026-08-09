package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	legacyports "github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type recordingRunner struct {
	commands []legacyports.Command
	run      func(legacyports.Command) legacyports.CommandResult
}

func (runner *recordingRunner) Run(_ context.Context, command legacyports.Command) (legacyports.CommandResult, error) {
	runner.commands = append(runner.commands, command)
	if runner.run != nil {
		return runner.run(command), nil
	}
	if strings.HasSuffix(strings.Join(command.Argv, " "), "plugin list") {
		for index := len(runner.commands) - 2; index >= 0; index-- {
			argv := runner.commands[index].Argv
			if len(argv) >= 4 && argv[1] == "plugin" && argv[2] == "install" {
				return legacyports.CommandResult{Stdout: []byte("• " + argv[3] + " (v1.0.0)")}, nil
			}
		}
	}
	return legacyports.CommandResult{}, nil
}

func TestActivatorInstallsCopilotAndVSCodeThroughManagedMarketplace(t *testing.T) {
	t.Parallel()
	for _, client := range []domain.ClientID{domain.ClientCopilot, domain.ClientVSCode} {
		client := client
		t.Run(string(client), func(t *testing.T) {
			runner := &recordingRunner{}
			request := activationRequest(t, client)
			request.BackendExecutable = "/test/bin/copilot"
			outcome, err := (Activator{Runner: runner}).Activate(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if outcome.Activation != domain.ActivationActive || outcome.Verification != domain.VerificationInstalled || len(outcome.UserActions) != 0 {
				t.Fatalf("outcome = %+v", outcome)
			}
			marketplace := managedMarketplaceName(request.Plan.PhysicalArtifactID)
			want := [][]string{
				{"/test/bin/copilot", "plugin", "marketplace", "add", request.Delivery.ActivePath},
				{"/test/bin/copilot", "plugin", "install", "demo@" + marketplace},
				{"/test/bin/copilot", "plugin", "list"},
			}
			if got := commandArgv(runner.commands); !reflect.DeepEqual(got, want) {
				t.Fatalf("commands = %#v, want %#v", got, want)
			}
		})
	}
}

func TestActivatorUpdateRecoversMissingMarketplaceAndPlugin(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{run: func(command legacyports.Command) legacyports.CommandResult {
		joined := strings.Join(command.Argv, " ")
		if strings.Contains(joined, "marketplace update") || strings.Contains(joined, "plugin update") {
			return legacyports.CommandResult{ExitCode: 1, Stderr: []byte("missing")}
		}
		if strings.HasSuffix(joined, "plugin list") {
			return legacyports.CommandResult{Stdout: []byte("• demo@agentplugins-8f97b00da374 (v1.0.0)")}
		}
		return legacyports.CommandResult{}
	}}
	request := activationRequest(t, domain.ClientCopilot)
	request.BackendExecutable = "/test/bin/copilot"
	request.Replacing = true
	outcome, err := (Activator{Runner: runner}).Activate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Activation != domain.ActivationActive {
		t.Fatalf("outcome = %+v", outcome)
	}
	marketplace := managedMarketplaceName(request.Plan.PhysicalArtifactID)
	want := [][]string{
		{"/test/bin/copilot", "plugin", "marketplace", "update", marketplace},
		{"/test/bin/copilot", "plugin", "marketplace", "add", request.Delivery.ActivePath},
		{"/test/bin/copilot", "plugin", "update", "demo@" + marketplace},
		{"/test/bin/copilot", "plugin", "install", "demo@" + marketplace},
		{"/test/bin/copilot", "plugin", "list"},
	}
	if got := commandArgv(runner.commands); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestActivatorDoesNotTrustCopilotInstallExitZeroWithoutListingEvidence(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{run: func(legacyports.Command) legacyports.CommandResult { return legacyports.CommandResult{} }}
	request := activationRequest(t, domain.ClientCopilot)
	request.BackendExecutable = "/test/bin/copilot"
	outcome, err := (Activator{Runner: runner}).Activate(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "not listed") {
		t.Fatalf("verification error = %v", err)
	}
	if outcome.Activation != domain.ActivationFailed || outcome.Verification != domain.VerificationFailed || len(outcome.LocalActions) != 1 {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestActivatorUsesCodexCLIAndVerifiesJSONState(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{run: func(command legacyports.Command) legacyports.CommandResult {
		if strings.Contains(strings.Join(command.Argv, " "), "plugin list --json") {
			return legacyports.CommandResult{Stdout: []byte(`{"installed":[{"pluginId":"demo@agentplugins-8f97b00da374","name":"demo","marketplaceName":"agentplugins-8f97b00da374","installed":true,"enabled":true}],"available":[]}`)}
		}
		return legacyports.CommandResult{}
	}}
	request := activationRequest(t, domain.ClientCodex)
	request.BackendExecutable = "/test/bin/codex"
	outcome, err := (Activator{Runner: runner}).Activate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Activation != domain.ActivationActive || outcome.Verification != domain.VerificationInstalled {
		t.Fatalf("outcome = %+v", outcome)
	}
	marketplace := managedMarketplaceName(request.Plan.PhysicalArtifactID)
	want := [][]string{
		{"/test/bin/codex", "plugin", "marketplace", "add", request.Delivery.ActivePath, "--json"},
		{"/test/bin/codex", "plugin", "add", "demo@" + marketplace, "--json"},
		{"/test/bin/codex", "plugin", "list", "--json"},
	}
	if got := commandArgv(runner.commands); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestCodexVerificationRequiresExactEnabledManagedEntry(t *testing.T) {
	t.Parallel()
	request := activationRequest(t, domain.ClientCodex)
	request.BackendExecutable = "/test/bin/codex"
	request.VerifyOnly = true
	marketplace := managedMarketplaceName(request.Plan.PhysicalArtifactID)
	cases := map[string]string{
		"old shape":         `{"plugins":[{"name":"demo"}]}`,
		"disabled":          fmt.Sprintf(`{"installed":[{"name":"demo","marketplaceName":%q,"installed":true,"enabled":false}]}`, marketplace),
		"wrong marketplace": `{"installed":[{"name":"demo","marketplaceName":"other","installed":true,"enabled":true}]}`,
		"unrelated":         fmt.Sprintf(`{"installed":[{"name":"demo-extra","marketplaceName":%q,"installed":true,"enabled":true}]}`, marketplace),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			runner := &recordingRunner{run: func(legacyports.Command) legacyports.CommandResult {
				return legacyports.CommandResult{Stdout: []byte(body)}
			}}
			if _, err := (Activator{Runner: runner}).Activate(context.Background(), request); err == nil {
				t.Fatal("untrusted Codex listing was accepted")
			}
		})
	}
}

func TestCopilotVerificationUsesOnlyExactHealthyStdoutToken(t *testing.T) {
	t.Parallel()
	request := activationRequest(t, domain.ClientCopilot)
	request.BackendExecutable = "/test/bin/copilot"
	request.VerifyOnly = true
	spec := "demo@" + managedMarketplaceName(request.Plan.PhysicalArtifactID)
	cases := []legacyports.CommandResult{
		{Stdout: []byte("• demo (v1)")},
		{Stderr: []byte("• " + spec + " (v1)")},
		{Stdout: []byte("• " + spec + " disabled")},
		{Stdout: []byte("error: " + spec)},
		{Stdout: []byte("• demo-extra@" + managedMarketplaceName(request.Plan.PhysicalArtifactID) + " (v1)")},
	}
	for _, listed := range cases {
		runner := &recordingRunner{run: func(legacyports.Command) legacyports.CommandResult { return listed }}
		if _, err := (Activator{Runner: runner}).Activate(context.Background(), request); err == nil {
			t.Fatalf("untrusted Copilot listing was accepted: %+v", listed)
		}
	}
}

func TestKiroVerificationUsesExactHealthyStdoutIdentity(t *testing.T) {
	t.Parallel()
	request := activationRequest(t, domain.ClientKiro)
	request.BackendExecutable = "/test/bin/kiro-cli"
	request.VerifyOnly = true
	request.Plan.Components = []domain.ComponentDecision{{Kind: domain.ComponentMCPServer, Name: "demo-server", Support: domain.SupportNative}}
	for _, listed := range []legacyports.CommandResult{
		{Stderr: []byte("demo-server enabled")}, {Stdout: []byte("demo-server disabled")},
		{Stdout: []byte("demo-server error")}, {Stdout: []byte("demo-server-other enabled")},
	} {
		runner := &recordingRunner{run: func(legacyports.Command) legacyports.CommandResult { return listed }}
		if _, err := (Activator{Runner: runner}).Activate(context.Background(), request); err == nil {
			t.Fatalf("untrusted Kiro listing was accepted: %+v", listed)
		}
	}
}

func TestClientListingDoesNotConvertUnknownAuthentication(t *testing.T) {
	t.Parallel()
	request := activationRequest(t, domain.ClientCopilot)
	request.BackendExecutable = "/test/bin/copilot"
	outcome, err := (Activator{Runner: &recordingRunner{}}).Activate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Authentication != domain.AuthenticationNotChecked {
		t.Fatalf("authentication = %s", outcome.Authentication)
	}
}

func TestActivatorImportsMCPOnlyPackageThroughKiroCLIAndVerifiesListing(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{run: func(command legacyports.Command) legacyports.CommandResult {
		if strings.Contains(strings.Join(command.Argv, " "), "mcp list global") {
			return legacyports.CommandResult{Stdout: []byte("demo-server enabled")}
		}
		return legacyports.CommandResult{}
	}}
	request := activationRequest(t, domain.ClientKiro)
	request.BackendExecutable = "/test/bin/kiro-cli"
	request.Plan.Components = []domain.ComponentDecision{{Kind: domain.ComponentMCPServer, Name: "demo-server", Support: domain.SupportNative}}
	outcome, err := (Activator{Runner: runner}).Activate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Activation != domain.ActivationActive || outcome.Verification != domain.VerificationInstalled {
		t.Fatalf("outcome = %+v", outcome)
	}
	want := [][]string{
		{"/test/bin/kiro-cli", "mcp", "import", "--file", filepath.Join(request.Delivery.ActivePath, "mcp.json"), "global", "--force"},
		{"/test/bin/kiro-cli", "mcp", "list", "global"},
	}
	if got := commandArgv(runner.commands); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestActivatorKeepsKiroUIComponentsToOneActionableManualStep(t *testing.T) {
	t.Parallel()
	request := activationRequest(t, domain.ClientKiro)
	request.BackendExecutable = "/test/bin/kiro-cli"
	request.Plan.Components = []domain.ComponentDecision{{Kind: domain.ComponentSkill, Name: "guide", Support: domain.SupportNative}}
	outcome, err := (Activator{Runner: &recordingRunner{}}).Activate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Activation != domain.ActivationManual || len(outcome.LocalActions) != 1 || !strings.Contains(outcome.LocalActions[0], request.Delivery.ActivePath) || !strings.Contains(outcome.LocalActions[0], "verify") {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestActivatorCleansNewMarketplaceWhenPluginInstallFails(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{run: func(command legacyports.Command) legacyports.CommandResult {
		if strings.Contains(strings.Join(command.Argv, " "), "plugin install") {
			return legacyports.CommandResult{ExitCode: 1, Stderr: []byte("install failed")}
		}
		return legacyports.CommandResult{}
	}}
	request := activationRequest(t, domain.ClientCopilot)
	request.BackendExecutable = "/test/bin/copilot"
	if _, err := (Activator{Runner: runner}).Activate(context.Background(), request); err == nil {
		t.Fatal("failed install unexpectedly succeeded")
	}
	marketplace := managedMarketplaceName(request.Plan.PhysicalArtifactID)
	want := [][]string{
		{"/test/bin/copilot", "plugin", "marketplace", "add", request.Delivery.ActivePath},
		{"/test/bin/copilot", "plugin", "install", "demo@" + marketplace},
		{"/test/bin/copilot", "plugin", "marketplace", "remove", marketplace},
	}
	if got := commandArgv(runner.commands); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestActivatorUsesPathSpecificManualHintsWithoutLeakingThemToJSON(t *testing.T) {
	t.Parallel()
	for _, client := range []domain.ClientID{domain.ClientCodex, domain.ClientKiro, domain.ClientVSCode} {
		client := client
		t.Run(string(client), func(t *testing.T) {
			request := activationRequest(t, client)
			outcome, err := (Activator{}).Activate(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if outcome.Activation != domain.ActivationManual || len(outcome.UserActions) != 1 || len(outcome.LocalActions) != 1 {
				t.Fatalf("outcome = %+v", outcome)
			}
			if !strings.Contains(outcome.LocalActions[0], request.Delivery.ActivePath) {
				t.Fatalf("local action = %q", outcome.LocalActions[0])
			}
			body, err := json.Marshal(outcome)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), request.Delivery.ActivePath) {
				t.Fatalf("public JSON leaked local path: %s", body)
			}
		})
	}
}

func TestActivatorKeepsCursorManualUntilClientDiscoveryIsVerified(t *testing.T) {
	t.Parallel()
	request := activationRequest(t, domain.ClientCursor)
	outcome, err := (Activator{}).Activate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Activation != domain.ActivationManual || outcome.Verification != domain.VerificationPackageValid || len(outcome.UserActions) != 1 {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestDeactivatorPreviewsThenRemovesNativeCopilotLifecycle(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{}
	request := domain.DeactivationRequest{
		Client: domain.DetectedClient{ClientID: domain.ClientCopilot}, DeclaredName: "demo",
		CurrentActivation: domain.ActivationActive, BackendExecutable: "/test/bin/copilot",
		PhysicalArtifactID: "demo-0123456789ab",
	}
	preview, err := (Activator{Runner: runner}).Deactivate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.ArtifactRemovalAllowed || preview.ExternalRemovalComplete || len(runner.commands) != 0 {
		t.Fatalf("preview = %+v, commands = %+v", preview, runner.commands)
	}
	request.Confirmed = true
	outcome, err := (Activator{Runner: runner}).Deactivate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.ArtifactRemovalAllowed || !outcome.ExternalRemovalComplete {
		t.Fatalf("outcome = %+v", outcome)
	}
	want := [][]string{
		{"/test/bin/copilot", "plugin", "uninstall", "demo@" + managedMarketplaceName(request.PhysicalArtifactID)},
		{"/test/bin/copilot", "plugin", "marketplace", "remove", managedMarketplaceName(request.PhysicalArtifactID)},
	}
	if got := commandArgv(runner.commands); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestDeactivatorTreatsAlreadyAbsentNativeObjectsAsRemoved(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{run: func(command legacyports.Command) legacyports.CommandResult {
		if strings.Contains(strings.Join(command.Argv, " "), "marketplace remove") {
			return legacyports.CommandResult{ExitCode: 1, Stderr: []byte(`Marketplace "demo" is not registered`)}
		}
		return legacyports.CommandResult{ExitCode: 1, Stderr: []byte(`Plugin "demo" is not installed`)}
	}}
	outcome, err := (Activator{Runner: runner}).Deactivate(context.Background(), domain.DeactivationRequest{
		Client: domain.DetectedClient{ClientID: domain.ClientVSCode}, DeclaredName: "demo",
		CurrentActivation: domain.ActivationActive, BackendExecutable: "/test/bin/copilot",
		PhysicalArtifactID: "demo-0123456789ab", Confirmed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.ArtifactRemovalAllowed || !outcome.ExternalRemovalComplete {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestDeactivatorDoesNotClaimManualCopilotLifecycle(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{}
	outcome, err := (Activator{Runner: runner}).Deactivate(context.Background(), domain.DeactivationRequest{
		Client: domain.DetectedClient{ClientID: domain.ClientCopilot}, DeclaredName: "demo",
		CurrentActivation: domain.ActivationManual, BackendExecutable: "/test/bin/copilot",
		PhysicalArtifactID: "demo-0123456789ab", Confirmed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.ArtifactRemovalAllowed || len(outcome.UserActions) != 1 || len(runner.commands) != 0 {
		t.Fatalf("outcome = %+v, commands = %+v", outcome, runner.commands)
	}
}

func TestDeactivatorPreservesManualClientArtifactsUntilAcknowledged(t *testing.T) {
	t.Parallel()
	for _, client := range []domain.ClientID{domain.ClientCodex, domain.ClientKiro, domain.ClientVSCode} {
		outcome, err := (Activator{}).Deactivate(context.Background(), domain.DeactivationRequest{
			Client: domain.DetectedClient{ClientID: client}, DeclaredName: "demo",
			CurrentActivation: domain.ActivationManual,
		})
		if err != nil {
			t.Fatal(err)
		}
		if outcome.ArtifactRemovalAllowed || outcome.Activation != domain.ActivationManual || outcome.ExternalRemovalComplete || len(outcome.UserActions) != 1 {
			t.Fatalf("client %s outcome = %+v", client, outcome)
		}
		if !strings.Contains(outcome.UserActions[0], "--external-uninstalled") {
			t.Fatalf("client %s actions = %+v", client, outcome.UserActions)
		}
	}
}

func activationRequest(t *testing.T, client domain.ClientID) domain.ActivationRequest {
	t.Helper()
	base := filepath.Join(t.TempDir(), "managed")
	active := filepath.Join(base, "demo")
	if err := os.MkdirAll(active, 0o755); err != nil {
		t.Fatal(err)
	}
	return domain.ActivationRequest{
		Client:       domain.DetectedClient{ClientID: client, Status: domain.DetectionDetected},
		DeclaredName: "demo",
		Plan: domain.DeliveryPlan{
			ClientID: client, ActivePath: active, PhysicalArtifactID: "demo-0123456789ab",
			Authentication: domain.AuthenticationNotChecked,
		},
		Delivery: domain.StagedDelivery{ClientID: client, OwnedBase: base, ActivePath: active},
	}
}

func commandArgv(commands []legacyports.Command) [][]string {
	result := make([][]string, 0, len(commands))
	for _, command := range commands {
		result = append(result, command.Argv)
	}
	return result
}
