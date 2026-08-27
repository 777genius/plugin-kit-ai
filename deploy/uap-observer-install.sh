#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
install_lib="$script_dir/uap-observer-install-lib.sh"
test "$(sha256sum "$install_lib" | cut -d' ' -f1)" = 8520fc8342af4393ef788f11bf006b90e830b3794f79ef4db35190f578f4086a
. "$install_lib"

if [ "$(id -u)" -ne 0 ]; then
  echo "installer must run as root" >&2
  exit 1
fi

usage='usage: uap-observer-install.sh SOURCE_ROOT ADAPTER_CONFIG ADAPTER_SHA256 OBSERVER_CONFIG OBSERVER_SHA256 CADDY_2.11.4_LINUX_AMD64_ARCHIVE CADDY_CONFIG CADDY_CONFIG_SHA256 EGRESS_ALLOWLIST EGRESS_ALLOWLIST_SHA256'
stage_root=/opt/uap-observer-source.new
runtime_manifest_digest=a09f5be915e56ce482b45f5248d9164a04005b7bbb741a8ce51b84a179d3cec7
caddy_archive_digest=527fbf917c39189a1e3b31d34fa955601680b2d5c8055d2a87b8b9588dec7bb9
closure_digest=
closure_stage=
closure_final=
install -d -o root -g root -m 0755 /run/lock
exec 9>/run/lock/uap-observer-install.lock
flock -n 9 || { echo "another observer install is active" >&2; exit 1; }

# Recovery is deliberately before even requiring, opening, hashing, or
# otherwise validating caller-controlled arguments.  A retry after power loss
# must be able to restore the fixed system state with every input unavailable.
recover_observer_install "$stage_root" /opt/uap-observer-closures /opt/uap-observer-current /etc/systemd/system systemctl

untrusted_source_root=${1:-/opt/uap-observer}
untrusted_adapter_config=${2:?$usage}
adapter_config_digest=${3:?$usage}
untrusted_observer_config=${4:?$usage}
observer_config_digest=${5:?$usage}
untrusted_caddy_archive=${6:?$usage}
untrusted_caddy_config=${7:?$usage}
caddy_config_digest=${8:?$usage}
untrusted_egress_allowlist=${9:?$usage}
egress_allowlist_digest=${10:?$usage}
install_identity=$(observer_install_input_identity \
  "$untrusted_source_root" "$runtime_manifest_digest" \
  "$untrusted_adapter_config" "$adapter_config_digest" \
  "$untrusted_observer_config" "$observer_config_digest" \
  "$untrusted_caddy_archive" "$caddy_archive_digest" \
  "$untrusted_caddy_config" "$caddy_config_digest" \
  "$untrusted_egress_allowlist" "$egress_allowlist_digest")
if [ -e /opt/uap-observer-current ] || [ -L /opt/uap-observer-current ]; then
  observer_validate_no_partial_paths
  observer_validate_completed_closure /opt/uap-observer-closures /opt/uap-observer-current "$install_identity"
  installed_target=$(observer_read_symlink_neutral /opt/uap-observer-current)
  installed_closure="/opt/$installed_target"
  observer_validate_installed_closure_sources "$installed_closure" "$untrusted_source_root" \
    "$untrusted_adapter_config" "$untrusted_observer_config" "$untrusted_caddy_config" "$untrusted_egress_allowlist" \
    8094eb1172f889159782a913755911d6bba5bd40ef0bff9831a002e590879b28 \
    a431e83871091f3ac4908f115f9f7f51150cce4f060d784dd2ae609d1dede1e5 \
    b7105518e3ed1c0761f232e44fc09345535533c9cb0abf0e12809416c7ac64d9
  observer_validate_installed_accounts_and_state "$installed_closure"
  observer_validate_protected_inputs "$installed_closure"
  echo "observer files already installed; verified identical immutable closure"
  exit 0
