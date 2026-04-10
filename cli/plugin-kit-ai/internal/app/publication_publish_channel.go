package app

import (
	"fmt"
	"strings"
)

func (service PluginService) publishSelectedChannel(opts PluginPublishOptions) (PluginPublishResult, error) {
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
	return buildLocalPublishResult(channel, result), nil
}

func buildLocalPublishResult(channel string, result PluginPublicationMaterializeResult) PluginPublishResult {
	lines := []string{fmt.Sprintf("Publish channel: %s", channel)}
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
	}
}
