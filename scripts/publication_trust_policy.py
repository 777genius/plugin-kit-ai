#!/usr/bin/env python3
"""Pure publication trust policy shared by the preparer and signer."""

from __future__ import annotations

from pathlib import Path
from typing import Any

from directory_publication import DIGEST_RE, SHA_RE, read_json, require


CONFIG_FIELDS = {"schema_version", "repository", "snapshot_lifetime_days"}
OPTIONAL_CONFIG_FIELDS = {
    "local_evidence_main_anchor", "trusted_evidence_workflows",
    "trusted_external_evidence",
}
TRUSTED_WORKFLOW_FIELDS = {
    "workflow", "protected_source_ref", "source_digest_policy",
    "allow_self_hosted_runners",
}


def load_publication_trust_config(path: Path) -> dict[str, Any]:
    """Load and strictly validate the code-owned publication trust config."""
    value = read_json(path, max_bytes=64 << 10)
    require(
        isinstance(value, dict)
        and CONFIG_FIELDS <= set(value) <= CONFIG_FIELDS | OPTIONAL_CONFIG_FIELDS,
        f"{path}: invalid publication config fields",
    )
    require(
        type(value["schema_version"]) is int and value["schema_version"] == 1,
        f"{path}: schema_version must be 1",
    )
    require(
        isinstance(value["repository"], str) and value["repository"].count("/") == 1,
        f"{path}: invalid repository",
    )
    require(
        type(value["snapshot_lifetime_days"]) is int
        and 1 <= value["snapshot_lifetime_days"] <= 30,
        f"{path}: invalid lifetime",
    )
    anchor = value.get("local_evidence_main_anchor")
    require(
        anchor is None or isinstance(anchor, str) and SHA_RE.fullmatch(anchor) is not None,
        f"{path}: invalid local evidence main anchor",
    )
    workflows = value.setdefault("trusted_evidence_workflows", [])
    require(isinstance(workflows, list), f"{path}: invalid trusted evidence workflows")
    workflow_names: list[str] = []
    for item in workflows:
        require(
            isinstance(item, dict) and set(item) == TRUSTED_WORKFLOW_FIELDS,
            f"{path}: invalid trusted evidence workflow policy",
        )
        require(
            isinstance(item["workflow"], str)
            and "/.github/workflows/" in item["workflow"]
            and isinstance(item["protected_source_ref"], str)
            and item["protected_source_ref"].startswith("refs/heads/")
            and item["source_digest_policy"]
            in {"artifact_revision", "protected_workflow_source"}
            and type(item["allow_self_hosted_runners"]) is bool,
            f"{path}: invalid trusted evidence workflow policy",
        )
        workflow_names.append(item["workflow"])
    require(
        len(workflow_names) == len(set(workflow_names)),
        f"{path}: duplicate trusted evidence workflow",
    )
    external = value.setdefault("trusted_external_evidence", [])
    require(isinstance(external, list), f"{path}: invalid trusted external evidence")
    for item in external:
        require(
            isinstance(item, dict)
            and set(item) == {"repository", "revision", "path", "digest"},
            f"{path}: invalid trusted external evidence entry",
        )
        require(
            isinstance(item["repository"], str) and item["repository"].count("/") == 1
            and isinstance(item["revision"], str)
            and SHA_RE.fullmatch(item["revision"]) is not None
            and isinstance(item["path"], str) and item["path"]
            and not item["path"].startswith("/") and "\\" not in item["path"]
            and ".." not in Path(item["path"]).parts
            and isinstance(item["digest"], str)
            and DIGEST_RE.fullmatch(item["digest"]) is not None,
            f"{path}: invalid trusted external evidence identity",
        )
    return value


def trusted_workflow_policy(
    evidence: dict[str, Any], config: dict[str, Any], label: str,
) -> dict[str, Any]:
    """Require an exact workflow/repository/ref/digest-policy binding."""
    artifact = evidence["artifact"]
    trust = evidence["trust"]
    workflow = trust["workflow"]
    policy = next(
        (item for item in config["trusted_evidence_workflows"]
         if item["workflow"] == workflow),
        None,
    )
    require(policy is not None, f"{label}: evidence workflow has no reviewed trust policy")
    require(
        workflow.startswith(artifact["repository"] + "/.github/workflows/"),
        f"{label}: workflow and artifact repositories differ",
    )
    require(
        trust["source_ref"] == policy["protected_source_ref"],
        f"{label}: evidence source ref is not trusted",
    )
    # The standard-library wire validator binds source_digest to the immutable
    # artifact revision. Assert it here as policy too so this function remains
    # safe when called independently by prepare.
    require(
        trust["source_digest"] == artifact["revision"],
        f"{label}: evidence source digest is not bound to the exact artifact revision",
    )
    require(
        policy["source_digest_policy"]
        in {"artifact_revision", "protected_workflow_source"},
        f"{label}: evidence source digest policy is invalid",
    )
    return policy


