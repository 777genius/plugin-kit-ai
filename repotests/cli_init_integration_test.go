package pluginkitairepo_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPluginKitAIInitGeneratesBuildableModule(t *testing.T) {
	for _, platform := range []string{"claude", "codex-runtime", "codex-package", "gemini", "cursor"} {
		t.Run(platform, func(t *testing.T) {
			root := RepoRoot(t)
			cliDir := filepath.Join(root, "cli", "plugin-kit-ai")

			binDir := t.TempDir()
			bin := filepath.Join(binDir, "plugin-kit-ai")
			build := exec.Command("go", "build", "-o", bin, "./cmd/plugin-kit-ai")
			build.Dir = cliDir
			if out, err := build.CombinedOutput(); err != nil {
				t.Fatalf("build plugin-kit-ai: %v\n%s", err, out)
			}

			plugRoot := t.TempDir()
			args := []string{"init", "genplug", "--platform", platform, "-o", plugRoot, "--extras"}
			if platform != "gemini" && platform != "codex-package" && platform != "cursor" {
				args = append(args, "--runtime", "go")
			}
			run := exec.Command(bin, args...)
			if out, err := run.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai init: %v\n%s", err, out)
			}
			if platform == "gemini" || platform == "codex-package" || platform == "cursor" {
				validate := exec.Command(bin, "validate", plugRoot, "--platform", platform)
				validate.Env = append(os.Environ(), "GOWORK=off")
				if out, err := validate.CombinedOutput(); err != nil {
					t.Fatalf("plugin-kit-ai validate: %v\n%s", err, out)
				}
				for _, rel := range []string{"launcher.yaml", "go.mod"} {
					if _, err := os.Stat(filepath.Join(plugRoot, rel)); !os.IsNotExist(err) {
						t.Fatalf("%s starter unexpectedly wrote %s", platform, rel)
					}
				}
				return
			}

			env := newGoModuleEnv(t)
			wireGeneratedGoModuleToLocalSDK(t, plugRoot, env)

			tidy := exec.Command("go", "mod", "tidy")
			tidy.Dir = plugRoot
			tidy.Env = env
			if out, err := tidy.CombinedOutput(); err != nil {
				t.Fatalf("go mod tidy in generated module: %v\n%s", err, out)
			}

			validate := exec.Command(bin, "validate", plugRoot, "--platform", platform)
			validate.Env = env
			if out, err := validate.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai validate: %v\n%s", err, out)
			}

			test := exec.Command("go", "test", "./...")
			test.Dir = plugRoot
			test.Env = env
			if out, err := test.CombinedOutput(); err != nil {
				t.Fatalf("go test in generated module: %v\n%s", err, out)
			}

			vet := exec.Command("go", "vet", "./...")
			vet.Dir = plugRoot
			vet.Env = env
			if out, err := vet.CombinedOutput(); err != nil {
				t.Fatalf("go vet in generated module: %v\n%s", err, out)
			}
		})
	}
}
