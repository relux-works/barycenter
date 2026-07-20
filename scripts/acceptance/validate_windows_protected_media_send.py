#!/usr/bin/env python3
"""Fail-closed validation for production-dark Windows protected-media send."""

from __future__ import annotations

import hashlib
import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/windows-protected-media-send-v1.json"


class WindowsProtectedMediaSendError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise WindowsProtectedMediaSendError(message)


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-windows-protected-media-send.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-28zhpl", "wrong task")
    require(contract.get("publishedAt") == "2026-07-20", "publication date drifted")
    require(contract.get("decision") == {
        "result": "production-dark-windows-protected-media-send-foundation",
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
        "nativeDPAPIVerified": False,
        "deltaReviewRequired": True,
    }, "production-dark decision drifted")

    artifacts = {item.get("id"): item for item in contract.get("artifacts", [])}
    require(set(artifacts) == {
        "send-pipeline", "send-tests", "send-vectors", "windows-key-state",
        "macos-parity-vectors", "protocol-authority", "opaque-router", "adr",
        "threat-model", "design-review",
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
        'ReserveSendGeneration(installationID, request.GroupID, "media"',
        '"windows-protected-stage-"',
        '"windows-protected-chunk-%s-%d"',
        '"windows-protected-finalize-"',
        '"windows-protected-delete-"',
        "zeroBytes(plaintext)",
        "group.Metadata.CommitDigest != draft.CommitDigest",
        "group.Metadata.TargetSnapshotDigest != draft.Context.TargetSnapshotDigest",
        "decoder.DisallowUnknownFields()",
        "s.isActive(entry.Name())",
        'os.MkdirTemp(s.ciphertextRoot, ".prepare-"+request.DraftID+"-")',
        "os.Rename(directory, finalDirectory)",
        "errors.Is(statErr, os.ErrNotExist)",
        "WindowsProtectedMediaAppPrivateDeleteOnTerminal",
        "WindowsProtectedMediaUserOwnedRetain",
        "!s.sealer.ProductionApproved() && !s.fixtureMode",
        "newWindowsProtectedMediaSendServiceForAudit",
    }:
        require(token in source, f"send invariant missing: {token}")
    for token in {"AES", "ChaCha", "ffmpeg", "exec.Command", "http.NewRequest", "websocket"}:
        require(token not in source, f"unreviewed production primitive/wiring: {token}")

    stage = source.split("type WindowsProtectedMediaStageRequest struct", 1)[1].split(
        "type WindowsProtectedMediaRemoteObject", 1
    )[0]
    for token in {"SourcePath", "Plaintext", "ContentKey", "PrivateKey"}:
        require(token not in stage, f"plaintext/secret entered upload shape: {token}")

    key_state = (ROOT / artifacts["windows-key-state"]["path"]).read_text(encoding="utf-8")
    for token in {"AcquireLock(lockPath)", "fileShareNone", "moveReplaceExisting|moveWriteThrough"}:
        require(token in key_state, f"cross-process key-state serialization missing: {token}")

    tests = (ROOT / artifacts["send-tests"]["path"]).read_text(encoding="utf-8")
    for test_name in {
        "TestWindowsProtectedMediaProductionProviderGate",
        "TestWindowsProtectedMediaPublishesGoldenCiphertextAndCleansOwnedPlaintext",
        "TestWindowsProtectedMediaInterruptedUploadResumesExactCiphertext",
        "TestWindowsProtectedMediaTargetFailuresPrecedeGenerationReservation",
        "TestWindowsProtectedMediaProviderFailuresConsumeReservationWithoutPersistence",
        "TestWindowsProtectedMediaResumeRejectsSourceCiphertextAuthorAndEpochDrift",
        "TestWindowsProtectedMediaCancelAndExpiryDeleteRemoteAndOwnedPlaintext",
        "TestWindowsProtectedMediaUserOwnedSourceRetainedAndKindsSharePipeline",
        "TestWindowsProtectedMediaConcurrentDuplicateDraftFailsBusyAndRecoverySkipsActive",
        "TestWindowsProtectedMediaStoredStateRejectsUnknownFields",
        "TestWindowsProtectedMediaPublishedCheckpointDoesNotRefinalizeAfterCleanupRetry",
        "TestWindowsProtectedMediaAlreadyMissingOwnedPlaintextCleanupConverges",
        "TestWindowsProtectedMediaStateLessFinalOrphanIsRecoverableAndDoesNotConsumeGeneration",
    }:
        require(test_name in tests, f"fixture missing: {test_name}")

    vectors = json.loads((ROOT / artifacts["send-vectors"]["path"]).read_text(encoding="utf-8"))
    macos = json.loads((ROOT / artifacts["macos-parity-vectors"]["path"]).read_text(encoding="utf-8"))
    require(vectors.get("contract") == "windows-protected-media-send-v1-vectors", "wrong vectors")
    require(vectors.get("status") == "audit-fixture-only-production-disabled", "fixture represented as production")
    require(vectors.get("crossPlatformFixture") == macos.get("contract"), "cross-platform fixture authority drifted")
    for key in {"fixtureSuite", "fixtureContainer", "sourceSHA256", "manifestSHA256", "ciphertextSHA256", "chunks", "resume"}:
        require(vectors.get(key) == macos.get(key), f"macOS fixture parity drifted: {key}")
    require(len(vectors.get("failClosed", [])) == 11, "fail-closed inventory drifted")

    invariants = set(contract.get("invariants", []))
    require(len(invariants) == 22, "invariant inventory drifted")
    for item in {
        "unsupported-or-removed-recipient-fails-before-generation-reservation",
        "windows-share-none-key-state-lock-serializes-generation-cross-process",
        "service-owned-plaintext-buffer-zeroed-after-provider-return",
        "resume-reuses-exact-ciphertext-without-reseal-or-generation-reuse",
        "resume-rechecks-author-epoch-commit-target-and-source",
        "cancel-and-expiry-delete-staged-remote-before-local-cleanup",
        "active-drafts-are-not-expiry-cleaned",
        "initial-ciphertext-and-state-published-by-atomic-draft-directory-rename",
        "terminal-cleanup-is-idempotent-when-owned-plaintext-is-already-absent",
        "finalized-remote-revision-checkpointed-before-terminal-cleanup",
        "no-plaintext-fallback-or-coordinator-plaintext-route",
        "runtime-composition-and-capability-remain-dark",
    }:
        require(item in invariants, f"invariant missing: {item}")

    runtime = "\n".join(
        path.read_text(encoding="utf-8")
        for path in (ROOT / "pulsar-win").glob("*.go")
        if path.name not in {
            "windows_protected_media_send.go", "windows_encrypted_media_client.go",
        } and not path.name.endswith("_test.go")
    )
    require("WindowsProtectedMediaSendService" not in runtime, "send pipeline runtime-wired")
    client = (ROOT / "pulsar-win/windows_encrypted_media_client.go").read_text(encoding="utf-8")
    require("NewWindowsProtectedMediaSendService" in client and
            "intentionally absent from" in client and
            "CapabilityAdvertised: false" in client and
            "RuntimeWiringApproved: false" in client,
            "Windows client is not the exact production-dark send integration boundary")

    manual = contract.get("manualEvidence", {})
    require(manual.get("epic") == "EPIC-260714-th54l3", "manual epic missing")
    for name, value in manual.items():
        if name != "epic":
            require(value in {"not-run", "not-run-no-selected-stack"}, f"manual evidence invented: {name}")
    require(set(contract.get("openProductionGates", [])) == {
        "EPC-001", "EPC-002", "EPC-004", "EPC-005", "TASK-260712-1ulshp",
    }, "production gate hidden or closed")


def main() -> int:
    validate(load())
    print("Windows protected-media send: PASS (production disabled)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
