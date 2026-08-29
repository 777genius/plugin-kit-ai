from __future__ import annotations

import base64
import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from datetime import datetime, timedelta, timezone
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
import materialize_launch_evidence as evidence
import launch_observer_signatures as signatures
import prepare_directory_publication as prepare


GIT = "/usr/bin/git"


def git(repo: Path, *args: str) -> str:
    return subprocess.check_output([GIT, "-C", str(repo), *args], text=True).strip()


def tuple_value(plugin: str, client: str, ordinal: int) -> dict:
    revision = hashlib.sha1(f"revision-{plugin}".encode()).hexdigest()
    return {
        "product_id": plugin,
        "tree_digest": "sha256:" + hashlib.sha256(f"tree-{plugin}".encode()).hexdigest(),
        "manifest_digest": "sha256:" + hashlib.sha256(f"manifest-{plugin}".encode()).hexdigest(),
        "distribution_id": f"publisher/{plugin}",
        "distribution_kind": "community",
        "release_sequence": 1,
        "package_version": "1.0.0",
        "source_repository": "owner/repository",
        "source_revision": revision,
        "source_path": f"plugins/{plugin}",
        "snapshot_sequence": 1,
        "snapshot_digest": "sha256:" + "1" * 64,
        "binary_digest": "sha256:" + "2" * 64,
        "dependency_identity": f"dependency-{plugin}",
        "installer_version": "0.1.12",
        "adapter_version": "0.1.12",
        "client_version": f"client-{client}-1",
        "os": "linux",
        "architecture": "amd64",
        "observed_at": f"2026-08-23T00:{ordinal:02d}:00Z",
    }


def authoritative_rows() -> list[dict]:
    rows = []
    ordinal = 0
    for plugin in sorted(evidence.HEROES):
        for client in sorted(evidence.HERO_CLIENTS):
            rows.append({
                "id": f"{ordinal:024x}", "scenario": "hero_5x3_runtime",
                "plugin": plugin, "client": client, "level": "runtime",
                "outcome": "passed", "tuple": tuple_value(plugin, client, ordinal),
                "reason": "observed", "details": {
                    "evidence_basis": "protected_external_observer",
                    "native_discovery_proof": True,
                },
            })
            ordinal += 1
    rows.append({
        "id": f"{ordinal:024x}", "scenario": "chatgpt_registered_binding",
        "plugin": "cloudflare-docs", "client": "chatgpt", "level": "runtime",
        "outcome": "passed", "tuple": tuple_value("cloudflare-docs", "chatgpt", ordinal),
        "reason": "observed", "details": {
            "evidence_basis": "protected_external_observer",
            "native_discovery_proof": False,
            "public_mcp_proof": True,
        },
    })
    return rows


