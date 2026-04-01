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

## Что это такое

- один authored project вместо россыпи вручную поддерживаемых target-файлов
- один управляемый workflow через `render`, `validate` и CI
- одно место, где видно, что stable, что beta и какие границы поддержки действуют для target’ов

## Чем это не является

- это не обещание одинаковой зрелости для каждого agent’а и каждого target’а
- это не универсальная runtime-библиотека для всех экосистем
- это не просто набор несвязанных starter-репозиториев, которые заставляют слишком рано дробить работу
- это не история, где starter’ы, wrapper’ы или команды важнее самой модели проекта

## Главная идея

- один authored project вместо россыпи вручную поддерживаемых target files
- один управляемый workflow через `render`, `validate` и CI
- несколько поддерживаемых выходных форм для runtime, package, extension и workspace-config target’ов
- честные границы поддержки вместо обещаний фальшивой parity

## Модель системы

1. Начните с самого узкого реального требования: обычно это starter или существующий репозиторий, который нужно мигрировать.
2. Держите package-standard project как главный authored source of truth.
3. Рендерите только те выходы и target-артефакты, которые действительно нужны репозиторию.
4. Проверяйте результат строгими проверками перед handoff.
5. Оставляйте тот же managed project по мере роста репозитория на новые target’ы, выходные формы и способы поставки.

## Почему первое впечатление может быть неверным

- названия starter’ов специально выглядят конкретно, потому что они оптимизируют первый правильный путь
- списки target’ов заметны, потому что система умеет рендерить больше одной выходной формы
- CLI находится на виду, потому что workflow должен быть воспроизводимым, а не потому что продукт — “просто CLI”

Сам проект — это managed repo model, которая стоит за всеми этими входными точками.

<div class="docs-grid">
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

## С чего лучше начинать

- Начинайте с `go`, когда нужен самый сильный путь для продакшена и минимум лишних зависимостей.
- Выбирайте `node --typescript`, когда команде нужен поддерживаемый путь на JavaScript или TypeScript внутри репозитория.
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

## Последний стабильный релиз

- Текущая публичная опорная версия в этом наборе docs — [`v1.0.6`](/ru/releases/v1-0-6).
- Именно этот релиз сделал shared runtime-package delivery для Python и Node полноценным поддерживаемым путём, а не частичной историей.
- Начинайте с него, если вам важны актуальные пользовательские migration notes.

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
- Пользовательские release notes и migration notes.

## Что сознательно вынесено

- Материалы внутренних release rehearsal.
- Maintainer-only audit notes и operational checklists.
- Внутренности wrapper packages, замаскированные под API.
