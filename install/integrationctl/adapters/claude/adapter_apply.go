package claude

import (
	"context"
	"os"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (a Adapter) PlanInstall(_ context.Context, in ports.PlanInstallInput) (ports.AdapterPlan, error) {
	marketplaceRoot := managedMarketplaceRoot(a.userHome(), in.Manifest.IntegrationID)
	settings := a.settingsPath(in.Policy.Scope, "")
	manual, blocking := blockingSteps(in.Inspect)
	if blocked, message := a.marketplaceAddBlocked(in.Policy.Scope, "", in.Manifest.IntegrationID); blocked {
		manual = append(manual, message)
		blocking = true
	}
	return ports.AdapterPlan{
		TargetID:       a.ID(),
		ActionClass:    "install_missing",
		Summary:        "Install Claude plugin through a managed local marketplace",
		ReloadRequired: true,
		PathsTouched:   []string{marketplaceRoot, settings},
		ManualSteps:    manual,
		Blocking:       blocking,
		EvidenceKey:    "target.claude.native_surface",
	}, nil
}

func (a Adapter) ApplyInstall(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Claude install requires resolved source", nil)
	}
	commandDir := a.commandDirForScope(in.Policy.Scope, workspaceRootFromApplyInput(in))
	materializedRoot, marketplaceName, pluginRef, err := a.syncManagedMarketplace(ctx, in.Manifest, in.ResolvedSource.LocalPath)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	if err := a.runClaude(ctx, []string{"claude", "plugin", "marketplace", "add", materializedRoot}, commandDir); err != nil {
		return ports.ApplyResult{}, err
	}
	installArgv := []string{"claude", "plugin", "install", pluginRef}
	if scope := strings.TrimSpace(in.Policy.Scope); scope != "" {
		installArgv = append(installArgv, "--scope", scope)
	}
	if err := a.runClaude(ctx, installArgv, commandDir); err != nil {
		_ = a.runClaude(ctx, []string{"claude", "plugin", "marketplace", "remove", marketplaceName}, commandDir)
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallInstalled,
		ActivationState:    domain.ActivationReloadPending,
		OwnedNativeObjects: a.ownedObjects(in.Manifest.IntegrationID, in.Policy.Scope, workspaceRootFromApplyInput(in), materializedRoot),
		EvidenceClass:      domain.EvidenceConfirmed,
		ReloadRequired:     true,
		ManualSteps:        []string{"run /reload-plugins in Claude Code if the current session should pick up the new plugin immediately"},
		AdapterMetadata: map[string]any{
			"marketplace_name":         marketplaceName,
			"plugin_ref":               pluginRef,
			"materialized_source_root": materializedRoot,
			"marketplace_add_argv":     []string{"claude", "plugin", "marketplace", "add", materializedRoot},
			"plugin_install_argv":      installArgv,
		},
	}, nil
}

func (a Adapter) PlanUpdate(_ context.Context, in ports.PlanUpdateInput) (ports.AdapterPlan, error) {
	manual, blocking := blockingSteps(in.Inspect)
	return ports.AdapterPlan{
		TargetID:       a.ID(),
		ActionClass:    "update_version",
		Summary:        "Refresh managed Claude marketplace and reinstall the plugin",
		ReloadRequired: true,
		PathsTouched: []string{
			managedMarketplaceRoot(a.userHome(), in.CurrentRecord.IntegrationID),
			a.settingsPath(in.CurrentRecord.Policy.Scope, workspaceRootFromRecord(in.CurrentRecord)),
		},
		ManualSteps: manual,
		Blocking:    blocking,
		EvidenceKey: "target.claude.native_surface",
	}, nil
}

