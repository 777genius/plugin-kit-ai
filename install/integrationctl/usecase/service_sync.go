package usecase

import (
	"context"
	"fmt"
	"sort"

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
		if s.appendBatchUpdateResult(ctx, dryRun, record, &report) {
			successes++
		}
	}
	if successes == 0 && len(report.Warnings) > 0 {
		report.Summary = "No managed integrations were updated successfully."
	}
	sortSyncReportTargets(&report)
	return report, nil
}

func (s Service) sync(ctx context.Context, dryRun bool) (domain.Report, error) {
	lock, err := s.loadWorkspaceLockForSync(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	state, current, err := s.loadCurrentInstallations(ctx)
	if err != nil {
		return domain.Report{}, err
	}

	report := domain.Report{
		OperationID: operationID("sync", "workspace", s.now()),
		Summary:     fmt.Sprintf("Processed workspace sync for %d desired integration(s).", len(lock.Integrations)),
	}
	desiredIDs := map[string]struct{}{}

	for _, item := range lock.Integrations {
		s.syncDesiredIntegration(ctx, dryRun, item, current, desiredIDs, &report)
	}
	s.syncUndesiredIntegrations(ctx, dryRun, state.Installations, desiredIDs, &report)
	if len(report.Targets) == 0 && len(report.Warnings) == 0 {
		report.Summary = "Workspace sync found no changes."
	}
	return report, nil
}
