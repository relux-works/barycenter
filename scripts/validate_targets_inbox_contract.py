#!/usr/bin/env python3
"""Fail-closed validator for the Phase 2 targets/inbox parity contract."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
CONTRACT_PATH = ROOT / "acceptance" / "targets-inbox-contract-v1.json"


class ContractError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ContractError(message)


def load(path: Path = CONTRACT_PATH) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract: dict[str, Any]) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p2-targets-inbox-parity.v1", "wrong contract id")
    require(set(contract.get("extends", [])) == {
        "p1-transmission-v1", "p1-history-presence-telegram-v1",
        "p1-moderation-control-plane.v1", "p2-air-lifecycle.v1",
    }, "upstream authority set changed")

    reuse = contract.get("reuse", {})
    for key in (
        "parallelACLAllowed", "parallelHistoryAllowed", "parallelModerationAllowed",
        "telegramOwnedDeliveryStateAllowed",
    ):
        require(reuse.get(key) is False, f"parallel authority enabled: {key}")
    require("immutable target snapshot" in reuse.get("audienceAuthority", ""), "target snapshot authority changed")

    strict = contract.get("strictJSON", {})
    require(strict.get("maximumBodyBytes") == 16384, "body limit changed")
    require(all(strict.get(key) == "reject" for key in (
        "unknownFields", "duplicateFields", "explicitNull", "trailingValues"
    )), "strict JSON weakened")

    create = contract.get("create", {})
    require(create.get("route") == "POST /v1/transmissions", "parallel create route introduced")
    require(create.get("maximumSelectors") == 64, "selector limit changed")
    require(create.get("deduplicateBeforeOriginFilter") is True, "target dedup ordering changed")
    require(create.get("identityFieldsInRequestAllowed") is False, "request identity injection enabled")
    require(create.get("contentPolicyGrantRequiredForUserMedia") is True, "content policy gate removed")

    snapshot = contract.get("targetSnapshot", {})
    require(snapshot.get("persistInCreateTransaction") is True, "snapshot is not atomic with create")
    require(snapshot.get("includesOfflineBindings") is True, "offline targets were removed")
    for key in (
        "laterMemberExpansionAllowed", "replacementBindingInheritsAccess",
        "currentAirMembershipAloneGrantsAccess", "blockedTargetGrantsMediaBytes",
    ):
        require(snapshot.get(key) is False, f"snapshot ACL widened: {key}")
    require(snapshot.get("nonTargetKnownIDResult") == "404 nonexistence surface", "non-target oracle introduced")

    delivery = contract.get("mediaDelivery", {})
    require(delivery.get("trackKind") == "audio_track", "track kind changed")
    require(delivery.get("trackDeliveries") == ["queue", "replace"], "track delivery enum changed")
    targeted = delivery.get("targetedTrack", {})
    require(targeted.get("nonTargetSameAirDiscoversMetadata") is False, "non-target discovers targeted track")
    require(targeted.get("nonTargetMainProgramChanges") is False, "non-target playback can change")
    require(targeted.get("broadcastFallbackAllowed") is False, "broadcast fallback enabled")

    mixed = contract.get("mixedVersion", {})
    require(mixed.get("mandatoryOnlineTargetMissingRequiredCapability") == "422 unsupported_targets",
            "mixed-version fail-closed result changed")
    require(mixed.get("partialCreateAllowed") is False, "partial mixed-version create enabled")
    require(mixed.get("phase1TrackTarget").startswith("unsupported"), "Phase 1 track target is not explicit")
    require("never late autoplay" in mixed.get("offlineTargetUnknownCapability", ""), "offline target can autoplay late")

    eligibility = contract.get("inboxEligibility", {})
    require(eligibility.get("defaultTTLSeconds") == 30 * 24 * 60 * 60, "inbox TTL changed")
    require(len(eligibility.get("eligible", [])) == 9, "eligible missed-reason set changed")
    require("blocked" in eligibility.get("ineligibleTargetStates", []), "blocked target became inbox eligible")
    for key in ("autoPlayOnReconnect", "autoQueueOnReconnect", "notificationCreatesPlaybackAuthority",
                "newAirMemberDiscoversPriorItems"):
        require(eligibility.get(key) is False, f"inbox authority widened: {key}")
    require(eligibility.get("formerTargetAfterBindingReplacement") == "404 and no replay authority",
            "former binding retained replay authority")

    entry = contract.get("inboxEntry", {})
    required_fields = entry.get("requiredFields", [])
    require(len(required_fields) == len(set(required_fields)) and len(required_fields) >= 13,
            "inbox field set is incomplete or duplicated")
    require(entry.get("receiptAggregate").startswith("reuse p1 history"), "parallel receipt aggregate introduced")
    require(entry.get("actionHintsAreAuthority") is False, "action hints became mutation authority")
    require("never serialize m_, tr_" in entry.get("wireIdentifierBoundary", ""),
            "inbox wire identifier boundary weakened")

    pagination = contract.get("pagination", {})
    require((pagination.get("minimumLimit"), pagination.get("defaultLimit"), pagination.get("maximumLimit")) ==
            (1, 20, 100), "pagination limits changed")
    require(pagination.get("cursorTTLSeconds") == 86400, "cursor TTL changed")
    require(pagination.get("maximumLiveCursorsPerActor") == 128, "cursor bound changed")
    require(pagination.get("clientTokenContainsTenantOrMediaID") is False, "cursor leaks identifiers")
    require(pagination.get("membershipChangesExpandFrozenPage") is False, "cursor scope expands with membership")
    require(pagination.get("invalidExpiredOrCrossActorCursor") == "410 cursor_expired",
            "cursor failure surface changed")

    receipts = contract.get("receiptPagination", {})
    require(receipts.get("route") == "GET /v1/history/{history_item_id}/receipts",
            "receipt pagination route changed")
    require((receipts.get("minimumLimit"), receipts.get("defaultLimit"), receipts.get("maximumLimit")) ==
            (1, 20, 100), "receipt pagination limits changed")
    require(receipts.get("cursorTTLSeconds") == 86400 and
            receipts.get("maximumLiveCursorsPerActor") == 128,
            "receipt cursor bounds changed")
    require(receipts.get("rawTargetIdentityReturned") is False,
            "receipt projection exposes target identity")
    require(receipts.get("readTriggersPlaybackOrQueue") is False,
            "receipt read can trigger playback")

    replay = contract.get("replay", {})
    require(replay.get("manualExplicitActionRequired") is True, "replay is no longer manual")
    require(replay.get("reauthorizeActorBindingMediaAndPolicy") is True, "replay reauthorization removed")
    require(replay.get("createsNewTransmission") is True and replay.get("mutatesOriginalReceipt") is False,
            "replay lineage semantics changed")
    require(replay.get("maximumDepth") == 8, "replay depth bound changed")
    require(replay.get("lateAutoPlayAllowed") is False, "late autoplay enabled")
    require(replay.get("defaultInboxReplayAudience") == "same exact current recipient installation",
            "inbox replay target widened")
    require(replay.get("responseIdentifiers") == [
        "ir_ replay request capability", "hi_ history item capability"
    ], "inbox replay leaks raw resource identifiers")

    deletion = contract.get("statusAndDelete", {})
    require(deletion.get("localDismissDeletesMedia") is False, "local dismiss deletes media")
    require(deletion.get("localDismissCancelsOtherTargets") is False, "local dismiss cancels peers")
    require(deletion.get("senderDeleteAuthority").startswith("existing media delete"), "parallel delete authority introduced")
    require(deletion.get("deletedDisabledOrExpiredFetch") == "404 nonexistence surface", "terminal fetch oracle introduced")

    policy = contract.get("contentPolicy", {})
    require(policy.get("contract") == "p2-content-policy-consent.v1", "content policy contract changed")
    require(policy.get("currentVersion") == "1.0", "content policy version changed")
    require(policy.get("currentPolicyHash") ==
            "a4d59ec7e9bfd8aeb2ec5d84356517580bde8df4540e6a2162f9206cd7ecd30e",
            "content policy binary hash changed")
    require(set(policy.get("localeHashes", {})) == {"en", "ru"}, "content policy locale set changed")
    require(policy.get("controllingLanguage") == "en", "controlling language changed")
    require(policy.get("termsURL") == "https://barycenter.live/legal/terms", "terms URL changed")
    require(policy.get("contentGuidelinesURL") ==
            "https://barycenter.live/legal/content-guidelines", "guidelines URL changed")
    require(policy.get("currentVersionOwnedByServer") is True and policy.get("clientTimestampTrusted") is False,
            "content policy authority moved to client")
    require(set(policy.get("requiredFor", [])) == {
        "user media upload", "create transmission", "manual replay"
    }, "content policy gate coverage changed")
    for key in (
        "termsAcceptanceSeparateFromPerUploadRightsReminder",
        "telegramAudioDocumentCheckedBeforeDownload",
        "legacyTelegramVoicePolicyUnchanged",
    ):
        require(policy.get(key) is True, f"content policy invariant disabled: {key}")
    require(policy.get("contentFilenameOrRawTransportIDStored") is False,
            "content policy grant stores content metadata")
    require(policy.get("acceptanceProvesOwnershipOrLegalValidity") is False,
            "content policy makes an ownership claim")
    require(set(policy.get("surfaces", [])) == {
        "coordinator HTTP", "Windows", "macOS", "Telegram"
    }, "content policy surface parity changed")

    moderation = contract.get("moderation", {})
    require(moderation.get("reportRoute").startswith("reuse POST /v1/history"), "parallel report route introduced")
    require(moderation.get("reportImmediateGlobalEffect") == "none", "report gained global side effects")
    require(set(moderation.get("reportDoesNot", [])) == {
        "delete media", "cancel unrelated targets", "disable sender",
        "disable orbit", "grant moderator authority",
    }, "anti-denial-of-service report boundary changed")
    quarantine = moderation.get("quarantine", {})
    require(quarantine.get("authority").startswith("moderation operator decision"), "quarantine lacks operator authority")
    require(quarantine.get("reportCountThresholdAllowed") is False, "report-count quarantine enabled")
    require(quarantine.get("reversible") is True and quarantine.get("audited") is True,
            "quarantine is not reversible and audited")

    parity = contract.get("parity", {})
    require(set(parity.get("surfaces", [])) == {
        "coordinator HTTP", "Windows", "macOS", "Telegram"
    }, "parity surface set changed")
    require(parity.get("sameFieldsEnumsAuthorizationAndErrors") is True, "surface contract divergence enabled")
    require(parity.get("telegramCallbackCarriesBearerOrNumericIdentity") is False, "Telegram callback leaks authority")
    require(parity.get("telegramLocalQueueAllowed") is False, "Telegram parallel queue enabled")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("path", nargs="?", type=Path, default=CONTRACT_PATH)
    args = parser.parse_args()
    contract = load(args.path)
    validate(contract)
    print(json.dumps({
        "status": "pass",
        "contract": contract["contract"],
        "eligibleInboxReasons": len(contract["inboxEligibility"]["eligible"]),
        "surfaces": contract["parity"]["surfaces"],
        "lateAutoPlay": contract["replay"]["lateAutoPlayAllowed"],
        "parallelACL": contract["reuse"]["parallelACLAllowed"],
    }, sort_keys=True))


if __name__ == "__main__":
    main()
