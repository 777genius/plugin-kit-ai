---
title: "Граница поддержки"
description: "Краткий гид по тому, что plugin-kit-ai считает stable, beta и сознательно вне области поддержки."
canonicalId: "page:reference:support-boundary"
section: "reference"
locale: "ru"
generated: false
translationRequired: true
---

# Граница поддержки

Эта страница коротко отвечает на простой вопрос: на что можно опираться уже сейчас, а к чему стоит относиться осторожно.

## Безопасные значения по умолчанию

- Go — рекомендуемый production path.
- `validate --strict` — главная проверка готовности для локальных Python и Node runtime-проектов.
- CLI wrappers — это способы установки CLI, а не runtime API.

## Что stable по умолчанию

- main public CLI contract
- рекомендуемый путь через Go SDK
- стабильный локальный Python и Node subset на поддерживаемых runtime target’ах
- target’ы, которые явно помечены как stable в generated support matrix

## Что использовать осторожно

- beta paths, которые ещё меняются
- workspace-config targets, когда вам на самом деле нужен исполняемый плагин
- install wrappers, если вам в действительности нужен runtime API или SDK

## Что вне scope

- считать, что у всех targets одинаковые runtime guarantees
- относиться к wrapper packages как к SDK или runtime-контрактам
- считать, что experimental surfaces несут долгосрочные compatibility promises

Свяжите эту страницу с [Политикой версий и совместимости](/ru/reference/version-and-compatibility), [Поддержкой target’ов](/ru/reference/target-support) и [Моделью стабильности](/ru/concepts/stability-model).
