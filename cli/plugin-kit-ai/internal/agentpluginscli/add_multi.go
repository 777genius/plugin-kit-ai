package agentpluginscli

import (
	"context"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	clientplanner "github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/planner"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/transaction"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/usecase"
	"github.com/spf13/cobra"
)

type addTargetResult struct {
	Target     string        `json:"target"`
	Status     string        `json:"status"`
	NextAction string        `json:"next_action,omitempty"`
	Output     addResultData `json:"output"`
}

type addMultiResult struct {
	OperationID string            `json:"operation_id,omitempty"`
	Batch       bool              `json:"batch"`
	Status      string            `json:"status"`
	Succeeded   int               `json:"succeeded"`
	Failed      int               `json:"failed"`
	Plugin      string            `json:"plugin"`
	Version     string            `json:"version,omitempty"`
	Source      string            `json:"source"`
	Revision    string            `json:"revision,omitempty"`
	TreeDigest  string            `json:"tree_digest,omitempty"`
	DryRun      bool              `json:"dry_run"`
	Targets     []addTargetResult `json:"targets"`
}

// runAddMany is deliberately not implemented as repeated CLI invocations. It
// resolves and validates one package, detects clients once, builds every plan,
// and only begins applying after the complete selected set passes preflight.
func runAddMany(ctx context.Context, cmd *cobra.Command, app App, opts *options, source string, targets []domain.ClientID, activationComplete, authComplete bool) error {
	return runAddManyWithClients(ctx, cmd, app, opts, source, targets, activationComplete, authComplete, nil)
}

func runAddManyWithClients(ctx context.Context, cmd *cobra.Command, app App, opts *options, source string, targets []domain.ClientID, activationComplete, authComplete bool, clients []domain.DetectedClient) error {
	writeProgress(app, opts.format, "Resolving and validating one Agent Plugin package for every selected target...")
	loaded, err := app.loadPackage(ctx, source)
	if err != nil {
		return err
	}
	if loaded.cleanup != nil {
		defer loaded.cleanup()
	}
	return runAddManyLoaded(ctx, cmd, app, opts, loaded, targets, activationComplete, authComplete, clients)
}

