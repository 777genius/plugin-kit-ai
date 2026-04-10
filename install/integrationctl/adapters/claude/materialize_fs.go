package claude

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func copyNativeClaudePackage(sourceRoot, destRoot string) error {
	for _, path := range []string{
		filepath.Join(sourceRoot, ".claude-plugin"),
		filepath.Join(sourceRoot, ".mcp.json"),
		filepath.Join(sourceRoot, "settings.json"),
		filepath.Join(sourceRoot, ".lsp.json"),
		filepath.Join(sourceRoot, "hooks"),
		filepath.Join(sourceRoot, "skills"),
		filepath.Join(sourceRoot, "commands"),
		filepath.Join(sourceRoot, "agents"),
	} {
		if _, err := copyPathIfExists(path, filepath.Join(destRoot, filepath.Base(path))); err != nil {
			return domain.NewError(domain.ErrMutationApply, "copy native Claude package", err)
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
		return true, filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			target := filepath.Join(dest, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			return copyFile(path, target)
		})
	}
	return true, copyFile(src, dest)
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

func marshalJSON(value any) ([]byte, error) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
