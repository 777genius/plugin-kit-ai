package app

import (
	"io"
)

func inspectBundleArchive(archivePath string) (exportMetadata, error) {
	tr, closer, err := openBundleArchive(archivePath)
	if err != nil {
		return exportMetadata{}, err
	}
	defer closer()

	for {
		metadata, done, err := readBundleArchiveMetadataEntry(tr)
		if done || err != nil {
			return metadata, err
		}
		if _, err := io.Copy(io.Discard, tr); err != nil {
			return exportMetadata{}, err
		}
	}
}
