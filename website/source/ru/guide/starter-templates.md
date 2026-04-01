---
title: "Стартовые шаблоны"
description: "Официальные starter repositories для типовых входных путей в plugin-kit-ai, а не граница managed project model."
canonicalId: "page:guide:starter-templates"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Стартовые шаблоны

Если нужен проверенный старт вместо пустой директории, используйте официальные starter repositories.

## Выбор за 60 секунд

- Выбирайте **Go**, когда нужен самый сильный self-contained путь для продакшена.
- Выбирайте **Node/TypeScript**, когда нужен основной поддерживаемый путь без Go.
- Выбирайте **Python** только тогда, когда репозиторий осознанно Python-first и остаётся repo-local.
- Выбирайте **Codex** или **Claude** по первому реальному target, который нужно поддержать сейчас, а не по тому, что может понадобиться когда-нибудь потом.

## Важно: starter’ы — это точки входа

Названия starter’ов специально разделены по основному пути, например Codex или Claude.

Это **не** означает, что модель продукта навсегда запирается в одном agent family.

Starter помогает выбрать правильную первую форму для:

- основного runtime-требования
- языка команды
- первого поддерживаемого target’а

После этого сохраняйте repo в managed project model и расширяйте его по реальной необходимости.

Для более широкой картины прочитайте [Один проект, несколько target’ов](/ru/guide/one-project-multiple-targets).

## Лучшие варианты по умолчанию

- Самый сильный default для Codex: [plugin-kit-ai-starter-codex-go](https://github.com/777genius/plugin-kit-ai-starter-codex-go)
- Самый сильный default для Claude: [plugin-kit-ai-starter-claude-go](https://github.com/777genius/plugin-kit-ai-starter-claude-go)
- Основной supported non-Go путь для Codex: [plugin-kit-ai-starter-codex-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-codex-node-typescript)
- Основной supported non-Go путь для Claude: [plugin-kit-ai-starter-claude-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-claude-node-typescript)

## Codex Runtime

- [plugin-kit-ai-starter-codex-go](https://github.com/777genius/plugin-kit-ai-starter-codex-go)
- [plugin-kit-ai-starter-codex-python](https://github.com/777genius/plugin-kit-ai-starter-codex-python)
- [plugin-kit-ai-starter-codex-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-codex-node-typescript)

## Claude

- [plugin-kit-ai-starter-claude-go](https://github.com/777genius/plugin-kit-ai-starter-claude-go)
- [plugin-kit-ai-starter-claude-python](https://github.com/777genius/plugin-kit-ai-starter-claude-python)
- [plugin-kit-ai-starter-claude-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-claude-node-typescript)

## Shared-package reference starters

Они полезны, когда вы уже точно знаете, что хотите shared dependency `plugin-kit-ai-runtime`, а не vendored helper files.

- [codex-python-runtime-package-starter](https://github.com/777genius/plugin-kit-ai/tree/main/examples/starters/codex-python-runtime-package-starter)
- [claude-node-typescript-runtime-package-starter](https://github.com/777genius/plugin-kit-ai/tree/main/examples/starters/claude-node-typescript-runtime-package-starter)

Это reference starters в основном репозитории, а не отдельные GitHub template repos.

## Когда лучше брать starter

Используйте starter, когда:

- хотите сразу получить проверенный repo layout
- хотите сравнить свой проект с минимальным поддерживаемым примером
- хотите быстрее онбордить команду без догадок по scaffolding

Используйте `plugin-kit-ai init` напрямую, когда:

- нужен fresh repo from first principles
- нужно явно выбрать флаги и path
- вы встраиваете plugin-kit-ai в уже существующую структуру репозитория

## Практическое правило

- Берите **template repo**, когда нужен самый чистый публичный поток `Use this template`.
- Берите **starter из основного репозитория**, когда хотите изучить canonical source, сравнить layouts или начать с shared-package reference path.
- Берите **`plugin-kit-ai init`**, когда у вас уже есть репозиторий и нужно внедрить managed project model без копирования starter’а.

## Безопасная mental model

- выбирайте starter под **первый** правильный путь
- не считайте семейство starter’а окончательной границей repo
- считайте managed project model долгосрочным source of truth

Свяжите эту страницу с [Выбором стартового репозитория](/ru/guide/choose-a-starter), [Примерами и рецептами](/ru/guide/examples-and-recipes) и [Managed Project Model](/ru/concepts/managed-project-model).
