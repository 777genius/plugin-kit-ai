package usecase

import (
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func TestValidateExistingUpdateTargetsRejectsEmptyPlans(t *testing.T) {
	t.Parallel()

	err := validateExistingUpdateTargets(nil)
	if err == nil || !strings.Contains(err.Error(), "update requires at least one planned target") {
		t.Fatalf("err = %v", err)
	}
}

func TestFinalizeExistingUpdateStateUpsertsRecord(t *testing.T) {
	t.Parallel()

	state := finalizeExistingUpdateState(ports.StateFile{}, domain.InstallationRecord{IntegrationID: "demo"})
	if len(state.Installations) != 1 || state.Installations[0].IntegrationID != "demo" {
		t.Fatalf("state = %+v", state)
	}
}

func TestExistingUpdateReportSortsTargets(t *testing.T) {
	t.Parallel()

	report := existingUpdateReport("op", "demo", []domain.TargetReport{
		{TargetID: "gemini"},
		{TargetID: "claude"},
	})
	if len(report.Targets) != 2 || report.Targets[0].TargetID != "claude" || report.Targets[1].TargetID != "gemini" {
		t.Fatalf("targets = %+v", report.Targets)
	}
}
