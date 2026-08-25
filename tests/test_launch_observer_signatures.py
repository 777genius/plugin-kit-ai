#!/usr/bin/env python3

import base64
import json
import sys
import unittest
from datetime import datetime, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
import launch_observer_signatures as signatures  # noqa: E402


class EvidenceRedactionTests(unittest.TestCase):
    def test_exact_argv_bypasses_are_rejected_before_upload(self) -> None:
        unsafe_argv = (
            ["agentplugins", "--user-data-dir=/Users/alice/Profile"],
            ["agentplugins", r"--user-data-dir=C:\Users\alice\Profile"],
            ["agentplugins", "--api-key=secret"],
            ["agentplugins", "--api_key", "secret"],
            ["agentplugins", "/password:secret"],
            ["agentplugins", "--auth", "Bearer secret"],
            ["agentplugins", "TOKEN=secret"],
            ["agentplugins", "--env", "PASSWORD=secret"],
            ["agentplugins", "--env=API_KEY=secret"],
            ["client", "--signature=secret"],
            ["client", "--sig", "secret"],
            ["client", "--aws-secret-access-key=secret"],
            ["client", "--x-amz-signature", "secret"],
        )
        for argv in unsafe_argv:
            with self.subTest(argv=argv), self.assertRaisesRegex(
                ValueError, "absolute local path or credential material"
            ):
                signatures.validate_evidence_redaction({"trace": {"argv": argv}})

    def test_embedded_path_url_and_oauth_query_bypasses_are_rejected(self) -> None:
        unsafe = (
            "file:///Users/alice/Profile/config.json",
            "path:/Users/alice/Profile",
            "callback=https://example.test/cb?code=secret",
            "callback=https://example.test/cb?state=secret",
            "callback=https://example.test/cb?token=secret",
            "callback=https://example.test/cb?credential=secret",
            "prefix=https://user:pass@example.test/private",
        )
        for value in unsafe:
            with self.subTest(value=value), self.assertRaisesRegex(
                ValueError, "absolute local path or credential material",
            ):
                signatures.validate_evidence_redaction({"detail": value})

    def test_cloud_credentials_fragments_and_workspace_paths_are_rejected(self) -> None:
        unsafe = (
            "https://example.test/cb?api_key=secret",
            "https://example.test/cb?access_token=secret",
            "https://example.test/cb#access_token=secret",
            "https://example.test/#route?code=secret",
            "https://example.test/cb?X-Amz-Signature=secret",
            "workspace:/Users/alice/private-project",
        )
        for value in unsafe:
            with self.subTest(value=value), self.assertRaisesRegex(
                ValueError, "absolute local path or credential material",
            ):
                signatures.validate_evidence_redaction({"detail": value})

    def test_recursive_sanitizer_preserves_operation_and_only_exports_digests(self) -> None:
        raw = {
            "trace": {
                "argv": [
                    "agentplugins", "add", "--user-data-dir=/Users/alice/Profile",
                    "--api-key=secret", "--env", "PASSWORD=hunter2",
                ]
            }
        }
        sanitized = signatures.sanitize_evidence(raw)
        argv = sanitized["trace"]["argv"]
        self.assertEqual(argv[:2], ["agentplugins", "add"])
        self.assertTrue(argv[2].startswith("--user-data-dir=<redacted:absolute-path:sha256:"))
        self.assertTrue(argv[3].startswith("--api-key=<redacted:credential:sha256:"))
        self.assertEqual(argv[4], "--env")
        self.assertRegex(argv[5], r"^PASSWORD=<redacted:credential:sha256:[a-f0-9]{64}>$")
        serialized = json.dumps(sanitized)
        for private_value in ("/Users/alice/Profile", "secret", "hunter2"):
            self.assertNotIn(private_value, serialized)
        signatures.validate_evidence_redaction(sanitized)

    def test_safe_hashes_and_credential_free_urls_are_not_false_positives(self) -> None:
        digest = "sha256:" + "a" * 64
        safe = {
            "trace": {"argv": [
                "agentplugins", "--digest=" + digest, "--endpoint=https://api.example.test/v1",
                "npm", "audit", "signatures", "--verify-signature",
            ]},
            "token_digest": digest,
            "authorization_url": "https://login.example.test/oauth/authorize?client_id=public",
        }
        self.assertEqual(signatures.sanitize_evidence(safe), safe)
        signatures.validate_evidence_redaction(safe)

    def test_observer_verification_uses_recursive_redaction_validator(self) -> None:
        artifacts = {
            "runtime-attestations.json": {"trace": {"argv": ["client", "--token=secret"]}},
            "notion-oauth-attestations.json": {},
            "chatgpt-cloudflare-attestation.json": {},
            "consent.json": {},
        }
        now = datetime.now(timezone.utc).replace(microsecond=0)
        bundle = {
            "schema_version": 1,
            "challenge": "a" * 64,
            "signed_at": now.isoformat(),
            "key_id": "observer",
            "artifacts": artifacts,
            "signature": base64.b64encode(b"x" * 64).decode(),
        }
        with self.assertRaisesRegex(ValueError, "protected observer bundle contains"):
            signatures.verify_observer_bundle(
                bundle,
                challenge="a" * 64,
                public_key_base64=base64.b64encode(b"x" * 32).decode(),
                expected_key_id="observer",
                now=now,
            )


if __name__ == "__main__":
    unittest.main()
