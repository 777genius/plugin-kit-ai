---
title: "Managed Project Model"
description: "Главная идея plugin-kit-ai: один authored project, rendered outputs, строгая проверка и осознанный рост по target’ам."
canonicalId: "page:concepts:managed-project-model"
section: "concepts"
locale: "ru"
generated: false
translationRequired: true
---

# Managed Project Model

Если нужна одна страница, которая объясняет, чем на самом деле является `plugin-kit-ai`, начните с неё.

## В одном предложении

`plugin-kit-ai` — это управляемая система для plugin-проектов: вы держите один authored repo, рендерите нужные выходы под target’ы, проверяете результат и развиваете проект осознанно, не превращая его в набор ad-hoc glue.

## Быстрая перенастройка мышления

| Что бросается в глаза первым | Неверный вывод | Правильное чтение |
| --- | --- | --- |
| Названия starter’ов вроде Codex или Claude | Repo навсегда привязан к одной agent family | Название starter’а лишь оптимизирует первый правильный путь |
| Видимый CLI workflow | Продукт в основном сводится к generator tool | CLI — это воспроизводимая workflow-поверхность для managed project |
| Runtime, package и workspace target’ы | Всё имеет один и тот же operational contract | Разные outputs задуманы специально и имеют явные границы поддержки |

## Модель в четырёх частях

1. **Один authored project**
   Package-standard project остаётся долгосрочным source of truth.
2. **Rendered target outputs**
   Артефакты для runtime, package, extension и workspace-config target’ов производятся из этого source of truth.
3. **Строгие проверки готовности**
   `render`, `validate --strict` и соседние проверки доказывают, что намерение и output всё ещё согласованы.
4. **Осознанное расширение**
   Repo может расти на новые outputs и target’ы без притворства, что каждая поверхность одинаково зрелая.

## С чем проект чаще всего путают

`plugin-kit-ai` часто принимают за что-то одно из этого:

- набор starter’ов
- CLI, который один раз пишет файлы
- runtime helper package
- матрицу target’ов без объединяющей модели

Все эти части реально существуют, но ни одна из них сама по себе не определяет продукт.

## Что на самом деле остаётся единым

Единой остаётся именно managed project model:

- один layout репозитория
- один authored source of truth
- один воспроизводимый workflow
- одна история проверки готовности
- одна история handoff для команды и CI

## Что может меняться

Эти части могут меняться, не ломая саму модель:

- какой starter вы берёте первым
- какой runtime выбираете сначала
- какие target’ы рендерите
- используют ли Python или Node локальные helper-файлы или `plugin-kit-ai-runtime`
- какие stable и beta surfaces команда готова брать в работу

## Какое обещание даёт продукт

Обещание продукта — не “все target’ы ведут себя одинаково”.

Обещание продукта такое:

- один managed project вместо вручную поддерживаемых target-файлов
- одна система, которая делает rendered outputs воспроизводимыми
- одна публичная support boundary, которая честно говорит, что stable, а что нет

## Что это значит на практике

- Начинайте с самого узкого реального требования.
- Держите repo внутри managed project model.
- Добавляйте новые outputs только когда они реально нужны продукту.
- Делите repo по ownership или release cadence, а не потому что названия starter’ов выглядят слишком конкретно.

## Что читать дальше

- [Зачем plugin-kit-ai](/ru/concepts/why-plugin-kit-ai)
- [Один проект, несколько target’ов](/ru/guide/one-project-multiple-targets)
- [Модель target’ов](/ru/concepts/target-model)
- [Процесс авторинга](/ru/reference/authoring-workflow)
