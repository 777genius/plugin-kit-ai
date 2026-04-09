package cursor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/portablemcp"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/safemutate"
	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type Adapter struct {
	FS          ports.FileSystem
	SafeMutator ports.SafeFileMutator
	ProjectRoot string
	UserHome    string
}

func (Adapter) ID() domain.TargetID { return domain.TargetCursor }

func (Adapter) Capabilities(context.Context) (ports.Capabilities, error) {
	return ports.Capabilities{
		InstallMode:               "config_projection",
		SupportsNativeUpdate:      false,
		SupportsNativeRemove:      true,
		SupportsRepair:            true,
		SupportsScopeUser:         true,
		SupportsScopeProject:      true,
		MayTriggerInteractiveAuth: true,
		SupportedSourceKinds:      []string{"local_path", "github_repo_path", "git_url"},
		EvidenceKey:               "target.cursor.native_surface",
	}, nil
}

func (a Adapter) Inspect(ctx context.Context, in ports.InspectInput) (ports.InspectResult, error) {
	config := a.targetConfigPath(in.Scope)
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

func (a Adapter) PlanInstall(_ context.Context, in ports.PlanInstallInput) (ports.AdapterPlan, error) {
	configPath := a.targetConfigPath("user")
	if strings.EqualFold(strings.TrimSpace(in.Policy.Scope), "project") {
		configPath = a.targetConfigPath("project")
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
	docPath := a.targetConfigPath(in.Policy.Scope)
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
	configPath := a.targetConfigPath("user")
	if target, ok := in.CurrentRecord.Targets[domain.TargetCursor]; ok {
		configPath = configPathFromTarget(target, configPath)
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
	configPath := a.targetConfigPath("user")
	if target, ok := in.Record.Targets[domain.TargetCursor]; ok {
		configPath = configPathFromTarget(target, configPath)
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
	docPath := configPathFromTarget(target, a.targetConfigPath(in.Record.Policy.Scope))
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
			"config_path":   docPath,
			"removed_aliases": aliases,
			"wrapped_style": wrapped,
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

func (a Adapter) fs() ports.FileSystem {
	if a.FS != nil {
		return a.FS
	}
	return fsadapter.OS{}
}

func (a Adapter) mutator() ports.SafeFileMutator {
	if a.SafeMutator != nil {
		return a.SafeMutator
	}
	return safemutate.OS{}
}

func (a Adapter) targetConfigPath(scope string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "project" {
		return filepath.Join(a.projectRoot(), ".cursor", "mcp.json")
	}
	return filepath.Join(a.userHome(), ".cursor", "mcp.json")
}

func (a Adapter) projectRoot() string {
	if strings.TrimSpace(a.ProjectRoot) != "" {
		return a.ProjectRoot
	}
	cwd, _ := os.Getwd()
	return cwd
}

func (a Adapter) userHome() string {
	if strings.TrimSpace(a.UserHome) != "" {
		return a.UserHome
	}
	home, _ := os.UserHomeDir()
	return home
}

func renderCursorServers(loaded portablemcp.Loaded, packageRoot string) (map[string]any, []string, error) {
	projected := make(map[string]any, len(loaded.Servers))
	aliases := make([]string, 0, len(loaded.Servers))
	for alias, server := range loaded.Servers {
		switch server.Type {
		case "stdio":
			item := map[string]any{
				"command": interpolatePackageRoot(server.Stdio.Command, packageRoot),
			}
			if len(server.Stdio.Args) > 0 {
				args := make([]any, 0, len(server.Stdio.Args))
				for _, arg := range server.Stdio.Args {
					args = append(args, interpolatePackageRoot(arg, packageRoot))
				}
				item["args"] = args
			}
			if len(server.Stdio.Env) > 0 {
				env := make(map[string]any, len(server.Stdio.Env))
				for key, value := range server.Stdio.Env {
					env[key] = interpolatePackageRoot(value, packageRoot)
				}
				item["env"] = env
			}
			projected[alias] = item
		case "remote":
			item := map[string]any{
				"url": interpolatePackageRoot(server.Remote.URL, packageRoot),
			}
			if len(server.Remote.Headers) > 0 {
				headers := make(map[string]any, len(server.Remote.Headers))
				for key, value := range server.Remote.Headers {
					headers[key] = interpolatePackageRoot(value, packageRoot)
				}
				item["headers"] = headers
			}
			projected[alias] = item
		default:
			return nil, nil, domain.NewError(domain.ErrUnsupportedTarget, "unsupported Cursor portable MCP server type "+server.Type, nil)
		}
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return projected, aliases, nil
}

func interpolatePackageRoot(value, packageRoot string) string {
	return strings.ReplaceAll(value, "${package.root}", packageRoot)
}

func mergeServers(existing, owned map[string]any) map[string]any {
	out := make(map[string]any, len(existing)+len(owned))
	for key, value := range existing {
		out[key] = value
	}
	for key, value := range owned {
		out[key] = value
	}
	return out
}

func marshalCursorDocument(servers map[string]any, wrapped bool) ([]byte, error) {
	var body []byte
	var err error
	if wrapped {
		body, err = json.MarshalIndent(map[string]any{"mcpServers": servers}, "", "  ")
	} else {
		body, err = json.MarshalIndent(servers, "", "  ")
	}
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func (a Adapter) readDocument(ctx context.Context, path string) (map[string]any, bool, []byte, error) {
	body, err := a.fs().ReadFile(ctx, path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, true, nil, nil
		}
		return nil, false, nil, domain.NewError(domain.ErrMutationApply, "read Cursor MCP config", err)
	}
	doc, wrapped, err := a.readDocumentBytes(body)
	if err != nil {
		return nil, false, nil, err
	}
	return doc, wrapped, body, nil
}

func (a Adapter) readDocumentBytes(body []byte) (map[string]any, bool, error) {
	doc := map[string]any{}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, false, domain.NewError(domain.ErrMutationApply, "parse Cursor MCP config", err)
	}
	if raw, ok := doc["mcpServers"]; ok {
		servers, ok := raw.(map[string]any)
		if !ok {
			return nil, false, domain.NewError(domain.ErrMutationApply, "Cursor mcpServers must be a JSON object", nil)
		}
		return servers, true, nil
	}
	return doc, false, nil
}

func (a Adapter) verifyAliases(ctx context.Context, path string, aliases []string) error {
	doc, _, _, err := a.readDocument(ctx, path)
	if err != nil {
		return err
	}
	for _, alias := range aliases {
		if _, ok := doc[alias]; !ok {
			return domain.NewError(domain.ErrMutationApply, fmt.Sprintf("Cursor MCP alias %q was not persisted", alias), nil)
		}
	}
	return nil
}

func (a Adapter) verifyMissingAliases(ctx context.Context, path string, aliases []string) error {
	doc, _, _, err := a.readDocument(ctx, path)
	if err != nil {
		return err
	}
	for _, alias := range aliases {
		if _, ok := doc[alias]; ok {
			return domain.NewError(domain.ErrMutationApply, fmt.Sprintf("Cursor MCP alias %q still exists after removal", alias), nil)
		}
	}
	return nil
}

func protectionForScope(scope string) domain.ProtectionClass {
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		return domain.ProtectionWorkspace
	}
	return domain.ProtectionUserMutable
}

func ownedObjectsForConfig(path string, aliases []string, protection domain.ProtectionClass) []domain.NativeObjectRef {
	out := make([]domain.NativeObjectRef, 0, 1+len(aliases))
	out = append(out, domain.NativeObjectRef{
		Kind:            "file",
		Path:            path,
		ProtectionClass: protection,
	})
	for _, alias := range aliases {
		out = append(out, domain.NativeObjectRef{
			Kind:            "cursor_mcp_server",
			Name:            alias,
			Path:            path,
			ProtectionClass: protection,
		})
	}
	return out
}

func ownedAliases(items []domain.NativeObjectRef) []string {
	var out []string
	for _, item := range items {
		if item.Kind == "cursor_mcp_server" && strings.TrimSpace(item.Name) != "" {
			out = append(out, item.Name)
		}
	}
	sort.Strings(out)
	return out
}

func configPathFromTarget(target domain.TargetInstallation, fallback string) string {
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "file" && strings.TrimSpace(item.Path) != "" {
			return item.Path
		}
	}
	if metadataPath, ok := target.AdapterMetadata["config_path"].(string); ok && strings.TrimSpace(metadataPath) != "" {
		return metadataPath
	}
	return fallback
}
