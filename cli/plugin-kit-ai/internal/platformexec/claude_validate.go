package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func (claudeAdapter) Validate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]Diagnostic, error) {
	var diagnostics []Diagnostic
	authoredRoot := authoredRootHint(state, graph.Portable)
	if graph.Launcher == nil {
		if rel := claudePrimaryHookPath(state); rel != "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "claude",
				Message:  fmt.Sprintf("Claude hooks require %s/%s when %s/targets/claude/hooks/** is authored", authoredRoot, pluginmodel.LauncherFileName, authoredRoot),
			})
		} else if !claudeHasPackageOnlySurface(graph, state) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(filepath.Join(authoredRoot, pluginmodel.FileName)),
				Target:   "claude",
				Message:  fmt.Sprintf("target claude without %s/launcher.yaml must author at least one package-only surface such as %s/mcp/servers.yaml, %s/skills/, %s/targets/claude/settings.json, %s/targets/claude/lsp.json, %s/targets/claude/user-config.json, %s/targets/claude/manifest.extra.json, or %s/targets/claude/commands/** and %s/targets/claude/agents/**", authoredRoot, authoredRoot, authoredRoot, authoredRoot, authoredRoot, authoredRoot, authoredRoot, authoredRoot, authoredRoot),
			})
		}
	}
	if graph.Launcher != nil {
		for _, rel := range state.ComponentPaths("hooks") {
			full := filepath.Join(root, rel)
			body, err := os.ReadFile(full)
			if err != nil {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "claude",
					Message:  fmt.Sprintf("Claude hooks file %s is not readable: %v", rel, err),
				})
				continue
			}
			mismatches, err := validateClaudeHookEntrypoints(body, graph.Launcher.Entrypoint)
			if err != nil {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "claude",
					Message:  fmt.Sprintf("Claude hooks file %s is invalid JSON: %v", rel, err),
				})
				continue
			}
			for _, mismatch := range mismatches {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeEntrypointMismatch,
					Path:     rel,
					Target:   "claude",
					Message:  mismatch,
				})
			}
		}
	}
	diagnostics = append(diagnostics, validateClaudeSettings(root, state.DocPath("settings"))...)
	diagnostics = append(diagnostics, validateClaudeLSP(root, state.DocPath("lsp"))...)
	diagnostics = append(diagnostics, validateClaudeUserConfig(root, state.DocPath("user_config"))...)
	return diagnostics, nil
}

func validateClaudeSettings(root, rel string) []Diagnostic {
	doc, _, ok, err := loadClaudeJSONDoc(root, rel, "Claude settings")
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "claude",
			Message:  err.Error(),
		}}
	}
	if !ok {
		return nil
	}
	if value, exists := doc["agent"]; exists {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "claude",
				Message:  fmt.Sprintf(`Claude settings file %s must set "agent" as a non-empty string when present`, rel),
			}}
		}
	}
	return nil
}

func validateClaudeLSP(root, rel string) []Diagnostic {
	_, _, ok, err := loadClaudeJSONDoc(root, rel, "Claude LSP")
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "claude",
			Message:  err.Error(),
		}}
	}
	if !ok {
		return nil
	}
	return nil
}

func validateClaudeUserConfig(root, rel string) []Diagnostic {
	doc, _, ok, err := loadClaudeJSONDoc(root, rel, "Claude userConfig")
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "claude",
			Message:  err.Error(),
		}}
	}
	if !ok {
		return nil
	}
	for key, value := range doc {
		if _, ok := value.(map[string]any); !ok {
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "claude",
				Message:  fmt.Sprintf("Claude userConfig entry %q in %s must be a JSON object", key, rel),
			}}
		}
	}
	return nil
}
