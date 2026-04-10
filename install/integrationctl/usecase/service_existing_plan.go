package usecase

import (
	"context"
	"fmt"
	"sort"

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
