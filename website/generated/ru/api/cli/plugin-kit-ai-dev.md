---
title: "plugin-kit-ai dev"
description: "Следит за проектом, повторно рендерит, валидирует, пересобирает и перезапускает фикстуры при изменениях."
canonicalId: "command:plugin-kit-ai:dev"
surface: "cli"
section: "api"
locale: "ru"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai dev"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai dev" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai dev

Сгенерировано из реального Cobra command tree.

Следит за проектом, повторно рендерит, валидирует, пересобирает и перезапускает фикстуры при изменениях.

## plugin-kit-ai dev

Следит за проектом, повторно рендерит, валидирует, пересобирает и перезапускает фикстуры при изменениях.

### Описание

Watch launcher-based runtime targets in a fast inner loop.

Each cycle re-renders the selected target, performs runtime-aware rebuilds when needed,
runs strict validation, and reruns the configured stable Claude or Codex fixture smoke tests.

Gemini's Go hook lane stays public-beta and is intentionally outside this stable watch loop.
For Gemini use render, render --check, validate --strict, inspect, capabilities --mode runtime,
make test-gemini-runtime-smoke, then gemini extensions link . and optionally rerun
make test-gemini-runtime-live after changes.

```
plugin-kit-ai dev [path] [flags]
```

### Опции

```
      --all                 run every stable event for the selected platform on each cycle
      --event string        stable event to execute (for example Stop, PreToolUse, UserPromptSubmit, or Notify)
      --fixture string      fixture JSON path for single-event runs (default: fixtures/&lt;platform&gt;/&lt;event&gt;.json)
      --golden-dir string   golden output directory (default: goldens/&lt;platform&gt;)
  -h, --help                справка по dev
      --interval duration   poll interval for watch mode (default 750ms)
      --once                run a single render/validate/test cycle and exit
      --platform string     target override ("claude" or "codex-runtime")
```

### См. также

* plugin-kit-ai	 - CLI plugin-kit-ai для создания проектов и служебных операций вокруг AI-плагинов.