fi
observer_validate_first_install_closures_root /opt/uap-observer-closures
# Every authoritative temporary parent exists before the cleanup journal is
# created, so recovery can fsync the complete parent inventory on any failure.
install -d -o root -g root -m 0755 /opt/uap-observer-closures /usr/local/libexec /usr/local/bin /etc/caddy
test ! -e "$stage_root"
install -d -o root -g root -m 0700 "$stage_root"
cleanup() {
  status=${1:-1}
  trap - EXIT HUP INT TERM
  set +e
  recover_observer_install "$stage_root" /opt/uap-observer-closures /opt/uap-observer-current /etc/systemd/system systemctl
  recovery_status=$?
  set -e
  if [ "$recovery_status" -ne 0 ]; then
    echo "observer rollback incomplete; durable recovery journal retained" >&2
    if [ "$status" -eq 0 ]; then status=$recovery_status; fi
  fi
  exit "$status"
}
trap 'cleanup $?' EXIT
trap 'exit 1' HUP INT TERM

install -D -o root -g root -m 0444 "$untrusted_source_root/deploy/uap-observer-runtime.sha256" "$stage_root/deploy/uap-observer-runtime.sha256"
test "$(sha256sum "$stage_root/deploy/uap-observer-runtime.sha256" | cut -d' ' -f1)" = "$runtime_manifest_digest"
while read -r expected relative extra; do
  test -z "${extra:-}"
  case "$expected:$relative" in
    [0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]*:[A-Za-z0-9._-]*) ;;
    *) echo "invalid runtime closure entry" >&2; exit 1 ;;
  esac
  case "/$relative/" in */../*|*/./*|//* ) echo "unsafe runtime closure path" >&2; exit 1 ;; esac
  test -f "$untrusted_source_root/$relative"
  test ! -L "$untrusted_source_root/$relative"
  install -D -o root -g root -m 0444 "$untrusted_source_root/$relative" "$stage_root/$relative"
done < "$stage_root/deploy/uap-observer-runtime.sha256"
install -o root -g root -m 0400 "$untrusted_adapter_config" "$stage_root/adapter-config.json"
install -o root -g root -m 0644 "$untrusted_observer_config" "$stage_root/observer-config.json"
install -o root -g root -m 0644 "$untrusted_egress_allowlist" "$stage_root/egress-allowlist.json"
install -o root -g root -m 0400 "$untrusted_caddy_archive" "$stage_root/caddy_2.11.4_linux_amd64.tar.gz"
test "$(sha256sum "$stage_root/caddy_2.11.4_linux_amd64.tar.gz" | cut -d' ' -f1)" = "527fbf917c39189a1e3b31d34fa955601680b2d5c8055d2a87b8b9588dec7bb9"
test "$(tar -tzf "$stage_root/caddy_2.11.4_linux_amd64.tar.gz" | sort | tr '\n' ' ')" = "LICENSE README.md caddy "
tar -xOf "$stage_root/caddy_2.11.4_linux_amd64.tar.gz" caddy > "$stage_root/caddy"
chown root:root "$stage_root/caddy"
chmod 0500 "$stage_root/caddy"
install -o root -g root -m 0400 "$untrusted_caddy_config" "$stage_root/Caddyfile"
(cd "$stage_root" && sha256sum -c deploy/uap-observer-runtime.sha256)
source_root=$stage_root
adapter_config=$stage_root/adapter-config.json
observer_config=$stage_root/observer-config.json
caddy_binary=$stage_root/caddy
caddy_config=$stage_root/Caddyfile
runner_source="$source_root/observer/fixed_runner.py"
adapter_source="$source_root/observer/fixed_adapters.py"
egress_proxy_source="$source_root/deploy/uap-observer-egress-proxy.py"
runner_digest=8094eb1172f889159782a913755911d6bba5bd40ef0bff9831a002e590879b28
adapter_digest=a431e83871091f3ac4908f115f9f7f51150cce4f060d784dd2ae609d1dede1e5
caddy_digest=b7105518e3ed1c0761f232e44fc09345535533c9cb0abf0e12809416c7ac64d9

test -f "$runner_source"
test "$(sha256sum "$runner_source" | cut -d' ' -f1)" = "$runner_digest"
test "$(sha256sum "$adapter_source" | cut -d' ' -f1)" = "$adapter_digest"
test "sha256:$(sha256sum "$adapter_config" | cut -d' ' -f1)" = "$adapter_config_digest"
test "sha256:$(sha256sum "$observer_config" | cut -d' ' -f1)" = "$observer_config_digest"
test "$(sha256sum "$caddy_binary" | cut -d' ' -f1)" = "$caddy_digest"
test "$(sha256sum "$source_root/caddy_2.11.4_linux_amd64.tar.gz" | cut -d' ' -f1)" = "$caddy_archive_digest"
test "$("$caddy_binary" version | awk '{print $1}')" = "v2.11.4"
test "sha256:$(sha256sum "$caddy_config" | cut -d' ' -f1)" = "$caddy_config_digest"
test "sha256:$(sha256sum "$stage_root/egress-allowlist.json" | cut -d' ' -f1)" = "$egress_allowlist_digest"
test "$(uname -s):$(uname -m)" = "Linux:x86_64"
test "$(stat -c '%u:%a' "$adapter_config")" = "0:400"
! grep -q REPLACE_WITH "$observer_config"
! grep -q observer.example.invalid "$caddy_config"
(cd "$source_root" && sha256sum -c deploy/uap-observer-runtime.sha256)

getent group uap-observer-signer-ipc >/dev/null || groupadd --system uap-observer-signer-ipc
getent group uap-observer-runner-ipc >/dev/null || groupadd --system uap-observer-runner-ipc
getent group uap-observer-adapter-config >/dev/null || groupadd --system uap-observer-adapter-config
install -d -o root -g root -m 0755 /var/empty
for identity in codex cursor kiro control; do
  getent group "uap-observer-$identity" >/dev/null || groupadd --system "uap-observer-$identity"
  id "uap-observer-$identity" >/dev/null 2>&1 || useradd --system --gid "uap-observer-$identity" --home-dir "/var/empty/uap-observer-$identity" --shell /usr/sbin/nologin "uap-observer-$identity"
  usermod -a -G uap-observer-adapter-config "uap-observer-$identity"
  install -d -o "uap-observer-$identity" -g "uap-observer-$identity" -m 0700 "/var/empty/uap-observer-$identity"
done
getent group uap-observer >/dev/null || groupadd --system uap-observer
getent group uap-observer-egress >/dev/null || groupadd --system uap-observer-egress
getent group caddy >/dev/null || groupadd --system caddy
id caddy >/dev/null 2>&1 || useradd --system --gid caddy --home-dir /var/lib/caddy --shell /usr/sbin/nologin caddy
id uap-observer >/dev/null 2>&1 || useradd --system --gid uap-observer --home-dir /nonexistent --shell /usr/sbin/nologin uap-observer
id uap-observer-egress >/dev/null 2>&1 || useradd --system --gid uap-observer-egress --home-dir /nonexistent --shell /usr/sbin/nologin uap-observer-egress
usermod -a -G uap-observer-signer-ipc,uap-observer-runner-ipc uap-observer
PYTHONDONTWRITEBYTECODE=1 PYTHONPATH="$source_root" python3 -B -c 'from observer.fixed_runner import reviewed_service_identities; reviewed_service_identities()'
PYTHONDONTWRITEBYTECODE=1 python3 -B "$egress_proxy_source" --config "$stage_root/egress-allowlist.json" --validate-config

identity_uids=
identity_gids=
for identity in codex cursor kiro control; do
  account="uap-observer-$identity"
  uid=$(id -u "$account")
  gid=$(id -g "$account")
  test "$uid" -ne 0
  test "$(getent passwd "$account" | cut -d: -f6)" = "/var/empty/$account"
  shell=$(getent passwd "$account" | cut -d: -f7)
  case "$shell" in /usr/sbin/nologin|/sbin/nologin|/bin/false) ;; *) echo "adapter login shell differs" >&2; exit 1;; esac
  test "$(getent group "$account" | cut -d: -f3)" = "$gid"
  test "$(stat -c '%u:%g:%a' "/var/empty/$account")" = "$uid:$gid:700"
  groups=$(id -G "$account")
  config_gid=$(getent group uap-observer-adapter-config | cut -d: -f3)
  test "$(printf '%s\n' $groups | sort -n | tr '\n' ' ')" = "$(printf '%s\n' "$gid" "$config_gid" | sort -n | uniq | tr '\n' ' ')"
  case " $identity_uids " in *" $uid "*) echo "adapter UIDs are not distinct" >&2; exit 1;; esac
  case " $identity_gids " in *" $gid "*) echo "adapter GIDs are not distinct" >&2; exit 1;; esac
  identity_uids="$identity_uids $uid"
  identity_gids="$identity_gids $gid"
done

for unit in uap-observer.service uap-observer-signer.service uap-observer-runner.service uap-observer-runner.socket uap-observer-caddy.service uap-observer-egress-proxy.service uap-observer-egress-proxy.socket; do
  if systemctl is-active --quiet "$unit"; then
    echo "stop observer services before installing a new immutable closure" >&2
    exit 1
  fi
done

install -d -o root -g root -m 0711 /var/lib/uap-observer
install -d -o uap-observer -g uap-observer -m 0700 /var/lib/uap-observer/state
install -d -o root -g root -m 0711 /var/lib/uap-observer/jobs /var/lib/uap-observer/workspaces /var/lib/uap-observer/profiles /var/lib/uap-observer/proofs
for client in codex cursor kiro; do
  install -d -o "uap-observer-$client" -g "uap-observer-$client" -m 0700 "/var/lib/uap-observer/profiles/$client" "/var/lib/uap-observer/workspaces/$client"
done
install -d -o root -g root -m 0755 /var/lib/uap-observer-human
install -d -o root -g uap-observer-control -m 0750 /var/lib/uap-observer-human/pending
install -d -o root -g root -m 0700 /var/lib/uap-observer-human/consumed
install -d -o root -g root -m 0700 /var/lib/uap-observer-human/reserved
install -d -o root -g root -m 0755 /var/lib/uap-observer-consent
install -d -o root -g uap-observer-adapter-config -m 0750 /var/lib/uap-observer-consent/pending
install -d -o root -g root -m 0700 /var/lib/uap-observer-consent/consumed
install -d -o root -g root -m 0700 /var/lib/uap-observer-consent/reserved
install -d -o caddy -g caddy -m 0700 /var/lib/caddy /var/log/caddy
observer_validate_installed_accounts_and_state "$source_root"

test ! -e /opt/uap-observer-venv.new
PYTHONDONTWRITEBYTECODE=1 python3 -B -m venv /opt/uap-observer-venv.new
PYTHONDONTWRITEBYTECODE=1 /opt/uap-observer-venv.new/bin/python -B -m pip install --no-compile --require-hashes --no-deps -r "$source_root/observer/requirements.lock"
PYTHONDONTWRITEBYTECODE=1 /opt/uap-observer-venv.new/bin/python -B -m jsonschema -i "$adapter_config" "$source_root/deploy/uap-observer-adapter-config.schema.json"
PYTHONDONTWRITEBYTECODE=1 PYTHONPATH="$source_root" /opt/uap-observer-venv.new/bin/python -B -c 'import sys; from observer.fixed_adapters import strict_json_loads,validate_config; validate_config(strict_json_loads(open(sys.argv[1], "rb").read()))' "$adapter_config"
PYTHONDONTWRITEBYTECODE=1 PYTHONPATH="$source_root" /opt/uap-observer-venv.new/bin/python -B -c 'import sys; from pathlib import Path; from observer.config import Config; Config.load(Path(sys.argv[1]))' "$observer_config"
observer_remove_python_bytecode /opt/uap-observer-venv.new

# Prove that the externally reviewed allowlist is exactly the canonical set of
# hosts reachable from the fixed observer and adapter configuration. DNS is
# intentionally deferred to the gateway for every new tunnel.
PYTHONDONTWRITEBYTECODE=1 python3 -B - "$adapter_config" "$observer_config" "$stage_root/egress-allowlist.json" <<'PY'
import json,math,sys
from pathlib import Path
from urllib.parse import urlsplit
def strict(path):
    def pairs(items):
        value={}
        folded=set()
        for key, child in items:
            normalized=key.casefold()
            if key in value or normalized in folded:
                raise ValueError("duplicate or case-confusable JSON member")
            value[key]=child
            folded.add(normalized)
        return value
    def constant(value): raise ValueError("non-finite JSON number")
    def floating(value):
        decoded=float(value)
        if not math.isfinite(decoded): raise ValueError("non-finite JSON number")
        return decoded
    with open(path,"rb") as stream:
        return json.loads(stream.read(),object_pairs_hook=pairs,parse_constant=constant,parse_float=floating)
adapter=strict(sys.argv[1])
observer=strict(sys.argv[2])
urls=[item["endpoint"] for item in adapter["matrix"]]
urls += [adapter["chatgpt"]["mcp_endpoint"], observer["jwks_url"], observer["github_api_url"]]
required_hosts={urlsplit(url).hostname for url in urls} | {"github.com"}
if None in required_hosts or any(urlsplit(url).scheme != "https" or urlsplit(url).port not in (None,443) for url in urls):
    raise SystemExit("observer egress hostname is invalid")
egress_hosts=adapter.get("egress_hosts")
if not isinstance(egress_hosts,list) or not required_hosts <= set(egress_hosts):
    raise SystemExit("adapter egress_hosts omits a configured endpoint, JWKS, or GitHub host")
allowlist=strict(sys.argv[3])
if type(allowlist.get("schema_version")) is not int or allowlist.get("schema_version") != 1 or allowlist.get("hosts") != egress_hosts:
    raise SystemExit("egress allowlist differs from exact adapter egress_hosts")
PY

test ! -e /opt/uap-observer-runtime.new
install -d -o root -g root -m 0755 /opt/uap-observer-runtime.new/observer /opt/uap-observer-runtime.new/tests/e2e/schemas
for source in "$source_root"/observer/*.py; do
  install -o root -g root -m 0444 "$source" "/opt/uap-observer-runtime.new/observer/$(basename "$source")"
done
for source in "$source_root"/tests/e2e/schemas/*.schema.json; do
  install -o root -g root -m 0444 "$source" "/opt/uap-observer-runtime.new/tests/e2e/schemas/$(basename "$source")"
done
install -o root -g root -m 0555 "$source_root/deploy/uap-observer-signer.py" /opt/uap-observer-runtime.new/uap-observer-signer.py

install -o root -g root -m 0755 "$runner_source" /usr/local/libexec/uap-observer-runner.new
install -o root -g root -m 0555 "$egress_proxy_source" /usr/local/libexec/uap-observer-egress-proxy.new
test "$(sha256sum /usr/local/libexec/uap-observer-runner.new | cut -d' ' -f1)" = "$runner_digest"
install -o root -g root -m 0555 "$adapter_source" /usr/local/libexec/uap-observer-fixed-adapter.new
for name in runtime notion chatgpt consent; do
  test ! -e "/usr/local/libexec/uap-observer-adapter-$name.new"
  ln /usr/local/libexec/uap-observer-fixed-adapter.new "/usr/local/libexec/uap-observer-adapter-$name.new"
done
install -o root -g root -m 0555 "$source_root/deploy/uap-observer-attest-chatgpt.py" /usr/local/libexec/uap-observer-attest-chatgpt.new
install -o root -g root -m 0555 "$source_root/deploy/uap-observer-attest-consent.py" /usr/local/libexec/uap-observer-attest-consent.new
install -o root -g root -m 0555 "$source_root/deploy/uap-observer-provision-profile.py" /usr/local/libexec/uap-observer-provision-profile.new
install -o root -g root -m 0755 "$caddy_binary" /usr/local/bin/caddy.new
install -o root -g root -m 0644 "$observer_config" /etc/uap-observer.json.new
install -o root -g root -m 0644 "$stage_root/egress-allowlist.json" /etc/uap-observer-egress-allowlist.json.new
install -o root -g uap-observer-adapter-config -m 0640 "$adapter_config" /etc/uap-observer-adapter-config.json.new
installed_adapter_config_digest="sha256:$(sha256sum /etc/uap-observer-adapter-config.json.new | cut -d' ' -f1)"
PYTHONDONTWRITEBYTECODE=1 /opt/uap-observer-venv.new/bin/python -B - "$adapter_digest" "$installed_adapter_config_digest" <<'PY'
import json,sys
artifacts = {
    "runtime-attestations.json": "runtime", "notion-oauth-attestations.json": "notion",
    "chatgpt-cloudflare-attestation.json": "chatgpt", "consent.json": "consent",
}
value = {
    "schema_version": 1,
    "config": {"path": "/opt/uap-observer-current/etc/uap-observer-adapter-config.json", "sha256": sys.argv[2]},
    "artifacts": {
        artifact: {"path": f"/opt/uap-observer-current/libexec/uap-observer-adapter-{name}", "sha256": "sha256:" + sys.argv[1]}
        for artifact, name in artifacts.items()
    },
}
with open("/etc/uap-observer-adapters.json.new", "x", encoding="utf-8") as stream:
    json.dump(value, stream, sort_keys=True, separators=(",", ":"))
PY
chown root:root /etc/uap-observer-adapters.json.new
chmod 0644 /etc/uap-observer-adapters.json.new

install -o root -g caddy -m 0640 "$caddy_config" /etc/caddy/Caddyfile.new
systemd_stage="$stage_root/systemd"
install -d -o root -g root -m 0755 "$systemd_stage/uap-observer.service.d" "$systemd_stage/uap-observer-runner.service.d"
for unit in uap-observer.service uap-observer-signer.service uap-observer-runner.service uap-observer-runner.socket uap-observer-caddy.service uap-observer-egress-proxy.service uap-observer-egress-proxy.socket; do
  install -o root -g root -m 0644 "$source_root/deploy/$unit" "$systemd_stage/$unit"
done
for service in uap-observer uap-observer-runner; do
  {
    printf '%s\n' '[Service]' 'IPAddressDeny=any'
    # systemd's address filter is bidirectional. The observer must accept
    # Caddy's exact loopback peer; only the gateway is available to the runner.
    if [ "$service" = uap-observer ]; then printf '%s\n' 'IPAddressAllow=127.0.0.1/32'; fi
    printf '%s\n' 'IPAddressAllow=127.0.0.2/32'
    if [ "$service" = uap-observer-runner ]; then
      PYTHONDONTWRITEBYTECODE=1 PYTHONPATH="$source_root" python3 -B - "$adapter_config" <<'PY'
import sys
from pathlib import Path
from observer.fixed_runner import validate_adapter_input_access
for path in validate_adapter_input_access(Path(sys.argv[1])):
    print(f"BindReadOnlyPaths={path}")
PY
    fi
  } > "$systemd_stage/$service.service.d/egress.conf"
  chown root:root "$systemd_stage/$service.service.d/egress.conf"
  chmod 0644 "$systemd_stage/$service.service.d/egress.conf"
done
observer_normalize_tree_mtime "$systemd_stage"

(cd /opt/uap-observer-runtime.new && PYTHONDONTWRITEBYTECODE=1 /opt/uap-observer-venv.new/bin/python -B -c 'import cryptography,jsonschema; import observer.http_server')
test "$(sha256sum /usr/local/libexec/uap-observer-runner.new | cut -d' ' -f1)" = "$runner_digest"
test "$(sha256sum /usr/local/libexec/uap-observer-fixed-adapter.new | cut -d' ' -f1)" = "$adapter_digest"
test "$(sha256sum /usr/local/bin/caddy.new | cut -d' ' -f1)" = "$caddy_digest"
test "$(/usr/local/bin/caddy.new version | awk '{print $1}')" = "v2.11.4"
/usr/local/bin/caddy.new validate --config /etc/caddy/Caddyfile.new --adapter caddyfile
adapter_inode=$(stat -c '%d:%i' /usr/local/libexec/uap-observer-fixed-adapter.new)
for name in runtime notion chatgpt consent; do
  test "$(stat -c '%d:%i' "/usr/local/libexec/uap-observer-adapter-$name.new")" = "$adapter_inode"
done
for unit in uap-observer.service uap-observer-signer.service uap-observer-runner.service uap-observer-runner.socket uap-observer-caddy.service uap-observer-egress-proxy.service uap-observer-egress-proxy.socket; do
  cmp "$source_root/deploy/$unit" "$systemd_stage/$unit"
done
cmp "$observer_config" /etc/uap-observer.json.new
cmp "$adapter_config" /etc/uap-observer-adapter-config.json.new
cmp "$caddy_config" /etc/caddy/Caddyfile.new

# Build one immutable, complete version. No consumer references it until the
# single current-pointer rename below.
closure_stage="/opt/uap-observer-closures/.new-$$"
test ! -e "$closure_stage"
test ! -e "$closure_final"
install -d -o root -g root -m 0755 "$closure_stage/libexec" "$closure_stage/bin" "$closure_stage/etc"
install -d -o root -g root -m 0755 "$closure_stage/systemd"
observer_copy_systemd_tree_neutral "$systemd_stage" "$closure_stage/systemd"
mv /opt/uap-observer-venv.new "$closure_stage/venv"
mv /opt/uap-observer-runtime.new "$closure_stage/runtime"
mv /usr/local/libexec/uap-observer-runner.new "$closure_stage/libexec/uap-observer-runner"
mv /usr/local/libexec/uap-observer-egress-proxy.new "$closure_stage/libexec/uap-observer-egress-proxy"
mv /usr/local/libexec/uap-observer-fixed-adapter.new "$closure_stage/libexec/uap-observer-fixed-adapter"
for name in runtime notion chatgpt consent; do
  mv "/usr/local/libexec/uap-observer-adapter-$name.new" "$closure_stage/libexec/uap-observer-adapter-$name"
done
mv /usr/local/libexec/uap-observer-attest-chatgpt.new "$closure_stage/libexec/uap-observer-attest-chatgpt"
mv /usr/local/libexec/uap-observer-attest-consent.new "$closure_stage/libexec/uap-observer-attest-consent"
mv /usr/local/libexec/uap-observer-provision-profile.new "$closure_stage/libexec/uap-observer-provision-profile"
mv /usr/local/bin/caddy.new "$closure_stage/bin/caddy"
mv /etc/uap-observer.json.new "$closure_stage/etc/uap-observer.json"
mv /etc/uap-observer-egress-allowlist.json.new "$closure_stage/etc/uap-observer-egress-allowlist.json"
mv /etc/uap-observer-adapter-config.json.new "$closure_stage/etc/uap-observer-adapter-config.json"
mv /etc/uap-observer-adapters.json.new "$closure_stage/etc/uap-observer-adapters.json"
mv /etc/caddy/Caddyfile.new "$closure_stage/etc/Caddyfile"
printf '%s\n' 'complete-v1' > "$closure_stage/.complete"
printf '%s\n' "$install_identity" > "$closure_stage/.install-identity"
apply_observer_closure_modes "$closure_stage"
observer_normalize_tree_mtime "$closure_stage"
observer_sync_tree "$closure_stage"
closure_digest=$(observer_closure_identity "$closure_stage")
closure_final="/opt/uap-observer-closures/$closure_digest"
test ! -e "$closure_final"
unit_validation="$stage_root/unit-validation"
install -d -o root -g root -m 0700 "$unit_validation"
for unit in uap-observer.service uap-observer-signer.service uap-observer-runner.service uap-observer-runner.socket uap-observer-caddy.service uap-observer-egress-proxy.service uap-observer-egress-proxy.socket; do
  sed "s|/opt/uap-observer-current|$closure_stage|g" "$systemd_stage/$unit" > "$unit_validation/$unit"
done
systemd-analyze verify "$unit_validation"/*.service "$unit_validation"/*.socket
PYTHONDONTWRITEBYTECODE=1 PYTHONPATH="$closure_stage/runtime" "$closure_stage/venv/bin/python" -B - "$closure_stage" "$runner_digest" "$adapter_digest" <<'PY'
import grp,hashlib,sys
from pathlib import Path
from observer.config import Config
from observer.fixed_runner import Adapter, ARTIFACT_ORDER, ReviewedRunner, read_owned_regular
from observer.runner import SocketRunner
root=Path(sys.argv[1])
Config.load(root / "etc/uap-observer.json")
SocketRunner(Path("/run/unused-validation.sock"), root / "libexec/uap-observer-runner", "sha256:" + sys.argv[2], 840)
gid=grp.getgrnam("uap-observer-adapter-config").gr_gid
config=root / "etc/uap-observer-adapter-config.json"
encoded=read_owned_regular(config, 4 << 20, owner_uid=0, exact_mode=0o640, group_gid=gid)
config_digest="sha256:" + hashlib.sha256(encoded).hexdigest()
adapters=tuple(Adapter(name, root / f"libexec/uap-observer-adapter-{kind}", "sha256:" + sys.argv[3], config, config_digest) for name,kind in zip(ARTIFACT_ORDER,("runtime","notion","chatgpt","consent")))
ReviewedRunner(adapters, Path("/var/lib/uap-observer/jobs"))
PY
systemd_backup="$stage_root/systemd-backup"
journal_observer_systemd "$systemd_backup" /etc/systemd/system
observer_sync_tree "$systemd_backup"
printf '%s\n' "$closure_digest" > "$stage_root/closure-digest"
chown root:root "$stage_root/closure-digest"
chmod 0600 "$stage_root/closure-digest"
sync -f "$stage_root/closure-digest"
printf '%s\n' committed-v1 > "$stage_root/.journal-committed.new"
chown root:root "$stage_root/.journal-committed.new"
chmod 0600 "$stage_root/.journal-committed.new"
sync -f "$stage_root/.journal-committed.new"
mv "$stage_root/.journal-committed.new" "$stage_root/journal-committed"
observer_sync_directory "$stage_root"
mv "$closure_stage" "$closure_final"
closure_stage=
sync -f /opt/uap-observer-closures
activate_observer_systemd "$systemd_stage" /etc/systemd/system "$systemd_backup"
observer_sync_tree /etc/systemd/system
reload_observer_systemd systemctl
test "$(observer_closure_identity "$closure_final")" = "$closure_digest"
ln -s "uap-observer-closures/$closure_digest" /opt/uap-observer-current.new
mv -T /opt/uap-observer-current.new /opt/uap-observer-current
sync -f /opt
echo "observer files installed; provision the root key and reviewed adapters before enabling services"
