#!/usr/bin/env python3
"""Generate the deterministic Phase 1 root-review diff inventory."""

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


def task_card(task_id: str) -> dict[str, str]:
    matches = sorted(ROOT.glob(f".task-board/**/{task_id}_*/README.md"))
    if len(matches) != 1:
        raise RuntimeError(f"expected one task card for {task_id}, found {len(matches)}")
    readme = matches[0]
    progress = readme.with_name("progress.md")
    body = readme.read_text()
    progress_body = progress.read_text() if progress.exists() else ""
    title = body.splitlines()[0].removeprefix("# ").strip()
    status = section(progress_body, "Status") or section(body, "Status")
    ac = section(body, "Acceptance Criteria")
    if not ac:
        raise RuntimeError(f"missing acceptance criteria for {task_id}")
    return {
        "title": title,
        "status_at_review": status,
        "acceptance_criteria": ac,
        "acceptance_criteria_sha256": sha256(ac),
        "card": str(readme.relative_to(ROOT)),
    }


def scenario_map() -> dict[str, list[str]]:
    result: dict[str, list[str]] = {}

    def add(ids: str, scenarios: str) -> None:
        for task_id in ids.split():
            result[task_id] = scenarios.split()

    add("TASK-260712-1vtwkl", "A1 A8")
    add("TASK-260712-z6h6wh TASK-260712-1bnos4 TASK-260712-2af2dp", "A1 A2 A7")
    add("TASK-260712-1sae4q TASK-260712-3mcof4", "A2 A6 A7")
    add("TASK-260712-12ojcb", "A5 A7")
    add("TASK-260712-gj0cko TASK-260712-3huupe TASK-260712-jolzhh", "A1 A2 A5 A6 A7")
    add(
        "TASK-260712-51y5k9 TASK-260712-1aprcb TASK-260712-1g70av "
        "TASK-260712-2qpp6w TASK-260712-26ip33 TASK-260712-2bbz13 "
        "TASK-260712-31vvjt TASK-260712-2qc27p TASK-260712-2cdjq8",
        "A2 A3 A4 A5 A6 A7",
    )
    add("TASK-260712-16zfvu TASK-260712-g9ycx5 TASK-260712-1epb3a TASK-260712-1x0lot", "A6 A8")
    add("TASK-260712-2kec2s TASK-260712-3t9nr8", "A6 A8")
    add(
        "TASK-260712-1hqiek TASK-260712-1viwvi TASK-260712-2zbmq4 "
        "TASK-260712-1g6lk8 TASK-260712-8mwyiv TASK-260712-3d6cnn",
        "A3 A4",
    )
    add(
        "TASK-260712-3coble TASK-260712-1gx6mh TASK-260712-3dmllz "
        "TASK-260712-1c1ska TASK-260712-2hcq1g TASK-260712-21ers7 "
        "TASK-260712-3e4p0c TASK-260712-3d0zgu TASK-260712-1f9jtm",
        "A5 A6 A7",
    )
    add("TASK-260712-1c04pk TASK-260712-2lrpc0", "A1 A8")
    add("TASK-260712-30abcm TASK-260712-2w4gyw", "A1 A2 A7 A8")
    add(
        "TASK-260712-9i5se7 TASK-260712-3lg0ht TASK-260712-ut6akw "
        "TASK-260712-25at8b TASK-260712-c7dmv8 TASK-260712-1s6h6t "
        "TASK-260712-1p8ykc TASK-260712-3dqc3l TASK-260712-2fe5bz",
        "A1 A2 A6 A7 A8",
    )
    add("TASK-260712-1cdoxh", "A1 A2 A3 A4 A5 A6 A7 A8")
    add("TASK-260712-pbfz37 TASK-260712-34stvx TASK-260712-dlltnr", "A6 A8")
    add("TASK-260712-e1ie4x TASK-260712-2s4e9p", "A1 A8")
    add("TASK-260712-176b74", "A2 A3 A4 A5 A6 A7")
    add("TASK-260712-1uz0za", "A3 A4")
    add("TASK-260712-1xkn75", "A1 A2 A5 A6 A7")
    add("TASK-260712-wy05n6", "A1 A2 A5 A6 A7 A8")
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
    if path.startswith(("docs/", "assets/")):
        return "product-evidence-and-assets"
    if path.startswith((".task-board/", ".planning/")):
        return "governance"
    return "repository-support"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--baseline", required=True)
    parser.add_argument("--candidate", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()

    baseline = git("rev-parse", args.baseline).strip()
    candidate = git("rev-parse", args.candidate).strip()
    commits = git("rev-list", "--first-parent", "--reverse", f"{baseline}..{candidate}").splitlines()
    scenarios = scenario_map()
    intervals = []
    file_tasks: dict[str, set[str]] = defaultdict(set)
    task_commits: dict[str, list[str]] = defaultdict(list)

    for commit in commits:
        parent = git("rev-parse", f"{commit}^1").strip()
        subject = git("show", "-s", "--format=%s", commit).strip()
        task_ids = sorted(
            {"TASK-" + match[5:].lower() for match in TASK_RE.findall(subject)}
        )
        if len(task_ids) != 1:
            raise RuntimeError(f"commit {commit} has {len(task_ids)} task IDs: {subject}")
        task_id = task_ids[0]
        if task_id not in scenarios:
            raise RuntimeError(f"missing A1-A8 mapping for {task_id}")
        paths = git("diff", "--name-only", "--no-renames", parent, commit).splitlines()
        for path in paths:
            file_tasks[path].add(task_id)
        task_commits[task_id].append(commit)
        intervals.append(
            {
                "commit": commit,
                "first_parent": parent,
                "subject": subject,
                "task_id": task_id,
                "scenarios": scenarios[task_id],
                "changed_files": len(paths),
            }
        )

    numstat: dict[str, tuple[int | None, int | None]] = {}
    for line in git("diff", "--numstat", "--no-renames", baseline, candidate).splitlines():
        added, deleted, path = line.split("\t", 2)
        numstat[path] = (
            None if added == "-" else int(added),
            None if deleted == "-" else int(deleted),
        )

    changed_paths = git("diff", "--name-only", "--no-renames", baseline, candidate).splitlines()
    unmapped = sorted(set(changed_paths) - set(file_tasks))
    if unmapped:
        raise RuntimeError(f"files without task mapping: {unmapped}")

    files = []
    for path in changed_paths:
        tasks = sorted(file_tasks[path])
        blob_check = subprocess.run(
            ["git", "cat-file", "-e", f"{candidate}:{path}"],
            cwd=ROOT,
            capture_output=True,
        )
        blob = git("rev-parse", f"{candidate}:{path}").strip() if blob_check.returncode == 0 else None
        added, deleted = numstat[path]
        files.append(
            {
                "path": path,
                "classification": classify(path),
                "candidate_blob": blob,
                "added_lines": added,
                "deleted_lines": deleted,
                "task_ids": tasks,
                "scenarios": sorted({s for task in tasks for s in scenarios[task]}),
            }
        )

    tasks = {}
    for task_id in sorted(task_commits):
        card = task_card(task_id)
        tasks[task_id] = {
            **card,
            "scenarios": scenarios[task_id],
            "first_parent_commits": task_commits[task_id],
        }

    manifest = {
        "schema": "barycenter.p1-root-review-manifest.v1",
        "baseline": baseline,
        "reviewed_candidate": candidate,
        "review_decision": "engineering-evidence-ready; phase-1-acceptance-withheld",
        "acceptance_boundary": {
            "independent_reviews": "open",
            "manual_real_app_hardware": "open in EPIC-260714-th54l3",
            "partner_center_and_iarc": "open in TASK-260715-24ube9",
            "accepted_build": None,
        },
        "source_scenarios": "docs/spec-self-contained-audio.md#194-acceptance-scenarios",
        "scenario_task_mapping_policy": "Task-level mapping is explicit in tasks; each file inherits the union of every first-parent task interval that changed it.",
        "totals": {
            "first_parent_intervals": len(intervals),
            "unique_tasks": len(tasks),
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
    args.output.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    print(
        f"wrote {args.output}: {len(files)} files, {len(tasks)} tasks, "
        f"{len(intervals)} first-parent intervals"
    )


if __name__ == "__main__":
    main()
