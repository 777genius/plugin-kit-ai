package platformexec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func TestValidateGeminiHookEntrypoints(t *testing.T) {
	body := []byte(`{
  "hooks": {
    "SessionStart": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiSessionStart"}]}],
    "SessionEnd": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiSessionEnd"}]}],
    "BeforeModel": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiBeforeModel"}]}],
    "AfterModel": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiAfterModel"}]}],
    "BeforeToolSelection": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiBeforeToolSelection"}]}],
    "BeforeAgent": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiBeforeAgent"}]}],
    "AfterAgent": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiAfterAgent"}]}],
    "BeforeTool": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiBeforeTool"}]}],
    "AfterTool": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiAfterTool"}]}]
  }
}`)
	mismatches, err := validateGeminiHookEntrypoints(body, "./bin/demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(mismatches) != 0 {
		t.Fatalf("mismatches = %v", mismatches)
	}
}

func TestValidateGeminiHookEntrypointsMismatch(t *testing.T) {
	body := []byte(`{
  "hooks": {
    "SessionStart": [{"matcher":"resume","hooks":[{"type":"command","command":"./bin/other GeminiSessionStart"}]}]
  }
}`)
	mismatches, err := validateGeminiHookEntrypoints(body, "./bin/demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(mismatches) == 0 {
		t.Fatal("expected mismatches")
	}
}

func TestGeminiExtensionDirBase(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	got := geminiExtensionDirBase(filepath.Join(cwd, "."))
	if got != filepath.Base(cwd) {
		t.Fatalf("base = %q, want %q", got, filepath.Base(cwd))
	}
}

func TestInferGeminiEntrypoint(t *testing.T) {
	t.Parallel()
	body := []byte(`{
  "hooks": {
    "SessionStart": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}demo GeminiSessionStart"}]}],
    "SessionEnd": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}demo GeminiSessionEnd"}]}]
  }
}`)
	got, ok := inferGeminiEntrypoint(body)
	if !ok {
		t.Fatal("expected entrypoint inference")
	}
	if got != "./bin/demo" {
		t.Fatalf("entrypoint = %q", got)
	}
}

