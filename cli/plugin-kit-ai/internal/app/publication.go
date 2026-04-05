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
	"github.com/777genius/plugin-kit-ai/cli/internal/publishschema"
	"github.com/777genius/plugin-kit-ai/cli/internal/repostate"
)

type PluginPublicationMaterializeOptions struct {
	Root        string
	Target      string
	Dest        string
	PackageRoot string
	DryRun      bool
}

type PluginPublicationMaterializeResult struct {
	Target            string            `json:"target"`
	Mode              string            `json:"mode"`
	MarketplaceFamily string            `json:"marketplace_family"`
	Dest              string            `json:"dest"`
	PackageRoot       string            `json:"package_root"`
	Details           map[string]string `json:"details"`
	NextSteps         []string          `json:"next_steps"`
	Lines             []string          `json:"-"`
}

type PluginPublicationRemoveOptions struct {
	Root        string
	Target      string
	Dest        string
	PackageRoot string
	DryRun      bool
}

type PluginPublicationRemoveResult struct {
	Lines []string
}

type PluginPublicationVerifyRootOptions struct {
	Root        string
	Target      string
	Dest        string
	PackageRoot string
}

type PluginPublicationRootIssue struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type PluginPublicationVerifyRootResult struct {
	Ready       bool                         `json:"ready"`
	Status      string                       `json:"status"`
	Dest        string                       `json:"dest"`
	PackageRoot string                       `json:"package_root"`
	CatalogPath string                       `json:"catalog_path"`
	IssueCount  int                          `json:"issue_count"`
	Issues      []PluginPublicationRootIssue `json:"issues"`
	NextSteps   []string                     `json:"next_steps"`
	Lines       []string                     `json:"-"`
}

type PluginPublishOptions struct {
	Root        string
	Channel     string
	Dest        string
	PackageRoot string
	DryRun      bool
	All         bool
}

type PluginPublishIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PluginPublishResult struct {
	Channel       string                `json:"channel,omitempty"`
	Target        string                `json:"target,omitempty"`
	Ready         bool                  `json:"ready"`
	Status        string                `json:"status"`
	Mode          string                `json:"mode"`
	WorkflowClass string                `json:"workflow_class"`
	Dest          string                `json:"dest,omitempty"`
	PackageRoot   string                `json:"package_root,omitempty"`
	Details       map[string]string     `json:"details"`
	IssueCount    int                   `json:"issue_count"`
	Issues        []PluginPublishIssue  `json:"issues"`
	WarningCount  int                   `json:"warning_count,omitempty"`
	Warnings      []string              `json:"warnings,omitempty"`
	NextSteps     []string              `json:"next_steps"`
	ChannelCount  int                   `json:"channel_count,omitempty"`
	Channels      []PluginPublishResult `json:"channels,omitempty"`
	Lines         []string              `json:"-"`
}

func (service PluginService) Publish(opts PluginPublishOptions) (PluginPublishResult, error) {
	if opts.All {
		return service.publishAll(opts)
	}
	channel := strings.TrimSpace(opts.Channel)
	if channel == "gemini-gallery" {
		return service.publishGeminiGallery(opts)
	}
	target, err := publishTargetForChannel(channel)
	if err != nil {
		return PluginPublishResult{}, err
	}
	result, err := service.PublicationMaterialize(PluginPublicationMaterializeOptions{
		Root:        opts.Root,
		Target:      target,
		Dest:        opts.Dest,
		PackageRoot: opts.PackageRoot,
		DryRun:      opts.DryRun,
	})
	if err != nil {
		return PluginPublishResult{}, err
	}
	lines := []string{
		fmt.Sprintf("Publish channel: %s", channel),
	}
	lines = append(lines, result.Lines...)
	return PluginPublishResult{
		Channel:       channel,
		Target:        result.Target,
		Ready:         true,
		Status:        "ready",
		Mode:          result.Mode,
		WorkflowClass: "local_marketplace_root",
		Dest:          result.Dest,
		PackageRoot:   result.PackageRoot,
		Details:       cloneStringMap(result.Details),
		IssueCount:    0,
		Issues:        []PluginPublishIssue{},
		WarningCount:  0,
		Warnings:      []string{},
		NextSteps:     cloneStrings(result.NextSteps),
		Lines:         lines,
	}, nil
}

