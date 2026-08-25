#!/usr/bin/env python3
"""Verify challenge-bound responses from the protected launch observer."""

from __future__ import annotations

import base64
import binascii
import hashlib
import json
import re
from datetime import datetime, timedelta, timezone
from typing import Any
from urllib.parse import parse_qsl, urlsplit


SIGNATURE_DOMAIN = b"UAP-STABLE-LAUNCH-OBSERVER-BUNDLE-V1\0"
MAX_BUNDLE_AGE = timedelta(minutes=30)
FORBIDDEN_KEY = re.compile(
    r"(?i)(?:api[_-]?key|access[_-]?token|token|secret|signature|sig|passw(?:or)?d|"
    r"cookie|authorization|credential|oauth[_-]?(?:code|state|token)|x[_-]?amz[_-]?[a-z0-9_-]+)"
)
SENSITIVE_ARGUMENT = re.compile(
    r"(?i)^(?:api[_-]?key|access[_-]?token|token|secret|passw(?:or)?d|cookie|"
    r"auth(?:entication|orization)?|credential|signature|sig|aws-secret-access-key|"
    r"aws-session-token|x-amz-(?:signature|security-token|credential)|oauth[_-]?(?:code|state|token))$"
)
ENVIRONMENT_ARGUMENT = re.compile(r"(?i)^(?:env|environment|env[_-]?var)$")
ENVIRONMENT_ASSIGNMENT = re.compile(r"^([A-Za-z_][A-Za-z0-9_]*)(=)(.*)$", re.DOTALL)
SAFE_DIGEST = re.compile(r"^(?:sha256:)?[a-fA-F0-9]{40,128}$")
POSIX_ABSOLUTE_PATH = re.compile(r"(?<![A-Za-z0-9:/])/(?!/)[^\s,;]+")
WINDOWS_ABSOLUTE_PATH = re.compile(r"(?i)(?<![A-Za-z0-9])(?:[A-Z]:[\\/]|\\\\)[^\s,;]+")
REDACTION = re.compile(r"^<redacted:(?:credential|absolute-path):sha256:[a-f0-9]{64}>$")
URL = re.compile(r"(?i)https?://[^\s,;]+")
FILE_URL = re.compile(r"(?i)file:///(?:[^\s,;]+)")
LABELED_ABSOLUTE_PATH = re.compile(r"(?i)\b(path|file|dir|directory|root|cwd|home|workspace):(/[^\s,;]+)")
SENSITIVE_QUERY = re.compile(
    r"(?i)^(?:code|state|api[_-]?key|access[_-]?token|token|secret|credential|signature|sig|"
    r"oauth[_-]?(?:code|state|token)|x[_-]?amz[_-]?[a-z0-9_-]+)$"
)


