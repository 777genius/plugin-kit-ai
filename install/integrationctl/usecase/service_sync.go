package usecase

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func (s Service) updateAll(ctx context.Context, dryRun bool) (domain.Report, error) {
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

func (s Service) sync(ctx context.Context, dryRun bool) (domain.Report, error) {
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
