"""Exercise only the installer's lock block using disposable temporary files."""

import fcntl
import os
from pathlib import Path
import shutil
import stat
import subprocess
import sys
import tempfile
import unittest


INSTALLER = Path(__file__).resolve().parents[1] / "deploy/uap-observer-install.sh"
LOCK_PATH = "/run/lock/uap-observer-install.lock"
SOURCE = INSTALLER.read_text()
LOCK_BLOCK = SOURCE.split('case "${UAP_OBSERVER_INSTALL_LOCK_FD:-}" in', 1)[1]
LOCK_BLOCK = 'case "${UAP_OBSERVER_INSTALL_LOCK_FD:-}" in' + LOCK_BLOCK.split(
    "\n# Recovery is deliberately", 1
)[0]
VALIDATION = LOCK_BLOCK.split("<<'PY'\n", 1)[1].split("\nPY\n", 1)[0]


class LockFixture(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="uap-install-lock-test-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.lock = self.root / "install.lock"

    def file(self, name="install.lock", mode=0o644):
        path = self.root / name
        path.write_text("preserved lock contents\n")
        path.chmod(mode)
        return path

    def run_shell(self, script, descriptor=None, environment=None):
        passed = ()
        if descriptor is not None:
            script = f"exec 9<&{descriptor}\n" + script
            passed = (descriptor,)
        env = dict(os.environ)
        env.pop("UAP_OBSERVER_INSTALL_LOCK_FD", None)
        env.update(environment or {})
        return subprocess.run(
            ["/bin/sh", "-c", "set -eu\n" + script],
            pass_fds=passed, env=env, text=True, capture_output=True, timeout=5,
        )

    def open_lock(self, path):
        descriptor = os.open(path, os.O_RDWR)
        self.addCleanup(os.close, descriptor)
        return descriptor

    def validation(self, descriptor, root_identity=True):
        source = VALIDATION
        if root_identity:
            # Only the expected root identity is mapped to the test owner.
            source = source.replace("info.st_uid != 0", "info.st_uid != os.geteuid()")
        return self.run_shell(
            f"'{sys.executable}' -B - '{self.lock}' <<'PY'\n{source}\nPY\n",
            descriptor,
        )


class DescriptorValidationTests(LockFixture):
    def test_legacy_mode_normalized_without_replacing_or_truncating(self):
        self.file()
        before = self.lock.stat()
        result = self.validation(self.open_lock(self.lock))
        self.assertEqual(result.returncode, 0, result.stderr)
        after = self.lock.stat()
        self.assertEqual((before.st_dev, before.st_ino), (after.st_dev, after.st_ino))
        self.assertEqual(stat.S_IMODE(after.st_mode), 0o600)
        self.assertEqual(self.lock.read_text(), "preserved lock contents\n")

    def test_wrong_descriptor_does_not_chmod_either_file(self):
        self.file()
        other = self.file("other.lock")
        result = self.validation(self.open_lock(other))
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(stat.S_IMODE(other.stat().st_mode), 0o644)
        self.assertEqual(stat.S_IMODE(self.lock.stat().st_mode), 0o644)

    def test_symlink_rejected_without_changing_target(self):
        outside = self.file("outside")
        self.lock.symlink_to(outside)
        result = self.validation(self.open_lock(outside))
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(stat.S_IMODE(outside.stat().st_mode), 0o644)
        self.assertEqual(outside.read_text(), "preserved lock contents\n")

    def test_hardlink_rejected_without_changing_target(self):
        outside = self.file("outside")
        os.link(outside, self.lock)
        result = self.validation(self.open_lock(self.lock))
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(stat.S_IMODE(outside.stat().st_mode), 0o644)
        self.assertEqual(outside.read_text(), "preserved lock contents\n")

    def test_nonregular_file_rejected(self):
        self.lock.mkdir()
        descriptor = os.open(self.lock, os.O_RDONLY)
        self.addCleanup(os.close, descriptor)
        before = self.lock.stat().st_mode
        result = self.validation(descriptor)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.lock.stat().st_mode, before)

    @unittest.skipIf(os.geteuid() == 0, "requires nonroot test owner")
    def test_production_root_identity_rejects_nonroot_owner(self):
        self.file()
        result = self.validation(self.open_lock(self.lock), root_identity=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(stat.S_IMODE(self.lock.stat().st_mode), 0o644)


class StandaloneAcquisitionTests(LockFixture):
    def acquisition(self):
        block = LOCK_BLOCK.split("\nflock -n 9", 1)[0].replace(LOCK_PATH, str(self.lock))
        return self.run_shell("umask 022\n" + block + '\ntest "$(umask)" = 0022\n')

    def test_fresh_lock_is_private_and_umask_restored(self):
        result = self.acquisition()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(stat.S_IMODE(self.lock.stat().st_mode), 0o600)

    def test_legacy_lock_not_truncated_or_replaced_on_open(self):
        self.file()
        before = self.lock.stat()
        result = self.acquisition()
        self.assertEqual(result.returncode, 0, result.stderr)
        after = self.lock.stat()
        self.assertEqual((before.st_dev, before.st_ino, before.st_mode),
                         (after.st_dev, after.st_ino, after.st_mode))
        self.assertEqual(self.lock.read_text(), "preserved lock contents\n")

    def test_dangling_symlink_never_creates_target(self):
        outside = self.root / "outside"
        self.lock.symlink_to(outside)
        self.assertNotEqual(self.acquisition().returncode, 0)
        self.assertFalse(outside.exists())

    def test_fifo_rejected_without_blocking(self):
        os.mkfifo(self.lock)
        self.assertNotEqual(self.acquisition().returncode, 0)
        self.assertTrue(stat.S_ISFIFO(self.lock.stat().st_mode))


@unittest.skipUnless(sys.platform.startswith("linux") and shutil.which("flock"),
                     "whole lock block requires Linux /proc, stat and flock")
class InstallLockBlockTests(LockFixture):
    def block(self, environment=None, descriptor=None, suffix=""):
        block = LOCK_BLOCK.replace(LOCK_PATH, str(self.lock))
        block = block.replace("info.st_uid != 0", "info.st_uid != os.geteuid()")
        block = block.replace("'0:600:1'", f"'{os.geteuid()}:600:1'")
        return self.run_shell("umask 022\n" + block + suffix, descriptor, environment)

    def test_fresh_lock_under_normal_umask_and_restore_umask(self):
        result = self.block(suffix='\ntest "$(umask)" = 0022\n')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(stat.S_IMODE(self.lock.stat().st_mode), 0o600)

    def test_existing_legacy_lock_preserves_identity_and_contents(self):
        self.file()
        before = self.lock.stat()
        result = self.block()
        self.assertEqual(result.returncode, 0, result.stderr)
        after = self.lock.stat()
        self.assertEqual((before.st_dev, before.st_ino), (after.st_dev, after.st_ino))
        self.assertEqual(stat.S_IMODE(after.st_mode), 0o600)
        self.assertEqual(self.lock.read_text(), "preserved lock contents\n")

    def test_inherited_exact_fd9_keeps_lock_held(self):
        self.file(mode=0o600)
        descriptor = self.open_lock(self.lock)
        fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
        result = self.block({"UAP_OBSERVER_INSTALL_LOCK_FD": "9"}, descriptor)
        self.assertEqual(result.returncode, 0, result.stderr)
        competitor = self.open_lock(self.lock)
        with self.assertRaises(BlockingIOError):
            fcntl.flock(competitor, fcntl.LOCK_EX | fcntl.LOCK_NB)

    def test_wrong_or_missing_inherited_descriptor_is_rejected(self):
        self.file()
        other = self.file("other.lock")
        for value in ("8", "09", "invalid", "9"):
            with self.subTest(value=value):
                result = self.block({"UAP_OBSERVER_INSTALL_LOCK_FD": value}, self.open_lock(other))
                self.assertNotEqual(result.returncode, 0)
        self.assertNotEqual(self.block({"UAP_OBSERVER_INSTALL_LOCK_FD": "9"}).returncode, 0)
        self.assertEqual(stat.S_IMODE(other.stat().st_mode), 0o644)
        self.assertEqual(stat.S_IMODE(self.lock.stat().st_mode), 0o644)

    def test_symlink_rejected_without_creating_missing_target(self):
        outside = self.root / "outside"
        self.lock.symlink_to(outside)
        self.assertNotEqual(self.block().returncode, 0)
        self.assertFalse(outside.exists())

    def test_symlink_and_hardlink_rejected_without_outside_change(self):
        outside = self.file("outside")
        for kind in ("symlink", "hardlink"):
            with self.subTest(kind=kind):
                if kind == "symlink":
                    self.lock.symlink_to(outside)
                else:
                    os.link(outside, self.lock)
                self.assertNotEqual(self.block().returncode, 0)
                self.assertEqual(stat.S_IMODE(outside.stat().st_mode), 0o644)
                self.assertEqual(outside.read_text(), "preserved lock contents\n")
                self.lock.unlink()

    def test_busy_legacy_lock_has_no_effects(self):
        self.file()
        descriptor = self.open_lock(self.lock)
        fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
        before = self.lock.stat()
        result = self.block()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("another observer install is active", result.stderr)
        after = self.lock.stat()
        self.assertEqual((before.st_dev, before.st_ino, before.st_mode),
                         (after.st_dev, after.st_ino, after.st_mode))
        self.assertEqual(self.lock.read_text(), "preserved lock contents\n")


if __name__ == "__main__":
    unittest.main()
