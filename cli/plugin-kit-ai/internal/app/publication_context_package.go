package app

import (
	"slices"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
)

func (ctx publicationContext) expectedMaterializedPackageArtifacts() ([]pluginmanifest.Artifact, pluginmanifest.RenderResult, error) {
	generated, err := pluginmanifest.Generate(ctx.root, ctx.target)
	if err != nil {
		return nil, pluginmanifest.RenderResult{}, err
	}
	managedPaths, err := inspectionManagedPathsForTarget(ctx.inspection, ctx.target)
	if err != nil {
		return nil, pluginmanifest.RenderResult{}, err
	}
	managedPaths = append(managedPaths, ctx.graph.Portable.Paths("skills")...)
	managedPaths = slices.Compact(sortedSlashPaths(managedPaths))
	packageFiles, err := materializedPackageArtifacts(ctx.root, ctx.packageRoot, managedPaths, generated)
	if err != nil {
		return nil, pluginmanifest.RenderResult{}, err
	}
	return packageFiles, generated, nil
}
