from __future__ import annotations

import argparse
import base64
import copy
import errno
import hashlib
import json
import os
import shutil
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

import build_chatgpt_observer_projection as generator  # noqa: E402
import directory_publication as publication  # noqa: E402
from observe_launch_scenario import package_identity  # noqa: E402

publication.OPENSSL = shutil.which("openssl") or publication.OPENSSL
PublicationError = publication.PublicationError


APP_ID = "plugin_asdk_app_" + "a" * 32
INSTALLATION_ID = "12345678-1234-4234-9234-123456789abc"
PHYSICAL_ARTIFACT_ID = generator.physical_artifact_id("demo", INSTALLATION_ID)
SNAPSHOT_DIGEST = ""


class ChatGPTProjectionGeneratorTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.root = Path(self.temporary.name).resolve()
        self.feed = self.root / "feed"
        self.feed.mkdir()
        (self.feed / "snapshots").mkdir()
        fixture = ROOT / "tests/fixtures/directory-publication"
        shutil.copy(fixture / "latest.json", self.feed / "latest.json")
        shutil.copy(fixture / "trusted-keys.json", self.root / "trusted-keys.json")
        self.snapshot = json.loads((fixture / "snapshot.json").read_text())
        distribution = next(item for item in self.snapshot["distributions"] if item["id"] == "example/demo")
        distribution["release_policies"] = [{
            "current_evidence": [],
            "minimum_installer_version": "0.1.6",
            "release_sequence": 2,
            "status": "active",
            "targets": [{
                "app_binding": {"app_key": "demo", "id": APP_ID, "mcp_server": "demo"},
                "authentication": "not_required",
                "client": "chatgpt",
                "delivery": "managed",
                "scopes": ["user"],
            }],
        }]
        self.snapshot["evidence"] = []
        self.package = self.root / "package"
        self.package.mkdir()
        portable_manifest = {
            "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
            "name": "demo", "version": "1.0.0", "description": "Demo package.",
            "author": {"name": "Example", "email": "dev@example.test", "url": "https://example.test"},
            "homepage": "https://example.test/demo", "repository": "https://example.test/repository",
            "license": "MIT", "keywords": ["demo"],
        }
        self.write_json(self.package / "plugin.json", portable_manifest)
        self.write_json(self.package / "mcp.json", {
            "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
            "mcpServers": {"demo": {"type": "streamable-http", "url": "https://docs.example.test/mcp"}},
        })
        (self.package / "README.md").write_bytes(b"# Demo\n")
        (self.package / "NOTICE").write_bytes(b"Demo notice\n")
        self.release().update(package_identity(self.package))
        self.write_signed_snapshot()
        self.projection = self.root / "projection"
        (self.projection / ".codex-plugin").mkdir(parents=True)
        projected_manifest = {
            key: portable_manifest[key]
            for key in ("name", "version", "description", "author", "homepage", "repository", "license", "keywords")
        }
        projected_manifest.update({"mcpServers": "./.mcp.json", "apps": "./.app.json"})
        (self.projection / ".codex-plugin/plugin.json").write_bytes(generator.released_manifest_json(projected_manifest))
        (self.projection / ".app.json").write_bytes(generator.released_compact_json({"apps": {"demo": {"id": APP_ID}}}))
        (self.projection / ".mcp.json").write_bytes(generator.released_json({
            "mcpServers": {"demo": {"type": "http", "url": "https://docs.example.test/mcp"}},
        }))
        (self.projection / "README.md").write_bytes(b"# Demo\n")
        (self.projection / "NOTICE").write_bytes(b"Demo notice\n")
        marketplace = {
            "name": "agentplugins-" + hashlib.sha256(PHYSICAL_ARTIFACT_ID.encode()).hexdigest()[:12],
            "plugins": [{
                "name": "demo", "source": {"source": "local", "path": "./"},
                "policy": {"installation": "AVAILABLE", "authentication": "ON_INSTALL"},
                "category": "Productivity",
            }],
        }
        (self.projection / ".agents/plugins").mkdir(parents=True)
        (self.projection / ".agents/plugins/marketplace.json").write_bytes(generator.released_json(marketplace))
        self.binary = self.root / "agentplugins"
        self.binary.write_bytes(b"#!/bin/sh\n[ \"$1\" = version ] && printf 'agentplugins 0.1.24\\n'\n")
        self.binary.chmod(0o755)
        self.fake_binary_digest = "sha256:" + hashlib.sha256(self.binary.read_bytes()).hexdigest()
        self.binary_digest_patcher = mock.patch.object(
            generator, "CURRENT_LINUX_AMD64_DIGEST", self.fake_binary_digest,
        )
        self.binary_digest_patcher.start()
        self.addCleanup(self.binary_digest_patcher.stop)
        self.add = self.root / "add.json"
        self.state = self.root / "state-v2.json"
        self.write_add()
        self.write_state()

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def test_exact_released_0_1_24_capture_has_six_expected_files_and_bytes(self) -> None:
        fixture_path = ROOT / "tests/e2e/fixtures/agentplugins-0.1.24-chatgpt-projection-golden.json"
        fixture = json.loads(fixture_path.read_text())
        self.assertEqual(fixture["schema_version"], 1)
        capture = fixture["capture"]
        self.assertEqual(capture["release_commit"], "c78c79e44efd5ad07083d63436d9170b107df6cb")
        self.assertEqual(capture["release_id"], 379284682)
        self.assertTrue(capture["github_attestation_verified"])
        self.assertEqual(capture["product_id"], "cloudflare-docs")
        self.assertEqual(capture["distribution_id"], "777genius/cloudflare-docs-bridge")
        self.assertEqual(capture["package_revision"], "224e4c065c69ff0b4e326e7796283524df9bfd2f")
        self.assertEqual(capture["directory_sequence"], 13)
        self.assertEqual(capture["physical_artifact_id"], "cloudflare-docs-0181c64e55ca")
        golden = {name: base64.b64decode(body, validate=True) for name, body in fixture["files_base64"].items()}
        self.assertEqual(set(golden), {
            ".codex-plugin/plugin.json", ".app.json", ".mcp.json",
            ".agents/plugins/marketplace.json", "README.md", "NOTICE",
        })
        expected_digests = {
            ".codex-plugin/plugin.json": "66ee56e7d4435672234b97f1b04897dcbcff7ac936c289ae4fb7f0d1d3ec5943",
            ".app.json": "266df3bc8a755d7bf97ad35aa264771fff662fdc79bc4e1aba4a2133beee509e",
            ".mcp.json": "39501ad8ae5e7b2cdfb0b96c3ea0d89ff2bbdb9c377e4384091885449fc1ee9a",
            ".agents/plugins/marketplace.json": "73bed9541c470a13636c1a64bae16adfa8abdc434d8e1d5c20f90e075a294f9e",
            "README.md": "f3ed74d8b982df67ba7a658d48069926b8c80cd58f363392f773de8ddf717e99",
            "NOTICE": "2550921ce9a9709ad66174c1cb69a3a12de4b084fea4b3a2867ca41208fcadf7",
        }
        self.assertEqual({name: hashlib.sha256(body).hexdigest() for name, body in golden.items()}, expected_digests)
        manifest = golden[".codex-plugin/plugin.json"]
        self.assertIn(b'"author": {\n    "name": "777genius",\n    "url":', manifest)
        self.assertNotIn(b'"email"', manifest)
        self.assertEqual(golden[".app.json"], generator.released_compact_json({
            "apps": {"cloudflare-docs": {"id": "plugin_asdk_app_6a78e90cf73481918ef10cdb87cd4bb4"}},
        }))

    @staticmethod
    def write_json(path: Path, value: object) -> None:
        path.write_bytes(generator.canonical_output(value))

    @staticmethod
    def write_projection_json(path: Path, value: object) -> None:
        if path.name == ".app.json":
            body = generator.released_compact_json(value)
        elif path.name == "plugin.json":
            body = generator.released_manifest_json(value)  # type: ignore[arg-type]
        else:
            body = generator.released_json(value)
        path.write_bytes(body)

    def write_signed_snapshot(self) -> None:
        body = publication.canonical_json(self.snapshot)
        digest = publication.sha256_digest(body)
        seeds = json.loads((ROOT / "tests/fixtures/directory-publication/test-private-seeds.json").read_text())
        private = publication.ed25519_private_key(seeds["test-current"])
        signature = publication.ed25519_sign(private, publication.signature_message(body))
        envelope = {
            "algorithm": "Ed25519",
            "envelope_schema_version": 1,
            "key_id": "test-current",
            "sequence": self.snapshot["sequence"],
            "signature": base64.b64encode(signature).decode("ascii"),
            "signature_domain": "UAP-DIRECTORY-SNAPSHOT-ED25519-V1",
            "snapshot_digest": digest,
            "snapshot_schema_version": 1,
        }
        stem = f"{self.snapshot['sequence']:020d}"
        (self.feed / "snapshots" / f"{stem}.json").write_bytes(body)
        (self.feed / "snapshots" / f"{stem}.envelope.json").write_bytes(publication.canonical_json(envelope))
        self.snapshot_digest = digest

    def release(self) -> dict[str, object]:
        distribution = next(item for item in self.snapshot["distributions"] if item["id"] == "example/demo")
        return next(item for item in distribution["releases"] if item["sequence"] == 2)

    def write_add(self, mutate=None) -> None:
        release = self.release()
        source = release["package_source"]
        identity = {
            "plugin": "demo",
            "version": release["package_version"],
            "source": f"{source['repository']}//{source['path']}",
            "revision": source["revision"],
            "tree_digest": release["tree_digest"],
            "manifest_digest": release["manifest_digest"],
            "dry_run": False,
        }
        value = {
            "schema_version": 1,
            "command": "add",
            "result": "success",
            "data": {
                **identity,
                "operation_id": "operation-demo",
                "batch": True,
                "status": "completed",
                "succeeded": 1,
                "failed": 0,
                "acquisition": {
                    "acquisition_id": "acquisition-demo",
                    "acquisition_count": 1,
                    "tree_digest": release["tree_digest"],
                    "manifest_digest": release["manifest_digest"],
                    "closure_digest": "sha256:" + "3" * 64,
                    "source_kind": "github",
                    "fetched": True,
                    "validated": True,
                },
                "target_outcomes": {
                    "chatgpt": {
                        "outcome": "passed",
                        "acquisition_id": "acquisition-demo",
                        "tree_digest": release["tree_digest"],
                        "manifest_digest": release["manifest_digest"],
                        "closure_digest": "sha256:" + "3" * 64,
                    },
                },
                "directory": {
                    "product_id": "demo",
                    "distribution_id": "example/demo",
                    "distribution_kind": "upstream",
                    "desired_release_sequence": 2,
                    "snapshot_schema": 1,
                    "snapshot_sequence": 15,
                    "snapshot_digest": self.snapshot_digest,
                },
                "targets": [{
                    "target": "chatgpt",
                    "status": "external_completed",
                    "next_action": "install in ChatGPT",
                    "output": {
                        **identity,
                        "operation_id": "operation-demo",
                        "next_action": "install in ChatGPT",
                        "result": {
                            "installation_id": INSTALLATION_ID,
                            "plan": {
                                "client_id": "chatgpt",
                                "scope": "user",
                                "status": "manual_activation_required",
                                "package_mode": "compatibility_projection",
                                "activation": "manual_activation_required",
                                "authentication": "not_required",
                                "policy": "allowed",
                                "verification": "package_validated",
                                "physical_artifact_id": PHYSICAL_ARTIFACT_ID,
                                "components": [
                                    {"kind": "app", "name": "demo", "support": "projected"},
                                    {"kind": "mcp_server", "name": "demo", "support": "projected"},
                                ],
                                "user_actions": ["install in ChatGPT"],
                                "warnings": [],
                            },
                            "activation": {
                                "activation": "manual_activation_required",
                                "authentication": "not_required",
                                "policy": "allowed",
                                "verification": "package_validated",
                                "user_actions": ["install in ChatGPT"],
                            },
                            "requires_confirmation": False,
                            "mutated": True,
                            "group_phase": "external_completed",
                        },
                    },
                }],
            },
        }
        if mutate:
            mutate(value)
        self.write_json(self.add, value)

    def write_state(self, mutate=None) -> None:
        fixture = json.loads((ROOT / "tests/fixtures/agentplugins-0.1.14/state-v2.json").read_text())
        release = self.release()
        source = release["package_source"]
        installation = fixture["installations"][0]
        installation["installation_id"] = INSTALLATION_ID
        installation["declared_name"] = "demo"
        installation["origin_mode"] = "directory"
        installation["directory"] = {
            "product_id": "demo", "distribution_id": "example/demo", "distribution_kind": "upstream",
            "desired_release_sequence": 2, "snapshot_schema": 1, "snapshot_sequence": 15,
            "snapshot_digest": self.snapshot_digest,
        }
        installation["source"].update({
            "repository": source["repository"], "package_subpath": source["path"],
            "resolved_revision": source["revision"], "tree_digest": release["tree_digest"],
        })
        installation["package"].update({
            "declared_name": "demo", "version": release["package_version"],
            "manifest_digest": release["manifest_digest"],
        })
        original = next(iter(installation["clients"].values()))
        original.update({
            "client_id": "chatgpt", "target_locator": str(self.projection),
            "physical_artifact_id": PHYSICAL_ARTIFACT_ID,
            "package_revision": {
                "version": release["package_version"], "resolved_revision": source["revision"],
                "tree_digest": release["tree_digest"], "manifest_digest": release["manifest_digest"],
                "distribution_id": "example/demo", "release_sequence": 2,
            },
            "affected_surfaces": ["chatgpt"],
        })
        managed_digest = generator.projection_artifact_digest(self.projection)
        original["native_objects"] = [{
            "object_id": "package:chatgpt:" + PHYSICAL_ARTIFACT_ID,
            "kind": "managed_package_directory",
            "logical_name": "demo",
            "path": str(self.projection),
            "managed_digest": managed_digest,
            "protection_class": "managed",
        }]
        receipt = original["receipts"][-1]
        receipt.update({
            "client_binding_id": original["client_binding_id"],
            "active_path": str(self.projection),
            "staging_path": str(self.projection.parent / ".agentplugins-staging-demo"),
            "backup_path": str(self.projection.parent / ".agentplugins-backup-demo"),
            "after_digest": managed_digest,
        })
        installation["clients"] = {original["client_binding_id"]: original}
        if mutate:
            mutate(fixture)
        self.write_json(self.state, fixture)

    def args(self, output: Path | None = None) -> argparse.Namespace:
        return argparse.Namespace(
            feed=self.feed,
            trusted_keys=self.root / "trusted-keys.json",
            now="2026-08-30T00:00:00Z",
            minimum_sequence=15,
            add_evidence=self.add,
            state=self.state,
            package_root=self.package,
            projection_root=self.projection,
            cli_binary=self.binary,
            installer_version="0.1.24",
            product_id="demo",
            distribution_id="example/demo",
            release_sequence=2,
            app_key="demo",
            app_id=APP_ID,
            observed_at="2026-08-30T00:00:00Z",
            output=output or self.root / "generated",
        )

    def generate(self, output: Path | None = None) -> tuple[dict[str, object], dict[str, object]]:
        args = self.args(output)
        return generator.generate(args)

    def test_builds_existing_canonical_contract_from_signed_and_cli_evidence(self) -> None:
        managed_digest = generator.projection_artifact_digest(self.projection)
        app, receipt = self.generate()
        self.assertEqual(app, {"apps": {"demo": {"id": APP_ID}}})
        self.assertEqual(receipt["application_id"], APP_ID)
        self.assertEqual(receipt["product_id"], "demo")
        self.assertEqual(
            json.loads((self.projection / ".codex-plugin/plugin.json").read_text()),
            {
                "name": "demo", "version": "1.0.0", "description": "Demo package.",
                "author": {"name": "Example", "email": "dev@example.test", "url": "https://example.test"},
                "homepage": "https://example.test/demo", "repository": "https://example.test/repository",
                "license": "MIT", "keywords": ["demo"],
                "mcpServers": "./.mcp.json", "apps": "./.app.json",
            },
        )
        self.assertEqual(
            json.loads((self.projection / ".agents/plugins/marketplace.json").read_text()),
            {
                "name": "agentplugins-" + hashlib.sha256(PHYSICAL_ARTIFACT_ID.encode()).hexdigest()[:12],
                "plugins": [{
                    "name": "demo", "source": {"source": "local", "path": "./"},
                    "policy": {"installation": "AVAILABLE", "authentication": "ON_INSTALL"},
                    "category": "Productivity",
                }],
            },
        )
        self.assertFalse((self.projection / "assets").exists())
        self.assertEqual(receipt["projection"], {
            "app_json_digest": "sha256:" + hashlib.sha256((self.projection / ".app.json").read_bytes()).hexdigest(),
            "codex_manifest_digest": "sha256:" + hashlib.sha256((self.projection / ".codex-plugin/plugin.json").read_bytes()).hexdigest(),
            "managed_digest": managed_digest,
            "mcp_json_digest": "sha256:" + hashlib.sha256((self.projection / ".mcp.json").read_bytes()).hexdigest(),
            "mcp_url": "https://docs.example.test/mcp",
        })
        release = self.release()
        self.assertEqual(receipt["tuple"], {
            "adapter_version": "0.1.24",
            "architecture": "amd64",
            "binary_digest": "sha256:" + hashlib.sha256(self.binary.read_bytes()).hexdigest(),
            "client_version": None,
            "dependency_identity": "remote-mcp:docs.example.test",
            "distribution_id": "example/demo",
            "distribution_kind": "upstream",
            "installer_version": "0.1.24",
            "manifest_digest": release["manifest_digest"],
            "observed_at": "2026-08-30T00:00:00Z",
            "os": "linux",
            "package_version": "1.0.0",
            "product_id": "demo",
            "release_sequence": 2,
            "snapshot_digest": self.snapshot_digest,
            "snapshot_sequence": 15,
            "source_path": "plugins/demo",
            "source_repository": "example/plugins",
            "source_revision": "a" * 40,
            "tree_digest": release["tree_digest"],
        })
        for name, value in (("app-binding.json", app), ("projection-receipt.json", receipt)):
            body = (self.root / "generated" / name).read_bytes()
            self.assertEqual(body, generator.canonical_output(value))
            self.assertFalse(body.endswith(b"\n"))
            self.assertEqual((self.root / "generated" / name).stat().st_mode & 0o777, 0o640)

    def test_same_inputs_produce_identical_bytes(self) -> None:
        self.generate(self.root / "first")
        self.generate(self.root / "second")
        for name in ("app-binding.json", "projection-receipt.json"):
            self.assertEqual((self.root / "first" / name).read_bytes(), (self.root / "second" / name).read_bytes())

    def test_projection_requires_exact_released_json_bytes(self) -> None:
        def reversed_objects(value):
            if isinstance(value, dict):
                return {key: reversed_objects(item) for key, item in reversed(tuple(value.items()))}
            if isinstance(value, list):
                return [reversed_objects(item) for item in value]
            return value

        paths = {
            "manifest": self.projection / ".codex-plugin/plugin.json",
            "mcp": self.projection / ".mcp.json",
            "app": self.projection / ".app.json",
        }
        for label, path in paths.items():
            with self.subTest(label=label):
                original = path.read_bytes()
                value = json.loads(original)
                if label == "app":
                    replacement = json.dumps(value, indent=2, sort_keys=False).encode() + b"\n"
                else:
                    replacement = json.dumps(
                        reversed_objects(value), indent=2, sort_keys=False,
                    ).encode() + b"\n"
                self.assertEqual(json.loads(replacement), value)
                self.assertNotEqual(replacement, original)
                path.write_bytes(replacement)
                self.write_state()
                with self.assertRaisesRegex(PublicationError, "bytes differ from the released"):
                    generator.generate(self.args(self.root / f"out-{label}"))
                path.write_bytes(original)
                self.write_state()

    def test_go_json_profiles_bind_key_order_whitespace_and_html_escaping(self) -> None:
        value = {"z": "<tag>&\u2028", "a": {"b": 1}}
        self.assertEqual(
            generator.released_json(value),
            b'{\n  "a": {\n    "b": 1\n  },\n  "z": "\\u003ctag\\u003e\\u0026\\u2028"\n}\n',
        )
        self.assertEqual(
            generator.released_compact_json(value),
            b'{"a":{"b":1},"z":"\\u003ctag\\u003e\\u0026\\u2028"}',
        )
        self.assertEqual(
            generator.released_manifest_json({
                "name": "demo",
                "author": {"url": "https://example.test", "email": "dev@example.test", "name": "Example"},
            }),
            b'{\n  "author": {\n    "name": "Example",\n    "email": "dev@example.test",\n    "url": "https://example.test"\n  },\n  "name": "demo"\n}\n',
        )

    def test_full_build_accepts_released_author_without_optional_email(self) -> None:
        actual_manifest = json.loads((ROOT / "plugins/cloudflare-docs/plugin.json").read_text())
        self.assertEqual(actual_manifest["author"], {
            "name": "777genius", "url": "https://github.com/777genius",
        })
        portable = json.loads((self.package / "plugin.json").read_text())
        portable["author"] = actual_manifest["author"]
        self.write_json(self.package / "plugin.json", portable)
        self.release().update(package_identity(self.package))
        self.write_signed_snapshot()

        projected = json.loads((self.projection / ".codex-plugin/plugin.json").read_text())
        projected["author"] = actual_manifest["author"]
        (self.projection / ".codex-plugin/plugin.json").write_bytes(
            generator.released_manifest_json(projected)
        )
        self.write_add()
        self.write_state()
        output = self.root / "author-without-email"
        generator.generate(self.args(output))
        self.assertTrue((output / "app-binding.json").is_file())
        self.assertTrue((output / "projection-receipt.json").is_file())

    def test_add_binds_uuid_v4_installation_to_physical_artifact(self) -> None:
        self.assertEqual(PHYSICAL_ARTIFACT_ID, "demo-449be4b09efa")
        cases = (
            (
                "installation",
                lambda value: value["data"]["targets"][0]["output"]["result"].update(
                    {"installation_id": "installation-demo"}
                ),
            ),
            (
                "artifact",
                lambda value: value["data"]["targets"][0]["output"]["result"]["plan"].update(
                    {"physical_artifact_id": "demo-attacker000000"}
                ),
            ),
        )
        for label, mutate in cases:
            with self.subTest(label=label):
                self.write_add(mutate)
                with self.assertRaisesRegex(PublicationError, "installation ID|physical artifact ID"):
                    generator.generate(self.args(self.root / f"out-{label}"))
                self.write_add()

    def test_wrong_requested_or_projected_app_id_fails_closed(self) -> None:
        for label, mutate in (
            ("argument", lambda: setattr(self.args(), "app_id", "plugin_asdk_app_" + "b" * 32)),
            ("projection", lambda: self.write_projection_json(self.projection / ".app.json", {"apps": {"demo": {"id": "plugin_asdk_app_" + "b" * 32}}})),
        ):
            with self.subTest(label=label):
                args = self.args(self.root / f"out-{label}")
                if label == "argument":
                    args.app_id = "plugin_asdk_app_" + "b" * 32
                else:
                    mutate()
                with self.assertRaises(PublicationError):
                    generator.generate(args)
                self.assertFalse(args.output.exists())
                if label == "projection":
                    self.write_projection_json(self.projection / ".app.json", {"apps": {"demo": {"id": APP_ID}}})

    def test_stale_snapshot_fails_closed(self) -> None:
        self.snapshot["expires_at"] = "2026-08-29T00:00:00Z"
        self.write_signed_snapshot()
        with self.assertRaisesRegex(PublicationError, "expired"):
            generator.generate(self.args())

    def test_release_source_and_digest_mismatches_fail_closed(self) -> None:
        cases = (
            ("source", lambda value: value["data"].update({"revision": "b" * 40})),
            ("digest", lambda value: value["data"]["directory"].update({"snapshot_digest": "sha256:" + "0" * 64})),
            ("release", lambda value: value["data"]["directory"].update({"desired_release_sequence": 1})),
        )
        for label, mutate in cases:
            with self.subTest(label=label):
                self.write_add(mutate)
                with self.assertRaises(PublicationError):
                    generator.generate(self.args())
                self.write_add()

    def test_wrong_cli_version_and_extra_signed_target_field_fail_closed(self) -> None:
        self.binary.write_bytes(b"#!/bin/sh\nprintf 'agentplugins 0.1.23\\n'\n")
        self.binary.chmod(0o755)
        wrong_version_digest = "sha256:" + hashlib.sha256(self.binary.read_bytes()).hexdigest()
        with mock.patch.object(generator, "CURRENT_LINUX_AMD64_DIGEST", wrong_version_digest):
            with self.assertRaisesRegex(PublicationError, "version output"):
                generator.generate(self.args())
        self.binary.write_bytes(b"#!/bin/sh\n[ \"$1\" = version ] && printf 'agentplugins 0.1.24\\n'\n")
        self.binary.chmod(0o755)
        distribution = next(item for item in self.snapshot["distributions"] if item["id"] == "example/demo")
        distribution["release_policies"][0]["targets"][0]["app_binding"]["extra"] = True
        self.write_signed_snapshot()
        with self.assertRaises(PublicationError):
            generator.generate(self.args())

    def test_generator_rejects_other_version_and_fake_current_binary(self) -> None:
        args = self.args(self.root / "wrong-current-version")
        args.installer_version = "0.1.25"
        self.binary.write_bytes(b"#!/bin/sh\nprintf 'agentplugins 0.1.25\\n'\n")
        self.binary.chmod(0o755)
        with self.assertRaisesRegex(PublicationError, "requires agentplugins 0.1.24"):
            generator.generate(args)

        self.binary.write_bytes(b"#!/bin/sh\nprintf 'agentplugins 0.1.24\\n'\n")
        self.binary.chmod(0o755)
        with mock.patch.object(
            generator,
            "CURRENT_LINUX_AMD64_DIGEST",
            "sha256:e79125f7ffabd11c6e211d6b049c2eb2b36eb1aba3a76ce27cac819aeba1e6ca",
        ):
            with self.assertRaisesRegex(PublicationError, "exact approved Linux/amd64 asset"):
                generator.generate(self.args(self.root / "fake-current-binary"))

    def test_ambiguous_state_client_and_extra_projection_field_fail_closed(self) -> None:
        def duplicate(state):
            installation = state["installations"][0]
            client = copy.deepcopy(next(iter(installation["clients"].values())))
            client["client_binding_id"] = "client-second"
            installation["clients"]["client-second"] = client

        self.write_state(duplicate)
        with self.assertRaises(PublicationError):
            generator.generate(self.args())
        self.write_state()
        self.write_projection_json(self.projection / ".app.json", {"apps": {"demo": {"id": APP_ID, "extra": True}}})
        with self.assertRaises(PublicationError):
            generator.generate(self.args())

    def test_output_directory_is_create_once(self) -> None:
        self.generate()
        with self.assertRaisesRegex(PublicationError, "already exists"):
            self.generate()

    def test_existing_empty_output_directory_is_never_replaced(self) -> None:
        output = self.root / "occupied"
        output.mkdir()
        with self.assertRaisesRegex(PublicationError, "already exists"):
            generator.generate(self.args(output))
        self.assertEqual(list(output.iterdir()), [])

    def test_projection_byte_tamper_fails_managed_receipt_binding(self) -> None:
        self.write_projection_json(self.projection / ".mcp.json", {
            "mcpServers": {"demo": {"type": "http", "url": "https://evil.example.test/mcp"}},
        })
        with self.assertRaisesRegex(PublicationError, "managed projection digest"):
            generator.generate(self.args())

    def test_coherent_state_and_mcp_substitution_fails_approved_package_binding(self) -> None:
        for label, url in (
            ("foreign-host", "https://evil.example.test/mcp"),
            ("foreign-path", "https://docs.example.test/other-path"),
        ):
            with self.subTest(label=label):
                self.write_projection_json(self.projection / ".mcp.json", {
                    "mcpServers": {"demo": {"type": "http", "url": url}},
                })
                self.write_state()
                args = self.args(self.root / f"out-{label}")
                with self.assertRaisesRegex(PublicationError, "not derived from the approved source package"):
                    generator.generate(args)
                self.assertFalse(args.output.exists())
        self.write_projection_json(self.projection / ".mcp.json", {
            "mcpServers": {"demo": {"type": "http", "url": "https://docs.example.test/mcp"}},
        })
        self.write_state()

    def test_coherent_state_cannot_bless_manifest_or_readme_substitution(self) -> None:
        original_manifest = json.loads((self.projection / ".codex-plugin/plugin.json").read_text())
        cases = (
            ("manifest", self.projection / ".codex-plugin/plugin.json", {**original_manifest, "description": "substituted"}, "official manifest"),
            ("readme", self.projection / "README.md", None, "README.md"),
        )
        for label, path, value, message in cases:
            with self.subTest(label=label):
                if value is None:
                    path.write_bytes(b"substituted\n")
                else:
                    self.write_projection_json(path, value)
                self.write_state()
                with self.assertRaisesRegex(PublicationError, message):
                    generator.generate(self.args())
                if label == "manifest":
                    self.write_projection_json(path, original_manifest)
                else:
                    path.write_bytes(b"# Demo\n")
        self.write_state()

    def test_coherent_state_cannot_bless_marketplace_notice_or_assets(self) -> None:
        marketplace = self.projection / ".agents/plugins/marketplace.json"
        original_marketplace = marketplace.read_bytes()
        cases = (
            ("marketplace", marketplace, b'{"name":"foreign","plugins":[]}\n', "marketplace.json"),
            ("notice", self.projection / "NOTICE", b"foreign notice\n", "NOTICE"),
        )
        for label, path, body, message in cases:
            with self.subTest(label=label):
                original = path.read_bytes()
                path.write_bytes(body)
                self.write_state()
                with self.assertRaisesRegex(PublicationError, message):
                    generator.generate(self.args())
                path.write_bytes(original)
        (self.projection / "assets").mkdir()
        (self.projection / "assets/icon.png").write_bytes(b"unexpected")
        self.write_state()
        with self.assertRaisesRegex(PublicationError, "outside the exact approved transformation"):
            generator.generate(self.args())
        shutil.rmtree(self.projection / "assets")
        marketplace.write_bytes(original_marketplace)
        self.write_state()

    def test_output_overlap_is_rejected_before_any_write(self) -> None:
        projection_digest = generator.projection_artifact_digest(self.projection)
        package_digest = package_identity(self.package)["tree_digest"]
        cases = (
            ("projection", self.projection),
            ("inside-projection", self.projection / "generated"),
            ("contains-projection", self.root),
            ("package", self.package),
            ("inside-package", self.package / "generated"),
        )
        for label, output in cases:
            with self.subTest(label=label):
                before = set(output.iterdir()) if output.exists() and output.is_dir() else set()
                with self.assertRaisesRegex(PublicationError, "overlaps an authenticated input root"):
                    generator.generate(self.args(output))
                after = set(output.iterdir()) if output.exists() and output.is_dir() else set()
                self.assertEqual(after, before)
                self.assertEqual(generator.projection_artifact_digest(self.projection), projection_digest)
                self.assertEqual(package_identity(self.package)["tree_digest"], package_digest)

    def test_output_symlink_ancestor_is_rejected_without_write_through(self) -> None:
        destination = self.root / "outside"
        destination.mkdir()
        link = self.root / "output-link"
        link.symlink_to(destination, target_is_directory=True)
        args = self.args(link / "generated")
        projection_digest = generator.projection_artifact_digest(self.projection)
        with self.assertRaisesRegex(PublicationError, "symlink or non-directory ancestor"):
            generator.generate(args)
        self.assertEqual(list(destination.iterdir()), [])
        self.assertFalse(args.output.exists())
        self.assertEqual(generator.projection_artifact_digest(self.projection), projection_digest)

    def test_protected_root_rename_into_output_parent_is_rejected_before_stage(self) -> None:
        output_parent = self.root / "publish-parent"
        output = output_parent / "generated"
        original_members = sorted(path.name for path in self.package.iterdir())
        real_open = generator._open_or_create_output_parent
        swapped = False

        def swap_then_open(candidate, protected_authorities):
            nonlocal swapped
            self.package.rename(output_parent)
            swapped = True
            return real_open(candidate, protected_authorities)

        try:
            with mock.patch.object(generator, "_open_or_create_output_parent", side_effect=swap_then_open):
                with self.assertRaisesRegex(PublicationError, "aliases an authenticated input root"):
                    generator.generate(self.args(output))
            self.assertTrue(swapped)
            self.assertFalse(output.exists())
            self.assertEqual(
                [path.name for path in output_parent.iterdir() if path.name.startswith(".generated.")],
                [],
            )
        finally:
            if swapped and output_parent.exists():
                output_parent.rename(self.package)
        self.assertEqual(sorted(path.name for path in self.package.iterdir()), original_members)

    def test_package_rename_and_path_recreation_between_build_and_write_is_rejected(self) -> None:
        output_parent = self.root / "package-authority-swap"
        output = output_parent / "generated"
        swapped = False

        def swap_before_publish():
            nonlocal swapped
            self.package.rename(output_parent)
            self.package.mkdir()
            swapped = True

        try:
            with mock.patch.object(generator, "_publication_preflight_hook", side_effect=swap_before_publish):
                with self.assertRaisesRegex(
                    PublicationError, "pathname changed between validation and publication",
                ):
                    generator.generate(self.args(output))
            self.assertFalse(output.exists())
            self.assertEqual(
                [path.name for path in output_parent.iterdir() if path.name.startswith(".generated.")],
                [],
            )
        finally:
            if swapped:
                shutil.rmtree(self.package)
                output_parent.rename(self.package)

    def test_projection_rename_and_path_recreation_between_build_and_write_is_rejected(self) -> None:
        output_parent = self.root / "projection-authority-swap"
        output = output_parent / "generated"
        swapped = False

        def swap_before_publish():
            nonlocal swapped
            self.projection.rename(output_parent)
            self.projection.mkdir()
            swapped = True

        try:
            with mock.patch.object(generator, "_publication_preflight_hook", side_effect=swap_before_publish):
                with self.assertRaisesRegex(
                    PublicationError, "pathname changed between validation and publication",
                ):
                    generator.generate(self.args(output))
            self.assertFalse(output.exists())
            self.assertEqual(
                [path.name for path in output_parent.iterdir() if path.name.startswith(".generated.")],
                [],
            )
        finally:
            if swapped:
                shutil.rmtree(self.projection)
                output_parent.rename(self.projection)

    def test_projection_content_mutation_between_validation_and_publish_is_rejected(self) -> None:
        output = self.root / "stale-projection"
        def mutate_before_publish():
            (self.projection / "README.md").write_bytes(b"stale attacker bytes\n")

        with mock.patch.object(generator, "_publication_preflight_hook", side_effect=mutate_before_publish):
            with self.assertRaisesRegex(PublicationError, "input tree changed after validation"):
                generator.generate(self.args(output))
        self.assertFalse(output.exists())
        self.assertFalse(any(path.name.startswith(".stale-projection.") for path in self.root.iterdir()))

    def test_nested_add_and_delete_between_validation_and_publish_are_rejected(self) -> None:
        notice = (self.projection / "NOTICE").read_bytes()

        def add_before_publish():
            nested = self.package / "nested"
            nested.mkdir()
            (nested / "attacker").write_bytes(b"attacker")

        with mock.patch.object(generator, "_publication_preflight_hook", side_effect=add_before_publish):
            with self.assertRaisesRegex(PublicationError, "input tree changed after validation"):
                generator.generate(self.args(self.root / "nested-add"))
        shutil.rmtree(self.package / "nested")

        def delete_before_publish():
            (self.projection / "NOTICE").unlink()

        try:
            with mock.patch.object(generator, "_publication_preflight_hook", side_effect=delete_before_publish):
                with self.assertRaisesRegex(PublicationError, "input tree changed after validation"):
                    generator.generate(self.args(self.root / "nested-delete"))
        finally:
            (self.projection / "NOTICE").write_bytes(notice)
        self.assertFalse((self.root / "nested-add").exists())
        self.assertFalse((self.root / "nested-delete").exists())

    def test_module_exposes_no_payload_writer_or_publication_capability(self) -> None:
        self.assertFalse(hasattr(generator, "_write_outputs"))
        self.assertFalse(hasattr(generator, "write_outputs"))
        self.assertFalse(hasattr(generator, "_ValidatedPublication"))
        self.assertFalse(hasattr(generator, "_PUBLICATION_CAPABILITY"))

    def test_input_mutation_during_staged_write_quarantines_owned_stage(self) -> None:
        output = self.root / "during-stage"
        real_revalidate = generator._revalidate_protected_tree
        calls = 0

        def mutate_on_post_stage(expected):
            nonlocal calls
            calls += 1
            if calls == 3:
                (self.projection / "README.md").write_bytes(b"changed during stage\n")
            return real_revalidate(expected)

        with mock.patch.object(generator, "_revalidate_protected_tree", side_effect=mutate_on_post_stage):
            with self.assertRaisesRegex(PublicationError, "input tree changed after validation"):
                generator.generate(self.args(output))
        self.assertGreaterEqual(calls, 3)
        self.assertFalse(output.exists())
        self.assertFalse(any(path.name.startswith(".during-stage.") for path in self.root.iterdir()))
        rejected = list(self.root.glob(".rejected-stage-*"))
        self.assertEqual(len(rejected), 1)
        self.assertTrue((rejected[0] / "app-binding.json").is_file())

    def test_stage_open_emfile_in_crowded_parent_quarantines_exact_empty_stage(self) -> None:
        output = self.root / "stage-open-emfile"
        for index in range(4097):
            (self.root / f"crowd-{index:04d}").touch()
        real_open = generator.os.open
        injected = False

        def fail_exact_stage_open(path, flags, *args, **kwargs):
            nonlocal injected
            if (
                not injected
                and isinstance(path, str)
                and path.startswith(f".{output.name}.")
                and path.endswith(".tmp")
            ):
                injected = True
                raise OSError(errno.EMFILE, os.strerror(errno.EMFILE))
            return real_open(path, flags, *args, **kwargs)

        with mock.patch.object(generator.os, "open", side_effect=fail_exact_stage_open):
            with self.assertRaisesRegex(OSError, "Too many open files"):
                generator.generate(self.args(output))
        self.assertTrue(injected)
        self.assertFalse(output.exists())
        self.assertEqual(
            [path.name for path in self.root.iterdir() if path.name.startswith(f".{output.name}.")],
            [],
        )
        rejected = list(self.root.glob(".rejected-stage-*"))
        self.assertEqual(len(rejected), 1)
        self.assertEqual(list(rejected[0].iterdir()), [])

    def test_stage_swap_before_descriptor_open_preserves_foreign_and_exact_owned_stage(self) -> None:
        output = self.root / "stage-preopen-swap"
        real_open = generator.os.open
        swapped = False

        def swap_before_stage_open(path, flags, *args, **kwargs):
            nonlocal swapped
            if (
                not swapped
                and isinstance(path, str)
                and path.startswith(f".{output.name}.")
                and path.endswith(".tmp")
                and flags & getattr(os, "O_DIRECTORY", 0)
            ):
                parent_descriptor = kwargs["dir_fd"]
                os.rename(path, "owned-before-open", src_dir_fd=parent_descriptor, dst_dir_fd=parent_descriptor)
                os.mkdir(path, mode=0o700, dir_fd=parent_descriptor)
                foreign = real_open(path, os.O_RDONLY | os.O_DIRECTORY, dir_fd=parent_descriptor)
                try:
                    marker = real_open(
                        "foreign-marker", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600,
                        dir_fd=foreign,
                    )
                    os.close(marker)
                finally:
                    os.close(foreign)
                swapped = True
            return real_open(path, flags, *args, **kwargs)

        with mock.patch.object(generator.os, "open", side_effect=swap_before_stage_open):
            with self.assertRaisesRegex(PublicationError, "changed before descriptor open"):
                generator.generate(self.args(output))
        self.assertTrue(swapped)
        self.assertFalse(output.exists())
        self.assertFalse((self.root / "owned-before-open").exists())
        rejected = list(self.root.glob(".rejected-stage-*"))
        self.assertEqual(len(rejected), 2)
        self.assertEqual(
            sum((path / "foreign-marker").is_file() for path in rejected),
            1,
        )

    def test_initial_stage_identity_error_preserves_create_once_residue(self) -> None:
        output = self.root / "stage-initial-stat-error"
        real_identity = generator._stage_path_identity
        injected = False

        def fail_initial_identity(parent_descriptor, name):
            nonlocal injected
            if not injected and name.startswith(f".{output.name}.") and name.endswith(".tmp"):
                injected = True
                raise OSError(errno.EIO, os.strerror(errno.EIO))
            return real_identity(parent_descriptor, name)

        with mock.patch.object(generator, "_stage_path_identity", side_effect=fail_initial_identity):
            with self.assertRaisesRegex(OSError, "Input/output error"):
                generator.generate(self.args(output))
        self.assertTrue(injected)
        self.assertFalse(output.exists())
        self.assertEqual(
            [path.name for path in self.root.iterdir() if path.name.startswith(f".{output.name}.")],
            [],
        )
        rejected = list(self.root.glob(".rejected-stage-*"))
        self.assertEqual(len(rejected), 1)
        self.assertEqual(list(rejected[0].iterdir()), [])

    def test_stage_path_replacement_during_rename_is_quarantined_without_publication(self) -> None:
        output = self.root / "stage-replaced"
        real_rename = generator._rename_noreplace
        swapped = False

        def replace_stage(parent_descriptor, source, destination):
            nonlocal swapped
            if not swapped and destination == output.name:
                os.rename(source, "legitimate-retained", src_dir_fd=parent_descriptor, dst_dir_fd=parent_descriptor)
                os.mkdir(source, mode=0o700, dir_fd=parent_descriptor)
                attacker = os.open(source, os.O_RDONLY | os.O_DIRECTORY, dir_fd=parent_descriptor)
                try:
                    marker = os.open("attacker", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600, dir_fd=attacker)
                    os.close(marker)
                finally:
                    os.close(attacker)
                swapped = True
            return real_rename(parent_descriptor, source, destination)

        with mock.patch.object(generator, "_rename_noreplace", side_effect=replace_stage):
            with self.assertRaisesRegex(PublicationError, "replaced during rename"):
                generator.generate(self.args(output))
        self.assertTrue(swapped)
        self.assertFalse(output.exists())
        self.assertFalse((self.root / "legitimate-retained").exists())
        quarantined = list(self.root.glob(".foreign-stage-replaced.*"))
        self.assertEqual(len(quarantined), 1)
        self.assertTrue((quarantined[0] / "attacker").is_file())
        owned = list(self.root.glob(".rejected-stage-*"))
        self.assertEqual(len(owned), 1)
        self.assertTrue((owned[0] / "app-binding.json").is_file())

    def test_stage_replacement_with_foreign_file_during_rename_is_quarantined(self) -> None:
        output = self.root / "stage-file-replaced"
        real_rename = generator._rename_noreplace
        swapped = False

        def replace_with_file(parent_descriptor, source, destination):
            nonlocal swapped
            real_rename(parent_descriptor, source, destination)
            if not swapped and destination == output.name:
                os.rename(destination, "owned-retained", src_dir_fd=parent_descriptor, dst_dir_fd=parent_descriptor)
                descriptor = os.open(
                    destination,
                    os.O_WRONLY | os.O_CREAT | os.O_EXCL,
                    0o600,
                    dir_fd=parent_descriptor,
                )
                os.write(descriptor, b"preserve foreign file")
                os.close(descriptor)
                swapped = True

        with mock.patch.object(generator, "_rename_noreplace", side_effect=replace_with_file):
            with self.assertRaisesRegex(PublicationError, "replaced during rename"):
                generator.generate(self.args(output))
        self.assertTrue(swapped)
        self.assertFalse(output.exists())
        foreign = list(self.root.glob(f".foreign-{output.name}.*"))
        self.assertEqual(len(foreign), 1)
        self.assertEqual(foreign[0].read_bytes(), b"preserve foreign file")
        owned = list(self.root.glob(f".rejected-{output.name}.*"))
        self.assertEqual(len(owned), 1)
        self.assertTrue((owned[0] / "app-binding.json").is_file())

    def test_stage_cleanup_race_preserves_foreign_and_owned_inodes(self) -> None:
        parent = self.root / "cleanup-race"
        parent.mkdir()
        owned = parent / "owned"
        owned.mkdir()
        (owned / "app-binding.json").write_bytes(b"owned")
        (owned / "projection-receipt.json").write_bytes(b"owned")
        expected = generator._directory_identity(owned.stat())
        foreign = parent / "foreign"
        foreign.mkdir()
        (foreign / "attacker").write_bytes(b"preserve me")
        parent_descriptor = os.open(parent, os.O_RDONLY | os.O_DIRECTORY)
        real_rename = generator._rename_noreplace
        swapped = False

        def swap_after_identity_check(descriptor, source, destination):
            nonlocal swapped
            if not swapped and source == "owned":
                os.rename("owned", "owned-moved", src_dir_fd=descriptor, dst_dir_fd=descriptor)
                os.rename("foreign", "owned", src_dir_fd=descriptor, dst_dir_fd=descriptor)
                swapped = True
            return real_rename(descriptor, source, destination)

        try:
            with mock.patch.object(generator, "_rename_noreplace", side_effect=swap_after_identity_check):
                exact_quarantine = generator._quarantine_owned_stage(parent_descriptor, expected)
        finally:
            os.close(parent_descriptor)
        self.assertTrue(swapped)
        self.assertIsNotNone(exact_quarantine)
        exact_path = parent / str(exact_quarantine)
        rejected = list(parent.glob(".rejected-stage-*"))
        self.assertEqual(len(rejected), 2)
        self.assertEqual(generator._directory_identity(exact_path.stat()), expected)
        foreign_quarantine = next(path for path in rejected if path.name != exact_quarantine)
        self.assertEqual((foreign_quarantine / "attacker").read_bytes(), b"preserve me")
        self.assertEqual((exact_path / "app-binding.json").read_bytes(), b"owned")

    def test_stage_recovery_bounds_crowded_parent_before_member_stat(self) -> None:
        parent = self.root / "bounded-recovery"
        parent.mkdir()
        for index in range(4097):
            (parent / f"entry-{index:04d}").touch()
        parent_descriptor = os.open(parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            with mock.patch.object(generator.os, "stat", side_effect=AssertionError("must bound before stat")):
                with self.assertRaisesRegex(PublicationError, "too many entries"):
                    generator._quarantine_owned_stage(parent_descriptor, (1, 1))
        finally:
            os.close(parent_descriptor)

    def test_tree_snapshot_bounds_member_collection_before_sort_or_stat(self) -> None:
        class FakeScandir:
            def __enter__(self):
                return iter(
                    argparse.Namespace(name=f"entry-{index:05d}")
                    for index in range(generator.MAX_PROJECTION_FILES + generator.MAX_SNAPSHOT_DIRECTORIES)
                )

            def __exit__(self, *_args):
                return False

        with (
            mock.patch.object(generator.os, "scandir", return_value=FakeScandir()),
            mock.patch.object(generator.os, "stat", side_effect=AssertionError("must bound before stat")),
        ):
            with self.assertRaisesRegex(PublicationError, "snapshot member limits"):
                generator._stable_tree_snapshot(self.projection, "projection")

    def test_tree_snapshot_uses_one_global_nested_discovery_budget(self) -> None:
        root = self.root / "nested-budget"
        for directory, count in (("a", 3), ("b", 2)):
            child = root / directory
            child.mkdir(parents=True, exist_ok=True)
            for index in range(count):
                (child / f"{directory}-file-{index}").touch()
        real_stat = generator.os.stat
        second_child_stats = 0

        def track_stat(path, *args, **kwargs):
            nonlocal second_child_stats
            if isinstance(path, str) and path.startswith("b-file-"):
                second_child_stats += 1
            return real_stat(path, *args, **kwargs)

        with (
            mock.patch.object(generator, "MAX_PROJECTION_FILES", 4),
            mock.patch.object(generator, "MAX_SNAPSHOT_DIRECTORIES", 3),
            mock.patch.object(generator.os, "stat", side_effect=track_stat),
        ):
            with self.assertRaisesRegex(PublicationError, "snapshot member limits"):
                generator._stable_tree_snapshot(root, "projection")
        self.assertEqual(second_child_stats, 0)

    def test_stage_members_are_exact_at_pre_and_post_rename_boundaries(self) -> None:
        actions = ("extra", "replace", "tamper", "type")
        for boundary_call, boundary in ((1, "pre"), (2, "post")):
            for action in actions:
                with self.subTest(boundary=boundary, action=action):
                    output = self.root / f"stage-members-{boundary}-{action}"
                    real_seal = generator._publication_directory_seal
                    calls = 0

                    def mutate_then_seal(descriptor, expected_bodies):
                        nonlocal calls
                        calls += 1
                        if calls == boundary_call:
                            if action == "extra":
                                member = os.open(
                                    "attacker-extra", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o640,
                                    dir_fd=descriptor,
                                )
                                os.close(member)
                            elif action == "replace":
                                os.unlink("app-binding.json", dir_fd=descriptor)
                                member = os.open(
                                    "app-binding.json", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o640,
                                    dir_fd=descriptor,
                                )
                                os.write(member, b"replacement")
                                os.close(member)
                            elif action == "tamper":
                                member = os.open(
                                    "projection-receipt.json", os.O_WRONLY | os.O_TRUNC,
                                    dir_fd=descriptor,
                                )
                                os.write(member, b"tampered")
                                os.close(member)
                            else:
                                os.unlink("app-binding.json", dir_fd=descriptor)
                                os.mkdir("app-binding.json", mode=0o700, dir_fd=descriptor)
                        return real_seal(descriptor, expected_bodies)

                    rejected_before = set(self.root.glob(".rejected-*"))
                    with mock.patch.object(generator, "_publication_directory_seal", side_effect=mutate_then_seal):
                        with self.assertRaises(PublicationError):
                            generator.generate(self.args(output))
                    self.assertFalse(output.exists())
                    rejected_after = set(self.root.glob(".rejected-*"))
                    self.assertEqual(len(rejected_after - rejected_before), 1)

    def test_source_mutation_inside_publication_rename_rejects_exact_output(self) -> None:
        for label, target in (
            ("package", self.package / "README.md"),
            ("projection", self.projection / "README.md"),
        ):
            with self.subTest(label=label):
                output = self.root / f"rename-source-{label}"
                real_rename = generator._rename_noreplace
                mutated = False
                original = target.read_bytes()

                def mutate_after_rename(parent_descriptor, source, destination):
                    nonlocal mutated
                    real_rename(parent_descriptor, source, destination)
                    if not mutated and destination == output.name:
                        target.write_bytes(f"{label} changed at rename\n".encode())
                        mutated = True

                try:
                    with mock.patch.object(generator, "_rename_noreplace", side_effect=mutate_after_rename):
                        with self.assertRaisesRegex(PublicationError, "input tree changed after validation"):
                            generator.generate(self.args(output))
                finally:
                    target.write_bytes(original)
                self.assertTrue(mutated)
                self.assertFalse(output.exists())
                rejected = list(self.root.glob(f".rejected-{output.name}.*"))
                self.assertEqual(len(rejected), 1)
                self.assertTrue((rejected[0] / "app-binding.json").is_file())
                self.assertTrue((rejected[0] / "projection-receipt.json").is_file())

    def test_foreign_output_substitution_during_source_rejection_preserves_primary_error(self) -> None:
        output = self.root / "rejection-substitution"
        owned_relocated = self.root / "owned-relocated"
        original = (self.package / "README.md").read_bytes()
        real_revalidate = generator._revalidate_protected_tree
        real_reject = generator._reject_published_output
        calls = 0
        substituted = False

        def substitute_then_reject(expected):
            nonlocal calls, substituted
            calls += 1
            if calls == 5:
                output.rename(owned_relocated)
                output.mkdir()
                (output / "foreign-marker").write_bytes(b"preserve foreign")
                (self.package / "README.md").write_bytes(b"source changed at rejection\n")
                substituted = True
            return real_revalidate(expected)

        def cleanup_then_raise(*args, **kwargs):
            real_reject(*args, **kwargs)
            raise OSError(errno.EIO, "injected cleanup failure after preservation")

        try:
            with (
                mock.patch.object(generator, "_revalidate_protected_tree", side_effect=substitute_then_reject),
                mock.patch.object(generator, "_reject_published_output", side_effect=cleanup_then_raise),
            ):
                with self.assertRaisesRegex(PublicationError, "input tree changed after validation"):
                    generator.generate(self.args(output))
        finally:
            (self.package / "README.md").write_bytes(original)
        self.assertTrue(substituted)
        self.assertFalse(output.exists())
        foreign = list(self.root.glob(f".foreign-{output.name}.*"))
        self.assertEqual(len(foreign), 1)
        self.assertEqual((foreign[0] / "foreign-marker").read_bytes(), b"preserve foreign")
        rejected = list(self.root.glob(f".rejected-{output.name}.*"))
        self.assertEqual(len(rejected), 1)
        self.assertTrue((rejected[0] / "app-binding.json").is_file())
        self.assertFalse(owned_relocated.exists())

    def test_output_case_alias_is_rejected_or_remains_a_distinct_sibling(self) -> None:
        alternate = self.root / "PACKAGE"
        aliases_package = alternate.exists() and os.path.samefile(alternate, self.package)
        output = alternate / "generated"
        if aliases_package:
            with self.assertRaisesRegex(PublicationError, "aliases an authenticated input root"):
                generator.generate(self.args(output))
            self.assertFalse(output.exists())
            return

        # This assertion is intentionally conditional on a truly case-sensitive
        # filesystem: a distinct sibling must not be rejected merely by folding.
        alternate.mkdir()
        self.generate(output)
        self.assertTrue((output / "app-binding.json").is_file())

    def test_source_package_nested_reserved_path_is_not_silently_ignored(self) -> None:
        nested = self.package / "nested"
        nested.mkdir()
        (nested / ".plugin-kit-ai.lock").write_bytes(b"attacker")
        with self.assertRaisesRegex(PublicationError, "reserved ownership metadata"):
            generator.generate(self.args())

    def test_root_and_nested_ownership_markers_are_rejected_in_both_input_trees(self) -> None:
        cases = (
            ("package-root-git", self.package / ".git", True),
            ("package-root-lock", self.package / ".plugin-kit-ai.lock", False),
            ("package-nested-git", self.package / "nested" / ".git", True),
            ("package-nested-lock", self.package / "nested" / ".plugin-kit-ai.lock", False),
            ("projection-root-git", self.projection / ".git", True),
            ("projection-root-lock", self.projection / ".plugin-kit-ai.lock", False),
            ("projection-nested-git", self.projection / "nested" / ".git", True),
            ("projection-nested-lock", self.projection / "nested" / ".plugin-kit-ai.lock", False),
        )
        for label, marker, directory in cases:
            with self.subTest(label=label):
                marker.parent.mkdir(parents=True, exist_ok=True)
                if directory:
                    marker.mkdir()
                else:
                    marker.write_bytes(b"attacker")
                try:
                    with self.assertRaisesRegex(PublicationError, "reserved ownership metadata"):
                        generator.generate(self.args(self.root / f"out-{label}"))
                finally:
                    if directory:
                        marker.rmdir()
                    else:
                        marker.unlink()
                    if marker.parent.name == "nested" and not any(marker.parent.iterdir()):
                        marker.parent.rmdir()

    def test_unapproved_package_bytes_fail_before_output(self) -> None:
        self.write_json(self.package / "mcp.json", {
            "mcpServers": {"demo": {"type": "streamable-http", "url": "https://evil.example.test/mcp"}},
        })
        args = self.args()
        with self.assertRaisesRegex(PublicationError, "bytes differ from the signed release"):
            generator.generate(args)
        self.assertFalse(args.output.exists())

    def test_projection_post_read_app_mutation_is_detected(self) -> None:
        real_read = generator._read_snapshot_file
        changed = False

        def mutate_after_read(descriptor, size, label):
            nonlocal changed
            body = real_read(descriptor, size, label)
            if not changed and label.endswith("file '.app.json'"):
                changed = True
                self.write_projection_json(self.projection / ".app.json", {"apps": {"demo": {"id": "plugin_asdk_app_" + "b" * 32}}})
            return body

        args = self.args()
        with mock.patch.object(generator, "_read_snapshot_file", side_effect=mutate_after_read):
            with self.assertRaisesRegex(PublicationError, "changed .* reading"):
                generator.generate(args)
        self.assertTrue(changed)
        self.assertFalse(args.output.exists())

    def test_projection_nested_insertion_after_traversal_is_detected(self) -> None:
        nested = self.projection / "nested"
        nested.mkdir()
        (nested / "original.txt").write_bytes(b"original")
        self.write_state()
        real_read = generator._read_snapshot_file
        inserted = False

        def insert_after_read(descriptor, size, label):
            nonlocal inserted
            body = real_read(descriptor, size, label)
            if not inserted and label.endswith("file 'nested/original.txt'"):
                inserted = True
                (nested / "inserted.txt").write_bytes(b"inserted")
            return body

        args = self.args()
        with mock.patch.object(generator, "_read_snapshot_file", side_effect=insert_after_read):
            with self.assertRaisesRegex(PublicationError, "changed after"):
                generator.generate(args)
        self.assertTrue(inserted)
        self.assertFalse(args.output.exists())

    def test_cli_symlink_leaf_and_ancestor_are_rejected_before_execution(self) -> None:
        leaf = self.root / "agentplugins-link"
        leaf.symlink_to(self.binary)
        real_directory = self.root / "real-bin"
        real_directory.mkdir()
        shutil.copy2(self.binary, real_directory / "agentplugins")
        ancestor = self.root / "bin-link"
        ancestor.symlink_to(real_directory, target_is_directory=True)
        with mock.patch.object(generator.subprocess, "run") as run:
            for label, path in (("leaf", leaf), ("ancestor", ancestor / "agentplugins")):
                with self.subTest(label=label):
                    with self.assertRaisesRegex(PublicationError, "path is unsafe"):
                        generator.validate_binary(path, "0.1.24", self.fake_binary_digest)
            run.assert_not_called()

    def test_binary_path_swap_executes_authenticated_image_and_fails_authority_recheck(self) -> None:
        real_run = generator.subprocess.run
        original_parent = self.binary.parent
        moved_parent = original_parent.with_name(original_parent.name + "-moved")

        def swap_then_run(*args, **kwargs):
            original_parent.rename(moved_parent)
            original_parent.mkdir()
            replacement = original_parent / "agentplugins"
            replacement.write_bytes(b"#!/bin/sh\nprintf 'agentplugins 9.9.9\\n'\n")
            replacement.chmod(0o755)
            return real_run(*args, **kwargs)

        with mock.patch.object(generator.subprocess, "run", side_effect=swap_then_run):
            with self.assertRaisesRegex(PublicationError, "ancestor pathname was replaced"):
                generator.validate_binary(self.binary, "0.1.24", self.fake_binary_digest)
        shutil.rmtree(original_parent)
        moved_parent.rename(original_parent)

    def test_binary_leaf_swap_cannot_change_executed_image(self) -> None:
        saved = self.binary.with_name("agentplugins-authenticated")
        real_run = generator.subprocess.run
        observed: list[str] = []

        def swap_then_run(*args, **kwargs):
            self.binary.rename(saved)
            self.binary.write_bytes(b"#!/bin/sh\nprintf 'agentplugins 9.9.9\\n'\n")
            self.binary.chmod(0o755)
            completed = real_run(*args, **kwargs)
            observed.append(completed.stdout)
            return completed

        try:
            with mock.patch.object(generator.subprocess, "run", side_effect=swap_then_run):
                with self.assertRaisesRegex(PublicationError, "leaf pathname was replaced"):
                    generator.validate_binary(self.binary, "0.1.24", self.fake_binary_digest)
            self.assertEqual(observed, ["agentplugins 0.1.24\n"])
        finally:
            self.binary.unlink(missing_ok=True)
            saved.rename(self.binary)

    def test_unapproved_cli_digest_is_rejected_before_execution(self) -> None:
        with mock.patch.object(generator, "_run_authenticated_binary") as run:
            with self.assertRaisesRegex(PublicationError, "exact approved Linux/amd64 asset"):
                generator.validate_binary(
                    self.binary,
                    "0.1.24",
                    "sha256:" + "0" * 64,
                )
        run.assert_not_called()

    def test_atomic_publish_loses_race_without_replacing_winner(self) -> None:
        output = self.root / "raced"
        real_rename = generator._rename_noreplace

        def install_winner(parent_descriptor, source, destination):
            os.mkdir(destination, mode=0o700, dir_fd=parent_descriptor)
            winner_dir = os.open(destination, os.O_RDONLY | os.O_DIRECTORY, dir_fd=parent_descriptor)
            try:
                winner = os.open("winner", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600, dir_fd=winner_dir)
                try:
                    os.write(winner, b"winner")
                finally:
                    os.close(winner)
            finally:
                os.close(winner_dir)
            return real_rename(parent_descriptor, source, destination)

        with mock.patch.object(generator, "_rename_noreplace", side_effect=install_winner):
            with self.assertRaisesRegex(PublicationError, "already exists"):
                generator.generate(self.args(output))
        self.assertEqual((output / "winner").read_bytes(), b"winner")


if __name__ == "__main__":
    unittest.main()
