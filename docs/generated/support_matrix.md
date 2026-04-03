# Generated Support Matrix

This generated table is the canonical per-event runtime support contract for shipped runtime claims. Gemini runtime hooks appear here alongside Claude and Codex; packaging-only or workspace-config-only targets remain documented in SUPPORT.md.

| Platform | Event | Status | Maturity | Contract Class | V1 Target | Invocation | Carrier | Transport Modes | Scaffold | Validate | Capabilities | Live Test | Summary |
|----------|-------|--------|----------|----------------|-----------|------------|---------|-----------------|----------|----------|--------------|-----------|---------|
| claude | Stop | runtime_supported | stable | production-ready | true | argv_command_casefold | stdin_json | process | true | true | stop_gate | claude_cli | Claude Stop command hook |
| claude | PreToolUse | runtime_supported | stable | production-ready | true | argv_command_casefold | stdin_json | process | true | true | tool_gate | claude_cli | Claude PreToolUse command hook |
| claude | UserPromptSubmit | runtime_supported | stable | production-ready | true | argv_command_casefold | stdin_json | process | true | true | prompt_submit_gate | claude_cli | Claude UserPromptSubmit command hook |
| claude | SessionStart | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | session_start | claude_cli | Claude SessionStart beta hook |
| claude | SessionEnd | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | session_end | claude_cli | Claude SessionEnd beta hook |
| claude | Notification | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | notify | claude_cli | Claude Notification beta hook |
| claude | PostToolUse | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | post_tool | claude_cli | Claude PostToolUse beta hook |
| claude | PostToolUseFailure | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | post_tool_failure | claude_cli | Claude PostToolUseFailure beta hook |
| claude | PermissionRequest | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | permission_request | claude_cli | Claude PermissionRequest beta hook |
| claude | SubagentStart | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | subagent_start | claude_cli | Claude SubagentStart beta hook |
| claude | SubagentStop | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | subagent_stop | claude_cli | Claude SubagentStop beta hook |
| claude | PreCompact | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | pre_compact | claude_cli | Claude PreCompact beta hook |
| claude | Setup | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | setup | claude_cli | Claude Setup beta hook |
| claude | TeammateIdle | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | teammate_idle | claude_cli | Claude TeammateIdle beta hook |
| claude | TaskCompleted | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | task_completed | claude_cli | Claude TaskCompleted beta hook |
| claude | ConfigChange | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | config_change | claude_cli | Claude ConfigChange beta hook |
| claude | WorktreeCreate | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | worktree_create | claude_cli | Claude WorktreeCreate beta hook |
| claude | WorktreeRemove | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | worktree_remove | claude_cli | Claude WorktreeRemove beta hook |
| gemini | SessionStart | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | gemini_session_start | gemini_extension | Gemini SessionStart beta hook |
| gemini | SessionEnd | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | gemini_session_end | gemini_extension | Gemini SessionEnd beta hook |
| gemini | BeforeTool | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | gemini_before_tool | gemini_extension | Gemini BeforeTool beta hook |
| gemini | AfterTool | runtime_supported | beta | runtime-supported but not stable | false | argv_command_casefold | stdin_json | process | true | true | gemini_after_tool | gemini_extension | Gemini AfterTool beta hook |
| codex | Notify | runtime_supported | stable | production-ready | true | argv_command_casefold | argv_json | process | true | true | notify | codex_notify | Codex notify hook |
