package claude

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/process"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type Adapter struct {
	Runner      ports.ProcessRunner
	FS          ports.FileSystem
	ProjectRoot string
	UserHome    string
}

func (Adapter) ID() domain.TargetID { return domain.TargetClaude }

func (Adapter) Capabilities(context.Context) (ports.Capabilities, error) {
	return ports.Capabilities{
		InstallMode:              "native_cli",
		SupportsNativeUpdate:     true,
		SupportsNativeRemove:     true,
		SupportsAutoUpdatePolicy: true,
		SupportsScopeUser:        true,
		SupportsScopeProject:     true,
		RequiresReload:           true,
		SupportedSourceKinds:     []string{"local_path", "github_repo_path", "git_url"},
		EvidenceKey:              "target.claude.native_surface",
	}, nil
}

func (a Adapter) Inspect(_ context.Context, in ports.InspectInput) (ports.InspectResult, error) {
	settings := a.settingsPath(scopeForInspect(in))
	_, cmdErr := exec.LookPath("claude")
	_, statErr := os.Stat(settings)
	restrictions := []domain.EnvironmentRestrictionCode{}
	state := domain.InstallRemoved
	if cmdErr != nil && statErr != nil {
		restrictions = append(restrictions, domain.RestrictionSourceToolMissing)
	}
	if statErr == nil || cmdErr == nil {
		state = domain.InstallInstalled
	}
	return ports.InspectResult{
		TargetID:                a.ID(),
		Installed:               statErr == nil,
		State:                   state,
		ActivationState:         domain.ActivationReloadPending,
		ConfigPrecedenceContext: []string{"project", "user", "managed"},
		EnvironmentRestrictions: restrictions,
		SettingsFiles:           []string{settings},
		EvidenceClass:           domain.EvidenceConfirmed,
	}, nil
}

