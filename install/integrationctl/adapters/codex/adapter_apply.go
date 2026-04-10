package codex

import (
	"context"
	"os"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (a Adapter) PlanInstall(_ context.Context, in ports.PlanInstallInput) (ports.AdapterPlan, error) {
	paths := a.pathsForScope(in.Policy.Scope, "", in.Manifest.IntegrationID)
	return ports.AdapterPlan{
		TargetID:          a.ID(),
		ActionClass:       "install_missing",
		Summary:           "Materialize a Codex local marketplace entry and plugin bundle, then wait for native activation",
		PathsTouched:      []string{paths.CatalogPath, paths.PluginRoot, paths.ConfigPath},
		NewThreadRequired: true,
		ManualSteps:       manualInstallSteps(marketplaceLocationLabel(paths.Scope), in.Manifest.IntegrationID),
		EvidenceKey:       "target.codex.native_surface",
	}, nil
}

func (a Adapter) ApplyInstall(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Codex install requires resolved source", nil)
	}
	paths := a.pathsForScope(in.Policy.Scope, workspaceRootFromApplyInput(in), in.Manifest.IntegrationID)
	if err := a.syncManagedPlugin(ctx, in.Manifest, in.ResolvedSource.LocalPath, paths.PluginRoot); err != nil {
		return ports.ApplyResult{}, err
	}
	catalogName, err := mergeMarketplaceEntry(paths.CatalogPath, marketplaceEntryDoc(in.Manifest, paths.PluginRoot))
	if err != nil {
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallActivationPending,
		ActivationState:    domain.ActivationNativePending,
		OwnedNativeObjects: ownedObjects(paths.Scope, paths.CatalogPath, paths.PluginRoot, in.Manifest.IntegrationID),
		EvidenceClass:      domain.EvidenceConfirmed,
		NewThreadRequired:  true,
		EnvironmentRestrictions: []domain.EnvironmentRestrictionCode{
			domain.RestrictionNativeActivation,
			domain.RestrictionNewThreadRequired,
		},
		ManualSteps: manualInstallSteps(marketplaceLocationLabel(paths.Scope), in.Manifest.IntegrationID),
		AdapterMetadata: map[string]any{
			"catalog_path":      paths.CatalogPath,
			"catalog_name":      catalogName,
			"plugin_root":       paths.PluginRoot,
			"plugin_name":       in.Manifest.IntegrationID,
			"activation_method": "plugin_directory_install",
		},
	}, nil
}

func (a Adapter) PlanUpdate(_ context.Context, in ports.PlanUpdateInput) (ports.AdapterPlan, error) {
	paths := a.pathsForRecord(in.CurrentRecord)
	return ports.AdapterPlan{
		TargetID:          a.ID(),
		ActionClass:       "update_version",
		Summary:           "Refresh the Codex plugin bundle and local marketplace entry",
		PathsTouched:      []string{paths.CatalogPath, paths.PluginRoot, paths.ConfigPath},
		RestartRequired:   true,
		NewThreadRequired: true,
		ManualSteps:       manualUpdateSteps(in.CurrentRecord.IntegrationID),
		EvidenceKey:       "target.codex.native_surface",
	}, nil
}

func (a Adapter) ApplyUpdate(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.Record == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Codex update requires current record", nil)
	}
	if in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Codex update requires resolved source", nil)
	}
	if _, ok := in.Record.Targets[domain.TargetCodex]; !ok {
		return ports.ApplyResult{}, domain.NewError(domain.ErrStateConflict, "Codex target is missing from installation record", nil)
	}
	paths := a.pathsForRecord(*in.Record)
	if err := a.syncManagedPlugin(ctx, in.Manifest, in.ResolvedSource.LocalPath, paths.PluginRoot); err != nil {
		return ports.ApplyResult{}, err
	}
	catalogName, err := mergeMarketplaceEntry(paths.CatalogPath, marketplaceEntryDoc(in.Manifest, paths.PluginRoot))
	if err != nil {
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallActivationPending,
		ActivationState:    domain.ActivationNativePending,
		OwnedNativeObjects: ownedObjects(paths.Scope, paths.CatalogPath, paths.PluginRoot, in.Record.IntegrationID),
		EvidenceClass:      domain.EvidenceConfirmed,
		RestartRequired:    true,
		NewThreadRequired:  true,
		EnvironmentRestrictions: []domain.EnvironmentRestrictionCode{
			domain.RestrictionNativeActivation,
			domain.RestrictionRestartRequired,
			domain.RestrictionNewThreadRequired,
		},
		ManualSteps: manualUpdateSteps(in.Record.IntegrationID),
		AdapterMetadata: map[string]any{
			"catalog_path":      paths.CatalogPath,
			"catalog_name":      catalogName,
			"plugin_root":       paths.PluginRoot,
			"plugin_name":       in.Record.IntegrationID,
			"activation_method": "plugin_directory_refresh",
		},
	}, nil
}

