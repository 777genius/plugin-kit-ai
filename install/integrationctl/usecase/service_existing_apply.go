package usecase

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type removedExistingTarget struct {
	Planned plannedExistingTarget
	Result  ports.ApplyResult
}

func (s Service) applyExisting(ctx context.Context, record domain.InstallationRecord, action string, planned []plannedExistingTarget) (domain.Report, error) {
	if action == "remove_orphaned_target" {
		return s.applyRemoveExisting(ctx, record, planned)
	}
	if action == "repair_drift" {
		return s.applyRepairExisting(ctx, record, planned)
	}
	if action == "update_version" {
		return s.applyUpdateExisting(ctx, record, planned)
	}
	if len(planned) != 1 {
		return domain.Report{}, domain.NewError(domain.ErrMutationApply, "non-dry-run existing lifecycle currently supports one target at a time until rollback is implemented", nil)
	}
	target := planned[0]
	defer cleanupPlannedExisting(planned)
	if target.Plan.Blocking {
		return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
	}
	operationID := operationID(actionNamePrefix(action), record.IntegrationID, s.now())
	unlock, err := s.LockManager.Acquire(ctx, "state")
	if err != nil {
		return domain.Report{}, domain.NewError(domain.ErrLockAcquire, "acquire integrationctl state lock", err)
	}
	defer func() { _ = unlock() }()

	startedAt := s.now().UTC().Format(time.RFC3339)
	if err := s.Journal.Start(ctx, domain.OperationRecord{
		OperationID:   operationID,
		Type:          actionNamePrefix(action),
		IntegrationID: record.IntegrationID,
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
	if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "inspect", Status: "ok"}); err != nil {
		return domain.Report{}, err
	}
	if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "plan", Status: "ok"}); err != nil {
		return domain.Report{}, err
	}

	var applyResult ports.ApplyResult
	switch action {
	case "update_version":
		applyResult, err = target.Adapter.ApplyUpdate(ctx, ports.ApplyInput{
			Plan:           target.Plan,
			Manifest:       *target.Manifest,
			ResolvedSource: target.Resolved,
			Policy:         record.Policy,
			Inspect:        target.Inspect,
			Record:         &record,
		})
	case "remove_orphaned_target":
		applyResult, err = target.Adapter.ApplyRemove(ctx, ports.ApplyInput{
			Plan:    target.Plan,
			Policy:  record.Policy,
			Inspect: target.Inspect,
			Record:  &record,
		})
	case "repair_drift":
		applyResult, err = target.Adapter.Repair(ctx, ports.RepairInput{
			Record:         record,
			Inspect:        target.Inspect,
			Manifest:       target.Manifest,
			ResolvedSource: target.Resolved,
		})
	case "enable_target":
		toggle := target.Adapter.(ports.ToggleTargetAdapter)
		applyResult, err = toggle.ApplyEnable(ctx, ports.ApplyInput{
			Plan:    target.Plan,
			Policy:  record.Policy,
			Inspect: target.Inspect,
			Record:  &record,
		})
	case "disable_target":
		toggle := target.Adapter.(ports.ToggleTargetAdapter)
		applyResult, err = toggle.ApplyDisable(ctx, ports.ApplyInput{
			Plan:    target.Plan,
			Policy:  record.Policy,
			Inspect: target.Inspect,
			Record:  &record,
		})
	default:
		return domain.Report{}, domain.NewError(domain.ErrUsage, "unsupported existing lifecycle action "+action, nil)
	}
	if err != nil {
		_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "failed"})
		return domain.Report{}, err
	}
	if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "ok"}); err != nil {
		return domain.Report{}, err
	}

	verifyRecord := provisionalRecordForExisting(record, target, applyResult)
	verified, err := s.verifyPostApply(ctx, record.IntegrationID, record.Policy, &verifyRecord, target.Adapter, action)
	if err != nil {
		_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "failed"})
		return domain.Report{}, err
	}
	if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "ok"}); err != nil {
		return domain.Report{}, err
	}

	state, err := s.StateStore.Load(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	items, found := findInstallationMutable(state.Installations, record.IntegrationID)
	if !found {
		return domain.Report{}, domain.NewError(domain.ErrStateConflict, "integration disappeared from state during apply: "+record.IntegrationID, nil)
	}
	nextRecord := items
	switch action {
	case "update_version":
		fallthrough
	case "repair_drift":
		fallthrough
	case "enable_target":
		fallthrough
	case "disable_target":
		if target.Manifest != nil {
			nextRecord.ResolvedVersion = target.Manifest.Version
			nextRecord.ResolvedSourceRef = target.Manifest.ResolvedRef
			nextRecord.SourceDigest = target.Manifest.SourceDigest
			nextRecord.ManifestDigest = target.Manifest.ManifestDigest
		}
		nextRecord.LastCheckedAt = startedAt
		nextRecord.LastUpdatedAt = startedAt
		nextRecord.Targets[target.TargetID] = targetInstallationFromExisting(target, applyResult, verified)
		state.Installations = upsertInstallation(state.Installations, nextRecord)
	case "remove_orphaned_target":
		delete(nextRecord.Targets, target.TargetID)
		if len(nextRecord.Targets) == 0 {
			state.Installations = removeInstallation(state.Installations, nextRecord.IntegrationID)
		} else {
			nextRecord.LastCheckedAt = startedAt
			nextRecord.LastUpdatedAt = startedAt
			state.Installations = upsertInstallation(state.Installations, nextRecord)
		}
	}
	if err := s.StateStore.Save(ctx, state); err != nil {
		return domain.Report{}, err
	}
	if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "persist_state", Status: "ok"}); err != nil {
		return domain.Report{}, err
	}
	if err := s.Journal.Finish(ctx, operationID, "committed"); err != nil {
		return domain.Report{}, err
	}
	committed = true
	return domain.Report{
		OperationID: operationID,
		Summary:     summaryForExisting(action, record.IntegrationID),
		Targets: []domain.TargetReport{
			toAppliedTargetReport(target.Delivery, target.Inspect, verified, target.Plan, applyResult),
		},
	}, nil
}

