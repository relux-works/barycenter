# P2 targets/inbox parity regression evidence

This handoff closes the repository-automated scope of
`TASK-260712-1vklop`. It does not claim that B5, B6 or B7 has passed on real
packaged applications or physical installations. That acceptance remains in
`TASK-260712-3u5cdn` under manual epic `EPIC-260714-th54l3`.

The machine-readable authority is
[`p2-targets-inbox-parity-regressions.v1`](../../acceptance/targets-inbox-parity-regressions-v1.json).
Its validator rejects missing invariants, missing executable test anchors,
platform fixture drift, early streamed-track claims and any attempt to promote
the manual result from `manual-required`.

## B5 — explicit target isolation

The adversarial store regression creates one Air containing source A, same-Air
non-target B and explicit target C. It proves that a C-only transmission is
absent from B's transmission and inbox surfaces and that B cannot fetch the
media even with the exact transmission and media IDs. Reading C's inbox does
not mutate the transmission. Expiry retains only a terminal readable row and
removes replay authority.

After acceptance, a fourth member joins the Air. That member cannot discover
the old transmission or fetch its media. A second create selects B and C with
both orbit and installation selectors for C: the immutable snapshot contains
exactly B and C once each, excludes A when `include_origin=false`, and never
expands to the later member. Existing HTTP tests retain the external 404
nonexistence surface for guessed or known IDs.

Automated anchors include:

- `TestTargetsInboxB5ExplicitACLAndFrozenAudienceRegression`;
- `TestMediaDownloadHTTPEnforcesOwnerAndExactTargetACL`;
- `TestTransmissionTargetReferencesAreOpaqueCredentialBoundAndGenerationSafe`;
- `TestAuthorizedInboxPaginationIsolationDismissAndReplay`;
- `TestTransmissionInboxEligibleReceiptIsAtomicExactAndIdempotent`.

## B6 — mixed versions without a silent fallback

The common capability policy requires `media_clip_v1` for Phase 1 clips and
requires `audio_track_v1`, `queue_replace_v1` and `stream_variant_v1` for a
track. A mandatory online target missing any requirement causes one atomic
`422 unsupported_targets`; it creates neither a transmission nor request row.
Only opaque target references and sorted missing capability names are returned.

Windows, macOS and Telegram tests read the same regression fixture. The two
desktop models must render targeted track policy as `unsupported`; no queue or
replace fallback is synthesized. This is an honest pre-streaming boundary, not
evidence that the downstream streamed-track runtime or a real Phase 1/Phase 2
fleet has passed. Runtime and hands-on mixed-fleet proof remains manual in
`TASK-260712-3u5cdn` after the streamed-track story exists.

Automated anchors include:

- `TestCommonExplicitCapabilityPolicyCoversClipAndTrack`;
- `TestExplicitTargetsFailAtomicallyWithOpaqueSortedCapabilityDetails`;
- `TestTransmissionHTTPUsesOpaqueTargetsAndFailsClosedOnMixedVersions`;
- the shared Windows, macOS and Telegram parity-fixture tests.

## B7 — consent, report and authority-driven revocation

Current version/hash consent and the separate per-upload rights acknowledgement
remain mandatory for new user uploads. Consent is also rechecked for
transmission creation and explicit manual replay. A material policy hash change
invalidates the prior grant without rewriting already accepted media.

A report immediately protects only its reporter: it denies that reporter's
fetch, replay and future target while preserving unrelated targets and
moderation evidence. Report count alone cannot quarantine, delete or disable.
Reviewed delete or actor/orbit-disable authority revokes future fetch and
replay through the canonical media, scheduler and inbox boundaries. Telegram
history actions use those same services, and callbacks remain opaque,
actor/chat/message-bound, expiring and replay-safe.

Automated anchors include:

- `TestContentPolicyMaterialChangeAndRateLimitFailClosed` and the HTTP/Telegram
  consent gates;
- `TestModerationReportProtectsOnlyReporterFetchInboxReplayAndFutureDelivery`;
- `TestModerationDecisionIsCrashResumableIdempotentAndEnforced`;
- `TestTransmissionInboxIneligibleReceiptsAndCanonicalRevocation`;
- Telegram history callback binding and finalization tests.

## Pagination, migration and parity

Inbox and receipt cursors stay actor, credential-generation, view and limit
bound. Inserts after page one cannot expand the frozen chain, cross-actor
cursors expire, and only digests are stored. One eligible terminal target
receipt creates one inbox row in the same transaction; exact receipt retries
do not duplicate it. Additive upgrade/backfill and the exact previous-binary
rollback suite remain part of the pinned coordinator acceptance run.

The shared fixture freezes:

- five UI freshness states and four server-derived audiences;
- thirteen fail-closed commands;
- the canonical replay, dismiss, delete, report and block outcomes;
- manual replay, zero late autoplay and explicit unsupported-track behavior;
- the six opaque prefixes that must never be rendered.

## Reproduction

```sh
python3 scripts/validate_targets_inbox_parity_regressions.py
python3 -m unittest scripts/acceptance/test_targets_inbox_parity.py
(cd coordinator && go test ./... && go test -race ./...)
(cd pulsar-win && go test ./... && go test -race ./...)
(cd node-app && xcrun swift test)
python3 scripts/acceptance/run_automated.py --suite all --run-id <run-id>
```

The final command records repository automation only. Real packaged UI,
Narrator/VoiceOver, audible behavior, physical mixed fleets and real network
fetch denial must remain `manual-required` until `TASK-260712-3u5cdn` is
executed and accepted.
