package platformexec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

func (geminiAdapter) Validate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]Diagnostic, error) {
	meta, _, err := readYAMLDoc[geminiPackageMeta](root, state.DocPath("package_metadata"))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	}
	hookPaths := state.ComponentPaths("hooks")
	var diagnostics []Diagnostic
	if base := geminiExtensionDirBase(root); base != graph.Manifest.Name {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityWarning,
			Code:     CodeGeminiDirNameMismatch,
			Path:     root,
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension directory basename %q does not match extension name %q", base, graph.Manifest.Name),
		})
	}
	if graph.Portable.MCP != nil {
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "gemini")
		if err != nil {
			return nil, err
		}
		diagnostics = append(diagnostics, validateGeminiMCPServers(graph.Portable.MCP.Path, projected)...)
	}
	diagnostics = append(diagnostics, validateGeminiExcludeTools(state.DocPath("package_metadata"), meta.ExcludeTools)...)
	diagnostics = append(diagnostics, validateGeminiContext(graph, state, meta)...)
	diagnostics = append(diagnostics, validateGeminiSettings(root, state.ComponentPaths("settings"))...)
	diagnostics = append(diagnostics, validateGeminiThemes(root, state.ComponentPaths("themes"))...)
	diagnostics = append(diagnostics, validateGeminiPolicies(root, state.ComponentPaths("policies"))...)
	diagnostics = append(diagnostics, validateGeminiCommands(root, state.ComponentPaths("commands"))...)
	diagnostics = append(diagnostics, validateGeminiHookFiles(root, hookPaths)...)
	if graph.Launcher != nil {
		diagnostics = append(diagnostics, validateGeminiHookEntrypointConsistency(root, hookPaths, strings.TrimSpace(graph.Launcher.Entrypoint))...)
	}
	diagnostics = append(diagnostics, validateGeminiGeneratedHooks(root, graph, hookPaths)...)
	extension, ok, err := readImportedGeminiExtension(root)
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json is invalid: %v", err),
		})
		return diagnostics, nil
	}
	if !ok {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  "Gemini extension manifest gemini-extension.json is not readable",
		})
		return diagnostics, nil
	}
	if strings.TrimSpace(extension.Name) != strings.TrimSpace(graph.Manifest.Name) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json sets name %q; expected %q from plugin.yaml", strings.TrimSpace(extension.Name), strings.TrimSpace(graph.Manifest.Name)),
		})
	}
	if strings.TrimSpace(extension.Version) != strings.TrimSpace(graph.Manifest.Version) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json sets version %q; expected %q from plugin.yaml", strings.TrimSpace(extension.Version), strings.TrimSpace(graph.Manifest.Version)),
		})
	}
	if strings.TrimSpace(extension.Description) != strings.TrimSpace(graph.Manifest.Description) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json sets description %q; expected %q from plugin.yaml", strings.TrimSpace(extension.Description), strings.TrimSpace(graph.Manifest.Description)),
		})
	}
	if !geminiPackageMetaEqual(meta, extension.Meta) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  "Gemini extension manifest gemini-extension.json package metadata does not match targets/gemini/package.yaml",
		})
	}
	if settings, err := loadGeminiSettings(root, state.ComponentPaths("settings")); err != nil {
		return nil, err
	} else if len(settings) > 0 {
		if !jsonDocumentsEqual(settings, extension.Settings) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     "gemini-extension.json",
				Target:   "gemini",
				Message:  "Gemini extension manifest gemini-extension.json settings do not match authored targets/gemini/settings/**",
			})
		}
	} else if len(extension.Settings) > 0 {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  "Gemini extension manifest gemini-extension.json may not define settings when targets/gemini/settings/** is absent",
		})
	}
	if themes, err := loadGeminiThemes(root, state.ComponentPaths("themes")); err != nil {
		return nil, err
	} else if len(themes) > 0 {
		if !jsonDocumentsEqual(themes, extension.Themes) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     "gemini-extension.json",
				Target:   "gemini",
				Message:  "Gemini extension manifest gemini-extension.json themes do not match authored targets/gemini/themes/**",
			})
		}
	} else if len(extension.Themes) > 0 {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  "Gemini extension manifest gemini-extension.json may not define themes when targets/gemini/themes/** is absent",
		})
	}
	if len(extension.MCPServers) > 0 {
		diagnostics = append(diagnostics, validateGeminiMCPServers("gemini-extension.json", extension.MCPServers)...)
	}
	if graph.Portable.MCP != nil {
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "gemini")
		if err != nil {
			return nil, err
		}
		if !jsonDocumentsEqual(projected, extension.MCPServers) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     "gemini-extension.json",
				Target:   "gemini",
				Message:  "Gemini extension manifest gemini-extension.json mcpServers do not match authored portable MCP projection",
			})
		}
	} else if len(extension.MCPServers) > 0 {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  "Gemini extension manifest gemini-extension.json may not define mcpServers when portable MCP is absent",
		})
	}
	if expected, ok, err := selectGeminiPrimaryContext(graph, state, meta); err != nil {
		return nil, err
	} else if ok {
		if strings.TrimSpace(extension.Meta.ContextFileName) != expected.ArtifactName {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     "gemini-extension.json",
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json sets contextFileName %q; expected %q from authored context selection", strings.TrimSpace(extension.Meta.ContextFileName), expected.ArtifactName),
			})
		}
		if !fileExists(filepath.Join(root, expected.ArtifactName)) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     expected.ArtifactName,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini primary context file %s is not readable", expected.ArtifactName),
			})
		}
	} else if strings.TrimSpace(extension.Meta.ContextFileName) != "" {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json sets contextFileName %q without an authored primary context", strings.TrimSpace(extension.Meta.ContextFileName)),
		})
	}
	return diagnostics, nil
}

