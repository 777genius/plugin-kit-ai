package platformexec

import (
	"fmt"
	"strings"
)

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
		diagnostics = append(diagnostics, validateGeminiMCPServer(path, serverName, server)...)
	}
	return diagnostics
}

func validateGeminiMCPServer(path, serverName string, server map[string]any) []Diagnostic {
	var diagnostics []Diagnostic
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
	_, hasURL := geminiOptionalString(server["url"])
	_, hasHTTPURL := geminiOptionalString(server["httpUrl"])
	if countTruthy(hasCommand, hasURL, hasHTTPURL) != 1 {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     path,
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension MCP server %q must define exactly one transport via command, url, or httpUrl", serverName),
		})
	}

	diagnostics = append(diagnostics, validateGeminiMCPCommandFields(path, serverName, server, hasCommand)...)
	diagnostics = append(diagnostics, validateGeminiMCPArgs(path, serverName, server["args"])...)
	diagnostics = append(diagnostics, validateGeminiMCPEnv(path, serverName, server["env"])...)
	diagnostics = append(diagnostics, validateGeminiMCPCwd(path, serverName, server["cwd"])...)

	if hasCommand && strings.Contains(command, " ") && server["args"] == nil {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityWarning,
			Code:     CodeGeminiMCPCommandStyle,
			Path:     path,
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension MCP server %q uses a space-delimited command string; prefer command plus args", serverName),
		})
	}
	return diagnostics
}

func validateGeminiMCPCommandFields(path, serverName string, server map[string]any, hasCommand bool) []Diagnostic {
	var diagnostics []Diagnostic
	if server["args"] != nil && !hasCommand {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     path,
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension MCP server %q may only use args with command-based stdio transport", serverName),
		})
	}
	if server["env"] != nil && !hasCommand {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     path,
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension MCP server %q may only use env with command-based stdio transport", serverName),
		})
	}
	if server["cwd"] != nil && !hasCommand {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     path,
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension MCP server %q may only use cwd with command-based stdio transport", serverName),
		})
	}
	return diagnostics
}

func validateGeminiMCPArgs(path, serverName string, value any) []Diagnostic {
	if value == nil {
		return nil
	}
	items, valid := geminiStringSlice(value)
	if !valid {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     path,
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension MCP server %q args must be an array of strings", serverName),
		}}
	}
	if len(items) == 0 {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     path,
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension MCP server %q args may not be empty when provided", serverName),
		}}
	}
	return nil
}

func validateGeminiMCPEnv(path, serverName string, value any) []Diagnostic {
	if value == nil {
		return nil
	}
	if _, valid := geminiStringMap(value); !valid {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     path,
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension MCP server %q env must be an object of string values", serverName),
		}}
	}
	return nil
}

func validateGeminiMCPCwd(path, serverName string, value any) []Diagnostic {
	if value == nil {
		return nil
	}
	if cwd, ok := value.(string); !ok || strings.TrimSpace(cwd) == "" {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     path,
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension MCP server %q cwd must be a non-empty string", serverName),
		}}
	}
	return nil
}

func countTruthy(values ...bool) int {
	total := 0
	for _, value := range values {
		if value {
			total++
		}
	}
	return total
}
