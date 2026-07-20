#!/usr/bin/env python3
"""Generate the deterministic E2EE C4-C6 engineering review handoff packet."""

from __future__ import annotations

import argparse
import hashlib
import json
import pathlib
import subprocess
from typing import Iterable


ROOT = pathlib.Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT = ROOT / "acceptance/phase3/e2ee-c4-c6-engineering-review-pack-v1.json"
SOURCE_COMMIT = "9d7ace6dc7337cd2191f35b0d8373228cf759398"
SOURCE_TREE = "ef819c9bd3e18e7532630510622f28e486f20007"
DESIGN_REVIEW_MERGE = "4dd9612"

COMPONENTS = (
    ("TASK-260712-2e2ymn", "acceptance/phase3/e2ee-threat-model-v1.json", "scripts/acceptance/test_e2ee_threat_model.py"),
    ("TASK-260712-16xmy2", "acceptance/phase3/protected-media-container-spike-v1.json", "scripts/acceptance/test_protected_media_container_spike.py"),
    ("TASK-260712-3er89x", "acceptance/phase3/group-crypto-library-spike-v1.json", "scripts/acceptance/test_group_crypto_library_spike.py"),
    ("TASK-260712-2ys1ww", "acceptance/phase3/e2ee-protocol-key-lifecycle-v1.json", "scripts/acceptance/test_e2ee_protocol_key_lifecycle.py"),
    ("TASK-260712-3w1cst", "acceptance/phase3/e2ee-schema-epoch-foundation-v1.json", "scripts/acceptance/test_e2ee_schema_epoch_foundation.py"),
    ("TASK-260712-20j5tm", "acceptance/phase3/e2ee-coordinator-routing-rotation-v1.json", "scripts/acceptance/test_e2ee_coordinator_routing_rotation.py"),
    ("TASK-260712-1yz5ca", "acceptance/phase3/e2ee-opaque-media-router-v1.json", "scripts/acceptance/test_e2ee_opaque_media_router.py"),
    ("TASK-260712-1x9ruo", "acceptance/phase3/macos-e2ee-key-state-v1.json", "scripts/acceptance/test_macos_e2ee_key_state.py"),
    ("TASK-260712-25dzp4", "acceptance/phase3/windows-e2ee-key-state-v1.json", "scripts/acceptance/test_windows_e2ee_key_state.py"),
    ("TASK-260712-2i0w6x", "acceptance/phase3/e2ee-report-evidence-moderation-export-v1.json", "scripts/acceptance/test_e2ee_report_moderation_export.py"),
    ("TASK-260712-1rziyo", "acceptance/phase3/e2ee-recovery-device-transfer-v1.json", "scripts/acceptance/test_e2ee_recovery_device_transfer.py"),
    ("TASK-260712-2kcduo", "acceptance/phase3/macos-protected-media-send-v1.json", "scripts/acceptance/test_macos_protected_media_send.py"),
    ("TASK-260712-tcwn44", "acceptance/phase3/macos-protected-media-playback-v1.json", "scripts/acceptance/test_macos_protected_media_playback.py"),
    ("TASK-260712-3980vy", "acceptance/phase3/macos-e2ee-live-ptt-v1.json", "scripts/acceptance/test_macos_e2ee_live_ptt.py"),
    ("TASK-260712-28zhpl", "acceptance/phase3/windows-protected-media-send-v1.json", "scripts/acceptance/test_windows_protected_media_send.py"),
    ("TASK-260712-1u57qz", "acceptance/phase3/windows-protected-media-playback-v1.json", "scripts/acceptance/test_windows_protected_media_playback.py"),
    ("TASK-260712-39vjzd", "acceptance/phase3/windows-e2ee-live-ptt-v1.json", "scripts/acceptance/test_windows_e2ee_live_ptt.py"),
    ("TASK-260712-2nppt6", "acceptance/phase3/macos-encrypted-media-client-path-v1.json", "scripts/acceptance/test_macos_encrypted_media_client_path.py"),
    ("TASK-260712-2q4jbu", "acceptance/phase3/windows-encrypted-media-client-path-v1.json", "scripts/acceptance/test_windows_encrypted_media_client_path.py"),
)

