package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/scaffold"
	"github.com/777genius/plugin-kit-ai/plugininstall"
)

type fakeInstaller struct {
	called bool
	res    plugininstall.Result
	err    error
}

func (f *fakeInstaller) Install(ctx context.Context, p plugininstall.Params) (plugininstall.Result, error) {
	f.called = true
	if p.Owner != "a" || p.Repo != "b" {
		return plugininstall.Result{}, errors.New("bad params")
	}
	return f.res, f.err
}

func TestInstallRunner_usesFake(t *testing.T) {
	t.Parallel()
	f := &fakeInstaller{}
	r := NewInstallRunner(f)
	_, err := r.Install(context.Background(), plugininstall.Params{Owner: "a", Repo: "b", Tag: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if !f.called {
		t.Fatal("fake not called")
	}
}

func TestNewInstallRunner_nilUsesFacadeType(t *testing.T) {
	t.Parallel()
	r := NewInstallRunner(nil)
	if _, ok := r.Installer.(plugininstallFacade); !ok {
		t.Fatalf("want plugininstallFacade, got %T", r.Installer)
	}
}

func TestInitRunner_unknownPlatform(t *testing.T) {
	t.Parallel()
	var r InitRunner
	_, err := r.Run(InitOptions{ProjectName: "okname", Platform: "bad-platform"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInitRunner_claudeStableDefault(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	got, err := r.Run(InitOptions{ProjectName: "genplug", Platform: "claude", OutputDir: out, Extras: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != out {
		t.Fatalf("out = %q, want %q", got, out)
	}
	for _, rel := range []string{
		"go.mod",
		"plugin.yaml",
		filepath.Join("targets", "claude", "hooks", "hooks.json"),
		filepath.Join("targets", "claude", "settings.json"),
		filepath.Join("targets", "claude", "lsp.json"),
		filepath.Join("targets", "claude", "user-config.json"),
		filepath.Join("targets", "claude", "manifest.extra.json"),
		filepath.Join("cmd", "genplug", "main.go"),
		filepath.Join(".claude-plugin", "plugin.json"),
		filepath.Join("hooks", "hooks.json"),
		"README.md",
		"Makefile",
		".goreleaser.yml",
		filepath.Join("skills", "genplug", "SKILL.md"),
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	assertRuntimeTestAssetsExist(t, out, "claude")
	hooksBody, err := os.ReadFile(filepath.Join(out, "targets", "claude", "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	hooks := string(hooksBody)
	for _, want := range []string{`"Stop"`, `"PreToolUse"`, `"UserPromptSubmit"`} {
		if !strings.Contains(hooks, want) {
			t.Fatalf("default Claude hooks missing %s:\n%s", want, hooks)
		}
	}
	for _, unwanted := range []string{`"SessionStart"`, `"WorktreeRemove"`} {
		if strings.Contains(hooks, unwanted) {
			t.Fatalf("default Claude hooks unexpectedly contain %s:\n%s", unwanted, hooks)
		}
	}
	mainBody, err := os.ReadFile(filepath.Join(out, "cmd", "genplug", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	mainGo := string(mainBody)
	if !strings.Contains(mainGo, "OnStop") || !strings.Contains(mainGo, "OnUserPromptSubmit") {
		t.Fatalf("default Claude main.go missing stable handlers:\n%s", mainGo)
	}
	if strings.Contains(mainGo, "OnSessionStart") {
		t.Fatalf("default Claude main.go unexpectedly contains extended handler:\n%s", mainGo)
	}
}

func TestInitRunner_claudeWithoutExtrasStaysMinimal(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	_, err := r.Run(InitOptions{ProjectName: "genplug", Platform: "claude", OutputDir: out})
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		filepath.Join("targets", "claude", "settings.json"),
		filepath.Join("targets", "claude", "lsp.json"),
		filepath.Join("targets", "claude", "user-config.json"),
		filepath.Join("targets", "claude", "manifest.extra.json"),
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to stay absent, err=%v", rel, err)
		}
	}
}

func TestInitRunner_claudeExtendedHooks(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	_, err := r.Run(InitOptions{
		ProjectName:         "genplug",
		Platform:            "claude",
		OutputDir:           out,
		ClaudeExtendedHooks: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	hooksBody, err := os.ReadFile(filepath.Join(out, "targets", "claude", "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	hooks := string(hooksBody)
	for _, want := range []string{`"Stop"`, `"SessionStart"`, `"WorktreeRemove"`} {
		if !strings.Contains(hooks, want) {
			t.Fatalf("extended Claude hooks missing %s:\n%s", want, hooks)
		}
	}
	mainBody, err := os.ReadFile(filepath.Join(out, "cmd", "genplug", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	mainGo := string(mainBody)
	for _, want := range []string{"OnStop", "OnSessionStart", "OnWorktreeRemove"} {
		if !strings.Contains(mainGo, want) {
			t.Fatalf("extended Claude main.go missing %s:\n%s", want, mainGo)
		}
	}
	assertRuntimeTestAssetsExist(t, out, "claude")
	for _, rel := range []string{
		filepath.Join("fixtures", "claude", "SessionStart.json"),
		filepath.Join("goldens", "claude", "SessionStart.stdout"),
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); !os.IsNotExist(err) {
			t.Fatalf("expected stable-only runtime test assets, but %s exists: %v", rel, err)
		}
	}
}

func TestInitRunner_claudeExtendedHooksRejectedOutsideClaude(t *testing.T) {
	t.Parallel()
	var r InitRunner
	_, err := r.Run(InitOptions{
		ProjectName:         "genplug",
		Platform:            "codex-runtime",
		OutputDir:           filepath.Join(t.TempDir(), "genplug"),
		ClaudeExtendedHooks: true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--claude-extended-hooks is only supported with --platform claude") {
		t.Fatalf("error = %q", err)
	}
}

func TestInitRunner_geminiPackagingStarter(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	got, err := r.Run(InitOptions{ProjectName: "genplug", Platform: "gemini", OutputDir: out})
	if err != nil {
		t.Fatal(err)
	}
	if got != out {
		t.Fatalf("out = %q, want %q", got, out)
	}
	for _, rel := range []string{
		"plugin.yaml",
		filepath.Join("targets", "gemini", "package.yaml"),
		filepath.Join("targets", "gemini", "contexts", "GEMINI.md"),
		"GEMINI.md",
		"gemini-extension.json",
		"README.md",
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	manifestBody, err := os.ReadFile(filepath.Join(out, "gemini-extension.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(manifestBody)
	for _, want := range []string{`"name": "genplug"`, `"contextFileName": "GEMINI.md"`} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("gemini manifest missing %q:\n%s", want, manifest)
		}
	}
	for _, rel := range []string{
		"go.mod",
		"launcher.yaml",
		filepath.Join("cmd", "genplug", "main.go"),
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); !os.IsNotExist(err) {
			t.Fatalf("unexpected gemini runtime scaffold file %s", rel)
		}
	}
	assertRuntimeTestAssetsAbsent(t, out)
}

func TestInitRunner_geminiPackagingStarterWithPortableMCP(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	_, err := r.Run(InitOptions{ProjectName: "genplug", Platform: "gemini", OutputDir: out, Extras: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		filepath.Join("mcp", "servers.yaml"),
		"gemini-extension.json",
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	mcpBody, err := os.ReadFile(filepath.Join(out, "mcp", "servers.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"format: plugin-kit-ai/mcp",
		`url: "https://example.com/mcp"`,
		"- \"gemini\"",
	} {
		if !strings.Contains(string(mcpBody), want) {
			t.Fatalf("gemini portable MCP missing %q:\n%s", want, mcpBody)
		}
	}
	manifestBody, err := os.ReadFile(filepath.Join(out, "gemini-extension.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"mcpServers"`, `"https://example.com/mcp"`} {
		if !strings.Contains(string(manifestBody), want) {
			t.Fatalf("gemini-extension.json missing %q:\n%s", want, manifestBody)
		}
	}
}

func TestInitRunner_geminiRejectsInvalidExtensionNameEarly(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "Demo_Extension")
	_, err := r.Run(InitOptions{ProjectName: "Demo_Extension", Platform: "gemini", OutputDir: out})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid Gemini extension name") {
		t.Fatalf("error = %q", err)
	}
}

func TestInitRunner_geminiRejectsRuntimeFlag(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	_, err := r.Run(InitOptions{ProjectName: "genplug", Platform: "gemini", Runtime: "python", OutputDir: out})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--runtime is not supported with --platform gemini") {
		t.Fatalf("error = %q", err)
	}
}

func TestInitRunner_opencodeWorkspaceStarter(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	got, err := r.Run(InitOptions{ProjectName: "genplug", Platform: "opencode", OutputDir: out, Extras: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != out {
		t.Fatalf("out = %q, want %q", got, out)
	}
	for _, rel := range []string{
		"plugin.yaml",
		filepath.Join("mcp", "servers.yaml"),
		filepath.Join("targets", "opencode", "package.yaml"),
		filepath.Join("targets", "opencode", "config.extra.json"),
		filepath.Join("targets", "opencode", "package.json"),
		filepath.Join("targets", "opencode", "commands", "genplug.md"),
		filepath.Join("targets", "opencode", "agents", "genplug.md"),
		filepath.Join("targets", "opencode", "themes", "genplug.json"),
		filepath.Join("targets", "opencode", "tools", "genplug.ts"),
		filepath.Join("targets", "opencode", "plugins", "example.js"),
		"opencode.json",
		"README.md",
		filepath.Join("skills", "genplug", "SKILL.md"),
		filepath.Join(".opencode", "skills", "genplug", "SKILL.md"),
		filepath.Join(".opencode", "commands", "genplug.md"),
		filepath.Join(".opencode", "agents", "genplug.md"),
		filepath.Join(".opencode", "themes", "genplug.json"),
		filepath.Join(".opencode", "tools", "genplug.ts"),
		filepath.Join(".opencode", "plugins", "example.js"),
		filepath.Join(".opencode", "package.json"),
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	for _, rel := range []string{
		"launcher.yaml",
		"go.mod",
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); !os.IsNotExist(err) {
			t.Fatalf("unexpected opencode starter file %s", rel)
		}
	}
	assertRuntimeTestAssetsAbsent(t, out)
	skillBody, err := os.ReadFile(filepath.Join(out, "skills", "genplug", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"name: genplug",
		"description: Portable shared skill stub for genplug.",
		"execution_mode: docs_only",
		"supported_agents:",
		"  - claude",
		"  - codex",
	} {
		if !strings.Contains(string(skillBody), want) {
			t.Fatalf("OpenCode skill stub missing %q:\n%s", want, skillBody)
		}
	}
	pluginBody, err := os.ReadFile(filepath.Join(out, "targets", "opencode", "plugins", "example.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pluginBody), "export const ExamplePlugin = async") {
		t.Fatalf("unexpected OpenCode plugin starter:\n%s", pluginBody)
	}
	if strings.Contains(string(pluginBody), "export default") {
		t.Fatalf("OpenCode plugin starter still uses deprecated export default shape:\n%s", pluginBody)
	}
	opencodeBody, err := os.ReadFile(filepath.Join(out, "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"mcp"`, `"https://example.com/mcp"`} {
		if !strings.Contains(string(opencodeBody), want) {
			t.Fatalf("opencode.json missing %q:\n%s", want, opencodeBody)
		}
	}
	packageJSONBody, err := os.ReadFile(filepath.Join(out, "targets", "opencode", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"@opencode-ai/plugin": "1.3.11"`, `"type": "module"`} {
		if !strings.Contains(string(packageJSONBody), want) {
			t.Fatalf("OpenCode package.json missing %q:\n%s", want, packageJSONBody)
		}
	}
	readmeBody, err := os.ReadFile(filepath.Join(out, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"official-style named async plugin exports",
		"targets/opencode/tools/",
		"@opencode-ai/plugin",
		"mcp/servers.yaml",
	} {
		if !strings.Contains(string(readmeBody), want) {
			t.Fatalf("OpenCode README missing %q:\n%s", want, readmeBody)
		}
	}
}

func TestInitRunner_opencodeRejectsRuntimeFlag(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	_, err := r.Run(InitOptions{ProjectName: "genplug", Platform: "opencode", Runtime: "python", OutputDir: out})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--runtime is not supported with --platform opencode") {
		t.Fatalf("error = %q", err)
	}
}

func TestInitRunner_cursorWorkspaceStarter(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	got, err := r.Run(InitOptions{ProjectName: "genplug", Platform: "cursor", OutputDir: out, Extras: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != out {
		t.Fatalf("out = %q, want %q", got, out)
	}
	for _, rel := range []string{
		"plugin.yaml",
		filepath.Join("mcp", "servers.yaml"),
		"README.md",
		filepath.Join("targets", "cursor", "rules", "project.mdc"),
		filepath.Join("targets", "cursor", "AGENTS.md"),
		filepath.Join(".cursor", "rules", "project.mdc"),
		"AGENTS.md",
		filepath.Join(".cursor", "mcp.json"),
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	for _, rel := range []string{
		"launcher.yaml",
		"go.mod",
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); !os.IsNotExist(err) {
			t.Fatalf("unexpected cursor starter file %s", rel)
		}
	}
	assertRuntimeTestAssetsAbsent(t, out)
	mcpBody, err := os.ReadFile(filepath.Join(out, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"https://example.com/mcp"`, `"type": "http"`} {
		if !strings.Contains(string(mcpBody), want) {
			t.Fatalf(".cursor/mcp.json missing %q:\n%s", want, mcpBody)
		}
	}
}

func TestInitRunner_cursorRejectsRuntimeFlag(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	_, err := r.Run(InitOptions{ProjectName: "genplug", Platform: "cursor", Runtime: "python", OutputDir: out})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--runtime is not supported with --platform cursor") {
		t.Fatalf("error = %q", err)
	}
}

func TestInitRunner_codexRuntime(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	got, err := r.Run(InitOptions{ProjectName: "genplug", Platform: "codex-runtime", OutputDir: out, Extras: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != out {
		t.Fatalf("out = %q, want %q", got, out)
	}
	for _, rel := range []string{
		"plugin.yaml",
		filepath.Join("targets", "codex-runtime", "package.yaml"),
		filepath.Join(".codex", "config.toml"),
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	assertRuntimeTestAssetsExist(t, out, "codex-runtime")
	for _, rel := range []string{
		filepath.Join(".codex-plugin", "plugin.json"),
		"AGENTS.md",
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); !os.IsNotExist(err) {
			t.Fatalf("unexpected codex runtime starter file %s", rel)
		}
	}
}

func TestInitRunner_codexRuntimePython(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	got, err := r.Run(InitOptions{ProjectName: "genplug", Platform: "codex-runtime", Runtime: "python", OutputDir: out, Extras: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != out {
		t.Fatalf("out = %q, want %q", got, out)
	}
	for _, rel := range []string{
		"plugin.yaml",
		filepath.Join("targets", "codex-runtime", "package.yaml"),
		filepath.Join("src", "main.py"),
		filepath.Join("bin", "genplug"),
		filepath.Join(".github", "workflows", "bundle-release.yml"),
		filepath.Join(".codex", "config.toml"),
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	assertRuntimeTestAssetsExist(t, out, "codex-runtime")
	for _, rel := range []string{
		filepath.Join(".codex-plugin", "plugin.json"),
		"AGENTS.md",
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); !os.IsNotExist(err) {
			t.Fatalf("unexpected codex runtime starter file %s", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(out, ".plugin-kit-ai", "project.toml")); !os.IsNotExist(err) {
		t.Fatalf("unsupported old manifest should not be generated, stat err = %v", err)
	}
}

func TestInitRunner_codexRuntimePythonRuntimePackage(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	got, err := r.Run(InitOptions{
		ProjectName:           "genplug",
		Platform:              "codex-runtime",
		Runtime:               "python",
		RuntimePackage:        true,
		RuntimePackageVersion: scaffold.DefaultRuntimePackageVersion,
		OutputDir:             out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != out {
		t.Fatalf("out = %q, want %q", got, out)
	}
	if _, err := os.Stat(filepath.Join(out, "src", "plugin_runtime.py")); !os.IsNotExist(err) {
		t.Fatalf("vendored helper should stay absent, stat err=%v", err)
	}
	reqBody, err := os.ReadFile(filepath.Join(out, "requirements.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reqBody), "plugin-kit-ai-runtime=="+scaffold.DefaultRuntimePackageVersion) {
		t.Fatalf("requirements.txt missing shared runtime package:\n%s", reqBody)
	}
	mainBody, err := os.ReadFile(filepath.Join(out, "src", "main.py"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainBody), "from plugin_kit_ai_runtime import") {
		t.Fatalf("main.py missing shared runtime import:\n%s", mainBody)
	}
}

func TestInitRunner_codexRuntimeNodeTypeScript(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	got, err := r.Run(InitOptions{
		ProjectName: "genplug",
		Platform:    "codex-runtime",
		Runtime:     "node",
		TypeScript:  true,
		OutputDir:   out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != out {
		t.Fatalf("out = %q, want %q", got, out)
	}
	for _, rel := range []string{
		"plugin.yaml",
		"launcher.yaml",
		"package.json",
		"tsconfig.json",
		filepath.Join("src", "main.ts"),
		filepath.Join("bin", "genplug"),
		filepath.Join(".codex", "config.toml"),
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	assertRuntimeTestAssetsExist(t, out, "codex-runtime")
}

func TestInitRunner_codexRuntimeNodeTypeScriptRuntimePackage(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	got, err := r.Run(InitOptions{
		ProjectName:           "genplug",
		Platform:              "codex-runtime",
		Runtime:               "node",
		TypeScript:            true,
		RuntimePackage:        true,
		RuntimePackageVersion: scaffold.DefaultRuntimePackageVersion,
		OutputDir:             out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != out {
		t.Fatalf("out = %q, want %q", got, out)
	}
	if _, err := os.Stat(filepath.Join(out, "src", "plugin-runtime.ts")); !os.IsNotExist(err) {
		t.Fatalf("vendored helper should stay absent, stat err=%v", err)
	}
	mainBody, err := os.ReadFile(filepath.Join(out, "src", "main.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainBody), `from "plugin-kit-ai-runtime"`) {
		t.Fatalf("main.ts missing shared runtime import:\n%s", mainBody)
	}
	pkgBody, err := os.ReadFile(filepath.Join(out, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pkgBody), `"plugin-kit-ai-runtime": "`+scaffold.DefaultRuntimePackageVersion+`"`) {
		t.Fatalf("package.json missing shared runtime dependency:\n%s", pkgBody)
	}
	assertRuntimeTestAssetsExist(t, out, "codex-runtime")
}

func assertRuntimeTestAssetsExist(t *testing.T, root, platform string) {
	t.Helper()
	var rels []string
	switch platform {
	case "claude":
		rels = []string{
			filepath.Join("fixtures", "claude", "Stop.json"),
			filepath.Join("fixtures", "claude", "PreToolUse.json"),
			filepath.Join("fixtures", "claude", "UserPromptSubmit.json"),
			filepath.Join("goldens", "claude", "Stop.stdout"),
			filepath.Join("goldens", "claude", "Stop.stderr"),
			filepath.Join("goldens", "claude", "Stop.exitcode"),
			filepath.Join("goldens", "claude", "PreToolUse.stdout"),
			filepath.Join("goldens", "claude", "PreToolUse.stderr"),
			filepath.Join("goldens", "claude", "PreToolUse.exitcode"),
			filepath.Join("goldens", "claude", "UserPromptSubmit.stdout"),
			filepath.Join("goldens", "claude", "UserPromptSubmit.stderr"),
			filepath.Join("goldens", "claude", "UserPromptSubmit.exitcode"),
		}
	case "codex-runtime":
		rels = []string{
			filepath.Join("fixtures", "codex-runtime", "Notify.json"),
			filepath.Join("goldens", "codex-runtime", "Notify.stdout"),
			filepath.Join("goldens", "codex-runtime", "Notify.stderr"),
			filepath.Join("goldens", "codex-runtime", "Notify.exitcode"),
		}
	default:
		t.Fatalf("unsupported platform %q", platform)
	}
	for _, rel := range rels {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
}

func assertRuntimeTestAssetsAbsent(t *testing.T, root string) {
	t.Helper()
	for _, rel := range []string{"fixtures", "goldens"} {
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to stay absent, err=%v", rel, err)
		}
	}
}

func TestInitRunner_claudeNodeExtrasEmitBundleReleaseWorkflow(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	_, err := r.Run(InitOptions{
		ProjectName: "genplug",
		Platform:    "claude",
		Runtime:     "node",
		OutputDir:   out,
		Extras:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	workflowBody, err := os.ReadFile(filepath.Join(out, ".github", "workflows", "bundle-release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(workflowBody)
	for _, want := range []string{
		"actions/setup-node@v6",
		"777genius/plugin-kit-ai/setup-plugin-kit-ai@v1",
		"plugin-kit-ai validate . --platform claude --strict",
		"plugin-kit-ai bundle publish . --platform claude --repo ${{ github.repository }} --tag ${{ github.ref_name }}",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("workflow missing %q:\n%s", want, workflow)
		}
	}
}

func TestInitRunner_TypeScriptRejectedOutsideNodeRuntime(t *testing.T) {
	t.Parallel()
	var r InitRunner
	_, err := r.Run(InitOptions{
		ProjectName: "genplug",
		Platform:    "codex-runtime",
		TypeScript:  true,
		OutputDir:   filepath.Join(t.TempDir(), "genplug"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--typescript requires --runtime node") {
		t.Fatalf("error = %q", err)
	}
}

func TestInitRunner_RuntimePackageRejectedOutsidePythonNode(t *testing.T) {
	t.Parallel()
	var r InitRunner
	_, err := r.Run(InitOptions{
		ProjectName:    "genplug",
		Platform:       "codex-runtime",
		RuntimePackage: true,
		OutputDir:      filepath.Join(t.TempDir(), "genplug"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--runtime-package requires --runtime python or --runtime node") {
		t.Fatalf("error = %q", err)
	}
}

func TestInitRunner_RuntimePackageVersionRejectedWithoutRuntimePackage(t *testing.T) {
	t.Parallel()
	var r InitRunner
	_, err := r.Run(InitOptions{
		ProjectName:           "genplug",
		Platform:              "codex-runtime",
		Runtime:               "python",
		RuntimePackageVersion: scaffold.DefaultRuntimePackageVersion,
		OutputDir:             filepath.Join(t.TempDir(), "genplug"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--runtime-package-version requires --runtime-package") {
		t.Fatalf("error = %q", err)
	}
}

func TestInitRunner_codexPackage(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	got, err := r.Run(InitOptions{ProjectName: "genplug", Platform: "codex-package", OutputDir: out, Extras: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != out {
		t.Fatalf("out = %q, want %q", got, out)
	}
	for _, rel := range []string{
		"plugin.yaml",
		filepath.Join("mcp", "servers.yaml"),
		filepath.Join("targets", "codex-package", "package.yaml"),
		filepath.Join(".codex-plugin", "plugin.json"),
		".mcp.json",
		filepath.Join("skills", "genplug", "SKILL.md"),
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	for _, rel := range []string{
		"launcher.yaml",
		filepath.Join(".codex", "config.toml"),
		"AGENTS.md",
		"go.mod",
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); !os.IsNotExist(err) {
			t.Fatalf("unexpected codex package starter file %s", rel)
		}
	}
	assertRuntimeTestAssetsAbsent(t, out)
	manifestBody, err := os.ReadFile(filepath.Join(out, ".codex-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"mcpServers": "./.mcp.json"`, `"name": "genplug"`} {
		if !strings.Contains(string(manifestBody), want) {
			t.Fatalf("codex plugin manifest missing %q:\n%s", want, manifestBody)
		}
	}
}
