package app

import (
	"fmt"
	"os"
	"strings"
)

type PluginBundleInstallOptions struct {
	Archive string
	Dest    string
	Force   bool
}

type PluginBundleInstallResult struct {
	Lines []string
}

func (PluginService) BundleInstall(opts PluginBundleInstallOptions) (PluginBundleInstallResult, error) {
	archivePath := strings.TrimSpace(opts.Archive)
	if archivePath == "" {
		return PluginBundleInstallResult{}, fmt.Errorf("bundle install requires a local .tar.gz bundle path")
	}
	lowerArchivePath := strings.ToLower(archivePath)
	if strings.HasPrefix(lowerArchivePath, "http://") || strings.HasPrefix(lowerArchivePath, "https://") {
		return PluginBundleInstallResult{}, fmt.Errorf("bundle install supports local .tar.gz bundles only; remote URLs are out of scope")
	}
	if !strings.HasSuffix(lowerArchivePath, ".tar.gz") {
		return PluginBundleInstallResult{}, fmt.Errorf("bundle install requires a local .tar.gz bundle path")
	}
	dest := strings.TrimSpace(opts.Dest)
	if dest == "" {
		return PluginBundleInstallResult{}, fmt.Errorf("bundle install requires --dest")
	}

	metadata, err := inspectBundleArchive(archivePath)
	if err != nil {
		return PluginBundleInstallResult{}, err
	}
	if err := validateBundleMetadata(metadata); err != nil {
		return PluginBundleInstallResult{}, err
	}

	installedPath, err := installBundleArchive(archivePath, dest, opts.Force)
	if err != nil {
		return PluginBundleInstallResult{}, err
	}

	lines := []string{
		fmt.Sprintf("Bundle: plugin=%s platform=%s runtime=%s manager=%s", metadata.PluginName, metadata.Platform, metadata.Runtime, displayBundleManager(metadata.Manager)),
		"Bundle source: " + archivePath,
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
	return PluginBundleInstallResult{Lines: lines}, nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func pathEmpty(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func resolvedBundleNext(metadata exportMetadata, dest string) []string {
	if len(metadata.Next) == 0 {
		return canonicalBundleNext(metadata.Platform, dest)
	}
	out := make([]string, 0, len(metadata.Next))
	for _, step := range metadata.Next {
		step = strings.TrimSpace(step)
		if step == "" {
			continue
		}
		out = append(out, strings.Replace(step, " .", " "+dest, 1))
	}
	if len(out) == 0 {
		return canonicalBundleNext(metadata.Platform, dest)
	}
	return out
}

func canonicalBundleNext(platform, dest string) []string {
	return []string{
		"plugin-kit-ai doctor " + dest,
		"plugin-kit-ai bootstrap " + dest,
		fmt.Sprintf("plugin-kit-ai validate %s --platform %s --strict", dest, platform),
	}
}

func displayBundleManager(manager string) string {
	manager = strings.TrimSpace(manager)
	if manager == "" {
		return "none"
	}
	return manager
}
