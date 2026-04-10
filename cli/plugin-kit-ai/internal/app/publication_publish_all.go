package app

import (
	"fmt"
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
	channels := plannedPublishAllChannels(inspection)
	if len(channels) == 0 {
		return emptyPublishPlan(root), nil
	}
	if err := validatePublishAllDest(opts, channels); err != nil {
		return PluginPublishResult{}, err
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
