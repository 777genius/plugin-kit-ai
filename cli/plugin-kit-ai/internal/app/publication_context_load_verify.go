package app

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

	publicationState, err := discoverPublicationContextState(ctx)
	if err != nil {
		return publicationContext{}, err
	}
	packageRoot, err := resolvePublicationContextPackageRoot(ctx, opts.PackageRoot)
	if err != nil {
		return publicationContext{}, err
	}
	return withPublicationContextVerify(ctx, packageRoot, publicationState), nil
}
