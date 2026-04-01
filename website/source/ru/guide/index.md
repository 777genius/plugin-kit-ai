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
- Кажется, что repo уже пошёл не туда: прочитайте [Антипаттерны выбора](/ru/guide/decision-anti-patterns), [Выбор стартового репозитория](/ru/guide/choose-a-starter) и [Выбор target](/ru/guide/choose-a-target).
- Нужно безопасно вернуться с неверного пути: прочитайте [Восстановление пути](/ru/guide/path-recovery), [Плейбук обновлений и миграции](/ru/guide/upgrade-and-migration-playbook) и [Rollout на уровне команды](/ru/guide/team-scale-rollout).
- Нужно выбрать один repo как стандарт для остальных: прочитайте [Стратегию reference repo](/ru/guide/reference-repo-strategy), [Внедрение в команду](/ru/guide/team-adoption) и [Стандарт репозитория](/ru/reference/repository-standard).
- Нужно понять, не уплыл ли уже baseline команды: прочитайте [Сигналы drift baseline](/ru/guide/baseline-drift-signals), [Стратегию reference repo](/ru/guide/reference-repo-strategy) и [Стандарт репозитория](/ru/reference/repository-standard).
- Нужно решить, является ли special-case repo здоровым исключением или нет: прочитайте [Политику здоровых исключений](/ru/guide/healthy-exception-policy), [Сигналы drift baseline](/ru/guide/baseline-drift-signals) и [Восстановление пути](/ru/guide/path-recovery).
- Внедрение в команду: прочитайте [Внедрение в команду](/ru/guide/team-adoption), [Готовность к продакшену](/ru/guide/production-readiness) и [Интеграцию с CI](/ru/guide/ci-integration).
- Обновления и миграции на уровне команды: прочитайте [Rollout на уровне команды](/ru/guide/team-scale-rollout), [Плейбук обновлений и миграции](/ru/guide/upgrade-and-migration-playbook), [Релизы](/ru/releases/) и [Миграцию существующей конфигурации](/ru/guide/migrate-existing-config).
- Поставка Python или Node: прочитайте [Выбор модели поставки](/ru/guide/choose-delivery-model) и [Bundle handoff](/ru/guide/bundle-handoff).

## Выбор по роли

- Новый автор плагина: идите в [Быстрый старт](/ru/guide/quickstart), [Соберите первый плагин](/ru/guide/first-plugin) и [Примеры и рецепты](/ru/guide/examples-and-recipes).
- Тимлид или maintainer: идите в [Внедрение в команду](/ru/guide/team-adoption), [Готовность к продакшену](/ru/guide/production-readiness) и [Интеграцию с CI](/ru/guide/ci-integration).
- Владелец repo, который планирует координированный rollout: идите в [Rollout на уровне команды](/ru/guide/team-scale-rollout), [Плейбук обновлений и миграции](/ru/guide/upgrade-and-migration-playbook) и [Политику версий и совместимости](/ru/reference/version-and-compatibility).
- Владелец repo, который выбирает эталонный repo для команды: идите в [Стратегию reference repo](/ru/guide/reference-repo-strategy), [Стандарт репозитория](/ru/reference/repository-standard) и [Восстановление пути](/ru/guide/path-recovery).
- Владелец repo, который проверяет, не начал ли стандарт уже плыть: идите в [Сигналы drift baseline](/ru/guide/baseline-drift-signals), [Стратегию reference repo](/ru/guide/reference-repo-strategy) и [Rollout на уровне команды](/ru/guide/team-scale-rollout).
- Владелец repo, который решает, оправдано ли одно особое исключение: идите в [Политику здоровых исключений](/ru/guide/healthy-exception-policy), [Границу поддержки](/ru/reference/support-boundary) и [Восстановление пути](/ru/guide/path-recovery).
- Владелец repo, который планирует обновления: идите в [Плейбук обновлений и миграции](/ru/guide/upgrade-and-migration-playbook), [Релизы](/ru/releases/) и [Миграцию существующей конфигурации](/ru/guide/migrate-existing-config).
- Ответственный за Python или Node путь: идите в [Выбор модели поставки](/ru/guide/choose-delivery-model), [Bundle handoff](/ru/guide/bundle-handoff) и [Node/TypeScript runtime](/ru/guide/node-typescript-runtime).
- Ответственный за packaging или workspace-config: идите в [Выбор target](/ru/guide/choose-a-target), [Package и workspace targets](/ru/guide/package-and-workspace-targets) и [Поддержку target’ов](/ru/reference/target-support).

## Выбор по ближайшей задаче

- Нужно быстро получить первый рабочий плагин: [Быстрый старт](/ru/guide/quickstart)
- Сначала нужно выбрать starter или target: [Выбор стартового репозитория](/ru/guide/choose-a-starter) и [Выбор target](/ru/guide/choose-a-target)
- Нужно понять, не выбран ли уже неправильный путь: [Антипаттерны выбора](/ru/guide/decision-anti-patterns)
- Нужно исправить неверный выбор и не разнести его дальше: [Восстановление пути](/ru/guide/path-recovery)
- Нужно выбрать один repo как чистый baseline для команды: [Стратегия reference repo](/ru/guide/reference-repo-strategy)
- Нужно проверить, не расходится ли уже baseline команды: [Сигналы drift baseline](/ru/guide/baseline-drift-signals)
- Нужно понять, является ли один repo здоровым исключением или уже drift: [Политика здоровых исключений](/ru/guide/healthy-exception-policy)
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
  <a class="docs-card" href="./decision-anti-patterns">
    <h2>Антипаттерны выбора</h2>
    <p>Поймайте самые дорогие ошибки выбора раньше, чем starter, target, runtime или delivery model превратятся в командную привычку.</p>
  </a>
  <a class="docs-card" href="./path-recovery">
    <h2>Восстановление пути</h2>
    <p>Безопасно вернитесь с неверного пути, если repo ещё работает, но уже не подходит для следующего этапа проекта.</p>
  </a>
  <a class="docs-card" href="./reference-repo-strategy">
    <h2>Стратегия reference repo</h2>
    <p>Выберите один repo, который будет учить правильному стандарту, до того как templates, rollout plan или командная привычка закрепят неверный baseline.</p>
  </a>
  <a class="docs-card" href="./baseline-drift-signals">
    <h2>Сигналы drift baseline</h2>
    <p>Поймайте момент, когда repo ещё выглядит здоровым, но объявленный стандарт и реальный baseline команды уже начинают расходиться.</p>
  </a>
  <a class="docs-card" href="./healthy-exception-policy">
    <h2>Политика здоровых исключений</h2>
    <p>Решите, когда special-case repo ещё оправдан и узок, а когда он уже стал нездоровым drift под более мягким названием.</p>
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
  <a class="docs-card" href="./team-scale-rollout">
    <h2>Rollout на уровне команды</h2>
    <p>Раскатывайте новые defaults, release guidance и support decisions сразу на несколько repo без drift и устных договорённостей.</p>
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
