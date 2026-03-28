package platformexec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/pluginmodel"
	"gopkg.in/yaml.v3"
)

type artifactDir struct {
	src string
	dst string
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func marshalJSON(value any) ([]byte, error) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func mustJSON(value any) []byte {
	body, err := marshalJSON(value)
	if err != nil {
		panic(err)
	}
	return body
}

func decodeJSONObject(body []byte, label string) (map[string]any, error) {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("%s is invalid JSON: %w", label, err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}

func mustYAML(value any) []byte {
	body, err := yaml.Marshal(value)
	if err != nil {
		panic(err)
	}
	return body
}

func copyArtifactDirs(root string, dirs ...artifactDir) ([]pluginmodel.Artifact, error) {
	var artifacts []pluginmodel.Artifact
	for _, dir := range dirs {
		copied, err := copyArtifacts(root, dir.src, dir.dst)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, copied...)
	}
	return artifacts, nil
}

func copyArtifacts(root, srcDir, dstRoot string) ([]pluginmodel.Artifact, error) {
	full := filepath.Join(root, srcDir)
	var artifacts []pluginmodel.Artifact
	if _, err := os.Stat(full); err != nil {
		return nil, nil
	}
	err := filepath.WalkDir(full, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(full, path)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.ToSlash(filepath.Join(dstRoot, rel)),
			Content: body,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(artifacts, func(a, b pluginmodel.Artifact) int { return strings.Compare(a.RelPath, b.RelPath) })
	return artifacts, nil
}

func copySingleArtifactIfExists(root, srcRel, dstRel string) ([]pluginmodel.Artifact, error) {
	body, err := os.ReadFile(filepath.Join(root, srcRel))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return []pluginmodel.Artifact{{RelPath: filepath.ToSlash(dstRel), Content: body}}, nil
}

func compactArtifacts(artifacts []pluginmodel.Artifact) []pluginmodel.Artifact {
	slices.SortFunc(artifacts, func(a, b pluginmodel.Artifact) int { return strings.Compare(a.RelPath, b.RelPath) })
	out := make([]pluginmodel.Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		n := len(out)
		if n > 0 && out[n-1].RelPath == artifact.RelPath {
			out[n-1] = artifact
			continue
		}
		out = append(out, artifact)
	}
	return out
}

func loadNativeExtraDoc(root string, state pluginmodel.TargetState, kind string, format pluginmodel.NativeDocFormat) (pluginmodel.NativeExtraDoc, error) {
	return pluginmodel.LoadNativeExtraDoc(root, state.DocPath(kind), format)
}

func readYAMLDoc[T any](root string, rel string) (T, bool, error) {
	var out T
	if strings.TrimSpace(rel) == "" {
		return out, false, nil
	}
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return out, false, err
	}
	if err := yaml.Unmarshal(body, &out); err != nil {
		return out, true, err
	}
	return out, true, nil
}

func renderManagedPluginArtifacts(name string, manifest pluginmodel.Manifest, portable pluginmodel.PortableComponents, includeAgents bool, relPath string, extra pluginmodel.NativeExtraDoc, label string, managedPaths []string) ([]pluginmodel.Artifact, error) {
	doc := map[string]any{
		"name":        name,
		"version":     manifest.Version,
		"description": manifest.Description,
	}
	if len(portable.Paths("skills")) > 0 {
		doc["skills"] = "./skills/"
	}
	if includeAgents {
		doc["agents"] = "./agents/"
	}
	if portable.MCP != nil {
		doc["mcpServers"] = "./.mcp.json"
	}
	if err := pluginmodel.MergeNativeExtraObject(doc, extra, label, managedPaths); err != nil {
		return nil, err
	}
	pluginJSON, err := marshalJSON(doc)
	if err != nil {
		return nil, err
	}
	artifacts := []pluginmodel.Artifact{{RelPath: relPath, Content: pluginJSON}}
	if portable.MCP != nil {
		mcpJSON, err := marshalJSON(portable.MCP.Servers)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{RelPath: ".mcp.json", Content: mcpJSON})
	}
	return artifacts, nil
}

