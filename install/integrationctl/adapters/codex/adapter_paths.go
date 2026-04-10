package codex

import (
	"os"
	"path/filepath"
	"strings"

	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type codexSurfacePaths struct {
	Scope         string
	WorkspaceRoot string
	CatalogPath   string
	PluginRoot    string
	ConfigPath    string
}

func (a Adapter) fs() ports.FileSystem {
	if a.FS != nil {
		return a.FS
	}
	return fsadapter.OS{}
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

func normalizedScope(scope string) string {
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		return "project"
	}
	return "user"
}

func (a Adapter) marketplaceRoot(scope string, workspaceRoot string) string {
	if normalizedScope(scope) == "project" {
		return filepath.Join(a.effectiveProjectRoot(workspaceRoot), ".agents", "plugins")
	}
	return filepath.Join(a.userHome(), ".agents", "plugins")
}

func (a Adapter) catalogPath(scope string, workspaceRoot string) string {
	return filepath.Join(a.marketplaceRoot(scope, workspaceRoot), "marketplace.json")
}

func (a Adapter) pluginRoot(scope, workspaceRoot, integrationID string) string {
	return filepath.Join(a.marketplaceRoot(scope, workspaceRoot), "plugins", integrationID)
}

func (a Adapter) cachePath(marketplaceName, integrationID string) string {
	return filepath.Join(a.userHome(), ".codex", "plugins", "cache", marketplaceName, integrationID, "local")
}

func (a Adapter) pathsForScope(scope, workspaceRoot, integrationID string) codexSurfacePaths {
	scope = normalizedScope(scope)
	return codexSurfacePaths{
		Scope:         scope,
		WorkspaceRoot: workspaceRoot,
		CatalogPath:   a.catalogPath(scope, workspaceRoot),
		PluginRoot:    a.pluginRoot(scope, workspaceRoot, integrationID),
		ConfigPath:    filepath.Join(a.userHome(), ".codex", "config.toml"),
	}
}

func (a Adapter) pathsForRecord(record domain.InstallationRecord) codexSurfacePaths {
	workspaceRoot := workspaceRootFromRecord(record)
	paths := a.pathsForScope(record.Policy.Scope, workspaceRoot, record.IntegrationID)
	target, ok := record.Targets[domain.TargetCodex]
	if !ok {
		return paths
	}
	paths.CatalogPath = catalogPathFromTarget(target, paths.CatalogPath)
	paths.PluginRoot = pluginRootFromTarget(target, paths.PluginRoot)
	return paths
}

func workspaceRootFromInspectInput(in ports.InspectInput) string {
	if in.Record != nil {
		return workspaceRootFromRecord(*in.Record)
	}
	return ""
}

func workspaceRootFromApplyInput(in ports.ApplyInput) string {
	if in.Record != nil {
		return workspaceRootFromRecord(*in.Record)
	}
	return ""
}

func workspaceRootFromRecord(record domain.InstallationRecord) string {
	if strings.EqualFold(strings.TrimSpace(record.Policy.Scope), "project") {
		return strings.TrimSpace(record.WorkspaceRoot)
	}
	return ""
}

func protectionForScope(scope string) domain.ProtectionClass {
	if normalizedScope(scope) == "project" {
		return domain.ProtectionWorkspace
	}
	return domain.ProtectionUserMutable
}

func ownedObjects(scope, catalogPath, pluginRoot, pluginName string) []domain.NativeObjectRef {
	return []domain.NativeObjectRef{
		{
			Kind:            "marketplace_catalog",
			Path:            catalogPath,
			ProtectionClass: protectionForScope(scope),
		},
		{
			Kind:            "marketplace_entry",
			Name:            pluginName,
			Path:            catalogPath,
			ProtectionClass: protectionForScope(scope),
		},
		{
			Kind:            "plugin_root",
			Name:            pluginName,
			Path:            pluginRoot,
			ProtectionClass: protectionForScope(scope),
		},
	}
}

func pluginRootFromRecord(record *domain.InstallationRecord) string {
	if record == nil {
		return ""
	}
	target, ok := record.Targets[domain.TargetCodex]
	if !ok {
		return ""
	}
	return pluginRootFromTarget(target, "")
}

func pluginRootFromTarget(target domain.TargetInstallation, fallback string) string {
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "plugin_root" && strings.TrimSpace(item.Path) != "" {
			return item.Path
		}
	}
	return fallback
}

func catalogPathFromTarget(target domain.TargetInstallation, fallback string) string {
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "marketplace_catalog" && strings.TrimSpace(item.Path) != "" {
			return item.Path
		}
	}
	return fallback
}

func marketplaceNameFromRecord(record *domain.InstallationRecord) string {
	if record == nil {
		return ""
	}
	target, ok := record.Targets[domain.TargetCodex]
	if !ok || target.AdapterMetadata == nil {
		return ""
	}
	value, _ := target.AdapterMetadata["catalog_name"].(string)
	return strings.TrimSpace(value)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
