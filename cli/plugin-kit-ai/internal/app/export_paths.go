package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

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

func exportOutputPath(root, name, platform, runtime, output string) string {
	if strings.TrimSpace(output) != "" {
		return output
	}
	file := fmt.Sprintf("%s_%s_%s_bundle.tar.gz", name, platform, runtime)
	return filepath.Join(root, file)
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
