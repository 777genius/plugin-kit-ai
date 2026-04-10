package usecase

import (
	"context"
	"fmt"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func (s Service) runSync(ctx context.Context, dryRun bool) (domain.Report, error) {
	lock, err := s.loadWorkspaceLockForSync(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	state, current, err := s.loadCurrentInstallations(ctx)
	if err != nil {
		return domain.Report{}, err
	}

	report := s.newWorkspaceSyncReport(len(lock.Integrations))
	desiredIDs := map[string]struct{}{}
	s.syncDesiredIntegrations(ctx, dryRun, lock.Integrations, current, desiredIDs, &report)
	s.syncUndesiredIntegrations(ctx, dryRun, state.Installations, desiredIDs, &report)
	finalizeWorkspaceSyncReport(&report)
	return report, nil
}

func (s Service) newWorkspaceSyncReport(desiredCount int) domain.Report {
	return domain.Report{
		OperationID: operationID("sync", "workspace", s.now()),
		Summary:     fmt.Sprintf("Processed workspace sync for %d desired integration(s).", desiredCount),
	}
}

func (s Service) syncDesiredIntegrations(
	ctx context.Context,
	dryRun bool,
	items []domain.WorkspaceLockIntegration,
	current map[string]domain.InstallationRecord,
	desiredIDs map[string]struct{},
	report *domain.Report,
) {
	for _, item := range items {
		s.syncDesiredIntegration(ctx, dryRun, item, current, desiredIDs, report)
	}
}

func finalizeWorkspaceSyncReport(report *domain.Report) {
	if len(report.Targets) == 0 && len(report.Warnings) == 0 {
		report.Summary = "Workspace sync found no changes."
	}
}
