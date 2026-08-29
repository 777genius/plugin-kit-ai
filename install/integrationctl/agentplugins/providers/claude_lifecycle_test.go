package providers

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	legacyports "github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func TestClaudeSkillsDirAddUpdateRemoveUseOnlyExactListVerification(t *testing.T) {
	t.Parallel()
	request := activationRequest(t, domain.ClientClaude)
	request.BackendExecutable = "/test/bin/claude"
	request.Client.ConfigRoot = filepath.Join(t.TempDir(), "claude-config")
	listing := claudeListing(request.DeclaredName, request.Delivery.ActivePath, true)
	runner := &recordingRunner{run: func(legacyports.Command) legacyports.CommandResult {
		return legacyports.CommandResult{Stdout: []byte(listing)}
	}}
	activator := Activator{Runner: runner}

	for _, replacing := range []bool{false, true} {
		request.Replacing = replacing
		outcome, err := activator.Activate(context.Background(), request)
		if err != nil || outcome.Activation != domain.ActivationActive || outcome.Verification != domain.VerificationInstalled {
			t.Fatalf("replacing=%t outcome=%+v err=%v", replacing, outcome, err)
		}
	}
	remove, err := activator.Deactivate(context.Background(), domain.DeactivationRequest{
		Client: request.Client, DeclaredName: request.DeclaredName, CurrentActivation: domain.ActivationActive,
		Confirmed: true, BackendExecutable: request.BackendExecutable, ManagedArtifactPath: request.Delivery.ActivePath,
	})
	if err != nil || !remove.ArtifactRemovalAllowed || !remove.ExternalRemovalComplete {
		t.Fatalf("remove=%+v err=%v", remove, err)
	}
	want := [][]string{
		{"/test/bin/claude", "plugin", "list", "--json"},
		{"/test/bin/claude", "plugin", "list", "--json"},
		{"/test/bin/claude", "plugin", "list", "--json"},
	}
	if got := commandArgv(runner.commands); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want list-only lifecycle %#v", got, want)
	}
}

func TestClaudeDryRunRemovalExecutesNoCommand(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{}
	outcome, err := (Activator{Runner: runner}).Deactivate(context.Background(), domain.DeactivationRequest{
		Client: domain.DetectedClient{ClientID: domain.ClientClaude}, DeclaredName: "demo",
		CurrentActivation: domain.ActivationActive, Confirmed: false, BackendExecutable: "/test/bin/claude",
		ManagedArtifactPath: filepath.Join(t.TempDir(), "skills", "demo"),
	})
	if err != nil || !outcome.ArtifactRemovalAllowed || len(runner.commands) != 0 {
		t.Fatalf("outcome=%+v commands=%+v err=%v", outcome, runner.commands, err)
	}
}

func TestClaudeExactListVerificationRejectsCollisionAndDrivesInstallFailure(t *testing.T) {
	t.Parallel()
	request := activationRequest(t, domain.ClientClaude)
	request.BackendExecutable = "/test/bin/claude"
	foreign := filepath.Join(t.TempDir(), "foreign", "demo")
	runner := &recordingRunner{run: func(legacyports.Command) legacyports.CommandResult {
		return legacyports.CommandResult{Stdout: []byte(claudeListing("demo", foreign, true))}
	}}
	outcome, err := (Activator{Runner: runner}).Activate(context.Background(), request)
	if err == nil || outcome.Activation != domain.ActivationFailed || outcome.Verification != domain.VerificationFailed || !outcome.AuthoritativeObservation {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
}

func TestClaudePluginStatusRequiresExactOfficialIdentity(t *testing.T) {
	t.Parallel()
	managed := filepath.Join(t.TempDir(), "claude-config", "skills", "managed")
	cases := map[string]struct {
		body string
		want claudeStatus
	}{
		"installed":  {claudeListing("demo", managed, true), claudeStatusInstalled},
		"empty":      {`[]`, claudeStatusAbsent},
		"disabled":   {claudeListing("demo", managed, false), claudeStatusAbsent},
		"wrong path": {claudeListing("demo", filepath.Join(t.TempDir(), "foreign"), true), claudeStatusCollision},
		"wrong id":   {claudeListing("other", managed, true), claudeStatusAbsent},
		"malformed":  {`[{"id":"demo@skills-dir"}]`, claudeStatusUnknown},
		"object":     {`{"plugins":[]}`, claudeStatusUnknown},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			if got := claudePluginStatus([]byte(test.body), "demo", managed); got != test.want {
				t.Fatalf("status=%d want=%d body=%s", got, test.want, test.body)
			}
		})
	}
}

func TestClaudeNativeIdentityRejectsUnmanagedSkillsDirCollision(t *testing.T) {
	t.Parallel()
	config := filepath.Join(t.TempDir(), "claude-config")
	root := filepath.Join(config, "skills")
	foreign := filepath.Join(root, "foreign-demo")
	writeIdentityFile(t, filepath.Join(foreign, ".claude-plugin", "plugin.json"), `{"name":"demo"}`)
	plan := identityPlan(root)
	plan.ActivePath = filepath.Join(root, "managed-demo")
	plan.NativeRegistryExecutable = "/test/bin/claude"
	runner := &identityRunner{result: legacyports.CommandResult{Stdout: []byte(claudeListing("demo", foreign, true))}}
	observation, err := (NativeIdentityObserver{Runner: runner}).ObserveNativeIdentity(context.Background(), domain.DetectedClient{ClientID: domain.ClientClaude}, plan, nil)
	if err != nil || observation.State != domain.NativeIdentityUnmanaged || !observation.NativeDiscoveryAttempted {
		t.Fatalf("observation=%+v err=%v", observation, err)
	}
}

func claudeListing(name, installPath string, enabled bool) string {
	return fmt.Sprintf(`[{"id":%q,"version":"1.0.0","scope":"user","enabled":%t,"installPath":%q,"mcpServers":{}}]`, name+"@skills-dir", enabled, installPath)
}
