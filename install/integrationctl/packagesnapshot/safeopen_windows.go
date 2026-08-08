//go:build windows

package packagesnapshot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func openSnapshotRegular(root, rel string) (*os.File, error) {
	parts := strings.Split(rel, "/")
	ancestors := make([]os.FileInfo, 0, len(parts))
	current := root
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("unsafe snapshot ancestor %q", part)
		}
		ancestors = append(ancestors, info)
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	for index, part := range parts[:len(parts)-1] {
		checkPath := filepath.Join(append([]string{root}, parts[:index+1]...)...)
		info, statErr := os.Lstat(checkPath)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, ancestors[index]) {
			_ = file.Close()
			return nil, fmt.Errorf("snapshot ancestor %q changed while opening", part)
		}
	}
	return file, nil
}
