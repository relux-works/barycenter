#!/usr/bin/env python3
"""Validate the frozen macOS native codec probe and optional runtime evidence."""

from __future__ import annotations

import argparse
import hashlib
import json
import platform
import plistlib
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/codec-spike/macos-native-probe-v1.json"
EXPECTED_FIXTURES = [
    "mp3_cbr_12s",
    "mp3_vbr_12s",
    "aac_m4a_12s",
    "aac_adts_12s",
    "opus_ogg_cbr_12s",
    "opus_ogg_vbr_12s",
]


def load_json(path: Path) -> dict:
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def validate_contract(contract: dict) -> None:
    assert contract["schemaVersion"] == 1
    assert contract["contract"] == "p2-macos-native-streaming-decoder-probe.v1"
    assert contract["candidateId"] == "macos-avfoundation-resource-loader-v1"
    assert contract["manualEpic"] == "EPIC-260714-th54l3"
    assert contract["package"] == {
        "bundleIdentifier": "live.barycenter.PulsarMacNativeCodecProbe",
        "appSandbox": True,
        "hardenedRuntime": True,
        "networkClient": False,
        "networkServer": False,
        "audioOutput": False,
        "adHocSignatureAllowedForEngineering": True,
        "productionSignatureProven": False,
        "notarizationProven": False,
    }
    assert contract["streamBoundary"]["implementation"] == "AVAssetResourceLoaderDelegate"
    assert contract["streamBoundary"]["maximumUnderlyingReadBytes"] == 65_536
    assert contract["streamBoundary"]["maximumPreparedReadBytes"] == 1_048_576
    assert contract["streamBoundary"]["decoderOwnsNetwork"] is False
    assert contract["streamBoundary"]["renderCallbackUsed"] is False
    assert [fixture["id"] for fixture in contract["fixtures"]] == EXPECTED_FIXTURES
    assert contract["requiredLifecycle"] == [
        "scheduled-start",
        "pause-without-read",
        "generation-safe-seek",
        "resume",
        "decoder-drain",
        "cooperative-cancel",
    ]
    assert contract["acceptance"]["passOrExactReject"] is True
    assert contract["acceptance"]["physicalHardwareClaim"] is False


