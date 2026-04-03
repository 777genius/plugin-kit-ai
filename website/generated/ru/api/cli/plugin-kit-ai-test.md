---
title: "plugin-kit-ai test"
description: "Запускает стабильные smoke-тесты на фикстурах против launcher entrypoint."
canonicalId: "command:plugin-kit-ai:test"
surface: "cli"
section: "api"
locale: "ru"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai test"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai test" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai test

Сгенерировано из реального Cobra command tree.

Запускает стабильные smoke-тесты на фикстурах против launcher entrypoint.

## plugin-kit-ai test

Запускает стабильные smoke-тесты на фикстурах против launcher entrypoint.

### Описание

Run stable Claude or Codex runtime smoke tests from JSON fixtures.

The command loads a fixture, invokes the configured launcher entrypoint with the correct carrier
(stdin JSON for Claude stable hooks, argv JSON for Codex notify), and optionally compares or updates
golden stdout/stderr/exitcode files for CI-grade regression checks.

Gemini's Go hook lane stays public-beta and is intentionally outside this stable fixture surface.
For Gemini use go test, render --check, validate --strict, and a real Gemini CLI session via
gemini extensions link .

```
plugin-kit-ai test [path] [flags]
```

### Опции

```
      --all                 run every stable event for the selected platform
      --event string        stable event to execute (for example Stop, PreToolUse, UserPromptSubmit, or Notify)
      --fixture string      fixture JSON path for single-event runs (default: fixtures/&lt;platform&gt;/&lt;event&gt;.json)
      --format string       output format: text or json (default "text")
      --golden-dir string   golden output directory (default: goldens/&lt;platform&gt;)
  -h, --help                справка по test
      --platform string     target override ("claude" or "codex-runtime")
      --update-golden       write current stdout/stderr/exitcode outputs into the golden files
```

### См. также

* plugin-kit-ai	 - CLI plugin-kit-ai для создания проектов и служебных операций вокруг AI-плагинов.
