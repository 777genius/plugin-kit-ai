package nativeconfig

import (
	"fmt"
	"io"
	"os"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/atomicfile"
)

type osFiles struct{}

func (osFiles) ReadNoFollow(path string) ([]byte, os.FileMode, bool, error) {
	file, err := openNoFollow(path)
	if os.IsNotExist(err) {
		return nil, 0o600, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("open native config without following links: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, 0, false, fmt.Errorf("inspect native config: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, false, fmt.Errorf("native config must be a regular file")
	}
	body, err := io.ReadAll(file)
	if err != nil {
		return nil, 0, false, fmt.Errorf("read native config: %w", err)
	}
	return body, info.Mode().Perm(), true, nil
}

func (osFiles) WriteAtomic(path string, body []byte, mode os.FileMode) error {
	return atomicfile.Write(path, body, mode)
}

func (osFiles) RemoveNoFollow(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect native config before remove: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("native config remove target must be a regular file")
	}
	return os.Remove(path)
}
