package usecase

import (
	"context"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type existingUpdateRuntime struct {
	operationID   string
	startedAt     string
	state         ports.StateFile
	nextRecord    domain.InstallationRecord
	reportTargets []domain.TargetReport
}

func (s Service) applyUpdatedExistingTarget(ctx context.Context, record domain.InstallationRecord, target plannedExistingTarget, runtime *existingUpdateRuntime) (bool, error) {
	if err := s.appendExistingInspectPlanSteps(ctx, runtime.operationID, target.TargetID); err != nil {
		return false, err
	}
	applyResult, err := applyExistingUpdateMutation(ctx, record, target)
	if err != nil {
		_ = s.Journal.AppendStep(ctx, runtime.operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "failed"})
		return s.finishUpdateExistingFailure(ctx, runtime, target, err, "update failed after partial progress; degraded state persisted")
	}
	if err := s.Journal.AppendStep(ctx, runtime.operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "ok"}); err != nil {
		return false, err
	}
	verified, err := s.verifyUpdatedExistingTarget(ctx, record, target, applyResult)
	if err != nil {
		_ = s.Journal.AppendStep(ctx, runtime.operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "failed"})
		return s.finishUpdateExistingFailure(ctx, runtime, target, err, "update verification failed after partial progress; degraded state persisted")
	}
	if err := s.Journal.AppendStep(ctx, runtime.operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "ok"}); err != nil {
		return false, err
	}
	runtime.nextRecord.Targets[target.TargetID] = targetInstallationFromExisting(target, applyResult, verified)
	applyManifestMetadata(&runtime.nextRecord, *target.Manifest, runtime.startedAt)
	runtime.reportTargets = append(runtime.reportTargets, toAppliedTargetReport(target.Delivery, target.Inspect, verified, target.Plan, applyResult))
	return false, nil
}

func applyExistingUpdateMutation(ctx context.Context, record domain.InstallationRecord, target plannedExistingTarget) (ports.ApplyResult, error) {
	input := ports.ApplyInput{
		Plan:           target.Plan,
		Manifest:       *target.Manifest,
		ResolvedSource: target.Resolved,
		Policy:         record.Policy,
		Inspect:        target.Inspect,
		Record:         &record,
	}
	if target.Adopted {
		return target.Adapter.ApplyInstall(ctx, input)
	}
	return target.Adapter.ApplyUpdate(ctx, input)
}

func (s Service) verifyUpdatedExistingTarget(ctx context.Context, record domain.InstallationRecord, target plannedExistingTarget, applyResult ports.ApplyResult) (ports.InspectResult, error) {
	verifyRecord := provisionalRecordForExisting(record, target, applyResult)
	return s.verifyPostApply(ctx, record.IntegrationID, record.Policy, &verifyRecord, target.Adapter, "update_version")
}
