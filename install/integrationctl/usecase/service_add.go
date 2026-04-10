package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type plannedTargetInstall struct {
	TargetID domain.TargetID
	Delivery domain.Delivery
	Adapter  ports.TargetAdapter
	Inspect  ports.InspectResult
	Plan     ports.AdapterPlan
}

type appliedTargetInstall struct {
	Planned plannedTargetInstall
	Result  ports.ApplyResult
	Verify  ports.InspectResult
}

func (s Service) add(ctx context.Context, in AddInput) (domain.Report, error) {
	resolved, err := s.SourceResolver.Resolve(ctx, domain.IntegrationRef{Raw: in.Source})
	if err != nil {
		return domain.Report{}, err
	}
	defer cleanupResolvedSource(resolved)
	manifest, err := s.ManifestLoader.Load(ctx, resolved)
	if err != nil {
		return domain.Report{}, err
	}
	selectedTargets, err := resolveRequestedTargets(manifest, in.Targets)
	if err != nil {
		return domain.Report{}, err
	}
	policy := domain.InstallPolicy{
		Scope:           defaultString(in.Scope, "user"),
		AutoUpdate:      defaultBool(in.AutoUpdate, true),
		AdoptNewTargets: defaultString(in.AdoptNewTargets, "manual"),
		AllowPrerelease: defaultBool(in.AllowPrerelease, false),
	}
	opPrefix := "add"
	summary := fmt.Sprintf("Install plan for integration %q at version %s.", manifest.IntegrationID, manifest.Version)
	if in.DryRun {
		opPrefix = "plan_add"
		summary = fmt.Sprintf("Dry-run plan for integration %q at version %s.", manifest.IntegrationID, manifest.Version)
	}
	report := domain.Report{
		OperationID: operationID(opPrefix, manifest.IntegrationID, s.now()),
		Summary:     summary,
	}
	planned := make([]plannedTargetInstall, 0, len(selectedTargets))
	for _, target := range selectedTargets {
		item, err := s.planTargetInstall(ctx, manifest, policy, target)
		if err != nil {
			return domain.Report{}, err
		}
		planned = append(planned, item)
		report.Targets = append(report.Targets, toTargetReport(item.Delivery, item.Inspect, item.Plan))
	}
	sort.Slice(report.Targets, func(i, j int) bool { return report.Targets[i].TargetID < report.Targets[j].TargetID })
	if in.DryRun {
		return report, nil
	}
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
	}
	return s.applyAdd(ctx, report.OperationID, manifest, resolved, policy, planned)
}

func (s Service) planTargetInstall(ctx context.Context, manifest domain.IntegrationManifest, policy domain.InstallPolicy, target domain.TargetID) (plannedTargetInstall, error) {
	adapter, ok := s.Adapters[target]
	if !ok {
		return plannedTargetInstall{}, domain.NewError(domain.ErrUnsupportedTarget, "adapter not registered for "+string(target), nil)
	}
	delivery := findDelivery(manifest.Deliveries, target)
	if delivery == nil {
		return plannedTargetInstall{}, domain.NewError(domain.ErrUnsupportedTarget, "delivery not available for "+string(target), nil)
	}
	inspect, err := adapter.Inspect(ctx, ports.InspectInput{IntegrationID: manifest.IntegrationID, Scope: policy.Scope})
	if err != nil {
		return plannedTargetInstall{}, err
	}
	plan, err := adapter.PlanInstall(ctx, ports.PlanInstallInput{Manifest: manifest, Policy: policy, Inspect: inspect})
	if err != nil {
		return plannedTargetInstall{}, err
	}
	if _, err := s.validateEvidence(ctx, target, plan.EvidenceKey); err != nil {
		return plannedTargetInstall{}, err
	}
	return plannedTargetInstall{
		TargetID: target,
		Delivery: *delivery,
		Adapter:  adapter,
		Inspect:  inspect,
		Plan:     plan,
	}, nil
}

