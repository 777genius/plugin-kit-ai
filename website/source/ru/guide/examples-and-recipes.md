---
title: "Примеры и рецепты"
description: "Путеводитель по публичным example repos, starter repos, локальным runtime references и skill examples в plugin-kit-ai."
canonicalId: "page:guide:examples-and-recipes"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Примеры и рецепты

Используйте эту страницу, когда хотите увидеть, как `plugin-kit-ai` выглядит в реальных репозиториях, а не только в абстрактных объяснениях.

## 1. Production plugin examples

Это самые наглядные примеры законченных публичных форм:

- `codex-basic-prod`: production repo для Codex runtime
- `claude-basic-prod`: production repo для Claude
- `codex-package-prod`: target для Codex package
- `gemini-extension-package`: packaging target для Gemini extension
- `cursor-basic`: workspace-config target для Cursor
- `opencode-basic`: workspace-config target для OpenCode

Читайте их, когда нужен:

- конкретный layout репозитория
- реальный пример rendered outputs
- честный публичный пример того, как выглядит “здоровый” проект

Важно: эти examples показывают отдельные публичные формы продукта, а не требуют делить реальную систему на отдельный repo под каждый target.

## 2. Starter repos

Используйте starter repos, когда хотите начинать не с пустой директории, а с known-good baseline.

Они лучше всего подходят для:

- первого старта
- онбординга команды
- выбора между Go, Python, Node, Claude и Codex

Но не путайте каталог starter repos с архитектурным ограничением продукта: один managed repo всё равно может позже вести несколько target’ов.

Если вы ещё выбираете стартовую точку, свяжите это с [Выбором стартового репозитория](/ru/guide/choose-a-starter).

## 3. Local runtime references

Каталог `examples/local` показывает локальные Python и Node runtime references.

Он полезен, когда:

- нужно глубже понять interpreted runtime story
- вы хотите сравнить JavaScript, TypeScript и Python local-runtime setups
- нужен конкретный reference сверх starter repos

## 4. Skill examples

Каталог `examples/skills` показывает примеры skills и вспомогательных интеграций.

Это не главный entrypoint для большинства авторов плагинов, но он полезен, когда:

- вы хотите встроить docs, review или formatting helpers в более широкий workflow
- нужно понять, как соседние skills могут жить рядом с plugin repos

## Что читать по цели

- Нужен самый сильный runtime example: начните с production example для Codex или Claude, потом прочитайте [Плагин для команды](/ru/guide/team-ready-plugin).
- Нужны packaging или workspace-config examples: начните с примеров для Codex package, Gemini, Cursor или OpenCode, потом прочитайте [Package и workspace targets](/ru/guide/package-and-workspace-targets).
- Нужна чистая стартовая точка, а не finished example: идите в [Стартовые шаблоны](/ru/guide/starter-templates).
- Сначала нужно выбрать сам target: прочитайте [Выбор target](/ru/guide/choose-a-target).
- Сначала нужен общий обзор продукта: прочитайте [Что можно построить](/ru/guide/what-you-can-build).

## Главное правило

Examples должны прояснять публичный контракт, а не заменять его.

Используйте example repos, чтобы увидеть форму, layout и healthy outputs. Используйте остальную документацию, чтобы понять, что стабильно, что опционально и что проект действительно обещает.

Если вы хотите понять, как эти формы могут жить в одном и том же repo, читайте [Один проект, несколько target’ов](/ru/guide/one-project-multiple-targets).
