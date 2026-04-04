package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/777genius/plugin-kit-ai/cli/internal/exitx"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
	"github.com/spf13/cobra"
)

var publicationCmd = newPublicationCmd(pluginService)

type publicationMaterializeRunner interface {
	PublicationMaterialize(app.PluginPublicationMaterializeOptions) (app.PluginPublicationMaterializeResult, error)
	PublicationRemove(app.PluginPublicationRemoveOptions) (app.PluginPublicationRemoveResult, error)
	PublicationVerifyRoot(app.PluginPublicationVerifyRootOptions) (app.PluginPublicationVerifyRootResult, error)
}

func newPublicationCmd(runner inspectRunner) *cobra.Command {
	var target string
	var format string
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
				Target: target,
			})
			if err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(format)) {
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
				out, err := json.MarshalIndent(buildPublicationJSONReport(report, warnings, target), "", "  ")
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			default:
				return fmt.Errorf("unsupported format %q (use text or json)", format)
			}
		},
	}
	cmd.Flags().StringVar(&target, "target", "all", `publication target ("all", "claude", "codex-package", or "gemini")`)
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	cmd.AddCommand(newPublicationDoctorCmd(runner))
	if materializer, ok := any(runner).(publicationMaterializeRunner); ok {
		cmd.AddCommand(newPublicationMaterializeCmd(materializer))
		cmd.AddCommand(newPublicationRemoveCmd(materializer))
	}
	return cmd
}

