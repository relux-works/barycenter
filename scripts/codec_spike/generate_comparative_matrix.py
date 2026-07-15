#!/usr/bin/env python3
"""Build the fail-closed codec/player comparison from pinned probe artifacts."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / "acceptance" / "codec-spike" / "comparative-matrix-v1.json"
MANUAL_EPIC = "EPIC-260714-th54l3"
PAIRINGS = ["windows_windows", "windows_macos", "macos_macos"]

SOURCES = {
    "rubric": "acceptance/codec-spike/rubric-v1.json",
    "stream-contract": "acceptance/codec-spike/stream-contract-v1.json",
    "license-audit": "acceptance/codec-spike/license-audit-v1.json",
    "bundled-macos-evidence": ".task-board/.resources/TASK-260712-1canzv/decode-evidence-macos-arm64.json",
    "bundled-windows-evidence": ".task-board/.resources/TASK-260712-1canzv/decode-evidence-windows-amd64.json",
    "bundled-macos-receipt": ".task-board/.resources/TASK-260712-1canzv/receipt-macos-arm64.json",
    "bundled-windows-receipt": ".task-board/.resources/TASK-260712-1canzv/receipt-windows-amd64.json",
    "media-foundation-receipt": ".task-board/.resources/TASK-260712-298tyq/receipt-windows-media-foundation.json",
    "native-macos-evidence": ".task-board/.resources/TASK-260712-350u8d/evidence-macos-arm64.json",
    "native-macos-receipt": ".task-board/.resources/TASK-260712-350u8d/receipt-macos-arm64.json",
    "pure-go-macos-evidence": ".task-board/.resources/TASK-260712-3vkcki/evidence-macos-arm64.json",
    "pure-go-windows-evidence": ".task-board/.resources/TASK-260712-3vkcki/evidence-windows-amd64.json",
    "pure-go-macos-receipt": ".task-board/.resources/TASK-260712-3vkcki/receipt-macos-arm64.json",
    "pure-go-windows-receipt": ".task-board/.resources/TASK-260712-3vkcki/receipt-windows-amd64.json",
}


def read_json(path: str) -> dict[str, Any]:
    return json.loads((ROOT / path).read_text(encoding="utf-8"))


def digest(path: str) -> str:
    return hashlib.sha256((ROOT / path).read_bytes()).hexdigest()


def artifact_index() -> list[dict[str, str]]:
    return [
        {"id": source_id, "path": path, "sha256": digest(path)}
        for source_id, path in SOURCES.items()
    ]


def gate(gate_id: str, status: str, evidence: list[str], reason: str) -> dict[str, Any]:
    row: dict[str, Any] = {
        "id": gate_id,
        "status": status,
        "evidence": evidence,
        "reason": reason,
    }
    if status == "not-run":
        row["manualEpic"] = MANUAL_EPIC
    return row


def pairings(blockers: list[str]) -> list[dict[str, Any]]:
    platforms = {
        "windows_windows": ["windows", "windows"],
        "windows_macos": ["windows", "macos"],
        "macos_macos": ["macos", "macos"],
    }
    return [
        {
            "id": pairing,
            "platforms": platforms[pairing],
            "status": "rejected",
            "blockingGates": blockers,
        }
        for pairing in PAIRINGS
    ]


def build_matrix() -> dict[str, Any]:
    rubric = read_json(SOURCES["rubric"])
    bundled_mac = read_json(SOURCES["bundled-macos-evidence"])
    bundled_win = read_json(SOURCES["bundled-windows-evidence"])
    mf = read_json(SOURCES["media-foundation-receipt"])["evidence"]
    native_mac = read_json(SOURCES["native-macos-evidence"])
    pure_mac = read_json(SOURCES["pure-go-macos-evidence"])
    pure_win = read_json(SOURCES["pure-go-windows-evidence"])
    fixtures = rubric["smokeFixtures"]

    bundled_mac_rows = {row["fixture"]: row for row in bundled_mac["decodeResults"]}
    bundled_win_rows = {row["fixture"]: row for row in bundled_win["decodeResults"]}
    mf_rows = {row["id"]: row for row in mf["fixtures"]}
    native_mac_rows = {row["id"]: row for row in native_mac["fixtures"]}
    pure_mac_rows = {row["id"]: row for row in pure_mac["fixtures"]}
    pure_win_rows = {row["id"]: row for row in pure_win["fixtures"]}

    bundled_formats = [
        {
            "fixtureId": fixture,
            "windowsOutcome": "decode" if bundled_win_rows[fixture]["full"]["errorCode"] == 0 else "reject",
            "macosOutcome": "decode" if bundled_mac_rows[fixture]["full"]["errorCode"] == 0 else "reject",
            "formatPass": bundled_win_rows[fixture]["full"]["errorCode"] == 0
            and bundled_mac_rows[fixture]["full"]["errorCode"] == 0,
        }
        for fixture in fixtures
    ]
    native_formats = [
        {
            "fixtureId": fixture,
            "windowsOutcome": (
                "decode" if mf_rows[fixture]["expected"] == "decode"
                else f"reject:{mf_rows[fixture]['openHRESULT']}"
            ),
            "macosOutcome": native_mac_rows[fixture]["outcome"],
            "formatPass": mf_rows[fixture]["expected"] == "decode"
            and native_mac_rows[fixture]["outcome"] == "decode",
        }
        for fixture in fixtures
    ]
    pure_formats = [
        {
            "fixtureId": fixture,
            "windowsOutcome": pure_win_rows[fixture]["outcome"],
            "macosOutcome": pure_mac_rows[fixture]["outcome"],
            "formatPass": pure_win_rows[fixture]["outcome"].startswith("decode")
            and pure_mac_rows[fixture]["outcome"].startswith("decode")
            and "rejected" not in pure_win_rows[fixture]["outcome"]
            and "rejected" not in pure_mac_rows[fixture]["outcome"],
        }
        for fixture in fixtures
    ]

    shared_not_run = [
        gate("scheduled_start_p95_30", "not-run", [], "Smoke timing is not the frozen 30-sample packaged-hardware matrix."),
        gate("seek_to_audio_p95_30", "not-run", [], "Smoke timing is not the frozen 30-sample packaged-hardware matrix."),
        gate("peak_rss_2h", "not-run", [], "No two-hour process-tree RSS series was run on physical packaged nodes."),
        gate("rss_growth_2h", "not-run", [], "No two-hour process-tree RSS growth series was run."),
        gate("rss_slope_2h", "not-run", [], "No two-hour process-tree RSS slope was measured."),
        gate("range_fault_cache_reuse", "not-run", ["stream-contract"], "The transport substrate passes deterministic fault tests, but no candidate-specific packaged pairing run exists."),
    ]

    bundled_gates = [
        gate("all_required_formats", "pass", ["bundled-macos-evidence", "bundled-windows-evidence"], "All six smoke fixtures decode on both hosted architectures."),
        gate("start_before_full_download", "not-run", ["bundled-macos-evidence", "bundled-windows-evidence"], "The decoder probe receives coordinator-prepared local files; end-to-end first-audio range evidence is absent."),
        gate("pause_seek_resume_drain_cancel", "pass", ["bundled-macos-evidence", "bundled-windows-evidence"], "All fixtures preserve the frozen lifecycle and generation seam."),
        *shared_not_run,
        gate("hostile_input", "pass", ["bundled-macos-evidence", "bundled-windows-evidence"], "Hostile fixtures terminate under process and output bounds without a crash."),
        gate("store_sandbox_release_package", "fail", ["bundled-macos-receipt", "bundled-windows-receipt"], "Windows ARM64, production signing, notarization and accepted native-decoder isolation are absent."),
        gate("license_release_disposition", "fail", ["license-audit"], "The audit requires release-time counsel, source-offer, notices, SBOM and advisory closure."),
    ]
    native_gates = [
        gate("all_required_formats", "fail", ["media-foundation-receipt", "native-macos-evidence"], "Windows rejects both Ogg/Opus fixtures with 0xC00D36C4."),
        gate("start_before_full_download", "fail", ["media-foundation-receipt", "native-macos-evidence"], "macOS requests at least the complete source before first PCM; several Windows fixtures also exceed source length before first sample."),
        gate("pause_seek_resume_drain_cancel", "fail", ["media-foundation-receipt", "native-macos-evidence"], "Lifecycle is unavailable for Windows Ogg/Opus and the cold macOS MP3 row fails its lifecycle timing gate."),
        *shared_not_run,
        gate("hostile_input", "not-run", ["media-foundation-receipt", "native-macos-evidence"], "The complete identical hostile corpus was not run through both native candidates."),
        gate("store_sandbox_release_package", "fail", ["media-foundation-receipt", "native-macos-receipt"], "Windows is CI test-signed and macOS is ad-hoc signed; no production notarized/release package pair exists."),
        gate("license_release_disposition", "pass", ["license-audit"], "Inbox/native frameworks avoid bundled third-party decoder distribution."),
    ]
    pure_gates = [
        gate("all_required_formats", "fail", ["pure-go-macos-evidence", "pure-go-windows-evidence"], "AAC is rejected without reads because the audited GPL-only module is forbidden."),
        gate("start_before_full_download", "fail", ["pure-go-macos-evidence", "pure-go-windows-evidence"], "MP3 first PCM is incremental, but seek construction full-scans MP3 and Ogg has no random-seek API."),
        gate("pause_seek_resume_drain_cancel", "fail", ["pure-go-macos-evidence", "pure-go-windows-evidence"], "No acceptable seek lifecycle exists for MP3 or Ogg and AAC cannot decode."),
        *shared_not_run,
        gate("hostile_input", "pass", ["pure-go-macos-evidence", "pure-go-windows-evidence"], "Truncated and corrupt MP3/Ogg fixtures remain bounded and panic-free; race coverage passes."),
        gate("store_sandbox_release_package", "fail", ["pure-go-macos-receipt", "pure-go-windows-receipt"], "CGo-free research binaries are not sandboxed production application packages."),
        gate("license_release_disposition", "fail", ["license-audit", "pure-go-macos-receipt", "pure-go-windows-receipt"], "The only audited AAC module is GPL-2.0-only and is intentionally absent."),
    ]

    combinations = [
        {
            "id": "bundled-ffmpeg-both-platforms",
            "windowsCandidate": "bundled-ffmpeg-8.1.2-v1",
            "macosCandidate": "bundled-ffmpeg-8.1.2-v1",
            "formatRows": bundled_formats,
            "hardGates": bundled_gates,
            "pairings": pairings([row["id"] for row in bundled_gates if row["status"] != "pass"]),
            "conclusion": "rejected",
        },
        {
            "id": "native-mf-plus-avfoundation",
            "windowsCandidate": "native-canonical-aac-v1/media-foundation",
            "macosCandidate": "native-canonical-aac-v1/avfoundation-probe",
            "formatRows": native_formats,
            "hardGates": native_gates,
            "pairings": pairings([row["id"] for row in native_gates if row["status"] != "pass"]),
            "conclusion": "rejected",
        },
        {
            "id": "pure-go-both-platforms",
            "windowsCandidate": "pure-go-composite-v1",
            "macosCandidate": "pure-go-composite-v1",
            "formatRows": pure_formats,
            "hardGates": pure_gates,
            "pairings": pairings([row["id"] for row in pure_gates if row["status"] != "pass"]),
            "conclusion": "rejected",
        },
    ]

    return {
        "schemaVersion": 1,
        "contract": "p2-codec-player-comparative-matrix.v1",
        "generatedFromRubric": rubric["contract"],
        "claimClass": "repository-and-hosted-engineering-evidence",
        "manualEvidence": MANUAL_EPIC,
        "rules": {
            "requiredPairings": PAIRINGS,
            "requiredFixtures": fixtures,
            "scoreAveragingAllowed": False,
            "selectionRule": "One complete combination must pass every format row, every hard gate and every required pairing.",
        },
        "artifacts": artifact_index(),
        "transportSubstrate": {
            "status": "repository-pass-only",
            "artifact": "stream-contract",
            "rangeProfiles": rubric["rangeProfiles"],
            "candidateIntegrationClaimed": False,
        },
        "combinations": combinations,
        "rawFailures": [
            {"combination": "native-mf-plus-avfoundation", "artifact": "media-foundation-receipt", "jsonPointer": "/evidence/fixtures/4/openHRESULT", "observed": mf_rows["opus_ogg_cbr_12s"]["openHRESULT"], "meaning": "Windows Media Foundation cannot open Ogg/Opus."},
            {"combination": "native-mf-plus-avfoundation", "artifact": "native-macos-evidence", "jsonPointer": "/fixtures/0/bytesBeforeFirstSample", "observed": native_mac_rows["mp3_cbr_12s"]["bytesBeforeFirstSample"], "sourceBytes": native_mac_rows["mp3_cbr_12s"]["sourceBytes"], "meaning": "Native macOS consumes the complete MP3 before first PCM."},
            {"combination": "pure-go-both-platforms", "artifact": "pure-go-macos-evidence", "jsonPointer": "/fixtures/2/outcome", "observed": pure_mac_rows["aac_m4a_12s"]["outcome"], "meaning": "AAC is unavailable under the accepted license boundary."},
            {"combination": "pure-go-both-platforms", "artifact": "pure-go-windows-evidence", "jsonPointer": "/fixtures/0/seekPrepareBytes", "observed": pure_win_rows["mp3_cbr_12s"]["seekPrepareBytes"], "sourceBytes": pure_win_rows["mp3_cbr_12s"]["sourceBytes"], "meaning": "MP3 seek construction requires a full scan."},
            {"combination": "pure-go-both-platforms", "artifact": "pure-go-windows-evidence", "jsonPointer": "/fixtures/4/seekSupported", "observed": pure_win_rows["opus_ogg_cbr_12s"]["seekSupported"], "meaning": "The pure-Go Ogg reader has no random-seek contract."},
            {"combination": "bundled-ffmpeg-both-platforms", "artifact": "bundled-windows-receipt", "jsonPointer": "/shippingDecision", "observed": read_json(SOURCES["bundled-windows-receipt"])["shippingDecision"], "meaning": "The technically viable bundled prototype lacks release evidence."},
        ],
        "selection": {
            "allowed": False,
            "selectedCombination": None,
            "productionImplementationMayProceed": False,
            "decision": "no-complete-combination-passes-every-hard-gate",
            "nextTask": "TASK-260712-2eympi",
        },
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, default=OUTPUT)
    args = parser.parse_args()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(build_matrix(), indent=2, sort_keys=False) + "\n", encoding="utf-8")
    print(args.output)


if __name__ == "__main__":
    main()
