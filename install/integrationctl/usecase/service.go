package usecase

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type Service struct {
	SourceResolver       ports.SourceResolver
	ManifestLoader       ports.ManifestLoader
	StateStore           ports.StateStore
	WorkspaceLock        ports.WorkspaceLockStore
	LockManager          ports.LockManager
	Journal              ports.OperationJournal
	Evidence             ports.EvidenceRegistry
	Adapters             map[domain.TargetID]ports.TargetAdapter
	CurrentWorkspaceRoot string
	Now                  func() time.Time
}

type AddInput struct {
	Source          string
	Targets         []string
	Scope           string
	AutoUpdate      *bool
	AdoptNewTargets string
	AllowPrerelease *bool
	DryRun          bool
}

type NamedDryRunInput struct {
	Name   string
	Target string
	DryRun bool
}

func (s Service) List(ctx context.Context) (domain.Report, error) {
	state, err := s.StateStore.Load(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	if len(state.Installations) == 0 {
		return domain.Report{Summary: "No managed integrations are installed yet."}, nil
	}
	report := domain.Report{Summary: fmt.Sprintf("%d managed integration(s) in state.", len(state.Installations))}
	for _, inst := range state.Installations {
		targets := sortedTargets(inst.Targets)
		for _, targetID := range targets {
			ti := inst.Targets[targetID]
			report.Targets = append(report.Targets, domain.TargetReport{
				TargetID:          string(targetID),
				DeliveryKind:      string(ti.DeliveryKind),
				CapabilitySurface: append([]string(nil), ti.CapabilitySurface...),
				State:             string(ti.State),
				ActivationState:   string(ti.ActivationState),
				CatalogPolicy:     cloneCatalogPolicy(ti.CatalogPolicy),
				SourceAccessState: ti.SourceAccessState,
			})
		}
	}
	return report, nil
}

func (s Service) Doctor(ctx context.Context) (domain.Report, error) {
	state, err := s.StateStore.Load(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	openOps, err := s.Journal.ListOpen(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	var degradedCount, activationPendingCount, authPendingCount int
	report := domain.Report{}
	for _, inst := range state.Installations {
		for _, targetID := range sortedTargets(inst.Targets) {
			ti := inst.Targets[targetID]
			if !doctorTargetNeedsAttention(ti) {
				continue
			}
			switch ti.State {
			case domain.InstallDegraded:
				degradedCount++
			case domain.InstallActivationPending:
				activationPendingCount++
			case domain.InstallAuthPending:
				authPendingCount++
			}
			report.Targets = append(report.Targets, domain.TargetReport{
				TargetID:                string(targetID),
				DeliveryKind:            string(ti.DeliveryKind),
				CapabilitySurface:       append([]string(nil), ti.CapabilitySurface...),
				ActionClass:             "doctor_attention",
				State:                   string(ti.State),
				ActivationState:         string(ti.ActivationState),
				InteractiveAuthState:    ti.InteractiveAuthState,
				CatalogPolicy:           cloneCatalogPolicy(ti.CatalogPolicy),
				EnvironmentRestrictions: restrictionsToStrings(ti.EnvironmentRestrictions),
				SourceAccessState:       ti.SourceAccessState,
				ManualSteps:             doctorManualSteps(inst.IntegrationID, ti),
			})
		}
	}
	report.Summary = fmt.Sprintf("Doctor: %d installation(s), %d open operation journal(s), %d degraded target(s), %d activation-pending target(s), %d auth-pending target(s).", len(state.Installations), len(openOps), degradedCount, activationPendingCount, authPendingCount)
	for _, op := range openOps {
		report.Warnings = append(report.Warnings, doctorWarningForOperation(op))
	}
	sort.Slice(report.Targets, func(i, j int) bool {
		if report.Targets[i].TargetID == report.Targets[j].TargetID {
			return report.Targets[i].DeliveryKind < report.Targets[j].DeliveryKind
		}
		return report.Targets[i].TargetID < report.Targets[j].TargetID
	})
	return report, nil
}

func (s Service) Add(ctx context.Context, in AddInput) (domain.Report, error) {
	resolved, err := s.SourceResolver.Resolve(ctx, domain.IntegrationRef{Raw: in.Source})
	if err != nil {
		return domain.Report{}, err
	}
	defer cleanupResolvedSource(resolved)
	manifest, err := s.ManifestLoader.Load(ctx, resolved)
	if err != nil {
		return domain.Report{}, err
	}
	selectedTargets, err := resolveRequestedTargets(manifest, in.Targets)
	if err != nil {
		return domain.Report{}, err
	}
	policy := domain.InstallPolicy{
		Scope:           defaultString(in.Scope, "user"),
		AutoUpdate:      defaultBool(in.AutoUpdate, true),
		AdoptNewTargets: defaultString(in.AdoptNewTargets, "manual"),
		AllowPrerelease: defaultBool(in.AllowPrerelease, false),
	}
	opPrefix := "add"
	summary := fmt.Sprintf("Install plan for integration %q at version %s.", manifest.IntegrationID, manifest.Version)
	if in.DryRun {
		opPrefix = "plan_add"
		summary = fmt.Sprintf("Dry-run plan for integration %q at version %s.", manifest.IntegrationID, manifest.Version)
	}
	report := domain.Report{
		OperationID: operationID(opPrefix, manifest.IntegrationID, s.now()),
		Summary:     summary,
	}
	planned := make([]plannedTargetInstall, 0, len(selectedTargets))
	for _, target := range selectedTargets {
		item, err := s.planTargetInstall(ctx, manifest, policy, target)
		if err != nil {
			return domain.Report{}, err
		}
		planned = append(planned, item)
		report.Targets = append(report.Targets, toTargetReport(item.Delivery, item.Inspect, item.Plan))
	}
	sort.Slice(report.Targets, func(i, j int) bool { return report.Targets[i].TargetID < report.Targets[j].TargetID })
	if in.DryRun {
		return report, nil
	}
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
	}
	return s.applyAdd(ctx, report.OperationID, manifest, resolved, policy, planned)
}

func (s Service) Update(ctx context.Context, in NamedDryRunInput) (domain.Report, error) {
	return s.executeExisting(ctx, in, "update_version")
}

func (s Service) Remove(ctx context.Context, in NamedDryRunInput) (domain.Report, error) {
	return s.executeExisting(ctx, in, "remove_orphaned_target")
}

func (s Service) Repair(ctx context.Context, in NamedDryRunInput) (domain.Report, error) {
	return s.executeExisting(ctx, in, "repair_drift")
}

func (s Service) Enable(ctx context.Context, in NamedDryRunInput) (domain.Report, error) {
	return s.executeExisting(ctx, in, "enable_target")
}

func (s Service) Disable(ctx context.Context, in NamedDryRunInput) (domain.Report, error) {
	return s.executeExisting(ctx, in, "disable_target")
}

func (s Service) UpdateAll(ctx context.Context, dryRun bool) (domain.Report, error) {
	state, err := s.StateStore.Load(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	if len(state.Installations) == 0 {
		return domain.Report{Summary: "No managed integrations to update."}, nil
	}
	installations := append([]domain.InstallationRecord(nil), state.Installations...)
	sort.Slice(installations, func(i, j int) bool { return installations[i].IntegrationID < installations[j].IntegrationID })
	report := domain.Report{
		OperationID: operationID("batch_update", "all", s.now()),
		Summary:     fmt.Sprintf("Processed update for %d managed integration(s).", len(installations)),
	}
	successes := 0
	for _, record := range installations {
		item, err := s.planExisting(ctx, NamedDryRunInput{Name: record.IntegrationID, DryRun: dryRun}, "update_version")
		if err != nil {
			report.Warnings = append(report.Warnings, "Update skipped for "+record.IntegrationID+": "+err.Error())
			continue
		}
		successes++
		report.Targets = append(report.Targets, item.Targets...)
	}
	if successes == 0 && len(report.Warnings) > 0 {
		report.Summary = "No managed integrations were updated successfully."
	}
	sort.Slice(report.Targets, func(i, j int) bool {
		if report.Targets[i].TargetID == report.Targets[j].TargetID {
			return report.Targets[i].DeliveryKind < report.Targets[j].DeliveryKind
		}
		return report.Targets[i].TargetID < report.Targets[j].TargetID
	})
	return report, nil
}

func (s Service) Sync(ctx context.Context, dryRun bool) (domain.Report, error) {
	if s.WorkspaceLock == nil {
		return domain.Report{}, domain.NewError(domain.ErrUsage, "workspace lock store is not configured", nil)
	}
	lock, err := s.WorkspaceLock.Load(ctx)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.Report{}, domain.NewError(domain.ErrUsage, "workspace lock not found: "+s.WorkspaceLock.Path(), err)
		}
		return domain.Report{}, err
	}
	if strings.TrimSpace(lock.APIVersion) != "" && strings.TrimSpace(lock.APIVersion) != "v1" {
		return domain.Report{}, domain.NewError(domain.ErrUsage, "workspace lock api_version must be v1", nil)
	}

	state, err := s.StateStore.Load(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	current := map[string]domain.InstallationRecord{}
	for _, record := range state.Installations {
		current[record.IntegrationID] = record
	}

	report := domain.Report{
		OperationID: operationID("sync", "workspace", s.now()),
		Summary:     fmt.Sprintf("Processed workspace sync for %d desired integration(s).", len(lock.Integrations)),
	}
	desiredIDs := map[string]struct{}{}

	for _, item := range lock.Integrations {
		source := resolveWorkspaceLockSource(s.WorkspaceLock.Path(), item.Source)
		resolved, manifest, err := s.resolveDesiredSourceManifest(ctx, source)
		if err != nil {
			report.Warnings = append(report.Warnings, "Sync skipped for source "+item.Source+": "+err.Error())
			continue
		}
		cleanupResolvedSource(resolved)
		desiredIDs[manifest.IntegrationID] = struct{}{}
		desiredPolicy := desiredPolicyFromLock(item.Policy)
		targets, err := resolveRequestedTargets(manifest, item.Targets)
		if err != nil {
			report.Warnings = append(report.Warnings, "Sync skipped for "+manifest.IntegrationID+": "+err.Error())
			continue
		}

		record, exists := current[manifest.IntegrationID]
		switch {
		case !exists:
			itemReport, err := s.Add(ctx, AddInput{
				Source:          source,
				Targets:         targetIDsToStrings(targets),
				Scope:           desiredPolicy.Scope,
				AutoUpdate:      boolPtr(desiredPolicy.AutoUpdate),
				AdoptNewTargets: desiredPolicy.AdoptNewTargets,
				AllowPrerelease: boolPtr(desiredPolicy.AllowPrerelease),
				DryRun:          dryRun,
			})
			if err != nil {
				report.Warnings = append(report.Warnings, "Sync add failed for "+manifest.IntegrationID+": "+err.Error())
				continue
			}
			report.Targets = append(report.Targets, itemReport.Targets...)
		case syncNeedsReplace(record, source, desiredPolicy, targets, item.Version):
			if record.Policy.Scope != "project" || desiredPolicy.Scope != "project" {
				report.Warnings = append(report.Warnings, "Sync skipped replace for "+manifest.IntegrationID+": replacing non-project scoped integrations is blocked")
				continue
			}
			removeReport, err := s.Remove(ctx, NamedDryRunInput{Name: record.IntegrationID, DryRun: dryRun})
			if err != nil {
				report.Warnings = append(report.Warnings, "Sync remove-before-add failed for "+manifest.IntegrationID+": "+err.Error())
				continue
			}
			addReport, err := s.Add(ctx, AddInput{
				Source:          source,
				Targets:         targetIDsToStrings(targets),
				Scope:           desiredPolicy.Scope,
				AutoUpdate:      boolPtr(desiredPolicy.AutoUpdate),
				AdoptNewTargets: desiredPolicy.AdoptNewTargets,
				AllowPrerelease: boolPtr(desiredPolicy.AllowPrerelease),
				DryRun:          dryRun,
			})
			if err != nil {
				report.Warnings = append(report.Warnings, "Sync re-add failed for "+manifest.IntegrationID+": "+err.Error())
				report.Targets = append(report.Targets, removeReport.Targets...)
				continue
			}
			report.Targets = append(report.Targets, removeReport.Targets...)
			report.Targets = append(report.Targets, addReport.Targets...)
		case syncNeedsUpdate(record, source, item.Version):
			itemReport, err := s.Update(ctx, NamedDryRunInput{Name: record.IntegrationID, DryRun: dryRun})
			if err != nil {
				report.Warnings = append(report.Warnings, "Sync update failed for "+manifest.IntegrationID+": "+err.Error())
				continue
			}
			report.Targets = append(report.Targets, itemReport.Targets...)
		default:
			report.Warnings = append(report.Warnings, "Sync no-op for "+manifest.IntegrationID+": desired state already matches workspace intent")
		}
	}

	for _, record := range state.Installations {
		if _, keep := desiredIDs[record.IntegrationID]; keep {
			continue
		}
		if record.Policy.Scope != "project" {
			report.Warnings = append(report.Warnings, "Sync skipped unmanaged-scope removal for "+record.IntegrationID+": scope="+record.Policy.Scope)
			continue
		}
		itemReport, err := s.Remove(ctx, NamedDryRunInput{Name: record.IntegrationID, DryRun: dryRun})
		if err != nil {
			report.Warnings = append(report.Warnings, "Sync remove failed for "+record.IntegrationID+": "+err.Error())
			continue
		}
		report.Targets = append(report.Targets, itemReport.Targets...)
	}

	if len(report.Targets) == 0 && len(report.Warnings) == 0 {
		report.Summary = "Workspace sync found no changes."
	}
	return report, nil
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

func (s Service) executeExisting(ctx context.Context, in NamedDryRunInput, action string) (domain.Report, error) {
	return s.planExisting(ctx, in, action)
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

type removedExistingTarget struct {
	Planned plannedExistingTarget
	Result  ports.ApplyResult
}

func (s Service) applyRemoveExisting(ctx context.Context, record domain.InstallationRecord, planned []plannedExistingTarget) (domain.Report, error) {
	if len(planned) == 0 {
		return domain.Report{}, domain.NewError(domain.ErrMutationApply, "remove requires at least one planned target", nil)
	}
	defer cleanupPlannedExisting(planned)
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
	}
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
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
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "inspect", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "plan", Status: "ok"}); err != nil {
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
			rollbackFailed, rollbackWarnings := s.rollbackRemovedExisting(ctx, operationID, record, removed)
			if len(rollbackFailed) > 0 {
				state.Installations = upsertInstallation(state.Installations, degradedRecordForRemoveFailure(record, startedAt, target.TargetID, rollbackFailed))
				if saveErr := s.StateStore.Save(ctx, state); saveErr != nil {
					return domain.Report{}, saveErr
				}
				if stepErr := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: "state", Action: "persist_degraded_state", Status: "ok"}); stepErr != nil {
					return domain.Report{}, stepErr
				}
				if finishErr := s.Journal.Finish(ctx, operationID, "degraded"); finishErr != nil {
					return domain.Report{}, finishErr
				}
				committed = true
				msg := "remove failed and rollback was incomplete; degraded state persisted"
				if len(rollbackWarnings) > 0 {
					msg += ": " + strings.Join(rollbackWarnings, "; ")
				}
				return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
			}
			if finishErr := s.Journal.Finish(ctx, operationID, "rolled_back"); finishErr != nil {
				return domain.Report{}, finishErr
			}
			committed = true
			msg := "remove failed and removed targets were rolled back"
			if len(rollbackWarnings) > 0 {
				msg += ": " + strings.Join(rollbackWarnings, "; ")
			}
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		verified, err := s.verifyPostApply(ctx, record.IntegrationID, record.Policy, &record, target.Adapter, "remove_orphaned_target")
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "failed"})
			rollbackFailed, rollbackWarnings := s.rollbackRemovedExisting(ctx, operationID, record, append(removed, removedExistingTarget{Planned: target, Result: applyResult}))
			if len(rollbackFailed) > 0 {
				state.Installations = upsertInstallation(state.Installations, degradedRecordForRemoveFailure(record, startedAt, target.TargetID, rollbackFailed))
				if saveErr := s.StateStore.Save(ctx, state); saveErr != nil {
					return domain.Report{}, saveErr
				}
				if stepErr := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: "state", Action: "persist_degraded_state", Status: "ok"}); stepErr != nil {
					return domain.Report{}, stepErr
				}
				if finishErr := s.Journal.Finish(ctx, operationID, "degraded"); finishErr != nil {
					return domain.Report{}, finishErr
				}
				committed = true
				msg := "remove verification failed and rollback was incomplete; degraded state persisted"
				if len(rollbackWarnings) > 0 {
					msg += ": " + strings.Join(rollbackWarnings, "; ")
				}
				return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
			}
			if finishErr := s.Journal.Finish(ctx, operationID, "rolled_back"); finishErr != nil {
				return domain.Report{}, finishErr
			}
			committed = true
			msg := "remove verification failed and removed targets were rolled back"
			if len(rollbackWarnings) > 0 {
				msg += ": " + strings.Join(rollbackWarnings, "; ")
			}
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		removed = append(removed, removedExistingTarget{Planned: target, Result: applyResult})
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

