package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
)

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
	manifest := pluginmanifest.Default("codex-inspect", "codex-runtime", "go", "codex inspect", true)
	if err := pluginmanifest.Save(root, manifest, true); err != nil {
		t.Fatal(err)
	}
	if err := pluginmanifest.SaveLauncher(root, pluginmanifest.DefaultLauncher("codex-inspect", "go"), true); err != nil {
		t.Fatal(err)
	}
	mustWriteInspectFile(t, root, filepath.Join("targets", "codex-runtime", "package.yaml"), "model_hint: gpt-5.4-mini\n")

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
