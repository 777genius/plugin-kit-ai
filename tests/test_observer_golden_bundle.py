from __future__ import annotations

import base64
import json
import sys
import tempfile
import unittest
from datetime import datetime, timezone
from pathlib import Path

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
sys.path.insert(0, str(ROOT / "scripts"))

import launch_observer_signatures as signatures
import materialize_launch_evidence as materialize
import run_launch_evidence_e2e as launch
from observer.schema_validation import validate_artifact_schemas
from observer.tests.test_observer_service import Fixture, artifacts


class GoldenObserverBundleTest(unittest.TestCase):
    def test_exact_12_3_1_bundle_crosses_all_consumers(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            fixture_root = root / "fixture"; fixture_root.mkdir()
            request = Fixture(fixture_root).request()
            exact = artifacts(request["challenge"]["value"])
            validate_artifact_schemas(
                exact, challenge=request["challenge"]["value"],
                scenario_contract_digest=request["scenario_contract_digest"], expected_bindings=request,
            )
            key = Ed25519PrivateKey.generate()
            now = datetime.now(timezone.utc).replace(microsecond=0)
            bundle = {
                "schema_version": 1, "challenge": request["challenge"]["value"],
                "signed_at": now.isoformat().replace("+00:00", "Z"), "key_id": "golden-observer",
                "artifacts": exact,
            }
            bundle["signature"] = base64.b64encode(key.sign(signatures.signed_payload(bundle))).decode()
            verified = signatures.verify_observer_bundle(
                bundle, challenge=request["challenge"]["value"],
                public_key_base64=base64.b64encode(key.public_key().public_bytes_raw()).decode(),
                expected_key_id="golden-observer", now=now,
            )
            harness = launch.LaunchHarness.__new__(launch.LaunchHarness)
            harness.external_pr_evidence = None
            paths = {}
            for name, value in verified.items():
                path = root / name; path.write_text(json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n"); paths[name] = path
            harness.challenge = request["challenge"]
            harness.github_run_id = request["github"]["run_id"]
            harness.github_run_attempt = request["github"]["run_attempt"]
            harness.consent = exact["consent.json"]
            harness.consent_digest = launch.sha256_file(paths["consent.json"])
            primary = harness._load_attestations(paths["runtime-attestations.json"], allow_external_pr=True)
            notion = harness._load_attestations(paths["notion-oauth-attestations.json"])
            chatgpt = harness._load_attestations(paths["chatgpt-cloudflare-attestation.json"])
            self.assertEqual((len(primary), len(notion), len(chatgpt)), (12, 3, 1))
            rows = []
            for record in [*primary.values(), *notion.values(), *chatgpt.values()]:
                rows.append({
                    "id": f"golden-{len(rows)}", "scenario": record["scenario_id"],
                    "plugin": record["plugin"], "client": record["client"], "level": "runtime",
                    "outcome": "passed", "reason": "golden contract", "tuple": record["tuple"],
                    "details": {"evidence_basis": "protected_external_observer",
                                "native_discovery_proof": record["client"] != "chatgpt",
                                "public_mcp_proof": record["client"] == "chatgpt"},
                })
            selected = materialize.selected_rows({
                "schema_version": 5,
                "evidence_class": "released_binary",
                "matrix": rows,
            })
            self.assertEqual(len(selected), 16)
            self.assertEqual(len([materialize.evidence_record(row) for row in selected]), 16)


if __name__ == "__main__":
    unittest.main()
