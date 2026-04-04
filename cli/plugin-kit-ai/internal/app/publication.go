package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationexec"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publishschema"
)

type PluginPublicationMaterializeOptions struct {
	Root        string
	Target      string
	Dest        string
	PackageRoot string
}

type PluginPublicationMaterializeResult struct {
	Lines []string
}

type PluginPublicationRemoveOptions struct {
	Root        string
	Target      string
	Dest        string
	PackageRoot string
}

type PluginPublicationRemoveResult struct {
	Lines []string
}

type PluginPublicationVerifyRootOptions struct {
	Root        string
	Target      string
	Dest        string
	PackageRoot string
}

type PluginPublicationRootIssue struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type PluginPublicationVerifyRootResult struct {
	Ready       bool                         `json:"ready"`
	Status      string                       `json:"status"`
	Dest        string                       `json:"dest"`
	PackageRoot string                       `json:"package_root"`
	CatalogPath string                       `json:"catalog_path"`
	IssueCount  int                          `json:"issue_count"`
	Issues      []PluginPublicationRootIssue `json:"issues"`
	NextSteps   []string                     `json:"next_steps"`
	Lines       []string                     `json:"-"`
}

func (PluginService) PublicationMaterialize(opts PluginPublicationMaterializeOptions) (PluginPublicationMaterializeResult, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	target := strings.TrimSpace(opts.Target)
	switch target {
	case "codex-package", "claude":
	default:
		return PluginPublicationMaterializeResult{}, fmt.Errorf("publication materialize supports only %q or %q", "codex-package", "claude")
	}
	dest := strings.TrimSpace(opts.Dest)
	if dest == "" {
		return PluginPublicationMaterializeResult{}, fmt.Errorf("publication materialize requires --dest")
	}

	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	if _, err := graph.Manifest.SelectedTargets(target); err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	publicationState, err := publishschema.Discover(root)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	inspection, _, err := pluginmanifest.Inspect(root, target)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	publication := inspection.Publication
	if _, ok := publicationPackageForTarget(publication, target); !ok {
		return PluginPublicationMaterializeResult{}, fmt.Errorf("target %s is not publication-capable", target)
	}
	channel, ok := publicationChannelForTarget(publication, target)
	if !ok {
		return PluginPublicationMaterializeResult{}, fmt.Errorf("target %s requires authored publication channel metadata under publish/...", target)
	}

	packageRoot, err := normalizePackageRoot(opts.PackageRoot, graph.Manifest.Name)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	rendered, err := pluginmanifest.Render(root, target)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	managedPaths, err := inspectionManagedPathsForTarget(inspection, target)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	managedPaths = append(managedPaths, graph.Portable.Paths("skills")...)
	managedPaths = slices.Compact(sortedSlashPaths(managedPaths))
	packageFiles, err := materializedPackageArtifacts(root, packageRoot, managedPaths, rendered)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	catalogArtifact, err := publicationexec.RenderLocalCatalogArtifact(graph, publicationState, target, "./"+packageRoot)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	mergedCatalog, err := mergeCatalogAtDestination(dest, target, catalogArtifact)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}

	destPackageRoot := filepath.Join(dest, filepath.FromSlash(packageRoot))
	if err := os.RemoveAll(destPackageRoot); err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	if err := pluginmanifest.WriteArtifacts(dest, packageFiles); err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	if err := pluginmanifest.WriteArtifacts(dest, []pluginmanifest.Artifact{{
		RelPath: catalogArtifact.RelPath,
		Content: mergedCatalog,
	}}); err != nil {
		return PluginPublicationMaterializeResult{}, err
	}

	lines := []string{
		fmt.Sprintf("Materialized publication target: %s", target),
		fmt.Sprintf("Marketplace family: %s", channel.Family),
		fmt.Sprintf("Marketplace root: %s", filepath.Clean(dest)),
		fmt.Sprintf("Package root: %s", packageRoot),
		fmt.Sprintf("Wrote package files: %d", len(packageFiles)),
		fmt.Sprintf("Updated catalog artifact: %s", catalogArtifact.RelPath),
	}
	if len(rendered.StalePaths) > 0 {
		lines = append(lines, fmt.Sprintf("Source render drift observed: %d stale managed path(s) were bypassed by materializing fresh generated outputs", len(rendered.StalePaths)))
	}
	lines = append(lines,
		"Next:",
		fmt.Sprintf("  plugin-kit-ai publication doctor %s", root),
		fmt.Sprintf("  inspect %s with the vendor CLI from the marketplace root", channel.Family),
	)
	return PluginPublicationMaterializeResult{Lines: lines}, nil
}

