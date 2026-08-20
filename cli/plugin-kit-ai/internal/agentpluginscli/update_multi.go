package agentpluginscli

import (
	"context"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/ports"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/usecase"
	"github.com/spf13/cobra"
)

type updateTargetResult struct {
	Target     string        `json:"target"`
	Selected   bool          `json:"selected"`
	Status     string        `json:"status"`
	NextAction string        `json:"next_action,omitempty"`
	Output     addResultData `json:"output"`
}

type updateMultiResult struct {
	OperationID string               `json:"operation_id,omitempty"`
	Batch       bool                 `json:"batch"`
	Status      string               `json:"status"`
	Succeeded   int                  `json:"succeeded"`
	Failed      int                  `json:"failed"`
	Plugin      string               `json:"plugin"`
	Version     string               `json:"version,omitempty"`
	Source      string               `json:"source"`
	Revision    string               `json:"revision,omitempty"`
	TreeDigest  string               `json:"tree_digest,omitempty"`
	DryRun      bool                 `json:"dry_run"`
	Targets     []updateTargetResult `json:"targets"`
}

type updatePreflightFailure struct {
	target         domain.ClientID
	input          usecase.AddInput
	err            error
	observation    domain.ActivationOutcome
	observationErr error
	observed       bool
}

// observedActivation replays the exact read-only native observation produced
// during preflight. This lets the use case persist recognized negative evidence
// after the whole plan has been reviewed, without invoking the client a second
// time or authorizing package materialization.
type observedActivation struct {
	delegate ports.ClientActivator
	outcome  domain.ActivationOutcome
	err      error
}

type recordingActivation struct {
	delegate ports.ClientActivator
	outcome  domain.ActivationOutcome
	err      error
	observed bool
}

func (activation *recordingActivation) Activate(ctx context.Context, request domain.ActivationRequest) (domain.ActivationOutcome, error) {
	activation.outcome, activation.err = activation.delegate.Activate(ctx, request)
	activation.observed = true
	return activation.outcome, activation.err
}

func (activation *recordingActivation) Deactivate(ctx context.Context, request domain.DeactivationRequest) (domain.DeactivationOutcome, error) {
	return activation.delegate.Deactivate(ctx, request)
}

func (activation observedActivation) Activate(context.Context, domain.ActivationRequest) (domain.ActivationOutcome, error) {
	return activation.outcome, activation.err
}

func (activation observedActivation) Deactivate(ctx context.Context, request domain.DeactivationRequest) (domain.DeactivationOutcome, error) {
	return activation.delegate.Deactivate(ctx, request)
}

