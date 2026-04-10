package usecase

import (
	"context"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func ensureExistingTargetsNotBlocking(planned []plannedExistingTarget) error {
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
	}
	return nil
}

func ensureExistingTargetsResolved(planned []plannedExistingTarget, action string) error {
	for _, target := range planned {
		if target.Manifest == nil || target.Resolved == nil {
			return domain.NewError(domain.ErrMutationApply, action+" requires resolved source and manifest for target "+string(target.TargetID), nil)
		}
	}
	return nil
}

func (s Service) appendExistingInspectPlanSteps(ctx context.Context, operationID string, targetID domain.TargetID) error {
	if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(targetID), Action: "inspect", Status: "ok"}); err != nil {
		return err
	}
	return s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(targetID), Action: "plan", Status: "ok"})
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
