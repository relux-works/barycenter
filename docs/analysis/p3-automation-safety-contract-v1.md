# P3 soundboard and automation safety contract v1

Task: `TASK-260712-3sj8ox`
Contract identifier: `automation-safety-v1`

This document closes the phase-three automation-surface decision before any
schema, listener or scheduler is added. It is normative for the soundboard and
automation story. The executable vocabulary lives in
`coordinator/internal/automation/contract.go`.

## 1. Threat model and selected surface

The first supported external automation entry point is the coordinator's
existing authenticated HTTPS control plane:

```text
POST /v1/automation/triggers
Authorization: Bearer <one scoped automation secret>
Idempotency-Key: <8..512 visible ASCII bytes>
Content-Type: application/json
```

The server accepts no query parameters, cookies, form encoding, redirect or
credential in a URL. The ingress must terminate authenticated production TLS;
the handler trusts the same reviewed proxy boundary as the other coordinator
APIs. It emits no CORS allow headers and rejects requests carrying a browser
`Origin` header. The strict JSON body is limited to 16 KiB and rejects duplicate
or unknown fields, `null`, invalid UTF-8 and trailing values.

There is no loopback listener and no webhook receiver in v1. A loopback API
would add a browser-CSRF/DNS-rebinding boundary and a second desktop listener;
a webhook would add unauthenticated-internet ingress, replay and callback-secret
handling. The chosen HTTPS API reuses identity, target ACL, Air policy,
idempotency, rate limits, audit and the ordinary transmission service. This
contract also forbids outbound callbacks, user-supplied URLs and server-side
fetches, so automation cannot become an SSRF surface.

Schedules are created by a current same-orbit primary through the later control
API and are claimed internally by the coordinator. They do not call the public
trigger route with a stored bearer. Desktop manual soundboard actions use the
ordinary control credential and a separate soundboard route/feature flag; they
never mint an automation principal implicitly.

## 2. Trigger shape and hard media boundary

The scoped API request is exactly:

```json
{
  "cue_id":"cq_01J00000000000000000000000",
  "audience":{"kind":"own_barycenter"},
  "delivery":"overlay"
}
```

It contains no media ID, source URL, file path, bytes, schedule time, caller
timestamp, volume, ducking override, priority, force, emergency or DND bypass.
The principal may trigger only a saved `cue_id` in its immutable allow-list.

An eligible saved cue is exactly one of:

- a durable owner-orbit reference to a canonical `ready` `audio_clip`; or
- a durable reference to a shipped `builtin_cue` whose asset digest matches its
  reviewed hash pin.

Both forms reuse common media ACL, rights disable, report, quota and delete
services. Saving a cue does not copy bytes into an automation-only store and
does not make deleted, expired, disabled or unready media playable.

The following are always rejected as `cue_not_eligible`: `voice_clip` from any
source, microphone recordings, Telegram voice, `audio_track`, stream variants,
live PTT sessions, arbitrary media IDs and arbitrary URLs. Automation contains
no dependency on a capture interface and can never request microphone
permission, open an input device or invoke a capture sender. Converting a
recorded voice clip into an `audio_clip` merely to cross this boundary is also
forbidden.

V1 automation delivery is only `overlay`. `interrupt`, `after_current`, `queue`
and `replace` are rejected rather than confirmed or downgraded: a non-interactive
caller cannot approve an interrupt fallback, and delayed queue playback could
escape the quiet-hours decision. The ordinary manual soundboard may use the
existing user-confirmed clip delivery contract, but remains a distinct trigger
kind and flag.

## 3. Principals and least privilege

Only a current primary using a control credential may issue an automation
principal for its own orbit. The response shows a random 256-bit secret once.
Only a versioned keyed hash is stored; logs, history, audit and list responses
contain neither the secret nor its hash. A principal record freezes:

```text
principal_id, owner_orbit_id, issued_by_actor_id, display_name,
allowed_cue_ids, allowed_audience_kinds, allowed_target_refs,
bound_air_id, max_target_count, issued_at, expires_at,
disabled_at, disabled_by_actor_id, revoked_at, revoked_by_actor_id
```

The only v1 permission is `automation:trigger`. A principal cannot create or
edit cues, schedules, principals, DND, Air membership, content policy or
blocks. It cannot read media bytes or history. Expiry is required and is at
most 90 days after issue. Allowed cue IDs and exact target references are
immutable; narrowing requires issuing a replacement and revoking the old
principal.

`disabled_at` is a reversible operator pause. `revoked_at` is terminal. Both
take effect in the serialized writer transaction. Unknown, disabled, revoked
and expired secrets all return `401 invalid_automation_credential`; the exact
internal reason remains in privacy-bounded audit as `principal_disabled`,
`principal_revoked` or `principal_expired`.

