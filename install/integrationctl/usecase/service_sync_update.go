package usecase

import (
	"context"
	"sort"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func (s Service) appendBatchUpdateResult(ctx context.Context, dryRun bool, record domain.InstallationRecord, report *domain.Report) bool {
	item, err := s.planExisting(ctx, NamedDryRunInput{Name: record.IntegrationID, DryRun: dryRun}, "update_version")
	if err != nil {
		report.Warnings = append(report.Warnings, "Update skipped for "+record.IntegrationID+": "+err.Error())
		return false
	}
	report.Targets = append(report.Targets, item.Targets...)
	return true
}

func sortSyncReportTargets(report *domain.Report) {
	sort.Slice(report.Targets, func(i, j int) bool {
		if report.Targets[i].TargetID == report.Targets[j].TargetID {
			return report.Targets[i].DeliveryKind < report.Targets[j].DeliveryKind
		}
		return report.Targets[i].TargetID < report.Targets[j].TargetID
	})
}
