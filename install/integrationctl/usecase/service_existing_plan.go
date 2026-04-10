package usecase

import (
	"context"
	"fmt"
	"sort"
	"time"

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
	record, err := s.loadExistingPlanRecord(ctx, in.Name)
	if err != nil {
		return domain.Report{}, err
	}
	report := newExistingPlanReport(action, record.IntegrationID, s.now())
	sharedResolved, sharedManifest, cleanupShared, err := s.resolveExistingSharedSource(ctx, record, action, in.DryRun)
	if err != nil {
		return domain.Report{}, err
	}
	defer cleanupShared()
	selectedTargetIDs, err := s.selectExistingTargets(record, in.Target, action)
	if err != nil {
		return domain.Report{}, err
	}
	planned, err := s.planSelectedExistingTargets(ctx, record, selectedTargetIDs, action, sharedResolved, sharedManifest)
	if err != nil {
		return domain.Report{}, err
	}
	appendExistingTargetReports(&report, planned)
	adopted, warnings, err := s.planAdoptedExistingTargets(ctx, record, action, sharedResolved, sharedManifest)
	if err != nil {
		cleanupPlannedExisting(planned)
		return domain.Report{}, err
	}
	planned = append(planned, adopted...)
	appendExistingTargetReports(&report, adopted)
	report.Warnings = append(report.Warnings, warnings...)
	report = finalizeExistingPlanReport(report)
	if in.DryRun {
		cleanupPlannedExisting(planned)
		return report, nil
	}
	return s.applyExisting(ctx, record, action, planned)
}

func (s Service) planSelectedExistingTargets(
	ctx context.Context,
	record domain.InstallationRecord,
	targetIDs []domain.TargetID,
	action string,
	sharedResolved *ports.ResolvedSource,
	sharedManifest *domain.IntegrationManifest,
) ([]plannedExistingTarget, error) {
	planned := make([]plannedExistingTarget, 0, len(targetIDs))
	for _, targetID := range targetIDs {
		item, err := s.planExistingTarget(ctx, record, targetID, action, sharedResolved, sharedManifest)
		if err != nil {
			cleanupPlannedExisting(planned)
			return nil, err
		}
		planned = append(planned, item)
	}
	return planned, nil
}

func appendExistingTargetReports(report *domain.Report, planned []plannedExistingTarget) {
	for _, item := range planned {
		report.Targets = append(report.Targets, item.Report)
	}
}

func shouldPlanExistingAdoptedTargets(action string, sharedResolved *ports.ResolvedSource, sharedManifest *domain.IntegrationManifest) bool {
	return action == "update_version" && sharedResolved != nil && sharedManifest != nil
}

func (s Service) planAdoptedExistingTargets(
	ctx context.Context,
	record domain.InstallationRecord,
	action string,
	sharedResolved *ports.ResolvedSource,
	sharedManifest *domain.IntegrationManifest,
) ([]plannedExistingTarget, []string, error) {
	if !shouldPlanExistingAdoptedTargets(action, sharedResolved, sharedManifest) {
		return nil, nil, nil
	}
	return s.planAdoptedUpdateTargets(ctx, record, *sharedManifest, *sharedResolved)
}

func finalizeExistingPlanReport(report domain.Report) domain.Report {
	sort.Slice(report.Targets, func(i, j int) bool { return report.Targets[i].TargetID < report.Targets[j].TargetID })
	return report
}

func (s Service) loadExistingPlanRecord(ctx context.Context, name string) (domain.InstallationRecord, error) {
	state, err := s.StateStore.Load(ctx)
	if err != nil {
		return domain.InstallationRecord{}, err
	}
	record, ok := findInstallation(state.Installations, name)
	if !ok {
		return domain.InstallationRecord{}, domain.NewError(domain.ErrStateConflict, "integration not found in state: "+name, nil)
	}
	return record, nil
}

func newExistingPlanReport(action, integrationID string, now time.Time) domain.Report {
	return domain.Report{
		OperationID: operationID("plan_"+action, integrationID, now),
		Summary:     fmt.Sprintf("Dry-run %s plan for %q.", action, integrationID),
	}
}

func shouldResolveExistingSharedSource(action string, dryRun bool) bool {
	if action == "update_version" {
		return true
	}
	if !dryRun && (action == "remove_orphaned_target" || action == "repair_drift") {
		return true
	}
	return false
}

func (s Service) resolveExistingSharedSource(
	ctx context.Context,
	record domain.InstallationRecord,
	action string,
	dryRun bool,
) (*ports.ResolvedSource, *domain.IntegrationManifest, func(), error) {
	if !shouldResolveExistingSharedSource(action, dryRun) {
		return nil, nil, func() {}, nil
	}
	resolved, manifest, err := s.resolveCurrentSourceManifest(ctx, record)
	if err != nil {
		return nil, nil, nil, err
	}
	return &resolved, &manifest, func() { cleanupResolvedSource(resolved) }, nil
}
