package main

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	pluginkitai "github.com/777genius/plugin-kit-ai/sdk"
	"github.com/777genius/plugin-kit-ai/sdk/claude"
	"github.com/777genius/plugin-kit-ai/sdk/codex"
	"github.com/777genius/plugin-kit-ai/sdk/gemini"
)

// PLUGIN_KIT_AI_E2E_TRACE, when set to a file path, appends one JSON line per hook invocation (for CLI e2e).

func trace(rec map[string]any) {
	p := os.Getenv("PLUGIN_KIT_AI_E2E_TRACE")
	if p == "" {
		return
	}
	rec["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}

func geminiOverride(key string) string {
	return strings.TrimSpace(os.Getenv("PLUGIN_KIT_AI_E2E_GEMINI_" + key))
}

func geminiOverrideMessage(key string) (string, bool) {
	override := geminiOverride(key)
	if !strings.HasPrefix(override, "message:") {
		return "", false
	}
	return strings.TrimPrefix(override, "message:"), true
}

func geminiOverrideDeny(key string) (string, bool) {
	override := geminiOverride(key)
	if !strings.HasPrefix(override, "deny:") {
		return "", false
	}
	return strings.TrimPrefix(override, "deny:"), true
}

func geminiOverrideStop(key string) (string, bool) {
	override := geminiOverride(key)
	if !strings.HasPrefix(override, "stop:") {
		return "", false
	}
	return strings.TrimPrefix(override, "stop:"), true
}

func geminiOverrideContext(key string) (string, bool) {
	override := geminiOverride(key)
	if !strings.HasPrefix(override, "context:") {
		return "", false
	}
	return strings.TrimPrefix(override, "context:"), true
}

func main() {
	app := pluginkitai.New(pluginkitai.Config{Name: "plugin-kit-ai-e2e"})
	app.Claude().OnStop(func(*claude.StopEvent) *claude.Response {
		trace(map[string]any{"hook": "Stop", "outcome": "allow"})
		return claude.Allow()
	})
	app.Claude().OnPreToolUse(func(e *claude.PreToolUseEvent) *claude.PreToolResponse {
		var ti struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(e.ToolInput, &ti)
		// Optional: real Claude CLI e2e uses a benign Bash command; model refuses true rm -rf /.
		if sub := strings.TrimSpace(os.Getenv("PLUGIN_KIT_AI_E2E_PRETOOL_DENY_SUBSTRING")); sub != "" && strings.Contains(ti.Command, sub) {
			trace(map[string]any{"hook": "PreToolUse", "outcome": "deny", "command": ti.Command, "match": sub})
			return claude.PreToolDeny("blocked: plugin-kit-ai CLI integration marker")
		}
		if strings.Contains(ti.Command, "rm -rf /") {
			trace(map[string]any{"hook": "PreToolUse", "outcome": "deny", "command": ti.Command})
			return claude.PreToolDeny("dangerous")
		}
		trace(map[string]any{"hook": "PreToolUse", "outcome": "allow", "command": ti.Command})
		return claude.PreToolAllow()
	})
	app.Claude().OnUserPromptSubmit(func(e *claude.UserPromptEvent) *claude.UserPromptResponse {
		if strings.Contains(strings.ToLower(e.Prompt), "secret") {
			trace(map[string]any{"hook": "UserPromptSubmit", "outcome": "block"})
			return claude.UserPromptBlock("no secrets")
		}
		trace(map[string]any{"hook": "UserPromptSubmit", "outcome": "allow"})
		return claude.UserPromptAllow()
	})
	app.Codex().OnNotify(func(e *codex.NotifyEvent) *codex.Response {
		trace(map[string]any{
			"hook":     "Notify",
			"outcome":  "continue",
			"client":   e.Client,
			"raw_json": string(e.RawJSON()),
		})
		return codex.Continue()
	})
	app.Gemini().OnSessionStart(func(e *gemini.SessionStartEvent) *gemini.SessionStartResponse {
		if message, ok := geminiOverrideMessage("SESSION_START"); ok {
			trace(map[string]any{
				"hook":    "SessionStart",
				"outcome": "message",
				"source":  e.Source,
				"cwd":     e.CWD,
			})
			return gemini.SessionStartMessage(message)
		}
		trace(map[string]any{
			"hook":    "SessionStart",
			"outcome": "continue",
			"source":  e.Source,
			"cwd":     e.CWD,
		})
		return gemini.SessionStartContinue()
	})
	app.Gemini().OnSessionEnd(func(e *gemini.SessionEndEvent) *gemini.SessionEndResponse {
		if message, ok := geminiOverrideMessage("SESSION_END"); ok {
			trace(map[string]any{
				"hook":    "SessionEnd",
				"outcome": "message",
				"reason":  e.Reason,
				"cwd":     e.CWD,
			})
			return gemini.SessionEndMessage(message)
		}
		trace(map[string]any{
			"hook":    "SessionEnd",
			"outcome": "continue",
			"reason":  e.Reason,
			"cwd":     e.CWD,
		})
		return gemini.SessionEndContinue()
	})
	app.Gemini().OnBeforeTool(func(e *gemini.BeforeToolEvent) *gemini.BeforeToolResponse {
		rec := map[string]any{
			"hook":             "BeforeTool",
			"tool_name":        e.ToolName,
			"has_input":        strings.TrimSpace(string(e.ToolInput)) != "",
			"input_size":       len(e.ToolInput),
			"has_mcp_context":  strings.TrimSpace(string(e.MCPContext)) != "",
			"mcp_context_size": len(e.MCPContext),
		}
		if strings.TrimSpace(e.OriginalRequestName) != "" {
			rec["original_request_name"] = e.OriginalRequestName
		}
		if reason, ok := geminiOverrideDeny("BEFORE_TOOL"); ok {
			rec["outcome"] = "deny"
			trace(rec)
			return gemini.BeforeToolDeny(reason)
		}
		if geminiOverride("BEFORE_TOOL") == "rewrite_input" {
			rec["outcome"] = "rewrite_input"
			rec["rewrite_path"] = "README.md"
			trace(rec)
			resp, err := gemini.BeforeToolRewriteInputValue(map[string]any{
				"file_path": "README.md",
			})
			if err != nil {
				return gemini.BeforeToolDeny(err.Error())
			}
			return resp
		}
		rec["outcome"] = "continue"
		trace(rec)
		return gemini.BeforeToolContinue()
	})
	app.Gemini().OnAfterTool(func(e *gemini.AfterToolEvent) *gemini.AfterToolResponse {
		rec := map[string]any{
			"hook":             "AfterTool",
			"tool_name":        e.ToolName,
			"has_input":        strings.TrimSpace(string(e.ToolInput)) != "",
			"input_size":       len(e.ToolInput),
			"has_response":     strings.TrimSpace(string(e.ToolResponse)) != "",
			"response_size":    len(e.ToolResponse),
			"has_mcp_context":  strings.TrimSpace(string(e.MCPContext)) != "",
			"mcp_context_size": len(e.MCPContext),
		}
		if strings.TrimSpace(e.OriginalRequestName) != "" {
			rec["original_request_name"] = e.OriginalRequestName
		}
		if context, ok := geminiOverrideContext("AFTER_TOOL"); ok {
			rec["outcome"] = "add_context"
			trace(rec)
			return gemini.AfterToolAddContext(context)
		}
		if reason, ok := geminiOverrideStop("AFTER_TOOL"); ok {
			rec["outcome"] = "stop"
			trace(rec)
			return gemini.AfterToolStop(reason)
		}
		if geminiOverride("AFTER_TOOL") == "tailcall" {
			rec["outcome"] = "tail_call"
			trace(rec)
			resp, err := gemini.AfterToolTailCallValue("read_file", map[string]any{"file_path": "README.md"})
			if err != nil {
				return gemini.AfterToolDeny(err.Error())
			}
			return resp
		}
		rec["outcome"] = "continue"
		trace(rec)
		return gemini.AfterToolContinue()
	})
	os.Exit(app.Run())
}
