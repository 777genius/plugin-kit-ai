---
title: "plugin-kit-ai capabilities"
description: "Показывает сгенерированные metadata по целям, пакетам и поддержке runtime."
canonicalId: "command:plugin-kit-ai:capabilities"
surface: "cli"
section: "api"
locale: "ru"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai capabilities"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai capabilities" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai capabilities

Сгенерировано из реального Cobra command tree.

Показывает сгенерированные metadata по целям, пакетам и поддержке runtime.

## plugin-kit-ai capabilities

Показывает сгенерированные metadata по целям, пакетам и поддержке runtime.

### Описание

Shows generated contract metadata.

Default mode is target/package-oriented because plugin authors usually need to understand target class,
production boundary, import/render/validate support, and supported component kinds.

Use --mode runtime to inspect runtime-event support for Claude, Codex, and Gemini.

```
plugin-kit-ai capabilities [flags]
```

### Опции

```
      --format string     output format: table or json (default "table")
  -h, --help              справка по capabilities
      --mode string       capability view: targets or runtime (default "targets")
      --platform string   limit output to a single platform
```

### См. также

* plugin-kit-ai	 - CLI plugin-kit-ai для создания проектов и служебных операций вокруг AI-плагинов.
