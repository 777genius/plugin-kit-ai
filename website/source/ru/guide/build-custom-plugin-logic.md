---
title: "Build Custom Plugin Logic"
description: "Advanced путь для плагинов, в которых ценность живёт в runtime code, hooks и orchestration."
canonicalId: "page:guide:build-custom-plugin-logic"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Build Custom Plugin Logic

Выбирайте этот путь, когда плагин не просто подключает существующий сервис или локальный инструмент.

Это advanced path для repo, в которых ценность живёт в:

- runtime code, которым владеете вы
- hooks и orchestration logic
- policy, transformation или guardrail behavior
- custom behavior, которого не было бы без вашего кода

Если вы подключаете hosted service вроде Notion или Stripe, откройте [Что именно вы собираете](/ru/guide/choose-what-you-are-building) и начните с `online-service`.
Если вы подключаете local tool вроде Docker Hub или HubSpot Developer, начните с `local-tool`.

## Стартуйте отсюда

```bash
plugin-kit-ai init my-plugin --template custom-logic
cd my-plugin
plugin-kit-ai inspect . --authoring
go mod tidy
plugin-kit-ai validate . --platform codex-runtime --strict
plugin-kit-ai test . --platform codex-runtime --event Notify
```

Для стартового Go scaffolding один раз запустите `go mod tidy`, чтобы проект записал `go.sum` перед первым циклом validate или test.

## Что вы редактируете

Authored source of truth живёт под `plugin/`.

Обычно важны такие файлы:

- `plugin/plugin.yaml`
- `plugin/launcher.yaml`
- `plugin/targets/...`
- ваш runtime entrypoint вроде `cmd/<name>/main.go` или `plugin/main.*`

Используйте `plugin-kit-ai inspect . --authoring`, когда нужно точно увидеть границу между editable source, managed guidance files и generated target outputs.

## Что генерируется

`plugin-kit-ai generate` по-прежнему владеет generated output files в корне repo.

Обычно это включает:

- root guidance files вроде `README.md`, `CLAUDE.md`, `AGENTS.md` и `GENERATED.md`
- native output для target'а, который вы ship'ите, например `.codex/config.toml`, `hooks/hooks.json` или `gemini-extension.json`

Редактируйте source под `plugin/`.
Root outputs воспринимайте как managed outputs.

## Почему этот путь более advanced

По сравнению с `online-service` и `local-tool` этот путь даёт:

- больше контроля над поведением
- больше ответственности за runtime contract
- больше пространства для tests, hooks и policy logic

Именно поэтому он остаётся на первом экране, но помечается как advanced path.

## Первый запуск по runtime shape

### Go runtime

```bash
go mod tidy
go test ./...
plugin-kit-ai validate . --platform codex-runtime --strict
plugin-kit-ai test . --platform codex-runtime --event Notify
```

### Node или Python runtime

```bash
plugin-kit-ai doctor .
plugin-kit-ai bootstrap .
plugin-kit-ai validate . --platform codex-runtime --strict
plugin-kit-ai test . --platform codex-runtime --event Notify
```

## Куда идти глубже

- Откройте [Быстрый старт](/ru/guide/quickstart), если хотите сравнить этот путь с более простыми job-first starter'ами.
- Откройте [Создайте первый plugin](/ru/guide/first-plugin), если вам нужен узкий legacy-compatible tutorial для Codex runtime.
- Откройте [Выбор target](/ru/guide/choose-a-target), когда понадобятся конкретные решения по способу поставки.
- Откройте [Один проект, несколько target'ов](/ru/guide/one-project-multiple-targets), когда repo будет готов расти в несколько outputs.
