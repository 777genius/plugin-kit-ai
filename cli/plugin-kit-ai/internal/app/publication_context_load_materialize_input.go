package app

import (
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publishschema"
)

type materializePublicationContextInput struct {
	publication      publicationmodel.Model
	publicationState publishschema.State
	channel          publicationmodel.Channel
	packageRoot      string
}

func resolveMaterializePublicationContextInput(ctx publicationContext, packageRootInput string) (materializePublicationContextInput, error) {
	publicationState, err := discoverPublicationContextState(ctx)
	if err != nil {
		return materializePublicationContextInput{}, err
	}
	publication, err := requirePublicationCapableTarget(ctx)
	if err != nil {
		return materializePublicationContextInput{}, err
	}
	channel, err := requireMaterializePublicationChannel(ctx, publication)
	if err != nil {
		return materializePublicationContextInput{}, err
	}
	packageRoot, err := resolvePublicationContextPackageRoot(ctx, packageRootInput)
	if err != nil {
		return materializePublicationContextInput{}, err
	}
	return materializePublicationContextInput{
		publication:      publication,
		publicationState: publicationState,
		channel:          channel,
		packageRoot:      packageRoot,
	}, nil
}
