#!/usr/bin/env python3
"""Validate the frozen Phase 3 gate contract without claiming execution."""

from __future__ import annotations

import hashlib
import itertools
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
MATRIX = ROOT / "acceptance/phase3/gate-matrix-v1.json"
TEMPLATE = ROOT / "acceptance/phase3/result-template-v1.json"
HANDOFF = ROOT / "docs/acceptance/phase3-gate-matrix.md"


class Phase3GateMatrixError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise Phase3GateMatrixError(message)


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: Path = MATRIX) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def board_task(root: Path, task: str) -> list[Path]:
    return list((root / ".task-board").glob(f"**/{task}_*/progress.md"))


def scripts_in(command: str) -> list[str]:
    return [part for part in command.split() if part.endswith(".py") and part.startswith("scripts/")]


def validate(value: dict[str, Any], root: Path = ROOT) -> None:
    require(value.get("schemaVersion") == 1, "unsupported matrix schema")
    require(value.get("contract") == "p3-gate-matrix-evidence.v1", "wrong matrix contract")
    require(value.get("task") == "TASK-260712-3da0vz", "wrong owner task")
    require(value.get("owner") == "Ivan Oparin", "acceptance owner changed")
    require(value.get("scope") == "engineering-contract-only", "scope changed")
    require(value.get("executionStatus") == "frozen-not-executed", "execution was falsely claimed")
    require(value.get("manualEpic") == "EPIC-260714-th54l3", "manual epic changed")

    anchors = value.get("sourceAnchors", [])
    require(len(anchors) == 9, "source anchor inventory changed")
    for anchor in anchors:
        relative = Path(anchor.get("path", ""))
        require(relative.parts and not relative.is_absolute() and ".." not in relative.parts,
                f"unsafe source anchor: {relative}")
        path = root / relative
        require(path.is_file(), f"missing source anchor: {relative}")
        require(anchor.get("sha256") == digest(path), f"source anchor drift: {relative}")

    claims = value.get("claimClasses", {})
    require(set(claims) == {
        "repositoryEngineering", "manualFinal", "independentReview", "betaFinal", "forbidden",
    }, "claim classes changed")
    require(len(claims.get("forbidden", [])) == 4, "claim boundary weakened")

    provenance = value.get("provenance", {})
    require(len(provenance.get("required", [])) == 10, "provenance inventory changed")
    require("one root commit" in provenance.get("sameBuildRule", ""), "same-build rule weakened")
    require("no averaging" in provenance.get("failureRule", ""), "failure denominator weakened")

    fixtures = value.get("fixturePack", {})
    capture_lock = root / fixtures.get("captureLock", "")
    require(capture_lock.is_file(), "capture fixture lock missing")
    require(fixtures.get("captureLockSHA256") == digest(capture_lock), "capture fixture lock drift")
    for key in ("liveModel", "e2eeThreatModel", "e2eeLifecycle", "automationEngineeringEvidence"):
        require((root / fixtures.get(key, "")).is_file(), f"fixture source missing: {key}")

    roster = value.get("environmentRoster", {})
    require(roster.get("directedPlatformPairings") == [
        "windows_windows", "windows_macos", "macos_windows", "macos_macos",
    ], "directed platform pairing matrix changed")
    require([row.get("id") for row in roster.get("windows", [])] == [
        "windows10_x64", "windows11_x64", "windows11_arm64",
    ], "Windows roster changed")
    require([row.get("id") for row in roster.get("macos", [])] == ["macos14_arm64"],
            "macOS roster changed")
    require(roster.get("captureRoutes") == ["speaker", "headphone", "unknown"],
            "capture route roster changed")
    require(roster.get("captureWorkflows") == ["recorded_clip", "local_self_test", "live_ptt"],
            "capture workflow roster changed")
    require(roster.get("realHomeMinimum") == {
        "homes": 2, "participants": 3, "distinctInternetConnections": 2,
    }, "real-home minimum changed")
    require(roster.get("status") == "missing-real-roster", "real roster was invented")

    order = value.get("featureFlagOrder")
    require(order == ["live_ptt", "e2ee_media", "soundboard_cues", "automation"],
            "feature flag order changed")
    matrix = value.get("featureFlagMatrix", [])
    require(len(matrix) == 16, "all sixteen feature-flag permutations are required")
    expected_ids = ["".join(str(bit) for bit in bits) for bits in itertools.product((0, 1), repeat=4)]
    require([row.get("id") for row in matrix] == expected_ids, "feature-flag permutations changed")
    for row in matrix:
        bits = [bool(row.get(flag)) for flag in order]
        require(row["id"] == "".join("1" if bit else "0" for bit in bits),
                f"feature-flag id mismatch: {row.get('id')}")
        invalid = row["automation"] and not row["soundboard_cues"]
        require((row.get("classification") == "invalid-automation-requires-soundboard") == invalid,
                f"automation dependency classification changed: {row['id']}")

    capabilities = value.get("promotionCapabilities", {})
    require(list(capabilities) == ["live_ptt", "e2ee_media", "soundboard_cues", "automation"],
            "separate promotion capabilities changed")
    require(all(item.get("status") != "ready" for item in capabilities.values()),
            "capability was falsely marked ready")
    require(capabilities["e2ee_media"]["status"] == "blocked-by-deferred-e2ee-and-review",
            "E2EE owner gate was removed")

    layout = value.get("artifactLayout", {})
    require(layout.get("campaignIdPattern") == "p3-YYYYMMDDTHHMMSSZ-<12hex-root-head>",
            "campaign id contract changed")
    require(layout.get("privateWorkingRoot", "").startswith(".temp/acceptance/phase3/"),
            "private evidence root changed")
    require(len(layout.get("requiredPerRun", [])) == 6, "per-run artifact set changed")
    require("never committed" in layout.get("exportRule", ""), "raw evidence export allowed")
    require(layout.get("retention", {}).get("privateRawDays") == 30, "private retention changed")

    expected_gates = [
        "C1", "C2", "C3", "C4", "C5", "C6", "C7",
        "NF-jitter", "NF-reconnect", "NF-secure-storage",
        "NF-external-security-review", "NF-root-review", "NF-realtime-review",
        "NF-automation-review", "NF-privacy-store-review", "NF-migration-recovery-review",
        "NF-disclosures", "NF-rollout-recovery", "NF-beta",
    ]
    gates = value.get("gates", {})
    require(list(gates) == expected_gates, "gate inventory or order changed")
    for gate_id, gate in gates.items():
        require("pass" not in gate.get("status", ""), f"gate falsely passed: {gate_id}")
        require(gate.get("preflightCommand"), f"gate command missing: {gate_id}")
        require(gate.get("labPath"), f"gate lab/review path missing: {gate_id}")
        require(gate.get("finalArtifact"), f"gate artifact missing: {gate_id}")
        for script in scripts_in(gate["preflightCommand"]):
            require((root / script).is_file(), f"gate command script missing: {gate_id}: {script}")
        require(scripts_in(gate["preflightCommand"]), f"gate command is not repository-backed: {gate_id}")
        for key in ("manualTask", "manualPreconditionTask", "engineeringTask", "reviewTask"):
            if task := gate.get(key):
                require(board_task(root, task), f"unknown gate owner: {gate_id}: {task}")

    manual_tasks = {
        gate.get(key) for gate in gates.values()
        for key in ("manualTask", "manualPreconditionTask") if gate.get(key)
    }
    require(manual_tasks == {
        "TASK-260712-flaiie", "TASK-260712-2e80pr", "TASK-260712-yj668d",
        "TASK-260712-1gyohk", "TASK-260712-30xwu2", "TASK-260712-1actom",
    }, "manual gate ownership changed")
    for task in manual_tasks:
        paths = board_task(root, task)
        require(len(paths) == 1 and "EPIC-260714-th54l3" in paths[0].as_posix(),
                f"manual task is not routed exclusively to manual epic: {task}")

    reviews = value.get("reviewRoster", [])
    require([item.get("role") for item in reviews] == [
        "root-line-review", "external-crypto-implementation", "independent-realtime",
        "independent-automation", "independent-privacy-store", "independent-migration-recovery",
    ], "review roster changed")
    require(reviews[0].get("owner") == "Ivan Oparin" and reviews[0].get("independent") is False,
            "root reviewer ownership changed")
    require(all(item.get("owner") is None and item.get("status") == "missing-input"
                for item in reviews[1:]), "independent reviewers were invented")

    approved = value.get("approvedInputs", {})
    require(approved.get("operator") == "Relux Works", "approved operator changed")
    require(approved.get("accountableOwner") == "Ivan Oparin", "accountable owner changed")
    require(approved.get("supportMailbox") == "support@barycenter.live", "support mailbox changed")
    require(approved.get("moderationMailbox") == "moderator@barycenter.live",
            "moderation mailbox changed")
    require(approved.get("finalSubmitAuthority") == "Ivan Oparin", "submit authority changed")
    require(approved.get("privacyURL") == "https://barycenter.live/legal/privacy",
            "privacy URL changed")
    require(approved.get("legalCounselReviewRequired") is False,
            "approved legal counsel decision changed")
    require(approved.get("moderationCoverage") == "Monday-Friday 10:00-19:00 GMT+4",
            "moderation coverage changed")

    beta = value.get("beta", {})
    require(beta.get("consecutiveDays") == 7 and beta.get("immutableDailyRecords") == 7,
            "seven-day beta changed")
    require(len(beta.get("prohibitedIncidents", [])) == 5, "beta incident rubric changed")
    require(len(beta.get("resetTriggers", [])) == 5, "beta reset triggers changed")
    require("restart at day one" in beta.get("resetAction", ""), "beta reset rule weakened")

    privacy = value.get("privacy", {})
    require(privacy.get("consentOwner") == "Ivan Oparin", "consent owner changed")
    require(privacy.get("requiredBeforeManualRun") is True, "participant consent removed")
    require(privacy.get("sanitizationRequired") is True, "sanitization disabled")
    require(set(privacy.get("forbiddenArtifacts", [])) == {
        "audio bytes", "traffic payload", "key material", "original filename", "transcript",
        "bearer token", "raw actor id", "raw chat id", "email", "local filesystem path",
    }, "privacy denylist changed")

    blockers = value.get("blockers", [])
    require([item.get("id") for item in blockers] == [
        "real-windows-macos-audio-hardware-roster",
        "two-home-network-and-beta-participant-roster",
        "deferred-e2ee-design-implementation-and-review",
        "independent-domain-reviewers",
        "public-policy-hosting-mail-delivery-and-store-record", "phase3-observability-export",
    ], "blocker inventory changed")
    require(all(item.get("status") not in ("closed", "pass") for item in blockers),
            "unresolved blocker was closed")

    template = json.loads((root / TEMPLATE.relative_to(ROOT)).read_text(encoding="utf-8"))
    require(template.get("contract") == "p3-gate-result.v1", "wrong result template")
    require(template.get("gateContract") == value["contract"], "result template points elsewhere")
    require(template.get("status") == "not-run" and template.get("manualEvidence") == "not-run",
            "result template claims execution")
    require(template.get("artifacts") == [] and template.get("samples") == [],
            "result template contains invented evidence")

    handoff = root / HANDOFF.relative_to(ROOT)
    require(handoff.is_file(), "human-readable gate matrix missing")
    text = handoff.read_text(encoding="utf-8")
    for heading in (
        "## Claim boundary", "## Provenance and fixtures", "## Environment and flag roster",
        "## C1-C7 map", "## Section 21.4 and exit gates", "## Artifact layout and privacy",
        "## Approved inputs and explicit blockers", "## Downstream execution rule",
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
        "featureFlagPermutations": len(value["featureFlagMatrix"]),
        "openBlockers": len(value["blockers"]),
    }, sort_keys=True))


if __name__ == "__main__":
    main()
