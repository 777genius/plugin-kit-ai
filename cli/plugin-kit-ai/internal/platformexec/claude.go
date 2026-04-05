package platformexec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

type claudeAdapter struct{}

func (claudeAdapter) ID() string { return "claude" }

func (claudeAdapter) DetectNative(root string) bool {
	return fileExists(filepath.Join(root, ".claude-plugin", "plugin.json")) ||
		fileExists(filepath.Join(root, "hooks", "hooks.json")) ||
		fileExists(filepath.Join(root, "settings.json")) ||
		fileExists(filepath.Join(root, ".lsp.json")) ||
		fileExists(filepath.Join(root, "commands")) ||
		fileExists(filepath.Join(root, "agents"))
}

func (claudeAdapter) RefineDiscovery(root string, state *pluginmodel.TargetState) error {
	if rel := state.DocPath("package_metadata"); strings.TrimSpace(rel) != "" {
		if _, ok, err := readYAMLDoc[claudePackageMeta](root, rel); err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		} else if !ok {
			return nil
		}
	}
	for _, doc := range []struct {
		kind  string
		label string
	}{
		{kind: "settings", label: "Claude settings"},
		{kind: "lsp", label: "Claude LSP"},
		{kind: "user_config", label: "Claude userConfig"},
	} {
		if _, _, _, err := loadClaudeJSONDoc(root, state.DocPath(doc.kind), doc.label); err != nil {
			return err
		}
	}
	return nil
}

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

	if hookArtifacts, hookBody, warnings, err := importClaudeHooks(root, pluginManifest); err != nil {
		return ImportResult{}, err
	} else {
		result.Artifacts = append(result.Artifacts, hookArtifacts...)
		result.Warnings = append(result.Warnings, warnings...)
		if len(hookBody) > 0 {
			if entrypoint, ok := inferClaudeEntrypoint(hookBody); ok && result.Launcher == nil {
				result.Launcher = &pluginmodel.Launcher{
					Runtime:    "go",
					Entrypoint: entrypoint,
				}
			} else if ok {
				result.Launcher.Entrypoint = entrypoint
			}
		}
	}

	if copied, warnings, err := importClaudeComponentRefs(root, "commands", filepath.Join("targets", "claude", "commands"), pluginManifest.CommandsOverride, pluginManifest.CommandsRefs); err != nil {
		return ImportResult{}, err
	} else {
		result.Artifacts = append(result.Artifacts, copied...)
		result.Warnings = append(result.Warnings, warnings...)
	}
	if copied, warnings, err := importClaudeComponentRefs(root, "agents", filepath.Join("targets", "claude", "agents"), pluginManifest.AgentsOverride, pluginManifest.AgentsRefs); err != nil {
		return ImportResult{}, err
	} else {
		result.Artifacts = append(result.Artifacts, copied...)
		result.Warnings = append(result.Warnings, warnings...)
	}

	if copied, warning, err := importClaudeStructuredDoc(root, "settings.json", filepath.Join("targets", "claude", "settings.json"), manifestPresent && pluginManifest.SettingsProvided, pluginManifest.Settings, "Claude manifest settings"); err != nil {
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
			RelPath: filepath.Join("targets", "claude", "user-config.json"),
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
			RelPath: filepath.Join("targets", "claude", "manifest.extra.json"),
			Content: mustJSON(pluginManifest.Extra),
		})
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join("targets", "claude", "manifest.extra.json")),
			Message: "preserved unsupported Claude manifest fields under targets/claude/manifest.extra.json",
		})
	}
	result.Artifacts = compactArtifacts(result.Artifacts)
	return result, nil
}

