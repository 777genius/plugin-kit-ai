package pluginmanifest

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/platformexec"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publishschema"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
)

type discoveredPackage struct {
	layout      authoredLayout
	graph       PackageGraph
	publication publishschema.State
	warnings    []Warning
}

func discoverPackage(root string) (PackageGraph, []Warning, error) {
	state, err := discoverPackageState(root)
	if err != nil {
		return PackageGraph{}, nil, err
	}
	return state.graph, state.warnings, nil
}

func discoverPackageState(root string) (discoveredPackage, error) {
	layout, err := detectAuthoredLayout(root)
	if err != nil {
		return discoveredPackage{}, err
	}
	return discoverPackageStateInLayout(root, layout)
}

func discoverPackageStateInLayout(root string, layout authoredLayout) (discoveredPackage, error) {
	manifest, warnings, err := loadManifestWithWarnings(root)
	if err != nil {
		return discoveredPackage{}, err
	}
	launcher, err := loadLauncherForTargets(root, manifest.EnabledTargets())
	if err != nil {
		return discoveredPackage{}, err
	}
	graph := PackageGraph{
		Manifest: manifest,
		Launcher: launcher,
		Portable: newPortableComponents(),
		Targets:  make(map[string]TargetState, len(manifest.Targets)),
	}
	if err := validateRemovedPortableInputs(root, layout, manifest.EnabledTargets()); err != nil {
		return discoveredPackage{}, err
	}
	sourceSet := map[string]struct{}{layout.Path(FileName): {}}
	if launcher != nil {
		sourceSet[layout.Path(LauncherFileName)] = struct{}{}
	}
	if fileExists(filepath.Join(root, layout.Path("README.md"))) {
		sourceSet[layout.Path("README.md")] = struct{}{}
	}
	publication, err := discoverPublication(root, layout)
	if err != nil {
		return discoveredPackage{}, err
	}
	if err := publication.ValidateTargets(manifest.EnabledTargets()); err != nil {
		return discoveredPackage{}, err
	}
	addSourceFiles(sourceSet, publication.Paths())

	skillPaths := discoverFiles(root, layout.Path(filepath.Join("skills")), func(rel string) bool {
		return strings.HasSuffix(rel, "SKILL.md")
	})
	graph.Portable.Add("skills", skillPaths...)
	addSourceFiles(sourceSet, skillPaths)

	if mcpDoc, ok, err := discoverMCP(root, layout); err != nil {
		return discoveredPackage{}, err
	} else if ok {
		graph.Portable.MCP = mcpDoc
		sourceSet[mcpDoc.Path] = struct{}{}
	}

	for _, target := range manifest.EnabledTargets() {
		state, err := discoverTarget(root, layout, target)
		if err != nil {
			return discoveredPackage{}, err
		}
		graph.Targets[target] = state
		addSourceFiles(sourceSet, targetFiles(state))
	}

	graph.SourceFiles = sortedKeys(sourceSet)
	return discoveredPackage{
		layout:      layout,
		graph:       graph,
		publication: publication,
		warnings:    warnings,
	}, nil
}

func loadLauncherForTargets(root string, targets []string) (*Launcher, error) {
	requires := false
	for _, target := range targets {
		profile, ok := platformmeta.Lookup(target)
		if !ok {
			continue
		}
		if profile.Launcher.Requirement == platformmeta.LauncherRequired {
			requires = true
			break
		}
	}
	launcher, err := loadLauncher(root)
	if err == nil {
		return &launcher, nil
	}
	if os.IsNotExist(err) && !requires {
		return nil, nil
	}
	if os.IsNotExist(err) {
		layout, lerr := detectAuthoredLayout(root)
		if lerr != nil {
			return nil, lerr
		}
		return nil, fmt.Errorf("required launcher missing: %s", layout.Path(LauncherFileName))
	}
	return nil, err
}

func requiresLauncherForTarget(target string) bool {
	profile, ok := platformmeta.Lookup(target)
	return ok && profile.Launcher.Requirement == platformmeta.LauncherRequired
}

func discoverTarget(root string, layout authoredLayout, target string) (TargetComponents, error) {
	profile, ok := platformmeta.Lookup(target)
	if !ok {
		return TargetComponents{}, fmt.Errorf("unsupported target %q", target)
	}
	state := newTargetComponents(target)
	docKinds := map[string]struct{}{}
	mirrorKinds := map[string]struct{}{}
	for _, spec := range profile.ManagedArtifacts {
		if spec.Kind == platformmeta.ManagedArtifactMirror {
			mirrorKinds[spec.ComponentKind] = struct{}{}
		}
	}
	for _, spec := range profile.NativeDocs {
		docKinds[spec.Kind] = struct{}{}
		path := filepath.ToSlash(spec.Path)
		if fileExists(filepath.Join(root, path)) {
			state.SetDoc(spec.Kind, path)
			if _, ok := mirrorKinds[spec.Kind]; ok {
				state.AddComponent(spec.Kind, path)
			}
		}
	}
	for _, kind := range profile.Contract.TargetComponentKinds {
		if _, isDoc := docKinds[kind]; isDoc {
			continue
		}
		dir := layout.Path(filepath.Join("targets", target, kind))
		state.AddComponent(kind, discoverFiles(root, dir, nil)...)
	}
	adapter, ok := platformexec.Lookup(target)
	if !ok {
		return TargetComponents{}, fmt.Errorf("unsupported target %q", target)
	}
	if err := adapter.RefineDiscovery(root, &state); err != nil {
		return TargetComponents{}, err
	}
	return state, nil
}

func discoverFiles(root, dir string, keep func(rel string) bool) []string {
	full := filepath.Join(root, dir)
	var out []string
	_ = filepath.WalkDir(full, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if keep != nil && !keep(rel) {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	slices.Sort(out)
	return out
}

func discoverMCP(root string, layout authoredLayout) (*PortableMCP, bool, error) {
	for _, legacyRel := range []string{"mcp/servers.json", "mcp/servers.yml"} {
		if fileExists(filepath.Join(root, layout.Path(legacyRel))) {
			return nil, false, fmt.Errorf("unsupported portable MCP authored path %s: use src/mcp/servers.yaml", legacyRel)
		}
	}
	for _, rel := range []string{"mcp/servers.yaml"} {
		authoredRel := layout.Path(rel)
		full := filepath.Join(root, authoredRel)
		body, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		parsed, err := pluginmodel.ParsePortableMCP(authoredRel, body)
		if err != nil {
			return nil, false, err
		}
		return &PortableMCP{Path: authoredRel, Servers: parsed.Servers, File: parsed.File}, true, nil
	}
	return nil, false, nil
}
