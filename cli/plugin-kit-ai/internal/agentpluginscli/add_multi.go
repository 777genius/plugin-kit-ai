package agentpluginscli

import (
	"context"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
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
	OperationID    string                  `json:"operation_id,omitempty"`
	Batch          bool                    `json:"batch"`
	Status         string                  `json:"status"`
	Succeeded      int                     `json:"succeeded"`
	Failed         int                     `json:"failed"`
	Plugin         string                  `json:"plugin"`
	Version        string                  `json:"version,omitempty"`
	Source         string                  `json:"source"`
	Revision       string                  `json:"revision,omitempty"`
	TreeDigest     string                  `json:"tree_digest,omitempty"`
	ManifestDigest string                  `json:"manifest_digest,omitempty"`
	Directory      *domain.DirectoryOrigin `json:"directory,omitempty"`
	DryRun         bool                    `json:"dry_run"`
	Targets        []addTargetResult       `json:"targets"`
}

// runAddMany is deliberately not implemented as repeated CLI invocations. It
// resolves and validates one package, detects clients once, builds every plan,
// and only begins applying after the complete selected set passes preflight.
func runAddMany(ctx context.Context, cmd *cobra.Command, app App, opts *options, source string, targets []domain.ClientID, activationComplete, authComplete bool) error {
	return runAddManyWithClients(ctx, cmd, app, opts, source, targets, activationComplete, authComplete, nil)
}

func runAddManyWithClients(ctx context.Context, cmd *cobra.Command, app App, opts *options, source string, targets []domain.ClientID, activationComplete, authComplete bool, clients []domain.DetectedClient) error {
	writeProgress(app, opts.format, "Resolving and validating one Agent Plugin package for every selected target...")
	loaded, err := app.loadPackageFor(ctx, source, app.addResolutionRequest(source, targets))
	if err != nil {
		return err
	}
	if loaded.cleanup != nil {
		defer loaded.cleanup()
	}
	return runAddManyLoaded(ctx, cmd, app, opts, loaded, targets, activationComplete, authComplete, clients)
}

