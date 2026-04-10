package platformexec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateOpenCodeCommandFiles(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		if filepath.Ext(rel) != ".md" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode command file %s must use the .md extension", rel),
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
				Message:  fmt.Sprintf("OpenCode command file %s is not readable: %v", rel, err),
			})
			continue
		}
		frontmatter, markdown, err := parseMarkdownFrontmatterDocument(body, "OpenCode command file "+rel)
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
		if strings.TrimSpace(markdown) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode command file %s must define a markdown command template body", rel),
			})
		}
		if description, ok := frontmatter["description"]; ok {
			text, ok := description.(string)
			if !ok || strings.TrimSpace(text) == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode command file %s frontmatter field %q must be a non-empty string", rel, "description"),
				})
			}
		}
	}
	return diagnostics
}

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
		if mode, ok := frontmatter["mode"]; ok {
			text, ok := mode.(string)
			if !ok || strings.TrimSpace(text) == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a non-empty string", rel, "mode"),
				})
			}
		}
		if model, ok := frontmatter["model"]; ok {
			text, ok := model.(string)
			if !ok || strings.TrimSpace(text) == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a non-empty string", rel, "model"),
				})
			}
		}
		if variant, ok := frontmatter["variant"]; ok {
			text, ok := variant.(string)
			if !ok || strings.TrimSpace(text) == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a non-empty string", rel, "variant"),
				})
			}
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
		if raw, ok := frontmatter["color"]; ok {
			text, ok := raw.(string)
			if !ok || strings.TrimSpace(text) == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a non-empty string", rel, "color"),
				})
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

func validateOpenCodeInstructions(root string, rel string) []Diagnostic {
	if strings.TrimSpace(rel) == "" {
		return nil
	}
	values, _, err := readYAMLDoc[[]string](root, rel)
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("parse %s: %v", rel, err),
		}}
	}
	if len(values) == 0 {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode instructions file %s must contain at least one instruction path", rel),
		}}
	}
	var diagnostics []Diagnostic
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode instructions file %s entry %d must be a non-empty string", rel, i),
			})
		}
	}
	return diagnostics
}

func validateOpenCodePermission(root string, rel string) []Diagnostic {
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
			Message:  fmt.Sprintf("OpenCode permission file %s is not readable: %v", rel, err),
		}}
	}
	var permission any
	if err := json.Unmarshal(body, &permission); err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("parse %s: %v", rel, err),
		}}
	}
	if !isOpenCodePermissionValue(permission) {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode permission file %s must be a JSON string or object", rel),
		}}
	}
	return nil
}

func validateOpenCodeThemeFiles(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		if filepath.Ext(rel) != ".json" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode theme file %s must use the .json extension", rel),
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
				Message:  fmt.Sprintf("OpenCode theme file %s is not readable: %v", rel, err),
			})
			continue
		}
		doc, err := decodeJSONObject(body, "OpenCode theme file "+rel)
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
		if _, ok := doc["theme"]; !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode theme file %s must define a top-level theme object", rel),
			})
		}
	}
	return diagnostics
}