func (claudeAdapter) Generate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]pluginmodel.Artifact, error) {
	entrypoint := ""
	if graph.Launcher != nil {
		entrypoint = graph.Launcher.Entrypoint
	}
	if claudeHooksRequireLauncher(graph, state) && strings.TrimSpace(entrypoint) == "" {
		return nil, fmt.Errorf("required launcher missing: %s", pluginmodel.LauncherFileName)
	}
	if graph.Launcher == nil && !claudePackageOnlyMode(graph, state) {
		return nil, fmt.Errorf("invalid %s: target claude without launcher.yaml must author at least one package-only surface such as mcp/servers.yaml, skills/, targets/claude/settings.json, targets/claude/lsp.json, targets/claude/user-config.json, targets/claude/manifest.extra.json, targets/claude/commands/**, or targets/claude/agents/**", pluginmodel.FileName)
	}
	_, settingsBody, settingsPresent, err := loadClaudeJSONDoc(root, state.DocPath("settings"), "Claude settings")
	if err != nil {
		return nil, err
	}
	_, lspBody, lspPresent, err := loadClaudeJSONDoc(root, state.DocPath("lsp"), "Claude LSP")
	if err != nil {
		return nil, err
	}
	userConfig, _, userConfigPresent, err := loadClaudeJSONDoc(root, state.DocPath("user_config"), "Claude userConfig")
	if err != nil {
		return nil, err
	}
	extra, err := loadNativeExtraDoc(root, state, "manifest_extra", pluginmodel.NativeDocFormatJSON)
	if err != nil {
		return nil, err
	}
	doc := map[string]any{
		"name":        graph.Manifest.Name,
		"version":     graph.Manifest.Version,
		"description": graph.Manifest.Description,
	}
	if len(graph.Portable.Paths("skills")) > 0 {
		doc["skills"] = "./skills/"
	}
	if len(state.ComponentPaths("agents")) > 0 {
		doc["agents"] = "./agents/"
	}
	if graph.Portable.MCP != nil {
		doc["mcpServers"] = "./.mcp.json"
	}
	if userConfigPresent {
		doc["userConfig"] = userConfig
	}
	if err := pluginmodel.MergeNativeExtraObject(doc, extra, "claude manifest.extra.json", claudeManifestManagedPaths()); err != nil {
		return nil, err
	}
	pluginJSON, err := marshalJSON(doc)
	if err != nil {
		return nil, err
	}
	artifacts := []pluginmodel.Artifact{{
		RelPath: filepath.Join(".claude-plugin", "plugin.json"),
		Content: pluginJSON,
	}}
	if graph.Portable.MCP != nil {
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "claude")
		if err != nil {
			return nil, err
		}
		mcpJSON, err := marshalJSON(projected)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{RelPath: ".mcp.json", Content: mcpJSON})
	}
	if settingsPresent {
		artifacts = append(artifacts, pluginmodel.Artifact{RelPath: "settings.json", Content: settingsBody})
	}
	if lspPresent {
		artifacts = append(artifacts, pluginmodel.Artifact{RelPath: ".lsp.json", Content: lspBody})
	}
	if hookPaths := state.ComponentPaths("hooks"); len(hookPaths) > 0 {
		copied, err := copyArtifacts(root, authoredComponentDir(state, "hooks", filepath.Join("targets", "claude", "hooks")), "hooks")
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, copied...)
	} else if claudeUsesGeneratedHooks(graph, state) {
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join("hooks", "hooks.json"),
			Content: defaultClaudeHooks(entrypoint),
		})
	}
	copiedKinds := []artifactDir{
		{src: authoredComponentDir(state, "agents", filepath.Join("targets", "claude", "agents")), dst: "agents"},
		{src: authoredComponentDir(state, "commands", filepath.Join("targets", "claude", "commands")), dst: "commands"},
	}
	copied, err := copyArtifactDirs(root, copiedKinds...)
	if err != nil {
		return nil, err
	}
	return append(artifacts, copied...), nil
}

func (claudeAdapter) ManagedPaths(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]string, error) {
	if claudeUsesGeneratedHooks(graph, state) || fileExists(filepath.Join(root, "hooks", "hooks.json")) {
		return []string{filepath.ToSlash(filepath.Join("hooks", "hooks.json"))}, nil
	}
	return nil, nil
}

func (claudeAdapter) Validate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]Diagnostic, error) {
	var diagnostics []Diagnostic
	if graph.Launcher == nil {
		if rel := claudePrimaryHookPath(state); rel != "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "claude",
				Message:  fmt.Sprintf("Claude hooks require %s when targets/claude/hooks/** is authored", pluginmodel.LauncherFileName),
			})
		} else if !claudeHasPackageOnlySurface(graph, state) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     pluginmodel.FileName,
				Target:   "claude",
				Message:  "target claude without launcher.yaml must author at least one package-only surface such as mcp/servers.yaml, skills/, targets/claude/settings.json, targets/claude/lsp.json, targets/claude/user-config.json, targets/claude/manifest.extra.json, targets/claude/commands/**, or targets/claude/agents/**",
			})
		}
	}
	if graph.Launcher != nil {
		for _, rel := range state.ComponentPaths("hooks") {
			full := filepath.Join(root, rel)
			body, err := os.ReadFile(full)
			if err != nil {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "claude",
					Message:  fmt.Sprintf("Claude hooks file %s is not readable: %v", rel, err),
				})
				continue
			}
			mismatches, err := validateClaudeHookEntrypoints(body, graph.Launcher.Entrypoint)
			if err != nil {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "claude",
					Message:  fmt.Sprintf("Claude hooks file %s is invalid JSON: %v", rel, err),
				})
				continue
			}
			for _, mismatch := range mismatches {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeEntrypointMismatch,
					Path:     rel,
					Target:   "claude",
					Message:  mismatch,
				})
			}
		}
	}
	diagnostics = append(diagnostics, validateClaudeSettings(root, state.DocPath("settings"))...)
	diagnostics = append(diagnostics, validateClaudeLSP(root, state.DocPath("lsp"))...)
	diagnostics = append(diagnostics, validateClaudeUserConfig(root, state.DocPath("user_config"))...)
	return diagnostics, nil
}

