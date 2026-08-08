# Cloudflare Docs

Community package for the official Cloudflare hosted MCP plugin for up-to-date Cloudflare documentation and reference lookups through Cloudflare's remote documentation server.

<!-- agentplugins-install:start -->
## Install

```bash
npx agentplugins@0.1.0-beta.1 add cloudflare-docs
```
<!-- agentplugins-install:end -->

This is an independent community package for [Agent Plugins 1.0](https://agent-plugins.org/specification). It is not an endorsement or an official package from Cloudflare Docs.

- Component: MCP server
- Transport: `streamable-http`
- Upstream documentation: https://developers.cloudflare.com/agents/model-context-protocol/mcp-servers-for-cloudflare/
- Authentication: The documentation server is public; the client manages any connection prompts.

Review the server's tools, scopes, and write capabilities before enabling it. Agent Plugins 1.0 standardizes packaging, not permissions or sandboxing.