func (s Service) applyRemoveExisting(ctx context.Context, record domain.InstallationRecord, planned []plannedExistingTarget) (domain.Report, error) {
	if len(planned) == 0 {
		return domain.Report{}, domain.NewError(domain.ErrMutationApply, "remove requires at least one planned target", nil)
	}
	defer cleanupPlannedExisting(planned)
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
	}
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
	}
	operationID := operationID(actionNamePrefix("remove_orphaned_target"), record.IntegrationID, s.now())
	unlock, err := s.LockManager.Acquire(ctx, "state")
	if err != nil {
		return domain.Report{}, domain.NewError(domain.ErrLockAcquire, "acquire integrationctl state lock", err)
	}
	defer func() { _ = unlock() }()

	startedAt := s.now().UTC().Format(time.RFC3339)
	if err := s.Journal.Start(ctx, domain.OperationRecord{
		OperationID:   operationID,
		Type:          "remove",
		IntegrationID: record.IntegrationID,
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

	state, err := s.StateStore.Load(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	removed := make([]removedExistingTarget, 0, len(planned))
	reportTargets := make([]domain.TargetReport, 0, len(planned))
	for _, target := range planned {
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "inspect", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "plan", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		applyResult, err := target.Adapter.ApplyRemove(ctx, ports.ApplyInput{
			Plan:    target.Plan,
			Policy:  record.Policy,
			Inspect: target.Inspect,
			Record:  &record,
		})
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "failed"})
			rollbackFailed, rollbackWarnings := s.rollbackRemovedExisting(ctx, operationID, record, removed)
			if len(rollbackFailed) > 0 {
				state.Installations = upsertInstallation(state.Installations, degradedRecordForRemoveFailure(record, startedAt, target.TargetID, rollbackFailed))
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
				msg := "remove failed and rollback was incomplete; degraded state persisted"
				if len(rollbackWarnings) > 0 {
					msg += ": " + strings.Join(rollbackWarnings, "; ")
				}
				return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
			}
			if finishErr := s.Journal.Finish(ctx, operationID, "rolled_back"); finishErr != nil {
				return domain.Report{}, finishErr
			}
			committed = true
			msg := "remove failed and removed targets were rolled back"
			if len(rollbackWarnings) > 0 {
				msg += ": " + strings.Join(rollbackWarnings, "; ")
			}
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		verified, err := s.verifyPostApply(ctx, record.IntegrationID, record.Policy, &record, target.Adapter, "remove_orphaned_target")
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "failed"})
			rollbackFailed, rollbackWarnings := s.rollbackRemovedExisting(ctx, operationID, record, append(removed, removedExistingTarget{Planned: target, Result: applyResult}))
			if len(rollbackFailed) > 0 {
				state.Installations = upsertInstallation(state.Installations, degradedRecordForRemoveFailure(record, startedAt, target.TargetID, rollbackFailed))
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
				msg := "remove verification failed and rollback was incomplete; degraded state persisted"
				if len(rollbackWarnings) > 0 {
					msg += ": " + strings.Join(rollbackWarnings, "; ")
				}
				return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
			}
			if finishErr := s.Journal.Finish(ctx, operationID, "rolled_back"); finishErr != nil {
				return domain.Report{}, finishErr
			}
			committed = true
			msg := "remove verification failed and removed targets were rolled back"
			if len(rollbackWarnings) > 0 {
				msg += ": " + strings.Join(rollbackWarnings, "; ")
			}
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		removed = append(removed, removedExistingTarget{Planned: target, Result: applyResult})
		reportTargets = append(reportTargets, toAppliedTargetReport(target.Delivery, target.Inspect, verified, target.Plan, applyResult))
	}

	items, found := findInstallationMutable(state.Installations, record.IntegrationID)
	if !found {
		return domain.Report{}, domain.NewError(domain.ErrStateConflict, "integration disappeared from state during apply: "+record.IntegrationID, nil)
	}
	nextRecord := cloneInstallationRecord(items)
	for _, target := range removed {
		delete(nextRecord.Targets, target.Planned.TargetID)
	}
	if len(nextRecord.Targets) == 0 {
		state.Installations = removeInstallation(state.Installations, nextRecord.IntegrationID)
	} else {
		nextRecord.LastCheckedAt = startedAt
		nextRecord.LastUpdatedAt = startedAt
		state.Installations = upsertInstallation(state.Installations, nextRecord)
	}
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
		Summary:     summaryForExisting("remove_orphaned_target", record.IntegrationID),
		Targets:     reportTargets,
	}, nil
}

