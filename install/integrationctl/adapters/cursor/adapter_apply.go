package cursor

import (
	"context"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/portablemcp"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (a Adapter) PlanInstall(_ context.Context, in ports.PlanInstallInput) (ports.AdapterPlan, error) {
	configPath := a.targetConfigPath("user", "")
	if strings.EqualFold(strings.TrimSpace(in.Policy.Scope), "project") {
		configPath = a.targetConfigPath("project", "")
	}
	return ports.AdapterPlan{
		TargetID:     a.ID(),
		ActionClass:  "install_missing",
		Summary:      "Project or global MCP reconciliation for Cursor",
		PathsTouched: []string{configPath},
		EvidenceKey:  "target.cursor.native_surface",
	}, nil
}

func (a Adapter) ApplyInstall(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Cursor apply requires resolved source", nil)
	}
	docPath := a.targetConfigPath(in.Policy.Scope, workspaceRootFromApplyInput(in))
	loader := portablemcp.Loader{FS: a.fs()}
	loaded, err := loader.LoadForTarget(ctx, in.ResolvedSource.LocalPath, domain.TargetCursor)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	projected, aliases, err := renderCursorServers(loaded, in.ResolvedSource.LocalPath)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	doc, wrapped, originalBody, err := a.readDocument(ctx, docPath)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	if in.Record != nil {
		if target, ok := in.Record.Targets[domain.TargetCursor]; ok {
			for _, alias := range ownedAliases(target.OwnedNativeObjects) {
				if _, keep := projected[alias]; keep {
					continue
				}
				delete(doc, alias)
			}
		}
	}
	merged := mergeServers(doc, projected)
	body, err := marshalCursorDocument(merged, wrapped)
	if err != nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "marshal Cursor MCP config", err)
	}
	if _, err := a.mutator().MutateFile(ctx, ports.SafeFileMutationInput{
		Path: docPath,
		Mode: 0o644,
		Build: func(_ []byte, _ bool) ([]byte, error) {
			return body, nil
		},
		ValidateBefore: func(next []byte) error {
			_, _, err := a.readDocumentBytes(next)
			return err
		},
		ValidateAfter: func(ctx context.Context, path string, _ []byte) error {
			return a.verifyAliases(ctx, path, aliases)
		},
	}); err != nil {
		if len(originalBody) > 0 {
			_ = a.fs().WriteFileAtomic(ctx, docPath, originalBody, 0o644)
		}
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallInstalled,
		ActivationState:    domain.ActivationNotRequired,
		OwnedNativeObjects: ownedObjectsForConfig(docPath, aliases, protectionForScope(in.Policy.Scope)),
		EvidenceClass:      domain.EvidenceConfirmed,
		AdapterMetadata: map[string]any{
			"config_path":   docPath,
			"owned_aliases": aliases,
			"portable_path": loaded.Path,
			"wrapped_style": wrapped,
		},
	}, nil
}

func (a Adapter) PlanUpdate(_ context.Context, in ports.PlanUpdateInput) (ports.AdapterPlan, error) {
	configPath := a.targetConfigPath("user", "")
	if target, ok := in.CurrentRecord.Targets[domain.TargetCursor]; ok {
		configPath = configPathFromTarget(target, a.targetConfigPath(in.CurrentRecord.Policy.Scope, workspaceRootFromRecord(in.CurrentRecord)))
	}
	return ports.AdapterPlan{
		TargetID:     a.ID(),
		ActionClass:  "update_version",
		Summary:      "Owned-entry reconciliation for Cursor MCP",
		PathsTouched: []string{configPath},
		EvidenceKey:  "target.cursor.native_surface",
	}, nil
}

func (a Adapter) ApplyUpdate(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	return a.ApplyInstall(ctx, in)
}

func (a Adapter) PlanRemove(_ context.Context, in ports.PlanRemoveInput) (ports.AdapterPlan, error) {
	configPath := a.targetConfigPath("user", "")
	if target, ok := in.Record.Targets[domain.TargetCursor]; ok {
		configPath = configPathFromTarget(target, a.targetConfigPath(in.Record.Policy.Scope, workspaceRootFromRecord(in.Record)))
	}
	return ports.AdapterPlan{
		TargetID:     a.ID(),
		ActionClass:  "remove_orphaned_target",
		Summary:      "Remove owned Cursor MCP entries",
		PathsTouched: []string{configPath},
		EvidenceKey:  "target.cursor.native_surface",
	}, nil
}

func (a Adapter) ApplyRemove(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.Record == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Cursor remove requires current record", nil)
	}
	target, ok := in.Record.Targets[domain.TargetCursor]
	if !ok {
		return ports.ApplyResult{}, domain.NewError(domain.ErrStateConflict, "Cursor target is missing from installation record", nil)
	}
	docPath := configPathFromTarget(target, a.targetConfigPath(in.Record.Policy.Scope, workspaceRootFromRecord(*in.Record)))
	doc, wrapped, originalBody, err := a.readDocument(ctx, docPath)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	aliases := ownedAliases(target.OwnedNativeObjects)
	if len(aliases) == 0 {
		return ports.ApplyResult{
			TargetID:        a.ID(),
			State:           domain.InstallRemoved,
			ActivationState: domain.ActivationNotRequired,
			EvidenceClass:   domain.EvidenceConfirmed,
		}, nil
	}
	for _, alias := range aliases {
		delete(doc, alias)
	}
	body, err := marshalCursorDocument(doc, wrapped)
	if err != nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "marshal Cursor MCP config", err)
	}
	if _, err := a.mutator().MutateFile(ctx, ports.SafeFileMutationInput{
		Path: docPath,
		Mode: 0o644,
		Build: func(_ []byte, _ bool) ([]byte, error) {
			return body, nil
		},
		ValidateBefore: func(next []byte) error {
			_, _, err := a.readDocumentBytes(next)
			return err
		},
		ValidateAfter: func(ctx context.Context, path string, _ []byte) error {
			return a.verifyMissingAliases(ctx, path, aliases)
		},
	}); err != nil {
		if len(originalBody) > 0 {
			_ = a.fs().WriteFileAtomic(ctx, docPath, originalBody, 0o644)
		}
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:        a.ID(),
		State:           domain.InstallRemoved,
		ActivationState: domain.ActivationNotRequired,
		EvidenceClass:   domain.EvidenceConfirmed,
		AdapterMetadata: map[string]any{
			"config_path":     docPath,
			"removed_aliases": aliases,
			"wrapped_style":   wrapped,
		},
	}, nil
}

func (a Adapter) Repair(ctx context.Context, in ports.RepairInput) (ports.ApplyResult, error) {
	if in.Manifest == nil || in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrRepairApply, "Cursor repair requires resolved source and manifest", nil)
	}
	result, err := a.ApplyInstall(ctx, ports.ApplyInput{
		Manifest:       *in.Manifest,
		ResolvedSource: in.ResolvedSource,
		Policy:         in.Record.Policy,
		Inspect:        in.Inspect,
		Record:         &in.Record,
	})
	if err != nil {
		return ports.ApplyResult{}, err
	}
	result.State = domain.InstallInstalled
	if len(result.ManualSteps) == 0 {
		result.ManualSteps = append(result.ManualSteps, "repair reconciled managed Cursor MCP entries into the effective config layer")
	}
	return result, nil
}
