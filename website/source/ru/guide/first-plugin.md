---
title: "Соберите первый плагин"
description: "Минимальный пошаговый сценарий от init до строгой проверки готовности."
canonicalId: "page:guide:first-plugin"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Соберите первый плагин

Этот гайд использует самый сильный путь по умолчанию и специально держит сценарий узким:

- target: `codex-runtime`
- язык: `go`
- readiness gate: `validate --strict`

Узость этого tutorial нужна только для первого запуска. Если вам сразу важна более широкая история про один repo и несколько outputs, после него идите в [Один проект, несколько target’ов](/ru/guide/one-project-multiple-targets).

## 1. Установите CLI

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai version
```

## 2. Создайте проект

```bash
plugin-kit-ai init my-plugin
cd my-plugin
```

Путь `init` по умолчанию уже является рекомендуемой стартовой точкой для продакшена.

## 3. Сгенерируйте target-файлы

```bash
plugin-kit-ai generate .
```

Не редактируйте сгенерированные target-файлы вручную как главный источник истины. Держите исходное состояние проекта внутри обычного `plugin-kit-ai` workflow.

## 4. Прогоните проверку готовности

```bash
plugin-kit-ai validate . --platform codex-runtime --strict
```

Используйте это как главную проверку готовности для локального проекта.

## Что у вас теперь есть

- один plugin repo
- authored files под `src/`
- generated output для Codex runtime
- понятная проверка готовности через `validate --strict`

## 5. Когда менять путь

Переходите на другой путь только когда это действительно нужно:

- выбирайте `claude` для плагинов Claude
- выбирайте `--runtime node --typescript` для основного стабильного пути без Go
- выбирайте `--runtime python`, когда проект остаётся локальным для репозитория, а команда осознанно Python-first
- выбирайте `codex-package`, `gemini`, `opencode` или `cursor`, только если вам действительно нужен другой способ поставки

Это не означает, что репозиторий должен навсегда остаться single-target: начинайте с самого важного target'а сегодня и добавляйте остальные только по реальной необходимости.

## Следующие шаги

- Прочитайте [Выбор runtime](/ru/concepts/choosing-runtime), прежде чем уходить с пути по умолчанию.
- Прочитайте [Один проект, несколько target’ов](/ru/guide/one-project-multiple-targets), если для вас важна идея одного repo и нескольких outputs как основная идея продукта.
- Используйте [Стартовые шаблоны](/ru/guide/starter-templates), когда нужен проверенный пример репозитория.
- Откройте [Справочник CLI](/ru/api/cli/), когда нужно точное поведение команд.
