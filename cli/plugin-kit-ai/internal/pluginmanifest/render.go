package pluginmanifest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/777genius/plugin-kit-ai/cli/internal/platformexec"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationexec"
	"github.com/777genius/plugin-kit-ai/cli/internal/publishschema"
	"github.com/777genius/plugin-kit-ai/cli/internal/scaffold"
	"github.com/777genius/plugin-kit-ai/cli/internal/targetcontracts"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
)

func generatePackage(root string, target string) (RenderResult, error) {
	ctx, _, err := loadPackageContext(root, target)
	if err != nil {
		return RenderResult{}, err
	}
	artifactMap := map[string][]byte{}
	for _, name := range ctx.selectedTargets {
		generated, err := renderTargetArtifacts(root, ctx.graph, name)
		if err != nil {
			return RenderResult{}, err
		}
		for _, artifact := range generated {
			relPath := filepath.ToSlash(filepath.Clean(artifact.RelPath))
			if existing, ok := artifactMap[relPath]; ok {
				if !bytes.Equal(existing, artifact.Content) {
					return RenderResult{}, fmt.Errorf("conflicting generated artifact %s across targets", relPath)
				}
				continue
			}
			artifactMap[relPath] = artifact.Content
		}
	}
	publicationArtifacts, err := publicationexec.Generate(ctx.graph, ctx.publication, ctx.selectedTargets)
	if err != nil {
		return RenderResult{}, err
	}
	for _, artifact := range publicationArtifacts {
		relPath := filepath.ToSlash(filepath.Clean(artifact.RelPath))
		if existing, ok := artifactMap[relPath]; ok {
			if !bytes.Equal(existing, artifact.Content) {
				return RenderResult{}, fmt.Errorf("conflicting generated artifact %s across publication channels and targets", relPath)
			}
			continue
		}
		artifactMap[relPath] = artifact.Content
	}
	if ctx.layout.IsCanonical() {
		if claudeBoundary, err := buildRootClaudeBoundaryArtifact(ctx.layout); err != nil {
			return RenderResult{}, err
		} else if claudeBoundary != nil {
			artifactMap[claudeBoundary.RelPath] = claudeBoundary.Content
		}
		if readme, err := buildRootReadmeArtifact(root, ctx.layout, ctx.graph.Manifest); err != nil {
			return RenderResult{}, err
		} else if readme != nil {
			artifactMap[readme.RelPath] = readme.Content
		}
		if generatedGuide, err := buildRootGeneratedGuideArtifact(root, ctx.layout, ctx.graph, ctx.publication); err != nil {
			return RenderResult{}, err
		} else if generatedGuide != nil {
			artifactMap[generatedGuide.RelPath] = generatedGuide.Content
		}
	}
	artifacts := make([]Artifact, 0, len(artifactMap))
	for path, content := range artifactMap {
		artifacts = append(artifacts, Artifact{RelPath: path, Content: content})
	}
	slices.SortFunc(artifacts, func(a, b Artifact) int { return strings.Compare(a.RelPath, b.RelPath) })

	expected := map[string]struct{}{}
	for _, artifact := range artifacts {
		expected[artifact.RelPath] = struct{}{}
	}
	var stale []string
	for _, path := range expectedManagedPaths(root, ctx.layout, ctx.graph, ctx.publication, ctx.selectedTargets) {
		if _, ok := expected[path]; ok {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, path)); err == nil {
			stale = append(stale, path)
		}
	}
	slices.Sort(stale)
	return RenderResult{Artifacts: artifacts, StalePaths: stale}, nil
}

