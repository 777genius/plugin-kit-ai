# Generated Support Matrix

This generated table is the canonical per-event support contract for shipped runtime claims.

| Platform | Event | Status | Maturity | V1 Target | Invocation | Carrier | Transport Modes | Scaffold | Validate | Capabilities | Live Test | Summary |
|----------|-------|--------|----------|-----------|------------|---------|-----------------|----------|----------|--------------|-----------|---------|
| claude | Stop | runtime_supported | beta | true | argv_command_casefold | stdin_json | process | true | true | stop_gate | claude_cli | Claude Stop command hook |
| claude | PreToolUse | runtime_supported | beta | true | argv_command_casefold | stdin_json | process | true | true | tool_gate | claude_cli | Claude PreToolUse command hook |
| claude | UserPromptSubmit | runtime_supported | beta | true | argv_command_casefold | stdin_json | process | true | true | prompt_submit_gate | claude_cli | Claude UserPromptSubmit command hook |
| claude | SessionStart | runtime_supported | beta | false | argv_command_casefold | stdin_json | process | true | true | session_start | claude_cli | Claude SessionStart hook |
| claude | SessionEnd | runtime_supported | beta | false | argv_command_casefold | stdin_json | process | true | true | session_end | claude_cli | Claude SessionEnd hook |
| claude | Notification | runtime_supported | beta | false | argv_command_casefold | stdin_json | process | true | true | notification | claude_cli | Claude Notification hook |
| claude | PostToolUse | runtime_supported | beta | false | argv_command_casefold | stdin_json | process | true | true | posttooluse | claude_cli | Claude PostToolUse hook |
| claude | PostToolUseFailure | runtime_supported | beta | false | argv_command_casefold | stdin_json | process | true | true | posttooluse_failure | claude_cli | Claude PostToolUseFailure hook |
| claude | PermissionRequest | runtime_supported | beta | false | argv_command_casefold | stdin_json | process | true | true | permission_request | claude_cli | Claude PermissionRequest hook |
| claude | SubagentStart | runtime_supported | beta | false | argv_command_casefold | stdin_json | process | true | true | subagent_start | claude_cli | Claude SubagentStart hook |
| claude | SubagentStop | runtime_supported | beta | false | argv_command_casefold | stdin_json | process | true | true | subagent_stop | claude_cli | Claude SubagentStop hook |
| claude | PreCompact | runtime_supported | beta | false | argv_command_casefold | stdin_json | process | true | true | pre_compact | claude_cli | Claude PreCompact hook |
| claude | Setup | runtime_supported | beta | false | argv_command_casefold | stdin_json | process | true | true | setup | claude_cli | Claude Setup hook |
| claude | TeammateIdle | runtime_supported | beta | false | argv_command_casefold | stdin_json | process | true | true | teammate_idle | claude_cli | Claude TeammateIdle hook |
| claude | TaskCompleted | runtime_supported | beta | false | argv_command_casefold | stdin_json | process | true | true | task_completed | claude_cli | Claude TaskCompleted hook |
| claude | ConfigChange | runtime_supported | beta | false | argv_command_casefold | stdin_json | process | true | true | config_change | claude_cli | Claude ConfigChange hook |
| claude | WorktreeCreate | runtime_supported | beta | false | argv_command_casefold | stdin_json | process | true | true | worktree_create | claude_cli | Claude WorktreeCreate hook |
| claude | WorktreeRemove | runtime_supported | beta | false | argv_command_casefold | stdin_json | process | true | true | worktree_remove | claude_cli | Claude WorktreeRemove hook |
| codex | Notify | runtime_supported | beta | true | argv_command | argv_json | process | true | true | notify | codex_notify | Codex notify hook |
