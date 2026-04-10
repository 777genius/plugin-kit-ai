package main

import (
	"bytes"
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/777genius/plugin-kit-ai/cli/internal/exitx"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
	"github.com/spf13/cobra"
)

func TestNormalizePublicationModelInitializesNilSlices(t *testing.T) {
	t.Parallel()
	model := normalizePublicationModel(publicationmodel.Model{
		Packages: []publicationmodel.Package{{}},
		Channels: []publicationmodel.Channel{{}},
	})
	if model.Packages[0].ChannelFamilies == nil || model.Packages[0].AuthoredInputs == nil || model.Packages[0].ManagedArtifacts == nil {
		t.Fatalf("package slices = %+v", model.Packages[0])
	}
	if model.Channels[0].PackageTargets == nil {
		t.Fatalf("channel slices = %+v", model.Channels[0])
	}
}

func TestWarningMessagesProjectsWarningBodies(t *testing.T) {
	t.Parallel()
	got := warningMessages([]pluginmanifest.Warning{{Message: "first"}, {Message: "second"}})
	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("warnings = %+v", got)
	}
}

func TestPublicationDoctorIssueErrWrapsExitCodeOne(t *testing.T) {
	t.Parallel()

	err := publicationDoctorIssueErr(false)
	if err == nil {
		t.Fatal("expected error")
	}
	if code := exitx.Code(err); code != 1 {
		t.Fatalf("exit code = %d", code)
	}
}

func TestPublicationDoctorIssueErrSkipsReadyReports(t *testing.T) {
	t.Parallel()

	if err := publicationDoctorIssueErr(true); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestPublicationDoctorRequestedTargetTrimsWhitespace(t *testing.T) {
	t.Parallel()

	if got := publicationDoctorRequestedTarget(" gemini "); got != "gemini" {
		t.Fatalf("requested target = %q", got)
	}
}

func TestNewPublicationDoctorJSONReportCopiesSlices(t *testing.T) {
	t.Parallel()

	issues := []publicationIssue{{Code: "demo"}}
	nextSteps := []string{"step"}
	missing := []string{"gemini"}
	report := newPublicationDoctorJSONReport("gemini", []string{"warn"}, publicationmodel.Model{}, publicationDiagnosis{
		Ready:                 true,
		Status:                "ready",
		Issues:                issues,
		NextSteps:             nextSteps,
		MissingPackageTargets: missing,
	}, &app.PluginPublicationVerifyRootResult{Ready: true})
	issues[0].Code = "changed"
	nextSteps[0] = "changed"
	missing[0] = "changed"
	if report.Issues[0].Code != "demo" || report.NextSteps[0] != "step" || report.MissingPackageTargets[0] != "gemini" {
		t.Fatalf("report = %+v", report)
	}
}

func TestPublicationDoctorJSONIssueErrUsesDiagnosisReadyFlag(t *testing.T) {
	t.Parallel()

	if err := publicationDoctorJSONIssueErr(publicationDiagnosis{Ready: true}); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	err := publicationDoctorJSONIssueErr(publicationDiagnosis{Ready: false})
	if code := exitx.Code(err); code != 1 {
		t.Fatalf("exit code = %d", code)
	}
}

func TestWritePublicationDoctorJSONReportEmitsJSONEnvelope(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	err := writePublicationDoctorJSONReport(cmd, pluginmanifest.Inspection{}, nil, "gemini", publicationDiagnosis{Ready: true, Status: "ready"}, &app.PluginPublicationVerifyRootResult{Ready: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got == "" || got[0] != '{' {
		t.Fatalf("output = %q", got)
	}
}

func TestMarshalPublicationDoctorJSONProducesIndentedEnvelope(t *testing.T) {
	t.Parallel()

	body, err := marshalPublicationDoctorJSON(publicationDoctorJSONReport{
		Format:        "plugin-kit-ai/publication-doctor-report",
		SchemaVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 || body[0] != '{' {
		t.Fatalf("body = %q", body)
	}
}

func TestWritePublicationDoctorJSONBodyAddsTrailingNewline(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := writePublicationDoctorJSONBody(cmd, []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "{\"ok\":true}\n" {
		t.Fatalf("output = %q", got)
	}
}
