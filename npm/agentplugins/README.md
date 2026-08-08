# agentplugins

Install and manage portable Agent Plugins 1.0 packages across Codex/ChatGPT,
Cursor, GitHub Copilot/VS Code, and Kiro.

```bash
npx --yes agentplugins@0.1.0-beta.1 add context7
```

`agentplugins` is an independent community CLI built in
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
npx --yes agentplugins@0.1.0-beta.1 doctor
npx --yes agentplugins@0.1.0-beta.1 list
npx --yes agentplugins@0.1.0-beta.1 add context7 --dry-run --target cursor
npx --yes agentplugins@0.1.0-beta.1 add context7 --target cursor --yes
npx --yes agentplugins@0.1.0-beta.1 update context7 --target cursor --yes
npx --yes agentplugins@0.1.0-beta.1 remove context7 --target cursor --yes
```

Each mutation changes one selected client. `--yes` never means install
everywhere. Beta supports user scope and reports whether a client can install
automatically or requires manual activation. For older `plugin-kit-ai` installations, run the explicit migration
before the first standard installation:

```bash
npx --yes agentplugins@0.1.0-beta.1 migrate-state --dry-run
npx --yes agentplugins@0.1.0-beta.1 migrate-state --yes
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

If update or remove detects a missing or modified managed directory, beta stops
without changing state or deleting files. Preserve any changed directory and
restore the exact tracked package before retrying; automatic drift repair is not
part of v0.1.