func (s Service) applyRepairExisting(ctx context.Context, record domain.InstallationRecord, planned []plannedExistingTarget) (domain.Report, error) {
	if len(planned) == 0 {
		return domain.Report{}, domain.NewError(domain.ErrMutationApply, "repair requires at least one planned target", nil)
	}
	defer cleanupPlannedExisting(planned)
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
		if target.Manifest == nil || target.Resolved == nil {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "repair requires resolved source and manifest for target "+string(target.TargetID), nil)
		}
	}
	operationID := operationID(actionNamePrefix("repair_drift"), record.IntegrationID, s.now())
	unlock, err := s.LockManager.Acquire(ctx, "state")
	if err != nil {
		return domain.Report{}, domain.NewError(domain.ErrLockAcquire, "acquire integrationctl state lock", err)
	}
	defer func() { _ = unlock() }()

	startedAt := s.now().UTC().Format(time.RFC3339)
	if err := s.Journal.Start(ctx, domain.OperationRecord{
		OperationID:   operationID,
		Type:          "repair",
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
	nextRecord := cloneInstallationRecord(items)
	reportTargets := make([]domain.TargetReport, 0, len(planned))
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
	}
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
	}
	for _, target := range planned {
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "inspect", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "plan", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		applyResult, err := target.Adapter.Repair(ctx, ports.RepairInput{
			Record:         record,
			Inspect:        target.Inspect,
			Manifest:       target.Manifest,
			ResolvedSource: target.Resolved,
		})
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "failed"})
			markTargetDegraded(&nextRecord, target.TargetID)
			applyManifestMetadata(&nextRecord, *target.Manifest, startedAt)
			state.Installations = upsertInstallation(state.Installations, nextRecord)
			if saveErr := s.StateStore.Save(ctx, state); saveErr != nil {
				return domain.Report{}, saveErr
			}
			if stepErr := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: "state", Action: "persist_degraded_state", Status: "ok"}); stepErr != nil {
				return domain.Report{}, stepErr
			}
			if finishErr := s.Journal.Finish(ctx, operationID, "degraded"); finishErr != nil {
				return domain.Report{}, finishErr
			}
			committed = true
			return domain.Report{}, domain.NewError(domain.ErrRepairApply, "repair failed after partial progress; degraded state persisted", err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		verifyRecord := provisionalRecordForExisting(record, target, applyResult)
		verified, err := s.verifyPostApply(ctx, record.IntegrationID, record.Policy, &verifyRecord, target.Adapter, "repair_drift")
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "failed"})
			markTargetDegraded(&nextRecord, target.TargetID)
			applyManifestMetadata(&nextRecord, *target.Manifest, startedAt)
			state.Installations = upsertInstallation(state.Installations, nextRecord)
			if saveErr := s.StateStore.Save(ctx, state); saveErr != nil {
				return domain.Report{}, saveErr
			}
			if stepErr := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: "state", Action: "persist_degraded_state", Status: "ok"}); stepErr != nil {
				return domain.Report{}, stepErr
			}
			if finishErr := s.Journal.Finish(ctx, operationID, "degraded"); finishErr != nil {
				return domain.Report{}, finishErr
			}
			committed = true
			return domain.Report{}, domain.NewError(domain.ErrRepairApply, "repair verification failed after partial progress; degraded state persisted", err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		nextRecord.Targets[target.TargetID] = targetInstallationFromExisting(target, applyResult, verified)
		applyManifestMetadata(&nextRecord, *target.Manifest, startedAt)
		reportTargets = append(reportTargets, toAppliedTargetReport(target.Delivery, target.Inspect, verified, target.Plan, applyResult))
	}

	state.Installations = upsertInstallation(state.Installations, nextRecord)
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
		Summary:     summaryForExisting("repair_drift", record.IntegrationID),
		Targets:     reportTargets,
	}, nil
}

