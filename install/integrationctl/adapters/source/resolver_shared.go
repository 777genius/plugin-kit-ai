package source

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func cleanupRoot(localPath, subdir string) string {
	if strings.TrimSpace(subdir) == "" {
		return localPath
	}
	root := localPath
	for range strings.Split(strings.Trim(subdir, "/"), "/") {
		root = filepath.Dir(root)
	}
	return root
}

func hashLocalTree(root string) (string, error) {
	hasher := sha256.New()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, _ = hasher.Write([]byte(filepath.ToSlash(rel)))
		_, _ = hasher.Write([]byte{0})
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, _ = hasher.Write(data)
		_, _ = hasher.Write([]byte{0})
		return nil
	})
	if err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func isCommandNotFound(err error) bool {
	return errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist)
}

func commandOutput(result ports.CommandResult) string {
	if text := strings.TrimSpace(string(result.Stderr)); text != "" {
		return text
	}
	return strings.TrimSpace(string(result.Stdout))
}
