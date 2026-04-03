package platformexec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/codexconfig"
	"github.com/777genius/plugin-kit-ai/cli/internal/codexmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/pelletier/go-toml/v2"
)

type codexPackageAdapter struct{}
type codexRuntimeAdapter struct{}

func (codexPackageAdapter) ID() string { return "codex-package" }
func (codexRuntimeAdapter) ID() string { return "codex-runtime" }

func (codexPackageAdapter) DetectNative(root string) bool {
	return fileExists(filepath.Join(root, ".codex-plugin", "plugin.json"))
}

func (codexRuntimeAdapter) DetectNative(root string) bool {
	return fileExists(filepath.Join(root, ".codex", "config.toml"))
}

func (codexPackageAdapter) RefineDiscovery(root string, state *pluginmodel.TargetState) error {
	if rel := state.DocPath("package_metadata"); strings.TrimSpace(rel) != "" {
		if _, ok, err := readYAMLDoc[codexPackageMeta](root, rel); err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		} else if !ok {
			return nil
		}
	}
	if rel := state.DocPath("interface"); strings.TrimSpace(rel) != "" {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return err
		}
		if _, err := codexmanifest.ParseInterfaceDoc(body); err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}
	}
	if rel := state.DocPath("app_manifest"); strings.TrimSpace(rel) != "" {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return err
		}
		if _, err := codexmanifest.ParseAppManifestDoc(body); err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}
	}
	return nil
}

func (codexRuntimeAdapter) RefineDiscovery(root string, state *pluginmodel.TargetState) error {
	if rel := state.DocPath("package_metadata"); strings.TrimSpace(rel) != "" {
		if _, ok, err := readYAMLDoc[codexRuntimeMeta](root, rel); err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		} else if !ok {
			return nil
		}
	}
	return nil
}

func (codexPackageAdapter) Import(root string, seed ImportSeed) (ImportResult, error) {
	result := ImportResult{Manifest: seed.Manifest, Launcher: nil}
	pluginManifest, _, err := readImportedCodexPluginManifest(root)
	if err != nil {
		return ImportResult{}, err
	}
	result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
		RelPath: filepath.Join("targets", "codex-package", "package.yaml"),
		Content: mustYAML(pluginManifest.PackageMeta),
	})
	if strings.TrimSpace(pluginManifest.Name) != "" {
		result.Manifest.Name = pluginManifest.Name
	}
	if strings.TrimSpace(pluginManifest.Version) != "" {
		result.Manifest.Version = pluginManifest.Version
	}
	if strings.TrimSpace(pluginManifest.Description) != "" {
		result.Manifest.Description = pluginManifest.Description
	}

	extra := cloneStringMap(pluginManifest.Extra)
	if pluginManifest.Interface != nil {
		body, err := marshalJSON(pluginManifest.Interface)
		if err != nil {
			return ImportResult{}, err
		}
		result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join("targets", "codex-package", "interface.json"),
			Content: body,
		})
	}
	if ref := strings.TrimSpace(pluginManifest.AppsRef); ref != "" {
		appBody, err := os.ReadFile(filepath.Join(root, cleanRelativeRef(ref)))
		if err != nil {
			return ImportResult{}, err
		}
		if _, err := codexmanifest.ParseAppManifestDoc(appBody); err != nil {
			return ImportResult{}, fmt.Errorf("parse %s: %w", filepath.ToSlash(cleanRelativeRef(ref)), err)
		}
		result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join("targets", "codex-package", "app.json"),
			Content: append([]byte(nil), appBody...),
		})
		if ref != codexmanifest.AppsRef {
			result.Warnings = append(result.Warnings, pluginmodel.Warning{
				Kind:    pluginmodel.WarningFidelity,
				Path:    filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
				Message: "normalized Codex plugin apps path to the managed ./.app.json location",
			})
		}
	}
	if len(extra) > 0 {
		result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join("targets", "codex-package", "manifest.extra.json"),
			Content: mustJSON(extra),
		})
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join("targets", "codex-package", "manifest.extra.json")),
			Message: "preserved unsupported Codex plugin manifest fields under targets/codex-package/manifest.extra.json",
		})
	}
	if strings.TrimSpace(pluginManifest.SkillsPath) != "" && strings.TrimSpace(pluginManifest.SkillsPath) != codexmanifest.SkillsRef {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Message: "normalized Codex plugin skills path to the managed ./skills/ location",
		})
	}
	if strings.TrimSpace(pluginManifest.MCPServersRef) != "" && strings.TrimSpace(pluginManifest.MCPServersRef) != codexmanifest.MCPServersRef {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Message: "normalized Codex plugin mcpServers path to the managed ./.mcp.json location",
		})
	}
	if fileExists(filepath.Join(root, "agents")) {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningIgnoredImport,
			Path:    "agents",
			Message: "ignored unsupported import asset: agents",
		})
	}
	result.Artifacts = compactArtifacts(result.Artifacts)
	return result, nil
}

