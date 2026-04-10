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

func newPublicationDoctorCmd(runner inspectRunner) *cobra.Command {
	var target string
	var format string
	var dest string
	var packageRoot string
	cmd := &cobra.Command{
		Use:           "doctor [path]",
		Short:         "Inspect publication readiness without mutating files",
		Long:          "Read-only publication readiness check for package-capable targets and authored publish/... channels.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			report, warnings, err := runner.Inspect(app.PluginInspectOptions{
				Root:   root,
				Target: target,
			})
			if err != nil {
				return err
			}
			diagnosis := diagnosePublication(root, target, report)
			localRoot, err := maybeVerifyPublicationLocalRoot(runner, root, target, dest, packageRoot, diagnosis.Status)
			if err != nil {
				return err
			}
			diagnosis = mergePublicationDiagnosisWithLocalRoot(diagnosis, target, localRoot)
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "", "text":
				for _, warning := range warnings {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning.Message)
				}
				for _, line := range diagnosis.Lines {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
				}
				if localRoot != nil {
					for _, line := range localRoot.Lines {
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
					}
				}
				if diagnosis.Ready {
					return nil
				}
				return exitx.Wrap(errors.New("publication doctor found issues"), 1)
			case "json":
				body, marshalErr := json.MarshalIndent(buildPublicationDoctorJSONReport(report, warnings, target, diagnosis, localRoot), "", "  ")
				if marshalErr != nil {
					return marshalErr
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", body)
				if diagnosis.Ready {
					return nil
				}
				return exitx.Wrap(errors.New("publication doctor found issues"), 1)
			default:
				return fmt.Errorf("unsupported format %q (use text or json)", format)
			}
		},
	}
	cmd.Flags().StringVar(&target, "target", "all", `publication target ("all", "claude", "codex-package", or "gemini")`)
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	cmd.Flags().StringVar(&dest, "dest", "", "optional materialized marketplace root to verify for local codex-package or claude publication flows")
	cmd.Flags().StringVar(&packageRoot, "package-root", "", "relative package root inside the destination marketplace root (default: plugins/<name>)")
	return cmd
}

func buildPublicationDoctorJSONReport(report pluginmanifest.Inspection, warnings []pluginmanifest.Warning, requestedTarget string, diagnosis publicationDiagnosis, localRoot *app.PluginPublicationVerifyRootResult) publicationDoctorJSONReport {
	warningMessages := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		warningMessages = append(warningMessages, warning.Message)
	}
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
	warningMessages := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		warningMessages = append(warningMessages, warning.Message)
	}
	return publicationJSONReport{
		Format:          "plugin-kit-ai/publication-report",
		SchemaVersion:   1,
		RequestedTarget: strings.TrimSpace(requestedTarget),
		WarningCount:    len(warningMessages),
		Warnings:        warningMessages,
		Publication:     normalizePublicationModel(report.Publication),
	}
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

func maybeVerifyPublicationLocalRoot(runner inspectRunner, root, requestedTarget, dest, packageRoot, diagnosisStatus string) (*app.PluginPublicationVerifyRootResult, error) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return nil, nil
	}
	if diagnosisStatus == "inactive" || diagnosisStatus == "needs_channels" {
		return nil, nil
	}
	verifier, ok := any(runner).(interface {
		PublicationVerifyRoot(app.PluginPublicationVerifyRootOptions) (app.PluginPublicationVerifyRootResult, error)
	})
	if !ok {
		return nil, fmt.Errorf("publication doctor local-root verification is not available for this runner")
	}
	result, err := verifier.PublicationVerifyRoot(app.PluginPublicationVerifyRootOptions{
		Root:        root,
		Target:      requestedTarget,
		Dest:        dest,
		PackageRoot: packageRoot,
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func mergePublicationDiagnosisWithLocalRoot(diagnosis publicationDiagnosis, requestedTarget string, localRoot *app.PluginPublicationVerifyRootResult) publicationDiagnosis {
	if localRoot == nil {
		return diagnosis
	}
	if diagnosis.Ready {
		diagnosis.Ready = localRoot.Ready
		if !localRoot.Ready {
			diagnosis.Status = localRoot.Status
		}
	}
	for _, issue := range localRoot.Issues {
		diagnosis.Issues = append(diagnosis.Issues, publicationIssue{
			Code:    issue.Code,
			Target:  strings.TrimSpace(requestedTarget),
			Path:    issue.Path,
			Message: issue.Message,
		})
	}
	if !localRoot.Ready {
		diagnosis.NextSteps = appendUniqueStrings(diagnosis.NextSteps, localRoot.NextSteps...)
	}
	return diagnosis
}

func normalizePublicationLocalRoot(localRoot *app.PluginPublicationVerifyRootResult) *app.PluginPublicationVerifyRootResult {
	if localRoot == nil {
		return nil
	}
	clone := *localRoot
	if clone.Issues == nil {
		clone.Issues = []app.PluginPublicationRootIssue{}
	}
	if clone.NextSteps == nil {
		clone.NextSteps = []string{}
	}
	return &clone
}
