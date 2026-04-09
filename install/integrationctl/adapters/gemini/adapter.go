package gemini

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/process"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type Adapter struct {
	Runner   ports.ProcessRunner
	FS       ports.FileSystem
	UserHome string
}

func (Adapter) ID() domain.TargetID { return domain.TargetGemini }

func (Adapter) Capabilities(context.Context) (ports.Capabilities, error) {
	return ports.Capabilities{
		InstallMode:              "native_cli",
		SupportsNativeUpdate:     true,
		SupportsNativeRemove:     true,
		SupportsLinkMode:         true,
		SupportsAutoUpdatePolicy: true,
		SupportsScopeUser:        true,
		SupportsScopeProject:     true,
		SupportsRepair:           true,
		RequiresRestart:          true,
		SupportedSourceKinds:     []string{"local_path", "github_repo_path", "git_url"},
		EvidenceKey:              "target.gemini.native_surface",
	}, nil
}

func (a Adapter) Inspect(ctx context.Context, in ports.InspectInput) (ports.InspectResult, error) {
	home := a.userHome()
	settings := a.settingsPath(scopeFromInspectInput(in))
	workspaceSettings := ""
	if in.Record != nil && strings.EqualFold(strings.TrimSpace(in.Record.Policy.Scope), "project") {
		if cwd, err := os.Getwd(); err == nil {
			workspaceSettings = filepath.Join(cwd, ".gemini", "settings.json")
		}
	}
	trusted := filepath.Join(home, ".gemini", "trustedFolders.json")
	extensionDir := ""
	if in.Record != nil {
		extensionDir = filepath.Join(home, ".gemini", "extensions", in.Record.IntegrationID)
	}
	_, cmdErr := exec.LookPath("gemini")
	_, extErr := os.Stat(extensionDir)
	restrictions := []domain.EnvironmentRestrictionCode{}
	state := domain.InstallRemoved
	if cmdErr != nil {
		restrictions = append(restrictions, domain.RestrictionSourceToolMissing)
	}
	disabled, err := a.isDisabled(ctx, in)
	if err != nil {
		return ports.InspectResult{}, err
	}
	if extErr == nil {
		if disabled {
			state = domain.InstallDisabled
		} else {
			state = domain.InstallInstalled
		}
	}
	settingsFiles := []string{settings, trusted}
	if workspaceSettings != "" {
		settingsFiles = append(settingsFiles, workspaceSettings)
	}
	settingsFiles = append(settingsFiles, a.systemSettingsPaths()...)
	return ports.InspectResult{
		TargetID:                a.ID(),
		Installed:               extErr == nil,
		State:                   state,
		ActivationState:         domain.ActivationRestartPending,
		ConfigPrecedenceContext: []string{"cli", "env", "system_settings", "system_defaults", "workspace", "user"},
		EnvironmentRestrictions: restrictions,
		TrustResolutionSource:   trusted,
		SettingsFiles:           dedupeStrings(settingsFiles),
		EvidenceClass:           domain.EvidenceConfirmed,
	}, nil
}

func (a Adapter) PlanEnable(_ context.Context, in ports.PlanToggleInput) (ports.AdapterPlan, error) {
	return ports.AdapterPlan{
		TargetID:        a.ID(),
		ActionClass:     "enable_target",
		Summary:         "Enable Gemini extension in the native scope",
		RestartRequired: true,
		PathsTouched:    []string{a.settingsPath(scopeFromRecord(in.Record))},
		EvidenceKey:     "target.gemini.native_surface",
	}, nil
}

func (a Adapter) ApplyEnable(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.Record == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Gemini enable requires current record", nil)
	}
	name := strings.TrimSpace(in.Record.IntegrationID)
	if name == "" {
		return ports.ApplyResult{}, domain.NewError(domain.ErrStateConflict, "Gemini enable requires integration identity", nil)
	}
	scope := geminiToggleScope(scopeFromRecord(*in.Record))
	argv := []string{"gemini", "extensions", "enable", name, "--scope", scope}
	if err := a.runGemini(ctx, argv, ""); err != nil {
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallInstalled,
		ActivationState:    domain.ActivationRestartPending,
		OwnedNativeObjects: ownedGeminiObjects(*in.Record, a.userHome()),
		EvidenceClass:      domain.EvidenceConfirmed,
		ManualSteps:        []string{"restart Gemini CLI after enabling the extension"},
		AdapterMetadata: map[string]any{
			"toggle_argv": argv,
			"toggle":      "enable",
		},
	}, nil
}