func (service PluginService) publishAll(opts PluginPublishOptions) (PluginPublishResult, error) {
	if !opts.DryRun {
		return PluginPublishResult{}, fmt.Errorf("publish --all currently supports only --dry-run planning")
	}
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	inspection, _, err := pluginmanifest.Inspect(root, "all")
	if err != nil {
		return PluginPublishResult{}, err
	}
	channels := orderedPublicationChannels(inspection.Publication)
	if len(channels) == 0 {
		next := []string{
			"author at least one publication channel under publish/...",
			fmt.Sprintf("run plugin-kit-ai publication doctor %s", root),
		}
		lines := []string{
			"Publish selection: all authored channels",
			fmt.Sprintf("Mode: %s", publicationModeLabel(true)),
			"Channel count: 0",
			"Status: needs_channels (no authored publication channels exist under publish/...)",
			"Next:",
		}
		for _, step := range next {
			lines = append(lines, "  "+step)
		}
		return PluginPublishResult{
			Ready:         false,
			Status:        "needs_channels",
			Mode:          publicationModeLabel(true),
			WorkflowClass: "multi_channel_plan",
			Details:       map[string]string{},
			IssueCount:    0,
			Issues:        []PluginPublishIssue{},
			WarningCount:  0,
			Warnings:      []string{},
			NextSteps:     next,
			ChannelCount:  0,
			Channels:      []PluginPublishResult{},
			Lines:         lines,
		}, nil
	}
	if channelsNeedLocalDest(channels) && strings.TrimSpace(opts.Dest) == "" {
		return PluginPublishResult{}, fmt.Errorf("publish --all --dry-run requires --dest because authored publication channels include local marketplace roots")
	}

	var (
		results  []PluginPublishResult
		warnings []string
		next     []string
		ready    = true
	)
	if !channelsNeedLocalDest(channels) {
		if dest := strings.TrimSpace(opts.Dest); dest != "" {
			warnings = append(warnings, fmt.Sprintf("destination root %s is ignored because the authored publication channels are repository/release rooted", filepath.Clean(dest)))
		}
		if pkg := strings.TrimSpace(opts.PackageRoot); pkg != "" {
			warnings = append(warnings, fmt.Sprintf("package root %s is ignored because the authored publication channels are repository/release rooted", filepath.Clean(pkg)))
		}
	}
	for _, channel := range channels {
		result, err := service.Publish(PluginPublishOptions{
			Root:        root,
			Channel:     channel.Family,
			Dest:        opts.Dest,
			PackageRoot: opts.PackageRoot,
			DryRun:      true,
		})
		if err != nil {
			return PluginPublishResult{}, err
		}
		results = append(results, result)
		if !result.Ready {
			ready = false
		}
		next = appendUniquePublishSteps(append(next, result.NextSteps...))
	}

	status := "ready"
	if !ready {
		status = "needs_attention"
	}
	lines := []string{
		"Publish selection: all authored channels",
		fmt.Sprintf("Mode: %s", publicationModeLabel(true)),
		fmt.Sprintf("Channel count: %d", len(results)),
	}
	channelNames := make([]string, 0, len(results))
	for _, result := range results {
		channelNames = append(channelNames, result.Channel)
	}
	lines = append(lines, fmt.Sprintf("Authored channels: %s", strings.Join(channelNames, ", ")))
	if dest := strings.TrimSpace(opts.Dest); dest != "" && channelsNeedLocalDest(channels) {
		lines = append(lines, fmt.Sprintf("Destination root: %s", filepath.Clean(dest)))
	}
	for _, warning := range warnings {
		lines = append(lines, "Warning: "+warning)
	}
	for i, result := range results {
		lines = append(lines, fmt.Sprintf("Channel %d/%d: %s", i+1, len(results), result.Channel))
		lines = append(lines, fmt.Sprintf("  Status: %s", result.Status))
		lines = append(lines, fmt.Sprintf("  Workflow: %s", result.WorkflowClass))
		if result.Target != "" {
			lines = append(lines, fmt.Sprintf("  Target: %s", result.Target))
		}
		if result.PackageRoot != "" {
			lines = append(lines, fmt.Sprintf("  Package root: %s", result.PackageRoot))
		}
		for _, issue := range result.Issues {
			lines = append(lines, fmt.Sprintf("  Issue[%s]: %s", issue.Code, issue.Message))
		}
		for _, warning := range result.Warnings {
			lines = append(lines, "  Warning: "+warning)
		}
	}
	if ready {
		lines = append(lines, "Status: ready (every authored publication channel is ready for its bounded dry-run workflow)")
	} else {
		lines = append(lines, "Status: needs_attention (one or more authored publication channels still need follow-up)")
	}
	if len(next) > 0 {
		lines = append(lines, "Next:")
		for _, step := range next {
			lines = append(lines, "  "+step)
		}
	}
	return PluginPublishResult{
		Ready:         ready,
		Status:        status,
		Mode:          publicationModeLabel(true),
		WorkflowClass: "multi_channel_plan",
		Dest:          cleanedDestForMulti(opts.Dest, channels),
		Details:       map[string]string{},
		IssueCount:    0,
		Issues:        []PluginPublishIssue{},
		WarningCount:  len(warnings),
		Warnings:      cloneStrings(warnings),
		NextSteps:     next,
		ChannelCount:  len(results),
		Channels:      clonePublishResults(results),
		Lines:         lines,
	}, nil
}

