package usecase

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/transaction"
)

type GroupInput struct {
	Targets             []AddInput
	CompatibilityChecks []AddInput
	OperationGroupID    string
	DryRun              bool
	Confirmed           bool
	Switch              bool
	Repair              bool
}

type GroupResult struct {
	InstallationID   string                   `json:"installation_id"`
	OperationGroupID string                   `json:"operation_group_id,omitempty"`
	Targets          []AddResult              `json:"targets"`
	Receipts         []domain.MutationReceipt `json:"-"`
	Mutated          bool                     `json:"mutated"`
}

func (service Service) AddGroup(ctx context.Context, input GroupInput) (GroupResult, error) {
	return service.applyGroup(ctx, input, false)
}

func (service Service) UpdateGroup(ctx context.Context, input GroupInput) (GroupResult, error) {
	return service.applyGroup(ctx, input, true)
}

func (service Service) SwitchGroup(ctx context.Context, input GroupInput) (GroupResult, error) {
	input.Switch = true
	return service.applyGroup(ctx, input, true)
}

func (service Service) RepairGroup(ctx context.Context, input GroupInput) (GroupResult, error) {
	if service.StateStore == nil {
		return GroupResult{}, fmt.Errorf("state store is required")
	}
	state, err := service.StateStore.Load()
	if err != nil {
		return GroupResult{}, err
	}
	for _, target := range input.Targets {
		index, existing, _, err := findStickyInstallation(state, target, domain.ComputeSourceBindingID(target.Envelope.Source))
		if err != nil || !existing {
			if err != nil {
				return GroupResult{}, err
			}
			return GroupResult{}, fmt.Errorf("repair source is not installed")
		}
		installation := state.Installations[index]
		matched := false
		for _, client := range installation.Clients {
			if !sameNativeBackend(domain.ClientID(client.ClientID), target.Client.ClientID) || client.Scope != string(target.Scope) {
				continue
			}
			if !packageRevisionMatches(client.PackageRevision, target.Envelope) {
				return GroupResult{}, fmt.Errorf("repair must use the exact applied package revision for %s", target.Client.ClientID)
			}
			if installation.OriginMode == domain.OriginModeDirectory && (target.DirectoryResolution == nil || client.PackageRevision.DistributionID != target.DirectoryResolution.DistributionID || client.PackageRevision.ReleaseSequence != target.DirectoryResolution.DesiredReleaseSequence) {
				return GroupResult{}, fmt.Errorf("repair must use the exact applied Directory release")
			}
			matched = true
			break
		}
		if !matched {
			return GroupResult{}, fmt.Errorf("repair target %s is not installed", target.Client.ClientID)
		}
	}
	input.Repair = true
	return service.applyGroup(ctx, input, true)
}

type plannedGroupTarget struct {
	input           AddInput
	plan            domain.DeliveryPlan
	resultIndexes   []int
	clientBindingID string
	managed         *domain.ClientBinding
	delivery        domain.StagedDelivery
	dataReceipt     domain.DataReceipt
	dataCreated     bool
}