func (a Adapter) PlanDisable(_ context.Context, in ports.PlanToggleInput) (ports.AdapterPlan, error) {
	return ports.AdapterPlan{
		TargetID:        a.ID(),
		ActionClass:     "disable_target",
		Summary:         "Disable Gemini extension in the native scope",
		RestartRequired: true,
		PathsTouched:    []string{a.settingsPath(scopeFromRecord(in.Record))},
		EvidenceKey:     "target.gemini.native_surface",
	}, nil
}

func (a Adapter) ApplyDisable(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.Record == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Gemini disable requires current record", nil)
	}
	name := strings.TrimSpace(in.Record.IntegrationID)
	if name == "" {
		return ports.ApplyResult{}, domain.NewError(domain.ErrStateConflict, "Gemini disable requires integration identity", nil)
	}
	scope := geminiToggleScope(scopeFromRecord(*in.Record))
	argv := []string{"gemini", "extensions", "disable", name, "--scope", scope}
	if err := a.runGemini(ctx, argv, ""); err != nil {
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallDisabled,
		ActivationState:    domain.ActivationRestartPending,
		OwnedNativeObjects: ownedGeminiObjects(*in.Record, a.userHome()),
		EvidenceClass:      domain.EvidenceConfirmed,
		ManualSteps:        []string{"restart Gemini CLI after disabling the extension"},
		AdapterMetadata: map[string]any{
			"toggle_argv": argv,
			"toggle":      "disable",
		},
	}, nil
}

func (a Adapter) PlanInstall(_ context.Context, in ports.PlanInstallInput) (ports.AdapterPlan, error) {
	paths := []string{filepath.Join(a.userHome(), ".gemini", "extensions", in.Manifest.IntegrationID)}
	if in.Manifest.RequestedRef.Kind == "local_path" {
		paths = append(paths, in.Manifest.RequestedRef.Value)
	}
	manualSteps, blocking := a.securityBlockers(in.Manifest, in.Policy.Scope)
	return ports.AdapterPlan{
		TargetID:        a.ID(),
		ActionClass:     "install_missing",
		Summary:         "Install Gemini extension using native extension workflow",
		RestartRequired: true,
		PathsTouched:    paths,
		ManualSteps:     manualSteps,
		Blocking:        blocking,
		EvidenceKey:     "target.gemini.native_surface",
	}, nil
}

func (a Adapter) ApplyInstall(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Gemini install requires resolved source", nil)
	}
	argv, dir, cleanup, materializedRoot, err := a.installCommand(ctx, in)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	if cleanup != nil {
		defer cleanup()
	}
	if strings.TrimSpace(in.ResolvedSource.Kind) == "local_path" {
		if err := a.activateManagedLocalInstall(materializedRoot, in.Manifest.IntegrationID); err != nil {
			return ports.ApplyResult{}, err
		}
	} else {
		if err := a.runGemini(ctx, argv, dir); err != nil {
			return ports.ApplyResult{}, err
		}
	}
	owned := []domain.NativeObjectRef{{
		Kind:            "extension_dir",
		Path:            filepath.Join(a.userHome(), ".gemini", "extensions", in.Manifest.IntegrationID),
		ProtectionClass: domain.ProtectionUserMutable,
	}}
	metadata := map[string]any{
		"install_argv":   argv,
		"extension_name": in.Manifest.IntegrationID,
		"install_mode":   installModeForKind(in.ResolvedSource.Kind),
	}
	if materializedRoot != "" {
		owned = append(owned, domain.NativeObjectRef{
			Kind:            "managed_source_root",
			Path:            materializedRoot,
			ProtectionClass: domain.ProtectionUserMutable,
		})
		metadata["materialized_source_root"] = materializedRoot
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallInstalled,
		ActivationState:    domain.ActivationRestartPending,
		OwnedNativeObjects: owned,
		EvidenceClass:      domain.EvidenceConfirmed,
		ManualSteps:        []string{"restart Gemini CLI to load the updated extension and merged configuration"},
		AdapterMetadata:    metadata,
	}, nil
}

