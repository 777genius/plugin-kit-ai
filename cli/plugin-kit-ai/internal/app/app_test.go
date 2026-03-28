package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall"
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
}

func TestInitRunner_claudeExtendedHooksRejectedOutsideClaude(t *testing.T) {
	t.Parallel()
	var r InitRunner
	_, err := r.Run(InitOptions{
		ProjectName:         "genplug",
		Platform:            "codex",
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
		filepath.Join("contexts", "GEMINI.md"),
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

func TestInitRunner_codex(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	got, err := r.Run(InitOptions{ProjectName: "genplug", Platform: "codex", OutputDir: out, Extras: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != out {
		t.Fatalf("out = %q, want %q", got, out)
	}
	for _, rel := range []string{
		"plugin.yaml",
		filepath.Join("targets", "codex", "package.yaml"),
		"AGENTS.md",
		filepath.Join(".codex-plugin", "plugin.json"),
		filepath.Join(".codex", "config.toml"),
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
}

func TestInitRunner_codexPython(t *testing.T) {
	t.Parallel()
	var r InitRunner
	out := filepath.Join(t.TempDir(), "genplug")
	got, err := r.Run(InitOptions{ProjectName: "genplug", Platform: "codex", Runtime: "python", OutputDir: out, Extras: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != out {
		t.Fatalf("out = %q, want %q", got, out)
	}
	for _, rel := range []string{
		"plugin.yaml",
		filepath.Join("targets", "codex", "package.yaml"),
		filepath.Join("src", "main.py"),
		filepath.Join("bin", "genplug"),
		"AGENTS.md",
		filepath.Join(".codex-plugin", "plugin.json"),
		filepath.Join(".codex", "config.toml"),
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, ".plugin-kit-ai", "project.toml")); !os.IsNotExist(err) {
		t.Fatalf("unsupported old manifest should not be generated, stat err = %v", err)
	}
}
