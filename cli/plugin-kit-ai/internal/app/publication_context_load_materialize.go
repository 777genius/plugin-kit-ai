package app

import (
	"fmt"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publishschema"
)

func loadPublicationContextForMaterialize(opts PluginPublicationMaterializeOptions) (publicationContext, error) {
	ctx, err := resolvePublicationBaseContext(
		opts.Root,
		opts.Target,
		opts.Dest,
		"publication materialize supports only %q or %q",
		"publication materialize requires --dest",
	)
	if err != nil {
		return publicationContext{}, err
	}

	publicationState, err := publishschema.DiscoverInLayout(ctx.root, pluginmodel.SourceDirName)
	if err != nil {
		return publicationContext{}, err
	}
	publication := ctx.inspection.Publication
	if _, ok := publicationPackageForTarget(publication, ctx.target); !ok {
		return publicationContext{}, fmt.Errorf("target %s is not publication-capable", ctx.target)
	}
	channel, ok := publicationChannelForTarget(publication, ctx.target)
	if !ok {
		return publicationContext{}, fmt.Errorf("target %s requires authored publication channel metadata under src/publish/...", ctx.target)
	}
	packageRoot, err := normalizePackageRoot(opts.PackageRoot, ctx.graph.Manifest.Name)
	if err != nil {
		return publicationContext{}, err
	}

	ctx.packageRoot = packageRoot
	ctx.publication = publication
	ctx.publicationState = publicationState
	ctx.channel = channel
	return ctx, nil
}
