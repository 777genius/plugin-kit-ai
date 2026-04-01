---
title: "Процесс авторинга"
description: "Основной workflow от init до render, validate, test и handoff."
canonicalId: "page:reference:authoring-workflow"
section: "reference"
locale: "ru"
generated: false
translationRequired: true
---

# Процесс авторинга

Рекомендуемый workflow намеренно простой:

```text
init -> render -> validate --strict -> test -> handoff
```

<MermaidDiagram
  :chart="`
flowchart LR
  Init[init or migrate] --> Render[render]
  Render --> Validate[validate --strict]
  Validate --> Test[test or smoke checks]
  Test --> Handoff[handoff]
  Bootstrap[doctor or bootstrap when needed] -. supports .-> Render
  Bootstrap -. supports .-> Validate
`"
/>

## Что означает каждый шаг

| Шаг | Назначение |
| --- | --- |
| `init` | Создать package-standard layout проекта |
| `render` | Сгенерировать target artifacts из исходного состояния проекта |
| `validate --strict` | Запустить главную проверку готовности |
| `test` | Запустить стабильные smoke-тесты там, где это применимо |
| `export` / bundle flow | Выпустить handoff artifacts для поддерживаемых Python и Node сценариев |

## Правила, которые держат repo здоровым

- исходное состояние проекта живёт в package-standard layout
- generated target files — это outputs, а не долгосрочный source of truth
- strict validation — это обязательная проверка, а не необязательная опция

Этот workflow одинаково важен и для single-target, и для multi-target repo.

Разница только в том, что в multi-target проекте цикл `render` и `validate` повторяется для каждого target’а, который repo действительно обещает поддерживать.

## Когда workflow меняется

Workflow может расширяться в специальных случаях:

- `doctor` и `bootstrap` важны для Python и Node runtime-путей
- `import` и `normalize` важны при миграции native config в управляемую модель проекта
- bundle commands важны для portable Python и Node handoff flows

Начинайте с [Быстрого старта](/ru/guide/quickstart), если нужен самый короткий путь, или с [Миграции существующего native config](/ru/guide/migrate-existing-config), если вы входите с legacy target files.
