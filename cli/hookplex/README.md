# hookplex CLI

Module: `github.com/hookplex/hookplex/cli`. Builds the **`hookplex`** binary: `init`, `validate`, `capabilities`, `install`, `version`, plus experimental `skills` authoring commands.

Current CLI contract status in this source tree: approved for `public-stable` in the pending `v1.0` release. Repository-wide compatibility and release policy live in [../../docs/SUPPORT.md](../../docs/SUPPORT.md) and [../../docs/RELEASE.md](../../docs/RELEASE.md).

`hookplex init` scaffolds either a **Codex project** (`--platform codex`, default) or a **Claude plugin** (`--platform claude`).
`hookplex validate` checks a project against descriptor-driven platform rules.
`hookplex capabilities` prints generated runtime support and capability metadata.

```bash
# from repository root
go build -o bin/hookplex ./cli/hookplex/cmd/hookplex
```

Current-state behavior:

- `init`: project scaffold for `codex` or `claude`
- `validate`: descriptor-driven project validation
- `capabilities`: generated support/capability introspection in table or JSON
- `install`: plugin binary from GitHub Releases with checksum verification
- `version`: build/version info
- `skills init|validate|render`: experimental SKILL.md authoring and agent render tooling

See the root [README.md](../../README.md) for current CLI behavior, shipped scope, and canonical support links.

`go.mod` uses:

- `replace github.com/hookplex/hookplex/sdk => ../../sdk/hookplex`
- `replace github.com/hookplex/hookplex/plugininstall => ../../install/plugininstall`