func runAddManyLoaded(ctx context.Context, cmd *cobra.Command, app App, opts *options, loaded loadedPackage, targets []domain.ClientID, activationComplete, authComplete bool, clients []domain.DetectedClient) error {
	if len(targets) == 1 {
		if activationComplete || authComplete {
			return runAddLoaded(ctx, cmd, app, opts, loaded, activationComplete, authComplete, clients)
		}
		if state, err := app.StateStore.Load(); err == nil {
			if installation, ok := locallyMatchedInstallation(state, loaded.envelope.Manifest.Name); ok && installationHasTarget(installation, targets[0], string(domain.ScopeUser)) {
				return runAddLoaded(ctx, cmd, app, opts, loaded, false, false, clients)
			}
		}
	}
	combined := addMultiResult{
		Batch: true, Status: "planned", Plugin: loaded.envelope.Manifest.Name,
		Version: loaded.envelope.Manifest.Version, Source: publicPackageSource(loaded.envelope.Source),
		Revision: loaded.envelope.Source.ResolvedRevision, TreeDigest: loaded.envelope.TreeDigest, ManifestDigest: loaded.envelope.ManifestDigest,
		Directory: cloneDirectoryOrigin(loaded.directory),
		DryRun:    opts.dryRun, Targets: make([]addTargetResult, 0, len(targets)),
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
	service := lifecycleService(app, detected)
	inputs := make([]usecase.AddInput, len(selected))
	for index, client := range selected {
		clientPackage := cloneLoadedPackage(loaded)
		if err := prepareLoadedPackageForClient(&clientPackage, client.ClientID); err != nil {
			return fmt.Errorf("preflight target %s: %w; no target was changed", client.ClientID, err)
		}
		input := usecase.AddInput{
			Envelope: clientPackage.envelope, Client: client, Scope: domain.ScopeUser,
			DryRun: true, Interactive: app.Terminal, Hints: clientPackage.hints,
			BackendExecutable:  backendExecutable(client, detected),
			ActivationComplete: activationComplete, AuthComplete: authComplete,
			PersistAuthoritativeObservations: false,
			OriginMode:                       loaded.origin, DirectoryResolution: cloneDirectoryOrigin(loaded.directory),
			DistributionSuspended: loaded.distributionSuspended, ReleaseRevoked: loaded.releaseRevoked,
		}
		inputs[index] = input
	}
	operationID, err := newOperationGroupID()
	if err != nil {
		return err
	}
	combined.OperationID = operationID
	groupInput := usecase.GroupInput{Targets: inputs, OperationGroupID: operationID, DryRun: true}
	planned, err := service.AddGroup(ctx, groupInput)
	combined.Targets = combined.Targets[:0]
	for index, result := range planned.Targets {
		output := newAddResultData(inputs[index].Envelope, result, true)
		output.OperationID = operationID
		combined.Targets = append(combined.Targets, addTargetResult{Target: string(selected[index].ClientID), Status: string(result.Plan.Status), Output: output, NextAction: nextLifecycleAction(result)})
	}
	combined.Succeeded = len(planned.Targets)
	if err != nil {
		combined.Status, combined.Failed, combined.Succeeded = "preflight_failed", len(inputs), 0
		_ = renderAddMultiResult(cmd, opts, combined, loaded.envelope)
		return fmt.Errorf("group preflight failed; no target was changed: %w%s", err, addGroupNextAction(combined.Targets))
	}
	if opts.dryRun {
		return renderAddMultiResult(cmd, opts, combined, loaded.envelope)
	}
	writeProgress(app, opts.format, "Applying the completely preflighted multi-target plan...")
	groupInput.DryRun, groupInput.Confirmed = false, true
	applied, err := service.AddGroup(ctx, groupInput)
	combined.Status, combined.Targets, combined.Succeeded = "completed", combined.Targets[:0], 0
	for index, result := range applied.Targets {
		output := newAddResultData(inputs[index].Envelope, result, false)
		output.OperationID = operationID
		combined.Targets = append(combined.Targets, addTargetResult{Target: string(selected[index].ClientID), Status: string(result.Plan.Status), Output: output, NextAction: nextLifecycleAction(result)})
		combined.Succeeded++
	}
	if err != nil {
		combined.Status, combined.Failed, combined.Succeeded = "group_rolled_back", len(inputs), 0
		_ = renderAddMultiResult(cmd, opts, combined, loaded.envelope)
		return err
	}
	if err := renderAddMultiResult(cmd, opts, combined, loaded.envelope); err != nil {
		return err
	}
	if app.Terminal && opts.format == "human" && len(applied.Targets) == 1 {
		input := inputs[0]
		input.DryRun = false
		input.InstallationID = applied.Targets[0].InstallationID
		return resumeInteractiveLifecycle(ctx, cmd, service, input, inputs[0].Envelope, applied.Targets[0])
	}
	return nil
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
	if len(result.Targets) == 1 {
		return renderAddResult(cmd.OutOrStdout(), "human", envelope, result.Targets[0].Output.Result, result.DryRun)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Plugin: %s %s\n", result.Plugin, result.Version); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Targets: %s\n", addResultTargets(result.Targets)); err != nil {
		return err
	}
	for _, target := range result.Targets {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", target.Target, target.Status); err != nil {
			return err
		}
		if target.NextAction != "" && !fullyInstalled(target.Output.Result.Activation) {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "    Next: %s\n", target.NextAction); err != nil {
				return err
			}
		}
	}
	if result.DryRun {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "No changes made (dry run)."); err != nil {
			return err
		}
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

func addGroupNextAction(targets []addTargetResult) string {
	for _, target := range targets {
		if target.NextAction != "" {
			return "; next action: " + target.NextAction
		}
	}
	return ""
}
