package usecase

import (
	"context"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (s Service) planExistingTarget(ctx context.Context, record domain.InstallationRecord, targetID domain.TargetID, action string, sharedResolved *ports.ResolvedSource, sharedManifest *domain.IntegrationManifest) (plannedExistingTarget, error) {
	base, err := s.loadExistingTargetBase(ctx, record, targetID)
	if err != nil {
		return plannedExistingTarget{}, err
	}
	return s.dispatchExistingTargetPlan(ctx, record, action, base, sharedResolved, sharedManifest)
}

func (s Service) loadExistingTargetBase(ctx context.Context, record domain.InstallationRecord, targetID domain.TargetID) (plannedExistingTarget, error) {
	target, ok := record.Targets[targetID]
	if !ok {
		return plannedExistingTarget{}, domain.NewError(domain.ErrStateConflict, "target missing from installation record: "+string(targetID), nil)
	}
	adapter, ok := s.Adapters[targetID]
	if !ok {
		return plannedExistingTarget{}, domain.NewError(domain.ErrUnsupportedTarget, "adapter not registered for "+string(targetID), nil)
	}
	inspect, err := adapter.Inspect(ctx, ports.InspectInput{IntegrationID: record.IntegrationID, Record: &record, Scope: record.Policy.Scope})
	if err != nil {
		return plannedExistingTarget{}, err
	}
	delivery := domain.Delivery{
		TargetID:      targetID,
		DeliveryKind:  target.DeliveryKind,
		NativeRefHint: target.NativeRef,
	}
	base := plannedExistingTarget{
		TargetID: targetID,
		Current:  target,
		Delivery: delivery,
		Adapter:  adapter,
		Inspect:  inspect,
	}
	return base, nil
}

func (s Service) dispatchExistingTargetPlan(
	ctx context.Context,
	record domain.InstallationRecord,
	action string,
	base plannedExistingTarget,
	sharedResolved *ports.ResolvedSource,
	sharedManifest *domain.IntegrationManifest,
) (plannedExistingTarget, error) {
	switch action {
	case "remove_orphaned_target":
		return s.planExistingRemoval(ctx, record, base, sharedResolved, sharedManifest)
	case "enable_target":
		return s.planExistingToggle(ctx, record, base, true)
	case "disable_target":
		return s.planExistingToggle(ctx, record, base, false)
	case "update_version":
		return s.planExistingMutation(ctx, record, base, sharedResolved, sharedManifest, false)
	case "repair_drift":
		return s.planExistingMutation(ctx, record, base, sharedResolved, sharedManifest, true)
	default:
		return plannedExistingTarget{}, domain.NewError(domain.ErrUsage, "unsupported existing lifecycle action "+action, nil)
	}
}

func (s Service) planExistingRemoval(ctx context.Context, record domain.InstallationRecord, item plannedExistingTarget, sharedResolved *ports.ResolvedSource, sharedManifest *domain.IntegrationManifest) (plannedExistingTarget, error) {
	plan, err := item.Adapter.PlanRemove(ctx, ports.PlanRemoveInput{Record: record, Inspect: item.Inspect})
	if err != nil {
		return plannedExistingTarget{}, err
	}
	if _, err := s.validateEvidence(ctx, item.TargetID, plan.EvidenceKey); err != nil {
		return plannedExistingTarget{}, err
	}
	item.Plan = plan
	item.Manifest = cloneManifestPtr(sharedManifest)
	item.Resolved = cloneResolvedPtr(sharedResolved)
	item.Report = toTargetReport(item.Delivery, item.Inspect, plan)
	return item, nil
}

func (s Service) planExistingToggle(ctx context.Context, record domain.InstallationRecord, item plannedExistingTarget, enable bool) (plannedExistingTarget, error) {
	toggle, ok := item.Adapter.(ports.ToggleTargetAdapter)
	if !ok {
		action := "disable"
		if enable {
			action = "enable"
		}
		return plannedExistingTarget{}, domain.NewError(domain.ErrUnsupportedTarget, "target "+string(item.TargetID)+" does not support "+action, nil)
	}
	var (
		plan ports.AdapterPlan
		err  error
	)
	if enable {
		plan, err = toggle.PlanEnable(ctx, ports.PlanToggleInput{Record: record, Inspect: item.Inspect})
	} else {
		plan, err = toggle.PlanDisable(ctx, ports.PlanToggleInput{Record: record, Inspect: item.Inspect})
	}
	if err != nil {
		return plannedExistingTarget{}, err
	}
	if _, err := s.validateEvidence(ctx, item.TargetID, plan.EvidenceKey); err != nil {
		return plannedExistingTarget{}, err
	}
	item.Plan = plan
	item.Report = toTargetReport(item.Delivery, item.Inspect, plan)
	return item, nil
}

func (s Service) planExistingMutation(ctx context.Context, record domain.InstallationRecord, item plannedExistingTarget, sharedResolved *ports.ResolvedSource, sharedManifest *domain.IntegrationManifest, repair bool) (plannedExistingTarget, error) {
	resolved, manifest, err := s.resolveExistingMutationSource(ctx, record, sharedResolved, sharedManifest)
	if err != nil {
		return plannedExistingTarget{}, err
	}
	nextDelivery := findDelivery(manifest.Deliveries, item.TargetID)
	if nextDelivery == nil {
		return plannedExistingTarget{}, domain.NewError(domain.ErrUnsupportedTarget, "updated manifest no longer exposes target "+string(item.TargetID), nil)
	}
	plan, err := item.Adapter.PlanUpdate(ctx, ports.PlanUpdateInput{
		CurrentRecord: record,
		NextManifest:  manifest,
		Inspect:       item.Inspect,
	})
	if err != nil {
		return plannedExistingTarget{}, err
	}
	if repair {
		plan.ActionClass = "repair_drift"
		plan.Summary = "Repair managed drift for target " + string(item.TargetID)
	}
	if _, err := s.validateEvidence(ctx, item.TargetID, plan.EvidenceKey); err != nil {
		return plannedExistingTarget{}, err
	}
	item.Delivery = *nextDelivery
	item.Plan = plan
	item.Manifest = &manifest
	item.Resolved = &resolved
	item.Report = toTargetReport(*nextDelivery, item.Inspect, plan)
	return item, nil
}

func (s Service) resolveExistingMutationSource(ctx context.Context, record domain.InstallationRecord, sharedResolved *ports.ResolvedSource, sharedManifest *domain.IntegrationManifest) (ports.ResolvedSource, domain.IntegrationManifest, error) {
	if sharedResolved != nil && sharedManifest != nil {
		return *sharedResolved, *sharedManifest, nil
	}
	return s.resolveCurrentSourceManifest(ctx, record)
}

func cloneManifestPtr(manifest *domain.IntegrationManifest) *domain.IntegrationManifest {
	if manifest == nil {
		return nil
	}
	manifestCopy := *manifest
	return &manifestCopy
}

func cloneResolvedPtr(resolved *ports.ResolvedSource) *ports.ResolvedSource {
	if resolved == nil {
		return nil
	}
	resolvedCopy := *resolved
	return &resolvedCopy
}
