package pluginmodel

import (
	"fmt"
	"os"
	"strings"
)

func (m *PortableMCP) RenderForTarget(target string) (map[string]any, error) {
	if m == nil {
		return nil, nil
	}
	if m.File == nil {
		return nil, fmt.Errorf("portable MCP model is missing typed file data")
	}
	return m.File.RenderLegacyProjection(target), nil
}

func (file PortableMCPFile) RenderLegacyProjection(target string) map[string]any {
	target = NormalizeTarget(target)
	out := map[string]any{}
	for alias, server := range file.Servers {
		if !server.appliesTo(target) {
			continue
		}
		generated := server.generate(target)
		if len(generated) == 0 {
			continue
		}
		out[alias] = generated
	}
	return out
}

func (server PortableMCPServer) appliesTo(target string) bool {
	if target == "" || len(server.Targets) == 0 {
		return true
	}
	for _, entry := range server.Targets {
		if entry == target {
			return true
		}
	}
	return false
}

func (server PortableMCPServer) generate(target string) map[string]any {
	var out map[string]any
	switch server.Type {
	case "stdio":
		out = renderPortableMCPStdio(target, server.Stdio)
	case "remote":
		out = renderPortableMCPRemote(target, server.Remote)
	default:
		return nil
	}
	if target != "" {
		if override := server.Overrides[NormalizeTarget(target)]; len(override) > 0 {
			mergeExtraObject(out, cloneAnyMap(override))
		}
		if passthrough := server.Passthrough[NormalizeTarget(target)]; len(passthrough) > 0 {
			mergeExtraObject(out, cloneAnyMap(passthrough))
		}
	}
	generated, ok := translatePortableMCPValue(target, out).(map[string]any)
	if !ok {
		return out
	}
	return generated
}

func renderPortableMCPStdio(target string, stdio *PortableMCPStdio) map[string]any {
	if stdio == nil {
		return nil
	}
	target = NormalizeTarget(target)
	switch target {
	case "opencode":
		command := make([]any, 0, 1+len(stdio.Args))
		command = append(command, stdio.Command)
		for _, arg := range stdio.Args {
			command = append(command, arg)
		}
		out := map[string]any{
			"type":    "local",
			"command": command,
		}
		if len(stdio.Env) > 0 {
			out["environment"] = stringMapToAny(stdio.Env)
		}
		return out
	default:
		out := map[string]any{
			"command": stdio.Command,
		}
		if len(stdio.Args) > 0 {
			args := make([]any, 0, len(stdio.Args))
			for _, arg := range stdio.Args {
				args = append(args, arg)
			}
			out["args"] = args
		}
		if len(stdio.Env) > 0 {
			out["env"] = stringMapToAny(stdio.Env)
		}
		return out
	}
}

func renderPortableMCPRemote(target string, remote *PortableMCPRemote) map[string]any {
	if remote == nil {
		return nil
	}
	target = NormalizeTarget(target)
	switch target {
	case "gemini":
		out := map[string]any{}
		if remote.Protocol == "sse" {
			out["url"] = remote.URL
		} else {
			out["httpUrl"] = remote.URL
		}
		if len(remote.Headers) > 0 {
			out["headers"] = stringMapToAny(remote.Headers)
		}
		return out
	case "opencode":
		out := map[string]any{
			"type": "remote",
			"url":  remote.URL,
		}
		if len(remote.Headers) > 0 {
			out["headers"] = stringMapToAny(remote.Headers)
		}
		return out
	default:
		kind := "http"
		if remote.Protocol == "sse" {
			kind = "sse"
		}
		out := map[string]any{
			"type": kind,
			"url":  remote.URL,
		}
		if len(remote.Headers) > 0 {
			out["headers"] = stringMapToAny(remote.Headers)
		}
		return out
	}
}

func translatePortableMCPValue(target string, value any) any {
	switch typed := value.(type) {
	case string:
		return translatePortableMCPString(target, typed)
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, entry := range typed {
			out[key] = translatePortableMCPValue(target, entry)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, entry := range typed {
			out = append(out, translatePortableMCPValue(target, entry))
		}
		return out
	default:
		return value
	}
}

func translatePortableMCPString(target, value string) string {
	replacements := portableMCPVariableReplacements(target)
	for from, to := range replacements {
		value = strings.ReplaceAll(value, from, to)
	}
	return value
}

func portableMCPVariableReplacements(target string) map[string]string {
	target = NormalizeTarget(target)
	packageRoot := "."
	switch target {
	case "gemini":
		packageRoot = "${extensionPath}"
	case "cursor-workspace":
		packageRoot = "${workspaceFolder}"
	}
	return map[string]string{
		"${package.root}": packageRoot,
		"${path.sep}":     string(os.PathSeparator),
	}
}

func mustPortableMCPMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}
