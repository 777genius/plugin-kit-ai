package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func (claudeAdapter) Import(root string, seed ImportSeed) (ImportResult, error) {
	result := ImportResult{
		Manifest: seed.Manifest,
		Launcher: seed.Launcher,
	}
	pluginManifest, _, manifestPresent, err := readImportedClaudePluginManifest(root)
	if err != nil {
		return ImportResult{}, err
	}
	if manifestPresent {
		if strings.TrimSpace(pluginManifest.Name) != "" {
			result.Manifest.Name = pluginManifest.Name
		}
		if strings.TrimSpace(pluginManifest.Version) != "" {
			result.Manifest.Version = pluginManifest.Version
		}
		if strings.TrimSpace(pluginManifest.Description) != "" {
			result.Manifest.Description = pluginManifest.Description
		}
	} else {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    ".claude-plugin/plugin.json",
			Message: "native Claude plugin imported without manifest; package-standard defaults were derived from the directory name",
		})
	}
	for _, warning := range pluginManifest.Warnings {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: warning,
		})
	}
	if strings.TrimSpace(pluginManifest.Name) != "" && pluginManifest.Name != seed.Manifest.Name {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: "normalized Claude plugin identity into canonical package-standard plugin.yaml",
		})
	}
	if copied, warnings, err := importClaudePortableSkills(root, pluginManifest); err != nil {
		return ImportResult{}, err
	} else {
		result.Artifacts = append(result.Artifacts, copied...)
		result.Warnings = append(result.Warnings, warnings...)
	}

	if hookArtifacts, hookBody, warnings, err := importClaudeHooks(root, pluginManifest); err != nil {
		return ImportResult{}, err
	} else {
		result.Warnings = append(result.Warnings, warnings...)
		if len(hookBody) > 0 {
			if entrypoint, ok := inferClaudeEntrypoint(hookBody); ok && result.Launcher == nil {
				result.Launcher = &pluginmodel.Launcher{
					Runtime:    "go",
					Entrypoint: entrypoint,
				}
			} else if ok {
				result.Launcher.Entrypoint = entrypoint
			} else {
				result.DroppedKinds = append(result.DroppedKinds, "hooks")
				result.Warnings = append(result.Warnings, pluginmodel.Warning{
					Kind:    pluginmodel.WarningFidelity,
					Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
					Message: "Claude hooks were omitted from canonical package-standard import because their commands do not map to launcher.yaml entrypoint semantics",
				})
				hookArtifacts = nil
			}
		}
		result.Artifacts = append(result.Artifacts, hookArtifacts...)
	}

	if copied, warnings, err := importClaudeComponentRefs(root, "commands", filepath.Join(pluginmodel.SourceDirName, "targets", "claude", "commands"), pluginManifest.CommandsOverride, pluginManifest.CommandsRefs); err != nil {
		return ImportResult{}, err
	} else {
		result.Artifacts = append(result.Artifacts, copied...)
		result.Warnings = append(result.Warnings, warnings...)
	}
	if copied, warnings, err := importClaudeComponentRefs(root, "agents", filepath.Join(pluginmodel.SourceDirName, "targets", "claude", "agents"), pluginManifest.AgentsOverride, pluginManifest.AgentsRefs); err != nil {
		return ImportResult{}, err
	} else {
		result.Artifacts = append(result.Artifacts, copied...)
		result.Warnings = append(result.Warnings, warnings...)
	}

	if copied, warning, err := importClaudeStructuredDoc(root, "settings.json", filepath.Join(pluginmodel.SourceDirName, "targets", "claude", "settings.json"), manifestPresent && pluginManifest.SettingsProvided, pluginManifest.Settings, "Claude manifest settings"); err != nil {
		return ImportResult{}, err
	} else {
		result.Artifacts = append(result.Artifacts, copied...)
		if strings.TrimSpace(warning) != "" {
			result.Warnings = append(result.Warnings, pluginmodel.Warning{Kind: pluginmodel.WarningFidelity, Path: "settings.json", Message: warning})
		}
	}
	if copied, warnings, err := importClaudeLSP(root, pluginManifest); err != nil {
		return ImportResult{}, err
	} else {
		result.Artifacts = append(result.Artifacts, copied...)
		result.Warnings = append(result.Warnings, warnings...)
	}
	if pluginManifest.UserConfigProvided {
		result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "claude", "user-config.json"),
			Content: mustJSON(pluginManifest.UserConfig),
		})
	}
	if copied, warnings, err := importClaudeMCP(root, pluginManifest); err != nil {
		return ImportResult{}, err
	} else {
		result.Artifacts = append(result.Artifacts, copied...)
		result.Warnings = append(result.Warnings, warnings...)
	}
	if len(pluginManifest.Extra) > 0 {
		result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "claude", "manifest.extra.json"),
			Content: mustJSON(pluginManifest.Extra),
		})
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, "targets", "claude", "manifest.extra.json")),
			Message: "preserved unsupported Claude manifest fields under targets/claude/manifest.extra.json",
		})
	}
	result.Artifacts = compactArtifacts(result.Artifacts)
	return result, nil
}

