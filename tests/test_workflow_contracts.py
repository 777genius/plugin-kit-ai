import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[1]


class WorkflowContractTests(unittest.TestCase):
    def test_release_publish_uses_tested_script(self) -> None:
        workflow = (REPO_ROOT / ".github/workflows/live-e2e.yml").read_text()

        self.assertIn("run: scripts/publish_github_release.sh", workflow)


if __name__ == "__main__":
    unittest.main()
