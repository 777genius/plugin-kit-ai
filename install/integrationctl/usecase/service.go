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
	SourceResolver ports.SourceResolver
	ManifestLoader ports.ManifestLoader
	StateStore     ports.StateStore
	WorkspaceLock  ports.WorkspaceLockStore
	LockManager    ports.LockManager
	Journal        ports.OperationJournal
	Evidence       ports.EvidenceRegistry
	Adapters       map[domain.TargetID]ports.TargetAdapter
	Now            func() time.Time
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
				State:             string(ti.State),
				ActivationState:   string(ti.ActivationState),
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
	summary := fmt.Sprintf("Doctor: %d installation(s), %d open operation journal(s).", len(state.Installations), len(openOps))
	report := domain.Report{Summary: summary}
	for _, op := range openOps {
		report.Warnings = append(report.Warnings, fmt.Sprintf("Open operation %s is still marked %s.", op.OperationID, op.Status))
	}
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
	if (action == "remove_orphaned_target" || action == "repair_drift" || action == "update_version") && !in.DryRun {
		resolved, manifest, err := s.resolveCurrentSourceManifest(ctx, record)
		if err != nil {
			return domain.Report{}, err
		}
		sharedResolved = &resolved
		sharedManifest = &manifest
		defer cleanupResolvedSource(resolved)
	}
	for _, targetID := range sortedTargets(record.Targets) {
		item, err := s.planExistingTarget(ctx, record, targetID, action, sharedResolved, sharedManifest)
		if err != nil {
			cleanupPlannedExisting(planned)
			return domain.Report{}, err
		}
		planned = append(planned, item)
		report.Targets = append(report.Targets, item.Report)
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
	inspect, err := adapter.Inspect(ctx, ports.InspectInput{Record: &record, Scope: record.Policy.Scope})
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
		nextRecord.ResolvedVersion = target.Manifest.Version
		nextRecord.ResolvedSourceRef = target.Manifest.ResolvedRef
		nextRecord.SourceDigest = target.Manifest.SourceDigest
		nextRecord.ManifestDigest = target.Manifest.ManifestDigest
		nextRecord.LastCheckedAt = startedAt
		nextRecord.LastUpdatedAt = startedAt
		nextRecord.Targets[target.TargetID] = domain.TargetInstallation{
			TargetID:                target.TargetID,
			DeliveryKind:            target.Delivery.DeliveryKind,
			State:                   applyResult.State,
			NativeRef:               firstNonEmpty(target.Delivery.NativeRefHint, nextRecord.Targets[target.TargetID].NativeRef),
			ActivationState:         applyResult.ActivationState,
			InteractiveAuthState:    applyResult.InteractiveAuthState,
			EnvironmentRestrictions: append([]domain.EnvironmentRestrictionCode(nil), applyResult.EnvironmentRestrictions...),
			SourceAccessState:       firstNonEmpty(applyResult.SourceAccessState, target.Inspect.SourceAccessState),
			OwnedNativeObjects:      append([]domain.NativeObjectRef(nil), applyResult.OwnedNativeObjects...),
			AdapterMetadata:         cloneMetadata(applyResult.AdapterMetadata),
		}
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
			toAppliedTargetReport(target.Delivery, target.Plan, applyResult),
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
		removed = append(removed, removedExistingTarget{Planned: target, Result: applyResult})
		reportTargets = append(reportTargets, toAppliedTargetReport(target.Delivery, target.Plan, applyResult))
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
		nextRecord.Targets[target.TargetID] = targetInstallationFromExisting(target, applyResult)
		applyManifestMetadata(&nextRecord, *target.Manifest, startedAt)
		reportTargets = append(reportTargets, toAppliedTargetReport(target.Delivery, target.Plan, applyResult))
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
		applyResult, err := target.Adapter.ApplyUpdate(ctx, ports.ApplyInput{
			Plan:           target.Plan,
			Manifest:       *target.Manifest,
			ResolvedSource: target.Resolved,
			Policy:         record.Policy,
			Inspect:        target.Inspect,
			Record:         &record,
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
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "update failed after partial progress; degraded state persisted", err)
		}
		if err := s.Journal.AppendStep(ctx, operationID, domain.JournalStep{Target: string(target.TargetID), Action: "apply", Status: "ok"}); err != nil {
			return domain.Report{}, err
		}
		nextRecord.Targets[target.TargetID] = targetInstallationFromExisting(target, applyResult)
		applyManifestMetadata(&nextRecord, *target.Manifest, startedAt)
		reportTargets = append(reportTargets, toAppliedTargetReport(target.Delivery, target.Plan, applyResult))
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
		ActionClass:              plan.ActionClass,
		State:                    string(inspect.State),
		ActivationState:          string(inspect.ActivationState),
		InteractiveAuthState:     inspect.InteractiveAuthState,
		RestartRequired:          plan.RestartRequired,
		ReloadRequired:           plan.ReloadRequired,
		NewThreadRequired:        plan.NewThreadRequired,
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
	inspect, err := adapter.Inspect(ctx, ports.InspectInput{Scope: policy.Scope})
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
				state.Installations = upsertInstallation(state.Installations, degradedRecordFromApplied(manifest, policy, startedAt, rollbackFailed))
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
		applied = append(applied, appliedTargetInstall{Planned: target, Result: applyResult})
		reportTargets = append(reportTargets, toAppliedTargetReport(target.Delivery, target.Plan, applyResult))
	}

	state.Installations = upsertInstallation(state.Installations, installationRecordFromApplied(manifest, policy, startedAt, applied))
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

