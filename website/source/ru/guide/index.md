---
title: "Гайды"
description: "Стартовый раздел публичной документации plugin-kit-ai."
canonicalId: "page:guide:index"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
aside: false
outline: false
---

<div class="docs-hero docs-hero--compact">
  <p class="docs-kicker">GUIDE</p>
  <h1>Начните здесь</h1>
  <p class="docs-lead">
    Используйте этот раздел, когда вам нужен кратчайший путь к корректной настройке, а не глубокий тур по внутренностям проекта.
  </p>
</div>

## Если запомнить только одну мысль

Начинайте со starter’а или target’а под первое реальное требование, но дальше мыслите проект как один managed repo, который со временем может рендерить больше одной выходной формы.

Если проект всё ещё кажется размытым, сначала прочитайте [Managed Project Model](/ru/concepts/managed-project-model).

## Как читать этот раздел

<div class="docs-flow" aria-label="Как читать раздел Guide">
  <div class="docs-flow__step">
    <strong>Сначала поймите продукт</strong>
    <span>Прочитайте <a href="./what-you-can-build">Что можно построить</a> и <a href="./one-project-multiple-targets">Один проект, несколько target’ов</a>.</span>
  </div>
  <div class="docs-flow__arrow" aria-hidden="true">→</div>
  <div class="docs-flow__step">
    <strong>Потом выберите первый путь</strong>
    <span>Определите target, runtime или starter под первое реальное требование, а не под все возможные будущие сценарии сразу.</span>
  </div>
  <div class="docs-flow__arrow" aria-hidden="true">→</div>
  <div class="docs-flow__step">
    <strong>Соберите и проверьте</strong>
    <span>Идите по самому узкому поддерживаемому tutorial path, затем докажите корректность через <code>validate --strict</code>.</span>
  </div>
  <div class="docs-flow__arrow" aria-hidden="true">→</div>
  <div class="docs-flow__step">
    <strong>Расширяйте только по необходимости</strong>
    <span>Добавляйте delivery flow, новые target’ы и CI только после того, как core managed project уже здоров.</span>
  </div>
</div>

## Типовые маршруты чтения

- Первый вход: прочитайте [Установку](/ru/guide/installation), потом [Быстрый старт](/ru/guide/quickstart), потом [Соберите первый плагин](/ru/guide/first-plugin).
- Выбор пути: прочитайте [Что можно построить](/ru/guide/what-you-can-build), [Один проект, несколько target’ов](/ru/guide/one-project-multiple-targets), [Выбор runtime](/ru/concepts/choosing-runtime) и [Package и workspace targets](/ru/guide/package-and-workspace-targets).
- Внедрение в команду: прочитайте [Внедрение в команду](/ru/guide/team-adoption), [Готовность к продакшену](/ru/guide/production-readiness) и [Интеграцию с CI](/ru/guide/ci-integration).
- Обновления и миграции на уровне команды: прочитайте [Плейбук обновлений и миграции](/ru/guide/upgrade-and-migration-playbook), [Релизы](/ru/releases/) и [Миграцию существующей конфигурации](/ru/guide/migrate-existing-config).
- Поставка Python или Node: прочитайте [Выбор модели поставки](/ru/guide/choose-delivery-model) и [Bundle handoff](/ru/guide/bundle-handoff).

## Выбор по роли

- Новый автор плагина: идите в [Быстрый старт](/ru/guide/quickstart), [Соберите первый плагин](/ru/guide/first-plugin) и [Примеры и рецепты](/ru/guide/examples-and-recipes).
- Тимлид или maintainer: идите в [Внедрение в команду](/ru/guide/team-adoption), [Готовность к продакшену](/ru/guide/production-readiness) и [Интеграцию с CI](/ru/guide/ci-integration).
- Владелец repo, который планирует обновления: идите в [Плейбук обновлений и миграции](/ru/guide/upgrade-and-migration-playbook), [Релизы](/ru/releases/) и [Миграцию существующей конфигурации](/ru/guide/migrate-existing-config).
- Ответственный за Python или Node путь: идите в [Выбор модели поставки](/ru/guide/choose-delivery-model), [Bundle handoff](/ru/guide/bundle-handoff) и [Node/TypeScript runtime](/ru/guide/node-typescript-runtime).
- Ответственный за packaging или workspace-config: идите в [Выбор target](/ru/guide/choose-a-target), [Package и workspace targets](/ru/guide/package-and-workspace-targets) и [Поддержку target’ов](/ru/reference/target-support).

## Выбор по ближайшей задаче

- Нужно быстро получить первый рабочий плагин: [Быстрый старт](/ru/guide/quickstart)
- Сначала нужно выбрать starter или target: [Выбор стартового репозитория](/ru/guide/choose-a-starter) и [Выбор target](/ru/guide/choose-a-target)
- Нужен живой пример до выбора: [Примеры и рецепты](/ru/guide/examples-and-recipes)
- Нужен безопасный production path: [Готовность к продакшену](/ru/guide/production-readiness)

