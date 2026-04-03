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
    Соберите плагин в одном репозитории, а потом рендерите поддерживаемые выходы для Claude,
    Codex, Gemini и других target'ов из того же процесса вместо ручной поддержки отдельных конфигураций.
  </p>
</div>

## Один репозиторий, много поддерживаемых выходов

- Начинайте с одного репозитория плагина, а не с отдельного репозитория под каждую экосистему.
- Добавляйте поддерживаемые выходы для Claude, Codex, Gemini и других target’ов по мере роста продукта.
- Держите один процесс через `render`, `validate` и CI.
- Не превращайте конфигурацию в набор одноразовых шаблонов и хрупких скриптов.

## Что важно понять сразу

- репозиторий и процесс остаются едиными
- глубина поддержки зависит от target’а
- runtime plugins, package outputs и workspace-config targets не дают одинаковые гарантии
- честное обещание здесь: один репозиторий и много поддерживаемых выходов, а не фальшивая parity везде

<div class="docs-grid">
  <a class="docs-card" href="./guide/quickstart">
    <h2>Быстрый старт</h2>
    <p>Стартуйте с одного сильного пути, а расширение на другие target’ы отложите на второй шаг.</p>
  </a>
  <a class="docs-card" href="./guide/what-you-can-build">
    <h2>Что можно построить</h2>
    <p>Посмотрите, как один repo может покрывать Claude, Codex, Gemini, bundles и config outputs.</p>
  </a>
  <a class="docs-card" href="./guide/choose-a-starter">
    <h2>Выбор стартового репозитория</h2>
    <p>Выберите starter как entrypoint, а не как окончательную границу продукта.</p>
  </a>
  <a class="docs-card" href="./reference/support-boundary">
    <h2>Граница поддержки</h2>
    <p>Проверьте, где поддержка самая сильная, а где глубина зависит от target’а.</p>
  </a>
</div>

## С чего лучше начинать

- Прочитайте [Модель управляемого проекта](/ru/concepts/managed-project-model), если вам нужно самое короткое объяснение того, чем вообще является этот продукт.
- Начинайте с `go`, когда нужен самый сильный путь для продакшена и минимум лишних зависимостей.
- Выбирайте `node --typescript`, когда команде нужен поддерживаемый путь на JavaScript или TypeScript внутри репозитория.
- Выбирайте `python`, когда репозиторий осознанно Python-first.
- Воспринимайте npm и PyPI пакеты `plugin-kit-ai` как способы установить CLI, а не как runtime-библиотеки.
- Используйте `validate --strict` как финальную проверку перед тем, как передавать репозиторий другому человеку или машине.

## Найдите свой сценарий

- Новый автор плагина: начните с [Установки](/ru/guide/installation), [Быстрого старта](/ru/guide/quickstart) и [Первого плагина](/ru/guide/first-plugin).
- Тимлид или maintainer: начните с [Плагина для команды](/ru/guide/team-ready-plugin), [Готовности к продакшену](/ru/guide/production-readiness) и [Интеграции с CI](/ru/guide/ci-integration).
- Команда на Python или Node: начните с [Выбора модели поставки](/ru/guide/choose-delivery-model), [Bundle handoff](/ru/guide/bundle-handoff) и [v1.0.6](/ru/releases/v1-0-6).
- Packaging или workspace config: начните с [Выбора target](/ru/guide/choose-a-target), [Package и workspace targets](/ru/guide/package-and-workspace-targets) и [Поддержки target’ов](/ru/reference/target-support).

## Кому этот сайт особенно полезен

- Отдельным авторам плагинов, которым нужен надёжный первый старт.
- Командам, которым нужен репозиторий, который другой человек сможет проверить и выпустить.
- Python и Node командам, которым нужна поддерживаемая история поставки, а не только локальный scaffold.
- Интеграторам, которым нужен точный публичный API, поддержка target’ов и граница релизных изменений.

## Читайте в таком порядке

<div class="docs-grid">
  <a class="docs-card" href="./guide/quickstart">
    <h2>1. Быстрый старт</h2>
    <p>Стартуйте с одного сильного пути до того, как начнёте думать о расширении.</p>
  </a>
  <a class="docs-card" href="./guide/what-you-can-build">
    <h2>2. Что можно построить</h2>
    <p>Посмотрите, как тот же репозиторий позже покрывает больше поддерживаемых выходов.</p>
  </a>
  <a class="docs-card" href="./guide/choose-a-starter">
    <h2>3. Выбор стартового репозитория</h2>
    <p>Выберите starter как точку входа, а не как окончательную границу продукта.</p>
  </a>
  <a class="docs-card" href="./reference/support-boundary">
    <h2>4. Граница поддержки</h2>
    <p>Посмотрите, что stable, что beta и что проект сознательно пока не обещает.</p>
  </a>
</div>

Если вы новый пользователь, после этих четырёх страниц можно не идти глубже сразу.

## Последний стабильный релиз

- Текущая публичная опорная версия в этом наборе docs — [`v1.0.6`](/ru/releases/v1-0-6).
- Именно этот релиз сделал shared runtime-package delivery для Python и Node полноценным поддерживаемым путём, а не частичной историей.
- Начинайте с него, если вам важны актуальные пользовательские заметки об изменениях.

## Что с этим можно сделать

- Делайте плагины для Codex runtime и Claude hooks из одной управляемой модели проекта.
- Используйте Go для самого сильного продакшен-пути или Python и Node для поддерживаемых локальных runtime-проектов.
- Отдавайте portable Python и Node bundles, когда нужны скачиваемые артефакты вместо живого репозитория.
- Переиспользуйте helper-логику через `plugin-kit-ai-runtime`, когда общий runtime package лучше подходит, чем копирование файлов в каждый репозиторий.
- Работайте с runtime, package, extension и workspace-config target’ами при явной и понятной границе поддержки.

## Что покрывает сайт

- Публичные гайды для пользователей и авторов плагинов.
- Сгенерированный API reference из реального кода и дерева команд.
- Публичные support и platform metadata.
- Пользовательские release notes и заметки об изменениях.
- Публичные policy-страницы про versioning, совместимость и ожидания по поддержке.

## Что сознательно вынесено

- Материалы внутренних release rehearsal.
- Maintainer-only audit notes и operational checklists.
- Внутренности wrapper packages, замаскированные под API.
