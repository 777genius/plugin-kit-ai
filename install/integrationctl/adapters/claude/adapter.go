package claude

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	scope := scopeForInspect(in)
	settings := a.settingsPath(scope)
	_, cmdErr := exec.LookPath("claude")
	_, statErr := os.Stat(settings)
	restrictions := []domain.EnvironmentRestrictionCode{}
	settingsFiles := []string{settings}
	state := domain.InstallRemoved
	integrationID := strings.TrimSpace(in.IntegrationID)
	if integrationID == "" && in.Record != nil {
		integrationID = strings.TrimSpace(in.Record.IntegrationID)
	}
	if cmdErr != nil && statErr != nil {
		restrictions = append(restrictions, domain.RestrictionSourceToolMissing)
	}
	if managedPath, managed, ok := a.readManagedSettings(scope); ok {
		settingsFiles = append(settingsFiles, managedPath)
		if managed.blocksAllMarketplaceAdds() {
			restrictions = append(restrictions, domain.RestrictionManagedPolicyBlock)
		} else if integrationID != "" {
			if blocked, _ := a.marketplaceAddBlocked(scope, integrationID); blocked {
				restrictions = append(restrictions, domain.RestrictionManagedPolicyBlock)
			}
		}
	}
	if integrationID != "" {
		if seedPath, ok := a.seedManagedMarketplacePath(integrationID, in.Record); ok {
			settingsFiles = append(settingsFiles, seedPath)
			restrictions = append(restrictions, domain.RestrictionReadOnlyNativeLayer)
		}
	}
	if hasRestriction(restrictions, domain.RestrictionManagedPolicyBlock) {
		restrictions = dedupeRestrictions(restrictions)
	}
	if hasRestriction(restrictions, domain.RestrictionReadOnlyNativeLayer) {
		restrictions = dedupeRestrictions(restrictions)
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
		SettingsFiles:           settingsFiles,
		EvidenceClass:           domain.EvidenceConfirmed,
	}, nil
}