func (codexRuntimeAdapter) Import(root string, seed ImportSeed) (ImportResult, error) {
	result := ImportResult{
		Manifest: seed.Manifest,
		Launcher: seed.Launcher,
	}
	config, _, err := readImportedCodexConfig(root)
	if err != nil {
		return ImportResult{}, err
	}
	meta := codexRuntimeMeta{}
	if strings.TrimSpace(config.Model) != "" {
		meta.ModelHint = config.Model
	}
	result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
		RelPath: filepath.Join("targets", "codex-runtime", "package.yaml"),
		Content: mustYAML(meta),
	})
	if len(config.Extra) > 0 {
		body, err := toml.Marshal(config.Extra)
		if err != nil {
			return ImportResult{}, err
		}
		result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join("targets", "codex-runtime", "config.extra.toml"),
			Content: body,
		})
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join("targets", "codex-runtime", "config.extra.toml")),
			Message: "preserved unsupported Codex config fields under targets/codex-runtime/config.extra.toml",
		})
	}
	if len(config.Notify) > 0 && result.Launcher != nil && strings.TrimSpace(config.Notify[0]) != "" {
		result.Launcher.Entrypoint = strings.TrimSpace(config.Notify[0])
	}
	if len(config.Notify) > 0 && !pluginmodel.IsCanonicalCodexNotify(config.Notify) {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".codex", "config.toml")),
			Message: "normalized Codex notify argv to the managed [entrypoint, \"notify\"] shape",
		})
	}
	copied, err := copyArtifactDirs(root,
		artifactDir{src: "commands", dst: filepath.Join("targets", "codex-runtime", "commands")},
		artifactDir{src: "contexts", dst: filepath.Join("targets", "codex-runtime", "contexts")},
	)
	if err != nil {
		return ImportResult{}, err
	}
	result.Artifacts = append(result.Artifacts, copied...)
	result.Artifacts = compactArtifacts(result.Artifacts)
	return result, nil
}

func (codexPackageAdapter) Render(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]pluginmodel.Artifact, error) {
	extra, err := loadNativeExtraDoc(root, state, "manifest_extra", pluginmodel.NativeDocFormatJSON)
	if err != nil {
		return nil, err
	}
	meta, _, err := readYAMLDoc[codexPackageMeta](root, state.DocPath("package_metadata"))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	}
	managedPaths := managedKeysForNativeDoc("codex-package", "manifest_extra")
	if err := pluginmodel.ValidateNativeExtraDocConflicts(extra, "codex-package manifest.extra.json", managedPaths); err != nil {
		return nil, err
	}
	doc := map[string]any{
		"name":        graph.Manifest.Name,
		"version":     graph.Manifest.Version,
		"description": graph.Manifest.Description,
	}
	meta.Apply(doc)
	if len(graph.Portable.Paths("skills")) > 0 {
		doc["skills"] = codexmanifest.SkillsRef
	}
	if graph.Portable.MCP != nil {
		doc["mcpServers"] = codexmanifest.MCPServersRef
	}

	var artifacts []pluginmodel.Artifact
	if rel := strings.TrimSpace(state.DocPath("interface")); rel != "" {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		interfaceDoc, err := codexmanifest.ParseInterfaceDoc(body)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		doc["interface"] = interfaceDoc
	}
	if rel := strings.TrimSpace(state.DocPath("app_manifest")); rel != "" {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		appDoc, err := codexmanifest.ParseAppManifestDoc(body)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		if codexmanifest.AppManifestEnabled(appDoc) {
			doc["apps"] = codexmanifest.AppsRef
			artifacts = append(artifacts, pluginmodel.Artifact{
				RelPath: ".app.json",
				Content: body,
			})
		}
	}
	if err := pluginmodel.MergeNativeExtraObject(doc, extra, "codex-package manifest.extra.json", managedPaths); err != nil {
		return nil, err
	}
	pluginJSON, err := marshalJSON(doc)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, pluginmodel.Artifact{
		RelPath: filepath.Join(".codex-plugin", "plugin.json"),
		Content: pluginJSON,
	})
	if graph.Portable.MCP != nil {
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "codex-package")
		if err != nil {
			return nil, err
		}
		mcpJSON, err := marshalJSON(projected)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: ".mcp.json",
			Content: mcpJSON,
		})
	}
	return compactArtifacts(artifacts), nil
}

