# Five launch plugins

Choose one: these are independent examples, not sequential steps. The first
four need no account credentials. The CLI detects Codex, Cursor, GitHub
Copilot/VS Code, and Kiro; if several are installed, choose one when prompted.

Agent Code Navigator is skills-only. Context7 and Chrome DevTools use reviewed,
integrity-locked npm runtime closures. Cloudflare Docs uses a public remote MCP
server and also has a registered ChatGPT development binding. Notion requires
client-managed OAuth.

The registered Cloudflare Docs personal app passed Plugins UI discovery,
manual activation, and read-only runtime. The repository package separately
passed marketplace ingestion and official manager installation; package-routed
ChatGPT Work runtime remains unproved.

## 1. Agent Code Navigator

Install:

```bash
npx universal-agent-plugins add agent-code-navigator
```

Try:

```text
Map this sandbox repository's architecture and explain which search tool you use for each claim.
```

Expected: the agent loads the routing and architecture-map skills without
starting an MCP server or modifying the repository.

## 2. Context7

The reviewed short name currently fails closed while its next signed Directory
release is awaiting the remaining protected client evidence. Until that release
is promoted, install the exact no-account package that passed Codex, Cursor, and
Kiro runtime E2E:

```bash
npx universal-agent-plugins add \
  777genius/universal-agent-plugins@dcd94db0bfafe5ff5c4b1f1154ee1f7c656c19e4//plugins/context7
```

Try:

```text
Use Context7 to find the current React documentation for useEffect cleanup.
```

Expected: the agent resolves the React library and queries its current
documentation without an account.

## 3. Cloudflare Docs

Install:

```bash
npx universal-agent-plugins add cloudflare-docs
```

Try:

```text
Use Cloudflare Docs to explain the current difference between Workers bindings and environment variables.
```

Expected: the public Streamable HTTP MCP server answers without an account.

## 4. Chrome DevTools

Install:

```bash
npx universal-agent-plugins add chrome-devtools
```

Try:

```text
Use Chrome DevTools to open a blank page and list the available browser pages.
```

Expected: the agent starts the reviewed Chrome DevTools runtime and controls an
isolated browser session. A compatible local browser must be installed.

## 5. Notion

Install:

```bash
npx universal-agent-plugins add notion
```

Try:

```text
Connect Notion, then search only for the synthetic test page I name.
```

Expected: the client asks for OAuth consent before a read-only query. Use a
dedicated test workspace for repeatable verification.

## OAuth follow-up

After a no-auth plugin works, test Cloudflare Radar, Figma, Linear, or Notion in
a dedicated test workspace. A one-off personal-workspace check is allowed only
with explicit owner approval, a synthetic read-only probe, no private content in
the result, immediate client cleanup, and provider-grant revocation. If a safe
granular provider revoke is unavailable, record cleanup as partial instead of
using a broader destructive action or claiming completion. Automated or
repeatable OAuth tests always require a dedicated test account or workspace.
Confirm the requested scopes before approval and begin with a read-only query.
OAuth success is client-specific and is tracked in the [test matrix](TEST_MATRIX.md),
not inferred from schema validation.
