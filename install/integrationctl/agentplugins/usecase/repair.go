package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/pathpolicy"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/transaction"
)

// Repair replaces a missing or modified managed directory from a freshly
// resolved package. It never uses a persisted target until that target has
// been matched to the target resolver's current safe result.
func (service Service) Repair(ctx context.Context, input AddInput) (AddResult, error) {
	if service.StateStore == nil || service.Planner == nil || service.Targets == nil || service.Stager == nil {
		return AddResult{}, fmt.Errorf("agentplugins repair dependencies are incomplete")
	}
	release, err := service.beginMutation(ctx, input.DryRun, input.Confirmed)
	if err != nil {
		return AddResult{}, err
	}
	if release != nil {
		defer func() { _ = release() }()
	}
	state, err := service.StateStore.Load()
	if err != nil {
		return AddResult{}, err
	}
	index, existing := findSourceInstallation(state, domain.ComputeSourceBindingID(input.Envelope.Source))
	if !existing {
		return AddResult{}, fmt.Errorf("resolved source is not bound to an installation")
	}
	installation := state.Installations[index]
	if installation.InstallationID != input.InstallationID || installation.DeclaredName != input.Envelope.Manifest.Name {
		return AddResult{}, fmt.Errorf("resolved repair source identity does not match the selected installation")
	}
	physicalID := domain.ComputePhysicalArtifactID(installation.DeclaredName, installation.InstallationID)
	plan, err := service.Planner.Plan(ctx, input.Envelope, input.Client, input.Scope, physicalID)
	if err != nil {
		return AddResult{}, err
	}
	result := AddResult{InstallationID: installation.InstallationID, Plan: plan}
	clientKey := domain.ComputeClientBindingID(installation.InstallationID, string(input.Client.ClientID), string(input.Scope), plan.ActivePath)
	client, ok := installation.Clients[clientKey]
	if !ok || client.Materialization != domain.MaterializationMaterialized {
		return result, fmt.Errorf("plugin is not materialized for %s", input.Client.ClientID)
	}
	result.Activation = lifecycleOutcome(client)
	if !packageRevisionMatches(client.PackageRevision, input.Envelope) {
		return result, fmt.Errorf("resolved repair package differs from the installed revision; use update")
	}
	target, err := service.Targets.ResolveTarget(ctx, input.Client, input.Scope, client.PhysicalArtifact)
	if err != nil {
		return result, fmt.Errorf("resolve managed repair target: %w", err)
	}
	if err := pathpolicy.RequireExactPath(target.ActivePath, client.TargetLocator); err != nil {
		return result, fmt.Errorf("refuse repair of untrusted persisted target: %w", err)
	}
	expectedDigest := managedDigest(client)
	if expectedDigest == "" {
		return result, fmt.Errorf("managed package digest is missing; refusing repair")
	}
	if verifyErr := service.Stager.Verify(ctx, target.ActivePath, expectedDigest); verifyErr == nil {
		verified, verifyErr := service.verifyClientReadOnly(ctx, input, result, client)
		if verifyErr != nil {
			return result, verifyErr
		}
		if verified.Activation == domain.ActivationActive && verified.Verification == domain.VerificationInstalled {
			result.Activation = verified
		}
		result.NoChange = true
		return result, nil
	}
	if input.DryRun {
		return result, nil
	}
	if !input.Confirmed {
		result.RequiresConfirmation = true
		return result, nil
	}
	operationID := strings.TrimSpace(input.OperationID)
	if operationID == "" {
		operationID, err = newOperationID()
		if err != nil {
			return result, err
		}
	}
	delivery, err := service.Stager.Stage(ctx, input.Envelope, plan, operationID, input.Hints)
	if err != nil {
		return result, err
	}
	defer func() { _ = service.Stager.Discard(context.Background(), delivery) }()
	if delivery.ActivePath != target.ActivePath {
		return result, fmt.Errorf("stager returned an unexpected repair target")
	}
	if delivery.ArtifactDigest != expectedDigest {
		return result, fmt.Errorf("resolved repair projection digest differs from the originally managed package")
	}
	kernel := service.Kernel
	kernel.StateStore = service.StateStore
	receipt, err := kernel.ApplyDirectory(ctx, transaction.DirectoryMutation{
		OperationID: operationID, InstallationID: installation.InstallationID, ClientBindingID: clientKey,
		Sequence: nextSequence(client), OwnedBase: delivery.OwnedBase, ActivePath: target.ActivePath,
		StagingPath: delivery.StagingPath, BeforeDigest: "", AfterDigest: delivery.ArtifactDigest,
		NativeObjects: delivery.NativeObjects, Activation: client.Activation, Authentication: client.Authentication,
		Policy: client.Policy, Verification: client.Verification, DesiredState: state,
		Verify: func(verifyContext context.Context, activePath string) error {
			return service.Stager.Verify(verifyContext, activePath, delivery.ArtifactDigest)
		},
	})
	result.Receipt = receipt
	if err != nil {
		return result, err
	}
	result.Mutated = true
	return result, nil
}
