---
title: "plugin-kit-ai init"
description: "Создаёт каркас проекта plugin-kit-ai."
canonicalId: "command:plugin-kit-ai:init"
surface: "cli"
section: "api"
locale: "ru"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai init"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai init" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai init

Сгенерировано из реального Cobra command tree.

Создаёт каркас проекта plugin-kit-ai.

## plugin-kit-ai init

Создаёт каркас проекта plugin-kit-ai.

### Описание

Создаёт каркас plugin-kit-ai проекта в package-standard формате.

Выберите lane, который соответствует вашей цели:

Быстрый локальный плагин:
  Используйте `--runtime python` или `--runtime node`, когда локальная итерация в репозитории важнее, чем пакетная поставка.
  Это поддерживаемые пути для executable runtime, но не равноценные production-пути.

Production-ready репозиторий плагина:
  Обычный `init` оставляет наиболее надёжный поддерживаемый runtime-путь. `--runtime go` остаётся значением по умолчанию, а `--platform codex-runtime` остаётся целевой платформой по умолчанию.
  Используйте `--platform claude` для Claude hooks, а `--claude-extended-hooks` добавляйте только когда осознанно нужен более широкий runtime-поддерживаемый набор.
  Используйте `--platform codex-package` для официального Codex plugin bundle без локальной `notify`/runtime-обвязки.
  Используйте `--platform opencode` для OpenCode workspace-config lane без launcher/runtime scaffold.
  Используйте `--platform cursor` для Cursor workspace-config lane без launcher/runtime scaffold.

Уже есть нативная конфигурация:
  Используйте `plugin-kit-ai import`, чтобы привести текущие нативные файлы Claude/Codex/Gemini/OpenCode/Cursor к authored layout package-standard проекта.
  `init` нужен для создания нового package-standard проекта, а не для сохранения нативных файлов как основного authored source of truth.

Публичные флаги:
  --platform   Поддерживаются: `codex-runtime` (по умолчанию), `codex-package`, `claude`, `gemini`, `opencode` и `cursor`.
  --runtime    Поддерживаются: `go` (по умолчанию), `python`, `node`, `shell`; `shell` доступен только для launcher-based targets.
  --typescript Генерирует TypeScript scaffold поверх node runtime lane (требует `--runtime node`).
  --runtime-package
               Для `--runtime python` или `--runtime node` импортирует общий пакет `plugin-kit-ai-runtime` вместо вендоринга helper-файла в `src/`.
  --runtime-package-version
               Фиксирует версию зависимости `plugin-kit-ai-runtime`. Обязательно для development build; выпущенные CLI по умолчанию используют собственный stable tag.
  -o, --output Целевой каталог (по умолчанию: `./&lt;project-name&gt;`).
  -f, --force  Разрешает запись в непустой каталог и перезапись сгенерированных файлов.
  --extras     Дополнительно генерирует optional release helpers, например `Makefile`, `.goreleaser.yml`, переносимые `skills/` и scaffold для stable Python/Node bundle-release workflow там, где это поддерживается.
  --claude-extended-hooks
               Для `--platform claude` генерирует полный runtime-поддерживаемый набор hooks вместо стабильного поднабора по умолчанию.

```
plugin-kit-ai init [project-name] [flags]
```

### Опции

```
      --claude-extended-hooks            для `--platform claude` генерирует полный runtime-поддерживаемый набор hooks вместо стабильного поднабора по умолчанию
      --extras                           включает optional scaffold-файлы (runtime-зависимые extras, а также skills и команды)
  -f, --force                            перезаписывает сгенерированные файлы и разрешает непустой output-каталог
  -h, --help                             справка по init
  -o, --output string                    output directory (default: ./&lt;project-name&gt;)
      --platform string                  целевой lane (`codex-runtime`, `codex-package`, `claude`, `gemini`, `opencode` или `cursor`) (по умолчанию `codex-runtime`)
      --runtime string                   runtime (`go`, `python`, `node` или `shell`) (по умолчанию `go`)
      --runtime-package                  для `--runtime python` или `--runtime node` импортирует общий пакет `plugin-kit-ai-runtime` вместо вендоринга helper-файла
      --runtime-package-version string   фиксирует версию сгенерированной зависимости `plugin-kit-ai-runtime`
      --typescript                       генерирует TypeScript scaffold поверх node runtime lane
```

### См. также

* plugin-kit-ai	 - CLI plugin-kit-ai для создания проектов и служебных операций вокруг AI-плагинов.
