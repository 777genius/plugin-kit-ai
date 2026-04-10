package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (s Service) applyRemoveExisting(ctx context.Context, record domain.InstallationRecord, planned []plannedExistingTarget) (domain.Report, error) {
	if len(planned) == 0 {
		return domain.Report{}, domain.NewError(domain.ErrMutationApply, "remove requires at least one planned target", nil)
	}
	defer cleanupPlannedExisting(planned)
	if err := ensureExistingTargetsNotBlocking(planned); err != nil {
		return domain.Report{}, err
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
		if err := s.appendExistingInspectPlanSteps(ctx, operationID, target.TargetID); err != nil {
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
			failureErr, done, failureState, finishErr := s.finishRemoveExistingFailure(ctx, operationID, state, record, startedAt, target.TargetID, removed, err, "remove failed and removed targets were rolled back", "remove failed and rollback was incomplete; degraded state persisted")
			if finishErr != nil {
				return domain.Report{}, finishErr
			}
			if done {
				state = failureState
				committed = true
				return domain.Report{}, failureErr
			}
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		verified, err := s.verifyPostApply(ctx, record.IntegrationID, record.Policy, &record, target.Adapter, "remove_orphaned_target")
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "failed"})
			failureRemoved := appendRemovedExisting(removed, target, applyResult)
			failureErr, done, failureState, finishErr := s.finishRemoveExistingFailure(ctx, operationID, state, record, startedAt, target.TargetID, failureRemoved, err, "remove verification failed and removed targets were rolled back", "remove verification failed and rollback was incomplete; degraded state persisted")
			if finishErr != nil {
				return domain.Report{}, finishErr
			}
			if done {
				state = failureState
				committed = true
				return domain.Report{}, failureErr
			}
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		removed = appendRemovedExisting(removed, target, applyResult)
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