func toAppliedTargetReport(delivery domain.Delivery, plan ports.AdapterPlan, result ports.ApplyResult) domain.TargetReport {
	report := domain.TargetReport{
		TargetID:             string(delivery.TargetID),
		DeliveryKind:         string(delivery.DeliveryKind),
		ActionClass:          plan.ActionClass,
		State:                string(result.State),
		ActivationState:      string(result.ActivationState),
		InteractiveAuthState: result.InteractiveAuthState,
		RestartRequired:      result.RestartRequired,
		ReloadRequired:       result.ReloadRequired,
		NewThreadRequired:    result.NewThreadRequired,
		SourceAccessState:    result.SourceAccessState,
		EvidenceKey:          plan.EvidenceKey,
		ManualSteps:          append([]string(nil), result.ManualSteps...),
	}
	for _, restriction := range result.EnvironmentRestrictions {
		report.EnvironmentRestrictions = append(report.EnvironmentRestrictions, string(restriction))
	}
	return report
}

func installationRecordFromApplied(manifest domain.IntegrationManifest, policy domain.InstallPolicy, startedAt string, applied []appliedTargetInstall) domain.InstallationRecord {
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
		Targets:            targets,
		LastCheckedAt:      startedAt,
		LastUpdatedAt:      startedAt,
	}
}

func degradedRecordFromApplied(manifest domain.IntegrationManifest, policy domain.InstallPolicy, startedAt string, applied []appliedTargetInstall) domain.InstallationRecord {
	record := installationRecordFromApplied(manifest, policy, startedAt, applied)
	for key, target := range record.Targets {
		target.State = domain.InstallDegraded
		record.Targets[key] = target
	}
	return record
}

func targetInstallationFromApplied(item appliedTargetInstall) domain.TargetInstallation {
	return domain.TargetInstallation{
		TargetID:                item.Planned.TargetID,
		DeliveryKind:            item.Planned.Delivery.DeliveryKind,
		State:                   item.Result.State,
		NativeRef:               item.Planned.Delivery.NativeRefHint,
		ActivationState:         item.Result.ActivationState,
		InteractiveAuthState:    item.Result.InteractiveAuthState,
		EnvironmentRestrictions: append([]domain.EnvironmentRestrictionCode(nil), item.Result.EnvironmentRestrictions...),
		SourceAccessState:       firstNonEmpty(item.Result.SourceAccessState, item.Planned.Inspect.SourceAccessState),
		OwnedNativeObjects:      append([]domain.NativeObjectRef(nil), item.Result.OwnedNativeObjects...),
		AdapterMetadata:         cloneMetadata(item.Result.AdapterMetadata),
	}
}

func targetInstallationFromExisting(item plannedExistingTarget, result ports.ApplyResult) domain.TargetInstallation {
	out := domain.TargetInstallation{
		TargetID:                item.TargetID,
		DeliveryKind:            item.Delivery.DeliveryKind,
		State:                   result.State,
		NativeRef:               firstNonEmpty(item.Delivery.NativeRefHint, item.Current.NativeRef),
		ActivationState:         result.ActivationState,
		InteractiveAuthState:    result.InteractiveAuthState,
		EnvironmentRestrictions: append([]domain.EnvironmentRestrictionCode(nil), result.EnvironmentRestrictions...),
		SourceAccessState:       firstNonEmpty(result.SourceAccessState, item.Inspect.SourceAccessState),
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
	degraded := append([]domain.TargetID{failedTarget}, rollbackFailed...)
	for _, targetID := range degraded {
		target, ok := next.Targets[targetID]
		if !ok {
			continue
		}
		target.State = domain.InstallDegraded
		next.Targets[targetID] = target
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
			Targets: map[domain.TargetID]domain.TargetInstallation{
				item.Planned.TargetID: targetInstallationFromApplied(item),
			},
			LastCheckedAt: startedAt,
			LastUpdatedAt: startedAt,
		}
		inspect, err := item.Planned.Adapter.Inspect(ctx, ports.InspectInput{Record: &record, Scope: policy.Scope})
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
		inspect, err := item.Planned.Adapter.Inspect(ctx, ports.InspectInput{Record: &record, Scope: record.Policy.Scope})
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
	out.EnvironmentRestrictions = append([]domain.EnvironmentRestrictionCode(nil), in.EnvironmentRestrictions...)
	out.OwnedNativeObjects = append([]domain.NativeObjectRef(nil), in.OwnedNativeObjects...)
	out.AdapterMetadata = cloneMetadata(in.AdapterMetadata)
	return out
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
