---
title: "Интеграция с CI"
description: "Как превратить публичный authored flow в стабильный CI gate для plugin-kit-ai проектов."
canonicalId: "page:guide:ci-integration"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Интеграция с CI

Самая безопасная история для CI не обязана быть сложной. Она просто должна строго проверять публичный контракт проекта.

<MermaidDiagram
  :chart="`
flowchart LR
  Doctor[doctor] --> Bootstrap[bootstrap when needed]
  Bootstrap --> Render[render]
  Render --> Validate[validate --strict]
  Validate --> Smoke[smoke or bundle checks]
`"
/>

## Минимальный CI gate

Для большинства authored projects базовый путь такой:

```bash
plugin-kit-ai doctor .
plugin-kit-ai render .
plugin-kit-ai validate . --platform <target> --strict
```

Если у вашего path есть stable smoke tests или bundle checks, добавляйте их после validation gate, а не вместо него.

## Почему это работает

- `doctor` рано ловит отсутствующие runtime prerequisites
- `render` доказывает, что generated outputs можно воспроизвести из исходного состояния проекта
- `validate --strict` доказывает, что repo внутренне согласован для выбранного target

Если repo multi-target, эта же логика должна выполняться по каждому target’у, который команда реально обещает поддерживать.

## Заметки по runtime

### Go

Go — самый чистый CI path, потому что execution machine не обязана иметь Python или Node просто для удовлетворения runtime path.

### Node/TypeScript

Явно добавляйте bootstrap:

```bash
plugin-kit-ai doctor .
plugin-kit-ai bootstrap .
plugin-kit-ai render .
plugin-kit-ai validate . --platform codex-runtime --strict
```

### Python

Используйте тот же паттерн, что и для Node, и явно фиксируйте версию Python в CI.

## Частые ошибки в CI

- запускать `validate --strict` без `render`
- воспринимать rendered artifacts как вручную поддерживаемые файлы
- забывать про runtime prerequisites для Node или Python paths
- обещать совместимость для target, который находится вне stable support boundary

## Рекомендуемое правило

Если CI не может воспроизвести authored outputs и пройти `validate --strict`, значит repo не готов к стабильному handoff.

Для multi-target repo это означает не "один зелёный прогон где-то рядом", а явный зелёный прогон по каждому target’у в support scope.

Свяжите эту страницу с [Готовностью к продакшену](/ru/guide/production-readiness), [Границей поддержки](/ru/reference/support-boundary) и [Диагностикой проблем](/ru/reference/troubleshooting).
