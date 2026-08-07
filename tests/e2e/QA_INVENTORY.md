# E2E QA inventory

## Claims to verify

- The GitHub marketplace installs in Codex from a fresh sandbox project.
- A skills-only plugin is discoverable after installation.
- At least one stdio and one Streamable HTTP hero MCP can initialize and list tools.
- Cursor discovers an unchanged portable package from its documented local path.
- Kiro imports an unchanged package folder and completes a real MCP-backed query.
- OAuth-required endpoints return an auth challenge without exposing secrets.
- ChatGPT can discover an OAuth-enabled MCP in developer mode when the account surface permits it.
- README commands match the actual client commands.
- GitHub CI is green on the public `main` branch.

## Functional checks

| Flow | Happy path | Off-happy path | Evidence |
| --- | --- | --- | --- |
| Codex marketplace | Add the public release ref, install Context7, call `resolve-library-id` | Missing ref, wrong package, path escape, or malformed tool output fails the job | Workflow URL/commit, source ref/commit, reproduction commands, sanitized transcript |
| Skills plugin | New sandbox session discovers skills | Unrelated prompt does not need the skill | Session transcript |
| stdio MCP | Initialize and list tools | Missing executable reports component failure | Harness JSON |
| HTTP MCP | Initialize and list tools | Auth endpoint reports 401/challenge | Harness JSON |
| Cursor | Local package appears in Customize | Invalid fixture is rejected or absent | App logs and evidence JSON |
| Kiro | Import Context7 folder, activate Power, call both Context7 tools | Approval remains explicit; no real project is opened | Sanitized evidence JSON |
| ChatGPT OAuth | Connection discovers auth, user consent completes, and a synthetic read-only search returns a marker | Pending/cancelled OAuth must not claim success; client and provider cleanup are tracked separately | Sanitized evidence JSON and matrix status |

Automated Codex records must come from a fresh `CODEX_HOME` and disposable Git
repository. They must exclude credentials, raw tool output, account data, and
absolute temporary paths. The committed record links to the successful public
workflow run that produced the downloadable artifact.

## Visual checks

- Quick start is visible without searching the repository.
- Logo remains legible at 128x128.
