package cursor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func TestApplyInstallPreservesUnmanagedEntries(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	sourceRoot := t.TempDir()
	mustWriteFile(t, filepath.Join(projectRoot, ".cursor", "mcp.json"), `{"mcpServers":{"user-owned":{"command":"node","args":["user.mjs"]}}}`)
	mustWriteFile(t, filepath.Join(sourceRoot, "src", "mcp", "servers.yaml"), "api_version: v1\nservers:\n  release-checks:\n    type: stdio\n    stdio:\n      command: node\n      args:\n        - ${package.root}/bin/release-checks.mjs\n")

	adapter := Adapter{
		FS:          fsadapter.OS{},
		ProjectRoot: projectRoot,
		UserHome:    t.TempDir(),
	}
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
	doc := readJSONFile(t, filepath.Join(projectRoot, ".cursor", "mcp.json"))
	servers := mustObject(t, doc["mcpServers"])
	if _, ok := servers["user-owned"]; !ok {
		t.Fatal("expected unmanaged Cursor MCP entry to remain")
	}
	releaseChecks := mustObject(t, servers["release-checks"])
	args := mustStringSlice(t, releaseChecks["args"])
	if len(args) != 1 || args[0] != filepath.Join(sourceRoot, "bin", "release-checks.mjs") {
		t.Fatalf("args = %#v, want interpolated package root", args)
	}
}

func TestApplyRemoveDeletesOnlyOwnedAliases(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	configPath := filepath.Join(projectRoot, ".cursor", "mcp.json")
	mustWriteFile(t, configPath, `{"mcpServers":{"user-owned":{"command":"node","args":["user.mjs"]},"release-checks":{"command":"node","args":["managed.mjs"]}}}`)

	adapter := Adapter{
		FS:          fsadapter.OS{},
		ProjectRoot: projectRoot,
		UserHome:    t.TempDir(),
	}
	record := domain.InstallationRecord{
		Policy: domain.InstallPolicy{Scope: "project"},
		Targets: map[domain.TargetID]domain.TargetInstallation{
			domain.TargetCursor: {
				TargetID: domain.TargetCursor,
				OwnedNativeObjects: []domain.NativeObjectRef{
					{Kind: "file", Path: configPath},
					{Kind: "cursor_mcp_server", Name: "release-checks", Path: configPath},
				},
			},
		},
	}
	_, err := adapter.ApplyRemove(context.Background(), ports.ApplyInput{Record: &record})
	if err != nil {
		t.Fatalf("apply remove: %v", err)
	}
	doc := readJSONFile(t, configPath)
	servers := mustObject(t, doc["mcpServers"])
	if _, ok := servers["release-checks"]; ok {
		t.Fatal("expected owned Cursor MCP entry to be removed")
	}
	if _, ok := servers["user-owned"]; !ok {
		t.Fatal("expected unmanaged Cursor MCP entry to remain")
	}
}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	return out
}

func mustObject(t *testing.T, value any) map[string]any {
	t.Helper()
	out, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value is %T, want object", value)
	}
	return out
}

func mustStringSlice(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value is %T, want []any", value)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			t.Fatalf("item is %T, want string", item)
		}
		out = append(out, s)
	}
	return out
}
