package usecase

import (
	"context"
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
