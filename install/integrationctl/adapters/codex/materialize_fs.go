package codex

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func copyNativeCodexPackage(sourceRoot, destRoot string) error {
	for _, path := range []string{
		filepath.Join(sourceRoot, ".codex-plugin"),
		filepath.Join(sourceRoot, ".mcp.json"),
		filepath.Join(sourceRoot, ".app.json"),
		filepath.Join(sourceRoot, "skills"),
		filepath.Join(sourceRoot, "assets"),
	} {
		if _, err := copyPathIfExists(path, filepath.Join(destRoot, filepath.Base(path))); err != nil {
			return domain.NewError(domain.ErrMutationApply, "copy native Codex package", err)
		}
	}
	return nil
}

func copyPathIfExists(src, dest string) (bool, error) {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		if err := copyDir(src, dest); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := copyFile(src, dest); err != nil {
		return false, err
	}
	return true, nil
}

func copyDir(src, dest string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dest string) error {
	body, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, body, 0o644)
}
