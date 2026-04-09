package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/portablemcp"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
	"github.com/tailscale/hujson"
	"gopkg.in/yaml.v3"
)

type Adapter struct {
	FS          ports.FileSystem
	ProjectRoot string
	UserHome    string
}

type packageMeta struct {
	Plugins []pluginRef `yaml:"plugins,omitempty"`
}

type pluginRef struct {
	Name    string
	Options map[string]any
}

func (r *pluginRef) UnmarshalYAML(node *yaml.Node) error {
	if node == nil {
		*r = pluginRef{}
		return nil
	}
	switch node.Kind {
	case yaml.ScalarNode:
		var name string
		if err := node.Decode(&name); err != nil {
			return err
		}
		r.Name = strings.TrimSpace(name)
		r.Options = nil
		return nil
	case yaml.MappingNode:
		var raw map[string]any
		if err := node.Decode(&raw); err != nil {
			return err
		}
		for key := range raw {
			switch key {
			case "name", "options":
			default:
				return errors.New("unsupported OpenCode plugin metadata field " + key)
			}
		}
		name, _ := raw["name"].(string)
		r.Name = strings.TrimSpace(name)
		if options, ok := raw["options"]; ok {
			typed, ok := options.(map[string]any)
			if !ok {
				return errors.New("OpenCode plugin metadata options must be a mapping")
			}
			r.Options = typed
		}
		return nil
	default:
		return errors.New("OpenCode plugin metadata entries must be strings or mappings")
	}
}

func (r pluginRef) jsonValue() any {
	if len(r.Options) == 0 {
		return r.Name
	}
	return []any{r.Name, r.Options}
}

type sourceMaterial struct {
	WholeFields map[string]any
	Plugins     []pluginRef
	MCP         map[string]any
	CopyFiles   []copyFile
}

type configMutation struct {
	WholeSet      map[string]any
	WholeRemove   []string
	PluginsSet    []pluginRef
	PluginsRemove []string
	MCPSet        map[string]any
	MCPRemove     []string
}

type configPatchResult struct {
	Body            []byte
	ConfigPath      string
	ManagedKeys     []string
	OwnedPluginRefs []string
	OwnedMCPAliases []string
}

type copyFile struct {
	Source      string
	Destination string
}

type inspectSurface struct {
	ConfigPath              string
	SettingsFiles           []string
	ConfigPrecedenceContext []string
	EnvironmentRestrictions []domain.EnvironmentRestrictionCode
	VolatileOverride        bool
	SourceAccessState       string
}

func (Adapter) ID() domain.TargetID { return domain.TargetOpenCode }

func (Adapter) Capabilities(context.Context) (ports.Capabilities, error) {
	return ports.Capabilities{
		InstallMode:          "config_projection",
		SupportsNativeUpdate: false,
		SupportsNativeRemove: true,
		SupportsScopeUser:    true,
		SupportsScopeProject: true,
		SupportsRepair:       true,
		SupportedSourceKinds: []string{"local_path", "github_repo_path", "git_url"},
		EvidenceKey:          "target.opencode.native_surface",
	}, nil
}

func (a Adapter) Inspect(_ context.Context, in ports.InspectInput) (ports.InspectResult, error) {
	surface := a.inspectSurface(in.Scope)
	config := surface.ConfigPath
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
		SourceAccessState:        surface.SourceAccessState,
		SettingsFiles:            append([]string(nil), surface.SettingsFiles...),
		EvidenceClass:            domain.EvidenceConfirmed,
	}, nil
}

func (a Adapter) PlanInstall(ctx context.Context, in ports.PlanInstallInput) (ports.AdapterPlan, error) {
	configPath := a.configPath(in.Policy.Scope)
	paths := []string{configPath}
	if root := a.assetsRoot(in.Policy.Scope); root != "" {
		paths = append(paths, root)
	}
	manualSteps, blocking := planBlockingManualSteps(in.Inspect)
	return ports.AdapterPlan{
		TargetID:        a.ID(),
		ActionClass:     "install_missing",
		Summary:         "Project or global OpenCode projection",
		RestartRequired: true,
		PathsTouched:    paths,
		ManualSteps:     manualSteps,
		Blocking:        blocking,
		EvidenceKey:     "target.opencode.native_surface",
	}, nil
}

func (a Adapter) ApplyInstall(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if err := a.requireMutableEnvironment(); err != nil {
		return ports.ApplyResult{}, err
	}
	if in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "OpenCode apply requires resolved source", nil)
	}
	material, err := a.loadSourceMaterial(ctx, in.ResolvedSource.LocalPath, in.Policy.Scope)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	configPath := a.configPath(in.Policy.Scope)
	patch, err := a.patchConfig(ctx, configPath, configMutation{
		WholeSet:   material.WholeFields,
		PluginsSet: material.Plugins,
		MCPSet:     material.MCP,
	}, nil)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	copiedPaths, err := a.copyOwnedFiles(material.CopyFiles)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallInstalled,
		ActivationState:    domain.ActivationRestartPending,
		OwnedNativeObjects: ownedObjects(patch.ConfigPath, patch.ManagedKeys, patch.OwnedPluginRefs, patch.OwnedMCPAliases, copiedPaths, protectionForScope(in.Policy.Scope)),
		EvidenceClass:      domain.EvidenceConfirmed,
		ManualSteps:        []string{"restart OpenCode to pick up updated config and projected files"},
		AdapterMetadata: map[string]any{
			"config_path":          patch.ConfigPath,
			"managed_config_keys":  patch.ManagedKeys,
			"owned_plugin_refs":    patch.OwnedPluginRefs,
			"owned_mcp_aliases":    patch.OwnedMCPAliases,
			"copied_paths":         copiedPaths,
			"config_body_checksum": len(patch.Body),
		},
	}, nil
}

