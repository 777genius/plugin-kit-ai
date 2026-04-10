package opencode

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
	"github.com/tailscale/hujson"
)

func (a Adapter) patchConfig(ctx context.Context, path string, mutation configMutation, target *domain.TargetInstallation) (configPatchResult, error) {
	body, err := a.fs().ReadFile(ctx, path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "read OpenCode config", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		body = []byte("{}\n")
	}
	ast, err := hujson.Parse(body)
	if err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "parse OpenCode config", err)
	}
	obj, ok := ast.Value.(*hujson.Object)
	if !ok {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "OpenCode config root must be an object", nil)
	}
	doc, err := decodeConfigMap(body)
	if err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "decode OpenCode config", err)
	}
	currentPlugins, err := existingPluginRefs(doc["plugin"])
	if err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "parse OpenCode plugin refs", err)
	}
	currentMCP, err := existingObjectMap(doc["mcp"], "mcp")
	if err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "parse OpenCode MCP config", err)
	}
	oldPluginRefs := mapFromSlice(ownedPluginRefsOrMetadata(target), func(value string) string { return value })
	oldMCPAliases := mapFromSlice(ownedMCPAliasesOrMetadata(target), func(value string) string { return value })
	for _, ref := range mutation.PluginsSet {
		name := strings.TrimSpace(ref.Name)
		if name == "" {
			continue
		}
		if existing, ok := currentPlugins[name]; ok && !oldPluginRefs[name] && !pluginRefsEqual(existing, ref) {
			return configPatchResult{}, domain.NewError(domain.ErrStateConflict, "OpenCode plugin ref conflict for "+name, nil)
		}
	}
	for alias, desired := range mutation.MCPSet {
		if existing, ok := currentMCP[alias]; ok && !oldMCPAliases[alias] && !jsonValuesEqual(existing, desired) {
			return configPatchResult{}, domain.NewError(domain.ErrStateConflict, "OpenCode MCP alias conflict for "+alias, nil)
		}
	}
	for _, key := range mutation.WholeRemove {
		if strings.TrimSpace(key) == "" || key == "$schema" {
			continue
		}
		removeTopLevelMember(obj, key)
	}
	mergedPlugins := mergePluginRefs(currentPlugins, mutation.PluginsRemove, mutation.PluginsSet)
	if len(mergedPlugins) == 0 {
		removeTopLevelMember(obj, "plugin")
	} else if err := setTopLevelMember(obj, "plugin", pluginRefsToJSON(mergedPlugins)); err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "patch OpenCode plugin refs", err)
	}
	mergedMCP := mergeNamedObject(currentMCP, mutation.MCPRemove, mutation.MCPSet)
	if len(mergedMCP) == 0 {
		removeTopLevelMember(obj, "mcp")
	} else if err := setTopLevelMember(obj, "mcp", mergedMCP); err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "patch OpenCode MCP config", err)
	}
	for key, value := range mutation.WholeSet {
		if err := setTopLevelMember(obj, key, value); err != nil {
			return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "patch OpenCode config", err)
		}
	}
	if len(obj.Members) == 0 {
		if err := a.fs().Remove(ctx, path); err != nil {
			return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "remove empty OpenCode config", err)
		}
		return configPatchResult{
			ConfigPath: path,
		}, nil
	}
	rendered := ast.Pack()
	if _, err := a.mutator().MutateFile(ctx, ports.SafeFileMutationInput{
		Path: path,
		Mode: 0o644,
		Build: func(_ []byte, _ bool) ([]byte, error) {
			return rendered, nil
		},
		ValidateBefore: func(next []byte) error {
			_, err := hujson.Parse(next)
			return err
		},
		ValidateAfter: func(_ context.Context, path string, _ []byte) error {
			body, err := a.fs().ReadFile(ctx, path)
			if err != nil {
				return err
			}
			_, err = hujson.Parse(body)
			return err
		},
	}); err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "write OpenCode config", err)
	}
	return configPatchResult{
		Body:            rendered,
		ConfigPath:      path,
		ManagedKeys:     sortedManagedKeys(mutation.WholeSet),
		OwnedPluginRefs: pluginRefNames(mutation.PluginsSet),
		OwnedMCPAliases: sortedMapKeys(mutation.MCPSet),
	}, nil
}
