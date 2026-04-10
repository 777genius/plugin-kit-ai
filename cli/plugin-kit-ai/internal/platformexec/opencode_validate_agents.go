package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateOpenCodeAgentFiles(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		if filepath.Ext(rel) != ".md" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode agent file %s must use the .md extension", rel),
			})
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode agent file %s is not readable: %v", rel, err),
			})
			continue
		}
		frontmatter, _, err := parseMarkdownFrontmatterDocument(body, "OpenCode agent file "+rel)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  err.Error(),
			})
			continue
		}
		diagnostics = append(diagnostics, validateOpenCodeAgentFrontmatter(rel, frontmatter)...)
	}
	return diagnostics
}

func validateOpenCodeDefaultAgent(root string, rel string) []Diagnostic {
	if strings.TrimSpace(rel) == "" {
		return nil
	}
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode default agent file %s is not readable: %v", rel, err),
		}}
	}
	if strings.TrimSpace(string(body)) == "" {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode default agent file %s must contain a non-empty agent name", rel),
		}}
	}
	return nil
}

func validateOpenCodeAgentFrontmatter(rel string, frontmatter map[string]any) []Diagnostic {
	var diagnostics []Diagnostic
	description, ok := frontmatter["description"].(string)
	if !ok || strings.TrimSpace(description) == "" {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode agent file %s must declare a non-empty frontmatter description", rel),
		})
	}
	for _, field := range []string{"mode", "model", "variant", "color"} {
		diagnostics = append(diagnostics, validateOpenCodeAgentStringField(rel, frontmatter, field)...)
	}
	for _, numericField := range []string{"temperature", "top_p"} {
		if raw, ok := frontmatter[numericField]; ok {
			if _, ok := raw.(float64); !ok {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a number", rel, numericField),
				})
			}
		}
	}
	for _, boolField := range []string{"disable", "hidden"} {
		if raw, ok := frontmatter[boolField]; ok {
			if _, ok := raw.(bool); !ok {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a boolean", rel, boolField),
				})
			}
		}
	}
	if raw, ok := frontmatter["steps"]; ok {
		value, ok := raw.(int)
		if !ok || value <= 0 {
			if floatValue, ok := raw.(float64); !ok || floatValue != float64(int(floatValue)) || int(floatValue) <= 0 {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a positive integer", rel, "steps"),
				})
			}
		}
	}
	if raw, ok := frontmatter["options"]; ok {
		if _, ok := raw.(map[string]any); !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be an object", rel, "options"),
			})
		}
	}
	if raw, ok := frontmatter["permission"]; ok && !isOpenCodePermissionValue(raw) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a string or object", rel, "permission"),
		})
	}
	if _, ok := frontmatter["tools"]; ok {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q is deprecated; use %q instead", rel, "tools", "permission"),
		})
	}
	if _, ok := frontmatter["maxSteps"]; ok {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q is deprecated; use %q instead", rel, "maxSteps", "steps"),
		})
	}
	return diagnostics
}

func validateOpenCodeAgentStringField(rel string, frontmatter map[string]any, field string) []Diagnostic {
	raw, ok := frontmatter[field]
	if !ok {
		return nil
	}
	text, ok := raw.(string)
	if ok && strings.TrimSpace(text) != "" {
		return nil
	}
	return []Diagnostic{{
		Severity: SeverityFailure,
		Code:     CodeManifestInvalid,
		Path:     rel,
		Target:   "opencode",
		Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a non-empty string", rel, field),
	}}
}
