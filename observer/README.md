# Protected stable-launch observer

This service accepts only `POST /v1/stable-launch/observe` on loopback. Caddy is
the public TLS boundary. The application verifies GitHub Actions OIDC with the
official JWKS, matches an exact identity policy, corroborates the public run and
job, consumes `jti` once, and then invokes one fixed root-owned runner.

The request cannot select a checkout, command, executable, adapter, workspace,
or container. A root cgroup supervisor receives
only the canonical request and GitHub attestation over a peer-credential checked
Unix socket. Its self-contained installed source is checked against the reviewed
SHA-256 before the observer accepts traffic. It executes four hardlinked
entrypoints of the tracked `fixed_adapters.py` source from a root-owned manifest,
re-checks the common adapter and config digests immediately
before execution, places every adapter/client tree in a root-controlled cgroup,
then drops it to one of four separate `uap-observer-{codex,cursor,kiro,control}`
UIDs. A client can traverse only its own `0700` profile/workspace and cannot read
another client's dedicated auth. Those UIDs cannot move processes into the
delegated parent. The supervisor kills the complete job cgroup
on success, failure, spawn error, or timeout; a stuck reap or populated cgroup
terminates the supervisor so systemd owns final control-group cleanup. The
runner starts on an empty synthetic root and bind-mounts only reviewed system
libraries, fixed tools, copied profiles, disposable workspaces, and output
paths. Each adapter verifies the effective kernel mount table against that
positive allowlist before making the no-real-project claim. It cannot read
observer state, broad workspace roots, the signing socket/key, or Docker sockets.
Runner output must be the four allowlisted JSON artifacts and is rejected if it
contains credential-like values, absolute paths, or path URI variants.

Responses are cached by canonical request digest for 30 minutes. A process-wide
file lock provides single-flight execution; identical request digests coalesce
and a different digest is rejected while protected work is active. Each
completed run directory is published by atomic rename. Cached bundles are
accepted only after canonical JSON, challenge, schema, freshness, key identity,
and Ed25519 signature verification. The private key is held only by the separate
root signer service; both privileged sockets authenticate the observer UID with
Linux `SO_PEERCRED`.
The scarce application execution bucket is charged only after OIDC validation,
public-run corroboration, and single-use-token consumption, so tokenless,
invalid, and replayed requests cannot exhaust authenticated capacity.

Install the complete reviewed checkout at `/opt/uap-observer`. Replace the
public-key placeholder in a separate root-owned observer config and provision a
root-owned `0640` adapter config matching
`deploy/uap-observer-adapter-config.schema.json`; it contains only pinned
binaries/profiles, request/release identities, the fixed consent-record directory, and ChatGPT binding data,
never argv, environment, or secrets. Run
`deploy/uap-observer-install.sh SOURCE_ROOT ADAPTER_CONFIG sha256:ADAPTER_DIGEST OBSERVER_CONFIG sha256:OBSERVER_DIGEST CADDY_2.11.4_LINUX_AMD64_ARCHIVE CADDY_CONFIG sha256:CADDY_CONFIG_DIGEST`.
The installer creates the exact hardlinks and digest-pinned manifest, fails
closed on placeholders, first copies every hash-locked input into a root-only
staging closure, verifies that closure and both external configs, verifies the
final `0644`/`0640`/`0755` installed modes through the runtime startup checks,
and journals every prior systemd unit/drop-in before replacement. Any mutation
or daemon-reload failure restores the exact previous bootstrap state before the
closure pointer can move. The installer creates dedicated service identities and IPC
groups, and installs dependencies into an isolated venv using
`requirements.lock` with `--require-hashes --no-deps`. It does not enable or
start services. The Caddy input must be the official Linux amd64 v2.11.4
`caddy_2.11.4_linux_amd64.tar.gz` archive (official archive SHA-256
`527fbf917c39189a1e3b31d34fa955601680b2d5c8055d2a87b8b9588dec7bb9`,
extracted binary SHA-256
`b7105518e3ed1c0761f232e44fc09345535533c9cb0abf0e12809416c7ac64d9`).
The installer also verifies `caddy version` is exactly `v2.11.4` and validates
the supplied non-placeholder Caddyfile. Separately enable the socket, runner,
signer, observer, and proxy only after provisioning the root-owned Ed25519 key.

Never point an adapter at an original saved home. For each test client, first run
`uap-observer-provision-profile --client CLIENT --root-owned-seed ABS_PATH --seed-digest show`,
then repeat with the printed `sha256:` value. The helper rejects links/special
files, copies into `/var/lib/uap-observer/profiles/CLIENT` with that client's UID
and `0700`/`0600` modes, leaves the root-owned seed untouched, and never prints
file contents. A non-empty existing profile is never overwritten.

The runtime and Notion adapters use only source-fixed Codex, Cursor, and Kiro
version, native MCP list, and challenge-bound tool invocation argv against fresh
disposable Git roots and adapter-owned test profiles. A pass requires a root-owned
manager receipt, the exact product in both native list observations, and a
structured successful tool-call event; prompt echo alone is inconclusive.
Create consent with `uap-observer-attest-consent`; its root-owned `O_EXCL`
record binds the full canonical request digest and is atomically moved into the
root-only consumed directory by the supervisor only after the fixed consent
applet succeeds. The ChatGPT adapter separately verifies the exact `.app.json`, performs a direct
no-proxy/no-redirect public Cloudflare MCP initialize/list/read-only probe, and
then reads a short-lived challenge/request-bound root-owned human attestation
created with `O_EXCL` by `uap-observer-attest-chatgpt`; the supervisor atomically
tombstones it after adapter success. No real user project, copied auth, raw provider
output, cookies, or tokens are accepted into evidence.
The signer accepts only the exact Phase 6 artifact set: twelve non-Notion
runtime pairs, three Notion pairs, and one ChatGPT UI pair, all uniquely and
cross-bound to the consent, challenge, run, directory/release identities, and
pseudonyms.

Run focused tests with:

```sh
timeout 120s python -m unittest discover -s observer/tests -v
```
