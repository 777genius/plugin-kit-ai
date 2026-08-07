#!/usr/bin/env python3
"""Validate privacy and filesystem invariants for sanitized client E2E evidence."""

from __future__ import annotations

import json
import re
from pathlib import Path
from urllib.parse import urlsplit


ROOT = Path(__file__).resolve().parents[1]
RESULTS_DIR = ROOT / "tests" / "e2e" / "results"
REQUIRED_PRIVACY_EXCLUSIONS = {
    "credentials",
    "tokens",
    "cookies",
    "OAuth codes",
    "OAuth state",
    "account identifiers",
    "authorization URLs",
}
DATED_EVIDENCE_NAME = re.compile(r".+-\d{4}-\d{2}-\d{2}\.json\Z")


class ValidationError(RuntimeError):
    """Raised when evidence violates a repository security invariant."""


def _validate_redirect_origin(value: object, source: Path) -> None:
    if value is None:
        return
    if not isinstance(value, str):
        raise ValidationError(f"{source}: observed_redirect_origin must be a string")

    parsed = urlsplit(value)
    if (
        parsed.scheme != "https"
        or not parsed.netloc
        or parsed.username is not None
        or parsed.password is not None
        or parsed.path not in ("", "/")
        or parsed.query
        or parsed.fragment
    ):
        raise ValidationError(
            f"{source}: observed_redirect_origin must contain only an HTTPS scheme and authority"
        )


def _validate_evidence_paths(data: dict[str, object], source: Path, root: Path) -> None:
    root = root.resolve()
    for raw_path in data.get("evidence", []):
        if not isinstance(raw_path, str) or not raw_path:
            raise ValidationError(f"{source}: evidence paths must be non-empty strings")
        path = Path(raw_path)
        if path.is_absolute():
            raise ValidationError(f"{source}: evidence path must be repository-relative: {raw_path}")
        resolved = (root / path).resolve()
        if not resolved.is_relative_to(root):
            raise ValidationError(f"{source}: evidence path escapes the repository: {raw_path}")
        if not resolved.is_file():
            raise ValidationError(f"{source}: evidence path does not exist: {raw_path}")


def _validate_privacy(data: dict[str, object], source: Path) -> None:
    privacy = data.get("privacy")
    if not isinstance(privacy, dict):
        raise ValidationError(f"{source}: privacy must be an object")
    excluded = privacy.get("excluded")
    if not isinstance(excluded, list):
        raise ValidationError(f"{source}: privacy.excluded must be an array")
    missing = REQUIRED_PRIVACY_EXCLUSIONS - set(excluded)
    if missing:
        missing_text = ", ".join(sorted(missing))
        raise ValidationError(f"{source}: privacy.excluded is missing: {missing_text}")


def _validate_cleanup(data: dict[str, object], source: Path) -> None:
    cleanup = data.get("cleanup")
    if not isinstance(cleanup, dict):
        return

    status = cleanup.get("status")
    tracked_fields = (
        "connection_removed",
        "developer_mode_restored",
        "provider_grant_revoked",
    )
    if status == "completed":
        for field in tracked_fields:
            if cleanup.get(field) is not True:
                raise ValidationError(f"{source}: completed cleanup requires {field}=true")
        if not cleanup.get("provider_revocation_method"):
            raise ValidationError(
                f"{source}: completed cleanup requires provider_revocation_method"
            )
        if not cleanup.get("completed_at_utc"):
            raise ValidationError(f"{source}: completed cleanup requires completed_at_utc")
    elif status == "partial":
        if "provider_grant_revoked" not in cleanup:
            raise ValidationError(
                f"{source}: partial cleanup requires provider_grant_revoked"
            )
        if all(cleanup.get(field) is True for field in tracked_fields):
            raise ValidationError(
                f"{source}: partial cleanup requires an incomplete or unknown dimension"
            )
        if not cleanup.get("provider_revocation_observation"):
            raise ValidationError(
                f"{source}: partial cleanup requires provider_revocation_observation"
            )
        if not cleanup.get("last_checked_at_utc"):
            raise ValidationError(
                f"{source}: partial cleanup requires last_checked_at_utc"
            )


def validate_evidence_file(source: Path, root: Path = ROOT) -> None:
    data = json.loads(source.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise ValidationError(f"{source}: evidence root must be an object")
    _validate_redirect_origin(data.get("observed_redirect_origin"), source)
    _validate_evidence_paths(data, source, root)
    _validate_privacy(data, source)
    _validate_cleanup(data, source)


def validate_all(root: Path = ROOT) -> int:
    results_dir = root / "tests" / "e2e" / "results"
    sources = sorted(
        source
        for source in results_dir.glob("*.json")
        if DATED_EVIDENCE_NAME.fullmatch(source.name)
    )
    if not sources:
        raise ValidationError("no dated client evidence files found")
    for source in sources:
        validate_evidence_file(source, root)
    return len(sources)


def main() -> int:
    try:
        count = validate_all()
    except (OSError, json.JSONDecodeError, ValidationError) as error:
        print(f"client evidence validation failed: {error}")
        return 1
    print(f"validated {count} sanitized client evidence files")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
