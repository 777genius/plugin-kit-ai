package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (s Service) list(ctx context.Context) (domain.Report, error) {
	state, err := s.StateStore.Load(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	if len(state.Installations) == 0 {
		return domain.Report{Summary: "No managed integrations are installed yet."}, nil
	}
	report := domain.Report{Summary: fmt.Sprintf("%d managed integration(s) in state.", len(state.Installations))}
	for _, inst := range state.Installations {
		targets := sortedTargets(inst.Targets)
		for _, targetID := range targets {
			ti := inst.Targets[targetID]
			report.Targets = append(report.Targets, domain.TargetReport{
				TargetID:          string(targetID),
				DeliveryKind:      string(ti.DeliveryKind),
				CapabilitySurface: append([]string(nil), ti.CapabilitySurface...),
				State:             string(ti.State),
				ActivationState:   string(ti.ActivationState),
				CatalogPolicy:     cloneCatalogPolicy(ti.CatalogPolicy),
				SourceAccessState: ti.SourceAccessState,
			})
		}
	}
	return report, nil
}

func (s Service) doctor(ctx context.Context) (domain.Report, error) {
	state, err := s.StateStore.Load(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	openOps, err := s.Journal.ListOpen(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	var degradedCount, activationPendingCount, authPendingCount int
	report := domain.Report{}
	for _, inst := range state.Installations {
		for _, targetID := range sortedTargets(inst.Targets) {
			ti := inst.Targets[targetID]
			if !doctorTargetNeedsAttention(ti) {
				continue
			}
			switch ti.State {
			case domain.InstallDegraded:
				degradedCount++
			case domain.InstallActivationPending:
				activationPendingCount++
			case domain.InstallAuthPending:
				authPendingCount++
			}
			report.Targets = append(report.Targets, domain.TargetReport{
				TargetID:                string(targetID),
				DeliveryKind:            string(ti.DeliveryKind),
				CapabilitySurface:       append([]string(nil), ti.CapabilitySurface...),
				ActionClass:             "doctor_attention",
				State:                   string(ti.State),
				ActivationState:         string(ti.ActivationState),
				InteractiveAuthState:    ti.InteractiveAuthState,
				CatalogPolicy:           cloneCatalogPolicy(ti.CatalogPolicy),
				EnvironmentRestrictions: restrictionsToStrings(ti.EnvironmentRestrictions),
				SourceAccessState:       ti.SourceAccessState,
				ManualSteps:             doctorManualSteps(inst.IntegrationID, ti),
			})
		}
	}
	report.Summary = fmt.Sprintf("Doctor: %d installation(s), %d open operation journal(s), %d degraded target(s), %d activation-pending target(s), %d auth-pending target(s).", len(state.Installations), len(openOps), degradedCount, activationPendingCount, authPendingCount)
	for _, op := range openOps {
		report.Warnings = append(report.Warnings, doctorWarningForOperation(op))
	}
	sort.Slice(report.Targets, func(i, j int) bool {
		if report.Targets[i].TargetID == report.Targets[j].TargetID {
			return report.Targets[i].DeliveryKind < report.Targets[j].DeliveryKind
		}
		return report.Targets[i].TargetID < report.Targets[j].TargetID
	})
	return report, nil
}

func toTargetReport(delivery domain.Delivery, inspect ports.InspectResult, plan ports.AdapterPlan) domain.TargetReport {
	report := domain.TargetReport{
		TargetID:                 string(delivery.TargetID),
		DeliveryKind:             string(delivery.DeliveryKind),
		CapabilitySurface:        append([]string(nil), delivery.CapabilitySurface...),
		ActionClass:              plan.ActionClass,
		State:                    string(inspect.State),
		ActivationState:          string(inspect.ActivationState),
		InteractiveAuthState:     inspect.InteractiveAuthState,
		RestartRequired:          plan.RestartRequired,
		ReloadRequired:           plan.ReloadRequired,
		NewThreadRequired:        plan.NewThreadRequired,
		CatalogPolicy:            cloneCatalogPolicy(inspect.CatalogPolicy),
		VolatileOverrideDetected: inspect.VolatileOverrideDetected,
		TrustResolutionSource:    inspect.TrustResolutionSource,
		SourceAccessState:        inspect.SourceAccessState,
		EvidenceKey:              plan.EvidenceKey,
		ManualSteps:              append([]string(nil), plan.ManualSteps...),
	}
	for _, restriction := range inspect.EnvironmentRestrictions {
		report.EnvironmentRestrictions = append(report.EnvironmentRestrictions, string(restriction))
	}
	return report
}

func toAppliedTargetReport(delivery domain.Delivery, inspect ports.InspectResult, verified ports.InspectResult, plan ports.AdapterPlan, result ports.ApplyResult) domain.TargetReport {
	state := result.State
	if verified.State != "" {
		state = verified.State
	}
	activationState := result.ActivationState
	if verified.ActivationState != "" {
		activationState = verified.ActivationState
	}
	interactiveAuthState := result.InteractiveAuthState
	if strings.TrimSpace(verified.InteractiveAuthState) != "" {
		interactiveAuthState = verified.InteractiveAuthState
	}
	sourceAccessState := firstNonEmpty(verified.SourceAccessState, result.SourceAccessState)
	environmentRestrictions := append([]domain.EnvironmentRestrictionCode(nil), result.EnvironmentRestrictions...)
	if len(verified.EnvironmentRestrictions) > 0 {
		environmentRestrictions = append([]domain.EnvironmentRestrictionCode(nil), verified.EnvironmentRestrictions...)
	}
	report := domain.TargetReport{
		TargetID:             string(delivery.TargetID),
		DeliveryKind:         string(delivery.DeliveryKind),
		CapabilitySurface:    append([]string(nil), delivery.CapabilitySurface...),
		ActionClass:          plan.ActionClass,
		State:                string(state),
		ActivationState:      string(activationState),
		InteractiveAuthState: interactiveAuthState,
		RestartRequired:      result.RestartRequired,
		ReloadRequired:       result.ReloadRequired,
		NewThreadRequired:    result.NewThreadRequired,
		CatalogPolicy:        cloneCatalogPolicy(firstNonNilCatalogPolicy(verified.CatalogPolicy, inspect.CatalogPolicy)),
		SourceAccessState:    sourceAccessState,
		EvidenceKey:          plan.EvidenceKey,
		ManualSteps:          append([]string(nil), result.ManualSteps...),
	}
	for _, restriction := range environmentRestrictions {
		report.EnvironmentRestrictions = append(report.EnvironmentRestrictions, string(restriction))
	}
	return report
}

func actionNamePrefix(action string) string {
	switch action {
	case "update_version":
		return "update"
	case "remove_orphaned_target":
		return "remove"
	case "repair_drift":
		return "repair"
	case "enable_target":
		return "enable"
	case "disable_target":
		return "disable"
	default:
		return "operation"
	}
}

func summaryForExisting(action, integrationID string) string {
	switch action {
	case "update_version":
		return fmt.Sprintf("Updated integration %q.", integrationID)
	case "remove_orphaned_target":
		return fmt.Sprintf("Removed managed targets from integration %q.", integrationID)
	case "repair_drift":
		return fmt.Sprintf("Repaired managed targets for integration %q.", integrationID)
	case "enable_target":
		return fmt.Sprintf("Enabled managed targets for integration %q.", integrationID)
	case "disable_target":
		return fmt.Sprintf("Disabled managed targets for integration %q.", integrationID)
	default:
		return fmt.Sprintf("Applied %s for %q.", action, integrationID)
	}
}

func doctorTargetNeedsAttention(ti domain.TargetInstallation) bool {
	if ti.State == domain.InstallDegraded || ti.State == domain.InstallActivationPending || ti.State == domain.InstallAuthPending {
		return true
	}
	switch ti.ActivationState {
	case domain.ActivationNativePending, domain.ActivationReloadPending, domain.ActivationRestartPending, domain.ActivationNewThreadPending:
		return true
	}
	return false
}

func doctorManualSteps(integrationID string, ti domain.TargetInstallation) []string {
	steps := []string{}
	switch ti.State {
	case domain.InstallDegraded:
		steps = append(steps, "run plugin-kit-ai integrations repair "+integrationID)
	case domain.InstallActivationPending:
		steps = append(steps, "complete the vendor-native activation step for this target")
	case domain.InstallAuthPending:
		steps = append(steps, "complete the required authentication flow for this target")
	}
	for _, restriction := range ti.EnvironmentRestrictions {
		switch restriction {
		case domain.RestrictionNewThreadRequired:
			steps = append(steps, "start a new agent thread before using the integration")
		case domain.RestrictionReloadRequired:
			steps = append(steps, "reload the current agent session to pick up the integration")
		case domain.RestrictionRestartRequired:
			steps = append(steps, "restart the agent CLI or desktop app")
		case domain.RestrictionNativeActivation:
			steps = append(steps, "finish the native activation flow in the target agent")
		case domain.RestrictionNativeAuthRequired, domain.RestrictionSourceAuthRequired:
			steps = append(steps, "complete the missing authentication step and rerun repair")
		}
	}
	return dedupeStrings(steps)
}

func doctorWarningForOperation(op domain.OperationRecord) string {
	switch op.Status {
	case "degraded":
		return fmt.Sprintf("Operation %s for %s ended degraded - run plugin-kit-ai integrations repair %s.", op.OperationID, op.IntegrationID, op.IntegrationID)
	case "in_progress":
		return fmt.Sprintf("Operation %s for %s is still marked in_progress - inspect the journal and rerun repair if the process was interrupted.", op.OperationID, op.IntegrationID)
	case "failed":
		return fmt.Sprintf("Operation %s for %s failed before commit - inspect the journal and rerun the desired lifecycle command.", op.OperationID, op.IntegrationID)
	default:
		return fmt.Sprintf("Open operation %s for %s is still marked %s.", op.OperationID, op.IntegrationID, op.Status)
	}
}

func restrictionsToStrings(in []domain.EnvironmentRestrictionCode) []string {
	out := make([]string, 0, len(in))
	for _, restriction := range in {
		out = append(out, string(restriction))
	}
	return out
}

func dedupeStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
