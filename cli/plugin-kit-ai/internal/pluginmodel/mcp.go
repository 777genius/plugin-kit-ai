package pluginmodel

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
	"gopkg.in/yaml.v3"
)

const PortableMCPLegacyFormatMarker = "plugin-kit-ai/mcp"

var portableMCPAliasRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type PortableMCPFile struct {
	APIVersion string                       `yaml:"api_version,omitempty" json:"api_version,omitempty"`
	Format     string                       `yaml:"format,omitempty" json:"format,omitempty"`
	Version    int                          `yaml:"version,omitempty" json:"version,omitempty"`
	Servers    map[string]PortableMCPServer `yaml:"servers" json:"servers"`
}

type PortableMCPServer struct {
	Description string                    `yaml:"description,omitempty" json:"description,omitempty"`
	Type        string                    `yaml:"type,omitempty" json:"type,omitempty"`
	Stdio       *PortableMCPStdio         `yaml:"stdio,omitempty" json:"stdio,omitempty"`
	Remote      *PortableMCPRemote        `yaml:"remote,omitempty" json:"remote,omitempty"`
	Targets     []string                  `yaml:"targets,omitempty" json:"targets,omitempty"`
	Overrides   map[string]map[string]any `yaml:"overrides,omitempty" json:"overrides,omitempty"`
	Passthrough map[string]map[string]any `yaml:"passthrough,omitempty" json:"passthrough,omitempty"`
}

type PortableMCPStdio struct {
	Command string            `yaml:"command,omitempty" json:"command,omitempty"`
	Args    []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
}

