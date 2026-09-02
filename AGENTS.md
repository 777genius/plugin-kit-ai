# Repository instructions

Before changing the package loader, client adapters, Directory, compatibility
claims, or release pipeline, read and follow
`docs/ECOSYSTEM_GUARDRAILS.md`.

In particular:

- treat published Agent Plugins schemas and specification text as the only
  normative portable contract;
- do not add UAP-only requirements or vendor-specific portable semantics;
- keep validation, installation, activation, OAuth, and runtime evidence
  separate;
- keep direct local and immutable Git installation independent of the Directory;
- run the exact pinned conformance corpus for loader changes;
- do not claim official status, endorsement, or certification;
- do not post to external discussions or contact maintainers without explicit
  project-owner approval.
