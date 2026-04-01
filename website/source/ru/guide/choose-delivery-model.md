---
title: "Выбор модели поставки"
description: "Как выбрать между локальными helper-файлами и shared runtime package для Python и Node плагинов."
canonicalId: "page:guide:choose-delivery-model"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Выбор модели поставки

У Python и Node плагинов есть два поддерживаемых способа доставки helper-логики. Ни один из них не является legacy. Они решают разные практические задачи.

<MermaidDiagram
  :chart="`
flowchart TD
  Start[Python or Node plugin] --> Shared{Нужна одна reusable dependency across repos}
  Shared -->|Да| Package[shared runtime package]
  Shared -->|Нет| Smooth{Нужен самый гладкий self contained start}
  Smooth -->|Да| Vendored[vendored helper]
  Smooth -->|Нет| Package
`"
/>

## Два режима

- `vendored helper`: scaffold записывает helper-файлы прямо в репозиторий
- `shared runtime package`: `--runtime-package` подключает `plugin-kit-ai-runtime` как dependency вместо записи helper в `src/`

## Когда выбирать vendored helper

- нужен самый гладкий первый старт
- вы хотите, чтобы репозиторий оставался самодостаточным
- хотите видеть helper implementation прямо в репозитории
- команда ещё не стандартизировалась на одной версии helper-пакета в PyPI или npm

Это путь по умолчанию, потому что он проще всего для первого старта на Python и Node.

## Когда выбирать shared runtime package

- нужна одна reusable helper dependency на несколько plugin repos
- удобнее обновлять helper behavior через обычные package version bumps
- команда готова pin'ить версии в `requirements.txt` или `package.json`
- вы уже знаете, что репозиторий должен идти по shared dependency path с первого дня

## Что при этом не меняется

- Go всё ещё остаётся рекомендуемым путём по умолчанию, когда нужен самый сильный путь для продакшена
- Python всё ещё требует Python `3.10+` на машине исполнения
- Node всё ещё требует Node.js `20+` на машине исполнения
- `validate --strict` остаётся главной проверкой готовности
- CLI install packages не превращаются в runtime API

## Рекомендуемая политика для команды

- выбирайте Go, когда нужен самый сильный долгосрочно поддерживаемый путь
- выбирайте vendored helpers, когда нужен самый гладкий Python или Node старт
- выбирайте shared runtime package, когда вы уже знаете, что нужна reusable dependency strategy across repos

## Правило миграции

Переход с vendored helpers на `plugin-kit-ai-runtime` — это поддерживаемая миграция. Это не запасной вариант и не путь устаревания.

Свяжите эту страницу с [Выбором starter repo](/ru/guide/choose-a-starter), [Bundle handoff](/ru/guide/bundle-handoff), [Стартовыми шаблонами](/ru/guide/starter-templates) и [Готовностью к продакшену](/ru/guide/production-readiness).