func runAddManyLoaded(ctx context.Context, cmd *cobra.Command, app App, opts *options, loaded loadedPackage, targets []domain.ClientID, activationComplete, authComplete bool, clients []domain.DetectedClient) error {
	combined := addMultiResult{
		Batch: true, Status: "planned", Plugin: loaded.envelope.Manifest.Name,
		Version: loaded.envelope.Manifest.Version, Source: publicPackageSource(loaded.envelope.Source),
		Revision: loaded.envelope.Source.ResolvedRevision, TreeDigest: loaded.envelope.TreeDigest,
		DryRun: opts.dryRun, Targets: make([]addTargetResult, 0, len(targets)),
	}
	if clients == nil {
		detectedClients, err := app.Detector.Detect(ctx)
		if err != nil {
			return fmt.Errorf("detect AI clients: %w", err)
		}
		clients = detectedClients
	}
	detected := make(map[domain.ClientID]domain.DetectedClient, len(clients)+1)
	for _, client := range clients {
		detected[client.ClientID] = client
	}
	selected := make([]domain.DetectedClient, 0, len(targets))
	missingTarget := domain.ClientID("")
	for _, target := range targets {
		client, ok := detected[target]
		if target == domain.ClientChatGPT && !ok {
			client = domain.DetectedClient{ClientID: target, DisplayName: "ChatGPT", Status: domain.DetectionNotDetected}
			detected[target] = client
			ok = true
		}
		if !ok || (client.Status != domain.DetectionDetected && target != domain.ClientChatGPT) {
			missingTarget = target
			combined.Failed++
			combined.Targets = append(combined.Targets, addTargetResult{Target: string(target), Status: "not_detected", Output: newAddResultData(loaded.envelope, usecase.AddResult{}, true)})
			continue
		}
		combined.Targets = append(combined.Targets, addTargetResult{Target: string(target), Status: "detected", Output: newAddResultData(loaded.envelope, usecase.AddResult{}, true)})
		selected = append(selected, client)
	}
	if missingTarget != "" {
		combined.Status = "preflight_failed"
		_ = renderAddMultiResult(cmd, opts, combined, loaded.envelope)
		return fmt.Errorf("target %q was not detected; no target was changed", missingTarget)
	}
	combined.Targets = combined.Targets[:0]
	planner := clientplanner.Planner{ManagedRoot: app.ManagedRoot, Detected: detected}
	service := usecase.Service{
		StateStore: app.StateStore, Planner: planner, Targets: planner,
		Stager: app.Stager, Activator: app.Activator, Lock: app.MutationLock,
		Kernel: transaction.Kernel{StateStore: app.StateStore, Directory: app.Directory},
	}
	inputs := make([]usecase.AddInput, len(selected))
	plannedInstallationID := ""
	for index, client := range selected {
		clientPackage := cloneLoadedPackage(loaded)
		if err := prepareLoadedPackageForClient(&clientPackage, client.ClientID); err != nil {
			return fmt.Errorf("preflight target %s: %w; no target was changed", client.ClientID, err)
		}
		input := usecase.AddInput{
			Envelope: clientPackage.envelope, Client: client, Scope: domain.ScopeUser,
			DryRun: true, Interactive: false, Hints: clientPackage.hints,
			BackendExecutable:  backendExecutable(client, detected),
			ActivationComplete: activationComplete, AuthComplete: authComplete,
			PersistAuthoritativeObservations: false,
			InstallationID:                   plannedInstallationID,
		}
		planned, planErr := service.Add(ctx, input)
		if plannedInstallationID == "" {
			plannedInstallationID = planned.InstallationID
		}
		entry := addTargetResult{Target: string(client.ClientID), Status: string(planned.Plan.Status), Output: newAddResultData(clientPackage.envelope, planned, true), NextAction: nextLifecycleAction(planned)}
		combined.Targets = append(combined.Targets, entry)
		if planErr != nil || planned.Plan.Status == domain.PlanUnsupported {
			combined.Status = "preflight_failed"
			combined.Failed++
			_ = renderAddMultiResult(cmd, opts, combined, loaded.envelope)
			if planErr != nil {
				return fmt.Errorf("preflight target %s: %w; no target was changed", client.ClientID, planErr)
			}
			return fmt.Errorf("preflight target %s is unsupported; no target was changed", client.ClientID)
		}
		combined.Succeeded++
		input.DryRun = opts.dryRun
		inputs[index] = input
	}
	if opts.dryRun {
		return renderAddMultiResult(cmd, opts, combined, loaded.envelope)
	}
	if opts.format == "human" {
		if err := renderAddMultiResult(cmd, opts, combined, loaded.envelope); err != nil {
			return err
		}
	}

	writeProgress(app, opts.format, "Applying the completely preflighted multi-target plan...")
	operationID, err := newOperationGroupID()
	if err != nil {
		return err
	}
	combined.Status = "completed"
	combined.OperationID = operationID
	combined.Succeeded = 0
	combined.Failed = 0
	combined.Targets = combined.Targets[:0]
	installationID := plannedInstallationID
	for index := range inputs {
		inputs[index].Confirmed = true
		inputs[index].InstallationID = installationID
	}
	if app.GroupLifecycle != nil {
		applied, groupErr := app.GroupLifecycle.AddGroup(ctx, AddGroupInput{OperationID: operationID, Inputs: inputs})
		if len(applied) > len(inputs) || len(applied) != len(inputs) && groupErr == nil {
			return fmt.Errorf("install engine returned %d add results for %d targets", len(applied), len(inputs))
		}
		for index, result := range applied {
			entry := addTargetResult{Target: string(selected[index].ClientID), Status: string(result.Plan.Status), Output: newAddResultData(inputs[index].Envelope, result, false), NextAction: nextLifecycleAction(result)}
			combined.Targets = append(combined.Targets, entry)
			combined.Succeeded++
		}
		if groupErr != nil {
			for index := len(applied); index < len(inputs); index++ {
				combined.Targets = append(combined.Targets, addTargetResult{Target: string(selected[index].ClientID), Status: "rolled_back", Output: newAddResultData(inputs[index].Envelope, usecase.AddResult{}, false)})
			}
			combined.Status = "group_rolled_back"
			combined.Failed = len(inputs)
			combined.Succeeded = 0
			_ = renderAddMultiResult(cmd, opts, combined, loaded.envelope)
			return groupErr
		}
		return renderAddMultiResult(cmd, opts, combined, loaded.envelope)
	}
	for index, input := range inputs {
		result, applyErr := service.Add(ctx, input)
		if installationID == "" {
			installationID = result.InstallationID
		}
		entry := addTargetResult{Target: string(selected[index].ClientID), Status: string(result.Plan.Status), Output: newAddResultData(input.Envelope, result, false), NextAction: nextLifecycleAction(result)}
		combined.Targets = append(combined.Targets, entry)
		if applyErr != nil {
			combined.Status = "partial_failure"
			combined.Failed++
			_ = renderAddMultiResult(cmd, opts, combined, loaded.envelope)
			return fmt.Errorf("apply target %s failed after complete preflight: %w", selected[index].ClientID, applyErr)
		}
		combined.Succeeded++
	}
	return renderAddMultiResult(cmd, opts, combined, loaded.envelope)
}

func publicPackageSource(source domain.SourceIdentity) string {
	if source.Repository != "" {
		value := source.Repository
		if source.PackageSubpath != "" {
			value += "//" + source.PackageSubpath
		}
		return value
	}
	if source.ResolvedRevision != "" {
		return "direct immutable source"
	}
	return "direct local source"
}

func cloneLoadedPackage(source loadedPackage) loadedPackage {
	clone := source
	clone.cleanup = nil
	clone.envelope.Inventory.Skills = append([]string(nil), source.envelope.Inventory.Skills...)
	clone.envelope.Inventory.MCPServers = append([]string(nil), source.envelope.Inventory.MCPServers...)
	clone.envelope.Inventory.AppBindings = append([]string(nil), source.envelope.Inventory.AppBindings...)
	clone.envelope.App.Bindings = make(map[string]domain.AppBinding, len(source.envelope.App.Bindings))
	for key, value := range source.envelope.App.Bindings {
		clone.envelope.App.Bindings[key] = value
	}
	return clone
}

func renderAddMultiResult(cmd *cobra.Command, opts *options, result addMultiResult, envelope domain.PackageEnvelope) error {
	if opts.format == "json" {
		overall := "success"
		if result.Failed > 0 {
			overall = "failure"
		}
		return writeJSONResult(cmd.OutOrStdout(), "add", overall, result)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Plugin: %s %s\n", result.Plugin, result.Version)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Targets: %s\n", addResultTargets(result.Targets))
	for _, target := range result.Targets {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", target.Target, target.Status)
		if target.NextAction != "" && !fullyInstalled(target.Output.Result.Activation) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    Next: %s\n", target.NextAction)
		}
	}
	if result.DryRun {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No changes made (dry run).")
	}
	return nil
}

func addResultTargets(results []addTargetResult) string {
	values := make([]string, len(results))
	for index, result := range results {
		values[index] = result.Target
	}
	return strings.Join(values, ",")
}
