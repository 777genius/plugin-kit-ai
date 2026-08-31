"""Portable guards plus mandatory Linux execution probes (no models or services)."""
from __future__ import annotations

import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location(
    "catalog_process_isolation", Path(__file__).resolve().parents[1]
    / "scripts" / "catalog_process_isolation.py")
isolation = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(isolation)


class GuardTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.base = Path(self.temporary.name).resolve()
        self.case = self.base / "case"
        self.case.mkdir(mode=0o700)
        self.environment = patch.dict(os.environ, {"RUNNER_TEMP": str(self.base)}, clear=True)
        self.environment.start()
        self.addCleanup(self.environment.stop)

    def command(self, **changes):
        args = dict(argv=["/usr/bin/true"], writable_root=self.case, cwd=self.case,
                    env={"HOME": str(self.case), "GITHUB_TOKEN": "secret"},
                    read_only_paths=(), bwrap=Path("/usr/bin/bwrap"))
        args.update(changes)
        return isolation._command(**args)

    def test_boundary_flags_and_no_secret_environment(self):
        command = self.command()
        for flag in ("--unshare-user", "--unshare-pid", "--disable-userns",
                     "--as-pid-1", "--die-with-parent", "--clearenv", "--new-session"):
            self.assertIn(flag, command)
        self.assertNotIn("--unshare-net", command)
        self.assertNotIn("GITHUB_TOKEN", command)
        self.assertNotIn("secret", command)
        self.assertEqual(command[-2:], ["--", "/usr/bin/true"])
        self.assertEqual(command.count("--bind"), 1)

    def test_rejects_shell_string_and_outside_cwd(self):
        for changes in ({"argv": "echo unsafe"}, {"argv": []}, {"cwd": self.base}):
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                self.command(**changes)

    def test_rejects_shared_root_and_workspace_overlap(self):
        self.case.chmod(0o755)
        with self.assertRaises(ValueError):
            self.command()
        self.case.chmod(0o700)
        with patch.dict(os.environ, {"GITHUB_WORKSPACE": str(self.case)}):
            with self.assertRaises(ValueError):
                self.command()
        with self.assertRaises(ValueError):
            self.command(writable_root=self.base, cwd=self.base)

    def test_rejects_aliases_broad_and_overlapping_exposure(self):
        alias = self.base / "alias"
        alias.symlink_to(self.case, target_is_directory=True)
        for changes in ({"writable_root": alias}, {"read_only_paths": (self.base,)},
                        {"read_only_paths": (self.case,)}, {"read_only_paths": (alias,)}):
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                self.command(**changes)

    def test_rejects_hardlink_to_outside_inode(self):
        outside = self.base / "producer.py"
        outside.write_text("trusted")
        os.link(outside, self.case / "innocent")
        with self.assertRaisesRegex(ValueError, "hardlinks"):
            self.command()

    def test_allows_fully_contained_hardlinks(self):
        original = self.case / "package"
        original.write_text("installed")
        nested = self.case / "nested"
        nested.mkdir()
        os.link(original, nested / "package-link")
        self.assertIn("--bind", self.command())

    def test_tools_are_exposed_readonly_after_temp_hiding(self):
        tools = self.base / "exposure-tools"
        tools.mkdir()
        command = self.command(read_only_paths=(tools,))
        mount = command.index(str(tools))
        self.assertEqual(command[mount - 1], "--ro-bind")
        self.assertLess(command.index("--tmpfs"), mount)
        self.assertGreater(command.index("--remount-ro"), mount)

    def test_missing_wrapper_never_launches_unisolated_child(self):
        with patch.object(isolation.subprocess, "Popen") as popen:
            with self.assertRaises(RuntimeError):
                isolation.run_isolated(["true"], writable_root=self.case, cwd=self.case,
                                       env={}, read_only_paths=(), timeout=1,
                                       bwrap=self.base / "missing")
            popen.assert_not_called()


