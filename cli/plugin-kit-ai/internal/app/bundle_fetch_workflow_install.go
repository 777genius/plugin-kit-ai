package app

func installFetchedBundleSource(source bundleRemoteSource, opts PluginBundleFetchOptions) (exportMetadata, string, error) {
	archivePath, cleanup, err := writeTempBundleArchive(source.ArchiveBytes)
	if err != nil {
		return exportMetadata{}, "", err
	}
	defer cleanup()

	metadata, err := inspectBundleArchive(archivePath)
	if err != nil {
		return exportMetadata{}, "", err
	}
	if err := validateBundleMetadata(metadata); err != nil {
		return exportMetadata{}, "", err
	}
	if err := validateFetchedBundleMatchesRequest(metadata, opts); err != nil {
		return exportMetadata{}, "", err
	}

	installedPath, err := installBundleArchive(archivePath, opts.Dest, opts.Force)
	if err != nil {
		return exportMetadata{}, "", err
	}
	return metadata, installedPath, nil
}