<div class="docs-grid">
  <a class="docs-card" href="./quickstart">
    <h2>Быстрый старт</h2>
    <p>Используйте самый короткий поддерживаемый путь от установки до рабочего репозитория плагина.</p>
  </a>
  <a class="docs-card" href="./installation">
    <h2>Установка</h2>
    <p>Выберите правильный канал установки и сразу поймите, где публичный API, а где только способ доставки CLI.</p>
  </a>
  <a class="docs-card" href="./what-you-can-build">
    <h2>Что можно построить</h2>
    <p>Посмотрите реальные формы продукта: плагины Codex и Claude, bundle handoff, shared runtime package и цели для упаковки и конфигурации.</p>
  </a>
  <a class="docs-card" href="./one-project-multiple-targets">
    <h2>Один проект, несколько target’ов</h2>
    <p>Поймите ключевую идею продукта: starter — это вход, а managed project model может покрывать больше одной выходной формы.</p>
  </a>
  <a class="docs-card" href="./choose-a-target">
    <h2>Выбор target</h2>
    <p>Разберитесь между Codex runtime, Claude, Codex package, Gemini, OpenCode и Cursor без необходимости собирать картину из нескольких страниц.</p>
  </a>
  <a class="docs-card" href="./first-plugin">
    <h2>Соберите первый плагин</h2>
    <p>Пройдите самый короткий поддерживаемый путь от scaffold до `validate --strict`.</p>
  </a>
  <a class="docs-card" href="./team-adoption">
    <h2>Внедрение в команду</h2>
    <p>Используйте публичный путь для rollout plugin-kit-ai в команде без скрытых устных договорённостей.</p>
  </a>
  <a class="docs-card" href="./upgrade-and-migration-playbook">
    <h2>Плейбук обновлений и миграции</h2>
    <p>Используйте безопасный публичный путь для принятия новых defaults, релизов и managed project model в существующих repo.</p>
  </a>
  <a class="docs-card" href="./team-ready-plugin">
    <h2>Сделайте плагин готовым для команды</h2>
    <p>Выйдите за пределы первого зелёного прогона и подготовьте репозиторий к CI, передаче другим людям и командному использованию.</p>
  </a>
  <a class="docs-card" href="./claude-plugin">
    <h2>Соберите плагин для Claude</h2>
    <p>Используйте стабильный путь Claude, когда вам нужны именно hooks Claude, а не основной путь Codex runtime.</p>
  </a>
  <a class="docs-card" href="./node-typescript-runtime">
    <h2>Node/TypeScript runtime</h2>
    <p>Выберите основной стабильный путь без Go для локальных runtime-плагинов.</p>
  </a>
  <a class="docs-card" href="./starter-templates">
    <h2>Стартовые шаблоны</h2>
    <p>Берите официальный starter, когда нужен проверенный layout для Claude или Codex.</p>
  </a>
  <a class="docs-card" href="./examples-and-recipes">
    <h2>Примеры и рецепты</h2>
    <p>Смотрите реальные plugin repos, starter repos, локальные runtime references и skill examples, не копаясь по дереву репозитория.</p>
  </a>
  <a class="docs-card" href="./choose-a-starter">
    <h2>Выбор стартового репозитория</h2>
    <p>Используйте практическую матрицу, чтобы выбрать стартовый шаблон по платформе, runtime и модели передачи артефактов.</p>
  </a>
  <a class="docs-card" href="./choose-delivery-model">
    <h2>Выбор модели поставки</h2>
    <p>Выберите между локальными helper-файлами и общим runtime package для Python и Node.</p>
  </a>
  <a class="docs-card" href="./bundle-handoff">
    <h2>Bundle handoff</h2>
    <p>Используйте export, локальную установку, удалённую загрузку и GitHub Releases publish, когда Python или Node плагин нужно передавать как готовый артефакт.</p>
  </a>
  <a class="docs-card" href="./package-and-workspace-targets">
    <h2>Package и workspace targets</h2>
    <p>Разберитесь с Codex package, Gemini, OpenCode и Cursor до того, как начнёте ожидать от них поведения runtime-плагинов.</p>
  </a>
  <a class="docs-card" href="./migrate-existing-config">
    <h2>Миграция существующей конфигурации</h2>
    <p>Переведите вручную поддерживаемые native target files в package-standard authored model.</p>
  </a>
  <a class="docs-card" href="./production-readiness">
    <h2>Готовность к продакшену</h2>
    <p>Используйте публичный checklist перед тем, как называть репозиторий стабильным, готовым к передаче другим людям или зрелым для CI.</p>
  </a>
  <a class="docs-card" href="./ci-integration">
    <h2>Интеграция с CI</h2>
    <p>Превратите публичный authored flow в предсказуемый CI gate, который ловит drift до handoff.</p>
  </a>
</div>
