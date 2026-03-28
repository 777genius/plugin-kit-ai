package app

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/runtimecheck"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/validate"
)

type PluginExportOptions struct {
	Root     string
	Platform string
	Output   string
}

type PluginExportResult struct {
	Lines []string
}

type exportMetadata struct {
	PluginName     string   `json:"plugin_name"`
	Platform       string   `json:"platform"`
	Runtime        string   `json:"runtime"`
	Manager        string   `json:"manager"`
	BootstrapModel string   `json:"bootstrap_model"`
	Next           []string `json:"next"`
	BundleFormat   string   `json:"bundle_format"`
	GeneratedBy    string   `json:"generated_by"`
}

func (PluginService) Export(opts PluginExportOptions) (PluginExportResult, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	platform := strings.TrimSpace(opts.Platform)
	if platform == "" {
		return PluginExportResult{}, fmt.Errorf("export requires --platform")
	}

	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return PluginExportResult{}, err
	}
	if _, err := graph.Manifest.SelectedTargets(platform); err != nil {
		return PluginExportResult{}, err
	}
	if platform != "codex-runtime" && platform != "claude" {
		return PluginExportResult{}, fmt.Errorf("export supports only launcher-based interpreted targets: codex-runtime or claude")
	}
	if graph.Launcher == nil {
		return PluginExportResult{}, fmt.Errorf("export requires launcher-based target %q with launcher.yaml", platform)
	}
	switch graph.Launcher.Runtime {
	case "python", "node", "shell":
	default:
		return PluginExportResult{}, fmt.Errorf("export supports only interpreted runtimes (python, node, shell); found %q", graph.Launcher.Runtime)
	}

	report, err := validate.Validate(root, platform)
	if err != nil {
		if re, ok := err.(*validate.ReportError); ok {
			report = re.Report
		} else {
			return PluginExportResult{}, err
		}
	}
	if len(report.Failures) > 0 {
		return PluginExportResult{}, fmt.Errorf("export requires validate --strict to pass for %s", platform)
	}
	if len(report.Warnings) > 0 {
		return PluginExportResult{}, fmt.Errorf("export requires validate --strict to pass for %s: %d warning(s) present", platform, len(report.Warnings))
	}
	if drift, err := pluginmanifest.Drift(root, platform); err != nil {
		return PluginExportResult{}, err
	} else if len(drift) > 0 {
		return PluginExportResult{}, fmt.Errorf("export requires rendered artifacts to be up to date for %s (run plugin-kit-ai render . and plugin-kit-ai render . --check)", platform)
	}

	project, err := runtimecheck.Inspect(runtimecheck.Inputs{
		Root:     root,
		Targets:  []string{platform},
		Launcher: graph.Launcher,
	})
	if err != nil {
		return PluginExportResult{}, err
	}
	diagnosis := runtimecheck.Diagnose(project)
	if diagnosis.Status != runtimecheck.StatusReady {
		return PluginExportResult{}, fmt.Errorf("export requires runtime readiness: %s", diagnosis.Reason)
	}

	rendered, err := pluginmanifest.Render(root, platform)
	if err != nil {
		return PluginExportResult{}, err
	}

	files, err := exportFileList(root, graph, project, rendered.Artifacts)
	if err != nil {
		return PluginExportResult{}, err
	}
	outputPath := exportOutputPath(root, graph.Manifest.Name, platform, graph.Launcher.Runtime, opts.Output)
	if rel, ok := relWithinRoot(root, outputPath); ok {
		files = slices.DeleteFunc(files, func(path string) bool { return path == rel })
	}
	metadata := exportMetadata{
		PluginName:     graph.Manifest.Name,
		Platform:       platform,
		Runtime:        graph.Launcher.Runtime,
		Manager:        exportManager(project),
		BootstrapModel: exportBootstrapModel(project),
		Next: []string{
			"plugin-kit-ai doctor .",
			"plugin-kit-ai bootstrap .",
			fmt.Sprintf("plugin-kit-ai validate . --platform %s --strict", platform),
		},
		BundleFormat: "tar.gz",
		GeneratedBy:  "plugin-kit-ai export",
	}
	if err := writeExportArchive(root, outputPath, files, metadata); err != nil {
		return PluginExportResult{}, err
	}

	lines := []string{
		project.ProjectLine(),
		"Exported bundle: " + outputPath,
		fmt.Sprintf("Included files: %d", len(files)+1),
		"Next:",
		"  tar -xzf " + filepath.Base(outputPath),
		"  plugin-kit-ai doctor .",
		"  plugin-kit-ai bootstrap .",
		fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
	}
	return PluginExportResult{Lines: lines}, nil
}

