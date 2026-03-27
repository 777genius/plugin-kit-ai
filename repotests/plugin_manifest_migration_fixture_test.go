package pluginkitairepo_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginKitAIMigrationFixtures_RoundTripToStrictValidation(t *testing.T) {
	root := RepoRoot(t)
	pluginKitAIBin := buildPluginKitAI(t)
	sdkDir := filepath.Join(root, "sdk", "plugin-kit-ai")

	cases := []struct {
		name     string
		fixture  string
		platform string
	}{
		{name: "legacy_codex_go", fixture: "legacy_codex_go", platform: "codex"},
		{name: "legacy_claude_go", fixture: "legacy_claude_go", platform: "claude"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := filepath.Join(root, "repotests", "testdata", "plugin_manifest_migration", tc.fixture)
			dstParent := t.TempDir()
			dst := filepath.Join(dstParent, tc.fixture)
			copyTree(t, src, dst)

			replaceArg := "github.com/plugin-kit-ai/plugin-kit-ai/sdk=" + sdkDir
			modEdit := exec.Command("go", "mod", "edit", "-replace", replaceArg)
			modEdit.Dir = dst
			modEdit.Env = append(os.Environ(), "GOWORK=off")
			if out, err := modEdit.CombinedOutput(); err != nil {
				t.Fatalf("go mod edit: %v\n%s", err, out)
			}

			importCmd := exec.Command(pluginKitAIBin, "import", dst, "--from", tc.platform, "--force")
			importCmd.Env = append(os.Environ(), "GOWORK=off")
			importOut, err := importCmd.CombinedOutput()
			if err != nil {
				t.Fatalf("plugin-kit-ai import: %v\n%s", err, importOut)
			}
			if tc.platform == "codex" && !strings.Contains(string(importOut), "Warning: ignored unsupported import asset: .mcp.json") {
				t.Fatalf("expected import warning for codex fixture:\n%s", importOut)
			}

			normalizeCmd := exec.Command(pluginKitAIBin, "normalize", dst)
			normalizeCmd.Env = append(os.Environ(), "GOWORK=off")
			if out, err := normalizeCmd.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai normalize: %v\n%s", err, out)
			}

			renderCmd := exec.Command(pluginKitAIBin, "render", dst, "--target", "all")
			renderCmd.Env = append(os.Environ(), "GOWORK=off")
			if out, err := renderCmd.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai render: %v\n%s", err, out)
			}

			validateCmd := exec.Command(pluginKitAIBin, "validate", dst, "--platform", tc.platform, "--strict")
			validateCmd.Env = append(os.Environ(), "GOWORK=off")
			if out, err := validateCmd.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai validate --strict: %v\n%s", err, out)
			}

			testCmd := exec.Command("go", "test", "./...")
			testCmd.Dir = dst
			testCmd.Env = append(os.Environ(), "GOWORK=off")
			if out, err := testCmd.CombinedOutput(); err != nil {
				t.Fatalf("go test fixture: %v\n%s", err, out)
			}
		})
	}
}
