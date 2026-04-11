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
	policyInput, err := resolveMaterializePublicationPolicyInput(ctx)
	if err != nil {
		return materializePublicationContextInput{}, err
	}
	packageRoot, err := resolvePublicationContextPackageRoot(ctx, packageRootInput)
	if err != nil {
		return materializePublicationContextInput{}, err
	}
	return buildMaterializePublicationContextInput(policyInput, packageRoot), nil
}

type materializePublicationPolicyInput struct {
	publication      publicationmodel.Model
	publicationState publishschema.State
	channel          publicationmodel.Channel
}

func resolveMaterializePublicationPolicyInput(ctx publicationContext) (materializePublicationPolicyInput, error) {
	publicationState, err := discoverPublicationContextState(ctx)
	if err != nil {
		return materializePublicationPolicyInput{}, err
	}
	publication, err := requirePublicationCapableTarget(ctx)
	if err != nil {
		return materializePublicationPolicyInput{}, err
	}
	channel, err := requireMaterializePublicationChannel(ctx, publication)
	if err != nil {
		return materializePublicationPolicyInput{}, err
	}
	return materializePublicationPolicyInput{
		publication:      publication,
		publicationState: publicationState,
		channel:          channel,
	}, nil
}

func buildMaterializePublicationContextInput(policy materializePublicationPolicyInput, packageRoot string) materializePublicationContextInput {
	return materializePublicationContextInput{
		publication:      policy.publication,
		publicationState: policy.publicationState,
		channel:          policy.channel,
		packageRoot:      packageRoot,
	}
}