Revoke and quick-disable race semantics are fail-closed. Every claim
reauthenticates the principal and feature state in the same transaction that
creates the execution. If revoke commits first, the claim is denied. If a
claim commits first, the subsequent revoke marks all of that principal's
accepted, preparing, ready, scheduled or playing automation executions for the
normal generation-safe cancel/disarm path; active audio fade-stops. A trigger
cannot survive the revoke transaction merely because its HTTP request started
earlier. Orbit quick-disable applies the same rule to all automation in that
orbit. Manual soundboard remains separately controllable.

## 4. Audience and policy precedence

Automation accepts only:

- `own_barycenter`, bound to the principal's owner orbit;
- `current_air`, only when the principal is bound to the exact Air ID and the
  owner orbit is still an authorized member; or
- `explicit`, one to 64 opaque Barycenter/Pulsar target references, each present
  in the principal's immutable allow-list.

`this_pulsar` is intentionally absent. A scoped principal is not an exact
installation-local user gesture and therefore never obtains the local DND
exception. An explicit selector for the same Pulsar remains remote automation
and respects DND. Unknown, stale, out-of-domain, transitive or unauthorized
targets do not expand the request and collapse to `audience_not_allowed`.
Every accepted execution resolves the selected audience into the standard
immutable transmission target snapshot; later Air joins never expand it.

Admission precedence is exact:

1. `automation` feature and orbit quick-disable;
2. credential resolution, expiry, disabled and revoked state;
3. idempotency replay/conflict;
4. principal/orbit attempt rate and concurrency limits;
5. `automation:trigger`, cue and audience scope;
6. cue existence, ready state, eligible kind, rights and moderation state;
7. source-orbit quiet hours;
8. target selector and current Air policy resolution;
9. recipient block;
10. recipient effective DND;
11. online state and exact node/delivery capabilities;
12. ordinary transmission barrier, playback receipts and cancellation.

Quiet hours are evaluated before target expansion, so a skipped event does not
leak recipient state. Once outside quiet hours, the existing per-target
block-before-DND-before-online precedence is unchanged. Blocked rows remain
`blocked/actor_blocked` or `blocked/orbit_blocked`; DND rows remain
`missed_dnd/local_dnd` or `missed_dnd/orbit_dnd`; a later block/DND uses
`cancelled/sender_blocked` or `cancelled/dnd_enabled`. No automation request,
principal or administrator action can loosen recipient DND or the local volume
ceiling. The recipient mixer applies its local ceiling last.

## 5. Quiet hours, schedules and at-most-once identity

Enabling automation for an orbit requires an explicit IANA timezone and an
explicit quiet-hours policy. Absence or invalidity keeps automation disabled;
an empty window set must be deliberately stored, not inferred. Windows are
weekly half-open local-time intervals `[start,end)` at minute precision. A
cross-midnight interval belongs to its start weekday. A schedule may add
stricter windows but cannot weaken the orbit policy. API and schedule triggers
both use the same effective policy; manual soundboard gestures do not.

A schedule stores its own IANA timezone, local minute, weekday set, revision
and enabled state. Its occurrence key is:

```text
schedule_id / schedule_revision / local_date / local_HH:MM
```

The coordinator atomically inserts this unique claim before creating a
transmission. Concurrent ticks, restart and a backward wall-clock jump cannot
create a second execution. A spring-forward local minute that does not exist is
skipped. During a fall-back fold, only the first occurrence (the earlier UTC
instant) may claim the key; the repeated wall minute is a duplicate. The
scheduler claims only while that local minute is current. Missed minutes are
never caught up after restart, clock jump, quiet hours, disabled state, DND or
policy denial.

API idempotency is scoped to the resolved principal. The key and canonical
request are stored only as digests. Exact committed replay returns the original
sanitized execution without creating work; reuse with another request is
`409 idempotency_conflict`. Principal state is rechecked before replay, so a
revoked credential cannot use replay as an authenticated read channel.

The v1 runaway bounds are five authenticated non-replay attempts per principal
per rolling minute, twenty per owner orbit per rolling hour, one nonterminal
execution per principal and two per orbit. Every request or due schedule tick
that reaches this guard reserves a position even when a later cue, quiet-hour,
audience or capability check rejects it. A resolved audience never exceeds 64 target
installations or the principal's smaller `max_target_count`. Rejections are
audited as `too_many_attempts` or `execution_in_progress` and include a bounded
`Retry-After` where applicable. Failed attempts consume the limiter reservation
so invalid retries cannot bypass it.

