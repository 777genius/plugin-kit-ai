package platformexec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func buildOpenCodeConfig(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) (map[string]any, error) {
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
	managedPaths := managedOpenCodeConfigPaths()
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
	if err := appendOpenCodeDefaultAgent(root, state, doc); err != nil {
		return nil, err
	}
	if err := appendOpenCodeInstructions(root, state, doc); err != nil {
		return nil, err
	}
	if err := appendOpenCodePermission(root, state, doc); err != nil {
		return nil, err
	}
	if err := pluginmodel.MergeNativeExtraObject(doc, extra, "opencode config.extra.json", managedPaths); err != nil {
		return nil, err
	}
	return doc, nil
}

func managedOpenCodeConfigPaths() []string {
	return []string{"$schema", "plugin", "mcp", "default_agent", "instructions", "permission", "mode"}
}

func appendOpenCodeDefaultAgent(root string, state pluginmodel.TargetState, doc map[string]any) error {
	rel := strings.TrimSpace(state.DocPath("default_agent"))
	if rel == "" {
		return nil
	}
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return err
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return fmt.Errorf("%s must contain a non-empty agent name", rel)
	}
	doc["default_agent"] = text
	return nil
}

func appendOpenCodeInstructions(root string, state pluginmodel.TargetState, doc map[string]any) error {
	rel := strings.TrimSpace(state.DocPath("instructions_config"))
	if rel == "" {
		return nil
	}
	instructions, _, err := readYAMLDoc[[]string](root, rel)
	if err != nil {
		return fmt.Errorf("parse %s: %w", rel, err)
	}
	if len(instructions) == 0 {
		return fmt.Errorf("%s must contain at least one instruction path", rel)
	}
	for i, instruction := range instructions {
		if strings.TrimSpace(instruction) == "" {
			return fmt.Errorf("%s instruction entry %d must be a non-empty string", rel, i)
		}
		instructions[i] = strings.TrimSpace(instruction)
	}
	doc["instructions"] = instructions
	return nil
}

func appendOpenCodePermission(root string, state pluginmodel.TargetState, doc map[string]any) error {
	rel := strings.TrimSpace(state.DocPath("permission_config"))
	if rel == "" {
		return nil
	}
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return err
	}
	var permission any
	if err := json.Unmarshal(body, &permission); err != nil {
		return fmt.Errorf("parse %s: %w", rel, err)
	}
	if !isOpenCodePermissionValue(permission) {
		return fmt.Errorf("%s must be a JSON string or object", rel)
	}
	doc["permission"] = permission
	return nil
}
