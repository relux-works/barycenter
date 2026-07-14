# Phase 1 media ingest rollout handoff

- Date: 2026-07-14
- Task: `TASK-260712-jolzhh`
- Story: `STORY-260712-ld674h`

This is the concise developer and operator entry point for phase-one generic
media ingest. It describes the contract that is implemented now, the seams
owned by later stories, and a reversible rollout. Detailed design and test
evidence remain in the linked contract documents at the end.

This is engineering evidence only. Real application, physical device, speaker,
network, Store-package and human-listening checks belong to the separate manual
epic `EPIC-260714-th54l3`; none is implied by this handoff.

## Shipped boundary

Application uploads and Telegram voice messages now converge on one
transport-neutral `SubmitMedia` processing service and one authoritative
`media_items` lifecycle. The current release provides:

- control-authenticated, idempotent upload-session creation;
- session-scoped, append-only resumable byte upload;
- server-side signature, ffprobe and bounded ffmpeg validation;
- canonical PCM `s16le`, 44.1-kHz, stereo WAV publication;
- owner-control and immutable-target download authorization;
- immediate logical delete/revocation plus durable physical cleanup;
- seven-day ready-clip retention and 90-day content-free ingest audit;
- Telegram acceptance-order and legacy `play_voice` compatibility; and
- exact-predecessor migration, rollback and roll-forward regression gates.

Two important boundaries remain deliberately closed:

1. production node access to `GET /v1/media/{id}` is fail-closed until
   `TASK-260712-1aprcb` connects persisted immutable transmission targets to
   `MediaTargetSnapshotReader`; owning control access works now; and
2. app-facing media routes currently use the existing
   `self_service_onboarding` gate (`DUET_SELF_SERVICE_ONBOARDING=1`). The
   conceptual separate `app_media_upload` flag from the product specification
   is not implemented yet.

## Upload and retry contract

### Create or replay

`POST /v1/media/uploads` requires one live control credential, one
`Idempotency-Key` of 16-128 safe ASCII characters, and strict JSON:

```json
{
  "kind": "voice_clip",
  "title": "optional display title",
  "size_bytes": 12345
}
```

`kind` is `voice_clip` or `audio_clip`; declared size is positive and no more
than 50 MiB. A new request returns `201`; an exact replay returns `200` with
the same `upload_id`, `media_id` and reproducible scoped `upload_token`.
Reusing the key for different input returns `409 upload_state_conflict`.
Neither long-lived nor scoped credentials appear in a URL or SQLite plaintext.

The creation transaction enforces these defaults:

| Limit | Default |
| --- | --- |
| upload starts | 10 per rolling minute, actor and orbit bounded |
| concurrently processing media | 3, actor and orbit bounded |
| new declared bytes | 1 GiB per orbit per rolling 24 hours |
| one clip | 50 MiB and 180 seconds |
| upload-session lifetime | 1 hour |

An idempotent replay is resolved before quota accounting and consumes no
second reservation.

### Resume and finalize

`PUT /v1/media/uploads/{upload_id}` requires the scoped upload bearer, exactly
one canonical decimal `Upload-Offset`, `Content-Type: application/octet-stream`
and a known `Content-Length`. Transfer encoding is rejected.

Uploads are append-only. Each successful response returns the authoritative
`Upload-Offset`, `Upload-Length` and `Upload-Expires` headers plus the current
JSON state. Client behavior is:

1. persist the session ID, scoped token and declared length locally;
2. send the next chunk at the last confirmed offset;
3. on `409 upload_offset_conflict`, use the response's authoritative offset
   rather than creating another media item;
4. on transport loss, repeat the same request/session; and
5. after the declared length is reached, repeat the final request if necessary
   until the state is `completed` or a terminal error is returned.

A zero-byte PUT at the final offset recovers a crash between offset persistence
and transition to `finalizing`. A repeated final request resumes processing or
returns the existing completed result without appending or publishing twice.
Wrong path/token pairs, expired scoped credentials and unknown sessions all
collapse to `401 upload_credential_invalid`; they are not existence oracles.

## Durable state and failure behavior

```text
upload: open -> finalizing -> completed
          |          |
          +----------+-> failed | expired

media:  processing -> ready -> deleted | expired
           |
           +---------> failed -> deleted | expired
```

The media item does not become `ready` until probe, normalization, metadata,
canonical-file fsync and the publication transaction all succeed. Failed,
deleted and expired rows expose no storage key. A stale worker cannot revive a
terminal row because every mutation uses a persisted revision CAS.

