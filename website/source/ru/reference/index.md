---
title: "Справочник"
description: "Справочные материалы про каналы установки, контракты и ключевые стабильные правила."
canonicalId: "page:reference:index"
section: "reference"
locale: "ru"
generated: false
translationRequired: true
aside: false
outline: false
---

<div class="docs-hero docs-hero--compact">
  <p class="docs-kicker">СПРАВОЧНИК</p>
  <h1>Стабильные факты</h1>
  <p class="docs-lead">
    Справочные страницы держат публичные правила в обозримом виде: способы установки, support matrices и точные правила, на которые можно опираться в работе.
  </p>
</div>

## Выбор за 60 секунд

- Нужен точный ответ по установке: откройте [Каналы установки](/ru/reference/install-channels).
- Нужен точный контракт репозитория: откройте [Стандарт репозитория](/ru/reference/repository-standard).
- Нужен точный повседневный authoring path: откройте [Процесс авторинга](/ru/reference/authoring-workflow).
- Нужна точная граница поддержки: откройте [Границу поддержки](/ru/reference/support-boundary) и [Поддержку target’ов](/ru/reference/target-support).
- Нужна короткая сравнительная таблица по путям: откройте [Обещания поддержки по путям](/ru/reference/support-promise-by-path).
- Нужна rule по версиям и совместимости: откройте [Политику версий и совместимости](/ru/reference/version-and-compatibility).
- Нужен короткий ответ на частую проблему: откройте [Частые вопросы](/ru/reference/faq) или [Диагностику проблем](/ru/reference/troubleshooting).
- Нужен публичный путь для помощи или contribution: откройте [Как получить помощь и внести вклад](/ru/reference/get-help-and-contribute).

## Когда идти в этот раздел

- когда вам нужен точный контракт, а не tutorial
- когда нужно быстро понять, что stable, а что beta
- когда нужен короткий и точный ответ про установку, поддержку, validation или устройство repo

## С чего лучше начать

- Путаетесь в установке: [Каналы установки](/ru/reference/install-channels)
- Нужно понять, что реально поддерживается: [Граница поддержки](/ru/reference/support-boundary)
- Нужно быстро понять, у какого пути сильнее обещание: [Обещания поддержки по путям](/ru/reference/support-promise-by-path)
- Нужно быстро увидеть, какие target’ы готовы для runtime: [Поддержка target’ов](/ru/reference/target-support)
- Нужно понять, как выглядит здоровый repo: [Стандарт репозитория](/ru/reference/repository-standard)
- Нужно увидеть канонический рабочий путь: [Процесс авторинга](/ru/reference/authoring-workflow)

<div class="docs-grid">
  <a class="docs-card" href="./install-channels">
    <h2>Каналы установки</h2>
    <p>Поймите разницу между Homebrew, npm, PyPI и verified script, не смешивая способы установки с runtime API.</p>
  </a>
  <a class="docs-card" href="./authoring-workflow">
    <h2>Процесс авторинга</h2>
    <p>Посмотрите на канонический `init -> render -> validate --strict -> test -> handoff` flow.</p>
  </a>
  <a class="docs-card" href="./repository-standard">
    <h2>Стандарт репозитория</h2>
    <p>Посмотрите, как должен выглядеть здоровый plugin repo и какие файлы являются source of truth, а какие — generated outputs.</p>
  </a>
  <a class="docs-card" href="./support-boundary">
    <h2>Граница поддержки</h2>
    <p>Посмотрите, что stable, что beta и что не стоит воспринимать как долгосрочный контракт.</p>
  </a>
  <a class="docs-card" href="./target-support">
    <h2>Поддержка target’ов</h2>
    <p>Смотрите, какие target’ы подходят для runtime, какие относятся только к packaging, а какие сознательно стоят вне главного стабильного пути.</p>
  </a>
  <a class="docs-card" href="./support-promise-by-path">
    <h2>Обещания поддержки по путям</h2>
    <p>Сравните Go, Node, Python, shell, package и workspace-config пути по силе обещаний и операционной цене.</p>
  </a>
  <a class="docs-card" href="./version-and-compatibility">
    <h2>Политика версий и совместимости</h2>
    <p>Поймите публичный baseline, ожидания от stable и beta, и чем install channels отличаются от runtime-контрактов.</p>
  </a>
  <a class="docs-card" href="./faq">
    <h2>Частые вопросы</h2>
    <p>Быстро разберите частые вопросы про wrappers, выбор runtime и strict validation.</p>
  </a>
  <a class="docs-card" href="./troubleshooting">
    <h2>Диагностика проблем</h2>
    <p>Разберите самые частые проблемы с установкой, runtime, render и validation.</p>
  </a>
  <a class="docs-card" href="./glossary">
    <h2>Словарь терминов</h2>
    <p>Нормализуйте ключевые термины, чтобы target, исходное состояние проекта, wrapper и handoff значили одно и то же по всему сайту.</p>
  </a>
  <a class="docs-card" href="./get-help-and-contribute">
    <h2>Как получить помощь и внести вклад</h2>
    <p>Найдите публичный путь для issues, pull requests, security-reporting и аккуратных community contribution.</p>
  </a>
</div>

## Чем этот раздел не является

- Это не лучшее место, чтобы впервые выбирать starter, target или runtime.
- Он не заменяет guide-страницы, когда сначала нужен общий product story.
- Это раздел для подтверждения точных правил, когда вы уже понимаете, какую задачу решаете.
