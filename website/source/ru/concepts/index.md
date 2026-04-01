---
title: "Концепции"
description: "Ключевые концепции plugin-kit-ai."
canonicalId: "page:concepts:index"
section: "concepts"
locale: "ru"
generated: false
translationRequired: true
aside: false
outline: false
---

<div class="docs-hero docs-hero--compact">
  <p class="docs-kicker">КОНЦЕПЦИИ</p>
  <h1>Ментальная модель</h1>
  <p class="docs-lead">
    Публичные концепции объясняют модель продукта, уровни поддержки и типы target’ов, не затягивая пользователя во внутреннюю release-механику.
  </p>
</div>

## Базовые идеи

- Публичные docs описывают поддерживаемое пользовательское поведение, а не внутренние процессы команды.
- API reference генерируется из реальных источников истины.
- Install wrappers — это каналы доставки CLI, а не программные API.
- Stability и maturity не менее важны, чем сами сигнатуры.

## В каком порядке читать

- Начните с [Зачем plugin-kit-ai](/ru/concepts/why-plugin-kit-ai), если ещё решаете, подходит ли проект вашей команде.
- Прочитайте [Модель управляемого проекта](/ru/concepts/managed-project-model), если вам нужно самое короткое объяснение того, чем вообще является продукт.
- Прочитайте [Выбор runtime](/ru/concepts/choosing-runtime) до того, как выбирать Go, Python, Node или shell.
- Прочитайте [Модель target’ов](/ru/concepts/target-model), прежде чем считать любой target полноценным runtime-плагином.
- Прочитайте [Модель стабильности](/ru/concepts/stability-model), прежде чем обещать долгую совместимость другим пользователям.

<div class="docs-grid">
  <a class="docs-card" href="./why-plugin-kit-ai">
    <h2>Зачем plugin-kit-ai</h2>
    <p>Поймите, какую проблему решает проект и когда это вообще не ваш инструмент.</p>
  </a>
  <a class="docs-card" href="./managed-project-model">
    <h2>Модель управляемого проекта</h2>
    <p>Посмотрите на самое короткое определение продукта: один authored repo, rendered outputs, строгая validation и явные границы путей.</p>
  </a>
  <a class="docs-card" href="./authoring-architecture">
    <h2>Архитектура авторинга</h2>
    <p>Посмотрите, как исходное состояние проекта, generated-файлы, validation, target’ы и handoff складываются в единую систему.</p>
  </a>
  <a class="docs-card" href="./stability-model">
    <h2>Модель стабильности</h2>
    <p>Поймите, что значат public-stable, beta и experimental, прежде чем завязываться на конкретную поверхность API или target.</p>
  </a>
  <a class="docs-card" href="./target-model">
    <h2>Модель target’ов</h2>
    <p>Посмотрите на практическую разницу между runtime, package, extension и workspace-config target’ами.</p>
  </a>
  <a class="docs-card" href="./choosing-runtime">
    <h2>Выбор runtime</h2>
    <p>Выберите между Go, Python, Node и shell по практическим ограничениям проекта, а не только по вкусу команды.</p>
  </a>
</div>
