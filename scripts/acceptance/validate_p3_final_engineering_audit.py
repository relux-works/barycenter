#!/usr/bin/env python3
"""Fail-closed validation for the final non-E2EE Phase 3 engineering audit."""

from __future__ import annotations

import hashlib
import importlib.util
import json
import pathlib
import subprocess
import sys


ROOT = pathlib.Path(__file__).resolve().parents[2]
AUDIT_PATH = ROOT / "acceptance/phase3/final-engineering-audit-v1.json"
REPORT_PATH = ROOT / "docs/analysis/p3-final-engineering-audit.md"


class AuditError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AuditError(message)


def load(path: pathlib.Path = AUDIT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def git(*args: str, binary: bool = False):
    return subprocess.run(
        ["git", *args], cwd=ROOT, check=True, capture_output=True,
        text=not binary,
    ).stdout


def snapshot_bytes(commit: str, path: str) -> bytes:
    return git("show", f"{commit}:{path}", binary=True)


def snapshot_json(commit: str, path: str) -> dict:
    return json.loads(snapshot_bytes(commit, path))


def load_handoff_validator():
    path = ROOT / "scripts/acceptance/validate_phase3_engineering_handoff.py"
    spec = importlib.util.spec_from_file_location("phase3_handoff_for_final_audit", path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def validate(audit: dict) -> None:
    require(audit.get("schemaVersion") == 1, "unsupported schema")
    require(audit.get("contract") == "p3-final-engineering-audit.v1", "wrong contract")
    require(audit.get("task") == "TASK-260712-2b5685", "wrong task")
    require(audit.get("auditedAt") == "2026-07-17", "audit date drifted")

    baseline = audit.get("auditBaselineCommit", "")
    root_source = audit.get("rootReviewedSourceCommit", "")
    root_tree = audit.get("rootReviewedSourceTree", "")
    require(git("rev-parse", baseline).strip() == baseline, "audit baseline unavailable")
    require(git("merge-base", "--is-ancestor", baseline, "HEAD").strip() == "",
            "audit baseline is not an ancestor")
    require(git("rev-parse", root_source).strip() == root_source, "root source unavailable")
    require(git("rev-parse", f"{root_source}^{{tree}}").strip() == root_tree,
            "root source tree drifted")

    decision = audit.get("decision", {})
    require(decision.get("result") == "non-e2ee-phase3-engineering-complete-release-blocked",
            "audit decision drifted")
    require(decision.get("phase3NonE2EEEngineeringComplete") is True,
            "non-E2EE engineering not complete")
    require(decision.get("engineeringPacketAccepted") is True,
            "engineering packet not accepted")
    for key in (
        "originalEpicComplete", "productionAccepted", "storeSubmissionAllowed",
        "promotionAllowed", "betaAccepted", "manualEvidenceClaimed",
        "independentReviewClaimed", "e2eeAccepted", "releaseAuthorityGranted",
    ):
        require(decision.get(key) is False, f"false final acceptance recorded: {key}")
    require(decision.get("openReviewedScopeCriticalOrHighFindings") == [],
            "reviewed-scope Critical/High finding remains")

    anchors = audit.get("sourceAnchors", [])
    require(len(anchors) == 18, "source anchor inventory drifted")
    require(len({item.get("path") for item in anchors}) == len(anchors),
            "duplicate source anchor")
    for item in anchors:
        raw = snapshot_bytes(baseline, item.get("path", ""))
        require(hashlib.sha256(raw).hexdigest() == item.get("sha256"),
                f"snapshot digest mismatch: {item.get('path')}")

    delta = audit.get("postRootReviewDelta", {})
    start = delta.get("fromMergeExclusive", "")
    end = delta.get("toBaselineInclusive", "")
    require(end == baseline, "post-root delta does not end at audit baseline")
    name_status = git("diff", "--name-status", f"{start}..{end}", binary=True)
    numstat = git("diff", "--numstat", f"{start}..{end}", binary=True)
    require(hashlib.sha256(name_status).hexdigest() == delta.get("nameStatusSHA256"),
            "post-root name/status digest drifted")
    require(hashlib.sha256(numstat).hexdigest() == delta.get("numstatSHA256"),
            "post-root numstat digest drifted")
    changed = [line.split("\t", 1)[1] for line in name_status.decode().splitlines()]
    require(len(changed) == delta.get("changedPathsNoRenames") == 59,
            "post-root path count drifted")
    additions = deletions = 0
    for line in numstat.decode().splitlines():
        added, deleted, _ = line.split("\t", 2)
        additions += 0 if added == "-" else int(added)
        deletions += 0 if deleted == "-" else int(deleted)
    require(additions == delta.get("addedLines") == 15593, "post-root additions drifted")
    require(deletions == delta.get("deletedLines") == 69, "post-root deletions drifted")

    top_counts: dict[str, int] = {}
    for path in changed:
        top = path.split("/", 1)[0]
        top_counts[top] = top_counts.get(top, 0) + 1
    require(set(top_counts) == set(delta.get("allowedTopLevels", [])),
            "unreviewed top-level path entered audit range")
    require(top_counts == {
        ".planning": 1, ".task-board": 34, "acceptance": 5, "docs": 8, "scripts": 11,
    }, "post-root path classification drifted")

    product_prefixes = ("coordinator/", "pulsar-win/", "node-app/", "scripts/e2ee_container/")
    product_changed = [path for path in git(
        "diff", "--name-only", f"{root_source}..{baseline}"
    ).splitlines() if path.startswith(product_prefixes)]
    require(product_changed == delta.get("productRuntimeChangedPaths") == [],
            "post-root product delta requires review")
    dependencies = [
        path for path in changed
        if pathlib.PurePosixPath(path).name in {
            "go.mod", "go.sum", "Package.resolved", "Package.swift"
        } or path.endswith(".lock")
    ]
    require(dependencies == delta.get("dependencyLockChangedPaths") == [],
            "unreviewed dependency change entered candidate")
    workflow_deploy = [
        path for path in changed if path.startswith((".github/", "deploy/"))
    ]
    require(workflow_deploy == delta.get("workflowOrDeployChangedPaths") == [],
            "unreviewed workflow/deploy change entered candidate")

    first_parent = git("rev-list", "--first-parent", "--reverse", f"{start}..{end}").splitlines()
    recorded = [item.get("commit") for item in audit.get("firstParentMerges", [])]
    require(first_parent == recorded, "first-parent merge inventory drifted")
    require(len(recorded) == delta.get("firstParentMergeCount") == 11,
            "first-parent merge count drifted")
    require([item.get("pullRequest") for item in audit.get("firstParentMerges", [])] ==
            list(range(257, 268)), "pull-request sequence drifted")

    handoff = snapshot_json(baseline, "acceptance/phase3/engineering-handoff-v1.json")
    handoff_validator = load_handoff_validator()
    try:
        handoff_validator.validate(handoff)
    except Exception as error:
        raise AuditError(f"engineering handoff invalid: {error}") from error
    require(handoff.get("decision", {}).get("phase3PromotionAllowed") is False,
            "handoff promoted Phase 3")
    require(len(handoff.get("gateIndex", {})) == 19, "handoff gate count drifted")

    dispositions = audit.get("reviewDisposition", {})
    require(set(dispositions) == {
        "rootLineReview", "realtime", "automation", "privacyStore", "migrationRecovery"
    }, "review disposition inventory drifted")
    for name, record in dispositions.items():
        packet = snapshot_json(baseline, record.get("source", ""))
        packet_decision = packet.get("decision", {})
        if name == "rootLineReview":
            require(packet_decision.get("result") ==
                    "non-e2ee-engineering-baseline-accepted-production-blocked",
                    "root review decision drifted")
        else:
            require(packet_decision.get("repositoryPreflightPassed") is True,
                    f"technical pre-review missing: {name}")
            require(packet_decision.get("criticalOrHighTechnicalFindingsOpen") is False,
                    f"technical Critical/High remains: {name}")
            require(packet_decision.get("phase3PromotionAllowed") is False,
                    f"technical review promoted Phase 3: {name}")
            require(record.get("externalTask"), f"external task missing: {name}")
        require(record.get("criticalOrHighOpen") is False,
                f"audit disposition has Critical/High open: {name}")

    capabilities = audit.get("capabilityDecisions", {})
    require(set(capabilities) == {
        "live_ptt", "capture_quality", "soundboard_cues", "automation", "e2ee_media"
    }, "capability decision inventory drifted")
    for name, record in capabilities.items():
        require(record.get("engineeringDecision") in {
            "complete", "complete-with-native-effects-unverified",
            "deferred-out-of-scope-unavailable",
        }, f"engineering decision missing: {name}")
        require(record.get("promotionDecision") == "hold", f"capability promoted: {name}")
        require(record.get("productionActivationAllowed") is False,
                f"capability activation allowed: {name}")
        require(record.get("rollbackOwner", "").startswith("Ivan Oparin"),
                f"rollback owner missing: {name}")
        require(record.get("openTasks"), f"open task inventory missing: {name}")
    require("not end-to-end encrypted" in capabilities["live_ptt"].get("disclosure", ""),
            "Live PTT disclosure drifted")
    require("no selected library" in capabilities["e2ee_media"].get("disclosure", ""),
            "E2EE unavailable boundary drifted")

    evidence = audit.get("evidenceBoundary", {})
    require(evidence.get("gateCount") == 19, "gate boundary drifted")
    require(evidence.get("manualTaskCount") == 19, "manual task count drifted")
    require(evidence.get("manualTaskStatus") == "backlog-not-run", "manual status drifted")
    require(evidence.get("deferredE2EETaskCount") == 18, "E2EE task count drifted")
    require(evidence.get("phase3IndependentApprovalCount") == 4,
            "independent approval count drifted")
    for key in (
        "rawC1ThroughC7ArtifactsCommitted", "signedProductionArtifactsAccepted",
        "publicPolicyEvidenceAccepted", "partnerCenterEvidenceAccepted",
        "rolloutRecoveryEvidenceAccepted",
    ):
        require(evidence.get(key) is False, f"external evidence fabricated: {key}")
    require(evidence.get("betaDailyRecordsAccepted") == 0, "beta records fabricated")
    tree_paths = git("ls-tree", "-r", "--name-only", baseline).splitlines()
    require(not any(path.startswith(("acceptance/phase3/manual/", "acceptance/phase3/campaigns/"))
                    for path in tree_paths), "raw manual campaign committed")

    beta = audit.get("betaContinuity", {})
    require(beta.get("status") == "not-started", "beta falsely started")
    require(beta.get("acceptedConsecutiveDays") == 0, "beta day fabricated")
    require(beta.get("sameBuildOrFlagClaim") is False, "beta continuity fabricated")
    require(beta.get("task") == "TASK-260712-1actom", "beta owner drifted")
    require("resets the affected cohort to day one" in beta.get("resetRule", ""),
            "beta reset rule weakened")

    placeholders = audit.get("placeholderAndClaimAudit", {})
    require(placeholders.get("unownedPlaceholders") == [], "unowned placeholder remains")
    require(placeholders.get("productionHashFields") == "null-and-blocking",
            "production placeholder boundary drifted")
    require("none is represented as pass" in placeholders.get("conclusion", ""),
            "placeholder claim boundary weakened")

    store = snapshot_json(baseline, "docs/store/phase3/disclosure-draft-v1.json")
    require(store.get("eligibleForSubmission") is False, "Store draft became submittable")
    require(store.get("partnerCenterMutationClaimed") is False,
            "Partner Center mutation fabricated")

    report = REPORT_PATH.read_text(encoding="utf-8")
    for fragment in (
        "non-E2EE Phase 3 repository engineering complete",
        "zero post-review product/runtime",
        "The beta has not started",
        "The original epic therefore remains open",
        "Every absence is task-owned",
    ):
        require(fragment in report, f"final audit report missing: {fragment}")

    commands = audit.get("requiredReproduction", [])
    require(len(commands) == 5, "reproduction command inventory drifted")
    require(all(command.startswith("python3 ") for command in commands),
            "non-reproducible command recorded")
    next_state = audit.get("nextState", {})
    require(next_state.get("strictInlineEngineeringTask") is None,
            "invented next inline task")
    require(next_state.get("originalEpicMayCloseNow") is False,
            "original epic falsely closable")


def main() -> int:
    audit = load()
    validate(audit)
    print(json.dumps({
        "contract": audit["contract"],
        "decision": audit["decision"]["result"],
        "postRootPaths": audit["postRootReviewDelta"]["changedPathsNoRenames"],
        "productRuntimeDelta": 0,
        "promotionAllowed": False,
        "status": "pass",
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
