package app

import (
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publishschema"
)

func loadPublicationContextForVerify(opts PluginPublicationVerifyRootOptions) (publicationContext, error) {
	ctx, err := resolvePublicationBaseContext(
		opts.Root,
		opts.Target,
		opts.Dest,
		"publication doctor local-root verification supports only %q or %q",
		"publication doctor local-root verification requires --dest",
	)
	if err != nil {
		return publicationContext{}, err
	}

	publicationState, err := publishschema.DiscoverInLayout(ctx.root, pluginmodel.SourceDirName)
	if err != nil {
		return publicationContext{}, err
	}
	packageRoot, err := normalizePackageRoot(opts.PackageRoot, ctx.graph.Manifest.Name)
	if err != nil {
		return publicationContext{}, err
	}

	ctx.packageRoot = packageRoot
	ctx.publication = ctx.inspection.Publication
	ctx.publicationState = publicationState
	return ctx, nil
}
