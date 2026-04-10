package opencode

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (a Adapter) Inspect(ctx context.Context, in ports.InspectInput) (ports.InspectResult, error) {
	surface := a.inspectSurface(in.Scope, workspaceRootFromInspectInput(in))
	config := surface.ConfigPath
	observed := []domain.NativeObjectRef{}
	if in.Record != nil {
		if target, ok := in.Record.Targets[domain.TargetOpenCode]; ok {
			config = configPathFromTarget(target, config)
		}
	}
	_, cmdErr := exec.LookPath("opencode")
	_, statErr := os.Stat(config)
	restrictions := append([]domain.EnvironmentRestrictionCode(nil), surface.EnvironmentRestrictions...)
	state := domain.InstallRemoved
	if cmdErr != nil && statErr != nil {
		restrictions = append(restrictions, domain.RestrictionSourceToolMissing)
	}
	if in.Record != nil {
		if target, ok := in.Record.Targets[domain.TargetOpenCode]; ok && statErr == nil {
			body, err := a.fs().ReadFile(ctx, config)
			if err != nil {
				return ports.InspectResult{}, domain.NewError(domain.ErrMutationApply, "read OpenCode config during inspect", err)
			}
			doc, err := decodeConfigMap(body)
			if err != nil {
				return ports.InspectResult{}, domain.NewError(domain.ErrMutationApply, "parse OpenCode config during inspect", err)
			}
			present := false
			plugins, err := existingPluginRefs(doc["plugin"])
			if err != nil {
				return ports.InspectResult{}, domain.NewError(domain.ErrMutationApply, "parse OpenCode plugin refs during inspect", err)
			}
			for _, ref := range ownedPluginRefs(target) {
				if _, ok := plugins[ref]; ok {
					present = true
					observed = append(observed, domain.NativeObjectRef{Kind: "opencode_plugin_ref", Name: ref, Path: config})
				}
			}
			mcp, err := existingObjectMap(doc["mcp"], "mcp")
			if err != nil {
				return ports.InspectResult{}, domain.NewError(domain.ErrMutationApply, "parse OpenCode MCP config during inspect", err)
			}
			for _, alias := range ownedMCPAliases(target) {
				if _, ok := mcp[alias]; ok {
					present = true
					observed = append(observed, domain.NativeObjectRef{Kind: "opencode_mcp_server", Name: alias, Path: config})
				}
			}
			for _, key := range ownedConfigKeys(target) {
				if _, ok := doc[key]; ok {
					present = true
					observed = append(observed, domain.NativeObjectRef{Kind: "opencode_config_key", Name: key, Path: config})
				}
			}
			if present {
				state = domain.InstallInstalled
			} else {
				state = domain.InstallRemoved
			}
			return ports.InspectResult{
				TargetID:                 a.ID(),
				Installed:                present,
				State:                    state,
				ActivationState:          domain.ActivationRestartPending,
				ConfigPrecedenceContext:  surface.ConfigPrecedenceContext,
				EnvironmentRestrictions:  restrictions,
				VolatileOverrideDetected: surface.VolatileOverride,
				ObservedNativeObjects:    observed,
				SourceAccessState:        surface.SourceAccessState,
				SettingsFiles:            append([]string(nil), surface.SettingsFiles...),
				EvidenceClass:            domain.EvidenceConfirmed,
			}, nil
		}
	}
	if statErr == nil || cmdErr == nil {
		state = domain.InstallInstalled
	}
	return ports.InspectResult{
		TargetID:                 a.ID(),
		Installed:                statErr == nil,
		State:                    state,
		ActivationState:          domain.ActivationRestartPending,
		ConfigPrecedenceContext:  surface.ConfigPrecedenceContext,
		EnvironmentRestrictions:  restrictions,
		VolatileOverrideDetected: surface.VolatileOverride,
		ObservedNativeObjects:    observed,
		SourceAccessState:        surface.SourceAccessState,
		SettingsFiles:            append([]string(nil), surface.SettingsFiles...),
		EvidenceClass:            domain.EvidenceConfirmed,
	}, nil
}

func (a Adapter) inspectSurface(scope string, workspaceRoot string) inspectSurface {
	settings := []string{}
	restrictions := []domain.EnvironmentRestrictionCode{}
	volatile := false
	sourceAccess := ""

	var configPath string
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		root := a.effectiveProjectRoot(workspaceRoot)
		configPath = preferredConfigPath(
			filepath.Join(root, "opencode.json"),
			filepath.Join(root, "opencode.jsonc"),
		)
		settings = append(settings, preferredExistingPaths(
			filepath.Join(root, "opencode.json"),
			filepath.Join(root, "opencode.jsonc"),
		)...)
	} else {
		configPath = preferredConfigPath(
			filepath.Join(a.userHome(), ".config", "opencode", "opencode.json"),
			filepath.Join(a.userHome(), ".config", "opencode", "opencode.jsonc"),
			filepath.Join(a.userHome(), ".local", "share", "opencode", "opencode.jsonc"),
		)
		settings = append(settings, preferredExistingPaths(
			filepath.Join(a.userHome(), ".config", "opencode", "opencode.json"),
			filepath.Join(a.userHome(), ".config", "opencode", "opencode.jsonc"),
			filepath.Join(a.userHome(), ".local", "share", "opencode", "opencode.jsonc"),
		)...)
	}

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
