#!/usr/bin/env python3
"""Fail-closed validation for the production-dark macOS E2EE live PTT bridge."""

from __future__ import annotations

import hashlib
import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/macos-e2ee-live-ptt-v1.json"


class MacE2EELivePTTError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise MacE2EELivePTTError(message)


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-macos-e2ee-live-ptt.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-3980vy", "wrong task")
    require(contract.get("publishedAt") == "2026-07-20", "publication date drifted")
    require(contract.get("decision") == {
        "result": "production-dark-macos-e2ee-live-ptt-foundation",
        "productionEnabled": False,
        "runtimeHTTPWired": False,
        "capabilityAdvertised": False,
        "productionLibrarySelected": False,
        "productionSuiteSelected": False,
        "auditFixtureOnly": True,
        "plaintextFallbackAllowed": False,
        "coordinatorDecryptsSpeech": False,
        "coordinatorPersistsLiveFrames": False,
        "signedPackageTested": False,
        "deltaReviewRequired": True,
    }, "production-dark decision drifted")

    artifacts = {item.get("id"): item for item in contract.get("artifacts", [])}
    require(set(artifacts) == {
        "live-bridge", "key-state", "live-tests", "live-vectors", "adr",
        "protocol-authority", "opaque-live-contract", "opaque-router",
        "design-review",
    }, "artifact inventory incomplete")
    for name, item in artifacts.items():
        path = ROOT / item.get("path", "")
        require(path.is_file(), f"artifact missing: {name}")
        require(sha256(path) == item.get("sha256"), f"artifact drifted: {name}")

    require(contract.get("bounds") == {
        "opaqueHeaderBytes": 84,
        "ciphertextBytes": 512,
        "opaqueMessageBytes": 596,
        "plaintextBytes": 400,
        "frameMS": 20,
        "jitterBufferMS": 60,
        "gapFrames": 8,
        "durationMS": 300000,
        "maximumSequence": 15000,
    }, "bounds drifted")

    source = (ROOT / artifacts["live-bridge"]["path"]).read_text(encoding="utf-8")
    for token in {
        "bytes[1] = 0x45", "domain: \"live_ptt\"",
        "authorization.currentAuthorization()", "crypto.seal(", "crypto.open(",
        "outgoingNonces.insert", "incomingNonces.insert",
        "commitDigest: current.metadata.commitDigest",
        "current.commitDigest == context.commitDigest",
        "lastPlaintextFrame", "receiver.receive(try channel.open(opaque))",
        "crossProcessGenerationSerializationApproved", "providerNotApproved",
        "destroyCryptoLocked()",
    }:
        require(token in source, f"live invariant missing: {token}")
    for token in {"AES.GCM", "ChaChaPoly", "CryptoKit", "URLSession(", "FileHandle("}:
        require(token not in source, f"unreviewed primitive or persistence: {token}")

    sender = (ROOT / "node-app/Sources/NodeCore/MacLiveCaptureSender.swift").read_text()
    callback = sender.split("private func captureCallback", 1)[1].split(
        "private func drainSamplesLocked", 1)[0]
    for token in {"crypto", "seal(", "encoded()", "trySendFrame"}:
        require(token not in callback, f"crypto/transport entered capture callback: {token}")

    tests = (ROOT / artifacts["live-tests"]["path"]).read_text(encoding="utf-8")
    for name in {
        "wireContract", "retryAndAuthenticationBarrier", "tamperFailsClosed",
        "replayAndNonceReuse", "membershipChangeTerminates", "aadBinding",
        "productionProviderGate", "witnessedEpochDerivation",
        "crossInstallationRoundTrip", "providerOutputAndDurationBounds",
    }:
        require(name in tests, f"fixture missing: {name}")

    vectors = json.loads((ROOT / artifacts["live-vectors"]["path"]).read_text())
    require(vectors.get("status") == "audit-fixture-only-production-disabled",
            "fixture vectors represented as production")
    frame = vectors.get("opaqueFrame", {})
    require(frame.get("encodedHex", "").startswith("42450101"), "BE magic drifted")
    require(len(bytes.fromhex(frame.get("encodedHex", ""))) == 88,
            "opaque wire fixture length drifted")
    require(len(vectors.get("aadBindings", [])) == 21, "AAD inventory drifted")
    require(len(vectors.get("failClosed", [])) == 11, "fail-closed inventory drifted")

    invariants = set(contract.get("invariants", []))
    require(len(invariants) == 17, "invariant inventory drifted")
    for item in {
        "witnessed-epoch-and-live-generation-before-derivation",
        "exact-air-target-sender-session-epoch-commit-generation-sequence-codec-timing-aad",
        "transport-retry-reuses-exact-ciphertext-and-nonce",
        "authentication-required-before-jitter-decoder-fec-plc",
        "membership-or-epoch-change-terminates-and-destroys-session-once",
        "runtime-composition-ui-claim-and-capability-remain-dark",
    }:
        require(item in invariants, f"invariant missing: {item}")

    main = (ROOT / "node-app/Sources/NodeApp/main.swift").read_text(encoding="utf-8")
    require("MacE2EELiveSessionFactory" not in main, "E2EE live runtime-wired")
    require("e2ee_media_v1" not in main, "E2EE capability advertised")

    manual = contract.get("manualEvidence", {})
    require(manual.get("epic") == "EPIC-260714-th54l3", "manual epic missing")
    for name, value in manual.items():
        if name != "epic":
            require(value in {"not-run", "not-run-no-selected-stack"},
                    f"manual evidence invented: {name}")
    require(set(contract.get("openProductionGates", [])) == {
        "EPC-001", "EPC-002", "EPC-004", "EPC-005", "TASK-260712-1ulshp",
    }, "production gate hidden or closed")


def main() -> int:
    validate(load())
    print("macOS E2EE live PTT: PASS (production disabled)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
