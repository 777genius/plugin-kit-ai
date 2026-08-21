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
	PurgeData           bool
}

type RemoveResult struct {
	InstallationID       string                     `json:"installation_id"`
	Plugin               string                     `json:"plugin"`
	ClientID             domain.ClientID            `json:"client_id"`
	RequiresConfirmation bool                       `json:"requires_confirmation"`
	Mutated              bool                       `json:"mutated"`
	Deactivation         domain.DeactivationOutcome `json:"deactivation,omitempty"`
	Receipt              domain.MutationReceipt     `json:"-"`
	AffectedSurfaces     []string                   `json:"affected_surfaces,omitempty"`
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
	if input.PurgeData {
		if service.PluginData == nil {
			return RemoveResult{}, fmt.Errorf("PLUGIN_DATA manager is required for purge")
		}
		for _, receipt := range installation.DataReceipts {
			if receipt.State != domain.DataReceiptOwned {
				return RemoveResult{}, fmt.Errorf("PLUGIN_DATA receipt %s is not safely owned; purge aborted before mutation", receipt.DataReceiptID)
			}
			if err := service.PluginData.ValidateData(ctx, receipt); err != nil {
				return RemoveResult{}, fmt.Errorf("validate all PLUGIN_DATA receipts before purge: %w", err)
			}
		}
	}
	clientKey, client, err := findClientBinding(installation, input.Client.ClientID, input.Scope)
	if err != nil {
		return RemoveResult{}, err
	}
	result := RemoveResult{
		InstallationID:   installation.InstallationID,
		Plugin:           installation.DeclaredName,
		ClientID:         input.Client.ClientID,
		AffectedSurfaces: append([]string(nil), client.AffectedSurfaces...),
	}
	if len(result.AffectedSurfaces) == 0 {
		result.AffectedSurfaces = []string{client.ClientID}
	}
	deactivation, err := service.Activator.Deactivate(ctx, domain.DeactivationRequest{
		Client: input.Client, DeclaredName: installation.DeclaredName,
		CurrentActivation: client.Activation, Interactive: input.Interactive,
		ExternalUninstalled: input.ExternalUninstalled,
		Confirmed:           input.Confirmed && !input.DryRun,
		PhysicalArtifactID:  client.PhysicalArtifact,
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
	if err := service.finalizeRemovedBinding(ctx, installation.InstallationID, clientKey, input.PurgeData); err != nil {
		return result, err
	}
	return result, nil
}

func (service Service) finalizeRemovedBinding(ctx context.Context, installationID, clientKey string, purge bool) error {
	return service.finalizeRemovedBindings(ctx, installationID, []string{clientKey}, purge)
}

func (service Service) finalizeRemovedBindings(ctx context.Context, installationID string, clientKeys []string, purge bool) error {
	state, err := service.StateStore.Load()
	if err != nil {
		return err
	}
	for index, installation := range state.Installations {
		if installation.InstallationID != installationID {
			continue
		}
		removed := make([]domain.ClientBinding, 0, len(clientKeys))
		for _, clientKey := range clientKeys {
			client, ok := installation.Clients[clientKey]
			if !ok || client.Materialization != domain.MaterializationAbsent {
				return fmt.Errorf("removed binding state is not authoritative")
			}
			removed = append(removed, client)
			delete(installation.Clients, clientKey)
		}
		active := 0
		for _, binding := range installation.Clients {
			if binding.Materialization != domain.MaterializationAbsent {
				active++
			}
		}
		if active == 0 {
			for key, binding := range installation.Clients {
				if binding.Materialization == domain.MaterializationAbsent {
					delete(installation.Clients, key)
				}
			}
			// State written before PLUGIN_DATA receipts existed (and packages that
			// did not need stdio data while installed) can reach their final-binding
			// removal without a receipt. Establish the minimal owned data root now,
			// before discarding the last binding's backend/scope provenance. Normal
			// removal always retains it; purge is the only consuming operation.
			if len(installation.DataReceipts) == 0 {
				if service.PluginData == nil {
					return fmt.Errorf("PLUGIN_DATA manager is required to retain final-binding data")
				}
				for _, client := range removed {
					receipt, _, err := service.PluginData.EnsureData(ctx, installation.InstallationID, client.PhysicalArtifact, client.Scope)
					if err != nil {
						return fmt.Errorf("establish retained PLUGIN_DATA ownership: %w", err)
					}
					if installation.DataReceipts == nil {
						installation.DataReceipts = map[string]domain.DataReceipt{}
					}
					installation.DataReceipts[receipt.DataReceiptID] = receipt
				}
			}
			if purge {
				for _, receipt := range installation.DataReceipts {
					if err := service.PluginData.PurgeData(ctx, receipt); err != nil {
						return fmt.Errorf("purge owned PLUGIN_DATA: %w", err)
					}
				}
				state.Installations = append(state.Installations[:index], state.Installations[index+1:]...)
				return service.StateStore.Save(state)
			}
			installation.DataRetained = true
		}
		installation.UpdatedAt = service.now().Format(time.RFC3339Nano)
		state.Installations[index] = installation
		return service.StateStore.Save(state)
	}
	return fmt.Errorf("installation disappeared after removal")
}

// PurgeRetainedData handles the only target-less removal: an explicit purge of
// a data_retained installation after complete ownership preflight.
func (service Service) PurgeRetainedData(ctx context.Context, selector string, confirmed bool) error {
	if service.StateStore == nil || service.PluginData == nil {
		return fmt.Errorf("state and PLUGIN_DATA managers are required")
	}
	release, err := service.beginMutation(ctx, false, confirmed)
	if err != nil {
		return err
	}
	if release != nil {
		defer func() { _ = release() }()
	}
	state, err := service.StateStore.Load()
	if err != nil {
		return err
	}
	index, installation, err := findInstallation(state, selector)
	if err != nil {
		return err
	}
	if !installation.DataRetained || len(installation.Clients) != 0 {
		return fmt.Errorf("installation is not a data_retained record")
	}
	for _, receipt := range installation.DataReceipts {
		if receipt.State != domain.DataReceiptOwned {
			return fmt.Errorf("PLUGIN_DATA receipt %s is not safely owned", receipt.DataReceiptID)
		}
		if err := service.PluginData.ValidateData(ctx, receipt); err != nil {
			return err
		}
	}
	if !confirmed {
		return nil
	}
	for _, receipt := range installation.DataReceipts {
		if err := service.PluginData.PurgeData(ctx, receipt); err != nil {
			return err
		}
	}
	state.Installations = append(state.Installations[:index], state.Installations[index+1:]...)
	return service.StateStore.Save(state)
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
