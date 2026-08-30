from __future__ import annotations

import copy
import json
import subprocess
import tempfile
import unittest
from pathlib import Path

from scripts.directory_staged_supersession import (
    SupersessionError,
    require_material_payload_change,
)


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "directory_staged_supersession.py"


def payload() -> dict:
    return {
        "products": [{"id": "context7"}],
        "distributions": [{"id": "context7-community"}],
        "evidence": [{"id": "context7-e2e"}],
        "revocations": [],
    }


class DirectoryStagedSupersessionTests(unittest.TestCase):
    def test_rejects_metadata_only_change(self) -> None:
        candidate = {
            **payload(), "candidate_schema_version": 1,
            "publication_id": "20", "source_commit": "a" * 40,
        }
        staged = {
            **payload(), "snapshot_schema_version": 1, "sequence": 19,
            "publication_id": "19", "source_commit": "b" * 40,
            "generated_at": "2026-08-30T00:00:00Z",
            "expires_at": "2026-09-06T00:00:00Z",
        }
        with self.assertRaisesRegex(SupersessionError, "does not change"):
            require_material_payload_change(candidate, staged)

    def test_accepts_each_material_payload_change(self) -> None:
        staged = payload()
        for field in ("products", "distributions", "evidence", "revocations"):
            with self.subTest(field=field):
                candidate = copy.deepcopy(staged)
                candidate[field].append({"id": "changed"})
                require_material_payload_change(candidate, staged)

    def test_cli_fails_closed_for_unchanged_or_missing_payload(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            candidate = root / "candidate.json"
            staged = root / "staged.json"
            candidate.write_text(json.dumps(payload()))
            staged.write_text(json.dumps(payload()))
            unchanged = subprocess.run(
                ["python3", str(SCRIPT), "--candidate", str(candidate),
                 "--staged-snapshot", str(staged)],
                text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            )
            self.assertNotEqual(unchanged.returncode, 0)
            self.assertIn("does not change", unchanged.stderr)

            candidate.write_text(json.dumps({"products": []}))
            missing = subprocess.run(
                ["python3", str(SCRIPT), "--candidate", str(candidate),
                 "--staged-snapshot", str(staged)],
                text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            )
            self.assertNotEqual(missing.returncode, 0)
            self.assertIn("is missing", missing.stderr)
