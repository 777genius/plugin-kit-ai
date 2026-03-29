package pluginkitairepo_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStarterRepos_LayoutAndReadmesStayAligned(t *testing.T) {
	root := RepoRoot(t)
	landing := readRepoFile(t, root, "examples", "starters", "README.md")
	mustContain(t, landing, "# Canonical Starter Repos")
	mustContain(t, landing, "brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai")
	mustContain(t, landing, "npm i -g plugin-kit-ai")
	mustContain(t, landing, "pipx install plugin-kit-ai")
	mustContain(t, landing, "plugin-kit-ai doctor .")
	mustContain(t, landing, "plugin-kit-ai bootstrap .")
	mustContain(t, landing, "plugin-kit-ai validate . --platform <codex-runtime|claude> --strict")
	mustContain(t, landing, "plugin-kit-ai bundle publish . --platform <codex-runtime|claude> --repo owner/repo --tag v1.0.0")
	mustContain(t, landing, "plugin-kit-ai bundle fetch owner/repo --tag v1.0.0 --platform <codex-runtime|claude> --runtime <python|node> --dest ./handoff-plugin")
	mustContain(t, landing, "requirements.txt")
	mustContain(t, landing, "dist/main.js")

	cases := []struct {
		name          string
		platform      string
		runtime       string
		dir           string
		requiredFiles []string
	}{
		{
			name:     "codex-python-starter",
			platform: "codex-runtime",
			runtime:  "python",
			dir:      filepath.Join(root, "examples", "starters", "codex-python-starter"),
			requiredFiles: []string{
				"README.md",
				"plugin.yaml",
				"launcher.yaml",
				"requirements.txt",
				"src/main.py",
				"targets/codex-runtime/package.yaml",
				".codex/config.toml",
				".github/workflows/bundle-release.yml",
			},
		},
		{
			name:     "codex-node-typescript-starter",
			platform: "codex-runtime",
			runtime:  "node",
			dir:      filepath.Join(root, "examples", "starters", "codex-node-typescript-starter"),
			requiredFiles: []string{
				"README.md",
				"plugin.yaml",
				"launcher.yaml",
				"package.json",
				"tsconfig.json",
				"src/main.ts",
				"targets/codex-runtime/package.yaml",
				".codex/config.toml",
				".github/workflows/bundle-release.yml",
			},
		},
		{
			name:     "claude-python-starter",
			platform: "claude",
			runtime:  "python",
			dir:      filepath.Join(root, "examples", "starters", "claude-python-starter"),
			requiredFiles: []string{
				"README.md",
				"plugin.yaml",
				"launcher.yaml",
				"requirements.txt",
				"src/main.py",
				".claude-plugin/plugin.json",
				"hooks/hooks.json",
				"targets/claude/hooks/hooks.json",
				".github/workflows/bundle-release.yml",
			},
		},
		{
			name:     "claude-node-typescript-starter",
			platform: "claude",
			runtime:  "node",
			dir:      filepath.Join(root, "examples", "starters", "claude-node-typescript-starter"),
			requiredFiles: []string{
				"README.md",
				"plugin.yaml",
				"launcher.yaml",
				"package.json",
				"tsconfig.json",
				"src/main.ts",
				".claude-plugin/plugin.json",
				"hooks/hooks.json",
				"targets/claude/hooks/hooks.json",
				".github/workflows/bundle-release.yml",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, rel := range tc.requiredFiles {
				if !fileExists(filepath.Join(tc.dir, rel)) {
					t.Fatalf("starter missing %s", rel)
				}
			}

			readme := readRepoFile(t, tc.dir, "README.md")
			mustContain(t, readme, "## Who It Is For")
			mustContain(t, readme, "## Prerequisites")
			mustContain(t, readme, "## First Run")
			mustContain(t, readme, "plugin-kit-ai doctor .")
			mustContain(t, readme, "plugin-kit-ai bootstrap .")
			mustContain(t, readme, "plugin-kit-ai validate . --platform "+tc.platform+" --strict")
			mustContain(t, readme, "## Local Smoke")
			mustContain(t, readme, "## Ship It")
			mustContain(t, readme, "public-stable")
			mustContain(t, readme, "plugin-kit-ai bundle publish . --platform "+tc.platform+" --repo owner/repo --tag v1.0.0")
			mustContain(t, readme, "plugin-kit-ai bundle fetch owner/repo --tag v1.0.0 --platform "+tc.platform+" --runtime "+tc.runtime+" --dest ./handoff-plugin")

			switch tc.runtime {
			case "python":
				mustContain(t, readme, "requirements.txt")
				mustContain(t, readme, ".venv")
			case "node":
				mustContain(t, readme, "dist/main.js")
				mustContain(t, readme, "npm")
			}
		})
	}
}

