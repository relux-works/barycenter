#!/usr/bin/env python3
"""Fail-closed validation for production-dark Windows protected-media playback."""

from __future__ import annotations

import hashlib
import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/windows-protected-media-playback-v1.json"


class WindowsProtectedMediaPlaybackError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise WindowsProtectedMediaPlaybackError(message)


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-windows-protected-media-playback.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-1u57qz", "wrong task")
    require(contract.get("publishedAt") == "2026-07-20", "publication date drifted")
    require(contract.get("decision") == {
        "result": "production-dark-windows-protected-media-playback-foundation",
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
        "nativeDPAPIVerified": False,
        "deltaReviewRequired": True,
    }, "production-dark decision drifted")

    artifacts = {item.get("id"): item for item in contract.get("artifacts", [])}
    require(set(artifacts) == {
        "playback-pipeline", "bounded-player", "ciphertext-cache", "playback-tests",
        "playback-vectors", "macos-parity-vectors", "windows-key-state",
        "protocol-authority", "opaque-router", "threat-model", "adr",
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
        "!s.opener.ProductionApproved() && !s.fixtureMode",
        "newWindowsProtectedMediaPlaybackServiceForAudit",
        "AuthenticateAndDecrypt(",
        "authorizeAndRevalidate()",
        "LoadGrant(installationID, request.HistoryGrantID, checkedAt)",
        "current.Metadata.CommitDigest != frozenGroup.CommitDigest",
        "windowsProtectedMediaRevocationPath",
        "r.markRevoked()",
        "r.cache.Tombstone",
        "r.cache.Invalidate",
        "WindowsProtectedMediaPlaybackRangeRequest{",
        "cloneWindowsProtectedMediaPlaybackRoute",
    }:
        require(token in source, f"playback invariant missing: {token}")
    for token in {"aes.", "chacha", "ffmpeg", "exec.Command", "http.NewRequest", "websocket"}:
        require(token.lower() not in source.lower(), f"unreviewed production primitive/wiring: {token}")

    player = (ROOT / artifacts["bounded-player"]["path"]).read_text(encoding="utf-8")
    for token in {
        "chunks WindowsStreamChunkReader",
        "newWindowsStreamCandidatePlayer",
        "reflect.DeepEqual(player.chunks.Manifest(), manifest)",
        "chunks := player.chunks",
        "verifyErr = verifier.VerifyWhole()",
    }:
        require(token in player, f"authenticated player injection missing: {token}")
    cache = (ROOT / artifacts["ciphertext-cache"]["path"]).read_text(encoding="utf-8")
    require('cache.hmac("variant", manifest.Identity, manifest.VariantURL, manifest.ETag)' in cache,
            "cache authority does not include exact route")
    for token in {
        "globalWindowsStreamProcessLocks.lockFor",
        "mergePersistedIndexProcessLocked",
        "cache.tombstones[value] = true",
        'os.CreateTemp(cache.dir, ".index-*.part")',
    }:
        require(token in cache, f"multi-instance cache coordination missing: {token}")

    tests = (ROOT / artifacts["playback-tests"]["path"]).read_text(encoding="utf-8")
    for name in {
        "TestWindowsProtectedMediaPlaybackSharedFixtureParity",
        "TestWindowsProtectedMediaPlaybackProductionAndPolicyRemainDark",
        "TestWindowsProtectedMediaPlaybackIncrementalRestartCiphertextOnly",
        "TestWindowsProtectedMediaPlaybackTamperNeverReachesDecoder",
        "TestWindowsProtectedMediaPlaybackDowngradeExpiryTargetAndGrantFailClosed",
        "TestWindowsProtectedMediaPlaybackRevocationMarkerIsMonotonicAcrossActors",
        "TestWindowsProtectedMediaPlaybackConcurrentDistinctActorsMergeDurableCache",
        "TestWindowsProtectedMediaPlaybackMembershipRotationPurgesButAllowsBoundedRegrant",
        "TestWindowsProtectedMediaPlaybackCandidatePlayerGetsAuthenticatedReader",
        "TestWindowsProtectedMediaOpenLeaseRedactsAndZeros",
    }:
        require(name in tests, f"fixture missing: {name}")

    vectors = json.loads((ROOT / artifacts["playback-vectors"]["path"]).read_text(encoding="utf-8"))
    macos = json.loads((ROOT / artifacts["macos-parity-vectors"]["path"]).read_text(encoding="utf-8"))
    require(vectors.get("contract") == "windows-protected-media-playback-v1-vectors", "wrong vectors")
    require(vectors.get("status") == "audit-fixture-only-production-disabled",
            "fixture represented as production")
    for key in {"fixtureSuite", "fixtureContainer", "platformProducers", "ciphertextSHA256", "chunks", "bounds"}:
        require(vectors.get(key) == macos.get(key), f"macOS fixture parity drifted: {key}")
    require(len(vectors.get("failClosed", [])) == 9, "fail-closed fixture drifted")

    invariants = set(contract.get("invariants", []))
    require(len(invariants) == 20, "invariant inventory drifted")
    for item in {
        "provider-record-authentication-required-before-decoder-bytes",
        "durable-cache-contains-ciphertext-and-hmac-obscured-public-metadata-only",
        "cache-authority-includes-exact-variant-url-not-only-content-labels",
        "concurrent-cache-actors-serialize-read-merge-write-with-monotonic-tombstones",
        "transport-provider-and-caller-route-slices-are-defensively-frozen",
        "monotonic-route-scoped-revocation-marker-survives-parallel-actors-and-restart",
        "existing-generation-seek-clock-volume-receipt-and-pcm-bounds-preserved",
        "no-plaintext-fallback-or-coordinator-decrypt-path",
    }:
        require(item in invariants, f"invariant missing: {item}")

    runtime = "\n".join(
        path.read_text(encoding="utf-8")
        for path in (ROOT / "pulsar-win").glob("*.go")
        if path.name != "windows_protected_media_playback.go" and not path.name.endswith("_test.go")
    )
    require("WindowsProtectedMediaPlaybackService" not in runtime, "playback runtime-wired")

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
    print("Windows protected-media playback: PASS (production disabled)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
