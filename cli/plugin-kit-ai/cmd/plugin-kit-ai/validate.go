package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/validate"
	"github.com/spf13/cobra"
)

type validateRunner func(root, platform string) (validate.Report, error)

var validateCmd = newValidateCmd(validate.Validate)

type validateJSONReport struct {
	validate.Report
	Format            string                  `json:"format"`
	SchemaVersion     int                     `json:"schema_version"`
	RequestedPlatform string                  `json:"requested_platform,omitempty"`
	Outcome           string                  `json:"outcome"`
	OK                bool                    `json:"ok"`
	StrictMode        bool                    `json:"strict_mode"`
	StrictFailed      bool                    `json:"strict_failed"`
	WarningCount      int                     `json:"warning_count"`
	FailureCount      int                     `json:"failure_count"`
	Publication       *publicationmodel.Model `json:"publication,omitempty"`
}

func newValidateCmd(run validateRunner) *cobra.Command {
	var platform string
	var strict bool
	var format string

	cmd := &cobra.Command{
		Use:   "validate [path]",
		Short: "Validate a package-standard plugin-kit-ai project",
		Long: `Validate a package-standard plugin-kit-ai project.

Text mode is the human-readable default and prints Warning:/Failure: lines.
Use --format json for CI or automation. That mode emits the versioned
"plugin-kit-ai/validate-report" contract with schema_version=1 and an
explicit outcome of "passed", "failed", or "failed_strict_warnings".`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := run(args[0], platform)
			var reportErr *validate.ReportError
			hasReportErr := err != nil && errors.As(err, &reportErr)
			if hasReportErr {
				report = reportErr.Report
			}
			report = normalizeValidateReport(report)
			publication := discoverValidatePublication(args[0], platform)

			switch format {
			case "json":
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
				body, marshalErr := json.MarshalIndent(buildValidateJSONReport(report, platform, strict, err, publication), "", "  ")
				if marshalErr != nil {
					return marshalErr
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", body)
				if err != nil {
					return err
				}
				if len(report.Failures) > 0 {
					return &validate.ReportError{Report: report}
				}
				if strict && len(report.Warnings) > 0 {
					return fmt.Errorf("validation warnings treated as errors (%d warning(s))", len(report.Warnings))
				}
				return nil
			default:
				for _, warning := range report.Warnings {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning.Message)
				}
				for _, hint := range geminiValidateWarningHints(report) {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Hint: %s\n", hint)
				}
				if len(report.Failures) > 0 {
					cmd.SilenceUsage = true
					cmd.SilenceErrors = true
					for _, line := range validatePublicationText(publication) {
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
					}
					for _, failure := range report.Failures {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Failure: %s\n", failure.Message)
					}
					for _, hint := range geminiValidateFailureHints(report) {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Hint: %s\n", hint)
					}
					if hasReportErr {
						return err
					}
					return &validate.ReportError{Report: report}
				}
				if err != nil {
					return err
				}
				if strict && len(report.Warnings) > 0 {
					return fmt.Errorf("validation warnings treated as errors (%d warning(s))", len(report.Warnings))
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Validated %s\n", args[0])
				for _, line := range validatePublicationText(publication) {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
				}
				for _, hint := range geminiValidateSuccessHints(args[0], report) {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Hint: %s\n", hint)
				}
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&platform, "platform", "", `target override ("codex-package", "codex-runtime", "claude", "gemini", "opencode", or "cursor")`)
	cmd.Flags().BoolVar(&strict, "strict", false, "treat validation warnings as errors")
	cmd.Flags().StringVar(&format, "format", "text", `output format ("text" or "json")`)
	return cmd
}

func normalizeValidateReport(report validate.Report) validate.Report {
	if report.Checks == nil {
		report.Checks = []string{}
	}
	if report.Warnings == nil {
		report.Warnings = []validate.Warning{}
	}
	if report.Failures == nil {
		report.Failures = []validate.Failure{}
	}
	return report
}

func buildValidateJSONReport(report validate.Report, requestedPlatform string, strict bool, runErr error, publication *publicationmodel.Model) validateJSONReport {
	failureCount := len(report.Failures)
	warningCount := len(report.Warnings)
	strictFailed := strict && failureCount == 0 && warningCount > 0
	ok := runErr == nil && failureCount == 0 && !strictFailed
	outcome := "passed"
	switch {
	case failureCount > 0:
		outcome = "failed"
	case strictFailed:
		outcome = "failed_strict_warnings"
	}
	return validateJSONReport{
		Report:            report,
		Format:            "plugin-kit-ai/validate-report",
		SchemaVersion:     1,
		RequestedPlatform: requestedPlatform,
		Outcome:           outcome,
		OK:                ok,
		StrictMode:        strict,
		StrictFailed:      strictFailed,
		WarningCount:      warningCount,
		FailureCount:      failureCount,
		Publication:       publication,
	}
}

func discoverValidatePublication(root, platform string) *publicationmodel.Model {
	inspection, _, err := pluginmanifest.Inspect(root, validatePublicationTarget(platform))
	if err != nil {
		return nil
	}
	if len(inspection.Publication.Packages) == 0 && len(inspection.Publication.Channels) == 0 {
		return nil
	}
	publication := inspection.Publication
	return &publication
}

func validatePublicationTarget(platform string) string {
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return "all"
	}
	return platform
}

func validatePublicationText(publication *publicationmodel.Model) []string {
	if publication == nil {
		return nil
	}
	lines := []string{
		fmt.Sprintf("Publication: api_version=%s packages=%d channels=%d", publication.Core.APIVersion, len(publication.Packages), len(publication.Channels)),
	}
	for _, channel := range publication.Channels {
		line := fmt.Sprintf("Publication channel: %s path=%s targets=%s",
			channel.Family,
			channel.Path,
			strings.Join(channel.PackageTargets, ","),
		)
		if details := formatValidatePublicationDetails(channel.Details); details != "" {
			line += " details=" + details
		}
		lines = append(lines, line)
	}
	return lines
}

func formatValidatePublicationDetails(details map[string]string) string {
	if len(details) == 0 {
		return ""
	}
	keys := make([]string, 0, len(details))
	for key, value := range details {
		if strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	if len(keys) == 0 {
		return ""
	}
	items := make([]string, 0, len(keys))
	for _, key := range keys {
		items = append(items, key+"="+details[key])
	}
	return strings.Join(items, ",")
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
			appendHint(&hints, seen, "for the Gemini Go runtime lane keep launcher.yaml entrypoint, the built binary under bin/, and rendered hooks/hooks.json aligned before rerunning validate.")
		}
	}
	if len(hints) > 0 {
		appendHint(&hints, seen, "after validate is green, run make test-gemini-runtime, relink the extension with gemini extensions link ., then use make test-gemini-runtime-live when you need real CLI evidence.")
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
		"Gemini Go runtime is validate-clean; run make test-gemini-runtime before relinking the extension.",
		"relink the extension with gemini extensions link . before checking the runtime path in a real Gemini CLI session.",
		"use make test-gemini-runtime-live when you need real CLI evidence after the repo-local runtime gate is green.",
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
