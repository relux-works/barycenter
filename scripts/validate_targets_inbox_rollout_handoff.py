#!/usr/bin/env python3
"""Fail closed when the P2 targets/inbox rollout handoff drifts or overclaims."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "acceptance" / "targets-inbox-rollout-handoff-v1.json"
HANDOFF = ROOT / "docs" / "analysis" / "p2-targets-inbox-rollout-handoff.md"


class HandoffError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise HandoffError(message)


def load(path: Path = MANIFEST) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(value: dict[str, Any], root: Path = ROOT) -> None:
    require(value.get("schemaVersion") == 1, "unsupported rollout schema")
    require(value.get("contract") == "p2-targets-inbox-rollout-handoff.v1", "wrong contract")
    require(value.get("task") == "TASK-260712-20cuna", "wrong owner task")
    require(value.get("scope") == "repository-engineering-handoff", "scope changed")
    require(value.get("executionStatus") == "documented-not-executed", "rollout was falsely claimed")
    require(value.get("sourceContracts") == [
        "p1-transmission-v1", "pulsar.air-lifecycle-policy.v1", "p2-targets-inbox-parity.v1",
        "pulsar.targets-inbox-presentation.v1", "p2-targets-inbox-parity-regressions.v1",
    ], "source contract order changed")

    surfaces = value.get("surfaces", [])
    surface_keys = [(item.get("method"), item.get("path")) for item in surfaces]
    require(len(surface_keys) == 8 and len(set(surface_keys)) == 8, "API surface inventory changed")
    require(("POST", "/v1/transmissions") in surface_keys, "create route missing")
    require(("POST", "/v1/inbox/{ib_}/replays") in surface_keys, "manual replay route missing")

    rollout = value.get("rollout", [])
    require([stage.get("sequence") for stage in rollout] == list(range(1, 8)), "rollout order changed")
    require([stage.get("id") for stage in rollout] == [
        "freeze-and-backup", "coordinator-schema-dark", "reconcile-and-read",
        "desktop-and-telegram-models-dark", "bounded-clip-cohort",
        "streamed-track-extension", "manual-acceptance-and-rollout-rehearsal",
    ], "rollout stages changed")
    require(all(len(stage.get("required", [])) >= 4 for stage in rollout), "rollout guard missing")
    require(rollout[5].get("blockedByStory") == "STORY-260712-2ori1t", "track gate changed")
    require(rollout[6].get("manualTasks") == ["TASK-260712-3u5cdn", "TASK-260712-3qybi2"],
            "manual rollout tasks changed")

    mixed = value.get("mixedVersion", {})
    require(mixed.get("targetedTrackBeforeStreamStory", "").startswith("visible unsupported"),
            "targeted tracks were enabled early")
    for key in ("partialCreateAllowed", "broadcastFallbackAllowed", "plaintextFallbackForFutureE2EEAllowed"):
        require(mixed.get(key) is False, f"unsafe mixed-version fallback enabled: {key}")

    rollback = value.get("rollback", {})
    require(rollback.get("stopWriterBeforeArtifactChange") is True, "single-writer rollback guard removed")
    require(rollback.get("downMigrationAllowed") is False, "destructive down migration enabled")
    require(rollback.get("manualSQLLifecycleMutationAllowed") is False, "manual lifecycle mutation enabled")
    require(set(rollback.get("preserveTables", [])) == {
        "transmission_target_references", "transmission_targets", "transmission_inbox_items",
        "transmission_replay_lineage", "transmission_inbox_cursors", "transmission_receipt_cursors",
        "content_policy_acceptances", "moderation_reports",
    }, "rollback preservation set changed")
    require(rollback.get("testedPredecessorRevisions") == [
        "0c1e1946ff692aa553c19ca6bf7328150d1a24b8",
        "2aa97c2d08cb93b110200ae159fd43265410ff5a",
    ], "tested predecessor revisions changed")

    downstream = value.get("downstream", [])
    downstream_ids = [item.get("task") for item in downstream]
    require(len(downstream_ids) == len(set(downstream_ids)) == 11, "downstream task inventory changed")
    for task in downstream_ids:
        require(list((root / ".task-board").glob(f"**/{task}_*/progress.md")), f"unknown downstream task: {task}")

    evidence = value.get("evidence", [])
    require(len(evidence) >= 8, "evidence index incomplete")
    for anchor in evidence:
        relative = Path(anchor.get("path", ""))
        require(relative.parts and not relative.is_absolute() and ".." not in relative.parts,
                f"unsafe evidence path: {relative}")
        path = root / relative
        require(path.is_file(), f"missing evidence file: {relative}")
        symbol = anchor.get("symbol", "")
        require(symbol and symbol in path.read_text(encoding="utf-8"),
                f"missing evidence symbol {symbol} in {relative}")

    manual = value.get("manualBoundary", [])
    require([(item.get("task"), item.get("epic"), item.get("status")) for item in manual] == [
        ("TASK-260712-3u5cdn", "EPIC-260714-th54l3", "manual-required"),
        ("TASK-260712-3qybi2", "EPIC-260714-th54l3", "manual-required"),
    ], "manual evidence was claimed or rerouted")

    handoff_path = root / HANDOFF.relative_to(ROOT)
    require(handoff_path.is_file(), "implementation handoff is missing")
    handoff = handoff_path.read_text(encoding="utf-8")
    for heading in (
        "## Final API, receipt and rights contract", "## Required rollout order",
        "## Drain-before-rollback and roll-forward", "## Mixed-version window",
        "## Downstream ownership", "## Evidence and manual boundary",
    ):
        require(heading in handoff, f"handoff section missing: {heading}")
    for task in downstream_ids + ["TASK-260712-20cuna"]:
        require(task in handoff, f"handoff does not name {task}")


def main() -> None:
    value = load()
    validate(value)
    print(json.dumps({
        "status": "pass",
        "contract": value["contract"],
        "executionStatus": value["executionStatus"],
        "rolloutStages": len(value["rollout"]),
        "downstreamTasks": len(value["downstream"]),
        "manualStatus": sorted({item["status"] for item in value["manualBoundary"]}),
    }, sort_keys=True))


if __name__ == "__main__":
    main()