func validateGeminiGeneratedHooks(root string, graph pluginmodel.PackageGraph, authoredHookPaths []string) []Diagnostic {
	const generatedHooksPath = "hooks/hooks.json"
	var diagnostics []Diagnostic
	body, err := os.ReadFile(filepath.Join(root, generatedHooksPath))
	if len(authoredHookPaths) > 0 {
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     generatedHooksPath,
				Target:   "gemini",
				Message:  "Gemini generated hooks/hooks.json is not readable",
			})
			return diagnostics
		}
		renderedHooks, err := parseGeminiHooks(body)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     generatedHooksPath,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini generated hooks file %s is invalid: %v", generatedHooksPath, err),
			})
			return diagnostics
		}
		authoredBody, readErr := os.ReadFile(filepath.Join(root, authoredHookPaths[0]))
		if readErr != nil {
			return diagnostics
		}
		authoredHooks, parseErr := parseGeminiHooks(authoredBody)
		if parseErr != nil {
			return diagnostics
		}
		if !jsonDocumentsEqual(authoredHooks, renderedHooks) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     generatedHooksPath,
				Target:   "gemini",
				Message:  "Gemini generated hooks/hooks.json does not match authored targets/gemini/hooks/hooks.json",
			})
		}
		return diagnostics
	}
	if !geminiUsesGeneratedHooks(graph, pluginmodel.TargetState{Target: "gemini"}) {
		if err == nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     generatedHooksPath,
				Target:   "gemini",
				Message:  "Gemini generated hooks/hooks.json may not exist when no authored hooks or generated launcher hooks are expected",
			})
		}
		return diagnostics
	}
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     generatedHooksPath,
			Target:   "gemini",
			Message:  "Gemini generated hooks/hooks.json is not readable",
		})
		return diagnostics
	}
	renderedHooks, err := parseGeminiHooks(body)
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     generatedHooksPath,
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini generated hooks file %s is invalid: %v", generatedHooksPath, err),
		})
		return diagnostics
	}
	expectedHooks, err := parseGeminiHooks(defaultGeminiHooks(strings.TrimSpace(graph.Launcher.Entrypoint)))
	if err != nil {
		return diagnostics
	}
	if !jsonDocumentsEqual(expectedHooks, renderedHooks) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     generatedHooksPath,
			Target:   "gemini",
			Message:  "Gemini generated hooks/hooks.json does not match the managed launcher-derived hooks projection",
		})
	}
	return diagnostics
}

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

