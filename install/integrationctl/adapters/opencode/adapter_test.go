package opencode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func TestApplyInstallPatchesConfigAndCopiesAssets(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	sourceRoot := t.TempDir()
	mustWriteOpenCodeFile(t, filepath.Join(projectRoot, "opencode.json"), "{\n  // keep me\n  \"theme\": \"midnight\",\n  \"mcp\": {\"user-owned\":{\"type\":\"local\",\"command\":[\"node\",\"user.js\"]}},\n  \"plugin\": [\"@user/existing\"]\n}\n")
	mustWriteOpenCodeFile(t, filepath.Join(sourceRoot, "src", "mcp", "servers.yaml"), "api_version: v1\nservers:\n  context7:\n    type: stdio\n    stdio:\n      command: npx\n      args:\n        - -y\n        - '@upstash/context7-mcp'\n    targets:\n      - opencode\n")
	mustWriteOpenCodeFile(t, filepath.Join(sourceRoot, "src", "targets", "opencode", "package.yaml"), "plugins:\n  - '@acme/opencode-demo-plugin'\n")
	mustWriteOpenCodeFile(t, filepath.Join(sourceRoot, "src", "targets", "opencode", "default_agent.txt"), "reviewer\n")
	mustWriteOpenCodeFile(t, filepath.Join(sourceRoot, "src", "targets", "opencode", "instructions.yaml"), "- AGENTS.md\n")
	mustWriteOpenCodeFile(t, filepath.Join(sourceRoot, "src", "targets", "opencode", "permission.json"), "{\"bash\":\"ask\"}\n")
	mustWriteOpenCodeFile(t, filepath.Join(sourceRoot, "src", "targets", "opencode", "config.extra.json"), "{\"watcher\":{\"ignore\":[\"dist/**\"]}}\n")
	mustWriteOpenCodeFile(t, filepath.Join(sourceRoot, "src", "targets", "opencode", "plugins", "example.js"), "export const ExamplePlugin = async () => ({})\n")
	mustWriteOpenCodeFile(t, filepath.Join(sourceRoot, "src", "targets", "opencode", "package.json"), "{\n  \"name\": \"demo\",\n  \"private\": true\n}\n")
	mustWriteOpenCodeFile(t, filepath.Join(sourceRoot, "src", "skills", "demo", "SKILL.md"), "# Demo\n")

	adapter := Adapter{FS: fsadapter.OS{}, ProjectRoot: projectRoot, UserHome: t.TempDir()}
	result, err := adapter.ApplyInstall(context.Background(), ports.ApplyInput{
		Policy: domain.InstallPolicy{Scope: "project"},
		ResolvedSource: &ports.ResolvedSource{
			Kind:      "local_path",
			LocalPath: sourceRoot,
		},
	})
	if err != nil {
		t.Fatalf("apply install: %v", err)
	}
	if result.State != domain.InstallInstalled {
		t.Fatalf("state = %s, want installed", result.State)
	}
	body, err := os.ReadFile(filepath.Join(projectRoot, "opencode.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(body)
	for _, want := range []string{"\"theme\"", "midnight", "\"plugin\"", "\"mcp\"", "\"$schema\"", "\"default_agent\"", "reviewer", "\"instructions\"", "\"permission\"", "\"watcher\"", "\"@user/existing\"", "\"user-owned\""} {
		if !strings.Contains(text, want) {
			t.Fatalf("OpenCode config missing %q:\n%s", want, text)
		}
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".opencode", "plugins", "example.js")); err != nil {
		t.Fatalf("stat projected plugin: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".opencode", "package.json")); err != nil {
		t.Fatalf("stat projected package.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".opencode", "skills", "demo", "SKILL.md")); err != nil {
		t.Fatalf("stat projected skill: %v", err)
	}
}

func TestApplyUpdateRemovesStaleOwnedEntriesAndFiles(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	sourceRoot := t.TempDir()
	mustWriteOpenCodeFile(t, filepath.Join(projectRoot, "opencode.json"), "{\n  \"theme\": \"midnight\",\n  \"plugin\": [\"@user/existing\", \"@acme/opencode-demo-plugin\"],\n  \"mcp\": {\"user-owned\":{\"type\":\"local\",\"command\":[\"node\",\"user.js\"]},\"context7\":{\"type\":\"local\",\"command\":[\"npx\",\"-y\",\"@upstash/context7-mcp\"]}},\n  \"default_agent\": \"reviewer\"\n}\n")
	mustWriteOpenCodeFile(t, filepath.Join(projectRoot, ".opencode", "plugins", "example.js"), "old\n")
	mustWriteOpenCodeFile(t, filepath.Join(projectRoot, ".opencode", "plugins", "stale.js"), "stale\n")
	mustWriteOpenCodeFile(t, filepath.Join(projectRoot, ".opencode", "package.json"), "{\n  \"name\": \"demo-old\"\n}\n")
	mustWriteOpenCodeFile(t, filepath.Join(sourceRoot, "src", "targets", "opencode", "package.yaml"), "plugins:\n  - '@acme/opencode-next-plugin'\n")
	mustWriteOpenCodeFile(t, filepath.Join(sourceRoot, "src", "targets", "opencode", "plugins", "example.js"), "new\n")
	mustWriteOpenCodeFile(t, filepath.Join(sourceRoot, "src", "targets", "opencode", "package.json"), "{\n  \"name\": \"demo-next\"\n}\n")
	mustWriteOpenCodeFile(t, filepath.Join(sourceRoot, "src", "targets", "opencode", "permission.json"), "{\"edit\":\"allow\"}\n")

	adapter := Adapter{FS: fsadapter.OS{}, ProjectRoot: projectRoot, UserHome: t.TempDir()}
	result, err := adapter.ApplyUpdate(context.Background(), ports.ApplyInput{
		Record: &domain.InstallationRecord{
			Policy: domain.InstallPolicy{Scope: "project"},
			Targets: map[domain.TargetID]domain.TargetInstallation{
				domain.TargetOpenCode: {
					TargetID:           domain.TargetOpenCode,
					OwnedNativeObjects: ownedObjects(filepath.Join(projectRoot, "opencode.json"), []string{"default_agent"}, []string{"@acme/opencode-demo-plugin"}, []string{"context7"}, []string{filepath.Join(projectRoot, ".opencode", "plugins", "example.js"), filepath.Join(projectRoot, ".opencode", "plugins", "stale.js"), filepath.Join(projectRoot, ".opencode", "package.json")}, domain.ProtectionWorkspace),
				},
			},
		},
		ResolvedSource: &ports.ResolvedSource{Kind: "local_path", LocalPath: sourceRoot},
	})
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if result.State != domain.InstallInstalled {
		t.Fatalf("state = %s, want installed", result.State)
	}
	body, err := os.ReadFile(filepath.Join(projectRoot, "opencode.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(body)
	for _, want := range []string{"\"@user/existing\"", "\"@acme/opencode-next-plugin\"", "\"user-owned\"", "\"permission\""} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated config missing %q:\n%s", want, text)
		}
	}
	for _, notWant := range []string{"\"@acme/opencode-demo-plugin\"", "\"context7\"", "\"default_agent\""} {
		if strings.Contains(text, notWant) {
			t.Fatalf("updated config still contains %q:\n%s", notWant, text)
		}
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".opencode", "plugins", "stale.js")); !os.IsNotExist(err) {
		t.Fatalf("stale projected file still exists: %v", err)
	}
}

func TestApplyRemoveDeletesOnlyOwnedEntries(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	configPath := filepath.Join(projectRoot, "opencode.json")
	mustWriteOpenCodeFile(t, configPath, "{\n  \"theme\": \"midnight\",\n  \"plugin\": [\"@user/existing\", \"@acme/opencode-demo-plugin\"],\n  \"mcp\": {\"user-owned\":{\"type\":\"local\",\"command\":[\"node\",\"user.js\"]},\"context7\":{\"type\":\"local\",\"command\":[\"npx\",\"-y\",\"@upstash/context7-mcp\"]}},\n  \"permission\": {\"bash\":\"ask\"}\n}\n")
	mustWriteOpenCodeFile(t, filepath.Join(projectRoot, ".opencode", "plugins", "example.js"), "owned\n")

	adapter := Adapter{FS: fsadapter.OS{}, ProjectRoot: projectRoot, UserHome: t.TempDir()}
	_, err := adapter.ApplyRemove(context.Background(), ports.ApplyInput{
		Record: &domain.InstallationRecord{
			Policy: domain.InstallPolicy{Scope: "project"},
			Targets: map[domain.TargetID]domain.TargetInstallation{
				domain.TargetOpenCode: {
					TargetID:           domain.TargetOpenCode,
					OwnedNativeObjects: ownedObjects(configPath, []string{"permission"}, []string{"@acme/opencode-demo-plugin"}, []string{"context7"}, []string{filepath.Join(projectRoot, ".opencode", "plugins", "example.js")}, domain.ProtectionWorkspace),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("apply remove: %v", err)
	}
	body, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(body)
	for _, want := range []string{"\"theme\"", "midnight", "\"@user/existing\"", "\"user-owned\""} {
		if !strings.Contains(text, want) {
			t.Fatalf("remove config missing %q:\n%s", want, text)
		}
	}
	for _, notWant := range []string{"\"@acme/opencode-demo-plugin\"", "\"context7\"", "\"permission\""} {
		if strings.Contains(text, notWant) {
			t.Fatalf("remove config still contains %q:\n%s", notWant, text)
		}
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".opencode", "plugins", "example.js")); !os.IsNotExist(err) {
		t.Fatalf("owned projected file still exists: %v", err)
	}
}

func TestInspectDetectsVolatileOpenCodeOverrides(t *testing.T) {
	projectRoot := t.TempDir()
	adapter := Adapter{FS: fsadapter.OS{}, ProjectRoot: projectRoot, UserHome: t.TempDir()}
	t.Setenv("OPENCODE_CONFIG", filepath.Join(projectRoot, "custom.json"))
	t.Setenv("OPENCODE_CONFIG_DIR", filepath.Join(projectRoot, "custom-dir"))
	t.Setenv("OPENCODE_CONFIG_CONTENT", `{"plugin":["@volatile/demo"]}`)

	result, err := adapter.Inspect(context.Background(), ports.InspectInput{Scope: "project"})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !result.VolatileOverrideDetected {
		t.Fatal("expected volatile override detection")
	}
	found := false
	for _, restriction := range result.EnvironmentRestrictions {
		if restriction == domain.RestrictionVolatileOverride {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected volatile override restriction, got %v", result.EnvironmentRestrictions)
	}
}

func TestProjectScopeUsesNearestGitRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	projectRoot := filepath.Join(root, "repo", "nested", "pkg")
	mustWriteOpenCodeFile(t, filepath.Join(root, "repo", ".git"), "")
	adapter := Adapter{FS: fsadapter.OS{}, ProjectRoot: projectRoot, UserHome: t.TempDir()}

	got := adapter.configPath("project")
	want := filepath.Join(root, "repo", "opencode.json")
	if got != want {
		t.Fatalf("config path = %s, want %s", got, want)
	}
}

func mustWriteOpenCodeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
