package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/codexmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func validateCodexMCPDiagnostics(root string, graph pluginmodel.PackageGraph, pluginManifest codexmanifest.ImportedPluginManifest) ([]Diagnostic, error) {
	var diagnostics []Diagnostic
	if hasMCP := graph.Portable.MCP != nil; hasMCP {
		if strings.TrimSpace(pluginManifest.MCPServersRef) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     codexmanifest.PluginManifestPath(),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json must reference %q when portable MCP is authored", codexmanifest.MCPServersRef),
			})
		}
	} else if strings.TrimSpace(pluginManifest.MCPServersRef) != "" {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     codexmanifest.PluginManifestPath(),
			Target:   "codex-package",
			Message:  "Codex plugin manifest .codex-plugin/plugin.json may not reference mcpServers when no portable MCP is authored",
		})
	}
	if ref := strings.TrimSpace(pluginManifest.MCPServersRef); ref != "" {
		if ref != codexmanifest.MCPServersRef {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     codexmanifest.PluginManifestPath(),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json must use %q for mcpServers when present", codexmanifest.MCPServersRef),
			})
		}
		refPath, err := resolveRelativeRef(root, ref)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     codexmanifest.PluginManifestPath(),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json uses an invalid mcpServers ref %q: %v", ref, err),
			})
			return diagnostics, nil
		}
		mcpBody, err := os.ReadFile(filepath.Join(root, refPath))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     filepath.ToSlash(refPath),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex MCP manifest %s is not readable: %v", filepath.ToSlash(refPath), err),
			})
			return diagnostics, nil
		}
		renderedMCP, err := decodeJSONObject(mcpBody, fmt.Sprintf("Codex MCP manifest %s", filepath.ToSlash(refPath)))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(refPath),
				Target:   "codex-package",
				Message:  err.Error(),
			})
			return diagnostics, nil
		}
		if graph.Portable.MCP != nil {
			projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "codex-package")
			if err != nil {
				return nil, err
			}
			if !jsonDocumentsEqual(projected, renderedMCP) {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeGeneratedContractInvalid,
					Path:     filepath.ToSlash(refPath),
					Target:   "codex-package",
					Message:  "Codex MCP manifest .mcp.json does not match authored portable MCP projection",
				})
			}
		}
	}
	return diagnostics, nil
}

func validateCodexInterfaceDiagnostics(root string, state pluginmodel.TargetState, pluginManifest codexmanifest.ImportedPluginManifest) ([]Diagnostic, error) {
	if rel := strings.TrimSpace(state.DocPath("interface")); rel != "" {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(rel),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex interface doc %s is not readable: %v", filepath.ToSlash(rel), err),
			}}, err
		}
		interfaceDoc, err := codexmanifest.ParseInterfaceDoc(body)
		if err != nil {
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(rel),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex interface doc %s is invalid: %v", filepath.ToSlash(rel), err),
			}}, err
		}
		if !jsonDocumentsEqual(interfaceDoc, pluginManifest.Interface) {
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     codexmanifest.PluginManifestPath(),
				Target:   "codex-package",
				Message:  "Codex plugin manifest .codex-plugin/plugin.json interface does not match targets/codex-package/interface.json",
			}}, nil
		}
		return nil, nil
	}
	if pluginManifest.Interface != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     codexmanifest.PluginManifestPath(),
			Target:   "codex-package",
			Message:  "Codex plugin manifest .codex-plugin/plugin.json may not define interface when targets/codex-package/interface.json is absent",
		}}, nil
	}
	return nil, nil
}

func validateCodexAppDiagnostics(root string, state pluginmodel.TargetState, pluginManifest codexmanifest.ImportedPluginManifest) ([]Diagnostic, error) {
	var diagnostics []Diagnostic
	var authoredAppEnabled bool
	var authoredAppDoc map[string]any
	if rel := strings.TrimSpace(state.DocPath("app_manifest")); rel != "" {
		sourceBody, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(rel),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex app manifest %s is not readable: %v", filepath.ToSlash(rel), err),
			}}, err
		}
		appDoc, err := codexmanifest.ParseAppManifestDoc(sourceBody)
		if err != nil {
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(rel),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex app manifest %s is invalid: %v", filepath.ToSlash(rel), err),
			}}, err
		}
		authoredAppDoc = appDoc
		authoredAppEnabled = codexmanifest.AppManifestEnabled(appDoc)
	}
	if authoredAppEnabled && strings.TrimSpace(pluginManifest.AppsRef) == "" {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     codexmanifest.PluginManifestPath(),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json must reference %q when targets/codex-package/app.json is enabled", codexmanifest.AppsRef),
		})
	}
	if !authoredAppEnabled && strings.TrimSpace(pluginManifest.AppsRef) != "" {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     codexmanifest.PluginManifestPath(),
			Target:   "codex-package",
			Message:  "Codex plugin manifest .codex-plugin/plugin.json may not reference apps when targets/codex-package/app.json is empty or absent",
		})
	}
	if ref := strings.TrimSpace(pluginManifest.AppsRef); ref != "" {
		if ref != codexmanifest.AppsRef {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     codexmanifest.PluginManifestPath(),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json must use %q for apps when present", codexmanifest.AppsRef),
			})
		}
		refPath, err := resolveRelativeRef(root, ref)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     codexmanifest.PluginManifestPath(),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json uses an invalid apps ref %q: %v", ref, err),
			})
			return diagnostics, nil
		}
		body, err := os.ReadFile(filepath.Join(root, refPath))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     filepath.ToSlash(refPath),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex app manifest %s is not readable: %v", filepath.ToSlash(refPath), err),
			})
			return diagnostics, nil
		}
		renderedAppDoc, err := codexmanifest.ParseAppManifestDoc(body)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(refPath),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex app manifest %s is invalid: %v", filepath.ToSlash(refPath), err),
			})
			return diagnostics, nil
		}
		if authoredAppEnabled && !jsonDocumentsEqual(authoredAppDoc, renderedAppDoc) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     filepath.ToSlash(refPath),
				Target:   "codex-package",
				Message:  "Codex app manifest .app.json does not match targets/codex-package/app.json",
			})
		}
	}
	return diagnostics, nil
}
