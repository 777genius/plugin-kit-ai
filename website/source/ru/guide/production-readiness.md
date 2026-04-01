---
title: "Готовность к продакшену"
description: "Публичный checklist, помогающий понять, готов ли plugin-kit-ai проект к CI, handoff и широкому использованию."
canonicalId: "page:guide:production-readiness"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Готовность к продакшену

Используйте этот checklist перед тем, как называть проект production-ready, handoff-ready или достаточно стабильным для широкого показа.

<MermaidDiagram
  :chart="`
flowchart LR
  Path[Выбран путь] --> Source[Один source of truth]
  Source --> Checks[render and validate gates]
  Checks --> Boundary[Support boundary confirmed]
  Boundary --> Handoff[Docs and handoff are explicit]
  Handoff --> Ready[Production ready]
`"
/>

## 1. Осознанно выберите путь

- по умолчанию выбирайте Go, когда нужен самый сильный путь для продакшена
- выбирайте Node/TypeScript только тогда, когда компромисс локального non-Go runtime действительно нужен
- выбирайте Python только тогда, когда проект остаётся локальным для репозитория, а команда осознанно Python-first
- не воспринимайте workspace-config или packaging target’ы так, будто у них те же runtime guarantees, что у основного пути

## 2. Держите один источник истины

- исходное состояние проекта должно жить в package-standard layout
- generated target files — это outputs, а не долгосрочный источник истины
- не патчите generated files вручную, если ожидаете, что `render` потом сохранит эти правки

## 3. Прогоняйте обязательные проверки

Как минимум, репозиторий должен чисто проходить такой flow:

```bash
plugin-kit-ai doctor .
plugin-kit-ai render .
plugin-kit-ai validate . --platform <target> --strict
```

Для Python и Node runtime-путей `doctor` и `bootstrap` — это часть готовности, а не необязательная полировка.

Если repo поддерживает несколько target’ов, этот gate должен явно повторяться для каждого target’а в заявленном support scope.

## 4. Проверяйте границу поддержки

- убедитесь, что основной target и каждый дополнительный target в scope действительно находятся внутри публичной границы поддержки
- подтвердите, является ли путь stable, beta или сознательно уже основного пути
- смотрите generated target support matrix до того, как обещать совместимость другим пользователям

## 5. Не смешивайте установку и API

- Homebrew, npm и PyPI пакеты — это способы установить CLI
- это не runtime API и не SDK
- публичный API живёт в generated API section и в задокументированных stable workflows

## 6. Документируйте handoff

Для публичного repo должны быть очевидны такие вещи:

- какой target является основным и какие дополнительные target’ы реально поддерживаются
- какой runtime используется и меняется ли он по target’ам
- как ставятся prerequisites
- какая команда или какой набор команд являются главной проверкой готовности
- зависит ли проект от shared runtime package или от Go SDK path

## 7. Ссылайтесь на актуальные release notes

Если репозиторий опирается на текущий стабильный путь, ведите пользователей на последний release note, где объяснены путь по умолчанию и миграционная история.

Сейчас такой базовый релиз — [v1.0.6](/ru/releases/v1-0-6).

## Финальное правило

Если коллега не может клонировать репозиторий, пройти задокументированный flow, успешно выполнить `validate --strict` и понять выбранный путь без tribal knowledge, значит проект ещё не готов к продакшену.

Свяжите эту страницу с [Границей поддержки](/ru/reference/support-boundary), [Поддержкой target’ов](/ru/reference/target-support) и [Процессом авторинга](/ru/reference/authoring-workflow).