func (codexRuntimeAdapter) Render(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]pluginmodel.Artifact, error) {
	entrypoint := ""
	if graph.Launcher != nil {
		entrypoint = graph.Launcher.Entrypoint
	}
	if strings.TrimSpace(entrypoint) == "" {
		return nil, fmt.Errorf("required launcher missing: %s", pluginmodel.LauncherFileName)
	}
	model := ""
	if meta, ok, err := readYAMLDoc[codexRuntimeMeta](root, state.DocPath("package_metadata")); err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	} else if ok {
		model = strings.TrimSpace(meta.ModelHint)
	}
	if strings.TrimSpace(model) == "" {
		model = "gpt-5.4-mini"
	}
	configExtra, err := loadNativeExtraDoc(root, state, "config_extra", pluginmodel.NativeDocFormatTOML)
	if err != nil {
		return nil, err
	}
	if err := pluginmodel.ValidateNativeExtraDocConflicts(configExtra, "codex-runtime config.extra.toml", managedKeysForNativeDoc("codex-runtime", "config_extra")); err != nil {
		return nil, err
	}
	var config bytes.Buffer
	config.WriteString("# Generated by plugin-kit-ai. DO NOT EDIT.\n")
	config.WriteString(fmt.Sprintf("model = %q\n", model))
	config.WriteString(fmt.Sprintf("notify = [%q, %q]\n", entrypoint, "notify"))
	if extraBody := pluginmodel.TrimmedExtraBody(configExtra); len(extraBody) > 0 {
		config.WriteByte('\n')
		config.Write(extraBody)
		config.WriteByte('\n')
	}
	artifacts := []pluginmodel.Artifact{{
		RelPath: filepath.Join(".codex", "config.toml"),
		Content: config.Bytes(),
	}}
	copied, err := copyArtifactDirs(root,
		artifactDir{src: filepath.Join("targets", "codex-runtime", "commands"), dst: "commands"},
		artifactDir{src: filepath.Join("targets", "codex-runtime", "contexts"), dst: "contexts"},
	)
	if err != nil {
		return nil, err
	}
	return append(artifacts, copied...), nil
}

func (codexPackageAdapter) ManagedPaths(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]string, error) {
	return nil, nil
}

func (codexRuntimeAdapter) ManagedPaths(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]string, error) {
	return nil, nil
}

