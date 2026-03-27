package runtime

import (
	"fmt"
	"strings"
)

func CanonicalInvocationName(platform PlatformID, raw string) string {
	raw = strings.TrimSpace(raw)
	if platform != "claude" {
		return raw
	}
	switch {
	case strings.EqualFold(raw, "Stop"):
		return "Stop"
	case strings.EqualFold(raw, "PreToolUse"):
		return "PreToolUse"
	case strings.EqualFold(raw, "UserPromptSubmit"):
		return "UserPromptSubmit"
	default:
		return raw
	}
}

func attachMismatchWarning(res Result, rawName, decodedName string) Result {
	decodedName = strings.TrimSpace(decodedName)
	if decodedName == "" {
		return res
	}
	if strings.EqualFold(decodedName, strings.TrimSpace(rawName)) {
		return res
	}
	res.Stderr = fmt.Sprintf("plugin-kit-ai: hook_event_name %q does not match argv hook %q; using argv\n", decodedName, rawName) + res.Stderr
	return res
}
