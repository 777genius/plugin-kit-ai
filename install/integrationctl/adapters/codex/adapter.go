package codex

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
	"github.com/pelletier/go-toml/v2"
)

type Adapter struct {
	FS          ports.FileSystem
	ProjectRoot string
	UserHome    string
}

func (Adapter) ID() domain.TargetID { return domain.TargetCodex }

func (Adapter) Capabilities(context.Context) (ports.Capabilities, error) {
	return ports.Capabilities{
		InstallMode:          "marketplace_prepare",
		SupportsNativeUpdate: false,
		SupportsNativeRemove: false,
		SupportsScopeUser:    true,
		SupportsScopeProject: true,
		SupportsRepair:       true,
		RequiresRestart:      true,
		RequiresNewThread:    true,
		SupportedSourceKinds: []string{"local_path", "github_repo_path", "git_url"},
		EvidenceKey:          "target.codex.native_surface",
	}, nil
}

func (a Adapter) Inspect(_ context.Context, in ports.InspectInput) (ports.InspectResult, error) {
	catalogPath := a.catalogPath(scopeForInspect(in))
	if in.Record != nil {
		if target, ok := in.Record.Targets[domain.TargetCodex]; ok {
			catalogPath = catalogPathFromTarget(target, catalogPath)
		}
	}
	configPath := filepath.Join(a.userHome(), ".codex", "config.toml")
	pluginRoot := pluginRootFromRecord(in.Record)
	if pluginRoot == "" {
		pluginRoot = a.pluginRoot(scopeForInspect(in), integrationIDForInspect(in.Record))
	}
	_, cmdErr := exec.LookPath("codex")
	catalogInfo, catalogErr := os.Stat(catalogPath)
	pluginInfo, pluginErr := os.Stat(pluginRoot)
	configInfo, configErr := os.Stat(configPath)
	restrictions := []domain.EnvironmentRestrictionCode{}
	if cmdErr != nil {
		restrictions = append(restrictions, domain.RestrictionSourceToolMissing)
	}
	state := domain.InstallRemoved
	activation := domain.ActivationNotRequired
	observed := []domain.NativeObjectRef{}
	if catalogErr == nil && !catalogInfo.IsDir() {
		observed = append(observed, domain.NativeObjectRef{
			Kind:            "marketplace_catalog",
			Path:            catalogPath,
			ProtectionClass: protectionForScope(scopeForInspect(in)),
		})
	}
	if pluginErr == nil && pluginInfo.IsDir() {
		observed = append(observed, domain.NativeObjectRef{
			Kind:            "plugin_root",
			Path:            pluginRoot,
			ProtectionClass: protectionForScope(scopeForInspect(in)),
		})
	}
	if configErr == nil && !configInfo.IsDir() {
		observed = append(observed, domain.NativeObjectRef{
			Kind:            "config_file",
			Path:            configPath,
			ProtectionClass: domain.ProtectionUserMutable,
		})
	}
	var catalogPolicy *domain.CatalogPolicySnapshot
	warnings := []string{}
	marketplaceName := marketplaceNameFromRecord(in.Record)
	if entry, found, err := readMarketplaceEntry(catalogPath, integrationIDForInspect(in.Record)); err == nil && found {
		catalogPolicy = policyFromEntry(entry)
	} else if err != nil {
		warnings = append(warnings, err.Error())
	}
	if strings.TrimSpace(marketplaceName) == "" {
		if doc, err := readMarketplace(catalogPath); err == nil {
			marketplaceName = strings.TrimSpace(doc.Name)
		}
	}
	cachePath := ""
	cacheInfo := os.FileInfo(nil)
	var cacheErr error
	if id := integrationIDForInspect(in.Record); id != "" && marketplaceName != "" {
		cachePath = a.cachePath(marketplaceName, id)
		cacheInfo, cacheErr = os.Stat(cachePath)
		if cacheErr == nil && cacheInfo != nil && cacheInfo.IsDir() {
			observed = append(observed, domain.NativeObjectRef{
				Kind:            "installed_cache_bundle",
				Name:            id,
				Path:            cachePath,
				ProtectionClass: domain.ProtectionUserMutable,
			})
		}
	}
	pluginRef := ""
	if id := integrationIDForInspect(in.Record); id != "" && marketplaceName != "" {
		pluginRef = id + "@" + marketplaceName
	}
	configState, configWarning := readPluginConfigState(configPath, pluginRef)
	if configWarning != "" {
		warnings = append(warnings, configWarning)
	}
	if configState.Present {
		observed = append(observed, domain.NativeObjectRef{
			Kind:            "plugin_toggle",
			Name:            pluginRef,
			Path:            configPath,
			ProtectionClass: domain.ProtectionUserMutable,
		})
	}
	cacheExists := cacheErr == nil && cacheInfo != nil && cacheInfo.IsDir()
	switch {
	case cacheExists && configState.Present && configState.Disabled:
		state = domain.InstallDisabled
		activation = domain.ActivationComplete
	case cacheExists:
		if catalogErr != nil || pluginErr != nil {
			warnings = append(warnings, "Codex installed cache bundle exists but managed marketplace source is missing or drifted")
			state = domain.InstallDegraded
		} else {
			state = domain.InstallInstalled
		}
		activation = domain.ActivationComplete
	case catalogErr == nil && pluginErr == nil:
		state = domain.InstallActivationPending
		activation = domain.ActivationNativePending
		restrictions = append(restrictions, domain.RestrictionNativeActivation, domain.RestrictionNewThreadRequired)
	case catalogErr == nil || pluginErr == nil:
		state = domain.InstallDegraded
		activation = domain.ActivationNativePending
		restrictions = append(restrictions, domain.RestrictionNativeActivation)
	default:
		state = domain.InstallRemoved
		activation = domain.ActivationNotRequired
	}
	return ports.InspectResult{
		TargetID:                a.ID(),
		Installed:               catalogErr == nil && pluginErr == nil,
		State:                   state,
		ActivationState:         activation,
		CatalogPolicy:           catalogPolicy,
		ConfigPrecedenceContext: []string{"repo_marketplace", "personal_marketplace", "cache", "config"},
		EnvironmentRestrictions: restrictions,
		ObservedNativeObjects:   observed,
		SettingsFiles:           []string{catalogPath, configPath},
		Warnings:                warnings,
		EvidenceClass:           domain.EvidenceConfirmed,
	}, nil
}

