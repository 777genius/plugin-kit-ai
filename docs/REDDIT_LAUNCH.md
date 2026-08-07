# Reddit launch draft

Use this only after the repository is public and the validation workflow is
green. Replace every bracketed value with a verified fact.

## Suggested title

Showcase: I rebuilt 26 MCP and Agent Skill packages for the new Agent Plugins 1.0 format

## Draft

Disclosure: I maintain this project.

OpenAI and its launch partners recently introduced Agent Plugins, a shared,
vendor-neutral package format for Agent Skills and MCP server configurations.

I already maintained a cross-agent plugin catalog, but it relied on a custom
source format and generated separate client packages. I rebuilt the portable
catalog natively around Agent Plugins 1.0:

- root `plugin.json` manifests
- standard `mcp.json` configurations
- Agent Skills under `skills/`
- no dependency on the old generator library
- official schema and semantic validation
- pinned stdio dependencies
- vendor source and authentication notes for every package

Current status:

- 26 portable packages
- 25 MCP packages and 1 skills-first code-navigation package
- [CI link]
- [clients and scenarios actually tested]

This does not mean every package behaves identically in every client.
Installation, authentication, permissions, and supported transports remain
client-specific. Four Anthropic-hosted connectors were intentionally excluded
instead of being mislabeled as portable.

Repo: [GitHub URL]

Spec: https://agent-plugins.org/

I would especially value feedback on:

1. Which client should be added to the verification matrix first?
2. Which authentication flow is still unclear?
3. What evidence would make you trust a community plugin catalog?

This is an independent open-source project and is not affiliated with or
endorsed by OpenAI or the service vendors.

## Posting notes

- Start with `r/mcp`, then adapt the post for `r/codex` or `r/OpenAI`.
- Read the current subreddit rules immediately before posting.
- Do not request stars or upvotes.
- Do not make identical cross-posts on the same day.
- Include real test evidence and answer technical questions in the comments.
