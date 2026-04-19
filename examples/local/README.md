# Repo-Local Plugin Examples

These examples are reference implementations for the fast local plugin entrance layer.
For copy-first starter repos, see [../starters/README.md](../starters/README.md).

- [codex-python-local](./codex-python-local): repo-local `codex-runtime` example for Python teams using `plugin-kit-ai bootstrap .`, `.venv`, `validate --strict`, launcher-based `notify`, and the helper API in `plugin/plugin_runtime.py` that mirrors the shared `plugin-kit-ai-runtime` package
- [codex-node-local](./codex-node-local): repo-local `codex-runtime` example for Node teams using `plugin-kit-ai bootstrap .`, `validate --strict`, launcher-based `notify`, and the helper API in `plugin/plugin-runtime.mjs` that mirrors the shared `plugin-kit-ai-runtime` package
- [codex-node-typescript-local](./codex-node-typescript-local): repo-local `codex-runtime` example for TypeScript teams using `plugin-kit-ai doctor .`, `plugin-kit-ai bootstrap .`, built output under `dist/`, and the helper API in `plugin/plugin-runtime.ts` that mirrors the shared `plugin-kit-ai-runtime` package

These Node/TypeScript and Python examples are the `public-stable` repo-local local-runtime subset.
They are supported paths for teams that prefer those runtimes, but they still require Python or Node to be installed on the machine running the plugin.
Launcher-based `shell` authoring remains `public-beta` and is covered through runtime docs plus `polyglot-smoke`, not through a checked-in local example repo.
They complement, not replace, the production reference repos in [../plugins/README.md](../plugins/README.md).
Go now also has copy-first starters in [../starters/README.md](../starters/README.md), and the production examples remain the clearest long-term support and release story when you want the most self-contained delivery model.
