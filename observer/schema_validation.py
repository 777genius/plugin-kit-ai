"""Validation against the reviewed repository evidence schemas."""

from __future__ import annotations

import json
import hashlib
from functools import lru_cache
from pathlib import Path
from typing import Any

from jsonschema import Draft202012Validator, FormatChecker, RefResolver

from .canonical import ARTIFACT_NAMES

SCHEMA_ROOT = Path(__file__).parents[1] / "tests" / "e2e" / "schemas"
BASE_URI = "https://uap.invalid/schemas/"
GITHUB_ATTESTATION_IDENTITY = {
    "subject": "repo:777genius@13103045/universal-agent-plugins@1326737541:environment:stable-launch-e2e",
    "repository": "777genius/universal-agent-plugins",
    "repository_owner": "777genius",
    "repository_id": "1326737541",
    "repository_owner_id": "13103045",
    "ref": "refs/heads/main",
    "environment": "stable-launch-e2e",
    "workflow_ref": "777genius/universal-agent-plugins/.github/workflows/directory-publication.yml@refs/heads/main",
    "job_workflow_ref": "777genius/universal-agent-plugins/.github/workflows/launch-evidence-e2e.yml@refs/heads/main",
    "workflow": "launch-evidence-e2e.yml",
    "job": "protected-observer-inputs",
}


@lru_cache(maxsize=1)
def _validators() -> tuple[dict[str, Draft202012Validator], Draft202012Validator]:
    if not SCHEMA_ROOT.is_dir():
        raise ValueError("reviewed observer schemas are unavailable")
    schemas: dict[str, dict[str, Any]] = {}
    for path in SCHEMA_ROOT.glob("*.schema.json"):
        value = json.loads(path.read_text(encoding="utf-8"))
        value = {**value, "$id": BASE_URI + path.name}
        schemas[path.name] = value
    try:
        runtime = schemas["runtime-attestations.schema.json"]
        consent = schemas["consent.schema.json"]
    except KeyError:
        raise ValueError("reviewed observer schemas are incomplete") from None
    store = {BASE_URI + name: schema for name, schema in schemas.items()}
    role_schemas = {
        "runtime-attestations.json": schemas["hero-runtime-attestations.schema.json"],
        "notion-oauth-attestations.json": schemas["notion-runtime-attestations.schema.json"],
        "chatgpt-cloudflare-attestation.json": schemas["chatgpt-runtime-attestation.schema.json"],
    }
    consent_resolver = RefResolver(BASE_URI + "consent.schema.json", consent, store=store)
    checker = FormatChecker()
    return (
        {name: Draft202012Validator(schema, resolver=RefResolver(BASE_URI + name, schema, store=store), format_checker=checker) for name, schema in role_schemas.items()},
        Draft202012Validator(consent, resolver=consent_resolver, format_checker=checker),
    )


def validate_artifact_schemas(
    artifacts: dict[str, Any], *, challenge: str, scenario_contract_digest: str | None = None,
    expected_bindings: dict[str, str] | None = None,
) -> None:
    if set(artifacts) != ARTIFACT_NAMES:
        raise ValueError("observer artifact set is not canonical")
    role_validators, consent_validator = _validators()
    for name in (
        "runtime-attestations.json", "notion-oauth-attestations.json",
        "chatgpt-cloudflare-attestation.json",
    ):
        errors = sorted(role_validators[name].iter_errors(artifacts[name]), key=lambda item: list(item.absolute_path))
        if errors:
            raise ValueError(f"{name} does not match the reviewed schema")
        for record in artifacts[name].get("attestations", []):
            if record.get("challenge") != challenge:
                raise ValueError(f"{name} is not challenge-bound")
    errors = sorted(consent_validator.iter_errors(artifacts["consent.json"]), key=lambda item: list(item.absolute_path))
    if errors:
        raise ValueError("consent.json does not match the reviewed schema")
    if artifacts["consent.json"].get("challenge") != challenge:
        raise ValueError("consent.json is not challenge-bound")
    if scenario_contract_digest is not None and artifacts["consent.json"].get("scenario_contract_digest") != scenario_contract_digest:
        raise ValueError("consent.json is not scenario-contract-bound")
    _validate_exact_phase_six_set(artifacts, challenge=challenge, expected_bindings=expected_bindings)