func (a Adapter) ApplyUpdate(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.Record == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Claude update requires current record", nil)
	}
	if in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Claude update requires resolved source", nil)
	}
	if seedPath, ok := a.seedManagedMarketplacePath(in.Record.IntegrationID, in.Record); ok {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Claude marketplace is seed-managed and read-only: "+seedPath, nil)
	}
	commandDir := a.commandDirForScope(in.Record.Policy.Scope, workspaceRootFromRecord(*in.Record))
	materializedRoot, marketplaceName, pluginRef, err := a.syncManagedMarketplace(ctx, in.Manifest, in.ResolvedSource.LocalPath)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	updateArgv := []string{"claude", "plugin", "marketplace", "update", marketplaceName}
	if err := a.runClaude(ctx, updateArgv, commandDir); err != nil {
		return ports.ApplyResult{}, err
	}
	uninstallArgv := []string{"claude", "plugin", "uninstall", pluginRef}
	if scope := strings.TrimSpace(in.Record.Policy.Scope); scope != "" {
		uninstallArgv = append(uninstallArgv, "--scope", scope)
	}
	if err := a.runClaude(ctx, uninstallArgv, commandDir); err != nil {
		return ports.ApplyResult{}, err
	}
	installArgv := []string{"claude", "plugin", "install", pluginRef}
	if scope := strings.TrimSpace(in.Record.Policy.Scope); scope != "" {
		installArgv = append(installArgv, "--scope", scope)
	}
	if err := a.runClaude(ctx, installArgv, commandDir); err != nil {
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallInstalled,
		ActivationState:    domain.ActivationReloadPending,
		OwnedNativeObjects: a.ownedObjects(in.Manifest.IntegrationID, in.Record.Policy.Scope, workspaceRootFromRecord(*in.Record), materializedRoot),
		EvidenceClass:      domain.EvidenceConfirmed,
		ReloadRequired:     true,
		ManualSteps:        []string{"run /reload-plugins in Claude Code so the updated plugin package is reloaded in the current session"},
		AdapterMetadata: map[string]any{
			"marketplace_name":         marketplaceName,
			"plugin_ref":               pluginRef,
			"materialized_source_root": materializedRoot,
			"marketplace_update_argv":  updateArgv,
			"plugin_uninstall_argv":    uninstallArgv,
			"plugin_install_argv":      installArgv,
		},
	}, nil
}

func (a Adapter) PlanRemove(_ context.Context, in ports.PlanRemoveInput) (ports.AdapterPlan, error) {
	manual, blocking := blockingSteps(in.Inspect)
	return ports.AdapterPlan{
		TargetID:       a.ID(),
		ActionClass:    "remove_orphaned_target",
		Summary:        "Uninstall Claude plugin and remove the managed local marketplace",
		ReloadRequired: true,
		PathsTouched: []string{
			managedMarketplaceRoot(a.userHome(), in.Record.IntegrationID),
			a.settingsPath(in.Record.Policy.Scope, workspaceRootFromRecord(in.Record)),
		},
		ManualSteps: manual,
		Blocking:    blocking,
		EvidenceKey: "target.claude.native_surface",
	}, nil
}

func (a Adapter) ApplyRemove(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.Record == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Claude remove requires current record", nil)
	}
	if seedPath, ok := a.seedManagedMarketplacePath(in.Record.IntegrationID, in.Record); ok {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Claude marketplace is seed-managed and read-only: "+seedPath, nil)
	}
	commandDir := a.commandDirForScope(in.Record.Policy.Scope, workspaceRootFromRecord(*in.Record))
	marketplaceName := marketplaceNameFromRecord(*in.Record)
	if marketplaceName == "" {
		marketplaceName = managedMarketplaceName(in.Record.IntegrationID)
	}
	pluginRef := pluginRefFromRecord(*in.Record)
	if pluginRef == "" {
		pluginRef = in.Record.IntegrationID + "@" + marketplaceName
	}
	uninstallArgv := []string{"claude", "plugin", "uninstall", pluginRef}
	if scope := strings.TrimSpace(in.Record.Policy.Scope); scope != "" {
		uninstallArgv = append(uninstallArgv, "--scope", scope)
	}
	if err := a.runClaude(ctx, uninstallArgv, commandDir); err != nil {
		return ports.ApplyResult{}, err
	}
	removeMarketArgv := []string{"claude", "plugin", "marketplace", "remove", marketplaceName}
	if err := a.runClaude(ctx, removeMarketArgv, commandDir); err != nil {
		return ports.ApplyResult{}, err
	}
	if materializedRoot := materializedRootFromRecord(*in.Record); materializedRoot != "" {
		if err := os.RemoveAll(materializedRoot); err != nil && !os.IsNotExist(err) {
			return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "remove Claude managed marketplace root", err)
		}
	}
	return ports.ApplyResult{
		TargetID:        a.ID(),
		State:           domain.InstallRemoved,
		ActivationState: domain.ActivationReloadPending,
		EvidenceClass:   domain.EvidenceConfirmed,
		ReloadRequired:  true,
		ManualSteps:     []string{"run /reload-plugins in Claude Code so the removed plugin disappears from the current session"},
		AdapterMetadata: map[string]any{
			"marketplace_name":      marketplaceName,
			"plugin_ref":            pluginRef,
			"plugin_uninstall_argv": uninstallArgv,
			"marketplace_remove":    removeMarketArgv,
		},
	}, nil
}

