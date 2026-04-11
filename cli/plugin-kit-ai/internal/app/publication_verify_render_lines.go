package app

import (
	"fmt"
	"path/filepath"
)

func buildPublicationVerifyRootLines(ctx publicationContext, plan publicationVerifyPlan, status publicationVerifyStatus) []string {
	lines := basePublicationVerifyRootLines(ctx, plan)
	if status.ready {
		return append(lines, "Status: ready (materialized marketplace root is in sync)")
	}
	lines = appendPublicationVerifyIssueLines(lines, plan.issues)
	lines = append(lines, "Status: needs_sync (materialized marketplace root is missing files or has drift)", "Next:")
	for _, step := range status.nextSteps {
		lines = append(lines, "  "+step)
	}
	return lines
}

func basePublicationVerifyRootLines(ctx publicationContext, plan publicationVerifyPlan) []string {
	return []string{
		fmt.Sprintf("Local marketplace root: %s", filepath.Clean(ctx.dest)),
		fmt.Sprintf("Package root: %s", ctx.packageRoot),
		fmt.Sprintf("Catalog artifact: %s", plan.catalogRel),
	}
}

func appendPublicationVerifyIssueLines(lines []string, issues []PluginPublicationRootIssue) []string {
	for _, issue := range issues {
		lines = append(lines, fmt.Sprintf("Issue[%s]: %s", issue.Code, issue.Message))
	}
	return lines
}
