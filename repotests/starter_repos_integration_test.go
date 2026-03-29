package pluginkitairepo_test

import (
	"bytes"
	"os"
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
	mustContain(t, landing, "stable Go, Python, and Node authoring on Codex and Claude")
	mustContain(t, landing, "brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai")
	mustContain(t, landing, "npm i -g plugin-kit-ai")
	mustContain(t, landing, "pipx install plugin-kit-ai")
	mustContain(t, landing, "plugin-kit-ai doctor .")
	mustContain(t, landing, "plugin-kit-ai bootstrap .")
	mustContain(t, landing, "github.com/777genius/plugin-kit-ai/sdk@v1.0.3")
	mustContain(t, landing, "go test ./...")
	mustContain(t, landing, "go build -o bin/<starter-name> ./cmd/<starter-name>")
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
		absent        []string
		contains      []string
	}{
		{
			name:     "codex-go-starter",
			platform: "codex-runtime",
			runtime:  "go",
			dir:      filepath.Join(root, "examples", "starters", "codex-go-starter"),
			requiredFiles: []string{
				"README.md",
				"plugin.yaml",
				"launcher.yaml",
				"go.mod",
				"cmd/codex-go-starter/main.go",
				"targets/codex-runtime/package.yaml",
				".codex/config.toml",
			},
			absent: []string{
				".github/workflows/bundle-release.yml",
				".claude-plugin/plugin.json",
				"hooks/hooks.json",
			},
			contains: []string{
				"SDK-first handlers",
				"go build -o bin/codex-go-starter ./cmd/codex-go-starter",
			},
		},
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
			absent: []string{
				".claude-plugin/plugin.json",
				"hooks/hooks.json",
			},
			contains: []string{
				"requirements.txt",
				".venv",
				"plugin-kit-ai bundle publish . --platform codex-runtime --repo owner/repo --tag v1.0.0",
				"plugin-kit-ai bundle fetch owner/repo --tag v1.0.0 --platform codex-runtime --runtime python --dest ./handoff-plugin",
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
			absent: []string{
				".claude-plugin/plugin.json",
				"hooks/hooks.json",
			},
			contains: []string{
				"dist/main.js",
				"npm",
				"plugin-kit-ai bundle publish . --platform codex-runtime --repo owner/repo --tag v1.0.0",
				"plugin-kit-ai bundle fetch owner/repo --tag v1.0.0 --platform codex-runtime --runtime node --dest ./handoff-plugin",
			},
		},
		{
			name:     "claude-go-starter",
			platform: "claude",
			runtime:  "go",
			dir:      filepath.Join(root, "examples", "starters", "claude-go-starter"),
			requiredFiles: []string{
				"README.md",
				"plugin.yaml",
				"launcher.yaml",
				"go.mod",
				"cmd/claude-go-starter/main.go",
				".claude-plugin/plugin.json",
				"hooks/hooks.json",
				"targets/claude/hooks/hooks.json",
			},
			absent: []string{
				".github/workflows/bundle-release.yml",
				".codex/config.toml",
				"targets/codex-runtime/package.yaml",
			},
			contains: []string{
				"SDK-first handlers",
				"go build -o bin/claude-go-starter ./cmd/claude-go-starter",
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
			absent: []string{
				".codex/config.toml",
				"targets/codex-runtime/package.yaml",
			},
			contains: []string{
				"requirements.txt",
				".venv",
				"plugin-kit-ai bundle publish . --platform claude --repo owner/repo --tag v1.0.0",
				"plugin-kit-ai bundle fetch owner/repo --tag v1.0.0 --platform claude --runtime python --dest ./handoff-plugin",
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
			absent: []string{
				".codex/config.toml",
				"targets/codex-runtime/package.yaml",
			},
			contains: []string{
				"dist/main.js",
				"npm",
				"plugin-kit-ai bundle publish . --platform claude --repo owner/repo --tag v1.0.0",
				"plugin-kit-ai bundle fetch owner/repo --tag v1.0.0 --platform claude --runtime node --dest ./handoff-plugin",
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
			for _, rel := range tc.absent {
				if fileExists(filepath.Join(tc.dir, rel)) {
					t.Fatalf("starter unexpectedly contains %s", rel)
				}
			}

			readme := readRepoFile(t, tc.dir, "README.md")
			mustContain(t, readme, "## Who It Is For")
			mustContain(t, readme, "## Prerequisites")
			mustContain(t, readme, "## First Run")
			mustContain(t, readme, "plugin-kit-ai validate . --platform "+tc.platform+" --strict")
			mustContain(t, readme, "## Local Smoke")
			mustContain(t, readme, "## Ship It")
			for _, needle := range tc.contains {
				mustContain(t, readme, needle)
			}

			switch tc.runtime {
			case "go":
				mustContain(t, readme, "go test ./...")
				mustContain(t, readme, "Production plugin examples")
				mustNotContain(t, readme, "plugin-kit-ai bundle fetch")
				mustNotContain(t, readme, "plugin-kit-ai bundle publish")
				mustNotContain(t, readme, "plugin-kit-ai bootstrap .")
			case "python", "node":
				mustContain(t, readme, "plugin-kit-ai doctor .")
				mustContain(t, readme, "plugin-kit-ai bootstrap .")
				mustContain(t, readme, "public-stable")
			}
		})
	}

	mustNotContain(t, readRepoFile(t, root, "examples", "starters", "codex-python-starter", "src", "main.py"), "handle_claude")
	mustNotContain(t, readRepoFile(t, root, "examples", "starters", "codex-python-starter", "src", "main.py"), "Stop|PreToolUse|UserPromptSubmit")
	mustNotContain(t, readRepoFile(t, root, "examples", "starters", "codex-node-typescript-starter", "src", "main.ts"), "handleClaude")
	mustNotContain(t, readRepoFile(t, root, "examples", "starters", "codex-node-typescript-starter", "src", "main.ts"), "PreToolUse")
	mustNotContain(t, readRepoFile(t, root, "examples", "starters", "claude-python-starter", "src", "main.py"), "notify")
	mustNotContain(t, readRepoFile(t, root, "examples", "starters", "claude-node-typescript-starter", "src", "main.ts"), "\"notify\"")
}

func TestStarterRepos_Smoke(t *testing.T) {
	root := RepoRoot(t)
	sdkDir := filepath.Join(root, "sdk", "plugin-kit-ai")
	pluginKitAIBin := buildPluginKitAI(t)

	cases := []struct {
		name      string
		source    string
		platform  string
		runtime   string
		ready     func() bool
		prepare   func(t *testing.T, workDir string)
		smoke     func(t *testing.T, workDir string)
		validate  func(t *testing.T, workDir string)
	}{
		{
			name:     "codex-go-starter",
			source:   filepath.Join(root, "examples", "starters", "codex-go-starter"),
			platform: "codex-runtime",
			runtime:  "go",
			ready:    goRuntimeAvailable,
			prepare: func(t *testing.T, workDir string) {
				goStarterPrepare(t, workDir, sdkDir, "codex-go-starter")
			},
			validate: func(t *testing.T, workDir string) {
				validate := exec.Command(pluginKitAIBin, "validate", workDir, "--platform", "codex-runtime", "--strict")
				validate.Env = append(os.Environ(), "GOWORK=off")
				if out, err := validate.CombinedOutput(); err != nil {
					t.Fatalf("plugin-kit-ai validate starter: %v\n%s", err, out)
				}
				assertCodexConfig(t, workDir, "gpt-5.4-mini", "./bin/codex-go-starter")
			},
			smoke: func(t *testing.T, workDir string) {
				entry := localExampleEntrypointPath(workDir, "go")
				cmd := exec.Command(entry, "notify", `{"client":"codex-tui"}`)
				var stdout bytes.Buffer
				var stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("run codex go starter notify: %v\nstderr=%s", err, stderr.String())
				}
				if strings.TrimSpace(stdout.String()) != "" {
					t.Fatalf("stdout = %q, want empty", stdout.String())
				}
			},
		},
		{
			name:     "codex-python-starter",
			source:   filepath.Join(root, "examples", "starters", "codex-python-starter"),
			platform: "codex-runtime",
			runtime:  "python",
			ready:    pythonRuntimeAvailable,
			validate: func(t *testing.T, workDir string) {
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

				validate := exec.Command(pluginKitAIBin, "validate", workDir, "--platform", "codex-runtime", "--strict")
				if out, err := validate.CombinedOutput(); err != nil {
					t.Fatalf("plugin-kit-ai validate starter: %v\n%s", err, out)
				}
				assertCodexConfig(t, workDir, "gpt-5.4-mini", "./bin/codex-python-starter")
			},
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
			runtime:  "node",
			ready:    nodeAndNPMAvailable,
			validate: func(t *testing.T, workDir string) {
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

				validate := exec.Command(pluginKitAIBin, "validate", workDir, "--platform", "codex-runtime", "--strict")
				if out, err := validate.CombinedOutput(); err != nil {
					t.Fatalf("plugin-kit-ai validate starter: %v\n%s", err, out)
				}
				assertCodexConfig(t, workDir, "gpt-5.4-mini", "./bin/codex-node-typescript-starter")
			},
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
				if runtime.GOOS == "windows" {
					mustContain(t, entry, ".cmd")
				}
			},
		},
		{
			name:     "claude-go-starter",
			source:   filepath.Join(root, "examples", "starters", "claude-go-starter"),
			platform: "claude",
			runtime:  "go",
			ready:    goRuntimeAvailable,
			prepare: func(t *testing.T, workDir string) {
				goStarterPrepare(t, workDir, sdkDir, "claude-go-starter")
			},
			validate: func(t *testing.T, workDir string) {
				validate := exec.Command(pluginKitAIBin, "validate", workDir, "--platform", "claude", "--strict")
				validate.Env = append(os.Environ(), "GOWORK=off")
				if out, err := validate.CombinedOutput(); err != nil {
					t.Fatalf("plugin-kit-ai validate starter: %v\n%s", err, out)
				}
			},
			smoke: func(t *testing.T, workDir string) {
				entry := localExampleEntrypointPath(workDir, "go")
				assertClaudeStableSubsetEntry(t, entry)
			},
		},
		{
			name:     "claude-python-starter",
			source:   filepath.Join(root, "examples", "starters", "claude-python-starter"),
			platform: "claude",
			runtime:  "python",
			ready:    pythonRuntimeAvailable,
			validate: func(t *testing.T, workDir string) {
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

				validate := exec.Command(pluginKitAIBin, "validate", workDir, "--platform", "claude", "--strict")
				if out, err := validate.CombinedOutput(); err != nil {
					t.Fatalf("plugin-kit-ai validate starter: %v\n%s", err, out)
				}
			},
			smoke: func(t *testing.T, workDir string) {
				entry := localExampleEntrypointPath(workDir, "python")
				assertClaudeStableSubsetEntry(t, entry)
			},
		},
		{
			name:     "claude-node-typescript-starter",
			source:   filepath.Join(root, "examples", "starters", "claude-node-typescript-starter"),
			platform: "claude",
			runtime:  "node",
			ready:    nodeAndNPMAvailable,
			validate: func(t *testing.T, workDir string) {
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

				validate := exec.Command(pluginKitAIBin, "validate", workDir, "--platform", "claude", "--strict")
				if out, err := validate.CombinedOutput(); err != nil {
					t.Fatalf("plugin-kit-ai validate starter: %v\n%s", err, out)
				}
			},
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

			if tc.prepare != nil {
				tc.prepare(t, workDir)
			}
			tc.validate(t, workDir)
			tc.smoke(t, workDir)
		})
	}
}

func goRuntimeAvailable() bool {
	_, err := exec.LookPath("go")
	return err == nil
}

func goStarterPrepare(t *testing.T, workDir, sdkDir, binaryName string) {
	t.Helper()

	replaceArg := "github.com/777genius/plugin-kit-ai/sdk=" + sdkDir
	modEdit := exec.Command("go", "mod", "edit", "-replace", replaceArg)
	modEdit.Dir = workDir
	modEdit.Env = append(os.Environ(), "GOWORK=off")
	if out, err := modEdit.CombinedOutput(); err != nil {
		t.Fatalf("go mod edit: %v\n%s", err, out)
	}

	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = workDir
	tidyCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := tidyCmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy starter: %v\n%s", err, out)
	}

	testCmd := exec.Command("go", "test", "./...")
	testCmd.Dir = workDir
	testCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := testCmd.CombinedOutput(); err != nil {
		t.Fatalf("go test starter: %v\n%s", err, out)
	}

	binName := filepath.Join("bin", binaryName)
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	build := exec.Command("go", "build", "-o", binName, "./cmd/"+binaryName)
	build.Dir = workDir
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build starter: %v\n%s", err, out)
	}
}
