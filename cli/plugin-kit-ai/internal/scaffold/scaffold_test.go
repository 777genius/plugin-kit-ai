package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateProjectName(t *testing.T) {
	t.Parallel()
	ok := []string{"a", "ab", "my-plugin", "My_Plugin2"}
	for _, s := range ok {
		if err := ValidateProjectName(s); err != nil {
			t.Errorf("ValidateProjectName(%q) = %v, want nil", s, err)
		}
	}
	bad := []string{"", " ", "9bad", "bad@", "a" + strings.Repeat("b", 64)}
	for _, s := range bad {
		if err := ValidateProjectName(s); err == nil {
			t.Errorf("ValidateProjectName(%q) = nil, want error", s)
		}
	}
}

func TestLookupPlatform(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"claude", "codex"} {
		if _, ok := LookupPlatform(name); !ok {
			t.Fatalf("LookupPlatform(%q) = missing", name)
		}
	}
	if _, ok := LookupPlatform("gemini"); ok {
		t.Fatal("unexpected platform")
	}
}

func TestPaths_Codex(t *testing.T) {
	t.Parallel()
	got := Paths("codex", "my-plugin", true)
	for _, want := range []string{
		"go.mod",
		filepath.Join("cmd", "my-plugin", "main.go"),
		"plugin.yaml",
		"AGENTS.md",
		"README.md",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
		filepath.Join("commands", "my-plugin.md"),
	} {
		if !contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
}

func TestPathsForRuntime_CodexPython(t *testing.T) {
	t.Parallel()
	got := PathsForRuntime("codex", "python", "my-plugin", true)
	for _, want := range []string{
		"plugin.yaml",
		"AGENTS.md",
		filepath.Join("src", "main.py"),
		filepath.Join("bin", "my-plugin"),
		filepath.Join("bin", "my-plugin.cmd"),
		"README.md",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
		filepath.Join("commands", "my-plugin.md"),
	} {
		if !contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
}

func TestPaths_ClaudeCurrentState(t *testing.T) {
	t.Parallel()
	got := Paths("claude", "my-plugin", true)
	for _, want := range []string{
		"go.mod",
		filepath.Join("cmd", "my-plugin", "main.go"),
		"plugin.yaml",
		"README.md",
		"Makefile",
		".goreleaser.yml",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
		filepath.Join("commands", "my-plugin.md"),
	} {
		if !contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
}

func TestPathsForRuntime_ClaudeShell(t *testing.T) {
	t.Parallel()
	got := PathsForRuntime("claude", "shell", "my-plugin", true)
	for _, want := range []string{
		"plugin.yaml",
		filepath.Join("scripts", "main.sh"),
		filepath.Join("bin", "my-plugin"),
		filepath.Join("bin", "my-plugin.cmd"),
		"README.md",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
		filepath.Join("commands", "my-plugin.md"),
	} {
		if !contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
}

func TestWrite_Codex(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	err := Write(root, Data{
		ProjectName: "my-plugin",
		ModulePath:  DefaultModulePath("my-plugin"),
		Description: "plugin-kit-ai plugin",
		Platform:    "codex",
		WithExtras:  true,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"plugin.yaml",
		"AGENTS.md",
		filepath.Join("cmd", "my-plugin", "main.go"),
		filepath.Join("skills", "my-plugin", "SKILL.md"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
}

func TestWrite_ClaudeCreatesPluginManifest(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	err := Write(root, Data{
		ProjectName: "my-plugin",
		ModulePath:  DefaultModulePath("my-plugin"),
		Description: "plugin-kit-ai plugin",
		Platform:    "claude",
		WithExtras:  true,
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(root, "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, want := range []string{
		"schema_version: 1",
		`name: "my-plugin"`,
		`version: "0.1.0"`,
		`runtime: "go"`,
		`enabled:`,
		`- "claude"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plugin.yaml missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		`claude: {}`,
		`agents:`,
		`hooks:`,
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("plugin.yaml unexpectedly contains %q:\n%s", unwanted, got)
		}
	}
}

func TestWrite_CodexPythonIncludesPluginManifestAndLauncher(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	err := Write(root, Data{
		ProjectName: "my-plugin",
		Description: "plugin-kit-ai plugin",
		Platform:    "codex",
		Runtime:     "python",
		WithExtras:  true,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"plugin.yaml",
		"AGENTS.md",
		filepath.Join("src", "main.py"),
		filepath.Join("bin", "my-plugin"),
		filepath.Join("bin", "my-plugin.cmd"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
}

func TestWrite_ShellLauncherIsExecutable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	err := Write(root, Data{
		ProjectName: "my-plugin",
		Description: "plugin-kit-ai plugin",
		Platform:    "claude",
		Runtime:     "shell",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "bin", "my-plugin"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("launcher mode = %v, want executable", info.Mode())
	}
}

func TestRenderTemplate_PythonLauncherWindowsFallbackOrder(t *testing.T) {
	t.Parallel()
	body, _, err := RenderTemplate("python.launcher.cmd.tmpl", Data{})
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, want := range []string{
		`.venv\Scripts\python.exe`,
		`where python`,
		`where python3`,
		`plugin-kit-ai launcher: python not found`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("launcher missing %q:\n%s", want, got)
		}
	}
}

func TestRenderTemplate_ShellLauncherWindowsRequiresBash(t *testing.T) {
	t.Parallel()
	body, _, err := RenderTemplate("shell.launcher.cmd.tmpl", Data{})
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, want := range []string{
		`where bash`,
		`plugin-kit-ai launcher: bash not found`,
		`bash "%ROOT%\scripts\main.sh" %*`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("launcher missing %q:\n%s", want, got)
		}
	}
}

func TestRenderTemplate_ExecutableReadmesIncludeBootstrapGuidance(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		template string
		runtime  string
		wants    []string
	}{
		{
			name:     "claude-python",
			template: "README.executable.md.tmpl",
			runtime:  "python",
			wants: []string{
				"Status: `public-beta`, repo-local executable ABI",
				"system Python `3.10+`",
				"plugin-kit-ai validate . --platform claude --strict",
				"Do not write debug logs or human-readable status lines to stdout.",
			},
		},
		{
			name:     "codex-node",
			template: "codex.README.executable.md.tmpl",
			runtime:  "node",
			wants: []string{
				"system Node.js `20+`",
				"package-lock.json",
				"TypeScript remains a build-to-JavaScript path",
				"plugin-kit-ai validate . --platform codex --strict",
			},
		},
		{
			name:     "claude-shell",
			template: "README.executable.md.tmpl",
			runtime:  "shell",
			wants: []string{
				"POSIX shell on Unix",
				"`bash` in `PATH`",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			body, _, err := RenderTemplate(tc.template, Data{
				Runtime:    tc.runtime,
				Entrypoint: "./bin/demo",
			})
			if err != nil {
				t.Fatal(err)
			}
			got := string(body)
			for _, want := range tc.wants {
				if !strings.Contains(got, want) {
					t.Fatalf("template missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestRenderTemplate_GoReadmesIncludeStableContractGuidance(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		template string
		wants    []string
	}{
		{
			name:     "claude-go",
			template: "README.md.tmpl",
			wants: []string{
				"Status: `production-ready`, stable default path",
				"Bootstrap contract: Go `1.22+`",
				"plugin-kit-ai validate . --platform claude --strict",
				"Do not write debug logs or human-readable status lines to stdout.",
			},
		},
		{
			name:     "codex-go",
			template: "codex.README.md.tmpl",
			wants: []string{
				"Status: `production-ready`, stable default path",
				"Bootstrap contract: Go `1.22+`",
				"plugin-kit-ai validate . --platform codex --strict",
				"Keep stdout reserved for Codex responses; write diagnostics to stderr only.",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			body, _, err := RenderTemplate(tc.template, Data{
				ProjectName: "demo",
				Entrypoint:  "./bin/demo",
			})
			if err != nil {
				t.Fatal(err)
			}
			got := string(body)
			for _, want := range tc.wants {
				if !strings.Contains(got, want) {
					t.Fatalf("template missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func contains(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}
