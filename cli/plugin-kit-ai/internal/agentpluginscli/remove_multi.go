package agentpluginscli

import (
	"context"
	"fmt"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/usecase"
	"github.com/spf13/cobra"
)

type removeTargetResult struct {
	Target string           `json:"target"`
	Status string           `json:"status"`
	Output removeResultData `json:"output"`
}

type removeMultiResult struct {
	OperationID         string               `json:"operation_id,omitempty"`
	Batch               bool                 `json:"batch"`
	Status              string               `json:"status"`
	Succeeded           int                  `json:"succeeded"`
	Failed              int                  `json:"failed"`
	Plugin              string               `json:"plugin"`
	PluginDataPreserved bool                 `json:"plugin_data_preserved"`
	DataRetained        bool                 `json:"data_retained"`
	RetainedData        []string             `json:"retained_data,omitempty"`
	DryRun              bool                 `json:"dry_run"`
	Targets             []removeTargetResult `json:"targets"`
}

func runRemoveMany(ctx context.Context, cmd *cobra.Command, app App, opts *options, selector string, targets []domain.ClientID) error {
	state, err := app.StateStore.Load()
	if err != nil {
		return err
	}
	installation, err := selectInstallation(state, selector)
	if err != nil {
		return err
	}
	if installation.Package.LoaderKind == domain.LoaderKindLegacy {
		return fmt.Errorf("legacy removal is integration-wide; use --target legacy-all")
	}
	clients, err := app.Detector.Detect(ctx)
	if err != nil {
		return fmt.Errorf("detect AI clients: %w", err)
	}
	detected := make(map[domain.ClientID]domain.DetectedClient, len(clients)+1)
	for _, client := range clients {
		detected[client.ClientID] = client
	}
	service := lifecycleService(app, detected)
	inputs := make([]usecase.RemoveInput, 0, len(targets))
	selected := make([]domain.DetectedClient, 0, len(targets))
	result := removeMultiResult{Batch: true, Status: "planned", Plugin: installation.DeclaredName, PluginDataPreserved: !opts.purgeData, DryRun: opts.dryRun, Targets: make([]removeTargetResult, 0, len(targets))}
	for _, target := range targets {
		if !installationHasTarget(installation, target, opts.scope) {
			return fmt.Errorf("plugin is not installed for target %q in %s scope; no target was changed", target, opts.scope)
		}
		client, ok := detected[target]
		if !ok {
			client = domain.DetectedClient{ClientID: target, DisplayName: string(target), Status: domain.DetectionNotDetected}
			detected[target] = client
		}
		input := usecase.RemoveInput{
			Selector: selector, Client: client, Scope: domain.ScopeUser,
			DryRun: true, Interactive: false, ExternalUninstalled: opts.externalUninstalled,
			BackendExecutable: backendExecutable(client, detected),
		}
		inputs = append(inputs, input)
		selected = append(selected, client)
	}
	operationID, err := newOperationGroupID()
	if err != nil {
		return err
	}
	result.OperationID = operationID
	plannedGroup, planErr := service.RemoveGroup(ctx, usecase.RemoveGroupInput{Selector: selector, Targets: inputs, OperationGroupID: operationID, DryRun: true, PurgeData: opts.purgeData})
	for index, planned := range plannedGroup.Targets {
		status := "planned"
		if !planned.Deactivation.ArtifactRemovalAllowed {
			status = "blocked"
		}
		output := newRemoveResultData(installation, planned, true)
		output.OperationID = operationID
		result.Targets = append(result.Targets, removeTargetResult{Target: string(selected[index].ClientID), Status: status, Output: output})
	}
	result.Succeeded = len(plannedGroup.Targets)
	if planErr != nil {
		result.Status, result.Failed, result.Succeeded = "preflight_failed", len(inputs), 0
		_ = renderRemoveMultiResult(cmd, opts, result)
		return fmt.Errorf("group remove preflight failed; no target was changed: %w", planErr)
	}
	if opts.dryRun {
		return renderRemoveMultiResult(cmd, opts, result)
	}
	result.Status = "applying"
	result.Succeeded = 0
	result.Failed = 0
	result.Targets = result.Targets[:0]
	for index := range inputs {
		inputs[index].Confirmed = true
	}
	appliedGroup, groupErr := service.RemoveGroup(ctx, usecase.RemoveGroupInput{Selector: selector, Targets: inputs, OperationGroupID: operationID, Confirmed: true, PurgeData: opts.purgeData})
	appliedResults := appliedGroup.Targets
	if len(appliedResults) > len(inputs) || len(appliedResults) != len(inputs) && groupErr == nil {
		return fmt.Errorf("install engine returned %d remove results for %d targets", len(appliedResults), len(inputs))
	}
	for index, applied := range appliedResults {
		output := newRemoveResultData(installation, applied, false)
		output.OperationID = operationID
		status := string(applied.GroupPhase)
		if status == "" {
			status = "apply_failed"
		}
		result.Targets = append(result.Targets, removeTargetResult{Target: string(selected[index].ClientID), Status: status, Output: output})
		if applied.GroupPhase == usecase.GroupTargetExternalCompleted {
			result.Succeeded++
		}
	}
	if groupErr != nil {
		result.Status, result.Failed = groupFailureStatus(appliedGroup.Phase), len(inputs)-result.Succeeded
		_ = renderRemoveMultiResult(cmd, opts, result)
		return groupErr
	}
	result.Status = string(appliedGroup.Phase)
	if after, loadErr := app.StateStore.Load(); loadErr == nil {
		if retained, ok := locallyMatchedInstallation(after, installation.InstallationID); ok && retained.DataRetained {
			result.DataRetained = true
			result.Status = "data_retained"
			for _, receipt := range retained.DataReceipts {
				result.RetainedData = append(result.RetainedData, receipt.Locator)
			}
		}
	}
	return renderRemoveMultiResult(cmd, opts, result)
}

func renderRemoveMultiResult(cmd *cobra.Command, opts *options, result removeMultiResult) error {
	if opts.format == "json" {
		overall := "success"
		if result.Failed > 0 {
			overall = "failure"
		}
		return writeJSONResult(cmd.OutOrStdout(), "remove", overall, result)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Remove %s: %s\n", result.Plugin, result.Status); err != nil {
		return err
	}
	for _, target := range result.Targets {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", target.Target, target.Status); err != nil {
			return err
		}
	}
	if result.PluginDataPreserved {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "  PLUGIN_DATA: preserved"); err != nil {
			return err
		}
	}
	return nil
}