func writeArtifacts(root string, artifacts []Artifact) error {
	for _, artifact := range artifacts {
		full := filepath.Join(root, filepath.FromSlash(artifact.RelPath))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, artifact.Content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func removeArtifacts(root string, relPaths []string) error {
	for _, relPath := range relPaths {
		full := filepath.Join(root, filepath.FromSlash(relPath))
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func driftPackage(root string, target string) ([]string, error) {
	result, err := generatePackage(root, target)
	if err != nil {
		return nil, err
	}
	var drift []string
	for _, artifact := range result.Artifacts {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(artifact.RelPath)))
		if err != nil {
			drift = append(drift, artifact.RelPath)
			continue
		}
		if !artifactContentEqual(body, artifact.Content) {
			drift = append(drift, artifact.RelPath)
		}
	}
	drift = append(drift, result.StalePaths...)
	slices.Sort(drift)
	return slices.Compact(drift), nil
}

func renderTargetArtifacts(root string, graph PackageGraph, target string) ([]Artifact, error) {
	tc := graph.Targets[target]
	adapter, ok := platformexec.Lookup(target)
	if !ok {
		return nil, fmt.Errorf("unsupported target %q", target)
	}
	return adapter.Generate(root, graph, tc)
}

func artifactContentEqual(actual, expected []byte) bool {
	if bytes.Equal(actual, expected) {
		return true
	}
	if !looksLikeText(actual) || !looksLikeText(expected) {
		return false
	}
	return bytes.Equal(normalizeTextNewlines(actual), normalizeTextNewlines(expected))
}

func looksLikeText(body []byte) bool {
	return utf8.Valid(body) && !bytes.Contains(body, []byte{0})
}

func normalizeTextNewlines(body []byte) []byte {
	body = bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
	body = bytes.ReplaceAll(body, []byte("\r"), []byte("\n"))
	return body
}

func expectedManagedPaths(root string, layout authoredLayout, graph PackageGraph, publication publishschema.State, selected []string) []string {
	seen := map[string]struct{}{}
	for _, target := range selected {
		profile, ok := platformmeta.Lookup(target)
		if !ok {
			continue
		}
		tc := graph.Targets[target]
		for _, spec := range profile.ManagedArtifacts {
			switch spec.Kind {
			case platformmeta.ManagedArtifactStatic:
				seen[spec.Path] = struct{}{}
			case platformmeta.ManagedArtifactPortableMCP:
				if graph.Portable.MCP != nil {
					seen[spec.Path] = struct{}{}
				}
			case platformmeta.ManagedArtifactPortableSkills:
				sourceRoot := filepath.ToSlash(strings.TrimSpace(spec.SourceRoot))
				if sourceRoot == "" {
					sourceRoot = "skills"
				}
				addManagedCopies(seen, graph.Portable.Paths("skills"), sourceRoot, spec.OutputRoot)
			case platformmeta.ManagedArtifactMirror:
				if spec.OutputRoot == "" {
					rel := filepath.ToSlash(strings.TrimSpace(tc.DocPath(spec.ComponentKind)))
					if rel == "" {
						continue
					}
					relPath, err := filepath.Rel(filepath.ToSlash(spec.SourceRoot), rel)
					if err != nil {
						continue
					}
					seen[filepath.ToSlash(filepath.Join(spec.OutputRoot, relPath))] = struct{}{}
					continue
				}
				addManagedCopies(seen, tc.ComponentPaths(spec.ComponentKind), spec.SourceRoot, spec.OutputRoot)
			case platformmeta.ManagedArtifactSelectedContext:
				continue
			}
		}
		if adapter, ok := platformexec.Lookup(target); ok {
			extraPaths, err := adapter.ManagedPaths(root, graph, tc)
			if err == nil {
				for _, path := range extraPaths {
					seen[path] = struct{}{}
				}
			}
		}
	}
	for _, path := range publicationexec.ManagedPaths(publication, selected) {
		seen[path] = struct{}{}
	}
	if layout.IsCanonical() {
		seen["GENERATED.md"] = struct{}{}
		if fileExists(filepath.Join(root, layout.Path("README.md"))) {
			seen["README.md"] = struct{}{}
		}
	}
	return sortedKeys(seen)
}

func discoveredTargetKinds(tc TargetComponents) []string {
	var kinds []string
	for kind, path := range tc.Docs {
		if strings.TrimSpace(path) != "" {
			kinds = append(kinds, kind)
		}
	}
	for kind, paths := range tc.Components {
		if len(paths) > 0 {
			kinds = append(kinds, kind)
		}
	}
	slices.Sort(kinds)
	return kinds
}

func unsupportedKinds(entry targetcontracts.Entry, graph PackageGraph, tc TargetComponents) []string {
	supportedPortable := setOf(entry.PortableComponentKinds)
	var unsupported []string
	if len(graph.Portable.Paths("skills")) > 0 && !supportedPortable["skills"] {
		unsupported = append(unsupported, "skills")
	}
	if graph.Portable.MCP != nil && !supportedPortable["mcp_servers"] {
		unsupported = append(unsupported, "mcp_servers")
	}
	supportedNative := setOf(entry.TargetComponentKinds)
	for _, kind := range discoveredTargetKinds(tc) {
		if !supportedNative[kind] {
			unsupported = append(unsupported, kind)
		}
	}
	slices.Sort(unsupported)
	return slices.Compact(unsupported)
}

func targetFiles(tc TargetComponents) []string {
	var out []string
	for _, path := range tc.Docs {
		if strings.TrimSpace(path) != "" {
			out = append(out, path)
		}
	}
	for _, paths := range tc.Components {
		out = append(out, paths...)
	}
	slices.Sort(out)
	return out
}

func buildRootReadmeArtifact(root string, layout authoredLayout, manifest Manifest) (*Artifact, error) {
	if !layout.IsCanonical() {
		return nil, nil
	}
	authoredReadme := layout.Path("README.md")
	authoredReadmePath := filepath.Join(root, authoredReadme)
	if _, err := os.Stat(authoredReadmePath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	title := strings.TrimSpace(manifest.Name)
	if title == "" {
		title = "plugin"
	}
	var body strings.Builder
	body.WriteString("# ")
	body.WriteString(title)
	body.WriteString("\n\n")
	body.WriteString("This file is generated by `plugin-kit-ai generate`.\n")
	body.WriteString("Do not edit it by hand. Edit files under `src/`, especially [`src/README.md`](./src/README.md).\n\n")
	body.WriteString("Start here:\n\n")
	body.WriteString("- full plugin guide: [`src/README.md`](./src/README.md)\n")
	body.WriteString("- boundary instructions for humans and agents: [`AGENTS.md`](./AGENTS.md)\n")
	body.WriteString("- generated root output inventory: [`GENERATED.md`](./GENERATED.md)\n\n")
	body.WriteString("This plugin root is the native/generated output surface for the supported targets.\n")
	artifact := Artifact{RelPath: "README.md", Content: []byte(body.String())}
	return &artifact, nil
}

func buildRootClaudeBoundaryArtifact(layout authoredLayout) (*Artifact, error) {
	if !layout.IsCanonical() {
		return nil, nil
	}
	body, _, err := scaffold.RenderTemplate("ROOT.CLAUDE.md.tmpl", scaffold.Data{})
	if err != nil {
		return nil, err
	}
	return &Artifact{
		RelPath: "CLAUDE.md",
		Content: body,
	}, nil
}

func stripLeadingMarkdownTitle(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")
	if len(lines) == 0 {
		return body
	}
	if strings.HasPrefix(strings.TrimSpace(lines[0]), "# ") {
		return strings.TrimLeft(strings.Join(lines[1:], "\n"), "\n")
	}
	return body
}

func buildRootGeneratedGuideArtifact(root string, layout authoredLayout, graph PackageGraph, publication publishschema.State) (*Artifact, error) {
	if !layout.IsCanonical() {
		return nil, nil
	}
	paths, err := generatedArtifactInventory(root, layout, graph, publication, graph.Manifest.EnabledTargets())
	if err != nil {
		return nil, err
	}
	var body strings.Builder
	body.WriteString("# Generated Outputs\n\n")
	body.WriteString("This file is generated by `plugin-kit-ai generate`.\n")
	body.WriteString("Do not edit the paths below by hand. Edit only `src/`, then regenerate.\n\n")
	body.WriteString("This inventory covers the full plugin package across all enabled targets.\n\n")
	body.WriteString("## Boundary Docs\n\n")
	body.WriteString("These committed root docs are guidance files and are not generated outputs:\n\n")
	for _, rel := range boundaryDocsForLayout(layout) {
		body.WriteString("- `")
		body.WriteString(rel)
		body.WriteString("`\n")
	}
	body.WriteString("\n## Managed Generated Outputs\n\n")
	body.WriteString("`README.md` is a generated root entrypoint that points readers to `src/README.md`.\n\n")
	for _, rel := range paths {
		body.WriteString("- `")
		body.WriteString(rel)
		body.WriteString("`\n")
	}
	body.WriteString("\n## Refresh\n\n")
	body.WriteString("```bash\n")
	body.WriteString("plugin-kit-ai normalize .\n")
	body.WriteString("plugin-kit-ai generate .\n")
	body.WriteString("plugin-kit-ai generate --check .\n")
	body.WriteString("```\n")
	return &Artifact{
		RelPath: "GENERATED.md",
		Content: []byte(body.String()),
	}, nil
}

func generatedArtifactInventory(root string, layout authoredLayout, graph PackageGraph, publication publishschema.State, selected []string) ([]string, error) {
	artifactMap := map[string]struct{}{}
	boundarySet := map[string]struct{}{}
	for _, rel := range boundaryDocsForLayout(layout) {
		boundarySet[filepath.ToSlash(filepath.Clean(rel))] = struct{}{}
	}
	for _, target := range selected {
		generated, err := renderTargetArtifacts(root, graph, target)
		if err != nil {
			return nil, err
		}
		for _, artifact := range generated {
			rel := filepath.ToSlash(filepath.Clean(artifact.RelPath))
			if _, skip := boundarySet[rel]; skip {
				continue
			}
			artifactMap[rel] = struct{}{}
		}
	}
	publicationArtifacts, err := publicationexec.Generate(graph, publication, selected)
	if err != nil {
		return nil, err
	}
	for _, artifact := range publicationArtifacts {
		artifactMap[filepath.ToSlash(filepath.Clean(artifact.RelPath))] = struct{}{}
	}
	if readme, err := buildRootReadmeArtifact(root, layout, graph.Manifest); err != nil {
		return nil, err
	} else if readme != nil {
		artifactMap[readme.RelPath] = struct{}{}
	}
	artifactMap["GENERATED.md"] = struct{}{}
	return sortedKeys(artifactMap), nil
}

func boundaryDocsForLayout(layout authoredLayout) []string {
	if !layout.IsCanonical() {
		return nil
	}
	return []string{"CLAUDE.md", "AGENTS.md"}
}

func generatedGuideForLayout(layout authoredLayout) string {
	if !layout.IsCanonical() {
		return ""
	}
	return "GENERATED.md"
}
