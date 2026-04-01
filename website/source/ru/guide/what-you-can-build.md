---
title: "Что можно построить"
description: "Широкий публичный обзор реальных сценариев и форм продукта, которые поддерживает plugin-kit-ai."
canonicalId: "page:guide:what-you-can-build"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Что можно построить

Эта страница — широкая карта продукта. Читайте её, когда нужно понять, какие реальные результаты даёт `plugin-kit-ai`, ещё до выбора runtime, стартового репозитория или target.

<MermaidDiagram
  :chart="`
flowchart TD
  Product[plugin-kit-ai product shapes] --> Runtime[Runtime plugins]
  Product --> Multi[One managed repo with multiple outputs]
  Product --> Bundle[Portable bundle handoff]
  Product --> Shared[Shared runtime package]
  Product --> Package[Package and extension targets]
  Product --> Workspace[Workspace config targets]
`"
/>

## 1. Runtime-плагины для Codex

Это основной публичный путь по умолчанию.

Используйте его, когда нужен:

- самый сильный старт для продакшена
- управляемая модель проекта вместо ручного редактирования target-файлов
- ясный путь через `render` и `validate --strict`

Runtime-плагины для Codex можно делать на:

- Go для самого сильного стандартного продакшен-контракта
- Node/TypeScript для основного стабильного non-Go пути
- Python для команд, которые осознанно остаются на локальном Python runtime

## 2. Плагины для Claude Hooks

Используйте Claude-путь, когда Claude hooks действительно являются требованием продукта.

Это правильный выбор, если:

- вам нужны hooks именно Claude
- стабильного подмножества Claude достаточно для вашего плагина
- нужен более сильный и предсказуемый процесс авторинга, чем при ручной правке native files

## 3. Репозитории плагинов, готовые для команды

`plugin-kit-ai` — это не только scaffolding. Это ещё и путь к репозиторию, который другой коллега может понять, проверить и использовать без скрытых договорённостей.

Это означает, что система поддерживает:

- строгие проверки готовности
- понятные сценарии для CI
- явный выбор пути и target’а
- предсказуемая передача между авторами и downstream-пользователями

## 4. Один managed project, который может покрывать несколько выходов

Продукт шире, чем это кажется по названиям starter’ов.

Это не побочная возможность, а одна из центральных идей продукта.

Публичные starter family разделены по **первому** runtime или target path, но managed project model шире этого.

Это означает, что один проект может оставаться единым source of truth и при этом управлять:

- основным runtime path
- дополнительными package или workspace-config target’ами
- а когда продукту это действительно нужно, и несколькими agent-facing output family

Практическая mental model описана в [Один проект, несколько target’ов](/ru/guide/one-project-multiple-targets).

## 5. Portable bundle handoff для Python и Node

Для поддерживаемых Python и Node путей можно выйти за пределы локального authoring и собирать portable bundle artifacts для handoff.

Это важно, когда:

- модель поставки требует скачиваемые артефакты вместо live repo
- нужен более чистый сценарий установки для downstream-пользователей Python и Node путей
- вы используете bundle publish/fetch flow как часть release handoff

Подробный public flow описан в [Bundle handoff](/ru/guide/bundle-handoff).

## 6. Shared runtime package

Python и Node helper-логика может жить либо:

- в vendored helper files внутри repo
- в общем `plugin-kit-ai-runtime` package

Это даёт поддерживаемый путь для:

- reusable runtime helpers на несколько repo
- более чистые обновления зависимостей
- стандартизированного helper API без ручного копирования scaffolded files

## 7. Targets для package, extension и workspace-config

Не каждая публичная форма — это repo-local runtime plugin.

`plugin-kit-ai` также покрывает:

- packaging-oriented lanes
- extension-style targets
- workspace-config integration targets

Эти target’ы важны, когда конечный продукт — это packaging или configuration, а не исполняемый плагин.

Перед выбором этих путей прочитайте [Package и workspace targets](/ru/guide/package-and-workspace-targets).

## 8. Generated public reference

Этот docs site также даёт generated reference для:

- реального дерева CLI-команд
- Go SDK
- Node и Python runtime helpers
- platform events
- capability-level cross-platform views

Именно так публичная документация остаётся привязанной к реальным исходным данным, а не превращается в устаревший текстовый слой.

## Безопасный порядок чтения

Если вы ещё решаете, что именно делать:

1. прочитайте эту страницу
2. прочитайте [Выбор runtime](/ru/concepts/choosing-runtime)
3. прочитайте [Модель target’ов](/ru/concepts/target-model)
4. выберите starter repo или default `init` path

Свяжите эту страницу с [Примерами и рецептами](/ru/guide/examples-and-recipes), [Выбором starter repo](/ru/guide/choose-a-starter), [Выбором delivery model](/ru/guide/choose-delivery-model), [Bundle handoff](/ru/guide/bundle-handoff), [Package и workspace targets](/ru/guide/package-and-workspace-targets) и [API поверхностями](/ru/api/).
