# ADR: Public Docs Platform

Status: Accepted

Decision:

- Build the public docs site in `website/`, not on top of the legacy `docs/` tree.
- Keep maintainer/internal docs versioned in `maintainer-docs/` and exclude them from public build, navigation, search, sitemap, and SEO.
- Use `vitepress@2.0.0-alpha.17` as the rendering layer with exact version pinning.
- Generate API reference through official or established generators:
  - Cobra docs for CLI
  - gomarkdoc for public Go packages
  - TypeDoc plus `typedoc-plugin-markdown` for the Node runtime
  - pydoc-markdown for `plugin-kit-ai-runtime`
  - repo-native descriptor exports for events, capabilities, and support summaries
- Normalize generator output into a unified registry and deterministic markdown tree before the VitePress build.
- Publish bilingual public docs under `/docs/en/` and `/docs/ru/`, with `/docs/` reserved for a noindex language gateway.

Consequences:

- Public docs information architecture stays independent from maintainer process docs.
- Generator churn is isolated behind the docs pipeline instead of leaking into site structure.
- Generated output remains reviewable in git diffs because committed markdown and registries are source artifacts, not build artifacts.
