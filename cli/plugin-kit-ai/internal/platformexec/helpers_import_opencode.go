package platformexec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/tailscale/hujson"
	"gopkg.in/yaml.v3"
)

type opencodePackageMeta struct {
	Plugins []opencodePluginRef `yaml:"plugins,omitempty"`
}

type opencodePluginRef struct {
	Name    string
	Options map[string]any
}

func (r *opencodePluginRef) UnmarshalYAML(node *yaml.Node) error {
	if node == nil {
		*r = opencodePluginRef{}
		return nil
	}
	switch node.Kind {
	case yaml.ScalarNode:
		var name string
		if err := node.Decode(&name); err != nil {
			return err
		}
		r.Name = strings.TrimSpace(name)
		r.Options = nil
		return nil
	case yaml.MappingNode:
		var raw map[string]any
		if err := node.Decode(&raw); err != nil {
			return err
		}
		for key := range raw {
			switch key {
			case "name", "options":
			default:
				return fmt.Errorf("unsupported OpenCode plugin metadata field %q", key)
			}
		}
		name, _ := raw["name"].(string)
		r.Name = strings.TrimSpace(name)
		if options, ok := raw["options"]; ok {
			typed, ok := options.(map[string]any)
			if !ok {
				return fmt.Errorf("OpenCode plugin metadata options must be a mapping")
			}
			r.Options = typed
		} else {
			r.Options = nil
		}
		return nil
	default:
		return fmt.Errorf("OpenCode plugin metadata entries must be strings or mappings")
	}
}

func (r opencodePluginRef) MarshalYAML() (any, error) {
	if len(r.Options) == 0 {
		return strings.TrimSpace(r.Name), nil
	}
	return map[string]any{
		"name":    strings.TrimSpace(r.Name),
		"options": r.Options,
	}, nil
}

func (r opencodePluginRef) jsonValue() any {
	name := strings.TrimSpace(r.Name)
	if len(r.Options) == 0 {
		return name
	}
	return []any{name, r.Options}
}

type importedOpenCodeConfig struct {
	Plugins          []opencodePluginRef
	PluginsProvided  bool
	MCP              map[string]any
	MCPProvided      bool
	Commands         map[string]any
	CommandsProvided bool
	Agents           map[string]any
	AgentsProvided   bool
	DefaultAgent     string
	DefaultAgentSet  bool
	Instructions     []string
	InstructionsSet  bool
	Permission       any
	PermissionSet    bool
	Extra            map[string]any
}

func decodeImportedOpenCodeConfig(body []byte) (importedOpenCodeConfig, error) {
	body, err := hujson.Standardize(body)
	if err != nil {
		return importedOpenCodeConfig{}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return importedOpenCodeConfig{}, err
	}
	out := importedOpenCodeConfig{}
	if pluginsRaw, ok := raw["plugin"]; ok {
		out.PluginsProvided = true
		values, ok := pluginsRaw.([]any)
		if !ok {
			return importedOpenCodeConfig{}, fmt.Errorf("OpenCode config field %q must be an array of strings or [name, options] tuples", "plugin")
		}
		out.Plugins = make([]opencodePluginRef, 0, len(values))
		for i, value := range values {
			ref, err := normalizeImportedOpenCodePluginRef(value)
			if err != nil {
				return importedOpenCodeConfig{}, fmt.Errorf("OpenCode config field %q has invalid entry at index %d: %w", "plugin", i, err)
			}
			out.Plugins = append(out.Plugins, ref)
		}
	}
	if mcpRaw, ok := raw["mcp"]; ok {
		out.MCPProvided = true
		servers, ok := mcpRaw.(map[string]any)
		if !ok {
			return importedOpenCodeConfig{}, fmt.Errorf("OpenCode config field %q must be a JSON object", "mcp")
		}
		out.MCP = servers
	}
	if commandsRaw, ok := raw["command"]; ok {
		out.CommandsProvided = true
		values, ok := commandsRaw.(map[string]any)
		if !ok {
			return importedOpenCodeConfig{}, fmt.Errorf("OpenCode config field %q must be a JSON object", "command")
		}
		out.Commands = values
	}
	if agentsRaw, ok := raw["agent"]; ok {
		out.AgentsProvided = true
		values, ok := agentsRaw.(map[string]any)
		if !ok {
			return importedOpenCodeConfig{}, fmt.Errorf("OpenCode config field %q must be a JSON object", "agent")
		}
		out.Agents = values
	}
	if defaultAgent, ok := raw["default_agent"]; ok {
		text, ok := defaultAgent.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return importedOpenCodeConfig{}, fmt.Errorf("OpenCode config field %q must be a non-empty string", "default_agent")
		}
		out.DefaultAgent = strings.TrimSpace(text)
		out.DefaultAgentSet = true
	}
	if instructions, ok := raw["instructions"]; ok {
		values, ok := instructions.([]any)
		if !ok {
			return importedOpenCodeConfig{}, fmt.Errorf("OpenCode config field %q must be an array of strings", "instructions")
		}
		out.Instructions = make([]string, 0, len(values))
		for i, value := range values {
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return importedOpenCodeConfig{}, fmt.Errorf("OpenCode config field %q must contain non-empty strings (invalid entry at index %d)", "instructions", i)
			}
			out.Instructions = append(out.Instructions, strings.TrimSpace(text))
		}
		out.InstructionsSet = true
	}
	if permission, ok := raw["permission"]; ok {
		if !isOpenCodePermissionValue(permission) {
			return importedOpenCodeConfig{}, fmt.Errorf("OpenCode config field %q must be a string or JSON object", "permission")
		}
		out.Permission = permission
		out.PermissionSet = true
	}
	if deprecatedMode, ok := raw["mode"]; ok {
		values, ok := deprecatedMode.(map[string]any)
		if !ok {
			return importedOpenCodeConfig{}, fmt.Errorf("OpenCode config field %q must be a JSON object", "mode")
		}
		if !out.AgentsProvided {
			out.Agents = map[string]any{}
			out.AgentsProvided = true
		}
		for name, value := range values {
			if _, exists := out.Agents[name]; exists {
				continue
			}
			out.Agents[name] = value
		}
	}
	delete(raw, "$schema")
	delete(raw, "plugin")
	delete(raw, "mcp")
	delete(raw, "command")
	delete(raw, "agent")
	delete(raw, "default_agent")
	delete(raw, "instructions")
	delete(raw, "permission")
	delete(raw, "mode")
	if len(raw) > 0 {
		out.Extra = raw
	}
	return out, nil
}

