//go:build windows

package nativeconfig

import (
	"fmt"
	"os"
)

func openNoFollow(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("native config symlink is not allowed")
	}
	return os.Open(path)
}
