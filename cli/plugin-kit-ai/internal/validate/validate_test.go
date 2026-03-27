package validate

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/pluginmanifest"
)

func TestValidate_ManifestMissing(t *testing.T) {
	t.Parallel()
	_, err := Validate(t.TempDir(), "")
	var re *ReportError
	if !errors.As(err, &re) {
		t.Fatalf("expected ReportError, got %v", err)
	}
	if got := re.Report.Failures[0].Kind; got != FailureManifestMissing {
		t.Fatalf("failure kind = %q", got)
	}
	if re.Error() != "required manifest missing: plugin.yaml" {
		t.Fatalf("error = %q", re.Error())
	}
}

func TestValidate_LegacyProjectManifestRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, filepath.Join(".plugin-kit-ai", "project.toml"), "schema_version = 1\nplatform = \"codex\"\nruntime = \"shell\"\nexecution_mode = \"launcher\"\nentrypoint = \"./bin/x\"\n")

	_, err := Validate(dir, "")
	var re *ReportError
	if !errors.As(err, &re) {
		t.Fatalf("expected ReportError, got %v", err)
	}
	if got := re.Report.Failures[0].Kind; got != FailureManifestInvalid {
		t.Fatalf("failure kind = %q", got)
	}
	if !strings.Contains(re.Error(), ".plugin-kit-ai/project.toml is not supported") {
		t.Fatalf("error = %q", re.Error())
	}
}

func TestFindPython_UsesPlatformAwareLookupOrder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if runtime.GOOS == "windows" {
		venv := filepath.Join(root, ".venv", "Scripts", "python.exe")
		mustWriteValidateFile(t, root, filepath.Join(".venv", "Scripts", "python.exe"), "binary")
		got, err := findPython(root)
		if err != nil {
			t.Fatal(err)
		}
		if got.Path != venv {
			t.Fatalf("findPython = %q, want %q", got.Path, venv)
		}
		return
	}
	venv := filepath.Join(root, ".venv", "bin", "python3")
	mustWriteValidateFile(t, root, filepath.Join(".venv", "bin", "python3"), "binary")
	got, err := findPython(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != venv {
		t.Fatalf("findPython = %q, want %q", got.Path, venv)
	}
}

func TestValidatePythonRuntime_BrokenProjectVenvShowsRecoveryGuidance(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", "")
	if runtime.GOOS == "windows" {
		mustWriteValidateFile(t, root, filepath.Join(".venv", "Scripts", "python.exe"), "not-a-real-exe")
	} else {
		mustWriteValidateFile(t, root, filepath.Join(".venv", "bin", "python3"), "#!/usr/bin/env bash\nexit 0\n")
	}

	err := validatePythonRuntime(root)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "recreate .venv") {
		t.Fatalf("error = %q", err)
	}
}

func TestValidate_ManifestProject_ShellRequiresBashOnWindows(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific")
	}
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "README.md", "# x\n")
	mustWriteValidateFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "x"
version: "0.1.0"
description: "x"
runtime: "shell"
entrypoint: "./bin/x"
targets: ["codex"]
`)
	mustWriteValidateFile(t, dir, filepath.Join("AGENTS.md"), "repo instructions\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "codex", "package.yaml"), "model_hint: gpt-5.4-mini\n")
	mustWriteValidateFile(t, dir, filepath.Join(".codex", "config.toml"), "notify = [\"./bin/x\", \"notify\"]\n")
	mustWriteValidateFile(t, dir, filepath.Join(".codex-plugin", "plugin.json"), "{}\n")
	mustWriteValidateFile(t, dir, filepath.Join("bin", "x.cmd"), "@echo off\r\n")
	mustWriteValidateFile(t, dir, filepath.Join("scripts", "main.sh"), "#!/usr/bin/env bash\nexit 0\n")

	report, err := Validate(dir, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if _, bashErr := exec.LookPath("bash"); bashErr != nil {
		var found bool
		for _, failure := range report.Failures {
			if failure.Kind == FailureRuntimeNotFound && strings.Contains(failure.Message, "bash") {
				found = true
			}
		}
		if !found {
			t.Fatalf("failures = %+v", report.Failures)
		}
	}
}

func TestValidate_ManifestProject_WindowsCmdLauncherAccepted(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific")
	}
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "README.md", "# x\n")
	mustWriteValidateFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "x"
version: "0.1.0"
description: "x"
runtime: "python"
entrypoint: "./bin/x"
targets: ["codex"]
`)
	mustWriteValidateFile(t, dir, filepath.Join("AGENTS.md"), "repo instructions\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "codex", "package.yaml"), "model_hint: gpt-5.4-mini\n")
	mustWriteValidateFile(t, dir, filepath.Join(".codex", "config.toml"), "notify = [\"./bin/x\", \"notify\"]\n")
	mustWriteValidateFile(t, dir, filepath.Join(".codex-plugin", "plugin.json"), "{}\n")
	mustWriteValidateFile(t, dir, filepath.Join("bin", "x.cmd"), "@echo off\r\n")
	mustWriteValidateFile(t, dir, filepath.Join("src", "main.py"), "print('ok')\n")

	report, err := Validate(dir, "codex")
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range report.Failures {
		if failure.Kind == FailureLauncherInvalid {
			t.Fatalf("unexpected launcher failure: %+v", report.Failures)
		}
	}
}

func TestShellLauncherPassthrough(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	dir := filepath.Join(parent, "project with spaces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteValidateFile(t, dir, filepath.Join("bin", "x"), "#!/usr/bin/env bash\nset -euo pipefail\nROOT=\"$(CDPATH= cd -- \"$(dirname -- \"$0\")/..\" && pwd)\"\nexec \"$ROOT/scripts/main.sh\" \"$@\"\n")
	mustWriteValidateFile(t, dir, filepath.Join("scripts", "main.sh"), "#!/usr/bin/env bash\nset -euo pipefail\nhook_name=\"${1:-}\"\nif [[ \"$hook_name\" == \"notify\" ]]; then\n  printf '%s' \"$2\" >&2\n  exit 7\nfi\ncat >/dev/null\nprintf '{}'\n")
	mustChmodExecutable(t, filepath.Join(dir, "bin", "x"))
	mustChmodExecutable(t, filepath.Join(dir, "scripts", "main.sh"))

	cmd := exec.Command(filepath.Join(dir, "bin", "x"), "Stop")
	cmd.Stdin = strings.NewReader("{\"ok\":true}\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("launcher stop: %v\n%s", err, out)
	}
	if string(out) != "{}" {
		t.Fatalf("stdout = %q, want {}", string(out))
	}

	cmd = exec.Command(filepath.Join(dir, "bin", "x"), "notify", `{"client":"codex-tui"}`)
	out, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("want ExitError, got %v", err)
	}
	if exitErr.ExitCode() != 7 {
		t.Fatalf("exit code = %d, want 7", exitErr.ExitCode())
	}
	if strings.TrimSpace(string(out)) != `{"client":"codex-tui"}` {
		t.Fatalf("stderr passthrough = %q", string(out))
	}
}

func mustWriteValidateFile(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustChmodExecutable(t *testing.T, full string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Chmod(full, 0o755); err != nil {
		t.Fatal(err)
	}
}

func saveTestManifest(t *testing.T, root, platform, runtimeName string) {
	t.Helper()
	manifest := pluginmanifest.Default("x", platform, runtimeName, "x", false)
	if err := pluginmanifest.Save(root, manifest, false); err != nil {
		t.Fatal(err)
	}
}
