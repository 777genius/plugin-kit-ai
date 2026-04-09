package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type stubRunner struct {
	commands []ports.Command
	run      func(ports.Command) (ports.CommandResult, error)
}

func (s *stubRunner) Run(ctx context.Context, cmd ports.Command) (ports.CommandResult, error) {
	s.commands = append(s.commands, cmd)
	if s.run != nil {
		return s.run(cmd)
	}
	return ports.CommandResult{ExitCode: 0}, nil
}

func TestApplyInstallLocalUsesManagedGeminiInstall(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	writeGeminiFile(t, filepath.Join(root, "src", "targets", "gemini", "package.yaml"), "context_file_name: \"GEMINI.md\"\nexclude_tools:\n  - \"run_shell_command(rm -rf)\"\nplan_directory: \".gemini/plans\"\n")
	writeGeminiFile(t, filepath.Join(root, "src", "targets", "gemini", "manifest.extra.json"), "{\n  \"plan\": {\"retentionDays\": 7},\n  \"x_galleryTopic\": \"gemini-cli-extension\"\n}\n")
	writeGeminiFile(t, filepath.Join(root, "src", "targets", "gemini", "contexts", "GEMINI.md"), "# Primary\n")
	writeGeminiFile(t, filepath.Join(root, "src", "targets", "gemini", "contexts", "RELEASE.md"), "# Extra\n")
	writeGeminiFile(t, filepath.Join(root, "src", "targets", "gemini", "settings", "release-profile.yaml"), "name: \"release-profile\"\ndescription: \"Release profile\"\nenv_var: \"RELEASE_PROFILE\"\nsensitive: false\n")
	writeGeminiFile(t, filepath.Join(root, "src", "targets", "gemini", "themes", "release-dawn.yaml"), "name: \"release-dawn\"\nbackground:\n  primary: \"#fff9f2\"\n")
	writeGeminiFile(t, filepath.Join(root, "src", "targets", "gemini", "commands", "deploy.toml"), "description = \"Deploy\"\n")
	writeGeminiFile(t, filepath.Join(root, "src", "targets", "gemini", "policies", "release-review.toml"), "[[rule]]\ntoolName = \"dangerous\"\ndecision = \"ask_user\"\n")
	writeGeminiFile(t, filepath.Join(root, "src", "targets", "gemini", "hooks", "hooks.json"), "{\n  \"hooks\": {}\n}\n")
	writeGeminiFile(t, filepath.Join(root, "src", "targets", "gemini", "agents", "reviewer.md"), "# reviewer\n")
	writeGeminiFile(t, filepath.Join(root, "src", "skills", "security-audit", "SKILL.md"), "# Skill\n")
	writeGeminiFile(t, filepath.Join(root, "src", "mcp", "servers.yaml"), "api_version: v1\nservers:\n  docs:\n    type: remote\n    remote:\n      protocol: streamable_http\n      url: \"https://example.com/mcp\"\n    targets:\n      - gemini\n  release-checks:\n    type: stdio\n    stdio:\n      command: node\n      args:\n        - ${package.root}/bin/release-checks.mjs\n    targets:\n      - gemini\n")
	runner := &stubRunner{}
	adapter := Adapter{Runner: runner, FS: fsadapter.OS{}, UserHome: home}

	result, err := adapter.ApplyInstall(context.Background(), ports.ApplyInput{
		Manifest: domain.IntegrationManifest{
			IntegrationID: "gemini-demo",
			Version:       "0.1.0",
			Description:   "Gemini demo extension",
			RequestedRef:  domain.RequestedSourceRef{Kind: "local_path", Value: root},
			ResolvedRef:   domain.ResolvedSourceRef{Kind: "local_path", Value: root},
		},
		ResolvedSource: &ports.ResolvedSource{
			Kind:      "local_path",
			LocalPath: root,
		},
		Policy: domain.InstallPolicy{Scope: "user", AutoUpdate: true},
	})
	if err != nil {
		t.Fatalf("apply install: %v", err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %+v", runner.commands)
	}
	got := runner.commands[0].Argv
	if len(got) != 4 || !equalStrings(got[:3], []string{"gemini", "extensions", "install"}) {
		t.Fatalf("argv = %#v", got)
	}
	managedRoot := filepath.Join(home, ".plugin-kit-ai", "materialized", "gemini", "gemini-demo")
	if got[3] != managedRoot {
		t.Fatalf("install path = %q, want %q", got[3], managedRoot)
	}
	manifestBody, err := os.ReadFile(filepath.Join(managedRoot, "gemini-extension.json"))
	if err != nil {
		t.Fatalf("read materialized manifest: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(manifestBody, &doc); err != nil {
		t.Fatalf("parse materialized manifest: %v", err)
	}
	if doc["name"] != "gemini-demo" || doc["version"] != "0.1.0" || doc["description"] != "Gemini demo extension" {
		t.Fatalf("materialized manifest identity = %#v", doc)
	}
	if doc["contextFileName"] != "GEMINI.md" {
		t.Fatalf("contextFileName = %#v", doc["contextFileName"])
	}
	if _, err := os.Stat(filepath.Join(managedRoot, "GEMINI.md")); err != nil {
		t.Fatalf("stat primary context: %v", err)
	}
	if _, err := os.Stat(filepath.Join(managedRoot, "contexts", "RELEASE.md")); err != nil {
		t.Fatalf("stat extra context: %v", err)
	}
	if _, err := os.Stat(filepath.Join(managedRoot, "commands", "deploy.toml")); err != nil {
		t.Fatalf("stat commands: %v", err)
	}
	if _, err := os.Stat(filepath.Join(managedRoot, "policies", "release-review.toml")); err != nil {
		t.Fatalf("stat policies: %v", err)
	}
	if _, err := os.Stat(filepath.Join(managedRoot, "hooks", "hooks.json")); err != nil {
		t.Fatalf("stat hooks: %v", err)
	}
	if _, err := os.Stat(filepath.Join(managedRoot, "skills", "security-audit", "SKILL.md")); err != nil {
		t.Fatalf("stat skills: %v", err)
	}
	if _, err := os.Stat(filepath.Join(managedRoot, "agents", "reviewer.md")); err != nil {
		t.Fatalf("stat agents: %v", err)
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if docs, _ := servers["docs"].(map[string]any); docs["httpUrl"] != "https://example.com/mcp" {
		t.Fatalf("docs projection = %#v", servers["docs"])
	}
	if checks, _ := servers["release-checks"].(map[string]any); checks["command"] != "node" {
		t.Fatalf("checks projection = %#v", servers["release-checks"])
	} else {
		args, _ := checks["args"].([]any)
		if len(args) != 1 || args[0] != "${extensionPath}/bin/release-checks.mjs" {
			t.Fatalf("checks args = %#v", checks["args"])
		}
	}
	if result.State != domain.InstallInstalled || result.ActivationState != domain.ActivationRestartPending {
		t.Fatalf("result = %+v", result)
	}
	if got := result.AdapterMetadata["materialized_source_root"]; got != managedRoot {
		t.Fatalf("materialized_source_root = %#v, want %q", got, managedRoot)
	}
}

func TestApplyInstallRemoteUsesGeminiInstallFlags(t *testing.T) {
	t.Parallel()
	runner := &stubRunner{}
	adapter := Adapter{Runner: runner, UserHome: t.TempDir()}

	_, err := adapter.ApplyInstall(context.Background(), ports.ApplyInput{
		Manifest: domain.IntegrationManifest{
			IntegrationID: "gemini-demo",
			RequestedRef:  domain.RequestedSourceRef{Kind: "git_url", Value: "https://github.com/acme/demo.git"},
			ResolvedRef:   domain.ResolvedSourceRef{Kind: "git_commit", Value: "https://github.com/acme/demo.git@abc123"},
		},
		ResolvedSource: &ports.ResolvedSource{
			Kind:      "git_url",
			LocalPath: t.TempDir(),
		},
		Policy: domain.InstallPolicy{Scope: "user", AutoUpdate: true, AllowPrerelease: true},
	})
	if err != nil {
		t.Fatalf("apply install: %v", err)
	}
	want := []string{"gemini", "extensions", "install", "https://github.com/acme/demo.git", "--auto-update", "--pre-release"}
	if got := runner.commands[0].Argv; !equalStrings(got, want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
}

func TestApplyUpdateUsesGeminiUpdate(t *testing.T) {
	t.Parallel()
	runner := &stubRunner{}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	sourceRoot := filepath.Join(root, "source")
	writeGeminiFile(t, filepath.Join(sourceRoot, "src", "plugin.yaml"), "api_version: v1\nname: gemini-demo\nversion: 0.2.0\ndescription: Gemini demo extension\ntargets:\n  - gemini\n")
	writeGeminiFile(t, filepath.Join(sourceRoot, "src", "targets", "gemini", "package.yaml"), "context_file_name: \"GEMINI.md\"\n")
	writeGeminiFile(t, filepath.Join(sourceRoot, "src", "targets", "gemini", "contexts", "GEMINI.md"), "# Updated\n")
	adapter := Adapter{Runner: runner, FS: fsadapter.OS{}, UserHome: home}

	result, err := adapter.ApplyUpdate(context.Background(), ports.ApplyInput{
		Manifest: domain.IntegrationManifest{
			IntegrationID: "gemini-demo",
			Version:       "0.2.0",
			Description:   "Gemini demo extension",
			ResolvedRef:   domain.ResolvedSourceRef{Kind: "git_commit", Value: "https://github.com/acme/demo.git@def456"},
		},
		ResolvedSource: &ports.ResolvedSource{Kind: "local_path", LocalPath: sourceRoot},
		Record: &domain.InstallationRecord{
			IntegrationID:      "gemini-demo",
			RequestedSourceRef: domain.RequestedSourceRef{Kind: "local_path", Value: sourceRoot},
			Targets: map[domain.TargetID]domain.TargetInstallation{
				domain.TargetGemini: {
					TargetID: domain.TargetGemini,
					AdapterMetadata: map[string]any{
						"materialized_source_root": filepath.Join(home, ".plugin-kit-ai", "materialized", "gemini", "gemini-demo"),
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	want := []string{"gemini", "extensions", "update", "gemini-demo"}
	if got := runner.commands[0].Argv; !equalStrings(got, want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
	body, err := os.ReadFile(filepath.Join(home, ".plugin-kit-ai", "materialized", "gemini", "gemini-demo", "GEMINI.md"))
	if err != nil || !strings.Contains(string(body), "Updated") {
		t.Fatalf("materialized root not refreshed: %v %q", err, body)
	}
	if result.State != domain.InstallInstalled {
		t.Fatalf("result = %+v", result)
	}
}

func TestApplyRemoveUsesGeminiUninstall(t *testing.T) {
	t.Parallel()
	runner := &stubRunner{}
	root := t.TempDir()
	managedRoot := filepath.Join(root, ".plugin-kit-ai", "materialized", "gemini", "gemini-demo")
	writeGeminiFile(t, filepath.Join(managedRoot, "gemini-extension.json"), "{}\n")
	adapter := Adapter{Runner: runner, UserHome: root}

	result, err := adapter.ApplyRemove(context.Background(), ports.ApplyInput{
		Record: &domain.InstallationRecord{
			IntegrationID: "gemini-demo",
			Targets: map[domain.TargetID]domain.TargetInstallation{
				domain.TargetGemini: {
					TargetID: domain.TargetGemini,
					AdapterMetadata: map[string]any{
						"materialized_source_root": managedRoot,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("apply remove: %v", err)
	}
	want := []string{"gemini", "extensions", "uninstall", "gemini-demo"}
	if got := runner.commands[0].Argv; !equalStrings(got, want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
	if result.State != domain.InstallRemoved {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(managedRoot); !os.IsNotExist(err) {
		t.Fatalf("managed root still exists: %v", err)
	}
}

func TestRepairUsesGeminiUpdateSemantics(t *testing.T) {
	t.Parallel()
	runner := &stubRunner{}
	adapter := Adapter{Runner: runner, UserHome: t.TempDir()}

	result, err := adapter.Repair(context.Background(), ports.RepairInput{
		Record: domain.InstallationRecord{IntegrationID: "gemini-demo", Policy: domain.InstallPolicy{Scope: "user"}},
		Manifest: &domain.IntegrationManifest{
			IntegrationID: "gemini-demo",
			ResolvedRef:   domain.ResolvedSourceRef{Kind: "git_commit", Value: "https://github.com/acme/demo.git@def456"},
		},
	})
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	want := []string{"gemini", "extensions", "update", "gemini-demo"}
	if got := runner.commands[0].Argv; !equalStrings(got, want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
	if result.State != domain.InstallInstalled {
		t.Fatalf("result = %+v", result)
	}
}

func TestApplyInstallLocalBuildsFromGeneratedGeminiRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	writeGeminiFile(t, filepath.Join(root, "gemini-extension.json"), "{\n  \"name\": \"gemini-demo\",\n  \"version\": \"0.1.0\",\n  \"description\": \"demo\",\n  \"contextFileName\": \"GEMINI.md\"\n}\n")
	writeGeminiFile(t, filepath.Join(root, "GEMINI.md"), "# Root\n")
	runner := &stubRunner{}
	adapter := Adapter{Runner: runner, FS: fsadapter.OS{}, UserHome: home}

	_, err := adapter.ApplyInstall(context.Background(), ports.ApplyInput{
		Manifest: domain.IntegrationManifest{
			IntegrationID: "gemini-demo",
			Version:       "0.1.0",
			Description:   "demo",
			RequestedRef:  domain.RequestedSourceRef{Kind: "local_path", Value: root},
		},
		ResolvedSource: &ports.ResolvedSource{
			Kind:      "local_path",
			LocalPath: root,
		},
	})
	if err != nil {
		t.Fatalf("apply install: %v", err)
	}
	if got := runner.commands[0].Argv; len(got) != 4 || got[2] != "install" {
		t.Fatalf("argv = %#v", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".plugin-kit-ai", "materialized", "gemini", "gemini-demo", "GEMINI.md")); err != nil {
		t.Fatalf("stat copied root context: %v", err)
	}
}

func TestRunGeminiReturnsMutationErrorOnFailure(t *testing.T) {
	t.Parallel()
	runner := &stubRunner{
		run: func(cmd ports.Command) (ports.CommandResult, error) {
			return ports.CommandResult{ExitCode: 1, Stderr: []byte("boom")}, nil
		},
	}
	adapter := Adapter{Runner: runner, FS: fsadapter.OS{}, UserHome: t.TempDir()}
	root := t.TempDir()
	writeGeminiFile(t, filepath.Join(root, "gemini-extension.json"), "{\"name\":\"gemini-demo\",\"version\":\"0.1.0\",\"description\":\"demo\"}\n")

	_, err := adapter.ApplyInstall(context.Background(), ports.ApplyInput{
		Manifest: domain.IntegrationManifest{
			IntegrationID: "gemini-demo",
			Version:       "0.1.0",
			Description:   "demo",
			RequestedRef:  domain.RequestedSourceRef{Kind: "local_path", Value: root},
		},
		ResolvedSource: &ports.ResolvedSource{Kind: "local_path", LocalPath: root},
	})
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.ErrMutationApply {
		t.Fatalf("err = %#v, want mutation_apply", err)
	}
}

func writeGeminiFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func stringsContain(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
