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
	workspaceRoot := s.workspaceRootForPolicy(policy)
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
		if err := s.recordAddTargetPlanning(ctx, operationID, target); err != nil {
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
			report, failErr := s.handleFailedAdd(ctx, operationID, &state, manifest, policy, workspaceRoot, startedAt, applied, "install failed and rollback was incomplete; degraded state persisted", "install failed and applied targets were rolled back", err)
			committed = true
			return report, failErr
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		verifyRecord := provisionalRecordForAdd(manifest, policy, workspaceRoot, target, applyResult)
		verified, err := s.verifyPostApply(ctx, manifest.IntegrationID, policy, &verifyRecord, target.Adapter, "add")
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "failed"})
			report, failErr := s.handleFailedAdd(ctx, operationID, &state, manifest, policy, workspaceRoot, startedAt, append(applied, appliedTargetInstall{Planned: target, Result: applyResult}), "install verification failed and rollback was incomplete; degraded state persisted", "install verification failed and applied targets were rolled back", err)
			committed = true
			return report, failErr
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		applied = append(applied, appliedTargetInstall{Planned: target, Result: applyResult, Verify: verified})
		reportTargets = append(reportTargets, toAppliedTargetReport(target.Delivery, target.Inspect, verified, target.Plan, applyResult))
	}

	state.Installations = upsertInstallation(state.Installations, installationRecordFromApplied(manifest, policy, workspaceRoot, startedAt, applied))
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

func (s Service) recordAddTargetPlanning(ctx context.Context, operationID string, target plannedTargetInstall) error {
	if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "inspect", Status: "ok"}); err != nil {
		return err
	}
	return s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "plan", Status: "ok"})
}

func (s Service) handleFailedAdd(ctx context.Context, operationID string, state *ports.StateFile, manifest domain.IntegrationManifest, policy domain.InstallPolicy, workspaceRoot string, startedAt string, applied []appliedTargetInstall, degradedMessage string, rolledBackMessage string, cause error) (domain.Report, error) {
	rollbackFailed, rollbackWarnings := s.rollbackAppliedAdd(ctx, operationID, manifest, policy, workspaceRoot, startedAt, applied)
	if len(rollbackFailed) > 0 {
		state.Installations = upsertInstallation(state.Installations, degradedRecordFromApplied(manifest, policy, workspaceRoot, startedAt, rollbackFailed))
		if saveErr := s.StateStore.Save(ctx, *state); saveErr != nil {
			return domain.Report{}, saveErr
		}
		if stepErr := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: "state", Action: "persist_degraded_state", Status: "ok"}); stepErr != nil {
			return domain.Report{}, stepErr
		}
		if finishErr := s.Journal.Finish(ctx, operationID, "degraded"); finishErr != nil {
			return domain.Report{}, finishErr
		}
		return domain.Report{}, domain.NewError(domain.ErrMutationApply, appendRollbackWarnings(degradedMessage, rollbackWarnings), cause)
	}
	if finishErr := s.Journal.Finish(ctx, operationID, "rolled_back"); finishErr != nil {
		return domain.Report{}, finishErr
	}
	return domain.Report{}, domain.NewError(domain.ErrMutationApply, appendRollbackWarnings(rolledBackMessage, rollbackWarnings), cause)
}

func appendRollbackWarnings(message string, warnings []string) string {
	if len(warnings) == 0 {
		return message
	}
	return message + ": " + strings.Join(warnings, "; ")
}

func (s Service) rollbackAppliedAdd(ctx context.Context, operationID string, manifest domain.IntegrationManifest, policy domain.InstallPolicy, workspaceRoot string, startedAt string, applied []appliedTargetInstall) ([]appliedTargetInstall, []string) {
	failed := make([]appliedTargetInstall, 0)
	warnings := make([]string, 0)
	for i := len(applied) - 1; i >= 0; i-- {
		item := applied[i]
		record := installationRecordFromApplied(manifest, policy, workspaceRoot, startedAt, []appliedTargetInstall{item})
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
