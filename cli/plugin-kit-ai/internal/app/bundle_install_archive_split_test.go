package app

import (
	"archive/tar"
	"strings"
	"testing"
)

func TestDecodeBundleArchiveMetadataRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := decodeBundleArchiveMetadata([]byte("{"))
	if err == nil || !strings.Contains(err.Error(), "valid .plugin-kit-ai-export.json") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateBundleHeaderTypeRejectsSymlink(t *testing.T) {
	t.Parallel()

	err := validateBundleHeaderType(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %v", err)
	}
}
