package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/validate"
)

type fakeValidateRunner struct {
	report validate.Report
	err    error
}

func (f fakeValidateRunner) Run(_ string, _ string) (validate.Report, error) {
	return f.report, f.err
}

func TestValidateWritesJSONOutput(t *testing.T) {
	t.Parallel()
	cmd := newValidateCmd(fakeValidateRunner{
		report: validate.Report{
			Platform: "codex-runtime",
			Checks:   []string{"plugin_manifest"},
			Warnings: []validate.Warning{{
				Kind:    validate.WarningManifestUnknownField,
				Path:    "plugin.yaml",
				Message: "unknown plugin.yaml field: extra_field",
			}},
		},
	}.Run)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--platform", "codex-runtime", "--format", "json", "."})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{
		`"format": "plugin-kit-ai/validate-report"`,
		`"schema_version": 1`,
		`"requested_platform": "codex-runtime"`,
		`"outcome": "passed"`,
		`"platform": "codex-runtime"`,
		`"ok": true`,
		`"strict_mode": false`,
		`"strict_failed": false`,
		`"warning_count": 1`,
		`"failure_count": 0`,
		`"checks": [`,
		`"warnings": [`,
		`"failures": []`,
		`"path": "plugin.yaml"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("json output missing %q:\n%s", want, output)
		}
	}
}

func TestValidateJSONPrintsReportOnFailure(t *testing.T) {
	t.Parallel()
	report := validate.Report{
		Checks: []string{},
		Failures: []validate.Failure{{
			Kind:    validate.FailureManifestMissing,
			Path:    "plugin.yaml",
			Message: "required manifest missing: plugin.yaml",
		}},
	}
	cmd := newValidateCmd(fakeValidateRunner{
		err: &validate.ReportError{Report: report},
	}.Run)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--format", "json", "."})
	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(buf.String(), `"kind": "manifest_missing"`) {
		t.Fatalf("json output missing failure payload:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `"failures": [`) {
		t.Fatalf("json output missing failures array:\n%s", buf.String())
	}
	for _, want := range []string{
		`"format": "plugin-kit-ai/validate-report"`,
		`"schema_version": 1`,
		`"outcome": "failed"`,
		`"ok": false`,
		`"strict_mode": false`,
		`"strict_failed": false`,
		`"warning_count": 0`,
		`"failure_count": 1`,
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("json output missing %q:\n%s", want, buf.String())
		}
	}
}

func TestValidateJSONMarksStrictWarningFailure(t *testing.T) {
	t.Parallel()
	cmd := newValidateCmd(fakeValidateRunner{
		report: validate.Report{
			Warnings: []validate.Warning{{
				Kind:    validate.WarningManifestUnknownField,
				Message: "unknown plugin.yaml field: extra_field",
			}},
		},
	}.Run)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--strict", "--format", "json", "."})
	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected strict warning error")
	}
	for _, want := range []string{
		`"format": "plugin-kit-ai/validate-report"`,
		`"schema_version": 1`,
		`"outcome": "failed_strict_warnings"`,
		`"ok": false`,
		`"strict_mode": true`,
		`"strict_failed": true`,
		`"warning_count": 1`,
		`"failure_count": 0`,
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("json output missing %q:\n%s", want, buf.String())
		}
	}
}

func TestValidateTextPrintsFailuresForReportErrors(t *testing.T) {
	t.Parallel()
	report := validate.Report{
		Warnings: []validate.Warning{{
			Kind:    validate.WarningManifestUnknownField,
			Message: "unknown plugin.yaml field: extra_field",
		}},
		Failures: []validate.Failure{{
			Kind:    validate.FailureManifestMissing,
			Path:    "plugin.yaml",
			Message: "required manifest missing: plugin.yaml",
		}},
	}
	cmd := newValidateCmd(fakeValidateRunner{
		err: &validate.ReportError{Report: report},
	}.Run)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"."})
	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	output := buf.String()
	for _, want := range []string{
		"Warning: unknown plugin.yaml field: extra_field",
		"Failure: required manifest missing: plugin.yaml",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("text output missing %q:\n%s", want, output)
		}
	}
}

func TestValidateHelpMentionsJSONContract(t *testing.T) {
	t.Parallel()
	cmd := newValidateCmd(fakeValidateRunner{}.Run)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{
		`--format json`,
		`plugin-kit-ai/validate-report`,
		`schema_version=1`,
		`failed_strict_warnings`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
}
