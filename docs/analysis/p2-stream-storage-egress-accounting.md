# P2 streamed-track storage, processing and egress accounting

`TASK-260712-2ogntd` establishes the candidate-neutral accounting and quota
boundary for streamed tracks. It does not enable `stream_track_v1`, select a
production codec, publish a range endpoint or claim production traffic.

## Authoritative projections

There is one accounting path. Operator views and quota decisions read the same
SQLite authorities instead of maintaining an eventually consistent counter
cache:

| Dimension | Authority | Quota boundary |
|---|---|---|
| upload starts and input bytes | `media_upload_sessions` joined to `audio_track` media | before a new idempotent upload session is inserted |
| original and canonical retained bytes | active `stream_track_metadata` and staged/ready canonical `stream_variants` | before metadata or a staged variant is inserted |
| temporary disk and concurrent work | active `stream_processing_jobs` reservation/current/high-water fields | before a processing lease is admitted |
| range requests and actual egress | immutable `stream_egress_events` | a playback lease is reserved before the first request; actual bytes are recorded after each write |
| active egress allowance | remaining bytes in active `stream_egress_sessions` | before a new playback generation is admitted |

Delete, expiry and variant revocation disappear from the current retained-byte
projection immediately. Historical actual egress remains in its 24-hour
window. Idempotency hashes make upload, processing, playback and range retries
non-duplicating without storing plaintext request keys.

## Approved engineering defaults

The defaults are conservative engineering gates pending the later seven-day
beta calibration task. All byte values are binary bytes.

| Limit | Actor | Orbit |
|---|---:|---:|
| upload starts / 24 h | 100 | 500 |
| input bytes / 24 h | 5 GiB | 25 GiB |
| canonical retained bytes | 10 GiB | 50 GiB |
| reserved processing temp | 2 GiB | 8 GiB |
| concurrent processing jobs | 2 | 8 |
| total retained track bytes | 20 GiB | 100 GiB |
| actual plus active-reserved egress / 24 h | 100 GiB | 500 GiB |
| one admitted playback generation | at most 2x variant bytes | at most 2x variant bytes |

The existing hard item gate remains two hours and 500 MiB input. An actor or
orbit override is revision-conditional and audit logged with an operator id,
old/new structured policy and a bounded reason code. Lowering a quota blocks
new work but never terminates an already admitted playback.

## Active playback and amplification policy

`BeginStreamEgress` resolves the canonical media owner and ready immutable
variant, checks both actor and orbit 24-hour usage, and reserves twice the
variant size. The later range-serving task must use that returned session for
every authenticated conditional range.

Once admitted, quota changes cannot corrupt playback mid-stream. Each response
records requested and actual bytes plus `served`, `cache_refill`, `failed`,
`revoked` or `client_cancelled`. Repeated request keys are idempotent. Total
actual bytes cannot exceed the frozen reservation, so repeated range refill or
seek abuse fails with the distinct amplification error rather than silently
charging another tenant.

## Crash, retry and cleanup

Processing and playback reservations are durable leases. Startup reconciliation
and the coordinator's five-minute reconciliation loop expire leases whose
heartbeat/update is older than 30 minutes, release their temp/egress
reservations and increment crash-release counters. Completed, failed,
cancelled, revoked and expired rows are terminal. A retry uses a new
idempotency key; replay of an old key returns the exact terminal record.

The exact previous-coordinator rollback fixture creates metadata, a canonical
variant, an active processing lease and a partially served egress session,
runs the pinned Phase 1 predecessor, then verifies every accounting authority
after roll-forward. The predecessor ignores the additive tables.

## Privacy and operator surfaces

Accounting rows contain only actor/orbit/media/variant identifiers, byte and
request counts, outcome enums and timestamps. They have no filename, title,
caption, path, content, bearer or plaintext idempotency field. Quota errors
return only the failed dimension; they never reveal another scope's current
usage or limit.

The public `/healthz` response exposes only `ready`, `saturated` and the last
reconciliation timestamp. Exact cost/usage metrics require a moderation
operator credential with `list` capability:

- `GET /v1/moderation/stream-accounting` for aggregate storage, processing,
  range and actual-egress cost metrics;
- optional `scope_kind=actor|orbit&scope_id=...` for an authorized scope view;
- `POST /v1/moderation/stream-accounting/policies` with `decide` capability for
  a revision-conditional override;
- `GET /v1/moderation/stream-accounting/policies/audit` with `list` capability
  for the immutable adjustment audit.

Reads and mutations resolve the still-live operator token and required
capability again inside the same SQLite transaction as the view or adjustment;
middleware authentication alone is not treated as mutation authority.

Quota rejections drive the coarse saturation alert and exact authenticated
24-hour rejection/saturated-scope counters. Later observability work consumes
this surface; it must not create a parallel counter implementation.

## Deterministic evidence

Store and HTTP tests cover upload reservations, retained and canonical storage,
temporary disk, concurrent jobs, idempotent retry, crash release, actual range
bytes, cache refill, delete/retention projection, active-playback quota changes,
amplification rejection, tenant isolation, operator capabilities, audit and
public-health redaction. Relevant commands are:

```sh
cd coordinator
go test ./internal/store -run 'TestStream(Accounting|TrackUpload)' -count=1
go test ./cmd/duet-coordinator -run TestStreamAccountingHTTP -count=1
go test -tags previoushead ./internal/store -run TestMediaIngestExactPreviousHeadRollback -count=1
```
