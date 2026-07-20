#!/usr/bin/env python3
"""Fail-closed validation for the production-dark Windows E2EE live PTT bridge."""

from __future__ import annotations

import hashlib
import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/windows-e2ee-live-ptt-v1.json"


class WindowsE2EELivePTTError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise WindowsE2EELivePTTError(message)


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-windows-e2ee-live-ptt.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-39vjzd", "wrong task")
    require(contract.get("publishedAt") == "2026-07-20", "publication date drifted")
    require(contract.get("decision") == {
        "result": "production-dark-windows-e2ee-live-ptt-foundation",
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
        "live-bridge", "key-state", "live-tests", "live-vectors", "macos-vectors",
        "adr", "protocol-authority", "opaque-live-contract", "opaque-router",
        "macos-live-packet", "design-review",
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
        'copy(result[0:2], []byte("BE"))',
        'ReserveSendGeneration(identity.Metadata.InstallationID, request.GroupID, "live_ptt"',
        "current.Metadata.CommitDigest != initial.Metadata.CommitDigest",
        "c.authorization.CurrentAuthorization()", "c.crypto.Seal(", "c.crypto.Open(",
        "windowsLiveFramesEqual(*c.lastPlaintextFrame, frame)",
        "b.receiver.Receive(frame)", "newWindowsE2EELiveSessionFactoryForAudit",
        "ErrWindowsE2EELiveProviderNotApproved", "c.terminateLocked()",
    }:
        require(token in source, f"live invariant missing: {token}")
    for token in {
        "crypto/aes", "crypto/cipher", "chacha", "golang.org/x/crypto",
        "net/http", "os.WriteFile", "os.Create", "log.Printf", "slog.",
    }:
        require(token not in source, f"unreviewed primitive, I/O or logging: {token}")

    sender = (ROOT / "pulsar-win/windows_live_capture_sender.go").read_text(encoding="utf-8")
    capture_worker = sender.split(
        "func (s *WindowsLiveCaptureSender) captureWorker", 1
    )[1].split("func (s *WindowsLiveCaptureSender) nextSequence", 1)[0]
    transport_worker = sender.split(
        "func (s *WindowsLiveCaptureSender) transportWorker", 1
    )[1].split("func (s *WindowsLiveCaptureSender) finishCapture", 1)[0]
    for token in {"E2EE", ".Seal(", "trySendFrame"}:
        require(token not in capture_worker, f"crypto/transport entered capture worker: {token}")
    require("s.trySendFrame(*pending)" in transport_worker,
            "protected transport injection point moved into capture path")

    tests = (ROOT / artifacts["live-tests"]["path"]).read_text(encoding="utf-8")
    for name in {
        "OpaqueFrameMatchesAcceptedBEWire",
        "RetryReusesCiphertextAndAuthenticatesBeforeJitter",
        "TamperReplayAndNonceReuseFailClosed",
        "ProviderOutputAndDurationBounds",
        "ProviderAndCallerAliasingCannotMutateRetryState",
        "MembershipAndCommitChangeTerminate",
        "AADBindsSharedContextNotLocalRevision",
        "UnreviewedProviderCannotCrossProductionFactory",
        "FactoryReservesWitnessedGeneration",
        "CrossInstallationRoundTripWithSkewedLocalRevisions",
        "IncomingReorderRemainsWithinExistingWindow",
    }:
        require(name in tests, f"fixture missing: {name}")

    vectors = json.loads((ROOT / artifacts["live-vectors"]["path"]).read_text())
    macos_vectors = json.loads((ROOT / artifacts["macos-vectors"]["path"]).read_text())
    require(vectors.get("contract") == "p3-windows-e2ee-live-ptt-fixtures.v1",
            "wrong fixture contract")
    require(vectors.get("status") == "audit-fixture-only-production-disabled",
            "fixture vectors represented as production")
    for field in {"opaqueFrame", "bounds", "aadBindings"}:
        require(vectors.get(field) == macos_vectors.get(field),
                f"macOS-Windows fixture parity drifted: {field}")
    frame = vectors.get("opaqueFrame", {})
    require(frame.get("encodedHex", "").startswith("42450101"), "BE magic drifted")
    require(len(bytes.fromhex(frame.get("encodedHex", ""))) == 88,
            "opaque wire fixture length drifted")
    require(len(vectors.get("aadBindings", [])) == 21, "AAD inventory drifted")
    require(set(vectors.get("failClosed", [])) == {
        "tampered_ciphertext", "replayed_sequence", "reused_nonce", "stale_epoch",
        "changed_commit_digest", "foreign_target", "removed_sender",
        "gap_outside_window", "legacy_bp_downgrade", "unapproved_provider",
        "malformed_provider_output",
    }, "fail-closed inventory drifted")

    invariants = set(contract.get("invariants", []))
    require(len(invariants) == 17, "invariant inventory drifted")
    for item in {
        "witnessed-epoch-and-cross-process-live-generation-before-derivation",
        "exact-macos-parity-for-be-wire-and-shared-commit-bound-aad",
        "transport-retry-reuses-exact-ciphertext-and-nonce",
        "authentication-required-before-jitter-decoder-fec-plc",
        "membership-or-epoch-change-terminates-and-destroys-session-once",
        "runtime-composition-ui-claim-and-capability-remain-dark",
    }:
        require(item in invariants, f"invariant missing: {item}")

    for path in (ROOT / "pulsar-win").glob("*.go"):
        if path.name.endswith("_test.go") or path.name == "windows_e2ee_live_ptt.go":
            continue
        production = path.read_text(encoding="utf-8")
        if path.name == "windows_encrypted_media_client.go":
            require("NewWindowsE2EELiveSessionFactory" in production and
                    "intentionally absent from" in production and
                    "CapabilityAdvertised: false" in production and
                    "RuntimeWiringApproved: false" in production,
                    "Windows encrypted-media client is not the exact production-dark live boundary")
            continue
        require("WindowsE2EELiveSessionFactory" not in production and
                "NewWindowsE2EELiveFrameChannel" not in production and
                "NewWindowsE2EELiveSenderBridge" not in production,
                f"E2EE live runtime wired into production source: {path.name}")

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
    print("Windows E2EE live PTT: PASS (production disabled)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
