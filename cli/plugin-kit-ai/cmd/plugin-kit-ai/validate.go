package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/validate"
	"github.com/spf13/cobra"
)

var validatePlatform string
var validateStrict bool

type validateRunner interface {
	Validate(root, platform string) (validate.Report, error)
}

type validateRunnerFunc func(root, platform string) (validate.Report, error)

func (fn validateRunnerFunc) Validate(root, platform string) (validate.Report, error) {
	return fn(root, platform)
}

var validateCmd = newValidateCmd(validateRunnerFunc(validate.Validate))

func newValidateCmd(runner validateRunner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [path]",
		Short: "Validate a package-standard plugin-kit-ai project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := runner.Validate(args[0], validatePlatform)
			if err != nil {
				return err
			}
			for _, warning := range report.Warnings {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning.Message)
			}
			for _, hint := range geminiValidateWarningHints(report) {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Hint: %s\n", hint)
			}
			if len(report.Failures) > 0 {
				for _, failure := range report.Failures {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Failure: %s\n", failure.Message)
				}
				for _, hint := range geminiValidateFailureHints(report) {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Hint: %s\n", hint)
				}
				return &validate.ReportError{Report: report}
			}
			if validateStrict && len(report.Warnings) > 0 {
				return fmt.Errorf("validation warnings treated as errors (%d warning(s))", len(report.Warnings))
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Validated %s\n", args[0])
			for _, hint := range geminiValidateSuccessHints(args[0], report) {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Hint: %s\n", hint)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&validatePlatform, "platform", "", `target override ("codex-package", "codex-runtime", "claude", "gemini", or "opencode")`)
	cmd.Flags().BoolVar(&validateStrict, "strict", false, "treat validation warnings as errors")
	return cmd
}

func geminiValidateWarningHints(report validate.Report) []string {
	if !reportTouchesGemini(report) {
		return nil
	}
	seen := map[string]struct{}{}
	var hints []string
	for _, warning := range report.Warnings {
		switch warning.Kind {
		case validate.WarningGeminiDirNameMismatch:
			appendHint(&hints, seen, "rename the extension directory to match plugin.yaml name before running gemini extensions link .")
		case validate.WarningGeminiPolicyIgnored:
			appendHint(&hints, seen, "Gemini extension-tier policies ignore allow/yolo; keep only documented extension policy keys in targets/gemini/policies/*.toml.")
		}
	}
	return hints
}

func geminiValidateFailureHints(report validate.Report) []string {
	if !reportTouchesGemini(report) {
		return nil
	}
	seen := map[string]struct{}{}
	var hints []string
	for _, failure := range report.Failures {
		switch failure.Kind {
		case validate.FailureEntrypointMismatch, validate.FailureGeneratedContractInvalid:
			if strings.Contains(failure.Path, "hooks/hooks.json") || strings.Contains(failure.Message, "hooks/hooks.json") || strings.Contains(strings.ToLower(failure.Message), "generated artifact drift") {
				appendHint(&hints, seen, "rerun plugin-kit-ai render . to regenerate Gemini hooks/hooks.json from launcher.yaml, then rerun plugin-kit-ai validate . --platform gemini --strict.")
			}
		case validate.FailureLauncherInvalid, validate.FailureRuntimeTargetMissing:
			appendHint(&hints, seen, "for the Gemini Go beta lane keep launcher.yaml entrypoint, the built binary under bin/, and rendered hooks/hooks.json aligned before rerunning validate.")
		}
	}
	if len(hints) > 0 {
		appendHint(&hints, seen, "after validate is green, run make test-gemini-runtime-smoke, relink the extension with gemini extensions link ., then use make test-gemini-runtime-live when you need real CLI evidence.")
	}
	return hints
}

func geminiValidateSuccessHints(root string, report validate.Report) []string {
	if !strings.EqualFold(strings.TrimSpace(report.Platform), "gemini") {
		return nil
	}
	launcherPath := filepath.Join(root, "launcher.yaml")
	if _, err := os.Stat(launcherPath); err != nil {
		return nil
	}
	return []string{
		"Gemini Go beta lane is validate-clean; run make test-gemini-runtime-smoke before relinking the extension.",
		"relink the extension with gemini extensions link . before checking the runtime path in a real Gemini CLI session.",
		"use make test-gemini-runtime-live when you need real CLI evidence after the repo-local smoke is green.",
	}
}

func reportTouchesGemini(report validate.Report) bool {
	if strings.Contains(strings.ToLower(report.Platform), "gemini") {
		return true
	}
	for _, failure := range report.Failures {
		if strings.EqualFold(strings.TrimSpace(failure.Target), "gemini") || strings.Contains(strings.ToLower(failure.Message), "gemini") {
			return true
		}
	}
	for _, warning := range report.Warnings {
		if strings.Contains(strings.ToLower(warning.Message), "gemini") {
			return true
		}
	}
	return false
}

func appendHint(dst *[]string, seen map[string]struct{}, hint string) {
	if _, ok := seen[hint]; ok {
		return
	}
	seen[hint] = struct{}{}
	*dst = append(*dst, hint)
}
