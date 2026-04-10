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

func validateOpenCodePluginPackageJSON(root string, rel string) (map[string]any, []Diagnostic) {
	if strings.TrimSpace(rel) == "" {
		return nil, nil
	}
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode plugin dependency metadata %s is not readable: %v", rel, err),
		}}
	}
	doc, err := decodeJSONObject(body, "OpenCode plugin dependency metadata "+rel)
	if err != nil {
		return nil, []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  err.Error(),
		}}
	}
	return doc, nil
}

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

func openCodePackageDeclaresDependency(doc map[string]any, name string) bool {
	if len(doc) == 0 {
		return false
	}
	for _, field := range []string{"dependencies", "devDependencies", "peerDependencies"} {
		raw, ok := doc[field]
		if !ok {
			continue
		}
		deps, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if value, ok := deps[name]; ok {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
	}
	return false
}

func isOpenCodePluginEntryFile(rel string) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".js", ".mjs", ".cjs", ".ts", ".mts", ".cts":
		return true
	default:
		return false
	}
}