func (service PluginService) publishGeminiGallery(opts PluginPublishOptions) (PluginPublishResult, error) {
	if !opts.DryRun {
		return PluginPublishResult{}, fmt.Errorf("publish channel %q currently supports only --dry-run planning; Gemini publication is repository/release rooted, not local-catalog rooted", "gemini-gallery")
	}
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	inspection, _, err := pluginmanifest.Inspect(root, "gemini")
	if err != nil {
		return PluginPublishResult{}, err
	}
	publication := inspection.Publication
	if _, ok := publicationPackageForTarget(publication, "gemini"); !ok {
		return PluginPublishResult{}, fmt.Errorf("target %s is not publication-capable", "gemini")
	}
	channel, ok := publicationChannelForFamily(publication, "gemini-gallery")
	if !ok {
		return PluginPublishResult{}, fmt.Errorf("target %s requires authored publication channel metadata under %s", "gemini", publishschema.GeminiGalleryRel)
	}
	status, issues, nextSteps := diagnoseGeminiPublishEnvironment(root, channel)
	lines := []string{
		"Publish channel: gemini-gallery",
		"Publish target: gemini",
		fmt.Sprintf("Mode: %s", publicationModeLabel(true)),
		fmt.Sprintf("Channel manifest: %s", channel.Path),
		fmt.Sprintf("Distribution: %s", channel.Details["distribution"]),
		fmt.Sprintf("Manifest root: %s", channel.Details["manifest_root"]),
		fmt.Sprintf("Repository visibility: %s", channel.Details["repository_visibility"]),
		fmt.Sprintf("GitHub topic: %s", channel.Details["github_topic"]),
		"Publication model: repository/release rooted (no local marketplace root is materialized)",
	}
	if dest := strings.TrimSpace(opts.Dest); dest != "" {
		lines = append(lines, fmt.Sprintf("Destination root ignored: %s", filepath.Clean(dest)))
	}
	for _, issue := range issues {
		lines = append(lines, fmt.Sprintf("Issue[%s]: %s", issue.Code, issue.Message))
	}
	if status == "ready" {
		lines = append(lines, "Status: ready (repository or release publication plan is consistent with the current workspace)")
	} else {
		lines = append(lines, "Status: needs_repository (repository context is not yet ready for Gemini gallery publication)")
	}
	lines = append(lines, "Next:")
	for _, step := range nextSteps {
		lines = append(lines, "  "+step)
	}
	return PluginPublishResult{
		Channel:       "gemini-gallery",
		Target:        "gemini",
		Ready:         status == "ready",
		Status:        status,
		Mode:          publicationModeLabel(true),
		WorkflowClass: "repository_release_plan",
		Details: map[string]string{
			"distribution":          channel.Details["distribution"],
			"manifest_root":         channel.Details["manifest_root"],
			"repository_visibility": channel.Details["repository_visibility"],
			"github_topic":          channel.Details["github_topic"],
			"publication_model":     "repository_or_release_rooted",
		},
		IssueCount: len(issues),
		Issues:     issues,
		NextSteps:  nextSteps,
		Lines:      lines,
	}, nil
}

