---
title: "Быстрый старт"
description: "Самый быстрый поддерживаемый путь к рабочему проекту на plugin-kit-ai."
canonicalId: "page:guide:quickstart"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Быстрый старт

Это самый короткий поддерживаемый путь, если вам нужен реальный plugin repo без ручного редактирования target-файлов.

Он специально показывает один рекомендуемый стартовый path, а не конечную границу продукта.

`plugin-kit-ai` задуман как managed project model, в которой один repo может вести несколько target’ов и output shapes из одного authored source of truth.

## Если читать только одно

Начинайте с Go по умолчанию, если вы уже заранее не знаете, что вам нужны Claude hooks, Node/TypeScript или Python.

Но не путайте стартовый path с permanent limit: выбрать первый target не значит навсегда запретить остальные.

## Рекомендуемый старт по умолчанию

Если у вас нет сильной причины выбрать другой путь, начинайте так:

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai version
plugin-kit-ai init my-plugin
cd my-plugin
plugin-kit-ai render .
plugin-kit-ai validate . --platform codex-runtime --strict
```

Это даёт самый сильный путь по умолчанию:

- platform: `codex-runtime`
- runtime: `go`
- contract: public-stable путь авторинга по умолчанию

## Как выбрать правильный путь

| Цель | Лучший стартовый путь |
| --- | --- |
| Самый сильный production path | `codex-runtime` с `--runtime go` |
| Плагин для Claude | `claude` |
| Локальный Python plugin | `codex-runtime --runtime python` |
| Локальный TypeScript plugin | `codex-runtime --runtime node --typescript` |
| Package output для Codex | `codex-package` |
| Packaging для Gemini extension | `gemini` |
| Workspace config для OpenCode | `opencode` |
| Workspace config для Cursor | `cursor` |

Если продукту нужны несколько target’ов, всё равно начинайте с главного требования сегодня, а затем расширяйте один и тот же managed repo.

## Типовые первые команды

```bash
plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript
plugin-kit-ai doctor ./my-plugin
plugin-kit-ai bootstrap ./my-plugin
plugin-kit-ai render ./my-plugin
plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
```

## Что важно знать перед выбором Python или Node

- Python и Node поддерживаются как полноценный путь для стабильного локального сценария.
- Но на машине, которая запускает плагин, всё равно должен быть установлен Python `3.10+` или Node.js `20+`.
- Go остаётся рекомендуемым путём по умолчанию, когда нужен самый чистый production и distribution story.

## Что читать дальше

- Переходите к [Первому плагину](/ru/guide/first-plugin), если хотите самый узкий рекомендуемый tutorial.
- Переходите к [Один проект, несколько target’ов](/ru/guide/one-project-multiple-targets), если multi-target path является для вас ключевой частью продукта.
- Переходите к [Что можно построить](/ru/guide/what-you-can-build), если всё ещё сравниваете формы продукта.
- Переходите к [Выбору target](/ru/guide/choose-a-target), если уже понимаете продукт, но ещё решаете между Codex, Claude, Gemini, Cursor и OpenCode.
- Переходите к [Выбору starter repo](/ru/guide/choose-a-starter), если хотите стартовать не с пустого repo, а с шаблона.

См. [Выбор runtime](/ru/concepts/choosing-runtime) для модели выбора и [Установку](/ru/guide/installation) для каналов установки CLI.