func (a Adapter) PlanInstall(_ context.Context, in ports.PlanInstallInput) (ports.AdapterPlan, error) {
	scope := in.Policy.Scope
	if strings.TrimSpace(scope) == "" {
		scope = "user"
	}
	catalogPath := a.catalogPath(scope)
	pluginRoot := a.pluginRoot(scope, in.Manifest.IntegrationID)
	return ports.AdapterPlan{
		TargetID:          a.ID(),
		ActionClass:       "install_missing",
		Summary:           "Materialize a Codex local marketplace entry and plugin bundle, then wait for native activation",
		PathsTouched:      []string{catalogPath, pluginRoot, filepath.Join(a.userHome(), ".codex", "config.toml")},
		NewThreadRequired: true,
		ManualSteps: manualInstallSteps(
			marketplaceLocationLabel(scope),
			in.Manifest.IntegrationID,
		),
		EvidenceKey: "target.codex.native_surface",
	}, nil
}

func (a Adapter) ApplyInstall(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Codex install requires resolved source", nil)
	}
	scope := normalizedScope(in.Policy.Scope)
	pluginRoot := a.pluginRoot(scope, in.Manifest.IntegrationID)
	catalogPath := a.catalogPath(scope)
	if err := a.syncManagedPlugin(ctx, in.Manifest, in.ResolvedSource.LocalPath, pluginRoot); err != nil {
		return ports.ApplyResult{}, err
	}
	catalogName, err := mergeMarketplaceEntry(catalogPath, marketplaceEntryDoc(in.Manifest, pluginRoot))
	if err != nil {
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallActivationPending,
		ActivationState:    domain.ActivationNativePending,
		OwnedNativeObjects: ownedObjects(scope, catalogPath, pluginRoot, in.Manifest.IntegrationID),
		EvidenceClass:      domain.EvidenceConfirmed,
		NewThreadRequired:  true,
		EnvironmentRestrictions: []domain.EnvironmentRestrictionCode{
			domain.RestrictionNativeActivation,
			domain.RestrictionNewThreadRequired,
		},
		ManualSteps: manualInstallSteps(
			marketplaceLocationLabel(scope),
			in.Manifest.IntegrationID,
		),
		AdapterMetadata: map[string]any{
			"catalog_path":      catalogPath,
			"catalog_name":      catalogName,
			"plugin_root":       pluginRoot,
			"plugin_name":       in.Manifest.IntegrationID,
			"activation_method": "plugin_directory_install",
		},
	}, nil
}

