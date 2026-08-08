# Plugin verification matrix

Verified on 2026-08-08. Each column is independent: a schema pass is not an
install, an auth challenge is not OAuth success, and tool discovery is not a
tool call. `Direct harness` means MCP Inspector, not a client installation.

| Plugin | Schema | Installed | Tool called | OAuth | Clients |
| --- | --- | --- | --- | --- | --- |
| `agent-code-navigator` | Pass | Codex; Cursor local load; Kiro CLI resource | Packaged skill route passed | None | Codex; Cursor; Kiro |
| `atlassian` | Pass | No | No | Required - not tested | None |
| `chrome-devtools` | Pass | Codex; Cursor local load; Kiro workspace | `list_pages` | Local browser, not OAuth | Codex; Cursor; Kiro; direct harness |
| `cloudflare` | Pass | No | No | Required - not tested | None |
| `cloudflare-bindings` | Pass | No | No | Required - not tested | None |
| `cloudflare-docs` | Pass | Codex; Cursor local load; Kiro workspace | `search_cloudflare_documentation` | None | Codex; Cursor; Kiro; direct harness |
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
| `notion` | Pass | Codex; Cursor; Kiro | Authenticated read-only search in Codex, Cursor, Kiro, and raw ChatGPT MCP | Passed in Codex, Cursor, Kiro, and ChatGPT | Codex; Cursor; Kiro; ChatGPT; direct harness |
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
| `agentplugins 0.1.1` release binary | 26/26 pinned | 26/26 passed in an isolated Cursor provider HOME | Not implied | Not tested |
| `agentplugins 0.1.1` release binary, hero matrix | 5 pinned hero packages | 25/25 add/remove flows across isolated Codex, Cursor, Copilot, VS Code, and Kiro projections | Not implied | Not tested |
| Interactive hero runtime matrix | 5 local packages | Client-specific test loading in Codex, Cursor, and Kiro | 15/15 checks passed across 3 clients | 3/3 Notion OAuth + read-only runtime passed |

The first two rows prove source resolution, package validation, transactional
materialization, and removal; they do not imply client launch or tool calls.
Those claims are tracked independently in the interactive runtime row, and an
auth challenge is still not counted as OAuth success.
