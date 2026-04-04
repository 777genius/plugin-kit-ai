package publicationmodel

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publishschema"
	"github.com/777genius/plugin-kit-ai/cli/internal/targetcontracts"
)

type Core struct {
	APIVersion  string `json:"api_version"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type Package struct {
	Target           string            `json:"target"`
	PackageFamily    string            `json:"package_family"`
	ChannelFamilies  []string          `json:"channel_families"`
	TargetClass      string            `json:"target_class"`
	InstallModel     string            `json:"install_model,omitempty"`
	AuthoredInputs   []string          `json:"authored_inputs"`
	AuthoredDocs     map[string]string `json:"authored_docs,omitempty"`
	ManagedArtifacts []string          `json:"managed_artifacts"`
}

type Model struct {
	Core     Core      `json:"core"`
	Packages []Package `json:"packages"`
	Channels []Channel `json:"channels"`
}

type Channel struct {
	Family         string            `json:"family"`
	Path           string            `json:"path"`
	PackageTargets []string          `json:"package_targets"`
	Details        map[string]string `json:"details,omitempty"`
}

func Build(graph pluginmodel.PackageGraph, publication publishschema.State, selected []string) Model {
	out := Model{
		Core: Core{
			APIVersion:  strings.TrimSpace(graph.Manifest.APIVersion),
			Name:        strings.TrimSpace(graph.Manifest.Name),
			Version:     strings.TrimSpace(graph.Manifest.Version),
			Description: strings.TrimSpace(graph.Manifest.Description),
		},
		Packages: []Package{},
		Channels: []Channel{},
	}
	for _, target := range selected {
		pkg, ok := buildPackage(graph, publication, target)
		if !ok {
			continue
		}
		out.Packages = append(out.Packages, pkg)
	}
	slices.SortFunc(out.Packages, func(a, b Package) int {
		return strings.Compare(a.Target, b.Target)
	})
	out.Channels = buildChannels(publication, out.Packages)
	return out
}

func buildPackage(graph pluginmodel.PackageGraph, publication publishschema.State, target string) (Package, bool) {
	family, channels := packageFamilies(target)
	if family == "" {
		return Package{}, false
	}
	entry, ok := targetcontracts.Lookup(target)
	if !ok {
		return Package{}, false
	}
	state, ok := graph.Targets[target]
	if !ok {
		return Package{}, false
	}
	authoredSet := map[string]struct{}{
		pluginmodel.FileName: {},
	}
	if graph.Launcher != nil && entry.LauncherRequirement == "required" {
		authoredSet[pluginmodel.LauncherFileName] = struct{}{}
	}
	authoredDocs := map[string]string{}
	for kind, path := range state.Docs {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		authoredDocs[kind] = path
		authoredSet[path] = struct{}{}
	}
	for _, paths := range state.Components {
		for _, path := range paths {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			authoredSet[path] = struct{}{}
		}
	}
	for _, path := range graph.Portable.Paths("skills") {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		authoredSet[filepath.ToSlash(path)] = struct{}{}
	}
	if graph.Portable.MCP != nil && strings.TrimSpace(graph.Portable.MCP.Path) != "" {
		authoredSet[filepath.ToSlash(graph.Portable.MCP.Path)] = struct{}{}
	}
	for _, path := range publicationPathsForTarget(publication, target) {
		authoredSet[path] = struct{}{}
	}
	return Package{
		Target:           target,
		PackageFamily:    family,
		ChannelFamilies:  cloneStrings(channels),
		TargetClass:      entry.TargetClass,
		InstallModel:     entry.InstallModel,
		AuthoredInputs:   sortedKeys(authoredSet),
		AuthoredDocs:     cloneStringMap(authoredDocs),
		ManagedArtifacts: cloneStrings(entry.ManagedArtifacts),
	}, true
}

func packageFamilies(target string) (string, []string) {
	switch strings.TrimSpace(target) {
	case "codex-package":
		return "codex-plugin", []string{"codex-marketplace"}
	case "claude":
		return "claude-plugin", []string{"claude-marketplace"}
	case "gemini":
		return "gemini-extension", []string{"gemini-gallery"}
	default:
		return "", nil
	}
}

func buildChannels(publication publishschema.State, packages []Package) []Channel {
	out := []Channel{}
	if publication.Codex != nil {
		targets := packageTargetsForFamily(packages, "codex-marketplace")
		if len(targets) > 0 {
			out = append(out, Channel{
				Family:         "codex-marketplace",
				Path:           publication.Codex.Path,
				PackageTargets: targets,
				Details: map[string]string{
					"marketplace_name":      publication.Codex.MarketplaceName,
					"source_root":           publication.Codex.SourceRoot,
					"category":              publication.Codex.Category,
					"installation_policy":   publication.Codex.InstallationPolicy,
					"authentication_policy": publication.Codex.AuthenticationPolicy,
				},
			})
		}
	}
	if publication.Claude != nil {
		targets := packageTargetsForFamily(packages, "claude-marketplace")
		if len(targets) > 0 {
			out = append(out, Channel{
				Family:         "claude-marketplace",
				Path:           publication.Claude.Path,
				PackageTargets: targets,
				Details: map[string]string{
					"marketplace_name": publication.Claude.MarketplaceName,
					"owner_name":       publication.Claude.OwnerName,
					"source_root":      publication.Claude.SourceRoot,
				},
			})
		}
	}
	if publication.Gemini != nil {
		targets := packageTargetsForFamily(packages, "gemini-gallery")
		if len(targets) > 0 {
			out = append(out, Channel{
				Family:         "gemini-gallery",
				Path:           publication.Gemini.Path,
				PackageTargets: targets,
				Details: map[string]string{
					"distribution":          publication.Gemini.Distribution,
					"repository_visibility": publication.Gemini.RepositoryVisibility,
					"github_topic":          publication.Gemini.GitHubTopic,
					"manifest_root":         publication.Gemini.ManifestRoot,
				},
			})
		}
	}
	slices.SortFunc(out, func(a, b Channel) int {
		return strings.Compare(a.Family, b.Family)
	})
	return out
}

func publicationPathsForTarget(publication publishschema.State, target string) []string {
	switch strings.TrimSpace(target) {
	case "codex-package":
		if publication.Codex != nil {
			return []string{publication.Codex.Path}
		}
	case "claude":
		if publication.Claude != nil {
			return []string{publication.Claude.Path}
		}
	case "gemini":
		if publication.Gemini != nil {
			return []string{publication.Gemini.Path}
		}
	}
	return nil
}

func packageTargetsForFamily(packages []Package, family string) []string {
	var out []string
	for _, pkg := range packages {
		if slices.Contains(pkg.ChannelFamilies, family) {
			out = append(out, pkg.Target)
		}
	}
	slices.Sort(out)
	return out
}

func cloneStrings(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	return append([]string(nil), items...)
}

func cloneStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]string, len(items))
	for key, value := range items {
		out[key] = value
	}
	return out
}

func sortedKeys(items map[string]struct{}) []string {
	out := make([]string, 0, len(items))
	for key := range items {
		out = append(out, filepath.ToSlash(key))
	}
	slices.Sort(out)
	return out
}