func (s Service) applyAdd(ctx context.Context, operationID string, manifest domain.IntegrationManifest, resolved ports.ResolvedSource, policy domain.InstallPolicy, planned []plannedTargetInstall) (domain.Report, error) {
	unlock, err := s.LockManager.Acquire(ctx, "state")
	if err != nil {
		return domain.Report{}, domain.NewError(domain.ErrLockAcquire, "acquire integrationctl state lock", err)
	}
	defer func() { _ = unlock() }()

	state, err := s.StateStore.Load(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	if _, exists := findInstallation(state.Installations, manifest.IntegrationID); exists {
		return domain.Report{}, domain.NewError(domain.ErrStateConflict, "integration already exists in state: "+manifest.IntegrationID, nil)
	}

	startedAt := s.now().UTC().Format(time.RFC3339)
	if err := s.Journal.Start(ctx, domain.OperationRecord{
		OperationID:   operationID,
		Type:          "add",
		IntegrationID: manifest.IntegrationID,
		Status:        "in_progress",
		StartedAt:     startedAt,
	}); err != nil {
		return domain.Report{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.Journal.Finish(ctx, operationID, "failed")
		}
	}()
	applied := make([]appliedTargetInstall, 0, len(planned))
	reportTargets := make([]domain.TargetReport, 0, len(planned))
	for _, target := range planned {
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "inspect", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "plan", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}

		applyResult, err := target.Adapter.ApplyInstall(ctx, ports.ApplyInput{
			Plan:           target.Plan,
			Manifest:       manifest,
			ResolvedSource: &resolved,
			Policy:         policy,
			Inspect:        target.Inspect,
		})
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "failed"})
			rollbackFailed, rollbackWarnings := s.rollbackAppliedAdd(ctx, operationID, manifest, policy, startedAt, applied)
			if len(rollbackFailed) > 0 {
				state.Installations = upsertInstallation(state.Installations, degradedRecordFromApplied(manifest, policy, s.workspaceRootForPolicy(policy), startedAt, rollbackFailed))
				if saveErr := s.StateStore.Save(ctx, state); saveErr != nil {
					return domain.Report{}, saveErr
				}
				if stepErr := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: "state", Action: "persist_degraded_state", Status: "ok"}); stepErr != nil {
					return domain.Report{}, stepErr
				}
				if finishErr := s.Journal.Finish(ctx, operationID, "degraded"); finishErr != nil {
					return domain.Report{}, finishErr
				}
				committed = true
				msg := "install failed and rollback was incomplete; degraded state persisted"
				if len(rollbackWarnings) > 0 {
					msg += ": " + strings.Join(rollbackWarnings, "; ")
				}
				return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
			}
			if finishErr := s.Journal.Finish(ctx, operationID, "rolled_back"); finishErr != nil {
				return domain.Report{}, finishErr
			}
			committed = true
			msg := "install failed and applied targets were rolled back"
			if len(rollbackWarnings) > 0 {
				msg += ": " + strings.Join(rollbackWarnings, "; ")
			}
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		verifyRecord := provisionalRecordForAdd(manifest, policy, s.workspaceRootForPolicy(policy), target, applyResult)
		verified, err := s.verifyPostApply(ctx, manifest.IntegrationID, policy, &verifyRecord, target.Adapter, "add")
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "failed"})
			rollbackFailed, rollbackWarnings := s.rollbackAppliedAdd(ctx, operationID, manifest, policy, startedAt, append(applied, appliedTargetInstall{Planned: target, Result: applyResult}))
			if len(rollbackFailed) > 0 {
				state.Installations = upsertInstallation(state.Installations, degradedRecordFromApplied(manifest, policy, s.workspaceRootForPolicy(policy), startedAt, rollbackFailed))
				if saveErr := s.StateStore.Save(ctx, state); saveErr != nil {
					return domain.Report{}, saveErr
				}
				if stepErr := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: "state", Action: "persist_degraded_state", Status: "ok"}); stepErr != nil {
					return domain.Report{}, stepErr
				}
				if finishErr := s.Journal.Finish(ctx, operationID, "degraded"); finishErr != nil {
					return domain.Report{}, finishErr
				}
				committed = true
				msg := "install verification failed and rollback was incomplete; degraded state persisted"
				if len(rollbackWarnings) > 0 {
					msg += ": " + strings.Join(rollbackWarnings, "; ")
				}
				return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
			}
			if finishErr := s.Journal.Finish(ctx, operationID, "rolled_back"); finishErr != nil {
				return domain.Report{}, finishErr
			}
			committed = true
			msg := "install verification failed and applied targets were rolled back"
			if len(rollbackWarnings) > 0 {
				msg += ": " + strings.Join(rollbackWarnings, "; ")
			}
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		applied = append(applied, appliedTargetInstall{Planned: target, Result: applyResult, Verify: verified})
		reportTargets = append(reportTargets, toAppliedTargetReport(target.Delivery, target.Inspect, verified, target.Plan, applyResult))
	}

	state.Installations = upsertInstallation(state.Installations, installationRecordFromApplied(manifest, policy, s.workspaceRootForPolicy(policy), startedAt, applied))
	if err := s.StateStore.Save(ctx, state); err != nil {
		return domain.Report{}, err
	}
	if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: "state", Action: "persist_state", Status: "ok"}); err != nil {
		return domain.Report{}, err
	}
	if err := s.Journal.Finish(ctx, operationID, "committed"); err != nil {
		return domain.Report{}, err
	}
	committed = true
	sort.Slice(reportTargets, func(i, j int) bool { return reportTargets[i].TargetID < reportTargets[j].TargetID })
	return domain.Report{
		OperationID: operationID,
		Summary:     fmt.Sprintf("Installed integration %q at version %s.", manifest.IntegrationID, manifest.Version),
		Targets:     reportTargets,
	}, nil
}

