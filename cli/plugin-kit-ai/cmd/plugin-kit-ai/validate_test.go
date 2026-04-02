package main

import (
	"bytes"
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
		"Hint: after validate is green, relink the extension with gemini extensions link .",
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
