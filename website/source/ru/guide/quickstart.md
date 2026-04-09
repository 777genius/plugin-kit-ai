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

Это самый короткий рекомендуемый путь, если вам нужен один plugin repo, который потом можно расширять новыми способами поставки.

Сначала выберите один сильный стартовый путь. Package, extension и настройку интеграций в самом repo можно добавить позже, когда они действительно понадобятся продукту.

## Начните с задачи

Выберите форму repo по тому, что именно вы собираете:

- online service: `plugin-kit-ai init my-plugin --template online-service`
- local tool: `plugin-kit-ai init my-plugin --template local-tool`
- custom logic: `plugin-kit-ai init my-plugin --template custom-logic`

Если хотите сначала короткое объяснение, откройте [Что именно вы собираете](/ru/guide/choose-what-you-are-building).

## Если читать только одно

Начинайте с job-first пути выше, если вы заранее не знаете, что вам нужен backward-compatible путь по умолчанию на Go или какой-то конкретный advanced target.

Первый выбор - это стартовая точка, а не вечная граница репозитория.

## Backward-compatible путь по умолчанию

Если у вас нет сильной причины выбрать другой путь, начинайте так:

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai version
plugin-kit-ai init my-plugin
cd my-plugin
plugin-kit-ai generate .
plugin-kit-ai validate . --platform codex-runtime --strict
```

Это сохраняет самый сильный legacy-путь по умолчанию: Go-репозиторий для Codex runtime, который проще всего проверять, передавать другим и потом расширять.

## Почему это путь по умолчанию

- один репозиторий с первого дня
- самая чистая runtime и release story сегодня
- самая простая база для package, extension и integration lanes позже

## Что вы получите

- один plugin repo с первого дня
- authored files под `src/`
- generated output для Codex runtime из того же repo
- понятную проверку готовности через `validate --strict`

## Поддерживаемые пути для Node и Python

Если команда уже живёт в Node/TypeScript или Python, эти пути поддерживаются и видны с самого начала:

- `codex-runtime --runtime node --typescript`
- `codex-runtime --runtime python`
- оба варианта являются локальными interpreted runtime paths, поэтому на машине исполнения всё равно нужен Node.js `20+` или Python `3.10+`
- Go всё равно остаётся путём по умолчанию, когда нужен самый сильный общий сценарий для продакшна

## Если вы осознанно начинаете с Node или Python

Используйте этот альтернативный flow только тогда, когда выбор языка уже является частью продуктового требования:

```bash
plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript
plugin-kit-ai doctor ./my-plugin
plugin-kit-ai bootstrap ./my-plugin
plugin-kit-ai generate ./my-plugin
plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
```

Или стартуйте с Python:

```bash
plugin-kit-ai init my-plugin --platform codex-runtime --runtime python
plugin-kit-ai doctor ./my-plugin
plugin-kit-ai bootstrap ./my-plugin
plugin-kit-ai generate ./my-plugin
plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
```

## Что делать дальше

- правьте плагин под `src/`
- после изменений снова запускайте `plugin-kit-ai generate ./my-plugin`
- потом снова запускайте `plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict`
- и только после этого добавляйте другие способы поставки, если продукту это действительно нужно

## Что добавлять потом

| Цель | Что добавлять позже |
| --- | --- |
| Claude hooks как реальный продукт | `claude` |
| Официальный пакет Codex | `codex-package` |
| Пакет расширения Gemini | `gemini` |
| Настройка интеграции в самом repo | `opencode` или `cursor` |

`claude` выбирайте первым только тогда, когда hooks Claude уже являются реальным требованием продукта.

## Что расширяется потом

- repo остаётся единым, когда вы добавляете новые lanes
- package и extension lanes идут из того же authored source
- OpenCode и Cursor нужны тогда, когда repo должен хранить и вести настройку интеграции
- точная support boundary живёт в reference docs, а не в вашем первом стартовом flow

## Что читать дальше

- [Что именно вы собираете](/ru/guide/choose-what-you-are-building)
- [Создайте первый plugin](/ru/guide/first-plugin)
- [Что можно собрать](/ru/guide/what-you-can-build)
- [Выбор target](/ru/guide/choose-a-target)
