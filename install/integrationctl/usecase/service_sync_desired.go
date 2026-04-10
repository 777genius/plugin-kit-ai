package usecase

import (
	"context"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func (s Service) syncDesiredIntegration(
	ctx context.Context,
	dryRun bool,
	item domain.WorkspaceLockIntegration,
	current map[string]domain.InstallationRecord,
	desiredIDs map[string]struct{},
	report *domain.Report,
) {
	source := resolveWorkspaceLockSource(s.WorkspaceLock.Path(), item.Source)
	resolved, manifest, err := s.resolveDesiredSourceManifest(ctx, source)
	if err != nil {
		report.Warnings = append(report.Warnings, "Sync skipped for source "+item.Source+": "+err.Error())
		return
	}
	cleanupResolvedSource(resolved)
	desiredIDs[manifest.IntegrationID] = struct{}{}
	desiredPolicy := desiredPolicyFromLock(item.Policy)
	targets, err := resolveRequestedTargets(manifest, item.Targets)
	if err != nil {
		report.Warnings = append(report.Warnings, "Sync skipped for "+manifest.IntegrationID+": "+err.Error())
		return
	}

	record, exists := current[manifest.IntegrationID]
	switch {
	case !exists:
		s.syncDesiredAdd(ctx, dryRun, manifest.IntegrationID, source, desiredPolicy, targets, report)
	case syncNeedsReplace(record, source, desiredPolicy, targets, item.Version):
		s.syncDesiredReplace(ctx, dryRun, record, manifest.IntegrationID, source, desiredPolicy, targets, report)
	case syncNeedsUpdate(record, source, item.Version):
		s.syncDesiredUpdate(ctx, dryRun, manifest.IntegrationID, report)
	default:
		report.Warnings = append(report.Warnings, "Sync no-op for "+manifest.IntegrationID+": desired state already matches workspace intent")
	}
}

func (s Service) syncDesiredAdd(ctx context.Context, dryRun bool, integrationID, source string, desiredPolicy domain.InstallPolicy, targets []domain.TargetID, report *domain.Report) {
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
		report.Warnings = append(report.Warnings, "Sync add failed for "+integrationID+": "+err.Error())
		return
	}
	report.Targets = append(report.Targets, itemReport.Targets...)
}

func (s Service) syncDesiredReplace(ctx context.Context, dryRun bool, record domain.InstallationRecord, integrationID, source string, desiredPolicy domain.InstallPolicy, targets []domain.TargetID, report *domain.Report) {
	if record.Policy.Scope != "project" || desiredPolicy.Scope != "project" {
		report.Warnings = append(report.Warnings, "Sync skipped replace for "+integrationID+": replacing non-project scoped integrations is blocked")
		return
	}
	removeReport, err := s.Remove(ctx, NamedDryRunInput{Name: record.IntegrationID, DryRun: dryRun})
	if err != nil {
		report.Warnings = append(report.Warnings, "Sync remove-before-add failed for "+integrationID+": "+err.Error())
		return
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
		report.Warnings = append(report.Warnings, "Sync re-add failed for "+integrationID+": "+err.Error())
		report.Targets = append(report.Targets, removeReport.Targets...)
		return
	}
	report.Targets = append(report.Targets, removeReport.Targets...)
	report.Targets = append(report.Targets, addReport.Targets...)
}

func (s Service) syncDesiredUpdate(ctx context.Context, dryRun bool, integrationID string, report *domain.Report) {
	itemReport, err := s.Update(ctx, NamedDryRunInput{Name: integrationID, DryRun: dryRun})
	if err != nil {
		report.Warnings = append(report.Warnings, "Sync update failed for "+integrationID+": "+err.Error())
		return
	}
	report.Targets = append(report.Targets, itemReport.Targets...)
}