TERMINAL_REVIEWS = (
    ("TASK-260712-aniuyy", ".task-board/.resources/TASK-260712-aniuyy/TASK-260712-aniuyy_independent-design-review-v1.md"),
    ("TASK-260712-3w1cst", ".task-board/.resources/TASK-260712-3w1cst/TASK-260712-3w1cst_independent-delta-review-v1.md"),
    ("TASK-260712-20j5tm", ".task-board/.resources/TASK-260712-20j5tm/TASK-260712-20j5tm_independent-delta-review-v1.md"),
    ("TASK-260712-1yz5ca", ".task-board/.resources/TASK-260712-1yz5ca/independent-delta-review-completion.md"),
    ("TASK-260712-1x9ruo", ".task-board/.resources/TASK-260712-1x9ruo/TASK-260712-1x9ruo_independent-delta-review-v1.md"),
    ("TASK-260712-25dzp4", ".task-board/.resources/TASK-260712-25dzp4/TASK-260712-25dzp4_independent-cleanup-delta-review-v2.md"),
    ("TASK-260712-2i0w6x", ".task-board/.resources/TASK-260712-2i0w6x/TASK-260712-2i0w6x_independent-exact-sha-review.md"),
    ("TASK-260712-1rziyo", ".task-board/.resources/TASK-260712-1rziyo/TASK-260712-1rziyo_review-verdict.md"),
    ("TASK-260712-2kcduo", ".task-board/.resources/TASK-260712-2kcduo/TASK-260712-2kcduo_review-verdict.md"),
    ("TASK-260712-tcwn44", ".task-board/.resources/TASK-260712-tcwn44/TASK-260712-tcwn44_review-verdict-8c26762.md"),
    ("TASK-260712-3980vy", ".task-board/.resources/TASK-260712-3980vy/TASK-260712-3980vy_review-verdict-v2.md"),
    ("TASK-260712-28zhpl", ".task-board/.resources/TASK-260712-28zhpl/TASK-260712-28zhpl_re-review-verdict-b2a4af6.md"),
    ("TASK-260712-1u57qz", ".task-board/.resources/TASK-260712-1u57qz/TASK-260712-1u57qz_independent-review-verdict.md"),
    ("TASK-260712-39vjzd", ".task-board/.resources/TASK-260712-39vjzd/TASK-260712-39vjzd_independent-review-verdict.md"),
    ("TASK-260712-2nppt6", ".task-board/.resources/TASK-260712-2nppt6/TASK-260712-2nppt6_independent-review-verdict.md"),
    ("TASK-260712-2q4jbu", ".task-board/.resources/TASK-260712-2q4jbu/TASK-260712-2q4jbu_review-verdict-v1.md"),
)

IMPLEMENTATION_MERGES = (
    ("TASK-260712-3w1cst", "2ab8a13"),
    ("TASK-260712-20j5tm", "32fee4a"),
    ("TASK-260712-1yz5ca", "3b08b74"),
    ("TASK-260712-1x9ruo", "5f1756d"),
    ("TASK-260712-25dzp4", "80cfef9"),
    ("TASK-260712-2i0w6x", "f9fd2ec"),
    ("TASK-260712-1rziyo", "375dc1b"),
    ("TASK-260712-2kcduo", "856e8a0"),
    ("TASK-260712-tcwn44", "2aed627"),
    ("TASK-260712-3980vy", "94d5de0"),
    ("TASK-260712-28zhpl", "c5eede9"),
    ("TASK-260712-1u57qz", "e47eb6b"),
    ("TASK-260712-39vjzd", "c11352b"),
    ("TASK-260712-2nppt6", "d265228"),
    ("TASK-260712-2q4jbu", "9d7ace6"),
)

