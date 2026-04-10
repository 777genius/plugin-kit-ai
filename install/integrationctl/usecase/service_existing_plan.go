package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type plannedExistingTarget struct {
	TargetID domain.TargetID
	Current  domain.TargetInstallation
	Delivery domain.Delivery
	Adapter  ports.TargetAdapter
	Inspect  ports.InspectResult
	Plan     ports.AdapterPlan
	Manifest *domain.IntegrationManifest
	Resolved *ports.ResolvedSource
	Report   domain.TargetReport
	Adopted  bool
}

func (s Service) executeExisting(ctx context.Context, in NamedDryRunInput, action string) (domain.Report, error) {
	return s.planExisting(ctx, in, action)
}

func (s Service) planExisting(ctx context.Context, in NamedDryRunInput, action string) (domain.Report, error) {
	state, err := s.StateStore.Load(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	record, ok := findInstallation(state.Installations, in.Name)
	if !ok {
		return domain.Report{}, domain.NewError(domain.ErrStateConflict, "integration not found in state: "+in.Name, nil)
	}
	planned := make([]plannedExistingTarget, 0, len(record.Targets))
	report := domain.Report{
		OperationID: operationID("plan_"+action, record.IntegrationID, s.now()),
		Summary:     fmt.Sprintf("Dry-run %s plan for %q.", action, record.IntegrationID),
	}
	var sharedResolved *ports.ResolvedSource
	var sharedManifest *domain.IntegrationManifest
	if action == "update_version" || ((action == "remove_orphaned_target" || action == "repair_drift") && !in.DryRun) {
		resolved, manifest, err := s.resolveCurrentSourceManifest(ctx, record)
		if err != nil {
			return domain.Report{}, err
		}
		sharedResolved = &resolved
		sharedManifest = &manifest
		defer cleanupResolvedSource(resolved)
	}
	selectedTargetIDs, err := s.selectExistingTargets(record, in.Target, action)
	if err != nil {
		return domain.Report{}, err
	}
	for _, targetID := range selectedTargetIDs {
		item, err := s.planExistingTarget(ctx, record, targetID, action, sharedResolved, sharedManifest)
		if err != nil {
			cleanupPlannedExisting(planned)
			return domain.Report{}, err
		}
		planned = append(planned, item)
		report.Targets = append(report.Targets, item.Report)
	}
	if action == "update_version" && sharedResolved != nil && sharedManifest != nil {
		adopted, warnings, err := s.planAdoptedUpdateTargets(ctx, record, *sharedManifest, *sharedResolved)
		if err != nil {
			cleanupPlannedExisting(planned)
			return domain.Report{}, err
		}
		planned = append(planned, adopted...)
		for _, item := range adopted {
			report.Targets = append(report.Targets, item.Report)
		}
		report.Warnings = append(report.Warnings, warnings...)
	}
	sort.Slice(report.Targets, func(i, j int) bool { return report.Targets[i].TargetID < report.Targets[j].TargetID })
	if in.DryRun {
		cleanupPlannedExisting(planned)
		return report, nil
	}
	return s.applyExisting(ctx, record, action, planned)
}

func (s Service) selectExistingTargets(record domain.InstallationRecord, requestedTarget string, action string) ([]domain.TargetID, error) {
	if strings.TrimSpace(requestedTarget) != "" {
		targetID := domain.TargetID(strings.TrimSpace(requestedTarget))
		if _, ok := record.Targets[targetID]; !ok {
			return nil, domain.NewError(domain.ErrStateConflict, "target missing from installation record: "+string(targetID), nil)
		}
		return []domain.TargetID{targetID}, nil
	}
	if action != "enable_target" && action != "disable_target" {
		return sortedTargets(record.Targets), nil
	}
	var togglable []domain.TargetID
	for _, targetID := range sortedTargets(record.Targets) {
		adapter, ok := s.Adapters[targetID]
		if !ok {
			continue
		}
		if _, ok := adapter.(ports.ToggleTargetAdapter); ok {
			togglable = append(togglable, targetID)
		}
	}
	if len(togglable) == 0 {
		return nil, domain.NewError(domain.ErrUnsupportedTarget, "no installed targets for "+record.IntegrationID+" support "+strings.TrimSuffix(action, "_target"), nil)
	}
	if len(togglable) > 1 {
		return nil, domain.NewError(domain.ErrUsage, "multiple installed targets support "+strings.TrimSuffix(action, "_target")+"; rerun with --target", nil)
	}
	return togglable, nil
}

func (s Service) planExistingTarget(ctx context.Context, record domain.InstallationRecord, targetID domain.TargetID, action string, sharedResolved *ports.ResolvedSource, sharedManifest *domain.IntegrationManifest) (plannedExistingTarget, error) {
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
	switch action {
	case "remove_orphaned_target":
		plan, err := adapter.PlanRemove(ctx, ports.PlanRemoveInput{Record: record, Inspect: inspect})
		if err != nil {
			return plannedExistingTarget{}, err
		}
		if _, err := s.validateEvidence(ctx, targetID, plan.EvidenceKey); err != nil {
			return plannedExistingTarget{}, err
		}
		var manifest *domain.IntegrationManifest
		var resolved *ports.ResolvedSource
		if sharedManifest != nil {
			manifestCopy := *sharedManifest
			manifest = &manifestCopy
		}
		if sharedResolved != nil {
			resolvedCopy := *sharedResolved
			resolved = &resolvedCopy
		}
		return plannedExistingTarget{
			TargetID: targetID,
			Current:  target,
			Delivery: delivery,
			Adapter:  adapter,
			Inspect:  inspect,
			Plan:     plan,
			Manifest: manifest,
			Resolved: resolved,
			Report:   toTargetReport(delivery, inspect, plan),
		}, nil
	case "enable_target":
		toggle, ok := adapter.(ports.ToggleTargetAdapter)
		if !ok {
			return plannedExistingTarget{}, domain.NewError(domain.ErrUnsupportedTarget, "target "+string(targetID)+" does not support enable", nil)
		}
		plan, err := toggle.PlanEnable(ctx, ports.PlanToggleInput{Record: record, Inspect: inspect})
		if err != nil {
			return plannedExistingTarget{}, err
		}
		if _, err := s.validateEvidence(ctx, targetID, plan.EvidenceKey); err != nil {
			return plannedExistingTarget{}, err
		}
		return plannedExistingTarget{
			TargetID: targetID,
			Current:  target,
			Delivery: delivery,
			Adapter:  adapter,
			Inspect:  inspect,
			Plan:     plan,
			Report:   toTargetReport(delivery, inspect, plan),
		}, nil
	case "disable_target":
		toggle, ok := adapter.(ports.ToggleTargetAdapter)
		if !ok {
			return plannedExistingTarget{}, domain.NewError(domain.ErrUnsupportedTarget, "target "+string(targetID)+" does not support disable", nil)
		}
		plan, err := toggle.PlanDisable(ctx, ports.PlanToggleInput{Record: record, Inspect: inspect})
		if err != nil {
			return plannedExistingTarget{}, err
		}
		if _, err := s.validateEvidence(ctx, targetID, plan.EvidenceKey); err != nil {
			return plannedExistingTarget{}, err
		}
		return plannedExistingTarget{
			TargetID: targetID,
			Current:  target,
			Delivery: delivery,
			Adapter:  adapter,
			Inspect:  inspect,
			Plan:     plan,
			Report:   toTargetReport(delivery, inspect, plan),
		}, nil
	case "update_version":
		var resolved ports.ResolvedSource
		var manifest domain.IntegrationManifest
		if sharedResolved != nil && sharedManifest != nil {
			resolved = *sharedResolved
			manifest = *sharedManifest
		} else {
			var err error
			resolved, manifest, err = s.resolveCurrentSourceManifest(ctx, record)
			if err != nil {
				return plannedExistingTarget{}, err
			}
		}
		nextDelivery := findDelivery(manifest.Deliveries, targetID)
		if nextDelivery == nil {
			return plannedExistingTarget{}, domain.NewError(domain.ErrUnsupportedTarget, "updated manifest no longer exposes target "+string(targetID), nil)
		}
		plan, err := adapter.PlanUpdate(ctx, ports.PlanUpdateInput{
			CurrentRecord: record,
			NextManifest:  manifest,
			Inspect:       inspect,
		})
		if err != nil {
			return plannedExistingTarget{}, err
		}
		if _, err := s.validateEvidence(ctx, targetID, plan.EvidenceKey); err != nil {
			return plannedExistingTarget{}, err
		}
		return plannedExistingTarget{
			TargetID: targetID,
			Current:  target,
			Delivery: *nextDelivery,
			Adapter:  adapter,
			Inspect:  inspect,
			Plan:     plan,
			Manifest: &manifest,
			Resolved: &resolved,
			Report:   toTargetReport(*nextDelivery, inspect, plan),
		}, nil
	case "repair_drift":
		var resolved ports.ResolvedSource
		var manifest domain.IntegrationManifest
		if sharedResolved != nil && sharedManifest != nil {
			resolved = *sharedResolved
			manifest = *sharedManifest
		} else {
			var err error
			resolved, manifest, err = s.resolveCurrentSourceManifest(ctx, record)
			if err != nil {
				return plannedExistingTarget{}, err
			}
		}
		nextDelivery := findDelivery(manifest.Deliveries, targetID)
		if nextDelivery == nil {
			return plannedExistingTarget{}, domain.NewError(domain.ErrUnsupportedTarget, "updated manifest no longer exposes target "+string(targetID), nil)
		}
		plan, err := adapter.PlanUpdate(ctx, ports.PlanUpdateInput{
			CurrentRecord: record,
			NextManifest:  manifest,
			Inspect:       inspect,
		})
		if err != nil {
			return plannedExistingTarget{}, err
		}
		plan.ActionClass = "repair_drift"
		plan.Summary = "Repair managed drift for target " + string(targetID)
		if _, err := s.validateEvidence(ctx, targetID, plan.EvidenceKey); err != nil {
			return plannedExistingTarget{}, err
		}
		return plannedExistingTarget{
			TargetID: targetID,
			Current:  target,
			Delivery: *nextDelivery,
			Adapter:  adapter,
			Inspect:  inspect,
			Plan:     plan,
			Manifest: &manifest,
			Resolved: &resolved,
			Report:   toTargetReport(*nextDelivery, inspect, plan),
		}, nil
	default:
		return plannedExistingTarget{}, domain.NewError(domain.ErrUsage, "unsupported existing lifecycle action "+action, nil)
	}
}

func (s Service) resolveCurrentSourceManifest(ctx context.Context, record domain.InstallationRecord) (ports.ResolvedSource, domain.IntegrationManifest, error) {
	resolved, err := s.SourceResolver.Resolve(ctx, domain.IntegrationRef{Raw: record.RequestedSourceRef.Value})
	if err != nil {
		return ports.ResolvedSource{}, domain.IntegrationManifest{}, err
	}
	manifest, err := s.ManifestLoader.Load(ctx, resolved)
	if err != nil {
		return ports.ResolvedSource{}, domain.IntegrationManifest{}, err
	}
	if manifest.IntegrationID != record.IntegrationID {
		return ports.ResolvedSource{}, domain.IntegrationManifest{}, domain.NewError(domain.ErrStateConflict, "resolved source does not match installation identity "+record.IntegrationID, nil)
	}
	return resolved, manifest, nil
}

func (s Service) resolveDesiredSourceManifest(ctx context.Context, source string) (ports.ResolvedSource, domain.IntegrationManifest, error) {
	resolved, err := s.SourceResolver.Resolve(ctx, domain.IntegrationRef{Raw: source})
	if err != nil {
		return ports.ResolvedSource{}, domain.IntegrationManifest{}, err
	}
	manifest, err := s.ManifestLoader.Load(ctx, resolved)
	if err != nil {
		cleanupResolvedSource(resolved)
		return ports.ResolvedSource{}, domain.IntegrationManifest{}, err
	}
	return resolved, manifest, nil
}

func (s Service) planAdoptedUpdateTargets(ctx context.Context, record domain.InstallationRecord, manifest domain.IntegrationManifest, resolved ports.ResolvedSource) ([]plannedExistingTarget, []string, error) {
	existing := make(map[domain.TargetID]struct{}, len(record.Targets))
	for targetID := range record.Targets {
		existing[targetID] = struct{}{}
	}
	autoAdopt := strings.EqualFold(strings.TrimSpace(record.Policy.AdoptNewTargets), "auto")
	out := []plannedExistingTarget{}
	warnings := []string{}
	for _, delivery := range manifest.Deliveries {
		if _, ok := existing[delivery.TargetID]; ok {
			continue
		}
		if !autoAdopt {
			warnings = append(warnings, fmt.Sprintf("New target support is available for %s on %s, but adopt_new_targets=%s.", record.IntegrationID, delivery.TargetID, defaultString(record.Policy.AdoptNewTargets, "manual")))
			continue
		}
		adapter, ok := s.Adapters[delivery.TargetID]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("Automatic adoption skipped for %s on %s: no adapter is registered.", record.IntegrationID, delivery.TargetID))
			continue
		}
		inspect, err := adapter.Inspect(ctx, ports.InspectInput{IntegrationID: record.IntegrationID, Record: &record, Scope: record.Policy.Scope})
		if err != nil {
			return nil, nil, err
		}
		plan, err := adapter.PlanInstall(ctx, ports.PlanInstallInput{
			Manifest: manifest,
			Policy:   record.Policy,
			Inspect:  inspect,
		})
		if err != nil {
			return nil, nil, err
		}
		plan.ActionClass = "adopt_new_target"
		plan.Summary = "Adopt newly supported target " + string(delivery.TargetID)
		if _, err := s.validateEvidence(ctx, delivery.TargetID, plan.EvidenceKey); err != nil {
			return nil, nil, err
		}
		if plan.Blocking {
			warnings = append(warnings, fmt.Sprintf("Automatic adoption skipped for %s on %s: native environment blocks installation.", record.IntegrationID, delivery.TargetID))
			continue
		}
		resolvedCopy := resolved
		manifestCopy := manifest
		out = append(out, plannedExistingTarget{
			TargetID: delivery.TargetID,
			Delivery: delivery,
			Adapter:  adapter,
			Inspect:  inspect,
			Plan:     plan,
			Manifest: &manifestCopy,
			Resolved: &resolvedCopy,
			Report:   toTargetReport(delivery, inspect, plan),
			Adopted:  true,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TargetID < out[j].TargetID })
	return out, warnings, nil
}
