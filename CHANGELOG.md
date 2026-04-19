# Changelog

All notable changes to this repository are documented here. **CLI releases** (`plugin-kit-ai` binary built from `cli/plugin-kit-ai`) are versioned together with the monorepo for now; SDK history remains in [sdk/CHANGELOG.md](sdk/CHANGELOG.md).

## [Unreleased]

Changes after `v1.1.2` land here.

## [1.1.2] - 2026-04-19

### Fixed

- routed Gemini installs for GitHub repo-path sources through the managed local projection flow so first-party aliases such as `gitlab` and `notion` no longer fail during full multi-target `plugin-kit-ai add <source>` runs

## [1.1.1] - 2026-04-19

### Fixed

- restored integration installer compatibility with both legacy `src/plugin.yaml` trees and current `plugin/plugin.yaml` trees, which fixes `plugin-kit-ai add <source>` for first-party aliases such as `gitlab` and `notion` when the published CLI resolves current `universal-plugins-for-ai-agents` sources

## [1.1.0] - 2026-04-18

### Added

- repo-level lifecycle e2e coverage for managed integrations across `cursor`, `codex`, `opencode`, and `claude`, including add, update, remove, dry-run, persisted-state, and journal-phase verification
- public release pages for `v1.1.0` across the docs locales, plus generated sidebar updates so the current release appears in the published navigation

### Changed

- pinned Go SDK and shared `plugin-kit-ai-runtime` starter references now align on the `v1.1.0` version contract
- runtime authoring package upgrade smoke now exercises upgrades into `1.1.0` instead of stopping on the older patch line
- docs and website baseline messaging now point to `v1.1.0` as the current public release instead of leaving `v1.0.6` as the visible top-line baseline

Post-`v1.0.0` hardening on `main` continues here. The initial stable release was tagged as `v1.0.0` at commit `6e9379868a666e79d7530a02e171a160c2cb1689`.

### Added

- dedicated `polyglot-smoke` CI workflow for Ubuntu and Windows covering generated launcher smoke for `go`, `python`, `node`, and `shell`
- executable-ABI passthrough e2e coverage for stdin/stdout/stderr/exit-code preservation across generated runtime paths
- dedicated `release-assets` workflow to publish root GitHub Release archives and `checksums.txt` from a selected stable tag before downstream Homebrew/npm/PyPI channels consume the release

### Changed

- Windows launcher validation now accepts extensionless configured entrypoints such as `./bin/x` when the generated launcher file is `./bin/x.cmd`
- documentation now reflects post-`v1.0.0` contract status, the executable-ABI beta boundary, Windows runtime resolution rules, and the TypeScript-over-Node supported path
- Go SDK module root moved from `sdk/plugin-kit-ai/` to `sdk/`, making `github.com/777genius/plugin-kit-ai/sdk@v1.0.4` the first truthful normal-module release; `v1.0.3` remains published but is known-bad for Go SDK consumption
- maintainer-facing docs now distinguish monorepo Go `1.25.9` requirements from generated Go plugin projects that remain on Go `1.22+`, and the repository now ships root `LICENSE` and `SECURITY.md`

## [1.0.0] - 2026-03-26

Release tag: `v1.0.0`  
Release commit: `6e9379868a666e79d7530a02e171a160c2cb1689`

### Added

- **`docs/ARCHITECTURE.md`**, **`repotests/README.md`** — composition roots, exit-code notes, env vars for optional E2E.
- **`docs/FOUNDATION_REWRITE_VNEXT.md`** — Codex-first rewrite target: descriptor-driven core, platform-first API, explicit delivery phases, and acceptance bar for the foundation rewrite.
- **`docs/adr/`** — accepted rewrite ADR set for runtime foundation, descriptor system, unified capability policy, and transport model.
- **`cli/plugin-kit-ai/internal/app`** — `InstallRunner` / `InitRunner` between Cobra and `plugininstall` / `scaffold`; `plugin-kit-ai install` uses **signal-aware context** (interrupt/terminate).
- **`install/plugininstall`:** module `github.com/777genius/plugin-kit-ai/plugininstall` — GitHub Releases install with SHA256 (`checksums.txt`), `.tar.gz` / raw binary; **`domain.PickInstallAsset`**; **`ports.FileSystem`** **`PathExists`** / **`RemoveBestEffort`**; GitHub adapter split **`release.go`** / **`download.go`** (`NewClient` unchanged).
- **`plugin-kit-ai install`:** `owner/repo` with **`--tag`** or **`--latest`**; GoReleaser **`.tar.gz`** or **raw** `*-<goos>-<goarch>[.exe]` + mandatory **`checksums.txt`**; `[--dir bin] [--force] [--pre] [--output-name]`; optional `GITHUB_TOKEN` / `--github-token`; hidden `--github-api-base` for tests/Enterprise.
- **`cli/plugin-kit-ai`:** Cobra commands `init`, `install`, `version` (`runtime/debug.ReadBuildInfo`).
- **Workspace / tests:** `go.work` uses `./cli/plugin-kit-ai`, `./install/plugininstall`, `./sdk`; integration/guard tests live under **`repotests/`** (mock GitHub install, module guards, optional live E2E).
- **Integration test:** `plugin-kit-ai init` in a temp dir → `go mod edit -replace` to local SDK → `go test` / `go vet` on the generated module.
- **Repository tooling:** root `Makefile` (`make test`, `make vet`, optional **`make test-e2e-live`** — live GitHub install checks), `.goreleaser.yml`, `.github/workflows/ci.yml`, `scripts/install.sh` (bootstrap plugin-kit-ai; see comments for `plugin-kit-ai install`).

### Changed

- **Release candidate contract:** root docs, support policy, SDK stability notes, rehearsal artifacts, and audit ledger now describe the approved `v1.0` stable set plus the remaining beta leftovers.
- **Phase 1 foundation prep:** primary docs now describe only current shipped behavior; rewrite-target claims moved to `docs/FOUNDATION_REWRITE_VNEXT.md` and `docs/adr/`.
- **`plugin-kit-ai install`:** **`--latest`** (GitHub `releases/latest`); raw binary assets matching `claude-notifications-<goos>-<goarch>`; `--pre` for prerelease; **`--output-name`**; GitHub API client rejects release JSON above 32 MiB with a clear network error.
- **`install/plugininstall`:** atomic install writes `fsync` the temp file and best-effort `fsync` the install directory after `rename` (skipped on Windows).
- **Downloads:** HTTP client follows **HTTP→HTTP** redirects (e.g. `httptest`) while still blocking insecure downgrades from HTTPS; integration tests cover **302** asset URLs and **429** then success on `checksums.txt`.
- **Raw binary releases:** when several assets match `-GOOS-GOARCH`, companion tools named `sound-preview-*`, `list-devices-*`, `list-sounds-*` are ignored so repos like claude-notifications-go resolve to the main binary. Optional **`make test-e2e-live`** (`PLUGIN_KIT_AI_E2E_LIVE=1`, package **`./repotests`**) runs real GitHub install + `--version` checks.