func (PluginService) PublicationMaterialize(opts PluginPublicationMaterializeOptions) (PluginPublicationMaterializeResult, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	target := strings.TrimSpace(opts.Target)
	switch target {
	case "codex-package", "claude":
	default:
		return PluginPublicationMaterializeResult{}, fmt.Errorf("publication materialize supports only %q or %q", "codex-package", "claude")
	}
	dest := strings.TrimSpace(opts.Dest)
	if dest == "" {
		return PluginPublicationMaterializeResult{}, fmt.Errorf("publication materialize requires --dest")
	}

	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	if _, err := graph.Manifest.SelectedTargets(target); err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	publicationState, err := publishschema.Discover(root)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	inspection, _, err := pluginmanifest.Inspect(root, target)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	publication := inspection.Publication
	if _, ok := publicationPackageForTarget(publication, target); !ok {
		return PluginPublicationMaterializeResult{}, fmt.Errorf("target %s is not publication-capable", target)
	}
	channel, ok := publicationChannelForTarget(publication, target)
	if !ok {
		return PluginPublicationMaterializeResult{}, fmt.Errorf("target %s requires authored publication channel metadata under publish/...", target)
	}

	packageRoot, err := normalizePackageRoot(opts.PackageRoot, graph.Manifest.Name)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	rendered, err := pluginmanifest.Render(root, target)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	managedPaths, err := inspectionManagedPathsForTarget(inspection, target)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	managedPaths = append(managedPaths, graph.Portable.Paths("skills")...)
	managedPaths = slices.Compact(sortedSlashPaths(managedPaths))
	packageFiles, err := materializedPackageArtifacts(root, packageRoot, managedPaths, rendered)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	catalogArtifact, err := publicationexec.RenderLocalCatalogArtifact(graph, publicationState, target, "./"+packageRoot)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}
	mergedCatalog, err := mergeCatalogAtDestination(dest, target, catalogArtifact)
	if err != nil {
		return PluginPublicationMaterializeResult{}, err
	}

	destPackageRoot := filepath.Join(dest, filepath.FromSlash(packageRoot))
	packageRootAction := "create"
	if info, statErr := os.Stat(destPackageRoot); statErr == nil && info.IsDir() {
		packageRootAction = "replace"
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return PluginPublicationMaterializeResult{}, statErr
	}
	catalogAction := "create"
	catalogFull := filepath.Join(dest, filepath.FromSlash(catalogArtifact.RelPath))
	if _, statErr := os.Stat(catalogFull); statErr == nil {
		catalogAction = "merge"
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return PluginPublicationMaterializeResult{}, statErr
	}
	if !opts.DryRun {
		if err := os.RemoveAll(destPackageRoot); err != nil {
			return PluginPublicationMaterializeResult{}, err
		}
		if err := pluginmanifest.WriteArtifacts(dest, packageFiles); err != nil {
			return PluginPublicationMaterializeResult{}, err
		}
		if err := pluginmanifest.WriteArtifacts(dest, []pluginmanifest.Artifact{{
			RelPath: catalogArtifact.RelPath,
			Content: mergedCatalog,
		}}); err != nil {
			return PluginPublicationMaterializeResult{}, err
		}
	}

	nextSteps := []string{
		fmt.Sprintf("plugin-kit-ai publication doctor %s", root),
		fmt.Sprintf("plugin-kit-ai publication doctor %s --target %s --dest %s", root, target, dest),
		fmt.Sprintf("inspect %s with the vendor CLI from the marketplace root", channel.Family),
	}
	lines := []string{
		fmt.Sprintf("Materialized publication target: %s", target),
		fmt.Sprintf("Marketplace family: %s", channel.Family),
		fmt.Sprintf("Marketplace root: %s", filepath.Clean(dest)),
		fmt.Sprintf("Package root: %s", packageRoot),
		fmt.Sprintf("Mode: %s", publicationModeLabel(opts.DryRun)),
		fmt.Sprintf("Package root action: %s", packageRootAction),
		fmt.Sprintf("Package files: %d", len(packageFiles)),
		fmt.Sprintf("Catalog artifact action: %s %s", catalogAction, catalogArtifact.RelPath),
	}
	if len(rendered.StalePaths) > 0 {
		lines = append(lines, fmt.Sprintf("Source render drift observed: %d stale managed path(s) were bypassed by materializing fresh generated outputs", len(rendered.StalePaths)))
	}
	lines = append(lines, "Next:")
	for _, step := range nextSteps {
		lines = append(lines, "  "+step)
	}
	return PluginPublicationMaterializeResult{
		Target:            target,
		Mode:              publicationModeLabel(opts.DryRun),
		MarketplaceFamily: channel.Family,
		Dest:              filepath.Clean(dest),
		PackageRoot:       packageRoot,
		Details: map[string]string{
			"package_root_action":     packageRootAction,
			"package_file_count":      fmt.Sprintf("%d", len(packageFiles)),
			"catalog_artifact":        catalogArtifact.RelPath,
			"catalog_artifact_action": catalogAction,
		},
		NextSteps: nextSteps,
		Lines:     lines,
	}, nil
}

