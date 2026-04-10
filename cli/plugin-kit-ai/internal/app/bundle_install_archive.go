package app

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func inspectBundleArchive(archivePath string) (exportMetadata, error) {
	tr, closer, err := openBundleArchive(archivePath)
	if err != nil {
		return exportMetadata{}, err
	}
	defer closer()

	var metadata exportMetadata
	foundMetadata := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return exportMetadata{}, err
		}
		name, err := validateBundleHeader(hdr)
		if err != nil {
			return exportMetadata{}, err
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			return exportMetadata{}, err
		}
		if name != ".plugin-kit-ai-export.json" {
			continue
		}
		if err := json.Unmarshal(body, &metadata); err != nil {
			return exportMetadata{}, fmt.Errorf("bundle install requires valid .plugin-kit-ai-export.json: %w", err)
		}
		foundMetadata = true
	}
	if !foundMetadata {
		return exportMetadata{}, fmt.Errorf("bundle install requires .plugin-kit-ai-export.json in archive root")
	}
	return metadata, nil
}

func openBundleArchive(path string) (*tar.Reader, func() error, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	closeFn := func() error {
		err1 := gz.Close()
		err2 := f.Close()
		if err1 != nil {
			return err1
		}
		return err2
	}
	return tar.NewReader(gz), closeFn, nil
}

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
	switch hdr.Typeflag {
	case tar.TypeReg, tar.TypeDir:
		return name, nil
	case tar.TypeSymlink, tar.TypeLink:
		return "", fmt.Errorf("bundle install refuses symlink entry %s", hdr.Name)
	default:
		return "", fmt.Errorf("bundle install refuses unsupported archive entry %s", hdr.Name)
	}
}
