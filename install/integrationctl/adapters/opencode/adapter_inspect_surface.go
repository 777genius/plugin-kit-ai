package opencode

import (
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"os/exec"
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
