# P3 saved-cue media lifecycle

Task: `TASK-260712-hb5xz2`
Policy identifier: `saved_cue_lifecycle_v1`

This increment implements the durable cue/media boundary required by
`automation-safety-v1`. It is persistence and repository behavior only: it
adds no HTTP route, scheduler, automation principal, desktop composition or
advertised node capability.

## Accepted source matrix

| Source | Save | Reason |
| --- | --- | --- |
| Same-orbit canonical `ready` app `audio_clip` | yes | Reuses generic ingest, storage, digest and owner authority |
| `pulsar.recording-cue.v1` at SHA-256 `479b1a9d605ac12454e3449e129991b7ce8599251506ca54a93be0b6144730fd` | yes | Reviewed 15,404-byte, 160 ms package asset |
| Foreign, corrupt, unready or terminal media | no | Privacy-safe not-found/invalid result |
| `voice_clip`, Telegram voice, `audio_track`, stream variant or live PTT | no | Outside the cue-only safety contract |
| Arbitrary builtin name/hash, URL, path or uploaded builtin copy | no | No parallel source, upload or fetch authority |

Create and replace require a current same-orbit primary control credential and
current content-policy consent. A node credential cannot mutate the library.
The media row remains canonical for ownership, readiness, report/delete,
moderation and physical bytes. A saved cue stores only its pinned reference and
the accepted revision/digest/size/duration snapshot.

## Pinning, quotas and ordering

An active media-backed `saved_cues` row is an explicit retention pin.
`ExpiredMediaItems` and the lifecycle backlog exclude active pins. Direct
expiry also fails closed while a pin exists. Deleting or replacing the final
pin immediately runs the canonical terminal transition if the media's ordinary
expiry has already passed; this produces the existing storage-cleanup and
delivery-cancellation work rather than a cue-specific byte path.

Per owner orbit, active usage is limited to 64 cues and 50 MiB. A source is at
most 10 MiB and 60 seconds. Usage is derived transactionally with
`COUNT`/`SUM` from active rows, so create, replace, delete, rollback and restart
cannot strand a mutable quota counter. Partial unique indexes deduplicate one
active cue per media ID or exact builtin version in an orbit.

`revision` orders every mutation. `source_generation` changes only when a
source becomes unsafe: replace, delete or source revocation. Rename increments
the row revision without invalidating already accepted work. Replace/delete
first writes a durable `saved_cue_revocations` entry for the old generation;
its canonical actions are `cancel` for not-started work, `fade_stop` for active
audio and `resume_once` for an interrupted main stream. Later automation
runtime tasks consume that outbox without confusing old work with the new
generation.

## Revocation and reconciliation

Canonical media delete/expiry revokes its active cues in the same SQLite writer
transaction. Actor moderation disable revokes cues created by that actor or
backed by that actor's media. Orbit moderation disable revokes every owned cue.
These mutations also create audit and generation-specific cancellation rows;
an active cue cannot outlive the authority that made it valid.

Startup runs `ReconcileSavedCues`. It fails closed any active row whose orbit or
actor is no longer active, whose media is no longer the same ready app clip, or
whose builtin metadata differs from the binary's exact pin. Base media expiry
alone is not corruption while an explicit pin remains. This makes mixed-version
rollback safe: an older binary may ignore the additive tables, and the current
binary reconciles canonical changes when it returns.

## Automated evidence boundary

Repository tests cover:

- pinned media surviving ordinary expiry and expiring through canonical cleanup
  after the final cue is deleted;
- source dedupe, quota accounting, rename/replace ordering and cancellation
  completion;
- rejection of foreign, unready, wrong-class, oversized and node-authorized
  sources;
- exact packaged builtin metadata with no `media_items` side effect;
- media/actor/orbit revocation, startup reconciliation and injected transaction
  rollback.

No real app, packaged desktop UI, audible output or hardware result is claimed
by this task. Those remain in the manual-test epic and later P3 acceptance
tasks.
