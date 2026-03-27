package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	_, err := r.Run(InitOptions{ProjectName: "okname", Platform: "gemini"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInitRunner_claudeCurrentState(t *testing.T) {
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
		filepath.Join("cmd", "genplug", "main.go"),
		filepath.Join(".claude-plugin", "plugin.json"),
		filepath.Join("hooks", "hooks.json"),
		"README.md",
		"Makefile",
		".goreleaser.yml",
		filepath.Join("skills", "genplug", "SKILL.md"),
		filepath.Join("commands", "genplug.md"),
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
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
		t.Fatalf("legacy manifest should not be generated, stat err = %v", err)
	}
}
