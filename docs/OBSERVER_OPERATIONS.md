# Observer operations

This is the preflight and deployment checklist for one **fresh, disposable,
Linux x86-64 observer host**. It is not an upgrade procedure. Use only fresh
dedicated test identities and synthetic disposable Git roots. Never copy Mac
authentication, a saved home, cookies, tokens, or client profiles to this host,
and never point an observer client at a real project.

## 1. Freeze and verify the inputs

Set the approved 40-hex commit supplied with the release record; do not use a
branch name. Work from an offline-delivered checkout and record command output
in a non-secret change ticket.

```sh
SOURCE_ROOT=/opt/uap-observer
REVIEWED_COMMIT=<approved-40-hex-commit>
test "$(git -C "$SOURCE_ROOT" rev-parse HEAD)" = "$REVIEWED_COMMIT"
test -z "$(git -C "$SOURCE_ROOT" status --porcelain --untracked-files=all)"
git -C "$SOURCE_ROOT" fsck --no-dangling
(cd "$SOURCE_ROOT" && sha256sum -c deploy/uap-observer-runtime.sha256)
```

Record `REVIEWED_COMMIT`, the runtime-manifest digest, and the deployment input
digests below. Do not record the signing key or profile contents.

```sh
sha256sum "$SOURCE_ROOT/deploy/uap-observer-runtime.sha256" \
  /root/uap-observer-adapter-config.template.json /root/uap-observer.json \
  /root/Caddyfile /root/caddy_2.11.4_linux_amd64.tar.gz \
  /root/uap-observer-egress-allowlist.json
```

The Caddy archive must be the official Linux amd64 v2.11.4 archive with SHA-256
`527fbf917c39189a1e3b31d34fa955601680b2d5c8055d2a87b8b9588dec7bb9`.
The adapter config must validate against
`deploy/uap-observer-adapter-config.schema.json`. The protected input tree and
installer-managed deployment files are:

| Artifact | Installed path | Mode |
| --- | --- | --- |
| Git, Codex, Kiro native executables | `/opt/uap-observer-inputs/bin/{git,codex,kiro,kiro-cli-chat}` | root-owned `0755` regular files; Kiro requires both digest-pinned executables |
| Complete Cursor Agent bundle | `/opt/uap-observer-inputs/cursor/` plus `/opt/uap-observer-inputs/cursor-bundle.json` | root-owned closure; directories `0755`, files `0644` or `0755`, canonical manifest `0644` |
| ChatGPT app binding and projection receipt | `/opt/uap-observer-inputs/chatgpt/{app-binding.json,projection-receipt.json}` | `root:uap-observer-adapter-config`, `0640` |
| independently captured external-PR evidence | `/opt/uap-observer-inputs/external-pr-evidence.json` | `root:uap-observer-adapter-config`, `0640` |
| operator-approved proxy FQDN allowlist | `/opt/uap-observer-current/etc/uap-observer-egress-allowlist.json` | `root:root`, `0644`, one-link regular file |
| repository proxy executable | `/opt/uap-observer-current/libexec/uap-observer-egress-proxy` | `root:root`, `0755`, one-link regular file |
| repository proxy units | `/opt/uap-observer-current/systemd/` and `/etc/systemd/system/uap-observer-egress-proxy.{socket,service}` | `root:root`, `0644`, one-link regular files |

The proxy executable and units are repository source/runtime-closure files, not
separate operator inputs. The installer copies and verifies them. Do not stage
or copy units into `/etc/systemd/system` manually.

There may be no other entries under `/opt/uap-observer-inputs`; directories
must be root-owned and not group/other-writable. Every file must have one hard
link, no path may be a symlink, and every `sha256:` in the adapter config must
match its bytes. Cursor is one bounded bundle because its launcher depends on
the sibling Node runtime and JavaScript/native modules. The manifest binds
every relative path, mode, size, and digest; a launcher-only copy is invalid.
The external PR record must validate against
`tests/e2e/schemas/external-pr-evidence.schema.json`, identify a genuinely
external unmerged PR, and bind its successful head checks to the exact release
and Directory identity. A local fork simulation is not sufficient.

Build that exact tree from the approved, digest-recorded input directory. The
group creation is safe before the idempotent installer creates the remaining
service identities.

```sh
getent group uap-observer-adapter-config >/dev/null || \
  groupadd --system uap-observer-adapter-config
install -d -o root -g root -m 0755 /opt/uap-observer-inputs \
  /opt/uap-observer-inputs/bin /opt/uap-observer-inputs/chatgpt \
  /opt/uap-observer-inputs/cursor
for name in git codex kiro kiro-cli-chat; do
  install -o root -g root -m 0755 "/root/approved-inputs/$name" \
    "/opt/uap-observer-inputs/bin/$name"
done
cp -a --reflink=auto /root/approved-inputs/cursor/. \
  /opt/uap-observer-inputs/cursor/
chown -R root:root /opt/uap-observer-inputs/cursor
find /opt/uap-observer-inputs/cursor -type d -exec chmod 0755 {} +
find /opt/uap-observer-inputs/cursor -type f -perm /111 -exec chmod 0755 {} +
find /opt/uap-observer-inputs/cursor -type f ! -perm /111 -exec chmod 0644 {} +
PYTHONDONTWRITEBYTECODE=1 PYTHONPATH="$SOURCE_ROOT" python3 -B \
  "$SOURCE_ROOT/scripts/build_observer_client_bundle.py" \
  /opt/uap-observer-inputs/cursor \
  /opt/uap-observer-inputs/cursor-bundle.json
for name in app-binding.json projection-receipt.json; do
  install -o root -g uap-observer-adapter-config -m 0640 \
    "/root/approved-inputs/$name" "/opt/uap-observer-inputs/chatgpt/$name"
done
install -o root -g uap-observer-adapter-config -m 0640 \
  /root/approved-inputs/external-pr-evidence.json \
  /opt/uap-observer-inputs/external-pr-evidence.json
sha256sum /opt/uap-observer-inputs/bin/* \
  /opt/uap-observer-inputs/cursor-bundle.json \
  /opt/uap-observer-inputs/chatgpt/* \
  /opt/uap-observer-inputs/external-pr-evidence.json
```

