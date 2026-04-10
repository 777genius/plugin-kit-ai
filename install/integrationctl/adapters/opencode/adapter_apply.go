package opencode

import (
	"context"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (a Adapter) PlanInstall(ctx context.Context, in ports.PlanInstallInput) (ports.AdapterPlan, error) {
	configPath := a.configPath(in.Policy.Scope, "")
	paths := []string{configPath}
	if root := a.assetsRoot(in.Policy.Scope, ""); root != "" {
		paths = append(paths, root)
	}
	manualSteps, blocking := planBlockingManualSteps(in.Inspect)
	return ports.AdapterPlan{
		TargetID:        a.ID(),
		ActionClass:     "install_missing",
		Summary:         "Project or global OpenCode projection",
		RestartRequired: true,
		PathsTouched:    paths,
		ManualSteps:     manualSteps,
		Blocking:        blocking,
		EvidenceKey:     "target.opencode.native_surface",
	}, nil
}

func (a Adapter) ApplyInstall(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
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
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallInstalled,
		ActivationState:    domain.ActivationRestartPending,
		OwnedNativeObjects: ownedObjects(patch.ConfigPath, patch.ManagedKeys, patch.OwnedPluginRefs, patch.OwnedMCPAliases, copiedPaths, protectionForScope(in.Policy.Scope)),
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
	}, nil
}

func (a Adapter) PlanUpdate(_ context.Context, in ports.PlanUpdateInput) (ports.AdapterPlan, error) {
	configPath := a.configPath("user", "")
	if target, ok := in.CurrentRecord.Targets[domain.TargetOpenCode]; ok {
		configPath = configPathFromTarget(target, a.configPath(in.CurrentRecord.Policy.Scope, workspaceRootFromRecord(in.CurrentRecord)))
	}
	manualSteps, blocking := planBlockingManualSteps(in.Inspect)
	return ports.AdapterPlan{
		TargetID:        a.ID(),
		ActionClass:     "update_version",
		Summary:         "Owned projection refresh for OpenCode",
		RestartRequired: true,
		PathsTouched:    []string{configPath, a.assetsRoot(in.CurrentRecord.Policy.Scope, workspaceRootFromRecord(in.CurrentRecord))},
		ManualSteps:     manualSteps,
		Blocking:        blocking,
		EvidenceKey:     "target.opencode.native_surface",
	}, nil
}

func (a Adapter) ApplyUpdate(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
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
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallInstalled,
		ActivationState:    domain.ActivationRestartPending,
		OwnedNativeObjects: ownedObjects(patch.ConfigPath, patch.ManagedKeys, patch.OwnedPluginRefs, patch.OwnedMCPAliases, nextCopiedPaths, protectionForScope(in.Record.Policy.Scope)),
		EvidenceClass:      domain.EvidenceConfirmed,
		ManualSteps:        []string{"restart OpenCode to pick up updated config and projected files"},
		AdapterMetadata: map[string]any{
			"config_path":          patch.ConfigPath,
			"managed_config_keys":  patch.ManagedKeys,
			"owned_plugin_refs":    patch.OwnedPluginRefs,
			"owned_mcp_aliases":    patch.OwnedMCPAliases,
			"copied_paths":         nextCopiedPaths,
			"config_body_checksum": len(patch.Body),
		},
	}, nil
}

func (a Adapter) PlanRemove(_ context.Context, in ports.PlanRemoveInput) (ports.AdapterPlan, error) {
	configPath := a.configPath("user", "")
	if target, ok := in.Record.Targets[domain.TargetOpenCode]; ok {
		configPath = configPathFromTarget(target, a.configPath(in.Record.Policy.Scope, workspaceRootFromRecord(in.Record)))
	}
	manualSteps, blocking := planBlockingManualSteps(in.Inspect)
	return ports.AdapterPlan{
		TargetID:     a.ID(),
		ActionClass:  "remove_orphaned_target",
		Summary:      "Remove owned OpenCode projection",
		PathsTouched: []string{configPath, a.assetsRoot(in.Record.Policy.Scope, workspaceRootFromRecord(in.Record))},
		ManualSteps:  manualSteps,
		Blocking:     blocking,
		EvidenceKey:  "target.opencode.native_surface",
	}, nil
}

func (a Adapter) ApplyRemove(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.Record == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "OpenCode remove requires current record", nil)
	}
	target, ok := in.Record.Targets[domain.TargetOpenCode]
	if !ok {
		return ports.ApplyResult{}, domain.NewError(domain.ErrStateConflict, "OpenCode target is missing from installation record", nil)
	}
	configPath := configPathFromTarget(target, a.configPath(in.Record.Policy.Scope, workspaceRootFromRecord(*in.Record)))
	patch, err := a.patchConfig(ctx, configPath, configMutation{
		WholeRemove:   ownedConfigKeys(target),
		PluginsRemove: ownedPluginRefs(target),
		MCPRemove:     ownedMCPAliases(target),
	}, &target)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	if err := a.removeStaleFiles(ctx, copiedFilePaths(target), nil); err != nil {
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:        a.ID(),
		State:           domain.InstallRemoved,
		ActivationState: domain.ActivationRestartPending,
		EvidenceClass:   domain.EvidenceConfirmed,
		ManualSteps:     []string{"restart OpenCode to unload removed managed config and projected files"},
		AdapterMetadata: map[string]any{
			"config_path":          patch.ConfigPath,
			"managed_config_keys":  nil,
			"owned_plugin_refs":    nil,
			"owned_mcp_aliases":    nil,
			"copied_paths":         nil,
			"config_body_checksum": len(patch.Body),
		},
	}, nil
}

func (a Adapter) Repair(ctx context.Context, in ports.RepairInput) (ports.ApplyResult, error) {
	if in.Manifest == nil || in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrRepairApply, "OpenCode repair requires resolved source and manifest", nil)
	}
	result, err := a.ApplyUpdate(ctx, ports.ApplyInput{
		Manifest:       *in.Manifest,
		ResolvedSource: in.ResolvedSource,
		Policy:         in.Record.Policy,
		Inspect:        in.Inspect,
		Record:         &in.Record,
	})
	if err != nil {
		return ports.ApplyResult{}, err
	}
	if len(result.ManualSteps) == 0 {
		result.ManualSteps = append(result.ManualSteps, "repair reconciled managed OpenCode config and projected files")
	}
	return result, nil
}
