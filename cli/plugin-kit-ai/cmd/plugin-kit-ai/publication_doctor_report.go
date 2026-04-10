package main

import (
	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/spf13/cobra"
)

func renderPublicationDoctorJSON(cmd *cobra.Command, report pluginmanifest.Inspection, warnings []pluginmanifest.Warning, requestedTarget string, diagnosis publicationDiagnosis, localRoot *app.PluginPublicationVerifyRootResult) error {
	if err := writePublicationDoctorJSONReport(cmd, report, warnings, requestedTarget, diagnosis, localRoot); err != nil {
		return err
	}
	return publicationDoctorJSONIssueErr(diagnosis)
}

func writePublicationDoctorJSONReport(cmd *cobra.Command, report pluginmanifest.Inspection, warnings []pluginmanifest.Warning, requestedTarget string, diagnosis publicationDiagnosis, localRoot *app.PluginPublicationVerifyRootResult) error {
	return writePublicationDoctorJSON(cmd, publicationDoctorJSONReportForOutput(report, warnings, requestedTarget, diagnosis, localRoot))
}

func publicationDoctorJSONReportForOutput(report pluginmanifest.Inspection, warnings []pluginmanifest.Warning, requestedTarget string, diagnosis publicationDiagnosis, localRoot *app.PluginPublicationVerifyRootResult) publicationDoctorJSONReport {
	return buildPublicationDoctorJSONReport(report, warnings, requestedTarget, diagnosis, localRoot)
}

func publicationDoctorJSONIssueErr(diagnosis publicationDiagnosis) error {
	return publicationDoctorIssueErr(diagnosis.Ready)
}
