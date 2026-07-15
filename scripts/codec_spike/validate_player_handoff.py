#!/usr/bin/env python3
"""Fail-closed validation for the codec/player no-go ADR handoff."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance" / "codec-spike" / "player-handoff-v1.json"
EXPECTED_COMBINATIONS = {
    "bundled-ffmpeg-both-platforms",
    "native-mf-plus-avfoundation",
    "pure-go-both-platforms",
}


class HandoffError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise HandoffError(message)


def load(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate(contract: dict[str, Any]) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported handoff schema")
    require(contract.get("contract") == "p2-codec-player-adr-handoff.v1", "wrong handoff contract")

    inputs = contract.get("frozenInputs", [])
    require({row.get("id") for row in inputs} == {
        "comparative-matrix", "stream-contract", "rubric", "license-audit"
    }, "frozen inputs are incomplete")
    source: dict[str, dict[str, Any]] = {}
    for row in inputs:
        path = ROOT / row["path"]
        require(path.is_file(), f"missing frozen input: {row['id']}")
        require(row.get("sha256") == digest(path), f"frozen input digest mismatch: {row['id']}")
        source[row["id"]] = load(path)

    matrix = source["comparative-matrix"]
    stream = source["stream-contract"]
    rubric = source["rubric"]
    license_audit = source["license-audit"]
    decision = contract.get("decision", {})
    require(matrix["selection"]["allowed"] is False, "source matrix unexpectedly permits selection")
    require(decision == {
        "production": "no-go",
        "selectedCombination": None,
        "selectedCodec": None,
        "selectedContainer": None,
        "phase2ProductionBlocked": True,
        "reason": matrix["selection"]["decision"],
        "sourceMatrixSelectionAllowed": False,
    }, "ADR decision does not preserve the no-go matrix result")

    mode = contract.get("downstreamMode", {})
    require(mode.get("engineeringMayProceed") is True, "candidate-neutral engineering must remain possible")
    for key in (
        "productionMediaGenerationAllowed", "productionPlaybackAllowed",
        "storeSubmissionAllowed", "decoderFallbackAllowed",
        "runtimeExecutableDownloadAllowed", "sandboxWeakeningAllowed",
        "unknownCodecAcceptanceAllowed",
    ):
        require(mode.get(key) is False, f"no-go escape enabled: {key}")

    selection = contract.get("variantSelection", {})
    require(selection.get("productionSelection").startswith("disabled-"), "production variant selection must be disabled")
    require(selection.get("allowedCandidateCodecsForSchemaAndTestsOnly") == stream["candidateInputs"]["codecs"],
            "candidate codec enum drifted")
    require(selection.get("allowedCandidateContainersForSchemaAndTestsOnly") == stream["candidateInputs"]["containers"],
            "candidate container enum drifted")
    require(selection.get("storageKeyFormat") == stream["storagePolicy"]["storageKeyFormat"], "storage key drifted")

    wire = contract.get("wire", {})
    for key in (
        "path", "methods", "authorization", "credentialsInUrl", "singleRangeOnly",
        "successStatuses", "missingUnauthorizedRevokedStatus", "etag", "ifRangeMismatch", "cacheControl",
    ):
        require(wire.get(key) == stream["http"][key], f"wire contract drifted: {key}")
    integrity = contract.get("integrity", {})
    for key in (
        "algorithm", "chunkSizeBytes", "wholeObjectRequired", "decodeBeforeIntegrity",
        "mixedEtagAction", "chunkMismatchAction",
    ):
        require(integrity.get(key) == stream["integrity"][key], f"integrity contract drifted: {key}")
    cache = contract.get("cache", {})
    for key in (
        "scope", "globalCeilingBytes", "perVariantCeilingBytes", "pinnedCeilingBytes",
        "maximumChunkBytes", "maximumNetworkReadBytes", "eviction", "cacheKey", "deleteOrDisableAction",
    ):
        require(cache.get(key) == stream["cache"][key], f"cache contract drifted: {key}")

    pcm = contract.get("pcm", {})
    require({key: pcm.get(key) for key in ("sampleRateHz", "channels", "sampleFormat")} ==
            {key: rubric["decodedPCM"][key] for key in ("sampleRateHz", "channels", "sampleFormat")},
            "PCM contract drifted")
    require(pcm.get("ringCeilingBytes") == 1048576, "PCM ring ceiling changed")
    adapter = contract.get("decoderAdapter", {})
    require(adapter.get("productionImplementations") == [], "no-go handoff registered a production decoder")
    require(adapter.get("ownsNetwork") is False and adapter.get("ownsDiskCache") is False,
            "decoder crossed the transport/cache boundary")
    require(adapter.get("renderCallbackCallsDecoder") is False
            and adapter.get("renderCallbackMayAllocateWaitOrLock") is False,
            "render callback safety drifted")

    scheduler = contract.get("scheduler", {})
    limits = {row["metric"]: row["limit"] for row in rubric["hardGates"]}
    require(scheduler.get("scheduledSkewP95LimitMS") == limits["scheduled_skew_ms"], "skew limit drifted")
    require(scheduler.get("trackStartP95LimitMS") == limits["track_start_ms"], "start limit drifted")
    require(scheduler.get("seekToAudioP95LimitMS") == limits["seek_to_audio_ms"], "seek limit drifted")

    fixtures = contract.get("fixtures", [])
    require([row.get("id") for row in fixtures] == rubric["smokeFixtures"], "fixture order or set drifted")
    for row in fixtures:
        path = ROOT / row["path"]
        require(path.is_file() and row.get("sha256") == digest(path), f"fixture digest mismatch: {row.get('id')}")

    evidence = contract.get("evidenceRequirements", {})
    require(evidence.get("requiredPairings") == rubric["requiredPairings"], "pairing evidence drifted")
    require(evidence.get("rangeProfiles") == rubric["rangeProfiles"], "range profiles drifted")
    require(evidence.get("timedWarmupsPerGroup") == rubric["samplePlan"]["warmupsPerTimedGroup"], "warmup count drifted")
    require(evidence.get("timedSamplesPerGroup") == rubric["samplePlan"]["measuredSamplesPerTimedGroup"], "sample count drifted")
    require(evidence.get("scoreAveragingAllowed") is False, "score averaging must remain forbidden")
    require(evidence.get("manualEpic") == rubric["claimBoundary"]["manualEpic"], "manual boundary drifted")
    require(contract.get("releaseObligations") == license_audit["releaseGates"], "release obligations drifted")
    require({row.get("id") for row in contract.get("rejectedCombinations", [])} == EXPECTED_COMBINATIONS,
            "rejected combination history is incomplete")
    require(len(contract.get("reopenConditions", [])) >= 5, "reopen conditions are incomplete")
    require(set(contract.get("consumers", {})) == {"STORY-260712-2ori1t", "STORY-260712-1qfbiw"},
            "downstream handoff consumers changed")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("path", nargs="?", type=Path, default=CONTRACT_PATH)
    args = parser.parse_args()
    contract = load(args.path)
    validate(contract)
    print(json.dumps({
        "status": "pass",
        "contract": contract["contract"],
        "production": contract["decision"]["production"],
        "engineeringMode": contract["downstreamMode"]["mode"],
        "fixtures": len(contract["fixtures"]),
        "productionImplementations": len(contract["decoderAdapter"]["productionImplementations"]),
    }, sort_keys=True))


if __name__ == "__main__":
    main()
