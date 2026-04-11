package app

import (
	"archive/tar"
	"bytes"
	"strings"
	"testing"
)

func TestValidateBundleMetadataEnvelopeRejectsMissingPluginName(t *testing.T) {
	t.Parallel()

	err := validateBundleMetadataEnvelope(exportMetadata{
		BundleFormat: "tar.gz",
		GeneratedBy:  "plugin-kit-ai export",
	})
	if err == nil || !strings.Contains(err.Error(), "metadata.plugin_name") {
		t.Fatalf("error = %v", err)
	}
}

func TestExtractBundleArchiveEntryRejectsUnsupportedType(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tr := tar.NewReader(&buf)
	err := extractBundleArchiveEntry(tr, t.TempDir(), &tar.Header{Name: "demo", Typeflag: tar.TypeSymlink})
	if err == nil || !strings.Contains(err.Error(), "symlink entry") {
		t.Fatalf("error = %v", err)
	}
}
