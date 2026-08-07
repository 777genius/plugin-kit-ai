# Upstream sources

Checked on 2026-08-07. Vendor names and trademarks belong to their owners.

OpenAI-only auth and capability adapter values are taken from OpenAI's published
[GitHub](https://github.com/openai/plugins/blob/11c74d6ba24d3a6d48f54a194cd00ef3beea18f9/plugins/github/.mcp.json),
[Figma](https://github.com/openai/plugins/blob/11c74d6ba24d3a6d48f54a194cd00ef3beea18f9/plugins/figma/.codex-plugin/plugin.json),
[Linear](https://github.com/openai/plugins/blob/11c74d6ba24d3a6d48f54a194cd00ef3beea18f9/plugins/linear/.mcp.json),
[Notion](https://github.com/openai/plugins/blob/11c74d6ba24d3a6d48f54a194cd00ef3beea18f9/plugins/notion/.mcp.json),
and [Sentry](https://github.com/openai/plugins/blob/11c74d6ba24d3a6d48f54a194cd00ef3beea18f9/plugins/sentry/.codex-plugin/plugin.json)
packages at commit `11c74d6ba24d3a6d48f54a194cd00ef3beea18f9`.
No host-specific auth value is inferred when a published package is unavailable.

CI uses OpenAI's Apache-2.0 licensed
[`plugin-creator` validator](https://github.com/openai/codex/blob/a4b129eb3e1a6929c09d6e2e1af0638122c56f0d/codex-rs/skills/src/assets/samples/plugin-creator/scripts/validate_plugin.py)
from a pinned Codex commit with a recorded SHA-256 digest.

| Package | Upstream documentation | Purpose |
| --- | --- | --- |
| `agent-code-navigator` | https://github.com/777genius/universal-plugins-for-ai-agents/tree/main/plugins/agent-code-navigator | Route code discovery across exact search, semantic search, LSP navigation, and call-graph analysis. |
| `atlassian` | https://support.atlassian.com/atlassian-rovo-mcp-server/docs/getting-started-with-the-atlassian-remote-mcp-server/ | Community package for the official Atlassian Rovo MCP plugin for Jira, Confluence, and Compass workflows through Atlassian's hosted remote MCP service. |
| `chrome-devtools` | https://github.com/ChromeDevTools/chrome-devtools-mcp | Community package for Chrome DevTools MCP plugin for Claude, Codex, Gemini, OpenCode, and Cursor. Launch the official Chrome DevTools MCP server so agents can inspect pages, automate flows, analyze performance, and debug browser state. |
| `cloudflare` | https://developers.cloudflare.com/agents/model-context-protocol/mcp-servers-for-cloudflare/ | Community package for the official Cloudflare API MCP plugin for token-efficient access to the Cloudflare API through Cloudflare's hosted Code Mode server. |
| `cloudflare-bindings` | https://developers.cloudflare.com/agents/model-context-protocol/mcp-servers-for-cloudflare/ | Community package for the official Cloudflare Workers Bindings MCP integration for storage, AI, and compute primitives. |
| `cloudflare-docs` | https://developers.cloudflare.com/agents/model-context-protocol/mcp-servers-for-cloudflare/ | Community package for the official Cloudflare hosted MCP plugin for up-to-date Cloudflare documentation and reference lookups through Cloudflare's remote documentation server. |
| `cloudflare-observability` | https://developers.cloudflare.com/agents/model-context-protocol/mcp-servers-for-cloudflare/ | Community package for the official Cloudflare hosted MCP plugin for logs, analytics, and production debugging through Cloudflare's remote observability server. |
| `cloudflare-radar` | https://developers.cloudflare.com/agents/model-context-protocol/mcp-servers-for-cloudflare/ | Community package for the official Cloudflare Radar MCP plugin for internet telemetry, traffic trends, and network intelligence through Cloudflare's hosted Radar server. |
| `context7` | https://context7.com | Community package for Context7 MCP plugin for Claude, Codex, Gemini, OpenCode, and Cursor. Pull up-to-date, version-specific documentation and code examples directly from source repositories into agent context. |
| `docker-hub` | https://github.com/docker/hub-mcp | Community package for the official Docker Hub MCP plugin for repository, image, and Docker Hub workflows through Docker's containerized stdio server. |
| `figma` | https://developers.figma.com/docs/figma-mcp-server/remote-server-installation/ | Community package for the official Figma MCP plugin for design context, code-to-design workflows, and authenticated access to Figma Design, Make, and FigJam through Figma's remote MCP service. |
| `firebase` | https://firebase.google.com | Google Firebase MCP integration. Manage Firestore databases, authentication, cloud functions, hosting, and storage. Build and manage your Firebase backend directly from your development workflow. |
| `github` | https://github.com/features | Community Agent Plugin for repository, issue, pull request, review, and search workflows through GitHub's MCP server. |
| `gitlab` | https://gitlab.com | Community package for GitLab MCP plugin for Claude, Codex, Gemini, OpenCode, and Cursor. Connect agents to GitLab so they can inspect projects, issues, merge requests, and related DevOps workflows through the official GitLab MCP server over HTTP. |
| `greptile` | https://greptile.com | Community package for the official Greptile MCP integration for repository search and code intelligence tooling. |
| `heroku` | https://devcenter.heroku.com/articles/heroku-remote-mcp-server | Community package for the official Heroku hosted MCP plugin for apps, add-ons, logs, Postgres, and operational workflows through Heroku's remote MCP service. |
| `hubspot-crm` | https://developers.hubspot.com/mcp | Community package for the official HubSpot remote MCP integration for beta, read-only CRM object access through HubSpot's hosted MCP server. |
| `hubspot-developer` | https://developers.hubspot.com/mcp | Local HubSpot Developer MCP integration for project scaffolding, CMS, builds, logs, and app workflows via the HubSpot CLI. |
| `linear` | https://linear.app | Community package for the official Linear MCP integration for workspace management, issues, and project workflows. |
| `neon` | https://mcp.neon.tech/ | Community package for the official Neon hosted MCP plugin for database, branch, and project workflows through Neon's remote MCP server. |
| `notion` | https://developers.notion.com/docs/get-started-with-mcp | Community package for the official Notion hosted MCP plugin for user-authorized workspace access. Search pages, read docs, edit content, and work with Notion knowledge directly from AI agents. |
| `sentry` | https://mcp.sentry.dev | Community package for the official Sentry hosted MCP plugin for human-in-the-loop debugging, issue triage, and incident workflows through Sentry's remote MCP service. |
| `statsig` | https://docs.statsig.com/integrations/mcp/overview | Community package for the official Statsig MCP plugin for experiments, feature flags, configs, metrics, and console workflows through Statsig's hosted MCP service. |
| `stripe` | https://docs.stripe.com/mcp | Community package for the official Stripe hosted MCP plugin for payments, billing, customer, and documentation workflows through Stripe's remote MCP service. |
| `supabase` | https://supabase.com/docs/guides/ai-tools/mcp | Community package for the official Supabase MCP integration for development and database operations from agent workflows. |
| `vercel` | https://vercel.com/docs/agent-resources/vercel-mcp | Community package for the official Vercel hosted MCP plugin for project, deployment, log, and documentation workflows through Vercel's remote MCP service. |
