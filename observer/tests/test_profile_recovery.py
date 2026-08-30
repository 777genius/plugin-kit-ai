from __future__ import annotations

import importlib.util
import hashlib
import json
import os
import shutil
import stat
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).parents[2]
HELPER_PATH = ROOT / "deploy/uap-observer-recover-profile-seed.py"
SPEC = importlib.util.spec_from_file_location("uap_observer_recover_profile_seed", HELPER_PATH)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError("profile recovery helper could not be loaded")
helper = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(helper)


def release_tuple(plugin: str) -> dict:
    digest = "sha256:" + "a" * 64
    return {
        "product_id": plugin, "tree_digest": digest, "manifest_digest": digest,
        "distribution_id": f"owner/{plugin}", "distribution_kind": "upstream",
        "release_sequence": 1, "package_version": "1.0.0",
        "source_repository": f"owner/{plugin}", "source_revision": "b" * 40,
        "source_path": f"plugins/{plugin}", "snapshot_sequence": 1,
        "snapshot_digest": digest, "binary_digest": digest,
        "dependency_identity": "locked", "installer_version": "0.1.18",
        "adapter_version": "r14d", "client_version": None, "os": "linux",
        "architecture": "x86_64", "observed_at": "2026-08-26T00:00:00Z",
    }


