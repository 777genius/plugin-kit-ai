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

Release 0.1.5 passed exact native npm bootstrap proofs on all six supported
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
npx universal-agent-plugins add context7 --dry-run --target cursor
npx universal-agent-plugins add context7 --target codex,cursor,kiro
npx universal-agent-plugins update context7 --target codex,cursor,kiro
npx universal-agent-plugins repair context7 --target codex,cursor,kiro
npx universal-agent-plugins remove context7 --target codex,cursor,kiro
npx universal-agent-plugins switch context7 --to upstash/context7
```

`add`, `update`, `repair`, and `remove` accept comma-separated targets. `repair`
reapplies or reactivates the recorded revision; it does not update or change
source. `switch` moves the complete installation to a qualified Directory
distribution or exact source, so it uses `--to` instead of `--target`.

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
repair action. It never overwrites an unowned client installation.

Activation is reported per client. The CLI completes supported automatic steps;
when a client requires its UI, it reports `prepared` or manual activation and
prints the next action. OAuth and consent prompts remain visible and
user-controlled. Cancelling keeps the package and reports authentication as
pending or cancelled rather than runtime success. Directory badges and status
describe only the exact schema, materialization, discovery, runtime, or OAuth
evidence collected—not every plugin/client/environment combination.
