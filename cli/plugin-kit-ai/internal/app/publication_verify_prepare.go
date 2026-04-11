package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationexec"
)

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

func collectPublicationVerifyPackageIssues(ctx publicationContext, expectedPackageFiles []pluginmanifest.Artifact) ([]PluginPublicationRootIssue, error) {
	var issues []PluginPublicationRootIssue
	if info, err := os.Stat(ctx.destPackageRoot()); err != nil || !info.IsDir() {
		issues = append(issues, PluginPublicationRootIssue{
			Code:    "missing_materialized_package_root",
			Path:    ctx.packageRoot,
			Message: fmt.Sprintf("materialized package root %s is missing", ctx.packageRoot),
		})
	}
	for _, artifact := range expectedPackageFiles {
		if _, err := os.Stat(filepath.Join(ctx.dest, filepath.FromSlash(artifact.RelPath))); err != nil {
			if os.IsNotExist(err) {
				issues = append(issues, PluginPublicationRootIssue{
					Code:    "missing_materialized_package_artifact",
					Path:    artifact.RelPath,
					Message: fmt.Sprintf("materialized package artifact %s is missing", artifact.RelPath),
				})
				continue
			}
			return nil, err
		}
	}
	return issues, nil
}

func collectPublicationVerifyCatalogIssues(ctx publicationContext, catalogRel string, generatedCatalog []byte) ([]PluginPublicationRootIssue, error) {
	catalogFull := filepath.Join(ctx.dest, filepath.FromSlash(catalogRel))
	existing, err := os.ReadFile(catalogFull)
	if err == nil {
		return diagnosePublicationVerifyCatalogIssues(ctx, existing, generatedCatalog)
	}
	if os.IsNotExist(err) {
		return []PluginPublicationRootIssue{{
			Code:    "missing_materialized_catalog_artifact",
			Path:    catalogRel,
			Message: fmt.Sprintf("materialized catalog artifact %s is missing", catalogRel),
		}}, nil
	}
	return nil, err
}

func diagnosePublicationVerifyCatalogIssues(ctx publicationContext, existing, generatedCatalog []byte) ([]PluginPublicationRootIssue, error) {
	catalogIssues, err := publicationexec.DiagnoseCatalogArtifact(ctx.target, existing, generatedCatalog, ctx.graph.Manifest.Name)
	if err != nil {
		return nil, err
	}
	issues := make([]PluginPublicationRootIssue, 0, len(catalogIssues))
	for _, issue := range catalogIssues {
		issues = append(issues, PluginPublicationRootIssue{
			Code:    issue.Code,
			Path:    issue.Path,
			Message: issue.Message,
		})
	}
	return issues, nil
}