func (s Service) applyRepairExisting(ctx context.Context, record domain.InstallationRecord, planned []plannedExistingTarget) (domain.Report, error) {
	if len(planned) == 0 {
		return domain.Report{}, domain.NewError(domain.ErrMutationApply, "repair requires at least one planned target", nil)
	}
	defer cleanupPlannedExisting(planned)
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
		if target.Manifest == nil || target.Resolved == nil {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "repair requires resolved source and manifest for target "+string(target.TargetID), nil)
		}
	}
	operationID := operationID(actionNamePrefix("repair_drift"), record.IntegrationID, s.now())
	unlock, err := s.LockManager.Acquire(ctx, "state")
	if err != nil {
		return domain.Report{}, domain.NewError(domain.ErrLockAcquire, "acquire integrationctl state lock", err)
	}
	defer func() { _ = unlock() }()

	startedAt := s.now().UTC().Format(time.RFC3339)
	if err := s.Journal.Start(ctx, domain.OperationRecord{
		OperationID:   operationID,
		Type:          "repair",
		IntegrationID: record.IntegrationID,
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

	state, err := s.StateStore.Load(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	items, found := findInstallationMutable(state.Installations, record.IntegrationID)
	if !found {
		return domain.Report{}, domain.NewError(domain.ErrStateConflict, "integration disappeared from state during apply: "+record.IntegrationID, nil)
	}
	nextRecord := cloneInstallationRecord(items)
	reportTargets := make([]domain.TargetReport, 0, len(planned))
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
	}
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
	}
	for _, target := range planned {
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "inspect", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "plan", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		applyResult, err := target.Adapter.Repair(ctx, ports.RepairInput{
			Record:         record,
			Inspect:        target.Inspect,
			Manifest:       target.Manifest,
			ResolvedSource: target.Resolved,
		})
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "failed"})
			markTargetDegraded(&nextRecord, target.TargetID)
			applyManifestMetadata(&nextRecord, *target.Manifest, startedAt)
			state.Installations = upsertInstallation(state.Installations, nextRecord)
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
			return domain.Report{}, domain.NewError(domain.ErrRepairApply, "repair failed after partial progress; degraded state persisted", err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		verifyRecord := provisionalRecordForExisting(record, target, applyResult)
		verified, err := s.verifyPostApply(ctx, record.IntegrationID, record.Policy, &verifyRecord, target.Adapter, "repair_drift")
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "failed"})
			markTargetDegraded(&nextRecord, target.TargetID)
			applyManifestMetadata(&nextRecord, *target.Manifest, startedAt)
			state.Installations = upsertInstallation(state.Installations, nextRecord)
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
			return domain.Report{}, domain.NewError(domain.ErrRepairApply, "repair verification failed after partial progress; degraded state persisted", err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		nextRecord.Targets[target.TargetID] = targetInstallationFromExisting(target, applyResult, verified)
		applyManifestMetadata(&nextRecord, *target.Manifest, startedAt)
		reportTargets = append(reportTargets, toAppliedTargetReport(target.Delivery, target.Inspect, verified, target.Plan, applyResult))
	}

	state.Installations = upsertInstallation(state.Installations, nextRecord)
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
		Summary:     summaryForExisting("repair_drift", record.IntegrationID),
		Targets:     reportTargets,
	}, nil
}

