package validate

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidate_CannotInferPlatform(t *testing.T) {
	t.Parallel()
	_, err := Validate(t.TempDir(), "")
	var re *ReportError
	if !errors.As(err, &re) {
		t.Fatalf("expected ReportError, got %v", err)
	}
	if got := re.Report.Failures[0].Kind; got != FailureCannotInferPlatform {
		t.Fatalf("failure kind = %q", got)
	}
	if re.Error() != "could not infer platform" {
		t.Fatalf("error = %q", re.Error())
	}
}

func TestValidate_UnknownPlatform(t *testing.T) {
	t.Parallel()
	_, err := Validate(t.TempDir(), "nope")
	var re *ReportError
	if !errors.As(err, &re) {
		t.Fatalf("expected ReportError, got %v", err)
	}
	if got := re.Report.Failures[0].Kind; got != FailureUnknownPlatform {
		t.Fatalf("failure kind = %q", got)
	}
	if re.Error() != "unknown platform \"nope\"" {
		t.Fatalf("error = %q", re.Error())
	}
}

func TestValidate_RequiredFileMissing(t *testing.T) {
	t.Parallel()
	report, err := Validate(t.TempDir(), "codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) == 0 || report.Failures[0].Kind != FailureRequiredFileMissing {
		t.Fatalf("failures = %+v", report.Failures)
	}
}

func TestValidate_ForbiddenFilePresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.22\n")
	mustWriteValidateFile(t, dir, "README.md", "# x\n")
	mustWriteValidateFile(t, dir, "AGENTS.md", "repo instructions\n")
	mustWriteValidateFile(t, dir, filepath.Join(".codex", "config.toml"), "notify = [\"./bin/x\", \"notify\"]\n")
	mustWriteValidateFile(t, dir, filepath.Join(".claude-plugin", "plugin.json"), "{}\n")

	report, err := Validate(dir, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) == 0 || report.Failures[0].Kind != FailureForbiddenFilePresent {
		t.Fatalf("failures = %+v", report.Failures)
	}
	if !strings.Contains(report.Failures[0].Message, ".claude-plugin/plugin.json") {
		t.Fatalf("message = %q", report.Failures[0].Message)
	}
}

func TestValidate_BuildFailed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.22\n")
	mustWriteValidateFile(t, dir, "README.md", "# x\n")
	mustWriteValidateFile(t, dir, "AGENTS.md", "repo instructions\n")
	mustWriteValidateFile(t, dir, filepath.Join(".codex", "config.toml"), "notify = [\"./bin/x\", \"notify\"]\n")
	mustWriteValidateFile(t, dir, "broken.go", "package main\nfunc main() {\n")

	report, err := Validate(dir, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) == 0 || report.Failures[0].Kind != FailureBuildFailed {
		t.Fatalf("failures = %+v", report.Failures)
	}
	if !strings.Contains((&ReportError{Report: report}).Error(), "go build ./...:") {
		t.Fatalf("report error = %q", (&ReportError{Report: report}).Error())
	}
}

func TestValidate_ManifestProject_CodexShell(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, filepath.Join(".plugin-kit-ai", "project.toml"), "schema_version = 1\nplatform = \"codex\"\nruntime = \"shell\"\nexecution_mode = \"launcher\"\nentrypoint = \"./bin/x\"\n")
	mustWriteValidateFile(t, dir, "README.md", "# x\n")
	mustWriteValidateFile(t, dir, "AGENTS.md", "repo instructions\n")
	mustWriteValidateFile(t, dir, filepath.Join(".codex", "config.toml"), "notify = [\"./bin/x\", \"notify\"]\n")
	mustWriteValidateFile(t, dir, filepath.Join("bin", "x"), "#!/usr/bin/env bash\nexec \"$(CDPATH= cd -- \"$(dirname -- \"$0\")/..\" && pwd)/scripts/main.sh\" \"$@\"\n")
	mustWriteValidateFile(t, dir, filepath.Join("scripts", "main.sh"), "#!/usr/bin/env bash\nexit 0\n")
	mustChmodExecutable(t, filepath.Join(dir, "bin", "x"))
	mustChmodExecutable(t, filepath.Join(dir, "scripts", "main.sh"))

	report, err := Validate(dir, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("failures = %+v", report.Failures)
	}
}

