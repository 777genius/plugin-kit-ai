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

## Выбор за 60 секунд

- Нужен самый безопасный production path по умолчанию: выбирайте Go.
- Нужен самый безопасный ориентир для interpreted runtimes: доверяйте `validate --strict` для поддерживаемых Python и Node путей.
- Нужно простое правило про wrappers: относитесь к ним как к путям установки CLI, а не как к runtime API.
- Нужна быстрая матрица по target’ам: свяжите эту страницу с [Поддержкой target’ов](/ru/reference/target-support).
- Нужна короткая сравнительная таблица по обещаниям разных путей: откройте [Обещания поддержки по путям](/ru/reference/support-promise-by-path).

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

## Что помогает решить эта страница

- безопасен ли путь как default choice
- достаточно ли стабилен путь для долгосрочного командного использования
- не путаете ли вы install, packaging или workspace lanes с runtime contract

Свяжите эту страницу с [Обещаниями поддержки по путям](/ru/reference/support-promise-by-path), [Поддержкой target’ов](/ru/reference/target-support) и [Моделью стабильности](/ru/concepts/stability-model).
Если реальный вопрос уже в том, может ли один repo оставаться особым без превращения в нездоровый drift, прочитайте [Политику здоровых исключений](/ru/guide/healthy-exception-policy).
