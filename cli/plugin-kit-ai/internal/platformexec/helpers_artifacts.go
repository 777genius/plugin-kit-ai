package platformexec

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
)

type artifactDir struct {
	src string
	dst string
}

func copyArtifactDirs(root string, dirs ...artifactDir) ([]pluginmodel.Artifact, error) {
	var artifacts []pluginmodel.Artifact
	for _, dir := range dirs {
		copied, err := copyArtifacts(root, dir.src, dir.dst)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, copied...)
	}
	return artifacts, nil
}

func copyArtifacts(root, srcDir, dstRoot string) ([]pluginmodel.Artifact, error) {
	full := filepath.Join(root, srcDir)
	var artifacts []pluginmodel.Artifact
	if _, err := os.Stat(full); err != nil {
		return nil, nil
	}
	err := filepath.WalkDir(full, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(full, path)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.ToSlash(filepath.Join(dstRoot, rel)),
			Content: body,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(artifacts, func(a, b pluginmodel.Artifact) int { return strings.Compare(a.RelPath, b.RelPath) })
	return artifacts, nil
}

func copySingleArtifactIfExists(root, srcRel, dstRel string) ([]pluginmodel.Artifact, error) {
	if strings.TrimSpace(srcRel) == "" {
		return nil, nil
	}
	body, err := os.ReadFile(filepath.Join(root, srcRel))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return []pluginmodel.Artifact{{RelPath: filepath.ToSlash(dstRel), Content: body}}, nil
}

func authoredComponentDir(state pluginmodel.TargetState, kind, fallback string) string {
	paths := state.ComponentPaths(kind)
	if len(paths) == 0 {
		return filepath.ToSlash(fallback)
	}
	dir := filepath.ToSlash(filepath.Dir(paths[0]))
	if dir == "." {
		return filepath.ToSlash(fallback)
	}
	return dir
}

func compactArtifacts(artifacts []pluginmodel.Artifact) []pluginmodel.Artifact {
	slices.SortFunc(artifacts, func(a, b pluginmodel.Artifact) int { return strings.Compare(a.RelPath, b.RelPath) })
	out := make([]pluginmodel.Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		n := len(out)
		if n > 0 && out[n-1].RelPath == artifact.RelPath {
			out[n-1] = artifact
			continue
		}
		out = append(out, artifact)
	}
	return out
}

func loadNativeExtraDoc(root string, state pluginmodel.TargetState, kind string, format pluginmodel.NativeDocFormat) (pluginmodel.NativeExtraDoc, error) {
	return pluginmodel.LoadNativeExtraDoc(root, state.DocPath(kind), format)
}

func managedKeysForNativeDoc(target, kind string) []string {
	profile, ok := platformmeta.Lookup(target)
	if !ok {
		return nil
	}
	for _, doc := range profile.NativeDocs {
		if doc.Kind != kind {
			continue
		}
		if len(doc.ManagedKeys) == 0 {
			return nil
		}
		return append([]string(nil), doc.ManagedKeys...)
	}
	return nil
}

func renderPortableMCPForTarget(mcp *pluginmodel.PortableMCP, target string) (map[string]any, error) {
	if mcp == nil {
		return nil, nil
	}
	return mcp.RenderForTarget(target)
}

func importedPortableMCPArtifact(sourceTarget string, servers map[string]any) (pluginmodel.Artifact, error) {
	body, err := pluginmodel.ImportedPortableMCPYAML(sourceTarget, servers)
	if err != nil {
		return pluginmodel.Artifact{}, err
	}
	return pluginmodel.Artifact{
		RelPath: filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, "mcp", "servers.yaml")),
		Content: body,
	}, nil
}

func discoverFiles(root, dir string, allow func(string) bool) []string {
	full := filepath.Join(root, dir)
	var out []string
	filepath.WalkDir(full, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if allow != nil && !allow(rel) {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	slices.Sort(out)
	return out
}

func cleanRelativeRef(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}

func resolveRelativeRef(root, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", nil
	}
	if filepath.IsAbs(ref) {
		return "", fmt.Errorf("ref %q must stay within the plugin root", ref)
	}
	cleaned := filepath.Clean(ref)
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("ref %q must stay within the plugin root", ref)
	}
	cleaned = strings.TrimPrefix(cleaned, "."+string(filepath.Separator))
	cleaned = filepath.ToSlash(cleaned)
	if cleaned == "" || cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("ref %q must stay within the plugin root", ref)
	}
	return cleaned, nil
}

func copyArtifactsFromRefs(root string, refs []string, dstRoot string) ([]pluginmodel.Artifact, error) {
	var artifacts []pluginmodel.Artifact
	for _, ref := range refs {
		var err error
		ref, err = resolveRelativeRef(root, ref)
		if err != nil {
			return nil, err
		}
		if ref == "" {
			continue
		}
		full := filepath.Join(root, ref)
		info, err := os.Stat(full)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			copied, err := copyArtifacts(root, ref, dstRoot)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, copied...)
			continue
		}
		body, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.ToSlash(filepath.Join(dstRoot, filepath.Base(ref))),
			Content: body,
		})
	}
	return compactArtifacts(artifacts), nil
}
