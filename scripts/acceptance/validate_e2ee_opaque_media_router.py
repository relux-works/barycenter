#!/usr/bin/env python3
"""Fail-closed validation for the production-dark opaque E2EE media router."""

from __future__ import annotations

import hashlib
import json
import pathlib
import re


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/e2ee-opaque-media-router-v1.json"


class E2EEOpaqueRouterError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise E2EEOpaqueRouterError(message)


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-e2ee-opaque-media-router.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-1yz5ca", "wrong task")
    require(contract.get("publishedAt") == "2026-07-20", "publication date drifted")
    require(contract.get("decision") == {
        "result": "dormant-keyless-opaque-router-foundation",
        "productionEnabled": False,
        "runtimeHTTPWired": False,
        "capabilityAdvertised": False,
        "productionLibrarySelected": False,
        "productionSuiteSelected": False,
        "productionContainerSelected": False,
        "plaintextFallbackAllowed": False,
        "coordinatorDecryptsContent": False,
        "coordinatorPersistsLiveFrames": False,
        "deltaReviewRequired": True,
    }, "production-dark decision drifted")

    artifacts = {item.get("id"): item for item in contract.get("artifacts", [])}
    require(set(artifacts) == {
        "schema", "foundation-repository", "opaque-object-router",
        "opaque-live-router", "opaque-router-tests", "opaque-live-contract",
        "coordinator-contract-tests", "protocol-authority", "protocol-vectors",
        "threat-model", "key-lifecycle-packet", "schema-foundation-packet",
        "routing-rotation-packet", "adr",
    }, "artifact inventory incomplete")
    for name, item in artifacts.items():
        path = ROOT / item.get("path", "")
        require(path.is_file(), f"artifact missing: {name}")
        require(sha256(path) == item.get("sha256"), f"artifact drifted: {name}")

    schema = (ROOT / artifacts["schema"]["path"]).read_text(encoding="utf-8")
    tables = set(contract.get("additiveTables", []))
    require(tables == {
        "e2ee_protected_object_recipients", "e2ee_protected_object_chunks",
        "e2ee_protected_egress_usage", "e2ee_opaque_live_sessions",
        "e2ee_opaque_live_recipients",
    }, "opaque router table inventory drifted")
    for table in tables:
        require(f"CREATE TABLE IF NOT EXISTS {table}" in schema, f"table missing: {table}")
    for trigger in {
        "e2ee_protected_object_recipient_immutable",
        "e2ee_protected_object_chunk_immutable",
        "e2ee_opaque_live_binding_immutable",
        "e2ee_opaque_live_recipient_binding_immutable",
    }:
        require(trigger in schema, f"immutability trigger missing: {trigger}")
    require("enabled INTEGER NOT NULL DEFAULT 0 CHECK(enabled = 0)" in schema,
            "E2EE production lock removed")

    declared_columns = set(re.findall(
        r"^\s{2}([a-z][a-z0-9_]*)\s+(?:TEXT|INTEGER|BLOB)\b", schema, re.MULTILINE
    ))
    forbidden = {
        "private_key", "key_package_private_key", "epoch_secret", "sender_key",
        "content_key", "recovery_secret", "history_grant_secret", "plaintext",
        "decrypted_evidence", "decoded_audio", "live_frame_payload",
    }
    require(not (declared_columns & forbidden), "secret/plaintext storage column admitted")

    bounds = contract.get("bounds", {})
    require(bounds == {
        "objectBytes": 67108864,
        "objectChunks": 1024,
        "chunkBytes": 1048576,
        "rangeBytes": 4194304,
        "concurrentStagedObjectsPerActor": 4,
        "rollingUploadBytesPerActor": 536870912,
        "rangeAdmissionFloorBytes": 1048576,
        "rollingEgressBytesPerDevice": 536870912,
        "liveCiphertextBytes": 512,
        "liveBurstFrames": 8,
        "liveFramesPerSecond": 50,
        "liveGapFrames": 8,
        "liveDurationMS": 300000,
    }, "resource bounds drifted")

    object_router = (ROOT / artifacts["opaque-object-router"]["path"]).read_text(encoding="utf-8")
    live_router = (ROOT / artifacts["opaque-live-router"]["path"]).read_text(encoding="utf-8")
    live_contract = (ROOT / artifacts["opaque-live-contract"]["path"]).read_text(encoding="utf-8")
    require('copy(result[0:2], []byte("BE"))' in live_contract,
            "distinct protected live magic missing")
    require('string(raw[0:2]) != "BE"' in live_contract,
            "protected live decoder magic check missing")
    require('[]byte("BP")' not in live_contract, "legacy plaintext magic admitted")
    require("DecodeOpaqueLiveFrame" in live_router and "DecodeLivePTTBinaryFrame" not in live_router,
            "opaque live route downgraded to legacy decoder")
    require("e2ee_protected_object_chunks" in object_router and
            "payloadDigestMatches" in object_router and
            "StreamRangeRequestChargeBytes" in object_router,
            "ciphertext hash/range/quota boundary incomplete")
    require(all(token not in object_router + live_router for token in {
        "media_items", "stream_track_variants", "transmissions",
        "transmission_inbox_items", "saved_cues",
    }), "opaque router wrote a legacy plaintext service")
    require(all(token not in object_router + live_router for token in {
        "slog.", "log.Printf", "fmt.Printf", "logger.",
    }), "opaque bytes can enter ordinary logs")

    invariants = set(contract.get("routingInvariants", []))
    require(len(invariants) == 15, "routing invariant inventory drifted")
    for item in {
        "whole-object-count-length-and-digest-before-finalize",
        "membership-change-requires-rotation-before-upload-finalize-or-fetch",
        "non-target-revoked-removed-rejoined-and-forked-fetch-denied",
        "protected-BE-frame-cannot-downgrade-to-legacy-BP-frame",
        "restart-terminates-live-session-and-generation-cannot-reset",
        "live-frame-payload-never-persisted",
        "legacy-plaintext-services-remain-separate",
    }:
        require(item in invariants, f"routing invariant missing: {item}")
    require(all(value == "covered" for value in contract.get("fixtures", {}).values()),
            "fixture represented as covered without evidence")
    require(set(contract.get("openProductionGates", [])) == {
        "EPC-001", "EPC-002", "EPC-004", "EPC-005", "TASK-260712-1ulshp",
    }, "production gate hidden or closed")

    manual = contract.get("manualEvidence", {})
    require(manual.get("epic") == "EPIC-260714-th54l3", "manual epic missing")
    for key, value in manual.items():
        if key != "epic":
            require(value in {"not-run", "not-run-no-selected-stack"},
                    f"manual evidence invented: {key}")

    production_sources = [
        ROOT / "coordinator/internal/protocol/protocol.go",
        ROOT / "coordinator/cmd/duet-coordinator/main.go",
        ROOT / "pulsar-win/main.go",
        ROOT / "node-app/Sources/NodeCore/Protocol.swift",
        ROOT / "node-app/Sources/NodeApp/main.swift",
    ]
    for path in production_sources:
        require("e2ee_media_v1" not in path.read_text(encoding="utf-8"),
                f"capability leaked into production: {path}")
    require(not list((ROOT / "protocol/golden").glob("*e2ee*")),
            "draft E2EE added to production golden wire catalog")


def main() -> int:
    validate(load())
    print("e2ee opaque media router: PASS (production disabled)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