func (s Service) applyUpdateExisting(ctx context.Context, record domain.InstallationRecord, planned []plannedExistingTarget) (domain.Report, error) {
	if len(planned) == 0 {
		return domain.Report{}, domain.NewError(domain.ErrMutationApply, "update requires at least one planned target", nil)
	}
	defer cleanupPlannedExisting(planned)
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
		if target.Manifest == nil || target.Resolved == nil {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "update requires resolved source and manifest for target "+string(target.TargetID), nil)
		}
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
	nextRecord := cloneInstallationRecord(items)
	reportTargets := make([]domain.TargetReport, 0, len(planned))
	for _, target := range planned {
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "inspect", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "plan", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		var applyResult ports.ApplyResult
		var err error
		if target.Adopted {
			applyResult, err = target.Adapter.ApplyInstall(ctx, ports.ApplyInput{
				Plan:           target.Plan,
				Manifest:       *target.Manifest,
				ResolvedSource: target.Resolved,
				Policy:         record.Policy,
				Inspect:        target.Inspect,
				Record:         &record,
			})
		} else {
			applyResult, err = target.Adapter.ApplyUpdate(ctx, ports.ApplyInput{
				Plan:           target.Plan,
				Manifest:       *target.Manifest,
				ResolvedSource: target.Resolved,
				Policy:         record.Policy,
				Inspect:        target.Inspect,
				Record:         &record,
			})
		}
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "failed"})
			markPlannedTargetDegraded(&nextRecord, target)
			applyManifestMetadata(&nextRecord, *target.Manifest, startedAt)
			state.Installations = upsertInstallation(state.Installations, nextRecord)
			if saveErr := s.StateStore.Save(ctx, state); saveErr != nil {
				return domain.Report{}, saveErr
			}
			if stepErr := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: "state", Action: "persist_degraded_state", Status: "ok"}); stepErr != nil {
				return domain.Report{}, stepErr
			}
			if finishErr := s.Journal.Finish(ctx, operationID, "degraded"); finishErr != nil {
				return domain.Report{}, finishErr
			}
			committed = true
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "update failed after partial progress; degraded state persisted", err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		verifyRecord := provisionalRecordForExisting(record, target, applyResult)
		verified, err := s.verifyPostApply(ctx, record.IntegrationID, record.Policy, &verifyRecord, target.Adapter, "update_version")
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "failed"})
			markPlannedTargetDegraded(&nextRecord, target)
			applyManifestMetadata(&nextRecord, *target.Manifest, startedAt)
			state.Installations = upsertInstallation(state.Installations, nextRecord)
			if saveErr := s.StateStore.Save(ctx, state); saveErr != nil {
				return domain.Report{}, saveErr
			}
			if stepErr := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: "state", Action: "persist_degraded_state", Status: "ok"}); stepErr != nil {
				return domain.Report{}, stepErr
			}
			if finishErr := s.Journal.Finish(ctx, operationID, "degraded"); finishErr != nil {
				return domain.Report{}, finishErr
			}
			committed = true
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "update verification failed after partial progress; degraded state persisted", err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		nextRecord.Targets[target.TargetID] = targetInstallationFromExisting(target, applyResult, verified)
		applyManifestMetadata(&nextRecord, *target.Manifest, startedAt)
		reportTargets = append(reportTargets, toAppliedTargetReport(target.Delivery, target.Inspect, verified, target.Plan, applyResult))
	}

	state.Installations = upsertInstallation(state.Installations, nextRecord)
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
		Summary:     summaryForExisting("update_version", record.IntegrationID),
		Targets:     reportTargets,
	}, nil
}

