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
	source, manifest, err := s.resolveDesiredSyncManifest(ctx, item)
	if err != nil {
		report.Warnings = append(report.Warnings, syncDesiredSourceWarning(item.Source, err))
		return
	}
	desiredIDs[manifest.IntegrationID] = struct{}{}
	desiredPolicy, targets, err := resolveDesiredSyncTargets(manifest, item)
	if err != nil {
		report.Warnings = append(report.Warnings, syncDesiredManifestWarning(manifest.IntegrationID, err))
		return
	}

	record, exists := current[manifest.IntegrationID]
	switch classifyDesiredSyncAction(record, exists, source, desiredPolicy, targets, item.Version) {
	case desiredSyncActionAdd:
		s.syncDesiredAdd(ctx, dryRun, manifest.IntegrationID, source, desiredPolicy, targets, report)
	case desiredSyncActionReplace:
		s.syncDesiredReplace(ctx, dryRun, record, manifest.IntegrationID, source, desiredPolicy, targets, report)
	case desiredSyncActionUpdate:
		s.syncDesiredUpdate(ctx, dryRun, manifest.IntegrationID, report)
	default:
		report.Warnings = append(report.Warnings, syncDesiredNoopWarning(manifest.IntegrationID))
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

type desiredSyncAction string

const (
	desiredSyncActionAdd     desiredSyncAction = "add"
	desiredSyncActionReplace desiredSyncAction = "replace"
	desiredSyncActionUpdate  desiredSyncAction = "update"
	desiredSyncActionNoop    desiredSyncAction = "noop"
)

func (s Service) resolveDesiredSyncManifest(ctx context.Context, item domain.WorkspaceLockIntegration) (string, domain.IntegrationManifest, error) {
	source := resolveWorkspaceLockSource(s.WorkspaceLock.Path(), item.Source)
	resolved, manifest, err := s.resolveDesiredSourceManifest(ctx, source)
	if err != nil {
		return source, domain.IntegrationManifest{}, err
	}
	cleanupResolvedSource(resolved)
	return source, manifest, nil
}

func resolveDesiredSyncTargets(manifest domain.IntegrationManifest, item domain.WorkspaceLockIntegration) (domain.InstallPolicy, []domain.TargetID, error) {
	desiredPolicy := desiredPolicyFromLock(item.Policy)
	targets, err := resolveRequestedTargets(manifest, item.Targets)
	if err != nil {
		return domain.InstallPolicy{}, nil, err
	}
	return desiredPolicy, targets, nil
}

func classifyDesiredSyncAction(record domain.InstallationRecord, exists bool, source string, desiredPolicy domain.InstallPolicy, targets []domain.TargetID, desiredVersion string) desiredSyncAction {
	switch {
	case !exists:
		return desiredSyncActionAdd
	case syncNeedsReplace(record, source, desiredPolicy, targets, desiredVersion):
		return desiredSyncActionReplace
	case syncNeedsUpdate(record, source, desiredVersion):
		return desiredSyncActionUpdate
	default:
		return desiredSyncActionNoop
	}
}

func syncDesiredSourceWarning(source string, err error) string {
	return "Sync skipped for source " + source + ": " + err.Error()
}

func syncDesiredManifestWarning(integrationID string, err error) string {
	return "Sync skipped for " + integrationID + ": " + err.Error()
}

func syncDesiredNoopWarning(integrationID string) string {
	return "Sync no-op for " + integrationID + ": desired state already matches workspace intent"
}
