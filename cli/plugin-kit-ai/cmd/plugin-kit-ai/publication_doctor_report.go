package main

import (
	"encoding/json"
	"errors"
	"fmt"

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
