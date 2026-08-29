from __future__ import annotations

import argparse
import copy
import hashlib
import importlib.util
import json
import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("validate_review_journey", ROOT / "scripts" / "validate_review_journey.py")
assert SPEC and SPEC.loader
journey = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(journey)


class PromotionReadinessTests(unittest.TestCase):
    def test_standard_github_origin_forms_are_accepted(self) -> None:
        expected = "ChromeDevTools/chrome-devtools-mcp"
        for origin in (
            "https://github.com/ChromeDevTools/chrome-devtools-mcp.git",
            "git@github.com:ChromeDevTools/chrome-devtools-mcp.git",
            "ssh://git@github.com/ChromeDevTools/chrome-devtools-mcp.git",
        ):
            with self.subTest(origin=origin):
                self.assertEqual(journey.github_repository_from_origin(origin), expected)

    def git(self, repository: Path, *args: str, environment: dict[str, str] | None = None) -> str:
        env = {**os.environ, "PATH": "/usr/bin:/bin", **(environment or {})}
        completed = subprocess.run(["git", *args], cwd=repository, env=env, text=True, capture_output=True, check=True)
        return completed.stdout.strip()

    def commit(self, repository: Path, message: str, timestamp: str) -> str:
        self.git(repository, "add", ".")
        identity = {
            "GIT_AUTHOR_NAME": "Fixture", "GIT_AUTHOR_EMAIL": "fixture@example.invalid", "GIT_AUTHOR_DATE": timestamp,
            "GIT_COMMITTER_NAME": "Fixture", "GIT_COMMITTER_EMAIL": "fixture@example.invalid", "GIT_COMMITTER_DATE": timestamp,
        }
        self.git(repository, "commit", "-qm", message, environment=identity)
        return self.git(repository, "rev-parse", "HEAD")

    def fixture(self) -> tuple[tempfile.TemporaryDirectory[str], Path, argparse.Namespace, dict]:
        temporary = tempfile.TemporaryDirectory()
        root = Path(temporary.name)
        repository = root / "official"
        package = repository / "packages" / "chrome-devtools"
        package.parent.mkdir(parents=True)
        shutil.copytree(ROOT / "plugins/cloudflare-docs", package)
        manifest = json.loads((package / "plugin.json").read_text())
        manifest["name"] = "chrome-devtools"
        manifest["repository"] = "https://github.com/Example/Official"
        manifest.pop("version")
        (package / "plugin.json").write_text(json.dumps(manifest, indent=2) + "\n")
        self.git(repository, "init", "-q", "-b", "main")
        reviewed = self.commit(repository, "reviewed PR head", "2026-01-01T00:00:00Z")
        facts = journey.package_facts(package)
        (repository / "MERGE-NOTE.md").write_text("squash metadata outside package\n")
        candidate = self.commit(repository, "official merged candidate", "2026-01-02T00:00:00Z")
        self.git(repository, "remote", "add", "origin", "https://github.com/Example/Official.git")
        self.git(repository, "update-ref", "refs/remotes/origin/main", candidate)
        self.git(repository, "update-ref", "refs/remotes/origin/pull/7/head", reviewed)
        artifact = {"repository": "example/evidence", "revision": candidate, "path": "evidence/codex.json", "digest": "sha256:" + "1" * 64}
        evidence = {
            "schema_version": 1, "id": "official-materialization-codex", "product_id": "chrome-devtools",
            "distribution_id": "example/chrome-devtools", "release_sequence": 1,
            "package_tree_digest": facts["tree_digest"], "manifest_digest": facts["manifest_digest"],
            "source_repository": "Example/Official", "source_revision": candidate, "source_path": "packages/chrome-devtools",
            "level": "materialization", "outcome": "passed", "client": "codex", "client_version": "1.0",
            "installer_version": "0.1.18", "os": "linux", "architecture": "amd64", "observed_at": "2026-01-02T00:00:00Z",
            "artifact": artifact,
        }
        record = {
            "schema_version": 3, "repository": "Example/Official", "path": "packages/chrome-devtools",
            "reviewed_revision": reviewed, "reviewed_tree_digest": facts["tree_digest"], "reviewed_manifest_digest": facts["manifest_digest"],
            "product_id": "chrome-devtools", "manifest_name": "chrome-devtools", "distribution_id": "example/chrome-devtools",
            "release_sequence": 1,
            "policy": {"status": "active", "minimum_installer_version": "0.1.18", "targets": [
                {"client": "codex", "scopes": ["user"], "delivery": "managed", "authentication": "not_required"}
            ]}, "evidence": [evidence],
        }
        review_path = root / "review.json"
        review_path.write_text(json.dumps(record))
        metadata_path = root / "pr-metadata.json"
        metadata_path.write_text(json.dumps({
            "schema_version": 1, "repository_id": "Example/Official", "upstream_pr_number": 7,
            "url": "https://github.com/Example/Official/pull/7", "state": "MERGED", "is_draft": False,
            "head_ref_oid": reviewed, "merge_commit_oid": candidate, "base_ref_name": "main",
            "default_branch": "main", "merged_at": "2026-01-02T00:00:00Z",
        }))
        args = argparse.Namespace(
            repository=repository, pr_metadata=metadata_path,
            path="packages/chrome-devtools", review_record=review_path, candidate_output=root / "candidate.json",
        )
        return temporary, repository, args, record

    def write_record(self, args: argparse.Namespace, record: dict) -> None:
        args.review_record.write_text(json.dumps(record))

    def replace_reviewed_package(self, repository: Path, args: argparse.Namespace, record: dict, mutate) -> None:
        package = repository / args.path
        mutate(package)
        reviewed = self.commit(repository, "updated reviewed PR head", "2026-01-03T00:00:00Z")
        facts = journey.package_facts(package)
        (repository / "MERGE-NOTE-2.md").write_text("merge metadata outside package\n")
        candidate = self.commit(repository, "updated official merge", "2026-01-04T00:00:00Z")
        metadata = json.loads(args.pr_metadata.read_text())
        metadata.update({"head_ref_oid": reviewed, "merge_commit_oid": candidate, "merged_at": "2026-01-04T00:00:00Z"})
        args.pr_metadata.write_text(json.dumps(metadata))
        self.git(repository, "update-ref", "refs/remotes/origin/pull/7/head", reviewed)
        self.git(repository, "update-ref", "refs/remotes/origin/main", candidate)
        record.update({"reviewed_revision": reviewed, "reviewed_tree_digest": facts["tree_digest"], "reviewed_manifest_digest": facts["manifest_digest"]})
        record["evidence"][0].update({
            "source_revision": candidate, "package_tree_digest": facts["tree_digest"],
            "manifest_digest": facts["manifest_digest"],
        })
        self.write_record(args, record)

    def test_exact_candidate_is_accepted_and_deterministic(self) -> None:
        temporary, _repository, args, _record = self.fixture()
        self.addCleanup(temporary.cleanup)
        first = journey.promotion(args)
        first_body = args.candidate_output.read_bytes()
        args.candidate_output = args.candidate_output.with_name("candidate-2.json")
        second = journey.promotion(args)
        self.assertEqual(first_body, args.candidate_output.read_bytes())
        self.assertEqual(first["candidate_digest"], second["candidate_digest"])
        self.assertEqual(first["candidate"]["source"]["byte_classification"], "exact")
        self.assertEqual(first["candidate"]["evidence"][0]["id"], "official-materialization-codex")
        self.assertEqual(first["candidate"]["release"]["package_version"], "")
        package_gate = next(item for item in first["gates"] if item["name"] == "package")
        self.assertEqual(package_gate["artifact"]["validator"], "build_registry.validate_release_package")
        self.assertTrue(package_gate["artifact"]["require_closed_runtime"])
        self.assertTrue(package_gate["artifact"]["runtime_policy_enforced"])
        self.assertEqual(package_gate["artifact"]["minimum_installer_version"], "0.1.18")
        self.assertEqual(package_gate["artifact"]["enforced_capabilities"], ["mcp"])

    def test_unversioned_package_requires_directory_description_and_keywords(self) -> None:
        for field, value, message in (
            ("description", "", "description required"),
            ("keywords", None, "keywords must be strings"),
        ):
            with self.subTest(field=field):
                temporary, repository, args, _record = self.fixture()
                with temporary:
                    manifest_path = repository / args.path / "plugin.json"
                    manifest = json.loads(manifest_path.read_text())
                    if value is None:
                        manifest.pop(field)
                    else:
                        manifest[field] = value
                    manifest_path.write_text(json.dumps(manifest) + "\n")
                    with self.assertRaisesRegex(journey.RegistryError, message):
                        journey.validated_package_facts(repository / args.path)

    def test_materialization_preflights_blob_size_before_writing(self) -> None:
        temporary, repository, args, _record = self.fixture()
        with temporary, tempfile.TemporaryDirectory() as output:
            with mock.patch.object(journey, "PORTABLE_MAX_FILE_BYTES", 1):
                with self.assertRaisesRegex(journey.JourneyError, "file exceeds 1 bytes"):
                    journey.materialize(
                        repository,
                        json.loads(args.pr_metadata.read_text())["head_ref_oid"],
                        args.path,
                        Path(output),
                    )
            self.assertEqual(list(Path(output).iterdir()), [])

    def test_mismatched_and_unsafe_origins_are_rejected(self) -> None:
        for origin in ("https://github.com/wrong/repository.git", "https://token@github.com/example/official.git", "https://example.com/example/official.git"):
            with self.subTest(origin=origin):
                temporary, repository, args, _record = self.fixture()
                with temporary:
                    self.git(repository, "remote", "set-url", "origin", origin)
                    with self.assertRaises(journey.JourneyError):
                        journey.promotion(args)

    def test_ambiguous_fetch_and_credential_bearing_push_origins_are_rejected(self) -> None:
        temporary, repository, args, _record = self.fixture()
        with temporary:
            self.git(repository, "config", "--add", "remote.origin.url", "git@github.com:Example/Official.git")
            with self.assertRaisesRegex(journey.JourneyError, "exactly one fetch URL"):
                journey.promotion(args)
        temporary, repository, args, _record = self.fixture()
        with temporary:
            self.git(repository, "config", "remote.origin.pushurl", "https://token@github.com/Example/Official.git")
            with self.assertRaisesRegex(journey.JourneyError, "credential-bearing"):
                journey.promotion(args)

    def test_candidate_must_be_in_official_default_history(self) -> None:
        temporary, repository, args, _record = self.fixture()
        self.addCleanup(temporary.cleanup)
        metadata = json.loads(args.pr_metadata.read_text())
        self.git(repository, "update-ref", "refs/remotes/origin/main", metadata["head_ref_oid"])
        with self.assertRaisesRegex(journey.JourneyError, "not reachable"):
            journey.promotion(args)

    def test_pr_metadata_is_strict_and_fetched_head_must_match(self) -> None:
        mutations = {
            "draft": lambda value: value.__setitem__("is_draft", True),
            "open": lambda value: value.__setitem__("state", "OPEN"),
            "invalid historical base": lambda value: value.__setitem__("base_ref_name", "bad branch"),
            "short head": lambda value: value.__setitem__("head_ref_oid", "abc"),
            "no merge time": lambda value: value.__setitem__("merged_at", None),
            "nonpositive number": lambda value: value.__setitem__("upstream_pr_number", 0),
        }
        for label, mutate in mutations.items():
            with self.subTest(label=label):
                temporary, _repository, args, _record = self.fixture()
                with temporary:
                    metadata = json.loads(args.pr_metadata.read_text())
                    mutate(metadata)
                    args.pr_metadata.write_text(json.dumps(metadata))
                    with self.assertRaises(journey.JourneyError):
                        journey.promotion(args)
        temporary, repository, args, _record = self.fixture()
        with temporary:
            metadata = json.loads(args.pr_metadata.read_text())
            self.git(repository, "update-ref", "refs/remotes/origin/pull/7/head", metadata["merge_commit_oid"])
            with self.assertRaisesRegex(journey.JourneyError, "fetched PR ref differs"):
                journey.promotion(args)

    def test_historical_pr_base_survives_default_branch_rename(self) -> None:
        temporary, repository, args, _record = self.fixture()
        self.addCleanup(temporary.cleanup)
        metadata = json.loads(args.pr_metadata.read_text())
        metadata["default_branch"] = "trunk"
        args.pr_metadata.write_text(json.dumps(metadata))
        self.git(repository, "update-ref", "refs/remotes/origin/trunk", metadata["merge_commit_oid"])
        result = journey.promotion(args)
        self.assertEqual(result["candidate"]["source"]["official_default_ref"], "refs/remotes/origin/trunk")
        self.assertEqual(metadata["base_ref_name"], "main")

    def test_changed_moved_and_missing_bytes_are_classified(self) -> None:
        for classification in ("changed", "moved", "missing"):
            with self.subTest(classification=classification):
                temporary, repository, args, record = self.fixture()
                with temporary:
                    metadata = json.loads(args.pr_metadata.read_text())
                    self.git(repository, "checkout", "-q", metadata["head_ref_oid"])
                    package = repository / args.path
                    if classification == "changed":
                        manifest = json.loads((package / "plugin.json").read_text())
                        manifest["description"] = "different merged bytes"
                        (package / "plugin.json").write_text(json.dumps(manifest) + "\n")
                    elif classification == "moved":
                        self.git(repository, "mv", args.path, "packages/moved-fixture")
                    else:
                        self.git(repository, "rm", "-qr", args.path)
                    candidate = self.commit(repository, classification, "2026-01-03T00:00:00Z")
                    metadata["merge_commit_oid"] = candidate
                    args.pr_metadata.write_text(json.dumps(metadata))
                    record["evidence"][0]["source_revision"] = candidate
                    self.write_record(args, record)
                    self.git(repository, "update-ref", "refs/remotes/origin/main", candidate)
                    with self.assertRaises(journey.JourneyError) as raised:
                        journey.promotion(args)
                    self.assertEqual(raised.exception.diagnostics["byte_classification"], classification)

    def test_manifest_name_mismatch_is_rejected(self) -> None:
        temporary, _repository, args, record = self.fixture()
        self.addCleanup(temporary.cleanup)
        record["manifest_name"] = "directory-name-does-not-match"
        self.write_record(args, record)
        with self.assertRaisesRegex(journey.JourneyError, "existing Directory product"):
            journey.promotion(args)

    def test_partial_wrong_target_and_wrong_sha_evidence_are_rejected(self) -> None:
        mutations = {
            "partial": lambda record: record["policy"]["targets"].append({"client": "cursor", "scopes": ["user"], "delivery": "managed", "authentication": "not_required"}),
            "wrong target": lambda record: record["evidence"][0].__setitem__("client", "cursor"),
            "wrong SHA": lambda record: record["evidence"][0].__setitem__("source_revision", "0" * 40),
        }
        for label, mutate in mutations.items():
            with self.subTest(label=label):
                temporary, _repository, args, original = self.fixture()
                with temporary:
                    record = copy.deepcopy(original)
                    mutate(record)
                    self.write_record(args, record)
                    with self.assertRaises(journey.JourneyError):
                        journey.promotion(args)

    def test_unclosed_live_npx_is_rejected_by_real_release_validator(self) -> None:
        temporary, repository, args, record = self.fixture()
        with temporary:
            def live_npx(package: Path) -> None:
                (package / "mcp.json").write_text(json.dumps({
                    "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
                    "mcpServers": {"fixture": {"type": "stdio", "command": "npx", "args": ["-y", "fixture@1.0.0"]}},
                }) + "\n")
            self.replace_reviewed_package(repository, args, record, live_npx)
            with self.assertRaisesRegex(journey.RegistryError, "live npx without"):
                journey.promotion(args)

    def test_locked_runtime_rejects_incompatible_installer_policy(self) -> None:
        temporary, repository, args, record = self.fixture()
        with temporary:
            record["policy"]["minimum_installer_version"] = "0.1.12"
            record["evidence"][0]["installer_version"] = "0.1.12"

            def locked_runtime(package: Path) -> None:
                shutil.rmtree(package)
                shutil.copytree(ROOT / "plugins" / "context7", package)
                manifest = json.loads((package / "plugin.json").read_text())
                manifest["name"] = "chrome-devtools"
                manifest["repository"] = "https://github.com/Example/Official"
                (package / "plugin.json").write_text(json.dumps(manifest, indent=2) + "\n")

            self.replace_reviewed_package(repository, args, record, locked_runtime)
            with self.assertRaisesRegex(
                journey.RegistryError,
                "locked npm runtime requires minimum installer version 0.1.13 or newer",
            ):
                journey.promotion(args)

    def test_evidence_repository_binding_is_exact_and_case_sensitive(self) -> None:
        temporary, _repository, args, record = self.fixture()
        self.addCleanup(temporary.cleanup)
        record["evidence"][0]["source_repository"] = "example/official"
        self.write_record(args, record)
        with self.assertRaisesRegex(journey.JourneyError, "exact official release tuple"):
            journey.promotion(args)

    def test_evidence_artifact_uses_directory_evidence_path_grammar(self) -> None:
        temporary, _repository, args, record = self.fixture()
        self.addCleanup(temporary.cleanup)
        record["evidence"][0]["artifact"]["path"] = "evidence outputs/codex.json"
        self.write_record(args, record)
        result = journey.promotion(args)
        self.assertEqual(
            result["candidate"]["evidence"][0]["artifact"]["path"],
            "evidence outputs/codex.json",
        )

    def test_missing_product_minimum_capability_is_rejected(self) -> None:
        temporary, repository, args, record = self.fixture()
        with temporary:
            def skills_only(package: Path) -> None:
                (package / "mcp.json").unlink()
                shutil.copytree(ROOT / "tests/fixtures/plugins/fixture-bridge/skills", package / "skills")
            self.replace_reviewed_package(repository, args, record, skills_only)
            with self.assertRaisesRegex(journey.JourneyError, "minimum capabilities"):
                journey.promotion(args)

    def test_registry_path_grammar_is_shared_and_rejects_ambiguous_paths(self) -> None:
        schema = journey.read_object(ROOT / "schemas/promotion-candidate.schema.json")
        distribution_schema = journey.read_object(ROOT / "schemas/directory-distribution.schema.json")
        evidence_schema = journey.read_object(ROOT / "schemas/directory-evidence.schema.json")
        self.assertEqual(schema["$defs"]["path"]["pattern"], distribution_schema["$defs"]["packageSource"]["properties"]["path"]["pattern"])
        self.assertEqual(schema["$defs"]["evidencePath"]["pattern"], evidence_schema["$defs"]["artifact"]["properties"]["path"]["pattern"])
        schema_pattern = re.compile(schema["$defs"]["path"]["pattern"])
        for path in ("with space", "unicodé", "drive:c", "../escape", "a/../b", "", "a//b", "a/"):
            with self.subTest(path=path):
                self.assertIsNone(schema_pattern.fullmatch(path))
                with self.assertRaises(journey.JourneyError):
                    journey.safe_git_path(path)

    def test_release_sequence_starts_at_one_increments_and_rejects_collisions(self) -> None:
        journey.validate_proposed_release_sequence({"distributions": []}, "owner/demo", "demo", 1)
        directory = {"distributions": [{
            "id": "owner/demo", "product_id": "demo", "kind": "upstream",
            "releases": [{"sequence": 1}, {"sequence": 3}],
        }]}
        journey.validate_proposed_release_sequence(directory, "owner/demo", "demo", 4)
        with self.assertRaisesRegex(journey.JourneyError, "must be 4"):
            journey.validate_proposed_release_sequence(directory, "owner/demo", "demo", 3)
        boundary = {"distributions": [{
            "id": "owner/demo", "product_id": "demo", "kind": "upstream",
            "releases": [{"sequence": 9_007_199_254_740_990}],
        }]}
        journey.validate_proposed_release_sequence(
            boundary, "owner/demo", "demo", 9_007_199_254_740_991,
        )
        for unsafe in (9_007_199_254_740_992, 9_007_199_254_740_993):
            with self.subTest(unsafe=unsafe), self.assertRaisesRegex(journey.JourneyError, "safe positive integer"):
                journey.validate_proposed_release_sequence(boundary, "owner/demo", "demo", unsafe)
        exhausted = copy.deepcopy(boundary)
        exhausted["distributions"][0]["releases"][0]["sequence"] = 9_007_199_254_740_991
        with self.assertRaisesRegex(journey.JourneyError, "exhausted"):
            journey.validate_proposed_release_sequence(
                exhausted, "owner/demo", "demo", 9_007_199_254_740_991,
            )
        for field, value in (("product_id", "other"), ("kind", "community_bridge")):
            collision = copy.deepcopy(directory)
            collision["distributions"][0][field] = value
            with self.assertRaisesRegex(journey.JourneyError, "collides"):
                journey.validate_proposed_release_sequence(collision, "owner/demo", "demo", 4)

        schema = journey.read_object(ROOT / "schemas/promotion-candidate.schema.json")
        self.assertEqual(schema["properties"]["release"]["properties"]["sequence"]["maximum"],
                         9_007_199_254_740_991)


if __name__ == "__main__":
    unittest.main()
