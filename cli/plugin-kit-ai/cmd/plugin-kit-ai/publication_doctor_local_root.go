package main

import "github.com/777genius/plugin-kit-ai/cli/internal/app"

func maybeVerifyPublicationLocalRoot(runner inspectRunner, root, requestedTarget, dest, packageRoot, diagnosisStatus string) (*app.PluginPublicationVerifyRootResult, error) {
	if !shouldVerifyPublicationLocalRoot(dest, diagnosisStatus) {
		return nil, nil
	}
	result, err := verifyPublicationLocalRootWithRunner(runner, publicationLocalRootOptions(root, requestedTarget, dest, packageRoot))
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func mergePublicationDiagnosisWithLocalRoot(diagnosis publicationDiagnosis, requestedTarget string, localRoot *app.PluginPublicationVerifyRootResult) publicationDiagnosis {
	if localRoot == nil {
		return diagnosis
	}
	diagnosis = mergePublicationDiagnosisLocalRootStatus(diagnosis, localRoot)
	diagnosis.Issues = append(diagnosis.Issues, localRootPublicationIssues(requestedTarget, localRoot)...)
	diagnosis.NextSteps = mergePublicationDiagnosisLocalRootNextSteps(diagnosis.NextSteps, localRoot)
	return diagnosis
}

func normalizePublicationLocalRoot(localRoot *app.PluginPublicationVerifyRootResult) *app.PluginPublicationVerifyRootResult {
	return normalizedPublicationLocalRoot(localRoot)
}
