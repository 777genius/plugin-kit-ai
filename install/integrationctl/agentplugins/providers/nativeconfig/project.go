package nativeconfig

import (
	"fmt"
	"strings"
)

func projectServer(codec Codec, server Server, placeholders Placeholders) (map[string]any, error) {
	serverType := strings.ToLower(strings.TrimSpace(server.Type))
	if serverType != "stdio" && serverType != "remote" {
		return nil, fmt.Errorf("MCP server type must be stdio or remote")
	}
	resolve := func(value string) (string, error) {
		replacements := []struct{ token, value string }{
			{"${PLUGIN_ROOT}", placeholders.PackageRoot},
			{"${package.root}", placeholders.PackageRoot},
			{"${PLUGIN_DATA}", placeholders.DataRoot},
		}
		for _, replacement := range replacements {
			if strings.Contains(value, replacement.token) {
				if strings.TrimSpace(replacement.value) == "" {
					return "", fmt.Errorf("explicit value for %s is required", replacement.token)
				}
				value = strings.ReplaceAll(value, replacement.token, replacement.value)
			}
		}
		if strings.Contains(value, "${") {
			return "", fmt.Errorf("unresolved placeholder in %q", value)
		}
		return value, nil
	}
	projectStrings := func(values []string) ([]string, error) {
		out := make([]string, len(values))
		for i, value := range values {
			resolved, err := resolve(value)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	}
	projectMap := func(values map[string]string) (map[string]string, error) {
		if len(values) == 0 {
			return nil, nil
		}
		out := make(map[string]string, len(values))
		for key, value := range values {
			resolvedKey, err := resolve(key)
			if err != nil {
				return nil, err
			}
			resolvedValue, err := resolve(value)
			if err != nil {
				return nil, err
			}
			if _, exists := out[resolvedKey]; exists {
				return nil, fmt.Errorf("placeholder expansion produced duplicate key %q", resolvedKey)
			}
			out[resolvedKey] = resolvedValue
		}
		return out, nil
	}

	if serverType == "stdio" {
		command, err := resolve(server.Command)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(command) == "" || server.URL != "" || len(server.Headers) > 0 || server.RemoteTransport != "" {
			return nil, fmt.Errorf("stdio MCP server requires command and forbids remote fields")
		}
		args, err := projectStrings(server.Args)
		if err != nil {
			return nil, err
		}
		env, err := projectMap(server.Env)
		if err != nil {
			return nil, err
		}
		if codec == CodecOpenCode {
			entry := map[string]any{"type": "local", "command": append([]string{command}, args...)}
			if len(env) > 0 {
				entry["environment"] = env
			}
			return entry, nil
		}
		entry := map[string]any{"command": command}
		if len(args) > 0 {
			entry["args"] = args
		}
		if len(env) > 0 {
			entry["env"] = env
		}
		if server.CWD != "" {
			cwd, err := resolve(server.CWD)
			if err != nil {
				return nil, err
			}
			entry["cwd"] = cwd
		}
		return entry, nil
	}

	url, err := resolve(server.URL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(url) == "" || server.Command != "" || len(server.Args) > 0 || len(server.Env) > 0 || server.CWD != "" {
		return nil, fmt.Errorf("remote MCP server requires url and forbids local process fields")
	}
	headers, err := projectMap(server.Headers)
	if err != nil {
		return nil, err
	}
	urlKey := "url"
	if codec == CodecGemini {
		switch server.RemoteTransport {
		case "streamable-http":
			urlKey = "httpUrl"
		case "sse":
			urlKey = "url"
		default:
			return nil, fmt.Errorf("Gemini remote MCP server requires streamable-http or sse transport")
		}
	}
	entry := map[string]any{urlKey: url}
	if codec == CodecOpenCode {
		entry["type"] = "remote"
	}
	if len(headers) > 0 {
		entry["headers"] = headers
	}
	return entry, nil
}
