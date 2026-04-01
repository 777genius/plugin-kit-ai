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
    Ведите один managed plugin project, рендерите выходы под нужные agent'ы и target'ы,
    и не превращайте репозиторий в набор одноразовых шаблонов и хрупких glue scripts.
  </p>
</div>

## В одном предложении

`plugin-kit-ai` — это управляемая система для plugin-проектов: вы ведёте один репозиторий, рендерите нужные выходы под конкретные target’ы, проверяете результат и передаёте дальше то, чему может доверять другой человек или CI.

Если нужна одна страница, которая объясняет продукт максимально ясно, прочитайте [Managed Project Model](/ru/concepts/managed-project-model).

## Три слоя продукта

<div class="docs-grid docs-grid--layers">
  <div class="docs-card docs-card--static">
    <h2>1. Модель проекта</h2>
    <p>Один authored repo остаётся source of truth. Это и есть настоящее ядро продукта.</p>
  </div>
  <div class="docs-card docs-card--static">
    <h2>2. Workflow-поверхность</h2>
    <p>`init`, `render`, `validate`, CI и generated API открывают воспроизводимый workflow вокруг этой модели.</p>
  </div>
  <div class="docs-card docs-card--static">
    <h2>3. Выходные формы</h2>
    <p>Runtime, package, extension и workspace-config target’ы — это выходы, которые способен производить managed project.</p>
  </div>
</div>

## Короткая перенастройка восприятия

- Starter’ы задают первый правильный путь, а не долгосрочную границу repo.
- Разные target’ы — это разные выходные формы с разной support boundary, а не одинаковые обещания.
- CLI и generated API открывают workflow, но сам продукт — это управляемая модель репозитория.

## Без и с plugin-kit-ai

<div class="docs-grid docs-grid--layers">
  <div class="docs-card docs-card--static">
    <h2>Без plugin-kit-ai</h2>
    <p>Команды правят target-файлы вручную, дублируют helper-код, объясняют workflow в чатах и постепенно накапливают drift внутри repo.</p>
  </div>
  <div class="docs-card docs-card--static">
    <h2>С plugin-kit-ai</h2>
    <p>Команды держат один authored repo, осознанно рендерят нужные выходы, проверяют результат и передают дальше то, что воспроизводимо для CI и других людей.</p>
  </div>
</div>

## Карта системы

<div class="docs-flow" aria-label="Схема системы plugin-kit-ai">
  <div class="docs-flow__step">
    <strong>Начните с реальной задачи</strong>
    <span>Возьмите starter или мигрируйте существующий repo, когда уже понятно первое требование по target’у или runtime.</span>
  </div>
  <div class="docs-flow__arrow" aria-hidden="true">→</div>
  <div class="docs-flow__step">
    <strong>Держите один managed project</strong>
    <span>Считайте package-standard project главным authored source of truth, а не поддерживайте target-файлы вручную.</span>
  </div>
  <div class="docs-flow__arrow" aria-hidden="true">→</div>
  <div class="docs-flow__step">
    <strong>Рендерите и проверяйте</strong>
    <span>Генерируйте только нужные выходы, а затем строгой проверкой доказывайте, что они согласованы с проектом.</span>
  </div>
  <div class="docs-flow__arrow" aria-hidden="true">→</div>
  <div class="docs-flow__step">
    <strong>Расширяйте осознанно</strong>
    <span>Добавляйте runtime, package, extension и workspace-config выходы, не превращая repo в набор ad-hoc glue.</span>
  </div>
</div>

<div class="docs-grid">
  <a class="docs-card" href="./concepts/managed-project-model">
    <h2>Понять ядро модели</h2>
    <p>Прочитайте самую короткую и точную формулировку продукта до выбора runtime, starter’а или target’а.</p>
  </a>
  <a class="docs-card" href="./guide/">
    <h2>Быстрый старт</h2>
    <p>Поставьте CLI, поймите основные поддерживаемые пути и быстро дойдите до первого рабочего плагина.</p>
  </a>
  <a class="docs-card" href="./reference/">
    <h2>Справочник</h2>
    <p>Используйте публичный справочник для каналов установки, поддержки target’ов и стабильных правил, на которые можно опираться.</p>
  </a>
  <a class="docs-card" href="./api/">
    <h2>Сгенерированный API</h2>
    <p>Просматривайте живой reference для CLI, Go SDK, Node runtime, Python runtime, событий платформ и capabilities.</p>
  </a>
  <a class="docs-card" href="./releases/">
    <h2>Релизы</h2>
    <p>Следите за пользовательскими изменениями, миграциями и границей breaking changes по мере развития проекта.</p>
  </a>
</div>

## Что здесь считается «plugin»

- runtime plugin, когда repo владеет исполняемым поведением
- package или extension output, когда repo рендерит устанавливаемые артефакты
- workspace-config output, когда repo владеет интеграцией или конфигурацией редактора

Модель продукта остаётся той же самой, даже когда меняется форма выхода.

## С чего начать по сценарию

- Новый автор плагина: начните с [Установки](/ru/guide/installation), [Быстрого старта](/ru/guide/quickstart) и [Первого плагина](/ru/guide/first-plugin).
- Тимлид или maintainer: начните с [Плагина для команды](/ru/guide/team-ready-plugin), [Готовности к продакшену](/ru/guide/production-readiness) и [Интеграции с CI](/ru/guide/ci-integration).
- Команда на Python или Node: начните с [Выбора модели поставки](/ru/guide/choose-delivery-model), [Bundle handoff](/ru/guide/bundle-handoff) и [v1.0.6](/ru/releases/v1-0-6).
- Packaging или workspace config: начните с [Выбора target](/ru/guide/choose-a-target), [Package и workspace targets](/ru/guide/package-and-workspace-targets) и [Поддержки target’ов](/ru/reference/target-support).

## Выберите свой путь

<div class="docs-grid">
  <a class="docs-card" href="./guide/first-plugin">
    <h2>Первый production plugin</h2>
    <p>Пройдите самый короткий рекомендуемый путь от создания проекта до строгой проверки готовности.</p>
  </a>
  <a class="docs-card" href="./guide/what-you-can-build">
    <h2>Что реально можно построить</h2>
    <p>Поймите, что именно можно сделать с plugin-kit-ai, прежде чем выбирать путь, шаблон или target.</p>
  </a>
  <a class="docs-card" href="./concepts/why-plugin-kit-ai">
    <h2>Зачем это вообще нужно</h2>
    <p>Поймите, какую проблему решает проект, кому он подходит и какие компромиссы заложены сознательно.</p>
  </a>
  <a class="docs-card" href="./reference/support-boundary">
    <h2>Понять границу</h2>
    <p>Посмотрите, что stable, что beta и что проект сознательно пока не обещает.</p>
  </a>
</div>

## Текущая публичная опорная версия

- Текущая публичная опорная версия в этом наборе docs — [`v1.0.6`](/ru/releases/v1-0-6).
- Именно этот релиз сделал shared runtime-package delivery для Python и Node полноценным поддерживаемым путём, а не частичной историей.
- Начинайте с него, если вам важны актуальные пользовательские migration notes.

## Что покрывает сайт

- Публичные гайды для пользователей и авторов плагинов.
- Сгенерированный API reference из реального кода и дерева команд.
- Публичные support и platform metadata.
- Пользовательские release notes и migration notes.

## Что сознательно вынесено

- Материалы внутренних release rehearsal.
- Maintainer-only audit notes и operational checklists.
- Внутренности wrapper packages, замаскированные под API.
