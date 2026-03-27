# repotests

Интеграционные и guard-тесты **монорепозитория** hookplex (пакет `hookplexrepo_test` в корневом модуле `github.com/hookplex/hookplex`).

## Что здесь

- **Guard:** `TestSDKModule`, `TestCLIModule`, `TestPlugininstallModule` — подпроцессом гоняют `go test ./...` в `sdk/hookplex`, `cli/hookplex`, `install/plugininstall`.
- **Интеграция:** `hookplex install` с моком GitHub (`hookplex_install_integration_test.go`), install compatibility matrix (`hookplex_install_compatibility_test.go`), `hookplex init` + сгенерированный модуль (`cli_init_integration_test.go`).
- **CLI introspection:** `hookplex capabilities` integration check.
- **Live GitHub:** `TestLiveInstall_*` — только с **`HOOKPLEX_E2E_LIVE=1`** и без `-short`; см. `make test-e2e-live`.
- **Claude / hookplex-e2e:** JSON-фикстуры в `testdata/e2e_claude/` и opt-in real CLI smoke — **`HOOKPLEX_RUN_CLAUDE_CLI=1`**, флаг **`-args -claude-model=...`**.
- **Codex / hookplex-e2e:** opt-in real CLI smoke — **`HOOKPLEX_RUN_CODEX_CLI=1`**, флаг **`-args -codex-model=...`**. Для hermetic smoke `notify` подаётся через CLI config override; project-scoped `.codex/config.toml` остаётся частью scaffold/validate contract, а не runtime env prerequisite теста.
- Codex smoke intentionally distinguishes repo failures from Codex runtime-environment failures. If `codex exec` hits known runtime panics before the hook fires, the test skips instead of reporting a false hookplex regression.

## Линии запуска

- **required:** обычный `go test ./...`; deterministic unit/integration/guard coverage
- **extended:** subprocess smoke с локально установленными CLI и opt-in env vars
- **nightly/live:** реальные внешние зависимости, live install checks и Claude live-model sanity

Практическое правило:

- external CLI smoke лучше запускать отдельными `go test` invocation’ами на платформу, даже внутри одного lane; это снижает риск смешанных hangs из-за дочерних процессов и tool-specific runtime.

## Переменные окружения

| Переменная | Назначение |
|------------|------------|
| `HOOKPLEX_REPO_ROOT` | Редко: переопределить корень репо (по умолчанию — walk-up по `go.mod` с `module github.com/hookplex/hookplex`). |
| `HOOKPLEX_E2E_LIVE=1` | Включить live-тесты против github.com. |
| `HOOKPLEX_E2E_NOTIFICATIONS_TAG` | Тег для pinned live-теста (по умолчанию `v1.34.0`). |
| `HOOKPLEX_E2E_TARBALL_OWNER_REPO` | Опциональный live tarball repo для install compatibility smoke. |
| `HOOKPLEX_E2E_TARBALL_TAG` | Тег для live tarball compatibility smoke. |
| `HOOKPLEX_E2E_TARBALL_BINARY` | Ожидаемое имя установленного бинаря для live tarball smoke. |
| `HOOKPLEX_E2E_UNSUPPORTED_OWNER_REPO` | Опциональный live repo с неподдерживаемым layout для clean-failure smoke. |
| `HOOKPLEX_E2E_UNSUPPORTED_TAG` | Тег для unsupported live smoke. |
| `HOOKPLEX_E2E_UNSUPPORTED_EXPECT_EXIT` | Ожидаемый exit code unsupported live smoke. |
| `HOOKPLEX_E2E_UNSUPPORTED_SUBSTRING` | Опциональная diagnostic substring для unsupported live smoke. |
| `GITHUB_TOKEN` | Опционально для API при live / rate limit. |
| `HOOKPLEX_RUN_CLAUDE_CLI=1` | Реальный бинарник `claude` для CLI E2E. |
| `HOOKPLEX_SKIP_CLAUDE_CLI=1` | Явно выключить CLI E2E. |
| `HOOKPLEX_E2E_CLAUDE` | Путь к бинарнику `claude`, если не в `PATH`. |
| `HOOKPLEX_RUN_CODEX_CLI=1` | Реальный бинарник `codex` для CLI E2E. |
| `HOOKPLEX_SKIP_CODEX_CLI=1` | Явно выключить Codex CLI E2E. |
| `HOOKPLEX_E2E_CODEX` | Путь к бинарнику `codex`, если не в `PATH`. |
| `HOOKPLEX_BIND_TESTS=1` | Явно включить bind/listen-зависимые install/integration tests в средах, где loopback может быть недоступен. |

## Запуск

Из корня репозитория:

```bash
make test-required
make test-extended
make test-live-cli
make test-e2e-live
```
