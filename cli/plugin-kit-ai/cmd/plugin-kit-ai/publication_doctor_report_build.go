package main

import (
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
)

func buildPublicationDoctorJSONReport(report pluginmanifest.Inspection, warnings []pluginmanifest.Warning, requestedTarget string, diagnosis publicationDiagnosis, localRoot *app.PluginPublicationVerifyRootResult) publicationDoctorJSONReport {
	return newPublicationDoctorJSONReport(
		publicationDoctorRequestedTarget(requestedTarget),
		publicationDoctorWarnings(warnings),
		normalizePublicationModel(report.Publication),
		diagnosis,
		normalizePublicationLocalRoot(localRoot),
	)
}

func newPublicationDoctorJSONReport(requestedTarget string, warnings []string, publication publicationmodel.Model, diagnosis publicationDiagnosis, localRoot *app.PluginPublicationVerifyRootResult) publicationDoctorJSONReport {
	return publicationDoctorJSONReport{
		Format:                "plugin-kit-ai/publication-doctor-report",
		SchemaVersion:         1,
		RequestedTarget:       requestedTarget,
		Ready:                 diagnosis.Ready,
		Status:                diagnosis.Status,
		WarningCount:          len(warnings),
		Warnings:              warnings,
		IssueCount:            len(diagnosis.Issues),
		Issues:                publicationDoctorIssues(diagnosis.Issues),
		NextSteps:             publicationDoctorNextSteps(diagnosis.NextSteps),
		MissingPackageTargets: publicationDoctorMissingTargets(diagnosis.MissingPackageTargets),
		LocalRoot:             localRoot,
		Publication:           publication,
	}
}

func buildPublicationJSONReport(report pluginmanifest.Inspection, warnings []pluginmanifest.Warning, requestedTarget string) publicationJSONReport {
	return publicationJSONReport{
		Format:          "plugin-kit-ai/publication-report",
		SchemaVersion:   1,
		RequestedTarget: publicationDoctorRequestedTarget(requestedTarget),
		WarningCount:    len(warnings),
		Warnings:        publicationDoctorWarnings(warnings),
		Publication:     normalizePublicationModel(report.Publication),
	}
}

func publicationDoctorRequestedTarget(requestedTarget string) string {
	return strings.TrimSpace(requestedTarget)
}

func publicationDoctorWarnings(warnings []pluginmanifest.Warning) []string {
	return warningMessages(warnings)
}

func publicationDoctorIssues(issues []publicationIssue) []publicationIssue {
	return append([]publicationIssue{}, issues...)
}

func publicationDoctorNextSteps(nextSteps []string) []string {
	return append([]string(nil), nextSteps...)
}

func publicationDoctorMissingTargets(targets []string) []string {
	return append([]string(nil), targets...)
}
