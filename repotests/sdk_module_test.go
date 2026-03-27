package pluginkitairepo_test

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSDKModule runs the full SDK test suite (module sdk/plugin-kit-ai).
// Repository root is a workspace-only module so `go test ./...` from the plugin-kit-ai
// repo root satisfies the iter1 DoD while keeping sdk/plugin-kit-ai/go.mod as in the plan.
func TestSDKModule(t *testing.T) {
	root := RepoRoot(t)
	sdkDir := filepath.Join(root, "sdk", "plugin-kit-ai")
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = sdkDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test in sdk/plugin-kit-ai: %v\n%s", err, out)
	}
}
