package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/777genius/plugin-kit-ai/cli/internal/exitx"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
	"github.com/spf13/cobra"
)

var publicationTarget string
var publicationFormat string

var publicationCmd = newPublicationCmd(pluginService)

func newPublicationCmd(runner inspectRunner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publication [path]",
		Short: "Show the publication-oriented package and channel view",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			report, warnings, err := runner.Inspect(app.PluginInspectOptions{
				Root:   root,
				Target: publicationTarget,
			})
			if err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(publicationFormat)) {
			case "", "text":
				for _, warning := range warnings {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning.Message)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "publication %s %s api_version=%s\n",
					report.Publication.Core.Name,
					report.Publication.Core.Version,
					report.Publication.Core.APIVersion,
				)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "packages: %d channels: %d\n",
					len(report.Publication.Packages),
					len(report.Publication.Channels),
				)
				for _, pkg := range report.Publication.Packages {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  package[%s]: family=%s channels=%s inputs=%d managed=%d\n",
						pkg.Target,
						pkg.PackageFamily,
						strings.Join(pkg.ChannelFamilies, ","),
						len(pkg.AuthoredInputs),
						len(pkg.ManagedArtifacts),
					)
				}
				for _, channel := range report.Publication.Channels {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  channel[%s]: path=%s targets=%s",
						channel.Family,
						channel.Path,
						strings.Join(channel.PackageTargets, ","),
					)
					if details := inspectChannelDetails(channel.Details); details != "" {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), " details=%s", details)
					}
					_, _ = fmt.Fprintln(cmd.OutOrStdout())
				}
				return nil
			case "json":
				out, err := json.MarshalIndent(buildPublicationJSONReport(report, warnings, publicationTarget), "", "  ")
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			default:
				return fmt.Errorf("unsupported format %q (use text or json)", publicationFormat)
			}
		},
	}
	cmd.Flags().StringVar(&publicationTarget, "target", "all", `publication target ("all", "claude", "codex-package", or "gemini")`)
	cmd.Flags().StringVar(&publicationFormat, "format", "text", "output format: text or json")
	cmd.AddCommand(newPublicationDoctorCmd(runner))
	return cmd
}

func newPublicationDoctorCmd(runner inspectRunner) *cobra.Command {
	var format string
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
				Target: publicationTarget,
			})
			if err != nil {
				return err
			}
			diagnosis := diagnosePublication(report)
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "", "text":
				for _, warning := range warnings {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning.Message)
				}
				for _, line := range diagnosis.Lines {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
				}
				if diagnosis.Ready {
					return nil
				}
				return exitx.Wrap(errors.New("publication doctor found issues"), 1)
			case "json":
				body, marshalErr := json.MarshalIndent(buildPublicationDoctorJSONReport(report, warnings, publicationTarget, diagnosis), "", "  ")
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
	cmd.Flags().StringVar(&publicationTarget, "target", "all", `publication target ("all", "claude", "codex-package", or "gemini")`)
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	return cmd
}

type publicationDiagnosis struct {
	Ready                 bool
	Status                string
	Lines                 []string
	NextSteps             []string
	MissingPackageTargets []string
	Issues                []publicationIssue
}

type publicationIssue struct {
	Code          string `json:"code"`
	Target        string `json:"target,omitempty"`
	ChannelFamily string `json:"channel_family,omitempty"`
	Path          string `json:"path,omitempty"`
	Message       string `json:"message"`
}