Supported phase-one inputs are WAV/PCM, MP3, M4A/AAC, M4A/ALAC, ADTS AAC,
OGG/Opus, OGG/Vorbis and FLAC. Extension and caller MIME are not authoritative.
Unsupported, corrupt, truncated, polyglot, excessive-duration, invalid-layout,
probe/worker timeout, invalid-output and output-cap failures never become
downloadable. HTTP reports the stable `422 media_processing_failed` envelope;
worker diagnostics, tokens, titles, paths and source names stay out of it.

Temporary upload bytes live under `${media_dir}/.uploads` (`0700`, files
`0600`). Private processing artifacts are removed on startup. Canonical bytes
use opaque `media/v1/<64 lowercase hex>` keys below
`${media_dir}/canonical`; caller names never become paths. Publication and
cleanup use durable outbox receipts, so restart can reconcile an interrupted
filesystem/SQLite boundary.

## Retention, delete and compatibility

| Data | Implemented behavior |
| --- | --- |
| open/finalizing upload | expires after one hour; source cleanup is retried |
| failed app upload/session bytes | 15-minute maintenance removes them within the 24-hour maximum |
| failed Telegram private source | retained for the legacy debug contract, then removed on delete or seven-day expiry |
| ready clip | expires seven days after creation |
| deleted/expired canonical bytes | immediately non-readable, then asynchronously unlinked |
| transmission/history metadata | separate 30-day policy owned downstream |
| reported evidence | separate restricted policy, up to 30 days |
| content-free ingest audit | pruned after 90 days |
| SQLite recovery points | current published backup window is seven days |

Application uploads always receive the seven-day clip window. The default
`media.retention_days` is also seven days for Telegram compatibility. Existing
installations with an explicit older value keep that value, so operators must
set `media.retention_days: 7` before enabling the phase-one rollout.

Owner-control `DELETE /v1/media/{id}` is idempotent. It revokes every new read
inside the successful transaction, cancels pending publication, closes an open
upload, writes a content-free audit transition, and enqueues physical cleanup
plus delivery cancellation. Unknown, foreign, already expired and guessed IDs
all return the same `404 media_not_found` to a valid credential.

Telegram acceptance atomically creates the common item and same-ID legacy row.
The common item owns processing and terminal state; the compatibility row keeps
the established acceptance-time FIFO, bot replies, `KindVoice`, `play_voice`
and `/media/{id}.wav` path during mixed rollout. The current serial scheduler
can disarm queued legacy voice and stop an active voice. Immutable target rows,
prepare/play receipts and final click-free `fade_stop` semantics remain owned
by the transmission story.

## Cross-story handoff

| Consumer/owner | Contract handed over here |
| --- | --- |
| identity/onboarding (`STORY-260712-2ve1c8`) | Upload creation re-resolves the live actor, orbit, membership, role and control capability inside the writer transaction. Secure clients store the control credential; scoped upload tokens are separate and short-lived. |
| transmission/scheduler (`STORY-260712-25lysg`) | `TASK-260712-1aprcb` must persist immutable accepted target snapshots and implement the existing fail-closed reader. It also consumes idempotent `media_lifecycle_v1` cancellations and owns final wire/ramp behavior. |
| main UI/capture (`STORY-260712-2e36uz`) | Clients use create/replay, authoritative offsets and terminal state exactly as above; local drafts are retained until confirmed completion or explicit cancel/delete. Client MIME and extension do not bypass server validation. |
| Telegram/history/presence (`STORY-260712-34kbkn`) | Telegram already uses common processing while legacy ordering/playback stays compatible. Later inline routing and history query work must reference the common media ID without treating presence or current Air membership as a download grant. |
| policy/moderation and Store compliance (`STORY-260712-1tgryz`, `STORY-260712-1i0doc`) | Delete, retention, non-disclosure, audit and report hooks are backend inputs. Public privacy/support copy must disclose backup expiry and must not claim physical erasure earlier than the actual provider window. |
| later saved cues/automation (`STORY-260712-326wd5`) | Reuse common media lifecycle, opaque storage and ACL; do not introduce an automation-only byte store or retention bypass. |

No consumer may authorize generic media from a copied ID, current approach,
presence, inbox visibility, current Air membership, legacy WAV link or content
hash. Only owner control or an immutable accepted target snapshot grants a
generic read.

## Rollout checklist

