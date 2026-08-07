# Chrome DevTools

Portable Agent Plugins package for Chrome DevTools MCP. Inspect pages, automate flows, analyze performance, and debug browser state.

This is an independent community package for [Agent Plugins 1.0](https://agent-plugins.org/specification). It is not an endorsement or an official package from Chrome DevTools.

- Component: MCP server
- Transport: `stdio`
- Upstream documentation: https://github.com/ChromeDevTools/chrome-devtools-mcp
- Authentication: No service credential is declared by the package. The launched browser controls its own session.

Review the server's tools, scopes, and write capabilities before enabling it. Agent Plugins 1.0 standardizes packaging, not permissions or sandboxing.