func diagnosePublication(report pluginmanifest.Inspection) publicationDiagnosis {
	lines := []string{
		fmt.Sprintf("Publication: %s %s api_version=%s", report.Publication.Core.Name, report.Publication.Core.Version, report.Publication.Core.APIVersion),
		fmt.Sprintf("Packages: %d", len(report.Publication.Packages)),
		fmt.Sprintf("Channels: %d", len(report.Publication.Channels)),
	}
	if len(report.Publication.Packages) == 0 {
		next := []string{
			"enable at least one package-capable target: claude, codex-package, or gemini",
		}
		issues := []publicationIssue{{
			Code:    "no_publication_targets",
			Message: "no publication-capable package targets are enabled for the requested scope",
		}}
		lines = append(lines,
			"Issue[no_publication_targets]: no publication-capable package targets are enabled for the requested scope",
			"Status: inactive (no publication-capable package targets enabled)",
			"Next:",
			"  "+next[0],
		)
		return publicationDiagnosis{Ready: false, Status: "inactive", Lines: lines, NextSteps: next, Issues: issues}
	}

	channelTargets := map[string]struct{}{}
	for _, channel := range report.Publication.Channels {
		for _, target := range channel.PackageTargets {
			channelTargets[target] = struct{}{}
		}
		line := fmt.Sprintf("Channel[%s]: path=%s targets=%s", channel.Family, channel.Path, strings.Join(channel.PackageTargets, ","))
		if details := inspectChannelDetails(channel.Details); details != "" {
			line += " details=" + details
		}
		lines = append(lines, line)
	}

	var missing []publicationmodel.Package
	for _, pkg := range report.Publication.Packages {
		lines = append(lines, fmt.Sprintf("Package[%s]: family=%s channels=%s managed=%d",
			pkg.Target,
			pkg.PackageFamily,
			strings.Join(pkg.ChannelFamilies, ","),
			len(pkg.ManagedArtifacts),
		))
		if _, ok := channelTargets[pkg.Target]; !ok {
			missing = append(missing, pkg)
		}
	}
	if len(missing) == 0 {
		next := []string{
			"run plugin-kit-ai validate . --strict",
			"run plugin-kit-ai publication . --format json for CI or automation handoff",
		}
		lines = append(lines,
			"Status: ready (every publication-capable package target has an authored publication channel)",
			"Next:",
			"  "+next[0],
			"  "+next[1],
		)
		return publicationDiagnosis{Ready: true, Status: "ready", Lines: lines, NextSteps: next}
	}

	next := publicationNextStepsForMissing(missing)
	lines = append(lines, "Status: needs_channels (one or more publication-capable package targets have no authored publish/... channel)")
	lines = append(lines, "Next:")
	missingTargets := make([]string, 0, len(missing))
	issues := make([]publicationIssue, 0, len(missing))
	for _, pkg := range missing {
		missingTargets = append(missingTargets, pkg.Target)
		channelFamily, channelPath := expectedPublicationChannel(pkg.Target)
		message := fmt.Sprintf("target %s requires authored %s at %s", pkg.Target, channelFamily, channelPath)
		issues = append(issues, publicationIssue{
			Code:          "missing_channel",
			Target:        pkg.Target,
			ChannelFamily: channelFamily,
			Path:          channelPath,
			Message:       message,
		})
		lines = append(lines, fmt.Sprintf("Issue[missing_channel]: %s", message))
	}
	slices.Sort(missingTargets)
	for _, step := range next {
		lines = append(lines, "  "+step)
	}
	return publicationDiagnosis{
		Ready:                 false,
		Status:                "needs_channels",
		Lines:                 lines,
		NextSteps:             next,
		MissingPackageTargets: missingTargets,
		Issues:                issues,
	}
}

func publicationNextStepsForMissing(missing []publicationmodel.Package) []string {
	stepSet := map[string]struct{}{}
	var steps []string
	for _, pkg := range missing {
		var step string
		switch pkg.Target {
		case "codex-package":
			step = "add publish/codex/marketplace.yaml, then rerun plugin-kit-ai render . and plugin-kit-ai validate . --strict"
		case "claude":
			step = "add publish/claude/marketplace.yaml, then rerun plugin-kit-ai render . and plugin-kit-ai validate . --strict"
		case "gemini":
			step = "add publish/gemini/gallery.yaml, keep gemini-extension.json in the repository or release root, then rerun plugin-kit-ai validate . --strict"
		default:
			continue
		}
		if _, ok := stepSet[step]; ok {
			continue
		}
		stepSet[step] = struct{}{}
		steps = append(steps, step)
	}
	slices.Sort(steps)
	return steps
}

func expectedPublicationChannel(target string) (family string, path string) {
	switch target {
	case "codex-package":
		return "codex-marketplace", "publish/codex/marketplace.yaml"
	case "claude":
		return "claude-marketplace", "publish/claude/marketplace.yaml"
	case "gemini":
		return "gemini-gallery", "publish/gemini/gallery.yaml"
	default:
		return "", ""
	}
}

type publicationDoctorJSONReport struct {
	Format                string                 `json:"format"`
	SchemaVersion         int                    `json:"schema_version"`
	RequestedTarget       string                 `json:"requested_target,omitempty"`
	Ready                 bool                   `json:"ready"`
	Status                string                 `json:"status"`
	WarningCount          int                    `json:"warning_count"`
	Warnings              []string               `json:"warnings"`
	IssueCount            int                    `json:"issue_count"`
	Issues                []publicationIssue     `json:"issues"`
	NextSteps             []string               `json:"next_steps"`
	MissingPackageTargets []string               `json:"missing_package_targets,omitempty"`
	Publication           publicationmodel.Model `json:"publication"`
}

type publicationJSONReport struct {
	Format          string                 `json:"format"`
	SchemaVersion   int                    `json:"schema_version"`
	RequestedTarget string                 `json:"requested_target,omitempty"`
	WarningCount    int                    `json:"warning_count"`
	Warnings        []string               `json:"warnings"`
	Publication     publicationmodel.Model `json:"publication"`
}

func buildPublicationDoctorJSONReport(report pluginmanifest.Inspection, warnings []pluginmanifest.Warning, requestedTarget string, diagnosis publicationDiagnosis) publicationDoctorJSONReport {
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
