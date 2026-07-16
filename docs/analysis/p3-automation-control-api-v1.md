# P3 soundboard and automation control API v1

Task: `TASK-260712-1kk8bd`

This document freezes the authenticated HTTP and repository boundary layered
on `automation-safety-v1`. The control surface is registered on the existing
coordinator HTTPS API. It creates no loopback listener, webhook, callback or
outbound fetch. The scoped trigger route has a separate authentication path
and remains a generic `404` in production until the downstream runtime service
is deliberately installed.

## Shared request boundary

Every route rejects query parameters, browser `Origin`, unsupported methods,
unknown or duplicate JSON members, explicit `null`, invalid UTF-8, trailing
values and bodies above 16 KiB. Mutation requests require exactly one
`Content-Type: application/json` and one printable-ASCII `Idempotency-Key` of
8–512 bytes. Responses use `Cache-Control: no-store`; no route emits a CORS
allow header.

Control requests use an ordinary control bearer and are re-authorized as the
current same-orbit primary inside the SQLite writer transaction. Middleware
authentication is not mutation authority. Every mutation persists only the
actor-scoped idempotency-key digest, canonical request digest and a sanitized
response projection in the same transaction as the resource change. The
canonical request envelope includes the HTTP method and exact route, so a key
cannot replay a result across resource IDs. Exact replay returns the committed
resource; cross-operation, cross-route or different-request reuse returns
`409 idempotency_conflict`.

## Routes

| Method and path | Request or result |
|---|---|
| `GET /v1/soundboard/cues` | active builtin/user cue projections plus `order_revision` |
| `POST /v1/soundboard/cues` | create from one canonical app `audio_clip` or the hash-pinned recording builtin |
| `PATCH /v1/soundboard/cues/{cue_id}` | rename with `expected_revision` |
| `DELETE /v1/soundboard/cues/{cue_id}` | tombstone with `expected_revision`; shared media lifecycle owns cancellation and unpin |
| `PUT /v1/soundboard/cues/order` | exact permutation with `expected_order_revision` |
| `GET/PUT /v1/automation/status` | independent flags, emergency disable, IANA timezone and quiet hours |
| `GET/POST /v1/automation/schedules` | list or create schedules |
| `PUT/DELETE /v1/automation/schedules/{schedule_id}` | full replace or tombstone with CAS revision |
| `POST /v1/automation/schedules/{schedule_id}/enable` | arm only against the current policy revision |
| `POST /v1/automation/schedules/{schedule_id}/disable` | disarm with CAS revision |
| `GET/POST /v1/automation/principals` | list non-secret metadata or issue one immutable principal |
| `POST /v1/automation/principals/{principal_id}/revoke` | terminal revoke with CAS revision |
| `POST /v1/automation/triggers` | strict scoped boundary; no production service is composed by this task |

Schedule weekdays are integers `0..6` (`0` is Sunday), and `local_time` is
`HH:MM`. Delivery is exactly `overlay`. New schedules are created disarmed and
require the explicit enable command. A quiet-hours entry is:

```json
{"weekday":1,"start_minute":1320,"end_minute":360}
```

It is a weekly half-open local interval. `end_minute < start_minute` crosses
midnight and belongs to the start weekday. Entries are sorted canonically;
duplicates, overlaps, equal endpoints and out-of-range values are rejected.
Schedule entries are additional deny windows, so they cannot weaken the orbit
policy. Any feature-policy revision atomically disarms enabled schedules;
clients must replace/review them against the new revision before re-enable.

## Targets and principal secrets

Clients submit only current opaque `trf_` target capabilities. The writer
rechecks credential binding, expiry, Air domain and Pulsar pairing generation,
then converts each reference to a canonical subject digest. Raw target
capabilities are never stored in automation tables. Schedules retain the
generation-fenced canonical subject needed by the later runtime; principals
retain only immutable allow-list digests. Foreign, expired and unknown
references collapse to the same `audience_not_allowed` result.

Principal issuance returns a random 256-bit lowercase-hex secret only in the
first committed response. SQLite stores its versioned domain-separated digest;
the idempotency record, list response, audit/log path and history projection
contain neither secret nor digest. An exact HTTP retry returns the same
principal with `secret_available:false` and no `secret` member. A caller that
lost the first response must issue a replacement and revoke the unusable
principal; the server never recovers the credential. Revoke becomes visible in
the same serialized writer transaction.

## Scoped trigger boundary

The trigger request is the frozen cue/audience/overlay shape from
`p3-automation-safety-contract-v1.md`. It rejects cookies, query parameters,
browser origins, credentials outside the Authorization header and target URLs.
Unknown, disabled, revoked and expired secrets share
`401 invalid_automation_credential`. No runtime service is installed here, so
the production handler returns the generic route-missing `404`; rate limits,
quiet-hour admission, target snapshot creation, transmission dispatch and
revoke/disarm execution remain `TASK-260712-1eva0y`.

## Stable control errors

Malformed shapes use `invalid_request`; CAS conflicts use `revision_conflict`;
idempotency reuse uses `idempotency_conflict`. Resource lookups are orbit
scoped and collapse to `cue_not_found`, `schedule_not_found` or
`principal_not_found`. Ineligible sources use `cue_not_eligible`, quota uses
`cue_quota_exceeded`, target capabilities use `audience_not_allowed`, missing
content-policy consent uses `content_policy_acceptance_required`, and stale
control authority fails as ordinary authentication/authorization.

All evidence in this task is repository/HTTP automation. It claims no audible
playback, packaged control, local-volume behavior, physical DST transition or
real hardware result; those remain in the manual-test epic.
