package usecase

import (
	"context"
	"fmt"
	"sort"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type plannedTargetInstall struct {
	TargetID domain.TargetID
	Delivery domain.Delivery
	Adapter  ports.TargetAdapter
	Inspect  ports.InspectResult
	Plan     ports.AdapterPlan
}

type appliedTargetInstall struct {
	Planned plannedTargetInstall
	Result  ports.ApplyResult
	Verify  ports.InspectResult
}

func (s Service) add(ctx context.Context, in AddInput) (domain.Report, error) {
	resolved, err := s.SourceResolver.Resolve(ctx, domain.IntegrationRef{Raw: in.Source})
	if err != nil {
		return domain.Report{}, err
	}
	defer cleanupResolvedSource(resolved)
	manifest, err := s.ManifestLoader.Load(ctx, resolved)
	if err != nil {
		return domain.Report{}, err
	}
	selectedTargets, err := resolveRequestedTargets(manifest, in.Targets)
	if err != nil {
		return domain.Report{}, err
	}
	policy := domain.InstallPolicy{
		Scope:           defaultString(in.Scope, "user"),
		AutoUpdate:      defaultBool(in.AutoUpdate, true),
		AdoptNewTargets: defaultString(in.AdoptNewTargets, "manual"),
		AllowPrerelease: defaultBool(in.AllowPrerelease, false),
	}
	opPrefix := "add"
	summary := fmt.Sprintf("Install plan for integration %q at version %s.", manifest.IntegrationID, manifest.Version)
	if in.DryRun {
		opPrefix = "plan_add"
		summary = fmt.Sprintf("Dry-run plan for integration %q at version %s.", manifest.IntegrationID, manifest.Version)
	}
	report := domain.Report{
		OperationID: operationID(opPrefix, manifest.IntegrationID, s.now()),
		Summary:     summary,
	}
	planned := make([]plannedTargetInstall, 0, len(selectedTargets))
	for _, target := range selectedTargets {
		item, err := s.planTargetInstall(ctx, manifest, policy, target)
		if err != nil {
			return domain.Report{}, err
		}
		planned = append(planned, item)
		report.Targets = append(report.Targets, toTargetReport(item.Delivery, item.Inspect, item.Plan))
	}
	sort.Slice(report.Targets, func(i, j int) bool { return report.Targets[i].TargetID < report.Targets[j].TargetID })
	if in.DryRun {
		return report, nil
	}
	for _, target := range planned {
		if target.Plan.Blocking {
			return domain.Report{}, domain.NewError(domain.ErrMutationApply, "planned mutation is blocked for target "+string(target.TargetID)+"; rerun with --dry-run to inspect manual steps", nil)
		}
	}
	return s.applyAdd(ctx, report.OperationID, manifest, resolved, policy, planned)
}
