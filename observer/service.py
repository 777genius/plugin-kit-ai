"""Stable-launch observation orchestration."""

from __future__ import annotations

import fcntl
import hashlib
import json
import os
import re
import shutil
import stat
import tempfile
import threading
import time
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Callable, Iterator

from .auth import AuthContext, GitHubCorroborator, OidcVerifier, ReplayStore
from .canonical import canonical_json, request_digest
from .config import Config
from .runner import SocketRunner
from .schema_validation import validate_artifact_schemas
from .signer import CacheExpiredError, SocketSigner
from .secure_files import read_owned_regular, write_new_owned

HEX64 = re.compile(r"^[a-f0-9]{64}$")
HEX40 = re.compile(r"^[a-f0-9]{40}$")
DIGEST = re.compile(r"^sha256:[a-f0-9]{64}$")
CHALLENGE_DOMAIN = b"UAP-STABLE-LAUNCH-CHALLENGE-V1\0"
CACHE_SECONDS = 30 * 60
MAX_RESPONSE_BYTES = 8 << 20
MAX_CACHE_RUNS = 64
MAX_CACHE_BYTES = 512 << 20


class RequestValidationError(ValueError):
    """A client request that fails the canonical stable-launch contract."""


class WorkBusyError(ValueError):
    """A different protected observation is already running."""


class WorkCoordinator:
    def __init__(self) -> None:
        self._condition = threading.Condition()
        self._active_digest: str | None = None

    def enter(self, digest: str, deadline: float) -> bool:
        with self._condition:
            if self._active_digest is None:
                self._active_digest = digest
                return True
            if self._active_digest != digest:
                raise WorkBusyError("a different observer request is already running")
            while self._active_digest == digest:
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise TimeoutError("timed out waiting for identical observer request")
                self._condition.wait(remaining)
            return False

    def leave(self, digest: str) -> None:
        with self._condition:
            if self._active_digest == digest:
                self._active_digest = None
                self._condition.notify_all()


