# Skill Examples

These examples are intentionally small, but each one demonstrates a real beta-adoption path for `plugin-kit-ai skills`.

- `go-command-lint`
  - canonical `SKILL.md` plus a Go command entrypoint
  - shows the recommended typed executable path
- `cli-wrapper-formatter`
  - canonical `SKILL.md` that wraps an existing external formatter command
  - shows that the subsystem is not Go-only
- `docs-only-review`
  - canonical `SKILL.md` with no executable command
  - shows that a skill can stay instruction-only

Each example keeps the authored source under `skills/<name>/SKILL.md` and commits rendered outputs under `generated/skills/...`.
Treat the authored `SKILL.md` as canonical and the generated outputs as disposable render targets.
