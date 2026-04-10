package main

import (
	"fmt"
	"slices"

	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
)

func inactivePublicationDiagnosis(lines []string) publicationDiagnosis {
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

func readyPublicationDiagnosis(lines []string, model publicationmodel.Model) publicationDiagnosis {
	next := publicationReadyNextSteps(model)
	lines = append(lines,
		"Status: ready (every publication-capable package target has an authored publication channel)",
		"Next:",
	)
	for _, step := range next {
		lines = append(lines, "  "+step)
	}
	return publicationDiagnosis{Ready: true, Status: "ready", Lines: lines, NextSteps: next}
}

func missingChannelPublicationDiagnosis(lines []string, missing []publicationmodel.Package) publicationDiagnosis {
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

func repositoryPublicationDiagnosis(lines []string, issues []publicationIssue, next []string) publicationDiagnosis {
	for _, issue := range issues {
		lines = append(lines, fmt.Sprintf("Issue[%s]: %s", issue.Code, issue.Message))
	}
	lines = append(lines, "Status: needs_repository (publication metadata is authored, but repository-rooted Gemini distribution prerequisites are missing)")
	lines = append(lines, "Next:")
	for _, step := range next {
		lines = append(lines, "  "+step)
	}
	return publicationDiagnosis{
		Ready:     false,
		Status:    "needs_repository",
		Lines:     lines,
		NextSteps: next,
		Issues:    issues,
	}
}

func artifactPublicationDiagnosis(lines []string, issues []publicationIssue) publicationDiagnosis {
	next := publicationNextStepsForArtifactIssues(issues)
	for _, issue := range issues {
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
		Issues:    issues,
	}
}