def codesign_entitlements(app: Path) -> dict:
    completed = subprocess.run(
        ["codesign", "-d", "--entitlements", ":-", str(app)],
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    raw = completed.stdout or completed.stderr
    start = raw.find(b"<?xml")
    if start < 0:
        start = raw.find(b"<plist")
    if start < 0:
        raise AssertionError("codesign did not emit an entitlement plist")
    return plistlib.loads(raw[start:])


def codesign_metadata(app: Path) -> str:
    completed = subprocess.run(
        ["codesign", "-dv", "--verbose=4", str(app)],
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
    )
    flag_line = next((line for line in completed.stdout.splitlines() if " flags=" in line), "")
    assert "runtime" in flag_line
    return completed.stdout


def validate_evidence(evidence: dict, contract: dict) -> None:
    assert evidence["schemaVersion"] == 1
    assert evidence["contract"] == contract["contract"]
    assert evidence["candidateId"] == contract["candidateId"]
    assert evidence["claimClass"] == "repository-engineering-prototype"
    assert evidence["bundleIdentifier"] == contract["package"]["bundleIdentifier"]
    assert evidence["sandboxEntitlement"] is True
    assert evidence["networkClientEntitlement"] is False
    assert evidence["renderCallbackUsed"] is False
    assert evidence["decoderOwnsNetwork"] is False
    assert evidence["maximumUnderlyingReadBytes"] == 65_536
    assert evidence["peakRSSBytes"] > 0
    fixtures = evidence["fixtures"]
    assert [fixture["id"] for fixture in fixtures] == EXPECTED_FIXTURES
    for fixture in fixtures:
        assert fixture["sourceBytes"] > 0
        assert fixture["maximumReadBytes"] <= 65_536
        assert fixture["outcome"] in {"decode", "reject"}
        if fixture["outcome"] == "decode":
            assert fixture["samples"] > 0
            assert fixture["pcmBytes"] > 0
            assert fixture["trackStartMS"] >= 0
            assert fixture["seekToSampleMS"] >= 0
            assert fixture["scheduledSkewMS"] >= 0
            assert fixture["seekGeneration"] == 2
            assert fixture["resumed"] is True
            assert fixture["drained"] is True
            assert fixture["cancelled"] is True
            expected_lifecycle = (
                fixture["pausedWithoutRead"] is True
                and fixture["trackStartMS"] <= contract["engineeringGates"]["trackStartMS"]
                and fixture["seekToSampleMS"] <= contract["engineeringGates"]["seekToSampleMS"]
                and fixture["scheduledSkewMS"] <= contract["engineeringGates"]["scheduledSkewMS"]
            )
            assert fixture["passedLifecycle"] is expected_lifecycle
        else:
            assert fixture["errorDomain"]
            assert isinstance(fixture["errorCode"], int)
            assert fixture["errorDescription"]

    all_decode = all(fixture["outcome"] == "decode" for fixture in fixtures)
    all_incremental = all(
        fixture["startBeforeFullFile"] for fixture in fixtures if fixture["outcome"] == "decode"
    )
    expected_pass = all_decode and all_incremental and all(
        fixture["passedLifecycle"] for fixture in fixtures
    )
    assert evidence["passed"] is expected_pass
    if expected_pass:
        assert evidence["shippingDecision"] == "engineering-candidate-only-manual-matrix-required"
    else:
        assert evidence["shippingDecision"].startswith("rejected-")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--evidence", type=Path)
    parser.add_argument("--app", type=Path)
    parser.add_argument("--receipt", type=Path)
    args = parser.parse_args()

    contract = load_json(CONTRACT_PATH)
    validate_contract(contract)
    if args.evidence is None:
        print("macOS native codec probe contract valid")
        return 0

    evidence = load_json(args.evidence)
    validate_evidence(evidence, contract)
    if args.app is None or args.receipt is None:
        raise SystemExit("--app and --receipt are required with --evidence")
    executable = args.app / "Contents/MacOS/pulsar-macos-native-codec-probe"
    entitlements = codesign_entitlements(args.app)
    assert entitlements == {"com.apple.security.app-sandbox": True}
    metadata = codesign_metadata(args.app)
    flag_line = next(line for line in metadata.splitlines() if " flags=" in line)
    cdhash_line = next(line for line in metadata.splitlines() if line.startswith("CDHash="))
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(args.app)], check=True)
    (args.receipt.parent / "entitlements.plist").write_bytes(
        plistlib.dumps(entitlements, fmt=plistlib.FMT_XML, sort_keys=True)
    )
    resources = args.app / "Contents/Resources"
    resource_receipts = []
    for name in ["macos-native-probe-v1.json", *[item["file"] for item in contract["fixtures"]]]:
        path = resources / name
        resource_receipts.append({"name": name, "bytes": path.stat().st_size, "sha256": sha256(path)})
    receipt = {
        "schemaVersion": 1,
        "contract": contract["contract"],
        "claimClass": "repository-engineering-prototype",
        "runner": {
            "os": platform.platform(),
            "architecture": platform.machine(),
            "realHardwareClaim": False,
        },
        "bundleIdentifier": contract["package"]["bundleIdentifier"],
        "signature": "ad-hoc-engineering",
        "hardenedRuntime": True,
        "codesignRuntimeFlag": flag_line.split(" flags=", 1)[1].split(" hashes=", 1)[0],
        "codesignCDHash": cdhash_line.split("=", 1)[1],
        "entitlements": entitlements,
        "executableBytes": executable.stat().st_size,
        "executableSha256": sha256(executable),
        "evidenceSha256": sha256(args.evidence),
        "sealedResources": resource_receipts,
        "productionSignature": "not-proven",
        "notarization": "not-proven",
        "manualEvidence": "EPIC-260714-th54l3",
    }
    args.receipt.parent.mkdir(parents=True, exist_ok=True)
    args.receipt.write_text(json.dumps(receipt, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps({"passed": evidence["passed"], "shippingDecision": evidence["shippingDecision"]}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