func (a Adapter) PlanUpdate(_ context.Context, in ports.PlanUpdateInput) (ports.AdapterPlan, error) {
	manualSteps, blocking := a.securityBlockers(in.NextManifest, in.CurrentRecord.Policy.Scope)
	return ports.AdapterPlan{
		TargetID:        a.ID(),
		ActionClass:     "update_version",
		Summary:         "Update Gemini extension",
		RestartRequired: true,
		PathsTouched:    []string{filepath.Join(a.userHome(), ".gemini", "extensions", in.CurrentRecord.IntegrationID)},
		ManualSteps:     manualSteps,
		Blocking:        blocking,
		EvidenceKey:     "target.gemini.native_surface",
	}, nil
}

func (a Adapter) ApplyUpdate(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.Record == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Gemini update requires current record", nil)
	}
	name := strings.TrimSpace(in.Record.IntegrationID)
	if name == "" {
		return ports.ApplyResult{}, domain.NewError(domain.ErrStateConflict, "Gemini update requires integration identity", nil)
	}
	metadata := map[string]any{
		"extension_name":  name,
		"resolved_source": in.Manifest.ResolvedRef.Value,
	}
	owned := []domain.NativeObjectRef{{
		Kind:            "extension_dir",
		Path:            filepath.Join(a.userHome(), ".gemini", "extensions", name),
		ProtectionClass: domain.ProtectionUserMutable,
	}}
	if in.ResolvedSource != nil && strings.TrimSpace(in.ResolvedSource.Kind) == "local_path" {
		materializedRoot, err := a.syncManagedLocalSource(ctx, in.Manifest, in.ResolvedSource.LocalPath)
		if err != nil {
			return ports.ApplyResult{}, err
		}
		if err := a.activateManagedLocalInstall(materializedRoot, name); err != nil {
			return ports.ApplyResult{}, err
		}
		metadata["materialized_source_root"] = materializedRoot
		owned = append(owned, domain.NativeObjectRef{
			Kind:            "managed_source_root",
			Path:            materializedRoot,
			ProtectionClass: domain.ProtectionUserMutable,
		})
		metadata["update_mode"] = "local_projection"
	} else {
		argv := []string{"gemini", "extensions", "update", name}
		if err := a.runGemini(ctx, argv, ""); err != nil {
			return ports.ApplyResult{}, err
		}
		metadata["update_argv"] = argv
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallInstalled,
		ActivationState:    domain.ActivationRestartPending,
		OwnedNativeObjects: owned,
		EvidenceClass:      domain.EvidenceConfirmed,
		ManualSteps:        []string{"restart Gemini CLI after the native extension update completes"},
		AdapterMetadata:    metadata,
	}, nil
}

func (a Adapter) PlanRemove(_ context.Context, in ports.PlanRemoveInput) (ports.AdapterPlan, error) {
	return ports.AdapterPlan{
		TargetID:     a.ID(),
		ActionClass:  "remove_orphaned_target",
		Summary:      "Remove Gemini extension",
		PathsTouched: []string{filepath.Join(a.userHome(), ".gemini", "extensions", in.Record.IntegrationID)},
		EvidenceKey:  "target.gemini.native_surface",
	}, nil
}

func (a Adapter) ApplyRemove(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.Record == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Gemini remove requires current record", nil)
	}
	name := strings.TrimSpace(in.Record.IntegrationID)
	if name == "" {
		return ports.ApplyResult{}, domain.NewError(domain.ErrStateConflict, "Gemini remove requires integration identity", nil)
	}
	metadata := map[string]any{
		"extension_name": name,
	}
	if installModeFromRecord(*in.Record) == "local_projection" || materializedRootFromRecord(*in.Record) != "" {
		if err := os.RemoveAll(a.extensionDir(name)); err != nil && !os.IsNotExist(err) {
			return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "remove Gemini local extension dir", err)
		}
		metadata["remove_mode"] = "local_projection"
	} else {
		argv := []string{"gemini", "extensions", "uninstall", name}
		if err := a.runGemini(ctx, argv, ""); err != nil {
			return ports.ApplyResult{}, err
		}
		metadata["remove_argv"] = argv
	}
	if materializedRoot := materializedRootFromRecord(*in.Record); materializedRoot != "" {
		if err := os.RemoveAll(materializedRoot); err != nil && !os.IsNotExist(err) {
			return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "remove managed Gemini source root", err)
		}
	}
	return ports.ApplyResult{
		TargetID:        a.ID(),
		State:           domain.InstallRemoved,
		ActivationState: domain.ActivationRestartPending,
		EvidenceClass:   domain.EvidenceConfirmed,
		ManualSteps:     []string{"restart Gemini CLI after uninstall so the removed extension is no longer loaded"},
		AdapterMetadata: metadata,
	}, nil
}

