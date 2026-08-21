package usecase

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/pathpolicy"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/transaction"
)

type RemoveGroupInput struct {
	Selector         string
	Targets          []RemoveInput
	OperationGroupID string
	DryRun           bool
	Confirmed        bool
	PurgeData        bool
}

type RemoveGroupResult struct {
	InstallationID   string                   `json:"installation_id"`
	OperationGroupID string                   `json:"operation_group_id,omitempty"`
	Targets          []RemoveResult           `json:"targets"`
	Receipts         []domain.MutationReceipt `json:"-"`
	Mutated          bool                     `json:"mutated"`
}

type plannedRemoval struct {
	input         RemoveInput
	clientKey     string
	client        domain.ClientBinding
	targetRoot    string
	resultIndexes []int
}

func (service Service) RemoveGroup(ctx context.Context, input RemoveGroupInput) (RemoveGroupResult, error) {
	if len(input.Targets) == 0 {
		return RemoveGroupResult{}, fmt.Errorf("at least one installed target is required")
	}
	if service.StateStore == nil || service.Targets == nil || service.Stager == nil || service.Activator == nil {
		return RemoveGroupResult{}, fmt.Errorf("agentplugins group removal dependencies are incomplete")
	}
	release, err := service.beginMutation(ctx, input.DryRun, input.Confirmed)
	if err != nil {
		return RemoveGroupResult{}, err
	}
	if release != nil {
		defer func() { _ = release() }()
	}
	state, err := service.StateStore.Load()
	if err != nil {
		return RemoveGroupResult{}, err
	}
	installationIndex, installation, err := findInstallation(state, input.Selector)
	if err != nil {
		return RemoveGroupResult{}, err
	}
	groupID := strings.TrimSpace(input.OperationGroupID)
	if groupID == "" {
		groupID, err = newOperationID()
		if err != nil {
			return RemoveGroupResult{}, err
		}
	}
	result := RemoveGroupResult{InstallationID: installation.InstallationID, OperationGroupID: groupID, Targets: make([]RemoveResult, len(input.Targets))}
	if input.PurgeData {
		if service.PluginData == nil {
			return result, fmt.Errorf("PLUGIN_DATA manager is required for purge")
		}
		for _, receipt := range installation.DataReceipts {
			if receipt.State != domain.DataReceiptOwned {
				return result, fmt.Errorf("PLUGIN_DATA receipt %s is not safely owned", receipt.DataReceiptID)
			}
			if err := service.PluginData.ValidateData(ctx, receipt); err != nil {
				return result, err
			}
		}
	}
	byBinding := map[string]int{}
	planned := []plannedRemoval{}
	for targetIndex, targetInput := range input.Targets {
		clientKey, client, err := findClientBinding(installation, targetInput.Client.ClientID, targetInput.Scope)
		if err != nil {
			// Either shared Copilot surface addresses the one physical binding.
			for key, candidate := range installation.Clients {
				if sameNativeBackend(domain.ClientID(candidate.ClientID), targetInput.Client.ClientID) && candidate.Scope == string(targetInput.Scope) && candidate.Materialization != domain.MaterializationAbsent {
					clientKey, client, err = key, candidate, nil
					break
				}
			}
			if err != nil {
				return result, err
			}
		}
		result.Targets[targetIndex] = RemoveResult{InstallationID: installation.InstallationID, Plugin: installation.DeclaredName, ClientID: targetInput.Client.ClientID,
			AffectedSurfaces: append([]string(nil), client.AffectedSurfaces...)}
		if prior, ok := byBinding[clientKey]; ok {
			planned[prior].resultIndexes = append(planned[prior].resultIndexes, targetIndex)
			continue
		}
		expected := managedDigest(client)
		if expected == "" {
			return result, fmt.Errorf("managed package digest is missing")
		}
		target, err := service.Targets.ResolveTarget(ctx, targetInput.Client, targetInput.Scope, client.PhysicalArtifact)
		if err != nil {
			return result, err
		}
		if sameNativeBackend(domain.ClientID(client.ClientID), targetInput.Client.ClientID) {
			target.ActivePath = client.TargetLocator
			target.TargetRoot = filepath.Dir(client.TargetLocator)
		}
		if err := pathpolicy.RequireExactPath(target.ActivePath, client.TargetLocator); err != nil {
			return result, err
		}
		if err := service.Stager.Verify(ctx, client.TargetLocator, expected); err != nil {
			return result, fmt.Errorf("ownership verification failed before grouped removal: %w", err)
		}
		outcome, err := service.Activator.Deactivate(ctx, domain.DeactivationRequest{Client: targetInput.Client, DeclaredName: installation.DeclaredName,
			CurrentActivation: client.Activation, Interactive: targetInput.Interactive, ExternalUninstalled: targetInput.ExternalUninstalled,
			Confirmed: false, PhysicalArtifactID: client.PhysicalArtifact, BackendExecutable: targetInput.BackendExecutable})
		result.Targets[targetIndex].Deactivation = outcome
		if err != nil {
			return result, err
		}
		byBinding[clientKey] = len(planned)
		planned = append(planned, plannedRemoval{input: targetInput, clientKey: clientKey, client: client, targetRoot: target.TargetRoot, resultIndexes: []int{targetIndex}})
	}
	if input.DryRun || !input.Confirmed {
		return result, nil
	}
	for plannedIndex := range planned {
		item := &planned[plannedIndex]
		outcome, err := service.Activator.Deactivate(ctx, domain.DeactivationRequest{Client: item.input.Client, DeclaredName: installation.DeclaredName,
			CurrentActivation: item.client.Activation, Interactive: item.input.Interactive, ExternalUninstalled: item.input.ExternalUninstalled,
			Confirmed: true, PhysicalArtifactID: item.client.PhysicalArtifact, BackendExecutable: item.input.BackendExecutable})
		for _, resultIndex := range item.resultIndexes {
			result.Targets[resultIndex].Deactivation = outcome
		}
		if err != nil {
			return result, fmt.Errorf("external deactivation failed; managed materialization was retained for repair: %w", err)
		}
		if !outcome.ArtifactRemovalAllowed {
			return result, fmt.Errorf("client did not authorize managed artifact removal")
		}
	}
	removals := make([]transaction.DirectoryRemoval, 0, len(planned))
	for index, item := range planned {
		expected := managedDigest(item.client)
		activePath := item.client.TargetLocator
		operationID := fmt.Sprintf("%s-%03d", groupID, index+1)
		removals = append(removals, transaction.DirectoryRemoval{OperationID: operationID, OperationGroupID: groupID,
			InstallationID: installation.InstallationID, ClientBindingID: item.clientKey, Sequence: nextSequence(item.client), OwnedBase: item.targetRoot,
			ActivePath: activePath, BeforeDigest: expected, Verify: func(verifyContext context.Context, path string) error {
				return service.Stager.Verify(verifyContext, path, expected)
			}})
	}
	installation.OperationGroupID = groupID
	state.Installations[installationIndex] = installation
	kernel := service.Kernel
	kernel.StateStore = service.StateStore
	receipts, err := kernel.RemoveDirectoryGroup(ctx, transaction.DirectoryRemovalGroup{OperationGroupID: groupID, Removals: removals, DesiredState: state})
	result.Receipts = receipts
	if err != nil {
		return result, err
	}
	result.Mutated = true
	for index := range result.Targets {
		result.Targets[index].Mutated = true
	}
	keys := make([]string, 0, len(planned))
	for _, item := range planned {
		keys = append(keys, item.clientKey)
	}
	sort.Strings(keys)
	if err := service.finalizeRemovedBindings(ctx, installation.InstallationID, keys, input.PurgeData); err != nil {
		return result, err
	}
	return result, nil
}