func TestValidate_ManifestProject_RuntimeNotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, filepath.Join(".plugin-kit-ai", "project.toml"), "schema_version = 1\nplatform = \"codex\"\nruntime = \"node\"\nexecution_mode = \"launcher\"\nentrypoint = \"./bin/x\"\n")
	mustWriteValidateFile(t, dir, "README.md", "# x\n")
	mustWriteValidateFile(t, dir, "AGENTS.md", "repo instructions\n")
	mustWriteValidateFile(t, dir, filepath.Join(".codex", "config.toml"), "notify = [\"./bin/x\", \"notify\"]\n")
	mustWriteValidateFile(t, dir, filepath.Join("bin", "x"), "#!/usr/bin/env bash\n")
	mustWriteValidateFile(t, dir, "package.json", "{}\n")
	mustWriteValidateFile(t, dir, filepath.Join("src", "main.mjs"), "process.exit(0)\n")
	mustChmodExecutable(t, filepath.Join(dir, "bin", "x"))

	report, err := Validate(dir, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if _, nodeErr := exec.LookPath("node"); nodeErr != nil {
		if len(report.Failures) == 0 || report.Failures[0].Kind != FailureRuntimeNotFound {
			t.Fatalf("failures = %+v", report.Failures)
		}
		return
	}
	if len(report.Failures) != 0 {
		t.Fatalf("failures = %+v", report.Failures)
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
		if got != venv {
			t.Fatalf("findPython = %q, want %q", got, venv)
		}
		return
	}
	venv := filepath.Join(root, ".venv", "bin", "python3")
	mustWriteValidateFile(t, root, filepath.Join(".venv", "bin", "python3"), "binary")
	got, err := findPython(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != venv {
		t.Fatalf("findPython = %q, want %q", got, venv)
	}
}

func TestValidate_ManifestProject_ShellRequiresBashOnWindows(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific")
	}
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, filepath.Join(".plugin-kit-ai", "project.toml"), "schema_version = 1\nplatform = \"codex\"\nruntime = \"shell\"\nexecution_mode = \"launcher\"\nentrypoint = \"./bin/x\"\n")
	mustWriteValidateFile(t, dir, "README.md", "# x\n")
	mustWriteValidateFile(t, dir, "AGENTS.md", "repo instructions\n")
	mustWriteValidateFile(t, dir, filepath.Join(".codex", "config.toml"), "notify = [\"./bin/x\", \"notify\"]\n")
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
		return
	}
}

func TestValidate_ManifestProject_WindowsCmdLauncherAccepted(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific")
	}
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, filepath.Join(".plugin-kit-ai", "project.toml"), "schema_version = 1\nplatform = \"codex\"\nruntime = \"python\"\nexecution_mode = \"launcher\"\nentrypoint = \"./bin/x\"\n")
	mustWriteValidateFile(t, dir, "README.md", "# x\n")
	mustWriteValidateFile(t, dir, "AGENTS.md", "repo instructions\n")
	mustWriteValidateFile(t, dir, filepath.Join(".codex", "config.toml"), "notify = [\"./bin/x\", \"notify\"]\n")
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

func TestValidate_ManifestProject_EntrypointMismatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, filepath.Join(".plugin-kit-ai", "project.toml"), "schema_version = 1\nplatform = \"claude\"\nruntime = \"shell\"\nexecution_mode = \"launcher\"\nentrypoint = \"./bin/x\"\n")
	mustWriteValidateFile(t, dir, "README.md", "# x\n")
	mustWriteValidateFile(t, dir, filepath.Join(".claude-plugin", "plugin.json"), "{}\n")
	mustWriteValidateFile(t, dir, filepath.Join("hooks", "hooks.json"), "{ \"hooks\": { \"Stop\": [{ \"hooks\": [{ \"type\": \"command\", \"command\": \"./bin/y Stop\" }] }] } }\n")
	mustWriteValidateFile(t, dir, filepath.Join("bin", "x"), "#!/usr/bin/env bash\n")
	mustWriteValidateFile(t, dir, filepath.Join("scripts", "main.sh"), "#!/usr/bin/env bash\nexit 0\n")
	mustChmodExecutable(t, filepath.Join(dir, "bin", "x"))
	mustChmodExecutable(t, filepath.Join(dir, "scripts", "main.sh"))

	report, err := Validate(dir, "claude")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, failure := range report.Failures {
		if failure.Kind == FailureEntrypointMismatch {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("failures = %+v", report.Failures)
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
