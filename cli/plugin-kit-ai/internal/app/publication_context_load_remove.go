package app

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

	publication, err := requirePublicationCapableTarget(ctx)
	if err != nil {
		return publicationContext{}, err
	}
	channel, err := requireRemovePublicationChannel(ctx, publication)
	if err != nil {
		return publicationContext{}, err
	}
	packageRoot, err := resolvePublicationContextPackageRoot(ctx, opts.PackageRoot)
	if err != nil {
		return publicationContext{}, err
	}
	return withPublicationContextRemove(ctx, packageRoot, publication, channel), nil
}
