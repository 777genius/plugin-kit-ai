package app

import (
	"fmt"
	"strings"

	pluginkitai "github.com/777genius/plugin-kit-ai/sdk"
)

func resolveRuntimeTestPlatform(enabledTargets []string, requested string) (string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested != "" {
		if !isRuntimeTestPlatform(requested) {
			return "", runtimeTestUnsupportedPlatformError(enabledTargets, requested)
		}
		for _, target := range enabledTargets {
			if target == requested {
				return requested, nil
			}
		}
		return "", fmt.Errorf("plugin.yaml does not enable target %q", requested)
	}

	var candidates []string
	for _, target := range enabledTargets {
		if isRuntimeTestPlatform(target) {
			candidates = append(candidates, target)
		}
	}
	switch len(candidates) {
	case 0:
		return "", runtimeTestUnsupportedPlatformError(enabledTargets, requested)
	case 1:
		return candidates[0], nil
	default:
		return "", fmt.Errorf("test requires --platform when multiple launcher-based runtime targets are enabled (%s)", strings.Join(candidates, ", "))
	}
}

func isRuntimeTestPlatform(platform string) bool {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "claude", "codex-runtime":
		return true
	default:
		return false
	}
}

func stableRuntimeSupport(target string) []runtimeTestSupport {
	target = strings.ToLower(strings.TrimSpace(target))
	out := make([]runtimeTestSupport, 0, 4)
	for _, entry := range pluginkitai.Supported() {
		if mapSupportPlatformToTarget(string(entry.Platform)) != target {
			continue
		}
		if string(entry.Status) != "runtime_supported" || string(entry.Maturity) != "stable" {
			continue
		}
		out = append(out, runtimeTestSupport{
			Platform: target,
			Event:    string(entry.Event),
			Carrier:  string(entry.Carrier),
		})
	}
	return out
}

func mapSupportPlatformToTarget(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "codex":
		return "codex-runtime"
	default:
		return strings.ToLower(strings.TrimSpace(platform))
	}
}

func selectRuntimeTestCases(supported []runtimeTestSupport, requestedEvent string, all bool) ([]runtimeTestSupport, error) {
	if all {
		if strings.TrimSpace(requestedEvent) != "" {
			return nil, fmt.Errorf("--event cannot be used with --all")
		}
		return append([]runtimeTestSupport(nil), supported...), nil
	}
	requestedEvent = strings.TrimSpace(requestedEvent)
	if requestedEvent == "" {
		if len(supported) == 1 {
			return []runtimeTestSupport{supported[0]}, nil
		}
		names := make([]string, 0, len(supported))
		for _, item := range supported {
			names = append(names, item.Event)
		}
		return nil, fmt.Errorf("test requires --event or --all; supported stable events: %s", strings.Join(names, ", "))
	}
	for _, item := range supported {
		if strings.EqualFold(item.Event, requestedEvent) {
			return []runtimeTestSupport{item}, nil
		}
	}
	names := make([]string, 0, len(supported))
	for _, item := range supported {
		names = append(names, item.Event)
	}
	return nil, fmt.Errorf("unsupported stable event %q for %s; supported: %s", requestedEvent, supported[0].Platform, strings.Join(names, ", "))
}
