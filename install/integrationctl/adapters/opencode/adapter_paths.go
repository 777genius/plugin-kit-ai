package opencode

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/safemutate"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (a Adapter) fs() ports.FileSystem {
	if a.FS != nil {
		return a.FS
	}
	return fsadapter.OS{}
}

func (a Adapter) mutator() ports.SafeFileMutator {
	if a.SafeMutator != nil {
		return a.SafeMutator
	}
	return safemutate.OS{}
}

func (a Adapter) configPath(scope string, workspaceRoot string) string {
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		return preferredConfigPath(
			filepath.Join(a.effectiveProjectRoot(workspaceRoot), "opencode.json"),
			filepath.Join(a.effectiveProjectRoot(workspaceRoot), "opencode.jsonc"),
		)
	}
	return preferredConfigPath(
		filepath.Join(a.userHome(), ".config", "opencode", "opencode.json"),
		filepath.Join(a.userHome(), ".config", "opencode", "opencode.jsonc"),
		filepath.Join(a.userHome(), ".local", "share", "opencode", "opencode.jsonc"),
	)
}

func (a Adapter) assetsRoot(scope string, workspaceRoot string) string {
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		return filepath.Join(a.effectiveProjectRoot(workspaceRoot), ".opencode")
	}
	return filepath.Join(a.userHome(), ".config", "opencode")
}

func (a Adapter) projectRoot(workspaceRoot string) string {
	if root := strings.TrimSpace(workspaceRoot); root != "" {
		return filepath.Clean(root)
	}
	if strings.TrimSpace(a.ProjectRoot) != "" {
		return a.ProjectRoot
	}
	cwd, _ := os.Getwd()
	return cwd
}

func (a Adapter) effectiveProjectRoot(workspaceRoot string) string {
	root := filepath.Clean(a.projectRoot(workspaceRoot))
	for {
		if root == "." || root == string(filepath.Separator) || strings.TrimSpace(root) == "" {
			return a.projectRoot(workspaceRoot)
		}
		if fileExists(filepath.Join(root, ".git")) {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			return a.projectRoot(workspaceRoot)
		}
		root = parent
	}
}

func (a Adapter) userHome() string {
	if strings.TrimSpace(a.UserHome) != "" {
		return a.UserHome
	}
	home, _ := os.UserHomeDir()
	return home
}

func preferredConfigPath(candidates ...string) string {
	for _, path := range candidates {
		if fileExists(path) {
			return path
		}
	}
	for _, path := range candidates {
		if strings.TrimSpace(path) != "" {
			return path
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func configPathFromTarget(target domain.TargetInstallation, fallback string) string {
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "file" && strings.TrimSpace(item.Path) != "" && (strings.HasSuffix(item.Path, "opencode.json") || strings.HasSuffix(item.Path, "opencode.jsonc")) {
			return item.Path
		}
	}
	if metadataPath, ok := target.AdapterMetadata["config_path"].(string); ok && strings.TrimSpace(metadataPath) != "" {
		return metadataPath
	}
	return fallback
}

func workspaceRootFromInspectInput(in ports.InspectInput) string {
	if in.Record != nil {
		return workspaceRootFromRecord(*in.Record)
	}
	if strings.EqualFold(strings.TrimSpace(in.Scope), "project") {
		return ""
	}
	return ""
}

func workspaceRootFromApplyInput(in ports.ApplyInput) string {
	if in.Record != nil {
		return workspaceRootFromRecord(*in.Record)
	}
	if strings.EqualFold(strings.TrimSpace(in.Policy.Scope), "project") {
		return ""
	}
	return ""
}

func workspaceRootFromRecord(record domain.InstallationRecord) string {
	if strings.EqualFold(strings.TrimSpace(record.Policy.Scope), "project") {
		return strings.TrimSpace(record.WorkspaceRoot)
	}
	return ""
}

func (a Adapter) assetsRootForPath(path string) string {
	clean := filepath.Clean(path)
	parts := strings.Split(clean, string(filepath.Separator))
	for i, part := range parts {
		if part != ".opencode" {
			continue
		}
		prefix := string(filepath.Separator)
		if i > 0 {
			prefix = filepath.Join(parts[:i+1]...)
			if !strings.HasPrefix(prefix, string(filepath.Separator)) && strings.HasPrefix(clean, string(filepath.Separator)) {
				prefix = string(filepath.Separator) + prefix
			}
		}
		return prefix
	}
	return filepath.Join(a.userHome(), ".config", "opencode")
}

func (a Adapter) removeEmptyParents(path, stop string) {
	dir := filepath.Dir(path)
	stop = filepath.Clean(stop)
	for dir != "." && dir != string(filepath.Separator) {
		if filepath.Clean(dir) == stop {
			_ = a.fs().Remove(context.Background(), dir)
			return
		}
		if err := a.fs().Remove(context.Background(), dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
