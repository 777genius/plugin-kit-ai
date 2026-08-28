from __future__ import annotations

import base64
import errno
import hashlib
import http.client
import importlib.util
import json
import os
import pwd
import re
import select
import signal
import shutil
import socket
import stat
import struct
import subprocess
import sys
import tempfile
import threading
import time
import unittest
import zipfile
from unittest import mock
from dataclasses import replace
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives.asymmetric import ed25519, padding, rsa
from cryptography.hazmat.primitives.hashes import SHA256

import observer.fixed_runner as fixed_runner
import observer.fixed_adapters as fixed_adapters
from observer import client_bundle
from observer.auth import AuthenticationError, GitHubCorroborator, JwksCache, OidcVerifier, ReplayStore
from observer.canonical import canonical_json, request_digest, signed_payload, validate_redacted
from observer.config import Config, IdentityPolicy
from observer.fixed_runner import Adapter, ReviewedRunner, serve as serve_runner
from observer.http_server import BoundedThreadingHTTPServer, MAX_REQUEST_BYTES, ObserverHandler
from observer.runner import SocketRunner
from observer.schema_validation import validate_artifact_schemas
from observer.secure_files import read_owned_regular
from observer.service import CHALLENGE_DOMAIN, ObserverService, WorkBusyError
from observer.signer import SocketSigner
from observer.signer import CacheExpiredError
from observer.tests.classification import requires_disposable_observer_host


def b64url(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode()


def sealed_tuple(product: str) -> dict[str, Any]:
    digest = "sha256:" + "a" * 64
    return {
        "product_id": product, "tree_digest": digest, "manifest_digest": digest,
        "distribution_id": f"owner/{product}", "distribution_kind": "upstream",
        "release_sequence": 1, "package_version": "1.0.0",
        "source_repository": f"owner/{product}", "source_revision": "b" * 40,
        "source_path": f"plugins/{product}", "snapshot_sequence": 1,
        "snapshot_digest": digest, "binary_digest": digest,
        "dependency_identity": "locked", "installer_version": "0.1.18",
        "adapter_version": "r14d", "client_version": None,
        "os": "linux", "architecture": "x86_64", "observed_at": "2026-08-26T00:00:00Z",
    }


def native_fixture_relative(plugin: str) -> Path:
    return (
        Path("skills/code-tool-router/SKILL.md")
        if plugin == "agent-code-navigator" else Path(f"active-{plugin}.json")
    )


class Fixture:
    def __init__(self, root: Path):
        self.root = root
        self.now = 1_800_000_000
        self.rsa_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
        self.observer_key = ed25519.Ed25519PrivateKey.generate()
        public_bytes = self.observer_key.public_key().public_bytes_raw()
        numbers = self.rsa_key.public_key().public_numbers()
        self.kid = "fixture-rsa-key"
        self.policy = IdentityPolicy(
            repository="777genius/universal-agent-plugins",
            repository_id="1326737541",
            repository_owner_id="13103045",
            ref="refs/heads/main",
            ref_type="branch",
            environment="stable-launch-e2e",
            workflow_ref="777genius/universal-agent-plugins/.github/workflows/directory-publication.yml@refs/heads/main",
            job_workflow_ref="777genius/universal-agent-plugins/.github/workflows/launch-evidence-e2e.yml@refs/heads/main",
            workflow="Signed Directory publication",
            event_names=("push", "workflow_dispatch"),
            job_name_suffix="protected-observer-inputs",
        )
        runner_source = root / "runner"
        runner_source.write_text("#!/bin/sh\nexit 1\n")
        runner_source.chmod(0o755)
        self.config = Config(
            bind_host="127.0.0.1", bind_port=8765, state_root=root / "state",
            jwks_url="https://token.actions.githubusercontent.com/.well-known/jwks",
            github_api_url="https://api.github.com", audience="stable-launch-observer",
            issuer="https://token.actions.githubusercontent.com", key_id="fixture-ed25519",
            public_key_base64=base64.b64encode(public_bytes).decode(),
            cli_release_repository="777genius/plugin-kit-ai", cli_release_tag="agentplugins-v0.1.18",
            signer_socket=root / "sign.sock", runner_socket=root / "runner.sock",
            runner_source_path=runner_source,
            runner_source_digest="sha256:" + hashlib.sha256(runner_source.read_bytes()).hexdigest(),
            runner_user="root", runner_timeout_seconds=5,
            policies=(self.policy,), enforce_root_ownership=False,
        )
        self.jwks = {"keys": [{
            "kty": "RSA", "use": "sig", "alg": "RS256", "kid": self.kid,
            "n": b64url(numbers.n.to_bytes((numbers.n.bit_length() + 7) // 8, "big")),
            "e": b64url(numbers.e.to_bytes((numbers.e.bit_length() + 7) // 8, "big")),
        }]}

    def request(self) -> dict[str, Any]:
        sha = "a" * 40
        release = "sha256:" + "b" * 64
        directory = "sha256:" + "c" * 64
        scenario = "sha256:" + "1" * 64
        challenge = {
            "nonce": "d" * 64, "github_sha": sha, "run_id": "1001", "run_attempt": "2",
            "release_manifest_digest": release, "directory_digest": directory,
            "scenario_contract_digest": scenario, "root_id": "e" * 64,
        }
        challenge["value"] = hashlib.sha256(CHALLENGE_DOMAIN + canonical_json(challenge)).hexdigest()
        value = {
            "schema_version": 1, "purpose": "stable-launch-e2e",
            "catalog_repository": self.policy.repository,
            "cli_release_repository": "777genius/plugin-kit-ai", "cli_release_tag": "agentplugins-v0.1.18",
            "release_manifest_digest": release, "release_checksums_digest": "sha256:" + "f" * 64,
            "directory_digest": directory, "scenario_contract_digest": scenario,
            "github": {"sha": sha, "run_id": "1001", "run_attempt": "2"},
            "challenge": challenge,
        }
        return value

    def claims(self, *, jti: str = "fixture-jti-0001", **changes: Any) -> dict[str, Any]:
        claims = {
            "iss": self.config.issuer, "aud": self.config.audience,
            "sub": "repo:777genius/universal-agent-plugins:environment:stable-launch-e2e",
            "iat": self.now - 10, "nbf": self.now - 10, "exp": self.now + 300, "jti": jti,
            "repository": self.policy.repository, "repository_owner": "777genius",
            "repository_id": self.policy.repository_id,
            "repository_owner_id": self.policy.repository_owner_id, "ref": self.policy.ref,
            "sha": "a" * 40, "run_id": "1001", "run_attempt": "2",
            "environment": self.policy.environment, "workflow_ref": self.policy.workflow_ref,
            "job_workflow_ref": self.policy.job_workflow_ref,
            "workflow_sha": "a" * 40, "job_workflow_sha": "a" * 40,
            "workflow": self.policy.workflow, "event_name": "workflow_dispatch", "ref_type": "branch",
        }
        claims.update(changes)
        return claims

    def token(self, **changes: Any) -> str:
        header = {"alg": "RS256", "typ": "JWT", "kid": self.kid}
        claims = self.claims(**changes)
        message = b64url(canonical_json(header)) + "." + b64url(canonical_json(claims))
        signature = self.rsa_key.sign(message.encode(), padding.PKCS1v15(), SHA256())
        return message + "." + b64url(signature)

    def fetch(self, url: str) -> Any:
        if url == self.config.jwks_url:
            return self.jwks
        if url.endswith("/jobs?filter=latest&per_page=100"):
            return {"jobs": [{
                "name": "required-stable-launch-evidence / protected-observer-inputs",
                "run_id": 1001, "run_attempt": 2, "head_sha": "a" * 40,
                "status": "in_progress", "workflow_name": self.policy.workflow,
            }]}
        if url.endswith("/actions/runs/1001/attempts/2"):
            return {
                "id": 1001, "run_attempt": 2, "head_sha": "a" * 40,
                "path": ".github/workflows/directory-publication.yml", "event": "workflow_dispatch",
                "repository": {"full_name": self.policy.repository, "id": 1326737541, "owner": {"id": 13103045}},
            }
        raise AssertionError(url)


def artifacts(challenge: str = "a" * 64) -> dict[str, Any]:
    consent = {
            "schema_version": 1, "purpose": "stable-launch-e2e", "consent": True,
            "mode": "enforced", "challenge": challenge, "run_id": "1001", "run_attempt": "2",
            "catalog_sha": "a" * 40, "scenario_contract_digest": "sha256:" + "1" * 64,
            "pseudonymous_identity_id": "fixture-identity", "pseudonymous_workspace_id": "e" * 64,
            "dedicated_identity": True, "disposable_project_status": "disposed",
            "operation_mode": "read-only", "auth_origin": "fresh-dedicated-identity",
            "cleanup_outcome": "cleaned", "no_real_project_proof": {
                "real_project_accessed": False, "absolute_paths_exported": False,
                "credential_material_exported": False, "auth_copied": False,
                "enforcement": "systemd-positive-mount-allowlist-v1",
            },
        }
    exported_consent = (json.dumps(consent, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode()
    consent_digest = "sha256:" + hashlib.sha256(exported_consent).hexdigest()
    digest = "sha256:" + "9" * 64
    observed = "2026-08-23T12:00:00Z"
    github = {
        "subject": "repo:777genius/universal-agent-plugins:environment:stable-launch-e2e",
        "repository": "777genius/universal-agent-plugins", "repository_owner": "777genius",
        "repository_id": "1326737541", "repository_owner_id": "13103045",
        "ref": "refs/heads/main", "environment": "stable-launch-e2e",
        "workflow_ref": "777genius/universal-agent-plugins/.github/workflows/directory-publication.yml@refs/heads/main",
        "job_workflow_ref": "777genius/universal-agent-plugins/.github/workflows/launch-evidence-e2e.yml@refs/heads/main",
        "sha": "a" * 40, "run_id": "1001", "run_attempt": "2",
        "workflow": "launch-evidence-e2e.yml", "job": "protected-observer-inputs",
        "challenge": challenge,
    }
    def record(plugin: str, client: str, level: str, scenario: str) -> dict[str, Any]:
        tuple_value = {
            "product_id": plugin, "tree_digest": digest, "manifest_digest": digest,
            "distribution_id": "owner/package", "distribution_kind": "upstream",
            "release_sequence": 1, "package_version": "1.0.0",
            "source_repository": "owner/repository", "source_revision": "b" * 40,
            "source_path": "plugins/package", "snapshot_sequence": 1,
            "snapshot_digest": digest, "binary_digest": digest,
            "dependency_identity": "locked", "installer_version": "1",
            "adapter_version": "1", "client_version": "fixture-client-1",
            "os": "linux", "architecture": "x86_64", "observed_at": observed,
        }
        value = {
            "plugin": plugin, "client": client, "level": level, "outcome": "inconclusive",
            "reason": "fixture observation", "tuple": tuple_value, "challenge": challenge,
            "run_id": "1001", "run_attempt": "2", "scenario_id": scenario,
            "identity_id": "fixture-identity", "consent_artifact_digest": consent_digest,
            "pseudonymous_identity_id": "fixture-identity", "pseudonymous_workspace_id": "e" * 64,
            "dedicated_identity": True, "disposable_project_status": "disposed",
            "operation_mode": "read-only", "auth_origin": "fresh-dedicated-identity",
            "cleanup_outcome": "cleaned", "no_real_project_proof": dict(consent["no_real_project_proof"]),
            "release_manifest_digest": "sha256:" + "b" * 64,
            "release_checksums_digest": "sha256:" + "f" * 64,
            "directory_digest": "sha256:" + "c" * 64,
            "scenario_contract_digest": "sha256:" + "1" * 64,
            "github_attestation": github,
        }
        if plugin == "notion":
            value["oauth_artifact_approved"] = True
        if client == "chatgpt":
            value.update({"registered_app_binding": True, "ui_activation": True, "read_only": True})
            value["public_mcp_evidence"] = {
                "basis": "protected_external_observer", "observer": "public-mcp-command-v1",
                "endpoint": "https://docs.mcp.cloudflare.com/mcp", "protocol_version": "2025-06-18",
                "initialize": {"method": "initialize", "passed": True},
                "list": {"method": "tools/list", "required_name": "search_cloudflare_documentation", "passed": True},
                "read": {"method": "tools/call", "name": "search_cloudflare_documentation", "read_only": True, "marker_digest": digest, "passed": True},
            }
        return value
    clients = ("codex", "cursor", "kiro")
    runtime_plugins = ("agent-code-navigator", "context7", "cloudflare-docs", "chrome-devtools")
    external = {
        "schema_version": 1, "challenge": challenge,
        "catalog_repository": "777genius/universal-agent-plugins",
        "fork_owner": "fixture-owner", "fork_repository": "fixture-owner/universal-agent-plugins",
        "pr_number": 123, "pr_url": "https://github.com/777genius/universal-agent-plugins/pull/123",
        "head_sha": "2" * 40, "base_sha": "a" * 40, "merge_commit_sha": None,
        "changed_paths": ["plugins/context7/plugin.json"],
        "check_runs": [{"name": "validate", "conclusion": "success", "head_sha": "2" * 40}],
        "final_review": {"state": "closed", "decision": "validated", "reviewer_count": 1, "closed_at": observed, "merged_at": None},
        "observed_at": observed,
        "immutable_artifact": {"digest": digest, "reference": "urn:" + digest},
        "binding": {
            "catalog_repository": "777genius/universal-agent-plugins", "catalog_sha": "a" * 40,
            "directory_snapshot_digest": "sha256:" + "c" * 64, "directory_sequence": 1,
            "directory_publication_id": "fixture-publication", "directory_source_commit": "4" * 40,
            "release_repository": "777genius/plugin-kit-ai", "release_tag": "agentplugins-v0.1.18",
            "release_commit": "5" * 40, "release_manifest_digest": "sha256:" + "b" * 64,
        },
    }
    return {
        "runtime-attestations.json": {"schema_version": 1, "external_pr_evidence": external, "attestations": [record(plugin, client, "runtime", "hero_5x3_runtime") for plugin in runtime_plugins for client in clients]},
        "notion-oauth-attestations.json": {"schema_version": 1, "attestations": [record("notion", client, "runtime", "hero_5x3_runtime") for client in clients]},
        "chatgpt-cloudflare-attestation.json": {"schema_version": 1, "attestations": [record("cloudflare-docs", "chatgpt", "runtime", "chatgpt_registered_binding")]},
        "consent.json": consent,
    }


class FakeRunner:
    def __init__(self, value: Any = None, delay: float = 0):
        self.value, self.delay, self.calls, self.contexts = value, delay, 0, []
        self.lock = threading.Lock()

    def run(self, run_dir: Path, context: dict[str, Any], *, deadline: float | None = None) -> dict[str, Any]:
        del deadline
        with self.lock:
            self.calls += 1
            self.contexts.append(context)
        time.sleep(self.delay)
        if isinstance(self.value, Exception):
            raise self.value
        from observer.canonical import validate_artifacts
        value = self.value if self.value is not None else artifacts(context["request"]["challenge"]["value"])
        return validate_artifacts(value)


class TransactionRunner(FakeRunner):
    def __init__(self, state_root: Path, *, fail_first_commit: bool = False):
        super().__init__()
        self.state_root = state_root
        self.fail_first_commit = fail_first_commit
        self.transactions: list[str] = []

    def transaction(self, challenge: str, action: str, *, deadline: float | None = None) -> None:
        del challenge, deadline
        self.transactions.append(action)
        if action == "commit":
            responses = list((self.state_root / "runs").glob("*/response.json"))
            if len(responses) != 1:
                raise AssertionError("one signed response must be published before record consumption")
            if self.fail_first_commit:
                self.fail_first_commit = False
                raise ValueError("injected commit interruption")


class FakeSigner:
    def __init__(self, key: ed25519.Ed25519PrivateKey | None = None):
        self.key = key or ed25519.Ed25519PrivateKey.generate()

    def sign(self, unsigned: dict[str, Any], *, deadline: float | None = None) -> str:
        del deadline
        return base64.b64encode(self.key.sign(signed_payload(unsigned))).decode()

    def verify_cached(self, encoded: bytes, *, challenge: str, now: float) -> dict[str, Any]:
        bundle = json.loads(encoded)
        if canonical_json(bundle) != encoded or bundle["challenge"] != challenge:
            raise ValueError("invalid fixture cache")
        unsigned = {key: value for key, value in bundle.items() if key != "signature"}
        self.key.public_key().verify(base64.b64decode(bundle["signature"]), signed_payload(unsigned))
        signed_at = datetime.fromisoformat(bundle["signed_at"].replace("Z", "+00:00")).timestamp()
        if now - signed_at > 1800:
            raise CacheExpiredError("stale")
        return bundle


class ObserverTests(unittest.TestCase):
    def test_protected_reads_reject_parent_symlinks_and_hardlinks(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            protected = root / "protected"
            protected.mkdir(mode=0o700)
            target = protected / "record"
            target.write_bytes(b"value")
            target.chmod(0o600)
            alias = root / "alias"
            alias.symlink_to(protected, target_is_directory=True)
            with self.assertRaises(OSError):
                read_owned_regular(alias / "record", 32, owner_uid=os.geteuid())
            linked = protected / "linked"
            os.link(target, linked)
            with self.assertRaisesRegex(ValueError, "regular file"):
                read_owned_regular(target, 32, owner_uid=os.geteuid())

    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.fixture = Fixture(Path(self.temp.name))

    def tearDown(self) -> None:
        self.temp.cleanup()

    def service(self, runner: FakeRunner | None = None, fetch=None) -> tuple[ObserverService, FakeRunner, FakeSigner]:
        fixture, runner, signer = self.fixture, runner or FakeRunner(), FakeSigner(self.fixture.observer_key)
        selected_fetch = fetch or fixture.fetch
        verifier = OidcVerifier(fixture.config, fetch=selected_fetch, now=lambda: fixture.now)
        service = ObserverService(
            fixture.config, verifier=verifier, corroborator=GitHubCorroborator(fixture.config, selected_fetch),
            replay=ReplayStore(fixture.config.state_root / "replay", lambda: fixture.now),
            runner=runner, signer=signer, now=lambda: fixture.now,
        )
        return service, runner, signer

    def test_success_is_signed_and_cached_for_fresh_oidc_jti(self) -> None:
        service, runner, signer = self.service()
        first = service.observe(self.fixture.request(), self.fixture.token(jti="fixture-jti-0001"))
        second = service.observe(self.fixture.request(), self.fixture.token(jti="fixture-jti-0002"))
        self.assertEqual(first, second)
        self.assertEqual(runner.calls, 1)
        bundle = json.loads(first)
        signature = base64.b64decode(bundle.pop("signature"))
        signer.key.public_key().verify(signature, signed_payload(bundle))
        self.assertEqual(bundle["challenge"], self.fixture.request()["challenge"]["value"])
        github = runner.contexts[0]["github_attestation"]
        self.assertEqual(github, artifacts(bundle["challenge"])["runtime-attestations.json"]["attestations"][0]["github_attestation"])

    def test_oidc_replay_is_rejected(self) -> None:
        service, _, _ = self.service()
        token = self.fixture.token(jti="fixture-jti-replay")
        service.observe(self.fixture.request(), token)
        with self.assertRaisesRegex(AuthenticationError, "already used"):
            service.observe(self.fixture.request(), token)

    def test_expired_cache_is_atomically_replaced(self) -> None:
        service, runner, _ = self.service()
        first = service.observe(self.fixture.request(), self.fixture.token(jti="fixture-cache-old"))
        self.fixture.now += 1801
        second = service.observe(self.fixture.request(), self.fixture.token(jti="fixture-cache-new"))
        self.assertNotEqual(first, second)
        self.assertEqual(runner.calls, 2)
        runs = self.fixture.config.state_root / "runs"
        self.assertEqual(len([path for path in runs.iterdir() if not path.name.startswith(".")]), 1)

    def test_interrupted_record_commit_retries_from_durable_signed_response_without_double_execution(self) -> None:
        runner = TransactionRunner(self.fixture.config.state_root, fail_first_commit=True)
        service, _, _ = self.service(runner)
        with self.assertRaisesRegex(ValueError, "commit interruption"):
            service.observe(self.fixture.request(), self.fixture.token(jti="fixture-commit-interrupted"))
        response = service.observe(self.fixture.request(), self.fixture.token(jti="fixture-commit-recovery"))
        self.assertEqual(json.loads(response)["challenge"], self.fixture.request()["challenge"]["value"])
        self.assertEqual(runner.calls, 1)
        self.assertEqual(runner.transactions, ["rollback", "commit", "commit"])

    def test_expired_oidc_is_rejected(self) -> None:
        service, _, _ = self.service()
        with self.assertRaisesRegex(AuthenticationError, "expired"):
            service.observe(self.fixture.request(), self.fixture.token(iat=self.fixture.now - 700, nbf=self.fixture.now - 700, exp=self.fixture.now - 100))

    def test_wrong_exact_claim_is_rejected(self) -> None:
        service, _, _ = self.service()
        with self.assertRaisesRegex(AuthenticationError, "not allowlisted"):
            service.observe(self.fixture.request(), self.fixture.token(ref="refs/heads/feature"))

    def test_wrong_run_claim_is_rejected_before_execution(self) -> None:
        def stale_attempt_fetch(url: str) -> Any:
            return self.fixture.fetch(url.replace("/attempts/3", "/attempts/2"))
        service, runner, _ = self.service(fetch=stale_attempt_fetch)
        with self.assertRaisesRegex(AuthenticationError, "run does not corroborate"):
            service.observe(self.fixture.request(), self.fixture.token(run_attempt="3"))
        self.assertEqual(runner.calls, 0)

    def test_public_job_mismatch_fails_closed(self) -> None:
        fixture = self.fixture
        def wrong_fetch(url: str) -> Any:
            value = fixture.fetch(url)
            if url.endswith("/jobs?filter=latest&per_page=100"):
                value["jobs"][0]["head_sha"] = "0" * 40
            return value
        service, runner, _ = self.service(fetch=wrong_fetch)
        with self.assertRaisesRegex(AuthenticationError, "job does not corroborate"):
            service.observe(fixture.request(), fixture.token())
        self.assertEqual(runner.calls, 0)

    def test_completed_job_cannot_authorize_a_new_observation(self) -> None:
        fixture = self.fixture
        def completed_fetch(url: str) -> Any:
            value = fixture.fetch(url)
            if url.endswith("/jobs?filter=latest&per_page=100"):
                value["jobs"][0].update({"status": "completed", "conclusion": "success"})
            return value
        service, runner, _ = self.service(fetch=completed_fetch)
        with self.assertRaisesRegex(AuthenticationError, "not active"):
            service.observe(fixture.request(), fixture.token())
        self.assertEqual(runner.calls, 0)

    def test_concurrent_identical_requests_are_single_flight(self) -> None:
        service, runner, _ = self.service(FakeRunner(delay=0.05))
        responses: list[bytes] = []
        failures: list[Exception] = []
        def call(index: int) -> None:
            try:
                responses.append(service.observe(self.fixture.request(), self.fixture.token(jti=f"fixture-concurrent-{index}")))
            except Exception as error:
                failures.append(error)
        threads = [threading.Thread(target=call, args=(index,)) for index in range(2)]
        for thread in threads: thread.start()
        for thread in threads: thread.join()
        self.assertFalse(failures)
        self.assertEqual(len(responses), 2)
        self.assertEqual(responses[0], responses[1])
        self.assertEqual(runner.calls, 1)

    def test_redaction_violation_fails_and_does_not_publish_run(self) -> None:
        unsafe = artifacts()
        unsafe["runtime-attestations.json"]["debug_path"] = "/home/runner/private.json"
        service, _, _ = self.service(FakeRunner(unsafe))
        with self.assertRaisesRegex(ValueError, "absolute path"):
            service.observe(self.fixture.request(), self.fixture.token())
        runs = self.fixture.config.state_root / "runs"
        self.assertFalse(runs.exists() and any(path.name[0] != "." for path in runs.iterdir()))

    def test_nested_credential_value_is_rejected(self) -> None:
        unsafe = artifacts()
        unsafe["runtime-attestations.json"]["access_token"] = ["bare-secret"]
        service, _, _ = self.service(FakeRunner(unsafe))
        with self.assertRaisesRegex(ValueError, "credential-like"):
            service.observe(self.fixture.request(), self.fixture.token())

    def test_path_uri_and_quoted_path_variants_are_rejected(self) -> None:
        unsafe = (
            "path:/home/alice/private.json", "file:///home/alice/private.json",
            "workspace:/Users/alice/private-project",
            "'/home/alice/private.json'", r"path:C:\Users\alice\private.json",
            r"\\?\C:\Users\alice\private.json",
        )
        for value in unsafe:
            with self.subTest(value=value), self.assertRaisesRegex(ValueError, "absolute path"):
                validate_redacted({"message": value})
        with self.assertRaisesRegex(ValueError, "URL credential"):
            validate_redacted({"endpoint": "https://example.test/callback?access_token=secret"})
        with self.assertRaisesRegex(ValueError, "URL credential"):
            validate_redacted({"endpoint": "https://example.test/callback#access_token=secret"})
        validate_redacted({"endpoint": "https://api.example.test/v1"})

    def test_common_tokens_any_url_payload_and_encoded_paths_are_rejected(self) -> None:
        unsafe = (
            "fixture-only:gh" + "p_" + ("A" * 24),
            "fixture-only:" + "eyJ" + ("A" * 12) + "." + "eyJ" + ("B" * 12) + "." + ("C" * 16),
            "fixture-only:AK" + "IA" + ("A" * 16),
            "https://fixture-user:fixture-password@example.invalid/path",
            "https://example.invalid/path?token=fixture-only",
            "fixture-only%20token%3Dvalue",
            "%252Fhome%252Ffixture-only%252Fartifact.json",
        )
        for value in unsafe:
            with self.subTest(value=value), self.assertRaises(ValueError):
                validate_redacted({"nested": [{"value": value}]})
    def test_adversarial_credential_names_values_and_argv_are_rejected(self) -> None:
        samples = (
            {"apiKey": "fixture-only-placeholder"},
            {"client_secret": "fixture-only-placeholder"},
            {"PASSWORD": "fixture-only-placeholder"},
            {"value": "Bearer fixture-only-placeholder"},
            {"value": "token=fixture-only-placeholder"},
            {"argv": ["fixture-tool", "--password=fixture-only-placeholder"]},
            {"argv": ["fixture-tool", "Authorization: Basic fixture-only-placeholder"]},
        )
        for sample in samples:
            with self.subTest(sample=sample), self.assertRaises(ValueError):
                validate_redacted(sample)

    def test_private_key_double_slash_and_nested_encoded_credentials_are_rejected(self) -> None:
        unsafe = (
            {"private_key": "fixture-secret"},
            {"Private-Key-Token": "fixture-secret"},
            {"nested": [{"MiXeD_SeCrEt_ToKeN": "fixture-secret"}]},
            {"path": "//home/alice/secret"},
            {"endpoint": "https://example.test/path%3Faccess_token%3Dfixture-secret"},
            {"endpoint": "https://example.test/path%25253Fprivate_key%25253Dfixture-secret"},
        )
        for sample in unsafe:
            with self.subTest(sample=sample), self.assertRaises(ValueError):
                validate_redacted(sample)
        with self.assertRaisesRegex(ValueError, "recursion bound"):
            validate_redacted({"value": "%252525252Fhome%252525252Falice"})

    def test_distinct_concurrent_request_fails_fast(self) -> None:
        service, runner, _ = self.service(FakeRunner(delay=0.15))
        first_request = self.fixture.request()
        second_request = self.fixture.request()
        second_request["challenge"]["nonce"] = "9" * 64
        challenge = second_request["challenge"]
        challenge["value"] = hashlib.sha256(CHALLENGE_DOMAIN + canonical_json({
            key: challenge[key] for key in (
                "github_sha", "run_id", "run_attempt", "release_manifest_digest",
                "directory_digest", "scenario_contract_digest", "root_id", "nonce",
            )
        })).hexdigest()
        failures: list[Exception] = []
        thread = threading.Thread(target=lambda: service.observe(first_request, self.fixture.token(jti="fixture-distinct-leader")))
        thread.start()
        deadline = time.monotonic() + 5
        while runner.calls == 0 and time.monotonic() < deadline:
            time.sleep(0.005)
        self.assertEqual(runner.calls, 1, "leader did not enter the protected runner")
        try:
            with self.assertRaises(WorkBusyError):
                service.observe(second_request, self.fixture.token(jti="fixture-distinct-follower"))
        finally:
            thread.join()
        self.assertEqual(runner.calls, 1)

    def test_cache_symlink_and_fifo_are_rejected_without_following(self) -> None:
        service, _, _ = self.service()
        request = self.fixture.request()
        service.observe(request, self.fixture.token(jti="fixture-cache-safe"))
        response = service._target(request, request_digest(request)) / "response.json"
        saved = response.read_bytes()
        response.unlink()
        outside = Path(self.temp.name) / "outside.json"
        outside.write_bytes(saved)
        response.symlink_to(outside)
        with self.assertRaises(OSError):
            service.observe(request, self.fixture.token(jti="fixture-cache-symlink"))
        response.unlink()
        os.mkfifo(response, 0o600)
        started = time.monotonic()
        with self.assertRaises(ValueError):
            service.observe(request, self.fixture.token(jti="fixture-cache-fifo"))
        self.assertLess(time.monotonic() - started, 1)

    def test_cache_signature_tampering_is_rejected_without_rerunning(self) -> None:
        service, runner, _ = self.service()
        request = self.fixture.request()
        service.observe(request, self.fixture.token(jti="fixture-cache-signed"))
        response = service._target(request, request_digest(request)) / "response.json"
        bundle = json.loads(response.read_bytes())
        bundle["signature"] = base64.b64encode(b"x" * 64).decode()
        response.write_bytes(canonical_json(bundle))
        with self.assertRaises(InvalidSignature):
            service.observe(request, self.fixture.token(jti="fixture-cache-tampered"))
        self.assertEqual(runner.calls, 1)

    def test_cache_retention_is_bounded_and_rejects_untrusted_shapes(self) -> None:
        service, _, _ = self.service()
        runs = self.fixture.config.state_root / "runs"
        runs.mkdir(parents=True)
        for index in range(64):
            entry = runs / f"run-{index:03d}"
            entry.mkdir(mode=0o700)
            (entry / "response.json").write_bytes(b"{}")
            os.utime(entry / "response.json", (index + 1, index + 1))
        service._retain_cache(runs)
        self.assertEqual(len(list(runs.iterdir())), 63)
        (runs / "bad").symlink_to(next(runs.iterdir()), target_is_directory=True)
        with self.assertRaisesRegex(ValueError, "untrusted"):
            service._retain_cache(runs)

    def test_schema_invalid_artifact_is_never_signed(self) -> None:
        invalid = artifacts(self.fixture.request()["challenge"]["value"])
        del invalid["consent.json"]["run_id"]
        service, _, _ = self.service(FakeRunner(invalid))
        with self.assertRaisesRegex(ValueError, "reviewed schema"):
            service.observe(self.fixture.request(), self.fixture.token())

    def test_exact_phase_six_set_rejects_omissions_duplicates_misplacement_and_foreign_bindings(self) -> None:
        challenge = self.fixture.request()["challenge"]["value"]
        exact = artifacts(challenge)
        validate_artifact_schemas(
            exact, challenge=challenge,
            scenario_contract_digest=self.fixture.request()["scenario_contract_digest"],
            expected_bindings=self.fixture.request(),
        )
        mutations = []
        missing = json.loads(json.dumps(exact)); missing["runtime-attestations.json"]["attestations"].pop(); mutations.append(missing)
        duplicate = json.loads(json.dumps(exact)); duplicate["runtime-attestations.json"]["attestations"][-1] = duplicate["runtime-attestations.json"]["attestations"][0]; mutations.append(duplicate)
        misplaced = json.loads(json.dumps(exact)); misplaced["notion-oauth-attestations.json"]["attestations"][0]["plugin"] = "context7"; mutations.append(misplaced)
        mixed_run = json.loads(json.dumps(exact)); mixed_run["runtime-attestations.json"]["attestations"][0]["run_id"] = "9000"; mutations.append(mixed_run)
        foreign_pseudonym = json.loads(json.dumps(exact)); foreign_pseudonym["runtime-attestations.json"]["attestations"][0]["pseudonymous_identity_id"] = "foreign-identity"; mutations.append(foreign_pseudonym)
        foreign_directory = json.loads(json.dumps(exact)); foreign_directory["runtime-attestations.json"]["attestations"][0]["directory_digest"] = "sha256:" + "0" * 64; mutations.append(foreign_directory)
        missing_subject = json.loads(json.dumps(exact)); del missing_subject["runtime-attestations.json"]["attestations"][0]["github_attestation"]["subject"]; mutations.append(missing_subject)
        wrong_owner_id = json.loads(json.dumps(exact)); wrong_owner_id["runtime-attestations.json"]["attestations"][0]["github_attestation"]["repository_owner_id"] = "0"; mutations.append(wrong_owner_id)
        missing_enforcement = json.loads(json.dumps(exact)); del missing_enforcement["consent.json"]["no_real_project_proof"]["enforcement"]; mutations.append(missing_enforcement)
        for mutation in mutations:
            with self.subTest(index=mutations.index(mutation)), self.assertRaises(ValueError):
                validate_artifact_schemas(mutation, challenge=challenge, expected_bindings=self.fixture.request())

    def test_github_trust_endpoints_are_pinned(self) -> None:
        with self.assertRaisesRegex(ValueError, "GitHub API endpoint"):
            replace(self.fixture.config, github_api_url="https://mirror.invalid").validate()

    def test_runner_wall_time_is_fixed(self) -> None:
        with self.assertRaisesRegex(ValueError, "wall-time"):
            replace(self.fixture.config, runner_timeout_seconds=841).validate()

    def test_runner_failure_is_sanitized_and_atomic(self) -> None:
        service, _, _ = self.service(FakeRunner(RuntimeError("secret runner output")))
        with self.assertRaisesRegex(RuntimeError, "secret runner output"):
            service.observe(self.fixture.request(), self.fixture.token())
        runs = self.fixture.config.state_root / "runs"
        self.assertEqual(list(runs.iterdir()), [])


class HttpBoundaryTests(unittest.TestCase):
    def test_invalid_bearer_is_rejected_before_request_body_is_read(self) -> None:
        class Rejecting:
            def authenticate(self, token: str, *, on_authenticated=None) -> object:
                raise AuthenticationError("invalid")
            def observe_authenticated(self, request: Any, auth: object, *, deadline: float) -> bytes:
                raise AssertionError("invalid caller body must never be parsed")
        server = BoundedThreadingHTTPServer(("127.0.0.1", 0), ObserverHandler)
        server.service = Rejecting()  # type: ignore[attr-defined]
        thread = threading.Thread(target=server.serve_forever)
        thread.start()
        try:
            connection = socket.create_connection(("127.0.0.1", server.server_port), timeout=2)
            connection.sendall(
                b"POST /v1/stable-launch/observe HTTP/1.1\r\nHost: localhost\r\n"
                b"Authorization: Bearer bogus\r\nContent-Type: application/json\r\nContent-Length: 100000\r\n\r\n"
            )
            self.assertIn(b" 401 ", connection.recv(4096))
            connection.close()
        finally:
            server.shutdown(); server.server_close(); thread.join()

    def test_body_over_128k_is_rejected_without_calling_service(self) -> None:
        class NeverCalled:
            def authenticate(self, token: str, *, on_authenticated=None) -> object:
                return object()
            def observe_authenticated(self, request: Any, auth: object, *, deadline: float) -> bytes:
                raise AssertionError("service must not be called")
        server = BoundedThreadingHTTPServer(("127.0.0.1", 0), ObserverHandler)
        server.service = NeverCalled()  # type: ignore[attr-defined]
        thread = threading.Thread(target=server.serve_forever)
        thread.start()
        try:
            connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=2)
            connection.putrequest("POST", "/v1/stable-launch/observe")
            connection.putheader("Content-Type", "application/json")
            connection.putheader("Content-Length", str(MAX_REQUEST_BYTES + 1))
            connection.putheader("Authorization", "Bearer ignored")
            connection.endheaders()
            response = connection.getresponse()
            self.assertEqual(response.status, 400)
            self.assertEqual(json.loads(response.read()), {"error": "request rejected"})
        finally:
            server.shutdown()
            server.server_close()
            thread.join()

    def test_tokenless_requests_do_not_charge_authenticated_global_capacity(self) -> None:
        class NeverCalled:
            def authenticate(self, token: str, *, on_authenticated=None) -> object:
                if token != "valid":
                    raise AuthenticationError("invalid")
                on_authenticated()
                return object()
            def observe_authenticated(self, request: Any, auth: object, *, deadline: float) -> bytes:
                return b"{}"
        server = BoundedThreadingHTTPServer(("127.0.0.1", 0), ObserverHandler)
        server.service = NeverCalled()  # type: ignore[attr-defined]
        thread = threading.Thread(target=server.serve_forever)
        thread.start()
        try:
            statuses = []
            for _ in range(40):
                connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=10)
                connection.request("POST", "/v1/stable-launch/observe", body=b"{}", headers={"Content-Type": "application/json"})
                response = connection.getresponse()
                statuses.append(response.status)
                response.read()
                connection.close()
            self.assertEqual(statuses, [401] * 40)
            connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=10)
            connection.request("POST", "/v1/stable-launch/observe", body=b"{}", headers={"Content-Type": "application/json", "Authorization": "Bearer valid"})
            response = connection.getresponse()
            self.assertEqual(response.status, 200)
            response.read()
            connection.close()
        finally:
            server.shutdown()
            server.server_close()
            thread.join()

    def test_replayed_valid_token_is_rejected_before_global_charge(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fixture = Fixture(Path(temporary))
            verifier = OidcVerifier(fixture.config, fetch=fixture.fetch, now=lambda: fixture.now)
            service = ObserverService(
                fixture.config, verifier=verifier,
                corroborator=GitHubCorroborator(fixture.config, fixture.fetch),
                replay=ReplayStore(fixture.config.state_root / "replay", lambda: fixture.now),
                runner=FakeRunner(), signer=FakeSigner(fixture.observer_key), now=lambda: fixture.now,
            )
            charges: list[str] = []
            token = fixture.token(jti="fixture-rate-replay")
            service.observe(fixture.request(), token, on_authenticated=lambda: charges.append("charged"))
            with self.assertRaisesRegex(AuthenticationError, "already used"):
                service.observe(fixture.request(), token, on_authenticated=lambda: charges.append("charged"))
            self.assertEqual(charges, ["charged"])


class ExternalSignerTests(unittest.TestCase):
    def test_signer_strictly_rejects_non_rfc_and_case_confusable_evidence(self) -> None:
        helper_path = Path(__file__).parents[2] / "deploy" / "uap-observer-signer.py"
        spec = importlib.util.spec_from_file_location("uap_observer_signer_strict", helper_path)
        self.assertIsNotNone(spec and spec.loader)
        helper = importlib.util.module_from_spec(spec)  # type: ignore[arg-type]
        spec.loader.exec_module(helper)  # type: ignore[union-attr]
        unsigned = {
            "schema_version": 1, "challenge": "a" * 64,
            "signed_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
            "key_id": "fixture-ed25519", "artifacts": artifacts(),
        }
        valid = signed_payload(unsigned)
        helper.validate_payload(valid, key_id="fixture-ed25519")
        encoded = valid[len(helper.SIGNATURE_DOMAIN):]
        adversarial = (
            encoded[:-1] + b',"Extension":{},"extension":{}}',
            encoded[:-1] + b',"value":NaN}',
            encoded[:-1] + b',"value":Infinity}',
            encoded[:-1] + b',"value":1e400}',
            encoded[:-1] + b',"schema_version":1}',
        )
        for body in adversarial:
            with self.subTest(body=body[-80:]), self.assertRaises(ValueError):
                helper.validate_payload(helper.SIGNATURE_DOMAIN + body, key_id="fixture-ed25519")

    @requires_disposable_observer_host
    def test_socket_helper_signs_only_canonical_bundle(self) -> None:
        helper_path = Path(__file__).parents[2] / "deploy" / "uap-observer-signer.py"
        spec = importlib.util.spec_from_file_location("uap_observer_signer", helper_path)
        self.assertIsNotNone(spec and spec.loader)
        helper = importlib.util.module_from_spec(spec)  # type: ignore[arg-type]
        spec.loader.exec_module(helper)  # type: ignore[union-attr]
        with tempfile.TemporaryDirectory() as temporary:
            socket_path = Path(temporary) / "sign.sock"
            try:
                server = helper.socketserver.UnixStreamServer(str(socket_path), helper.Handler)
            except PermissionError:
                self.skipTest("provider sandbox blocks AF_UNIX socket creation")
            private_key = ed25519.Ed25519PrivateKey.generate()
            server.private_key = private_key
            server.key_id = "fixture-ed25519"
            server.allowed_uid = os.geteuid()
            thread = threading.Thread(target=server.serve_forever)
            thread.start()
            try:
                unsigned = {
                    "schema_version": 1, "challenge": "a" * 64,
                    "signed_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
                    "key_id": "fixture-ed25519", "artifacts": artifacts(),
                }
                public = base64.b64encode(private_key.public_key().public_bytes_raw()).decode()
                signer = SocketSigner(socket_path, public, "fixture-ed25519")
                signature = base64.b64decode(signer.sign(unsigned))
                private_key.public_key().verify(signature, signed_payload(unsigned))
                with self.assertRaisesRegex(ValueError, "invalid response"):
                    signer.sign({**unsigned, "key_id": "untrusted-key"})
                server.allowed_uid = os.geteuid() + 1
                with self.assertRaisesRegex(ValueError, "invalid response"):
                    signer.sign(unsigned)
            finally:
                server.shutdown()
                server.server_close()
                thread.join()

    def test_service_failure_is_redacted(self) -> None:
        class Failing:
            def authenticate(self, token: str, *, on_authenticated=None) -> object:
                return object()
            def observe_authenticated(self, request: Any, auth: object, *, deadline: float) -> bytes:
                raise ValueError("provider secret should never escape")
        server = BoundedThreadingHTTPServer(("127.0.0.1", 0), ObserverHandler)
        server.service = Failing()  # type: ignore[attr-defined]
        thread = threading.Thread(target=server.serve_forever)
        thread.start()
        try:
            body = b"{}"
            connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=2)
            connection.request(
                "POST", "/v1/stable-launch/observe", body=body,
                headers={"Content-Type": "application/json", "Authorization": "Bearer fixture"},
            )
            response = connection.getresponse()
            encoded = response.read()
            self.assertEqual(response.status, 503)
            self.assertNotIn(b"secret", encoded)
        finally:
            server.shutdown()
            server.server_close()
            thread.join()


class JwksHardeningTests(unittest.TestCase):
    def test_random_unknown_kids_share_one_refresh_per_cooldown(self) -> None:
        private = rsa.generate_private_key(public_exponent=65537, key_size=2048)
        numbers = private.public_key().public_numbers()
        def jwk(kid: str) -> dict[str, str]:
            return {
                "kty": "RSA", "use": "sig", "alg": "RS256", "kid": kid,
                "n": b64url(numbers.n.to_bytes((numbers.n.bit_length() + 7) // 8, "big")),
                "e": b64url(numbers.e.to_bytes((numbers.e.bit_length() + 7) // 8, "big")),
            }
        clock = [0.0]
        calls = []
        active = [{"keys": [jwk("known")]}]
        def fetch(url: str) -> Any:
            calls.append(url)
            return active[0]
        cache = JwksCache("https://jwks.test", fetch, lambda: clock[0])
        cache.key("known")
        failures = []
        threads = []
        for index in range(12):
            thread = threading.Thread(target=lambda kid=f"unknown-{index}": _capture_error(lambda: cache.key(kid), failures))
            threads.append(thread)
            thread.start()
        for thread in threads:
            thread.join()
        self.assertEqual(len(calls), 2)
        self.assertEqual(len(failures), 12)
        active[0] = {"keys": [jwk("known"), jwk("rotated")]}
        clock[0] = 61
        self.assertIsNotNone(cache.key("rotated"))
        self.assertEqual(len(calls), 3)

    def test_withdrawn_key_is_rejected_on_the_next_successful_refresh(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fixture = Fixture(Path(temporary))
            clock = [0.0]
            replacement = {"keys": [{**fixture.jwks["keys"][0], "kid": "replacement"}]}
            values = [fixture.jwks, replacement]
            cache = JwksCache("https://jwks.test", lambda _: values.pop(0), lambda: clock[0])
            cache.key(fixture.kid)
            clock[0] = cache.REFRESH_SECONDS + 1
            with self.assertRaises(AuthenticationError):
                cache.key(fixture.kid)

    def test_jwks_outage_fails_closed_after_hard_stale_age(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fixture = Fixture(Path(temporary))
            clock, available = [0.0], [True]
            def fetch(_: str) -> Any:
                if available[0]:
                    return fixture.jwks
                raise OSError("outage")
            cache = JwksCache("https://jwks.test", fetch, lambda: clock[0])
            cache.key(fixture.kid)
            available[0] = False
            clock[0] = cache.MAX_STALE_SECONDS + 1
            with self.assertRaisesRegex(AuthenticationError, "discovery failed"):
                cache.key(fixture.kid)


def _capture_error(call, failures: list[Exception]) -> None:  # type: ignore[no-untyped-def]
    try:
        call()
    except Exception as error:
        failures.append(error)


@requires_disposable_observer_host
class FixedRunnerFixtureTests(unittest.TestCase):
    def test_final_revalidation_fails_closed_and_preserves_every_primary_failure(self) -> None:
        for label, primary in (
            ("success", None),
            ("nonzero", ValueError("reviewed adapter failed")),
            ("timeout", TimeoutError("reviewed adapter exceeded deadline")),
            ("spawn", OSError("spawn failed")),
            ("cancellation", KeyboardInterrupt("cancelled")),
        ):
            calls = []
            def mutated() -> None:
                calls.append("revalidated")
                raise ValueError("protected proof changed")
            with self.subTest(path=label), self.assertRaises(ValueError) as caught:
                fixed_runner.finalize_client_job(None, mutated, primary)
            self.assertEqual(calls, ["revalidated"])
            if primary is None:
                self.assertIn("protected proof changed", str(caught.exception))
            else:
                self.assertIs(caught.exception.__cause__, primary)

    def test_post_cgroup_revalidation_binds_root_proofs_and_native_bytes(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            profile = root / "profile"
            proof = root / "proofs" / "codex"
            profile.mkdir(mode=0o700)
            proof.mkdir(parents=True, mode=0o700)
            (proof / "native").mkdir(mode=0o700)
            entries, receipt_rows = [], []
            native = profile / "context7.json"
            for plugin in sorted(fixed_runner.RUNTIME_HEROES):
                component_kind = "skill" if plugin == "agent-code-navigator" else "mcp"
                body = (
                    b"---\nname: code-tool-router\n---\n"
                    if component_kind == "skill" else canonical_json({"plugin": plugin})
                )
                active = (
                    profile / "skills" / "code-tool-router" / "SKILL.md"
                    if component_kind == "skill" else profile / f"{plugin}.json"
                )
                active.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
                active.write_bytes(body)
                active.chmod(0o440)
                proof_native = proof / "native" / f"{plugin}.blob"
                proof_native.write_bytes(body)
                proof_native.chmod(0o440)
                native_digest = "sha256:" + hashlib.sha256(body).hexdigest()
                evidence = {
                    "manager_add_sha256": "sha256:" + "9" * 64,
                    "manager_info_sha256": "sha256:" + "a" * 64,
                    "post_add_doctor_sha256": "sha256:" + "b" * 64,
                }
                tuple_value = {"product_id": plugin}
                entries.append({
                    "plugin": plugin, "component_kind": component_kind,
                    "tuple": tuple_value,
                    "native_config": {"path": str(proof_native), "sha256": native_digest},
                    "client_config": {"path": str(active), "sha256": native_digest},
                    **evidence,
                })
                receipt_rows.append({"name": plugin, "tuple": tuple_value, **evidence})
            projection = proof / "native-projection.json"
            projection.write_bytes(canonical_json({
                "schema_version": 2, "client_id": "codex", "entries": entries,
            }))
            projection.chmod(0o440)
            receipts = proof / "receipts.json"
            receipts.write_bytes(canonical_json({"schema_version": 1, "receipts": receipt_rows}))
            receipts.chmod(0o440)
            config = root / "adapter.json"
            config.write_bytes(canonical_json({"clients": {"codex": {
                "profile": str(profile), "native_projection": {
                    "path": str(projection),
                    "sha256": "sha256:" + hashlib.sha256(projection.read_bytes()).hexdigest(),
                },
            }}}))
            config.chmod(0o640)
            config_digest = "sha256:" + hashlib.sha256(config.read_bytes()).hexdigest()
            fixed_runner.revalidate_client_proofs(
                config, config_digest, "codex", os.geteuid(), os.getegid(),
            )
            original_projection = projection.read_bytes()
            for malformed in (
                {**json.loads(original_projection), "schema_version": True},
                {**json.loads(original_projection), "unexpected": 1},
            ):
                projection.write_bytes(canonical_json(malformed))
                config_value = json.loads(config.read_bytes())
                config_value["clients"]["codex"]["native_projection"]["sha256"] = "sha256:" + hashlib.sha256(projection.read_bytes()).hexdigest()
                config.write_bytes(canonical_json(config_value))
                with self.subTest(projection_boundary=tuple(malformed)), self.assertRaisesRegex(ValueError, "projection schema"):
                    fixed_runner.revalidate_client_proofs(
                        config, "sha256:" + hashlib.sha256(config.read_bytes()).hexdigest(),
                        "codex", os.geteuid(), os.getegid(),
                    )
            projection.write_bytes(original_projection)
            config_value = json.loads(config.read_bytes())
            config_value["clients"]["codex"]["native_projection"]["sha256"] = "sha256:" + hashlib.sha256(original_projection).hexdigest()
            config.write_bytes(canonical_json(config_value))
            config_digest = "sha256:" + hashlib.sha256(config.read_bytes()).hexdigest()
            native.write_bytes(b"native-v2")
            with self.assertRaisesRegex(ValueError, "native config changed"):
                fixed_runner.revalidate_client_proofs(
                    config, config_digest, "codex", os.geteuid(), os.getegid(),
                )

    @staticmethod
    def _reap_process(process: subprocess.Popen[str]) -> None:
        if process.poll() is None:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
        process.communicate()

    @staticmethod
    def _populate_complete_observer_inventory(systemd: Path) -> None:
        systemd.mkdir()
        for name in (
            "uap-observer.service", "uap-observer-signer.service", "uap-observer-runner.service",
            "uap-observer-runner.socket", "uap-observer-caddy.service",
            "uap-observer-egress-proxy.service", "uap-observer-egress-proxy.socket",
        ):
            (systemd / name).write_text("installed\n")
        for name in ("uap-observer.service.d", "uap-observer-runner.service.d"):
            (systemd / name).mkdir()

    def test_installer_recovery_journal_identity_is_strict_json_and_exact_schema(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        invalid_bodies = (
            b'{"version":1,"version":1,"present":[],"records":[]}',
            b'{"version":1,"Version":1,"present":[],"records":[]}',
            b'{"version":true,"present":[],"records":[]}',
            b'{"version":1,"present":[]}',
            b'{"version":1,"present":{},"records":[]}',
            b'{"version":1,"present":[],"records":{},"unexpected":0}',
        )
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            systemd = root / "systemd"
            self._populate_complete_observer_inventory(systemd)
            for index, body in enumerate(invalid_bodies):
                backup = root / f"journal-{index}"
                subprocess.run(
                    ["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"',
                     "sh", str(helper), str(backup), str(systemd)], check=True,
                )
                identity = backup / "identity.json"
                identity.write_bytes(body)
                identity.chmod(0o600)
                result = subprocess.run(
                    ["/bin/sh", "-c", '. "$1"; validate_observer_systemd_journal "$2"',
                     "sh", str(helper), str(backup)], text=True, capture_output=True,
                )
                with self.subTest(identity_body=body):
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn("installer recovery journal is invalid", result.stderr)

    @staticmethod
    def _copy_observer_inventory(reviewed: Path, systemd: Path) -> None:
        """Copy one already-populated authoritative fixture without recreating it."""
        if systemd.exists():
            if any(systemd.iterdir()):
                raise FileExistsError(f"observer inventory destination is not empty: {systemd}")
        else:
            systemd.mkdir()
        for source in reviewed.iterdir():
            destination = systemd / source.name
            if source.is_dir():
                shutil.copytree(source, destination, copy_function=shutil.copy2)
            else:
                shutil.copy2(source, destination, follow_symlinks=False)
        # Rebind metadata and xattrs after copy2 has read each source.  This
        # models the installer's O_NOATIME copy and exact post-copy result.
        sources = [reviewed, *reviewed.rglob("*")]
        for source in sorted(sources, key=lambda path: len(path.parts), reverse=True):
            destination = systemd if source == reviewed else systemd / source.relative_to(reviewed)
            shutil.copystat(source, destination, follow_symlinks=False)

    def _require_private_noatime_view(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            probe = Path(temporary) / "probe"; probe.symlink_to("target")
            result = subprocess.run(
                ["/bin/sh", "-c", '. "$1"; observer_read_symlink_neutral "$2"',
                 "sh", str(helper), str(probe)], text=True, capture_output=True,
            )
        if result.returncode != 0 and "mount" in result.stderr.lower():
            self.skipTest("private read-only no-atime bind remount is denied by the provider sandbox")
        self.assertEqual(result.returncode, 0, result.stderr)

    def _create_hashed_closure(self, closures: Path, helper: Path) -> tuple[Path, str]:
        staged = closures / ".fixture-new"
        libexec = staged / "libexec"; libexec.mkdir(parents=True)
        adapter = libexec / "uap-observer-fixed-adapter"
        adapter.write_text("#!/bin/sh\nexit 1\n"); adapter.chmod(0o755)
        for name in ("runtime", "notion", "chatgpt", "consent"):
            os.link(adapter, libexec / f"uap-observer-adapter-{name}")
        self._populate_complete_observer_inventory(staged / "systemd")
        digest = subprocess.check_output(
            ["/bin/sh", "-c", '. "$1"; observer_closure_identity "$2"',
             "sh", str(helper), str(staged)], text=True,
        ).strip()
        closure = closures / digest; staged.rename(closure)
        return closure, digest

    def _completed_closure_fixture(self, root: Path) -> tuple[Path, Path, str, str]:
        self._require_private_noatime_view()
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        closures = root / "uap-observer-closures"
        staged = closures / ".new"
        staged.mkdir(parents=True)
        (staged / ".complete").write_text("complete-v1\n")
        install_identity = "a" * 64
        (staged / ".install-identity").write_text(install_identity + "\n")
        (staged / "payload").write_text("reviewed closure\n")
        (staged / "payload-link").symlink_to("payload")
        etc = staged / "etc"
        etc.mkdir()
        (etc / "uap-observer-adapter-config.json").write_text("{}\n")
        (etc / "Caddyfile").write_text("{}\n")
        (etc / "uap-observer-egress-allowlist.json").write_text("{}\n")
        libexec = staged / "libexec"
        libexec.mkdir()
        fixed_adapter = libexec / "uap-observer-fixed-adapter"
        fixed_adapter.write_text("#!/bin/sh\nexit 1\n")
        fixed_adapter.chmod(0o755)
        egress_proxy = libexec / "uap-observer-egress-proxy"
        egress_proxy.write_text("#!/bin/sh\nexit 1\n")
        egress_proxy.chmod(0o755)
        for name in ("runtime", "notion", "chatgpt", "consent"):
            os.link(fixed_adapter, libexec / f"uap-observer-adapter-{name}")
        reviewed_systemd = staged / "systemd"
        reviewed_systemd.mkdir()
        for unit in ("uap-observer.service", "uap-observer-signer.service", "uap-observer-runner.service",
                     "uap-observer-runner.socket", "uap-observer-caddy.service",
                     "uap-observer-egress-proxy.service", "uap-observer-egress-proxy.socket"):
            (reviewed_systemd / unit).write_text(f"reviewed {unit}\n")
        for service in ("uap-observer", "uap-observer-runner"):
            dropin = reviewed_systemd / f"{service}.service.d"
            dropin.mkdir()
            (dropin / "egress.conf").write_text(f"reviewed {service} egress\n")
        for path in (staged / ".complete", staged / ".install-identity", staged / "payload"):
            path.chmod(0o644)
        for path in (etc / "uap-observer-adapter-config.json", etc / "Caddyfile"):
            path.chmod(0o640)
        (etc / "uap-observer-egress-allowlist.json").chmod(0o644)
        identity = subprocess.check_output(
            ["/bin/sh", "-c", '. "$1"; observer_closure_identity "$2"', "sh", str(helper), str(staged)],
            text=True,
        ).strip()
        closure = closures / identity
        staged.rename(closure)
        systemd = root / "systemd"
        self._copy_observer_inventory(closure / "systemd", systemd)
        current = root / "uap-observer-current"
        current.symlink_to(f"uap-observer-closures/{identity}")
        return closure, current, install_identity, f"{os.getuid()}:{os.getgid()}"

    def _validate_completed_closure(
        self, root: Path, identity: str, owner: str, *,
        config_gid: int | None = None, caddy_gid: int | None = None,
    ) -> subprocess.CompletedProcess[str]:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        script = 'set -eu; . "$1"; observer_validate_completed_closure "$2" "$3" "$4" "$5" "$6" "$7" "$8"'
        return subprocess.run(
            ["/bin/sh", "-c", script, "sh", str(helper), str(root / "uap-observer-closures"),
             str(root / "uap-observer-current"), identity, owner, str(root / "systemd"),
             str(os.getgid() if config_gid is None else config_gid),
             str(os.getgid() if caddy_gid is None else caddy_gid)],
            text=True, capture_output=True,
        )

    def test_identical_second_install_validation_is_an_exact_no_op(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            _, _, install_identity, owner = self._completed_closure_fixture(root)
            # Discover the fixture once, then arm every access time below its
            # modification time. Re-running rglob inside snapshot() can itself
            # update a directory atime and make the no-op assertion depend on
            # relatime timing. A fixed path set makes any later ordinary read
            # by validation observable and deterministic.
            paths = sorted(root.rglob("*"))
            for path in paths:
                info = path.lstat()
                os.utime(path, ns=(max(0, info.st_mtime_ns - 1), info.st_mtime_ns), follow_symlinks=False)

            def snapshot() -> list[tuple]:
                result = []
                for path in paths:
                    info = path.lstat()
                    payload = b""
                    attrs = tuple(sorted(
                        (os.fsencode(name), os.getxattr(path, name, follow_symlinks=False))
                        for name in os.listxattr(path, follow_symlinks=False)
                    ))
                    if stat.S_ISREG(info.st_mode):
                        descriptor = os.open(path, os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW | getattr(os, "O_NOATIME", 0))
                        try:
                            while block := os.read(descriptor, 1 << 20): payload += block
                            self.assertEqual(os.fstat(descriptor), info)
                        finally: os.close(descriptor)
                    result.append((
                        str(path.relative_to(root)), info.st_dev, info.st_ino, stat.S_IFMT(info.st_mode),
                        stat.S_IMODE(info.st_mode), info.st_uid, info.st_gid, info.st_nlink,
                        info.st_atime_ns, info.st_mtime_ns, info.st_ctime_ns, payload,
                        attrs,
                    ))
                return result
            before = snapshot()
            for _ in range(2):
                result = self._validate_completed_closure(root, install_identity, owner)
                self.assertEqual(result.returncode, 0, result.stderr)
            after = snapshot()
            self.assertEqual(after, before)

    def test_private_noatime_symlink_read_is_metadata_neutral_and_exchange_bound(self) -> None:
        if os.geteuid() != 0:
            self.skipTest("private mount namespace probe requires root")
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); link = root / "pointer"; link.symlink_to("first-relative-target")
            fields = lambda value: (
                value.st_dev, value.st_ino, stat.S_IFMT(value.st_mode), stat.S_IMODE(value.st_mode),
                value.st_uid, value.st_gid, value.st_nlink, value.st_atime_ns, value.st_mtime_ns, value.st_ctime_ns,
            )
            before = fields(link.lstat())
            result = subprocess.run(
                ["/bin/sh", "-c", '. "$1"; observer_read_symlink_neutral "$2"',
                 "sh", str(helper), str(link)], text=True, capture_output=True,
            )
            if result.returncode != 0 and "mount" in result.stderr.lower():
                self.skipTest("provider sandbox does not permit the required private no-atime bind view")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout, "first-relative-target")
            self.assertEqual(fields(link.lstat()), before)

            readfd, writefd = os.pipe()
            try:
                process = subprocess.Popen(
                    ["/bin/sh", "-c", '. "$1"; observer_read_symlink_neutral "$2"',
                     "sh", str(helper), str(link)],
                    env={**os.environ, "UAP_OBSERVER_TEST_SYMLINK_PIN_READY_FD": str(writefd)},
                    pass_fds=(writefd,), text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                    start_new_session=True,
                )
                self.addCleanup(self._reap_process, process)
                os.close(writefd); writefd = -1
                ready, _, _ = select.select([readfd], [], [], 10)
                self.assertTrue(ready, "symlink descriptor was not pinned deterministically")
                self.assertEqual(os.read(readfd, 1), b"1")
                deadline = time.monotonic() + 10
                children = Path(f"/proc/{process.pid}/task/{process.pid}/children")
                while time.monotonic() < deadline:
                    child_pids = children.read_text().split()
                    if any(
                        Path(f"/proc/{pid}/stat").read_text().rsplit(")", 1)[1].split()[0] == "T"
                        for pid in child_pids
                    ):
                        break
                    self.assertIsNone(process.poll(), "symlink probe exited before its exchange barrier")
                    time.sleep(0.01)
                else:
                    self.fail("symlink probe did not enter its exchange barrier")
                link.unlink(); link.symlink_to("exchanged-target")
                os.killpg(process.pid, signal.SIGCONT)
                stdout, stderr = process.communicate(timeout=10)
                self.assertNotEqual(process.returncode, 0, stdout + stderr)
                self.assertEqual(os.readlink(link), "exchanged-target")
            finally:
                os.close(readfd)
                if writefd >= 0: os.close(writefd)

    def test_symlink_neutral_read_fails_closed_when_noatime_is_unavailable(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            link = Path(temporary) / "pointer"; link.symlink_to("relative-target")
            before = link.lstat()
            unsupported = subprocess.run(
                ["/bin/sh", "-c", '. "$1"; observer_read_symlink_neutral "$2"',
                 "sh", str(helper), str(link)],
                env={**os.environ, "UAP_OBSERVER_TEST_NOATIME_UNSUPPORTED": "1"},
                text=True, capture_output=True,
            )
            self.assertNotEqual(unsupported.returncode, 0)
            after = link.lstat()
            self.assertEqual(
                (after.st_dev, after.st_ino, stat.S_IFMT(after.st_mode), stat.S_IMODE(after.st_mode),
                 after.st_uid, after.st_gid, after.st_nlink, after.st_atime_ns, after.st_mtime_ns, after.st_ctime_ns),
                (before.st_dev, before.st_ino, stat.S_IFMT(before.st_mode), stat.S_IMODE(before.st_mode),
                 before.st_uid, before.st_gid, before.st_nlink, before.st_atime_ns, before.st_mtime_ns, before.st_ctime_ns),
            )

    def test_second_install_validation_fails_closed_on_every_identity_boundary(self) -> None:
        mutations = {
            "content drift": lambda closure, current: (closure / "payload").write_text("drift\n"),
            "partial closure": lambda closure, current: (closure / ".complete").unlink(),
            "unsafe mode": lambda closure, current: (closure / "payload").chmod(0o666),
            "unexpected path": lambda closure, current: (closure / "unexpected").write_text("surprise\n"),
            "symlink": lambda closure, current: ((closure / "payload").unlink(), (closure / "payload").symlink_to(".complete")),
            "wrong pointer": lambda closure, current: (current.unlink(), current.symlink_to("uap-observer-closures/" + "0" * 64)),
            "input identity mismatch": lambda closure, current: None,
            "systemd content drift": lambda closure, current: (current.parent / "systemd/uap-observer.service").write_text("drift\n"),
            "systemd unexpected path": lambda closure, current: (current.parent / "systemd/uap-observer.service.d/unexpected.conf").write_text("drift\n"),
            "systemd symlink": lambda closure, current: ((current.parent / "systemd/uap-observer-runner.socket").unlink(), (current.parent / "systemd/uap-observer-runner.socket").symlink_to("uap-observer.service")),
            "adapter copy replacement": lambda closure, current: self._replace_adapter_with_copy(closure),
            "adapter external hardlink": lambda closure, current: os.link(closure / "libexec/uap-observer-fixed-adapter", current.parent / "external-adapter-link"),
        }
        for name, mutate in mutations.items():
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                closure, current, install_identity, owner = self._completed_closure_fixture(root)
                mutate(closure, current)
                if name == "input identity mismatch":
                    install_identity = "b" * 64
                result = self._validate_completed_closure(root, install_identity, owner)
                self.assertNotEqual(result.returncode, 0)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            _, _, install_identity, owner = self._completed_closure_fixture(root)
            wrong_owner = f"{os.getuid() + 1}:{os.getgid()}"
            result = self._validate_completed_closure(root, install_identity, wrong_owner)
            self.assertNotEqual(result.returncode, 0)

    def test_completed_closure_binds_configuration_metadata_to_current_groups(self) -> None:
        for name, kwargs in (
            ("adapter config GID drift", {"config_gid": os.getgid() + 1}),
            ("Caddyfile GID drift", {"caddy_gid": os.getgid() + 1}),
        ):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                _, _, install_identity, owner = self._completed_closure_fixture(root)
                result = self._validate_completed_closure(root, install_identity, owner, **kwargs)
                self.assertNotEqual(result.returncode, 0)
        for relative in ("etc/uap-observer-adapter-config.json", "etc/Caddyfile"):
            with self.subTest(relative=relative), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                closure, _, install_identity, owner = self._completed_closure_fixture(root)
                os.link(closure / relative, root / "external-link")
                result = self._validate_completed_closure(root, install_identity, owner)
                self.assertNotEqual(result.returncode, 0)

    @staticmethod
    def _replace_adapter_with_copy(closure: Path) -> None:
        adapter = closure / "libexec/uap-observer-adapter-runtime"
        payload = adapter.read_bytes()
        mode = stat.S_IMODE(adapter.stat().st_mode)
        adapter.unlink()
        adapter.write_bytes(payload)
        adapter.chmod(mode)

    def test_production_installer_checks_idempotence_before_staging(self) -> None:
        repository = Path(__file__).parents[2]
        installer = (repository / "deploy/uap-observer-install.sh").read_text()
        library = (repository / "deploy/uap-observer-install-lib.sh").read_text()
        validation = installer.index("observer_validate_completed_closure")
        staging = installer.index('install -d -o root -g root -m 0700 "$stage_root"')
        self.assertLess(validation, staging)
        self.assertIn('printf \'%s\\n\' "$install_identity" > "$closure_stage/.install-identity"', installer)
        self.assertIn('observer_validate_installed_accounts_and_state', installer[:staging])
        self.assertIn('observer_validate_protected_inputs', installer[:staging])
        self.assertIn('observer_runtime=${2:-$closure/runtime}', library)
        self.assertIn(
            'observer_validate_installed_accounts_and_state "$source_root" "$source_root"',
            installer,
        )

    def test_proxy_runtime_sources_are_closed_and_installed_at_service_paths(self) -> None:
        repository = Path(__file__).parents[2]
        manifest_paths = {
            line.split("  ", 1)[1]
            for line in (repository / "deploy/uap-observer-runtime.sha256").read_text().splitlines()
        }
        proxy_sources = {
            "deploy/uap-observer-egress-proxy.py",
            "deploy/uap-observer-egress-proxy.service",
            "deploy/uap-observer-egress-proxy.socket",
        }
        self.assertLessEqual(proxy_sources, manifest_paths)
        installer = (repository / "deploy/uap-observer-install.sh").read_text()
        self.assertIn(
            'mv /usr/local/libexec/uap-observer-egress-proxy.new "$closure_stage/libexec/uap-observer-egress-proxy"',
            installer,
        )
        service = (repository / "deploy/uap-observer-egress-proxy.service").read_text()
        self.assertIn(
            "ExecStart=/usr/bin/python3 -B /opt/uap-observer-current/libexec/uap-observer-egress-proxy "
            "--config /opt/uap-observer-current/etc/uap-observer-egress-allowlist.json --socket-fd 3",
            service,
        )
        socket = (repository / "deploy/uap-observer-egress-proxy.socket").read_text()
        self.assertIn("ListenStream=127.0.0.2:8766", socket)

    def test_production_pins_match_exact_bytes_and_fixture_reaches_beyond_both_checks(self) -> None:
        repository = Path(__file__).parents[2]
        manifest = repository / "deploy/uap-observer-runtime.sha256"
        lines = manifest.read_text().splitlines()
        self.assertGreaterEqual(len(lines), 35)
        for line in lines:
            match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9._/-]+)", line)
            self.assertIsNotNone(match, f"manifest entry does not use exactly two checksum separators: {line!r}")
            assert match is not None
            self.assertEqual(hashlib.sha256((repository / match.group(2)).read_bytes()).hexdigest(), match.group(1))
        installer_path = repository / "deploy/uap-observer-install.sh"
        installer_text = installer_path.read_text()
        helper_digest = hashlib.sha256((repository / "deploy/uap-observer-install-lib.sh").read_bytes()).hexdigest()
        manifest_digest = hashlib.sha256(manifest.read_bytes()).hexdigest()
        self.assertIn(f'= {helper_digest}\n', installer_text)
        self.assertIn(f'runtime_manifest_digest={manifest_digest}\n', installer_text)
        if os.geteuid() != 0:
            self.skipTest("disposable production-entry fixture requires root")
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            helper_text = (repository / "deploy/uap-observer-install-lib.sh").read_text()
            fixture_installer = installer_text
            for old, new in (("/opt/", f"{root}/opt/"), ("/usr/local/", f"{root}/usr/local/"),
                             ("/etc/", f"{root}/etc/"), ("/run/", f"{root}/run/"), ("/var/", f"{root}/var/")):
                helper_text = helper_text.replace(old, new); fixture_installer = fixture_installer.replace(old, new)
            helper = root / "uap-observer-install-lib.sh"; helper.write_text(helper_text)
            fixture_helper_digest = hashlib.sha256(helper.read_bytes()).hexdigest()
            fixture_installer = re.sub(
                r'test "\$\(sha256sum "\$install_lib" \| cut -d\' \' -f1\)" = [0-9a-f]{64}',
                f'test "$(sha256sum "$install_lib" | cut -d\' \' -f1)" = {fixture_helper_digest}', fixture_installer,
            )
            archive = root / "caddy.tar.gz"; archive.write_bytes(b"disposable archive\n")
            archive_digest = hashlib.sha256(archive.read_bytes()).hexdigest()
            fixture_installer = fixture_installer.replace("527fbf917c39189a1e3b31d34fa955601680b2d5c8055d2a87b8b9588dec7bb9", archive_digest)
            marker = root / "beyond-pins"
            needle = 'test "$(sha256sum "$stage_root/deploy/uap-observer-runtime.sha256" | cut -d\' \' -f1)" = "$runtime_manifest_digest"\n'
            self.assertIn(needle, fixture_installer)
            fixture_installer = fixture_installer.replace(needle, needle + f"printf reached > '{marker}'\nexit 93\n", 1)
            installer = root / "uap-observer-install.sh"; installer.write_text(fixture_installer); installer.chmod(0o755)
            inputs = []
            for name in ("adapter.json", "observer.json", "Caddyfile", "egress-allowlist.json"):
                path = root / name; path.write_text("{}\n"); inputs.append(path)
            digests = [hashlib.sha256(path.read_bytes()).hexdigest() for path in inputs]
            (root / "opt").mkdir(); (root / "etc/systemd/system").mkdir(parents=True)
            result = subprocess.run(
                [str(installer), str(repository), str(inputs[0]), f"sha256:{digests[0]}",
                 str(inputs[1]), f"sha256:{digests[1]}", str(archive), str(inputs[2]), f"sha256:{digests[2]}",
                 str(inputs[3]), f"sha256:{digests[3]}"],
                env={**os.environ, "PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
                text=True, capture_output=True,
            )
            self.assertEqual(result.returncode, 93, result.stdout + result.stderr)
            self.assertEqual(marker.read_text(), "reached")

    def test_power_loss_recovery_precedes_untrusted_inputs_and_restores_exactly(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        installer = (Path(__file__).parents[2] / "deploy/uap-observer-install.sh").read_text()
        recovery = installer.index('recover_observer_install')
        self.assertLess(recovery, installer.index('untrusted_adapter_config=${2:?$usage}'))
        self.assertLess(recovery, installer.index('observer_install_input_identity'))
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            stage = root / "source.new"
            systemd = root / "systemd"
            closures = root / "closures"
            staged_partial = root / "runtime.new"
            stage.mkdir(mode=0o700); systemd.mkdir(); closures.mkdir(); staged_partial.mkdir()
            old = systemd / "uap-observer.service"
            old.write_text("old\n")
            backup = stage / "systemd-backup"
            subprocess.run(["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"', "sh", str(helper), str(backup), str(systemd)], check=True)
            old.write_text("partially activated\n")
            installed = stage / "systemd"; installed.mkdir()
            shutil.copy2(old, installed / old.name)
            digest = "a" * 64
            (stage / "closure-digest").write_text(digest + "\n")
            (stage / "journal-committed").write_text("committed-v1\n")
            (stage / "closure-digest").chmod(0o600)
            (stage / "journal-committed").chmod(0o600)
            published = closures / digest
            published.mkdir()
            manager = root / "systemctl"
            manager.write_text("#!/bin/sh\ntest \"$1\" = daemon-reload\n")
            manager.chmod(0o755)
            script = '''set -eu
. "$1"
fixture_stage=$2
fixture_partial=$3
cleanup_fixture() { rm -rf "$fixture_partial"; }
recover_observer_install "$2" "$4" "$5" "$6" "$7" cleanup_fixture
'''
            # No source/config/archive arguments exist: recovery uses only its
            # fixed fixture roots and the retained journal.
            result = subprocess.run(["/bin/sh", "-c", script, "sh", str(helper), str(stage), str(staged_partial), str(closures), str(root / "current"), str(systemd), str(manager)], text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(old.read_text(), "old\n")
            self.assertFalse(stage.exists())
            self.assertFalse(staged_partial.exists())
            self.assertFalse(published.exists())

    def test_production_entry_recovers_current_present_before_argument_expansion(self) -> None:
        if os.geteuid() != 0:
            self.skipTest("production installer entry requires root")
        self._require_private_noatime_view()
        repository = Path(__file__).parents[2]
        source_installer = (repository / "deploy/uap-observer-install.sh").read_text()
        source_helper = (repository / "deploy/uap-observer-install-lib.sh").read_text()
        for valid in (True, False):
            with self.subTest(valid=valid), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                replacements = {
                    "/opt/": f"{root}/opt/", "/usr/local/": f"{root}/usr/local/",
                    "/etc/": f"{root}/etc/", "/run/": f"{root}/run/",
                    "/var/": f"{root}/var/",
                }
                helper_text = source_helper
                installer_text = source_installer
                for old, new in replacements.items():
                    helper_text = helper_text.replace(old, new)
                    installer_text = installer_text.replace(old, new)
                helper = root / "uap-observer-install-lib.sh"
                installer = root / "uap-observer-install.sh"
                helper.write_text(helper_text)
                helper_digest = hashlib.sha256(helper.read_bytes()).hexdigest()
                installer_text = re.sub(
                    r'test "\$\(sha256sum "\$install_lib" \| cut -d\' \' -f1\)" = [0-9a-f]{64}',
                    f'test "$(sha256sum "$install_lib" | cut -d\' \' -f1)" = {helper_digest}',
                    installer_text,
                )
                installer.write_text(installer_text)
                installer.chmod(0o755)
                stage = root / "opt/uap-observer-source.new"
                closures = root / "opt/uap-observer-closures"
                systemd = root / "etc/systemd/system"
                stage.mkdir(parents=True, mode=0o700)
                closures.mkdir(parents=True)
                systemd.mkdir(parents=True)
                (root / "usr/local/libexec").mkdir(parents=True)
                (root / "usr/local/bin").mkdir(parents=True)
                (root / "etc/caddy").mkdir(parents=True)
                if valid:
                    backup = stage / "systemd-backup"
                    subprocess.run(
                        ["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"',
                         "sh", str(helper), str(backup), str(systemd)], check=True,
                    )
                    closure, digest = self._create_hashed_closure(closures, helper)
                    (stage / "journal-committed").write_text("committed-v1\n")
                    (stage / "closure-digest").write_text(digest + "\n")
                    (stage / "journal-committed").chmod(0o600)
                    (stage / "closure-digest").chmod(0o600)
                    current = root / "opt/uap-observer-current"
                    current.symlink_to(f"uap-observer-closures/{digest}")
                    self._copy_observer_inventory(closure / "systemd", stage / "systemd")
                    self._copy_observer_inventory(closure / "systemd", systemd)
                else:
                    (stage / "journal-committed").write_text("invalid\n")
                    (stage / "journal-committed").chmod(0o600)
                    current = root / "opt/uap-observer-current"
                    current.symlink_to("uap-observer-closures/" + "b" * 64)
                result = subprocess.run(
                    [str(installer)], text=True, capture_output=True,
                    env={**os.environ, "PATH": "/usr/bin:/bin"},
                )
                self.assertNotEqual(result.returncode, 0)
                if valid:
                    self.assertFalse(stage.exists())
                    self.assertIn("usage: uap-observer-install.sh", result.stderr)
                else:
                    self.assertTrue(stage.exists())
                    self.assertNotIn("usage: uap-observer-install.sh", result.stderr)

    def test_every_installer_new_path_is_in_shared_partial_inventory(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        inventory = subprocess.check_output(["/bin/sh", "-c", '. "$1"; observer_partial_paths', "sh", str(helper)], text=True).splitlines()
        for path in (
            "/usr/local/libexec/uap-observer-attest-chatgpt.new",
            "/usr/local/libexec/uap-observer-attest-consent.new",
            "/usr/local/libexec/uap-observer-provision-profile.new",
        ):
            with self.subTest(path=path):
                self.assertIn(path, inventory)
                with tempfile.TemporaryDirectory() as temporary:
                    partial = Path(temporary) / Path(path).name
                    partial.write_text("partial\n")
                    script = '''set -eu
. "$1"
fixture_partial=$2
fixture_inventory() { printf '%s\\n' "$fixture_partial"; }
observer_validate_no_partial_paths fixture_inventory
'''
                    result = subprocess.run(["/bin/sh", "-c", script, "sh", str(helper), str(partial)], text=True, capture_output=True)
                    self.assertNotEqual(result.returncode, 0)

    def test_partial_cleanup_retries_every_authoritative_parent_fsync(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            first = root / "one"; second = root / "two"
            first.mkdir(); second.mkdir()
            (first / "a.new").write_text("partial\n")
            (second / "b.new").write_text("partial\n")
            log = root / "sync.log"
            script = r'''set -u
. "$1"
fixture_root=$2
attempt=$3
fixture_log=$4
fixture_inventory() { printf '%s\n' "$fixture_root/one/a.new" "$fixture_root/two/b.new"; }
observer_sync_directory() {
  printf '%s %s\n' "$attempt" "$1" >> "$fixture_log"
  test "$attempt" != first || test "$1" != "$fixture_root/one"
}
observer_cleanup_recovery_partials fixture_inventory
'''
            first_result = subprocess.run(
                ["/bin/sh", "-c", script, "sh", str(helper), str(root), "first", str(log)],
                text=True, capture_output=True,
            )
            self.assertNotEqual(first_result.returncode, 0)
            self.assertFalse((first / "a.new").exists())
            self.assertFalse((second / "b.new").exists())
            second_result = subprocess.run(
                ["/bin/sh", "-c", script, "sh", str(helper), str(root), "retry", str(log)],
                text=True, capture_output=True,
            )
            self.assertEqual(second_result.returncode, 0, second_result.stderr)
            events = log.read_text().splitlines()
            self.assertIn(f"first {first}", events)
            self.assertIn(f"retry {first}", events)
            self.assertIn(f"retry {second}", events)

    def test_first_install_requires_empty_trusted_closure_inventory(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            closures = Path(temporary) / "closures"
            command = [
                "/bin/sh", "-c", '. "$1"; observer_validate_first_install_closures_root "$2" "$3"',
                "sh", str(helper), str(closures), f"{os.getuid()}:{os.getgid()}",
            ]
            self.assertEqual(subprocess.run(command).returncode, 0)
            closures.mkdir(mode=0o755)
            self.assertEqual(subprocess.run(command).returncode, 0)
            orphan = closures / ("a" * 64); orphan.mkdir()
            self.assertNotEqual(subprocess.run(command).returncode, 0)
            orphan.rmdir(); closures.chmod(0o775)
            self.assertNotEqual(subprocess.run(command).returncode, 0)
            closures.chmod(0o755); closures.rmdir(); closures.symlink_to(Path(temporary))
            self.assertNotEqual(subprocess.run(command).returncode, 0)

    def test_invalid_recovery_journal_fails_closed_without_cleanup_or_restore(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            stage, closures, systemd = root / "source.new", root / "closures", root / "systemd"
            stage.mkdir(mode=0o700); closures.mkdir(); systemd.mkdir()
            unit = systemd / "uap-observer.service"
            unit.write_text("partially activated\n")
            (stage / "journal-committed").write_text("invalid\n")
            (stage / "journal-committed").chmod(0o600)
            script = '''set -eu
. "$1"
cleanup_fixture() { :; }
recover_observer_install "$2" "$3" "$4" "$5" "$6" cleanup_fixture
'''
            result = subprocess.run(["/bin/sh", "-c", script, "sh", str(helper), str(stage), str(closures), str(root / "current"), str(systemd), "/bin/false"], text=True, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertTrue(stage.exists())
            self.assertEqual(unit.read_text(), "partially activated\n")

    def test_second_install_revalidates_every_supplied_input_and_checksum(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / "source"
            (source / "deploy").mkdir(parents=True)
            runtime = source / "runtime.txt"
            runtime.write_text("runtime\n")
            runtime_digest = hashlib.sha256(runtime.read_bytes()).hexdigest()
            manifest = source / "deploy/uap-observer-runtime.sha256"
            manifest.write_text(f"{runtime_digest}  runtime.txt\n")
            inputs = []
            for name in ("adapter.json", "observer.json", "caddy.tar.gz", "Caddyfile", "egress-allowlist.json"):
                path = root / name
                path.write_text(name + "\n")
                inputs.append(path)
            manifest_digest = hashlib.sha256(manifest.read_bytes()).hexdigest()
            digests = [hashlib.sha256(path.read_bytes()).hexdigest() for path in inputs]
            args = [str(source), manifest_digest, str(inputs[0]), f"sha256:{digests[0]}",
                    str(inputs[1]), f"sha256:{digests[1]}", str(inputs[2]), digests[2],
                    str(inputs[3]), f"sha256:{digests[3]}", str(inputs[4]), f"sha256:{digests[4]}"]
            def run_identity() -> subprocess.CompletedProcess[str]:
                command = ["/bin/sh", "-c", 'set -eu; . "$1"; shift; observer_install_input_identity "$@"',
                           "sh", str(helper), *args]
                return subprocess.run(command, text=True, capture_output=True)
            first = run_identity()
            second = run_identity()
            self.assertEqual(first.returncode, 0, first.stderr)
            self.assertEqual(second.returncode, 0, second.stderr)
            self.assertEqual(first.stdout, second.stdout)
            for name, mutate in (
                ("runtime content", lambda: runtime.write_text("drift\n")),
                ("adapter content", lambda: inputs[0].write_text("drift\n")),
                ("egress content", lambda: inputs[4].write_text("drift\n")),
                ("checksum argument", lambda: args.__setitem__(3, "sha256:" + "0" * 64)),
                ("symlink input", lambda: (inputs[3].unlink(), inputs[3].symlink_to(inputs[1]))),
            ):
                with self.subTest(name=name):
                    runtime.write_text("runtime\n")
                    inputs[0].write_text("adapter.json\n")
                    inputs[4].write_text("egress-allowlist.json\n")
                    if inputs[3].is_symlink():
                        inputs[3].unlink()
                        inputs[3].write_text("Caddyfile\n")
                    args[3] = f"sha256:{digests[0]}"
                    mutate()
                    result = run_identity()
                    self.assertNotEqual(result.returncode, 0)

    def test_real_closure_mode_helper_matches_runtime_startup_validation(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            closure = Path(temporary) / "closure"
            (closure / "libexec").mkdir(parents=True)
            (closure / "etc").mkdir()
            runner = closure / "libexec/uap-observer-runner"
            runner.write_bytes((Path(__file__).parents[1] / "fixed_runner.py").read_bytes())
            runner.chmod(0o700)
            fixed = closure / "libexec/uap-observer-fixed-adapter"
            fixed.write_text("#!/bin/sh\nexit 1\n")
            fixed.chmod(0o700)
            for name in ("runtime", "notion", "chatgpt", "consent"):
                os.link(fixed, closure / f"libexec/uap-observer-adapter-{name}")
            proxy = closure / "libexec/uap-observer-egress-proxy"
            proxy.write_text("#!/bin/sh\nexit 1\n")
            for name in ("uap-observer.json", "uap-observer-adapter-config.json", "uap-observer-adapters.json", "Caddyfile", "uap-observer-egress-allowlist.json"):
                (closure / "etc" / name).write_text("{}")
            helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
            subprocess.run(["/bin/sh", "-c", '. "$1"; apply_observer_closure_modes "$2" "$3" "$3"', "sh", str(helper), str(closure), str(os.getgid())], check=True)
            digest = "sha256:" + hashlib.sha256(runner.read_bytes()).hexdigest()
            SocketRunner(Path(temporary) / "unused.sock", runner, digest, 840)
            fixed_runner.read_owned_regular(closure / "etc/uap-observer.json", 1024, owner_uid=0, exact_mode=0o644, group_gid=0)
            fixed_runner.read_owned_regular(closure / "etc/uap-observer-adapter-config.json", 1024, owner_uid=0, exact_mode=0o640)
            self.assertEqual(stat.S_IMODE(fixed.stat().st_mode), 0o755)
            self.assertFalse(any(path.stat().st_mode & 0o022 for path in closure.rglob("*") if not path.is_symlink()))
            identity_script = '. "$1"; observer_closure_identity "$2"'
            first = subprocess.check_output(["/bin/sh", "-c", identity_script, "sh", str(helper), str(closure)], text=True).strip()
            (closure / "etc/uap-observer.json").chmod(0o600)
            mode_changed = subprocess.check_output(["/bin/sh", "-c", identity_script, "sh", str(helper), str(closure)], text=True).strip()
            (closure / "etc/uap-observer.json").write_text('{"rotation":1}')
            content_changed = subprocess.check_output(["/bin/sh", "-c", identity_script, "sh", str(helper), str(closure)], text=True).strip()
            self.assertRegex(first, r"^[a-f0-9]{64}$")
            self.assertEqual(len({first, mode_changed, content_changed}), 3)

    def test_systemd_bootstrap_rollback_is_exact_at_every_mutation_and_reload_boundary(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as capability_temporary:
            probe = Path(capability_temporary) / "probe"; probe.symlink_to("target")
            capability = subprocess.run(
                ["/bin/sh", "-c", '. "$1"; observer_read_symlink_neutral "$2"',
                 "sh", str(helper), str(probe)], text=True, capture_output=True,
            )
            if capability.returncode != 0 and "mount" in capability.stderr.lower():
                self.skipTest("private read-only no-atime bind remount is denied by the provider sandbox")
            self.assertEqual(capability.returncode, 0, capability.stderr)
        units = ("uap-observer.service", "uap-observer-signer.service", "uap-observer-runner.service", "uap-observer-runner.socket", "uap-observer-caddy.service", "uap-observer-egress-proxy.service", "uap-observer-egress-proxy.socket")
        def xattrs(path: Path) -> tuple[tuple[bytes, bytes], ...]:
            names = os.listxattr(path, follow_symlinks=False)
            return tuple(sorted(
                (os.fsencode(name), os.getxattr(path, name, follow_symlinks=False))
                for name in names
            ))
        def snapshot(root: Path) -> list[tuple[str, int, int, int, int, int, int, tuple[tuple[bytes, bytes], ...], bytes | str]]:
            values = []
            def visit(directory: Path) -> None:
                for entry in sorted(os.scandir(directory), key=lambda item: item.name):
                    path = Path(entry.path)
                    info = path.lstat()
                    metadata = xattrs(path)
                    if stat.S_ISLNK(info.st_mode):
                        payload: bytes | str = os.readlink(path)
                    elif stat.S_ISREG(info.st_mode):
                        payload = path.read_bytes()
                    else:
                        payload = b""
                    values.append((str(path.relative_to(root)), stat.S_IFMT(info.st_mode), stat.S_IMODE(info.st_mode), info.st_uid, info.st_gid, info.st_atime_ns, info.st_mtime_ns, metadata, payload))
                    if stat.S_ISDIR(info.st_mode):
                        visit(path)
                    os.utime(path, ns=(info.st_atime_ns, info.st_mtime_ns), follow_symlinks=False)
            visit(root)
            return values
        failpoints = (
            "before-tree-file-create",
            "after-tree-file-create",
            "before-tree-mkdir",
            "after-tree-mkdir",
            "before-tree-symlink",
            "after-tree-symlink",
            "before-tree-fsync",
            "after-tree-fsync",
            "before-rename",
            "after-rename",
            "before-displacement",
            "after-displacement",
            "after-installation",
            "before-install-directory-fsync",
            "after-install-directory-fsync",
            "before-displaced-tree-deletion",
            "before-tree-unlink",
            "after-tree-unlink",
            "before-tree-rmdir",
            "after-tree-rmdir",
            "after-displaced-tree-deletion",
            "before-delete-directory-fsync",
            "after-delete-directory-fsync",
            "after-installation,before-rollback-installed-removal",
            "after-installation,after-rollback-installed-removal",
            "after-installation,before-rollback-displaced-restore",
            "after-installation,after-rollback-displaced-restore",
            "before-displacement,before-rollback-staging-removal",
            "before-displacement,after-rollback-staging-removal",
            "before-daemon-reload",
            "after-daemon-reload",
        )
        catalog = set(re.findall(r'mutation_boundary\("([^"]+)"\)', helper.read_text()))
        exercised = {
            name for selection in failpoints if "daemon-reload" not in selection
            for name in selection.split(",")
        }
        exercised.update({"before-stale-directory-fsync", "after-stale-directory-fsync"})
        self.assertEqual(catalog, exercised)
        for failure in failpoints:
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                systemd_root, staged, backup = root / "systemd", root / "staged", root / "backup"
                systemd_root.mkdir(); staged.mkdir()
                (systemd_root / "uap-observer.service").write_text("old-unit\n")
                (systemd_root / "legacy-signer.service").write_text("legacy-target\n")
                (systemd_root / "uap-observer-signer.service").symlink_to("legacy-signer.service")
                old_dropin = systemd_root / "uap-observer.service.d"; old_dropin.mkdir()
                (old_dropin / "existing.conf").write_text("old-dropin\n")
                metadata_paths = (
                    systemd_root / "uap-observer.service",
                    old_dropin,
                    old_dropin / "existing.conf",
                    systemd_root / "uap-observer-signer.service",
                )
                try:
                    for index, path in enumerate(metadata_paths[:3]):
                        os.setxattr(path, "user.uap_observer_test", f"metadata-{index}".encode(), follow_symlinks=False)
                except OSError as error:
                    if error.errno in (errno.ENOTSUP, errno.EOPNOTSUPP, errno.EPERM):
                        self.skipTest("fixture filesystem does not expose required no-follow user xattrs")
                    raise
                try:
                    os.setxattr(metadata_paths[3], "user.uap_observer_test", b"metadata-3", follow_symlinks=False)
                except OSError as error:
                    if error.errno not in (errno.ENOTSUP, errno.EOPNOTSUPP, errno.EPERM, errno.EACCES):
                        raise
                setfacl = shutil.which("setfacl")
                if setfacl:
                    for path in (old_dropin, old_dropin / "existing.conf"):
                        result = subprocess.run(
                            [setfacl, "-m", "u:12345:r--", str(path)],
                            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                        )
                        if result.returncode == 0:
                            self.assertIn("system.posix_acl_access", os.listxattr(path, follow_symlinks=False))
                for path in metadata_paths:
                    try:
                        os.setxattr(path, "security.uap_observer_test", b"label", follow_symlinks=False)
                    except OSError as error:
                        if error.errno not in (errno.ENOTSUP, errno.EOPNOTSUPP, errno.EPERM, errno.EACCES, errno.EINVAL):
                            raise
                    else:
                        self.assertEqual(
                            os.getxattr(path, "security.uap_observer_test", follow_symlinks=False),
                            b"label",
                        )
                timestamp = 1_700_000_000_123_456_789
                for offset, path in enumerate(metadata_paths):
                    os.utime(path, ns=(timestamp + offset, timestamp + 100 + offset), follow_symlinks=False)
                for unit in units:
                    if unit == "uap-observer-signer.service":
                        (staged / unit).symlink_to("uap-observer.service")
                    else:
                        (staged / unit).write_text(f"new-{unit}\n")
                for service in ("uap-observer", "uap-observer-runner"):
                    directory = staged / f"{service}.service.d"; directory.mkdir()
                    (directory / "egress.conf").write_text("new-egress\n")
                manager = root / "systemctl"
                manager.write_text("#!/bin/sh\ntest \"$1\" = daemon-reload\n")
                manager.chmod(0o755)
                before = snapshot(systemd_root)
                script = '. "$1"; journal_observer_systemd "$2" "$3"; if activate_observer_systemd "$4" "$3"; then if [ -z "${UAP_OBSERVER_REPLACE_FAILPOINT:-}" ] && reload_observer_systemd "$5"; then exit 99; fi; fi; restore_observer_systemd "$2" "$3" "$4"; "$5" daemon-reload'
                environment = dict(os.environ)
                if "daemon-reload" in failure:
                    environment["UAP_OBSERVER_INSTALL_FAILPOINT"] = failure
                else:
                    environment["UAP_OBSERVER_REPLACE_FAILPOINT"] = failure
                result = subprocess.run(
                    ["/bin/sh", "-c", script, "sh", str(helper), str(backup), str(systemd_root), str(staged), str(manager)],
                    env=environment, text=True, capture_output=True,
                )
                if "daemon-reload" not in failure:
                    self.assertIn("observer replacement failpoint:", result.stderr)
                if result.returncode == 0:
                    self.assertEqual(snapshot(systemd_root), before)
                self.assertTrue(backup.is_dir(), "rollback journal disappeared before explicit resolution")

    def test_low_level_replacement_failpoint_indices_are_exhaustive_and_fail_closed(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"

        def fixture(root: Path) -> tuple[Path, Path, Path]:
            systemd = root / "systemd"; staged = root / "staged"; backup = root / "backup"
            systemd.mkdir(); staged.mkdir()
            live = systemd / "uap-observer.service"; live.write_text("baseline\n")
            reviewed = staged / live.name; reviewed.write_text("reviewed\n")
            subprocess.run(
                ["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"',
                 "sh", str(helper), str(backup), str(systemd)], check=True,
            )
            return systemd, staged, backup

        with tempfile.TemporaryDirectory() as temporary:
            systemd, staged, backup = fixture(Path(temporary))
            traced = subprocess.run(
                ["/bin/sh", "-c", '. "$1"; UAP_OBSERVER_COMPARE_BACKUP=$2 observer_replace_systemd_entries "$3" "$4" "$5"',
                 "sh", str(helper), str(backup), str(systemd),
                 str(staged / "uap-observer.service"), "uap-observer.service"],
                env={**os.environ, "UAP_OBSERVER_REPLACE_TRACE": "1"}, text=True, capture_output=True,
            )
            self.assertEqual(traced.returncode, 0, traced.stderr)
            boundaries = [
                (int(index), name) for index, name in re.findall(
                    r"^uap-observer-replace-boundary (\d+) ([a-z-]+)$", traced.stderr, re.MULTILINE,
                )
            ]
            self.assertEqual([index for index, _ in boundaries], list(range(1, len(boundaries) + 1)))
            self.assertTrue(boundaries)

        for index, name in boundaries:
            with self.subTest(index=index, name=name), tempfile.TemporaryDirectory() as temporary:
                systemd, staged, backup = fixture(Path(temporary))
                failed = subprocess.run(
                    ["/bin/sh", "-c", '. "$1"; UAP_OBSERVER_COMPARE_BACKUP=$2 observer_replace_systemd_entries "$3" "$4" "$5"',
                     "sh", str(helper), str(backup), str(systemd),
                     str(staged / "uap-observer.service"), "uap-observer.service"],
                    env={**os.environ, "UAP_OBSERVER_REPLACE_FAIL_AT": str(index)},
                    text=True, capture_output=True,
                )
                self.assertNotEqual(failed.returncode, 0, f"index {index} ({name}) was not reachable")
                restored = subprocess.run(
                    ["/bin/sh", "-c", '. "$1"; restore_observer_systemd "$2" "$3" "$4"',
                     "sh", str(helper), str(backup), str(systemd), str(staged)],
                    text=True, capture_output=True,
                )
                if restored.returncode == 0:
                    self.assertEqual((systemd / "uap-observer.service").read_text(), "baseline\n")
                else:
                    self.assertTrue(backup.is_dir(), "fail-closed state lost its durable journal")
                self.assertLessEqual(
                    {path.name for path in systemd.iterdir() if path.name.startswith("uap-observer")},
                    {"uap-observer.service"},
                )

        for name in ("before-stale-directory-fsync", "after-stale-directory-fsync"):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temporary:
                systemd, staged, backup = fixture(Path(temporary))
                (systemd / ".uap-observer-new-stale").write_text("discard\n")
                failed = subprocess.run(
                    ["/bin/sh", "-c", '. "$1"; UAP_OBSERVER_COMPARE_BACKUP=$2 observer_replace_systemd_entries "$3" "$4" "$5"',
                     "sh", str(helper), str(backup), str(systemd),
                     str(staged / "uap-observer.service"), "uap-observer.service"],
                    env={**os.environ, "UAP_OBSERVER_REPLACE_FAILPOINT": name},
                    text=True, capture_output=True,
                )
                self.assertNotEqual(failed.returncode, 0)
                self.assertIn(f"observer replacement failpoint: {name}", failed.stderr)
                self.assertTrue(backup.is_dir())

    def test_systemd_activation_replaces_dropins_with_exact_reviewed_inventory(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            systemd, staged = root / "systemd", root / "staged"
            systemd.mkdir(); staged.mkdir()
            units = (
                "uap-observer.service", "uap-observer-signer.service", "uap-observer-runner.service",
                "uap-observer-runner.socket", "uap-observer-caddy.service",
                "uap-observer-egress-proxy.service", "uap-observer-egress-proxy.socket",
            )
            for unit in units:
                (staged / unit).write_text(f"reviewed {unit}\n")
            for service in ("uap-observer", "uap-observer-runner"):
                reviewed = staged / f"{service}.service.d"
                reviewed.mkdir(); (reviewed / "egress.conf").write_text("sandboxed\n")
                installed = systemd / f"{service}.service.d"
                installed.mkdir(); (installed / "zz-override.conf").write_text("IPAddressDeny=\n")
            result = subprocess.run(
                ["/bin/sh", "-c", '. "$1"; activate_observer_systemd "$2" "$3"',
                 "sh", str(helper), str(staged), str(systemd)], text=True, capture_output=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            for service in ("uap-observer", "uap-observer-runner"):
                self.assertEqual(
                    {path.name for path in (systemd / f"{service}.service.d").iterdir()},
                    {"egress.conf"},
                )

    def test_systemd_activation_checkpoint_is_derived_from_all_nine_entries(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        script = r'''
. "$1"
observer_replace_systemd_entries() { return 0; }
observer_compare_systemd_journal() { return 0; }
validate_observer_systemd_journal() { return 0; }
validate_observer_systemd_inventory() { printf '%s\n' "$observer_install_step"; }
activate_observer_systemd /reviewed /systemd /journal
'''
        completed = subprocess.run(
            ["/bin/sh", "-c", script, "sh", str(helper)],
            text=True, capture_output=True,
        )
        self.assertEqual(completed.returncode, 0, completed.stderr)
        self.assertEqual(completed.stdout.strip(), "9")

        reload_script = r'''
. "$1"
observer_install_step=9
manager() { test "$1" = daemon-reload; }
UAP_OBSERVER_INSTALL_FAIL_AT=10
reload_observer_systemd manager
'''
        failed = subprocess.run(
            ["/bin/sh", "-c", reload_script, "sh", str(helper)],
            text=True, capture_output=True,
        )
        self.assertNotEqual(failed.returncode, 0, "post-topology reload boundary was not addressable")

    def test_systemd_activation_rejects_atime_only_drift_after_journaling(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        units = (
            "uap-observer.service", "uap-observer-signer.service", "uap-observer-runner.service",
            "uap-observer-runner.socket", "uap-observer-caddy.service",
            "uap-observer-egress-proxy.service", "uap-observer-egress-proxy.socket",
        )
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); systemd = root / "systemd"; staged = root / "staged"; backup = root / "backup"
            systemd.mkdir(); staged.mkdir()
            live = systemd / units[0]; live.write_text("original\n")
            for unit in units:
                (staged / unit).write_text(f"reviewed {unit}\n")
            for service in ("uap-observer", "uap-observer-runner"):
                directory = staged / f"{service}.service.d"; directory.mkdir(); (directory / "egress.conf").write_text("reviewed\n")
            original = 1_700_000_000_000_000_000
            os.utime(live, ns=(original, original + 1))
            subprocess.run(
                ["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"',
                 "sh", str(helper), str(backup), str(systemd)], check=True,
            )
            drifted = original + 123_456_789
            os.utime(live, ns=(drifted, original + 1))
            result = subprocess.run(
                ["/bin/sh", "-c", '. "$1"; activate_observer_systemd "$2" "$3" "$4"',
                 "sh", str(helper), str(staged), str(systemd), str(backup)],
                text=True, capture_output=True,
            )
            self.assertNotEqual(result.returncode, 0, "atime-only drift was silently accepted")
            self.assertEqual(live.stat().st_atime_ns, drifted)
            self.assertEqual(live.read_text(), "original\n")

    def test_interrupted_restore_retry_uses_durable_original_symlink_atime(self) -> None:
        self._require_private_noatime_view()
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); systemd = root / "systemd"; backup = root / "backup"
            systemd.mkdir()
            unit = systemd / "uap-observer.service"; unit.write_text("original unit\n")
            legacy = systemd / "legacy.service"; legacy.write_text("legacy\n")
            link = systemd / "uap-observer-signer.service"; link.symlink_to(legacy.name)
            original = 1_700_000_000_111_222_333
            os.utime(unit, ns=(original, original + 1))
            os.utime(link, ns=(original + 2, original + 3), follow_symlinks=False)
            subprocess.run(
                ["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"',
                 "sh", str(helper), str(backup), str(systemd)], check=True,
            )
            unit.write_text("mutated unit\n"); link.unlink(); link.write_text("mutated signer\n")
            failed = subprocess.run(
                ["/bin/sh", "-c", '. "$1"; restore_observer_systemd "$2" "$3"',
                 "sh", str(helper), str(backup), str(systemd)],
                env=dict(os.environ, UAP_OBSERVER_REPLACE_FAIL_AT="2"), text=True, capture_output=True,
            )
            self.assertNotEqual(failed.returncode, 0)
            restored = subprocess.run(
                ["/bin/sh", "-c", '. "$1"; restore_observer_systemd "$2" "$3"',
                 "sh", str(helper), str(backup), str(systemd)], text=True, capture_output=True,
            )
            self.assertEqual(restored.returncode, 0, restored.stderr)
            self.assertEqual(unit.stat().st_atime_ns, original)
            self.assertEqual(link.lstat().st_atime_ns, original + 2)
            self.assertEqual(unit.read_text(), "original unit\n")
            self.assertTrue(link.is_symlink())
            self.assertEqual(os.readlink(link), legacy.name)

    def test_systemd_atomic_replace_does_not_follow_raced_destination_symlink(self) -> None:
        self._require_private_noatime_view()
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            systemd = root / "systemd"; systemd.mkdir()
            staged = root / "reviewed.service"
            staged.write_bytes(b"reviewed\n" + b"x" * (16 << 20))
            destination = systemd / "uap-observer.service"
            destination.write_text("old\n")
            victim = root / "victim"
            victim.write_text("root-owned victim\n")
            (systemd / ".uap-observer-old-crash").symlink_to(victim)
            process = subprocess.Popen([
                "/bin/sh", "-c", '. "$1"; observer_replace_systemd_entries "$2" "$3" "$4"',
                "sh", str(helper), str(systemd), str(staged), destination.name,
            ], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
            self.addCleanup(self._reap_process, process)
            deadline = time.monotonic() + 5
            while time.monotonic() < deadline:
                if any(path.name.startswith(".uap-observer-new-") for path in systemd.iterdir()):
                    destination.unlink()
                    destination.symlink_to(victim)
                    break
                time.sleep(0.001)
            else:
                process.kill()
                self.fail("exclusive systemd staging entry was not observed")
            stdout, stderr = process.communicate(timeout=10)
            self.assertNotEqual(process.returncode, 0, "raced destination was silently overwritten")
            self.assertIn("destination raced", stderr)
            self.assertEqual(victim.read_text(), "root-owned victim\n")
            self.assertFalse((systemd / ".uap-observer-old-crash").exists())
            self.assertTrue(destination.is_symlink())
            self.assertEqual(os.readlink(destination), str(victim))

    def test_restore_keeps_complete_journal_metadata_unchanged_at_every_boundary(self) -> None:
        self._require_private_noatime_view()
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        def snapshot(path: Path) -> tuple:
            flags = os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW | getattr(os, "O_NOATIME", 0)
            def visit(parent: int, name: str, relative: str) -> tuple:
                info = os.stat(name, dir_fd=parent, follow_symlinks=False)
                proc_path = f"/proc/self/fd/{parent}/{name}"
                attrs = tuple(sorted((key, os.getxattr(proc_path, key, follow_symlinks=False)) for key in os.listxattr(proc_path, follow_symlinks=False)))
                common = (relative, stat.S_IFMT(info.st_mode), stat.S_IMODE(info.st_mode), info.st_uid, info.st_gid, info.st_atime_ns, info.st_mtime_ns, info.st_nlink, attrs)
                if stat.S_ISDIR(info.st_mode):
                    descriptor = os.open(name, flags | os.O_DIRECTORY, dir_fd=parent)
                    try:
                        return common + (tuple(visit(descriptor, child, f"{relative}/{child}") for child in sorted(os.listdir(descriptor))),)
                    finally: os.close(descriptor)
                if stat.S_ISREG(info.st_mode):
                    descriptor = os.open(name, flags, dir_fd=parent)
                    try:
                        payload = b""
                        while block := os.read(descriptor, 1 << 20): payload += block
                    finally: os.close(descriptor)
                    return common + (payload,)
                value = os.readlink(name, dir_fd=parent)
                os.utime(name, ns=(info.st_atime_ns, info.st_mtime_ns), dir_fd=parent, follow_symlinks=False)
                return common + (value,)
            parent = os.open(path.parent, flags | os.O_DIRECTORY)
            try: return visit(parent, path.name, path.name)
            finally: os.close(parent)
        for boundary in range(1, 8):
            with self.subTest(boundary=boundary), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary); systemd = root / "systemd"; backup = root / "backup"
                systemd.mkdir(); (systemd / "uap-observer.service").write_text("original\n")
                (systemd / "uap-observer-signer.service").symlink_to("legacy.service")
                subprocess.run(["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"', "sh", str(helper), str(backup), str(systemd)], check=True)
                before = snapshot(backup)
                (systemd / "uap-observer.service").write_text("mutated\n")
                (systemd / "uap-observer-signer.service").unlink()
                failed = subprocess.run(["/bin/sh", "-c", '. "$1"; restore_observer_systemd "$2" "$3"', "sh", str(helper), str(backup), str(systemd)], env=dict(os.environ, UAP_OBSERVER_REPLACE_FAIL_AT=str(boundary)))
                self.assertNotEqual(failed.returncode, 0)
                self.assertEqual(snapshot(backup), before)
                subprocess.run(["/bin/sh", "-c", '. "$1"; restore_observer_systemd "$2" "$3"', "sh", str(helper), str(backup), str(systemd)], check=True)
                self.assertEqual(snapshot(backup), before)

    def test_egress_service_and_socket_symlinks_are_journaled_and_rollback_recoverable(self) -> None:
        self._require_private_noatime_view()
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        units = ("uap-observer-egress-proxy.service", "uap-observer-egress-proxy.socket")
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            systemd, staged, backup = root / "systemd", root / "staged", root / "backup"
            systemd.mkdir(); staged.mkdir()
            targets = {}
            for unit in units:
                target = root / f"legacy-{unit}"
                target.write_text(f"legacy {unit}\n")
                (systemd / unit).symlink_to(target.name)
                targets[unit] = target.name
                (staged / unit).write_text(f"reviewed {unit}\n")
            subprocess.run([
                "/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"',
                "sh", str(helper), str(backup), str(systemd),
            ], check=True)
            replaced = subprocess.run([
                "/bin/sh", "-c",
                '. "$1"; UAP_OBSERVER_COMPARE_BACKUP=$2 observer_replace_systemd_entries "$3" "$4" "$5" "$6" "$7"',
                "sh", str(helper), str(backup), str(systemd),
                str(staged / units[0]), units[0], str(staged / units[1]), units[1],
            ], text=True, capture_output=True)
            self.assertEqual(replaced.returncode, 0, replaced.stderr)
            self.assertTrue(all((systemd / unit).is_file() and not (systemd / unit).is_symlink() for unit in units))
            restored = subprocess.run([
                "/bin/sh", "-c", '. "$1"; restore_observer_systemd "$2" "$3" "$4"',
                "sh", str(helper), str(backup), str(systemd), str(staged),
            ], text=True, capture_output=True)
            self.assertEqual(restored.returncode, 0, restored.stderr)
            for unit in units:
                self.assertTrue((systemd / unit).is_symlink())
                self.assertEqual(os.readlink(systemd / unit), targets[unit])

    def test_systemd_replace_rejects_raced_hardlink_content_and_metadata(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        for race in ("hardlink", "content", "metadata"):
            with self.subTest(race=race), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary); systemd = root / "systemd"; systemd.mkdir()
                staged = root / "reviewed"; staged.write_bytes(b"reviewed\n" + b"x" * (16 << 20))
                destination = systemd / "uap-observer.service"; destination.write_text("old\n")
                victim = root / "victim"; victim.write_text("victim\n")
                process = subprocess.Popen(["/bin/sh", "-c", '. "$1"; observer_replace_systemd_entries "$2" "$3" "$4"', "sh", str(helper), str(systemd), str(staged), destination.name], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
                self.addCleanup(self._reap_process, process)
                deadline = time.monotonic() + 5
                while time.monotonic() < deadline and not any(item.name.startswith(".uap-observer-new-") for item in systemd.iterdir()): time.sleep(0.001)
                self.assertLess(time.monotonic(), deadline, "exclusive replacement was not observed")
                if race == "hardlink": destination.unlink(); os.link(victim, destination)
                elif race == "content": destination.write_text("raced\n")
                else: destination.chmod(0o600)
                stdout, stderr = process.communicate(timeout=10)
                self.assertNotEqual(process.returncode, 0, stdout + stderr)
                self.assertIn("destination raced", stderr)
                self.assertEqual(victim.read_text(), "victim\n")

    def test_systemd_replace_rechecks_pinned_directory_and_top_inventory_before_mutation(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        for race in ("directory", "inventory"):
            with self.subTest(race=race), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary); systemd = root / "systemd"; systemd.mkdir()
                destination = systemd / "uap-observer.service.d"; destination.mkdir(); (destination / "old.conf").write_text("old\n")
                staged = root / "reviewed"; staged.mkdir(); (staged / "egress.conf").write_bytes(b"x" * (32 << 20))
                process = subprocess.Popen(["/bin/sh", "-c", '. "$1"; observer_replace_systemd_entries "$2" "$3" "$4"', "sh", str(helper), str(systemd), str(staged), destination.name], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
                self.addCleanup(self._reap_process, process)
                deadline = time.monotonic() + 5
                while time.monotonic() < deadline and not any(item.name.startswith(".uap-observer-new-") for item in systemd.iterdir()): time.sleep(0.001)
                self.assertLess(time.monotonic(), deadline, "exclusive replacement was not observed")
                if race == "directory": staged.chmod(0o700)
                else: (systemd / "uap-observer-unexpected.service").write_text("raced\n")
                stdout, stderr = process.communicate(timeout=15)
                self.assertNotEqual(process.returncode, 0, stdout + stderr)
                self.assertEqual((destination / "old.conf").read_text(), "old\n")
                if race == "inventory": self.assertIn("inventory changed", stderr)

    def test_systemd_replace_rebinds_inventory_after_displacement_and_rolls_back_exactly(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); systemd = root / "systemd"; systemd.mkdir()
            destination = systemd / "uap-observer.service"; destination.write_text("original\n")
            staged = root / "reviewed"; staged.write_text("reviewed\n")
            backup = root / "backup"
            subprocess.run(
                ["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"',
                 "sh", str(helper), str(backup), str(systemd)], check=True,
            )
            script = '. "$1"; UAP_OBSERVER_COMPARE_BACKUP=$2 observer_replace_systemd_entries "$3" "$4" "$5"'
            ready_read, ready_write = os.pipe(); resume_read, resume_write = os.pipe()
            process = None
            try:
                process = subprocess.Popen(
                    ["/bin/sh", "-c", script, "sh", str(helper), str(backup), str(systemd), str(staged), destination.name],
                    env={**os.environ, "UAP_OBSERVER_TEST_DISPLACEMENT_READY_FD": str(ready_write),
                         "UAP_OBSERVER_TEST_DISPLACEMENT_RESUME_FD": str(resume_read)},
                    pass_fds=(ready_write, resume_read), text=True, stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE, start_new_session=True,
                )
                os.close(ready_write); ready_write = -1
                os.close(resume_read); resume_read = -1
                ready, _, _ = select.select([ready_read], [], [], 10)
                self.assertTrue(ready, "replacement did not reach the displacement boundary")
                self.assertEqual(os.read(ready_read, 1), b"1")
                self.assertEqual(len(tuple(systemd.glob(".uap-observer-old-*"))), 1)
                unexpected = systemd / "uap-observer-unexpected.service"
                unexpected.write_text("preserve me\n")
                os.write(resume_write, b"1")
                os.close(resume_write); resume_write = -1
                stdout, stderr = process.communicate(timeout=15)
                self.assertNotEqual(process.returncode, 0, stdout + stderr)
                self.assertIn("inventory changed", stderr)
                self.assertFalse(destination.exists(), "rollback mutated after authoritative inventory drift")
                self.assertEqual(unexpected.read_text(), "preserve me\n")
                self.assertTrue(backup.is_dir(), "durable rollback journal was removed")
                self.assertEqual((backup / "items/0").read_text(), "original\n")
                displaced = tuple(systemd.glob(".uap-observer-old-*"))
                self.assertEqual(len(displaced), 1)
                self.assertEqual(displaced[0].read_text(), "original\n")
            finally:
                for descriptor in (ready_read, ready_write, resume_read, resume_write):
                    if descriptor >= 0:
                        os.close(descriptor)
                if process is not None and process.poll() is None:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.communicate()

    def test_recovery_rejects_unexpected_observer_name_and_preserves_journal(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); stage = root / "stage"; systemd = root / "systemd"; closures = root / "closures"
            stage.mkdir(mode=0o700); systemd.mkdir(); closures.mkdir()
            original = systemd / "uap-observer.service"; original.write_text("original\n")
            backup = stage / "systemd-backup"
            subprocess.run(
                ["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"',
                 "sh", str(helper), str(backup), str(systemd)], check=True,
            )
            original.write_text("partially installed\n")
            unexpected = systemd / "uap-observer-unexpected.service"; unexpected.write_text("do not touch\n")
            digest = "a" * 64
            for name, value in (("closure-digest", digest + "\n"), ("journal-committed", "committed-v1\n")):
                path = stage / name; path.write_text(value); path.chmod(0o600)
            (closures / digest).mkdir()
            manager = root / "systemctl"; manager.write_text("#!/bin/sh\nexit 97\n"); manager.chmod(0o755)
            result = subprocess.run(
                ["/bin/sh", "-c", '. "$1"; recover_observer_install "$2" "$3" "$4" "$5" "$6"',
                 "sh", str(helper), str(stage), str(closures), str(root / "current"), str(systemd), str(manager)],
                text=True, capture_output=True,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertTrue(stage.is_dir())
            self.assertEqual(original.read_text(), "partially installed\n", result.stdout + result.stderr)
            self.assertEqual(unexpected.read_text(), "do not touch\n")
            self.assertEqual((backup / "items/0").read_text(), "original\n")

    def test_recovery_failpoints_are_exactly_retryable_at_every_rollback_boundary(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        boundaries = (
            "before-rollback-daemon-reload", "after-rollback-daemon-reload",
            "before-rollback-closure-deletion", "after-rollback-closure-deletion",
            "after-rollback-closure-fsync", "before-journal-resolution", "after-journal-resolution",
            "before-partial-cleanup", "after-partial-cleanup",
            "before-rollback-data-deletion", "after-rollback-data-deletion",
            "after-resolved-journal-rename", "after-resolved-journal-parent-fsync",
            "before-journal-directory-deletion", "after-journal-directory-deletion",
            "after-journal-parent-fsync",
        )
        if selected := os.environ.get("UAP_OBSERVER_TEST_RECOVERY_BOUNDARY"):
            boundaries = (selected,)
        script = '. "$1"; fixture_cleanup() { :; }; recover_observer_install "$2" "$3" "$4" "$5" "$6" fixture_cleanup'
        for boundary in boundaries:
            with self.subTest(boundary=boundary), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary); stage = root / "stage"; systemd = root / "systemd"; closures = root / "closures"
                stage.mkdir(mode=0o700); systemd.mkdir(); closures.mkdir()
                original = systemd / "uap-observer.service"; original.write_text("original\n")
                backup = stage / "systemd-backup"
                subprocess.run(
                    ["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"',
                     "sh", str(helper), str(backup), str(systemd)], check=True,
                )
                original.write_text("partially installed\n")
                installed = stage / "systemd"; installed.mkdir()
                shutil.copy2(original, installed / original.name)
                victim = root / "victim"; victim.write_text("never overwrite\n")
                digest = "b" * 64
                for name, value in (("closure-digest", digest + "\n"), ("journal-committed", "committed-v1\n")):
                    path = stage / name; path.write_text(value); path.chmod(0o600)
                candidate = closures / digest; candidate.mkdir(); (candidate / "partial").write_text("discard\n")
                manager = root / "systemctl"; manager.write_text("#!/bin/sh\ntest \"$1\" = daemon-reload\n"); manager.chmod(0o755)
                failed = subprocess.run(
                    ["/bin/sh", "-c", script, "sh", str(helper), str(stage), str(closures),
                     str(root / "current"), str(systemd), str(manager)],
                    env={**os.environ, "UAP_OBSERVER_RECOVERY_FAILPOINT": boundary},
                    text=True, capture_output=True,
                )
                self.assertNotEqual(failed.returncode, 0, boundary)
                if stage.exists() and not (stage / "journal-resolved").exists():
                    self.assertTrue(backup.is_dir(), f"{boundary} lost unresolved rollback evidence")
                retried = subprocess.run(
                    ["/bin/sh", "-c", script, "sh", str(helper), str(stage), str(closures),
                     str(root / "current"), str(systemd), str(manager)],
                    text=True, capture_output=True,
                )
                self.assertEqual(retried.returncode, 0, retried.stderr)
                self.assertEqual(original.read_text(), "original\n")
                self.assertEqual(victim.read_text(), "never overwrite\n")
                self.assertFalse(candidate.exists())
                self.assertFalse(stage.exists())

    def test_systemd_journal_rejects_escape_and_non_single_link_topology_before_mutation(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        for kind in ("dropin-symlink", "unit-hardlink", "dropin-hardlink", "root-writable", "unit-nonroot"):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary); systemd = root / "systemd"; systemd.mkdir()
                unit = systemd / "uap-observer.service"; unit.write_text("old\n")
                dropin = systemd / "uap-observer.service.d"
                outside = root / "outside"; outside.mkdir()
                if kind == "root-writable":
                    systemd.chmod(0o777)
                elif kind == "unit-nonroot":
                    try:
                        os.chown(unit, 12345, 12345)
                    except OSError as error:
                        raise unittest.SkipTest(
                            "provider filesystem cannot create a non-root-owned topology fixture"
                        ) from error
                elif kind == "dropin-symlink":
                    dropin.symlink_to(outside, target_is_directory=True)
                else:
                    dropin.mkdir(); conf = dropin / "old.conf"; conf.write_text("old\n")
                    source = unit if kind == "unit-hardlink" else conf
                    os.link(source, outside / "external-link")
                before = unit.read_bytes()
                result = subprocess.run(
                    ["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"',
                     "sh", str(helper), str(root / "backup"), str(systemd)],
                    text=True, capture_output=True,
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(unit.read_bytes(), before)
                self.assertFalse((root / "backup").exists())

    def test_uncommitted_partial_journal_is_discarded_without_restore(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); stage = root / "stage"; systemd = root / "systemd"
            stage.mkdir(mode=0o700); systemd.mkdir()
            unit = systemd / "uap-observer.service"; unit.write_text("live\n")
            (stage / "closure-digest").write_text("a" * 64 + "\n")
            (stage / "closure-digest").chmod(0o600)
            (stage / "systemd-backup").mkdir()
            result = subprocess.run(
                ["/bin/sh", "-c", '. "$1"; cleanup_fixture() { :; }; recover_observer_install "$2" "$3" "$4" "$5" /bin/false cleanup_fixture',
                 "sh", str(helper), str(stage), str(root / "closures"), str(root / "current"), str(systemd)],
                text=True, capture_output=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(unit.read_text(), "live\n")
            self.assertFalse(stage.exists())

    def test_resolved_journal_without_authenticated_material_fails_closed(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); stage = root / "stage"
            stage.mkdir(mode=0o700)
            for name, value in (
                ("journal-resolved", "resolved-v1\n"),
                ("journal-committed", "committed-v1\n"),
                ("closure-digest", "a" * 64 + "\n"),
            ):
                (stage / name).write_text(value); (stage / name).chmod(0o600)
            # This is the durable interruption state: restoration completed and
            # was tombstoned, then backup removal committed before old markers.
            self.assertFalse((stage / "systemd-backup").exists())
            result = subprocess.run([
                "/bin/sh", "-c",
                '. "$1"; cleanup_fixture() { :; }; recover_observer_install "$2" "$3" "$4" "$5" /bin/false cleanup_fixture',
                "sh", str(helper), str(stage), str(root / "closures"),
                str(root / "current"), str(root / "systemd"),
            ], text=True, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertTrue(stage.exists())

    def test_reload_helper_propagates_manager_failure_in_conditional_context(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            manager = Path(temporary) / "systemctl"
            manager.write_text("#!/bin/sh\nexit 42\n"); manager.chmod(0o755)
            script = '. "$1"; observer_install_step=0; if reload_observer_systemd "$2"; then exit 99; else test "$?" -eq 1; fi'
            result = subprocess.run(["/bin/sh", "-c", script, "sh", str(helper), str(manager)], text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)

    def test_closure_identity_binds_xattrs_and_completed_validation_rejects_them(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            closure, _, install_identity, owner = self._completed_closure_fixture(root)
            config = closure / "etc/uap-observer-adapter-config.json"
            before = closure.name
            try:
                os.setxattr(config, "user.uap_observer_identity", b"named-acl-equivalent", follow_symlinks=False)
            except OSError as error:
                self.skipTest(f"fixture filesystem does not support user xattrs: {error}")
            helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
            after = subprocess.check_output(
                ["/bin/sh", "-c", '. "$1"; observer_closure_identity "$2"', "sh", str(helper), str(closure)], text=True,
            ).strip()
            self.assertNotEqual(after, before)
            result = self._validate_completed_closure(root, install_identity, owner)
            self.assertNotEqual(result.returncode, 0)

    def test_finalized_closure_imports_are_bytecode_free_and_identity_stable(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        installer = (Path(__file__).parents[2] / "deploy/uap-observer-install.sh").read_text()
        finalized = installer[installer.index('closure_digest=$(observer_closure_identity "$closure_stage")'):]
        for line in finalized.splitlines():
            if "python" in line and not line.lstrip().startswith("#"):
                self.assertIn("PYTHONDONTWRITEBYTECODE=1", line)
                self.assertIn("-B", line)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); closures = root / "closures"; staged = closures / ".new"
            runtime = staged / "runtime"; libexec = staged / "libexec"
            runtime.mkdir(parents=True); libexec.mkdir()
            (runtime / "finalized_module.py").write_text("VALUE = 7\n")
            adapter = libexec / "uap-observer-fixed-adapter"
            adapter.write_text("#!/bin/sh\nexit 1\n"); adapter.chmod(0o755)
            for name in ("runtime", "notion", "chatgpt", "consent"):
                os.link(adapter, libexec / f"uap-observer-adapter-{name}")
            self._populate_complete_observer_inventory(staged / "systemd")
            identity = subprocess.check_output(
                ["/bin/sh", "-c", '. "$1"; observer_closure_identity "$2"', "sh", str(helper), str(staged)], text=True,
            ).strip()
            closure = closures / identity; staged.rename(closure)
            subprocess.run(
                ["/usr/bin/python3", "-B", "-c", "import finalized_module; assert finalized_module.VALUE == 7"],
                env={**os.environ, "PYTHONDONTWRITEBYTECODE": "1", "PYTHONPATH": str(closure / "runtime")}, check=True,
            )
            self.assertFalse(any(path.name == "__pycache__" or path.suffix in {".pyc", ".pyo"} for path in closure.rglob("*")))
            repeated = subprocess.check_output(
                ["/bin/sh", "-c", '. "$1"; observer_closure_identity "$2"', "sh", str(helper), str(closure)], text=True,
            ).strip()
            self.assertEqual(repeated, identity)

    def test_real_venv_and_pip_build_path_is_bytecode_free_before_publication(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        installer = (Path(__file__).parents[2] / "deploy/uap-observer-install.sh").read_text()
        self.assertIn("pip install --no-compile --require-hashes", installer)
        self.assertLess(installer.index("observer_remove_python_bytecode /opt/uap-observer-venv.new"),
                        installer.index('mv /opt/uap-observer-venv.new "$closure_stage/venv"'))
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); package = root / "fixturepkg-1-py3-none-any.whl"
            with zipfile.ZipFile(package, "w") as wheel:
                wheel.writestr("fixturepkg.py", "VALUE = 7\n")
                wheel.writestr("fixturepkg-1.dist-info/METADATA", "Metadata-Version: 2.1\nName: fixturepkg\nVersion: 1\n")
                wheel.writestr("fixturepkg-1.dist-info/WHEEL",
                               "Wheel-Version: 1.0\nGenerator: observer-fixture\nRoot-Is-Purelib: true\nTag: py3-none-any\n")
                wheel.writestr("fixturepkg-1.dist-info/RECORD", "")
            venv = root / "venv"
            managed = Path.home() / ".local/share/uv/python"
            interpreters = [Path("/usr/bin/python3"), *sorted(managed.glob("*/bin/python3"))]
            failures = []
            for interpreter in interpreters:
                if not interpreter.is_file():
                    continue
                shutil.rmtree(venv, ignore_errors=True)
                built = subprocess.run([str(interpreter), "-B", "-m", "venv", str(venv)],
                                       text=True, capture_output=True)
                if built.returncode == 0:
                    break
                failures.append(f"{interpreter}: {built.stdout}{built.stderr}")
            else:
                self.fail("no available Python could construct a pip-backed venv:\n" + "\n".join(failures))
            subprocess.run(
                [str(venv / "bin/python"), "-B", "-m", "pip", "install", "--no-compile", "--no-deps",
                 str(package)],
                env={**os.environ, "PYTHONDONTWRITEBYTECODE": "1"}, check=True, capture_output=True, text=True,
            )
            # ensurepip itself normally leaves caches; exercise the production
            # descriptor-safe cleanup rather than manufacturing a clean tree.
            self.assertTrue(any(path.name == "__pycache__" for path in venv.rglob("*")))
            subprocess.run(["/bin/sh", "-c", '. "$1"; observer_remove_python_bytecode "$2"',
                            "sh", str(helper), str(venv)], check=True)
            self.assertFalse(any(path.name == "__pycache__" or path.suffix in {".pyc", ".pyo"}
                                 for path in venv.rglob("*")))

    def test_production_systemd_stage_closure_and_live_share_one_stable_authority(self) -> None:
        self._require_private_noatime_view()
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); stage = root / "stage"; closure = root / "closure"; live = root / "live"
            self._populate_complete_observer_inventory(stage)
            for name in ("uap-observer.service.d", "uap-observer-runner.service.d"):
                (stage / name).chmod(0o755)
            subprocess.run(["/bin/sh", "-c", '. "$1"; observer_normalize_tree_mtime "$2"; '
                            'mkdir "$3" "$4"; observer_copy_systemd_tree_neutral "$2" "$3"; '
                            'activate_observer_systemd "$2" "$4"',
                            "sh", str(helper), str(stage), str(closure), str(live)], check=True)
            for tree in (stage, closure, live):
                for name in ("uap-observer.service.d", "uap-observer-runner.service.d"):
                    self.assertEqual(stat.S_IMODE((tree / name).stat().st_mode), 0o755)
            compare = '. "$1"; observer_compare_systemd_trees "$2" "$3"'
            for left, right in ((stage, closure), (closure, live), (stage, live)):
                subprocess.run(["/bin/sh", "-c", compare, "sh", str(helper), str(left), str(right)], check=True)

    def test_closure_identity_binds_nanosecond_mtime_for_file_directory_and_symlink(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            closure, _, _, _ = self._completed_closure_fixture(Path(temporary))
            baseline = closure.name
            for target in (closure / "systemd/uap-observer.service",
                           closure / "systemd/uap-observer.service.d", closure / "payload-link"):
                with self.subTest(target=target.name):
                    before = target.lstat()
                    try:
                        os.utime(target, ns=(before.st_atime_ns, before.st_mtime_ns + 1), follow_symlinks=False)
                    except (NotImplementedError, OSError) as error:
                        if target.is_symlink():
                            self.skipTest(f"fixture filesystem does not support symlink mtime: {error}")
                        raise
                    changed = subprocess.check_output(
                        ["/bin/sh", "-c", '. "$1"; observer_closure_identity "$2"',
                         "sh", str(helper), str(closure)], text=True,
                    ).strip()
                    self.assertNotEqual(changed, baseline)
                    os.utime(target, ns=(before.st_atime_ns, before.st_mtime_ns), follow_symlinks=False)

    def test_completed_production_validation_rejects_stable_live_drift(self) -> None:
        for drift in ("content", "mode", "mtime", "xattr", "acl", "security-label"):
            with self.subTest(drift=drift), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                _, _, install_identity, owner = self._completed_closure_fixture(root)
                target = root / "systemd/uap-observer.service"
                if drift == "content": target.write_text("drift\n")
                elif drift == "mode": target.chmod(0o600)
                elif drift == "mtime":
                    info=target.stat(); os.utime(target,ns=(info.st_atime_ns,info.st_mtime_ns+1))
                elif drift == "xattr":
                    try: os.setxattr(target,"user.uap_observer_live_acl_label",b"drift",follow_symlinks=False)
                    except OSError as error: self.skipTest(f"fixture filesystem lacks xattrs: {error}")
                elif drift == "acl":
                    acl = struct.pack("<I", 2) + b"".join(
                        struct.pack("<HHI", tag, permissions, identifier)
                        for tag, permissions, identifier in (
                            (1, 6, 0xFFFFFFFF), (2, 4, 1), (4, 4, 0xFFFFFFFF),
                            (16, 4, 0xFFFFFFFF), (32, 4, 0xFFFFFFFF),
                        )
                    )
                    try: os.setxattr(target, "system.posix_acl_access", acl, follow_symlinks=False)
                    except OSError as error: self.skipTest(f"fixture filesystem lacks POSIX ACL xattrs: {error}")
                else:
                    capability = bytes.fromhex("0100000200040000000000000000000000000000")
                    try: os.setxattr(target, "security.capability", capability, follow_symlinks=False)
                    except OSError as error: self.skipTest(f"fixture filesystem lacks security xattrs: {error}")
                result = self._validate_completed_closure(root, install_identity, owner)
                self.assertNotEqual(result.returncode, 0, f"{drift} live drift was accepted")

    def test_current_recovery_uses_authenticated_closure_not_identically_tampered_stage_and_live(self) -> None:
        self._require_private_noatime_view()
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); stage = root / "stage"; closures = root / "closures"; systemd = root / "systemd"
            stage.mkdir(mode=0o700); closures.mkdir(); systemd.mkdir()
            subprocess.run(["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"', "sh", str(helper), str(stage / "systemd-backup"), str(systemd)], check=True)
            closure, digest = self._create_hashed_closure(closures, helper)
            for name, value in (("closure-digest", digest + "\n"), ("journal-committed", "committed-v1\n")):
                (stage / name).write_text(value); (stage / name).chmod(0o600)
            current = root / "current"; current.symlink_to(f"uap-observer-closures/{digest}")
            self._copy_observer_inventory(closure / "systemd", stage / "systemd")
            self._copy_observer_inventory(stage / "systemd", systemd)
            for tree in (stage / "systemd", systemd):
                (tree / "uap-observer.service").write_text("identically tampered\n")
            result = subprocess.run(
                ["/bin/sh", "-c", '. "$1"; cleanup_fixture() { :; }; recover_observer_install "$2" "$3" "$4" "$5" /bin/true cleanup_fixture',
                 "sh", str(helper), str(stage), str(closures), str(current), str(systemd)], text=True, capture_output=True,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertTrue(stage.is_dir())
            self.assertEqual((closure / "systemd/uap-observer.service").read_text(), "installed\n")

    def test_resolved_current_retry_revalidates_pointer_systemd_and_closure_drift(self) -> None:
        self._require_private_noatime_view()
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        for drift in ("pointer", "systemd", "closure"):
            with self.subTest(drift=drift), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary); stage = root / "stage"; closures = root / "closures"; systemd = root / "systemd"
                stage.mkdir(mode=0o700); closures.mkdir(); systemd.mkdir()
                subprocess.run(["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"', "sh", str(helper), str(stage / "systemd-backup"), str(systemd)], check=True)
                closure, digest = self._create_hashed_closure(closures, helper)
                for name, value in (("closure-digest", digest + "\n"), ("journal-committed", "committed-v1\n")):
                    (stage / name).write_text(value); (stage / name).chmod(0o600)
                current = root / "current"; current.symlink_to(f"uap-observer-closures/{digest}")
                self._copy_observer_inventory(closure / "systemd", stage / "systemd")
                self._copy_observer_inventory(closure / "systemd", systemd)
                command = ["/bin/sh", "-c", '. "$1"; cleanup_fixture() { :; }; recover_observer_install "$2" "$3" "$4" "$5" /bin/true cleanup_fixture',
                           "sh", str(helper), str(stage), str(closures), str(current), str(systemd)]
                interrupted = subprocess.run(command, env={**os.environ, "UAP_OBSERVER_RECOVERY_FAILPOINT": "after-journal-resolution"}, text=True, capture_output=True)
                self.assertNotEqual(interrupted.returncode, 0)
                self.assertTrue((stage / "journal-resolved").is_file())
                if drift == "pointer":
                    current.unlink(); current.symlink_to("uap-observer-closures/" + "0" * 64)
                elif drift == "systemd":
                    (systemd / "uap-observer.service").write_text("drifted\n")
                else:
                    (closure / "drift").write_text("drifted\n")
                retried = subprocess.run(command, text=True, capture_output=True)
                self.assertNotEqual(retried.returncode, 0)
                self.assertTrue((stage / "journal-resolved").is_file())

    def test_systemd_regular_comparison_rejects_coordinated_content_and_xattr_races(self) -> None:
        self._require_private_noatime_view()
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        for race in ("content", "xattr"):
            with self.subTest(race=race), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary); reviewed = root / "reviewed"; live = root / "live"
                self._populate_complete_observer_inventory(reviewed)
                self._copy_observer_inventory(reviewed, live)
                ready_read, ready_write = os.pipe(); resume_read, resume_write = os.pipe()
                process = None
                try:
                    process = subprocess.Popen(
                        ["/bin/sh", "-c", '. "$1"; observer_compare_systemd_trees "$2" "$3"',
                         "sh", str(helper), str(reviewed), str(live)],
                        env={**os.environ, "UAP_OBSERVER_TEST_SYSTEMD_COMPARE_READY_FD": str(ready_write),
                             "UAP_OBSERVER_TEST_SYSTEMD_COMPARE_RESUME_FD": str(resume_read),
                             "UAP_OBSERVER_TEST_SYSTEMD_COMPARE_NAME": "uap-observer.service"},
                        pass_fds=(ready_write, resume_read), text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                        start_new_session=True,
                    )
                    os.close(ready_write); ready_write = -1
                    os.close(resume_read); resume_read = -1
                    ready, _, _ = select.select([ready_read], [], [], 10)
                    self.assertTrue(ready, "systemd comparison did not reach coordinated boundary")
                    self.assertEqual(os.read(ready_read, 1), b"1")
                    target = live / "uap-observer.service"
                    if race == "content":
                        target.write_text("raced content\n")
                    else:
                        try: os.setxattr(target, "user.uap_observer_race", b"raced", follow_symlinks=False)
                        except OSError as error:
                            self.skipTest(f"fixture filesystem does not support user xattrs: {error}")
                    os.write(resume_write, b"1")
                    os.close(resume_write); resume_write = -1
                    stdout, stderr = process.communicate(timeout=15)
                    self.assertNotEqual(process.returncode, 0, stdout + stderr)
                finally:
                    for descriptor in (ready_read, ready_write, resume_read, resume_write):
                        if descriptor >= 0:
                            os.close(descriptor)
                    if process is not None and process.poll() is None:
                        os.killpg(process.pid, signal.SIGKILL)
                        process.communicate()

    def test_systemd_regular_comparison_rejects_incomplete_race_synchronization(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); reviewed = root / "reviewed"; live = root / "live"
            self._populate_complete_observer_inventory(reviewed)
            self._copy_observer_inventory(reviewed, live)
            ready_read, ready_write = os.pipe()
            try:
                completed = subprocess.run(
                    ["/bin/sh", "-c", '. "$1"; observer_compare_systemd_trees "$2" "$3"',
                     "sh", str(helper), str(reviewed), str(live)],
                    env={**os.environ, "UAP_OBSERVER_TEST_SYSTEMD_COMPARE_READY_FD": str(ready_write),
                         "UAP_OBSERVER_TEST_SYSTEMD_COMPARE_NAME": "uap-observer.service"},
                    pass_fds=(ready_write,), text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                    timeout=15,
                )
                self.assertNotEqual(completed.returncode, 0, completed.stdout + completed.stderr)
                self.assertIn("incomplete systemd comparison test synchronization", completed.stderr)
            finally:
                os.close(ready_read)
                os.close(ready_write)

    def test_systemd_closure_live_comparison_allows_only_volatile_atime_drift(self) -> None:
        self._require_private_noatime_view()
        if os.geteuid() != 0:
            self.skipTest("root ownership drift fixture requires root")
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); reviewed = root / "reviewed"; live = root / "live"
            self._populate_complete_observer_inventory(reviewed)
            self._copy_observer_inventory(reviewed, live)
            reviewed_unit = reviewed / "uap-observer.service"
            live_unit = live / "uap-observer.service"
            fixed_mtime = 1_700_000_000_123_456_789
            fixed_atime = fixed_mtime - 10 * 24 * 60 * 60 * 1_000_000_000
            for unit in (reviewed_unit, live_unit):
                os.setxattr(unit, "user.uap_observer_authority", b"reviewed", follow_symlinks=False)
                os.utime(unit, ns=(fixed_atime, fixed_mtime), follow_symlinks=False)

            # Model systemd's ordinary read after daemon-reload.  Relatime
            # must advance this deliberately old atime on the live unit.
            with live_unit.open("rb") as stream:
                self.assertEqual(stream.read(1), b"i")
            self.assertGreater(live_unit.stat().st_atime_ns, fixed_atime)
            self.assertEqual(reviewed_unit.stat().st_atime_ns, fixed_atime)

            command = [
                "/bin/sh", "-c", '. "$1"; observer_compare_systemd_trees "$2" "$3"',
                "sh", str(helper), str(reviewed), str(live),
            ]
            accepted = subprocess.run(command, text=True, capture_output=True)
            self.assertEqual(accepted.returncode, 0, accepted.stderr)

            original = live_unit.read_bytes()
            original_mode = stat.S_IMODE(live_unit.stat().st_mode)
            for drift in ("content", "mode", "owner", "xattr"):
                with self.subTest(drift=drift):
                    if drift == "content":
                        live_unit.write_bytes(b"authenticated content drift\n")
                    elif drift == "mode":
                        live_unit.chmod(original_mode ^ stat.S_IXUSR)
                    elif drift == "owner":
                        os.chown(live_unit, 1, 0)
                    else:
                        os.setxattr(live_unit, "user.uap_observer_authority", b"drifted", follow_symlinks=False)
                    rejected = subprocess.run(command, text=True, capture_output=True)
                    self.assertNotEqual(rejected.returncode, 0, f"{drift} drift passed authentication")
                    live_unit.write_bytes(original)
                    live_unit.chmod(original_mode)
                    os.chown(live_unit, 0, 0)
                    os.setxattr(live_unit, "user.uap_observer_authority", b"reviewed", follow_symlinks=False)
                    os.utime(live_unit, ns=(fixed_atime, fixed_mtime), follow_symlinks=False)

    def test_recovery_manager_read_advances_atime_but_stable_journal_authority_holds(self) -> None:
        self._require_private_noatime_view()
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); stage = root / "stage"; closures = root / "closures"; systemd = root / "systemd"
            stage.mkdir(mode=0o700); closures.mkdir(); self._populate_complete_observer_inventory(systemd)
            unit = systemd / "uap-observer.service"
            mtime = 1_700_000_000_123_456_789; old_atime = mtime - 30 * 86400 * 1_000_000_000
            os.utime(unit, ns=(old_atime, mtime))
            subprocess.run(["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"',
                            "sh", str(helper), str(stage / "systemd-backup"), str(systemd)], check=True)
            self._copy_observer_inventory(systemd, stage / "systemd")
            unit.write_text("installed value\n")
            digest = "a" * 64; (closures / digest).mkdir()
            for name, value in (("closure-digest", digest + "\n"), ("journal-committed", "committed-v1\n")):
                (stage / name).write_text(value); (stage / name).chmod(0o600)
            manager = root / "systemctl"
            manager.write_text(f"#!/bin/sh\ntest \"$1\" = daemon-reload\n/usr/bin/head -c 1 '{unit}' >/dev/null\n")
            manager.chmod(0o755)
            result = subprocess.run(
                ["/bin/sh", "-c", '. "$1"; cleanup_fixture() { :; }; recover_observer_install "$2" "$3" "$4" "$5" "$6" cleanup_fixture',
                 "sh", str(helper), str(stage), str(closures), str(root / "current"), str(systemd), str(manager)],
                text=True, capture_output=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertGreater(unit.stat().st_atime_ns, old_atime)
            self.assertEqual(unit.stat().st_mtime_ns, mtime)

    def test_resolved_journal_tombstone_failpoints_are_idempotent_and_do_not_hide_new_stage(self) -> None:
        self._require_private_noatime_view()
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        for failure_env in ({"UAP_OBSERVER_RECOVERY_FAILPOINT": "after-resolved-journal-rename"},
                            {"UAP_OBSERVER_TOMBSTONE_DELETE_FAIL_AT": "1"},
                            {"UAP_OBSERVER_TOMBSTONE_DELETE_FAIL_AT": "3"},
                            {"UAP_OBSERVER_TOMBSTONE_DELETE_FAIL_AT": "10"}):
            with self.subTest(failure_env=failure_env), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary); stage = root / "stage"; closures = root / "closures"; systemd = root / "systemd"
                stage.mkdir(mode=0o700); closures.mkdir(); systemd.mkdir()
                subprocess.run(["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"',
                                "sh", str(helper), str(stage / "systemd-backup"), str(systemd)], check=True)
                closure, digest = self._create_hashed_closure(closures, helper)
                for name, value in (("closure-digest", digest + "\n"), ("journal-committed", "committed-v1\n")):
                    (stage / name).write_text(value); (stage / name).chmod(0o600)
                current = root / "current"; current.symlink_to(f"uap-observer-closures/{digest}")
                self._copy_observer_inventory(closure / "systemd", stage / "systemd")
                self._copy_observer_inventory(closure / "systemd", systemd)
                command = ["/bin/sh", "-c", '. "$1"; cleanup_fixture() { :; }; recover_observer_install "$2" "$3" "$4" "$5" /bin/true cleanup_fixture',
                           "sh", str(helper), str(stage), str(closures), str(current), str(systemd)]
                failed = subprocess.run(command, env={**os.environ, **failure_env}, text=True, capture_output=True)
                self.assertNotEqual(failed.returncode, 0)
                tombstone = Path(str(stage) + ".resolved-tombstone")
                self.assertFalse(stage.exists())
                self.assertTrue(tombstone.is_dir())
                self.assertTrue((tombstone / "journal-tombstone").is_file())
                # A legitimate new uncommitted journal may claim the original
                # name while retry finishes only the authenticated sibling.
                stage.mkdir(mode=0o700); (stage / "new-payload").write_text("new\n")
                retried = subprocess.run(command, text=True, capture_output=True)
                self.assertEqual(retried.returncode, 0, retried.stderr)
                self.assertFalse(tombstone.exists())
                self.assertFalse(stage.exists())

    def test_unfsynced_tombstone_rename_can_retry_from_original_stage_name(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            stage = Path(temporary) / "stage"; stage.mkdir(mode=0o700)
            payload = stage / "payload"; payload.write_text("resolved data\n"); payload.chmod(0o600)
            command = ["/bin/sh", "-c", '. "$1"; observer_tombstone_resolved_journal "$2"',
                       "sh", str(helper), str(stage)]
            subprocess.run(command, check=True)
            tombstone = Path(str(stage) + ".resolved-tombstone")
            tombstone.rename(stage)  # model rename loss before the parent fsync
            self.assertTrue((stage / "journal-tombstone").is_file())
            subprocess.run(command, check=True)
            subprocess.run(["/bin/sh", "-c", '. "$1"; observer_cleanup_resolved_tombstone "$2"',
                            "sh", str(helper), str(stage)], check=True)
            self.assertFalse(stage.exists()); self.assertFalse(tombstone.exists())

    def test_removed_journal_parent_fsync_failure_is_retryable(self) -> None:
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); stage = root / "stage"
            stage.mkdir(mode=0o700)
            log = root / "sync.log"
            script = r'''. "$1"
fixture_stage=$2
fixture_log=$6
cleanup_fixture() { :; }
observer_sync_directory() {
  printf '%s\n' "$1" >> "$fixture_log"
  if [ "${FAIL_PARENT_SYNC:-}" = 1 ] && [ "$1" = "$(dirname "$fixture_stage")" ] && [ ! -e "$fixture_stage" ]; then return 75; fi
  command python3 - "$1" <<'PY'
import os,sys
fd=os.open(sys.argv[1],os.O_RDONLY|os.O_DIRECTORY)
try: os.fsync(fd)
finally: os.close(fd)
PY
}
recover_observer_install "$2" "$3" "$4" "$5" /bin/true cleanup_fixture
'''
            args = ["/bin/sh", "-c", script, "sh", str(helper), str(stage),
                    str(root / "closures"), str(root / "current"), str(root / "systemd"), str(log)]
            failed = subprocess.run(args, env=dict(os.environ, FAIL_PARENT_SYNC="1"), text=True, capture_output=True)
            self.assertNotEqual(failed.returncode, 0)
            self.assertFalse(stage.exists())
            retried = subprocess.run(args, text=True, capture_output=True)
            self.assertEqual(retried.returncode, 0, retried.stderr)
            self.assertEqual(log.read_text().splitlines().count(str(root)), 2)

    def test_current_recovery_syncs_pointer_parent_before_journal_removal(self) -> None:
        self._require_private_noatime_view()
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); stage = root / "stage"; closures = root / "closures"; systemd = root / "systemd"
            stage.mkdir(mode=0o700); closures.mkdir(); systemd.mkdir()
            subprocess.run(["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"', "sh", str(helper), str(stage / "systemd-backup"), str(systemd)], check=True)
            closure, digest = self._create_hashed_closure(closures, helper)
            for name, value in (("closure-digest", digest + "\n"), ("journal-committed", "committed-v1\n")):
                (stage / name).write_text(value); (stage / name).chmod(0o600)
            current = root / "current"; current.symlink_to(f"uap-observer-closures/{digest}")
            self._copy_observer_inventory(closure / "systemd", stage / "systemd")
            self._copy_observer_inventory(closure / "systemd", systemd)
            log = root / "log"
            script = r'''. "$1"
fixture_log=$6
observer_sync_directory() { printf 'sync %s\n' "$1" >> "$fixture_log"; }
cleanup_fixture() { printf 'cleanup\n' >> "$fixture_log"; }
recover_observer_install "$2" "$3" "$4" "$5" /bin/true cleanup_fixture
'''
            result = subprocess.run(
                ["/bin/sh", "-c", script, "sh", str(helper), str(stage), str(closures), str(current), str(systemd), str(log)],
                text=True, capture_output=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            events = log.read_text().splitlines()
            self.assertEqual(events[0], f"sync {root}")
            self.assertIn("cleanup", events)
            stage_sync = events.index(f"sync {stage}")
            self.assertLess(events.index("cleanup"), stage_sync)
            self.assertLess(stage_sync, len(events) - 1)
            self.assertEqual(events[-1], f"sync {root}")
            self.assertFalse(Path(str(stage) + ".resolved-tombstone").exists())

    def test_recovery_retains_committed_journal_when_pointer_sync_or_partial_cleanup_fails(self) -> None:
        self._require_private_noatime_view()
        helper = Path(__file__).parents[2] / "deploy/uap-observer-install-lib.sh"
        for failure in ("missing-closure", "pointer-sync", "systemd-drift", "partial-cleanup", "tombstone-delete"):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary); stage = root / "stage"; closures = root / "closures"; systemd = root / "systemd"
                stage.mkdir(mode=0o700); closures.mkdir()
                if failure == "missing-closure":
                    self._populate_complete_observer_inventory(systemd)
                else:
                    systemd.mkdir()
                subprocess.run(["/bin/sh", "-c", '. "$1"; journal_observer_systemd "$2" "$3"', "sh", str(helper), str(stage / "systemd-backup"), str(systemd)], check=True)
                if failure == "missing-closure":
                    digest = "e" * 64
                else:
                    closure, digest = self._create_hashed_closure(closures, helper)
                for name, value in (("closure-digest", digest + "\n"), ("journal-committed", "committed-v1\n")):
                    (stage / name).write_text(value); (stage / name).chmod(0o600)
                current = root / "current"; current.symlink_to(f"uap-observer-closures/{digest}")
                if failure != "missing-closure":
                    self._copy_observer_inventory(closure / "systemd", stage / "systemd")
                    self._copy_observer_inventory(closure / "systemd", systemd)
                if failure == "systemd-drift":
                    (systemd / "uap-observer.service").write_text("drifted\n")
                script = r'''. "$1"
failure=$6
observer_sync_directory() { test "$failure" != pointer-sync; }
cleanup_fixture() { test "$failure" != partial-cleanup; }
if [ "$failure" = tombstone-delete ]; then UAP_OBSERVER_TOMBSTONE_DELETE_FAIL_AT=1; export UAP_OBSERVER_TOMBSTONE_DELETE_FAIL_AT; fi
recover_observer_install "$2" "$3" "$4" "$5" /bin/true cleanup_fixture
'''
                result = subprocess.run(
                    ["/bin/sh", "-c", script, "sh", str(helper), str(stage), str(closures), str(current), str(systemd), failure],
                    text=True, capture_output=True,
                )
                self.assertNotEqual(result.returncode, 0)
                if failure == "tombstone-delete":
                    tombstone = Path(str(stage) + ".resolved-tombstone")
                    self.assertTrue((tombstone / "journal-tombstone").is_file())
                else:
                    marker = "journal-resolved" if failure == "partial-cleanup" else "journal-committed"
                    self.assertTrue((stage / marker).is_file())
                if failure == "partial-cleanup":
                    for control in ("journal-resolved", "journal-committed", "closure-digest", "recovery-outcome"):
                        self.assertTrue((stage / control).is_file(), f"lost authenticated retry control {control}")
                if failure in {"partial-cleanup", "tombstone-delete"}:
                    retried = subprocess.run(
                        ["/bin/sh", "-c", '. "$1"; cleanup_fixture() { :; }; recover_observer_install "$2" "$3" "$4" "$5" /bin/true cleanup_fixture',
                         "sh", str(helper), str(stage), str(closures), str(current), str(systemd)],
                        text=True, capture_output=True,
                    )
                    self.assertEqual(retried.returncode, 0, retried.stderr)
                    self.assertFalse(stage.exists())
                    self.assertFalse(Path(str(stage) + ".resolved-tombstone").exists())

    def test_cgroup_v2_path_stays_beneath_mount_and_kills_descendants_after_leader_exit(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            cgroup_root = root / "cgroup"
            parent = cgroup_root / "delegated" / "runner"
            parent.mkdir(parents=True)
            (cgroup_root / "cgroup.controllers").write_text("pids\n")
            identity = root / "self.cgroup"
            identity.write_text("0::/delegated/runner\n")
            target = fixed_runner.delegated_job_cgroup(cgroup_root, identity)
            self.assertEqual(target.parent, parent)
            calls: list[str] = []
            states = iter(("populated 1\n", "populated 0\n"))
            def remove() -> None:
                calls.append("rmdir")
                target.rmdir()
            fixed_runner.destroy_job_cgroup(
                target, kill=lambda: calls.append("cgroup.kill"),
                events=lambda: next(states), remove=remove,
            )
            self.assertEqual(calls, ["cgroup.kill", "rmdir"])

    def test_stuck_sigkill_wait_terminates_runner_with_a_hard_bound(self) -> None:
        class Stuck:
            pid = 99999999
            def poll(self): return None
            def wait(self, *, timeout=None):
                self.timeout = timeout
                raise fixed_runner.subprocess.TimeoutExpired("fixture", timeout)
        process = Stuck()
        exits: list[int] = []
        with self.assertRaisesRegex(RuntimeError, "fatal runner cleanup"):
            fixed_runner.kill_process_group(process, wait_seconds=0.01, fatal=exits.append)  # type: ignore[arg-type]
        self.assertEqual(process.timeout, 0.01)
        self.assertEqual(exits, [70])

    def test_nonempty_cgroup_terminates_runner_without_removal(self) -> None:
        removed: list[str] = []
        exits: list[int] = []
        with self.assertRaisesRegex(RuntimeError, "fatal runner cleanup"):
            fixed_runner.destroy_job_cgroup(
                Path("/fixture"), kill=lambda: None, events=lambda: "populated 1\n",
                remove=lambda: removed.append("removed"), wait_seconds=0, fatal=exits.append,
            )
        self.assertEqual(exits, [70])
        self.assertEqual(removed, [])

    def test_adapter_stuck_sigkill_wait_is_hard_bounded(self) -> None:
        class Stuck:
            pid = 99999999
            def poll(self): return None
            def wait(self, *, timeout=None):
                self.timeout = timeout
                raise fixed_adapters.subprocess.TimeoutExpired("fixture", timeout)
        process = Stuck()
        exits: list[int] = []
        with self.assertRaisesRegex(RuntimeError, "fatal adapter cleanup"):
            fixed_adapters.terminate_group(process, wait_seconds=0.01, fatal=exits.append)  # type: ignore[arg-type]
        self.assertEqual(process.timeout, 0.01)
        self.assertEqual(exits, [70])

    def test_root_challenge_record_is_reserved_committed_and_rollback_retryable(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            pending, reserved, consumed = root / "pending", root / "reserved", root / "consumed"
            pending.mkdir(mode=0o700)
            reserved.mkdir(mode=0o700)
            consumed.mkdir(mode=0o700)
            challenge = "a" * 64
            record = pending / f"{challenge}.json"
            record.write_text("{}")
            record.chmod(0o640)
            fixed_runner.transition_record(root, challenge, "reserve")
            self.assertFalse(record.exists())
            self.assertTrue((reserved / record.name).is_file())
            fixed_runner.transition_record(root, challenge, "rollback")
            self.assertTrue(record.is_file())
            fixed_runner.transition_record(root, challenge, "reserve")
            fixed_runner.transition_record(root, challenge, "commit")
            self.assertTrue((consumed / record.name).is_file())
            fixed_runner.transition_record(root, challenge, "commit")

    def test_client_artifacts_are_split_across_isolated_identities(self) -> None:
        self.assertEqual(fixed_runner.ARTIFACT_IDENTITIES["runtime-attestations.json"], ("codex", "cursor", "kiro"))
        self.assertEqual(fixed_runner.ARTIFACT_IDENTITIES["notion-oauth-attestations.json"], ("codex", "cursor", "kiro"))
        self.assertEqual(fixed_runner.ARTIFACT_IDENTITIES["chatgpt-cloudflare-attestation.json"], ("control",))
        self.assertEqual(len({identity for values in fixed_runner.ARTIFACT_IDENTITIES.values() for identity in values}), 4)

    def test_service_accounts_reject_all_uid_and_group_aliases(self) -> None:
        role_values = list(fixed_runner.SERVICE_ROLES.values())
        group_names = sorted({name for _, primary, _, supplemental in role_values for name in (primary, *supplemental)})
        group_ids = {name: 2000 + index for index, name in enumerate(group_names)}
        supplemental_members = {
            "uap-observer-adapter-config": [
                "uap-observer-codex", "uap-observer-cursor",
                "uap-observer-kiro", "uap-observer-control",
            ],
            "uap-observer-signer-ipc": ["uap-observer"],
            "uap-observer-runner-ipc": ["uap-observer"],
        }
        groups = {
            name: fixed_runner.grp.struct_group((name, "x", gid, supplemental_members.get(name, [])))
            for name, gid in group_ids.items()
        }
        users = {}
        memberships = {}
        for index, (_, (name, primary, home, supplemental)) in enumerate(fixed_runner.SERVICE_ROLES.items()):
            user = fixed_runner.pwd.struct_passwd((
                name, "x", 1000 + index, group_ids[primary], "", home, "/usr/sbin/nologin",
            ))
            users[name] = user
            memberships[name] = [group_ids[item] for item in (primary, *supplemental)]

        def validate(user_values=None, group_values=None) -> None:  # type: ignore[no-untyped-def]
            user_values = list(users.values()) if user_values is None else user_values
            group_values = list(groups.values()) if group_values is None else group_values
            by_uid = {item.pw_uid: item for item in users.values()}
            by_gid = {item.gr_gid: item for item in groups.values()}
            with (
                mock.patch.object(fixed_runner.pwd, "getpwnam", side_effect=users.__getitem__),
                mock.patch.object(fixed_runner.pwd, "getpwuid", side_effect=by_uid.__getitem__),
                mock.patch.object(fixed_runner.pwd, "getpwall", return_value=user_values),
                mock.patch.object(fixed_runner.grp, "getgrnam", side_effect=groups.__getitem__),
                mock.patch.object(fixed_runner.grp, "getgrgid", side_effect=by_gid.__getitem__),
                mock.patch.object(fixed_runner.grp, "getgrall", return_value=group_values),
                mock.patch.object(fixed_runner.os, "getgrouplist", side_effect=lambda name, _gid: memberships[name]),
            ):
                fixed_runner.reviewed_service_identities()

        validate()
        caddy = users["caddy"]
        observer = users["uap-observer"]
        shared_caddy = fixed_runner.pwd.struct_passwd((
            caddy.pw_name, caddy.pw_passwd, observer.pw_uid, caddy.pw_gid,
            caddy.pw_gecos, caddy.pw_dir, caddy.pw_shell,
        ))
        users["caddy"] = shared_caddy
        with self.assertRaisesRegex(ValueError, "alias|distinct|canonical"):
            validate()
        users["caddy"] = caddy
        alias = fixed_runner.pwd.struct_passwd((
            "unrelated-alias", "x", users["uap-observer-codex"].pw_uid,
            65500, "", "/nonexistent", "/bin/false",
        ))
        with self.assertRaisesRegex(ValueError, "alias"):
            validate(user_values=[*users.values(), alias])
        canonical_group = groups["uap-observer-codex"]
        group_alias = fixed_runner.grp.struct_group((
            "unrelated-group-alias", "x", canonical_group.gr_gid, [],
        ))
        with self.assertRaisesRegex(ValueError, "alias"):
            validate(group_values=[*groups.values(), group_alias])
        config_group = groups["uap-observer-adapter-config"]
        groups["uap-observer-adapter-config"] = fixed_runner.grp.struct_group((
            config_group.gr_name, config_group.gr_passwd, config_group.gr_gid,
            [*config_group.gr_mem, "unrelated-user"],
        ))
        with self.assertRaisesRegex(ValueError, "membership"):
            validate()
        groups["uap-observer-adapter-config"] = config_group
        for primary in ("caddy", "uap-observer", "uap-observer-codex"):
            with self.subTest(primary_group=primary):
                original = groups[primary]
                groups[primary] = fixed_runner.grp.struct_group((
                    original.gr_name, original.gr_passwd, original.gr_gid,
                    ["unexpected-supplemental-member"],
                ))
                with self.assertRaisesRegex(ValueError, "membership"):
                    validate()
                groups[primary] = original

    def test_adapter_input_access_is_checked_for_every_identity_on_reinstall(self) -> None:
        if os.geteuid() != 0:
            self.skipTest("root ownership fixture requires root")
        with tempfile.TemporaryDirectory() as temporary:
            protected = Path(temporary) / "uap-observer-inputs"
            (protected / "bin").mkdir(parents=True)
            (protected / "chatgpt").mkdir()
            (protected / "cursor").mkdir()
            (protected / "chrome-for-testing").mkdir()
            files = {
                protected / "bin/git": b"git",
                protected / "bin/codex": b"codex",
                protected / "bin/codex-code-mode-host": b"codex-host",
                protected / "bin/kiro": b"kiro",
                protected / "bin/kiro-cli-chat": b"kiro-chat",
                protected / "chatgpt/app-binding.json": b"app",
                protected / "chatgpt/projection-receipt.json": b"receipt",
                protected / "external-pr-evidence.json": b"evidence",
                protected / "cursor/cursor-agent": b"cursor",
                protected / "cursor/index.js": b"index",
                protected / "cursor/node": b"node",
                protected / "cursor/bash": b"bash",
                protected / "cursor/basename": b"basename",
                protected / "cursor/dirname": b"dirname",
                protected / "cursor/realpath": b"realpath",
                protected / "chrome-for-testing/chrome": b"chrome",
                protected / "chrome-for-testing/resources.pak": b"resources",
            }
            for path, payload in files.items():
                path.write_bytes(payload)
                executable = (
                    path.parent == protected / "bin"
                    or path.name in {"cursor-agent", "node", "bash", "basename", "dirname", "realpath"}
                    or path == protected / "chrome-for-testing/chrome"
                )
                bundled = path.parent in {protected / "cursor", protected / "chrome-for-testing"}
                path.chmod(0o755 if executable else (0o644 if bundled else 0o640))
                os.chown(path, 0, 0)
            for directory in (protected, protected / "bin", protected / "chatgpt", protected / "cursor", protected / "chrome-for-testing"):
                directory.chmod(0o755 if directory in {protected / "cursor", protected / "chrome-for-testing"} else 0o711)
                os.chown(directory, 0, 0)
            bundle_manifest = protected / "cursor-bundle.json"
            bundle_manifest.write_bytes(client_bundle.canonical_json(client_bundle.inventory_bundle(protected / "cursor")))
            bundle_manifest.chmod(0o644)
            os.chown(bundle_manifest, 0, 0)
            chrome_manifest = protected / "chrome-for-testing-bundle.json"
            chrome_manifest.write_bytes(client_bundle.canonical_json(client_bundle.inventory_bundle(protected / "chrome-for-testing")))
            chrome_manifest.chmod(0o644)
            os.chown(chrome_manifest, 0, 0)
            digest = lambda path: "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()
            config = Path(temporary) / "config.json"
            config.write_text(json.dumps({
                "git": {"binary": str(protected / "bin/git"), "sha256": digest(protected / "bin/git")},
                "clients": {
                    "codex": {
                        "binary": str(protected / "bin/codex"),
                        "sha256": digest(protected / "bin/codex"),
                        "companion_binary": str(protected / "bin/codex-code-mode-host"),
                        "companion_sha256": digest(protected / "bin/codex-code-mode-host"),
                    },
                    "cursor": {
                        "binary": str(protected / "cursor/cursor-agent"),
                        "sha256": digest(protected / "cursor/cursor-agent"),
                        "bundle": {
                            "root": str(protected / "cursor"),
                            "manifest": str(bundle_manifest),
                            "manifest_sha256": digest(bundle_manifest),
                        },
                    },
                    "kiro": {"binary": str(protected / "bin/kiro"), "sha256": digest(protected / "bin/kiro")},
                },
                "chrome_for_testing": {
                    "root": str(protected / "chrome-for-testing"),
                    "manifest": str(chrome_manifest),
                    "manifest_sha256": digest(chrome_manifest),
                    "binary": str(protected / "chrome-for-testing/chrome"),
                    "binary_sha256": digest(protected / "chrome-for-testing/chrome"),
                    "version": "152.0.7977.64",
                },
                "chatgpt": {
                    "app_binding_path": str(protected / "chatgpt/app-binding.json"),
                    "app_binding_sha256": digest(protected / "chatgpt/app-binding.json"),
                    "projection_receipt_path": str(protected / "chatgpt/projection-receipt.json"),
                    "projection_receipt_sha256": digest(protected / "chatgpt/projection-receipt.json"),
                },
                "external_pr_evidence": {
                    "path": str(protected / "external-pr-evidence.json"),
                    "sha256": digest(protected / "external-pr-evidence.json"),
                },
            }))
            value = json.loads(config.read_text())
            value["clients"]["kiro"].update({
                "companion_binary": str(protected / "bin/kiro-cli-chat"),
                "companion_sha256": digest(protected / "bin/kiro-cli-chat"),
            })
            config.write_text(json.dumps(value))
            identities = {
                name: (3000 + index, 4000 + index, frozenset({0, 4000 + index}))
                for index, name in enumerate(("codex", "cursor", "kiro", "control"))
            }
            config_group = fixed_runner.grp.struct_group(("uap-observer-adapter-config", "x", 0, []))
            with mock.patch.object(fixed_runner.grp, "getgrnam", return_value=config_group):
                probes: list[tuple[int, int, frozenset[int], tuple[tuple[Path, bool], ...]]] = []
                self.assertEqual(len(fixed_runner.validate_adapter_input_access(
                    config, protected_root=protected, identities=identities,
                    access_probe=lambda uid, gid, gids, paths: probes.append((uid, gid, gids, paths)),
                )), 12)
                self.assertEqual(
                    [(uid, gid, gids) for uid, gid, gids, _ in probes],
                    list(identities.values()),
                )
                self.assertTrue(all(len(paths) == 19 for _, _, _, paths in probes))
                for excluded, (denied_uid, _, _) in identities.items():
                    with self.subTest(excluded=excluded):
                        def deny(uid: int, _gid: int, _gids: frozenset[int], _paths: tuple[tuple[Path, bool], ...]) -> None:
                            if uid == denied_uid:
                                raise ValueError("kernel access denied")
                        with self.assertRaisesRegex(ValueError, "kernel access denied"):
                            fixed_runner.validate_adapter_input_access(
                                config, protected_root=protected, identities=identities,
                                access_probe=deny,
                            )
                cursor_index = protected / "cursor/index.js"
                cursor_index.write_bytes(b"tampered")
                with self.assertRaisesRegex(ValueError, "bundle bytes differ"):
                    fixed_runner.validate_adapter_input_access(
                        config, protected_root=protected, identities=identities,
                        access_probe=lambda *_args: None,
                    )
                cursor_index.write_bytes(files[cursor_index])
                unexpected = protected / "cursor/unexpected.js"
                unexpected.write_bytes(b"unexpected")
                unexpected.chmod(0o644)
                os.chown(unexpected, 0, 0)
                with self.assertRaisesRegex(ValueError, "bundle bytes differ"):
                    fixed_runner.validate_adapter_input_access(
                        config, protected_root=protected, identities=identities,
                        access_probe=lambda *_args: None,
                    )
                unexpected.unlink()
                # Search permission is sufficient: the probe must not require
                # directory listing permission from an adapter identity.
                # A production input root has searchable ancestors.  The
                # root-host orchestrator deliberately gives TMPDIR itself
                # mode 0700, so model that production property on every
                # disposable ancestor below /tmp and put the fixture modes
                # back even when the exact-identity child fails.
                fixture_root = Path(temporary).resolve()
                global_tmp = Path("/tmp").resolve()
                searchable = []
                ancestor = fixture_root
                while ancestor != global_tmp:
                    if global_tmp not in ancestor.parents:
                        self.fail(f"disposable fixture escaped /tmp: {ancestor}")
                    searchable.append((ancestor, stat.S_IMODE(ancestor.stat().st_mode)))
                    ancestor = ancestor.parent
                for ancestor, _mode in reversed(searchable):
                    ancestor.chmod(0o711)
                try:
                    fixed_runner.validate_adapter_input_access(
                        config, protected_root=protected, identities=identities,
                    )
                except ValueError as error:
                    if "Operation not permitted" in str(error):
                        self.skipTest("provider sandbox blocks exact setuid/setgroups access probe")
                    raise
                finally:
                    for ancestor, mode in searchable:
                        ancestor.chmod(mode)

    def test_identity_probe_directory_flags_use_opath_with_readonly_fallback(self) -> None:
        common = os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC
        synthetic_o_path = 0x40000000
        self.assertEqual(
            fixed_runner._identity_directory_open_flags(synthetic_o_path),
            synthetic_o_path | common,
        )
        fallback = fixed_runner._identity_directory_open_flags(0)
        self.assertEqual(fallback, os.O_RDONLY | common)
        self.assertFalse(fallback & synthetic_o_path)

    def test_fixed_runner_socket_executes_only_injected_reviewed_adapters(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            fixture_artifacts = artifacts()
            config = root / "adapter-config.json"
            config.write_bytes(b"{}")
            config.chmod(0o640)
            config_digest = "sha256:" + hashlib.sha256(config.read_bytes()).hexdigest()
            adapters = []
            for index, artifact_name in enumerate(fixed_runner.ARTIFACT_ORDER):
                artifact = fixture_artifacts[artifact_name]
                executable = root / f"adapter-{index}.py"
                executable.write_text(
                    "#!/usr/bin/env python3\n"
                    "import argparse,json,os\n"
                    "p=argparse.ArgumentParser(); p.add_argument('--artifact'); p.add_argument('--context'); p.add_argument('--output'); p.add_argument('--config'); p.add_argument('--config-sha256'); a=p.parse_args()\n"
                    f"value={artifact!r}\n"
                    "identity=os.environ.get('UAP_OBSERVER_ADAPTER_CLIENT')\n"
                    "if identity != 'control' and isinstance(value.get('attestations'),list): value={**value,'attestations':[item for item in value['attestations'] if item.get('client') == identity]}\n"
                    "json.dump(value,open(a.output,'x'))\n"
                )
                executable.chmod(0o755)
                adapters.append(Adapter(artifact_name, executable, "sha256:" + hashlib.sha256(executable.read_bytes()).hexdigest(), config, config_digest))
            try:
                listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            except PermissionError:
                self.skipTest("provider sandbox blocks AF_UNIX socket creation")
            socket_path = root / "runner.sock"
            listener.bind(str(socket_path))
            listener.listen(1)
            runner = ReviewedRunner(tuple(adapters), root / "state", protected=False)
            thread = threading.Thread(target=serve_runner, args=(listener, runner, os.geteuid()), kwargs={"once": True})
            thread.start()
            source = Path(__file__).parents[1] / "fixed_runner.py"
            client = SocketRunner(
                socket_path, source, "sha256:" + hashlib.sha256(source.read_bytes()).hexdigest(),
                60, enforce_root_ownership=False,
            )
            value = client.run(root / "observer-state-forbidden", {"request": {}, "github_attestation": {}})
            thread.join()
            listener.close()
            validate_artifact_schemas(value, challenge="a" * 64)
            self.assertEqual(value["consent.json"], fixture_artifacts["consent.json"])

    def test_runner_timeout_kills_the_entire_adapter_process_group(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            marker = root / "escaped-child"
            executable = root / "forking-adapter.py"
            executable.write_text(
                "#!/usr/bin/env python3\n"
                "import os,time\n"
                "if os.fork() == 0:\n"
                " time.sleep(0.5)\n"
                f" open({str(marker)!r},'w').write('escaped')\n"
                " os._exit(0)\n"
                "time.sleep(10)\n"
            )
            executable.chmod(0o755)
            digest = "sha256:" + hashlib.sha256(executable.read_bytes()).hexdigest()
            config = root / "adapter-config.json"
            config.write_bytes(b"{}")
            config.chmod(0o640)
            config_digest = "sha256:" + hashlib.sha256(config.read_bytes()).hexdigest()
            adapters = tuple(Adapter(name, executable, digest, config, config_digest) for name in fixed_runner.ARTIFACT_ORDER)
            runner = ReviewedRunner(adapters, root / "state", protected=False)
            original_timeout = fixed_runner.RUNNER_TOTAL_SECONDS
            fixed_runner.RUNNER_TOTAL_SECONDS = 0.15
            try:
                with self.assertRaisesRegex(TimeoutError, "deadline"):
                    runner.execute({"request": {}, "github_attestation": {}})
                time.sleep(0.7)
                self.assertFalse(marker.exists())
            finally:
                fixed_runner.RUNNER_TOTAL_SECONDS = original_timeout

    def test_manifest_requires_four_hardlinks_and_pins_config_digest(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            executable = root / "adapter-runtime"
            executable.write_text("#!/bin/sh\nexit 1\n")
            executable.chmod(0o755)
            config = root / "config.json"
            config.write_bytes(b"{}")
            config.chmod(0o640)
            digest = "sha256:" + hashlib.sha256(executable.read_bytes()).hexdigest()
            config_digest = "sha256:" + hashlib.sha256(config.read_bytes()).hexdigest()
            paths = {}
            for index, artifact in enumerate(fixed_runner.ARTIFACT_ORDER):
                path = executable if index == 0 else root / f"adapter-{index}"
                if index:
                    os.link(executable, path)
                paths[artifact] = {"path": str(path), "sha256": digest}
            manifest = root / "manifest.json"
            manifest.write_bytes(canonical_json({
                "schema_version": 1, "config": {"path": str(config), "sha256": config_digest},
                "artifacts": paths,
            }))
            manifest.chmod(0o640)
            loaded = fixed_runner.load_adapters(manifest)
            self.assertEqual(tuple(item.artifact for item in loaded), fixed_runner.ARTIFACT_ORDER)
            os.unlink(paths[fixed_runner.ARTIFACT_ORDER[-1]]["path"])
            replacement = Path(paths[fixed_runner.ARTIFACT_ORDER[-1]]["path"])
            replacement.write_bytes(executable.read_bytes())
            replacement.chmod(0o755)
            with self.assertRaisesRegex(ValueError, "hardlinks"):
                fixed_runner.load_adapters(manifest)


class FixedAdapterContractTests(unittest.TestCase):
    @staticmethod
    def install_projection(profile: Path, client: str, approved: dict[str, Any]) -> dict[str, str]:
        proof = profile.parent / "proofs" / client
        proof.mkdir(parents=True, mode=0o700, exist_ok=True)
        projection = proof / "native-projection.json"
        native_proof = proof / "native"
        native_proof.mkdir(mode=0o700)
        entries = []
        for plugin in sorted(fixed_adapters.HEROES):
            native = (
                profile / "skills" / "code-tool-router" / "SKILL.md"
                if plugin == "agent-code-navigator" else profile / f"{plugin}.native"
            )
            native.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
            native.write_text(
                "# code-tool-router\n" if plugin == "agent-code-navigator"
                else json.dumps({"plugin": plugin})
            )
            native.chmod(0o440)
            protected_native = native_proof / f"{plugin}.blob"
            protected_native.write_bytes(native.read_bytes())
            protected_native.chmod(0o440)
            entries.append({
                "plugin": plugin,
                "component_kind": "skill" if plugin == "agent-code-navigator" else "mcp",
                "tuple": approved if plugin == "context7" else {**approved, "product_id": plugin},
                "native_config": {"path": str(protected_native), "sha256": fixed_adapters.sha256(native.read_bytes())},
                "client_config": {"path": str(native), "sha256": fixed_adapters.sha256(native.read_bytes())},
                "manager_add_sha256": "sha256:" + "9" * 64,
                "manager_info_sha256": "sha256:" + "a" * 64,
                "post_add_doctor_sha256": "sha256:" + "b" * 64,
            })
        projection.write_bytes(fixed_adapters.canonical_json({
            "schema_version": 2, "client_id": client,
            "entries": entries,
        }))
        projection.chmod(0o440)
        receipts = proof / "receipts.json"
        receipts.write_text(json.dumps({"schema_version": 1, "receipts": [{
            "name": entry["plugin"], "tuple": entry["tuple"],
            "manager_add_sha256": entry["manager_add_sha256"],
            "manager_info_sha256": entry["manager_info_sha256"],
            "post_add_doctor_sha256": entry["post_add_doctor_sha256"],
        } for entry in entries]}))
        receipts.chmod(0o440)
        native_proof.chmod(0o510)
        proof.chmod(0o510)
        profile.chmod(0o510)
        return {"path": str(projection), "sha256": fixed_adapters.sha256(projection.read_bytes())}

    def test_full_sealed_tuple_constructs_runtime_tuple_and_partial_extra_bool_fail(self) -> None:
        approved = sealed_tuple("context7")
        fixed_adapters.validate_release_tuple(approved, "context7")
        completed = fixed_adapters.complete_tuple(
            {"plugin": "context7", "tuple": approved},
            {"client_version": "fixture-client-1"}, "2026-08-26T01:02:03Z",
        )
        self.assertEqual(completed["client_version"], "fixture-client-1")
        fixed_adapters.validate_release_tuple(completed, "context7", sealed=False)
        malformed = (
            {key: value for key, value in approved.items() if key != "binary_digest"},
            {**approved, "unexpected": 1},
            {**approved, "release_sequence": True},
            {**approved, "snapshot_sequence": False},
            {**approved, "client_version": "fabricated"},
        )
        for value in malformed:
            with self.subTest(tuple=value), self.assertRaises(ValueError):
                fixed_adapters.validate_release_tuple(value, "context7")

    def test_verified_git_requires_exact_immutable_schema_path(self) -> None:
        digest = "sha256:" + "a" * 64
        item = {"binary": "/opt/uap-observer-inputs/bin/git", "sha256": digest}
        with mock.patch.object(fixed_adapters, "verify_executable_file") as verify:
            self.assertEqual(
                fixed_adapters.verified_git(item, owner_uid=1234),
                Path("/opt/uap-observer-inputs/bin/git"),
            )
            verify.assert_called_once_with(
                Path("/opt/uap-observer-inputs/bin/git"), digest, owner_uid=1234,
            )

        item["binary"] = "/usr/bin/git"
        with mock.patch.object(fixed_adapters, "verify_executable_file") as verify:
            with self.assertRaisesRegex(ValueError, "fixed Git executable differs"):
                fixed_adapters.verified_git(item, owner_uid=1234)
            verify.assert_not_called()

    def test_fixed_node_runtime_requires_the_verified_cursor_bundle(self) -> None:
        bundle = {
            "root": "/opt/uap-observer-inputs/cursor",
            "manifest": "/opt/uap-observer-inputs/cursor-bundle.json",
            "manifest_sha256": "sha256:" + "a" * 64,
        }
        executable = mock.Mock(st_mode=stat.S_IFREG | 0o755)
        with (
            mock.patch.object(
                fixed_adapters, "verify_bundle", return_value=fixed_adapters.CURSOR_RUNTIME_MEMBERS,
            ) as verify,
            mock.patch.object(fixed_adapters.os, "lstat", return_value=executable),
        ):
            self.assertEqual(
                fixed_adapters.verified_runtime_node(bundle, owner_uid=1234),
                fixed_adapters.NODE_BINARY,
            )
            verify.assert_called_once_with(
                root=Path(bundle["root"]), manifest=Path(bundle["manifest"]),
                manifest_sha256=bundle["manifest_sha256"], owner_uid=1234,
            )

        with mock.patch.object(fixed_adapters, "verify_bundle", return_value=set()):
            with self.assertRaisesRegex(ValueError, "runtime closure is absent"):
                fixed_adapters.verified_runtime_node(bundle, owner_uid=1234)
        with self.assertRaisesRegex(ValueError, "bundle config differs"):
            fixed_adapters.verified_runtime_node({**bundle, "root": "/tmp/cursor"}, owner_uid=1234)

    def test_fixed_browser_runtime_requires_the_verified_complete_bundle(self) -> None:
        bundle = {
            "root": str(fixed_adapters.CHROME_ROOT),
            "manifest": str(fixed_adapters.CHROME_MANIFEST),
            "manifest_sha256": "sha256:" + "a" * 64,
            "binary": str(fixed_adapters.CHROME_BINARY),
            "binary_sha256": "sha256:" + "b" * 64,
            "version": fixed_adapters.CHROME_VERSION,
        }
        with (
            mock.patch.object(
                fixed_adapters, "verify_bundle", return_value={fixed_adapters.CHROME_BINARY},
            ) as verify_bundle,
            mock.patch.object(fixed_adapters, "verify_executable_file") as verify_binary,
        ):
            self.assertEqual(
                fixed_adapters.verified_runtime_browser(bundle, owner_uid=1234),
                fixed_adapters.CHROME_BINARY,
            )
            verify_bundle.assert_called_once_with(
                root=fixed_adapters.CHROME_ROOT,
                manifest=fixed_adapters.CHROME_MANIFEST,
                manifest_sha256=bundle["manifest_sha256"], owner_uid=1234,
            )
            verify_binary.assert_called_once_with(
                fixed_adapters.CHROME_BINARY, bundle["binary_sha256"], owner_uid=1234,
            )
        with mock.patch.object(fixed_adapters, "verify_bundle", return_value=set()):
            with self.assertRaisesRegex(ValueError, "absent from its verified bundle"):
                fixed_adapters.verified_runtime_browser(bundle, owner_uid=1234)
        with self.assertRaisesRegex(ValueError, "bundle config differs"):
            fixed_adapters.verified_runtime_browser({**bundle, "version": "152.0.0.0"}, owner_uid=1234)

    def test_chrome_runtime_config_requires_exact_headless_closure(self) -> None:
        args = ["/profile/launcher.mjs", "--no-usage-statistics", *fixed_adapters.CHROME_RUNTIME_ARGUMENTS]
        encoded = fixed_adapters.canonical_json({
            "mcpServers": {"chrome-devtools": {"command": "node", "args": args}},
        })
        fixed_adapters.validate_chrome_runtime_config(encoded)
        for rejected in (
            args[:-1],
            [args[0], "--browserUrl=http://127.0.0.1:9222", *fixed_adapters.CHROME_RUNTIME_ARGUMENTS],
            [*args, fixed_adapters.CHROME_RUNTIME_ARGUMENTS[-1]],
        ):
            with self.subTest(rejected=rejected), self.assertRaises(ValueError):
                fixed_adapters.validate_chrome_runtime_config(fixed_adapters.canonical_json({
                    "mcpServers": {"chrome-devtools": {"command": "node", "args": rejected}},
                }))

    def test_node_runtime_environment_is_proxy_aware_and_path_bounded(self) -> None:
        profile = Path("/var/lib/uap-observer/profiles/cursor")
        plain = fixed_adapters.runtime_environment(profile)
        self.assertEqual(plain["PATH"], str(fixed_adapters.GIT_BINARY.parent))
        self.assertNotIn("NODE_USE_ENV_PROXY", plain)

        node = fixed_adapters.runtime_environment(profile, fixed_adapters.NODE_BINARY)
        self.assertEqual(
            node["PATH"],
            os.pathsep.join((
                str(fixed_adapters.GIT_BINARY.parent),
                str(fixed_adapters.NODE_BINARY.parent),
            )),
        )
        self.assertEqual(node["NODE_USE_ENV_PROXY"], "1")
        self.assertEqual(node["HTTPS_PROXY"], fixed_adapters.FIXED_HTTPS_PROXY)

    def test_chrome_devtools_refuses_unverified_node_before_profile_access(self) -> None:
        with (
            mock.patch.object(
                fixed_adapters, "verified_executable",
                return_value=Path("/opt/uap-observer-inputs/cursor/cursor-agent"),
            ),
            self.assertRaisesRegex(ValueError, "verified fixed Node and browser runtimes"),
        ):
            fixed_adapters.invoke(
                {"profile": "/tmp"}, "chrome-devtools", "cursor", "a" * 64,
                Path("/tmp"), 1234,
            )

    def test_historical_external_pr_capture_is_reusable_for_same_bound_state(self) -> None:
        request = Fixture(Path(tempfile.mkdtemp())).request()
        evidence = artifacts(request["challenge"]["value"])["runtime-attestations.json"]["external_pr_evidence"]
        self.assertTrue(fixed_adapters.historical_external_pr_evidence_matches_request(evidence, request))
        rerun = json.loads(json.dumps(request))
        rerun["challenge"]["value"] = "f" * 64
        self.assertTrue(fixed_adapters.historical_external_pr_evidence_matches_request(evidence, rerun))
        fresh_challenge = "f" * 64
        fresh_artifacts = artifacts(fresh_challenge)
        fresh_artifacts["runtime-attestations.json"]["external_pr_evidence"] = evidence
        expected = Fixture(Path(tempfile.mkdtemp())).request()
        expected["challenge"]["value"] = fresh_challenge
        validate_artifact_schemas(
            fresh_artifacts, challenge=fresh_challenge,
            scenario_contract_digest=expected["scenario_contract_digest"],
            expected_bindings=expected,
        )
        for field, value in (
            ("cli_release_tag", "agentplugins-v0.1.13"),
            ("release_manifest_digest", "sha256:" + "0" * 64),
            ("directory_digest", "sha256:" + "0" * 64),
        ):
            rejected = json.loads(json.dumps(request))
            rejected[field] = value
            self.assertFalse(fixed_adapters.historical_external_pr_evidence_matches_request(evidence, rejected))
        rebound = json.loads(json.dumps(evidence))
        rebound["binding"]["catalog_sha"] = "0" * 40
        self.assertFalse(fixed_adapters.historical_external_pr_evidence_matches_request(rebound, request))
        foreign_head = json.loads(json.dumps(request))
        foreign_head["github"]["sha"] = "0" * 40
        self.assertFalse(fixed_adapters.historical_external_pr_evidence_matches_request(evidence, foreign_head))

    def test_per_client_binary_mapping_cannot_be_swapped_by_configuration(self) -> None:
        clients = {
            client: {
                "binary": f"/opt/uap-observer-inputs/bin/{client}",
                "sha256": "sha256:" + "a" * 64,
                "profile": f"/var/lib/uap-observer/profiles/{client}",
                "client_id": client,
                "native_projection": {"path": f"/var/lib/uap-observer/proofs/{client}/native-projection.json", "sha256": "sha256:" + "b" * 64},
            }
            for client in ("codex", "cursor", "kiro")
        }
        clients["codex"]["binary"], clients["cursor"]["binary"] = clients["cursor"]["binary"], clients["codex"]["binary"]
        config = {
            "schema_version": 1, "request_policy": {}, "git": {}, "clients": clients,
            "matrix": [], "consent_record": {}, "chatgpt": {},
            "chrome_for_testing": {
                "root": str(fixed_adapters.CHROME_ROOT),
                "manifest": str(fixed_adapters.CHROME_MANIFEST),
                "manifest_sha256": "sha256:" + "c" * 64,
                "binary": str(fixed_adapters.CHROME_BINARY),
                "binary_sha256": "sha256:" + "d" * 64,
                "version": fixed_adapters.CHROME_VERSION,
            },
            "workspace_root": "/var/lib/uap-observer/workspaces", "external_pr_evidence": {},
            "egress_hosts": ["api.github.com"],
        }
        with self.assertRaisesRegex(ValueError, "literal dedicated path"):
            fixed_adapters.validate_config(config)

    def test_mount_namespace_is_a_kernel_verified_positive_allowlist(self) -> None:
        root = "10 1 0:1 / / ro - tmpfs tmpfs ro\n"
        allowed = root + "11 10 8:1 /usr/bin /usr/bin ro - ext4 /dev/root ro\n"
        fixed_adapters.verify_positive_mount_namespace(allowed)
        fixed_adapters.verify_positive_mount_namespace(
            allowed + "12 10 8:2 /opt/uap-observer-inputs/cursor /opt/uap-observer-inputs/cursor ro - ext4 /dev/root ro\n",
        )
        for target in ("/var/www/customer-project", "/usr/local/src/repository", "/workspace/project", "/var/www/link-to-project", "/var/lib/uap-observer/state"):
            with self.subTest(target=target), self.assertRaisesRegex(ValueError, "non-allowlisted"):
                fixed_adapters.verify_positive_mount_namespace(allowed + f"12 10 8:2 /project {target} ro - ext4 /dev/fixture ro\n")
        with self.assertRaisesRegex(ValueError, "non-allowlisted"):
            fixed_adapters.verify_positive_mount_namespace(allowed + "12 10 0:9 / /var/www ro - tmpfs tmpfs ro\n")
        with self.assertRaisesRegex(ValueError, "non-allowlisted"):
            fixed_adapters.verify_positive_mount_namespace(
                allowed + "12 10 8:2 /opt/uap-observer-inputs/unexpected /opt/uap-observer-inputs/unexpected ro - ext4 /dev/fixture ro\n",
            )
        with self.assertRaisesRegex(ValueError, "alternate-path"):
            fixed_adapters.verify_positive_mount_namespace(
                root + "12 10 8:2 /var/www/customer-project /opt/uap-observer-inputs/bin/codex ro - ext4 /dev/fixture ro\n",
            )
        with self.assertRaisesRegex(ValueError, "synthetic"):
            fixed_adapters.verify_positive_mount_namespace("10 1 8:1 / / ro - ext4 /dev/root ro\n")

    @requires_disposable_observer_host
    def test_systemd_mount_namespace_keeps_only_auth_and_state_writable(self) -> None:
        systemd_run = shutil.which("systemd-run")
        if os.geteuid() != 0 or systemd_run is None or not Path("/run/systemd/system").is_dir():
            self.skipTest("systemd privileged mount namespace is unavailable")
        with tempfile.TemporaryDirectory() as temporary:
            profile = Path(temporary) / "profile"
            for client in ("codex", "cursor", "kiro"):
                for leaf in (".auth", ".state", ".config"):
                    (profile / client / leaf).mkdir(parents=True, exist_ok=True)
            probe = (
                "import errno,pathlib,sys\n"
                "root=pathlib.Path(sys.argv[1])\n"
                "for client in ('codex','cursor','kiro'):\n"
                " for leaf in ('.auth','.state'):\n"
                "  (root/client/leaf/'write-probe').write_text('ok')\n"
                " try: (root/client/'.config'/'forbidden').write_text('bad')\n"
                " except OSError as error:\n"
                "  assert error.errno in (errno.EROFS,errno.EACCES,errno.EPERM),error\n"
                " else: raise SystemExit('read-only profile path was writable')\n"
            )
            command = [
                systemd_run, "--quiet", "--wait", "--pipe", "--collect",
                "--property=Type=exec", "--property=PrivateMounts=yes",
                f"--property=BindReadOnlyPaths={profile}",
            ]
            command.extend(
                f"--property=BindPaths=-{profile / client / leaf}"
                for client in ("codex", "cursor", "kiro") for leaf in (".auth", ".state")
            )
            command.extend(["/usr/bin/python3", "-B", "-c", probe, str(profile)])
            completed = subprocess.run(command, text=True, capture_output=True, timeout=30)
            if completed.returncode != 0 and any(marker in (completed.stdout + completed.stderr).lower() for marker in ("failed to connect", "operation not permitted", "not supported", "access denied")):
                self.skipTest("systemd cannot create the required mount namespace on this host")
            self.assertEqual(completed.returncode, 0, completed.stdout + completed.stderr)

    def test_nonempty_runtime_notion_chatgpt_and_consent_validate_and_sign(self) -> None:
        observed = "2026-08-23T12:00:00Z"
        digest = "sha256:" + "a" * 64
        def release_tuple(product: str) -> dict[str, Any]:
            return {
                "product_id": product, "tree_digest": digest, "manifest_digest": digest,
                "distribution_id": "owner/package", "distribution_kind": "upstream",
                "release_sequence": 1, "package_version": "1.0.0",
                "source_repository": "owner/repository", "source_revision": "b" * 40,
                "source_path": "plugins/package", "snapshot_sequence": 1,
                "snapshot_digest": digest, "binary_digest": digest,
                "dependency_identity": "locked", "installer_version": "1",
                "adapter_version": "1", "client_version": None,
                "os": "linux", "architecture": "x86_64", "observed_at": observed,
            }
        request = Fixture(Path(tempfile.mkdtemp())).request()
        challenge = request["challenge"]["value"]
        consent = artifacts(challenge)["consent.json"]
        github = artifacts(challenge)["runtime-attestations.json"]["attestations"][0]["github_attestation"]
        original_invoke = fixed_adapters.invoke
        original_isolation = fixed_adapters.isolation_proof
        original_load = fixed_adapters.load_json
        original_mcp_call = fixed_adapters.mcp_call
        original_initialized = fixed_adapters.mcp_initialized
        original_wait = fixed_adapters.wait_human
        original_snapshot = fixed_adapters.regular_snapshot
        original_revalidate = fixed_adapters.revalidate_snapshot
        try:
            fixed_adapters.isolation_proof = lambda *_args: dict(fixed_adapters.PRIVACY_RESULT)
            fixed_adapters.invoke = lambda *args, **kwargs: ({
                "client_version": "codex-1", "client_id": "codex",
                "manager_before_digest": digest, "manager_after_digest": digest,
                "native_before_digest": digest, "native_after_digest": digest,
                "invocation_marker_digest": digest,
                "discovery_argv": ["codex", "mcp", "list", "--json"], "tool": "resolve-library-id",
            }, ["codex", "exec"], observed, observed)
            common = {"client": "codex", "application_id": "app", "endpoint": "https://example.test/mcp"}
            runtime_item = {**common, "plugin": "context7", "tuple": release_tuple("context7")}
            notion_item = {**common, "plugin": "notion", "tuple": release_tuple("notion")}
            runtime = fixed_adapters.runtime_record(runtime_item, {}, request, github, consent, Path("."), os.geteuid())
            notion = fixed_adapters.runtime_record(notion_item, {}, request, github, consent, Path("."), os.geteuid())

            chat_tuple = release_tuple("cloudflare-docs")
            binding = {"apps": {"cloudflare-docs": {"id": "plugin_asdk_app_" + "c" * 32}}}
            receipt = {"product_id": "cloudflare-docs", "application_id": "plugin_asdk_app_" + "c" * 32, "tuple": chat_tuple}
            fixed_adapters.regular_snapshot = lambda path, *args, **kwargs: {
                "body": fixed_adapters.canonical_json(binding if path.name == "app-binding.json" else receipt),
            }
            fixed_adapters.revalidate_snapshot = lambda *args, **kwargs: None
            responses = iter((
                ({"protocolVersion": "2025-06-18"}, "session"),
                ({"tools": [{"name": fixed_adapters.MCP_READ_TOOL}]}, "session"),
                ({"content": [{"type": "text", "text": "substantive"}]}, "session"),
            ))
            fixed_adapters.mcp_call = lambda *args, **kwargs: next(responses)
            fixed_adapters.mcp_initialized = lambda *args, **kwargs: None
            fixed_adapters.wait_human = lambda *args, **kwargs: {"observed_at": observed}
            chat_config = {"chatgpt": {
                "app_binding_path": "/opt/uap-observer-inputs/chatgpt/app-binding.json", "app_binding_sha256": digest,
                "app_id": "plugin_asdk_app_" + "c" * 32, "mcp_endpoint": fixed_adapters.MCP_ENDPOINT,
                "human_attestation_directory": "/fixture", "tuple": chat_tuple,
                "client_version": "chatgpt-web", "projection_receipt_path": "/opt/uap-observer-inputs/chatgpt/projection-receipt.json",
                "projection_receipt_sha256": digest,
            }}
            chat = fixed_adapters.chatgpt_artifact(chat_config, request, github, consent, os.geteuid())
        finally:
            fixed_adapters.invoke = original_invoke
            fixed_adapters.isolation_proof = original_isolation
            fixed_adapters.load_json = original_load
            fixed_adapters.mcp_call = original_mcp_call
            fixed_adapters.mcp_initialized = original_initialized
            fixed_adapters.wait_human = original_wait
            fixed_adapters.regular_snapshot = original_snapshot
            fixed_adapters.revalidate_snapshot = original_revalidate
        self.assertEqual((runtime["plugin"], notion["plugin"], chat["attestations"][0]["client"]), ("context7", "notion", "chatgpt"))
        produced = artifacts(challenge)
        validate_artifact_schemas(
            produced, challenge=challenge,
            scenario_contract_digest=request["scenario_contract_digest"],
            expected_bindings=request,
        )
        unsigned = {"schema_version": 1, "challenge": challenge, "signed_at": observed, "key_id": "fixture", "artifacts": produced}
        signer = FakeSigner()
        signature = base64.b64decode(signer.sign(unsigned))
        signer.key.public_key().verify(signature, signed_payload(unsigned))

    def test_public_mcp_content_requires_type_specific_payload(self) -> None:
        self.assertFalse(fixed_adapters.substantive_mcp_content({"type": "text"}))
        self.assertFalse(fixed_adapters.substantive_mcp_content({"type": "resource", "resource": {}}))
        self.assertFalse(fixed_adapters.substantive_mcp_content({"type": "resource_link"}))
        self.assertTrue(fixed_adapters.substantive_mcp_content({"type": "text", "text": "marker"}))

    def test_human_attestation_requires_exact_types_and_unambiguous_members(self) -> None:
        request = Fixture(Path(tempfile.mkdtemp())).request()
        app_id = "plugin_asdk_app_" + "c" * 32
        config = {"chatgpt": {"human_attestation_directory": "/fixture", "app_id": app_id}}
        now = time.time()
        record = {
            "schema_version": 1,
            "challenge": request["challenge"]["value"],
            "run_id": request["github"]["run_id"],
            "run_attempt": request["github"]["run_attempt"],
            "app_id": app_id,
            "request_digest": fixed_adapters.sha256(fixed_adapters.canonical_json(request)),
            "mcp_url": fixed_adapters.MCP_ENDPOINT,
            "consent": True, "ui_activation": True, "runtime_observed": True,
            "read_only": True, "no_secrets": True, "no_real_project": True,
            "observed_at": datetime.fromtimestamp(now - 1, timezone.utc).isoformat(),
            "expires_at": datetime.fromtimestamp(now + 300, timezone.utc).isoformat(),
        }
        with mock.patch.object(fixed_adapters, "read_regular", return_value=fixed_adapters.canonical_json(record)):
            self.assertEqual(fixed_adapters.wait_human(config, request, os.geteuid()), record)
        mutations = [("schema_version", True)] + [
            (field, 1) for field in (
                "consent", "ui_activation", "runtime_observed",
                "read_only", "no_secrets", "no_real_project",
            )
        ]
        for field, replacement in mutations:
            malformed = {**record, field: replacement}
            with self.subTest(field=field), mock.patch.object(fixed_adapters, "read_regular", return_value=fixed_adapters.canonical_json(malformed)), self.assertRaisesRegex(ValueError, "attestation is invalid"):
                fixed_adapters.wait_human(config, request, os.geteuid())
        canonical = fixed_adapters.canonical_json(record)
        ambiguous = (
            canonical[:-1] + b',"consent":true}',
            canonical[:-1] + b',"Consent":true}',
        )
        for encoded in ambiguous:
            with self.subTest(encoded=encoded[-32:]), mock.patch.object(fixed_adapters, "read_regular", return_value=encoded), self.assertRaisesRegex(ValueError, "duplicate or case-confusable"):
                fixed_adapters.wait_human(config, request, os.geteuid())

    def test_strict_structured_stream_decoders_reject_duplicates_case_aliases_and_nonfinite(self) -> None:
        adversarial = (
            b'{"type":"item.failed","type":"item.completed"}',
            b'{"type":"item.completed","Type":"item.failed"}',
            b'{"type":"item.completed","value":NaN}',
            b'{"type":"item.completed","value":Infinity}',
            b'{"type":"item.completed","value":-Infinity}',
            b'{"type":"item.completed","value":1e400}',
        )
        for encoded in adversarial:
            with self.subTest(encoded=encoded), self.assertRaises((ValueError, json.JSONDecodeError)):
                fixed_adapters.parsed_json_stream(encoded)
            with self.subTest(acp=encoded), self.assertRaises((ValueError, json.JSONDecodeError)):
                fixed_adapters.strict_json_loads(encoded)
            with self.subTest(runner=encoded), self.assertRaises((ValueError, json.JSONDecodeError)):
                fixed_runner.strict_json_loads(encoded)

    def test_public_mcp_decoder_rejects_ambiguous_nonfinite_and_bool_ids(self) -> None:
        class Response:
            status = 200
            headers: dict[str, str] = {}
            def __init__(self, body: bytes) -> None: self.body = body
            def geturl(self) -> str: return fixed_adapters.MCP_ENDPOINT
            def read(self, _limit: int) -> bytes: return self.body
            def __enter__(self): return self
            def __exit__(self, *_args): return False

        for body in (
            b'{"jsonrpc":"2.0","id":1,"id":1,"result":{}}',
            b'{"jsonrpc":"2.0","id":1,"result":{"value":NaN}}',
            b'{"jsonrpc":"2.0","id":true,"result":{}}',
        ):
            opener = mock.Mock()
            opener.open.return_value = Response(body)
            with self.subTest(body=body), mock.patch.object(fixed_adapters.urllib.request, "build_opener", return_value=opener), self.assertRaises(ValueError):
                fixed_adapters.mcp_call(fixed_adapters.MCP_ENDPOINT, 1, "tools/list", {})
        with self.assertRaisesRegex(ValueError, "request id"):
            fixed_adapters.mcp_call(fixed_adapters.MCP_ENDPOINT, True, "tools/list", {})

    def test_public_mcp_sse_parses_every_record_and_never_masks_earlier_evidence(self) -> None:
        class Response:
            status = 200
            headers: dict[str, str] = {}
            def __init__(self, body: bytes) -> None: self.body = body
            def geturl(self) -> str: return fixed_adapters.MCP_ENDPOINT
            def read(self, _limit: int) -> bytes: return self.body
            def __enter__(self): return self
            def __exit__(self, *_args): return False

        success = b'{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}'
        for first in (
            b'{malformed',
            b'{"jsonrpc":"2.0","id":1,"id":1,"result":{}}',
            b'{"jsonrpc":"2.0","id":1,"result":{"value":NaN}}',
            b'{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"bad"}}',
            b'{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"conflict"}]}}',
        ):
            body = b"event: message\ndata: " + first + b"\n\nevent: message\ndata: " + success + b"\n\n"
            opener = mock.Mock(); opener.open.return_value = Response(body)
            with self.subTest(first=first), mock.patch.object(fixed_adapters.urllib.request, "build_opener", return_value=opener), self.assertRaises((ValueError, json.JSONDecodeError)):
                fixed_adapters.mcp_call(fixed_adapters.MCP_ENDPOINT, 1, "tools/list", {})
        for body in (
            b"event: error\ndata: " + success + b"\n\n",
            b"event: message\ndata: " + success + b"\ndata: " + success + b"\n\n",
            b"event: message\nid: 1\ndata: " + success + b"\n\n",
        ):
            opener = mock.Mock(); opener.open.return_value = Response(body)
            with self.subTest(unexpected=body), mock.patch.object(fixed_adapters.urllib.request, "build_opener", return_value=opener), self.assertRaises(ValueError):
                fixed_adapters.mcp_call(fixed_adapters.MCP_ENDPOINT, 1, "tools/list", {})
        for body in (b"data: " + success + b"\n\n", b"event: message\ndata: " + success + b"\n\n"):
            opener = mock.Mock(); opener.open.return_value = Response(body)
            with self.subTest(valid=body), mock.patch.object(fixed_adapters.urllib.request, "build_opener", return_value=opener):
                self.assertEqual(fixed_adapters.mcp_call(fixed_adapters.MCP_ENDPOINT, 1, "tools/list", {})[0], {"tools": []})

    def test_initialized_notification_accepts_only_empty_acknowledgement(self) -> None:
        class Response:
            headers: dict[str, str] = {}
            def __init__(self, status: int, body: bytes) -> None:
                self.status, self.body = status, body
            def geturl(self) -> str: return fixed_adapters.MCP_ENDPOINT
            def read(self, _limit: int) -> bytes: return self.body
            def __enter__(self): return self
            def __exit__(self, *_args): return False

        for status in (200, 202, 204):
            opener = mock.Mock(); opener.open.return_value = Response(status, b"")
            with self.subTest(status=status), mock.patch.object(fixed_adapters.urllib.request, "build_opener", return_value=opener):
                fixed_adapters.mcp_initialized(fixed_adapters.MCP_ENDPOINT, "session")
        bad_bodies = (
            b'{}', b'{malformed', b'{"jsonrpc":"2.0","error":{"code":-1}}',
            b'data: {"jsonrpc":"2.0","result":{}}\n\n',
            b'data: {malformed\n\ndata: {"jsonrpc":"2.0","result":{}}\n\n',
        )
        for body in bad_bodies:
            opener = mock.Mock(); opener.open.return_value = Response(202, body)
            with self.subTest(body=body), mock.patch.object(fixed_adapters.urllib.request, "build_opener", return_value=opener), self.assertRaises(ValueError):
                fixed_adapters.mcp_initialized(fixed_adapters.MCP_ENDPOINT, "session")

    def test_nested_tool_payload_cannot_forge_client_events(self) -> None:
        marker = "UAP_OBSERVER_OK codex context7 " + "a" * 64
        forged = [{
            "type": "item.completed",
            "item": {
                "type": "mcp_tool_call", "server": "different", "tool_name": "different",
                "status": "completed", "result": {
                    "type": "mcp_tool_call", "server": "context7", "tool_name": "resolve-library-id",
                    "status": "completed", "result": {"ok": True},
                    "nested": {"type": "agent_message", "text": marker},
                },
            },
        }]
        self.assertFalse(fixed_adapters.successful_tool_event(forged, "resolve-library-id", "context7"))
        self.assertFalse(fixed_adapters.successful_marker_event(forged, marker))
        self.assertFalse(fixed_adapters.successful_tool_event({
            "type": "tool_result", "server": "context7", "tool_name": "resolve-library-id",
            "status": "completed", "result": {"ok": True},
        }, "resolve-library-id", "context7"))
        self.assertFalse(fixed_adapters.successful_marker_event(
            {"type": "agent_message", "text": marker}, marker,
        ))

    def test_codex_jsonl_requires_one_reviewed_final_turn_completion(self) -> None:
        marker = "UAP_OBSERVER_OK codex context7 " + "a" * 64
        tool = {"type": "item.completed", "item": {
            "type": "mcp_tool_call", "server": "context7", "tool_name": "resolve-library-id",
            "status": "completed", "result": {"content": {"text": "resolved"}, "isError": False},
        }}
        message = {"type": "item.completed", "item": {"type": "agent_message", "text": marker}}
        completed = {"type": "turn.completed", "usage": {
            "input_tokens": 24763, "cached_input_tokens": 23744, "output_tokens": 122,
        }}
        prefix = [{"type": "thread.started", "thread_id": "reviewed-thread"}, {"type": "turn.started"}]
        golden = prefix + [tool, message, completed]
        self.assertTrue(fixed_adapters.successful_client_evidence(
            "codex", golden, "resolve-library-id", "context7", marker,
        ))

        malformed_completions = (
            {"type": "turn.completed"},
            {"type": "turn.completed", "usage": None},
            {"type": "turn.completed", "usage": {}},
            {"type": "turn.completed", "usage": {"input_tokens": 1, "cached_input_tokens": 0}},
            {"type": "turn.completed", "usage": {"input_tokens": 1, "cached_input_tokens": 0, "output_tokens": 1, "total_tokens": 2}},
            {"type": "turn.completed", "usage": {"input_tokens": True, "cached_input_tokens": 0, "output_tokens": 1}},
            {"type": "turn.completed", "usage": {"input_tokens": -1, "cached_input_tokens": 0, "output_tokens": 1}},
            {"type": "turn.completed", "usage": {"input_tokens": 1, "cached_input_tokens": 2, "output_tokens": 1}},
            {"type": "turn.completed", "usage": completed["usage"], "status": "completed"},
            {"type": "turn.completed", "usage": completed["usage"], "error": None},
        )
        rejected = (
            prefix + [tool, message],
            prefix + [completed, tool, message],
            prefix + [tool, completed, message],
            prefix + [tool, message, completed, completed],
            prefix + [tool, message, {"type": "turn.failed", "error": {"message": "failed"}}],
            prefix + [tool, message, {"type": "turn.cancelled"}],
            prefix + [tool, message, {"type": "turn.canceled"}],
            prefix + [tool, message, completed, {"type": "turn.failed", "error": {"message": "conflict"}}],
            prefix + [tool, message, completed, {"type": "item.completed", "item": {"type": "agent_message", "text": "extra"}}],
            prefix + [tool, message, {**completed, "type": "turn.failed"}],
            *(prefix + [tool, message, terminal] for terminal in malformed_completions),
        )
        for stream in rejected:
            with self.subTest(stream=stream):
                self.assertFalse(fixed_adapters.successful_client_evidence(
                    "codex", stream, "resolve-library-id", "context7", marker,
                ))

    def test_codex_current_mcp_stream_requires_exact_started_completed_pair(self) -> None:
        marker = "UAP_OBSERVER_OK codex context7 " + "b" * 64
        arguments = {"libraryName": "React", "query": "React library"}
        started = {"type": "item.started", "item": {
            "id": "call-1", "type": "mcp_tool_call", "server": "context7",
            "tool": "resolve-library-id", "status": "in_progress", "arguments": arguments,
            "result": None, "error": None,
        }}
        completed = {"type": "item.completed", "item": {
            **started["item"], "status": "completed",
            "result": {"content": [{"type": "text", "text": "resolved"}], "structured_content": None},
        }}
        preface = {"type": "item.completed", "item": {
            "id": "preface-1", "type": "agent_message",
            "text": "I will perform the read-only Context7 lookup.",
        }}
        message = {"type": "item.completed", "item": {
            "id": "message-1", "type": "agent_message", "text": marker,
        }}
        completed_turn = {"type": "turn.completed", "usage": {
            "input_tokens": 24763, "cached_input_tokens": 23744,
            "cache_write_input_tokens": 0, "output_tokens": 122, "reasoning_output_tokens": 20,
        }}
        prefix = [{"type": "thread.started", "thread_id": "current-thread"}, {"type": "turn.started"}]
        golden = prefix + [preface, started, completed, message, completed_turn]
        self.assertTrue(fixed_adapters.successful_client_evidence(
            "codex", golden, "resolve-library-id", "context7", marker,
        ))
        self.assertTrue(fixed_adapters.successful_client_evidence(
            "codex", prefix + [started, completed, message, completed_turn],
            "resolve-library-id", "context7", marker,
        ))

        mutations = []
        for path, value in (
            ((4, "item", "id"), "different-call"),
            ((3, "item", "arguments"), {"libraryName": "Vue", "query": "Vue library"}),
            ((3, "item", "error"), {"message": "failed"}),
            ((4, "item", "status"), "failed"),
            ((4, "item", "result"), {"content": [], "structured_content": None}),
            ((5, "item", "text"), "different-marker"),
            ((3, "item", "extra"), True),
        ):
            candidate = json.loads(json.dumps(golden))
            target: Any = candidate
            for key in path[:-1]:
                target = target[key]
            target[path[-1]] = value
            mutations.append(candidate)
        mutations.extend((
            prefix + [preface, completed, message, completed_turn],
            prefix + [preface, started, completed, message, completed_turn, completed_turn],
            prefix + [{**preface, "item": {**preface["item"], "text": marker}}, started, completed, message, completed_turn],
        ))
        for stream in mutations:
            with self.subTest(stream=stream):
                self.assertFalse(fixed_adapters.successful_client_evidence(
                    "codex", stream, "resolve-library-id", "context7", marker,
                ))

    def test_cursor_jsonl_arrays_cannot_attest_through_production_parser(self) -> None:
        marker = "UAP_OBSERVER_OK cursor context7 " + "a" * 64
        tool = {"type": "tool_call", "subtype": "completed", "tool_call": {"mcpToolCall": {
            "args": {"serverName": "context7", "toolName": "resolve-library-id"},
            "result": {"success": {"content": "resolved"}},
        }}}
        message = {"type": "assistant", "message": {
            "role": "assistant", "content": [{"type": "text", "text": marker}],
        }}

        normal = b"\n".join(fixed_adapters.canonical_json(event) for event in (tool, message)) + b"\n"
        parsed = fixed_adapters.parsed_json_stream(normal)
        self.assertEqual(parsed, [tool, message])
        self.assertFalse(fixed_adapters.successful_client_evidence(
            "cursor", parsed, "resolve-library-id", "context7", marker,
        ))

        array_line = fixed_adapters.canonical_json([tool, message]) + b"\n"
        nested_array_line = fixed_adapters.canonical_json([[tool], [message]]) + b"\n"
        nested_array_lines = (
            fixed_adapters.canonical_json([tool]) + b"\n"
            + fixed_adapters.canonical_json([message]) + b"\n"
        )
        for encoded in (array_line, nested_array_line, nested_array_lines):
            with self.subTest(encoded=encoded):
                parsed = fixed_adapters.parsed_json_stream(encoded)
                self.assertTrue(any(isinstance(event, list) for event in parsed))
                self.assertFalse(fixed_adapters.successful_client_evidence(
                    "cursor", parsed, "resolve-library-id", "context7", marker,
                ))

    def test_codex_jsonl_rejects_alias_conflicts_and_unreviewed_marker_fields(self) -> None:
        marker = "UAP_OBSERVER_OK codex context7 " + "a" * 64
        tool = {"type": "item.completed", "item": {
            "type": "mcp_tool_call", "server": "context7", "tool_name": "resolve-library-id",
            "status": "completed", "result": {"content": {"text": "resolved"}, "isError": False},
        }}
        message = {"type": "item.completed", "item": {"type": "agent_message", "text": marker}}
        completed = {"type": "turn.completed", "usage": {
            "input_tokens": 24763, "cached_input_tokens": 23744, "output_tokens": 122,
        }}
        self.assertTrue(fixed_adapters.successful_client_evidence(
            "codex", [tool, message, completed], "resolve-library-id", "context7", marker,
        ))

        tool_extras = (
            {"name": "different"},
            {"tool": "different"},
            {"tool_name": "resolve-library-id", "name": "different"},
            {"server_name": "different"},
            {"mcp_server": "different"},
            {"product_id": "different"},
            {"server": "context7", "server_name": "different"},
            {"error": None},
            {"role": "assistant"},
        )
        for extra in tool_extras:
            forged = json.loads(json.dumps(tool))
            forged["item"].update(extra)
            with self.subTest(tool_extra=extra):
                self.assertFalse(fixed_adapters.successful_client_evidence(
                    "codex", [forged, message, completed], "resolve-library-id", "context7", marker,
                ))

        marker_extras = (
            {"role": "assistant"},
            {"role": "user"},
            {"server": "context7"},
            {"server_name": "different"},
            {"tool": "resolve-library-id"},
            {"tool_name": "different"},
            {"name": "different"},
            {"status": "completed"},
        )
        for extra in marker_extras:
            forged = json.loads(json.dumps(message))
            forged["item"].update(extra)
            with self.subTest(marker_extra=extra):
                self.assertFalse(fixed_adapters.successful_client_evidence(
                    "codex", [tool, forged, completed], "resolve-library-id", "context7", marker,
                ))

        unreviewed_streams = (
            [tool, {**message, "role": "assistant"}, completed],
            [{**tool, "server": "context7"}, message, completed],
            [[tool], message, completed],
            [tool, [message], completed],
        )
        for stream in unreviewed_streams:
            with self.subTest(unreviewed_event_shape=stream):
                self.assertFalse(fixed_adapters.successful_client_evidence(
                    "codex", stream, "resolve-library-id", "context7", marker,
                ))

    @staticmethod
    def codex_skill_records(marker: str, skill_path: Path, skill_body: str) -> list[dict[str, Any]]:
        read = f'/bin/bash -lc "sed -n \'1,240p\' {skill_path}"'
        search = "/bin/bash -lc \"rg -n '^UAP_SKILL_SECRET_' .\""
        return [
            {"type": "thread.started", "thread_id": "reviewed-skill-thread"},
            {"type": "turn.started"},
            {"type": "item.completed", "item": {
                "id": "item-preface", "type": "agent_message",
                "text": "I am using the installed code-tool-router skill for exact search.",
            }},
            {"type": "item.started", "item": {
                "id": "item-read", "type": "command_execution", "command": read,
                "aggregated_output": "", "exit_code": None, "status": "in_progress",
            }},
            {"type": "item.completed", "item": {
                "id": "item-read", "type": "command_execution", "command": read,
                "aggregated_output": skill_body, "exit_code": 0, "status": "completed",
            }},
            {"type": "item.started", "item": {
                "id": "item-search", "type": "command_execution", "command": search,
                "aggregated_output": "", "exit_code": None, "status": "in_progress",
            }},
            {"type": "item.completed", "item": {
                "id": "item-search", "type": "command_execution", "command": search,
                "aggregated_output": f"./uap-skill-probe.txt:1:{marker}\n", "exit_code": 0, "status": "completed",
            }},
            {"type": "item.completed", "item": {"id": "item-marker", "type": "agent_message", "text": marker}},
            {"type": "turn.completed", "usage": {
                "input_tokens": 44805, "cached_input_tokens": 28160,
                "cache_write_input_tokens": 0, "output_tokens": 258,
                "reasoning_output_tokens": 16,
            }},
        ]

    def test_codex_0147_skill_stream_requires_skill_read_hidden_search_and_marker(self) -> None:
        marker = "UAP_SKILL_SECRET_" + "a" * 64
        skill_path = Path("/var/lib/uap-observer/profiles/codex/.codex/plugins/cache/reviewed/agent-code-navigator/skills/code-tool-router/SKILL.md")
        skill_body = "---\nname: code-tool-router\n---\n# Code Tool Router\n"
        records = self.codex_skill_records(marker, skill_path, skill_body)
        self.assertTrue(fixed_adapters.successful_codex_skill_evidence(
            records, marker, skill_path, skill_body.encode(),
        ))
        without_preface = [*records[:2], *records[3:]]
        self.assertTrue(fixed_adapters.successful_codex_skill_evidence(
            without_preface, marker, skill_path, skill_body.encode(),
        ))
        mutations = (
            lambda rows: rows[2]["item"].update(text=marker),
            lambda rows: rows[3]["item"].update(command=rows[3]["item"]["command"].replace(str(skill_path), "/foreign/SKILL.md")),
            lambda rows: rows[4]["item"].update(aggregated_output=skill_body + "forged\n"),
            lambda rows: rows[5]["item"].update(command="/bin/bash -lc \"rg UAP_SKILL_SECRET_ .\""),
            lambda rows: rows[6]["item"].update(aggregated_output=marker + "\n"),
            lambda rows: rows[6]["item"].update(exit_code=1),
            lambda rows: rows[6]["item"].update(id="item-read"),
            lambda rows: rows[7]["item"].update(text=f"`{marker}`"),
            lambda rows: rows.insert(7, {"type": "item.completed", "item": {"id": "extra", "type": "agent_message", "text": "extra"}}),
            lambda rows: rows[8].update(type="turn.failed"),
        )
        for mutation in mutations:
            forged = json.loads(json.dumps(records))
            mutation(forged)
            with self.subTest(mutation=mutation):
                self.assertFalse(fixed_adapters.successful_codex_skill_evidence(
                    forged, marker, skill_path, skill_body.encode(),
                ))

    @staticmethod
    def cursor_thinking(session: str, text: str = "reviewed read-only step") -> list[dict]:
        return [
            {"type": "thinking", "subtype": "delta", "session_id": session, "text": text, "timestamp_ms": 1},
            {"type": "thinking", "subtype": "completed", "session_id": session, "timestamp_ms": 2},
        ]

    @staticmethod
    def cursor_tool_pair(session: str, kind: str, started: dict, completed: dict, sequence: int) -> list[dict]:
        call, model, tool_id = f"call-{sequence}", f"model-{sequence}", f"tool-{sequence}"
        common = {"hookAdditionalContexts": [], "startedAtMs": str(sequence), "toolCallId": tool_id}
        outer = {"type": "tool_call", "session_id": session, "model_call_id": model, "call_id": call, "timestamp_ms": sequence}
        key = kind + "ToolCall"
        return [
            {**outer, "subtype": "started", "tool_call": {**common, key: started}},
            {**outer, "subtype": "completed", "timestamp_ms": sequence + 1, "tool_call": {**common, "completedAtMs": str(sequence + 1), key: completed}},
        ]

    @classmethod
    def cursor_stream_shell(cls, prompt: str, workspace: Path, marker: str) -> tuple[list[dict], str]:
        session = "cursor-session"
        return ([
            {"type": "system", "subtype": "init", "session_id": session, "apiKeySource": "login", "cwd": str(workspace), "model": "Auto", "permissionMode": "default"},
            {"type": "user", "session_id": session, "message": {"role": "user", "content": [{"type": "text", "text": prompt}]}},
            *cls.cursor_thinking(session),
        ], session)

    @classmethod
    def cursor_finish(cls, session: str, marker: str) -> list[dict]:
        return [
            *cls.cursor_thinking(session, "return the exact result"),
            {"type": "assistant", "session_id": session, "message": {"role": "assistant", "content": [{"type": "text", "text": marker}]}},
            {"type": "result", "subtype": "success", "session_id": session, "request_id": "request-1", "duration_ms": 10, "duration_api_ms": 9, "is_error": False, "result": "preface" + marker, "usage": {"cacheReadTokens": 1, "cacheWriteTokens": 0, "inputTokens": 2, "outputTokens": 3}},
        ]

    def test_cursor_20260825_skill_stream_requires_read_search_marker_and_terminal(self) -> None:
        marker = "UAP_SKILL_SECRET_" + "a" * 64
        workspace = Path("/tmp/cursor-skill-workspace")
        skill = Path("/profile/.cursor/plugins/local/acn/skills/code-tool-router/SKILL.md")
        body = b"---\nname: code-tool-router\n---\n"
        rows, session = self.cursor_stream_shell(fixed_adapters.SKILL_PROBE_PROMPT, workspace, marker)
        rows += self.cursor_tool_pair(session, "read", {"args": {"path": str(skill)}}, {"args": {"path": str(skill)}, "result": {"success": {"content": body.decode(), "exceededLimit": False, "fileSize": len(body), "isEmpty": False, "path": str(skill), "readRange": {"startLine": 1, "endLine": 4}, "relatedCursorRulePaths": [], "relatedCursorRules": [], "totalLines": 4}}}, 10)
        rows += self.cursor_thinking(session)
        grep_args = {"caseInsensitive": False, "multiline": False, "offset": 0, "path": str(workspace), "pattern": fixed_adapters.SKILL_PROBE_QUERY, "toolCallId": "tool-20"}
        grep_success = {"outputMode": "content", "path": str(workspace), "pattern": fixed_adapters.SKILL_PROBE_QUERY, "workspaceResults": {str(workspace): {"content": {"clientTruncated": False, "matches": [{"file": "./uap-skill-probe.txt", "matches": [{"content": marker, "contentTruncated": False, "isContextLine": False, "lineNumber": 1}]}], "ripgrepTruncated": False, "totalLines": 1, "totalMatchedLines": 1}}}}
        rows += self.cursor_tool_pair(session, "grep", {"args": grep_args}, {"args": grep_args, "result": {"success": grep_success}}, 20)
        rows += self.cursor_finish(session, marker)
        self.assertTrue(fixed_adapters.successful_cursor_skill_evidence(rows, marker, skill, body, workspace))
        mutations = (
            lambda value: value[0].update(cwd="/foreign"),
            lambda value: value[4]["tool_call"]["readToolCall"]["args"].update(path="/foreign/SKILL.md"),
            lambda value: value[8]["tool_call"]["grepToolCall"]["args"].update(pattern="UAP_SKILL_SECRET_"),
            lambda value: value[9]["tool_call"]["grepToolCall"]["result"]["success"]["workspaceResults"][str(workspace)]["content"]["matches"][0]["matches"][0].update(content="forged"),
            lambda value: value[-2]["message"]["content"][0].update(text=f"`{marker}`"),
            lambda value: value[-1].update(is_error=True),
        )
        for mutation in mutations:
            forged = json.loads(json.dumps(rows)); mutation(forged)
            with self.subTest(mutation=mutation):
                self.assertFalse(fixed_adapters.successful_cursor_skill_evidence(forged, marker, skill, body, workspace))

    def test_cursor_20260825_mcp_stream_requires_discovery_call_marker_and_terminal(self) -> None:
        marker = "UAP_OBSERVER_OK cursor context7 " + "b" * 64
        workspace, plugin, tool = Path("/tmp/cursor-mcp-workspace"), "context7", "resolve-library-id"
        rows, session = self.cursor_stream_shell(fixed_adapters.mcp_probe_prompt(plugin, tool, marker), workspace, marker)
        discovery_args = {"server": plugin, "toolCallId": "tool-10", "toolName": tool}
        description = json.dumps({"mode": "single_tool", "namespace": plugin, "namespaceStatus": "ready", "tool": {"tool": tool}})
        rows += self.cursor_tool_pair(session, "getMcpTools", {"args": discovery_args}, {"args": discovery_args, "result": {"success": {"content": description}}}, 10)
        rows += self.cursor_thinking(session)
        target_args = {"args": {"libraryName": "React", "query": "React"}, "name": f"{plugin}-{tool}", "providerIdentifier": plugin, "serverIdentifier": plugin, "skipApproval": False, "smartModeApprovalOnly": False, "toolCallId": "tool-20", "toolName": tool}
        rows += self.cursor_tool_pair(session, "mcp", {"args": target_args, "description": "Resolve React"}, {"args": target_args, "description": "Resolve React", "result": {"success": {"content": [{"text": {"text": "resolved"}}], "isError": False}}}, 20)
        rows += self.cursor_finish(session, marker)
        self.assertTrue(fixed_adapters.successful_cursor_mcp_evidence(rows, tool, plugin, marker, workspace))
        mutations = (
            lambda value: value[4]["tool_call"]["getMcpToolsToolCall"]["args"].update(server="foreign"),
            lambda value: value[5]["tool_call"]["getMcpToolsToolCall"]["result"]["success"].update(content="{}"),
            lambda value: value[5]["tool_call"]["getMcpToolsToolCall"]["result"]["success"].update(content=json.dumps({"mode": "single_tool", "namespace": plugin, "namespaceStatus": "ready", "tool": {"tool": tool, "description": marker}})),
            lambda value: value[8]["tool_call"]["mcpToolCall"]["args"].update(providerIdentifier="foreign"),
            lambda value: value[9]["tool_call"]["mcpToolCall"]["result"]["success"].update(isError=True),
            lambda value: value[-1].update(subtype="failed"),
        )
        for mutation in mutations:
            forged = json.loads(json.dumps(rows)); mutation(forged)
            with self.subTest(mutation=mutation):
                self.assertFalse(fixed_adapters.successful_cursor_mcp_evidence(forged, tool, plugin, marker, workspace))

    def test_cursor_20260825_mcp_stream_accepts_direct_tool_descriptor_fail_closed(self) -> None:
        self.assertEqual(fixed_adapters.MCP_PROBE_TOOLS["notion"], "notion-search")
        marker = "UAP_OBSERVER_OK cursor context7 " + "c" * 64
        workspace, plugin, tool = Path("/tmp/cursor-mcp-workspace"), "context7", "resolve-library-id"
        rows, session = self.cursor_stream_shell(fixed_adapters.mcp_probe_prompt(plugin, tool, marker), workspace, marker)
        discovery_args = {"server": plugin, "toolCallId": "tool-10", "toolName": tool}
        descriptor = {
            "description": "Resolve a package name to a Context7 library identifier.",
            "inputSchema": {
                "$schema": "https://json-schema.org/draft/2020-12/schema",
                "type": "object",
                "properties": {
                    "libraryName": {"type": "string"},
                    "query": {"type": "string"},
                },
                "required": ["query", "libraryName"],
            },
            "tool": tool,
        }
        rows += self.cursor_tool_pair(
            session,
            "getMcpTools",
            {"args": discovery_args},
            {"args": discovery_args, "result": {"success": {"content": json.dumps(descriptor)}}},
            10,
        )
        rows += self.cursor_thinking(session)
        target_args = {"args": {"libraryName": "React", "query": "React"}, "name": f"{plugin}-{tool}", "providerIdentifier": plugin, "serverIdentifier": plugin, "skipApproval": False, "smartModeApprovalOnly": False, "toolCallId": "tool-20", "toolName": tool}
        rows += self.cursor_tool_pair(session, "mcp", {"args": target_args, "description": "Resolve React"}, {"args": target_args, "description": "Resolve React", "result": {"success": {"content": [{"text": {"text": "resolved"}}], "isError": False}}}, 20)
        rows += self.cursor_finish(session, marker)
        self.assertTrue(fixed_adapters.successful_cursor_mcp_evidence(rows, tool, plugin, marker, workspace))
        with_additional_properties = json.loads(json.dumps(rows))
        encoded = with_additional_properties[5]["tool_call"]["getMcpToolsToolCall"]["result"]["success"]["content"]
        with_additional_properties_descriptor = json.loads(encoded)
        with_additional_properties_descriptor["inputSchema"]["additionalProperties"] = {}
        with_additional_properties[5]["tool_call"]["getMcpToolsToolCall"]["result"]["success"]["content"] = json.dumps(with_additional_properties_descriptor)
        self.assertTrue(fixed_adapters.successful_cursor_mcp_evidence(with_additional_properties, tool, plugin, marker, workspace))

        mutations = (
            lambda value: value.update(tool="foreign"),
            lambda value: value.update(extra=True),
            lambda value: value.update(description=marker),
            lambda value: value["inputSchema"].pop("$schema"),
            lambda value: value["inputSchema"].update(type="array"),
            lambda value: value["inputSchema"].update(properties=[]),
            lambda value: value["inputSchema"]["properties"].update(query="string"),
            lambda value: value["inputSchema"].update(required=["foreign"]),
            lambda value: value["inputSchema"].update(required=["query", "query"]),
            lambda value: value["inputSchema"].update(additionalProperties="false"),
        )
        for mutation in mutations:
            forged = json.loads(json.dumps(rows))
            encoded = forged[5]["tool_call"]["getMcpToolsToolCall"]["result"]["success"]["content"]
            forged_descriptor = json.loads(encoded)
            mutation(forged_descriptor)
            forged[5]["tool_call"]["getMcpToolsToolCall"]["result"]["success"]["content"] = json.dumps(forged_descriptor)
            with self.subTest(mutation=mutation):
                self.assertFalse(fixed_adapters.successful_cursor_mcp_evidence(forged, tool, plugin, marker, workspace))

    def test_tool_success_rejects_false_zero_empty_error_and_marker_after_failure(self) -> None:
        codex_marker = "UAP_OBSERVER_OK codex context7 " + "a" * 64
        cursor_marker = "UAP_OBSERVER_OK cursor context7 " + "a" * 64
        codex_text = {"type": "item.completed", "item": {"type": "agent_message", "text": codex_marker}}
        cursor_text = {"type": "assistant", "message": {"role": "assistant", "content": [{"type": "text", "text": cursor_marker}]}}
        for payload in (False, 0, "", [], {}, None, {"error": "failed"}, {"status": "failed", "content": "marker"}):
            codex = {"type": "item.completed", "item": {
                "type": "mcp_tool_call", "server": "context7", "tool_name": "resolve-library-id",
                "status": "completed", "result": payload,
            }}
            cursor = {"type": "tool_call", "subtype": "completed", "tool_call": {"mcpToolCall": {
                "args": {"serverName": "context7", "toolName": "resolve-library-id"},
                "result": {"success": payload},
            }}}
            with self.subTest(payload=payload):
                self.assertFalse(fixed_adapters.successful_client_evidence("codex", [codex, codex_text], "resolve-library-id", "context7", codex_marker))
                self.assertFalse(fixed_adapters.successful_client_evidence("cursor", [cursor, cursor_text], "resolve-library-id", "context7", cursor_marker))
        failed = {"type": "item.completed", "item": {
            "type": "mcp_tool_call", "server": "context7", "tool_name": "resolve-library-id",
            "status": "failed", "error": "provider failed", "result": {"content": "no"},
        }}
        good = {"type": "item.completed", "item": {
            "type": "mcp_tool_call", "server": "context7", "tool_name": "resolve-library-id",
            "status": "completed", "result": {"content": "resolved"},
        }}
        self.assertFalse(fixed_adapters.successful_client_evidence(
            "codex", [failed, good, codex_text], "resolve-library-id", "context7", codex_marker,
        ))
        self.assertFalse(fixed_adapters.successful_client_evidence(
            "codex", [codex_text, good], "resolve-library-id", "context7", codex_marker,
        ))

    def test_tool_success_rejects_nested_controls_failed_events_and_extra_terminal_calls(self) -> None:
        marker = "UAP_OBSERVER_OK codex context7 " + "a" * 64
        text = {"type": "item.completed", "item": {"type": "agent_message", "text": marker}}
        turn = {"type": "turn.completed", "usage": {
            "input_tokens": 24763, "cached_input_tokens": 23744, "output_tokens": 122,
        }}
        def event(result: object, *, event_type: str = "item.completed", server: str = "context7") -> dict[str, object]:
            return {"type": event_type, "item": {
                "type": "mcp_tool_call", "server": server, "tool_name": "resolve-library-id",
                "status": "completed", "result": result,
            }}
        golden = event({"content": {"text": "resolved"}, "isError": False})
        self.assertTrue(fixed_adapters.successful_client_evidence(
            "codex", [golden, text, turn], "resolve-library-id", "context7", marker,
        ))
        for payload in (
            {"content": "resolved", "isError": True},
            {"content": "resolved", "success": False},
            {"content": "resolved", "ok": False},
            {"content": {"status": "cancelled", "text": "resolved"}},
            {"content": {"success": 0, "text": "resolved"}},
        ):
            with self.subTest(payload=payload):
                self.assertFalse(fixed_adapters.successful_client_evidence(
                    "codex", [event(payload), text, turn], "resolve-library-id", "context7", marker,
                ))
        self.assertFalse(fixed_adapters.successful_client_evidence(
            "codex", [event({"content": "failed"}, event_type="item.failed"), golden, text],
            "resolve-library-id", "context7", marker,
        ))
        self.assertFalse(fixed_adapters.successful_client_evidence(
            "codex", [golden, event({"content": "other"}, server="other"), text],
            "resolve-library-id", "context7", marker,
        ))
        for extra in (
            {"type": "turn.failed", "error": {"message": "unrelated failure"}},
            {"type": "item.completed", "item": {"type": "agent_message", "text": "extra terminal text"}},
            {"type": "item.completed", "item": {"type": "mcp_tool_call", "server": "other", "tool_name": "other", "status": "completed", "result": {"content": "extra"}}},
        ):
            with self.subTest(extra_terminal=extra):
                self.assertFalse(fixed_adapters.successful_client_evidence(
                    "codex", [golden, text, extra], "resolve-library-id", "context7", marker,
                ))
        self.assertFalse(fixed_adapters.successful_client_evidence(
            "codex", [golden, {"type": "thread.started"}, text],
            "resolve-library-id", "context7", marker,
        ))

    def test_cursor_whole_stream_rejects_extra_terminal_and_nested_failure_controls(self) -> None:
        marker = "UAP_OBSERVER_OK cursor context7 " + "a" * 64
        tool = {"type": "tool_call", "subtype": "completed", "tool_call": {"mcpToolCall": {
            "args": {"serverName": "context7", "toolName": "resolve-library-id"},
            "result": {"success": {"content": "resolved"}},
        }}}
        text = {"type": "assistant", "message": {"role": "assistant", "content": [{"type": "text", "text": marker}]}}
        for stream in (
            [tool, text, {"type": "assistant", "message": {"role": "assistant", "content": [{"type": "text", "text": "extra"}]}}],
            [tool, {"type": "status", "payload": {"healthy": False}}, text],
            [tool, {"type": "error", "message": "unrelated"}, text],
            [tool, {"type": "tool_call", "subtype": "completed", "tool_call": {"mcpToolCall": {"args": {"serverName": "other", "toolName": "other"}, "result": {"success": {"content": "extra"}}}}}, text],
            [tool, text, {"type": "result", "subtype": "success", "is_error": True, "result": "provider failed"}],
            [tool, text, {"type": "result", "subtype": "success", "isError": False, "result": "extra terminal"}],
            [tool, text, {"type": "result", "subtype": "failed", "is_error": False, "result": "failed"}],
            [tool, text, {"type": "result", "subtype": "ambiguous", "result": "unknown"}],
        ):
            with self.subTest(stream=stream):
                self.assertFalse(fixed_adapters.successful_client_evidence(
                    "cursor", stream, "resolve-library-id", "context7", marker,
                ))

    def test_cursor_legacy_two_event_grammar_is_never_accepted(self) -> None:
        marker = "UAP_OBSERVER_OK cursor context7 " + "a" * 64
        tool = {"type": "tool_call", "subtype": "completed", "call_id": "call-1", "tool_call": {"mcpToolCall": {
            "args": {"serverName": "context7", "toolName": "resolve-library-id", "arguments": {}},
            "result": {"success": {"content": "resolved"}},
        }}}
        assistant = {"type": "assistant", "message": {"role": "assistant", "content": [{"type": "text", "text": marker}]}}
        self.assertFalse(fixed_adapters.successful_client_evidence(
            "cursor", [tool, assistant], "resolve-library-id", "context7", marker,
        ))
        mutations = (
            lambda value: value[0].update(extra=True),
            lambda value: value[0].update(terminal_count=2),
            lambda value: value[0].update(subtype="ambiguous"),
            lambda value: value[0]["tool_call"].update(extra=True),
            lambda value: value[0]["tool_call"]["mcpToolCall"].update(extra=True),
            lambda value: value[0]["tool_call"]["mcpToolCall"]["args"].update(server="context7"),
            lambda value: value[0]["tool_call"]["mcpToolCall"]["args"].update(terminalCount=2),
            lambda value: value[0]["tool_call"]["mcpToolCall"]["result"].update(isError=False),
            lambda value: value[0]["tool_call"]["mcpToolCall"]["result"]["success"].update(extra=True),
            lambda value: value[1].update(extra=True),
            lambda value: value[1]["message"].update(extra=True),
            lambda value: value[1]["message"]["content"][0].update(extra=True),
        )
        for mutation in mutations:
            forged = json.loads(json.dumps([tool, assistant]))
            mutation(forged)
            with self.subTest(forged=forged):
                self.assertFalse(fixed_adapters.successful_client_evidence(
                    "cursor", forged, "resolve-library-id", "context7", marker,
                ))

    def test_cursor_stream_rejects_array_events_without_recursive_collapsing(self) -> None:
        marker = "UAP_OBSERVER_OK cursor context7 " + "a" * 64
        tool = {"type": "tool_call", "subtype": "completed", "call_id": "call-1", "tool_call": {"mcpToolCall": {
            "args": {"serverName": "context7", "toolName": "resolve-library-id", "arguments": {}},
            "result": {"success": {"content": "resolved"}},
        }}}
        assistant = {"type": "assistant", "message": {
            "role": "assistant", "content": [{"type": "text", "text": marker}],
        }}
        self.assertFalse(fixed_adapters.successful_client_evidence(
            "cursor", [tool, assistant], "resolve-library-id", "context7", marker,
        ))
        malformed_streams = (
            [[tool, tool], [assistant, assistant]],
            [[tool], assistant],
            [tool, [assistant]],
            [[[tool]], [[assistant]]],
            [[tool, assistant]],
            [{"type": "wrapper", "events": [tool, tool]}, assistant],
            [tool, {"type": "wrapper", "events": [assistant, assistant]}],
        )
        for stream in malformed_streams:
            with self.subTest(stream=stream):
                self.assertFalse(fixed_adapters.successful_client_evidence(
                    "cursor", stream, "resolve-library-id", "context7", marker,
                ))

    @requires_disposable_observer_host
    def test_native_projection_requires_exact_tuple_contained_native_file_and_mode(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            profile = Path(temporary) / "profile"
            profile.mkdir(mode=0o700)
            (profile / ".agentplugins").mkdir(mode=0o700)
            approved = sealed_tuple("context7")
            native_projection = self.install_projection(profile, "cursor", approved)
            item = {"profile": str(profile), "client_id": "cursor", "native_projection": native_projection}
            self.assertEqual(
                fixed_adapters.verified_native_projection(item, "context7", approved, owner_uid=os.geteuid())["client_id"],
                "cursor",
            )
            with self.assertRaisesRegex(ValueError, "approved tuple"):
                fixed_adapters.verified_native_projection(item, "context7", {**approved, "package_version": "9.9.9"}, owner_uid=os.geteuid())
            native = profile / "context7.native"
            native.chmod(0o644)
            with self.assertRaisesRegex(ValueError, "mode"):
                fixed_adapters.verified_native_projection(item, "context7", approved, owner_uid=os.geteuid())

    def test_kiro_marker_text_never_establishes_tool_evidence(self) -> None:
        marker = "UAP_OBSERVER_OK kiro context7 " + "a" * 64
        self.assertFalse(fixed_adapters.successful_client_evidence(
            "kiro", [marker, {"type": "assistant", "message": marker}],
            "resolve-library-id", "context7", marker,
        ))

    @staticmethod
    def kiro_acp_records(marker: str) -> list[dict[str, Any]]:
        session, call = "opaque-session", "opaque-call"
        update = lambda body: {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": session, "update": body}}
        title = "@cloudflare-docs/search_cloudflare_documentation"
        meta = {"kiro": {"serverName": "cloudflare-docs"}}
        tools = [
            {"name": "kiro_power", "enabled": True},
            {"name": "search_cloudflare_documentation"},
            {"name": "read_file", "enabled": True},
        ]
        return [
            {"jsonrpc": "2.0", "id": 0, "result": {"protocolVersion": 1, "agentCapabilities": {"mcpCapabilities": {"http": True}}}},
            {"jsonrpc": "2.0", "id": 1, "result": {"sessionId": session}},
            update({"sessionUpdate": "_kiro/mcp/status", "status": "connecting", "serverName": "cloudflare-docs", "tools": json.loads(json.dumps(tools))}),
            update({"sessionUpdate": "_kiro/mcp/status", "status": "connected", "serverName": "cloudflare-docs", "tools": json.loads(json.dumps(tools))}),
            update({"sessionUpdate": "tool_call", "status": "pending", "title": title, "toolCallId": call, "_meta": meta}),
            {"jsonrpc": "2.0", "id": "permission-opaque", "method": "session/request_permission", "params": {
                "sessionId": session, "toolCall": {"toolCallId": call, "title": title, "status": "pending"},
                "options": [
                    {"optionId": "once", "name": "Allow once", "kind": "allow_once"},
                    {"optionId": "always", "name": "Allow always", "kind": "allow_always"},
                    {"optionId": "reject", "name": "Reject once", "kind": "reject_once"},
                    {"optionId": "reject-always", "name": "Reject always", "kind": "reject_always"},
                ],
            }},
            update({"sessionUpdate": "tool_call_update", "status": "in_progress", "toolCallId": call, "_meta": meta}),
            update({"sessionUpdate": "tool_call_update", "status": "completed", "title": title, "toolCallId": call,
                    "content": [{"type": "content", "content": {"type": "text", "text": "reviewed result"}}],
                    "rawOutput": {"response": {"content": "reviewed result"}}, "_meta": meta}),
            update({"sessionUpdate": "agent_message_chunk", "content": {"type": "text", "text": marker[:17]}}),
            update({"sessionUpdate": "agent_message_chunk", "content": {"type": "text", "text": marker[17:]}}),
            update({"sessionUpdate": "session_info_update", "status": "success", "_meta": {"kiro": {"kind": "turn_completion"}}}),
            update({"sessionUpdate": "session_info_update", "stopReason": "end_turn", "_meta": {"kiro": {"kind": "turn_end"}}}),
            {"jsonrpc": "2.0", "id": 2, "result": {"stopReason": "end_turn"}},
        ]

    def test_kiro_acp_2200_exact_positive_contract_and_permission_answer(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "a" * 64
        contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
        answer = None
        for record in self.kiro_acp_records(marker):
            candidate = contract.accept(record)
            if candidate is not None:
                answer = candidate
        self.assertTrue(contract.complete())
        self.assertEqual(answer, {"jsonrpc": "2.0", "id": "permission-opaque", "result": {"outcome": {"outcome": "selected", "optionId": "once"}}})

    def test_kiro_acp_2200_reports_client_display_error_before_terminal_order(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "a" * 64
        records = self.kiro_acp_records(marker)
        display_error = {
            "jsonrpc": "2.0", "method": "session/update", "params": {
                "sessionId": "opaque-session", "update": {
                    "sessionUpdate": "session_info_update",
                    "_meta": {"kiro": {
                        "kind": "display_error", "displayError": True,
                        "errorType": "mcp_connection_error", "message": "redacted",
                    }},
                },
            },
        }
        contract = fixed_adapters.KiroACPContract(
            "cloudflare-docs", "search_cloudflare_documentation", marker,
        )
        for record in records[:2]:
            contract.accept(record)
        with self.assertRaisesRegex(ValueError, "client display error"):
            contract.accept(display_error)

    def test_kiro_acp_2200_accepts_current_power_discovery_and_client_target_shape(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "6" * 64
        records = self.kiro_acp_records(marker)
        session = "opaque-session"
        def update(body: dict[str, Any]) -> dict[str, Any]:
            return {
                "jsonrpc": "2.0", "method": "session/update",
                "params": {"sessionId": session, "update": body},
            }
        power = "opaque-power-call"
        power_records = [
            update({
                "sessionUpdate": "tool_call", "status": "in_progress", "title": "Kiro Powers",
                "toolCallId": power, "kind": "other", "rawInput": {"action": "list"},
                "_meta": {"kiro": {"toolOrigin": "default"}},
            }),
            update({
                "sessionUpdate": "tool_call_update", "status": "completed", "title": "Kiro Powers",
                "toolCallId": power,
                "content": [{"type": "content", "content": {"type": "text", "text": "Available powers"}}],
                "rawInput": {"action": "list"}, "rawOutput": {"response": "Available powers"},
                "_meta": {"kiro": {"toolOrigin": "default"}},
            }),
        ]
        records[4:4] = power_records
        target = records[6]["params"]["update"]
        target.update({
            "kind": "other",
            "rawInput": {
                "query": "Cloudflare Durable Objects SQLite storage API",
                "_meta": {"_activePath": [], "_completedPaths": [["query"]], "_isValid": True},
            },
            "_meta": {"kiro": {"serverName": "cloudflare-docs", "toolOrigin": "client"}},
        })
        options = json.loads(json.dumps(records[7]["params"]["options"]))
        interaction = {
            "interactionType": "tool_approval", "options": options,
            "question": "@cloudflare-docs/search_cloudflare_documentation",
            "toolCallId": "opaque-call",
        }
        records.insert(7, update({
            "sessionUpdate": "session_info_update",
            "_meta": {"kiro": {
                "kind": "pending_interaction", **interaction,
                "pendingInteraction": json.loads(json.dumps(interaction)),
            }},
        }))
        records[8]["params"]["_meta"] = {"kiro": {"opaque": True}}
        resolved = {
            "outcome": "selected", "selectedOption": "once", "toolCallId": "opaque-call",
        }
        records.insert(9, update({
            "sessionUpdate": "session_info_update",
            "_meta": {"kiro": {
                "kind": "interaction_resolved", **resolved,
                "interactionResolved": json.loads(json.dumps(resolved)),
            }},
        }))
        current_meta = {"kiro": {"serverName": "cloudflare-docs", "toolOrigin": "client"}}
        current_input = {
            "query": "Cloudflare Durable Objects SQLite storage API",
            "_meta": {"_activePath": [], "_completedPaths": [["query"]], "_isValid": True},
        }
        records[10]["params"]["update"].update({"rawInput": current_input, "_meta": current_meta})
        records.insert(11, json.loads(json.dumps(records[10])))
        records[12]["params"]["update"].update({
            "rawInput": current_input, "_meta": current_meta,
            "rawOutput": {
                "imageBase64Urls": [],
                "message": "Tool completed successfully",
                "response": "Tool completed successfully",
            },
        })
        records[13]["params"]["update"]["_meta"] = {"kiro": {"replayId": "opaque-replay"}}
        records[14]["params"]["update"]["_meta"] = {"kiro": {"replayId": "opaque-replay"}}
        records[15]["params"]["update"] = {
            "sessionUpdate": "session_info_update",
            "_meta": {"kiro": {
                "kind": "turn_completion", "status": "success", "elapsedTime": 1234,
                "promptTurnSummaries": [{
                    "unit": "credit", "unitPlural": "credits", "usage": 0.25,
                    "usedTools": ["kiro_power", "chrome-devtools/list_pages"],
                }],
                "requestIds": ["opaque-request-1", "opaque-request-2"],
            }},
        }
        records[16]["params"]["update"] = {
            "sessionUpdate": "session_info_update",
            "_meta": {"kiro": {
                "kind": "turn_end", "messageId": "opaque-message", "stopReason": "end_turn",
                "turnEnd": {"stopReason": "end_turn"},
            }},
        }
        contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
        answer = None
        for record in records:
            candidate = contract.accept(record)
            if candidate is not None:
                answer = candidate
        self.assertTrue(contract.complete())
        self.assertEqual(contract.power_phase, "completed")
        self.assertEqual(contract.target_shape, "client")
        self.assertEqual(contract.target_in_progress_count, 2)
        self.assertEqual(answer, {"jsonrpc": "2.0", "id": "permission-opaque", "result": {"outcome": {"outcome": "selected", "optionId": "once"}}})

        mutations = (
            lambda row: row["params"]["update"]["_meta"]["kiro"].update(interactionType="question"),
            lambda row: row["params"]["update"]["_meta"]["kiro"].update(toolCallId="foreign"),
            lambda row: row["params"]["update"]["_meta"]["kiro"]["pendingInteraction"].update(question="foreign"),
            lambda row: row["params"]["update"]["_meta"]["kiro"]["options"].reverse(),
        )
        interaction_record = records[7]
        for mutation in mutations:
            candidate = json.loads(json.dumps(records))
            mutation(candidate[7])
            contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
            with self.subTest(mutation=mutation), self.assertRaisesRegex(ValueError, "pending interaction"):
                for record in candidate:
                    contract.accept(record)
        self.assertEqual(interaction_record["params"]["update"]["sessionUpdate"], "session_info_update")

        flow_mutations = (
            lambda rows: rows[6]["params"]["update"]["rawInput"].update(query="foreign"),
            lambda rows: rows[10]["params"]["update"]["rawInput"].update(query="foreign"),
            lambda rows: rows[10]["params"]["update"]["_meta"]["kiro"].update(serverName="foreign"),
            lambda rows: rows.insert(12, json.loads(json.dumps(rows[11]))),
            lambda rows: rows.pop(7),
            lambda rows: rows.insert(8, json.loads(json.dumps(rows[7]))),
            lambda rows: rows[9]["params"]["update"]["_meta"]["kiro"].update(outcome="rejected"),
            lambda rows: rows[9]["params"]["update"]["_meta"]["kiro"].update(selectedOption="always"),
            lambda rows: rows[9]["params"]["update"]["_meta"]["kiro"].update(toolCallId="foreign"),
            lambda rows: rows[9]["params"]["update"]["_meta"]["kiro"]["interactionResolved"].update(outcome="rejected"),
            lambda rows: rows.pop(9),
            lambda rows: rows.insert(10, json.loads(json.dumps(rows[9]))),
            lambda rows: rows[12]["params"]["update"]["rawOutput"].update(response="foreign"),
            lambda rows: rows[12]["params"]["update"]["rawOutput"].update(message="foreign"),
            lambda rows: rows[12]["params"]["update"]["rawOutput"].update(imageBase64Urls=["foreign"]),
            lambda rows: rows[12]["params"]["update"]["rawOutput"].update(response="Error: foreign", message="Error: foreign"),
            lambda rows: rows[12]["params"]["update"]["content"][0]["content"].update(text="Error: foreign"),
            lambda rows: rows[13]["params"]["update"]["_meta"]["kiro"].update(replayId=""),
            lambda rows: rows[14]["params"]["update"]["_meta"]["kiro"].update(replayId="foreign"),
            lambda rows: rows[14]["params"]["update"]["_meta"]["kiro"].update(extra=True),
            lambda rows: rows[15]["params"]["update"]["_meta"]["kiro"].update(status="failed"),
            lambda rows: rows[15]["params"]["update"]["_meta"]["kiro"].update(elapsedTime=-1),
            lambda rows: rows[15]["params"]["update"]["_meta"]["kiro"].update(requestIds=[]),
            lambda rows: rows[15]["params"]["update"]["_meta"]["kiro"]["promptTurnSummaries"][0].update(usedTools=[]),
            lambda rows: rows[16]["params"]["update"]["_meta"]["kiro"].update(stopReason="cancelled"),
            lambda rows: rows[16]["params"]["update"]["_meta"]["kiro"].update(messageId=""),
            lambda rows: rows[16]["params"]["update"]["_meta"]["kiro"]["turnEnd"].update(stopReason="cancelled"),
        )
        for mutation in flow_mutations:
            candidate = json.loads(json.dumps(records))
            mutation(candidate)
            contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
            with self.subTest(flow_mutation=mutation), self.assertRaises(ValueError):
                for record in candidate:
                    contract.accept(record)

    def test_kiro_current_probe_input_binds_every_argument_path(self) -> None:
        self.assertEqual(
            fixed_adapters.kiro_probe_input("context7"),
            {
                "libraryName": "React",
                "query": "React library",
                "_meta": {
                    "_activePath": [],
                    "_completedPaths": [["libraryName"], ["query"]],
                    "_isValid": True,
                },
            },
        )
        self.assertEqual(
            fixed_adapters.kiro_probe_input("notion"),
            {
                "query": "UAP read-only probe",
                "_meta": {
                    "_activePath": ["query"],
                    "_completedPaths": [["query"]],
                    "_isValid": True,
                },
            },
        )

    def test_kiro_acp_secret_store_is_bounded_private_and_atomic(self) -> None:
        key = "kiro.mcp." + "a" * 64 + ".tokens"
        with tempfile.TemporaryDirectory() as temporary:
            home = Path(temporary).resolve() / "profile"
            parent = home / ".state" / "uap-observer"
            parent.mkdir(parents=True, mode=0o700)
            parent.chmod(0o700)
            path = parent / "kiro-acp-secrets.json"
            path.write_text(json.dumps({key: "initial"}))
            path.chmod(0o600)
            store = fixed_adapters.KiroACPSecretStore.from_environment(
                {"HOME": str(home)},
            )
            self.assertIsNotNone(store)
            assert store is not None
            request = lambda identifier, method, params: {
                "jsonrpc": "2.0", "id": identifier, "method": method, "params": params,
            }
            self.assertEqual(
                store.accept(request(1, "_kiro/secret/get", {"key": key}))["result"],
                {"value": "initial"},
            )
            self.assertEqual(
                store.accept(request(2, "_kiro/secret/store", {"key": key, "value": "rotated"}))["result"],
                {},
            )
            self.assertEqual(json.loads(path.read_text()), {key: "rotated"})
            with self.assertRaisesRegex(ValueError, "already in use"):
                fixed_adapters.KiroACPSecretStore(path)
            self.assertEqual(
                store.accept(request(3, "_kiro/secret/delete", {"key": key}))["result"],
                {},
            )
            self.assertEqual(json.loads(path.read_text()), {})
            store.close()
            self.assertIsNone(
                fixed_adapters.KiroACPSecretStore.from_environment(
                    {"HOME": str(Path(temporary) / "missing")},
                ),
            )

    def test_kiro_acp_secret_store_rejects_unscoped_or_malformed_requests(self) -> None:
        key = "kiro.mcp." + "b" * 64 + ".client"
        with tempfile.TemporaryDirectory() as temporary:
            parent = Path(temporary).resolve() / "uap-observer"
            parent.mkdir(mode=0o700)
            path = parent / "kiro-acp-secrets.json"
            path.write_text("{}")
            path.chmod(0o600)
            store = fixed_adapters.KiroACPSecretStore(path)
            malformed = (
                {"jsonrpc": "2.0", "id": True, "method": "_kiro/secret/get", "params": {"key": key}},
                {"jsonrpc": "2.0", "id": 1, "method": "_kiro/secret/get", "params": {"key": "foreign"}},
                {"jsonrpc": "2.0", "id": 1, "method": "_kiro/secret/store", "params": {"key": key}},
                {"jsonrpc": "2.0", "id": 1, "method": "_kiro/secret/delete", "params": {"key": key, "extra": True}},
            )
            for request in malformed:
                with self.subTest(request=request), self.assertRaises(ValueError):
                    store.accept(request)
            store.close()

    def test_kiro_acp_2200_power_discovery_fail_closed(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "5" * 64
        base = self.kiro_acp_records(marker)
        session = "opaque-session"
        def update(body: dict[str, Any]) -> dict[str, Any]:
            return {
                "jsonrpc": "2.0", "method": "session/update",
                "params": {"sessionId": session, "update": body},
            }
        start = update({
            "sessionUpdate": "tool_call", "status": "in_progress", "title": "Kiro Powers",
            "toolCallId": "power", "kind": "other", "rawInput": {"action": "list"},
            "_meta": {"kiro": {"toolOrigin": "default"}},
        })
        completed = update({
            "sessionUpdate": "tool_call_update", "status": "completed", "title": "Kiro Powers",
            "toolCallId": "power",
            "content": [{"type": "content", "content": {"type": "text", "text": "Available powers"}}],
            "rawInput": {"action": "list"}, "rawOutput": {"response": "Available powers"},
            "_meta": {"kiro": {"toolOrigin": "default"}},
        })
        mutations = (
            lambda rows: rows[0]["params"]["update"]["rawInput"].update(action="execute"),
            lambda rows: rows[0]["params"]["update"]["_meta"]["kiro"].update(toolOrigin="client"),
            lambda rows: rows[1]["params"]["update"].update(status="failed"),
            lambda rows: rows[1]["params"]["update"].update(content=[]),
            lambda rows: rows[1]["params"]["update"].update(rawOutput={"response": "error"}),
            lambda rows: rows.append(json.loads(json.dumps(rows[1]))),
        )
        for mutation in mutations:
            power_records = json.loads(json.dumps([start, completed]))
            mutation(power_records)
            records = json.loads(json.dumps(base))
            records[4:4] = power_records
            contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                for record in records:
                    contract.accept(record)

    def test_kiro_acp_2200_accepts_bounded_official_session_new_extensions(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "7" * 64
        records = self.kiro_acp_records(marker)
        records[1]["result"].update({
            "modes": {"currentModeId": "agent", "availableModes": [{"id": "agent"}]},
            "configOptions": [{"id": "model"}],
            "_meta": {"kiro": {"opaque": True}},
        })
        contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
        for record in records:
            contract.accept(record)
        self.assertTrue(contract.complete())

        mutations = (
            lambda result: result.update(extra=True),
            lambda result: result.update(modes=[]),
            lambda result: result.update(modes={"currentModeId": "", "availableModes": []}),
            lambda result: result.update(configOptions=[{}]),
            lambda result: result.update(_meta=[]),
        )
        for mutation in mutations:
            forged = json.loads(json.dumps(records[1]))
            mutation(forged["result"])
            contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
            contract.accept(records[0])
            with self.subTest(mutation=mutation), self.assertRaisesRegex(ValueError, "session/new response differs"):
                contract.accept(forged)

    def test_kiro_acp_2200_adversarial_records_fail_closed(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "b" * 64
        base = self.kiro_acp_records(marker)
        mutations = [
            lambda rows: rows[0].update(id=False),
            lambda rows: rows[0]["result"].update(protocolVersion=True),
            lambda rows: rows[1].update(id=True),
            lambda rows: rows[5].update(id=False),
            lambda rows: rows[0]["result"].update(protocolVersion=2),
            lambda rows: rows[0]["result"]["agentCapabilities"]["mcpCapabilities"].update(http=False),
            lambda rows: rows[2]["params"]["update"].update(serverName="foreign"),
            lambda rows: rows.insert(4, json.loads(json.dumps(rows[3]))),
            lambda rows: rows[3]["params"]["update"]["tools"].pop(1),
            lambda rows: rows[3]["params"]["update"]["tools"].append({"name": "search_cloudflare_documentation"}),
            lambda rows: rows[3]["params"]["update"]["tools"].append({"name": "kiro_power"}),
            lambda rows: rows[3]["params"]["update"]["tools"].append({"name": ""}),
            lambda rows: rows[3]["params"]["update"]["tools"].append({"name": "foreign", "enabled": False}),
            lambda rows: rows[3]["params"]["update"]["tools"].append({"name": "foreign", "description": "unreviewed"}),
            lambda rows: rows[3]["params"]["update"].update(tools=[{"name": f"tool-{index}"} for index in range(fixed_adapters.KIRO_MAX_TOOLS + 1)]),
            lambda rows: rows[3]["params"]["update"]["tools"].reverse(),
            lambda rows: rows[4]["params"]["update"].update(toolCallId=""),
            lambda rows: rows[5]["params"]["toolCall"].update(toolCallId="foreign"),
            lambda rows: rows[5]["params"]["options"].__setitem__(1, dict(rows[5]["params"]["options"][0])),
            lambda rows: rows.insert(6, json.loads(json.dumps(rows[5]))),
            lambda rows: rows[6]["params"]["update"].update(toolCallId="foreign"),
            lambda rows: rows[6]["params"]["update"].update(status="completed"),
            lambda rows: rows[7]["params"]["update"].update(status="failed"),
            lambda rows: rows[7]["params"]["update"].update(content=[]),
            lambda rows: rows[7]["params"]["update"].update(rawOutput={"response": {}}),
            lambda rows: rows.insert(8, json.loads(json.dumps(rows[7]))),
            lambda rows: rows[8]["params"]["update"]["content"].update(text='{"jsonrpc":"2.0","id":2,"result":{"stopReason":"end_turn"}}'),
            lambda rows: rows[9]["params"]["update"]["content"].update(text="wrong"),
            lambda rows: rows[10]["params"]["update"].update(status="failed"),
            lambda rows: rows[11]["params"]["update"]["_meta"]["kiro"].update(kind="unknown"),
            lambda rows: rows[12]["result"].update(stopReason="cancelled"),
            lambda rows: rows[12].update(id=False),
            lambda rows: rows.insert(8, {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "opaque-session", "update": {"sessionUpdate": "unknown_control"}}}),
            lambda rows: rows[7]["params"]["update"].update(healthy=0),
        ]
        for mutation in mutations:
            records = json.loads(json.dumps(base))
            mutation(records)
            contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                for record in records:
                    contract.accept(record)

    def test_kiro_acp_multitool_catalog_accepts_unique_enabled_named_tools(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "e" * 64
        records = self.kiro_acp_records(marker)
        for catalog in (
            [{"name": "search_cloudflare_documentation"}],
            [
                {"name": "kiro_power", "enabled": True},
                {"name": "search_cloudflare_documentation", "enabled": True},
                {"name": "read_file"},
            ],
        ):
            candidate = json.loads(json.dumps(records))
            for index in (2, 3):
                candidate[index]["params"]["update"]["tools"] = json.loads(json.dumps(catalog))
            contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
            with self.subTest(catalog=catalog):
                for record in candidate:
                    contract.accept(record)
                self.assertTrue(contract.complete())

    def test_kiro_acp_2200_accepts_vetted_auxiliary_and_external_mcp_status(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "f" * 64
        records = self.kiro_acp_records(marker)
        tools = [{"name": "search_cloudflare_documentation"}, {"name": "other_tool"}]
        external = [
            {"jsonrpc": "2.0", "method": "_kiro/tools/didChange", "params": {"sessionId": "opaque-session", "tools": []}},
            {"jsonrpc": "2.0", "method": "_kiro/mcp/status", "params": {"sessionId": "opaque-session", "servers": [{"name": "cloudflare-docs", "status": "connecting", "tools": []}]}},
            {"jsonrpc": "2.0", "method": "_kiro/mcp/status", "params": {"sessionId": "opaque-session", "servers": [{"name": "cloudflare-docs", "status": "connected", "tools": tools}]}},
            {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "opaque-session", "update": {"sessionUpdate": "config_option_update", "configOptions": []}}},
        ]
        candidate = [*records[:2], *external, *records[4:]]
        contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
        for record in candidate:
            contract.accept(record)
        self.assertTrue(contract.complete())

        for mutation in (
            lambda rows: rows[2]["params"].update(sessionId="foreign"),
            lambda rows: rows[3]["params"]["servers"][0].update(status="failed"),
            lambda rows: rows[4]["params"]["servers"][0]["tools"].append({"name": "search_cloudflare_documentation"}),
            lambda rows: rows.insert(2, {"jsonrpc": "2.0", "method": "_kiro/unknown", "params": {"sessionId": "opaque-session"}}),
        ):
            forged = json.loads(json.dumps(candidate))
            mutation(forged)
            contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                for record in forged:
                    contract.accept(record)

    def test_kiro_acp_2200_reconciles_equivalent_external_and_session_status(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "9" * 64
        records = self.kiro_acp_records(marker)
        tools = [{"name": "kiro_power"}, {"name": "search_cloudflare_documentation"}]
        for index in (2, 3):
            records[index]["params"]["update"]["tools"] = json.loads(json.dumps(tools))
        external = [
            {"jsonrpc": "2.0", "method": "_kiro/mcp/status", "params": {"sessionId": "opaque-session", "servers": [{"name": "cloudflare-docs", "status": "connecting", "tools": []}]}},
            {"jsonrpc": "2.0", "method": "_kiro/mcp/status", "params": {"sessionId": "opaque-session", "servers": [{"name": "cloudflare-docs", "status": "connecting", "tools": []}]}},
            {"jsonrpc": "2.0", "method": "_kiro/mcp/status", "params": {"sessionId": "opaque-session", "servers": [{"name": "cloudflare-docs", "status": "connecting", "tools": []}]}},
            {"jsonrpc": "2.0", "method": "_kiro/mcp/status", "params": {"sessionId": "opaque-session", "servers": [{"name": "cloudflare-docs", "status": "connected", "tools": tools}]}},
            {"jsonrpc": "2.0", "method": "_kiro/mcp/status", "params": {"sessionId": "opaque-session", "servers": [{"name": "cloudflare-docs", "status": "connected", "tools": tools}]}},
        ]
        candidate = [*records[:2], *external, *records[2:]]
        contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
        for record in candidate:
            contract.accept(record)
        self.assertTrue(contract.complete())
        self.assertEqual(contract.discovery, ["connecting", "connected"])

        flooded = [*records[:2], *[json.loads(json.dumps(external[0])) for _ in range(fixed_adapters.KIRO_MAX_AUXILIARY + 2)]]
        contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
        with self.assertRaisesRegex(ValueError, "auxiliary notification bound exceeded"):
            for record in flooded:
                contract.accept(record)

        unrelated_progress = json.loads(json.dumps(candidate))
        unrelated_progress[3]["params"]["servers"].append({"name": "other", "status": "connecting", "tools": []})
        contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
        for record in unrelated_progress:
            contract.accept(record)
        self.assertTrue(contract.complete())

        for mutation in (
            lambda rows: rows[3]["params"]["servers"][0].update(extra="changed"),
            lambda rows: rows[3]["params"]["servers"][0].update(_meta={"changed": True}),
        ):
            forged = json.loads(json.dumps(candidate))
            mutation(forged)
            contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
            with self.subTest(repeated_external_mutation=mutation), self.assertRaisesRegex(ValueError, "changed outside"):
                for record in forged:
                    contract.accept(record)

        for mutation in (
            lambda rows: rows[6]["params"]["servers"][0].update(extra="changed"),
            lambda rows: rows[7]["params"]["update"]["tools"].reverse(),
        ):
            forged = json.loads(json.dumps(candidate))
            mutation(forged)
            contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                for record in forged:
                    contract.accept(record)

    def test_kiro_acp_2200_accepts_only_typed_bounded_pre_session_auxiliary_updates(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "8" * 64
        records = self.kiro_acp_records(marker)
        stale = {"jsonrpc": "2.0", "method": "session/update", "params": {
            "sessionId": "prior-session",
            "update": {"sessionUpdate": "config_option_update", "configOptions": []},
        }}
        candidate = [records[0], stale, *records[1:]]
        contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
        for record in candidate:
            contract.accept(record)
        self.assertTrue(contract.complete())

        mutations = (
            lambda row: row["params"]["update"].update(sessionUpdate="unknown"),
            lambda row: row["params"].update(sessionId=""),
            lambda row: row["params"].update(update={"sessionUpdate": "config_option_update"}),
            lambda row: row["params"]["update"].update(unexpected={}),
            lambda row: row["params"]["update"].update(configOptions={}),
        )
        for mutation in mutations:
            forged = json.loads(json.dumps(stale))
            mutation(forged)
            contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
            contract.accept(records[0])
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                contract.accept(forged)

        foreign_after_new = json.loads(json.dumps(stale))
        contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
        contract.accept(records[0])
        contract.accept(records[1])
        with self.assertRaisesRegex(ValueError, "session update envelope differs"):
            contract.accept(foreign_after_new)

    def test_kiro_acp_2200_accepts_only_typed_progressive_context_notifications(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "6" * 64
        records = self.kiro_acp_records(marker)
        progressive = {"jsonrpc": "2.0", "method": "_kiro/progressive_context/items_changed", "params": {
            "sessionId": "opaque-session", "status": "success",
            "items": [{"name": "code-tool-router", "type": "skill"}],
        }}
        candidate = [*records[:2], progressive, *records[2:]]
        contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
        for record in candidate:
            contract.accept(record)
        self.assertTrue(contract.complete())

        empty = json.loads(json.dumps(progressive))
        empty["params"]["items"] = []
        candidate = [*records[:2], empty, *records[2:]]
        contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
        for record in candidate:
            contract.accept(record)
        self.assertTrue(contract.complete())

        mutations = (
            lambda params: params.update(status="failed"),
            lambda params: params.update(status="loading", items=[]),
            lambda params: params.update(items=[None]),
            lambda params: params.update(extra=True),
            lambda params: params.update(sessionId="foreign"),
        )
        for mutation in mutations:
            forged = json.loads(json.dumps(progressive))
            mutation(forged["params"])
            contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
            contract.accept(records[0])
            contract.accept(records[1])
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                contract.accept(forged)

    def test_kiro_acp_2200_accepts_context_usage_during_target_execution(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "5" * 64
        records = self.kiro_acp_records(marker)
        context_usage = {"jsonrpc": "2.0", "method": "session/update", "params": {
            "sessionId": "opaque-session", "update": {
                "sessionUpdate": "session_info_update",
                "_meta": {"kiro": {"kind": "context_usage", "usagePercentage": 1.0}},
            },
        }}
        candidate = [*records[:5], context_usage, *records[5:]]
        contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
        for record in candidate:
            contract.accept(record)
        self.assertTrue(contract.complete())

        malformed = json.loads(json.dumps(context_usage))
        malformed["params"]["update"]["status"] = "success"
        contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
        for record in records[:5]:
            contract.accept(record)
        with self.assertRaises(ValueError):
            contract.accept(malformed)

    def test_kiro_acp_2200_accepts_only_exact_nonproof_turn_lifecycle_updates(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "4" * 64
        records = self.kiro_acp_records(marker)
        updates = [
            {"sessionUpdate": "session_info_update", "_meta": {"kiro": {"kind": "user_message_id_assigned", "userMessageId": "opaque-message"}}},
            {"sessionUpdate": "session_info_update", "_meta": {"kiro": {"kind": "turn_start", "turnStart": True, "messageId": "opaque-turn"}}},
            {"sessionUpdate": "session_info_update", "title": "Use the reviewed tool", "_meta": {"kiro": {"kind": "focus_update", "title": "Use the reviewed tool", "focus": {"title": "Use the reviewed tool"}}}},
        ]
        auxiliary = [{"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "opaque-session", "update": update}} for update in updates]
        candidate = [*records[:4], *auxiliary, *records[4:]]
        contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
        for record in candidate:
            contract.accept(record)
        self.assertTrue(contract.complete())

        mutations = (
            lambda rows: rows[0]["params"]["update"]["_meta"]["kiro"].update(userMessageId=""),
            lambda rows: rows[1]["params"]["update"]["_meta"]["kiro"].update(messageId=""),
            lambda rows: rows[1]["params"]["update"]["_meta"]["kiro"].update(turnStart=False),
            lambda rows: rows[2]["params"]["update"].update(title="different"),
        )
        for mutation in mutations:
            forged = json.loads(json.dumps(auxiliary))
            mutation(forged)
            contract = fixed_adapters.KiroACPContract("cloudflare-docs", "search_cloudflare_documentation", marker)
            for record in records[:4]:
                contract.accept(record)
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                for record in forged:
                    contract.accept(record)

    @staticmethod
    def kiro_skill_acp_records(marker: str, skill_path: Path) -> list[dict[str, Any]]:
        session, disclose, search = "skill-session", "disclose-call", "search-call"
        update = lambda body: {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": session, "update": body}}
        skill_meta = {
            "kiro": {
                "toolOrigin": "acp", "toolId": "disclose_context",
                "disclosedContext": {
                    "type": "skill", "displayName": "code-tool-router", "uri": skill_path.as_uri(),
                },
            },
        }
        search_meta = {"kiro": {"toolOrigin": "default", "toolId": "grep_search"}}
        commands = [{
            "name": name, "description": f"fixture {name}",
            "_meta": {"kiro": {
                "type": "skill", "scope": "global", "matched": True,
                "path": str(skill_path if name == "code-tool-router" else skill_path.parent.parent / name / "SKILL.md"),
            }},
        } for name in (
            "code-architecture-map", "code-impact-analysis",
            "code-intelligence-doctor", "code-tool-router",
        )]
        output = f"You searched for ^UAP_SKILL_SECRET_ and received the following results:\nuap-skill-probe.txt\n1:{marker}"
        return [
            {"jsonrpc": "2.0", "id": 0, "result": {"protocolVersion": 1, "agentCapabilities": {"mcpCapabilities": {"http": True}}}},
            {"jsonrpc": "2.0", "id": 1, "result": {"sessionId": session}},
            {"jsonrpc": "2.0", "method": "_kiro/tools/didChange", "params": {"sessionId": session, "tools": []}},
            update({"sessionUpdate": "config_option_update", "configOptions": []}),
            update({"sessionUpdate": "available_commands_update", "availableCommands": commands}),
            update({
                "sessionUpdate": "tool_call", "status": "pending", "title": "Loaded skill: code-tool-router",
                "toolCallId": disclose, "kind": "other", "rawInput": {"name": "code-tool-router"}, "_meta": skill_meta,
            }),
            {"jsonrpc": "2.0", "id": "skill-permission", "method": "session/request_permission", "params": {
                "sessionId": session,
                "toolCall": {"toolCallId": disclose, "title": "Loaded skill: code-tool-router", "status": "pending"},
                "options": [
                    {"optionId": "once", "name": "Allow once", "kind": "allow_once"},
                    {"optionId": "always", "name": "Allow always", "kind": "allow_always"},
                    {"optionId": "reject", "name": "Reject once", "kind": "reject_once"},
                    {"optionId": "reject-always", "name": "Reject always", "kind": "reject_always"},
                ],
            }},
            update({"sessionUpdate": "tool_call_update", "status": "in_progress", "toolCallId": disclose, "_meta": skill_meta}),
            update({
                "sessionUpdate": "tool_call_update", "status": "completed", "toolCallId": disclose,
                "content": [{"type": "content", "content": {"type": "text", "text": "Loaded code-tool-router"}}],
                "rawOutput": {"skill": "code-tool-router", "path": str(skill_path), "success": True}, "_meta": skill_meta,
            }),
            update({
                "sessionUpdate": "tool_call", "status": "pending", "title": "Grep Search",
                "toolCallId": search, "kind": "search",
                "rawInput": {"query": "^UAP_SKILL_SECRET_", "explanation": "Find the hidden fixture marker"},
                "_meta": search_meta,
            }),
            update({"sessionUpdate": "tool_call_update", "status": "in_progress", "toolCallId": search, "_meta": search_meta}),
            update({
                "sessionUpdate": "tool_call_update", "status": "completed", "title": "Grep Search",
                "toolCallId": search, "content": [{"type": "content", "content": {"type": "text", "text": output}}],
                "rawOutput": {"output": output, "success": True}, "_meta": search_meta,
            }),
            update({"sessionUpdate": "agent_message_chunk", "content": {"type": "text", "text": f"`{marker[:24]}"}}),
            update({"sessionUpdate": "agent_message_chunk", "content": {"type": "text", "text": f"{marker[24:]}`"}}),
            update({"sessionUpdate": "session_info_update", "status": "success", "_meta": {"kiro": {"kind": "turn_completion"}}}),
            update({"sessionUpdate": "session_info_update", "stopReason": "end_turn", "_meta": {"kiro": {"kind": "turn_end"}}}),
            {"jsonrpc": "2.0", "id": 2, "result": {"stopReason": "end_turn"}},
        ]

    def test_kiro_acp_2200_skill_positive_contract_uses_hidden_marker(self) -> None:
        marker = "UAP_SKILL_SECRET_" + "a" * 64
        skill_path = Path("/var/lib/uap-observer/profiles/kiro/.kiro/skills/code-tool-router/SKILL.md")
        contract = fixed_adapters.KiroACPSkillContract(marker, skill_path)
        answer = None
        for record in self.kiro_skill_acp_records(marker, skill_path):
            candidate = contract.accept(record)
            if candidate is not None:
                answer = candidate
        self.assertTrue(contract.complete())
        self.assertEqual(answer, {
            "jsonrpc": "2.0", "id": "skill-permission",
            "result": {"outcome": {"outcome": "selected", "optionId": "once"}},
        })

    def test_kiro_acp_2200_skill_reports_client_display_error(self) -> None:
        marker = "UAP_SKILL_SECRET_" + "a" * 64
        skill_path = Path("/var/lib/uap-observer/profiles/kiro/.kiro/skills/code-tool-router/SKILL.md")
        records = self.kiro_skill_acp_records(marker, skill_path)
        display_error = {
            "jsonrpc": "2.0", "method": "session/update", "params": {
                "sessionId": "skill-session", "update": {
                    "sessionUpdate": "session_info_update",
                    "_meta": {"kiro": {
                        "kind": "display_error", "displayError": True,
                        "errorType": "client_error", "message": "redacted",
                    }},
                },
            },
        }
        contract = fixed_adapters.KiroACPSkillContract(marker, skill_path)
        for record in records[:2]:
            contract.accept(record)
        with self.assertRaisesRegex(ValueError, "client display error"):
            contract.accept(display_error)

    def test_kiro_acp_2200_skill_adversarial_records_fail_closed(self) -> None:
        marker = "UAP_SKILL_SECRET_" + "b" * 64
        skill_path = Path("/var/lib/uap-observer/profiles/kiro/.kiro/skills/code-tool-router/SKILL.md")
        base = self.kiro_skill_acp_records(marker, skill_path)
        mutations = (
            lambda rows: rows[0].update(id=False),
            lambda rows: rows[1].update(id=True),
            lambda rows: rows[4]["params"]["update"]["availableCommands"][-1]["_meta"]["kiro"].update(matched=False),
            lambda rows: rows[4]["params"]["update"]["availableCommands"][-1]["_meta"]["kiro"].update(path="/foreign/SKILL.md"),
            lambda rows: rows.insert(5, json.loads(json.dumps(rows[4]))),
            lambda rows: rows[5]["params"]["update"]["rawInput"].update(name="foreign"),
            lambda rows: rows[5]["params"]["update"]["_meta"]["kiro"].update(toolOrigin="default"),
            lambda rows: rows[6]["params"]["toolCall"].update(toolCallId="foreign"),
            lambda rows: rows[6]["params"]["options"][1].update(optionId="once"),
            lambda rows: rows[6].update(id=True),
            lambda rows: rows[8]["params"]["update"]["rawOutput"].update(path="/foreign/SKILL.md"),
            lambda rows: rows[9]["params"]["update"]["rawInput"].update(query="UAP_SKILL_SECRET_"),
            lambda rows: rows[9]["params"]["update"]["_meta"]["kiro"].update(toolOrigin="acp"),
            lambda rows: rows[11]["params"]["update"].update(
                content=[{"type": "content", "content": {"type": "text", "text": "prompt echo"}}],
                rawOutput={"output": "prompt echo", "success": True},
            ),
            lambda rows: rows[11]["params"]["update"].update(status="failed"),
            lambda rows: rows.__setitem__(9, rows[12]),
            lambda rows: rows.insert(12, {"jsonrpc": "2.0", "method": "_kiro/unknown", "params": {"sessionId": "skill-session"}}),
            lambda rows: rows[14]["params"]["update"].update(status="failed"),
            lambda rows: rows[16]["result"].update(stopReason="cancelled"),
            lambda rows: rows[16].update(id=2.0),
        )
        for mutation in mutations:
            records = json.loads(json.dumps(base))
            mutation(records)
            contract = fixed_adapters.KiroACPSkillContract(marker, skill_path)
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                for record in records:
                    contract.accept(record)

    def test_kiro_acp_runner_writes_only_canonical_fixed_requests(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "c" * 64
        records = self.kiro_acp_records(marker)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            executable = root / "kiro-fixture.py"
            executable.write_text(
                "#!/usr/bin/python3\nimport json,sys,time\n"
                f"rows={records!r}\n"
                "seen=[]\n"
                "def request():\n line=sys.stdin.readline(); seen.append(json.loads(line)); return seen[-1]\n"
                "def emit(row): print(json.dumps(row,separators=(',',':')),flush=True)\n"
                "request(); emit(rows[0]); request(); emit(rows[1])\n"
                "for row in rows[2:4]: emit(row)\n"
                "request()\n"
                "for row in rows[4:6]: emit(row)\n"
                "request()\n"
                "open('requests.json','w').write(json.dumps(seen,separators=(',',':')))\n"
                "for row in rows[6:]: emit(row)\n"
                "time.sleep(10)\n"
            )
            executable.chmod(0o755)
            summary, _, _ = fixed_adapters.run_kiro_acp(
                executable, workspace=root, environment={"PATH": str(root)},
                plugin="cloudflare-docs", tool="search_cloudflare_documentation", marker=marker, timeout=3,
            )
            seen = json.loads((root / "requests.json").read_text())
            self.assertEqual([(item.get("id"), item.get("method")) for item in seen[:3]], [(0, "initialize"), (1, "session/new"), (2, "session/prompt")])
            self.assertEqual(seen[0]["params"]["protocolVersion"], 1)
            self.assertNotIn("_meta", seen[0]["params"]["clientCapabilities"])
            self.assertEqual(seen[1]["params"], {"cwd": str(root), "mcpServers": []})
            self.assertEqual(seen[2]["params"]["sessionId"], "opaque-session")
            self.assertEqual(seen[3]["result"]["outcome"], {"outcome": "selected", "optionId": "once"})
            self.assertEqual(summary["target_chain"], ["pending", "in_progress", "completed"])

    def test_kiro_acp_runner_advertises_and_serves_scoped_secret_storage(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "e" * 64
        records = self.kiro_acp_records(marker)
        key = "kiro.mcp." + "d" * 64 + ".tokens"
        secret_request = {
            "jsonrpc": "2.0", "id": 41, "method": "_kiro/secret/get",
            "params": {"key": key},
        }
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve()
            secret_parent = root / ".state" / "uap-observer"
            secret_parent.mkdir(parents=True, mode=0o700)
            secret_parent.chmod(0o700)
            secret_path = secret_parent / "kiro-acp-secrets.json"
            secret_path.write_text(json.dumps({key: "opaque-test-value"}))
            secret_path.chmod(0o600)
            executable = root / "kiro-secret-fixture.py"
            executable.write_text(
                "#!/usr/bin/python3\nimport json,sys,time\n"
                f"rows={records!r}\nsecret={secret_request!r}\nseen=[]\n"
                "def request():\n line=sys.stdin.readline(); seen.append(json.loads(line)); return seen[-1]\n"
                "def emit(row): print(json.dumps(row,separators=(',',':')),flush=True)\n"
                "request(); emit(rows[0]); request(); emit(secret); request(); emit(rows[1])\n"
                "for row in rows[2:4]: emit(row)\n"
                "request()\n"
                "for row in rows[4:6]: emit(row)\n"
                "request()\n"
                "open('secret-requests.json','w').write(json.dumps(seen,separators=(',',':')))\n"
                "for row in rows[6:]: emit(row)\n"
                "time.sleep(10)\n"
            )
            executable.chmod(0o755)
            environment = {"PATH": str(root), "HOME": str(root)}
            fixed_adapters.run_kiro_acp(
                executable, workspace=root, environment=environment,
                plugin="cloudflare-docs", tool="search_cloudflare_documentation",
                marker=marker, timeout=3,
            )
            seen = json.loads((root / "secret-requests.json").read_text())
            self.assertEqual(
                seen[0]["params"]["clientCapabilities"]["_meta"],
                {"kiro": {"secretStorage": True, "openExternalUrl": False}},
            )
            self.assertEqual(seen[2], {
                "jsonrpc": "2.0", "id": 41,
                "result": {"value": "opaque-test-value"},
            })

    def test_kiro_acp_skill_runner_never_places_hidden_marker_in_prompt(self) -> None:
        marker = "UAP_SKILL_SECRET_" + "c" * 64
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            skill_path = root / "profile" / ".kiro" / "skills" / "code-tool-router" / "SKILL.md"
            skill_path.parent.mkdir(parents=True)
            skill_path.write_text("# code-tool-router\n")
            records = self.kiro_skill_acp_records(marker, skill_path)
            executable = root / "kiro-skill-fixture.py"
            executable.write_text(
                "#!/usr/bin/python3\nimport json,sys,time\n"
                f"rows={records!r}\n"
                "seen=[]\n"
                "def request():\n line=sys.stdin.readline(); seen.append(json.loads(line)); return seen[-1]\n"
                "def emit(row): print(json.dumps(row,separators=(',',':')),flush=True)\n"
                "request(); emit(rows[0]); request(); emit(rows[1])\n"
                "for row in rows[2:5]: emit(row)\n"
                "request()\n"
                "for row in rows[5:7]: emit(row)\n"
                "request()\n"
                "open('skill-requests.json','w').write(json.dumps(seen,separators=(',',':')))\n"
                "for row in rows[7:]: emit(row)\n"
                "time.sleep(10)\n"
            )
            executable.chmod(0o755)
            summary, _, _ = fixed_adapters.run_kiro_acp(
                executable, workspace=root, environment={"PATH": str(root)},
                plugin="agent-code-navigator", tool="grep_search", marker=marker,
                skill_path=skill_path, timeout=3,
            )
            seen = json.loads((root / "skill-requests.json").read_text())
            prompt = seen[2]["params"]["prompt"][0]["text"]
            self.assertNotIn(marker, prompt)
            self.assertNotIn("c" * 64, prompt)
            self.assertIn("UAP_SKILL_SECRET_", prompt)
            self.assertEqual(summary["capability"], "skill")
            self.assertEqual(summary["target_chain"], ["disclose", "search", "marker"])

    def test_skill_probe_is_private_exclusive_and_challenge_bound(self) -> None:
        marker = "UAP_SKILL_SECRET_" + "d" * 64
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            root.chmod(0o700)
            probe = fixed_adapters.write_skill_probe(root, marker)
            self.assertEqual(probe.read_text(), marker + "\n")
            self.assertEqual(stat.S_IMODE(probe.stat().st_mode), 0o400)
            with self.assertRaises(FileExistsError):
                fixed_adapters.write_skill_probe(root, marker)

    def test_kiro_acp_successful_prompt_response_is_chunk_independent_boundary(self) -> None:
        marker = "UAP_OBSERVER_OK kiro cloudflare-docs " + "d" * 64
        rows = self.kiro_acp_records(marker)
        session = "opaque-session"
        unrelated_failure = {
            "jsonrpc": "2.0", "method": "session/update", "params": {
                "sessionId": session, "update": {
                    "sessionUpdate": "tool_call_update", "status": "failed",
                    "title": "kiro_power", "toolCallId": "unrelated-power-call",
                    "_meta": {"kiro": {"serverName": "kiro_power"}},
                },
            },
        }
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for mode in ("same_chunk", "next_chunk"):
                workspace = root / mode
                workspace.mkdir()
                executable = workspace / "kiro-fixture.py"
                executable.write_text(
                    "#!/usr/bin/python3\nimport json,os,sys,time\n"
                    f"rows={rows!r}\n"
                    f"failure={unrelated_failure!r}\n"
                    "def request(): return json.loads(sys.stdin.readline())\n"
                    "def line(row): return json.dumps(row,separators=(',',':'))+'\\n'\n"
                    "def emit(row): sys.stdout.write(line(row)); sys.stdout.flush()\n"
                    "request(); emit(rows[0]); request(); emit(rows[1])\n"
                    "for row in rows[2:4]: emit(row)\n"
                    "request()\n"
                    "for row in rows[4:6]: emit(row)\n"
                    "request()\n"
                    "for row in rows[6:-1]: emit(row)\n"
                    + (
                        "os.write(sys.stdout.fileno(),(line(rows[-1])+line(failure)).encode())\n"
                        if mode == "same_chunk" else
                        "emit(rows[-1]); time.sleep(0.25); emit(failure)\n"
                    )
                    + "time.sleep(10)\n"
                )
                executable.chmod(0o755)
                with self.subTest(mode=mode):
                    summary, _, _ = fixed_adapters.run_kiro_acp(
                        executable, workspace=workspace, environment={"PATH": str(workspace)},
                        plugin="cloudflare-docs", tool="search_cloudflare_documentation",
                        marker=marker, timeout=3,
                    )
                    self.assertEqual(summary["turn_end"], "end_turn")

    def test_kiro_acp_runner_rejects_malformed_overflow_timeout_and_early_exit(self) -> None:
        cases = {
            "malformed": "print('{',flush=True)",
            "duplicate_top_level": "print('{\"jsonrpc\":\"2.0\",\"id\":0,\"id\":0,\"result\":{}}',flush=True)",
            "duplicate_nested": "print('{\"jsonrpc\":\"2.0\",\"id\":0,\"result\":{\"protocolVersion\":1,\"agentCapabilities\":{\"mcpCapabilities\":{\"http\":true,\"http\":true}}}}',flush=True)",
            "nan": "print('{\"jsonrpc\":\"2.0\",\"id\":0,\"result\":{\"protocolVersion\":NaN}}',flush=True)",
            "infinity": "print('{\"jsonrpc\":\"2.0\",\"id\":0,\"result\":{\"protocolVersion\":Infinity}}',flush=True)",
            "overflowing_number": "print('{\"jsonrpc\":\"2.0\",\"id\":0,\"result\":{\"protocolVersion\":1e9999}}',flush=True)",
            "overflow": f"import sys;sys.stdout.write('x'*{fixed_adapters.KIRO_MAX_LINE + 1});sys.stdout.flush();time.sleep(10)",
            "timeout": "time.sleep(10)",
            "early": "raise SystemExit(0)",
        }
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for name, body in cases.items():
                executable = root / f"{name}.py"
                executable.write_text(f"#!/usr/bin/python3\nimport time\n{body}\n")
                executable.chmod(0o755)
                with self.subTest(name=name), self.assertRaises((ValueError, TimeoutError)):
                    fixed_adapters.run_kiro_acp(
                        executable, workspace=root, environment={"PATH": str(root)},
                        plugin="cloudflare-docs", tool="search_cloudflare_documentation", marker="marker", timeout=1,
                    )

    def test_native_discovery_rejects_incidental_identity_text(self) -> None:
        self.assertFalse(fixed_adapters.native_discovery_present({"error": {"missing": "context7"}}, "context7"))
        self.assertFalse(fixed_adapters.native_discovery_present({"message": "context7"}, "context7"))
        self.assertTrue(fixed_adapters.native_discovery_present({"servers": [{"name": "context7"}]}, "context7"))

    def test_native_discovery_rejects_false_controls_at_every_envelope_depth(self) -> None:
        approved = sealed_tuple("context7")
        candidate = {"name": "context7", "tuple": approved}
        controls = ("health", "healthy", "readiness", "ready", "connection", "connected", "connectivity", "enabled", "running", "loaded")
        self.assertTrue(fixed_adapters.native_discovery_present(
            {"data": {"inventory": {"servers": [candidate]}}}, "context7", approved,
        ))
        for control in controls:
            envelopes = (
                {control: False, "servers": [candidate]},
                {"data": {control: False, "servers": [candidate]}},
                {"data": {"inventory": {control: False, "servers": [candidate]}}},
                {"servers": [{**candidate, control: False}]},
            )
            for envelope in envelopes:
                with self.subTest(control=control, envelope=envelope):
                    self.assertFalse(fixed_adapters.native_discovery_present(
                        envelope, "context7", approved,
                    ))
        self.assertTrue(fixed_adapters.manager_receipt_present({"products": ["context7"]}, "context7"))
        for field in ("health", "healthy", "readiness", "ready", "connection", "connected", "connectivity", "enabled", "running", "loaded"):
            with self.subTest(negated_control=field):
                self.assertFalse(fixed_adapters.native_discovery_present(
                    {"servers": [{"name": "context7", field: False}]}, "context7",
                ))
        for status in ("disconnected", "not healthy", "not available", "degraded", "cancelled"):
            with self.subTest(negated_state=status):
                self.assertFalse(fixed_adapters.native_discovery_present(
                    {"servers": {"context7": {"status": status}}}, "context7",
                ))

    def test_native_discovery_rejects_zero_controls_recursively_but_allows_nonzero_numbers(self) -> None:
        approved = sealed_tuple("context7")
        candidate = {"name": "context7", "tuple": approved}
        controls = ("health", "healthy", "readiness", "ready", "connection", "connected", "connectivity", "enabled", "running", "loaded")
        for field in controls:
            for zero in (0, 0.0):
                envelope = {"outer": {"inner": {field: zero}}, "servers": [candidate]}
                with self.subTest(field=field, zero=zero):
                    self.assertFalse(fixed_adapters.native_discovery_present(envelope, "context7", approved))
            self.assertTrue(fixed_adapters.native_discovery_present({field: 7, "servers": [candidate]}, "context7", approved))
        for field in ("success", "ok"):
            self.assertTrue(fixed_adapters.explicit_failure_marker({"nested": {field: 0}}))
            self.assertTrue(fixed_adapters.explicit_failure_marker({"nested": {field: False}}))
            self.assertFalse(fixed_adapters.explicit_failure_marker({"nested": {field: 7}}))

    def test_native_discovery_rejects_same_collection_and_cross_depth_mixed_records(self) -> None:
        approved = sealed_tuple("context7")
        good = {"name": "context7", "tuple": approved}
        partial = {"name": "context7"}
        conflict = {"name": "context7", "tuple": {**approved, "package_version": "9.9.9"}}
        for value in (
            {"servers": [good, partial]},
            {"servers": [good, conflict]},
            {"servers": [good, dict(good)]},
            {"servers": [good], "nested": {"connections": [partial]}},
            {"servers": [good], "nested": {"entries": [conflict]}},
            {"servers": [good], "nested": {"mcpServers": [dict(good)]}},
        ):
            with self.subTest(value=value):
                self.assertFalse(fixed_adapters.native_discovery_present(value, "context7", approved))

    def test_native_discovery_rejects_keyed_and_text_identity_contradictions(self) -> None:
        approved = sealed_tuple("context7")
        self.assertFalse(fixed_adapters.native_discovery_present({
            "servers": {"context7": {"name": "attacker", "tuple": approved}},
        }, "context7", approved))
        self.assertFalse(fixed_adapters.native_discovery_present({
            "servers": {"context7": {"name": "attacker", "tuple": approved}},
            "connections": [{"name": "context7", "tuple": approved}],
        }, "context7", approved))
        for client in ("cursor", "kiro"):
            with self.subTest(client=client):
                self.assertFalse(fixed_adapters.native_discovery_output_present(
                    client, ["context7: connected", "context7: failed"], "context7",
                ))

    def test_current_client_command_and_discovery_contracts_are_exact(self) -> None:
        self.assertEqual(
            fixed_adapters.CLIENT_ARGUMENTS["codex"],
            ("exec", "--skip-git-repo-check", "--json", "--ephemeral", "--sandbox", "read-only"),
        )
        self.assertEqual(
            fixed_adapters.CLIENT_ARGUMENTS["cursor"],
            ("--print", "--output-format", "stream-json", "--mode", "ask", "--force", "--sandbox", "enabled", "--trust", "--approve-mcps"),
        )
        self.assertEqual(
            fixed_adapters.CLIENT_ARGUMENTS["kiro"],
            ("acp", "--agent-engine", "v3", "--auth-method", "cli"),
        )
        self.assertEqual(fixed_adapters.CLIENT_DISCOVERY_ARGUMENTS["codex"], ("mcp", "list", "--json"))
        self.assertEqual(fixed_adapters.CLIENT_DISCOVERY_ARGUMENTS["cursor"], ("mcp", "list"))
        self.assertEqual(fixed_adapters.CLIENT_DISCOVERY_ARGUMENTS["kiro"], ("mcp", "list"))

    def test_plain_native_discovery_distinguishes_installed_before_from_healthy_after(self) -> None:
        cursor = fixed_adapters.parsed_native_discovery("cursor", b"context7: connected\nother: connected\n")
        self.assertTrue(fixed_adapters.native_discovery_output_present("cursor", cursor, "context7"))
        self.assertFalse(fixed_adapters.native_discovery_output_present("cursor", ["missing context7"], "context7"))
        self.assertFalse(fixed_adapters.native_discovery_output_present("kiro", ["context7: failed"], "context7"))
        colored = fixed_adapters.parsed_native_discovery("kiro", b"\x1b[32m\xe2\x9c\x93 context7 connected\x1b[0m\n")
        self.assertTrue(fixed_adapters.native_discovery_output_present("kiro", colored, "context7"))
        approval = ["context7: not loaded (needs approval)"]
        self.assertTrue(fixed_adapters.native_discovery_output_present(
            "cursor", approval, "context7", phase="before",
        ))
        self.assertFalse(fixed_adapters.native_discovery_output_present(
            "cursor", approval, "context7", phase="after",
        ))
        for status in ("not loaded", "needs approval", "disconnected", "stopped", "disabled", "failed", "error", "unavailable"):
            with self.subTest(status=status):
                self.assertFalse(fixed_adapters.native_discovery_output_present(
                    "cursor", [f"context7: {status}"], "context7", phase="after",
                ))
        self.assertFalse(fixed_adapters.native_discovery_output_present(
            "cursor", ["context7"], "context7", phase="after",
        ))
        for status in ("not connected", "not ready", "not running", "not enabled", "connected but degraded", "ready (unhealthy)"):
            with self.subTest(explicit_negative=status):
                self.assertFalse(fixed_adapters.native_discovery_output_present(
                    "cursor", [f"context7: {status}"], "context7", phase="after",
                ))
        self.assertFalse(fixed_adapters.native_discovery_output_present(
            "cursor", ["context7: connected (latency 2ms)"], "context7", phase="after",
        ))

    def test_receipt_and_native_identity_require_the_exact_approved_tuple(self) -> None:
        approved = sealed_tuple("context7")
        correct = {"name": "context7", "tuple": dict(approved)}
        wrong = {"name": "context7", "tuple": {**approved, "package_version": "9.9.9"}}
        self.assertTrue(fixed_adapters.manager_receipt_present({"receipts": [correct]}, "context7", approved))
        self.assertFalse(fixed_adapters.manager_receipt_present({"receipts": [wrong]}, "context7", approved))
        self.assertTrue(fixed_adapters.native_discovery_present({"servers": [correct]}, "context7", approved))
        self.assertFalse(fixed_adapters.native_discovery_present({"servers": [wrong]}, "context7", approved))

    def test_client_output_is_killed_at_hard_byte_limit(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            binary = root / "noisy.py"
            binary.write_text("#!/usr/bin/env python3\nimport sys,time\nsys.stdout.write('x'*100000); sys.stdout.flush(); time.sleep(10)\n")
            binary.chmod(0o755)
            original = fixed_adapters.MAX_STDOUT
            fixed_adapters.MAX_STDOUT = 64
            try:
                with self.assertRaisesRegex(ValueError, "size bound"):
                    fixed_adapters.run_client([str(binary)], workspace=root, environment={"PATH": "/usr/bin:/bin"}, timeout=2)
            finally:
                fixed_adapters.MAX_STDOUT = original

    @requires_disposable_observer_host
    def test_fixed_client_argv_uses_disposable_root_and_expected_marker(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            profile, workspace = root / "profile", root / "workspace"
            profile.mkdir(mode=0o700)
            workspace.mkdir(mode=0o700)
            (profile / ".agentplugins").mkdir(mode=0o700)
            receipts = profile / ".agentplugins" / "receipts.json"
            approved = sealed_tuple("context7")
            receipts.write_text(json.dumps({"products": [{"name": "context7", "tuple": approved}]}))
            receipts.chmod(0o600)
            binary = root / "codex-fixture.py"
            binary.write_text(
                "#!/usr/bin/python3\n"
                "import json,sys\n"
                "if sys.argv[1:] == ['--version']: print('codex-fixture-v1')\n"
                f"elif sys.argv[1:] == ['mcp','list','--json']: print(json.dumps({{'servers':[{{'name':'context7','tuple':{approved!r}}}]}}))\n"
                "else:\n"
                " marker=sys.argv[-1].split(': ',1)[-1]\n"
                " events=[{'type':'thread.started','thread_id':'reviewed-thread'},{'type':'turn.started'},{'type':'item.completed','item':{'type':'mcp_tool_call','server':'context7','tool_name':'resolve-library-id','status':'completed','result':{'library':'context7'}}},{'type':'item.completed','item':{'type':'agent_message','text':marker}},{'type':'turn.completed','usage':{'input_tokens':24763,'cached_input_tokens':23744,'output_tokens':122}}]\n"
                " print('\\n'.join(json.dumps(event) for event in events))\n"
            )
            binary.chmod(0o755)
            item = {
                "binary": str(binary), "sha256": "sha256:" + hashlib.sha256(binary.read_bytes()).hexdigest(),
                "profile": str(profile), "client_id": "codex", "native_projection": self.install_projection(profile, "codex", approved),
            }
            with (
                mock.patch.object(fixed_adapters, "verified_executable", return_value=binary),
                mock.patch.object(
                    fixed_adapters, "native_discovery_output_present",
                    wraps=fixed_adapters.native_discovery_output_present,
                ) as discovery_check,
            ):
                marker, argv, _, _ = fixed_adapters.invoke(item, "context7", "codex", "a" * 64, workspace, os.geteuid(), approved)
            self.assertEqual(len(discovery_check.call_args_list), 2)
            self.assertTrue(all(call.args[3] == approved for call in discovery_check.call_args_list))
            self.assertEqual(
                [call.kwargs["phase"] for call in discovery_check.call_args_list],
                ["before", "after"],
            )
            self.assertEqual(marker["client_version"], "codex-fixture-v1")
            self.assertEqual(argv[1:4], ["exec", "--skip-git-repo-check", "--json"])
            self.assertNotIn(str(profile), argv)

    @requires_disposable_observer_host
    def test_fixed_client_rejects_prompt_echo_without_tool_event(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            profile, workspace = root / "profile", root / "workspace"
            profile.mkdir(mode=0o700)
            workspace.mkdir(mode=0o700)
            (profile / ".agentplugins").mkdir(mode=0o700)
            receipts = profile / ".agentplugins" / "receipts.json"
            approved = sealed_tuple("context7")
            receipts.write_text(json.dumps({"products": [{"name": "context7", "tuple": approved}]}))
            receipts.chmod(0o600)
            binary = root / "codex-fixture.py"
            binary.write_text(
                "#!/usr/bin/python3\n"
                "import json,sys\n"
                "if sys.argv[1:] == ['--version']: print('codex-fixture-v1')\n"
                f"elif sys.argv[1:] == ['mcp','list','--json']: print(json.dumps({{'servers':[{{'name':'context7','tuple':{approved!r}}}]}}))\n"
                "else: print(json.dumps(sys.argv[-1].split(': ',1)[-1]))\n"
            )
            binary.chmod(0o755)
            item = {"binary": str(binary), "sha256": "sha256:" + hashlib.sha256(binary.read_bytes()).hexdigest(), "profile": str(profile), "client_id": "codex", "native_projection": self.install_projection(profile, "codex", approved)}
            with (
                mock.patch.object(fixed_adapters, "verified_executable", return_value=binary),
                self.assertRaisesRegex(ValueError, "successful exact tool invocation"),
            ):
                fixed_adapters.invoke(item, "context7", "codex", "a" * 64, workspace, os.geteuid(), approved)

    @requires_disposable_observer_host
    def test_cursor_invocation_uses_stream_json_and_requires_healthy_after_approval(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            profile, workspace = root / "profile", root / "workspace"
            profile.mkdir(mode=0o700)
            workspace.mkdir(mode=0o700)
            (profile / ".agentplugins").mkdir(mode=0o700)
            receipts = profile / ".agentplugins" / "receipts.json"
            approved = sealed_tuple("context7")
            receipts.write_text(json.dumps({"products": [{"name": "context7", "tuple": approved}]}))
            receipts.chmod(0o600)
            binary = root / "cursor-fixture.py"
            binary.write_text(
                "#!/usr/bin/python3\n"
                "import json,os,sys\n"
                "state=os.path.join(os.environ['HOME'],'approved')\n"
                "if sys.argv[1:] == ['--version']: print('cursor-fixture-v1')\n"
                "elif sys.argv[1:] == ['mcp','list']:\n"
                " print('context7: connected' if os.path.exists(state) else 'context7: not loaded (needs approval)')\n"
                "else:\n"
                " assert sys.argv[1:4] == ['--print','--output-format','stream-json']\n"
                " open(state,'w').close()\n"
                " prompt=sys.argv[-1]\n"
                " marker=prompt.rsplit(': ',1)[-1]\n"
                " session='cursor-session'\n"
                " def thinking(sequence):\n"
                "  return [{'type':'thinking','subtype':'delta','session_id':session,'text':'reviewed read-only step','timestamp_ms':sequence},{'type':'thinking','subtype':'completed','session_id':session,'timestamp_ms':sequence+1}]\n"
                " def pair(kind,started,completed,sequence):\n"
                "  key=kind+'ToolCall'; call='call-'+str(sequence); model='model-'+str(sequence); tool_id='tool-'+str(sequence); common={'hookAdditionalContexts':[],'startedAtMs':str(sequence),'toolCallId':tool_id}; outer={'type':'tool_call','session_id':session,'model_call_id':model,'call_id':call,'timestamp_ms':sequence}; return [{**outer,'subtype':'started','tool_call':{**common,key:started}},{**outer,'subtype':'completed','timestamp_ms':sequence+1,'tool_call':{**common,'completedAtMs':str(sequence+1),key:completed}}]\n"
                " events=[{'type':'system','subtype':'init','session_id':session,'apiKeySource':'login','cwd':os.getcwd(),'model':'Auto','permissionMode':'default'},{'type':'user','session_id':session,'message':{'role':'user','content':[{'type':'text','text':prompt}]}},*thinking(1)]\n"
                " discovery_args={'server':'context7','toolCallId':'tool-10','toolName':'resolve-library-id'}\n"
                " description=json.dumps({'mode':'single_tool','namespace':'context7','namespaceStatus':'ready','tool':{'tool':'resolve-library-id'}})\n"
                " events+=pair('getMcpTools',{'args':discovery_args},{'args':discovery_args,'result':{'success':{'content':description}}},10)+thinking(12)\n"
                " target_args={'args':{'libraryName':'React','query':'React'},'name':'context7-resolve-library-id','providerIdentifier':'context7','serverIdentifier':'context7','skipApproval':False,'smartModeApprovalOnly':False,'toolCallId':'tool-20','toolName':'resolve-library-id'}\n"
                " events+=pair('mcp',{'args':target_args,'description':'Resolve React'},{'args':target_args,'description':'Resolve React','result':{'success':{'content':[{'text':{'text':'resolved'}}],'isError':False}}},20)+thinking(22)\n"
                " events+=[{'type':'assistant','session_id':session,'message':{'role':'assistant','content':[{'type':'text','text':marker}]}},{'type':'result','subtype':'success','session_id':session,'request_id':'request-1','duration_ms':10,'duration_api_ms':9,'is_error':False,'result':'preface'+marker,'usage':{'cacheReadTokens':1,'cacheWriteTokens':0,'inputTokens':2,'outputTokens':3}}]\n"
                " print('\\n'.join(json.dumps(event) for event in events))\n"
            )
            binary.chmod(0o755)
            item = {
                "binary": str(binary), "sha256": "sha256:" + hashlib.sha256(binary.read_bytes()).hexdigest(),
                "profile": str(profile), "client_id": "cursor", "native_projection": self.install_projection(profile, "cursor", approved),
            }
            with mock.patch.object(fixed_adapters, "verified_executable", return_value=binary):
                marker, argv, _, _ = fixed_adapters.invoke(
                    item, "context7", "cursor", "a" * 64, workspace, os.geteuid(), approved,
                )
            self.assertEqual(marker["client_version"], "cursor-fixture-v1")
            self.assertEqual(argv[1:4], ["--print", "--output-format", "stream-json"])

    @requires_disposable_observer_host
    def test_profile_sealer_derives_receipt_and_projection_from_manager_info_and_native_files(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            seed, add_dir, info_dir, doctor_dir = root / "seed", root / "add", root / "info", root / "doctor"
            seed.mkdir(mode=0o700)
            add_dir.mkdir(mode=0o700)
            info_dir.mkdir(mode=0o700)
            doctor_dir.mkdir(mode=0o700)
            mapping, matrix = {}, []
            for plugin in sorted(fixed_adapters.HEROES):
                relative = (
                    ".cursor/skills/code-tool-router/SKILL.md"
                    if plugin == "agent-code-navigator" else ".cursor/mcp.json"
                )
                mapping[plugin] = relative
                native = seed / relative
                native.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
                native.write_text(
                    "# code-tool-router\n" if plugin == "agent-code-navigator"
                    else json.dumps({
                        "plugin": plugin,
                        "mcpServers": {"chrome-devtools": {
                            "command": "node",
                            "args": [
                                "/profile/launcher.mjs", "--no-usage-statistics",
                                *fixed_adapters.CHROME_RUNTIME_ARGUMENTS,
                            ],
                        }},
                    })
                )
                native.chmod(0o600)
                approved = {
                    "product_id": plugin, "package_version": "1.0.0",
                    "distribution_id": f"reviewed/{plugin}", "release_sequence": 1,
                    "distribution_kind": "upstream", "snapshot_sequence": 1,
                    "tree_digest": "sha256:" + "a" * 64,
                    "manifest_digest": "sha256:" + "b" * 64,
                    "snapshot_digest": "sha256:" + "d" * 64,
                    "binary_digest": "sha256:" + "e" * 64,
                    "source_repository": f"upstream/{plugin}",
                    "source_revision": "c" * 40, "source_path": f"plugins/{plugin}",
                    "dependency_identity": "locked", "installer_version": "0.1.18",
                    "adapter_version": "r14d", "client_version": None,
                    "os": "linux", "architecture": "x86_64",
                    "observed_at": "2026-08-26T00:00:00Z",
                }
                matrix.append({"plugin": plugin, "client": "cursor", "tuple": approved})
                (add_dir / f"{plugin}.json").write_text(json.dumps({
                    "schema_version": 1, "command": "add", "result": "success",
                    "data": {
                        "status": "completed", "plugin": plugin,
                        "source": f"upstream/{plugin}//plugins/{plugin}",
                        "revision": "c" * 40, "failed": 0,
                        "targets": [{"target": "cursor", "status": "external_completed",
                                     "output": {"result": {
                                         "plan": {
                                             "client_id": "cursor",
                                             "status": "manual_activation_required",
                                             "activation": "manual_activation_required",
                                         },
                                         "activation": {
                                             "activation": "active", "authentication": "not_checked",
                                             "policy": "allowed", "verification": "installation_verified",
                                         },
                                         "requires_confirmation": False,
                                         "group_phase": "external_completed",
                                     }}}],
                    },
                }))
                (info_dir / f"{plugin}.json").write_text(json.dumps({
                    "schema_version": 1, "command": "info", "result": "success",
                    "data": {"name": plugin, "source": f"upstream/{plugin}//plugins/{plugin}", "clients": [{
                        "client_id": "cursor",
                        "scope": "user", "materialization": "materialized", "activation": "active",
                        "policy": "allowed", "verification": "installation_verified",
                        "receipt_reconciled": True, "native_discovery_reconciled": True,
                        "native_identity_state": "managed",
                        "package_revision": {
                            "version": "1.0.0", "resolved_revision": "c" * 40,
                            "tree_digest": "sha256:" + "a" * 64,
                            "manifest_digest": "sha256:" + "b" * 64,
                        },
                    }], "mixed_version": False},
                }))
            doctor = {
                "schema_version": 1, "command": "doctor", "result": "success", "data": {
                    "clients": [{"client_id": "cursor", "detected": True}],
                    "inventory": [{
                        "plugin": row["plugin"],
                        "source": f'{row["tuple"]["source_repository"]}//{row["tuple"]["source_path"]}',
                        "revision": row["tuple"]["source_revision"], "status": "completed",
                    } for row in matrix],
                },
            }
            for plugin in sorted(fixed_adapters.HEROES):
                (doctor_dir / f"{plugin}.json").write_text(json.dumps(doctor))
            matrix_file = root / "matrix.json"
            matrix_file.write_text(json.dumps({"matrix": matrix}))
            native_map = root / "native-map.json"
            native_map.write_text(json.dumps(mapping))
            helper = Path(__file__).parents[2] / "deploy" / "uap-observer-seal-profile.py"
            common = [
                "/usr/bin/python3", str(helper), "--client", "cursor",
                "--root-owned-seed", str(seed), "--matrix-file", str(matrix_file),
                "--manager-add-directory", str(add_dir),
                "--manager-info-directory", str(info_dir),
                "--post-doctor-directory", str(doctor_dir),
                "--native-config-map", str(native_map),
            ]
            projection_digest = subprocess.run(
                [*common, "--digest-only"], check=True, text=True, stdout=subprocess.PIPE,
            ).stdout.strip()
            original_bodies = {
                "add": (add_dir / f'{matrix[0]["plugin"]}.json').read_bytes(),
                "info": (info_dir / f'{matrix[0]["plugin"]}.json').read_bytes(),
                "doctor": (doctor_dir / f'{matrix[0]["plugin"]}.json').read_bytes(),
            }
            strict_mutations = {
                "add": original_bodies["add"].replace(b'"failed": 0', b'"failed": 1, "failed": 0', 1),
                "info": original_bodies["info"].replace(b'"command": "info"', b'"command": "failure", "command": "info"', 1),
                "doctor": original_bodies["doctor"].replace(b'"result": "success"', b'"result": "failure", "result": "success"', 1),
            }
            evidence_paths = {
                "add": add_dir / f'{matrix[0]["plugin"]}.json',
                "info": info_dir / f'{matrix[0]["plugin"]}.json',
                "doctor": doctor_dir / f'{matrix[0]["plugin"]}.json',
            }
            for label, encoded in strict_mutations.items():
                evidence_paths[label].write_bytes(encoded)
                with self.subTest(duplicate_family=label), self.assertRaises(subprocess.CalledProcessError):
                    subprocess.run([*common, "--digest-only"], check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                evidence_paths[label].write_bytes(original_bodies[label])
            for label, path in evidence_paths.items():
                value = json.loads(original_bodies[label])
                value["data"]["nonfinite_probe"] = float("nan")
                path.write_text(json.dumps(value))
                with self.subTest(nonfinite_family=label), self.assertRaises(subprocess.CalledProcessError):
                    subprocess.run([*common, "--digest-only"], check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                path.write_bytes(original_bodies[label])
            for label, path in (
                ("add", add_dir / f'{matrix[0]["plugin"]}.json'),
                ("info", info_dir / f'{matrix[0]["plugin"]}.json'),
                ("doctor", doctor_dir / f'{matrix[0]["plugin"]}.json'),
            ):
                changed = json.loads(original_bodies[label])
                changed["data"]["capture_nonce"] = f"different-{label}"
                path.write_text(json.dumps(changed))
                changed_digest = subprocess.run(
                    [*common, "--digest-only"], check=True, text=True, stdout=subprocess.PIPE,
                ).stdout.strip()
                self.assertNotEqual(changed_digest, projection_digest, label)
                path.write_bytes(original_bodies[label])
            config = root / "adapter.json"
            config.write_text(json.dumps({
                "matrix": matrix, "clients": {"cursor": {"native_projection": {
                    "path": "/var/lib/uap-observer/proofs/cursor/native-projection.json",
                    "sha256": projection_digest,
                }}},
            }))
            protected_add = add_dir / f'{matrix[0]["plugin"]}.json'
            changed_add = json.loads(protected_add.read_text())
            changed_add["data"]["capture_nonce"] = "different-after-config-freeze"
            protected_add.write_text(json.dumps(changed_add))
            with self.assertRaises(subprocess.CalledProcessError):
                subprocess.run(
                    [*common, "--adapter-config", str(config)], check=True,
                    text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                )
            protected_add.write_bytes(original_bodies["add"])
            failed = subprocess.run(
                [*common, "--adapter-config", str(config)],
                env={**os.environ, "UAP_OBSERVER_SEAL_FAILPOINT": "after-staging"},
                text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            )
            self.assertNotEqual(failed.returncode, 0)
            self.assertFalse((seed / ".uap-observer-proof").exists())
            self.assertEqual(
                list(seed.glob(".uap-observer-proof.stage.*")), [],
                "failed sealing left an unpublished proof stage",
            )
            ready_read, ready_write = os.pipe()
            resume_read, resume_write = os.pipe()
            pinned_seed, attacker_seed = root / "seed-pinned", root / "attacker-seed"
            attacker_seed.mkdir(mode=0o700)
            race_environment = {
                **os.environ,
                "UAP_OBSERVER_SEAL_TEST_RACE_POINT": "before-publication",
                "UAP_OBSERVER_SEAL_TEST_READY_FD": str(ready_write),
                "UAP_OBSERVER_SEAL_TEST_RESUME_FD": str(resume_read),
            }
            raced = subprocess.Popen(
                [*common, "--adapter-config", str(config)], env=race_environment,
                pass_fds=(ready_write, resume_read), text=True,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            )
            os.close(ready_write); os.close(resume_read)
            try:
                self.assertEqual(os.read(ready_read, 1), b"1")
                seed.rename(pinned_seed)
                seed.symlink_to(attacker_seed, target_is_directory=True)
                os.write(resume_write, b"1")
                _stdout, _stderr = raced.communicate(timeout=10)
                self.assertNotEqual(raced.returncode, 0)
                self.assertFalse((attacker_seed / ".uap-observer-proof").exists())
                self.assertFalse((pinned_seed / ".uap-observer-proof").exists())
                self.assertEqual(list(pinned_seed.glob(".uap-observer-proof.stage.*")), [])
            finally:
                os.close(ready_read); os.close(resume_write)
                if raced.poll() is None:
                    raced.kill(); raced.wait()
                if seed.is_symlink():
                    seed.unlink()
                if pinned_seed.exists():
                    pinned_seed.rename(seed)
            completed = subprocess.run([
                *common, "--adapter-config", str(config),
            ], check=True, text=True, stdout=subprocess.PIPE)
            projection = seed / ".uap-observer-proof" / "native-projection.json"
            receipts = seed / ".uap-observer-proof" / "receipts.json"
            self.assertEqual(completed.stdout.strip(), fixed_adapters.sha256(projection.read_bytes()))
            self.assertEqual(stat.S_IMODE(projection.stat().st_mode), 0o400)
            self.assertEqual(stat.S_IMODE(receipts.stat().st_mode), 0o400)
            value = json.loads(projection.read_text())
            self.assertEqual(value["schema_version"], 2)
            self.assertEqual({entry["plugin"] for entry in value["entries"]}, fixed_adapters.HEROES)
            self.assertEqual(
                {entry["plugin"]: entry["component_kind"] for entry in value["entries"]},
                {plugin: ("skill" if plugin == "agent-code-navigator" else "mcp") for plugin in fixed_adapters.HEROES},
            )
            self.assertTrue(all(entry["native_config"]["path"].endswith(f'/{entry["plugin"]}.blob') for entry in value["entries"]))
            self.assertTrue(all(entry["native_config"]["path"].startswith("/var/lib/uap-observer/proofs/cursor/native/") for entry in value["entries"]))
            self.assertTrue(all(entry["client_config"]["path"].startswith("/var/lib/uap-observer/profiles/cursor/") for entry in value["entries"]))
            cursor_mcp_paths = {
                entry["client_config"]["path"]
                for entry in value["entries"] if entry["component_kind"] == "mcp"
            }
            self.assertEqual(cursor_mcp_paths, {"/var/lib/uap-observer/profiles/cursor/.cursor/mcp.json"})
            receipt_value = json.loads(receipts.read_text())
            self.assertTrue(fixed_adapters.receipt_binds_projection(receipt_value, value))
            forged_receipt = json.loads(json.dumps(receipt_value))
            forged_receipt["receipts"][0]["manager_add_sha256"] = "sha256:" + "0" * 64
            self.assertFalse(fixed_adapters.receipt_binds_projection(forged_receipt, value))

            specification = importlib.util.spec_from_file_location("profile_sealer", helper)
            self.assertIsNotNone(specification)
            self.assertIsNotNone(specification.loader)
            sealer = importlib.util.module_from_spec(specification)
            specification.loader.exec_module(sealer)
            approved = matrix[0]["tuple"]
            sealer.validate_approved_tuple(approved, matrix[0]["plugin"])
            for extra in (
                {"revision": approved["source_revision"]},
                {"source": f'{approved["source_repository"]}//{approved["source_path"]}'},
                {"SourceRevision": approved["source_revision"]},
                {"source-revision": approved["source_revision"]},
                {"nested": {"source_revision": approved["source_revision"]}},
            ):
                with self.subTest(approved_tuple_extra=extra), self.assertRaisesRegex(ValueError, "approved source tuple"):
                    sealer.validate_approved_tuple({**approved, **extra}, matrix[0]["plugin"])
            manager_add = json.loads((add_dir / f'{matrix[0]["plugin"]}.json').read_text())
            manager_info = json.loads((info_dir / f'{matrix[0]["plugin"]}.json').read_text())
            sealer.matching_add(manager_add, matrix[0]["plugin"], "cursor", approved)
            sealer.matching_client(manager_info, matrix[0]["plugin"], "cursor", approved)
            approved_inventory = {row["plugin"]: row["tuple"] for row in matrix}
            sealer.matching_doctor(doctor, "cursor", approved_inventory)
            for mutation in (
                lambda value: value["data"]["clients"].append({"client_id": "kiro", "detected": True}),
                lambda value: value["data"]["inventory"].pop(),
                lambda value: value["data"]["inventory"][0].update(revision="d" * 40),
                lambda value: value["data"]["inventory"][0].update(product_id="notion"),
                lambda value: value["data"]["inventory"][0].update(name=False),
                lambda value: value["data"]["inventory"][0].update(ProductId=value["data"]["inventory"][0]["plugin"]),
                lambda value: value["data"]["inventory"][0].update(**{"product-id": value["data"]["inventory"][0]["plugin"]}),
                lambda value: value["data"]["inventory"][0].update(nested={"name": "notion", "source_revision": "c" * 40}),
                lambda value: value["data"]["inventory"][0].update(nested={"name": "unreviewed", "source_revision": "c" * 40}),
                lambda value: value["data"]["inventory"][0].update(nested={"source_revision": "c" * 40, "productId": value["data"]["inventory"][0]["plugin"]}),
                lambda value: value["data"]["inventory"][0].update(healthy=False),
                lambda value: value.update(result="failure"),
            ):
                invalid_doctor = json.loads(json.dumps(doctor))
                mutation(invalid_doctor)
                with self.assertRaises(ValueError):
                    sealer.matching_doctor(invalid_doctor, "cursor", approved_inventory)
            tuple_doctor = json.loads(json.dumps(doctor))
            tuple_doctor["data"]["inventory"][0]["tuple"] = approved_inventory[tuple_doctor["data"]["inventory"][0]["plugin"]]
            sealer.matching_doctor(tuple_doctor, "cursor", approved_inventory)
            for mutation in (
                lambda value: value["data"]["inventory"][0].update(source="attacker/fork"),
                lambda value: value["data"]["inventory"][0].update(revision="d" * 40),
                lambda value: value["data"]["inventory"][0].update(source_revision="d" * 40),
            ):
                invalid_doctor = json.loads(json.dumps(tuple_doctor))
                mutation(invalid_doctor)
                with self.assertRaisesRegex(ValueError, "source tuple"):
                    sealer.matching_doctor(invalid_doctor, "cursor", approved_inventory)
            for field in ("success", "ok"):
                for rejected in (False, 0, 0.0):
                    with self.subTest(field=field, rejected=rejected):
                        self.assertTrue(sealer.prohibited_lifecycle_state({"nested": {field: rejected}}))
                for accepted in (True, 1, -1, 0.5):
                    with self.subTest(field=field, accepted=accepted):
                        self.assertFalse(sealer.prohibited_lifecycle_state({"nested": {field: accepted}}))
            for status in ("success", "completed", "external_completed"):
                accepted = json.loads(json.dumps(manager_add))
                accepted["data"]["targets"][0]["status"] = status
                sealer.matching_add(accepted, matrix[0]["plugin"], "cursor", approved)
            for mutation in (
                lambda value: value["data"]["targets"][0]["output"]["result"]["activation"].update(activation="manual_activation_required"),
                lambda value: value["data"]["targets"][0]["output"]["result"].update(requires_confirmation=True),
                lambda value: value["data"]["targets"][0].update(manual_activation_required=True),
                lambda value: value["data"].update(partial=True),
                lambda value: value["data"].update(warnings=["policy_suspended"]),
                lambda value: value["data"].update(events=[{"status": "failed"}]),
                lambda value: value["data"].update(audit={"success": False}),
                lambda value: value["data"].update(cancellations=1),
            ):
                incomplete = json.loads(json.dumps(manager_add))
                mutation(incomplete)
                with self.assertRaisesRegex(ValueError, "prohibited lifecycle"):
                    sealer.matching_add(incomplete, matrix[0]["plugin"], "cursor", approved)
            controls = ("health", "healthy", "readiness", "ready", "connection", "connected", "connectivity", "enabled", "running", "loaded")
            for control in controls:
                for zero in (0, 0.0):
                    incomplete = json.loads(json.dumps(manager_add))
                    incomplete["data"]["targets"][0]["nested"] = {"deeper": {control: zero}}
                    with self.subTest(control=control, zero=zero), self.assertRaisesRegex(ValueError, "prohibited lifecycle"):
                        sealer.matching_add(incomplete, matrix[0]["plugin"], "cursor", approved)
                accepted = json.loads(json.dumps(manager_add))
                accepted["data"]["targets"][0]["nested"] = {"deeper": {control: 7}}
                sealer.matching_add(accepted, matrix[0]["plugin"], "cursor", approved)
            for spelling in ("manual_activation_required", "manual-activation-required", "manual activation required"):
                for location in ("key", "value"):
                    incomplete = json.loads(json.dumps(manager_add))
                    nested = incomplete["data"]["targets"][0].setdefault("nested", {})
                    if location == "key":
                        nested[spelling] = True
                    else:
                        nested["state"] = spelling
                    with self.subTest(spelling=spelling, location=location), self.assertRaisesRegex(ValueError, "prohibited lifecycle"):
                        sealer.matching_add(incomplete, matrix[0]["plugin"], "cursor", approved)
            for spelling in ("MANUAL_ACTIVATION_REQUIRED", "Manual-Activation-Required", "Manual Activation Required"):
                incomplete = json.loads(json.dumps(manager_add))
                incomplete["data"]["targets"][0]["nested"] = {"State": spelling}
                with self.subTest(cased_spelling=spelling), self.assertRaisesRegex(ValueError, "prohibited lifecycle"):
                    sealer.matching_add(incomplete, matrix[0]["plugin"], "cursor", approved)
            for mutation, message in (
                (lambda value: value["data"].pop("revision"), "canonical source"),
                (lambda value: value["data"].update(revision="d" * 40), "canonical source"),
                (lambda value: value["data"].update(source="attacker/fork//plugins/package"), "canonical source"),
                (lambda value: value["data"].update(source_revision="d" * 40), "revision alias"),
                (lambda value: value["data"]["targets"][0]["output"].update(resolved_revision="d" * 40), "revision alias"),
                (lambda value: value["data"]["targets"][0].update(package_revision="d" * 40), "package_revision authority"),
                (lambda value: value["data"].update(Revision="d" * 40), "unexpected revision authority"),
                (lambda value: value["data"]["targets"][0].update(sourceRevision="d" * 40), "unexpected revision authority"),
                (lambda value: value["data"].update(SOURCEREVISION="d" * 40), "unexpected revision authority"),
                (lambda value: value["data"].update(**{"source-repository": "attacker/fork"}), "unexpected source authority"),
                (lambda value: value["data"].update(PACKAGEREVISION={}), "unexpected revision authority"),
                (lambda value: value["data"].update(failed=False), "canonical source"),
            ):
                forged = json.loads(json.dumps(manager_add))
                mutation(forged)
                with self.assertRaisesRegex(ValueError, message):
                    sealer.matching_add(forged, matrix[0]["plugin"], "cursor", approved)
            for mutation, message in (
                (lambda value: value["data"]["clients"][0]["package_revision"].pop("resolved_revision"), "approved release tuple"),
                (lambda value: value["data"]["clients"][0]["package_revision"].update(resolved_revision="d" * 40), "approved release tuple"),
                (lambda value: value["data"].update(source="attacker/fork//plugins/package"), "package source"),
                (lambda value: value["data"].update(revision="d" * 40), "revision alias"),
                (lambda value: value["data"]["clients"][0].update(source_revision="d" * 40), "revision alias"),
                (lambda value: value["data"]["clients"][0].update(package_revision="d" * 40), "approved release tuple"),
                (lambda value: value["data"]["clients"][0].update(nested={"source": "attacker/fork//plugins/package"}), "source authority"),
                (lambda value: value["data"]["clients"][0].update(nested={"Source": "attacker/fork//plugins/package"}), "unexpected source authority"),
                (lambda value: value["data"]["clients"][0].update(nested={"SOURCEREPOSITORY": "attacker/fork"}), "unexpected source authority"),
                (lambda value: value["data"]["clients"][0].update(nested={"packageRevision": "d" * 40}), "unexpected revision authority"),
                (lambda value: value["data"]["clients"][0].update(nested={"source_revision": approved["source_revision"], "SOURCEREVISION": approved["source_revision"]}), "unexpected revision authority"),
            ):
                forged = json.loads(json.dumps(manager_info))
                mutation(forged)
                with self.assertRaisesRegex(ValueError, message):
                    sealer.matching_client(forged, matrix[0]["plugin"], "cursor", approved)
            for mutation, message in (
                (lambda value: value["data"]["inventory"][0].update(package_revision="d" * 40), "package_revision authority"),
                (lambda value: value["data"]["inventory"][0].update(resolved_revision="d" * 40), "revision alias"),
                (lambda value: value["data"]["inventory"][0].update(Revision="d" * 40), "unexpected revision authority"),
                (lambda value: value["data"]["inventory"][0].update(sourceRevision="d" * 40), "unexpected revision authority"),
                (lambda value: value["data"]["inventory"][0].update(SOURCEREVISION="d" * 40), "unexpected revision authority"),
                (lambda value: value["data"]["inventory"][0].update(**{"source-revision": "d" * 40}), "unexpected revision authority"),
                (lambda value: value["data"]["inventory"][0].update(PACKAGEREVISION="d" * 40), "unexpected revision authority"),
                (lambda value: value["data"]["inventory"][0].update(nested={"deeper": {"Revision": "c" * 40}}), "unexpected revision authority"),
                (lambda value: value["data"]["inventory"][0].update(nested={"deeper": {"source-repository": approved["source_repository"]}}), "unexpected source authority"),
            ):
                forged = json.loads(json.dumps(doctor))
                mutation(forged)
                with self.assertRaisesRegex(ValueError, message):
                    sealer.matching_doctor(forged, "cursor", approved_inventory)
            for impostor in (True, 1, 1.0):
                forged_info = json.loads(json.dumps(manager_info))
                forged_info["data"]["clients"][0]["nested"] = {"source_revision": impostor}
                forged_doctor = json.loads(json.dumps(doctor))
                forged_doctor["data"]["inventory"][0]["nested"] = {"source_revision": impostor}
                with self.subTest(info_exact_type_impostor=impostor), self.assertRaisesRegex(ValueError, "revision alias"):
                    sealer.matching_client(forged_info, matrix[0]["plugin"], "cursor", approved)
                with self.subTest(doctor_exact_type_impostor=impostor), self.assertRaisesRegex(ValueError, "revision alias"):
                    sealer.matching_doctor(forged_doctor, "cursor", approved_inventory)
            for boolean in (True, False):
                forged_add = json.loads(json.dumps(manager_add)); forged_add["schema_version"] = boolean
                forged_info = json.loads(json.dumps(manager_info)); forged_info["schema_version"] = boolean
                forged_doctor = json.loads(json.dumps(doctor)); forged_doctor["schema_version"] = boolean
                with self.subTest(add_schema_boolean=boolean), self.assertRaises(ValueError):
                    sealer.matching_add(forged_add, matrix[0]["plugin"], "cursor", approved)
                with self.subTest(info_schema_boolean=boolean), self.assertRaises(ValueError):
                    sealer.matching_client(forged_info, matrix[0]["plugin"], "cursor", approved)
                with self.subTest(doctor_schema_boolean=boolean), self.assertRaises(ValueError):
                    sealer.matching_doctor(forged_doctor, "cursor", approved_inventory)

            native_plugin = "context7"
            native_path = seed / mapping[native_plugin]
            native_original = native_path.read_bytes()
            for native_value in (
                {"plugin": native_plugin, "revision": "d" * 40},
                {"plugin": native_plugin, "nested": {"source_revision": "d" * 40}},
            ):
                native_path.write_text(json.dumps(native_value))
                with self.subTest(native_alias=native_value), self.assertRaises(subprocess.CalledProcessError):
                    subprocess.run([*common, "--digest-only"], check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            native_path.write_bytes(b'{"plugin":"context7","value":Infinity}')
            with self.assertRaises(subprocess.CalledProcessError):
                subprocess.run([*common, "--digest-only"], check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            native_path.write_bytes(native_original)

    def test_profile_sealer_accepts_exact_completion_attestation_and_real_doctor(self) -> None:
        helper = Path(__file__).parents[2] / "deploy" / "uap-observer-seal-profile.py"
        specification = importlib.util.spec_from_file_location("completion_profile_sealer", helper)
        self.assertIsNotNone(specification)
        self.assertIsNotNone(specification.loader)
        sealer = importlib.util.module_from_spec(specification)
        specification.loader.exec_module(sealer)
        approved = {
            "package_version": "1.0.0",
            "source_repository": "upstash/context7",
            "source_revision": "769c6cd22c3d95462d1f55d789e9532cabefa5a9",
            "source_path": "plugins/agent-plugins/context7",
            "tree_digest": "sha256:08eed3b67f2e71a11b68baa594380c2f69ec1bc97584d701deaf7942ac34c0d8",
            "manifest_digest": "sha256:d01781acd899aefa9445a290cf43a481230321934d62f9c8a2aab06a89718236",
        }
        add = {
            "schema_version": 1, "command": "add", "result": "success",
            "data": {
                "dry_run": False, "plugin": "context7",
                "source": "upstash/context7//plugins/agent-plugins/context7",
                "revision": approved["source_revision"], "version": approved["package_version"],
                "tree_digest": approved["tree_digest"], "manifest_digest": approved["manifest_digest"],
                "next_action": "verify reviewed authentication requirements",
                "result": {
                    "installation_id": "test-installation", "mutated": True,
                    "requires_confirmation": False,
                    "plan": {
                        "client_id": "cursor", "scope": "user", "status": "manual_activation_required",
                        "activation": "manual_activation_required", "verification": "package_validated",
                    },
                    "activation": {
                        "activation": "active", "authentication": "authenticated",
                        "policy": "allowed", "verification": "installation_verified",
                        "activation_attested": True, "authentication_attested": True,
                    },
                },
            },
        }
        self.assertTrue(sealer.matching_add(add, "context7", "cursor", approved))
        for mutation in (
            lambda value: value["data"]["result"]["activation"].update(activation_attested=False),
            lambda value: value["data"]["result"].update(mutated=False),
            lambda value: value["data"]["result"].update(nested={"tree_digest": "sha256:" + "f" * 64}),
        ):
            invalid = json.loads(json.dumps(add))
            mutation(invalid)
            with self.assertRaises(ValueError):
                sealer.matching_add(invalid, "context7", "cursor", approved)
        info = {
            "schema_version": 1, "command": "info", "result": "success",
            "data": {
                "name": "context7", "source": "upstash/context7//plugins/agent-plugins/context7",
                "mixed_version": False,
                "clients": [{
                    "client_id": "cursor", "scope": "user", "materialization": "materialized",
                    "activation": "active", "verification": "installation_verified", "policy": "allowed",
                    "receipt_reconciled": False, "native_discovery_reconciled": False,
                    "native_identity_state": "indeterminate",
                    "package_revision": {
                        "version": approved["package_version"], "resolved_revision": approved["source_revision"],
                        "tree_digest": approved["tree_digest"], "manifest_digest": approved["manifest_digest"],
                    },
                }],
            },
        }
        sealer.matching_client(info, "context7", "cursor", approved, completion_attested=True)
        with self.assertRaisesRegex(ValueError, "incomplete or unreconciled"):
            sealer.matching_client(info, "context7", "cursor", approved)
        automatic_add = json.loads(json.dumps(add))
        automatic_add["data"]["result"]["plan"]["client_id"] = "kiro"
        automatic_add["data"]["result"]["activation"].pop("activation_attested")
        self.assertTrue(sealer.matching_add(automatic_add, "context7", "kiro", approved))
        for impostor in (None, False, 0, 1, 1.0, "true"):
            invalid = json.loads(json.dumps(automatic_add))
            invalid["data"]["result"]["activation"]["activation_attested"] = impostor
            with self.subTest(automatic_activation_attested=impostor), self.assertRaises(ValueError):
                sealer.matching_add(invalid, "context7", "kiro", approved)
        kiro_info = json.loads(json.dumps(info))
        kiro_record = kiro_info["data"]["clients"][0]
        kiro_record["client_id"] = "kiro"
        for field in ("receipt_reconciled", "native_discovery_reconciled", "native_identity_state"):
            kiro_record.pop(field)
        sealer.matching_client(
            kiro_info, "context7", "kiro", approved, completion_attested=True,
        )
        with self.assertRaisesRegex(ValueError, "incomplete or unreconciled"):
            sealer.matching_client(kiro_info, "context7", "kiro", approved)
        expected_clients = ("chatgpt", "codex", "copilot", "cursor", "kiro", "vscode")
        doctor = {
            "schema_version": 1, "command": "doctor", "result": "success",
            "data": {
                "clients": [
                    {"client_id": name, "status": "detected" if name == "cursor" else "not_detected"}
                    for name in expected_clients
                ],
                "findings": [], "installation_count": 5, "open_operation_count": 0,
                "read_only": True,
                "supported_clients": [
                    {"client_id": name, "package_mode": "native"} for name in expected_clients
                ],
                "tool_version": "0.1.18",
            },
        }
        sealer.matching_doctor(doctor, "cursor", {"context7": approved})
        healthy_doctor = json.loads(json.dumps(doctor))
        healthy_doctor["data"]["findings"] = [{
            "status": "healthy", "code": "no_degradation_detected",
            "message": "no tracked degradation was detected",
        }]
        sealer.matching_doctor(healthy_doctor, "cursor", {"context7": approved})
        malformed_healthy = json.loads(json.dumps(healthy_doctor))
        malformed_healthy["data"]["findings"][0]["extra"] = True
        with self.assertRaisesRegex(ValueError, "complete five-plugin profile"):
            sealer.matching_doctor(malformed_healthy, "cursor", {"context7": approved})
        incomplete_doctor = json.loads(json.dumps(doctor))
        incomplete_doctor["data"]["findings"] = [{"status": "degraded"}]
        with self.assertRaisesRegex(ValueError, "complete five-plugin profile"):
            sealer.matching_doctor(incomplete_doctor, "cursor", {"context7": approved})

    def test_profile_sealer_rejects_real_incomplete_cursor_and_kiro_records(self) -> None:
        helper = Path(__file__).parents[2] / "deploy" / "uap-observer-seal-profile.py"
        specification = importlib.util.spec_from_file_location("real_shape_profile_sealer", helper)
        self.assertIsNotNone(specification)
        self.assertIsNotNone(specification.loader)
        sealer = importlib.util.module_from_spec(specification)
        specification.loader.exec_module(sealer)
        fixtures = Path(__file__).parents[2] / "tests" / "fixtures" / "agentplugins-0.1.14"
        add = json.loads((fixtures / "add.json").read_text())
        info = json.loads((fixtures / "info.json").read_text())
        add["data"]["targets"] = [
            target for target in add["data"]["targets"] if target["target"] == "cursor"
        ]
        approved = {
            "package_version": "1.0.0",
            "source_repository": "upstash/context7",
            "source_revision": "769c6cd22c3d95462d1f55d789e9532cabefa5a9",
            "source_path": "plugins/agent-plugins/context7",
            "distribution_id": "upstash/context7", "release_sequence": 1,
            "tree_digest": "sha256:08eed3b67f2e71a11b68baa594380c2f69ec1bc97584d701deaf7942ac34c0d8",
            "manifest_digest": "sha256:d01781acd899aefa9445a290cf43a481230321934d62f9c8a2aab06a89718236",
        }
        with self.assertRaisesRegex(ValueError, "prohibited lifecycle"):
            sealer.matching_add(add, "context7", "cursor", approved)
        for client in ("cursor", "kiro"):
            incomplete = json.loads(json.dumps(info))
            incomplete["data"]["clients"] = [
                record for record in incomplete["data"]["clients"] if record["client_id"] == client
            ]
            with self.subTest(client=client), self.assertRaisesRegex(ValueError, "incomplete or unreconciled"):
                sealer.matching_client(incomplete, "context7", client, approved)
        completed = json.loads(json.dumps(info))
        completed["data"]["clients"] = [
            record for record in completed["data"]["clients"] if record["client_id"] == "cursor"
        ]
        completed["data"]["clients"][0].update({
            "activation": "active", "verification": "installation_verified",
            "receipt_reconciled": True, "native_discovery_reconciled": True,
            "native_identity_state": "managed",
        })
        sealer.matching_client(completed, "context7", "cursor", approved)
        revision = info["data"]["clients"][1]["package_revision"]
        self.assertEqual(
            set(revision), {"version", "resolved_revision", "tree_digest", "manifest_digest"},
        )

    @requires_disposable_observer_host
    def test_profile_sealer_native_config_bound_is_exact_and_mutation_safe(self) -> None:
        helper = Path(__file__).parents[2] / "deploy" / "uap-observer-seal-profile.py"
        specification = importlib.util.spec_from_file_location("bounded_profile_sealer", helper)
        self.assertIsNotNone(specification)
        self.assertIsNotNone(specification.loader)
        sealer = importlib.util.module_from_spec(specification)
        specification.loader.exec_module(sealer)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            root.chmod(0o700)
            native = root / "mcp.json"
            native.write_bytes(b"x" * sealer.MAX_NATIVE_CONFIG_BYTES)
            native.chmod(0o600)
            root_fd = os.open(root, os.O_RDONLY | os.O_DIRECTORY)
            try:
                self.assertEqual(
                    len(sealer.protected_file_at(root_fd, "mcp.json", mode=0o600)),
                    sealer.MAX_NATIVE_CONFIG_BYTES,
                )
                native.write_bytes(b"x" * (sealer.MAX_NATIVE_CONFIG_BYTES + 1))
                native.chmod(0o600)
                with self.assertRaisesRegex(ValueError, "4 MiB"):
                    sealer.protected_file_at(root_fd, "mcp.json", mode=0o600)
                native.write_bytes(b"{}")
                native.chmod(0o600)
                original_read = os.read
                changed = False
                def mutate_after_read(descriptor: int, count: int) -> bytes:
                    nonlocal changed
                    body = original_read(descriptor, count)
                    if body and not changed:
                        changed = True
                        native.write_bytes(b'{"changed":true}')
                        native.chmod(0o600)
                    return body
                with mock.patch.object(sealer.os, "read", side_effect=mutate_after_read), self.assertRaisesRegex(ValueError, "changed"):
                    sealer.protected_file_at(root_fd, "mcp.json", mode=0o600)
            finally:
                os.close(root_fd)

    def test_consent_control_record_binds_the_full_request_digest(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            fixture = Fixture(root)
            request = fixture.request()
            record = artifacts(request["challenge"]["value"])["consent.json"]
            control = {**record, "request_digest": fixed_adapters.sha256(canonical_json(request))}
            path = root / f"{request['challenge']['value']}.json"
            path.write_bytes(canonical_json(control))
            path.chmod(0o640)
            original = fixed_adapters.CONSENT_DIRECTORY
            original_isolation = fixed_adapters.isolation_proof
            fixed_adapters.CONSENT_DIRECTORY = root
            fixed_adapters.isolation_proof = lambda *_args: dict(fixed_adapters.PRIVACY_RESULT)
            try:
                config = {"consent_record": {"directory": str(root)}}
                self.assertEqual(fixed_adapters.consent_record(config, request, os.geteuid()), record)
                control["request_digest"] = "sha256:" + "0" * 64
                path.write_bytes(canonical_json(control))
                with self.assertRaisesRegex(ValueError, "complete request"):
                    fixed_adapters.consent_record(config, request, os.geteuid())
            finally:
                fixed_adapters.CONSENT_DIRECTORY = original
                fixed_adapters.isolation_proof = original_isolation


@requires_disposable_observer_host
class ProfileProvisioningTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        helper_path = Path(__file__).parents[2] / "deploy" / "uap-observer-provision-profile.py"
        spec = importlib.util.spec_from_file_location("uap_observer_provision_profile", helper_path)
        if spec is None or spec.loader is None:
            raise RuntimeError("profile helper could not be loaded")
        cls.helper = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.helper)

    def test_transaction_schema_rejects_boolean_identifier(self) -> None:
        digest = "sha256:" + "a" * 64
        body = json.loads(self.helper.transaction_body("codex", digest, True))
        body["schema_version"] = True
        payload = {key: body[key] for key in ("schema_version", "client", "seed_digest", "profile_preexisting", "phase", "previous_publication")}
        body["payload_sha256"] = hashlib.sha256(json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
        with self.assertRaisesRegex(ValueError, "marker is invalid"):
            self.helper.validate_transaction(json.dumps(body).encode(), "codex")

    def test_staging_tree_removal_fsyncs_parent_before_cleanup_continues(self) -> None:
        calls = []
        with mock.patch.object(self.helper, "remove_tree", side_effect=lambda parent, name: calls.append(("remove", parent, name))), mock.patch.object(self.helper, "fsync_directory", side_effect=lambda parent: calls.append(("fsync", parent))):
            self.helper.remove_tree_durable(17, ".codex.new")
        self.assertEqual(calls, [("remove", 17, ".codex.new"), ("fsync", 17)])

    def test_interrupted_recovery_durably_removes_proof_staging_before_marker(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            profile_root, proof_root = root / "profiles", root / "proofs"
            profile_root.mkdir(); proof_root.mkdir()
            (profile_root / ".codex.new").mkdir()
            (proof_root / ".codex.new").mkdir()
            digest = "sha256:" + "a" * 64
            marker = profile_root / ".codex.transaction"
            marker.write_bytes(self.helper.transaction_body("codex", digest, False))
            marker.chmod(0o600)
            profile_fd = os.open(profile_root, self.helper.OPEN_DIRECTORY)
            proof_fd = os.open(proof_root, self.helper.OPEN_DIRECTORY)
            calls = []
            original_remove = self.helper.remove_tree
            original_fsync = self.helper.fsync_directory
            def remove(parent: int, name: str) -> None:
                calls.append(("remove", parent, name))
                original_remove(parent, name)
            def sync(parent: int) -> None:
                calls.append(("fsync", parent))
                original_fsync(parent)
            try:
                with mock.patch.object(self.helper, "remove_tree", side_effect=remove), mock.patch.object(self.helper, "fsync_directory", side_effect=sync):
                    self.assertFalse(self.helper.recover_transaction(
                        profile_fd, proof_fd, "codex", marker.name, digest,
                        os.geteuid(), os.getegid(),
                    ))
            finally:
                os.close(proof_fd); os.close(profile_fd)
            proof_remove = calls.index(("remove", proof_fd, ".codex.new"))
            self.assertEqual(calls[proof_remove + 1], ("fsync", proof_fd))
            self.assertFalse(marker.exists())

    def test_recovery_resyncs_already_absent_proof_target_before_removing_marker(self) -> None:
        for proof_name in (".codex.new", "codex"):
            with self.subTest(proof_name=proof_name), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                profile_root, proof_root = root / "profiles", root / "proofs"
                profile_root.mkdir(); proof_root.mkdir(); (proof_root / proof_name).mkdir()
                digest = "sha256:" + "a" * 64
                marker = profile_root / ".codex.transaction"
                marker.write_bytes(self.helper.transaction_body("codex", digest, False))
                marker.chmod(0o600)
                profile_fd = os.open(profile_root, self.helper.OPEN_DIRECTORY)
                proof_fd = os.open(proof_root, self.helper.OPEN_DIRECTORY)
                original_remove = self.helper.remove_tree
                original_fsync = self.helper.fsync_directory
                removed_target = False

                def remove(parent: int, name: str) -> None:
                    nonlocal removed_target
                    original_remove(parent, name)
                    if parent == proof_fd and name == proof_name:
                        removed_target = True

                def fail_after_target_unlink(parent: int) -> None:
                    if parent == proof_fd and removed_target:
                        raise OSError("proof parent fsync failed")
                    original_fsync(parent)

                try:
                    with mock.patch.object(self.helper, "remove_tree", side_effect=remove), mock.patch.object(self.helper, "fsync_directory", side_effect=fail_after_target_unlink), self.assertRaisesRegex(OSError, "proof parent fsync failed"):
                        self.helper.recover_transaction(
                            profile_fd, proof_fd, "codex", marker.name, digest,
                            os.geteuid(), os.getegid(),
                        )
                    self.assertFalse((proof_root / proof_name).exists())
                    self.assertTrue(marker.exists(), "recovery marker was removed before deletion durability")
                    calls = []
                    def sync_retry(parent: int) -> None:
                        calls.append(("fsync", parent, marker.exists()))
                        original_fsync(parent)
                    with mock.patch.object(self.helper, "fsync_directory", side_effect=sync_retry):
                        self.assertFalse(self.helper.recover_transaction(
                            profile_fd, proof_fd, "codex", marker.name, digest,
                            os.geteuid(), os.getegid(),
                        ))
                    self.assertIn(("fsync", proof_fd, True), calls)
                    self.assertFalse(marker.exists())
                finally:
                    os.close(proof_fd); os.close(profile_fd)

    def test_committed_marker_without_publication_record_resumes_rollback(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            profile_root, proof_root = root / "profiles", root / "proofs"
            profile_root.mkdir(); proof_root.mkdir()
            (profile_root / "codex").mkdir(); (profile_root / "codex" / "published").write_text("partial")
            (proof_root / "codex").mkdir(); (proof_root / "codex" / "proof").write_text("partial")
            digest = "sha256:" + "a" * 64
            marker = profile_root / ".codex.transaction"
            marker.write_bytes(self.helper.transaction_body("codex", digest, True, "committed"))
            marker.chmod(0o600)
            lock = profile_root / ".codex.lock"
            lock.write_bytes(b""); lock.chmod(0o600)
            profile_fd = os.open(profile_root, self.helper.OPEN_DIRECTORY)
            proof_fd = os.open(proof_root, self.helper.OPEN_DIRECTORY)
            lock_fd = os.open(lock, os.O_RDWR | os.O_NOFOLLOW)
            try:
                self.assertFalse(self.helper.recover_transaction(
                    profile_fd, proof_fd, "codex", marker.name, digest,
                    os.geteuid(), os.getegid(), lock_fd,
                ))
            finally:
                os.close(lock_fd); os.close(proof_fd); os.close(profile_fd)
            self.assertTrue((profile_root / "codex").is_dir())
            self.assertEqual(list((profile_root / "codex").iterdir()), [])
            self.assertFalse((proof_root / "codex").exists())
            self.assertFalse(marker.exists())

    def test_torn_publication_record_recovers_only_with_authenticated_transition(self) -> None:
        digest = "sha256:" + "a" * 64
        prior_values = ("", "sha256:" + "b" * 64)
        helper_path = Path(__file__).parents[2] / "deploy" / "uap-observer-provision-profile.py"
        child = (
            "import importlib.util,os,sys\n"
            "from pathlib import Path\n"
            "spec=importlib.util.spec_from_file_location('provision',sys.argv[1]); h=importlib.util.module_from_spec(spec); spec.loader.exec_module(h)\n"
            "root=Path(sys.argv[2]); digest=sys.argv[3]\n"
            "pfd=os.open(root/'profiles',h.OPEN_DIRECTORY); qfd=os.open(root/'proofs',h.OPEN_DIRECTORY); lfd=os.open(root/'profiles/.codex.lock',os.O_RDWR|os.O_NOFOLLOW)\n"
            "try: result=h.recover_transaction(pfd,qfd,'codex','.codex.transaction',digest,os.geteuid(),os.getegid(),lfd)\n"
            "finally: os.close(lfd); os.close(qfd); os.close(pfd)\n"
            "print('forward' if result else 'rollback')\n"
        )
        for previous in prior_values:
            for prefix_length in range(len(digest) + 1):
                with self.subTest(previous=previous or "empty", prefix_length=prefix_length), tempfile.TemporaryDirectory() as temporary:
                    root = Path(temporary)
                    profile_root, proof_root = root / "profiles", root / "proofs"
                    profile_root.mkdir(); proof_root.mkdir()
                    (profile_root / "codex").mkdir()
                    (profile_root / "codex" / "published").write_text("new")
                    (proof_root / "codex").mkdir()
                    (proof_root / "codex" / "proof").write_text("new")
                    marker = profile_root / ".codex.transaction"
                    marker.write_bytes(self.helper.transaction_body(
                        "codex", digest, True, "committed", previous,
                    ))
                    marker.chmod(0o600)
                    lock = profile_root / ".codex.lock"
                    lock.write_bytes(digest.encode("ascii")[:prefix_length])
                    lock.chmod(0o600)
                    result = subprocess.run(
                        [sys.executable, "-c", child, str(helper_path), str(root), digest],
                        check=True, text=True, stdout=subprocess.PIPE,
                    )
                    if prefix_length == len(digest):
                        self.assertEqual(result.stdout.strip(), "forward")
                        self.assertEqual(lock.read_text(), digest)
                        self.assertTrue(marker.exists())
                    else:
                        self.assertEqual(result.stdout.strip(), "rollback")
                        self.assertEqual(lock.read_text(), previous)
                        self.assertTrue((profile_root / "codex").is_dir())
                        self.assertEqual(list((profile_root / "codex").iterdir()), [])
                        self.assertFalse((proof_root / "codex").exists())
                        self.assertFalse(marker.exists())

    def test_publication_record_write_exposes_every_torn_prefix_and_fsync_failure(self) -> None:
        digest = ("sha256:" + "a" * 64).encode("ascii")
        with tempfile.TemporaryDirectory() as temporary:
            record = Path(temporary) / "record"
            for prefix_length in range(len(digest)):
                with self.subTest(prefix_length=prefix_length):
                    record.write_bytes(b"sha256:" + b"b" * 64)
                    descriptor = os.open(record, os.O_RDWR | os.O_NOFOLLOW)
                    original_write = os.write
                    wrote_prefix = False

                    def interrupted_write(fd: int, body) -> int:
                        nonlocal wrote_prefix
                        if wrote_prefix or prefix_length == 0:
                            raise OSError("injected publication write error")
                        wrote_prefix = True
                        return original_write(fd, body[:prefix_length])

                    try:
                        if prefix_length == 0:
                            def fail_after_truncate(name: str) -> None:
                                if name == "after_publication_record_truncate":
                                    raise OSError("injected publication write error")
                            context = mock.patch.object(self.helper, "checkpoint", side_effect=fail_after_truncate)
                        else:
                            context = mock.patch.object(self.helper.os, "write", side_effect=interrupted_write)
                        with context, self.assertRaisesRegex(OSError, "publication write error"):
                            self.helper.write_publication_record(descriptor, digest)
                    finally:
                        os.close(descriptor)
                    self.assertEqual(record.read_bytes(), digest[:prefix_length])

            record.write_bytes(b"sha256:" + b"b" * 64)
            descriptor = os.open(record, os.O_RDWR | os.O_NOFOLLOW)
            try:
                with mock.patch.object(self.helper.os, "fsync", side_effect=OSError("injected publication fsync failure")), self.assertRaisesRegex(OSError, "fsync failure"):
                    self.helper.write_publication_record(descriptor, digest)
            finally:
                os.close(descriptor)
            self.assertEqual(record.read_bytes(), digest)

    def test_malformed_publication_record_without_marker_remains_fail_closed(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            record = Path(temporary) / "record"
            record.write_bytes(b"sha256:attacker-prefix")
            descriptor = os.open(record, os.O_RDWR | os.O_NOFOLLOW)
            try:
                with self.assertRaisesRegex(ValueError, "publication record is invalid"):
                    self.helper.published_seed_digest(descriptor)
            finally:
                os.close(descriptor)
            self.assertEqual(record.read_bytes(), b"sha256:attacker-prefix")

    def test_authenticated_marker_does_not_authorize_conflicting_valid_publication(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            profile_root, proof_root = root / "profiles", root / "proofs"
            profile_root.mkdir(); proof_root.mkdir()
            digest = "sha256:" + "a" * 64
            previous = "sha256:" + "b" * 64
            conflicting = "sha256:" + "c" * 64
            marker = profile_root / ".codex.transaction"
            marker.write_bytes(self.helper.transaction_body("codex", digest, False, "committed", previous))
            marker.chmod(0o600)
            lock = profile_root / ".codex.lock"
            lock.write_text(conflicting); lock.chmod(0o600)
            profile_fd = os.open(profile_root, self.helper.OPEN_DIRECTORY)
            proof_fd = os.open(proof_root, self.helper.OPEN_DIRECTORY)
            lock_fd = os.open(lock, os.O_RDWR | os.O_NOFOLLOW)
            try:
                with self.assertRaisesRegex(ValueError, "conflicts with authenticated transaction"):
                    self.helper.recover_transaction(
                        profile_fd, proof_fd, "codex", marker.name, digest,
                        os.geteuid(), os.getegid(), lock_fd,
                    )
            finally:
                os.close(lock_fd); os.close(proof_fd); os.close(profile_fd)
            self.assertEqual(lock.read_text(), conflicting)
            self.assertTrue(marker.exists())

    def test_durable_rollback_sidecar_overrides_committed_forward_state(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            profile_root, proof_root = root / "profiles", root / "proofs"
            profile_root.mkdir(); proof_root.mkdir()
            (profile_root / "codex").mkdir(); (profile_root / "codex" / "published").write_text("partial")
            (proof_root / "codex").mkdir(); (proof_root / "codex" / "proof").write_text("partial")
            digest = "sha256:" + "a" * 64
            marker = profile_root / ".codex.transaction"
            marker.write_bytes(self.helper.transaction_body("codex", digest, False, "committed"))
            marker.chmod(0o600)
            sidecar = profile_root / ".codex.transaction.new"
            sidecar.write_bytes(self.helper.transaction_body("codex", digest, False, "rollback"))
            sidecar.chmod(0o600)
            lock = profile_root / ".codex.lock"
            lock.write_text(digest); lock.chmod(0o600)
            profile_fd = os.open(profile_root, self.helper.OPEN_DIRECTORY)
            proof_fd = os.open(proof_root, self.helper.OPEN_DIRECTORY)
            lock_fd = os.open(lock, os.O_RDWR | os.O_NOFOLLOW)
            try:
                self.assertFalse(self.helper.recover_transaction(
                    profile_fd, proof_fd, "codex", marker.name, digest,
                    os.geteuid(), os.getegid(), lock_fd,
                ))
            finally:
                os.close(lock_fd); os.close(proof_fd); os.close(profile_fd)
            self.assertFalse((profile_root / "codex").exists())
            self.assertFalse((proof_root / "codex").exists())
            self.assertEqual(lock.read_bytes(), b"")
            self.assertFalse(marker.exists())
            self.assertFalse(sidecar.exists())

    def test_incomplete_transaction_sidecar_is_discarded_and_recovery_is_idempotent(self) -> None:
        digest = "sha256:" + "a" * 64
        complete = self.helper.transaction_body("codex", digest, True, "rollback")
        for body in (b"", complete[:1], complete[:len(complete) // 2], b"{" + b" " * 4096):
            with self.subTest(staged_bytes=len(body)), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                profile_root, proof_root = root / "profiles", root / "proofs"
                profile_root.mkdir(); proof_root.mkdir()
                (profile_root / "codex").mkdir()
                (profile_root / "codex" / "published").write_text("partial")
                (proof_root / "codex").mkdir()
                (proof_root / "codex" / "proof").write_text("partial")
                marker = profile_root / ".codex.transaction"
                marker.write_bytes(self.helper.transaction_body("codex", digest, True, "committed"))
                marker.chmod(0o600)
                sidecar = profile_root / ".codex.transaction.new"
                sidecar.write_bytes(body)
                sidecar.chmod(0o600)
                lock = profile_root / ".codex.lock"
                lock.write_text(digest); lock.chmod(0o600)
                profile_fd = os.open(profile_root, self.helper.OPEN_DIRECTORY)
                proof_fd = os.open(proof_root, self.helper.OPEN_DIRECTORY)
                lock_fd = os.open(lock, os.O_RDWR | os.O_NOFOLLOW)
                try:
                    for _attempt in range(2):
                        self.assertFalse(self.helper.recover_transaction(
                            profile_fd, proof_fd, "codex", marker.name, digest,
                            os.geteuid(), os.getegid(), lock_fd,
                        ))
                finally:
                    os.close(lock_fd); os.close(proof_fd); os.close(profile_fd)
                self.assertTrue((profile_root / "codex").is_dir())
                self.assertEqual(list((profile_root / "codex").iterdir()), [])
                self.assertFalse((proof_root / "codex").exists())
                self.assertFalse(marker.exists())
                self.assertFalse(sidecar.exists())
                self.assertEqual(lock.read_bytes(), b"")

    def test_incomplete_initial_sidecar_cannot_wedge_a_fresh_transaction(self) -> None:
        digest = "sha256:" + "a" * 64
        body = self.helper.transaction_body("codex", digest, False, "preparing")
        for staged in (b"", body[:len(body) // 2]):
            with self.subTest(staged_bytes=len(staged)), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                profile_root, proof_root = root / "profiles", root / "proofs"
                profile_root.mkdir(); proof_root.mkdir()
                sidecar = profile_root / ".codex.transaction.new"
                sidecar.write_bytes(staged)
                sidecar.chmod(0o600)
                profile_fd = os.open(profile_root, self.helper.OPEN_DIRECTORY)
                proof_fd = os.open(proof_root, self.helper.OPEN_DIRECTORY)
                try:
                    self.assertFalse(self.helper.recover_transaction(
                        profile_fd, proof_fd, "codex", ".codex.transaction", digest,
                        os.geteuid(), os.getegid(),
                    ))
                    self.assertFalse(self.helper.recover_transaction(
                        profile_fd, proof_fd, "codex", ".codex.transaction", digest,
                        os.geteuid(), os.getegid(),
                    ))
                finally:
                    os.close(proof_fd); os.close(profile_fd)
                self.assertFalse(sidecar.exists())

    def test_unprotected_incomplete_sidecar_fails_closed(self) -> None:
        digest = "sha256:" + "a" * 64
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            profile_root, proof_root = root / "profiles", root / "proofs"
            profile_root.mkdir(); proof_root.mkdir()
            sidecar = profile_root / ".codex.transaction.new"
            sidecar.write_bytes(b"")
            sidecar.chmod(0o640)
            profile_fd = os.open(profile_root, self.helper.OPEN_DIRECTORY)
            proof_fd = os.open(proof_root, self.helper.OPEN_DIRECTORY)
            try:
                with self.assertRaisesRegex(ValueError, "sidecar is not protected"):
                    self.helper.recover_transaction(
                        profile_fd, proof_fd, "codex", ".codex.transaction", digest,
                        os.geteuid(), os.getegid(),
                    )
            finally:
                os.close(proof_fd); os.close(profile_fd)
            self.assertTrue(sidecar.exists())

    def test_authenticated_transaction_sidecar_conflict_fails_closed(self) -> None:
        digest = "sha256:" + "a" * 64
        other_digest = "sha256:" + "b" * 64
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            profile_root, proof_root = root / "profiles", root / "proofs"
            profile_root.mkdir(); proof_root.mkdir()
            marker = profile_root / ".codex.transaction"
            marker.write_bytes(self.helper.transaction_body("codex", digest, False, "preparing"))
            marker.chmod(0o600)
            sidecar = profile_root / ".codex.transaction.new"
            sidecar.write_bytes(self.helper.transaction_body("codex", other_digest, False, "committed"))
            sidecar.chmod(0o600)
            profile_fd = os.open(profile_root, self.helper.OPEN_DIRECTORY)
            proof_fd = os.open(proof_root, self.helper.OPEN_DIRECTORY)
            try:
                with self.assertRaisesRegex(ValueError, "sidecar differs"):
                    self.helper.recover_transaction(
                        profile_fd, proof_fd, "codex", marker.name, digest,
                        os.geteuid(), os.getegid(),
                    )
            finally:
                os.close(proof_fd); os.close(profile_fd)
            self.assertTrue(marker.exists())
            self.assertTrue(sidecar.exists())

    def test_durable_committed_sidecar_is_promoted_before_recovery(self) -> None:
        digest = "sha256:" + "a" * 64
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            profile_root, proof_root = root / "profiles", root / "proofs"
            profile_root.mkdir(); proof_root.mkdir()
            (profile_root / "codex").mkdir(); (proof_root / "codex").mkdir()
            marker = profile_root / ".codex.transaction"
            marker.write_bytes(self.helper.transaction_body("codex", digest, False, "preparing"))
            marker.chmod(0o600)
            sidecar = profile_root / ".codex.transaction.new"
            sidecar.write_bytes(self.helper.transaction_body("codex", digest, False, "committed"))
            sidecar.chmod(0o600)
            lock = profile_root / ".codex.lock"
            lock.write_text(digest); lock.chmod(0o600)
            profile_fd = os.open(profile_root, self.helper.OPEN_DIRECTORY)
            proof_fd = os.open(proof_root, self.helper.OPEN_DIRECTORY)
            lock_fd = os.open(lock, os.O_RDWR | os.O_NOFOLLOW)
            try:
                self.assertTrue(self.helper.recover_transaction(
                    profile_fd, proof_fd, "codex", marker.name, digest,
                    os.geteuid(), os.getegid(), lock_fd,
                ))
            finally:
                os.close(lock_fd); os.close(proof_fd); os.close(profile_fd)
            self.assertEqual(self.helper.validate_transaction(marker.read_bytes(), "codex")["phase"], "committed")
            self.assertFalse(sidecar.exists())

    def test_native_projection_validator_rejects_ambiguous_or_incomplete_schema(self) -> None:
        digest = "sha256:" + "a" * 64
        entry = {
            "plugin": "context7", "component_kind": "mcp", "tuple": sealed_tuple("context7"),
            "native_config": {"path": "/var/lib/uap-observer/proofs/codex/native/context7.blob", "sha256": digest},
            "client_config": {"path": "/var/lib/uap-observer/profiles/codex/context7.json", "sha256": digest},
            "manager_add_sha256": digest, "manager_info_sha256": digest,
            "post_add_doctor_sha256": digest,
        }
        entries = [
            {
                **entry, "plugin": plugin,
                "component_kind": "skill" if plugin == "agent-code-navigator" else "mcp",
                "tuple": sealed_tuple(plugin),
                "native_config": {
                    "path": f"/var/lib/uap-observer/proofs/codex/native/{plugin}.blob", "sha256": digest,
                },
                "client_config": {
                    "path": (
                        "/var/lib/uap-observer/profiles/codex/skills/code-tool-router/SKILL.md"
                        if plugin == "agent-code-navigator"
                        else f"/var/lib/uap-observer/profiles/codex/{plugin}.json"
                    ),
                    "sha256": digest,
                },
            }
            for plugin in sorted(fixed_adapters.HEROES)
        ]
        valid = {"schema_version": 2, "client_id": "codex", "entries": entries}
        self.assertEqual(self.helper.validate_native_projection(valid, "codex"), entries)
        kiro_entries = [{
            **item,
            "native_config": {
                "path": f'/var/lib/uap-observer/proofs/kiro/native/{item["plugin"]}.blob',
                "sha256": digest,
            },
            "client_config": {
                "path": (
                    "/var/lib/uap-observer/profiles/kiro/.kiro/skills/code-tool-router/SKILL.md"
                    if item["plugin"] == "agent-code-navigator"
                    else "/var/lib/uap-observer/profiles/kiro/.kiro/settings/mcp.json"
                ),
                "sha256": digest,
            },
        } for item in entries]
        valid_kiro = {"schema_version": 2, "client_id": "kiro", "entries": kiro_entries}
        self.assertEqual(self.helper.validate_native_projection(valid_kiro, "kiro"), kiro_entries)
        for conflicting in (
            [{**kiro_entries[0], "client_config": {"path": "/var/lib/uap-observer/profiles/kiro/other/mcp.json", "sha256": digest}}, *kiro_entries[1:]],
            [kiro_entries[0], {**kiro_entries[1],
              "native_config": {**kiro_entries[1]["native_config"], "sha256": "sha256:" + "c" * 64},
              "client_config": {**kiro_entries[1]["client_config"], "sha256": "sha256:" + "c" * 64}}, *kiro_entries[2:]],
        ):
            with self.subTest(conflicting_shared_path=conflicting), self.assertRaises(ValueError):
                self.helper.validate_native_projection({**valid_kiro, "entries": conflicting}, "kiro")
        receipts = {"schema_version": 1, "receipts": [{
            "name": entry["plugin"], "tuple": entry["tuple"],
            "manager_add_sha256": entry["manager_add_sha256"],
            "manager_info_sha256": entry["manager_info_sha256"],
            "post_add_doctor_sha256": entry["post_add_doctor_sha256"],
        } for entry in entries]}
        self.helper.validate_receipts(receipts, entries)
        for mutation in (
            lambda value: value.update(schema_version=True),
            lambda value: value["receipts"].pop(),
            lambda value: value["receipts"][0].update(tuple=sealed_tuple("context7")),
            lambda value: value["receipts"][0].update(manager_add_sha256="sha256:" + "f" * 64),
        ):
            forged = json.loads(json.dumps(receipts))
            mutation(forged)
            with self.assertRaises(ValueError):
                self.helper.validate_receipts(forged, entries)
        malformed = (
            {**valid, "schema_version": True},
            {**valid, "client_id": "cursor"},
            {**valid, "entries": []},
            {**valid, "unexpected": 1},
            {**valid, "entries": [{key: value for key, value in entry.items() if key != "client_config"}]},
            {**valid, "entries": [{**entry, "unexpected": 1}]},
            {**valid, "entries": [{**entry, "client_config": {"path": entry["client_config"]["path"]}}]},
            {**valid, "entries": [{**entry, "client_config": {**entry["client_config"], "sha256": True}}]},
            {**valid, "entries": [{**entries[0], "tuple": {"product_id": entries[0]["plugin"]}}, *entries[1:]]},
            {**valid, "entries": [{**entries[0], "tuple": {**entries[0]["tuple"], "extra": 1}}, *entries[1:]]},
            {**valid, "entries": [{**entries[0], "tuple": {**entries[0]["tuple"], "snapshot_sequence": True}}, *entries[1:]]},
            {**valid, "entries": [{**entries[0], "component_kind": "mcp"}, *entries[1:]]},
            {**valid, "entries": [{**entries[0], "client_config": {
                **entries[0]["client_config"],
                "path": "/var/lib/uap-observer/profiles/codex/agent-code-navigator.json",
            }}, *entries[1:]]},
            {**valid, "entries": [entries[0], {**entries[1], "client_config": {
                **entries[1]["client_config"],
                "path": "/var/lib/uap-observer/profiles/codex/skills/code-tool-router/SKILL.md",
            }}, *entries[2:]]},
            {**valid, "entries": [{**item, "client_config": {
                **item["client_config"],
                "path": "/var/lib/uap-observer/profiles/codex/shared.json",
            }} for item in entries]},
        )
        for value in malformed:
            with self.subTest(value=value), self.assertRaises(ValueError):
                self.helper.validate_native_projection(value, "codex")
        for encoded in (
            b'{"schema_version":1,"schema_version":1,"client_id":"codex","entries":[]}',
            b'{"schema_version":1,"Schema_Version":1,"client_id":"codex","entries":[]}',
            b'{"schema_version":1,"client_id":"codex","entries":[],"value":NaN}',
            b'{"schema_version":1,"client_id":"codex","entries":[],"value":1e400}',
        ):
            with self.subTest(encoded=encoded), self.assertRaises((ValueError, json.JSONDecodeError)):
                self.helper.strict_json_loads(encoded)

    def test_kiro_skill_and_shared_mcp_config_are_protected_provisioning_surfaces(self) -> None:
        digest = fixed_adapters.sha256(b"{}")
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            staging = root / "staging"
            proof = root / "proof"
            (staging / ".kiro" / "settings").mkdir(parents=True)
            (staging / ".kiro" / "settings" / "mcp.json").write_bytes(b"{}")
            (staging / ".kiro" / "skills" / "code-tool-router").mkdir(parents=True)
            (staging / ".kiro" / "skills" / "code-tool-router" / "SKILL.md").write_bytes(b"{}")
            (proof / "native").mkdir(parents=True)
            entries = []
            for plugin in sorted(fixed_adapters.HEROES):
                (proof / "native" / f"{plugin}.blob").write_bytes(b"{}")
                entries.append({
                    "plugin": plugin,
                    "component_kind": "skill" if plugin == "agent-code-navigator" else "mcp",
                    "tuple": sealed_tuple(plugin),
                    "native_config": {
                        "path": f"/var/lib/uap-observer/proofs/kiro/native/{plugin}.blob",
                        "sha256": digest,
                    },
                    "client_config": {
                        "path": (
                            "/var/lib/uap-observer/profiles/kiro/.kiro/skills/code-tool-router/SKILL.md"
                            if plugin == "agent-code-navigator"
                            else "/var/lib/uap-observer/profiles/kiro/.kiro/settings/mcp.json"
                        ),
                        "sha256": digest,
                    },
                    "manager_add_sha256": "sha256:" + "9" * 64,
                    "manager_info_sha256": "sha256:" + "a" * 64,
                    "post_add_doctor_sha256": "sha256:" + "b" * 64,
                })
            (proof / "native-projection.json").write_text(json.dumps({
                "schema_version": 2, "client_id": "kiro", "entries": entries,
            }))
            (proof / "receipts.json").write_text(json.dumps({
                "schema_version": 1, "receipts": [{
                    "name": entry["plugin"], "tuple": entry["tuple"],
                    "manager_add_sha256": entry["manager_add_sha256"],
                    "manager_info_sha256": entry["manager_info_sha256"],
                    "post_add_doctor_sha256": entry["post_add_doctor_sha256"],
                } for entry in entries],
            }))
            for path in (proof / "native-projection.json", proof / "receipts.json"):
                path.chmod(0o440)
            staging_fd = os.open(staging, self.helper.OPEN_DIRECTORY)
            proof_fd = os.open(proof, self.helper.OPEN_DIRECTORY)
            try:
                files, directories = self.helper.active_native_paths(proof_fd, "kiro", staging_fd)
            finally:
                os.close(proof_fd)
                os.close(staging_fd)
            self.assertEqual(files, {
                (".kiro", "settings", "mcp.json"),
                (".kiro", "skills", "code-tool-router", "SKILL.md"),
            })
            self.assertEqual(directories, {
                (), (".kiro",), (".kiro", "settings"), (".kiro", "skills"),
                (".kiro", "skills", "code-tool-router"),
            })

    @requires_disposable_observer_host
    def test_copy_tree_uses_descriptors_and_preserves_exact_private_modes(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source, destination = root / "source", root / "destination"
            source.mkdir(mode=0o700)
            destination.mkdir(mode=0o700)
            (source / "nested").mkdir(mode=0o700)
            (source / "nested" / "credential.json").write_text("fixture")
            (source / "nested" / "credential.json").chmod(0o600)
            source_fd = os.open(source, self.helper.OPEN_DIRECTORY)
            destination_fd = os.open(destination, self.helper.OPEN_DIRECTORY)
            try:
                count, total = self.helper.copy_tree(source_fd, destination_fd, hashlib.sha256())
            finally:
                os.close(source_fd)
                os.close(destination_fd)
            copied = destination / "nested" / "credential.json"
            self.assertEqual((count, total), (2, len("fixture")))
            self.assertEqual(copied.read_text(), "fixture")
            self.assertEqual(stat.S_IMODE(copied.stat().st_mode), 0o600)

    @requires_disposable_observer_host
    def test_copy_tree_rejects_links_and_mutable_seed_entries(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / "source"
            source.mkdir(mode=0o700)
            (source / "regular").write_text("fixture")
            (source / "regular").chmod(0o600)
            os.link(source / "regular", source / "hardlink")
            source_fd = os.open(source, self.helper.OPEN_DIRECTORY)
            try:
                with self.assertRaisesRegex(ValueError, "hardlinked"):
                    self.helper.checked_entry(source_fd, "regular")
                (source / "hardlink").unlink()
                (source / "symlink").symlink_to("regular")
                with self.assertRaisesRegex(ValueError, "link or special"):
                    self.helper.checked_entry(source_fd, "symlink")
                (source / "regular").chmod(0o620)
                with self.assertRaisesRegex(ValueError, "protected"):
                    self.helper.checked_entry(source_fd, "regular")
            finally:
                os.close(source_fd)

    @requires_disposable_observer_host
    def test_real_service_uid_can_use_provisioned_root_readonly_boundary(self) -> None:
        workspace = Path(__file__).parents[2]
        with tempfile.TemporaryDirectory(dir="/run", prefix="uap-observer-service-") as temporary:
            root = Path(temporary)
            root.chmod(0o711)
            service_uid = service_gid = next(
                candidate for candidate in range(62000, 65000)
                if all(account.pw_uid != candidate for account in pwd.getpwall())
            )
            profile_root, proof_root = root / "profiles", root / "proofs"
            profile_root.mkdir(mode=0o711)
            proof_root.mkdir(mode=0o711)
            (profile_root / "codex").mkdir(mode=0o700)
            try:
                os.chown(profile_root / "codex", service_uid, service_gid)
            except OSError as error:
                if error.errno == errno.EINVAL:
                    self.skipTest("provider user namespace does not map a disposable service UID")
                raise
            seed = root / "seed"
            for directory in (seed / ".config", seed / ".auth", seed / ".cache", seed / ".state"):
                directory.mkdir(parents=True, mode=0o700)
            proof_seed = seed / self.helper.PROOF_SEED_NAME
            native_proof = proof_seed / "native"
            native_proof.mkdir(parents=True, mode=0o700)
            approved = sealed_tuple("context7")
            entries = []
            for plugin in sorted(fixed_adapters.HEROES):
                body = json.dumps({"plugin": plugin}).encode()
                relative = (
                    Path("skills/code-tool-router/SKILL.md")
                    if plugin == "agent-code-navigator" else Path(".config") / f"{plugin}.json"
                )
                active = seed / relative
                active.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
                active.write_bytes(body)
                native = native_proof / f"{plugin}.blob"
                native.write_bytes(body)
                entry_tuple = approved if plugin == "context7" else {**approved, "product_id": plugin}
                entries.append({
                    "plugin": plugin,
                    "component_kind": "skill" if plugin == "agent-code-navigator" else "mcp",
                    "tuple": entry_tuple,
                    "native_config": {
                        "path": str(proof_root / "codex" / "native" / f"{plugin}.blob"),
                        "sha256": fixed_adapters.sha256(body),
                    },
                    "client_config": {
                        "path": str(profile_root / "codex" / relative),
                        "sha256": fixed_adapters.sha256(body),
                    },
                    "manager_add_sha256": "sha256:" + "9" * 64,
                    "manager_info_sha256": "sha256:" + "a" * 64,
                    "post_add_doctor_sha256": "sha256:" + "b" * 64,
                })
            projection = fixed_adapters.canonical_json({
                "schema_version": 2, "client_id": "codex", "entries": entries,
            })
            (proof_seed / "native-projection.json").write_bytes(projection)
            (proof_seed / "receipts.json").write_text(json.dumps({
                "schema_version": 1, "receipts": [{
                    "name": entry["plugin"], "tuple": entry["tuple"],
                    "manager_add_sha256": entry["manager_add_sha256"],
                    "manager_info_sha256": entry["manager_info_sha256"],
                    "post_add_doctor_sha256": entry["post_add_doctor_sha256"],
                } for entry in entries],
            }))
            account = mock.Mock(pw_uid=service_uid, pw_gid=service_gid)
            argv = ["provision", "--client", "codex", "--root-owned-seed", str(seed), "--seed-digest", "show"]
            patches = (
                mock.patch.object(self.helper, "PROFILE_ROOT", profile_root),
                mock.patch.object(self.helper, "PROOF_ROOT", proof_root),
                mock.patch.object(self.helper.pwd, "getpwnam", return_value=account),
            )
            with patches[0], patches[1], patches[2], mock.patch("sys.argv", argv), mock.patch("sys.stdout") as output:
                self.helper.main()
                seed_digest = output.write.call_args_list[0].args[0].strip()
            argv[-1] = seed_digest
            with mock.patch.object(self.helper, "PROFILE_ROOT", profile_root), mock.patch.object(self.helper, "PROOF_ROOT", proof_root), mock.patch.object(self.helper.pwd, "getpwnam", return_value=account), mock.patch("sys.argv", argv):
                self.helper.main()
            item = {
                "profile": str(profile_root / "codex"), "client_id": "codex",
                "native_projection": {
                    "path": str(proof_root / "codex" / "native-projection.json"),
                    "sha256": fixed_adapters.sha256(projection),
                },
            }
            child = (
                "import json,os,sys\n"
                "from pathlib import Path\n"
                "from observer import fixed_adapters as adapter\n"
                "uid=int(sys.argv[1]); gid=int(sys.argv[2]); item=json.loads(sys.argv[3]); approved=json.loads(sys.argv[4])\n"
                "os.setgroups([gid]); os.setgid(gid); os.setuid(uid)\n"
                "projection=adapter.verified_native_projection(item,'context7',approved,owner_uid=uid)\n"
                "profile=Path(item['profile']); proof=Path(item['native_projection']['path'])\n"
                "readable=bool(proof.read_bytes()) and bool((profile/'.config/context7.json').read_bytes())\n"
                "writable=[]\n"
                "for name in ('.auth','.cache','.state'):\n"
                " p=profile/name/'write-test'; p.write_text('ok'); writable.append(p.read_text()=='ok')\n"
                "blocked=[]\n"
                "for operation in (lambda:(profile/'.config/context7.json').write_text('bad'),lambda:(profile/'.config/context7.json').rename(profile/'.config/moved'),lambda:(profile/'.config').rename(profile/'.config-moved')):\n"
                " try: operation(); blocked.append(False)\n"
                " except PermissionError: blocked.append(True)\n"
                "print(json.dumps({'adapter':projection['client_id']=='codex','readable':readable,'writable':all(writable),'immutable':all(blocked)}))\n"
            )
            result = subprocess.run([
                "/usr/bin/python3", "-c", child, str(service_uid), str(service_gid),
                json.dumps(item), json.dumps(approved),
            ], cwd=workspace, check=True, text=True, stdout=subprocess.PIPE)
            self.assertEqual(json.loads(result.stdout), {
                "adapter": True, "readable": True, "writable": True, "immutable": True,
            })

    @requires_disposable_observer_host
    def test_publication_failpoints_rollback_and_retry_both_profile_and_proof(self) -> None:
        boundaries = (
            "after_transaction_staging_create", "after_transaction_partial_write",
            "after_transaction_file_fsync", "after_transaction_rename", "after_transaction_fsync",
            "after_profile_staging_mkdir", "after_profile_staging_fsync",
            "after_staged_file_fsync", "after_staged_directory_fsync", "after_profile_copy", "after_profile_copy_fsync",
            "after_empty_profile_remove", "after_empty_profile_remove_fsync",
            "after_proof_staging_rename", "after_proof_staging_fsync",
            "after_proof_ownership", "after_profile_ownership",
            "after_profile_publish", "after_profile_publish_fsync",
            "after_proof_publish", "after_proof_publish_fsync",
            "after_transaction_commit_staging_create", "after_transaction_commit_partial_write",
            "after_transaction_commit_file_fsync", "after_transaction_commit_rename", "after_transaction_commit_fsync",
            "after_publication_record_fsync", "after_transaction_cleanup_fsync",
        )
        workspace = Path(__file__).parents[2]
        with tempfile.TemporaryDirectory(dir="/run", prefix="uap-observer-publication-") as temporary:
            root = Path(temporary)
            seed = root / "seed"
            seed.mkdir(mode=0o700)
            proof_seed = seed / self.helper.PROOF_SEED_NAME
            native_proof = proof_seed / "native"
            native_proof.mkdir(parents=True, mode=0o700)
            (proof_seed / "receipts.json").write_text(json.dumps({
                "schema_version": 1, "receipts": [{
                    "name": plugin, "tuple": sealed_tuple(plugin),
                    "manager_add_sha256": "sha256:" + "9" * 64,
                    "manager_info_sha256": "sha256:" + "a" * 64,
                    "post_add_doctor_sha256": "sha256:" + "b" * 64,
                } for plugin in sorted(fixed_adapters.HEROES)],
            }))
            (proof_seed / "receipts.json").chmod(0o400)
            (proof_seed / "native-projection.json").write_text(json.dumps({
                "schema_version": 2, "client_id": "codex", "entries": [{
                    "plugin": plugin,
                    "component_kind": "skill" if plugin == "agent-code-navigator" else "mcp",
                    "tuple": sealed_tuple(plugin),
                    "native_config": {
                        "path": f"/var/lib/uap-observer/proofs/codex/native/{plugin}.blob",
                        "sha256": fixed_adapters.sha256(b"{}"),
                    },
                    "client_config": {
                        "path": str(Path("/var/lib/uap-observer/profiles/codex") / native_fixture_relative(plugin)),
                        "sha256": fixed_adapters.sha256(b"{}"),
                    },
                    "manager_add_sha256": "sha256:" + "9" * 64,
                    "manager_info_sha256": "sha256:" + "a" * 64,
                    "post_add_doctor_sha256": "sha256:" + "b" * 64,
                } for plugin in sorted(fixed_adapters.HEROES)],
            }))
            (proof_seed / "native-projection.json").chmod(0o400)
            for plugin in ("agent-code-navigator", "context7", "cloudflare-docs", "chrome-devtools", "notion"):
                (native_proof / f"{plugin}.blob").write_text("{}")
                (native_proof / f"{plugin}.blob").chmod(0o400)
            for plugin in sorted(fixed_adapters.HEROES):
                active = seed / native_fixture_relative(plugin)
                active.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
                active.write_text("{}")
                active.chmod(0o600)
            account = mock.Mock(pw_uid=os.geteuid(), pw_gid=os.getegid())

            for boundary in boundaries:
                with self.subTest(boundary=boundary):
                    profile_root, proof_root = root / "profiles", root / "proofs"
                    profile_root.mkdir(mode=0o711)
                    proof_root.mkdir(mode=0o711)
                    (profile_root / "codex").mkdir(mode=0o700)
                    argv = ["provision", "--client", "codex", "--root-owned-seed", str(seed), "--seed-digest", "show"]
                    with mock.patch.object(self.helper, "PROFILE_ROOT", profile_root), mock.patch.object(self.helper, "PROOF_ROOT", proof_root), mock.patch.object(self.helper.pwd, "getpwnam", return_value=account), mock.patch("sys.argv", argv), mock.patch("sys.stdout") as output:
                        self.helper.main()
                        digest = output.write.call_args_list[0].args[0].strip()
                    argv[-1] = digest
                    def failpoint(name: str) -> None:
                        if name == boundary:
                            raise OSError(f"failpoint {name}")
                    with mock.patch.object(self.helper, "PROFILE_ROOT", profile_root), mock.patch.object(self.helper, "PROOF_ROOT", proof_root), mock.patch.object(self.helper.pwd, "getpwnam", return_value=account), mock.patch.object(self.helper, "checkpoint", side_effect=failpoint), mock.patch("sys.argv", argv), self.assertRaisesRegex(OSError, boundary):
                        self.helper.main()
                    self.assertTrue((profile_root / "codex").is_dir())
                    self.assertEqual(list((profile_root / "codex").iterdir()), [])
                    self.assertEqual(stat.S_IMODE((profile_root / "codex").stat().st_mode), 0o700)
                    self.assertFalse((proof_root / "codex").exists())
                    with mock.patch.object(self.helper, "PROFILE_ROOT", profile_root), mock.patch.object(self.helper, "PROOF_ROOT", proof_root), mock.patch.object(self.helper.pwd, "getpwnam", return_value=account), mock.patch("sys.argv", argv):
                        self.helper.main()
                    self.assertTrue((profile_root / "codex" / "active-context7.json").is_file())
                    self.assertTrue((proof_root / "codex" / "native-projection.json").is_file())
                    self.assertEqual(stat.S_IMODE((profile_root / "codex").stat().st_mode), 0o510)
                    self.assertEqual(stat.S_IMODE((profile_root / "codex" / "active-context7.json").stat().st_mode), 0o440)
                    shutil.rmtree(profile_root)
                    shutil.rmtree(proof_root)

            rollback_cases = {
                "after_transaction_rollback_staging_create": "after_publication_record_fsync",
                "after_transaction_rollback_partial_write": "after_publication_record_fsync",
                "after_transaction_rollback_file_fsync": "after_publication_record_fsync",
                "after_transaction_rollback_rename": "after_publication_record_fsync",
                "after_transaction_rollback_fsync": "after_publication_record_fsync",
                "after_rollback_profile_staging_removal": "after_profile_copy",
                "after_rollback_proof_staging_removal": "after_proof_staging_fsync",
                "after_rollback_profile_removal": "after_publication_record_fsync",
                "after_rollback_proof_removal": "after_publication_record_fsync",
                "after_rollback_empty_profile_restore": "after_publication_record_fsync",
                "after_rollback_publication_restore": "after_publication_record_fsync",
                "after_rollback_marker_cleanup_fsync": "after_publication_record_fsync",
            }
            for boundary, trigger in rollback_cases.items():
                with self.subTest(rollback_boundary=boundary):
                    profile_root, proof_root = root / "profiles", root / "proofs"
                    profile_root.mkdir(mode=0o711)
                    proof_root.mkdir(mode=0o711)
                    (profile_root / "codex").mkdir(mode=0o700)
                    argv = ["provision", "--client", "codex", "--root-owned-seed", str(seed), "--seed-digest", "show"]
                    with mock.patch.object(self.helper, "PROFILE_ROOT", profile_root), mock.patch.object(self.helper, "PROOF_ROOT", proof_root), mock.patch.object(self.helper.pwd, "getpwnam", return_value=account), mock.patch("sys.argv", argv), mock.patch("sys.stdout") as output:
                        self.helper.main()
                        digest = output.write.call_args_list[0].args[0].strip()
                    argv[-1] = digest
                    rollback_started = False
                    def interrupt_rollback(name: str) -> None:
                        nonlocal rollback_started
                        if name == trigger and not rollback_started:
                            rollback_started = True
                            raise OSError(f"begin rollback at {trigger}")
                        if rollback_started and name == boundary:
                            raise OSError(f"interrupt rollback at {boundary}")
                    with mock.patch.object(self.helper, "PROFILE_ROOT", profile_root), mock.patch.object(self.helper, "PROOF_ROOT", proof_root), mock.patch.object(self.helper.pwd, "getpwnam", return_value=account), mock.patch.object(self.helper, "checkpoint", side_effect=interrupt_rollback), mock.patch("sys.argv", argv), self.assertRaisesRegex(OSError, boundary):
                        self.helper.main()
                    marker = profile_root / ".codex.transaction"
                    self.assertTrue(
                        marker.exists() or boundary == "after_rollback_marker_cleanup_fsync",
                        "rollback journal disappeared before restoration completed",
                    )
                    profile_fd = os.open(profile_root, self.helper.OPEN_DIRECTORY)
                    proof_fd = os.open(proof_root, self.helper.OPEN_DIRECTORY)
                    lock_fd = os.open(profile_root / ".codex.lock", os.O_RDWR | os.O_NOFOLLOW)
                    try:
                        self.assertFalse(self.helper.recover_transaction(
                            profile_fd, proof_fd, "codex", marker.name, digest,
                            account.pw_uid, account.pw_gid, lock_fd,
                        ))
                        self.assertFalse(self.helper.recover_transaction(
                            profile_fd, proof_fd, "codex", marker.name, digest,
                            account.pw_uid, account.pw_gid, lock_fd,
                        ))
                    finally:
                        os.close(lock_fd); os.close(proof_fd); os.close(profile_fd)
                    self.assertTrue((profile_root / "codex").is_dir())
                    self.assertEqual(list((profile_root / "codex").iterdir()), [])
                    self.assertFalse((proof_root / "codex").exists())
                    self.assertFalse(marker.exists())
                    self.assertFalse((profile_root / ".codex.transaction.new").exists())
                    with mock.patch.object(self.helper, "PROFILE_ROOT", profile_root), mock.patch.object(self.helper, "PROOF_ROOT", proof_root), mock.patch.object(self.helper.pwd, "getpwnam", return_value=account), mock.patch("sys.argv", argv):
                        self.helper.main()
                    self.assertTrue((profile_root / "codex" / "active-context7.json").is_file())
                    self.assertTrue((proof_root / "codex" / "native-projection.json").is_file())
                    shutil.rmtree(profile_root)
                    shutil.rmtree(proof_root)

    @requires_disposable_observer_host
    def test_hard_termination_at_every_persistence_boundary_recovers_in_fresh_process(self) -> None:
        boundaries = (
            "after_transaction_staging_create", "after_transaction_partial_write",
            "after_transaction_file_fsync", "after_transaction_rename", "after_transaction_fsync",
            "after_profile_staging_mkdir", "after_profile_staging_fsync",
            "after_staged_file_fsync", "after_staged_directory_fsync", "after_profile_copy", "after_profile_copy_fsync",
            "after_empty_profile_remove", "after_empty_profile_remove_fsync",
            "after_proof_staging_rename", "after_proof_staging_fsync",
            "after_proof_ownership", "after_profile_ownership",
            "after_profile_publish", "after_profile_publish_fsync",
            "after_proof_publish", "after_proof_publish_fsync",
            "after_transaction_commit_staging_create", "after_transaction_commit_partial_write",
            "after_transaction_commit_file_fsync", "after_transaction_commit_rename", "after_transaction_commit_fsync",
            "after_publication_record_fsync", "after_transaction_cleanup_fsync",
        )
        workspace = Path(__file__).parents[2]
        helper_path = workspace / "deploy" / "uap-observer-provision-profile.py"
        with tempfile.TemporaryDirectory(dir="/run", prefix="uap-observer-recovery-") as temporary:
            root = Path(temporary)
            seed = root / "seed"
            proof_seed = seed / self.helper.PROOF_SEED_NAME
            native_proof = proof_seed / "native"
            native_proof.mkdir(parents=True, mode=0o700)
            (proof_seed / "receipts.json").write_text(json.dumps({
                "schema_version": 1, "receipts": [{
                    "name": plugin, "tuple": sealed_tuple(plugin),
                    "manager_add_sha256": "sha256:" + "9" * 64,
                    "manager_info_sha256": "sha256:" + "a" * 64,
                    "post_add_doctor_sha256": "sha256:" + "b" * 64,
                } for plugin in sorted(fixed_adapters.HEROES)],
            }))
            (proof_seed / "receipts.json").chmod(0o400)
            (proof_seed / "native-projection.json").write_text(json.dumps({
                "schema_version": 2, "client_id": "codex", "entries": [{
                    "plugin": plugin,
                    "component_kind": "skill" if plugin == "agent-code-navigator" else "mcp",
                    "tuple": sealed_tuple(plugin),
                    "native_config": {
                        "path": f"/var/lib/uap-observer/proofs/codex/native/{plugin}.blob",
                        "sha256": fixed_adapters.sha256(b"{}"),
                    },
                    "client_config": {
                        "path": str(Path("/var/lib/uap-observer/profiles/codex") / native_fixture_relative(plugin)),
                        "sha256": fixed_adapters.sha256(b"{}"),
                    },
                    "manager_add_sha256": "sha256:" + "9" * 64,
                    "manager_info_sha256": "sha256:" + "a" * 64,
                    "post_add_doctor_sha256": "sha256:" + "b" * 64,
                } for plugin in sorted(fixed_adapters.HEROES)],
            }))
            (proof_seed / "native-projection.json").chmod(0o400)
            for plugin in ("agent-code-navigator", "context7", "cloudflare-docs", "chrome-devtools", "notion"):
                (native_proof / f"{plugin}.blob").write_text("{}")
                (native_proof / f"{plugin}.blob").chmod(0o400)
            for plugin in sorted(fixed_adapters.HEROES):
                active = seed / native_fixture_relative(plugin)
                active.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
                active.write_text("{}")
                active.chmod(0o600)
            launcher = (
                "import importlib.util,os,sys,types\n"
                "from pathlib import Path\n"
                f"spec=importlib.util.spec_from_file_location('provision', {str(helper_path)!r})\n"
                "module=importlib.util.module_from_spec(spec); spec.loader.exec_module(module)\n"
                "module.PROFILE_ROOT=Path(sys.argv[1]); module.PROOF_ROOT=Path(sys.argv[2])\n"
                "module.pwd.getpwnam=lambda _name: types.SimpleNamespace(pw_uid=os.geteuid(),pw_gid=os.getegid())\n"
                "boundary=sys.argv[3]\n"
                "module.checkpoint=lambda name: os._exit(97) if name == boundary else None\n"
                "sys.argv=['provision','--client','codex','--root-owned-seed',sys.argv[4],'--seed-digest',sys.argv[5]]\n"
                "raise SystemExit(module.main())\n"
            )
            digest_process = subprocess.run(
                ["/usr/bin/python3", "-c", launcher, str(root / "unused-profiles"), str(root / "unused-proofs"), "none", str(seed), "show"],
                check=True, text=True, stdout=subprocess.PIPE,
            )
            digest = digest_process.stdout.strip()
            for boundary in boundaries:
                with self.subTest(boundary=boundary):
                    profile_root, proof_root = root / f"profiles-{boundary}", root / f"proofs-{boundary}"
                    profile_root.mkdir(mode=0o711)
                    proof_root.mkdir(mode=0o711)
                    (profile_root / "codex").mkdir(mode=0o700)
                    crashed = subprocess.run([
                        "/usr/bin/python3", "-c", launcher, str(profile_root), str(proof_root), boundary, str(seed), digest,
                    ])
                    self.assertEqual(crashed.returncode, 97)
                    subprocess.run([
                        "/usr/bin/python3", "-c", launcher, str(profile_root), str(proof_root), "none", str(seed), digest,
                    ], check=True)
                    self.assertTrue((profile_root / "codex" / "active-context7.json").is_file())
                    self.assertTrue((proof_root / "codex" / "native-projection.json").is_file())
                    self.assertEqual(stat.S_IMODE((profile_root / "codex").stat().st_mode), 0o510)
                    self.assertEqual(stat.S_IMODE((profile_root / "codex" / "active-context7.json").stat().st_mode), 0o440)

    @requires_disposable_observer_host
    def test_two_process_serialization_recovers_after_sigkill_to_one_publication(self) -> None:
        workspace = Path(__file__).parents[2]
        helper_path = workspace / "deploy" / "uap-observer-provision-profile.py"
        with tempfile.TemporaryDirectory(dir="/run", prefix="uap-observer-serialization-") as temporary:
            root = Path(temporary)
            seed = root / "seed"
            proof_seed = seed / self.helper.PROOF_SEED_NAME
            native_proof = proof_seed / "native"
            native_proof.mkdir(parents=True, mode=0o700)
            (proof_seed / "receipts.json").write_text(json.dumps({
                "schema_version": 1, "receipts": [{
                    "name": plugin, "tuple": sealed_tuple(plugin),
                    "manager_add_sha256": "sha256:" + "9" * 64,
                    "manager_info_sha256": "sha256:" + "a" * 64,
                    "post_add_doctor_sha256": "sha256:" + "b" * 64,
                } for plugin in sorted(fixed_adapters.HEROES)],
            }))
            (proof_seed / "native-projection.json").write_text(json.dumps({
                "schema_version": 2, "client_id": "codex", "entries": [{
                    "plugin": plugin,
                    "component_kind": "skill" if plugin == "agent-code-navigator" else "mcp",
                    "tuple": sealed_tuple(plugin),
                    "native_config": {
                        "path": f"/var/lib/uap-observer/proofs/codex/native/{plugin}.blob",
                        "sha256": fixed_adapters.sha256(b"{}"),
                    },
                    "client_config": {
                        "path": str(Path("/var/lib/uap-observer/profiles/codex") / native_fixture_relative(plugin)),
                        "sha256": fixed_adapters.sha256(b"{}"),
                    },
                    "manager_add_sha256": "sha256:" + "9" * 64,
                    "manager_info_sha256": "sha256:" + "a" * 64,
                    "post_add_doctor_sha256": "sha256:" + "b" * 64,
                } for plugin in sorted(fixed_adapters.HEROES)],
            }))
            for plugin in sorted(fixed_adapters.HEROES):
                (native_proof / f"{plugin}.blob").write_text("{}")
            for plugin in sorted(fixed_adapters.HEROES):
                active = seed / native_fixture_relative(plugin)
                active.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
                active.write_text("{}")
            profile_root, proof_root = root / "profiles", root / "proofs"
            profile_root.mkdir(mode=0o711)
            proof_root.mkdir(mode=0o711)
            (profile_root / "codex").mkdir(mode=0o700)
            ready = root / "holder-ready"
            launcher = (
                "import importlib.util,os,signal,sys,types\n"
                "from pathlib import Path\n"
                f"spec=importlib.util.spec_from_file_location('provision', {str(helper_path)!r})\n"
                "module=importlib.util.module_from_spec(spec); spec.loader.exec_module(module)\n"
                "module.PROFILE_ROOT=Path(sys.argv[1]); module.PROOF_ROOT=Path(sys.argv[2])\n"
                "module.pwd.getpwnam=lambda _name: types.SimpleNamespace(pw_uid=os.geteuid(),pw_gid=os.getegid())\n"
                "mode=sys.argv[6]; ready=Path(sys.argv[7])\n"
                "def checkpoint(name):\n"
                " if mode=='hold' and name=='after_transaction_fsync': ready.write_text('ready'); signal.pause()\n"
                "module.checkpoint=checkpoint\n"
                "sys.argv=['provision','--client','codex','--root-owned-seed',sys.argv[3],'--seed-digest',sys.argv[4]]\n"
                "raise SystemExit(module.main())\n"
            )
            digest = subprocess.run([
                "/usr/bin/python3", "-c", launcher, str(profile_root), str(proof_root),
                str(seed), "show", "unused", "normal", str(ready),
            ], check=True, text=True, stdout=subprocess.PIPE).stdout.strip()
            base = [
                "/usr/bin/python3", "-c", launcher, str(profile_root), str(proof_root),
                str(seed), digest, "unused",
            ]
            holder = subprocess.Popen([*base, "hold", str(ready)], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            deadline = time.monotonic() + 5
            while not ready.exists() and holder.poll() is None and time.monotonic() < deadline:
                time.sleep(0.01)
            self.assertTrue(ready.exists(), holder.stderr.read().decode() if holder.poll() is not None else "holder did not reach persistence boundary")
            before = sorted(path.name for path in profile_root.iterdir())
            waiter = subprocess.Popen([*base, "normal", str(ready)], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            time.sleep(0.2)
            self.assertIsNone(waiter.poll(), "second provisioner bypassed the per-client lock")
            self.assertEqual(sorted(path.name for path in profile_root.iterdir()), before)
            os.kill(holder.pid, signal.SIGKILL)
            holder.wait(timeout=5)
            holder.communicate()
            waiter_stdout, waiter_stderr = waiter.communicate(timeout=10)
            self.assertEqual(waiter.returncode, 0, waiter_stderr.decode())
            self.assertIn(b"profile provisioned", waiter_stdout)
            self.assertEqual([path.name for path in profile_root.iterdir() if path.is_dir()], ["codex"])
            self.assertEqual([path.name for path in proof_root.iterdir() if path.is_dir()], ["codex"])
            self.assertFalse(any(".new" in path.name or "transaction" in path.name for path in (*profile_root.iterdir(), *proof_root.iterdir())))
            active_inode = (profile_root / "codex" / "active-context7.json").stat().st_ino
            fresh = subprocess.run([*base, "normal", str(ready)], check=True, text=True, stdout=subprocess.PIPE)
            self.assertIn("already provisioned", fresh.stdout)
            self.assertEqual((profile_root / "codex" / "active-context7.json").stat().st_ino, active_inode)
            self.assertFalse(any(".new" in path.name or "transaction" in path.name for path in (*profile_root.iterdir(), *proof_root.iterdir())))


if __name__ == "__main__":
    unittest.main()
