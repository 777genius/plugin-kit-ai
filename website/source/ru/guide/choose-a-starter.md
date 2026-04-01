---
title: "Выбор стартового репозитория"
description: "Практическая матрица для выбора правильного official starter по target, runtime и модели поставки."
canonicalId: "page:guide:choose-a-starter"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Выбор стартового репозитория

Используйте эту страницу, когда нужен самый быстрый путь к проверенному репозиторию и вы не хотите угадывать правильный starter только по имени шаблона.

Перед выбором держите в голове одно важное правило:

- starter показывает, **как начать**
- но не определяет окончательную границу проекта

Если эта разница пока неочевидна, сначала прочитайте [Один проект, несколько target’ов](/ru/guide/one-project-multiple-targets).

## Выбор за 60 секунд

- выбирайте Go, когда нужен самый сильный путь для продакшена
- выбирайте Node/TypeScript, когда нужен основной поддерживаемый путь без Go
- выбирайте Python, когда репозиторий осознанно Python-first и остаётся локальным для репозитория
- выбирайте Claude starters только тогда, когда Claude hooks — это реальное product requirement

## Лучшие варианты по умолчанию

- Лучший общий default для Codex: [plugin-kit-ai-starter-codex-go](https://github.com/777genius/plugin-kit-ai-starter-codex-go)
- Лучший общий default для Claude: [plugin-kit-ai-starter-claude-go](https://github.com/777genius/plugin-kit-ai-starter-claude-go)
- Лучший non-Go default для Codex: [plugin-kit-ai-starter-codex-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-codex-node-typescript)
- Лучший non-Go default для Claude: [plugin-kit-ai-starter-claude-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-claude-node-typescript)

## Быстрое дерево решений

1. Нужен самый self-contained delivery path? Берите Go.
2. Нужен основной поддерживаемый путь без Go? Берите Node/TypeScript.
3. Нужен осознанный Python-first repo-local path? Берите Python.
4. Нужен Claude hook coverage как первый реальный target? Берите Claude. Иначе начинайте с Codex.

## Матрица starter’ов

| Цель | Лучшее семейство starter’ов | Почему |
| --- | --- | --- |
| Самый сильный Codex production path | [plugin-kit-ai-starter-codex-go](https://github.com/777genius/plugin-kit-ai-starter-codex-go) | Go-first production path с самой чистой историей передачи другим людям |
| Repo-local Codex plugin на Python | [plugin-kit-ai-starter-codex-python](https://github.com/777genius/plugin-kit-ai-starter-codex-python) | Stable Python subset с проверенным layout репозитория |
| Repo-local Codex plugin на Node/TS | [plugin-kit-ai-starter-codex-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-codex-node-typescript) | Основной поддерживаемый путь без Go |
| Самый сильный Claude production path | [plugin-kit-ai-starter-claude-go](https://github.com/777genius/plugin-kit-ai-starter-claude-go) | Stable Claude subset плюс самый чистый путь для продакшена |
| Repo-local Claude plugin на Python | [plugin-kit-ai-starter-claude-python](https://github.com/777genius/plugin-kit-ai-starter-claude-python) | Stable Claude hook subset с Python helpers |
| Repo-local Claude plugin на Node/TS | [plugin-kit-ai-starter-claude-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-claude-node-typescript) | Stable Claude hook subset для TypeScript-first команд |

## Когда использовать shared-package reference starters

Используйте shared-package reference starters, когда вы уже точно знаете, что команда хочет `plugin-kit-ai-runtime` как reusable dependency вместо vendored helper files.

Этот путь лучше, когда:

- нужна общая dependency across multiple plugin repos
- команда готова явно pin'ить и обновлять runtime package
- вы не хотите копировать helper files в каждый repo

Reference starters:

- [codex-python-runtime-package-starter](https://github.com/777genius/plugin-kit-ai/tree/main/examples/starters/codex-python-runtime-package-starter)
- [claude-node-typescript-runtime-package-starter](https://github.com/777genius/plugin-kit-ai/tree/main/examples/starters/claude-node-typescript-runtime-package-starter)

## Когда не нужно переоптимизировать выбор

Не тратьте слишком много времени на поиск идеального starter.

Если не уверены:

1. начинайте с Go starter ради самого сильного варианта по умолчанию
2. начинайте с Node/TypeScript starter ради основного поддерживаемого пути без Go
3. переходите к Python или shared-package variant только тогда, когда командный компромисс уже реален

## Хорошая командная политика

Выбор starter’а на уровне команды должен быть достаточно стабильным, чтобы:

- все узнавали layout репозитория
- CI использовал один и тот же readiness flow
- handoff не зависел от объяснений maintainer’а

Свяжите эту страницу со [Стартовыми шаблонами](/ru/guide/starter-templates), [Выбором модели поставки](/ru/guide/choose-delivery-model), [Bundle handoff](/ru/guide/bundle-handoff) и [Стандартом репозитория](/ru/reference/repository-standard).
