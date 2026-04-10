package main

import (
	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/spf13/cobra"
)

func renderPublicationDoctorJSON(cmd *cobra.Command, report pluginmanifest.Inspection, warnings []pluginmanifest.Warning, requestedTarget string, diagnosis publicationDiagnosis, localRoot *app.PluginPublicationVerifyRootResult) error {
	if err := writePublicationDoctorJSON(cmd, buildPublicationDoctorJSONReport(report, warnings, requestedTarget, diagnosis, localRoot)); err != nil {
		return err
	}
	return publicationDoctorIssueErr(diagnosis.Ready)
}