func (s Service) validateEvidence(ctx context.Context, target domain.TargetID, key string) (ports.EvidenceEntry, error) {
	if strings.TrimSpace(key) == "" {
		return ports.EvidenceEntry{}, domain.NewError(domain.ErrEvidenceViolation, "adapter plan missing evidence key for "+string(target), nil)
	}
	entry, err := s.Evidence.Get(ctx, key)
	if err != nil {
		return ports.EvidenceEntry{}, domain.NewError(domain.ErrEvidenceViolation, "unknown evidence key "+key, err)
	}
	return entry, nil
}

func installationRecordFromApplied(manifest domain.IntegrationManifest, policy domain.InstallPolicy, workspaceRoot string, startedAt string, applied []appliedTargetInstall) domain.InstallationRecord {
	targets := make(map[domain.TargetID]domain.TargetInstallation, len(applied))
	for _, item := range applied {
		targets[item.Planned.TargetID] = targetInstallationFromApplied(item)
	}
	return domain.InstallationRecord{
		IntegrationID:      manifest.IntegrationID,
		RequestedSourceRef: manifest.RequestedRef,
		ResolvedSourceRef:  manifest.ResolvedRef,
		ResolvedVersion:    manifest.Version,
		SourceDigest:       manifest.SourceDigest,
		ManifestDigest:     manifest.ManifestDigest,
		Policy:             policy,
		WorkspaceRoot:      workspaceRoot,
		Targets:            targets,
		LastCheckedAt:      startedAt,
		LastUpdatedAt:      startedAt,
	}
}

func degradedRecordFromApplied(manifest domain.IntegrationManifest, policy domain.InstallPolicy, workspaceRoot string, startedAt string, applied []appliedTargetInstall) domain.InstallationRecord {
	record := installationRecordFromApplied(manifest, policy, workspaceRoot, startedAt, applied)
	for key, target := range record.Targets {
		target.State = domain.InstallDegraded
		record.Targets[key] = target
	}
	return record
}

func targetInstallationFromApplied(item appliedTargetInstall) domain.TargetInstallation {
	state := item.Result.State
	if item.Verify.State != "" {
		state = item.Verify.State
	}
	activationState := item.Result.ActivationState
	if item.Verify.ActivationState != "" {
		activationState = item.Verify.ActivationState
	}
	interactiveAuthState := item.Result.InteractiveAuthState
	if strings.TrimSpace(item.Verify.InteractiveAuthState) != "" {
		interactiveAuthState = item.Verify.InteractiveAuthState
	}
	environmentRestrictions := append([]domain.EnvironmentRestrictionCode(nil), item.Result.EnvironmentRestrictions...)
	if len(item.Verify.EnvironmentRestrictions) > 0 {
		environmentRestrictions = append([]domain.EnvironmentRestrictionCode(nil), item.Verify.EnvironmentRestrictions...)
	}
	return domain.TargetInstallation{
		TargetID:                item.Planned.TargetID,
		DeliveryKind:            item.Planned.Delivery.DeliveryKind,
		CapabilitySurface:       append([]string(nil), item.Planned.Delivery.CapabilitySurface...),
		State:                   state,
		NativeRef:               item.Planned.Delivery.NativeRefHint,
		ActivationState:         activationState,
		InteractiveAuthState:    interactiveAuthState,
		CatalogPolicy:           cloneCatalogPolicy(firstNonNilCatalogPolicy(item.Verify.CatalogPolicy, item.Planned.Inspect.CatalogPolicy)),
		EnvironmentRestrictions: environmentRestrictions,
		SourceAccessState:       firstNonEmpty(item.Verify.SourceAccessState, item.Result.SourceAccessState, item.Planned.Inspect.SourceAccessState),
		OwnedNativeObjects:      append([]domain.NativeObjectRef(nil), item.Result.OwnedNativeObjects...),
		AdapterMetadata:         cloneMetadata(item.Result.AdapterMetadata),
	}
}

