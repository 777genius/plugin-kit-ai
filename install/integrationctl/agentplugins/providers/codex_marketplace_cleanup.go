package providers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// managedCodexMarketplaceRegistered proves that the exact manager-owned
// marketplace still points at the exact managed package before native cleanup.
// A same-name user replacement is never removed.
func managedCodexMarketplaceRegistered(configRoot, marketplace, managedArtifactPath string) (bool, error) {
	if strings.TrimSpace(configRoot) == "" || strings.TrimSpace(managedArtifactPath) == "" {
		return false, fmt.Errorf("managed Codex marketplace ownership evidence is incomplete")
	}
	body, err := os.ReadFile(filepath.Join(configRoot, "config.toml"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect managed Codex marketplace config: %w", err)
	}
	var document map[string]any
	if err := toml.Unmarshal(body, &document); err != nil {
		return false, fmt.Errorf("inspect managed Codex marketplace config: %w", err)
	}
	marketplaces, ok := document["marketplaces"].(map[string]any)
	if !ok {
		if _, present := document["marketplaces"]; present {
			return false, fmt.Errorf("refuse managed Codex marketplace cleanup because the registry shape is not recognized")
		}
		return false, nil
	}
	entryValue, present := marketplaces[marketplace]
	if !present {
		return false, nil
	}
	entry, ok := entryValue.(map[string]any)
	if !ok {
		return false, fmt.Errorf("refuse managed Codex marketplace cleanup because %s has an unrecognized entry", marketplace)
	}
	source, ok := entry["source"].(string)
	if !ok || strings.TrimSpace(source) == "" {
		return false, fmt.Errorf("refuse managed Codex marketplace cleanup because %s has no local source", marketplace)
	}
	if sourceType, present := entry["source_type"]; present && sourceType != "local" {
		return false, fmt.Errorf("refuse managed Codex marketplace cleanup because %s is not a local source", marketplace)
	}
	if filepath.Clean(source) != filepath.Clean(managedArtifactPath) {
		return false, fmt.Errorf("refuse managed Codex marketplace cleanup because %s no longer points at the managed artifact", marketplace)
	}
	return true, nil
}
