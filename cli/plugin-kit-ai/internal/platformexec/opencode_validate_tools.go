package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateOpenCodeToolFiles(root string, rels []string, packageDoc map[string]any) []Diagnostic {
	if len(rels) == 0 {
		return nil
	}
	var (
		diagnostics      []Diagnostic
		hasDefinition    bool
		usesPluginHelper bool
		seenCaseFolded   = map[string]string{}
	)
	for _, rel := range rels {
		clean := filepath.ToSlash(filepath.Clean(rel))
		if clean != rel || strings.Contains(clean, "..") {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode tool file %s must stay within targets/opencode/tools without path traversal", rel),
			})
			continue
		}
		lower := strings.ToLower(clean)
		if prior, ok := seenCaseFolded[lower]; ok && prior != clean {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode tool files %s and %s collide on case-insensitive filesystems", prior, rel),
			})
		} else {
			seenCaseFolded[lower] = clean
		}
		fullPath := filepath.Join(root, rel)
		info, err := os.Lstat(fullPath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode tool file %s is not readable: %v", rel, err),
			})
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode tool file %s must not be a symlink", rel),
			})
			continue
		}
		if info.IsDir() {
			continue
		}
		body, err := os.ReadFile(fullPath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode tool file %s is not readable: %v", rel, err),
			})
			continue
		}
		if isOpenCodePluginEntryFile(rel) {
			hasDefinition = true
		}
		if strings.Contains(string(body), `@opencode-ai/plugin`) {
			usesPluginHelper = true
		}
	}
	if !hasDefinition {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join("src", "targets", "opencode", "tools")),
			Target:   "opencode",
			Message:  "OpenCode standalone tools require at least one JS/TS tool definition file under src/targets/opencode/tools",
		})
	}
	if usesPluginHelper && !openCodePackageDeclaresDependency(packageDoc, "@opencode-ai/plugin") {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join("src", "targets", "opencode", "package.json")),
			Target:   "opencode",
			Message:  `OpenCode standalone tool files that import "@opencode-ai/plugin" must declare that dependency in src/targets/opencode/package.json`,
		})
	}
	return diagnostics
}
