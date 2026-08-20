package agentpluginscli

import (
	"context"
	"fmt"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/usecase"
	"github.com/spf13/cobra"
)

type repairTargetResult struct {
	Target string           `json:"target"`
	Status string           `json:"status"`
	Output repairResultData `json:"output"`
}

type repairMultiResult struct {
	OperationID string               `json:"operation_id,omitempty"`
	Batch       bool                 `json:"batch"`
	Status      string               `json:"status"`
	Succeeded   int                  `json:"succeeded"`
	Failed      int                  `json:"failed"`
	Plugin      string               `json:"plugin"`
	DryRun      bool                 `json:"dry_run"`
	Targets     []repairTargetResult `json:"targets"`
}

func runRepairMany(ctx context.Context, cmd *cobra.Command, app App, opts *options, selector string, targets []domain.ClientID) error {
	state, err := app.StateStore.Load()
	if err != nil {
		return err
	}
	installation, err := selectInstallation(state, selector)
	if err != nil {
		return err
	}
	if installation.NeedsRebind || installation.Package.LoaderKind != domain.LoaderKindAgentPlugins {
		return fmt.Errorf("repair requires a bound Agent Plugins installation")
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
	packages := make(map[string]loadedPackage)
	defer func() {
		for _, loaded := range packages {
			if loaded.cleanup != nil {
				_ = loaded.cleanup()
			}
		}
	}()
	inputs := make([]usecase.AddInput, 0, len(targets))
	result := repairMultiResult{Batch: true, Status: "planned", Plugin: installation.DeclaredName, DryRun: opts.dryRun, Targets: make([]repairTargetResult, 0, len(targets))}
	for _, target := range targets {
		binding, err := selectedRepairBinding(installation, target, domain.ScopeUser)
		if err != nil {
			return fmt.Errorf("preflight target %s: %w; no target was changed", target, err)
		}
		source, err := repairSource(installation, binding)
		if err != nil {
			return fmt.Errorf("preflight target %s: %w; no target was changed", target, err)
		}
		base, ok := packages[source]
		if !ok {
			writeProgress(app, opts.format, "Resolving and validating one exact installed package revision...")
			base, err = app.loadPackage(ctx, source)
			if err != nil {
				return err
			}
			packages[source] = base
		}
		loaded := cloneLoadedPackage(base)
		restoreCatalogEvidence(&loaded, binding)
		if err := prepareLoadedPackageForClient(&loaded, target); err != nil {
			return fmt.Errorf("preflight target %s: %w; no target was changed", target, err)
		}
		client, ok := detected[target]
		if target == domain.ClientChatGPT && !ok {
			client = domain.DetectedClient{ClientID: target, DisplayName: "ChatGPT", Status: domain.DetectionNotDetected}
			detected[target] = client
			ok = true
		}
		if !ok || (client.Status != domain.DetectionDetected && target != domain.ClientChatGPT) {
			return fmt.Errorf("target %q was not detected; no target was changed", target)
		}
		input := usecase.AddInput{
			Envelope: loaded.envelope, Client: client, Scope: domain.ScopeUser,
			DryRun: true, Interactive: false, Hints: loaded.hints,
			InstallationID:    installation.InstallationID,
			BackendExecutable: backendExecutable(client, detected),
		}
		planned, planErr := service.Repair(ctx, input)
		result.Targets = append(result.Targets, repairTargetResult{Target: string(target), Status: string(planned.Plan.Status), Output: newRepairResultData(installation, planned, true)})
		if planErr != nil || planned.Plan.Status == domain.PlanUnsupported {
			result.Status = "preflight_failed"
			result.Failed++
			_ = renderRepairMultiResult(cmd, opts, result)
			if planErr != nil {
				return fmt.Errorf("preflight target %s: %w; no target was changed", target, planErr)
			}
			return fmt.Errorf("preflight target %s is unsupported; no target was changed", target)
		}
		result.Succeeded++
		input.DryRun = opts.dryRun
		inputs = append(inputs, input)
	}
	if opts.dryRun {
		return renderRepairMultiResult(cmd, opts, result)
	}
	if opts.format == "human" {
		if err := renderRepairMultiResult(cmd, opts, result); err != nil {
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
		appliedResults, groupErr := app.GroupLifecycle.RepairGroup(ctx, AddGroupInput{OperationID: operationID, Inputs: inputs})
		if len(appliedResults) > len(inputs) || len(appliedResults) != len(inputs) && groupErr == nil {
			return fmt.Errorf("install engine returned %d repair results for %d targets", len(appliedResults), len(inputs))
		}
		for index, applied := range appliedResults {
			result.Targets = append(result.Targets, repairTargetResult{Target: string(targets[index]), Status: string(applied.Plan.Status), Output: newRepairResultData(installation, applied, false)})
			result.Succeeded++
		}
		if groupErr != nil {
			for index := len(appliedResults); index < len(inputs); index++ {
				result.Targets = append(result.Targets, repairTargetResult{Target: string(targets[index]), Status: "rolled_back", Output: newRepairResultData(installation, usecase.AddResult{}, false)})
			}
			result.Status = "group_rolled_back"
			result.Failed = len(inputs)
			result.Succeeded = 0
			_ = renderRepairMultiResult(cmd, opts, result)
			return groupErr
		}
		return renderRepairMultiResult(cmd, opts, result)
	}
	for index, input := range inputs {
		applied, applyErr := service.Repair(ctx, input)
		result.Targets = append(result.Targets, repairTargetResult{Target: string(targets[index]), Status: string(applied.Plan.Status), Output: newRepairResultData(installation, applied, false)})
		if applyErr != nil {
			result.Status = "partial_failure"
			result.Failed++
			_ = renderRepairMultiResult(cmd, opts, result)
			return fmt.Errorf("apply target %s failed after complete preflight: %w", targets[index], applyErr)
		}
		result.Succeeded++
	}
	return renderRepairMultiResult(cmd, opts, result)
}

func renderRepairMultiResult(cmd *cobra.Command, opts *options, result repairMultiResult) error {
	if opts.format == "json" {
		overall := "success"
		if result.Failed > 0 {
			overall = "failure"
		}
		return writeJSONResult(cmd.OutOrStdout(), "repair", overall, result)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Repair %s: %s\n", result.Plugin, result.Status)
	for _, target := range result.Targets {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", target.Target, target.Status)
	}
	return nil
}