class LaunchEvidenceBundleTests(unittest.TestCase):
    def launch(self, observer: bytes) -> dict:
        return {
            "schema_version": 3,
            "run": {
                "mode": "enforced", "runtime_claims": True,
                "github_sha": "a" * 40, "github_run_id": "123",
                "github_run_attempt": "2",
                "caller_event_name": "push", "caller_ref": "refs/heads/main",
                "caller_workflow_ref": "owner/repository/.github/workflows/directory-publication.yml@refs/heads/main",
                "observer_bundle_digest": evidence.sha256(observer),
            },
            "release": {
                "manifest_digest": "sha256:" + "e" * 64,
                "checksums_digest": "sha256:" + "f" * 64,
            },
            "directory": {"sequence": 7, "snapshot_digest": "sha256:" + "b" * 64},
            "scenario_contract": {"digest": "sha256:" + "9" * 64},
            "matrix": authoritative_rows(),
            "summary": {"required_gates_complete": True, "hero_runtime_results": 15},
        }

    def build(self, root: Path):
        observer = evidence.canonical_json({"schema_version": 1, "signed": True})
        (root / "launch-evidence.json").write_text(json.dumps(self.launch(observer), indent=2) + "\n")
        (root / "signed-observer-bundle.json").write_bytes(observer)
        with mock.patch.object(evidence, "validate_with_schema"):
            return evidence.build_bundle(
                root, repository="owner/repository",
                workflow="owner/repository/.github/workflows/launch-evidence-e2e.yml",
                source_ref="refs/heads/main", source_digest="a" * 40,
                expected_run_id="123", expected_run_attempt="2", expected_publication_id="456",
                expected_caller_event_name="push", expected_caller_ref="refs/heads/main",
                expected_caller_workflow_ref="owner/repository/.github/workflows/directory-publication.yml@refs/heads/main",
                expected_sequence=7, expected_snapshot_digest="sha256:" + "b" * 64,
                expected_source_commit="c" * 40,
            )

    def test_selects_only_exact_authoritative_runtime_and_oauth_matrix(self) -> None:
        rows = authoritative_rows()
        rows.append({"scenario": "hero_5x3_lifecycle", "level": "materialization"})
        selected = evidence.selected_rows({"matrix": rows})
        self.assertEqual(len(selected), 16)
        self.assertEqual(sum(row["level"] == "runtime" for row in selected), 16)
        self.assertEqual(sum(row["client"] == "chatgpt" and row["level"] == "runtime" for row in selected), 1)

    def test_attester_verifies_signature_and_exact_signed_authoritative_rows(self) -> None:
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

        launch = self.launch(b"placeholder")
        launch["run"]["challenge"] = "d" * 64
        for row in launch["matrix"]:
            row["details"]["runtime_proof"] = True
            row["details"]["run_id"] = "123"
            row["details"]["run_attempt"] = "2"
            row["details"]["resolution"] = {"derived": True}
        artifacts = {
            "runtime-attestations.json": {"schema_version": 1, "attestations": []},
            "notion-oauth-attestations.json": {"schema_version": 1, "attestations": []},
            "chatgpt-cloudflare-attestation.json": {"schema_version": 1, "attestations": []},
            "consent.json": {"schema_version": 1},
        }
        for row in launch["matrix"]:
            record = {
                "plugin": row["plugin"], "client": row["client"], "level": row["level"],
                "outcome": "passed", "reason": row["reason"], "tuple": row["tuple"], "challenge": "d" * 64,
                "run_id": "123", "run_attempt": "2",
                "release_manifest_digest": launch["release"]["manifest_digest"],
                "release_checksums_digest": launch["release"]["checksums_digest"],
                "directory_digest": launch["directory"]["snapshot_digest"],
                "scenario_contract_digest": launch["scenario_contract"]["digest"],
                "github_attestation": {
                    "repository": "owner/repository", "sha": "a" * 40,
                    "run_id": "123", "run_attempt": "2", "workflow": "launch-evidence-e2e.yml",
                    "job": "protected-observer-inputs", "challenge": "d" * 64,
                },
            }
            row["details"].update({
                field: record[field]
                for field in evidence.AUTHORITATIVE_DETAIL_FIELDS
                if field in record
            })
            name = (
                "chatgpt-cloudflare-attestation.json" if row["client"] == "chatgpt"
                else "notion-oauth-attestations.json" if row["plugin"] == "notion"
                else "runtime-attestations.json"
            )
            artifacts[name]["attestations"].append(record)
        private_key = Ed25519PrivateKey.generate()
        public_key = base64.b64encode(private_key.public_key().public_bytes_raw()).decode()
        bundle = {
            "schema_version": 1, "challenge": "d" * 64,
            "signed_at": datetime.now(timezone.utc).replace(microsecond=0).isoformat(),
            "key_id": "observer-v1", "artifacts": artifacts,
        }
        bundle["signature"] = base64.b64encode(private_key.sign(signatures.signed_payload(bundle))).decode()
        schema_patch = mock.patch.object(evidence, "validate_observer_artifact_schemas")
        schema_patch.start()
        self.addCleanup(schema_patch.stop)
        evidence.verify_authoritative_observer_rows(
            launch, bundle, repository="owner/repository",
            public_key=public_key, key_id="observer-v1",
        )
        delayed = {**bundle, "signed_at": (datetime.now(timezone.utc) - timedelta(hours=2)).replace(microsecond=0).isoformat()}
        delayed["signature"] = base64.b64encode(private_key.sign(signatures.signed_payload(delayed))).decode()
        with self.assertRaisesRegex(ValueError, "stale"):
            evidence.verify_authoritative_observer_rows(
                launch, delayed, repository="owner/repository",
                public_key=public_key, key_id="observer-v1",
            )
        evidence.verify_authoritative_observer_rows(
            launch, delayed, repository="owner/repository",
            public_key=public_key, key_id="observer-v1", enforce_freshness=False,
        )
        forged = json.loads(json.dumps(launch))
        forged["matrix"][0]["tuple"]["client_version"] = "forged"
        with self.assertRaisesRegex(evidence.EvidenceError, "differs from signed observer"):
            evidence.verify_authoritative_observer_rows(
                forged, bundle, repository="owner/repository",
                public_key=public_key, key_id="observer-v1",
            )
        forged_details = json.loads(json.dumps(launch))
        forged_details["matrix"][0]["details"]["github_attestation"]["job"] = "enforced-stable-gate"
        with self.assertRaisesRegex(evidence.EvidenceError, "differs from signed observer"):
            evidence.verify_authoritative_observer_rows(
                forged_details, bundle, repository="owner/repository",
                public_key=public_key, key_id="observer-v1",
            )
        tampered = {**bundle, "signature": base64.b64encode(b"x" * 64).decode()}
        with self.assertRaisesRegex(ValueError, "signature is invalid"):
            evidence.verify_authoritative_observer_rows(
                launch, tampered, repository="owner/repository",
                public_key=public_key, key_id="observer-v1",
            )
        rebound = json.loads(json.dumps(bundle))
        rebound["artifacts"]["runtime-attestations.json"]["attestations"][0]["scenario_contract_digest"] = "sha256:" + "8" * 64
        rebound["signature"] = base64.b64encode(private_key.sign(signatures.signed_payload(rebound))).decode()
        with self.assertRaisesRegex(evidence.EvidenceError, "protected OIDC job"):
            evidence.verify_authoritative_observer_rows(
                launch, rebound, repository="owner/repository",
                public_key=public_key, key_id="observer-v1",
            )

    def test_duplicate_applicability_is_rejected(self) -> None:
        rows = authoritative_rows()
        rows[-1] = dict(rows[-1], id="different")
        rows.append(dict(rows[-1]))
        with self.assertRaisesRegex(evidence.EvidenceError, "expected 16|duplicate"):
            evidence.selected_rows({"matrix": rows})

    def test_wrong_attempt_event_or_caller_is_rejected(self) -> None:
        observer = evidence.canonical_json({"schema_version": 1, "signed": True})
        for field, value in (
            ("github_run_attempt", "3"),
            ("caller_event_name", "workflow_dispatch"),
            ("caller_ref", "refs/heads/feature"),
            ("caller_workflow_ref", "owner/repository/.github/workflows/live-e2e.yml@refs/heads/main"),
        ):
            with self.subTest(field=field), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                launch = self.launch(observer)
                launch["run"][field] = value
                (root / "launch-evidence.json").write_bytes(evidence.canonical_json(launch))
                (root / "signed-observer-bundle.json").write_bytes(observer)
                with mock.patch.object(evidence, "validate_with_schema"), self.assertRaisesRegex(
                    evidence.EvidenceError, "mismatch",
                ):
                    evidence.build_bundle(
                        root, repository="owner/repository", workflow="owner/repository/.github/workflows/launch-evidence-e2e.yml",
                        source_ref="refs/heads/main", source_digest="a" * 40,
                        expected_run_id="123", expected_run_attempt="2",
                        expected_caller_event_name="push", expected_caller_ref="refs/heads/main",
                        expected_caller_workflow_ref="owner/repository/.github/workflows/directory-publication.yml@refs/heads/main",
                        expected_publication_id="456", expected_sequence=7,
                        expected_snapshot_digest="sha256:" + "b" * 64,
                        expected_source_commit="c" * 40,
                    )

    def test_bundle_is_canonical_bounded_and_checksum_complete(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            digest, files = self.build(root)
            self.assertEqual(digest, evidence.sha256(files["launch-evidence.json"]))
            self.assertEqual(len([path for path in files if path.startswith("directory-evidence/records/")]), 16)
            self.assertEqual(
                files["launch-evidence.json"],
                evidence.canonical_json(json.loads(files["launch-evidence.json"])),
            )
            expected_checksums = evidence.checksum_bytes({key: value for key, value in files.items() if key != "SHA256SUMS"})
            self.assertEqual(files["SHA256SUMS"], expected_checksums)
            evidence.write_bundle(root, files)
            evidence.verify_exact_bundle(root, files)
            (root / "extra.json").write_text("{}\n")
            with self.assertRaisesRegex(evidence.EvidenceError, "paths differ"):
                evidence.verify_exact_bundle(root, files)

    def test_attestation_verification_uses_the_exact_offline_bundle(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            _, files = self.build(root)
            evidence.write_bundle(root, files)
            attestation = root / "provenance.jsonl"
            attestation.write_text('{"bundle":true}\n')
            verifier = ROOT / "scripts" / "materialize_launch_evidence.py"
            with mock.patch.object(evidence, "GH", str(verifier)), mock.patch.object(
                evidence.subprocess, "run", return_value=subprocess.CompletedProcess([], 0),
            ) as run:
                evidence.verify_attestation(
                    root, bundle_path=attestation, repository="owner/repository",
                    workflow="owner/repository/.github/workflows/launch-evidence-e2e.yml",
                    source_ref="refs/heads/main", source_digest="a" * 40,
                )
            command = run.call_args.args[0]
            self.assertEqual(command[command.index("--bundle") + 1], str(attestation))
            self.assertNotIn("GH_TOKEN", run.call_args.kwargs.get("env", {}))

    def test_every_bundle_layer_is_rederived_even_after_local_checksum_rewrite(self) -> None:
        for target in (
            "launch-evidence.json", "signed-observer-bundle.json",
            "bundle-identity.json", "directory-evidence/index.json", "leaf",
        ):
            with self.subTest(target=target), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                _, files = self.build(root)
                evidence.write_bundle(root, files)
                path = target
                if target == "leaf":
                    path = next(name for name in files if name.startswith("directory-evidence/records/"))
                (root / path).write_bytes((root / path).read_bytes() + b" ")
                actual = {
                    item.relative_to(root).as_posix(): item.read_bytes()
                    for item in root.rglob("*") if item.is_file() and item.name != "SHA256SUMS"
                }
                (root / "SHA256SUMS").write_bytes(evidence.checksum_bytes(actual))
                with self.assertRaises(evidence.EvidenceError):
                    _, expected = self.build(root)
                    evidence.verify_exact_bundle(root, expected)


class PermanentCommitTests(unittest.TestCase):
    def setUp(self) -> None:
        self.validator_patch = mock.patch.object(evidence, "validate_directory")
        self.validate_directory = self.validator_patch.start()
        self.temporary = tempfile.TemporaryDirectory()
        self.repo = Path(self.temporary.name) / "repo"
        self.repo.mkdir()
        git(self.repo, "init", "-q")
        git(self.repo, "config", "user.name", "test")
        git(self.repo, "config", "user.email", "test@example.com")
        (self.repo / "registry").mkdir()
        directory = {
            "schema_version": 1,
            "products": [],
            "distributions": [{
                "id": "publisher/demo", "product_id": "demo",
                "release_policies": [{"release_sequence": 1, "current_evidence": []}],
            }],
            "evidence": [],
        }
        (self.repo / "registry" / "directory.json").write_bytes(evidence.canonical_json(directory))
        git(self.repo, "add", "registry/directory.json")
        git(self.repo, "commit", "-qm", "base")
        self.parent = git(self.repo, "rev-parse", "HEAD")

    def tearDown(self) -> None:
        self.temporary.cleanup()
        self.validator_patch.stop()

    def files(self, publication_id: str = "123") -> tuple[str, dict[str, bytes]]:
        record = {
            "schema_version": 1, "id": "launch/demo/codex/runtime/0123456789abcdef01234567",
            "product_id": "demo", "distribution_id": "publisher/demo", "release_sequence": 1,
            "package_tree_digest": "sha256:" + "1" * 64,
            "manifest_digest": "sha256:" + "2" * 64,
            "source_repository": "owner/repository", "source_revision": "3" * 40,
            "source_path": "plugins/demo", "level": "runtime", "outcome": "passed",
            "client": "codex", "client_version": "1", "installer_version": "0.1.12",
            "adapter_version": "0.1.12", "os": "linux", "architecture": "amd64",
            "dependency_identity": "dependency", "observed_at": "2026-08-23T00:00:00Z",
        }
        path = "directory-evidence/records/demo-codex-runtime.json"
        launch = evidence.canonical_json({"launch": True})
        record_body = evidence.canonical_json(record)
        digest = evidence.sha256(launch)
        observer = evidence.canonical_json({"bundle": True})
        index = {
            "schema_version": 1, "launch_evidence_digest": digest,
            "signed_observer_bundle_digest": evidence.sha256(observer),
            "repository": "owner/repository",
            "workflow": "owner/repository/.github/workflows/launch-evidence-e2e.yml",
            "source_ref": "refs/heads/main", "source_digest": "4" * 40,
            "workflow_run_id": "123", "workflow_run_attempt": "2",
            "caller_event_name": "push", "caller_ref": "refs/heads/main",
            "caller_workflow_ref": "owner/repository/.github/workflows/directory-publication.yml@refs/heads/main",
            "publication_id": publication_id, "publication_sequence": 7,
            "publication_snapshot_digest": "sha256:" + "5" * 64,
            "publication_source_commit": self.parent,
            "records": [{"id": record["id"], "path": path, "digest": evidence.sha256(record_body)}],
        }
        files = {
            "launch-evidence.json": launch,
            "signed-observer-bundle.json": observer,
            path: record_body,
            "directory-evidence/index.json": evidence.canonical_json(index),
        }
        files["bundle-identity.json"] = evidence.canonical_json({
            "schema_version": 1, "launch_evidence_digest": digest,
            "signed_observer_bundle_digest": evidence.sha256(observer),
            "directory_evidence_index_digest": evidence.sha256(files["directory-evidence/index.json"]),
            "records": index["records"],
        })
        files[evidence.ATTESTATION_BUNDLE_NAME] = b'{"bundle":true}\n'
        files["SHA256SUMS"] = evidence.checksum_bytes(files)
        return digest, files

    def test_two_ref_commits_are_bounded_and_acyclic(self) -> None:
        digest, files = self.files()
        result = evidence.materialize_commits(
            self.repo, self.repo, self.repo, files, repository="owner/repository",
            main_parent=self.parent, ledger_parent=self.parent,
            approval_target=self.parent, digest=digest,
        )
        ledger = result["ledger_commit"]
        main = result["main_commit"]
        self.assertEqual(git(self.repo, "show", "-s", "--format=%P", ledger), self.parent)
        self.assertEqual(git(self.repo, "show", "-s", "--format=%P", main), self.parent)
        self.assertEqual(git(self.repo, "diff", "--name-only", self.parent, main), "registry/directory.json")
        self.assertEqual(
            subprocess.run(
                [GIT, "-C", str(self.repo), "grep", "-F", ledger, ledger],
                stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            ).returncode,
            1,
        )
        source = json.loads(subprocess.check_output([
            GIT, "-C", str(self.repo), "show", f"{main}:registry/directory.json",
        ]))
        pointer = source["evidence"][0]
        self.assertEqual(pointer["artifact"]["revision"], ledger)
        self.assertEqual(pointer["trust"]["source_digest"], "4" * 40)
        self.assertTrue(pointer["trust"]["bundle_manifest"]["path"].endswith("/bundle-identity.json"))
        self.assertIn("launch-evidence.json", git(self.repo, "ls-tree", "-r", "--name-only", ledger))
        self.assertIn("signed-observer-bundle.json", git(self.repo, "ls-tree", "-r", "--name-only", ledger))
        self.validate_directory.assert_called_once()
        self.assertEqual(result["approval_target"], self.parent)

    def test_full_directory_validation_blocks_commit_before_cas(self) -> None:
        digest, files = self.files()
        self.validate_directory.side_effect = evidence.RegistryError("evidence target mismatch")
        with self.assertRaisesRegex(evidence.EvidenceError, "full Directory validation"):
            evidence.materialize_commits(
                self.repo, self.repo, self.repo, files, repository="owner/repository",
                main_parent=self.parent, ledger_parent=self.parent,
                approval_target=self.parent, digest=digest,
            )

    def test_evidence_parent_may_follow_approval_with_discovery_only_changes(self) -> None:
        (self.repo / "discovery").mkdir()
        (self.repo / "discovery" / "latest.json").write_text("{}\n")
        git(self.repo, "add", "discovery/latest.json")
        git(self.repo, "commit", "-qm", "chore(discovery): publish sequence 2")
        discovery_parent = git(self.repo, "rev-parse", "HEAD")
        digest, files = self.files()
        result = evidence.materialize_commits(
            self.repo, self.repo, self.repo, files, repository="owner/repository",
            main_parent=discovery_parent, ledger_parent=discovery_parent,
            approval_target=self.parent, digest=digest,
        )
        self.assertEqual(result["ledger_parent"], discovery_parent)
        self.assertEqual(result["approval_target"], self.parent)
        self.assertEqual(
            git(self.repo, "show", "-s", "--format=%P", result["ledger_commit"]),
            discovery_parent,
        )

    def test_whole_run_retry_accepts_only_exact_persisted_state(self) -> None:
        digest, files = self.files(publication_id="456")
        result = evidence.materialize_commits(
            self.repo, self.repo, self.repo, files, repository="owner/repository",
            main_parent=self.parent, ledger_parent=self.parent,
            approval_target=self.parent, digest=digest,
        )
        git(self.repo, "tag", "directory-publication-schema-1-launch-approved", self.parent)
        git(self.repo, "checkout", "-q", "--detach", result["ledger_commit"])
        base_files = dict(files)
        base_files.pop(evidence.ATTESTATION_BUNDLE_NAME)
        base_files.pop("SHA256SUMS")
        base_files["SHA256SUMS"] = evidence.checksum_bytes(base_files)
        with mock.patch.object(evidence, "build_bundle", return_value=(digest, base_files)) as build_bundle, mock.patch.object(
            evidence, "verify_attestation",
        ) as verify_attestation:
            evidence.verify_completed_state(
                self.repo, self.repo, repository="owner/repository",
                main_commit=result["main_commit"], main_parent=self.parent,
                expected_run_id="123", source_digest="4" * 40,
                expected_publication_id="456",
                expected_publication_source_commit=self.parent,
                caller_event_name="push", caller_ref="refs/heads/main",
                caller_workflow_ref="owner/repository/.github/workflows/directory-publication.yml@refs/heads/main",
                approval_tag="refs/tags/directory-publication-schema-1-launch-approved",
                observer_public_key="trusted-key", observer_key_id="observer-v1",
            )
            with self.assertRaisesRegex(evidence.EvidenceError, "exact workflow run"):
                evidence.verify_completed_state(
                    self.repo, self.repo, repository="owner/repository",
                    main_commit=result["main_commit"], main_parent=self.parent,
                    expected_run_id="999", source_digest="4" * 40,
                    expected_publication_id="456",
                    expected_publication_source_commit=self.parent,
                    caller_event_name="push", caller_ref="refs/heads/main",
                    caller_workflow_ref="owner/repository/.github/workflows/directory-publication.yml@refs/heads/main",
                    approval_tag="refs/tags/directory-publication-schema-1-launch-approved",
                    observer_public_key="trusted-key", observer_key_id="observer-v1",
                )
            verify_attestation.assert_called_once()
            self.assertEqual(
                verify_attestation.call_args.kwargs["bundle_path"].name,
                evidence.ATTESTATION_BUNDLE_NAME,
            )
            self.assertFalse(build_bundle.call_args.kwargs["enforce_observer_freshness"])

    def test_existing_immutable_digest_root_is_rejected(self) -> None:
        digest, files = self.files()
        first = evidence.materialize_commits(
            self.repo, self.repo, self.repo, files, repository="owner/repository",
            main_parent=self.parent, ledger_parent=self.parent,
            approval_target=self.parent, digest=digest,
        )
        git(self.repo, "checkout", "-q", "--detach", first["ledger_commit"])
        with self.assertRaisesRegex(evidence.EvidenceError, "already exists"):
            evidence.materialize_commits(
                self.repo, self.repo, self.repo, files, repository="owner/repository",
                main_parent=first["ledger_commit"], ledger_parent=first["ledger_commit"],
                approval_target=first["ledger_commit"], digest=digest,
            )

    def test_new_pass_replaces_current_pointer_but_preserves_history(self) -> None:
        _, files = self.files()
        index = json.loads(files["directory-evidence/index.json"])
        current = json.loads(files[index["records"][0]["path"]])
        current.update({"artifact": {}, "trust": {}})
        previous = dict(current)
        previous["id"] = "launch/demo/codex/runtime/previous"
        previous["observed_at"] = "2026-08-22T00:00:00Z"
        source = json.loads((self.repo / "registry" / "directory.json").read_text())
        source["evidence"] = [previous]
        source["distributions"][0]["release_policies"][0]["current_evidence"] = [previous["id"]]

        updated = evidence.update_directory(source, [current])

        self.assertEqual({item["id"] for item in updated["evidence"]}, {previous["id"], current["id"]})
        self.assertEqual(
            updated["distributions"][0]["release_policies"][0]["current_evidence"],
            [current["id"]],
        )


class ProtectedWorkflowChainTests(unittest.TestCase):
    def test_attested_workflow_source_can_differ_from_ledger_artifact_revision(self) -> None:
        workflow = "owner/repository/.github/workflows/launch-evidence-e2e.yml"
        source_digest = "a" * 40
        ledger_revision = "b" * 40
        config = {"trusted_evidence_workflows": [{
            "workflow": workflow, "protected_source_ref": "refs/heads/main",
            "source_digest_policy": "protected_workflow_source",
            "allow_self_hosted_runners": False,
        }], "trusted_external_evidence": []}
        with tempfile.TemporaryDirectory() as temporary:
            artifact_dir = Path(temporary) / "artifact"
            artifact_dir.mkdir()
            _, files = LaunchEvidenceBundleTests().build(artifact_dir)
            index_value = json.loads(files["directory-evidence/index.json"])
            indexed = index_value["records"][0]
            record = json.loads(files[indexed["path"]])
            root = "registry/evidence/sha256/" + index_value["launch_evidence_digest"].removeprefix("sha256:")
            pointer = evidence.pointer_for(
                record, repository="owner/repository", ledger_commit=ledger_revision,
                root=root, index=index_value,
                index_digest=evidence.sha256(files["directory-evidence/index.json"]),
                bundle_identity_digest=evidence.sha256(files["bundle-identity.json"]),
            )
            with mock.patch.object(evidence, "validate_with_schema"), \
             mock.patch.object(prepare.Path, "is_file", return_value=True), \
             mock.patch.object(prepare.subprocess, "run", return_value=subprocess.CompletedProcess([], 0)) as run:
                prepare.verify_evidence_trust(
                    pointer, config, Path(temporary), files[indexed["path"]],
                    manifest_body=files["bundle-identity.json"],
                    launch_body=files["launch-evidence.json"],
                    observer_body=files["signed-observer-bundle.json"],
                    index_body=files["directory-evidence/index.json"],
                )
        command = run.call_args.args[0]
        self.assertIn("--source-digest", command)
        self.assertIn(source_digest, command)
        self.assertNotEqual(source_digest, ledger_revision)


class RealDirectoryValidationTests(unittest.TestCase):
    def test_real_domain_validator_accepts_exact_release_and_rejects_unreviewed_target(self) -> None:
        source = json.loads((ROOT / "registry" / "directory.json").read_text())
        distribution = next(item for item in source["distributions"] if item["id"] == "upstash/context7")
        release = distribution["releases"][0]
        package = release["package_source"]
        record = {
            "schema_version": 1, "id": "launch/context7/codex/runtime/domain-validator",
            "product_id": "context7", "distribution_id": distribution["id"],
            "release_sequence": release["sequence"], "package_tree_digest": release["tree_digest"],
            "manifest_digest": release["manifest_digest"], "source_repository": package["repository"],
            "source_revision": package["revision"], "source_path": package["path"],
            "level": "runtime", "outcome": "passed", "client": "codex", "client_version": "1",
            "installer_version": "0.1.13", "adapter_version": "0.1.13", "os": "linux",
            "architecture": "amd64", "dependency_identity": "none", "observed_at": "2026-08-23T00:00:00Z",
            "artifact": {"repository": "owner/evidence", "revision": "a" * 40, "path": "context7.json", "digest": "sha256:" + "b" * 64},
        }
        updated = evidence.update_directory(source, [record])
        evidence.validate_directory(updated, verify_packages=False, repository_root=ROOT)
        rejected = json.loads(json.dumps(record))
        rejected["id"] = "launch/context7/chatgpt/runtime/unreviewed-target"
        rejected["client"] = "chatgpt"
        with self.assertRaisesRegex(evidence.RegistryError, "not a reviewed release target"):
            evidence.validate_directory(
                evidence.update_directory(source, [rejected]),
                verify_packages=False, repository_root=ROOT,
            )


if __name__ == "__main__":
    unittest.main()
