# repotests

Интеграционные и guard-тесты **монорепозитория** plugin-kit-ai (пакет `plugin-kit-airepo_test` в корневом модуле `github.com/777genius/plugin-kit-ai`).

## Что здесь

- **Guard:** `TestSDKModule`, `TestCLIModule`, `TestPlugininstallModule` — подпроцессом гоняют `go test ./...` в `sdk`, `cli/plugin-kit-ai`, `install/plugininstall`.
- **Интеграция:** `plugin-kit-ai install` с моком GitHub (`plugin-kit-ai_install_integration_test.go`), install compatibility matrix (`plugin-kit-ai_install_compatibility_test.go`), `plugin-kit-ai init` + сгенерированный модуль (`cli_init_integration_test.go`).
- **Plugin manifest lifecycle:** CLI workflow `import -> normalize -> render -> validate --strict` для package-standard проектов и current native target imports.
- **CLI introspection:** `plugin-kit-ai capabilities` integration check.
- **Contract clarity:** generated support metadata and public docs stay aligned.
- **Production examples:** reference Claude/Codex plugin repos stay render-clean, strict-valid, buildable, and locally smokeable.
- **Live GitHub:** `TestLiveInstall_*` — только с **`PLUGIN_KIT_AI_E2E_LIVE=1`** и без `-short`; см. `make test-e2e-live`.
- **Claude / plugin-kit-ai-e2e:** JSON-фикстуры в `testdata/e2e_claude/` и opt-in real CLI smoke — **`PLUGIN_KIT_AI_RUN_CLAUDE_CLI=1`**, флаг **`-args -claude-model=...`**.
- **Codex / plugin-kit-ai-e2e:** opt-in real CLI smoke — **`PLUGIN_KIT_AI_RUN_CODEX_CLI=1`**, флаг **`-args -codex-model=...`**. Для hermetic smoke `notify` и `mcp get` могут подаваться через CLI config override; project-scoped `.codex/config.toml` остаётся частью scaffold/validate contract, а не runtime env prerequisite теста.
- **Cursor / workspace-config live smoke:** opt-in real CLI smoke — **`PLUGIN_KIT_AI_RUN_CURSOR_CLI=1`**, флаг **`-args -cursor-model=...`**. Тесты проверяют documented repo-local subset: `.cursor/mcp.json`, `.cursor/rules/**`, optional root `AGENTS.md`, structured `--output-format`, и deterministic local MCP tool invocation.
- **Gemini / extension lifecycle:** opt-in real CLI smoke — **`PLUGIN_KIT_AI_GEMINI_BIN=/path/to/gemini`** or **`GEMINI_BIN=/path/to/gemini`**. Тест работает в `Temp HOME` и проверяет `gemini extensions link|config|disable|enable` против rendered reference extension, не трогая реальный user state.
- **Gemini / runtime hooks:** extra opt-in real CLI smoke — **`PLUGIN_KIT_AI_RUN_GEMINI_RUNTIME_LIVE=1`** плюс `PLUGIN_KIT_AI_GEMINI_BIN`/`GEMINI_BIN`. Тест собирает generated Gemini Go runtime repo, линкует extension в isolated home и проверяет реальный `SessionStart` hook dispatch через trace helper.
- **Portable MCP multi-agent live smoke:** opt-in shared authored-config suite — **`PLUGIN_KIT_AI_RUN_PORTABLE_MCP_LIVE=1`** плюс target-specific live env vars. Один `mcp/servers.yaml` рендерится в Claude, Codex package, Gemini, OpenCode и Cursor; дальше suite проверяет реальный vendor CLI path в честной для каждой платформы глубине: Claude `mcp get` preflight plus `--mcp-config` against a config synthesized from the rendered `.mcp.json`, Codex `mcp get` preflight plus `exec`, Gemini `extensions link` plus `extensions list`, Cursor live MCP tool call, OpenCode `serve` init smoke. Если Claude/Codex CLI видит projected MCP config, но конкретная print/exec session не экспонирует tool или модель завершает задачу без tool call, тест делает explicit skip как vendor-session variability, а не ложный red на portable MCP projection.
- Codex smoke intentionally distinguishes repo failures from Codex runtime-environment failures. If `codex exec` hits known runtime panics before the hook fires, the test skips instead of reporting a false plugin-kit-ai regression.
- Codex live notify coverage has two layers: the stable real-CLI smoke uses explicit `-c notify=...` override to prove `codex exec` still invokes the hook path end-to-end, while `TestCodexCLINotifyUsesRenderedProjectConfig` probes whether the current Codex build actually honors project-local `.codex/config.toml` during `exec`. If the vendor build ignores project-local config, that probe skips with the captured live evidence instead of producing a false repo regression.
- Codex live MCP coverage also has a positive preflight: `TestCodexCLIMCPGetWithOverride` uses explicit `-c mcp_servers...` overrides against the real CLI to prove `codex mcp get --json` still surfaces the projected server contract end-to-end without depending on undocumented project-config behavior.
- Codex package live MCP coverage also has a rendered-bundle path: `TestCodexPackageMCPGetUsesRenderedSidecar` reads the rendered `.mcp.json` from a real `codex-package` workspace, synthesizes documented `-c mcp_servers...` overrides from that sidecar, and probes `codex mcp get --json`; `TestCodexPackageExecUsesRenderedSidecarMCP` then asks the real CLI to call that MCP tool through `codex exec`, skipping only if the current model session declines to surface the tool after a successful config preflight.
- Codex package live MCP coverage also includes the checked-in production example: `TestCodexPackageProductionExampleMCPGetUsesRenderedSidecar` and `TestCodexPackageProductionExampleMCPListUsesRenderedSidecar` render `examples/plugins/codex-package-prod`, project its rendered `.mcp.json` back into documented `-c mcp_servers...` overrides, and verify real `codex mcp get --json` plus `codex mcp list --json` against the live CLI.
- Codex live config coverage also includes `TestCodexCLIMCPUsesRenderedProjectConfig`, which renders `targets/codex-runtime/config.extra.toml` into `.codex/config.toml` and probes `codex mcp get` against the real CLI. If the current Codex build ignores project-local MCP config, that test also skips with the captured live output instead of inventing support.

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
| `PLUGIN_KIT_AI_GEMINI_BIN` | Путь к бинарнику `gemini` для local Gemini extension E2E. |
| `GEMINI_BIN` | Fallback-путь к бинарнику `gemini` для local Gemini extension E2E. |
| `PLUGIN_KIT_AI_RUN_GEMINI_RUNTIME_LIVE=1` | Включить реальный Gemini runtime hook smoke поверх generated Go beta lane. |
| `PLUGIN_KIT_AI_BIND_TESTS=1` | Явно включить bind/listen-зависимые install/integration tests в средах, где loopback может быть недоступен. |

## Запуск

Из корня репозитория:

```bash
make test-required
make test-plugin-manifest-workflow
make test-install-compat
make test-extended
make test-cursor-live
make test-portable-mcp-live
make test-live-cli
make test-e2e-live
```
