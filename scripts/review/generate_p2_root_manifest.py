#!/usr/bin/env python3
"""Generate the deterministic Phase 2 root-review diff inventory."""

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
    "e4aa266913ed7daed1ae07c50d7b33c1e7d1288f": "TASK-260712-3nq0tq",
    "a6f39633d093fc8c8e0a463e55c8797a18a446b2": "TASK-260712-3nq0tq",
    "8f91187d3ab9bb62fe31a00407a1a7058df27d9b": "TASK-260712-14u0yk",
    "80e892b91afdfa8203eab7c19b14b294a3b7db2d": "TASK-260712-2bk0vy",
    "f52fae1c0949b48d898732cd5408b3468caa18da": "TASK-260712-2bk0vy",
    "76d054d8ef8e8195ef3cfad32fcfbe01f4354b53": "TASK-260712-2ubzyf",
    "e6609944848150c9f540582643562df409017dff": "TASK-260712-2ubzyf",
    "d3db8c9f367bc5de2fd40bd047050100ddcc1825": "TASK-260712-14rxuk",
    "496c07272e4a5406b44be8709fa84c9b5932cdda": "TASK-260712-14rxuk",
    "f1041ae6a340e7368cf87a1fd8f90fa2cd4203b3": "TASK-260712-2g3fkt",
    "090be8c68b74319c6ca50e063c78d61d2ae16064": "TASK-260712-28mn7w",
    "fb50d39754f343e4eb89f527af4aa434b587c6bd": "TASK-260712-2sicfs",
    "f5be2ec60a9e94080142982d51729ee4687a3901": "TASK-260712-n11rg6",
    "7681c961a703a73dd4f4d5e030f954eb866faab3": "TASK-260712-qi81vf",
}

CONTEXT_OVERRIDES = {
    "ecd633839f9929cb4d7f7f546655e2a5c0d6c27f": "p3-e2ee-independent-audit-gate",
    "4fcbe0a2faed5fa3a39b2cb74aa719bf14b00543": "product-positioning-brief",
}


def git(*args: str) -> str:
    return subprocess.run(
        ["git", *args], cwd=ROOT, check=True, text=True, capture_output=True
    ).stdout


def sha256(text: str) -> str:
    return hashlib.sha256(text.encode()).hexdigest()


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
    if not acceptance:
        raise RuntimeError(f"missing acceptance criteria for {task_id}")
    return {
        "title": body.splitlines()[0].removeprefix("# ").strip(),
        "status_at_review": section(progress_body, "Status"),
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
        "TASK-260712-17yizc TASK-260712-3n36ny TASK-260712-kr64r2 "
        "TASK-260712-2vhf80 TASK-260712-25862f TASK-260712-2bjdlb "
        "TASK-260712-2i3u7v TASK-260712-31zja2 TASK-260712-2zdetx "
        "TASK-260712-3nq0tq",
        "B2 B3 B4 B6 20.5-migration 20.5-scale",
    )
    add(
        "TASK-260712-14u0yk TASK-260712-dqdoqj TASK-260712-1vdlkw "
        "TASK-260712-1canzv TASK-260712-298tyq TASK-260712-350u8d "
        "TASK-260712-3vkcki TASK-260712-ibuaxj TASK-260712-2eympi",
        "B1 B6 20.5-track-start 20.5-start-skew 20.5-seek 20.5-memory",
    )
    add(
        "TASK-260712-2rlkp7 TASK-260712-1c34fe TASK-260712-2bk0vy "
        "TASK-260712-2ctf3x TASK-260712-2j5fkr TASK-260712-2zoy4u "
        "TASK-260712-2vipy3 TASK-260712-2nto40 TASK-260712-cuplon "
        "TASK-260712-1vklop TASK-260712-20cuna",
        "B5 B6 B7 20.5-migration",
    )
    add(
        "TASK-260712-1n5fks TASK-260712-31rkpe TASK-260712-285pag "
        "TASK-260712-3lf8r0 TASK-260712-17w78q TASK-260712-1q2kwa "
        "TASK-260712-3aj8w2 TASK-260712-3lximx TASK-260712-2psvhu",
        "B1 B6 20.5-track-start 20.5-start-skew 20.5-seek 20.5-memory",
    )
    add("TASK-260712-2ogntd", "B1 B6 20.5-accounting")
    add("TASK-260712-2h6snp", "B1 B2 B3 B4 B6 20.5-accounting")
    add("TASK-260712-wt2n7m", "B5 B6 B7")
    add("TASK-260712-2ubzyf", "B1 B2 B3 B4 B5 B6 B7 18-rollout")
    add(
        "TASK-260712-14rxuk TASK-260712-2g3fkt TASK-260712-28mn7w "
        "TASK-260712-2sicfs TASK-260712-n11rg6 TASK-260712-qi81vf "
        "TASK-260712-1kfnpu",
        "B1 B2 B3 B4 B5 B6 B7 17-observability 18-rollout 20.5 20.6-beta",
    )
    return result


