package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/ports"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/transaction"
)

type LegacyRemoveInput struct {
	Selector    string
	DryRun      bool
	Confirmed   bool
	OperationID string
}

type LegacyRemoveResult struct {
	InstallationID       string                  `json:"installation_id"`
	Plugin               string                  `json:"plugin"`
	Targets              []string                `json:"targets"`
	LegacyPlan           ports.LegacyRemovalPlan `json:"legacy_plan"`
	RequiresConfirmation bool                    `json:"requires_confirmation"`
	Reconciled           bool                    `json:"reconciled,omitempty"`
	Mutated              bool                    `json:"mutated"`
}

func (service Service) RemoveLegacy(ctx context.Context, input LegacyRemoveInput) (LegacyRemoveResult, error) {
	if service.StateStore == nil || service.Legacy == nil || service.LegacyLock == nil {
		return LegacyRemoveResult{}, fmt.Errorf("legacy lifecycle dependencies are incomplete")
	}
	release, err := service.beginMutation(ctx, input.DryRun, input.Confirmed)
	if err != nil {
		return LegacyRemoveResult{}, err
	}
	if release != nil {
		defer func() { _ = release() }()
	}
	state, err := service.StateStore.Load()
	if err != nil {
		return LegacyRemoveResult{}, err
	}
	installationIndex, installation, err := findInstallation(state, input.Selector)
	if err != nil {
		return LegacyRemoveResult{}, err
	}
	if installation.Package.LoaderKind != domain.LoaderKindLegacy {
		return LegacyRemoveResult{}, fmt.Errorf("installation %s is not managed by the legacy lifecycle", installation.InstallationID)
	}
	result := LegacyRemoveResult{
		InstallationID: installation.InstallationID,
		Plugin:         installation.DeclaredName,
		Targets:        materializedTargets(installation),
	}
	exists, err := service.Legacy.Exists(ctx, installation.DeclaredName)
	if err != nil {
		return result, fmt.Errorf("inspect legacy lifecycle state: %w", err)
	}
	if exists {
		result.LegacyPlan, err = service.Legacy.PlanRemove(ctx, installation.DeclaredName)
		if err != nil {
			return result, fmt.Errorf("plan legacy removal: %w", err)
		}
	} else {
		result.LegacyPlan = ports.LegacyRemovalPlan{Summary: "legacy lifecycle already reports this installation absent"}
		result.Reconciled = true
	}
	if input.DryRun {
		return result, nil
	}
	if !input.Confirmed {
		result.RequiresConfirmation = true
		return result, nil
	}
	if exists {
		if _, err := service.Legacy.Remove(ctx, installation.DeclaredName); err != nil {
			return result, fmt.Errorf("remove through legacy lifecycle: %w", err)
		}
	}
	legacyRelease, err := service.LegacyLock.Acquire(ctx, "state")
	if err != nil {
		return result, fmt.Errorf("acquire legacy state lock for reconciliation: %w", err)
	}
	defer func() { _ = legacyRelease() }()
	stillExists, err := service.Legacy.Exists(ctx, installation.DeclaredName)
	if err != nil {
		return result, fmt.Errorf("verify legacy lifecycle removal: %w", err)
	}
	if stillExists {
		return result, fmt.Errorf("legacy lifecycle changed before reconciliation; retry after reviewing the current legacy state")
	}
	operationID := strings.TrimSpace(input.OperationID)
	if operationID == "" {
		operationID, err = newOperationID()
		if err != nil {
			return result, err
		}
	}
	timestamp := service.now().Format(time.RFC3339Nano)
	for key, client := range installation.Clients {
		if client.Materialization == domain.MaterializationAbsent && len(client.NativeObjects) == 0 {
			continue
		}
		client.Receipts = append(client.Receipts, domain.MutationReceipt{
			OperationID: operationID + "-" + client.ClientBindingID,
			Sequence:    nextSequence(client), MutationType: "legacy_remove_bridge",
			ClientBindingID: client.ClientBindingID, BeforeDigest: managedDigest(client),
			Phase: transaction.ReceiptPhaseCommitted,
		})
		client.Materialization = domain.MaterializationAbsent
		client.Activation = domain.ActivationNotRequired
		client.Authentication = domain.AuthenticationNotRequired
		client.Policy = domain.PolicyAllowed
		client.Verification = domain.VerificationNotRun
		client.NativeObjects = nil
		client.UpdatedAt = timestamp
		installation.Clients[key] = client
	}
	installation.UpdatedAt = timestamp
	state.Installations[installationIndex] = installation
	if err := service.StateStore.Save(state); err != nil {
		return result, fmt.Errorf("reconcile Agent Plugins state after legacy removal: %w", err)
	}
	result.Mutated = true
	return result, nil
}

func materializedTargets(installation domain.Installation) []string {
	values := make([]string, 0, len(installation.Clients))
	for _, client := range installation.Clients {
		if client.Materialization != domain.MaterializationAbsent || len(client.NativeObjects) > 0 {
			values = append(values, client.ClientID+"/"+client.Scope)
		}
	}
	sort.Strings(values)
	return values
}
