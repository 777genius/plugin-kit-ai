package main

import (
	"fmt"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
)

func diagnoseMissingPublicationArtifacts(root string, model publicationmodel.Model) []publicationIssue {
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
	return issues
}
