package pluginmanifest

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

type authoredLayout struct {
	RootRel string
}

func (l authoredLayout) IsCanonical() bool {
	return filepath.ToSlash(strings.TrimSpace(l.RootRel)) == pluginmodel.SourceDirName
}

func (l authoredLayout) Path(rel string) string {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" {
		return filepath.ToSlash(strings.TrimSpace(l.RootRel))
	}
	if strings.TrimSpace(l.RootRel) == "" {
		return rel
	}
	return filepath.ToSlash(filepath.Join(l.RootRel, rel))
}

func validateRemovedPortableInputs(root string, layout authoredLayout, targets []string) error {
	if removedPortableInputExists(root, layout, "agents") && !looksLikeManagedAgentsOutput(root, layout, targets) {
		return errors.New(rootAgentsMigrationMessage(targets))
	}
	if removedPortableInputExists(root, layout, "contexts") && !looksLikeManagedContextsOutput(root, layout, targets) {
		return errors.New(rootContextsMigrationMessage(targets))
	}
	return nil
}

func removedPortableInputExists(root string, layout authoredLayout, rel string) bool {
	candidates := []string{filepath.ToSlash(rel)}
	if canonical := layout.Path(rel); canonical != rel {
		candidates = append(candidates, filepath.ToSlash(canonical))
	}
	for _, candidate := range candidates {
		if authoredInputExists(root, candidate) {
			return true
		}
	}
	return false
}

func looksLikeManagedAgentsOutput(root string, layout authoredLayout, targets []string) bool {
	targetSet := setOf(targets)
	if !targetSet["claude"] {
		return false
	}
	return len(discoverFiles(root, layout.Path(filepath.Join("targets", "claude", "agents")), nil)) > 0
}

func looksLikeManagedContextsOutput(root string, layout authoredLayout, targets []string) bool {
	targetSet := setOf(targets)
	if targetSet["gemini"] && len(discoverFiles(root, layout.Path(filepath.Join("targets", "gemini", "contexts")), nil)) > 0 {
		return true
	}
	if targetSet["codex-runtime"] && len(discoverFiles(root, layout.Path(filepath.Join("targets", "codex-runtime", "contexts")), nil)) > 0 {
		return true
	}
	return false
}

func rootAgentsMigrationMessage(targets []string) string {
	targetSet := setOf(targets)
	switch {
	case targetSet["claude"]:
		return "portable agents were removed: move repo-root agents/ into targets/claude/agents/; Gemini agents remain preview-only and Codex lanes do not support agents"
	case targetSet["gemini"]:
		return "portable agents were removed: repo-root agents/ is no longer supported; Gemini agents remain preview-only in this wave"
	default:
		return "portable agents were removed: repo-root agents/ is no longer a canonical authored input"
	}
}

func rootContextsMigrationMessage(targets []string) string {
	targetSet := setOf(targets)
	var destinations []string
	if targetSet["gemini"] {
		destinations = append(destinations, "targets/gemini/contexts/")
	}
	if targetSet["codex-runtime"] {
		destinations = append(destinations, "targets/codex-runtime/contexts/")
	}
	if len(destinations) > 0 {
		return fmt.Sprintf("portable contexts were removed: move repo-root contexts/ into %s", strings.Join(destinations, " and/or "))
	}
	return "portable contexts were removed: repo-root contexts/ is no longer supported for these targets"
}

func authoredInputExists(root, rel string) bool {
	full := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(full)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return true
	}
	var hasFile bool
	_ = filepath.WalkDir(full, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		hasFile = true
		return io.EOF
	})
	return hasFile
}

func detectAuthoredLayout(root string) (authoredLayout, error) {
	canonical := authoredLayout{RootRel: pluginmodel.SourceDirName}
	if legacyRel := rootLegacyPortableMCPPath(root); legacyRel != "" {
		return authoredLayout{}, fmt.Errorf("unsupported portable MCP authored path %s: use src/mcp/servers.yaml", legacyRel)
	}
	canonicalPresent := authoredLayoutPresent(root, canonical)
	rootPresent := rootAuthoredLayoutPresent(root)
	switch {
	case canonicalPresent && rootPresent:
		return authoredLayout{}, fmt.Errorf("mixed authored layout: keep manual plugin sources only under %s/ and remove root-authored plugin files", pluginmodel.SourceDirName)
	case canonicalPresent:
		return canonical, nil
	case rootPresent:
		return authoredLayout{}, fmt.Errorf("unsupported authored layout: move manual plugin sources into %s/", pluginmodel.SourceDirName)
	default:
		return canonical, nil
	}
}

func rootLegacyPortableMCPPath(root string) string {
	for _, rel := range []string{
		filepath.ToSlash(filepath.Join("mcp", "servers.json")),
		filepath.ToSlash(filepath.Join("mcp", "servers.yml")),
	} {
		if authoredInputExists(root, rel) {
			return rel
		}
	}
	return ""
}

func authoredLayoutPresent(root string, layout authoredLayout) bool {
	for _, rel := range authoredSentinelPaths() {
		if authoredInputExists(root, layout.Path(rel)) {
			return true
		}
	}
	return false
}

func rootAuthoredLayoutPresent(root string) bool {
	for _, rel := range rootAuthoredSentinelPaths() {
		if authoredInputExists(root, rel) {
			return true
		}
	}
	return false
}

func authoredSentinelPaths() []string {
	return []string{
		FileName,
		LauncherFileName,
		filepath.ToSlash(filepath.Join("mcp", "servers.yaml")),
		"skills",
		"targets",
		"publish",
	}
}

func rootAuthoredSentinelPaths() []string {
	return []string{
		FileName,
		LauncherFileName,
		filepath.ToSlash(filepath.Join("mcp", "servers.yaml")),
		filepath.ToSlash(filepath.Join("mcp", "servers.yml")),
		filepath.ToSlash(filepath.Join("mcp", "servers.json")),
		"targets",
		"publish",
	}
}