func (a Adapter) PlanUpdate(_ context.Context, in ports.PlanUpdateInput) (ports.AdapterPlan, error) {
	configPath := a.configPath("user")
	if target, ok := in.CurrentRecord.Targets[domain.TargetOpenCode]; ok {
		configPath = configPathFromTarget(target, configPath)
	}
	manualSteps, blocking := planBlockingManualSteps(in.Inspect)
	return ports.AdapterPlan{
		TargetID:        a.ID(),
		ActionClass:     "update_version",
		Summary:         "Owned projection refresh for OpenCode",
		RestartRequired: true,
		PathsTouched:    []string{configPath, a.assetsRoot(in.CurrentRecord.Policy.Scope)},
		ManualSteps:     manualSteps,
		Blocking:        blocking,
		EvidenceKey:     "target.opencode.native_surface",
	}, nil
}

func (a Adapter) ApplyUpdate(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if err := a.requireMutableEnvironment(); err != nil {
		return ports.ApplyResult{}, err
	}
	if in.Record == nil || in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "OpenCode update requires current record and resolved source", nil)
	}
	target, ok := in.Record.Targets[domain.TargetOpenCode]
	if !ok {
		return ports.ApplyResult{}, domain.NewError(domain.ErrStateConflict, "OpenCode target is missing from installation record", nil)
	}
	material, err := a.loadSourceMaterial(ctx, in.ResolvedSource.LocalPath, in.Record.Policy.Scope)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	configPath := configPathFromTarget(target, a.configPath(in.Record.Policy.Scope))
	patch, err := a.patchConfig(ctx, configPath, material.mutationForUpdate(target), &target)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	nextCopiedPaths, err := a.copyOwnedFiles(material.CopyFiles)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	if err := a.removeStaleFiles(ctx, copiedFilePaths(target), nextCopiedPaths); err != nil {
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallInstalled,
		ActivationState:    domain.ActivationRestartPending,
		OwnedNativeObjects: ownedObjects(patch.ConfigPath, patch.ManagedKeys, patch.OwnedPluginRefs, patch.OwnedMCPAliases, nextCopiedPaths, protectionForScope(in.Record.Policy.Scope)),
		EvidenceClass:      domain.EvidenceConfirmed,
		ManualSteps:        []string{"restart OpenCode to pick up updated config and projected files"},
		AdapterMetadata: map[string]any{
			"config_path":          patch.ConfigPath,
			"managed_config_keys":  patch.ManagedKeys,
			"owned_plugin_refs":    patch.OwnedPluginRefs,
			"owned_mcp_aliases":    patch.OwnedMCPAliases,
			"copied_paths":         nextCopiedPaths,
			"config_body_checksum": len(patch.Body),
		},
	}, nil
}

func (a Adapter) PlanRemove(_ context.Context, in ports.PlanRemoveInput) (ports.AdapterPlan, error) {
	configPath := a.configPath("user")
	if target, ok := in.Record.Targets[domain.TargetOpenCode]; ok {
		configPath = configPathFromTarget(target, configPath)
	}
	manualSteps, blocking := planBlockingManualSteps(in.Inspect)
	return ports.AdapterPlan{
		TargetID:     a.ID(),
		ActionClass:  "remove_orphaned_target",
		Summary:      "Remove owned OpenCode projection",
		PathsTouched: []string{configPath, a.assetsRoot(in.Record.Policy.Scope)},
		ManualSteps:  manualSteps,
		Blocking:     blocking,
		EvidenceKey:  "target.opencode.native_surface",
	}, nil
}

func (a Adapter) ApplyRemove(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if err := a.requireMutableEnvironment(); err != nil {
		return ports.ApplyResult{}, err
	}
	if in.Record == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "OpenCode remove requires current record", nil)
	}
	target, ok := in.Record.Targets[domain.TargetOpenCode]
	if !ok {
		return ports.ApplyResult{}, domain.NewError(domain.ErrStateConflict, "OpenCode target is missing from installation record", nil)
	}
	configPath := configPathFromTarget(target, a.configPath(in.Record.Policy.Scope))
	patch, err := a.patchConfig(ctx, configPath, configMutation{
		WholeRemove:   ownedConfigKeys(target),
		PluginsRemove: ownedPluginRefs(target),
		MCPRemove:     ownedMCPAliases(target),
	}, &target)
	if err != nil {
		return ports.ApplyResult{}, err
	}
	if err := a.removeStaleFiles(ctx, copiedFilePaths(target), nil); err != nil {
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:        a.ID(),
		State:           domain.InstallRemoved,
		ActivationState: domain.ActivationRestartPending,
		EvidenceClass:   domain.EvidenceConfirmed,
		ManualSteps:     []string{"restart OpenCode to unload removed managed config and projected files"},
		AdapterMetadata: map[string]any{
			"config_path":          patch.ConfigPath,
			"managed_config_keys":  nil,
			"owned_plugin_refs":    nil,
			"owned_mcp_aliases":    nil,
			"copied_paths":         nil,
			"config_body_checksum": len(patch.Body),
		},
	}, nil
}

