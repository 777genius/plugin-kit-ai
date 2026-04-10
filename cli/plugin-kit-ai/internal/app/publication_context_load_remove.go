package app

import "fmt"

func loadPublicationContextForRemove(opts PluginPublicationRemoveOptions) (publicationContext, error) {
	ctx, err := resolvePublicationBaseContext(
		opts.Root,
		opts.Target,
		opts.Dest,
		"publication remove supports only %q or %q",
		"publication remove requires --dest",
	)
	if err != nil {
		return publicationContext{}, err
	}

	publication := ctx.inspection.Publication
	if _, ok := publicationPackageForTarget(publication, ctx.target); !ok {
		return publicationContext{}, fmt.Errorf("target %s is not publication-capable", ctx.target)
	}
	channel, ok := publicationChannelForTarget(publication, ctx.target)
	if !ok {
		return publicationContext{}, fmt.Errorf("target %s requires authored publication channel metadata under publish/...", ctx.target)
	}
	packageRoot, err := normalizePackageRoot(opts.PackageRoot, ctx.graph.Manifest.Name)
	if err != nil {
		return publicationContext{}, err
	}

	ctx.packageRoot = packageRoot
	ctx.publication = publication
	ctx.channel = channel
	return ctx, nil
}
