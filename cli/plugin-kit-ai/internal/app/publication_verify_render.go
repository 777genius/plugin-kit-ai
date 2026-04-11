package app

import (
	"fmt"
	"path/filepath"
)

func buildPublicationVerifyRootResult(ctx publicationContext, plan publicationVerifyPlan) PluginPublicationVerifyRootResult {
	lines := []string{
		fmt.Sprintf("Local marketplace root: %s", filepath.Clean(ctx.dest)),
		fmt.Sprintf("Package root: %s", ctx.packageRoot),
		fmt.Sprintf("Catalog artifact: %s", plan.catalogRel),
	}
	nextSteps := []string{}
	status := "ready"
	ready := true
	if len(plan.issues) > 0 {
		status = "needs_sync"
		ready = false
		for _, issue := range plan.issues {
			lines = append(lines, fmt.Sprintf("Issue[%s]: %s", issue.Code, issue.Message))
		}
		nextSteps = []string{
			fmt.Sprintf("run plugin-kit-ai publication materialize %s --target %s --dest %s", ctx.root, ctx.target, ctx.dest),
		}
		lines = append(lines, "Status: needs_sync (materialized marketplace root is missing files or has drift)", "Next:")
		for _, step := range nextSteps {
			lines = append(lines, "  "+step)
		}
	} else {
		lines = append(lines, "Status: ready (materialized marketplace root is in sync)")
	}
	return PluginPublicationVerifyRootResult{
		Ready:       ready,
		Status:      status,
		Dest:        filepath.Clean(ctx.dest),
		PackageRoot: ctx.packageRoot,
		CatalogPath: plan.catalogRel,
		IssueCount:  len(plan.issues),
		Issues:      plan.issues,
		NextSteps:   nextSteps,
		Lines:       lines,
	}
}
