package opencode

import (
	"context"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func sortedManagedKeys(values map[string]any) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		if key == "$schema" {
			continue
		}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func ownedObjects(configPath string, managedKeys, pluginRefs, mcpAliases, copiedPaths []string, protection domain.ProtectionClass) []domain.NativeObjectRef {
	out := []domain.NativeObjectRef{{
		Kind:            "file",
		Path:            configPath,
		ProtectionClass: protection,
	}}
	for _, key := range managedKeys {
		out = append(out, domain.NativeObjectRef{
			Kind:            "opencode_config_key",
			Name:            key,
			Path:            configPath,
			ProtectionClass: protection,
		})
	}
	for _, name := range pluginRefs {
		out = append(out, domain.NativeObjectRef{
			Kind:            "opencode_plugin_ref",
			Name:            name,
			Path:            configPath,
			ProtectionClass: protection,
		})
	}
	for _, alias := range mcpAliases {
		out = append(out, domain.NativeObjectRef{
			Kind:            "opencode_mcp_server",
			Name:            alias,
			Path:            configPath,
			ProtectionClass: protection,
		})
	}
	for _, path := range copiedPaths {
		out = append(out, domain.NativeObjectRef{
			Kind:            "file",
			Path:            path,
			ProtectionClass: protection,
		})
	}
	return out
}

func protectionForScope(scope string) domain.ProtectionClass {
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		return domain.ProtectionWorkspace
	}
	return domain.ProtectionUserMutable
}

func sortedMapKeys(values map[string]any) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func ownedConfigKeys(target domain.TargetInstallation) []string {
	var out []string
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "opencode_config_key" && strings.TrimSpace(item.Name) != "" {
			out = append(out, item.Name)
		}
	}
	if len(out) == 0 {
		if raw, ok := target.AdapterMetadata["managed_config_keys"].([]string); ok {
			out = append(out, raw...)
		} else if raw, ok := target.AdapterMetadata["managed_config_keys"].([]any); ok {
			for _, value := range raw {
				if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
					out = append(out, text)
				}
			}
		}
	}
	sort.Strings(out)
	return dedupeStrings(out)
}

func ownedPluginRefs(target domain.TargetInstallation) []string {
	var out []string
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "opencode_plugin_ref" && strings.TrimSpace(item.Name) != "" {
			out = append(out, item.Name)
		}
	}
	if len(out) == 0 {
		if raw, ok := target.AdapterMetadata["owned_plugin_refs"].([]string); ok {
			out = append(out, raw...)
		} else if raw, ok := target.AdapterMetadata["owned_plugin_refs"].([]any); ok {
			for _, value := range raw {
				if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
					out = append(out, text)
				}
			}
		}
	}
	sort.Strings(out)
	return dedupeStrings(out)
}

func ownedMCPAliases(target domain.TargetInstallation) []string {
	var out []string
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "opencode_mcp_server" && strings.TrimSpace(item.Name) != "" {
			out = append(out, item.Name)
		}
	}
	if len(out) == 0 {
		if raw, ok := target.AdapterMetadata["owned_mcp_aliases"].([]string); ok {
			out = append(out, raw...)
		} else if raw, ok := target.AdapterMetadata["owned_mcp_aliases"].([]any); ok {
			for _, value := range raw {
				if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
					out = append(out, text)
				}
			}
		}
	}
	sort.Strings(out)
	return dedupeStrings(out)
}

func ownedPluginRefsOrMetadata(target *domain.TargetInstallation) []string {
	if target == nil {
		return nil
	}
	return ownedPluginRefs(*target)
}

func ownedMCPAliasesOrMetadata(target *domain.TargetInstallation) []string {
	if target == nil {
		return nil
	}
	return ownedMCPAliases(*target)
}

func copiedFilePaths(target domain.TargetInstallation) []string {
	var out []string
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "file" && strings.TrimSpace(item.Path) != "" && !strings.HasSuffix(item.Path, "opencode.json") && !strings.HasSuffix(item.Path, "opencode.jsonc") {
			out = append(out, item.Path)
		}
	}
	sort.Strings(out)
	return dedupeStrings(out)
}

func (a Adapter) removeStaleFiles(ctx context.Context, previous, keep []string) error {
	keepSet := mapFromSlice(keep, func(value string) string { return value })
	for _, path := range previous {
		if keepSet[path] {
			continue
		}
		if err := a.fs().Remove(ctx, path); err != nil {
			return domain.NewError(domain.ErrMutationApply, "remove stale OpenCode projected asset", err)
		}
		a.removeEmptyParents(path, a.assetsRootForPath(path))
	}
	return nil
}

func mapFromSlice[T any](values []T, keyFn func(T) string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		key := strings.TrimSpace(keyFn(value))
		if key != "" {
			out[key] = true
		}
	}
	return out
}

func subtractStrings(current, next []string) []string {
	nextSet := mapFromSlice(next, func(value string) string { return value })
	var out []string
	for _, item := range current {
		if !nextSet[item] {
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return dedupeStrings(out)
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := values[:0]
	var last string
	for _, value := range values {
		if value == "" || value == last {
			continue
		}
		out = append(out, value)
		last = value
	}
	return out
}
