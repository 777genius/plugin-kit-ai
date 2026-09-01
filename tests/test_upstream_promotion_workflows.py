import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[1]
OBSERVER = ROOT / ".github/workflows/upstream-promotion-observer.yml"
POLICY = ROOT / ".github/workflows/upstream-promotion-policy.yml"
AUTO_MERGE = ROOT / ".github/workflows/upstream-promotion-auto-merge.yml"


def workflow(path: Path) -> dict:
    return yaml.load(path.read_text(), Loader=yaml.BaseLoader)


class UpstreamPromotionWorkflowTests(unittest.TestCase):
    def test_observer_is_scheduled_manual_serial_and_uses_app_pr_not_main_push(self) -> None:
        value = workflow(OBSERVER)
        self.assertEqual(set(value["on"]), {"schedule", "workflow_dispatch"})
        self.assertEqual(value["concurrency"]["cancel-in-progress"], "false")
        self.assertEqual(value["permissions"], {"actions": "read", "contents": "read", "pull-requests": "read"})
        self.assertEqual(value["jobs"]["observe"]["environment"], "upstream-promotion-observer")
        body = OBSERVER.read_text()
        self.assertIn("actions/create-github-app-token@bcd2ba49218906704ab6c1aa796996da409d3eb1", body)
        self.assertIn("secrets.UPSTREAM_PROMOTION_APP_ID", body)
        self.assertIn("secrets.UPSTREAM_PROMOTION_APP_PRIVATE_KEY", body)
        self.assertIn("permission-contents: write", body)
        self.assertIn("permission-pull-requests: write", body)
        self.assertNotIn("DIRECTORY_PUBLISHER", body)
        self.assertNotIn("publisher-identity", body)
        self.assertIn("scripts/run_upstream_promotion_materialization.py", body)
        self.assertIn("scripts/validate_review_journey.py promotion", body)
        self.assertIn('git push origin "HEAD:refs/heads/$BRANCH"', body)
        self.assertIn("gh pr create", body)
        self.assertNotIn("git push origin main", body)
        self.assertLess(body.index("Upload immutable observer evidence"), body.index("open the protected promotion PR"))

    def test_policy_executes_only_base_validator_and_authenticates_run_artifact(self) -> None:
        value = workflow(POLICY)
        self.assertEqual(set(value["on"]), {"pull_request"})
        self.assertEqual(value["permissions"], {"actions": "read", "contents": "read", "pull-requests": "read"})
        job = value["jobs"]["policy"]
        self.assertEqual(job["name"], "upstream-promotion-policy")
        body = POLICY.read_text()
        self.assertIn("trusted/scripts/upstream_promotion.py verify-pr", body)
        self.assertNotIn("candidate/scripts/", body)
        self.assertIn("exact observer evidence artifact is unavailable", body)
        self.assertIn("official PR identity changed", body)

    def test_auto_merge_requires_successful_policy_artifact_and_never_admin_merges(self) -> None:
        value = workflow(AUTO_MERGE)
        self.assertEqual(set(value["on"]), {"workflow_run"})
        self.assertEqual(value["permissions"], {"actions": "read", "contents": "write", "pull-requests": "write"})
        body = AUTO_MERGE.read_text()
        self.assertIn("upstream-promotion-verdict-", body)
        self.assertIn('gh pr merge "$PR_NUMBER" --repo "$GITHUB_REPOSITORY" --auto --squash --delete-branch', body)
        self.assertNotIn("--admin", body)


if __name__ == "__main__":
    unittest.main()
