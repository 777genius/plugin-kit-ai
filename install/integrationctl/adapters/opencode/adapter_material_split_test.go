package opencode

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
)

func TestLoadSourceMaterialRejectsEmptyInstructionPath(t *testing.T) {
	t.Parallel()

	sourceRoot := t.TempDir()
	mustWriteOpenCodeFile(t, filepath.Join(sourceRoot, "src", "targets", "opencode", "instructions.yaml"), "- AGENTS.md\n- \"\"\n")

	adapter := Adapter{FS: fsadapter.OS{}, ProjectRoot: t.TempDir(), UserHome: t.TempDir()}
	_, err := adapter.loadSourceMaterial(context.Background(), sourceRoot, "project", adapter.ProjectRoot)
	if err == nil {
		t.Fatal("expected empty instruction path error")
	}
	if !strings.Contains(err.Error(), "OpenCode instructions must contain only non-empty paths") {
		t.Fatalf("error = %v", err)
	}
}
