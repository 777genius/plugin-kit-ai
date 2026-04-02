package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/777genius/plugin-kit-ai/cli/internal/validate"
	"github.com/spf13/cobra"
)

type validateRunner func(root, platform string) (validate.Report, error)

var validateCmd = newValidateCmd(validate.Validate)

type validateJSONReport struct {
	validate.Report
	OK           bool `json:"ok"`
	StrictMode   bool `json:"strict_mode"`
	StrictFailed bool `json:"strict_failed"`
	WarningCount int  `json:"warning_count"`
	FailureCount int  `json:"failure_count"`
}

func newValidateCmd(run validateRunner) *cobra.Command {
	var platform string
	var strict bool
	var format string

	cmd := &cobra.Command{
		Use:   "validate [path]",
		Short: "Validate a package-standard plugin-kit-ai project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := run(args[0], platform)
			var reportErr *validate.ReportError
			if err != nil && errors.As(err, &reportErr) {
				report = reportErr.Report
			}
			report = normalizeValidateReport(report)

			switch format {
			case "json":
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
				body, marshalErr := json.MarshalIndent(buildValidateJSONReport(report, strict, err), "", "  ")
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
				if err != nil {
					return err
				}
				for _, warning := range report.Warnings {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning.Message)
				}
				if len(report.Failures) > 0 {
					for _, failure := range report.Failures {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Failure: %s\n", failure.Message)
					}
					return &validate.ReportError{Report: report}
				}
				if strict && len(report.Warnings) > 0 {
					return fmt.Errorf("validation warnings treated as errors (%d warning(s))", len(report.Warnings))
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Validated %s\n", args[0])
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&platform, "platform", "", `target override ("codex-package", "codex-runtime", "claude", "gemini", or "opencode")`)
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

func buildValidateJSONReport(report validate.Report, strict bool, runErr error) validateJSONReport {
	failureCount := len(report.Failures)
	warningCount := len(report.Warnings)
	strictFailed := strict && failureCount == 0 && warningCount > 0
	ok := runErr == nil && failureCount == 0 && !strictFailed
	return validateJSONReport{
		Report:       report,
		OK:           ok,
		StrictMode:   strict,
		StrictFailed: strictFailed,
		WarningCount: warningCount,
		FailureCount: failureCount,
	}
}