func (a Adapter) Repair(ctx context.Context, in ports.RepairInput) (ports.ApplyResult, error) {
	if err := a.requireMutableEnvironment(); err != nil {
		return ports.ApplyResult{}, err
	}
	if in.Manifest == nil || in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrRepairApply, "OpenCode repair requires resolved source and manifest", nil)
	}
	result, err := a.ApplyUpdate(ctx, ports.ApplyInput{
		Manifest:       *in.Manifest,
		ResolvedSource: in.ResolvedSource,
		Policy:         in.Record.Policy,
		Inspect:        in.Inspect,
		Record:         &in.Record,
	})
	if err != nil {
		return ports.ApplyResult{}, err
	}
	if len(result.ManualSteps) == 0 {
		result.ManualSteps = append(result.ManualSteps, "repair reconciled managed OpenCode config and projected files")
	}
	return result, nil
}

func (a Adapter) fs() ports.FileSystem {
	if a.FS != nil {
		return a.FS
	}
	return fsadapter.OS{}
}

func (a Adapter) configPath(scope string) string {
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		return preferredConfigPath(
			filepath.Join(a.effectiveProjectRoot(), "opencode.json"),
			filepath.Join(a.effectiveProjectRoot(), "opencode.jsonc"),
		)
	}
	return preferredConfigPath(
		filepath.Join(a.userHome(), ".config", "opencode", "opencode.json"),
		filepath.Join(a.userHome(), ".config", "opencode", "opencode.jsonc"),
		filepath.Join(a.userHome(), ".local", "share", "opencode", "opencode.jsonc"),
	)
}

func (a Adapter) assetsRoot(scope string) string {
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		return filepath.Join(a.effectiveProjectRoot(), ".opencode")
	}
	return filepath.Join(a.userHome(), ".config", "opencode")
}

func (a Adapter) projectRoot() string {
	if strings.TrimSpace(a.ProjectRoot) != "" {
		return a.ProjectRoot
	}
	cwd, _ := os.Getwd()
	return cwd
}

func (a Adapter) effectiveProjectRoot() string {
	root := filepath.Clean(a.projectRoot())
	for {
		if root == "." || root == string(filepath.Separator) || strings.TrimSpace(root) == "" {
			return a.projectRoot()
		}
		if fileExists(filepath.Join(root, ".git")) {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			return a.projectRoot()
		}
		root = parent
	}
}

func (a Adapter) userHome() string {
	if strings.TrimSpace(a.UserHome) != "" {
		return a.UserHome
	}
	home, _ := os.UserHomeDir()
	return home
}

func (a Adapter) loadSourceMaterial(ctx context.Context, sourceRoot, scope string) (sourceMaterial, error) {
	fields := map[string]any{
		"$schema": "https://opencode.ai/config.json",
	}
	plugins, err := readPlugins(filepath.Join(sourceRoot, "src", "targets", "opencode", "package.yaml"))
	if err != nil {
		return sourceMaterial{}, err
	}
	loader := portablemcp.Loader{FS: a.fs()}
	if loaded, err := loader.LoadForTarget(ctx, sourceRoot, domain.TargetOpenCode); err == nil {
		projected := renderOpenCodeMCP(loaded, sourceRoot)
		if len(projected) == 0 {
			projected = nil
		}
		material := sourceMaterial{WholeFields: fields, Plugins: plugins, MCP: projected}
		if err := material.loadFirstClassDocs(sourceRoot); err != nil {
			return sourceMaterial{}, err
		}
		extra, err := readConfigExtra(filepath.Join(sourceRoot, "src", "targets", "opencode", "config.extra.json"))
		if err != nil {
			return sourceMaterial{}, err
		}
		if err := material.mergeExtra(extra); err != nil {
			return sourceMaterial{}, err
		}
		copyFiles, err := collectCopyFiles(sourceRoot, a.assetsRoot(scope))
		if err != nil {
			return sourceMaterial{}, err
		}
		material.CopyFiles = copyFiles
		return material, nil
	} else if !isMissingPortableMCP(err) {
		return sourceMaterial{}, err
	}
	material := sourceMaterial{WholeFields: fields, Plugins: plugins}
	if err := material.loadFirstClassDocs(sourceRoot); err != nil {
		return sourceMaterial{}, err
	}
	extra, err := readConfigExtra(filepath.Join(sourceRoot, "src", "targets", "opencode", "config.extra.json"))
	if err != nil {
		return sourceMaterial{}, err
	}
	if err := material.mergeExtra(extra); err != nil {
		return sourceMaterial{}, err
	}
	copyFiles, err := collectCopyFiles(sourceRoot, a.assetsRoot(scope))
	if err != nil {
		return sourceMaterial{}, err
	}
	material.CopyFiles = copyFiles
	return material, nil
}