func discoverFiles(root, dir string, allow func(string) bool) []string {
	full := filepath.Join(root, dir)
	var out []string
	filepath.WalkDir(full, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if allow != nil && !allow(rel) {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	slices.Sort(out)
	return out
}

type importedClaudeHooksFile struct {
	Hooks map[string][]importedClaudeHookEntry `json:"hooks"`
}

type importedClaudeHookEntry struct {
	Hooks []importedClaudeHookCommand `json:"hooks"`
}

type importedClaudeHookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func parseClaudeHooks(body []byte) (importedClaudeHooksFile, error) {
	var hooks importedClaudeHooksFile
	if err := json.Unmarshal(body, &hooks); err != nil {
		return importedClaudeHooksFile{}, err
	}
	return hooks, nil
}

func trimClaudeHookCommand(command, hookName string) (string, bool) {
	command = strings.TrimSpace(command)
	suffix := " " + strings.TrimSpace(hookName)
	if !strings.HasSuffix(command, suffix) {
		return "", false
	}
	entrypoint := strings.TrimSpace(strings.TrimSuffix(command, suffix))
	if entrypoint == "" {
		return "", false
	}
	return entrypoint, true
}

func inferClaudeEntrypoint(body []byte) (string, bool) {
	hooks, err := parseClaudeHooks(body)
	if err != nil {
		return "", false
	}
	for hookName, entries := range hooks.Hooks {
		for _, entry := range entries {
			for _, command := range entry.Hooks {
				if command.Type != "command" {
					continue
				}
				if entrypoint, ok := trimClaudeHookCommand(command.Command, hookName); ok {
					return entrypoint, true
				}
			}
		}
	}
	return "", false
}

func validateClaudeHookEntrypoints(body []byte, entrypoint string) ([]string, error) {
	hooks, err := parseClaudeHooks(body)
	if err != nil {
		return nil, err
	}
	var mismatches []string
	for hookName, entries := range hooks.Hooks {
		expected := strings.TrimSpace(entrypoint) + " " + strings.TrimSpace(hookName)
		foundCommand := false
		for _, entry := range entries {
			for _, command := range entry.Hooks {
				if strings.TrimSpace(command.Type) != "command" {
					continue
				}
				foundCommand = true
				if strings.TrimSpace(command.Command) != expected {
					mismatches = append(mismatches, fmt.Sprintf("entrypoint mismatch: Claude hook %q uses %q; expected %q from launcher.yaml entrypoint", hookName, command.Command, expected))
				}
			}
		}
		if !foundCommand {
			mismatches = append(mismatches, fmt.Sprintf("entrypoint mismatch: Claude hook %q declares no command hooks; expected %q", hookName, expected))
		}
	}
	return mismatches, nil
}

func defaultClaudeHooks(entrypoint string) []byte {
	type hookCommand struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	}
	type hookEntry struct {
		Hooks []hookCommand `json:"hooks"`
	}
	hooks := map[string][]hookEntry{}
	for _, name := range stableClaudeHookNames() {
		hooks[name] = []hookEntry{{Hooks: []hookCommand{{Type: "command", Command: entrypoint + " " + name}}}}
	}
	body, _ := marshalJSON(map[string]any{"hooks": hooks})
	return body
}

func stableClaudeHookNames() []string {
	return []string{"Stop", "PreToolUse", "UserPromptSubmit"}
}

type claudePackageMeta struct{}

type codexRuntimeMeta struct {
	ModelHint string `yaml:"model_hint,omitempty"`
}

type codexPackageMeta struct{}

type geminiPackageMeta struct {
	ContextFileName string   `yaml:"context_file_name,omitempty"`
	ExcludeTools    []string `yaml:"exclude_tools,omitempty"`
	MigratedTo      string   `yaml:"migrated_to,omitempty"`
	PlanDirectory   string   `yaml:"plan_directory,omitempty"`
}

type importedCodexPluginManifest struct {
	Name          string
	Version       string
	Description   string
	SkillsPath    string
	MCPServersRef string
	Extra         map[string]any
}

type importedCodexNativeConfig struct {
	Model  string
	Notify []string
	Extra  map[string]any
}

type importedGeminiExtension struct {
	Name        string
	Version     string
	Description string
	Meta        geminiPackageMeta
	MCPServers  map[string]any
	Settings    []any
	Themes      []any
	Extra       map[string]any
}

type importedClaudePluginManifest struct {
	Name               string
	Version            string
	Description        string
	CommandsRefs       []string
	CommandsOverride   bool
	AgentsRefs         []string
	AgentsOverride     bool
	HookRefs           []string
	HooksOverride      bool
	InlineHooks        map[string]any
	LSPRefs            []string
	LSPOverride        bool
	InlineLSP          map[string]any
	MCPRefs            []string
	MCPOverride        bool
	InlineMCP          map[string]any
	Settings           map[string]any
	SettingsProvided   bool
	UserConfig         map[string]any
	UserConfigProvided bool
	Extra              map[string]any
	Warnings           []string
}

func decodeClaudePathField(value any) ([]string, map[string]any, bool, string) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, nil, true, ""
		}
		return []string{text}, nil, true, ""
	case []any:
		refs := jsonStringArray(typed)
		if len(refs) == len(typed) {
			return refs, nil, true, ""
		}
		return nil, nil, false, "uses an unsupported mixed array shape"
	case map[string]any:
		return nil, typed, true, ""
	default:
		return nil, nil, false, "uses an unsupported value shape"
	}
}

