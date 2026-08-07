# E2E QA inventory

## Claims to verify

- The GitHub marketplace installs in Codex from a fresh sandbox project.
- A skills-only plugin is discoverable after installation.
- At least one stdio and one Streamable HTTP hero MCP can initialize and list tools.
- Cursor discovers an unchanged portable package from its documented local path.
- OAuth-required endpoints return an auth challenge without exposing secrets.
- ChatGPT can discover an OAuth-enabled MCP in developer mode when the account surface permits it.
- README commands match the actual client commands.
- GitHub CI is green on the public `main` branch.

## Functional checks

| Flow | Happy path | Off-happy path | Evidence |
| --- | --- | --- | --- |
| Codex marketplace | Add, list, install hero plugin | Unknown plugin fails clearly | Command transcript and JSON status |
| Skills plugin | New sandbox session discovers skills | Unrelated prompt does not need the skill | Session transcript |
| stdio MCP | Initialize and list tools | Missing executable reports component failure | Harness JSON |
| HTTP MCP | Initialize and list tools | Auth endpoint reports 401/challenge | Harness JSON |
| Cursor | Local package appears in Customize | Invalid fixture is rejected or absent | App logs and evidence JSON |
| ChatGPT OAuth | Connection discovers auth, user consent completes, and a synthetic read-only search returns a marker | Pending/cancelled OAuth must not claim success; client and provider cleanup are tracked separately | Sanitized evidence JSON and matrix status |

## Visual checks

- Quick start is visible without searching the repository.
- Logo remains legible at 128x128.
