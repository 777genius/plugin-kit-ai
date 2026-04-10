package publicationexec

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

func MergeCatalogArtifact(target string, existing, generated []byte) ([]byte, error) {
	switch strings.TrimSpace(target) {
	case "codex-package":
		return mergeCatalogDocument(existing, generated, "name", "interface")
	case "claude":
		return mergeCatalogDocument(existing, generated, "name", "owner")
	default:
		return nil, fmt.Errorf("local publication materialization supports only %q or %q", "codex-package", "claude")
	}
}

func RemoveCatalogArtifact(target string, existing []byte, pluginName string) ([]byte, bool, error) {
	switch strings.TrimSpace(target) {
	case "codex-package", "claude":
	default:
		return nil, false, fmt.Errorf("local publication materialization supports only %q or %q", "codex-package", "claude")
	}
	var current map[string]any
	if err := json.Unmarshal(existing, &current); err != nil {
		return nil, false, fmt.Errorf("parse existing marketplace artifact: %w", err)
	}
	currentPlugins, err := decodePluginEntries(current["plugins"])
	if err != nil {
		return nil, false, err
	}
	filtered := make([]map[string]any, 0, len(currentPlugins))
	removed := false
	for _, plugin := range currentPlugins {
		if strings.TrimSpace(stringValue(plugin["name"])) == strings.TrimSpace(pluginName) {
			removed = true
			continue
		}
		filtered = append(filtered, plugin)
	}
	if !removed {
		return append([]byte(nil), existing...), false, nil
	}
	slices.SortFunc(filtered, func(a, b map[string]any) int {
		return strings.Compare(strings.TrimSpace(stringValue(a["name"])), strings.TrimSpace(stringValue(b["name"])))
	})
	current["plugins"] = encodePluginEntries(filtered)
	body, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return body, true, nil
}

type CatalogIssue struct {
	Code    string
	Path    string
	Message string
}

func DiagnoseCatalogArtifact(target string, existing, generated []byte, pluginName string) ([]CatalogIssue, error) {
	switch strings.TrimSpace(target) {
	case "codex-package", "claude":
	default:
		return nil, fmt.Errorf("local publication materialization supports only %q or %q", "codex-package", "claude")
	}
	var current map[string]any
	if err := json.Unmarshal(existing, &current); err != nil {
		return nil, fmt.Errorf("parse existing marketplace artifact: %w", err)
	}
	var next map[string]any
	if err := json.Unmarshal(generated, &next); err != nil {
		return nil, fmt.Errorf("parse generated marketplace artifact: %w", err)
	}
	requiredTopLevelKeys := []string{"name"}
	if strings.TrimSpace(target) == "codex-package" {
		requiredTopLevelKeys = append(requiredTopLevelKeys, "interface")
	}
	if strings.TrimSpace(target) == "claude" {
		requiredTopLevelKeys = append(requiredTopLevelKeys, "owner")
	}
	var issues []CatalogIssue
	for _, key := range requiredTopLevelKeys {
		if currentValue, ok := current[key]; ok {
			if !jsonDocumentsEqual(currentValue, next[key]) {
				issues = append(issues, CatalogIssue{
					Code:    "drifted_materialized_catalog_identity",
					Path:    key,
					Message: fmt.Sprintf("catalog field %s does not match the authored publication identity", key),
				})
			}
		}
	}
	currentPlugins, err := decodePluginEntries(current["plugins"])
	if err != nil {
		return nil, err
	}
	nextPlugins, err := decodePluginEntries(next["plugins"])
	if err != nil {
		return nil, err
	}
	if len(nextPlugins) != 1 {
		return nil, fmt.Errorf("generated marketplace artifact must contain exactly one plugin entry")
	}
	generatedPlugin := nextPlugins[0]
	for _, plugin := range currentPlugins {
		if strings.TrimSpace(stringValue(plugin["name"])) != strings.TrimSpace(pluginName) {
			continue
		}
		if !jsonDocumentsEqual(plugin, generatedPlugin) {
			issues = append(issues, CatalogIssue{
				Code:    "drifted_materialized_catalog_entry",
				Path:    "plugins",
				Message: fmt.Sprintf("catalog entry for plugin %s is out of sync with current authored publication data", pluginName),
			})
		}
		return issues, nil
	}
	issues = append(issues, CatalogIssue{
		Code:    "missing_materialized_catalog_entry",
		Path:    "plugins",
		Message: fmt.Sprintf("catalog entry for plugin %s is missing", pluginName),
	})
	return issues, nil
}

func mergeCatalogDocument(existing, generated []byte, requiredTopLevelKeys ...string) ([]byte, error) {
	if len(existing) == 0 {
		return append([]byte(nil), generated...), nil
	}
	var current map[string]any
	if err := json.Unmarshal(existing, &current); err != nil {
		return nil, fmt.Errorf("parse existing marketplace artifact: %w", err)
	}
	var next map[string]any
	if err := json.Unmarshal(generated, &next); err != nil {
		return nil, fmt.Errorf("parse generated marketplace artifact: %w", err)
	}
	for _, key := range requiredTopLevelKeys {
		if currentValue, ok := current[key]; ok {
			nextValue := next[key]
			if !jsonDocumentsEqual(currentValue, nextValue) {
				return nil, fmt.Errorf("existing marketplace artifact sets %s differently; materialize requires a matching %s across the marketplace root", key, key)
			}
		}
	}
	currentPlugins, err := decodePluginEntries(current["plugins"])
	if err != nil {
		return nil, err
	}
	nextPlugins, err := decodePluginEntries(next["plugins"])
	if err != nil {
		return nil, err
	}
	if len(nextPlugins) != 1 {
		return nil, fmt.Errorf("generated marketplace artifact must contain exactly one plugin entry")
	}
	generatedPlugin := nextPlugins[0]
	generatedName := strings.TrimSpace(stringValue(generatedPlugin["name"]))
	if generatedName == "" {
		return nil, fmt.Errorf("generated marketplace artifact plugin entry is missing name")
	}
	replaced := false
	for i, plugin := range currentPlugins {
		if strings.TrimSpace(stringValue(plugin["name"])) == generatedName {
			currentPlugins[i] = generatedPlugin
			replaced = true
			break
		}
	}
	if !replaced {
		currentPlugins = append(currentPlugins, generatedPlugin)
	}
	slices.SortFunc(currentPlugins, func(a, b map[string]any) int {
		return strings.Compare(strings.TrimSpace(stringValue(a["name"])), strings.TrimSpace(stringValue(b["name"])))
	})
	next["plugins"] = encodePluginEntries(currentPlugins)
	return json.MarshalIndent(next, "", "  ")
}
