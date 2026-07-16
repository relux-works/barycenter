#!/usr/bin/env python3
"""Validate the executable B5-B7 regression evidence map without claiming manual proof."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "acceptance" / "targets-inbox-parity-regressions-v1.json"


class EvidenceError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise EvidenceError(message)


def load(path: Path = MANIFEST) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(value: dict[str, Any], root: Path = ROOT) -> None:
    require(value.get("schemaVersion") == 1, "unsupported evidence schema")
    require(value.get("contract") == "p2-targets-inbox-parity-regressions.v1", "wrong evidence contract")
    require(value.get("sourceContract") == "p2-targets-inbox-parity.v1", "wrong source contract")
    require(value.get("scope") == "repository-automated-only", "manual evidence was promoted")

    required = value.get("requiredInvariants", [])
    require(len(required) == len(set(required)) and len(required) >= 19, "required invariant inventory is incomplete")
    evidence = value.get("evidence", [])
    evidence_ids = [item.get("id") for item in evidence]
    require(len(evidence_ids) == len(set(evidence_ids)) and len(evidence_ids) >= 9, "evidence IDs are missing or duplicated")
    covered: set[str] = set()
    for item in evidence:
        invariants = item.get("invariants", [])
        require(invariants and set(invariants) <= set(required), f"unknown invariant in {item.get('id')}")
        covered.update(invariants)
        anchors = item.get("anchors", [])
        require(anchors, f"evidence {item.get('id')} has no executable anchors")
        for anchor in anchors:
            relative = Path(anchor.get("path", ""))
            require(relative.parts and not relative.is_absolute() and ".." not in relative.parts,
                    f"unsafe evidence path: {relative}")
            path = root / relative
            require(path.is_file(), f"missing evidence source: {relative}")
            symbol = anchor.get("symbol", "")
            require(symbol and symbol in path.read_text(encoding="utf-8"),
                    f"missing evidence symbol {symbol} in {relative}")
    require(covered == set(required), f"uncovered invariants: {sorted(set(required) - covered)}")

    fixture = value.get("sharedSurfaceFixture", {})
    require(fixture.get("surfaceStates") == ["loading", "ready", "stale", "offline", "coordinator_error"],
            "surface state fixture changed")
    require(fixture.get("audiences") == ["this_pulsar", "own_barycenter", "current_air", "explicit"],
            "audience fixture changed")
    require(len(fixture.get("commands", [])) == 13 and len(set(fixture.get("commands", []))) == 13,
            "command fixture is incomplete")
    require(fixture.get("canonicalOutcomes") == [
        "replay_accepted", "inbox_dismissed", "media_deleted", "report_received", "sender_blocked"
    ], "canonical outcomes diverged")
    require(fixture.get("targetedTrackPolicy") == "unsupported", "streamed-track work was claimed early")
    require(fixture.get("manualReplayRequired") is True and fixture.get("lateAutoplayAllowed") is False,
            "manual replay boundary changed")
    require(set(fixture.get("opaquePrefixesNeverRendered", [])) == {"trf_", "ib_", "hi_", "ic_", "hc_", "rc_"},
            "opaque rendering fixture changed")

    gates = value.get("gates", {})
    require(set(gates) == {"B5", "B6", "B7"}, "B5-B7 gate map is incomplete")
    for gate, mapping in gates.items():
        require(set(mapping.get("evidence", [])) <= set(evidence_ids), f"{gate} references unknown evidence")
        require(mapping.get("manualTask") == "TASK-260712-3u5cdn", f"{gate} manual boundary changed")
        require("repository" in mapping.get("claim", "") or gate == "B6", f"{gate} overclaims evidence")

    manual = value.get("manualEvidence", [])
    require(len(manual) == 1, "manual evidence routing changed")
    boundary = manual[0]
    require(boundary.get("task") == "TASK-260712-3u5cdn" and
            boundary.get("epic") == "EPIC-260714-th54l3" and
            boundary.get("status") == "manual-required", "manual task was claimed or rerouted")
    require(len(boundary.get("claims", [])) >= 6, "manual evidence inventory is incomplete")


def main() -> None:
    value = load()
    validate(value)
    print(json.dumps({
        "status": "pass",
        "contract": value["contract"],
        "scope": value["scope"],
        "gates": sorted(value["gates"]),
        "invariants": len(value["requiredInvariants"]),
        "manualStatus": value["manualEvidence"][0]["status"],
    }, sort_keys=True))


if __name__ == "__main__":
    main()
