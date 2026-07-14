# Overlay controller scheduler outcome

Task: `TASK-260712-31vvjt`
Accepted engineering code: `d0e1b925aa72048c243739d61bcf61fb51443ab7`
Pull request: `#26`
Hosted exact-code CI: `29331940948`

## Accepted implementation

- Added one durable coordinator scheduler per persisted orbit or approach
  playback domain. Overlay and interrupt share a FIFO ordered by trusted
  `accepted_at` and then transmission ULID, including opposite approach
  origins and equal-time ties.
- Added an additive `transmission_scheduler_state` companion with exact
  three-second prepare deadlines, durable decision/T timestamps, restart
  reconciliation and previous-HEAD-compatible backfill.
- Re-evaluated immutable binding, block, DND, liveness, capability and fresh
  per-socket RTT evidence before scheduling. The schedule is exactly
  `T = decision + max(2*maxRTT + 250 ms, 500 ms)` with the frozen 100 ms late
  window and a monotonic timer floor that cannot be extended by wall-clock
  rollback.
- Added generation-bound prepare/play/cancel routing, authenticated receipt
  ownership, partial/no-ready terminal outcomes, bounded start/end/cancel
  watchdogs, safety-stop handling for impossible receipts and strict
  delivery-expiry exclusion (`T < expires_at`).
- Added media delete/expiry, sender cancel, target revoke, orbit leave and
  approach split cleanup. A split disarms non-started targets while an already
  started target retains its bounded end watchdog.
- Kept `after_current` on the legacy Session FSM with exact target lists,
  idempotent element enqueue, exact-element cancellation and generic media ACL
  URLs. No interrupt fallback is invented by the scheduler.
- Tied RTT and receipt authority to the exact authenticated WebSocket token
  witness. Reconnect clears predecessor RTT, capability loss prevents resend,
  a revoked exact socket may acknowledge its cancellation, and a replacement
  occupant cannot mutate predecessor state or DND.

## Automated evidence

- Coordinator: `go vet ./...`, `go test ./...`, focused race suites, 20x
  shuffled scheduler stress, CGO-disabled build and exact previous-HEAD
  rollback passed locally.
- Windows compatibility: vet, unit tests, race tests and Windows amd64
  cross-build passed locally.
- macOS compatibility: release build passed locally. The selected local Swift
  toolchain still cannot import the existing `Testing` module; hosted macOS
  `node-core` is the authoritative test result.
- Hosted exact-code run `29331940948` passed coordinator, node-core,
  pulsar-win and the signed packaged-probe job.
- `task-board validate` and `git diff --check` passed.

## Explicit evidence boundary

This outcome is best-effort engineering evidence only. It does not claim
real-app playback, audible mixing, measured multi-node p95 skew, packaged
installation, Windows 10/11 behavior or any physical-hardware result. Those
checks remain in manual epic `EPIC-260714-th54l3`.

## Handoff

`TASK-260712-2qc27p` can now build the transmission regression matrix against
the persisted FIFO/barrier/restart/cancellation behavior without changing the
frozen wire contract or treating manual hardware evidence as complete.
