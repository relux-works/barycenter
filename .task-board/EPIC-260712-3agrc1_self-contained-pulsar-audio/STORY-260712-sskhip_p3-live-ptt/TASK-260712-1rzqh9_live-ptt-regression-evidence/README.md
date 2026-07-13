# Prove C1-C2 live PTT safety, latency and intelligibility

## Description
Close live PTT with deterministic, hostile and real two-home evidence across all platform pairings.

## Scope
Run 100 press, repeat, hold, release and lost-release cycles from varied foreground apps plus hidden, lock, suspend or sleep, quit, permission revoke, device loss and reconnect. Measure acoustic or calibrated mouth-to-ear p50 at most 800 ms and p95 at most 1500 ms with frozen sample counts on two real home networks for Windows-Windows, Windows-macOS and macOS-macOS. Inject 2 percent loss, jitter, reordering, slow recipient and coordinator backpressure; evaluate intelligibility by the agreed metric, bound jitter and coordinator memory and verify one-speaker serialization, DND, partial targets, toggle fallback and main-program recovery. Record build hashes and rollback.

## Acceptance Criteria
C1-C2 have raw sanitized evidence. No cycle sticks capture, no stale session or old frame plays, latency gates and intelligibility pass under 2 percent loss, every buffer remains bounded and main program returns cleanly. A failed platform or environment is marked toggle-only or blocks promotion; hardware-only and seven-day work is handed to acceptance exactly.