func (s Service) rollbackAppliedAdd(ctx context.Context, operationID string, manifest domain.IntegrationManifest, policy domain.InstallPolicy, startedAt string, applied []appliedTargetInstall) ([]appliedTargetInstall, []string) {
	failed := make([]appliedTargetInstall, 0)
	warnings := make([]string, 0)
	for i := len(applied) - 1; i >= 0; i-- {
		item := applied[i]
		record := domain.InstallationRecord{
			IntegrationID:      manifest.IntegrationID,
			RequestedSourceRef: manifest.RequestedRef,
			ResolvedSourceRef:  manifest.ResolvedRef,
			ResolvedVersion:    manifest.Version,
			SourceDigest:       manifest.SourceDigest,
			ManifestDigest:     manifest.ManifestDigest,
			Policy:             policy,
			WorkspaceRoot:      s.workspaceRootForPolicy(policy),
			Targets: map[domain.TargetID]domain.TargetInstallation{
				item.Planned.TargetID: targetInstallationFromApplied(item),
			},
			LastCheckedAt: startedAt,
			LastUpdatedAt: startedAt,
		}
		inspect, err := item.Planned.Adapter.Inspect(ctx, ports.InspectInput{IntegrationID: record.IntegrationID, Record: &record, Scope: policy.Scope})
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(item.Planned.TargetID), Action: "rollback", Status: "failed"})
			failed = append(failed, item)
			warnings = append(warnings, "rollback inspect failed for "+string(item.Planned.TargetID)+": "+err.Error())
			continue
		}
		plan, err := item.Planned.Adapter.PlanRemove(ctx, ports.PlanRemoveInput{Record: record, Inspect: inspect})
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(item.Planned.TargetID), Action: "rollback", Status: "failed"})
			failed = append(failed, item)
			warnings = append(warnings, "rollback plan failed for "+string(item.Planned.TargetID)+": "+err.Error())
			continue
		}
		if _, err := s.validateEvidence(ctx, item.Planned.TargetID, plan.EvidenceKey); err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(item.Planned.TargetID), Action: "rollback", Status: "failed"})
			failed = append(failed, item)
			warnings = append(warnings, "rollback evidence validation failed for "+string(item.Planned.TargetID)+": "+err.Error())
			continue
		}
		if _, err := item.Planned.Adapter.ApplyRemove(ctx, ports.ApplyInput{
			Plan:    plan,
			Policy:  policy,
			Inspect: inspect,
			Record:  &record,
		}); err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(item.Planned.TargetID), Action: "rollback", Status: "failed"})
			failed = append(failed, item)
			warnings = append(warnings, "rollback apply failed for "+string(item.Planned.TargetID)+": "+err.Error())
			continue
		}
		_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(item.Planned.TargetID), Action: "rollback", Status: "ok"})
	}
	return failed, warnings
}

func provisionalRecordForAdd(manifest domain.IntegrationManifest, policy domain.InstallPolicy, workspaceRoot string, target plannedTargetInstall, result ports.ApplyResult) domain.InstallationRecord {
	return domain.InstallationRecord{
		IntegrationID:      manifest.IntegrationID,
		RequestedSourceRef: manifest.RequestedRef,
		ResolvedSourceRef:  manifest.ResolvedRef,
		ResolvedVersion:    manifest.Version,
		SourceDigest:       manifest.SourceDigest,
		ManifestDigest:     manifest.ManifestDigest,
		Policy:             policy,
		WorkspaceRoot:      workspaceRoot,
		Targets: map[domain.TargetID]domain.TargetInstallation{
			target.TargetID: {
				TargetID:           target.TargetID,
				DeliveryKind:       target.Delivery.DeliveryKind,
				CapabilitySurface:  append([]string(nil), target.Delivery.CapabilitySurface...),
				NativeRef:          target.Delivery.NativeRefHint,
				OwnedNativeObjects: append([]domain.NativeObjectRef(nil), result.OwnedNativeObjects...),
				AdapterMetadata:    cloneMetadata(result.AdapterMetadata),
			},
		},
	}
}
