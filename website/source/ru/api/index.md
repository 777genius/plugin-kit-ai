---
title: "API"
description: "Сгенерированный API-справочник для plugin-kit-ai."
canonicalId: "page:api:index"
section: "api"
locale: "ru"
generated: false
translationRequired: true
aside: false
outline: false
---

<div class="docs-hero docs-hero--compact">
  <p class="docs-kicker">СГЕНЕРИРОВАННЫЙ СПРАВОЧНИК</p>
  <h1>API поверхности</h1>
  <p class="docs-lead">
    Этот справочник генерируется из реального CLI, пакетов и структурированных метаданных. Он разделён по публичным разделам, чтобы по мере роста проекта API оставался понятным и предсказуемым.
  </p>
</div>

<div class="docs-grid">
  <a class="docs-card" href="./cli/">
    <h2>CLI</h2>
    <p>Команды, экспортированные из живого дерева Cobra.</p>
  </a>
  <a class="docs-card" href="./go-sdk/">
    <h2>Go SDK</h2>
    <p>Публичные Go-пакеты для стабильных путей интеграции.</p>
  </a>
  <a class="docs-card" href="./runtime-node/">
    <h2>Node Runtime</h2>
    <p>Типизированные runtime-helpers для JS и TS.</p>
  </a>
  <a class="docs-card" href="./runtime-python/">
    <h2>Python Runtime</h2>
    <p>Только публичные Python runtime-helpers, без wrapper-пакетов для установки.</p>
  </a>
  <a class="docs-card" href="./platform-events/">
    <h2>События платформ</h2>
    <p>События и точки входа, сгруппированные по целевым платформам.</p>
  </a>
  <a class="docs-card" href="./capabilities/">
    <h2>Capabilities</h2>
    <p>Взгляд на систему через capabilities, а не только через дерево пакетов.</p>
  </a>
</div>

## Выбор за 60 секунд

- Открывайте `CLI`, когда занимаетесь авторингом, validate, bundle или inspect для plugin repo.
- Открывайте `Go SDK`, когда строите самый сильный production-oriented runtime path.
- Открывайте `Node Runtime` или `Python Runtime`, когда уже выбрали поддерживаемый repo-local interpreted runtime path и теперь нужны helper APIs.
- Открывайте `Platform Events`, когда уже знаете target platform и нужен event-level contract.
- Открывайте `Capabilities`, когда хотите сравнивать похожее поведение между платформами, а не читать одну platform tree за раз.

## С чего лучше начать

- Нужна главная пользовательская поверхность: начинайте с [CLI](./cli/).
- Нужен самый сильный production default: начинайте с [Go SDK](./go-sdk/).
- Нужны interpreted runtime helpers: начинайте с [Node Runtime](./runtime-node/) или [Python Runtime](./runtime-python/).
- Нужна детализация по событиям платформы: начинайте с [Platform Events](./platform-events/).
- Нужна карта поведения между платформами: начинайте с [Capabilities](./capabilities/).

## Выбор по роли

<div class="docs-grid">
  <a class="docs-card" href="./cli/">
    <h2>Я веду plugin repo</h2>
    <p>Начинайте с CLI, когда нужны реальные команды для init, render, validate, inspect и bundle-операций.</p>
  </a>
  <a class="docs-card" href="./go-sdk/">
    <h2>Мне нужен самый сильный runtime path</h2>
    <p>Начинайте с Go SDK, когда нужен самый сильный поддерживаемый runtime-контракт и наименьшая downstream runtime-нагрузка.</p>
  </a>
  <a class="docs-card" href="./runtime-node/">
    <h2>Я отвечаю за Node или TypeScript путь</h2>
    <p>Начинайте с Node Runtime, когда repo уже выбрал поддерживаемый локальный Node path и теперь нужны helper APIs.</p>
  </a>
  <a class="docs-card" href="./runtime-python/">
    <h2>Я отвечаю за Python путь</h2>
    <p>Начинайте с Python Runtime, когда repo уже выбрал поддерживаемый локальный Python path и теперь нужны helper APIs.</p>
  </a>
  <a class="docs-card" href="./platform-events/">
    <h2>Я глубоко интегрируюсь с одной платформой</h2>
    <p>Начинайте с Platform Events, когда главный вопрос — event-level поведение одной целевой платформы.</p>
  </a>
  <a class="docs-card" href="./capabilities/">
    <h2>Я сравниваю поведение между платформами</h2>
    <p>Начинайте с Capabilities, когда нужен единый cross-platform map, а не чтение platform trees по одной.</p>
  </a>
</div>

## Выбор по вопросу

- «Какую команду запускать дальше?» Начинайте с [CLI](./cli/).
- «Какие пакеты должен импортировать мой Go plugin?» Начинайте с [Go SDK](./go-sdk/).
- «Какой helper API нужен моему поддерживаемому Node или Python path?» Начинайте с [Node Runtime](./runtime-node/) или [Python Runtime](./runtime-python/).
- «Какие события вообще есть у этой платформы?» Начинайте с [Platform Events](./platform-events/).
- «Какая capability есть сразу на нескольких платформах?» Начинайте с [Capabilities](./capabilities/).

## Как выбрать нужную поверхность

- Открывайте `CLI`, когда нужны команды, флаги и сам рабочий процесс автора плагина.
- Открывайте `Go SDK`, когда строите самый сильный путь для production runtime-плагина.
- Открывайте `Node Runtime` или `Python Runtime`, когда нужны helper APIs для поддерживаемых локальных Python или Node проектов.
- Открывайте `Platform Events`, когда выбираете события конкретной платформы.
- Открывайте `Capabilities`, когда нужен взгляд поперёк платформ на то, на что plugin может реагировать или что может контролировать.

## Что покрывает эта API-зона

- живое дерево команд Cobra
- публичные Go-пакеты
- shared runtime helper APIs для Node и Python
- события конкретных платформ
- metadata по capabilities поперёк платформ

## Чем эта API-зона не является

- Это не лучший первый вход, если вы ещё выбираете target, runtime или starter.
- Она не заменяет guide-страницы для first-time setup, delivery и team handoff.
- Это generated reference, привязанный к реальным исходным данным, поэтому лучше всего он работает после того, как вы уже понимаете, какая поверхность вам нужна.

## С чем читать вместе

- [Что можно построить](/ru/guide/what-you-can-build), если вы ещё выбираете между runtime, package и workspace outputs.
- [Выбор target](/ru/guide/choose-a-target), если вам ещё нужен правильный target family.
- [Обещания поддержки по путям](/ru/reference/support-promise-by-path), если главное решение связано с силой promise и операционной ценой.