func renderOpenCodeMCP(loaded portablemcp.Loaded, sourceRoot string) map[string]any {
	out := make(map[string]any, len(loaded.Servers))
	for alias, server := range loaded.Servers {
		switch server.Type {
		case "stdio":
			command := make([]any, 0, 1+len(server.Stdio.Args))
			command = append(command, interpolatePackageRoot(server.Stdio.Command, sourceRoot))
			for _, arg := range server.Stdio.Args {
				command = append(command, interpolatePackageRoot(arg, sourceRoot))
			}
			entry := map[string]any{
				"type":    "local",
				"command": command,
			}
			if len(server.Stdio.Env) > 0 {
				env := make(map[string]any, len(server.Stdio.Env))
				for key, value := range server.Stdio.Env {
					env[key] = interpolatePackageRoot(value, sourceRoot)
				}
				entry["environment"] = env
			}
			out[alias] = entry
		case "remote":
			entry := map[string]any{
				"type": "remote",
				"url":  interpolatePackageRoot(server.Remote.URL, sourceRoot),
			}
			if len(server.Remote.Headers) > 0 {
				headers := make(map[string]any, len(server.Remote.Headers))
				for key, value := range server.Remote.Headers {
					headers[key] = interpolatePackageRoot(value, sourceRoot)
				}
				entry["headers"] = headers
			}
			out[alias] = entry
		}
	}
	return out
}

func (a Adapter) patchConfig(ctx context.Context, path string, mutation configMutation, target *domain.TargetInstallation) (configPatchResult, error) {
	body, err := a.fs().ReadFile(ctx, path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "read OpenCode config", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		body = []byte("{}\n")
	}
	ast, err := hujson.Parse(body)
	if err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "parse OpenCode config", err)
	}
	obj, ok := ast.Value.(*hujson.Object)
	if !ok {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "OpenCode config root must be an object", nil)
	}
	doc, err := decodeConfigMap(body)
	if err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "decode OpenCode config", err)
	}
	currentPlugins, err := existingPluginRefs(doc["plugin"])
	if err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "parse OpenCode plugin refs", err)
	}
	currentMCP, err := existingObjectMap(doc["mcp"], "mcp")
	if err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "parse OpenCode MCP config", err)
	}
	oldPluginRefs := mapFromSlice(ownedPluginRefsOrMetadata(target), func(value string) string { return value })
	oldMCPAliases := mapFromSlice(ownedMCPAliasesOrMetadata(target), func(value string) string { return value })
	for _, ref := range mutation.PluginsSet {
		name := strings.TrimSpace(ref.Name)
		if name == "" {
			continue
		}
		if existing, ok := currentPlugins[name]; ok && !oldPluginRefs[name] && !pluginRefsEqual(existing, ref) {
			return configPatchResult{}, domain.NewError(domain.ErrStateConflict, "OpenCode plugin ref conflict for "+name, nil)
		}
	}
	for alias, desired := range mutation.MCPSet {
		if existing, ok := currentMCP[alias]; ok && !oldMCPAliases[alias] && !jsonValuesEqual(existing, desired) {
			return configPatchResult{}, domain.NewError(domain.ErrStateConflict, "OpenCode MCP alias conflict for "+alias, nil)
		}
	}
	for _, key := range mutation.WholeRemove {
		if strings.TrimSpace(key) == "" || key == "$schema" {
			continue
		}
		removeTopLevelMember(obj, key)
	}
	mergedPlugins := mergePluginRefs(currentPlugins, mutation.PluginsRemove, mutation.PluginsSet)
	if len(mergedPlugins) == 0 {
		removeTopLevelMember(obj, "plugin")
	} else if err := setTopLevelMember(obj, "plugin", pluginRefsToJSON(mergedPlugins)); err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "patch OpenCode plugin refs", err)
	}
	mergedMCP := mergeNamedObject(currentMCP, mutation.MCPRemove, mutation.MCPSet)
	if len(mergedMCP) == 0 {
		removeTopLevelMember(obj, "mcp")
	} else if err := setTopLevelMember(obj, "mcp", mergedMCP); err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "patch OpenCode MCP config", err)
	}
	for key, value := range mutation.WholeSet {
		if err := setTopLevelMember(obj, key, value); err != nil {
			return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "patch OpenCode config", err)
		}
	}
	if len(obj.Members) == 0 {
		if err := a.fs().Remove(ctx, path); err != nil {
			return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "remove empty OpenCode config", err)
		}
		return configPatchResult{
			ConfigPath: path,
		}, nil
	}
	rendered := ast.Pack()
	if err := a.fs().WriteFileAtomic(ctx, path, rendered, 0o644); err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "write OpenCode config", err)
	}
	return configPatchResult{
		Body:            rendered,
		ConfigPath:      path,
		ManagedKeys:     sortedManagedKeys(mutation.WholeSet),
		OwnedPluginRefs: pluginRefNames(mutation.PluginsSet),
		OwnedMCPAliases: sortedMapKeys(mutation.MCPSet),
	}, nil
}

func (a Adapter) copyOwnedFiles(files []copyFile) ([]string, error) {
	if len(files) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(files))
	for _, item := range files {
		body, err := os.ReadFile(item.Source)
		if err != nil {
			return nil, domain.NewError(domain.ErrMutationApply, "read OpenCode source asset", err)
		}
		if err := a.fs().WriteFileAtomic(context.Background(), item.Destination, body, 0o644); err != nil {
			return nil, domain.NewError(domain.ErrMutationApply, "write OpenCode projected asset", err)
		}
		out = append(out, item.Destination)
	}
	sort.Strings(out)
	return out, nil
}

