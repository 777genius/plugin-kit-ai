package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func (s Service) applyUpdateExisting(ctx context.Context, record domain.InstallationRecord, planned []plannedExistingTarget) (domain.Report, error) {
	if len(planned) == 0 {
		return domain.Report{}, domain.NewError(domain.ErrMutationApply, "update requires at least one planned target", nil)
	}
	defer cleanupPlannedExisting(planned)
	if err := ensureExistingTargetsNotBlocking(planned); err != nil {
		return domain.Report{}, err
	}
	if err := ensureExistingTargetsResolved(planned, "update"); err != nil {
		return domain.Report{}, err
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
	runtime := existingUpdateRuntime{
		operationID:   operationID,
		startedAt:     startedAt,
		state:         state,
		nextRecord:    cloneInstallationRecord(items),
		reportTargets: make([]domain.TargetReport, 0, len(planned)),
	}
	for _, target := range planned {
		persisted, err := s.applyUpdatedExistingTarget(ctx, record, target, &runtime)
		if err != nil {
			committed = persisted
			return domain.Report{}, err
		}
	}

	runtime.state.Installations = upsertInstallation(runtime.state.Installations, runtime.nextRecord)
	if err := s.StateStore.Save(ctx, runtime.state); err != nil {
		return domain.Report{}, err
	}
	if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: "state", Action: "persist_state", Status: "ok"}); err != nil {
		return domain.Report{}, err
	}
	if err := s.Journal.Finish(ctx, operationID, "committed"); err != nil {
		return domain.Report{}, err
	}
	committed = true
	sort.Slice(runtime.reportTargets, func(i, j int) bool { return runtime.reportTargets[i].TargetID < runtime.reportTargets[j].TargetID })
	return domain.Report{
		OperationID: operationID,
		Summary:     summaryForExisting("update_version", record.IntegrationID),
		Targets:     runtime.reportTargets,
	}, nil
}
