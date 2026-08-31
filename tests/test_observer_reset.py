import fcntl
import hashlib
import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from unittest import mock
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "uap_observer_reset", ROOT / "deploy/uap-observer-reset.py",
)
assert SPEC is not None and SPEC.loader is not None
reset = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = reset
SPEC.loader.exec_module(reset)


MACHINE = "a" * 32
CATALOG = "f" * 40
OLD_INSTALL = "b" * 64
OLD_CLOSURE = "c" * 64
NEW_INSTALL = "d" * 64
NEW_CLOSURE = "e" * 64


class FakeSystemd:
    def __init__(self, *, public_active: bool = True, recreate_failure: bool = False):
        self.states = {
            unit: {"active": True, "enabled": unit != reset.PUBLIC_HOP_UNIT}
            for unit in reset.ALL_UNITS
        }
        self.states["uap-observer-caddy.service"] = {"active": False, "enabled": False}
        self.states[reset.PUBLIC_HOP_UNIT] = {"active": public_active, "enabled": False}
        self.recreate_failure = recreate_failure
        self.events = []
        self.public_closure = OLD_CLOSURE if public_active else None
        self.failed = set()
        self.missing = set()

    def state(self, unit):
        state = dict(self.states[unit])
        if unit in self.failed or unit in self.missing:
            state["active"] = False
        return state

    def state_with_failure(self, unit):
        state, failed, _missing = self.state_details(unit)
        return state, failed

    def state_details(self, unit):
        return self.state(unit), unit in self.failed, unit in self.missing

    def is_failed(self, unit):
        return unit in self.failed

    def stop(self, units):
        self.events.append(("stop", tuple(units)))
        for unit in units:
            if unit in self.missing:
                raise reset.ResetError(f"cannot stop missing unit: {unit}")
            self.states[unit]["active"] = False
            if unit == reset.PUBLIC_HOP_UNIT:
                self.missing.add(unit)

    def start(self, units):
        self.events.append(("start", tuple(units)))
        for unit in units:
            self.states[unit]["active"] = True

    def enable(self, units):
        for unit in units:
            self.states[unit]["enabled"] = True

    def disable(self, units):
        for unit in units:
            self.states[unit]["enabled"] = False

    def daemon_reload(self):
        self.events.append(("daemon-reload",))
        self.missing.difference_update(reset.UNITS)

    def reset_failed(self, units):
        self.events.append(("reset-failed", tuple(units)))
        self.failed.difference_update(units)

    def validate_public_hop_contract(self):
        return {"contract": "exact-v1"}

    def recreate_public_hop(self, closure):
        self.events.append(("recreate-public-hop", closure))
        if self.recreate_failure:
            self.missing.discard(reset.PUBLIC_HOP_UNIT)
            self.failed.add(reset.PUBLIC_HOP_UNIT)
            self.states[reset.PUBLIC_HOP_UNIT]["active"] = False
            raise reset.ResetError("injected public hop recreation failure")
        self.failed.discard(reset.PUBLIC_HOP_UNIT)
        self.missing.discard(reset.PUBLIC_HOP_UNIT)
        self.public_closure = closure
        self.states[reset.PUBLIC_HOP_UNIT]["active"] = True

    def verify_public_hop(self, closure):
        if self.public_closure != closure or not self.states[reset.PUBLIC_HOP_UNIT]["active"]:
            raise reset.ResetError("public hop closure differs")

    def public_hop_pid(self):
        if not self.states[reset.PUBLIC_HOP_UNIT]["active"]:
            raise reset.ResetError("public hop has no process")
        return 4242


class FailOnce:
    def __init__(self, name):
        self.name = name
        self.triggered = False

    def __call__(self, name):
        if name == self.name and not self.triggered:
            self.triggered = True
            raise reset.InjectedFailure(name)


class FailPublicHopOnceSystemd(FakeSystemd):
    def __init__(self):
        super().__init__()
        self.failed_once = False

    def recreate_public_hop(self, closure):
        if not self.failed_once:
            self.failed_once = True
            self.events.append(("recreate-public-hop", closure))
            self.missing.discard(reset.PUBLIC_HOP_UNIT)
            self.failed.add(reset.PUBLIC_HOP_UNIT)
            self.states[reset.PUBLIC_HOP_UNIT]["active"] = False
            raise reset.ResetError("injected one-time public hop failure")
        super().recreate_public_hop(closure)


class FakeExecutor:
    def __init__(self, fixture, *, failure=None):
        self.fixture = fixture
        self.failure = failure
        self.prepare_count = 0

    def prepare_new(self, controller, journal):
        self.prepare_count += 1
        self.fixture.install_candidate(journal["prepared"]["manifest"])
        if self.failure:
            raise reset.ResetError(self.failure)
        controller._activate_recorded_units(journal, NEW_CLOSURE)
        return {"install_identity": NEW_INSTALL, "closure_digest": NEW_CLOSURE}

    def verify_new(self, controller, journal):
        new = journal["new"]
        controller._validate_install(new["install_identity"], new["closure_digest"])
        self.fixture.systemd.verify_public_hop(new["closure_digest"])


class LockCheckingExecutor(FakeExecutor):
    def prepare_new(self, controller, journal):
        descriptor = os.open(self.fixture.path("/run/lock/uap-observer-install.lock"), os.O_RDWR)
        try:
            with self.fixture.assertRaises(BlockingIOError):
                fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
        finally:
            os.close(descriptor)
        return super().prepare_new(controller, journal)


class ProjectionMismatchExecutor(FakeExecutor):
    def prepare_new(self, controller, journal):
        result = super().prepare_new(controller, journal)
        projection = self.fixture.path(
            "/var/lib/uap-observer/proofs/codex/native-projection.json"
        )
        projection.chmod(0o600)
        projection.write_bytes(b'{"client_id":"codex","entries":[{}],"schema_version":2}\n')
        projection.chmod(0o440)
        return result


class ObserverResetTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.root = Path(self.temporary.name)
        self.layout = reset.Layout.for_test(self.root)
        self.systemd = FakeSystemd()
        self._base_filesystem()
        self.executor = FakeExecutor(self)

    def tearDown(self):
        self.temporary.cleanup()

    def path(self, logical):
        return self.layout.path(logical)

    def mkdir(self, logical, mode=0o755):
        path = self.path(logical)
        path.mkdir(parents=True, exist_ok=True)
        path.chmod(mode)
        return path

    def file(self, logical, body=b"fixture\n", mode=0o644):
        path = self.path(logical)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(body)
        path.chmod(mode)
        return path

    def _install(self, install_identity, closure_digest):
        closures = self.mkdir("/opt/uap-observer-closures", 0o755)
        closure = closures / closure_digest
        closure.mkdir()
        closure.chmod(0o755)
        (closure / ".complete").write_text("complete-v1\n")
        (closure / ".install-identity").write_text(install_identity + "\n")
        (closure / "bin").mkdir()
        (closure / "bin" / "caddy").write_text("binary")
        libexec = closure / "libexec"
        libexec.mkdir()
        fixed_adapter = libexec / "uap-observer-fixed-adapter"
        fixed_adapter.write_text("adapter binary")
        fixed_adapter.chmod(0o555)
        for name in ("runtime", "notion", "chatgpt", "consent"):
            os.link(fixed_adapter, libexec / f"uap-observer-adapter-{name}")
        for marker in (closure / ".complete", closure / ".install-identity"):
            marker.chmod(0o644)
        current = self.path("/opt/uap-observer-current")
        current.parent.mkdir(parents=True, exist_ok=True)
        current.symlink_to(f"uap-observer-closures/{closure_digest}")

    def _mutable(self):
        state = self.mkdir("/var/lib/uap-observer", 0o711)
        for name in ("jobs", "profiles", "proofs", "state", "workspaces"):
            (state / name).mkdir()
        for root in ("/var/lib/uap-observer-human", "/var/lib/uap-observer-consent"):
            parent = self.mkdir(root, 0o755)
            for name in ("pending", "reserved", "consumed"):
                (parent / name).mkdir()

    def _units(self):
        systemd = self.mkdir("/etc/systemd/system", 0o755)
        closure_systemd = self.path("/opt/uap-observer-current").resolve() / "systemd"
        closure_systemd.mkdir()
        for root in (systemd, closure_systemd):
            for unit in reset.UNITS:
                path = root / unit
                path.write_text("[Unit]\n")
                path.chmod(0o644)
            for name in ("uap-observer.service.d", "uap-observer-runner.service.d"):
                dropin = root / name
                dropin.mkdir()
                dropin.chmod(0o755)
                child = dropin / "egress.conf"
                child.write_text("[Service]\n")
                child.chmod(0o644)
        self.systemd.missing.difference_update(reset.UNITS)

    def _base_filesystem(self):
        for logical in (
            "/run/lock", "/proc", "/sys/fs/cgroup/system.slice", "/usr/local/libexec",
        ):
            self.mkdir(logical)
        self.file("/etc/machine-id", (MACHINE + "\n").encode(), 0o444)
        sentinel = json.dumps({
            "machine_id": MACHINE,
            "purpose": reset.SENTINEL_PURPOSE,
            "schema_version": 1,
        }, sort_keys=True, separators=(",", ":")).encode() + b"\n"
        self.file("/etc/uap-observer-disposable.json", sentinel, 0o600)
        self.file("/etc/uap-observer-ed25519.key", b"private fixture\n", 0o600)
        self.mkdir("/opt/uap-observer-inputs", 0o755)
        self.mkdir("/etc/caddy", 0o755)
        self.mkdir("/var/lib/caddy", 0o700)
        self.mkdir("/var/log/caddy", 0o700)
        self.file("/var/lib/caddy/uap-vm-internal-Caddyfile", b"observer.test {\n}\n", 0o600)
        self.file(
            "/var/lib/caddy/.local/share/caddy/pki/authorities/local/root.crt",
            b"test internal CA\n", 0o600,
        )
        for name in ("root.key", "intermediate.crt", "intermediate.key"):
            self.file(
                f"/var/lib/caddy/.local/share/caddy/pki/authorities/local/{name}",
                f"test internal CA {name}\n".encode(), 0o600,
            )
        for logical in (
            "/root/caddy_2.11.4_linux_amd64.tar.gz", "/root/Caddyfile",
            "/root/uap-observer-adapter-config.json", "/root/uap-observer.json",
            "/root/uap-observer-egress-allowlist.json",
        ):
            self.file(logical, b"reviewed fixture\n", 0o600)
        self._prepared_bundle()
        self._install(OLD_INSTALL, OLD_CLOSURE)
        self._mutable()
        self._units()

    def _prepared_bundle(self):
        prepared = self.mkdir(reset.PREPARED_ROOT, 0o700)
        for name in reset.PREPARED_INVENTORY:
            child = prepared / name
            child.mkdir()
            child.chmod(0o700)
        evidence = prepared / "evidence"
        source = evidence / "source"
        source.mkdir()
        source.chmod(0o700)
        (source / "fixture").write_text("reviewed source\n")
        (source / "fixture").chmod(0o400)
        deploy = source / "deploy"
        deploy.mkdir()
        deploy.chmod(0o700)
        for name, mode in (
            ("uap-observer-reset.py", 0o755),
            ("uap-observer-install-lib.sh", 0o644),
        ):
            artifact = deploy / name
            artifact.write_bytes((ROOT / "deploy" / name).read_bytes())
            artifact.chmod(mode)
        clients = {}
        self.projection_bodies = {}
        for index, client in enumerate(reset.CLIENTS, start=1):
            for name in ("seeds", "path"):
                child = prepared / name / client
                child.mkdir()
                child.chmod(0o700)
                artifact = child / "fixture"
                artifact.write_text(f"{name}-{client}\n")
                artifact.chmod(0o400)
            projection_body = json.dumps(
                {"client_id": client, "entries": [], "schema_version": 2},
                sort_keys=True, separators=(",", ":"),
            ).encode() + b"\n"
            self.projection_bodies[client] = projection_body
            projection = "sha256:" + hashlib.sha256(projection_body).hexdigest()
            projection_file = prepared / "projection-digests" / f"{client}.sha256"
            projection_file.write_text(projection + "\n")
            projection_file.chmod(0o400)
            clients[client] = {
                "seed": f"seeds/{client}",
                "seed_digest": "sha256:" + str(index + 3) * 64,
                "projection_digest": projection,
            }
        adapter_config = {
            "clients": {
                client: {
                    "native_projection": {
                        "path": f"/var/lib/uap-observer/proofs/{client}/native-projection.json",
                        "sha256": clients[client]["projection_digest"],
                    },
                }
                for client in reset.CLIENTS
            },
        }
        installer_files = {
            "uap-observer-adapter-config.json": json.dumps(
                adapter_config, sort_keys=True, separators=(",", ":"),
            ).encode() + b"\n",
            "uap-observer.json": b"observer\n",
            "caddy_2.11.4_linux_amd64.tar.gz": b"archive\n",
            "Caddyfile": b"caddy\n",
            "uap-observer-egress-allowlist.json": b"egress\n",
        }
        for name, body in installer_files.items():
            path = evidence / name
            path.write_bytes(body)
            path.chmod(0o400)
        def digest(name):
            return "sha256:" + hashlib.sha256(installer_files[name]).hexdigest()
        manifest = {
            "schema_version": 1,
            "purpose": reset.PREPARED_PURPOSE,
            "catalog_sha": CATALOG,
            "new_install_identity": NEW_INSTALL,
            "installer": {
                "source_root": "evidence/source",
                "adapter_config": "evidence/uap-observer-adapter-config.json",
                "adapter_sha256": digest("uap-observer-adapter-config.json"),
                "observer_config": "evidence/uap-observer.json",
                "observer_sha256": digest("uap-observer.json"),
                "caddy_archive": "evidence/caddy_2.11.4_linux_amd64.tar.gz",
                "caddy_config": "evidence/Caddyfile",
                "caddy_config_sha256": digest("Caddyfile"),
                "egress_allowlist": "evidence/uap-observer-egress-allowlist.json",
                "egress_sha256": digest("uap-observer-egress-allowlist.json"),
            },
            "clients": clients,
            "tree_digests": {
                name: reset.prepared_tree_digest(prepared / name, os.getuid())
                for name in reset.PREPARED_INVENTORY
            },
        }
        manifest_path = prepared / "manifest.json"
        manifest_path.write_text(json.dumps(manifest, sort_keys=True, separators=(",", ":")) + "\n")
        manifest_path.chmod(0o400)

    def controller(
        self, *, failpoint=None, systemd=None, executor=None,
        closure_identity=None, source_revision=None,
    ):
        return reset.ResetController(
            self.layout, systemd=systemd or self.systemd,
            runtime_probe=reset.RuntimeProbe(self.layout), failpoint=failpoint,
            executor=executor or self.executor,
            closure_identity=closure_identity or (lambda path: path.name),
            source_revision=source_revision or (lambda _path: CATALOG),
        )

    def drift_unit_states_after_reboot(self):
        for unit in reset.ALL_UNITS:
            self.systemd.states[unit] = {"active": False, "enabled": False}
        self.systemd.states["uap-observer-caddy.service"] = {
            "active": True, "enabled": True,
        }
        self.systemd.public_closure = None
        self.systemd.missing.add(reset.PUBLIC_HOP_UNIT)

    def install_candidate(self, manifest):
        self._install(NEW_INSTALL, NEW_CLOSURE)
        self._mutable()
        for client in reset.CLIENTS:
            proof = self.mkdir(f"/var/lib/uap-observer/proofs/{client}", 0o710)
            projection = proof / "native-projection.json"
            projection.write_bytes(self.projection_bodies[client])
            projection.chmod(0o440)
            self.assertEqual(
                "sha256:" + hashlib.sha256(projection.read_bytes()).hexdigest(),
                manifest["clients"][client]["projection_digest"],
            )
        self._units()

    def test_apply_status_finalize_roundtrip_is_clean_and_recreates_public_hop(self):
        controller = self.controller()
        applied = controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.assertEqual(applied["phase"], "new-ready")
        self.assertEqual(applied["old_quarantined"], applied["old_total"])
        self.assertTrue(self.path("/opt/uap-observer-current").is_symlink())
        self.assertEqual(self.systemd.events[0], ("stop", (reset.PUBLIC_HOP_UNIT,)))
        result = controller.finalize(
            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE, NEW_INSTALL, NEW_CLOSURE,
        )
        self.assertEqual(result["phase"], "finalized")
        self.assertFalse(self.path(reset.PREPARED_ROOT).exists())
        self.assertFalse(controller.journal_dir.exists())
        self.assertTrue(self.path(reset.STABLE_HELPER).is_file())
        self.assertTrue(self.path(reset.STABLE_INSTALL_LIB).is_file())
        self.assertEqual(self.systemd.public_closure, NEW_CLOSURE)
        self.assertEqual(list(self.root.rglob(".uap-observer-reset-*")), [])

    def test_rollback_before_candidate_restores_exact_old_install_and_unit_state(self):
        controller = self.controller(failpoint=FailOnce("after-applied"))
        with self.assertRaises(reset.InjectedFailure):
            controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        result = controller.rollback(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.assertEqual(result["phase"], "rolled_back")
        controller._validate_install(OLD_INSTALL, OLD_CLOSURE)
        self.assertEqual(self.systemd.public_closure, OLD_CLOSURE)
        self.assertTrue(self.path(reset.PREPARED_ROOT).exists())
        self.assertFalse(controller.journal_dir.exists())

    def test_rollback_after_candidate_and_partial_install_removes_candidate_only(self):
        controller = self.controller()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.file("/usr/local/bin/caddy.new", b"partial", 0o755)
        controller.rollback(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        controller._validate_install(OLD_INSTALL, OLD_CLOSURE)
        self.assertFalse(self.path("/usr/local/bin/caddy.new").exists())
        self.assertNotEqual(self.path("/opt/uap-observer-current").readlink(), Path(f"uap-observer-closures/{NEW_CLOSURE}"))

    def test_apply_is_idempotent_after_every_quarantine_failpoint(self):
        for index in (0, len(reset.MANAGED) // 2, len(reset.MANAGED) - 1):
            with self.subTest(index=index):
                self.tearDown()
                self.setUp()
                failure = FailOnce(f"after-quarantine:{index}")
                with self.assertRaises(reset.InjectedFailure):
                    self.controller(failpoint=failure).apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
                result = self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
                self.assertEqual(result["phase"], "new-ready")

    def test_applied_phase_recovery_skips_gc_managed_units_after_reboot(self):
        with self.assertRaises(reset.InjectedFailure):
            self.controller(failpoint=FailOnce("after-applied")).apply(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            )
        self.systemd.missing.update(reset.ALL_UNITS)
        result = self.controller().apply(
            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
        )
        self.assertEqual(result["phase"], "new-ready")
        self.assertFalse(self.systemd.missing)

    def test_applied_phase_rollback_skips_gc_managed_units_after_reboot(self):
        with self.assertRaises(reset.InjectedFailure):
            self.controller(failpoint=FailOnce("after-applied")).apply(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            )
        self.systemd.missing.update(reset.ALL_UNITS)
        result = self.controller().rollback(
            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
        )
        self.assertEqual(result["phase"], "rolled_back")
        self.assertFalse(self.systemd.missing)
        self.assertEqual(self.systemd.public_closure, OLD_CLOSURE)

    def test_rollback_is_idempotent_after_candidate_and_restore_failpoints(self):
        for name in ("after-candidate-quarantine:0", "after-old-restore:0"):
            with self.subTest(name=name):
                self.tearDown()
                self.setUp()
                controller = self.controller()
                controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
                with self.assertRaises(reset.InjectedFailure):
                    self.controller(failpoint=FailOnce(name)).rollback(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
                self.controller().rollback(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
                self.controller()._validate_install(OLD_INSTALL, OLD_CLOSURE)

    def test_rollback_cleanup_is_resumable_after_every_candidate_delete(self):
        controller = self.controller()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        with self.assertRaises(reset.InjectedFailure):
            self.controller(failpoint=FailOnce("after-delete:0")).rollback(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            )
        self.assertEqual(self.controller().status(MACHINE)["phase"], "rollback-cleanup")
        self.drift_unit_states_after_reboot()
        result = self.controller().rollback(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.assertEqual(result["phase"], "rolled_back")
        self.controller()._validate_install(OLD_INSTALL, OLD_CLOSURE)
        self.assertEqual(self.systemd.public_closure, OLD_CLOSURE)
        self.assertEqual(
            self.systemd.states["uap-observer-caddy.service"],
            {"active": False, "enabled": False},
        )

    def test_rollback_completion_recovers_unit_drift_after_journal_unlink(self):
        self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        with self.assertRaises(reset.InjectedFailure):
            self.controller(failpoint=FailOnce("after-journal-unlink")).rollback(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            )
        self.drift_unit_states_after_reboot()
        result = self.controller().rollback(
            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
        )
        self.assertEqual(result["phase"], "rolled_back")
        self.assertEqual(self.systemd.public_closure, OLD_CLOSURE)
        self.assertEqual(
            self.systemd.states["uap-observer-caddy.service"],
            {"active": False, "enabled": False},
        )

    def test_rollback_completion_keeps_exact_active_transient_ingress(self):
        self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        with self.assertRaises(reset.InjectedFailure):
            self.controller(failpoint=FailOnce("after-journal-unlink")).rollback(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            )
        recreations = [
            event for event in self.systemd.events
            if event[0] == "recreate-public-hop"
        ]
        result = self.controller().rollback(
            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
        )
        self.assertEqual(result["phase"], "rolled_back")
        self.assertEqual(
            [
                event for event in self.systemd.events
                if event[0] == "recreate-public-hop"
            ],
            recreations,
        )

    def test_rollback_completion_is_resumable_at_every_tombstone_boundary(self):
        for failpoint in (
            "after-completion-marker", "after-journal-unlink",
            "after-journal-dir-remove", "after-completion-remove",
        ):
            with self.subTest(failpoint=failpoint):
                self.tearDown()
                self.setUp()
                self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
                with self.assertRaises(reset.InjectedFailure):
                    self.controller(failpoint=FailOnce(failpoint)).rollback(
                        MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                    )
                result = self.controller().rollback(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
                self.assertEqual(result["phase"], "rolled_back")
                self.assertFalse(self.path("/var/lib/uap-observer-reset.completed").exists())
                self.assertFalse(self.path("/var/lib/uap-observer-reset").exists())

    def test_finalize_is_resumable_after_partial_old_cleanup(self):
        controller = self.controller()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        with self.assertRaises(reset.InjectedFailure):
            self.controller(failpoint=FailOnce("after-delete:0")).finalize(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE, NEW_INSTALL, NEW_CLOSURE,
            )
        result = self.controller().finalize(
            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE, NEW_INSTALL, NEW_CLOSURE,
        )
        self.assertEqual(result["phase"], "finalized")

    def test_finalize_is_idempotent_after_completed_cleanup(self):
        controller = self.controller()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        first = controller.finalize(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE, NEW_INSTALL, NEW_CLOSURE)
        second = controller.finalize(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE, NEW_INSTALL, NEW_CLOSURE)
        self.assertEqual(first, second)

    def test_finalize_completion_is_resumable_at_every_tombstone_boundary(self):
        for failpoint in (
            "after-completion-marker", "after-journal-unlink",
            "after-journal-dir-remove", "after-completion-remove",
        ):
            with self.subTest(failpoint=failpoint):
                self.tearDown()
                self.setUp()
                self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
                with self.assertRaises(reset.InjectedFailure):
                    self.controller(failpoint=FailOnce(failpoint)).finalize(
                        MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE, NEW_INSTALL, NEW_CLOSURE,
                    )
                result = self.controller().finalize(
                    MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE, NEW_INSTALL, NEW_CLOSURE,
                )
                self.assertEqual(result["phase"], "finalized")
                self.assertFalse(self.path("/var/lib/uap-observer-reset.completed").exists())
                self.assertFalse(self.path("/var/lib/uap-observer-reset").exists())

    def test_finalize_completion_recovers_unit_drift_after_journal_unlink(self):
        self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        with self.assertRaises(reset.InjectedFailure):
            self.controller(failpoint=FailOnce("after-journal-unlink")).finalize(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                NEW_INSTALL, NEW_CLOSURE,
            )
        self.drift_unit_states_after_reboot()
        result = self.controller().finalize(
            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            NEW_INSTALL, NEW_CLOSURE,
        )
        self.assertEqual(result["phase"], "finalized")
        self.assertEqual(self.systemd.public_closure, NEW_CLOSURE)
        self.assertEqual(
            self.systemd.states["uap-observer-caddy.service"],
            {"active": False, "enabled": False},
        )

    def test_finalize_completion_clears_failed_transient_before_recreation(self):
        self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        with self.assertRaises(reset.InjectedFailure):
            self.controller(failpoint=FailOnce("after-journal-unlink")).finalize(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                NEW_INSTALL, NEW_CLOSURE,
            )
        self.systemd.states[reset.PUBLIC_HOP_UNIT]["active"] = False
        self.systemd.failed.add(reset.PUBLIC_HOP_UNIT)
        result = self.controller().finalize(
            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            NEW_INSTALL, NEW_CLOSURE,
        )
        self.assertEqual(result["phase"], "finalized")
        self.assertNotIn(reset.PUBLIC_HOP_UNIT, self.systemd.failed)
        self.assertEqual(self.systemd.public_closure, NEW_CLOSURE)

    def test_finalize_new_ready_clears_failed_transient_before_recreation(self):
        self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.systemd.states[reset.PUBLIC_HOP_UNIT]["active"] = False
        self.systemd.failed.add(reset.PUBLIC_HOP_UNIT)
        result = self.controller().finalize(
            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            NEW_INSTALL, NEW_CLOSURE,
        )
        self.assertEqual(result["phase"], "finalized")
        self.assertNotIn(reset.PUBLIC_HOP_UNIT, self.systemd.failed)
        self.assertEqual(self.systemd.public_closure, NEW_CLOSURE)

    def test_exact_postcondition_rejects_failed_inactive_unit_latch(self):
        self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.systemd.failed.add("uap-observer-caddy.service")
        journal = self.controller()._read_journal()
        with self.assertRaisesRegex(reset.ResetError, "failed after reset"):
            self.controller()._verify_recorded_units(journal, NEW_CLOSURE)

    def test_guards_reject_wrong_machine_identity_and_foreign_sentinel(self):
        controller = self.controller()
        with self.assertRaisesRegex(reset.ResetError, "machine-id"):
            controller.apply("f" * 32, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        with self.assertRaisesRegex(reset.ResetError, "closure markers"):
            controller.apply(MACHINE, CATALOG, "f" * 64, OLD_CLOSURE)
        sentinel = self.path("/etc/uap-observer-disposable.json")
        sentinel.write_text('{}\n')
        sentinel.chmod(0o600)
        with self.assertRaisesRegex(reset.ResetError, "sentinel"):
            controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)

    def test_control_symlink_and_hardlink_are_rejected(self):
        sentinel = self.path("/etc/uap-observer-disposable.json")
        original = sentinel.with_suffix(".original")
        sentinel.rename(original)
        sentinel.symlink_to(original.name)
        with self.assertRaises(reset.ResetError):
            self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        sentinel.unlink()
        original.rename(sentinel)
        os.link(sentinel, sentinel.with_suffix(".hardlink"))
        with self.assertRaises(reset.ResetError):
            self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)

    def test_quarantine_substitution_blocks_finalize_before_cleanup(self):
        controller = self.controller()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        journal = controller._read_journal()
        entry = journal["old"][0]
        target = self.path(entry["quarantine"])
        target.unlink()
        target.symlink_to("/nonexistent")
        with self.assertRaisesRegex(reset.ResetError, "substituted"):
            controller.finalize(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE, NEW_INSTALL, NEW_CLOSURE)
        self.assertTrue(controller.journal_path.exists())

    def test_quarantine_symlink_target_is_bound_even_when_stat_identity_is_reused(self):
        controller = self.controller()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        journal = controller._read_journal()
        entry = journal["old"][0]
        target = self.path(entry["quarantine"])
        original_target = entry["metadata"]["target"]
        target.unlink()
        target.symlink_to(original_target[:-1] + "f")
        # Model inode reuse and restored timestamps deterministically: every
        # stat field matches, but the journal still binds the original target.
        entry["metadata"] = {**reset.metadata(target), "target": original_target}
        controller._write_journal(journal)
        with self.assertRaisesRegex(reset.ResetError, "substituted"):
            controller.finalize(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                NEW_INSTALL, NEW_CLOSURE,
            )
        self.assertEqual(controller._read_journal()["phase"], "new-ready")
        self.assertTrue(target.is_symlink())
        for old in journal["old"]:
            quarantine = self.path(old["quarantine"])
            self.assertTrue(quarantine.exists() or quarantine.is_symlink())

    def test_quarantine_regular_file_mutation_blocks_finalize_before_cleanup(self):
        controller = self.controller()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        journal = controller._read_journal()
        entry = next(old for old in journal["old"] if old["metadata"]["kind"] == "regular")
        target = self.path(entry["quarantine"])
        target.write_bytes(target.read_bytes() + b"substituted\n")
        self.assertEqual(target.stat().st_ino, entry["metadata"]["inode"])
        with self.assertRaisesRegex(reset.ResetError, "substituted"):
            controller.finalize(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                NEW_INSTALL, NEW_CLOSURE,
            )
        self.assertEqual(controller._read_journal()["phase"], "new-ready")
        self.assertTrue(target.exists())
        for old in journal["old"]:
            quarantine = self.path(old["quarantine"])
            self.assertTrue(quarantine.exists() or quarantine.is_symlink())

    def test_cleanup_reopened_directory_substitution_rejects_before_deleting_contents(self):
        controller = self.controller()
        target = self.mkdir("/root/cleanup-target", 0o700)
        original = self.file("/root/cleanup-target/original", b"original\n")
        replacement = self.mkdir("/root/cleanup-replacement", 0o700)
        self.file("/root/cleanup-replacement/foreign", b"untouched\n")
        saved = target.with_name("cleanup-original")
        expected = reset.metadata(target)
        real_open = os.open
        opens = 0

        def substitute_on_reopen(path, flags, *args, **kwargs):
            nonlocal opens
            if path == target.name and flags & os.O_DIRECTORY and "dir_fd" in kwargs:
                opens += 1
                if opens == 2:
                    target.rename(saved)
                    replacement.rename(target)
            return real_open(path, flags, *args, **kwargs)

        with mock.patch.object(reset.os, "open", side_effect=substitute_on_reopen):
            with self.assertRaisesRegex(reset.ResetError, "substituted"):
                controller._remove_tree(target, expected=expected)
        self.assertEqual(opens, 2)
        self.assertEqual((saved / original.name).read_bytes(), b"original\n")
        self.assertEqual((target / "foreign").read_bytes(), b"untouched\n")

    def test_cleanup_regular_path_substitution_rejects_before_unlink(self):
        controller = self.controller()
        target = self.file("/root/cleanup-target", b"original\n")
        replacement = self.file("/root/cleanup-replacement", b"untouched\n")
        saved = target.with_name("cleanup-original")
        expected = reset.metadata(target)
        real_open = os.open

        def substitute_after_open(path, flags, *args, **kwargs):
            descriptor = real_open(path, flags, *args, **kwargs)
            if path == target.name and "dir_fd" in kwargs:
                target.rename(saved)
                replacement.rename(target)
            return descriptor

        with mock.patch.object(reset.os, "open", side_effect=substitute_after_open):
            with self.assertRaisesRegex(reset.ResetError, "substituted"):
                controller._remove_tree(target, expected=expected)
        self.assertEqual(saved.read_bytes(), b"original\n")
        self.assertEqual(target.read_bytes(), b"untouched\n")

    def test_cleanup_never_follows_symlinks_and_rejects_hardlinks_before_any_delete(self):
        for kind in ("symlink", "hardlink"):
            with self.subTest(kind=kind):
                self.tearDown()
                self.setUp()
                controller = self.controller()
                controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
                journal = controller._read_journal()
                state = next(
                    entry for entry in journal["old"]
                    if entry["source"] == "/var/lib/uap-observer"
                )
                external = self.file("/root/external-cleanup-marker", b"outside\n", 0o600)
                injected = self.path(state["quarantine"]) / "state/injected"
                if kind == "symlink":
                    injected.symlink_to(external)
                    controller.finalize(
                        MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                        NEW_INSTALL, NEW_CLOSURE,
                    )
                else:
                    os.link(external, injected)
                    with self.assertRaisesRegex(reset.ResetError, "hardlinked"):
                        controller.finalize(
                            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                            NEW_INSTALL, NEW_CLOSURE,
                        )
                    first = self.path(journal["old"][0]["quarantine"])
                    self.assertTrue(first.exists() or first.is_symlink())
                    self.assertEqual(controller.status(MACHINE)["phase"], "new-ready")
                self.assertTrue(external.exists())
                self.assertEqual(external.read_bytes(), b"outside\n")

    def test_exact_five_link_adapter_closure_finalizes(self):
        controller = self.controller()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        journal = controller._read_journal()
        closures = next(
            entry for entry in journal["old"]
            if entry["source"] == "/opt/uap-observer-closures"
        )
        fixed = (
            self.path(closures["quarantine"]) / OLD_CLOSURE
            / "libexec/uap-observer-fixed-adapter"
        )
        self.assertEqual(fixed.stat().st_nlink, 5)
        result = controller.finalize(
            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            NEW_INSTALL, NEW_CLOSURE,
        )
        self.assertEqual(result["phase"], "finalized")

    def test_exact_five_link_candidate_closure_rolls_back(self):
        controller = self.controller()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        fixed = self.path(
            f"/opt/uap-observer-closures/{NEW_CLOSURE}"
            "/libexec/uap-observer-fixed-adapter"
        )
        self.assertEqual(fixed.stat().st_nlink, 5)
        result = controller.rollback(
            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
        )
        self.assertEqual(result["phase"], "rolled_back")

    def test_jobs_and_pending_attestations_reject_before_first_rename(self):
        for logical in (
            "/var/lib/uap-observer/jobs/job",
            "/var/lib/uap-observer-human/pending/attestation",
            "/var/lib/uap-observer-consent/pending/consent",
        ):
            with self.subTest(logical=logical):
                self.tearDown()
                self.setUp()
                self.file(logical)
                with self.assertRaisesRegex(reset.ResetError, "pending work"):
                    self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
                self.assertTrue(self.path("/opt/uap-observer-current").is_symlink())
                self.assertEqual(self.systemd.events, [])
                self.assertFalse(self.controller().journal_path.exists())

    def test_pending_work_arriving_during_stop_restores_old_service_and_allows_rollback(self):
        before = {unit: dict(state) for unit, state in self.systemd.states.items()}
        real_stop = self.systemd.stop
        pending = self.path("/var/lib/uap-observer-human/pending/fixture")

        def stop_and_queue(units):
            real_stop(units)
            if "uap-observer-runner.service" in units:
                pending.write_text("pending test request\n")

        controller = self.controller()
        with mock.patch.object(self.systemd, "stop", side_effect=stop_and_queue):
            with self.assertRaisesRegex(reset.ResetError, "pending work"):
                controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.assertEqual(self.systemd.states, before)
        self.assertEqual(controller._read_journal()["phase"], "prepared")
        result = controller.rollback(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.assertEqual(result["phase"], "rolled_back")
        self.assertEqual(self.systemd.states, before)
        self.assertEqual(pending.read_text(), "pending test request\n")
        self.assertFalse(controller.journal_path.exists())

    def test_legacy_lock_mode_is_normalized_on_the_same_held_inode(self):
        lock = self.file("/run/lock/uap-observer-install.lock", b"lock\n", 0o644)
        before = lock.stat()
        with self.controller().locked() as descriptor:
            self.assertEqual(os.fstat(descriptor).st_ino, before.st_ino)
            self.assertEqual(lock.stat().st_mode & 0o777, 0o600)
            self.assertEqual(lock.read_bytes(), b"lock\n")

    def test_journal_parent_is_durable_before_journal_contents(self):
        controller = self.controller()
        events = []
        real_fsync = reset.fsync_directory

        def record_fsync(path):
            events.append(path)
            real_fsync(path)

        with mock.patch.object(reset, "fsync_directory", side_effect=record_fsync):
            controller._write_journal({"fixture": "journal"}, create=True)
        self.assertIn(controller.journal_dir.parent, events)
        self.assertLess(events.index(controller.journal_dir.parent), events.index(controller.journal_dir))

    def test_installer_adapter_partials_roll_back_across_each_rename_and_unlink(self):
        paths = [path for path in reset.PARTIALS if path in reset.HARDLINKED_ADAPTER_PARTIALS]
        failpoints = [None] + [
            f"{operation}:{index}"
            for operation in ("after-candidate-quarantine", "after-delete")
            for index in range(len(paths))
        ]
        for failpoint in failpoints:
            with self.subTest(failpoint=failpoint):
                self.tearDown()
                self.setUp()
                with self.assertRaises(reset.InjectedFailure):
                    self.controller(failpoint=FailOnce("after-applied")).apply(
                        MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                    )
                original = self.file(paths[0], b"adapter fixture\n", 0o555)
                for path in paths[1:]:
                    os.link(original, self.path(path))
                if failpoint:
                    with self.assertRaises(reset.InjectedFailure):
                        self.controller(failpoint=FailOnce(failpoint)).rollback(
                            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                        )
                result = self.controller().rollback(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
                self.assertEqual(result["phase"], "rolled_back")
                self.controller()._validate_install(OLD_INSTALL, OLD_CLOSURE)
                self.assertTrue(all(not self.path(path).exists() for path in paths))
                self.assertEqual(list(self.root.rglob(".uap-observer-reset-*")), [])

    def test_adapter_partial_hardlink_outside_cleanup_is_retained(self):
        with self.assertRaises(reset.InjectedFailure):
            self.controller(failpoint=FailOnce("after-applied")).apply(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            )
        paths = [path for path in reset.PARTIALS if path in reset.HARDLINKED_ADAPTER_PARTIALS]
        original = self.file(paths[0], b"adapter fixture\n", 0o555)
        for path in paths[1:]:
            os.link(original, self.path(path))
        external = self.path("/root/external-adapter")
        os.link(original, external)
        with self.assertRaisesRegex(reset.ResetError, "hardlinked.*escapes"):
            self.controller().rollback(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.assertEqual(external.read_bytes(), b"adapter fixture\n")
        self.assertEqual(external.stat().st_nlink, 6)
        self.controller()._validate_install(OLD_INSTALL, OLD_CLOSURE)

    def test_installer_resolved_tombstone_is_removed_on_rollback(self):
        with self.assertRaises(reset.InjectedFailure):
            self.controller(failpoint=FailOnce("after-applied")).apply(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            )
        tombstone = self.mkdir("/opt/uap-observer-source.new.resolved-tombstone", 0o700)
        (tombstone / "fixture").write_text("resolved installer staging\n")
        self.assertEqual(
            self.controller().rollback(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)["phase"],
            "rolled_back",
        )
        self.assertFalse(tombstone.exists())

    def test_install_lock_excludes_reset(self):
        lock = self.path("/run/lock/uap-observer-install.lock")
        descriptor = os.open(lock, os.O_RDWR | os.O_CREAT, 0o600)
        try:
            fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
            with self.assertRaisesRegex(reset.ResetError, "another observer install"):
                self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        finally:
            os.close(descriptor)

    def test_preserved_signing_key_and_static_input_drift_fail_closed(self):
        controller = self.controller()
        key_before = self.path("/etc/uap-observer-ed25519.key").read_bytes()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.assertEqual(self.path("/etc/uap-observer-ed25519.key").read_bytes(), key_before)
        inputs = self.path("/opt/uap-observer-inputs")
        inputs.chmod(0o700)
        with self.assertRaisesRegex(reset.ResetError, "preserved path changed"):
            controller.status(MACHINE)

    def test_public_hop_recreation_failure_keeps_old_quarantine_and_prepared_seeds(self):
        controller = self.controller()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        failing = FakeSystemd(recreate_failure=True)
        failing.states = self.systemd.states
        failing.states[reset.PUBLIC_HOP_UNIT]["active"] = False
        failing_executor = FakeExecutor(self)
        with self.assertRaisesRegex(reset.ResetError, "recreation failure"):
            self.controller(systemd=failing, executor=failing_executor).finalize(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE, NEW_INSTALL, NEW_CLOSURE,
            )
        self.assertTrue(controller.journal_path.exists())
        self.assertTrue(self.path(reset.PREPARED_ROOT).exists())

    def test_prepared_root_inventory_is_exact_and_rollback_preserves_it(self):
        self.mkdir(f"{reset.PREPARED_ROOT}/foreign", 0o700)
        with self.assertRaisesRegex(reset.ResetError, "prepared reset root inventory"):
            self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)

    def test_prepared_manifest_and_tree_substitution_reject_before_stop(self):
        seed = self.path(f"{reset.PREPARED_ROOT}/seeds/codex/fixture")
        seed.chmod(0o600)
        seed.write_text("substituted!\n")
        seed.chmod(0o400)
        with self.assertRaisesRegex(reset.ResetError, "prepared tree digest"):
            self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.assertEqual(self.systemd.events, [])
        self.assertTrue(self.path("/opt/uap-observer-current").is_symlink())

    def test_prepared_adapter_projection_must_match_reviewed_manifest(self):
        prepared = self.path(reset.PREPARED_ROOT)
        adapter_path = prepared / "evidence/uap-observer-adapter-config.json"
        adapter = json.loads(adapter_path.read_text())
        adapter["clients"]["codex"]["native_projection"]["sha256"] = "sha256:" + "0" * 64
        adapter_body = json.dumps(adapter, sort_keys=True, separators=(",", ":")).encode() + b"\n"
        adapter_path.chmod(0o600)
        adapter_path.write_bytes(adapter_body)
        adapter_path.chmod(0o400)
        manifest_path = prepared / "manifest.json"
        manifest = json.loads(manifest_path.read_text())
        manifest["installer"]["adapter_sha256"] = "sha256:" + hashlib.sha256(adapter_body).hexdigest()
        manifest["tree_digests"]["evidence"] = reset.prepared_tree_digest(
            prepared / "evidence", os.getuid(),
        )
        manifest_path.chmod(0o600)
        manifest_path.write_text(json.dumps(manifest, sort_keys=True, separators=(",", ":")) + "\n")
        manifest_path.chmod(0o400)
        with self.assertRaisesRegex(reset.ResetError, "adapter projection binding"):
            self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.assertEqual(self.systemd.events, [])

    def test_installed_projection_digest_mismatch_auto_rolls_back(self):
        executor = ProjectionMismatchExecutor(self)
        with self.assertRaisesRegex(reset.ResetError, "installed projection digest differs"):
            self.controller(executor=executor).apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.controller()._validate_install(OLD_INSTALL, OLD_CLOSURE)

    def test_prepared_hardlink_is_rejected(self):
        source = self.path(f"{reset.PREPARED_ROOT}/path/codex/fixture")
        os.link(source, source.with_name("hardlink"))
        with self.assertRaisesRegex(reset.ResetError, "hardlinked"):
            self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)

    def test_preserved_in_place_content_change_with_restored_mtime_is_rejected(self):
        controller = self.controller(failpoint=FailOnce("after-applied"))
        with self.assertRaises(reset.InjectedFailure):
            controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        key = self.path("/etc/uap-observer-ed25519.key")
        info = key.stat()
        original = key.read_bytes()
        key.write_bytes(b"X" * len(original))
        key.chmod(0o600)
        os.utime(key, ns=(info.st_atime_ns, info.st_mtime_ns))
        with self.assertRaisesRegex(reset.ResetError, "file content changed"):
            controller.status(MACHINE)

    def test_wrong_internal_ca_content_is_rejected_without_metadata_signal(self):
        controller = self.controller(failpoint=FailOnce("after-applied"))
        with self.assertRaises(reset.InjectedFailure):
            controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        ca = self.path("/var/lib/caddy/.local/share/caddy/pki/authorities/local/root.crt")
        info = ca.stat()
        encoded = ca.read_bytes()
        ca.write_bytes(b"Z" * len(encoded))
        ca.chmod(0o600)
        os.utime(ca, ns=(info.st_atime_ns, info.st_mtime_ns))
        with self.assertRaisesRegex(reset.ResetError, "content changed"):
            controller.status(MACHINE)

    def test_install_lock_remains_held_through_preparation(self):
        executor = LockCheckingExecutor(self)
        result = self.controller(executor=executor).apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.assertEqual(result["phase"], "new-ready")
        self.assertEqual(executor.prepare_count, 1)

    def test_catalog_is_bound_to_caller_and_exact_prepared_source(self):
        with self.assertRaisesRegex(reset.ResetError, "catalog SHA differs from caller"):
            self.controller().apply(MACHINE, "0" * 40, OLD_INSTALL, OLD_CLOSURE)
        self.assertEqual(self.systemd.events, [])
        with self.assertRaisesRegex(reset.ResetError, "source revision differs"):
            self.controller(source_revision=lambda _path: "1" * 40).apply(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            )
        self.assertEqual(self.systemd.events, [])

    def test_prepared_source_requires_self_contained_git_authority(self):
        git_root = self.root / "git-authority-fixtures"
        origin = git_root / "origin"
        origin.mkdir(parents=True)

        def git(*arguments, cwd=None):
            return subprocess.run(
                ["git", *arguments], cwd=cwd, check=True, text=True,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            ).stdout.strip()

        git("init", str(origin))
        git("config", "user.name", "UAP Test", cwd=origin)
        git("config", "user.email", "uap-test@example.invalid", cwd=origin)
        (origin / "fixture").write_text("source\n")
        git("add", "fixture", cwd=origin)
        git("commit", "-m", "fixture", cwd=origin)
        head = git("rev-parse", "HEAD", cwd=origin)

        standalone = git_root / "standalone"
        git("clone", "--no-hardlinks", str(origin), str(standalone))
        self.assertEqual(
            self.controller()._production_source_revision(standalone), head,
        )

        marker = git_root / "git-hook-executed"
        hook = git_root / "fsmonitor-hook"
        hook.write_text(f"#!/bin/sh\nprintf executed >{marker}\n")
        hook.chmod(0o755)
        git("config", "core.fsmonitor", str(hook), cwd=standalone)
        with self.assertRaisesRegex(reset.ResetError, "executable or path-bearing"):
            self.controller()._production_source_revision(standalone)
        self.assertFalse(marker.exists())
        git("config", "--unset", "core.fsmonitor", cwd=standalone)
        git("config", "filter.evil.clean", str(hook), cwd=standalone)
        with self.assertRaisesRegex(reset.ResetError, "unapproved section"):
            self.controller()._production_source_revision(standalone)
        self.assertFalse(marker.exists())

        linked = git_root / "linked"
        git("worktree", "add", "-b", "linked-fixture", str(linked), "HEAD", cwd=origin)
        with self.assertRaisesRegex(reset.ResetError, "in-tree root-owned"):
            self.controller()._production_source_revision(linked)

        shared = git_root / "shared"
        git("clone", "--shared", str(origin), str(shared))
        with self.assertRaisesRegex(reset.ResetError, "external state"):
            self.controller()._production_source_revision(shared)

        def fresh(name):
            checkout = git_root / name
            git("clone", "--no-hardlinks", str(origin), str(checkout))
            return checkout

        replaced = fresh("replaced")
        (replaced / "fixture").write_text("replacement payload\n")
        git("add", "fixture", cwd=replaced)
        git(
            "-c", "user.name=UAP Test", "-c", "user.email=uap-test@example.invalid",
            "commit", "-m", "replacement", cwd=replaced,
        )
        replacement = git("rev-parse", "HEAD", cwd=replaced)
        git("replace", head, replacement, cwd=replaced)
        git("reset", "--hard", head, cwd=replaced)
        with self.assertRaisesRegex(reset.ResetError, "external state"):
            self.controller()._production_source_revision(replaced)

        for flag in ("--skip-worktree", "--assume-unchanged"):
            checkout = fresh(flag.removeprefix("--"))
            git("update-index", flag, "fixture", cwd=checkout)
            (checkout / "fixture").write_text(f"hidden by {flag}\n")
            with self.subTest(flag=flag), self.assertRaisesRegex(
                reset.ResetError, "worktree differs",
            ):
                self.controller()._production_source_revision(checkout)

        ignored = fresh("ignored")
        (ignored / ".git/info/exclude").write_text("ignored.payload\n")
        (ignored / "ignored.payload").write_text("ignored but executable input\n")
        with self.assertRaisesRegex(reset.ResetError, "worktree differs"):
            self.controller()._production_source_revision(ignored)

    def test_prepared_digest_rejects_before_source_revision_callback(self):
        source = self.path(f"{reset.PREPARED_ROOT}/evidence/source/fixture")
        source.chmod(0o600)
        source.write_text("unreviewed source\n")
        source.chmod(0o400)
        marker = self.root / "source-revision-called"

        def unsafe_revision(_source):
            marker.write_text("called\n")
            return CATALOG

        with self.assertRaisesRegex(reset.ResetError, "prepared tree digest"):
            self.controller(source_revision=unsafe_revision).apply(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            )
        self.assertFalse(marker.exists())

    def test_closure_content_identity_is_recomputed_before_stop(self):
        with self.assertRaisesRegex(reset.ResetError, "closure content identity differs"):
            self.controller(closure_identity=lambda _path: "0" * 64).apply(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            )
        self.assertEqual(self.systemd.events, [])

    def test_wrong_old_install_guard_precedes_stable_bootstrap_rotation(self):
        with self.assertRaisesRegex(reset.ResetError, "closure markers"):
            self.controller().apply(
                MACHINE, CATALOG, "0" * 64, OLD_CLOSURE,
            )
        self.assertFalse(self.path(reset.STABLE_HELPER).exists())
        self.assertFalse(self.path(reset.STABLE_INSTALL_LIB).exists())

    def test_deployed_systemd_drift_rejects_before_bootstrap_stop_or_rename(self):
        variants = ("changed-unit", "masked-unit", "extra-drop-in")
        for variant in variants:
            with self.subTest(variant=variant):
                self.tearDown()
                self.setUp()
                unit = self.path("/etc/systemd/system/uap-observer.service")
                if variant == "changed-unit":
                    unit.write_text("[Unit]\nDescription=drifted\n")
                elif variant == "masked-unit":
                    unit.unlink()
                    unit.symlink_to("/dev/null")
                else:
                    self.file(
                        "/etc/systemd/system/uap-observer.service.d/foreign.conf",
                        b"[Service]\nEnvironment=DRIFT=1\n", 0o644,
                    )
                with self.assertRaisesRegex(reset.ResetError, "deployed systemd"):
                    self.controller().apply(
                        MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                    )
                self.assertEqual(self.systemd.events, [])
                self.assertFalse(self.path(reset.STABLE_HELPER).exists())
                self.assertFalse(self.path(reset.STABLE_INSTALL_LIB).exists())
                self.assertTrue(self.path("/opt/uap-observer-current").is_symlink())

    def test_journal_temp_recovery_is_idempotent_before_and_after_replace(self):
        for failpoint in ("after-journal-temp-fsync", "after-journal-replace"):
            with self.subTest(failpoint=failpoint):
                self.tearDown()
                self.setUp()
                with self.assertRaises(reset.InjectedFailure):
                    self.controller(failpoint=FailOnce(failpoint)).apply(
                        MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                    )
                result = self.controller().apply(
                    MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                )
                self.assertEqual(result["phase"], "new-ready")
                self.assertFalse(self.controller().journal_temporary.exists())

    def test_stable_bootstrap_is_resumable_between_atomic_file_replacements(self):
        for failpoint in ("after-stable-helper", "after-stable-install-lib"):
            with self.subTest(failpoint=failpoint):
                self.tearDown()
                self.setUp()
                with self.assertRaises(reset.InjectedFailure):
                    self.controller(failpoint=FailOnce(failpoint)).apply(
                        MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                    )
                result = self.controller().apply(
                    MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                )
                self.assertEqual(result["phase"], "new-ready")

    def test_journal_temp_transaction_substitution_is_rejected(self):
        controller = self.controller(failpoint=FailOnce("after-journal-temp-fsync"))
        with self.assertRaises(reset.InjectedFailure):
            controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        temporary = controller.journal_temporary
        value = json.loads(temporary.read_text())
        value["transaction_id"] = "0" * 24
        temporary.write_text(json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n")
        temporary.chmod(0o600)
        with self.assertRaisesRegex(reset.ResetError, "transaction binding differs"):
            self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)

    def test_finalize_recovers_after_mid_prepared_tree_cleanup(self):
        self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        failure = FailOnce(
            f"after-tree-delete:{Path(reset.PREPARED_ROOT).name}:1"
        )
        with self.assertRaises(reset.InjectedFailure):
            self.controller(failpoint=failure).finalize(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                NEW_INSTALL, NEW_CLOSURE,
            )
        result = self.controller().finalize(
            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            NEW_INSTALL, NEW_CLOSURE,
        )
        self.assertEqual(result["phase"], "finalized")

    def test_next_transaction_rotates_stable_bootstrap_from_new_reviewed_source(self):
        controller = self.controller()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        controller.finalize(
            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            NEW_INSTALL, NEW_CLOSURE,
        )
        previous = self.path(reset.STABLE_HELPER).read_bytes()
        self._prepared_bundle()
        prepared = self.path(reset.PREPARED_ROOT)
        helper = prepared / "evidence/source/deploy/uap-observer-reset.py"
        helper.chmod(0o700)
        helper.write_bytes(helper.read_bytes() + b"\n# next reviewed transaction\n")
        helper.chmod(0o755)
        manifest_path = prepared / "manifest.json"
        manifest = json.loads(manifest_path.read_text())
        manifest["tree_digests"]["evidence"] = reset.prepared_tree_digest(
            prepared / "evidence", os.getuid(),
        )
        manifest_path.chmod(0o600)
        manifest_path.write_text(
            json.dumps(manifest, sort_keys=True, separators=(",", ":")) + "\n"
        )
        manifest_path.chmod(0o400)
        with self.assertRaises(reset.InjectedFailure):
            self.controller(failpoint=FailOnce("after-journal")).apply(
                MACHINE, CATALOG, NEW_INSTALL, NEW_CLOSURE,
            )
        self.assertNotEqual(self.path(reset.STABLE_HELPER).read_bytes(), previous)
        self.assertEqual(self.path(reset.STABLE_HELPER).read_bytes(), helper.read_bytes())
        self.controller().rollback(MACHINE, CATALOG, NEW_INSTALL, NEW_CLOSURE)

    def test_finalize_recovers_after_mid_quarantine_tree_cleanup(self):
        controller = self.controller()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        journal = controller._read_journal()
        closure_entry = next(
            entry for entry in journal["old"]
            if entry["source"] == "/opt/uap-observer-closures"
        )
        failpoint = f"after-tree-delete:{Path(closure_entry['quarantine']).name}:1"
        with self.assertRaises(reset.InjectedFailure):
            self.controller(failpoint=FailOnce(failpoint)).finalize(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                NEW_INSTALL, NEW_CLOSURE,
            )
        result = self.controller().finalize(
            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            NEW_INSTALL, NEW_CLOSURE,
        )
        self.assertEqual(result["phase"], "finalized")

    def test_volatile_caddy_runtime_writes_are_allowed_but_root_substitution_is_not(self):
        controller = self.controller()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.file("/var/lib/caddy/autosave.json", b"volatile\n", 0o600)
        self.file("/var/log/caddy/access.log", b"request\n", 0o600)
        self.assertEqual(controller.status(MACHINE)["phase"], "new-ready")
        volatile = self.path("/var/log/caddy")
        replaced = volatile.with_name("caddy-old")
        volatile.rename(replaced)
        volatile.mkdir(mode=0o700)
        with self.assertRaisesRegex(reset.ResetError, "volatile preserved root was substituted"):
            controller.status(MACHINE)

    def test_exact_caddy_partial_is_recoverable_and_substitution_is_rejected(self):
        for valid in (True, False):
            with self.subTest(valid=valid):
                self.tearDown()
                self.setUp()
                controller = self.controller()
                controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
                body = self.path(
                    f"{reset.PREPARED_ROOT}/evidence/Caddyfile"
                ).read_bytes()
                if not valid:
                    body = b"substituted\n"
                self.file("/etc/caddy/Caddyfile.new", body, 0o640)
                if valid:
                    self.assertEqual(
                        controller.rollback(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)["phase"],
                        "rolled_back",
                    )
                    self.assertFalse(self.path("/etc/caddy/Caddyfile.new").exists())
                else:
                    with self.assertRaisesRegex(reset.ResetError, "Caddy partial differs"):
                        controller.rollback(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)

    def test_ca_inventory_rejects_missing_and_permissive_key_material(self):
        authority = self.path(reset.CA_AUTHORITY_ROOT)
        (authority / "root.key").chmod(0o640)
        with self.assertRaisesRegex(reset.ResetError, "CA file metadata differs"):
            self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        (authority / "root.key").chmod(0o600)
        (authority / "intermediate.key").unlink()
        with self.assertRaisesRegex(reset.ResetError, "CA inventory differs"):
            self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)

    @unittest.skipUnless(os.geteuid() == 0, "requires ownership mutation")
    def test_ca_inventory_rejects_wrong_owner(self):
        key = self.path(f"{reset.CA_AUTHORITY_ROOT}/root.key")
        os.chown(key, 1, 1)
        with self.assertRaisesRegex(reset.ResetError, "CA file metadata differs"):
            self.controller().apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)

    @unittest.skipUnless(sys.platform.startswith("linux"), "Linux byte-name contract")
    def test_prepared_digest_accepts_invalid_utf8_names_without_loss(self):
        directory = self.path(f"{reset.PREPARED_ROOT}/path/codex")
        descriptor = os.open(directory, os.O_RDONLY | os.O_DIRECTORY)
        try:
            child = os.open(
                b"invalid-\xff", os.O_WRONLY | os.O_CREAT | os.O_EXCL,
                0o400, dir_fd=descriptor,
            )
            os.write(child, b"fixture\n")
            os.close(child)
        finally:
            os.close(descriptor)
        digest = reset.prepared_tree_digest(
            self.path(f"{reset.PREPARED_ROOT}/path"), os.getuid(),
        )
        self.assertRegex(digest, r"sha256:[0-9a-f]{64}")

    def test_public_hop_contract_rejects_extra_argv_and_broad_paths(self):
        expected = {
            "FragmentPath": "/run/systemd/transient/uap-observer-caddy-internal.service",
            "Transient": "yes", "Type": "notify", "Restart": "no",
            "User": "caddy", "Group": "caddy",
            "ExecStart": (
                "/opt/uap-observer-current/bin/caddy run --environ --config "
                "/var/lib/caddy/uap-vm-internal-Caddyfile --adapter caddyfile"
            ),
            "PrivateTmp": "yes", "ProtectSystem": "strict", "ProtectHome": "yes",
            "ReadWritePaths": "/var/lib/caddy /var/log/caddy",
            "ReadOnlyPaths": "/opt/uap-observer-current",
            "AmbientCapabilities": "cap_net_bind_service",
            "CapabilityBoundingSet": "cap_net_bind_service",
            "NoNewPrivileges": "yes", "LimitNOFILE": "1048576",
            "Description": "Disposable UAP VM internal-CA ingress",
        }
        systemd = reset.Systemd()
        with mock.patch.object(systemd, "public_hop_contract", return_value=expected):
            normalized = systemd.validate_public_hop_contract()
            self.assertEqual(normalized["ReadWritePaths"], ["/var/lib/caddy", "/var/log/caddy"])
        dynamic = {
            **expected,
            "ExecStart": (
                "{ path=/opt/uap-observer-current/bin/caddy ; "
                "argv[]=/opt/uap-observer-current/bin/caddy run --environ --config "
                "/var/lib/caddy/uap-vm-internal-Caddyfile --adapter caddyfile ; "
                "ignore_errors=no ; start_time=[n/a] ; stop_time=[n/a] ; pid=0 ; "
                "code=(null) ; status=0/0 }"
            ),
        }
        with mock.patch.object(systemd, "public_hop_contract", return_value=dynamic):
            self.assertEqual(
                systemd.validate_public_hop_contract()["ExecStart"][0],
                "/opt/uap-observer-current/bin/caddy",
            )
        for field, value in (
            ("ExecStart", expected["ExecStart"] + " --watch"),
            ("ReadWritePaths", "/var/lib/caddy /var/log/caddy /"),
            ("ReadOnlyPaths", "/opt/uap-observer-current /etc"),
        ):
            drifted = {**expected, field: value}
            with self.subTest(field=field), mock.patch.object(
                systemd, "public_hop_contract", return_value=drifted,
            ), self.assertRaises(reset.ResetError):
                systemd.validate_public_hop_contract()

    def test_systemd_state_fails_closed_on_query_error(self):
        systemd = reset.Systemd()
        failed = mock.Mock(returncode=1, stdout="", stderr="Failed to connect to bus")
        with mock.patch.object(systemd, "_run", return_value=failed):
            with self.assertRaisesRegex(reset.ResetError, "active-state query failed"):
                systemd.state("uap-observer.service")

    def test_systemd_v255_transient_and_not_found_tuples_are_exact(self):
        systemd = reset.Systemd()
        active = mock.Mock(returncode=0, stdout="active\n", stderr="")
        transient = mock.Mock(returncode=0, stdout="transient\n", stderr="")
        with mock.patch.object(systemd, "_run", side_effect=(active, transient)):
            self.assertEqual(
                systemd.state_details(reset.PUBLIC_HOP_UNIT),
                ({"active": True, "enabled": False}, False, False),
            )
        inactive_missing = mock.Mock(returncode=4, stdout="inactive\n", stderr="")
        not_found = mock.Mock(returncode=4, stdout="not-found\n", stderr="")
        with mock.patch.object(
            systemd, "_run", side_effect=(inactive_missing, not_found),
        ):
            self.assertEqual(
                systemd.state_details(reset.PUBLIC_HOP_UNIT),
                ({"active": False, "enabled": False}, False, True),
            )
        wrong_transient = mock.Mock(returncode=1, stdout="transient\n", stderr="")
        with mock.patch.object(
            systemd, "_run", side_effect=(active, wrong_transient),
        ), self.assertRaisesRegex(reset.ResetError, "enablement query failed"):
            systemd.state_details(reset.PUBLIC_HOP_UNIT)
        with mock.patch.object(
            systemd, "_run", side_effect=(active, not_found),
        ), self.assertRaisesRegex(reset.ResetError, "presence query disagrees"):
            systemd.state_details(reset.PUBLIC_HOP_UNIT)

    def test_linux_mount_identity_record_must_be_unique_and_bounded(self):
        for encoded in (b"mnt_id:\t1\nmnt_id:\t2\n", b"x" * 4097):
            with self.subTest(size=len(encoded)), mock.patch.object(
                reset.sys, "platform", "linux",
            ), mock.patch.object(reset.os, "open", return_value=99,
            ), mock.patch.object(reset.os, "read", return_value=encoded), mock.patch.object(
                reset.os, "close",
            ), self.assertRaises(reset.ResetError):
                reset.descriptor_mount_id(3)
        with mock.patch.object(reset.sys, "platform", "linux"), mock.patch.object(
            reset.os, "open", side_effect=FileNotFoundError,
        ), self.assertRaisesRegex(reset.ResetError, "unavailable"):
            reset.descriptor_mount_id(3)

    def test_live_shape_keeps_regular_caddy_inactive_and_transient_exclusive(self):
        controller = self.controller()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.assertEqual(
            self.systemd.states["uap-observer-caddy.service"],
            {"active": False, "enabled": False},
        )
        self.assertTrue(self.systemd.states[reset.PUBLIC_HOP_UNIT]["active"])
        started = [event[1] for event in self.systemd.events if event[0] == "start"]
        self.assertTrue(all("uap-observer-caddy.service" not in units for units in started))

    def test_wrong_ingress_topology_is_rejected_before_stop_or_rename(self):
        variants = (
            ({"active": True, "enabled": False}, {"active": False, "enabled": False}),
            ({"active": False, "enabled": True}, {"active": True, "enabled": False}),
            ({"active": False, "enabled": False}, {"active": False, "enabled": False}),
        )
        for regular, transient in variants:
            with self.subTest(regular=regular, transient=transient):
                self.tearDown()
                self.setUp()
                self.systemd.states["uap-observer-caddy.service"] = regular
                self.systemd.states[reset.PUBLIC_HOP_UNIT] = transient
                with self.assertRaises(reset.ResetError):
                    self.controller().apply(
                        MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                    )
                self.assertEqual(self.systemd.events, [])
                self.assertTrue(self.path("/opt/uap-observer-current").is_symlink())

    def test_initial_failed_unit_is_rejected_before_stop_or_rename(self):
        self.systemd.failed.add(reset.PUBLIC_HOP_UNIT)
        with self.assertRaisesRegex(reset.ResetError, "failed before reset"):
            self.controller().apply(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            )
        self.assertEqual(self.systemd.events, [])
        self.assertTrue(self.path("/opt/uap-observer-current").is_symlink())

    def test_failed_new_transient_is_reset_before_automatic_rollback(self):
        systemd = FailPublicHopOnceSystemd()
        with self.assertRaisesRegex(reset.ResetError, "one-time public hop failure"):
            self.controller(systemd=systemd).apply(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            )
        self.controller(systemd=systemd)._validate_install(
            OLD_INSTALL, OLD_CLOSURE,
        )
        self.assertNotIn(reset.PUBLIC_HOP_UNIT, systemd.failed)
        self.assertEqual(systemd.public_closure, OLD_CLOSURE)
        self.assertIn(
            ("reset-failed", (reset.PUBLIC_HOP_UNIT,)), systemd.events,
        )

    def test_egress_readiness_uses_exact_proxy_and_scrubs_inherited_proxy_environment(self):
        allowlist = json.dumps(
            {"hosts": ["api.github.com"], "schema_version": 1},
            sort_keys=True, separators=(",", ":"),
        ).encode() + b"\n"
        self.file(
            f"/opt/uap-observer-closures/{OLD_CLOSURE}/etc/"
            "uap-observer-egress-allowlist.json",
            allowlist, 0o644,
        )
        journal = {"units": self.systemd.states}
        completed = mock.Mock(stdout="200", stderr="", returncode=0)
        with mock.patch.dict(os.environ, {
            "HTTP_PROXY": "http://foreign", "https_proxy": "http://foreign",
            "NO_PROXY": "*", "SAFE_FIXTURE": "retained",
        }, clear=True), mock.patch("subprocess.run", return_value=completed) as invoked:
            reset.PreparedExecutor(self.layout, self.systemd)._verify_egress_proxy(
                OLD_CLOSURE, journal,
            )
        arguments = invoked.call_args.args[0]
        environment = invoked.call_args.kwargs["env"]
        self.assertIn("--proxy", arguments)
        self.assertIn("http://127.0.0.2:8766", arguments)
        self.assertEqual(arguments[arguments.index("--noproxy") + 1], "")
        self.assertEqual(environment, {"SAFE_FIXTURE": "retained"})

    def test_public_hop_hostname_and_ca_command_are_strict(self):
        self.assertEqual(reset.caddy_hostname("observer.example.test {\n}\n"), "observer.example.test")
        for invalid in ("{\n}\n", "one.test {\n}\ntwo.test {\n}\n", "127.0.0.1 {\n}\n"):
            with self.assertRaises(reset.ResetError):
                reset.caddy_hostname(invalid)
        source = (ROOT / "deploy/uap-observer-reset.py").read_text()
        self.assertIn('"--cacert", str(ca_root)', source)
        self.assertNotIn('"--insecure"', source)
        self.assertIn('"--resolve", f"{host}:443:127.0.0.1"', source)
        self.assertIn('"--noproxy", "*"', source)
        self.assertIn('"--proxy", "http://127.0.0.2:8766", "--noproxy", ""', source)

    def test_public_hop_requires_the_observer_get_response_not_a_proxy_error(self):
        for status in ("404", "200", "403", "502", "000"):
            with self.subTest(status=status), mock.patch.object(
                reset.Systemd, "validate_public_hop_contract",
            ), mock.patch.object(reset.Systemd, "public_hop_pid", return_value=4242), mock.patch.object(
                reset.Path, "resolve", return_value=Path(f"/opt/uap-observer-closures/{NEW_CLOSURE}/bin/caddy"),
            ), mock.patch.object(reset.Path, "read_text", return_value="observer.example.test {\n}\n"), mock.patch.object(
                reset, "metadata", return_value={"kind": "regular", "nlink": 1, "mode": 0o600},
            ), mock.patch.object(reset, "regular_file_digest"), mock.patch.object(
                reset.subprocess, "run", return_value=mock.Mock(stdout=status),
            ):
                if status == "404":
                    reset.Systemd().verify_public_hop(NEW_CLOSURE)
                else:
                    with self.assertRaisesRegex(reset.ResetError, "observer GET endpoint"):
                        reset.Systemd().verify_public_hop(NEW_CLOSURE)

    def test_installer_accepts_only_reviewed_inherited_fd9(self):
        installer = (ROOT / "deploy/uap-observer-install.sh").read_text()
        self.assertIn('UAP_OBSERVER_INSTALL_LOCK_FD', installer)
        self.assertIn("test \"$(readlink -f /proc/self/fd/9)\" = /run/lock/uap-observer-install.lock", installer)
        self.assertIn("stat -c '%u:%a:%h' /run/lock/uap-observer-install.lock", installer)

    @unittest.skipUnless(sys.platform.startswith("linux") and os.geteuid() == 0, "requires privileged Linux sandbox")
    def test_linux_bind_mounts_fail_closed_without_deleting_external_content(self):
        for kind in ("directory", "regular"):
            with self.subTest(kind=kind):
                self.tearDown()
                self.setUp()
                external = self.root / f"external-{kind}"
                target = self.path(f"{reset.PREPARED_ROOT}/path/codex")
                if kind == "directory":
                    external.mkdir()
                    marker = external / "marker"
                    marker.write_text("outside\n")
                else:
                    external.write_text("outside\n")
                    target = target / "fixture"
                    marker = external
                mounted = subprocess.run(
                    ["mount", "--bind", str(external), str(target)],
                    check=False, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                )
                if mounted.returncode != 0:
                    self.skipTest("sandbox lacks bind-mount capability")
                try:
                    with self.assertRaisesRegex(reset.ResetError, "mount boundary"):
                        self.controller().apply(
                            MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
                        )
                    self.assertTrue(marker.exists())
                    self.assertEqual(marker.read_text(), "outside\n")
                    self.assertEqual(self.systemd.events, [])
                finally:
                    subprocess.run(
                        ["umount", str(target)], check=True,
                        stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                    )

    @unittest.skipUnless(sys.platform.startswith("linux") and os.geteuid() == 0, "requires privileged Linux sandbox")
    def test_privileged_linux_sandbox_roundtrip_has_zero_residue(self):
        controller = self.controller()
        controller.apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        controller.finalize(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE, NEW_INSTALL, NEW_CLOSURE)
        self.assertFalse(controller.journal_dir.exists())
        self.assertFalse(self.path(reset.PREPARED_ROOT).exists())

    def test_prepare_failure_automatically_rolls_back_and_preserves_primary_error(self):
        executor = FakeExecutor(self, failure="injected prepare failure")
        with self.assertRaisesRegex(reset.ResetError, "injected prepare failure"):
            self.controller(executor=executor).apply(MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE)
        self.controller()._validate_install(OLD_INSTALL, OLD_CLOSURE)
        self.assertFalse(self.path("/var/lib/uap-observer-reset").exists())

    def test_prepare_and_rollback_failure_reports_both_errors(self):
        systemd = FakeSystemd(recreate_failure=True)
        executor = FakeExecutor(self, failure="primary preparation failure")
        with self.assertRaises(reset.ResetError) as caught:
            self.controller(systemd=systemd, executor=executor).apply(
                MACHINE, CATALOG, OLD_INSTALL, OLD_CLOSURE,
            )
        message = str(caught.exception)
        self.assertIn("primary preparation failure", message)
        self.assertIn("automatic rollback also failed", message)
        self.assertIn("public hop recreation failure", message)


if __name__ == "__main__":
    unittest.main()
