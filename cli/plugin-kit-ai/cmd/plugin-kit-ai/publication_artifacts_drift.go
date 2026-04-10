package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
)

func diagnoseGeneratedPublicationArtifacts(root, requestedTarget string, model publicationmodel.Model) []publicationIssue {
	generated, err := pluginmanifest.Generate(root, normalizePublicationRequestedTarget(requestedTarget))
	if err != nil {
		return []publicationIssue{{
			Code:    "generate_probe_failed",
			Path:    pluginmanifest.FileName,
			Message: fmt.Sprintf("publication doctor could not probe generated publication artifacts: %v", err),
		}}
	}

	expectedBodies := make(map[string][]byte, len(generated.Artifacts))
	for _, artifact := range generated.Artifacts {
		expectedBodies[artifact.RelPath] = artifact.Content
	}

	issues := diagnosePublicationArtifactBodies(root, model, expectedBodies)
	for _, path := range generated.StalePaths {
		if isPublicationRelevantPath(path) {
			issues = append(issues, publicationIssue{
				Code:    "stale_generated_artifact",
				Path:    path,
				Message: fmt.Sprintf("generated publication artifact %s is stale and should be removed by generate", path),
			})
		}
	}
	return issues
}

func diagnosePublicationArtifactBodies(root string, model publicationmodel.Model, expectedBodies map[string][]byte) []publicationIssue {
	var issues []publicationIssue
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
	return issues
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
