# Gemini Runtime Audit

This note records the current production-ready boundary for the Gemini Go runtime lane.

## Supported Runtime Surface

The Gemini Go runtime is considered production-ready for:

- `SessionStart`
- `SessionEnd`
- `BeforeModel`
- `AfterModel`
- `BeforeToolSelection`
- `BeforeAgent`
- `AfterAgent`
- `BeforeTool`
- `AfterTool`

## Runtime Evidence

The supported Gemini runtime is backed by:

- typed Go SDK surface in `sdk/gemini`
- descriptor-backed runtime metadata and generated support tables
- scaffolded Gemini Go runtime repos via `plugin-kit-ai init --platform gemini --runtime go`
- strict render/validate contract checks
- deterministic repo-local runtime gate via `make test-gemini-runtime`
- dedicated opt-in real CLI runtime smoke via `make test-gemini-runtime-live`

The deterministic Gemini smoke now covers:

- lifecycle input decoding
- runtime control semantics such as `deny`, `continue:false`, `systemMessage`, `clearContext`, `suppressOutput`
- runtime transform semantics such as request/response rewrite, tool selection config, turn-local context injection, tool-input rewrite, tool-result context, and tail tool calls
- tool payload observability including `tool_input`, `tool_response`, `mcp_context`, and `original_request_name`

The live Gemini runtime smoke uses an explicit tool-use prompt for the tool path. On current Gemini CLI builds this is materially more reliable than injecting `@README.md` content and hoping the model still chooses a tool call. The live gate also requires the Gemini JSON envelope to report `response: "OK"` plus non-zero tool-call stats for `read_file`, so the production check now validates both hook traces and the vendor-reported tool result.

## Promotion Rule

Any future Gemini runtime surface stays `public-beta` until it has:

- descriptor-backed metadata
- scaffold and validate alignment
- deterministic smoke coverage
- production docs alignment
- sufficient live evidence for the intended stable promise