func importClaudeComponentRefs(root, kind, dstRoot string, overridden bool, refs []string) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	if !overridden {
		if !fileExists(filepath.Join(root, kind)) {
			return nil, nil, nil
		}
		refs = []string{kind}
	}
	if len(refs) == 0 {
		return nil, nil, nil
	}
	artifacts, err := copyArtifactsFromRefs(root, refs, dstRoot)
	if err != nil {
		return nil, nil, err
	}
	if !overridden {
		return artifacts, nil, nil
	}
	return artifacts, []pluginmodel.Warning{{
		Kind:    pluginmodel.WarningFidelity,
		Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
		Message: fmt.Sprintf("custom Claude %s paths were normalized into canonical package-standard layout", kind),
	}}, nil
}

func importClaudePortableSkills(root string, manifest importedClaudePluginManifest) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	if !manifest.SkillsOverride {
		if !fileExists(filepath.Join(root, "skills")) {
			return nil, nil, nil
		}
		artifacts, err := copyArtifactsFromRefs(root, []string{"skills"}, "skills")
		if err != nil {
			return nil, nil, err
		}
		return artifacts, nil, nil
	}
	if len(manifest.SkillsRefs) == 0 {
		return nil, nil, nil
	}
	artifacts, err := copyArtifactsFromRefs(root, manifest.SkillsRefs, "skills")
	if err != nil {
		return nil, nil, err
	}
	return artifacts, []pluginmodel.Warning{{
		Kind:    pluginmodel.WarningFidelity,
		Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
		Message: "custom Claude skills paths were normalized into canonical package-standard layout",
	}}, nil
}

