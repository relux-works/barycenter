# Phase 1 protocol and compatibility technical audit

- Date: 2026-07-15
- Task: `TASK-260712-176b74`
- Frozen review base: `aa86926103688473cc1d99185f627e277095f5a0`
- Review mode: rigorous inline self-audit with corrective patch
- Acceptance state: technical audit complete; independent signoff open

## Independence and evidence boundary

The requested execution mode forbids delegation and the same inline execution
chain implemented part of the protocol and scheduler under review. This report
therefore does **not** claim the acceptance criterion that the reviewer did not
implement the reviewed work. It is a reproducible technical review packet for
a genuinely separate reviewer. The task must remain open until that person
checks the patch and signs the report.

The automated evidence covers codecs, storage, HTTP resolution, scheduler
state, mixed-version behavior and deterministic client lifecycles. It does not
claim real application playback, audible continuity, physical-device timing or
packaged Windows/macOS behavior. Those observations remain in manual epic
`EPIC-260714-th54l3`.

## Reviewed contract

The normative chain is:

1. `docs/spec-self-contained-audio.md` and the root-review amendments;
2. `docs/analysis/p1-transmission-contract-v1.md` for acceptance, targeting,
   ordering, downgrade and receipt rules;
3. `docs/analysis/p1-clip-transmission-wire-contract.md` for WebSocket fields,
   conditional variants and clocks;
4. `docs/analysis/p1-transmission-rollout-handoff.md` for the supported
   mixed-version and rollback policy;
5. `protocol/golden/*.json` for the executable field-name contract.

The review enumerated all **39** message types. Coordinator Go has one typed
factory and one golden per type; Windows Go is gofmt-normalized byte-identical
to the coordinator's `protocol.go` and `codec.go`; Swift decodes and re-encodes
all 39 goldens and requires the same type count. The four capability constants
are identical across clients:

| Capability | Rollout rule |
| --- | --- |
| `media_clip_v1` | Advertised only after the authenticated clip client initializes |
| `overlay_mix_v1` | Required on every mandatory target; otherwise whole-transmission downgrade to `after_current` |
| `interrupt_resume_v1` | Required with readiness; otherwise explicit confirmation, never an invented fallback |
| `seamless_adoption_v1` | Used only when every participating peer supports it |

Unknown sorted capabilities remain additive; reconnect replaces rather than
unions the prior set. Unknown message types are ignored for forward
compatibility. A major envelope-version mismatch is not additive and must
terminate the connection before payload dispatch.

The closed delivery, target-status/reason, cancellation, DND, playback and
error vocabularies were traced from the frozen contracts into typed payloads,
store validators and runtime transition tests. Conditional fields are omitted,
not encoded as `null`; overlay and interrupt controls cannot coexist. The
legacy `play_voice` and `solo_voice` goldens and compatibility tests remain
unchanged.

## State-machine and timing audit

| Invariant | Re-derived result and executable evidence |
| --- | --- |
| Trusted order | `accepted_at` and transmission ID are coordinator-owned; HTTP rejects caller control. FIFO is `(accepted_at_ms, transmission_id)` and equal-time ULID ties are tested across delivery modes. |
| Barrier and clock | One scheduler owns each playback domain. Ready deadline is three seconds and `T = decision_now + max(2 * fresh_max_RTT + 250 ms, 500 ms)` with `start_deadline = T + 100 ms`. Multi-target max-RTT, stale clock and late-start tests pass. |
| No late autoplay | Generation-bound prepare/play/cancel, target tombstones, stale callbacks, reconnect reset and delivery-expiry tests all converge without arming old work. |
| Idempotency | Concurrent HTTP replay creates one acceptance; interrupt confirmation is bound and single-use; receipt transitions are idempotent and closed. |
| Cancellation/delete | Sender cancel, media delete/expiry, block, DND, revoke, approach leave/apart and acknowledgement timeout use exact generation-bound disarm/fade paths. |
| Mixed versions | Overlay downgrades as one transmission; interrupt requires explicit confirmation; `after_current` stays on the legacy Session bridge; capability loss never invents a fallback. |
| Telegram legacy race | Voice creates immediate `after_current`; a callback can atomically replace only not-started work with a new acceptance; after start it returns `too_late`. Concurrent callback tests remain green. |

## Findings

### P1-PROTO-001 — HIGH — closed

**Finding.** Swift rejected envelope major versions other than `1`, but both Go
runtime decoders accepted them. The coordinator could authenticate a `v=2`
`register`, and established coordinator/Windows sockets could continue after a
mixed-major frame. This contradicted `docs/protocol.md` and made payload
interpretation depend on platform.

**Correction.** Both mirrored Go decoders now reject a non-v1 envelope before
payload dispatch. Coordinator registration rejects it before credential
lookup; established coordinator sockets close. Windows reconnects. Swift's
already-strict decoder now exposes an explicitly tested runtime decision and
reconnects instead of merely ignoring the frame.

**Re-review evidence.** New tests cover strict and lenient Go decoders,
pre-auth registration, established coordinator connection, Windows read-loop
reconnect and Swift runtime classification. Focused, race and full contract
suites pass after the correction.

No other critical or high technical finding remains in this self-audit.

## Verification executed

- Coordinator protocol/hub focused tests: passed.
- Coordinator `-race` across protocol, store, session, hub and command runtime:
  passed.
- Windows wire/client focused tests and `-race`: passed.
- Swift protocol, clip-client and clock suites: 35 tests passed.
- Exact predecessor transmission-store rollback, including the pre-scheduler
  and pre-transmission-schema revisions: passed.
- Golden inventory: 39 files, all strict round-trips passed in coordinator,
  Windows and Swift; Windows mirror-source equality passed.
- Repository-wide exact-head acceptance and hosted CI are recorded separately
  after the corrective commit is frozen.

## Required independent signoff

A non-implementing reviewer must still:

1. diff the frozen base and corrective patch against the normative chain;
2. independently inspect all 39 message mappings and the closed enum tables;
3. confirm `P1-PROTO-001` is fixed without changing the forward-compatible
   unknown-type behavior;
4. record their identity, revision and approval on this task.

Until then, checklist item 1 and final task acceptance remain open, and strict
execution cannot advance to `TASK-260712-1uz0za`.
