#!/usr/bin/env python3
"""Create and publish the Directory's deterministic same-tree CAS marker."""

from __future__ import annotations

import argparse
import hashlib
import os
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Mapping, Sequence


GIT = "/usr/bin/git"
SHA_RE = re.compile(r"[0-9a-f]{40}")
PUBLICATION_ID_RE = re.compile(r"[A-Za-z0-9][A-Za-z0-9._-]{0,127}")
MARKER_NAME = "uap-directory-publisher[bot]"
MARKER_EMAIL = "uap-directory-publisher[bot]@users.noreply.github.com"
MARKER_EPOCH = 946684800  # 2000-01-01; the bounded hash offset is part of v1.
MARKER_SPAN = 100 * 366 * 24 * 60 * 60


class CasError(RuntimeError):
    """The requested publication does not match an allowed exact ref state."""


def _git(repo: Path, arguments: Sequence[str], *, input_text: str | None = None,
         check: bool = True, env: Mapping[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [GIT, "-C", str(repo), *arguments], input=input_text, text=True,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=check, env=env,
    )


def _require_sha(value: str, label: str) -> None:
    if SHA_RE.fullmatch(value) is None:
        raise CasError(f"{label} must be a full lowercase object ID")


def marker_timestamp(publication_id: str) -> int:
    if PUBLICATION_ID_RE.fullmatch(publication_id) is None:
        raise CasError("publication ID is invalid")
    digest = hashlib.sha256(("uap-directory-publication-marker-v1\0" + publication_id).encode("ascii")).digest()
    return MARKER_EPOCH + int.from_bytes(digest[:8], "big") % MARKER_SPAN


def marker_message(source: str, publication_id: str) -> str:
    _require_sha(source, "source commit")
    if PUBLICATION_ID_RE.fullmatch(publication_id) is None:
        raise CasError("publication ID is invalid")
    return (
        "chore(directory): record publication marker\n\n"
        "Directory-Publication-Marker: 1\n"
        f"Publication-ID: {publication_id}\n"
        f"Source-Commit: {source}\n"
    )


def create_marker(repo: Path, source: str, publication_id: str) -> str:
    """Write deterministic marker P: one parent, source tree, no changed paths."""
    _require_sha(source, "source commit")
    source_type = _git(repo, ["cat-file", "-t", source]).stdout.strip()
    if source_type != "commit":
        raise CasError("source object is not a commit")
    tree = _git(repo, ["show", "-s", "--format=%T", source]).stdout.strip()
    timestamp = marker_timestamp(publication_id)
    identity_env = dict(os.environ)
    identity_env.update({
        "GIT_AUTHOR_NAME": MARKER_NAME,
        "GIT_AUTHOR_EMAIL": MARKER_EMAIL,
        "GIT_AUTHOR_DATE": f"@{timestamp} +0000",
        "GIT_COMMITTER_NAME": MARKER_NAME,
        "GIT_COMMITTER_EMAIL": MARKER_EMAIL,
        "GIT_COMMITTER_DATE": f"@{timestamp} +0000",
    })
    marker = _git(
        repo, ["commit-tree", tree, "-p", source],
        input_text=marker_message(source, publication_id), env=identity_env,
    ).stdout.strip()
    validate_marker(repo, marker, source, publication_id)
    return marker


def validate_marker(repo: Path, marker: str, source: str, publication_id: str) -> None:
    _require_sha(marker, "marker commit")
    _require_sha(source, "source commit")
    if marker == source:
        raise CasError("marker commit must differ from its source")
    parents = _git(repo, ["show", "-s", "--format=%P", marker]).stdout.strip().split()
    if parents != [source]:
        raise CasError("marker commit does not have exactly the source parent")
    marker_tree = _git(repo, ["show", "-s", "--format=%T", marker]).stdout.strip()
    source_tree = _git(repo, ["show", "-s", "--format=%T", source]).stdout.strip()
    if marker_tree != source_tree:
        raise CasError("marker commit tree differs from source tree")
    if _git(repo, ["diff", "--quiet", source, marker], check=False).returncode != 0:
        raise CasError("marker commit changes paths")
    raw_message = _git(repo, ["show", "-s", "--format=%B", marker]).stdout
    if raw_message != marker_message(source, publication_id) + "\n":
        raise CasError("marker commit message differs from deterministic contract")
    timestamp = str(marker_timestamp(publication_id))
    expected_body = (
        f"tree {source_tree}\n"
        f"parent {source}\n"
        f"author {MARKER_NAME} <{MARKER_EMAIL}> {timestamp} +0000\n"
        f"committer {MARKER_NAME} <{MARKER_EMAIL}> {timestamp} +0000\n"
        "\n"
        + marker_message(source, publication_id)
    )
    expected_oid = _git(repo, ["hash-object", "-t", "commit", "--stdin"], input_text=expected_body).stdout.strip()
    if marker != expected_oid:
        raise CasError("marker commit object differs from deterministic contract")
    metadata = _git(repo, ["show", "-s", "--format=%an%n%ae%n%at%n%cn%n%ce%n%ct", marker]).stdout.splitlines()
    if metadata != [MARKER_NAME, MARKER_EMAIL, timestamp, MARKER_NAME, MARKER_EMAIL, timestamp]:
        raise CasError("marker commit identity or timestamp differs from deterministic contract")


@dataclass(frozen=True)
class RefState:
    main: str | None
    ledger: str | None
    sequence_tag: str | None


def read_ref_state(repo: Path, remote: str, main_ref: str, ledger_ref: str, tag_ref: str) -> RefState:
    completed = _git(repo, ["ls-remote", "--refs", remote, main_ref, ledger_ref, tag_ref])
    observed: dict[str, str] = {}
    for line in completed.stdout.splitlines():
        fields = line.split("\t", 1)
        if len(fields) != 2:
            raise CasError("remote returned a malformed ref line")
        oid, ref = fields
        if ref in observed or SHA_RE.fullmatch(oid) is None:
            raise CasError("remote returned an invalid or duplicate ref")
        observed[ref] = oid
    return RefState(observed.get(main_ref), observed.get(ledger_ref), observed.get(tag_ref))


def validate_materialized_descendant(repo: Path, materialized: str, signed: str) -> None:
    """Authenticate the one allowed post-publication ledger descendant.

    Site materialization is deliberately a separate, single-parent commit.  It
    may change the Pages tree, but it cannot change any signed registry byte.
    This lets an exact workflow rerun distinguish its own completed deployment
    transaction from arbitrary forward movement of the protected ledger.
    """
    _require_sha(materialized, "materialized ledger")
    _require_sha(signed, "signed ledger")
    if materialized == signed:
        raise CasError("materialized ledger must differ from the signed ledger")
    parents = _git(repo, ["show", "-s", "--format=%P", materialized]).stdout.strip().split()
    if parents != [signed]:
        raise CasError("materialized ledger is not the exact signed-commit child")
    if _git(repo, ["diff", "--quiet", signed, materialized, "--", "registry"], check=False).returncode != 0:
        raise CasError("materialized ledger changed signed registry bytes")
    message = _git(repo, ["show", "-s", "--format=%B", materialized]).stdout
    if message != "chore(directory): materialize signed production site\n\n":
        raise CasError("materialized ledger commit message is invalid")


def validate_staged_lineage(repo: Path, current: str, signed: str) -> str:
    """Return the exact site materialization below safe Discovery-only appends."""
    _require_sha(current, "current ledger")
    _require_sha(signed, "signed ledger")
    if _git(repo, ["merge-base", "--is-ancestor", signed, current], check=False).returncode != 0:
        raise CasError("signed ledger is not an ancestor of the current ledger")
    descendants = _git(
        repo, ["rev-list", "--reverse", "--ancestry-path", f"{signed}..{current}"],
    ).stdout.splitlines()
    if not descendants:
        raise CasError("staged publication has no materialized site commit")
    materialized = descendants[0]
    validate_materialized_descendant(repo, materialized, signed)
    previous = materialized
    for descendant in descendants[1:]:
        parents = _git(repo, ["show", "-s", "--format=%P", descendant]).stdout.strip().split()
        if parents != [previous]:
            raise CasError("staged ledger has a non-linear post-materialization append")
        changed = _git(
            repo, ["diff-tree", "--no-commit-id", "--name-only", "-r", previous, descendant],
        ).stdout.splitlines()
        if not changed or any(not path.startswith("discovery/") for path in changed):
            raise CasError("staged ledger has a non-Discovery post-materialization append")
        previous = descendant
    if _git(repo, ["diff", "--quiet", signed, current, "--", "registry"], check=False).returncode != 0:
        raise CasError("staged ledger changed signed registry bytes")
    return materialized


def atomic_transition(
    repo: Path, remote: str, *, source: str, marker: str, ledger_old: str,
    ledger_new: str, sequence_tag: str, attempts: int = 3,
    push_runner: Callable[[Sequence[str]], bool] | None = None,
    materialized_output: Path | None = None,
) -> str:
    """Publish only exact pre-state, accept only exact committed state, else fail."""
    for value, label in ((source, "source"), (marker, "marker"), (ledger_old, "old ledger"), (ledger_new, "new ledger")):
        _require_sha(value, label)
    if not sequence_tag.startswith("refs/tags/directory-publication-schema-1-sequence-"):
        raise CasError("sequence tag is outside the publication namespace")
    if not 1 <= attempts <= 3:
        raise CasError("attempt count must be between one and three")
    main_ref = "refs/heads/main"
    ledger_ref = "refs/heads/directory-publication-ledger"
    before = RefState(source, ledger_old, None)
    committed = RefState(marker, ledger_new, ledger_new)
    if materialized_output is not None:
        materialized_output.unlink(missing_ok=True)

    def accept(state: RefState) -> str | None:
        if state == committed:
            return "committed"
        if state.main == marker and state.sequence_tag == ledger_new and state.ledger is not None:
            # ls-remote proves the ref identity, then fetch and inspect that
            # exact immutable object before treating it as our rerun state.
            fetched = _git(repo, ["fetch", "--no-tags", remote, state.ledger], check=False)
            if fetched.returncode != 0:
                raise CasError("cannot acquire materialized ledger descendant")
            validate_materialized_descendant(repo, state.ledger, ledger_new)
            if materialized_output is not None:
                materialized_output.write_text(state.ledger + "\n", encoding="ascii")
            return "materialized"
        return None

    def push(arguments: Sequence[str]) -> bool:
        if push_runner is not None:
            return push_runner(arguments)
        return _git(repo, list(arguments), check=False).returncode == 0

    arguments = [
        "-c", "core.hooksPath=/dev/null", "push", "--atomic",
        f"--force-with-lease={main_ref}:{source}",
        f"--force-with-lease={ledger_ref}:{ledger_old}",
        f"--force-with-lease={sequence_tag}:",
        remote,
        f"{marker}:{main_ref}", f"{ledger_new}:{ledger_ref}", f"{ledger_new}:{sequence_tag}",
    ]
    for _attempt in range(attempts):
        try:
            state = read_ref_state(repo, remote, main_ref, ledger_ref, sequence_tag)
        except subprocess.CalledProcessError:
            continue
        accepted = accept(state)
        if accepted is not None:
            return accepted
        if state != before:
            raise CasError(f"publication ref conflict: observed {state}")
        push(arguments)
        # Always perform exact multi-ref readback.  This also resolves a lost
        # receive-pack response without regenerating any object or sequence.
        try:
            state = read_ref_state(repo, remote, main_ref, ledger_ref, sequence_tag)
        except subprocess.CalledProcessError:
            continue
        if state == committed:
            return "published"
        accepted = accept(state)
        if accepted is not None:
            return accepted
        if state != before:
            raise CasError(f"publication ref conflict after push: observed {state}")
    raise CasError("publication push failed with exact pre-state still present")


def materialize_transition(
    repo: Path, remote: str, *, ledger_old: str, ledger_new: str, attempts: int = 3,
    push_runner: Callable[[Sequence[str]], bool] | None = None,
) -> str:
    """Advance the ledger with an exact lease, accepting only exact readback."""
    _require_sha(ledger_old, "old ledger")
    _require_sha(ledger_new, "new ledger")
    if not 1 <= attempts <= 3:
        raise CasError("attempt count must be between one and three")
    ledger_ref = "refs/heads/directory-publication-ledger"

    def read() -> str | None:
        return read_ref_state(repo, remote, "refs/heads/__unused-main", ledger_ref, "refs/tags/__unused-tag").ledger

    arguments = [
        "-c", "core.hooksPath=/dev/null", "push",
        f"--force-with-lease={ledger_ref}:{ledger_old}", remote,
        f"{ledger_new}:{ledger_ref}",
    ]
    for _attempt in range(attempts):
        observed = read()
        if observed == ledger_new:
            return "committed"
        if observed != ledger_old:
            raise CasError(f"materialization ledger conflict: observed {observed}")
        succeeded = push_runner(arguments) if push_runner is not None else _git(repo, arguments, check=False).returncode == 0
        del succeeded  # Exact readback, not the transport response, is authoritative.
        observed = read()
        if observed == ledger_new:
            return "published"
        if observed != ledger_old:
            raise CasError(f"materialization ledger conflict after push: observed {observed}")
    raise CasError("materialization push failed with exact pre-state still present")


def evidence_transition(
    repo: Path, remote: str, *, main_old: str, main_new: str,
    ledger_old: str, ledger_new: str, approval_target: str, approval_tag: str,
    attempts: int = 3,
    push_runner: Callable[[Sequence[str]], bool] | None = None,
) -> str:
    """Atomically append evidence, select it on main, and approve the gated ledger.

    The approval tag deliberately targets ``approval_target``: that is the
    exact staged publication whose protected live gate produced the evidence.
    Discovery-only commits may already follow it before the permanent evidence
    child is appended to ``ledger_old``.
    """
    for value, label in (
        (main_old, "old main"), (main_new, "new main"),
        (ledger_old, "old ledger"), (ledger_new, "new ledger"),
        (approval_target, "approval target"),
    ):
        _require_sha(value, label)
    if approval_tag != "refs/tags/directory-publication-schema-1-launch-approved":
        raise CasError("launch approval tag is outside the fixed namespace")
    if not 1 <= attempts <= 3:
        raise CasError("attempt count must be between one and three")
    if _git(repo, ["merge-base", "--is-ancestor", approval_target, ledger_old], check=False).returncode != 0:
        raise CasError("approval target is not an ancestor of the evidence parent")
    if _git(repo, ["show", "-s", "--format=%P", ledger_new]).stdout.strip().split() != [ledger_old]:
        raise CasError("evidence ledger commit is not the exact parent child")
    if _git(repo, ["show", "-s", "--format=%P", main_new]).stdout.strip().split() != [main_old]:
        raise CasError("evidence main commit is not the exact parent child")
    main_ref = "refs/heads/main"
    ledger_ref = "refs/heads/directory-publication-ledger"
    before = RefState(main_old, ledger_old, None)
    committed = RefState(main_new, ledger_new, approval_target)
    arguments = [
        "-c", "core.hooksPath=/dev/null", "push", "--atomic",
        f"--force-with-lease={main_ref}:{main_old}",
        f"--force-with-lease={ledger_ref}:{ledger_old}",
        f"--force-with-lease={approval_tag}:",
        remote,
        f"{main_new}:{main_ref}", f"{ledger_new}:{ledger_ref}",
        f"{approval_target}:{approval_tag}",
    ]
    for _attempt in range(attempts):
        try:
            state = read_ref_state(repo, remote, main_ref, ledger_ref, approval_tag)
        except subprocess.CalledProcessError:
            continue
        if state == committed:
            return "committed"
        if state != before:
            raise CasError(f"evidence publication ref conflict: observed {state}")
        if push_runner is not None:
            push_runner(arguments)
        else:
            _git(repo, arguments, check=False)
        # The transport response is never authoritative. Resolve success,
        # conflict, or safe retry from one exact three-ref readback.
        try:
            state = read_ref_state(repo, remote, main_ref, ledger_ref, approval_tag)
        except subprocess.CalledProcessError:
            continue
        if state == committed:
            return "published"
        if state != before:
            raise CasError(f"evidence publication ref conflict after push: observed {state}")
    raise CasError("evidence publication push failed with exact pre-state still present")


def production_transition(
    repo: Path, remote: str, *, production_new: str, production_tag: str,
    attempts: int = 3, push_runner: Callable[[Sequence[str]], bool] | None = None,
) -> str:
    """Select the exact tree deployed to Pages using a monotonic CAS tag."""
    _require_sha(production_new, "new production commit")
    if production_tag != "refs/tags/directory-publication-schema-1-production":
        raise CasError("production tag is outside the fixed namespace")
    if not 1 <= attempts <= 3:
        raise CasError("attempt count must be between one and three")
    if _git(repo, ["cat-file", "-t", production_new]).stdout.strip() != "commit":
        raise CasError("new production object is not a commit")

    def read() -> str | None:
        return read_ref_state(
            repo, remote, "refs/heads/__unused-main",
            "refs/heads/__unused-ledger", production_tag,
        ).sequence_tag

    for _attempt in range(attempts):
        observed = read()
        if observed == production_new:
            return "committed"
        if observed is not None:
            fetched = _git(repo, ["fetch", "--no-tags", remote, observed], check=False)
            if fetched.returncode != 0:
                continue
            if _git(repo, ["merge-base", "--is-ancestor", observed, production_new], check=False).returncode != 0:
                raise CasError("production tag update would roll back or change ledger lineage")
        lease = f"--force-with-lease={production_tag}:{observed or ''}"
        arguments = [
            "-c", "core.hooksPath=/dev/null", "push", lease, remote,
            f"{production_new}:{production_tag}",
        ]
        if push_runner is not None:
            push_runner(arguments)
        else:
            _git(repo, arguments, check=False)
        current = read()
        if current == production_new:
            return "published"
        if current != observed:
            raise CasError(f"production tag conflict after push: observed {current}")
    raise CasError("production tag push failed with exact pre-state still present")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)
    marker_parser = subparsers.add_parser("marker")
    marker_parser.add_argument("--repo", type=Path, default=Path.cwd())
    marker_parser.add_argument("--source", required=True)
    marker_parser.add_argument("--publication-id", required=True)
    marker_parser.add_argument("--output", type=Path)
    publish_parser = subparsers.add_parser("publish")
    publish_parser.add_argument("--repo", type=Path, default=Path.cwd())
    publish_parser.add_argument("--remote", default="origin")
    publish_parser.add_argument("--source", required=True)
    publish_parser.add_argument("--marker", required=True)
    publish_parser.add_argument("--ledger-old", required=True)
    publish_parser.add_argument("--ledger-new", required=True)
    publish_parser.add_argument("--sequence-tag", required=True)
    publish_parser.add_argument("--materialized-output", type=Path)
    materialize_parser = subparsers.add_parser("materialize")
    materialize_parser.add_argument("--repo", type=Path, default=Path.cwd())
    materialize_parser.add_argument("--remote", default="origin")
    materialize_parser.add_argument("--ledger-old", required=True)
    materialize_parser.add_argument("--ledger-new", required=True)
    evidence_parser = subparsers.add_parser("evidence-publish")
    evidence_parser.add_argument("--repo", type=Path, default=Path.cwd())
    evidence_parser.add_argument("--remote", default="origin")
    evidence_parser.add_argument("--main-old", required=True)
    evidence_parser.add_argument("--main-new", required=True)
    evidence_parser.add_argument("--ledger-old", required=True)
    evidence_parser.add_argument("--ledger-new", required=True)
    evidence_parser.add_argument("--approval-target", required=True)
    evidence_parser.add_argument(
        "--approval-tag",
        default="refs/tags/directory-publication-schema-1-launch-approved",
    )
    production_parser = subparsers.add_parser("production-publish")
    production_parser.add_argument("--repo", type=Path, default=Path.cwd())
    production_parser.add_argument("--remote", default="origin")
    production_parser.add_argument("--production-new", required=True)
    production_parser.add_argument(
        "--production-tag",
        default="refs/tags/directory-publication-schema-1-production",
    )
    verify_materialized_parser = subparsers.add_parser("materialize-verify")
    verify_materialized_parser.add_argument("--repo", type=Path, default=Path.cwd())
    verify_materialized_parser.add_argument("--signed", required=True)
    verify_materialized_parser.add_argument("--materialized", required=True)
    staged_lineage_parser = subparsers.add_parser("staged-lineage-verify")
    staged_lineage_parser.add_argument("--repo", type=Path, default=Path.cwd())
    staged_lineage_parser.add_argument("--signed", required=True)
    staged_lineage_parser.add_argument("--current", required=True)
    args = parser.parse_args()
    try:
        if args.command == "marker":
            result = create_marker(args.repo, args.source, args.publication_id)
            if args.output:
                args.output.write_text(result + "\n", encoding="ascii")
        elif args.command == "publish":
            result = atomic_transition(
                args.repo, args.remote, source=args.source, marker=args.marker,
                ledger_old=args.ledger_old, ledger_new=args.ledger_new,
                sequence_tag=args.sequence_tag,
                materialized_output=args.materialized_output,
            )
        elif args.command == "materialize":
            result = materialize_transition(
                args.repo, args.remote, ledger_old=args.ledger_old,
                ledger_new=args.ledger_new,
            )
        elif args.command == "evidence-publish":
            result = evidence_transition(
                args.repo, args.remote, main_old=args.main_old, main_new=args.main_new,
                ledger_old=args.ledger_old, ledger_new=args.ledger_new,
                approval_target=args.approval_target, approval_tag=args.approval_tag,
            )
        elif args.command == "production-publish":
            result = production_transition(
                args.repo, args.remote, production_new=args.production_new,
                production_tag=args.production_tag,
            )
        elif args.command == "materialize-verify":
            validate_materialized_descendant(args.repo, args.materialized, args.signed)
            result = "valid"
        else:
            result = validate_staged_lineage(args.repo, args.current, args.signed)
    except (CasError, OSError, subprocess.SubprocessError) as error:
        print(f"directory-publication-cas: {error}", file=sys.stderr)
        return 1
    print(result)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
