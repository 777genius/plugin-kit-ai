package app

import (
	"fmt"
	"path/filepath"
)

func buildPublicationMaterializeResult(ctx publicationContext, plan publicationMaterializePlan, dryRun bool) PluginPublicationMaterializeResult {
	nextSteps := []string{
		fmt.Sprintf("plugin-kit-ai publication doctor %s", ctx.root),
		fmt.Sprintf("plugin-kit-ai publication doctor %s --target %s --dest %s", ctx.root, ctx.target, ctx.dest),
		fmt.Sprintf("inspect %s with the vendor CLI from the marketplace root", ctx.channel.Family),
	}
	lines := []string{
		fmt.Sprintf("Materialized publication target: %s", ctx.target),
		fmt.Sprintf("Marketplace family: %s", ctx.channel.Family),
		fmt.Sprintf("Marketplace root: %s", filepath.Clean(ctx.dest)),
		fmt.Sprintf("Package root: %s", ctx.packageRoot),
		fmt.Sprintf("Mode: %s", publicationModeLabel(dryRun)),
		fmt.Sprintf("Package root action: %s", plan.packageRootAction),
		fmt.Sprintf("Package files: %d", len(plan.packageFiles)),
		fmt.Sprintf("Catalog artifact action: %s %s", plan.catalogArtifactAct, plan.catalogArtifact.RelPath),
	}
	if len(plan.generated.StalePaths) > 0 {
		lines = append(lines, fmt.Sprintf("Source generate drift observed: %d stale managed path(s) were bypassed by materializing fresh generated outputs", len(plan.generated.StalePaths)))
	}
	lines = append(lines, "Next:")
	for _, step := range nextSteps {
		lines = append(lines, "  "+step)
	}
	return PluginPublicationMaterializeResult{
		Target:            ctx.target,
		Mode:              publicationModeLabel(dryRun),
		MarketplaceFamily: ctx.channel.Family,
		Dest:              filepath.Clean(ctx.dest),
		PackageRoot:       ctx.packageRoot,
		Details: map[string]string{
			"package_root_action":     plan.packageRootAction,
			"package_file_count":      fmt.Sprintf("%d", len(plan.packageFiles)),
			"catalog_artifact":        plan.catalogArtifact.RelPath,
			"catalog_artifact_action": plan.catalogArtifactAct,
		},
		NextSteps: nextSteps,
		Lines:     lines,
	}
}