func (a Adapter) Repair(ctx context.Context, in ports.RepairInput) (ports.ApplyResult, error) {
	if in.Manifest == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrRepairApply, "Gemini repair requires resolved manifest", nil)
	}
	if in.Record.IntegrationID == "" {
		return ports.ApplyResult{}, domain.NewError(domain.ErrRepairApply, "Gemini repair requires integration identity", nil)
	}
	result, err := a.ApplyUpdate(ctx, ports.ApplyInput{
		Plan:           ports.AdapterPlan{TargetID: a.ID(), ActionClass: "repair_drift", EvidenceKey: "target.gemini.native_surface"},
		Manifest:       *in.Manifest,
		ResolvedSource: in.ResolvedSource,
		Policy:         in.Record.Policy,
		Inspect:        in.Inspect,
		Record:         &in.Record,
	})
	if err != nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrRepairApply, "Gemini repair update failed", err)
	}
	if len(result.ManualSteps) == 0 {
		result.ManualSteps = append(result.ManualSteps, "restart Gemini CLI after repair to reload the updated extension")
	} else {
		result.ManualSteps = append(result.ManualSteps, "repair used native gemini extensions update semantics")
	}
	return result, nil
}

func (a Adapter) installCommand(ctx context.Context, in ports.ApplyInput) ([]string, string, func(), string, error) {
	if in.ResolvedSource == nil {
		return nil, "", nil, "", domain.NewError(domain.ErrMutationApply, "Gemini install requires resolved source", nil)
	}
	kind := strings.TrimSpace(in.ResolvedSource.Kind)
	switch kind {
	case "local_path":
		path, err := a.syncManagedLocalSource(ctx, in.Manifest, in.ResolvedSource.LocalPath)
		if err != nil {
			return nil, "", nil, "", err
		}
		return []string{"gemini", "extensions", "link", path}, "", nil, path, nil
	case "github_repo_path", "git_url":
		argv := []string{"gemini", "extensions", "install", in.Manifest.RequestedRef.Value}
		if in.Policy.AutoUpdate {
			argv = append(argv, "--auto-update")
		}
		if in.Policy.AllowPrerelease {
			argv = append(argv, "--pre-release")
		}
		return argv, "", nil, "", nil
	default:
		return nil, "", nil, "", domain.NewError(domain.ErrMutationApply, "Gemini does not support source kind "+kind, nil)
	}
}

func (a Adapter) syncManagedLocalSource(ctx context.Context, manifest domain.IntegrationManifest, sourceRoot string) (string, error) {
	root := filepath.Clean(sourceRoot)
	managedRoot := filepath.Join(a.userHome(), ".plugin-kit-ai", "materialized", "gemini", manifest.IntegrationID)
	parent := filepath.Dir(managedRoot)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", domain.NewError(domain.ErrMutationApply, "prepare Gemini managed materialization root", err)
	}
	tmpRoot, err := os.MkdirTemp(parent, manifest.IntegrationID+".tmp-*")
	if err != nil {
		return "", domain.NewError(domain.ErrMutationApply, "create Gemini materialization temp root", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmpRoot)
		}
	}()
	if fileExists(filepath.Join(root, "src", "plugin.yaml")) {
		if err := a.materializeAuthoredGeminiSource(ctx, manifest, root, tmpRoot); err != nil {
			return "", err
		}
	} else if fileExists(filepath.Join(root, "gemini-extension.json")) {
		if err := a.copyNativeGeminiPackage(root, tmpRoot); err != nil {
			return "", err
		}
	} else {
		if err := a.materializeAuthoredGeminiSource(ctx, manifest, root, tmpRoot); err != nil {
			return "", err
		}
	}
	if err := os.RemoveAll(managedRoot); err != nil && !os.IsNotExist(err) {
		return "", domain.NewError(domain.ErrMutationApply, "replace Gemini managed source root", err)
	}
	if err := os.Rename(tmpRoot, managedRoot); err != nil {
		return "", domain.NewError(domain.ErrMutationApply, "activate Gemini managed source root", err)
	}
	cleanup = false
	return managedRoot, nil
}