func (a Adapter) PlanUpdate(_ context.Context, in ports.PlanUpdateInput) (ports.AdapterPlan, error) {
	scope := normalizedScope(in.CurrentRecord.Policy.Scope)
	catalogPath := catalogPathFromTarget(in.CurrentRecord.Targets[domain.TargetCodex], a.catalogPath(scope))
	pluginRoot := pluginRootFromTarget(in.CurrentRecord.Targets[domain.TargetCodex], a.pluginRoot(scope, in.CurrentRecord.IntegrationID))
	return ports.AdapterPlan{
		TargetID:          a.ID(),
		ActionClass:       "update_version",
		Summary:           "Refresh the Codex plugin bundle and local marketplace entry",
		PathsTouched:      []string{catalogPath, pluginRoot, filepath.Join(a.userHome(), ".codex", "config.toml")},
		RestartRequired:   true,
		NewThreadRequired: true,
		ManualSteps:       manualUpdateSteps(in.CurrentRecord.IntegrationID),
		EvidenceKey:       "target.codex.native_surface",
	}, nil
}

func (a Adapter) ApplyUpdate(ctx context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.Record == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Codex update requires current record", nil)
	}
	if in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Codex update requires resolved source", nil)
	}
	target, ok := in.Record.Targets[domain.TargetCodex]
	if !ok {
		return ports.ApplyResult{}, domain.NewError(domain.ErrStateConflict, "Codex target is missing from installation record", nil)
	}
	scope := normalizedScope(in.Record.Policy.Scope)
	catalogPath := catalogPathFromTarget(target, a.catalogPath(scope))
	pluginRoot := pluginRootFromTarget(target, a.pluginRoot(scope, in.Record.IntegrationID))
	if err := a.syncManagedPlugin(ctx, in.Manifest, in.ResolvedSource.LocalPath, pluginRoot); err != nil {
		return ports.ApplyResult{}, err
	}
	catalogName, err := mergeMarketplaceEntry(catalogPath, marketplaceEntryDoc(in.Manifest, pluginRoot))
	if err != nil {
		return ports.ApplyResult{}, err
	}
	return ports.ApplyResult{
		TargetID:           a.ID(),
		State:              domain.InstallActivationPending,
		ActivationState:    domain.ActivationNativePending,
		OwnedNativeObjects: ownedObjects(scope, catalogPath, pluginRoot, in.Record.IntegrationID),
		EvidenceClass:      domain.EvidenceConfirmed,
		RestartRequired:    true,
		NewThreadRequired:  true,
		EnvironmentRestrictions: []domain.EnvironmentRestrictionCode{
			domain.RestrictionNativeActivation,
			domain.RestrictionRestartRequired,
			domain.RestrictionNewThreadRequired,
		},
		ManualSteps: manualUpdateSteps(in.Record.IntegrationID),
		AdapterMetadata: map[string]any{
			"catalog_path":      catalogPath,
			"catalog_name":      catalogName,
			"plugin_root":       pluginRoot,
			"plugin_name":       in.Record.IntegrationID,
			"activation_method": "plugin_directory_refresh",
		},
	}, nil
}

func (a Adapter) PlanRemove(_ context.Context, in ports.PlanRemoveInput) (ports.AdapterPlan, error) {
	scope := normalizedScope(in.Record.Policy.Scope)
	catalogPath := catalogPathFromTarget(in.Record.Targets[domain.TargetCodex], a.catalogPath(scope))
	pluginRoot := pluginRootFromTarget(in.Record.Targets[domain.TargetCodex], a.pluginRoot(scope, in.Record.IntegrationID))
	return ports.AdapterPlan{
		TargetID:        a.ID(),
		ActionClass:     "remove_orphaned_target",
		Summary:         "Remove the managed Codex marketplace entry and plugin bundle",
		PathsTouched:    []string{catalogPath, pluginRoot, filepath.Join(a.userHome(), ".codex", "config.toml")},
		ManualSteps:     manualRemoveSteps(in.Record.IntegrationID),
		RestartRequired: true,
		EvidenceKey:     "target.codex.native_surface",
	}, nil
}

