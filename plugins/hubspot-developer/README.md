# HubSpot Developer

Preview HubSpot Developer MCP integration for project scaffolding, CMS, builds, logs, and app workflows via the HubSpot CLI.

<!-- agentplugins-install:start -->
## Install

```bash
npx universal-agent-plugins add hubspot-developer --target codex
```
<!-- agentplugins-install:end -->

This is an independent community package for [Agent Plugins 1.0](https://agent-plugins.org/specification). It is not an endorsement or an official package from HubSpot Developer.

- Component: MCP server
- Transport: `stdio`
- Runtime: integrity-locked prerelease `@hubspot/cli@8.14.0-beta.1`; install scripts are disabled, and the platform-optional `fsevents` script is separately lock-accounted
- Preview status: this integration remains preview until HubSpot publishes `8.14.0` stable and that release is reviewed
- Requirement: Node.js 22 or newer; the first launch downloads the locked npm closure into plugin data
- Upstream documentation: https://developers.hubspot.com/mcp
- Privacy: HubSpot CLI usage tracking is disabled by default
- Authentication: Uses the local HubSpot CLI session selected by the user.
- Security review: `npm audit signatures` passes and the exact lockfile reports 0 known vulnerabilities after a reviewed Sentry override

Review the server's tools, scopes, and write capabilities before enabling it. Agent Plugins 1.0 standardizes packaging, not permissions or sandboxing.
