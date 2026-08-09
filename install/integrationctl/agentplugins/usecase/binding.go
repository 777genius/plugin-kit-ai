package usecase

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

type BindingChangeMode string

const (
	BindingChangeRebind        BindingChangeMode = "rebind"
	BindingChangeMigrateFormat BindingChangeMode = "migrate_format"
)

type ProvenanceSummary struct {
	Kind             string `json:"kind"`
	Repository       string `json:"repository,omitempty"`
	PackageSubpath   string `json:"package_subpath,omitempty"`
	ResolvedRevision string `json:"resolved_revision,omitempty"`
	TreeDigest       string `json:"tree_digest,omitempty"`
}

type FormatSummary struct {
	LoaderKind string `json:"loader_kind"`
	FormatID   string `json:"format_id"`
	SchemaURI  string `json:"schema_uri"`
}

type TargetChange struct {
	ClientID        string                      `json:"client_id"`
	Scope           string                      `json:"scope"`
	Materialization domain.MaterializationState `json:"materialization"`
	Decision        string                      `json:"decision"`
}

type BindingChangePlan struct {
	Mode               BindingChangeMode         `json:"mode"`
	InstallationID     string                    `json:"installation_id"`
	OldName            string                    `json:"old_name"`
	NewName            string                    `json:"new_name"`
	OldSource          ProvenanceSummary         `json:"old_source"`
	NewSource          ProvenanceSummary         `json:"new_source"`
	OldFormat          FormatSummary             `json:"old_format"`
	NewFormat          FormatSummary             `json:"new_format"`
	OldComponents      domain.ComponentInventory `json:"old_components"`
	NewComponents      domain.ComponentInventory `json:"new_components"`
	Targets            []TargetChange            `json:"targets,omitempty"`
	NativeObjectCount  int                       `json:"native_object_count"`
	PluginDataDecision string                    `json:"plugin_data_decision"`
	CanApply           bool                      `json:"can_apply"`
	Blockers           []string                  `json:"blockers,omitempty"`
}

type BindingChangeInput struct {
	Selector  string
	Envelope  domain.PackageEnvelope
	Confirmed bool
}

type BindingChangeResult struct {
	Plan                 BindingChangePlan `json:"plan"`
	RequiresConfirmation bool              `json:"requires_confirmation"`
	Mutated              bool              `json:"mutated"`
	NoChange             bool              `json:"no_change,omitempty"`
}

func (service Service) Rebind(ctx context.Context, input BindingChangeInput) (BindingChangeResult, error) {
	return service.changeBinding(ctx, input, BindingChangeRebind)
}

func (service Service) MigrateFormat(ctx context.Context, input BindingChangeInput) (BindingChangeResult, error) {
	return service.changeBinding(ctx, input, BindingChangeMigrateFormat)
}

func (service Service) changeBinding(ctx context.Context, input BindingChangeInput, mode BindingChangeMode) (BindingChangeResult, error) {
	if err := ctx.Err(); err != nil {
		return BindingChangeResult{}, err
	}
	if service.StateStore == nil {
		return BindingChangeResult{}, fmt.Errorf("state store is required")
	}
	if input.Envelope.LoaderKind != domain.LoaderKindAgentPlugins || input.Envelope.FormatID != domain.FormatIDAgentPluginsV1 {
		return BindingChangeResult{}, fmt.Errorf("new binding must be a standard Agent Plugins 1.0 package")
	}
	release, err := service.beginMutation(ctx, false, input.Confirmed)
	if err != nil {
		return BindingChangeResult{}, err
	}
	if release != nil {
		defer func() { _ = release() }()
	}
	state, err := service.StateStore.Load()
	if err != nil {
		return BindingChangeResult{}, err
	}
	index, installation, err := findInstallation(state, input.Selector)
	if err != nil {
		return BindingChangeResult{}, err
	}
	if mode == BindingChangeRebind && installation.Package.LoaderKind != input.Envelope.LoaderKind {
		return BindingChangeResult{}, fmt.Errorf("rebind cannot change package format; use migrate-format")
	}
	if mode == BindingChangeMigrateFormat && installation.Package.LoaderKind == input.Envelope.LoaderKind && installation.Package.FormatID == input.Envelope.FormatID {
		return BindingChangeResult{}, fmt.Errorf("installation already uses Agent Plugins 1.0; use rebind")
	}
	newSourceID := domain.ComputeSourceBindingID(input.Envelope.Source)
	plan := buildBindingChangePlan(mode, installation, input.Envelope)
	result := BindingChangeResult{Plan: plan}
	if mode == BindingChangeRebind && newSourceID == installation.Source.SourceBindingID &&
		installation.Source.TreeDigest == input.Envelope.TreeDigest && installation.Package.ManifestDigest == input.Envelope.ManifestDigest {
		result.NoChange = true
		return result, nil
	}
	for otherIndex, other := range state.Installations {
		if otherIndex != index && other.Source.SourceBindingID == newSourceID {
			return result, fmt.Errorf("new source is already bound to installation %s", other.InstallationID)
		}
	}
	if !plan.CanApply {
		if input.Confirmed {
			return result, fmt.Errorf("binding change is blocked: %s", strings.Join(plan.Blockers, "; "))
		}
		return result, nil
	}
	if !input.Confirmed {
		result.RequiresConfirmation = true
		return result, nil
	}
	timestamp := service.now().Format(time.RFC3339Nano)
	installation.DeclaredName = input.Envelope.Manifest.Name
	installation.Source = domain.SourceBinding{
		SourceBindingID: newSourceID, RequestedSource: input.Envelope.Source.RequestedSource,
		CanonicalSource: input.Envelope.Source.CanonicalSource, Repository: input.Envelope.Source.Repository,
		PackageSubpath: input.Envelope.Source.PackageSubpath, ResolvedRevision: input.Envelope.Source.ResolvedRevision,
		TreeDigest: input.Envelope.TreeDigest,
	}
	installation.Package = domain.PackageBinding{
		LoaderKind: input.Envelope.LoaderKind, FormatID: input.Envelope.FormatID,
		SchemaURI: input.Envelope.SchemaURI, DeclaredName: input.Envelope.Manifest.Name,
		Version: input.Envelope.Manifest.Version, ManifestDigest: input.Envelope.ManifestDigest,
		Inventory: input.Envelope.Inventory,
	}
	installation.NeedsRebind = false
	installation.UpdatedAt = timestamp
	state.Installations[index] = installation
	if err := service.StateStore.Save(state); err != nil {
		return result, fmt.Errorf("commit binding change: %w", err)
	}
	result.Mutated = true
	return result, nil
}