def trusted_external_artifact_policy(
    evidence: dict[str, Any], config: dict[str, Any], label: str,
) -> None:
    """Require the exact code-owned identity for signer-vouched external evidence."""
    require(
        any(
            evidence["artifact"] == allowed
            for allowed in config.get("trusted_external_evidence", [])
        ),
        f"{label}: external evidence artifact is not explicitly trusted",
    )


def validate_publication_eligibility_trust(
    products: list[dict[str, Any]], distributions: list[dict[str, Any]],
    evidence: list[dict[str, Any]], config: dict[str, Any],
) -> None:
    """Enforce signer-visible identity and trust for all candidate evidence."""
    distributions_by_id = {item["id"]: item for item in distributions}
    evidence_by_id = {item["id"]: item for item in evidence}
    require(
        len(distributions_by_id) == len(distributions),
        "duplicate distribution identity",
    )
    require(len(evidence_by_id) == len(evidence), "duplicate evidence identity")

    releases_by_identity: dict[tuple[str, int], dict[str, Any]] = {}
    for distribution in distributions:
        releases = {item["sequence"]: item for item in distribution["releases"]}
        policies = {
            item["release_sequence"]: item
            for item in distribution["release_policies"]
        }
        require(
            len(releases) == len(distribution["releases"])
            and len(policies) == len(distribution["release_policies"]),
            f"{distribution['id']}: duplicate release or policy identity",
        )
        for sequence, release in releases.items():
            releases_by_identity[(distribution["id"], sequence)] = release
        for sequence, policy in policies.items():
            require(sequence in releases, f"{distribution['id']}: policy references missing release {sequence}")
            release = releases[sequence]
            for evidence_id in policy["current_evidence"]:
                require(evidence_id in evidence_by_id, f"{distribution['id']}@{sequence}: missing evidence {evidence_id}")
                record = evidence_by_id[evidence_id]
                require(
                    record["distribution_id"] == distribution["id"]
                    and record["release_sequence"] == sequence
                    and record["package_tree_digest"] == release["tree_digest"],
                    f"{evidence_id}: evidence release/tree identity mismatch",
                )

    # Trust is a property of every byte the signer admits, not merely evidence
    # that is currently installable or selected by a policy.  This also keeps a
    # future policy/status transition from activating evidence the signer never
    # reviewed.
    for record in evidence:
        evidence_id = record["id"]
        identity = (record["distribution_id"], record["release_sequence"])
        require(
            record["distribution_id"] in distributions_by_id,
            f"{evidence_id}: evidence distribution is missing",
        )
        require(identity in releases_by_identity, f"{evidence_id}: evidence release is missing")
        require(
            record["package_tree_digest"] == releases_by_identity[identity]["tree_digest"],
            f"{evidence_id}: evidence release/tree identity mismatch",
        )
        trust = record.get("trust")
        require(isinstance(trust, dict), f"{evidence_id}: evidence trust is missing")
        if trust.get("kind") == "github_actions":
            trusted_workflow_policy(record, config, evidence_id)
        elif trust.get("kind") == "reviewed_external":
            trusted_external_artifact_policy(record, config, evidence_id)
        else:
            require(False, f"{evidence_id}: evidence trust kind is not supported")

    for product in products:
        require(
            product["default_distribution"] in distributions_by_id,
            f"{product['id']}: default distribution is missing",
        )
        distribution = distributions_by_id[product["default_distribution"]]
        if distribution["kind"] != "upstream" or distribution["status"] != "active":
            continue
        policies = {
            item["release_sequence"]: item
            for item in distribution["release_policies"]
        }
        required = {
            component for component, state in product["minimum_capabilities"].items()
            if state == "required"
        }
        eligible = [
            release for release in distribution["releases"]
            if policies[release["sequence"]]["status"] == "active"
            and required.issubset(release["components"])
        ]
        # Safety publications may deliberately leave no eligible release.
        if not eligible:
            continue
        release = max(eligible, key=lambda item: item["sequence"])
        policy = policies[release["sequence"]]
        passed: set[str] = set()
        for evidence_id in policy["current_evidence"]:
            observation = evidence_by_id[evidence_id]
            if (
                observation["distribution_id"] == distribution["id"]
                and observation["release_sequence"] == release["sequence"]
                and observation["package_tree_digest"] == release["tree_digest"]
                and observation.get("level") == "materialization"
                and observation.get("outcome") == "passed"
                and observation.get("trust", {}).get("kind") == "github_actions"
            ):
                trusted_workflow_policy(observation, config, evidence_id)
                client = observation.get("client")
                if isinstance(client, str):
                    passed.add(client)
        missing = sorted(
            target["client"] for target in policy["targets"]
            if target["client"] not in passed
        )
        require(
            not missing,
            f"{product['id']}: upstream default {distribution['id']}@{release['sequence']} "
            f"lacks exact passed materialization evidence for {','.join(missing)}",
        )
