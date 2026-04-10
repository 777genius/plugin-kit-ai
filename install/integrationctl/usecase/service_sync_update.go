package usecase

import (
	"context"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func (s Service) appendBatchUpdateResult(ctx context.Context, dryRun bool, record domain.InstallationRecord, report *domain.Report) bool {
	item, err := s.planExisting(ctx, NamedDryRunInput{Name: record.IntegrationID, DryRun: dryRun}, "update_version")
	if err != nil {
		report.Warnings = append(report.Warnings, batchUpdateSkipWarning(record.IntegrationID, err))
		return false
	}
	report.Targets = append(report.Targets, item.Targets...)
	return true
}
