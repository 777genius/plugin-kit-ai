"""Fail-closed Linux boundary for untrusted catalog children, never the producer.

Public network access is intentional. All host mounts are read-only; only a
private, caller-created case directory is writable. CI must run the Linux tests
before executing catalog packages: python -m unittest discover -s tests -p
'test_catalog_process_isolation.py' -v. No VM, daemon, or weaker fallback exists.
"""
from __future__ import annotations

import os
from pathlib import Path
import signal
import stat
import subprocess
import sys


_ENV = frozenset({
    "PATH", "LANG", "LC_ALL", "LC_CTYPE", "TZ", "CI", "NO_COLOR",
    "HOME", "CODEX_HOME", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME",
    "TMPDIR", "TMP", "TEMP", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM",
    "GIT_TERMINAL_PROMPT", "NPM_CONFIG_CACHE", "NPM_CONFIG_USERCONFIG",
    "NPM_CONFIG_PREFIX", "NPM_CONFIG_REGISTRY", "NPM_CONFIG_IGNORE_SCRIPTS",
    "NPM_CONFIG_AUDIT", "NPM_CONFIG_FUND", "NPM_CONFIG_UPDATE_NOTIFIER",
    "NPM_CONFIG_GLOBALCONFIG", "AGENTPLUGINS_HOME", "AGENTPLUGINS_DIRECTORY_ORIGIN",
    "PLUGIN_ROOT", "PLUGIN_DATA", "USERPROFILE", "APPDATA", "LOCALAPPDATA",
    "COREPACK_HOME", "NODE_REPL_HISTORY", "npm_config_audit", "npm_config_cache",
    "npm_config_fund", "npm_config_globalconfig", "npm_config_update_notifier",
    "npm_config_userconfig",
})


def _canonical(path: Path) -> Path:
    path = Path(path)
    if not path.is_absolute() or path != path.resolve(strict=True):
        raise ValueError(f"isolation paths must be absolute and contain no aliases: {path}")
    return path


def _overlap(left: Path, right: Path) -> bool:
    return left.is_relative_to(right) or right.is_relative_to(left)


def _command(argv, writable_root, cwd, env, read_only_paths, bwrap):
    root, work = _canonical(writable_root), _canonical(cwd)
    info = root.stat()
    if (not root.is_dir() or len(root.parts) < 3 or info.st_uid != os.getuid()
            or stat.S_IMODE(info.st_mode) & 0o077 or not work.is_relative_to(root)):
        raise ValueError("writable_root must be a private owned case directory containing cwd")
    workspace = os.environ.get("GITHUB_WORKSPACE")
    if workspace and _overlap(root, Path(workspace).resolve()):
        raise ValueError("case directory must not overlap the producer workspace")
    hidden = {Path(p).resolve() for p in ("/home", "/root", "/tmp", "/var/tmp", "/run")}
    for name in ("GITHUB_WORKSPACE", "RUNNER_TEMP"):
        if os.environ.get(name):
            hidden.add(Path(os.environ[name]).resolve())
    if root in hidden or any(p.is_relative_to(root) for p in hidden):
        raise ValueError("writable_root cannot contain a protected host directory")
    # Existing hardlinks could otherwise make an outside producer inode writable.
    links: dict[tuple[int, int], list[int]] = {}
    for directory, dirs, files in os.walk(root, followlinks=False):
        for name in dirs + files:
            path = Path(directory) / name
            item = path.lstat()
            if path.is_mount():
                raise ValueError("case directory cannot contain host mounts")
            if stat.S_ISREG(item.st_mode):
                count = links.setdefault((item.st_dev, item.st_ino), [0, item.st_nlink])
                count[0] += 1
                if count[1] != item.st_nlink:
                    raise ValueError("case directory hardlinks changed during validation")
    if any(observed != total for observed, total in links.values()):
        raise ValueError("case directory cannot contain hardlinks to outside inodes")
    exposed = tuple(_canonical(path) for path in read_only_paths)
    for path in exposed:
        if _overlap(path, root) or any(p.is_relative_to(path) for p in hidden):
            raise ValueError("read-only exposure must be narrow and separate from the case")
        if workspace and _overlap(path, Path(workspace).resolve()):
            raise ValueError("cannot re-expose producer workspace")
        if any(path.is_relative_to(Path(home)) for home in ("/home", "/root")):
            runner_temp = os.environ.get("RUNNER_TEMP")
            if not runner_temp or not path.is_relative_to(Path(runner_temp).resolve()):
                raise ValueError("home exposures must be explicit runner-temp tools only")
    if not argv or any(not isinstance(a, str) or "\0" in a for a in argv):
        raise ValueError("argv must be a nonempty sequence of strings")
    if isinstance(argv, (str, bytes)):
        raise ValueError("argv must not be a shell string")
    command = [str(bwrap), "--unshare-user", "--unshare-pid", "--unshare-ipc",
               "--unshare-uts", "--unshare-cgroup", "--disable-userns",
               "--die-with-parent", "--as-pid-1", "--new-session", "--cap-drop", "ALL",
               "--ro-bind", "/", "/", "--proc", "/proc", "--dev", "/dev"]
    # Hide ancestors first; subsequent mounts restore only explicit tool paths.
    for path in sorted(hidden, key=lambda p: (len(p.parts), str(p))):
        command += ["--tmpfs", str(path)]
    for path in exposed:
        command += ["--ro-bind", str(path), str(path)]
    # Ubuntu may resolve this through /run/systemd/resolve, which is hidden.
    resolver = Path("/etc/resolv.conf").resolve()
    if resolver.is_file() and any(resolver.is_relative_to(p) for p in hidden):
        command += ["--ro-bind", str(resolver), str(resolver)]
    command += ["--bind", str(root), str(root)]
    for path in sorted(hidden, key=lambda p: (-len(p.parts), str(p))):
        command += ["--remount-ro", str(path)]
    command += ["--remount-ro", "/dev", "--clearenv"]
    for name, value in sorted(env.items()):
        if name in _ENV:
            command += ["--setenv", name, value]
    command += ["--chdir", str(work), "--", *argv]
    return command


def run_isolated(argv: list[str], *, writable_root: Path, cwd: Path,
                 env: dict[str, str], read_only_paths: tuple[Path, ...],
                 timeout: int, bwrap: Path = Path("/usr/bin/bwrap")) -> subprocess.CompletedProcess[str]:
    """Run one child; namespace/setup failures remain nonzero, never run directly."""
    if sys.platform != "linux" or not bwrap.is_file() or not os.access(bwrap, os.X_OK):
        raise RuntimeError("catalog isolation requires Linux and executable /usr/bin/bwrap")
    if timeout <= 0:
        raise ValueError("timeout must be positive")
    command = _command(argv, writable_root, cwd, env, read_only_paths, bwrap)
    # No producer secrets enter the wrapper environment or any inherited fd.
    process = subprocess.Popen(command, env={"PATH": "/usr/bin:/bin"}, cwd="/",
                               stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, text=True, close_fds=True,
                               start_new_session=True)
    try:
        stdout, stderr = process.communicate(timeout=timeout)
    except BaseException:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        process.communicate()
        raise
    return subprocess.CompletedProcess(argv, process.returncode, stdout, stderr)
