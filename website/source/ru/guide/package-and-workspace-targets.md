---
title: "Package и workspace targets"
description: "Как использовать Codex package, Gemini, OpenCode и Cursor, не путая их с путями для исполняемых плагинов."
canonicalId: "page:guide:package-and-workspace-targets"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Package и workspace targets

Не каждый target в `plugin-kit-ai` является путём для исполняемого плагина.

Читайте эту страницу перед выбором `codex-package`, `gemini`, `opencode` или `cursor`, потому что эти targets решают другие задачи, чем `codex-runtime` и `claude`.

## Короткое правило

- выбирайте `codex-runtime` или `claude`, когда продуктом является исполняемый плагин
- выбирайте `codex-package` или `gemini`, когда продуктом являются package или extension artifacts
- выбирайте `opencode` или `cursor`, когда продуктом является конфигурация workspace внутри репозитория

<MermaidDiagram
  :chart="`
flowchart TD
  Product[Что является продуктом] --> Exec{Исполняемый plugin}
  Product --> Artifact{Package or extension artifact}
  Product --> Config{Workspace config}
  Exec --> Runtime[codex-runtime or claude]
  Artifact --> Package[codex-package or gemini]
  Config --> Workspace[opencode or cursor]
`"
/>

## Codex Package

Используйте `codex-package`, когда конечным результатом должен быть package для Codex, а не репозиторий с исполняемым плагином.

Это полезно, когда:

- packaging и есть реальный контракт поставки
- вам нужно, чтобы исходное состояние проекта оставалось управляемым в одном месте
- не нужно притворяться, что у этого target тот же runtime contract, что и у `codex-runtime`

У Codex package есть и жёсткий контракт layout bundle:

- `.codex-plugin/` содержит только `plugin.json`
- optional `.app.json` и `.mcp.json` лежат в корне plugin, а не внутри `.codex-plugin/`
- эти sidecar-файлы существуют только тогда, когда `.codex-plugin/plugin.json` ссылается на `./.app.json` или `./.mcp.json`

## Gemini

Используйте `gemini`, когда цель — пакет расширения для Gemini CLI.

Этот target специально ориентирован на packaging.

Его правильно воспринимать так:

- это полноценный extension-packaging path через `render`, `import` и `validate`
- это не основной runtime-путь
- его выбирают, когда Gemini extension artifacts и есть конечный продукт

## OpenCode

Используйте `opencode`, когда репозиторий должен владеть конфигурацией OpenCode workspace и связанными project assets.

Этот target важен, когда:

- проекту нужен управляемый `opencode.json`
- репозиторий должен владеть workspace-level MCP и config shape
- нужен документированный путь авторинга конфигурации вместо ручной правки файлов

Но не путайте это с самым сильным runtime contract.

## Cursor

Используйте `cursor`, когда репозиторий должен управлять конфигурацией Cursor workspace.

Документированный subset включает:

- `.cursor/mcp.json`
- `.cursor/rules/**` в корне проекта
- optional shared root `AGENTS.md`

Это target для workspace-config, а не основной runtime-путь.

## Практическое правило выбора

Выбирайте эти targets, когда результатом проекта являются:

- package artifacts
- extension packaging
- workspace config

Не выбирайте их только потому, что название похоже на runtime-путь.

Если вам на самом деле нужно исполняемое поведение плагина, вернитесь к [Выбору runtime](/ru/concepts/choosing-runtime) и начинайте оттуда.

## Правило готовности

Для этих targets правило здорового репозитория остаётся тем же:

- исходное состояние проекта живёт в package-standard layout
- rendered files являются outputs
- `render --check` и `validate --strict` остаются главными проверками

## Что читать вместе с этим

Читайте эту страницу вместе с [Моделью target’ов](/ru/concepts/target-model), [Поддержкой target’ов](/ru/reference/target-support) и [Границей поддержки](/ru/reference/support-boundary).
