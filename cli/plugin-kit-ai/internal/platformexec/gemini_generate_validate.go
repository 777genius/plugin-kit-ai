package platformexec

import (
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func validateGeminiRenderReady(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta) error {
	var diagnostics []Diagnostic
	diagnostics = append(diagnostics, validateGeminiExcludeTools(state.DocPath("package_metadata"), meta.ExcludeTools)...)
	if graph.Portable.MCP != nil {
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "gemini")
		if err != nil {
			return err
		}
		diagnostics = append(diagnostics, validateGeminiMCPServers(graph.Portable.MCP.Path, projected)...)
	}
	diagnostics = append(diagnostics, validateGeminiContext(graph, state, meta)...)
	diagnostics = append(diagnostics, validateGeminiSettings(root, state.ComponentPaths("settings"))...)
	diagnostics = append(diagnostics, validateGeminiThemes(root, state.ComponentPaths("themes"))...)
	diagnostics = append(diagnostics, validateGeminiPolicies(root, state.ComponentPaths("policies"))...)
	diagnostics = append(diagnostics, validateGeminiCommands(root, state.ComponentPaths("commands"))...)
	diagnostics = append(diagnostics, validateGeminiHookFiles(root, state.ComponentPaths("hooks"))...)
	if graph.Launcher != nil {
		diagnostics = append(diagnostics, validateGeminiHookEntrypointConsistency(root, state.ComponentPaths("hooks"), strings.TrimSpace(graph.Launcher.Entrypoint))...)
	}
	if failures := collectDiagnosticMessages(diagnostics, SeverityFailure); len(failures) > 0 {
		return fmt.Errorf(failures[0])
	}
	return nil
}

func collectDiagnosticMessages(diagnostics []Diagnostic, severity DiagnosticSeverity) []string {
	var messages []string
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == severity {
			messages = append(messages, diagnostic.Message)
		}
	}
	return messages
}