def canonical_json(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()


def signed_payload(bundle: dict[str, Any]) -> bytes:
    return SIGNATURE_DOMAIN + canonical_json({key: value for key, value in bundle.items() if key != "signature"})


def _redaction(kind: str, value: str) -> str:
    return f"<redacted:{kind}:sha256:{hashlib.sha256(value.encode()).hexdigest()}>"


def _safe_reference(value: str) -> bool:
    if SAFE_DIGEST.fullmatch(value) or REDACTION.fullmatch(value):
        return True
    parsed = urlsplit(value)
    return parsed.scheme in {"http", "https"} and bool(parsed.hostname) and not parsed.username and not parsed.password


def _argument_name(value: str) -> str:
    return value.lstrip("-/").replace("_", "-")


def _split_argument(value: str) -> tuple[str, str, str] | None:
    stripped = value.lstrip("-/")
    prefix = value[:len(value) - len(stripped)]
    for separator in ("=", ":"):
        if separator in stripped:
            name, argument = stripped.split(separator, 1)
            return prefix + name, separator, argument
    return None


def _sanitize_path_text(value: str) -> str:
    def replace(match: re.Match[str]) -> str:
        path = match.group(0)
        return _redaction("absolute-path", path)

    value = FILE_URL.sub(lambda match: _redaction("absolute-path", match.group(0)), value)
    value = LABELED_ABSOLUTE_PATH.sub(
        lambda match: f"{match.group(1)}:{_redaction('absolute-path', match.group(2))}", value,
    )
    value = WINDOWS_ABSOLUTE_PATH.sub(replace, value)
    return POSIX_ABSOLUTE_PATH.sub(replace, value)


def _sanitize_credential_text(value: str) -> str:
    value = re.sub(
        r"(?i)\b(Bearer|Basic)\s+(\S+)",
        lambda match: f"{match.group(1)} {_redaction('credential', match.group(2))}",
        value,
    )
    def sanitize_url(match: re.Match[str]) -> str:
        candidate = match.group(0)
        parsed = urlsplit(candidate)
        fragment_query = parsed.fragment.split("?", 1)[1] if "?" in parsed.fragment else parsed.fragment
        sensitive_query = any(
            SENSITIVE_QUERY.fullmatch(name)
            for component in (parsed.query, fragment_query)
            for name, _ in parse_qsl(component, keep_blank_values=True)
        )
        if parsed.username or parsed.password or sensitive_query:
            return _redaction("credential", candidate)
        return candidate

    return URL.sub(sanitize_url, value)


def sanitize_argv(argv: list[str]) -> list[str]:
    """Return least-data command evidence while retaining flags and value digests."""
    sanitized: list[str] = []
    redact_next: str | None = None
    for argument in argv:
        if not isinstance(argument, str):
            raise ValueError("command evidence argv must contain only strings")
        if redact_next:
            if REDACTION.fullmatch(argument):
                sanitized.append(argument)
            elif redact_next == "environment" and (environment := ENVIRONMENT_ASSIGNMENT.fullmatch(argument)):
                value = environment.group(3)
                if not REDACTION.fullmatch(value):
                    value = _redaction("credential", value)
                sanitized.append(environment.group(1) + "=" + value)
            else:
                sanitized.append(_redaction("credential", argument))
            redact_next = None
            continue
        split = _split_argument(argument)
        if split:
            flag, separator, assigned = split
            name = _argument_name(flag)
            if SENSITIVE_ARGUMENT.fullmatch(name):
                if assigned and not REDACTION.fullmatch(assigned):
                    assigned = _redaction("credential", assigned)
                sanitized.append(flag + separator + assigned)
                continue
            if ENVIRONMENT_ARGUMENT.fullmatch(name):
                environment = ENVIRONMENT_ASSIGNMENT.fullmatch(assigned)
                if environment:
                    value = environment.group(3)
                    if not REDACTION.fullmatch(value):
                        value = _redaction("credential", value)
                    sanitized.append(flag + separator + environment.group(1) + "=" + value)
                    continue
        name = _argument_name(argument)
        if SENSITIVE_ARGUMENT.fullmatch(name):
            sanitized.append(argument)
            redact_next = "credential"
            continue
        if ENVIRONMENT_ARGUMENT.fullmatch(name):
            sanitized.append(argument)
            redact_next = "environment"
            continue
        environment = ENVIRONMENT_ASSIGNMENT.fullmatch(argument)
        if environment and FORBIDDEN_KEY.search(environment.group(1)):
            value = environment.group(3)
            if not REDACTION.fullmatch(value):
                value = _redaction("credential", value)
            sanitized.append(environment.group(1) + "=" + value)
            continue
        sanitized.append(_sanitize_path_text(_sanitize_credential_text(argument)))
    if redact_next:
        raise ValueError("credential-like argv flag is missing its value")
    return sanitized


def sanitize_evidence(value: Any, *, key: str | None = None) -> Any:
    """Recursively normalize evidence without retaining local paths or secrets."""
    if isinstance(value, dict):
        sanitized: dict[str, Any] = {}
        for child_key, child in value.items():
            if child_key == "argv":
                if not isinstance(child, list):
                    raise ValueError("command evidence argv must be a list")
                sanitized[child_key] = sanitize_argv(child)
            else:
                sanitized[child_key] = sanitize_evidence(child, key=child_key)
        return sanitized
    if isinstance(value, list):
        return [sanitize_evidence(child, key=key) for child in value]
    if isinstance(value, str):
        if key and FORBIDDEN_KEY.search(key) and not _safe_reference(value):
            return _redaction("credential", value)
        return _sanitize_path_text(_sanitize_credential_text(value))
    return value


def validate_evidence_redaction(value: Any, *, context: str = "evidence") -> None:
    """Fail closed when recursive normalization would remove private material."""
    normalized = sanitize_evidence(value)
    if normalized != value:
        raise ValueError(f"{context} contains an absolute local path or credential material")


def verify_observer_bundle(
    bundle: dict[str, Any], *, challenge: str, public_key_base64: str,
    expected_key_id: str, now: datetime | None = None, enforce_freshness: bool = True,
) -> dict[str, Any]:
    """Return signed artifacts after strict Ed25519 and freshness validation."""
    required = {"schema_version", "challenge", "signed_at", "key_id", "artifacts", "signature"}
    if set(bundle) != required or bundle.get("schema_version") != 1:
        raise ValueError("protected observer returned a non-canonical signed bundle")
    if bundle.get("challenge") != challenge:
        raise ValueError("protected observer bundle is not correlated to this challenge")
    if bundle.get("key_id") != expected_key_id or not expected_key_id:
        raise ValueError("protected observer bundle key is not explicitly trusted")
    try:
        signed_at = datetime.fromisoformat(str(bundle["signed_at"]).replace("Z", "+00:00"))
    except (TypeError, ValueError) as error:
        raise ValueError("protected observer bundle timestamp is invalid") from error
    current = now or datetime.now(timezone.utc)
    if signed_at.tzinfo is None or (
        enforce_freshness
        and (signed_at > current + timedelta(minutes=2) or current - signed_at > MAX_BUNDLE_AGE)
    ):
        raise ValueError("protected observer bundle is stale or from the future")
    artifacts = bundle.get("artifacts")
    expected_artifacts = {
        "runtime-attestations.json", "notion-oauth-attestations.json",
        "chatgpt-cloudflare-attestation.json", "consent.json",
    }
    if not isinstance(artifacts, dict) or set(artifacts) != expected_artifacts:
        raise ValueError("protected observer bundle has a non-canonical artifact set")
    validate_evidence_redaction(artifacts, context="protected observer bundle")
    try:
        public_key = base64.b64decode(public_key_base64, validate=True)
        signature = base64.b64decode(str(bundle["signature"]), validate=True)
    except (binascii.Error, ValueError) as error:
        raise ValueError("protected observer Ed25519 material is not canonical base64") from error
    if len(public_key) != 32 or len(signature) != 64:
        raise ValueError("protected observer Ed25519 material has an invalid length")
    try:
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey

        Ed25519PublicKey.from_public_bytes(public_key).verify(signature, signed_payload(bundle))
    except ImportError as error:
        raise ValueError("cryptography is required to verify protected observer evidence") from error
    except Exception as error:
        if error.__class__.__module__.startswith("cryptography"):
            raise ValueError("protected observer bundle signature is invalid") from error
        raise
    return artifacts