func (PluginService) PublicationRemove(opts PluginPublicationRemoveOptions) (PluginPublicationRemoveResult, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	target := strings.TrimSpace(opts.Target)
	switch target {
	case "codex-package", "claude":
	default:
		return PluginPublicationRemoveResult{}, fmt.Errorf("publication remove supports only %q or %q", "codex-package", "claude")
	}
	dest := strings.TrimSpace(opts.Dest)
	if dest == "" {
		return PluginPublicationRemoveResult{}, fmt.Errorf("publication remove requires --dest")
	}

	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return PluginPublicationRemoveResult{}, err
	}
	if _, err := graph.Manifest.SelectedTargets(target); err != nil {
		return PluginPublicationRemoveResult{}, err
	}
	inspection, _, err := pluginmanifest.Inspect(root, target)
	if err != nil {
		return PluginPublicationRemoveResult{}, err
	}
	publication := inspection.Publication
	if _, ok := publicationPackageForTarget(publication, target); !ok {
		return PluginPublicationRemoveResult{}, fmt.Errorf("target %s is not publication-capable", target)
	}
	channel, ok := publicationChannelForTarget(publication, target)
	if !ok {
		return PluginPublicationRemoveResult{}, fmt.Errorf("target %s requires authored publication channel metadata under publish/...", target)
	}
	packageRoot, err := normalizePackageRoot(opts.PackageRoot, graph.Manifest.Name)
	if err != nil {
		return PluginPublicationRemoveResult{}, err
	}

	removedPackage := false
	destPackageRoot := filepath.Join(dest, filepath.FromSlash(packageRoot))
	if _, err := os.Stat(destPackageRoot); err == nil {
		if err := os.RemoveAll(destPackageRoot); err != nil {
			return PluginPublicationRemoveResult{}, err
		}
		removedPackage = true
	} else if !os.IsNotExist(err) {
		return PluginPublicationRemoveResult{}, err
	}

	catalogRel, err := publicationexec.CatalogArtifactPath(target)
	if err != nil {
		return PluginPublicationRemoveResult{}, err
	}
	removedCatalogEntry := false
	catalogFull := filepath.Join(dest, filepath.FromSlash(catalogRel))
	if existing, err := os.ReadFile(catalogFull); err == nil {
		updated, removed, err := publicationexec.RemoveCatalogArtifact(target, existing, graph.Manifest.Name)
		if err != nil {
			return PluginPublicationRemoveResult{}, err
		}
		if removed {
			if err := pluginmanifest.WriteArtifacts(dest, []pluginmanifest.Artifact{{
				RelPath: catalogRel,
				Content: updated,
			}}); err != nil {
				return PluginPublicationRemoveResult{}, err
			}
			removedCatalogEntry = true
		}
	} else if !os.IsNotExist(err) {
		return PluginPublicationRemoveResult{}, err
	}

	lines := []string{
		fmt.Sprintf("Removed publication target: %s", target),
		fmt.Sprintf("Marketplace family: %s", channel.Family),
		fmt.Sprintf("Marketplace root: %s", filepath.Clean(dest)),
		fmt.Sprintf("Package root: %s", packageRoot),
	}
	if removedPackage {
		lines = append(lines, "Removed package root: yes")
	} else {
		lines = append(lines, "Removed package root: no existing package root")
	}
	if removedCatalogEntry {
		lines = append(lines, fmt.Sprintf("Updated catalog artifact: %s", catalogRel))
	} else {
		lines = append(lines, fmt.Sprintf("Updated catalog artifact: no matching %q entry was present", graph.Manifest.Name))
	}
	lines = append(lines,
		"Next:",
		fmt.Sprintf("  review %s from the marketplace root if you keep additional plugins there", catalogRel),
	)
	return PluginPublicationRemoveResult{Lines: lines}, nil
}

