package cursor

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/pathpolicy"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

const (
	cursorPluginManifestRelPath = ".cursor-plugin/plugin.json"
	cursorPluginRootKind        = "cursor_plugin_root"
)

func (a Adapter) targetPluginRoot(integrationID string) string {
	return filepath.Join(a.userHome(), ".cursor", "plugins", "local", integrationID)
}

func shouldUsePluginPackage(manifest domain.IntegrationManifest, sourceRoot string) bool {
	if cursorPluginPackageExists(sourceRoot) {
		return true
	}
	for _, delivery := range manifest.Deliveries {
		if delivery.TargetID == domain.TargetCursor && delivery.DeliveryKind == domain.DeliveryCursorPlugin {
			return true
		}
	}
	return false
}

func cursorPluginPackageExists(sourceRoot string) bool {
	if strings.TrimSpace(sourceRoot) == "" {
		return false
	}
	return pathpolicy.FileExists(filepath.Join(sourceRoot, cursorPluginManifestRelPath))
}

func (a Adapter) syncManagedPlugin(ctx context.Context, manifest domain.IntegrationManifest, sourceRoot, pluginRoot string) error {
	_ = ctx
	_ = manifest
	root := filepath.Clean(sourceRoot)
	if !cursorPluginPackageExists(root) {
		return domain.NewError(domain.ErrMutationApply, "Cursor plugin package requires .cursor-plugin/plugin.json", nil)
	}
	parent := filepath.Dir(pluginRoot)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return domain.NewError(domain.ErrMutationApply, "prepare Cursor plugin parent", err)
	}
	tmpRoot, err := os.MkdirTemp(parent, filepath.Base(pluginRoot)+".tmp-*")
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "create Cursor materialization temp root", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmpRoot)
		}
	}()
	if err := copyNativeCursorPackage(root, tmpRoot); err != nil {
		return err
	}
	if err := copyCursorManifestRefs(root, tmpRoot); err != nil {
		return err
	}
	if !cursorPluginPackageExists(tmpRoot) {
		return domain.NewError(domain.ErrMutationApply, "Cursor package copy did not produce .cursor-plugin/plugin.json", nil)
	}
	if err := os.RemoveAll(pluginRoot); err != nil && !os.IsNotExist(err) {
		return domain.NewError(domain.ErrMutationApply, "replace Cursor managed plugin root", err)
	}
	if err := os.Rename(tmpRoot, pluginRoot); err != nil {
		return domain.NewError(domain.ErrMutationApply, "activate Cursor managed plugin root", err)
	}
	cleanup = false
	return nil
}

func copyCursorManifestRefs(sourceRoot, destRoot string) error {
	body, err := os.ReadFile(filepath.Join(sourceRoot, cursorPluginManifestRelPath))
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "read Cursor plugin manifest", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(body, &manifest); err != nil {
		return domain.NewError(domain.ErrMutationApply, "parse Cursor plugin manifest", err)
	}
	keys := []string{"logo", "commands", "agents", "skills", "rules", "hooks", "mcpServers"}
	for _, key := range keys {
		for _, ref := range cursorManifestStringRefs(manifest[key]) {
			if err := copyCursorRelativeRef(sourceRoot, destRoot, ref); err != nil {
				return err
			}
		}
	}
	return nil
}

func cursorManifestStringRefs(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if ref, ok := item.(string); ok {
				out = append(out, ref)
			}
		}
		return out
	default:
		return nil
	}
}

func copyCursorRelativeRef(sourceRoot, destRoot, ref string) error {
	rel, ok, err := safeCursorRelativeRef(ref)
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "validate Cursor manifest path", err)
	}
	if !ok {
		return nil
	}
	if strings.ContainsAny(rel, "*?[") {
		matches, err := filepath.Glob(filepath.Join(sourceRoot, rel))
		if err != nil {
			return domain.NewError(domain.ErrMutationApply, "expand Cursor manifest glob", err)
		}
		if len(matches) == 0 {
			return domain.NewError(domain.ErrMutationApply, fmt.Sprintf("Cursor manifest glob %q matched no files", ref), nil)
		}
		for _, match := range matches {
			if err := copyCursorMatchedPath(sourceRoot, destRoot, match); err != nil {
				return err
			}
		}
		return nil
	}
	copied, err := copyPathIfExists(filepath.Join(sourceRoot, rel), filepath.Join(destRoot, rel))
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "copy Cursor manifest path", err)
	}
	if !copied {
		return domain.NewError(domain.ErrMutationApply, fmt.Sprintf("Cursor manifest path %q does not exist", ref), nil)
	}
	return nil
}

func copyCursorMatchedPath(sourceRoot, destRoot, match string) error {
	rel, err := filepath.Rel(sourceRoot, match)
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "resolve Cursor manifest glob match", err)
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return domain.NewError(domain.ErrMutationApply, "Cursor manifest glob escaped plugin root", nil)
	}
	if _, err := copyPathIfExists(match, filepath.Join(destRoot, rel)); err != nil {
		return domain.NewError(domain.ErrMutationApply, "copy Cursor manifest glob match", err)
	}
	return nil
}

func safeCursorRelativeRef(ref string) (string, bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.Contains(ref, "://") || strings.HasPrefix(ref, "data:") || filepath.IsAbs(ref) {
		return "", false, nil
	}
	ref = strings.TrimPrefix(ref, "./")
	rel := filepath.Clean(filepath.FromSlash(ref))
	if rel == "." {
		return "", false, nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false, fmt.Errorf("path %q escapes plugin root", ref)
	}
	return rel, true, nil
}

func copyNativeCursorPackage(sourceRoot, destRoot string) error {
	for _, rel := range []string{
		".cursor-plugin",
		".mcp.json",
		"mcp.json",
		"skills",
		"rules",
		"agents",
		"commands",
		"hooks",
		"assets",
	} {
		if _, err := copyPathIfExists(filepath.Join(sourceRoot, rel), filepath.Join(destRoot, rel)); err != nil {
			return domain.NewError(domain.ErrMutationApply, "copy native Cursor package", err)
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
