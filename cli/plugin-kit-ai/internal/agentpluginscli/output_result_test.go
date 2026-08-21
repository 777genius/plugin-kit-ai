package agentpluginscli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/usecase"
)

func TestSwitchResultEnvelopeMatchesStructuredStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		status     string
		groupPhase usecase.GroupPhase
		wantResult string
	}{
		{name: "preflight failure", status: "preflight_failed", groupPhase: usecase.GroupPhasePlanned, wantResult: outputResultFailure},
		{name: "controlled rollback", status: string(usecase.GroupPhaseManagedRolledBack), groupPhase: usecase.GroupPhaseManagedRolledBack, wantResult: outputResultFailure},
		{name: "managed commit activation failure", status: string(usecase.GroupPhaseManagedActivationFailed), groupPhase: usecase.GroupPhaseManagedActivationFailed, wantResult: outputResultFailure},
		{name: "partial external outcome", status: string(usecase.GroupPhaseExternalPartialFailure), groupPhase: usecase.GroupPhaseExternalPartialFailure, wantResult: outputResultFailure},
		{name: "retained apply failure", status: "apply_failed", wantResult: outputResultFailure},
		{name: "real success", status: string(usecase.GroupPhaseCompleted), groupPhase: usecase.GroupPhaseCompleted, wantResult: outputResultSuccess},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result := switchOutput{Status: test.status}
			if test.groupPhase != "" {
				result.Group = &usecase.GroupResult{Phase: test.groupPhase}
			} else {
				result.Retained = &usecase.BindingChangeResult{}
			}

			var jsonOutput bytes.Buffer
			if err := renderSwitchResult(&jsonOutput, "json", result); err != nil {
				t.Fatal(err)
			}
			assertOutputEnvelopeResult(t, jsonOutput.Bytes(), "switch", test.wantResult)
			var structured struct {
				Data switchOutput `json:"data"`
			}
			if err := json.Unmarshal(jsonOutput.Bytes(), &structured); err != nil {
				t.Fatal(err)
			}
			if structured.Data.Status != test.status {
				t.Fatalf("structured status = %q, want %q", structured.Data.Status, test.status)
			}

			var humanOutput bytes.Buffer
			if err := renderSwitchResult(&humanOutput, "human", result); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(humanOutput.String(), "Switch: "+test.status) {
				t.Fatalf("human output = %q", humanOutput.String())
			}
			if test.wantResult != outputResultSuccess && strings.Contains(humanOutput.String(), "Switch: completed") {
				t.Fatalf("failure claimed completion: %q", humanOutput.String())
			}
		})
	}
}

func TestSingleTargetAddResultEnvelopeMatchesCommandOutcome(t *testing.T) {
	t.Parallel()
	success := domain.ActivationOutcome{
		Activation: domain.ActivationActive, Authentication: domain.AuthenticationNotRequired,
		Policy: domain.PolicyAllowed, Verification: domain.VerificationInstalled,
	}
	activationFailure := domain.ActivationOutcome{
		Activation: domain.ActivationFailed, Authentication: domain.AuthenticationNotChecked,
		Policy: domain.PolicyAllowed, Verification: domain.VerificationFailed,
	}
	tests := []struct {
		name       string
		result     usecase.AddResult
		commandErr error
		wantResult string
		wantHuman  string
	}{
		{
			name: "add activation failure", result: usecase.AddResult{Mutated: true, Activation: activationFailure},
			commandErr: errors.New("activate client"), wantResult: outputResultFailure, wantHuman: "Add: activation_failed",
		},
		{
			name: "resume activation failure", result: usecase.AddResult{Mutated: false, Activation: activationFailure},
			commandErr: errors.New("resume client activation"), wantResult: outputResultFailure, wantHuman: "Add: activation_failed",
		},
		{
			name: "controlled rollback without activation outcome", result: usecase.AddResult{},
			commandErr: errors.New("transaction rolled back"), wantResult: outputResultFailure, wantHuman: "Add: failed",
		},
		{
			name: "real success", result: usecase.AddResult{Mutated: true, Activation: success},
			wantResult: outputResultSuccess, wantHuman: "Installed and verified for the selected client.",
		},
	}
	envelope := domain.PackageEnvelope{Manifest: domain.PluginManifest{Name: "demo", Version: "1.0.0"}}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var jsonOutput bytes.Buffer
			if err := renderAddResultError(&jsonOutput, "json", envelope, test.result, false, test.commandErr); err != nil {
				t.Fatal(err)
			}
			assertOutputEnvelopeResult(t, jsonOutput.Bytes(), "add", test.wantResult)

			var humanOutput bytes.Buffer
			if err := renderAddResultError(&humanOutput, "human", envelope, test.result, false, test.commandErr); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(humanOutput.String(), test.wantHuman) {
				t.Fatalf("human output = %q", humanOutput.String())
			}
			if test.wantResult != outputResultSuccess && strings.Contains(strings.ToLower(humanOutput.String()), "completed") {
				t.Fatalf("failure claimed completion: %q", humanOutput.String())
			}
		})
	}
}

func assertOutputEnvelopeResult(t *testing.T, body []byte, command, result string) {
	t.Helper()
	var envelope outputEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode output %q: %v", body, err)
	}
	if envelope.SchemaVersion != outputSchemaVersion || envelope.Command != command || envelope.Result != result {
		t.Fatalf("output envelope = %+v", envelope)
	}
}