EXPLICIT_ANCHORS = (
    "protocol/e2ee-media-audit-v1.json",
    "protocol/e2ee-media-audit-v1-vectors.json",
    "protocol/e2ee-key-state-v1-vectors.json",
    "protocol/e2ee-recovery-v1-vectors.json",
    "protocol/macos-e2ee-live-ptt-v1-vectors.json",
    "protocol/windows-e2ee-live-ptt-v1-vectors.json",
    "protocol/macos-protected-media-send-v1-vectors.json",
    "protocol/windows-protected-media-send-v1-vectors.json",
    "protocol/macos-protected-media-playback-v1-vectors.json",
    "protocol/windows-protected-media-playback-v1-vectors.json",
    "protocol/macos-encrypted-media-client-path-v1.json",
    "protocol/windows-encrypted-media-client-path-v1.json",
    "node-app/Sources/NodeAppUI/PulsarEncryptedMediaModel.swift",
    "node-app/Sources/NodeAppUI/PulsarEncryptedMediaView.swift",
    "node-app/Sources/NodeApp/MacEncryptedMediaClientPathComposition.swift",
    "node-app/Tests/NodeAppUITests/PulsarEncryptedMediaModelTests.swift",
    "pulsar-win/windows_encrypted_media_client.go",
    "pulsar-win/windows_encrypted_media_client_test.go",
    "acceptance/phase3/gate-matrix-v1.json",
    "coordinator/go.mod",
    "coordinator/go.sum",
    "pulsar-win/go.mod",
    "pulsar-win/go.sum",
    "node-app/Package.swift",
    "node-app/Package.resolved",
)

REVIEW_TOOLING = (
    "scripts/e2ee_review/generate_implementation_review_packet.py",
    "scripts/e2ee_review/validate_cross_platform_vectors.py",
    "scripts/acceptance/validate_e2ee_c4_c6_review_pack.py",
    "scripts/acceptance/test_e2ee_c4_c6_review_pack.py",
    "scripts/acceptance/test_e2ee_cross_platform_parity.py",
    "scripts/acceptance/run_automated.py",
)

REVIEW_ARTIFACTS = (
    "docs/analysis/p3-e2ee-c4-c6-engineering-review-pack.md",
    ".task-board/.resources/TASK-260712-1bcpda/p3-e2ee-media-sequence.puml",
)


def run_git(*args: str) -> str:
    return subprocess.check_output(("git", *args), cwd=ROOT, text=True).strip()


def digest_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def digest_path(relative: str) -> str:
    return digest_bytes((ROOT / relative).read_bytes())


