package gemini

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (a Adapter) Inspect(ctx context.Context, in ports.InspectInput) (ports.InspectResult, error) {
	home := a.userHome()
	workspaceRoot := workspaceRootFromInspectInput(in)
	settings := a.settingsPath(scopeFromInspectInput(in), workspaceRoot)
	workspaceSettings := workspaceSettingsPath(workspaceRoot)
	enablement := a.enablementPath()
	trusted := filepath.Join(home, ".gemini", "trustedFolders.json")
	extensionDir := ""
	if in.Record != nil {
		extensionDir = filepath.Join(home, ".gemini", "extensions", in.Record.IntegrationID)
	}
	_, cmdErr := exec.LookPath("gemini")
	_, extErr := os.Stat(extensionDir)
	restrictions := []domain.EnvironmentRestrictionCode{}
	state := domain.InstallRemoved
	if cmdErr != nil {
		restrictions = append(restrictions, domain.RestrictionSourceToolMissing)
	}
	disabled, err := a.isDisabled(ctx, in)
	if err != nil {
		return ports.InspectResult{}, err
	}
	if extErr == nil {
		if disabled {
			state = domain.InstallDisabled
		} else {
			state = domain.InstallInstalled
		}
	}
	settingsFiles := []string{settings, enablement, trusted}
	if workspaceSettings != "" {
		settingsFiles = append(settingsFiles, workspaceSettings)
	}
	settingsFiles = append(settingsFiles, a.systemSettingsPaths()...)
	return ports.InspectResult{
		TargetID:                a.ID(),
		Installed:               extErr == nil,
		State:                   state,
		ActivationState:         domain.ActivationRestartPending,
		ConfigPrecedenceContext: []string{"cli", "env", "system_settings", "system_defaults", "workspace", "user"},
		EnvironmentRestrictions: restrictions,
		TrustResolutionSource:   trusted,
		SettingsFiles:           dedupeStrings(settingsFiles),
		EvidenceClass:           domain.EvidenceConfirmed,
	}, nil
}

func (a Adapter) isDisabled(ctx context.Context, in ports.InspectInput) (bool, error) {
	if in.Record == nil {
		return false, nil
	}
	scope := scopeFromInspectInput(in)
	workspaceRoot := workspaceRootFromInspectInput(in)
	if disabled, handled, err := a.disabledByEnablement(ctx, scope, workspaceRoot, strings.TrimSpace(in.Record.IntegrationID)); err != nil {
		return false, err
	} else if handled {
		return disabled, nil
	}
	body, err := a.fs().ReadFile(ctx, a.settingsPath(scope, workspaceRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, domain.NewError(domain.ErrMutationApply, "read Gemini settings during inspect", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return false, domain.NewError(domain.ErrMutationApply, "parse Gemini settings during inspect", err)
	}
	extensions, ok := doc["extensions"].(map[string]any)
	if !ok || extensions == nil {
		return false, nil
	}
	raw, ok := extensions["disabled"].([]any)
	if !ok {
		return false, nil
	}
	name := strings.TrimSpace(in.Record.IntegrationID)
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) == name {
			return true, nil
		}
	}
	return false, nil
}

func (a Adapter) disabledByEnablement(ctx context.Context, scope string, workspaceRoot string, name string) (bool, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, false, nil
	}
	body, err := a.fs().ReadFile(ctx, a.enablementPath())
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, nil
		}
		return false, false, domain.NewError(domain.ErrMutationApply, "read Gemini extension enablement during inspect", err)
	}
	var doc map[string]struct {
		Overrides []string `json:"overrides"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return false, false, domain.NewError(domain.ErrMutationApply, "parse Gemini extension enablement during inspect", err)
	}
	entry, ok := doc[name]
	if !ok {
		return false, false, nil
	}
	if scope == "project" {
		root := workspaceRootForScope(scope, workspaceRoot)
		if root == "" {
			return false, false, nil
		}
		expected := filepath.Clean(root) + string(os.PathSeparator) + "*"
		for _, override := range entry.Overrides {
			override = strings.TrimSpace(override)
			if override == "" {
				continue
			}
			if strings.TrimPrefix(override, "!") != expected {
				continue
			}
			return strings.HasPrefix(override, "!"), true, nil
		}
		return false, false, nil
	}
	for _, override := range entry.Overrides {
		override = strings.TrimSpace(override)
		if override == "!*" {
			return true, true, nil
		}
		if override == "*" {
			return false, true, nil
		}
	}
	return false, false, nil
}
