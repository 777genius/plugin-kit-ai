package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/pathpolicy"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/transaction"
)

type RemoveInput struct {
	Selector            string
	Client              domain.DetectedClient
	Scope               domain.InstallScope
	DryRun              bool
	Confirmed           bool
	Interactive         bool
	ExternalUninstalled bool
	OperationID         string
	BackendExecutable   string
}

type RemoveResult struct {
	InstallationID       string                     `json:"installation_id"`
	Plugin               string                     `json:"plugin"`
	ClientID             domain.ClientID            `json:"client_id"`
	RequiresConfirmation bool                       `json:"requires_confirmation"`
	Mutated              bool                       `json:"mutated"`
	Deactivation         domain.DeactivationOutcome `json:"deactivation,omitempty"`
	Receipt              domain.MutationReceipt     `json:"-"`
}

func (service Service) Remove(ctx context.Context, input RemoveInput) (RemoveResult, error) {
	if service.StateStore == nil || service.Targets == nil || service.Stager == nil || service.Activator == nil {
		return RemoveResult{}, fmt.Errorf("agentplugins service dependencies are incomplete")
	}
	release, err := service.beginMutation(ctx, input.DryRun, input.Confirmed)
	if err != nil {
		return RemoveResult{}, err
	}
	if release != nil {
		defer func() { _ = release() }()
	}
	state, err := service.StateStore.Load()
	if err != nil {
		return RemoveResult{}, err
	}
	installationIndex, installation, err := findInstallation(state, input.Selector)
	if err != nil {
		return RemoveResult{}, err
	}
	clientKey, client, err := findClientBinding(installation, input.Client.ClientID, input.Scope)
	if err != nil {
		return RemoveResult{}, err
	}
	result := RemoveResult{
		InstallationID: installation.InstallationID,
		Plugin:         installation.DeclaredName,
		ClientID:       input.Client.ClientID,
	}
	deactivation, err := service.Activator.Deactivate(ctx, domain.DeactivationRequest{
		Client: input.Client, DeclaredName: installation.DeclaredName,
		CurrentActivation: client.Activation, Interactive: input.Interactive,
		ExternalUninstalled: input.ExternalUninstalled,
		BackendExecutable:   input.BackendExecutable,
	})
	result.Deactivation = deactivation
	if err != nil {
		return result, err
	}
	if input.DryRun {
		return result, nil
	}
	if !deactivation.ArtifactRemovalAllowed {
		return result, nil
	}
	if !input.Confirmed {
		result.RequiresConfirmation = true
		return result, nil
	}
	if deactivation.ExternalRemovalComplete && client.Activation == domain.ActivationActive {
		if err := service.markDeactivated(state, installationIndex, clientKey); err != nil {
			return result, err
		}
		state, err = service.StateStore.Load()
		if err != nil {
			return result, err
		}
		installation = state.Installations[installationIndex]
		client = installation.Clients[clientKey]
	}
	expectedDigest := managedDigest(client)
	if expectedDigest == "" {
		return result, fmt.Errorf("managed package digest is missing; refusing removal and retaining state for reviewed recovery")
	}
	target, err := service.Targets.ResolveTarget(ctx, input.Client, input.Scope, client.PhysicalArtifact)
	if err != nil {
		return result, fmt.Errorf("resolve managed removal target: %w", err)
	}
	if err := pathpolicy.RequireExactPath(target.ActivePath, client.TargetLocator); err != nil {
		return result, fmt.Errorf("refuse removal from untrusted persisted target: %w", err)
	}
	if err := service.Stager.Verify(ctx, client.TargetLocator, expectedDigest); err != nil {
		return result, fmt.Errorf("managed package was changed or is missing; refusing silent removal and retaining state: %w", err)
	}
	operationID := strings.TrimSpace(input.OperationID)
	if operationID == "" {
		operationID, err = newOperationID()
		if err != nil {
			return result, err
		}
	}
	kernel := service.Kernel
	kernel.StateStore = service.StateStore
	receipt, err := kernel.RemoveDirectory(ctx, transaction.DirectoryRemoval{
		OperationID: operationID, InstallationID: installation.InstallationID,
		ClientBindingID: clientKey, Sequence: nextSequence(client),
		OwnedBase: target.TargetRoot, ActivePath: client.TargetLocator,
		BeforeDigest: expectedDigest,
		Verify: func(verifyContext context.Context, activePath string) error {
			return service.Stager.Verify(verifyContext, activePath, expectedDigest)
		},
	})
	result.Receipt = receipt
	if err != nil {
		return result, err
	}
	result.Mutated = true
	return result, nil
}

func (service Service) markDeactivated(state domain.StateFileV2, installationIndex int, clientKey string) error {
	installation := state.Installations[installationIndex]
	client := installation.Clients[clientKey]
	client.Activation = domain.ActivationNotRequired
	client.Verification = domain.VerificationPackageValid
	client.UpdatedAt = service.now().Format(time.RFC3339Nano)
	installation.Clients[clientKey] = client
	installation.UpdatedAt = client.UpdatedAt
	state.Installations[installationIndex] = installation
	return service.StateStore.Save(state)
}

func findInstallation(state domain.StateFileV2, selector string) (int, domain.Installation, error) {
	selector = strings.TrimSpace(selector)
	matchIndex := -1
	for index, installation := range state.Installations {
		if installation.InstallationID == selector {
			return index, installation, nil
		}
		if installation.DeclaredName == selector {
			if matchIndex >= 0 {
				return -1, domain.Installation{}, fmt.Errorf("installation name %q is ambiguous; use installation_id", selector)
			}
			matchIndex = index
		}
	}
	if matchIndex < 0 {
		return -1, domain.Installation{}, fmt.Errorf("installation %q was not found", selector)
	}
	return matchIndex, state.Installations[matchIndex], nil
}

func findClientBinding(installation domain.Installation, clientID domain.ClientID, scope domain.InstallScope) (string, domain.ClientBinding, error) {
	var matchKey string
	var match domain.ClientBinding
	for key, client := range installation.Clients {
		if client.ClientID != string(clientID) || client.Scope != string(scope) || client.Materialization == domain.MaterializationAbsent {
			continue
		}
		if matchKey != "" {
			return "", domain.ClientBinding{}, fmt.Errorf("multiple client bindings match %s/%s", clientID, scope)
		}
		matchKey, match = key, client
	}
	if matchKey == "" {
		return "", domain.ClientBinding{}, fmt.Errorf("plugin is not materialized for %s/%s", clientID, scope)
	}
	return matchKey, match, nil
}
