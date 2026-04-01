---
title: "Модель target’ов"
description: "Как plugin-kit-ai делит runtime, package, extension и workspace-config target’ы."
canonicalId: "page:concepts:target-model"
section: "concepts"
locale: "ru"
generated: false
translationRequired: true
---

# Модель target’ов

`plugin-kit-ai` поддерживает несколько типов target’ов, и на практике они означают разное.

## Короткое правило

- выбирайте runtime target, когда нужен исполняемый плагин
- выбирайте package или extension target, когда результатом должен быть артефакт для публикации или установки
- выбирайте workspace-config target, когда репозиторий должен управлять конфигурацией редактора или инструмента

<MermaidDiagram
  :chart="`
flowchart TD
  Goal[What are you shipping] --> Runtime{Executable plugin}
  Goal --> Package{Install or publish artifact}
  Goal --> Workspace{Repo owned workspace config}
  Runtime --> CodexRuntime[codex-runtime]
  Runtime --> Claude[claude]
  Package --> CodexPackage[codex-package]
  Package --> Gemini[gemini]
  Workspace --> OpenCode[opencode]
  Workspace --> Cursor[cursor]
`"
/>

## Runtime target’ы

Выбирайте runtime target’ы, когда проект сам владеет исполняемым поведением плагина.

Примеры:

- `codex-runtime`
- `claude`

Именно здесь сильнее всего важны выбор runtime, поведение обработчиков и strict validation.

## Package и extension target’ы

Используйте их, когда ваша модель поставки завязана на публикацию или установку артефакта, а не на локальный запуск плагина из репозитория.

Примеры:

- `codex-package`
- `gemini`

Эти target’ы нужны для правильных package или extension artifacts. Они не дают тот же контракт, что основной путь с исполняемым runtime-плагином.

## Workspace-config target’ы

Выбирайте их, когда нужно управлять конфигурацией и интеграцией на уровне репозитория, а не делать исполняемый плагин.

Примеры:

- `opencode`
- `cursor`

Эти target’ы полезны, но их не надо путать с главным runtime-путём.

## Практическое правило

- выбирайте runtime target’ы, когда нужен исполняемый плагин
- выбирайте package или extension target’ы, когда продуктом является артефакт для публикации или установки
- выбирайте workspace-config target’ы, когда настоящая цель — конфигурация под управлением репозитория

## Важное различие

Один проект не обязан навсегда означать только один target.

Ключевая граница здесь не в духе "одно имя starter’а навсегда". Ключевая граница такая:

- один managed authored project
- явный выбор основного target’а
- честные ожидания по поддержке для каждого rendered output

Публичное объяснение этой более широкой модели описано в [Один проект, несколько target’ов](/ru/guide/one-project-multiple-targets).

См. [Поддержку target’ов](/ru/reference/target-support) для компактной support matrix, [Границу поддержки](/ru/reference/support-boundary) для публичного контракта и [Package и workspace targets](/ru/guide/package-and-workspace-targets) для практического выбора пути.
