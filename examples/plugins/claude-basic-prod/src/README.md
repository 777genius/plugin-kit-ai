# claude-basic-prod

Reference Claude plugin repo for the current `plugin-kit-ai` production workflow.

Stable runtime promise:

- `Stop`
- `PreToolUse`
- `UserPromptSubmit`

This example matches the stable-default Claude scaffold. It does not opt into the extended hook set.
It also exercises the first-class Claude `settings.json` surface through `src/targets/claude/settings.json`.

## Workflow

```bash
plugin-kit-ai normalize .
plugin-kit-ai generate .
plugin-kit-ai generate --check .
plugin-kit-ai validate . --platform claude --strict
go test ./...
go build -o bin/claude-basic-prod ./cmd/claude-basic-prod
printf '%s' '{"session_id":"s","cwd":"/tmp","hook_event_name":"Stop"}' | ./bin/claude-basic-prod Stop
printf '%s' '{"session_id":"e2e-session","transcript_path":"/tmp/t.jsonl","cwd":"/tmp","permission_mode":"default","hook_event_name":"PreToolUse","tool_name":"Bash","tool_use_id":"toolu_e2e","tool_input":{"command":"echo ok"}}' | ./bin/claude-basic-prod PreToolUse
printf '%s' '{"session_id":"e2e-session","transcript_path":"/tmp/t.jsonl","cwd":"/tmp","permission_mode":"default","hook_event_name":"UserPromptSubmit","prompt":"hello e2e"}' | ./bin/claude-basic-prod UserPromptSubmit
```
