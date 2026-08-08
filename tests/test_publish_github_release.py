import os
import subprocess
import tempfile
import textwrap
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[1]
PUBLISH_SCRIPT = REPO_ROOT / "scripts/publish_github_release.sh"


class PublishGithubReleaseTests(unittest.TestCase):
    def run_script(self, lookup: str, lookup_exit: int = 0) -> tuple[subprocess.CompletedProcess[str], str]:
        with tempfile.TemporaryDirectory() as temp_dir:
            temp_path = Path(temp_dir)
            log_path = temp_path / "gh.log"
            mock_gh = temp_path / "gh"
            mock_gh.write_text(
                textwrap.dedent(
                    """\
                    #!/usr/bin/env bash
                    set -eu
                    printf '%s\\n' "$*" >>"$MOCK_GH_LOG"
                    if [[ "$1 $2" == "api graphql" ]]; then
                      printf '%s\\n' "$MOCK_GH_LOOKUP"
                      exit "$MOCK_GH_LOOKUP_EXIT"
                    fi
                    if [[ "$1 $2" == "release create" ]]; then
                      exit 0
                    fi
                    exit 2
                    """
                )
            )
            mock_gh.chmod(0o755)
            env = os.environ | {
                "GITHUB_REPOSITORY": "777genius/universal-agent-plugins",
                "GITHUB_REPOSITORY_OWNER": "777genius",
                "GITHUB_REF_NAME": "v0.1.4",
                "MOCK_GH_LOG": str(log_path),
                "MOCK_GH_LOOKUP": lookup,
                "MOCK_GH_LOOKUP_EXIT": str(lookup_exit),
                "PATH": f"{temp_path}{os.pathsep}{os.environ['PATH']}",
            }
            result = subprocess.run(
                [str(PUBLISH_SCRIPT)],
                env=env,
                capture_output=True,
                text=True,
                check=False,
            )
            return result, log_path.read_text()

    def test_existing_published_release_is_success_without_create(self) -> None:
        result, calls = self.run_script("v0.1.4\tfalse")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("release create", calls)

    def test_missing_release_is_created(self) -> None:
        result, calls = self.run_script("missing")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("release create v0.1.4", calls)

    def test_draft_release_fails_without_create(self) -> None:
        result, calls = self.run_script("v0.1.4\ttrue")

        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("release create", calls)

    def test_lookup_error_fails_without_create(self) -> None:
        result, calls = self.run_script("", lookup_exit=1)

        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("release create", calls)


if __name__ == "__main__":
    unittest.main()