func validateGeminiSettings(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	seenNames := map[string]string{}
	seenEnvVars := map[string]string{}
	for _, rel := range rels {
		body, raw, err := readGeminiYAMLMap(root, rel)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s is invalid YAML: %v", rel, err),
			})
			continue
		}
		var setting geminiSetting
		if err := yaml.Unmarshal(body, &setting); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s is invalid YAML: %v", rel, err),
			})
			continue
		}
		if message := validateGeminiSettingMap(rel, raw, setting); message != "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s: %s", rel, message),
			})
			continue
		}
		nameKey := strings.ToLower(strings.TrimSpace(setting.Name))
		if prev, ok := seenNames[nameKey]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s duplicates setting name %q already declared in %s", rel, setting.Name, prev),
			})
		} else {
			seenNames[nameKey] = rel
		}
		envKey := strings.ToLower(strings.TrimSpace(setting.EnvVar))
		if prev, ok := seenEnvVars[envKey]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s duplicates env_var %q already declared in %s", rel, setting.EnvVar, prev),
			})
		} else {
			seenEnvVars[envKey] = rel
		}
	}
	return diagnostics
}

func validateGeminiThemes(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	seenNames := map[string]string{}
	for _, rel := range rels {
		_, raw, err := readGeminiYAMLMap(root, rel)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini theme file %s is invalid YAML: %v", rel, err),
			})
			continue
		}
		name, _ := raw["name"].(string)
		if message := validateGeminiThemeMap(rel, raw); message != "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini theme file %s: %s", rel, message),
			})
			continue
		}
		name = strings.TrimSpace(name)
		nameKey := strings.ToLower(name)
		if prev, ok := seenNames[nameKey]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini theme file %s duplicates theme name %q already declared in %s", rel, name, prev),
			})
			continue
		}
		seenNames[nameKey] = rel
	}
	return diagnostics
}

func validateGeminiPolicies(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini policy file %s is not readable: %v", rel, err),
			})
			continue
		}
		text := string(body)
		for _, key := range []string{"allow", "yolo"} {
			if strings.Contains(text, key+" =") {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityWarning,
					Code:     CodeGeminiPolicyIgnored,
					Path:     rel,
					Target:   "gemini",
					Message:  fmt.Sprintf("Gemini extension policies ignore %q at extension tier", key),
				})
			}
		}
	}
	return diagnostics
}

func validateGeminiCommands(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		if filepath.Ext(rel) != ".toml" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini command file %s must use the .toml extension", rel),
			})
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini command file %s is not readable: %v", rel, err),
			})
			continue
		}
		var discard map[string]any
		if err := toml.Unmarshal(body, &discard); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini command file %s is invalid TOML: %v", rel, err),
			})
		}
	}
	return diagnostics
}

func validateGeminiHookFiles(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini JSON asset %s is not readable: %v", rel, err),
			})
			continue
		}
		var discard map[string]any
		if err := json.Unmarshal(body, &discard); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini hooks file %s is invalid JSON: %v", rel, err),
			})
			continue
		}
		hooks, ok := discard["hooks"].(map[string]any)
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini hooks file %s must define a top-level hooks object", rel),
			})
			continue
		}
		if hooks == nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini hooks file %s must define a top-level hooks object", rel),
			})
		}
	}
	return diagnostics
}

func validateGeminiHookEntrypointConsistency(root string, rels []string, entrypoint string) []Diagnostic {
	if strings.TrimSpace(entrypoint) == "" {
		return nil
	}
	var diagnostics []Diagnostic
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		mismatches, err := validateGeminiHookEntrypoints(body, entrypoint)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini hooks file %s is invalid JSON: %v", rel, err),
			})
			continue
		}
		for _, msg := range mismatches {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeEntrypointMismatch,
				Path:     rel,
				Target:   "gemini",
				Message:  msg,
			})
		}
	}
	return diagnostics
}
