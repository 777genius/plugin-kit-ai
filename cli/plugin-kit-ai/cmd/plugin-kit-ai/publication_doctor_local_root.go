package main

import (
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
)

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
