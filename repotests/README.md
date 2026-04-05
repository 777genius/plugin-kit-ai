# repotests

Интеграционные и guard-тесты **монорепозитория** plugin-kit-ai (пакет `plugin-kit-airepo_test` в корневом модуле `github.com/777genius/plugin-kit-ai`).

## Что здесь

- **Guard:** `TestSDKModule`, `TestCLIModule`, `TestPlugininstallModule` — подпроцессом гоняют `go test ./...` в `sdk`, `cli/plugin-kit-ai`, `install/plugininstall`.
- **Интеграция:** `plugin-kit-ai install` с моком GitHub (`plugin-kit-ai_install_integration_test.go`), install compatibility matrix (`plugin-kit-ai_install_compatibility_test.go`), `plugin-kit-ai init` + сгенерированный модуль (`cli_init_integration_test.go`).
- **Plugin manifest lifecycle:** CLI workflow `import -> normalize -> generate -> validate --strict` для package-standard проектов и current native target imports.
- **CLI introspection:** `plugin-kit-ai capabilities` integration check.
- **Contract clarity:** generated support metadata and public docs stay aligned.
- **Production examples:** reference Claude/Codex plugin repos stay generate-clean, strict-valid, buildable, and locally smokeable.
- **Live GitHub:** `TestLiveInstall_*` — только с **`PLUGIN_KIT_AI_E2E_LIVE=1`** и без `-short`; см. `make test-e2e-live`.
- **Claude / plugin-kit-ai-e2e:** JSON-фикстуры в `testdata/e2e_claude/` и opt-in real CLI smoke — **`PLUGIN_KIT_AI_RUN_CLAUDE_CLI=1`**, флаг **`-args -claude-model=...`**.
- **Codex / plugin-kit-ai-e2e:** opt-in real CLI smoke — **`PLUGIN_KIT_AI_RUN_CODEX_CLI=1`**, флаг **`-args -codex-model=...`**. Для hermetic smoke `notify` и `mcp get` могут подаваться через CLI config override; project-scoped `.codex/config.toml` остаётся частью scaffold/validate contract, а не runtime env prerequisite теста.
- **Cursor / workspace-config live smoke:** opt-in real CLI smoke — **`PLUGIN_KIT_AI_RUN_CURSOR_CLI=1`**, флаг **`-args -cursor-model=...`**. Тесты проверяют documented repo-local subset: `.cursor/mcp.json`, `.cursor/rules/**`, optional root `AGENTS.md`, structured `--output-format`, и deterministic local MCP tool invocation.
- **Gemini / extension lifecycle:** opt-in real CLI smoke — **`PLUGIN_KIT_AI_RUN_GEMINI_CLI=1`** плюс **`PLUGIN_KIT_AI_E2E_GEMINI=/path/to/gemini`**. При необходимости можно явно выключить через **`PLUGIN_KIT_AI_SKIP_GEMINI_CLI=1`**. Тест работает в `Temp HOME` и проверяет `gemini extensions link|config|disable|enable` против generated reference extension, не трогая реальный user state.
- **Gemini / runtime hooks:** extra opt-in real CLI smoke — **`PLUGIN_KIT_AI_RUN_GEMINI_RUNTIME_LIVE=1`** плюс `PLUGIN_KIT_AI_E2E_GEMINI`. Тест собирает generated Gemini Go runtime repo, линкует extension в isolated home и проверяет current production-ready 9-hook runtime: `SessionStart`, `SessionEnd`, model-path (`BeforeModel`/`AfterModel`), tool-selection path (`BeforeToolSelection`), agent-path (`BeforeAgent`/`AfterAgent`) и tool-path (`BeforeTool`/`AfterTool`) через trace helper, включая documented `tool_input`/`tool_response` payload presence. Для tool-path smoke используется explicit tool-use prompt, потому что на текущих Gemini CLI builds это надёжнее, чем инлайнить `@README.md` и надеяться на авто-вызов tool. Плюс live gate сейчас проверяет семь реальных сценариев: happy-path (`response: "OK"` с successful `read_file` stats and zero tool failures), blocked-tool control semantics, где `BeforeTool` deny даёт failed `read_file` vendor stats и отсутствие `AfterTool` trace, blocked-model control semantics, где `BeforeModel` deny даёт пустой `response`, нулевые tool stats и отсутствие `AfterModel`/tool-selection traces, model transform semantics, где `AfterModel` заменяет ответ и Gemini возвращает rewritten response без tool activity, но с уже произошедшим `BeforeToolSelection` planning trace, agent retry semantics, где `AfterAgent` deny один раз вызывает retry и live trace фиксирует второй `AfterAgent` с `stop_hook_active=true`, `BeforeToolSelection` `mode:"NONE"` semantics, где tools отключены ещё на tool-selection stage и tool activity остаётся нулевой, даже если Gemini всё ещё печатает tool-like text in `response`, и transform semantics, где `BeforeTool` переписывает отсутствующий `read_file` path на `README.md` и live trace показывает `rewrite_input`. Отдельные live probes на `BeforeModel synthetic_response` и allowlist / `mode:"ANY"` tool-selection пока не входят в release gate: на текущем `gemini 0.36.0` synthetic-response path игнорировался, а allowlist/ANY probes ушли в vendor `AbortError` или loop/capacity exhaustion. Use `make test-gemini-runtime` as the deterministic release gate for that runtime and `make test-gemini-runtime-live` as the matching opt-in live gate.
- **Portable MCP multi-agent live smoke:** opt-in shared authored-config suite — **`PLUGIN_KIT_AI_RUN_PORTABLE_MCP_LIVE=1`** плюс target-specific live env vars. Один `src/mcp/servers.yaml` рендерится в Claude, Codex package, Gemini, OpenCode и Cursor; дальше suite проверяет реальный vendor CLI path в честной для каждой платформы глубине: Claude `mcp get` preflight plus `--mcp-config` against a config synthesized from the generated `.mcp.json`, Codex `mcp get` preflight plus `exec`, Gemini `extensions link` plus `extensions list`, Cursor live MCP tool call, OpenCode `serve` init smoke. Если Claude/Codex CLI видит projected MCP config, но конкретная print/exec session не экспонирует tool или модель завершает задачу без tool call, тест делает explicit skip как vendor-session variability, а не ложный red на portable MCP projection.
- Codex smoke intentionally distinguishes repo failures from Codex runtime-environment failures. If `codex exec` hits known runtime panics before the hook fires, the test skips instead of reporting a false plugin-kit-ai regression.
- Codex live notify coverage has two layers: the stable real-CLI smoke uses explicit `-c notify=...` override to prove `codex exec` still invokes the hook path end-to-end, while `TestCodexCLINotifyUsesRenderedProjectConfig` probes whether the current Codex build actually honors project-local `.codex/config.toml` during `exec`. If the vendor build ignores project-local config, that probe skips with the captured live evidence instead of producing a false repo regression.
- Codex live runtime coverage also includes the checked-in production example: `TestCodexProductionExampleNotifyUsesRealCLI` copies `examples/plugins/codex-basic-prod`, rebuilds the real example binary, and proves that the live CLI can invoke that checked-in runtime through explicit `-c notify=...` override wiring. The same checked-in example now also has `TestCodexProductionExampleMCPGetWithOverride` and `TestCodexProductionExampleMCPListWithOverride`, which project its generated runtime MCP config back into documented `-c mcp_servers...` overrides and verify real `codex mcp get --json` plus `codex mcp list --json`. `TestCodexProductionExampleNotifyUsesRenderedProjectConfig`, `TestCodexProductionExampleMCPGetUsesRenderedProjectConfig`, and `TestCodexProductionExampleMCPListUsesRenderedProjectConfig` then probe the same example without overrides and skip if the current vendor build still ignores project-local `.codex/config.toml`.
- Those same checked-in production examples now also bridge into the mutable CLI config path: `TestCodexProductionExampleRuntimeMCPAddGetListRemoveInIsolatedHome` and `TestCodexPackageProductionExampleMCPAddGetListRemoveInIsolatedHome` take MCP truth from generated reference examples and verify real `codex mcp add|get|list|remove` in isolated config state. Their stronger auth-seeded counterparts `TestCodexProductionExampleRuntimeMCPAddGetListRemoveInAuthSeededCodexHome` and `TestCodexPackageProductionExampleMCPAddGetListRemoveInAuthSeededCodexHome` prove the same path continues to work inside a temporary authenticated `CODEX_HOME`.
- Codex live MCP coverage also has positive preflights: `TestCodexCLIMCPGetWithOverride` and `TestCodexCLIMCPListWithOverride` use explicit `-c mcp_servers...` overrides against the real CLI to prove `codex mcp get --json` and `codex mcp list --json` still surface the projected server contract end-to-end without depending on undocumented project-config behavior.
- Codex live MCP coverage also includes documented config-management E2E in an isolated temporary home: `TestCodexCLIMCPAddGetListRemoveStdioInIsolatedHome` and `TestCodexCLIMCPAddGetListRemoveHTTPInIsolatedHome` exercise real `codex mcp add|get|list|remove` without touching the user's actual Codex config.
- `TestCodexCLIMCPAddGetListRemoveStdioInAuthSeededCodexHome` adds a stronger positive variant: it seeds a temporary `CODEX_HOME` with live auth artifacts, confirms `codex login status`, and still completes real `mcp add|get|list|remove` for a stdio server end-to-end.
- `TestCodexCLIMCPAddGetListRemoveStdioWithEnvInAuthSeededCodexHome` and `TestCodexCLIMCPAddGetListRemoveHTTPInAuthSeededCodexHome` extend that same auth-seeded live path across the documented `codex mcp add --env ... -- <command>` and `codex mcp add --url ... --bearer-token-env-var ...` shapes.
- `TestCodexCLIMCPLoginLogoutRejectStdioInAuthSeededCodexHome` adds a real negative-auth contract check on top of that same authenticated mutable path: after a documented stdio `mcp add`, real `codex mcp login` and `codex mcp logout` both reject the server with the live CLI's documented OAuth-only diagnostics, while `get`, `list`, and `remove` still keep working.
- `TestCodexCLIMCPMissingServerBehaviorInAuthSeededCodexHome` adds a second real negative contract check: after a documented add/remove cycle, live `codex mcp get` fails with the current missing-server diagnostic, while `codex mcp remove` remains idempotent and returns a no-op message for the same missing entry.
- Codex live MCP coverage also includes `TestCodexCLIMCPAddExecStdioInIsolatedHome`, which probes whether a server added through isolated `codex mcp add` is then usable inside real `codex exec`. On the current CLI build this is evidence-only and may skip if isolated `CODEX_HOME` loses live auth for `exec`, while still preserving the positive `add|get|list|remove` evidence.
- `TestCodexCLIMCPAddExecStdioInAuthSeededCodexHome` goes one step further: it seeds a temporary `CODEX_HOME` with the current live auth artifacts, proves `codex login status` plus `mcp add|get|list` still work there, and then probes `codex exec` against that persisted MCP server. In the current build this can still skip, but now as MCP-tool/session variability rather than auth loss.
- Codex package live MCP coverage also has a generated-bundle path: `TestCodexPackageMCPGetUsesRenderedSidecar` and `TestCodexPackageMCPListUsesRenderedSidecar` read the generated `.mcp.json` from a real `codex-package` workspace, synthesize documented `-c mcp_servers...` overrides from that sidecar, and probe real `codex mcp get --json` plus `codex mcp list --json`; `TestCodexPackageExecUsesRenderedSidecarMCP` then asks the real CLI to call that MCP tool through `codex exec`, skipping only if the current model session declines to surface the tool after a successful config preflight.
- Codex package live MCP coverage also includes the checked-in production example: `TestCodexPackageProductionExampleMCPGetUsesRenderedSidecar` and `TestCodexPackageProductionExampleMCPListUsesRenderedSidecar` generate `examples/plugins/codex-package-prod`, project its generated `.mcp.json` back into documented `-c mcp_servers...` overrides, and verify real `codex mcp get --json` plus `codex mcp list --json` against the live CLI.
- The generated `codex-package` stdio sidecar path is also replayed through the mutable config workflow: `TestCodexPackageRenderedSidecarMCPAddGetListRemoveInIsolatedHome` and `TestCodexPackageRenderedSidecarMCPAddGetListRemoveInAuthSeededCodexHome` take the generated `.mcp.json` from a real package workspace and feed it into documented `codex mcp add|get|list|remove`.
- The same mutable replay now also covers a synthetic generated HTTP sidecar: `TestCodexPackageRenderedHTTPSidecarMCPAddGetListRemoveInIsolatedHome` and `TestCodexPackageRenderedHTTPSidecarMCPAddGetListRemoveInAuthSeededCodexHome` prove that a generated remote `.mcp.json` sidecar survives the same real `codex mcp add|get|list|remove` path.
- Codex live config coverage also includes `TestCodexCLIMCPUsesRenderedProjectConfig` and `TestCodexCLIMCPListUsesRenderedProjectConfig`, which generate `targets/codex-runtime/config.extra.toml` into `.codex/config.toml` and probe `codex mcp get` plus `codex mcp list` against the real CLI. If the current Codex build ignores project-local MCP config, those tests also skip with the captured live output instead of inventing support.

