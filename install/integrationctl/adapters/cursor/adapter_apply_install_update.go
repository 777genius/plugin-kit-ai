package cursor

import (
	"context"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/portablemcp"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (a Adapter) applyInstall(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Cursor apply requires resolved source", nil)
	}
	docPath := a.targetConfigPath(in.Policy.Scope, workspaceRootFromApplyInput(in))
	return a.applyProjection(ctx, in, docPath, ownedAliasesFromRecord(in.Record))
}

func (a Adapter) applyUpdate(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	return a.applyInstall(ctx, in)
}

func (a Adapter) applyProjection(ctx context.Context, in ports.ApplyInput, docPath string, owned []string) (ports.ApplyResult, error) {
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
	for _, alias := range owned {
		if _, keep := projected[alias]; keep {
			continue
		}
		delete(doc, alias)
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

func ownedAliasesFromRecord(record *domain.InstallationRecord) []string {
	if record == nil {
		return nil
	}
	target, ok := record.Targets[domain.TargetCursor]
	if !ok {
		return nil
	}
	return ownedAliases(target.OwnedNativeObjects)
}
