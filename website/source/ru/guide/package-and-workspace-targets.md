---
title: "Package и workspace targets"
description: "Как использовать package, extension и настройку интеграций в самом repo, не путая их с исполняемыми runtime-путями."
canonicalId: "page:guide:package-and-workspace-targets"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Package и workspace targets

Не каждый путь в `plugin-kit-ai` является исполняемым runtime-путём.

Читайте эту страницу перед выбором `codex-package`, `gemini`, `opencode` или `cursor`, потому что эти target'ы решают другую задачу, чем `codex-runtime` и `claude`.

## Короткое правило

- выбирайте `codex-runtime` или `claude`, когда продуктом является исполняемое поведение плагина
- выбирайте `codex-package` или `gemini`, когда продуктом являются package или extension artifacts
- выбирайте `opencode` или `cursor`, когда продуктом является настройка интеграции в самом repo

## Рекомендуемые package и extension lanes

### Codex Package

Используйте `codex-package`, когда конечным результатом должен быть пакет Codex.

Это правильный путь, когда:

- packaging и есть реальный delivery contract
- repo должен оставаться единым
- продукт должен выпускать официальный package artifact для Codex

### Gemini

Используйте `gemini`, когда цель - пакет расширения Gemini CLI.

Воспринимайте его так:

- это рекомендуемый extension path через `generate`, `import` и `validate`
- это правильный выбор, когда Gemini extension artifacts и есть конечный продукт
- это отдельный путь относительно стандартного Codex runtime старта

## Настройка интеграций в самом repo

### OpenCode

Используйте `opencode`, когда repo должен хранить OpenCode integration setup и связанные project assets.

### Cursor

Используйте `cursor`, когда repo должен хранить Cursor integration setup.

Эти пути ценны тогда, когда output - это настройка интеграции в repo, а не исполняемое поведение.

## Правило готовности

Для этих путей правило здорового repo остаётся тем же:

- authored project живёт в package-standard layout
- generated files являются outputs
- `generate --check` и `validate --strict` остаются главными gates

Если вам на самом деле нужно исполняемое поведение, вернитесь к [Выбору runtime](/ru/concepts/choosing-runtime).
