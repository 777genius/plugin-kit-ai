package main

import (
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
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
		return inactivePublicationDiagnosis(lines)
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
	switch {
	case len(missing) == 0 && len(artifactIssues) == 0 && len(repositoryIssues) == 0:
		return readyPublicationDiagnosis(lines, report.Publication)
	case len(missing) > 0:
		return missingChannelPublicationDiagnosis(lines, report.Layout.AuthoredRoot, missing)
	case len(repositoryIssues) > 0:
		return repositoryPublicationDiagnosis(lines, repositoryIssues, repositoryNext)
	default:
		return artifactPublicationDiagnosis(lines, artifactIssues)
	}
}