@unittest.skipUnless(sys.platform == "linux", "real isolation requires Linux namespaces")
class LinuxIsolationTests(GuardTests):
    def setUp(self):
        super().setUp()
        self.workspace = self.base / "workspace"
        self.workspace.mkdir()
        self.tools = self.base / "tools"
        self.tools.mkdir()
        self.output = self.base / "github-output"
        self.targets = [self.workspace / "producer.py", self.tools / "cli",
                        self.base / "evidence.json", self.output]
        for path in self.targets:
            path.write_text("trusted")
        os.environ.update(GITHUB_WORKSPACE=str(self.workspace),
                          GITHUB_OUTPUT=str(self.output), PARENT_SECRET="private-token")
        self.env = {**os.environ, "HOME": str(self.case), "TMPDIR": str(self.case),
                    "PATH": "/usr/bin:/bin"}
        # system Python is outside masked runner tool directories on Ubuntu.
        self.python = "/usr/bin/python3"

    def run_child(self, code, *, timeout=10):
        result = isolation.run_isolated(
            [self.python, "-c", code], writable_root=self.case, cwd=self.case,
            env=self.env, read_only_paths=(self.tools,), timeout=timeout)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result

    def test_real_write_denial_parent_proc_and_environment_absent(self):
        paths = [str(path) for path in self.targets]
        probe = f"""
import json, os
from pathlib import Path
denied = []
for target in {paths!r}:
    try:
        Path(target).write_text('tampered')
    except OSError:
        denied.append(target)
Path('owned').write_text('ok')
visible = []
for entry in Path('/proc').iterdir():
    if entry.name.isdigit():
        try:
            visible.append((entry / 'environ').read_bytes().decode(errors='replace'))
        except OSError:
            pass
assert not any('private-token' in value for value in visible)
assert not Path('/proc/{os.getpid()}/root').exists()
assert 'GITHUB_OUTPUT' not in os.environ
assert 'PARENT_SECRET' not in os.environ
assert Path({str(self.tools / 'cli')!r}).read_text() == 'trusted'
print(json.dumps(denied))
"""
        result = self.run_child(probe)
        self.assertEqual(json.loads(result.stdout), paths)
        self.assertEqual((self.case / "owned").read_text(), "ok")
        for path in self.targets:
            self.assertEqual(path.read_text(), "trusted")

    def test_nested_user_namespace_is_denied_despite_apparmor_allowance(self):
        self.run_child("""
import ctypes, errno, os
libc = ctypes.CDLL(None, use_errno=True)
libc.unshare.argtypes = [ctypes.c_int]
libc.unshare.restype = ctypes.c_int
before = os.readlink('/proc/self/ns/user')
result = libc.unshare(0x10000000)  # CLONE_NEWUSER
error = ctypes.get_errno()
assert result == -1, 'nested user namespace escaped --disable-userns'
assert error in (errno.EPERM, errno.ENOSPC), ('unexpected unshare failure', error)
assert os.readlink('/proc/self/ns/user') == before
""")

    def test_detached_grandchild_dies_when_initial_child_exits(self):
        self.run_child("""
import os, time
from pathlib import Path
if os.fork() == 0:
    os.setsid()
    os.close(1)
    os.close(2)
    Path('started').write_text('yes')
    time.sleep(0.4)
    Path('escaped').write_text('bad')
    os._exit(0)
while not Path('started').exists():
    time.sleep(0.01)
""")
        time.sleep(0.6)
        self.assertFalse((self.case / "escaped").exists())

    def test_timeout_kills_detached_descendant(self):
        with self.assertRaises(subprocess.TimeoutExpired):
            self.run_child("""
import os, time
from pathlib import Path
if os.fork() == 0:
    os.setsid()
    os.close(1)
    os.close(2)
    time.sleep(1.5)
    Path('escaped').write_text('bad')
    os._exit(0)
time.sleep(30)
""", timeout=1)
        time.sleep(0.7)
        self.assertFalse((self.case / "escaped").exists())


if __name__ == "__main__":
    unittest.main()
