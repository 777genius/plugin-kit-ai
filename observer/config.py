"""Fail-closed observer configuration."""

from __future__ import annotations

import json
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .secure_files import read_owned_regular


@dataclass(frozen=True)
class IdentityPolicy:
    repository: str
    repository_id: str
    repository_owner_id: str
    ref: str
    ref_type: str
    environment: str
    workflow_ref: str
    job_workflow_ref: str
    workflow: str
    event_names: tuple[str, ...]
    job_name_suffix: str

    @classmethod
    def from_json(cls, value: Any) -> "IdentityPolicy":
        required = set(cls.__annotations__)
        if not isinstance(value, dict) or set(value) != required:
            raise ValueError("identity policy has unexpected fields")
        events = value.get("event_names")
        string_fields = required - {"event_names"}
        if not all(isinstance(value[key], str) and value[key] for key in string_fields):
            raise ValueError("identity policy values must be non-empty strings")
        if not isinstance(events, list) or not events or not all(isinstance(item, str) and item for item in events):
            raise ValueError("identity policy event names must be non-empty strings")
        return cls(**{key: value[key] for key in string_fields}, event_names=tuple(events))


@dataclass(frozen=True)
class Config:
    bind_host: str
    bind_port: int
    state_root: Path
    jwks_url: str
    github_api_url: str
    audience: str
    issuer: str
    key_id: str
    public_key_base64: str
    cli_release_repository: str
    cli_release_tag: str
    signer_socket: Path
    runner_socket: Path
    runner_source_path: Path
    runner_source_digest: str
    runner_user: str
    runner_timeout_seconds: int
    policies: tuple[IdentityPolicy, ...]
    enforce_root_ownership: bool = True

    @classmethod
    def load(cls, path: Path) -> "Config":
        encoded = read_owned_regular(path, 256 << 10, owner_uid=0, exact_mode=0o644, group_gid=0)
        value = json.loads(encoded)
        required = {
            "bind_host", "bind_port", "state_root", "jwks_url", "github_api_url",
            "audience", "issuer", "key_id", "public_key_base64", "signer_socket",
            "runner_socket", "runner_source_path", "runner_source_digest",
            "runner_user",
            "runner_timeout_seconds", "cli_release_repository", "cli_release_tag", "policies",
        }
        if not isinstance(value, dict) or set(value) != required:
            raise ValueError("observer config has unexpected fields")
        policies = value.pop("policies")
        if not isinstance(policies, list) or not policies:
            raise ValueError("at least one identity policy is required")
        config = cls(
            bind_host=value["bind_host"], bind_port=value["bind_port"],
            state_root=Path(value["state_root"]), jwks_url=value["jwks_url"],
            github_api_url=value["github_api_url"], audience=value["audience"],
            issuer=value["issuer"], key_id=value["key_id"], public_key_base64=value["public_key_base64"],
            cli_release_repository=value["cli_release_repository"],
            cli_release_tag=value["cli_release_tag"], signer_socket=Path(value["signer_socket"]),
            runner_socket=Path(value["runner_socket"]), runner_source_path=Path(value["runner_source_path"]),
            runner_source_digest=value["runner_source_digest"], runner_user=value["runner_user"],
            runner_timeout_seconds=value["runner_timeout_seconds"],
            policies=tuple(IdentityPolicy.from_json(item) for item in policies),
            enforce_root_ownership=True,
        )
        config.validate()
        return config

    def validate(self) -> None:
        if self.bind_host != "127.0.0.1" or self.bind_port != 8765:
            raise ValueError("observer must bind only to 127.0.0.1:8765")
        if self.audience != "stable-launch-observer":
            raise ValueError("unexpected OIDC audience")
        if self.issuer != "https://token.actions.githubusercontent.com":
            raise ValueError("unexpected OIDC issuer")
        if self.jwks_url != "https://token.actions.githubusercontent.com/.well-known/jwks":
            raise ValueError("unexpected GitHub OIDC JWKS endpoint")
        if self.github_api_url != "https://api.github.com":
            raise ValueError("unexpected GitHub API endpoint")
        if not self.key_id or not 3 <= len(self.key_id) <= 64:
            raise ValueError("invalid observer key id")
        if not re.fullmatch(r"[A-Za-z0-9+/]{43}=", self.public_key_base64):
            raise ValueError("observer public key must be canonical base64")
        if not re.fullmatch(r"sha256:[a-f0-9]{64}", self.runner_source_digest):
            raise ValueError("runner source digest is invalid")
        if self.runner_user != "root":
            raise ValueError("unexpected runner service identity")
        if self.runner_timeout_seconds != 840:
            raise ValueError("unexpected runner wall-time bound")
        fixed_paths = {
            self.state_root: Path("/var/lib/uap-observer/state"),
            self.signer_socket: Path("/run/uap-observer-signer/sign.sock"),
            self.runner_socket: Path("/run/uap-observer-runner.sock"),
            self.runner_source_path: Path("/opt/uap-observer-current/libexec/uap-observer-runner"),
        }
        if any(actual != expected for actual, expected in fixed_paths.items()):
            raise ValueError("observer protected path differs")
        if self.cli_release_repository != "777genius/plugin-kit-ai" or self.cli_release_tag != "agentplugins-v0.1.18":
            raise ValueError("unexpected CLI release identity")
        for target in (self.state_root, self.signer_socket, self.runner_socket, self.runner_source_path):
            if not target.is_absolute():
                raise ValueError("observer paths must be absolute")
