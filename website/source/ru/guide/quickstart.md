---
title: "Быстрый старт"
description: "Самый быстрый рекомендуемый путь к рабочему проекту на plugin-kit-ai."
canonicalId: "page:guide:quickstart"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Быстрый старт

Это самый короткий рекомендуемый путь, если вам нужен один plugin repo, который потом можно расширять на новые delivery lanes.

Сначала выберите один сильный lane. Package, extension и repo-managed integration outputs можно добавить позже, когда они действительно понадобятся продукту.

## Если читать только одно

Начинайте с Go по умолчанию, если вы заранее не знаете, что продукт определяют Claude hooks, Node/TypeScript или Python.

Первый lane - это стартовая точка, а не вечная граница репозитория.

## Рекомендуемый старт по умолчанию

Если у вас нет сильной причины выбрать другой путь, начинайте так:

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai version
plugin-kit-ai init my-plugin
cd my-plugin
plugin-kit-ai generate .
plugin-kit-ai validate . --platform codex-runtime --strict
```

Это даёт самый сильный путь по умолчанию: Go-репозиторий для Codex runtime, который проще всего проверять, передавать другим и потом расширять.

## Почему это путь по умолчанию

- один репозиторий с первого дня
- самая чистая runtime и release story сегодня
- самая простая база для package, extension и integration lanes позже

## Как выбрать первый lane

| Цель | Рекомендуемый первый lane |
| --- | --- |
| Самый сильный runtime lane | `codex-runtime` с `--runtime go` |
| Официальный пакет Codex | `codex-package` |
| Пакет расширения Gemini | `gemini` |
| Локальный TypeScript runtime | `codex-runtime --runtime node --typescript` |
| Локальный Python runtime | `codex-runtime --runtime python` |

`claude` выбирайте первым только тогда, когда hooks Claude уже являются реальным требованием продукта.

## Типовые первые команды

```bash
plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript
plugin-kit-ai doctor ./my-plugin
plugin-kit-ai bootstrap ./my-plugin
plugin-kit-ai generate ./my-plugin
plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
```

## Что важно знать перед выбором Python или Node

- Python и Node - рекомендуемые локальные runtime lanes для команд, которые уже живут в этих стеках.
- Но на машине исполнения всё равно должен быть установлен Python `3.10+` или Node.js `20+`.
- Go остаётся рекомендуемым путём по умолчанию, когда нужен самый сильный runtime и distribution story.

## Что расширяется потом

- repo остаётся единым, когда вы добавляете новые lanes
- package и extension lanes идут из того же authored source
- OpenCode и Cursor нужны тогда, когда repo должен владеть integration config
- точная support boundary живёт в reference docs, а не в вашем первом стартовом flow
