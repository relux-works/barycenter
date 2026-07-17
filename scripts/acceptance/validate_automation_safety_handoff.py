#!/usr/bin/env python3
"""Fail-closed validation for the P3 automation engineering evidence handoff."""

from __future__ import annotations

import hashlib
import json
import pathlib
import subprocess


ROOT = pathlib.Path(__file__).resolve().parents[2]
HANDOFF_PATH = ROOT / "acceptance/phase3/automation-safety-evidence-v1.json"


class AutomationHandoffError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AutomationHandoffError(message)


def load(path: pathlib.Path = HANDOFF_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def git(*args: str) -> str:
    return subprocess.run(
        ["git", *args], cwd=ROOT, check=True, capture_output=True, text=True
    ).stdout.strip()


def digest(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate(handoff: dict) -> None:
    require(handoff.get("schemaVersion") == 1, "unsupported schema")
    require(
        handoff.get("contract") == "p3-automation-safety-evidence.v1",
        "wrong contract",
    )
    require(handoff.get("task") == "TASK-260712-2f0gpu", "wrong task")
    require(handoff.get("publishedAt") == "2026-07-17", "publication date drifted")

    baseline = handoff.get("upstreamBaselineCommit", "")
    require(len(baseline) == 40, "baseline commit missing")
    require(git("rev-parse", baseline) == baseline, "baseline commit unavailable")
    git("merge-base", "--is-ancestor", baseline, "HEAD")

    decision = handoff.get("decision", {})
    require(
        decision.get("result") == "engineering-evidence-complete-manual-c7-required",
        "engineering decision drifted",
    )
    require(decision.get("engineeringHandoffReady") is True, "handoff not ready")
    for key in ("c7Accepted", "productionPromotionAllowed"):
        require(decision.get(key) is False, f"false acceptance recorded: {key}")
    require(decision.get("manualEvidence") == "not-run", "manual evidence invented")
    require(decision.get("maximumRolloutStage") == 4, "rollout exceeded dark clients")

    frozen = handoff.get("frozenContract", {})
    require(
        frozen
        == {
            "version": "automation-safety-v1",
            "delivery": "overlay",
            "maximumExplicitSelectors": 64,
            "maximumAcceptedPerPrincipalMinute": 5,
            "maximumAcceptedPerOrbitHour": 20,
            "maximumConcurrentPerPrincipal": 1,
            "maximumConcurrentPerOrbit": 2,
            "missedMinuteCatchUpAllowed": False,
            "recipientLocalCeilingLast": True,
            "automationMayEnterCapture": False,
        },
        "frozen limits or invariants drifted",
    )

    expected_coverage = {
        "timezone-dst-clock",
        "quiet-hours",
        "dnd-block-air",
        "explicit-target-acl",
        "dedupe-restart-queue",
        "revoke-disable-cancel-limits",
        "local-ceiling",
        "no-microphone",
        "surface-history-parity",
        "rollback",
    }
    coverage = handoff.get("coverage", {})
    require(set(coverage) == expected_coverage, "coverage matrix incomplete")
    test_names: list[str] = []
    for key, record in coverage.items():
        require(
            str(record.get("repositoryStatus", "")).startswith("pass-"),
            f"repository evidence missing: {key}",
        )
        tests = record.get("tests", [])
        require(tests and len(tests) == len(set(tests)), f"invalid test inventory: {key}")
        test_names.extend(tests)

    anchors = handoff.get("sourceAnchors", [])
    require(len(anchors) == 23, "source anchor inventory drifted")
    require(
        len({item.get("path") for item in anchors}) == len(anchors),
        "duplicate source anchor",
    )
    combined_sources = ""
    for item in anchors:
        relative = pathlib.PurePosixPath(item.get("path", ""))
        require(
            relative.parts and not relative.is_absolute() and ".." not in relative.parts,
            "unsafe source anchor",
        )
        path = ROOT.joinpath(*relative.parts)
        require(path.is_file() and not path.is_symlink(), f"source anchor missing: {relative}")
        require(digest(path) == item.get("sha256"), f"source anchor drifted: {relative}")
        combined_sources += path.read_text(encoding="utf-8", errors="replace")
    for name in set(test_names):
        require(name in combined_sources, f"evidence test is not anchored: {name}")

    contract_source = (ROOT / "coordinator/internal/automation/contract.go").read_text()
    for fragment in (
        'ContractVersion = "automation-safety-v1"',
        "MaxExplicitSelectors      = 64",
        "MaxAcceptedPerMinute      = 5",
        "MaxAcceptedPerOrbitHour   = 20",
        "MaxConcurrentPerPrincipal = 1",
        "MaxConcurrentPerOrbit     = 2",
    ):
        require(fragment in contract_source, f"runtime contract drifted: {fragment}")

    runtime_test = (ROOT / "coordinator/internal/store/automation_runtime_test.go").read_text()
    for fragment in (
        "TestAutomationRuntimeAirDNDAndBlockPolicyMatrix",
        "TestAutomationRuntimeEnforcesPrincipalAndOrbitConcurrencyCaps",
        "quiet-hours transmissions",
        "TransmissionTargetMissedDND",
        "TransmissionTargetBlocked",
        "post-leave transmissions",
        "denied transmissions",
    ):
        require(fragment in runtime_test, f"policy matrix proof missing: {fragment}")

    clients = handoff.get("clientMatrix", {})
    require(set(clients) == {"windows", "macos", "telegram"}, "client matrix incomplete")
    for platform, record in clients.items():
        require(record.get("repositoryBuild") == "pass", f"build not passed: {platform}")
        require(record.get("repositoryTests") == "pass", f"tests not passed: {platform}")
        for key, value in record.items():
            if key not in {"repositoryBuild", "repositoryTests"}:
                require(value == "not-run", f"manual client result invented: {platform}.{key}")

    manual = handoff.get("manualAcceptance", {})
    require(manual.get("epic") == "EPIC-260714-th54l3", "manual epic drifted")
    require(manual.get("c7Task") == "TASK-260712-1gyohk", "manual C7 task drifted")
    require(manual.get("status") == "not-run", "manual C7 result invented")
    gaps = manual.get("remainingGaps", [])
    require(len(gaps) == 12 and len(set(gaps)) == 12, "manual gap inventory drifted")
    for required in (
        "signed-windows-app",
        "signed-macos-app",
        "physical-clock-and-timezone",
        "audible-dnd-block-air-and-ceiling",
        "real-client-kill-switch-latency",
        "seven-day-beta",
    ):
        require(required in gaps, f"manual gap missing: {required}")

    rollback = handoff.get("rollbackOrder", [])
    require([item.get("stage") for item in rollback] == [1, 2, 3, 4, 5], "rollback order drifted")
    require(
        rollback[0].get("action") == "emergency-disable-orbit-automation"
        and rollback[0].get("preservesManualSoundboard") is True,
        "emergency rollback boundary drifted",
    )
    require(
        rollback[-1].get("action") == "deploy-retained-predecessor-with-additive-rows-preserved",
        "unsafe predecessor rollback",
    )

    commands = handoff.get("rerunCommands", [])
    require(len(commands) == 6 and len(set(commands)) == 6, "rerun command inventory drifted")
    for fragment in (
        "TestAutomationRuntimeAirDNDAndBlockPolicyMatrix",
        "TestAutomationExactPreviousHeadRollback",
        "TelegramSoundboardAutomationParity",
        "WindowsSoundboardCompositionTriggersWithoutCapture",
        "xcrun swift test",
        "--suite all --require-clean",
    ):
        require(any(fragment in command for command in commands), f"rerun command missing: {fragment}")

    handoff_doc = (ROOT / "docs/analysis/p3-automation-safety-evidence-handoff.md").read_text()
    for fragment in (
        "TASK-260712-1gyohk",
        "manualEvidence",
        "Never delete additive automation tables",
        "signed Windows and signed macOS application interaction",
    ):
        require(fragment in handoff_doc, f"handoff disclosure missing: {fragment}")


def main() -> int:
    handoff = load()
    validate(handoff)
    print(
        json.dumps(
            {
                "contract": handoff["contract"],
                "decision": handoff["decision"]["result"],
                "manualEvidence": handoff["decision"]["manualEvidence"],
                "status": "pass",
            },
            sort_keys=True,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