func (codexPackageAdapter) Validate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]Diagnostic, error) {
	body, err := os.ReadFile(filepath.Join(root, ".codex-plugin", "plugin.json"))
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest %s is not readable: %v", filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")), err),
		}}, nil
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest %s is invalid JSON: %v", filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")), err),
		}}, nil
	}
	pluginManifest, err := codexmanifest.DecodeImportedPluginManifest(body)
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest %s is invalid: %v", filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")), err),
		}}, nil
	}
	var diagnostics []Diagnostic
	if strings.TrimSpace(pluginManifest.Name) != strings.TrimSpace(graph.Manifest.Name) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json sets name %q; expected %q from plugin.yaml", strings.TrimSpace(pluginManifest.Name), strings.TrimSpace(graph.Manifest.Name)),
		})
	}
	if strings.TrimSpace(pluginManifest.Version) != strings.TrimSpace(graph.Manifest.Version) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json sets version %q; expected %q from plugin.yaml", strings.TrimSpace(pluginManifest.Version), strings.TrimSpace(graph.Manifest.Version)),
		})
	}
	if strings.TrimSpace(pluginManifest.Description) != strings.TrimSpace(graph.Manifest.Description) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json sets description %q; expected %q from plugin.yaml", strings.TrimSpace(pluginManifest.Description), strings.TrimSpace(graph.Manifest.Description)),
		})
	}
	if meta, ok, err := readYAMLDoc[codexPackageMeta](root, state.DocPath("package_metadata")); err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	} else if ok {
		meta.Normalize()
		if !codexPackageMetaEqual(meta, pluginManifest.PackageMeta) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
				Target:   "codex-package",
				Message:  "Codex plugin manifest .codex-plugin/plugin.json package metadata does not match targets/codex-package/package.yaml",
			})
		}
	}
	if hasSkills := len(graph.Portable.Paths("skills")) > 0; hasSkills {
		if strings.TrimSpace(pluginManifest.SkillsPath) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
				Target:   "codex-package",
				Message:  "Codex plugin manifest .codex-plugin/plugin.json must reference ./skills/ when portable skills are authored",
			})
		}
	} else if strings.TrimSpace(pluginManifest.SkillsPath) != "" {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Target:   "codex-package",
			Message:  "Codex plugin manifest .codex-plugin/plugin.json may not reference skills when no portable skills are authored",
		})
	}
	if ref := strings.TrimSpace(pluginManifest.SkillsPath); ref != "" && ref != codexmanifest.SkillsRef {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json must use %q for skills when present", codexmanifest.SkillsRef),
		})
	}
	if hasMCP := graph.Portable.MCP != nil; hasMCP {
		if strings.TrimSpace(pluginManifest.MCPServersRef) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json must reference %q when portable MCP is authored", codexmanifest.MCPServersRef),
			})
		}
	} else if strings.TrimSpace(pluginManifest.MCPServersRef) != "" {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Target:   "codex-package",
			Message:  "Codex plugin manifest .codex-plugin/plugin.json may not reference mcpServers when no portable MCP is authored",
		})
	}
	if ref := strings.TrimSpace(pluginManifest.MCPServersRef); ref != "" {
		if ref != codexmanifest.MCPServersRef {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json must use %q for mcpServers when present", codexmanifest.MCPServersRef),
			})
		}
		if mcpBody, err := os.ReadFile(filepath.Join(root, cleanRelativeRef(ref))); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     filepath.ToSlash(cleanRelativeRef(ref)),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex MCP manifest %s is not readable: %v", filepath.ToSlash(cleanRelativeRef(ref)), err),
			})
		} else if renderedMCP, err := decodeJSONObject(mcpBody, fmt.Sprintf("Codex MCP manifest %s", filepath.ToSlash(cleanRelativeRef(ref)))); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(cleanRelativeRef(ref)),
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
					Path:     filepath.ToSlash(cleanRelativeRef(ref)),
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
				Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
				Target:   "codex-package",
				Message:  "Codex plugin manifest .codex-plugin/plugin.json interface does not match targets/codex-package/interface.json",
			})
		}
	} else if pluginManifest.Interface != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
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
			Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Target:   "codex-package",
			Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json must reference %q when targets/codex-package/app.json is enabled", codexmanifest.AppsRef),
		})
	}
	if !authoredAppEnabled && strings.TrimSpace(pluginManifest.AppsRef) != "" {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Target:   "codex-package",
			Message:  "Codex plugin manifest .codex-plugin/plugin.json may not reference apps when targets/codex-package/app.json is empty or absent",
		})
	}
	if ref := strings.TrimSpace(pluginManifest.AppsRef); ref != "" {
		if ref != codexmanifest.AppsRef {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex plugin manifest .codex-plugin/plugin.json must use %q for apps when present", codexmanifest.AppsRef),
			})
		}
		if body, err := os.ReadFile(filepath.Join(root, cleanRelativeRef(ref))); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     filepath.ToSlash(cleanRelativeRef(ref)),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex app manifest %s is not readable: %v", filepath.ToSlash(cleanRelativeRef(ref)), err),
			})
		} else if renderedAppDoc, err := codexmanifest.ParseAppManifestDoc(body); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(cleanRelativeRef(ref)),
				Target:   "codex-package",
				Message:  fmt.Sprintf("Codex app manifest %s is invalid: %v", filepath.ToSlash(cleanRelativeRef(ref)), err),
			})
		} else if authoredAppEnabled && !jsonDocumentsEqual(authoredAppDoc, renderedAppDoc) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     filepath.ToSlash(cleanRelativeRef(ref)),
				Target:   "codex-package",
				Message:  "Codex app manifest .app.json does not match targets/codex-package/app.json",
			})
		}
	}
	return diagnostics, nil
}

func codexPackageMetaEqual(left, right codexPackageMeta) bool {
	left.Normalize()
	right.Normalize()
	if (left.Author == nil) != (right.Author == nil) {
		return false
	}
	if left.Author != nil && right.Author != nil {
		if left.Author.Name != right.Author.Name || left.Author.Email != right.Author.Email || left.Author.URL != right.Author.URL {
			return false
		}
	}
	return left.Homepage == right.Homepage &&
		left.Repository == right.Repository &&
		left.License == right.License &&
		slices.Equal(left.Keywords, right.Keywords)
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

func cloneStringMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	body, _ := json.Marshal(values)
	out := map[string]any{}
	_ = json.Unmarshal(body, &out)
	return out
}
