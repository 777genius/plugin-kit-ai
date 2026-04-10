package cursor

import (
	"context"
	"os"
	"os/exec"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (a Adapter) Inspect(ctx context.Context, in ports.InspectInput) (ports.InspectResult, error) {
	config := a.targetConfigPath(in.Scope, workspaceRootFromInspectInput(in))
	observed := []domain.NativeObjectRef{}
	if in.Record != nil {
		if target, ok := in.Record.Targets[domain.TargetCursor]; ok {
			config = configPathFromTarget(target, config)
		}
	}
	_, cmdErr := exec.LookPath("cursor-agent")
	_, statErr := os.Stat(config)
	restrictions := []domain.EnvironmentRestrictionCode{}
	state := domain.InstallRemoved
	if cmdErr != nil && statErr != nil {
		restrictions = append(restrictions, domain.RestrictionSourceToolMissing)
	}
	if in.Record != nil {
		if target, ok := in.Record.Targets[domain.TargetCursor]; ok {
			aliases := ownedAliases(target.OwnedNativeObjects)
			if len(aliases) > 0 && statErr == nil {
				doc, _, _, err := a.readDocument(ctx, config)
				if err != nil {
					return ports.InspectResult{}, err
				}
				present := false
				for _, alias := range aliases {
					if _, ok := doc[alias]; ok {
						present = true
						observed = append(observed, domain.NativeObjectRef{Kind: "cursor_mcp_server", Name: alias, Path: config})
					}
				}
				if present {
					state = domain.InstallInstalled
				} else {
					state = domain.InstallRemoved
				}
				return ports.InspectResult{
					TargetID:                a.ID(),
					Installed:               present,
					State:                   state,
					ActivationState:         domain.ActivationNotRequired,
					ConfigPrecedenceContext: []string{"project", "global", "parent_discovery"},
					EnvironmentRestrictions: restrictions,
					ObservedNativeObjects:   observed,
					SettingsFiles:           []string{config},
					EvidenceClass:           domain.EvidenceConfirmed,
				}, nil
			}
		}
	}
	if statErr == nil || cmdErr == nil {
		state = domain.InstallInstalled
	}
	return ports.InspectResult{
		TargetID:                a.ID(),
		Installed:               statErr == nil,
		State:                   state,
		ActivationState:         domain.ActivationNotRequired,
		ConfigPrecedenceContext: []string{"project", "global", "parent_discovery"},
		EnvironmentRestrictions: restrictions,
		ObservedNativeObjects:   observed,
		SettingsFiles:           []string{config},
		EvidenceClass:           domain.EvidenceConfirmed,
	}, nil
}
