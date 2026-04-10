package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (s Service) planAdoptedUpdateTargets(ctx context.Context, record domain.InstallationRecord, manifest domain.IntegrationManifest, resolved ports.ResolvedSource) ([]plannedExistingTarget, []string, error) {
	existing := make(map[domain.TargetID]struct{}, len(record.Targets))
	for targetID := range record.Targets {
		existing[targetID] = struct{}{}
	}
	autoAdopt := strings.EqualFold(strings.TrimSpace(record.Policy.AdoptNewTargets), "auto")
	out := []plannedExistingTarget{}
	warnings := []string{}
	for _, delivery := range manifest.Deliveries {
		if _, ok := existing[delivery.TargetID]; ok {
			continue
		}
		item, warning, err := s.planAdoptedUpdateTarget(ctx, record, manifest, resolved, delivery, autoAdopt)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(warning) != "" {
			warnings = append(warnings, warning)
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TargetID < out[j].TargetID })
	return out, warnings, nil
}

func (s Service) planAdoptedUpdateTarget(ctx context.Context, record domain.InstallationRecord, manifest domain.IntegrationManifest, resolved ports.ResolvedSource, delivery domain.Delivery, autoAdopt bool) (plannedExistingTarget, string, error) {
	if !autoAdopt {
		return plannedExistingTarget{}, fmt.Sprintf("New target support is available for %s on %s, but adopt_new_targets=%s.", record.IntegrationID, delivery.TargetID, defaultString(record.Policy.AdoptNewTargets, "manual")), nil
	}
	adapter, ok := s.Adapters[delivery.TargetID]
	if !ok {
		return plannedExistingTarget{}, fmt.Sprintf("Automatic adoption skipped for %s on %s: no adapter is registered.", record.IntegrationID, delivery.TargetID), nil
	}
	inspect, err := adapter.Inspect(ctx, ports.InspectInput{IntegrationID: record.IntegrationID, Record: &record, Scope: record.Policy.Scope})
	if err != nil {
		return plannedExistingTarget{}, "", err
	}
	plan, err := adapter.PlanInstall(ctx, ports.PlanInstallInput{
		Manifest: manifest,
		Policy:   record.Policy,
		Inspect:  inspect,
	})
	if err != nil {
		return plannedExistingTarget{}, "", err
	}
	plan.ActionClass = "adopt_new_target"
	plan.Summary = "Adopt newly supported target " + string(delivery.TargetID)
	if _, err := s.validateEvidence(ctx, delivery.TargetID, plan.EvidenceKey); err != nil {
		return plannedExistingTarget{}, "", err
	}
	if plan.Blocking {
		return plannedExistingTarget{}, fmt.Sprintf("Automatic adoption skipped for %s on %s: native environment blocks installation.", record.IntegrationID, delivery.TargetID), nil
	}
	resolvedCopy := resolved
	manifestCopy := manifest
	return plannedExistingTarget{
		TargetID: delivery.TargetID,
		Delivery: delivery,
		Adapter:  adapter,
		Inspect:  inspect,
		Plan:     plan,
		Manifest: &manifestCopy,
		Resolved: &resolvedCopy,
		Report:   toTargetReport(delivery, inspect, plan),
		Adopted:  true,
	}, "", nil
}