func actionNamePrefix(action string) string {
	switch action {
	case "update_version":
		return "update"
	case "remove_orphaned_target":
		return "remove"
	case "repair_drift":
		return "repair"
	case "enable_target":
		return "enable"
	case "disable_target":
		return "disable"
	default:
		return "operation"
	}
}

func summaryForExisting(action, integrationID string) string {
	switch action {
	case "update_version":
		return fmt.Sprintf("Updated integration %q.", integrationID)
	case "remove_orphaned_target":
		return fmt.Sprintf("Removed managed targets from integration %q.", integrationID)
	case "repair_drift":
		return fmt.Sprintf("Repaired managed targets for integration %q.", integrationID)
	case "enable_target":
		return fmt.Sprintf("Enabled managed targets for integration %q.", integrationID)
	case "disable_target":
		return fmt.Sprintf("Disabled managed targets for integration %q.", integrationID)
	default:
		return fmt.Sprintf("Applied %s for %q.", action, integrationID)
	}
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func resolveRequestedTargets(manifest domain.IntegrationManifest, requested []string) ([]domain.TargetID, error) {
	if len(requested) == 0 {
		targets := make([]domain.TargetID, 0, len(manifest.Deliveries))
		for _, delivery := range manifest.Deliveries {
			targets = append(targets, delivery.TargetID)
		}
		return targets, nil
	}
	out := make([]domain.TargetID, 0, len(requested))
	available := map[domain.TargetID]struct{}{}
	for _, delivery := range manifest.Deliveries {
		available[delivery.TargetID] = struct{}{}
	}
	for _, raw := range requested {
		target := domain.TargetID(strings.ToLower(strings.TrimSpace(raw)))
		if _, ok := available[target]; !ok {
			return nil, domain.NewError(domain.ErrUnsupportedTarget, "manifest does not expose target "+string(target), nil)
		}
		out = append(out, target)
	}
	return out, nil
}

func findDelivery(items []domain.Delivery, target domain.TargetID) *domain.Delivery {
	for i := range items {
		if items[i].TargetID == target {
			return &items[i]
		}
	}
	return nil
}

func toTargetReport(delivery domain.Delivery, inspect ports.InspectResult, plan ports.AdapterPlan) domain.TargetReport {
	report := domain.TargetReport{
		TargetID:                 string(delivery.TargetID),
		DeliveryKind:             string(delivery.DeliveryKind),
		CapabilitySurface:        append([]string(nil), delivery.CapabilitySurface...),
		ActionClass:              plan.ActionClass,
		State:                    string(inspect.State),
		ActivationState:          string(inspect.ActivationState),
		InteractiveAuthState:     inspect.InteractiveAuthState,
		RestartRequired:          plan.RestartRequired,
		ReloadRequired:           plan.ReloadRequired,
		NewThreadRequired:        plan.NewThreadRequired,
		CatalogPolicy:            cloneCatalogPolicy(inspect.CatalogPolicy),
		VolatileOverrideDetected: inspect.VolatileOverrideDetected,
		TrustResolutionSource:    inspect.TrustResolutionSource,
		SourceAccessState:        inspect.SourceAccessState,
		EvidenceKey:              plan.EvidenceKey,
		ManualSteps:              append([]string(nil), plan.ManualSteps...),
	}
	for _, restriction := range inspect.EnvironmentRestrictions {
		report.EnvironmentRestrictions = append(report.EnvironmentRestrictions, string(restriction))
	}
	return report
}

type plannedTargetInstall struct {
	TargetID domain.TargetID
	Delivery domain.Delivery
	Adapter  ports.TargetAdapter
	Inspect  ports.InspectResult
	Plan     ports.AdapterPlan
}

type appliedTargetInstall struct {
	Planned plannedTargetInstall
	Result  ports.ApplyResult
	Verify  ports.InspectResult
}

func (s Service) planTargetInstall(ctx context.Context, manifest domain.IntegrationManifest, policy domain.InstallPolicy, target domain.TargetID) (plannedTargetInstall, error) {
	adapter, ok := s.Adapters[target]
	if !ok {
		return plannedTargetInstall{}, domain.NewError(domain.ErrUnsupportedTarget, "adapter not registered for "+string(target), nil)
	}
	delivery := findDelivery(manifest.Deliveries, target)
	if delivery == nil {
		return plannedTargetInstall{}, domain.NewError(domain.ErrUnsupportedTarget, "delivery not available for "+string(target), nil)
	}
	inspect, err := adapter.Inspect(ctx, ports.InspectInput{IntegrationID: manifest.IntegrationID, Scope: policy.Scope})
	if err != nil {
		return plannedTargetInstall{}, err
	}
	plan, err := adapter.PlanInstall(ctx, ports.PlanInstallInput{Manifest: manifest, Policy: policy, Inspect: inspect})
	if err != nil {
		return plannedTargetInstall{}, err
	}
	if _, err := s.validateEvidence(ctx, target, plan.EvidenceKey); err != nil {
		return plannedTargetInstall{}, err
	}
	return plannedTargetInstall{
		TargetID: target,
		Delivery: *delivery,
		Adapter:  adapter,
		Inspect:  inspect,
		Plan:     plan,
	}, nil
}

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
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "inspect", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "plan", Status: "ok"}); err != nil {
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
			rollbackFailed, rollbackWarnings := s.rollbackAppliedAdd(ctx, operationID, manifest, policy, startedAt, applied)
			if len(rollbackFailed) > 0 {
				state.Installations = upsertInstallation(state.Installations, degradedRecordFromApplied(manifest, policy, s.workspaceRootForPolicy(policy), startedAt, rollbackFailed))
				if saveErr := s.StateStore.Save(ctx, state); saveErr != nil {
					return domain.Report{}, saveErr
				}
				if stepErr := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: "state", Action: "persist_degraded_state", Status: "ok"}); stepErr != nil {
					return domain.Report{}, stepErr
				}
				if finishErr := s.Journal.Finish(ctx, operationID, "degraded"); finishErr != nil {
					return domain.Report{}, finishErr
				}
				committed = true
				msg := "install failed and rollback was incomplete; degraded state persisted"
				if len(rollbackWarnings) > 0 {
					msg += ": " + strings.Join(rollbackWarnings, "; ")
				}
				return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
			}
			if finishErr := s.Journal.Finish(ctx, operationID, "rolled_back"); finishErr != nil {
				return domain.Report{}, finishErr
			}
			committed = true
			msg := "install failed and applied targets were rolled back"
			if len(rollbackWarnings) > 0 {
				msg += ": " + strings.Join(rollbackWarnings, "; ")
			}
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		verifyRecord := provisionalRecordForAdd(manifest, policy, s.workspaceRootForPolicy(policy), target, applyResult)
		verified, err := s.verifyPostApply(ctx, manifest.IntegrationID, policy, &verifyRecord, target.Adapter, "add")
		if err != nil {
			_ = s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "failed"})
			rollbackFailed, rollbackWarnings := s.rollbackAppliedAdd(ctx, operationID, manifest, policy, startedAt, append(applied, appliedTargetInstall{Planned: target, Result: applyResult}))
			if len(rollbackFailed) > 0 {
				state.Installations = upsertInstallation(state.Installations, degradedRecordFromApplied(manifest, policy, s.workspaceRootForPolicy(policy), startedAt, rollbackFailed))
				if saveErr := s.StateStore.Save(ctx, state); saveErr != nil {
					return domain.Report{}, saveErr
				}
				if stepErr := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: "state", Action: "persist_degraded_state", Status: "ok"}); stepErr != nil {
					return domain.Report{}, stepErr
				}
				if finishErr := s.Journal.Finish(ctx, operationID, "degraded"); finishErr != nil {
					return domain.Report{}, finishErr
				}
				committed = true
				msg := "install verification failed and rollback was incomplete; degraded state persisted"
				if len(rollbackWarnings) > 0 {
					msg += ": " + strings.Join(rollbackWarnings, "; ")
				}
				return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
			}
			if finishErr := s.Journal.Finish(ctx, operationID, "rolled_back"); finishErr != nil {
				return domain.Report{}, finishErr
			}
			committed = true
			msg := "install verification failed and applied targets were rolled back"
			if len(rollbackWarnings) > 0 {
				msg += ": " + strings.Join(rollbackWarnings, "; ")
			}
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, msg, err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "verify", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		applied = append(applied, appliedTargetInstall{Planned: target, Result: applyResult, Verify: verified})
		reportTargets = append(reportTargets, toAppliedTargetReport(target.Delivery, target.Inspect, verified, target.Plan, applyResult))
	}

	state.Installations = upsertInstallation(state.Installations, installationRecordFromApplied(manifest, policy, s.workspaceRootForPolicy(policy), startedAt, applied))
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