func (a Adapter) runGemini(ctx context.Context, argv []string, dir string) error {
	result, err := a.runner().Run(ctx, ports.Command{Argv: argv, Dir: dir})
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "run Gemini CLI", err)
	}
	if result.ExitCode != 0 {
		msg := strings.TrimSpace(string(result.Stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(result.Stdout))
		}
		if msg == "" {
			msg = "Gemini CLI command failed"
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func installModeForKind(kind string) string {
	if strings.TrimSpace(kind) == "local_path" {
		return "local_projection"
	}
	return "install"
}

func installModeFromRecord(record domain.InstallationRecord) string {
	target, ok := record.Targets[domain.TargetGemini]
	if !ok {
		return ""
	}
	if value, ok := target.AdapterMetadata["install_mode"].(string); ok {
		return strings.TrimSpace(value)
	}
	if value, ok := target.AdapterMetadata["update_mode"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func (a Adapter) extensionDir(name string) string {
	return filepath.Join(a.userHome(), ".gemini", "extensions", name)
}

func (a Adapter) activateManagedLocalInstall(managedRoot, name string) error {
	if strings.TrimSpace(managedRoot) == "" {
		return domain.NewError(domain.ErrMutationApply, "Gemini local projection requires managed source root", nil)
	}
	extensionDir := a.extensionDir(name)
	parent := filepath.Dir(extensionDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return domain.NewError(domain.ErrMutationApply, "prepare Gemini extension dir parent", err)
	}
	tmpRoot, err := os.MkdirTemp(parent, name+".tmp-*")
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "create Gemini extension temp root", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmpRoot)
		}
	}()
	if err := copyDirIfExists(managedRoot, tmpRoot); err != nil {
		return domain.NewError(domain.ErrMutationApply, "copy Gemini managed source into extension dir", err)
	}
	existingEnv := filepath.Join(extensionDir, ".env")
	if fileExists(existingEnv) {
		if err := copyFile(existingEnv, filepath.Join(tmpRoot, ".env")); err != nil {
			return domain.NewError(domain.ErrMutationApply, "preserve Gemini local .env", err)
		}
	}
	if err := os.RemoveAll(extensionDir); err != nil && !os.IsNotExist(err) {
		return domain.NewError(domain.ErrMutationApply, "replace Gemini extension dir", err)
	}
	if err := os.Rename(tmpRoot, extensionDir); err != nil {
		return domain.NewError(domain.ErrMutationApply, "activate Gemini extension dir", err)
	}
	cleanup = false
	return nil
}

func materializedRootFromRecord(record domain.InstallationRecord) string {
	target, ok := record.Targets[domain.TargetGemini]
	if !ok {
		return ""
	}
	if value, ok := target.AdapterMetadata["materialized_source_root"].(string); ok && strings.TrimSpace(value) != "" {
		return filepath.Clean(value)
	}
	for _, obj := range target.OwnedNativeObjects {
		if obj.Kind == "managed_source_root" && strings.TrimSpace(obj.Path) != "" {
			return filepath.Clean(obj.Path)
		}
	}
	return ""
}

func scopeFromInspectInput(in ports.InspectInput) string {
	if in.Record != nil {
		return scopeFromRecord(*in.Record)
	}
	return defaultScope(in.Scope)
}

func scopeFromRecord(record domain.InstallationRecord) string {
	return defaultScope(record.Policy.Scope)
}

func defaultScope(scope string) string {
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		return "project"
	}
	return "user"
}

func geminiToggleScope(scope string) string {
	if scope == "project" {
		return "workspace"
	}
	return "user"
}

func (a Adapter) settingsPath(scope string) string {
	if scope == "project" {
		if cwd, err := os.Getwd(); err == nil {
			return filepath.Join(cwd, ".gemini", "settings.json")
		}
	}
	return filepath.Join(a.userHome(), ".gemini", "settings.json")
}

func (a Adapter) isDisabled(ctx context.Context, in ports.InspectInput) (bool, error) {
	if in.Record == nil {
		return false, nil
	}
	settingsPath := a.settingsPath(scopeFromInspectInput(in))
	body, err := a.fs().ReadFile(ctx, settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, domain.NewError(domain.ErrMutationApply, "read Gemini settings during inspect", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return false, domain.NewError(domain.ErrMutationApply, "parse Gemini settings during inspect", err)
	}
	extensions, ok := doc["extensions"].(map[string]any)
	if !ok || extensions == nil {
		return false, nil
	}
	raw, ok := extensions["disabled"].([]any)
	if !ok {
		return false, nil
	}
	name := strings.TrimSpace(in.Record.IntegrationID)
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) == name {
			return true, nil
		}
	}
	return false, nil
}

