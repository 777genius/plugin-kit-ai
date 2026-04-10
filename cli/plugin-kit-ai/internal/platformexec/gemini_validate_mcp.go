package platformexec

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

func geminiPackageMetaEqual(left, right geminiPackageMeta) bool {
	return strings.TrimSpace(left.ContextFileName) == strings.TrimSpace(right.ContextFileName) &&
		slices.Equal(normalizeGeminiExcludeTools(left.ExcludeTools), normalizeGeminiExcludeTools(right.ExcludeTools)) &&
		strings.TrimSpace(left.MigratedTo) == strings.TrimSpace(right.MigratedTo) &&
		strings.TrimSpace(left.PlanDirectory) == strings.TrimSpace(right.PlanDirectory)
}

func geminiExtensionDirBase(root string) string {
	abs, err := filepath.Abs(root)
	if err == nil {
		return filepath.Base(filepath.Clean(abs))
	}
	return filepath.Base(filepath.Clean(root))
}

func normalizeGeminiExcludeTools(values []string) []string {
	var out []string
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func validateGeminiExcludeTools(path string, values []string) []Diagnostic {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     path,
				Target:   "gemini",
				Message:  "Gemini exclude_tools entries must be non-empty strings naming built-in tools",
			}}
		}
	}
	return nil
}

func validateGeminiMCPServers(path string, servers map[string]any) []Diagnostic {
	var diagnostics []Diagnostic
	for serverName, raw := range servers {
		server, ok := raw.(map[string]any)
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     path,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension MCP server %q must be a JSON object", serverName),
			})
			continue
		}
		if _, blocked := server["trust"]; blocked {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     path,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension MCP server %q may not set trust", serverName),
			})
		}
		command, hasCommand := geminiOptionalString(server["command"])
		url, hasURL := geminiOptionalString(server["url"])
		httpURL, hasHTTPURL := geminiOptionalString(server["httpUrl"])
		transportCount := 0
		if hasCommand {
			transportCount++
		}
		if hasURL {
			transportCount++
		}
		if hasHTTPURL {
			transportCount++
		}
		if transportCount != 1 {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     path,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension MCP server %q must define exactly one transport via command, url, or httpUrl", serverName),
			})
		}
		if hasArgs := server["args"] != nil; hasArgs && !hasCommand {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     path,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension MCP server %q may only use args with command-based stdio transport", serverName),
			})
		}
		if hasEnv := server["env"] != nil; hasEnv && !hasCommand {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     path,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension MCP server %q may only use env with command-based stdio transport", serverName),
			})
		}
		if hasCwd := server["cwd"] != nil; hasCwd && !hasCommand {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     path,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension MCP server %q may only use cwd with command-based stdio transport", serverName),
			})
		}
		if value, ok := server["args"]; ok {
			items, valid := geminiStringSlice(value)
			if !valid {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     path,
					Target:   "gemini",
					Message:  fmt.Sprintf("Gemini extension MCP server %q args must be an array of strings", serverName),
				})
			} else if len(items) == 0 {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     path,
					Target:   "gemini",
					Message:  fmt.Sprintf("Gemini extension MCP server %q args may not be empty when provided", serverName),
				})
			}
		}
		if value, ok := server["env"]; ok {
			if _, valid := geminiStringMap(value); !valid {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     path,
					Target:   "gemini",
					Message:  fmt.Sprintf("Gemini extension MCP server %q env must be an object of string values", serverName),
				})
			}
		}
		if value, ok := server["cwd"]; ok {
			if cwd, ok := value.(string); !ok || strings.TrimSpace(cwd) == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     path,
					Target:   "gemini",
					Message:  fmt.Sprintf("Gemini extension MCP server %q cwd must be a non-empty string", serverName),
				})
			}
		}
		if hasCommand && strings.Contains(command, " ") && server["args"] == nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityWarning,
				Code:     CodeGeminiMCPCommandStyle,
				Path:     path,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension MCP server %q uses a space-delimited command string; prefer command plus args", serverName),
			})
		}
		_ = url
		_ = httpURL
	}
	return diagnostics
}

func geminiOptionalString(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	return text, true
}

func geminiStringSlice(value any) ([]string, bool) {
	raw, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, false
		}
		out = append(out, strings.TrimSpace(text))
	}
	return out, true
}

func geminiStringMap(value any) (map[string]string, bool) {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	out := make(map[string]string, len(raw))
	for key, item := range raw {
		text, ok := item.(string)
		if !ok {
			return nil, false
		}
		out[key] = text
	}
	return out, true
}
