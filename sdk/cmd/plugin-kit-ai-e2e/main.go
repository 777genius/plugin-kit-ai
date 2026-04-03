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

func geminiOverrideDenyOnce(key string) (string, bool) {
	override := geminiOverride(key)
	if !strings.HasPrefix(override, "deny_once:") {
		return "", false
	}
	return strings.TrimPrefix(override, "deny_once:"), true
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
	app.Gemini().OnBeforeModel(func(e *gemini.BeforeModelEvent) *gemini.BeforeModelResponse {
		if reason, ok := geminiOverrideDeny("BEFORE_MODEL"); ok {
			trace(map[string]any{
				"hook":         "BeforeModel",
				"outcome":      "deny",
				"has_request":  strings.TrimSpace(string(e.LLMRequest)) != "",
				"request_size": len(e.LLMRequest),
			})
			return gemini.BeforeModelDeny(reason)
		}
		switch geminiOverride("BEFORE_MODEL") {
		case "rewrite_request":
			trace(map[string]any{
				"hook":         "BeforeModel",
				"outcome":      "rewrite_request",
				"has_request":  strings.TrimSpace(string(e.LLMRequest)) != "",
				"request_size": len(e.LLMRequest),
			})
			resp, err := gemini.BeforeModelOverrideRequestValue(map[string]any{
				"model": "gemini-2.5-pro",
				"config": map[string]any{
					"temperature": 0.1,
				},
			})
			if err != nil {
				return gemini.BeforeModelDeny(err.Error())
			}
			return resp
		case "synthetic_response":
			trace(map[string]any{
				"hook":         "BeforeModel",
				"outcome":      "synthetic_response",
				"has_request":  strings.TrimSpace(string(e.LLMRequest)) != "",
				"request_size": len(e.LLMRequest),
			})
			resp, err := gemini.BeforeModelSyntheticResponseValue(map[string]any{
				"candidates": []any{
					map[string]any{
						"content": map[string]any{
							"role":  "model",
							"parts": []any{"synthetic"},
						},
					},
				},
			})
			if err != nil {
				return gemini.BeforeModelDeny(err.Error())
			}
			return resp
		}
		trace(map[string]any{
			"hook":         "BeforeModel",
			"outcome":      "continue",
			"has_request":  strings.TrimSpace(string(e.LLMRequest)) != "",
			"request_size": len(e.LLMRequest),
		})
		return gemini.BeforeModelContinue()
	})
	app.Gemini().OnAfterModel(func(e *gemini.AfterModelEvent) *gemini.AfterModelResponse {
		if reason, ok := geminiOverrideStop("AFTER_MODEL"); ok {
			trace(map[string]any{
				"hook":          "AfterModel",
				"outcome":       "stop",
				"has_request":   strings.TrimSpace(string(e.LLMRequest)) != "",
				"request_size":  len(e.LLMRequest),
				"has_response":  strings.TrimSpace(string(e.LLMResponse)) != "",
				"response_size": len(e.LLMResponse),
			})
			return gemini.AfterModelStop(reason)
		}
		if geminiOverride("AFTER_MODEL") == "replace_response" {
			trace(map[string]any{
				"hook":          "AfterModel",
				"outcome":       "replace_response",
				"has_request":   strings.TrimSpace(string(e.LLMRequest)) != "",
				"request_size":  len(e.LLMRequest),
				"has_response":  strings.TrimSpace(string(e.LLMResponse)) != "",
				"response_size": len(e.LLMResponse),
			})
			resp, err := gemini.AfterModelReplaceResponseValue(map[string]any{
				"candidates": []any{
					map[string]any{
						"content": map[string]any{
							"role":  "model",
							"parts": []any{"rewritten"},
						},
					},
				},
			})
			if err != nil {
				return gemini.AfterModelDeny(err.Error())
			}
			return resp
		}
		trace(map[string]any{
			"hook":          "AfterModel",
			"outcome":       "continue",
			"has_request":   strings.TrimSpace(string(e.LLMRequest)) != "",
			"request_size":  len(e.LLMRequest),
			"has_response":  strings.TrimSpace(string(e.LLMResponse)) != "",
			"response_size": len(e.LLMResponse),
		})
		return gemini.AfterModelContinue()
	})
	app.Gemini().OnBeforeToolSelection(func(e *gemini.BeforeToolSelectionEvent) *gemini.BeforeToolSelectionResponse {
		if geminiOverride("BEFORE_TOOL_SELECTION") == "quiet" {
			trace(map[string]any{
				"hook":         "BeforeToolSelection",
				"outcome":      "quiet",
				"has_request":  strings.TrimSpace(string(e.LLMRequest)) != "",
				"request_size": len(e.LLMRequest),
			})
			return gemini.BeforeToolSelectionQuiet()
		}
		switch geminiOverride("BEFORE_TOOL_SELECTION") {
		case "allow_only":
			trace(map[string]any{
				"hook":         "BeforeToolSelection",
				"outcome":      "allow_only",
				"has_request":  strings.TrimSpace(string(e.LLMRequest)) != "",
				"request_size": len(e.LLMRequest),
			})
			return gemini.BeforeToolSelectionAllowOnly("read_file", "list_directory")
		case "force_any":
			trace(map[string]any{
				"hook":         "BeforeToolSelection",
				"outcome":      "force_any",
				"has_request":  strings.TrimSpace(string(e.LLMRequest)) != "",
				"request_size": len(e.LLMRequest),
			})
			return gemini.BeforeToolSelectionForceAny("read_file")
		case "disable_all":
			trace(map[string]any{
				"hook":         "BeforeToolSelection",
				"outcome":      "disable_all",
				"has_request":  strings.TrimSpace(string(e.LLMRequest)) != "",
				"request_size": len(e.LLMRequest),
			})
			return gemini.BeforeToolSelectionDisableAll()
		}
		trace(map[string]any{
			"hook":         "BeforeToolSelection",
			"outcome":      "continue",
			"has_request":  strings.TrimSpace(string(e.LLMRequest)) != "",
			"request_size": len(e.LLMRequest),
		})
		return gemini.BeforeToolSelectionContinue()
	})
	app.Gemini().OnBeforeAgent(func(e *gemini.BeforeAgentEvent) *gemini.BeforeAgentResponse {
		if context, ok := geminiOverrideContext("BEFORE_AGENT"); ok {
			trace(map[string]any{
				"hook":    "BeforeAgent",
				"outcome": "add_context",
				"prompt":  e.Prompt,
			})
			return gemini.BeforeAgentAddContext(context)
		}
		trace(map[string]any{
			"hook":    "BeforeAgent",
			"outcome": "continue",
			"prompt":  e.Prompt,
		})
		return gemini.BeforeAgentContinue()
	})
	app.Gemini().OnAfterAgent(func(e *gemini.AfterAgentEvent) *gemini.AfterAgentResponse {
		if geminiOverride("AFTER_AGENT") == "clearcontext" {
			trace(map[string]any{
				"hook":             "AfterAgent",
				"outcome":          "clear_context",
				"prompt":           e.Prompt,
				"has_response":     strings.TrimSpace(e.PromptResponse) != "",
				"stop_hook_active": e.StopHookActive,
			})
			return gemini.AfterAgentClearContext()
		}
		if reason, ok := geminiOverrideDenyOnce("AFTER_AGENT"); ok {
			if e.StopHookActive {
				trace(map[string]any{
					"hook":             "AfterAgent",
					"outcome":          "continue",
					"prompt":           e.Prompt,
					"has_response":     strings.TrimSpace(e.PromptResponse) != "",
					"stop_hook_active": e.StopHookActive,
				})
				return gemini.AfterAgentContinue()
			}
			trace(map[string]any{
				"hook":             "AfterAgent",
				"outcome":          "deny",
				"prompt":           e.Prompt,
				"has_response":     strings.TrimSpace(e.PromptResponse) != "",
				"stop_hook_active": e.StopHookActive,
			})
			return gemini.AfterAgentDeny(reason)
		}
		if reason, ok := geminiOverrideDeny("AFTER_AGENT"); ok {
			trace(map[string]any{
				"hook":             "AfterAgent",
				"outcome":          "deny",
				"prompt":           e.Prompt,
				"has_response":     strings.TrimSpace(e.PromptResponse) != "",
				"stop_hook_active": e.StopHookActive,
			})
			return gemini.AfterAgentDeny(reason)
		}
		trace(map[string]any{
			"hook":             "AfterAgent",
			"outcome":          "continue",
			"prompt":           e.Prompt,
			"has_response":     strings.TrimSpace(e.PromptResponse) != "",
			"stop_hook_active": e.StopHookActive,
		})
		return gemini.AfterAgentContinue()
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
