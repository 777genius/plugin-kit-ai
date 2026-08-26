"""GitHub Actions OIDC verification and public job corroboration."""

from __future__ import annotations

import base64
import hashlib
import json
import os
import re
import stat
import threading
import time
import urllib.request
from collections import OrderedDict
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable

from cryptography.hazmat.primitives.asymmetric import padding, rsa
from cryptography.hazmat.primitives.hashes import SHA256

from .config import Config, IdentityPolicy

HEX40 = re.compile(r"^[a-f0-9]{40}$")
DIGITS = re.compile(r"^[1-9][0-9]*$")
JTI = re.compile(r"^[A-Za-z0-9._:-]{8,200}$")
FIXED_HTTPS_PROXY = "http://127.0.0.2:8766"


class AuthenticationError(ValueError):
    """A deliberately sanitized authentication failure."""


def _b64url(value: str) -> bytes:
    if not isinstance(value, str) or not re.fullmatch(r"[A-Za-z0-9_-]+", value):
        raise AuthenticationError("invalid GitHub OIDC token")
    try:
        return base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))
    except Exception:
        raise AuthenticationError("invalid GitHub OIDC token") from None


class JsonFetcher:
    def __init__(self) -> None:
        self._opener = urllib.request.build_opener(
            urllib.request.ProxyHandler({"https": FIXED_HTTPS_PROXY})
        )

    def __call__(self, url: str) -> Any:
        request = urllib.request.Request(
            url,
            headers={"Accept": "application/vnd.github+json", "User-Agent": "uap-stable-launch-observer/1"},
        )
        with self._opener.open(request, timeout=10) as response:
            if response.status != 200 or response.geturl() != url:
                raise AuthenticationError("GitHub identity corroboration failed")
            data = response.read((2 << 20) + 1)
        if len(data) > 2 << 20:
            raise AuthenticationError("GitHub identity corroboration failed")
        try:
            return json.loads(data)
        except (ValueError, UnicodeError):
            raise AuthenticationError("GitHub identity corroboration failed") from None


class JwksCache:
    """A bounded-stale cache; an outage can never extend key trust forever."""

    REFRESH_SECONDS = 300
    RETRY_SECONDS = 30
    MAX_STALE_SECONDS = 900

    def __init__(self, url: str, fetch: Callable[[str], Any], monotonic: Callable[[], float] = time.monotonic):
        self._url, self._fetch, self._monotonic = url, fetch, monotonic
        self._expires = 0.0
        self._keys: dict[str, Any] = {}
        self._last_success: float | None = None
        self._generation = 0
        self._next_unknown_refresh = 0.0
        self._negative: OrderedDict[str, int] = OrderedDict()
        self._lock = threading.Lock()

    def key(self, kid: str) -> rsa.RSAPublicKey:
        with self._lock:
            now = self._monotonic()
            refreshed_expired_cache = False
            hard_stale = self._last_success is None or now - self._last_success >= self.MAX_STALE_SECONDS
            if now >= self._expires or hard_stale:
                self._refresh(now, required=not self._keys or hard_stale)
                refreshed_expired_cache = True
            key = self._keys.get(kid)
            if key is not None:
                return key
            if self._negative.get(kid) == self._generation or now < self._next_unknown_refresh:
                raise AuthenticationError("GitHub OIDC signing key is not trusted")
            self._next_unknown_refresh = now + 60
            if not refreshed_expired_cache:
                self._refresh(now, required=False)
            key = self._keys.get(kid)
            if key is not None:
                return key
            self._negative[kid] = self._generation
            self._negative.move_to_end(kid)
            while len(self._negative) > 128:
                self._negative.popitem(last=False)
            raise AuthenticationError("GitHub OIDC signing key is not trusted")

    def _refresh(self, now: float, *, required: bool) -> None:
        try:
            value = self._fetch(self._url)
            keys = value.get("keys") if isinstance(value, dict) else None
            if not isinstance(keys, list) or not 1 <= len(keys) <= 32:
                raise ValueError("invalid JWKS")
            parsed: dict[str, rsa.RSAPublicKey] = {}
            for item in keys:
                if not isinstance(item, dict) or item.get("kty") != "RSA" or item.get("use") != "sig" or item.get("alg") != "RS256":
                    continue
                key_id, n, e = item.get("kid"), item.get("n"), item.get("e")
                if not all(isinstance(x, str) and x for x in (key_id, n, e)):
                    continue
                if len(key_id) > 200 or not 340 <= len(n) <= 1368 or len(e) > 8:
                    continue
                exponent, modulus = int.from_bytes(_b64url(e), "big"), int.from_bytes(_b64url(n), "big")
                if not 2048 <= modulus.bit_length() <= 8192 or exponent < 3 or exponent > 0xFFFFFFFF or exponent % 2 == 0:
                    continue
                parsed[key_id] = rsa.RSAPublicNumbers(exponent, modulus).public_key()
            if not parsed:
                raise ValueError("empty JWKS")
        except Exception:
            self._expires = now + self.RETRY_SECONDS
            self._next_unknown_refresh = max(self._next_unknown_refresh, now + 60)
            if required:
                self._keys.clear()
                raise AuthenticationError("GitHub OIDC key discovery failed") from None
            return
        self._keys, self._expires = parsed, now + self.REFRESH_SECONDS
        self._last_success = now
        self._generation += 1
        self._negative.clear()


