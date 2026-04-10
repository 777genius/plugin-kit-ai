package app

import (
	"fmt"
	"strings"
)

func (service PluginService) publish(opts PluginPublishOptions) (PluginPublishResult, error) {
	if opts.All {
		return service.publishAll(opts)
	}
	return service.publishSelectedChannel(opts)
}

func (service PluginService) publishAll(opts PluginPublishOptions) (PluginPublishResult, error) {
	return service.planPublishAll(opts)
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
