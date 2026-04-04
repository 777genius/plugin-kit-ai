package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
)

type fakeInspectRunner struct {
	report   pluginmanifest.Inspection
	warnings []pluginmanifest.Warning
	err      error
}

func (f fakeInspectRunner) Inspect(app.PluginInspectOptions) (pluginmanifest.Inspection, []pluginmanifest.Warning, error) {
	return f.report, f.warnings, f.err
}

func TestInspectTextShowsLauncherAndGeminiGuidance(t *testing.T) {
	t.Parallel()
	cmd := newInspectCmd(fakeInspectRunner{
		report: pluginmanifest.Inspection{
			Manifest: pluginmanifest.Manifest{
				Name:    "demo",
				Version: "0.1.0",
				Targets: []string{"gemini"},
			},
			Launcher: &pluginmanifest.Launcher{Runtime: "go", Entrypoint: "./bin/demo"},
			Targets: []pluginmanifest.InspectTarget{{
				Target:            "gemini",
				TargetClass:       "mcp_extension",
				ProductionClass:   "production-ready extension packaging lane",
				RuntimeContract:   "production-ready extension packaging plus optional production-ready 9-hook Go runtime",
				TargetNativeKinds: []string{"hooks", "contexts"},
				ManagedArtifacts:  []string{"gemini-extension.json", "hooks/hooks.json"},
			}},
		},
	})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--format", "text", "."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{
		"launcher: runtime=go entrypoint=./bin/demo",
		"next=go test ./...; plugin-kit-ai render --check .; plugin-kit-ai validate . --platform gemini --strict; gemini extensions link .",
		"runtime_gate=make test-gemini-runtime",
		"live_runtime_gate=make test-gemini-runtime-live",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("inspect output missing %q:\n%s", want, output)
		}
	}
}

func TestInspectTextShowsGeminiPackagingGuidanceWithoutLauncher(t *testing.T) {
	t.Parallel()
	cmd := newInspectCmd(fakeInspectRunner{
		report: pluginmanifest.Inspection{
			Manifest: pluginmanifest.Manifest{
				Name:    "demo",
				Version: "0.1.0",
				Targets: []string{"gemini"},
			},
			Targets: []pluginmanifest.InspectTarget{{
				Target:            "gemini",
				TargetClass:       "mcp_extension",
				ProductionClass:   "production-ready extension packaging lane",
				RuntimeContract:   "production-ready extension packaging plus optional production-ready 9-hook Go runtime",
				TargetNativeKinds: []string{"commands", "contexts"},
				ManagedArtifacts:  []string{"gemini-extension.json"},
			}},
		},
	})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--format", "text", "."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{
		"managed=gemini-extension.json",
		"next=render --check + validate --strict keep the packaging lane honest; add --runtime go when you want the Gemini production-ready 9-hook runtime",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("inspect output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "launcher: runtime=") {
		t.Fatalf("inspect output unexpectedly shows launcher:\n%s", output)
	}
}

func TestInspectTextIncludesNativeDocPathsForCodexLanes(t *testing.T) {
	root := t.TempDir()
	manifest := pluginmanifest.Default("codex-inspect", "codex-runtime", "go", "codex inspect", true)
	manifest.Targets = []string{"codex-package", "codex-runtime"}
	if err := pluginmanifest.Save(root, manifest, true); err != nil {
		t.Fatal(err)
	}
	if err := pluginmanifest.SaveLauncher(root, pluginmanifest.DefaultLauncher("codex-inspect", "go"), true); err != nil {
		t.Fatal(err)
	}
	mustWriteInspectFile(t, root, filepath.Join("targets", "codex-package", "package.yaml"), "homepage: https://example.com/demo\n")
	mustWriteInspectFile(t, root, filepath.Join("targets", "codex-package", "interface.json"), `{"defaultPrompt":["Inspect"]}`)
	mustWriteInspectFile(t, root, filepath.Join("targets", "codex-runtime", "package.yaml"), "model_hint: gpt-5.4-mini\n")
	mustWriteInspectFile(t, root, filepath.Join("targets", "codex-runtime", "config.extra.toml"), "approval_policy = \"never\"\n")

	var buf bytes.Buffer
	cmd := rootCmd
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"inspect", root, "--target", "all", "--format", "text"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{
		"docs=interface=targets/codex-package/interface.json,package_metadata=targets/codex-package/package.yaml",
		"docs=config_extra=targets/codex-runtime/config.extra.toml,package_metadata=targets/codex-runtime/package.yaml",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("inspect output missing %q:\n%s", want, output)
		}
	}
}

func TestInspectJSONUsesPortableContractShape(t *testing.T) {
	root := t.TempDir()
	mustWriteInspectFile(t, root, "plugin.yaml", "api_version: v1\nname: \"codex-inspect\"\nversion: \"0.1.0\"\ndescription: \"codex inspect\"\ntargets: [\"codex-runtime\"]\n")
	mustWriteInspectFile(t, root, "launcher.yaml", "runtime: go\nentrypoint: ./bin/codex-inspect\n")

	var buf bytes.Buffer
	cmd := rootCmd
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"inspect", root, "--target", "codex-runtime", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("inspect json parse: %v\n%s", err, buf.Bytes())
	}
	portable, ok := report["portable"].(map[string]any)
	if !ok {
		t.Fatalf("portable payload missing: %+v", report)
	}
	if _, ok := portable["items"].(map[string]any); !ok {
		t.Fatalf("portable.items missing or wrong shape: %+v", portable)
	}
	if _, found := portable["Items"]; found {
		t.Fatalf("portable should not expose legacy field name: %+v", portable)
	}
	if sourceFiles, ok := report["source_files"].([]any); !ok || len(sourceFiles) != 2 {
		t.Fatalf("source_files should be a non-null array with plugin and launcher, got %+v", report["source_files"])
	}
	targets, ok := report["targets"].([]any)
	if !ok || len(targets) != 1 {
		t.Fatalf("targets payload = %+v", report["targets"])
	}
	target, ok := targets[0].(map[string]any)
	if !ok {
		t.Fatalf("target payload = %+v", targets[0])
	}
	if _, ok := target["native_surface_tiers"].(map[string]any); !ok {
		t.Fatalf("native_surface_tiers missing from inspect json target: %+v", target)
	}
	if kinds, ok := target["target_native_kinds"].([]any); !ok || len(kinds) != 0 {
		t.Fatalf("target_native_kinds should be an empty array, got %+v", target["target_native_kinds"])
	}
	if kinds, ok := target["portable_kinds"].([]any); !ok || len(kinds) != 0 {
		t.Fatalf("portable_kinds should be an empty array, got %+v", target["portable_kinds"])
	}
}

func TestInspectHelpIncludesCursorTarget(t *testing.T) {
	t.Parallel()
	cmd := newInspectCmd(fakeInspectRunner{})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"cursor"`) {
		t.Fatalf("help output missing cursor target:\n%s", buf.String())
	}
}

func mustWriteInspectFile(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
