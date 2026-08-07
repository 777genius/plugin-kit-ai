# Client compatibility

Verified on 2026-08-07. "Supported" comes from the Agent Plugins project;
"local test" records what was actually available on the test machine.

| Client | Agent Plugins 1.0 components | Local availability | Verification |
| --- | --- | --- | --- |
| ChatGPT and Codex | Skills; stdio and Streamable HTTP MCP | ChatGPT web/desktop and Codex CLI 0.144.1 | Codex install + tool/skill calls passed; ChatGPT development MCP connection created and Notion provider login reached |
| Cursor | Skills; stdio, Streamable HTTP, legacy SSE MCP | Cursor 3.9.16 | Local package load + pinned stdio MCP connection passed |
| VS Code | Skills; stdio, Streamable HTTP, legacy SSE MCP | Not installed | Official documentation only |
| GitHub Copilot | Skills; stdio, Streamable HTTP, legacy SSE MCP | Copilot CLI not installed | Official documentation only |
| Kiro | Skills; stdio, Streamable HTTP, legacy SSE MCP | Not installed | Official documentation only |

## Component-compatible, not claimed as Agent Plugins 1.0 clients

| Client | Locally detected | Boundary |
| --- | --- | --- |
| Claude Code 2.1.205 | Yes | Has its own plugin marketplace; individual skills and MCP servers may be adapted |
| Gemini CLI 0.36.0 | Yes | Has separate extensions, skills, and MCP commands |
| OpenCode 1.18.4 | Yes | Not listed by the Agent Plugins compatibility directory |

The repository does not claim that a package installs unchanged in clients that
are absent from the official compatibility directory.

Sanitized client evidence is committed under [`tests/e2e/results`](../tests/e2e/results).
