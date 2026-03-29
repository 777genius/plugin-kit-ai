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
	for _, name := range []string{"claude", "codex-package", "codex-runtime", "gemini", "opencode"} {
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
		filepath.Join("targets", "gemini", "contexts", "GEMINI.md"),
		"README.md",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
	} {
		if !contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
}

func TestPaths_OpenCode(t *testing.T) {
	t.Parallel()
	got := Paths("opencode", "my-plugin", true)
	for _, want := range []string{
		"plugin.yaml",
		filepath.Join("targets", "opencode", "package.yaml"),
		filepath.Join("targets", "opencode", "config.extra.json"),
		"README.md",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
		filepath.Join("targets", "opencode", "commands", "my-plugin.md"),
		filepath.Join("targets", "opencode", "agents", "my-plugin.md"),
		filepath.Join("targets", "opencode", "themes", "my-plugin.json"),
		filepath.Join("targets", "opencode", "plugins", "example.js"),
		filepath.Join("targets", "opencode", "package.json"),
	} {
		if !contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
	for _, unwanted := range []string{"launcher.yaml", "go.mod"} {
		if contains(got, unwanted) {
			t.Fatalf("unexpected %q in %v", unwanted, got)
		}
	}
}

func TestPaths_CodexRuntime(t *testing.T) {
	t.Parallel()
	got := Paths("codex-runtime", "my-plugin", true)
	for _, want := range []string{
		"go.mod",
		filepath.Join("cmd", "my-plugin", "main.go"),
		"plugin.yaml",
		"launcher.yaml",
		filepath.Join("targets", "codex-runtime", "package.yaml"),
		"README.md",
	} {
		if !contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
}

func TestPaths_CodexPackage(t *testing.T) {
	t.Parallel()
	got := Paths("codex-package", "my-plugin", true)
	for _, want := range []string{
		"plugin.yaml",
		filepath.Join("targets", "codex-package", "package.yaml"),
		"README.md",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
	} {
		if !contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
	for _, unwanted := range []string{
		"go.mod",
		"launcher.yaml",
		filepath.Join("cmd", "my-plugin", "main.go"),
	} {
		if contains(got, unwanted) {
			t.Fatalf("unexpected %q in %v", unwanted, got)
		}
	}
}

func TestPathsForRuntime_CodexRuntimePython(t *testing.T) {
	t.Parallel()
	got := PathsForRuntime("codex-runtime", "python", "my-plugin", true)
	for _, want := range []string{
		"plugin.yaml",
		"launcher.yaml",
		filepath.Join("targets", "codex-runtime", "package.yaml"),
		filepath.Join("src", "main.py"),
		filepath.Join("bin", "my-plugin"),
		filepath.Join("bin", "my-plugin.cmd"),
		filepath.Join(".github", "workflows", "bundle-release.yml"),
		"README.md",
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
		filepath.Join("targets", "claude", "settings.json"),
		filepath.Join("targets", "claude", "lsp.json"),
		filepath.Join("targets", "claude", "user-config.json"),
		filepath.Join("targets", "claude", "manifest.extra.json"),
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
		filepath.Join("targets", "gemini", "contexts", "GEMINI.md"),
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

func TestPathsForRuntime_OpenCodeIgnoresExecutableScaffolding(t *testing.T) {
	t.Parallel()
	got := PathsForRuntime("opencode", "python", "my-plugin", true)
	for _, want := range []string{
		"plugin.yaml",
		filepath.Join("targets", "opencode", "package.yaml"),
		filepath.Join("targets", "opencode", "config.extra.json"),
		"README.md",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
		filepath.Join("targets", "opencode", "commands", "my-plugin.md"),
		filepath.Join("targets", "opencode", "agents", "my-plugin.md"),
		filepath.Join("targets", "opencode", "themes", "my-plugin.json"),
		filepath.Join("targets", "opencode", "plugins", "example.js"),
		filepath.Join("targets", "opencode", "package.json"),
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
		filepath.Join("targets", "claude", "settings.json"),
		filepath.Join("targets", "claude", "lsp.json"),
		filepath.Join("targets", "claude", "user-config.json"),
		filepath.Join("targets", "claude", "manifest.extra.json"),
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

func TestWrite_CodexRuntime(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	err := Write(root, Data{
		ProjectName: "my-plugin",
		ModulePath:  DefaultModulePath("my-plugin"),
		Description: "plugin-kit-ai plugin",
		Platform:    "codex-runtime",
		WithExtras:  true,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"plugin.yaml",
		"launcher.yaml",
		filepath.Join("cmd", "my-plugin", "main.go"),
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

func TestWrite_OpenCodeCreatesMinimalWorkspaceLane(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	err := Write(root, Data{
		ProjectName: "my-plugin",
		ModulePath:  DefaultModulePath("my-plugin"),
		Description: "plugin-kit-ai plugin",
		Platform:    "opencode",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"plugin.yaml",
		filepath.Join("targets", "opencode", "package.yaml"),
		"README.md",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	for _, rel := range []string{
		"launcher.yaml",
		filepath.Join("targets", "opencode", "config.extra.json"),
		filepath.Join("skills", "my-plugin", "SKILL.md"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to stay absent, err=%v", rel, err)
		}
	}
}

func TestWrite_OpenCodeExtrasCreateCompatibleSkillStub(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	err := Write(root, Data{
		ProjectName: "my-plugin",
		ModulePath:  DefaultModulePath("my-plugin"),
		Description: "plugin-kit-ai plugin",
		Platform:    "opencode",
		WithExtras:  true,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "skills", "my-plugin", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"name: my-plugin",
		"description: OpenCode-compatible skill stub for my-plugin.",
		"execution_mode: docs_only",
		"supported_agents:",
		"  - opencode",
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("OpenCode skill stub missing %q:\n%s", want, body)
		}
	}
	for _, rel := range []string{
		filepath.Join("targets", "opencode", "commands", "my-plugin.md"),
		filepath.Join("targets", "opencode", "agents", "my-plugin.md"),
		filepath.Join("targets", "opencode", "themes", "my-plugin.json"),
		filepath.Join("targets", "opencode", "plugins", "example.js"),
		filepath.Join("targets", "opencode", "package.json"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
}

func TestPaths_ClaudeWithoutExtrasStaysMinimal(t *testing.T) {
	t.Parallel()
	got := Paths("claude", "my-plugin", false)
	for _, unwanted := range []string{
		filepath.Join("targets", "claude", "settings.json"),
		filepath.Join("targets", "claude", "lsp.json"),
		filepath.Join("targets", "claude", "user-config.json"),
		filepath.Join("targets", "claude", "manifest.extra.json"),
	} {
		if contains(got, unwanted) {
			t.Fatalf("unexpected %q in %v", unwanted, got)
		}
	}
}

func TestWrite_ClaudeExtrasCreateOptionalJSONDocs(t *testing.T) {
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
	for _, rel := range []string{
		filepath.Join("targets", "claude", "settings.json"),
		filepath.Join("targets", "claude", "lsp.json"),
		filepath.Join("targets", "claude", "user-config.json"),
		filepath.Join("targets", "claude", "manifest.extra.json"),
	} {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if strings.TrimSpace(string(body)) != "{}" {
			t.Fatalf("%s = %q, want {}", rel, string(body))
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
		filepath.Join("targets", "gemini", "contexts", "GEMINI.md"),
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

func TestWrite_CodexRuntimePythonIncludesLauncher(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	err := Write(root, Data{
		ProjectName: "my-plugin",
		Description: "plugin-kit-ai plugin",
		Platform:    "codex-runtime",
		Runtime:     "python",
		WithExtras:  true,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"plugin.yaml",
		"launcher.yaml",
		filepath.Join("src", "main.py"),
		filepath.Join("bin", "my-plugin"),
		filepath.Join("bin", "my-plugin.cmd"),
		filepath.Join(".github", "workflows", "bundle-release.yml"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	workflowBody, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "bundle-release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(workflowBody)
	for _, want := range []string{
		"actions/setup-python@v5",
		"777genius/plugin-kit-ai/setup-plugin-kit-ai@v1",
		"plugin-kit-ai doctor .",
		"plugin-kit-ai bootstrap .",
		"plugin-kit-ai validate . --platform codex-runtime --strict",
		"plugin-kit-ai bundle publish . --platform codex-runtime --repo ${{ github.repository }} --tag ${{ github.ref_name }}",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("python workflow missing %q:\n%s", want, workflow)
		}
	}
}

func TestWrite_CodexRuntimeNodeTypeScriptIncludesBuiltOutputShape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	err := Write(root, Data{
		ProjectName: "my-plugin",
		Description: "plugin-kit-ai plugin",
		Platform:    "codex-runtime",
		Runtime:     "node",
		TypeScript:  true,
		WithExtras:  true,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"plugin.yaml",
		"launcher.yaml",
		"package.json",
		"tsconfig.json",
		filepath.Join("src", "main.ts"),
		filepath.Join("bin", "my-plugin"),
		filepath.Join("bin", "my-plugin.cmd"),
		filepath.Join(".github", "workflows", "bundle-release.yml"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	body, err := os.ReadFile(filepath.Join(root, "bin", "my-plugin"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "dist/main.js") {
		t.Fatalf("launcher does not point at built output:\n%s", body)
	}
	workflowBody, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "bundle-release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(workflowBody)
	for _, want := range []string{
		"actions/setup-node@v6",
		"777genius/plugin-kit-ai/setup-plugin-kit-ai@v1",
		"plugin-kit-ai doctor .",
		"plugin-kit-ai bootstrap .",
		"plugin-kit-ai validate . --platform codex-runtime --strict",
		"plugin-kit-ai bundle publish . --platform codex-runtime --repo ${{ github.repository }} --tag ${{ github.ref_name }}",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("node workflow missing %q:\n%s", want, workflow)
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
				"plugin-kit-ai bootstrap .",
				"plugin-kit-ai validate . --platform claude --strict",
				"CI-grade readiness gate",
				"managed dependency installation or packaged distribution",
				"--claude-extended-hooks",
			},
		},
		{
			name:     "codex-runtime-node",
			template: "codex-runtime.README.executable.md.tmpl",
			runtime:  "node",
			wants: []string{
				"Status: `public-beta`, repo-local executable ABI",
				"system Node.js `20+`",
				"package-lock.json",
				"Minimal JavaScript runtime scaffold using `src/main.mjs`",
				"--runtime node --typescript",
				"plugin-kit-ai bootstrap .",
				"plugin-kit-ai validate . --platform codex-runtime --strict",
				"CI-grade readiness gate",
				"local notify integration",
				"`targets/codex-runtime/package.yaml`: authored Codex runtime metadata",
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

func TestRenderTemplate_NodeTypeScriptScaffoldTemplates(t *testing.T) {
	t.Parallel()
	cases := []struct {
		template string
		wants    []string
	}{
		{
			template: "codex-runtime.README.executable.md.tmpl",
			wants: []string{
				"Generated TypeScript scaffold: `src/main.ts`, `tsconfig.json`, and built output under `dist/main.js`",
				"`plugin-kit-ai bootstrap .` runs `npm install` and `npm run build`",
			},
		},
		{
			template: "README.executable.md.tmpl",
			wants: []string{
				"Generated TypeScript scaffold: `src/main.ts`, `tsconfig.json`, and built output under `dist/main.js`",
				"`plugin-kit-ai bootstrap .` runs `npm install` and `npm run build`",
			},
		},
	}
	for _, tc := range cases {
		body, _, err := RenderTemplate(tc.template, Data{
			Runtime:    "node",
			TypeScript: true,
			Entrypoint: "./bin/demo",
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range tc.wants {
			if !strings.Contains(string(body), want) {
				t.Fatalf("%s missing %q:\n%s", tc.template, want, body)
			}
		}
	}
	body, _, err := RenderTemplate("node.package.json.tmpl", Data{
		ProjectName: "demo",
		TypeScript:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, want := range []string{`"build": "tsc -p tsconfig.json"`, `"typescript": "^5.9.0"`, `"@types/node"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("package template missing %q:\n%s", want, got)
		}
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
			name:     "codex-package",
			template: "codex-package.README.md.tmpl",
			wants: []string{
				"Package lane: `codex-package`",
				"Status: `production-ready` package contract",
				"Launcher: not used",
				"plugin-kit-ai validate . --platform codex-package --strict",
				"`targets/codex-package/manifest.extra.json`: official Codex manifest passthrough",
				".codex-plugin/plugin.json",
			},
		},
		{
			name:     "codex-runtime",
			template: "codex-runtime.README.md.tmpl",
			wants: []string{
				"Status: `production-ready`, stable default path",
				"Bootstrap contract: Go `1.22+`",
				"repo-local Codex notify integration",
				"plugin-kit-ai validate . --platform codex-runtime --strict",
				"## Stable Default",
				"`Notify`",
				"`targets/codex-runtime/package.yaml`: authored Codex runtime metadata",
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
		{
			name:     "opencode-go",
			template: "opencode.README.md.tmpl",
			wants: []string{
				"Target lane: `opencode`",
				"Platform family: `code_plugin`",
				"Launcher: not used",
				"plugin-kit-ai validate . --platform opencode --strict",
				"`targets/opencode/package.yaml`",
				"`targets/opencode/config.extra.json`",
				"`targets/opencode/commands/`",
				"`targets/opencode/agents/`",
				"`targets/opencode/themes/`",
				"`targets/opencode/plugins/`",
				"`targets/opencode/package.json`",
				"`opencode.json`",
				"official-style named async plugin exports",
				"`@opencode-ai/plugin`",
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

func TestRenderTemplate_OpenCodePluginStarterUsesOfficialShape(t *testing.T) {
	t.Parallel()
	body, _, err := RenderTemplate("opencode.plugin.js.tmpl", Data{ProjectName: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if !strings.Contains(got, "export const ExamplePlugin = async") {
		t.Fatalf("starter missing official named async export:\n%s", got)
	}
	if strings.Contains(got, "export default") {
		t.Fatalf("starter still uses deprecated export default shape:\n%s", got)
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

func TestBuildPlan_OpenCodeRejectsExplicitRuntime(t *testing.T) {
	t.Parallel()
	_, err := BuildPlan(Data{
		ProjectName: "my-plugin",
		Platform:    "opencode",
		Runtime:     "python",
	})
	if err == nil || !strings.Contains(err.Error(), "--runtime is not supported with --platform opencode") {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildPlan_TypeScriptRequiresNodeRuntime(t *testing.T) {
	t.Parallel()
	_, err := BuildPlan(Data{
		ProjectName: "my-plugin",
		Platform:    "codex-runtime",
		TypeScript:  true,
	})
	if err == nil || !strings.Contains(err.Error(), "--typescript requires --runtime node") {
		t.Fatalf("err = %v", err)
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
	for _, platform := range []string{"claude", "codex-package", "codex-runtime", "gemini", "opencode"} {
		for _, runtime := range []string{RuntimePython, RuntimeNode, RuntimeShell} {
			for _, file := range filesFor(platform, runtime, true, false) {
				out[file.Template] = struct{}{}
			}
			for _, file := range filesFor(platform, runtime, true, true) {
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
