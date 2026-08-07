# Four optional plugins to try first

These examples avoid account credentials and are designed to prove the package
flow before a user connects a private service. Choose one example. They are
independent alternatives, not sequential installation steps.

Add the release-pinned catalog once:

```bash
codex plugin marketplace add 777genius/universal-agent-plugins --ref v0.1.1
```

## 1. Agent Code Navigator

Install:

```bash
codex plugin add agent-code-navigator@universal-agent-plugins
```

Try:

```text
Map this sandbox repository's architecture and explain which search tool you use for each claim.
```

Expected: the agent loads the routing and architecture-map skills without
starting an MCP server or modifying the repository.

## 2. Context7

Install:

```bash
codex plugin add context7@universal-agent-plugins
```

Try:

```text
Use Context7 to find the current official quick start for Playwright locators and summarize it with source links.
```

Expected: Context7 starts through its pinned stdio package and returns current
documentation results.

## 3. Cloudflare Docs

Install:

```bash
codex plugin add cloudflare-docs@universal-agent-plugins
```

Try:

```text
Use Cloudflare Docs to explain the current difference between Workers bindings and environment variables.
```

Expected: the public Streamable HTTP MCP server answers without an account.

## 4. Chrome DevTools

Install:

```bash
codex plugin add chrome-devtools@universal-agent-plugins
```

Try only in a fresh test project and browser profile:

```text
Open the sandbox page, inspect its title, and report console errors without changing the page.
```

Expected: the pinned local MCP package launches and exposes browser-debugging
tools. Do not point this test at a signed-in browser profile.

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
