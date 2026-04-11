package app

import (
	"fmt"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publishschema"
)

func discoverPublicationContextState(ctx publicationContext) (publishschema.State, error) {
	return publishschema.DiscoverInLayout(ctx.root, ctx.inspection.Layout.AuthoredRoot)
}

func requirePublicationCapableTarget(ctx publicationContext) (publicationmodel.Model, error) {
	publication := ctx.inspection.Publication
	if _, ok := publicationPackageForTarget(publication, ctx.target); !ok {
		return publicationmodel.Model{}, fmt.Errorf("target %s is not publication-capable", ctx.target)
	}
	return publication, nil
}

func requireMaterializePublicationChannel(ctx publicationContext, publication publicationmodel.Model) (publicationmodel.Channel, error) {
	channel, ok := publicationChannelForTarget(publication, ctx.target)
	if ok {
		return channel, nil
	}
	authoredRoot := ctx.inspection.Layout.AuthoredRoot
	if authoredRoot == "" {
		authoredRoot = pluginmodel.SourceDirName
	}
	return publicationmodel.Channel{}, fmt.Errorf("target %s requires authored publication channel metadata under %s/publish/...", ctx.target, authoredRoot)
}

func requireRemovePublicationChannel(ctx publicationContext, publication publicationmodel.Model) (publicationmodel.Channel, error) {
	channel, ok := publicationChannelForTarget(publication, ctx.target)
	if ok {
		return channel, nil
	}
	return publicationmodel.Channel{}, fmt.Errorf("target %s requires authored publication channel metadata under publish/...", ctx.target)
}

func resolvePublicationContextPackageRoot(ctx publicationContext, packageRootInput string) (string, error) {
	return normalizePackageRoot(packageRootInput, ctx.graph.Manifest.Name)
}
