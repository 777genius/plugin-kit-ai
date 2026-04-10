package gemini

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

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
