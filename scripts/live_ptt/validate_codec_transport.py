#!/usr/bin/env python3
"""Fail-closed validation for the live codec/transport spike decision."""

from __future__ import annotations

import hashlib
import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
DECISION = ROOT / "acceptance/live-ptt/codec-transport-decision-v1.json"


class DecisionError(RuntimeError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise DecisionError(message)


def digest(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load() -> dict:
    return json.loads(DECISION.read_text(encoding="utf-8"))


def validate(data: dict) -> None:
    require(data.get("schemaVersion") == 1, "schema changed")
    require(data.get("contract") == "p3-live-codec-transport-decision.v1", "contract changed")
    require(data.get("taskId") == "TASK-260712-lo7a68", "task changed")
    require(data.get("owner") == "Ivan Oparin", "owner changed")
    require(data.get("ownerApproval", {}).get("state") == "approved-engineering-defaults",
            "engineering defaults are not approved")

    decision = data.get("decision", {})
    require(decision.get("status") == "engineering-profile-frozen-production-no-go",
            "fail-closed verdict changed")
    require(decision.get("productionLivePttAllowed") is False, "false production acceptance recorded")
    require(decision.get("runtimeDefaultEnabled") is False, "live PTT enabled without evidence")
    require(decision.get("wireContractEngineeringMayContinue") is True, "engineering handoff removed")

    profile = data.get("codecProfile", {})
    expected = {
        "implementation": "libopus", "version": "1.6.1", "sampleRateHz": 48000,
        "channels": 1, "frameMs": 20, "targetBitrateBps": 24000,
        "rateControl": "constrained-vbr", "complexity": 5, "dtx": False,
        "inbandFec": True, "expectedLossPercent": 2, "maxEncodedPayloadBytes": 400,
        "maxFramesPerSecond": 50,
    }
    for key, value in expected.items():
        require(profile.get(key) == value, f"codec profile drifted: {key}")
    require(profile.get("sourceSha256") ==
            "6ffcb593207be92584df15b32466ed64bbec99109f007c82205f0194572411a1",
            "libopus source hash changed")
    require("FEC" in profile.get("decoderLossRule", "") and "PLC" in profile.get("decoderLossRule", ""),
            "loss recovery rule incomplete")

    frame = data.get("binaryFrame", {})
    require(frame.get("byteOrder") == "network-big-endian", "byte order changed")
    require(frame.get("headerBytes") == 40 and frame.get("maxMessageBytes") == 440,
            "binary message bound changed")
    fields = frame.get("fields", [])
    require(len(fields) == 12, "binary header inventory changed")
    require(fields[0] == {"offset": 0, "bytes": 2, "name": "magic", "value": "BP"},
            "binary magic changed")
    offsets = [item.get("offset") for item in fields]
    require(offsets == sorted(offsets) and fields[-1].get("offset") == 38,
            "binary header layout invalid")
    by_name = {item.get("name"): item for item in fields}
    require(by_name.get("payload-length", {}).get("maximum") == 400,
            "payload allocation bound changed")
    require("before allocation" in frame.get("validation", ""), "pre-allocation validation removed")

    transport = data.get("transport", {})
    wss = transport.get("engineeringBaseline", {})
    require(wss.get("serverAndWindowsLibrary") == "github.com/gorilla/websocket v1.5.3",
            "WSS pin changed")
    require(wss.get("macosLibrary") == "Foundation URLSessionWebSocketTask", "macOS WSS path changed")
    require(wss.get("receiverJitterBufferMs") == 60, "jitter buffer changed")
    require("eight-frame/160 ms" in wss.get("queuePolicy", ""), "queue bound changed")
    require(wss.get("status") == "engineering-only-not-production-accepted",
            "WSS falsely accepted for production")
    quic = transport.get("datagramAlternative", {})
    require(quic.get("candidateImplementation") == "MsQuic 2.5.8", "QUIC candidate changed")
    require(quic.get("status") == "deferred-not-selected", "unproved QUIC selected")

    evidence = data.get("evidence", {})
    benchmark_ref = evidence.get("opusBenchmark", {})
    model_ref = evidence.get("transportModel", {})
    benchmark_path = ROOT / benchmark_ref.get("path", "")
    model_path = ROOT / model_ref.get("path", "")
    require(benchmark_path.is_file() and digest(benchmark_path) == benchmark_ref.get("sha256"),
            "Opus benchmark receipt changed")
    require(model_path.is_file() and digest(model_path) == model_ref.get("sha256"),
            "transport model receipt changed")

    benchmark = json.loads(benchmark_path.read_text(encoding="utf-8"))
    require("not Windows" in benchmark.get("claimBoundary", ""), "benchmark boundary widened")
    require(benchmark.get("host", {}).get("system") == "Darwin" and
            benchmark.get("host", {}).get("machine") == "x86_64", "benchmark host changed")
    host = benchmark.get("host", {})
    require("/Cellar/opus/1.6.1/" in host.get("libraryPathResolved", ""),
            "measured libopus path is not pinned")
    require(len(host.get("librarySha256", "")) == 64 and len(host.get("pkgConfigSha256", "")) == 64,
            "measured libopus binary provenance missing")
    profiles = {item.get("frameMs"): item for item in benchmark.get("profiles", [])}
    require(set(profiles) == {10, 20}, "10/20 ms benchmark matrix incomplete")
    selected = profiles[20]
    require(selected.get("libraryVersion") == "libopus 1.6.1", "measured library changed")
    require(selected.get("audioSeconds") == 240.0 and selected.get("frames") == 12000,
            "benchmark duration changed")
    require(selected.get("cpu", {}).get("realtimeFactor", 1) < 0.02, "codec CPU bound failed")
    require(selected.get("packetBytes", {}).get("max", 401) <= 400, "measured packet bound failed")
    require(selected.get("stateBytes", {}).get("encoder", 0) +
            selected.get("stateBytes", {}).get("decoder", 0) == 50136, "codec state bound changed")

    model = json.loads(model_path.read_text(encoding="utf-8"))
    require("not real network" in model.get("claimBoundary", ""), "model boundary widened")
    modeled = {(item.get("frameMs"), item.get("transport")): item for item in model.get("profiles", [])}
    require(set(modeled) == {(10, "wss-tcp"), (10, "quic-datagram"),
                            (20, "wss-tcp"), (20, "quic-datagram")}, "model matrix incomplete")
    require(all(item.get("budgetModelPass") is True and item.get("intelligibilityClaim") is False
                for item in modeled.values()), "model pass or claim boundary changed")
    require(modeled[(20, "wss-tcp")]["latencyMs"]["p50"] <= 800 and
            modeled[(20, "wss-tcp")]["latencyMs"]["p95"] <= 1500, "WSS budget model failed")
    require(modeled[(20, "wss-tcp")]["network"]["tcpHolEvents"] > 0, "TCP HOL not exercised")
    require(modeled[(20, "quic-datagram")]["network"]["fecRecovered"] > 0 and
            modeled[(20, "quic-datagram")]["network"]["plcConcealed"] > 0,
            "datagram FEC/PLC not exercised")
    slow = model.get("slowRecipient", {})
    require(slow.get("isolated") is True and slow.get("capacityFrames") == 8 and
            slow.get("fastRecipient", {}).get("droppedOldest") == 0 and
            slow.get("slowRecipient", {}).get("droppedOldest", 0) > 0,
            "slow recipient is not bounded and isolated")

    supply = data.get("supplyAndSecurity", {})
    require(supply.get("sbomPurl") == "pkg:generic/libopus@1.6.1", "SBOM identity changed")
    require("No zero-CVE claim" in supply.get("vulnerabilityDisposition", ""),
            "unsupported vulnerability claim recorded")
    require("no runtime code download" in supply.get("updateRule", ""), "runtime download allowed")

    package_gates = data.get("packageGates", {})
    require(package_gates.get("noFirstRunCodeDownload") == "required", "first-run download allowed")
    for platform_name in ("windows", "macos", "quic"):
        require(package_gates.get(platform_name, {}).get("status") == "fail",
                f"unproved package gate accepted: {platform_name}")

    blockers = data.get("openBlockers", [])
    require([item.get("id") for item in blockers] ==
            ["P3-LIVE-001", "P3-LIVE-002", "P3-LIVE-003", "P3-LIVE-004"],
            "open blocker inventory changed")
    require(all(item.get("severity") == "High" for item in blockers), "blocker severity weakened")
    manual = data.get("manualHandoff", {})
    require(manual.get("epicId") == "EPIC-260714-th54l3" and
            manual.get("taskId") == "TASK-260712-1rzqh9", "manual C2 handoff changed")

    sources = data.get("sourceReceipts", [])
    source_ids = {item.get("id") for item in sources}
    require(len(sources) == len(source_ids) >= 12, "source inventory incomplete or duplicated")
    for source in sources:
        require(source.get("url", "").startswith("https://") and source.get("retrievedAt") == "2026-07-16",
                "source receipt invalid")
    for required in ("libopus-release", "opus-license", "rfc6716", "rfc8854", "rfc6455",
                     "rfc9221", "gorilla-websocket-1.5.3", "apple-urlsession-websocket",
                     "msquic-2.5.8", "msquic-settings", "msquic-api", "nvd-pjsip-wrapper"):
        require(required in source_ids, f"missing source {required}")

    receipts = data.get("repositoryReceipts", [])
    require(len(receipts) == 7, "repository receipt inventory changed")
    for receipt in receipts:
        path = ROOT / receipt.get("path", "")
        require(path.is_file() and digest(path) == receipt.get("sha256"),
                f"repository receipt changed: {receipt.get('path')}")

    release = (ROOT / ".github/workflows/release.yml").read_text(encoding="utf-8")
    require("CGO_ENABLED=0 GOOS=windows" in release, "Windows package assumption changed")
    require("libopus" not in release, "libopus entered release without rerunning spike")
    require("github.com/gorilla/websocket v1.5.3" in
            (ROOT / "coordinator/go.mod").read_text(encoding="utf-8"), "coordinator WSS pin changed")
    require("github.com/gorilla/websocket v1.5.3" in
            (ROOT / "pulsar-win/go.mod").read_text(encoding="utf-8"), "Windows WSS pin changed")


def main() -> int:
    validate(load())
    print(f"live codec/transport decision valid: {DECISION.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
