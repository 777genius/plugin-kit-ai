---
title: "Что именно вы собираете"
description: "Выберите правильный стартовый путь в plugin-kit-ai до того, как уйдёте в target taxonomy."
canonicalId: "page:guide:choose-what-you-are-building"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Что именно вы собираете

Начинайте с задачи. Вам не нужно понимать `codex-package`, `runtime lanes` или `local MCP over stdio` до того, как вы создадите repo.

## Подключить онлайн-сервис

Используйте это, когда плагин должен подключаться к hosted-сервису вроде Notion, Stripe, Cloudflare или Vercel.

```bash
plugin-kit-ai init my-plugin --template online-service
```

Это создаёт:

- один authored repo под `src/`
- общий portable MCP source
- generated outputs для поддерживаемых package и workspace targets
- без launcher-кода по умолчанию

## Подключить локальный инструмент

Используйте это, когда плагин должен обращаться к repo-owned CLI, container или локальному executable tool вроде Docker Hub, Chrome DevTools или HubSpot Developer.

```bash
plugin-kit-ai init my-plugin --template local-tool
```

Это создаёт:

- один authored repo под `src/`
- общий local tool MCP wiring
- generated outputs для поддерживаемых package и workspace targets
- без launcher-кода по умолчанию

## Сделать свой plugin с логикой

Используйте это, когда продукт определяется hooks, runtime behavior или custom code.

```bash
plugin-kit-ai init my-plugin --template custom-logic
```

Это сохраняет самый сильный backward-compatible runtime-first путь и использует текущую launcher-backed модель authoring.

## Что делать дальше

После любого из этих стартов:

```bash
cd my-plugin
plugin-kit-ai inspect . --authoring
plugin-kit-ai generate .
plugin-kit-ai generate --check .
```

Потом валидируйте тот supported output, который реально собираетесь поставлять первым.

## Когда открывать advanced pages

- Открывайте [Быстрый старт](/ru/guide/quickstart), когда нужен самый короткий first-run flow.
- Открывайте [Выбор target](/ru/guide/choose-a-target), когда нужны конкретные решения по способу поставки.
- Открывайте [Что можно собрать](/ru/guide/what-you-can-build), когда нужна полная product map.