func (s Service) validateEvidence(ctx context.Context, target domain.TargetID, key string) (ports.EvidenceEntry, error) {
	if strings.TrimSpace(key) == "" {
		return ports.EvidenceEntry{}, domain.NewError(domain.ErrEvidenceViolation, "adapter plan missing evidence key for "+string(target), nil)
	}
	entry, err := s.Evidence.Get(ctx, key)
	if err != nil {
		return ports.EvidenceEntry{}, domain.NewError(domain.ErrEvidenceViolation, "unknown evidence key "+key, err)
	}
	return entry, nil
}

func toAppliedTargetReport(delivery domain.Delivery, inspect ports.InspectResult, verified ports.InspectResult, plan ports.AdapterPlan, result ports.ApplyResult) domain.TargetReport {
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
	sourceAccessState := firstNonEmpty(verified.SourceAccessState, result.SourceAccessState)
	environmentRestrictions := append([]domain.EnvironmentRestrictionCode(nil), result.EnvironmentRestrictions...)
	if len(verified.EnvironmentRestrictions) > 0 {
		environmentRestrictions = append([]domain.EnvironmentRestrictionCode(nil), verified.EnvironmentRestrictions...)
	}
	report := domain.TargetReport{
		TargetID:             string(delivery.TargetID),
		DeliveryKind:         string(delivery.DeliveryKind),
		CapabilitySurface:    append([]string(nil), delivery.CapabilitySurface...),
		ActionClass:          plan.ActionClass,
		State:                string(state),
		ActivationState:      string(activationState),
		InteractiveAuthState: interactiveAuthState,
		RestartRequired:      result.RestartRequired,
		ReloadRequired:       result.ReloadRequired,
		NewThreadRequired:    result.NewThreadRequired,
		CatalogPolicy:        cloneCatalogPolicy(firstNonNilCatalogPolicy(verified.CatalogPolicy, inspect.CatalogPolicy)),
		SourceAccessState:    sourceAccessState,
		EvidenceKey:          plan.EvidenceKey,
		ManualSteps:          append([]string(nil), result.ManualSteps...),
	}
	for _, restriction := range environmentRestrictions {
		report.EnvironmentRestrictions = append(report.EnvironmentRestrictions, string(restriction))
	}
	return report
}

