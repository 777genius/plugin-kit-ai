# Canonical Starter Repos

These starter repos are the copy-first entrance for stable Go, Python, and Node authoring on Codex and Claude.
These in-repo starter folders are the canonical source of truth.

Use them when you want the fastest 5-minute path to a working plugin repo, not the broader reference layer.
For deeper contract examples, see [../local/README.md](../local/README.md) and [../plugins/README.md](../plugins/README.md).

## Install `plugin-kit-ai`

Use the supported CLI install order:

1. Homebrew: `brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai`
2. npm: `npm i -g plugin-kit-ai` or `npx plugin-kit-ai@latest ...`
3. pipx: `pipx install plugin-kit-ai` or `pipx run plugin-kit-ai version`
4. Verified fallback: `curl -fsSL https://raw.githubusercontent.com/777genius/plugin-kit-ai/main/scripts/install.sh | sh`
5. CI: `777genius/plugin-kit-ai/setup-plugin-kit-ai@v1`

## Choose A Starter

- [codex-go-starter](./codex-go-starter): stable `codex-runtime` Notify starter for Go teams using the SDK-first production lane
- [codex-python-starter](./codex-python-starter): stable `codex-runtime` Notify starter for Python teams using `requirements.txt` plus a repo-local `.venv`
- [codex-node-typescript-starter](./codex-node-typescript-starter): stable `codex-runtime` Notify starter for Node/TypeScript teams using `npm` and built output under `dist/main.js`
- [claude-go-starter](./claude-go-starter): stable Claude hook starter for Go teams using the SDK-first production lane and the default `Stop`, `PreToolUse`, and `UserPromptSubmit` subset
- [claude-python-starter](./claude-python-starter): stable Claude hook starter for Python teams using the default `Stop`, `PreToolUse`, and `UserPromptSubmit` subset
- [claude-node-typescript-starter](./claude-node-typescript-starter): stable Claude hook starter for Node/TypeScript teams using `npm` and built output under `dist/main.js`

## Official Starter Templates

Use these when you want the real GitHub "Use this template" flow:

- [plugin-kit-ai-starter-codex-go](https://github.com/777genius/plugin-kit-ai-starter-codex-go)
- [plugin-kit-ai-starter-codex-python](https://github.com/777genius/plugin-kit-ai-starter-codex-python)
- [plugin-kit-ai-starter-codex-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-codex-node-typescript)
- [plugin-kit-ai-starter-claude-go](https://github.com/777genius/plugin-kit-ai-starter-claude-go)
- [plugin-kit-ai-starter-claude-python](https://github.com/777genius/plugin-kit-ai-starter-claude-python)
- [plugin-kit-ai-starter-claude-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-claude-node-typescript)

Click "Use this template" on one of those repos, then follow the starter README inside the generated repo.

## Quickstart

1. Copy one starter into a new repo.
2. Run the canonical first run for your runtime:

```bash
plugin-kit-ai doctor .
plugin-kit-ai bootstrap .
plugin-kit-ai validate . --platform <codex-runtime|claude> --strict
```

For Go starters, use the SDK-first first run instead:

```bash
go test ./...
go build -o bin/<starter-name> ./cmd/<starter-name>
plugin-kit-ai validate . --platform <codex-runtime|claude> --strict
```

3. Run the local smoke command from that starter README.
4. When you are ready to ship:

- Python/Node starters already include `.github/workflows/bundle-release.yml` and the stable GitHub Releases handoff path:

```bash
plugin-kit-ai doctor .
plugin-kit-ai bootstrap .
plugin-kit-ai validate . --platform <codex-runtime|claude> --strict
plugin-kit-ai bundle publish . --platform <codex-runtime|claude> --repo owner/repo --tag v1.0.0
plugin-kit-ai bundle fetch owner/repo --tag v1.0.0 --platform <codex-runtime|claude> --runtime <python|node> --dest ./handoff-plugin
```

- Go starters stay on the SDK-first production path and consume `github.com/777genius/plugin-kit-ai/sdk@v1.0.3` as a normal module. Use the production guidance in [../plugins/README.md](../plugins/README.md) and [../../docs/PRODUCTION.md](../../docs/PRODUCTION.md) when you need the clearest long-term release story.

## Opinionated Defaults

- Go starters keep one canonical SDK-first story: `go test ./...` plus `go build -o bin/<starter-name> ./cmd/<starter-name>`
- Python starters keep one canonical env story: `requirements.txt` plus a repo-local `.venv`
- Node starters keep one canonical package-manager story: `npm`
- TypeScript starters keep built output under `dist/main.js`

Supported alternatives still exist, but they are not encoded into the starter repos:

- Python alternatives: `uv`, `poetry`, `pipenv`
- Node alternatives: `pnpm`, `yarn`, `bun`
