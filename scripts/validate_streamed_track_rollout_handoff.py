#!/usr/bin/env python3
"""Fail closed when the P2 streamed-track rollout handoff drifts or overclaims."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "acceptance" / "streamed-track-rollout-handoff-v1.json"
HANDOFF = ROOT / "docs" / "analysis" / "p2-streamed-track-rollout-handoff.md"
PLAYER_HANDOFF = ROOT / "acceptance" / "codec-spike" / "player-handoff-v1.json"


class HandoffError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise HandoffError(message)


def load(path: Path = MANIFEST) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(value: dict[str, Any], root: Path = ROOT) -> None:
    require(value.get("schemaVersion") == 1, "unsupported rollout schema")
    require(value.get("contract") == "p2-streamed-track-rollout-handoff.v1", "wrong contract")
    require(value.get("task") == "TASK-260712-2ubzyf", "wrong owner task")
    require(value.get("scope") == "repository-engineering-handoff", "scope changed")
    require(value.get("executionStatus") == "documented-not-executed", "rollout was falsely claimed")
    require(value.get("sourceContracts") == [
        "p2-codec-player-adr-handoff.v1", "p2-stream-track-wire.v1",
        "p2-targets-inbox-rollout-handoff.v1", "pulsar.air-lifecycle-policy.v1",
        "p2-content-policy-consent.v1",
    ], "source contract order changed")

    activation = value.get("productionActivation", {})
    require(activation.get("featureFlag") == "streamed_tracks", "feature flag name changed")
    require(activation.get("featureFlagImplementation") ==
            "absent-until-reviewed-replacement-adr-and-schema-revision",
            "an unreviewed runtime switch was claimed")
    require(activation.get("currentValue") is False, "streamed tracks were enabled")
    require(activation.get("activationAllowedNow") is False, "current activation was allowed")
    require(activation.get("currentMaximumRolloutStage") == 4, "dark-deploy ceiling changed")
    require(activation.get("selectedProfile") is None, "production profile was selected")
    require(activation.get("productionDecoderRegistry") == [], "production decoder registered")
    require(activation.get("wireCapabilityAdvertised") is False, "wire capability advertised early")

    player = json.loads((root / PLAYER_HANDOFF.relative_to(ROOT)).read_text(encoding="utf-8"))
    require(player["decision"]["production"] == "no-go", "codec handoff no longer says no-go")
    require(player["decision"]["selectedCombination"] is None, "codec handoff selected a combination")
    matrix = value.get("variantMatrix", {})
    require(matrix.get("selectedProductionVariant") is None, "production variant selected")
    require(matrix.get("originalUploadIsDecoderInput") is False, "original upload became decoder input")
    require(matrix.get("candidatePairsForSchemaAndTestsOnly") == [
        {"codec": "mp3", "container": "mp3", "mime": "audio/mpeg"},
        {"codec": "aac-lc", "container": "m4a-faststart", "mime": "audio/mp4"},
        {"codec": "aac-lc", "container": "adts", "mime": "audio/aac"},
        {"codec": "opus", "container": "ogg", "mime": "audio/ogg"},
    ], "candidate-only variant matrix changed")

    bounds = value.get("bounds", {})
    expected_bounds = {
        "maximumInputBytes": 524288000, "maximumDurationMS": 7200000,
        "maximumChunkBytes": 1048576, "maximumNetworkReadBytes": 1048576,
        "installationCacheBytes": 536870912, "perVariantCacheBytes": 67108864,
        "pinnedCacheBytes": 134217728, "decoderRingBytes": 1048576,
        "seekPointSpacingMS": 10000, "minimumBufferedMS": 2000,
        "trackStartP95MS": 5000, "seekToAudioP95MS": 3000,
        "startSkewP95MS": 100, "processingAndEgressLeaseStaleMS": 1800000,
        "reconciliationIntervalMS": 300000,
    }
    require(bounds == expected_bounds, "frozen transport/cache/timing bounds changed")
    for field, player_field in (
        ("maximumChunkBytes", "maximumChunkBytes"),
        ("maximumNetworkReadBytes", "maximumNetworkReadBytes"),
        ("installationCacheBytes", "globalCeilingBytes"),
        ("perVariantCacheBytes", "perVariantCeilingBytes"),
        ("pinnedCacheBytes", "pinnedCeilingBytes"),
    ):
        require(bounds[field] == player["cache"][player_field], f"player handoff drift: {field}")

    quotas = value.get("quotaDefaults", {})
    require(quotas.get("actor") == {
        "uploadStarts24h": 100, "inputBytes24h": 5368709120,
        "canonicalBytes": 10737418240, "temporaryProcessingBytes": 2147483648,
        "concurrentJobs": 2, "retainedTrackBytes": 21474836480,
        "egressBytes24h": 107374182400,
    }, "actor quota defaults changed")
    require(quotas.get("orbit") == {
        "uploadStarts24h": 500, "inputBytes24h": 26843545600,
        "canonicalBytes": 53687091200, "temporaryProcessingBytes": 8589934592,
        "concurrentJobs": 8, "retainedTrackBytes": 107374182400,
        "egressBytes24h": 536870912000,
    }, "orbit quota defaults changed")
    require(quotas.get("onePlaybackReservationMaximumVariantMultiplier") == 2,
            "egress amplification reservation changed")
    require(quotas.get("betaCalibrationOwner") == "TASK-260712-2pnc5a",
            "beta calibration owner changed")

    rollout = value.get("rollout", [])
    require([stage.get("sequence") for stage in rollout] == list(range(1, 9)), "rollout order changed")
    require([stage.get("id") for stage in rollout] == [
        "freeze-backup-and-record", "coordinator-additive-schema-dark",
        "reconcile-and-observe-dark", "clients-and-adapters-dark",
        "replacement-adr-and-runtime-gate", "internal-orbit-enable",
        "bounded-orbit-expansion", "store-and-public-promotion",
    ], "rollout stages changed")
    require(all(len(stage.get("required", [])) == 4 for stage in rollout), "rollout guard missing")
    require(all(stage.get("activationAllowed") is False for stage in rollout),
            "current manifest claimed an executable activation stage")
    require(rollout[4].get("blockedByCurrentNoGo") is True, "replacement ADR gate removed")
    require(all(stage.get("manualEvidenceRequired") is True for stage in rollout[5:]),
            "manual promotion boundary weakened")

    mixed = value.get("mixedVersion", {})
    require(mixed.get("senderSelectedPolicies") == ["require_all", "supported_only_with_receipts"],
            "mixed-version policies changed")
    for key in ("clipFallbackAllowed", "spotifyFallbackAllowed", "broadcastFallbackAllowed",
                "lateAutoplayAfterUpgradeAllowed"):
        require(mixed.get(key) is False, f"unsafe fallback enabled: {key}")

    revocation = value.get("revocation", {})
    require(revocation.get("plainReport", "").startswith("reporter-local hide"),
            "plain report became global revocation")
    for key in ("senderDelete", "moderatorDelete", "variantRevoke", "ownerActorOrOrbitDisable"):
        rule = revocation.get(key, "").replace("-", " ")
        require("future open" in rule, f"future-open revocation missing: {key}")
    require("1 MiB" in revocation.get("alreadyOpenBoundedRead", ""), "bounded open-read rule changed")

    observability = value.get("observability", {})
    require(observability.get("publicHealthFields") == ["ready", "saturated", "last_reconciled_at"],
            "public health disclosure changed")
    require(len(observability.get("exactMetrics", [])) == 14, "operator metric inventory changed")
    require(len(set(observability.get("exactMetrics", []))) == 14, "duplicate operator metric")

    rollback = value.get("rollback", {})
    require(rollback.get("flagOffFirst") is True, "flag-off-first rollback guard removed")
    require(rollback.get("stopWriterBeforeArtifactChange") is True, "single-writer guard removed")
    require(rollback.get("downMigrationAllowed") is False, "destructive down migration enabled")
    require(rollback.get("manualSQLLifecycleMutationAllowed") is False,
            "manual lifecycle mutation enabled")
    require(rollback.get("testedPredecessorRevision") ==
            "06a06c099ed5b4f37f5e2dd3648772ffd041dfd9", "tested predecessor changed")
    require(set(rollback.get("preserveTables", [])) == {
        "stream_track_metadata", "stream_variants", "stream_variant_policy",
        "stream_processing_jobs", "stream_egress_sessions", "stream_egress_events",
        "stream_quota_policies", "stream_quota_policy_audit", "stream_playback_domains",
        "stream_queue_items", "transmission_targets", "transmission_inbox_items",
        "content_policy_acceptances", "moderation_reports",
    }, "rollback preservation set changed")

    downstream = value.get("downstream", [])
    downstream_ids = [item.get("task") for item in downstream]
    require(len(downstream_ids) == len(set(downstream_ids)) == 8, "downstream inventory changed")
    for task in downstream_ids:
        require(list((root / ".task-board").glob(f"**/{task}_*/progress.md")),
                f"unknown downstream task: {task}")

    evidence = value.get("evidence", [])
    require(len(evidence) >= 12, "evidence index incomplete")
    for anchor in evidence:
        relative = Path(anchor.get("path", ""))
        require(relative.parts and not relative.is_absolute() and ".." not in relative.parts,
                f"unsafe evidence path: {relative}")
        path = root / relative
        require(path.is_file(), f"missing evidence file: {relative}")
        symbol = anchor.get("symbol", "")
        require(symbol and symbol in path.read_text(encoding="utf-8"),
                f"missing evidence symbol {symbol} in {relative}")

    manual = value.get("manualBoundary", [])
    expected_manual = [
        "TASK-260712-1fpb9q", "TASK-260712-21kz3b", "TASK-260712-2bdi4a",
        "TASK-260712-3qybi2", "TASK-260712-3u5cdn", "TASK-260712-2pnc5a",
    ]
    require([item.get("task") for item in manual] == expected_manual, "manual task inventory changed")
    require(all(item.get("epic") == "EPIC-260714-th54l3" and
                item.get("status") == "manual-required" for item in manual),
            "manual evidence was claimed or rerouted")

    schema = (root / "coordinator/internal/store/stream_track_schema.go").read_text(encoding="utf-8")
    require("CHECK(production_selection_enabled = 0 AND selected_profile = '')" in schema,
            "database no-go guard changed without handoff revision")
    handoff = (root / HANDOFF.relative_to(ROOT)).read_text(encoding="utf-8")
    for heading in (
        "## Production decision and feature-flag assumptions",
        "## Frozen variant, transport and cache limits",
        "## Quota defaults and operator metrics", "## Required rollout order",
        "## Mixed-version and cross-story behavior",
        "## Delete, report, disable and cache revocation",
        "## Drain-before-rollback and roll-forward", "## Acceptance and manual boundary",
    ):
        require(heading in handoff, f"handoff section missing: {heading}")
    for task in downstream_ids + expected_manual + ["TASK-260712-2ubzyf"]:
        require(task in handoff, f"handoff does not name {task}")


def main() -> None:
    value = load()
    validate(value)
    print(json.dumps({
        "status": "pass", "contract": value["contract"],
        "executionStatus": value["executionStatus"],
        "currentMaximumRolloutStage": value["productionActivation"]["currentMaximumRolloutStage"],
        "rolloutStages": len(value["rollout"]),
        "manualTasks": len(value["manualBoundary"]),
    }, sort_keys=True))


if __name__ == "__main__":
    main()
