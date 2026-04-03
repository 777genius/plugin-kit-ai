package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/validate"
)

func TestValidateWritesGeminiRuntimeRecoveryHints(t *testing.T) {
	t.Parallel()
	prevPlatform := validatePlatform
	prevStrict := validateStrict
	validatePlatform = ""
	validateStrict = false
	t.Cleanup(func() {
		validatePlatform = prevPlatform
		validateStrict = prevStrict
	})

	cmd := newValidateCmd(validateRunnerFunc(func(root, platform string) (validate.Report, error) {
		return validate.Report{
			Platform: "gemini",
			Failures: []validate.Failure{
				{
					Kind:    validate.FailureEntrypointMismatch,
					Path:    "hooks/hooks.json",
					Target:  "gemini",
					Message: `Gemini hook "SessionStart" command "${extensionPath}${/}bin${/}old GeminiSessionStart" does not match launcher entrypoint "./bin/demo"`,
				},
			},
		}, nil
	}))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"."})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	output := stderr.String()
	for _, want := range []string{
		`Failure: Gemini hook "SessionStart" command`,
		"Hint: rerun plugin-kit-ai render . to regenerate Gemini hooks/hooks.json from launcher.yaml",
		"Hint: after validate is green, run make test-gemini-runtime, relink the extension with gemini extensions link .",
		"make test-gemini-runtime-live",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stderr missing %q:\n%s", want, output)
		}
	}
}

func TestValidateWritesGeminiWarningHints(t *testing.T) {
	t.Parallel()
	prevPlatform := validatePlatform
	prevStrict := validateStrict
	validatePlatform = ""
	validateStrict = false
	t.Cleanup(func() {
		validatePlatform = prevPlatform
		validateStrict = prevStrict
	})

	cmd := newValidateCmd(validateRunnerFunc(func(root, platform string) (validate.Report, error) {
		return validate.Report{
			Platform: "gemini",
			Warnings: []validate.Warning{
				{
					Kind:    validate.WarningGeminiDirNameMismatch,
					Message: `Gemini extension directory basename "tmp-ext" does not match extension name "demo-ext"`,
				},
				{
					Kind:    validate.WarningGeminiPolicyIgnored,
					Message: `Gemini extension policies ignore "allow" at extension tier`,
				},
			},
		}, nil
	}))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	for _, want := range []string{
		`Warning: Gemini extension directory basename "tmp-ext" does not match extension name "demo-ext"`,
		"Hint: rename the extension directory to match plugin.yaml name before running gemini extensions link .",
		"Hint: Gemini extension-tier policies ignore allow/yolo; keep only documented extension policy keys in targets/gemini/policies/*.toml.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout missing %q:\n%s", want, output)
		}
	}
}

func TestValidateWritesGeminiSuccessHintsForRuntimeLane(t *testing.T) {
	t.Parallel()
	prevPlatform := validatePlatform
	prevStrict := validateStrict
	validatePlatform = ""
	validateStrict = false
	t.Cleanup(func() {
		validatePlatform = prevPlatform
		validateStrict = prevStrict
	})

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "launcher.yaml"), []byte("runtime: go\nentrypoint: ./bin/demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newValidateCmd(validateRunnerFunc(func(gotRoot, platform string) (validate.Report, error) {
		if gotRoot != root {
			t.Fatalf("root = %q, want %q", gotRoot, root)
		}
		return validate.Report{Platform: "gemini"}, nil
	}))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{root})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	for _, want := range []string{
		"Validated " + root,
		"Hint: Gemini Go runtime is validate-clean; run make test-gemini-runtime before relinking the extension.",
		"Hint: relink the extension with gemini extensions link . before checking the runtime path in a real Gemini CLI session.",
		"Hint: use make test-gemini-runtime-live when you need real CLI evidence after the repo-local runtime gate is green.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout missing %q:\n%s", want, output)
		}
	}
}

func TestValidateHelpIncludesCursorTarget(t *testing.T) {
	t.Parallel()
	cmd := newValidateCmd(validateRunnerFunc(func(root, platform string) (validate.Report, error) {
		return validate.Report{}, nil
	}))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := stdout.String() + stderr.String()
	if !strings.Contains(output, `"cursor"`) {
		t.Fatalf("help output missing cursor target:\n%s", output)
	}
}