func (s Service) applyUpdateExisting(ctx context.Context, record domain.InstallationRecord, planned []plannedExistingTarget) (domain.Report, error) {
	if len(planned) == 0 {
		return domain.Report{}, domain.NewError(domain.ErrMutationApply, "update requires at least one planned target", nil)
	}
	defer cleanupPlannedExisting(planned)
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
		if target.Manifest == nil || target.Resolved == nil {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "update requires resolved source and manifest for target "+string(target.TargetID), nil)
		}
	}
	operationID := operationID(actionNamePrefix("update_version"), record.IntegrationID, s.now())
	unlock, err := s.LockManager.Acquire(ctx, "state")
	if err != nil {
		return domain.Report{}, domain.NewError(domain.ErrLockAcquire, "acquire integrationctl state lock", err)
	}
	defer func() { _ = unlock() }()

	startedAt := s.now().UTC().Format(time.RFC3339)
	if err := s.Journal.Start(ctx, domain.OperationRecord{
		OperationID:   operationID,
		Type:          "update",
		IntegrationID: record.IntegrationID,
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

	state, err := s.StateStore.Load(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	items, found := findInstallationMutable(state.Installations, record.IntegrationID)
	if !found {
		return domain.Report{}, domain.NewError(domain.ErrStateConflict, "integration disappeared from state during apply: "+record.IntegrationID, nil)
	}
	nextRecord := cloneInstallationRecord(items)
	reportTargets := make([]domain.TargetReport, 0, len(planned))
	for _, target := range planned {
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "inspect", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "plan", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		var applyResult ports.ApplyResult
		var err error
		if target.Adopted {
			applyResult, err = target.Adapter.ApplyInstall(ctx, ports.ApplyInput{
				Plan:           target.Plan,
				Manifest:       *target.Manifest,
				ResolvedSource: target.Resolved,
				Policy:         record.Policy,
				Inspect:        target.Inspect,
				Record:         &record,
			})
		} else {
			applyResult, err = target.Adapter.ApplyUpdate(ctx, ports.ApplyInput{
				Plan:           target.Plan,
				Manifest:       *target.Manifest,
				ResolvedSource: target.Resolved,
				Policy:         record.Policy,
				Inspect:        target.Inspect,
				Record:         &record,
			})
		}
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "failed"})
			markPlannedTargetDegraded(&nextRecord, target)
			applyManifestMetadata(&nextRecord, *target.Manifest, startedAt)
			state.Installations = upsertInstallation(state.Installations, nextRecord)
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
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "update failed after partial progress; degraded state persisted", err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		verifyRecord := provisionalRecordForExisting(record, target, applyResult)
		verified, err := s.verifyPostApply(ctx, record.IntegrationID, record.Policy, &verifyRecord, target.Adapter, "update_version")
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "failed"})
			markPlannedTargetDegraded(&nextRecord, target)
			applyManifestMetadata(&nextRecord, *target.Manifest, startedAt)
			state.Installations = upsertInstallation(state.Installations, nextRecord)
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
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "update verification failed after partial progress; degraded state persisted", err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		nextRecord.Targets[target.TargetID] = targetInstallationFromExisting(target, applyResult, verified)
		applyManifestMetadata(&nextRecord, *target.Manifest, startedAt)
		reportTargets = append(reportTargets, toAppliedTargetReport(target.Delivery, target.Inspect, verified, target.Plan, applyResult))
	}

	state.Installations = upsertInstallation(state.Installations, nextRecord)
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
		Summary:     summaryForExisting("update_version", record.IntegrationID),
		Targets:     reportTargets,
	}, nil
}

