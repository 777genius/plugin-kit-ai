package codex

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (a Adapter) Inspect(_ context.Context, in ports.InspectInput) (ports.InspectResult, error) {
	scope := scopeForInspect(in)
	workspaceRoot := workspaceRootFromInspectInput(in)
	paths := a.pathsForScope(scope, workspaceRoot, integrationIDForInspect(in.Record))
	if in.Record != nil {
		paths = a.pathsForRecord(*in.Record)
	}

	_, cmdErr := exec.LookPath("codex")
	catalogInfo, catalogErr := os.Stat(paths.CatalogPath)
	pluginInfo, pluginErr := os.Stat(paths.PluginRoot)
	configInfo, configErr := os.Stat(paths.ConfigPath)

	restrictions := []domain.EnvironmentRestrictionCode{}
	if cmdErr != nil {
		restrictions = append(restrictions, domain.RestrictionSourceToolMissing)
	}

	observed := []domain.NativeObjectRef{}
	if catalogErr == nil && !catalogInfo.IsDir() {
		observed = append(observed, domain.NativeObjectRef{
			Kind:            "marketplace_catalog",
			Path:            paths.CatalogPath,
			ProtectionClass: protectionForScope(scope),
		})
	}
	if pluginErr == nil && pluginInfo.IsDir() {
		observed = append(observed, domain.NativeObjectRef{
			Kind:            "plugin_root",
			Path:            paths.PluginRoot,
			ProtectionClass: protectionForScope(scope),
		})
	}
	if configErr == nil && !configInfo.IsDir() {
		observed = append(observed, domain.NativeObjectRef{
			Kind:            "config_file",
			Path:            paths.ConfigPath,
			ProtectionClass: domain.ProtectionUserMutable,
		})
	}

	var catalogPolicy *domain.CatalogPolicySnapshot
	warnings := []string{}
	entryFound := false
	if entry, found, err := readMarketplaceEntry(paths.CatalogPath, integrationIDForInspect(in.Record)); err == nil && found {
		entryFound = true
		catalogPolicy = policyFromEntry(entry)
	} else if err != nil {
		warnings = append(warnings, err.Error())
	}

	marketplaceName := marketplaceNameFromRecord(in.Record)
	if strings.TrimSpace(marketplaceName) == "" {
		if doc, err := readMarketplace(paths.CatalogPath); err == nil {
			marketplaceName = strings.TrimSpace(doc.Name)
		}
	}

	cacheExists := false
	if id := integrationIDForInspect(in.Record); id != "" && marketplaceName != "" {
		cachePath := a.cachePath(marketplaceName, id)
		if cacheInfo, cacheErr := os.Stat(cachePath); cacheErr == nil && cacheInfo != nil && cacheInfo.IsDir() {
			cacheExists = true
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
	configState, configWarning := readPluginConfigState(paths.ConfigPath, pluginRef)
	if configWarning != "" {
		warnings = append(warnings, configWarning)
	}
	if configState.Present {
		observed = append(observed, domain.NativeObjectRef{
			Kind:            "plugin_toggle",
			Name:            pluginRef,
			Path:            paths.ConfigPath,
			ProtectionClass: domain.ProtectionUserMutable,
		})
	}

	pluginExists := pluginErr == nil && pluginInfo != nil && pluginInfo.IsDir()
	preparedExists := entryFound && pluginExists
	partialPrepared := entryFound || pluginExists
	state := domain.InstallRemoved
	activation := domain.ActivationNotRequired
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
	case preparedExists:
		state = domain.InstallActivationPending
		activation = domain.ActivationNativePending
		restrictions = append(restrictions, domain.RestrictionNativeActivation, domain.RestrictionNewThreadRequired)
	case partialPrepared:
		state = domain.InstallDegraded
		activation = domain.ActivationNativePending
		restrictions = append(restrictions, domain.RestrictionNativeActivation)
	}

	return ports.InspectResult{
		TargetID:                a.ID(),
		Installed:               cacheExists || preparedExists || configState.Present,
		State:                   state,
		ActivationState:         activation,
		CatalogPolicy:           catalogPolicy,
		ConfigPrecedenceContext: []string{"repo_marketplace", "personal_marketplace", "cache", "config"},
		EnvironmentRestrictions: restrictions,
		ObservedNativeObjects:   observed,
		SettingsFiles:           []string{paths.CatalogPath, paths.ConfigPath},
		Warnings:                warnings,
		EvidenceClass:           domain.EvidenceConfirmed,
	}, nil
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
