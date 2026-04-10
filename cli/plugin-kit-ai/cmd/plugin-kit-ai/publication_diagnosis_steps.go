package main

import (
	"slices"

	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
)

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