func decodeClaudeUserConfig(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	default:
		return nil, false
	}
}

func readImportedClaudePluginManifest(root string) (importedClaudePluginManifest, []byte, bool, error) {
	body, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "plugin.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return importedClaudePluginManifest{}, nil, false, nil
		}
		return importedClaudePluginManifest{}, nil, false, err
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return importedClaudePluginManifest{}, nil, false, err
	}
	out := importedClaudePluginManifest{}
	if value, ok := raw["name"].(string); ok {
		out.Name = strings.TrimSpace(value)
	}
	if value, ok := raw["version"].(string); ok {
		out.Version = strings.TrimSpace(value)
	}
	if value, ok := raw["description"].(string); ok {
		out.Description = strings.TrimSpace(value)
	}
	if value, ok := raw["commands"]; ok {
		out.CommandsOverride = true
		refs, _, handled, warning := decodeClaudePathField(value)
		if handled {
			out.CommandsRefs = refs
		} else if warning != "" {
			out.Warnings = append(out.Warnings, fmt.Sprintf("Claude manifest field %q %s; skipped during import normalization", "commands", warning))
		}
		delete(raw, "commands")
	}
	if value, ok := raw["agents"]; ok {
		out.AgentsOverride = true
		refs, _, handled, warning := decodeClaudePathField(value)
		if handled {
			out.AgentsRefs = refs
		} else if warning != "" {
			out.Warnings = append(out.Warnings, fmt.Sprintf("Claude manifest field %q %s; skipped during import normalization", "agents", warning))
		}
		delete(raw, "agents")
	}
	if value, ok := raw["hooks"]; ok {
		out.HooksOverride = true
		refs, inline, handled, warning := decodeClaudePathField(value)
		if handled {
			out.HookRefs = refs
			out.InlineHooks = inline
		} else if warning != "" {
			out.Warnings = append(out.Warnings, fmt.Sprintf("Claude manifest field %q %s; skipped during import normalization", "hooks", warning))
		}
		delete(raw, "hooks")
	}
	if value, ok := raw["lspServers"]; ok {
		out.LSPOverride = true
		refs, inline, handled, warning := decodeClaudePathField(value)
		if handled {
			out.LSPRefs = refs
			out.InlineLSP = inline
		} else if warning != "" {
			out.Warnings = append(out.Warnings, fmt.Sprintf("Claude manifest field %q %s; skipped during import normalization", "lspServers", warning))
		}
		delete(raw, "lspServers")
	}
	if value, ok := raw["mcpServers"]; ok {
		out.MCPOverride = true
		refs, inline, handled, warning := decodeClaudePathField(value)
		if handled {
			out.MCPRefs = refs
			out.InlineMCP = inline
		} else if warning != "" {
			out.Warnings = append(out.Warnings, fmt.Sprintf("Claude manifest field %q %s; skipped during import normalization", "mcpServers", warning))
		}
		delete(raw, "mcpServers")
	}
	if value, ok := raw["settings"]; ok {
		if settings, ok := decodeClaudeUserConfig(value); ok {
			out.Settings = settings
			out.SettingsProvided = true
		} else {
			out.Warnings = append(out.Warnings, `Claude manifest field "settings" must be a JSON object for package-standard normalization; skipped during import normalization`)
		}
		delete(raw, "settings")
	}
	if value, ok := raw["userConfig"]; ok {
		if userConfig, ok := decodeClaudeUserConfig(value); ok {
			out.UserConfig = userConfig
			out.UserConfigProvided = true
		} else {
			out.Warnings = append(out.Warnings, `Claude manifest field "userConfig" must be a JSON object for package-standard normalization; skipped during import normalization`)
		}
		delete(raw, "userConfig")
	}
	delete(raw, "name")
	delete(raw, "version")
	delete(raw, "description")
	if len(raw) > 0 {
		out.Extra = raw
	}
	return out, body, true, nil
}

