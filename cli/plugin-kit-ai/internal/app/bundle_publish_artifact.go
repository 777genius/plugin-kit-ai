package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

type bundlePublishArtifact struct {
	Metadata    exportMetadata
	Body        []byte
	BundleName  string
	SidecarName string
	SidecarBody []byte
}

func prepareBundlePublishArtifact(root, platform string, deps bundlePublishDeps) (bundlePublishArtifact, error) {
	tmpFile, err := os.CreateTemp("", ".plugin-kit-ai-publish-*.tar.gz")
	if err != nil {
		return bundlePublishArtifact{}, err
	}
	exportPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(exportPath)

	if _, err := deps.Export(PluginExportOptions{
		Root:     root,
		Platform: platform,
		Output:   exportPath,
	}); err != nil {
		return bundlePublishArtifact{}, err
	}

	metadata, err := inspectBundleArchive(exportPath)
	if err != nil {
		return bundlePublishArtifact{}, err
	}
	if err := validateBundleMetadata(metadata); err != nil {
		return bundlePublishArtifact{}, err
	}

	body, err := os.ReadFile(exportPath)
	if err != nil {
		return bundlePublishArtifact{}, err
	}
	bundleName := fmt.Sprintf("%s_%s_%s_bundle.tar.gz", metadata.PluginName, metadata.Platform, metadata.Runtime)
	sum := sha256.Sum256(body)
	sidecarBody := []byte(hex.EncodeToString(sum[:]) + "  " + bundleName + "\n")
	return bundlePublishArtifact{
		Metadata:    metadata,
		Body:        body,
		BundleName:  bundleName,
		SidecarName: bundleName + ".sha256",
		SidecarBody: sidecarBody,
	}, nil
}