func collectCopyFiles(sourceRoot, assetsRoot string) ([]copyFile, error) {
	type pair struct{ src, dst string }
	pairs := []pair{
		{filepath.Join(sourceRoot, "src", "targets", "opencode", "commands"), filepath.Join(assetsRoot, "commands")},
		{filepath.Join(sourceRoot, "src", "targets", "opencode", "agents"), filepath.Join(assetsRoot, "agents")},
		{filepath.Join(sourceRoot, "src", "targets", "opencode", "themes"), filepath.Join(assetsRoot, "themes")},
		{filepath.Join(sourceRoot, "src", "targets", "opencode", "tools"), filepath.Join(assetsRoot, "tools")},
		{filepath.Join(sourceRoot, "src", "targets", "opencode", "plugins"), filepath.Join(assetsRoot, "plugins")},
		{filepath.Join(sourceRoot, "src", "skills"), filepath.Join(assetsRoot, "skills")},
	}
	var out []copyFile
	for _, pair := range pairs {
		if _, err := os.Stat(pair.src); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		err := filepath.WalkDir(pair.src, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(pair.src, path)
			if err != nil {
				return err
			}
			out = append(out, copyFile{
				Source:      path,
				Destination: filepath.Join(pair.dst, rel),
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if pkg := filepath.Join(sourceRoot, "src", "targets", "opencode", "package.json"); fileExists(pkg) {
		out = append(out, copyFile{Source: pkg, Destination: filepath.Join(assetsRoot, "package.json")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Destination < out[j].Destination })
	return out, nil
}

func readPlugins(path string) ([]pluginRef, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, domain.NewError(domain.ErrManifestLoad, "read OpenCode package metadata", err)
	}
	var meta packageMeta
	if err := yaml.Unmarshal(body, &meta); err != nil {
		return nil, domain.NewError(domain.ErrManifestLoad, "parse OpenCode package metadata", err)
	}
	var out []pluginRef
	for _, plugin := range meta.Plugins {
		plugin.Name = strings.TrimSpace(plugin.Name)
		if plugin.Name == "" {
			continue
		}
		out = append(out, plugin)
	}
	return out, nil
}

func (m *sourceMaterial) loadFirstClassDocs(sourceRoot string) error {
	defaultAgentPath := filepath.Join(sourceRoot, "src", "targets", "opencode", "default_agent.txt")
	if fileExists(defaultAgentPath) {
		body, err := os.ReadFile(defaultAgentPath)
		if err != nil {
			return domain.NewError(domain.ErrManifestLoad, "read OpenCode default agent", err)
		}
		text := strings.TrimSpace(string(body))
		if text == "" {
			return domain.NewError(domain.ErrManifestLoad, "OpenCode default agent must be a non-empty string", nil)
		}
		m.WholeFields["default_agent"] = text
	}
	instructionsPath := filepath.Join(sourceRoot, "src", "targets", "opencode", "instructions.yaml")
	if fileExists(instructionsPath) {
		body, err := os.ReadFile(instructionsPath)
		if err != nil {
			return domain.NewError(domain.ErrManifestLoad, "read OpenCode instructions", err)
		}
		var instructions []string
		if err := yaml.Unmarshal(body, &instructions); err != nil {
			return domain.NewError(domain.ErrManifestLoad, "parse OpenCode instructions", err)
		}
		if len(instructions) == 0 {
			return domain.NewError(domain.ErrManifestLoad, "OpenCode instructions must contain at least one path", nil)
		}
		for i, item := range instructions {
			instructions[i] = strings.TrimSpace(item)
			if instructions[i] == "" {
				return domain.NewError(domain.ErrManifestLoad, "OpenCode instructions must contain only non-empty paths", nil)
			}
		}
		m.WholeFields["instructions"] = instructions
	}
	permissionPath := filepath.Join(sourceRoot, "src", "targets", "opencode", "permission.json")
	if fileExists(permissionPath) {
		body, err := os.ReadFile(permissionPath)
		if err != nil {
			return domain.NewError(domain.ErrManifestLoad, "read OpenCode permission config", err)
		}
		var permission any
		if err := json.Unmarshal(body, &permission); err != nil {
			return domain.NewError(domain.ErrManifestLoad, "parse OpenCode permission config", err)
		}
		if !isPermissionValue(permission) {
			return domain.NewError(domain.ErrManifestLoad, "OpenCode permission must be a string or object", nil)
		}
		m.WholeFields["permission"] = permission
	}
	return nil
}

func (m *sourceMaterial) mergeExtra(extra map[string]any) error {
	for key, value := range extra {
		if _, exists := m.WholeFields[key]; exists || key == "plugin" || key == "mcp" || key == "mode" {
			return domain.NewError(domain.ErrManifestLoad, "OpenCode config.extra.json conflicts with managed key "+key, nil)
		}
		m.WholeFields[key] = value
	}
	return nil
}

func (m sourceMaterial) mutationForUpdate(target domain.TargetInstallation) configMutation {
	currentKeys := ownedConfigKeys(target)
	currentPlugins := ownedPluginRefs(target)
	currentMCP := ownedMCPAliases(target)
	return configMutation{
		WholeSet:      m.WholeFields,
		WholeRemove:   subtractStrings(currentKeys, sortedManagedKeys(m.WholeFields)),
		PluginsSet:    m.Plugins,
		PluginsRemove: subtractStrings(currentPlugins, pluginRefNames(m.Plugins)),
		MCPSet:        m.MCP,
		MCPRemove:     subtractStrings(currentMCP, sortedMapKeys(m.MCP)),
	}
}

func readConfigExtra(path string) (map[string]any, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, domain.NewError(domain.ErrManifestLoad, "read OpenCode config.extra.json", err)
	}
	var extra map[string]any
	if err := json.Unmarshal(body, &extra); err != nil {
		return nil, domain.NewError(domain.ErrManifestLoad, "parse OpenCode config.extra.json", err)
	}
	return extra, nil
}

func setTopLevelMember(obj *hujson.Object, key string, value any) error {
	memberValue, err := valueToHuJSONValue(value)
	if err != nil {
		return err
	}
	for i := range obj.Members {
		name := obj.Members[i].Name.Value.(hujson.Literal).String()
		if name == key {
			memberValue.BeforeExtra = obj.Members[i].Value.BeforeExtra
			memberValue.AfterExtra = obj.Members[i].Value.AfterExtra
			obj.Members[i].Value = memberValue
			return nil
		}
	}
	nameValue := hujson.Value{Value: hujson.String(key)}
	memberValue.BeforeExtra = []byte("\n  ")
	memberValue.AfterExtra = []byte{}
	obj.Members = append(obj.Members, hujson.ObjectMember{Name: nameValue, Value: memberValue})
	return nil
}

func valueToHuJSONValue(value any) (hujson.Value, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return hujson.Value{}, err
	}
	parsed, err := hujson.Parse(body)
	if err != nil {
		return hujson.Value{}, err
	}
	return parsed, nil
}

func removeTopLevelMember(obj *hujson.Object, key string) {
	filtered := obj.Members[:0]
	for i := range obj.Members {
		name := obj.Members[i].Name.Value.(hujson.Literal).String()
		if name != key {
			filtered = append(filtered, obj.Members[i])
		}
	}
	obj.Members = filtered
}

func decodeConfigMap(body []byte) (map[string]any, error) {
	body, err := hujson.Standardize(body)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}

func existingObjectMap(raw any, field string) (map[string]any, error) {
	if raw == nil {
		return map[string]any{}, nil
	}
	typed, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("OpenCode field " + field + " must be an object")
	}
	return typed, nil
}

func existingPluginRefs(raw any) (map[string]pluginRef, error) {
	if raw == nil {
		return map[string]pluginRef{}, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, errors.New("OpenCode field plugin must be an array")
	}
	out := make(map[string]pluginRef, len(values))
	for i, value := range values {
		ref, err := normalizePluginRef(value)
		if err != nil {
			return nil, errors.New("OpenCode plugin ref at index " + strconvI(i) + " is invalid: " + err.Error())
		}
		out[ref.Name] = ref
	}
	return out, nil
}

func normalizePluginRef(value any) (pluginRef, error) {
	switch typed := value.(type) {
	case string:
		name := strings.TrimSpace(typed)
		if name == "" {
			return pluginRef{}, errors.New("plugin ref must be a non-empty string")
		}
		return pluginRef{Name: name}, nil
	case []any:
		if len(typed) != 2 {
			return pluginRef{}, errors.New("tuple plugin ref must have exactly 2 items")
		}
		name, ok := typed[0].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return pluginRef{}, errors.New("tuple plugin ref name must be a non-empty string")
		}
		options, ok := typed[1].(map[string]any)
		if !ok {
			return pluginRef{}, errors.New("tuple plugin ref options must be an object")
		}
		return pluginRef{Name: strings.TrimSpace(name), Options: options}, nil
	default:
		return pluginRef{}, errors.New("plugin ref must be a string or [name, options] tuple")
	}
}