func targetInstallationFromExisting(item plannedExistingTarget, result ports.ApplyResult, verified ports.InspectResult) domain.TargetInstallation {
	state := result.State
	if verified.State != "" {
		state = verified.State
	}
	activationState := result.ActivationState
	if verified.ActivationState != "" {
		activationState = verified.ActivationState
	}
	interactiveAuthState := result.InteractiveAuthState
	if strings.TrimSpace(verified.InteractiveAuthState) != "" {
		interactiveAuthState = verified.InteractiveAuthState
	}
	environmentRestrictions := append([]domain.EnvironmentRestrictionCode(nil), result.EnvironmentRestrictions...)
	if len(verified.EnvironmentRestrictions) > 0 {
		environmentRestrictions = append([]domain.EnvironmentRestrictionCode(nil), verified.EnvironmentRestrictions...)
	}
	out := domain.TargetInstallation{
		TargetID:                item.TargetID,
		DeliveryKind:            item.Delivery.DeliveryKind,
		CapabilitySurface:       append([]string(nil), item.Delivery.CapabilitySurface...),
		State:                   state,
		NativeRef:               firstNonEmpty(item.Delivery.NativeRefHint, item.Current.NativeRef),
		ActivationState:         activationState,
		InteractiveAuthState:    interactiveAuthState,
		CatalogPolicy:           cloneCatalogPolicy(firstNonNilCatalogPolicy(verified.CatalogPolicy, item.Inspect.CatalogPolicy)),
		EnvironmentRestrictions: environmentRestrictions,
		SourceAccessState:       firstNonEmpty(verified.SourceAccessState, result.SourceAccessState, item.Inspect.SourceAccessState),
		OwnedNativeObjects:      append([]domain.NativeObjectRef(nil), result.OwnedNativeObjects...),
		AdapterMetadata:         cloneMetadata(result.AdapterMetadata),
	}
	if out.State == "" {
		out.State = domain.InstallInstalled
	}
	return out
}

func degradedRecordForRemoveFailure(record domain.InstallationRecord, startedAt string, failedTarget domain.TargetID, rollbackFailed []domain.TargetID) domain.InstallationRecord {
	next := cloneInstallationRecord(record)
	next.LastCheckedAt = startedAt
	next.LastUpdatedAt = startedAt

	if len(rollbackFailed) > 0 {
		for targetID, target := range next.Targets {
			target.State = domain.InstallDegraded
			next.Targets[targetID] = target
		}
		return next
	}

	target, ok := next.Targets[failedTarget]
	if ok {
		target.State = domain.InstallDegraded
		next.Targets[failedTarget] = target
	}
	return next
}

func (s Service) rollbackRemovedExisting(ctx context.Context, operationID string, record domain.InstallationRecord, removed []removedExistingTarget) ([]domain.TargetID, []string) {
	failed := make([]domain.TargetID, 0)
	warnings := make([]string, 0)
	for i := len(removed) - 1; i >= 0; i-- {
		item := removed[i]
		if item.Planned.Manifest == nil || item.Planned.Resolved == nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(item.Planned.TargetID), Action: "rollback", Status: "failed"})
			failed = append(failed, item.Planned.TargetID)
			warnings = append(warnings, "rollback install context missing for "+string(item.Planned.TargetID))
			continue
		}
		inspect, err := item.Planned.Adapter.Inspect(ctx, ports.InspectInput{IntegrationID: record.IntegrationID, Record: &record, Scope: record.Policy.Scope})
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(item.Planned.TargetID), Action: "rollback", Status: "failed"})
			failed = append(failed, item.Planned.TargetID)
			warnings = append(warnings, "rollback inspect failed for "+string(item.Planned.TargetID)+": "+err.Error())
			continue
		}
		plan, err := item.Planned.Adapter.PlanInstall(ctx, ports.PlanInstallInput{
			Manifest: *item.Planned.Manifest,
			Policy:   record.Policy,
			Inspect:  inspect,
		})
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(item.Planned.TargetID), Action: "rollback", Status: "failed"})
			failed = append(failed, item.Planned.TargetID)
			warnings = append(warnings, "rollback plan failed for "+string(item.Planned.TargetID)+": "+err.Error())
			continue
		}
		if _, err := s.validateEvidence(ctx, item.Planned.TargetID, plan.EvidenceKey); err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(item.Planned.TargetID), Action: "rollback", Status: "failed"})
			failed = append(failed, item.Planned.TargetID)
			warnings = append(warnings, "rollback evidence validation failed for "+string(item.Planned.TargetID)+": "+err.Error())
			continue
		}
		if _, err := item.Planned.Adapter.ApplyInstall(ctx, ports.ApplyInput{
			Plan:           plan,
			Manifest:       *item.Planned.Manifest,
			ResolvedSource: item.Planned.Resolved,
			Policy:         record.Policy,
			Inspect:        inspect,
			Record:         &record,
		}); err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(item.Planned.TargetID), Action: "rollback", Status: "failed"})
			failed = append(failed, item.Planned.TargetID)
			warnings = append(warnings, "rollback apply failed for "+string(item.Planned.TargetID)+": "+err.Error())
			continue
		}
		_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(item.Planned.TargetID), Action: "rollback", Status: "ok"})
	}
	return failed, warnings
}