func (a Adapter) ApplyRemove(_ context.Context, in ports.ApplyInput) (ports.ApplyResult, error) {
	if in.Record == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "Codex remove requires current record", nil)
	}
	target, ok := in.Record.Targets[domain.TargetCodex]
	if !ok {
		return ports.ApplyResult{}, domain.NewError(domain.ErrStateConflict, "Codex target is missing from installation record", nil)
	}
	scope := normalizedScope(in.Record.Policy.Scope)
	catalogPath := catalogPathFromTarget(target, a.catalogPath(scope))
	pluginRoot := pluginRootFromTarget(target, a.pluginRoot(scope, in.Record.IntegrationID))
	if err := removeMarketplaceEntry(catalogPath, in.Record.IntegrationID); err != nil {
		return ports.ApplyResult{}, err
	}
	if err := os.RemoveAll(pluginRoot); err != nil && !os.IsNotExist(err) {
		return ports.ApplyResult{}, domain.NewError(domain.ErrMutationApply, "remove Codex managed plugin root", err)
	}
	return ports.ApplyResult{
		TargetID:        a.ID(),
		State:           domain.InstallRemoved,
		ActivationState: domain.ActivationRestartPending,
		EvidenceClass:   domain.EvidenceConfirmed,
		RestartRequired: true,
		ManualSteps:     manualRemoveSteps(in.Record.IntegrationID),
		AdapterMetadata: map[string]any{
			"catalog_path": catalogPath,
			"plugin_root":  pluginRoot,
			"plugin_name":  in.Record.IntegrationID,
		},
	}, nil
}

func (a Adapter) Repair(ctx context.Context, in ports.RepairInput) (ports.ApplyResult, error) {
	if in.Manifest == nil || in.ResolvedSource == nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrRepairApply, "Codex repair requires resolved source and manifest", nil)
	}
	record := in.Record
	result, err := a.ApplyUpdate(ctx, ports.ApplyInput{
		Plan:           ports.AdapterPlan{TargetID: a.ID(), ActionClass: "repair_drift", EvidenceKey: "target.codex.native_surface"},
		Manifest:       *in.Manifest,
		ResolvedSource: in.ResolvedSource,
		Policy:         record.Policy,
		Inspect:        in.Inspect,
		Record:         &record,
	})
	if err != nil {
		return ports.ApplyResult{}, domain.NewError(domain.ErrRepairApply, "Codex repair refresh failed", err)
	}
	if len(result.ManualSteps) == 0 {
		result.ManualSteps = append(result.ManualSteps, "restart Codex, then use the Plugin Directory to refresh the prepared local plugin and open a new thread")
	}
	return result, nil
}

func (a Adapter) fs() ports.FileSystem {
	if a.FS != nil {
		return a.FS
	}
	return fsadapter.OS{}
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

func normalizedScope(scope string) string {
	if strings.EqualFold(strings.TrimSpace(scope), "project") {
		return "project"
	}
	return "user"
}

func (a Adapter) marketplaceRoot(scope string) string {
	if normalizedScope(scope) == "project" {
		return filepath.Join(a.effectiveProjectRoot(), ".agents", "plugins")
	}
	return filepath.Join(a.userHome(), ".agents", "plugins")
}

func (a Adapter) catalogPath(scope string) string {
	return filepath.Join(a.marketplaceRoot(scope), "marketplace.json")
}

func (a Adapter) pluginRoot(scope, integrationID string) string {
	return filepath.Join(a.marketplaceRoot(scope), "plugins", integrationID)
}

func (a Adapter) cachePath(marketplaceName, integrationID string) string {
	return filepath.Join(a.userHome(), ".codex", "plugins", "cache", marketplaceName, integrationID, "local")
}

func scopeForInspect(in ports.InspectInput) string {
	if in.Record != nil {
		return in.Record.Policy.Scope
	}
	return in.Scope
}

func integrationIDForInspect(record *domain.InstallationRecord) string {
	if record == nil {
		return ""
	}
	return strings.TrimSpace(record.IntegrationID)
}

func protectionForScope(scope string) domain.ProtectionClass {
	if normalizedScope(scope) == "project" {
		return domain.ProtectionWorkspace
	}
	return domain.ProtectionUserMutable
}

func ownedObjects(scope, catalogPath, pluginRoot, pluginName string) []domain.NativeObjectRef {
	return []domain.NativeObjectRef{
		{
			Kind:            "marketplace_catalog",
			Path:            catalogPath,
			ProtectionClass: protectionForScope(scope),
		},
		{
			Kind:            "marketplace_entry",
			Name:            pluginName,
			Path:            catalogPath,
			ProtectionClass: protectionForScope(scope),
		},
		{
			Kind:            "plugin_root",
			Name:            pluginName,
			Path:            pluginRoot,
			ProtectionClass: protectionForScope(scope),
		},
	}
}

func manualInstallSteps(location, pluginName string) []string {
	return []string{
		"open Codex Plugin Directory and install " + pluginName + " from the prepared " + location + " marketplace",
		"after installation, start a new Codex thread before using the plugin",
	}
}

func manualUpdateSteps(pluginName string) []string {
	return []string{
		"restart Codex so it re-reads the updated local marketplace source",
		"refresh or reinstall " + pluginName + " from the Codex Plugin Directory if the installed cache copy is stale",
		"open a new Codex thread before using the refreshed plugin",
	}
}

func manualRemoveSteps(pluginName string) []string {
	return []string{
		"if " + pluginName + " was already installed in Codex, uninstall it from the Codex Plugin Directory",
		"bundled apps stay managed separately in ChatGPT even after the plugin bundle is removed from Codex",
		"restart Codex after removing the plugin bundle",
	}
}

func marketplaceLocationLabel(scope string) string {
	if normalizedScope(scope) == "project" {
		return "project"
	}
	return "personal"
}

func pluginRootFromRecord(record *domain.InstallationRecord) string {
	if record == nil {
		return ""
	}
	target, ok := record.Targets[domain.TargetCodex]
	if !ok {
		return ""
	}
	return pluginRootFromTarget(target, "")
}

func pluginRootFromTarget(target domain.TargetInstallation, fallback string) string {
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "plugin_root" && strings.TrimSpace(item.Path) != "" {
			return item.Path
		}
	}
	return fallback
}