func runUpdateMany(ctx context.Context, cmd *cobra.Command, app App, opts *options, selector string, targets []domain.ClientID) error {
	state, err := app.StateStore.Load()
	if err != nil {
		return err
	}
	installation, err := selectInstallation(state, selector)
	if err != nil {
		return err
	}
	if installation.NeedsRebind || installation.Package.LoaderKind != domain.LoaderKindAgentPlugins {
		return fmt.Errorf("update requires a bound Agent Plugins installation")
	}
	writeProgress(app, opts.format, "Resolving and validating one updated Agent Plugin package for every selected target...")
	loaded, err := app.loadPackage(ctx, updateSource(installation))
	if err != nil {
		return err
	}
	if loaded.cleanup != nil {
		defer loaded.cleanup()
	}
	if domain.ComputeSourceBindingID(loaded.envelope.Source) != installation.Source.SourceBindingID {
		return fmt.Errorf("resolved source identity changed; use agentplugins switch after reviewing provenance")
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
	selectedTargets := make(map[domain.ClientID]struct{}, len(targets))
	for _, target := range targets {
		if !installationHasTarget(installation, target, opts.scope) {
			return fmt.Errorf("plugin is not installed for target %q in %s scope; no target was changed", target, opts.scope)
		}
		selectedTargets[target] = struct{}{}
	}
	preflightTargets := installationTargets(installation, opts.scope)
	inputs := make([]usecase.AddInput, 0, len(targets))
	applyTargets := make([]domain.ClientID, 0, len(targets))
	preflightFailures := make([]updatePreflightFailure, 0)
	result := updateMultiResult{
		Batch: true, Status: "planned", Plugin: loaded.envelope.Manifest.Name,
		Version: loaded.envelope.Manifest.Version, Source: publicPackageSource(loaded.envelope.Source),
		Revision: loaded.envelope.Source.ResolvedRevision, TreeDigest: loaded.envelope.TreeDigest,
		DryRun: opts.dryRun, Targets: make([]updateTargetResult, 0, len(preflightTargets)),
	}
	for _, target := range preflightTargets {
		_, selectedForRollout := selectedTargets[target]
		client, ok := detected[target]
		if target == domain.ClientChatGPT && !ok {
			client = domain.DetectedClient{ClientID: target, DisplayName: "ChatGPT", Status: domain.DetectionNotDetected}
			detected[target] = client
			ok = true
		}
		if !ok || (client.Status != domain.DetectionDetected && target != domain.ClientChatGPT) {
			return fmt.Errorf("target %q was not detected; no target was changed", target)
		}
		clientPackage := cloneLoadedPackage(loaded)
		if err := prepareLoadedPackageForClient(&clientPackage, target); err != nil {
			return fmt.Errorf("preflight target %s: %w; no target was changed", target, err)
		}
		input := usecase.AddInput{
			Envelope: clientPackage.envelope, Client: client, Scope: domain.ScopeUser,
			DryRun: true, Interactive: false, Hints: clientPackage.hints,
			BackendExecutable:                backendExecutable(client, detected),
			PersistAuthoritativeObservations: !opts.dryRun,
		}
		recorder := &recordingActivation{delegate: app.Activator}
		preflightService := service
		preflightService.Activator = recorder
		planned, planErr := preflightService.Update(ctx, input)
		entry := updateTargetResult{Target: string(target), Selected: selectedForRollout, Status: string(planned.Plan.Status), Output: newAddResultData(clientPackage.envelope, planned, true), NextAction: nextLifecycleAction(planned)}
		result.Targets = append(result.Targets, entry)
		if planErr != nil || planned.Plan.Status == domain.PlanUnsupported {
			result.Status = "preflight_failed"
			result.Failed++
			if planErr == nil {
				planErr = fmt.Errorf("target is unsupported")
			}
			preflightFailures = append(preflightFailures, updatePreflightFailure{
				target: target, input: input, err: planErr,
				observation: recorder.outcome, observationErr: recorder.err, observed: recorder.observed,
			})
			continue
		}
		result.Succeeded++
		if selectedForRollout {
			input.DryRun = opts.dryRun
			inputs = append(inputs, input)
			applyTargets = append(applyTargets, target)
		}
	}
	if len(preflightFailures) > 0 {
		_ = renderUpdateMultiResult(cmd, opts, result)
		if !opts.dryRun && mutationConfirmed(app, opts) {
			for _, failure := range preflightFailures {
				if !failure.observed || !failure.observation.AuthoritativeObservation {
					continue
				}
				input := failure.input
				input.DryRun = false
				input.Confirmed = true
				input.PersistAuthoritativeObservations = true
				observationService := service
				observationService.Activator = observedActivation{delegate: app.Activator, outcome: failure.observation, err: failure.observationErr}
				persisted, persistErr := observationService.Update(ctx, input)
				if persistErr == nil || !persisted.Mutated {
					if persistErr == nil {
						persistErr = fmt.Errorf("native verifier observation did not return its preflight error")
					}
					return fmt.Errorf("persist authoritative observation for target %s after complete preflight: %w", failure.target, persistErr)
				}
			}
		}
		failure := preflightFailures[0]
		return fmt.Errorf("preflight target %s: %w; no package target was changed", failure.target, failure.err)
	}
	if opts.dryRun {
		return renderUpdateMultiResult(cmd, opts, result)
	}
	if opts.format == "human" {
		if err := renderUpdateMultiResult(cmd, opts, result); err != nil {
			return err
		}
	}
	result.Status = "completed"
	operationID, err := newOperationGroupID()
	if err != nil {
		return err
	}
	result.OperationID = operationID
	result.Succeeded = len(result.Targets) - result.Failed
	result.Failed = 0
	targetIndexes := make(map[domain.ClientID]int, len(result.Targets))
	for index := range result.Targets {
		targetIndexes[domain.ClientID(result.Targets[index].Target)] = index
	}
	for index := range inputs {
		inputs[index].Confirmed = true
		inputs[index].PersistAuthoritativeObservations = true
	}
	if app.GroupLifecycle != nil {
		appliedResults, groupErr := app.GroupLifecycle.UpdateGroup(ctx, AddGroupInput{OperationID: operationID, Inputs: inputs})
		if len(appliedResults) > len(inputs) || len(appliedResults) != len(inputs) && groupErr == nil {
			return fmt.Errorf("install engine returned %d update results for %d targets", len(appliedResults), len(inputs))
		}
		for index, applied := range appliedResults {
			target := applyTargets[index]
			result.Targets[targetIndexes[target]] = updateTargetResult{Target: string(target), Selected: true, Status: string(applied.Plan.Status), Output: newAddResultData(inputs[index].Envelope, applied, false), NextAction: nextLifecycleAction(applied)}
		}
		if groupErr != nil {
			for _, target := range applyTargets {
				entry := result.Targets[targetIndexes[target]]
				entry.Status = "rolled_back"
				result.Targets[targetIndexes[target]] = entry
			}
			result.Status = "group_rolled_back"
			result.Failed = len(inputs)
			result.Succeeded = 0
			_ = renderUpdateMultiResult(cmd, opts, result)
			return groupErr
		}
		return renderUpdateMultiResult(cmd, opts, result)
	}
	for index, input := range inputs {
		applied, applyErr := service.Update(ctx, input)
		target := applyTargets[index]
		entry := updateTargetResult{Target: string(target), Selected: true, Status: string(applied.Plan.Status), Output: newAddResultData(input.Envelope, applied, false), NextAction: nextLifecycleAction(applied)}
		result.Targets[targetIndexes[target]] = entry
		if applyErr != nil {
			result.Status = "partial_failure"
			result.Failed++
			result.Succeeded--
			_ = renderUpdateMultiResult(cmd, opts, result)
			return fmt.Errorf("apply target %s failed after complete preflight: %w", target, applyErr)
		}
	}
	return renderUpdateMultiResult(cmd, opts, result)
}

func installationHasTarget(installation domain.Installation, target domain.ClientID, scope string) bool {
	for _, binding := range installation.Clients {
		if binding.ClientID == string(target) && binding.Scope == scope && binding.Materialization != domain.MaterializationAbsent {
			return true
		}
	}
	return false
}

func installationTargets(installation domain.Installation, scope string) []domain.ClientID {
	seen := make(map[domain.ClientID]struct{}, len(installation.Clients))
	result := make([]domain.ClientID, 0, len(installation.Clients))
	for _, binding := range installation.Clients {
		target := domain.ClientID(binding.ClientID)
		if binding.Scope != scope || binding.Materialization == domain.MaterializationAbsent || !supportedTarget(target) {
			continue
		}
		if _, duplicate := seen[target]; duplicate {
			continue
		}
		seen[target] = struct{}{}
		result = append(result, target)
	}
	sortTargets(result)
	return result
}

func renderUpdateMultiResult(cmd *cobra.Command, opts *options, result updateMultiResult) error {
	if opts.format == "json" {
		// The command error is returned separately. Keep a successfully emitted
		// versioned envelope consistent with single-target lifecycle output, and
		// carry operation failure in data.status/failed and each target status.
		return writeJSONOutput(cmd.OutOrStdout(), "update", result)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Plugin: %s %s\n", result.Plugin, result.Version)
	values := make([]string, len(result.Targets))
	for index, target := range result.Targets {
		values[index] = target.Target
		rollout := "preflight only"
		if target.Selected {
			rollout = "selected"
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s (%s)\n", target.Target, target.Status, rollout)
		if target.NextAction != "" && !fullyInstalled(target.Output.Result.Activation) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    Next: %s\n", target.NextAction)
		}
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Targets: %s\n", strings.Join(values, ","))
	return nil
}
