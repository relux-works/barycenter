# P1 transmission HTTP resolution

Task: `TASK-260712-2qpp6w` — transmission-http-resolution

Frozen input: `docs/analysis/p1-transmission-contract-v1.md`

## Accepted implementation boundary

The coordinator now exposes the phase-one transmission acceptance surface:

- `POST /v1/transmissions` requires a control credential and one actor-scoped
  `Idempotency-Key`;
- `GET /v1/transmissions/{id}` accepts an actor credential and applies the
  creator, current source-orbit and exact snapshotted-target views; and
- `POST /v1/transmissions/{id}/cancel` requires a control credential and the
  strict empty object `{}`.

All three paths reject query parameters. Create and cancel use a 16 KiB strict
JSON boundary which rejects non-object bodies, invalid UTF-8, explicit nulls,
unknown or duplicate fields and trailing values. Create accepts only the
frozen media, audience, origin and delivery enums. `queue` and `replace` remain
recognized but disabled. The coordinator captures `accepted_at` after bearer
resolution and before body-dependent repository work; no request field can
supply ordering time.

The canonical idempotency digest includes every original semantic field and
the resolved `include_origin` default, but excludes the fallback proof. Only
SHA-256 digests of the key and canonical request are stored. An exact committed
retry returns the original transmission and acceptance timestamp, while a key
reuse with different input fails without re-resolving media or audience.

## Atomic media and audience resolution

The store reauthenticates the exact control bearer and performs media checks,
domain discovery, selector expansion, policy evaluation, capability decisions,
confirmation consumption, request idempotency and snapshot creation in one
immediate SQLite writer transaction.

`this_pulsar`, `own_barycenter`, `current_air` and explicit Barycenter/Pulsar
selectors resolve only live actor, membership, orbit, slot and credential
bindings. The current pairwise approach ID is persisted as the playback domain;
unknown, empty or out-of-domain explicit selectors fail closed. Duplicate
selectors are collapsed before origin filtering. Corrupt state containing more
than one active approach, or an invalid active link, is rejected rather than
selecting one nondeterministically.

Media lookup is orbit-scoped. Unknown, cross-orbit, deleted and expired media
share the non-disclosing `media_not_found` result; an authorized item which is
not ready returns `media_not_ready`. Phase-two tracks cannot enter a phase-one
clip delivery. Microphone defaults to excluding the authenticated origin;
file, Telegram and built-in origins default to including it, and
`this_pulsar` cannot be combined with exclusion.

Each accepted row freezes the actor, orbit, slot, binding generation, online
decision, capability decision, block/DND result and delivery inputs. Block is
evaluated before DND, then online state and capability. Presence must belong to
the current authenticated socket and be within the frozen 12-second window.
`messages_only` permits user clips but suppresses built-in cues; only an exact
local `this_pulsar` intent bypasses DND.

## Whole-delivery and confirmation semantics

If any online mandatory target lacks exact clip/overlay capability, the single
stored `effective_delivery` becomes `after_current` with
`mandatory_target_missing_overlay_capability`. Targets are never split across
delivery modes. Direct overlay longer than 60 seconds is rejected with ordered
interrupt and after-current alternatives.

An interrupt capability or runtime-resume gap creates no transmission, target,
idempotency or FIFO row. Instead the coordinator returns ordered overlay and
after-current alternatives plus a 256-bit opaque confirmation token. Only the
token digest is retained, for five minutes, bound to actor, idempotency key,
canonical request and the alternatives actually offered.

Confirmation repeats the unchanged original input, consumes the proof once and
receives a fresh coordinator acceptance timestamp. Availability is rechecked
inside the transaction. Improvement does not override the sender's selected
fallback; deterioration consumes the old proof, creates a fresh challenge and
schedules nothing. Wrong actor, key, request, selection, expiry and replay all
collapse to `confirmation_invalid`. A committed transmission remains normally
idempotent even when the retry omits the already-consumed proof.

## Safe reads and cancellation seam

The creator and a current source-orbit primary see all target rows. Other
current source-orbit actors and exact current bindings from the immutable
snapshot see aggregate counts and only their own rows. Everyone else receives
the same not-found result. Responses expose stable status/count fields and
target timing, but never actor IDs, credential generations, capabilities,
tokens, URLs, names or paths. A blocked row does not reveal whether the hidden
rule was actor- or orbit-scoped.

Sender cancellation is authorized only for the creator or a current
source-orbit primary and only before any durable `playing` or `played` row.
The writer transaction decides the start/cancel race, directly cancels unsent
rows and moves prepared, ready or scheduled rows to `cancelling`, returning
their exact persisted generations as `DisarmTargets`. Repeating the accepted
sender cancellation is stable and reports `changed=false`.

This task deliberately stops at that durable scheduler seam. The later
`TASK-260712-31vvjt` controller must consume `DisarmTargets`, emit and reconcile
generation-bound `cancel_media(action=disarm)`, recheck capability at the FIFO
barrier and implement the remaining start/restart races. No node command or
acknowledgement is claimed here.

## Automated evidence

- Coordinator `go vet ./...` and `go test ./...` pass, including legacy-schema
  migration, injected rollback and the existing previous-head transmission
  round trip.
- Race-enabled store, hub and HTTP suites pass.
- Focused tests cover mixed capability fleets, origin defaults, explicit
  selector deduplication and fail-closed errors, block/DND precedence, safe
  visibility, cancellation authority/races, concurrent idempotency and every
  confirmation binding/replay branch.
- Windows `go vet ./...`, `go test ./...`, race-enabled wire/root tests and
  `GOOS=windows GOARCH=amd64 go build ./...` pass.
- `swift build`, `git diff --check` and `task-board validate` pass. Swift emits
  the repository's pre-existing Sendable warnings; this task changes no Swift
  source.

No real-app, speaker, packaged-install or physical-hardware result is claimed.
Those checks remain deferred in manual epic `EPIC-260714-th54l3`.

## Downstream handoff

- `TASK-260712-26ip33` and `TASK-260712-2bbz13` must implement the real media
  preparation/play/cancel hooks before advertising the new capability flags.
- `TASK-260712-31vvjt` owns the domain FIFO, three-second barrier, RTT start,
  capability recheck, disarm delivery and restart reconciliation.
- Presence work must replace the current conservative authenticated-hub bridge
  with explicit runtime `main_active` and `interrupt_resume_ready` signals
  without weakening the 12-second/binding boundary.
- Telegram/history adapters may call the same store boundary, but cannot add a
  caller timestamp, bypass field, per-target delivery split or implicit
  interrupt fallback.
