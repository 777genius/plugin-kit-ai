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

func TestValidate_GeminiRejectsInvalidExtensionName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "Demo_Extension"
version: "0.1.0"
description: "demo"
targets: ["gemini"]
`)
	mustWriteValidateFile(t, dir, filepath.Join("targets", "gemini", "package.yaml"), "context_file_name: GEMINI.md\n")
	mustWriteValidateFile(t, dir, filepath.Join("contexts", "GEMINI.md"), "# Gemini\n")
	mustWriteValidateFile(t, dir, "gemini-extension.json", "{}\n")

	_, err := Validate(dir, "gemini")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid Gemini extension name") {
		t.Fatalf("error = %q", err)
	}
}

func TestValidate_GeminiRejectsTrustAndAmbiguousContexts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "gemini-demo"
version: "0.1.0"
description: "demo"
targets: ["gemini"]
`)
	mustWriteValidateFile(t, dir, filepath.Join("mcp", "servers.json"), `{"demo":{"command":"node server.mjs","trust":true}}`)
	mustWriteValidateFile(t, dir, filepath.Join("contexts", "FIRST.md"), "# First\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "gemini", "contexts", "SECOND.md"), "# Second\n")

	report, err := Validate(dir, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	var foundTrust, foundContext bool
	for _, failure := range report.Failures {
		if strings.Contains(failure.Message, "may not set trust") {
			foundTrust = true
		}
		if strings.Contains(failure.Message, "primary context selection is ambiguous") {
			foundContext = true
		}
	}
	if !foundTrust || !foundContext {
		t.Fatalf("failures = %+v", report.Failures)
	}
	var warnedCommand bool
	for _, warning := range report.Warnings {
		if warning.Kind == WarningGeminiMCPCommandStyle {
			warnedCommand = true
		}
	}
	if !warnedCommand {
		t.Fatalf("warnings = %+v", report.Warnings)
	}
}

func TestValidate_GeminiWarnsOnIgnoredPolicyKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "gemini-policy"
version: "0.1.0"
description: "demo"
targets: ["gemini"]
`)
	mustWriteValidateFile(t, dir, filepath.Join("contexts", "GEMINI.md"), "# Gemini\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "gemini", "policies", "review.toml"), "allow = true\nyolo = true\n")

	report, err := Validate(dir, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	var allowWarn, yoloWarn bool
	for _, warning := range report.Warnings {
		if strings.Contains(warning.Message, `"allow"`) {
			allowWarn = true
		}
		if strings.Contains(warning.Message, `"yolo"`) {
			yoloWarn = true
		}
	}
	if !allowWarn || !yoloWarn {
		t.Fatalf("warnings = %+v", report.Warnings)
	}
}

func TestValidate_GeminiRejectsInvalidCommandTOML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "gemini-command"
version: "0.1.0"
description: "demo"
targets: ["gemini"]
`)
	mustWriteValidateFile(t, dir, filepath.Join("contexts", "GEMINI.md"), "# Gemini\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "gemini", "commands", "bad.toml"), "description = [\n")

	report, err := Validate(dir, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, failure := range report.Failures {
		if strings.Contains(failure.Message, "invalid TOML") {
			found = true
		}
	}
	if !found {
		t.Fatalf("failures = %+v", report.Failures)
	}
}

func TestValidate_GeminiRejectsUnsupportedHooksLayout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "gemini-hooks"
version: "0.1.0"
description: "demo"
targets: ["gemini"]
`)
	mustWriteValidateFile(t, dir, filepath.Join("contexts", "GEMINI.md"), "# Gemini\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "gemini", "hooks", "extra.json"), "{}\n")

	report, err := Validate(dir, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, failure := range report.Failures {
		if strings.Contains(failure.Message, "unsupported Gemini hooks layout") {
			found = true
		}
	}
	if !found {
		t.Fatalf("failures = %+v", report.Failures)
	}
}

func TestValidate_GeminiRejectsHooksWithoutTopLevelHooksObject(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "gemini-hooks"
version: "0.1.0"
description: "demo"
targets: ["gemini"]
`)
	mustWriteValidateFile(t, dir, filepath.Join("contexts", "GEMINI.md"), "# Gemini\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "gemini", "hooks", "hooks.json"), "{}\n")

	report, err := Validate(dir, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, failure := range report.Failures {
		if strings.Contains(failure.Message, "must define a top-level hooks object") {
			found = true
		}
	}
	if !found {
		t.Fatalf("failures = %+v", report.Failures)
	}
}

func TestValidate_GeminiRejectsNonYAMLSettingsAndThemes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "gemini-assets"
version: "0.1.0"
description: "demo"
targets: ["gemini"]
`)
	mustWriteValidateFile(t, dir, filepath.Join("contexts", "GEMINI.md"), "# Gemini\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "gemini", "settings", "api-token.json"), "{}\n")

	report, err := Validate(dir, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, failure := range report.Failures {
		if strings.Contains(failure.Message, "unsupported Gemini setting file") {
			found = true
		}
	}
	if !found {
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

func TestRequireMinVersion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		runtime   string
		output    string
		wantMajor int
		wantMinor int
		wantErr   string
	}{
		{
			name:      "python-supported",
			runtime:   "python",
			output:    "Python 3.11.8\n",
			wantMajor: 3,
			wantMinor: 10,
		},
		{
			name:      "python-too-old",
			runtime:   "python",
			output:    "Python 3.9.18\n",
			wantMajor: 3,
			wantMinor: 10,
			wantErr:   "below the supported minimum 3.10",
		},
		{
			name:      "node-supported",
			runtime:   "node",
			output:    "v20.11.1\n",
			wantMajor: 20,
			wantMinor: 0,
		},
		{
			name:      "node-too-old",
			runtime:   "node",
			output:    "v18.19.0\n",
			wantMajor: 20,
			wantMinor: 0,
			wantErr:   "below the supported minimum 20.0",
		},
		{
			name:      "unparseable",
			runtime:   "node",
			output:    "hello",
			wantMajor: 20,
			wantMinor: 0,
			wantErr:   "unsupported version output",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := requireMinVersion(tc.runtime, tc.output, tc.wantMajor, tc.wantMinor)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("requireMinVersion() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("requireMinVersion() error = %v, want substring %q", err, tc.wantErr)
			}
		})
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
targets: ["codex"]
`)
	mustWriteValidateFile(t, dir, pluginmanifest.LauncherFileName, "runtime: shell\nentrypoint: ./bin/x\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "codex-runtime", "package.yaml"), "model_hint: gpt-5.4-mini\n")
	mustWriteValidateFile(t, dir, filepath.Join(".codex", "config.toml"), "notify = [\"./bin/x\", \"notify\"]\n")
	mustWriteValidateFile(t, dir, filepath.Join("bin", "x.cmd"), "@echo off\r\n")
	mustWriteValidateFile(t, dir, filepath.Join("scripts", "main.sh"), "#!/usr/bin/env bash\nexit 0\n")

	report, err := Validate(dir, "codex-runtime")
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

func TestValidate_CodexRejectsManifestExtraCanonicalOverride(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "README.md", "# x\n")
	mustWriteValidateFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "x"
version: "0.1.0"
description: "x"
targets: ["codex-package"]
`)
	mustWriteValidateFile(t, dir, filepath.Join("targets", "codex-package", "manifest.extra.json"), `{"name":"override"}`)
	mustWriteValidateFile(t, dir, filepath.Join(".codex-plugin", "plugin.json"), "{}\n")

	report, err := Validate(dir, "codex-package")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, failure := range report.Failures {
		if strings.Contains(failure.Message, `codex-package manifest.extra.json may not override canonical field "name"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("failures = %+v", report.Failures)
	}
}

