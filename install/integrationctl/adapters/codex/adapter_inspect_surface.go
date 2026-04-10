package codex

import (
	"os"
	"os/exec"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

type codexObservedSurface struct {
	Restrictions []domain.EnvironmentRestrictionCode
	Observed     []domain.NativeObjectRef
	CatalogFound bool
	PluginFound  bool
}

func observeCodexSurface(scope string, paths codexSurfacePaths) codexObservedSurface {
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

	return codexObservedSurface{
		Restrictions: restrictions,
		Observed:     observed,
		CatalogFound: catalogErr == nil && catalogInfo != nil && !catalogInfo.IsDir(),
		PluginFound:  pluginErr == nil && pluginInfo != nil && pluginInfo.IsDir(),
	}
}

type codexInspectDetails struct {
	CatalogPolicy *domain.CatalogPolicySnapshot
	Warnings      []string
	Observed      []domain.NativeObjectRef
	CacheExists   bool
	ConfigState   pluginConfigState
	EntryFound    bool
}

func (a Adapter) inspectDetails(inputs codexInspectInputs) codexInspectDetails {
	warnings := []string{}
	catalogPolicy := (*domain.CatalogPolicySnapshot)(nil)
	entryFound := false
	if entry, found, err := readMarketplaceEntry(inputs.Paths.CatalogPath, inputs.IntegrationID); err == nil && found {
		entryFound = true
		catalogPolicy = policyFromEntry(entry)
	} else if err != nil {
		warnings = append(warnings, err.Error())
	}

	marketplaceName := marketplaceNameFromRecord(inputs.Record)
	if strings.TrimSpace(marketplaceName) == "" {
		if doc, err := readMarketplace(inputs.Paths.CatalogPath); err == nil {
			marketplaceName = strings.TrimSpace(doc.Name)
		}
	}

	observed := []domain.NativeObjectRef{}
	cacheExists := false
	if inputs.IntegrationID != "" && marketplaceName != "" {
		cachePath := a.cachePath(marketplaceName, inputs.IntegrationID)
		if cacheInfo, cacheErr := os.Stat(cachePath); cacheErr == nil && cacheInfo != nil && cacheInfo.IsDir() {
			cacheExists = true
			observed = append(observed, domain.NativeObjectRef{
				Kind:            "installed_cache_bundle",
				Name:            inputs.IntegrationID,
				Path:            cachePath,
				ProtectionClass: domain.ProtectionUserMutable,
			})
		}
	}

	pluginRef := ""
	if inputs.IntegrationID != "" && marketplaceName != "" {
		pluginRef = inputs.IntegrationID + "@" + marketplaceName
	}
	configState, configWarning := readPluginConfigState(inputs.Paths.ConfigPath, pluginRef)
	if configWarning != "" {
		warnings = append(warnings, configWarning)
	}
	if configState.Present {
		observed = append(observed, domain.NativeObjectRef{
			Kind:            "plugin_toggle",
			Name:            pluginRef,
			Path:            inputs.Paths.ConfigPath,
			ProtectionClass: domain.ProtectionUserMutable,
		})
	}

	return codexInspectDetails{
		CatalogPolicy: catalogPolicy,
		Warnings:      append([]string{}, warnings...),
		Observed:      observed,
		CacheExists:   cacheExists,
		ConfigState:   configState,
		EntryFound:    entryFound,
	}
}
