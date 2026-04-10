package pathpolicy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func TestNormalizeScope(t *testing.T) {
	t.Parallel()
	if got := NormalizeScope("project"); got != "project" {
		t.Fatalf("NormalizeScope(project) = %q", got)
	}
	if got := NormalizeScope(" user "); got != "user" {
		t.Fatalf("NormalizeScope(user) = %q", got)
	}
}

func TestProjectRootPrefersWorkspaceThenProjectRoot(t *testing.T) {
	t.Parallel()
	if got := ProjectRoot("/tmp/workspace", "/tmp/project"); got != filepath.Clean("/tmp/workspace") {
		t.Fatalf("ProjectRoot workspace = %q", got)
	}
	if got := ProjectRoot("", "/tmp/project"); got != filepath.Clean("/tmp/project") {
		t.Fatalf("ProjectRoot project = %q", got)
	}
}

func TestEffectiveGitRootWalksToRepositoryBoundary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "nested", "repo")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := EffectiveGitRoot(workspace, ""); got != root {
		t.Fatalf("EffectiveGitRoot = %q, want %q", got, root)
	}
}

func TestPreferredExistingPathPrefersExistingCandidate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	first := filepath.Join(root, "one.json")
	second := filepath.Join(root, "two.json")
	if err := os.WriteFile(second, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := PreferredExistingPath(first, second); got != second {
		t.Fatalf("PreferredExistingPath = %q, want %q", got, second)
	}
}

func TestWorkspaceRootFromRecordProjectOnly(t *testing.T) {
	t.Parallel()
	project := domain.InstallationRecord{
		Policy:        domain.InstallPolicy{Scope: "project"},
		WorkspaceRoot: "/tmp/workspace",
	}
	if got := WorkspaceRootFromRecord(project); got != "/tmp/workspace" {
		t.Fatalf("WorkspaceRootFromRecord(project) = %q", got)
	}
	user := domain.InstallationRecord{
		Policy:        domain.InstallPolicy{Scope: "user"},
		WorkspaceRoot: "/tmp/workspace",
	}
	if got := WorkspaceRootFromRecord(user); got != "" {
		t.Fatalf("WorkspaceRootFromRecord(user) = %q", got)
	}
}

func TestWorkspaceRootFromInputsUsesRecordOnly(t *testing.T) {
	t.Parallel()
	record := domain.InstallationRecord{
		Policy:        domain.InstallPolicy{Scope: "project"},
		WorkspaceRoot: "/tmp/workspace",
	}
	if got := WorkspaceRootFromInspect(ports.InspectInput{Record: &record}); got != "/tmp/workspace" {
		t.Fatalf("WorkspaceRootFromInspect = %q", got)
	}
	if got := WorkspaceRootFromApply(ports.ApplyInput{Record: &record}); got != "/tmp/workspace" {
		t.Fatalf("WorkspaceRootFromApply = %q", got)
	}
}

func TestProtectionForScope(t *testing.T) {
	t.Parallel()
	if got := ProtectionForScope("project"); got != domain.ProtectionWorkspace {
		t.Fatalf("ProtectionForScope(project) = %q", got)
	}
	if got := ProtectionForScope("user"); got != domain.ProtectionUserMutable {
		t.Fatalf("ProtectionForScope(user) = %q", got)
	}
}
