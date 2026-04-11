package app

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
)

func readBundleArchiveMetadataEntry(tr *tar.Reader) (exportMetadata, bool, error) {
	hdr, err := tr.Next()
	if err == io.EOF {
		return exportMetadata{}, true, fmt.Errorf("bundle install requires .plugin-kit-ai-export.json in archive root")
	}
	if err != nil {
		return exportMetadata{}, true, err
	}
	name, err := validateBundleHeader(hdr)
	if err != nil {
		return exportMetadata{}, true, err
	}
	if hdr.Typeflag == tar.TypeDir || name != ".plugin-kit-ai-export.json" {
		return exportMetadata{}, false, nil
	}
	body, err := io.ReadAll(tr)
	if err != nil {
		return exportMetadata{}, true, err
	}
	metadata, err := decodeBundleArchiveMetadata(body)
	return metadata, true, err
}

func decodeBundleArchiveMetadata(body []byte) (exportMetadata, error) {
	var metadata exportMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return exportMetadata{}, fmt.Errorf("bundle install requires valid .plugin-kit-ai-export.json: %w", err)
	}
	return metadata, nil
}
