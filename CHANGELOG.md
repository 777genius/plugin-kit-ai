# Changelog

All notable changes to this repository are documented here. **CLI releases** (`plugin-kit-ai` binary built from `cli/plugin-kit-ai`) are versioned together with the monorepo for now; SDK history remains in [sdk/plugin-kit-ai/CHANGELOG.md](sdk/plugin-kit-ai/CHANGELOG.md).

## [Unreleased]

Post-`v1.0.0` hardening on `main` lands here. The initial stable release was tagged as `v1.0.0` at commit `6e9379868a666e79d7530a02e171a160c2cb1689`.

### Added

- dedicated `polyglot-smoke` CI workflow for Ubuntu and Windows covering generated launcher smoke for `go`, `python`, `node`, and `shell`
- executable-ABI passthrough e2e coverage for stdin/stdout/stderr/exit-code preservation across generated runtime paths

### Changed

- Windows launcher validation now accepts extensionless configured entrypoints such as `./bin/x` when the generated launcher file is `./bin/x.cmd`
- documentation now reflects post-`v1.0.0` contract status, the executable-ABI beta boundary, Windows runtime resolution rules, and the TypeScript-over-Node supported path
- Go starter, scaffold, and production example consumption now target `github.com/777genius/plugin-kit-ai/sdk@v1.0.3`; public `go mod edit -replace` onboarding was removed in favor of the SDK submodule tagging contract

## [1.0.0] - 2026-03-26

Release tag: `v1.0.0`  
Release commit: `6e9379868a666e79d7530a02e171a160c2cb1689`

### Added

- **`docs/ARCHITECTURE.md`**, **`repotests/README.md`** — composition roots, exit-code notes, env vars for optional E2E.
- **`docs/FOUNDATION_REWRITE_VNEXT.md`** — Codex-first rewrite target: descriptor-driven core, platform-first API, explicit delivery phases, and acceptance bar for the foundation rewrite.
- **`docs/adr/`** — accepted rewrite ADR set for runtime foundation, descriptor system, unified capability policy, and transport model.
- **`cli/plugin-kit-ai/internal/app`** — `InstallRunner` / `InitRunner` between Cobra and `plugininstall` / `scaffold`; `plugin-kit-ai install` uses **signal-aware context** (interrupt/terminate).
- **`install/plugininstall`:** module `github.com/plugin-kit-ai/plugin-kit-ai/plugininstall` — GitHub Releases install with SHA256 (`checksums.txt`), `.tar.gz` / raw binary; **`domain.PickInstallAsset`**; **`ports.FileSystem`** **`PathExists`** / **`RemoveBestEffort`**; GitHub adapter split **`release.go`** / **`download.go`** (`NewClient` unchanged).
- **`plugin-kit-ai install`:** `owner/repo` with **`--tag`** or **`--latest`**; GoReleaser **`.tar.gz`** or **raw** `*-<goos>-<goarch>[.exe]` + mandatory **`checksums.txt`**; `[--dir bin] [--force] [--pre] [--output-name]`; optional `GITHUB_TOKEN` / `--github-token`; hidden `--github-api-base` for tests/Enterprise.
- **`cli/plugin-kit-ai`:** Cobra commands `init`, `install`, `version` (`runtime/debug.ReadBuildInfo`).
- **Workspace / tests:** `go.work` uses `./cli/plugin-kit-ai`, `./install/plugininstall`, `./sdk/plugin-kit-ai`; integration/guard tests live under **`repotests/`** (mock GitHub install, module guards, optional live E2E).
- **Integration test:** `plugin-kit-ai init` in a temp dir → `go mod edit -replace` to local SDK → `go test` / `go vet` on the generated module.
- **Repository tooling:** root `Makefile` (`make test`, `make vet`, optional **`make test-e2e-live`** — live GitHub install checks), `.goreleaser.yml`, `.github/workflows/ci.yml`, `scripts/install.sh` (bootstrap plugin-kit-ai; see comments for `plugin-kit-ai install`).

### Changed

- **Release candidate contract:** root docs, support policy, SDK stability notes, rehearsal artifacts, and audit ledger now describe the approved `v1.0` stable set plus the remaining beta leftovers.
- **Phase 1 foundation prep:** primary docs now describe only current shipped behavior; rewrite-target claims moved to `docs/FOUNDATION_REWRITE_VNEXT.md` and `docs/adr/`.
- **`plugin-kit-ai install`:** **`--latest`** (GitHub `releases/latest`); raw binary assets matching `claude-notifications-<goos>-<goarch>`; `--pre` for prerelease; **`--output-name`**; GitHub API client rejects release JSON above 32 MiB with a clear network error.
- **`install/plugininstall`:** atomic install writes `fsync` the temp file and best-effort `fsync` the install directory after `rename` (skipped on Windows).
- **Downloads:** HTTP client follows **HTTP→HTTP** redirects (e.g. `httptest`) while still blocking insecure downgrades from HTTPS; integration tests cover **302** asset URLs and **429** then success on `checksums.txt`.
- **Raw binary releases:** when several assets match `-GOOS-GOARCH`, companion tools named `sound-preview-*`, `list-devices-*`, `list-sounds-*` are ignored so repos like claude-notifications-go resolve to the main binary. Optional **`make test-e2e-live`** (`PLUGIN_KIT_AI_E2E_LIVE=1`, package **`./repotests`**) runs real GitHub install + `--version` checks.
