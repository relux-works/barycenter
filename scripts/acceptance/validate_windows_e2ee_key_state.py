#!/usr/bin/env python3
"""Fail-closed validation for the production-dark Windows E2EE key-state boundary."""

from __future__ import annotations

import hashlib
import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/windows-e2ee-key-state-v1.json"


class WindowsE2EEKeyStateError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise WindowsE2EEKeyStateError(message)


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-windows-e2ee-key-state.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-25dzp4", "wrong task")
    require(contract.get("publishedAt") == "2026-07-20", "publication date drifted")
    require(contract.get("decision") == {
        "result": "production-dark-current-user-dpapi-key-state-foundation",
        "productionEnabled": False,
        "runtimeWired": False,
        "capabilityAdvertised": False,
        "productionLibrarySelected": False,
        "productionSuiteSelected": False,
        "productionContainerSelected": False,
        "nativeDPAPITested": False,
        "signedPackageTested": False,
        "plaintextFallbackAllowed": False,
        "deltaReviewRequired": True,
    }, "production-dark decision drifted")

    artifacts = {item.get("id"): item for item in contract.get("artifacts", [])}
    require(set(artifacts) == {
        "repository", "windows-default", "nonwindows-default", "repository-tests",
        "dpapi-adapter", "windows-file-adapter", "state-vectors", "adr",
        "macos-key-state", "threat-model", "key-lifecycle", "schema-foundation",
        "routing-rotation", "opaque-router",
    }, "artifact inventory incomplete")
    for name, item in artifacts.items():
        path = ROOT / item.get("path", "")
        require(path.is_file(), f"artifact missing: {name}")
        require(sha256(path) == item.get("sha256"), f"artifact drifted: {name}")

    require(contract.get("dpapiPolicy") == {
        "scope": "current-user",
        "uiForbidden": True,
        "localMachine": False,
        "optionalEntropy": False,
        "secretDescription": False,
        "nonWindowsPlaintextFallback": False,
    }, "DPAPI policy drifted")
    require(contract.get("durabilityPolicy") == {
        "crossProcessExclusiveLock": True,
        "temporaryFilesContainCiphertextOnly": True,
        "writeThroughTemporaryHandle": True,
        "flushBeforeClose": True,
        "replaceAndWriteThroughMove": True,
        "destinationReadback": True,
        "independentWitness": True,
    }, "durability policy drifted")
    require(set(contract.get("protectedSlots", [])) == {
        "device_metadata", "device_signing", "device_agreement",
        "group", "grant", "content_cache",
    }, "protected slot separation drifted")
    require(contract.get("bounds") == {
        "privateKeyBytes": 4096,
        "opaqueGroupStateBytes": 1048576,
        "grantBytes": 65536,
        "cachedContentKeys": 32,
        "cachedContentKeyBytes": 65536,
        "individualContentKeyBytes": 4096,
        "protectedPlaintextEnvelopeBytes": 3145728,
        "protectedCiphertextBytes": 4194304,
    }, "key-state bounds drifted")

    source = (ROOT / artifacts["repository"]["path"]).read_text(encoding="utf-8")
    windows_default = (ROOT / artifacts["windows-default"]["path"]).read_text(encoding="utf-8")
    nonwindows_default = (ROOT / artifacts["nonwindows-default"]["path"]).read_text(encoding="utf-8")
    dpapi = (ROOT / artifacts["dpapi-adapter"]["path"]).read_text(encoding="utf-8")
    native_files = (ROOT / artifacts["windows-file-adapter"]["path"]).read_text(encoding="utf-8")
    for token in {
        "AcquireLock(lockPath)",
        "moveReplaceExisting|moveWriteThrough",
        "fileFlagWriteThrough",
        "Flush(handle)",
        "windowsE2EEEnvelopeMagic = [4]byte{'B', 'E', 'K', 'S'}",
        'windowsE2EEDeviceMetadata  windowsE2EEKind = "device_metadata"',
        'windowsE2EEDeviceSigning   windowsE2EEKind = "device_signing"',
        'windowsE2EEDeviceAgreement windowsE2EEKind = "device_agreement"',
        "previousCommitDigest != previous.CommitDigest",
        "epoch != previous.Epoch+1",
        "r.deleteSlot(windowsE2EEGroup",
    }:
        require(token in source, f"key-state source boundary missing: {token}")
    for token in {"config.json", "log.Printf", "slog.", "fmt.Printf", "e2ee_media_v1"}:
        require(token not in source, f"forbidden key-state source token: {token}")
    require("dpapiDataProtector{api: windowsDataProtectionAPI{}}" in windows_default,
            "Windows DPAPI default missing")
    require("return nil, ErrWindowsE2EEUnavailable" in nonwindows_default,
            "non-Windows plaintext fallback admitted")
    require("p.api.Protect(plaintext, cryptprotectUIForbidden)" in dpapi and
            "p.api.Unprotect(ciphertext, cryptprotectUIForbidden)" in dpapi,
            "current-user DPAPI flags drifted")
    require("spec.Share" in native_files and "windows.CreateFile" in native_files,
            "native share-none lock path missing")

    state_write = source.index("r.writeProtectedBytes(statePath, recordBytes)")
    witness_write = source.index("r.writeProtectedBytes(witnessPath, witnessBytes)")
    final_reload = source.index("r.loadRecord(kind, scope, installationID)", witness_write)
    require(state_write < witness_write < final_reload,
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
    require(len(invariants) == 14, "state invariant inventory drifted")
    for item in {
        "device-metadata-signing-and-agreement-use-distinct-dpapi-files",
        "repository-wide-share-none-lock-covers-read-validate-write-readback-ack",
        "state-and-witness-readback-complete-before-ack",
        "ambiguous-success-consumes-revision-and-generation",
        "group-epoch-advances-exactly-one-with-exact-predecessor-digest",
        "no-config-logs-telemetry-crash-or-runtime-capability-wiring",
        "revoked-only-active-member-is-a-removed-endpoint",
    }:
        require(item in invariants, f"state invariant missing: {item}")
    require(len(contract.get("fixtures", {})) == 16 and
            all(value == "covered" for value in contract.get("fixtures", {}).values()),
            "fixture represented as covered without evidence")

    for path in (ROOT / "pulsar-win").glob("*.go"):
        if path.name.endswith("_test.go"):
            continue
        if path.name in {
            "windows_e2ee_key_state.go", "windows_e2ee_key_state_default_windows.go",
            "windows_e2ee_key_state_default_other.go", "windows_e2ee_key_state_test.go",
        }:
            continue
        production = path.read_text(encoding="utf-8")
        require("WindowsE2EEKeyStateRepository" not in production and
                "newDefaultWindowsE2EEKeyStateRepository" not in production,
                f"key state wired into production source: {path.name}")

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
    print("Windows E2EE key state: PASS (production disabled)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
