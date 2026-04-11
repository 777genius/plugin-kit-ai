package app

import (
	"archive/tar"
	"fmt"
	"path/filepath"
	"strings"
)

func validateBundleHeader(hdr *tar.Header) (string, error) {
	name := normalizeExportPath(hdr.Name)
	if name == "" {
		return "", fmt.Errorf("bundle install refuses invalid archive path %q", hdr.Name)
	}
	if strings.HasPrefix(name, "../") || strings.Contains(name, "/../") {
		return "", fmt.Errorf("bundle install refuses path traversal entry %s", hdr.Name)
	}
	if filepath.IsAbs(hdr.Name) {
		return "", fmt.Errorf("bundle install refuses absolute archive path %s", hdr.Name)
	}
	return name, validateBundleHeaderType(hdr)
}

func validateBundleHeaderType(hdr *tar.Header) error {
	switch hdr.Typeflag {
	case tar.TypeReg, tar.TypeDir:
		return nil
	case tar.TypeSymlink, tar.TypeLink:
		return fmt.Errorf("bundle install refuses symlink entry %s", hdr.Name)
	default:
		return fmt.Errorf("bundle install refuses unsupported archive entry %s", hdr.Name)
	}
}
