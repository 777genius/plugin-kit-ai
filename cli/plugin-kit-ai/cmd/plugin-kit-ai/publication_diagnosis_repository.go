package main

import (
	"fmt"

	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/repostate"
)

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
