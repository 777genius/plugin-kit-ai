package claude

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type listedPlugin struct {
	ID      string `json:"id"`
	Scope   string `json:"scope"`
	Enabled bool   `json:"enabled"`
}

func (a Adapter) Inspect(ctx context.Context, in ports.InspectInput) (ports.InspectResult, error) {
	scope := scopeForInspect(in)
	workspaceRoot := workspaceRootFromInspectInput(in)
	settings := a.settingsPath(scope, workspaceRoot)
	_, cmdErr := exec.LookPath("claude")
	_, statErr := os.Stat(settings)
	restrictions := []domain.EnvironmentRestrictionCode{}
	settingsFiles := []string{settings}
	state := domain.InstallRemoved
	integrationID := strings.TrimSpace(in.IntegrationID)
	if integrationID == "" && in.Record != nil {
		integrationID = strings.TrimSpace(in.Record.IntegrationID)
	}
	if cmdErr != nil && statErr != nil {
		restrictions = append(restrictions, domain.RestrictionSourceToolMissing)
	}
	if managedPath, managed, ok := a.readManagedSettings(scope, workspaceRoot); ok {
		settingsFiles = append(settingsFiles, managedPath)
		if managed.blocksAllMarketplaceAdds() {
			restrictions = append(restrictions, domain.RestrictionManagedPolicyBlock)
		} else if integrationID != "" {
			if blocked, _ := a.marketplaceAddBlocked(scope, workspaceRoot, integrationID); blocked {
				restrictions = append(restrictions, domain.RestrictionManagedPolicyBlock)
			}
		}
	}
	if integrationID != "" {
		if seedPath, ok := a.seedManagedMarketplacePath(integrationID, in.Record); ok {
			settingsFiles = append(settingsFiles, seedPath)
			restrictions = append(restrictions, domain.RestrictionReadOnlyNativeLayer)
		}
	}
	if hasRestriction(restrictions, domain.RestrictionManagedPolicyBlock) {
		restrictions = dedupeRestrictions(restrictions)
	}
	if hasRestriction(restrictions, domain.RestrictionReadOnlyNativeLayer) {
		restrictions = dedupeRestrictions(restrictions)
	}
	if cmdErr == nil && integrationID != "" {
		if nativeState, ok, err := a.inspectPluginList(ctx, scope, workspaceRoot, integrationID, in.Record); err != nil {
			return ports.InspectResult{}, err
		} else if ok {
			state = nativeState
		}
	} else if statErr == nil || cmdErr == nil {
		state = domain.InstallInstalled
	}
	return ports.InspectResult{
		TargetID:                a.ID(),
		Installed:               statErr == nil,
		State:                   state,
		ActivationState:         domain.ActivationReloadPending,
		ConfigPrecedenceContext: []string{"project", "user", "managed"},
		EnvironmentRestrictions: restrictions,
		SettingsFiles:           settingsFiles,
		EvidenceClass:           domain.EvidenceConfirmed,
	}, nil
}

func (a Adapter) inspectPluginList(ctx context.Context, scope string, workspaceRoot string, integrationID string, record *domain.InstallationRecord) (domain.InstallState, bool, error) {
	commandDir := a.commandDirForScope(scope, workspaceRoot)
	result, err := a.runner().Run(ctx, ports.Command{
		Argv: []string{"claude", "plugin", "list", "--json"},
		Dir:  commandDir,
	})
	if err != nil {
		return "", false, domain.NewError(domain.ErrMutationApply, "run Claude plugin list", err)
	}
	if result.ExitCode != 0 {
		msg := strings.TrimSpace(string(result.Stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(result.Stdout))
		}
		if msg == "" {
			msg = "Claude plugin list failed"
		}
		return "", false, domain.NewError(domain.ErrMutationApply, msg, nil)
	}
	var items []listedPlugin
	if err := json.Unmarshal(result.Stdout, &items); err != nil {
		return "", false, domain.NewError(domain.ErrMutationApply, "parse Claude plugin list JSON", err)
	}
	wantRef := integrationID + "@" + managedMarketplaceName(integrationID)
	if record != nil {
		if value := pluginRefFromRecord(*record); value != "" {
			wantRef = value
		}
	}
	wantScope := strings.ToLower(strings.TrimSpace(scope))
	for _, item := range items {
		if strings.TrimSpace(item.ID) != wantRef {
			continue
		}
		if wantScope != "" && strings.ToLower(strings.TrimSpace(item.Scope)) != wantScope {
			continue
		}
		if item.Enabled {
			return domain.InstallInstalled, true, nil
		}
		return domain.InstallDisabled, true, nil
	}
	return domain.InstallRemoved, true, nil
}

func scopeForInspect(in ports.InspectInput) string {
	if in.Record != nil {
		return in.Record.Policy.Scope
	}
	return in.Scope
}