func (a Adapter) PlanRemove(_ context.Context, in ports.PlanRemoveInput) (ports.AdapterPlan, error) {
	paths := a.pathsForRecord(in.Record)
	return ports.AdapterPlan{
		TargetID:        a.ID(),
		ActionClass:     "remove_orphaned_target",
		Summary:         "Remove the managed Codex marketplace entry and plugin bundle",
		PathsTouched:    []string{paths.CatalogPath, paths.PluginRoot, paths.ConfigPath},
		ManualSteps:     manualRemoveSteps(in.Record.IntegrationID),
		RestartRequired: true,
		EvidenceKey:     "target.codex.native_surface",
	}, nil
}

func (a Adapter) ApplyRemove(_ context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.Record == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Codex remove requires current record", nil)
	}
	if _, ok := in.Record.Targets[domain.TargetCodex]; !ok {
		return ports.ApplyResult{}, domain.NewError(domain.ErrStateConflict, "Codex target is missing from installation record", nil)
	}
	paths := a.pathsForRecord(*in.Record)
	if err := removeMarketplaceEntry(paths.CatalogPath, in.Record.IntegrationID); err != nil {
		return ports.ApplyResult{}, err
	}
	if err := os.RemoveAll(paths.PluginRoot); err != nil && !os.IsNotExist(err) {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "remove Codex managed plugin root", err)
	}
	return ports.ApplyResult{
		TargetID:        a.ID(),
		State:           domain.InstallRemoved,
		ActivationState: domain.ActivationRestartPending,
		EvidenceClass:   domain.EvidenceConfirmed,
		RestartRequired: true,
		ManualSteps:     manualRemoveSteps(in.Record.IntegrationID),
		AdapterMetadata: map[string]any{
			"catalog_path": paths.CatalogPath,
			"plugin_root":  paths.PluginRoot,
			"plugin_name":  in.Record.IntegrationID,
		},
	}, nil
}

func (a Adapter) Repair(ctx context.Context, in ports.RepairInput) (ports.ApplyResult, error) {
	if in.Manifest == nil || in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrRepairApply, "Codex repair requires resolved source and manifest", nil)
	}
	record := in.Record
	result, err := a.ApplyUpdate(ctx, ports.ApplyInput{
		Plan:           ports.AdapterPlan{TargetID: a.ID(), ActionClass: "repair_drift", EvidenceKey: "target.codex.native_surface"},
		Manifest:       *in.Manifest,
		ResolvedSource: in.ResolvedSource,
		Policy:         record.Policy,
		Inspect:        in.Inspect,
		Record:         &record,
	})
	if err != nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrRepairApply, "Codex repair refresh failed", err)
	}
	if len(result.ManualSteps) == 0 {
		result.ManualSteps = append(result.ManualSteps, "restart Codex, then use the Plugin Directory to refresh the prepared local plugin and open a new thread")
	}
	return result, nil
}

func manualInstallSteps(location, pluginName string) []string {
	return []string{
		"open Codex Plugin Directory and install " + pluginName + " from the prepared " + location + " marketplace",
		"after installation, start a new Codex thread before using the plugin",
	}
}

func manualUpdateSteps(pluginName string) []string {
	return []string{
		"restart Codex so it re-reads the updated local marketplace source",
		"refresh or reinstall " + pluginName + " from the Codex Plugin Directory if the installed cache copy is stale",
		"open a new Codex thread before using the refreshed plugin",
	}
}

func manualRemoveSteps(pluginName string) []string {
	return []string{
		"if " + pluginName + " was already installed in Codex, uninstall it from the Codex Plugin Directory",
		"bundled apps stay managed separately in ChatGPT even after the plugin bundle is removed from Codex",
		"restart Codex after removing the plugin bundle",
	}
}

func marketplaceLocationLabel(scope string) string {
	if normalizedScope(scope) == "project" {
		return "project"
	}
	return "personal"
}
