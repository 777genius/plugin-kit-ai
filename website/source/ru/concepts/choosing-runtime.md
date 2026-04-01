---
title: "Выбор runtime"
description: "Как выбирать между Go, Python, Node и shell."
canonicalId: "page:concepts:choosing-runtime"
section: "concepts"
locale: "ru"
generated: false
translationRequired: true
---

# Выбор runtime

Выбор runtime — это не только вопрос любимого языка. Он меняет то, как запускается плагин, что должно быть установлено на машине исполнения и насколько простыми будут CI и handoff.

<MermaidDiagram
  :chart="`
flowchart TD
  Start[Нужен runtime path] --> Prod{Нужен самый сильный production path}
  Prod -->|Да| Go[go]
  Prod -->|Нет| Local{Плагин repo local по дизайну}
  Local -->|Да| Team{Команда Python first или Node first}
  Team --> Python[python]
  Team --> Node[node or node --typescript]
  Local -->|Нет| Escape{Нужен только узкий escape hatch}
  Escape --> Shell[shell beta]
`"
/>

## Выбирайте Go, когда

- нужен самый сильный поддерживаемый путь
- нужны типизированные обработчики и самый чистый путь для продакшена
- важно, чтобы пользователям не приходилось ставить Python или Node для запуска плагина
- хочется минимальных проблем с bootstrap в CI и на других машинах

Go — рекомендуемый путь по умолчанию для production-oriented plugins.

## Выбирайте Python или Node, когда

- плагин по дизайну repo-local
- команда уже живёт в этом runtime
- вы готовы сами владеть runtime bootstrap
- вас устраивает, что на машине исполнения должен стоять Python `3.10+` или Node.js `20+`

Это поддерживаемый путь для локальных runtime-проектов, но он не убирает зависимости среды исполнения.

## Выбирайте Shell только когда

- нужен ограниченный escape hatch
- вы осознанно принимаете более узкий beta-contract

Shell не является рекомендуемым путём по умолчанию.

## Безопасная матрица выбора

| Ситуация | Рекомендуемый выбор |
| --- | --- |
| Самый сильный путь для продакшена | `go` |
| Основной стабильный путь без Go | `node --typescript` |
| Локальная Python-first команда | `python` |
| Ограниченный beta escape hatch | `shell` |

См. [Быстрый старт](/ru/guide/quickstart) для кратчайшего поддерживаемого пути и [Модель стабильности](/ru/concepts/stability-model) для словаря контрактов.