def _validate_exact_phase_six_set(
    artifacts: dict[str, Any], *, challenge: str,
    expected_bindings: dict[str, str] | None,
) -> None:
    clients = {"codex", "cursor", "kiro"}
    runtime_plugins = {"agent-code-navigator", "context7", "cloudflare-docs", "chrome-devtools"}
    expected_pairs = {
        "runtime-attestations.json": {(plugin, client) for plugin in runtime_plugins for client in clients},
        "notion-oauth-attestations.json": {("notion", client) for client in clients},
        "chatgpt-cloudflare-attestation.json": {("cloudflare-docs", "chatgpt")},
    }
    consent = artifacts["consent.json"]
    external = artifacts["runtime-attestations.json"].get("external_pr_evidence")
    if not isinstance(external, dict):
        raise ValueError("primary runtime artifact lacks immutable historical external PR evidence")
    if expected_bindings is not None:
        external_binding = external.get("binding")
        if (
            external.get("catalog_repository") != expected_bindings.get("catalog_repository")
            or not isinstance(external_binding, dict)
            or external_binding.get("catalog_repository") != expected_bindings.get("catalog_repository")
            or external_binding.get("catalog_sha") != external.get("base_sha")
            or external_binding.get("catalog_sha") != expected_bindings.get("github", {}).get("sha")
            or external_binding.get("directory_snapshot_digest") != expected_bindings.get("directory_digest")
            or external_binding.get("release_repository") != expected_bindings.get("cli_release_repository")
            or external_binding.get("release_tag") != expected_bindings.get("cli_release_tag")
            or external_binding.get("release_manifest_digest") != expected_bindings.get("release_manifest_digest")
        ):
            raise ValueError("immutable historical external PR evidence targets another catalog, Directory, or stable release")
    exported_consent = (json.dumps(consent, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode()
    consent_digest = "sha256:" + hashlib.sha256(exported_consent).hexdigest()
    binding_fields = (
        "release_manifest_digest", "release_checksums_digest", "directory_digest",
        "scenario_contract_digest",
    )
    common_bindings: dict[str, str] | None = None
    release_identities: dict[str, dict[str, Any]] = {}
    for name, pairs in expected_pairs.items():
        records = artifacts[name]["attestations"]
        actual = [(record.get("plugin"), record.get("client")) for record in records]
        if len(actual) != len(set(actual)) or set(actual) != pairs:
            raise ValueError(f"{name} does not contain the exact unique Phase 6 pair set")
        for record in records:
            plugin, client = record["plugin"], record["client"]
            expected_level = "runtime"
            expected_scenario = "chatgpt_registered_binding" if client == "chatgpt" else "hero_5x3_runtime"
            if record.get("level") != expected_level or record.get("scenario_id") != expected_scenario:
                raise ValueError(f"{name} contains a misplaced attestation")
            if plugin == "notion" and record.get("oauth_artifact_approved") is not True:
                raise ValueError("Notion runtime evidence lacks separately approved OAuth fields")
            if client == "chatgpt":
                if (
                    "native_discovery_evidence" in record
                    or not isinstance(record.get("public_mcp_evidence"), dict)
                    or any(record.get(field) is not True for field in ("registered_app_binding", "ui_activation", "read_only"))
                ):
                    raise ValueError("ChatGPT evidence must use public MCP proof, never native discovery proof")
            if (
                record.get("challenge") != challenge
                or record.get("run_id") != consent.get("run_id")
                or record.get("run_attempt") != consent.get("run_attempt")
                or record.get("identity_id") != consent.get("pseudonymous_identity_id")
                or record.get("pseudonymous_identity_id") != consent.get("pseudonymous_identity_id")
                or record.get("pseudonymous_workspace_id") != consent.get("pseudonymous_workspace_id")
                or record.get("consent_artifact_digest") != consent_digest
            ):
                raise ValueError(f"{name} contains foreign run, consent, or pseudonym bindings")
            bindings = {field: record.get(field) for field in binding_fields}
            if bindings["scenario_contract_digest"] != consent.get("scenario_contract_digest"):
                raise ValueError(f"{name} is not scenario-contract-bound")
            if common_bindings is None:
                common_bindings = bindings  # type: ignore[assignment]
            elif bindings != common_bindings:
                raise ValueError("attestation artifacts mix directory or release identities")
            github = record.get("github_attestation")
            if not isinstance(github, dict) or (
                any(github.get(field) != expected for field, expected in GITHUB_ATTESTATION_IDENTITY.items())
                or str(github.get("run_id")) != consent.get("run_id")
                or str(github.get("run_attempt")) != consent.get("run_attempt")
                or github.get("sha") != consent.get("catalog_sha")
                or github.get("challenge") != challenge
            ):
                raise ValueError(f"{name} contains a foreign GitHub run binding")
            tuple_value = record.get("tuple")
            if not isinstance(tuple_value, dict) or tuple_value.get("product_id") != plugin:
                raise ValueError(f"{name} contains a foreign release tuple")
            static_tuple = {key: value for key, value in tuple_value.items() if key not in {"client_version", "observed_at"}}
            previous = release_identities.setdefault(plugin, static_tuple)
            if static_tuple != previous:
                raise ValueError("attestation artifacts mix release tuple identities")
    if expected_bindings is not None:
        selected = {field: expected_bindings.get(field) for field in binding_fields}
        if common_bindings != selected:
            raise ValueError("attestation artifacts differ from the authorized request bindings")
