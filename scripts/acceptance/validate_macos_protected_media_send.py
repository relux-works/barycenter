#!/usr/bin/env python3
"""Fail-closed validation for production-dark macOS protected-media send."""

from __future__ import annotations

import hashlib
import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/macos-protected-media-send-v1.json"


class MacProtectedMediaSendError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise MacProtectedMediaSendError(message)


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-macos-protected-media-send.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-2kcduo", "wrong task")
    require(contract.get("publishedAt") == "2026-07-20", "publication date drifted")
    require(contract.get("decision") == {
        "result": "production-dark-macos-protected-media-send-foundation",
        "productionEnabled": False,
        "runtimeHTTPWired": False,
        "capabilityAdvertised": False,
        "productionLibrarySelected": False,
        "productionSuiteSelected": False,
        "productionContainerSelected": False,
        "auditFixtureOnly": True,
        "plaintextFallbackAllowed": False,
        "coordinatorPlaintextPath": False,
        "signedPackageTested": False,
        "deltaReviewRequired": True,
    }, "production-dark decision drifted")

    artifacts = {item.get("id"): item for item in contract.get("artifacts", [])}
    require(set(artifacts) == {
        "send-pipeline", "send-tests", "send-vectors", "key-state",
        "protocol-authority", "opaque-router", "adr", "threat-model",
    }, "artifact inventory incomplete")
    for name, item in artifacts.items():
        path = ROOT / item.get("path", "")
        require(path.is_file(), f"artifact missing: {name}")
        require(sha256(path) == item.get("sha256"), f"artifact drifted: {name}")

    require(contract.get("bounds") == {
        "plaintextBytes": 67108864,
        "ciphertextBytes": 67108864,
        "chunkBytes": 1048576,
        "chunks": 1024,
        "encryptedManifestBytes": 1048576,
        "opaqueEnvelopeBytes": 1048576,
        "draftLifetimeMS": 86400000,
        "recoveryDraftsPerRun": 100,
        "recipients": 64,
    }, "bounds drifted")

    source = (ROOT / artifacts["send-pipeline"]["path"]).read_text(encoding="utf-8")
    for token in {
        "claimProtectedMediaSendOwnership()",
        'domain: "media"',
        'idempotencyKey: "mac-protected-stage-',
        'idempotencyKey: "mac-protected-chunk-',
        'idempotencyKey: "mac-protected-finalize-',
        "Set(artifact.chunks.map(\\.nonce)).count == artifact.chunks.count",
        "sourceFingerprint",
        "appPrivateDeleteOnTerminal",
        "userOwnedRetain",
        "fixtureMode = false",
        "fixtureMode = true",
        "guard sealer.productionApproved || fixtureMode",
    }:
        require(token in source, f"send invariant missing: {token}")
    for token in {"AES.GCM", "ChaChaPoly", "AVAssetExportSession", "ffmpeg", "URLSession("}:
        require(token not in source, f"unreviewed production primitive/wiring: {token}")

    key_state = (ROOT / artifacts["key-state"]["path"]).read_text(encoding="utf-8")
    require("protectedMediaSendOwnerClaimed" in key_state,
            "single key-state send owner not enforced")
    require("guard !protectedMediaSendOwnerClaimed" in key_state,
            "second send owner does not fail closed")

    tests = (ROOT / artifacts["send-tests"]["path"]).read_text(encoding="utf-8")
    for test_name in {
        "productionInitializerCannotEnableAuditFixtureProvider",
        "fixturePipelinePublishesCiphertextAndCleansOwnedPlaintext",
        "interruptedUploadResumesExactCiphertextWithoutGenerationReuse",
        "unsupportedRecipientFailsBeforeGenerationReservation",
        "duplicateNoncesFailClosedAndConsumeReservedGeneration",
        "invalidProviderSignatureFailsClosedBeforeCiphertextPersistence",
        "sourceAndCiphertextTamperEachFailClosedOnResume",
        "explicitCancelDeletesRemoteStageCiphertextAndOwnedPlaintext",
        "expiredCrashRecoveryIsBoundedAndCleansOwnedPlaintext",
        "keyStateRepositoryAllowsOnlyOneProtectedSendOwner",
        "userOwnedSelectedFileIsRetainedAfterPublication",
        "clipTrackAndSavedCueShareTheBoundedProtectedPipeline",
    }:
        require(test_name in tests, f"fixture missing: {test_name}")

    vectors = json.loads((ROOT / artifacts["send-vectors"]["path"]).read_text(encoding="utf-8"))
    require(vectors.get("contract") == "macos-protected-media-send-v1-vectors",
            "wrong send vectors")
    require(vectors.get("status") == "audit-fixture-only-production-disabled",
            "fixture vectors represented as production")
    require(vectors.get("fixtureSuite") == "AUDIT_FIXTURE_SUITE_NOT_FOR_PRODUCTION",
            "fixture suite drifted")
    require(len(vectors.get("chunks", [])) == 2, "golden chunks drifted")
    require(len(vectors.get("failClosed", [])) == 7, "tamper vectors drifted")
    require(vectors.get("resume", {}).get("expectedSealCount") == 1,
            "resume would reseal and risk nonce reuse")

    invariants = set(contract.get("invariants", []))
    require(len(invariants) == 17, "invariant inventory drifted")
    for item in {
        "unsupported-recipient-fails-before-generation-reservation",
        "generation-reserved-before-provider-seal",
        "resume-reuses-exact-ciphertext-without-reseal",
        "source-and-ciphertext-digests-revalidated-on-resume",
        "only-app-private-owned-plaintext-is-deleted",
        "no-plaintext-fallback-or-coordinator-plaintext-route",
        "single-key-state-send-owner-per-repository",
        "runtime-composition-and-capability-remain-dark",
    }:
        require(item in invariants, f"invariant missing: {item}")

    main = (ROOT / "node-app/Sources/NodeApp/main.swift").read_text(encoding="utf-8")
    require("MacProtectedMediaSendService" not in main, "send pipeline runtime-wired")
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
    print("macOS protected-media send: PASS (production disabled)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
