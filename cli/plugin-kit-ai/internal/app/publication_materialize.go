package app

func (PluginService) publicationMaterialize(opts PluginPublicationMaterializeOptions) (PluginPublicationMaterializeResult, error) {
	ctx, err := loadPublicationContextForMaterialize(opts)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	plan, err := preparePublicationMaterialize(ctx)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	if err := applyPublicationMaterialize(ctx, plan, opts.DryRun); err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	return buildPublicationMaterializeResult(ctx, plan, opts.DryRun), nil
}
