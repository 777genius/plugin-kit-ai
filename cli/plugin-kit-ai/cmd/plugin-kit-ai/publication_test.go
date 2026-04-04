package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/exitx"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
)

func TestPublicationTextShowsPackagesAndChannels(t *testing.T) {
	t.Parallel()
	cmd := newPublicationCmd(fakeInspectRunner{
		report: pluginmanifest.Inspection{
			Publication: publicationmodel.Model{
				Core: publicationmodel.Core{
					APIVersion:  "v1",
					Name:        "demo",
					Version:     "0.1.0",
					Description: "demo plugin",
				},
				Packages: []publicationmodel.Package{
					{
						Target:          "codex-package",
						PackageFamily:   "codex-plugin",
						ChannelFamilies: []string{"codex-marketplace"},
						AuthoredInputs:  []string{"plugin.yaml", "publish/codex/marketplace.yaml"},
						ManagedArtifacts: []string{
							".codex-plugin/plugin.json",
							".agents/plugins/marketplace.json",
						},
					},
				},
				Channels: []publicationmodel.Channel{
					{
						Family:         "codex-marketplace",
						Path:           "publish/codex/marketplace.yaml",
						PackageTargets: []string{"codex-package"},
						Details: map[string]string{
							"marketplace_name":      "local-repo",
							"source_root":           "./",
							"category":              "Productivity",
							"installation_policy":   "AVAILABLE",
							"authentication_policy": "ON_INSTALL",
						},
					},
				},
			},
		},
	})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--format", "text", "."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{
		"publication demo 0.1.0 api_version=v1",
		"packages: 1 channels: 1",
		"package[codex-package]: family=codex-plugin channels=codex-marketplace inputs=2 managed=2",
		"channel[codex-marketplace]: path=publish/codex/marketplace.yaml targets=codex-package",
		"details=authentication_policy=ON_INSTALL,category=Productivity,installation_policy=AVAILABLE,marketplace_name=local-repo,source_root=./",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("publication output missing %q:\n%s", want, output)
		}
	}
}

func TestPublicationJSONEmitsPublicationModelOnly(t *testing.T) {
	t.Parallel()
	cmd := newPublicationCmd(fakeInspectRunner{
		report: pluginmanifest.Inspection{
			Publication: publicationmodel.Model{
				Core: publicationmodel.Core{
					APIVersion:  "v1",
					Name:        "gemini-publish",
					Version:     "0.1.0",
					Description: "gemini publish",
				},
				Packages: []publicationmodel.Package{
					{
						Target:          "gemini",
						PackageFamily:   "gemini-extension",
						ChannelFamilies: []string{"gemini-gallery"},
					},
				},
				Channels: []publicationmodel.Channel{
					{
						Family:         "gemini-gallery",
						Path:           "publish/gemini/gallery.yaml",
						PackageTargets: []string{"gemini"},
						Details: map[string]string{
							"distribution":          "github_release",
							"repository_visibility": "public",
							"github_topic":          "gemini-cli-extension",
							"manifest_root":         "release_archive_root",
						},
					},
				},
			},
		},
		warnings: []pluginmanifest.Warning{
			{Message: "publish/gemini/gallery.yaml is discoverable"},
		},
	})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--format", "json", "."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json parse: %v\n%s", err, buf.Bytes())
	}
	if payload["format"] != "plugin-kit-ai/publication-report" {
		t.Fatalf("format = %+v", payload["format"])
	}
	if payload["schema_version"] != float64(1) {
		t.Fatalf("schema_version = %+v", payload["schema_version"])
	}
	if payload["warning_count"] != float64(1) {
		t.Fatalf("warning_count = %+v", payload["warning_count"])
	}
	warnings, ok := payload["warnings"].([]any)
	if !ok || len(warnings) != 1 || warnings[0] != "publish/gemini/gallery.yaml is discoverable" {
		t.Fatalf("warnings = %+v", payload["warnings"])
	}
	publication, ok := payload["publication"].(map[string]any)
	if !ok || publication["core"] == nil || publication["packages"] == nil || publication["channels"] == nil {
		t.Fatalf("publication payload = %+v", payload)
	}
	if _, found := payload["core"]; found {
		t.Fatalf("publication json should use envelope shape, not raw model: %+v", payload)
	}
}

