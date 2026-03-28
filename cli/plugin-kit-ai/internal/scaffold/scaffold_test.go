package scaffold

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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
	for _, name := range []string{"claude", "codex", "gemini"} {
		if _, ok := LookupPlatform(name); !ok {
			t.Fatalf("LookupPlatform(%q) = missing", name)
		}
	}
	if _, ok := LookupPlatform("unknown"); ok {
		t.Fatal("unexpected platform")
	}
}

func TestPaths_Gemini(t *testing.T) {
	t.Parallel()
	got := Paths("gemini", "my-plugin", true)
	for _, want := range []string{
		"plugin.yaml",
		filepath.Join("targets", "gemini", "package.yaml"),
		filepath.Join("contexts", "GEMINI.md"),
		"README.md",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
	} {
		if !contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
}

func TestPaths_Codex(t *testing.T) {
	t.Parallel()
	got := Paths("codex", "my-plugin", true)
	for _, want := range []string{
		"go.mod",
		filepath.Join("cmd", "my-plugin", "main.go"),
		"plugin.yaml",
		"launcher.yaml",
		filepath.Join("targets", "codex", "package.yaml"),
		"AGENTS.md",
		"README.md",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
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
		"launcher.yaml",
		filepath.Join("targets", "codex", "package.yaml"),
		"AGENTS.md",
		filepath.Join("src", "main.py"),
		filepath.Join("bin", "my-plugin"),
		filepath.Join("bin", "my-plugin.cmd"),
		"README.md",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
	} {
		if !contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
}

func TestPaths_ClaudeStableDefault(t *testing.T) {
	t.Parallel()
	got := Paths("claude", "my-plugin", true)
	for _, want := range []string{
		"go.mod",
		filepath.Join("cmd", "my-plugin", "main.go"),
		"plugin.yaml",
		"launcher.yaml",
		filepath.Join("targets", "claude", "hooks", "hooks.json"),
		"README.md",
		"Makefile",
		".goreleaser.yml",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
	} {
		if !contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
}

func TestPathsForRuntime_GeminiIgnoresExecutableScaffolding(t *testing.T) {
	t.Parallel()
	got := PathsForRuntime("gemini", "python", "my-plugin", true)
	for _, want := range []string{
		"plugin.yaml",
		filepath.Join("targets", "gemini", "package.yaml"),
		filepath.Join("contexts", "GEMINI.md"),
		"README.md",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
	} {
		if !contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
	for _, unwanted := range []string{
		"launcher.yaml",
		filepath.Join("src", "main.py"),
		filepath.Join("bin", "my-plugin"),
		filepath.Join("bin", "my-plugin.cmd"),
	} {
		if contains(got, unwanted) {
			t.Fatalf("unexpected %q in %v", unwanted, got)
		}
	}
}

func TestPathsForRuntime_ClaudeShell(t *testing.T) {
	t.Parallel()
	got := PathsForRuntime("claude", "shell", "my-plugin", true)
	for _, want := range []string{
		"plugin.yaml",
		"launcher.yaml",
		filepath.Join("targets", "claude", "hooks", "hooks.json"),
		filepath.Join("scripts", "main.sh"),
		filepath.Join("bin", "my-plugin"),
		filepath.Join("bin", "my-plugin.cmd"),
		"README.md",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
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
		"launcher.yaml",
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
		`format: plugin-kit-ai/package`,
		`name: "my-plugin"`,
		`version: "0.1.0"`,
		`targets:`,
		`- "claude"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plugin.yaml missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		`schema_version:`,
		`components:`,
		`runtime:`,
		`entrypoint:`,
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("plugin.yaml unexpectedly contains %q:\n%s", unwanted, got)
		}
	}
}

func TestWrite_GeminiCreatesPackagingStarter(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	err := Write(root, Data{
		ProjectName: "my-plugin",
		ModulePath:  DefaultModulePath("my-plugin"),
		Description: "plugin-kit-ai plugin",
		Platform:    "gemini",
		WithExtras:  true,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"plugin.yaml",
		filepath.Join("targets", "gemini", "package.yaml"),
		filepath.Join("contexts", "GEMINI.md"),
		"README.md",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	body, err := os.ReadFile(filepath.Join(root, "targets", "gemini", "package.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `context_file_name: "GEMINI.md"`) {
		t.Fatalf("gemini package.yaml missing context_file_name:\n%s", body)
	}
	for _, rel := range []string{
		"launcher.yaml",
		"go.mod",
		filepath.Join("cmd", "my-plugin", "main.go"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Fatalf("unexpected Gemini starter file %s", rel)
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
		"launcher.yaml",
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
				"fastest path",
				"Status: `public-beta`, repo-local executable ABI",
				"system Python `3.10+`",
				"recreate it",
				".venv\\\\Scripts\\\\activate",
				"plugin-kit-ai validate . --platform claude --strict",
				"CI-grade readiness gate",
				"managed dependency installation or packaged distribution",
				"--claude-extended-hooks",
			},
		},
		{
			name:     "codex-node",
			template: "codex.README.executable.md.tmpl",
			runtime:  "node",
			wants: []string{
				"fastest path",
				"Status: `public-beta`, repo-local executable ABI",
				"system Node.js `20+`",
				"package-lock.json",
				"TypeScript remains a build-to-JavaScript path",
				"npm install && npm run build",
				"plugin-kit-ai validate . --platform codex --strict",
				"CI-grade readiness gate",
				"managed dependency installation or packaged distribution",
				"`AGENTS.md`: repository instructions for Codex",
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
				"long-term support, packaged distribution, and the clearest release story matter",
				"plugin-kit-ai validate . --platform claude --strict",
				"--claude-extended-hooks",
			},
		},
		{
			name:     "codex-go",
			template: "codex.README.md.tmpl",
			wants: []string{
				"Status: `production-ready`, stable default path",
				"Bootstrap contract: Go `1.22+`",
				"long-term support, packaged distribution, and the clearest release story matter",
				"plugin-kit-ai validate . --platform codex --strict",
				"## Stable Default",
				"`Notify`",
				"`targets/codex/package.yaml`: authored Codex package metadata",
				"Keep stdout reserved for Codex responses and write diagnostics to stderr only.",
			},
		},
		{
			name:     "gemini-go",
			template: "gemini.README.md.tmpl",
			wants: []string{
				"Platform family: `extension_package`",
				"Launcher contract: `none`",
				"Runtime claim: `packaging-only target`",
				"plugin-kit-ai render .",
				"plugin-kit-ai validate . --platform gemini --strict",
				"no `launcher.yaml`",
				"`targets/gemini/package.yaml`",
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

func TestBuildPlan_GeminiRejectsExplicitRuntime(t *testing.T) {
	t.Parallel()
	_, err := BuildPlan(Data{
		ProjectName: "my-plugin",
		Platform:    "gemini",
		Runtime:     "python",
	})
	if err == nil || !strings.Contains(err.Error(), "--runtime is not supported with --platform gemini") {
		t.Fatalf("err = %v", err)
	}
}

func TestRenderTemplate_CodexAgentsTemplatesStayActionOriented(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		template string
		data     Data
		wants    []string
	}{
		{
			name:     "codex-go",
			template: "codex.AGENTS.md.tmpl",
			data:     Data{ProjectName: "demo", Entrypoint: "./bin/demo"},
			wants: []string{
				"Codex project instructions:",
				"./bin/demo notify '{\"client\":\"codex-tui\"}'",
				"Put repository-specific operating instructions here.",
				"stdout and diagnostics on stderr",
			},
		},
		{
			name:     "codex-exec",
			template: "codex.AGENTS.executable.md.tmpl",
			data:     Data{ProjectName: "demo", Entrypoint: "./bin/demo"},
			wants: []string{
				"Codex project instructions:",
				"./bin/demo notify '{\"client\":\"codex-tui\"}'",
				"Put repository-specific operating instructions here.",
				"stdout and diagnostics on stderr",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			body, _, err := RenderTemplate(tc.template, tc.data)
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

func TestRenderTemplate_ClaudeHooksDefaultAndExtended(t *testing.T) {
	t.Parallel()
	defaultBody, _, err := RenderTemplate("targets.claude.hooks.json.tmpl", Data{Entrypoint: "./bin/demo"})
	if err != nil {
		t.Fatal(err)
	}
	defaultHooks := string(defaultBody)
	for _, want := range []string{`"Stop"`, `"PreToolUse"`, `"UserPromptSubmit"`} {
		if !strings.Contains(defaultHooks, want) {
			t.Fatalf("default hooks missing %q:\n%s", want, defaultHooks)
		}
	}
	for _, unwanted := range []string{`"SessionStart"`, `"WorktreeRemove"`} {
		if strings.Contains(defaultHooks, unwanted) {
			t.Fatalf("default hooks unexpectedly contain %q:\n%s", unwanted, defaultHooks)
		}
	}

	extendedBody, _, err := RenderTemplate("targets.claude.hooks.json.tmpl", Data{
		Entrypoint:          "./bin/demo",
		ClaudeExtendedHooks: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	extendedHooks := string(extendedBody)
	for _, want := range []string{`"Stop"`, `"SessionStart"`, `"WorktreeRemove"`} {
		if !strings.Contains(extendedHooks, want) {
			t.Fatalf("extended hooks missing %q:\n%s", want, extendedHooks)
		}
	}
}

func TestTemplateDirectoryContainsOnlyLiveScaffoldOrApprovedTemplates(t *testing.T) {
	t.Parallel()
	live := liveTemplateNames()
	approvedExternal := map[string]struct{}{
		"command.md.tmpl": {},
	}
	entries, err := fs.ReadDir(tmplFS, "templates")
	if err != nil {
		t.Fatal(err)
	}
	var unexpected []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, ok := live[name]; ok {
			continue
		}
		if _, ok := approvedExternal[name]; ok {
			continue
		}
		unexpected = append(unexpected, name)
	}
	sort.Strings(unexpected)
	if len(unexpected) > 0 {
		t.Fatalf("unexpected unreferenced templates: %v", unexpected)
	}
}

func liveTemplateNames() map[string]struct{} {
	out := map[string]struct{}{}
	for _, def := range generatedPlatforms {
		for _, file := range def.Files {
			out[file.Template] = struct{}{}
		}
	}
	for _, platform := range []string{"claude", "codex", "gemini"} {
		for _, runtime := range []string{RuntimePython, RuntimeNode, RuntimeShell} {
			for _, file := range filesFor(platform, runtime, true) {
				out[file.Template] = struct{}{}
			}
		}
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}
