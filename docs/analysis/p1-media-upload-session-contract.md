# P1 authenticated resumable media upload sessions

`TASK-260712-1bnos4` adds the app-facing upload boundary for generic media. It intentionally stops at the durable `finalizing` state; validation, probing and canonical media publication belong to `TASK-260712-2af2dp`.

## HTTP contract

### Create or replay a session

`POST /v1/media/uploads`

- Authentication: one `Authorization: Bearer <control-token>` header. Node-only and satellite authority is rejected.
- Idempotency: one `Idempotency-Key` header matching 16–128 safe ASCII characters.
- Body: strict JSON with `kind` (`voice_clip` or `audio_clip`), optional `title`, and positive `size_bytes`.
- Success: `201` for a new row and `200` for an idempotent replay. Both return `upload_id`, `media_id`, the scoped `upload_token` while the session is active, offset, length, expiry and status.
- `Location` contains only `/v1/media/uploads/{upload_id}`. Neither the control token nor scoped token is placed in a URL.

The store re-resolves the control credential, active slot, membership and non-satellite role inside the same SQLite writer transaction that checks idempotency, reserves quota and creates the rows. A stale credential accepted by HTTP middleware therefore cannot win a mutation race.

The scoped token is HMAC-SHA-256 derived from the high-entropy control token plus the orbit, actor and hashed idempotency identity. SQLite stores only SHA-256 digests. This makes a retry reproducible without plaintext persistence; a current control credential can remint the scoped capability after control rotation, invalidating the old scoped token.

### Append or finalize bytes

`PUT /v1/media/uploads/{upload_id}`

- Authentication: the session-scoped bearer token, never the long-lived control token.
- Required headers: canonical decimal `Upload-Offset`, `Content-Type: application/octet-stream`, and a known `Content-Length`; transfer encoding is not accepted.
- Writes are append-only. A stale or reordered offset returns `409 upload_offset_conflict` with the authoritative persisted offset.
- A body larger than the remaining declared bytes returns `413 upload_too_large` before the canonical temp file is touched.
- Actual body bytes must equal `Content-Length`; short or extra bodies return `400 upload_length_mismatch`.
- Reaching the declared length atomically transitions the session to `finalizing`. A zero-byte request at the final offset recovers a crash between offset persistence and finalization. A later repeated final request returns the existing finalizing/completed result without appending again.

Unknown IDs, wrong path/token pairs, malformed scoped tokens, expired tokens and terminal failed/expired sessions all collapse to the same non-disclosing `401 upload_credential_invalid` response.

## Phase-one quotas

The creation transaction enforces the defaults below for both actor/orbit safety where applicable:

- at most 10 upload starts per rolling minute;
- at most 3 processing media items concurrently;
- at most 1 GiB of new upload bytes per orbit per rolling 24 hours;
- at most 50 MiB declared bytes per item.

Open and finalizing sessions reserve their full declared size, preventing concurrent oversubscription. Terminal failed/expired rows count their actual received bytes. An idempotent replay is resolved before quota evaluation and does not consume a second reservation.

## Temp-file durability and cleanup

Upload storage is `${media_dir}/.uploads`, forced to mode `0700`; session files and staging chunks use mode `0600`. Paths are derived only from validated server-generated upload IDs.

Each request first streams into a random staging file. Under a bounded per-session lock, the coordinator reconciles the canonical `.part` file to the persisted offset, appends and fsyncs the bytes, then advances SQLite with compare-and-swap semantics.

- Crash after file append but before DB commit: the next request truncates the uncommitted tail to the persisted offset.
- Crash after DB offset commit: the fsynced bytes already match the durable offset.
- Persisted offset ahead of the file: the session fails closed as `upload_temp_missing`; it is never made ready.
- Crash during staging: startup removes orphan `.chunk-*` files; scheduled maintenance also removes stale chunks.
- Expired open/finalizing session: media becomes failed with sanitized code `upload_expired`, the session becomes expired, and its `.part` file is removed.
- Crash between file removal and DB acknowledgement: removal is idempotently retried and `temp_cleaned_at` is committed afterward.

Startup performs reconciliation immediately. Production also runs bounded maintenance every 15 minutes; authenticated traffic opportunistically performs the same bounded pass.

Filesystem failures are logged only as fixed operation classes. Credentials, idempotency keys, request titles, filenames and absolute local paths are never included in upload logs.

## Verification

Coverage includes HTTP success and stable-error contracts, credential-domain separation, middleware/store authorization races, idempotent concurrency, control rotation, rate/concurrency/daily/hard-byte quotas, monotonic and competing offsets, actual-length enforcement, repeated finalization, crash-tail recovery, real store reopen, expiry and durable cleanup, orphan staging cleanup, secret/path redaction, race-detector execution, and an exact rollback/roll-forward test against predecessor commit `31bbeb9257b2555c86858c4087521466b58d673a`.