func (PluginService) PublicationRemove(opts PluginPublicationRemoveOptions) (PluginPublicationRemoveResult, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	target := strings.TrimSpace(opts.Target)
	switch target {
	case "codex-package", "claude":
	default:
		return PluginPublicationRemoveResult{}, fmt.Errorf("publication remove supports only %q or %q", "codex-package", "claude")
	}
	dest := strings.TrimSpace(opts.Dest)
	if dest == "" {
		return PluginPublicationRemoveResult{}, fmt.Errorf("publication remove requires --dest")
	}

	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return PluginPublicationRemoveResult{}, err
	}
	if _, err := graph.Manifest.SelectedTargets(target); err != nil {
		return PluginPublicationRemoveResult{}, err
	}
	inspection, _, err := pluginmanifest.Inspect(root, target)
	if err != nil {
		return PluginPublicationRemoveResult{}, err
	}
	publication := inspection.Publication
	if _, ok := publicationPackageForTarget(publication, target); !ok {
		return PluginPublicationRemoveResult{}, fmt.Errorf("target %s is not publication-capable", target)
	}
	channel, ok := publicationChannelForTarget(publication, target)
	if !ok {
		return PluginPublicationRemoveResult{}, fmt.Errorf("target %s requires authored publication channel metadata under publish/...", target)
	}
	packageRoot, err := normalizePackageRoot(opts.PackageRoot, graph.Manifest.Name)
	if err != nil {
		return PluginPublicationRemoveResult{}, err
	}

	removedPackage := false
	destPackageRoot := filepath.Join(dest, filepath.FromSlash(packageRoot))
	if _, err := os.Stat(destPackageRoot); err == nil {
		if !opts.DryRun {
			if err := os.RemoveAll(destPackageRoot); err != nil {
				return PluginPublicationRemoveResult{}, err
			}
		}
		removedPackage = true
	} else if !os.IsNotExist(err) {
		return PluginPublicationRemoveResult{}, err
	}

	catalogRel, err := publicationexec.CatalogArtifactPath(target)
	if err != nil {
		return PluginPublicationRemoveResult{}, err
	}
	removedCatalogEntry := false
	catalogFull := filepath.Join(dest, filepath.FromSlash(catalogRel))
	if existing, err := os.ReadFile(catalogFull); err == nil {
		updated, removed, err := publicationexec.RemoveCatalogArtifact(target, existing, graph.Manifest.Name)
		if err != nil {
			return PluginPublicationRemoveResult{}, err
		}
		if removed {
			if !opts.DryRun {
				if err := pluginmanifest.WriteArtifacts(dest, []pluginmanifest.Artifact{{
					RelPath: catalogRel,
					Content: updated,
				}}); err != nil {
					return PluginPublicationRemoveResult{}, err
				}
			}
			removedCatalogEntry = true
		}
	} else if !os.IsNotExist(err) {
		return PluginPublicationRemoveResult{}, err
	}

	lines := []string{
		fmt.Sprintf("Removed publication target: %s", target),
		fmt.Sprintf("Marketplace family: %s", channel.Family),
		fmt.Sprintf("Marketplace root: %s", filepath.Clean(dest)),
		fmt.Sprintf("Package root: %s", packageRoot),
		fmt.Sprintf("Mode: %s", publicationModeLabel(opts.DryRun)),
	}
	if removedPackage {
		lines = append(lines, "Package root action: remove")
	} else {
		lines = append(lines, "Package root action: no existing package root")
	}
	if removedCatalogEntry {
		lines = append(lines, fmt.Sprintf("Catalog artifact action: prune %s", catalogRel))
	} else {
		lines = append(lines, fmt.Sprintf("Catalog artifact action: no matching %q entry was present", graph.Manifest.Name))
	}
	lines = append(lines,
		"Next:",
		fmt.Sprintf("  plugin-kit-ai publication doctor %s --target %s --dest %s", root, target, dest),
		fmt.Sprintf("  review %s from the marketplace root if you keep additional plugins there", catalogRel),
	)
	return PluginPublicationRemoveResult{Lines: lines}, nil
}

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

