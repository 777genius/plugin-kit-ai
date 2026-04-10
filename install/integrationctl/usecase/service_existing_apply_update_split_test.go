package usecase

import (
	"errors"
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

func TestBuildExistingUpdateApplyInputCarriesMutationInputs(t *testing.T) {
	t.Parallel()

	input := buildExistingUpdateApplyInput(domain.InstallationRecord{IntegrationID: "demo"}, plannedExistingTarget{
		Plan: ports.AdapterPlan{ActionClass: "update"},
		Manifest: &domain.IntegrationManifest{
			Version: "1.2.3",
		},
		Resolved: &ports.ResolvedSource{Kind: "bundle"},
	})
	if input.Plan.ActionClass != "update" || input.Manifest.Version != "1.2.3" || input.ResolvedSource == nil || input.ResolvedSource.Kind != "bundle" || input.Record == nil || input.Record.IntegrationID != "demo" {
		t.Fatalf("input = %+v", input)
	}
}

func TestExistingUpdateVerifyRecordDelegatesProvisionalRecord(t *testing.T) {
	t.Parallel()

	record := domain.InstallationRecord{
		IntegrationID: "demo",
		Targets: map[domain.TargetID]domain.TargetInstallation{
			"gemini": {TargetID: "gemini"},
		},
	}
	target := plannedExistingTarget{
		TargetID: "gemini",
		Manifest: &domain.IntegrationManifest{
			Version: "2.0.0",
		},
		Resolved: &ports.ResolvedSource{Resolved: domain.ResolvedSourceRef{Value: "registry.example/demo@2.0.0"}},
	}
	got := existingUpdateVerifyRecord(record, target, ports.ApplyResult{})
	if got.ResolvedVersion != "2.0.0" || got.Targets["gemini"].TargetID != "gemini" {
		t.Fatalf("record = %+v", got)
	}
}

func TestDegradedExistingUpdateStateMarksTargetAndMetadata(t *testing.T) {
	t.Parallel()

	state := degradedExistingUpdateState(ports.StateFile{}, domain.InstallationRecord{
		IntegrationID: "demo",
		Targets:       map[domain.TargetID]domain.TargetInstallation{},
	}, plannedExistingTarget{
		TargetID: "gemini",
		Delivery: domain.Delivery{TargetID: "gemini"},
		Manifest: &domain.IntegrationManifest{
			Version:        "1.2.3",
			ResolvedRef:    domain.ResolvedSourceRef{Value: "registry.example/demo@1.2.3"},
			SourceDigest:   "source",
			ManifestDigest: "manifest",
		},
	}, "2026-04-10T20:00:00Z")
	if len(state.Installations) != 1 || state.Installations[0].ResolvedVersion != "1.2.3" || state.Installations[0].Targets["gemini"].State != domain.InstallDegraded {
		t.Fatalf("state = %+v", state)
	}
}

func TestExistingUpdateFailureErrorWrapsMutationApply(t *testing.T) {
	t.Parallel()

	err := existingUpdateFailureError("update failed after partial progress; degraded state persisted", errors.New("cause"))
	if err == nil || !strings.Contains(err.Error(), "update failed after partial progress; degraded state persisted") {
		t.Fatalf("err = %v", err)
	}
}
