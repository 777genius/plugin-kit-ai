package platformexec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		if ref != codexmanifest.AppsRef || pluginManifest.LegacyAppsRef {
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
				Message:  fmt.Sprintf("Codex app manifest %s is invalid JSON: %v", filepath.ToSlash(rel), err),
			}}, nil
		}
		if codexmanifest.AppManifestEnabled(appDoc) {
			if body, err := os.ReadFile(filepath.Join(root, ".app.json")); err != nil {
				return []Diagnostic{{
					Severity: SeverityFailure,
					Code:     CodeGeneratedContractInvalid,
					Path:     ".app.json",
					Target:   "codex-package",
					Message:  fmt.Sprintf("Codex app manifest %s is not readable: %v", ".app.json", err),
				}}, nil
			} else {
				if _, err := codexmanifest.ParseAppManifestDoc(body); err != nil {
					return []Diagnostic{{
						Severity: SeverityFailure,
						Code:     CodeManifestInvalid,
						Path:     ".app.json",
						Target:   "codex-package",
						Message:  fmt.Sprintf("Codex app manifest %s is invalid JSON: %v", ".app.json", err),
					}}, nil
				}
			}
		}
	}
	return nil, nil
}

func (codexRuntimeAdapter) Validate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]Diagnostic, error) {
	body, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     filepath.ToSlash(filepath.Join(".codex", "config.toml")),
			Target:   "codex-runtime",
			Message:  fmt.Sprintf("Codex config file %s is not readable: %v", filepath.ToSlash(filepath.Join(".codex", "config.toml")), err),
		}}, nil
	}
	var config struct {
		Model  string   `toml:"model"`
		Notify []string `toml:"notify"`
	}
	if err := toml.Unmarshal(body, &config); err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join(".codex", "config.toml")),
			Target:   "codex-runtime",
			Message:  fmt.Sprintf("Codex config file %s is invalid TOML: %v", filepath.ToSlash(filepath.Join(".codex", "config.toml")), err),
		}}, nil
	}
	if graph.Launcher == nil {
		return nil, nil
	}
	var diagnostics []Diagnostic
	expectedNotify := []string{graph.Launcher.Entrypoint, "notify"}
	if len(config.Notify) != len(expectedNotify) || len(config.Notify) == 0 || strings.TrimSpace(config.Notify[0]) != expectedNotify[0] || (len(config.Notify) > 1 && strings.TrimSpace(config.Notify[1]) != expectedNotify[1]) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeEntrypointMismatch,
			Path:     filepath.ToSlash(filepath.Join(".codex", "config.toml")),
			Target:   "codex-runtime",
			Message:  fmt.Sprintf("entrypoint mismatch: Codex notify argv uses %q; expected %q from launcher.yaml entrypoint", config.Notify, expectedNotify),
		})
	}
	if meta, ok, err := readYAMLDoc[codexRuntimeMeta](root, state.DocPath("package_metadata")); err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	} else if ok && strings.TrimSpace(meta.ModelHint) != "" && strings.TrimSpace(config.Model) != strings.TrimSpace(meta.ModelHint) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     filepath.ToSlash(filepath.Join(".codex", "config.toml")),
			Target:   "codex-runtime",
			Message:  fmt.Sprintf("Codex config model %q does not match targets/codex-runtime/package.yaml model_hint %q", strings.TrimSpace(config.Model), strings.TrimSpace(meta.ModelHint)),
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