func hasRestriction(items []domain.EnvironmentRestrictionCode, want domain.EnvironmentRestrictionCode) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func dedupeRestrictions(items []domain.EnvironmentRestrictionCode) []domain.EnvironmentRestrictionCode {
	seen := map[domain.EnvironmentRestrictionCode]struct{}{}
	out := make([]domain.EnvironmentRestrictionCode, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func (a Adapter) PlanInstall(_ context.Context, in ports.PlanInstallInput) (ports.AdapterPlan, error) {
	marketplaceRoot := managedMarketplaceRoot(a.userHome(), in.Manifest.IntegrationID)
	settings := a.settingsPath(in.Policy.Scope)
	manual, blocking := blockingSteps(in.Inspect)
	if blocked, message := a.marketplaceAddBlocked(in.Policy.Scope, in.Manifest.IntegrationID); blocked {
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
	manual, blocking := blockingSteps(in.Inspect)
	return ports.AdapterPlan{
		TargetID:       a.ID(),
		ActionClass:    "update_version",
		Summary:        "Refresh managed Claude marketplace and reinstall the plugin",
		ReloadRequired: true,
		PathsTouched: []string{
			managedMarketplaceRoot(a.userHome(), in.CurrentRecord.IntegrationID),
			a.settingsPath(in.CurrentRecord.Policy.Scope),
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
	manual, blocking := blockingSteps(in.Inspect)
	return ports.AdapterPlan{
		TargetID:       a.ID(),
		ActionClass:    "remove_orphaned_target",
		Summary:        "Uninstall Claude plugin and remove the managed local marketplace",
		ReloadRequired: true,
		PathsTouched: []string{
			managedMarketplaceRoot(a.userHome(), in.Record.IntegrationID),
			a.settingsPath(in.Record.Policy.Scope),
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
	if in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrRepairApply, "Claude repair requires resolved source", nil)
	}
	if seedPath, ok := a.seedManagedMarketplacePath(in.Record.IntegrationID, &in.Record); ok {
		return ports.ApplyResult{}, domain.NewError(domain.ErrRepairApply, "Claude marketplace is seed-managed and read-only: "+seedPath, nil)
	}
	materializedRoot, marketplaceName, pluginRef, err := a.syncManagedMarketplace(ctx, *in.Manifest, in.ResolvedSource.LocalPath)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	updateMarketArgv := []string{"claude", "plugin", "marketplace", "update", marketplaceName}
	if err := a.runClaude(ctx, updateMarketArgv, ""); err != nil {
		addMarketArgv := []string{"claude", "plugin", "marketplace", "add", materializedRoot}
		if addErr := a.runClaude(ctx, addMarketArgv, ""); addErr != nil {
			return ports.ApplyResult{}, addErr
		}
		updateMarketArgv = addMarketArgv
	}
	uninstallArgv := []string{"claude", "plugin", "uninstall", pluginRef}
	if scope := strings.TrimSpace(in.Record.Policy.Scope); scope != "" {
		uninstallArgv = append(uninstallArgv, "--scope", scope)
	}
	_ = a.runClaude(ctx, uninstallArgv, "")
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
	for _, restriction := range inspect.EnvironmentRestrictions {
		if restriction == domain.RestrictionReadOnlyNativeLayer {
			return []string{"this Claude marketplace is seed-managed and read-only; ask an administrator to update the seed image instead of mutating it locally"}, true
		}
		if restriction == domain.RestrictionManagedPolicyBlock {
			return []string{"managed settings block adding this Claude marketplace; ask an administrator to update the allowlist or seed configuration"}, true
		}
	}
	return []string{"install or configure Claude Code before applying this integration"}, true
}

type managedSettings struct {
	StrictKnownMarketplaces json.RawMessage `json:"strictKnownMarketplaces"`
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

func (a Adapter) readManagedSettings(scope string) (string, managedSettings, bool) {
	for _, candidate := range a.managedSettingsCandidates(scope) {
		body, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		var doc managedSettings
		if err := json.Unmarshal(body, &doc); err != nil {
			continue
		}
		return candidate, doc, true
	}
	return "", managedSettings{}, false
}

func (a Adapter) managedSettingsCandidates(scope string) []string {
	candidates := []string{}
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "project" {
		root := strings.TrimSpace(a.ProjectRoot)
		if root == "" {
			if cwd, err := os.Getwd(); err == nil {
				root = cwd
			}
		}
		if root != "" {
			candidates = append(candidates, filepath.Join(root, ".claude", "managed-settings.json"))
		}
	}
	candidates = append(candidates,
		filepath.Join(a.userHome(), ".claude", "managed-settings.json"),
		"/etc/claude-code/managed-settings.json",
	)
	return candidates
}

func (m managedSettings) blocksAllMarketplaceAdds() bool {
	raw := strings.TrimSpace(string(m.StrictKnownMarketplaces))
	return raw == "[]"
}

func (a Adapter) marketplaceAddBlocked(scope, integrationID string) (bool, string) {
	_, managed, ok := a.readManagedSettings(scope)
	if !ok {
		return false, ""
	}
	raw := strings.TrimSpace(string(managed.StrictKnownMarketplaces))
	if raw == "" || raw == "null" {
		return false, ""
	}
	if raw == "[]" {
		return true, "managed settings set strictKnownMarketplaces to an empty allowlist, so no new Claude marketplaces can be added"
	}
	var allowlist []map[string]any
	if err := json.Unmarshal(managed.StrictKnownMarketplaces, &allowlist); err != nil {
		return false, ""
	}
	managedRoot := managedMarketplaceRoot(a.userHome(), integrationID)
	for _, entry := range allowlist {
		source, _ := entry["source"].(string)
		if source != "pathPattern" {
			continue
		}
		pattern, _ := entry["pathPattern"].(string)
		if pattern == "" {
			continue
		}
		if re, err := regexp.Compile(pattern); err == nil && re.MatchString(managedRoot) {
			return false, ""
		}
	}
	return true, "managed strictKnownMarketplaces does not allow the integrationctl-managed Claude marketplace path; ask an administrator to allow this path pattern or pre-seed the marketplace"
}

func (a Adapter) seedManagedMarketplacePath(integrationID string, record *domain.InstallationRecord) (string, bool) {
	marketplaceName := managedMarketplaceName(integrationID)
	if record != nil {
		if value := marketplaceNameFromRecord(*record); value != "" {
			marketplaceName = value
		}
	}
	seedDirs := strings.TrimSpace(os.Getenv("CLAUDE_CODE_PLUGIN_SEED_DIR"))
	if seedDirs == "" {
		return "", false
	}
	for _, root := range strings.Split(seedDirs, string(os.PathListSeparator)) {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		path := filepath.Join(root, "marketplaces", marketplaceName)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}
