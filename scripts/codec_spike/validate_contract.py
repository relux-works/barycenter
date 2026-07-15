#!/usr/bin/env python3
"""Fail-closed validation for the frozen Phase 2 codec-spike rubric."""

from __future__ import annotations

import json
import pathlib
import re


ROOT = pathlib.Path(__file__).resolve().parents[2]
RUBRIC_PATH = ROOT / "acceptance" / "codec-spike" / "rubric-v1.json"
TEMPLATE_PATH = ROOT / "acceptance" / "templates" / "codec-spike-evidence-v1.json"


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def load() -> dict:
    with RUBRIC_PATH.open(encoding="utf-8") as stream:
        return json.load(stream)


def validate(rubric: dict) -> None:
    require(rubric.get("schemaVersion") == 1, "unsupported rubric schema")
    require(rubric.get("contract") == "p2-codec-spike-rubric.v1", "wrong contract id")
    require(len(rubric.get("sourceSections", [])) == 3, "source mapping must cover 20.2, B1 and 20.5")
    require(rubric.get("requiredPairings") == [
        "windows_windows", "windows_macos", "macos_macos"
    ], "all three platform pairings must be frozen")
    require(rubric.get("packageArchitectures") == {
        "windowsBuild": ["amd64", "arm64"],
        "macosBuild": ["arm64"],
        "minimumRealTimingMatrix": ["Windows x64 packaged node", "Apple-silicon macOS packaged node"],
    }, "package architecture matrix changed")

    toolchain = rubric.get("fixtureToolchain", {})
    require(toolchain.get("version") == "8.1.2", "fixture encoder version moved")
    require(toolchain.get("releaseSha256") ==
            "464beb5e7bf0c311e68b45ae2f04e9cc2af88851abb4082231742a74d97b524c",
            "fixture encoder source digest moved")
    require(toolchain.get("signatureSha256") ==
            "0a0963fccd70597838073f3e31b20f4a4d8cc2b5e577472c9a5a1f22624246f8",
            "fixture encoder signature digest moved")
    require(re.fullmatch(r"[0-9A-F]{40}", toolchain.get("signingKeyFingerprint", "")) is not None,
            "fixture toolchain signing fingerprint invalid")

    gates = {gate.get("metric"): gate for gate in rubric.get("hardGates", [])}
    expected = {
        "track_start_ms": (5000, "nearest-rank-p95", 30),
        "seek_to_audio_ms": (3000, "nearest-rank-p95", 30),
        "scheduled_skew_ms": (100, "nearest-rank-p95", 30),
        "peak_rss_mib": (200, "maximum", 1),
        "duration_rss_growth_mib": (16, "maximum", 1),
        "duration_rss_slope_mib_per_hour": (1, "absolute-maximum", 1),
    }
    require(set(gates) == set(expected), "hard-gate inventory changed")
    for metric, (limit, method, samples) in expected.items():
        gate = gates[metric]
        require((gate.get("limit"), gate.get("method"), gate.get("samples")) ==
                (limit, method, samples), f"{metric} gate changed")
        require(gate.get("groupBy"), f"{metric} lacks aggregation dimensions")

    fixture_classes = rubric.get("fixtureClasses", [])
    ids = [item.get("id") for item in fixture_classes]
    require(len(ids) == len(set(ids)) == 6, "long fixture ids must be unique and complete")
    require({item.get("codec") for item in fixture_classes} == {"mp3", "aac-lc", "opus"},
            "MP3, AAC-LC and Opus are mandatory")
    for codec in ("mp3", "aac-lc", "opus"):
        durations = {item.get("durationSeconds") for item in fixture_classes if item.get("codec") == codec}
        require(durations == {3600, 7200}, f"{codec} must have one-hour and two-hour fixtures")
        require(any(item.get("seekHeavy") for item in fixture_classes if item.get("codec") == codec),
                f"{codec} lacks a seek-heavy fixture")

    smoke = rubric.get("smokeFixtures", [])
    hostile = rubric.get("hostileFixtures", [])
    require(len(smoke) == len(set(smoke)) == 6, "smoke corpus changed")
    require(len(hostile) == len({item.get("id") for item in hostile}) == 8,
            "hostile corpus changed")
    require(all(item.get("base") in smoke for item in hostile), "hostile fixture has an unknown base")

    profiles = rubric.get("rangeProfiles", [])
    require(profiles == [
        "normal", "no_range", "slow_256kbit", "reset_mid_body", "truncate_body",
        "corrupt_chunk", "etag_flip", "revoked"
    ], "range fault profiles changed")
    require(rubric.get("requiredArtifactKinds") == [
        "fixture-lock", "range-requests", "metric-samples", "rss-series", "package-receipts", "sbom"
    ], "required evidence artifacts changed")
    candidates = rubric.get("candidates", [])
    require([item.get("id") for item in candidates] == [
        "native-canonical-aac-v1", "pure-go-composite-v1", "bundled-ffmpeg-8.1.2-v1"
    ], "candidate shortlist changed")
    require(all(item.get("firstRunExecutableDownload") is False for item in candidates),
            "a candidate permits runtime executable download")

    proof_requirements = {item.get("requirement") for item in rubric.get("proofMap", [])}
    for phrase in (
        "decode MP3/AAC/Opus", "range/chunk fetch", "pause/seek/resume",
        "bounded memory on two-hour media", "scheduled start", "Store/AppContainer compatibility",
        "license suitability", "B1 one-hour start before full download", "20.5 start/seek/skew/RSS",
    ):
        require(phrase in proof_requirements, f"proof map missing {phrase}")

    with TEMPLATE_PATH.open(encoding="utf-8") as stream:
        template = json.load(stream)
    require(template.get("rubric") == rubric["contract"], "evidence template uses another rubric")
    require(template.get("candidateId") == "unmeasured", "template must not claim a candidate")
    require(template.get("coverage") == {"pairings": [], "fixtureIds": [], "rangeProfiles": []},
            "template must remain visibly unmeasured")
    require(template.get("artifacts") == [], "template must not contain invented artifacts")


def main() -> int:
    validate(load())
    print(f"codec spike contract valid: {RUBRIC_PATH.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
