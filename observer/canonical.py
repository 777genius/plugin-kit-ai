"""Canonical encoding and least-data evidence validation."""

from __future__ import annotations

import hashlib
import json
import re
from typing import Any
from urllib.parse import unquote, urlsplit

SIGNATURE_DOMAIN = b"UAP-STABLE-LAUNCH-OBSERVER-BUNDLE-V1\0"
ARTIFACT_NAMES = {
    "runtime-attestations.json", "notion-oauth-attestations.json",
    "chatgpt-cloudflare-attestation.json", "consent.json",
}
FORBIDDEN_KEY_PARTS = (
    "apikey", "accesstoken", "refreshtoken", "idtoken", "clientsecret",
    "privatekey", "secretkey", "xamzsignature", "signature", "token",
    "secret", "password", "passwd", "cookie", "authorization",
    "credential", "oauthcode", "oauthstate", "oauthtoken",
)
POSIX_PATH = re.compile(r"(?<![A-Za-z0-9/:])/{1,}[^\s,;\"']+")
WINDOWS_PATH = re.compile(r"(?i)(?<![A-Za-z0-9])(?:\\\\\?\\[A-Z]:\\|[A-Z]:[\\/]|\\\\)[^\s,;\"']+")
UNSAFE_URI = re.compile(r"(?i)(?<![A-Za-z0-9+.-])(?:file|ftp|path|workspace):[\\/]+[^\s,;\"']+")
SHA_REFERENCE = re.compile(r"^sha256:[a-f0-9]{64}$")
BEARER = re.compile(r"(?i)\b(?:Bearer|Basic)\s+\S+")
URL_CREDENTIAL = re.compile(r"(?i)https?://[^\s/@:]+:[^\s/@]+@[^\s/]+")
INLINE_CREDENTIAL = re.compile(
    r"(?i)(?:^|[?&#;,\s-])(?:api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|private[_-]?key|secret[_-]?key|x-amz-signature|authorization|password|token|secret)\s*(?:=|:)\s*[^\s,;&#]+"
)
HTTP_URL = re.compile(r"(?i)https?://[^\s,;\"']+")
REDACTION = re.compile(r"^<redacted:(?:credential|absolute-path):sha256:[a-f0-9]{64}>$")
GITHUB_TOKEN = re.compile(r"\b(?:gh[pousr]_[A-Za-z0-9]{20,255}|github_pat_[A-Za-z0-9_]{20,255})\b")
JWT_TOKEN = re.compile(r"\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b")
AWS_KEY = re.compile(r"\b(?:AKIA|ASIA|AIDA|AROA|AIPA|ANPA|ANVA)[A-Z0-9]{16}\b")
MAX_CLASSIFIED_STRING = 1 << 20
MAX_PERCENT_DECODE_ROUNDS = 4


def canonical_json(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def signed_payload(bundle_without_signature: dict[str, Any]) -> bytes:
    return SIGNATURE_DOMAIN + canonical_json(bundle_without_signature)


def request_digest(value: Any) -> str:
    return hashlib.sha256(canonical_json(value)).hexdigest()


def _safe_http_url(value: str) -> bool:
    if not re.fullmatch(r"https?://[^\s]+", value, re.IGNORECASE):
        return False
    parsed = urlsplit(value)
    return (
        parsed.scheme in {"http", "https"} and bool(parsed.hostname)
        and not parsed.username and not parsed.password
        and not parsed.query and not parsed.fragment
    )


def _credential_key(key: str) -> bool:
    normalized = re.sub(r"[^a-z0-9]", "", key.casefold())
    return any(part in normalized for part in FORBIDDEN_KEY_PARTS)


def _classify_string(value: str) -> None:
    if BEARER.search(value) or URL_CREDENTIAL.search(value) or GITHUB_TOKEN.search(value) or JWT_TOKEN.search(value) or AWS_KEY.search(value):
        raise ValueError("evidence contains authorization material")
    for match in HTTP_URL.finditer(value):
        matched = urlsplit(match.group(0))
        if matched.username or matched.password or matched.query or matched.fragment:
            raise ValueError("evidence contains URL credential material")
    parsed = urlsplit(value)
    if parsed.scheme and (parsed.query or parsed.fragment):
        raise ValueError("evidence contains URL credential material")
    if parsed.scheme in {"http", "https"} and (parsed.username or parsed.password):
        raise ValueError("evidence contains URL credential material")
    if INLINE_CREDENTIAL.search(value):
        raise ValueError("evidence contains credential-like material")
    if UNSAFE_URI.search(value) or POSIX_PATH.search(value) or WINDOWS_PATH.search(value):
        raise ValueError("evidence contains an absolute path")


def validate_redacted(value: Any, *, key: str | None = None) -> None:
    if key == "credential_material_exported" and value is False:
        return
    if key and _credential_key(key):
        if not isinstance(value, str) or not (REDACTION.fullmatch(value) or SHA_REFERENCE.fullmatch(value)):
            raise ValueError("evidence contains credential-like material")
        return
    if isinstance(value, dict):
        for child_key, child in value.items():
            if not isinstance(child_key, str):
                raise ValueError("evidence object keys must be strings")
            validate_redacted(child, key=child_key)
        return
    if isinstance(value, list):
        for child in value:
            validate_redacted(child, key=key)
        return
    if isinstance(value, str):
        if len(value) > MAX_CLASSIFIED_STRING:
            raise ValueError("evidence string exceeds classification bound")
        if REDACTION.fullmatch(value) or SHA_REFERENCE.fullmatch(value):
            return
        decoded = value
        for _ in range(MAX_PERCENT_DECODE_ROUNDS + 1):
            _classify_string(decoded)
            candidate = unquote(decoded)
            if candidate == decoded:
                break
            decoded = candidate
        else:
            raise ValueError("evidence exceeds percent-decoding recursion bound")


def validate_artifacts(value: Any) -> dict[str, Any]:
    if not isinstance(value, dict) or set(value) != ARTIFACT_NAMES:
        raise ValueError("runner returned a non-canonical artifact set")
    for name, artifact in value.items():
        if not isinstance(artifact, dict):
            raise ValueError(f"{name} must contain a JSON object")
    validate_redacted(value)
    return value
