package cursor

import (
	"os"
	"path/filepath"
	"sort"
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

func (a Adapter) targetConfigPath(scope string, workspaceRoot string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "project" {
		return filepath.Join(a.projectRoot(workspaceRoot), ".cursor", "mcp.json")
	}
	return filepath.Join(a.userHome(), ".cursor", "mcp.json")
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

func (a Adapter) userHome() string {
	if strings.TrimSpace(a.UserHome) != "" {
		return a.UserHome
	}
	home, _ := os.UserHomeDir()
	return home
}

func protectionForScope(scope string) domain.ProtectionClass {
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		return domain.ProtectionWorkspace
	}
	return domain.ProtectionUserMutable
}

func ownedObjectsForConfig(path string, aliases []string, protection domain.ProtectionClass) []domain.NativeObjectRef {
	out := make([]domain.NativeObjectRef, 0, 1+len(aliases))
	out = append(out, domain.NativeObjectRef{
		Kind:            "file",
		Path:            path,
		ProtectionClass: protection,
	})
	for _, alias := range aliases {
		out = append(out, domain.NativeObjectRef{
			Kind:            "cursor_mcp_server",
			Name:            alias,
			Path:            path,
			ProtectionClass: protection,
		})
	}
	return out
}

func ownedAliases(items []domain.NativeObjectRef) []string {
	var out []string
	for _, item := range items {
		if item.Kind == "cursor_mcp_server" && strings.TrimSpace(item.Name) != "" {
			out = append(out, item.Name)
		}
	}
	sort.Strings(out)
	return out
}

func configPathFromTarget(target domain.TargetInstallation, fallback string) string {
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "file" && strings.TrimSpace(item.Path) != "" {
			return item.Path
		}
	}
	if metadataPath, ok := target.AdapterMetadata["config_path"].(string); ok && strings.TrimSpace(metadataPath) != "" {
		return metadataPath
	}
	return fallback
}
