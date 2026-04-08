---
title: "Граница поддержки"
description: "Самый короткий практический ответ о том, что plugin-kit-ai рекомендует, поддерживает осторожно и оставляет experimental."
canonicalId: "page:reference:support-boundary"
section: "reference"
locale: "ru"
generated: false
translationRequired: true
---

# Граница поддержки

Эта страница отвечает на один практический вопрос: что можно рекомендовать уже сейчас, что считать advanced, а что остаётся experimental?

## Безопасные defaults

- Go - рекомендуемый runtime lane по умолчанию.
- `validate --strict` - главная проверка готовности для локальных Python и Node runtime repos.
- `Codex runtime Go`, `Codex package`, `Gemini packaging`, `Gemini Go runtime` и Claude default stable lane - главные Recommended production lanes.
- `Python` и `Node` - поддерживаемые non-Go lanes и рекомендуемый non-Go выбор, когда компромисс локального interpreted runtime выбран осознанно.

## Как это маппится на формальный контракт

- `Recommended` обычно означает самые сильные текущие production lanes внутри `public-stable`.
- `Advanced` означает поддерживаемую surface с более узким, специализированным или осторожным контрактом.
- `Experimental` означает opt-in churn вне нормального compatibility expectation.

Используйте формальные термины `public-stable`, `public-beta` и `public-experimental`, когда вы задаёте policy для команды или обещаете совместимость downstream-пользователям.

## Что это означает сегодня

- Claude рекомендуется на default stable hook lane.
- Codex рекомендуется и для `Notify` runtime lane, и для официального `codex-package` lane.
- Gemini packaging рекомендуется, и promoted Gemini Go runtime тоже production-ready.
- OpenCode и Cursor - это repo-managed integration lanes, а не слабые targets и не стандартная runtime starting point.

## Advanced surfaces

Используйте их только тогда, когда компромисс осознан и явно нужен:

- OpenCode и Cursor, когда repo должен владеть integration config, а не runtime lane
- более узкие или специализированные runtime expansions вне основных рекомендуемых lanes
- install wrappers, если вас на самом деле интересует доставка CLI, а не runtime APIs или SDKs
- специальные config surfaces, которые полезны, но не являются первым default для большинства команд

## Experimental surfaces

Относитесь к experimental областям как к opt-in и high-churn. Они полезны ранним пользователям, но не должны молча становиться долгосрочной team policy.