def canonical_json(value: object) -> bytes:
    return (json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n").encode()


def artifact_paths(packet_path: str) -> Iterable[str]:
    packet = json.loads((ROOT / packet_path).read_text(encoding="utf-8"))
    artifacts = packet.get("artifacts", [])
    if isinstance(artifacts, list):
        for item in artifacts:
            if isinstance(item, dict) and isinstance(item.get("path"), str):
                yield item["path"]


def anchor(relative: str, category: str) -> dict:
    path = ROOT / relative
    if not path.is_file():
        raise FileNotFoundError(relative)
    return {"path": relative, "category": category, "sha256": digest_path(relative)}


def merge_entry(task: str, abbreviated: str) -> dict:
    commit = run_git("rev-parse", abbreviated)
    parents = run_git("show", "-s", "--format=%P", commit).split()
    if len(parents) != 2:
        raise ValueError(f"implementation interval is not a merge: {commit}")
    return {
        "task": task,
        "mergeCommit": commit,
        "tree": run_git("rev-parse", f"{commit}^{{tree}}"),
        "producerHead": parents[1],
        "subject": run_git("show", "-s", "--format=%s", commit),
    }


def build_packet() -> dict:
    if run_git("rev-parse", f"{SOURCE_COMMIT}^{{tree}}") != SOURCE_TREE:
        raise ValueError("source candidate tree drifted")

    components = []
    source_paths = set(EXPLICIT_ANCHORS)
    for task, packet, test in COMPONENTS:
        source_paths.update(artifact_paths(packet))
        source_paths.add(test)
        components.append({
            "task": task,
            "packet": packet,
            "packetSha256": digest_path(packet),
            "acceptanceTest": test,
            "acceptanceTestSha256": digest_path(test),
            "status": "accepted-repository-evidence",
        })

    reviews = [
        {"task": task, "path": path, "sha256": digest_path(path), "status": "accepted"}
        for task, path in TERMINAL_REVIEWS
    ]
    source_paths.update(path for _, path in TERMINAL_REVIEWS)

    anchors = []
    component_packets = {packet for _, packet, _ in COMPONENTS}
    component_tests = {test for _, _, test in COMPONENTS}
    review_paths = {path for _, path in TERMINAL_REVIEWS}
    for path in sorted(source_paths):
        if path in component_packets:
            category = "component-packet"
        elif path in component_tests:
            category = "acceptance-test"
        elif path in review_paths:
            category = "independent-review"
        elif path.endswith(("go.mod", "go.sum", "Package.swift", "Package.resolved")):
            category = "dependency-manifest"
        elif path.startswith("protocol/"):
            category = "protocol-or-vector"
        elif path.startswith("docs/"):
            category = "analysis"
        elif path.endswith(("_test.go", "Tests.swift")) or "/Tests/" in path:
            category = "implementation-test"
        else:
            category = "implementation-source"
        anchors.append(anchor(path, category))

    name_status = run_git("diff", "--name-status", DESIGN_REVIEW_MERGE, SOURCE_COMMIT)
    changed_paths = [line.split("\t")[-1] for line in name_status.splitlines() if line]
    product_prefixes = ("coordinator/", "pulsar-win/", "node-app/Sources/", "scripts/e2ee_container/")
    product_paths = sorted(path for path in changed_paths if path.startswith(product_prefixes))

    return {
        "schemaVersion": 1,
        "contract": "e2ee-c4-c6-engineering-review-pack.v1",
        "task": "TASK-260712-1bcpda",
        "publishedAt": "2026-07-20",
        "sourceCandidate": {
            "mergeCommit": SOURCE_COMMIT,
            "tree": SOURCE_TREE,
            "designReviewMerge": run_git("rev-parse", DESIGN_REVIEW_MERGE),
            "implementationMerges": [merge_entry(*item) for item in IMPLEMENTATION_MERGES],
            "diff": {
                "changedPathCount": len(changed_paths),
                "productPathCount": len(product_paths),
                "nameStatusSha256": digest_bytes((name_status + "\n").encode()),
                "productPaths": product_paths,
            },
        },
        "decision": {
            "result": "engineering-evidence-pack-complete-manual-and-external-blocked",
            "repositoryPreflightPassed": True,
            "criticalOrHighEngineeringFindingsOpen": False,
            "c4Accepted": False,
            "c5Accepted": False,
            "c6Accepted": False,
            "externalImplementationReviewSatisfied": False,
            "productionE2EEEnabled": False,
            "manualEvidenceClaimed": False,
            "nextClosureTask": "TASK-260712-1ulshp",
        },
        "components": components,
        "terminalIndependentReviews": reviews,
        "sourceAnchors": anchors,
        "reviewTooling": [
            {"path": path, "sha256": digest_path(path)} for path in REVIEW_TOOLING
        ],
        "reviewArtifacts": [
            {"path": path, "sha256": digest_path(path)} for path in REVIEW_ARTIFACTS
        ],
        "crossPlatformParity": {
            "status": "repository-fixture-parity-only",
            "validator": "scripts/e2ee_review/validate_cross_platform_vectors.py",
            "covered": ["protected-send", "protected-playback", "opaque-live-ptt", "client-path-gates"],
            "packagedAppInteroperability": "not-run",
        },
        "engineeringReruns": [
            {
                "id": "component-contract-and-mutation-tests",
                "status": "pass",
                "tests": 101,
                "scope": "nineteen-component-packets-plus-review-pack-and-parity",
            },
            {
                "id": "coordinator-e2ee-race",
                "status": "pass",
                "packages": 2,
                "storeDurationSeconds": 67.931,
            },
            {
                "id": "windows-e2ee-race",
                "status": "pass",
                "packages": 4,
            },
            {
                "id": "macos-e2ee-focused",
                "status": "pass",
                "tests": 51,
                "suites": 6,
            },
        ],
        "c4ThroughC6": {
            "C4": {
                "repositoryEvidence": ["membership-lineage-and-epoch-rotation-state-tests", "removed-device-routing-and-replay-tests", "current-epoch-transfer-and-explicit-history-grant-tests", "cross-platform-key-state-and-client-gate-vectors"],
                "claim": "engineering-preflight-only",
                "manualTask": "TASK-260712-yj668d",
                "manualStatus": "not-run",
            },
            "C5": {
                "repositoryEvidence": ["ciphertext-only-schema-constraints", "opaque-object-and-live-router-tests", "forbidden-plaintext-field-contract", "documented-metadata-disclosure-contract"],
                "claim": "engineering-preflight-only",
                "manualTask": "TASK-260712-yj668d",
                "storageAndTrafficCapture": "not-run",
            },
            "C6": {
                "repositoryEvidence": ["metadata-only-report-state-tests", "separate-explicit-decrypted-evidence-consent-tests", "bounded-evidence-retention-expiry-delete-audit-tests", "client-command-consent-separation-tests"],
                "claim": "engineering-preflight-only",
                "manualTask": "TASK-260712-yj668d",
                "packagedModerationWorkflow": "not-run",
            },
        },
        "dependencyInventory": {
            "scope": "source-manifests-not-final-build-sbom",
            "productionCryptoProvider": None,
            "productionSuites": [],
            "selectedContainer": None,
            "manifests": [
                {"path": path, "sha256": digest_path(path)}
                for path in EXPLICIT_ANCHORS
                if path.endswith(("go.mod", "go.sum", "Package.swift", "Package.resolved"))
            ],
            "finalBuildSBOM": "required-by-external-review-and-manual-build-freeze",
        },
        "featureFlagHandoff": {
            "e2ee_media": "absent-or-disabled",
            "live_ptt": "separate-coordinator-readable-capability",
            "activationAllowed": False,
            "requiredBeforeActivation": ["TASK-260712-1ulshp", "TASK-260712-yj668d", "TASK-260712-30xwu2", "TASK-260712-1actom"],
        },
        "reproducibleCommands": [
            "python3 scripts/e2ee_review/generate_implementation_review_packet.py --check",
            "python3 scripts/e2ee_review/validate_cross_platform_vectors.py",
            "python3 -m unittest scripts/acceptance/test_e2ee_c4_c6_review_pack.py scripts/acceptance/test_e2ee_cross_platform_parity.py",
            "python3 -m unittest " + " ".join(test for _, _, test in COMPONENTS),
            "cd coordinator && go test -race ./internal/e2eecontract ./internal/store -run 'E2EE|Opaque|Protected|HistoryGrant|Report'",
            "cd pulsar-win && go test -race ./... -run 'WindowsE2EE|WindowsProtected|WindowsEncrypted'",
            "DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter 'E2EE|ProtectedMedia|EncryptedMedia'",
            "python3 scripts/acceptance/run_automated.py --suite all --require-clean",
        ],
        "residualRisks": [
            {"id": "E2EE-PACK-R01", "owner": "TASK-260712-1ulshp", "status": "external-review-required", "text": "No qualified external implementation reviewer has closed the combined implementation."},
            {"id": "E2EE-PACK-R02", "owner": "TASK-260712-yj668d", "status": "manual-not-run", "text": "Packaged cross-platform interoperability, storage and traffic capture, OS secure storage, and moderation workflow are not run."},
            {"id": "E2EE-PACK-R03", "owner": "TASK-260712-1ulshp", "status": "production-selection-open", "text": "No production cryptographic provider, suite, protected container, or final build SBOM is selected."},
            {"id": "E2EE-PACK-R04", "owner": "TASK-260712-30xwu2", "status": "manual-not-run", "text": "Mixed-fleet rollback, device loss, transfer, recovery, and irreversible-loss drills are not run in packaged applications."},
            {"id": "E2EE-PACK-R05", "owner": "TASK-260712-1actom", "status": "beta-not-run", "text": "No E2EE-enabled beta soak or incident review exists."},
        ],
        "externalReviewHandoff": {
            "task": "TASK-260712-1ulshp",
            "independenceRequired": True,
            "independenceSatisfied": False,
            "reviewerIdentity": None,
            "approval": "required",
            "criticalHighClosureRequired": True,
            "protocolDeltaReopensDesignReview": True,
        },
        "manualHandoff": {
            "epic": "EPIC-260714-th54l3",
            "task": "TASK-260712-yj668d",
            "status": "not-run",
            "requiredPairings": ["windows-windows", "windows-macos", "macos-windows", "macos-macos"],
            "requiredPaths": ["protected-clip", "protected-track", "saved-cue", "protected-live-ptt"],
        },
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=pathlib.Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    packet = build_packet()
    encoded = json.dumps(packet, indent=2, ensure_ascii=False) + "\n"
    output = args.output if args.output.is_absolute() else ROOT / args.output
    if args.check:
        if not output.is_file() or output.read_text(encoding="utf-8") != encoded:
            raise SystemExit(f"generated packet drifted: {output}")
    else:
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(encoded, encoding="utf-8")
    print(json.dumps({"output": str(output.relative_to(ROOT)), "anchors": len(packet["sourceAnchors"]), "components": len(packet["components"]), "status": "pass"}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