func mergePluginRefs(existing map[string]pluginRef, remove []string, set []pluginRef) []pluginRef {
	removeSet := mapFromSlice(remove, func(value string) string { return value })
	setMap := make(map[string]pluginRef, len(set))
	for _, ref := range set {
		if strings.TrimSpace(ref.Name) == "" {
			continue
		}
		setMap[ref.Name] = ref
	}
	for name := range setMap {
		removeSet[name] = true
	}
	out := make([]pluginRef, 0, len(existing)+len(setMap))
	for name, ref := range existing {
		if !removeSet[name] {
			out = append(out, ref)
		}
	}
	for _, ref := range setMap {
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func mergeNamedObject(existing map[string]any, remove []string, set map[string]any) map[string]any {
	out := make(map[string]any, len(existing)+len(set))
	removeSet := mapFromSlice(remove, func(value string) string { return value })
	for key, value := range existing {
		if !removeSet[key] {
			out[key] = value
		}
	}
	for key, value := range set {
		out[key] = value
	}
	return out
}

func pluginRefsToJSON(refs []pluginRef) []any {
	out := make([]any, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.jsonValue())
	}
	return out
}

func pluginRefNames(refs []pluginRef) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref.Name) != "" {
			out = append(out, ref.Name)
		}
	}
	sort.Strings(out)
	return out
}