func exportFileList(root string, graph pluginmanifest.PackageGraph, project runtimecheck.Project, rendered []pluginmanifest.Artifact) ([]string, error) {
	set := map[string]struct{}{}
	for _, rel := range graph.SourceFiles {
		addExportPath(set, rel)
	}
	for _, artifact := range rendered {
		addExportPath(set, artifact.RelPath)
	}
	for _, rel := range launcherBundlePaths(root, project.Entrypoint) {
		addExportPath(set, rel)
	}
	switch project.Runtime {
	case "python":
		addDirIfExists(root, set, "src")
		addIfExists(root, set, "requirements.txt")
		addIfExists(root, set, "pyproject.toml")
		addIfExists(root, set, "uv.lock")
		addIfExists(root, set, "poetry.lock")
		addIfExists(root, set, "Pipfile")
		addIfExists(root, set, "Pipfile.lock")
	case "node":
		addDirIfExists(root, set, "src")
		addIfExists(root, set, "package.json")
		addIfExists(root, set, "tsconfig.json")
		addIfExists(root, set, ".yarnrc.yml")
		addIfExists(root, set, "package-lock.json")
		addIfExists(root, set, "npm-shrinkwrap.json")
		addIfExists(root, set, "pnpm-lock.yaml")
		addIfExists(root, set, "yarn.lock")
		addIfExists(root, set, "bun.lock")
		addIfExists(root, set, "bun.lockb")
		if project.Node.IsTypeScript && project.Node.UsesBuiltOutput {
			addDirIfExists(root, set, project.Node.OutputDir)
		} else if strings.TrimSpace(project.Node.RuntimeTarget) != "" {
			addIfExists(root, set, project.Node.RuntimeTarget)
		}
	case "shell":
		addDirIfExists(root, set, "scripts")
	}
	excludes := exportExcludedPaths(root, project)
	out := make([]string, 0, len(set))
	for rel := range set {
		if shouldExcludeExportPath(rel, excludes) {
			continue
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("export refuses symlinked path %s", rel)
		}
		if info.IsDir() {
			continue
		}
		out = append(out, rel)
	}
	slices.Sort(out)
	return out, nil
}

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

func addIfExists(root string, set map[string]struct{}, rel string) {
	rel = normalizeExportPath(rel)
	if rel == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
		set[rel] = struct{}{}
	}
}

func addDirIfExists(root string, set map[string]struct{}, rel string) {
	rel = normalizeExportPath(rel)
	if rel == "" {
		return
	}
	dir := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return
	}
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		sub, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		set[filepath.ToSlash(sub)] = struct{}{}
		return nil
	})
}

func addExportPath(set map[string]struct{}, rel string) {
	rel = normalizeExportPath(rel)
	if rel == "" {
		return
	}
	set[rel] = struct{}{}
}

func normalizeExportPath(rel string) string {
	rel = strings.TrimPrefix(strings.TrimSpace(rel), "./")
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == "" {
		return ""
	}
	return rel
}

func launcherBundlePaths(root, entrypoint string) []string {
	base := normalizeExportPath(entrypoint)
	if base == "" {
		return nil
	}
	candidates := []string{base}
	if !strings.HasSuffix(base, ".cmd") {
		candidates = append(candidates, base+".cmd")
	}
	out := make([]string, 0, len(candidates))
	for _, rel := range candidates {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			out = append(out, rel)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func exportExcludedPaths(root string, project runtimecheck.Project) []string {
	excludes := []string{".venv", "node_modules", ".pnp.cjs", ".pnp.loader.mjs"}
	if project.Python.ReadySource == runtimecheck.PythonEnvSourceManagerOwned && strings.TrimSpace(project.Python.ProbedEnvPath) != "" {
		if rel, ok := relWithinRoot(root, project.Python.ProbedEnvPath); ok {
			excludes = append(excludes, rel)
		}
	}
	return excludes
}

func shouldExcludeExportPath(rel string, excludes []string) bool {
	for _, exclude := range excludes {
		exclude = normalizeExportPath(exclude)
		if exclude == "" {
			continue
		}
		if rel == exclude || strings.HasPrefix(rel, exclude+"/") {
			return true
		}
	}
	return false
}

func relWithinRoot(root, path string) (string, bool) {
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

func exportOutputPath(root, name, platform, runtime, output string) string {
	if strings.TrimSpace(output) != "" {
		return output
	}
	file := fmt.Sprintf("%s_%s_%s_bundle.tar.gz", name, platform, runtime)
	return filepath.Join(root, file)
}

func exportManager(project runtimecheck.Project) string {
	switch project.Runtime {
	case "python":
		return project.Python.ManagerDisplay()
	case "node":
		return project.Node.ManagerDisplay()
	default:
		return "none"
	}
}

func exportBootstrapModel(project runtimecheck.Project) string {
	switch project.Runtime {
	case "python":
		return project.Python.CanonicalEnvSourceDisplay()
	case "node":
		if project.Node.IsTypeScript {
			return "recipient-side install and build"
		}
		return "recipient-side install"
	case "shell":
		return "launcher plus executable shell scripts"
	default:
		return "n/a"
	}
}
