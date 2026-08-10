import json
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[1]
README_PATH = REPO_ROOT / "README.md"
ICON_ROOT = REPO_ROOT / "assets" / "client-icons"
SUPPORT_DOC_PATHS = (
    REPO_ROOT / "docs" / "CLIENTS.md",
    REPO_ROOT / "docs" / "COMPATIBILITY.md",
    REPO_ROOT / "docs" / "TEST_MATRIX.md",
    REPO_ROOT / "docs" / "VERIFICATION.md",
)
CHATGPT_EVIDENCE_PATH = (
    REPO_ROOT
    / "tests"
    / "e2e"
    / "results"
    / "chatgpt-cloudflare-docs-desktop-package-2026-08-10.json"
)


class ReadmeClientTableTests(unittest.TestCase):
    def test_every_supported_client_has_a_sourced_logo(self) -> None:
        readme = README_PATH.read_text()
        expected_icons = {
            "openai.svg",
            "cursor.svg",
            "github-copilot.svg",
            "vscode.svg",
            "kiro.svg",
        }

        self.assertEqual(
            {path.name for path in ICON_ROOT.glob("*.svg")}, expected_icons
        )
        for icon in expected_icons:
            with self.subTest(icon=icon):
                self.assertIn(f'assets/client-icons/{icon}', readme)
                self.assertIn(f"`{icon}`", (ICON_ROOT / "README.md").read_text())

        self.assertEqual(readme.count("assets/client-icons/openai.svg"), 2)

    def test_chatgpt_claim_matches_recorded_evidence_boundary(self) -> None:
        readme = README_PATH.read_text()
        evidence = json.loads(CHATGPT_EVIDENCE_PATH.read_text())
        proved = set(evidence["scope"]["proved"])
        not_proved = set(evidence["scope"]["not_proved"])

        self.assertTrue(
            {
                "repository_marketplace_registration",
                "local_codex_plugin_package_ingestion",
                "official_manager_install",
                "exact_app_id_linkage",
            }.issubset(proved)
        )
        self.assertTrue(
            {
                "chatgpt_work_ui_discovery",
                "chatgpt_work_package_activation",
                "package_routed_runtime",
            }.issubset(not_proved)
        )
        self.assertIn("Repository\nmarketplace ingestion, official manager installation", readme)
        self.assertIn("ChatGPT Work UI discovery, activation,\nand package-routed runtime remain unproved", readme)
        self.assertNotIn(
            "local `.codex-plugin` ingestion and manager\nlifecycle are still separate, unproved steps",
            readme,
        )
        self.assertIn(
            "https://github.com/777genius/universal-agent-plugins/actions/runs/31363316668",
            readme,
        )

        support_docs = "\n".join(path.read_text() for path in SUPPORT_DOC_PATHS)
        self.assertNotIn("31350094295", support_docs)
        self.assertNotIn("repository marketplace ingestion and manager lifecycle are not", support_docs)
        self.assertNotIn("local `.codex-plugin` ingestion remains unproved", support_docs)
        self.assertIn("31363316668", support_docs)
        self.assertIn("package-routed runtime remain unproved", support_docs)


if __name__ == "__main__":
    unittest.main()
