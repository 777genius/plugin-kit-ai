package app

import "os"

type PluginBundleInstallOptions struct {
	Archive string
	Dest    string
	Force   bool
}

type PluginBundleInstallResult struct {
	Lines []string
}

func (PluginService) BundleInstall(opts PluginBundleInstallOptions) (PluginBundleInstallResult, error) {
	input, err := resolveBundleInstallInput(opts)
	if err != nil {
		return PluginBundleInstallResult{}, err
	}
	metadata, err := loadBundleInstallMetadata(input.archivePath)
	if err != nil {
		return PluginBundleInstallResult{}, err
	}
	installedPath, err := installBundleArchive(input.archivePath, input.dest, input.force)
	if err != nil {
		return PluginBundleInstallResult{}, err
	}
	return buildBundleInstallResult(metadata, input.archivePath, installedPath), nil
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