func ownedGeminiObjects(record domain.InstallationRecord, home string) []domain.NativeObjectRef {
	target, ok := record.Targets[domain.TargetGemini]
	if !ok {
		return nil
	}
	if len(target.OwnedNativeObjects) > 0 {
		return append([]domain.NativeObjectRef(nil), target.OwnedNativeObjects...)
	}
	owned := []domain.NativeObjectRef{{
		Kind:            "extension_dir",
		Path:            filepath.Join(home, ".gemini", "extensions", record.IntegrationID),
		ProtectionClass: domain.ProtectionUserMutable,
	}}
	if root := materializedRootFromRecord(record); root != "" {
		owned = append(owned, domain.NativeObjectRef{
			Kind:            "managed_source_root",
			Path:            root,
			ProtectionClass: domain.ProtectionUserMutable,
		})
	}
	return owned
}

func (a Adapter) systemSettingsPaths() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Library/Application Support/GeminiCli/system-defaults.json",
			"/Library/Application Support/GeminiCli/settings.json",
		}
	case "windows":
		programData := strings.TrimSpace(os.Getenv("ProgramData"))
		if programData == "" {
			programData = `C:\ProgramData`
		}
		return []string{
			filepath.Join(programData, "gemini-cli", "system-defaults.json"),
			filepath.Join(programData, "gemini-cli", "settings.json"),
		}
	default:
		return []string{
			"/etc/gemini-cli/system-defaults.json",
			"/etc/gemini-cli/settings.json",
		}
	}
}

func (a Adapter) securityBlockers(manifest domain.IntegrationManifest, scope string) ([]string, bool) {
	if !isGitBackedGeminiSource(manifest.RequestedRef.Kind) {
		return nil, false
	}
	settings, err := a.loadMergedSettings(scope)
	if err != nil {
		return []string{"inspect Gemini settings manually before installing or updating this Git-backed extension"}, true
	}
	security, _ := settings["security"].(map[string]any)
	if len(security) == 0 {
		return nil, false
	}
	source := strings.TrimSpace(manifest.RequestedRef.Value)
	allowed := stringSliceFromAny(security["allowedExtensions"])
	if len(allowed) > 0 {
		for _, pattern := range allowed {
			re, err := regexp.Compile(pattern)
			if err != nil {
				continue
			}
			if re.MatchString(source) {
				return nil, false
			}
		}
		return []string{
			"Gemini security.allowedExtensions does not permit this extension source",
			"adjust the allowlist in Gemini settings or choose an allowed source",
		}, true
	}
	if truthyBool(security["blockGitExtensions"]) {
		return []string{
			"Gemini security.blockGitExtensions is enabled for Git-backed extensions",
			"disable that policy or use a local extension source instead",
		}, true
	}
	return nil, false
}

func (a Adapter) loadMergedSettings(scope string) (map[string]any, error) {
	paths := []string{filepath.Join(a.userHome(), ".gemini", "settings.json")}
	if scope == "project" {
		if cwd, err := os.Getwd(); err == nil {
			paths = append(paths, filepath.Join(cwd, ".gemini", "settings.json"))
		}
	}
	paths = append(paths, a.systemSettingsPaths()...)
	merged := map[string]any{}
	for _, path := range paths {
		body, err := a.fs().ReadFile(context.Background(), path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, domain.NewError(domain.ErrMutationApply, "read Gemini settings", err)
		}
		var doc map[string]any
		if err := json.Unmarshal(body, &doc); err != nil {
			return nil, domain.NewError(domain.ErrMutationApply, "parse Gemini settings", err)
		}
		merged = mergeSettingsMaps(merged, doc)
	}
	return merged, nil
}

func mergeSettingsMaps(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	for key, value := range src {
		existing, hasExisting := dst[key]
		nextMap, nextIsMap := value.(map[string]any)
		prevMap, prevIsMap := existing.(map[string]any)
		if hasExisting && nextIsMap && prevIsMap {
			dst[key] = mergeSettingsMaps(prevMap, nextMap)
			continue
		}
		dst[key] = value
	}
	return dst
}

func isGitBackedGeminiSource(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "git_url", "github_repo_path":
		return true
	default:
		return false
	}
}

func stringSliceFromAny(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		if typed, ok := v.([]string); ok {
			return append([]string(nil), typed...)
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func truthyBool(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

func dedupeStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
