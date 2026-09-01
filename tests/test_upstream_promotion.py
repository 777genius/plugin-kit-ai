import argparse
import copy
import json
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from scripts import upstream_promotion as promotion
from scripts import run_upstream_promotion_materialization as lifecycle
from scripts.build_registry import RegistryError, validated_package_facts
from scripts.validate_review_journey import materialize


ROOT = Path(__file__).resolve().parents[1]
FAKE_DIGEST = "sha256:" + "1" * 64
MANIFEST_DIGEST = "sha256:" + "2" * 64
REVIEWED_SHA = "a" * 40
MERGE_SHA = "b" * 40


def run_git(repository: Path, *args: str) -> str:
    environment = {
        **os.environ, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull,
        "GIT_AUTHOR_NAME": "test", "GIT_AUTHOR_EMAIL": "test@example.com",
        "GIT_COMMITTER_NAME": "test", "GIT_COMMITTER_EMAIL": "test@example.com",
    }
    return subprocess.run(
        ["git", *args], cwd=repository, env=environment, check=True,
        text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
    ).stdout.strip()


def evidence(artifact_revision: str, artifact_path: str, artifact_digest: str) -> dict:
    return {
        "schema_version": 1,
        "id": "promotion/github/bbbbbbbbbbbb/codex",
        "product_id": "github", "distribution_id": "github/github", "release_sequence": 1,
        "package_tree_digest": FAKE_DIGEST, "manifest_digest": MANIFEST_DIGEST,
        "source_repository": "github/github-mcp-server", "source_revision": MERGE_SHA,
        "source_path": "agent-plugin", "level": "materialization", "outcome": "passed",
        "client": "codex", "client_version": "isolated-configuration-fixture",
        "installer_version": "0.1.26", "adapter_version": "agentplugins-0.1.26",
        "os": "linux", "architecture": "amd64", "dependency_identity": "agentplugins@0.1.26",
        "observed_at": "2026-09-01T00:00:00Z",
        "artifact": {
            "repository": "777genius/universal-agent-plugins", "revision": artifact_revision,
            "path": artifact_path, "digest": artifact_digest,
        },
        "trust": {
            "kind": "github_actions", "workflow": promotion.WORKFLOW,
            "source_ref": "refs/heads/main", "source_digest": "c" * 40,
        },
    }


def review_record(item: dict) -> dict:
    return {
        "schema_version": 3, "repository": "github/github-mcp-server", "path": "agent-plugin",
        "reviewed_revision": REVIEWED_SHA, "reviewed_tree_digest": FAKE_DIGEST,
        "reviewed_manifest_digest": MANIFEST_DIGEST, "product_id": "github", "manifest_name": "github",
        "distribution_id": "github/github", "release_sequence": 1,
        "policy": {
            "status": "active", "minimum_installer_version": "0.1.26",
            "targets": [{"client": "codex", "scopes": ["user"], "delivery": "managed", "authentication": "required"}],
        },
        "evidence": [item],
    }


def candidate(record: dict) -> dict:
    item = record["evidence"][0]
    return {
        "schema_version": 1, "decision": "reviewable_promotion_candidate",
        "product": {"id": "github", "manifest_name": "github"},
        "distribution": {"id": "github/github", "kind": "upstream"},
        "release": {
            "sequence": 1, "package_version": "", "agent_plugins_schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
            "components": ["mcp"], "tree_digest_algorithm": "agentplugins-tree-sha256-v1",
            "tree_digest": FAKE_DIGEST, "manifest_digest": MANIFEST_DIGEST,
        },
        "policy": copy.deepcopy(record["policy"]),
        "source": {
            "repository": "github/github-mcp-server", "path": "agent-plugin", "upstream_pr_number": 3169,
            "upstream_pr_url": "https://github.com/github/github-mcp-server/pull/3169",
            "merged_at": "2026-09-01T00:00:00Z", "reviewed_pr_head_sha": REVIEWED_SHA,
            "official_candidate_sha": MERGE_SHA, "official_default_ref": "refs/remotes/origin/main",
            "official_default_tip_sha": MERGE_SHA, "byte_classification": "exact",
        },
        "evidence": [{
            "id": item["id"], "record_digest": promotion.sha256(promotion.canonical(item)),
            "level": "materialization", "client": "codex", "installer_version": "0.1.26",
            "artifact": copy.deepcopy(item["artifact"]),
        }],
        "gate_artifacts": [{"name": name, "artifact_digest": FAKE_DIGEST} for name in (
            "pr_metadata", "repository_identity", "default_history", "reviewed_identity",
            "candidate_identity", "package", "policy", "evidence",
        )],
    }


