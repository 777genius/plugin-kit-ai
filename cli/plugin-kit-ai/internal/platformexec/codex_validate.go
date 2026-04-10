package platformexec

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/codexconfig"
	"github.com/777genius/plugin-kit-ai/cli/internal/codexmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func (codexPackageAdapter) Validate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]Diagnostic, error) {
	if err := codexmanifest.ValidatePluginDirLayout(root); err != nil {
		path := codexmanifest.PluginManifestPath()
		var layoutErr *codexmanifest.PluginDirLayoutError
		if errors.As(err, &layoutErr) && strings.TrimSpace(layoutErr.Path) != "" {
			path = layoutErr.Path
		}
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     path,
			Target:   "codex-package",
			Message:  err.Error(),
		}}, nil
	}
	body, err := os.ReadFile(filepath.Join(root, codexmanifest.PluginDir, codexmanifest.PluginFileName))
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     codexmanifest.PluginManifestPath(),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest %s is not readable: %v", codexmanifest.PluginManifestPath(), err),
		}}, nil
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     codexmanifest.PluginManifestPath(),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest %s is invalid JSON: %v", codexmanifest.PluginManifestPath(), err),
		}}, nil
	}
	pluginManifest, err := codexmanifest.DecodeImportedPluginManifest(body)
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     codexmanifest.PluginManifestPath(),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest %s is invalid: %v", codexmanifest.PluginManifestPath(), err),
		}}, nil
	}
	var diagnostics []Diagnostic
	for _, path := range codexmanifest.UnexpectedBundleSidecars(root, pluginManifest) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     path,
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex package bundle may not include %s without a matching .codex-plugin/plugin.json ref", path),
		})
	}
	if strings.TrimSpace(pluginManifest.Name) != strings.TrimSpace(graph.Manifest.Name) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     codexmanifest.PluginManifestPath(),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json sets name %q; expected %q from plugin.yaml", strings.TrimSpace(pluginManifest.Name), strings.TrimSpace(graph.Manifest.Name)),
		})
	}
	if strings.TrimSpace(pluginManifest.Version) != strings.TrimSpace(graph.Manifest.Version) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     codexmanifest.PluginManifestPath(),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json sets version %q; expected %q from plugin.yaml", strings.TrimSpace(pluginManifest.Version), strings.TrimSpace(graph.Manifest.Version)),
		})
	}
	if strings.TrimSpace(pluginManifest.Description) != strings.TrimSpace(graph.Manifest.Description) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     codexmanifest.PluginManifestPath(),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json sets description %q; expected %q from plugin.yaml", strings.TrimSpace(pluginManifest.Description), strings.TrimSpace(graph.Manifest.Description)),
		})
	}
	if meta, ok, err := readYAMLDoc[codexPackageMeta](root, state.DocPath("package_metadata")); err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	} else {
		expectedMeta := codexPackageMeta{
			Author:     manifestAuthorToCodex(graph.Manifest.Author),
			Homepage:   strings.TrimSpace(graph.Manifest.Homepage),
			Repository: strings.TrimSpace(graph.Manifest.Repository),
			License:    strings.TrimSpace(graph.Manifest.License),
			Keywords:   append([]string(nil), graph.Manifest.Keywords...),
		}
		if ok {
			mergeCodexPackageMeta(&expectedMeta, meta)
		}
		expectedMeta.Normalize()
		if !codexPackageMetaEqual(expectedMeta, pluginManifest.PackageMeta) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     codexmanifest.PluginManifestPath(),
				Target:   "codex-package",
				Message:  "Codex plugin manifest .codex-plugin/plugin.json package metadata does not match plugin.yaml plus optional targets/codex-package/package.yaml overrides",
			})
		}
	}
	if hasSkills := len(graph.Portable.Paths("skills")) > 0; hasSkills {
		if strings.TrimSpace(pluginManifest.SkillsPath) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     codexmanifest.PluginManifestPath(),
				Target:   "codex-package",
				Message:  "Codex plugin manifest .codex-plugin/plugin.json must reference ./skills/ when portable skills are authored",
			})
		}
	} else if strings.TrimSpace(pluginManifest.SkillsPath) != "" {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     codexmanifest.PluginManifestPath(),
			Target:   "codex-package",
			Message:  "Codex plugin manifest .codex-plugin/plugin.json may not reference skills when no portable skills are authored",
		})
	}
	if ref := strings.TrimSpace(pluginManifest.SkillsPath); ref != "" && ref != codexmanifest.SkillsRef {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     codexmanifest.PluginManifestPath(),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json must use %q for skills when present", codexmanifest.SkillsRef),
		})
	}
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
		} else if mcpBody, err := os.ReadFile(filepath.Join(root, refPath)); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     filepath.ToSlash(refPath),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex MCP manifest %s is not readable: %v", filepath.ToSlash(refPath), err),
			})
		} else if renderedMCP, err := decodeJSONObject(mcpBody, fmt.Sprintf("Codex MCP manifest %s", filepath.ToSlash(refPath))); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(refPath),
				Target:   "codex-package",
				Message:  err.Error(),
			})
		} else if graph.Portable.MCP != nil {
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
	var authoredAppEnabled bool
	var authoredAppDoc map[string]any
	if rel := strings.TrimSpace(state.DocPath("interface")); rel != "" {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(rel),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex interface doc %s is not readable: %v", filepath.ToSlash(rel), err),
			}}, nil
		}
		interfaceDoc, err := codexmanifest.ParseInterfaceDoc(body)
		if err != nil {
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(rel),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex interface doc %s is invalid: %v", filepath.ToSlash(rel), err),
			}}, nil
		}
		if !jsonDocumentsEqual(interfaceDoc, pluginManifest.Interface) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     codexmanifest.PluginManifestPath(),
				Target:   "codex-package",
				Message:  "Codex plugin manifest .codex-plugin/plugin.json interface does not match targets/codex-package/interface.json",
			})
		}
	} else if pluginManifest.Interface != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     codexmanifest.PluginManifestPath(),
			Target:   "codex-package",
			Message:  "Codex plugin manifest .codex-plugin/plugin.json may not define interface when targets/codex-package/interface.json is absent",
		})
	}
	if rel := strings.TrimSpace(state.DocPath("app_manifest")); rel != "" {
		sourceBody, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(rel),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex app manifest %s is not readable: %v", filepath.ToSlash(rel), err),
			}}, nil
		}
		appDoc, err := codexmanifest.ParseAppManifestDoc(sourceBody)
		if err != nil {
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(rel),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex app manifest %s is invalid: %v", filepath.ToSlash(rel), err),
			}}, nil
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
		} else if body, err := os.ReadFile(filepath.Join(root, refPath)); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     filepath.ToSlash(refPath),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex app manifest %s is not readable: %v", filepath.ToSlash(refPath), err),
			})
		} else if renderedAppDoc, err := codexmanifest.ParseAppManifestDoc(body); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(refPath),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex app manifest %s is invalid: %v", filepath.ToSlash(refPath), err),
			})
		} else if authoredAppEnabled && !jsonDocumentsEqual(authoredAppDoc, renderedAppDoc) {
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

func (codexRuntimeAdapter) Validate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]Diagnostic, error) {
	config, _, err := codexconfig.ReadImportedConfig(root)
	if err != nil {
		path := filepath.ToSlash(filepath.Join(".codex", "config.toml"))
		code := CodeManifestInvalid
		message := fmt.Sprintf("Codex config file %s is invalid: %v", path, err)
		if os.IsNotExist(err) {
			code = CodeGeneratedContractInvalid
			message = fmt.Sprintf("Codex config file %s is not readable: %v", path, err)
		}
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     code,
			Path:     path,
			Target:   "codex-runtime",
			Message:  message,
		}}, nil
	}
	configPath := filepath.ToSlash(filepath.Join(".codex", "config.toml"))
	if graph.Launcher == nil {
		return nil, nil
	}
	var diagnostics []Diagnostic
	expectedModel := "gpt-5.4-mini"
	configExtra, err := loadNativeExtraDoc(root, state, "config_extra", pluginmodel.NativeDocFormatTOML)
	if err != nil {
		return nil, err
	}
	expectedNotify := []string{graph.Launcher.Entrypoint, "notify"}
	if len(config.Notify) != len(expectedNotify) || len(config.Notify) == 0 || strings.TrimSpace(config.Notify[0]) != expectedNotify[0] || (len(config.Notify) > 1 && strings.TrimSpace(config.Notify[1]) != expectedNotify[1]) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeEntrypointMismatch,
			Path:     configPath,
			Target:   "codex-runtime",
			Message:  fmt.Sprintf("entrypoint mismatch: Codex notify argv uses %q; expected %q from launcher.yaml entrypoint", config.Notify, expectedNotify),
		})
	}
	if meta, ok, err := readYAMLDoc[codexRuntimeMeta](root, state.DocPath("package_metadata")); err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	} else if ok && strings.TrimSpace(meta.ModelHint) != "" {
		expectedModel = strings.TrimSpace(meta.ModelHint)
	}
	if strings.TrimSpace(config.Model) != expectedModel {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     configPath,
			Target:   "codex-runtime",
			Message:  fmt.Sprintf("Codex config model %q does not match expected model %q", strings.TrimSpace(config.Model), expectedModel),
		})
	}
	if len(configExtra.Fields) > 0 {
		if !jsonDocumentsEqual(configExtra.Fields, config.Extra) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     configPath,
				Target:   "codex-runtime",
				Message:  "Codex config .codex/config.toml passthrough fields do not match targets/codex-runtime/config.extra.toml",
			})
		}
	} else if len(config.Extra) > 0 {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     configPath,
			Target:   "codex-runtime",
			Message:  "Codex config .codex/config.toml may not define passthrough fields when targets/codex-runtime/config.extra.toml is absent",
		})
	}
	return diagnostics, nil
}
