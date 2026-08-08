import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[1]


class WorkflowContractTests(unittest.TestCase):
    def test_release_publish_is_idempotent(self) -> None:
        workflow = (REPO_ROOT / ".github/workflows/live-e2e.yml").read_text()

        view_release = 'gh release view "$GITHUB_REF_NAME"'
        create_release = 'gh release create "$GITHUB_REF_NAME"'

        self.assertIn(view_release, workflow)
        self.assertIn(create_release, workflow)
        self.assertLess(workflow.index(view_release), workflow.index(create_release))
        self.assertIn('test "$release_is_draft" = "false"', workflow)


if __name__ == "__main__":
    unittest.main()