func (s Service) verifyPostApply(ctx context.Context, integrationID string, policy domain.InstallPolicy, record *domain.InstallationRecord, adapter ports.TargetAdapter, action string) (ports.InspectResult, error) {
	inspect, err := adapter.Inspect(ctx, ports.InspectInput{
		IntegrationID: integrationID,
		Record:        record,
		Scope:         policy.Scope,
	})
	if err != nil {
		return ports.InspectResult{}, domain.NewError(domain.ErrMutationApply, "post-apply verify inspect failed", err)
	}
	switch action {
	case "add", "update_version", "repair_drift":
		if inspect.State == "" || inspect.State == domain.InstallRemoved {
			return inspect, domain.NewError(domain.ErrMutationApply, "post-apply verify did not observe an installed target state", nil)
		}
	case "enable_target":
		if inspect.State != domain.InstallInstalled {
			return inspect, domain.NewError(domain.ErrMutationApply, "post-apply verify did not observe an enabled installed target state", nil)
		}
	case "disable_target":
		if inspect.State != domain.InstallDisabled {
			return inspect, domain.NewError(domain.ErrMutationApply, "post-apply verify did not observe a disabled target state", nil)
		}
	case "remove_orphaned_target":
		if inspect.State != domain.InstallRemoved {
			return inspect, domain.NewError(domain.ErrMutationApply, "post-apply verify still observes the target after remove", nil)
		}
	}
	return inspect, nil
}

func markPlannedTargetDegraded(record *domain.InstallationRecord, target plannedExistingTarget) {
	if _, ok := record.Targets[target.TargetID]; ok {
		markTargetDegraded(record, target.TargetID)
		return
	}
	record.Targets[target.TargetID] = domain.TargetInstallation{
		TargetID:          target.TargetID,
		DeliveryKind:      target.Delivery.DeliveryKind,
		CapabilitySurface: append([]string(nil), target.Delivery.CapabilitySurface...),
		State:             domain.InstallDegraded,
		NativeRef:         target.Delivery.NativeRefHint,
		ActivationState:   target.Inspect.ActivationState,
		CatalogPolicy:     cloneCatalogPolicy(target.Inspect.CatalogPolicy),
		EnvironmentRestrictions: append([]domain.EnvironmentRestrictionCode(nil),
			target.Inspect.EnvironmentRestrictions...,
		),
		SourceAccessState: target.Inspect.SourceAccessState,
	}
}

func applyManifestMetadata(record *domain.InstallationRecord, manifest domain.IntegrationManifest, at string) {
	record.ResolvedVersion = manifest.Version
	record.ResolvedSourceRef = manifest.ResolvedRef
	record.SourceDigest = manifest.SourceDigest
	record.ManifestDigest = manifest.ManifestDigest
	record.LastCheckedAt = at
	record.LastUpdatedAt = at
}

func markTargetDegraded(record *domain.InstallationRecord, targetID domain.TargetID) {
	target, ok := record.Targets[targetID]
	if !ok {
		return
	}
	target.State = domain.InstallDegraded
	record.Targets[targetID] = target
}

func provisionalRecordForExisting(record domain.InstallationRecord, target plannedExistingTarget, result ports.ApplyResult) domain.InstallationRecord {
	next := cloneInstallationRecord(record)
	if next.Targets == nil {
		next.Targets = map[domain.TargetID]domain.TargetInstallation{}
	}
	next.Targets[target.TargetID] = targetInstallationFromExisting(target, result, ports.InspectResult{})
	if target.Manifest != nil {
		applyManifestMetadata(&next, *target.Manifest, record.LastUpdatedAt)
	}
	return next
}
