package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (s Service) applyRepairExisting(ctx context.Context, record domain.InstallationRecord, planned []plannedExistingTarget) (domain.Report, error) {
	if len(planned) == 0 {
		return domain.Report{}, domain.NewError(domain.ErrMutationApply, "repair requires at least one planned target", nil)
	}
	defer cleanupPlannedExisting(planned)
	if err := ensureExistingTargetsNotBlocking(planned); err != nil {
		return domain.Report{}, err
	}
	if err := ensureExistingTargetsResolved(planned, "repair"); err != nil {
		return domain.Report{}, err
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
		if err := s.appendExistingInspectPlanSteps(ctx, operationID, target.TargetID); err != nil {
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
