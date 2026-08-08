package ports

import (
	"context"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

type SchemaRegistry interface {
	Supports(schemaURI string) bool
	Validate(schemaURI string, value any) error
	Digest(schemaURI string) (string, bool)
}

type PackageLoader interface {
	Load(context.Context, domain.LoadInput) (domain.PackageEnvelope, error)
}

type ClientDetector interface {
	Detect(context.Context) ([]domain.DetectedClient, error)
}

type DeliveryPlanner interface {
	Plan(context.Context, domain.PackageEnvelope, domain.DetectedClient, domain.InstallScope, string) (domain.DeliveryPlan, error)
}

type DeliveryTargetResolver interface {
	ResolveTarget(context.Context, domain.DetectedClient, domain.InstallScope, string) (domain.DeliveryTarget, error)
}

type PackageStager interface {
	Stage(context.Context, domain.PackageEnvelope, domain.DeliveryPlan, string, domain.CompatibilityHints) (domain.StagedDelivery, error)
	Discard(context.Context, domain.StagedDelivery) error
	Verify(context.Context, string, string) error
}

type ClientActivator interface {
	Activate(context.Context, domain.ActivationRequest) (domain.ActivationOutcome, error)
	Deactivate(context.Context, domain.DeactivationRequest) (domain.DeactivationOutcome, error)
}
