#!/usr/bin/env python3
"""Fail-closed validation for the E2EE C4-C6 engineering review pack."""

from __future__ import annotations

import hashlib
import json
import pathlib
import re
import subprocess


ROOT = pathlib.Path(__file__).resolve().parents[2]
PACK_PATH = ROOT / "acceptance/phase3/e2ee-c4-c6-engineering-review-pack-v1.json"
GATE_MATRIX_PATH = ROOT / "acceptance/phase3/gate-matrix-v1.json"
SHA256 = re.compile(r"[0-9a-f]{64}")


class E2EEReviewPackError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise E2EEReviewPackError(message)


def load(path: pathlib.Path = PACK_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def digest(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def git(*args: str) -> str:
    return subprocess.check_output(("git", *args), cwd=ROOT, text=True).strip()


def validate(pack: dict) -> None:
    require(pack.get("schemaVersion") == 1, "unsupported schema")
    require(pack.get("contract") == "e2ee-c4-c6-engineering-review-pack.v1", "wrong contract")
    require(pack.get("task") == "TASK-260712-1bcpda", "wrong task")
    require(pack.get("publishedAt") == "2026-07-20", "publication date drifted")

    candidate = pack.get("sourceCandidate", {})
    require(candidate.get("mergeCommit") == "9d7ace6dc7337cd2191f35b0d8373228cf759398",
            "source candidate drifted")
    require(candidate.get("tree") == "ef819c9bd3e18e7532630510622f28e486f20007",
            "source candidate tree drifted")
    require(git("rev-parse", f"{candidate['mergeCommit']}^{{tree}}") == candidate["tree"],
            "source candidate is not reproducible")
    merges = candidate.get("implementationMerges", [])
    expected_tasks = [
        "TASK-260712-3w1cst", "TASK-260712-20j5tm", "TASK-260712-1yz5ca",
        "TASK-260712-1x9ruo", "TASK-260712-25dzp4", "TASK-260712-2i0w6x",
        "TASK-260712-1rziyo", "TASK-260712-2kcduo", "TASK-260712-tcwn44",
        "TASK-260712-3980vy", "TASK-260712-28zhpl", "TASK-260712-1u57qz",
        "TASK-260712-39vjzd", "TASK-260712-2nppt6", "TASK-260712-2q4jbu",
    ]
    require([item.get("task") for item in merges] == expected_tasks,
            "implementation merge inventory drifted")
    require(len({item.get("mergeCommit") for item in merges}) == 15,
            "implementation merge duplicated")
    for item in merges:
        require(git("rev-parse", f"{item['mergeCommit']}^{{tree}}") == item.get("tree"),
                f"merge tree mismatch: {item.get('task')}")
        parents = git("show", "-s", "--format=%P", item["mergeCommit"]).split()
        require(len(parents) == 2 and parents[1] == item.get("producerHead"),
                f"producer merge lineage mismatch: {item.get('task')}")

    diff = candidate.get("diff", {})
    name_status = git("diff", "--name-status", candidate.get("designReviewMerge", ""), candidate["mergeCommit"])
    require(hashlib.sha256((name_status + "\n").encode()).hexdigest() == diff.get("nameStatusSha256"),
            "implementation diff digest mismatch")
    paths = [line.split("\t")[-1] for line in name_status.splitlines() if line]
    require(len(paths) == diff.get("changedPathCount"), "implementation path count drifted")
    product_prefixes = ("coordinator/", "pulsar-win/", "node-app/Sources/", "scripts/e2ee_container/")
    require(sorted(path for path in paths if path.startswith(product_prefixes)) == diff.get("productPaths"),
            "implementation product path inventory drifted")
    require(len(diff.get("productPaths", [])) == diff.get("productPathCount"),
            "implementation product path count drifted")

    components = pack.get("components", [])
    require(len(components) == 19, "component packet inventory incomplete")
    require(len({item.get("task") for item in components}) == 19, "component task duplicated")
    require(set(expected_tasks) <= {item.get("task") for item in components},
            "implemented component packet missing")
    for item in components:
        packet_path = ROOT / item.get("packet", "")
        test_path = ROOT / item.get("acceptanceTest", "")
        require(packet_path.is_file() and test_path.is_file(), f"component evidence missing: {item.get('task')}")
        require(digest(packet_path) == item.get("packetSha256"), f"component packet drifted: {item.get('task')}")
        require(digest(test_path) == item.get("acceptanceTestSha256"), f"component test drifted: {item.get('task')}")
        packet = json.loads(packet_path.read_text(encoding="utf-8"))
        require(packet.get("task") == item.get("task"), f"component packet owner mismatch: {item.get('task')}")
        decision = packet.get("decision", {})
        for key in (
            "productionEnabled", "e2eeFeatureEnabled", "capabilityAdvertised", "runtimeWired",
            "runtimeHTTPWired", "productionLibrarySelected", "productionSuiteSelected",
            "productionContainerSelected", "productClaimAllowed", "coordinatorDecryptsContent",
            "coordinatorDecryptsSpeech", "coordinatorContentKeyAccess", "plaintextFallbackAllowed",
        ):
            require(decision.get(key) is not True, f"component falsely enabled {key}: {item.get('task')}")
        production = packet.get("production", {})
        require(not any(value is True for value in production.values()),
                f"client production claim enabled: {item.get('task')}")

    reviews = pack.get("terminalIndependentReviews", [])
    require(len(reviews) == 16 and len({item.get("task") for item in reviews}) == 16,
            "terminal independent review inventory incomplete")
    require(reviews[0].get("task") == "TASK-260712-aniuyy", "design review not first")
    for item in reviews:
        path = ROOT / item.get("path", "")
        require(path.is_file(), f"review evidence missing: {item.get('task')}")
        require(digest(path) == item.get("sha256"), f"review evidence drifted: {item.get('task')}")
        require(item.get("status") == "accepted", f"review not accepted: {item.get('task')}")

    anchors = pack.get("sourceAnchors", [])
    require(len(anchors) >= 120, "source anchor inventory incomplete")
    require(len({item.get("path") for item in anchors}) == len(anchors), "source anchor duplicated")
    for item in anchors:
        path = ROOT / item.get("path", "")
        require(path.is_file(), f"source anchor missing: {item.get('path')}")
        require(SHA256.fullmatch(item.get("sha256", "")) is not None, "source digest malformed")
        require(digest(path) == item["sha256"], f"source digest mismatch: {item['path']}")

    tooling = pack.get("reviewTooling", [])
    require(len(tooling) == 6 and len({item.get("path") for item in tooling}) == 6,
            "review tooling inventory incomplete")
    for item in tooling:
        path = ROOT / item.get("path", "")
        require(path.is_file() and digest(path) == item.get("sha256"),
                f"review tooling drifted: {item.get('path')}")
    artifacts = pack.get("reviewArtifacts", [])
    require(len(artifacts) == 2, "review artifact inventory incomplete")
    for item in artifacts:
        path = ROOT / item.get("path", "")
        require(path.is_file() and digest(path) == item.get("sha256"),
                f"review artifact drifted: {item.get('path')}")

    decision = pack.get("decision", {})
    require(decision.get("result") == "engineering-evidence-pack-complete-manual-and-external-blocked",
            "decision drifted")
    require(decision.get("repositoryPreflightPassed") is True and
            decision.get("criticalOrHighEngineeringFindingsOpen") is False,
            "engineering preflight is not clean")
    for key in ("c4Accepted", "c5Accepted", "c6Accepted", "externalImplementationReviewSatisfied",
                "productionE2EEEnabled", "manualEvidenceClaimed"):
        require(decision.get(key) is False, f"fail-closed decision violated: {key}")
    require(decision.get("nextClosureTask") == "TASK-260712-1ulshp", "external closure task drifted")

    parity = pack.get("crossPlatformParity", {})
    require(parity.get("status") == "repository-fixture-parity-only" and
            parity.get("packagedAppInteroperability") == "not-run" and
            len(parity.get("covered", [])) == 4,
            "cross-platform evidence was overstated")
    reruns = pack.get("engineeringReruns", [])
    require([item.get("id") for item in reruns] == [
        "component-contract-and-mutation-tests", "coordinator-e2ee-race",
        "windows-e2ee-race", "macos-e2ee-focused",
    ], "engineering rerun inventory drifted")
    require(all(item.get("status") == "pass" for item in reruns),
            "engineering rerun did not pass")
    require(reruns[0].get("tests") == 101 and reruns[1].get("packages") == 2 and
            reruns[2].get("packages") == 4 and reruns[3].get("tests") == 51 and
            reruns[3].get("suites") == 6,
            "engineering rerun evidence count drifted")
    c456 = pack.get("c4ThroughC6", {})
    require(set(c456) == {"C4", "C5", "C6"}, "C4-C6 inventory incomplete")
    for gate in c456.values():
        require(gate.get("claim") == "engineering-preflight-only" and
                gate.get("manualTask") == "TASK-260712-yj668d",
                "C4-C6 acceptance was overstated")
    require(c456["C4"].get("manualStatus") == "not-run", "C4 manual evidence invented")
    require(c456["C5"].get("storageAndTrafficCapture") == "not-run", "C5 capture evidence invented")
    require(c456["C6"].get("packagedModerationWorkflow") == "not-run", "C6 workflow evidence invented")

    dependencies = pack.get("dependencyInventory", {})
    require(dependencies.get("scope") == "source-manifests-not-final-build-sbom",
            "dependency scope overstated")
    require(dependencies.get("productionCryptoProvider") is None and
            dependencies.get("productionSuites") == [] and
            dependencies.get("selectedContainer") is None,
            "production cryptography was selected")
    require(len(dependencies.get("manifests", [])) == 6, "dependency manifest inventory incomplete")
    for item in dependencies["manifests"]:
        path = ROOT / item.get("path", "")
        require(path.is_file() and digest(path) == item.get("sha256"),
                f"dependency manifest drifted: {item.get('path')}")
    require(dependencies.get("finalBuildSBOM") == "required-by-external-review-and-manual-build-freeze",
            "final build SBOM falsely claimed")

    flags = pack.get("featureFlagHandoff", {})
    require(flags.get("e2ee_media") == "absent-or-disabled" and
            flags.get("live_ptt") == "separate-coordinator-readable-capability" and
            flags.get("activationAllowed") is False,
            "feature flag handoff became unsafe")
    require(flags.get("requiredBeforeActivation") == [
        "TASK-260712-1ulshp", "TASK-260712-yj668d", "TASK-260712-30xwu2", "TASK-260712-1actom",
    ], "activation gate inventory drifted")

    residual = pack.get("residualRisks", [])
    require([item.get("id") for item in residual] == [
        "E2EE-PACK-R01", "E2EE-PACK-R02", "E2EE-PACK-R03", "E2EE-PACK-R04", "E2EE-PACK-R05",
    ], "residual risk inventory drifted")
    require(all(item.get("status") != "closed" for item in residual), "residual risk falsely closed")
    external = pack.get("externalReviewHandoff", {})
    require(external.get("task") == "TASK-260712-1ulshp" and
            external.get("independenceRequired") is True and
            external.get("independenceSatisfied") is False and
            external.get("reviewerIdentity") is None and
            external.get("approval") == "required" and
            external.get("criticalHighClosureRequired") is True,
            "external review was self-certified")
    manual = pack.get("manualHandoff", {})
    require(manual.get("epic") == "EPIC-260714-th54l3" and
            manual.get("task") == "TASK-260712-yj668d" and
            manual.get("status") == "not-run" and
            len(manual.get("requiredPairings", [])) == 4 and
            len(manual.get("requiredPaths", [])) == 4,
            "manual handoff was hidden or falsely completed")

    matrix = json.loads(GATE_MATRIX_PATH.read_text(encoding="utf-8"))
    require(matrix.get("gates", {}).get("C4", {}).get("status") == "blocked-by-deferred-e2ee-and-independent-review" and
            matrix.get("gates", {}).get("C5", {}).get("status") == "blocked-by-deferred-e2ee-and-independent-review" and
            matrix.get("gates", {}).get("C6", {}).get("status") == "blocked-by-deferred-e2ee-and-independent-review",
            "historical Phase 3 gate matrix no longer blocks C4-C6")
    require(len(pack.get("reproducibleCommands", [])) == 8 and
            all("<" not in command for command in pack.get("reproducibleCommands", [])),
            "reproduction command inventory incomplete")


def main() -> int:
    pack = load()
    validate(pack)
    print(json.dumps({
        "contract": pack["contract"],
        "sourceCandidate": pack["sourceCandidate"]["mergeCommit"],
        "components": len(pack["components"]),
        "anchors": len(pack["sourceAnchors"]),
        "manualEvidence": "not-run",
        "externalReview": "required",
        "status": "pass",
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
