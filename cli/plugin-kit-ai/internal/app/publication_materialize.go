package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
)

func (PluginService) publicationMaterialize(opts PluginPublicationMaterializeOptions) (PluginPublicationMaterializeResult, error) {
	ctx, err := loadPublicationContextForMaterialize(opts)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	packageFiles, generated, err := ctx.expectedMaterializedPackageArtifacts()
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	catalogArtifact, err := ctx.renderLocalCatalogArtifact()
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	mergedCatalog, err := mergeCatalogAtDestination(ctx.dest, ctx.target, catalogArtifact)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}

	packageRootAction := "create"
	if info, statErr := os.Stat(ctx.destPackageRoot()); statErr == nil && info.IsDir() {
		packageRootAction = "replace"
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return PluginPublicationMaterializeResult{}, statErr
	}
	catalogAction := "create"
	catalogFull := filepath.Join(ctx.dest, filepath.FromSlash(catalogArtifact.RelPath))
	if _, statErr := os.Stat(catalogFull); statErr == nil {
		catalogAction = "merge"
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return PluginPublicationMaterializeResult{}, statErr
	}
	if !opts.DryRun {
		if err := os.RemoveAll(ctx.destPackageRoot()); err != nil {
			return PluginPublicationMaterializeResult{}, err
		}
		if err := pluginmanifest.WriteArtifacts(ctx.dest, packageFiles); err != nil {
			return PluginPublicationMaterializeResult{}, err
		}
		if err := pluginmanifest.WriteArtifacts(ctx.dest, []pluginmanifest.Artifact{{
			RelPath: catalogArtifact.RelPath,
			Content: mergedCatalog,
		}}); err != nil {
			return PluginPublicationMaterializeResult{}, err
		}
	}

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
		fmt.Sprintf("Mode: %s", publicationModeLabel(opts.DryRun)),
		fmt.Sprintf("Package root action: %s", packageRootAction),
		fmt.Sprintf("Package files: %d", len(packageFiles)),
		fmt.Sprintf("Catalog artifact action: %s %s", catalogAction, catalogArtifact.RelPath),
	}
	if len(generated.StalePaths) > 0 {
		lines = append(lines, fmt.Sprintf("Source generate drift observed: %d stale managed path(s) were bypassed by materializing fresh generated outputs", len(generated.StalePaths)))
	}
	lines = append(lines, "Next:")
	for _, step := range nextSteps {
		lines = append(lines, "  "+step)
	}
	return PluginPublicationMaterializeResult{
		Target:            ctx.target,
		Mode:              publicationModeLabel(opts.DryRun),
		MarketplaceFamily: ctx.channel.Family,
		Dest:              filepath.Clean(ctx.dest),
		PackageRoot:       ctx.packageRoot,
		Details: map[string]string{
			"package_root_action":     packageRootAction,
			"package_file_count":      fmt.Sprintf("%d", len(packageFiles)),
			"catalog_artifact":        catalogArtifact.RelPath,
			"catalog_artifact_action": catalogAction,
		},
		NextSteps: nextSteps,
		Lines:     lines,
	}, nil
}
