# Client compatibility

The CLI adapts a standard Agent Plugins package to each client's supported
skills and MCP configuration. "CLI delivery" is installation support; it does
not by itself claim that every external service, OAuth flow, or model runtime
was tested in that client.

| Client | Agent Plugins 1.0 components | Local availability | Verification |
| --- | --- | --- | --- |
| Codex | Skills; stdio and Streamable HTTP MCP | Codex CLI 0.147.0 | Automated public marketplace install + Context7 tool call; separate Figma OAuth/read-only runtime passed |
| ChatGPT | Skills and registered Streamable HTTP MCP app bindings; no stdio | ChatGPT web/desktop | Cloudflare Docs registered personal app passed manual activation and read-only runtime; separate repository package ingestion and official manager installation passed, while package-routed Work runtime remains unproved |
| Cursor | Skills; stdio, Streamable HTTP, legacy SSE MCP | Cursor 3.9.16 | Local package load + pinned stdio MCP connection passed; no marketplace-install claim |
| VS Code | Skills; stdio, Streamable HTTP, legacy SSE MCP | Not installed | Shares the managed Copilot plugin when Copilot CLI is available; VS Code UI runtime remains separately untested |
| GitHub Copilot | Skills; stdio, Streamable HTTP, legacy SSE MCP | Copilot CLI 1.0.78 | Five hero packages passed automatic marketplace add, plugin install, verification, uninstall, and marketplace cleanup in an isolated profile |
| Kiro | Skills; stdio, Streamable HTTP, legacy SSE MCP | Kiro IDE 1.0.288 | Local folder import, power activation, and Context7 resolve/query calls passed in a disposable project |
| Claude Code | Skills and MCP | Claude Code 2.1.251 | Managed package lifecycle and native plugin listing passed in an isolated profile |
| Gemini CLI | Skills and MCP | Gemini CLI 0.57.0 | Managed package lifecycle and MCP discovery passed in an isolated profile; model-account runtime remains separate and unclaimed |
| OpenCode | Skills and MCP | OpenCode 1.18.25 | Managed package lifecycle and live Chrome DevTools model tool calls passed in an isolated profile |
| Cline | Skills and MCP | Disposable product-shaped profile | Managed package lifecycle passed; live extension/model runtime remains untested |
| Windsurf | Prepared skills and MCP package | Disposable product-shaped profile | Preparation, repair, and removal passed; manual activation and live model runtime remain untested |

The compatibility directory describes client support for the standard. It is
not proof that this repository was installed in every listed client. Marketplace
and directory manifests are client-owned adapters, not portable 1.0 files.

OpenAI delivery is intentionally split: five stdio MCP packages are Codex-only,
twenty remote MCP packages require registered ChatGPT app bindings, and the one
skills-only package has no MCP transport. Only Cloudflare Docs currently has a
repository binding and a tested registered personal app. The app ID linkage is
proved. Separate repository marketplace ingestion, official manager installation,
cache materialization, and desktop control-plane parsing also passed. ChatGPT
Work UI activation and package-routed runtime remain unproved.

Claude Code, Gemini CLI, OpenCode, Cline, and Windsurf are supported through
client adapters rather than a claim that those products natively consume every
Agent Plugins 1.0 file unchanged. The Directory exposes a target only when the
current stable CLI can produce that client's delivery safely.

Sanitized client evidence is committed under [`tests/e2e/results`](../tests/e2e/results).
