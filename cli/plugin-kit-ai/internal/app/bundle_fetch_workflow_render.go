package app

import (
	"fmt"
	"strings"
)

func buildBundleFetchResult(metadata exportMetadata, source bundleRemoteSource, installedPath string) PluginBundleFetchResult {
	lines := []string{
		fmt.Sprintf("Bundle: plugin=%s platform=%s runtime=%s manager=%s", metadata.PluginName, metadata.Platform, metadata.Runtime, displayBundleManager(metadata.Manager)),
		"Bundle source: " + source.BundleSource,
		"Checksum source: " + source.ChecksumSource,
		"Installed path: " + installedPath,
	}
	if strings.TrimSpace(metadata.RuntimeRequirement) != "" {
		lines = append(lines, "Runtime requirement: "+metadata.RuntimeRequirement)
	}
	if strings.TrimSpace(metadata.RuntimeInstallHint) != "" {
		lines = append(lines, "Runtime install hint: "+metadata.RuntimeInstallHint)
	}
	lines = append(lines, "Next:")
	for _, step := range resolvedBundleNext(metadata, installedPath) {
		lines = append(lines, "  "+step)
	}
	return PluginBundleFetchResult{Lines: lines}
}
