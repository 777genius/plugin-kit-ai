---
title: "Модель стабильности"
description: "Как plugin-kit-ai различает public-stable, public-beta и experimental области."
canonicalId: "page:concepts:stability-model"
section: "concepts"
locale: "ru"
generated: false
translationRequired: true
---

# Модель стабильности

`plugin-kit-ai` специально явно показывает, какие области стабильны, а какие ещё меняются.

<MermaidDiagram
  :chart="`
flowchart TD
  Stable[public stable] --> Beta[public beta]
  Beta --> Experimental[experimental]
  StableNote[Normal production expectations] -.-> Stable
  BetaNote[Supported but not frozen] -.-> Beta
  ExperimentalNote[High churn and no normal compatibility expectation] -.-> Experimental
`"
/>

## Public-Stable

Воспринимайте `public-stable` как уровень, на который можно опираться с нормальными production-ожиданиями.

Примеры в текущем направлении проекта:

- core CLI команды вроде `init`, `validate`, `test`, `capabilities`, `inspect`, `install` и `version`
- рекомендуемый путь через Go SDK
- стабильный локальный Python и Node subset на поддерживаемых runtime target’ах
- strict validation и deterministic generated-artifact checks

## Public-Beta

`public-beta` — это поддерживаемый, но ещё не замороженный контракт.

Обычно сюда попадают:

- target’ы, которые ещё расширяют своё поддерживаемое поведение
- config или packaging области с более высоким churn
- удобные workflow-фичи, которые полезны, но пока не на том же уровне гарантий, что основной путь

Beta можно использовать в реальных проектах, если компромисс оправдан, но не стоит относиться к beta так, будто у неё те же долгосрочные гарантии совместимости, что у stable path.

## Public-Experimental

Experimental значит именно это:

- полезно для ранних пользователей
- сознательно вне нормального compatibility expectation
- может резко меняться или исчезнуть

Не делайте experimental области частью долгоживущего production-контракта, если не готовы сами поглощать churn.

## Практическое правило

Безопасный default такой:

1. Предпочитайте `go`, когда нужен самый сильный путь.
2. Предпочитайте явно стабильные CLI и runtime области вместо convenience beta paths.
3. Используйте `validate --strict` как главную проверку готовности для локальных Python и Node runtime-проектов.

См. [Выбор runtime](/ru/concepts/choosing-runtime) для модели выбора пути и [Политику версий и совместимости](/ru/reference/version-and-compatibility) для публичного policy-слоя, который команда может сделать стандартом.