type PortableMCPRemote struct {
	Protocol string            `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	URL      string            `yaml:"url,omitempty" json:"url,omitempty"`
	Headers  map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
}

type ParsedPortableMCP struct {
	Servers map[string]any
	File    *PortableMCPFile
}

func ParsePortableMCP(rel string, body []byte) (ParsedPortableMCP, error) {
	raw := map[string]any{}
	switch strings.ToLower(filepathExt(rel)) {
	case ".json":
		if err := json.Unmarshal(body, &raw); err != nil {
			return ParsedPortableMCP{}, fmt.Errorf("parse %s: %w", rel, err)
		}
	default:
		if err := yaml.Unmarshal(body, &raw); err != nil {
			return ParsedPortableMCP{}, fmt.Errorf("parse %s: %w", rel, err)
		}
	}
	if raw == nil {
		raw = map[string]any{}
	}
	_, hasAPIVersion := raw["api_version"]
	_, hasFormat := raw["format"]
	_, hasVersion := raw["version"]
	switch {
	case hasAPIVersion:
		if hasFormat || hasVersion {
			return ParsedPortableMCP{}, fmt.Errorf("portable MCP file %s must not mix api_version with legacy format/version markers", rel)
		}
	case hasFormat || hasVersion:
		if !hasFormat {
			return ParsedPortableMCP{}, fmt.Errorf("portable MCP file %s legacy schema must declare %q", rel, PortableMCPLegacyFormatMarker)
		}
		if !hasVersion {
			return ParsedPortableMCP{}, fmt.Errorf("portable MCP file %s legacy schema must declare version", rel)
		}
	default:
		return ParsedPortableMCP{}, fmt.Errorf("portable MCP file %s must declare api_version", rel)
	}
	return parsePortableMCPFile(rel, body)
}

func parsePortableMCPFile(rel string, body []byte) (ParsedPortableMCP, error) {
	var file PortableMCPFile
	switch strings.ToLower(filepathExt(rel)) {
	case ".json":
		if err := json.Unmarshal(body, &file); err != nil {
			return ParsedPortableMCP{}, fmt.Errorf("parse %s: %w", rel, err)
		}
	default:
		if err := yaml.Unmarshal(body, &file); err != nil {
			return ParsedPortableMCP{}, fmt.Errorf("parse %s: %w", rel, err)
		}
	}
	normalizePortableMCPFile(&file)
	if err := file.Validate(); err != nil {
		return ParsedPortableMCP{}, fmt.Errorf("parse %s: %w", rel, err)
	}
	return ParsedPortableMCP{
		Servers: mustPortableMCPMap(file.RenderLegacyProjection("")),
		File:    &file,
	}, nil
}

func normalizePortableMCPFile(file *PortableMCPFile) {
	file.APIVersion = strings.TrimSpace(file.APIVersion)
	file.Format = strings.TrimSpace(file.Format)
	if file.Servers == nil {
		file.Servers = map[string]PortableMCPServer{}
	}
	for alias, server := range file.Servers {
		server.Description = strings.TrimSpace(server.Description)
		server.Type = strings.ToLower(strings.TrimSpace(server.Type))
		if server.Stdio != nil {
			server.Stdio.Command = strings.TrimSpace(server.Stdio.Command)
			server.Stdio.Args = normalizeStringSlice(server.Stdio.Args)
			server.Stdio.Env = normalizeStringMap(server.Stdio.Env)
		}
		if server.Remote != nil {
			server.Remote.Protocol = normalizeRemoteProtocol(server.Remote.Protocol)
			server.Remote.URL = strings.TrimSpace(server.Remote.URL)
			server.Remote.Headers = normalizeStringMap(server.Remote.Headers)
		}
		server.Targets = normalizePortableMCPTargets(server.Targets)
		server.Overrides = normalizePortableMCPObjectMap(server.Overrides)
		server.Passthrough = normalizePortableMCPObjectMap(server.Passthrough)
		file.Servers[alias] = server
	}
}

func normalizeStringSlice(values []string) []string {
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

func normalizeStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizePortableMCPTargets(values []string) []string {
	var out []string
	for _, value := range values {
		target := NormalizeTarget(value)
		if target == "" {
			continue
		}
		out = append(out, target)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func normalizePortableMCPObjectMap(values map[string]map[string]any) map[string]map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]map[string]any, len(values))
	for key, value := range values {
		target := NormalizeTarget(key)
		if target == "" || len(value) == 0 {
			continue
		}
		out[target] = cloneAnyMap(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRemoteProtocol(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "http":
		return "streamable_http"
	default:
		return value
	}
}

func (file PortableMCPFile) Validate() error {
	switch {
	case file.APIVersion != "":
		if file.APIVersion != APIVersionV1 {
			return fmt.Errorf("portable MCP api_version must be %q", APIVersionV1)
		}
		if file.Format != "" || file.Version != 0 {
			return fmt.Errorf("portable MCP api_version may not be mixed with legacy format/version markers")
		}
	case file.Format != "" || file.Version != 0:
		if file.Format != PortableMCPLegacyFormatMarker {
			return fmt.Errorf("portable MCP legacy format must be %q", PortableMCPLegacyFormatMarker)
		}
		if file.Version != 1 {
			return fmt.Errorf("portable MCP legacy version %d is unsupported", file.Version)
		}
	default:
		return fmt.Errorf("portable MCP api_version must be %q", APIVersionV1)
	}
	if len(file.Servers) == 0 {
		return fmt.Errorf("portable MCP servers must not be empty")
	}
	supportedTargets := setOf(platformmeta.IDs())
	for alias, server := range file.Servers {
		if !portableMCPAliasRe.MatchString(strings.TrimSpace(alias)) {
			return fmt.Errorf("portable MCP server %q must use lowercase letters, digits, and hyphens only", alias)
		}
		switch server.Type {
		case "stdio":
			if server.Stdio == nil {
				return fmt.Errorf("portable MCP server %q type stdio requires stdio config", alias)
			}
			if strings.TrimSpace(server.Stdio.Command) == "" {
				return fmt.Errorf("portable MCP server %q type stdio requires stdio.command", alias)
			}
			if server.Remote != nil {
				return fmt.Errorf("portable MCP server %q type stdio may not define remote config", alias)
			}
		case "remote":
			if server.Remote == nil {
				return fmt.Errorf("portable MCP server %q type remote requires remote config", alias)
			}
			if strings.TrimSpace(server.Remote.URL) == "" {
				return fmt.Errorf("portable MCP server %q type remote requires remote.url", alias)
			}
			switch server.Remote.Protocol {
			case "streamable_http", "sse":
			default:
				return fmt.Errorf("portable MCP server %q remote.protocol must be streamable_http or sse", alias)
			}
			if server.Stdio != nil {
				return fmt.Errorf("portable MCP server %q type remote may not define stdio config", alias)
			}
		default:
			return fmt.Errorf("portable MCP server %q type must be stdio or remote", alias)
		}
		if len(server.Targets) == 0 {
		} else {
			for _, target := range server.Targets {
				if !supportedTargets[target] {
					return fmt.Errorf("portable MCP server %q targets contains unsupported target %q", alias, target)
				}
			}
		}
		for target := range server.Overrides {
			if !supportedTargets[target] {
				return fmt.Errorf("portable MCP server %q overrides contains unsupported target %q", alias, target)
			}
			if managed := portableMCPManagedKeys(server, target); hasManagedConflict(server.Overrides[target], managed) {
				return fmt.Errorf("portable MCP server %q overrides for target %q may not override managed MCP projection keys", alias, target)
			}
		}
		for target := range server.Passthrough {
			if !supportedTargets[target] {
				return fmt.Errorf("portable MCP server %q passthrough contains unsupported target %q", alias, target)
			}
			if managed := portableMCPManagedKeys(server, target); hasManagedConflict(server.Passthrough[target], managed) {
				return fmt.Errorf("portable MCP server %q passthrough for target %q may not override managed MCP projection keys", alias, target)
			}
		}
	}
	return nil
}

func portableMCPManagedKeys(server PortableMCPServer, target string) map[string]struct{} {
	target = NormalizeTarget(target)
	keys := map[string]struct{}{}
	switch server.Type {
	case "stdio":
		switch target {
		case "opencode":
			keys["type"] = struct{}{}
			keys["command"] = struct{}{}
			keys["environment"] = struct{}{}
		default:
			keys["command"] = struct{}{}
			keys["args"] = struct{}{}
			keys["env"] = struct{}{}
		}
	case "remote":
		switch target {
		case "gemini":
			if server.Remote != nil && server.Remote.Protocol == "sse" {
				keys["url"] = struct{}{}
			} else {
				keys["httpUrl"] = struct{}{}
			}
			keys["headers"] = struct{}{}
		case "opencode":
			keys["type"] = struct{}{}
			keys["url"] = struct{}{}
			keys["headers"] = struct{}{}
		default:
			keys["type"] = struct{}{}
			keys["url"] = struct{}{}
			keys["headers"] = struct{}{}
		}
	}
	return keys
}

func hasManagedConflict(extra map[string]any, managed map[string]struct{}) bool {
	for key := range extra {
		if _, ok := managed[key]; ok {
			return true
		}
	}
	return false
}

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

func stringMapToAny(values map[string]string) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = cloneAnyValue(value)
	}
	return out
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, cloneAnyValue(item))
		}
		return out
	default:
		return typed
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
	case "cursor":
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

func MarshalPortableMCPFile(file PortableMCPFile) ([]byte, error) {
	canonicalizePortableMCPFile(&file)
	normalizePortableMCPFile(&file)
	if err := file.Validate(); err != nil {
		return nil, err
	}
	body, err := yaml.Marshal(file)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func ImportedPortableMCPYAML(sourceTarget string, servers map[string]any) ([]byte, error) {
	file, err := PortableMCPFileFromNative(sourceTarget, servers)
	if err != nil {
		return nil, err
	}
	return MarshalPortableMCPFile(file)
}

func PortableMCPFileFromNative(sourceTarget string, servers map[string]any) (PortableMCPFile, error) {
	target := NormalizeTarget(sourceTarget)
	file := PortableMCPFile{
		APIVersion: APIVersionV1,
		Servers:    map[string]PortableMCPServer{},
	}
	for alias, raw := range servers {
		serverMap, ok := raw.(map[string]any)
		if !ok {
			return PortableMCPFile{}, fmt.Errorf("native MCP server %q must be a JSON object", alias)
		}
		server, err := portableMCPServerFromNative(alias, target, cloneAnyMap(serverMap))
		if err != nil {
			return PortableMCPFile{}, err
		}
		file.Servers[alias] = server
	}
	normalizePortableMCPFile(&file)
	if err := file.Validate(); err != nil {
		return PortableMCPFile{}, err
	}
	return file, nil
}

func canonicalizePortableMCPFile(file *PortableMCPFile) {
	file.APIVersion = APIVersionV1
	file.Format = ""
	file.Version = 0
}

func portableMCPServerFromNative(alias, target string, raw map[string]any) (PortableMCPServer, error) {
	server := PortableMCPServer{}
	if target != "" {
		server.Targets = []string{target}
	}
	if target == "opencode" {
		if kind, _ := raw["type"].(string); strings.EqualFold(strings.TrimSpace(kind), "local") {
			command, err := anyStringSlice(raw["command"])
			if err != nil || len(command) == 0 {
				return PortableMCPServer{}, fmt.Errorf("native OpenCode MCP server %q local command must be a non-empty string array", alias)
			}
			server.Type = "stdio"
			server.Stdio = &PortableMCPStdio{
				Command: command[0],
				Args:    append([]string(nil), command[1:]...),
				Env:     anyToStringMap(raw["environment"]),
			}
			delete(raw, "type")
			delete(raw, "command")
			delete(raw, "environment")
			return withPortableMCPPassthrough(server, target, raw), nil
		}
		if kind, _ := raw["type"].(string); strings.EqualFold(strings.TrimSpace(kind), "remote") {
			server.Type = "remote"
			server.Remote = &PortableMCPRemote{
				Protocol: "streamable_http",
				URL:      optionalAnyString(raw["url"]),
				Headers:  anyToStringMap(raw["headers"]),
			}
			delete(raw, "type")
			delete(raw, "url")
			delete(raw, "headers")
			return withPortableMCPPassthrough(server, target, raw), nil
		}
	}
	if httpURL := optionalAnyString(raw["httpUrl"]); httpURL != "" {
		server.Type = "remote"
		server.Remote = &PortableMCPRemote{
			Protocol: "streamable_http",
			URL:      httpURL,
			Headers:  anyToStringMap(raw["headers"]),
		}
		delete(raw, "httpUrl")
		delete(raw, "headers")
		return withPortableMCPPassthrough(server, target, raw), nil
	}
	if command := optionalAnyString(raw["command"]); command != "" {
		server.Type = "stdio"
		server.Stdio = &PortableMCPStdio{
			Command: command,
			Args:    mustStringSlice(raw["args"]),
			Env:     anyToStringMap(raw["env"]),
		}
		delete(raw, "command")
		delete(raw, "args")
		delete(raw, "env")
		return withPortableMCPPassthrough(server, target, raw), nil
	}
	if url := optionalAnyString(raw["url"]); url != "" {
		protocol := "streamable_http"
		switch strings.ToLower(strings.TrimSpace(optionalAnyString(raw["type"]))) {
		case "sse":
			protocol = "sse"
		case "http":
			protocol = "streamable_http"
		case "remote":
			protocol = "streamable_http"
		}
		if target == "gemini" {
			protocol = "sse"
		}
		server.Type = "remote"
		server.Remote = &PortableMCPRemote{
			Protocol: protocol,
			URL:      url,
			Headers:  anyToStringMap(raw["headers"]),
		}
		delete(raw, "type")
		delete(raw, "url")
		delete(raw, "headers")
		return withPortableMCPPassthrough(server, target, raw), nil
	}
	return PortableMCPServer{}, fmt.Errorf("native MCP server %q could not be normalized into portable MCP v1", alias)
}

func withPortableMCPPassthrough(server PortableMCPServer, target string, raw map[string]any) PortableMCPServer {
	if len(raw) == 0 || target == "" {
		return server
	}
	if server.Passthrough == nil {
		server.Passthrough = map[string]map[string]any{}
	}
	server.Passthrough[target] = raw
	return server
}

func optionalAnyString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func anyToStringMap(value any) map[string]string {
	switch typed := value.(type) {
	case map[string]string:
		return normalizeStringMap(typed)
	case map[string]any:
		out := map[string]string{}
		for key, raw := range typed {
			text := optionalAnyString(raw)
			if strings.TrimSpace(key) == "" || text == "" {
				continue
			}
			out[key] = text
		}
		return normalizeStringMap(out)
	default:
		return nil
	}
}

func anyStringSlice(value any) ([]string, error) {
	switch typed := value.(type) {
	case []string:
		return normalizeStringSlice(typed), nil
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := optionalAnyString(item)
			if text == "" {
				return nil, fmt.Errorf("string array contains an empty or non-string item")
			}
			out = append(out, text)
		}
		return normalizeStringSlice(out), nil
	default:
		return nil, fmt.Errorf("expected string array")
	}
}

func mustStringSlice(value any) []string {
	out, err := anyStringSlice(value)
	if err != nil {
		return nil
	}
	return out
}

func filepathExt(path string) string {
	idx := strings.LastIndex(path, ".")
	if idx < 0 {
		return ""
	}
	return path[idx:]
}