func sortedManagedKeys(values map[string]any) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		if key == "$schema" {
			continue
		}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func isPermissionValue(value any) bool {
	if _, ok := value.(string); ok {
		return true
	}
	_, ok := value.(map[string]any)
	return ok
}

func preferredConfigPath(candidates ...string) string {
	for _, path := range candidates {
		if fileExists(path) {
			return path
		}
	}
	for _, path := range candidates {
		if strings.TrimSpace(path) != "" {
			return path
		}
	}
	return ""
}

func ownedObjects(configPath string, managedKeys, pluginRefs, mcpAliases, copiedPaths []string, protection domain.ProtectionClass) []domain.NativeObjectRef {
	out := []domain.NativeObjectRef{{
		Kind:            "file",
		Path:            configPath,
		ProtectionClass: protection,
	}}
	for _, key := range managedKeys {
		out = append(out, domain.NativeObjectRef{
			Kind:            "opencode_config_key",
			Name:            key,
			Path:            configPath,
			ProtectionClass: protection,
		})
	}
	for _, name := range pluginRefs {
		out = append(out, domain.NativeObjectRef{
			Kind:            "opencode_plugin_ref",
			Name:            name,
			Path:            configPath,
			ProtectionClass: protection,
		})
	}
	for _, alias := range mcpAliases {
		out = append(out, domain.NativeObjectRef{
			Kind:            "opencode_mcp_server",
			Name:            alias,
			Path:            configPath,
			ProtectionClass: protection,
		})
	}
	for _, path := range copiedPaths {
		out = append(out, domain.NativeObjectRef{
			Kind:            "file",
			Path:            path,
			ProtectionClass: protection,
		})
	}
	return out
}

func protectionForScope(scope string) domain.ProtectionClass {
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		return domain.ProtectionWorkspace
	}
	return domain.ProtectionUserMutable
}

func sortedMapKeys(values map[string]any) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func interpolatePackageRoot(value, packageRoot string) string {
	return strings.ReplaceAll(value, "${package.root}", packageRoot)
}

func configPathFromTarget(target domain.TargetInstallation, fallback string) string {
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "file" && strings.TrimSpace(item.Path) != "" && (strings.HasSuffix(item.Path, "opencode.json") || strings.HasSuffix(item.Path, "opencode.jsonc")) {
			return item.Path
		}
	}
	if metadataPath, ok := target.AdapterMetadata["config_path"].(string); ok && strings.TrimSpace(metadataPath) != "" {
		return metadataPath
	}
	return fallback
}

func isMissingPortableMCP(err error) bool {
	if err == nil {
		return false
	}
	var de *domain.Error
	if errors.As(err, &de) {
		return de.Code == domain.ErrManifestLoad && strings.Contains(strings.ToLower(de.Message), "portable mcp file not found")
	}
	return false
}

func (a Adapter) inspectSurface(scope string) inspectSurface {
	settings := []string{}
	restrictions := []domain.EnvironmentRestrictionCode{}
	volatile := false
	sourceAccess := ""
	if strings.TrimSpace(os.Getenv("OPENCODE_CONFIG_CONTENT")) != "" {
		volatile = true
		restrictions = append(restrictions, domain.RestrictionVolatileOverride)
		sourceAccess = "inline_config_override"
	}
	if customFile := strings.TrimSpace(os.Getenv("OPENCODE_CONFIG")); customFile != "" {
		volatile = true
		restrictions = append(restrictions, domain.RestrictionVolatileOverride)
		settings = append(settings, customFile)
		if sourceAccess == "" {
			sourceAccess = "custom_config_file_override"
		}
	}
	if customDir := strings.TrimSpace(os.Getenv("OPENCODE_CONFIG_DIR")); customDir != "" {
		volatile = true
		restrictions = append(restrictions, domain.RestrictionVolatileOverride)
		settings = append(settings, customDir)
		if sourceAccess == "" {
			sourceAccess = "custom_config_dir_override"
		}
	}

	var configPath string
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		root := a.effectiveProjectRoot()
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
	return inspectSurface{
		ConfigPath:              configPath,
		SettingsFiles:           settings,
		ConfigPrecedenceContext: []string{"remote", "global", "custom_config", "project", ".opencode", "inline_config", "managed"},
		EnvironmentRestrictions: dedupeRestrictionCodes(restrictions),
		VolatileOverride:        volatile,
		SourceAccessState:       sourceAccess,
	}
}

func planBlockingManualSteps(inspect ports.InspectResult) ([]string, bool) {
	if !inspect.VolatileOverrideDetected {
		return nil, false
	}
	steps := []string{
		"unset OPENCODE_CONFIG, OPENCODE_CONFIG_DIR, and OPENCODE_CONFIG_CONTENT before mutating managed OpenCode state",
		"run the same command again after the volatile OpenCode override layer is removed",
	}
	return steps, true
}

