package app

import (
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationexec"
)

func (ctx publicationContext) renderLocalCatalogArtifact() (pluginmanifest.Artifact, error) {
	return publicationexec.RenderLocalCatalogArtifact(ctx.graph, ctx.publicationState, ctx.target, "./"+ctx.packageRoot)
}

func (ctx publicationContext) catalogArtifactPath() (string, error) {
	return publicationexec.CatalogArtifactPath(ctx.target)
}

func (ctx publicationContext) destPackageRoot() string {
	return filepath.Join(ctx.dest, filepath.FromSlash(ctx.packageRoot))
}
