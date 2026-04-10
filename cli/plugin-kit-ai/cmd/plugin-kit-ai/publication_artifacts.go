package main

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
)

func diagnosePublicationArtifacts(root, requestedTarget string, model publicationmodel.Model) []publicationIssue {
	var issues []publicationIssue
	issues = append(issues, diagnoseMissingPublicationArtifacts(root, model)...)
	if fileExists(filepath.Join(root, pluginmodel.SourceDirName, pluginmanifest.FileName)) ||
		fileExists(filepath.Join(root, pluginmodel.LegacySourceDirName, pluginmanifest.FileName)) {
		issues = append(issues, diagnoseGeneratedPublicationArtifacts(root, requestedTarget, model)...)
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
