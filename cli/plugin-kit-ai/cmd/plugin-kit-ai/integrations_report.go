package main

import (
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl"
	"github.com/spf13/cobra"
)

func validateUpdateArgs(all bool, args []string) error {
	if all {
		if len(args) != 0 {
			return fmt.Errorf("update --all does not accept a name")
		}
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("update requires exactly one integration name unless --all is set")
	}
	return nil
}

func printIntegrationReport(cmd *cobra.Command, report integrationctl.Report) {
	if strings.TrimSpace(report.OperationID) != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Operation: %s\n", report.OperationID)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), report.Summary)
	for _, warning := range report.Warnings {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning)
	}
	for _, target := range report.Targets {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s: action=%s delivery=%s state=%s", target.TargetID, target.ActionClass, target.DeliveryKind, target.State)
		if target.ActivationState != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), " activation=%s", target.ActivationState)
		}
		if target.EvidenceKey != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), " evidence=%s", target.EvidenceKey)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
		for _, step := range target.ManualSteps {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  next - %s\n", step)
		}
		for _, restriction := range target.EnvironmentRestrictions {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  restriction - %s\n", restriction)
		}
	}
}

func boolPtr(v bool) *bool { return &v }

func firstNormalizedTarget(values []string) string {
	normalized := integrationctl.NormalizeTargets(values)
	if len(normalized) == 0 {
		return ""
	}
	return normalized[0]
}