class ProfileRecoveryTests(unittest.TestCase):
    def test_source_and_installed_provisioner_layouts_load_deterministically(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            installed = Path(temporary)
            installed_helper = installed / "uap-observer-recover-profile-seed"
            installed_provisioner = installed / "uap-observer-provision-profile"
            shutil.copy2(HELPER_PATH, installed_helper)
            shutil.copy2(ROOT / "deploy/uap-observer-provision-profile.py", installed_provisioner)

            loaded = subprocess.run(
                [sys.executable, "-B", str(installed_helper), "--help"],
                text=True, capture_output=True,
            )
            self.assertEqual(loaded.returncode, 0, loaded.stdout + loaded.stderr)
            self.assertIn("--archived-profile", loaded.stdout)

            shutil.copy2(
                ROOT / "deploy/uap-observer-provision-profile.py",
                installed / "uap-observer-provision-profile.py",
            )
            ambiguous = subprocess.run(
                [sys.executable, "-B", str(installed_helper), "--help"],
                text=True, capture_output=True,
            )
            self.assertNotEqual(ambiguous.returncode, 0)
            self.assertIn("requires exactly one provisioner dependency", ambiguous.stderr)

            (installed / "uap-observer-provision-profile.py").unlink()
            installed_provisioner.unlink()
            installed_provisioner.symlink_to(ROOT / "deploy/uap-observer-provision-profile.py")
            linked = subprocess.run(
                [sys.executable, "-B", str(installed_helper), "--help"],
                text=True, capture_output=True,
            )
            self.assertNotEqual(linked.returncode, 0)
            self.assertIn("is not a protected regular file", linked.stderr)

    def fixture(self, root: Path) -> tuple[Path, Path, Path, bytes]:
        profile, proof, output = root / "profile", root / "proof", root / "output"
        profile.mkdir(mode=0o700); proof.mkdir(mode=0o700); output.mkdir(mode=0o700)
        (profile / "auth").mkdir(mode=0o700)
        (profile / "auth" / "credential").write_bytes(b"opaque credential")
        native = proof / "native"
        native.mkdir(mode=0o700)
        digest = "sha256:" + "a" * 64
        entries = []
        for plugin in sorted(helper.HEROES):
            blob = native / f"{plugin}.blob"
            blob.write_bytes(b"{}")
            entries.append({
                "plugin": plugin,
                "component_kind": "skill" if plugin == "agent-code-navigator" else "mcp",
                "tuple": release_tuple(plugin),
                "native_config": {
                    "path": f"/var/lib/uap-observer/proofs/codex/native/{plugin}.blob",
                    "sha256": digest,
                },
                "client_config": {
                    "path": (
                        "/var/lib/uap-observer/profiles/codex/skills/code-tool-router/SKILL.md"
                        if plugin == "agent-code-navigator"
                        else f"/var/lib/uap-observer/profiles/codex/{plugin}.json"
                    ),
                    "sha256": digest,
                },
                "manager_add_sha256": digest, "manager_info_sha256": digest,
                "post_add_doctor_sha256": digest,
            })
        projection_body = json.dumps({
            "schema_version": 2, "client_id": "codex", "entries": entries,
        }).encode()
        (proof / "native-projection.json").write_bytes(projection_body)
        (proof / "receipts.json").write_text(json.dumps({
            "schema_version": 1,
            "receipts": [{
                "name": entry["plugin"], "tuple": entry["tuple"],
                "manager_add_sha256": entry["manager_add_sha256"],
                "manager_info_sha256": entry["manager_info_sha256"],
                "post_add_doctor_sha256": entry["post_add_doctor_sha256"],
            } for entry in entries],
        }))
        for path in (*profile.rglob("*"), *proof.rglob("*")):
            path.chmod(0o700 if path.is_dir() else 0o600)
        matrix = [{
            "plugin": plugin, "client": client, "tuple": release_tuple(plugin),
            "application_id": f"{client}-{plugin}", "endpoint": "https://example.invalid/mcp",
        } for client in sorted(helper.CLIENTS) for plugin in sorted(helper.HEROES)]
        adapter = {
            "schema_version": 1, "request_policy": {}, "git": {},
            "clients": {
                "codex": {
                    "client_id": "codex",
                    "binary": "/opt/uap-observer-inputs/bin/codex",
                    "sha256": "sha256:" + "1" * 64,
                    "profile": "/var/lib/uap-observer/profiles/codex",
                    "companion_binary": "/opt/uap-observer-inputs/bin/codex-code-mode-host",
                    "companion_sha256": "sha256:" + "2" * 64,
                    "native_projection": {
                        "path": "/var/lib/uap-observer/proofs/codex/native-projection.json",
                        "sha256": "sha256:" + hashlib.sha256(projection_body).hexdigest(),
                    },
                },
                "cursor": {}, "kiro": {},
            },
            "matrix": matrix, "consent_record": {}, "chatgpt": {},
            "chrome_for_testing": {}, "workspace_root": "/var/lib/uap-observer/workspaces",
            "external_pr_evidence": {}, "egress_hosts": [],
        }
        return profile, proof, output, json.dumps(adapter).encode()

    def recover(self, profile: Path, proof: Path, output: Path, adapter: bytes, **kwargs) -> tuple[int, int, bool]:
        profile_fd = os.open(profile, helper.OPEN_DIRECTORY)
        proof_fd = os.open(proof, helper.OPEN_DIRECTORY)
        output_fd = os.open(output, helper.OPEN_DIRECTORY)
        try:
            def publish(parent_fd: int, staged: str, final: str) -> None:
                os.rename(staged, final, src_dir_fd=parent_fd, dst_dir_fd=parent_fd)
            with mock.patch.object(helper, "publish_noreplace", side_effect=publish):
                return helper.reconstruct_seed(
                    profile_fd, proof_fd, output_fd, "seed", "codex", adapter,
                    owner_uid=os.geteuid(), owner_gid=os.getegid(), **kwargs,
                )
        finally:
            os.close(output_fd); os.close(proof_fd); os.close(profile_fd)

    def test_reconstructs_normalized_seed_and_exact_proof_inventory(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            profile, proof, output, adapter = self.fixture(Path(temporary))
            count, size, already = self.recover(profile, proof, output, adapter)
            self.assertFalse(already)
            seed = output / "seed"
            self.assertGreater(count, 0); self.assertGreater(size, 0)
            self.assertEqual((seed / "auth" / "credential").read_bytes(), b"opaque credential")
            self.assertEqual(set((seed / helper.PROOF_SEED_NAME).iterdir()), {
                seed / helper.PROOF_SEED_NAME / "receipts.json",
                seed / helper.PROOF_SEED_NAME / "native-projection.json",
                seed / helper.PROOF_SEED_NAME / "native",
            })
            for path in (seed, *seed.rglob("*")):
                self.assertEqual(path.stat().st_uid, os.geteuid())
                self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o700 if path.is_dir() else 0o600)

    def test_rejects_links_hardlinks_and_special_files(self) -> None:
        cases = ("symlink", "hardlink", "fifo", "writable")
        for case in cases:
            with self.subTest(case=case), tempfile.TemporaryDirectory() as temporary:
                profile, proof, output, adapter = self.fixture(Path(temporary))
                if case == "symlink":
                    (profile / "bad").symlink_to("auth/credential")
                elif case == "hardlink":
                    os.link(profile / "auth" / "credential", profile / "bad")
                else:
                    if case == "fifo":
                        os.mkfifo(profile / "bad", 0o600)
                    else:
                        (profile / "auth" / "credential").chmod(0o666)
                with self.assertRaisesRegex(ValueError, "link|special|hardlinked|protected"):
                    self.recover(profile, proof, output, adapter)
                self.assertEqual(list(output.iterdir()), [])

    def test_rejects_proof_inventory_tuple_mismatch_and_reserved_collision(self) -> None:
        for case in ("inventory", "tuple", "collision"):
            with self.subTest(case=case), tempfile.TemporaryDirectory() as temporary:
                profile, proof, output, adapter = self.fixture(Path(temporary))
                if case == "inventory":
                    (proof / "extra").write_bytes(b"x")
                elif case == "tuple":
                    receipts = json.loads((proof / "receipts.json").read_text())
                    receipts["receipts"][0]["tuple"]["package_version"] = "other"
                    (proof / "receipts.json").write_text(json.dumps(receipts))
                    (proof / "receipts.json").chmod(0o600)
                else:
                    (profile / helper.PROOF_SEED_NAME).mkdir(mode=0o700)
                with self.assertRaises(ValueError):
                    self.recover(profile, proof, output, adapter)
                self.assertEqual(list(output.iterdir()), [])

    def test_failure_removes_only_staging_and_preserves_existing_parent_entries(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            profile, proof, output, adapter = self.fixture(Path(temporary))
            sentinel = output / "keep"
            sentinel.write_text("keep")
            with mock.patch.object(helper, "checkpoint", side_effect=OSError("injected failure")), self.assertRaisesRegex(OSError, "injected"):
                self.recover(profile, proof, output, adapter)
            self.assertEqual(sentinel.read_text(), "keep")
            self.assertEqual([path.name for path in output.iterdir()], ["keep"])

    def test_combined_profile_and_proof_bounds_fail_closed(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            profile, proof, output, adapter = self.fixture(Path(temporary))
            with self.assertRaisesRegex(ValueError, "file-count bound"):
                self.recover(profile, proof, output, adapter, limits=(2, helper.MAX_BYTES))
            self.assertEqual(list(output.iterdir()), [])
            with self.assertRaisesRegex(ValueError, "byte bound"):
                self.recover(profile, proof, output, adapter, limits=(helper.MAX_FILES, 1))
            self.assertEqual(list(output.iterdir()), [])

    def test_source_mutation_during_copy_fails_and_removes_staging(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            profile, proof, output, adapter = self.fixture(Path(temporary))
            credential = profile / "auth" / "credential"
            inode = credential.stat().st_ino
            original_read = helper.os.read
            changed = False

            def mutate_after_read(descriptor: int, count: int) -> bytes:
                nonlocal changed
                body = original_read(descriptor, count)
                if body and not changed and os.fstat(descriptor).st_ino == inode:
                    changed = True
                    credential.write_bytes(b"substituted")
                    credential.chmod(0o600)
                return body

            with mock.patch.object(helper.os, "read", side_effect=mutate_after_read), self.assertRaisesRegex(ValueError, "changed"):
                self.recover(profile, proof, output, adapter)
            self.assertEqual(list(output.iterdir()), [])

    def test_root_absolute_inputs_and_nonexistent_output_are_mandatory(self) -> None:
        argv = ["recover", "--client", "codex", "--archived-profile", "relative-profile",
                "--archived-proof", "/proof", "--adapter-config", "/adapter.json",
                "--output-seed", "/output"]
        with mock.patch("sys.argv", argv), mock.patch.object(helper.os, "geteuid", return_value=0), self.assertRaisesRegex(SystemExit, "absolute"):
            helper.main()
        with mock.patch("sys.argv", argv), mock.patch.object(helper.os, "geteuid", return_value=123), self.assertRaisesRegex(SystemExit, "requires root"):
            helper.main()
        with tempfile.TemporaryDirectory() as temporary:
            profile, proof, output, adapter = self.fixture(Path(temporary))
            existing = output / "seed"
            existing.write_text("keep")
            with self.assertRaises(OSError):
                self.recover(profile, proof, output, adapter)
            self.assertEqual(existing.read_text(), "keep")

    def test_current_adapter_projection_and_tuple_must_match_before_copy(self) -> None:
        for case in ("projection-path", "projection-digest", "tuple"):
            with self.subTest(case=case), tempfile.TemporaryDirectory() as temporary:
                profile, proof, output, adapter_body = self.fixture(Path(temporary))
                adapter = json.loads(adapter_body)
                if case == "projection-path":
                    adapter["clients"]["codex"]["native_projection"]["path"] = "/var/lib/uap-observer/proofs/cursor/native-projection.json"
                elif case == "projection-digest":
                    adapter["clients"]["codex"]["native_projection"]["sha256"] = "sha256:" + "0" * 64
                else:
                    row = next(row for row in adapter["matrix"] if row["client"] == "codex" and row["plugin"] == "context7")
                    row["tuple"]["package_version"] = "different-release"
                with self.assertRaisesRegex(ValueError, "binding differs|tuple differs"):
                    self.recover(profile, proof, output, json.dumps(adapter).encode())
                self.assertEqual(list(output.iterdir()), [])

    def test_depth_bound_fails_before_cleanup_recursion_can_leave_credentials(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            profile, proof, output, adapter = self.fixture(Path(temporary))
            parent = profile
            for _ in range(helper.MAX_DEPTH + 1):
                parent = parent / "d"
                parent.mkdir(mode=0o700)
            with self.assertRaisesRegex(ValueError, "directory-depth bound"):
                self.recover(profile, proof, output, adapter)
            self.assertEqual(list(output.iterdir()), [])

    def test_publish_race_preserves_raced_target_and_removes_staging(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            profile, proof, output, adapter = self.fixture(Path(temporary))
            sentinel = output / "seed"

            def raced_publish(_parent_fd: int, _staged: str, _final: str) -> None:
                sentinel.write_text("raced target")
                raise FileExistsError("destination appeared before publication")

            profile_fd = os.open(profile, helper.OPEN_DIRECTORY)
            proof_fd = os.open(proof, helper.OPEN_DIRECTORY)
            output_fd = os.open(output, helper.OPEN_DIRECTORY)
            try:
                with mock.patch.object(helper, "publish_noreplace", side_effect=raced_publish), self.assertRaises(FileExistsError):
                    helper.reconstruct_seed(
                        profile_fd, proof_fd, output_fd, "seed", "codex", adapter,
                        owner_uid=os.geteuid(), owner_gid=os.getegid(),
                    )
            finally:
                os.close(output_fd); os.close(proof_fd); os.close(profile_fd)
            self.assertEqual(sentinel.read_text(), "raced target")
            self.assertEqual([path.name for path in output.iterdir()], ["seed"])

    def test_parent_fsync_failure_after_rename_retries_as_identical_published_seed(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            profile, proof, output, adapter = self.fixture(Path(temporary))
            profile_fd = os.open(profile, helper.OPEN_DIRECTORY)
            proof_fd = os.open(proof, helper.OPEN_DIRECTORY)
            output_fd = os.open(output, helper.OPEN_DIRECTORY)
            original_fsync = helper.os.fsync
            published = False

            def publish(parent_fd: int, staged: str, final: str) -> None:
                nonlocal published
                os.rename(staged, final, src_dir_fd=parent_fd, dst_dir_fd=parent_fd)
                published = True

            def fail_publish_fsync(descriptor: int) -> None:
                if published and descriptor == output_fd:
                    raise OSError("parent fsync failed after rename")
                original_fsync(descriptor)

            try:
                with mock.patch.object(helper, "publish_noreplace", side_effect=publish), mock.patch.object(helper.os, "fsync", side_effect=fail_publish_fsync), self.assertRaisesRegex(OSError, "after rename"):
                    helper.reconstruct_seed(
                        profile_fd, proof_fd, output_fd, "seed", "codex", adapter,
                        owner_uid=os.geteuid(), owner_gid=os.getegid(),
                    )
                self.assertTrue((output / "seed").is_dir())
                published = False
                result = helper.reconstruct_seed(
                    profile_fd, proof_fd, output_fd, "seed", "codex", adapter,
                    owner_uid=os.geteuid(), owner_gid=os.getegid(),
                )
            finally:
                os.close(output_fd); os.close(proof_fd); os.close(profile_fd)
            self.assertTrue(result[2])
            self.assertEqual([path.name for path in output.iterdir()], ["seed"])


if __name__ == "__main__":
    unittest.main()