func (a Adapter) PlanInstall(_ context.Context, in ports.PlanInstallInput) (ports.AdapterPlan, error) {
	marketplaceRoot := managedMarketplaceRoot(a.userHome(), in.Manifest.IntegrationID)
	settings := a.settingsPath(in.Policy.Scope)
	manual, blocking := blockingSteps(in.Inspect)
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
	materializedRoot, marketplaceName, pluginRef, err := a.syncManagedMarketplace(ctx, in.Manifest, in.ResolvedSource.LocalPath)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	if err := a.runClaude(ctx, []string{"claude", "plugin", "marketplace", "add", materializedRoot}, ""); err != nil {
		return ports.ApplyResult{}, err
	}
	installArgv := []string{"claude", "plugin", "install", pluginRef}
	if scope := strings.TrimSpace(in.Policy.Scope); scope != "" {
		installArgv = append(installArgv, "--scope", scope)
	}
	if err := a.runClaude(ctx, installArgv, ""); err != nil {
		_ = a.runClaude(ctx, []string{"claude", "plugin", "marketplace", "remove", marketplaceName}, "")
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallInstalled,
		ActivationState:    domain.ActivationReloadPending,
		OwnedNativeObjects: a.ownedObjects(in.Manifest.IntegrationID, in.Policy.Scope, materializedRoot),
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
	return ports.AdapterPlan{
		TargetID:       a.ID(),
		ActionClass:    "update_version",
		Summary:        "Refresh managed Claude marketplace and reinstall the plugin",
		ReloadRequired: true,
		PathsTouched: []string{
			managedMarketplaceRoot(a.userHome(), in.CurrentRecord.IntegrationID),
			a.settingsPath(in.CurrentRecord.Policy.Scope),
		},
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
	materializedRoot, marketplaceName, pluginRef, err := a.syncManagedMarketplace(ctx, in.Manifest, in.ResolvedSource.LocalPath)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	updateArgv := []string{"claude", "plugin", "marketplace", "update", marketplaceName}
	if err := a.runClaude(ctx, updateArgv, ""); err != nil {
		return ports.ApplyResult{}, err
	}
	uninstallArgv := []string{"claude", "plugin", "uninstall", pluginRef}
	if scope := strings.TrimSpace(in.Record.Policy.Scope); scope != "" {
		uninstallArgv = append(uninstallArgv, "--scope", scope)
	}
	if err := a.runClaude(ctx, uninstallArgv, ""); err != nil {
		return ports.ApplyResult{}, err
	}
	installArgv := []string{"claude", "plugin", "install", pluginRef}
	if scope := strings.TrimSpace(in.Record.Policy.Scope); scope != "" {
		installArgv = append(installArgv, "--scope", scope)
	}
	if err := a.runClaude(ctx, installArgv, ""); err != nil {
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallInstalled,
		ActivationState:    domain.ActivationReloadPending,
		OwnedNativeObjects: a.ownedObjects(in.Manifest.IntegrationID, in.Record.Policy.Scope, materializedRoot),
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
	return ports.AdapterPlan{
		TargetID:       a.ID(),
		ActionClass:    "remove_orphaned_target",
		Summary:        "Uninstall Claude plugin and remove the managed local marketplace",
		ReloadRequired: true,
		PathsTouched: []string{
			managedMarketplaceRoot(a.userHome(), in.Record.IntegrationID),
			a.settingsPath(in.Record.Policy.Scope),
		},
		EvidenceKey: "target.claude.native_surface",
	}, nil
}

func (a Adapter) ApplyRemove(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.Record == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Claude remove requires current record", nil)
	}
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
	if err := a.runClaude(ctx, uninstallArgv, ""); err != nil {
		return ports.ApplyResult{}, err
	}
	removeMarketArgv := []string{"claude", "plugin", "marketplace", "remove", marketplaceName}
	if err := a.runClaude(ctx, removeMarketArgv, ""); err != nil {
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
	record := in.Record
	return a.ApplyUpdate(ctx, ports.ApplyInput{
		Manifest:       *in.Manifest,
		ResolvedSource: in.ResolvedSource,
		Record:         &record,
	})
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

func (a Adapter) runner() ports.ProcessRunner {
	if a.Runner != nil {
		return a.Runner
	}
	return process.OS{}
}

func (a Adapter) fs() ports.FileSystem {
	if a.FS != nil {
		return a.FS
	}
	return fsadapter.OS{}
}

func (a Adapter) userHome() string {
	if strings.TrimSpace(a.UserHome) != "" {
		return a.UserHome
	}
	home, _ := os.UserHomeDir()
	return home
}

func (a Adapter) settingsPath(scope string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	switch scope {
	case "project":
		root := strings.TrimSpace(a.ProjectRoot)
		if root == "" {
			if cwd, err := os.Getwd(); err == nil {
				root = cwd
			}
		}
		return filepath.Join(root, ".claude", "settings.json")
	default:
		return filepath.Join(a.userHome(), ".claude", "settings.json")
	}
}

func (a Adapter) ownedObjects(integrationID, scope, materializedRoot string) []domain.NativeObjectRef {
	return []domain.NativeObjectRef{
		{
			Kind:            "managed_marketplace_root",
			Path:            materializedRoot,
			ProtectionClass: domain.ProtectionUserMutable,
		},
		{
			Kind:            "settings_file",
			Path:            a.settingsPath(scope),
			ProtectionClass: protectionForScope(scope),
		},
	}
}

func scopeForInspect(in ports.InspectInput) string {
	if in.Record != nil {
		return in.Record.Policy.Scope
	}
	return in.Scope
}

func blockingSteps(inspect ports.InspectResult) ([]string, bool) {
	if len(inspect.EnvironmentRestrictions) == 0 {
		return nil, false
	}
	return []string{"install or configure Claude Code before applying this integration"}, true
}

func protectionForScope(scope string) domain.ProtectionClass {
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		return domain.ProtectionWorkspace
	}
	return domain.ProtectionUserMutable
}

func managedMarketplaceRoot(home, integrationID string) string {
	return filepath.Join(home, ".plugin-kit-ai", "materialized", "claude", integrationID)
}

func managedMarketplaceName(integrationID string) string {
	return "integrationctl-" + strings.ToLower(strings.TrimSpace(integrationID))
}

func marketplaceNameFromRecord(record domain.InstallationRecord) string {
	target, ok := record.Targets[domain.TargetClaude]
	if !ok {
		return ""
	}
	if value, ok := target.AdapterMetadata["marketplace_name"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return ""
}

func pluginRefFromRecord(record domain.InstallationRecord) string {
	target, ok := record.Targets[domain.TargetClaude]
	if !ok {
		return ""
	}
	if value, ok := target.AdapterMetadata["plugin_ref"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return ""
}

func materializedRootFromRecord(record domain.InstallationRecord) string {
	target, ok := record.Targets[domain.TargetClaude]
	if !ok {
		return ""
	}
	if value, ok := target.AdapterMetadata["materialized_source_root"].(string); ok && strings.TrimSpace(value) != "" {
		return filepath.Clean(value)
	}
	for _, obj := range target.OwnedNativeObjects {
		if obj.Kind == "managed_marketplace_root" && strings.TrimSpace(obj.Path) != "" {
			return filepath.Clean(obj.Path)
		}
	}
	return ""
}
