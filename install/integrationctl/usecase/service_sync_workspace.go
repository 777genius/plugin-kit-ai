package usecase

import (
	"context"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func (s Service) runSync(ctx context.Context, dryRun bool) (domain.Report, error) {
	lock, state, current, err := s.loadSyncWorkspaceState(ctx)
	if err != nil {
		return domain.Report{}, err
	}

	report := s.newWorkspaceSyncReport(len(lock.Integrations))
	desiredIDs := newSyncDesiredIDs()
	s.syncDesiredIntegrations(ctx, dryRun, lock.Integrations, current, desiredIDs, &report)
	s.syncUndesiredIntegrations(ctx, dryRun, state.Installations, desiredIDs, &report)
	finalizeWorkspaceSyncReport(&report)
	return report, nil
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