class UpstreamPromotionTests(unittest.TestCase):
    def test_real_watch_list_is_schema_valid_sorted_and_accepts_root_package(self) -> None:
        watch = promotion.validate_watch(ROOT / "registry/upstream-promotions.json")
        self.assertEqual([item["product_id"] for item in watch["entries"]], ["chrome-devtools", "cloudflare-docs", "github"])
        self.assertEqual(watch["entries"][0]["package_path"], ".")

    def test_materialize_supports_an_exact_repository_root_without_path_escape(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            repository = Path(temporary) / "repository"
            output = Path(temporary) / "output"
            repository.mkdir()
            run_git(repository, "init", "-q")
            (repository / "plugin.json").write_text("{}\n")
            (repository / "mcp.json").write_text("{}\n")
            (repository / "nested").mkdir()
            (repository / "nested/file.txt").write_text("ok\n")
            run_git(repository, "add", ".")
            run_git(repository, "commit", "-qm", "fixture")
            revision = run_git(repository, "rev-parse", "HEAD")
            output.mkdir()
            materialize(repository, revision, ".", output)
            self.assertEqual(sorted(path.relative_to(output).as_posix() for path in output.rglob("*") if path.is_file()), ["mcp.json", "nested/file.txt", "plugin.json"])

    def test_external_root_package_does_not_depend_on_checkout_directory_name(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "checkout"
            shutil.copytree(ROOT / "plugins/context7", root)
            with self.assertRaisesRegex(RegistryError, "name must match package directory"):
                validated_package_facts(root)
            facts = validated_package_facts(root, require_directory_name=False)
            self.assertEqual(facts["manifest_name"], "context7")

    def test_materialization_runner_uses_only_disposable_client_homes_and_cleans_up(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            cli = root / "agentplugins"
            cli.write_text(
                "#!/usr/bin/env python3\n"
                "import json,sys\n"
                "cmd=sys.argv[1]; dry='--dry-run' in sys.argv\n"
                "clients=['codex','cursor','kiro']\n"
                "targets=[{'target':x,'status':'external_completed'} for x in clients]\n"
                "data={}\n"
                "if cmd=='add' and dry: data={'revision':'b'*40}\n"
                "elif cmd=='add': data={'status':'completed','succeeded':3,'failed':0,'targets':targets,'plugin':'github','revision':'b'*40,'tree_digest':'sha256:'+'1'*64,'manifest_digest':'sha256:'+'2'*64,'version':'','target_outcomes':{x:{'outcome':'passed'} for x in clients}}\n"
                "elif cmd=='info': data={'name':'github'}\n"
                "elif cmd=='doctor': data={'healthy':True}\n"
                "elif cmd=='remove': data={'status':'completed','succeeded':3,'failed':0,'targets':targets,'plugin_data_preserved':False,'data_retained':False}\n"
                "elif cmd=='list': data={'installations':[]}\n"
                "print(json.dumps({'schema_version':1,'command':cmd,'result':'success','data':data}))\n"
            )
            cli.chmod(0o755)
            sandbox = root / "sandbox"
            args = argparse.Namespace(
                cli=cli, installer_version="0.1.26", product_id="github",
                repository="github/github-mcp-server", revision=MERGE_SHA,
                path="agent-plugin", targets="codex,cursor,kiro", sandbox=sandbox,
                run_repository="777genius/universal-agent-plugins", run_id="1",
                run_attempt="1", source_sha="c" * 40, keep_sandbox=False,
            )
            result = lifecycle.run(args)
            self.assertEqual(result["outcome"], "passed")
            self.assertEqual(result["clients"], ["codex", "cursor", "kiro"])
            self.assertFalse(sandbox.exists())

    def test_select_refuses_a_merged_pr_when_reviewed_head_changed(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            watch = promotion.validate_watch(ROOT / "registry/upstream-promotions.json")
            watch["entries"] = [watch["entries"][0]]
            watch_path = root / "watch.json"
            watch_path.write_bytes(promotion.pretty(watch))
            gh = root / "gh"
            gh.write_text(
                "#!/usr/bin/env python3\n"
                "import json,sys\n"
                "endpoint=sys.argv[-1]\n"
                "if endpoint.endswith('pulls?state=open&per_page=100'):\n"
                " print('[]')\n"
                "elif endpoint.endswith('/pulls/2623'):\n"
                " print(json.dumps({'number':2623,'state':'closed','draft':False,'merged_at':'2026-09-01T00:00:00Z','merge_commit_sha':'b'*40,'html_url':'https://github.com/ChromeDevTools/chrome-devtools-mcp/pull/2623','head':{'sha':'d'*40},'base':{'ref':'main'}}))\n"
                "else:\n"
                " print(json.dumps({'default_branch':'main'}))\n"
            )
            gh.chmod(0o755)
            args = argparse.Namespace(watch=watch_path, directory=ROOT / "registry/directory.json", gh=gh)
            with mock.patch.dict(os.environ, {"GH_TOKEN": "fixture"}):
                result = promotion.select(args)
            self.assertEqual(result["decision"], "none")
            self.assertEqual(result["diagnostics"][0]["outcome"], "reviewed_head_changed")

    def test_apply_and_verify_exact_two_commit_promotion(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            repository = Path(temporary) / "repository"
            repository.mkdir()
            run_git(repository, "init", "-q")
            (repository / "registry").mkdir()
            for name in ("directory.json", "review-preview.json", "review-search.json"):
                shutil.copy2(ROOT / "registry" / name, repository / "registry" / name)
            run_git(repository, "add", ".")
            run_git(repository, "commit", "-qm", "base")
            base = run_git(repository, "rev-parse", "HEAD")

            raw_path = f"evidence/upstream-promotions/github/{MERGE_SHA}/materialization.json"
            raw_file = repository / raw_path
            raw_file.parent.mkdir(parents=True)
            raw = {
                "schema_version": 1, "outcome": "passed", "product_id": "github",
                "repository": "github/github-mcp-server", "revision": MERGE_SHA, "path": "agent-plugin",
                "clients": ["codex"], "installer_version": "0.1.26",
                "package": {"package_version": "", "tree_digest": FAKE_DIGEST, "manifest_digest": MANIFEST_DIGEST},
                "operations": {"add": "passed"}, "observed_at": "2026-09-01T00:00:00Z",
                "run": {"repository": "777genius/universal-agent-plugins", "id": "1", "attempt": "1", "source_sha": "c" * 40},
                "sandbox": {"kind": "disposable", "real_user_project_used": False, "removed": True},
            }
            raw_file.write_bytes(promotion.pretty(raw))
            run_git(repository, "add", raw_path)
            run_git(repository, "commit", "-qm", "evidence")
            evidence_commit = run_git(repository, "rev-parse", "HEAD")

            item = evidence(evidence_commit, raw_path, promotion.sha256(raw_file.read_bytes()))
            review = review_record(item)
            proposed = candidate(review)
            audit = repository / f"registry/upstream-promotion-audit/github/{MERGE_SHA}"
            audit.mkdir(parents=True)
            review_path, candidate_path = audit / "review-record.json", audit / "promotion-candidate.json"
            review_path.write_bytes(promotion.pretty(review))
            candidate_path.write_bytes(promotion.pretty(proposed))
            promotion.apply_candidate(argparse.Namespace(candidate=candidate_path, review_record=review_path, directory=repository / "registry/directory.json"))
            source = promotion.read_object(repository / "registry/directory.json")
            (repository / "registry/review-preview.json").write_bytes(promotion.encoded(promotion.directory_preview(source)))
            (repository / "registry/review-search.json").write_bytes(promotion.encoded(promotion.directory_search(source)))
            run_git(repository, "add", ".")
            run_git(repository, "commit", "-qm", "promotion")
            head = run_git(repository, "rev-parse", "HEAD")

            result = promotion.verify_pr(argparse.Namespace(
                repository=repository, base_sha=base, head_sha=head,
                branch="automation/upstream-promotion-github-bbbbbbbbbbbb",
            ))
            self.assertEqual(result["outcome"], "verified")
            self.assertEqual(result["observer_run_id"], "1")

            review["evidence"][0]["installer_version"] = "0.1.25"
            review_path.write_bytes(promotion.pretty(review))
            with self.assertRaisesRegex(promotion.PromotionError, "candidate evidence projection"):
                promotion.apply_candidate(argparse.Namespace(candidate=candidate_path, review_record=review_path, directory=repository / "registry/directory.json"))


if __name__ == "__main__":
    unittest.main()
