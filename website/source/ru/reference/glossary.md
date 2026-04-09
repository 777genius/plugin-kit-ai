---
title: "Словарь терминов"
description: "Короткие определения публичных терминов, которые встречаются в docs plugin-kit-ai."
canonicalId: "page:reference:glossary"
section: "reference"
locale: "ru"
generated: false
translationRequired: true
---

# Словарь терминов

Используйте эту страницу, когда какой-то термин тормозит чтение docs. Цель здесь не идеальная теория, а быстрое общее понимание.

## Authored State

Часть repo, которой команда владеет напрямую. `generate` превращает этот source в target-specific output.

## Generated Target Files

Файлы, которые появляются для конкретного target после генерации. Это реальный delivery output, но не долгосрочный source of truth.

## Path

Практический способ собрать и поставлять plugin. Примеры: default Go runtime path, локальный Node/TypeScript path и repo-owned integration setup.

## Target

Output, в который вы целитесь, например `codex-runtime`, `claude`, `codex-package`, `gemini`, `opencode` или `cursor`.

## Runtime Path

Path, в котором repo напрямую владеет исполняемым поведением plugin.

## Package Or Extension Path

Path, сфокусированный на правильном package или extension artifact, а не на основной исполняемой runtime-форме.

## Repo-Owned Integration Setup

Path, где repo в основном поставляет checked-in configuration для другого tool или workspace.

## Install Channel

Способ установить CLI, например через Homebrew, npm, PyPI или verified script. Это не public runtime API.

## Shared Runtime Package

Зависимость `plugin-kit-ai-runtime`, которую используют одобренные Python и Node flows вместо копирования helper files в каждый repo.

## Support Boundary

Публичная граница между тем, что проект рекомендует по умолчанию, что поддерживает осторожнее и что оставляет experimental.

## Readiness Gate

Проверка, которую стоит считать сигналом, что repo уже достаточно здоров для handoff. Для большинства repo это `validate --strict`.

## Handoff

Момент, когда другой человек, другая машина или другой пользователь может использовать repo без скрытых шагов setup.

Связанные страницы: [Модель target'ов](/ru/concepts/target-model), [Граница поддержки](/ru/reference/support-boundary) и [Готовность к продакшену](/ru/guide/production-readiness).
