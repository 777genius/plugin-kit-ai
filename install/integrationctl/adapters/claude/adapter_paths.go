package claude

import (
	"os"
	"path/filepath"
	"strings"

	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/process"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

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

func (a Adapter) settingsPath(scope string, workspaceRoot string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	switch scope {
	case "project":
		root := effectiveWorkspaceRoot(workspaceRoot, a.ProjectRoot)
		return filepath.Join(root, ".claude", "settings.json")
	default:
		return filepath.Join(a.userHome(), ".claude", "settings.json")
	}
}

func (a Adapter) ownedObjects(integrationID, scope, workspaceRoot, materializedRoot string) []domain.NativeObjectRef {
	return []domain.NativeObjectRef{
		{
			Kind:            "managed_marketplace_root",
			Path:            materializedRoot,
			ProtectionClass: domain.ProtectionUserMutable,
		},
		{
			Kind:            "settings_file",
			Path:            a.settingsPath(scope, workspaceRoot),
			ProtectionClass: protectionForScope(scope),
		},
	}
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

func effectiveWorkspaceRoot(workspaceRoot string, projectRoot string) string {
	if root := strings.TrimSpace(workspaceRoot); root != "" {
		return filepath.Clean(root)
	}
	if root := strings.TrimSpace(projectRoot); root != "" {
		return filepath.Clean(root)
	}
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Clean(cwd)
	}
	return ""
}

func (a Adapter) commandDirForScope(scope string, workspaceRoot string) string {
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		return effectiveWorkspaceRoot(workspaceRoot, a.ProjectRoot)
	}
	return ""
}
