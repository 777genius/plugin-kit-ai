package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
)

func (service PluginService) planPublishAll(opts PluginPublishOptions) (PluginPublishResult, error) {
	root, err := validatePublishAllOptions(opts)
	if err != nil {
		return PluginPublishResult{}, err
	}
	inspection, _, err := pluginmanifest.Inspect(root, "all")
	if err != nil {
		return PluginPublishResult{}, err
	}
	channels := orderedPublicationChannels(inspection.Publication)
	if len(channels) == 0 {
		return emptyPublishPlan(root), nil
	}
	if channelsNeedLocalDest(channels) && strings.TrimSpace(opts.Dest) == "" {
		return PluginPublishResult{}, fmt.Errorf("publish --all --dry-run requires --dest because authored publication channels include local marketplace roots")
	}
	results, warnings, next, ready, err := service.runPublishPlan(root, opts, channels)
	if err != nil {
		return PluginPublishResult{}, err
	}
	return buildPublishPlanResult(opts, channels, results, warnings, next, ready), nil
}

func validatePublishAllOptions(opts PluginPublishOptions) (string, error) {
	if !opts.DryRun {
		return "", fmt.Errorf("publish --all currently supports only --dry-run planning")
	}
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	return root, nil
}

func emptyPublishPlan(root string) PluginPublishResult {
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
	}
}

func (service PluginService) runPublishPlan(root string, opts PluginPublishOptions, channels []publicationmodel.Channel) ([]PluginPublishResult, []string, []string, bool, error) {
	var (
		results  []PluginPublishResult
		warnings []string
		next     []string
		ready    = true
	)
	if !channelsNeedLocalDest(channels) {
		warnings = append(warnings, ignoredPublishPlanWarnings(opts)...)
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
			return nil, nil, nil, false, err
		}
		results = append(results, result)
		if !result.Ready {
			ready = false
		}
		next = appendUniquePublishSteps(append(next, result.NextSteps...))
	}
	return results, warnings, next, ready, nil
}

func ignoredPublishPlanWarnings(opts PluginPublishOptions) []string {
	var warnings []string
	if dest := strings.TrimSpace(opts.Dest); dest != "" {
		warnings = append(warnings, fmt.Sprintf("destination root %s is ignored because the authored publication channels are repository/release rooted", filepath.Clean(dest)))
	}
	if pkg := strings.TrimSpace(opts.PackageRoot); pkg != "" {
		warnings = append(warnings, fmt.Sprintf("package root %s is ignored because the authored publication channels are repository/release rooted", filepath.Clean(pkg)))
	}
	return warnings
}

func buildPublishPlanResult(opts PluginPublishOptions, channels []publicationmodel.Channel, results []PluginPublishResult, warnings []string, next []string, ready bool) PluginPublishResult {
	status := "ready"
	if !ready {
		status = "needs_attention"
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
		Lines:         publishPlanLines(results, warnings, next, ready, opts.Dest, channels),
	}
}

func publishPlanLines(results []PluginPublishResult, warnings []string, next []string, ready bool, dest string, channels []publicationmodel.Channel) []string {
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
	if cleaned := strings.TrimSpace(dest); cleaned != "" && channelsNeedLocalDest(channels) {
		lines = append(lines, fmt.Sprintf("Destination root: %s", filepath.Clean(cleaned)))
	}
	for _, warning := range warnings {
		lines = append(lines, "Warning: "+warning)
	}
	lines = append(lines, publishPlanChannelLines(results)...)
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
	return lines
}

func publishPlanChannelLines(results []PluginPublishResult) []string {
	lines := make([]string, 0, len(results)*6)
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
	return lines
}
