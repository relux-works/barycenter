#!/usr/bin/env python3
"""Validate the frozen Phase 2 gate and evidence contract without claiming proof."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
MATRIX = ROOT / "acceptance" / "phase2" / "gate-matrix-v1.json"
TEMPLATE = ROOT / "acceptance" / "phase2" / "result-template-v1.json"
HANDOFF = ROOT / "docs" / "acceptance" / "phase2-gate-matrix.md"


class GateMatrixError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise GateMatrixError(message)


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: Path = MATRIX) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def board_task(root: Path, task: str) -> list[Path]:
    return list((root / ".task-board").glob(f"**/{task}_*/progress.md"))


def command_script(command: str) -> str | None:
    if command.startswith("<"):
        return None
    parts = command.split()
    if not parts:
        return None
    if parts[0] in ("python3", "bash", "pwsh") and len(parts) > 1:
        return parts[1]
    if parts[0].startswith("scripts/"):
        return parts[0]
    return None


def validate(value: dict[str, Any], root: Path = ROOT) -> None:
    require(value.get("schemaVersion") == 1, "unsupported matrix schema")
    require(value.get("contract") == "p2-gate-matrix-evidence.v1", "wrong matrix contract")
    require(value.get("task") == "TASK-260712-14rxuk", "wrong owner task")
    require(value.get("owner") == "Ivan Oparin", "acceptance owner changed")
    require(value.get("scope") == "engineering-contract-only", "scope changed")
    require(value.get("executionStatus") == "frozen-not-executed", "execution was falsely claimed")
    require(value.get("manualEpic") == "EPIC-260714-th54l3", "manual epic changed")

    production = value.get("productionGate", {})
    require(production.get("status") == "blocked", "current production gate was opened")
    require(production.get("selectedCodecPlayerCombination") is None, "codec/player selected")
    require(production.get("streamedTracksActivationAllowed") is False, "streamed tracks enabled")
    require(production.get("maximumCurrentRolloutStage") == 4, "dark rollout ceiling changed")
    require(len(production.get("reopenRequires", [])) == 4, "replacement ADR gate incomplete")

    anchors = value.get("sourceAnchors", [])
    require(len(anchors) == 7, "source anchor inventory changed")
    for anchor in anchors:
        relative = Path(anchor.get("path", ""))
        require(relative.parts and not relative.is_absolute() and ".." not in relative.parts,
                f"unsafe source anchor: {relative}")
        path = root / relative
        require(path.is_file(), f"missing source anchor: {relative}")
        require(anchor.get("sha256") == digest(path), f"source anchor drift: {relative}")

    player = json.loads((root / "acceptance/codec-spike/player-handoff-v1.json").read_text())
    rollout = json.loads((root / "acceptance/streamed-track-rollout-handoff-v1.json").read_text())
    rubric = json.loads((root / "acceptance/codec-spike/rubric-v1.json").read_text())
    require(player["decision"]["production"] == "no-go", "codec handoff no longer no-go")
    require(player["decision"]["selectedCombination"] is None, "codec handoff selected a combination")
    require(rollout["productionActivation"]["activationAllowedNow"] is False,
            "rollout handoff now allows activation")
    require(rollout["productionActivation"]["currentMaximumRolloutStage"] == 4,
            "rollout handoff stage drift")

    claims = value.get("claimClasses", {})
    require(set(claims) == {"repository-automated-preflight", "manual-final", "beta-final", "forbidden"},
            "claim classes changed")
    require(len(claims.get("forbidden", [])) == 4, "claim boundary weakened")

    clocks = value.get("clocks", {})
    require(clocks.get("latency") == "process monotonic clock", "latency clock changed")
    require(clocks.get("percentileMethod") == "nearest-rank", "percentile method changed")
    require(clocks.get("sampleIntervalMS") == 1000, "resource sampling clock changed")
    require("10 ms" in clocks.get("wallClockSync", ""), "cross-node clock bound changed")
    require("never discarded" in clocks.get("failures", ""), "sample failures may be discarded")

    fixtures = value.get("fixturePack", {})
    require(fixtures.get("rubric") == rubric["contract"], "fixture rubric changed")
    require(fixtures.get("rubricSha256") == digest(root / "acceptance/codec-spike/rubric-v1.json"),
            "fixture rubric hash drift")
    require(fixtures.get("toolchain") == "FFmpeg 8.1.2", "fixture toolchain changed")
    require(fixtures.get("toolchainReleaseSha256") == rubric["fixtureToolchain"]["releaseSha256"],
            "fixture toolchain digest changed")
    require(fixtures.get("longClasses") == [item["id"] for item in rubric["fixtureClasses"]],
            "long fixture class order changed")
    require("current no-go permits no final B1 sample" in fixtures.get("selectionRule", ""),
            "fixture selection bypasses no-go")

    roster = value.get("environmentRoster", {})
    require(roster.get("pairings") == rubric["requiredPairings"], "platform pairing matrix changed")
    require(roster.get("airRealMinimum") == {"barycenters": 3, "pulsars": 5},
            "real Air minimum changed")
    require(roster.get("airSyntheticMaximum") == {"barycenters": 8, "pulsars": 20},
            "synthetic Air maximum changed")
    require([row.get("id") for row in roster.get("windows", [])] ==
            ["windows10_x64", "windows11_x64", "windows11_arm64"], "Windows roster changed")
    require([row.get("id") for row in roster.get("macos", [])] == ["macos14_arm64"],
            "macOS roster changed")

    samples = value.get("samplePlan", {})
    require(samples.get("warmupsPerTimedGroup") == rubric["samplePlan"]["warmupsPerTimedGroup"] == 3,
            "warmup count changed")
    require(samples.get("measuredSamplesPerTimedGroup") ==
            rubric["samplePlan"]["measuredSamplesPerTimedGroup"] == 30, "timed sample count changed")
    require(samples.get("oneHourPlaybackRunsPerPairing") == 1, "one-hour run count changed")
    require(samples.get("twoHourDurationRunsPerPairing") == 1, "two-hour run count changed")
    require(samples.get("airScenarioRepetitions") == 30, "Air repetition count changed")
    require(samples.get("syntheticLoad") == {
        "barycenters": 8, "pulsars": 20, "commandsPerIteration": 20,
        "iterations": 30, "duplicateCommandsAllowed": 0, "lostCommandsAllowed": 0,
    }, "synthetic load rules changed")
    require(samples.get("beta") == {
        "consecutiveWindows": 7, "windowHours": 24,
        "immutableDailyRecords": 7, "criticalIncidentsAllowed": 0,
    }, "seven-day beta rules changed")

    layout = value.get("artifactLayout", {})
    require(layout.get("campaignIdPattern") == "p2-YYYYMMDDTHHMMSSZ-<12hex-root-head>",
            "campaign id contract changed")
    require(layout.get("privateWorkingRoot", "").startswith(".temp/acceptance/phase2/"),
            "private evidence root changed")
    require(len(layout.get("requiredPerRun", [])) == 6, "per-run artifact set changed")
    require("raw participant data and audio are never committed" in layout.get("exportRule", ""),
            "raw evidence export allowed")

    expected_gates = [
        "B1", "B2", "B3", "B4", "B5", "B6", "B7",
        "20.5-track-start", "20.5-start-skew", "20.5-seek", "20.5-memory",
        "20.5-accounting", "20.5-migration", "20.5-scale",
        "17-observability", "18-rollout", "20.6-beta",
    ]
    gates = value.get("gates", {})
    require(list(gates) == expected_gates, "gate inventory or order changed")
    for gate_id, gate in gates.items():
        require(gate.get("status") != "pass", f"manual gate falsely passed: {gate_id}")
        commands = [gate.get(key) for key in ("command", "preflightCommand") if gate.get(key)]
        require(commands, f"gate has no command path: {gate_id}")
        artifacts = [gate.get(key) for key in ("artifact", "preflightArtifact", "finalArtifact") if gate.get(key)]
        require(artifacts, f"gate has no artifact path: {gate_id}")
        for command in commands:
            require(not command.startswith("<"), f"gate command remains a placeholder: {gate_id}")
            script = command_script(command)
            require(script is not None, f"gate command has no repository script: {gate_id}")
            require((root / script).is_file(), f"gate command script missing: {gate_id}: {script}")
        for key in ("manualTask", "manualPreconditionTask", "engineeringTask"):
            task = gate.get(key)
            if task:
                require(board_task(root, task), f"unknown gate owner: {gate_id}: {task}")

    hard = {item["metric"]: item for item in rubric["hardGates"]}
    threshold_map = {
        "20.5-track-start": "track_start_ms",
        "20.5-start-skew": "scheduled_skew_ms",
        "20.5-seek": "seek_to_audio_ms",
        "20.5-memory": "peak_rss_mib",
    }
    for gate_id, metric in threshold_map.items():
        threshold = gates[gate_id]["threshold"]
        source = hard[metric]
        require(threshold == {key: source[key] for key in ("metric", "method", "limit", "samples", "warmups")},
                f"hard gate drift: {gate_id}")

    manual_tasks = {
        gate.get(key) for gate in gates.values()
        for key in ("manualTask", "manualPreconditionTask") if gate.get(key)
    }
    require(manual_tasks == {
        "TASK-260712-1fpb9q", "TASK-260712-2bdi4a", "TASK-260712-21kz3b", "TASK-260712-3u5cdn",
        "TASK-260712-3qybi2", "TASK-260712-2pnc5a",
    }, "manual gate ownership changed")
    for task in manual_tasks:
        paths = board_task(root, task)
        require(len(paths) == 1 and "EPIC-260714-th54l3" in paths[0].as_posix(),
                f"manual task is not routed exclusively to manual epic: {task}")

    incidents = value.get("criticalIncidentRubric", {})
    require(len(incidents.get("critical", [])) == 7, "critical incident inventory changed")
    require("beta restarts at day one" in incidents.get("action", ""), "critical reset rule weakened")
    require("same restart rule" in incidents.get("unapprovedChange", ""), "unapproved change reset removed")

    privacy = value.get("privacy", {})
    require(privacy.get("consentOwner") == "Ivan Oparin", "consent owner changed")
    require(privacy.get("requiredBeforeManualRun") is True, "participant consent no longer required")
    require(privacy.get("sanitizationRequired") is True, "sanitization disabled")
    require(set(privacy.get("forbiddenArtifacts", [])) == {
        "audio bytes", "original filename", "caption", "transcript", "bearer token",
        "raw actor id", "raw chat id", "email", "local filesystem path",
    }, "privacy denylist changed")

    blockers = value.get("blockers", [])
    require([item.get("id") for item in blockers] == [
        "codec-player-no-go", "real-lab-hardware-roster", "participant-consent-roster",
        "production-credentials-and-feature-flag-authority", "phase2-observability-export",
    ], "blocker inventory changed")
    require(all(item.get("status") != "closed" for item in blockers), "unresolved blocker was closed")

    template = json.loads((root / TEMPLATE.relative_to(ROOT)).read_text(encoding="utf-8"))
    require(template.get("contract") == "p2-gate-result.v1", "wrong result template")
    require(template.get("gateContract") == value["contract"], "result template points elsewhere")
    require(template.get("status") == "not-run", "result template claims execution")
    require(template.get("artifacts") == [] and template.get("samples") == [],
            "result template contains invented evidence")

    handoff = root / HANDOFF.relative_to(ROOT)
    require(handoff.is_file(), "human-readable gate matrix missing")
    text = handoff.read_text(encoding="utf-8")
    for heading in (
        "## Claim boundary and current blocker", "## Provenance, fixtures and clocks",
        "## Platform, topology and sample roster", "## B1-B7 evidence map",
        "## Sections 17, 18, 20.5 and 20.6", "## Artifact layout and privacy",
        "## Explicit blockers and downstream handoff",
    ):
        require(heading in text, f"handoff heading missing: {heading}")
    for gate_id in expected_gates:
        require(f"`{gate_id}`" in text, f"handoff omits gate: {gate_id}")


def main() -> None:
    value = load()
    validate(value)
    print(json.dumps({
        "status": "pass", "contract": value["contract"],
        "executionStatus": value["executionStatus"], "gates": len(value["gates"]),
        "manualTasks": len({gate.get(key) for gate in value["gates"].values()
                            for key in ("manualTask", "manualPreconditionTask") if gate.get(key)}),
        "productionGate": value["productionGate"]["status"],
    }, sort_keys=True))


if __name__ == "__main__":
    main()
