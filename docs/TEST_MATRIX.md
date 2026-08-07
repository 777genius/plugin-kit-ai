# Plugin verification matrix

Status meanings:

- `Tested`: installed or exercised in a named client or direct MCP harness.
- `Schema only`: package structure validates, but runtime behavior was not run.
- `Auth required`: the endpoint correctly requested authentication; no behavior
  claim is made until OAuth or token setup succeeds.

| Plugin | Package | Endpoint/package | Auth | Behavior | Evidence |
| --- | --- | --- | --- | --- | --- |
| `agent-code-navigator` | Tested | Tested | None | Codex skill call passed | Codex CLI 0.144.1 + MCP harness, 2026-08-07 |
| `context7` | Tested | Tested | None | Codex and Cursor calls/connections passed | Codex 0.144.1, Cursor 3.9.16, Inspector 2.1.0 |
| `cloudflare-docs` | Tested | Tested | None | Search call passed | MCP Inspector 2.1.0, 2026-08-07 |
| `cloudflare-radar` | Tested | Reachable | Auth required | OAuth discovery confirmed | MCP Inspector 2.1.0, 2026-08-07 |
| `chrome-devtools` | Tested | Tested | Local browser | 29 tools discovered in an isolated sandbox | MCP Inspector 2.1.0, 2026-08-07 |
| `figma` | Tested | Tested | Auth required | OAuth discovery passed; successful consent pending | Inspector + Codex 0.144.1 |
| `github` | Tested | Tested | Auth required | Auth discovery passed | MCP Inspector 2.1.0; `GITHUB_PAT_TOKEN` adapter metadata |
| `linear` | Tested | Tested | Auth required | Auth discovery passed | MCP Inspector 2.1.0, 2026-08-07 |
| `notion` | Tested | Tested | OAuth passed | ChatGPT raw MCP development connection completed OAuth and returned `UAP_NOTION_E2E_OK 0` from an authenticated read-only search; repository package install remains separate | ChatGPT web + Inspector + Codex 0.144.1 |
| `sentry` | Tested | Tested | Auth required | Auth discovery passed | MCP Inspector 2.1.0, 2026-08-07 |
| Remaining 16 packages | Schema only | Reachable or package-verified | Varies | Not run | See `COMPATIBILITY.md` |

A `401` or OAuth prompt proves auth discovery, not successful authenticated
behavior. Sanitized raw evidence is in [`tests/e2e/results`](../tests/e2e/results).