func installationRecordFromApplied(manifest domain.IntegrationManifest, policy domain.InstallPolicy, workspaceRoot string, startedAt string, applied []appliedTargetInstall) domain.InstallationRecord {
	targets := make(map[domain.TargetID]domain.TargetInstallation, len(applied))
	for _, item := range applied {
		targets[item.Planned.TargetID] = targetInstallationFromApplied(item)
	}
	return domain.InstallationRecord{
		IntegrationID:      manifest.IntegrationID,
		RequestedSourceRef: manifest.RequestedRef,
		ResolvedSourceRef:  manifest.ResolvedRef,
		ResolvedVersion:    manifest.Version,
		SourceDigest:       manifest.SourceDigest,
		ManifestDigest:     manifest.ManifestDigest,
		Policy:             policy,
		WorkspaceRoot:      workspaceRoot,
		Targets:            targets,
		LastCheckedAt:      startedAt,
		LastUpdatedAt:      startedAt,
	}
}

func degradedRecordFromApplied(manifest domain.IntegrationManifest, policy domain.InstallPolicy, workspaceRoot string, startedAt string, applied []appliedTargetInstall) domain.InstallationRecord {
	record := installationRecordFromApplied(manifest, policy, workspaceRoot, startedAt, applied)
	for key, target := range record.Targets {
		target.State = domain.InstallDegraded
		record.Targets[key] = target
	}
	return record
}

