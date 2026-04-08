---
title: "Модель target'ов"
description: "Как plugin-kit-ai делит runtime, package, extension и repo-managed integration lanes."
canonicalId: "page:concepts:target-model"
section: "concepts"
locale: "ru"
generated: false
translationRequired: true
---

# Модель target'ов

`plugin-kit-ai` поддерживает несколько типов target'ов, потому что продуктам нужны разные delivery models.

## Короткое правило

- выбирайте runtime lane, когда нужен исполняемый plugin behavior
- выбирайте package или extension lane, когда output - это устанавливаемый артефакт
- выбирайте repo-managed integration lane, когда repo должен владеть configuration и integration shape

<MermaidDiagram
  :chart="`
flowchart TD
  Goal[Что вы поставляете] --> Runtime{Исполняемое поведение}
  Goal --> Package{Устанавливаемый артефакт}
  Goal --> Integration{Repo managed integration}
  Runtime --> CodexRuntime[codex-runtime]
  Runtime --> Claude[claude]
  Package --> CodexPackage[codex-package]
  Package --> Gemini[gemini]
  Integration --> OpenCode[opencode]
  Integration --> Cursor[cursor]
`"
/>

## Runtime lanes

Используйте их, когда проект сам владеет исполняемым поведением плагина.

Примеры:

- `codex-runtime`
- `claude`

## Package и extension lanes

Используйте их, когда продукт - это артефакт для установки, публикации или поставки.

Примеры:

- `codex-package`
- `gemini`

## Repo-managed integration lanes

Используйте их, когда repo должен владеть integration config и workspace behavior.

Примеры:

- `opencode`
- `cursor`

## Важное различие

Один проект не обязан навсегда означать только один target.

Ключевая граница здесь такая:

- один managed authored project
- явный выбор primary lane
- честные ожидания по поддержке для каждого generated output
