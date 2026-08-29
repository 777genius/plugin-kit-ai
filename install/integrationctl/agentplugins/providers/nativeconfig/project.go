package nativeconfig

import (
	"fmt"
	"strings"

	"github.com/tailscale/hujson"
)

// DesiredReceipt projects one exact server entry without reading or mutating a
// config file. resolvedPath must be the already-selected clean absolute JSON or
// JSONC path. Apply returns the same receipt when it applies the same request.
func DesiredReceipt(resolvedPath string, codec Codec, name string, server Server, placeholders Placeholders) (Receipt, error) {
	if err := validateExactPath(resolvedPath, "resolved native config path"); err != nil {
		return Receipt{}, err
	}
	if strings.TrimSpace(name) == "" {
		return Receipt{}, fmt.Errorf("MCP entry name is required")
	}
	if !supportedCodec(codec) {
		return Receipt{}, fmt.Errorf("unsupported native config codec %q", codec)
	}
	projected, err := projectServer(codec, server, placeholders)
	if err != nil {
		return Receipt{}, err
	}
	value, err := jsonValue(projected)
	if err != nil {
		return Receipt{}, err
	}
	member := &hujson.ObjectMember{Value: value}
	digest, err := entryDigest(codec, name, member)
	if err != nil {
		return Receipt{}, err
	}
	return Receipt{Version: "1", Path: resolvedPath, Codec: codec, Name: name, Digest: digest}, nil
}

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
			if server.CWD != "" {
				return nil, fmt.Errorf("OpenCode local MCP server does not accept cwd")
			}
			entry := map[string]any{"type": "local", "command": append([]string{command}, args...)}
			if len(env) > 0 {
				entry["environment"] = env
			}
			return entry, nil
		}
		if (codec == CodecWindsurf || codec == CodecCline) && server.CWD != "" {
			return nil, fmt.Errorf("%s stdio MCP server does not accept cwd", codec)
		}
		if codec == CodecCline {
			transport := map[string]any{"type": "stdio", "command": command}
			if len(args) > 0 {
				transport["args"] = args
			}
			if len(env) > 0 {
				transport["env"] = env
			}
			return map[string]any{"transport": transport}, nil
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
	} else if codec == CodecWindsurf {
		switch server.RemoteTransport {
		case "streamable-http":
			urlKey = "serverUrl"
		case "sse":
			urlKey = "url"
		default:
			return nil, fmt.Errorf("Windsurf remote MCP server requires streamable-http or sse transport")
		}
	} else if codec == CodecCline {
		transportType := ""
		switch server.RemoteTransport {
		case "streamable-http":
			transportType = "streamableHttp"
		case "sse":
			transportType = "sse"
		default:
			return nil, fmt.Errorf("Cline remote MCP server requires streamable-http or sse transport")
		}
		transport := map[string]any{"type": transportType, "url": url}
		if len(headers) > 0 {
			transport["headers"] = headers
		}
		return map[string]any{"transport": transport}, nil
	} else if server.RemoteTransport != "" {
		return nil, fmt.Errorf("%s remote MCP server does not accept remote_transport", codec)
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
