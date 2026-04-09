package gemini

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

func (a Adapter) Inspect(_ context.Context, in ports.InspectInput) (ports.InspectResult, error) {
	home := a.userHome()
	settings := filepath.Join(home, ".gemini", "settings.json")
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
	_, statErr := os.Stat(settings)
	_, extErr := os.Stat(extensionDir)
	restrictions := []domain.EnvironmentRestrictionCode{}
	state := domain.InstallRemoved
	if cmdErr != nil && statErr != nil && extErr != nil {
		restrictions = append(restrictions, domain.RestrictionSourceToolMissing)
	}
	if statErr == nil || extErr == nil || cmdErr == nil {
		state = domain.InstallInstalled
	}
	settingsFiles := []string{settings, trusted}
	if workspaceSettings != "" {
		settingsFiles = append(settingsFiles, workspaceSettings)
	}
	return ports.InspectResult{
		TargetID:                a.ID(),
		Installed:               extErr == nil,
		State:                   state,
		ActivationState:         domain.ActivationRestartPending,
		ConfigPrecedenceContext: []string{"cli", "env", "system_settings", "workspace", "user"},
		EnvironmentRestrictions: restrictions,
		TrustResolutionSource:   trusted,
		SettingsFiles:           settingsFiles,
		EvidenceClass:           domain.EvidenceConfirmed,
	}, nil
}

func (a Adapter) PlanInstall(_ context.Context, in ports.PlanInstallInput) (ports.AdapterPlan, error) {
	paths := []string{filepath.Join(a.userHome(), ".gemini", "extensions", in.Manifest.IntegrationID)}
	if in.Manifest.RequestedRef.Kind == "local_path" {
		paths = append(paths, in.Manifest.RequestedRef.Value)
	}
	return ports.AdapterPlan{
		TargetID:        a.ID(),
		ActionClass:     "install_missing",
		Summary:         "Install Gemini extension using native extension workflow",
		RestartRequired: true,
		PathsTouched:    paths,
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
	if err := a.runGemini(ctx, argv, dir); err != nil {
		return ports.ApplyResult{}, err
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
	return ports.AdapterPlan{
		TargetID:        a.ID(),
		ActionClass:     "update_version",
		Summary:         "Update Gemini extension",
		RestartRequired: true,
		PathsTouched:    []string{filepath.Join(a.userHome(), ".gemini", "extensions", in.CurrentRecord.IntegrationID)},
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
		metadata["materialized_source_root"] = materializedRoot
		owned = append(owned, domain.NativeObjectRef{
			Kind:            "managed_source_root",
			Path:            materializedRoot,
			ProtectionClass: domain.ProtectionUserMutable,
		})
	}
	argv := []string{"gemini", "extensions", "update", name}
	if err := a.runGemini(ctx, argv, ""); err != nil {
		return ports.ApplyResult{}, err
	}
	metadata["update_argv"] = argv
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
	argv := []string{"gemini", "extensions", "uninstall", name}
	if err := a.runGemini(ctx, argv, ""); err != nil {
		return ports.ApplyResult{}, err
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
		AdapterMetadata: map[string]any{
			"remove_argv":    argv,
			"extension_name": name,
		},
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
		return []string{"gemini", "extensions", "install", path}, "", nil, path, nil
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
	if fileExists(filepath.Join(root, "gemini-extension.json")) {
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
	return "install"
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
