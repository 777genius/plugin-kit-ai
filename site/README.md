# Universal Agent Plugins site

Static Nuxt 3 frontend for the generated plugin registry.

Requirements: Node.js 22 or newer and `../registry/index.json` generated from
the repository registry. Production builds intentionally fail if that index is
missing, malformed, contains an unpinned external source, or does not contain
the 26 built-ins.

```bash
pnpm install --frozen-lockfile
pnpm lint
pnpm typecheck
pnpm test
NUXT_APP_BASE_URL=/universal-agent-plugins/ pnpm generate
```

For site-only development and verification while the generated index is not
available, point Nuxt at the deliberately tiny fixture:

```bash
UAP_REGISTRY_PATH=tests/fixtures/registry.valid.json pnpm dev
```

The site emits no analytics or tracking requests. See `NOTICE.md` and the icon
README files under `public/` for copied/adapted code and mark attribution.