func catalogPathFromTarget(target domain.TargetInstallation, fallback string) string {
	for _, item := range target.OwnedNativeObjects {
		if item.Kind == "marketplace_catalog" && strings.TrimSpace(item.Path) != "" {
			return item.Path
		}
	}
	return fallback
}

func marketplaceNameFromRecord(record *domain.InstallationRecord) string {
	if record == nil {
		return ""
	}
	target, ok := record.Targets[domain.TargetCodex]
	if !ok || target.AdapterMetadata == nil {
		return ""
	}
	value, _ := target.AdapterMetadata["catalog_name"].(string)
	return strings.TrimSpace(value)
}

type marketplaceDoc struct {
	Name      string           `json:"name,omitempty"`
	Interface map[string]any   `json:"interface,omitempty"`
	Plugins   []map[string]any `json:"plugins,omitempty"`
	Extra     map[string]any   `json:"-"`
}

type pluginConfigDoc struct {
	Plugins map[string]pluginConfigEntry `toml:"plugins"`
}

type pluginConfigEntry struct {
	Enabled *bool `toml:"enabled"`
}

type pluginConfigState struct {
	Present  bool
	Disabled bool
}

func (d *marketplaceDoc) UnmarshalJSON(body []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	d.Extra = map[string]any{}
	if value, ok := raw["name"].(string); ok {
		d.Name = strings.TrimSpace(value)
	}
	if value, ok := raw["interface"].(map[string]any); ok {
		d.Interface = value
	}
	if items, ok := raw["plugins"].([]any); ok {
		d.Plugins = make([]map[string]any, 0, len(items))
		for _, item := range items {
			doc, ok := item.(map[string]any)
			if !ok {
				return domain.NewError(domain.ErrMutationApply, "Codex marketplace plugins entries must be JSON objects", nil)
			}
			d.Plugins = append(d.Plugins, doc)
		}
	}
	delete(raw, "name")
	delete(raw, "interface")
	delete(raw, "plugins")
	for key, value := range raw {
		d.Extra[key] = value
	}
	return nil
}

func (d marketplaceDoc) MarshalJSON() ([]byte, error) {
	raw := map[string]any{}
	for key, value := range d.Extra {
		raw[key] = value
	}
	if strings.TrimSpace(d.Name) != "" {
		raw["name"] = d.Name
	}
	if len(d.Interface) > 0 {
		raw["interface"] = d.Interface
	}
	raw["plugins"] = d.Plugins
	return json.Marshal(raw)
}

func readMarketplace(path string) (marketplaceDoc, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return marketplaceDoc{Extra: map[string]any{}, Plugins: []map[string]any{}}, nil
		}
		return marketplaceDoc{}, domain.NewError(domain.ErrMutationApply, "read Codex marketplace catalog", err)
	}
	var doc marketplaceDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return marketplaceDoc{}, domain.NewError(domain.ErrMutationApply, "parse Codex marketplace catalog", err)
	}
	if doc.Extra == nil {
		doc.Extra = map[string]any{}
	}
	if doc.Plugins == nil {
		doc.Plugins = []map[string]any{}
	}
	return doc, nil
}