@dataclass(frozen=True)
class AuthContext:
    claims: dict[str, Any]
    policy: IdentityPolicy


class OidcVerifier:
    def __init__(
        self, config: Config, fetch: Callable[[str], Any] | None = None,
        now: Callable[[], float] = time.time, monotonic: Callable[[], float] = time.monotonic,
    ):
        self.config, self.fetch, self.now = config, fetch or JsonFetcher(), now
        self.jwks = JwksCache(config.jwks_url, self.fetch, monotonic)

    def verify(self, token: str, github: dict[str, Any] | None = None) -> AuthContext:
        if not isinstance(token, str) or len(token) > 16_384:
            raise AuthenticationError("invalid GitHub OIDC token")
        parts = token.split(".")
        if len(parts) != 3:
            raise AuthenticationError("invalid GitHub OIDC token")
        try:
            header = json.loads(_b64url(parts[0]))
            claims = json.loads(_b64url(parts[1]))
        except (ValueError, UnicodeError, json.JSONDecodeError):
            raise AuthenticationError("invalid GitHub OIDC token") from None
        if not isinstance(header, dict) or header.get("alg") != "RS256" or header.get("typ") != "JWT":
            raise AuthenticationError("GitHub OIDC must use RS256")
        kid = header.get("kid")
        if not isinstance(kid, str) or not kid or len(kid) > 200:
            raise AuthenticationError("GitHub OIDC key id is missing")
        try:
            self.jwks.key(kid).verify(_b64url(parts[2]), (parts[0] + "." + parts[1]).encode("ascii"), padding.PKCS1v15(), SHA256())
        except AuthenticationError:
            raise
        except Exception:
            raise AuthenticationError("GitHub OIDC signature is invalid") from None
        if not isinstance(claims, dict):
            raise AuthenticationError("invalid GitHub OIDC claims")
        policy = self._validate_claims(claims, github)
        return AuthContext(claims=claims, policy=policy)

    def _validate_claims(self, claims: dict[str, Any], github: dict[str, Any] | None) -> IdentityPolicy:
        required = {
            "iss", "aud", "sub", "iat", "nbf", "exp", "jti", "repository",
            "repository_owner", "repository_id", "repository_owner_id", "ref", "sha", "run_id",
            "run_attempt", "environment", "workflow_ref", "job_workflow_ref",
            "workflow_sha", "job_workflow_sha", "workflow", "event_name", "ref_type",
        }
        if any(key not in claims for key in required):
            raise AuthenticationError("GitHub OIDC claims are incomplete")
        if claims["iss"] != self.config.issuer or claims["aud"] != self.config.audience:
            raise AuthenticationError("GitHub OIDC issuer or audience is not allowed")
        current = int(self.now())
        if any(type(claims[key]) is not int for key in ("iat", "nbf", "exp")):
            raise AuthenticationError("GitHub OIDC time claims are invalid")
        if claims["iat"] > current + 30 or claims["nbf"] > current + 30 or claims["exp"] < current - 30:
            raise AuthenticationError("GitHub OIDC token is expired or not yet valid")
        if claims["exp"] <= claims["iat"] or claims["exp"] - claims["iat"] > 600:
            raise AuthenticationError("GitHub OIDC token lifetime is invalid")
        if not isinstance(claims["jti"], str) or not JTI.fullmatch(claims["jti"]):
            raise AuthenticationError("GitHub OIDC jti is invalid")
        if github is not None:
            if not isinstance(github, dict) or set(github) != {"sha", "run_id", "run_attempt"}:
                raise AuthenticationError("request GitHub identity is invalid")
            if not HEX40.fullmatch(str(github["sha"])) or not all(DIGITS.fullmatch(str(github[key])) for key in ("run_id", "run_attempt")):
                raise AuthenticationError("request GitHub identity is invalid")
            if any(str(claims[key]) != str(github[key]) for key in ("sha", "run_id", "run_attempt")):
                raise AuthenticationError("request and OIDC GitHub identities differ")
        for policy in self.config.policies:
            owner = policy.repository.split("/", 1)[0]
            expected_sub = f"repo:{policy.repository}:environment:{policy.environment}"
            exact = {
                "repository": policy.repository, "repository_owner": owner,
                "repository_id": policy.repository_id,
                "repository_owner_id": policy.repository_owner_id, "ref": policy.ref,
                "environment": policy.environment, "ref_type": policy.ref_type,
                "workflow_ref": policy.workflow_ref, "job_workflow_ref": policy.job_workflow_ref,
                "workflow": policy.workflow, "sub": expected_sub,
            }
            if (
                all(str(claims[key]) == value for key, value in exact.items())
                and claims["event_name"] in policy.event_names
                and claims["workflow_sha"] == claims["sha"]
                and claims["job_workflow_sha"] == claims["sha"]
            ):
                return policy
        raise AuthenticationError("GitHub OIDC identity is not allowlisted")


