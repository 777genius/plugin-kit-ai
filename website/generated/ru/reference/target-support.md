---
title: "Поддержка target’ов"
description: "Сводка по поддержке target’ов"
canonicalId: "page:reference:target-support"
surface: "reference"
section: "reference"
locale: "ru"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "docs/generated/target_support_matrix.md"
translationRequired: false
---
# Поддержка target’ов

Используйте эту страницу, когда нужно быстро понять, какая цель готова к production-использованию, а какая остаётся только упаковочным или workspace-config вариантом.

| Цель | Класс production | Runtime-контракт | Модель установки |
| --- | --- | --- | --- |
| claude | готово для production | стабильный поднабор runtime | marketplace или локально |
| codex-package | package-вариант для production | только официальный пакет | marketplace или локально |
| codex-runtime | runtime-вариант для production | стабильный notify-runtime | локально в репозитории |
| gemini | runtime-supported beta extension target | упаковка, не runtime | установка копированием |
| cursor | только упаковка | workspace-config вариант | конфигурация workspace |
| opencode | только упаковка | workspace-config вариант | конфигурация workspace |

Для полной картины свяжите эту матрицу с [Границей поддержки](/ru/reference/support-boundary) и [Моделью target’ов](/ru/concepts/target-model).
