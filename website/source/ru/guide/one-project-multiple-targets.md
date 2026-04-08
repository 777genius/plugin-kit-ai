---
title: "Один проект, несколько target’ов"
description: "Как один repo в plugin-kit-ai может поддерживать несколько агентов или выходных target’ов."
canonicalId: "page:guide:one-project-multiple-targets"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Один проект, несколько target’ов

Это одна из самых важных идей в `plugin-kit-ai`:

- **starter repo** даёт хорошую первую точку входа
- **один repo** может вырасти дальше этой первой точки входа

Не путайте семейство starter’а с долгосрочной границей проекта.

## Короткое правило

Начинайте с runtime или target’а, который является **главным требованием сегодня**.

После этого продолжайте считать репозиторий одним source of truth и рендерите только те outputs, которые реально нужны.

<MermaidDiagram
  :chart="`
flowchart LR
  Repo[One authored repo] --> Generate[generate]
  Generate --> CodexRuntime[codex-runtime]
  Generate --> Claude[claude]
  Generate --> CodexPackage[codex-package]
  Generate --> Gemini[gemini]
  Generate --> OpenCode[opencode]
  Generate --> Cursor[cursor]
`"
/>

То есть проект может стартовать как:

- Codex-first plugin repo
- Claude-first plugin repo
- package/config-first repo

и со временем всё равно стать более широким проектом.

## Почему starter’ы выглядят agent-specific

Официальные starter’ы специально разделены по основному пути:

- Codex starter’ы оптимизируют путь Codex runtime по умолчанию
- Claude starter’ы оптимизируют стабильный путь Claude hooks
- языковые варианты оптимизируют первый выбор runtime для команды

Это делает первый запуск предсказуемым.

Чего это **не** означает:

- что `plugin-kit-ai` навсегда поддерживает только одного агента
- что для каждого агента обязательно нужен отдельный repo
- что имя starter’а определяет окончательную границу продукта

## Что на самом деле остаётся единым

Объединяющая часть - это сам repo и общий workflow.

Это означает, что команда ведёт один authored project, а затем использует `generate`, `validate`, import/normalize flow и target directories для управления нужными выходами.

На практике единым остаётся:

- один layout репозитория
- один способ собирать и поддерживать проект
- одна история валидации
- одна история для CI
- одно место, где ревьюятся generated outputs

## Что значит “несколько target’ов” на практике

Обычно это выглядит как один из двух сценариев.

### 1. Один основной runtime и несколько дополнительных outputs

Пример:

- основное поведение плагина живёт в Codex runtime
- но тот же repo также управляет package/config target’ами вроде Gemini, OpenCode или Cursor

Это самый частый вариант широкого проекта.

### 2. Один repo, который покрывает больше одного agent family

Пример:

- команда начинает с Codex как основного runtime path
- позже тому же repo нужны Claude-specific outputs или поддержка Claude

Здесь важно не врать в документации:

- это **не** обещание искусственной parity между всеми агентами
- это **да** обещание, что `plugin-kit-ai` даёт один repo и один workflow вместо россыпи вручную поддерживаемых target files

## Безопасная mental model

Думайте так:

1. выберите лучший starter под **первое** реальное требование
2. относитесь к starter’у как к входу, а не как к клетке
3. сохраняйте repo единым
4. добавляйте target’ы и outputs по мере реальной необходимости

## Когда всё-таки лучше разделять repo

Отдельные repo всё ещё имеют смысл, когда:

- у команд явно разные release cadence
- runtime-логика продуктов между собой почти не связана
- границы владения важнее общего способа вести проект

Не делите repo только потому, что названия starter’ов выглядят agent-specific.

## Что читать дальше

- [Стартовые шаблоны](/ru/guide/starter-templates)
- [Выбор starter repo](/ru/guide/choose-a-starter)
- [Что можно построить](/ru/guide/what-you-can-build)
- [Модель target’ов](/ru/concepts/target-model)