## Линии запуска

- **required:** обычный `go test ./...`; deterministic unit/integration/guard coverage
- **extended:** subprocess smoke с локально установленными CLI и opt-in env vars
- **nightly/live:** реальные внешние зависимости, live install checks и Claude live-model sanity

Практическое правило:

- external CLI smoke лучше запускать отдельными `go test` invocation’ами на платформу, даже внутри одного lane; это снижает риск смешанных hangs из-за дочерних процессов и tool-specific runtime.

## Переменные окружения

| Переменная | Назначение |
|------------|------------|
| `PLUGIN_KIT_AI_REPO_ROOT` | Редко: переопределить корень репо (по умолчанию — walk-up по `go.mod` с `module github.com/777genius/plugin-kit-ai`). |
| `PLUGIN_KIT_AI_E2E_LIVE=1` | Включить live-тесты против github.com. |
| `PLUGIN_KIT_AI_E2E_NOTIFICATIONS_TAG` | Тег для pinned live-теста (по умолчанию `v1.34.0`). |
| `PLUGIN_KIT_AI_E2E_TARBALL_OWNER_REPO` | Опциональный live tarball repo для install compatibility smoke. |
| `PLUGIN_KIT_AI_E2E_TARBALL_TAG` | Тег для live tarball compatibility smoke. |
| `PLUGIN_KIT_AI_E2E_TARBALL_BINARY` | Ожидаемое имя установленного бинаря для live tarball smoke. |
| `PLUGIN_KIT_AI_E2E_UNSUPPORTED_OWNER_REPO` | Опциональный live repo с неподдерживаемым layout для clean-failure smoke. |
| `PLUGIN_KIT_AI_E2E_UNSUPPORTED_TAG` | Тег для unsupported live smoke. |
| `PLUGIN_KIT_AI_E2E_UNSUPPORTED_EXPECT_EXIT` | Ожидаемый exit code unsupported live smoke. |
| `PLUGIN_KIT_AI_E2E_UNSUPPORTED_SUBSTRING` | Опциональная diagnostic substring для unsupported live smoke. |
| `GITHUB_TOKEN` | Опционально для API при live / rate limit. |
| `PLUGIN_KIT_AI_RUN_CLAUDE_CLI=1` | Реальный бинарник `claude` для CLI E2E. |
| `PLUGIN_KIT_AI_SKIP_CLAUDE_CLI=1` | Явно выключить CLI E2E. |
| `PLUGIN_KIT_AI_E2E_CLAUDE` | Путь к бинарнику `claude`, если не в `PATH`. |
| `PLUGIN_KIT_AI_RUN_CODEX_CLI=1` | Реальный бинарник `codex` для CLI E2E. |
| `PLUGIN_KIT_AI_SKIP_CODEX_CLI=1` | Явно выключить Codex CLI E2E. |
| `PLUGIN_KIT_AI_E2E_CODEX` | Путь к бинарнику `codex`, если не в `PATH`. |
| `PLUGIN_KIT_AI_RUN_PORTABLE_MCP_LIVE=1` | Включить shared portable MCP live suite поверх реальных CLI. |
| `PLUGIN_KIT_AI_PORTABLE_MCP_CODEX_FALLBACK_MODEL` | Опциональная fallback-model для Codex portable MCP live suite (по умолчанию `gpt-5.4`). |
| `PLUGIN_KIT_AI_RUN_CURSOR_CLI=1` | Реальный бинарник `cursor-agent` для CLI E2E. |
| `PLUGIN_KIT_AI_SKIP_CURSOR_CLI=1` | Явно выключить Cursor CLI E2E. |
| `PLUGIN_KIT_AI_E2E_CURSOR` | Путь к бинарнику `cursor-agent`, если не в `PATH`. |
| `PLUGIN_KIT_AI_RUN_GEMINI_CLI=1` | Включить реальный Gemini extension lifecycle smoke. |
| `PLUGIN_KIT_AI_E2E_GEMINI` | Предпочтительный путь к бинарнику `gemini` для Gemini CLI E2E. |
| `PLUGIN_KIT_AI_SKIP_GEMINI_CLI=1` | Явно выключить Gemini CLI E2E. |
| `PLUGIN_KIT_AI_RUN_GEMINI_RUNTIME_LIVE=1` | Включить реальный Gemini runtime hook smoke поверх generated Go runtime lane. |
| `PLUGIN_KIT_AI_BIND_TESTS=1` | Явно включить bind/listen-зависимые install/integration tests в средах, где loopback может быть недоступен. |

## Запуск

Из корня репозитория:

```bash
make test-required
make test-plugin-manifest-workflow
make test-install-compat
make test-extended
make test-gemini-runtime
make test-gemini-runtime-live
make test-cursor-live
make test-portable-mcp-live
make test-live-cli
make test-e2e-live
```
