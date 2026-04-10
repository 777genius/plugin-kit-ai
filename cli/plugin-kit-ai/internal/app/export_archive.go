package app

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func writeExportArchive(root, output string, files []string, metadata exportMetadata) error {
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	f, err := os.Create(output)
	if err != nil {
		return err
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	gz.Name = ""
	gz.Comment = ""
	gz.ModTime = time.Unix(0, 0)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

	body, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	if err := writeArchiveEntry(tw, ".plugin-kit-ai-export.json", body, 0o644); err != nil {
		return err
	}
	for _, rel := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		mode := int64(info.Mode().Perm())
		if err := writeArchiveEntry(tw, rel, body, mode); err != nil {
			return err
		}
	}
	return nil
}

func writeArchiveEntry(tw *tar.Writer, rel string, body []byte, mode int64) error {
	name := filepath.ToSlash(filepath.Clean(rel))
	if strings.HasPrefix(name, "../") || name == ".." || filepath.IsAbs(name) {
		return fmt.Errorf("invalid archive path %s", rel)
	}
	hdr := &tar.Header{
		Name:     name,
		Mode:     mode,
		Size:     int64(len(body)),
		ModTime:  time.Unix(0, 0),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(body)
	return err
}
