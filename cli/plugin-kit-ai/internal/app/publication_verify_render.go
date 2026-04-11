package app

import "path/filepath"

func buildPublicationVerifyRootResult(ctx publicationContext, plan publicationVerifyPlan) PluginPublicationVerifyRootResult {
	status := buildPublicationVerifyRootStatus(ctx, plan)
	lines := buildPublicationVerifyRootLines(ctx, plan, status)
	return PluginPublicationVerifyRootResult{
		Ready:       status.ready,
		Status:      status.label,
		Dest:        filepath.Clean(ctx.dest),
		PackageRoot: ctx.packageRoot,
		CatalogPath: plan.catalogRel,
		IssueCount:  len(plan.issues),
		Issues:      plan.issues,
		NextSteps:   status.nextSteps,
		Lines:       lines,
	}
}
