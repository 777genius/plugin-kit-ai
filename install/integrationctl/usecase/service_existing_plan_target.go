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
	target, err := loadExistingTargetRecord(record, targetID)
	if err != nil {
		return plannedExistingTarget{}, err
	}
	adapter, err := s.loadExistingTargetAdapter(targetID)
	if err != nil {
		return plannedExistingTarget{}, err
	}
	inspect, err := inspectExistingTargetAdapter(ctx, adapter, record)
	if err != nil {
		return plannedExistingTarget{}, err
	}
	base := plannedExistingTarget{
		TargetID: targetID,
		Current:  target,
		Delivery: existingTargetDelivery(targetID, target),
		Adapter:  adapter,
		Inspect:  inspect,
	}
	return base, nil
}

func loadExistingTargetRecord(record domain.InstallationRecord, targetID domain.TargetID) (domain.TargetInstallation, error) {
	target, ok := record.Targets[targetID]
	if !ok {
		return domain.TargetInstallation{}, domain.NewError(domain.ErrStateConflict, "target missing from installation record: "+string(targetID), nil)
	}
	return target, nil
}

func (s Service) loadExistingTargetAdapter(targetID domain.TargetID) (ports.TargetAdapter, error) {
	adapter, ok := s.Adapters[targetID]
	if !ok {
		return nil, domain.NewError(domain.ErrUnsupportedTarget, "adapter not registered for "+string(targetID), nil)
	}
	return adapter, nil
}

func inspectExistingTargetAdapter(ctx context.Context, adapter ports.TargetAdapter, record domain.InstallationRecord) (ports.InspectResult, error) {
	return adapter.Inspect(ctx, ports.InspectInput{IntegrationID: record.IntegrationID, Record: &record, Scope: record.Policy.Scope})
}

func existingTargetDelivery(targetID domain.TargetID, target domain.TargetInstallation) domain.Delivery {
	return domain.Delivery{
		TargetID:      targetID,
		DeliveryKind:  target.DeliveryKind,
		NativeRefHint: target.NativeRef,
	}
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
	return finalizeExistingRemovalPlan(item, plan, sharedResolved, sharedManifest), nil
}

func finalizeExistingRemovalPlan(item plannedExistingTarget, plan ports.AdapterPlan, sharedResolved *ports.ResolvedSource, sharedManifest *domain.IntegrationManifest) plannedExistingTarget {
	item.Plan = plan
	item.Manifest = cloneManifestPtr(sharedManifest)
	item.Resolved = cloneResolvedPtr(sharedResolved)
	item.Report = toTargetReport(item.Delivery, item.Inspect, plan)
	return item
}

func (s Service) planExistingToggle(ctx context.Context, record domain.InstallationRecord, item plannedExistingTarget, enable bool) (plannedExistingTarget, error) {
	toggle, err := existingToggleAdapter(item, enable)
	if err != nil {
		return plannedExistingTarget{}, err
	}
	var (
		plan ports.AdapterPlan
		planErr error
	)
	if enable {
		plan, planErr = toggle.PlanEnable(ctx, ports.PlanToggleInput{Record: record, Inspect: item.Inspect})
	} else {
		plan, planErr = toggle.PlanDisable(ctx, ports.PlanToggleInput{Record: record, Inspect: item.Inspect})
	}
	if planErr != nil {
		return plannedExistingTarget{}, planErr
	}
	if _, err := s.validateEvidence(ctx, item.TargetID, plan.EvidenceKey); err != nil {
		return plannedExistingTarget{}, err
	}
	return finalizeExistingTogglePlan(item, plan), nil
}

func existingToggleAdapter(item plannedExistingTarget, enable bool) (ports.ToggleTargetAdapter, error) {
	toggle, ok := item.Adapter.(ports.ToggleTargetAdapter)
	if !ok {
		return nil, domain.NewError(domain.ErrUnsupportedTarget, "target "+string(item.TargetID)+" does not support "+existingToggleAction(enable), nil)
	}
	return toggle, nil
}

func existingToggleAction(enable bool) string {
	if enable {
		return "enable"
	}
	return "disable"
}

func finalizeExistingTogglePlan(item plannedExistingTarget, plan ports.AdapterPlan) plannedExistingTarget {
	item.Plan = plan
	item.Report = toTargetReport(item.Delivery, item.Inspect, plan)
	return item
}

func (s Service) planExistingMutation(ctx context.Context, record domain.InstallationRecord, item plannedExistingTarget, sharedResolved *ports.ResolvedSource, sharedManifest *domain.IntegrationManifest, repair bool) (plannedExistingTarget, error) {
	resolved, manifest, err := s.resolveExistingMutationSource(ctx, record, sharedResolved, sharedManifest)
	if err != nil {
		return plannedExistingTarget{}, err
	}
	nextDelivery, err := requireExistingMutationDelivery(manifest, item.TargetID)
	if err != nil {
		return plannedExistingTarget{}, err
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
	return finalizeExistingMutationPlan(item, *nextDelivery, plan, resolved, manifest), nil
}

func (s Service) resolveExistingMutationSource(ctx context.Context, record domain.InstallationRecord, sharedResolved *ports.ResolvedSource, sharedManifest *domain.IntegrationManifest) (ports.ResolvedSource, domain.IntegrationManifest, error) {
	if sharedResolved != nil && sharedManifest != nil {
		return *sharedResolved, *sharedManifest, nil
	}
	return s.resolveCurrentSourceManifest(ctx, record)
}

func requireExistingMutationDelivery(manifest domain.IntegrationManifest, targetID domain.TargetID) (*domain.Delivery, error) {
	nextDelivery := findDelivery(manifest.Deliveries, targetID)
	if nextDelivery == nil {
		return nil, domain.NewError(domain.ErrUnsupportedTarget, "updated manifest no longer exposes target "+string(targetID), nil)
	}
	return nextDelivery, nil
}

func finalizeExistingMutationPlan(item plannedExistingTarget, delivery domain.Delivery, plan ports.AdapterPlan, resolved ports.ResolvedSource, manifest domain.IntegrationManifest) plannedExistingTarget {
	item.Delivery = delivery
	item.Plan = plan
	item.Manifest = &manifest
	item.Resolved = &resolved
	item.Report = toTargetReport(delivery, item.Inspect, plan)
	return item
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
