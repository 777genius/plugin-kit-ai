package claude

import (
	"context"
	"os"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

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
	uninstallArgv := scopedPluginArgv([]string{"claude", "plugin", "uninstall", pluginRef}, in.Record.Policy.Scope)
	if err := a.runClaude(ctx, uninstallArgv, commandDir); err != nil {
		return ports.ApplyResult{}, err
	}
	removeMarketplaceArgv := []string{"claude", "plugin", "marketplace", "remove", marketplaceName}
	if err := a.runClaude(ctx, removeMarketplaceArgv, commandDir); err != nil {
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
			"marketplace_remove":    removeMarketplaceArgv,
		},
	}, nil
}
