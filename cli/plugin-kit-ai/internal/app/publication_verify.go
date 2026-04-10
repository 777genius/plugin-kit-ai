package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/cli/internal/publicationexec"
)

func (PluginService) publicationVerifyRoot(opts PluginPublicationVerifyRootOptions) (PluginPublicationVerifyRootResult, error) {
	ctx, err := loadPublicationContextForVerify(opts)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	expectedPackageFiles, _, err := ctx.expectedMaterializedPackageArtifacts()
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	generatedCatalog, err := ctx.renderLocalCatalogArtifact()
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	catalogRel, err := ctx.catalogArtifactPath()
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}

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
			return PluginPublicationVerifyRootResult{}, err
		}
	}
	catalogFull := filepath.Join(ctx.dest, filepath.FromSlash(catalogRel))
	if existing, err := os.ReadFile(catalogFull); err == nil {
		catalogIssues, err := publicationexec.DiagnoseCatalogArtifact(ctx.target, existing, generatedCatalog.Content, ctx.graph.Manifest.Name)
		if err != nil {
			return PluginPublicationVerifyRootResult{}, err
		}
		for _, issue := range catalogIssues {
			issues = append(issues, PluginPublicationRootIssue{
				Code:    issue.Code,
				Path:    issue.Path,
				Message: issue.Message,
			})
		}
	} else if os.IsNotExist(err) {
		issues = append(issues, PluginPublicationRootIssue{
			Code:    "missing_materialized_catalog_artifact",
			Path:    catalogRel,
			Message: fmt.Sprintf("materialized catalog artifact %s is missing", catalogRel),
		})
	} else {
		return PluginPublicationVerifyRootResult{}, err
	}

	lines := []string{
		fmt.Sprintf("Local marketplace root: %s", filepath.Clean(ctx.dest)),
		fmt.Sprintf("Package root: %s", ctx.packageRoot),
		fmt.Sprintf("Catalog artifact: %s", catalogRel),
	}
	nextSteps := []string{}
	status := "ready"
	ready := true
	if len(issues) > 0 {
		status = "needs_sync"
		ready = false
		for _, issue := range issues {
			lines = append(lines, fmt.Sprintf("Issue[%s]: %s", issue.Code, issue.Message))
		}
		nextSteps = []string{
			fmt.Sprintf("run plugin-kit-ai publication materialize %s --target %s --dest %s", ctx.root, ctx.target, ctx.dest),
		}
		lines = append(lines, "Status: needs_sync (materialized marketplace root is missing files or has drift)", "Next:")
		for _, step := range nextSteps {
			lines = append(lines, "  "+step)
		}
	} else {
		lines = append(lines,
			"Status: ready (materialized marketplace root is in sync)",
		)
	}
	return PluginPublicationVerifyRootResult{
		Ready:       ready,
		Status:      status,
		Dest:        filepath.Clean(ctx.dest),
		PackageRoot: ctx.packageRoot,
		CatalogPath: catalogRel,
		IssueCount:  len(issues),
		Issues:      issues,
		NextSteps:   nextSteps,
		Lines:       lines,
	}, nil
}
