package usecase

import (
	"context"
	"fmt"
	"sort"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func (s Service) runUpdateAll(ctx context.Context, dryRun bool) (domain.Report, error) {
	state, err := s.StateStore.Load(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	if len(state.Installations) == 0 {
		return domain.Report{Summary: "No managed integrations to update."}, nil
	}

	installations := sortedSyncInstallations(state.Installations)
	report := s.newBatchUpdateReport(len(installations))
	successes := s.appendBatchUpdateResults(ctx, dryRun, installations, &report)
	finalizeBatchUpdateReport(&report, successes)
	sortSyncReportTargets(&report)
	return report, nil
}

func sortedSyncInstallations(installations []domain.InstallationRecord) []domain.InstallationRecord {
	copied := append([]domain.InstallationRecord(nil), installations...)
	sort.Slice(copied, func(i, j int) bool { return copied[i].IntegrationID < copied[j].IntegrationID })
	return copied
}

func (s Service) newBatchUpdateReport(count int) domain.Report {
	return domain.Report{
		OperationID: operationID("batch_update", "all", s.now()),
		Summary:     fmt.Sprintf("Processed update for %d managed integration(s).", count),
	}
}

func (s Service) appendBatchUpdateResults(
	ctx context.Context,
	dryRun bool,
	installations []domain.InstallationRecord,
	report *domain.Report,
) int {
	successes := 0
	for _, record := range installations {
		if s.appendBatchUpdateResult(ctx, dryRun, record, report) {
			successes++
		}
	}
	return successes
}

func finalizeBatchUpdateReport(report *domain.Report, successes int) {
	if successes == 0 && len(report.Warnings) > 0 {
		report.Summary = "No managed integrations were updated successfully."
	}
}
