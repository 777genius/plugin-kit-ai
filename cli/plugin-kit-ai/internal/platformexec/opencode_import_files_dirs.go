package platformexec

import (
	"os"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func importDirectoryArtifacts(source opencodeImportSource, dstRoot string, keep func(string) bool) ([]pluginmodel.Artifact, error) {
	artifacts, _, err := importDirectoryArtifactsWithWarnings([]opencodeImportSource{source}, dstRoot, keep)
	return artifacts, err
}

func importDirectoryArtifactsWithWarnings(sources []opencodeImportSource, dstRoot string, keep func(string) bool) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	artifacts := map[string]pluginmodel.Artifact{}
	var warnings []pluginmodel.Warning
	for _, source := range sources {
		used, err := appendImportedDirectoryArtifacts(artifacts, source, dstRoot, keep)
		if err != nil {
			return nil, nil, err
		}
		if source.warnOnUse && used {
			warnings = append(warnings, pluginmodel.Warning{
				Kind:    pluginmodel.WarningFidelity,
				Path:    source.warnPath,
				Message: source.warnMsg,
			})
		}
	}
	return sortedImportedArtifacts(artifacts), warnings, nil
}

func appendImportedDirectoryArtifacts(artifacts map[string]pluginmodel.Artifact, source opencodeImportSource, dstRoot string, keep func(string) bool) (bool, error) {
	full := source.dir
	if _, err := os.Stat(full); err != nil {
		return false, nil
	}
	var used bool
	err := filepath.WalkDir(full, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(full, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if keep != nil && !keep(rel) {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		dst := filepath.ToSlash(filepath.Join(dstRoot, rel))
		artifacts[dst] = pluginmodel.Artifact{RelPath: dst, Content: body}
		used = true
		return nil
	})
	return used, err
}

func sortedImportedArtifacts(artifacts map[string]pluginmodel.Artifact) []pluginmodel.Artifact {
	out := make([]pluginmodel.Artifact, 0, len(artifacts))
	for _, rel := range sortedArtifactKeys(artifacts) {
		out = append(out, artifacts[rel])
	}
	return out
}
