package usecase

import (
	"context"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func (s Service) syncUndesiredIntegrations(ctx context.Context, dryRun bool, installations []domain.InstallationRecord, desiredIDs map[string]struct{}, report *domain.Report) {
	for _, record := range installations {
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
}