class ObserverService:
    def __init__(
        self, config: Config, *, verifier: OidcVerifier | None = None,
        corroborator: GitHubCorroborator | None = None, replay: ReplayStore | None = None,
        runner: SocketRunner | None = None, signer: SocketSigner | None = None,
        now: Callable[[], float] = time.time, monotonic: Callable[[], float] = time.monotonic,
    ):
        self.config, self.now, self.monotonic = config, now, monotonic
        self.verifier = verifier or OidcVerifier(config, now=now)
        self.corroborator = corroborator or GitHubCorroborator(config)
        self.replay = replay or ReplayStore(config.state_root / "replay", now)
        self.runner = runner or SocketRunner(
            config.runner_socket, config.runner_source_path, config.runner_source_digest,
            config.runner_timeout_seconds, runner_user=config.runner_user,
            enforce_root_ownership=config.enforce_root_ownership,
        )
        self.signer = signer or SocketSigner(config.signer_socket, config.public_key_base64, config.key_id)
        self._coordinator = WorkCoordinator()
        config.state_root.mkdir(parents=True, exist_ok=True, mode=0o700)

    def observe(
        self, request: Any, bearer_token: str, *, deadline: float | None = None,
        on_authenticated: Callable[[], None] | None = None,
    ) -> bytes:
        deadline = deadline if deadline is not None else self.monotonic() + 900
        try:
            validated = validate_request(request, self.config)
        except ValueError as error:
            raise RequestValidationError("observer request is invalid") from error
        auth = self.authenticate(bearer_token, on_authenticated=on_authenticated)
        return self._observe_validated(validated, auth, deadline=deadline)

    def authenticate(
        self, bearer_token: str, *, on_authenticated: Callable[[], None] | None = None,
    ) -> AuthContext:
        """Authenticate, publicly corroborate, and replay-charge before body I/O."""
        auth = self.verifier.verify(bearer_token)
        self.corroborator.corroborate(auth)
        self.replay.consume(auth.claims["jti"], auth.claims["exp"])
        if on_authenticated is not None:
            on_authenticated()
        return auth

    def observe_authenticated(
        self, request: Any, auth: AuthContext, *, deadline: float,
    ) -> bytes:
        try:
            validated = validate_request(request, self.config)
        except ValueError as error:
            raise RequestValidationError("observer request is invalid") from error
        return self._observe_validated(validated, auth, deadline=deadline)

    def _observe_validated(
        self, validated: dict[str, Any], auth: AuthContext, *, deadline: float,
    ) -> bytes:
        if any(str(auth.claims[key]) != str(validated["github"][key]) for key in ("sha", "run_id", "run_attempt")):
            raise RequestValidationError("request and authenticated GitHub identities differ")
        if validated["catalog_repository"] != auth.policy.repository:
            raise RequestValidationError("request catalog repository is not allowlisted")
        digest = request_digest(validated)
        leader = self._coordinator.enter(digest, deadline)
        if not leader:
            cached = self._cached_response(validated, digest)
            if cached is None:
                raise ValueError("coalesced observer request did not produce a cache entry")
            return cached
        try:
            with self._process_lock():
                cached = self._cached_response(validated, digest)
                if cached is not None:
                    return cached
                return self._execute(validated, auth, digest, deadline)
        finally:
            self._coordinator.leave(digest)

    @contextmanager
    def _process_lock(self) -> Iterator[None]:
        lock_path = self.config.state_root / "observer.lock"
        try:
            write_new_owned(lock_path, b"")
        except FileExistsError:
            pass
        descriptor = os.open(lock_path, os.O_RDWR | os.O_NOFOLLOW | os.O_CLOEXEC)
        try:
            info = os.fstat(descriptor)
            if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o077:
                raise ValueError("observer process lock is not protected")
            try:
                fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                raise WorkBusyError("observer process lock is held") from None
            yield
        finally:
            fcntl.flock(descriptor, fcntl.LOCK_UN)
            os.close(descriptor)

    def _target(self, request: dict[str, Any], digest: str) -> Path:
        github = request["github"]
        return self.config.state_root / "runs" / f"{github['run_id']}-{github['run_attempt']}-{digest}"

    def _cached_response(self, request: dict[str, Any], digest: str) -> bytes | None:
        response_path = self._target(request, digest) / "response.json"
        try:
            data = read_owned_regular(response_path, MAX_RESPONSE_BYTES, owner_uid=os.geteuid())
            self.signer.verify_cached(data, challenge=request["challenge"]["value"], now=self.now())
        except (FileNotFoundError, CacheExpiredError):
            return None
        if hasattr(self.runner, "transaction"):
            self.runner.transaction(request["challenge"]["value"], "commit")
        return data

    def _execute(self, request: dict[str, Any], auth: AuthContext, digest: str, deadline: float) -> bytes:
        claims = auth.claims
        runs = self.config.state_root / "runs"
        runs.mkdir(parents=True, exist_ok=True, mode=0o700)
        self._retain_cache(runs)
        target = self._target(request, digest)
        temporary = Path(tempfile.mkdtemp(prefix=".pending-", dir=runs))
        os.chmod(temporary, 0o700)
        try:
            if hasattr(self.runner, "transaction"):
                self.runner.transaction(request["challenge"]["value"], "rollback", deadline=deadline)
            github_attestation = {
                "subject": claims["sub"], "repository": claims["repository"],
                "repository_owner": claims["repository_owner"],
                "repository_id": str(claims["repository_id"]),
                "repository_owner_id": str(claims["repository_owner_id"]),
                "ref": claims["ref"], "environment": claims["environment"],
                "workflow_ref": claims["workflow_ref"],
                "job_workflow_ref": claims["job_workflow_ref"],
                "sha": claims["sha"],
                "run_id": claims["run_id"], "run_attempt": claims["run_attempt"],
                "workflow": "launch-evidence-e2e.yml", "job": auth.policy.job_name_suffix,
                "challenge": request["challenge"]["value"],
            }
            artifacts = self.runner.run(
                temporary, {"request": request, "github_attestation": github_attestation}, deadline=deadline,
            )
            validate_artifact_schemas(
                artifacts, challenge=request["challenge"]["value"],
                scenario_contract_digest=request["scenario_contract_digest"],
                expected_bindings=request,
            )
            unsigned = {
                "schema_version": 1, "challenge": request["challenge"]["value"],
                "signed_at": datetime.fromtimestamp(self.now(), timezone.utc).isoformat().replace("+00:00", "Z"),
                "key_id": self.config.key_id, "artifacts": artifacts,
            }
            bundle = {**unsigned, "signature": self.signer.sign(unsigned, deadline=deadline)}
            response = canonical_json(bundle)
            if len(response) > MAX_RESPONSE_BYTES:
                raise ValueError("observer response exceeds size bound")
            write_new_owned(temporary / "response.json", response)
            os.utime(temporary / "response.json", (self.now(), self.now()))
            if target.exists():
                cached = self._cached_response(request, digest)
                if cached is not None:
                    shutil.rmtree(temporary, ignore_errors=True)
                    return cached
                retired = runs / f".expired-{target.name}-{os.getpid()}"
                os.replace(target, retired)
                os.replace(temporary, target)
                shutil.rmtree(retired, ignore_errors=True)
                self._sync_published(target)
                if hasattr(self.runner, "transaction"):
                    self.runner.transaction(request["challenge"]["value"], "commit", deadline=deadline)
                return response
            os.replace(temporary, target)
            self._sync_published(target)
            if hasattr(self.runner, "transaction"):
                self.runner.transaction(request["challenge"]["value"], "commit", deadline=deadline)
            return response
        except Exception:
            if temporary.exists():
                shutil.rmtree(temporary)
            raise

    @staticmethod
    def _sync_published(target: Path) -> None:
        response_fd = os.open(target / "response.json", os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
        directory_fd = os.open(target, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
        parent_fd = os.open(target.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
        try:
            os.fsync(response_fd)
            os.fsync(directory_fd)
            os.fsync(parent_fd)
        finally:
            os.close(response_fd)
            os.close(directory_fd)
            os.close(parent_fd)

    def _retain_cache(self, runs: Path) -> None:
        """Bound observer-owned cache entries without following hostile links."""
        entries: list[tuple[float, int, Path]] = []
        for path in runs.iterdir():
            info = path.lstat()
            if path.name.startswith(".pending-"):
                if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid():
                    raise ValueError("observer cache contains an untrusted pending entry")
                continue
            if path.name.startswith(".expired-"):
                if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid():
                    raise ValueError("observer cache contains an untrusted retired entry")
                shutil.rmtree(path)
                continue
            if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o077:
                raise ValueError("observer cache contains an untrusted entry")
            children = list(path.iterdir())
            if len(children) != 1 or children[0].name != "response.json":
                raise ValueError("observer cache entry is incomplete")
            response_info = children[0].lstat()
            if not stat.S_ISREG(response_info.st_mode) or response_info.st_uid != os.geteuid() or response_info.st_nlink != 1:
                raise ValueError("observer cache response is untrusted")
            entries.append((response_info.st_mtime, response_info.st_size, path))
        total = sum(size for _, size, _ in entries)
        for modified, size, path in sorted(entries):
            if len(entries) < MAX_CACHE_RUNS and total + MAX_RESPONSE_BYTES <= MAX_CACHE_BYTES:
                break
            shutil.rmtree(path)
            entries.remove((modified, size, path))
            total -= size


def validate_request(value: Any, config: Config) -> dict[str, Any]:
    fields = {
        "schema_version", "purpose", "catalog_repository", "cli_release_repository",
        "cli_release_tag", "release_manifest_digest", "release_checksums_digest",
        "directory_digest", "scenario_contract_digest", "github", "challenge",
    }
    if not isinstance(value, dict) or set(value) != fields or value.get("schema_version") != 1 or value.get("purpose") != "stable-launch-e2e":
        raise ValueError("observer request is not canonical")
    if value.get("cli_release_repository") != config.cli_release_repository or value.get("cli_release_tag") != config.cli_release_tag:
        raise ValueError("observer request release identity is not allowed")
    if not all(DIGEST.fullmatch(str(value.get(key))) for key in ("release_manifest_digest", "release_checksums_digest", "directory_digest", "scenario_contract_digest")):
        raise ValueError("observer request digests are invalid")
    github, challenge = value.get("github"), value.get("challenge")
    if not isinstance(github, dict) or set(github) != {"sha", "run_id", "run_attempt"}:
        raise ValueError("observer request GitHub identity is invalid")
    if not HEX40.fullmatch(str(github["sha"])) or not str(github["run_id"]).isdigit() or not str(github["run_attempt"]).isdigit():
        raise ValueError("observer request GitHub identity is invalid")
    challenge_fields = {"value", "nonce", "github_sha", "run_id", "run_attempt", "release_manifest_digest", "directory_digest", "scenario_contract_digest", "root_id"}
    if not isinstance(challenge, dict) or set(challenge) != challenge_fields or not all(isinstance(item, str) for item in challenge.values()):
        raise ValueError("observer request challenge is invalid")
    if any(challenge[a] != github[b] for a, b in (("github_sha", "sha"), ("run_id", "run_id"), ("run_attempt", "run_attempt"))):
        raise ValueError("observer request challenge identity differs")
    if challenge["release_manifest_digest"] != value["release_manifest_digest"] or challenge["directory_digest"] != value["directory_digest"]:
        raise ValueError("observer request challenge digests differ")
    if challenge["scenario_contract_digest"] != value["scenario_contract_digest"]:
        raise ValueError("observer request scenario contract differs")
    if not all(HEX64.fullmatch(challenge[key]) for key in ("value", "nonce", "root_id")):
        raise ValueError("observer request challenge is invalid")
    framed = canonical_json({
        "github_sha": challenge["github_sha"], "run_id": challenge["run_id"],
        "run_attempt": challenge["run_attempt"], "release_manifest_digest": challenge["release_manifest_digest"],
        "directory_digest": challenge["directory_digest"], "root_id": challenge["root_id"], "nonce": challenge["nonce"],
        "scenario_contract_digest": challenge["scenario_contract_digest"],
    })
    if challenge["value"] != hashlib.sha256(CHALLENGE_DOMAIN + framed).hexdigest():
        raise ValueError("observer request challenge is invalid")
    return value