func claudeUsesGeneratedHooks(graph pluginmodel.PackageGraph, state pluginmodel.TargetState) bool {
	if graph.Launcher == nil || strings.TrimSpace(graph.Launcher.Entrypoint) == "" {
		return false
	}
	return len(state.ComponentPaths("hooks")) == 0
}

func claudeHooksRequireLauncher(graph pluginmodel.PackageGraph, state pluginmodel.TargetState) bool {
	if len(state.ComponentPaths("hooks")) > 0 {
		return true
	}
	return graph.Launcher != nil
}

func claudePackageOnlyMode(graph pluginmodel.PackageGraph, state pluginmodel.TargetState) bool {
	return graph.Launcher == nil && len(state.ComponentPaths("hooks")) == 0 && claudeHasPackageOnlySurface(graph, state)
}

func claudeHasPackageOnlySurface(graph pluginmodel.PackageGraph, state pluginmodel.TargetState) bool {
	if len(graph.Portable.Paths("skills")) > 0 || graph.Portable.MCP != nil {
		return true
	}
	for _, kind := range []string{"settings", "lsp", "user_config", "manifest_extra"} {
		if strings.TrimSpace(state.DocPath(kind)) != "" {
			return true
		}
	}
	for _, kind := range []string{"commands", "agents"} {
		if len(state.ComponentPaths(kind)) > 0 {
			return true
		}
	}
	return false
}

func claudePrimaryHookPath(state pluginmodel.TargetState) string {
	hookPaths := state.ComponentPaths("hooks")
	if len(hookPaths) == 0 {
		return ""
	}
	return hookPaths[0]
}

func claudeManifestManagedPaths() []string {
	return []string{
		"name",
		"version",
		"description",
		"skills",
		"agents",
		"commands",
		"hooks",
		"mcpServers",
		"lspServers",
		"settings",
		"userConfig",
	}
}

func loadClaudeJSONDoc(root, rel, label string) (map[string]any, []byte, bool, error) {
	if strings.TrimSpace(rel) == "" {
		return nil, nil, false, nil
	}
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, nil, false, fmt.Errorf("%s %s is not readable: %w", label, rel, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, nil, true, fmt.Errorf("%s %s is invalid JSON: %w", label, rel, err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, body, true, nil
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
			Message: "inline Claude mcpServers were normalized into mcp/servers.yaml",
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
			Message: "custom Claude mcpServers path was normalized into mcp/servers.yaml",
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

func validateClaudeSettings(root, rel string) []Diagnostic {
	doc, _, ok, err := loadClaudeJSONDoc(root, rel, "Claude settings")
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "claude",
			Message:  err.Error(),
		}}
	}
	if !ok {
		return nil
	}
	if value, exists := doc["agent"]; exists {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "claude",
				Message:  fmt.Sprintf(`Claude settings file %s must set "agent" as a non-empty string when present`, rel),
			}}
		}
	}
	return nil
}

func validateClaudeLSP(root, rel string) []Diagnostic {
	_, _, ok, err := loadClaudeJSONDoc(root, rel, "Claude LSP")
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "claude",
			Message:  err.Error(),
		}}
	}
	if !ok {
		return nil
	}
	return nil
}

func validateClaudeUserConfig(root, rel string) []Diagnostic {
	doc, _, ok, err := loadClaudeJSONDoc(root, rel, "Claude userConfig")
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "claude",
			Message:  err.Error(),
		}}
	}
	if !ok {
		return nil
	}
	for key, value := range doc {
		if _, ok := value.(map[string]any); !ok {
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "claude",
				Message:  fmt.Sprintf("Claude userConfig entry %q in %s must be a JSON object", key, rel),
			}}
		}
	}
	return nil
}
