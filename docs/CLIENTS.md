# Client compatibility

Verified on 2026-08-10. "Supported" comes from the Agent Plugins project;
"local test" records what was actually available on the test machine.

| Client | Agent Plugins 1.0 components | Local availability | Verification |
| --- | --- | --- | --- |
| Codex | Skills; stdio and Streamable HTTP MCP | Codex CLI 0.147.0 | Automated public marketplace install + Context7 tool call; separate Figma OAuth/read-only runtime passed |
| ChatGPT | Skills and registered Streamable HTTP MCP app bindings; no stdio | ChatGPT web/desktop | Cloudflare Docs registered personal app was Installed under Personal, activated manually with user-attested UI evidence, and passed read-only runtime; local `.codex-plugin` ingestion remains unproved |
| Cursor | Skills; stdio, Streamable HTTP, legacy SSE MCP | Cursor 3.9.16 | Local package load + pinned stdio MCP connection passed; no marketplace-install claim |
| VS Code | Skills; stdio, Streamable HTTP, legacy SSE MCP | Not installed | Shares the managed Copilot plugin when Copilot CLI is available; VS Code UI runtime remains separately untested |
| GitHub Copilot | Skills; stdio, Streamable HTTP, legacy SSE MCP | Copilot CLI 1.0.78 | Five hero packages passed automatic marketplace add, plugin install, verification, uninstall, and marketplace cleanup in an isolated profile |
| Kiro | Skills; stdio, Streamable HTTP, legacy SSE MCP | Kiro IDE 1.0.288 | Local folder import, power activation, and Context7 resolve/query calls passed in a disposable project |

The compatibility directory describes client support for the standard. It is
not proof that this repository was installed in every listed client. Marketplace
and directory manifests are client-owned adapters, not portable 1.0 files.

OpenAI delivery is intentionally split: five stdio MCP packages are Codex-only,
twenty remote MCP packages require registered ChatGPT app bindings, and the one
skills-only package has no MCP transport. Only Cloudflare Docs currently has a
repository binding and a tested registered personal app. The app ID linkage is
proved; repository marketplace ingestion and manager lifecycle are not.

## Component-compatible, not claimed as Agent Plugins 1.0 clients

| Client | Locally detected | Boundary |
| --- | --- | --- |
| Claude Code 2.1.205 | Yes | Has its own plugin marketplace; individual skills and MCP servers may be adapted |
| Gemini CLI 0.36.0 | Yes | Has separate extensions, skills, and MCP commands |
| OpenCode 1.18.4 | Yes | Not listed by the Agent Plugins compatibility directory |

The repository does not claim that a package installs unchanged in clients that
are absent from the official compatibility directory.

Sanitized client evidence is committed under [`tests/e2e/results`](../tests/e2e/results).
