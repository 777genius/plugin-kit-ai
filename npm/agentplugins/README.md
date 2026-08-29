# Universal Agent Plugins

`universal-agent-plugins` is the public npm package for installing and managing
portable Agent Plugins 1.0 packages. It installs the `agentplugins` binary;
`npx universal-agent-plugins` and a globally installed `agentplugins` run the
same lifecycle manager, not separate engines.

Prerequisite: Node.js 22 or newer.

```bash
npx universal-agent-plugins add context7
```

`universal-agent-plugins` is an independent community CLI built in
[`plugin-kit-ai`](https://github.com/777genius/plugin-kit-ai). It is not an
official OpenAI or Agent Plugins project and is not affiliated with
`sigilco/agentplugins` or `@agentplugins/cli`. Agent Plugins 1.0 defines the
portable `plugin.json` package; this CLI supplies installation and lifecycle
policy.

The npm package has no `postinstall`. On first execution it downloads only the
binary matching the exact npm version, verifies the SHA-256 embedded in the npm
tarball, then caches it under XDG Cache or LocalAppData. It never falls back to
`latest` and never sends `GITHUB_TOKEN` to public downloads.

Release 0.1.15 passed exact native npm bootstrap proofs on all six supported
platforms:

| OS | x64 | arm64 |
| --- | --- | --- |
| macOS | Tested | Tested |
| Linux | Tested | Tested |
| Windows | Tested | Tested |

Other operating systems and CPU architectures are unsupported and fail before
the binary runs.

```bash
npx universal-agent-plugins doctor
npx universal-agent-plugins list
npx universal-agent-plugins search docs
npx universal-agent-plugins validate ./my-plugin
npx universal-agent-plugins add context7 --dry-run --target cursor
npx universal-agent-plugins add context7 --target codex,cursor,kiro
npx universal-agent-plugins outdated --all
npx universal-agent-plugins update --all
npx universal-agent-plugins update context7 --target codex,cursor,kiro
npx universal-agent-plugins repair context7 --target codex,cursor,kiro
npx universal-agent-plugins remove context7 --target codex,cursor,kiro
npx universal-agent-plugins switch context7 --to upstash/context7
```

## Choose clients

In an interactive terminal, `add` without `--target` detects supported clients.
One detected client is selected automatically. With several clients, the CLI
shows a multi-select with all detected clients selected by default; press Enter
to keep all of them. Scripts and CI must choose explicitly, for example
`--target claude,gemini,opencode`. `add`, `update`, `repair`, and `remove` accept
the same comma-separated syntax.

Detection checks installed executables and documented configuration surfaces.
It does not start an agent, log in, complete OAuth, or prove browser/tool
runtime behavior.

The following matrix describes the current release source. New client support
is available through `npx` once a release containing this source is published.

| Client | Evidence |
| --- | --- |
| Claude Code | Claude Code 2.1.205: real isolated `add`, native list, `update`, and `remove` lifecycle |
| Gemini CLI | Gemini CLI 0.36.0: real isolated configuration and lifecycle |
| OpenCode | OpenCode 1.18.4: real isolated configuration and lifecycle |
| Cline | Isolated native-configuration lifecycle; no client runtime or login |
| Windsurf | Contract and mock coverage only; Windsurf was absent from the test host |

`repair` reapplies or reactivates the recorded revision; it does not update or
change source. `switch` moves the complete installation to a qualified
Directory distribution or exact source, so it uses `--to` instead of
`--target`.

`search` combines reviewed Directory releases with a separately signed
Discovery Index. Discovery results remain visibly unreviewed and use an exact,
publisher-qualified selector such as `discovery:owner/repo//path`. Before any
mutation, the CLI reacquires the indexed commit and validates the package. The
index signature authenticates discovery metadata; it does not endorse package
code or runtime behavior. `outdated` is read-only. `update --all` preserves each
installation's recorded source and targets and performs a complete batch
preflight before applying updates.

A successful non-dry-run, multi-target `add --format json` includes one
`data.acquisition` proof and a `data.target_outcomes` object keyed by every
requested target. The acquisition count is exactly one and every passed target
binds the same acquisition ID, validated tree digest, manifest digest, and
closure digest. `fetched` is true only for a remote GitHub or Directory source;
`source_kind` distinguishes `github`, `directory`, and `local` acquisitions.
The closure digest is SHA-256 over the length-prefixed tuple of source kind,
repository, package subpath, resolved revision, tree digest, and manifest
digest, prefixed by the domain `agentplugins/grouped-acquisition-closure/v1`.
The domain is itself the first length-prefixed field. The digest excludes
requested and canonical source strings so local paths and
host-specific data never enter the public proof. Dry runs omit this completed
proof, and failed or partial group outcomes never label every target `passed`.

Short names resolve through the signed
[Universal Agent Plugins Directory](https://github.com/777genius/universal-agent-plugins).
The CLI shows the selected immutable release, publisher, source, and verification
status before installation. A community bridge remains clearly attributed and
is not presented as its upstream publisher. You can also install a valid package
directly from a local directory or exact GitHub source without Directory
submission:

```bash
npx universal-agent-plugins add ./my-plugin --target cursor
npx universal-agent-plugins add owner/repo@0123456789abcdef0123456789abcdef01234567//plugins/my-plugin --target cursor
```

Replace `0123456789abcdef0123456789abcdef01234567` with the full lowercase
40-character commit SHA you reviewed. A branch, tag, abbreviated SHA, or the
word `commit` is not accepted.

For Agent Plugins 1.0, the root `plugin.json` is the install authority and may
reference standard components such as `mcp.json` and `skills/`. `plugin.yaml`
is legacy authoring input only. It cannot be merged with or silently override a
`plugin.json`; changing a legacy package format is a separate explicit action.

Every operation validates the source and preflights all affected targets before
changing managed files or state. `--dry-run` prints the same plan without
writing. Managed changes are staged and committed together; on failure the CLI
rolls back what it can prove it owns, or preserves the safe state and prints a
repair action. Observed unowned entries fail closed before mutation. Portable
filesystems cannot provide a linearizable content compare-and-swap against a
non-cooperating client that writes in the final syscall-sized window, so the CLI
rechecks and reports identity conflicts instead of promising impossible
concurrency isolation.

Activation is reported per client. The CLI completes supported automatic steps;
when a client requires its UI, it reports `prepared` or manual activation and
prints the next action. OAuth and consent prompts remain visible and
user-controlled. Cancelling keeps the package and reports authentication as
pending or cancelled rather than runtime success. Directory badges and status
describe only the exact schema, materialization, discovery, runtime, or OAuth
evidence collected—not every plugin/client/environment combination.

Kiro skills are installed directly into the documented global skills path.
For packages with MCP servers, agentplugins atomically merges only its owned
entries into Kiro's global `mcp.json`, preserves unrelated configuration, and
uses a complete supported Kiro CLI distribution for a bounded structured ACP
v1 initialize/session handshake. The handshake supplies no MCP definitions and
sends no prompt, model turn, tool call, or permission response: Kiro must load
the installed native configuration and report each planned server connected
with enabled tools. The verifier keeps ACP stdin open while it drains a bounded
quiet settlement window, and rejects EOF, partial, contradictory, or malformed
protocol evidence before supervised containment stops and reaps the long-lived
child. Automatic Kiro ACP verification is available only on Linux after
capability preflight proves delegated cgroup v2 creation, atomic
CLONE_INTO_CGROUP placement, and cgroup.kill. macOS, Windows, and Linux hosts
without every required proof fail preflight before any MCP mutation. On those
hosts, use Kiro's documented manual skill and MCP configuration paths; that
workflow is outside this
automatic CLI path. Failure to start ACP after a supported preflight may still
leave a manual verification action. This proves session loading, not runtime
tool end-to-end behavior. Once ACP starts, companion-launch
failure (including a missing `kiro-cli-chat`), EOF, timeout, authentication
failure, malformed or partial output, contradiction, and non-clean exit are
authoritative activation failures; managed state stays committed for explicit
repair and is never reported active.