Before installation or any manager source resolution, freeze the reviewed
pre-projection adapter template and derive its source matrix exactly once:

```sh
test -f /root/uap-observer-adapter-config.template.json
test ! -L /root/uap-observer-adapter-config.template.json
test "$(stat -c '%U %a %h' /root/uap-observer-adapter-config.template.json)" = "root 400 1"
PYTHONPATH="$SOURCE_ROOT" python3 - \
  /root/uap-observer-adapter-config.template.json /root/uap-observer-matrix.json \
  "$SOURCE_ROOT/deploy/uap-observer-adapter-config.schema.json" <<'PY'
import copy, json, math, os, pathlib, sys, tempfile
import jsonschema
source, target = map(pathlib.Path, sys.argv[1:3])
schema_path = pathlib.Path(sys.argv[3])
def strict_load(path):
    def pairs(items):
        value, folded = {}, set()
        for key, child in items:
            normalized = key.casefold()
            if key in value or normalized in folded:
                raise SystemExit(f"{path}: duplicate or case-confusable JSON member")
            value[key] = child; folded.add(normalized)
        return value
    def constant(value): raise SystemExit(f"{path}: non-finite JSON number {value}")
    def finite(value):
        decoded = float(value)
        if not math.isfinite(decoded): constant(value)
        return decoded
    return json.loads(path.read_bytes(), object_pairs_hook=pairs,
                      parse_constant=constant, parse_float=finite)
value = strict_load(source)
schema = strict_load(schema_path)
clients = value.get("clients") if isinstance(value, dict) else None
if (type(value.get("schema_version")) is not int or value["schema_version"] != 1
    or not isinstance(clients, dict) or set(clients) != {"codex", "cursor", "kiro"}
    or any(not isinstance(record, dict) or "native_projection" in record
           for record in clients.values())):
    raise SystemExit("reviewed pre-projection template is not the exact unprojected schema")
validated = copy.deepcopy(value)
for client in ("codex", "cursor", "kiro"):
    validated["clients"][client]["native_projection"] = {
        "path": f"/var/lib/uap-observer/proofs/{client}/native-projection.json",
        "sha256": "sha256:" + "0" * 64,
    }
jsonschema.validate(validated, schema)
matrix = value.get("matrix")
body = json.dumps({"matrix": matrix}, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
fd, temporary = tempfile.mkstemp(prefix=".matrix.", dir=target.parent)
try:
    os.fchmod(fd, 0o400); os.write(fd, body); os.fsync(fd)
finally: os.close(fd)
os.link(temporary, target, follow_symlinks=False); os.unlink(temporary)
directory = os.open(target.parent, os.O_RDONLY | os.O_DIRECTORY)
try: os.fsync(directory)
finally: os.close(directory)
PY
sha256sum /root/uap-observer-matrix.json
```

Do not create, read, hash, or validate the final adapter config at this point. Its three
projection digests do not exist until the first bootstrap phase below.

Acquire the manager from the exact immutable public release; a package-manager
facade, locally built binary, or the sanitized repository fixture is not a
deployment input. The repository resolver checks GitHub's immutable release
flag, tag commit, exact asset set, manifest/checksums agreement, selected asset
SHA-256, and GitHub artifact attestation before writing the binary:

```sh
install -d -o root -g root -m 0700 /root/approved-inputs/agentplugins-0.1.18
PYTHONPATH="$SOURCE_ROOT" python3 - <<'PY'
import hashlib
import subprocess
from pathlib import Path
from scripts.run_launch_evidence_e2e import resolve_github_release

root = Path("/root/approved-inputs/agentplugins-0.1.18")
binary, manifest, manifest_digest = resolve_github_release(
    "777genius/plugin-kit-ai", "agentplugins-v0.1.18",
    root / "agentplugins", asset_name="agentplugins_0.1.18_linux_amd64",
)
if manifest_digest != "sha256:0e8f7316ddef542067bdd7276273fffa3bc00532afed8fd42be12f612aedea57":
    raise SystemExit("release-manifest.json differs from the deployment pin")
checksums = "sha256:" + hashlib.sha256((root / "checksums.txt").read_bytes()).hexdigest()
if checksums != "sha256:d581ac34d9880afe998f8f871df285b5474623778d2eae98ebc8780a932a9fa8":
    raise SystemExit("checksums.txt differs from the deployment pin")
observed = subprocess.run([binary, "version"], check=True, text=True,
                          stdout=subprocess.PIPE).stdout.strip()
if manifest["version"] != "0.1.18" or observed != "agentplugins 0.1.18":
    raise SystemExit("selected manager binary is not exact agentplugins 0.1.18")
PY
```

This requires authenticated GitHub attestation verification and fails closed
when public provenance cannot be verified. Retain the emitted release identity,
manifest, checksums, binary digest, and attestation JSON in the non-secret
change ticket.

Use exact, approved Linux client binaries at the paths above, not wrappers.
Create authentication independently on this disposable host with dedicated
test accounts. For each pinned binary, use a separate empty root such as
`/root/profile-seeds/{codex,cursor,kiro}`, set `HOME`, `XDG_CONFIG_HOME`, and
`XDG_CACHE_HOME` inside that root, and additionally set `CODEX_HOME` inside the
Codex root. Never share a profile between clients.