func publishTargetForChannel(channel string) (string, error) {
	switch strings.TrimSpace(channel) {
	case "codex-marketplace":
		return "codex-package", nil
	case "claude-marketplace":
		return "claude", nil
	default:
		return "", fmt.Errorf("unsupported publish channel %q", channel)
	}
}

func (PluginService) PublicationVerifyRoot(opts PluginPublicationVerifyRootOptions) (PluginPublicationVerifyRootResult, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	target := strings.TrimSpace(opts.Target)
	switch target {
	case "codex-package", "claude":
	default:
		return PluginPublicationVerifyRootResult{}, fmt.Errorf("publication doctor local-root verification supports only %q or %q", "codex-package", "claude")
	}
	dest := strings.TrimSpace(opts.Dest)
	if dest == "" {
		return PluginPublicationVerifyRootResult{}, fmt.Errorf("publication doctor local-root verification requires --dest")
	}

	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	if _, err := graph.Manifest.SelectedTargets(target); err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	publicationState, err := publishschema.Discover(root)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	inspection, _, err := pluginmanifest.Inspect(root, target)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	packageRoot, err := normalizePackageRoot(opts.PackageRoot, graph.Manifest.Name)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	rendered, err := pluginmanifest.Render(root, target)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	managedPaths, err := inspectionManagedPathsForTarget(inspection, target)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	managedPaths = append(managedPaths, graph.Portable.Paths("skills")...)
	managedPaths = slices.Compact(sortedSlashPaths(managedPaths))
	expectedPackageFiles, err := materializedPackageArtifacts(root, packageRoot, managedPaths, rendered)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	generatedCatalog, err := publicationexec.RenderLocalCatalogArtifact(graph, publicationState, target, "./"+packageRoot)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}

	catalogRel, err := publicationexec.CatalogArtifactPath(target)
	if err != nil {
		return PluginPublicationVerifyRootResult{}, err
	}
	var issues []PluginPublicationRootIssue
	destPackageRoot := filepath.Join(dest, filepath.FromSlash(packageRoot))
	if info, err := os.Stat(destPackageRoot); err != nil || !info.IsDir() {
		issues = append(issues, PluginPublicationRootIssue{
			Code:    "missing_materialized_package_root",
			Path:    packageRoot,
			Message: fmt.Sprintf("materialized package root %s is missing", packageRoot),
		})
	}
	for _, artifact := range expectedPackageFiles {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(artifact.RelPath))); err != nil {
			if os.IsNotExist(err) {
				issues = append(issues, PluginPublicationRootIssue{
					Code:    "missing_materialized_package_artifact",
					Path:    artifact.RelPath,
					Message: fmt.Sprintf("materialized package artifact %s is missing", artifact.RelPath),
				})
				continue
			}
			return PluginPublicationVerifyRootResult{}, err
		}
	}
	catalogFull := filepath.Join(dest, filepath.FromSlash(catalogRel))
	if existing, err := os.ReadFile(catalogFull); err == nil {
		catalogIssues, err := publicationexec.DiagnoseCatalogArtifact(target, existing, generatedCatalog.Content, graph.Manifest.Name)
		if err != nil {
			return PluginPublicationVerifyRootResult{}, err
		}
		for _, issue := range catalogIssues {
			issues = append(issues, PluginPublicationRootIssue{
				Code:    issue.Code,
				Path:    issue.Path,
				Message: issue.Message,
			})
		}
	} else if os.IsNotExist(err) {
		issues = append(issues, PluginPublicationRootIssue{
			Code:    "missing_materialized_catalog_artifact",
			Path:    catalogRel,
			Message: fmt.Sprintf("materialized catalog artifact %s is missing", catalogRel),
		})
	} else {
		return PluginPublicationVerifyRootResult{}, err
	}

	lines := []string{
		fmt.Sprintf("Local marketplace root: %s", filepath.Clean(dest)),
		fmt.Sprintf("Package root: %s", packageRoot),
		fmt.Sprintf("Catalog artifact: %s", catalogRel),
	}
	nextSteps := []string{}
	status := "ready"
	ready := true
	if len(issues) > 0 {
		status = "needs_sync"
		ready = false
		for _, issue := range issues {
			lines = append(lines, fmt.Sprintf("Issue[%s]: %s", issue.Code, issue.Message))
		}
		nextSteps = []string{
			fmt.Sprintf("run plugin-kit-ai publication materialize %s --target %s --dest %s", root, target, dest),
		}
		lines = append(lines, "Status: needs_sync (materialized marketplace root is missing files or has drift)", "Next:")
		for _, step := range nextSteps {
			lines = append(lines, "  "+step)
		}
	} else {
		lines = append(lines,
			"Status: ready (materialized marketplace root is in sync)",
		)
	}
	return PluginPublicationVerifyRootResult{
		Ready:       ready,
		Status:      status,
		Dest:        filepath.Clean(dest),
		PackageRoot: packageRoot,
		CatalogPath: catalogRel,
		IssueCount:  len(issues),
		Issues:      issues,
		NextSteps:   nextSteps,
		Lines:       lines,
	}, nil
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