func cleanRelativeRef(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}

func copyArtifactsFromRefs(root string, refs []string, dstRoot string) ([]pluginmodel.Artifact, error) {
	var artifacts []pluginmodel.Artifact
	for _, ref := range refs {
		ref = cleanRelativeRef(ref)
		if ref == "" {
			continue
		}
		full := filepath.Join(root, ref)
		info, err := os.Stat(full)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			copied, err := copyArtifacts(root, ref, dstRoot)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, copied...)
			continue
		}
		body, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.ToSlash(filepath.Join(dstRoot, filepath.Base(ref))),
			Content: body,
		})
	}
	return compactArtifacts(artifacts), nil
}

func jsonStringArray(values []any) []string {
	var out []string
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func readImportedCodexConfig(root string) (importedCodexNativeConfig, []byte, error) {
	body, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	if err != nil {
		return importedCodexNativeConfig{}, nil, err
	}
	var raw map[string]any
	if err := toml.Unmarshal(body, &raw); err != nil {
		return importedCodexNativeConfig{}, nil, err
	}
	config := importedCodexNativeConfig{}
	if value, ok := raw["model"].(string); ok {
		config.Model = strings.TrimSpace(value)
	}
	if values, ok := raw["notify"].([]any); ok {
		config.Notify = jsonStringArray(values)
	}
	delete(raw, "model")
	delete(raw, "notify")
	if len(raw) > 0 {
		config.Extra = raw
	}
	return config, body, nil
}

func readImportedCodexPluginManifest(root string) (importedCodexPluginManifest, []byte, error) {
	body, err := os.ReadFile(filepath.Join(root, ".codex-plugin", "plugin.json"))
	if err != nil {
		return importedCodexPluginManifest{}, nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return importedCodexPluginManifest{}, nil, err
	}
	out := importedCodexPluginManifest{}
	if value, ok := raw["name"].(string); ok {
		out.Name = strings.TrimSpace(value)
	}
	if value, ok := raw["version"].(string); ok {
		out.Version = strings.TrimSpace(value)
	}
	if value, ok := raw["description"].(string); ok {
		out.Description = strings.TrimSpace(value)
	}
	if value, ok := raw["skills"].(string); ok {
		out.SkillsPath = strings.TrimSpace(value)
	}
	if value, ok := raw["mcpServers"].(string); ok {
		out.MCPServersRef = strings.TrimSpace(value)
	}
	delete(raw, "name")
	delete(raw, "version")
	delete(raw, "description")
	delete(raw, "skills")
	delete(raw, "mcpServers")
	if len(raw) > 0 {
		out.Extra = raw
	}
	return out, body, nil
}

func decodeImportedGeminiExtension(body []byte) (importedGeminiExtension, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return importedGeminiExtension{}, err
	}
	out := importedGeminiExtension{}
	if value, ok := raw["name"].(string); ok && strings.TrimSpace(value) != "" {
		out.Name = value
	}
	if value, ok := raw["version"].(string); ok && strings.TrimSpace(value) != "" {
		out.Version = value
	}
	if value, ok := raw["description"].(string); ok && strings.TrimSpace(value) != "" {
		out.Description = value
	}
	if servers, ok := raw["mcpServers"].(map[string]any); ok && len(servers) > 0 {
		out.MCPServers = servers
	}
	if value, ok := raw["contextFileName"].(string); ok && strings.TrimSpace(value) != "" {
		out.Meta.ContextFileName = value
	}
	if values, ok := raw["excludeTools"].([]any); ok {
		out.Meta.ExcludeTools = jsonStringArray(values)
	}
	if value, ok := raw["migratedTo"].(string); ok && strings.TrimSpace(value) != "" {
		out.Meta.MigratedTo = value
	}
	if plan, ok := raw["plan"].(map[string]any); ok {
		if directory, ok := plan["directory"].(string); ok && strings.TrimSpace(directory) != "" {
			out.Meta.PlanDirectory = directory
			delete(plan, "directory")
			if len(plan) == 0 {
				delete(raw, "plan")
			} else {
				raw["plan"] = plan
			}
		}
	}
	if values, ok := raw["settings"].([]any); ok {
		out.Settings = values
	}
	if values, ok := raw["themes"].([]any); ok {
		out.Themes = values
	}
	delete(raw, "name")
	delete(raw, "version")
	delete(raw, "description")
	delete(raw, "mcpServers")
	delete(raw, "contextFileName")
	delete(raw, "excludeTools")
	delete(raw, "migratedTo")
	delete(raw, "settings")
	delete(raw, "themes")
	if plan, ok := raw["plan"].(map[string]any); ok && len(plan) == 0 {
		delete(raw, "plan")
	}
	if len(raw) > 0 {
		out.Extra = raw
	}
	return out, nil
}