func writeMarketplace(path string, doc marketplaceDoc) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return domain.NewError(domain.ErrMutationApply, "prepare Codex marketplace dir", err)
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "marshal Codex marketplace catalog", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return domain.NewError(domain.ErrMutationApply, "write Codex marketplace catalog", err)
	}
	return nil
}

func marketplaceEntryDoc(manifest domain.IntegrationManifest, pluginRoot string) map[string]any {
	return map[string]any{
		"name": manifest.IntegrationID,
		"source": map[string]any{
			"source": "local",
			"path":   "./plugins/" + manifest.IntegrationID,
		},
		"policy": map[string]any{
			"installation":   "AVAILABLE",
			"authentication": "ON_INSTALL",
		},
		"category": "Productivity",
	}
}

func mergeMarketplaceEntry(path string, entry map[string]any) (string, error) {
	doc, err := readMarketplace(path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(doc.Name) == "" {
		doc.Name = defaultMarketplaceName(path)
	}
	name, _ := entry["name"].(string)
	if strings.TrimSpace(name) == "" {
		return "", domain.NewError(domain.ErrMutationApply, "Codex marketplace entry is missing plugin name", nil)
	}
	replaced := false
	for i, existing := range doc.Plugins {
		existingName, _ := existing["name"].(string)
		if strings.TrimSpace(existingName) == strings.TrimSpace(name) {
			doc.Plugins[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		doc.Plugins = append(doc.Plugins, entry)
	}
	slices.SortFunc(doc.Plugins, func(a, b map[string]any) int {
		return strings.Compare(pluginName(a), pluginName(b))
	})
	if err := writeMarketplace(path, doc); err != nil {
		return "", err
	}
	return doc.Name, nil
}

func removeMarketplaceEntry(path, pluginName string) error {
	doc, err := readMarketplace(path)
	if err != nil {
		return err
	}
	filtered := make([]map[string]any, 0, len(doc.Plugins))
	for _, item := range doc.Plugins {
		if strings.TrimSpace(pluginNameFromEntry(item)) == strings.TrimSpace(pluginName) {
			continue
		}
		filtered = append(filtered, item)
	}
	doc.Plugins = filtered
	return writeMarketplace(path, doc)
}

func readMarketplaceEntry(path, pluginName string) (map[string]any, bool, error) {
	doc, err := readMarketplace(path)
	if err != nil {
		return nil, false, err
	}
	for _, item := range doc.Plugins {
		if strings.TrimSpace(pluginNameFromEntry(item)) == strings.TrimSpace(pluginName) {
			return item, true, nil
		}
	}
	return nil, false, nil
}

func policyFromEntry(entry map[string]any) *domain.CatalogPolicySnapshot {
	policy, _ := entry["policy"].(map[string]any)
	out := &domain.CatalogPolicySnapshot{}
	if value, ok := policy["installation"].(string); ok {
		out.Installation = strings.TrimSpace(value)
	}
	if value, ok := policy["authentication"].(string); ok {
		out.Authentication = strings.TrimSpace(value)
	}
	if value, ok := entry["category"].(string); ok {
		out.Category = strings.TrimSpace(value)
	}
	if out.Installation == "" && out.Authentication == "" && out.Category == "" {
		return nil
	}
	return out
}

func defaultMarketplaceName(path string) string {
	_ = path
	return "integrationctl-managed"
}

func pluginName(item map[string]any) string {
	return strings.TrimSpace(pluginNameFromEntry(item))
}

func pluginNameFromEntry(item map[string]any) string {
	value, _ := item["name"].(string)
	return strings.TrimSpace(value)
}

func readPluginConfigState(path, pluginRef string) (pluginConfigState, string) {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(pluginRef) == "" {
		return pluginConfigState{}, ""
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return pluginConfigState{}, ""
		}
		return pluginConfigState{}, "read Codex config.toml: " + err.Error()
	}
	var doc pluginConfigDoc
	if err := toml.Unmarshal(body, &doc); err != nil {
		return pluginConfigState{}, "parse Codex config.toml: " + err.Error()
	}
	entry, ok := doc.Plugins[pluginRef]
	if !ok {
		return pluginConfigState{}, ""
	}
	state := pluginConfigState{Present: true}
	if entry.Enabled != nil && !*entry.Enabled {
		state.Disabled = true
	}
	return state, ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