func buildBindingChangePlan(mode BindingChangeMode, installation domain.Installation, envelope domain.PackageEnvelope) BindingChangePlan {
	plan := BindingChangePlan{
		Mode: mode, InstallationID: installation.InstallationID,
		OldName: installation.DeclaredName, NewName: envelope.Manifest.Name,
		OldSource: provenanceFromBinding(installation.Source), NewSource: provenanceFromEnvelope(envelope),
		OldFormat:     FormatSummary{LoaderKind: installation.Package.LoaderKind, FormatID: installation.Package.FormatID, SchemaURI: installation.Package.SchemaURI},
		NewFormat:     FormatSummary{LoaderKind: envelope.LoaderKind, FormatID: envelope.FormatID, SchemaURI: envelope.SchemaURI},
		OldComponents: installation.Package.Inventory, NewComponents: envelope.Inventory,
		PluginDataDecision: "not_transferred", CanApply: true,
	}
	for _, client := range installation.Clients {
		decision := "reinstall_after_binding_change"
		if client.Materialization != domain.MaterializationAbsent {
			plan.CanApply = false
			plan.Blockers = append(plan.Blockers, fmt.Sprintf("remove %s/%s target first", client.ClientID, client.Scope))
			decision = "remove_first"
		}
		plan.Targets = append(plan.Targets, TargetChange{
			ClientID: client.ClientID, Scope: client.Scope, Materialization: client.Materialization, Decision: decision,
		})
		plan.NativeObjectCount += len(client.NativeObjects)
		if len(client.NativeObjects) > 0 {
			plan.CanApply = false
			plan.Blockers = append(plan.Blockers, fmt.Sprintf("%s/%s still owns native objects", client.ClientID, client.Scope))
		}
	}
	sort.Slice(plan.Targets, func(i, j int) bool {
		if plan.Targets[i].ClientID == plan.Targets[j].ClientID {
			return plan.Targets[i].Scope < plan.Targets[j].Scope
		}
		return plan.Targets[i].ClientID < plan.Targets[j].ClientID
	})
	sort.Strings(plan.Blockers)
	return plan
}

func provenanceFromBinding(source domain.SourceBinding) ProvenanceSummary {
	kind := "remote"
	if source.Repository != "" {
		kind = "github"
	} else if filepath.IsAbs(source.CanonicalSource) {
		kind = "local"
	}
	return ProvenanceSummary{
		Kind: kind, Repository: source.Repository, PackageSubpath: source.PackageSubpath,
		ResolvedRevision: source.ResolvedRevision, TreeDigest: source.TreeDigest,
	}
}

func provenanceFromEnvelope(envelope domain.PackageEnvelope) ProvenanceSummary {
	kind := "remote"
	if envelope.Source.Repository != "" {
		kind = "github"
	} else if filepath.IsAbs(envelope.Source.CanonicalSource) {
		kind = "local"
	}
	return ProvenanceSummary{
		Kind: kind, Repository: envelope.Source.Repository, PackageSubpath: envelope.Source.PackageSubpath,
		ResolvedRevision: envelope.Source.ResolvedRevision, TreeDigest: envelope.TreeDigest,
	}
}
