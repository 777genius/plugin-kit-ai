package agentpluginscli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/providers"
)

func TestSwitchSynthesizesRemoteChatGPTAndRecoversAfterActivationFailure(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, nil)
	first := writePortableChatGPTPlugin(t)
	second := writePortableChatGPTPlugin(t)
	secondManifest := filepath.Join(second, "plugin.json")
	body, err := os.ReadFile(secondManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondManifest, []byte(strings.Replace(string(body), `"version": "1.0.0"`, `"version": "2.0.0"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := fixture.execute(false, "add", first, "--target", "chatgpt"); err != nil {
		t.Fatal(err)
	}
	// No desktop detector entry exists. switch must synthesize the already-bound
	// remote ChatGPT target just as add/update/repair do.
	preview, _, err := fixture.execute(false, "switch", "demo", "--to", second, "--dry-run", "--format", "json")
	if err != nil {
		t.Fatalf("switch preview without ChatGPT detection: %v", err)
	}
	assertVersionedJSON(t, preview, "switch")
	previewed, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	previewedBinding := onlyCLIClient(previewed.Installations[0])
	if previewed.Installations[0].Source.CanonicalSource != first || previewedBinding.PackageRevision.Version != "1.0.0" || len(previewedBinding.Receipts) != 1 {
		t.Fatalf("remote switch preview mutated state: %+v", previewed.Installations[0])
	}

	failure := &cliObservedActivator{outcome: domain.ActivationOutcome{
		Activation: domain.ActivationFailed, Authentication: domain.AuthenticationPending,
		Policy: domain.PolicyAllowed, Verification: domain.VerificationFailed,
	}, err: errors.New("injected remote activation failure")}
	fixture.app.Lifecycle.Activator = failure
	if _, _, err := fixture.execute(false, "switch", "demo", "--to", second, "--format", "json"); err == nil || !strings.Contains(err.Error(), "injected remote activation failure") {
		t.Fatalf("switch failure = %v", err)
	}
	failed, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	failedBinding := onlyCLIClient(failed.Installations[0])
	if failed.Installations[0].Source.CanonicalSource != second || failedBinding.PackageRevision.Version != "2.0.0" || failedBinding.Activation != domain.ActivationFailed {
		t.Fatalf("failed switch did not retain recoverable managed commit: %+v", failed.Installations[0])
	}

	fixture.app.Lifecycle.Activator = providers.Activator{}
	if _, _, err := fixture.execute(false, "switch", "demo", "--to", second, "--format", "json"); err != nil {
		t.Fatalf("switch recovery without ChatGPT detection: %v", err)
	}
	recovered, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	recoveredBinding := onlyCLIClient(recovered.Installations[0])
	if recoveredBinding.Activation != domain.ActivationManual || recoveredBinding.Verification == domain.VerificationInstalled || len(recoveredBinding.Receipts) < 3 {
		t.Fatalf("remote switch recovery claimed too much or lost receipts: %+v", recoveredBinding)
	}
}

func writePortableChatGPTPlugin(t *testing.T) string {
	t.Helper()
	root := writeCLIPlugin(t)
	if err := os.WriteFile(filepath.Join(root, ".app.json"), []byte(`{"apps":{"demo":{"id":"asdk_app_demo_123"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestSharedCopilotVSCodeSurfacesSurviveUpdateRepairAndVscodeRemoval(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientVSCode)})
	plugin := writeCLIPlugin(t)
	if _, _, err := fixture.execute(false, "add", plugin, "--target", "vscode"); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(plugin, "plugin.json")
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(strings.Replace(string(body), `"version": "1.0.0"`, `"version": "2.0.0"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fixture.execute(false, "update", "demo", "--target", "vscode"); err != nil {
		t.Fatalf("update through shared vscode surface: %v", err)
	}
	updated, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding := onlyCLIClient(updated.Installations[0])
	if binding.PackageRevision.Version != "2.0.0" || !reflect.DeepEqual(binding.AffectedSurfaces, []string{"copilot", "vscode"}) || len(binding.Receipts) != 2 {
		t.Fatalf("shared update binding = %+v", binding)
	}
	if err := os.RemoveAll(binding.TargetLocator); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fixture.execute(false, "repair", "demo", "--target", "vscode"); err != nil {
		t.Fatalf("repair through shared vscode surface: %v", err)
	}
	repaired, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding = onlyCLIClient(repaired.Installations[0])
	if !reflect.DeepEqual(binding.AffectedSurfaces, []string{"copilot", "vscode"}) || len(binding.Receipts) != 3 {
		t.Fatalf("shared repair dropped a logical surface: %+v", binding)
	}
	stdout, _, err := fixture.execute(false, "remove", "demo", "--target", "vscode", "--external-uninstalled", "--format", "json")
	if err != nil {
		t.Fatalf("remove through shared vscode surface: %v", err)
	}
	if !strings.Contains(stdout, `"affected_surfaces":["copilot","vscode"]`) {
		t.Fatalf("shared removal did not report both logical surfaces: %s", stdout)
	}
	removed, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.Installations[0].Clients) != 0 || !removed.Installations[0].DataRetained {
		t.Fatalf("shared physical binding was not removed once: %+v", removed.Installations[0])
	}
}

func TestVSCodeOnlySharedUpdateRejectsExplicitUndetectedCopilotWithoutMutation(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientVSCode)})
	plugin := writeCLIPlugin(t)
	if _, _, err := fixture.execute(false, "add", plugin, "--target", "vscode"); err != nil {
		t.Fatal(err)
	}
	before, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	bindingBefore := onlyCLIClient(before.Installations[0])
	body, err := os.ReadFile(filepath.Join(plugin, "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugin, "plugin.json"), []byte(strings.Replace(string(body), `"version": "1.0.0"`, `"version": "2.0.0"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := fixture.execute(false, "update", "demo", "--target", "copilot,vscode"); err == nil || !strings.Contains(err.Error(), `target "copilot" was not detected`) {
		t.Fatalf("explicit undetected Copilot update error = %v", err)
	}
	after, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	bindingAfter := onlyCLIClient(after.Installations[0])
	if bindingAfter.PackageRevision.Version != bindingBefore.PackageRevision.Version || len(bindingAfter.Receipts) != len(bindingBefore.Receipts) || bindingAfter.TargetLocator != bindingBefore.TargetLocator {
		t.Fatalf("failed explicit shared update mutated binding: before=%+v after=%+v", bindingBefore, bindingAfter)
	}
}