func TestValidate_CodexRejectsConfigExtraCanonicalOverrideAndModelDrift(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "README.md", "# x\n")
	mustWriteValidateFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "x"
version: "0.1.0"
description: "x"
targets: ["codex-runtime"]
`)
	mustWriteValidateFile(t, dir, pluginmanifest.LauncherFileName, "runtime: go\nentrypoint: ./bin/x\n")
	mustWriteValidateFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.22\n")
	mustWriteValidateFile(t, dir, filepath.Join("cmd", "x", "main.go"), "package main\nfunc main() {}\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "codex-runtime", "package.yaml"), "model_hint: gpt-5.4-mini\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "codex-runtime", "config.extra.toml"), "notify = [\"./bin/x\", \"notify\"]\n")
	mustWriteValidateFile(t, dir, filepath.Join(".codex", "config.toml"), "model = \"gpt-4.1\"\nnotify = [\"./bin/other\", \"notify\"]\n")

	report, err := Validate(dir, "codex-runtime")
	if err != nil {
		t.Fatal(err)
	}
	var foundExtra, foundNotify, foundModel bool
	for _, failure := range report.Failures {
		if strings.Contains(failure.Message, `codex-runtime config.extra.toml may not override canonical field "notify"`) {
			foundExtra = true
		}
		if strings.Contains(failure.Message, `entrypoint mismatch: Codex notify argv uses ["./bin/other" "notify"]`) {
			foundNotify = true
		}
		if strings.Contains(failure.Message, `does not match targets/codex-runtime/package.yaml model_hint "gpt-5.4-mini"`) {
			foundModel = true
		}
	}
	if !foundExtra || !foundNotify || !foundModel {
		t.Fatalf("failures = %+v", report.Failures)
	}
}

func TestValidate_GeminiRejectsManifestExtraCanonicalOverride(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "gemini-demo"
version: "0.1.0"
description: "demo"
targets: ["gemini"]
`)
	mustWriteValidateFile(t, dir, filepath.Join("contexts", "GEMINI.md"), "# Gemini\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "gemini", "manifest.extra.json"), `{"plan":{"directory":".gemini/other"}}`)

	report, err := Validate(dir, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, failure := range report.Failures {
		if strings.Contains(failure.Message, `gemini manifest.extra.json may not override canonical field "plan.directory"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("failures = %+v", report.Failures)
	}
}

func TestValidate_GeminiRejectsInvalidMCPTransportShape(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "gemini-transport"
version: "0.1.0"
description: "demo"
targets: ["gemini"]
`)
	mustWriteValidateFile(t, dir, filepath.Join("contexts", "GEMINI.md"), "# Gemini\n")
	mustWriteValidateFile(t, dir, filepath.Join("mcp", "servers.json"), `{
  "stdio-and-sse":{"command":"node","url":"https://example.com/sse"},
  "bad-args":{"url":"https://example.com/http","args":["serve"]},
  "bad-env":{"command":"node","args":["server.mjs"],"env":{"TOKEN":1}},
  "bad-cwd":{"command":"node","args":["server.mjs"],"cwd":true}
}`)

	report, err := Validate(dir, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	var foundTransport, foundArgs, foundEnv, foundCwd bool
	for _, failure := range report.Failures {
		switch {
		case strings.Contains(failure.Message, "must define exactly one transport"):
			foundTransport = true
		case strings.Contains(failure.Message, "may only use args with command-based stdio transport"):
			foundArgs = true
		case strings.Contains(failure.Message, "env must be an object of string values"):
			foundEnv = true
		case strings.Contains(failure.Message, "cwd must be a non-empty string"):
			foundCwd = true
		}
	}
	if !foundTransport || !foundArgs || !foundEnv || !foundCwd {
		t.Fatalf("failures = %+v", report.Failures)
	}
}

func TestValidate_GeminiRejectsMalformedSettingsThemesAndExcludeTools(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "gemini-assets"
version: "0.1.0"
description: "demo"
targets: ["gemini"]
`)
	mustWriteValidateFile(t, dir, filepath.Join("contexts", "GEMINI.md"), "# Gemini\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "gemini", "package.yaml"), "exclude_tools:\n  - \"\"\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "gemini", "settings", "api-token.yaml"), "name: api-token\ndescription: token\nenv_var: API_TOKEN\nsensitive: maybe\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "gemini", "themes", "empty.yaml"), "name: release-dawn\n")

	report, err := Validate(dir, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	var foundExclude, foundSetting, foundTheme bool
	for _, failure := range report.Failures {
		switch {
		case strings.Contains(failure.Message, "exclude_tools entries must be non-empty strings"):
			foundExclude = true
		case strings.Contains(failure.Message, "Gemini setting file"):
			foundSetting = true
		case strings.Contains(failure.Message, "Gemini themes require at least one theme token besides name"):
			foundTheme = true
		}
	}
	if !foundExclude || !foundSetting || !foundTheme {
		t.Fatalf("failures = %+v", report.Failures)
	}
}

func TestValidate_GeminiRejectsInvalidThemeShapeAndDuplicateSettings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "gemini-assets"
version: "0.1.0"
description: "demo"
targets: ["gemini"]
`)
	mustWriteValidateFile(t, dir, filepath.Join("contexts", "GEMINI.md"), "# Gemini\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "gemini", "settings", "first.yaml"), "name: release-profile\ndescription: one\nenv_var: RELEASE_PROFILE\nsensitive: false\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "gemini", "settings", "second.yaml"), "name: duplicate\ndescription: two\nenv_var: RELEASE_PROFILE\nsensitive: false\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "gemini", "themes", "broken.yaml"), "name: release-dawn\nbackground: \"#fff9f2\"\n")

	report, err := Validate(dir, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	var foundTheme, foundDuplicate bool
	for _, failure := range report.Failures {
		switch {
		case strings.Contains(failure.Message, `Gemini theme key "background" must be a YAML object`):
			foundTheme = true
		case strings.Contains(failure.Message, `duplicates env_var "RELEASE_PROFILE"`):
			foundDuplicate = true
		}
	}
	if !foundTheme || !foundDuplicate {
		t.Fatalf("failures = %+v", report.Failures)
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
targets: ["codex-runtime"]
`)
	mustWriteValidateFile(t, dir, pluginmanifest.LauncherFileName, "runtime: python\nentrypoint: ./bin/x\n")
	mustWriteValidateFile(t, dir, filepath.Join("targets", "codex-runtime", "package.yaml"), "model_hint: gpt-5.4-mini\n")
	mustWriteValidateFile(t, dir, filepath.Join(".codex", "config.toml"), "notify = [\"./bin/x\", \"notify\"]\n")
	mustWriteValidateFile(t, dir, filepath.Join("bin", "x.cmd"), "@echo off\r\n")
	mustWriteValidateFile(t, dir, filepath.Join("src", "main.py"), "print('ok')\n")

	report, err := Validate(dir, "codex-runtime")
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range report.Failures {
		if failure.Kind == FailureLauncherInvalid {
			t.Fatalf("unexpected launcher failure: %+v", report.Failures)
		}
	}
}

func TestValidateNodeRuntimeTarget_MissingBuiltOutputShowsRecoveryGuidance(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, filepath.Join("bin", "x"), "#!/usr/bin/env bash\nset -euo pipefail\nROOT=\"$(CDPATH= cd -- \"$(dirname -- \"$0\")/..\" && pwd)\"\nexec node \"$ROOT/dist/main.js\" \"$@\"\n")
	mustChmodExecutable(t, filepath.Join(dir, "bin", "x"))

	var report Report
	validateNodeRuntimeTarget(dir, "./bin/x", &report)
	if len(report.Failures) != 1 {
		t.Fatalf("failures = %+v", report.Failures)
	}
	failure := report.Failures[0]
	if failure.Kind != FailureRuntimeTargetMissing {
		t.Fatalf("failure kind = %q", failure.Kind)
	}
	if failure.Path != "dist/main.js" {
		t.Fatalf("failure path = %q", failure.Path)
	}
	if !strings.Contains(failure.Message, "npm install && npm run build") {
		t.Fatalf("failure message = %q", failure.Message)
	}
}

func TestValidateNodeRuntimeTarget_TypeScriptLaneShowsTypeScriptGuidance(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, filepath.Join("bin", "x"), "#!/usr/bin/env bash\nset -euo pipefail\nROOT=\"$(CDPATH= cd -- \"$(dirname -- \"$0\")/..\" && pwd)\"\nexec node \"$ROOT/dist/main.js\" \"$@\"\n")
	mustChmodExecutable(t, filepath.Join(dir, "bin", "x"))
	mustWriteValidateFile(t, dir, "tsconfig.json", "{}\n")
	mustWriteValidateFile(t, dir, "package.json", `{"scripts":{"build":"tsc -p tsconfig.json"}}`)

	var report Report
	validateNodeRuntimeTarget(dir, "./bin/x", &report)
	if len(report.Failures) != 1 {
		t.Fatalf("failures = %+v", report.Failures)
	}
	failure := report.Failures[0]
	if !strings.Contains(failure.Message, "TypeScript scaffold expects built output") {
		t.Fatalf("failure message = %q", failure.Message)
	}
	if !strings.Contains(failure.Message, "plugin-kit-ai bootstrap .") {
		t.Fatalf("failure message = %q", failure.Message)
	}
}

func TestValidateRuntimeTargetExecutable_NonExecutableScriptFails(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix-only executable-bit check")
	}

	dir := t.TempDir()
	mustWriteValidateFile(t, dir, filepath.Join("scripts", "main.sh"), "#!/usr/bin/env bash\nexit 0\n")

	var report Report
	validateRuntimeTargetExecutable(dir, filepath.Join("scripts", "main.sh"), &report)
	if len(report.Failures) != 1 {
		t.Fatalf("failures = %+v", report.Failures)
	}
	failure := report.Failures[0]
	if failure.Kind != FailureRuntimeTargetMissing {
		t.Fatalf("failure kind = %q", failure.Kind)
	}
	if !strings.Contains(failure.Message, "is not executable") {
		t.Fatalf("failure message = %q", failure.Message)
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
