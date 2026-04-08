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
    Работайте из одного репозитория, начинайте с рекомендуемого production-lane и позже расширяйтесь
    на packages, extensions и repo-managed integrations без разрыва authoring workflow.
  </p>
</div>

## Рекомендуемые production lanes

- `Codex runtime Go` для самого сильного runtime-пути по умолчанию.
- `Codex package`, когда продуктом является официальный пакет Codex.
- `Gemini packaging`, когда продуктом является пакет расширения Gemini.
- `Gemini Go runtime`, когда нужен продвинутый 9-hook runtime lane.
- `Claude default lane`, когда Claude hooks уже являются реальным требованием продукта.

## Что важно понять сразу

- один репозиторий остаётся source of truth по мере добавления новых lanes
- выбирайте lane под реальную delivery model сегодняшнего продукта
- расширяйтесь позже из того же repo, когда продукту понадобятся новые outputs
- используйте `generate` и `validate --strict` как общий readiness workflow

<div class="docs-grid">
  <a class="docs-card" href="./guide/quickstart">
    <h2>Быстрый старт</h2>
    <p>Начните с самого сильного стартового пути, а расширение оставьте на второй шаг.</p>
  </a>
  <a class="docs-card" href="./guide/what-you-can-build">
    <h2>Что можно построить</h2>
    <p>Посмотрите на общую форму продукта: runtime, package, extension и repo-managed integration lanes.</p>
  </a>
  <a class="docs-card" href="./guide/choose-a-target">
    <h2>Выбор target</h2>
    <p>Сопоставьте target с вашей delivery model, а не пытайтесь считать все outputs одним и тем же продуктом.</p>
  </a>
  <a class="docs-card" href="./reference/support-boundary">
    <h2>Точный контракт</h2>
    <p>Переходите в reference, когда нужен точный язык совместимости и границ поддержки.</p>
  </a>
</div>

## С чего лучше начинать

- Начинайте с `go`, когда нужен самый сильный runtime и release story.
- Выбирайте `node --typescript`, когда команде нужен основной non-Go runtime lane.
- Выбирайте `python`, когда репозиторий осознанно Python-first и остаётся локальным.
- Выбирайте package, extension и repo-managed integration lanes только тогда, когда именно они являются конечным продуктом.
- Используйте `validate --strict` как readiness gate перед handoff и CI.

## Читайте в таком порядке

<div class="docs-grid">
  <a class="docs-card" href="./guide/quickstart">
    <h2>1. Быстрый старт</h2>
    <p>Начните с одного рекомендуемого пути до того, как уйдёте в taxonomy target’ов.</p>
  </a>
  <a class="docs-card" href="./guide/what-you-can-build">
    <h2>2. Что можно построить</h2>
    <p>Посмотрите на общую product shape по runtime, package, extension и integration lanes.</p>
  </a>
  <a class="docs-card" href="./guide/choose-a-target">
    <h2>3. Выбор target</h2>
    <p>Выберите lane, который соответствует реальной delivery model продукта сегодня.</p>
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
