# Universal Agent Plugins

Install and manage portable Agent Plugins 1.0 packages across Codex/ChatGPT,
Cursor, GitHub Copilot/VS Code, and Kiro.

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

```bash
npx universal-agent-plugins doctor
npx universal-agent-plugins list
npx universal-agent-plugins add context7 --dry-run --target cursor
npx universal-agent-plugins add context7 --target cursor
npx universal-agent-plugins update context7 --target cursor
npx universal-agent-plugins remove context7 --target cursor
```

For GitHub Copilot CLI, `agentplugins` performs the native install, update, and
remove automatically through a managed local marketplace. VS Code discovers
the same installation automatically, so selecting either target once is
enough. Codex/ChatGPT and Kiro print one exact, path-specific next step when
their client UI must finish the installation.

Short names resolve through the pinned
[`universal-agent-plugins`](https://github.com/777genius/universal-agent-plugins)
catalog. You can also install a different valid Agent Plugins 1.0 package from
a local directory or an exact GitHub source:

```bash
npx universal-agent-plugins add ./my-plugin --target cursor
npx universal-agent-plugins add owner/repo@commit//plugins/my-plugin --target cursor
```

The package must have a root `plugin.json` using the supported Agent Plugins
1.0 schema. Optional root `mcp.json` and `skills/*/SKILL.md` components are
installed only where the selected client supports them. The downloaded binary
and installed command remain `agentplugins`.

Each mutation changes one selected client and asks before changing it. v0.1
supports user scope and reports whether a client can install automatically or
requires manual activation. For older `plugin-kit-ai` installations, run the
explicit migration before the first standard installation:

```bash
npx universal-agent-plugins migrate-state --dry-run
npx universal-agent-plugins migrate-state
```

The migration validates the complete State v2 result, creates a byte-for-byte
backup, and keeps legacy `plugin.yaml` packages on their original lifecycle
until an explicit `migrate-format`.

Legacy migration intentionally shares the old `plugin-kit-ai` sentinel lock. If
a crash leaves `~/.plugin-kit-ai/locks/state.lock`, first stop every
`plugin-kit-ai`, `integrationctl`, and `agentplugins` process (or reboot), then
inspect the PID recorded in that JSON file. Only when no owner can still be
running, preserve it instead of deleting it:

```bash
mv -n ~/.plugin-kit-ai/locks/state.lock ~/.plugin-kit-ai/locks/state.lock.stale
```

On Windows:

```powershell
Move-Item $HOME\.plugin-kit-ai\locks\state.lock $HOME\.plugin-kit-ai\locks\state.lock.stale
```

If ownership is uncertain, do not move the lock.

OAuth remains client-managed. Prepared or auth-pending packages are never
reported as installed.

If update or remove detects a missing or modified managed directory, v0.1.0 stops
without changing state or deleting files. Preserve any changed directory and
restore the exact tracked package before retrying; automatic drift repair is not
part of v0.1.