func readImportedGeminiExtension(root string) (importedGeminiExtension, bool, error) {
	body, err := os.ReadFile(filepath.Join(root, "gemini-extension.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return importedGeminiExtension{}, false, nil
		}
		return importedGeminiExtension{}, false, err
	}
	data, err := decodeImportedGeminiExtension(body)
	if err != nil {
		return importedGeminiExtension{}, false, err
	}
	return data, true, nil
}

func importedGeminiSettingsArtifacts(values []any) []pluginmodel.Artifact {
	used := map[string]int{}
	var artifacts []pluginmodel.Artifact
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		setting := geminiSetting{}
		if name, ok := item["name"].(string); ok {
			setting.Name = name
		}
		if description, ok := item["description"].(string); ok {
			setting.Description = description
		}
		if envVar, ok := item["envVar"].(string); ok {
			setting.EnvVar = envVar
		}
		if sensitive, ok := item["sensitive"].(bool); ok {
			setting.Sensitive = sensitive
		}
		filename := collisionSafeSlug(setting.Name, used) + ".yaml"
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join("targets", "gemini", "settings", filename),
			Content: mustYAML(setting),
		})
	}
	return artifacts
}

func importedGeminiThemeArtifacts(values []any) []pluginmodel.Artifact {
	used := map[string]int{}
	var artifacts []pluginmodel.Artifact
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		name, _ := item["name"].(string)
		filename := collisionSafeSlug(name, used) + ".yaml"
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join("targets", "gemini", "themes", filename),
			Content: mustYAML(item),
		})
	}
	return artifacts
}

