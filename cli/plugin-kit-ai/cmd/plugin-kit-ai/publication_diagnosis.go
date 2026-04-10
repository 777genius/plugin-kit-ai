package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/repostate"
)

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
	repositoryIssues, repositoryNext := diagnoseGeminiRepositoryIssues(root, report.Publication)
	if len(missing) == 0 && len(artifactIssues) == 0 && len(repositoryIssues) == 0 {
		next := publicationReadyNextSteps(report.Publication)
		lines = append(lines,
			"Status: ready (every publication-capable package target has an authored publication channel)",
			"Next:",
		)
		for _, step := range next {
			lines = append(lines, "  "+step)
		}
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

	if len(repositoryIssues) > 0 {
		for _, issue := range repositoryIssues {
			lines = append(lines, fmt.Sprintf("Issue[%s]: %s", issue.Code, issue.Message))
		}
		lines = append(lines, "Status: needs_repository (publication metadata is authored, but repository-rooted Gemini distribution prerequisites are missing)")
		lines = append(lines, "Next:")
		for _, step := range repositoryNext {
			lines = append(lines, "  "+step)
		}
		return publicationDiagnosis{
			Ready:     false,
			Status:    "needs_repository",
			Lines:     lines,
			NextSteps: repositoryNext,
			Issues:    repositoryIssues,
		}
	}

	next = publicationNextStepsForArtifactIssues(artifactIssues)
	for _, issue := range artifactIssues {
		lines = append(lines, fmt.Sprintf("Issue[%s]: %s", issue.Code, issue.Message))
	}
	lines = append(lines, "Status: needs_generate (authored publication inputs exist, but generated publication artifacts are missing)")
	lines = append(lines, "Next:")
	for _, step := range next {
		lines = append(lines, "  "+step)
	}
	return publicationDiagnosis{
		Ready:     false,
		Status:    "needs_generate",
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
			step = "add src/publish/codex/marketplace.yaml, then rerun plugin-kit-ai generate . and plugin-kit-ai validate . --strict"
		case "claude":
			step = "add src/publish/claude/marketplace.yaml, then rerun plugin-kit-ai generate . and plugin-kit-ai validate . --strict"
		case "gemini":
			step = "add src/publish/gemini/gallery.yaml, keep gemini-extension.json in the repository or release root, then rerun plugin-kit-ai validate . --strict"
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
		"run plugin-kit-ai generate . to regenerate package and publication artifacts",
		"run plugin-kit-ai validate . --strict to confirm generated publication outputs are in sync",
	}
}

func publicationReadyNextSteps(model publicationmodel.Model) []string {
	steps := []string{
		"run plugin-kit-ai validate . --strict",
		"run plugin-kit-ai publication . --format json for CI or automation handoff",
	}
	for _, channel := range model.Channels {
		switch channel.Family {
		case "codex-marketplace":
			steps = append(steps, "run plugin-kit-ai publication materialize . --target codex-package --dest <marketplace-root> --dry-run to preview the local Codex marketplace root")
		case "claude-marketplace":
			steps = append(steps, "run plugin-kit-ai publication materialize . --target claude --dest <marketplace-root> --dry-run to preview the local Claude marketplace root")
		case "gemini-gallery":
			steps = append(steps, "confirm the GitHub repository stays public and tagged with the gemini-cli-extension topic")
			switch channel.Details["distribution"] {
			case "github_release":
				steps = append(steps, "ensure GitHub release archives keep gemini-extension.json at the archive root")
			default:
				steps = append(steps, "keep gemini-extension.json at the repository root for git-based installs and gallery indexing")
			}
			steps = append(steps, "use gemini extensions link <path> for live Gemini CLI verification before publishing")
		}
	}
	return appendUniqueStrings(nil, steps...)
}

func diagnoseGeminiRepositoryIssues(root string, model publicationmodel.Model) ([]publicationIssue, []string) {
	channel, ok := expectedGeminiPublicationChannel(model)
	if !ok {
		return nil, nil
	}
	state := repostate.Inspect(root)
	var issues []publicationIssue
	var next []string
	if !state.GitAvailable {
		issues = append(issues, publicationIssue{
			Code:          "gemini_git_cli_unavailable",
			Target:        "gemini",
			ChannelFamily: "gemini-gallery",
			Path:          channel.Path,
			Message:       "git is unavailable, so repository-rooted Gemini gallery prerequisites cannot be verified",
		})
		next = append(next, "install git and rerun plugin-kit-ai publication doctor . --target gemini")
		return issues, next
	}
	if !state.InGitRepo {
		issues = append(issues, publicationIssue{
			Code:          "gemini_git_repository_missing",
			Target:        "gemini",
			ChannelFamily: "gemini-gallery",
			Path:          channel.Path,
			Message:       "Gemini gallery publication expects a Git repository, but the current workspace is not inside one",
		})
		next = append(next, "initialize a Git repository for this plugin before publishing to the Gemini gallery")
	}
	if !state.HasOriginRemote {
		issues = append(issues, publicationIssue{
			Code:          "gemini_origin_remote_missing",
			Target:        "gemini",
			ChannelFamily: "gemini-gallery",
			Path:          channel.Path,
			Message:       "Gemini gallery publication expects a GitHub-backed repository or release source, but no origin remote is configured",
		})
		next = append(next, "add a GitHub origin remote for this plugin repository before publishing")
	} else if !state.OriginIsGitHub {
		issues = append(issues, publicationIssue{
			Code:          "gemini_origin_not_github",
			Target:        "gemini",
			ChannelFamily: "gemini-gallery",
			Path:          channel.Path,
			Message:       fmt.Sprintf("Gemini gallery publication expects GitHub distribution metadata, but origin points to %s", state.OriginHost),
		})
		next = append(next, "move the publication remote to a public GitHub repository before publishing to the Gemini gallery")
	}
	if len(issues) == 0 {
		return nil, nil
	}
	next = append(next, "confirm the GitHub repository stays public and tagged with the gemini-cli-extension topic")
	switch channel.Details["distribution"] {
	case "github_release":
		next = append(next, "prepare a public GitHub repository first, then publish release archives that keep gemini-extension.json at the archive root")
	default:
		next = append(next, "keep gemini-extension.json at the repository root once the GitHub repository is ready")
	}
	return issues, appendUniqueStrings(nil, next...)
}

func expectedGeminiPublicationChannel(model publicationmodel.Model) (publicationmodel.Channel, bool) {
	for _, channel := range model.Channels {
		if channel.Family == "gemini-gallery" {
			return channel, true
		}
	}
	return publicationmodel.Channel{}, false
}

func expectedPublicationChannel(target string) (family string, path string) {
	switch target {
	case "codex-package":
		return "codex-marketplace", "src/publish/codex/marketplace.yaml"
	case "claude":
		return "claude-marketplace", "src/publish/claude/marketplace.yaml"
	case "gemini":
		return "gemini-gallery", "src/publish/gemini/gallery.yaml"
	default:
		return "", ""
	}
}
