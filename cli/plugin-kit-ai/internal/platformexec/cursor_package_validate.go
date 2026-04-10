package platformexec

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func (cursorAdapter) Validate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]Diagnostic, error) {
	var diagnostics []Diagnostic
	manifest, ok, err := readImportedCursorPluginManifest(root)
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     cursorPluginManifestPath,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin manifest %s is invalid: %v", cursorPluginManifestPath, err),
		}}, nil
	}
	if !ok {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     cursorPluginManifestPath,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin manifest %s is not readable", cursorPluginManifestPath),
		}}, nil
	}
	diagnostics = append(diagnostics, validateCursorPluginIdentity(manifest, graph)...)
	if graph.Portable.MCP != nil {
		mcpDiagnostics, err := validateCursorPortableMCP(root, manifest, graph)
		if err != nil {
			return nil, err
		}
		return append(diagnostics, mcpDiagnostics...), nil
	}
	if _, ok := manifest["mcpServers"]; ok {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     cursorPluginManifestPath,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin manifest %s may not define mcpServers when portable MCP is absent", cursorPluginManifestPath),
		})
	}
	return diagnostics, nil
}

func validateCursorPluginIdentity(manifest map[string]any, graph pluginmodel.PackageGraph) []Diagnostic {
	var diagnostics []Diagnostic
	if got := stringMapField(manifest, "name"); got != strings.TrimSpace(graph.Manifest.Name) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     cursorPluginManifestPath,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin manifest %s sets name %q; expected %q from plugin.yaml", cursorPluginManifestPath, got, strings.TrimSpace(graph.Manifest.Name)),
		})
	}
	if got := stringMapField(manifest, "version"); got != strings.TrimSpace(graph.Manifest.Version) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     cursorPluginManifestPath,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin manifest %s sets version %q; expected %q from plugin.yaml", cursorPluginManifestPath, got, strings.TrimSpace(graph.Manifest.Version)),
		})
	}
	if got := stringMapField(manifest, "description"); got != strings.TrimSpace(graph.Manifest.Description) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     cursorPluginManifestPath,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin manifest %s sets description %q; expected %q from plugin.yaml", cursorPluginManifestPath, got, strings.TrimSpace(graph.Manifest.Description)),
		})
	}
	return diagnostics
}

func validateCursorPortableMCP(root string, manifest map[string]any, graph pluginmodel.PackageGraph) ([]Diagnostic, error) {
	var diagnostics []Diagnostic
	ref, ok := manifest["mcpServers"]
	if !ok {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     cursorPluginManifestPath,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin manifest %s must reference %q when portable MCP is authored", cursorPluginManifestPath, cursorPluginMCPRef),
		})
		return diagnostics, nil
	}
	refText, ok := ref.(string)
	if !ok {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     cursorPluginManifestPath,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin manifest %s field %q must be the string %q", cursorPluginManifestPath, "mcpServers", cursorPluginMCPRef),
		})
		return diagnostics, nil
	}
	if strings.TrimSpace(refText) != cursorPluginMCPRef {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     cursorPluginManifestPath,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin manifest %s must use %q for mcpServers when portable MCP is present", cursorPluginManifestPath, cursorPluginMCPRef),
		})
	}
	projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "cursor")
	if err != nil {
		return nil, err
	}
	rendered, ok, err := readCursorMCPServers(filepath.Join(root, ".mcp.json"), ".mcp.json")
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     ".mcp.json",
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin MCP manifest .mcp.json is invalid: %v", err),
		})
		return diagnostics, nil
	}
	if !ok {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     ".mcp.json",
			Target:   "cursor",
			Message:  "Cursor plugin MCP manifest .mcp.json is not readable",
		})
		return diagnostics, nil
	}
	if !jsonDocumentsEqual(projected, rendered) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     ".mcp.json",
			Target:   "cursor",
			Message:  "Cursor plugin MCP manifest .mcp.json does not match authored portable MCP projection",
		})
	}
	return diagnostics, nil
}
