# hookplex

Go tooling for AI coding CLI plugins.

## Contract Status

This source tree contains the approved `v1.0` contract plus explicitly marked **public-experimental** surfaces.

Stable now:

- SDK root API and approved Claude/Codex event surfaces
- CLI commands `init`, `validate`, `capabilities`, `install`, `version`
- generated Claude/Codex required scaffold contract

Beta now:

- optional scaffold extras from `hookplex init --extras`
- experimental `hookplex skills` authoring/rendering subsystem
- any future surfaces not explicitly promoted through the audit ledger

Canonical sources of truth:

- event support contract: [docs/generated/support_matrix.md](docs/generated/support_matrix.md)
- compatibility and public-surface policy: [docs/SUPPORT.md](docs/SUPPORT.md)
- delivery status ledger: [docs/STATUS.md](docs/STATUS.md)
- release lanes and shipping gates: [docs/RELEASE.md](docs/RELEASE.md)
- release notes template: [docs/RELEASE_NOTES_TEMPLATE.md](docs/RELEASE_NOTES_TEMPLATE.md)
- release rehearsal worksheet: [docs/REHEARSAL_TEMPLATE.md](docs/REHEARSAL_TEMPLATE.md)
- `v0.9` stable-candidate audit: [docs/V0_9_AUDIT.md](docs/V0_9_AUDIT.md)
- beta-breaking migration registry: [docs/MIGRATIONS.md](docs/MIGRATIONS.md)
- post-`v1` hardening mode: [docs/V1_0_X_HARDENING.md](docs/V1_0_X_HARDENING.md)
- diagnostics contract: [docs/DIAGNOSTICS.md](docs/DIAGNOSTICS.md)
- install compatibility contract: [docs/INSTALL_COMPATIBILITY.md](docs/INSTALL_COMPATIBILITY.md)
- threat model: [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md)

Maintainer-only historical context:

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- [docs/FOUNDATION_REWRITE_VNEXT.md](docs/FOUNDATION_REWRITE_VNEXT.md)
- [docs/adr/README.md](docs/adr/README.md)

## Shipped Scope

What ships now:

- `sdk/hookplex`: generated multi-platform runtime with peer public packages for Claude and Codex
- `cli/hookplex`: `hookplex init`, `hookplex validate`, `hookplex capabilities`, `hookplex install`, `hookplex version`
- `cli/hookplex` experimental skills layer: `hookplex skills init|validate|render`
- `install/plugininstall`: GitHub Releases installer with checksum verification

For the experimental skills layer, handwritten `skills/<name>/SKILL.md` is supported directly. `hookplex skills init` is convenience scaffold, not a required authoring path.
For `hookplex install`, the stable contract covers verified third-party plugin installation only. It does not promise self-update or an auto-update subsystem for the `hookplex` CLI itself.

Current runtime support:

- Claude stable: `Stop`, `PreToolUse`, `UserPromptSubmit`
- Claude beta: `SessionStart`, `SessionEnd`, `Notification`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `SubagentStart`, `SubagentStop`, `PreCompact`, `Setup`, `TeammateIdle`, `TaskCompleted`, `ConfigChange`, `WorktreeCreate`, `WorktreeRemove`
- Codex: `Notify`

Release boundary notes:

- Codex stable support does not guarantee the health of the external `codex exec` runtime before hook execution.
- Claude stable support covers the declared event set only.
- Additional official Claude hooks may be runtime-supported in `public-beta` before they are promoted through the audit ledger.
- Experimental typed custom Claude hooks can be registered locally through `sdk/claude` generic helper functions when upstream support lags behind.
- Experimental typed custom Codex hooks can be registered locally through `sdk/codex` generic helper functions for future argv-JSON hook additions.

Current CLI scaffold targets:

- `--platform codex` (default)
- `--platform claude`

Generator-backed artifacts:

- runtime descriptor registry and invocation resolvers
- public platform registrars
- scaffold platform definitions
- validate rules
- capabilities registry
- generated support contract matrix

## Repository Layout

- `sdk/hookplex`: SDK runtime, public platform packages, descriptor generator
- `cli/hookplex`: CLI scaffold, validation, install wiring
- `install/plugininstall`: installer subsystem
- `repotests`: integration and guard tests
- `docs`: support policy, status ledger, release policy, generated contract docs

## Build And Test

Requirements:

- Go `1.22+`

Common commands from repo root:

```bash
go run ./cmd/hookplex-gen
go build -o bin/hookplex ./cli/hookplex/cmd/hookplex
./bin/hookplex version

go test ./sdk/hookplex/...
go test ./cli/hookplex/...
go test ./install/plugininstall/...
go test ./repotests -run TestHookplexInitGeneratesBuildableModule -count=1
go test ./...
```

## SDK

Root package `hookplex` is now composition/runtime only. Platform APIs live in peer public packages:

- `github.com/hookplex/hookplex/sdk`
- `github.com/hookplex/hookplex/sdk/claude`
- `github.com/hookplex/hookplex/sdk/codex`

Claude example:

```go
package main

import (
	"os"

	hookplex "github.com/hookplex/hookplex/sdk"
	"github.com/hookplex/hookplex/sdk/claude"
)

func main() {
	app := hookplex.New(hookplex.Config{Name: "claude-demo"})
	app.Claude().OnStop(func(*claude.StopEvent) *claude.Response {
		return claude.Allow()
	})
	os.Exit(app.Run())
}
```

Codex example:

```go
package main

import (
	"os"

	hookplex "github.com/hookplex/hookplex/sdk"
	"github.com/hookplex/hookplex/sdk/codex"
)

func main() {
	app := hookplex.New(hookplex.Config{Name: "codex-demo"})
	app.Codex().OnNotify(func(*codex.NotifyEvent) *codex.Response {
		return codex.Continue()
	})
	os.Exit(app.Run())
}
```

See:

- [sdk/hookplex/README.md](sdk/hookplex/README.md)
- [docs/generated/support_matrix.md](docs/generated/support_matrix.md)
- [docs/SUPPORT.md](docs/SUPPORT.md)

## CLI

Build the CLI:

```bash
go build -o bin/hookplex ./cli/hookplex/cmd/hookplex
```

Examples:

```bash
./bin/hookplex init my-plugin
./bin/hookplex init my-plugin --platform claude --extras
./bin/hookplex validate ./my-plugin --platform codex
./bin/hookplex skills init lint-repo --template go-command
./bin/hookplex skills validate .
./bin/hookplex skills render . --target all
./bin/hookplex capabilities --format json --platform claude
./bin/hookplex install owner/repo --tag v1.0.0 --goos linux --goarch amd64
```

`hookplex install` success output is intentionally compact but deterministic:

- installed file path
- resolved release ref and source (`--tag` or `--latest`)
- selected asset
- target GOOS/GOARCH
- overwrite notice only when `--force` replaced an existing file

The command verifies `checksums.txt` from the target release and installs third-party plugin binaries only. Self-update remains out of scope.
Supported and refused release layouts are documented in [docs/INSTALL_COMPATIBILITY.md](docs/INSTALL_COMPATIBILITY.md).

See:

- [cli/hookplex/README.md](cli/hookplex/README.md)
- [docs/SKILLS.md](docs/SKILLS.md)
- [docs/RELEASE.md](docs/RELEASE.md)
