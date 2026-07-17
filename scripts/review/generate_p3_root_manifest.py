#!/usr/bin/env python3
"""Generate the deterministic non-E2EE Phase 3 root-review inventory."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
from collections import defaultdict
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
TASK_RE = re.compile(r"task-\d{6}-[a-z0-9]+", re.IGNORECASE)

COMMIT_OVERRIDES = {
    "3907516f58129879ac26c22744d749cbb92d0790": "p3-root-review-start",
    "e254b974f91bb78bb872900726da1f6b9c1a508d": "p3-sequence-resume",
    "f33f1fbb8330ce946e5ecf748f7a522d2ba32d81": "TASK-260712-2kj9kj",
    "92c8b8807517921cfcc5d230dab07e4321383c67": "TASK-260712-2kj9kj",
    "b4fb6f7abdf0f4f669b123afb9f3a136a0161efb": "TASK-260712-2jbo5i",
    "bb023c8c48d7b63ab3a5f42af16d0d5f3b59e0a1": "TASK-260712-2jbo5i",
    "4d2fa559b5ceb818ff239e36495c53bc5f841b30": "TASK-260712-3sj8ox",
    "41bad23e0eefde221209d412e251dbeca56b6be5": "TASK-260712-3sj8ox",
    "478e1aa1c5431e8fdbf443e62afceb5844475dd4": "TASK-260712-16xmy2",
    "747f6862706d7a6f68be1cc2013f3f69ff8ce84c": "TASK-260712-16xmy2",
    "b3a64badf1232d2273f74af4baa0b6e8f07bbaca": "TASK-260712-3er89x",
    "73bdc18d14721936ad75c86cd135105175cc101e": "TASK-260712-3er89x",
}

DEFERRED_E2EE = {
    "TASK-260712-2e2ymn",
    "TASK-260712-16xmy2",
    "TASK-260712-3er89x",
    "TASK-260712-2ys1ww",
}


def git(*args: str) -> str:
    return subprocess.run(
        ["git", *args], cwd=ROOT, check=True, text=True, capture_output=True
    ).stdout


def sha256(value: str) -> str:
    return hashlib.sha256(value.encode()).hexdigest()


def section(markdown: str, heading: str) -> str:
    match = re.search(
        rf"^## {re.escape(heading)}\s*$\n(.*?)(?=^## |\Z)",
        markdown,
        flags=re.MULTILINE | re.DOTALL,
    )
    return match.group(1).strip() if match else ""


def task_card(task_id: str, candidate: str) -> dict[str, str]:
    matches = sorted(
        path
        for path in git("ls-tree", "-r", "--name-only", candidate, ".task-board").splitlines()
        if path.endswith("/README.md") and f"/{task_id}_" in path
    )
    if len(matches) != 1:
        raise RuntimeError(f"expected one task card for {task_id}, found {len(matches)}")
    readme = matches[0]
    progress = str(Path(readme).with_name("progress.md"))
    body = git("show", f"{candidate}:{readme}")
    progress_body = git("show", f"{candidate}:{progress}")
    acceptance = section(body, "Acceptance Criteria")
    scope = section(body, "Scope")
    if not acceptance or not scope:
        raise RuntimeError(f"missing scope or acceptance criteria for {task_id}")
    return {
        "title": body.splitlines()[0].removeprefix("# ").strip(),
        "status_at_review": section(progress_body, "Status"),
        "scope": scope,
        "scope_sha256": sha256(scope),
        "acceptance_criteria": acceptance,
        "acceptance_criteria_sha256": sha256(acceptance),
        "card": readme,
    }


def gate_map() -> dict[str, list[str]]:
    result: dict[str, list[str]] = {}

    def add(ids: str, gates: str) -> None:
        for task_id in ids.split():
            result[task_id] = gates.split()

    add(
        "TASK-260712-lo7a68 TASK-260712-3qviqc TASK-260712-3vzbbl "
        "TASK-260712-19w1qn TASK-260712-26mnp1 TASK-260712-1ckdr7 "
        "TASK-260712-ezdhpf TASK-260712-2kj9kj TASK-260712-2jbo5i",
        "C1 C2 21.4-realtime",
    )
    add(
        "TASK-260712-3sj8ox TASK-260712-hb5xz2 TASK-260712-3sv87k "
        "TASK-260712-1kk8bd TASK-260712-1eva0y TASK-260712-11e4e3 "
        "TASK-260712-1yw7fo TASK-260712-288j4a TASK-260712-uht9e2 "
        "TASK-260712-89fzlc TASK-260712-1oodka TASK-260712-2f0gpu",
        "C7 21.4-automation",
    )
    add(
        "TASK-260712-1gmsvh TASK-260712-1pw1l1 TASK-260712-39czd2 "
        "TASK-260712-2egweh TASK-260712-wcdz08 TASK-260712-1getbv "
        "TASK-260712-39zh8g TASK-260712-1023d7",
        "C3 21.4-capture-quality",
    )
    add(
        "TASK-260712-3da0vz TASK-260712-2uo81g",
        "C1 C2 C3 C4 C5 C6 C7 21.4-gates 21.4-observability",
    )
    add("TASK-260712-3g0axs", "C1 C2 C3 C7 21.4-root-review")
    add(
        "TASK-260712-2e2ymn TASK-260712-16xmy2 TASK-260712-3er89x "
        "TASK-260712-2ys1ww",
        "C4 C5 C6 deferred-e2ee",
    )
    return result


def classify(path: str) -> str:
    if path.startswith("coordinator/"):
        return "coordinator"
    if path.startswith("pulsar-win/"):
        return "windows-client"
    if path.startswith("node-app/"):
        return "macos-client"
    if path.startswith("protocol/"):
        return "protocol-and-goldens"
    if path.startswith(("acceptance/", "scripts/", ".github/")):
        return "verification-and-contract"
    if path.startswith("docs/"):
        return "operational-and-product-docs"
    if path.startswith((".task-board/", ".planning/")):
        return "governance"
    return "repository-context"


def interval_owner(commit: str, subject: str) -> tuple[str, str]:
    override = COMMIT_OVERRIDES.get(commit)
    if override in {"p3-sequence-resume", "p3-root-review-start"}:
        return override, "repository-context"
    found = sorted({"TASK-" + match[5:].lower() for match in TASK_RE.findall(subject)})
    owner = override
    if owner is None and len(found) == 1:
        owner = found[0]
    if owner is None:
        raise RuntimeError(f"unmapped interval {commit}: {subject}")
    if owner in DEFERRED_E2EE:
        return owner, "deferred-e2ee"
    return owner, "reviewed-non-e2ee"


def diff_metrics(parent: str, commit: str) -> tuple[str, int, int, int]:
    patch = git("diff", "--no-ext-diff", "--no-renames", "--unified=0", parent, commit)
    hunks = sum(line.startswith("@@ ") for line in patch.splitlines())
    added = deleted = 0
    for line in git("diff", "--numstat", "--no-renames", parent, commit).splitlines():
        raw_added, raw_deleted, _ = line.split("\t", 2)
        added += 0 if raw_added == "-" else int(raw_added)
        deleted += 0 if raw_deleted == "-" else int(raw_deleted)
    return sha256(patch), hunks, added, deleted


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--baseline", required=True)
    parser.add_argument("--candidate", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()

    baseline = git("rev-parse", args.baseline).strip()
    candidate = git("rev-parse", args.candidate).strip()
    candidate_tree = git("rev-parse", f"{candidate}^{{tree}}").strip()
    gates = gate_map()
    commits = git("rev-list", "--first-parent", "--reverse", f"{baseline}..{candidate}").splitlines()
    intervals = []
    file_owners: dict[str, set[str]] = defaultdict(set)
    file_scopes: dict[str, set[str]] = defaultdict(set)
    task_commits: dict[str, list[str]] = defaultdict(list)

    for commit in commits:
        parent = git("rev-parse", f"{commit}^1").strip()
        subject = git("show", "-s", "--format=%s", commit).strip()
        owner, review_scope = interval_owner(commit, subject)
        if review_scope != "repository-context" and owner not in gates:
            raise RuntimeError(f"missing C1-C7 mapping for {owner}")
        paths = git("diff", "--name-only", "--no-renames", parent, commit).splitlines()
        for path in paths:
            file_owners[path].add(owner)
            file_scopes[path].add(review_scope)
        if review_scope != "repository-context":
            task_commits[owner].append(commit)
        patch_digest, hunks, added, deleted = diff_metrics(parent, commit)
        intervals.append({
            "commit": commit,
            "first_parent": parent,
            "subject": subject,
            "owner": owner,
            "review_scope": review_scope,
            "gates": gates.get(owner, []),
            "changed_files": len(paths),
            "hunks": hunks,
            "added_lines": added,
            "deleted_lines": deleted,
            "patch_sha256": patch_digest,
        })

    numstat: dict[str, tuple[int | None, int | None]] = {}
    for line in git("diff", "--numstat", "--no-renames", baseline, candidate).splitlines():
        added, deleted, path = line.split("\t", 2)
        numstat[path] = (
            None if added == "-" else int(added),
            None if deleted == "-" else int(deleted),
        )
    paths = git("diff", "--name-only", "--no-renames", baseline, candidate).splitlines()
    unmapped = sorted(set(paths) - set(file_owners))
    if unmapped:
        raise RuntimeError(f"files without interval ownership: {unmapped}")

    files = []
    for path in paths:
        owners = sorted(file_owners[path])
        scopes = sorted(file_scopes[path])
        exists = subprocess.run(
            ["git", "cat-file", "-e", f"{candidate}:{path}"], cwd=ROOT, capture_output=True
        ).returncode == 0
        added, deleted = numstat[path]
        files.append({
            "path": path,
            "classification": classify(path),
            "candidate_blob": git("rev-parse", f"{candidate}:{path}").strip() if exists else None,
            "added_lines": added,
            "deleted_lines": deleted,
            "owners": owners,
            "review_scopes": scopes,
            "gates": sorted({gate for owner in owners for gate in gates.get(owner, [])}),
        })

    tasks = {
        task_id: {
            **task_card(task_id, candidate),
            "review_scope": "deferred-e2ee" if task_id in DEFERRED_E2EE else "reviewed-non-e2ee",
            "gates": gates[task_id],
            "first_parent_commits": task_commits[task_id],
        }
        for task_id in sorted(task_commits)
    }
    manifest = {
        "schema": "barycenter.p3-root-review-manifest.v1",
        "baseline": baseline,
        "reviewed_candidate": candidate,
        "review_decision": "engineering-baseline-accepted-production-blocked",
        "acceptance_boundary": {
            "non_e2ee_phase3_engineering": "accepted-for-reversible-continuation",
            "manual_real_app_hardware": "open in EPIC-260714-th54l3",
            "deferred_e2ee": "excluded and owned by EPIC-260716-3qsztl",
            "independent_reviews": "required after root review",
            "production_promotion": "blocked",
            "accepted_source_candidate": candidate,
            "accepted_source_tree": candidate_tree,
            "accepted_build": None,
            "accepted_package": None,
        },
        "mapping_policy": "Every first-parent diff hunk is hashed and owned by one task or explicit repository context. E2EE intervals remain inventoried but excluded from this root review.",
        "totals": {
            "first_parent_intervals": len(intervals),
            "reviewed_intervals": sum(item["review_scope"] == "reviewed-non-e2ee" for item in intervals),
            "deferred_e2ee_intervals": sum(item["review_scope"] == "deferred-e2ee" for item in intervals),
            "repository_context_intervals": sum(item["review_scope"] == "repository-context" for item in intervals),
            "unique_reviewed_tasks": sum(item["review_scope"] == "reviewed-non-e2ee" for item in tasks.values()),
            "unique_deferred_e2ee_tasks": sum(item["review_scope"] == "deferred-e2ee" for item in tasks.values()),
            "changed_files": len(files),
            "aggregate_interval_hunks": sum(item["hunks"] for item in intervals),
            "added_lines_no_renames": sum(item[0] or 0 for item in numstat.values()),
            "deleted_lines_no_renames": sum(item[1] or 0 for item in numstat.values()),
            "unmapped_files": 0,
        },
        "tasks": tasks,
        "first_parent_intervals": intervals,
        "files": files,
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    print(
        f"wrote {args.output}: {len(files)} files, {len(tasks)} tasks, "
        f"{len(intervals)} intervals, {manifest['totals']['aggregate_interval_hunks']} hunks"
    )


if __name__ == "__main__":
    main()
