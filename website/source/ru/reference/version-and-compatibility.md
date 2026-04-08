---
title: "Политика версий и совместимости"
description: "Как мыслить про релизы, compatibility promises, wrappers, SDK, и vocabulary поддержки в plugin-kit-ai."
canonicalId: "page:reference:version-and-compatibility"
section: "reference"
locale: "ru"
generated: false
translationRequired: true
---

# Политика версий и совместимости

Эта страница даёт короткий публичный ответ на командный вопрос:

- какие версии задают текущий baseline и какие promises совместимости реально достаточно сильны, чтобы сделать их стандартом

## Выбор за 60 секунд

- читайте эту страницу, когда команде нужен один компактный policy doc про релизы, wrappers, SDK, runtimes и promises совместимости
- читайте [Границу поддержки](/ru/reference/support-boundary), когда нужен самый короткий практический ответ про поддержку
- читайте [Релизы](/ru/releases/), когда нужна история конкретного релиза

## Публичный baseline

Полезно думать о версиях в трёх слоях:

- release line, которую вы стандартизируете между repo
- support level lane внутри этой release line
- install или delivery mechanism вокруг этого lane

Эти слои связаны, но это не одно и то же.

## Что именно покрывает совместимость

Самое сильное публичное обещание относится к:

- заявленному публичному CLI contract
- рекомендуемому пути через Go SDK
- рекомендуемым локальным Python и Node runtime lanes на поддерживаемых target'ах
- задокументированному поведению `public-stable` generated outputs

Совместимость не означает, что каждый wrapper, convenience path или специализированная surface движутся с одинаковой силой обещаний.

## Публичный язык и формальные термины

Используйте простую трансляцию:

- `Recommended` обычно означает lane внутри самого сильного текущего `public-stable` контракта
- `Advanced` означает поддерживаемую surface, которая уже, осторожнее или специализированнее первого default
- `Experimental` означает opt-in churn без нормального compatibility expectation

Когда команде нужен точный policy layer, используйте формальные термины `public-stable`, `public-beta` и `public-experimental`.

## Wrappers, SDK и runtime APIs

Не смешивайте эти категории.

- Homebrew, npm, PyPI и verified script - это install channels для CLI
- Go SDK - это публичная SDK surface
- runtime APIs привязаны к своим объявленным runtime lanes

## Что командам стоит стандартизировать

Здоровые команды обычно стандартизируют:

- одну явную release baseline
- один основной lane с понятной support story
- один validation gate перед handoff и rollout
- одно общее понимание формальных compatibility terms

## Финальное правило

Стандартизируйте только ту release line и тот lane, чьё публичное обещание ваша команда действительно готова защищать через CI, handoff и rollout.
