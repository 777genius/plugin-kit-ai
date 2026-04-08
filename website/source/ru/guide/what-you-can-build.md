---
title: "Что можно построить"
description: "Публичный обзор того, как один plugin repo вырастает в несколько delivery lanes."
canonicalId: "page:guide:what-you-can-build"
section: "guide"
locale: "ru"
generated: false
translationRequired: true
---

# Что можно построить

`plugin-kit-ai` строится вокруг простой идеи: держите один authored repo, начинайте с одного рекомендуемого lane и расширяйтесь позже только тогда, когда продукту действительно нужны новые outputs.

<MermaidDiagram
  :chart="`
flowchart TD
  Product[One authored repo] --> Runtime[Runtime lane]
  Product --> Package[Package lane]
  Product --> Extension[Extension lane]
  Product --> Bundle[Bundle handoff]
  Product --> Integration[Repo managed integration lane]
  Product --> Shared[Shared runtime package]
`"
/>

## Рекомендуемая стартовая форма

Большинству команд стоит начинать с одного из этих lanes:

- `Codex runtime Go`
- `Codex package`
- `Gemini packaging`
- `Claude default lane`

Если runtime stack уже определён заранее, можно стартовать и с:

- `Node/TypeScript`
- `Python`

## Расширяйтесь позже из того же repo

После того как первый lane здоров, тот же repo можно расширить до:

- Claude outputs, когда hooks становятся частью продукта
- Codex package outputs, когда важна package-доставка
- Gemini extension packaging, когда Gemini становится реальным delivery lane
- OpenCode и Cursor, когда repo должен владеть integration config
- portable bundle handoff для поддерживаемых Python и Node repos

## Один repo, много поддерживаемых outputs

Реальная форма продукта - это не "много случайных target'ов", а один authored repo, который со временем начинает выпускать несколько поддерживаемых outputs по мере расширения delivery model.

## Repo, готовый для команды

Смысл не только в scaffolding. Смысл в repo, который другой коллега может понять, проверить и выпустить.

Это означает:

- один source of truth под `src/`
- один validation workflow через `generate`, `validate` и CI
- явный выбор lanes вместо ручной правки native files
- предсказуемый handoff между авторами и downstream-пользователями

## Bundle и shared runtime paths

Для поддерживаемых Python и Node lanes repo может также выпускать:

- portable bundle artifacts для handoff
- shared helper delivery через `plugin-kit-ai-runtime`

Это choices поверх того же authored repo, а не отдельные продукты.

## Какие delivery models покрываются

`plugin-kit-ai` может покрывать:

- runtime lanes для исполняемого поведения плагина
- package lanes для официальных package artifacts
- extension lanes для extension-style delivery
- repo-managed integration lanes для config и workspace ownership

В этом и состоит реальная multi-target story: один repo, один workflow, несколько delivery lanes со временем.
