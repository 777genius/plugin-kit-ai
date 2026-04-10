package platformexec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type claudePackageMeta struct{}

type importedClaudePluginManifest struct {
	Name               string
	Version            string
	Description        string
	SkillsRefs         []string
	SkillsOverride     bool
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
	if value, ok := raw["skills"]; ok {
		out.SkillsOverride = true
		refs, _, handled, warning := decodeClaudePathField(value)
		if handled {
			out.SkillsRefs = refs
		} else if warning != "" {
			out.Warnings = append(out.Warnings, fmt.Sprintf("Claude manifest field %q %s; skipped during import normalization", "skills", warning))
		}
		delete(raw, "skills")
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