func geminiPublishPlanSteps(root string, channel publicationmodel.Channel) []string {
	steps := []string{fmt.Sprintf("run plugin-kit-ai publication doctor %s --target gemini", root)}
	switch channel.Details["distribution"] {
	case "github_release":
		steps = append(steps, "build a release archive that keeps gemini-extension.json at the archive root")
	default:
		steps = append(steps, "keep gemini-extension.json at the repository root for git-based installs and gallery indexing")
	}
	steps = append(steps, "use gemini extensions link <path> for live Gemini CLI verification before publishing")
	return steps
}

func diagnoseGeminiPublishEnvironment(root string, channel publicationmodel.Channel) (string, []PluginPublishIssue, []string) {
	repoIssues, repoSteps := diagnoseGeminiRepositoryContext(root, channel)
	issues := make([]PluginPublishIssue, 0, len(repoIssues))
	for _, issue := range repoIssues {
		issues = append(issues, PluginPublishIssue{Code: issue.Code, Message: issue.Message})
	}
	steps := append([]string{}, repoSteps...)
	steps = append(steps, geminiPublishPlanSteps(root, channel)...)
	status := "ready"
	if len(issues) > 0 {
		status = "needs_repository"
	}
	return status, issues, appendUniquePublishSteps(steps)
}

func diagnoseGeminiRepositoryContext(root string, channel publicationmodel.Channel) ([]PluginPublishIssue, []string) {
	state := repostate.Inspect(root)
	var issues []PluginPublishIssue
	var next []string
	if !state.GitAvailable {
		issues = append(issues, PluginPublishIssue{
			Code:    "gemini_git_cli_unavailable",
			Message: "git is unavailable, so repository-rooted Gemini gallery prerequisites cannot be verified",
		})
		next = append(next, "install git and rerun plugin-kit-ai publish --channel gemini-gallery --dry-run")
		return issues, next
	}
	if !state.InGitRepo {
		issues = append(issues, PluginPublishIssue{
			Code:    "gemini_git_repository_missing",
			Message: "Gemini gallery publication expects a Git repository, but the current workspace is not inside one",
		})
		next = append(next, "initialize a Git repository for this plugin before publishing to the Gemini gallery")
	}
	if !state.HasOriginRemote {
		issues = append(issues, PluginPublishIssue{
			Code:    "gemini_origin_remote_missing",
			Message: "Gemini gallery publication expects a GitHub-backed repository or release source, but no origin remote is configured",
		})
		next = append(next, "add a GitHub origin remote for this plugin repository before publishing")
	} else if !state.OriginIsGitHub {
		issues = append(issues, PluginPublishIssue{
			Code:    "gemini_origin_not_github",
			Message: fmt.Sprintf("Gemini gallery publication expects GitHub distribution metadata, but origin points to %s", state.OriginHost),
		})
		next = append(next, "move the publication remote to a public GitHub repository before publishing to the Gemini gallery")
	}
	if len(issues) == 0 {
		next = append(next, "confirm the GitHub repository stays public and tagged with the gemini-cli-extension topic")
	} else if channel.Details["distribution"] == "github_release" {
		next = append(next, "prepare a public GitHub repository first, then publish release archives from that repository")
	}
	return issues, next
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

func materializedPackageArtifacts(root, packageRoot string, managedPaths []string, rendered pluginmanifest.RenderResult) ([]pluginmanifest.Artifact, error) {
	renderedBodies := make(map[string][]byte, len(rendered.Artifacts))
	for _, artifact := range rendered.Artifacts {
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