func (service Service) applyGroup(ctx context.Context, input GroupInput, replace bool) (GroupResult, error) {
	if len(input.Targets) == 0 {
		return GroupResult{}, fmt.Errorf("at least one target is required")
	}
	if service.StateStore == nil || service.Planner == nil || service.Stager == nil || service.Activator == nil {
		return GroupResult{}, fmt.Errorf("agentplugins group dependencies are incomplete")
	}
	groupID := strings.TrimSpace(input.OperationGroupID)
	if groupID == "" {
		var err error
		groupID, err = newOperationID()
		if err != nil {
			return GroupResult{}, err
		}
	}
	release, err := service.beginMutation(ctx, input.DryRun, input.Confirmed)
	if err != nil {
		return GroupResult{}, err
	}
	if release != nil {
		defer func() { _ = release() }()
	}
	state, err := service.StateStore.Load()
	if err != nil {
		return GroupResult{}, err
	}
	first := input.Targets[0]
	if first.OriginMode == "" && first.DirectoryResolution != nil {
		first.OriginMode = domain.OriginModeDirectory
		input.Targets[0] = first
	}
	if err := validateOperationOrigin(first.OriginMode, first.DirectoryResolution); err != nil {
		return GroupResult{}, err
	}
	if first.Envelope.LoaderKind != domain.LoaderKindAgentPlugins {
		return GroupResult{}, fmt.Errorf("group accepts only standard Agent Plugins packages")
	}
	sourceID := domain.ComputeSourceBindingID(first.Envelope.Source)
	installationIndex, existing, sticky, err := findStickyInstallation(state, first, sourceID)
	if err != nil {
		return GroupResult{}, err
	}
	installationID := strings.TrimSpace(first.InstallationID)
	var lifecycleBaseline *domain.Installation
	if existing {
		installation := state.Installations[installationIndex]
		baseline := installation
		lifecycleBaseline = &baseline
		installationID = installation.InstallationID
		if sticky && installation.Source.SourceBindingID != sourceID && !input.Switch {
			sameDistribution := installation.OriginMode == domain.OriginModeDirectory && installation.Directory != nil && first.DirectoryResolution != nil && installation.Directory.DistributionID == first.DirectoryResolution.DistributionID
			if !sameDistribution {
				return GroupResult{}, fmt.Errorf("installation is source-sticky; use switch")
			}
		}
		if installation.NeedsRebind {
			return GroupResult{}, fmt.Errorf("installation requires explicit rebind")
		}
		if input.Switch {
			if installation.DeclaredName != first.Envelope.Manifest.Name {
				return GroupResult{}, fmt.Errorf("switch must preserve manifest identity")
			}
			if installation.OriginMode == domain.OriginModeDirectory && normalizedOriginMode(first.OriginMode) == domain.OriginModeDirectory && installation.Directory != nil && first.DirectoryResolution != nil && installation.Directory.ProductID != first.DirectoryResolution.ProductID {
				return GroupResult{}, fmt.Errorf("switch must remain within one Directory product")
			}
			for otherIndex, other := range state.Installations {
				if otherIndex != installationIndex && other.Source.SourceBindingID == sourceID {
					return GroupResult{}, fmt.Errorf("switch source is already bound to installation %s", other.InstallationID)
				}
			}
		} else if !input.Repair {
			if err := validateDirectoryTransition(installation, first); err != nil {
				return GroupResult{}, err
			}
		}
		if replace && !input.Switch && !input.Repair && installation.OriginMode == domain.OriginModeDirect && immutableDirectGit(installation.Source) {
			return GroupResult{}, fmt.Errorf("direct full-SHA installations require explicit switch")
		}
		if !replace && installation.OriginMode == domain.OriginModeDirectory && installation.Directory != nil && first.DirectoryResolution != nil && first.DirectoryResolution.DesiredReleaseSequence != installation.Directory.DesiredReleaseSequence {
			return GroupResult{}, fmt.Errorf("adding targets must retain recorded release sequence %d; update separately", installation.Directory.DesiredReleaseSequence)
		}
		if !replace && installation.Source.TreeDigest != first.Envelope.TreeDigest {
			return GroupResult{}, fmt.Errorf("adding targets must use recorded desired package bytes; update separately")
		}
	} else if replace {
		return GroupResult{}, fmt.Errorf("update requires an existing installation")
	}
	if installationID == "" {
		installationID, err = domain.NewInstallationID()
		if err != nil {
			return GroupResult{}, err
		}
	}
	result := GroupResult{InstallationID: installationID, OperationGroupID: groupID, Targets: make([]AddResult, len(input.Targets))}
	physical := map[string]int{}
	planned := make([]plannedGroupTarget, 0, len(input.Targets))
	for targetIndex, target := range input.Targets {
		if target.OriginMode == "" && target.DirectoryResolution != nil {
			target.OriginMode = domain.OriginModeDirectory
			input.Targets[targetIndex] = target
		}
		if err := validateOperationOrigin(target.OriginMode, target.DirectoryResolution); err != nil {
			return result, err
		}
		if target.Envelope.TreeDigest != first.Envelope.TreeDigest || target.Envelope.ManifestDigest != first.Envelope.ManifestDigest || domain.ComputeSourceBindingID(target.Envelope.Source) != sourceID {
			return result, fmt.Errorf("all targets in one operation group must use one immutable package")
		}
		if target.Scope != domain.ScopeUser {
			return result, fmt.Errorf("%s scope is not proven; group mutation supports user scope only", target.Scope)
		}
		if target.ReleaseRevoked && normalizedOriginMode(target.OriginMode) == domain.OriginModeDirectory {
			return result, fmt.Errorf("revoked release cannot be exposed, updated, or repaired")
		}
		if target.DistributionSuspended && normalizedOriginMode(target.OriginMode) == domain.OriginModeDirectory && !input.Repair {
			return result, fmt.Errorf("suspended distribution blocks group add/update")
		}
		plan, err := service.Planner.Plan(ctx, target.Envelope, target.Client, target.Scope, domain.ComputePhysicalArtifactID(target.Envelope.Manifest.Name, installationID))
		if err != nil {
			return result, err
		}
		result.Targets[targetIndex] = AddResult{InstallationID: installationID, Plan: plan}
		if target.ReleaseRevoked && normalizedOriginMode(target.OriginMode) == domain.OriginModeDirect {
			result.Targets[targetIndex].Plan.Warnings = append(result.Targets[targetIndex].Plan.Warnings, "direct_source_digest_matches_known_revoked_directory_release")
		}
		if plan.Status == domain.PlanUnsupported {
			return result, fmt.Errorf("target %s is unsupported; group preflight caused no mutation", target.Client.ClientID)
		}
		if err := preflightRuntime(target.Envelope, plan); err != nil {
			return result, err
		}
		key := plan.ActivePath
		if sameNativeBackend(target.Client.ClientID, domain.ClientCopilot) {
			key = "shared-copilot-vscode:" + plan.PhysicalArtifactID
		} else if existing {
			for _, binding := range state.Installations[installationIndex].Clients {
				if binding.PhysicalArtifact == plan.PhysicalArtifactID && sameNativeBackend(domain.ClientID(binding.ClientID), target.Client.ClientID) && binding.Materialization != domain.MaterializationAbsent {
					key = binding.TargetLocator
					break
				}
			}
		}
		if priorIndex, ok := physical[key]; ok {
			prior := &planned[priorIndex]
			if !sameNativeBackend(prior.input.Client.ClientID, target.Client.ClientID) {
				return result, fmt.Errorf("targets collide on physical backend %s", key)
			}
			prior.resultIndexes = append(prior.resultIndexes, targetIndex)
			result.Targets[targetIndex].Plan = prior.plan
			continue
		}
		clientID := domain.ComputeClientBindingID(installationID, string(target.Client.ClientID), string(target.Scope), plan.ActivePath)
		var managed *domain.ClientBinding
		if existing {
			if binding, ok := state.Installations[installationIndex].Clients[clientID]; ok {
				copy := binding
				managed = &copy
			} else if sameNativeBackend(target.Client.ClientID, domain.ClientCopilot) {
				for _, binding := range state.Installations[installationIndex].Clients {
					if binding.PhysicalArtifact == plan.PhysicalArtifactID && sameNativeBackend(domain.ClientID(binding.ClientID), target.Client.ClientID) && binding.Materialization != domain.MaterializationAbsent {
						copy := binding
						managed = &copy
						clientID = binding.ClientBindingID
						plan.ActivePath = binding.TargetLocator
						plan.TargetRoot = filepath.Dir(binding.TargetLocator)
						break
					}
				}
			}
		}
		if replace && managed == nil {
			return result, fmt.Errorf("update target %s is not installed", target.Client.ClientID)
		}
		result.Targets[targetIndex].Plan = plan
		if err := service.observeGroupNativeIdentity(ctx, target.Client, plan, managed, input.Repair); err != nil {
			return result, err
		}
		physical[key] = len(planned)
		planned = append(planned, plannedGroupTarget{input: target, plan: plan, resultIndexes: []int{targetIndex}, clientBindingID: clientID, managed: managed})
	}
	if replace && existing && !input.Repair {
		compatibleBindings := map[string]bool{}
		for _, target := range planned {
			if target.managed != nil {
				compatibleBindings[target.managed.ClientBindingID] = true
			}
		}
		checks := input.CompatibilityChecks
		if input.Switch {
			checks = nil
		}
		for _, check := range checks {
			if check.Envelope.TreeDigest != first.Envelope.TreeDigest || check.Envelope.ManifestDigest != first.Envelope.ManifestDigest {
				return result, fmt.Errorf("compatibility preflight must use the update candidate bytes")
			}
			plan, err := service.Planner.Plan(ctx, check.Envelope, check.Client, check.Scope, domain.ComputePhysicalArtifactID(check.Envelope.Manifest.Name, installationID))
			if err != nil {
				return result, err
			}
			if plan.Status == domain.PlanUnsupported {
				return result, fmt.Errorf("update candidate is incompatible with installed binding %s", check.Client.ClientID)
			}
			if err := preflightRuntime(check.Envelope, plan); err != nil {
				return result, err
			}
			for _, binding := range state.Installations[installationIndex].Clients {
				if sameNativeBackend(domain.ClientID(binding.ClientID), check.Client.ClientID) && binding.Scope == string(check.Scope) {
					compatibleBindings[binding.ClientBindingID] = true
				}
			}
		}
		for _, binding := range state.Installations[installationIndex].Clients {
			if binding.Materialization == domain.MaterializationAbsent {
				continue
			}
			if !compatibleBindings[binding.ClientBindingID] {
				return result, fmt.Errorf("update group must preflight every installed physical binding; %s is missing", binding.ClientID)
			}
		}
	}
	if input.DryRun || !input.Confirmed {
		return result, nil
	}
	cleanup := func() {
		for _, target := range planned {
			if target.delivery.StagingPath != "" {
				_ = service.Stager.Discard(context.Background(), target.delivery)
			}
			if target.dataCreated {
				_ = service.PluginData.PurgeData(context.Background(), target.dataReceipt)
			}
		}
	}
	for targetIndex := range planned {
		target := &planned[targetIndex]
		operationID := fmt.Sprintf("%s-%03d", groupID, targetIndex+1)
		if packageNeedsPluginData(target.input.Envelope) {
			if service.PluginData == nil {
				cleanup()
				return result, fmt.Errorf("PLUGIN_DATA manager is required for stdio MCP packages")
			}
			receipt, created, err := service.PluginData.EnsureData(ctx, installationID, target.plan.PhysicalArtifactID, string(target.input.Scope))
			if err != nil {
				cleanup()
				return result, err
			}
			target.dataReceipt, target.dataCreated = receipt, created
		}
		delivery, err := service.stagePackage(ctx, target.input.Envelope, target.plan, operationID, target.input.Hints, target.dataReceipt.Locator)
		if err != nil {
			cleanup()
			return result, err
		}
		target.delivery = delivery
	}
	defer cleanup()
	for _, target := range planned {
		if err := service.observeGroupNativeIdentity(ctx, target.input.Client, target.plan, target.managed, input.Repair); err != nil {
			return result, fmt.Errorf("native identity changed before group commit: %w", err)
		}
	}
	desired := state
	for _, target := range planned {
		desired, installationIndex = upsertPreparedInstallation(desired, installationIndex, existing, target.input, target.plan, installationID, target.clientBindingID, sourceID, service.now())
		existing = true
		installation := desired.Installations[installationIndex]
		client := installation.Clients[target.clientBindingID]
		for _, resultIndex := range target.resultIndexes {
			client.AffectedSurfaces = append(client.AffectedSurfaces, string(input.Targets[resultIndex].Client.ClientID))
		}
		sort.Strings(client.AffectedSurfaces)
		if target.dataReceipt.DataReceiptID != "" {
			if installation.DataReceipts == nil {
				installation.DataReceipts = map[string]domain.DataReceipt{}
			}
			installation.DataReceipts[target.dataReceipt.DataReceiptID] = target.dataReceipt
			client.DataReceiptID = target.dataReceipt.DataReceiptID
		}
		installation.DataRetained = false
		installation.OperationGroupID = groupID
		installation.Clients[target.clientBindingID] = client
		desired.Installations[installationIndex] = installation
	}
	if input.Switch {
		installation := desired.Installations[installationIndex]
		installation.Source = domain.SourceBinding{SourceBindingID: sourceID, RequestedSource: first.Envelope.Source.RequestedSource,
			CanonicalSource: first.Envelope.Source.CanonicalSource, Repository: first.Envelope.Source.Repository, PackageSubpath: first.Envelope.Source.PackageSubpath,
			ResolvedRevision: first.Envelope.Source.ResolvedRevision, TreeDigest: first.Envelope.TreeDigest}
		installation.OriginMode = normalizedOriginMode(first.OriginMode)
		installation.Directory = cloneDirectoryOrigin(first.DirectoryResolution)
		desired.Installations[installationIndex] = installation
	} else if input.Repair && lifecycleBaseline != nil {
		installation := desired.Installations[installationIndex]
		installation.Source = lifecycleBaseline.Source
		installation.Package = lifecycleBaseline.Package
		installation.OriginMode = lifecycleBaseline.OriginMode
		installation.Directory = cloneDirectoryOrigin(lifecycleBaseline.Directory)
		desired.Installations[installationIndex] = installation
	}
	mutations := make([]transaction.DirectoryMutation, 0, len(planned))
	for targetIndex, target := range planned {
		client := desired.Installations[installationIndex].Clients[target.clientBindingID]
		before := ""
		if target.managed != nil {
			before = managedDigest(*target.managed)
		}
		operationID := fmt.Sprintf("%s-%03d", groupID, targetIndex+1)
		delivery := target.delivery
		mutations = append(mutations, transaction.DirectoryMutation{OperationID: operationID, InstallationID: installationID, ClientBindingID: target.clientBindingID,
			Sequence: nextSequence(client), OwnedBase: delivery.OwnedBase, ActivePath: delivery.ActivePath, StagingPath: delivery.StagingPath,
			BeforeDigest: before, AfterDigest: delivery.ArtifactDigest, NativeObjects: delivery.NativeObjects, Activation: target.plan.Activation,
			Authentication: target.plan.Authentication, Policy: domain.PolicyAllowed, Verification: target.plan.Verification,
			Verify: func(verifyContext context.Context, activePath string) error {
				return service.Stager.Verify(verifyContext, activePath, delivery.ArtifactDigest)
			}})
	}
	kernel := service.Kernel
	kernel.StateStore = service.StateStore
	receipts, err := kernel.ApplyDirectoryGroup(ctx, transaction.DirectoryGroup{OperationGroupID: groupID, Mutations: mutations, DesiredState: desired})
	if err != nil {
		result.Receipts = receipts
		if len(receipts) > 0 {
			for index := range planned {
				planned[index].dataCreated = false
			}
		}
		return result, err
	}
	result.Receipts, result.Mutated = receipts, true
	for index := range planned {
		planned[index].dataCreated = false
	}
	for _, target := range planned {
		outcome, activationErr := service.Activator.Activate(ctx, domain.ActivationRequest{Client: target.input.Client, Plan: target.plan, Delivery: target.delivery,
			DeclaredName: target.input.Envelope.Manifest.Name, Replacing: replace, Interactive: target.input.Interactive, BackendExecutable: target.input.BackendExecutable})
		_, persistErr := service.updateLifecycle(installationID, target.clientBindingID, outcome)
		for _, resultIndex := range target.resultIndexes {
			result.Targets[resultIndex].Activation = outcome
			result.Targets[resultIndex].Mutated = true
		}
		if activationErr != nil {
			return result, activationErr
		}
		if persistErr != nil {
			return result, persistErr
		}
	}
	return result, nil
}

