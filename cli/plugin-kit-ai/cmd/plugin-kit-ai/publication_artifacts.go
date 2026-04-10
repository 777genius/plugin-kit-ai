package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
)

func diagnosePublicationArtifacts(root, requestedTarget string, model publicationmodel.Model) []publicationIssue {
	var issues []publicationIssue
	for _, pkg := range model.Packages {
		if path := expectedPackageArtifactPath(pkg.Target); path != "" && !fileExists(filepath.Join(root, path)) {
			issues = append(issues, publicationIssue{
				Code:    "missing_package_artifact",
				Target:  pkg.Target,
				Path:    path,
				Message: fmt.Sprintf("target %s is missing generated package artifact %s", pkg.Target, path),
			})
		}
	}
	for _, channel := range model.Channels {
		if path := expectedChannelArtifactPath(channel.Family); path != "" && !fileExists(filepath.Join(root, path)) {
			issues = append(issues, publicationIssue{
				Code:          "missing_channel_artifact",
				ChannelFamily: channel.Family,
				Path:          path,
				Message:       fmt.Sprintf("channel %s is missing generated publication artifact %s", channel.Family, path),
			})
		}
	}
	if fileExists(filepath.Join(root, pluginmodel.SourceDirName, pluginmanifest.FileName)) ||
		fileExists(filepath.Join(root, pluginmodel.LegacySourceDirName, pluginmanifest.FileName)) {
		generated, err := pluginmanifest.Generate(root, normalizePublicationRequestedTarget(requestedTarget))
		if err != nil {
			issues = append(issues, publicationIssue{
				Code:    "generate_probe_failed",
				Path:    pluginmanifest.FileName,
				Message: fmt.Sprintf("publication doctor could not probe generated publication artifacts: %v", err),
			})
		} else {
			expectedBodies := make(map[string][]byte, len(generated.Artifacts))
			for _, artifact := range generated.Artifacts {
				expectedBodies[artifact.RelPath] = artifact.Content
			}
			for _, pkg := range model.Packages {
				if path := expectedPackageArtifactPath(pkg.Target); path != "" {
					if issue, ok := diagnosePublicationArtifactDrift(root, path, expectedBodies[path], "drifted_package_artifact"); ok {
						issue.Target = pkg.Target
						issues = append(issues, issue)
					}
				}
			}
			for _, channel := range model.Channels {
				if path := expectedChannelArtifactPath(channel.Family); path != "" {
					if issue, ok := diagnosePublicationArtifactDrift(root, path, expectedBodies[path], "drifted_channel_artifact"); ok {
						issue.ChannelFamily = channel.Family
						issues = append(issues, issue)
					}
				}
			}
			for _, path := range generated.StalePaths {
				if isPublicationRelevantPath(path) {
					issues = append(issues, publicationIssue{
						Code:    "stale_generated_artifact",
						Path:    path,
						Message: fmt.Sprintf("generated publication artifact %s is stale and should be removed by generate", path),
					})
				}
			}
		}
	}
	slices.SortFunc(issues, func(a, b publicationIssue) int {
		if cmp := strings.Compare(a.Code, b.Code); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.Target, b.Target); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.ChannelFamily, b.ChannelFamily); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Path, b.Path)
	})
	return issues
}

func normalizePublicationRequestedTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return "all"
	}
	return target
}

func expectedPackageArtifactPath(target string) string {
	switch target {
	case "codex-package":
		return filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json"))
	case "claude":
		return filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json"))
	case "gemini":
		return "gemini-extension.json"
	default:
		return ""
	}
}

func expectedChannelArtifactPath(family string) string {
	switch family {
	case "codex-marketplace":
		return filepath.ToSlash(filepath.Join(".agents", "plugins", "marketplace.json"))
	case "claude-marketplace":
		return filepath.ToSlash(filepath.Join(".claude-plugin", "marketplace.json"))
	default:
		return ""
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func diagnosePublicationArtifactDrift(root, path string, expected []byte, code string) (publicationIssue, bool) {
	if len(expected) == 0 || !fileExists(filepath.Join(root, path)) {
		return publicationIssue{}, false
	}
	current, err := os.ReadFile(filepath.Join(root, path))
	if err != nil || bytes.Equal(current, expected) {
		return publicationIssue{}, false
	}
	return publicationIssue{
		Code:    code,
		Path:    path,
		Message: fmt.Sprintf("generated publication artifact %s is out of sync with current authored inputs", path),
	}, true
}

func isPublicationRelevantPath(path string) bool {
	switch filepath.ToSlash(filepath.Clean(path)) {
	case filepath.ToSlash(filepath.Join(".agents", "plugins", "marketplace.json")),
		filepath.ToSlash(filepath.Join(".claude-plugin", "marketplace.json")),
		filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
		filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
		"gemini-extension.json":
		return true
	default:
		return false
	}
}
