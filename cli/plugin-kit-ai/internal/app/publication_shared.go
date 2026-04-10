package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationexec"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
)

func publicationModeLabel(dryRun bool) string {
	if dryRun {
		return "dry-run"
	}
	return "apply"
}

func orderedPublicationChannels(model publicationmodel.Model) []publicationmodel.Channel {
	order := map[string]int{
		"codex-marketplace":  0,
		"claude-marketplace": 1,
		"gemini-gallery":     2,
	}
	out := append([]publicationmodel.Channel(nil), model.Channels...)
	slices.SortFunc(out, func(a, b publicationmodel.Channel) int {
		oa, oka := order[a.Family]
		ob, okb := order[b.Family]
		switch {
		case oka && okb && oa != ob:
			return oa - ob
		case oka && !okb:
			return -1
		case !oka && okb:
			return 1
		default:
			return strings.Compare(a.Family, b.Family)
		}
	})
	return out
}

func channelsNeedLocalDest(channels []publicationmodel.Channel) bool {
	for _, channel := range channels {
		switch channel.Family {
		case "codex-marketplace", "claude-marketplace":
			return true
		}
	}
	return false
}

func cleanedDestForMulti(dest string, channels []publicationmodel.Channel) string {
	if !channelsNeedLocalDest(channels) {
		return ""
	}
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return ""
	}
	return filepath.Clean(dest)
}

func clonePublishResults(items []PluginPublishResult) []PluginPublishResult {
	if len(items) == 0 {
		return []PluginPublishResult{}
	}
	out := make([]PluginPublishResult, 0, len(items))
	for _, item := range items {
		cloned := item
		cloned.Details = cloneStringMap(item.Details)
		cloned.Issues = append([]PluginPublishIssue(nil), item.Issues...)
		cloned.Warnings = cloneStrings(item.Warnings)
		cloned.NextSteps = cloneStrings(item.NextSteps)
		cloned.Channels = clonePublishResults(item.Channels)
		cloned.Lines = nil
		out = append(out, cloned)
	}
	return out
}

func publicationPackageForTarget(model publicationmodel.Model, target string) (publicationmodel.Package, bool) {
	for _, pkg := range model.Packages {
		if pkg.Target == target {
			return pkg, true
		}
	}
	return publicationmodel.Package{}, false
}

func publicationChannelForTarget(model publicationmodel.Model, target string) (publicationmodel.Channel, bool) {
	for _, channel := range model.Channels {
		if slices.Contains(channel.PackageTargets, target) {
			return channel, true
		}
	}
	return publicationmodel.Channel{}, false
}

func publicationChannelForFamily(model publicationmodel.Model, family string) (publicationmodel.Channel, bool) {
	for _, channel := range model.Channels {
		if channel.Family == family {
			return channel, true
		}
	}
	return publicationmodel.Channel{}, false
}

func normalizePackageRoot(value, pluginName string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = filepath.ToSlash(filepath.Join("plugins", pluginName))
	}
	value = filepath.ToSlash(filepath.Clean(value))
	if value == "." || value == "" {
		return "", fmt.Errorf("package root must stay below the marketplace root")
	}
	if strings.HasPrefix(value, "/") || value == ".." || strings.HasPrefix(value, "../") || strings.Contains(value, "/../") {
		return "", fmt.Errorf("package root must stay relative to the marketplace root")
	}
	return value, nil
}

func inspectionManagedPathsForTarget(inspection pluginmanifest.Inspection, target string) ([]string, error) {
	for _, item := range inspection.Targets {
		if item.Target == target {
			return append([]string(nil), item.ManagedArtifacts...), nil
		}
	}
	return nil, fmt.Errorf("inspect output does not include target %s", target)
}

func materializedPackageArtifacts(root, packageRoot string, managedPaths []string, generated pluginmanifest.RenderResult) ([]pluginmanifest.Artifact, error) {
	renderedBodies := make(map[string][]byte, len(generated.Artifacts))
	for _, artifact := range generated.Artifacts {
		renderedBodies[filepath.ToSlash(artifact.RelPath)] = artifact.Content
	}
	out := make([]pluginmanifest.Artifact, 0, len(managedPaths))
	for _, rel := range managedPaths {
		var body []byte
		if generated, ok := renderedBodies[rel]; ok {
			body = append([]byte(nil), generated...)
		} else {
			sourceBody, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("read managed package artifact %s: %w", rel, err)
			}
			body = sourceBody
		}
		out = append(out, pluginmanifest.Artifact{
			RelPath: filepath.ToSlash(filepath.Join(packageRoot, rel)),
			Content: body,
		})
	}
	slices.SortFunc(out, func(a, b pluginmanifest.Artifact) int { return strings.Compare(a.RelPath, b.RelPath) })
	return out, nil
}

func mergeCatalogAtDestination(dest, target string, generated pluginmanifest.Artifact) ([]byte, error) {
	full := filepath.Join(dest, filepath.FromSlash(generated.RelPath))
	existing, err := os.ReadFile(full)
	if err == nil {
		return publicationexec.MergeCatalogArtifact(target, existing, generated.Content)
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	return generated.Content, nil
}

func sortedSlashPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" {
			continue
		}
		out = append(out, path)
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
		return map[string]string{}
	}
	out := make(map[string]string, len(items))
	for key, value := range items {
		out[key] = value
	}
	return out
}

func appendUniquePublishSteps(steps []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(steps))
	for _, step := range steps {
		step = strings.TrimSpace(step)
		if step == "" {
			continue
		}
		if _, ok := seen[step]; ok {
			continue
		}
		seen[step] = struct{}{}
		out = append(out, step)
	}
	return out
}