type geminiSetting struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	EnvVar      string `yaml:"env_var" json:"envVar"`
	Sensitive   bool   `yaml:"sensitive" json:"sensitive"`
}

var geminiSettingEnvVarRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var geminiThemeObjectKeys = map[string]struct{}{
	"background": {},
	"text":       {},
	"status":     {},
	"ui":         {},
}

var geminiThemeStringArrayKeys = map[string]struct{}{
	"GradientColors": {},
	"gradient":       {},
}

func collisionSafeSlug(name string, used map[string]int) string {
	base := strings.TrimSpace(strings.ToLower(name))
	if base == "" {
		base = "item"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "item"
	}
	used[slug]++
	if used[slug] == 1 {
		return slug
	}
	return fmt.Sprintf("%s-%d", slug, used[slug])
}

type geminiContextSelection struct {
	ArtifactName string
	SourcePath   string
}

func geminiExtraContextArtifactPath(rel string) string {
	return filepath.ToSlash(filepath.Join("contexts", filepath.Base(rel)))
}

func loadGeminiSettings(root string, rels []string) ([]map[string]any, error) {
	if len(rels) == 0 {
		return nil, nil
	}
	seenNames := map[string]string{}
	seenEnvVars := map[string]string{}
	var settings []map[string]any
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		var raw map[string]any
		if err := yaml.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		var setting geminiSetting
		if err := yaml.Unmarshal(body, &setting); err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		if message := validateGeminiSettingMap(rel, raw, setting); message != "" {
			return nil, fmt.Errorf("invalid %s: %s", rel, message)
		}
		nameKey := strings.ToLower(strings.TrimSpace(setting.Name))
		if prev, ok := seenNames[nameKey]; ok {
			return nil, fmt.Errorf("invalid %s: Gemini setting name %q duplicates %s", rel, setting.Name, prev)
		}
		seenNames[nameKey] = rel
		envKey := strings.ToLower(strings.TrimSpace(setting.EnvVar))
		if prev, ok := seenEnvVars[envKey]; ok {
			return nil, fmt.Errorf("invalid %s: Gemini setting env_var %q duplicates %s", rel, setting.EnvVar, prev)
		}
		seenEnvVars[envKey] = rel
		settings = append(settings, map[string]any{
			"name":        setting.Name,
			"description": setting.Description,
			"envVar":      setting.EnvVar,
			"sensitive":   setting.Sensitive,
		})
	}
	return settings, nil
}

func loadGeminiThemes(root string, rels []string) ([]map[string]any, error) {
	if len(rels) == 0 {
		return nil, nil
	}
	seenNames := map[string]string{}
	var themes []map[string]any
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		var raw map[string]any
		if err := yaml.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		if raw == nil {
			raw = map[string]any{}
		}
		name, _ := raw["name"].(string)
		if message := validateGeminiThemeMap(rel, raw); message != "" {
			return nil, fmt.Errorf("invalid %s: %s", rel, message)
		}
		name = strings.TrimSpace(name)
		nameKey := strings.ToLower(name)
		if prev, ok := seenNames[nameKey]; ok {
			return nil, fmt.Errorf("invalid %s: Gemini theme name %q duplicates %s", rel, name, prev)
		}
		seenNames[nameKey] = rel
		theme := map[string]any{}
		for key, value := range raw {
			switch strings.TrimSpace(key) {
			case "":
				continue
			case "name":
				theme["name"] = value
			default:
				theme[key] = value
			}
		}
		themes = append(themes, theme)
	}
	return themes, nil
}

