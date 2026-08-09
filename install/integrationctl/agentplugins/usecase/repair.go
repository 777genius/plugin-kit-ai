package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/pathpolicy"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/ports"
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
	if !ok || (client.Materialization != domain.MaterializationMaterialized && client.Materialization != domain.MaterializationDegraded) {
		return result, fmt.Errorf("plugin is not materialized for %s", input.Client.ClientID)
	}
	if client.ClientBindingID != clientKey || client.ClientID != string(input.Client.ClientID) ||
		client.Scope != string(input.Scope) || client.PhysicalArtifact != physicalID {
		return result, fmt.Errorf("managed repair target identity does not match the selected binding")
	}
	result.Activation = lifecycleOutcome(client)
	if !packageRevisionMatches(client.PackageRevision, input.Envelope) {
		return result, fmt.Errorf("resolved repair package differs from the installed revision; use update")
	}
	if client.PackageRevision == nil || strings.TrimSpace(client.PackageRevision.ResolvedRevision) != strings.TrimSpace(input.Envelope.Source.ResolvedRevision) {
		return result, fmt.Errorf("resolved repair package does not match the exact installed revision")
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
	verifyErr := service.Stager.Verify(ctx, target.ActivePath, expectedDigest)
	if verifyErr == nil {
		verified, verifyErr := service.verifyClientReadOnly(ctx, input, result, client)
		if verifyErr != nil {
			return result, verifyErr
		}
		corrected := client
		corrected.Materialization = domain.MaterializationMaterialized
		if corrected.Verification == domain.VerificationFailed {
			corrected.Verification = domain.VerificationPackageValid
		}
		if verified.Activation != "" {
			corrected.Activation = verified.Activation
		}
		if verified.Authentication != "" {
			corrected.Authentication = verified.Authentication
		}
		if verified.Policy != "" {
			corrected.Policy = verified.Policy
		}
		if verified.Verification != "" {
			corrected.Verification = verified.Verification
		}
		result.Activation = lifecycleOutcome(corrected)
		if corrected.Materialization == client.Materialization && sameLifecycleOutcome(lifecycleOutcome(corrected), lifecycleOutcome(client)) {
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
		corrected.UpdatedAt = service.now().Format("2006-01-02T15:04:05.999999999Z07:00")
		installation.Clients[clientKey] = corrected
		installation.UpdatedAt = corrected.UpdatedAt
		state.Installations[index] = installation
		if err := service.StateStore.Save(state); err != nil {
			return result, fmt.Errorf("persist corrected repair verification state: %w", err)
		}
		result.Mutated = true
		return result, nil
	}
	var verification *ports.VerificationError
	if !errors.As(verifyErr, &verification) || (verification.Kind != ports.VerificationAbsent && verification.Kind != ports.VerificationDigestMismatch) {
		return result, fmt.Errorf("managed package integrity could not be determined; refusing repair: %w", verifyErr)
	}
	beforeDigest := ""
	if verification.Kind == ports.VerificationDigestMismatch {
		if strings.TrimSpace(verification.ActualDigest) == "" {
			return result, fmt.Errorf("managed package verifier reported a digest mismatch without the actual digest; refusing repair")
		}
		beforeDigest = verification.ActualDigest
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
	verifiedState := lifecycleOutcome(client)
	verifiedState.Verification = domain.VerificationPackageValid
	desiredClient := client
	desiredClient.Materialization = domain.MaterializationMaterialized
	desiredClient.Verification = domain.VerificationPackageValid
	desiredClient.UpdatedAt = service.now().Format("2006-01-02T15:04:05.999999999Z07:00")
	installation.Clients[clientKey] = desiredClient
	installation.UpdatedAt = desiredClient.UpdatedAt
	state.Installations[index] = installation
	receipt, err := kernel.ApplyDirectory(ctx, transaction.DirectoryMutation{
		OperationID: operationID, InstallationID: installation.InstallationID, ClientBindingID: clientKey,
		Sequence: nextSequence(client), OwnedBase: delivery.OwnedBase, ActivePath: target.ActivePath,
		StagingPath: delivery.StagingPath, BeforeDigest: beforeDigest, AfterDigest: delivery.ArtifactDigest,
		NativeObjects: delivery.NativeObjects, Activation: verifiedState.Activation, Authentication: verifiedState.Authentication,
		Policy: verifiedState.Policy, Verification: verifiedState.Verification, DesiredState: state,
		Verify: func(verifyContext context.Context, activePath string) error {
			return service.Stager.Verify(verifyContext, activePath, delivery.ArtifactDigest)
		},
	})
	result.Receipt = receipt
	if err != nil {
		return result, err
	}
	result.Mutated = true
	result.Activation = verifiedState
	return result, nil
}

func sameLifecycleOutcome(left, right domain.ActivationOutcome) bool {
	return left.Activation == right.Activation && left.Authentication == right.Authentication &&
		left.Policy == right.Policy && left.Verification == right.Verification
}