func targetInstallationFromApplied(item appliedTargetInstall) domain.TargetInstallation {
	state := item.Result.State
	if item.Verify.State != "" {
		state = item.Verify.State
	}
	activationState := item.Result.ActivationState
	if item.Verify.ActivationState != "" {
		activationState = item.Verify.ActivationState
	}
	interactiveAuthState := item.Result.InteractiveAuthState
	if strings.TrimSpace(item.Verify.InteractiveAuthState) != "" {
		interactiveAuthState = item.Verify.InteractiveAuthState
	}
	environmentRestrictions := append([]domain.EnvironmentRestrictionCode(nil), item.Result.EnvironmentRestrictions...)
	if len(item.Verify.EnvironmentRestrictions) > 0 {
		environmentRestrictions = append([]domain.EnvironmentRestrictionCode(nil), item.Verify.EnvironmentRestrictions...)
	}
	return domain.TargetInstallation{
		TargetID:                item.Planned.TargetID,
		DeliveryKind:            item.Planned.Delivery.DeliveryKind,
		CapabilitySurface:       append([]string(nil), item.Planned.Delivery.CapabilitySurface...),
		State:                   state,
		NativeRef:               item.Planned.Delivery.NativeRefHint,
		ActivationState:         activationState,
		InteractiveAuthState:    interactiveAuthState,
		CatalogPolicy:           cloneCatalogPolicy(firstNonNilCatalogPolicy(item.Verify.CatalogPolicy, item.Planned.Inspect.CatalogPolicy)),
		EnvironmentRestrictions: environmentRestrictions,
		SourceAccessState:       firstNonEmpty(item.Verify.SourceAccessState, item.Result.SourceAccessState, item.Planned.Inspect.SourceAccessState),
		OwnedNativeObjects:      append([]domain.NativeObjectRef(nil), item.Result.OwnedNativeObjects...),
		AdapterMetadata:         cloneMetadata(item.Result.AdapterMetadata),
	}
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

func degradedRecordForRemoveFailure(record domain.InstallationRecord, startedAt string, failedTarget domain.TargetID, rollbackFailed []domain.TargetID) domain.InstallationRecord {
	next := cloneInstallationRecord(record)
	next.LastCheckedAt = startedAt
	next.LastUpdatedAt = startedAt

	// Remove is integration-wide. If rollback is incomplete, keep the record but
	// conservatively degrade the whole integration instead of claiming untouched
	// targets are still healthy.
	if len(rollbackFailed) > 0 {
		for targetID, target := range next.Targets {
			target.State = domain.InstallDegraded
			next.Targets[targetID] = target
		}
		return next
	}

	target, ok := next.Targets[failedTarget]
	if ok {
		target.State = domain.InstallDegraded
		next.Targets[failedTarget] = target
	}
	return next
}

func (s Service) rollbackAppliedAdd(ctx context.Context, operationID string, manifest domain.IntegrationManifest, policy domain.InstallPolicy, startedAt string, applied []appliedTargetInstall) ([]appliedTargetInstall, []string) {
	failed := make([]appliedTargetInstall, 0)
	warnings := make([]string, 0)
	for i := len(applied) - 1; i >= 0; i-- {
		item := applied[i]
		record := domain.InstallationRecord{
			IntegrationID:      manifest.IntegrationID,
			RequestedSourceRef: manifest.RequestedRef,
			ResolvedSourceRef:  manifest.ResolvedRef,
			ResolvedVersion:    manifest.Version,
			SourceDigest:       manifest.SourceDigest,
			ManifestDigest:     manifest.ManifestDigest,
			Policy:             policy,
			WorkspaceRoot:      s.workspaceRootForPolicy(policy),
			Targets: map[domain.TargetID]domain.TargetInstallation{
				item.Planned.TargetID: targetInstallationFromApplied(item),
			},
			LastCheckedAt: startedAt,
			LastUpdatedAt: startedAt,
		}
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

func findInstallation(items []domain.InstallationRecord, name string) (domain.InstallationRecord, bool) {
	name = strings.TrimSpace(name)
	for _, item := range items {
		if item.IntegrationID == name {
			return item, true
		}
	}
	return domain.InstallationRecord{}, false
}

func findInstallationMutable(items []domain.InstallationRecord, name string) (domain.InstallationRecord, bool) {
	return findInstallation(items, name)
}

func sortedTargets(m map[domain.TargetID]domain.TargetInstallation) []domain.TargetID {
	out := make([]domain.TargetID, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func upsertInstallation(items []domain.InstallationRecord, next domain.InstallationRecord) []domain.InstallationRecord {
	for i := range items {
		if items[i].IntegrationID == next.IntegrationID {
			items[i] = next
			return items
		}
	}
	return append(items, next)
}

func removeInstallation(items []domain.InstallationRecord, name string) []domain.InstallationRecord {
	out := items[:0]
	for _, item := range items {
		if item.IntegrationID != name {
			out = append(out, item)
		}
	}
	return out
}

func cloneMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneInstallationRecord(in domain.InstallationRecord) domain.InstallationRecord {
	out := in
	if len(in.Targets) == 0 {
		out.Targets = map[domain.TargetID]domain.TargetInstallation{}
		return out
	}
	out.Targets = make(map[domain.TargetID]domain.TargetInstallation, len(in.Targets))
	for key, value := range in.Targets {
		out.Targets[key] = cloneTargetInstallation(value)
	}
	return out
}

func cloneTargetInstallation(in domain.TargetInstallation) domain.TargetInstallation {
	out := in
	out.CapabilitySurface = append([]string(nil), in.CapabilitySurface...)
	out.CatalogPolicy = cloneCatalogPolicy(in.CatalogPolicy)
	out.EnvironmentRestrictions = append([]domain.EnvironmentRestrictionCode(nil), in.EnvironmentRestrictions...)
	out.OwnedNativeObjects = append([]domain.NativeObjectRef(nil), in.OwnedNativeObjects...)
	out.AdapterMetadata = cloneMetadata(in.AdapterMetadata)
	return out
}

func cloneCatalogPolicy(in *domain.CatalogPolicySnapshot) *domain.CatalogPolicySnapshot {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func firstNonNilCatalogPolicy(values ...*domain.CatalogPolicySnapshot) *domain.CatalogPolicySnapshot {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
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

func doctorTargetNeedsAttention(ti domain.TargetInstallation) bool {
	if ti.State == domain.InstallDegraded || ti.State == domain.InstallActivationPending || ti.State == domain.InstallAuthPending {
		return true
	}
	switch ti.ActivationState {
	case domain.ActivationNativePending, domain.ActivationReloadPending, domain.ActivationRestartPending, domain.ActivationNewThreadPending:
		return true
	}
	return false
}

func doctorManualSteps(integrationID string, ti domain.TargetInstallation) []string {
	steps := []string{}
	switch ti.State {
	case domain.InstallDegraded:
		steps = append(steps, "run plugin-kit-ai integrations repair "+integrationID)
	case domain.InstallActivationPending:
		steps = append(steps, "complete the vendor-native activation step for this target")
	case domain.InstallAuthPending:
		steps = append(steps, "complete the required authentication flow for this target")
	}
	for _, restriction := range ti.EnvironmentRestrictions {
		switch restriction {
		case domain.RestrictionNewThreadRequired:
			steps = append(steps, "start a new agent thread before using the integration")
		case domain.RestrictionReloadRequired:
			steps = append(steps, "reload the current agent session to pick up the integration")
		case domain.RestrictionRestartRequired:
			steps = append(steps, "restart the agent CLI or desktop app")
		case domain.RestrictionNativeActivation:
			steps = append(steps, "finish the native activation flow in the target agent")
		case domain.RestrictionNativeAuthRequired, domain.RestrictionSourceAuthRequired:
			steps = append(steps, "complete the missing authentication step and rerun repair")
		}
	}
	return dedupeStrings(steps)
}

func doctorWarningForOperation(op domain.OperationRecord) string {
	switch op.Status {
	case "degraded":
		return fmt.Sprintf("Operation %s for %s ended degraded - run plugin-kit-ai integrations repair %s.", op.OperationID, op.IntegrationID, op.IntegrationID)
	case "in_progress":
		return fmt.Sprintf("Operation %s for %s is still marked in_progress - inspect the journal and rerun repair if the process was interrupted.", op.OperationID, op.IntegrationID)
	case "failed":
		return fmt.Sprintf("Operation %s for %s failed before commit - inspect the journal and rerun the desired lifecycle command.", op.OperationID, op.IntegrationID)
	default:
		return fmt.Sprintf("Open operation %s for %s is still marked %s.", op.OperationID, op.IntegrationID, op.Status)
	}
}

func restrictionsToStrings(in []domain.EnvironmentRestrictionCode) []string {
	out := make([]string, 0, len(in))
	for _, restriction := range in {
		out = append(out, string(restriction))
	}
	return out
}

func dedupeStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

func provisionalRecordForAdd(manifest domain.IntegrationManifest, policy domain.InstallPolicy, workspaceRoot string, target plannedTargetInstall, result ports.ApplyResult) domain.InstallationRecord {
	return domain.InstallationRecord{
		IntegrationID:      manifest.IntegrationID,
		RequestedSourceRef: manifest.RequestedRef,
		ResolvedSourceRef:  manifest.ResolvedRef,
		ResolvedVersion:    manifest.Version,
		SourceDigest:       manifest.SourceDigest,
		ManifestDigest:     manifest.ManifestDigest,
		Policy:             policy,
		WorkspaceRoot:      workspaceRoot,
		Targets: map[domain.TargetID]domain.TargetInstallation{
			target.TargetID: {
				TargetID:           target.TargetID,
				DeliveryKind:       target.Delivery.DeliveryKind,
				CapabilitySurface:  append([]string(nil), target.Delivery.CapabilitySurface...),
				NativeRef:          target.Delivery.NativeRefHint,
				OwnedNativeObjects: append([]domain.NativeObjectRef(nil), result.OwnedNativeObjects...),
				AdapterMetadata:    cloneMetadata(result.AdapterMetadata),
			},
		},
	}
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

func (s Service) workspaceRootForPolicy(policy domain.InstallPolicy) string {
	if !strings.EqualFold(strings.TrimSpace(policy.Scope), "project") {
		return ""
	}
	if root := strings.TrimSpace(s.CurrentWorkspaceRoot); root != "" {
		return filepath.Clean(root)
	}
	return ""
}

func defaultBool(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func operationID(prefix, integrationID string, t time.Time) string {
	return fmt.Sprintf("%s_%s_%d", prefix, sanitizeID(integrationID), t.Unix())
}

func cleanupResolvedSource(source ports.ResolvedSource) {
	if strings.TrimSpace(source.CleanupPath) == "" {
		return
	}
	_ = os.RemoveAll(source.CleanupPath)
}

func cleanupPlannedExisting(items []plannedExistingTarget) {
	for _, item := range items {
		if item.Resolved != nil {
			cleanupResolvedSource(*item.Resolved)
		}
	}
}

func desiredPolicyFromLock(in domain.InstallPolicy) domain.InstallPolicy {
	return domain.InstallPolicy{
		Scope:           defaultString(in.Scope, "project"),
		AutoUpdate:      in.AutoUpdate,
		AdoptNewTargets: defaultString(in.AdoptNewTargets, "manual"),
		AllowPrerelease: in.AllowPrerelease,
	}
}

func resolveWorkspaceLockSource(lockPath, source string) string {
	source = strings.TrimSpace(source)
	if source == "" || filepath.IsAbs(source) {
		return source
	}
	if strings.Contains(source, ":") && !strings.HasPrefix(source, ".") && !strings.HasPrefix(source, "..") {
		return source
	}
	return filepath.Join(filepath.Dir(lockPath), source)
}

func targetIDsToStrings(items []domain.TargetID) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, string(item))
	}
	return out
}

func boolPtr(v bool) *bool { return &v }

func syncNeedsReplace(record domain.InstallationRecord, source string, desiredPolicy domain.InstallPolicy, desiredTargets []domain.TargetID, desiredVersion string) bool {
	if strings.TrimSpace(record.RequestedSourceRef.Value) != strings.TrimSpace(source) {
		return true
	}
	if record.Policy.Scope != desiredPolicy.Scope {
		return true
	}
	if len(record.Targets) != len(desiredTargets) {
		return true
	}
	currentTargets := sortedTargets(record.Targets)
	sort.Slice(desiredTargets, func(i, j int) bool { return desiredTargets[i] < desiredTargets[j] })
	for i := range currentTargets {
		if currentTargets[i] != desiredTargets[i] {
			return true
		}
	}
	if desiredVersion != "" && strings.TrimSpace(record.ResolvedVersion) != strings.TrimSpace(desiredVersion) {
		return false
	}
	return false
}

func syncNeedsUpdate(record domain.InstallationRecord, source, desiredVersion string) bool {
	if strings.TrimSpace(record.RequestedSourceRef.Value) != strings.TrimSpace(source) {
		return false
	}
	if strings.TrimSpace(desiredVersion) == "" {
		return false
	}
	return strings.TrimSpace(record.ResolvedVersion) != strings.TrimSpace(desiredVersion)
}

func sanitizeID(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "integration"
	}
	return b.String()
}
