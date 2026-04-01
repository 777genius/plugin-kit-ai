---
title: "Архитектура авторинга"
description: "Как исходное состояние проекта, render, validation, target’ы и handoff складываются в plugin-kit-ai."
canonicalId: "page:concepts:authoring-architecture"
section: "concepts"
locale: "ru"
generated: false
translationRequired: true
---

# Архитектура авторинга

`plugin-kit-ai` проще понять, если перестать мыслить вручную правлеными target-файлами и начать мыслить одной управляемой системой проекта.

## Базовая форма

```text
исходное состояние проекта -> render -> target outputs -> validate --strict -> handoff
```

<MermaidDiagram
  :chart="`
flowchart LR
  Source[Исходное состояние проекта] --> Render[plugin-kit-ai render]
  Render --> Runtime[Runtime outputs]
  Render --> Package[Package or extension outputs]
  Render --> Workspace[Workspace config outputs]
  Runtime --> Validate[validate --strict]
  Package --> Validate
  Workspace --> Validate
  Doctor[doctor and bootstrap when needed] -. supports .-> Validate
  Validate --> Handoff[Handoff to teammate, CI, machine, or downstream user]
`"
/>

Это основной цикл, на котором держатся публичная документация, generated API и поддерживаемые сценарии авторинга.

## Исходное состояние проекта

Исходное состояние проекта живёт в package-standard layout. Именно здесь репозиторий фиксирует намерение.

Это означает:

- исходное состояние проекта — долгосрочный источник истины
- target-файлы — это outputs
- миграция нужна для переноса native config в эту модель, а не для сохранения native files как основного контракта

## Render

`render` превращает исходное состояние проекта в артефакты для нужного target’а.

Его стоит воспринимать как часть нормального workflow, а не как удобный helper, который запускается только в конце.

## Target’ы

Не все target’ы равнозначны.

- runtime target’ы связаны с исполняемым поведением
- package и extension target’ы связаны с delivery artifacts
- workspace-config target’ы связаны с интеграцией и конфигурацией под управлением репозитория

Именно поэтому выбор target меняет практический контракт проекта, а не только формат файлов на выходе.

## Validation

`validate --strict` — это проверка готовности, которая доказывает, что исходное состояние проекта, generated artifacts и заявленный target реально согласованы.

Для Python и Node runtime target’ов `doctor` и `bootstrap` часто нужно воспринимать рядом с validation как часть одного практического сценария.

## Handoff

Цель всей системы — надёжный handoff:

- другому члену команды
- в CI
- на другую машину
- downstream-пользователю

Если репозиторий работает только у исходного автора, значит архитектура авторинга не справилась.

## Практическое следствие

Проект сознательно делает структуру более жёсткой, потому что публичная цель здесь не в “максимальной гибкости любой ценой”. Цель — предсказуемый процесс авторинга, явные границы поддержки и меньше drift между намерением и output.

Свяжите эту страницу с [Моделью target’ов](/ru/concepts/target-model), [Процессом авторинга](/ru/reference/authoring-workflow) и [Готовностью к продакшену](/ru/guide/production-readiness).
