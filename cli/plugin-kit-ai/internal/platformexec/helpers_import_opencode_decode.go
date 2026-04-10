package platformexec

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tailscale/hujson"
)

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
