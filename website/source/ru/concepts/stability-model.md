---
title: "Модель стабильности"
description: "Как plugin-kit-ai различает public-stable, public-beta и public-experimental области."
canonicalId: "page:concepts:stability-model"
section: "concepts"
locale: "ru"
generated: false
translationRequired: true
---

# Модель стабильности

`plugin-kit-ai` использует формальные contract terms, чтобы команды могли точно понять, что именно стандартизировать.

<MermaidDiagram
  :chart="`
flowchart TD
  Stable[public stable] --> Beta[public beta]
  Beta --> Experimental[public experimental]
  StableNote[Normal production expectations] -.-> Stable
  BetaNote[Supported but not frozen] -.-> Beta
  ExperimentalNote[Opt in churn] -.-> Experimental
`"
/>

## Публичный язык и формальный язык

Публичные docs используют более простой первый vocabulary:

- `Recommended` обычно указывает на самые сильные текущие `public-stable` lanes
- `Advanced` указывает на поддерживаемые surfaces, которые уже, осторожнее или специализированнее
- `Experimental` соответствует `public-experimental`

Когда вы задаёте compatibility policy, формальные термины важнее.

## Public-Stable

Воспринимайте `public-stable` как уровень, на который можно опираться с нормальными production expectations.

## Public-Beta

Воспринимайте `public-beta` как поддерживаемый, но ещё не замороженный контракт.

Используйте beta только тогда, когда компромисс осознан и действительно оправдан для продукта.

## Public-Experimental

Воспринимайте `public-experimental` как opt-in churn вне нормального compatibility expectation.

## Практическое правило

1. Предпочитайте рекомендуемый lane для того продукта, который вы строите.
2. Используйте точные формальные terms только тогда, когда нужна policy или compatibility precision.
3. Используйте `validate --strict` как readiness gate для repo, который собираетесь выпускать.