// Repair is the one lifecycle operation where an owned native object may be
// absent: recreating that exact object is its purpose. A foreign or
// indeterminate object remains blocking, and an existing managed object still
// has to match its recorded ownership digest.
func (service Service) observeGroupNativeIdentity(ctx context.Context, client domain.DetectedClient, plan domain.DeliveryPlan, managed *domain.ClientBinding, repair bool) error {
	if !repair || service.NativeObserver == nil {
		return service.observeNativeIdentity(ctx, client, plan, managed)
	}
	observation, err := service.NativeObserver.ObserveNativeIdentity(ctx, client, plan, managed)
	if err != nil {
		return fmt.Errorf("observe native identity for %s: %w", client.ClientID, err)
	}
	if observation.State == domain.NativeIdentityAbsent && managed != nil {
		return nil
	}
	switch observation.State {
	case domain.NativeIdentityManaged:
		if managed == nil {
			return fmt.Errorf("native identity already exists without matching agentplugins ownership; automatic adoption is disabled")
		}
		expected := managedDigest(*managed)
		if expected == "" || observation.Digest == "" || expected != observation.Digest {
			return fmt.Errorf("native identity ownership digest is stale or does not match")
		}
		return nil
	case domain.NativeIdentityUnmanaged:
		return fmt.Errorf("native identity is unmanaged; automatic adoption is disabled")
	case domain.NativeIdentityIndeterminate:
		return fmt.Errorf("native identity ownership is indeterminate; refusing repair")
	case domain.NativeIdentityAbsent:
		return fmt.Errorf("native identity is absent without a recorded managed binding")
	default:
		return fmt.Errorf("native identity observer returned an unknown state")
	}
}
