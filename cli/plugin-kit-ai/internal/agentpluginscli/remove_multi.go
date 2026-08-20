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
	OperationID string               `json:"operation_id,omitempty"`
	Batch       bool                 `json:"batch"`
	Status      string               `json:"status"`
	Succeeded   int                  `json:"succeeded"`
	Failed      int                  `json:"failed"`
	Plugin      string               `json:"plugin"`
	DryRun      bool                 `json:"dry_run"`
	Targets     []removeTargetResult `json:"targets"`
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
	result := removeMultiResult{Batch: true, Status: "planned", Plugin: installation.DeclaredName, DryRun: opts.dryRun, Targets: make([]removeTargetResult, 0, len(targets))}
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
		planned, planErr := service.Remove(ctx, input)
		status := "planned"
		if !planned.Deactivation.ArtifactRemovalAllowed {
			status = "blocked"
		}
		result.Targets = append(result.Targets, removeTargetResult{Target: string(target), Status: status, Output: newRemoveResultData(installation, planned, true)})
		if planErr != nil || !planned.Deactivation.ArtifactRemovalAllowed {
			result.Status = "preflight_failed"
			result.Failed++
			_ = renderRemoveMultiResult(cmd, opts, result)
			if planErr != nil {
				return fmt.Errorf("preflight target %s: %w; no target was changed", target, planErr)
			}
			return fmt.Errorf("preflight target %s blocks removal; no target was changed", target)
		}
		result.Succeeded++
		input.DryRun = opts.dryRun
		inputs = append(inputs, input)
		selected = append(selected, client)
	}
	if opts.dryRun {
		return renderRemoveMultiResult(cmd, opts, result)
	}
	if opts.format == "human" {
		if err := renderRemoveMultiResult(cmd, opts, result); err != nil {
			return err
		}
	}
	result.Status = "completed"
	operationID, err := newOperationGroupID()
	if err != nil {
		return err
	}
	result.OperationID = operationID
	result.Succeeded = 0
	result.Failed = 0
	result.Targets = result.Targets[:0]
	for index := range inputs {
		inputs[index].Confirmed = true
	}
	if app.GroupLifecycle != nil {
		appliedResults, groupErr := app.GroupLifecycle.RemoveGroup(ctx, RemoveGroupInput{OperationID: operationID, Inputs: inputs})
		if len(appliedResults) > len(inputs) || len(appliedResults) != len(inputs) && groupErr == nil {
			return fmt.Errorf("install engine returned %d remove results for %d targets", len(appliedResults), len(inputs))
		}
		for index, applied := range appliedResults {
			result.Targets = append(result.Targets, removeTargetResult{Target: string(selected[index].ClientID), Status: "removed", Output: newRemoveResultData(installation, applied, false)})
			result.Succeeded++
		}
		if groupErr != nil {
			for index := len(appliedResults); index < len(inputs); index++ {
				result.Targets = append(result.Targets, removeTargetResult{Target: string(selected[index].ClientID), Status: "rolled_back", Output: newRemoveResultData(installation, usecase.RemoveResult{}, false)})
			}
			result.Status = "group_rolled_back"
			result.Failed = len(inputs)
			result.Succeeded = 0
			_ = renderRemoveMultiResult(cmd, opts, result)
			return groupErr
		}
		return renderRemoveMultiResult(cmd, opts, result)
	}
	for index, input := range inputs {
		applied, applyErr := service.Remove(ctx, input)
		status := "removed"
		if applyErr != nil {
			status = "failed"
		}
		result.Targets = append(result.Targets, removeTargetResult{Target: string(selected[index].ClientID), Status: status, Output: newRemoveResultData(installation, applied, false)})
		if applyErr != nil {
			result.Status = "partial_failure"
			result.Failed++
			_ = renderRemoveMultiResult(cmd, opts, result)
			return fmt.Errorf("apply target %s failed after complete preflight: %w", selected[index].ClientID, applyErr)
		}
		result.Succeeded++
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
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Remove %s: %s\n", result.Plugin, result.Status)
	for _, target := range result.Targets {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", target.Target, target.Status)
	}
	return nil
}