class ReplayStore:
    MAX_ENTRIES = 4096
    def __init__(self, root: Path, now: Callable[[], float] = time.time):
        self.root, self.now = root, now
        self._lock = threading.Lock()

    def consume(self, jti: str, expires_at: int) -> None:
        self.root.mkdir(parents=True, exist_ok=True, mode=0o700)
        target = self.root / hashlib.sha256(jti.encode()).hexdigest()
        with self._lock:
            entries = list(self.root.iterdir())
            retained = 0
            for old in entries:
                try:
                    info = old.lstat()
                    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_nlink != 1:
                        raise AuthenticationError("GitHub OIDC replay store is not trusted")
                    if int(old.read_text()) < int(self.now()) - 60:
                        old.unlink()
                    else:
                        retained += 1
                except AuthenticationError:
                    raise
                except (OSError, ValueError):
                    raise AuthenticationError("GitHub OIDC replay store is not trusted") from None
            if retained >= self.MAX_ENTRIES:
                raise AuthenticationError("GitHub OIDC replay store quota exceeded")
            try:
                descriptor = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            except FileExistsError:
                raise AuthenticationError("GitHub OIDC token was already used") from None
            with os.fdopen(descriptor, "w") as stream:
                stream.write(str(expires_at))


class GitHubCorroborator:
    def __init__(self, config: Config, fetch: Callable[[str], Any] | None = None):
        self.config, self.fetch = config, fetch or JsonFetcher()

    def corroborate(self, auth: AuthContext) -> None:
        c, p = auth.claims, auth.policy
        root = self.config.github_api_url.rstrip("/")
        run_url = f"{root}/repos/{p.repository}/actions/runs/{c['run_id']}/attempts/{c['run_attempt']}"
        jobs_url = run_url + "/jobs?filter=latest&per_page=100"
        run, jobs = self.fetch(run_url), self.fetch(jobs_url)
        workflow_path = p.workflow_ref.split("@", 1)[0].removeprefix(p.repository + "/")
        if not isinstance(run, dict) or str(run.get("run_attempt")) != str(c["run_attempt"]) or run.get("head_sha") != c["sha"]:
            raise AuthenticationError("public GitHub run does not corroborate OIDC identity")
        if str(run.get("id")) != str(c["run_id"]) or run.get("path") != workflow_path or run.get("event") != c["event_name"]:
            raise AuthenticationError("public GitHub run does not corroborate OIDC identity")
        repository = run.get("repository")
        owner = repository.get("owner") if isinstance(repository, dict) else None
        if not isinstance(repository, dict) or repository.get("full_name") != p.repository or str(repository.get("id")) != p.repository_id or not isinstance(owner, dict) or str(owner.get("id")) != p.repository_owner_id:
            raise AuthenticationError("public GitHub repository does not corroborate OIDC identity")
        values = jobs.get("jobs") if isinstance(jobs, dict) else None
        if not isinstance(values, list):
            raise AuthenticationError("public GitHub jobs response is invalid")
        matching = [
            job for job in values
            if isinstance(job, dict)
            and (job.get("name") == p.job_name_suffix or str(job.get("name", "")).endswith(" / " + p.job_name_suffix))
        ]
        if len(matching) != 1:
            raise AuthenticationError("public GitHub job does not corroborate OIDC identity")
        job = matching[0]
        if str(job.get("run_id")) != str(c["run_id"]) or str(job.get("run_attempt")) != str(c["run_attempt"]) or job.get("head_sha") != c["sha"]:
            raise AuthenticationError("public GitHub job does not corroborate OIDC identity")
        if job.get("workflow_name") != p.workflow:
            raise AuthenticationError("public GitHub job workflow does not corroborate OIDC identity")
        if job.get("status") != "in_progress":
            raise AuthenticationError("public GitHub job is not active")
