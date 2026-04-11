package app

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

	publicationState, err := discoverPublicationContextState(ctx)
	if err != nil {
		return publicationContext{}, err
	}
	publication, err := requirePublicationCapableTarget(ctx)
	if err != nil {
		return publicationContext{}, err
	}
	channel, err := requireMaterializePublicationChannel(ctx, publication)
	if err != nil {
		return publicationContext{}, err
	}
	packageRoot, err := resolvePublicationContextPackageRoot(ctx, opts.PackageRoot)
	if err != nil {
		return publicationContext{}, err
	}
	return withPublicationContextMaterialize(ctx, packageRoot, publication, publicationState, channel), nil
}