func TestPublicationHelpMentionsSupportedTargets(t *testing.T) {
	t.Parallel()
	cmd := newPublicationCmd(fakeInspectRunner{})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{
		`publication target ("all", "claude", "codex-package", or "gemini")`,
		`output format: text or json`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
}

func TestPublicationDoctorReturnsExitCodeOneWhenChannelsAreMissing(t *testing.T) {
	t.Parallel()
	cmd := newPublicationDoctorCmd(fakeInspectRunner{
		report: pluginmanifest.Inspection{
			Publication: publicationmodel.Model{
				Core: publicationmodel.Core{
					APIVersion: "v1",
					Name:       "demo",
					Version:    "0.1.0",
				},
				Packages: []publicationmodel.Package{
					{
						Target:          "gemini",
						PackageFamily:   "gemini-extension",
						ChannelFamilies: []string{"gemini-gallery"},
						ManagedArtifacts: []string{
							"gemini-extension.json",
						},
					},
				},
			},
		},
	})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if code := exitx.Code(err); code != 1 {
		t.Fatalf("exit code = %d", code)
	}
	output := buf.String()
	for _, want := range []string{
		"Issue[missing_channel]: target gemini requires authored gemini-gallery at publish/gemini/gallery.yaml",
		"Status: needs_channels",
		"Next:",
		"add publish/gemini/gallery.yaml",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("publication doctor output missing %q:\n%s", want, output)
		}
	}
}

func TestPublicationDoctorReportsReadyWhenChannelsExist(t *testing.T) {
	t.Parallel()
	cmd := newPublicationDoctorCmd(fakeInspectRunner{
		report: pluginmanifest.Inspection{
			Publication: publicationmodel.Model{
				Core: publicationmodel.Core{
					APIVersion: "v1",
					Name:       "demo",
					Version:    "0.1.0",
				},
				Packages: []publicationmodel.Package{
					{
						Target:           "codex-package",
						PackageFamily:    "codex-plugin",
						ChannelFamilies:  []string{"codex-marketplace"},
						ManagedArtifacts: []string{".codex-plugin/plugin.json", ".agents/plugins/marketplace.json"},
					},
				},
				Channels: []publicationmodel.Channel{
					{
						Family:         "codex-marketplace",
						Path:           "publish/codex/marketplace.yaml",
						PackageTargets: []string{"codex-package"},
						Details: map[string]string{
							"marketplace_name": "local-repo",
						},
					},
				},
			},
		},
	})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{
		"Status: ready",
		"Channel[codex-marketplace]: path=publish/codex/marketplace.yaml targets=codex-package",
		"run plugin-kit-ai validate . --strict",
		"run plugin-kit-ai publication . --format json",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("publication doctor output missing %q:\n%s", want, output)
		}
	}
}

