package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/portablemcp"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"gopkg.in/yaml.v3"
)

func (a Adapter) loadSourceMaterial(ctx context.Context, sourceRoot, scope string, workspaceRoot string) (sourceMaterial, error) {
	fields := map[string]any{
		"$schema": "https://opencode.ai/config.json",
	}
	plugins, err := readPlugins(filepath.Join(sourceRoot, "src", "targets", "opencode", "package.yaml"))
	if err != nil {
		return sourceMaterial{}, err
	}
	loader := portablemcp.Loader{FS: a.fs()}
	if loaded, err := loader.LoadForTarget(ctx, sourceRoot, domain.TargetOpenCode); err == nil {
		projected := renderOpenCodeMCP(loaded, sourceRoot)
		if len(projected) == 0 {
			projected = nil
		}
		material := sourceMaterial{WholeFields: fields, Plugins: plugins, MCP: projected}
		if err := material.loadFirstClassDocs(sourceRoot); err != nil {
			return sourceMaterial{}, err
		}
		extra, err := readConfigExtra(filepath.Join(sourceRoot, "src", "targets", "opencode", "config.extra.json"))
		if err != nil {
			return sourceMaterial{}, err
		}
		if err := material.mergeExtra(extra); err != nil {
			return sourceMaterial{}, err
		}
		copyFiles, err := collectCopyFiles(sourceRoot, a.assetsRoot(scope, workspaceRoot))
		if err != nil {
			return sourceMaterial{}, err
		}
		material.CopyFiles = copyFiles
		return material, nil
	} else if !isMissingPortableMCP(err) {
		return sourceMaterial{}, err
	}
	material := sourceMaterial{WholeFields: fields, Plugins: plugins}
	if err := material.loadFirstClassDocs(sourceRoot); err != nil {
		return sourceMaterial{}, err
	}
	extra, err := readConfigExtra(filepath.Join(sourceRoot, "src", "targets", "opencode", "config.extra.json"))
	if err != nil {
		return sourceMaterial{}, err
	}
	if err := material.mergeExtra(extra); err != nil {
		return sourceMaterial{}, err
	}
	copyFiles, err := collectCopyFiles(sourceRoot, a.assetsRoot(scope, workspaceRoot))
	if err != nil {
		return sourceMaterial{}, err
	}
	material.CopyFiles = copyFiles
	return material, nil
}

func renderOpenCodeMCP(loaded portablemcp.Loaded, sourceRoot string) map[string]any {
	out := make(map[string]any, len(loaded.Servers))
	for alias, server := range loaded.Servers {
		switch server.Type {
		case "stdio":
			command := make([]any, 0, 1+len(server.Stdio.Args))
			command = append(command, interpolatePackageRoot(server.Stdio.Command, sourceRoot))
			for _, arg := range server.Stdio.Args {
				command = append(command, interpolatePackageRoot(arg, sourceRoot))
			}
			entry := map[string]any{
				"type":    "local",
				"command": command,
			}
			if len(server.Stdio.Env) > 0 {
				env := make(map[string]any, len(server.Stdio.Env))
				for key, value := range server.Stdio.Env {
					env[key] = interpolatePackageRoot(value, sourceRoot)
				}
				entry["environment"] = env
			}
			out[alias] = entry
		case "remote":
			entry := map[string]any{
				"type": "remote",
				"url":  interpolatePackageRoot(server.Remote.URL, sourceRoot),
			}
			if len(server.Remote.Headers) > 0 {
				headers := make(map[string]any, len(server.Remote.Headers))
				for key, value := range server.Remote.Headers {
					headers[key] = interpolatePackageRoot(value, sourceRoot)
				}
				entry["headers"] = headers
			}
			out[alias] = entry
		}
	}
	return out
}

func (a Adapter) copyOwnedFiles(files []copyFile) ([]string, error) {
	if len(files) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(files))
	for _, item := range files {
		body, err := os.ReadFile(item.Source)
		if err != nil {
			return nil, domain.NewError(domain.ErrMutationApply, "read OpenCode source asset", err)
		}
		if err := a.fs().WriteFileAtomic(context.Background(), item.Destination, body, 0o644); err != nil {
			return nil, domain.NewError(domain.ErrMutationApply, "write OpenCode projected asset", err)
		}
		out = append(out, item.Destination)
	}
	sort.Strings(out)
	return out, nil
}

