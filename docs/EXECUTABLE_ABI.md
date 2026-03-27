# Executable Plugin ABI

`plugin-kit-ai` supports an executable plugin ABI for runtimes beyond Go. This ABI is a low-level contract for repository-local executable plugins. The Go SDK remains the first-class typed authoring path.

Current status: `public-beta`.

## Invocation

Claude hooks:

- command shape: `<entrypoint> <HookName>`
- payload carrier: `stdin_json`
- example: `./bin/my-plugin Stop`

Codex `Notify`:

- command shape: `<entrypoint> notify <json-payload>`
- payload carrier: `argv_json`
- example: `./bin/my-plugin notify '{"client":"codex-tui"}'`

## Response Rules

- stdout is reserved for the upstream hook response payload
- stderr is reserved for diagnostics and human-readable errors
- exit code must be passed through unchanged by any launcher layer
- launcher scripts must not parse or transform hook payloads
- launcher scripts must not rewrite stdout

For Claude hooks, stdout must match Claude's upstream hook output format for the invoked event.
For Codex `notify`, successful completion is represented by exit code `0`; stdout is typically empty.

## Execution Model

- Go projects may use direct executable mode
- interpreted runtimes (`python`, `node`, `shell`) use a stable entrypoint plus a launcher wrapper
- the stable entrypoint path is recorded in `plugin.yaml` for new projects
- Windows launcher resolution is platform-aware:
  - `python`: `.venv\Scripts\python.exe`, then `python`, then `python3`
  - `shell`: requires `bash` in `PATH`
  - generated launcher files use `.cmd`, while config entrypoints remain extensionless such as `./bin/my-plugin`
- TypeScript is not a first-class runtime; supported usage is compile-to-JavaScript and run through the `node` runtime entrypoint

Legacy note:

- older executable projects may still use `.plugin-kit-ai/project.toml` until migrated

The launcher is intentionally minimal:

- discover the runtime
- locate the project root from the launcher path
- `exec` the runtime target with original argv
- preserve stdin/stdout/stderr/exit code

Current hardening coverage:

- generated launcher smoke exists for `go`, `python`, `node`, and `shell`
- ABI passthrough e2e verifies stdin/stdout/stderr/exit-code preservation
- CI includes a dedicated `polyglot-smoke` lane for Ubuntu and Windows

## Non-Goals In This Iteration

- managed dependency installation for interpreted runtimes
- release/install packaging for Python or Node ecosystems
- TypeScript-specific runtime support
- ABI changes to Claude or Codex wire formats
