# GitHub

Community Agent Plugin for repository, issue, pull request, review, and search workflows through GitHub's MCP server.

<!-- agentplugins-install:start -->
## Install

```bash
npx universal-agent-plugins add github
```
<!-- agentplugins-install:end -->

This is an independent community package for [Agent Plugins 1.0](https://agent-plugins.org/specification). It is not an endorsement or an official package from GitHub.

- Component: MCP server
- Transport: `streamable-http`
- Upstream documentation: https://github.com/features
- Authentication: Authentication is client-managed. No PAT or Authorization header is stored in this package.

Review the server's tools, scopes, and write capabilities before enabling it. Agent Plugins 1.0 standardizes packaging, not permissions or sandboxing.