func TestGeminiImportInfersEntrypointWhenLauncherSeeded(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{
  "hooks": {
    "SessionStart": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiSessionStart"}]}],
    "SessionEnd": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiSessionEnd"}]}],
    "BeforeModel": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiBeforeModel"}]}],
    "AfterModel": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiAfterModel"}]}],
    "BeforeToolSelection": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiBeforeToolSelection"}]}],
    "BeforeAgent": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiBeforeAgent"}]}],
    "AfterAgent": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiAfterAgent"}]}],
    "BeforeTool": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiBeforeTool"}]}],
    "AfterTool": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiAfterTool"}]}]
  }
}`)
	if err := os.WriteFile(filepath.Join(root, "hooks", "hooks.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	launcher := &pluginmodel.Launcher{Runtime: "go", Entrypoint: "./bin/placeholder"}
	result, err := (geminiAdapter{}).Import(root, ImportSeed{
		Manifest: pluginmodel.Manifest{Name: "demo", Version: "0.1.0", Description: "demo"},
		Launcher: launcher,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Launcher == nil {
		t.Fatal("expected launcher")
	}
	if result.Launcher.Entrypoint != "./bin/demo" {
		t.Fatalf("entrypoint = %q", result.Launcher.Entrypoint)
	}
}

func TestGeminiImportCreatesLauncherWhenHooksExposeEntrypoint(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{
  "hooks": {
    "SessionStart": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiSessionStart"}]}],
    "SessionEnd": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiSessionEnd"}]}],
    "BeforeModel": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiBeforeModel"}]}],
    "AfterModel": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiAfterModel"}]}],
    "BeforeToolSelection": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiBeforeToolSelection"}]}],
    "BeforeAgent": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiBeforeAgent"}]}],
    "AfterAgent": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiAfterAgent"}]}],
    "BeforeTool": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiBeforeTool"}]}],
    "AfterTool": [{"matcher":"*","hooks":[{"type":"command","command":"./bin/demo GeminiAfterTool"}]}]
  }
}`)
	if err := os.WriteFile(filepath.Join(root, "hooks", "hooks.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := (geminiAdapter{}).Import(root, ImportSeed{
		Manifest: pluginmodel.Manifest{Name: "demo", Version: "0.1.0", Description: "demo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Launcher == nil {
		t.Fatal("expected inferred launcher")
	}
	if result.Launcher.Runtime != "go" {
		t.Fatalf("runtime = %q", result.Launcher.Runtime)
	}
	if result.Launcher.Entrypoint != "./bin/demo" {
		t.Fatalf("entrypoint = %q", result.Launcher.Entrypoint)
	}
}

func TestGeminiRenderGeneratesDefaultHooksFromLauncher(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "targets", "gemini", "contexts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "targets", "gemini", "package.yaml"), []byte("context_file_name: GEMINI.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "targets", "gemini", "contexts", "GEMINI.md"), []byte("# Gemini\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	graph := pluginmodel.PackageGraph{
		Manifest: pluginmodel.Manifest{Name: "demo-gemini", Version: "0.1.0", Description: "demo", Targets: []string{"gemini"}},
		Launcher: &pluginmodel.Launcher{Runtime: "go", Entrypoint: "./bin/demo"},
	}
	state := pluginmodel.NewTargetState("gemini")
	state.SetDoc("package_metadata", filepath.Join("targets", "gemini", "package.yaml"))
	state.AddComponent("contexts", filepath.Join("targets", "gemini", "contexts", "GEMINI.md"))
	artifacts, err := (geminiAdapter{}).Render(root, graph, state)
	if err != nil {
		t.Fatal(err)
	}
	var hooksBody []byte
	for _, artifact := range artifacts {
		if artifact.RelPath == filepath.ToSlash(filepath.Join("hooks", "hooks.json")) {
			hooksBody = artifact.Content
			break
		}
	}
	if len(hooksBody) == 0 {
		t.Fatal("expected generated hooks/hooks.json")
	}
	for _, want := range []string{
		`${extensionPath}${/}bin${/}demo GeminiSessionStart`,
		`${extensionPath}${/}bin${/}demo GeminiSessionEnd`,
		`${extensionPath}${/}bin${/}demo GeminiBeforeTool`,
		`${extensionPath}${/}bin${/}demo GeminiAfterTool`,
	} {
		if !strings.Contains(string(hooksBody), want) {
			t.Fatalf("hooks/hooks.json missing %q:\n%s", want, hooksBody)
		}
	}
	if mismatches, err := validateGeminiHookEntrypoints(hooksBody, "./bin/demo"); err != nil {
		t.Fatal(err)
	} else if len(mismatches) != 0 {
		t.Fatalf("mismatches = %v", mismatches)
	}
}

func TestGeminiManagedPathsIncludesGeneratedHooksForDedicatedRuntimeRepo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "targets", "gemini", "contexts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "targets", "gemini", "package.yaml"), []byte("context_file_name: GEMINI.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "targets", "gemini", "contexts", "GEMINI.md"), []byte("# Gemini\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	graph := pluginmodel.PackageGraph{
		Manifest: pluginmodel.Manifest{Name: "demo-gemini", Version: "0.1.0", Description: "demo", Targets: []string{"gemini"}},
		Launcher: &pluginmodel.Launcher{Runtime: "go", Entrypoint: "./bin/demo"},
	}
	state := pluginmodel.NewTargetState("gemini")
	state.SetDoc("package_metadata", filepath.Join("targets", "gemini", "package.yaml"))
	state.AddComponent("contexts", filepath.Join("targets", "gemini", "contexts", "GEMINI.md"))
	paths, err := (geminiAdapter{}).ManagedPaths(root, graph, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("expected managed paths")
	}
	foundHooks := false
	foundContext := false
	for _, path := range paths {
		if path == "hooks/hooks.json" {
			foundHooks = true
		}
		if path == "GEMINI.md" {
			foundContext = true
		}
	}
	if !foundHooks {
		t.Fatalf("managed paths = %v, want hooks/hooks.json", paths)
	}
	if !foundContext {
		t.Fatalf("managed paths = %v, want GEMINI.md", paths)
	}
}
