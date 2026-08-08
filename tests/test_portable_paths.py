from __future__ import annotations

import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


SCRIPTS = Path(__file__).resolve().parents[1] / "scripts"
sys.path.insert(0, str(SCRIPTS))
import portable_paths  # noqa: E402


class PortablePathLimitTests(unittest.TestCase):
    def test_depth_64_is_allowed_and_depth_65_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            current = root
            for index in range(64):
                current /= f"d{index}"
                current.mkdir()
            portable_paths.validate_tree(root)
            too_deep = current / "overflow"
            too_deep.mkdir()
            with self.assertRaisesRegex(ValueError, "exceeds depth 64"):
                portable_paths.validate_tree(root)

    def test_file_count_limit_matches_runtime_boundary(self) -> None:
        with tempfile.TemporaryDirectory() as tmp, patch.object(portable_paths, "MAX_FILES", 1):
            root = Path(tmp)
            (root / "one").write_bytes(b"1")
            (root / "two").write_bytes(b"2")
            with self.assertRaisesRegex(ValueError, "exceeds 1 files"):
                portable_paths.validate_tree(root)

    def test_individual_and_total_size_limits_match_runtime_boundaries(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "one").write_bytes(b"1234")
            with patch.object(portable_paths, "MAX_FILE_BYTES", 3):
                with self.assertRaisesRegex(ValueError, "file exceeds 3 bytes"):
                    portable_paths.validate_tree(root)
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "one").write_bytes(b"123")
            (root / "two").write_bytes(b"456")
            with patch.object(portable_paths, "MAX_TREE_BYTES", 5):
                with self.assertRaisesRegex(ValueError, "exceeds 5 total bytes"):
                    portable_paths.validate_tree(root)


if __name__ == "__main__":
    unittest.main()
