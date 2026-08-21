package usecase

import (
	"context"
	"reflect"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func TestSingleTargetServiceResolvesEitherLogicalSharedSurfaceToOnePhysicalBinding(t *testing.T) {
	t.Parallel()
	service, store, _ := serviceFixture(t)
	copilot := domain.DetectedClient{ClientID: domain.ClientCopilot, DisplayName: "Copilot", Status: domain.DetectionDetected}
	vscode := domain.DetectedClient{ClientID: domain.ClientVSCode, DisplayName: "VS Code", Status: domain.DetectionDetected}

	add := addInput(t, copilot, "https://example.com/shared-service")
	add.Confirmed = true
	added, err := service.Add(context.Background(), add)
	if err != nil {
		t.Fatal(err)
	}
	update := addInput(t, vscode, "https://example.com/shared-service")
	setEnvelopeVersion(t, &update.Envelope, "2.0.0", "sha256:shared-v2", "sha256:shared-manifest-v2")
	update.Confirmed = true
	if _, err := service.Update(context.Background(), update); err != nil {
		t.Fatalf("update through alternate shared surface: %v", err)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Installations) != 1 || len(state.Installations[0].Clients) != 1 {
		t.Fatalf("shared service created duplicate physical bindings: %+v", state.Installations)
	}
	binding := onlyBinding(state.Installations[0])
	if binding.ClientBindingID != added.Receipt.ClientBindingID || binding.ClientID != string(domain.ClientCopilot) || binding.PackageRevision.Version != "2.0.0" || !reflect.DeepEqual(binding.AffectedSurfaces, []string{"copilot", "vscode"}) || len(binding.Receipts) != 2 {
		t.Fatalf("shared service binding = %+v", binding)
	}
}
