package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func readImportedOpenCodeConfigFromDir(root string, displayBase string) (importedOpenCodeConfig, string, []pluginmodel.Warning, bool, error) {
	path, warnings, ok, err := resolveOpenCodeConfigPathInDir(root, displayBase)
	if err != nil || !ok {
		return importedOpenCodeConfig{}, "", warnings, ok, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return importedOpenCodeConfig{}, "", warnings, false, err
	}
	data, err := decodeImportedOpenCodeConfig(body)
	if err != nil {
		return importedOpenCodeConfig{}, "", warnings, false, err
	}
	displayPath := filepath.Base(path)
	if strings.TrimSpace(displayBase) != "" {
		displayPath = filepath.ToSlash(filepath.Join(displayBase, filepath.Base(path)))
	}
	return data, displayPath, warnings, true, nil
}

func importDirectoryArtifacts(source opencodeImportSource, dstRoot string, keep func(string) bool) ([]pluginmodel.Artifact, error) {
	artifacts, _, err := importDirectoryArtifactsWithWarnings([]opencodeImportSource{source}, dstRoot, keep)
	return artifacts, err
}

func importDirectoryArtifactsWithWarnings(sources []opencodeImportSource, dstRoot string, keep func(string) bool) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	artifacts := map[string]pluginmodel.Artifact{}
	var warnings []pluginmodel.Warning
	for _, source := range sources {
		full := source.dir
		if _, err := os.Stat(full); err != nil {
			continue
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
			artifacts[filepath.ToSlash(filepath.Join(dstRoot, rel))] = pluginmodel.Artifact{
				RelPath: filepath.ToSlash(filepath.Join(dstRoot, rel)),
				Content: body,
			}
			used = true
			return nil
		})
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
	out := make([]pluginmodel.Artifact, 0, len(artifacts))
	for _, rel := range sortedArtifactKeys(artifacts) {
		out = append(out, artifacts[rel])
	}
	return out, warnings, nil
}

func importOpenCodeToolArtifacts(workspaceRoot, workspaceDisplay string) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	legacyDir := filepath.Join(workspaceRoot, "tool")
	if _, err := os.Stat(legacyDir); err == nil {
		return nil, nil, fmt.Errorf("unsupported OpenCode native path %s: use %s", filepath.ToSlash(filepath.Join(workspaceDisplay, "tool")), filepath.ToSlash(filepath.Join(workspaceDisplay, "tools")))
	} else if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	sources := []opencodeImportSource{{
		dir:     filepath.Join(workspaceRoot, "tools"),
		display: filepath.ToSlash(filepath.Join(workspaceDisplay, "tools")),
	}}
	return importDirectoryArtifactsRejectingSymlinks(sources, filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "tools"), nil)
}

func importDirectoryArtifactsRejectingSymlinks(sources []opencodeImportSource, dstRoot string, keep func(string) bool) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	artifacts := map[string]pluginmodel.Artifact{}
	for _, source := range sources {
		full := source.dir
		if _, err := os.Stat(full); err != nil {
			continue
		}
		err := filepath.WalkDir(full, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("OpenCode native import does not support symlinks under %s", source.display)
			}
			if d.IsDir() {
				return nil
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
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	out := make([]pluginmodel.Artifact, 0, len(artifacts))
	for _, rel := range sortedArtifactKeys(artifacts) {
		out = append(out, artifacts[rel])
	}
	return out, nil, nil
}

func mergeOpenCodeObject(dst, src map[string]any) {
	if len(src) == 0 {
		return
	}
	for key, value := range src {
		existing, hasExisting := dst[key].(map[string]any)
		incoming, incomingIsMap := value.(map[string]any)
		if hasExisting && incomingIsMap {
			mergeOpenCodeObject(existing, incoming)
			dst[key] = existing
			continue
		}
		dst[key] = value
	}
}
