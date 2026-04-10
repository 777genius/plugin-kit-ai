package app

import (
	"path/filepath"
	"slices"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationexec"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publishschema"
)

type publicationContext struct {
	root             string
	target           string
	dest             string
	packageRoot      string
	graph            pluginmanifest.PackageGraph
	inspection       pluginmanifest.Inspection
	publication      publicationmodel.Model
	publicationState publishschema.State
	channel          publicationmodel.Channel
}

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

func (ctx publicationContext) renderLocalCatalogArtifact() (pluginmanifest.Artifact, error) {
	return publicationexec.RenderLocalCatalogArtifact(ctx.graph, ctx.publicationState, ctx.target, "./"+ctx.packageRoot)
}

func (ctx publicationContext) catalogArtifactPath() (string, error) {
	return publicationexec.CatalogArtifactPath(ctx.target)
}

func (ctx publicationContext) destPackageRoot() string {
	return filepath.Join(ctx.dest, filepath.FromSlash(ctx.packageRoot))
}