1. Back up SQLite with the documented procedure and record the release
   artifact/checksum. Media bytes are not covered by the current DB-only
   Litestream backup.
2. Install both `ffmpeg` and `ffprobe`; confirm the coordinator service user
   can execute them.
3. Set `media.retention_days: 7`. Confirm `media_dir` is on the intended
   filesystem and writable by the coordinator service user.
4. Check free space with `df -h <media_dir>`. Free-space status is not yet part
   of `/healthz`; do not infer it from a green response.
5. Deploy the additive schema with `DUET_SELF_SERVICE_ONBOARDING=0`. Existing
   Telegram and legacy-node behavior remains available.
6. Inspect `/healthz`. Require top-level `status: ok` and
   `media_lifecycle.healthy: true`. `media_processing` is currently emitted
   only as `{"status":"unavailable"}` on initialization failure. When
   Telegram or self-service upload is enabled, absence of that key is the
   initialized healthy case; when both are disabled, no processor is required.
7. Confirm lifecycle failures do not grow and the pending storage, temporary,
   legacy and cancellation backlogs converge. The current coordinator installs
   its legacy-session cancellation sink; any future mode without a sink must
   keep requests pending rather than falsely marking them delivered.
8. Deploy the transmission target-store/scheduler and capable node releases
   before enabling app delivery. Keep generic node download fail-closed until
   that persisted reader is wired.
9. Enable `DUET_SELF_SERVICE_ONBOARDING=1` to expose onboarding and app media
   routes. Start with a bounded cohort and monitor processing failures, disk
   space, upload quota responses and lifecycle backlogs.

Useful non-content checks on the coordinator host:

```sh
command -v ffmpeg
command -v ffprobe
sudo -u duet test -r /etc/duet/coordinator.yml
sudo -u duet test -w /var/lib/duet/media
df -h /var/lib/duet/media
curl -fsS http://127.0.0.1:8080/healthz | jq '{status,media_processing,media_lifecycle}'
```

Adjust paths and the health address to the deployment; do not paste config or
credentials into logs or tickets.

## Rollback and roll-forward

For rollback, stop the current coordinator so there is one SQLite writer,
disable `DUET_SELF_SERVICE_ONBOARDING`, and deploy the release's recorded
immediate-predecessor artifact. Leave all additive media tables and media
directories in place: the predecessor ignores unknown schema, while dropping
tables or deleting outbox files destroys roll-forward recovery state.

The predecessor continues its legacy Telegram path. App upload/session routes
are unavailable while the feature is off, so clients must retain local drafts
and retry after roll-forward; they must not report successful delivery.
Rollback does not undo an already committed logical delete and must not be
used to resurrect media intentionally.

On roll-forward, deploy the current artifact with the feature still off first.
Startup reconciliation restores common authority over linked Telegram rows,
publication/cleanup receipts and mutations made by the predecessor. Require a
successful lifecycle sweep and converging backlogs before serving app routes.
A restored SQLite backup can contain pre-delete state and likewise must run
normal reconciliation/retention before traffic is enabled.

The build-tagged `previoushead` suite executes each schema/upload/processing/
lifecycle/integration boundary against its exact pinned predecessor. That is a
compatibility regression gate, not authorization to deploy an arbitrary source
commit instead of a recorded release artifact.

## Evidence and deeper contracts

- `docs/analysis/p1-media-ingest-acceptance-evidence.md` - deterministic story
  acceptance map and reproducible commands;
- `docs/analysis/p1-media-ingest-persistence-contract.md` - schema, CAS and
  additive migration contract;
- `docs/analysis/p1-media-upload-session-contract.md` - exact auth, quota,
  retry and temp-file rules;
- `docs/analysis/p1-submitmedia-processing-contract.md` - worker sandbox,
  canonical output, publication and failures;
- `docs/analysis/p1-media-download-target-acl-contract.md` - owner/target ACL
  and non-disclosure;
- `docs/analysis/p1-media-delete-retention-contract.md` - lifecycle,
  cancellation, backup and privacy rules; and
- `docs/analysis/p1-media-acl-delete-retention-integration.md` - Telegram and
  current-runtime compatibility plus rollback behavior.

The synthetic gate does not prove microphone capture, speaker output,
Store/MSIX runtime behavior, real network interruption, audible quality or
physical Windows/macOS behavior. Those checks remain open in
`EPIC-260714-th54l3`, principally `TASK-260712-1vtwkl`,
`TASK-260712-2hodti` and `TASK-260712-e5mfqj`.
