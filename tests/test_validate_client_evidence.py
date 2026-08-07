from __future__ import annotations

import importlib.util
import json
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = (
    Path(__file__).resolve().parents[1] / "scripts" / "validate_client_evidence.py"
)
SPEC = importlib.util.spec_from_file_location("validate_client_evidence", MODULE_PATH)
assert SPEC and SPEC.loader
validator = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(validator)


class ClientEvidenceValidatorTests(unittest.TestCase):
    def make_fixture(self, root: Path) -> Path:
        artifact = root / "assets" / "evidence" / "proof.txt"
        artifact.parent.mkdir(parents=True)
        artifact.write_text("sanitized proof", encoding="utf-8")

        evidence = root / "tests" / "e2e" / "results" / "chatgpt-web-2026-08-07.json"
        evidence.parent.mkdir(parents=True)
        evidence.write_text(
            json.dumps(
                {
                    "client": "ChatGPT web",
                    "observed_redirect_origin": "https://app.notion.com",
                    "evidence": ["assets/evidence/proof.txt"],
                    "privacy": {
                        "sanitized": True,
                        "excluded": sorted(validator.REQUIRED_PRIVACY_EXCLUSIONS),
                    },
                    "cleanup": {
                        "status": "partial",
                        "connection_removed": True,
                        "developer_mode_restored": True,
                        "provider_grant_revoked": False,
                        "provider_revocation_observation": "Provider grant remains active",
                        "last_checked_at_utc": "2026-08-07T15:04:00Z",
                    },
                }
            ),
            encoding="utf-8",
        )
        return evidence

    def test_valid_partial_cleanup(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_fixture(root)
            validator.validate_evidence_file(evidence, root)

    def test_missing_evidence_path_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_fixture(root)
            data = json.loads(evidence.read_text())
            data["evidence"] = ["assets/evidence/missing.txt"]
            evidence.write_text(json.dumps(data))
            with self.assertRaises(validator.ValidationError):
                validator.validate_evidence_file(evidence, root)

    def test_redirect_query_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_fixture(root)
            data = json.loads(evidence.read_text())
            data["observed_redirect_origin"] = "https://app.notion.com?code=secret"
            evidence.write_text(json.dumps(data))
            with self.assertRaises(validator.ValidationError):
                validator.validate_evidence_file(evidence, root)

    def test_out_of_range_redirect_port_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_fixture(root)
            data = json.loads(evidence.read_text())
            data["observed_redirect_origin"] = "https://app.notion.com:99999"
            evidence.write_text(json.dumps(data))
            with self.assertRaises(validator.ValidationError):
                validator.validate_evidence_file(evidence, root)

    def test_malformed_redirect_authority_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_fixture(root)
            data = json.loads(evidence.read_text())
            data["observed_redirect_origin"] = "https://["
            evidence.write_text(json.dumps(data))
            with self.assertRaises(validator.ValidationError):
                validator.validate_evidence_file(evidence, root)

    def test_completed_cleanup_requires_provider_revocation(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_fixture(root)
            data = json.loads(evidence.read_text())
            data["cleanup"]["status"] = "completed"
            evidence.write_text(json.dumps(data))
            with self.assertRaises(validator.ValidationError):
                validator.validate_evidence_file(evidence, root)

    def test_partial_cleanup_requires_observation_and_timestamp(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_fixture(root)
            data = json.loads(evidence.read_text())
            data["cleanup"].pop("provider_revocation_observation", None)
            data["cleanup"].pop("last_checked_at_utc", None)
            evidence.write_text(json.dumps(data))
            with self.assertRaises(validator.ValidationError):
                validator.validate_evidence_file(evidence, root)

    def test_partial_cleanup_requires_incomplete_dimension(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_fixture(root)
            data = json.loads(evidence.read_text())
            data["cleanup"].update(
                {
                    "provider_grant_revoked": True,
                    "provider_revocation_observation": "All dimensions complete",
                    "last_checked_at_utc": "2026-08-07T15:04:00Z",
                }
            )
            evidence.write_text(json.dumps(data))
            with self.assertRaises(validator.ValidationError):
                validator.validate_evidence_file(evidence, root)

    def test_future_year_evidence_is_validated(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_fixture(root)
            future = evidence.with_name("chatgpt-web-2027-01-02.json")
            evidence.rename(future)
            data = json.loads(future.read_text())
            data["privacy"]["excluded"].remove("tokens")
            future.write_text(json.dumps(data))
            with self.assertRaises(validator.ValidationError):
                validator.validate_all(root)

    def test_invalid_calendar_date_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_fixture(root)
            evidence.rename(evidence.with_name("chatgpt-web-2027-13-40.json"))
            with self.assertRaises(validator.ValidationError):
                validator.validate_all(root)

    def test_undated_evidence_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_fixture(root)
            evidence.rename(evidence.with_name("chatgpt-web.json"))
            with self.assertRaises(validator.ValidationError):
                validator.validate_all(root)

    def test_nested_evidence_is_validated(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_fixture(root)
            nested = evidence.parent / "archive" / evidence.name
            nested.parent.mkdir()
            evidence.rename(nested)
            data = json.loads(nested.read_text())
            data["privacy"]["excluded"].remove("tokens")
            nested.write_text(json.dumps(data))
            with self.assertRaises(validator.ValidationError):
                validator.validate_all(root)

    def test_nested_latest_filename_is_not_exempt(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_fixture(root)
            nested = evidence.parent / "archive" / "latest.json"
            nested.parent.mkdir()
            evidence.rename(nested)
            with self.assertRaises(validator.ValidationError):
                validator.validate_all(root)

    def make_automated_codex_fixture(self, root: Path) -> Path:
        evidence = root / "tests" / "e2e" / "results" / "codex-ci-2026-08-07.json"
        evidence.parent.mkdir(parents=True)
        evidence.write_text(
            json.dumps(
                {
                    "client": "Codex CLI",
                    "evidence_type": "automated_public_install",
                    "real_user_project_used": False,
                    "source": {
                        "repository": validator.AUTOMATED_CODEX_REPOSITORY,
                        "ref": "v0.1.0",
                        "commit_sha": "a" * 40,
                    },
                    "workflow": {
                        "url": "https://github.com/777genius/universal-agent-plugins/actions/runs/123",
                        "commit_sha": "b" * 40,
                    },
                    "reproduction": {
                        "commands": [
                            "codex plugin marketplace add 777genius/universal-agent-plugins --ref v0.1.0 --json"
                        ]
                    },
                    "transcript": ["one", "two", "three"],
                    "privacy": {
                        "sanitized": True,
                        "excluded": sorted(validator.REQUIRED_PRIVACY_EXCLUSIONS),
                    },
                }
            ),
            encoding="utf-8",
        )
        return evidence

    def test_valid_automated_codex_provenance(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_automated_codex_fixture(root)
            validator.validate_evidence_file(evidence, root)

    def test_automated_codex_requires_pinned_reproduction(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_automated_codex_fixture(root)
            data = json.loads(evidence.read_text())
            data["reproduction"]["commands"] = ["codex plugin marketplace add owner/repo"]
            evidence.write_text(json.dumps(data))
            with self.assertRaises(validator.ValidationError):
                validator.validate_evidence_file(evidence, root)

    def test_automated_codex_rejects_temp_paths(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_automated_codex_fixture(root)
            data = json.loads(evidence.read_text())
            data["transcript"][0] = "installedPath=/tmp/secret/plugin"
            evidence.write_text(json.dumps(data))
            with self.assertRaises(validator.ValidationError):
                validator.validate_evidence_file(evidence, root)


if __name__ == "__main__":
    unittest.main()
