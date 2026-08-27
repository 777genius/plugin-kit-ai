from __future__ import annotations

import hashlib
import os
import tempfile
import unittest
from pathlib import Path

from observer.client_bundle import canonical_json, inventory_bundle, verify_bundle


class ClientBundleTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.root = Path(self.temporary.name).resolve() / "cursor"
        self.root.mkdir(mode=0o755)
        (self.root / "cursor-agent").write_bytes(b"agent")
        (self.root / "cursor-agent").chmod(0o755)
        (self.root / "index.js").write_bytes(b"index")
        (self.root / "index.js").chmod(0o644)
        self.uid = os.geteuid()
        self.gid = os.getegid()
        self.manifest = self.root.parent / "cursor-bundle.json"
        self.manifest.write_bytes(canonical_json(inventory_bundle(
            self.root, owner_uid=self.uid, owner_gid=self.gid,
        )))
        self.manifest.chmod(0o644)

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def digest(self) -> str:
        return "sha256:" + hashlib.sha256(self.manifest.read_bytes()).hexdigest()

    def verify(self) -> tuple[Path, ...]:
        return verify_bundle(
            root=self.root,
            manifest=self.manifest,
            manifest_sha256=self.digest(),
            owner_uid=self.uid,
            owner_gid=self.gid,
        )

    def test_complete_bundle_is_digest_bound(self) -> None:
        self.assertEqual(
            self.verify(),
            (self.root / "cursor-agent", self.root / "index.js"),
        )

    def test_changed_or_unexpected_file_is_rejected(self) -> None:
        (self.root / "index.js").write_bytes(b"tampered")
        with self.assertRaisesRegex(ValueError, "bundle bytes differ"):
            self.verify()
        (self.root / "index.js").write_bytes(b"index")
        (self.root / "extra.js").write_bytes(b"extra")
        (self.root / "extra.js").chmod(0o644)
        with self.assertRaisesRegex(ValueError, "bundle bytes differ"):
            self.verify()

    def test_symlink_and_noncanonical_manifest_are_rejected(self) -> None:
        (self.root / "link").symlink_to("index.js")
        with self.assertRaisesRegex(ValueError, "metadata differs"):
            inventory_bundle(self.root, owner_uid=self.uid, owner_gid=self.gid)
        (self.root / "link").unlink()
        self.manifest.write_bytes(self.manifest.read_bytes() + b" ")
        with self.assertRaisesRegex(ValueError, "manifest bytes are not canonical"):
            self.verify()


if __name__ == "__main__":
    unittest.main()
