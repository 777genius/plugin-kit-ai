package agentpluginscli

import (
	"context"
	"fmt"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/usecase"
	"github.com/spf13/cobra"
)

type repairTargetResult struct {
	Target     string           `json:"target"`
	Status     string           `json:"status"`
	NextAction string           `json:"next_action,omitempty"`
	Output     repairResultData `json:"output"`
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
	var recordedSequence uint64
	var directSource string
	for index, target := range targets {
		binding, err := selectedRepairBinding(installation, target, domain.ScopeUser)
		if err != nil {
			return fmt.Errorf("preflight target %s: %w; no target was changed", target, err)
		}
		if binding.PackageRevision == nil {
			return fmt.Errorf("repair target %s has no recorded package revision", target)
		}
		if installation.OriginMode == domain.OriginModeDirectory {
			if index == 0 {
				recordedSequence = binding.PackageRevision.ReleaseSequence
			}
			if binding.PackageRevision.ReleaseSequence != recordedSequence {
				return fmt.Errorf("grouped repair requires one recorded immutable release; run update to converge mixed revisions")
			}
		} else {
			source, err := repairSource(installation, binding)
			if err != nil {
				return err
			}
			if index == 0 {
				directSource = source
			}
			if source != directSource {
				return fmt.Errorf("grouped repair requires one recorded immutable package")
			}
		}
	}
	writeProgress(app, opts.format, "Resolving and validating one exact installed package revision...")
	var loaded loadedPackage
	if installation.OriginMode == domain.OriginModeDirectory {
		loaded, err = app.loadInstalledPackage(ctx, installation, targets, domain.DirectoryRepair, recordedSequence)
	} else {
		loaded, err = app.loadPackageFor(ctx, directSource, packageResolutionRequest{Targets: targets, Operation: domain.DirectoryRepair})
	}
	if err != nil {
		return err
	}
	if loaded.cleanup != nil {
		defer loaded.cleanup()
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
	inputs := make([]usecase.AddInput, 0, len(targets))
	result := repairMultiResult{Batch: true, Status: "planned", Plugin: installation.DeclaredName, DryRun: opts.dryRun, Targets: make([]repairTargetResult, 0, len(targets))}
	for _, target := range targets {
		clientPackage := cloneLoadedPackage(loaded)
		if err := prepareLoadedPackageForClient(&clientPackage, target); err != nil {
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
			Envelope: clientPackage.envelope, Client: client, Scope: domain.ScopeUser,
			Interactive: false, Hints: clientPackage.hints,
			InstallationID:    installation.InstallationID,
			BackendExecutable: backendExecutable(client, detected),
			OriginMode:        loaded.origin, DirectoryResolution: cloneDirectoryOrigin(loaded.directory),
			DistributionSuspended: loaded.distributionSuspended, ReleaseRevoked: loaded.releaseRevoked,
		}
		inputs = append(inputs, input)
	}
	operationID, err := newOperationGroupID()
	if err != nil {
		return err
	}
	result.OperationID = operationID
	planned, err := service.RepairGroup(ctx, usecase.GroupInput{Targets: inputs, OperationGroupID: operationID, DryRun: true, Repair: true})
	for index, targetResult := range planned.Targets {
		output := newRepairResultData(installation, targetResult, true)
		output.OperationID = operationID
		result.Targets = append(result.Targets, repairTargetResult{Target: string(targets[index]), Status: string(targetResult.Plan.Status), NextAction: output.NextAction, Output: output})
	}
	result.Succeeded = len(planned.Targets)
	if err != nil {
		result.Status, result.Failed, result.Succeeded = "preflight_failed", len(inputs), 0
		_ = renderRepairMultiResult(cmd, opts, result)
		return fmt.Errorf("group repair preflight failed; no target was changed: %w", err)
	}
	if opts.dryRun {
		return renderRepairMultiResult(cmd, opts, result)
	}
	result.Status = "completed"
	result.Succeeded = 0
	result.Failed = 0
	result.Targets = result.Targets[:0]
	appliedGroup, groupErr := service.RepairGroup(ctx, usecase.GroupInput{Targets: inputs, OperationGroupID: operationID, Confirmed: true, Repair: true})
	appliedResults := appliedGroup.Targets
	if len(appliedResults) > len(inputs) || len(appliedResults) != len(inputs) && groupErr == nil {
		return fmt.Errorf("install engine returned %d repair results for %d targets", len(appliedResults), len(inputs))
	}
	for index, applied := range appliedResults {
		output := newRepairResultData(installation, applied, false)
		output.OperationID = operationID
		result.Targets = append(result.Targets, repairTargetResult{Target: string(targets[index]), Status: string(applied.Plan.Status), NextAction: output.NextAction, Output: output})
		result.Succeeded++
	}
	if groupErr != nil {
		result.Status, result.Failed, result.Succeeded = "group_rolled_back", len(inputs), 0
		_ = renderRepairMultiResult(cmd, opts, result)
		return groupErr
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
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Repair %s: %s\n", result.Plugin, result.Status); err != nil {
		return err
	}
	for _, target := range result.Targets {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", target.Target, target.Status); err != nil {
			return err
		}
		if target.NextAction != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "    Next: %s\n", target.NextAction); err != nil {
				return err
			}
		}
	}
	if result.Status == "completed" {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Managed package repaired from the exact installed revision and its package digest verified; external client activation and authentication were not reverified."); err != nil {
			return err
		}
	}
	return nil
}
