# P1 Telegram, history and presence parity regressions

- Date: 2026-07-15
- Task: `TASK-260712-3d0zgu`
- Contract: `p1-history-presence-telegram-v1`

This is the deterministic acceptance map for the Phase 1 Telegram, history
and presence story. It combines focused unit tests with store, HTTP and
coordinator integration tests. It does not claim a real Telegram client,
audible playback, packaged app, physical device or hardware result; those
remain in `EPIC-260714-th54l3`.

## Regression matrix

| Contract risk | Deterministic evidence |
| --- | --- |
| Processing completes out of order or a no-action voice gains a decision delay | `TestTelegramVoiceSubmitMediaAdapterPreservesFIFORepliesAndLegacyPlayback` proves trusted acceptance order, first-after-current behavior and zero synthetic `wait`; `TestVoiceProcessingCompletionCannotReorderSenders` proves the reorder buffer. |
| A callback is forged, expired, replayed, moved to another message, actor, orbit or role | `TestCallbackRegistryRejectsForgedExpiredCrossActorAndCrossOrbit`, `TestCallbackQueryReplayIsIdempotentAndConsumedTokenIsActorBound`, `TestSourcePrimaryAuthorizationRequiresCurrentSourceOrbitPrimary` and `TestTelegramParityCrossUserCallbackAndQueryReplayCannotMutate`. |
| Default replacement races playback or another callback | `TestTelegramInlineStartWinsWithoutReplacement`, `TestTelegramInlineConcurrentChoicesProduceOneReplacement`, `TestTelegramInlineChoiceAtomicallyReplacesDefaultAndDeduplicates` and transaction fault injection in `TestTelegramInlineFaultsRollBackDefaultAndReplacementTransactions`. |
| Interrupt fallback is invented or loses exact reason | `TestTelegramInlineInterruptRequiresExplicitFallback`, `TestOverlayControllerNeverInventsInterruptFallback` and the exact shared `confirmation.*`, `callback.*` and `downgrade.*` presentation assertions. |
| Voice/audio/document errors are trusted from Telegram metadata | `TestAudioAndDocumentProduceTypedHintOnlyEvents`, `TestAttachmentFailureVocabularyUsesCommonIngestProof`, `TestTelegramAudioAndDocumentHintsReachCommonIngestWithoutTrust`, `TestTelegramMediaGroupIsHonestlyRejectedBeforeIngest` and the media adapter failure tests. |
| Mixed node capability splits protocols or hides downgrade | `TestTelegramParityMixedCapabilitiesDowngradeAsOneTargetSet` and `TestOverlayControllerWholeDowngradeNeverSplitsTargetProtocols` prove one `after_current` target set with `mandatory_target_missing_overlay_capability`. |
| History pagination shifts, crosses tenant or grants actions | `TestHistoryMediaProjectionPaginationAndCursorBinding`, `TestHistoryTransmissionVisibilityRetentionAggregatesAndActions`, the strict history HTTP tests and history action service tests prove frozen pages, viewer binding, `404` collapse and mutation reauthorization. |
| Presence/history expose private runtime or raw identity state | `TestPresenceProjectionStalenessSanitizationAndDND`, `TestHistoryHTTPMediaPaginationValidationAndRedaction`, `TestPrivacySafeMetadataFallbacksNeverRenderRawIdentifiers` and subject-reference tests cover microphone, process, device, transport and raw-ID canaries. |
| DND/block precedence, expiry or receipt reason drifts | `TestTransmissionBlocksAndLayeredDNDPersistOwnershipAndRevision`, scheduler recheck tests, `TestHistoryBlockedReceiptReasonRequiresBlockOwnership` and exact EN/RU reason-copy tests cover local/orbit layers and actor/orbit ownership. |
| App and bot invent different pairwise names or callback text | `TestTelegramRoutingChoicesMatchSharedPairwisePresentation`, the SHA-256 presentation golden and `TestCallbackPolicyAndDowngradeCopyIsExactAcrossBothLocales`; Telegram callback answers now consume the same `callback.*` catalog as app clients. |

## Shared presentation correction

The regression review found that Telegram callback answer text still used a
private Russian-only switch. `CallbackResultLabel` now owns the finite
`applied`, `already_applied`, `requires_confirmation`, `too_late`, `expired`,
`forbidden`, `unsupported` and `failed` vocabulary in the same EN/RU catalog
as delivery, audience, downgrade, confirmation and receipt labels. The bot
selects Russian from that shared label; app clients can select either locale
from the same semantic key. Unknown values collapse to `callback.failed` and
are never reflected.

## Acceptance boundary

The automated suite proves state, authorization, ordering, redaction and
localized-copy invariants with deterministic clocks, SQLite transactions,
fake transports and protocol fixtures. Manual validation still must confirm
Telegram rendering, real callback delivery, audible ordering, DND behavior
and mixed packaged Windows/macOS installations on real services and hardware.