func normalizeImportedOpenCodePluginRef(value any) (opencodePluginRef, error) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return opencodePluginRef{}, fmt.Errorf("plugin ref must be a non-empty string")
		}
		return opencodePluginRef{Name: text}, nil
	case []any:
		if len(typed) != 2 {
			return opencodePluginRef{}, fmt.Errorf("tuple plugin ref must have exactly 2 items")
		}
		name, ok := typed[0].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return opencodePluginRef{}, fmt.Errorf("tuple plugin ref name must be a non-empty string")
		}
		options, ok := typed[1].(map[string]any)
		if !ok {
			return opencodePluginRef{}, fmt.Errorf("tuple plugin ref options must be an object")
		}
		return opencodePluginRef{Name: strings.TrimSpace(name), Options: options}, nil
	default:
		return opencodePluginRef{}, fmt.Errorf("plugin ref must be a string or [name, options] tuple")
	}
}

func validateOpenCodePluginRefs(refs []opencodePluginRef) error {
	for i, ref := range refs {
		if strings.TrimSpace(ref.Name) == "" {
			return fmt.Errorf("plugin entry %d must define a non-empty name", i)
		}
		if ref.Options == nil {
			continue
		}
		for key := range ref.Options {
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("plugin entry %d options may not contain empty keys", i)
			}
		}
	}
	return nil
}

func jsonValuesForOpenCodePlugins(refs []opencodePluginRef) []any {
	out := make([]any, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.jsonValue())
	}
	return out
}

func isOpenCodePermissionValue(value any) bool {
	if _, ok := value.(string); ok {
		return true
	}
	_, ok := value.(map[string]any)
	return ok
}

func resolveOpenCodeConfigPathInDir(dir string, warningBase string) (string, []pluginmodel.Warning, bool, error) {
	jsonRel := "opencode.json"
	jsoncRel := "opencode.jsonc"
	jsonPath := filepath.Join(dir, jsonRel)
	jsoncPath := filepath.Join(dir, jsoncRel)
	hasJSON := fileExists(jsonPath)
	hasJSONC := fileExists(jsoncPath)
	warnPath := jsoncRel
	if strings.TrimSpace(warningBase) != "" {
		warnPath = filepath.ToSlash(filepath.Join(warningBase, jsoncRel))
	}
	switch {
	case hasJSON && hasJSONC:
		return jsonPath, []pluginmodel.Warning{{
			Kind:    pluginmodel.WarningFidelity,
			Path:    warnPath,
			Message: "ignored opencode.jsonc because opencode.json takes precedence during OpenCode import normalization",
		}}, true, nil
	case hasJSON:
		return jsonPath, nil, true, nil
	case hasJSONC:
		return jsoncPath, nil, true, nil
	default:
		return "", nil, false, nil
	}
}

func resolveOpenCodeConfigPath(root string) (string, []pluginmodel.Warning, bool, error) {
	path, warnings, ok, err := resolveOpenCodeConfigPathInDir(root, "")
	if err != nil || !ok {
		return "", warnings, ok, err
	}
	rel, rerr := filepath.Rel(root, path)
	if rerr != nil {
		return "", nil, false, rerr
	}
	return filepath.ToSlash(rel), warnings, true, nil
}

func readImportedOpenCodeConfigFromFile(path string, displayPath string) (importedOpenCodeConfig, string, []pluginmodel.Warning, bool, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return importedOpenCodeConfig{}, "", nil, false, err
	}
	data, err := decodeImportedOpenCodeConfig(body)
	if err != nil {
		return importedOpenCodeConfig{}, displayPath, nil, false, err
	}
	if strings.TrimSpace(displayPath) == "" {
		displayPath = filepath.ToSlash(path)
	}
	return data, displayPath, nil, true, nil
}
