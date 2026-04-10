package opencode

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

var execLookPath = exec.LookPath

func (a Adapter) inspectSurface(scope string, workspaceRoot string) inspectSurface {
	settings := []string{}
	restrictions := []domain.EnvironmentRestrictionCode{}
	volatile := false
	sourceAccess := ""

	configPath, candidates := a.inspectSurfacePaths(scope, workspaceRoot)
	settings = append(settings, preferredExistingPaths(candidates...)...)
	settings = dedupeStrings(settings)

	for _, path := range a.managedConfigPaths() {
		if fileExists(path) {
			restrictions = append(restrictions, domain.RestrictionReadOnlyNativeLayer)
			settings = append(settings, path)
			if sourceAccess == "" {
				sourceAccess = "managed_config_layer"
			}
		}
	}

	return inspectSurface{
		ConfigPath:              configPath,
		SettingsFiles:           settings,
		ConfigPrecedenceContext: []string{"remote", "global", "project", ".opencode", "managed"},
		EnvironmentRestrictions: dedupeRestrictionCodes(restrictions),
		VolatileOverride:        volatile,
		SourceAccessState:       sourceAccess,
	}
}

func (a Adapter) inspectSurfacePaths(scope string, workspaceRoot string) (string, []string) {
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		root := a.effectiveProjectRoot(workspaceRoot)
		candidates := []string{
			filepath.Join(root, "opencode.json"),
			filepath.Join(root, "opencode.jsonc"),
		}
		return preferredConfigPath(candidates...), candidates
	}
	candidates := []string{
		filepath.Join(a.userHome(), ".config", "opencode", "opencode.json"),
		filepath.Join(a.userHome(), ".config", "opencode", "opencode.jsonc"),
		filepath.Join(a.userHome(), ".local", "share", "opencode", "opencode.jsonc"),
	}
	return preferredConfigPath(candidates...), candidates
}

func planBlockingManualSteps(inspect ports.InspectResult) ([]string, bool) {
	steps := []string{}
	blocking := false
	for _, restriction := range inspect.EnvironmentRestrictions {
		if restriction == domain.RestrictionReadOnlyNativeLayer {
			steps = append(steps,
				"OpenCode managed config is active at a higher-precedence system layer",
				"ask an administrator to update or remove the managed OpenCode config before mutating this integration",
			)
			blocking = true
			break
		}
	}
	return dedupeStrings(steps), blocking
}

func (a Adapter) managedConfigPaths() []string {
	switch runtime.GOOS {
	case "darwin":
		userName := strings.TrimSpace(filepath.Base(a.userHome()))
		return dedupeStrings([]string{
			"/Library/Application Support/opencode/opencode.json",
			"/Library/Application Support/opencode/opencode.jsonc",
			filepath.Join("/Library/Managed Preferences", userName, "ai.opencode.managed.plist"),
			"/Library/Managed Preferences/ai.opencode.managed.plist",
		})
	case "linux":
		return []string{
			"/etc/opencode/opencode.json",
			"/etc/opencode/opencode.jsonc",
		}
	case "windows":
		base := strings.TrimSpace(os.Getenv("ProgramData"))
		if base == "" {
			base = strings.TrimSpace(os.Getenv("ALLUSERSPROFILE"))
		}
		if base == "" {
			return nil
		}
		return []string{
			filepath.Join(base, "opencode", "opencode.json"),
			filepath.Join(base, "opencode", "opencode.jsonc"),
		}
	default:
		return nil
	}
}

func preferredExistingPaths(candidates ...string) []string {
	var out []string
	for _, path := range candidates {
		if fileExists(path) {
			out = append(out, path)
		}
	}
	return out
}

func dedupeRestrictionCodes(values []domain.EnvironmentRestrictionCode) []domain.EnvironmentRestrictionCode {
	if len(values) == 0 {
		return nil
	}
	seen := map[domain.EnvironmentRestrictionCode]struct{}{}
	out := make([]domain.EnvironmentRestrictionCode, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