func TestStarterRepos_Smoke(t *testing.T) {
	root := RepoRoot(t)
	pluginKitAIBin := buildPluginKitAI(t)

	cases := []struct {
		name     string
		source   string
		platform string
		ready    func() bool
		smoke    func(t *testing.T, workDir string)
	}{
		{
			name:     "codex-python-starter",
			source:   filepath.Join(root, "examples", "starters", "codex-python-starter"),
			platform: "codex-runtime",
			ready:    pythonRuntimeAvailable,
			smoke: func(t *testing.T, workDir string) {
				entry := localExampleEntrypointPath(workDir, "python")
				cmd := exec.Command(entry, "notify", `{"client":"codex-tui"}`)
				var stdout bytes.Buffer
				var stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("run codex python starter notify: %v\nstderr=%s", err, stderr.String())
				}
				if strings.TrimSpace(stdout.String()) != "" {
					t.Fatalf("stdout = %q, want empty", stdout.String())
				}
			},
		},
		{
			name:     "codex-node-typescript-starter",
			source:   filepath.Join(root, "examples", "starters", "codex-node-typescript-starter"),
			platform: "codex-runtime",
			ready:    nodeAndNPMAvailable,
			smoke: func(t *testing.T, workDir string) {
				entry := localExampleEntrypointPath(workDir, "node")
				cmd := exec.Command(entry, "notify", `{"client":"codex-tui"}`)
				var stdout bytes.Buffer
				var stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("run codex node starter notify: %v\nstderr=%s", err, stderr.String())
				}
				if strings.TrimSpace(stdout.String()) != "" {
					t.Fatalf("stdout = %q, want empty", stdout.String())
				}
			},
		},
		{
			name:     "claude-python-starter",
			source:   filepath.Join(root, "examples", "starters", "claude-python-starter"),
			platform: "claude",
			ready:    pythonRuntimeAvailable,
			smoke: func(t *testing.T, workDir string) {
				entry := localExampleEntrypointPath(workDir, "python")
				assertClaudeStableSubsetEntry(t, entry)
			},
		},
		{
			name:     "claude-node-typescript-starter",
			source:   filepath.Join(root, "examples", "starters", "claude-node-typescript-starter"),
			platform: "claude",
			ready:    nodeAndNPMAvailable,
			smoke: func(t *testing.T, workDir string) {
				entry := localExampleEntrypointPath(workDir, "node")
				assertClaudeStableSubsetEntry(t, entry)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if !tc.ready() {
				t.Skip("runtime not available")
			}

			workDir := filepath.Join(t.TempDir(), tc.name)
			copyTree(t, tc.source, workDir)

			doctor := exec.Command(pluginKitAIBin, "doctor", workDir)
			out, err := doctor.CombinedOutput()
			if err == nil {
				t.Fatalf("expected doctor to require bootstrap before starter first run:\n%s", out)
			}
			mustContain(t, string(out), "Status: needs_bootstrap")

			bootstrap := exec.Command(pluginKitAIBin, "bootstrap", workDir)
			if out, err := bootstrap.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai bootstrap starter: %v\n%s", err, out)
			}

			validate := exec.Command(pluginKitAIBin, "validate", workDir, "--platform", tc.platform, "--strict")
			if out, err := validate.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai validate starter: %v\n%s", err, out)
			}

			if tc.platform == "codex-runtime" {
				assertCodexConfig(t, workDir, "gpt-5.4-mini", "./bin/"+tc.name)
			}
			tc.smoke(t, workDir)

			if runtime.GOOS == "windows" && tc.platform == "codex-runtime" {
				mustContain(t, localExampleEntrypointPath(workDir, "node"), ".cmd")
			}
		})
	}
}
