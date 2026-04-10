package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
)

func (service PluginService) publish(opts PluginPublishOptions) (PluginPublishResult, error) {
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