## 6. Exact execution and denial vocabulary

Every request or schedule attempt creates at most one privacy-bounded audit row
with these immutable attribution fields:

```text
execution_id, trigger_kind(manual_soundboard|scoped_api|schedule), owner_orbit_id,
principal_id|null, schedule_id|null, schedule_revision|null,
issued_by_actor_id, cue_id, cue_revision, media_id_or_builtin_digest,
audience_kind, selector_digest, resolved_target_count,
delivery, idempotency_digest|null, occurrence_key|null,
feature_revision, policy_revision, claimed_at, transmission_id|null,
outcome, reason_code, completed_at
```

History exposes display-safe principal/schedule labels and trigger kind only to
the same viewers already authorized for the transmission. It never exposes a
secret, secret hash, raw selector set, hidden block scope, local path or media
URL.

Pre-target denial reasons are exactly:

| Reason | Meaning |
| --- | --- |
| `automation_disabled` | global/orbit flag off or quiet policy unavailable |
| `invalid_automation_credential` | public collapse for unknown/disabled/revoked/expired secret |
| `principal_disabled`, `principal_revoked`, `principal_expired` | internal audit-only credential state |
| `idempotency_conflict` | same principal/key, different canonical request |
| `insufficient_scope` | permission or cue scope absent |
| `cue_not_found`, `cue_not_ready`, `cue_not_eligible` | privacy-safe cue admission failure |
| `quiet_hours` | effective source quiet window active |
| `too_many_attempts`, `execution_in_progress` | runaway guard |
| `audience_not_allowed`, `air_policy_denied` | selector/principal/Air policy failure |
| `automation_capability_missing`, `delivery_capability_missing` | mixed-version fail-closed result |

Schedule-only misses use the same reason in audit and create no transmission.
After a target snapshot exists, the ordinary transmission status/reason
vocabulary is authoritative. Revoke/disable cancellation adds the explicit
causes `principal_revoked`, `schedule_disabled` and `automation_disabled` to
the later schema and protocol work; those tasks must not translate them to a
user cancellation.

## 7. Mixed versions, flags and rollout

`soundboard_cues` and `automation` are independent. A manual cue can ship while
automation is disabled. The trigger route is unregistered and returns the same
generic `404` while `automation` is off; schedule ticks only record a bounded
disabled audit and create no transmission. Disabling the flag uses the same
cancel/disarm rule as orbit quick-disable.

Automation reuses media transport but requires the later
`automation_cue_v1` node capability in addition to `media_clip_v1` and the
exact overlay capability. This capability means the node preserves automation
attribution/cancel behavior; it is not a new playback command. A selected online
target without either capability rejects the whole execution as
`automation_capability_missing` or `delivery_capability_missing`. There is no
legacy `play_voice`, `after_current`, plaintext, per-target or manual-soundboard
downgrade. Offline targets use the normal `missed_offline` receipt and never
autoplay on reconnect.

Rollout remains additive: schema first, coordinator with route/claims dark,
node releases withholding `automation_cue_v1` until their path is complete,
internal orbit, mixed-version checks, then bounded orbit enablement. Rollback
withdraws the route and schedule claims first, drains or disarms nonterminal
automation, and leaves all additive principal, schedule, claim and audit rows
intact. No production behavior is enabled by this contract task.

## 8. Downstream implementation obligations

- `TASK-260712-hb5xz2` owns the durable eligible cue reference and shared media
  lifecycle; it may not broaden the kind matrix.
- `TASK-260712-3sv87k` owns additive principal/schedule/execution/claim schema
  and unique occurrence/idempotency constraints.
- `TASK-260712-1kk8bd` owns strict control and trigger HTTP boundaries, one-time
  token display and origin/CORS rejection.
- `TASK-260712-1eva0y` owns atomic claims, policy precedence, rate/concurrency
  bounds, revoke/disable disarm and the no-capture dependency test.
- `TASK-260712-11e4e3` owns privacy-bounded history/audit and quick-disable.
- Platform and Telegram tasks may render/control this model but cannot mint a
second token type, listener, webhook, bypass or fallback.

The accepted implementation detail for `TASK-260712-11e4e3` is frozen in
`p3-automation-history-audit-v1.md`: canonical history enrichment, terminal
denial items, append-only control attribution, and revision/idempotency-bound
quick actions all reuse the shared transmission and automation authorities.

Unit, integration, race and contract tests may prove deterministic repository
behavior. Real scheduled audible playback, packaged-app controls, local volume,
DST on supported devices and physical quick-disable remain in the dedicated
manual testing epic `EPIC-260714-th54l3`; this task claims none of them.
