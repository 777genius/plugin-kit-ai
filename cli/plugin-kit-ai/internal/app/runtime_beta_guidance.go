package app

import (
	"fmt"
	"strings"
)

func runtimeTestUnsupportedPlatformError(enabledTargets []string, requested string) error {
	if isGeminiBetaRuntimeTarget(requested, enabledTargets) {
		return fmt.Errorf("plugin-kit-ai test currently covers only stable runtime targets: claude or codex-runtime. Gemini has a production-ready Go runtime; use go test ./..., plugin-kit-ai render --check ., plugin-kit-ai validate . --platform gemini --strict, plugin-kit-ai inspect . --target gemini, plugin-kit-ai capabilities --mode runtime --platform gemini, make test-gemini-runtime, then gemini extensions link . and optionally make test-gemini-runtime-live")
	}
	return fmt.Errorf("test supports only launcher-based runtime targets: claude or codex-runtime")
}

func runtimeDevUnsupportedPlatformError(enabledTargets []string, requested string) error {
	if isGeminiBetaRuntimeTarget(requested, enabledTargets) {
		return fmt.Errorf("plugin-kit-ai dev currently covers only stable runtime targets: claude or codex-runtime. Gemini has a production-ready Go runtime; use plugin-kit-ai render ., plugin-kit-ai render --check ., plugin-kit-ai validate . --platform gemini --strict, plugin-kit-ai inspect . --target gemini, plugin-kit-ai capabilities --mode runtime --platform gemini, make test-gemini-runtime, then gemini extensions link . and optionally make test-gemini-runtime-live after changes")
	}
	return fmt.Errorf("dev supports only launcher-based runtime targets: claude or codex-runtime")
}

func isGeminiBetaRuntimeTarget(requested string, enabledTargets []string) bool {
	if strings.EqualFold(strings.TrimSpace(requested), "gemini") {
		return true
	}
	if strings.TrimSpace(requested) != "" {
		return false
	}
	for _, target := range enabledTargets {
		if strings.EqualFold(strings.TrimSpace(target), "gemini") {
			return true
		}
	}
	return false
}
