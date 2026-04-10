package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
)

func exportFileList(root string, graph pluginmanifest.PackageGraph, project runtimecheck.Project, generated []pluginmanifest.Artifact) ([]string, error) {
	set := map[string]struct{}{}
	for _, rel := range graph.SourceFiles {
		addExportPath(set, rel)
	}
	for _, artifact := range generated {
		addExportPath(set, artifact.RelPath)
	}
	for _, rel := range launcherBundlePaths(root, project.Entrypoint) {
		addExportPath(set, rel)
	}
	switch project.Runtime {
	case "python":
		addDirIfExists(root, set, "src")
		addIfExists(root, set, "requirements.txt")
		addIfExists(root, set, "pyproject.toml")
		addIfExists(root, set, "uv.lock")
		addIfExists(root, set, "poetry.lock")
		addIfExists(root, set, "Pipfile")
		addIfExists(root, set, "Pipfile.lock")
	case "node":
		addDirIfExists(root, set, "src")
		addIfExists(root, set, "package.json")
		addIfExists(root, set, "tsconfig.json")
		addIfExists(root, set, ".yarnrc.yml")
		addIfExists(root, set, "package-lock.json")
		addIfExists(root, set, "npm-shrinkwrap.json")
		addIfExists(root, set, "pnpm-lock.yaml")
		addIfExists(root, set, "yarn.lock")
		addIfExists(root, set, "bun.lock")
		addIfExists(root, set, "bun.lockb")
		if project.Node.IsTypeScript && project.Node.UsesBuiltOutput {
			addDirIfExists(root, set, project.Node.OutputDir)
		} else if strings.TrimSpace(project.Node.RuntimeTarget) != "" {
			addIfExists(root, set, project.Node.RuntimeTarget)
		}
	case "shell":
		addDirIfExists(root, set, "scripts")
	}
	excludes := exportExcludedPaths(root, project)
	out := make([]string, 0, len(set))
	for rel := range set {
		if shouldExcludeExportPath(rel, excludes) {
			continue
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("export refuses symlinked path %s", rel)
		}
		if info.IsDir() {
			continue
		}
		out = append(out, rel)
	}
	slices.Sort(out)
	return out, nil
}

func addIfExists(root string, set map[string]struct{}, rel string) {
	rel = normalizeExportPath(rel)
	if rel == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
		set[rel] = struct{}{}
	}
}

func addDirIfExists(root string, set map[string]struct{}, rel string) {
	rel = normalizeExportPath(rel)
	if rel == "" {
		return
	}
	dir := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return
	}
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		sub, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		set[filepath.ToSlash(sub)] = struct{}{}
		return nil
	})
}

func addExportPath(set map[string]struct{}, rel string) {
	rel = normalizeExportPath(rel)
	if rel == "" {
		return
	}
	set[rel] = struct{}{}
}

func normalizeExportPath(rel string) string {
	rel = strings.TrimPrefix(strings.TrimSpace(rel), "./")
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == "" {
		return ""
	}
	return rel
}

func launcherBundlePaths(root, entrypoint string) []string {
	base := normalizeExportPath(entrypoint)
	if base == "" {
		return nil
	}
	candidates := []string{base}
	if !strings.HasSuffix(base, ".cmd") {
		candidates = append(candidates, base+".cmd")
	}
	out := make([]string, 0, len(candidates))
	for _, rel := range candidates {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			out = append(out, rel)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func exportExcludedPaths(root string, project runtimecheck.Project) []string {
	excludes := []string{".venv", "node_modules", ".pnp.cjs", ".pnp.loader.mjs"}
	if project.Python.ReadySource == runtimecheck.PythonEnvSourceManagerOwned && strings.TrimSpace(project.Python.ProbedEnvPath) != "" {
		if rel, ok := relWithinRoot(root, project.Python.ProbedEnvPath); ok {
			excludes = append(excludes, rel)
		}
	}
	return excludes
}

func shouldExcludeExportPath(rel string, excludes []string) bool {
	for _, exclude := range excludes {
		exclude = normalizeExportPath(exclude)
		if exclude == "" {
			continue
		}
		if rel == exclude || strings.HasPrefix(rel, exclude+"/") {
			return true
		}
	}
	return false
}

func relWithinRoot(root, path string) (string, bool) {
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

func exportOutputPath(root, name, platform, runtime, output string) string {
	if strings.TrimSpace(output) != "" {
		return output
	}
	file := fmt.Sprintf("%s_%s_%s_bundle.tar.gz", name, platform, runtime)
	return filepath.Join(root, file)
}
