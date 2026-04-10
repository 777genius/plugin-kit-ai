package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/pelletier/go-toml/v2"
)

type marketplaceDoc struct {
	Name      string           `json:"name,omitempty"`
	Interface map[string]any   `json:"interface,omitempty"`
	Plugins   []map[string]any `json:"plugins,omitempty"`
	Extra     map[string]any   `json:"-"`
}

type pluginConfigDoc struct {
	Plugins map[string]pluginConfigEntry `toml:"plugins"`
}

type pluginConfigEntry struct {
	Enabled *bool `toml:"enabled"`
}

type pluginConfigState struct {
	Present  bool
	Disabled bool
}

func (d *marketplaceDoc) UnmarshalJSON(body []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	d.Extra = map[string]any{}
	if value, ok := raw["name"].(string); ok {
		d.Name = strings.TrimSpace(value)
	}
	if value, ok := raw["interface"].(map[string]any); ok {
		d.Interface = value
	}
	if items, ok := raw["plugins"].([]any); ok {
		d.Plugins = make([]map[string]any, 0, len(items))
		for _, item := range items {
			doc, ok := item.(map[string]any)
			if !ok {
				return domain.NewError(domain.ErrMutationApply, "Codex marketplace plugins entries must be JSON objects", nil)
			}
			d.Plugins = append(d.Plugins, doc)
		}
	}
	delete(raw, "name")
	delete(raw, "interface")
	delete(raw, "plugins")
	for key, value := range raw {
		d.Extra[key] = value
	}
	return nil
}

func (d marketplaceDoc) MarshalJSON() ([]byte, error) {
	raw := map[string]any{}
	for key, value := range d.Extra {
		raw[key] = value
	}
	if strings.TrimSpace(d.Name) != "" {
		raw["name"] = d.Name
	}
	if len(d.Interface) > 0 {
		raw["interface"] = d.Interface
	}
	raw["plugins"] = d.Plugins
	return json.Marshal(raw)
}

func readMarketplace(path string) (marketplaceDoc, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return marketplaceDoc{Extra: map[string]any{}, Plugins: []map[string]any{}}, nil
		}
		return marketplaceDoc{}, domain.NewError(domain.ErrMutationApply, "read Codex marketplace catalog", err)
	}
	var doc marketplaceDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return marketplaceDoc{}, domain.NewError(domain.ErrMutationApply, "parse Codex marketplace catalog", err)
	}
	if doc.Extra == nil {
		doc.Extra = map[string]any{}
	}
	if doc.Plugins == nil {
		doc.Plugins = []map[string]any{}
	}
	return doc, nil
}

func writeMarketplace(path string, doc marketplaceDoc) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return domain.NewError(domain.ErrMutationApply, "prepare Codex marketplace dir", err)
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "marshal Codex marketplace catalog", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return domain.NewError(domain.ErrMutationApply, "write Codex marketplace catalog", err)
	}
	return nil
}

func marketplaceEntryDoc(manifest domain.IntegrationManifest, pluginRoot string) map[string]any {
	return map[string]any{
		"name": manifest.IntegrationID,
		"source": map[string]any{
			"source": "local",
			"path":   "./plugins/" + manifest.IntegrationID,
		},
		"policy": map[string]any{
			"installation":   "AVAILABLE",
			"authentication": "ON_INSTALL",
		},
		"category": "Productivity",
	}
}

func mergeMarketplaceEntry(path string, entry map[string]any) (string, error) {
	doc, err := readMarketplace(path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(doc.Name) == "" {
		doc.Name = defaultMarketplaceName(path)
	}
	name, _ := entry["name"].(string)
	if strings.TrimSpace(name) == "" {
		return "", domain.NewError(domain.ErrMutationApply, "Codex marketplace entry is missing plugin name", nil)
	}
	replaced := false
	for i, existing := range doc.Plugins {
		existingName, _ := existing["name"].(string)
		if strings.TrimSpace(existingName) == strings.TrimSpace(name) {
			doc.Plugins[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		doc.Plugins = append(doc.Plugins, entry)
	}
	slices.SortFunc(doc.Plugins, func(a, b map[string]any) int {
		return strings.Compare(pluginName(a), pluginName(b))
	})
	if err := writeMarketplace(path, doc); err != nil {
		return "", err
	}
	return doc.Name, nil
}

func removeMarketplaceEntry(path, pluginName string) error {
	doc, err := readMarketplace(path)
	if err != nil {
		return err
	}
	filtered := make([]map[string]any, 0, len(doc.Plugins))
	for _, item := range doc.Plugins {
		if strings.TrimSpace(pluginNameFromEntry(item)) == strings.TrimSpace(pluginName) {
			continue
		}
		filtered = append(filtered, item)
	}
	doc.Plugins = filtered
	return writeMarketplace(path, doc)
}

func readMarketplaceEntry(path, pluginName string) (map[string]any, bool, error) {
	doc, err := readMarketplace(path)
	if err != nil {
		return nil, false, err
	}
	for _, item := range doc.Plugins {
		if strings.TrimSpace(pluginNameFromEntry(item)) == strings.TrimSpace(pluginName) {
			return item, true, nil
		}
	}
	return nil, false, nil
}

func policyFromEntry(entry map[string]any) *domain.CatalogPolicySnapshot {
	policy, _ := entry["policy"].(map[string]any)
	out := &domain.CatalogPolicySnapshot{}
	if value, ok := policy["installation"].(string); ok {
		out.Installation = strings.TrimSpace(value)
	}
	if value, ok := policy["authentication"].(string); ok {
		out.Authentication = strings.TrimSpace(value)
	}
	if value, ok := entry["category"].(string); ok {
		out.Category = strings.TrimSpace(value)
	}
	if out.Installation == "" && out.Authentication == "" && out.Category == "" {
		return nil
	}
	return out
}

func defaultMarketplaceName(path string) string {
	_ = path
	return "integrationctl-managed"
}

func pluginName(item map[string]any) string {
	return strings.TrimSpace(pluginNameFromEntry(item))
}

func pluginNameFromEntry(item map[string]any) string {
	value, _ := item["name"].(string)
	return strings.TrimSpace(value)
}

func readPluginConfigState(path, pluginRef string) (pluginConfigState, string) {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(pluginRef) == "" {
		return pluginConfigState{}, ""
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return pluginConfigState{}, ""
		}
		return pluginConfigState{}, "read Codex config.toml: " + err.Error()
	}
	var doc pluginConfigDoc
	if err := toml.Unmarshal(body, &doc); err != nil {
		return pluginConfigState{}, "parse Codex config.toml: " + err.Error()
	}
	entry, ok := doc.Plugins[pluginRef]
	if !ok {
		return pluginConfigState{}, ""
	}
	state := pluginConfigState{Present: true}
	if entry.Enabled != nil && !*entry.Enabled {
		state.Disabled = true
	}
	return state, ""
}
