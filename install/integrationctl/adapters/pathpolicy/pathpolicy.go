package pathpolicy

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func UserHome(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	home, _ := os.UserHomeDir()
	return home
}

func ProjectRoot(workspaceRoot, projectRoot string) string {
	if root := strings.TrimSpace(workspaceRoot); root != "" {
		return filepath.Clean(root)
	}
	if root := strings.TrimSpace(projectRoot); root != "" {
		return filepath.Clean(root)
	}
	cwd, _ := os.Getwd()
	return filepath.Clean(cwd)
}

func EffectiveGitRoot(workspaceRoot, projectRoot string) string {
	fallback := ProjectRoot(workspaceRoot, projectRoot)
	root := filepath.Clean(fallback)
	for {
		if root == "." || root == string(filepath.Separator) || strings.TrimSpace(root) == "" {
			return fallback
		}
		if FileExists(filepath.Join(root, ".git")) {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			return fallback
		}
		root = parent
	}
}

func PreferredExistingPath(candidates ...string) string {
	for _, path := range candidates {
		if FileExists(path) {
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

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func NormalizeScope(scope string) string {
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		return "project"
	}
	return "user"
}

func WorkspaceRootFromInspect(in ports.InspectInput) string {
	if in.Record != nil {
		return WorkspaceRootFromRecord(*in.Record)
	}
	return ""
}

func WorkspaceRootFromApply(in ports.ApplyInput) string {
	if in.Record != nil {
		return WorkspaceRootFromRecord(*in.Record)
	}
	return ""
}

func WorkspaceRootFromRecord(record domain.InstallationRecord) string {
	if NormalizeScope(record.Policy.Scope) == "project" {
		return strings.TrimSpace(record.WorkspaceRoot)
	}
	return ""
}

func ProtectionForScope(scope string) domain.ProtectionClass {
	if NormalizeScope(scope) == "project" {
		return domain.ProtectionWorkspace
	}
	return domain.ProtectionUserMutable
}