func importClaudeHooks(root string, manifest importedClaudePluginManifest) ([]pluginmodel.Artifact, []byte, []pluginmodel.Warning, error) {
	const dst = "targets/claude/hooks/hooks.json"
	if manifest.HooksOverride {
		switch {
		case manifest.InlineHooks != nil:
			body := mustJSON(manifest.InlineHooks)
			return []pluginmodel.Artifact{{RelPath: dst, Content: body}}, body, []pluginmodel.Warning{{
				Kind:    pluginmodel.WarningFidelity,
				Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
				Message: "custom Claude hooks were normalized into targets/claude/hooks/hooks.json",
			}}, nil
		case len(manifest.HookRefs) == 1:
			ref := cleanRelativeRef(manifest.HookRefs[0])
			body, err := os.ReadFile(filepath.Join(root, ref))
			if err != nil {
				return nil, nil, nil, err
			}
			return []pluginmodel.Artifact{{RelPath: dst, Content: body}}, body, []pluginmodel.Warning{{
				Kind:    pluginmodel.WarningFidelity,
				Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
				Message: "custom Claude hooks path was normalized into targets/claude/hooks/hooks.json",
			}}, nil
		case len(manifest.HookRefs) > 1:
			body, err := mergeClaudeHookRefs(root, manifest.HookRefs)
			if err != nil {
				return nil, nil, nil, err
			}
			return []pluginmodel.Artifact{{RelPath: dst, Content: body}}, body, []pluginmodel.Warning{{
				Kind:    pluginmodel.WarningFidelity,
				Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
				Message: "custom Claude hooks path array was normalized into canonical package-standard layout",
			}}, nil
		default:
			return nil, nil, nil, nil
		}
	}
	body, err := os.ReadFile(filepath.Join(root, "hooks", "hooks.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, err
	}
	return []pluginmodel.Artifact{{RelPath: dst, Content: body}}, body, nil, nil
}

func importClaudeStructuredDoc(root, rootPath, targetPath string, inlineProvided bool, inline map[string]any, inlineLabel string) ([]pluginmodel.Artifact, string, error) {
	if body, err := os.ReadFile(filepath.Join(root, rootPath)); err == nil {
		return []pluginmodel.Artifact{{RelPath: targetPath, Content: body}}, "", nil
	} else if !os.IsNotExist(err) {
		return nil, "", err
	}
	if inlineProvided {
		return []pluginmodel.Artifact{{RelPath: targetPath, Content: mustJSON(inline)}}, fmt.Sprintf("%s was normalized into %s", inlineLabel, filepath.ToSlash(targetPath)), nil
	}
	return nil, "", nil
}

func importClaudeLSP(root string, manifest importedClaudePluginManifest) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	const targetPath = "targets/claude/lsp.json"
	if fileExists(filepath.Join(root, ".lsp.json")) {
		body, err := os.ReadFile(filepath.Join(root, ".lsp.json"))
		if err != nil {
			return nil, nil, err
		}
		return []pluginmodel.Artifact{{RelPath: targetPath, Content: body}}, nil, nil
	}
	if !manifest.LSPOverride {
		return nil, nil, nil
	}
	switch {
	case manifest.InlineLSP != nil:
		return []pluginmodel.Artifact{{RelPath: targetPath, Content: mustJSON(manifest.InlineLSP)}}, []pluginmodel.Warning{{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: "inline Claude lspServers were normalized into targets/claude/lsp.json",
		}}, nil
	case len(manifest.LSPRefs) == 1:
		ref := cleanRelativeRef(manifest.LSPRefs[0])
		body, err := os.ReadFile(filepath.Join(root, ref))
		if err != nil {
			return nil, nil, err
		}
		return []pluginmodel.Artifact{{RelPath: targetPath, Content: body}}, []pluginmodel.Warning{{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: "custom Claude lspServers path was normalized into targets/claude/lsp.json",
		}}, nil
	case len(manifest.LSPRefs) > 1:
		body, err := mergeClaudeObjectRefs(root, manifest.LSPRefs, "Claude lspServers")
		if err != nil {
			return nil, nil, err
		}
		return []pluginmodel.Artifact{{RelPath: targetPath, Content: body}}, []pluginmodel.Warning{{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: "custom Claude lspServers path array was normalized into canonical package-standard layout",
		}}, nil
	default:
		return nil, nil, nil
	}
}