Exercise one real, non-secret network request from **each exact pinned binary**
with `HTTP_PROXY`, `HTTPS_PROXY`, and `ALL_PROXY` set to the approved test proxy
and `NO_PROXY` empty. During this preflight, block direct Internet egress for
the login process and retain sanitized proxy connection metadata plus the
binary SHA-256 and exit result. A successful version-only command is not a
proxy test. Stop if any client bypasses or ignores the proxy.

Use only the clients' supported fresh-host login paths: Codex supports
`codex login --device-auth`; Cursor supports `NO_OPEN_BROWSER=1 login`
(invoke the pinned Cursor executable's `login` command) and API-key
automation; Kiro supports its remote device flow and `KIRO_API_KEY` headless
mode. API keys may be supplied only transiently to the login/preflight process:
never put them in a script, image, adapter config, service environment, or
evidence. Prefer device flow for the persistent test profile.

Materialize the five reviewed heroes separately in each seed. Short names are
not eligible protected inputs in agentplugins 0.1.18. Derive every add argument
from the approved tuple as the canonical
`source_repository@source_revision//source_path`; reject a missing revision, a
non-40-lowercase-hex revision, or any source field mismatch. Agentplugins 0.1.18
prepares these clients and retains its immutable pre-execution plan in the add
envelope. That nested plan may record `manual_activation_required`; the sibling
realized activation must instead be `active` with `installation_verified`, no
confirmation pending, and `group_phase: external_completed`. The same manual
state anywhere outside that historical plan is incomplete and cannot be sealed.
Its real successful add envelope uses `result: success`, `data.status:
completed`, and a selected target status of `external_completed` (the validator
below also accepts the documented success/completed status spellings at the
target boundary).

The manager detects clients through `PATH`. Never run it with an ambient client
on `PATH`. Create one root-owned temporary bin directory per seed containing
the pinned Git binary and only the exact pinned client under a name that 0.1.18
recognizes (`codex`, `cursor`, or `kiro-cli`). Cursor uses a fixed two-line
launcher into the verified bundle because its executable cannot be separated
from its sibling dependencies. Create `.codex` and export `CODEX_HOME` only for
Codex; an empty `.codex` directory is itself a positive Codex detection signal.
Before any source is resolved or installed, the sole detection gate is exactly
`agentplugins doctor --format json` (no source or target operands):

```sh
AGENTPLUGINS=/root/approved-inputs/agentplugins-0.1.18/agentplugins
for client in codex cursor kiro; do
  seed="/root/profile-seeds/$client"
  evidence="/root/profile-seed-evidence/$client"
  client_path="/root/profile-seed-path/$client"
  install -d -o root -g root -m 0700 "$seed" "$seed/.config" "$seed/.cache" "$seed/.auth" "$seed/.state" \
    "$seed/.agentplugins" "$evidence/add" "$evidence/info" \
    "$evidence/doctor" "$evidence/post-doctor" "$client_path"
  if [ "$client" = codex ]; then
    install -d -o root -g root -m 0700 "$seed/.codex"
  fi
  install -o root -g root -m 0755 /opt/uap-observer-inputs/bin/git "$client_path/git"
  case "$client" in
    codex) install -o root -g root -m 0755 /opt/uap-observer-inputs/bin/codex "$client_path/codex" ;;
    cursor)
      printf '%s\n' '#!/bin/sh' \
        'exec /opt/uap-observer-inputs/cursor/cursor-agent "$@"' \
        >"$client_path/cursor"
      chown root:root "$client_path/cursor"
      chmod 0755 "$client_path/cursor"
      ;;
    kiro)
      install -o root -g root -m 0755 /opt/uap-observer-inputs/bin/kiro "$client_path/kiro-cli"
      install -o root -g root -m 0755 /opt/uap-observer-inputs/bin/kiro-cli-chat "$client_path/kiro-cli-chat"
      ;;
  esac
  printf '%s\n' '#!/bin/sh' \
    'exec /opt/uap-observer-inputs/cursor/node "$@"' \
    >"$client_path/node"
  chown root:root "$client_path/node"
  chmod 0755 "$client_path/node"
  # Record and compare regular-file identity, inode, link count, mode, and digest.
  # Stop on a symlink, hardlink, digest mismatch, or an alias outside this list.
  find "$client_path" -maxdepth 1 -type f -printf '%f %D:%i %n %m\n' | LC_ALL=C sort
  sha256sum "$client_path"/* /opt/uap-observer-inputs/bin/git
  test "$(sha256sum "$client_path/git" | cut -d' ' -f1)" = \
    "$(sha256sum /opt/uap-observer-inputs/bin/git | cut -d' ' -f1)"
  case "$client" in
    codex) expected_names='codex git node'; aliases='codex' ;;
    cursor) expected_names='cursor git node'; aliases='' ;;
    kiro) expected_names='git kiro-cli kiro-cli-chat node'; aliases='kiro-cli' ;;
  esac
  test "$(find "$client_path" -mindepth 1 -maxdepth 1 -printf '%f\n' | LC_ALL=C sort | tr '\n' ' ' | sed 's/ $//')" = "$expected_names"
  for alias in $aliases; do
    test -f "$client_path/$alias" && test ! -L "$client_path/$alias"
    test "$(stat -c %h "$client_path/$alias")" = 1
    test "$(sha256sum "$client_path/$alias" | cut -d' ' -f1)" = \
      "$(sha256sum "/opt/uap-observer-inputs/bin/$client" | cut -d' ' -f1)"
  done
  if [ "$client" = cursor ]; then
    test "$(sed -n '2p' "$client_path/cursor")" = \
      'exec /opt/uap-observer-inputs/cursor/cursor-agent "$@"'
    PYTHONPATH="$SOURCE_ROOT" python3 -B - <<'PY'
import json
from pathlib import Path
from observer.client_bundle import verify_bundle
config = json.loads(Path("/root/uap-observer-adapter-config.template.json").read_text())
bundle = config["clients"]["cursor"]["bundle"]
verify_bundle(root=Path(bundle["root"]), manifest=Path(bundle["manifest"]), manifest_sha256=bundle["manifest_sha256"])
PY
  fi
  test "$(sed -n '2p' "$client_path/node")" = \
    'exec /opt/uap-observer-inputs/cursor/node "$@"'
  if [ "$client" = kiro ]; then
    test "$(sha256sum "$client_path/kiro-cli-chat" | cut -d' ' -f1)" = \
      59f47eb75928fa158df1cea31382cb39a4eb0d8ec7afbcfc4c6e75693d35163e
    test "$(sha256sum "$client_path/kiro-cli" | cut -d' ' -f1)" = \
      14d835aff3772afb9ffb71e395b433df516c091dea8c43daef46e7cb66368358
  fi
  set -- env HOME="$seed" XDG_CONFIG_HOME="$seed/.config" \
    XDG_CACHE_HOME="$seed/.cache" AGENTPLUGINS_HOME="$seed/.agentplugins" \
    NODE_USE_ENV_PROXY=1 PATH="$client_path"
  if [ "$client" = codex ]; then set -- "$@" CODEX_HOME="$seed/.codex"; fi
  "$@" "$AGENTPLUGINS" doctor --format json >"$evidence/doctor/detection.json"
  python3 - "$client" "$evidence/doctor/detection.json" <<'PY'
import json, math, pathlib, sys
expected, path = sys.argv[1:]
def pairs(items):
    value, folded = {}, set()
    for key, child in items:
        normalized = key.casefold()
        if key in value or normalized in folded:
            raise SystemExit("doctor returned duplicate or case-confusable JSON member")
        value[key] = child; folded.add(normalized)
    return value
def constant(value): raise SystemExit(f"doctor returned non-finite JSON number {value}")
def finite(value):
    decoded = float(value)
    if not math.isfinite(decoded): constant(value)
    return decoded
value = json.loads(pathlib.Path(path).read_bytes(), object_pairs_hook=pairs,
                   parse_constant=constant, parse_float=finite)
if (not isinstance(value, dict) or type(value.get("schema_version")) is not int
        or value["schema_version"] != 1 or value.get("command") != "doctor"
        or value.get("result") != "success"):
    raise SystemExit("doctor did not return the exact successful 0.1.18 envelope")
positive = []
def visit(item):
    if isinstance(item, dict):
        client = item.get("client_id")
        detected = item.get("detected") is True or item.get("status") == "detected"
        if isinstance(client, str) and detected:
            positive.append(client)
        for child in item.values(): visit(child)
    elif isinstance(item, list):
        for child in item: visit(child)
visit(value.get("data"))
if positive != [expected]:
    raise SystemExit("doctor did not detect exactly the intended target")
PY
  for plugin in agent-code-navigator context7 cloudflare-docs chrome-devtools notion; do
    source="$(python3 - "$client" "$plugin" /root/uap-observer-matrix.json <<'PY'
import json, re, sys
client, plugin, path = sys.argv[1:]
matrix = json.load(open(path, encoding="utf-8"))["matrix"]
matches = [row["tuple"] for row in matrix if row["client"] == client and row["plugin"] == plugin]
if len(matches) != 1:
    raise SystemExit("approved matrix does not contain exactly one tuple")
item = matches[0]
revision = item.get("source_revision")
if not isinstance(revision, str) or re.fullmatch(r"[0-9a-f]{40}", revision) is None:
    raise SystemExit("approved source revision is not a full lowercase commit SHA")
repository, source_path = item.get("source_repository"), item.get("source_path")
if not isinstance(repository, str) or not isinstance(source_path, str) or not repository or not source_path:
    raise SystemExit("approved package source is incomplete")
print(f"{repository}@{revision}//{source_path}")
PY
)"
    "$@" "$AGENTPLUGINS" add "$source" --target "$client" --format json \
      >"$evidence/add/$plugin.json"
    "$@" "$AGENTPLUGINS" doctor --format json \
      >"$evidence/post-doctor/$plugin.json"
    "$@" "$AGENTPLUGINS" info "$plugin" --target "$client" --format json \
      >"$evidence/info/$plugin.json"
  done
done
```

The Node proxy flag is inert for non-Node components. For Chrome DevTools it
makes the digest-bound Node runtime inherit the same loopback-only proxy
boundary as the manager and reviewed client; it does not add another network
route or executable search path.

Stop unless the pre-add doctor file is structured successful JSON, names the
intended target as the sole detected client, and contains no other recognized
client. Stop unless every add
file is the exact successful structured 0.1.18 envelope, its top-level source
and revision reproduce the approved canonical argument, and the one requested
target has status `success`, `completed`, or `external_completed`, with no
failed target or incomplete, warning, error, cancellation, audit, event, or
conflicting lifecycle state at any depth.

The hosted probe established all five exact canonical sources install for
Codex and materialize for Cursor. A separate disposable Cursor 2026.08.25
capture proved Agent Code Navigator skill use and Context7 MCP runtime against
the current full JSONL lifecycle; it is capability evidence, not the final
five-package Cursor row. Kiro must be logged in before attempting any Power/MCP
activation. An unauthenticated Kiro probe mutated its seed and then
failed four MCP activations; that is partial state, not sealable evidence. On
any post-mutation failure, stop, retain the sanitized failure record, log in,
remove or repair the partial registrations using the pinned client and manager,
rerun doctor/add/info for all five sources, and seal only after the repaired
native state is complete. Complete each returned human `next_action` in the
exact pinned client using only its seed and disposable identity, then verify all
five native registrations. If activation cannot be completed on this Linux
host, that profile is inconclusive and must not be provisioned or represented
as reconciled. The validated Kiro runtime grammar is ACP protocol v1 from Kiro
CLI 2.20.0, invoked only as `kiro-cli acp --agent-engine v3 --auth-method cli`.
The protected `kiro-cli` digest is
`14d835aff3772afb9ffb71e395b433df516c091dea8c43daef46e7cb66368358` and
the required protected `kiro-cli-chat` companion digest is
`59f47eb75928fa158df1cea31382cb39a4eb0d8ec7afbcfc4c6e75693d35163e`.
The adapter sends `session/new` with `mcpServers: []`; Kiro must load the sealed
native `~/.kiro/settings/mcp.json` itself. The checked-in file is a sanitized
summary of the observed multi-tool catalog, permission, tool-result, marker,
and terminal shapes; it is not an ordered raw ACP capture. Agent Code Navigator is
a separate capability: Kiro must discover and disclose the sealed
`code-tool-router/SKILL.md`, run the default `grep_search` tool with the exact
hidden-marker query, and return only the marker found in tool output. Prompt
echo and MCP discovery do not count as skill runtime. These capability probes
are not the final five-plugin-by-three-client launch matrix. Do not claim
15/15/PASS until the external live matrix supplies all 15 required results.

After human activation, repeat post-add doctor without a source operand and
info with 0.1.18's installed identity syntax: the installed plugin name plus
its exact `--target`. There is no receipt-export or automatic client-activation
operation in agentplugins 0.1.18:

```sh
for client in codex cursor kiro; do
  seed="/root/profile-seeds/$client"
  evidence="/root/profile-seed-evidence/$client"
  client_path="/root/profile-seed-path/$client"
  set -- env HOME="$seed" XDG_CONFIG_HOME="$seed/.config" \
    XDG_CACHE_HOME="$seed/.cache" AGENTPLUGINS_HOME="$seed/.agentplugins" PATH="$client_path"
  if [ "$client" = codex ]; then set -- "$@" CODEX_HOME="$seed/.codex"; fi
  for plugin in agent-code-navigator context7 cloudflare-docs chrome-devtools notion; do
    "$@" "$AGENTPLUGINS" doctor --format json \
      >"$evidence/post-doctor/$plugin.json"
    "$@" "$AGENTPLUGINS" info "$plugin" --target "$client" --format json \
      >"$evidence/info/$plugin.json"
  done
done
```

Freeze `/root/uap-observer-matrix.json` first as an object containing only the
final `matrix` array, root-owned mode `0400`. For each client, create
`/root/native-config-$client.json` as a JSON object
mapping each exact hero name to the profile-relative regular file the pinned
client actually reads for that hero. Do not guess paths, hand-write tuple
receipts, or treat name/status output as tuple binding. The repository sealer
checks each manager record against the approved matrix tuple and each native
config's containment, ownership, mode, link count, and SHA-256. It emits the
manager receipt and immutable native projection consumed by the adapter:

```sh
install -d -o root -g root -m 0700 /root/projection-digests
for client in codex cursor kiro; do
  python3 "$SOURCE_ROOT/deploy/uap-observer-seal-profile.py" \
    --client "$client" --root-owned-seed "/root/profile-seeds/$client" \
    --matrix-file /root/uap-observer-matrix.json \
    --manager-add-directory "/root/profile-seed-evidence/$client/add" \
    --manager-info-directory "/root/profile-seed-evidence/$client/info" \
    --post-doctor-directory "/root/profile-seed-evidence/$client/post-doctor" \
    --native-config-map "/root/native-config-$client.json" --digest-only \
    >"/root/projection-digests/$client.sha256"
done
PYTHONPATH="$SOURCE_ROOT" python3 - /root/uap-observer-adapter-config.template.json \
  /root/uap-observer-adapter-config.json /root/projection-digests \
  "$SOURCE_ROOT/deploy/uap-observer-adapter-config.schema.json" <<'PY'
import json, math, os, pathlib, re, sys, tempfile
import jsonschema
source, target, digests, schema_path = map(pathlib.Path, sys.argv[1:])
def strict_load(path):
    def pairs(items):
        value, folded = {}, set()
        for key, child in items:
            normalized = key.casefold()
            if key in value or normalized in folded:
                raise SystemExit(f"{path}: duplicate or case-confusable JSON member")
            value[key] = child; folded.add(normalized)
        return value
    def constant(value): raise SystemExit(f"{path}: non-finite JSON number {value}")
    def finite(value):
        decoded = float(value)
        if not math.isfinite(decoded): constant(value)
        return decoded
    return json.loads(path.read_bytes(), object_pairs_hook=pairs,
                      parse_constant=constant, parse_float=finite)
value = strict_load(source)
schema = strict_load(schema_path)
clients = value.get("clients") if isinstance(value, dict) else None
if (type(value.get("schema_version")) is not int or value["schema_version"] != 1
    or not isinstance(clients, dict) or set(clients) != {"codex", "cursor", "kiro"}
    or any(not isinstance(record, dict) or "native_projection" in record
           for record in clients.values())):
    raise SystemExit("reviewed pre-projection template is not the exact unprojected schema")
for client in ("codex", "cursor", "kiro"):
    digest = (digests / f"{client}.sha256").read_text(encoding="ascii").strip()
    if re.fullmatch(r"sha256:[a-f0-9]{64}", digest) is None:
        raise SystemExit(f"{client}: projection digest is malformed")
    value["clients"][client]["native_projection"] = {
        "path": f"/var/lib/uap-observer/proofs/{client}/native-projection.json",
        "sha256": digest,
    }
jsonschema.validate(value, schema)
body = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
fd, temporary = tempfile.mkstemp(prefix=".adapter.", dir=target.parent)
try:
    os.fchmod(fd, 0o400)
    view = memoryview(body)
    while view:
        written = os.write(fd, view)
        if written <= 0: raise OSError("short adapter-config write")
        view = view[written:]
    os.fsync(fd)
finally:
    os.close(fd)
os.replace(temporary, target)
directory = os.open(target.parent, os.O_RDONLY | os.O_DIRECTORY)
try: os.fsync(directory)
finally: os.close(directory)
PY
python3 -m jsonschema -i /root/uap-observer-adapter-config.json \
  "$SOURCE_ROOT/deploy/uap-observer-adapter-config.schema.json"
for client in codex cursor kiro; do
  sealed_digest="$(python3 "$SOURCE_ROOT/deploy/uap-observer-seal-profile.py" \
    --client "$client" --root-owned-seed "/root/profile-seeds/$client" \
    --matrix-file /root/uap-observer-matrix.json \
    --adapter-config /root/uap-observer-adapter-config.json \
    --manager-add-directory "/root/profile-seed-evidence/$client/add" \
    --manager-info-directory "/root/profile-seed-evidence/$client/info" \
    --post-doctor-directory "/root/profile-seed-evidence/$client/post-doctor" \
    --native-config-map "/root/native-config-$client.json")"
  test "$sealed_digest" = "$(cat "/root/projection-digests/$client.sha256")"
done
```

Real 0.1.18 info JSON reports `data.source` as `repository//path` (without the
revision in that display field). Its `package_revision` has exactly `version`,
`resolved_revision`, `tree_digest`, and `manifest_digest`; it does not repeat
distribution metadata. The sealer checks those fields and the displayed source
against the approved tuple. Distribution ID, kind, and release sequence come
only from that approved tuple and are copied into the sealed receipt.
After all three projections are sealed, remove the temporary
`/root/profile-seed-path` tree; it is not part of a profile seed or deployment
input.

The final adapter config names each projection's eventual protected path under
`/var/lib/uap-observer/proofs/<client>` and exact SHA-256. This is intentionally
two phase: `--digest-only` consumes only the frozen matrix/add/info/post-doctor,
native-map, and seed inputs; insert those printed digests into the final config,
schema-validate and freeze that config; then run the writing phase from unchanged
inputs against the final config and require the same digest. A
mismatch requires correction and re-review; never rewrite generated bytes to
make a digest match. Provisioning moves `.uap-observer-proof` out of the copied
profile, installs its root-owned receipt, projection, and authoritative
native-config snapshots as client-group-readable `0440` files below a
root-owned non-writable parent,
and leaves no proof material writable or renameable by the client.

The installed profiles are isolated at
`/var/lib/uap-observer/profiles/{codex,cursor,kiro}`. Provisioning changes the
profile root and every directory on an active native-config path to
`root:<client-group>` mode `0510`, and every active native config to root-owned
mode `0440`. Those directories are the non-renameable boundary. Pre-existing
authentication and state files and their dedicated subdirectories stay
client-owned (`0600`/`0700`); no other seed content may be client-writable. The
runner mounts the profile tree read-only and remounts only the reviewed
per-client `.auth` and `.state` roots writable through optional exact binds.
All `.config`, client-native config, cache, and other profile paths remain
read-only in the deployed namespace. The helper refuses a non-empty
destination. Do not use any seed exported from a Mac or normal workstation,
and never bake credentials into the source checkout or protected input tree.

The adapter config's `egress_hosts` is the canonical reviewed contract covering
every exact MCP, observer, GitHub, client, and provider FQDN needed by this
deployment. The operator must provide the proxy allowlist as an immutable deployment input,
not derive it during installation. It is canonical JSON with exactly
`schema_version` set to `1` and a non-empty `hosts` array of bytewise-sorted,
unique, exact lowercase ASCII FQDNs. The JSON has no extra keys or insignificant
whitespace and ends with one LF. Hosts have no ports, URLs, IP literals, leading
dots, or wildcards. Its `hosts` array must equal adapter `egress_hosts` byte for
byte. Every matrix endpoint, ChatGPT MCP endpoint, observer JWKS/API endpoint,
and GitHub release host must be included; the installer checks that required
subset and exact allowlist equality. Provider hosts can therefore be added
without falsifying matrix endpoints. Review the current
provider-owned host list for Codex, Cursor, Kiro, GitHub, and the configured
ChatGPT MCP before approving it; redirects and CDN hosts are separate exact
entries and are never implicitly trusted. Record this exact tuple in the change
ticket:

```text
EGRESS_ALLOWLIST=/root/uap-observer-egress-allowlist.json
EGRESS_SHA256=sha256:<64-lowercase-hex>
EGRESS_ALLOWLIST_MODE=0:0:644:1
```

Validate it only after checking its recorded digest, ownership, mode, link
count, and canonical form. The repository socket listens only on
`127.0.0.2:8766`; the repository service reads the closure copy of that exact
input, fails closed on malformed or unlisted CONNECT targets, resolves names
itself, and provides no transparent or direct-fallback mode.

```sh
test "$(stat -c '%u:%g:%a:%h' "$EGRESS_ALLOWLIST")" = \
  "$EGRESS_ALLOWLIST_MODE"
test "sha256:$(sha256sum "$EGRESS_ALLOWLIST" | cut -d' ' -f1)" = \
  "$EGRESS_SHA256"
PYTHONDONTWRITEBYTECODE=1 python3 -B \
  "$SOURCE_ROOT/deploy/uap-observer-egress-proxy.py" \
  --config "$EGRESS_ALLOWLIST" --validate-config
```

For each exact pinned client, perform the earlier real-request proxy preflight
with `HTTP_PROXY=http://127.0.0.2:8766`,
`HTTPS_PROXY=http://127.0.0.2:8766`, `ALL_PROXY=http://127.0.0.2:8766`, and
an explicitly empty `NO_PROXY`. Repeat once with the proxy stopped while the
operator-managed host firewall rejects the process's direct egress; the request
must fail. A client passes only if the running-proxy request appears
in sanitized proxy connection metadata and the stopped-proxy request cannot use
a direct path. Every observed provider hostname must already be an exact host in
the final configured set. On any discrepancy, stop, revise and re-review the
configs and allowlist together, re-digest them, and repeat every preflight;
never add an allowlist-only exception or direct fallback to make it pass.

## 2. Identity, network, and GitHub

Generate a new root Ed25519 key on this host. Store canonical base64 of its raw
32-byte private value at `/etc/uap-observer-ed25519.key`, owned by root with
mode `0600` and one hard link. Put canonical base64 of the corresponding raw
32-byte public value in `deploy/uap-observer.json`'s
`public_key_base64`; keep key ID `uap-stable-launch-2026-08`. Publish only this
non-secret identity tuple:

```sh
umask 077
python3 - <<'PY'
import base64, os
from pathlib import Path
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

private = Ed25519PrivateKey.generate()
key_path = Path("/etc/uap-observer-ed25519.key")
with key_path.open("xb") as stream:
    stream.write(base64.b64encode(private.private_bytes_raw()) + b"\n")
    stream.flush()
    os.fsync(stream.fileno())
os.chmod(key_path, 0o600)
public = base64.b64encode(private.public_key().public_bytes_raw()).decode()
Path("/root/uap-observer-public-key.txt").write_text(public + "\n")
PY
PUBLIC_KEY="$(tr -d '\n' </root/uap-observer-public-key.txt)"
jq --arg public_key "$PUBLIC_KEY" \
  '.public_key_base64 = $public_key' \
  "$SOURCE_ROOT/deploy/uap-observer.json" >/root/uap-observer.json
chmod 0644 /root/uap-observer.json
test "$(stat -c '%u:%g:%a:%h' /etc/uap-observer-ed25519.key)" = "0:0:600:1"
```

Run this only where pinned `cryptography` is already available, and never print
or copy the private value. Record the public file's value and key ID together.

```text
STABLE_LAUNCH_OBSERVER_KEY_ID=uap-stable-launch-2026-08
STABLE_LAUNCH_OBSERVER_ED25519_PUBLIC_KEY=<44-character-base64-public-key>
```

In repository variables, set those two values. In the protected
`stable-launch-e2e` environment, set
`STABLE_LAUNCH_OBSERVER_ENDPOINT=https://<observer-fqdn>/v1/stable-launch/observe`
and require the intended human reviewer. Do not create a GitHub secret for any
of these non-secret values. Confirm the `directory-publication` protections and
that the reusable protected job has `id-token: write`; the observer policy
accepts only `push`, `schedule`, and `workflow_dispatch` from the exact main-ref
workflow identity.

Replace `observer.example.invalid` in a copy of `deploy/Caddyfile` with the
public observer FQDN. Create public A (and AAAA only when IPv6 is actually
routed) DNS records before starting Caddy. In the operator-managed host
firewall, permit inbound TCP 80/443, restrict SSH to the administration source,
permit the proxy's required outbound DNS/HTTPS, and reject every direct Internet
path from observer and client processes. Do not expose port 8765 or 8766. The
repository does not install UID-, cgroup-, or service-identity firewall rules,
so do not claim that a service account alone provides this isolation: select,
document, and test concrete firewall mechanics for the host. Separately,
installer-created systemd drop-ins apply `IPAddressDeny=any` with only the
required loopback allowances to the observer and runner services; those unit
controls do not replace the host firewall. Cloud images with
`manage_etc_hosts: true` overwrite `/etc/hosts`, so do not use a local hosts
entry as persistent DNS configuration.

## 3. Install and start

First compute the three `sha256:` arguments from the final adapter, observer,
and Caddy configs. As root, run the reviewed installer exactly once:

```sh
ADAPTER_SHA256="sha256:$(sha256sum /root/uap-observer-adapter-config.json | cut -d' ' -f1)"
OBSERVER_SHA256="sha256:$(sha256sum /root/uap-observer.json | cut -d' ' -f1)"
CADDY_SHA256="sha256:$(sha256sum /root/Caddyfile | cut -d' ' -f1)"
"$SOURCE_ROOT/deploy/uap-observer-install.sh" "$SOURCE_ROOT" \
  /root/uap-observer-adapter-config.json "$ADAPTER_SHA256" \
  /root/uap-observer.json "$OBSERVER_SHA256" \
  /root/caddy_2.11.4_linux_amd64.tar.gz \
  /root/Caddyfile "$CADDY_SHA256" \
  "$EGRESS_ALLOWLIST" "$EGRESS_SHA256"
```

The installer does not enable or start anything. After it succeeds, provision
each isolated profile with the installed helper's two-pass digest check:

```sh
for client in codex cursor kiro; do
  digest="$(/opt/uap-observer-current/libexec/uap-observer-provision-profile \
    --client "$client" --root-owned-seed "/root/profile-seeds/$client" \
    --seed-digest show)"
  /opt/uap-observer-current/libexec/uap-observer-provision-profile \
    --client "$client" --root-owned-seed "/root/profile-seeds/$client" \
    --seed-digest "$digest"
done
```

After the signing key, isolated profiles, firewall rules, and immutable proxy
inputs exist, start the proxy socket first and prove its listener and service
health before starting the four repository units:

```sh
systemctl daemon-reload
systemctl enable --now uap-observer-egress-proxy.socket
systemctl is-active uap-observer-egress-proxy.socket
ss -lntp | grep -E '127\.0\.0\.2:8766([[:space:]]|$)'
! ss -lntp | grep -E '(0\.0\.0\.0|\[::\]|:::):8766([[:space:]]|$)'
curl --silent --show-error --output /dev/null --connect-timeout 10 --max-time 30 \
  --proxy http://127.0.0.2:8766 "https://<reviewed-allowlisted-health-fqdn>/"
systemctl is-active uap-observer-egress-proxy.service
systemctl enable --now uap-observer-runner.socket \
  uap-observer-signer.service uap-observer.service \
  uap-observer-caddy.service
systemctl is-active uap-observer-runner.socket uap-observer-signer.service \
  uap-observer.service uap-observer-caddy.service
systemctl --failed
curl --fail --silent --show-error --output /dev/null \
  "https://<observer-fqdn>/v1/stable-launch/observe" || test "$?" -eq 22
ss -lnt | grep -E '127\.0\.0\.1:8765|127\.0\.0\.2:8766|:80 |:443 '
! ss -lnt | grep -E '(0\.0\.0\.0|\[::\]|:::):876(5|6)([[:space:]]|$)'
```

The proxy probe accepts any origin HTTP status; it deterministically checks the
CONNECT tunnel and TLS without requiring an arbitrary root path to return 2xx.
The unauthenticated observer probe is expected to be rejected; it proves only
TLS and routing. Health is complete only when the proxy socket/service and all
four repository units are active, `127.0.0.1:8765` and `127.0.0.2:8766` are the
only 8765/8766 listeners, Caddy owns the public listener, direct client egress
is rejected, and a protected workflow gets a signed bundle whose public key
and key ID match the repository variables.

## 4. Operate one evidence run

Before approving the protected job, obtain its challenge, run ID/attempt,
catalog SHA, canonical request digest, scenario-contract digest, and fresh
pseudonymous identity/root IDs through the release-coordinator channel. Create
the root consent record with `/opt/uap-observer-current/libexec/uap-observer-attest-consent`;
choose only `read-only` or `synthetic` and `fresh-dedicated-identity` (or `none`)
as true for this disposable run.

```sh
/opt/uap-observer-current/libexec/uap-observer-attest-consent \
  --challenge <64-hex> --run-id <decimal> --run-attempt <decimal> \
  --catalog-sha <40-hex> --request-digest sha256:<64-hex> \
  --scenario-contract-digest sha256:<64-hex> \
  --identity-id <fresh-pseudonymous-64-hex> \
  --logical-root-id <fresh-pseudonymous-64-hex> \
  --operation-mode synthetic --auth-origin fresh-dedicated-identity
```

For ChatGPT, use only the configured dedicated test app and the public
Cloudflare Docs MCP. After visually confirming consent, UI activation, the
read-only runtime result, no secrets, and no real-project access, run
`/opt/uap-observer-current/libexec/uap-observer-attest-chatgpt` with the exact challenge,
run/attempt, app ID, and request digest. Create this human attestation no more
than **five minutes before** the observer will consume it; never pre-stage it.
Pass `yes` only for facts the human actually observed. Both helpers use
exclusive, challenge-specific files and intentionally accept no free-form text.

```sh
/opt/uap-observer-current/libexec/uap-observer-attest-chatgpt \
  --challenge <64-hex> --run-id <decimal> --run-attempt <decimal> \
  --app-id plugin_asdk_app_<32-hex> --request-digest sha256:<64-hex> \
  --consent yes --ui-activation yes --runtime-observed yes \
  --read-only yes --no-secrets yes --no-real-project yes
```

Approve the `stable-launch-e2e` environment only after consent and the external
PR evidence are ready. Preserve the GitHub run URL/attempt, immutable artifact
names and digests, observer key ID/public key, unit status, installed closure
identity, and sanitized failure/status output. Never archive the private key,
profile trees, auth material, raw client output, or absolute local paths as
evidence.

## 5. Failure and rollback

On a failed first install, keep services stopped and retain the installer output
and recovery journal; rerun the same reviewed command to complete its built-in
recovery. On a runtime or proxy failure, record sanitized unit status, remove
the host from DNS/load balancing, stop Caddy and the observer before stopping
the proxy, and keep the failed GitHub gate red:

```sh
systemctl stop uap-observer-caddy.service uap-observer.service \
  uap-observer-runner.socket uap-observer-runner.service \
  uap-observer-signer.service
systemctl stop uap-observer-egress-proxy.socket \
  uap-observer-egress-proxy.service
! ss -lnt | grep -E ':(80|443|8765|8766)([[:space:]]|$)'
```

Do not bypass OIDC, reuse consent/attestations, edit the installed closure or
proxy allowlist in place, relax the no-direct-egress firewall policy, or
substitute fixture evidence. Proxy rollback means restore the last independently
reviewed source commit and allowlist tuple (including its recorded digest and
mode) on a new disposable host; do not keep this host serving while changing
those inputs.

The current installer supports a fresh host only. If
`/opt/uap-observer-current` exists, it accepts only byte-identical inputs and
verifies that closure; it does not upgrade, switch to another closure, or
provide a post-deployment downgrade. Rollback for a changed release is therefore
to return DNS to the last independently healthy host (if one exists) and dispose
this test host. Build and verify another fresh disposable host for every change.