func TestPublicationDoctorHelpIncludesReadOnlyReadinessCheck(t *testing.T) {
	t.Parallel()
	cmd := newPublicationDoctorCmd(fakeInspectRunner{})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{
		"Read-only publication readiness check",
		`publication target ("all", "claude", "codex-package", or "gemini")`,
		"output format: text or json",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
}

func TestPublicationDoctorJSONEmitsStableReportForMissingChannels(t *testing.T) {
	t.Parallel()
	cmd := newPublicationDoctorCmd(fakeInspectRunner{
		report: pluginmanifest.Inspection{
			Publication: publicationmodel.Model{
				Core: publicationmodel.Core{
					APIVersion:  "v1",
					Name:        "demo",
					Version:     "0.1.0",
					Description: "demo plugin",
				},
				Packages: []publicationmodel.Package{
					{
						Target:           "gemini",
						PackageFamily:    "gemini-extension",
						ChannelFamilies:  []string{"gemini-gallery"},
						ManagedArtifacts: []string{"gemini-extension.json"},
					},
				},
			},
		},
		warnings: []pluginmanifest.Warning{
			{Message: "publish/gemini/gallery.yaml is not authored yet"},
		},
	})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--format", "json", "--target", "gemini", "."})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if code := exitx.Code(err); code != 1 {
		t.Fatalf("exit code = %d", code)
	}
	var payload map[string]any
	if parseErr := json.Unmarshal(buf.Bytes(), &payload); parseErr != nil {
		t.Fatalf("json parse: %v\n%s", parseErr, buf.Bytes())
	}
	if payload["format"] != "plugin-kit-ai/publication-doctor-report" {
		t.Fatalf("format = %+v", payload["format"])
	}
	if payload["schema_version"] != float64(1) {
		t.Fatalf("schema_version = %+v", payload["schema_version"])
	}
	if payload["requested_target"] != "gemini" {
		t.Fatalf("requested_target = %+v", payload["requested_target"])
	}
	if payload["ready"] != false {
		t.Fatalf("ready = %+v", payload["ready"])
	}
	if payload["status"] != "needs_channels" {
		t.Fatalf("status = %+v", payload["status"])
	}
	if payload["warning_count"] != float64(1) {
		t.Fatalf("warning_count = %+v", payload["warning_count"])
	}
	if payload["issue_count"] != float64(1) {
		t.Fatalf("issue_count = %+v", payload["issue_count"])
	}
	warnings, ok := payload["warnings"].([]any)
	if !ok || len(warnings) != 1 || warnings[0] != "publish/gemini/gallery.yaml is not authored yet" {
		t.Fatalf("warnings = %+v", payload["warnings"])
	}
	issues, ok := payload["issues"].([]any)
	if !ok || len(issues) != 1 {
		t.Fatalf("issues = %+v", payload["issues"])
	}
	issue, ok := issues[0].(map[string]any)
	if !ok {
		t.Fatalf("issue = %+v", issues[0])
	}
	if issue["code"] != "missing_channel" || issue["target"] != "gemini" || issue["channel_family"] != "gemini-gallery" || issue["path"] != "publish/gemini/gallery.yaml" {
		t.Fatalf("issue = %+v", issue)
	}
	nextSteps, ok := payload["next_steps"].([]any)
	if !ok || len(nextSteps) == 0 {
		t.Fatalf("next_steps = %+v", payload["next_steps"])
	}
	missingTargets, ok := payload["missing_package_targets"].([]any)
	if !ok || len(missingTargets) != 1 || missingTargets[0] != "gemini" {
		t.Fatalf("missing_package_targets = %+v", payload["missing_package_targets"])
	}
	publication, ok := payload["publication"].(map[string]any)
	if !ok || publication["core"] == nil || publication["packages"] == nil || publication["channels"] == nil {
		t.Fatalf("publication = %+v", payload["publication"])
	}
}

func TestPublicationDoctorJSONReportsReadyState(t *testing.T) {
	t.Parallel()
	cmd := newPublicationDoctorCmd(fakeInspectRunner{
		report: pluginmanifest.Inspection{
			Publication: publicationmodel.Model{
				Core: publicationmodel.Core{
					APIVersion:  "v1",
					Name:        "demo",
					Version:     "0.1.0",
					Description: "demo plugin",
				},
				Packages: []publicationmodel.Package{
					{
						Target:           "codex-package",
						PackageFamily:    "codex-plugin",
						ChannelFamilies:  []string{"codex-marketplace"},
						ManagedArtifacts: []string{".codex-plugin/plugin.json", ".agents/plugins/marketplace.json"},
					},
				},
				Channels: []publicationmodel.Channel{
					{
						Family:         "codex-marketplace",
						Path:           "publish/codex/marketplace.yaml",
						PackageTargets: []string{"codex-package"},
						Details: map[string]string{
							"marketplace_name": "local-repo",
						},
					},
				},
			},
		},
	})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--format", "json", "."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if parseErr := json.Unmarshal(buf.Bytes(), &payload); parseErr != nil {
		t.Fatalf("json parse: %v\n%s", parseErr, buf.Bytes())
	}
	if payload["status"] != "ready" {
		t.Fatalf("status = %+v", payload["status"])
	}
	if payload["ready"] != true {
		t.Fatalf("ready = %+v", payload["ready"])
	}
	if payload["issue_count"] != float64(0) {
		t.Fatalf("issue_count = %+v", payload["issue_count"])
	}
	issues, ok := payload["issues"].([]any)
	if !ok || len(issues) != 0 {
		t.Fatalf("issues = %+v", payload["issues"])
	}
	if _, found := payload["missing_package_targets"]; found {
		t.Fatalf("missing_package_targets should be omitted for ready payload: %+v", payload)
	}
}
