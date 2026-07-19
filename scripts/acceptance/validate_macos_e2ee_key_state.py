#!/usr/bin/env python3
"""Fail-closed validation for the production-dark macOS E2EE key-state boundary."""

from __future__ import annotations

import hashlib
import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/macos-e2ee-key-state-v1.json"


class MacE2EEKeyStateError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise MacE2EEKeyStateError(message)


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-macos-e2ee-key-state.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-1x9ruo", "wrong task")
    require(contract.get("publishedAt") == "2026-07-20", "publication date drifted")
    require(contract.get("decision") == {
        "result": "production-dark-device-only-key-state-foundation",
        "productionEnabled": False,
        "runtimeWired": False,
        "capabilityAdvertised": False,
        "productionLibrarySelected": False,
        "productionSuiteSelected": False,
        "productionContainerSelected": False,
        "hardwareKeychainTested": False,
        "signedPackageTested": False,
        "plaintextFallbackAllowed": False,
        "deltaReviewRequired": True,
    }, "production-dark decision drifted")

    artifacts = {item.get("id"): item for item in contract.get("artifacts", [])}
    require(set(artifacts) == {
        "repository", "repository-tests", "state-vectors", "adr", "threat-model",
        "key-lifecycle", "schema-foundation", "routing-rotation", "opaque-router",
    }, "artifact inventory incomplete")
    for name, item in artifacts.items():
        path = ROOT / item.get("path", "")
        require(path.is_file(), f"artifact missing: {name}")
        require(sha256(path) == item.get("sha256"), f"artifact drifted: {name}")

    policy = contract.get("keychainPolicy", {})
    require(policy == {
        "service": "works.relux.pulsar.e2ee",
        "accessibility": "kSecAttrAccessibleWhenUnlockedThisDeviceOnly",
        "synchronizable": False,
        "dataProtectionKeychain": True,
        "preferencesStorage": False,
        "installationRandomBytes": 32,
    }, "Keychain policy drifted")
    require(set(contract.get("keychainSlots", [])) == {
        "device_metadata", "device_signing", "device_agreement",
        "group", "grant", "content_cache",
    }, "Keychain slot separation drifted")
    require(contract.get("bounds") == {
        "privateKeyBytes": 4096,
        "opaqueGroupStateBytes": 1048576,
        "grantBytes": 65536,
        "cachedContentKeys": 32,
        "cachedContentKeyBytes": 65536,
        "individualContentKeyBytes": 4096,
    }, "key-state bounds drifted")

    source = (ROOT / artifacts["repository"]["path"]).read_text(encoding="utf-8")
    for token in {
        'static let service = "works.relux.pulsar.e2ee"',
        "kSecAttrAccessibleWhenUnlockedThisDeviceOnly",
        "kSecAttrSynchronizable as String: false",
        "kSecUseDataProtectionKeychain as String: true",
        "SecRandomCopyBytes(kSecRandomDefault, count",
        'case deviceMetadata = "device_metadata"',
        'case deviceSigning = "device_signing"',
        'case deviceAgreement = "device_agreement"',
        "previousCommitDigest == payload.commitDigest",
        "epoch == payload.epoch + 1",
        "try deleteSlot(kind: .group",
    }:
        require(token in source, f"key-state source boundary missing: {token}")
    for token in {"UserDefaults", "os_log", "Logger(", "print(", "e2ee_media_v1"}:
        require(token not in source, f"forbidden key-state source token: {token}")

    record_write = min(source.index("try store.add(recordData"),
                       source.index("try store.update(recordData"))
    record_read = source.index("guard try store.read(account: accounts.record) == recordData")
    witness_write = min(source.index("try store.add(witnessData"),
                        source.index("try store.update(witnessData"))
    witness_read = source.index("guard try store.read(account: accounts.witness) == witnessData")
    final_reload = source.index("try loadRecord(kind: kind, scope: scope", witness_read)
    require(record_write < record_read < witness_write < witness_read < final_reload,
            "persist-before-ack ordering drifted")

    vectors = json.loads((ROOT / artifacts["state-vectors"]["path"]).read_text(encoding="utf-8"))
    require(vectors.get("contract") == "e2ee-key-state.v1", "wrong state vectors")
    require(vectors.get("status") == "production-disabled", "state vectors enabled")
    require(len(vectors.get("transitions", [])) == 5, "transition vector inventory drifted")
    require({item.get("name") for item in vectors.get("crash_vectors", [])} == {
        "record-written-witness-not-written",
        "record-and-witness-written-readback-lost",
    }, "crash vector inventory drifted")
    target_results = {item.get("name"): item.get("expected")
                      for item in vectors.get("target_vectors", [])}
    require(target_results.get("only-revoked-devices") == "removed_endpoint",
            "EPC-005 revoked-only semantics drifted")

    invariants = set(contract.get("stateInvariants", []))
    require(len(invariants) == 13, "state invariant inventory drifted")
    for item in {
        "device-metadata-signing-and-agreement-use-distinct-slots",
        "state-and-witness-readback-complete-before-ack",
        "ambiguous-success-consumes-revision-and-generation",
        "group-epoch-advances-exactly-one-with-exact-predecessor-digest",
        "no-preferences-logs-telemetry-crash-or-runtime-capability-wiring",
        "revoked-only-active-member-is-a-removed-endpoint",
    }:
        require(item in invariants, f"state invariant missing: {item}")
    require(len(contract.get("fixtures", {})) == 14 and
            all(value == "covered" for value in contract.get("fixtures", {}).values()),
            "fixture represented as covered without evidence")

    composition_root = (ROOT / "node-app/Sources/NodeApp/main.swift").read_text(encoding="utf-8")
    require("MacE2EEKeyStateRepository" not in composition_root,
            "key state wired into production composition root")
    require("e2ee_media_v1" not in composition_root,
            "E2EE capability advertised by production composition root")

    manual = contract.get("manualEvidence", {})
    require(manual.get("epic") == "EPIC-260714-th54l3", "manual epic missing")
    for key, value in manual.items():
        if key != "epic":
            require(value in {"not-run", "not-run-no-selected-stack"},
                    f"manual evidence invented: {key}")
    require(set(contract.get("openProductionGates", [])) == {
        "EPC-001", "EPC-002", "EPC-004", "EPC-005", "TASK-260712-1ulshp",
    }, "production gate hidden or closed")


def main() -> int:
    validate(load())
    print("macOS E2EE key state: PASS (production disabled)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
