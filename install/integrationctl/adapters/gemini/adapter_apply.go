package gemini

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (a Adapter) PlanInstall(_ context.Context, in ports.PlanInstallInput) (ports.AdapterPlan, error) {
	paths := []string{filepath.Join(a.userHome(), ".gemini", "extensions", in.Manifest.IntegrationID)}
	if in.Manifest.RequestedRef.Kind == "local_path" {
		paths = append(paths, in.Manifest.RequestedRef.Value)
	}
	manualSteps, blocking := a.securityBlockers(in.Manifest, in.Policy.Scope, "")
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
	manualSteps, blocking := a.securityBlockers(in.NextManifest, in.CurrentRecord.Policy.Scope, in.CurrentRecord.WorkspaceRoot)
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
		if err := a.runGemini(ctx, argv, a.commandDirForRecord(*in.Record)); err != nil {
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
		if err := a.runGemini(ctx, argv, a.commandDirForRecord(*in.Record)); err != nil {
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
	switch kind := strings.TrimSpace(in.ResolvedSource.Kind); kind {
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
	if err := writeGeminiInstallMetadata(filepath.Join(tmpRoot, ".gemini-extension-install.json"), managedRoot); err != nil {
		return err
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

func writeGeminiInstallMetadata(path string, source string) error {
	doc := map[string]any{
		"source": strings.TrimSpace(source),
		"type":   "link",
	}
	body, err := marshalJSON(doc)
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "marshal Gemini install metadata", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return domain.NewError(domain.ErrMutationApply, "write Gemini install metadata", err)
	}
	return nil
}