func collectCopyFiles(sourceRoot, assetsRoot string) ([]copyFile, error) {
	type pair struct{ src, dst string }
	pairs := []pair{
		{filepath.Join(sourceRoot, "src", "targets", "opencode", "commands"), filepath.Join(assetsRoot, "commands")},
		{filepath.Join(sourceRoot, "src", "targets", "opencode", "agents"), filepath.Join(assetsRoot, "agents")},
		{filepath.Join(sourceRoot, "src", "targets", "opencode", "themes"), filepath.Join(assetsRoot, "themes")},
		{filepath.Join(sourceRoot, "src", "targets", "opencode", "tools"), filepath.Join(assetsRoot, "tools")},
		{filepath.Join(sourceRoot, "src", "targets", "opencode", "plugins"), filepath.Join(assetsRoot, "plugins")},
		{filepath.Join(sourceRoot, "src", "skills"), filepath.Join(assetsRoot, "skills")},
	}
	var out []copyFile
	for _, pair := range pairs {
		if _, err := os.Stat(pair.src); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		err := filepath.WalkDir(pair.src, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(pair.src, path)
			if err != nil {
				return err
			}
			out = append(out, copyFile{
				Source:      path,
				Destination: filepath.Join(pair.dst, rel),
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if pkg := filepath.Join(sourceRoot, "src", "targets", "opencode", "package.json"); fileExists(pkg) {
		out = append(out, copyFile{Source: pkg, Destination: filepath.Join(assetsRoot, "package.json")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Destination < out[j].Destination })
	return out, nil
}

func readPlugins(path string) ([]pluginRef, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, domain.NewError(domain.ErrManifestLoad, "read OpenCode package metadata", err)
	}
	var meta packageMeta
	if err := yaml.Unmarshal(body, &meta); err != nil {
		return nil, domain.NewError(domain.ErrManifestLoad, "parse OpenCode package metadata", err)
	}
	var out []pluginRef
	for _, plugin := range meta.Plugins {
		plugin.Name = strings.TrimSpace(plugin.Name)
		if plugin.Name == "" {
			continue
		}
		out = append(out, plugin)
	}
	return out, nil
}

func (m *sourceMaterial) loadFirstClassDocs(sourceRoot string) error {
	defaultAgentPath := filepath.Join(sourceRoot, "src", "targets", "opencode", "default_agent.txt")
	if fileExists(defaultAgentPath) {
		body, err := os.ReadFile(defaultAgentPath)
		if err != nil {
			return domain.NewError(domain.ErrManifestLoad, "read OpenCode default agent", err)
		}
		text := strings.TrimSpace(string(body))
		if text == "" {
			return domain.NewError(domain.ErrManifestLoad, "OpenCode default agent must be a non-empty string", nil)
		}
		m.WholeFields["default_agent"] = text
	}
	instructionsPath := filepath.Join(sourceRoot, "src", "targets", "opencode", "instructions.yaml")
	if fileExists(instructionsPath) {
		body, err := os.ReadFile(instructionsPath)
		if err != nil {
			return domain.NewError(domain.ErrManifestLoad, "read OpenCode instructions", err)
		}
		var instructions []string
		if err := yaml.Unmarshal(body, &instructions); err != nil {
			return domain.NewError(domain.ErrManifestLoad, "parse OpenCode instructions", err)
		}
		if len(instructions) == 0 {
			return domain.NewError(domain.ErrManifestLoad, "OpenCode instructions must contain at least one path", nil)
		}
		for i, item := range instructions {
			instructions[i] = strings.TrimSpace(item)
			if instructions[i] == "" {
				return domain.NewError(domain.ErrManifestLoad, "OpenCode instructions must contain only non-empty paths", nil)
			}
		}
		m.WholeFields["instructions"] = instructions
	}
	permissionPath := filepath.Join(sourceRoot, "src", "targets", "opencode", "permission.json")
	if fileExists(permissionPath) {
		body, err := os.ReadFile(permissionPath)
		if err != nil {
			return domain.NewError(domain.ErrManifestLoad, "read OpenCode permission config", err)
		}
		var permission any
		if err := json.Unmarshal(body, &permission); err != nil {
			return domain.NewError(domain.ErrManifestLoad, "parse OpenCode permission config", err)
		}
		if !isPermissionValue(permission) {
			return domain.NewError(domain.ErrManifestLoad, "OpenCode permission must be a string or object", nil)
		}
		m.WholeFields["permission"] = permission
	}
	return nil
}

func (m *sourceMaterial) mergeExtra(extra map[string]any) error {
	for key, value := range extra {
		if _, exists := m.WholeFields[key]; exists || key == "plugin" || key == "mcp" || key == "mode" {
			return domain.NewError(domain.ErrManifestLoad, "OpenCode config.extra.json conflicts with managed key "+key, nil)
		}
		m.WholeFields[key] = value
	}
	return nil
}

func (m sourceMaterial) mutationForUpdate(target domain.TargetInstallation) configMutation {
	currentKeys := ownedConfigKeys(target)
	currentPlugins := ownedPluginRefs(target)
	currentMCP := ownedMCPAliases(target)
	return configMutation{
		WholeSet:      m.WholeFields,
		WholeRemove:   subtractStrings(currentKeys, sortedManagedKeys(m.WholeFields)),
		PluginsSet:    m.Plugins,
		PluginsRemove: subtractStrings(currentPlugins, pluginRefNames(m.Plugins)),
		MCPSet:        m.MCP,
		MCPRemove:     subtractStrings(currentMCP, sortedMapKeys(m.MCP)),
	}
}

func readConfigExtra(path string) (map[string]any, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, domain.NewError(domain.ErrManifestLoad, "read OpenCode config.extra.json", err)
	}
	var extra map[string]any
	if err := json.Unmarshal(body, &extra); err != nil {
		return nil, domain.NewError(domain.ErrManifestLoad, "parse OpenCode config.extra.json", err)
	}
	return extra, nil
}

func isPermissionValue(value any) bool {
	if _, ok := value.(string); ok {
		return true
	}
	_, ok := value.(map[string]any)
	return ok
}

func interpolatePackageRoot(value, packageRoot string) string {
	return strings.ReplaceAll(value, "${package.root}", packageRoot)
}

func isMissingPortableMCP(err error) bool {
	if err == nil {
		return false
	}
	var de *domain.Error
	if errors.As(err, &de) {
		return de.Code == domain.ErrManifestLoad && strings.Contains(strings.ToLower(de.Message), "portable mcp file not found")
	}
	return false
}
