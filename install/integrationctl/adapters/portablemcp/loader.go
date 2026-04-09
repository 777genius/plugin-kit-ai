package portablemcp

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
	"gopkg.in/yaml.v3"
)

var aliasRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Loader struct {
	FS ports.FileSystem
}

type File struct {
	APIVersion string            `yaml:"api_version"`
	Servers    map[string]Server `yaml:"servers"`
}

type Server struct {
	Type    string   `yaml:"type"`
	Targets []string `yaml:"targets"`
	Stdio   *Stdio   `yaml:"stdio,omitempty"`
	Remote  *Remote  `yaml:"remote,omitempty"`
}

type Stdio struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
}

type Remote struct {
	Protocol string            `yaml:"protocol,omitempty"`
	URL      string            `yaml:"url"`
	Headers  map[string]string `yaml:"headers,omitempty"`
}

type Loaded struct {
	Path    string
	File    File
	Servers map[string]Server
}

func (l Loader) LoadForTarget(ctx context.Context, root string, target domain.TargetID) (Loaded, error) {
	path, body, err := l.readPortableMCP(ctx, root)
	if err != nil {
		return Loaded{}, err
	}
	var file File
	if err := yaml.Unmarshal(body, &file); err != nil {
		return Loaded{}, domain.NewError(domain.ErrManifestLoad, "parse portable MCP", err)
	}
	file.APIVersion = strings.TrimSpace(file.APIVersion)
	if file.APIVersion != "v1" {
		return Loaded{}, domain.NewError(domain.ErrManifestLoad, "portable MCP api_version must be v1", nil)
	}
	if len(file.Servers) == 0 {
		return Loaded{}, domain.NewError(domain.ErrManifestLoad, "portable MCP servers must not be empty", nil)
	}
	servers := map[string]Server{}
	for alias, server := range file.Servers {
		alias = strings.TrimSpace(alias)
		if !aliasRE.MatchString(alias) {
			return Loaded{}, domain.NewError(domain.ErrManifestLoad, fmt.Sprintf("portable MCP alias %q is invalid", alias), nil)
		}
		normalized, err := normalizeServer(alias, server)
		if err != nil {
			return Loaded{}, err
		}
		if matchesTarget(normalized.Targets, target) {
			servers[alias] = normalized
		}
	}
	if len(servers) == 0 {
		return Loaded{}, domain.NewError(domain.ErrManifestLoad, "portable MCP does not define any servers for "+string(target), nil)
	}
	return Loaded{Path: path, File: file, Servers: servers}, nil
}

func normalizeServer(alias string, server Server) (Server, error) {
	server.Type = strings.ToLower(strings.TrimSpace(server.Type))
	server.Targets = normalizeTargets(server.Targets)
	switch server.Type {
	case "stdio":
		if server.Stdio == nil {
			return Server{}, domain.NewError(domain.ErrManifestLoad, fmt.Sprintf("portable MCP server %q requires stdio config", alias), nil)
		}
		server.Stdio.Command = strings.TrimSpace(server.Stdio.Command)
		server.Stdio.Args = normalizeStrings(server.Stdio.Args)
		server.Stdio.Env = normalizeStringMap(server.Stdio.Env)
		if server.Stdio.Command == "" {
			return Server{}, domain.NewError(domain.ErrManifestLoad, fmt.Sprintf("portable MCP server %q requires stdio.command", alias), nil)
		}
		if server.Remote != nil {
			return Server{}, domain.NewError(domain.ErrManifestLoad, fmt.Sprintf("portable MCP server %q type stdio may not define remote config", alias), nil)
		}
	case "remote":
		if server.Remote == nil {
			return Server{}, domain.NewError(domain.ErrManifestLoad, fmt.Sprintf("portable MCP server %q requires remote config", alias), nil)
		}
		server.Remote.Protocol = strings.ToLower(strings.TrimSpace(server.Remote.Protocol))
		server.Remote.URL = strings.TrimSpace(server.Remote.URL)
		server.Remote.Headers = normalizeStringMap(server.Remote.Headers)
		if server.Remote.URL == "" {
			return Server{}, domain.NewError(domain.ErrManifestLoad, fmt.Sprintf("portable MCP server %q requires remote.url", alias), nil)
		}
		if server.Stdio != nil {
			return Server{}, domain.NewError(domain.ErrManifestLoad, fmt.Sprintf("portable MCP server %q type remote may not define stdio config", alias), nil)
		}
	default:
		return Server{}, domain.NewError(domain.ErrManifestLoad, fmt.Sprintf("portable MCP server %q type must be stdio or remote", alias), nil)
	}
	return server, nil
}

func (l Loader) readPortableMCP(ctx context.Context, root string) (string, []byte, error) {
	candidates := []string{
		filepath.Join(root, "src", "mcp", "servers.yaml"),
		filepath.Join(root, "mcp", "servers.yaml"),
	}
	for _, path := range candidates {
		body, err := l.FS.ReadFile(ctx, path)
		if err == nil {
			return path, body, nil
		}
	}
	return "", nil, domain.NewError(domain.ErrManifestLoad, "portable MCP file not found", nil)
}

func normalizeTargets(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	sort.Strings(out)
	uniq := out[:0]
	for _, value := range out {
		if len(uniq) == 0 || uniq[len(uniq)-1] != value {
			uniq = append(uniq, value)
		}
	}
	return uniq
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
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

func matchesTarget(targets []string, target domain.TargetID) bool {
	if len(targets) == 0 {
		return true
	}
	want := string(target)
	for _, item := range targets {
		if item == want {
			return true
		}
	}
	return false
}
