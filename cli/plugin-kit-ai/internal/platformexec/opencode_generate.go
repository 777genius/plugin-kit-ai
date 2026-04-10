package platformexec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func (opencodeAdapter) Generate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]pluginmodel.Artifact, error) {
	meta, _, err := readYAMLDoc[opencodePackageMeta](root, state.DocPath("package_metadata"))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	}
	if err := validateOpenCodePluginRefs(meta.Plugins); err != nil {
		return nil, fmt.Errorf("%s %w", state.DocPath("package_metadata"), err)
	}
	extra, err := loadNativeExtraDoc(root, state, "config_extra", pluginmodel.NativeDocFormatJSON)
	if err != nil {
		return nil, err
	}
	managedPaths := []string{"$schema", "plugin", "mcp", "default_agent", "instructions", "permission", "mode"}
	if err := pluginmodel.ValidateNativeExtraDocConflicts(extra, "opencode config.extra.json", managedPaths); err != nil {
		return nil, err
	}
	doc := map[string]any{
		"$schema": "https://opencode.ai/config.json",
	}
	if len(meta.Plugins) > 0 {
		doc["plugin"] = jsonValuesForOpenCodePlugins(meta.Plugins)
	}
	if graph.Portable.MCP != nil {
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "opencode")
		if err != nil {
			return nil, err
		}
		doc["mcp"] = projected
	}
	if rel := strings.TrimSpace(state.DocPath("default_agent")); rel != "" {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		text := strings.TrimSpace(string(body))
		if text == "" {
			return nil, fmt.Errorf("%s must contain a non-empty agent name", rel)
		}
		doc["default_agent"] = text
	}
	if rel := strings.TrimSpace(state.DocPath("instructions_config")); rel != "" {
		instructions, _, err := readYAMLDoc[[]string](root, rel)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		if len(instructions) == 0 {
			return nil, fmt.Errorf("%s must contain at least one instruction path", rel)
		}
		for i, instruction := range instructions {
			if strings.TrimSpace(instruction) == "" {
				return nil, fmt.Errorf("%s instruction entry %d must be a non-empty string", rel, i)
			}
			instructions[i] = strings.TrimSpace(instruction)
		}
		doc["instructions"] = instructions
	}
	if rel := strings.TrimSpace(state.DocPath("permission_config")); rel != "" {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		var permission any
		if err := json.Unmarshal(body, &permission); err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		if !isOpenCodePermissionValue(permission) {
			return nil, fmt.Errorf("%s must be a JSON string or object", rel)
		}
		doc["permission"] = permission
	}
	if err := pluginmodel.MergeNativeExtraObject(doc, extra, "opencode config.extra.json", managedPaths); err != nil {
		return nil, err
	}
	body, err := marshalJSON(doc)
	if err != nil {
		return nil, err
	}
	artifacts := []pluginmodel.Artifact{{
		RelPath: "opencode.json",
		Content: body,
	}}
	skillArtifacts, err := renderPortableSkills(root, graph.Portable.Paths("skills"), ".opencode/skills")
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, skillArtifacts...)
	copied, err := copyArtifactDirs(root,
		artifactDir{src: authoredComponentDir(state, "commands", filepath.Join("targets", "opencode", "commands")), dst: filepath.Join(".opencode", "commands")},
		artifactDir{src: authoredComponentDir(state, "agents", filepath.Join("targets", "opencode", "agents")), dst: filepath.Join(".opencode", "agents")},
		artifactDir{src: authoredComponentDir(state, "themes", filepath.Join("targets", "opencode", "themes")), dst: filepath.Join(".opencode", "themes")},
		artifactDir{src: authoredComponentDir(state, "tools", filepath.Join("targets", "opencode", "tools")), dst: filepath.Join(".opencode", "tools")},
		artifactDir{src: authoredOpenCodePluginDir(root, state), dst: filepath.Join(".opencode", "plugins")},
	)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, copied...)
	packageArtifacts, err := copySingleArtifactIfExists(root, state.DocPath("local_plugin_dependencies"), filepath.Join(".opencode", "package.json"))
	if err != nil {
		return nil, err
	}
	return append(artifacts, packageArtifacts...), nil
}

func (opencodeAdapter) ManagedPaths(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]string, error) {
	return nil, nil
}

func renderPortableSkills(root string, paths []string, outputRoot string) ([]pluginmodel.Artifact, error) {
	var artifacts []pluginmodel.Artifact
	for _, rel := range paths {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		normalizedRel := filepath.ToSlash(rel)
		switch {
		case strings.HasPrefix(normalizedRel, pluginmodel.SourceDirName+"/skills/"):
			normalizedRel = strings.TrimPrefix(normalizedRel, pluginmodel.SourceDirName+"/")
		case normalizedRel == pluginmodel.SourceDirName+"/skills":
			normalizedRel = "skills"
		}
		child, err := filepath.Rel(filepath.FromSlash("skills"), filepath.FromSlash(normalizedRel))
		if err != nil {
			return nil, err
		}
		if child == "." || strings.HasPrefix(child, ".."+string(filepath.Separator)) || child == ".." {
			return nil, fmt.Errorf("portable skill path %s must live under skills/", rel)
		}
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.ToSlash(filepath.Join(outputRoot, child)),
			Content: body,
		})
	}
	return compactArtifacts(artifacts), nil
}

func authoredOpenCodePluginDir(root string, state pluginmodel.TargetState) string {
	if paths := state.ComponentPaths("local_plugin_code"); len(paths) > 0 {
		dir := filepath.ToSlash(filepath.Dir(paths[0]))
		if dir != "." {
			return dir
		}
	}
	for _, candidate := range []string{
		filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "plugins"),
	} {
		if _, err := os.Stat(filepath.Join(root, candidate)); err == nil {
			return filepath.ToSlash(candidate)
		}
	}
	return filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "plugins"))
}