func (a Adapter) Repair(ctx context.Context, in ports.RepairInput) (ports.ApplyResult, error) {
	if in.Manifest == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrRepairApply, "Claude repair requires resolved manifest", nil)
	}
	if in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrRepairApply, "Claude repair requires resolved source", nil)
	}
	if seedPath, ok := a.seedManagedMarketplacePath(in.Record.IntegrationID, &in.Record); ok {
		return ports.ApplyResult{}, domain.NewError(domain.ErrRepairApply, "Claude marketplace is seed-managed and read-only: "+seedPath, nil)
	}
	commandDir := a.commandDirForScope(in.Record.Policy.Scope, workspaceRootFromRecord(in.Record))
	materializedRoot, marketplaceName, pluginRef, err := a.syncManagedMarketplace(ctx, *in.Manifest, in.ResolvedSource.LocalPath)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	updateMarketArgv := []string{"claude", "plugin", "marketplace", "update", marketplaceName}
	if err := a.runClaude(ctx, updateMarketArgv, commandDir); err != nil {
		addMarketArgv := []string{"claude", "plugin", "marketplace", "add", materializedRoot}
		if addErr := a.runClaude(ctx, addMarketArgv, commandDir); addErr != nil {
			return ports.ApplyResult{}, addErr
		}
		updateMarketArgv = addMarketArgv
	}
	uninstallArgv := []string{"claude", "plugin", "uninstall", pluginRef}
	if scope := strings.TrimSpace(in.Record.Policy.Scope); scope != "" {
		uninstallArgv = append(uninstallArgv, "--scope", scope)
	}
	_ = a.runClaude(ctx, uninstallArgv, commandDir)
	installArgv := []string{"claude", "plugin", "install", pluginRef}
	if scope := strings.TrimSpace(in.Record.Policy.Scope); scope != "" {
		installArgv = append(installArgv, "--scope", scope)
	}
	if err := a.runClaude(ctx, installArgv, commandDir); err != nil {
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallInstalled,
		ActivationState:    domain.ActivationReloadPending,
		OwnedNativeObjects: a.ownedObjects(in.Manifest.IntegrationID, in.Record.Policy.Scope, workspaceRootFromRecord(in.Record), materializedRoot),
		EvidenceClass:      domain.EvidenceConfirmed,
		ReloadRequired:     true,
		ManualSteps:        []string{"run /reload-plugins in Claude Code so the repaired plugin package is reloaded in the current session"},
		AdapterMetadata: map[string]any{
			"marketplace_name":         marketplaceName,
			"plugin_ref":               pluginRef,
			"materialized_source_root": materializedRoot,
			"marketplace_refresh_argv": updateMarketArgv,
			"plugin_uninstall_argv":    uninstallArgv,
			"plugin_install_argv":      installArgv,
		},
	}, nil
}

func (a Adapter) runClaude(ctx context.Context, argv []string, dir string) error {
	result, err := a.runner().Run(ctx, ports.Command{Argv: argv, Dir: dir})
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "run Claude CLI", err)
	}
	if result.ExitCode != 0 {
		msg := strings.TrimSpace(string(result.Stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(result.Stdout))
		}
		if msg == "" {
			msg = "Claude CLI command failed"
		}
		return domain.NewError(domain.ErrMutationApply, msg, nil)
	}
	return nil
}
