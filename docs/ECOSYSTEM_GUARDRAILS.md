# Ecosystem and governance guardrails

These are binding project rules for Universal Agent Plugins (UAP). They keep
the CLI useful to users without turning UAP-specific behavior into hidden
requirements for the Agent Plugins standard.

## Product boundary

- UAP is an independent, community-maintained installer and lifecycle manager.
- Agent Plugins is the vendor-neutral package standard. Its published
  specification and canonical schemas are the only normative authority.
- The Directory is optional discovery infrastructure, not part of the standard
  and not a requirement for installation.
- A package can be installed from a local directory or immutable Git source
  without being listed in the Directory.

## Specification fidelity

- `plugin.json` is the authoritative portable manifest. UAP MUST NOT require a
  second UAP-specific manifest for a standard package.
- Published schema identifiers are matched explicitly to locally pinned schema
  bytes. Unknown versions fail closed; schemas are never fetched while loading.
- UAP MUST distinguish standard validity from runtime readiness. Executable
  permissions, local client support, activation, OAuth, and tool health belong
  to install/runtime policy, not portable package validation.
- Unknown standard fields are preserved or reported exactly as the published
  version requires. UAP MUST NOT assign them private semantics.
- Agent Plugins 1.1 remains unsupported while it is a working draft. Support is
  considered only after publication against an exact release and schema digest.

## Neutrality

- No vendor receives privileged portable semantics. Client-specific adapters
  may differ only where client capabilities or activation flows actually differ.
- Provenance labels are factual: upstream means the package is physically in
  the owner's repository; community bridge and community are not vendor
  endorsement.
- Search ranking and compatibility claims MUST use documented signals and MUST
  not present discovery, schema validity, installation, activation, runtime,
  OAuth, or human review as equivalent evidence.
- A future transfer to the official `agentplugins` organization would require a
  Technical Steering Committee decision. Repository placement alone would not
  make UAP an official implementation.

## Interoperability

Interoperability means that independent implementations apply the same
mandatory package contract and observable failure boundaries, while documenting
the choices the specification deliberately leaves optional. A valid package
must not depend on one installer's private behavior.

- Maintain a process adapter that exposes the UAP loader without installing a
  package or starting an MCP server.
- Run the community conformance corpus proposed in
  [agent-plugins-spec discussion #81](https://github.com/agentplugins/agent-plugins-spec/discussions/81)
  in CI with strict reporting.
- Pin the corpus to an exact commit. Its `main` branch and npm release have
  already differed, so updates require a reviewed pull request rather than
  silently changing the gate.
- A green third-party corpus is interoperability evidence, not official
  certification. UAP's own security, lifecycle, adapter, and runtime tests remain
  separate gates.

## Distribution and governance

The official [technical charter](https://github.com/agentplugins/agent-plugins-spec/blob/main/GOVERNANCE.md)
includes reference implementations, conformance tests, documentation, and
tooling that support interoperable ecosystems. That scope makes collaboration
possible; it does not automatically make any existing installer official.

If the TSC expresses interest in adopting or incubating UAP, it must be possible
for the governed project to control and reproduce all release-critical assets:

- installer facade and Go engine source;
- release workflows, artifact provenance, and dependency pins;
- npm package and binary release permissions;
- Directory signing policy and keys, if the Directory is included;
- compatibility policy, security response, and maintainer succession.

Today the npm facade and product repository live in UAP, while the shared Go
engine is released from `plugin-kit-ai`. This is acceptable for an independent
MVP, but formal adoption requires an explicit ownership decision. Do not hide
this boundary with generated source, a submodule, or branding.

Do not move the engine only to change GitHub's language bar. Start an ownership
migration when the TSC expresses concrete incubation interest or another real
consumer needs an independently versioned engine. The chosen design must have
one authoritative source, preserve release reproducibility, and avoid duplicated
implementations.

## Contribution and communication

- Prefer conformance fixes, reproducible evidence, and neutral interoperability
  contributions before proposing organizational transfer.
- Do not claim endorsement, official status, or conformance certification that
  the TSC has not granted.
- Public proposals, posts in specification discussions, and direct maintainer
  outreach require explicit project-owner approval.

## Pull request checklist

Before merging a loader, Directory, client-adapter, or release change:

1. Which published specification rule or real client capability authorizes it?
2. Does it add a hidden package requirement or vendor-specific portable meaning?
3. Are validation, installation, activation, OAuth, and runtime evidence still
   reported as separate states?
4. Does the pinned conformance corpus still pass without executing plugin code?
5. Can local and immutable Git sources still work without the Directory?
6. Are source ownership and release-critical dependencies visible and
   reproducible?