func (PluginService) PublicationVerifyRoot(opts PluginPublicationVerifyRootOptions) (PluginPublicationVerifyRootResult, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	target := strings.TrimSpace(opts.Target)
	switch target {
	case "codex-package", "claude":
	default:
		return PluginPublicationVerifyRootResult{}, fmt.Errorf("publication doctor local-root verification supports only %q or %q", "codex-package", "claude")
	}
	dest := strings.TrimSpace(opts.Dest)
	if dest == "" {
		return PluginPublicationVerifyRootResult{}, fmt.Errorf("publication doctor local-root verification requires --dest")
	}

	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	if _, err := graph.Manifest.SelectedTargets(target); err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	publicationState, err := publishschema.Discover(root)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	inspection, _, err := pluginmanifest.Inspect(root, target)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	packageRoot, err := normalizePackageRoot(opts.PackageRoot, graph.Manifest.Name)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	rendered, err := pluginmanifest.Render(root, target)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	managedPaths, err := inspectionManagedPathsForTarget(inspection, target)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	managedPaths = append(managedPaths, graph.Portable.Paths("skills")...)
	managedPaths = slices.Compact(sortedSlashPaths(managedPaths))
	expectedPackageFiles, err := materializedPackageArtifacts(root, packageRoot, managedPaths, rendered)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	generatedCatalog, err := publicationexec.RenderLocalCatalogArtifact(graph, publicationState, target, "./"+packageRoot)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}

	catalogRel, err := publicationexec.CatalogArtifactPath(target)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	var issues []PluginPublicationRootIssue
	destPackageRoot := filepath.Join(dest, filepath.FromSlash(packageRoot))
	if info, err := os.Stat(destPackageRoot); err != nil || !info.IsDir() {
		issues = append(issues, PluginPublicationRootIssue{
			Code:    "missing_materialized_package_root",
			Path:    packageRoot,
			Message: fmt.Sprintf("materialized package root %s is missing", packageRoot),
		})
	}
	for _, artifact := range expectedPackageFiles {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(artifact.RelPath))); err != nil {
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
	catalogFull := filepath.Join(dest, filepath.FromSlash(catalogRel))
	if existing, err := os.ReadFile(catalogFull); err == nil {
		catalogIssues, err := publicationexec.DiagnoseCatalogArtifact(target, existing, generatedCatalog.Content, graph.Manifest.Name)
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
		fmt.Sprintf("Local marketplace root: %s", filepath.Clean(dest)),
		fmt.Sprintf("Package root: %s", packageRoot),
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
			fmt.Sprintf("run plugin-kit-ai publication materialize %s --target %s --dest %s", root, target, dest),
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
		Dest:        filepath.Clean(dest),
		PackageRoot: packageRoot,
		CatalogPath: catalogRel,
		IssueCount:  len(issues),
		Issues:      issues,
		NextSteps:   nextSteps,
		Lines:       lines,
	}, nil
}

func publicationPackageForTarget(model publicationmodel.Model, target string) (publicationmodel.Package, bool) {
	for _, pkg := range model.Packages {
		if pkg.Target == target {
			return pkg, true
		}
	}
	return publicationmodel.Package{}, false
}

func publicationChannelForTarget(model publicationmodel.Model, target string) (publicationmodel.Channel, bool) {
	for _, channel := range model.Channels {
		if slices.Contains(channel.PackageTargets, target) {
			return channel, true
		}
	}
	return publicationmodel.Channel{}, false
}

func normalizePackageRoot(value, pluginName string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = filepath.ToSlash(filepath.Join("plugins", pluginName))
	}
	value = filepath.ToSlash(filepath.Clean(value))
	if value == "." || value == "" {
		return "", fmt.Errorf("package root must stay below the marketplace root")
	}
	if strings.HasPrefix(value, "/") || value == ".." || strings.HasPrefix(value, "../") || strings.Contains(value, "/../") {
		return "", fmt.Errorf("package root must stay relative to the marketplace root")
	}
	return value, nil
}

func inspectionManagedPathsForTarget(inspection pluginmanifest.Inspection, target string) ([]string, error) {
	for _, item := range inspection.Targets {
		if item.Target == target {
			return append([]string(nil), item.ManagedArtifacts...), nil
		}
	}
	return nil, fmt.Errorf("inspect output does not include target %s", target)
}

func materializedPackageArtifacts(root, packageRoot string, managedPaths []string, rendered pluginmanifest.RenderResult) ([]pluginmanifest.Artifact, error) {
	renderedBodies := make(map[string][]byte, len(rendered.Artifacts))
	for _, artifact := range rendered.Artifacts {
		renderedBodies[filepath.ToSlash(artifact.RelPath)] = artifact.Content
	}
	out := make([]pluginmanifest.Artifact, 0, len(managedPaths))
	for _, rel := range managedPaths {
		var body []byte
		if generated, ok := renderedBodies[rel]; ok {
			body = append([]byte(nil), generated...)
		} else {
			sourceBody, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("read managed package artifact %s: %w", rel, err)
			}
			body = sourceBody
		}
		out = append(out, pluginmanifest.Artifact{
			RelPath: filepath.ToSlash(filepath.Join(packageRoot, rel)),
			Content: body,
		})
	}
	slices.SortFunc(out, func(a, b pluginmanifest.Artifact) int { return strings.Compare(a.RelPath, b.RelPath) })
	return out, nil
}

func mergeCatalogAtDestination(dest, target string, generated pluginmanifest.Artifact) ([]byte, error) {
	full := filepath.Join(dest, filepath.FromSlash(generated.RelPath))
	existing, err := os.ReadFile(full)
	if err == nil {
		return publicationexec.MergeCatalogArtifact(target, existing, generated.Content)
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	return generated.Content, nil
}

func sortedSlashPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" {
			continue
		}
		out = append(out, path)
	}
	slices.Sort(out)
	return out
}
