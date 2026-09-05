package agentpluginscli

import (
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/spf13/cobra"
)

func authorizeSecurityAssessment(cmd *cobra.Command, app App, opts *options, loaded *loadedPackage) error {
	if loaded.security == nil || loaded.securityAuthorized {
		return nil
	}
	assessment := *loaded.security
	if opts.format == "human" {
		renderSecurityAssessment(cmd, assessment)
	}
	if assessment.Counts.Blocking == 0 || opts.dryRun {
		loaded.securityAuthorized = true
		return nil
	}
	if opts.acceptSecurityRisk {
		if opts.format == "human" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Continuing because --accept-security-risk was explicitly provided.")
		}
		loaded.securityAuthorized = true
		return nil
	}
	if opts.format != "human" || !app.Terminal {
		if opts.format == "json" {
			if err := writeJSONResult(cmd.OutOrStdout(), cmd.Name(), outputResultFailure, map[string]any{
				"status": "blocked_by_security_review", "security": assessment,
				"next_action": "Review the findings and rerun with --accept-security-risk only if you accept the risk.",
			}); err != nil {
				return err
			}
		}
		return fmt.Errorf("installation stopped: automated security checks found %d blocking finding(s); review them and rerun with --accept-security-risk to continue", assessment.Counts.Blocking)
	}
	confirmed, err := promptYesNo(cmd.InOrStdin(), cmd.OutOrStdout(), "Blocking security findings were detected. Continue anyway? [y/N]")
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("installation cancelled after automated security review; no files were changed")
	}
	loaded.securityAuthorized = true
	return nil
}

func renderSecurityAssessment(cmd *cobra.Command, assessment domain.SecurityAssessment) {
	source := "local scan"
	switch assessment.Evidence {
	case domain.SecurityEvidenceSignedIndex:
		source = "signed index"
	case domain.SecurityEvidenceCache:
		source = "verified cache"
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Automated security checks: %s (%s %s, exact package revision)\n", securityOutcomeLabel(assessment), assessment.Scanner.ID, assessment.Scanner.Version)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Evidence: %s. Automated checks are not a guarantee of safety.\n", source)
	for _, finding := range assessment.Findings {
		location := strings.TrimSpace(finding.Path)
		if finding.Line > 0 {
			location = fmt.Sprintf("%s:%d", location, finding.Line)
		}
		if location != "" {
			location += " - "
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s %s%s\n", strings.ToUpper(finding.Disposition), finding.Code, location, finding.Message)
	}
}

func securityOutcomeLabel(assessment domain.SecurityAssessment) string {
	if assessment.Counts.Blocking > 0 {
		return fmt.Sprintf("%d blocking, %d warning", assessment.Counts.Blocking, assessment.Counts.Warnings)
	}
	if assessment.Counts.Warnings > 0 {
		return fmt.Sprintf("%d warning(s), no blocking findings", assessment.Counts.Warnings)
	}
	return "no blocking findings"
}
