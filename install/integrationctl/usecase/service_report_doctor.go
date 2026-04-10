package usecase

import (
	"fmt"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func buildDoctorReport(installations []domain.InstallationRecord) (domain.Report, int, int, int) {
	var degradedCount, activationPendingCount, authPendingCount int
	report := domain.Report{}
	for _, inst := range installations {
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
	return report, degradedCount, activationPendingCount, authPendingCount
}

func sortDoctorTargets(targets []domain.TargetReport) {
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].TargetID == targets[j].TargetID {
			return targets[i].DeliveryKind < targets[j].DeliveryKind
		}
		return targets[i].TargetID < targets[j].TargetID
	})
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
