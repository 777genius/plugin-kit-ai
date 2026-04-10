package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateOpenCodePluginFiles(root string, rels []string, packageDoc map[string]any) []Diagnostic {
	if len(rels) == 0 {
		return nil
	}
	var (
		diagnostics      []Diagnostic
		hasEntry         bool
		usesPluginHelper bool
	)
	for _, rel := range rels {
		fullPath := filepath.Join(root, rel)
		info, err := os.Stat(fullPath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode local plugin file %s is not readable: %v", rel, err),
			})
			continue
		}
		if info.IsDir() {
			continue
		}
		if isOpenCodePluginEntryFile(rel) {
			hasEntry = true
		}
		body, err := os.ReadFile(fullPath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode local plugin file %s is not readable: %v", rel, err),
			})
			continue
		}
		src := string(body)
		if strings.Contains(src, `export default`) && strings.Contains(src, `setup(`) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  "OpenCode local plugin file uses the old scaffold shape `export default { setup() { ... } }`; use official named async plugin exports instead",
			})
		}
		if strings.Contains(src, `@opencode-ai/plugin`) {
			usesPluginHelper = true
		}
	}
	if !hasEntry {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join("src", "targets", "opencode", "plugins")),
			Target:   "opencode",
			Message:  "OpenCode local plugin code requires at least one JS/TS plugin entry file under src/targets/opencode/plugins (for example .js, .mjs, .cjs, .ts, .mts, or .cts)",
		})
	}
	if usesPluginHelper && !openCodePackageDeclaresDependency(packageDoc, "@opencode-ai/plugin") {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join("src", "targets", "opencode", "package.json")),
			Target:   "opencode",
			Message:  `OpenCode plugin files that import "@opencode-ai/plugin" must declare that dependency in src/targets/opencode/package.json`,
		})
	}
	return diagnostics
}

func isOpenCodePluginEntryFile(rel string) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".js", ".mjs", ".cjs", ".ts", ".mts", ".cts":
		return true
	default:
		return false
	}
}
