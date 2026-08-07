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
        screenshot = root / "assets" / "screenshots" / "proof.png"
        screenshot.parent.mkdir(parents=True)
        screenshot.write_bytes(b"\x89PNG\r\n\x1a\n")

        evidence = root / "tests" / "e2e" / "results" / "chatgpt-web-2026-08-07.json"
        evidence.parent.mkdir(parents=True)
        evidence.write_text(
            json.dumps(
                {
                    "client": "ChatGPT web",
                    "observed_redirect_origin": "https://app.notion.com",
                    "evidence": ["assets/screenshots/proof.png"],
                    "privacy": {
                        "sanitized": True,
                        "excluded": sorted(validator.REQUIRED_PRIVACY_EXCLUSIONS),
                    },
                    "cleanup": {
                        "status": "partial",
                        "connection_removed": True,
                        "developer_mode_restored": True,
                        "provider_grant_revoked": False,
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
            data["evidence"] = ["assets/screenshots/missing.png"]
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

    def test_completed_cleanup_requires_provider_revocation(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = self.make_fixture(root)
            data = json.loads(evidence.read_text())
            data["cleanup"]["status"] = "completed"
            evidence.write_text(json.dumps(data))
            with self.assertRaises(validator.ValidationError):
                validator.validate_evidence_file(evidence, root)


if __name__ == "__main__":
    unittest.main()
