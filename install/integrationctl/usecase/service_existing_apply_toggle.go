package usecase

import (
	"context"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (s Service) applyToggleExisting(ctx context.Context, record domain.InstallationRecord, action string, planned []plannedExistingTarget) (domain.Report, error) {
	target, err := validateSingleExistingToggleTarget(planned)
	if err != nil {
		return domain.Report{}, err
	}
	defer cleanupPlannedExisting(planned)
	if err := ensureExistingTogglePlanAllowed(target); err != nil {
		return domain.Report{}, err
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

	if err := s.appendExistingPlanJournal(ctx, operationID, target); err != nil {
		return domain.Report{}, err
	}

	applyResult, err := s.applyExistingToggleMutation(ctx, action, record, target)
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

	if err := s.persistExistingToggleState(ctx, record, action, target, applyResult, verified, startedAt); err != nil {
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

func validateSingleExistingToggleTarget(planned []plannedExistingTarget) (plannedExistingTarget, error) {
	if len(planned) != 1 {
		return plannedExistingTarget{}, domain.NewError(domain.ErrMutationApply, "non-dry-run existing lifecycle currently supports one target at a time until rollback is implemented", nil)
	}
	return planned[0], nil
}

func ensureExistingTogglePlanAllowed(target plannedExistingTarget) error {
	if target.Plan.Blocking {
		return domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
	}
	return nil
}

func (s Service) appendExistingPlanJournal(ctx context.Context, operationID string, target plannedExistingTarget) error {
	if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "inspect", Status: "ok"}); err != nil {
		return err
	}
	if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "plan", Status: "ok"}); err != nil {
		return err
	}
	return nil
}

func (s Service) applyExistingToggleMutation(ctx context.Context, action string, record domain.InstallationRecord, target plannedExistingTarget) (ports.ApplyResult, error) {
	toggle := target.Adapter.(ports.ToggleTargetAdapter)
	input := buildExistingToggleApplyInput(record, target)
	apply, err := existingToggleMutationApply(toggle, action)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	return apply(ctx, input)
}

func buildExistingToggleApplyInput(record domain.InstallationRecord, target plannedExistingTarget) ports.ApplyInput {
	return ports.ApplyInput{
		Plan:    target.Plan,
		Policy:  record.Policy,
		Inspect: target.Inspect,
		Record:  &record,
	}
}

func existingToggleMutationApply(toggle ports.ToggleTargetAdapter, action string) (func(context.Context, ports.ApplyInput) (ports.ApplyResult, error), error) {
	switch action {
	case "enable_target":
		return toggle.ApplyEnable, nil
	case "disable_target":
		return toggle.ApplyDisable, nil
	default:
		return nil, domain.NewError(domain.ErrUsage, "unsupported existing lifecycle action "+action, nil)
	}
}

func (s Service) persistExistingToggleState(ctx context.Context, record domain.InstallationRecord, action string, target plannedExistingTarget, applyResult ports.ApplyResult, verified ports.InspectResult, startedAt string) error {
	state, err := s.StateStore.Load(ctx)
	if err != nil {
		return err
	}
	items, found := findInstallationMutable(state.Installations, record.IntegrationID)
	if !found {
		return domain.NewError(domain.ErrStateConflict, "integration disappeared from state during apply: "+record.IntegrationID, nil)
	}
	nextRecord := items
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
	return s.StateStore.Save(ctx, state)
}
