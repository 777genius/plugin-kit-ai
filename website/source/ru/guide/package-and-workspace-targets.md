---
title: "Package и workspace targets"
description: "Как использовать package, extension и repo-managed integration lanes, не путая их с runtime-путями."
canonicalId: "page:guide:package-and-workspace-targets"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Package и workspace targets

Не каждый lane в `plugin-kit-ai` является исполняемым runtime-путём.

Читайте эту страницу перед выбором `codex-package`, `gemini`, `opencode` или `cursor`, потому что эти lanes решают другую delivery-задачу, чем `codex-runtime` и `claude`.

## Короткое правило

- выбирайте `codex-runtime` или `claude`, когда продуктом является исполняемое поведение плагина
- выбирайте `codex-package` или `gemini`, когда продуктом являются package или extension artifacts
- выбирайте `opencode` или `cursor`, когда продуктом является repo-managed integration config

## Рекомендуемые package и extension lanes

### Codex Package

Используйте `codex-package`, когда конечным результатом должен быть пакет Codex.

Это правильный lane, когда:

- packaging и есть реальный delivery contract
- repo должен оставаться единым
- продукт должен выпускать официальный package artifact для Codex

### Gemini

Используйте `gemini`, когда цель - пакет расширения Gemini CLI.

Воспринимайте его так:

- это рекомендуемый extension lane через `generate`, `import` и `validate`
- это правильный выбор, когда Gemini extension artifacts и есть конечный продукт
- это отдельный lane относительно стандартного Codex runtime старта

## Repo-managed integration lanes

### OpenCode

Используйте `opencode`, когда repo должен владеть OpenCode integration config и связанными project assets.

### Cursor

Используйте `cursor`, когда repo должен владеть Cursor integration config.

Эти lanes ценны тогда, когда output - это управляемая integration/config ownership, а не исполняемое поведение.

## Правило готовности

Для этих lanes правило здорового repo остаётся тем же:

- authored project живёт в package-standard layout
- generated files являются outputs
- `generate --check` и `validate --strict` остаются главными gates

Если вам на самом деле нужно исполняемое поведение, вернитесь к [Выбору runtime](/ru/concepts/choosing-runtime).
