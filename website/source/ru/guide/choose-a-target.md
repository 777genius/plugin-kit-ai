---
title: "Выбор target"
description: "Практический гид по выбору lane под вашу delivery model."
canonicalId: "page:guide:choose-a-target"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Выбор target

Используйте эту страницу, когда вы уже понимаете, что хотите работать с `plugin-kit-ai`, но ещё сопоставляете repo с тем, как именно будет доставляться продукт.

Выбор target означает выбор главного lane на сегодня, а не вечный lock-in на один output.

<MermaidDiagram
  :chart="`
flowchart TD
  Need[Что нужно продукту сейчас] --> Exec{Исполняемое поведение}
  Need --> Artifact{Package или extension}
  Need --> Config{Repo managed integration}
  Exec --> Codex[codex-runtime]
  Exec --> Claude[claude]
  Artifact --> CodexPackage[codex-package]
  Artifact --> Gemini[gemini]
  Config --> OpenCode[opencode]
  Config --> Cursor[cursor]
`"
/>

## Короткое правило

- выбирайте `codex-runtime`, когда нужен самый сильный runtime lane по умолчанию
- выбирайте `claude`, когда Claude hooks и есть реальное требование продукта
- выбирайте `codex-package`, когда продуктом является официальный пакет Codex
- выбирайте `gemini`, когда продуктом является пакет расширения Gemini
- выбирайте `opencode` или `cursor`, когда repo должен владеть integration/config outputs

## Краткий справочник по target'ам

| Target | Когда выбирать | Lane |
| --- | --- | --- |
| `codex-runtime` | Нужен основной путь для исполняемого плагина | Рекомендуемый runtime lane |
| `claude` | Нужны именно Claude hooks | Рекомендуемый Claude lane |
| `codex-package` | Нужен package output для Codex | Рекомендуемый package lane |
| `gemini` | Вы выпускаете пакет расширения Gemini | Рекомендуемый extension lane |
| `opencode` | Нужна repo-owned OpenCode integration config | Repo-managed integration lane |
| `cursor` | Нужна repo-owned Cursor integration config | Repo-managed integration lane |

## Безопасный выбор по умолчанию

Если вы не уверены, начинайте с `codex-runtime` и стандартного Go lane.

Это даёт самую чистую production starting point перед тем, как идти в более узкий или специализированный lane.

## Что делать, если target'ов нужно несколько

- сначала выберите основной lane, который определяет продукт сегодня
- держите repo единым
- добавляйте новые target'ы только тогда, когда появляется реальная delivery или integration задача