func newPublicationMaterializeCmd(runner publicationMaterializeRunner) *cobra.Command {
	var target string
	var dest string
	var packageRoot string
	cmd := &cobra.Command{
		Use:   "materialize [path]",
		Short: "Materialize a safe local marketplace root for Codex or Claude",
		Long: `Create or update a local marketplace root for a single publication-capable package target.

This workflow is intentionally limited to documented local/catalog flows:
- Codex marketplace roots with .agents/plugins/marketplace.json
- Claude marketplace roots with .claude-plugin/marketplace.json

It copies the materialized package bundle under a managed package root, then merges or creates the marketplace catalog artifact.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			result, err := runner.PublicationMaterialize(app.PluginPublicationMaterializeOptions{
				Root:        root,
				Target:      target,
				Dest:        dest,
				PackageRoot: packageRoot,
			})
			if err != nil {
				return err
			}
			for _, line := range result.Lines {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "", `materialization target ("claude" or "codex-package")`)
	cmd.Flags().StringVar(&dest, "dest", "", "destination marketplace root directory")
	cmd.Flags().StringVar(&packageRoot, "package-root", "", "relative package root inside the destination marketplace root (default: plugins/<name>)")
	_ = cmd.MarkFlagRequired("target")
	_ = cmd.MarkFlagRequired("dest")
	return cmd
}

func newPublicationRemoveCmd(runner publicationMaterializeRunner) *cobra.Command {
	var target string
	var dest string
	var packageRoot string
	cmd := &cobra.Command{
		Use:   "remove [path]",
		Short: "Remove a materialized local marketplace package root and catalog entry",
		Long: `Remove a single plugin from a local Codex or Claude marketplace root.

This workflow is intentionally scoped to documented local/catalog flows and is safe to rerun.
It removes the selected package root and prunes the matching plugin entry from the marketplace catalog while preserving the marketplace root itself.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			result, err := runner.PublicationRemove(app.PluginPublicationRemoveOptions{
				Root:        root,
				Target:      target,
				Dest:        dest,
				PackageRoot: packageRoot,
			})
			if err != nil {
				return err
			}
			for _, line := range result.Lines {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "", `removal target ("claude" or "codex-package")`)
	cmd.Flags().StringVar(&dest, "dest", "", "destination marketplace root directory")
	cmd.Flags().StringVar(&packageRoot, "package-root", "", "relative package root inside the destination marketplace root (default: plugins/<name>)")
	_ = cmd.MarkFlagRequired("target")
	_ = cmd.MarkFlagRequired("dest")
	return cmd
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

func diagnosePublication(root, requestedTarget string, report pluginmanifest.Inspection) publicationDiagnosis {
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
	artifactIssues := diagnosePublicationArtifacts(root, requestedTarget, report.Publication)
	if len(missing) == 0 && len(artifactIssues) == 0 {
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
	if len(missing) > 0 {
		lines = append(lines, "Status: needs_channels (one or more publication-capable package targets have no authored publish/... channel)")
		lines = append(lines, "Next:")
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

	next = publicationNextStepsForArtifactIssues(artifactIssues)
	for _, issue := range artifactIssues {
		lines = append(lines, fmt.Sprintf("Issue[%s]: %s", issue.Code, issue.Message))
	}
	lines = append(lines, "Status: needs_render (authored publication inputs exist, but generated publication artifacts are missing)")
	lines = append(lines, "Next:")
	for _, step := range next {
		lines = append(lines, "  "+step)
	}
	return publicationDiagnosis{
		Ready:     false,
		Status:    "needs_render",
		Lines:     lines,
		NextSteps: next,
		Issues:    artifactIssues,
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

func publicationNextStepsForArtifactIssues(issues []publicationIssue) []string {
	if len(issues) == 0 {
		return []string{}
	}
	return []string{
		"run plugin-kit-ai render . to regenerate package and publication artifacts",
		"run plugin-kit-ai validate . --strict to confirm generated publication outputs are in sync",
	}
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

func diagnosePublicationArtifacts(root, requestedTarget string, model publicationmodel.Model) []publicationIssue {
	var issues []publicationIssue
	for _, pkg := range model.Packages {
		if path := expectedPackageArtifactPath(pkg.Target); path != "" && !fileExists(filepath.Join(root, path)) {
			issues = append(issues, publicationIssue{
				Code:    "missing_package_artifact",
				Target:  pkg.Target,
				Path:    path,
				Message: fmt.Sprintf("target %s is missing generated package artifact %s", pkg.Target, path),
			})
		}
	}
	for _, channel := range model.Channels {
		if path := expectedChannelArtifactPath(channel.Family); path != "" && !fileExists(filepath.Join(root, path)) {
			issues = append(issues, publicationIssue{
				Code:          "missing_channel_artifact",
				ChannelFamily: channel.Family,
				Path:          path,
				Message:       fmt.Sprintf("channel %s is missing generated publication artifact %s", channel.Family, path),
			})
		}
	}
	if fileExists(filepath.Join(root, pluginmanifest.FileName)) {
		rendered, err := pluginmanifest.Render(root, normalizePublicationRequestedTarget(requestedTarget))
		if err != nil {
			issues = append(issues, publicationIssue{
				Code:    "render_probe_failed",
				Path:    pluginmanifest.FileName,
				Message: fmt.Sprintf("publication doctor could not probe generated publication artifacts: %v", err),
			})
		} else {
			expectedBodies := make(map[string][]byte, len(rendered.Artifacts))
			for _, artifact := range rendered.Artifacts {
				expectedBodies[artifact.RelPath] = artifact.Content
			}
			for _, pkg := range model.Packages {
				if path := expectedPackageArtifactPath(pkg.Target); path != "" {
					if issue, ok := diagnosePublicationArtifactDrift(root, path, expectedBodies[path], "drifted_package_artifact"); ok {
						issue.Target = pkg.Target
						issues = append(issues, issue)
					}
				}
			}
			for _, channel := range model.Channels {
				if path := expectedChannelArtifactPath(channel.Family); path != "" {
					if issue, ok := diagnosePublicationArtifactDrift(root, path, expectedBodies[path], "drifted_channel_artifact"); ok {
						issue.ChannelFamily = channel.Family
						issues = append(issues, issue)
					}
				}
			}
			for _, path := range rendered.StalePaths {
				if isPublicationRelevantPath(path) {
					issues = append(issues, publicationIssue{
						Code:    "stale_generated_artifact",
						Path:    path,
						Message: fmt.Sprintf("generated publication artifact %s is stale and should be removed by render", path),
					})
				}
			}
		}
	}
	slices.SortFunc(issues, func(a, b publicationIssue) int {
		if cmp := strings.Compare(a.Code, b.Code); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.Target, b.Target); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.ChannelFamily, b.ChannelFamily); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Path, b.Path)
	})
	return issues
}

func normalizePublicationRequestedTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return "all"
	}
	return target
}

func expectedPackageArtifactPath(target string) string {
	switch target {
	case "codex-package":
		return filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json"))
	case "claude":
		return filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json"))
	case "gemini":
		return "gemini-extension.json"
	default:
		return ""
	}
}

func expectedChannelArtifactPath(family string) string {
	switch family {
	case "codex-marketplace":
		return filepath.ToSlash(filepath.Join(".agents", "plugins", "marketplace.json"))
	case "claude-marketplace":
		return filepath.ToSlash(filepath.Join(".claude-plugin", "marketplace.json"))
	default:
		return ""
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func diagnosePublicationArtifactDrift(root, path string, expected []byte, code string) (publicationIssue, bool) {
	if len(expected) == 0 || !fileExists(filepath.Join(root, path)) {
		return publicationIssue{}, false
	}
	current, err := os.ReadFile(filepath.Join(root, path))
	if err != nil || bytes.Equal(current, expected) {
		return publicationIssue{}, false
	}
	return publicationIssue{
		Code:    code,
		Path:    path,
		Message: fmt.Sprintf("generated publication artifact %s is out of sync with current authored inputs", path),
	}, true
}

func isPublicationRelevantPath(path string) bool {
	switch filepath.ToSlash(filepath.Clean(path)) {
	case filepath.ToSlash(filepath.Join(".agents", "plugins", "marketplace.json")),
		filepath.ToSlash(filepath.Join(".claude-plugin", "marketplace.json")),
		filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
		filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
		"gemini-extension.json":
		return true
	default:
		return false
	}
}

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

func appendUniqueStrings(base []string, extra ...string) []string {
	seen := make(map[string]struct{}, len(base))
	out := append([]string(nil), base...)
	for _, item := range out {
		seen[item] = struct{}{}
	}
	for _, item := range extra {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
