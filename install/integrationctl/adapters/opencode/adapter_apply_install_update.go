package opencode

import (
	"context"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (a Adapter) applyInstall(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "OpenCode apply requires resolved source", nil)
	}
	material, err := a.loadSourceMaterial(ctx, in.ResolvedSource.LocalPath, in.Policy.Scope, workspaceRootFromApplyInput(in))
	if err != nil {
		return ports.ApplyResult{}, err
	}
	configPath := a.configPath(in.Policy.Scope, workspaceRootFromApplyInput(in))
	patch, err := a.patchConfig(ctx, configPath, configMutation{
		WholeSet:   material.WholeFields,
		PluginsSet: material.Plugins,
		MCPSet:     material.MCP,
	}, nil)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	copiedPaths, err := a.copyOwnedFiles(material.CopyFiles)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	return applyProjectionResult(a.ID(), in.Policy.Scope, patch, copiedPaths), nil
}

func (a Adapter) applyUpdate(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.Record == nil || in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "OpenCode update requires current record and resolved source", nil)
	}
	target, ok := in.Record.Targets[domain.TargetOpenCode]
	if !ok {
		return ports.ApplyResult{}, domain.NewError(domain.ErrStateConflict, "OpenCode target is missing from installation record", nil)
	}
	material, err := a.loadSourceMaterial(ctx, in.ResolvedSource.LocalPath, in.Record.Policy.Scope, workspaceRootFromRecord(*in.Record))
	if err != nil {
		return ports.ApplyResult{}, err
	}
	configPath := configPathFromTarget(target, a.configPath(in.Record.Policy.Scope, workspaceRootFromRecord(*in.Record)))
	patch, err := a.patchConfig(ctx, configPath, material.mutationForUpdate(target), &target)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	nextCopiedPaths, err := a.copyOwnedFiles(material.CopyFiles)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	if err := a.removeStaleFiles(ctx, copiedFilePaths(target), nextCopiedPaths); err != nil {
		return ports.ApplyResult{}, err
	}
	return applyProjectionResult(a.ID(), in.Record.Policy.Scope, patch, nextCopiedPaths), nil
}

func applyProjectionResult(targetID domain.TargetID, scope string, patch configPatchResult, copiedPaths []string) ports.ApplyResult {
	return ports.ApplyResult{
		TargetID:           targetID,
		State:              domain.InstallInstalled,
		ActivationState:    domain.ActivationRestartPending,
		OwnedNativeObjects: ownedObjects(patch.ConfigPath, patch.ManagedKeys, patch.OwnedPluginRefs, patch.OwnedMCPAliases, copiedPaths, protectionForScope(scope)),
		EvidenceClass:      domain.EvidenceConfirmed,
		ManualSteps:        []string{"restart OpenCode to pick up updated config and projected files"},
		AdapterMetadata: map[string]any{
			"config_path":          patch.ConfigPath,
			"managed_config_keys":  patch.ManagedKeys,
			"owned_plugin_refs":    patch.OwnedPluginRefs,
			"owned_mcp_aliases":    patch.OwnedMCPAliases,
			"copied_paths":         copiedPaths,
			"config_body_checksum": len(patch.Body),
		},
	}
}
