package main

import (
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
)

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

