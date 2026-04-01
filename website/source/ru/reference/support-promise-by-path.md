---
title: "Обещания поддержки по путям"
description: "Сравните обещания поддержки, операционную цену и безопасные варианты по умолчанию для Go, Node, Python, shell, package и workspace-config путей."
canonicalId: "page:reference:support-promise-by-path"
section: "reference"
locale: "ru"
generated: false
translationRequired: true
---

# Обещания поддержки по путям

Откройте эту страницу, когда команда уже поняла модель продукта и теперь хочет получить один практический ответ: какой путь несёт самое сильное обещание, а какие компромиссы становятся вашей ответственностью?

## Выбор за 60 секунд

- Нужен самый сильный production default: выбирайте Go.
- Нужен поддерживаемый interpreted runtime path: выбирайте Node или Python и сразу считайте `validate --strict` и runtime bootstrap частью своего контракта.
- Нужен узкий escape hatch: относитесь к shell как к beta и держите его минимальным.
- Нужен артефакт, extension или repo-owned config вместо исполняемой plugin logic: осознанно выбирайте package или workspace-config targets, а не ориентируйтесь только на знакомое имя экосистемы.

## Что помогает решить эта страница

- какой путь безопаснее всего брать как вариант по умолчанию для новой команды
- у какого пути самое сильное публичное обещание поддержки
- какой путь перекладывает больше операционной цены на ваш repo и execution machines
- в какой момент target уже перестаёт быть runtime story

## Короткое правило

- Go — самый сильный поддерживаемый runtime path.
- Node и Python — поддерживаемые локальные runtime paths, но ваш repo берёт на себя больше runtime bootstrap.
- Shell — узкий beta escape hatch, а не default.
- Package и workspace-config targets — полноценные outputs, но они не являются runtime-контрактами.

## Сравнительная таблица

| Путь | Публичное обещание | Что остаётся на вашей стороне | Лучший вариант по умолчанию для | Когда не стоит брать |
| --- | --- | --- | --- | --- |
| Go runtime | самый сильный stable runtime path | обычная дисциплина по repo, CI и релизам | долгоживущие production plugin repos | команде нужен только быстрый локальный эксперимент на другом runtime |
| Node/TypeScript runtime | stable local runtime path на поддерживаемых targets | наличие Node.js, runtime bootstrap, hygiene зависимостей | repo-local teams, которые уже живут в Node | вам нужен самый лёгкий операционный handoff между машинами |
| Python runtime | stable local runtime path на поддерживаемых targets | наличие Python, bootstrap окружения, hygiene зависимостей | automation-heavy local teams, которые уже живут в Python | вам нужна работа без зависимости от интерпретатора на execution machines |
| Shell runtime | узкий beta escape hatch | переносимость shell, более узкий контракт, повышенная осторожность | жёстко ограниченные точечные escape hatches | вам нужен основной долгосрочный production path |
| Package или extension outputs | стабильный packaging-oriented output там, где он явно поддерживается | packaging workflow, release discipline, target-specific expectations | installable artifacts вроде Codex package или Gemini extension outputs | вам на самом деле нужна исполняемая runtime logic |
| Workspace-config outputs | стабильное repo-owned владение workspace-конфигом там, где это документировано | жизненный цикл repo-owned config и проверки editor/tool integration | repo-managed integration files в стиле Cursor или OpenCode | вам нужны runtime handlers, а не configuration files |

## Как читать эту таблицу

- `Публичное обещание` показывает силу документированной support line.
- `Что остаётся на вашей стороне` показывает, куда уходит операционная цена.
- `Когда не стоит брать` защищает от самой частой категории ошибок: когда package или workspace outputs начинают воспринимать как runtime plugins, а interpreted runtimes — как пути без bootstrap.

## Самые безопасные варианты по умолчанию по ситуации

| Ситуация | Самый безопасный default |
| --- | --- |
| Новая команда, нужен самый сильный production default | Go runtime |
| Локальная команда уже живёт в Node | Node/TypeScript runtime |
| Локальная команда уже живёт в Python | Python runtime |
| Артефакт или extension и есть конечный продукт | package или extension target |
| Конечный продукт — repo-managed editor или tool integration | workspace-config target |

## В чём реальная разница по цене

- Go снимает больше всего downstream runtime friction.
- Node и Python сохраняют managed project model, но не снимают с вас ownership над интерпретатором.
- Package и workspace outputs могут быть правильным продуктом, но их нельзя внутренне продавать как «почти то же самое, что runtime».
- Shell полезен только тогда, когда вы сознательно принимаете более узкое обещание.

## Где обычно ошибаются

- Считают, что у всех target’ов примерно одинаковое runtime promise. Это не так.
- Относятся к install wrappers как к runtime API. Это install channels.
- Думают, что Node или Python “слабые” потому что они не Go. Они поддерживаются, но переносят больше операционной цены на ваш repo.
- Считают package или workspace outputs второстепенными. Это реальные поддерживаемые outputs, просто они решают другую задачу.

## С чем читать вместе

- Откройте [Границу поддержки](/ru/reference/support-boundary), если нужна короткая линия stable vs beta.
- Откройте [Поддержку target’ов](/ru/reference/target-support), если нужна компактная target matrix.
- Откройте [Выбор runtime](/ru/concepts/choosing-runtime), если вы ещё выбираете между Go, Node, Python и shell.
- Откройте [Package и workspace targets](/ru/guide/package-and-workspace-targets), если продукт вообще не является исполняемым runtime behavior.