func (a Adapter) requireMutableEnvironment() error {
	if strings.TrimSpace(os.Getenv("OPENCODE_CONFIG")) == "" &&
		strings.TrimSpace(os.Getenv("OPENCODE_CONFIG_DIR")) == "" &&
		strings.TrimSpace(os.Getenv("OPENCODE_CONFIG_CONTENT")) == "" {
		return nil
	}
	return domain.NewError(domain.ErrMutationApply, "OpenCode mutation is blocked while OPENCODE_CONFIG, OPENCODE_CONFIG_DIR, or OPENCODE_CONFIG_CONTENT is set", nil)
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

func ownedConfigKeys(target domain.TargetInstallation) []string {
	var out []string
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "opencode_config_key" && strings.TrimSpace(item.Name) != "" {
			out = append(out, item.Name)
		}
	}
	if len(out) == 0 {
		if raw, ok := target.AdapterMetadata["managed_config_keys"].([]string); ok {
			out = append(out, raw...)
		} else if raw, ok := target.AdapterMetadata["managed_config_keys"].([]any); ok {
			for _, value := range raw {
				if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
					out = append(out, text)
				}
			}
		}
	}
	sort.Strings(out)
	return dedupeStrings(out)
}

func ownedPluginRefs(target domain.TargetInstallation) []string {
	var out []string
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "opencode_plugin_ref" && strings.TrimSpace(item.Name) != "" {
			out = append(out, item.Name)
		}
	}
	if len(out) == 0 {
		if raw, ok := target.AdapterMetadata["owned_plugin_refs"].([]string); ok {
			out = append(out, raw...)
		} else if raw, ok := target.AdapterMetadata["owned_plugin_refs"].([]any); ok {
			for _, value := range raw {
				if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
					out = append(out, text)
				}
			}
		}
	}
	sort.Strings(out)
	return dedupeStrings(out)
}

func ownedMCPAliases(target domain.TargetInstallation) []string {
	var out []string
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "opencode_mcp_server" && strings.TrimSpace(item.Name) != "" {
			out = append(out, item.Name)
		}
	}
	if len(out) == 0 {
		if raw, ok := target.AdapterMetadata["owned_mcp_aliases"].([]string); ok {
			out = append(out, raw...)
		} else if raw, ok := target.AdapterMetadata["owned_mcp_aliases"].([]any); ok {
			for _, value := range raw {
				if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
					out = append(out, text)
				}
			}
		}
	}
	sort.Strings(out)
	return dedupeStrings(out)
}

func ownedPluginRefsOrMetadata(target *domain.TargetInstallation) []string {
	if target == nil {
		return nil
	}
	return ownedPluginRefs(*target)
}

func ownedMCPAliasesOrMetadata(target *domain.TargetInstallation) []string {
	if target == nil {
		return nil
	}
	return ownedMCPAliases(*target)
}

func copiedFilePaths(target domain.TargetInstallation) []string {
	var out []string
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "file" && strings.TrimSpace(item.Path) != "" && !strings.HasSuffix(item.Path, "opencode.json") && !strings.HasSuffix(item.Path, "opencode.jsonc") {
			out = append(out, item.Path)
		}
	}
	sort.Strings(out)
	return dedupeStrings(out)
}

func (a Adapter) removeStaleFiles(ctx context.Context, previous, keep []string) error {
	keepSet := mapFromSlice(keep, func(value string) string { return value })
	for _, path := range previous {
		if keepSet[path] {
			continue
		}
		if err := a.fs().Remove(ctx, path); err != nil {
			return domain.NewError(domain.ErrMutationApply, "remove stale OpenCode projected asset", err)
		}
		a.removeEmptyParents(path, a.assetsRootForPath(path))
	}
	return nil
}

func (a Adapter) assetsRootForPath(path string) string {
	projectRoot := filepath.Join(a.projectRoot(), ".opencode")
	if strings.HasPrefix(path, projectRoot) {
		return projectRoot
	}
	return filepath.Join(a.userHome(), ".config", "opencode")
}

func (a Adapter) removeEmptyParents(path, stop string) {
	dir := filepath.Dir(path)
	stop = filepath.Clean(stop)
	for dir != "." && dir != string(filepath.Separator) {
		if filepath.Clean(dir) == stop {
			_ = a.fs().Remove(context.Background(), dir)
			return
		}
		if err := a.fs().Remove(context.Background(), dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func mapFromSlice[T any](values []T, keyFn func(T) string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		key := strings.TrimSpace(keyFn(value))
		if key != "" {
			out[key] = true
		}
	}
	return out
}

func subtractStrings(current, next []string) []string {
	nextSet := mapFromSlice(next, func(value string) string { return value })
	var out []string
	for _, item := range current {
		if !nextSet[item] {
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return dedupeStrings(out)
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := values[:0]
	var last string
	for _, value := range values {
		if value == "" || value == last {
			continue
		}
		out = append(out, value)
		last = value
	}
	return out
}

func pluginRefsEqual(left, right pluginRef) bool {
	if strings.TrimSpace(left.Name) != strings.TrimSpace(right.Name) {
		return false
	}
	return jsonValuesEqual(left.Options, right.Options)
}

func jsonValuesEqual(left, right any) bool {
	leftBody, err := json.Marshal(left)
	if err != nil {
		return false
	}
	rightBody, err := json.Marshal(right)
	if err != nil {
		return false
	}
	return string(leftBody) == string(rightBody)
}

func strconvI(value int) string {
	return strconv.Itoa(value)
}