def classify(path: str) -> str:
    if path.startswith("coordinator/"):
        return "coordinator"
    if path.startswith("pulsar-win/"):
        return "windows-client"
    if path.startswith("node-app/"):
        return "macos-client"
    if path.startswith(("protocol/", "acceptance/", "scripts/", ".github/")):
        return "verification-and-contract"
    if path.startswith("docs/"):
        return "operational-and-product-docs"
    if path.startswith((".task-board/", ".planning/")):
        return "governance"
    return "repository-context"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--baseline", required=True)
    parser.add_argument("--candidate", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()

    baseline = git("rev-parse", args.baseline).strip()
    candidate = git("rev-parse", args.candidate).strip()
    gates = gate_map()
    commits = git("rev-list", "--first-parent", "--reverse", f"{baseline}..{candidate}").splitlines()
    intervals = []
    file_owners: dict[str, set[str]] = defaultdict(set)
    task_commits: dict[str, list[str]] = defaultdict(list)

    for commit in commits:
        parent = git("rev-parse", f"{commit}^1").strip()
        subject = git("show", "-s", "--format=%s", commit).strip()
        found = sorted({"TASK-" + match[5:].lower() for match in TASK_RE.findall(subject)})
        owner = COMMIT_OVERRIDES.get(commit)
        kind = "task"
        if owner is None and len(found) == 1:
            owner = found[0]
        elif owner is None and commit in CONTEXT_OVERRIDES:
            owner = CONTEXT_OVERRIDES[commit]
            kind = "repository-context"
        if owner is None:
            raise RuntimeError(f"unmapped interval {commit}: {subject}")
        if kind == "task" and owner not in gates:
            raise RuntimeError(f"missing B1-B7 mapping for {owner}")
        paths = git("diff", "--name-only", "--no-renames", parent, commit).splitlines()
        for path in paths:
            file_owners[path].add(owner)
        if kind == "task":
            task_commits[owner].append(commit)
        intervals.append({
            "commit": commit,
            "first_parent": parent,
            "subject": subject,
            "kind": kind,
            "owner": owner,
            "gates": gates.get(owner, []),
            "changed_files": len(paths),
        })

    numstat: dict[str, tuple[int | None, int | None]] = {}
    for line in git("diff", "--numstat", "--no-renames", baseline, candidate).splitlines():
        added, deleted, path = line.split("\t", 2)
        numstat[path] = (None if added == "-" else int(added), None if deleted == "-" else int(deleted))
    paths = git("diff", "--name-only", "--no-renames", baseline, candidate).splitlines()
    unmapped = sorted(set(paths) - set(file_owners))
    if unmapped:
        raise RuntimeError(f"files without interval ownership: {unmapped}")

    files = []
    for path in paths:
        owners = sorted(file_owners[path])
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
            "gates": sorted({gate for owner in owners for gate in gates.get(owner, [])}),
        })

    tasks = {
        task_id: {
            **task_card(task_id, candidate),
            "gates": gates[task_id],
            "first_parent_commits": task_commits[task_id],
        }
        for task_id in sorted(task_commits)
    }
    manifest = {
        "schema": "barycenter.p2-root-review-manifest.v1",
        "baseline": baseline,
        "reviewed_candidate": candidate,
        "review_decision": "engineering-baseline-accepted-production-blocked",
        "acceptance_boundary": {
            "production_phase2": "blocked",
            "manual_real_app_hardware": "open in EPIC-260714-th54l3",
            "independent_owner_approvals": "open in EPIC-260714-zmnd4n",
            "codec_selection": "no-go",
            "accepted_source_candidate": candidate,
            "accepted_source_tree": git("rev-parse", f"{candidate}^{{tree}}").strip(),
            "accepted_build": None,
            "accepted_package": None,
        },
        "mapping_policy": "Each file inherits every first-parent task interval and B1-B7/section gate that changed it; two non-Phase-2 repository-context intervals remain explicit.",
        "totals": {
            "first_parent_intervals": len(intervals),
            "unique_phase2_tasks": len(tasks),
            "repository_context_intervals": sum(item["kind"] == "repository-context" for item in intervals),
            "changed_files": len(files),
            "added_lines": sum(item[0] or 0 for item in numstat.values()),
            "deleted_lines": sum(item[1] or 0 for item in numstat.values()),
            "unmapped_files": 0,
        },
        "tasks": tasks,
        "first_parent_intervals": intervals,
        "files": files,
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {args.output}: {len(files)} files, {len(tasks)} tasks, {len(intervals)} intervals")


if __name__ == "__main__":
    main()
