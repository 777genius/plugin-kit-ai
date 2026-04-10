package main

import (
	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/spf13/cobra"
)

func renderPublicationDoctorJSON(cmd *cobra.Command, report pluginmanifest.Inspection, warnings []pluginmanifest.Warning, requestedTarget string, diagnosis publicationDiagnosis, localRoot *app.PluginPublicationVerifyRootResult) error {
	return renderPublicationDoctorJSONEnvelope(cmd, publicationDoctorJSONEnvelope(report, warnings, requestedTarget, diagnosis, localRoot), diagnosis)
}

func writePublicationDoctorJSONReport(cmd *cobra.Command, report pluginmanifest.Inspection, warnings []pluginmanifest.Warning, requestedTarget string, diagnosis publicationDiagnosis, localRoot *app.PluginPublicationVerifyRootResult) error {
	return writePublicationDoctorJSONEnvelope(cmd, publicationDoctorJSONEnvelope(report, warnings, requestedTarget, diagnosis, localRoot))
}

func renderPublicationDoctorJSONEnvelope(cmd *cobra.Command, report publicationDoctorJSONReport, diagnosis publicationDiagnosis) error {
	if err := writePublicationDoctorJSONEnvelope(cmd, report); err != nil {
		return err
	}
	return publicationDoctorJSONIssueErr(diagnosis)
}

func writePublicationDoctorJSONEnvelope(cmd *cobra.Command, report publicationDoctorJSONReport) error {
	return writePublicationDoctorJSON(cmd, report)
}

func publicationDoctorJSONEnvelope(report pluginmanifest.Inspection, warnings []pluginmanifest.Warning, requestedTarget string, diagnosis publicationDiagnosis, localRoot *app.PluginPublicationVerifyRootResult) publicationDoctorJSONReport {
	return buildPublicationDoctorJSONReport(report, warnings, requestedTarget, diagnosis, localRoot)
}

func publicationDoctorJSONIssueErr(diagnosis publicationDiagnosis) error {
	return publicationDoctorIssueErr(diagnosis.Ready)
}
