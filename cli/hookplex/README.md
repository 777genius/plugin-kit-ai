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

For the experimental skills subsystem, handwritten `skills/<name>/SKILL.md` is supported directly. `skills init` is convenience scaffold, not a required entrypoint.
For `install`, the stable CLI promise is limited to verified installation of third-party plugin binaries from GitHub Releases. It does not include self-update for the `hookplex` CLI itself.

`hookplex install` prints a deterministic success summary:

- installed file path
- release ref with source (`tag` or `latest`)
- selected asset name
- target GOOS/GOARCH
- overwrite notice only when an existing file was replaced

Supported and unsupported release layouts for `install` are documented in [../../docs/INSTALL_COMPATIBILITY.md](../../docs/INSTALL_COMPATIBILITY.md).

See the root [README.md](../../README.md) for current CLI behavior, shipped scope, and canonical support links.
See [../../docs/SKILLS.md](../../docs/SKILLS.md) for the skills workflow, positioning, and examples.

`go.mod` uses:

- `replace github.com/hookplex/hookplex/sdk => ../../sdk/hookplex`
- `replace github.com/hookplex/hookplex/plugininstall => ../../install/plugininstall`
