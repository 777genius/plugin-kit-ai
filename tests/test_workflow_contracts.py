import unittest
from pathlib import Path

import yaml


REPO_ROOT = Path(__file__).resolve().parents[1]
WORKFLOW_PATH = REPO_ROOT / ".github/workflows/live-e2e.yml"


def load_workflow() -> dict[str, object]:
    return yaml.load(WORKFLOW_PATH.read_text(), Loader=yaml.BaseLoader)


def job_run_commands(job: dict[str, object]) -> str:
    return "\n".join(
        step.get("run", "")
        for step in job["steps"]
        if isinstance(step, dict)
    )


class WorkflowContractTests(unittest.TestCase):
    def test_release_publish_uses_tested_script(self) -> None:
        workflow = WORKFLOW_PATH.read_text()

        self.assertIn("run: scripts/publish_github_release.sh", workflow)

    def test_agentplugins_version_cannot_be_overridden_at_dispatch(self) -> None:
        workflow = load_workflow()
        inputs = workflow["on"]["workflow_dispatch"]["inputs"]
        workflow_text = WORKFLOW_PATH.read_text()

        self.assertNotIn("agentplugins_version", inputs)
        self.assertNotIn("github.event.inputs.agentplugins_version", workflow_text)

    def test_agentplugins_lifecycle_and_projection_jobs_are_isolated(self) -> None:
        jobs = load_workflow()["jobs"]
        expected = {
            "agentplugins-package-lifecycle": (
                "scripts/run_agentplugins_lifecycle_e2e.py",
                "/tmp/agentplugins-package-lifecycle-e2e.json",
            ),
            "agentplugins-hero-projections": (
                "scripts/run_agentplugins_hero_matrix_e2e.py",
                "/tmp/agentplugins-hero-projections-e2e.json",
            ),
        }
        pinned_uses = {
            "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1",
            "actions/setup-python@5fda3b95a4ea91299a34e894583c3862153e4b97",
            "actions/setup-node@820762786026740c76f36085b0efc47a31fe5020",
            "actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a",
        }

        for name, (runner, evidence) in expected.items():
            with self.subTest(job=name):
                job = jobs[name]
                commands = job_run_commands(job)
                self.assertEqual(job["env"]["AGENTPLUGINS_VERSION"], "0.1.5")
                self.assertIn(
                    'npm install --global "universal-agent-plugins@${AGENTPLUGINS_VERSION}"',
                    commands,
                )
                self.assertIn(runner, commands)
                self.assertIn(
                    '--expected-version "${AGENTPLUGINS_VERSION}"', commands
                )
                self.assertIn(
                    'catalog_digest="sha256:$(sha256sum catalog/v1/catalog.json',
                    commands,
                )
                self.assertIn('--catalog-digest "${catalog_digest}"', commands)
                self.assertIn(evidence, commands)
                self.assertIn("check-jsonschema --schemafile", commands)
                self.assertIn("scripts/validate_client_evidence.py --file", commands)
                uses = {
                    step["uses"]
                    for step in job["steps"]
                    if isinstance(step, dict) and "uses" in step
                }
                self.assertEqual(uses, pinned_uses)

    def test_release_gate_needs_native_lifecycle_and_projection_jobs(self) -> None:
        jobs = load_workflow()["jobs"]
        needs = set(jobs["publish-release"]["needs"])
        agentplugins_jobs = {
            "copilot-native-lifecycle",
            "agentplugins-package-lifecycle",
            "agentplugins-hero-projections",
        }

        self.assertTrue(
            {
                "codex-marketplace-install",
                *agentplugins_jobs,
            }.issubset(needs)
        )
        for name in agentplugins_jobs:
            with self.subTest(job=name):
                self.assertEqual(
                    jobs[name]["env"]["AGENTPLUGINS_VERSION"], "0.1.5"
                )

    def test_copilot_native_job_binds_evidence_to_local_catalog(self) -> None:
        job = load_workflow()["jobs"]["copilot-native-lifecycle"]
        commands = job_run_commands(job)

        self.assertIn(
            'catalog_digest="sha256:$(sha256sum catalog/v1/catalog.json', commands
        )
        self.assertIn('--catalog-digest "${catalog_digest}"', commands)


if __name__ == "__main__":
    unittest.main()
