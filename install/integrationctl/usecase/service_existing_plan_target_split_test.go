package usecase

import (
	"context"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func TestLoadExistingTargetBaseRejectsMissingTarget(t *testing.T) {
	t.Parallel()

	svc := Service{}
	_, err := svc.loadExistingTargetBase(context.Background(), domain.InstallationRecord{
		IntegrationID: "demo",
		Targets:       map[domain.TargetID]domain.TargetInstallation{},
	}, domain.TargetClaude)
	if err == nil || !strings.Contains(err.Error(), "target missing from installation record: claude") {
		t.Fatalf("error = %v", err)
	}
}

func TestDispatchExistingTargetPlanRejectsUnsupportedAction(t *testing.T) {
	t.Parallel()

	svc := Service{}
	_, err := svc.dispatchExistingTargetPlan(context.Background(), domain.InstallationRecord{
		IntegrationID: "demo",
	}, "unknown_action", plannedExistingTarget{
		TargetID: domain.TargetClaude,
		Adapter:  stubTargetAdapter{id: domain.TargetClaude},
		Inspect:  ports.InspectResult{TargetID: domain.TargetClaude},
	}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported existing lifecycle action unknown_action") {
		t.Fatalf("error = %v", err)
	}
}
