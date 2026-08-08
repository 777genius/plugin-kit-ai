package providers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func TestActivatorKeepsCopilotManualEvenWhenCallerHasATerminal(t *testing.T) {
	t.Parallel()
	for _, interactive := range []bool{false, true} {
		request := activationRequest(t, domain.ClientCopilot)
		request.Interactive = interactive
		outcome, err := (Activator{}).Activate(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Activation != domain.ActivationManual || outcome.Verification != domain.VerificationPackageValid {
			t.Fatalf("outcome = %+v", outcome)
		}
		if len(outcome.UserActions) != 1 || !strings.Contains(outcome.UserActions[0], "copilot plugin install") {
			t.Fatalf("actions = %+v", outcome.UserActions)
		}
	}
}

func TestActivatorDoesNotOfferCopilotCommandForVSCodeOnlyTarget(t *testing.T) {
	t.Parallel()
	request := activationRequest(t, domain.ClientVSCode)
	outcome, err := (Activator{}).Activate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Activation != domain.ActivationManual || len(outcome.UserActions) != 1 || strings.Contains(outcome.UserActions[0], "copilot plugin") || !strings.Contains(outcome.UserActions[0], "VS Code") {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestActivatorKeepsUIOnlyClientsInManualActivationState(t *testing.T) {
	t.Parallel()
	for _, client := range []domain.ClientID{domain.ClientCodex, domain.ClientKiro} {
		request := activationRequest(t, client)
		request.Interactive = true
		outcome, err := (Activator{}).Activate(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Activation != domain.ActivationManual {
			t.Fatalf("client %s outcome = %+v", client, outcome)
		}
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

func TestDeactivatorNeverRemovesCopilotPackageBeforeManualClientUninstall(t *testing.T) {
	t.Parallel()
	for _, activation := range []domain.ActivationState{domain.ActivationManual, domain.ActivationActive} {
		for _, interactive := range []bool{false, true} {
			outcome, err := (Activator{}).Deactivate(context.Background(), domain.DeactivationRequest{
				Client: domain.DetectedClient{ClientID: domain.ClientCopilot}, DeclaredName: "demo",
				CurrentActivation: activation, Interactive: interactive,
			})
			if err != nil {
				t.Fatal(err)
			}
			if outcome.ArtifactRemovalAllowed || outcome.Activation != domain.ActivationManual || outcome.ExternalRemovalComplete {
				t.Fatalf("outcome = %+v", outcome)
			}
			if len(outcome.UserActions) != 1 || !strings.Contains(outcome.UserActions[0], "copilot plugin uninstall demo") {
				t.Fatalf("actions = %+v", outcome.UserActions)
			}
		}
	}
}

func TestDeactivatorAllowsCopiedArtifactRemovalAfterExplicitExternalUninstall(t *testing.T) {
	t.Parallel()
	for _, client := range []domain.ClientID{domain.ClientCodex, domain.ClientKiro, domain.ClientCopilot, domain.ClientVSCode} {
		outcome, err := (Activator{}).Deactivate(context.Background(), domain.DeactivationRequest{
			Client: domain.DetectedClient{ClientID: client}, DeclaredName: "demo",
			CurrentActivation: domain.ActivationManual, ExternalUninstalled: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !outcome.ArtifactRemovalAllowed || !outcome.ExternalRemovalComplete || len(outcome.UserActions) != 0 {
			t.Fatalf("client %s outcome = %+v", client, outcome)
		}
	}
}

func TestDeactivatorPreservesCopiedClientArtifactUntilExternalUninstallIsAcknowledged(t *testing.T) {
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
		Client: domain.DetectedClient{ClientID: client, Status: domain.DetectionDetected},
		Plan: domain.DeliveryPlan{
			ClientID: client, ActivePath: active, Authentication: domain.AuthenticationNotChecked,
		},
		Delivery: domain.StagedDelivery{ClientID: client, OwnedBase: base, ActivePath: active},
	}
}
