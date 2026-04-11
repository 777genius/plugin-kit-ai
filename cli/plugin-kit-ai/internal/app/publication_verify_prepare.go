package app

import "github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"

type publicationVerifyPlan struct {
	catalogRel string
	issues     []PluginPublicationRootIssue
}

func preparePublicationVerifyRoot(ctx publicationContext) (publicationVerifyPlan, error) {
	expectedPackageFiles, _, err := ctx.expectedMaterializedPackageArtifacts()
	if err != nil {
		return publicationVerifyPlan{}, err
	}
	catalogArtifact, err := ctx.renderLocalCatalogArtifact()
	if err != nil {
		return publicationVerifyPlan{}, err
	}
	catalogRel, err := ctx.catalogArtifactPath()
	if err != nil {
		return publicationVerifyPlan{}, err
	}
	issues, err := collectPublicationVerifyIssues(ctx, expectedPackageFiles, catalogRel, catalogArtifact.Content)
	if err != nil {
		return publicationVerifyPlan{}, err
	}
	return publicationVerifyPlan{
		catalogRel: catalogRel,
		issues:     issues,
	}, nil
}

func collectPublicationVerifyIssues(ctx publicationContext, expectedPackageFiles []pluginmanifest.Artifact, catalogRel string, generatedCatalog []byte) ([]PluginPublicationRootIssue, error) {
	issues, err := collectPublicationVerifyPackageIssues(ctx, expectedPackageFiles)
	if err != nil {
		return nil, err
	}
	catalogIssues, err := collectPublicationVerifyCatalogIssues(ctx, catalogRel, generatedCatalog)
	if err != nil {
		return nil, err
	}
	return append(issues, catalogIssues...), nil
}
