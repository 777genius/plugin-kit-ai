---
title: "Документация plugin-kit-ai"
description: "Публичная документация по plugin-kit-ai."
canonicalId: "page:home"
section: "home"
locale: "ru"
generated: false
translationRequired: true
aside: false
outline: false
---

<div class="docs-hero docs-hero--feature">
  <p class="docs-kicker">ПУБЛИЧНАЯ ДОКУМЕНТАЦИЯ</p>
  <h1>plugin-kit-ai</h1>
  <p class="docs-lead">
    Работайте из одного репозитория, начинайте с Go по умолчанию, а потом при необходимости
    добавляйте packages, Claude hooks, Gemini или настройку интеграций в самом репозитории.
  </p>
</div>

## Старт по умолчанию

- `Codex runtime Go` - это старт по умолчанию для самого сильного runtime и release story.

## Что важно понять сразу

- один репозиторий остаётся source of truth по мере добавления новых lanes
- выбирайте стартовый путь под то, что вам нужно прямо сейчас
- расширяйтесь позже из того же repo, когда продукту понадобятся новые outputs
- используйте `generate` и `validate --strict` как общий readiness workflow

## Поддерживаемые пути для Node и Python

- `codex-runtime --runtime node --typescript` - основной поддерживаемый non-Go путь.
- `codex-runtime --runtime python` - поддерживаемый путь для Python-first команды.
- оба варианта являются локальными interpreted runtime paths, поэтому на машине исполнения всё равно нужен Node.js `20+` или Python `3.10+`.
- это понятные ранние варианты для команд, которые уже живут в этих стеках, но это не старт по умолчанию.

<div class="docs-grid">
  <a class="docs-card" href="./guide/quickstart">
    <h2>Быстрый старт</h2>
    <p>Начните с самого сильного стартового пути, а всё остальное добавляйте позже.</p>
  </a>
  <a class="docs-card" href="./guide/what-you-can-build">
    <h2>Что можно построить</h2>
    <p>Посмотрите на общую форму продукта: runtime, package, extension и настройка интеграций в самом repo.</p>
  </a>
  <a class="docs-card" href="./guide/choose-a-target">
    <h2>Выбор target</h2>
    <p>Сопоставьте target с тем, как вы хотите поставлять плагин, а не пытайтесь считать все outputs одним и тем же.</p>
  </a>
  <a class="docs-card" href="./reference/support-boundary">
    <h2>Точный контракт</h2>
    <p>Переходите в reference, когда нужен точный язык совместимости и границ поддержки.</p>
  </a>
</div>

## Что добавлять потом

- Добавляйте `Claude default lane`, когда Claude hooks и есть реальное требование продукта.
- Добавляйте `Codex package` или `Gemini packaging`, когда продуктом становится package или extension output.
- Добавляйте `OpenCode` или `Cursor`, когда repo должен хранить и вести настройку интеграции.
- Используйте `validate --strict` как readiness gate перед handoff и CI.

## Читайте в таком порядке

<div class="docs-grid">
  <a class="docs-card" href="./guide/quickstart">
    <h2>1. Быстрый старт</h2>
    <p>Начните с одного рекомендуемого пути до того, как уйдёте в детали по target'ам.</p>
  </a>
  <a class="docs-card" href="./guide/what-you-can-build">
    <h2>2. Что можно построить</h2>
    <p>Посмотрите на общую product shape по runtime, package, extension и integration lanes.</p>
  </a>
  <a class="docs-card" href="./guide/choose-a-target">
    <h2>3. Выбор target</h2>
    <p>Выберите target, который соответствует тому, как вы реально хотите поставлять плагин сегодня.</p>
  </a>
  <a class="docs-card" href="./reference/support-boundary">
    <h2>4. Граница поддержки</h2>
    <p>Открывайте reference cluster, когда нужен точный compatibility language и support details.</p>
  </a>
</div>

Если вы новый пользователь, на этих четырёх страницах уже можно остановиться.

## Текущий базовый релиз репозитория

- Текущая публичная опорная версия в этом наборе docs - [`v1.0.6`](/ru/releases/v1-0-6).
- Этот релиз сделал shared runtime-package delivery для Python и Node полноценной поддерживаемой историей.
- Начинайте с него, если нужен актуальный baseline.
