# Plugin verification matrix

Verified on 2026-08-07. Each column is independent: a schema pass is not an
install, an auth challenge is not OAuth success, and tool discovery is not a
tool call. `Direct harness` means MCP Inspector, not a client installation.

| Plugin | Schema | Installed | Tool called | OAuth | Clients |
| --- | --- | --- | --- | --- | --- |
| `agent-code-navigator` | Pass | Codex | N/A - packaged skill/doctor ran | None | Codex |
| `atlassian` | Pass | No | No | Required - not tested | None |
| `chrome-devtools` | Pass | No | No - 29 tools discovered | Local browser, not OAuth | Direct harness |
| `cloudflare` | Pass | No | No | Required - not tested | None |
| `cloudflare-bindings` | Pass | No | No | Required - not tested | None |
| `cloudflare-docs` | Pass | No | `search_cloudflare_documentation` | None | Direct harness |
| `cloudflare-observability` | Pass | No | No | Required - not tested | None |
| `cloudflare-radar` | Pass | No | No | Discovery passed; consent not tested | Direct harness |
| `context7` | Pass | Codex; Kiro import; Cursor local load | `resolve-library-id`; `query-docs` | None | Codex; Kiro; Cursor; direct harness |
| `docker-hub` | Pass | No | No | Optional credentials - not tested | None |
| `figma` | Pass | No | No | Discovery passed; consent pending | Direct harness; Codex discovery |
| `firebase` | Pass | No | No | Local CLI login - not tested | None |
| `github` | Pass | No | No | Discovery passed; consent not tested | Direct harness |
| `gitlab` | Pass | No | No | Required - not tested | None |
| `greptile` | Pass | No | No | Required - not tested | None |
| `heroku` | Pass | No | No | Required - not tested | None |
| `hubspot-crm` | Pass | No | No | Required - not tested | None |
| `hubspot-developer` | Pass | No | No | Local CLI login - not tested | None |
| `linear` | Pass | No | No | Discovery passed; consent not tested | Direct harness |
| `neon` | Pass | No | No | Required - not tested | None |
| `notion` | Pass | No - raw endpoint only | Authenticated read-only search | Passed for ChatGPT raw MCP; package install separate | ChatGPT; direct harness; Codex discovery |
| `sentry` | Pass | No | No | Discovery passed; consent not tested | Direct harness |
| `statsig` | Pass | No | No | Required - not tested | None |
| `stripe` | Pass | No | No | Required - not tested | None |
| `supabase` | Pass | No | No | Required - not tested | None |
| `vercel` | Pass | No | No | Required - not tested | None |

The automated Codex record includes the public release ref and commit, workflow
URL and commit, copy-ready reproduction commands, and a sanitized three-event
transcript. All client records are under [`tests/e2e/results`](../tests/e2e/results).

## Installer lifecycle

| Installer | Catalog | Package add/remove | Client runtime | OAuth |
| --- | --- | --- | --- | --- |
| `agentplugins 0.1.0-beta.1-development` | 26/26 pinned | 26/26 passed in an isolated Cursor provider HOME | Not implied | Not tested |
| `agentplugins 0.1.0-beta.1-development` hero matrix | 5 pinned hero packages | 25/25 add/remove flows across isolated Codex, Cursor, Copilot, VS Code, and Kiro projections | Not implied | Not tested |

This row proves source resolution, package validation, transactional
materialization, state receipts, digest guards, and removal. It does not claim
that Cursor launched, discovered tools, called tools, or completed OAuth.
