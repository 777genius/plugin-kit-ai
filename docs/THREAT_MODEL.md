# Threat Model

This document captures the current trust boundaries for shipped `plugin-kit-ai` surfaces.

## Trust Boundaries

| Boundary | Untrusted Input | Trusted Component | Current Mitigation | Remaining Gap |
|----------|-----------------|-------------------|--------------------|---------------|
| Runtime payload decode | Claude stdin JSON, Codex argv JSON | descriptor-backed codec and runtime dispatch | explicit decode functions, typed handlers, decode error path | no formal size limits yet |
| Invocation args/env | CLI args and environment variables | resolver and process envelope builder | explicit invocation resolution, unknown invocation failure | ambient env remains broad outside targeted ports |
| Scaffold/config files | local mutable repo files | generated scaffold + validate rules | required/forbidden file checks, `go build` validation | no schema-level config validation yet |
| Release assets | GitHub release metadata, archives, raw binaries | installer selector/checksum/fs pipeline | checksum verification, asset selection policy, atomic writes | no signed provenance verification |

## High-Signal Risks And Coverage

- malformed Codex notify payload: covered by runtime regression tests
- malformed Claude hook payload: covered by subprocess and runtime decode tests
- checksum mismatch or missing checksums: covered by installer tests and exit-code contract
- asset ambiguity: covered by installer selector tests
- mixed scaffold markers causing wrong platform assumptions: covered by validate tests

## Accepted Gaps For This Phase

- no signed release verification
- no formal payload size ceilings
- no new public debug API
- real external CLI smoke still depends on opt-in local auth and network conditions
