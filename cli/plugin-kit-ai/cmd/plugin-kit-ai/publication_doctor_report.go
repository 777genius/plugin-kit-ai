package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/777genius/plugin-kit-ai/cli/internal/exitx"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
	"github.com/spf13/cobra"
)

type publicationDoctorJSONReport struct {
	Format                string                                 `json:"format"`
	SchemaVersion         int                                    `json:"schema_version"`
	RequestedTarget       string                                 `json:"requested_target,omitempty"`
	Ready                 bool                                   `json:"ready"`
	Status                string                                 `json:"status"`
	WarningCount          int                                    `json:"warning_count"`
	Warnings              []string                               `json:"warnings"`
	IssueCount            int                                    `json:"issue_count"`
	Issues                []publicationIssue                     `json:"issues"`
	NextSteps             []string                               `json:"next_steps"`
	MissingPackageTargets []string                               `json:"missing_package_targets,omitempty"`
	LocalRoot             *app.PluginPublicationVerifyRootResult `json:"local_root,omitempty"`
	Publication           publicationmodel.Model                 `json:"publication"`
}

type publicationJSONReport struct {
	Format          string                 `json:"format"`
	SchemaVersion   int                    `json:"schema_version"`
	RequestedTarget string                 `json:"requested_target,omitempty"`
	WarningCount    int                    `json:"warning_count"`
	Warnings        []string               `json:"warnings"`
	Publication     publicationmodel.Model `json:"publication"`
}

func renderPublicationDoctorJSON(cmd *cobra.Command, report pluginmanifest.Inspection, warnings []pluginmanifest.Warning, requestedTarget string, diagnosis publicationDiagnosis, localRoot *app.PluginPublicationVerifyRootResult) error {
	body, err := json.MarshalIndent(buildPublicationDoctorJSONReport(report, warnings, requestedTarget, diagnosis, localRoot), "", "  ")
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", body)
	if diagnosis.Ready {
		return nil
	}
	return exitx.Wrap(errors.New("publication doctor found issues"), 1)
}

func buildPublicationDoctorJSONReport(report pluginmanifest.Inspection, warnings []pluginmanifest.Warning, requestedTarget string, diagnosis publicationDiagnosis, localRoot *app.PluginPublicationVerifyRootResult) publicationDoctorJSONReport {
	warningMessages := warningMessages(warnings)
	publication := normalizePublicationModel(report.Publication)
	return publicationDoctorJSONReport{
		Format:                "plugin-kit-ai/publication-doctor-report",
		SchemaVersion:         1,
		RequestedTarget:       strings.TrimSpace(requestedTarget),
		Ready:                 diagnosis.Ready,
		Status:                diagnosis.Status,
		WarningCount:          len(warningMessages),
		Warnings:              warningMessages,
		IssueCount:            len(diagnosis.Issues),
		Issues:                append([]publicationIssue{}, diagnosis.Issues...),
		NextSteps:             append([]string(nil), diagnosis.NextSteps...),
		MissingPackageTargets: append([]string(nil), diagnosis.MissingPackageTargets...),
		LocalRoot:             normalizePublicationLocalRoot(localRoot),
		Publication:           publication,
	}
}

func buildPublicationJSONReport(report pluginmanifest.Inspection, warnings []pluginmanifest.Warning, requestedTarget string) publicationJSONReport {
	return publicationJSONReport{
		Format:          "plugin-kit-ai/publication-report",
		SchemaVersion:   1,
		RequestedTarget: strings.TrimSpace(requestedTarget),
		WarningCount:    len(warnings),
		Warnings:        warningMessages(warnings),
		Publication:     normalizePublicationModel(report.Publication),
	}
}

func warningMessages(warnings []pluginmanifest.Warning) []string {
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, warning.Message)
	}
	return out
}

func normalizePublicationModel(model publicationmodel.Model) publicationmodel.Model {
	if model.Packages == nil {
		model.Packages = []publicationmodel.Package{}
	}
	if model.Channels == nil {
		model.Channels = []publicationmodel.Channel{}
	}
	for i := range model.Packages {
		if model.Packages[i].ChannelFamilies == nil {
			model.Packages[i].ChannelFamilies = []string{}
		}
		if model.Packages[i].AuthoredInputs == nil {
			model.Packages[i].AuthoredInputs = []string{}
		}
		if model.Packages[i].ManagedArtifacts == nil {
			model.Packages[i].ManagedArtifacts = []string{}
		}
	}
	for i := range model.Channels {
		if model.Channels[i].PackageTargets == nil {
			model.Channels[i].PackageTargets = []string{}
		}
	}
	return model
}
