#!/usr/bin/env python3
"""Fail-closed validation for production-dark macOS protected-media playback."""

from __future__ import annotations

import hashlib
import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/macos-protected-media-playback-v1.json"


class MacProtectedMediaPlaybackError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise MacProtectedMediaPlaybackError(message)


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-macos-protected-media-playback.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-tcwn44", "wrong task")
    require(contract.get("publishedAt") == "2026-07-20", "publication date drifted")
    require(contract.get("decision") == {
        "result": "production-dark-macos-protected-media-playback-foundation",
        "productionEnabled": False,
        "runtimeHTTPWired": False,
        "capabilityAdvertised": False,
        "productionLibrarySelected": False,
        "productionSuiteSelected": False,
        "productionContainerSelected": False,
        "productionDecoderSelected": False,
        "auditFixtureOnly": True,
        "plaintextFallbackAllowed": False,
        "decryptedDiskCacheAllowed": False,
        "signedPackageTested": False,
        "deltaReviewRequired": True,
    }, "production-dark decision drifted")

    artifacts = {item.get("id"): item for item in contract.get("artifacts", [])}
    require(set(artifacts) == {
        "playback-pipeline", "bounded-player", "ciphertext-cache", "playback-tests",
        "playback-vectors", "adr", "protocol-authority", "opaque-router", "threat-model",
    }, "artifact inventory incomplete")
    for name, item in artifacts.items():
        path = ROOT / item.get("path", "")
        require(path.is_file(), f"artifact missing: {name}")
        require(sha256(path) == item.get("sha256"), f"artifact drifted: {name}")

    require(contract.get("bounds") == {
        "encryptedManifestBytes": 1048576,
        "opaqueEnvelopeBytes": 1048576,
        "signatureBytes": 65536,
        "chunkBytes": 1048576,
        "variantBytes": 67108864,
        "globalCacheBytes": 536870912,
        "pinnedCacheBytes": 134217728,
        "pcmRingBytes": 1048576,
    }, "bounds drifted")

    source = (ROOT / artifacts["playback-pipeline"]["path"]).read_text(encoding="utf-8")
    for token in {
        "guard opener.productionApproved || fixtureMode",
        "authenticateAndDecrypt(",
        "try revalidate()",
        "try await cache.tombstone",
        "try await cache.invalidate",
        "historyGrantID",
        "downgradeForbidden",
        "targetChanged",
        "MacProtectedMediaRangeRequest(",
    }:
        require(token in source, f"playback invariant missing: {token}")
    for token in {"AES.GCM", "ChaChaPoly", "AVAssetReader", "ffmpeg", "URLSession("}:
        require(token not in source, f"unreviewed production primitive/wiring: {token}")

    player = (ROOT / artifacts["bounded-player"]["path"]).read_text(encoding="utf-8")
    require("protectedChunks: MacStreamChunkReading? = nil" in player,
            "authenticated reader injection missing")
    require("injectedChunks ?? MacStreamCacheReader" in player,
            "protected reader not used at decoder boundary")
    cache = (ROOT / artifacts["ciphertext-cache"]["path"]).read_text(encoding="utf-8")
    for token in {"processLock", "synchronizeLocked()", "tombstones.formUnion",
                  "UUID().uuidString"}:
        require(token in cache, f"multi-instance cache coordination missing: {token}")

    tests = (ROOT / artifacts["playback-tests"]["path"]).read_text(encoding="utf-8")
    for name in {
        "sharedMacWindowsFixtureFreezesExactAuthenticatedRanges",
        "productionRemainsDarkWithoutApprovedProvider",
        "incrementalAuthenticatedPlaybackAndCiphertextOnlyRestartCache",
        "ciphertextTamperAndUnauthenticatedPlaintextNeverReachDecoder",
        "downgradeExpiryWrongTargetAndLocalPolicyFailClosedBeforeRanges",
        "historicalEpochRequiresLiveBoundedGrant",
        "membershipChangeAndExplicitRevocationPersistAsTombstones",
        "concurrentCacheHitCannotEraseAnotherVariantsDurableTombstone",
        "boundedCandidatePlayerReceivesOnlyAuthenticatedChunkReader",
    }:
        require(name in tests, f"fixture missing: {name}")

    vectors = json.loads((ROOT / artifacts["playback-vectors"]["path"]).read_text())
    require(vectors.get("status") == "audit-fixture-only-production-disabled",
            "fixture vectors represented as production")
    require(set(vectors.get("platformProducers", [])) == {"macos-fixture", "windows-fixture"},
            "shared fixture provenance drifted")
    require(len(vectors.get("chunks", [])) == 2, "range fixture drifted")
    require(len(vectors.get("failClosed", [])) == 8, "fail-closed fixture drifted")

    invariants = set(contract.get("invariants", []))
    require(len(invariants) == 16, "invariant inventory drifted")
    for item in {
        "aead-record-authentication-required-before-decoder-bytes",
        "durable-cache-contains-ciphertext-and-public-metadata-only",
        "cached-ciphertext-is-reauthenticated-after-restart",
        "revocation-and-membership-change-purge-and-tombstone",
        "concurrent-cache-index-writes-preserve-monotonic-revocation-tombstones",
        "runtime-composition-and-capability-remain-dark",
    }:
        require(item in invariants, f"invariant missing: {item}")

    main = (ROOT / "node-app/Sources/NodeApp/main.swift").read_text(encoding="utf-8")
    require("MacProtectedMediaPlaybackService" not in main, "playback runtime-wired")
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
    print("macOS protected-media playback: PASS (production disabled)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
