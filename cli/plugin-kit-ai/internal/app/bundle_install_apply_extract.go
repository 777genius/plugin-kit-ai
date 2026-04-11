package app

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func extractBundleArchiveToDir(archivePath, dest string) error {
	tr, closer, err := openBundleArchive(archivePath)
	if err != nil {
		return err
	}
	defer closer()
	return extractBundleArchive(tr, dest)
}

func extractBundleArchiveEntry(tr *tar.Reader, dest string, hdr *tar.Header) error {
	name, err := validateBundleHeader(hdr)
	if err != nil {
		return err
	}
	target := filepath.Join(dest, filepath.FromSlash(name))
	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, os.FileMode(hdr.Mode))
	case tar.TypeReg:
		return writeBundleArchiveFile(tr, target, hdr.Mode)
	default:
		return fmt.Errorf("bundle install refuses unsupported archive entry %s", name)
	}
}

func writeBundleArchiveFile(tr *tar.Reader, target string, mode int64) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	body, err := io.ReadAll(tr)
	if err != nil {
		return err
	}
	return os.WriteFile(target, body, os.FileMode(mode))
}