func importClaudeMCP(root string, manifest importedClaudePluginManifest) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	if fileExists(filepath.Join(root, ".mcp.json")) || !manifest.MCPOverride {
		return nil, nil, nil
	}
	switch {
	case manifest.InlineMCP != nil:
		artifact, err := importedPortableMCPArtifact("claude", manifest.InlineMCP)
		if err != nil {
			return nil, nil, err
		}
		return []pluginmodel.Artifact{artifact}, []pluginmodel.Warning{{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: "inline Claude mcpServers were normalized into src/mcp/servers.yaml",
		}}, nil
	case len(manifest.MCPRefs) == 1:
		ref := cleanRelativeRef(manifest.MCPRefs[0])
		body, err := os.ReadFile(filepath.Join(root, ref))
		if err != nil {
			return nil, nil, err
		}
		doc, err := decodeJSONObject(body, "Claude mcpServers")
		if err != nil {
			return nil, nil, err
		}
		artifact, err := importedPortableMCPArtifact("claude", doc)
		if err != nil {
			return nil, nil, err
		}
		return []pluginmodel.Artifact{artifact}, []pluginmodel.Warning{{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: "custom Claude mcpServers path was normalized into src/mcp/servers.yaml",
		}}, nil
	case len(manifest.MCPRefs) > 1:
		body, err := mergeClaudeObjectRefs(root, manifest.MCPRefs, "Claude mcpServers")
		if err != nil {
			return nil, nil, err
		}
		doc, err := decodeJSONObject(body, "Claude mcpServers")
		if err != nil {
			return nil, nil, err
		}
		artifact, err := importedPortableMCPArtifact("claude", doc)
		if err != nil {
			return nil, nil, err
		}
		return []pluginmodel.Artifact{artifact}, []pluginmodel.Warning{{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: "custom Claude mcpServers path array was normalized into canonical package-standard layout",
		}}, nil
	default:
		return nil, nil, nil
	}
}

func mergeClaudeHookRefs(root string, refs []string) ([]byte, error) {
	merged := map[string]any{}
	for _, ref := range refs {
		ref = cleanRelativeRef(ref)
		body, err := os.ReadFile(filepath.Join(root, ref))
		if err != nil {
			return nil, err
		}
		doc, err := decodeJSONObject(body, fmt.Sprintf("Claude hooks file %s", ref))
		if err != nil {
			return nil, err
		}
		value, ok := doc["hooks"]
		if !ok {
			return nil, fmt.Errorf("claude hooks file %s is incompatible with package-standard normalization: top-level \"hooks\" object required", ref)
		}
		hooksMap, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("claude hooks file %s is incompatible with package-standard normalization: top-level \"hooks\" must be a JSON object", ref)
		}
		if err := mergeClaudeHookTree(merged, hooksMap, ref, "hooks"); err != nil {
			return nil, err
		}
	}
	return marshalJSON(map[string]any{"hooks": merged})
}

func mergeClaudeHookTree(dst, src map[string]any, ref, path string) error {
	for key, srcValue := range src {
		nextPath := key
		if strings.TrimSpace(path) != "" {
			nextPath = path + "." + key
		}
		dstValue, exists := dst[key]
		if !exists {
			dst[key] = srcValue
			continue
		}
		switch typed := srcValue.(type) {
		case []any:
			dstSlice, ok := dstValue.([]any)
			if !ok {
				return fmt.Errorf("claude hooks file %s is incompatible with package-standard normalization: %s mixes array and non-array shapes", ref, nextPath)
			}
			dst[key] = append(dstSlice, typed...)
		case map[string]any:
			dstMap, ok := dstValue.(map[string]any)
			if !ok {
				return fmt.Errorf("claude hooks file %s is incompatible with package-standard normalization: %s mixes object and non-object shapes", ref, nextPath)
			}
			if err := mergeClaudeHookTree(dstMap, typed, ref, nextPath); err != nil {
				return err
			}
		default:
			if !reflect.DeepEqual(dstValue, srcValue) {
				return fmt.Errorf("claude hooks file %s is incompatible with package-standard normalization: %s has conflicting scalar values", ref, nextPath)
			}
		}
	}
	return nil
}

func mergeClaudeObjectRefs(root string, refs []string, label string) ([]byte, error) {
	merged := map[string]any{}
	for _, ref := range refs {
		ref = cleanRelativeRef(ref)
		body, err := os.ReadFile(filepath.Join(root, ref))
		if err != nil {
			return nil, err
		}
		doc, err := decodeJSONObject(body, fmt.Sprintf("%s file %s", label, ref))
		if err != nil {
			return nil, err
		}
		for key, value := range doc {
			if existing, ok := merged[key]; ok {
				if !reflect.DeepEqual(existing, value) {
					return nil, fmt.Errorf("%s path array cannot be normalized safely: duplicate key %q conflicts in %s", label, key, ref)
				}
				continue
			}
			merged[key] = value
		}
	}
	return marshalJSON(merged)
}