var geminiYAMLFileRe = regexp.MustCompile(`(?i)\.(yaml|yml)$`)

func readGeminiYAMLMap(root, rel string) ([]byte, map[string]any, error) {
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, nil, err
	}
	var raw map[string]any
	if err := yaml.Unmarshal(body, &raw); err != nil {
		return nil, nil, err
	}
	return body, raw, nil
}

func trimmedExtraBody(doc pluginmodel.NativeExtraDoc) []byte {
	return bytes.TrimSpace(doc.Raw)
}

func validateGeminiSettingMap(_ string, raw map[string]any, setting geminiSetting) string {
	_, hasSensitive := raw["sensitive"]
	_, sensitiveIsBool := raw["sensitive"].(bool)
	if strings.TrimSpace(setting.Name) == "" || strings.TrimSpace(setting.Description) == "" || strings.TrimSpace(setting.EnvVar) == "" || !hasSensitive || !sensitiveIsBool {
		return "Gemini settings require string name, description, env_var, and boolean sensitive"
	}
	if !geminiSettingEnvVarRe.MatchString(strings.TrimSpace(setting.EnvVar)) {
		return fmt.Sprintf("Gemini settings require env_var %q to be a valid environment variable name", setting.EnvVar)
	}
	return ""
}

func validateGeminiThemeMap(rel string, raw map[string]any) string {
	name, _ := raw["name"].(string)
	if strings.TrimSpace(name) == "" {
		return "Gemini themes require name"
	}
	if len(raw) <= 1 {
		return "Gemini themes require at least one theme token besides name"
	}
	for key, value := range raw {
		key = strings.TrimSpace(key)
		if key == "" || key == "name" || key == "type" {
			continue
		}
		switch {
		case geminiThemeRequiresObject(key):
			if _, ok := value.(map[string]any); !ok {
				return fmt.Sprintf("Gemini theme key %q must be a YAML object", key)
			}
			if message := validateGeminiThemeValue(filepath.ToSlash(filepath.Join(rel, key)), value); message != "" {
				return message
			}
		case geminiThemeRequiresStringArray(key):
			if _, ok := geminiStringSlice(value); !ok {
				return fmt.Sprintf("Gemini theme key %q must be an array of non-empty strings", key)
			}
		default:
			if message := validateGeminiThemeValue(filepath.ToSlash(filepath.Join(rel, key)), value); message != "" {
				return message
			}
		}
	}
	return ""
}

func validateGeminiThemeValue(path string, value any) string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return fmt.Sprintf("Gemini theme token %q must be a non-empty string", path)
		}
		return ""
	case []any:
		if _, ok := geminiStringSlice(typed); !ok {
			return fmt.Sprintf("Gemini theme token %q must be an array of non-empty strings", path)
		}
		return ""
	case map[string]any:
		if len(typed) == 0 {
			return fmt.Sprintf("Gemini theme object %q may not be empty", path)
		}
		for childKey, childValue := range typed {
			childKey = strings.TrimSpace(childKey)
			if childKey == "" {
				continue
			}
			if geminiThemeRequiresStringArray(childKey) {
				if _, ok := geminiStringSlice(childValue); !ok {
					return fmt.Sprintf("Gemini theme key %q must be an array of non-empty strings", filepath.ToSlash(filepath.Join(path, childKey)))
				}
				continue
			}
			if message := validateGeminiThemeValue(filepath.ToSlash(filepath.Join(path, childKey)), childValue); message != "" {
				return message
			}
		}
		return ""
	default:
		return fmt.Sprintf("Gemini theme token %q must be a non-empty string, string array, or object", path)
	}
}

func geminiThemeRequiresObject(key string) bool {
	_, ok := geminiThemeObjectKeys[key]
	return ok
}

func geminiThemeRequiresStringArray(key string) bool {
	_, ok := geminiThemeStringArrayKeys[key]
	return ok
}
