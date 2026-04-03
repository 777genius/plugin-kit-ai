---
title: "Диагностика проблем"
description: "Самые частые проблемы при установке, render, validate и bootstrap в plugin-kit-ai проектах."
canonicalId: "page:reference:troubleshooting"
section: "reference"
locale: "ru"
generated: false
translationRequired: true
---

# Диагностика проблем

## CLI установился, но не запускается

Проверьте, что binary действительно находится в shell `PATH`. Если вы ставили CLI через npm или PyPI, убедитесь, что пакет реально скачал опубликованный binary, а не воспринимайте сам пакет как runtime.

## Python или Node runtime-проекты падают слишком рано

Сначала проверьте сам runtime:

- Python runtime-проекты требуют Python `3.10+`
- Node runtime-проекты требуют Node.js `20+`

Используйте `plugin-kit-ai doctor <path>`, прежде чем считать, что сломан сам проект.

## Падает `validate --strict`

Воспринимайте это как сигнал, а не как шум. Смысл strict validation именно в том, чтобы ловить drift и readiness problems до того, как вы объявите проект здоровым.

Частые причины:

- generated artifacts устарели, потому что был пропущен `render`
- выбранная platform не соответствует исходному состоянию проекта
- выбранный runtime-путь требует bootstrap или исправления окружения

## `render` выдаёт не то, что ожидалось

Обычно это значит, что исходное состояние проекта и ваша ментальная модель уже разошлись. Проверьте package-standard layout, а не редактируйте generated target files вручную в попытке “починить” output.

## Я не понимаю, какой путь выбрать

Начинайте с пути Go по умолчанию, если нужен самый сильный контракт. Переходите на Node/TypeScript или Python только тогда, когда компромисс локального runtime действительно осознан и нужен.

См. [Процесс авторинга](/ru/reference/authoring-workflow) и [Частые вопросы](/ru/reference/faq).
