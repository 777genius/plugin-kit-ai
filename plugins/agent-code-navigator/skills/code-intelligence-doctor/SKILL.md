---
name: code-intelligence-doctor
description: Diagnose whether local code intelligence tools are installed and available. Use when exact search, semantic search, LSP navigation, or call-graph analysis appears missing or inconsistent.
license: Apache-2.0
compatibility: Requires Bash. Checks rg and optional uv, Semble, CodeGraphContext, and Serena installations without modifying them.
---

# Code Intelligence Doctor

Change to the directory containing this `SKILL.md`, then run the bundled
read-only diagnostic:

```bash
bash ./scripts/doctor.sh
```

The script checks:

- `rg`
- `uv` and `uvx`
- Semble `0.5.4`
- CodeGraphContext `0.5.6`
- optional Serena `1.6.1`

It does not install packages, index repositories, start MCP servers, edit
configuration, or write project files. Treat missing specialized tools as a
warning and fall back to exact search and direct source reads.
