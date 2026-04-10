package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationexec"
)

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

func materializedPackageArtifacts(root, packageRoot string, managedPaths []string, generated pluginmanifest.RenderResult) ([]pluginmanifest.Artifact, error) {
	renderedBodies := make(map[string][]byte, len(generated.Artifacts))
	for _, artifact := range generated.Artifacts {
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
