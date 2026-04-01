---
title: "Частые вопросы"
description: "Частые вопросы про выбор путей, установку, target’ы и рабочий процесс."
canonicalId: "page:reference:faq"
section: "reference"
locale: "ru"
generated: false
translationRequired: true
---

# Частые вопросы

## С чего начинать: Go, Python или Node?

Начинайте с Go, если нет реальной причины выбрать иначе. Выбирайте Node/TypeScript для основного поддерживаемого пути без Go. Выбирайте Python, когда плагин остаётся локальным для репозитория, а команда уже Python-first.

## npm и PyPI пакеты `plugin-kit-ai` — это runtime API?

Нет. Это способы установить CLI. Они не являются публичным runtime API и не являются SDK.

## Когда использовать bundle-команды?

Используйте bundle-команды, когда нужны переносимые Python или Node артефакты, которые другая машина сможет скачать или установить. Не путайте их с основным способом установки CLI.

## Можно ли держать native target files как source of truth?

Это не рекомендуемая долгосрочная модель. Исходное состояние проекта должно жить в package-standard layout, а target-файлы должны быть сгенерированными output-файлами.

## `render` — это опционально?

Нет, если вы хотите управляемую модель проекта. `render` — часть основного процесса, а не случайный helper.

## `validate --strict` — это опционально?

Воспринимайте его как главную проверку готовности, особенно для локальных Python и Node runtime-проектов.

## Один repo может вести несколько target’ов?

Да. Это одна из основных идей `plugin-kit-ai`.

Практическое правило такое:

- держите authored state в одном managed repo
- начинайте с главного target’а сегодня
- добавляйте другие target’ы, когда появляются реальные product, delivery или integration требования

См. [Один проект, несколько target’ов](/ru/guide/one-project-multiple-targets) и [Модель target’ов](/ru/concepts/target-model).

## Все targets одинаково стабильны?

Нет. Runtime, packaging, extension и workspace-config target’ы не несут одинаковое обещание по поддержке.

См. [Границу поддержки](/ru/reference/support-boundary), [Поддержку target’ов](/ru/reference/target-support) и [Процесс авторинга](/ru/reference/authoring-workflow).
