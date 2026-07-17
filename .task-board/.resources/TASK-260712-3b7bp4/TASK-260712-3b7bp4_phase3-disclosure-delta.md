# Phase 3 disclosure delta handoff

- Task: `TASK-260712-3b7bp4`
- Date: 17 July 2026
- Owner and submit authority: Ivan Oparin
- State: engineering draft; publication and Store submission are on hold

This document is the claim boundary for the Phase 3 engineering handoff. It is
not a published policy, a Partner Center mutation, an IARC submission or release
approval. The currently checked-in Phase 1 privacy and Store text remains the
only publication source until the external gates below close.

## Product claim boundary

| Capability | Allowed engineering wording | Prohibited wording before final gates |
| --- | --- | --- |
| `live_ptt` | Near-live push-to-talk code and deterministic preflights exist. The coordinator flag `DUET_LIVE_PTT` defaults off, clients do not advertise the capability for production, and manual C1-C3 evidence is not run. When eventually enabled, Live PTT is coordinator-routed readable audio and is **not end-to-end encrypted**. | available, released, production-ready, private from the coordinator, encrypted end to end, C1/C2/C3 passed |
| `e2ee_media` | Threat, container and protocol preparation contracts exist. No production crypto library, suite, protected-media implementation, key lifecycle or advertised capability exists. The feature is deferred and disabled. | encrypted, protected from the coordinator, secure recovery, forward secrecy, C4/C5/C6 passed |
| `soundboard_cues` | Saved-cue and client control code has deterministic repository evidence. Activation, audible behavior, accessibility and signed-app evidence remain pending. | released, audible matrix passed, Store-ready |
| `automation` | Schedule, principal, revoke, history and emergency-disable code has deterministic repository evidence. C7, signed-app, independent review, rollout and beta remain pending. | safe for production, C7 passed, independently approved |

The product UI must derive capability availability from the reviewed build and
runtime posture. It must not infer `e2ee_media` from HTTPS/WSS, local protected
credential storage, the protected-container spike or an `e2ee` label in an
audit-only contract. A build with Live PTT and no E2EE must say that the
coordinator can read and route Live PTT audio.

## Privacy delta

The current EN/RU policies already say that Phase 1 coordinator-readable audio
is not end-to-end encrypted. Before any Live PTT cohort is enabled, the owner
must publish a reviewed EN/RU revision that additionally states:

- microphone capture starts only from the local hold/control surface and stops
  on release, watchdog, lock, sleep, disconnect, rollback or quit;
- Live PTT frames are transiently relayed by the coordinator, are readable by
  it, and are not retained as audio by the Live PTT runtime;
- bounded lifecycle, health and drop aggregates may be retained, but raw audio,
  transcripts, keys, raw actor/session identifiers and arbitrary errors are not
  part of the observability export;
- Live PTT is not end-to-end encrypted unless a later reviewed build separately
  advertises `e2ee_media_v1`; the present build does not;
- the recipient may hear, record or independently copy delivered audio;
- report evidence, deletion, backups, Telegram and moderation retain the limits
  already described by the Phase 1 policy.

The controlling language remains English. Ivan Oparin reviews both EN and RU.
Live URL reachability and semantic equivalence belong to
`TASK-260717-35bll1`; mailbox routing belongs to `TASK-260714-200ib8`.

## Store and IARC delta

The machine draft is
[`docs/store/phase3/disclosure-draft-v1.json`](../store/phase3/disclosure-draft-v1.json).
It contains conditional EN/RU listing copy and an IARC delta assessment. None
of that copy may be submitted while its `eligibleForSubmission` value is false.

The IARC baseline remains conservative private user-audio UGC with reactive
report/block/delete controls. Live PTT, saved cues and schedules do not justify
reducing any Phase 1 answer. A later implemented protected-media path changes
transport confidentiality, not the fact that users can communicate arbitrary
audio; it also does not remove moderation or report disclosures.

Partner Center screenshots, WACK, certification notes, package hash and exact
listing record remain external/manual evidence. Ivan Oparin is the listing
owner, final submit authority and emergency withdrawal authority.

## Hold and reopen rules

Publication and submission remain blocked by the root engineering audit,
applicable manual C1-C7 gates, independent reviews, signed rollout/recovery
drills and the seven-day beta. E2EE claims additionally require every task in
`EPIC-260716-3qsztl` and its independent design/implementation reviews.

Any change to capture authority, audio retention, coordinator visibility,
metadata, report evidence, deletion, backup recovery, Telegram behavior,
feature flags, Store copy or policy text reopens this disclosure review and all
affected gates. Missing evidence is `pending` or `not-run`, never `pass`.
