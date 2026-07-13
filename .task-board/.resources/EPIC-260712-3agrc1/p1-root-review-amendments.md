# Phase 1 decomposition — root review amendments

Date: 2026-07-12. The task cards named below supersede any conflicting wording
in the initial solution-architect diagrams or handoff notes.

## Windows platform and capture

- `TASK-260712-1vtwkl` is a hard gate for the Windows capture engine. Evidence
  must come from the intended signed AppContainer MSIX on real Windows 10 and
  11, not an unpackaged or developer-mode-only probe.
- Capture, exact five-second record-then-play self-test, file intake, shortcut
  ownership, and final platform integration are separate tasks.
- Self-test recordings are disposable. A finalized user recording that could
  not upload is a durable app-private draft and survives restart until retry,
  confirmed upload, or explicit delete. Partial/cancelled captures are removed.
- Start/stop cues are outside committed microphone samples. Escape is scoped to
  a focused Pulsar surface; hidden recording is cancelled from tray/menu bar,
  not by a global bare-Escape hook.

## Identity

- Recovery material is shown once and not silently saved beside the credential
  it is meant to recover. Recovery reissues the control credential only.
- Invite, recovery, and Telegram-link codes have uniform errors, attempt
  limits, atomic single-use semantics, replay/concurrency tests, and complete
  secret/link/clipboard/pasteboard redaction.

## Ingest, ACL, and deletion

- Upload sessions use expiring scoped tokens, monotonic concurrency-safe
  offsets, actual-byte limits, idempotent finalize, and restart-safe cleanup.
- Untrusted probe/transcode workers have network protocols disabled and strict
  CPU, memory, time, argument, and output limits. Canonical storage publishes
  atomically; stale workers cannot publish terminal media.
- Media authorization is a dedicated owner/immutable-target-snapshot service.
  Delete/retention is a separate retry-safe worker. Their final integration
  waits for transmission target persistence and cancellation hooks.
- The transmission contract must freeze one sender-delete behavior for queued,
  prepared, scheduled, and already-playing media. Moderation uses the same rule.

## Transmission and realtime audio

- `accepted_at` is coordinator-owned; callers cannot jump FIFO order.
- One scheduler owns overlay and interrupt for the entire effective playback
  domain: an orbit while separate, a stable approach identity while joined.
- Ready deadline is three seconds and
  `T = now + max(2*maxRTT + 250 ms, 500 ms)`.
- If an overlay target lacks capability, the whole transmission has one visible
  `after_current` downgrade. An unsupported interrupt is different: nothing is
  scheduled until the user explicitly confirms overlay or after-current.
- Realtime output is
  `limiter(main * duck + overlay * overlay_gain + cues)`, followed by local
  master gain/ceiling. Defaults are -12 dB, 250 ms attack, and 600 ms release;
  pre-duck starts before the first clip sample.
- Render callbacks have instrumented guards against allocation, I/O, waits, and
  blocking locks. Prepared clips use generation-safe state. Interrupt resume is
  anchored to audible position including buffered frames and never invents a
  fallback.

## Telegram, history, presence, and moderation

- Legacy Telegram voice is enqueued as default `after_current` immediately
  when ready; there is no new decision-window delay. A callback may atomically
  replace only a not-started default and receives a new coordinator acceptance
  time. A race after start returns `too_late` without duplicate playback.
- Callback data is opaque, integrity-protected, actor/role-bound, expiring,
  replay-safe, and idempotent. Telegram metadata remains an untrusted hint.
- History is ActorContext-scoped, paginated, tenant-isolated, and exposes only
  authorized target detail/action capabilities. Replay is a new explicit
  transmission with newly resolved targets, never inbox autoplay.
- Local DND is stronger than orbit/remote policy and has no emergency bypass.
  Presence never exposes microphone/capture/process/device details or raw IDs.
- Moderation has separate least-privilege operator auth, audits every evidence
  read/action, and delegates block, delete, disable, revocation, disconnect, and
  cancellation to canonical services.

## Store gate

- `docs/analysis/store-policy-baseline-2026-07-12.md` records the official
  policy snapshot; it must be refreshed immediately before submission.
- Public legal/contact/hosting facts are explicit user inputs. No placeholder
  policy, mailbox, jurisdiction, SLA, or Partner Center authority is published.
- The current missing Swift `Testing` module is repaired by a pinned toolchain;
  it is not waived.
- Product `9P26FDCWV1GC` gets real locale-specific primary-function screenshots
  and certification notes for the accountless A1 path. Final submission waits
  for root line-by-line review, independent security/protocol/migration/audio
  reviews, all A1-A8 gates, and the approved external-submit authority.
